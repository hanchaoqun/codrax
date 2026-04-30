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
	path, err := store.SaveForTest(plan)
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
		// English to keep the historical "approve cancelled" /
		// "plan rejected" substring assertions deterministic across
		// the bilingual messages.go rollout.
		Language:     "en",
		WriteEnabled: true, // /approve gates on this; tests target post-gate behaviour
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
	path, err := store.SaveForTest(plan)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	out := &bytes.Buffer{}
	in := strings.NewReader("y\n")
	r := New(Config{
		Runner:       stubRunner{}, // no SetMode / SetPlanPath
		In:           in,
		Out:          out,
		RepoRoot:     "/tmp/repo",
		Branch:       "main",
		Render:       renderNothing,
		PlanStore:    store,
		WriteEnabled: true,
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

// TestApprove_RefusesTerminalStatus verifies /approve blocks
// re-approval of plans that have already reached a terminal
// resolution: applied (already landed), applied_failed (W1/W1b
// or git apply rejected, planner needs to be re-run), rejected
// (operator's deliberate /reject decision must not be silently
// undone).
//
// verify_failed is INTENTIONALLY NOT in this list — see
// TestApprove_AcceptsVerifyFailedForRetry. A plan that applied
// cleanly but failed verify is the env-fix retry path: the
// operator installs the missing test runner / starts the missing
// service / etc., then /approve to re-run apply+verify against
// the same reviewed plan.
func TestApprove_RefusesTerminalStatus(t *testing.T) {
	cases := []struct{ status string }{
		{types.PlanStatusApplied},
		{types.PlanStatusApplyFailed},
		{types.PlanStatusRejected},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			runner := &writeCapableRunner{}
			store := NewPlanStore(t.TempDir())
			plan := &types.ChangePlan{
				ID:      "plan-status-" + tc.status,
				Summary: "x",
				Status:  tc.status,
				Changes: []types.FileChange{{Path: "a.go", Kind: "modify"}},
			}
			path, err := store.SaveForTest(plan)
			if err != nil {
				t.Fatalf("Save: %v", err)
			}
			memStore, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
			if err != nil {
				t.Fatalf("memory.NewStore: %v", err)
			}
			t.Cleanup(func() { memStore.Close() })
			out := &bytes.Buffer{}
			r := New(Config{
				Runner:       runner,
				Store:        memStore,
				In:           strings.NewReader("y\n"), // shouldn't get this far
				Out:          out,
				RepoRoot:     "/tmp/repo",
				Branch:       "main",
				Language:     "en",
				Render:       renderNothing,
				PlanStore:    store,
				WriteEnabled: true,
			})
			r.pendingPlanPath = path

			r.handleApproveCmd("/approve")

			if runner.runCalled {
				t.Errorf("/approve should NOT dispatch Run when Status=%q", tc.status)
			}
			got := out.String()
			if !strings.Contains(got, "approve refused") {
				t.Errorf("output should include 'approve refused'; got %q", got)
			}
			if !strings.Contains(got, tc.status) {
				t.Errorf("output should name the offending status %q; got %q", tc.status, got)
			}
			if r.pendingPlanPath != "" {
				t.Errorf("pendingPlanPath should be cleared on non-pending refusal; got %q", r.pendingPlanPath)
			}
		})
	}
}

// TestApprove_AcceptsVerifyFailedForRetry locks the env-fix retry
// contract. A plan that applied cleanly but failed verify (because
// pytest wasn't installed, the test database wasn't running, a
// network endpoint flaked, etc.) MUST be re-approvable after the
// operator fixes the environment — re-running /mode plan would
// wastefully re-classify the same code-write request and produce
// an equivalent ChangePlan.
func TestApprove_AcceptsVerifyFailedForRetry(t *testing.T) {
	runner := &writeCapableRunner{}
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID:      "plan-verify-failed-retry",
		Summary: "x",
		Status:  types.PlanStatusVerifyFailed,
		Changes: []types.FileChange{{Path: "a.go", Kind: "modify"}},
	}
	path, err := store.SaveForTest(plan)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	memStore, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	t.Cleanup(func() { memStore.Close() })
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:       runner,
		Store:        memStore,
		In:           strings.NewReader("y\n"),
		Out:          out,
		RepoRoot:     "/tmp/repo",
		Branch:       "main",
		Render:       renderNothing,
		PlanStore:    store,
		Language:     "en",
		WriteEnabled: true,
	})
	r.pendingPlanPath = path

	r.handleApproveCmd("/approve")

	if !runner.runCalled {
		t.Error("/approve must dispatch Run for verify_failed plans (env-fix retry)")
	}
	if !strings.Contains(out.String(), "re-approving") || !strings.Contains(out.String(), "verify_failed") {
		t.Errorf("output should announce the verify_failed retry; got %q", out.String())
	}
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

