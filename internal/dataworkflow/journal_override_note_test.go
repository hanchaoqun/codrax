package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

// Batch-6 B4: when the completion gate normalizes a final evaluator
// verdict (repair_node -> complete), the typed override marker must be
// auditable on the top-level terminal decision/reason — not buried in
// resume.records[last].Evaluation only.
func TestLatestEvaluationOverrideNoteSurfaced(t *testing.T) {
	records := []WorkflowRecord{
		{Evaluation: &dataquery.Evaluation{Status: "complete", Reason: "all good"}},
		{Evaluation: &dataquery.Evaluation{
			Status: "complete",
			Reason: "assemble_answer produced \"30,0\"; current typed workflow state is complete; original_status=repair_node",
		}},
	}
	note := latestEvaluationOverrideNote(records)
	if !strings.Contains(note, "original_status=repair_node") {
		t.Fatalf("override note must carry the typed marker, got %q", note)
	}

	if latestEvaluationOverrideNote(records[:1]) != "" {
		t.Fatal("records without an override marker must yield no note")
	}
}
