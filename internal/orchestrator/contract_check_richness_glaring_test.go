package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// contract_check_richness_glaring_test.go — B2 v3 (2026-05-04).
// Tests for the three-layer quality contract validators:
//
//   - validateRichnessGlaringGap: optional facet marked Glaring +
//     evidence above family threshold + uncovered → Medium violation
//   - validatePrincipalProseUnderfilled: principal block with ≥3
//     typed claim_uses but zero inline-code anchors in prose
//   - GlaringGap suppresses RichnessRegression on the SAME facet
//   - countFacetCoverageDepth distinguishes label-only (declared
//     without anchored evidence) from anchored declarations
//   - reviewer input populates SystemDetectedGaps + PromotedFacetCoverage

// helper — build a minimal view with one Glaring optional facet.
func glaringView(family types.QuestionFamily, optionalKind types.AnswerFacetKind, evidenceCount int) *types.AnswerSemanticView {
	candidates := make([]string, evidenceCount)
	for i := range candidates {
		candidates[i] = "evidence_id_" + string(rune('a'+i))
	}
	return &types.AnswerSemanticView{
		Family: family,
		FacetCoverage: &types.FacetCoverageContract{
			Family: family,
			Optional: []types.FacetRequirement{
				{
					Kind:            optionalKind,
					Required:        types.FacetOptional,
					Tier:            types.TierEnrichment,
					PromotionPolicy: types.PromotionAdvisoryOnly,
					Strength:        types.EnrichmentGlaring,
					SourceCandidate: candidates,
				},
			},
		},
	}
}

// docWithFacetID builds a minimal V2 doc with ONE block declaring the
// given facet kind via block.FacetIDs[0].
func docWithFacetID(facetKind types.AnswerFacetKind) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:       "b1",
				Kind:     types.BlockSummary,
				Text:     "covered",
				FacetIDs: []string{string(facetKind)},
			},
		},
	}
}

func docEmpty() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockSummary, Text: "no facet"},
		},
	}
}

// TestValidateRichnessGlaringGap_FiresOnGlaringWithEnoughEvidence — the
// canonical positive: Architecture family, evidence=3 (== threshold),
// uncovered → Medium ViolRichnessGlaringGap.
func TestValidateRichnessGlaringGap_FiresOnGlaringWithEnoughEvidence(t *testing.T) {
	view := glaringView(types.QFArchitecture, types.FacetComponentRelation, 3)
	doc := docEmpty()
	vs := validateRichnessGlaringGap(doc, view)
	if len(vs) != 1 {
		t.Fatalf("expected 1 violation; got %d", len(vs))
	}
	if vs[0].Kind != types.ViolRichnessGlaringGap {
		t.Errorf("kind = %q, want ViolRichnessGlaringGap", vs[0].Kind)
	}
	if !strings.Contains(vs[0].Detail, "component_relation") {
		t.Errorf("detail must name facet kind; got %q", vs[0].Detail)
	}
}

// TestValidateRichnessGlaringGap_BelowThresholdSuppressed — evidence=2
// for QFArchitecture (threshold=3) → no violation.
func TestValidateRichnessGlaringGap_BelowThresholdSuppressed(t *testing.T) {
	view := glaringView(types.QFArchitecture, types.FacetComponentRelation, 2)
	doc := docEmpty()
	if vs := validateRichnessGlaringGap(doc, view); len(vs) != 0 {
		t.Errorf("evidence=2 below QFArchitecture threshold=3 must not fire; got %+v", vs)
	}
}

// TestValidateRichnessGlaringGap_NotMarkedGlaringSkipped — Strength=None
// (default) → no violation regardless of evidence.
func TestValidateRichnessGlaringGap_NotMarkedGlaringSkipped(t *testing.T) {
	view := glaringView(types.QFArchitecture, types.FacetComponentRelation, 5)
	view.FacetCoverage.Optional[0].Strength = types.EnrichmentNone
	doc := docEmpty()
	if vs := validateRichnessGlaringGap(doc, view); len(vs) != 0 {
		t.Errorf("Strength=None must skip; got %+v", vs)
	}
}

