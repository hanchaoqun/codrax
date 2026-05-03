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

// ── runStepIdentifierBackedByEvidenceOracle (Phase 4 ext) ────────

func TestStepIdentifierOracle_NoStepsReturnsEmpty(t *testing.T) {
	if vs := runStepIdentifierBackedByEvidenceOracle(&types.AnswerDocument{}, nil); len(vs) != 0 {
		t.Errorf("no steps → empty; got %+v", vs)
	}
}

func TestStepIdentifierOracle_EmptyVerifiedSetSkips(t *testing.T) {
	// Doc has steps with backtick identifiers, but the typed
	// evidence pool is empty (no evidence emitted, no symbols).
	// Oracle should silently skip — don't penalise legitimate
	// first-emit cases.
	doc := &types.AnswerDocument{
		Steps: []types.AnswerStep{{
			Index: 1, Description: "this calls `Foo` and then `Bar`.",
		}},
	}
	mut := types.NewMutableState("")
	if vs := runStepIdentifierBackedByEvidenceOracle(doc, mut); len(vs) != 0 {
		t.Errorf("empty verified set must skip; got %+v", vs)
	}
}

func TestStepIdentifierOracle_FiresOnUnverifiedIdentifier(t *testing.T) {
	mut := types.NewMutableState("")
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "ev-1", Source: "x.go", LineStart: 5,
			Subject: "RealFunction", AnchorSymbol: "RealFunction"},
	})
	doc := &types.AnswerDocument{
		Steps: []types.AnswerStep{{
			Index: 1, Description: "the answer cites `FabricatedName` here.",
		}},
	}
	vs := runStepIdentifierBackedByEvidenceOracle(doc, mut)
	if len(vs) != 1 || vs[0].Kind != types.ViolStepIdentifierUnverified {
		t.Fatalf("expected ViolStepIdentifierUnverified; got %+v", vs)
	}
	if !strings.Contains(vs[0].Detail, "FabricatedName") {
		t.Errorf("detail should name the unverified id; got %q", vs[0].Detail)
	}
}

func TestStepIdentifierOracle_PassesOnVerifiedIdentifier(t *testing.T) {
	mut := types.NewMutableState("")
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "ev-1", Source: "x.go", LineStart: 5, Subject: "RealFunction", AnchorSymbol: "RealFunction"},
	})
	doc := &types.AnswerDocument{
		Steps: []types.AnswerStep{{
			Index: 1, Description: "the answer cites `RealFunction` here.",
		}},
	}
	if vs := runStepIdentifierBackedByEvidenceOracle(doc, mut); len(vs) != 0 {
		t.Errorf("verified id must pass; got %+v", vs)
	}
}

func TestStepIdentifierOracle_PassesViaAnswerSymbolName(t *testing.T) {
	mut := types.NewMutableState("")
	// Evidence pool empty — verified set must include AnswerSymbol slate.
	doc := &types.AnswerDocument{
		Steps: []types.AnswerStep{{
			Index: 1, Description: "step references `MyHandler`.",
		}},
		Symbols: []types.AnswerSymbol{
			{Name: "MyHandler", File: "h.go", Line: 1, Kind: types.KindFunction},
		},
	}
	if vs := runStepIdentifierBackedByEvidenceOracle(doc, mut); len(vs) != 0 {
		t.Errorf("AnswerSymbol.Name should populate verified set; got %+v", vs)
	}
}

func TestStepIdentifierOracle_PassesViaMentionedEntities(t *testing.T) {
	mut := types.NewMutableState("")
	// MentionedEntities are user-verbatim; the oracle credits them
	// because the user said the name in the question.
	mut.SetRequestModel(types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			MentionedEntities: []string{"UserNamedThing"},
		},
	})
	doc := &types.AnswerDocument{
		Steps: []types.AnswerStep{{
			Index: 1, Description: "step uses `UserNamedThing`.",
		}},
	}
	if vs := runStepIdentifierBackedByEvidenceOracle(doc, mut); len(vs) != 0 {
		t.Errorf("MentionedEntities should populate verified set; got %+v", vs)
	}
}

