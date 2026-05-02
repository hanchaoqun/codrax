package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ── runFacetCoverageOracle ─────────────────────────────────────────

func TestFacetCoverageOracle_NilDocOrRMReturnsEmpty(t *testing.T) {
	if vs := runFacetCoverageOracle(nil, &types.RequestModel{}, nil); len(vs) != 0 {
		t.Errorf("nil doc should return empty; got %+v", vs)
	}
	if vs := runFacetCoverageOracle(&types.AnswerDocument{}, nil, nil); len(vs) != 0 {
		t.Errorf("nil rm should return empty; got %+v", vs)
	}
}

func TestFacetCoverageOracle_NoHardFacetsAfterCompileReturnsEmpty(t *testing.T) {
	// QFGeneric template is HARD/SOFT only with no SourceCandidate
	// expansion → all HARD facets degrade to SOFT under Phase 1's
	// fallback. No HARD remaining → no oracle violations.
	rm := &types.RequestModel{}
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation, Summary: "x"}
	if vs := runFacetCoverageOracle(doc, rm, nil); len(vs) != 0 {
		t.Errorf("QFGeneric no-evidence should produce zero hard facets; got %+v", vs)
	}
}

func TestFacetCoverageOracle_FiresOnUncoveredHardFacet(t *testing.T) {
	// Build a request where a HARD facet survives Phase 1 compile:
	// QFConfigPrecedence with a config-shaped evidence item that
	// matches FacetConfigPrecedenceRole's AcceptableForms (so
	// SourceCandidate is non-empty → HARD stays HARD).
	mut := types.NewMutableState("")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			ID: "ev-1", Source: "config.go", LineStart: 10,
			AnchorKind:  types.AnchorAssignment,
			DiagramRole: types.EvidenceDiagramRoleConfig,
		},
	})
	rm := &types.RequestModel{Intent: types.IntentConfigQuery}

	// AnswerDocument has NO claim_use annotations and no citations
	// matching the facet's accepted forms — uncovered.
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "Some answer text without any facet annotations.",
	}
	vs := runFacetCoverageOracle(doc, rm, mut)
	if len(vs) == 0 {
		t.Fatalf("expected at least one ViolFacetUncovered; got 0")
	}
	hasFacetUncovered := false
	for _, v := range vs {
		if v.Kind == types.ViolFacetUncovered {
			hasFacetUncovered = true
			if !strings.Contains(v.Detail, "facet=") {
				t.Errorf("violation Detail should name the facet; got %q", v.Detail)
			}
		}
	}
	if !hasFacetUncovered {
		t.Errorf("expected ViolFacetUncovered kind in violations: %+v", vs)
	}
}

func TestFacetCoverageOracle_PassesOnExplicitClaimUseAnnotation(t *testing.T) {
	// Same setup as the firing case, but the answer's Step carries
	// an explicit ClaimUse with FacetID matching the HARD facet.
	mut := types.NewMutableState("")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			ID: "ev-1", Source: "config.go", LineStart: 10,
			AnchorKind:  types.AnchorAssignment,
			DiagramRole: types.EvidenceDiagramRoleConfig,
		},
	})
	rm := &types.RequestModel{Intent: types.IntentConfigQuery}
	doc := &types.AnswerDocument{
		Shape: types.ShapeStepList,
		Steps: []types.AnswerStep{{
			Index: 1, Description: "Layer 2 override.",
			ClaimUse: &types.RenderedClaimUse{
				FacetID: string(types.FacetConfigPrecedenceRole),
			},
		}},
	}
	for _, v := range runFacetCoverageOracle(doc, rm, mut) {
		if v.Kind == types.ViolFacetUncovered &&
			strings.Contains(v.Detail, string(types.FacetConfigPrecedenceRole)) {
			t.Errorf("explicit claim_use should suppress uncovered violation for that facet; got %+v", v)
		}
	}
}

// ── runClaimFormSupportOracle ──────────────────────────────────────

func TestClaimFormSupportOracle_NoAnnotationReturnsEmpty(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape: types.ShapeStepList,
		Steps: []types.AnswerStep{{Index: 1, Description: "x", CitationRef: 0}},
	}
	if vs := runClaimFormSupportOracle(doc, nil); len(vs) != 0 {
		t.Errorf("no claim_use should return empty; got %+v", vs)
	}
}

