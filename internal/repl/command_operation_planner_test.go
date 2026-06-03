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
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":"false","continue_after":"true","work_dir":".","steps":[{"id":1,"title":"show go version","program":"go","args":"version","timeout_ms":"30000","risk_level":"low","side_effects":""}],}` + "\ntrailing"),
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
	if req.Steps[0].ID != "1" {
		t.Fatalf("numeric step id was not decoded as string: %+v", req.Steps[0])
	}
	if req.RequiresConfirmation {
		t.Fatal("requires_confirmation string false was not decoded")
	}
	if !req.ContinueAfter {
		t.Fatal("continue_after string true was not decoded")
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
		OperationProviders: []operation.ProviderInfo{{
			Name:        "mcp:slides",
			Kind:        "presentation_generation",
			Surfaces:    []string{"slides"},
			ToolName:    "run_operation",
			Description: "Create local slide decks.",
			Source:      "mcp",
		}},
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
		"operation_providers",
		"mcp:slides kind=presentation_generation",
		"description=Create local slide decks.",
		"查找文件",
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("planner request missing %q:\n%s", want, user)
		}
	}
}

func TestCommandOperationPlannerIncludesRecentOperationHandoff(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"use discovered option","program":"demo-tool","args":["--input","a.txt","--format","json"],"risk_level":"low","side_effects":[]}]}`),
		},
	}
	planner, ok := NewCommandOperationPlanner(adapter).(CommandOperationPlannerWithHandoff)
	if !ok {
		t.Fatal("planner should support operation handoff context")
	}
	handoff := `operation_result plan_id=op-help status=executed request="查看 demo-tool 帮助"
  step id=s1 cmd="demo-tool --help" status=executed exit_code=0 timed_out=false error="" output_preview="Usage: demo-tool --input FILE --format json" payload_ref=/tmp/codrax-operation/help.txt
operation_result plan_id=op-extract status=executed request="从大文件里提取关键错误"
  step id=s1 cmd="rg ERROR huge.log" status=executed exit_code=0 timed_out=false error="" output_preview="large_file_summary: ERROR E42 appears around line 8842; next inspect --context 20" payload_ref=/tmp/codrax-operation/huge-log-errors.txt`
	_, err := planner.PlanCommandOperationWithHandoff(context.Background(), "用 demo-tool 输出 json", "/repo", TurnPolicy{
		Operation:     "computer_operation",
		OperationKind: "computer_operation",
		RiskLevel:     "low",
	}, operation.CapabilitySnapshot{}, handoff)
	if err != nil {
		t.Fatalf("PlanCommandOperationWithHandoff: %v", err)
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
		"## recent_operation_context",
		"demo-tool --help",
		"Usage: demo-tool --input FILE --format json",
		"payload_ref=/tmp/codrax-operation/help.txt",
		"large_file_summary: ERROR E42 appears around line 8842",
		"payload_ref=/tmp/codrax-operation/huge-log-errors.txt",
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("planner request missing %q:\n%s", want, user)
		}
	}
	allMessages := ""
	for _, msg := range adapter.calls[0].messages {
		allMessages += "\n" + msg.Content
	}
	if !strings.Contains(allMessages, "For extraction requests, shape the command output") {
		t.Fatalf("planner prompt missing extraction-output shaping guidance:\n%s", allMessages)
	}
}

func TestCommandOperationPlannerHandlesPayloadRefOnlyHandoff(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"inspect payload ref","program":"head","args":["-100","/tmp/codrax-operation/big.txt"],"risk_level":"low","side_effects":[]}]}`),
		},
	}
	planner, ok := NewCommandOperationPlanner(adapter).(CommandOperationPlannerWithHandoff)
	if !ok {
		t.Fatal("planner should support operation handoff context")
	}
	handoff := `operation_result plan_id=op-big status=executed request="读取大文件"
  step id=s1 cmd="cat huge.txt" status=executed exit_code=0 timed_out=false failure_class= verification_status= verification_kind= verification_summary="" output_kind=large_output_summary output_summary="large output captured; inspect payload_ref with bounded reads" error="" output_preview="" payload_ref=/tmp/codrax-operation/big.txt`
	_, err := planner.PlanCommandOperationWithHandoff(context.Background(), "继续从大输出里找 ERROR42", "/repo", TurnPolicy{
		Operation:     "computer_operation",
		OperationKind: "computer_operation",
		RiskLevel:     "low",
	}, operation.CapabilitySnapshot{}, handoff)
	if err != nil {
		t.Fatalf("PlanCommandOperationWithHandoff: %v", err)
	}
	combined := ""
	for _, msg := range adapter.calls[0].messages {
		combined += "\n" + msg.Content
	}
	for _, want := range []string{
		"payload_ref=/tmp/codrax-operation/big.txt",
		"If only a payload_ref is available",
		"bounded follow-up read/search/summarize",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("planner prompt missing %q:\n%s", want, combined)
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
			FailureClass:  "command_not_found",
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
		"Do not include the failed command again",
		"failure_class=command_not_found",
		"executable file not found",
		"missing-tool: command not found",
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("replan request missing %q:\n%s", want, user)
		}
	}
}

