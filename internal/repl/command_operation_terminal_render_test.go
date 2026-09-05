package repl

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/operation"
)

// command_operation_terminal_render_test.go — a terminal that is not a
// step outcome mints no synthetic step (colleague_merge_audit §40.52,
// batch six fold-in review round seven #2): the budget terminal, the
// degraded planner round and the evaluator verdict carry their class on
// the result, render through the renderer's step-less Note shape with one
// readable sentence per language, and still hand the evaluator, the
// answerer and the classifier handoff the failure class they read before.

func terminalRenderPlan() operation.CommandOperationPlan {
	return operation.CommandOperationPlan{ID: "op-1", Status: operation.StatusExecuted}
}

func terminalRenderStructured(t *testing.T, lang string) operation.CommandOperationResult {
	t.Helper()
	_, err := unmarshalCommandOperationPlan([]byte("ä not json"))
	result, ok := commandOperationStructuredToolParamFailureResult(terminalRenderPlan(), err, lang)
	if !ok {
		t.Fatal("expected structured tool params degradation")
	}
	return result
}

// terminalRenderEvaluation mints the evaluator terminal for one verdict
// with the reason every evaluator case shares.
func terminalRenderEvaluation(t *testing.T, status operation.OperationEvaluationStatus) operation.CommandOperationResult {
	t.Helper()
	result, ok := commandOperationEvaluationTerminalResult(terminalRenderPlan(), operation.OperationEvaluation{
		Status: status,
		Reason: "the material is exhausted",
	})
	if !ok {
		t.Fatalf("expected an evaluator terminal for %s", status)
	}
	return result
}

