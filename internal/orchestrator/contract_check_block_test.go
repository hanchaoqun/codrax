package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ── B4-T5 V2 block oracle 单测 (4 validator × 4 case = 16+ + AllViolationKinds 完整性) ──

func minimalSummaryView() *types.AnswerSemanticView {
	return &types.AnswerSemanticView{
		Family: types.QFGeneric,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, MinCount: 1, MaxCount: 1, Required: true, Rationale: "summary required"},
		},
	}
}

func summaryBlock(id string) types.AnswerBlock {
	return types.AnswerBlock{ID: id, Kind: types.BlockSummary, Text: "x"}
}

// ── validateRequiredBlockCoverage ──────────────────────────

func TestRequiredBlockCoverage_MeetsMinPasses(t *testing.T) {
	view := minimalSummaryView()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{summaryBlock("b1")}}
	if vs := validateRequiredBlockCoverage(doc, view); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestRequiredBlockCoverage_MissingBlockFires(t *testing.T) {
	view := minimalSummaryView()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "x1", Kind: types.BlockOrderedList},
	}}
	vs := validateRequiredBlockCoverage(doc, view)
	if len(vs) != 1 || vs[0].Kind != types.ViolBlockCoverageMissing {
		t.Fatalf("expected ViolBlockCoverageMissing; got %+v", vs)
	}
	if !strings.Contains(vs[0].Detail, "summary") {
		t.Errorf("detail should name missing kind; got %q", vs[0].Detail)
	}
}

func TestRequiredBlockCoverage_OverEmittedFires(t *testing.T) {
	view := minimalSummaryView()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		summaryBlock("b1"),
		summaryBlock("b2"),
	}}
	vs := validateRequiredBlockCoverage(doc, view)
	if len(vs) != 1 || vs[0].Kind != types.ViolBlockCoverageMissing {
		t.Fatalf("expected violation on over-emit; got %+v", vs)
	}
}

func TestRequiredBlockCoverage_NilGuards(t *testing.T) {
	if vs := validateRequiredBlockCoverage(nil, minimalSummaryView()); vs != nil {
		t.Errorf("nil doc → nil violations; got %+v", vs)
	}
	if vs := validateRequiredBlockCoverage(&types.AnswerDocumentV2{}, nil); vs != nil {
		t.Errorf("nil view → nil violations; got %+v", vs)
	}
}

// ── validatePrincipalClaimUse ──────────────────────────────

func principalClaimUseView() *types.AnswerSemanticView {
	return &types.AnswerSemanticView{
		Family: types.QFRoleLookup,
		RequiredBlocks: []types.BlockRequirement{
			{
				Kind:                 types.BlockScalar,
				MinCount:             1,
				MaxCount:             1,
				Required:             true,
				AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact},
				SurfaceRoleHint:      types.SurfacePrincipal,
				Rationale:            "scalar required",
			},
		},
	}
}

func TestPrincipalClaimUse_PresentPasses(t *testing.T) {
	view := principalClaimUseView()
	cu := types.RenderedClaimUse{ClaimForm: types.ClaimDefinitionFact, SurfaceRole: types.SurfacePrincipal}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:        "s1",
			Kind:      types.BlockScalar,
			Text:      "42",
			ClaimUses: []types.RenderedClaimUse{cu},
		},
	}}
	if vs := validatePrincipalClaimUse(doc, view); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestPrincipalClaimUse_MissingFires(t *testing.T) {
	view := principalClaimUseView()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "s1", Kind: types.BlockScalar, Text: "42", SurfaceRole: types.SurfacePrincipal},
	}}
	vs := validatePrincipalClaimUse(doc, view)
	if len(vs) != 1 || vs[0].Kind != types.ViolPrincipalClaimUseMissing {
		t.Fatalf("expected ViolPrincipalClaimUseMissing; got %+v", vs)
	}
}

func TestPrincipalClaimUse_NonPrincipalSkipped(t *testing.T) {
	view := principalClaimUseView()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "s1", Kind: types.BlockScalar, Text: "42", SurfaceRole: types.SurfaceSupport},
	}}
	if vs := validatePrincipalClaimUse(doc, view); len(vs) != 0 {
		t.Errorf("non-principal block must skip; got %+v", vs)
	}
}

