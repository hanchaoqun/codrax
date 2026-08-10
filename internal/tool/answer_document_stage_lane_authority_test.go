package tool

import (
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDiagramVerifiedReadModeStagePrecedenceWiresPostValidation(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: repoRoot, Mode: types.ModeRead}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	if got := diagramVerifiedReadModeStagePrecedence(ctx, view); len(got) != 3 {
		t.Fatalf("expected three checkout-verified adjacent relations, got %+v", got)
	}
	doc := func(toLabel string) *types.AnswerDocumentV2 {
		return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "read-lane", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid",
				Body: "flowchart TD\n  A[Analyzer] --> E[" + toLabel + "]\n"},
			EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "E", RelationKind: types.DiagramRelPrecedence}},
		}}}
	}
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, doc("Explorer"), view, nil); len(got) != 0 {
		t.Fatalf("shared provider relation taught to the model must pass post validation: %+v", got)
	}
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, doc("Extractor"), view, nil); len(got) == 0 {
		t.Fatal("non-adjacent stage edge must remain unproved")
	}

	ctx.Mode = types.ModeApply
	if got := diagramVerifiedReadModeStagePrecedence(ctx, view); len(got) != 0 {
		t.Fatalf("write mode must not borrow read-lane authority: %+v", got)
	}
	ctx.Mode = types.ModeRead
	traceView := &types.AnswerSemanticView{Family: types.QFRootCauseTrace, RelationAxis: types.AxisFlow}
	if got := diagramVerifiedReadModeStagePrecedence(ctx, traceView); len(got) != 0 {
		t.Fatalf("Trace must retain its independent relation authority: %+v", got)
	}
}