// TestValidateRichnessGlaringGap_CoveredFacetSkipped — block.FacetIDs
// contains the facet → no violation.
func TestValidateRichnessGlaringGap_CoveredFacetSkipped(t *testing.T) {
	view := glaringView(types.QFArchitecture, types.FacetComponentRelation, 5)
	doc := docWithFacetID(types.FacetComponentRelation)
	if vs := validateRichnessGlaringGap(doc, view); len(vs) != 0 {
		t.Errorf("covered facet must skip; got %+v", vs)
	}
}

// TestValidateRichnessGlaringGap_FamilyThresholdsRespected — QFCallChain
// uses threshold=2; QFArchitecture uses threshold=3.
func TestValidateRichnessGlaringGap_FamilyThresholdsRespected(t *testing.T) {
	// CallChain at evidence=2 fires (threshold=2).
	cc := glaringView(types.QFCallChain, types.FacetBranchGuard, 2)
	if vs := validateRichnessGlaringGap(docEmpty(), cc); len(vs) != 1 {
		t.Errorf("QFCallChain evidence=2 must fire (threshold=2); got %+v", vs)
	}
	// Architecture at evidence=2 does not fire (threshold=3).
	arch := glaringView(types.QFArchitecture, types.FacetComponentRelation, 2)
	if vs := validateRichnessGlaringGap(docEmpty(), arch); len(vs) != 0 {
		t.Errorf("QFArchitecture evidence=2 below threshold=3 must skip; got %+v", vs)
	}
}

// TestValidateRichnessGlaringGap_SuppressesRichnessRegression — when
// glaring fires, the SAME facet's RichnessRegression entry is dropped
// (no double-billing).
func TestValidateRichnessGlaringGap_SuppressesRichnessRegression(t *testing.T) {
	view := glaringView(types.QFArchitecture, types.FacetComponentRelation, 3)
	doc := docEmpty()
	regressions := validateRichnessRegression(doc, view)
	for _, v := range regressions {
		if v.Kind == types.ViolRichnessRegression && strings.Contains(v.Detail, "component_relation") {
			t.Errorf("RichnessRegression must be suppressed when glaring fires; got %+v", v)
		}
	}
}

// TestValidatePrincipalProseUnderfilled_SummaryChecksBlockText — Summary
// block with 3 ClaimUses but no inline-code in Text → fires.
func TestValidatePrincipalProseUnderfilled_SummaryChecksBlockText(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:          "summary",
				Kind:        types.BlockSummary,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "this is plain prose without inline code references at all",
				ClaimUses: []types.RenderedClaimUse{
					{ClaimForm: types.ClaimDefinitionFact, EvidenceID: "e1"},
					{ClaimForm: types.ClaimCallEdge, EvidenceID: "e2"},
					{ClaimForm: types.ClaimGuardCondition, EvidenceID: "e3"},
				},
			},
		},
	}
	vs := validatePrincipalProseUnderfilled(doc)
	if len(vs) != 1 {
		t.Fatalf("expected 1 violation; got %d: %+v", len(vs), vs)
	}
	if vs[0].Kind != types.ViolPrincipalProseUnderfilled {
		t.Errorf("kind = %q, want ViolPrincipalProseUnderfilled", vs[0].Kind)
	}
}

// TestValidatePrincipalProseUnderfilled_SummaryWithInlineCodePasses —
// Summary with ≥1 inline-code segment → no violation.
func TestValidatePrincipalProseUnderfilled_SummaryWithInlineCodePasses(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:          "summary",
				Kind:        types.BlockSummary,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "the function `foo` returns when guard `x > 0` holds",
				ClaimUses: []types.RenderedClaimUse{
					{ClaimForm: types.ClaimDefinitionFact, EvidenceID: "e1"},
					{ClaimForm: types.ClaimCallEdge, EvidenceID: "e2"},
					{ClaimForm: types.ClaimGuardCondition, EvidenceID: "e3"},
				},
			},
		},
	}
	if vs := validatePrincipalProseUnderfilled(doc); len(vs) != 0 {
		t.Errorf("inline-code present → no violation; got %+v", vs)
	}
}

