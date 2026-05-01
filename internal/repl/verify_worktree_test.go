package repl

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/types"
)

// newVerifyREPL wires a minimal REPL with PlanStore + mem store so
// /verify and /worktree handlers have something to work against.
func newVerifyREPL(t *testing.T, runner Runner) (*REPL, *PlanStore, *bytes.Buffer) {
	t.Helper()
	store := NewPlanStore(t.TempDir())
	memStore, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	t.Cleanup(func() { memStore.Close() })
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:    runner,
		Store:     memStore,
		In:        strings.NewReader(""),
		Out:       out,
		RepoRoot:  t.TempDir(), // separate dir, no git init needed
		Branch:    "main",
		Render:    renderNothing,
		PlanStore: store,
	})
	return r, store, out
}

// ── /verify handler tests ─────────────────────────────────────────

func TestVerify_DisabledWhenNoStore(t *testing.T) {
	r, out := newScriptedREPL(t, nil)
	r.handleVerifyCmd("/verify")
	if !strings.Contains(out.String(), "/verify disabled") {
		t.Errorf("expected disabled message; got %q", out.String())
	}
}

func TestVerify_NoPendingPlan(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	r, out := newScriptedREPL(t, store)
	r.handleVerifyCmd("/verify")
	if !strings.Contains(out.String(), "no pending plan") {
		t.Errorf("expected no-pending message; got %q", out.String())
	}
}

func TestVerify_WrongStatusRefused(t *testing.T) {
	cases := []struct {
		status string
	}{
		{types.PlanStatusPending},
		{types.PlanStatusApplyFailed},
		{types.PlanStatusRejected},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			runner := &writeCapableRunner{}
			r, store, out := newVerifyREPL(t, runner)
			plan := &types.ChangePlan{
				ID:      "plan-vs-" + tc.status,
				Status:  tc.status,
				Changes: []types.FileChange{{Path: "a.go", Kind: "modify"}},
			}
			path, err := store.SaveForTest(plan)
			if err != nil {
				t.Fatalf("Save: %v", err)
			}
			r.pendingPlanPath = path
			r.handleVerifyCmd("/verify")
			if runner.runCalled {
				t.Errorf("/verify should refuse for Status=%q", tc.status)
			}
			if !strings.Contains(out.String(), "applied or verify_failed") {
				t.Errorf("output should explain the status requirement; got %q", out.String())
			}
		})
	}
}

func TestVerify_NoWorktreePathRefused(t *testing.T) {
	runner := &writeCapableRunner{}
	r, store, out := newVerifyREPL(t, runner)
	plan := &types.ChangePlan{
		ID:      "plan-no-wt",
		Status:  types.PlanStatusApplied,
		Changes: []types.FileChange{{Path: "a.go", Kind: "modify"}},
		// WorktreePath deliberately empty — Fix 4 wasn't enabled
		// when this plan was applied.
	}
	path, err := store.SaveForTest(plan)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	r.pendingPlanPath = path
	r.handleVerifyCmd("/verify")
	if runner.runCalled {
		t.Error("/verify should refuse when no preserved worktree")
	}
	if !strings.Contains(out.String(), "no preserved worktree") {
		t.Errorf("output should explain missing worktree; got %q", out.String())
	}
}

func TestVerify_HappyPathDispatches(t *testing.T) {
	runner := &capturingRunner{}
	r, store, _ := newVerifyREPL(t, runner)
	plan := &types.ChangePlan{
		ID:           "plan-ok",
		Status:       types.PlanStatusApplied,
		WorktreePath: "/tmp/preserved/abc-1",
		Changes:      []types.FileChange{{Path: "a.go", Kind: "modify"}},
	}
	path, err := store.SaveForTest(plan)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	r.pendingPlanPath = path
	r.handleVerifyCmd("/verify")
	if !runner.runCalled {
		t.Fatal("/verify should dispatch Run on happy path")
	}
	if runner.modeAtRun != types.ModeVerify {
		t.Errorf("mode = %q; want ModeVerify", runner.modeAtRun)
	}
	if runner.planPathAtRun != path {
		t.Errorf("planPath = %q; want %q", runner.planPathAtRun, path)
	}
}

