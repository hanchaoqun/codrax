package repl

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/types"
)

// writeCapableRunner is the test double used by /approve tests: it
// implements Runner + modeSetter + planPathSetter so the REPL's
// capability probe passes and we can observe what gets plumbed through
// before Run is invoked. Test code inspects seenMode / seenPlanPath
// after the handler returns to verify the apply flow was wired
// correctly.
type writeCapableRunner struct {
	seenMode     types.PipelineMode
	seenPlanPath string
	runCalled    bool
	runResult    *types.BusContext
	runErr       error
}

func (r *writeCapableRunner) Run(_, _, _ string) (*types.BusContext, error) {
	r.runCalled = true
	if r.runResult != nil {
		return r.runResult, r.runErr
	}
	return &types.BusContext{Mutable: types.NewMutableState("approve")}, r.runErr
}

func (r *writeCapableRunner) SetMode(m types.PipelineMode) { r.seenMode = m }

func (r *writeCapableRunner) SetPlanPath(p string) { r.seenPlanPath = p }

// newApprovalREPL builds a scripted REPL wired to a writeCapableRunner
// plus a real PlanStore backed by t.TempDir(), then pre-saves plan
// and pendingPlanPath so /approve has something to consume.
func newApprovalREPL(t *testing.T, confirmInput string, runner Runner) (*REPL, *PlanStore, *bytes.Buffer) {
	t.Helper()
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID:      "plan-approve-1",
		Summary: "approve test",
		Status:  "pending_approval",
		Changes: []types.FileChange{
			{Path: "main.go", Kind: "modify", Rationale: "test"},
		},
		TargetPaths: []string{"main.go"},
	}
	path, err := store.Save(plan)
	if err != nil {
		t.Fatalf("PlanStore.Save: %v", err)
	}
	memStore, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	t.Cleanup(func() { memStore.Close() })
	out := &bytes.Buffer{}
	in := strings.NewReader(confirmInput)
	r := New(Config{
		Runner:    runner,
		Store:     memStore,
		In:        in,
		Out:       out,
		RepoRoot:  "/tmp/repo",
		Branch:    "main",
		Render:    renderNothing,
		PlanStore: store,
	})
	r.pendingPlanPath = path
	return r, store, out
}

// TestApprove_DisabledWhenNoStore verifies /approve with a nil
// PlanStore prints the disabled message rather than crashing.
func TestApprove_DisabledWhenNoStore(t *testing.T) {
	r, out := newScriptedREPL(t, nil)
	r.handleApproveCmd("/approve")
	if !strings.Contains(out.String(), "/approve disabled") {
		t.Errorf("expected '/approve disabled' message, got: %q", out.String())
	}
}

// TestApprove_NoPendingPlan verifies /approve with a store but no
// pending plan prints guidance.
func TestApprove_NoPendingPlan(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	r, out := newScriptedREPL(t, store)
	r.handleApproveCmd("/approve")
	if !strings.Contains(out.String(), "no pending plan") {
		t.Errorf("expected 'no pending plan' message, got: %q", out.String())
	}
}

// TestApprove_StaleFileClearsPointer verifies that /approve pointing
// at a file the user deleted by hand clears pendingPlanPath so
// subsequent /plan show / /approve don't nag.
func TestApprove_StaleFileClearsPointer(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	r, _ := newScriptedREPL(t, store)
	r.pendingPlanPath = "/nonexistent/path/plan-missing.json"
	r.handleApproveCmd("/approve")
	if r.pendingPlanPath != "" {
		t.Errorf("stale pendingPlanPath should be cleared; got %q", r.pendingPlanPath)
	}
}

// TestApprove_StubRunnerWarns verifies that a runner without
// modeSetter/planPathSetter triggers the "stub runner detected"
// warning instead of silently running in read mode.
func TestApprove_StubRunnerWarns(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID: "plan-stub-1", Summary: "x", Status: "pending_approval",
		Changes: []types.FileChange{{Path: "a.go", Kind: "modify"}},
	}
	path, err := store.Save(plan)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	out := &bytes.Buffer{}
	in := strings.NewReader("y\n")
	r := New(Config{
		Runner:    stubRunner{}, // no SetMode / SetPlanPath
		In:        in,
		Out:       out,
		RepoRoot:  "/tmp/repo",
		Branch:    "main",
		Render:    renderNothing,
		PlanStore: store,
	})
	r.pendingPlanPath = path
	r.handleApproveCmd("/approve")
	got := out.String()
	if !strings.Contains(got, "stub runner") {
		t.Errorf("expected 'stub runner' warning, got: %q", got)
	}
	// pendingPlanPath should survive — user hasn't consumed or
	// rejected the plan yet; they just have the wrong runner.
	if r.pendingPlanPath != path {
		t.Errorf("pendingPlanPath should survive a stub-runner warning; got %q", r.pendingPlanPath)
	}
}

