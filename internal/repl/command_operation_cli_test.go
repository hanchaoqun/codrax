package repl

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/operation"
)

type fakeCLICommandPlanner struct {
	req operation.CommandOperationRequest
}

func (p fakeCLICommandPlanner) PlanCommandOperation(ctx context.Context, userLine, repoRoot string, policy TurnPolicy) (operation.CommandOperationRequest, error) {
	return p.req, nil
}

func (p fakeCLICommandPlanner) PlanCommandOperationWithSnapshot(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, snapshot operation.CapabilitySnapshot) (operation.CommandOperationRequest, error) {
	return p.req, nil
}

func (p fakeCLICommandPlanner) AnswerCommandOperationRecords(ctx context.Context, userLine string, records []commandOperationResultRecord, lang string) (string, error) {
	if len(records) == 0 {
		return "no records", nil
	}
	return "final: " + strings.TrimSpace(records[len(records)-1].Result.OutputPreview), nil
}

func TestOperationPlannerErrorsUseTypedCodes(t *testing.T) {
	noTool := newOperationPlannerNoToolError("command operation planner", nil)
	if !operationPlannerErrorHasCode(noTool, operationPlannerErrorNoToolCall) {
		t.Fatal("typed no-tool operation planner error not recognized")
	}
	wrapped := newOperationPlannerUnexpectedToolError("command operation planner", "wrong_tool", noTool)
	if !operationPlannerErrorHasCode(wrapped, operationPlannerErrorUnexpectedTool) {
		t.Fatal("typed unexpected-tool operation planner error not recognized")
	}
	if operationPlannerErrorHasCode(errors.New("command operation planner: LLM returned no tool_call"), operationPlannerErrorNoToolCall) {
		t.Fatal("plain operation error text must not satisfy typed no-tool code")
	}
}

func TestRunCommandOperationCLI_ExecutesAutoPlanAndReturnsFinalAnswer(t *testing.T) {
	t.Parallel()
	policy := operation.DefaultCommandPolicy()
	policy.DefaultWorkDir = t.TempDir()
	req := operation.CommandOperationRequest{
		Text:      "query local environment",
		WorkDir:   policy.DefaultWorkDir,
		RiskLevel: "low",
		Goal:      "query local environment",
		Steps: []operation.CommandStep{{
			ID:        "info",
			Title:     "print fixture",
			Shell:     "printf cli-operation-ok",
			RiskLevel: "low",
		}},
	}
	var progress bytes.Buffer
	answer, err := RunCommandOperationCLI(context.Background(), req.Text, TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "computer_operation",
		OperationKind:        "computer_operation",
		Source:               "current_message",
		RiskLevel:            "low",
	}, CommandOperationCLIConfig{
		Planner:       fakeCLICommandPlanner{req: req},
		Policy:        policy,
		RepoRoot:      policy.DefaultWorkDir,
		RuntimeAnchor: t.TempDir(),
		Language:      "zh",
		Progress:      &progress,
	})
	if err != nil {
		t.Fatalf("RunCommandOperationCLI returned error: %v", err)
	}
	if !strings.Contains(answer, "cli-operation-ok") {
		t.Fatalf("final answer missing command observation:\n%s", answer)
	}
	if out := progress.String(); !strings.Contains(out, "操作计划") || !strings.Contains(out, "cli-operation-ok") {
		t.Fatalf("progress should include plan and execution result, got:\n%s", out)
	}
}

func TestCommandOperationCLIFinalAnswerPreservesBudgetTerminalStatus(t *testing.T) {
	t.Parallel()
	records := []commandOperationResultRecord{{
		Plan: operation.CommandOperationPlan{
			ID:     "op-budget",
			Status: operation.StatusExecuted,
		},
		Result: operation.CommandOperationResult{
			PlanID:        "op-budget",
			Status:        operation.StatusBudgetExhausted,
			OutputPreview: "command operation command-round budget exhausted before the user goal was fully satisfied",
		},
	}}
	answer := commandOperationFinalMessageCLI(context.Background(), CommandOperationCLIConfig{
		Planner:  fakeCLICommandPlanner{},
		Language: "zh",
	}, "读取长网页并总结", records)
	if !strings.Contains(answer, "状态：部分结果") || !strings.Contains(answer, "预算上限") {
		t.Fatalf("budget terminal status must be visible before model summary:\n%s", answer)
	}
	if !strings.Contains(answer, "final:") {
		t.Fatalf("model summary should still be preserved after status prefix:\n%s", answer)
	}
}

func TestOperationFinalAnswerWarnsOnUncoveredMaterialRef(t *testing.T) {
	t.Parallel()
	records := []commandOperationResultRecord{{
		Plan: operation.CommandOperationPlan{ID: "op-fetch", Status: operation.StatusExecuted},
		Result: operation.CommandOperationResult{
			PlanID:     "op-fetch",
			Status:     operation.StatusExecuted,
			PayloadRef: "/tmp/manual.html",
		},
	}}
	answer := operationFinalReportWithRecordStatus("zh", "任务完成", records)
	if !strings.Contains(answer, "材料覆盖未完全验证") {
		t.Fatalf("uncovered payload ref should surface material coverage caveat:\n%s", answer)
	}
}

func TestOperationFinalAnswerNoMaterialWarningWhenPayloadExcerptExists(t *testing.T) {
	t.Parallel()
	payload := filepath.Join(t.TempDir(), "manual.txt")
	if err := os.WriteFile(payload, []byte("section one\nsection two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	records := []commandOperationResultRecord{{
		Plan: operation.CommandOperationPlan{ID: "op-fetch", Status: operation.StatusExecuted},
		Result: operation.CommandOperationResult{
			PlanID:     "op-fetch",
			Status:     operation.StatusExecuted,
			PayloadRef: payload,
		},
	}}
	answer := operationFinalReportWithRecordStatus("zh", "任务完成", records)
	if strings.Contains(answer, "材料覆盖未完全验证") {
		t.Fatalf("text payload excerpt should satisfy material coverage caveat:\n%s", answer)
	}
}

func TestOperationFinalAnswerNoMaterialWarningWhenLaterStepConsumesRef(t *testing.T) {
	t.Parallel()
	records := []commandOperationResultRecord{{
		Plan: operation.CommandOperationPlan{ID: "op-fetch", Status: operation.StatusExecuted},
		Result: operation.CommandOperationResult{
			PlanID:     "op-fetch",
			Status:     operation.StatusExecuted,
			PayloadRef: "/tmp/manual.html",
		},
	}, {
		Plan: operation.CommandOperationPlan{
			ID:     "op-extract",
			Status: operation.StatusExecuted,
			Steps:  []operation.CommandStep{{ID: "extract", Shell: "sed -n '/<article/,/<\\/article>/p' /tmp/manual.html"}},
		},
		Result: operation.CommandOperationResult{
			PlanID: "op-extract",
			Status: operation.StatusExecuted,
		},
	}}
	answer := operationFinalReportWithRecordStatus("zh", "任务完成", records)
	if strings.Contains(answer, "材料覆盖未完全验证") {
		t.Fatalf("consumed payload ref should not surface material coverage caveat:\n%s", answer)
	}
}
