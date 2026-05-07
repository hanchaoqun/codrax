package hint

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// G3 HintComposer tests — lock the 6-field contract and the
// structured render path. Non-strict rendering tolerates empty
// input; strict mode fails loud on missing fields.

func TestCompose_EmptyViolations_NonStrictReturnsEmptyHint(t *testing.T) {
	c := New(DefaultConfig())
	h, err := c.Compose(Context{}, nil)
	if err != nil {
		t.Fatalf("non-strict should tolerate empty input, got err=%v", err)
	}
	if h == nil {
		t.Fatalf("non-strict should return non-nil Hint for empty input")
	}
	// Render must not contain a "Retry Directive" heading: that is
	// now the prompt builder's job (section title). Emitting the
	// same H2 here would nest two identical headings in the final
	// user message.
	got := c.Render(h)
	if strings.Contains(got, "Retry Directive") {
		t.Errorf("Render must NOT carry a Retry Directive heading; builder owns that title. got %q", got)
	}
}

// TestRender_NoDuplicateHeader — positive check on a populated Hint:
// whichever structured body bullets render (WhatFailed / WhyItFailed
// / …) must do so without a leading H2 header, so the composer
// output splices cleanly under the builder's
// "Retry Directive (READ FIRST)" section title.
func TestRender_NoDuplicateHeader(t *testing.T) {
	c := New(DefaultConfig())
	h := &Hint{
		WhatFailed: "contract mismatch",
		ExactFix:   "pick another shape",
	}
	out := c.Render(h)
	if strings.Contains(out, "## Retry Directive") {
		t.Errorf("Render leaked a Retry Directive H2 heading into body: %q", out)
	}
	if !strings.Contains(out, "**What failed**") {
		t.Errorf("Render dropped the WhatFailed bullet: %q", out)
	}
}

func TestCompose_EmptyViolations_StrictErrors(t *testing.T) {
	c := New(Config{StrictMode: true})
	if _, err := c.Compose(Context{}, nil); err == nil {
		t.Fatal("strict mode should reject empty violations")
	}
}

func TestCompose_ShapeViolation_BuildsAllowedAndForbidden(t *testing.T) {
	c := New(DefaultConfig())
	v := []types.Violation{{
		Kind:   types.ViolFamilyMismatch,
		Detail: "LLM chose shape=boolean but contract requires value",
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "question_kind",
			Reason:     "finalizer picked boolean",
			Confidence: 0.85,
		},
	}}
	h, err := c.Compose(Context{
		TargetFamily:         types.QFRoleLookup,
		TargetRequiredBlocks: []types.AnswerBlockKind{types.BlockSummary, types.BlockScalar},
	}, v)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if h.WhatFailed == "" {
		t.Errorf("WhatFailed empty")
	}
	if !strings.Contains(h.WhyItFailed, "question_kind") {
		t.Errorf("WhyItFailed must mention SuspectedRoot.IRField, got %q", h.WhyItFailed)
	}
	if len(h.AllowedSet) != 2 || h.AllowedSet[0].Value != "summary" || h.AllowedSet[1].Value != "scalar" {
		t.Errorf("AllowedSet should enumerate the required block kinds, got %v", h.AllowedSet)
	}
	if len(h.ForbiddenPatterns) == 0 {
		t.Errorf("ForbiddenPatterns must contain the negative example")
	}
	out := c.Render(h)
	if !strings.Contains(out, "**Why it failed**") || !strings.Contains(out, "**How to fix now**") {
		t.Errorf("Render missing mandatory section headers; got %q", out)
	}
}

func TestCompose_CitationViolation_AllowedFromReadSet(t *testing.T) {
	c := New(DefaultConfig())
	v := []types.Violation{{
		Kind:          types.ViolCitation,
		Detail:        "0/3 citations in ReadSet",
		SuspectedRoot: types.SuspectedRoot{IRField: "ScannedSet", Confidence: 0.9},
	}}
	readSet := []string{"a.go", "b.go", "c.go"}
	h, _ := c.Compose(Context{ReadSet: readSet}, v)
	if len(h.AllowedSet) != 3 {
		t.Errorf("AllowedSet len=%d, want 3 (one per ReadSet file)", len(h.AllowedSet))
	}
	out := c.Render(h)
	if !strings.Contains(out, "a.go") {
		t.Errorf("Render must include ReadSet files, got %q", out)
	}
}