func TestPrincipalClaimUse_ItemLevelClaimSatisfies(t *testing.T) {
	view := principalClaimUseView()
	cu := types.RenderedClaimUse{ClaimForm: types.ClaimDefinitionFact}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:          "s1",
			Kind:        types.BlockScalar,
			SurfaceRole: types.SurfacePrincipal,
			Items:       []types.AnswerBlockItem{{Label: "v", ClaimUse: &cu}},
		},
	}}
	if vs := validatePrincipalClaimUse(doc, view); len(vs) != 0 {
		t.Errorf("item-level ClaimUse must satisfy; got %+v", vs)
	}
}

// ── validateDiagramEdgeSupport ─────────────────────────────

func diagramRequiredView(kind types.DiagramKind) *types.AnswerSemanticView {
	return &types.AnswerSemanticView{
		Family: types.QFCallChain,
		DiagramPlan: &types.DiagramFacetGraph{
			Required: true,
			Kind:     kind,
		},
	}
}

func TestDiagramEdgeSupport_RequiredAndPresentPasses(t *testing.T) {
	view := diagramRequiredView(types.DiagramSequence)
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "d1",
			Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramSequence,
				Body: "sequenceDiagram",
			},
		},
	}}
	if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestDiagramEdgeSupport_RequiredAbsentFires(t *testing.T) {
	view := diagramRequiredView(types.DiagramSequence)
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{summaryBlock("b1")}}
	vs := validateDiagramEdgeSupport(doc, view)
	if len(vs) != 1 || vs[0].Kind != types.ViolDiagramEdgeUnsupported {
		t.Fatalf("expected ViolDiagramEdgeUnsupported; got %+v", vs)
	}
}

func TestDiagramEdgeSupport_KindMismatchFires(t *testing.T) {
	view := diagramRequiredView(types.DiagramSequence)
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "d1",
			Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramFlow, // 故意不匹配
				Body: "flowchart LR",
			},
		},
	}}
	vs := validateDiagramEdgeSupport(doc, view)
	if len(vs) != 1 {
		t.Fatalf("expected 1 violation on kind mismatch; got %+v", vs)
	}
}

func TestDiagramEdgeSupport_NoDiagramPlanSkipped(t *testing.T) {
	view := &types.AnswerSemanticView{Family: types.QFGeneric}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{summaryBlock("b1")}}
	if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
		t.Errorf("no DiagramPlan should skip; got %+v", vs)
	}
}

// ── validateUncertaintyBlockPresence ───────────────────────

func uncertaintyView() *types.AnswerSemanticView {
	return &types.AnswerSemanticView{
		Family: types.QFRootCauseTrace,
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: types.FacetObservedArtifactFact, Required: types.FacetHardRequired},
			},
		},
		UncertaintyRules: []types.UncertaintyRule{
			{
				TriggerFacet:      string(types.FacetObservedArtifactFact),
				ExpectedBlockKind: types.BlockCaveat,
				MissingMessage:    "emit caveat for log drift",
			},
		},
	}
}

func TestUncertaintyBlockPresence_PresentPasses(t *testing.T) {
	view := uncertaintyView()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		summaryBlock("b1"),
		{ID: "c1", Kind: types.BlockCaveat, Text: "log drift"},
	}}
	if vs := validateUncertaintyBlockPresence(doc, view); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestUncertaintyBlockPresence_MissingFires(t *testing.T) {
	view := uncertaintyView()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{summaryBlock("b1")}}
	vs := validateUncertaintyBlockPresence(doc, view)
	if len(vs) != 1 || vs[0].Kind != types.ViolUncertaintyBlockMissing {
		t.Fatalf("expected ViolUncertaintyBlockMissing; got %+v", vs)
	}
	if !strings.Contains(vs[0].Repair, "caveat") {
		t.Errorf("repair should mention caveat; got %q", vs[0].Repair)
	}
}

func TestUncertaintyBlockPresence_TriggerNotInRequiredSkipped(t *testing.T) {
	view := uncertaintyView()
	view.FacetCoverage.Required = nil // remove the trigger facet
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{summaryBlock("b1")}}
	if vs := validateUncertaintyBlockPresence(doc, view); len(vs) != 0 {
		t.Errorf("trigger absent → skip; got %+v", vs)
	}
}

func TestUncertaintyBlockPresence_NoRulesSkipped(t *testing.T) {
	view := &types.AnswerSemanticView{Family: types.QFGeneric}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{summaryBlock("b1")}}
	if vs := validateUncertaintyBlockPresence(doc, view); len(vs) != 0 {
		t.Errorf("no UncertaintyRules → skip; got %+v", vs)
	}
}

