package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ── DefaultFallbackPolicy ─────────────────────────────────────────

func TestDefaultFallbackPolicy_CoversEveryViolationKind(t *testing.T) {
	// Every declared ViolationKind MUST have an entry in the
	// default policy. A new kind added without a mapping silently
	// degrades to FinalizerOnly via FallbackTargetForKind, which is
	// safe but masks the omission. This structural test catches it.
	p := DefaultFallbackPolicy()
	for _, kind := range types.AllViolationKinds() {
		if _, ok := p[kind]; !ok {
			t.Errorf("DefaultFallbackPolicy is missing %q (Block 3 requires every ViolationKind to map explicitly)", kind)
		}
	}
}

func TestDefaultFallbackPolicy_KnownPairs(t *testing.T) {
	// Spot-check the production-critical mappings — if any of these
	// drift, retry behaviour drifts visibly for the audit-2 trace
	// patterns Block 3 was designed to fix.
	cases := map[types.ViolationKind]FallbackTarget{
		// Phase 1-D source-fix (s1a-20260504-130143 forensic): SelfContradiction
		// is finalizer-prose-internal; let finalizer rewrite, do NOT
		// re-explore. Pre-fix BackToExplore amplified abstraction drift
		// in the second pass.
		types.ViolSelfContradiction:     FallbackFinalizerOnly,
		types.ViolViewIntentMismatch:   FallbackFinalizerOnly,
		types.ViolDeclaredCountDrift:    FallbackBackToExtract,
		types.ViolDiagramIdentifier:     FallbackFinalizerOnly,
		types.ViolMustInclude:           FallbackFinalizerOnly,
		types.ViolMustExclude:           FallbackFinalizerOnly,
		types.ViolCitation:              FallbackBackToExplore,
		types.ViolGhostAnchor:           FallbackBackToExplore,
		types.ViolAcceptance:            FallbackBackToExplore,
		types.ViolSuccessCriterion:      FallbackBackToExplore,
		types.ViolPlanCritic:            FallbackFailLoud,
		types.ViolReflectorObservation:  FallbackFailLoud,
		types.ViolIntentTraceShallow:    FallbackBackToExplore,
		types.ViolIntentEnumerateNotList: FallbackFinalizerOnly,
		types.ViolSubjectAnchorMissing:  FallbackBackToExtract,
		types.ViolPredicateAxisMissing:  FallbackBackToExplore,
	}
	for kind, want := range cases {
		got := FallbackTargetForKind(kind)
		if got != want {
			t.Errorf("FallbackTargetForKind(%q) = %s, want %s", kind, got, want)
		}
	}
}

// ── FallbackTargetForViolations ───────────────────────────────────