func TestCompose_SelfRefLiteral_ForbidsPrimaryEntity(t *testing.T) {
	c := New(DefaultConfig())
	v := []types.Violation{{
		Kind: types.ViolSelfRefLiteral,
		SuspectedRoot: types.SuspectedRoot{
			IRField: "answer_subject.kind", Confidence: 0.75,
		},
	}}
	h, _ := c.Compose(Context{PrimaryEntity: "explorer"}, v)
	var found bool
	for _, p := range h.ForbiddenPatterns {
		if strings.Contains(p, "explorer") {
			found = true
		}
	}
	if !found {
		t.Errorf("ForbiddenPatterns must call out self-reference to primary entity; got %v", h.ForbiddenPatterns)
	}
}

func TestCompose_StrictModeRequiresAllFields(t *testing.T) {
	c := New(Config{StrictMode: true, MaxAllowedSet: 5})
	// Missing TargetFamily / TargetRequiredBlocks → AllowedSet empty
	// for a shape violation → Validate fails.
	v := []types.Violation{{
		Kind:          types.ViolFamilyMismatch,
		Detail:        "boolean vs value",
		SuspectedRoot: types.SuspectedRoot{IRField: "question_kind", Confidence: 0.85},
	}}
	if _, err := c.Compose(Context{ /* no TargetFamily / TargetRequiredBlocks */ }, v); err == nil {
		t.Fatal("strict mode must fail when AllowedSet is empty")
	}
}

func TestRenderCompact_PreservesLegacySemicolonFormat(t *testing.T) {
	c := New(DefaultConfig())
	v := []types.Violation{
		{Kind: types.ViolFamilyMismatch, Detail: "boolean vs value"},
		{Kind: types.ViolCitation, Detail: "missing line", Repair: "add :N"},
	}
	h, _ := c.Compose(Context{}, v)
	got := c.RenderCompact(h)
	if got != "boolean vs value; missing line (add :N)" {
		t.Errorf("RenderCompact format drift: got %q", got)
	}
}

func TestLimitAllowedAndForbidden_TrimsWithEllipsis(t *testing.T) {
	c := New(Config{MaxAllowedSet: 2, MaxForbiddenPatterns: 1})
	v := []types.Violation{{
		Kind:          types.ViolCitation,
		SuspectedRoot: types.SuspectedRoot{IRField: "ScannedSet", Confidence: 0.9},
	}}
	ctx := Context{ReadSet: []string{"a.go", "b.go", "c.go", "d.go"}}
	h, _ := c.Compose(ctx, v)
	if len(h.AllowedSet) != 3 { // 2 kept + 1 "...and N more"
		t.Errorf("AllowedSet len=%d, want 3 (cap 2 + ellipsis)", len(h.AllowedSet))
	}
	if !strings.Contains(h.AllowedSet[2].Value, "more") {
		t.Errorf("ellipsis entry missing 'more' marker, got %v", h.AllowedSet[2])
	}
}

// composerExactFixSwitchKinds is the **canonical exhaustive list**
// of ViolationKind values that summariseExactFix MUST handle as a
// dedicated case. New ViolationKind constants added in
// internal/types/violation.go are NOT auto-included here — extend
// this slice when adding a kind that needs a dedicated repair-text
// branch, OR add the kind to the explicitly-skip whitelist below.
//
// This guards against P34: "violation kind ↔ retry-hint must be 1:1
// mapped". A ViolationKind without a switch case in summariseExactFix
// falls through to the generic "Address the violation(s) listed
// above and re-emit." which gives the LLM no actionable repair.
//
// When CI fails on TestComposer_AllViolationKindsHaveCase, the
// developer choices are:
//
//   1. Add a `case types.ViolXxx: return "..."` branch in
//      summariseExactFix and add `types.ViolXxx` to this slice
//      (preferred — the LLM gets actionable repair text).
//
//   2. Add `types.ViolXxx` to composerExactFixSkipWhitelist below
//      with a comment explaining why a generic fallback is OK.
var composerExactFixSwitchKinds = map[types.ViolationKind]bool{
	// Pre-V2 contract kinds.
	types.ViolFamilyMismatch:       true,
	types.ViolViewSwap:             true,
	types.ViolCitation:             true,
	types.ViolGhostAnchor:          true,
	types.ViolSelfRefLiteral:       true,
	types.ViolLiteralFormFailed:    true,
	types.ViolPreCompleteDowngrade: true,
	types.ViolMustInclude:          true,
	types.ViolMustExclude:          true,
	types.ViolAcceptance:           true,
	types.ViolSuccessCriterion:     true,
	// V2 carrier kinds (B2 1:1 mapping rollout).
	types.ViolPrincipalClaimUseMissing:        true,
	types.ViolDiagramEdgeUnsupported:          true,
	types.ViolUncertaintyBlockMissing:         true,
	types.ViolClaimFormUnsupported:            true,
	types.ViolBlockCoverageMissing:            true,
	types.ViolMissingRequestedRoleUndisclosed: true,
}

