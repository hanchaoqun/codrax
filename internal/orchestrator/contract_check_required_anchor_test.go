package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func requiredAnchorView(anchors ...string) *types.AnswerSemanticView {
	v := &types.AnswerSemanticView{}
	for _, a := range anchors {
		v.RequiredMechanismAnchors = append(v.RequiredMechanismAnchors,
			types.AnswerRequiredAnchor{Text: a, Kind: types.ContractTermSymbol})
	}
	return v
}

// The rendered re-check fires when a question-named anchor appears on
// NO structured surface of the final document — the post-mutation
// backstop the pre-emit chokepoint cannot provide.
func TestRequiredMechanismAnchorsRenderedMissing(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "s1", Kind: types.BlockSummary, Text: "prose mentioning MutableState only in text"},
	}}
	v := validateRequiredMechanismAnchorsRendered(doc, requiredAnchorView("MutableState"))
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %v", v)
	}
	if v[0].Kind != types.ViolPrincipalSupportMemberOmitted {
		t.Fatalf("kind = %s", v[0].Kind)
	}
	if !strings.Contains(v[0].Detail, "MutableState") {
		t.Fatalf("detail must name the missing anchor: %q", v[0].Detail)
	}
}

// Table cells are a legitimate carrier: an anchor annotated inside a
// cell suppresses the violation (forensic shape — the model's table
// carried every anchor in cells, not labels).
func TestRequiredMechanismAnchorsCellsSuppress(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "t1", Kind: types.BlockTable, Items: []types.AnswerBlockItem{
			{ID: "r1", Label: "StageAnalyze", Cells: []string{"请求", "AnalysisIR", "MutableState（重试计数）"}},
		}},
	}}
	if v := validateRequiredMechanismAnchorsRendered(doc, requiredAnchorView("MutableState")); v != nil {
		t.Fatalf("anchor inside a table cell must suppress the violation, got %v", v)
	}
}

// Labels, titles, and diagram endpoints keep working as before.
func TestRequiredMechanismAnchorsClassicSurfaces(t *testing.T) {
	cases := []struct {
		name string
		doc  *types.AnswerDocumentV2
	}{
		{"item label", &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
			{ID: "l1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{ID: "i1", Label: "MutableState"}}},
		}}},
		{"block title", &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSection, Title: "MutableState", Text: "x"},
		}}},
		{"edge endpoint", &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
			{ID: "d1", Kind: types.BlockDiagram, EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "MutableState", ToNode: "BusContext"}}},
		}}},
	}
	for _, tc := range cases {
		if v := validateRequiredMechanismAnchorsRendered(tc.doc, requiredAnchorView("MutableState")); v != nil {
			t.Errorf("%s: expected suppression, got %v", tc.name, v)
		}
	}
}

// No anchors compiled (non-mechanism families) → validator is inert.
func TestRequiredMechanismAnchorsRenderedNoOps(t *testing.T) {
	if v := validateRequiredMechanismAnchorsRendered(nil, nil); v != nil {
		t.Fatalf("nil inputs must no-op")
	}
	doc := &types.AnswerDocumentV2{}
	if v := validateRequiredMechanismAnchorsRendered(doc, &types.AnswerSemanticView{}); v != nil {
		t.Fatalf("empty anchor list must no-op")
	}
}
