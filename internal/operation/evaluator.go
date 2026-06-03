package operation

import "strings"

// OperationEvaluationStatus is the typed verdict produced by the operation
// lane's goal evaluator. It is separate from OperationStatus because it decides
// what should happen next across command/provider/material records; it does not
// describe whether any one command or provider action executed.
type OperationEvaluationStatus string

const (
	EvalComplete              OperationEvaluationStatus = "complete"
	EvalContinueCommand       OperationEvaluationStatus = "continue_command"
	EvalContinueProvider      OperationEvaluationStatus = "continue_provider"
	EvalNeedsApproval         OperationEvaluationStatus = "needs_approval"
	EvalNeedsClarification    OperationEvaluationStatus = "needs_clarification"
	EvalBlocked               OperationEvaluationStatus = "blocked"
	EvalBudgetExhausted       OperationEvaluationStatus = "budget_exhausted"
	EvalPartialAnswerPossible OperationEvaluationStatus = "partial_answer_possible"
)

// OperationEvaluation is a compact operation-lane decision. It is not source
// evidence and it does not execute anything; REPL orchestration still routes
// follow-up work through the existing command/provider executors and approval
// policy.
type OperationEvaluation struct {
	Status        OperationEvaluationStatus
	Reason        string
	Confidence    string
	MissingInputs []string
	Materials     []OperationMaterial
}

func NormalizeEvaluationStatus(status string) OperationEvaluationStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case string(EvalComplete):
		return EvalComplete
	case string(EvalContinueCommand), "command", "continue":
		return EvalContinueCommand
	case string(EvalContinueProvider), "provider":
		return EvalContinueProvider
	case string(EvalNeedsApproval):
		return EvalNeedsApproval
	case string(EvalNeedsClarification):
		return EvalNeedsClarification
	case string(EvalBlocked):
		return EvalBlocked
	case string(EvalBudgetExhausted):
		return EvalBudgetExhausted
	case string(EvalPartialAnswerPossible):
		return EvalPartialAnswerPossible
	default:
		return EvalPartialAnswerPossible
	}
}

func (e OperationEvaluation) IsTerminal() bool {
	switch e.Status {
	case EvalComplete, EvalBlocked, EvalBudgetExhausted, EvalPartialAnswerPossible:
		return true
	default:
		return false
	}
}