func TestVerify_ExplicitPlanIDArg(t *testing.T) {
	runner := &capturingRunner{}
	r, store, _ := newVerifyREPL(t, runner)
	plan := &types.ChangePlan{
		ID:           "plan-explicit",
		Status:       types.PlanStatusVerifyFailed, // still legal for /verify
		WorktreePath: "/tmp/wt/explicit",
		Changes:      []types.FileChange{{Path: "a.go", Kind: "modify"}},
	}
	if _, err := store.SaveForTest(plan); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Note: pendingPlanPath is empty — /verify must accept the arg.
	r.handleVerifyCmd("/verify plan-explicit")
	if !runner.runCalled {
		t.Fatal("/verify <id> should dispatch")
	}
	if !strings.HasSuffix(runner.planPathAtRun, "plan-explicit.json") {
		t.Errorf("planPath should end in plan-explicit.json; got %q", runner.planPathAtRun)
	}
}

// ── /worktree handler tests ───────────────────────────────────────

// TestMaybeNudgeWorktreeGC_FiresOnThreshold pins commit 46:
// after every Nth (default 10) successful /approve AND when
// >5 preserved worktrees exist, surfaces the "/worktree gc"
// hint. Quiet on counts below the threshold.
func TestMaybeNudgeWorktreeGC_FiresOnThreshold(t *testing.T) {
	r, _, out := newVerifyREPL(t, &writeCapableRunner{})
	// Seed 6 applied plans with WorktreePath populated.
	for i := 0; i < 6; i++ {
		plan := &types.ChangePlan{
			ID:           "plan-keep-" + string(rune('a'+i)),
			Status:       types.PlanStatusApplied,
			CreatedAt:    time.Now(),
			WorktreePath: "/tmp/wt-" + string(rune('a'+i)),
			TargetPaths:  []string{"x.go"},
			Changes:      []types.FileChange{{Path: "x.go", Kind: "create"}},
		}
		if _, err := r.planStore.SaveForTest(plan); err != nil {
			t.Fatalf("seed plan %d: %v", i, err)
		}
	}
	// Drive the counter to 10 (the trigger threshold).
	for i := 0; i < 10; i++ {
		r.maybeNudgeWorktreeGC()
	}
	if !strings.Contains(out.String(), "/worktree gc") {
		t.Errorf("expected gc hint after 10 successes + 6 preserved; got:\n%s", out.String())
	}
}

