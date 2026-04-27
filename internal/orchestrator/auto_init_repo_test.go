package orchestrator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestApplyPreHook_BareDirRefusesWithoutAuth covers the failure
// surface when the target is a bare (non-git) directory and the
// orchestrator was NOT pre-authorized via SetAutoInitRepo. The
// pre-hook should refuse with a message naming all three
// authorization paths so the operator can pick one.
func TestApplyPreHook_BareDirRefusesWithoutAuth(t *testing.T) {
	bare := t.TempDir() // no .git
	wtBase := t.TempDir()

	o := &Orchestrator{
		worktreeBase: wtBase,
		busCtx: &types.BusContext{
			MainRepoRoot: bare,
			TraceID:      "trace-bare-noauth",
			Mutable:      types.NewMutableState("apply"),
			PlanPath:     "", // skip the LoadChangePlanFromFile step
		},
		// autoInitRepo NOT set — orchestrator default is false.
	}
	// Seed a minimal ChangePlan so applyPreHook's stage 1 LoadChangePlanFromFile
	// loop is bypassed (PlanPath==""). The hook still runs the
	// repo-state probe + auto-init gate.
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{
		ID:          "plan-noauth-1",
		Changes:     []types.FileChange{{Path: "a.go", Kind: "create"}},
		TargetPaths: []string{"a.go"},
	})

	err := applyPreHook(o)
	if err == nil {
		t.Fatal("expected applyPreHook to fail without authorization")
	}
	msg := err.Error()
	for _, surface := range []string{"--auto-init-repo", "write_auto_init_repo", "REPL"} {
		if !strings.Contains(msg, surface) {
			t.Errorf("error message should name authorization surface %q; got: %s",
				surface, msg)
		}
	}
}

// TestApplyPreHook_BareDirSucceedsWithAuth verifies that with
// autoInitRepo=true the pre-hook runs `git init` + initial commit
// transparently and provisions a worktree. End state: WorktreePath
// is set and points at a real directory inside wtBase.
func TestApplyPreHook_BareDirSucceedsWithAuth(t *testing.T) {
	bare := t.TempDir()
	wtBase := t.TempDir()

	o := &Orchestrator{
		worktreeBase: wtBase,
		autoInitRepo: true,
		busCtx: &types.BusContext{
			MainRepoRoot: bare,
			TraceID:      "trace-bare-auth",
			Mutable:      types.NewMutableState("apply"),
		},
	}
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{
		ID:          "plan-auth-1",
		Changes:     []types.FileChange{{Path: "a.go", Kind: "create"}},
		TargetPaths: []string{"a.go"},
	})

	if err := applyPreHook(o); err != nil {
		t.Fatalf("applyPreHook with auth: %v", err)
	}
	if o.busCtx.WorktreePath == "" {
		t.Fatal("WorktreePath should be set after successful provisioning")
	}
	if _, err := os.Stat(o.busCtx.WorktreePath); err != nil {
		t.Errorf("worktree dir should exist on disk: %v", err)
	}
	// The auto-init transitioned the bare dir → ready repo. Verify
	// .git now exists in the main repo (the worktree itself shares
	// the parent's .git but lives under wtBase).
	if _, err := os.Stat(filepath.Join(bare, ".git")); err != nil {
		t.Errorf("main repo .git should exist after auto-init: %v", err)
	}
	t.Cleanup(func() {
		// Clean up the worktree we provisioned so the temp dir
		// can be removed.
		_ = os.RemoveAll(o.busCtx.WorktreePath)
	})
}

// TestApplyPreHook_ReadyRepoIgnoresAuthFlag verifies the auto-init
// gate is a no-op for a healthy repo: the flag value doesn't matter,
// EnsureInitialCommit is never called, and provisioning proceeds
// normally. This locks the idempotency contract.
func TestApplyPreHook_ReadyRepoIgnoresAuthFlag(t *testing.T) {
	main := t.TempDir()
	mustGitTopLevel(t, main, "init")
	mustGitTopLevel(t, main, "config", "user.email", "x@y")
	mustGitTopLevel(t, main, "config", "user.name", "x")
	mustGitTopLevel(t, main, "commit", "--allow-empty", "-m", "init")

	wtBase := t.TempDir()
	o := &Orchestrator{
		worktreeBase: wtBase,
		// autoInitRepo deliberately false — should not matter.
		autoInitRepo: false,
		busCtx: &types.BusContext{
			MainRepoRoot: main,
			TraceID:      "trace-ready",
			Mutable:      types.NewMutableState("apply"),
		},
	}
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{
		ID:          "plan-ready-1",
		Changes:     []types.FileChange{{Path: "a.go", Kind: "create"}},
		TargetPaths: []string{"a.go"},
	})

	if err := applyPreHook(o); err != nil {
		t.Fatalf("applyPreHook on ready repo: %v", err)
	}
	if o.busCtx.WorktreePath == "" {
		t.Fatal("WorktreePath should be set on ready repo")
	}
	t.Cleanup(func() { _ = os.RemoveAll(o.busCtx.WorktreePath) })
}

// mustGitTopLevel runs a git command in dir for orchestrator-test
// fixture setup. Distinct from the worktree package's helper to
// avoid cross-package symbol leakage.
func mustGitTopLevel(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
