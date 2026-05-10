package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// === P2 (2026-05-10) — live coverage status injection on retry ===

// TestLiveCoverageStatus_NoRetryStateReturnsEmpty — no PrevEmitJSON,
// no surface to compute against; helper short-circuits.
func TestLiveCoverageStatus_NoRetryStateReturnsEmpty(t *testing.T) {
	got := renderLiveCoverageStatus(nil, nil)
	if got != "" {
		t.Errorf("nil rs/ctx: want empty, got %q", got)
	}

	rs := &types.RetryState{Attempt: 1}
	got = renderLiveCoverageStatus(nil, rs)
	if got != "" {
		t.Errorf("empty PrevEmitJSON: want empty, got %q", got)
	}
}

// TestLiveCoverageStatus_NoViewReturnsEmpty — no AnswerSemanticView
// (single-shot test path), the helper degrades silently.
func TestLiveCoverageStatus_NoViewReturnsEmpty(t *testing.T) {
	rs := &types.RetryState{
		Attempt:      1,
		PrevEmitJSON: json.RawMessage(`{"blocks":[{"id":"s1","kind":"summary","text":"x"}]}`),
	}
	// nil ctx → no view → empty.
	got := renderLiveCoverageStatus(nil, rs)
	if got != "" {
		t.Errorf("nil ctx (no view): want empty, got %q", got)
	}
}

// TestLiveBlockKindBalance_GapMarked — block kind under-emitted gets ⚠;
// kind that meets MinCount gets ✓. R6: no internal vocab.
func TestLiveBlockKindBalance_GapMarked(t *testing.T) {
	prev := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
		},
	}
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true, MinCount: 1, MaxCount: 1},
			{Kind: types.BlockOrderedList, Required: true, MinCount: 1},
		},
	}
	got := liveBlockKindBalance(prev, view)
	if got == "" {
		t.Fatal("want non-empty rendering with mixed ✓/⚠")
	}
	// summary present → ✓, ordered_list missing → ⚠
	if !strings.Contains(got, "✓ `summary` — emitted 1") {
		t.Errorf("summary should render ✓ at count 1; got %q", got)
	}
	if !strings.Contains(got, "⚠ `ordered_list` — emitted 0") {
		t.Errorf("ordered_list should render ⚠ at count 0; got %q", got)
	}
	// R6: no internal vocab.
	for _, banned := range []string{
		"BlockRequirement", "MinCount", "MaxCount", "ViolKind",
		"AnchoredCount", "DeclaredCount", "FacetCoverage",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("must not leak internal token %q; got %q", banned, got)
		}
	}
}

// TestLiveBlockKindBalance_OverEmitted — over-MaxCount gets ⚠.
func TestLiveBlockKindBalance_OverEmitted(t *testing.T) {
	prev := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
			{ID: "s2", Kind: types.BlockSummary, Text: "y"},
		},
	}
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true, MinCount: 1, MaxCount: 1},
		},
	}
	got := liveBlockKindBalance(prev, view)
	if !strings.Contains(got, "⚠ `summary` — emitted 2 (over max 1)") {
		t.Errorf("over-MaxCount should render ⚠ with 'over max' detail; got %q", got)
	}
}

// TestLiveAspectCoverage_DeclaredButUnanchored — facet declared on
// blocks but no block has citation_ref / claim_form / evidence_id.
// Renders ⚠ "label-only" hint.
func TestLiveAspectCoverage_DeclaredButUnanchored(t *testing.T) {
	prev := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:       "l1",
				Kind:     types.BlockOrderedList,
				FacetIDs: []string{string(types.FacetEnumerationItem)},
				Items: []types.AnswerBlockItem{
					{ID: "i1", CitationRef: -1}, // no cite
				},
			},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{
					Kind:     types.FacetEnumerationItem,
					Required: types.FacetHardRequired,
				},
			},
		},
	}
	got := liveAspectCoverage(prev, view)
	if !strings.Contains(got, "⚠") {
		t.Errorf("declared but unanchored should render ⚠; got %q", got)
	}
	if !strings.Contains(got, "grounded with evidence") {
		t.Errorf("should explain the ground-with-evidence requirement; got %q", got)
	}
}