// TestMaybeNudgeWorktreeGC_QuietBelowThreshold pins the
// negative case: 9 successes shouldn't trigger.
func TestMaybeNudgeWorktreeGC_QuietBelowThreshold(t *testing.T) {
	r, _, out := newVerifyREPL(t, &writeCapableRunner{})
	for i := 0; i < 6; i++ {
		plan := &types.ChangePlan{
			ID:           "plan-quiet-" + string(rune('a'+i)),
			Status:       types.PlanStatusApplied,
			CreatedAt:    time.Now(),
			WorktreePath: "/tmp/wt-" + string(rune('a'+i)),
			TargetPaths:  []string{"x.go"},
			Changes:      []types.FileChange{{Path: "x.go", Kind: "create"}},
		}
		if _, err := r.planStore.SaveForTest(plan); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	for i := 0; i < 9; i++ {
		r.maybeNudgeWorktreeGC()
	}
	if strings.Contains(out.String(), "/worktree gc") {
		t.Errorf("hint should be quiet at 9 successes; got:\n%s", out.String())
	}
}

func TestWorktreeList_EmptyStore(t *testing.T) {
	r, _, out := newVerifyREPL(t, &writeCapableRunner{})
	r.handleWorktreeCmd("/worktree")
	if !strings.Contains(out.String(), "no preserved worktrees") {
		t.Errorf("empty store should say no preserved worktrees; got %q", out.String())
	}
}

func TestWorktreeList_OnlyAppliedWithPathRender(t *testing.T) {
	r, store, out := newVerifyREPL(t, &writeCapableRunner{})
	// Seed variety: one applied+path (should render), one applied but
	// no path, one pending (should be filtered).
	plans := []*types.ChangePlan{
		{
			ID:           "plan-shown",
			Summary:      "x",
			Status:       types.PlanStatusApplied,
			WorktreePath: "/tmp/wt/plan-shown",
			Changes:      []types.FileChange{{Path: "a.go", Kind: "modify"}},
		},
		{
			ID:      "plan-applied-nopath",
			Status:  types.PlanStatusApplied,
			Changes: []types.FileChange{{Path: "b.go", Kind: "modify"}},
		},
		{
			ID:      "plan-pending",
			Status:  types.PlanStatusPending,
			Changes: []types.FileChange{{Path: "c.go", Kind: "modify"}},
		},
	}
	for _, p := range plans {
		if _, err := store.SaveForTest(p); err != nil {
			t.Fatalf("Save %s: %v", p.ID, err)
		}
	}
	r.handleWorktreeCmd("/worktree list")
	output := out.String()
	if !strings.Contains(output, "plan-shown") || !strings.Contains(output, "/tmp/wt/plan-shown") {
		t.Errorf("applied+path plan should render; got %q", output)
	}
	if strings.Contains(output, "plan-applied-nopath") {
		t.Errorf("applied-without-path should be filtered; got %q", output)
	}
	if strings.Contains(output, "plan-pending") {
		t.Errorf("pending plan should be filtered; got %q", output)
	}
}

func TestWorktreeDiscard_NoSuchPlan(t *testing.T) {
	r, _, out := newVerifyREPL(t, &writeCapableRunner{})
	r.handleWorktreeCmd("/worktree discard ghost-plan")
	if !strings.Contains(out.String(), "/worktree discard") {
		t.Errorf("should surface an error about missing plan; got %q", out.String())
	}
}

func TestWorktreeDiscard_MissingArg(t *testing.T) {
	r, _, out := newVerifyREPL(t, &writeCapableRunner{})
	r.handleWorktreeCmd("/worktree discard")
	if !strings.Contains(out.String(), "specify which") {
		t.Errorf("missing arg should show guidance; got %q", out.String())
	}
}

func TestWorktreeDiscard_ClearsPathOnDisk(t *testing.T) {
	r, store, _ := newVerifyREPL(t, &writeCapableRunner{})
	// Use an absolute path that DiscardByPath can traverse without
	// git (belt-and-suspenders RemoveAll handles absence).
	wtPath := filepath.Join(t.TempDir(), "fake-wt")
	plan := &types.ChangePlan{
		ID:           "plan-discard-me",
		Status:       types.PlanStatusApplied,
		WorktreePath: wtPath,
		Changes:      []types.FileChange{{Path: "a.go", Kind: "modify"}},
	}
	if _, err := store.SaveForTest(plan); err != nil {
		t.Fatalf("Save: %v", err)
	}
	r.handleWorktreeCmd("/worktree discard plan-discard-me")

	reloaded, err := store.Load("plan-discard-me")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.WorktreePath != "" {
		t.Errorf("WorktreePath should be cleared after discard; got %q", reloaded.WorktreePath)
	}
	if reloaded.Status != types.PlanStatusApplied {
		t.Errorf("Status should remain %q after discard; got %q",
			types.PlanStatusApplied, reloaded.Status)
	}
}

func TestWorktreeCmd_UnknownSubcommand(t *testing.T) {
	r, _, out := newVerifyREPL(t, &writeCapableRunner{})
	r.handleWorktreeCmd("/worktree nuke-everything")
	if !strings.Contains(out.String(), "subcommands") {
		t.Errorf("unknown subcommand should list usage; got %q", out.String())
	}
}