func TestCommandOperationPlannerPromptForbidsNoopFactCommands(t *testing.T) {
	for _, want := range []string{
		"capability_snapshot is advisory context",
		"real read-only commands that retrieve the requested facts",
		"Do not use echo, printf, comments, or no-op placeholder commands",
		"emit needs_clarification or plan a safe discovery command",
		"Prefer iterative discovery over one-shot guessing",
		"continue_after=true",
		"For continuation requests",
		"emit status=complete",
		"Every filter command must have an explicit input source",
		`Do not emit bare filters such as "grep pattern"`,
		"collect both lanes when available",
		"runtime state/process/service evidence and application/package metadata",
		"Do not conclude absence from one empty process filter",
		"do not ask the user for information that can still be safely discovered",
		"Unknown facts are a reason to run another safe discovery batch",
		"Ask the user only for user-owned inputs",
	} {
		if !strings.Contains(commandOperationPlannerSystemPrompt, want) {
			t.Fatalf("planner prompt missing %q:\n%s", want, commandOperationPlannerSystemPrompt)
		}
	}
}

func TestCommandOperationContinuationEmptyClarificationCompletes(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"needs_clarification","risk_level":"low","requires_confirmation":false}`),
		},
	}
	planner, ok := NewCommandOperationPlanner(adapter).(CommandOperationContinuationPlanner)
	if !ok {
		t.Fatal("planner should support command continuation")
	}
	cont, err := planner.ContinueCommandOperation(context.Background(), "查询 VPN 软件和版本", "/repo", TurnPolicy{
		Operation:     "computer_operation",
		OperationKind: "computer_operation",
		RiskLevel:     "low",
	}, operation.CapabilitySnapshot{}, []commandOperationResultRecord{{
		Plan: operation.CommandOperationPlan{ID: "op-1", Status: operation.StatusExecuted},
		Result: operation.CommandOperationResult{
			PlanID: "op-1",
			Status: operation.StatusExecuted,
			StepResults: []operation.CommandStepResult{{
				StepID:        "s1",
				Status:        operation.StatusExecuted,
				OutputPreview: "utun4 VPN server 127.0.0.1",
			}},
		},
	}})
	if err != nil {
		t.Fatalf("ContinueCommandOperation: %v", err)
	}
	if !cont.Complete {
		t.Fatalf("empty clarification should fall back to final synthesis, got %+v", cont)
	}
	if cont.Request.Text != "" || len(cont.Request.ClarifyingQuestions) != 0 {
		t.Fatalf("empty clarification must not synthesize a generic user question: %+v", cont.Request)
	}
}

func TestCommandOperationContinuationComplete(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"complete","risk_level":"low","requires_confirmation":false,"block_reason":"enough observations"}`),
		},
	}
	planner, ok := NewCommandOperationPlanner(adapter).(CommandOperationContinuationPlanner)
	if !ok {
		t.Fatal("planner should support command continuation")
	}
	cont, err := planner.ContinueCommandOperation(context.Background(), "查询 go 信息", "/repo", TurnPolicy{
		Operation:     "computer_operation",
		OperationKind: "computer_operation",
		RiskLevel:     "low",
	}, operation.CapabilitySnapshot{}, []commandOperationResultRecord{{
		Plan: operation.CommandOperationPlan{ID: "op-1", Status: operation.StatusExecuted},
		Result: operation.CommandOperationResult{
			PlanID: "op-1",
			Status: operation.StatusExecuted,
			StepResults: []operation.CommandStepResult{{
				StepID:        "s1",
				Status:        operation.StatusExecuted,
				OutputPreview: "go version go1.22.5 darwin/arm64",
			}},
		},
	}})
	if err != nil {
		t.Fatalf("ContinueCommandOperation: %v", err)
	}
	if !cont.Complete || cont.Reason != "enough observations" {
		t.Fatalf("continuation=%+v", cont)
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
		"## continuation_context",
		"emit status=complete",
		"can still be discovered by safe read-only commands",
		"go version go1.22.5",
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("continuation prompt missing %q:\n%s", want, user)
		}
	}
}

func TestCommandOperationAnswerPromptForbidsVisibleReasoning(t *testing.T) {
	for _, want := range []string{
		"In this final operation report, do not output hidden reasoning",
		"chain-of-thought",
		"<think>",
		"meta commentary",
		"Output only the final user-facing report",
		"every user-visible sentence must be Chinese",
	} {
		if !strings.Contains(commandOperationAnswerSystemPrompt, want) {
			t.Fatalf("answer prompt missing %q:\n%s", want, commandOperationAnswerSystemPrompt)
		}
	}
}
