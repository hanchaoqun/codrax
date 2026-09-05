package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/operation"
)

// command_operation_followup_budget_test.go — the command-round budget is
// the operation's OWN, never the context window's (colleague_merge_audit
// §40.52, batch six fold-in review round five #2).
//
// A follow-up typed after a finished operation is a NEW operation planned
// and answered against the cross-operation window r.operationResults
// (capped at 6). The attempt used to receive that window as its record
// slice and derive its command-round counter from it, so after a
// five-round operation (window = five executed rounds + the budget
// round, above commandOperationMaxCommandRounds) the user's follow-up was
// refused as budget-exhausted before executor.Execute ever ran:
//
//   - the auto-execute follow-up lanes, since f73788fe6 (red on 480939385,
//     b6f7eeec3 and 533a939fb);
//   - an explicitly approved follow-up, since round four carried the same
//     window into /approve (red on 533a939fb; on b6f7eeec3 the approved
//     plan executed but the answerer saw only that round).
//
// The attempt now takes the operation's context and own rounds as
// distinct typed values with the counter carried verbatim: the budget
// reads own rounds only, the planner and answerer keep seeing context
// followed by own.

const followupBudgetGoRound = `{"status":"ready","risk_level":"low","requires_confirmation":false,"continue_after":true,"work_dir":".","goal":"print go version","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`

