package types

import "testing"

// TestWriteClosure_ResetExceptApplied locks the contract that warm
// retry reuses: per-cycle state is wiped (pendingApplies, fingerprints,
// rejectionCounts) but appliedSet survives so apply_patch idempotency
// stays aligned with disk after a `git reset --hard <best-sha>` rewinds
// the worktree to a state that already contains those files. Pre-fix
// bug: clearForReplan called Reset() which wiped appliedSet, then the
// next plan retry's apply_patch saw applied[path]=false but the file
// existed on disk → "file already exists in worktree" rejection.
func TestWriteClosure_ResetExceptApplied(t *testing.T) {
	c := NewWriteClosure()
	c.MarkApplied("a.go")
	c.MarkApplied("b/c.py")
	c.EnqueuePendingApply(PendingApply{Path: "x.go"})
	c.AppendFingerprint(ApplyVerifyFingerprint{AppliedCount: 2})

	c.ResetExceptApplied()

	if !c.HasApplied("a.go") || !c.HasApplied("b/c.py") {
		t.Errorf("ResetExceptApplied must preserve appliedSet; got %+v", c.AppliedSet())
	}
	if got := c.PendingApplies(); len(got) != 0 {
		t.Errorf("ResetExceptApplied must clear pendingApplies; got %d entries", len(got))
	}
	if got := c.Fingerprints(); len(got) != 0 {
		t.Errorf("ResetExceptApplied must clear fingerprints; got %d entries", len(got))
	}
}

// TestWriteClosure_Reset_ClearsApplied locks the cold-discard
// counterpart: full Reset still wipes appliedSet (the worktree was
// thrown away).
func TestWriteClosure_Reset_ClearsApplied(t *testing.T) {
	c := NewWriteClosure()
	c.MarkApplied("a.go")
	c.Reset()
	if c.HasApplied("a.go") {
		t.Error("Reset must clear appliedSet")
	}
}

func TestWriteClosure_ReplacePendingApplies(t *testing.T) {
	c := NewWriteClosure()
	c.EnqueuePendingApply(PendingApply{Path: "full-a.go"})
	c.EnqueuePendingApply(PendingApply{Path: "full-b.go"})
	c.ReplacePendingApplies([]PendingApply{{Path: "slice-a.go", Origin: "active_slice"}})
	got := c.PendingApplies()
	if len(got) != 1 || got[0].Path != "slice-a.go" || got[0].Origin != "active_slice" {
		t.Fatalf("ReplacePendingApplies = %+v, want active slice queue", got)
	}
	got[0].Path = "mutated"
	again := c.PendingApplies()
	if again[0].Path != "slice-a.go" {
		t.Fatalf("PendingApplies should return a defensive copy, got %+v", again)
	}
}
