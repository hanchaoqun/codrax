package repl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// seedWorktreeFile creates a file at the given relative path
// inside dir to simulate a partial-apply state.
func seedWorktreeFile(t *testing.T, dir, relPath string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// TestApproveRetryProgress_PartialState pins commit 13 #3:
// when the worktree has SOME of the plan's TargetPaths, the
// counts split correctly between applied / remaining.
func TestApproveRetryProgress_PartialState(t *testing.T) {
	wt := t.TempDir()
	seedWorktreeFile(t, wt, "a.go")
	seedWorktreeFile(t, wt, "b.go")
	plan := &types.ChangePlan{
		ID:           "plan-x",
		WorktreePath: wt,
		TargetPaths:  []string{"a.go", "b.go", "c.go", "d.go"},
	}
	applied, remaining := approveRetryProgress(plan)
	if applied != 2 || remaining != 2 {
		t.Errorf("applied=%d remaining=%d; want 2/2", applied, remaining)
	}
	pending := approveRetryPendingPaths(plan)
	if len(pending) != 2 {
		t.Errorf("expected 2 pending paths; got %v", pending)
	}
	for _, p := range pending {
		if p != "c.go" && p != "d.go" {
			t.Errorf("unexpected pending path %q (want c.go or d.go)", p)
		}
	}
}

// TestApproveRetryProgress_AllApplied pins the "everything's
// already there" path: 0 remaining when every TargetPath exists.
func TestApproveRetryProgress_AllApplied(t *testing.T) {
	wt := t.TempDir()
	seedWorktreeFile(t, wt, "a.go")
	plan := &types.ChangePlan{
		WorktreePath: wt,
		TargetPaths:  []string{"a.go"},
	}
	applied, remaining := approveRetryProgress(plan)
	if applied != 1 || remaining != 0 {
		t.Errorf("applied=%d remaining=%d; want 1/0", applied, remaining)
	}
	if got := approveRetryPendingPaths(plan); len(got) != 0 {
		t.Errorf("expected 0 pending; got %v", got)
	}
}

// TestApproveRetryProgress_NoWorktree falls back to "all
// remaining" when the plan has no preserved worktree path.
func TestApproveRetryProgress_NoWorktree(t *testing.T) {
	plan := &types.ChangePlan{TargetPaths: []string{"a.go", "b.go"}}
	applied, remaining := approveRetryProgress(plan)
	if applied != 0 || remaining != 2 {
		t.Errorf("applied=%d remaining=%d; want 0/2", applied, remaining)
	}
	if got := approveRetryPendingPaths(plan); got != nil {
		t.Errorf("expected nil pending without worktree; got %v", got)
	}
}

// TestApproveRetryPendingPaths_OverflowCap pins the cap+overflow
// rendering for plans with many remaining paths.
func TestApproveRetryPendingPaths_OverflowCap(t *testing.T) {
	wt := t.TempDir()
	plan := &types.ChangePlan{
		WorktreePath: wt,
		TargetPaths: []string{
			"a.go", "b.go", "c.go", "d.go", "e.go",
			"f.go", "g.go", "h.go",
		},
	}
	pending := approveRetryPendingPaths(plan)
	// cap = 5, overflow = 3 → 6 entries (5 paths + "(+3 more)").
	if len(pending) != 6 {
		t.Errorf("expected 6 entries (5 + overflow); got %d (%v)", len(pending), pending)
	}
	if pending[len(pending)-1] != "(+3 more)" {
		t.Errorf("last entry should be '(+3 more)'; got %q", pending[len(pending)-1])
	}
}