// TestCommandOperationTerminalRendersStepless pins the panel markdown of
// every result-level terminal in both languages.
//
// EVOLUTION RECORD (review round seven #2): on 42fcf3fd1 (and every older
// base back to af5825f3a) each of these results minted one StepResult with
// no StepID and the reason in both Error and OutputPreview, so the panel
// printed a "1. step" row with an empty backticked id and the status,
// followed by the reason twice (Error and Output) — in the approve,
// auto-execute and CLI lanes, EN and ZH alike.
//
// EVOLUTION RECORD (review round eight #1): on 42d2017e4 the round card's
// status switch had no arm for StatusBlocked / StatusNeedsClarification,
// so an evaluator verdict of blocked or needs_clarification rendered under
// the default "failed" header with no status word on the card (the
// synthetic step row that used to carry the word was removed by round
// seven #2): the evaluation_blocked_* and evaluation_clarification_* cases
// below were red there ("Operation plan `op-1` failed." / "操作计划
// `op-1` 执行失败。").
func TestCommandOperationTerminalRendersStepless(t *testing.T) {
	plan := terminalRenderPlan()
	for _, tc := range []struct {
		name   string
		lang   string
		result operation.CommandOperationResult
		class  string
		want   string
	}{
		{"command_budget_en", "en", commandOperationBudgetResult(plan, commandOperationBudgetCommandRounds, "en"), "budget_exhausted",
			"Operation plan `op-1` reached the operation budget.\n\nNote:\ncommand operation command-round budget exhausted before the user goal was fully satisfied"},
		{"command_budget_zh", "zh", commandOperationBudgetResult(plan, commandOperationBudgetCommandRounds, "zh"), "budget_exhausted",
			"操作计划 `op-1` 已达到本轮预算。\n\n说明：\n命令轮次预算已用尽，用户目标尚未完全达成，将基于已有结果作答。"},
		{"repair_budget_en", "en", commandOperationBudgetResult(plan, commandOperationBudgetRepairRounds, "en"), "budget_exhausted",
			"Operation plan `op-1` reached the operation budget.\n\nNote:\ncommand operation repair budget exhausted before the user goal was fully satisfied"},
		{"repair_budget_zh", "zh", commandOperationBudgetResult(plan, commandOperationBudgetRepairRounds, "zh"), "budget_exhausted",
			"操作计划 `op-1` 已达到本轮预算。\n\n说明：\n修复轮次预算已用尽，用户目标尚未完全达成，将基于已有结果作答。"},
		{"structured_en", "en", terminalRenderStructured(t, "en"), "structured_tool_params",
			"Operation plan `op-1` failed.\n\nNote:\nstructured tool parameters were malformed after compact repair; no further operation commands were executed"},
		{"structured_zh", "zh", terminalRenderStructured(t, "zh"), "structured_tool_params",
			"操作计划 `op-1` 执行失败。\n\n说明：\n结构化工具参数在紧凑修复后仍不合法；系统未执行新的操作命令，已基于现有结果降级作答"},
		{"evaluation_budget_en", "en", terminalRenderEvaluation(t, operation.EvalBudgetExhausted), "budget_exhausted",
			"Operation plan `op-1` reached the operation budget.\n\nNote:\nthe material is exhausted"},
		{"evaluation_budget_zh", "zh", terminalRenderEvaluation(t, operation.EvalBudgetExhausted), "budget_exhausted",
			"操作计划 `op-1` 已达到本轮预算。\n\n说明：\nthe material is exhausted"},
		{"evaluation_partial_en", "en", terminalRenderEvaluation(t, operation.EvalPartialAnswerPossible), "partial_answer_possible",
			"Operation plan `op-1` produced a partial result.\n\nNote:\nthe material is exhausted"},
		{"evaluation_partial_zh", "zh", terminalRenderEvaluation(t, operation.EvalPartialAnswerPossible), "partial_answer_possible",
			"操作计划 `op-1` 已形成部分结果。\n\n说明：\nthe material is exhausted"},
		{"evaluation_blocked_en", "en", terminalRenderEvaluation(t, operation.EvalBlocked), "blocked",
			"Operation plan `op-1` was blocked by policy or capability limits.\n\nNote:\nthe material is exhausted"},
		{"evaluation_blocked_zh", "zh", terminalRenderEvaluation(t, operation.EvalBlocked), "blocked",
			"操作计划 `op-1` 已被策略或能力边界阻止。\n\n说明：\nthe material is exhausted"},
		{"evaluation_clarification_en", "en", terminalRenderEvaluation(t, operation.EvalNeedsClarification), "needs_clarification",
			"Operation plan `op-1` needs clarification before it can continue.\n\nNote:\nthe material is exhausted"},
		{"evaluation_clarification_zh", "zh", terminalRenderEvaluation(t, operation.EvalNeedsClarification), "needs_clarification",
			"操作计划 `op-1` 需要补充信息后才能继续。\n\n说明：\nthe material is exhausted"},
	} {
		if len(tc.result.StepResults) != 0 {
			t.Errorf("%s: a terminal must not mint a synthetic step: %+v", tc.name, tc.result.StepResults)
		}
		if tc.result.FailureClass != tc.class || commandOperationPrimaryFailureClass(tc.result) != tc.class {
			t.Errorf("%s: failure class=%q primary=%q, want %q", tc.name, tc.result.FailureClass, commandOperationPrimaryFailureClass(tc.result), tc.class)
		}
		got := commandOperationResultMarkdown(tc.lang, plan, tc.result)
		if strings.Contains(got, "step `") {
			t.Errorf("%s: panel renders a synthetic step:\n%s", tc.name, got)
		}
		if got != tc.want {
			t.Errorf("%s: markdown=\n%s\nwant\n%s", tc.name, got, tc.want)
		}
		if c := strings.Count(got, tc.result.OutputPreview); c != 1 {
			t.Errorf("%s: the reason must appear exactly once, got %d:\n%s", tc.name, c, got)
		}
	}
	// a terminal writes no command memory entry: there is no command to
	// remember (the synthetic step used to yield a command-less entry)
	if entries := operation.BuildMemoryEntries(plan, commandOperationBudgetResult(plan, commandOperationBudgetCommandRounds, "en"), operation.CapabilitySnapshot{}); len(entries) != 0 {
		t.Fatalf("budget terminal minted %d command memory entries, want none: %+v", len(entries), entries)
	}
}

