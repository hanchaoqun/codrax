package repl

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/operation"
)

// command_operation_approve_carry_test.go — the /approve arm resumes the
// operation it belongs to, not a fresh one (colleague_merge_audit §40.52,
// batch six fold-in review round four #9).
//
// A multi-round operation parks its NEXT plan for approval after earlier
// rounds already ran (a continue_after batch whose continuation needs
// confirmation, or a failed batch whose repair plan escalates risk). The
// answerer is asked to answer the user's ORIGINAL request ("print both the
// go and git versions") from "all operation observations", so the records
// of every executed round — and the repair-round budget already spent —
// must travel with the parked plan and resume on /approve.
//
// EVOLUTION RECORD (review round four #9, pre-existing since fa9a90132):
// handleOperationApproveCmd called executeCommandOperationPlan(plan, …)
// → executeCommandOperationPlanAttempt(…, 0, nil): the approved round
// started from an empty record list, so the answer blob carried
// `rounds=1` and a single round[1] holding only the approved round while
// r.operationResults held both, and the repair budget restarted at 0. Red
// on b6f7eeec3 and on the 480939385 repl.go overlay alike (both
// `rounds=1`, the go-version round absent); green once the parked plan
// carries a pendingCommandOperationCarry keyed by plan ID.

// TestCommandOperationE2E_ApproveParkedContinuationAnswersFromAllRounds:
// round one (`go version`, low, continue_after) auto-executes; the
// continuation plan (`git version`, medium, requires confirmation) parks;
// /approve runs it; the answerer sees both rounds against the full request.
func TestCommandOperationE2E_ApproveParkedContinuationAnswersFromAllRounds(t *testing.T) {
	const request = "print both the go and git versions"
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{responses: []llm.Response{
		commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"continue_after":true,"work_dir":".","goal":"print go version","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`),
		commandOperationPlanResp(`{"status":"ready","risk_level":"medium","requires_confirmation":true,"work_dir":".","goal":"print git version","steps":[{"id":"s2","title":"show git version","program":"git","args":["version"],"risk_level":"medium","side_effects":["process_spawn"]}]}`),
		{Content: "Both versions printed.", StopReason: "end_turn"},
	}}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, request+"\n/approve\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.operationPolicy.AutoApprove = false
	r.operationPolicy.AutoLowRisk = true
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(adapter.calls) != 3 {
		t.Fatalf("adapter calls=%d, want 3 (planner, continuation, answer)\n%s", len(adapter.calls), out.String())
	}
	if r.pendingOperation != nil {
		t.Fatalf("approved continuation must not stay pending: %+v", r.pendingOperation)
	}
	if len(r.operationResults) != 2 {
		t.Fatalf("operationResults=%d, want 2 (go round + git round)", len(r.operationResults))
	}
	answerUser := lastUserRoleContent(adapter.calls[2].messages)
	if got := userRequestSection(answerUser); got != request {
		t.Fatalf("answer synthesis ## user_request = %q, want %q\n%s", got, request, answerUser)
	}
	assertAnswerBlobCarriesRounds(t, answerUser, 2, "print go version", "print git version")
	recent := store.Recent()
	if len(recent) == 0 {
		t.Fatal("expected the approved operation turn in memory")
	}
	last := recent[len(recent)-1]
	if last.Request != "/approve" || last.RequestForSummary != request {
		t.Fatalf("memory turn Request=%q RequestForSummary=%q, want %q / %q", last.Request, last.RequestForSummary, "/approve", request)
	}
}

// TestCommandOperationE2E_ApproveParkedReplanAnswersFromAllRounds: round
// one (a failing probe, low) auto-executes and fails; the repair plan
// escalates to high risk and parks; /approve runs it; the answerer sees
// the failed round and the repaired round against the full request.
func TestCommandOperationE2E_ApproveParkedReplanAnswersFromAllRounds(t *testing.T) {
	const request = "record a marker file, probing first"
	marker := filepath.Join(t.TempDir(), "codrax-approve-carry-marker")
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{responses: []llm.Response{
		commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"probe before writing","steps":[{"id":"bad","title":"failing probe","shell":"printf 'probe failed\\n'; exit 2","risk_level":"low","side_effects":[]}]}`),
		commandOperationPlanResp(`{"status":"ready","risk_level":"high","requires_confirmation":true,"work_dir":".","goal":"write the marker file","steps":[{"id":"write","title":"write marker fallback","program":"touch","args":["` + marker + `"],"risk_level":"high","side_effects":["local_file_write"]}]}`),
		{Content: "Marker written after the probe failed.", StopReason: "end_turn"},
	}}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, request+"\n/approve\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(adapter.calls) != 3 {
		t.Fatalf("adapter calls=%d, want 3 (planner, replan, answer)\n%s", len(adapter.calls), out.String())
	}
	if r.pendingOperation != nil {
		t.Fatalf("approved replan must not stay pending: %+v", r.pendingOperation)
	}
	if len(r.operationResults) != 2 {
		t.Fatalf("operationResults=%d, want 2 (failed probe + approved repair)", len(r.operationResults))
	}
	answerUser := lastUserRoleContent(adapter.calls[2].messages)
	if got := userRequestSection(answerUser); got != request {
		t.Fatalf("answer synthesis ## user_request = %q, want %q\n%s", got, request, answerUser)
	}
	assertAnswerBlobCarriesRounds(t, answerUser, 2, "failing probe", "write marker fallback")
}

// assertAnswerBlobCarriesRounds checks the answerer's user message lists
// every executed round (round[1..n]) and each named round marker.
func assertAnswerBlobCarriesRounds(t *testing.T, answerUser string, rounds int, markers ...string) {
	t.Helper()
	for i := 1; i <= rounds; i++ {
		tag := "round[" + strconv.Itoa(i) + "]"
		if !strings.Contains(answerUser, tag) {
			t.Fatalf("answer blob lacks %s — the approved round was answered without the operation's earlier records\n%s", tag, answerUser)
		}
	}
	if strings.Contains(answerUser, "round["+strconv.Itoa(rounds+1)+"]") {
		t.Fatalf("answer blob carries more than %d rounds\n%s", rounds, answerUser)
	}
	for _, m := range markers {
		if !strings.Contains(answerUser, m) {
			t.Fatalf("answer blob lacks the round marker %q\n%s", m, answerUser)
		}
	}
}
