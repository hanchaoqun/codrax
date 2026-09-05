package repl

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/operation"
)

// command_operation_approve_budget_carry_test.go — an approved in-loop plan
// resumes with the budget the loop had already spent, on both budget axes
// (colleague_merge_audit §40.52, batch six fold-in review round six #0/#1),
// a failed round at the command-round limit ends without a replanner call
// or a park in every lane, and a park whose counter differs from its own
// round count resumes at the carried counter (review round seven #0/#1).
//
// Every pin drives the real planner with a scripted adapter through Loop,
// with AutoApprove so the low-risk shell probes and repairs auto-execute
// while a high-risk plan still parks on its risk signal.

const (
	// approveBudgetGoRound is a low-risk executed round that asks to
	// continue.
	approveBudgetGoRound = `{"status":"ready","risk_level":"low","requires_confirmation":false,"continue_after":true,"work_dir":".","goal":"print go version","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`
	// approveBudgetFailingProbe is a low-risk shell probe that fails; as the
	// operation's first round it spends one repair.
	approveBudgetFailingProbe = `{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"probe","steps":[{"id":"bad","title":"failing probe","shell":"printf 'probe failed\\n'; exit 2","risk_level":"low","side_effects":[]}]}`
	// approveBudgetRepairOne / Two are distinct failing repairs (a repeated
	// failed command is rejected by the revised-plan preflight lint instead
	// of executing).
	approveBudgetRepairOne = `{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"repair one","steps":[{"id":"r1","title":"first repair","shell":"printf 'repair one failed\\n'; exit 3","risk_level":"low","side_effects":[]}]}`
	approveBudgetRepairTwo = `{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"repair two","steps":[{"id":"r2","title":"second repair","shell":"printf 'repair two failed\\n'; exit 4","risk_level":"low","side_effects":[]}]}`
	// approveBudgetHighFailingContinuation parks (high risk, confirmation
	// required) and fails once approved; approveBudgetLowFailingContinuation
	// is the same failing step on the auto-execute lane.
	approveBudgetHighFailingContinuation = `{"status":"ready","risk_level":"high","requires_confirmation":true,"work_dir":".","goal":"write fallback","steps":[{"id":"write","title":"write fallback","shell":"printf 'fallback failed\\n'; exit 5","risk_level":"high","side_effects":["local_file_write"]}]}`
	approveBudgetLowFailingContinuation  = `{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"write fallback","steps":[{"id":"write","title":"write fallback","shell":"printf 'fallback failed\\n'; exit 5","risk_level":"low","side_effects":[]}]}`
)

func newApproveBudgetREPL(t *testing.T, adapter llm.Adapter, input string) (*REPL, *bytes.Buffer) {
	t.Helper()
	r, _, out := newTurnPolicyREPL(t, newPolicyStore(t), &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}, &stubLocalResponder{}, input)
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.operationPolicy.AutoApprove = true
	r.operationPolicy.AutoLowRisk = true
	return r, out
}

// approveBudgetStatuses returns the window's statuses plus the counts of
// executed and failed rounds.
func approveBudgetStatuses(r *REPL) (statuses []operation.OperationStatus, executed, failed int) {
	for _, rec := range r.operationResults {
		statuses = append(statuses, rec.Result.Status)
		switch rec.Result.Status {
		case operation.StatusExecuted:
			executed++
		case operation.StatusFailed:
			failed++
		}
	}
	return statuses, executed, failed
}

