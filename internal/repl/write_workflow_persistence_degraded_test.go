package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// 件3 (status card): a persistence-degraded run appends the customer-facing
// disclosure to whatever status/next-action card it produces.
func TestWriteWorkflowNextActionLinesDisclosesPersistenceDegraded(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:                     "wf-repl",
		Status:                    types.WriteWorkflowRunInProgress,
		ActiveBatchID:             "b",
		PersistenceDegraded:       true,
		PersistenceDegradedReason: types.WriteWorkflowPersistenceDegradedNoStore,
		Batches: []types.WriteWorkflowBatch{{
			ID:     "b",
			Status: types.WriteWorkflowBatchApplying,
		}},
	}
	joined := strings.Join(writeWorkflowNextActionLines("en", run), "\n")
	if !strings.Contains(joined, "could not be fully saved") {
		t.Fatalf("status card must disclose persistence degradation, got:\n%s", joined)
	}
}

// 件3 (completion side-note): a completed run whose record is degraded still
// carries the disclosure on the completion card.
func TestWriteWorkflowNextActionLinesDisclosesPersistenceDegradedOnComplete(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:                     "wf-repl-complete",
		Status:                    types.WriteWorkflowRunComplete,
		ActiveBatchID:             "b",
		PersistenceDegraded:       true,
		PersistenceDegradedReason: types.WriteWorkflowPersistenceDegradedSaveFailed,
		Batches: []types.WriteWorkflowBatch{{
			ID:     "b",
			Status: types.WriteWorkflowBatchComplete,
			Completion: &types.WriteWorkflowCompletion{
				Verdict:    types.WriteWorkflowCompletionVerified,
				ReasonCode: "all_tests_pass",
			},
		}},
	}
	joined := strings.Join(writeWorkflowNextActionLines("en", run), "\n")
	if !strings.Contains(joined, "could not be fully saved") {
		t.Fatalf("completion card must disclose persistence degradation, got:\n%s", joined)
	}
}

// A healthy run shows no persistence note.
func TestWriteWorkflowNextActionLinesNoPersistenceNoteWhenHealthy(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-repl-ok",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "b",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "b",
			Status: types.WriteWorkflowBatchApplying,
		}},
	}
	joined := strings.Join(writeWorkflowNextActionLines("en", run), "\n")
	if strings.Contains(joined, "could not be fully saved") {
		t.Fatalf("healthy run must not show a persistence note, got:\n%s", joined)
	}
}
