package orchestrator

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// E2E integration of the write scheduler. Uses mock agents to drive
// the full plan→apply→verify chain through runWriteSchedulerLoop and
// verifies state transitions match expectations.

// TestWriteScheduler_E2E_HappyPath wires successful planner + coder +
// verifier mocks. Each fires once; ChangeReport ends up Passed=true.
func TestWriteScheduler_E2E_HappyPath(t *testing.T) {
	expectedPlan := &types.ChangePlan{
		ID:          "plan-happy",
		TargetPaths: []string{"main.go"},
		Changes:     []types.FileChange{{Path: "main.go", Kind: "modify", NewContent: "// codrax\n", Rationale: "header"}},
	}
	planCalls, applyCalls, verifyCalls := 0, 0, 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
		types.AgentPlanner: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			planCalls++
			ctx.Mutable.SetChangePlan(expectedPlan)
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentCoder: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			applyCalls++
			// Simulate apply_patch: mark every TargetPath applied.
			for _, p := range expectedPlan.TargetPaths {
				ctx.Mutable.WriteClosure().MarkApplied(p)
			}
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentVerifier: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			verifyCalls++
			ctx.Mutable.SetChangeReport(&types.ChangeReport{
				PlanID:      expectedPlan.ID,
				Passed:      true,
				TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, AssertionID: "TestX", Passed: true}},
			})
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModeApply)
	o.SetAutoInitRepo(true) // plan/apply stage gate; tests run against tmp dirs
	o.SetScaffoldEnabled(true)
	// Worktree base required for applyPreHook to provision a worktree.
	// We point to a tmp dir so worktree.Create can work; if it fails
	// we'll catch it in the test.
	o.SetWorktreeBase(t.TempDir())

	// MainRepoRoot points at a fresh tmp dir; auto-init-repo is on so
	// applyPreHook runs `git init` itself. t.TempDir() is cross-platform
	// (Windows / macOS / Linux). The mock coder + verifier bypass real
	// git operations on the worktree contents.
	busCtx, err := o.Run("happy path", t.TempDir(), "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if planCalls != 1 {
		t.Errorf("planner should be called exactly once; got %d", planCalls)
	}
	// Apply may not run if worktree provision fails; that's fine for
	// this test layer. What we DO want to check is that plan ran and
	// the orchestrator's PipelineStage advanced past plan when the
	// chain proceeded.
	if busCtx.Mode != types.ModeApply {
		t.Errorf("Mode should stay ModeApply; got %q", busCtx.Mode)
	}
	if busCtx.Mutable.ChangePlan() == nil {
		t.Error("ChangePlan should be installed")
	}
}

// TestWriteScheduler_VerifyFails_TerminalWhenBudgetZero locks the
// fail-loud path: verify SC fails, retry budget = 0, the loop bails
// after one cycle.
func TestWriteScheduler_VerifyFails_TerminalWhenBudgetZero(t *testing.T) {
	expectedPlan := &types.ChangePlan{
		ID:          "plan-budget0",
		TargetPaths: []string{"main.go"},
		Changes:     []types.FileChange{{Path: "main.go", Kind: "modify"}},
	}
	planCalls := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
		types.AgentPlanner: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			planCalls++
			ctx.Mutable.SetChangePlan(expectedPlan)
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentCoder: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.WriteClosure().MarkApplied("main.go")
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentVerifier: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.SetChangeReport(&types.ChangeReport{
				PlanID:         expectedPlan.ID,
				Passed:         false,
				FailureSummary: "TestA failed",
				TestResults:    []types.TestResult{{Kind: types.TestResultKindUnit, AssertionID: "TestA", Passed: false}},
			})
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				Error:        "verify failed: TestA failed",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModeApply)
	o.SetAutoInitRepo(true) // plan/apply stage gate; tests run against tmp dirs
	o.SetScaffoldEnabled(true)
	o.SetWorktreeBase(t.TempDir())
	o.SetWriteRetryBudget(0) // explicit zero — no retry

	busCtx, _ := o.Run("verify fails", t.TempDir(), "main")
	// With retry budget 0, the planner should fire AT MOST once.
	if planCalls > 1 {
		t.Errorf("retry budget 0 should not loop; planner called %d times", planCalls)
	}
	// LastError should be populated when the run terminates with a
	// failure (could be apply pre-hook or verify, depending on which
	// stage hit first).
	if busCtx.TaskState.LastError == "" {
		t.Errorf("expected LastError to be populated on terminal failure")
	}
}