func TestStepIdentifierOracle_NonIdentifierBackticksIgnored(t *testing.T) {
	mut := types.NewMutableState("")
	mut.AppendEvidence([]types.EvidenceItem{{ID: "x", Source: "x.go", LineStart: 1, Subject: "Real"}})
	// Backticks containing non-identifier content (Chinese, multi-
	// word, punctuation) must be silently ignored — they're prose
	// decoration, not load-bearing identifiers.
	doc := &types.AnswerDocument{
		Steps: []types.AnswerStep{{
			Index: 1, Description: "step has `中文` and `multi word phrase` and `Real`.",
		}},
	}
	if vs := runStepIdentifierBackedByEvidenceOracle(doc, mut); len(vs) != 0 {
		t.Errorf("non-identifier backticks must be ignored, only `Real` is identifier; got %+v", vs)
	}
}

func TestStepIdentifierOracle_DedupsSameMissingId(t *testing.T) {
	mut := types.NewMutableState("")
	mut.AppendEvidence([]types.EvidenceItem{{ID: "x", Source: "x.go", LineStart: 1, Subject: "Real"}})
	doc := &types.AnswerDocument{
		Steps: []types.AnswerStep{{
			Index: 1, Description: "uses `Bogus` and again `Bogus` and `Bogus`.",
		}},
	}
	vs := runStepIdentifierBackedByEvidenceOracle(doc, mut)
	if len(vs) != 1 {
		t.Fatalf("expected 1 violation; got %d", len(vs))
	}
	if !strings.Contains(vs[0].Detail, "Bogus") {
		t.Errorf("detail should name Bogus; got %q", vs[0].Detail)
	}
	if strings.Count(vs[0].Detail, "Bogus") > 2 {
		t.Errorf("dup-id should be reported once in detail; got %q", vs[0].Detail)
	}
}

// ── extractBacktickIdentifiers helper ─────────────────────────────

func TestExtractBacktickIdentifiers_BasicExtraction(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"step calls `Foo` then `Bar`.", []string{"Foo", "Bar"}},
		{"`X` and `Y`", []string{"X", "Y"}},
		// non-identifier content inside backticks: skipped
		{"`中文` `multi word`", nil},
		// no backticks
		{"plain prose with no backticks", nil},
		// unmatched backtick: ignored
		{"unbalanced `Foo without close", nil},
		// nested / consecutive: each pair handled
		{"`A``B`", []string{"A", "B"}},
	}
	for _, c := range cases {
		got := extractBacktickIdentifiers(c.in)
		if !equalStrSliceUnordered(got, c.want) {
			t.Errorf("input=%q: got %v want %v", c.in, got, c.want)
		}
	}
}

func equalStrSliceUnordered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, x := range a {
		seen[x]++
	}
	for _, y := range b {
		if seen[y] == 0 {
			return false
		}
		seen[y]--
	}
	return true
}

// ── sub-kind-aware facet template (Phase 4 extension) ────────────

func TestQFRootCauseTrace_RequiredSubKind_PanicSignal(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentRootCause,
		LogTriage: &types.LogBundle{
			Meta: types.LogMeta{Signals: []types.LogSignal{types.SignalPanic}},
		},
	}
	contract := types.CompileFacetCoverage(rm, nil)
	if contract == nil {
		t.Fatal("expected non-nil contract")
	}
	found := false
	for _, req := range contract.Required {
		if req.Kind == types.FacetObservedArtifactFact && req.RequiredSubKind == types.LogPanicFrame {
			found = true
		}
	}
	if !found {
		t.Errorf("panic signal must narrow FacetObservedArtifactFact RequiredSubKind to LogPanicFrame; got %+v", contract.Required)
	}
}

func TestQFRootCauseTrace_RequiredSubKind_PerfStallWinsOverLog(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentRootCause,
		LogTriage: &types.LogBundle{
			Meta: types.LogMeta{Signals: []types.LogSignal{types.SignalPanic}},
		},
		PerfTrace: &types.PerfBundle{
			Stalls: []types.PerfStall{{File: "render.go"}},
		},
	}
	contract := types.CompileFacetCoverage(rm, nil)
	if contract == nil {
		t.Fatal("expected non-nil contract")
	}
	for _, req := range contract.Required {
		if req.Kind == types.FacetObservedArtifactFact {
			if req.RequiredSubKind != types.PerfStallFrame {
				t.Errorf("perf-stall should win over log-panic; got RequiredSubKind=%q", req.RequiredSubKind)
			}
			return
		}
	}
	t.Error("FacetObservedArtifactFact not found in contract")
}