// ── runV2BlockOracles 联合 dispatch ────────────────────────

func TestRunV2BlockOracles_AggregatesAllViolations(t *testing.T) {
	view := uncertaintyView()
	view.RequiredBlocks = []types.BlockRequirement{
		{Kind: types.BlockSummary, MinCount: 1, MaxCount: 1, Required: true, Rationale: "summary"},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		// no summary, no caveat — both missing.
	}}
	vs := runV2BlockOracles(doc, view)
	if len(vs) < 2 {
		t.Errorf("expected ≥2 violations (coverage + uncertainty); got %d (%+v)", len(vs), vs)
	}
}

func TestRunV2BlockOracles_NilGuards(t *testing.T) {
	if vs := runV2BlockOracles(nil, &types.AnswerSemanticView{}); vs != nil {
		t.Errorf("nil doc → nil; got %+v", vs)
	}
	if vs := runV2BlockOracles(&types.AnswerDocumentV2{}, nil); vs != nil {
		t.Errorf("nil view → nil; got %+v", vs)
	}
}

// ── R2.3 V2 重接 facet_uncovered + richness_regression tests ──────

// TestValidateFacetCoverage_AllRequiredCovered confirms no fire
// when every Required facet has a block declaring its FacetID.
func TestValidateFacetCoverage_AllRequiredCovered(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.intro"}},
			{Kind: types.BlockOrderedList, FacetIDs: []string{"facet.steps"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: "facet.intro", Tier: types.TierEssential},
				{Kind: "facet.steps", Tier: types.TierExpected},
			},
		},
	}
	if vs := validateFacetCoverage(doc, view); len(vs) > 0 {
		t.Errorf("all required facets covered: got false fire %+v", vs)
	}
}

// TestValidateFacetCoverage_MissingFires confirms ViolFacetUncovered
// fires when a Required facet has no block declaring its FacetID.
func TestValidateFacetCoverage_MissingFires(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.intro"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: "facet.intro", Tier: types.TierEssential},
				{Kind: "facet.steps", Tier: types.TierExpected},
			},
		},
	}
	vs := validateFacetCoverage(doc, view)
	if len(vs) != 1 {
		t.Fatalf("want 1 violation; got %d (%+v)", len(vs), vs)
	}
	if vs[0].Kind != types.ViolFacetUncovered {
		t.Errorf("kind = %q, want ViolFacetUncovered", vs[0].Kind)
	}
	if !strings.Contains(vs[0].Detail, "facet.steps") {
		t.Errorf("Detail must name uncovered facet; got %q", vs[0].Detail)
	}
}

// TestValidateFacetCoverage_OptionalFacetSkipped confirms
// Tier=Enrichment is NOT a violation here (handled by
// validateRichnessRegression below).
func TestValidateFacetCoverage_OptionalFacetSkipped(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.intro"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: "facet.intro", Tier: types.TierEssential},
				{Kind: "facet.optional_extra", Tier: types.TierEnrichment},
			},
		},
	}
	if vs := validateFacetCoverage(doc, view); len(vs) > 0 {
		t.Errorf("Tier=Enrichment must skip facet_uncovered; got %+v", vs)
	}
}

// TestValidateFacetCoverage_ClaimUseFacetIDCounts confirms FacetID
// declared via item.ClaimUse.FacetID also satisfies coverage.
func TestValidateFacetCoverage_ClaimUseFacetIDCounts(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{
				{Label: "step1", ClaimUse: &types.RenderedClaimUse{FacetID: "facet.steps"}},
			},
		}},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: "facet.steps", Tier: types.TierEssential},
			},
		},
	}
	if vs := validateFacetCoverage(doc, view); len(vs) > 0 {
		t.Errorf("item.ClaimUse.FacetID must satisfy coverage; got %+v", vs)
	}
}

// TestValidateFacetCoverage_NilGuards covers nil cases.
func TestValidateFacetCoverage_NilGuards(t *testing.T) {
	if vs := validateFacetCoverage(nil, &types.AnswerSemanticView{}); vs != nil {
		t.Errorf("nil doc → nil")
	}
	if vs := validateFacetCoverage(&types.AnswerDocumentV2{}, nil); vs != nil {
		t.Errorf("nil view → nil")
	}
	if vs := validateFacetCoverage(&types.AnswerDocumentV2{}, &types.AnswerSemanticView{}); vs != nil {
		t.Errorf("nil FacetCoverage → nil")
	}
}