// composerExactFixSkipWhitelist names ViolationKind values that
// intentionally fall through to the generic fallback. Each entry
// must justify itself in a one-line comment.
var composerExactFixSkipWhitelist = map[types.ViolationKind]string{
	// Block 1 reviewer kinds — informational, mapped to FailLoud
	// in fallback_policy. They never reach summariseExactFix
	// because the run terminates before retry, so a generic
	// fallback is OK.
	types.ViolPlanCritic:              "Block-1 reviewer kind, FailLoud — never retries",
	types.ViolReflectorObservation:    "Block-1 reviewer kind, FailLoud — never retries",
	types.ViolAnswerReviewerDistilled: "Block-1 reviewer kind, FailLoud — never retries",

	// Block 2 + 3 kinds — these have fallback policy mappings but
	// the repair text is best derived from violation.Repair (set
	// upstream by the validator with kind-specific detail). The
	// generic fallback "Address the violation(s) listed above"
	// pairs with violation.Detail to give actionable text.
	types.ViolSelfContradiction:            "uses violation.Repair for fix prose",
	types.ViolDeclaredCountDrift:           "uses violation.Repair for fix prose",
	types.ViolDiagramIdentifier:            "uses violation.Repair for fix prose",
	types.ViolChainDemoted:                 "uses violation.Repair for fix prose",
	types.ViolViewIntentMismatch:           "uses violation.Repair for fix prose",
	types.ViolSubTopicCountMismatch:        "uses violation.Repair for fix prose",
	types.ViolExternalArtifactUnderdecoded: "uses violation.Repair for fix prose",
	types.ViolAuthorityOverreach:           "uses violation.Repair for fix prose",
	types.ViolIntentTraceShallow:           "uses violation.Repair for fix prose",
	types.ViolIntentEnumerateNotList:       "uses violation.Repair for fix prose",
	types.ViolIntentRootCauseNoCause:       "uses violation.Repair for fix prose",
	types.ViolIntentConfigNoTrail:          "uses violation.Repair for fix prose",
	types.ViolSubjectAnchorMissing:         "uses violation.Repair for fix prose",
	types.ViolPredicateAxisMissing:         "uses violation.Repair for fix prose",

	// Phase 4 (Semantic Surface Contract) kinds — same pattern.
	types.ViolFacetUncovered:           "uses violation.Repair for fix prose",
	types.ViolAbsenceScopeExceeded:     "uses violation.Repair for fix prose",
	types.ViolStepIdentifierUnverified: "uses violation.Repair for fix prose",

	// P1 #3 / P3 #6 / Phase 5 / Phase 6 stage 7 — same pattern.
	types.ViolSymbolAnchorMismatch:               "uses violation.Repair for fix prose",
	types.ViolEnumerationLabelUngrounded:         "uses violation.Repair for fix prose",
	types.ViolEnumerationItemLabelExtractorDrift: "uses violation.Repair for fix prose",
	types.ViolEnumerationLabelHallucinated:       "uses violation.Repair for fix prose (validator stamps per-block list of fabricated identifiers + the rewrite directive)",
	types.ViolDiagramEdgeEndpointHallucinated:    "uses violation.Repair for fix prose (validator stamps per-block list of fabricated mermaid endpoint identifiers + the rewrite directive)",
	types.ViolStructuralEnumerationDivergence:    "uses violation.Repair for fix prose",
	types.ViolLaneBlockKindMismatch:              "uses violation.Repair for fix prose (validator emits per-block lane attribution + AllowedBlocks set)",
	types.ViolRichnessRegression:                 "uses violation.Repair for fix prose",
	types.ViolValueSecondaryCitationOffFocus:     "uses violation.Repair for fix prose",

	// B6-F1 (post-shape consolidated audit, 2026-05-04) —
	// cross-citation single-locus oracle. Same pattern as the other
	// B5+/B6 oracles: validator stamps a kind-specific repair string
	// on Violation.Repair (names the conflicting symbol + the two
	// disagreeing loci verbatim) so the generic composer fallback
	// pairs naturally with the actionable Detail.
	types.ViolCrossCitationConflict: "uses violation.Repair for fix prose",

	// R10 (post_shape_residual_audit.md, 2026-05-04) — CGEC
	// frequency-to-violation bridges. Telemetry-only (operator-side
	// signals, not LLM-actionable) — Repair text addresses the
	// operator, not the LLM.
	types.ViolDemotionStorm:   "telemetry only — operator review signal, no LLM repair",
	types.ViolForcedReadStorm: "telemetry only — operator review signal, no LLM repair",

	// G3 (post_v2_runtime_gap_remediation, 2026-05-04) — typed-vs-label
	// drift advisory. SOFT-only and DELIBERATELY not promotable to
	// STRICT; the validator's Violation.Repair already names the
	// drifting edges and the section to consult, so the generic
	// composer fallback pairs with that actionable Detail.
	types.ViolDiagramEdgeLabelMismatch: "uses violation.Repair for fix prose",
	// G5 (post_v2_runtime_gap_remediation, 2026-05-04) — semantic
	// quality reviewer thinness signal. Reviewer-side Concerns are
	// surfaced verbatim via Violation.Repair (built by
	// runSemanticQualityReview in orchestrator/contract_check.go);
	// the generic composer fallback pairs with that actionable
	// Detail.
	types.ViolAnswerSemanticUnderfilled: "uses violation.Repair for fix prose",
	// 修 B (post_v2_runtime_gap_remediation, 2026-05-04) —
	// enumeration evidence pool structural gate. Repair text names
	// the missing N − K count + the gap-filling guidance verbatim;
	// composer fallback pairs.
	types.ViolEnumerationEvidenceUnderspecified: "uses violation.Repair for fix prose",
	// B3 v3 (2026-05-04) — diagram relation typed-first label-only
	// advisory. SOFT-only; validator's Violation.Repair names the
	// label-only edges + the typed declaration the LLM should add,
	// so the generic composer fallback pairs naturally.
	types.ViolDiagramRelationLabelOnly: "uses violation.Repair for fix prose",
	// B2 v3 (2026-05-04) — three-layer quality contract.
	// Validator Violation.Repair names the offending facet kind /
	// block id with actionable language; composer fallback pairs.
	types.ViolRichnessGlaringGap:        "uses violation.Repair for fix prose",
	types.ViolPrincipalProseUnderfilled: "uses violation.Repair for fix prose",
}