func TestClaimFormSupportOracle_FiresOnFormMismatch(t *testing.T) {
	mut := types.NewMutableState("")
	mut.AppendEvidence([]types.EvidenceItem{
		// Citation will resolve to this evidence; AnchorDefinition →
		// ClaimDefinitionFact via ClaimFormOf.
		{
			ID: "ev-def-1", Source: "a.go", LineStart: 5,
			AnchorKind: types.AnchorDefinition,
		},
	})
	doc := &types.AnswerDocument{
		Shape: types.ShapeStepList,
		Citations: []types.Citation{
			{File: "a.go", Line: 5},
		},
		// Step's ClaimUse declares call_edge but the cited evidence
		// is a definition_fact — mismatch.
		Steps: []types.AnswerStep{{
			Index: 1, Description: "x", CitationRef: 0,
			ClaimUse: &types.RenderedClaimUse{
				ClaimForm: types.ClaimCallEdge,
			},
		}},
	}
	vs := runClaimFormSupportOracle(doc, mut)
	if len(vs) != 1 || vs[0].Kind != types.ViolClaimFormUnsupported {
		t.Fatalf("expected ViolClaimFormUnsupported; got %+v", vs)
	}
	if !strings.Contains(vs[0].Detail, string(types.ClaimCallEdge)) ||
		!strings.Contains(vs[0].Detail, string(types.ClaimDefinitionFact)) {
		t.Errorf("violation Detail should name both claim_forms; got %q", vs[0].Detail)
	}
}

func TestClaimFormSupportOracle_PassesOnFormMatch(t *testing.T) {
	mut := types.NewMutableState("")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			ID: "ev-call-1", Source: "a.go", LineStart: 5,
			AnchorKind: types.AnchorCall,
		},
	})
	doc := &types.AnswerDocument{
		Shape:     types.ShapeStepList,
		Citations: []types.Citation{{File: "a.go", Line: 5}},
		Steps: []types.AnswerStep{{
			Index: 1, Description: "x", CitationRef: 0,
			ClaimUse: &types.RenderedClaimUse{
				ClaimForm: types.ClaimCallEdge,
			},
		}},
	}
	if vs := runClaimFormSupportOracle(doc, mut); len(vs) != 0 {
		t.Errorf("matched claim_form should pass; got %+v", vs)
	}
}

func TestClaimFormSupportOracle_UnknownProjectionSkipped(t *testing.T) {
	// Evidence projects to ClaimUnknown (no AnchorKind, no Origin,
	// no DiagramRole) — claim_form check is back-compat-safe and
	// must not fire.
	mut := types.NewMutableState("")
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "ev-vague-1", Source: "a.go", LineStart: 5},
	})
	doc := &types.AnswerDocument{
		Shape:     types.ShapeStepList,
		Citations: []types.Citation{{File: "a.go", Line: 5}},
		Steps: []types.AnswerStep{{
			Index: 1, Description: "x", CitationRef: 0,
			ClaimUse: &types.RenderedClaimUse{ClaimForm: types.ClaimCallEdge},
		}},
	}
	if vs := runClaimFormSupportOracle(doc, mut); len(vs) != 0 {
		t.Errorf("ClaimUnknown evidence projection should not fire violation; got %+v", vs)
	}
}

// ── runAbsenceScopeBoundOracle ────────────────────────────────────

func TestAbsenceScopeOracle_NoAbsenceClaimReturnsEmpty(t *testing.T) {
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation}
	if vs := runAbsenceScopeBoundOracle(doc, nil); len(vs) != 0 {
		t.Errorf("no exact_resolution should return empty; got %+v", vs)
	}
	doc2 := &types.AnswerDocument{
		Shape: types.ShapeExplanation,
		ExactResolution: &types.AnswerExactResolution{
			Status: types.AnswerExactResolutionExactMatch,
		},
	}
	if vs := runAbsenceScopeBoundOracle(doc2, nil); len(vs) != 0 {
		t.Errorf("non-absent status should return empty; got %+v", vs)
	}
}

func TestAbsenceScopeOracle_FiresOnUnboundedAbsence(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape: types.ShapeExplanation,
		ExactResolution: &types.AnswerExactResolution{
			Status: types.AnswerExactResolutionAbsent,
		},
		Citations: []types.Citation{
			// Citation has no Scope=Negative + no NegativePattern.
			{File: "a.go", Line: 1},
		},
	}
	vs := runAbsenceScopeBoundOracle(doc, nil)
	if len(vs) != 1 || vs[0].Kind != types.ViolAbsenceScopeExceeded {
		t.Fatalf("expected ViolAbsenceScopeExceeded; got %+v", vs)
	}
}

func TestAbsenceScopeOracle_PassesWithBoundedNegativeCitation(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape: types.ShapeExplanation,
		ExactResolution: &types.AnswerExactResolution{
			Status: types.AnswerExactResolutionAbsent,
		},
		Citations: []types.Citation{
			{File: "a.go", Line: 1},
			{
				File: "(grep)", Line: 1,
				Scope:           types.ScopeNegative,
				NegativePattern: "ExploreMidLoopHintBudget",
			},
		},
	}
	if vs := runAbsenceScopeBoundOracle(doc, nil); len(vs) != 0 {
		t.Errorf("bounded negative citation should pass; got %+v", vs)
	}
}