// TestValidateRichnessRegression_FiresWhenOptionalUnsurfaced
// confirms optional facets with evidence available but no block
// coverage fire ViolRichnessRegression.
func TestValidateRichnessRegression_FiresWhenOptionalUnsurfaced(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.intro"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Optional: []types.FacetRequirement{
				{
					Kind:            "facet.tip",
					Tier:            types.TierEnrichment,
					SourceCandidate: []string{"ev1", "ev2"},
				},
			},
		},
	}
	vs := validateRichnessRegression(doc, view)
	if len(vs) != 1 {
		t.Fatalf("want 1 richness regression; got %d (%+v)", len(vs), vs)
	}
	if vs[0].Kind != types.ViolRichnessRegression {
		t.Errorf("kind = %q, want ViolRichnessRegression", vs[0].Kind)
	}
}

// TestValidateRichnessRegression_NoEvidenceNoFire confirms an
// optional facet with empty SourceCandidate is NOT a regression.
func TestValidateRichnessRegression_NoEvidenceNoFire(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{Kind: types.BlockSummary}},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Optional: []types.FacetRequirement{
				{Kind: "facet.tip", Tier: types.TierEnrichment, SourceCandidate: nil},
			},
		},
	}
	if vs := validateRichnessRegression(doc, view); len(vs) > 0 {
		t.Errorf("no evidence available → not a regression; got %+v", vs)
	}
}

// TestValidateRichnessRegression_CoveredOptionalNoFire confirms
// covered Optional facet does not fire.
func TestValidateRichnessRegression_CoveredOptionalNoFire(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.tip"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Optional: []types.FacetRequirement{
				{Kind: "facet.tip", Tier: types.TierEnrichment, SourceCandidate: []string{"ev1"}},
			},
		},
	}
	if vs := validateRichnessRegression(doc, view); len(vs) > 0 {
		t.Errorf("covered optional must not fire; got %+v", vs)
	}
}

// ── R2.3 V2 重接 ClaimFormUnsupported tests ──────────────────────

// TestValidateClaimFormSupport_MatchPasses confirms when the LLM-
// declared ClaimForm matches ClaimFormOf(evidence), no fire.
func TestValidateClaimFormSupport_MatchPasses(t *testing.T) {
	mut := &types.MutableState{}
	// Definition-anchor evidence projects to ClaimDefinitionFact.
	mut.AppendEvidence([]types.EvidenceItem{{
		ID: "ev1", AnchorKind: types.AnchorDefinition,
		Source: "x.go", LineStart: 10, Subject: "Foo",
	}})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "b1",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				Label: "step1",
				ClaimUse: &types.RenderedClaimUse{
					EvidenceID: "ev1",
					ClaimForm:  types.ClaimDefinitionFact,
				},
			}},
		}},
	}
	if vs := validateClaimFormSupport(doc, mut); len(vs) > 0 {
		t.Errorf("matching ClaimForm must not fire; got %+v", vs)
	}
}

// TestValidateClaimFormSupport_MismatchFires confirms a typed-
// projection mismatch fires ViolClaimFormUnsupported.
func TestValidateClaimFormSupport_MismatchFires(t *testing.T) {
	mut := &types.MutableState{}
	// Call-anchor evidence → ClaimCallEdge.
	mut.AppendEvidence([]types.EvidenceItem{{
		ID: "ev1", AnchorKind: types.AnchorCall,
		Source: "x.go", LineStart: 50, Subject: "callerFn", Object: "calleeFn",
	}})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "b1",
			Kind: types.BlockBulletList,
			Items: []types.AnswerBlockItem{{
				Label: "x",
				ClaimUse: &types.RenderedClaimUse{
					EvidenceID: "ev1",
					ClaimForm:  types.ClaimDefinitionFact, // wrong — evidence is a call
				},
			}},
		}},
	}
	vs := validateClaimFormSupport(doc, mut)
	if len(vs) != 1 {
		t.Fatalf("want 1 violation; got %d (%+v)", len(vs), vs)
	}
	if vs[0].Kind != types.ViolClaimFormUnsupported {
		t.Errorf("kind = %q, want ViolClaimFormUnsupported", vs[0].Kind)
	}
	if !strings.Contains(vs[0].Detail, "definition_fact") || !strings.Contains(vs[0].Detail, "call_edge") {
		t.Errorf("Detail must name both forms; got %q", vs[0].Detail)
	}
}

