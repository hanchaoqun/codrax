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
