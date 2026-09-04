package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/operation"
)

// TestCommandOperationE2E_ApproveAnswersAgainstPlanRequestText pins the
// approval arm through the real REPL path: a request whose plan requires
// confirmation parks in pendingOperation, the user types /approve, and the
// answer synthesis model's `## user_request` receives the request the plan
// was built from (plan.RequestText — the user's own wording) while the
// memory turn keeps "/approve" as the display form. EVOLUTION RECORD
// (batch six fold-in, review round three #5): handleOperationApproveCmd
// called executeCommandOperationPlan(plan, "/approve", "/approve") since
// fa9a90132, so every approval-gated plan was answered against the slash
// command and the user's wording reached neither the answerer nor
// RequestForSummary — red on 480939385 (`## user_request = "/approve"`).
func TestCommandOperationE2E_ApproveAnswersAgainstPlanRequestText(t *testing.T) {
	const request = "请把 go 工具链版本打印出来"
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("medium")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"medium","requires_confirmation":true,"work_dir":".","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"medium","side_effects":[]}]}`),
			{Content: "Go 版本已打印。", StopReason: "end_turn"},
		},
	}
	r, runner, _ := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, request+"\n/approve\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.operationPolicy.AutoApprove = false
	r.operationPolicy.AutoLowRisk = false
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("approved operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("planner+answer calls=%d, want 2 (the plan must park, then run on /approve)", len(adapter.calls))
	}
	if r.pendingOperation != nil {
		t.Fatalf("approved plan must not stay pending: %+v", r.pendingOperation)
	}
	answerUser := lastUserRoleContent(adapter.calls[1].messages)
	if got := userRequestSection(answerUser); got != request {
		t.Fatalf("answer synthesis ## user_request = %q, want the plan's request text %q (the /approve display form must never reach the model)\n%s", got, request, answerUser)
	}
	recent := store.Recent()
	if len(recent) == 0 {
		t.Fatal("expected the approved operation turn in memory")
	}
	last := recent[len(recent)-1]
	if last.Request != "/approve" {
		t.Fatalf("memory turn Request = %q, want the display form %q", last.Request, "/approve")
	}
	if last.RequestForSummary != request {
		t.Fatalf("memory turn RequestForSummary = %q, want the request text %q", last.RequestForSummary, request)
	}
}

// TestCommandOperationE2E_TemplateExpansionAnswersAgainstExpandedRequest
// pins the request/display split through the producing chain instead of
// operationDispatch directly: Loop → prompt-template expansion
// (`r.dispatch(expanded, line)`) → turn policy → RouteOperation →
// operationDispatch(line, display) → executeCommandOperationPlan. The
// planner and the answerer see the expanded request; the memory turn keeps
// the typed "/mytemplate args" as the display form. A request/display swap
// at any hop of that chain is red here (review round three #7: the direct
// pin left the upstream producers unexercised).
func TestCommandOperationE2E_TemplateExpansionAnswersAgainstExpandedRequest(t *testing.T) {
	const (
		typed    = "/mytemplate args"
		expanded = "EXPANDED-REQUEST show me the go toolchain version for args"
	)
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`),
			{Content: "Go version reported.", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, typed+"\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.promptTemplates = map[string]promptTemplate{
		"mytemplate": {Name: "mytemplate", Body: "EXPANDED-REQUEST show me the go toolchain version for $@", Path: "scratch/mytemplate.md"},
	}
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("auto operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if !strings.Contains(out.String(), "Template /mytemplate expanded") {
		t.Fatalf("template expansion disclosure missing — the chain under test was not traversed:\n%s", out.String())
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("planner+answer calls=%d, want 2", len(adapter.calls))
	}
	plannerUser := lastUserRoleContent(adapter.calls[0].messages)
	if !strings.Contains(plannerUser, expanded) || strings.Contains(plannerUser, typed) {
		t.Fatalf("planner must plan from the expanded request, not the typed template line:\n%s", plannerUser)
	}
	answerUser := lastUserRoleContent(adapter.calls[1].messages)
	if got := userRequestSection(answerUser); got != expanded {
		t.Fatalf("answer synthesis ## user_request = %q, want the expanded request %q\n%s", got, expanded, answerUser)
	}
	recent := store.Recent()
	if len(recent) == 0 {
		t.Fatal("expected the operation turn in memory")
	}
	last := recent[len(recent)-1]
	if last.Request != typed {
		t.Fatalf("memory turn Request = %q, want the typed display form %q", last.Request, typed)
	}
	if last.RequestForSummary != expanded {
		t.Fatalf("memory turn RequestForSummary = %q, want the expanded request %q", last.RequestForSummary, expanded)
	}
}
