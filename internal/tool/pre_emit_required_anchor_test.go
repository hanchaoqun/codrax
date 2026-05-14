package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPreCheckRequiredMechanismAnchors_RequiresStructuredAnchorLabel(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredMechanismAnchors: []types.AnswerRequiredAnchor{
			{Text: "runTaskGraph", Kind: types.ContractTermSymbol},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "The mechanism starts from runTaskGraph but lacks a structured anchor item.",
		},
	}}
	hints := preCheckRequiredMechanismAnchors(doc, view)
	if len(hints) != 1 {
		t.Fatalf("hints len=%d want 1: %+v", len(hints), hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "runTaskGraph") {
		t.Fatalf("hint should name missing typed anchor, got %+v", hints[0])
	}
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:          "anchors",
		Kind:        types.BlockOrderedList,
		SurfaceRole: types.SurfacePrincipal,
		ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact}},
		Items:       []types.AnswerBlockItem{{Label: "runTaskGraph", CitationRef: 0}},
	})
	if hints := preCheckRequiredMechanismAnchors(doc, view); len(hints) != 0 {
		t.Fatalf("structured anchor item should satisfy contract, got %+v", hints)
	}
}

func TestRunPreEmitChecks_RequiredMechanismAnchorsIntegrated(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "Mechanism explanation."},
	}}
	view := &types.AnswerSemanticView{
		RequiredMechanismAnchors: []types.AnswerRequiredAnchor{
			{Text: "runTaskGraph", Kind: types.ContractTermSymbol},
		},
	}
	hints := runPreEmitChecks(doc, view, nil)
	if len(hints) == 0 {
		t.Fatal("expected required mechanism anchor hint")
	}
	found := false
	for _, hint := range hints {
		if strings.Contains(hint.ExpectedShape, "runTaskGraph") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("required mechanism anchor hint missing from runPreEmitChecks: %+v", hints)
	}
}
