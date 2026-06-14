package types

import "testing"

func TestDeriveWriteWorkflowNextActionViewReadyToPlanKeepsAutoPilotRunning(t *testing.T) {
	view := DeriveWriteWorkflowNextActionView(WriteWorkflowRun{
		RunID:         "wf-ready",
		Status:        WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: WriteWorkflowBatchReadyToPlan,
		}},
	})
	if view.State != WriteWorkflowNextRunning {
		t.Fatalf("ready_to_plan next state = %q, want running", view.State)
	}
	if view.RequiresUser {
		t.Fatalf("ready_to_plan must not require user action: %+v", view)
	}
	if view.PrimaryAction != WriteWorkflowNextActionWait {
		t.Fatalf("ready_to_plan primary action = %q, want wait", view.PrimaryAction)
	}
}

func TestDeriveWriteWorkflowNextActionViewPendingApprovalRequiresBatchDecision(t *testing.T) {
	view := DeriveWriteWorkflowNextActionView(WriteWorkflowRun{
		RunID:         "wf-approval",
		Status:        WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: WriteWorkflowBatchPendingApproval,
			PlanID: "plan-1",
		}},
	})
	if view.State != WriteWorkflowNextNeedsApproval {
		t.Fatalf("pending_approval next state = %q, want needs_approval", view.State)
	}
	if !view.RequiresUser {
		t.Fatalf("pending_approval must require user action: %+v", view)
	}
	if view.PrimaryAction != WriteWorkflowNextActionApproveBatch {
		t.Fatalf("pending_approval primary action = %q, want approve_batch", view.PrimaryAction)
	}
	if view.PlanID != "plan-1" {
		t.Fatalf("PlanID = %q, want plan-1", view.PlanID)
	}
}

func TestDeriveWriteWorkflowNextActionViewCompleteDoesNotBlockCompletion(t *testing.T) {
	view := DeriveWriteWorkflowNextActionView(WriteWorkflowRun{
		RunID:         "wf-complete",
		Status:        WriteWorkflowRunComplete,
		ActiveBatchID: "batch-1",
		Batches: []WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: WriteWorkflowBatchComplete,
			PlanID: "plan-1",
		}},
	})
	if view.State != WriteWorkflowNextComplete {
		t.Fatalf("complete next state = %q, want complete", view.State)
	}
	if view.RequiresUser {
		t.Fatalf("complete workflow should not require user action to be complete: %+v", view)
	}
	if view.PrimaryAction != WriteWorkflowNextActionMerge {
		t.Fatalf("complete primary action = %q, want merge", view.PrimaryAction)
	}
}
