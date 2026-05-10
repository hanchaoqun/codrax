package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// === P1 (2026-05-10) — emit-time pre-validation chokepoint ===

// TestRunPreEmitChecks_NilSafe — nil doc / nil view return nil
// (no spurious rejections; the post-emit chain takes over).
func TestRunPreEmitChecks_NilSafe(t *testing.T) {
	if got := runPreEmitChecks(nil, nil, nil); got != nil {
		t.Errorf("nil/nil: want nil, got %v", got)
	}
	if got := runPreEmitChecks(&types.AnswerDocumentV2{}, nil, nil); got != nil {
		t.Errorf("nil view: want nil, got %v", got)
	}
	if got := runPreEmitChecks(nil, &types.AnswerSemanticView{}, nil); got != nil {
		t.Errorf("nil doc: want nil, got %v", got)
	}
}

// TestPreCheckRequiredBlocks_MinCount — required block kind under-emitted.
func TestPreCheckRequiredBlocks_MinCount(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockOrderedList, Required: true, MinCount: 1, Rationale: "enumeration"},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
		},
	}
	hints := preCheckRequiredBlocks(doc, view)
	if len(hints) != 1 {
		t.Fatalf("want 1 fix hint, got %d", len(hints))
	}
	if !strings.Contains(hints[0].Field, "ordered_list") {
		t.Errorf("hint field should name the block kind; got %q", hints[0].Field)
	}
	if !strings.Contains(hints[0].ExpectedShape, "at least 1") {
		t.Errorf("hint should state the required minimum; got %q", hints[0].ExpectedShape)
	}
}

// TestPreCheckRequiredBlocks_MaxCount — required block kind over-emitted.
func TestPreCheckRequiredBlocks_MaxCount(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true, MinCount: 1, MaxCount: 1, Rationale: "lead"},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
			{ID: "s2", Kind: types.BlockSummary, Text: "y"},
		},
	}
	hints := preCheckRequiredBlocks(doc, view)
	if len(hints) != 1 {
		t.Fatalf("want 1 fix hint, got %d", len(hints))
	}
	if !strings.Contains(hints[0].ExpectedShape, "at most 1") {
		t.Errorf("hint should state the maximum cap; got %q", hints[0].ExpectedShape)
	}
}

// TestPreCheckRequiredBlocks_HappyPath — exact match returns no hints.
func TestPreCheckRequiredBlocks_HappyPath(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true, MinCount: 1, MaxCount: 1},
			{Kind: types.BlockOrderedList, Required: true, MinCount: 1},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
			{ID: "l1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{ID: "i1"}}},
		},
	}
	if hints := preCheckRequiredBlocks(doc, view); len(hints) != 0 {
		t.Errorf("happy path should produce no hints; got %v", hints)
	}
}

// TestPreCheckPrincipalClaimUse_Missing — principal block missing claim_uses.
func TestPreCheckPrincipalClaimUse_Missing(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{
				Kind:                 types.BlockOrderedList,
				Required:             true,
				MinCount:             1,
				AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact, types.ClaimCallEdge},
				SurfaceRoleHint:      types.SurfacePrincipal,
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:          "l1",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				Items:       []types.AnswerBlockItem{{ID: "i1"}},
				// no ClaimUses → fix hint expected
			},
		},
	}
	hints := preCheckPrincipalClaimUse(doc, view)
	if len(hints) != 1 {
		t.Fatalf("want 1 fix hint, got %d", len(hints))
	}
	if !strings.Contains(hints[0].Field, "claim_uses") {
		t.Errorf("hint field should name claim_uses; got %q", hints[0].Field)
	}
	if !strings.Contains(hints[0].ExpectedShape, "definition_fact") {
		t.Errorf("hint should list the acceptable forms; got %q", hints[0].ExpectedShape)
	}
}

// TestPreCheckPrincipalClaimUse_SingleFormRelaxation — when contract
// declares exactly one AcceptableClaimForm AND the block carries
// structural grounding (facet_ids + cited items), the missing
// claim_uses is treated as implicit-default and NOT flagged.
func TestPreCheckPrincipalClaimUse_SingleFormRelaxation(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{
				Kind:                 types.BlockOrderedList,
				Required:             true,
				MinCount:             1,
				AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact},
				SurfaceRoleHint:      types.SurfacePrincipal,
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:          "l1",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				FacetIDs:    []string{"enumeration_item"},
				Items:       []types.AnswerBlockItem{{ID: "i1", CitationRef: 0}},
			},
		},
	}
	if hints := preCheckPrincipalClaimUse(doc, view); len(hints) != 0 {
		t.Errorf("single-form relaxation should suppress the hint; got %v", hints)
	}
}