// TestValidateClaimFormSupport_GeneralisationOK confirms when
// ClaimFormOf(evidence) is ClaimUnknown (projection couldn't lock
// a form), the LLM is allowed to declare a more specific form.
func TestValidateClaimFormSupport_GeneralisationOK(t *testing.T) {
	mut := &types.MutableState{}
	// Bare evidence with no AnchorKind → ClaimUnknown.
	mut.AppendEvidence([]types.EvidenceItem{{ID: "ev1"}})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			Kind: types.BlockBulletList,
			Items: []types.AnswerBlockItem{{
				Label: "x",
				ClaimUse: &types.RenderedClaimUse{
					EvidenceID: "ev1",
					ClaimForm:  types.ClaimAssignmentFact,
				},
			}},
		}},
	}
	if vs := validateClaimFormSupport(doc, mut); len(vs) > 0 {
		t.Errorf("ClaimUnknown projection must allow generalisation; got %+v", vs)
	}
}

// TestValidateClaimFormSupport_UnknownEvidenceIDSkipped confirms
// EvidenceID not in pool is silently skipped (no spurious fire).
func TestValidateClaimFormSupport_UnknownEvidenceIDSkipped(t *testing.T) {
	mut := &types.MutableState{}
	mut.AppendEvidence([]types.EvidenceItem{{ID: "ev1", AnchorKind: types.AnchorDefinition}})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			Kind: types.BlockBulletList,
			Items: []types.AnswerBlockItem{{
				Label: "x",
				ClaimUse: &types.RenderedClaimUse{
					EvidenceID: "ev_phantom",
					ClaimForm:  types.ClaimDefinitionFact,
				},
			}},
		}},
	}
	if vs := validateClaimFormSupport(doc, mut); len(vs) > 0 {
		t.Errorf("unknown EvidenceID must be silently skipped; got %+v", vs)
	}
}

// TestValidateClaimFormSupport_NilGuards covers nil guards.
func TestValidateClaimFormSupport_NilGuards(t *testing.T) {
	if vs := validateClaimFormSupport(nil, &types.MutableState{}); vs != nil {
		t.Errorf("nil doc → nil")
	}
	if vs := validateClaimFormSupport(&types.AnswerDocumentV2{}, nil); vs != nil {
		t.Errorf("nil mut → nil")
	}
	if vs := validateClaimFormSupport(&types.AnswerDocumentV2{}, &types.MutableState{}); vs != nil {
		t.Errorf("empty pool → nil")
	}
}

// TestValidateClaimFormSupport_EmptyClaimFormSkipped confirms when
// LLM didn't declare a ClaimForm (empty), there's nothing to check.
func TestValidateClaimFormSupport_EmptyClaimFormSkipped(t *testing.T) {
	mut := &types.MutableState{}
	mut.AppendEvidence([]types.EvidenceItem{{ID: "ev1", AnchorKind: types.AnchorDefinition}})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			Kind: types.BlockBulletList,
			Items: []types.AnswerBlockItem{{
				Label:    "x",
				ClaimUse: &types.RenderedClaimUse{EvidenceID: "ev1"}, // no ClaimForm declared
			}},
		}},
	}
	if vs := validateClaimFormSupport(doc, mut); len(vs) > 0 {
		t.Errorf("empty ClaimForm must be silently skipped; got %+v", vs)
	}
}

// ── AllViolationKinds 完整性 (4 新 kind 在 covered + kindSymbols 双表) ──
func TestB4ViolationKindsRegistered(t *testing.T) {
	want := []types.ViolationKind{
		types.ViolBlockCoverageMissing,
		types.ViolPrincipalClaimUseMissing,
		types.ViolDiagramEdgeUnsupported,
		types.ViolUncertaintyBlockMissing,
	}
	all := types.AllViolationKinds()
	seen := map[types.ViolationKind]bool{}
	for _, k := range all {
		seen[k] = true
	}
	for _, k := range want {
		if !seen[k] {
			t.Errorf("AllViolationKinds() missing %q (B4 oracle should be registered)", k)
		}
	}
}