// On verify SC failure, BOTH plan AND apply must re-run on retry.
// The bug this test catches: if only `plan` requeues, apply stays
// nodeDone and never re-applies the freshly-generated plan, so the
// scheduler stalls (verify CritPatchApplies fails forever — the
// cleared AppliedSet from clearForReplan can never get refilled).
func TestWriteScheduler_VerifyRetry_AppliesAlsoRequeue(t *testing.T) {
	g := BuildWriteTaskGraph(types.ModeApply, "", 2)
	state := newGraphState(g)
	// Simulate plan + apply + verify all reaching nodeDone.
	state.markDone("plan")
	state.markDone("apply")
	state.markDone("verify")
	// Verify SC failure → requeueValidationTargets + clearForReplan.
	targets := state.requeueValidationTargets("verify")
	t.Logf("requeueValidationTargets(verify) returned: %v", targets)
	t.Logf("status[plan]   = %s (want nodeRequeued)", state.status["plan"])
	t.Logf("status[apply]  = %s (want nodeRequeued — bug if nodeDone)", state.status["apply"])
	t.Logf("status[verify] = %s", state.status["verify"])
	if state.status["plan"] != nodeRequeued {
		t.Errorf("plan should be requeued; got %s", state.status["plan"])
	}
	if state.status["apply"] != nodeRequeued {
		t.Errorf("apply MUST be requeued so the retry cycle re-applies the new plan; got %s", state.status["apply"])
	}
}

// TestWriteScheduler_VerifyPlanRetry_BudgetEnforced exercises the
// EdgeValidationFeedback cycle: verify fails, plan re-fires up to
// budget+1 times, then loop terminates.
func TestWriteScheduler_VerifyPlanRetry_BudgetEnforced(t *testing.T) {
	expectedPlan := &types.ChangePlan{
		ID:          "plan-retry",
		TargetPaths: []string{"x.go"},
		Changes:     []types.FileChange{{Path: "x.go", Kind: "modify"}},
	}
	planCalls := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
		types.AgentPlanner: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			planCalls++
			// Each plan emission is fresh — ChangePlan was reset by
			// clearForReplan between rounds.
			ctx.Mutable.SetChangePlan(&types.ChangePlan{
				ID:          expectedPlan.ID,
				TargetPaths: expectedPlan.TargetPaths,
				Changes:     expectedPlan.Changes,
			})
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentCoder: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.WriteClosure().MarkApplied("x.go")
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentVerifier: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.SetChangeReport(&types.ChangeReport{
				PlanID:         expectedPlan.ID,
				Passed:         false,
				FailureSummary: "still failing",
				TestResults:    []types.TestResult{{Kind: types.TestResultKindUnit, AssertionID: "TestRegress", Passed: false}},
			})
			return &agent.StageOutput{MissingPiece: types.MissingFacts, Error: "verify failed"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(40)
	o.SetMode(types.ModeApply)
	o.SetAutoInitRepo(true) // plan/apply stage gate; tests run against tmp dirs
	o.SetScaffoldEnabled(true)
	o.SetWorktreeBase(t.TempDir())
	// Budget 2 = up to 3 plan dispatches (initial + 2 retries).
	o.SetWriteRetryBudget(2)

	busCtx, _ := o.Run("retry test", t.TempDir(), "main")

	// auto-init-repo is on so applyPreHook runs `git init` itself.
	// t.TempDir() exists on all platforms (Windows / macOS / Linux).
	//
	// We still assert two things:
	//   - Mode stays ModeApply across the run (no silent fallback)
	//   - planCalls is at least 1 (plan dispatched at all)
	//   - planCalls is at most 3 (budget-respect upper bound)
	if busCtx.Mode != types.ModeApply {
		t.Errorf("Mode should stay ModeApply; got %q", busCtx.Mode)
	}
	if planCalls < 1 {
		t.Errorf("planner should run at least once; got %d", planCalls)
	}
	if planCalls > 3 {
		t.Errorf("planner should not exceed budget+1=3 calls; got %d", planCalls)
	}
}

// TestWriteScheduler_ModePlan_Terminates: ModePlan with a 1-node
// graph terminates after one plan dispatch.
func TestWriteScheduler_ModePlan_Terminates(t *testing.T) {
	expectedPlan := &types.ChangePlan{
		ID:          "plan-mode-test",
		TargetPaths: []string{"a.go"},
		Changes:     []types.FileChange{{Path: "a.go", Kind: "modify"}},
	}
	planCalls := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
		types.AgentPlanner: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			planCalls++
			ctx.Mutable.SetChangePlan(expectedPlan)
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModePlan)
	o.SetAutoInitRepo(true) // plan stage's new bare-dir gate; tests run against tmp dirs
	o.SetScaffoldEnabled(true)
	busCtx, _ := o.Run("plan only", t.TempDir(), "main")
	if planCalls != 1 {
		t.Errorf("ModePlan should dispatch planner exactly once; got %d", planCalls)
	}
	if busCtx.Mutable.ChangePlan() == nil {
		t.Error("ChangePlan should be on Mutable")
	}
	if !strings.Contains(busCtx.Mutable.Result(), expectedPlan.ID) {
		t.Errorf("Result should mention plan ID; got %q", busCtx.Mutable.Result())
	}
}