// TestPreCheckUncertaintyBlock_MissingWhenRequired
func TestPreCheckUncertaintyBlock_MissingWhenRequired(t *testing.T) {
	view := &types.AnswerSemanticView{
		UncertaintyRules: []types.UncertaintyRule{{TriggerFacet: ""}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
		},
	}
	hints := preCheckUncertaintyBlock(doc, view)
	if len(hints) != 1 {
		t.Fatalf("want 1 fix hint, got %d", len(hints))
	}
	if !strings.Contains(hints[0].Field, "caveat") {
		t.Errorf("hint should name caveat block; got %q", hints[0].Field)
	}
}

// TestPreCheckUncertaintyBlock_PresentSuppresses
func TestPreCheckUncertaintyBlock_PresentSuppresses(t *testing.T) {
	view := &types.AnswerSemanticView{
		UncertaintyRules: []types.UncertaintyRule{{TriggerFacet: ""}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
			{ID: "c1", Kind: types.BlockCaveat, Text: "what we did not search"},
		},
	}
	if hints := preCheckUncertaintyBlock(doc, view); len(hints) != 0 {
		t.Errorf("caveat present should suppress; got %v", hints)
	}
}

// TestPreCheckUncertaintyBlock_NoRules
func TestPreCheckUncertaintyBlock_NoRules(t *testing.T) {
	view := &types.AnswerSemanticView{}
	doc := &types.AnswerDocumentV2{}
	if hints := preCheckUncertaintyBlock(doc, view); len(hints) != 0 {
		t.Errorf("no rules → no hints; got %v", hints)
	}
}

// TestFormatEmitFixHints_RedlineAudit — pin the rejection envelope
// prose for R6 (no internal vocab leak) + R4 (generic) + LLM-facing
// purity (no third natural language).
func TestFormatEmitFixHints_RedlineAudit(t *testing.T) {
	hints := []emitFixHint{
		{Field: "blocks[].kind=ordered_list", ExpectedShape: "emit at least 1 block of kind=ordered_list", Reason: "enumeration"},
		{Field: "blocks[id=\"l1\"].claim_uses", ExpectedShape: "add a one-element claim_uses[]", Reason: "principal claim required"},
	}
	got := formatEmitFixHints(hints)

	// R6: must not leak internal vocab.
	for _, banned := range []string{
		"orchestrator", "ViolKind", "ViolationKind", "ClusterKey",
		"SuspectedRoot", "BuildRepairPlan", "RepairCluster",
		"AnswerDocumentV2", "FacetCoverage", "AnchoredCount",
		"DeclaredCount", "TaskGraph", "BusContext",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("rejection prose must not leak internal token %q; got %q", banned, got)
		}
	}

	// No third natural language.
	for _, tok := range []string{"diese", "todos", "tutti", "cette", "это", "هذا"} {
		if strings.Contains(got, tok) {
			t.Errorf("rejection prose must be EN only; banned %q in %q", tok, got)
		}
	}

	// Each hint renders verbatim Field + ExpectedShape.
	for _, h := range hints {
		if !strings.Contains(got, h.Field) {
			t.Errorf("hint Field %q missing from output; got %q", h.Field, got)
		}
		if !strings.Contains(got, h.ExpectedShape) {
			t.Errorf("hint ExpectedShape %q missing from output; got %q", h.ExpectedShape, got)
		}
	}

	// Empty list → empty string.
	if got := formatEmitFixHints(nil); got != "" {
		t.Errorf("empty hints → empty string; got %q", got)
	}
}

// TestRunPreEmitChecks_AggregatesAcrossAllAxes — happy + fail mix.
// Pin that the four checks all wire through the top-level dispatcher.
func TestRunPreEmitChecks_AggregatesAcrossAllAxes(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockOrderedList, Required: true, MinCount: 1, AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact, types.ClaimCallEdge}, SurfaceRoleHint: types.SurfacePrincipal},
		},
		UncertaintyRules: []types.UncertaintyRule{{TriggerFacet: ""}},
	}
	// Doc that fails 3 of 4 checks: missing ordered_list, missing
	// caveat, missing facet declarations (when we'd add facet
	// coverage), missing principal claim_use is moot because
	// ordered_list itself is missing.
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
		},
	}
	hints := runPreEmitChecks(doc, view, nil)
	if len(hints) < 2 {
		t.Errorf("want ≥2 fix hints (block coverage + uncertainty); got %d (%v)", len(hints), hints)
	}
}