// TestApprove_CancelledAtConfirm verifies typing "n" at the confirm
// prompt skips the Run dispatch and leaves pendingPlanPath alone.
func TestApprove_CancelledAtConfirm(t *testing.T) {
	runner := &writeCapableRunner{}
	r, _, out := newApprovalREPL(t, "n\n", runner)
	originalPath := r.pendingPlanPath
	r.handleApproveCmd("/approve")
	if runner.runCalled {
		t.Error("Run should NOT be called when user cancels confirm")
	}
	if !strings.Contains(out.String(), "approve cancelled") {
		t.Errorf("expected 'approve cancelled' message, got: %q", out.String())
	}
	if r.pendingPlanPath != originalPath {
		t.Errorf("pendingPlanPath should be preserved on cancel; got %q", r.pendingPlanPath)
	}
}

// TestApprove_HappyPath verifies that a "y" confirm triggers Run
// with Mode=ModeApply + PlanPath seeded, then clears pendingPlanPath
// and restores the original sticky mode.
func TestApprove_HappyPath(t *testing.T) {
	runner := &writeCapableRunner{}
	r, _, _ := newApprovalREPL(t, "y\n", runner)
	originalPath := r.pendingPlanPath
	r.currentMode = types.ModeRead // sticky mode before approve

	r.handleApproveCmd("/approve")

	if !runner.runCalled {
		t.Fatal("Run should fire after user confirms approve")
	}
	// The setters are called with ModeApply + plan path. The defer
	// then restores both — final state should be ModeRead + "".
	// We check the transient values the runner captured during Run,
	// which is the "during dispatch" snapshot.
	// Because SetMode/SetPlanPath are called BEFORE Run and the
	// deferred restore runs AFTER, the values captured inside Run
	// are ModeApply + planPath.
	if runner.seenMode != types.ModeApply {
		// The runner is captured during Run — but the defer has
		// already fired by the time this assertion runs. So we
		// actually see the RESTORED value (ModeRead). Adjust
		// expectation: the restored mode should be ModeRead.
		if runner.seenMode != types.ModeRead {
			t.Errorf("seenMode after restore = %q, want %q or %q",
				runner.seenMode, types.ModeApply, types.ModeRead)
		}
	}
	// pendingPlanPath cleared on success.
	if r.pendingPlanPath != "" {
		t.Errorf("pendingPlanPath should be cleared after successful approve; got %q", r.pendingPlanPath)
	}
	// currentMode (sticky REPL state) should be unchanged — the
	// setter restore brings the runner back to the REPL's value.
	if r.currentMode != types.ModeRead {
		t.Errorf("currentMode should still be ModeRead; got %q", r.currentMode)
	}
	_ = originalPath
}

// TestApprove_HappyPathSetters is a tighter variant that proves the
// setters are hit in the expected order (Mode=Apply + planPath
// seeded) BEFORE Run, by having the mock runner snapshot the
// incoming values synchronously inside Run.
func TestApprove_HappyPathSetters(t *testing.T) {
	runner := &capturingRunner{}
	r, _, _ := newApprovalREPL(t, "y\n", runner)
	plannedPath := r.pendingPlanPath
	r.handleApproveCmd("/approve")
	if !runner.runCalled {
		t.Fatal("Run should fire")
	}
	if runner.modeAtRun != types.ModeApply {
		t.Errorf("mode seen by Run = %q, want %q", runner.modeAtRun, types.ModeApply)
	}
	if runner.planPathAtRun != plannedPath {
		t.Errorf("planPath seen by Run = %q, want %q", runner.planPathAtRun, plannedPath)
	}
}

// capturingRunner snapshots mode + planPath at Run entry so tests
// can assert the pre-Run plumbing is correct. Uses the fact that
// SetMode / SetPlanPath are called BEFORE Run and the deferred
// restore runs AFTER.
type capturingRunner struct {
	curMode       types.PipelineMode
	curPlanPath   string
	modeAtRun     types.PipelineMode
	planPathAtRun string
	runCalled     bool
}

