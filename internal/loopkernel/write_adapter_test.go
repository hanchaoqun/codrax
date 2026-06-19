package loopkernel

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEventsFromWriteWorkflowRunPendingApprovalProjectsAsk(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-approval",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchPendingApproval,
			PlanID: "plan-1",
		}},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.Status != LoopRunStatusInProgress {
		t.Fatalf("status = %s, want in_progress", got.Status)
	}
	if got.Permission.State != PermissionAuthorityAsk || !got.Permission.RequiresUser {
		t.Fatalf("permission ask not projected: %+v", got.Permission)
	}
	if got.ActiveUnitID != "batch:batch-1" {
		t.Fatalf("active unit = %q", got.ActiveUnitID)
	}
}

func TestEventsFromWriteWorkflowRunVerifyingUsesActiveSlice(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-verify",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchVerifying,
			PlanID:        "plan-1",
			ActiveSliceID: "slice-2",
			Slices: []types.WriteWorkflowSlice{{
				ID:     "slice-1",
				Status: types.ChangePlanSliceVerified,
			}, {
				ID:     "slice-2",
				Status: types.ChangePlanSliceObserving,
			}},
		}},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.ActiveUnitID != "slice-2" {
		t.Fatalf("active unit = %q, want slice-2", got.ActiveUnitID)
	}
	if len(got.Units) != 1 || got.Units[0].Status != RuntimeUnitObserving {
		t.Fatalf("unit view = %+v, want observing", got.Units)
	}
}

func TestEventsFromWriteWorkflowRunCompleteProjectsProof(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-complete",
		Status:        types.WriteWorkflowRunComplete,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Completion: &types.WriteWorkflowCompletion{
				Verdict:    types.WriteWorkflowCompletionVerified,
				ReasonCode: "tests_passed",
			},
		}},
		Completion: &types.WriteWorkflowCompletion{
			Verdict:    types.WriteWorkflowCompletionVerified,
			ReasonCode: "tests_passed",
		},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.Status != LoopRunStatusComplete {
		t.Fatalf("status = %s, want complete", got.Status)
	}
	if got.Proof.State != ProofCoverageCovered {
		t.Fatalf("proof = %+v, want covered", got.Proof)
	}
	if len(got.Units) != 1 || got.Units[0].Status != RuntimeUnitComplete {
		t.Fatalf("unit view = %+v, want complete", got.Units)
	}
}

func TestEventsFromWriteWorkflowRunUnverifiedCompletionDoesNotRepair(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-unverified",
		Status:        types.WriteWorkflowRunComplete,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Completion: &types.WriteWorkflowCompletion{
				Verdict:    types.WriteWorkflowCompletionUnverified,
				ReasonCode: "runner_missing",
			},
		}},
		Completion: &types.WriteWorkflowCompletion{
			Verdict:    types.WriteWorkflowCompletionUnverified,
			ReasonCode: "runner_missing",
		},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.Proof.State != ProofCoverageUnavailable {
		t.Fatalf("proof = %+v, want unavailable", got.Proof)
	}
	if got.Proof.RecommendedAction == LoopActionRepair {
		t.Fatalf("unverified workflow must not force repair: %+v", got.Proof)
	}
}

func TestEventsFromWriteWorkflowRunBlockedProjectsBlocked(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-blocked",
		Status:        types.WriteWorkflowRunBlocked,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchBlocked,
		}},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.Status != LoopRunStatusBlocked {
		t.Fatalf("status = %s, want blocked", got.Status)
	}
	if len(got.Units) != 1 || got.Units[0].Status != RuntimeUnitBlocked {
		t.Fatalf("unit view = %+v, want blocked", got.Units)
	}
}
