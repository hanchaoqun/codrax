package operation

import (
	"strings"
	"testing"
)

func TestSummarizeStepOutputClassifiesHelpText(t *testing.T) {
	got := SummarizeStepOutput(CommandStepResult{
		Status:        StatusExecuted,
		OutputPreview: "Usage: demo-tool [OPTIONS]\n  --input FILE\n  --format json\n",
	})
	if got.Kind != "help_text" || !strings.Contains(got.Summary, "help text") {
		t.Fatalf("unexpected summary: %+v", got)
	}
}

func TestSummarizeStepOutputClassifiesSearchHits(t *testing.T) {
	got := SummarizeStepOutput(CommandStepResult{
		Status:        StatusExecuted,
		OutputPreview: "a.log:12:error one\nb.log:45:error two\n",
	})
	if got.Kind != "search_hits" || !strings.Contains(got.Summary, "first hit") {
		t.Fatalf("unexpected summary: %+v", got)
	}
}

func TestSummarizeStepOutputClassifiesFailure(t *testing.T) {
	got := SummarizeStepOutput(CommandStepResult{
		Status:        StatusFailed,
		FailureClass:  "command_not_found",
		OutputPreview: "missing: command not found",
	})
	if got.Kind != "error_output" || !strings.Contains(got.Summary, "command_not_found") {
		t.Fatalf("unexpected summary: %+v", got)
	}
}