// followupBudgetFullWindowREPL drives a five-round low-risk operation to
// its budget and answer (six adapter calls), then a follow-up turn whose
// continuation plan is followupPlan, then the input tail (e.g. "/approve");
// further responses follow it.
func followupBudgetFullWindowREPL(t *testing.T, followupPlan, inputTail string, after ...llm.Response) (*REPL, *scriptedChatAdapter, string) {
	t.Helper()
	classifier := &sequenceTurnPolicyClassifier{policies: []TurnPolicy{
		commandOperationPolicy("low"),
		{Route: RouteLocal, Operation: "elaborate", Source: "last_answer", Confidence: 0.9, Reason: "continue"},
	}}
	responses := []llm.Response{
		commandOperationPlanResp(followupBudgetGoRound), commandOperationPlanResp(followupBudgetGoRound), commandOperationPlanResp(followupBudgetGoRound), commandOperationPlanResp(followupBudgetGoRound), commandOperationPlanResp(followupBudgetGoRound),
		{Content: "five go rounds done.", StopReason: "end_turn"},
		commandOperationPlanResp(followupPlan),
	}
	responses = append(responses, after...)
	adapter := &scriptedChatAdapter{responses: responses}
	const request = "run five go rounds"
	r, _, out := newTurnPolicyREPL(t, newPolicyStore(t), classifier, &stubLocalResponder{}, request+"\nnow the git version too\n"+inputTail+"/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.operationPolicy.AutoApprove = false
	r.operationPolicy.AutoLowRisk = true
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	return r, adapter, out.String()
}

func requireFollowupRoundExecuted(t *testing.T, r *REPL, printed, goal, marker string) {
	t.Helper()
	if r.pendingOperation != nil {
		t.Fatalf("the follow-up plan must not stay pending: %+v", r.pendingOperation)
	}
	last := r.operationResults[len(r.operationResults)-1]
	if last.Plan.Goal != goal || last.Result.Status != operation.StatusExecuted {
		t.Fatalf("last record plan goal=%q status=%s preview=%q — the follow-up was refused instead of executed\n%s", last.Plan.Goal, last.Result.Status, last.Result.OutputPreview, printed)
	}
	if !strings.Contains(printed, marker) {
		t.Fatalf("the follow-up's round output %q did not appear:\n%s", marker, printed)
	}
}

// TestCommandOperationE2E_ApprovedFollowupAfterFullWindowExecutes (pin a):
// five executed rounds in the window; the follow-up's medium-risk plan
// parks; /approve executes it, and the answerer sees the window (six
// context rounds) followed by the approved round, against the follow-up's
// request text.
func TestCommandOperationE2E_ApprovedFollowupAfterFullWindowExecutes(t *testing.T) {
	r, adapter, printed := followupBudgetFullWindowREPL(t,
		`{"status":"ready","risk_level":"medium","requires_confirmation":true,"work_dir":".","goal":"print git version","steps":[{"id":"s2","title":"show git version","program":"git","args":["version"],"risk_level":"medium","side_effects":["process_spawn"]}]}`,
		"/approve\n",
		llm.Response{Content: "git version printed after the go rounds.", StopReason: "end_turn"},
	)
	if len(adapter.calls) != 8 {
		t.Fatalf("adapter calls=%d, want 8 (planner, 4 continuations, answer, follow-up continuation, answer)\n%s", len(adapter.calls), printed)
	}
	requireFollowupRoundExecuted(t, r, printed, "print git version", "git version")
	answerUser := lastUserRoleContent(adapter.calls[7].messages)
	// the follow-up request text is multi-paragraph, so match the whole
	// section rather than userRequestSection's first paragraph
	const wantRequest = "## user_request\nrun five go rounds\n\nFollow-up request:\nnow the git version too\n\n"
	if !strings.Contains(answerUser, wantRequest) {
		t.Fatalf("answer synthesis lacks the follow-up's request text %q\n%s", wantRequest, answerUser)
	}
	assertAnswerBlobCarriesRounds(t, answerUser, 7, "print go version", "print git version")
}

// TestCommandOperationE2E_AutoFollowupAfterFullWindowExecutes (pin b):
// five executed rounds in the window; the follow-up's low-risk read-only
// plan (`uname -s`, auto-eligible without approval) takes the auto-execute
// lane and executes — no "/approve" is typed, so a parked plan fails loud.
func TestCommandOperationE2E_AutoFollowupAfterFullWindowExecutes(t *testing.T) {
	r, adapter, printed := followupBudgetFullWindowREPL(t,
		`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"print the kernel name","steps":[{"id":"s2","title":"show kernel name","program":"uname","args":["-s"],"risk_level":"low","side_effects":[]}]}`,
		"",
		llm.Response{Content: "kernel name printed after the go rounds.", StopReason: "end_turn"},
	)
	if len(adapter.calls) != 8 {
		t.Fatalf("adapter calls=%d, want 8 (planner, 4 continuations, answer, follow-up continuation, answer)\n%s", len(adapter.calls), printed)
	}
	requireFollowupRoundExecuted(t, r, printed, "print the kernel name", "Darwin")
	answerUser := lastUserRoleContent(adapter.calls[7].messages)
	assertAnswerBlobCarriesRounds(t, answerUser, 7, "print go version", "print the kernel name")
}

// TestCommandOperationE2E_ApprovedContinuationResumesCounterAndStopsAtLimit
// (pin c): an in-loop continuation parks after the operation's first round;
// /approve resumes with the carried counter (1), the approved round and
// three auto continuations follow, and the operation stops at the limit
// of five executed rounds — the approval never resets the budget.
func TestCommandOperationE2E_ApprovedContinuationResumesCounterAndStopsAtLimit(t *testing.T) {
	const gitContinue = `{"status":"ready","risk_level":"medium","requires_confirmation":true,"continue_after":true,"work_dir":".","goal":"print git version","steps":[{"id":"s2","title":"show git version","program":"git","args":["version"],"risk_level":"medium","side_effects":["process_spawn"]}]}`
	adapter := &scriptedChatAdapter{responses: []llm.Response{
		commandOperationPlanResp(followupBudgetGoRound),
		commandOperationPlanResp(gitContinue),
		commandOperationPlanResp(followupBudgetGoRound), commandOperationPlanResp(followupBudgetGoRound), commandOperationPlanResp(followupBudgetGoRound),
		{Content: "stopped at the round limit.", StopReason: "end_turn"},
		// a sixth executed round would consume these
		commandOperationPlanResp(followupBudgetGoRound),
		{Content: "spare answer.", StopReason: "end_turn"},
	}}
	r, _, out := newTurnPolicyREPL(t, newPolicyStore(t), &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}, &stubLocalResponder{}, "print go and git versions, then keep going\n/approve\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.operationPolicy.AutoApprove = false
	r.operationPolicy.AutoLowRisk = true
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if r.pendingOperation != nil {
		t.Fatalf("the approved continuation must not stay pending: %+v", r.pendingOperation)
	}
	if len(adapter.calls) != 6 {
		t.Fatalf("adapter calls=%d, want 6 (planner, parked continuation, 3 continuations, answer) — a different count means the approval reset the round budget\n%s", len(adapter.calls), out.String())
	}
	executed := 0
	for _, rec := range r.operationResults {
		if rec.Result.Status == operation.StatusExecuted {
			executed++
		}
	}
	if executed != commandOperationMaxCommandRounds {
		t.Fatalf("executed rounds in the window=%d, want %d (the approved round resumed at counter 1)", executed, commandOperationMaxCommandRounds)
	}
	last := r.operationResults[len(r.operationResults)-1]
	if last.Result.Status != operation.StatusBudgetExhausted {
		t.Fatalf("the operation must stop at the limit with a budget round, last status=%s", last.Result.Status)
	}
}

// followupBudgetExtendedWindow builds a five-round executed window whose
// last round carries material and an evaluation that would grant the
// bounded extension if it were read as the operation's own rounds.
func followupBudgetExtendedWindow(t *testing.T) []commandOperationResultRecord {
	t.Helper()
	policy := operation.DefaultCommandPolicy()
	var window []commandOperationResultRecord
	for i := 0; i < commandOperationMaxCommandRounds; i++ {
		plan := operation.BuildCommandOperationPlan(operation.CommandOperationRequest{
			ID: "earlier-" + string(rune('a'+i)), Text: "read the earlier material", Goal: "earlier round",
			Steps: []operation.CommandStep{{ID: "s", Title: "earlier", Program: "go", Args: []string{"version"}, RiskLevel: "low"}},
		}, policy)
		rec := commandOperationResultRecord{Plan: plan, Result: operation.CommandOperationResult{
			PlanID: plan.ID, Status: operation.StatusExecuted, PayloadRef: "/tmp/codrax-earlier-material.txt",
			StepResults: []operation.CommandStepResult{{StepID: "s", Status: operation.StatusExecuted, PayloadRef: "/tmp/codrax-earlier-material.txt"}},
		}}
		if i == commandOperationMaxCommandRounds-1 {
			rec.Evaluation = &operation.OperationEvaluation{Status: operation.EvalContinueCommand, MaterialCoverageStatus: operation.MaterialCoveragePartial}
		}
		window = append(window, rec)
	}
	if got := commandOperationCommandRoundLimit(commandOperationOwnRecords(window)); got != commandOperationExtendedCommandRounds {
		t.Fatalf("the window read as own rounds must grant the extension (limit=%d, want %d) for the pin to prove anything", got, commandOperationExtendedCommandRounds)
	}
	return window
}

// TestCommandOperationContextEvaluationNeverExtendsFreshLimit (pin e,
// direct): a fresh follow-up state carrying that window as Context has an
// own-round limit of commandOperationMaxCommandRounds and no spent budget.
func TestCommandOperationContextEvaluationNeverExtendsFreshLimit(t *testing.T) {
	window := followupBudgetExtendedWindow(t)
	fresh := commandOperationAttemptState{Context: window}
	if got := commandOperationCommandRoundLimit(fresh.Own); got != commandOperationMaxCommandRounds {
		t.Fatalf("fresh follow-up limit=%d, want %d — the context window's evaluation extended it", got, commandOperationMaxCommandRounds)
	}
	if got := commandOperationExecutedRoundCount(fresh.Own); got != 0 {
		t.Fatalf("fresh follow-up executed rounds=%d, want 0", got)
	}
	plan := operation.CommandOperationPlan{ContinueAfter: true}
	if commandOperationContinuationBudgetExhausted(plan, operation.CommandOperationResult{Status: operation.StatusExecuted}, fresh.Own) {
		t.Fatal("a fresh follow-up has no continuation budget spent")
	}
	if got := len(commandOperationWindow(fresh.Context, fresh.Own)); got != len(window) {
		t.Fatalf("planner/answerer window=%d rounds, want the %d context rounds", got, len(window))
	}
}

// TestCommandOperationE2E_FollowupAfterExtendedWindowStopsAtOwnLimit (pin
// e, through Loop): a follow-up planned against that window runs its own
// continue_after chain and stops at five OWN executed rounds — not at the
// window's extended eight, and not at zero.
func TestCommandOperationE2E_FollowupAfterExtendedWindowStopsAtOwnLimit(t *testing.T) {
	window := followupBudgetExtendedWindow(t)
	classifier := &stubTurnPolicyClassifier{policy: TurnPolicy{Route: RouteLocal, Operation: "elaborate", Source: "last_answer", Confidence: 0.9, Reason: "continue"}}
	responses := []llm.Response{}
	for i := 0; i < commandOperationExtendedCommandRounds; i++ {
		responses = append(responses, commandOperationPlanResp(followupBudgetGoRound))
	}
	responses = append(responses, llm.Response{Content: "stopped at the follow-up's own limit.", StopReason: "end_turn"}, llm.Response{Content: "spare answer.", StopReason: "end_turn"})
	adapter := &scriptedChatAdapter{responses: responses}
	r, _, out := newTurnPolicyREPL(t, newPolicyStore(t), classifier, &stubLocalResponder{}, "keep reading\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.operationPolicy.AutoApprove = false
	r.operationPolicy.AutoLowRisk = true
	r.operationResults = append([]commandOperationResultRecord(nil), window...)
	r.lastAnswerOrigin = replAnswerOriginCommandOperationFinal
	// the earlier operation's answer, so the follow-up has a prior answer
	// to continue from
	r.recordTurn("read the earlier material", "read the earlier material", "earlier material read.", memory.KindPipeline)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	// follow-up continuation + 4 in-loop continuations + answer
	if len(adapter.calls) != commandOperationMaxCommandRounds+1 {
		t.Fatalf("adapter calls=%d, want %d\n%s", len(adapter.calls), commandOperationMaxCommandRounds+1, out.String())
	}
	ownExecuted := 0
	for _, rec := range r.operationResults {
		if rec.Plan.Goal == "print go version" && rec.Result.Status == operation.StatusExecuted {
			ownExecuted++
		}
	}
	if ownExecuted != commandOperationMaxCommandRounds {
		t.Fatalf("follow-up executed rounds=%d, want %d (own limit; the window's evaluation must not extend it)\n%s", ownExecuted, commandOperationMaxCommandRounds, out.String())
	}
	last := r.operationResults[len(r.operationResults)-1]
	if last.Result.Status != operation.StatusBudgetExhausted || last.Plan.Goal != "print go version" {
		t.Fatalf("the follow-up must stop with its own budget round, last goal=%q status=%s", last.Plan.Goal, last.Result.Status)
	}
}