// TestLiveAspectCoverage_NotDeclared — facet not declared on any block.
func TestLiveAspectCoverage_NotDeclared(t *testing.T) {
	prev := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: types.FacetEnumerationItem, Required: types.FacetHardRequired},
			},
		},
	}
	got := liveAspectCoverage(prev, view)
	if !strings.Contains(got, "⚠") {
		t.Errorf("not declared should render ⚠; got %q", got)
	}
	if !strings.Contains(got, "not declared on any block") {
		t.Errorf("should explicitly state 'not declared'; got %q", got)
	}
}

// TestLiveAspectCoverage_FullyGrounded — declared + cited renders ✓.
func TestLiveAspectCoverage_FullyGrounded(t *testing.T) {
	prev := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:       "l1",
				Kind:     types.BlockOrderedList,
				FacetIDs: []string{string(types.FacetEnumerationItem)},
				Items: []types.AnswerBlockItem{
					{ID: "i1", CitationRef: 0},
				},
			},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: types.FacetEnumerationItem, Required: types.FacetHardRequired},
			},
		},
	}
	got := liveAspectCoverage(prev, view)
	if !strings.Contains(got, "✓") {
		t.Errorf("fully grounded should render ✓; got %q", got)
	}
}

// TestLiveCountFacetDepth_BothChannels — block.facet_ids + claim_uses[].facet_id
// both contribute to declared count; ClaimForm presence anchors.
func TestLiveCountFacetDepth_BothChannels(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:       "l1",
				FacetIDs: []string{"current_code_path"},
				Items:    []types.AnswerBlockItem{{ID: "i1", CitationRef: 5}},
			},
			{
				ID:        "l2",
				ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact, FacetID: "current_code_path"}},
			},
		},
	}
	declared, anchored := liveCountFacetDepth(doc, "current_code_path")
	if declared != 2 {
		t.Errorf("both channels should each contribute to declared; want 2, got %d", declared)
	}
	if anchored != 2 {
		t.Errorf("both channels should each be anchored; want 2, got %d", anchored)
	}
}

// TestLiveCountFacetDepth_LabelOnly — block declares but has no items
// and no claim_uses → unanchored.
func TestLiveCountFacetDepth_LabelOnly(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "l1", FacetIDs: []string{"x"}},
		},
	}
	declared, anchored := liveCountFacetDepth(doc, "x")
	if declared != 1 {
		t.Errorf("label-only declared count; want 1, got %d", declared)
	}
	if anchored != 0 {
		t.Errorf("label-only anchored count; want 0, got %d", anchored)
	}
}

// TestRenderLiveCoverageStatus_FullStackR6 — exercise the full
// renderer; pin no internal vocab leaks.
func TestRenderLiveCoverageStatus_FullStackR6(t *testing.T) {
	docJSON := `{"blocks":[{"id":"s1","kind":"summary","text":"x"}]}`
	rs := &types.RetryState{
		Attempt:      1,
		PrevEmitJSON: json.RawMessage(docJSON),
	}
	// We need an AgentContext with a built view; simulate by passing
	// a context whose AnalysisIR carries an AnswerSurfacePlan.
	// Rather than building a full plan, we exercise the empty-view
	// path (returns "") AND the populated-view path through an
	// integration-style test below.

	// Direct view injection via mock: build a view by hand.
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true, MinCount: 1, MaxCount: 1},
			{Kind: types.BlockOrderedList, Required: true, MinCount: 1},
		},
	}
	// We can't easily plug a hand-built view through
	// renderLiveCoverageStatus without a ctx, so test the inner
	// helpers directly + the empty-view fast path here.
	if got := renderLiveCoverageStatus(nil, rs); got != "" {
		t.Errorf("nil ctx should yield empty string; got %q", got)
	}

	// Validate the inner helpers' R6-cleanliness end-to-end.
	prev := &types.AnswerDocumentV2{}
	_ = json.Unmarshal(rs.PrevEmitJSON, prev)
	combined := liveBlockKindBalance(prev, view)
	for _, banned := range []string{
		"DeclaredCount", "AnchoredCount", "FacetCoverage",
		"BlockRequirement", "ViolKind", "ClusterKey",
	} {
		if strings.Contains(combined, banned) {
			t.Errorf("rendered output must not leak %q; got %q", banned, combined)
		}
	}
}