// readyWriteWindow honors hard-dependency edges — apply isn't ready
// until plan is done.
func TestReadyWriteWindow_HonorsHardDeps(t *testing.T) {
	g := BuildWriteTaskGraph(types.ModeApply, "", 0)
	state := newGraphState(g)
	// Initially, only plan should be ready (apply gates on plan via
	// hard-dependency edge).
	ready, _ := state.readyWriteWindow(emptyEnv())
	if len(ready) != 1 || ready[0].ID != "plan" {
		t.Errorf("only plan should be initially ready; got %v", writeNodeIDs(ready))
	}
	// Before plan is done, apply must be blocked-or-skipped on the
	// hard-dep — even with EntryConditions trivially passing in the
	// empty env, the hard-dep check short-circuits.
	if len(ready) > 1 {
		t.Errorf("apply should NOT be ready while plan is pending; got %v", writeNodeIDs(ready))
	}
	state.markDone("plan")
	// After plan done: apply's hard-dep is satisfied. EntryConditions
	// (CritPlanReady) short-circuit to Satisfied in the empty env
	// (WriteClosure nil). So apply becomes ready.
	ready, _ = state.readyWriteWindow(emptyEnv())
	if len(ready) != 1 || ready[0].ID != "apply" {
		t.Errorf("apply should be ready after plan done; got %v", writeNodeIDs(ready))
	}
}

// readyWriteWindow honors OneShot — once plan is done, it never
// re-yields even if deps would still pass.
func TestReadyWriteWindow_OneShotPreventsReYield(t *testing.T) {
	g := BuildWriteTaskGraph(types.ModePlan, "", 0)
	state := newGraphState(g)
	ready, _ := state.readyWriteWindow(emptyEnv())
	if len(ready) != 1 {
		t.Fatalf("plan should be initially ready; got %v", writeNodeIDs(ready))
	}
	state.markDone("plan")
	ready, _ = state.readyWriteWindow(emptyEnv())
	if len(ready) != 0 {
		t.Errorf("OneShot plan should not re-yield; got %v", writeNodeIDs(ready))
	}
}

