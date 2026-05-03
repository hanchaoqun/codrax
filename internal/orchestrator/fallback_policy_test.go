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
		types.ViolSelfContradiction:     FallbackBackToExplore,
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
			name: "PlanCritic alone → FailLoud (informational)",
			vs:   []types.Violation{{Kind: types.ViolPlanCritic}},
			want: FallbackFailLoud,
		},
		{
			name: "PlanCritic + Citation → FailLoud (deepest wins)",
			vs: []types.Violation{
				{Kind: types.ViolPlanCritic},
				{Kind: types.ViolCitation},
			},
			want: FallbackFailLoud,
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