// TestComposer_AllViolationKindsHaveCase enforces P34's
// "violation kind ↔ retry-hint 1:1 mapping" invariant. Every
// declared ViolationKind must either:
//
//   (a) appear in composerExactFixSwitchKinds (has a dedicated
//       summariseExactFix case), OR
//
//   (b) appear in composerExactFixSkipWhitelist with justification
//       (intentionally uses the generic fallback).
//
// New ViolationKind constants without coverage will fail this test;
// the developer must explicitly choose (a) or (b) before merging.
func TestComposer_AllViolationKindsHaveCase(t *testing.T) {
	all := types.AllViolationKinds()
	if len(all) == 0 {
		t.Fatal("types.AllViolationKinds() returned empty — the upstream registry should not be empty")
	}
	missing := make([]types.ViolationKind, 0, len(all))
	for _, k := range all {
		if composerExactFixSwitchKinds[k] {
			continue
		}
		if _, skip := composerExactFixSkipWhitelist[k]; skip {
			continue
		}
		missing = append(missing, k)
	}
	if len(missing) > 0 {
		t.Fatalf("ViolationKind(s) missing dedicated summariseExactFix case AND not whitelisted: %v\n"+
			"  → add a `case types.ViolXxx:` in summariseExactFix + extend composerExactFixSwitchKinds, "+
			"OR add to composerExactFixSkipWhitelist with a justification comment.", missing)
	}
}

