package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestPlanMode_E2E_StubPlannerProducesChangePlan is the session-33
// B0 Day-5 T9 acceptance test. Locks the end-to-end plan-mode
// behavior: when SetMode(ModePlan) is in effect and the planner
// agent successfully installs a ChangePlan on Mutable via emit_
// change_plan, the orchestrator runPlanPhase picks up the plan,
// renders a human-visible summary into Result, and returns success.
//
// The planner is stubbed here (mock agent pre-populates Mutable.
// ChangePlan) because real LLM calls have no place in a unit test.
// The integration surface that matters — runPlanPhase reads the
// plan, runSingleShot can serialise it to disk — is exercised
// deterministically.
func TestPlanMode_E2E_StubPlannerProducesChangePlan(t *testing.T) {
	// Seed a ChangePlan that the stub planner will install.
	expectedPlan := &types.ChangePlan{
		ID:            "plan-e2e-test",
		Request:       "add a comment to main.go",
		Summary:       "Stub plan — adds a one-line header comment to main.go to prove the plan-mode dispatch chain.",
		Status:        "pending_approval",
		Changes: []types.FileChange{
			{Path: "main.go", Kind: "modify", NewContent: "// codrax\n", Rationale: "header comment"},
		},
		TargetPaths:     []string{"main.go"},
		AcceptanceTests: []string{"go build ./... passes"},
	}

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{
			Language:            "en",
		})),
		// The stub planner directly installs the ChangePlan on
		// Mutable — mimicking what emit_change_plan's Execute would
		// do in a real LLM-driven run. runPlanPhase then reads it
		// back and renders a summary to Result.
		types.AgentPlanner: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
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

	busCtx, err := o.Run("add a comment to main.go", t.TempDir(), "main")
	if err != nil {
		t.Fatalf("orch.Run: %v", err)
	}
	if busCtx == nil {
		t.Fatal("busCtx should not be nil post-Run")
	}

	// Mode stays locked at ModePlan through Run.
	if busCtx.Mode != types.ModePlan {
		t.Errorf("Mode = %q, want ModePlan", busCtx.Mode)
	}

	// No error path was triggered.
	if busCtx.TaskState.LastError != "" {
		t.Errorf("unexpected LastError: %q", busCtx.TaskState.LastError)
	}

	// ChangePlan was installed and survives to post-Run inspection.
	plan := busCtx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("ChangePlan should be installed on Mutable after plan mode")
	}
	if plan.ID != expectedPlan.ID {
		t.Errorf("plan ID = %q, want %q", plan.ID, expectedPlan.ID)
	}
	if len(plan.Changes) != 1 {
		t.Errorf("plan changes = %d, want 1", len(plan.Changes))
	}

	// Result renders a human-readable summary mentioning the plan ID
	// and the target file. This is what cmd/root.go prints to stdout
	// in single-shot mode.
	result := busCtx.Mutable.Result()
	if !strings.Contains(result, expectedPlan.ID) {
		t.Errorf("Result should mention plan ID %q; got %q", expectedPlan.ID, result)
	}
	if !strings.Contains(result, "main.go") {
		t.Errorf("Result should mention the target file 'main.go'; got %q", result)
	}
	// renderChangePlanSummary now follows BusContext.Language;
	// fixture leaves it empty which maps to zh-default. Accept
	// either localised heading.
	if !strings.Contains(result, "Proposed change plan") && !strings.Contains(result, "提议的 ChangePlan") {
		t.Errorf("Result should include the change-plan header (zh or en); got first 200 chars: %q",
			result[:min(200, len(result))])
	}

	// Terminal + Phase reached their expected final shape.
	if !busCtx.TaskState.IsTerminal {
		t.Error("TaskState.IsTerminal should be true post-Run")
	}
	if busCtx.PipelineStage != types.StagePlan {
		t.Errorf("PipelineStage = %q, want StagePlan", busCtx.PipelineStage)
	}
}

// TestPlanMode_E2E_FailsCleanlyWhenPlannerSkipsEmit locks the
// fail-loud contract: if the planner returns success but never
// installs a ChangePlan (LLM didn't call emit_change_plan), the
// orchestrator surfaces a clear error rather than silently producing
// an empty result.
func TestPlanMode_E2E_FailsCleanlyWhenPlannerSkipsEmit(t *testing.T) {
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
		types.AgentPlanner: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			// Success signal but no plan installed — represents the
			// case where the planner's LLM returned content but
			// didn't call emit_change_plan.
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModePlan)
	o.SetAutoInitRepo(true) // plan stage's new bare-dir gate; tests run against tmp dirs
	o.SetScaffoldEnabled(true)

	busCtx, err := o.Run("doomed request", t.TempDir(), "main")
	if err != nil {
		t.Fatalf("orch.Run: %v", err)
	}
	if busCtx.TaskState.LastError == "" {
		t.Error("expected LastError to be populated when ChangePlan is missing")
	}
	if !strings.Contains(busCtx.TaskState.LastError, "ChangePlan") &&
		!strings.Contains(busCtx.TaskState.LastError, "emit_change_plan") {
		t.Errorf("LastError should mention ChangePlan or emit_change_plan; got %q",
			busCtx.TaskState.LastError)
	}
	if busCtx.Mutable.ChangePlan() != nil {
		t.Error("ChangePlan should be nil when planner skipped emit")
	}
}

// TestPlanMode_E2E_PlannerErrorSurfaces locks that an error-carrying
// StageOutput from the planner flows to TaskState.LastError cleanly.
func TestPlanMode_E2E_PlannerErrorSurfaces(t *testing.T) {
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
		types.AgentPlanner: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				Error:        "planner: synthetic test error for plan-mode dispatch",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModePlan)
	o.SetAutoInitRepo(true) // plan stage's new bare-dir gate; tests run against tmp dirs
	o.SetScaffoldEnabled(true)

	busCtx, err := o.Run("doomed request", t.TempDir(), "main")
	if err != nil {
		t.Fatalf("orch.Run: %v", err)
	}
	if busCtx.TaskState.LastError == "" {
		t.Fatal("LastError should be populated from planner error")
	}
	if !strings.Contains(busCtx.TaskState.LastError, "synthetic test error") {
		t.Errorf("LastError should carry planner's error message; got %q",
			busCtx.TaskState.LastError)
	}
}

// min is a local helper (Go < 1.21 compatibility shim; Go 1.21+
// provides a builtin). Used only by the result-snippet bounds in
// TestPlanMode_E2E_StubPlannerProducesChangePlan.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
