package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

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
		if violation.Kind == types.ViolDiagramCallEdgeUnproven && strings.Contains(violation.Detail, "missing_call_anchor") {
			return
		}
	}
	t.Fatalf("post-emit oracle must reject body-edge authority omission: %+v", violations)
}