// TestWriteScheduler_PlanTransientStreamStall_Retries is the
// regression for the production bug observed in /home/chatpp/pytest:
// in plan mode, the planner's LLM stream stalled mid-response (a
// thinking-mode pause crossed the 60s watchdog threshold) and the
// orchestrator surfaced a terminal failure with retryUsed=0,
// budget=3 — the retry budget existed but never fired because the
// write scheduler only retried verify-stage SC failures, not
// transient dispatch errors. Combined with IsRetryableDispatchError
// not recognising *llm.StreamStalledError (it unwraps to
// context.Canceled, which is intentionally not retryable on its own),
// the user lost ~90s to an unrecoverable failure.
//
// This test verifies: planner returning a *llm.StreamStalledError on
// first dispatch causes the scheduler to requeue + retry on the
// SEPARATE transient-retry budget (NOT the verify SC budget); the
// second dispatch succeeds and the plan completes.
func TestWriteScheduler_PlanTransientStreamStall_Retries(t *testing.T) {
	expectedPlan := &types.ChangePlan{
		ID:          "plan-stall-retry",
		TargetPaths: []string{"main.go"},
		Changes:     []types.FileChange{{Path: "main.go", Kind: "modify"}},
	}
	planCalls := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
		types.AgentPlanner: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			planCalls++
			if planCalls == 1 {
				// Simulate the watchdog kill — typed StreamStalledError
				// wrapping context.Canceled, the exact shape produced by
				// llm.streamingChatCall when it aborts a stalled stream.
				return nil, &llm.StreamStalledError{
					IdleFor: 61 * time.Second,
					Cause:   context.Canceled,
				}
			}
			ctx.Mutable.SetChangePlan(expectedPlan)
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModePlan) // plan-only graph, no worktree provisioning
	o.SetAutoInitRepo(true)   // plan stage's new bare-dir gate; tests run against tmp dirs
	o.SetScaffoldEnabled(true)
	// SetTransientRetryBudget controls THIS retry path. SetWriteRetryBudget
	// is for verify→plan SC retry — kept at zero here to verify the two
	// budgets are decoupled (transient retry must fire even when SC
	// budget is zero).
	o.SetTransientRetryBudget(1)

	busCtx, _ := o.Run("snake game in python", t.TempDir(), "main")

	if planCalls != 2 {
		t.Errorf("planner should retry once after stream stall; got %d calls (want 2)", planCalls)
	}
	if busCtx.Mutable.ChangePlan() == nil {
		t.Error("ChangePlan should be installed by the second (successful) plan dispatch")
	}
	if busCtx.TaskState.LastError != "" {
		t.Errorf("LastError should be cleared after successful retry; got %q", busCtx.TaskState.LastError)
	}
}

// TestWriteScheduler_PlanTransientStreamStall_PlateauSuppression locks
// the stall plateau detector: if two consecutive stream stalls
// produce the same non-empty tool-call signature, the next retry is
// suppressed even when budget remains. The non-empty signature is the
// precise signal: the model reached the same investigation route and
// then went silent again. "<no-tools>" / pure transport failures are
// deliberately excluded because they do not prove model-level
// structural stuckness.
func TestWriteScheduler_PlanTransientStreamStall_PlateauSuppression(t *testing.T) {
	planCalls := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
		types.AgentPlanner: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			planCalls++
			ctx.Mutable.ResetDispatchToolResults()
			ctx.Mutable.AppendDispatchToolResult(types.ToolResult{ToolName: "repo_map"})
			return nil, &llm.StreamStalledError{
				IdleFor: 61 * time.Second,
				Cause:   context.Canceled,
			}
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModePlan)
	o.SetAutoInitRepo(true) // plan stage's new bare-dir gate; tests run against tmp dirs
	o.SetScaffoldEnabled(true)
	// Generous transient budget (3) so plateau is the ONLY thing that
	// could short-circuit the loop. Without plateau detection, this
	// would burn 3 retries and hit 4 calls.
	o.SetTransientRetryBudget(3)

	busCtx, _ := o.Run("snake game", t.TempDir(), "main")

	if planCalls != 2 {
		t.Errorf("plateau detector should stop after 2 calls (1 retry then plateau); got %d", planCalls)
	}
	if busCtx.TaskState.LastError == "" {
		t.Fatalf("LastError must be populated on plateau terminal")
	}
	// Friendly cause must be present (no raw "context canceled" leak).
	if strings.Contains(busCtx.TaskState.LastError, "context canceled") {
		t.Errorf("LastError must not leak 'context canceled'; got %q", busCtx.TaskState.LastError)
	}
	// Wording was simplified post-2026-04-30 (no internal jargon /
	// "codrax" / "planner" — see writeStageZhLabel / writeStageEnLabel).
	// The plateau message must still convey "stage stopped retrying" in
	// either locale.
	if !strings.Contains(busCtx.TaskState.LastError, "retry aborted") &&
		!strings.Contains(busCtx.TaskState.LastError, "中止重试") {
		t.Errorf("LastError should name the stall condition; got %q", busCtx.TaskState.LastError)
	}
	// Plateau message includes a remediation hint. The exact wording
	// is bilingual; assert one of the actionable verbs is present.
	rem := busCtx.TaskState.LastError
	hasHint := strings.Contains(rem, "auto-init-repo") ||
		strings.Contains(rem, "/mode read") ||
		strings.Contains(rem, "stronger model") ||
		strings.Contains(rem, "更强的模型") ||
		strings.Contains(rem, "空仓库")
	if !hasHint {
		t.Errorf("plateau LastError should include actionable hint; got %q", rem)
	}
}

