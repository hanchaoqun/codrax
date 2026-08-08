package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPreEmitEvidenceWithGroundedDiagramPrecedenceUsesExplicitListCarrier(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  A[StageAnalyze] --> E[StageExplore]\n  E --> X[StageExtract]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "E", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "E", ToNode: "X", RelationKind: types.DiagramRelPrecedence},
		},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	evidence := []types.EvidenceItem{{
		ID: "all-main-stages", Kind: types.EvidenceDirect,
		Subject: "AllMainStages", Source: "pipeline.go", LineStart: 1, LineEnd: 8,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "AllMainStages",
		Scope: types.ScopeLineRange, GroundingStatus: types.GroundingGrounded,
	}}
	gc := &ground.Context{LineIndex: map[string]map[int]string{
		"pipeline.go": {
			1: "func AllMainStages() []PipelineStage {",
			2: "    return []PipelineStage{",
			3: "        StageAnalyze,",
			4: "        StageExplore,",
			5: "        StageExtract,",
			6: "        StageFinalize,",
			7: "    }",
			8: "}",
		},
	}}

	augmented := preEmitEvidenceWithGroundedDiagramPrecedence(doc, view, evidence, gc)
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, augmented); len(got) != 0 {
		t.Fatalf("explicit ordered list should authorize adjacent precedence edges: %+v", got)
	}
	if len(augmented) != len(evidence)+2 {
		t.Fatalf("derived evidence count=%d, want %d", len(augmented), len(evidence)+2)
	}
	for _, item := range augmented[len(evidence):] {
		if item.AnchorKind != types.AnchorPrecedence || item.GroundingStatus != types.GroundingGrounded ||
			item.Producer != "answer.pre_emit.source_precedence" {
			t.Fatalf("unexpected derived precedence row: %+v", item)
		}
	}
}

func TestPreEmitEvidenceWithGroundedDiagramPrecedenceDoesNotUseStatementOrder(t *testing.T) {
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
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	evidence := []types.EvidenceItem{{
		ID: "run", Kind: types.EvidenceDirect,
		Subject: "run", Source: "pipeline.go", LineStart: 1, LineEnd: 5,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "run",
		Scope: types.ScopeLineRange, GroundingStatus: types.GroundingGrounded,
	}}
	gc := &ground.Context{LineIndex: map[string]map[int]string{
		"pipeline.go": {
			1: "func run() {",
			2: "    StageAnalyze()",
			3: "    helper(x, y)",
			4: "    StageExplore()",
			5: "}",
		},
	}}

	augmented := preEmitEvidenceWithGroundedDiagramPrecedence(doc, view, evidence, gc)
	got := DiagramCallEdgeEvidenceMismatches(doc, view, augmented)
	if len(augmented) != len(evidence) || len(got) != 1 || got[0].Relation != types.DiagramRelPrecedence {
		t.Fatalf("arbitrary statement order must remain unproven: evidence=%+v mismatches=%+v", augmented, got)
	}
}

func TestPreEmitEvidenceWithGroundedDiagramPrecedenceExcludesRuntimeTrace(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "trace", Kind: types.BlockDiagram,
		Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Body: "flowchart TD\n A --> B"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelPrecedence}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFRootCauseTrace, RelationAxis: types.AxisFlow}
	evidence := []types.EvidenceItem{{ID: "sentinel"}}
	got := preEmitEvidenceWithGroundedDiagramPrecedence(doc, view, evidence, &ground.Context{})
	if len(got) != 1 || got[0].ID != "sentinel" {
		t.Fatalf("runtime trace authority must remain untouched: %+v", got)
	}
}
