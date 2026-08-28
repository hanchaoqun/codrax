package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
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
	if receipt, ok := tool.AcceptedDiagramParticipantCoverageReceiptWithRuntimeContext(bus, doc, view, mut.EmittedEvidence()); ok {
		t.Fatalf("rejected participant coverage must not mint an accepted receipt: %+v", receipt)
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
	receipt, ok := tool.AcceptedDiagramParticipantCoverageReceiptWithRuntimeContext(bus, persisted, view, mut.EmittedEvidence())
	if !ok || receipt.Version != 1 || receipt.Required != 3 || receipt.Covered != 3 || receipt.UnprovenBoundaries != 1 {
		t.Fatalf("accepted participant/boundary shape lost its typed coverage receipt: ok=%t receipt=%+v", ok, receipt)
	}
	if fields, ok := acceptedDiagramParticipantCoverageReceiptLogFields(true, mut, &Orchestrator{busCtx: bus}, view); !ok ||
		!strings.Contains(fields, "version=1 status=accepted required=3 covered=3 unproven_boundaries=1") {
		t.Fatalf("accepted receipt was not projected onto the control-plane log shape: ok=%t fields=%q", ok, fields)
	}
	if fields, ok := acceptedDiagramParticipantCoverageReceiptLogFields(false, mut, &Orchestrator{busCtx: bus}, view); ok || fields != "" {
		t.Fatalf("failed whole contract must not publish an accepted coverage receipt: ok=%t fields=%q", ok, fields)
	}
	if len(persisted.Blocks[0].EdgeAnchors) != 1 {
		t.Fatalf("participant coverage check mutated model relations: %+v", persisted.Blocks[0].EdgeAnchors)
	}
}

func TestRunContractCheckPublishesAcceptedDiagramParticipantCoverageReceipt(t *testing.T) {
	source, err := os.ReadFile("contract_check.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "acceptedDiagramParticipantCoverageReceiptLogFields(result.Passed, mut, o, diagramParticipantCoverageView)") {
		t.Fatal("runContractCheck no longer publishes the accepted typed diagram participant receipt")
	}
}

func TestRunV2BlockOracles_RequiresWholeRelationScopeForDisconnectedLocalIslands(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	participants := []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: participants},
	}
	mut := types.NewMutableState("whole requested relation scope")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID: "carrier-local-pair", Producer: types.EvidenceProducerExplorerEmitEvidence,
		Kind: types.EvidenceRelationship, Subject: "BusContext.SetMutable", Predicate: "calls", Object: "Mutable.Load",
		Source: "internal/types/context.go", LineStart: 96, AnchorKind: types.AnchorCall,
		Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
	}})
	bus := &types.BusContext{RepoRoot: repoRoot, Mode: types.ModeRead, AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: mut}
	view := &types.AnswerSemanticView{
		Family: types.QFGeneric, RelationAxis: types.AxisFlow,
		DiagramPlan:                   &types.DiagramFacetGraph{Kind: types.DiagramArchitecture, Required: true},
		DiagramParticipantObligations: participants,
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Body: "flowchart LR\n Analyzer --> Explorer --> Extractor --> Finalizer\n BusContext --> Mutable"},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "Analyzer", ToNode: "Explorer", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "Explorer", ToNode: "Extractor", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "Extractor", ToNode: "Finalizer", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "BusContext", ToNode: "Mutable", FromIdentity: "BusContext.SetMutable", ToIdentity: "Mutable.Load", RelationKind: types.DiagramRelCall},
		},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{
			{Participant: "BusContext", Status: types.DiagramParticipantBoundaryUnproven},
			{Participant: "Mutable", Status: types.DiagramParticipantBoundaryUnproven},
		},
	}}}
	hasScopeViolation := func(violations []types.Violation) bool {
		for _, violation := range violations {
			if violation.Kind == types.ViolDiagramParticipantCoverage &&
				strings.Contains(violation.Detail, "requested_relation_scope_issue") {
				return true
			}
		}
		return false
	}
	if violations := runV2BlockOraclesWithOracleContext(context.Background(), doc, view, mut, nil, nil, bus); !hasScopeViolation(violations) {
		t.Fatalf("post-emit chokepoint accepted disconnected local islands without a whole-relation scope disclosure: %+v", violations)
	}
	doc.Blocks[0].RequestedRelationScope = types.DiagramRelationScopePartialUnproven
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	persisted := mut.AnswerDocumentV2()
	if persisted == nil || persisted.Blocks[0].RequestedRelationScope != types.DiagramRelationScopePartialUnproven {
		t.Fatalf("whole-relation scope was lost across persistence: %+v", persisted)
	}
	if violations := runV2BlockOraclesWithOracleContext(context.Background(), persisted, view, mut, nil, nil, bus); hasScopeViolation(violations) {
		t.Fatalf("exact model-authored whole-relation scope should satisfy the post-emit contract: %+v", violations)
	}
	if len(persisted.Blocks[0].EdgeAnchors) != 4 {
		t.Fatalf("whole-relation scope validation mutated model-authored edges: %+v", persisted.Blocks[0].EdgeAnchors)
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
	if receipt, ok := tool.AcceptedDiagramParticipantCoverageReceiptWithRuntimeContext(bus, doc, view, nil); ok {
		t.Fatalf("source-flow participant receipt leaked into Trace causal authority: %+v", receipt)
	}
}
