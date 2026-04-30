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
	// In apply stage all three authorization surfaces apply: CLI
	// flag, yaml key, AND the interactive y/N path (because
	// /approve prompts y/N). Each must surface in the message.
	for _, surface := range []string{"--auto-init-repo", "write_auto_init_repo", "/approve"} {
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
		worktreeBase:    wtBase,
		autoInitRepo:    true,
		scaffoldEnabled: true, // empty tmp dir; apply-tier gate now mirrors planPreHook's
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

// TestPlanPreHook_BareDirRefusesWithoutAuth — same shape as
// TestApplyPreHook_BareDirRefusesWithoutAuth but for the new plan-
// stage gate. Pre-2026-04-29 this gate did not exist; plan mode
// against a bare repo dispatched the planner unconditionally and
// burned watchdog stalls (production trace
// /home/chatpp/pytest 2026-04-29 06:03 was the symptom). The gate
// now refuses with the same authorization message applyPreHook uses.
func TestPlanPreHook_BareDirRefusesWithoutAuth(t *testing.T) {
	bare := t.TempDir() // no .git
	o := &Orchestrator{
		busCtx: &types.BusContext{
			MainRepoRoot: bare,
			TraceID:      "trace-plan-bare-noauth",
			Mutable:      types.NewMutableState("plan"),
		},
	}
	err := planPreHook(o)
	if err == nil {
		t.Fatal("expected planPreHook to fail without authorization")
	}
	msg := err.Error()
	// In PLAN stage the y/N interactive path is NOT available (the
	// user is blocked at plan generation, can never reach /approve
	// to see the y/N prompt) — listing it would deadlock the
	// instructions. Only the two structurally-applicable surfaces
	// must appear. The interactive option must NOT.
	for _, surface := range []string{"--auto-init-repo", "write_auto_init_repo"} {
		if !strings.Contains(msg, surface) {
			t.Errorf("error message should name authorization surface %q; got: %s",
				surface, msg)
		}
	}
	if strings.Contains(msg, "/approve") {
		t.Errorf("plan-stage message must NOT mention /approve y/N (deadlock — plan never reaches /approve); got: %s", msg)
	}
	if !o.busCtx.Mutable.ResultIsPlain() {
		t.Error("planPreHook fail-loud must mark result as plain text to avoid glamour fragmentation")
	}
}

// TestPlanPreHook_BareDirAllowsWithAuth — when autoInitRepo=true and
// scaffoldEnabled=true (empty bare dir), the gate logs but does NOT
// init the repo (defers that to applyPreHook). plan stage runs
// against the bare working tree for read-only repo_map / list_files;
// emit_change_plan validators don't need a git index. Returns nil
// so the planner dispatches.
func TestPlanPreHook_BareDirAllowsWithAuth(t *testing.T) {
	bare := t.TempDir()
	o := &Orchestrator{
		autoInitRepo:    true,
		scaffoldEnabled: true,
		busCtx: &types.BusContext{
			MainRepoRoot: bare,
			TraceID:      "trace-plan-bare-auth",
			Mutable:      types.NewMutableState("plan"),
		},
	}
	if err := planPreHook(o); err != nil {
		t.Fatalf("planPreHook should pass with both authorizations: %v", err)
	}
	// Confirm the bare repo was NOT init'd by planPreHook (the
	// .git directory doesn't appear). applyPreHook will init it
	// later when apply actually needs the worktree.
	if _, err := os.Stat(filepath.Join(bare, ".git")); !os.IsNotExist(err) {
		t.Errorf("planPreHook should NOT init the repo; .git exists: %v", err)
	}
}

// TestPlanPreHook_ReadyRepoIsNoop — the gate is silent on a
// healthy git repo with at least one commit.
func TestPlanPreHook_ReadyRepoIsNoop(t *testing.T) {
	repo := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git init unavailable: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-c", "user.email=t@t", "-c", "user.name=t",
		"commit", "--allow-empty", "-m", "init")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git commit unavailable: %v\n%s", err, out)
	}

	o := &Orchestrator{
		busCtx: &types.BusContext{
			MainRepoRoot: repo,
			TraceID:      "trace-plan-ready",
			Mutable:      types.NewMutableState("plan"),
		},
	}
	if err := planPreHook(o); err != nil {
		t.Errorf("planPreHook on ready repo should be no-op; got %v", err)
	}
}
