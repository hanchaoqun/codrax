package writeflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDeriveWorkflowExecutionViewPlannedAutoApprovalIsApplyReady(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchPlanned, plan.ID)

	view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
	if view.State != WorkflowExecutionApplyReady {
		t.Fatalf("state = %q, want %q: %+v", view.State, WorkflowExecutionApplyReady, view)
	}
	if !view.CanApply || !view.CanAutoApply || view.RequiresUser {
		t.Fatalf("apply-ready flags wrong: %+v", view)
	}
	if view.Approval.State != ApprovalExecutionAutoAllowed {
		t.Fatalf("approval state not surfaced: %+v", view.Approval)
	}
}

func TestDeriveWorkflowExecutionViewPendingManualApprovalRequiresUser(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionManual, "required")
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchPendingApproval, plan.ID)

	view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
	if view.State != WorkflowExecutionPendingApproval {
		t.Fatalf("state = %q, want %q: %+v", view.State, WorkflowExecutionPendingApproval, view)
	}
	if !view.RequiresUser || view.CanApply || view.CanAutoApply {
		t.Fatalf("pending approval flags wrong: %+v", view)
	}
}

func TestDeriveWorkflowExecutionViewPendingBatchWithAutoApprovalIsApplyReady(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchPendingApproval, plan.ID)

	view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
	if view.State != WorkflowExecutionApplyReady {
		t.Fatalf("state = %q, want %q: %+v", view.State, WorkflowExecutionApplyReady, view)
	}
	if view.RequiresUser || !view.CanAutoApply {
		t.Fatalf("auto executable pending batch should not require user: %+v", view)
	}
}

func TestDeriveWorkflowExecutionViewStaleApprovalIsInvalid(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchPlanned, plan.ID)
	plan.Summary = "changed after approval"

	view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
	if view.State != WorkflowExecutionApprovalInvalid {
		t.Fatalf("state = %q, want %q: %+v", view.State, WorkflowExecutionApprovalInvalid, view)
	}
	if !view.RequiresUser || view.CanApply {
		t.Fatalf("stale approval flags wrong: %+v", view)
	}
}

func TestDeriveWorkflowExecutionViewVerifyingRequiresObservation(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchVerifying, plan.ID)
	run.Batches[0].Attempts = append(run.Batches[0].Attempts, types.WriteWorkflowAttempt{
		Kind:   "apply",
		Status: "applied",
		PlanID: plan.ID,
	})

	view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
	if view.State != WorkflowExecutionObserveRequired {
		t.Fatalf("state = %q, want %q: %+v", view.State, WorkflowExecutionObserveRequired, view)
	}
	if !view.MustObserve || view.CanApply || view.CanPlan {
		t.Fatalf("observe-required flags wrong: %+v", view)
	}
}

func TestDeriveWorkflowExecutionViewFailedVerifyNeedsReplan(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchReadyToPlan, plan.ID)
	run.Batches[0].Attempts = append(run.Batches[0].Attempts, types.WriteWorkflowAttempt{
		Kind:              "verify",
		Status:            "failed",
		ReasonCode:        "tests_failed",
		FailureReasonCode: "assertion_failed",
		PlanID:            plan.ID,
	})

	view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
	if view.State != WorkflowExecutionNeedsReplan {
		t.Fatalf("state = %q, want %q: %+v", view.State, WorkflowExecutionNeedsReplan, view)
	}
	if !view.MustReplan || !view.CanPlan {
		t.Fatalf("needs-replan flags wrong: %+v", view)
	}
	if view.LatestVerifyStatus != "failed" || view.LatestVerifyFailureCode != "assertion_failed" {
		t.Fatalf("latest verify not surfaced: %+v", view)
	}
}

func TestDeriveWorkflowExecutionViewModePlanDoesNotExposeApplyReady(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchPlanned, plan.ID)
	run.Status = types.WriteWorkflowRunComplete

	view := DeriveWorkflowExecutionView(types.ModePlan, run, plan)
	if view.State != WorkflowExecutionPlanReady {
		t.Fatalf("state = %q, want %q: %+v", view.State, WorkflowExecutionPlanReady, view)
	}
	if view.CanApply || view.CanAutoApply || !view.RequiresUser {
		t.Fatalf("plan-mode flags wrong: %+v", view)
	}
}

func TestDeriveWorkflowExecutionViewSurfacesActiveSlice(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchVerifying, plan.ID)
	run.Batches[0].ActiveSliceID = "slice-2"
	run.Batches[0].Slices = []types.WriteWorkflowSlice{
		{ID: "slice-1", Status: types.ChangePlanSliceVerified},
		{ID: "slice-2", Status: types.ChangePlanSliceObserving},
	}

	view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
	if view.ActiveSliceID != "slice-2" || view.ActiveSliceStatus != types.ChangePlanSliceObserving {
		t.Fatalf("active slice not surfaced: %+v", view)
	}
}

func TestDeriveWorkflowExecutionViewProjectsLocalizationAuthorityFromExpectedPaths(t *testing.T) {
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchReadyToPlan, "")
	run.Batches[0].ExpectedPaths = []string{"pkg/maybe.py"}

	view := DeriveWorkflowExecutionView(types.ModeApply, run, nil)
	if view.Localization.State != loopkernel.LocalizationAuthorityObservedOnly {
		t.Fatalf("localization = %+v, want observed_only", view.Localization)
	}
	if !view.Localization.RequiresMoreContext || view.Localization.ReasonCode != "localization_observed_without_owner" {
		t.Fatalf("localization should require owner context: %+v", view.Localization)
	}
	if view.LocalizationGateEligible {
		t.Fatalf("batch expected paths alone should not be a hard localization gate: %+v", view)
	}
	if len(view.Localization.SourcePaths) != 1 || view.Localization.SourcePaths[0] != "pkg/maybe.py" {
		t.Fatalf("source paths not projected from expected paths: %+v", view.Localization.SourcePaths)
	}
}

func TestDeriveWorkflowExecutionViewSurfacesExploreAttempts(t *testing.T) {
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchReadyToPlan, "")
	run.Batches[0].Attempts = []types.WriteWorkflowAttempt{{
		Kind:       "explore",
		Status:     "complete",
		ReasonCode: "exploration_complete",
	}}

	view := DeriveWorkflowExecutionView(types.ModeApply, run, nil)
	if view.ExploreAttempts != 1 || view.LatestExploreStatus != "complete" || view.LatestExploreReason != "exploration_complete" {
		t.Fatalf("explore attempts not surfaced: %+v", view)
	}
}

func workflowExecutionRunForTest(status types.WriteWorkflowBatchStatus, planID string) types.WriteWorkflowRun {
	return types.WriteWorkflowRun{
		RunID:         "wf-execution",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: status,
			PlanID: planID,
		}},
	}
}
