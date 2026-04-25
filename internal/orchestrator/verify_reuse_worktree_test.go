package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func writePlanJSON(t *testing.T, path string, plan *types.ChangePlan) {
	t.Helper()
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
}

// /verify <plan-id> regression: the verify pre-hook must swap
// busCtx.RepoRoot to the preserved worktree path so run_tests
// executes against the applied bytes, NOT the main repo.
func TestVerifyPreHook_SwapsRepoRootToReusePath(t *testing.T) {
	preserved := t.TempDir()
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		MainRepoRoot: "/tmp/main",
		RepoRoot:     "/tmp/main", // would be the wrong target
		Mutable:      types.NewMutableState("verify"),
	}
	// Plan must be set so verifyPreHook doesn't try to load from
	// disk (no PlanPath supplied).
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{
		ID:          "plan-verify-reuse",
		TargetPaths: []string{"x.go"},
		Changes:     []types.FileChange{{Path: "x.go", Kind: "modify"}},
	})
	o.SetReuseWorktreePath(preserved)
	if err := verifyPreHook(o); err != nil {
		t.Fatalf("verifyPreHook: %v", err)
	}
	if o.busCtx.RepoRoot != preserved {
		t.Errorf("RepoRoot = %q; want preserved worktree %q", o.busCtx.RepoRoot, preserved)
	}
	// busCtx.WorktreePath MUST stay empty so the outer cleanup
	// defer does not discard the preserved worktree on Run exit.
	if o.busCtx.WorktreePath != "" {
		t.Errorf("WorktreePath should stay empty so cleanup defer is a no-op; got %q", o.busCtx.WorktreePath)
	}
}

// Reuse path that doesn't exist → fail-loud, don't silently fall
// back to the main repo.
func TestVerifyPreHook_RejectsMissingReusePath(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		MainRepoRoot: "/tmp/main",
		RepoRoot:     "/tmp/main",
		Mutable:      types.NewMutableState("verify"),
	}
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{
		ID:      "plan-stale",
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	})
	o.SetReuseWorktreePath("/tmp/this-path-does-not-exist-codrax-test")
	err := verifyPreHook(o)
	if err == nil {
		t.Error("verifyPreHook should fail when reuse path doesn't exist")
	}
}

// Reuse path that's a regular file (not a directory) → fail-loud.
func TestVerifyPreHook_RejectsReusePathThatIsAFile(t *testing.T) {
	dir := t.TempDir()
	bogus := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(bogus, []byte("plain file"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState("verify"),
	}
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{ID: "p", Changes: []types.FileChange{{Path: "x", Kind: "modify"}}})
	o.SetReuseWorktreePath(bogus)
	if err := verifyPreHook(o); err == nil {
		t.Error("verifyPreHook should fail when reuse path is a regular file")
	}
}

// CLI `--mode=verify --plan-file=X` should auto-pick up the worktree
// path from the on-disk plan (no explicit SetReuseWorktreePath call).
// This mirrors REPL `/verify <id>` behaviour without forcing the CLI
// user to discover the worktree path manually.
func TestVerifyPreHook_AutoPicksWorktreeFromPlanFile(t *testing.T) {
	preserved := t.TempDir()
	planDir := t.TempDir()
	planPath := filepath.Join(planDir, "plan.json")
	plan := &types.ChangePlan{
		ID:           "plan-cli-auto",
		Request:      "fix typo",
		Status:       types.PlanStatusApplied,
		WorktreePath: preserved,
		Changes:      []types.FileChange{{Path: "x.go", Kind: "modify"}},
		TargetPaths:  []string{"x.go"},
	}
	writePlanJSON(t, planPath, plan)

	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		MainRepoRoot: "/tmp/main",
		RepoRoot:     "/tmp/main",
		PlanPath:     planPath,
		Mutable:      types.NewMutableState("verify"),
	}
	// Note: no SetReuseWorktreePath call.
	if err := verifyPreHook(o); err != nil {
		t.Fatalf("verifyPreHook: %v", err)
	}
	if o.busCtx.RepoRoot != preserved {
		t.Errorf("RepoRoot should auto-swap to plan.WorktreePath; got %q want %q", o.busCtx.RepoRoot, preserved)
	}
	if o.busCtx.Mutable.ChangePlan() == nil {
		t.Errorf("plan should be loaded onto Mutable")
	}
}

// Empty reuse path → no-op (don't swap, don't fail).
func TestVerifyPreHook_EmptyReusePathIsNoop(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		MainRepoRoot: "/tmp/main",
		RepoRoot:     "/tmp/main",
		Mutable:      types.NewMutableState("verify"),
	}
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{ID: "p", Changes: []types.FileChange{{Path: "x", Kind: "modify"}}})
	if err := verifyPreHook(o); err != nil {
		t.Errorf("empty reuse path should not error; got %v", err)
	}
	if o.busCtx.RepoRoot != "/tmp/main" {
		t.Errorf("RepoRoot should stay /tmp/main; got %q", o.busCtx.RepoRoot)
	}
}