// TestApprove_RunErrorKeepsPendingPath verifies that if Run returns
// a pre-flight error (worktree provisioning, missing git, bare dir
// without auto-init authorization), pendingPlanPath is **kept** so
// the user can /plan show the surviving pending plan, fix the
// environment, and /approve again — or /reject deliberately. Earlier
// builds dropped the pointer and made the plan invisible until the
// user manually inspected the PlanStore JSON.
func TestApprove_RunErrorKeepsPendingPath(t *testing.T) {
	runner := &writeCapableRunner{runErr: errors.New("apply boom")}
	r, _, out := newApprovalREPL(t, "y\n", runner)
	priorPath := r.pendingPlanPath
	if priorPath == "" {
		t.Fatal("test setup: pendingPlanPath must be seeded")
	}
	r.handleApproveCmd("/approve")
	if !runner.runCalled {
		t.Fatal("Run should fire")
	}
	if r.pendingPlanPath != priorPath {
		t.Errorf("pendingPlanPath should be retained after pre-flight Run error; got %q want %q",
			r.pendingPlanPath, priorPath)
	}
	if !strings.Contains(out.String(), "apply boom") {
		t.Errorf("expected Run error text in output, got: %q", out.String())
	}
}

// TestApprove_TaskStateLastErrorKeepsPendingPath verifies the same
// retention behaviour for the "Run returned cleanly, but the
// pipeline hit a soft failure" path. The plan is already stamped
// applied_failed / verify_failed on disk, so /approve again would
// refuse on the status check; keeping the pointer lets /plan show
// surface the failed plan + diff for post-mortem inspection.
func TestApprove_TaskStateLastErrorKeepsPendingPath(t *testing.T) {
	runner := &writeCapableRunner{
		runResult: &types.BusContext{
			Mutable: types.NewMutableState("approve"),
			TaskState: types.TaskState{
				LastError: "apply: W1 violation",
			},
		},
	}
	r, _, _ := newApprovalREPL(t, "y\n", runner)
	priorPath := r.pendingPlanPath
	r.handleApproveCmd("/approve")
	if r.pendingPlanPath != priorPath {
		t.Errorf("pendingPlanPath should be retained when TaskState.LastError != \"\"; got %q want %q",
			r.pendingPlanPath, priorPath)
	}
}

// TestApprove_CleanSuccessClearsPendingPath verifies the only path
// that DOES drop the pointer: apply + verify both succeeded and
// TaskState.LastError is empty. The plan is now `applied`, not
// re-approvable, so clearing keeps the next /plan show honest.
func TestApprove_CleanSuccessClearsPendingPath(t *testing.T) {
	runner := &writeCapableRunner{
		runResult: &types.BusContext{
			Mutable: types.NewMutableState("approve"),
		},
	}
	r, _, _ := newApprovalREPL(t, "y\n", runner)
	if r.pendingPlanPath == "" {
		t.Fatal("test setup: pendingPlanPath must be seeded")
	}
	r.handleApproveCmd("/approve")
	if r.pendingPlanPath != "" {
		t.Errorf("pendingPlanPath should be cleared after clean success; got %q", r.pendingPlanPath)
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
		Language:  "en",
	})
	return r, out
}

// TestReject_SettlesPlanWithoutDeletingFile verifies the post-2026-04-30
// /reject contract: the plan file is KEPT on disk with
// Status=rejected (so /plan list / /plan show <id> can still
// surface it for audit), and the in-session pendingPlanPath is
// cleared. /plan clear is the no-audit alternative that deletes
// the file outright.
func TestReject_SettlesPlanWithoutDeletingFile(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID: "plan-reject-1", Summary: "reject test", Status: "pending_approval",
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	}
	path, err := store.SaveForTest(plan)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	r, out := newMemREPL(t, store)
	r.pendingPlanPath = path

	r.handleRejectCmd("/reject")

	if r.pendingPlanPath != "" {
		t.Errorf("pendingPlanPath should be cleared; got %q", r.pendingPlanPath)
	}
	settled, lerr := store.Load("plan-reject-1")
	if lerr != nil || settled == nil {
		t.Fatalf("plan file must remain on disk for audit (Settle, not Clear); load err=%v", lerr)
	}
	if settled.Status != types.PlanStatusRejected {
		t.Errorf("settled plan Status should be %q; got %q", types.PlanStatusRejected, settled.Status)
	}
	if !strings.Contains(out.String(), "rejected plan ") {
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
	path, err := store.SaveForTest(plan)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	r, out := newMemREPL(t, store)
	r.pendingPlanPath = path

	r.handleRejectCmd("/reject scope too wide")

	got := out.String()
	if !strings.Contains(got, "rejected plan ") {
		t.Errorf("expected 'plan rejected' message, got: %q", got)
	}
	if !strings.Contains(got, "scope too wide") {
		t.Errorf("expected reason text in output, got: %q", got)
	}
}