// TestValidatePrincipalProseUnderfilled_OrderedListChecksItemsAggregated
// — ordered_list aggregates items[].Text. Block.Text is empty (lead-in)
// but items have inline-code → no violation.
func TestValidatePrincipalProseUnderfilled_OrderedListChecksItemsAggregated(t *testing.T) {
	cu := types.RenderedClaimUse{ClaimForm: types.ClaimCallEdge, EvidenceID: "e1"}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:          "ol",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "", // empty lead-in (typical pattern)
				Items: []types.AnswerBlockItem{
					{ID: "i1", Text: "calls `foo`", CitationRef: 0},
					{ID: "i2", Text: "calls `bar`", CitationRef: 1},
					{ID: "i3", Text: "calls `baz`", CitationRef: 2},
				},
			},
		},
	}
	_ = cu
	if vs := validatePrincipalProseUnderfilled(doc); len(vs) != 0 {
		t.Errorf("ordered_list with inline-code in items[] → no violation; got %+v", vs)
	}
}

// TestValidatePrincipalProseUnderfilled_OrderedListWithoutInlineCodeFires
// — ordered_list with 3 items with claim_use but ZERO inline-code in
// any item.Text → fires.
func TestValidatePrincipalProseUnderfilled_OrderedListWithoutInlineCodeFires(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:          "ol",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Text: "first the system invokes the worker", CitationRef: 0},
					{ID: "i2", Text: "then the result is checked", CitationRef: 1},
					{ID: "i3", Text: "finally the response is returned", CitationRef: 2},
				},
			},
		},
	}
	vs := validatePrincipalProseUnderfilled(doc)
	if len(vs) != 1 {
		t.Fatalf("expected 1 violation; got %d: %+v", len(vs), vs)
	}
	if vs[0].Kind != types.ViolPrincipalProseUnderfilled {
		t.Errorf("kind = %q, want ViolPrincipalProseUnderfilled", vs[0].Kind)
	}
}

// TestValidatePrincipalProseUnderfilled_LabelAnchoredOrderedListSkips —
// when every item.label is identifier-shaped (StageLogTriage / etc.),
// the rendered row header is the inline anchor users see — skip the
// prose-density check even when items[].text has no backtick code.
// qf_arch run-1 forensic showed the validator over-firing on this
// shape across all 3 finalizer iterations.
func TestValidatePrincipalProseUnderfilled_LabelAnchoredOrderedListSkips(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:          "key_anchors",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "StageLogTriage", Text: "条件性预阶段处理日志输出", CitationRef: 0},
					{ID: "i2", Label: "StageAnalyze", Text: "意图分类与证据收集", CitationRef: 1},
					{ID: "i3", Label: "StageExplore", Text: "深入分析与符号溯源", CitationRef: 2},
				},
			},
		},
	}
	if vs := validatePrincipalProseUnderfilled(doc); len(vs) != 0 {
		t.Errorf("label-anchored ordered_list (every item.label identifier-shape) must skip prose-density check; got %+v", vs)
	}
}

func TestValidatePrincipalProseUnderfilled_SourceLocationLabelsSkip(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:          "locations",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				ClaimUses: []types.RenderedClaimUse{{
					ClaimForm: types.ClaimAssignmentFact,
				}},
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "internal/agent/analyzer.go:1903", Text: "sets the field", CitationRef: 0},
					{ID: "i2", Label: "internal/orchestrator/contract_check.go:63", Text: "sets the field", CitationRef: 1},
					{ID: "i3", Label: "internal/orchestrator/orchestrator.go:6362", Text: "sets the field", CitationRef: 2},
					{ID: "i4", Label: "internal/orchestrator/orchestrator.go:6494", Text: "sets the field", CitationRef: 3},
				},
			},
		},
	}
	if vs := validatePrincipalProseUnderfilled(doc); len(vs) != 0 {
		t.Fatalf("source-location labels are user-visible evidence anchors and should not force helper-name inline code; got %+v", vs)
	}
}