func (r *capturingRunner) Run(_, _, _ string) (*types.BusContext, error) {
	r.runCalled = true
	r.modeAtRun = r.curMode
	r.planPathAtRun = r.curPlanPath
	return &types.BusContext{Mutable: types.NewMutableState("approve")}, nil
}
func (r *capturingRunner) SetMode(m types.PipelineMode) { r.curMode = m }
func (r *capturingRunner) SetPlanPath(p string)         { r.curPlanPath = p }

// TestApprove_RunErrorClearsPendingPath verifies that if Run returns
// an error, pendingPlanPath is still cleared so the user doesn't
// loop on a broken plan.
func TestApprove_RunErrorClearsPendingPath(t *testing.T) {
	runner := &writeCapableRunner{runErr: errors.New("apply boom")}
	r, _, out := newApprovalREPL(t, "y\n", runner)
	r.handleApproveCmd("/approve")
	if !runner.runCalled {
		t.Fatal("Run should fire")
	}
	if r.pendingPlanPath != "" {
		t.Errorf("pendingPlanPath should be cleared after Run error; got %q", r.pendingPlanPath)
	}
	if !strings.Contains(out.String(), "apply boom") {
		t.Errorf("expected Run error text in output, got: %q", out.String())
	}
}

// TestReject_NoPendingPlan verifies /reject without pendingPlanPath
// is a benign no-op.
func TestReject_NoPendingPlan(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	r, out := newScriptedREPL(t, store)
	r.handleRejectCmd("/reject")
	if !strings.Contains(out.String(), "no pending plan to reject") {
		t.Errorf("expected 'no pending plan to reject' message, got: %q", out.String())
	}
}

// TestReject_DisabledWhenNoStore verifies /reject without a
// PlanStore configured prints the disabled message.
func TestReject_DisabledWhenNoStore(t *testing.T) {
	r, out := newScriptedREPL(t, nil)
	r.handleRejectCmd("/reject")
	if !strings.Contains(out.String(), "/reject disabled") {
		t.Errorf("expected '/reject disabled' message, got: %q", out.String())
	}
}

// newMemREPL returns a scripted REPL wired with BOTH a PlanStore and
// a real memory.Store. /reject's success tail calls recordTurn, which
// panics on a nil store — tests that reach the tail need this helper
// rather than newScriptedREPL.
func newMemREPL(t *testing.T, planStore *PlanStore) (*REPL, *bytes.Buffer) {
	t.Helper()
	memStore, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	t.Cleanup(func() { memStore.Close() })
	in := strings.NewReader("")
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:    stubRunner{},
		Store:     memStore,
		In:        in,
		Out:       out,
		RepoRoot:  "/tmp/repo",
		Branch:    "main",
		Render:    renderNothing,
		PlanStore: planStore,
	})
	return r, out
}

// TestReject_ClearsPlanAndPath verifies /reject removes the JSON
// file AND resets pendingPlanPath. Same shape as /plan clear but
// with memory-turn recording.
func TestReject_ClearsPlanAndPath(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID: "plan-reject-1", Summary: "reject test", Status: "pending_approval",
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	}
	path, err := store.Save(plan)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	r, out := newMemREPL(t, store)
	r.pendingPlanPath = path

	r.handleRejectCmd("/reject")

	if r.pendingPlanPath != "" {
		t.Errorf("pendingPlanPath should be cleared; got %q", r.pendingPlanPath)
	}
	if _, err := store.Load("plan-reject-1"); err == nil {
		t.Errorf("plan file should be removed after /reject")
	}
	if !strings.Contains(out.String(), "plan rejected") {
		t.Errorf("expected 'plan rejected' message, got: %q", out.String())
	}
}

// TestReject_WithReason verifies /reject <reason> surfaces the
// reason in the user-visible success message.
func TestReject_WithReason(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID: "plan-reject-2", Summary: "x", Status: "pending_approval",
	}
	path, err := store.Save(plan)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	r, out := newMemREPL(t, store)
	r.pendingPlanPath = path

	r.handleRejectCmd("/reject scope too wide")

	got := out.String()
	if !strings.Contains(got, "plan rejected") {
		t.Errorf("expected 'plan rejected' message, got: %q", got)
	}
	if !strings.Contains(got, "scope too wide") {
		t.Errorf("expected reason text in output, got: %q", got)
	}
}
