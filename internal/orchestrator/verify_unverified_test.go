package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestVerifyPostHook_RunnerMissingMarksPlanUnverified(t *testing.T) {
	dir := t.TempDir()
	plan := &types.ChangePlan{
		ID:          "plan-runner-missing",
		Status:      types.PlanStatusPending,
		Summary:     "fix python code",
		Request:     "fix",
		TargetPaths: []string{"src/app.py"},
		Changes: []types.FileChange{{
			Path:       "src/app.py",
			Kind:       "modify",
			NewContent: "print('fixed')\n",
		}},
	}
	planPath := dir + "/plan-runner-missing.json"
	if err := types.WritePlanToFile(plan, planPath); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	mu := types.NewMutableState("runner missing")
	mu.SetChangePlan(plan)
	mu.SetChangeReport(&types.ChangeReport{
		PlanID:         plan.ID,
		Channel:        types.ChangeReportChannelPostApplyVerify,
		Passed:         false,
		FailureKind:    types.FailureKindRunnerMissing,
		FailureSummary: "pytest is not installed",
	})
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Mutable:  mu,
			PlanPath: planPath,
			Language: "en",
		},
	}

	if err := verifyPostHook(o, &agent.StageOutput{Error: "verify failed: pytest is not installed"}); err != nil {
		t.Fatalf("verifyPostHook: %v", err)
	}
	reloaded, err := types.LoadChangePlanFromFile(planPath)
	if err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if reloaded.Status != types.PlanStatusUnverified {
		t.Fatalf("status = %q, want %q", reloaded.Status, types.PlanStatusUnverified)
	}
	if reloaded.AppliedAt == nil {
		t.Fatal("runner_missing unverified status should preserve an applied timestamp")
	}
	result := mu.Result()
	if !strings.Contains(result, "Unverified") || !strings.Contains(result, "pytest is not installed") {
		t.Fatalf("result should explain unavailable local verification, got %q", result)
	}
}

func TestVerifyPostHook_MissingReportMarksPlanUnverified(t *testing.T) {
	dir := t.TempDir()
	plan := &types.ChangePlan{
		ID:          "plan-missing-report",
		Status:      types.PlanStatusAppliedPendingVerify,
		Summary:     "fix python code",
		Request:     "fix",
		TargetPaths: []string{"src/app.py"},
		Changes: []types.FileChange{{
			Path:       "src/app.py",
			Kind:       "modify",
			NewContent: "print('fixed')\n",
		}},
	}
	planPath := dir + "/plan-missing-report.json"
	if err := types.WritePlanToFile(plan, planPath); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	mu := types.NewMutableState("missing report")
	mu.SetChangePlan(plan)
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Mutable:  mu,
			PlanPath: planPath,
			Language: "en",
		},
	}

	if err := verifyPostHook(o, &agent.StageOutput{Error: "verifier executor did not install a report"}); err != nil {
		t.Fatalf("verifyPostHook: %v", err)
	}
	reloaded, err := types.LoadChangePlanFromFile(planPath)
	if err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if reloaded.Status != types.PlanStatusUnverified {
		t.Fatalf("status = %q, want %q", reloaded.Status, types.PlanStatusUnverified)
	}
	if reloaded.AppliedAt == nil {
		t.Fatal("missing-report unverified status should preserve an applied timestamp")
	}
	result := mu.Result()
	if !strings.Contains(result, "Unverified") || !strings.Contains(result, "typed verification report") {
		t.Fatalf("result should explain missing typed verify report, got %q", result)
	}
}