// TestValidatePrincipalProseUnderfilled_BelowClaimFloorSkipped — only 2
// claim_uses → below floor=3 → no violation regardless of prose.
func TestValidatePrincipalProseUnderfilled_BelowClaimFloorSkipped(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:          "summary",
				Kind:        types.BlockSummary,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "no inline code",
				ClaimUses: []types.RenderedClaimUse{
					{ClaimForm: types.ClaimDefinitionFact, EvidenceID: "e1"},
					{ClaimForm: types.ClaimCallEdge, EvidenceID: "e2"},
				},
			},
		},
	}
	if vs := validatePrincipalProseUnderfilled(doc); len(vs) != 0 {
		t.Errorf("below claim floor=3 → no violation; got %+v", vs)
	}
}

// TestValidatePrincipalProseUnderfilled_ScalarSkipped — Scalar block
// is skipped (single claim_use convention).
func TestValidatePrincipalProseUnderfilled_ScalarSkipped(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:          "scalar",
				Kind:        types.BlockScalar,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "42",
				ClaimUses: []types.RenderedClaimUse{
					{ClaimForm: types.ClaimDefinitionFact, EvidenceID: "e1"},
					{ClaimForm: types.ClaimDefinitionFact, EvidenceID: "e2"},
					{ClaimForm: types.ClaimDefinitionFact, EvidenceID: "e3"},
				},
			},
		},
	}
	if vs := validatePrincipalProseUnderfilled(doc); len(vs) != 0 {
		t.Errorf("Scalar must skip; got %+v", vs)
	}
}

// TestCountInlineCodeSegments_SimpleCases — verbatim-substring counter
// behaviour for pairing backticks.
func TestCountInlineCodeSegments_SimpleCases(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"", 0},
		{"plain prose", 0},
		{"single `code` here", 1},
		{"two `a` and `b` segments", 2},
		{"`open without close", 0},
		{"backtick at end `", 0},
	}
	for _, tc := range cases {
		if got := countInlineCodeSegments(tc.s); got != tc.want {
			t.Errorf("countInlineCodeSegments(%q) = %d, want %d", tc.s, got, tc.want)
		}
	}
}

// TestCountFacetCoverageDepth_DeclaredVsAnchored — block.FacetIDs
// without claim_use → declared but not anchored; with EvidenceID →
// anchored.
func TestCountFacetCoverageDepth_DeclaredVsAnchored(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				// Declared via FacetIDs only — no anchor on this block.
				ID:       "shallow",
				Kind:     types.BlockSummary,
				FacetIDs: []string{"facet_x"},
			},
			{
				// Declared via FacetIDs AND anchored via ClaimUses[0].
				ID:       "deep",
				Kind:     types.BlockSummary,
				FacetIDs: []string{"facet_x"},
				ClaimUses: []types.RenderedClaimUse{
					{FacetID: "facet_x", EvidenceID: "e1", ClaimForm: types.ClaimDefinitionFact},
				},
			},
		},
	}
	declared, anchored := countFacetCoverageDepth(doc, "facet_x")
	// 2 FacetIDs declarations + 1 ClaimUses declaration = 3 declared
	// 1 of the FacetIDs declarations is anchored (block "deep")
	// + the ClaimUses declaration itself is anchored (1) = 2 anchored.
	if declared != 3 {
		t.Errorf("declared count = %d, want 3", declared)
	}
	if anchored != 2 {
		t.Errorf("anchored count = %d, want 2", anchored)
	}
}

// TestFamilyGlaringEvidenceThreshold_ExactValues pins the family-level
// thresholds documented in the design.
func TestFamilyGlaringEvidenceThreshold_ExactValues(t *testing.T) {
	cases := []struct {
		family types.QuestionFamily
		want   int
	}{
		{types.QFArchitecture, 3},
		{types.QFRootCauseTrace, 3},
		{types.QFCallChain, 2},
		{types.QFConfigPrecedence, 2},
		{types.QFEnumeration, 4},
		{types.QFRoleLookup, 4},
		{types.QFComparison, 4},
		{types.QFGeneric, 4},
	}
	for _, tc := range cases {
		// familyGlaringEvidenceThreshold is in the types package; we
		// verify via FacetRequirement.EffectiveMinEvidenceForGlaring
		// which calls it when MinEvidenceForGlaring is zero.
		req := types.FacetRequirement{}
		got := req.EffectiveMinEvidenceForGlaring(tc.family)
		if got != tc.want {
			t.Errorf("threshold for family=%q: got %d, want %d", tc.family, got, tc.want)
		}
	}
}