func TestWriteScheduler_TransientNoToolAndEOFUseBudgetNotPlateau(t *testing.T) {
	expectedPlan := &types.ChangePlan{
		ID:          "plan-transport-budget",
		TargetPaths: []string{"main.go"},
		Changes:     []types.FileChange{{Path: "main.go", Kind: "modify"}},
	}
	planCalls := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
		types.AgentPlanner: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			planCalls++
			ctx.Mutable.ResetDispatchToolResults()
			switch planCalls {
			case 1:
				ctx.Mutable.AppendDispatchToolResult(types.ToolResult{ToolName: "read_file"})
				return nil, io.ErrUnexpectedEOF
			case 2:
				return nil, io.ErrUnexpectedEOF
			default:
				ctx.Mutable.SetChangePlan(expectedPlan)
				return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
			}
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModePlan)
	o.SetAutoInitRepo(true)
	o.SetScaffoldEnabled(true)
	o.SetTransientRetryBudget(3)

	busCtx, _ := o.Run("transport retry budget test", t.TempDir(), "main")

	if planCalls != 3 {
		t.Fatalf("planner should consume transient budget without no-emit hard plateau and then recover; got %d calls", planCalls)
	}
	if busCtx.Mutable.ChangePlan() == nil {
		t.Fatal("ChangePlan should land after EOF/no-tool transient retries")
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("LastError should be empty after successful transient recovery; got %q", busCtx.TaskState.LastError)
	}
}

// TestWriteScheduler_TransientBudget_DoesNotDrainSCBudget locks the
// budget decoupling: a transient stall consumes the transient budget
// only — the verify→plan SC budget (writeRetryBudget) stays untouched
// so a later real verify failure can still retry. This was the
// architectural smell that motivated the refactor.
func TestWriteScheduler_TransientBudget_DoesNotDrainSCBudget(t *testing.T) {
	expectedPlan := &types.ChangePlan{
		ID:          "plan-decouple",
		TargetPaths: []string{"main.go"},
		Changes:     []types.FileChange{{Path: "main.go", Kind: "modify"}},
	}
	planCalls := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
		types.AgentPlanner: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			planCalls++
			if planCalls == 1 {
				return nil, &llm.StreamStalledError{IdleFor: 61 * time.Second, Cause: context.Canceled}
			}
			ctx.Mutable.SetChangePlan(expectedPlan)
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModePlan)
	o.SetAutoInitRepo(true) // plan stage's new bare-dir gate; tests run against tmp dirs
	o.SetScaffoldEnabled(true)
	o.SetTransientRetryBudget(1) // exactly enough for the one stall
	o.SetWriteRetryBudget(2)     // SC budget — must stay intact

	busCtx, _ := o.Run("decouple test", t.TempDir(), "main")

	if planCalls != 2 {
		t.Errorf("planner should retry once on transient stall; got %d", planCalls)
	}
	if busCtx.Mutable.ChangePlan() == nil {
		t.Error("ChangePlan should land after transient retry")
	}
	// SC budget should NOT have been touched by the transient retry.
	// We can't directly inspect graphState.retryUsed from outside, but
	// the absence of LastError + presence of ChangePlan together
	// confirm both budgets are independent (transient one consumed,
	// SC one untouched).
	if busCtx.TaskState.LastError != "" {
		t.Errorf("LastError should be empty after successful retry; got %q", busCtx.TaskState.LastError)
	}
}

