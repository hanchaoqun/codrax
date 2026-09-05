package repl

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/operation"
)

func commandOperationShouldRunMaterialEvaluator(records []commandOperationResultRecord) bool {
	if len(records) == 0 {
		return false
	}
	last := records[len(records)-1]
	if last.Result.Status != operation.StatusExecuted {
		return false
	}
	for _, ref := range commandOperationRecordPayloadRefs(last) {
		if strings.TrimSpace(ref) != "" {
			return true
		}
	}
	for _, step := range last.Result.StepResults {
		if strings.TrimSpace(step.PayloadRef) != "" {
			return true
		}
	}
	return false
}

func commandOperationAttachEvaluation(records []commandOperationResultRecord, eval operation.OperationEvaluation) []commandOperationResultRecord {
	if len(records) == 0 {
		return records
	}
	evalCopy := eval
	records[len(records)-1].Evaluation = &evalCopy
	return records
}

// commandOperationCommandRoundLimit grants one bounded extension only after
// the material evaluator has inspected the base-limit observation and emitted
// typed evidence that material-backed work must continue. Command approval is
// still evaluated for every generated continuation plan, so this changes
// investigation capacity without widening execution authority.
//
// The limit reads the operation's OWN rounds only (review round five #2):
// an evaluation carried by an earlier operation's round in the context
// window never extends a fresh operation's limit.
func commandOperationCommandRoundLimit(ownRecords commandOperationOwnRecords) int {
	executed := 0
	for i := range ownRecords {
		if ownRecords[i].Result.Status == operation.StatusExecuted {
			executed++
		}
		if executed < commandOperationMaxCommandRounds || ownRecords[i].Evaluation == nil {
			continue
		}
		eval := ownRecords[i].Evaluation
		if eval.Status != operation.EvalContinueCommand ||
			(eval.MaterialCoverageStatus != operation.MaterialCoveragePartial &&
				eval.MaterialCoverageStatus != operation.MaterialCoverageNotEvaluated) {
			continue
		}
		if commandOperationShouldRunMaterialEvaluator(ownRecords[:i+1]) {
			return commandOperationExtendedCommandRounds
		}
	}
	return commandOperationMaxCommandRounds
}

// commandOperationMaterialEvaluationNeedsBudget reads the operation's OWN
// rounds only (review round five #2).
func commandOperationMaterialEvaluationNeedsBudget(result operation.CommandOperationResult, eval *operation.OperationEvaluation, ownRecords commandOperationOwnRecords) bool {
	if result.Status != operation.StatusExecuted || !commandOperationShouldRunMaterialEvaluator(ownRecords) {
		return false
	}
	if commandOperationExecutedRoundCount(ownRecords) < commandOperationCommandRoundLimit(ownRecords) {
		return false
	}
	return eval == nil || eval.Status != operation.EvalComplete
}

func commandOperationEvaluationTerminalResult(plan operation.CommandOperationPlan, eval operation.OperationEvaluation) (operation.CommandOperationResult, bool) {
	status := commandOperationStatusFromEvaluation(eval.Status)
	if status == "" {
		return operation.CommandOperationResult{}, false
	}
	reason := strings.TrimSpace(eval.Reason)
	if reason == "" {
		reason = strings.TrimSpace(string(eval.Status))
	}
	return operation.CommandOperationResult{
		PlanID:        plan.ID,
		Status:        status,
		OutputPreview: reason,
		StepResults: []operation.CommandStepResult{{
			Status:        status,
			OutputPreview: reason,
			Error:         commandOperationEvaluationErrorForStatus(status, reason),
			FailureClass:  commandOperationEvaluationFailureClass(status),
		}},
	}, true
}

func commandOperationStatusFromEvaluation(status operation.OperationEvaluationStatus) operation.OperationStatus {
	switch status {
	case operation.EvalBlocked:
		return operation.StatusBlocked
	case operation.EvalBudgetExhausted:
		return operation.StatusBudgetExhausted
	case operation.EvalPartialAnswerPossible:
		return operation.StatusPartialAnswer
	case operation.EvalNeedsClarification:
		return operation.StatusNeedsClarification
	default:
		return ""
	}
}

func commandOperationEvaluationFailureClass(status operation.OperationStatus) string {
	switch status {
	case operation.StatusBudgetExhausted:
		return "budget_exhausted"
	case operation.StatusPartialAnswer:
		return "partial_answer_possible"
	case operation.StatusBlocked:
		return "blocked"
	case operation.StatusNeedsClarification:
		return "needs_clarification"
	default:
		return ""
	}
}

func commandOperationEvaluationErrorForStatus(status operation.OperationStatus, reason string) string {
	switch status {
	case operation.StatusBudgetExhausted, operation.StatusPartialAnswer, operation.StatusBlocked, operation.StatusNeedsClarification:
		return reason
	default:
		return ""
	}
}
