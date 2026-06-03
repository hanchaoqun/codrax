package operation

import "testing"

func TestNormalizeEvaluationStatus(t *testing.T) {
	tests := []struct {
		in   string
		want OperationEvaluationStatus
	}{
		{"complete", EvalComplete},
		{"continue_command", EvalContinueCommand},
		{"command", EvalContinueCommand},
		{"continue", EvalContinueCommand},
		{"continue_provider", EvalContinueProvider},
		{"provider", EvalContinueProvider},
		{"needs_clarification", EvalNeedsClarification},
		{"blocked", EvalBlocked},
		{"budget_exhausted", EvalBudgetExhausted},
		{"partial_answer_possible", EvalPartialAnswerPossible},
		{"unexpected", EvalPartialAnswerPossible},
		{"", EvalPartialAnswerPossible},
	}
	for _, tt := range tests {
		if got := NormalizeEvaluationStatus(tt.in); got != tt.want {
			t.Fatalf("NormalizeEvaluationStatus(%q)=%q want %q", tt.in, got, tt.want)
		}
	}
}

func TestOperationEvaluationIsTerminal(t *testing.T) {
	terminal := []OperationEvaluationStatus{
		EvalComplete,
		EvalBlocked,
		EvalBudgetExhausted,
		EvalPartialAnswerPossible,
	}
	for _, status := range terminal {
		if !(OperationEvaluation{Status: status}).IsTerminal() {
			t.Fatalf("%s should be terminal", status)
		}
	}
	nonTerminal := []OperationEvaluationStatus{
		EvalContinueCommand,
		EvalContinueProvider,
		EvalNeedsApproval,
		EvalNeedsClarification,
	}
	for _, status := range nonTerminal {
		if (OperationEvaluation{Status: status}).IsTerminal() {
			t.Fatalf("%s should not be terminal", status)
		}
	}
}
