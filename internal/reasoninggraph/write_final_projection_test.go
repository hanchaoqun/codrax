package reasoninggraph

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestWriteFinalReasoningGraphSummaryFromRunProjectsWorkflowCounts(t *testing.T) {
	run := &types.WriteWorkflowRun{
		RunID:         "wf-summary",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}
	got := WriteFinalReasoningGraphSummaryFromRun(run, 2)
	if got == nil {
		t.Fatal("summary must be projected")
	}
	if got.GraphID != "wf-summary" || got.EventCount == 0 || got.WorkflowEventCount == 0 || got.NodeCount == 0 {
		t.Fatalf("unexpected summary: %+v", got)
	}
	if len(got.EventRefs) == 0 || len(got.EventRefs) > 2 {
		t.Fatalf("event refs should be capped and non-empty: %+v", got.EventRefs)
	}
}

func TestWriteFinalReasoningGraphSummaryFromRunNilSafe(t *testing.T) {
	if got := WriteFinalReasoningGraphSummaryFromRun(nil, 0); got != nil {
		t.Fatalf("nil run should not project summary: %+v", got)
	}
}
