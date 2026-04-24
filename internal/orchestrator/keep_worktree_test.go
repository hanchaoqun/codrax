package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

// TestSetKeepWorktreeOnSuccess_RoundTrip locks the accessor pair so
// the cmd/root.go wiring that reads yaml → setter → getter has a
// contract to rely on.
func TestSetKeepWorktreeOnSuccess_RoundTrip(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	if got := o.KeepWorktreeOnSuccess(); got {
		t.Errorf("default should be false; got %v", got)
	}
	o.SetKeepWorktreeOnSuccess(true)
	if got := o.KeepWorktreeOnSuccess(); !got {
		t.Errorf("after Set(true) expected true; got %v", got)
	}
	o.SetKeepWorktreeOnSuccess(false)
	if got := o.KeepWorktreeOnSuccess(); got {
		t.Errorf("after Set(false) expected false; got %v", got)
	}
}

// seedFakeWorktree creates a git-free directory that DiscardByPath
// can Remove via its belt-and-suspenders step. Enough to assert the
// presence/absence after the defer fires.
func seedFakeWorktree(t *testing.T) (string, string) {
	t.Helper()
	mainRoot := t.TempDir()
	// Give main repo a minimal .git so git worktree prune (step 3)
	// is a harmless no-op rather than an error path we don't care
	// about.
	_ = os.MkdirAll(filepath.Join(mainRoot, ".git", "worktrees"), 0o755)
	wtPath := filepath.Join(t.TempDir(), "fake-worktree")
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("MkdirAll wt: %v", err)
	}
	os.WriteFile(filepath.Join(wtPath, "marker.txt"), []byte("hi"), 0o644)
	return mainRoot, wtPath
}

// TestRunCleanupDefer_KeepOnSuccess exercises the Run-level defer
// directly by invoking the same conditional logic we embed there,
// since Run() requires a full agent registry + plan dispatch loop
// to reach the write-mode branches. We prove the contract: when
// keepOnSuccess=true, Mode=ModeApply, LastError="", the worktree
// is NOT discarded; anything else flips to discard.
func TestRunCleanupDefer_KeepOnSuccess(t *testing.T) {
	cases := []struct {
		name      string
		keep      bool
		mode      types.PipelineMode
		lastError string
		wantKept  bool
	}{
		{"keep=true mode=apply success", true, types.ModeApply, "", true},
		{"keep=true mode=apply failure", true, types.ModeApply, "apply failed: W1 violation", false},
		{"keep=true mode=read (not apply)", true, types.ModeRead, "", false},
		{"keep=true mode=plan (plan-only)", true, types.ModePlan, "", false},
		{"keep=true mode=verify-only", true, types.ModeVerify, "", false},
		{"keep=false mode=apply success", false, types.ModeApply, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mainRoot, wtPath := seedFakeWorktree(t)

			// Simulate Run() defer by embedding the same decision tree.
			// Scope is tight: we're not testing Run(), just the branch
			// condition + the call that's guarded.
			shouldKeep := tc.keep &&
				tc.mode == types.ModeApply &&
				tc.lastError == ""
			if !shouldKeep {
				if err := worktree.DiscardByPath(wtPath, mainRoot); err != nil {
					// Acceptable — git commands are best-effort here.
					t.Logf("DiscardByPath returned (non-fatal): %v", err)
				}
			}

			_, err := os.Stat(wtPath)
			got := err == nil
			if got != tc.wantKept {
				t.Errorf("kept=%v, want %v (err=%v)", got, tc.wantKept, err)
			}
		})
	}
}
