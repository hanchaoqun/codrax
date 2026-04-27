package orchestrator

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestVerifyStallReason_TwoIdenticalRoundsDetected locks the core
// detector contract: two consecutive verify rounds with identical
// signal (applied count + pass/fail counts + failure summary hash)
// trigger retry suppression. The reason string is non-empty and
// names the matching dimensions.
func TestVerifyStallReason_TwoIdenticalRoundsDetected(t *testing.T) {
	wc := types.NewWriteClosure()
	fp := types.ApplyVerifyFingerprint{
		AppliedCount:       1,
		VerifyPassed:       0,
		VerifyFailed:       3,
		FailureSummaryHash: "abc123",
		Timestamp:          time.Unix(100, 0),
	}
	wc.AppendFingerprint(fp)
	fp.Timestamp = time.Unix(200, 0) // timestamp differs but signal same
	wc.AppendFingerprint(fp)

	reason := verifyStallReason(wc)
	if reason == "" {
		t.Errorf("two identical rounds should trigger stall; reason was empty")
	}
}

// TestVerifyStallReason_DifferentRoundsAllowed verifies normal
// progress doesn't trip the guard. Only EXACT same-signal adjacency
// triggers; any meaningful change (more applied, fewer failures,
// new failure shape) means the planner is still iterating.
func TestVerifyStallReason_DifferentRoundsAllowed(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(*types.ApplyVerifyFingerprint)
	}{
		{"applied count grew", func(f *types.ApplyVerifyFingerprint) { f.AppliedCount++ }},
		{"more passing tests", func(f *types.ApplyVerifyFingerprint) { f.VerifyPassed++; f.VerifyFailed-- }},
		{"different failure shape", func(f *types.ApplyVerifyFingerprint) { f.FailureSummaryHash = "different" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wc := types.NewWriteClosure()
			base := types.ApplyVerifyFingerprint{
				AppliedCount: 1, VerifyPassed: 0, VerifyFailed: 3,
				FailureSummaryHash: "abc123", Timestamp: time.Unix(100, 0),
			}
			wc.AppendFingerprint(base)
			next := base
			tc.mut(&next)
			next.Timestamp = time.Unix(200, 0)
			wc.AppendFingerprint(next)

			if reason := verifyStallReason(wc); reason != "" {
				t.Errorf("progress in dimension %q should NOT trip stall; got reason: %q",
					tc.name, reason)
			}
		})
	}
}

// TestVerifyStallReason_SingleRoundNeverStalls covers the cold-start
// case: only one fingerprint exists. There's no adjacency to compare
// → no stall. Without this guard a Run with retry budget=0 would
// erroneously seem "stalled" on its first verify.
func TestVerifyStallReason_SingleRoundNeverStalls(t *testing.T) {
	wc := types.NewWriteClosure()
	wc.AppendFingerprint(types.ApplyVerifyFingerprint{
		VerifyFailed:       3,
		FailureSummaryHash: "abc",
	})
	if reason := verifyStallReason(wc); reason != "" {
		t.Errorf("single fingerprint should not stall; got: %q", reason)
	}
}

// TestVerifyStallReason_NilClosureSafe defends against nil — the
// caller (write_scheduler retry decision) reads through Mutable
// which can be nil in some test fixtures and edge paths.
func TestVerifyStallReason_NilClosureSafe(t *testing.T) {
	if reason := verifyStallReason(nil); reason != "" {
		t.Errorf("nil closure should be no-op; got: %q", reason)
	}
}
