package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRunV2BlockOracles_GroundedPrecedenceParityAcrossPreEmitAndPostContract(t *testing.T) {
	mut := types.NewMutableState("ordered pipeline")
	mut.RecordPreReadSource("pipeline.go", []string{
		"func AllMainStages() []PipelineStage {",
		"    return []PipelineStage{",
		"        StageAnalyze,",
		"        StageExplore,",
		"        StageExtract,",
		"        StageFinalize,",
		"    }",
		"}",
	})
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "all-main-stages",
		Kind:            types.EvidenceDirect,
		Subject:         "AllMainStages",
		Source:          "pipeline.go",
		LineStart:       1,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "AllMainStages",
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}})
	bus := &types.BusContext{Mutable: mut}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  A[StageAnalyze] --> E[StageExplore]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "E", RelationKind: types.DiagramRelPrecedence,
		}},
	}}}

	violations := runV2BlockOraclesWithOracleContext(context.Background(), doc, view, mut, nil, nil, bus)
	for _, violation := range violations {
		if violation.Kind == types.ViolDiagramCallEdgeUnproven && violation.SuspectedRoot.IRField == "diagram_call_edge_evidence" {
			t.Fatalf("post-contract must re-derive the exact grounded precedence accepted at pre-emit: %+v", violation)
		}
	}

	// The authority is bound to the exact declared direction. A different
	// document cannot inherit the validation result from the accepted draft.
	doc.Blocks[0].Diagram.Body = "flowchart TD\n  E[StageExplore] --> A[StageAnalyze]\n"
	doc.Blocks[0].EdgeAnchors[0].FromNode = "E"
	doc.Blocks[0].EdgeAnchors[0].ToNode = "A"
	violations = runV2BlockOraclesWithOracleContext(context.Background(), doc, view, mut, nil, nil, bus)
	found := false
	for _, violation := range violations {
		if violation.Kind == types.ViolDiagramCallEdgeUnproven && violation.SuspectedRoot.IRField == "diagram_call_edge_evidence" {
			found = true
		}
	}
	if !found {
		t.Fatalf("reversed draft must not reuse forward precedence authority: %+v", violations)
	}
}

func TestRunV2BlockOracles_DiagramCallEdgeDirectionRequiresTypedEvidence(t *testing.T) {
	mut := types.NewMutableState("diagram direction")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "ev-run-runwith",
		Kind:            types.EvidenceRelationship,
		Subject:         "gate.Run",
		Predicate:       "calls",
		Object:          "gate.RunWith",
		Source:          "internal/analysis/gate/gate.go",
		LineStart:       135,
		AnchorKind:      types.AnchorCall,
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}})
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "sequence",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind:     types.DiagramSequence,
			Language: "mermaid",
			Body: "sequenceDiagram\n" +
				"  participant R as gate.Run\n" +
				"  participant RW as gate.RunWith\n" +
				"  RW->>R: delegate\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode:     "RW",
			ToNode:       "R",
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimCallEdge,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	violations := runV2BlockOraclesWithMut(doc, view, mut)
	found := false
	for _, violation := range violations {
		if violation.Kind == types.ViolDiagramCallEdgeUnproven && violation.SuspectedRoot.IRField == "diagram_call_edge_evidence" {
			found = true
		}
	}
	if !found {
		t.Fatalf("post-emit oracle dispatch must reject reverse call edge: %+v", violations)
	}

	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant R as gate.Run\n" +
		"  participant RW as gate.RunWith\n" +
		"  R->>RW: delegate\n"
	doc.Blocks[0].EdgeAnchors[0].FromNode = "R"
	doc.Blocks[0].EdgeAnchors[0].ToNode = "RW"
	for _, violation := range runV2BlockOraclesWithMut(doc, view, mut) {
		if violation.Kind == types.ViolDiagramCallEdgeUnproven && violation.SuspectedRoot.IRField == "diagram_call_edge_evidence" {
			t.Fatalf("evidence-backed direction must pass post-emit oracle: %+v", violation)
		}
	}
}

func TestRunV2BlockOracles_DiagramBodyEdgeCannotOmitTypedAnchor(t *testing.T) {
	mut := types.NewMutableState("diagram anchor omission")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "ev-run-runwith",
		Kind:            types.EvidenceRelationship,
		Subject:         "gate.Run",
		Predicate:       "calls",
		Object:          "gate.RunWith",
		Source:          "internal/analysis/gate/gate.go",
		LineStart:       135,
		AnchorKind:      types.AnchorCall,
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}})
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "sequence",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence,
			Body: "sequenceDiagram\n  participant R as gate.Run\n  participant RW as gate.RunWith\n  R->>RW: 1\n",
		},
	}}}
	violations := runV2BlockOraclesWithMut(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, mut)
	for _, violation := range violations {
		if violation.Kind == types.ViolDiagramCallEdgeUnproven && strings.Contains(violation.Detail, "missing_grounded_call_anchor") {
			return
		}
	}
	t.Fatalf("post-emit oracle must reject body-edge authority omission: %+v", violations)
}

func TestRunV2BlockOracles_GenericExplicitCallRequiresTypedEvidence(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "generic-sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence,
			Body: "sequenceDiagram\n  participant P as Parser.Parse\n  participant D as Decoder.Decode\n  P->>D: decode\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "P", ToNode: "D", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
		}},
	}}}
	mut := types.NewMutableState("generic explicit call")
	violations := runV2BlockOraclesWithMut(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, mut)
	for _, violation := range violations {
		if violation.Kind == types.ViolDiagramCallEdgeUnproven &&
			violation.SuspectedRoot.IRField == "diagram_call_edge_evidence" {
			return
		}
	}
	t.Fatalf("post-emit oracle must not let a generic family bypass explicit typed call authority: %+v", violations)
}
