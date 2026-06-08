package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestFinalDataTaskAnswerForCLIRejectsArtifactSummary(t *testing.T) {
	plan := dataquery.TaskPlan{
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
	}
	result := dataquery.Result{
		Answer:         "4 artifact(s)",
		OutputContract: plan.OutputContract,
	}
	answer, err := finalDataTaskAnswerForCLI("", nil, plan, result, "en")
	if err == nil {
		t.Fatalf("answer=%q, want artifact summary rejected", answer)
	}
	if !strings.Contains(err.Error(), "final answer candidate") {
		t.Fatalf("err=%v, want final answer candidate guard", err)
	}
}

func TestFinalDataTaskAnswerForCLIAcceptsTypedFinalAnswer(t *testing.T) {
	plan := dataquery.TaskPlan{
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
	}
	result := dataquery.Result{
		Answer:         "42",
		OutputContract: plan.OutputContract,
	}
	answer, err := finalDataTaskAnswerForCLI("", nil, plan, result, "en")
	if err != nil {
		t.Fatalf("finalDataTaskAnswerForCLI err=%v", err)
	}
	if strings.TrimSpace(answer) != "42" {
		t.Fatalf("answer=%q, want 42", answer)
	}
}