func TestAbsenceScopeOracle_RejectsNegativeScopeWithoutPattern(t *testing.T) {
	// Scope=Negative is necessary but not sufficient — without
	// NegativePattern the search query was not recorded, so the
	// scope is still unbounded from an audit perspective.
	doc := &types.AnswerDocument{
		Shape: types.ShapeExplanation,
		ExactResolution: &types.AnswerExactResolution{
			Status: types.AnswerExactResolutionAbsent,
		},
		Citations: []types.Citation{
			{File: "(grep)", Line: 1, Scope: types.ScopeNegative},
		},
	}
	if vs := runAbsenceScopeBoundOracle(doc, nil); len(vs) != 1 {
		t.Errorf("scope=negative without negative_pattern should still fire; got %+v", vs)
	}
}

// ── runRichnessTelemetryOracle (Phase 5) ──────────────────────────

func TestRichnessTelemetryOracle_NilDocOrRMReturnsEmpty(t *testing.T) {
	if vs := runRichnessTelemetryOracle(nil, &types.RequestModel{}, nil); len(vs) != 0 {
		t.Errorf("nil doc should return empty; got %+v", vs)
	}
	if vs := runRichnessTelemetryOracle(&types.AnswerDocument{}, nil, nil); len(vs) != 0 {
		t.Errorf("nil rm should return empty; got %+v", vs)
	}
}

func TestRichnessTelemetryOracle_NoOptionalFacetsReturnsEmpty(t *testing.T) {
	// QFCallChain template has zero FacetOptional entries — telemetry
	// oracle has nothing to flag.
	rm := &types.RequestModel{Intent: types.IntentTrace}
	doc := &types.AnswerDocument{Shape: types.ShapeStepList, Summary: "x"}
	if vs := runRichnessTelemetryOracle(doc, rm, nil); len(vs) != 0 {
		t.Errorf("no optional facets must return empty; got %+v", vs)
	}
}

func TestRichnessTelemetryOracle_FiresOnUncoveredOptional(t *testing.T) {
	// QFConfigPrecedence template has FacetDiagramSpine as the
	// Optional / TierEnrichment entry. Build a request that lands
	// in that family + an uncovered diagram_spine facet.
	mut := types.NewMutableState("")
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "ev-1", Source: "config.go", LineStart: 10,
			AnchorKind:  types.AnchorAssignment,
			DiagramRole: types.EvidenceDiagramRoleConfig},
	})
	rm := &types.RequestModel{Intent: types.IntentConfigQuery}

	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "Plain prose with no claim_use annotations.",
	}
	vs := runRichnessTelemetryOracle(doc, rm, mut)
	if len(vs) == 0 {
		t.Fatal("expected at least one ViolRichnessRegression for uncovered enrichment facet")
	}
	for _, v := range vs {
		if v.Kind != types.ViolRichnessRegression {
			t.Errorf("unexpected kind %q (expected richness_regression)", v.Kind)
		}
		// Repair must NOT mention "Phase 5" / "[CGEC]" / "finalizer"
		// (red-line audit on LLM-facing text).
		for _, banned := range []string{"Phase 5", "[CGEC]", "finalizer"} {
			if strings.Contains(v.Repair, banned) {
				t.Errorf("Repair text leaked internal token %q: %q", banned, v.Repair)
			}
		}
	}
}

func TestRichnessTelemetryOracle_RichnessTier_Default(t *testing.T) {
	// FacetRequirement{} (no Tier set) decodes via EffectiveTier.
	if got := (types.FacetRequirement{Required: types.FacetHardRequired}).EffectiveTier(); got != types.TierEssential {
		t.Errorf("hard → essential; got %q", got)
	}
	if got := (types.FacetRequirement{Required: types.FacetSoftRequired}).EffectiveTier(); got != types.TierExpected {
		t.Errorf("soft → expected; got %q", got)
	}
	if got := (types.FacetRequirement{Required: types.FacetOptional}).EffectiveTier(); got != types.TierEnrichment {
		t.Errorf("optional → enrichment; got %q", got)
	}
	// Explicit Tier overrides Required-derived value.
	if got := (types.FacetRequirement{
		Required: types.FacetHardRequired,
		Tier:     types.TierEnrichment,
	}).EffectiveTier(); got != types.TierEnrichment {
		t.Errorf("explicit Tier should override; got %q", got)
	}
}

// ── master switch ──────────────────────────────────────────────────

func TestFacetValidatorsEnabled_DefaultAndSet(t *testing.T) {
	original := FacetValidatorsEnabled()
	t.Cleanup(func() { SetFacetValidatorsEnabled(original) })

	if !FacetValidatorsEnabled() {
		t.Fatal("default must be true")
	}
	SetFacetValidatorsEnabled(false)
	if FacetValidatorsEnabled() {
		t.Error("after SetFacetValidatorsEnabled(false), getter should return false")
	}
	SetFacetValidatorsEnabled(true)
	if !FacetValidatorsEnabled() {
		t.Error("after SetFacetValidatorsEnabled(true), getter should return true")
	}
}