func TestFallbackTargetForViolations_PicksDeepest(t *testing.T) {
	// Mixed set: FinalizerOnly + BackToExplore should resolve to
	// BackToExplore (deeper). FailLoud beats BackToExplore.
	cases := []struct {
		name string
		vs   []types.Violation
		want FallbackTarget
	}{
		{
			name: "empty → finalizer_only",
			vs:   nil,
			want: FallbackFinalizerOnly,
		},
		{
			name: "single ViolFamilyMismatch (FinalizerOnly)",
			vs:   []types.Violation{{Kind: types.ViolFamilyMismatch}},
			want: FallbackFinalizerOnly,
		},
		{
			name: "single ViolCitation (BackToExplore)",
			vs:   []types.Violation{{Kind: types.ViolCitation}},
			want: FallbackBackToExplore,
		},
		{
			name: "Shape + Citation → BackToExplore wins",
			vs: []types.Violation{
				{Kind: types.ViolFamilyMismatch},
				{Kind: types.ViolCitation},
			},
			want: FallbackBackToExplore,
		},
		{
			name: "DeclaredCountDrift + Citation → BackToExplore",
			vs: []types.Violation{
				{Kind: types.ViolDeclaredCountDrift},
				{Kind: types.ViolCitation},
			},
			want: FallbackBackToExplore,
		},
		{
			// Phase 1-C: SOFT-only plan (PlanCritic alone) defaults
			// to FinalizerOnly when no HARD cluster is present.
			// Telemetry-only violation cannot block ship.
			name: "PlanCritic alone → FinalizerOnly (SOFT-only plan)",
			vs:   []types.Violation{{Kind: types.ViolPlanCritic}},
			want: FallbackFinalizerOnly,
		},
		{
			// Phase 1-C: SOFT FailLoud kind + HARD cluster → HARD
			// drives routing (Citation is BackToExplore-mapped).
			name: "PlanCritic + Citation → BackToExplore (HARD wins, SOFT FailLoud no-op)",
			vs: []types.Violation{
				{Kind: types.ViolPlanCritic},
				{Kind: types.ViolCitation},
			},
			want: FallbackBackToExplore,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FallbackTargetForViolations(tc.vs); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// TestFallbackTargetForViolations_RootCauseClustering pins Phase 1-A3
// cooccurrence-cluster picker semantics, replacing the pre-A3 "bucket
// by locus, count + deepest-tiebreak" model.
//
// Behaviour change vs pre-A3 (intentional, per design doc §3.1):
//
//   The pre-A3 picker chose by majority count: 2 finalizer + 1 explore
//   went to FinalizerOnly because finalizer-locus had more violations.
//   This is wrong when the lone explore violation is INDEPENDENT
//   (different root cause) — finalizer-only retries can never fix
//   the explore-local issue, so majority just delays the inevitable
//   escalation while burning retry budget.
//
//   The post-A3 picker uses BuildRepairPlan: typed cooccurrence rules
//   detect cause→consequence relationships and cluster only those
//   together (Derived do NOT inflate Primary's owner count).
//   Independent violations are singleton clusters; the deepest
//   cluster owner wins. R2.2 budget downgrade still applies as a
//   cost-optimization but is no longer the primary semantic.
func TestFallbackTargetForViolations_RootCauseClustering(t *testing.T) {
	cases := []struct {
		name string
		vs   []types.Violation
		want FallbackTarget
	}{
		{
			// 2 INDEPENDENT finalizer-local + 1 INDEPENDENT explore.
			// (PrincipalClaimUseMissing + DiagramEdgeUnsupported are
			// finalize-local kinds; without their parents
			// BlockCoverageMissing/FacetUncovered/ChainDemoted in the
			// set, no cooccurrence rule applies and they remain
			// singletons. Citation is independent explore-local.)
			//
			// Pre-A3 picked FinalizerOnly (majority); A3 picks
			// BackToExplore — the explore issue cannot be drowned
			// out by independent finalizer-local issues from a
			// different root cause.
			name: "3 independent (2 fin + 1 explore) → back_to_explore (deepest cluster wins)",
			vs: []types.Violation{
				{Kind: types.ViolPrincipalClaimUseMissing},
				{Kind: types.ViolDiagramEdgeUnsupported},
				{Kind: types.ViolCitation},
			},
			want: FallbackBackToExplore,
		},
		{
			// Same shape as above but 2 explore + 1 finalizer →
			// still back_to_explore. (Citation + GhostAnchor are
			// independent unless their derived consequences
			// StepIdentifierUnverified / EnumerationLabelUngrounded
			// are also present — they aren't here, so 2 singletons.)
			name: "2 explore + 1 finalizer → back_to_explore (deepest cluster wins)",
			vs: []types.Violation{
				{Kind: types.ViolCitation},
				{Kind: types.ViolGhostAnchor},
				{Kind: types.ViolPrincipalClaimUseMissing},
			},
			want: FallbackBackToExplore,
		},
		{
			// 1 finalizer + 1 extract: independent singletons; deepest
			// wins (extract).
			name: "1 finalizer + 1 extract → extract (deepest singleton)",
			vs: []types.Violation{
				{Kind: types.ViolPrincipalClaimUseMissing},
				{Kind: types.ViolDeclaredCountDrift},
			},
			want: FallbackBackToExtract,
		},
		{
			// COOCCURRENCE CLUSTER: SubjectAnchorMissing's typed
			// pipeline invariant is the precondition of step/symbol
			// verification; Derived violations DO NOT inflate the
			// owner. Single cluster Owner=Extract.
			name: "SubjectAnchorMissing + 2 derived → extract (one cluster)",
			vs: []types.Violation{
				{Kind: types.ViolSubjectAnchorMissing},
				{Kind: types.ViolStepIdentifierUnverified},
				{Kind: types.ViolSymbolAnchorMismatch},
			},
			want: FallbackBackToExtract,
		},
		{
			// COOCCURRENCE CLUSTER: BlockCoverageMissing peels its
			// derived consequences; cluster Owner=Extract.
			name: "BlockCoverageMissing + diagram edge derived → extract (one cluster)",
			vs: []types.Violation{
				{Kind: types.ViolBlockCoverageMissing,
					Detail: "required block kind=ordered_list appears 0 time(s)"},
				{Kind: types.ViolDiagramEdgeUnsupported},
			},
			want: FallbackBackToExtract,
		},
		{
			// FailLoud preserved: any LocusTerminal cluster forces
			// SOFT FailLoud kind alongside HARD clusters does NOT
			// override (Phase 1-C SOFT-immune: PlanCritic is
			// telemetry-only; HARD PrincipalClaimUseMissing drives
			// routing via finalizer-locus instead).
			name: "SOFT FailLoud kind does not override HARD clusters",
			vs: []types.Violation{
				{Kind: types.ViolPrincipalClaimUseMissing},
				{Kind: types.ViolDiagramEdgeUnsupported},
				{Kind: types.ViolPlanCritic}, // SOFT FailLoud-mapped
			},
			want: FallbackFinalizerOnly,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FallbackTargetForViolations(tc.vs); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// TestFallbackTargetForViolation_BlockCoverageSubClassification pins
// B3-F2's per-violation override on ViolBlockCoverageMissing: when
// the missing block kind is `diagram` or `table` (visualisation
// blocks the finalizer can re-emit from existing evidence), the
// fallback target overrides the kind's default BackToExtract to
// FinalizerOnly. Other missing block kinds retain the default.
func TestFallbackTargetForViolation_BlockCoverageSubClassification(t *testing.T) {
	cases := []struct {
		name   string
		v      types.Violation
		want   FallbackTarget
	}{
		{
			name: "missing diagram → finalizer_only (override)",
			v: types.Violation{
				Kind:   types.ViolBlockCoverageMissing,
				Detail: `required block kind=diagram appears 0 time(s) in answer; the family contract requires at least 1`,
			},
			want: FallbackFinalizerOnly,
		},
		{
			name: "missing table → finalizer_only (override)",
			v: types.Violation{
				Kind:   types.ViolBlockCoverageMissing,
				Detail: `required block kind=table appears 0 time(s); the family requires at least 1`,
			},
			want: FallbackFinalizerOnly,
		},
		{
			name: "missing scalar → back_to_extract (default)",
			v: types.Violation{
				Kind:   types.ViolBlockCoverageMissing,
				Detail: `required block kind=scalar appears 0 time(s); the family requires at least 1`,
			},
			want: FallbackBackToExtract,
		},
		{
			name: "scalar over-cap → back_to_extract (default; not 'appears 0')",
			v: types.Violation{
				Kind:   types.ViolBlockCoverageMissing,
				Detail: `required block kind=scalar appears 2 time(s); the family contract caps it at 1`,
			},
			want: FallbackBackToExtract,
		},
		{
			name: "missing ordered_list → back_to_extract (default; not visual)",
			v: types.Violation{
				Kind:   types.ViolBlockCoverageMissing,
				Detail: `required block kind=ordered_list appears 0 time(s)`,
			},
			want: FallbackBackToExtract,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FallbackTargetForViolation(tc.v); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// TestLocusOfTarget_AllTargetsMapped pins the FallbackTarget →
// RepairLocus mapping completeness invariant.
func TestLocusOfTarget_AllTargetsMapped(t *testing.T) {
	for _, tt := range []FallbackTarget{
		FallbackFinalizerOnly,
		FallbackBackToExtract,
		FallbackBackToExplore,
		FallbackBackToAnalyze,
		FallbackFailLoud,
	} {
		l := LocusOfTarget(tt)
		if l == "" {
			t.Errorf("FallbackTarget %q has no RepairLocus mapping", tt)
		}
	}
}

// ── FallbackTarget IsValid ────────────────────────────────────────

func TestFallbackTarget_IsValid(t *testing.T) {
	for _, t1 := range []FallbackTarget{
		FallbackFinalizerOnly,
		FallbackBackToExtract,
		FallbackBackToExplore,
		FallbackBackToAnalyze,
		FallbackFailLoud,
	} {
		if !t1.IsValid() {
			t.Errorf("%q must be valid", t1)
		}
	}
	if FallbackTarget("garbage").IsValid() {
		t.Error("unknown target must NOT be valid")
	}
}

// ── SetFallbackPolicyOverrides ────────────────────────────────────

func TestSetFallbackPolicyOverrides_ApplyAndRestore(t *testing.T) {
	t.Cleanup(func() { SetFallbackPolicyOverrides(nil) })
	// Default: ViolFamilyMismatch → FinalizerOnly
	if got := FallbackTargetForKind(types.ViolFamilyMismatch); got != FallbackFinalizerOnly {
		t.Fatalf("default ViolFamilyMismatch should map to FinalizerOnly; got %s", got)
	}
	// Override
	SetFallbackPolicyOverrides(map[string]string{
		string(types.ViolFamilyMismatch): string(FallbackBackToExplore),
	})
	if got := FallbackTargetForKind(types.ViolFamilyMismatch); got != FallbackBackToExplore {
		t.Fatalf("override should map ViolFamilyMismatch to BackToExplore; got %s", got)
	}
	// Restore by passing nil
	SetFallbackPolicyOverrides(nil)
	if got := FallbackTargetForKind(types.ViolFamilyMismatch); got != FallbackFinalizerOnly {
		t.Errorf("nil override should restore defaults; got %s", got)
	}
}

func TestSetFallbackPolicyOverrides_RejectsBackToAnalyze(t *testing.T) {
	t.Cleanup(func() { SetFallbackPolicyOverrides(nil) })
	// Operator tries to route ViolFamilyMismatch → BackToAnalyze. The override
	// is REJECTED per design red line and silently degrades to
	// FinalizerOnly (with a WARN log).
	SetFallbackPolicyOverrides(map[string]string{
		string(types.ViolFamilyMismatch): string(FallbackBackToAnalyze),
	})
	got := FallbackTargetForKind(types.ViolFamilyMismatch)
	if got != FallbackFinalizerOnly {
		t.Errorf("BackToAnalyze override should be rejected → FinalizerOnly; got %s", got)
	}
}

func TestSetFallbackPolicyOverrides_BackToAnalyzeEnumStillValid(t *testing.T) {
	// The enum value remains valid (no caller deletes it from
	// FallbackTarget.IsValid). The reject only applies at the
	// override-application layer.
	if !FallbackBackToAnalyze.IsValid() {
		t.Error("FallbackBackToAnalyze must stay enum-valid (operator-facing introspection)")
	}
}

func TestSetFallbackPolicyOverrides_DropsUnknownTargets(t *testing.T) {
	t.Cleanup(func() { SetFallbackPolicyOverrides(nil) })
	SetFallbackPolicyOverrides(map[string]string{
		string(types.ViolFamilyMismatch): "garbage_target", // invalid
		string(types.ViolCitation): string(FallbackFinalizerOnly), // valid
	})
	// Invalid → kept as default (FinalizerOnly per the default map).
	if got := FallbackTargetForKind(types.ViolFamilyMismatch); got != FallbackFinalizerOnly {
		t.Errorf("invalid target should fall back to default; got %s", got)
	}
	// Valid override applied.
	if got := FallbackTargetForKind(types.ViolCitation); got != FallbackFinalizerOnly {
		t.Errorf("valid override should apply; got %s", got)
	}
}

// ── PolicySnapshot ────────────────────────────────────────────────

func TestPolicySnapshot_Sorted(t *testing.T) {
	snap := PolicySnapshot()
	if len(snap) == 0 {
		t.Fatal("snapshot must not be empty")
	}
	for i := 1; i < len(snap); i++ {
		if string(snap[i-1].Kind) > string(snap[i].Kind) {
			t.Errorf("snapshot must be sorted by kind; %q > %q at index %d",
				snap[i-1].Kind, snap[i].Kind, i-1)
		}
	}
}

// ── FallbackTargetForKind unknown ────────────────────────────────

func TestFallbackTargetForKind_UnknownDefaultsFinalizerOnly(t *testing.T) {
	if got := FallbackTargetForKind("never_emitted_kind"); got != FallbackFinalizerOnly {
		t.Errorf("unknown kind default = %s, want %s", got, FallbackFinalizerOnly)
	}
}

// ── R2.2 finalize-local priority tests (post_shape_residual_audit
// 2026-05-04) ──────────────────────────────────────────────────────

// TestFallbackTargetForViolationsWithBudget_DowngradesWhenFinalizerLocalCoexists
// pins the R2.2 contract: when primary locus would deeper-pull AND
// the violation set has finalize-local violations AND budget is
// available, the picker downgrades to FinalizerOnly.
func TestFallbackTargetForViolationsWithBudget_DowngradesWhenFinalizerLocalCoexists(t *testing.T) {
	// Violations: 1 BackToExtract (DeclaredCountDrift) + 1
	// FinalizerOnly (DiagramIdentifier). Pre-R2.2 → tied 1-1 →
	// deepest tiebreak picks BackToExtract.
	vs := []types.Violation{
		{Kind: types.ViolDeclaredCountDrift},
		{Kind: types.ViolDiagramIdentifier},
	}

	// Budget=2, used=0 → downgrade fires.
	if got := FallbackTargetForViolationsWithBudget(vs, 0); got != FallbackFinalizerOnly {
		t.Errorf("budget=2 used=0 with finalize-local coexisting: got %s, want finalizer_only", got)
	}
	// Budget=2, used=1 → still has budget → downgrade fires.
	if got := FallbackTargetForViolationsWithBudget(vs, 1); got != FallbackFinalizerOnly {
		t.Errorf("budget=2 used=1: got %s, want finalizer_only", got)
	}
	// Budget=2, used=2 → exhausted → escalate to primary pick.
	if got := FallbackTargetForViolationsWithBudget(vs, 2); got != FallbackBackToExtract {
		t.Errorf("budget=2 used=2 (exhausted): got %s, want back_to_extract", got)
	}
}

// TestFallbackTargetForViolationsWithBudget_NoFinalizerLocalNoDowngrade
// confirms the downgrade requires finalize-local violations to
// exist; pure deeper violations escalate as before.
func TestFallbackTargetForViolationsWithBudget_NoFinalizerLocalNoDowngrade(t *testing.T) {
	vs := []types.Violation{
		{Kind: types.ViolDeclaredCountDrift},
		{Kind: types.ViolDeclaredCountDrift},
	}
	if got := FallbackTargetForViolationsWithBudget(vs, 0); got != FallbackBackToExtract {
		t.Errorf("no finalize-local in set: got %s, want back_to_extract", got)
	}
}

// TestFallbackTargetForViolationsWithBudget_SoftFailLoudDoesNotShortCircuit
// pins Phase 1-C: SOFT FailLoud-mapped kinds (PlanCritic /
// Reflector / RichnessRegression / etc.) NO LONGER short-circuit
// to fail_loud. They are telemetry-only and let the deepest HARD
// cluster drive routing. Pre-Phase-1-C, ViolPlanCritic + any
// SOFT cluster forced fail_loud (V2 runtime eval m1a-2 / u3a-1
// outlier root cause).
func TestFallbackTargetForViolationsWithBudget_SoftFailLoudDoesNotShortCircuit(t *testing.T) {
	vs := []types.Violation{
		{Kind: types.ViolPlanCritic},        // SOFT FailLoud-mapped
		{Kind: types.ViolDiagramIdentifier}, // SOFT FinalizerOnly-mapped
	}
	got := FallbackTargetForViolationsWithBudget(vs, 0)
	if got == FallbackFailLoud {
		t.Errorf("Phase 1-C: SOFT FailLoud kind MUST NOT short-circuit; got fail_loud")
	}
}

// TestFallbackTargetForViolations_BackCompatNoDowngrade pins the
// budget-less wrapper byte-identity contract.
func TestFallbackTargetForViolations_BackCompatNoDowngrade(t *testing.T) {
	vs := []types.Violation{
		{Kind: types.ViolDeclaredCountDrift},
		{Kind: types.ViolDiagramIdentifier},
	}
	if got := FallbackTargetForViolations(vs); got != FallbackBackToExtract {
		t.Errorf("budget-less wrapper must NOT downgrade: got %s, want back_to_extract", got)
	}
}

// TestFinalizerLocalRetryBudget_SetGet covers the setter / getter
// + non-positive reset to default.
func TestFinalizerLocalRetryBudget_SetGet(t *testing.T) {
	t.Cleanup(func() { SetFinalizerLocalRetryBudget(2) })
	SetFinalizerLocalRetryBudget(5)
	if got := FinalizerLocalRetryBudget(); got != 5 {
		t.Errorf("after set 5: got %d", got)
	}
	SetFinalizerLocalRetryBudget(0)
	if got := FinalizerLocalRetryBudget(); got != 2 {
		t.Errorf("non-positive must reset to 2: got %d", got)
	}
	SetFinalizerLocalRetryBudget(-1)
	if got := FinalizerLocalRetryBudget(); got != 2 {
		t.Errorf("negative must reset to 2: got %d", got)
	}
}
