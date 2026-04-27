package types

import (
	"testing"
	"time"
)

// TestApplyVerifyFingerprint_SameSignal locks the contract used by
// the verify→plan retry stall detector: SameSignal compares every
// progress field, ignoring Timestamp. Two rounds with identical
// applied / pass / fail counts AND identical FailureSummaryHash
// represent zero progress; differ in any field and we keep
// retrying.
func TestApplyVerifyFingerprint_SameSignal(t *testing.T) {
	base := ApplyVerifyFingerprint{
		AppliedCount:       2,
		VerifyPassed:       3,
		VerifyFailed:       1,
		FailureSummaryHash: "deadbeef",
		Timestamp:          time.Unix(100, 0),
	}
	identical := base
	identical.Timestamp = time.Unix(999, 0) // timestamp must NOT matter
	if !base.SameSignal(identical) {
		t.Errorf("identical signal (different timestamps) should match")
	}
	for _, mut := range []func(*ApplyVerifyFingerprint){
		func(f *ApplyVerifyFingerprint) { f.AppliedCount++ },
		func(f *ApplyVerifyFingerprint) { f.VerifyPassed++ },
		func(f *ApplyVerifyFingerprint) { f.VerifyFailed++ },
		func(f *ApplyVerifyFingerprint) { f.FailureSummaryHash += "x" },
	} {
		modified := base
		mut(&modified)
		if base.SameSignal(modified) {
			t.Errorf("differing signal should NOT match: %+v vs %+v", base, modified)
		}
	}
}

// TestWriteClosure_RecordRejection_Threshold locks the saturation
// behaviour: 3 consecutive rejections of the same path triggers
// SaturatedRejectionPath. 2 don't. A different path saturating
// independently shouldn't bleed across paths.
func TestWriteClosure_RecordRejection_Threshold(t *testing.T) {
	c := NewWriteClosure()

	c.RecordRejection("a.go")
	c.RecordRejection("a.go")
	if got := c.SaturatedRejectionPath(); got != "" {
		t.Errorf("2 rejections should NOT saturate; got %q", got)
	}
	c.RecordRejection("a.go")
	if got := c.SaturatedRejectionPath(); got != "a.go" {
		t.Errorf("3 rejections should saturate; got %q", got)
	}
	if got := c.RejectionCount("a.go"); got != 3 {
		t.Errorf("RejectionCount = %d, want 3", got)
	}
	if got := c.RejectionCount("b.go"); got != 0 {
		t.Errorf("RejectionCount(b.go) = %d, want 0 (never rejected)", got)
	}
}

// TestWriteClosure_MarkApplied_ClearsRejectionCounters verifies
// forward progress wipes the slate. The contract: once ANY apply
// succeeds, no path stays "saturated" — the LLM showed it can make
// progress, so the rejection state from earlier in the loop is no
// longer evidence of fixation.
func TestWriteClosure_MarkApplied_ClearsRejectionCounters(t *testing.T) {
	c := NewWriteClosure()
	c.RecordRejection("a.go")
	c.RecordRejection("a.go")
	c.RecordRejection("a.go")
	if c.SaturatedRejectionPath() != "a.go" {
		t.Fatal("setup: a.go should be saturated")
	}
	// Apply some OTHER path. Forward progress means the rejection
	// counters reset across the board.
	c.MarkApplied("b.go")
	if got := c.SaturatedRejectionPath(); got != "" {
		t.Errorf("MarkApplied should clear ALL rejection counts; SaturatedRejectionPath = %q", got)
	}
	if got := c.RejectionCount("a.go"); got != 0 {
		t.Errorf("RejectionCount(a.go) after MarkApplied = %d, want 0", got)
	}
}

// TestWriteClosure_Reset_ClearsRejectionCounters verifies that the
// per-task Reset wipes rejection state too — otherwise a counter
// from task 1 could leak into task 2's coder loop and trip
// SaturatedRejectionPath spuriously.
func TestWriteClosure_Reset_ClearsRejectionCounters(t *testing.T) {
	c := NewWriteClosure()
	for i := 0; i < 5; i++ {
		c.RecordRejection("x.go")
	}
	c.Reset()
	if got := c.SaturatedRejectionPath(); got != "" {
		t.Errorf("Reset should clear rejection counts; got %q", got)
	}
}

// TestWriteClosure_RecordRejection_EmptyPathIsNoop defends against
// caller bugs that pass an empty path: the counter shouldn't grow,
// and SaturatedRejectionPath shouldn't ever return "".
func TestWriteClosure_RecordRejection_EmptyPathIsNoop(t *testing.T) {
	c := NewWriteClosure()
	for i := 0; i < 5; i++ {
		c.RecordRejection("")
		c.RecordRejection("   ")
	}
	if got := c.SaturatedRejectionPath(); got != "" {
		t.Errorf("empty-path rejections should not saturate; got %q", got)
	}
}
