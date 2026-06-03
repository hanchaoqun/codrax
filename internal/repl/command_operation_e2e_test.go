package repl

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/operation"
)

func commandOperationPolicy(risk string) TurnPolicy {
	return TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "computer_operation",
		OperationKind:        "computer_operation",
		Source:               "current_message",
		RiskLevel:            risk,
		TargetSurface:        "desktop",
		Confidence:           0.9,
		Reason:               "user asked for a computer operation",
	}
}

func TestCommandOperationE2E_NeedsClarificationDoesNotEnterSourcePipeline(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("medium")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{commandOperationPlanResp(`{"status":"needs_clarification","risk_level":"medium","requires_confirmation":true,"questions":[{"id":"paths","question":"Which source and destination paths should be used?","suggestions":["provide the source path","provide the destination path"]}]}`)},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "帮我移动一个文件\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("clarification should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation != nil {
		t.Fatalf("clarification should not create a pending operation: %+v", r.pendingOperation)
	}
	printed := out.String()
	if !strings.Contains(printed, "Which source and destination paths should be used?") {
		t.Fatalf("clarification question missing:\n%s", printed)
	}
	if strings.Contains(printed, "completed") {
		t.Fatalf("clarification must not execute anything:\n%s", printed)
	}
}

func TestCommandOperationE2E_AutoLowRiskExecutesWithoutApprove(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`)},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "查询 go 版本\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.operationPolicy.AutoLowRisk = true
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("auto operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation != nil {
		t.Fatalf("auto operation should not remain pending: %+v", r.pendingOperation)
	}
	printed := out.String()
	if !strings.Contains(printed, "completed") || !strings.Contains(printed, "go version") {
		t.Fatalf("auto operation result missing:\n%s", printed)
	}
	if strings.Contains(printed, "awaiting approval") {
		t.Fatalf("auto low-risk operation should not ask for approval:\n%s", printed)
	}
}

func TestCommandOperationE2E_HardDeniedDestructiveCommandDoesNotExecute(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("high")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{commandOperationPlanResp(`{"status":"ready","risk_level":"high","requires_confirmation":true,"work_dir":".","steps":[{"id":"s1","title":"destructive remove","program":"rm","args":["-rf","/"],"risk_level":"high","side_effects":["delete files"]}]}`)},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "删除所有文件\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("blocked operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation != nil {
		t.Fatalf("blocked operation should not remain pending: %+v", r.pendingOperation)
	}
	printed := out.String()
	if !strings.Contains(printed, "blocked") {
		t.Fatalf("blocked plan message missing:\n%s", printed)
	}
	if strings.Contains(printed, "completed") {
		t.Fatalf("blocked destructive operation must not execute:\n%s", printed)
	}
}

func TestCommandOperationE2E_UnknownProgramRequiresManualApprovalEvenWhenAutoEnabled(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"run custom tool","program":"corp-custom-tool","args":["--version"],"risk_level":"low","side_effects":[]}]}`)},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "运行内部工具查询版本\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.operationPolicy.AutoLowRisk = true
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("manual unknown program should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation == nil {
		t.Fatal("unknown program should wait for manual approval, not auto-execute")
	}
	printed := out.String()
	if !strings.Contains(printed, "Run `/approve`") {
		t.Fatalf("manual approval message missing:\n%s", printed)
	}
	if strings.Contains(printed, "completed") {
		t.Fatalf("unknown program should not execute before approval:\n%s", printed)
	}
}

func TestCommandOperationE2E_PlannerRequestCarriesPolicySignals(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"needs_clarification","questions":[{"id":"target","question":"target?"}]}`),
		},
	}
	planner := NewCommandOperationPlanner(adapter)
	_, err := planner.PlanCommandOperation(context.Background(), "安装工具", "/repo", commandOperationPolicy("medium"))
	if err != nil {
		t.Fatalf("PlanCommandOperation: %v", err)
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
		"operation_kind=computer_operation",
		"risk=medium",
		"## repo_root\n/repo",
		"安装工具",
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("planner request missing %q:\n%s", want, user)
		}
	}
}
