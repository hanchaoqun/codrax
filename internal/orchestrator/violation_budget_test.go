package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Session 11 G4 — F5 ViolationBudget + C6 per-kind retry diff.
// Unit tests run the pure helpers in isolation (yieldSnapshot,
// dominantViolationKind, prependFailLoudWarning) to lock the
// behaviour; the full end-to-end retry integration is covered by
// the bug-replay harness once G2 lands.

func TestYieldDelta_ComputesAcrossFourAxes(t *testing.T) {
	prev := yieldSnapshot{ForcedReads: 2, ScannedSetSize: 10, DistinctViolationKindCount: 4, PatchesApplied: 0}
	now := yieldSnapshot{ForcedReads: 3, ScannedSetSize: 12, DistinctViolationKindCount: 7, PatchesApplied: 1}
	d := yieldDelta(prev, now)
	// 1 (ForcedReads) + 2 (ScannedSet) + 3 (DistinctKinds) + 1 (Patches) = 7
	if d != 7 {
		t.Errorf("yieldDelta = %d, want 7 (sum of four axis increments)", d)
	}
}

func TestYieldDelta_ZeroWhenNothingChanges(t *testing.T) {
	snap := yieldSnapshot{ForcedReads: 5, ScannedSetSize: 20, DistinctViolationKindCount: 10}
	if d := yieldDelta(snap, snap); d != 0 {
		t.Errorf("yieldDelta = %d, want 0 (no-op window)", d)
	}
}

// TestYieldDelta_SameKindRefireYieldsZero pins the A.2 audit fix
// (2026-05-02): the yield-kill gate must fire when the same
// ViolationKind keeps re-firing across windows. Pre-A.2, the
// ViolationsLogged scalar grew on every Append, so re-firing a single
// kind defeated MinRetryYield=1. After A.2, DistinctViolationKindCount
// stays flat and the kill gate sees zero progress.
func TestYieldDelta_SameKindRefireYieldsZero(t *testing.T) {
	prev := yieldSnapshot{DistinctViolationKindCount: 1}
	now := yieldSnapshot{DistinctViolationKindCount: 1} // same kind re-fired N times: still 1 distinct
	if d := yieldDelta(prev, now); d != 0 {
		t.Errorf("yieldDelta = %d, want 0 (same kind re-firing must yield zero)", d)
	}
}

func TestYieldDelta_NeverNegative(t *testing.T) {
	// Should a counter ever decrease (shouldn't in practice, but
	// defensive), the delta must stay zero for that axis — not
	// go negative.
	prev := yieldSnapshot{ForcedReads: 5}
	now := yieldSnapshot{ForcedReads: 3}
	if d := yieldDelta(prev, now); d != 0 {
		t.Errorf("yieldDelta = %d, want 0 (decreases must not produce negatives)", d)
	}
}

func TestCaptureYieldSnapshot_NilClosureSafe(t *testing.T) {
	s := captureYieldSnapshot(nil)
	if s != (yieldSnapshot{}) {
		t.Errorf("nil closure should produce zero snapshot, got %+v", s)
	}
}

func TestDominantViolationKind_PicksMostCommon(t *testing.T) {
	res := contract.Result{
		Violations: []types.Violation{
			{Kind: types.ViolFamilyMismatch},
			{Kind: types.ViolFamilyMismatch},
			{Kind: types.ViolCitation},
		},
	}
	if got := dominantViolationKind(res); got != types.ViolFamilyMismatch {
		t.Errorf("dominantViolationKind = %s, want ViolFamilyMismatch", got)
	}
}

func TestDominantViolationKind_EmptyReturnsEmpty(t *testing.T) {
	if got := dominantViolationKind(contract.Result{Passed: true}); got != "" {
		t.Errorf("passed result should return empty kind, got %s", got)
	}
	if got := dominantViolationKind(contract.Result{}); got != "" {
		t.Errorf("empty violations should return empty kind, got %s", got)
	}
}

func TestRetryBudgetByKind_UsesKindSpecificCap(t *testing.T) {
	// Default Session-11 defaults table (see DefaultRetryBudgetByKindSettings).
	def := types.DefaultRetryBudgetByKindSettings()
	if got := def.For(types.ViolFamilyMismatch, 99); got != 1 {
		t.Errorf("ViolFamilyMismatch cap = %d, want 1 (default shape budget)", got)
	}
	if got := def.For(types.ViolCitation, 99); got != 1 {
		t.Errorf("ViolCitation cap = %d, want 1", got)
	}
	if got := def.For(types.ViolGhostAnchor, 99); got != 2 {
		t.Errorf("ViolGhostAnchor cap = %d, want 2", got)
	}
	if got := def.For(types.ViolSuccessCriterion, 99); got != 1 {
		t.Errorf("unknown-kind (SC) cap = %d, want 1 (Other default)", got)
	}
}

func TestRetryBudgetByKind_ZeroFallsBackToFallback(t *testing.T) {
	r := types.RetryBudgetByKindSettings{} // all zero
	if got := r.For(types.ViolFamilyMismatch, 5); got != 5 {
		t.Errorf("zero-entry cap should fall back to fallback=5, got %d", got)
	}
}

func TestGraphState_PerKindCounterIsolated(t *testing.T) {
	s := &graphState{}
	s.recordRetryByKind(types.ViolFamilyMismatch)
	s.recordRetryByKind(types.ViolFamilyMismatch)
	s.recordRetryByKind(types.ViolCitation)
	if got := s.retryUsedForKind(types.ViolFamilyMismatch); got != 2 {
		t.Errorf("shape retries = %d, want 2", got)
	}
	if got := s.retryUsedForKind(types.ViolCitation); got != 1 {
		t.Errorf("citation retries = %d, want 1", got)
	}
	if got := s.retryUsedForKind(types.ViolGhostAnchor); got != 0 {
		t.Errorf("untouched kind should return 0, got %d", got)
	}
}

func TestPrependFailLoudWarning_EmitsHeaderWithFieldAndYieldKills(t *testing.T) {
	mut := types.NewMutableState("test")
	closure := mut.EvidenceClosure()
	// Populate a ledger entry so TopSuspectedField can surface a field.
	closure.AppendViolation(types.Violation{
		Kind: types.ViolFamilyMismatch,
		SuspectedRoot: types.SuspectedRoot{
			IRField: "question_kind", Confidence: 0.85,
		},
	})
	state := &graphState{yieldKillCount: 2}
	settings := types.PipelineSettings{
		ViolationBudget: types.DefaultViolationBudgetSettings(),
	}

	out := prependFailLoudWarning("real answer body\n", mut, state, "yield kill", settings)
	if !strings.Contains(out, "· Pipeline terminated") {
		t.Errorf("missing fail-loud header: %q", out)
	}
	if !strings.Contains(out, "yield kill") {
		t.Errorf("trigger missing: %q", out)
	}
	if !strings.Contains(out, "question_kind") {
		t.Errorf("top suspected field missing: %q", out)
	}
	if !strings.Contains(out, "2 yield kill(s)") {
		t.Errorf("yield kill count missing: %q", out)
	}
	// Original body preserved beneath the warning.
	if !strings.Contains(out, "real answer body") {
		t.Errorf("original answer body dropped: %q", out)
	}
}

func TestPrependFailLoudWarning_DisabledPassesThrough(t *testing.T) {
	settings := types.PipelineSettings{
		ViolationBudget: types.ViolationBudgetSettings{FailLoudEnabled: false},
	}
	out := prependFailLoudWarning("body", nil, nil, "x", settings)
	if out != "body" {
		t.Errorf("FailLoudEnabled=false should pass through, got %q", out)
	}
}
