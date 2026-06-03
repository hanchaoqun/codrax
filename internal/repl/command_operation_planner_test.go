package repl

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/operation"
)

func commandOperationPlanResp(payload string) llm.Response {
	return llm.Response{
		ToolCalls: []llm.ToolCall{{
			ID:     "call-op-1",
			Name:   "emit_command_operation_plan",
			Params: []byte(payload),
		}},
	}
}

func TestCommandOperationPlannerCompatJSON(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":"false","work_dir":".","steps":[{"id":"s1","title":"show go version","program":"go","args":"version","timeout_ms":"30000","risk_level":"low","side_effects":""}],}` + "\ntrailing"),
		},
	}
	planner := NewCommandOperationPlanner(adapter)
	req, err := planner.PlanCommandOperation(context.Background(), "查询 go 版本", "/repo", TurnPolicy{
		Operation:     "computer_operation",
		OperationKind: "computer_operation",
		RiskLevel:     "low",
	})
	if err != nil {
		t.Fatalf("PlanCommandOperation: %v", err)
	}
	if len(req.Steps) != 1 {
		t.Fatalf("Steps=%+v", req.Steps)
	}
	if req.Steps[0].Program != "go" || strings.Join(req.Steps[0].Args, " ") != "version" {
		t.Fatalf("step mismatch: %+v", req.Steps[0])
	}
	if req.RequiresConfirmation {
		t.Fatal("requires_confirmation string false was not decoded")
	}
}

func TestCommandOperationPlannerClarification(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"needs_clarification","risk_level":"medium","requires_confirmation":true,"questions":[{"id":"path","question":"Which file should be moved?","suggestions":"provide source path, provide destination path"}]}`),
		},
	}
	planner := NewCommandOperationPlanner(adapter)
	req, err := planner.PlanCommandOperation(context.Background(), "移动文件", "/repo", TurnPolicy{
		Operation:     "computer_operation",
		OperationKind: "computer_operation",
		RiskLevel:     "medium",
	})
	if err != nil {
		t.Fatalf("PlanCommandOperation: %v", err)
	}
	plan := operation.BuildCommandOperationPlan(req, operation.DefaultCommandPolicy())
	if plan.Status != operation.StatusNeedsClarification {
		t.Fatalf("Status=%q", plan.Status)
	}
	if len(plan.ClarifyingQuestions) != 1 || len(plan.ClarifyingQuestions[0].Suggestions) != 2 {
		t.Fatalf("ClarifyingQuestions=%+v", plan.ClarifyingQuestions)
	}
}

func TestCommandOperationPlannerIncludesCapabilitySnapshot(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"needs_clarification","questions":[{"id":"target","question":"target?"}]}`),
		},
	}
	planner, ok := NewCommandOperationPlanner(adapter).(CommandOperationPlannerWithSnapshot)
	if !ok {
		t.Fatal("planner should support capability snapshots")
	}
	snapshot := operation.CapabilitySnapshot{
		RepoRoot: "/repo",
		OS:       "linux",
		Arch:     "amd64",
		Commands: []operation.CapabilityCommand{{
			Name:   "rg",
			Path:   "/usr/bin/rg",
			Source: "look_path",
		}},
		Policy: operation.CapabilityPolicy{
			TimeoutMS:          120000,
			OutputPreviewBytes: 32768,
			UnknownProgram:     operation.ApprovalManual,
			ShellPolicy:        operation.ApprovalManual,
		},
	}
	_, err := planner.PlanCommandOperationWithSnapshot(context.Background(), "查找文件", "/repo", TurnPolicy{
		Operation:     "computer_operation",
		OperationKind: "computer_operation",
		RiskLevel:     "low",
	}, snapshot)
	if err != nil {
		t.Fatalf("PlanCommandOperationWithSnapshot: %v", err)
	}
	if len(adapter.calls) != 1 {
		t.Fatalf("Chat calls=%d, want 1", len(adapter.calls))
	}
	user := ""
	for i := len(adapter.calls[0].messages) - 1; i >= 0; i-- {
		if adapter.calls[0].messages[i].Role == "user" {
			user = adapter.calls[0].messages[i].Content
			break
		}
	}
	for _, want := range []string{
		"## capability_snapshot",
		"os: linux/amd64",
		"available_commands: rg=/usr/bin/rg",
		"查找文件",
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("planner request missing %q:\n%s", want, user)
		}
	}
}

func TestCommandOperationReplannerIncludesFailureContext(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"needs_clarification","risk_level":"medium","requires_confirmation":true,"questions":[{"id":"tool","question":"Which replacement command should be used?"}]}`),
		},
	}
	replanner, ok := NewCommandOperationPlanner(adapter).(CommandOperationReplanner)
	if !ok {
		t.Fatal("planner should support command replanning")
	}
	previous := operation.CommandOperationPlan{
		ID:           "op-prev",
		Status:       operation.StatusReady,
		RiskLevel:    "low",
		ApprovalMode: operation.ApprovalManual,
		WorkDir:      "/repo",
		Steps: []operation.CommandStep{{
			ID:        "s1",
			Title:     "show missing tool version",
			Program:   "missing-tool",
			Args:      []string{"--version"},
			RiskLevel: "low",
		}},
	}
	result := operation.CommandOperationResult{
		PlanID: "op-prev",
		Status: operation.StatusFailed,
		StepResults: []operation.CommandStepResult{{
			StepID:        "s1",
			Status:        operation.StatusFailed,
			ExitCode:      127,
			Error:         "executable file not found",
			OutputPreview: "missing-tool: command not found",
		}},
	}
	_, err := replanner.ReplanCommandOperation(context.Background(), "查询工具版本", "/repo", TurnPolicy{
		Operation:     "computer_operation",
		OperationKind: "computer_operation",
		RiskLevel:     "low",
	}, operation.CapabilitySnapshot{}, previous, result)
	if err != nil {
		t.Fatalf("ReplanCommandOperation: %v", err)
	}
	if len(adapter.calls) != 1 {
		t.Fatalf("Chat calls=%d, want 1", len(adapter.calls))
	}
	user := ""
	for i := len(adapter.calls[0].messages) - 1; i >= 0; i-- {
		if adapter.calls[0].messages[i].Role == "user" {
			user = adapter.calls[0].messages[i].Content
			break
		}
	}
	for _, want := range []string{
		"## replan_context",
		"previous_plan id=op-prev",
		"previous_result plan_id=op-prev status=failed",
		"executable file not found",
		"missing-tool: command not found",
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("replan request missing %q:\n%s", want, user)
		}
	}
}
