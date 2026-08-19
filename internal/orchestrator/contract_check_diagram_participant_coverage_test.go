package orchestrator

import (
	"context"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRunV2BlockOracles_PreservesTypedParticipantOrExplicitUnprovenBoundary(t *testing.T) {
	participants := []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "MutableState", Role: types.DiagramParticipantIncidentRequired},
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: participants},
	}
	mut := types.NewMutableState("typed participant coverage")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID: "analyzer-explorer", Producer: types.EvidenceProducerExplorerEmitEvidence, Kind: types.EvidenceRelationship,
		Subject: "Analyzer", Predicate: "calls", Object: "Explorer",
		Source: "internal/agent/analyzer.go", LineStart: 1,
		AnchorKind: types.AnchorCall, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
	}})
	bus := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: mut}
	view := &types.AnswerSemanticView{
		Family: types.QFGeneric, RelationAxis: types.AxisFlow,
		DiagramPlan:                   &types.DiagramFacetGraph{Kind: types.DiagramFlow, Required: true},
		DiagramParticipantObligations: participants,
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Body: "flowchart LR\n A[\"Analyzer\"] --> E[\"Explorer\"]\n M[\"MutableState\"]"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "E", RelationKind: types.DiagramRelCall}},
	}}}

	hasParticipantViolation := func(violations []types.Violation) bool {
		for _, violation := range violations {
			if violation.Kind == types.ViolDiagramParticipantCoverage {
				return true
			}
		}
		return false
	}
	if violations := runV2BlockOraclesWithOracleContext(context.Background(), doc, view, mut, nil, nil, bus); !hasParticipantViolation(violations) {
		t.Fatalf("post-emit chokepoint accepted a required participant with neither relation nor boundary: %+v", violations)
	}

	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
		Participant: "MutableState", Status: types.DiagramParticipantBoundaryUnproven,
	}}
	// Exercise the production pre-emit -> MutableState persistence -> post-emit
	// path. A direct-doc assertion alone misses clone omissions and allowed the
	// accepted typed boundary to disappear before the orchestrator gate.
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	persisted := mut.AnswerDocumentV2()
	if persisted == nil || len(persisted.Blocks) != 1 || len(persisted.Blocks[0].ParticipantBoundaries) != 1 {
		t.Fatalf("participant boundary was lost across AnswerDocumentV2 persistence: %+v", persisted)
	}
	if violations := runV2BlockOraclesWithOracleContext(context.Background(), persisted, view, mut, nil, nil, bus); hasParticipantViolation(violations) {
		t.Fatalf("model-authored exact unproven boundary should satisfy participant coverage without an invented edge: %+v", violations)
	}
	if len(persisted.Blocks[0].EdgeAnchors) != 1 {
		t.Fatalf("participant coverage check mutated model relations: %+v", persisted.Blocks[0].EdgeAnchors)
	}
}

func TestRunV2BlockOracles_DiagramParticipantCoverageNeverEntersTrace(t *testing.T) {
	participants := []types.DiagramParticipantHint{{Identity: "UI", Role: types.DiagramParticipantIncidentRequired}}
	rm := types.RequestModel{
		Intent: types.IntentTrace, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: participants},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFRootCauseTrace, RelationAxis: types.AxisFlow,
		DiagramPlan: &types.DiagramFacetGraph{Kind: types.DiagramFlow, Required: true},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "trace", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Body: "flowchart LR\n UI[\"UI\"]"},
	}}}
	bus := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	for _, violation := range runV2BlockOraclesWithOracleContext(context.Background(), doc, view, nil, nil, nil, bus) {
		if violation.Kind == types.ViolDiagramParticipantCoverage {
			t.Fatalf("source-flow participant contract leaked into Trace causal authority: %+v", violation)
		}
	}
}
