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

func TestDeriveWriteWorkflowNextActionViewPlannedInProgressKeepsAutoPilotRunning(t *testing.T) {
	view := DeriveWriteWorkflowNextActionView(WriteWorkflowRun{
		RunID:         "wf-planned",
		Status:        WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: WriteWorkflowBatchPlanned,
			PlanID: "plan-1",
		}},
		ProgressLedger: []WriteWorkflowProgress{{
			BatchID:    "batch-1",
			ReasonCode: "batch_planned",
		}},
	})
	if view.State != WriteWorkflowNextRunning {
		t.Fatalf("in-progress planned batch next state = %q, want running", view.State)
	}
	if view.RequiresUser {
		t.Fatalf("in-progress planned batch must not require user action: %+v", view)
	}
	if view.PrimaryAction != WriteWorkflowNextActionWait {
		t.Fatalf("in-progress planned primary action = %q, want wait", view.PrimaryAction)
	}
}

func TestDeriveWriteWorkflowNextActionViewPlanModeCompleteRequiresReview(t *testing.T) {
	view := DeriveWriteWorkflowNextActionView(WriteWorkflowRun{
		RunID:         "wf-plan-only",
		Status:        WriteWorkflowRunComplete,
		ActiveBatchID: "batch-1",
		Batches: []WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: WriteWorkflowBatchPlanned,
			PlanID: "plan-1",
		}},
		ProgressLedger: []WriteWorkflowProgress{{
			BatchID:    "batch-1",
			ReasonCode: "plan_mode_complete",
		}},
	})
	if view.State != WriteWorkflowNextPlanReady {
		t.Fatalf("plan-mode complete next state = %q, want plan_ready", view.State)
	}
	if !view.RequiresUser {
		t.Fatalf("plan-mode complete should require an explicit user decision before apply: %+v", view)
	}
	if view.PrimaryAction != WriteWorkflowNextActionReviewPlan {
		t.Fatalf("plan-mode complete primary action = %q, want review_plan", view.PrimaryAction)
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
			Completion: &WriteWorkflowCompletion{
				Verdict:    WriteWorkflowCompletionUnverified,
				ReasonCode: "runner_missing",
			},
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
	if view.CompletionVerdict != WriteWorkflowCompletionUnverified || view.CompletionReasonCode != "runner_missing" {
		t.Fatalf("completion verdict not surfaced: %+v", view)
	}
}
