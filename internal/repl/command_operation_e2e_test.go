package repl

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/operation"
	"github.com/hanchaoqun/codrax/internal/render"
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

func TestCommandOperationE2E_OperationMemoryFeedsPlannerOnlyOnOperationRoute(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"use remembered command","program":"demo-tool","args":["--input","a.txt"],"risk_level":"low","side_effects":[]}]}`)},
	}
	r, runner, _ := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.runtimeAnchor = t.TempDir()
	r.operationMemory = operation.NewMemoryStore(filepath.Join(r.runtimeAnchor, "operation", "memory.jsonl"))
	if err := r.operationMemory.Append(operation.MemoryEntry{
		Workspace:  r.commandOperationCapabilitySnapshot().RepoRoot,
		OS:         r.commandOperationCapabilitySnapshot().OS,
		Arch:       r.commandOperationCapabilitySnapshot().Arch,
		Capability: "computer_operation",
		Command:    "demo-tool --input a.txt",
		Outcome:    "executed",
		Lessons:    []string{"demo-tool worked with --input"},
	}); err != nil {
		t.Fatalf("append operation memory: %v", err)
	}

	r.operationDispatch("使用 demo-tool 处理 a.txt", "使用 demo-tool 处理 a.txt", commandOperationPolicy("low"))

	if len(runner.requests) != 0 {
		t.Fatalf("operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(adapter.calls) != 1 {
		t.Fatalf("planner calls=%d want 1", len(adapter.calls))
	}
	all := ""
	for _, msg := range adapter.calls[0].messages {
		all += "\n" + msg.Content
	}
	for _, want := range []string{
		"## operation_memory",
		"demo-tool worked with --input",
		"not source evidence",
	} {
		if !strings.Contains(all, want) {
			t.Fatalf("operation memory prompt missing %q:\n%s", want, all)
		}
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

func TestCommandOperationE2E_ClarificationAnswerResumesPlanning(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("medium")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"needs_clarification","risk_level":"medium","requires_confirmation":true,"questions":[{"id":"paths","question":"Which source and destination paths should be used?","suggestions":["provide the source path","provide the destination path"]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"medium","requires_confirmation":true,"work_dir":".","steps":[{"id":"s1","title":"move file","program":"mv","args":["a.txt","b.txt"],"risk_level":"medium","side_effects":["local_file_write"],"verify_hint":"path_exists:b.txt"}]}`),
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "帮我移动一个文件\n源是 a.txt，目标是 b.txt\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("clarification resume should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("planner calls=%d want 2", len(adapter.calls))
	}
	if r.pendingCommandClarification != nil {
		t.Fatalf("clarification should be cleared after ready plan: %+v", r.pendingCommandClarification)
	}
	if r.pendingOperation == nil {
		t.Fatal("ready resumed plan should be pending approval")
	}
	if got := r.pendingOperation.Steps[0].Program; got != "mv" {
		t.Fatalf("resumed plan program=%q want mv", got)
	}
	if !strings.Contains(out.String(), "Operation plan") && !strings.Contains(out.String(), "操作计划") {
		t.Fatalf("ready operation plan not rendered:\n%s", out.String())
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

func TestCommandOperationE2E_StopsPrearmedRendererSpinner(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`)},
	}
	r, runner, _ := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	var dock bytes.Buffer
	r.renderer = render.New(&dock, true)
	r.renderer.StartSpinner()
	t.Cleanup(func() {
		if r.renderer.SpinnerActive() {
			r.renderer.StopSpinner()
		}
	})

	r.operationDispatch("查询 go 版本", "查询 go 版本", commandOperationPolicy("low"))

	if r.renderer.SpinnerActive() {
		t.Fatal("operation planning must close the pre-armed classifier spinner before rendering the plan")
	}
	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation == nil {
		t.Fatal("manual command plan should remain pending approval")
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

func TestCommandOperationE2E_FailedApprovedCommandCreatesRevisedPlan(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("medium")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"medium","requires_confirmation":true,"work_dir":".","steps":[{"id":"s1","title":"show missing tool version","program":"definitely-missing-codrax-command","args":["--version"],"risk_level":"medium","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"show go version instead","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`),
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "查询工具版本\n/approve\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("expected initial plan + replan calls, got %d", len(adapter.calls))
	}
	if r.pendingOperation == nil {
		t.Fatal("revised plan should wait for manual approval when auto-low-risk is disabled")
	}
	if got := r.pendingOperation.Steps[0].Program; got != "go" {
		t.Fatalf("revised pending program=%q, want go", got)
	}
	printed := out.String()
	for _, want := range []string{
		"Operation plan",
		"failed",
		"revised command plan",
		"go version",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("replan output missing %q:\n%s", want, printed)
		}
	}
}