func TestQFRootCauseTrace_NoSignal_NoSubKind(t *testing.T) {
	rm := types.RequestModel{
		Intent:    types.IntentRootCause,
		LogTriage: &types.LogBundle{},
	}
	contract := types.CompileFacetCoverage(rm, nil)
	if contract == nil {
		t.Fatal("expected non-nil contract")
	}
	for _, req := range contract.Required {
		if req.Kind == types.FacetObservedArtifactFact {
			if req.RequiredSubKind != "" {
				t.Errorf("no-signal bundle should leave RequiredSubKind empty; got %q", req.RequiredSubKind)
			}
			return
		}
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

// ── runValueSecondaryCitationFocusOracle ──────────────────────────
//
// Phase 6 stage 7 (2026-05-03) replacement for the retired emit-time
// validateValueCitationFocus token-overlap heuristic.

func TestRunValueSecondaryCitationFocusOracle_FiresOnOffFocusSecondary(t *testing.T) {
	mut := types.NewMutableState("")
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "ev-1", Source: "x.go", LineStart: 10, LineEnd: 10,
			Subject: "TargetSymbol", AnchorSymbol: "TargetSymbol",
			GroundingStatus: types.GroundingGrounded},
		// Secondary citation overlaps a different evidence item whose
		// typed slots name a SIBLING — the off-focus signal.
		{ID: "ev-2", Source: "x.go", LineStart: 50, LineEnd: 50,
			Subject: "OtherSymbol", AnchorSymbol: "OtherSymbol",
			GroundingStatus: types.GroundingGrounded},
	})
	rm := &types.RequestModel{
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
	}
	doc := &types.AnswerDocument{
		Shape: types.ShapeValue,
		Value: &types.AnswerValue{Literal: "TargetSymbol", CitationRef: 0},
		Citations: []types.Citation{
			{File: "x.go", Line: 10},
			{File: "x.go", Line: 50},
		},
	}
	vs := runValueSecondaryCitationFocusOracle(doc, rm, mut)
	if len(vs) != 1 || vs[0].Kind != types.ViolValueSecondaryCitationOffFocus {
		t.Fatalf("expected 1 ViolValueSecondaryCitationOffFocus; got %+v", vs)
	}
	if !strings.Contains(vs[0].Detail, "TargetSymbol") {
		t.Errorf("detail should name the literal; got %q", vs[0].Detail)
	}
	if !strings.Contains(vs[0].Detail, "citations[1]") {
		t.Errorf("detail should name the offending citation index; got %q", vs[0].Detail)
	}
}

func TestRunValueSecondaryCitationFocusOracle_PassesOnFocusedSecondary(t *testing.T) {
	mut := types.NewMutableState("")
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "ev-1", Source: "x.go", LineStart: 10, LineEnd: 10,
			Subject: "TargetSymbol", AnchorSymbol: "TargetSymbol",
			GroundingStatus: types.GroundingGrounded},
		// Both citations overlap evidence items naming the same literal.
		{ID: "ev-2", Source: "x.go", LineStart: 50, LineEnd: 50,
			Subject: "Caller", Object: "TargetSymbol",
			AnchorSymbol: "Caller",
			GroundingStatus: types.GroundingGrounded},
	})
	rm := &types.RequestModel{
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
	}
	doc := &types.AnswerDocument{
		Shape: types.ShapeValue,
		Value: &types.AnswerValue{Literal: "TargetSymbol", CitationRef: 0},
		Citations: []types.Citation{
			{File: "x.go", Line: 10},
			{File: "x.go", Line: 50},
		},
	}
	if vs := runValueSecondaryCitationFocusOracle(doc, rm, mut); len(vs) != 0 {
		t.Errorf("focused secondary must pass; got %+v", vs)
	}
}

func TestRunValueSecondaryCitationFocusOracle_SkipsOnSingleCitation(t *testing.T) {
	mut := types.NewMutableState("")
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "ev-1", Source: "x.go", LineStart: 10, Subject: "Foo"},
	})
	rm := &types.RequestModel{
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
	}
	doc := &types.AnswerDocument{
		Shape:     types.ShapeValue,
		Value:     &types.AnswerValue{Literal: "Foo", CitationRef: 0},
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	}
	if vs := runValueSecondaryCitationFocusOracle(doc, rm, mut); len(vs) != 0 {
		t.Errorf("single-citation answer should skip; got %+v", vs)
	}
}