// approveBudgetRepairLane runs: failing probe → auto repair (go version,
// continue_after) → continuation (the lane's plan) → the continuation fails
// → scripted failing repairs. One repair is spent before the continuation,
// so the loop may grant commandOperationMaxRepairRounds-1 = 2 further ones:
// six adapter calls (planner, repair, continuation, two repairs, answer)
// and the repair-budget terminal. The spare repair and answer after the
// answer are consumed only when a third repair is granted.
func approveBudgetRepairLane(t *testing.T, continuation, inputTail string) (*REPL, *scriptedChatAdapter, string) {
	t.Helper()
	adapter := &scriptedChatAdapter{responses: []llm.Response{
		commandOperationPlanResp(approveBudgetFailingProbe),
		commandOperationPlanResp(approveBudgetGoRound),
		commandOperationPlanResp(continuation),
		commandOperationPlanResp(approveBudgetRepairOne),
		commandOperationPlanResp(approveBudgetRepairTwo),
		{Content: "repairs exhausted.", StopReason: "end_turn"},
		commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"repair three","steps":[{"id":"r3","title":"third repair","shell":"printf 'repair three failed\\n'; exit 6","risk_level":"low","side_effects":[]}]}`),
		{Content: "spare answer.", StopReason: "end_turn"},
	}}
	r, out := newApproveBudgetREPL(t, adapter, "probe, print the go version, then write the fallback\n"+inputTail+"/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	return r, adapter, out.String()
}

func requireRepairBudgetTerminal(t *testing.T, lane string, r *REPL, adapter *scriptedChatAdapter, printed string) {
	t.Helper()
	if r.pendingOperation != nil {
		t.Fatalf("%s lane: the continuation must not stay pending: %+v", lane, r.pendingOperation)
	}
	statuses, executed, failed := approveBudgetStatuses(r)
	if len(adapter.calls) != 6 {
		t.Fatalf("%s lane: adapter calls=%d, want 6 (planner, repair, continuation, two further repairs, answer) — a different count means the repair budget was not resumed where the loop left it; window=%v\n%s", lane, len(adapter.calls), statuses, printed)
	}
	// probe, approved/auto continuation, two further repairs
	if executed != 1 || failed != 4 {
		t.Fatalf("%s lane: executed=%d failed=%d, want 1/4 (the go round; the probe, the continuation and two further repairs); window=%v", lane, executed, failed, statuses)
	}
	last := r.operationResults[len(r.operationResults)-1]
	if last.Result.Status != operation.StatusBudgetExhausted || !strings.Contains(last.Result.OutputPreview, "repair budget exhausted") {
		t.Fatalf("%s lane: the operation must end with the repair-budget terminal, last status=%s preview=%q; window=%v", lane, last.Result.Status, last.Result.OutputPreview, statuses)
	}
}

// TestCommandOperationE2E_ApprovedContinuationAfterRepairKeepsRepairBudget
// (pin g): after one spent repair the continuation parks for approval;
// /approve resumes with ReplanAttempts=1, so the approved round's failure
// is granted two further repairs — the same remainder the auto-execute
// lane grants the identical script — and the adapter-call counts of the
// two lanes are equal.
//
// EVOLUTION RECORD (review round six #0): the two in-loop continuation
// parks carried the context, own rounds and counter but not the repair
// rounds spent, so an approved continuation restarted the repair budget
// at zero and the approve lane asked the replanner a third time — seven
// adapter calls, the third replan consuming the scripted answer (with a
// third repair scripted it executes, and the carried counter then ends
// the operation on the command-round budget instead of the repair
// budget). Red on 6f98f839d (approve lane: seven calls, window
// failed/executed/failed/failed/failed), pre-existing on 533a939fb
// (`parkCommandOperationPlan(nextPlan, records, 0)`), b6f7eeec3 and
// 480939385 (/approve resumed from a fresh state); the auto lane is green
// on every base.
func TestCommandOperationE2E_ApprovedContinuationAfterRepairKeepsRepairBudget(t *testing.T) {
	autoREPL, autoAdapter, autoPrinted := approveBudgetRepairLane(t, approveBudgetLowFailingContinuation, "")
	requireRepairBudgetTerminal(t, "auto", autoREPL, autoAdapter, autoPrinted)
	approveREPL, approveAdapter, approvePrinted := approveBudgetRepairLane(t, approveBudgetHighFailingContinuation, "/approve\n")
	requireRepairBudgetTerminal(t, "approve", approveREPL, approveAdapter, approvePrinted)
	if len(approveAdapter.calls) != len(autoAdapter.calls) {
		t.Fatalf("approve lane adapter calls=%d, auto lane=%d — the two lanes must grant the same repair remainder", len(approveAdapter.calls), len(autoAdapter.calls))
	}
}

// TestCommandOperationE2E_ApprovedRepairAfterFailedRoundResumesCounter
// (pin h): a repair plan parks after a failed probe (carry Own=[failed],
// CommandRounds=1); /approve resumes at the carried counter, so the
// approved round and its continue_after chain execute exactly four more
// rounds and the loop stops at the counter: six adapter calls (planner,
// repair, three continuations, answer), one failed and four executed
// rounds, no sixth executor round. The failed round counts on the counter
// but not on the executed-derived continuation predicate, so the loop
// stops without a budget record here (round five residual, ruled shape).
//
// EVOLUTION RECORD (review round six #1): the counter is carried verbatim
// (never re-derived from the own rounds). Re-deriving it from the executed
// own rounds — `commandRounds :=
// commandOperationExecutedRoundCount(resume.Own)` at the attempt entry,
// the pre-round-five derivation on 480939385, b6f7eeec3 and 533a939fb —
// resumes at 0 after a failed round and grants a fifth executed round
// (a sixth executor round); proven red on that mutation with a scratch
// overlay (not committed). Red on 533a939fb (counter re-derived from the
// carried records), b6f7eeec3 and 480939385 (/approve resumed from a
// fresh state).
func TestCommandOperationE2E_ApprovedRepairAfterFailedRoundResumesCounter(t *testing.T) {
	highRepair := `{"status":"ready","risk_level":"high","requires_confirmation":true,"continue_after":true,"work_dir":".","goal":"write fallback","steps":[{"id":"write","title":"write fallback","program":"touch","args":["` + filepath.Join(t.TempDir(), "marker") + `"],"risk_level":"high","side_effects":["local_file_write"]}]}`
	adapter := &scriptedChatAdapter{responses: []llm.Response{
		commandOperationPlanResp(approveBudgetFailingProbe),
		commandOperationPlanResp(highRepair),
		commandOperationPlanResp(approveBudgetGoRound), commandOperationPlanResp(approveBudgetGoRound), commandOperationPlanResp(approveBudgetGoRound),
		{Content: "stopped at the round limit.", StopReason: "end_turn"},
		// a sixth executor round (a fifth executed round) would consume these
		commandOperationPlanResp(approveBudgetGoRound),
		{Content: "spare answer.", StopReason: "end_turn"},
	}}
	r, out := newApproveBudgetREPL(t, adapter, "record a marker file, probing first, then keep going\n/approve\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if r.pendingOperation != nil {
		t.Fatalf("the approved repair must not stay pending: %+v", r.pendingOperation)
	}
	statuses, executed, failed := approveBudgetStatuses(r)
	if len(adapter.calls) != 6 {
		t.Fatalf("adapter calls=%d, want 6 (planner, parked repair, 3 continuations, answer) — a different count means the approval re-derived the counter instead of resuming at 1; window=%v\n%s", len(adapter.calls), statuses, out.String())
	}
	if failed != 1 || executed != commandOperationMaxCommandRounds-1 {
		t.Fatalf("failed=%d executed=%d, want 1/%d (the failed probe counts as the first executor round); window=%v", failed, executed, commandOperationMaxCommandRounds-1, statuses)
	}
	last := r.operationResults[len(r.operationResults)-1]
	if last.Result.Status != operation.StatusExecuted || last.Plan.Goal != "print go version" {
		t.Fatalf("the operation must stop at the counter after the fourth executed round, last goal=%q status=%s; window=%v", last.Plan.Goal, last.Result.Status, statuses)
	}
}

// replanRoutingAdapter serves the replanner's request from a dedicated slot
// so a scripted lane keeps the same queue whether or not the loop consults
// the replanner: a call whose prompt carries the replan context returns
// the repair plan and is counted; every other call pops the queue.
type replanRoutingAdapter struct {
	*scriptedChatAdapter
	repair      llm.Response
	replanCalls int
}

func (a *replanRoutingAdapter) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	for _, msg := range messages {
		if strings.Contains(msg.Content, "## replan_context") {
			a.replanCalls++
			return a.repair, nil
		}
	}
	return a.scriptedChatAdapter.Chat(ctx, messages, tools, opts)
}

// approveBudgetCounterLimitLane runs four executed continue_after rounds and
// a fifth, failing, auto-executed continuation — the counter reaches
// commandOperationMaxCommandRounds on the failed round. The repair the
// replanner would mint (high risk parks, low risk auto-executes) is served
// from the routing slot; the queue then holds the answer and a spare round
// plus answer that only a resumed or auto-continued repair could consume.
func approveBudgetCounterLimitLane(t *testing.T, repair, inputTail string) (*REPL, *replanRoutingAdapter, string) {
	t.Helper()
	adapter := &replanRoutingAdapter{
		scriptedChatAdapter: &scriptedChatAdapter{responses: []llm.Response{
			commandOperationPlanResp(approveBudgetGoRound),
			commandOperationPlanResp(approveBudgetGoRound),
			commandOperationPlanResp(approveBudgetGoRound),
			commandOperationPlanResp(approveBudgetGoRound),
			commandOperationPlanResp(approveBudgetLowFailingContinuation),
			{Content: "stopped at the command-round budget.", StopReason: "end_turn"},
			commandOperationPlanResp(approveBudgetGoRound),
			{Content: "spare answer.", StopReason: "end_turn"},
		}},
		repair: commandOperationPlanResp(repair),
	}
	r, out := newApproveBudgetREPL(t, adapter, "print the go version repeatedly then write the fallback\n"+inputTail+"/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	return r, adapter, out.String()
}

func requireCommandRoundBudgetWithoutReplan(t *testing.T, lane string, r *REPL, adapter *replanRoutingAdapter, printed, marker string) {
	t.Helper()
	if adapter.replanCalls != 0 {
		t.Fatalf("%s lane: replanner consulted %d time(s) for a failed round at the command-round limit — the budget terminal must be minted without a replanner call\n%s", lane, adapter.replanCalls, printed)
	}
	if r.pendingOperation != nil || r.pendingOperationCarry != nil {
		t.Fatalf("%s lane: nothing may park when the counter is at the limit: plan=%+v carry=%+v", lane, r.pendingOperation, r.pendingOperationCarry)
	}
	statuses, executed, failed := approveBudgetStatuses(r)
	// planner, four continuations (three go rounds and the failing one), answer
	if len(adapter.calls) != 6 {
		t.Fatalf("%s lane: adapter calls=%d, want 6 (planner, 4 continuations, answer); window=%v\n%s", lane, len(adapter.calls), statuses, printed)
	}
	if executed != commandOperationMaxCommandRounds-1 || failed != 1 {
		t.Fatalf("%s lane: executed=%d failed=%d, want %d/1; window=%v", lane, executed, failed, commandOperationMaxCommandRounds-1, statuses)
	}
	last := r.operationResults[len(r.operationResults)-1]
	if last.Result.Status != operation.StatusBudgetExhausted || !strings.Contains(last.Result.OutputPreview, "command-round budget exhausted") {
		t.Fatalf("%s lane: the failed round at the limit must end on the command-round budget terminal, last status=%s preview=%q; window=%v", lane, last.Result.Status, last.Result.OutputPreview, statuses)
	}
	if last.Plan.Goal != "write fallback" {
		t.Fatalf("%s lane: the budget terminal must belong to the failed round's plan, got goal=%q", lane, last.Plan.Goal)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("%s lane: the repair's marker file exists — a repair ran past the command-round limit", lane)
	}
	// the bordered panel wraps at the terminal width (mid-word included),
	// so the text checks read the panel with its border and whitespace
	// removed
	compact := strings.Join(strings.Fields(strings.ReplaceAll(printed, "│", "")), "")
	for _, banned := range []string{
		"revisedcommandplan",
		"step``",
	} {
		if strings.Contains(compact, banned) {
			t.Fatalf("%s lane: panel must not print %q:\n%s", lane, banned, printed)
		}
	}
	if got := strings.Count(compact, "command-roundbudgetexhaustedbeforetheusergoalwasfullysatisfied"); got != 1 {
		t.Fatalf("%s lane: the budget reason must appear exactly once on the panel, got %d:\n%s", lane, got, printed)
	}
	if !strings.Contains(compact, "Note:commandoperationcommand-roundbudgetexhausted") {
		t.Fatalf("%s lane: the budget terminal must render through the step-less Note shape:\n%s", lane, printed)
	}
}

// TestCommandOperationE2E_FailedRoundAtCounterLimitEndsWithoutReplan
// (pin i): a round that fails with the counter already at
// commandOperationCommandRoundLimit mints the command-round budget
// terminal directly — the replanner is not consulted, nothing parks, no
// approval is requested — identically when the repair would have parked
// (high risk; /approve then has nothing to approve) and when it would have
// auto-executed (low risk). Six adapter calls: planner, four
// continuations, answer.
//
// EVOLUTION RECORD (review round seven #0): the replan lane was gated on
// status/timeout/repair rounds/planner only while both continuation lanes
// pre-gate on commandRounds < commandOperationCommandRoundLimit(ownRecords),
// so a failed fifth round consulted the replanner (one wasted LLM call), a
// high-risk repair parked with the counter carried at the limit and the
// panel asked for /approve — which the loop refused before
// executor.Execute; the low-risk repair auto-continued into the same
// refusal. Red on 42fcf3fd1 and 6f98f839d (replanner consulted, repair
// parked, prompt then refused on /approve: seven calls), on 533a939fb
// (replanner consulted; the approve arm re-derived the counter from the
// executed rounds, so the approved repair ran past the limit) and on
// b6f7eeec3 / 480939385 (replanner consulted; /approve resumed from a
// fresh state and ran the repair).
func TestCommandOperationE2E_FailedRoundAtCounterLimitEndsWithoutReplan(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "marker")
	highRepair := `{"status":"ready","risk_level":"high","requires_confirmation":true,"work_dir":".","goal":"write fallback marker","steps":[{"id":"write","title":"write fallback","program":"touch","args":["` + marker + `"],"risk_level":"high","side_effects":["local_file_write"]}]}`
	lowRepair := `{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"write fallback marker","steps":[{"id":"write","title":"write fallback","program":"touch","args":["` + marker + `"],"risk_level":"low","side_effects":[]}]}`

	parkREPL, parkAdapter, parkPrinted := approveBudgetCounterLimitLane(t, highRepair, "")
	requireCommandRoundBudgetWithoutReplan(t, "high-risk repair (no approval given)", parkREPL, parkAdapter, parkPrinted, marker)
	approveREPL, approveAdapter, approvePrinted := approveBudgetCounterLimitLane(t, highRepair, "/approve\n")
	requireCommandRoundBudgetWithoutReplan(t, "high-risk repair (/approve typed)", approveREPL, approveAdapter, approvePrinted, marker)
	autoREPL, autoAdapter, autoPrinted := approveBudgetCounterLimitLane(t, lowRepair, "")
	requireCommandRoundBudgetWithoutReplan(t, "low-risk repair (auto)", autoREPL, autoAdapter, autoPrinted, marker)
	if len(approveAdapter.calls) != len(autoAdapter.calls) || len(parkAdapter.calls) != len(autoAdapter.calls) {
		t.Fatalf("adapter calls park=%d approve=%d auto=%d — every lane must end identically", len(parkAdapter.calls), len(approveAdapter.calls), len(autoAdapter.calls))
	}
}

// approveBudgetLintFailedContinuation is a continuation that fails
// operation.LintCommandOperationPlan (the shell starts with a pipe): it is
// recorded as a synthetic failed own round WITHOUT spending the counter, so
// the operation's park carries CommandRounds != len(Own).
const approveBudgetLintFailedContinuation = `{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"filter","steps":[{"id":"filter","title":"filter the output","shell":"| grep version","risk_level":"low","side_effects":[]}]}`

// approveBudgetLintLane runs: go round (counter 1) → lint-failed
// continuation (synthetic failed round, counter stays 1) → repair plan (the
// lane's; continue_after) → three go continuations → the executed-round
// count reaches the limit → command-round budget terminal → answer. Seven
// adapter calls: planner, continuation, replanner, three continuations,
// answer; the spare round and answer are consumed only when a lane grants
// an extra round.
func approveBudgetLintLane(t *testing.T, repair, inputTail string) (*REPL, *scriptedChatAdapter, string) {
	t.Helper()
	adapter := &scriptedChatAdapter{responses: []llm.Response{
		commandOperationPlanResp(approveBudgetGoRound),
		commandOperationPlanResp(approveBudgetLintFailedContinuation),
		commandOperationPlanResp(repair),
		commandOperationPlanResp(approveBudgetGoRound), commandOperationPlanResp(approveBudgetGoRound), commandOperationPlanResp(approveBudgetGoRound),
		{Content: "stopped at the command-round budget.", StopReason: "end_turn"},
		commandOperationPlanResp(approveBudgetGoRound),
		{Content: "spare answer.", StopReason: "end_turn"},
	}}
	r, out := newApproveBudgetREPL(t, adapter, "print the go version, filter it, then record a marker file\n"+inputTail+"/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	return r, adapter, out.String()
}

func requireLintLaneRemainder(t *testing.T, lane string, r *REPL, adapter *scriptedChatAdapter, printed string) {
	t.Helper()
	if r.pendingOperation != nil {
		t.Fatalf("%s lane: the repair must not stay pending: %+v", lane, r.pendingOperation)
	}
	statuses, executed, failed := approveBudgetStatuses(r)
	if len(adapter.calls) != 7 {
		t.Fatalf("%s lane: adapter calls=%d, want 7 (planner, continuation, replanner, three continuations, answer) — a different count means the resumed counter was derived from the own rounds instead of carried; window=%v\n%s", lane, len(adapter.calls), statuses, printed)
	}
	// seven rounds — the go round, the lint-failed continuation, the repair,
	// three continuations and the budget terminal — of which the REPL keeps
	// the last six: the first go round falls off the capped window
	want := []operation.OperationStatus{operation.StatusFailed, operation.StatusExecuted, operation.StatusExecuted, operation.StatusExecuted, operation.StatusExecuted, operation.StatusBudgetExhausted}
	if executed != commandOperationMaxCommandRounds-1 || failed != 1 || len(statuses) != len(want) {
		t.Fatalf("%s lane: executed=%d failed=%d window=%v, want %d/1 over %v", lane, executed, failed, statuses, commandOperationMaxCommandRounds-1, want)
	}
	for i := range want {
		if statuses[i] != want[i] {
			t.Fatalf("%s lane: window=%v, want %v", lane, statuses, want)
		}
	}
	last := r.operationResults[len(r.operationResults)-1]
	if last.Result.Status != operation.StatusBudgetExhausted || !strings.Contains(last.Result.OutputPreview, "command-round budget exhausted") {
		t.Fatalf("%s lane: the operation must end with the command-round budget terminal, last status=%s preview=%q; window=%v", lane, last.Result.Status, last.Result.OutputPreview, statuses)
	}
}

// TestCommandOperationE2E_ApprovedRepairAfterLintFailedContinuationGrantsSameRemainder
// (pin j): a lint-failed continuation appends a synthetic failed own round
// without spending the counter, so the repair parks with CommandRounds=1
// and two own rounds. /approve resumes at the carried counter — not at
// len(Own) — so the approved repair and its continue_after chain are
// granted the same remainder the auto-execute lane grants the identical
// script: four more executed rounds, seven adapter calls in both lanes.
//
// EVOLUTION RECORD (review round seven #1): pin h excludes only the
// executed-count re-derivation of the counter; `commandRounds :=
// len(resume.Own)` at the attempt entry kept every pin green because every
// pinned park had CommandRounds == len(Own). Under that mutation (scratch
// overlay, not committed) this pin's approve lane resumes at 2 and grants
// one round fewer than the auto lane — six calls against seven — the
// asymmetry the equal-remainder ruling forbids; the live tree carries the
// counter verbatim and both lanes are green. Green on 6f98f839d (counter
// carried verbatim) and on 533a939fb (its executed-count derivation
// yields 1 on this shape, equal to the carried value); red on b6f7eeec3
// and 480939385 (approve lane eight calls: /approve resumed from a fresh
// state and granted five executed rounds after the repair).
func TestCommandOperationE2E_ApprovedRepairAfterLintFailedContinuationGrantsSameRemainder(t *testing.T) {
	highRepair := `{"status":"ready","risk_level":"high","requires_confirmation":true,"continue_after":true,"work_dir":".","goal":"write fallback","steps":[{"id":"write","title":"write fallback","program":"touch","args":["` + filepath.Join(t.TempDir(), "marker") + `"],"risk_level":"high","side_effects":["local_file_write"]}]}`
	lowRepair := `{"status":"ready","risk_level":"low","requires_confirmation":false,"continue_after":true,"work_dir":".","goal":"write fallback","steps":[{"id":"write","title":"write fallback","program":"touch","args":["` + filepath.Join(t.TempDir(), "marker") + `"],"risk_level":"low","side_effects":[]}]}`
	autoREPL, autoAdapter, autoPrinted := approveBudgetLintLane(t, lowRepair, "")
	requireLintLaneRemainder(t, "auto", autoREPL, autoAdapter, autoPrinted)
	approveREPL, approveAdapter, approvePrinted := approveBudgetLintLane(t, highRepair, "/approve\n")
	requireLintLaneRemainder(t, "approve", approveREPL, approveAdapter, approvePrinted)
	if len(approveAdapter.calls) != len(autoAdapter.calls) {
		t.Fatalf("approve lane adapter calls=%d, auto lane=%d — the two lanes must grant the same command-round remainder", len(approveAdapter.calls), len(autoAdapter.calls))
	}
}