func TestDropRepeatedFailedCommandStepsRemovesOnlyFailedRetry(t *testing.T) {
	failedPlan := operation.CommandOperationPlan{
		ID: "op-failed",
		Steps: []operation.CommandStep{{
			ID:      "missing",
			Program: "definitely-missing-codrax-command",
			Args:    []string{"--version"},
		}, {
			ID:      "ok",
			Program: "pwd",
		}},
	}
	result := operation.CommandOperationResult{
		PlanID: "op-failed",
		Status: operation.StatusFailed,
		StepResults: []operation.CommandStepResult{{
			StepID: "missing",
			Status: operation.StatusFailed,
		}, {
			StepID: "ok",
			Status: operation.StatusExecuted,
		}},
	}
	revised := operation.CommandOperationRequest{
		RequiresConfirmation: true,
		Steps: []operation.CommandStep{{
			ID:      "retry-missing",
			Program: "definitely-missing-codrax-command",
			Args:    []string{"--version"},
		}, {
			ID:      "next",
			Program: "go",
			Args:    []string{"version"},
		}, {
			ID:      "repeat-ok",
			Program: "pwd",
		}},
	}
	filtered := dropRepeatedFailedCommandSteps(revised, failedPlan, result)
	if len(filtered.Steps) != 2 {
		t.Fatalf("filtered steps=%+v, want 2 steps", filtered.Steps)
	}
	if filtered.Steps[0].Program != "go" || filtered.Steps[1].Program != "pwd" {
		t.Fatalf("wrong steps filtered: %+v", filtered.Steps)
	}
}

func TestCommandReplanAutoExecuteEnvelope(t *testing.T) {
	base := operation.CommandOperationPlan{
		Status:       operation.StatusFailed,
		RiskLevel:    "medium",
		ApprovalMode: operation.ApprovalManual,
		WorkDir:      "/repo",
	}
	okPlan := operation.CommandOperationPlan{
		Status:       operation.StatusReady,
		RiskLevel:    "low",
		ApprovalMode: operation.ApprovalAutoLowRisk,
		WorkDir:      "/repo",
		Steps: []operation.CommandStep{{
			ID:           "s1",
			Program:      "go",
			Args:         []string{"version"},
			RiskLevel:    "low",
			AutoApproval: operation.StepAutoEligible,
		}},
	}
	if !commandReplanCanAutoExecute(base, okPlan) {
		t.Fatal("same-dir lower-risk read-only eligible replan should be allowed to auto-continue")
	}
	clone := func(plan operation.CommandOperationPlan) operation.CommandOperationPlan {
		plan.Steps = append([]operation.CommandStep(nil), plan.Steps...)
		return plan
	}
	changedDir := clone(okPlan)
	changedDir.WorkDir = "/tmp"
	if commandReplanCanAutoExecute(base, changedDir) {
		t.Fatal("changed workdir must require manual approval")
	}
	withShell := clone(okPlan)
	withShell.Steps[0].Shell = "go version"
	if commandReplanCanAutoExecute(base, withShell) {
		t.Fatal("shell replan must require manual approval")
	}
	withSideEffect := clone(okPlan)
	withSideEffect.Steps[0].SideEffects = []string{"local_file_write"}
	if commandReplanCanAutoExecute(base, withSideEffect) {
		t.Fatal("side-effecting replan must require manual approval")
	}
	escalated := clone(okPlan)
	escalated.RiskLevel = "high"
	if commandReplanCanAutoExecute(base, escalated) {
		t.Fatal("risk-escalating replan must require manual approval")
	}
}