// TestComposer_V2ViolationsRouteThroughV2Vocabulary verifies the
// new V2-specific exact-fix branches actually fire and use V2
// vocabulary (claim_use / diagram.kind / blocks[]) rather than
// V1 leftovers (steps[] / symbols[] / shape).
func TestComposer_V2ViolationsRouteThroughV2Vocabulary(t *testing.T) {
	c := New(DefaultConfig())
	cases := []struct {
		name           string
		kind           types.ViolationKind
		mustContain    []string
		mustNotContain []string
	}{
		{
			name:           "principal_claim_use_missing",
			kind:           types.ViolPrincipalClaimUseMissing,
			mustContain:    []string{"claim_uses", "citation_ref", "principal block"},
			mustNotContain: []string{"shape=", "steps[]"},
		},
		{
			name:           "diagram_edge_unsupported",
			kind:           types.ViolDiagramEdgeUnsupported,
			mustContain:    []string{"diagram.kind", "SEMANTIC family", "diagram.body"},
			mustNotContain: []string{"shape=value", "shape=explanation"},
		},
		{
			name:           "uncertainty_block_missing",
			kind:           types.ViolUncertaintyBlockMissing,
			mustContain:    []string{"caveat", "claim_uses", "claim_form"},
			mustNotContain: []string{"shape="},
		},
		{
			name:           "claim_form_unsupported",
			kind:           types.ViolClaimFormUnsupported,
			mustContain:    []string{"claim_uses[]", "citation_ref"},
			mustNotContain: []string{"shape="},
		},
		{
			name:           "block_coverage_missing",
			kind:           types.ViolBlockCoverageMissing,
			mustContain:    []string{"blocks[]", "Required Answer Blocks"},
			mustNotContain: []string{"steps[]", "symbols[]"},
		},
		{
			name:           "missing_requested_role_undisclosed",
			kind:           types.ViolMissingRequestedRoleUndisclosed,
			mustContain:    []string{"missing_requested_roles[]", "role:<default|config|runtime|override>", "semantic-view contract"},
			mustNotContain: []string{"shape=", "steps[]"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, err := c.Compose(Context{}, []types.Violation{{
				Kind:   tc.kind,
				Detail: "test",
			}})
			if err != nil {
				t.Fatalf("Compose returned err=%v", err)
			}
			fix := h.ExactFix
			for _, want := range tc.mustContain {
				if !strings.Contains(fix, want) {
					t.Errorf("%s ExactFix missing %q; got %q", tc.name, want, fix)
				}
			}
			for _, banned := range tc.mustNotContain {
				if strings.Contains(fix, banned) {
					t.Errorf("%s ExactFix leaks V1 token %q; got %q", tc.name, banned, fix)
				}
			}
		})
	}
}

// TestComposer_ClaimFormVocabularyMatchesInternalEnum (R1.2 lock,
// post_shape_residual_audit.md 2026-05-04) — the
// ViolClaimFormUnsupported AllowedClaimForm set surfaced to the LLM
// MUST be exactly the canonical AllClaimForms() set. Adding /
// removing a ClaimForm value MUST update both the enum AND this
// composer arm; the test shouts when they diverge.
//
// Pinned because:
//   1. validateClaimFormSupport compares LLM-emitted ClaimForm
//      against ClaimFormOf(evidence) projection, which only ever
//      yields one of the AllClaimForms() values. Any LLM-facing
//      hint listing higher-level "user-friendly" names (e.g. the
//      pre-R1.2 mechanism_step / enumeration_member / etc.) is a
//      contract drift bug — those forms can never satisfy the
//      validator and the LLM gets stuck false-firing.
//   2. The "no alias layer" red line means LLM and internal share
//      one vocabulary. This test enforces it structurally.
func TestComposer_ClaimFormVocabularyMatchesInternalEnum(t *testing.T) {
	got := buildAllowedSet([]types.Violation{{Kind: types.ViolClaimFormUnsupported}}, Context{})
	gotSet := map[string]bool{}
	for _, a := range got {
		if a.Kind == AllowedClaimForm {
			gotSet[a.Value] = true
		}
	}
	for _, want := range types.AllClaimForms() {
		if !gotSet[string(want)] {
			t.Errorf("composer ViolClaimFormUnsupported missing canonical ClaimForm %q from AllClaimForms()", want)
		}
	}
	if want, got := len(types.AllClaimForms()), len(gotSet); want != got {
		t.Errorf("composer surfaces %d claim_form values, AllClaimForms has %d — vocabulary drift", got, want)
	}
	// Banned: any of the V1-era higher-level names that don't exist
	// in the internal enum.
	for _, banned := range []string{
		"mechanism_step", "enumeration_member", "derivation_step",
		"divergence_caveat", "observed_artifact_fact", "assignment",
	} {
		if gotSet[banned] {
			t.Errorf("composer surfaces V1-era stale name %q (must be removed per R1.2)", banned)
		}
	}
}