// TestCommandOperationEvaluatorVerdictStatusesHaveCardArms enumerates the
// closed evaluator verdict set — the enum the evaluator tool schema hands
// the model, cross-checked against the typed constants — and pins that
// every verdict which mints a result-level terminal has its own round-card
// header in both languages: a known status falling through to the default
// "failed" header fails here, as does a terminal verdict the header roster
// does not name, a schema enum entry without a typed constant, or a roster
// entry no verdict reaches (review round eight #1).
func TestCommandOperationEvaluatorVerdictStatusesHaveCardArms(t *testing.T) {
	var schema struct {
		Properties struct {
			Status struct {
				Enum []string `json:"enum"`
			} `json:"status"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(operationEvaluationTool.Parameters, &schema); err != nil {
		t.Fatalf("decode evaluator tool schema: %v", err)
	}
	typed := []operation.OperationEvaluationStatus{
		operation.EvalComplete, operation.EvalContinueCommand, operation.EvalContinueProvider, operation.EvalNeedsApproval,
		operation.EvalNeedsClarification, operation.EvalBlocked, operation.EvalBudgetExhausted, operation.EvalPartialAnswerPossible,
	}
	if len(schema.Properties.Status.Enum) != len(typed) {
		t.Fatalf("evaluator schema enum=%v, typed constants=%v — the closed set changed; extend the roster", schema.Properties.Status.Enum, typed)
	}
	for _, raw := range schema.Properties.Status.Enum {
		if operation.NormalizeEvaluationStatus(raw) != operation.OperationEvaluationStatus(raw) {
			t.Fatalf("schema enum entry %q has no typed constant", raw)
		}
	}
	// the verdicts that mint no terminal: the loop continues, approves or
	// completes on them
	nonTerminal := map[operation.OperationEvaluationStatus]bool{
		operation.EvalComplete: true, operation.EvalContinueCommand: true, operation.EvalContinueProvider: true, operation.EvalNeedsApproval: true,
	}
	// the round-card header of every terminal verdict, EN then ZH
	headers := map[operation.OperationStatus][2]string{
		operation.StatusBudgetExhausted:    {"Operation plan `op-1` reached the operation budget.", "操作计划 `op-1` 已达到本轮预算。"},
		operation.StatusPartialAnswer:      {"Operation plan `op-1` produced a partial result.", "操作计划 `op-1` 已形成部分结果。"},
		operation.StatusBlocked:            {"Operation plan `op-1` was blocked by policy or capability limits.", "操作计划 `op-1` 已被策略或能力边界阻止。"},
		operation.StatusNeedsClarification: {"Operation plan `op-1` needs clarification before it can continue.", "操作计划 `op-1` 需要补充信息后才能继续。"},
	}
	failedHeader := [2]string{"Operation plan `op-1` failed.", "操作计划 `op-1` 执行失败。"}
	plan := terminalRenderPlan()
	terminals := 0
	for _, verdict := range typed {
		status := commandOperationStatusFromEvaluation(verdict)
		if status == "" {
			if !nonTerminal[verdict] {
				t.Fatalf("evaluator verdict %s mints no terminal and is not on the non-terminal roster", verdict)
			}
			continue
		}
		if nonTerminal[verdict] {
			t.Fatalf("evaluator verdict %s is on the non-terminal roster but mints a %s terminal", verdict, status)
		}
		want, ok := headers[status]
		if !ok {
			t.Fatalf("evaluator verdict %s mints a %s terminal the header roster does not name", verdict, status)
		}
		terminals++
		result := terminalRenderEvaluation(t, verdict)
		for i, lang := range []string{"en", "zh"} {
			card := commandOperationResultMarkdown(lang, plan, result)
			header := strings.SplitN(card, "\n", 2)[0]
			if header == failedHeader[i] {
				t.Errorf("%s/%s: the %s verdict fell through to the default failed header:\n%s", verdict, lang, status, card)
			}
			if header != want[i] {
				t.Errorf("%s/%s: header=%q, want %q", verdict, lang, header, want[i])
			}
		}
	}
	if terminals != len(headers) {
		t.Fatalf("terminal verdicts=%d, header roster=%d — a roster entry no verdict reaches", terminals, len(headers))
	}
}

// TestCommandOperationTerminalFailureClassReachesPromptsWithoutStep pins
// that the evaluator, the answerer, the replanner context and the
// classifier handoff still see the terminal's failure class — now from
// the result line — although no step row exists.
func TestCommandOperationTerminalFailureClassReachesPromptsWithoutStep(t *testing.T) {
	plan := terminalRenderPlan()
	budget := commandOperationBudgetResult(plan, commandOperationBudgetCommandRounds, "en")
	structured := terminalRenderStructured(t, "en")

	rendered := renderCommandResultForPrompt(budget)
	if !strings.Contains(rendered, "previous_result plan_id=op-1 status=budget_exhausted failure_class=budget_exhausted\n") || strings.Contains(rendered, "result[1]") {
		t.Fatalf("prompt result line must carry the class without a step row:\n%s", rendered)
	}
	repair := operationResultRepairContext(structured)
	if !strings.Contains(repair, "result plan_id=op-1 status=failed failure_class=structured_tool_params output_preview=") || strings.Contains(repair, "result[1]") {
		t.Fatalf("repair context must carry the class without a step row:\n%s", repair)
	}
	// a step outcome keeps its class on the step: no result-level echo
	step := operation.CommandOperationResult{PlanID: "op-2", Status: operation.StatusFailed, StepResults: []operation.CommandStepResult{{StepID: "s1", Status: operation.StatusFailed, ExitCode: 2, FailureClass: "nonzero_exit"}}}
	if got := renderCommandResultForPrompt(step); !strings.Contains(got, "previous_result plan_id=op-2 status=failed\n") || !strings.Contains(got, "step_id=s1 status=failed exit_code=2 timed_out=false failure_class=nonzero_exit") {
		t.Fatalf("step outcome rendering changed:\n%s", got)
	}

	records := []commandOperationResultRecord{{Plan: plan, Result: budget}}
	userPrompt := func(t *testing.T, adapter *scriptedChatAdapter) string {
		t.Helper()
		if len(adapter.calls) != 1 {
			t.Fatalf("Chat calls=%d, want 1", len(adapter.calls))
		}
		user := ""
		for _, msg := range adapter.calls[0].messages {
			if msg.Role == "user" {
				user = msg.Content
			}
		}
		return user
	}
	answerAdapter := &scriptedChatAdapter{responses: []llm.Response{{Content: "收到"}}}
	answerer := NewCommandOperationPlanner(answerAdapter).(CommandOperationRecordsAnswerer)
	if _, err := answerer.AnswerCommandOperationRecords(context.Background(), "read the long page", records, "en"); err != nil {
		t.Fatalf("AnswerCommandOperationRecords: %v", err)
	}
	if user := userPrompt(t, answerAdapter); !strings.Contains(user, "status=budget_exhausted failure_class=budget_exhausted") {
		t.Fatalf("answerer prompt lost the terminal's failure class:\n%s", user)
	}
	evalAdapter := &scriptedChatAdapter{responses: []llm.Response{operationEvaluationResp(`{"status":"complete","reason":"done","confidence":"high","material_coverage_status":"complete"}`)}}
	evaluator := NewCommandOperationPlanner(evalAdapter).(CommandOperationEvaluator)
	if _, err := evaluator.EvaluateCommandOperation(context.Background(), "read the long page", records, "en"); err != nil {
		t.Fatalf("EvaluateCommandOperation: %v", err)
	}
	if user := userPrompt(t, evalAdapter); !strings.Contains(user, "status=budget_exhausted failure_class=budget_exhausted") {
		t.Fatalf("evaluator prompt lost the terminal's failure class:\n%s", user)
	}
	r := &REPL{operationResults: records}
	if handoff := r.renderCommandOperationHandoff(); !strings.Contains(handoff, "operation_result plan_id=op-1 status=budget_exhausted failure_class=budget_exhausted request=") {
		t.Fatalf("classifier handoff lost the terminal's failure class:\n%s", handoff)
	}
}

// TestCommandOperationFailedRoundCommandBudgetExhaustedReadsTheLimit pins
// the precise signal of the round-seven #0 budget arm: a failed round at
// the operation's command-round limit, where the limit is the
// evaluation-granted extended limit when the own rounds earned it, so
// replanning stays allowed between the base and the extended limit.
func TestCommandOperationFailedRoundCommandBudgetExhaustedReadsTheLimit(t *testing.T) {
	failed := operation.CommandOperationResult{Status: operation.StatusFailed, StepResults: []operation.CommandStepResult{{StepID: "s1", Status: operation.StatusFailed, ExitCode: 2}}}
	timedOut := operation.CommandOperationResult{Status: operation.StatusFailed, StepResults: []operation.CommandStepResult{{StepID: "s1", Status: operation.StatusFailed, TimedOut: true}}}
	executed := operation.CommandOperationResult{Status: operation.StatusExecuted}
	base := make(commandOperationOwnRecords, 0, commandOperationMaxCommandRounds)
	for i := 0; i < commandOperationMaxCommandRounds; i++ {
		base = append(base, commandOperationResultRecord{Result: operation.CommandOperationResult{Status: operation.StatusExecuted}})
	}
	extended := make(commandOperationOwnRecords, 0, commandOperationMaxCommandRounds)
	for i := 0; i < commandOperationMaxCommandRounds; i++ {
		record := commandOperationResultRecord{Result: operation.CommandOperationResult{Status: operation.StatusExecuted, PayloadRef: "/tmp/material-page.txt"}}
		if i == commandOperationMaxCommandRounds-1 {
			record.Evaluation = &operation.OperationEvaluation{Status: operation.EvalContinueCommand, MaterialCoverageStatus: operation.MaterialCoveragePartial}
		}
		extended = append(extended, record)
	}
	if got := commandOperationCommandRoundLimit(extended); got != commandOperationExtendedCommandRounds {
		t.Fatalf("extended own rounds limit=%d, want %d", got, commandOperationExtendedCommandRounds)
	}
	for _, tc := range []struct {
		name   string
		result operation.CommandOperationResult
		rounds int
		own    commandOperationOwnRecords
		want   bool
	}{
		{"failed_at_base_limit", failed, commandOperationMaxCommandRounds, base, true},
		{"failed_below_base_limit", failed, commandOperationMaxCommandRounds - 1, base, false},
		{"failed_at_base_limit_with_extension_granted", failed, commandOperationMaxCommandRounds, extended, false},
		{"failed_at_extended_limit", failed, commandOperationExtendedCommandRounds, extended, true},
		{"timed_out_at_base_limit", timedOut, commandOperationMaxCommandRounds, base, false},
		{"executed_at_base_limit", executed, commandOperationMaxCommandRounds, base, false},
	} {
		if got := commandOperationFailedRoundCommandBudgetExhausted(tc.result, tc.rounds, tc.own); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