func TestRunValueSecondaryCitationFocusOracle_SkipsOnNonValueShape(t *testing.T) {
	mut := types.NewMutableState("")
	rm := &types.RequestModel{
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
	}
	doc := &types.AnswerDocument{
		Shape: types.ShapeStepList,
		Steps: []types.AnswerStep{{Index: 1, Description: "step1"}},
	}
	if vs := runValueSecondaryCitationFocusOracle(doc, rm, mut); len(vs) != 0 {
		t.Errorf("non-value shape should skip; got %+v", vs)
	}
}

func TestRunValueSecondaryCitationFocusOracle_SkipsOnNonFocusSubjectKind(t *testing.T) {
	mut := types.NewMutableState("")
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "ev-1", Source: "x.go", LineStart: 10, Subject: "Off"},
	})
	rm := &types.RequestModel{
		// SubjectNumeric is outside the focus set; the oracle skips
		// because numeric literals lack a stable identifier-lookup
		// discipline.
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectNumeric},
	}
	doc := &types.AnswerDocument{
		Shape: types.ShapeValue,
		Value: &types.AnswerValue{Literal: "Foo", CitationRef: 0},
		Citations: []types.Citation{
			{File: "x.go", Line: 10},
			{File: "x.go", Line: 99},
		},
	}
	if vs := runValueSecondaryCitationFocusOracle(doc, rm, mut); len(vs) != 0 {
		t.Errorf("non-focus subject kind should skip; got %+v", vs)
	}
}

func TestRunValueSecondaryCitationFocusOracle_SkipsOnEmptyPool(t *testing.T) {
	mut := types.NewMutableState("")
	rm := &types.RequestModel{
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
	}
	doc := &types.AnswerDocument{
		Shape: types.ShapeValue,
		Value: &types.AnswerValue{Literal: "Foo", CitationRef: 0},
		Citations: []types.Citation{
			{File: "x.go", Line: 10},
			{File: "x.go", Line: 99},
		},
	}
	if vs := runValueSecondaryCitationFocusOracle(doc, rm, mut); len(vs) != 0 {
		t.Errorf("empty evidence pool should skip; got %+v", vs)
	}
}

func TestRunValueSecondaryCitationFocusOracle_FiresWhenSecondaryHasNoMatchedEvidence(t *testing.T) {
	mut := types.NewMutableState("")
	// Only the primary citation has a matched evidence item.
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "ev-1", Source: "x.go", LineStart: 10, LineEnd: 10,
			Subject: "TargetSymbol", AnchorSymbol: "TargetSymbol",
			GroundingStatus: types.GroundingGrounded},
	})
	rm := &types.RequestModel{
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
	}
	doc := &types.AnswerDocument{
		Shape: types.ShapeValue,
		Value: &types.AnswerValue{Literal: "TargetSymbol", CitationRef: 0},
		Citations: []types.Citation{
			{File: "x.go", Line: 10},
			// Secondary citation hits a line with NO evidence overlap —
			// off-focus by definition (no typed-pool support at all).
			{File: "y.go", Line: 99},
		},
	}
	vs := runValueSecondaryCitationFocusOracle(doc, rm, mut)
	if len(vs) != 1 || vs[0].Kind != types.ViolValueSecondaryCitationOffFocus {
		t.Fatalf("expected ViolValueSecondaryCitationOffFocus; got %+v", vs)
	}
}

func TestRunValueSecondaryCitationFocusOracle_FilePathSubjectKind(t *testing.T) {
	mut := types.NewMutableState("")
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "ev-1", Source: "config/auth.yaml", LineStart: 1, LineEnd: 1,
			Subject: "config/auth.yaml", AnchorSymbol: "config/auth.yaml",
			GroundingStatus: types.GroundingGrounded},
		// Off-focus secondary: different file under typed pool.
		{ID: "ev-2", Source: "config/auth.yaml", LineStart: 20, LineEnd: 20,
			Subject: "different/path.yaml", GroundingStatus: types.GroundingGrounded},
	})
	rm := &types.RequestModel{
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectFilePath},
	}
	doc := &types.AnswerDocument{
		Shape: types.ShapeValue,
		Value: &types.AnswerValue{Literal: "config/auth.yaml", CitationRef: 0},
		Citations: []types.Citation{
			{File: "config/auth.yaml", Line: 1},
			{File: "config/auth.yaml", Line: 20},
		},
	}
	vs := runValueSecondaryCitationFocusOracle(doc, rm, mut)
	if len(vs) != 1 || vs[0].Kind != types.ViolValueSecondaryCitationOffFocus {
		t.Fatalf("expected ViolValueSecondaryCitationOffFocus on file-path off-focus secondary; got %+v", vs)
	}
}