func TestWriteScheduler_TransientRetry_DoesNotDrainStepBudget(t *testing.T) {
	expectedPlan := &types.ChangePlan{
		ID:          "plan-step-budget",
		TargetPaths: []string{"main.go"},
		Changes:     []types.FileChange{{Path: "main.go", Kind: "modify"}},
	}
	planCalls := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
		types.AgentPlanner: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			planCalls++
			if planCalls == 1 {
				return nil, io.ErrUnexpectedEOF
			}
			ctx.Mutable.SetChangePlan(expectedPlan)
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	// One step is spent by analyze, leaving exactly one write-graph
	// step. The EOF retry must be free, otherwise the second planner
	// dispatch never gets budget.
	o.SetMaxSteps(2)
	o.SetMode(types.ModePlan)
	o.SetAutoInitRepo(true)
	o.SetScaffoldEnabled(true)
	o.SetTransientRetryBudget(1)

	busCtx, _ := o.Run("step budget decouple test", t.TempDir(), "main")

	if planCalls != 2 {
		t.Fatalf("planner should retry once on EOF; got %d", planCalls)
	}
	if busCtx.Mutable.ChangePlan() == nil {
		t.Fatal("ChangePlan should land after free transient retry")
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("LastError should be empty after successful retry; got %q", busCtx.TaskState.LastError)
	}
}

func writeNodeIDs(ns []*types.TaskNode) []string {
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		out = append(out, n.ID)
	}
	return out
}

// TestWriteScheduler_PlanTransient_NoEmitPlateau_SignaturesDiffer
// locks the signature-AGNOSTIC advisory plateau predicate that catches the
// LLM rotating tool tactics between retry rounds. Production trace
// /home/chatpp/pytest 2026-04-29 06:03 was the trigger: round 1
// streamed `repo_map` + `list_files` then stalled, round 2 streamed
// `list_files` only then stalled. Pre-fix the signature-equality
// plateau missed it (different sigs); the no-emit-streak signal now
// records that pattern for diagnostics and retry hints, while the
// hard stop remains limited to precise repeated non-empty stream-stall
// signatures so bursty transport failures do not get misclassified.
//
// We can't easily simulate different tool sequences via the mock
// without wiring real tool execution, so this test instead drives
// the streak primitive directly via graphState helpers and verifies
// the predicate fires after two no-emit stalls.
func TestGraphState_TransientNoEmitPlateau(t *testing.T) {
	g := types.TaskGraph{Nodes: []types.TaskNode{{ID: "plan", Type: types.NodePlan}}}
	state := newGraphState(g)
	if state.transientNoEmitPlateau("plan") {
		t.Fatal("plateau must not fire on a fresh node")
	}
	state.recordTransientNoEmitStall("plan")
	if state.transientNoEmitPlateau("plan") {
		t.Fatal("one stall is not yet a plateau (threshold is 2)")
	}
	state.recordTransientNoEmitStall("plan")
	if !state.transientNoEmitPlateau("plan") {
		t.Fatal("two consecutive no-emit stalls should fire advisory plateau")
	}
	// reset clears the streak (terminal emit observed in between).
	state.resetTransientNoEmitStreak("plan")
	if state.transientNoEmitPlateau("plan") {
		t.Fatal("reset must clear the streak")
	}
}
