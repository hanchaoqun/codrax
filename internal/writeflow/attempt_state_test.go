package writeflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDeriveBatchAttemptState_FreshBatchKeepsStatus(t *testing.T) {
	st := DeriveBatchAttemptState(types.WriteWorkflowBatch{
		ID:     "batch-1",
		Status: types.WriteWorkflowBatchReadyToPlan,
	})
	if st.Phase != BatchPhaseReadyToPlan || st.Cause != "" {
		t.Fatalf("fresh batch should stay ready_to_plan with no cause, got %+v", st)
	}
}

func TestDeriveBatchAttemptState_FailedVerifyBecomesNeedsReplan(t *testing.T) {
	batch := types.WriteWorkflowBatch{
		ID:     "batch-1",
		Status: types.WriteWorkflowBatchReadyToPlan,
		PlanID: "plan-1",
		Attempts: []types.WriteWorkflowAttempt{
			{Kind: "plan", Status: "complete", PlanID: "plan-1"},
			{Kind: "apply", Status: "applied", ReasonCode: "apply_succeeded", PlanID: "plan-1"},
			{Kind: "verify", Status: "failed", ReasonCode: "tests_failed", PlanID: "plan-1", ReportID: "plan-1.report.json"},
		},
	}
	st := DeriveBatchAttemptState(batch)
	if st.Phase != BatchPhaseNeedsReplan {
		t.Fatalf("phase = %q, want needs_replan", st.Phase)
	}
	if st.Cause != "tests_failed" {
		t.Fatalf("cause = %q, want tests_failed", st.Cause)
	}
	if st.ReportID != "plan-1.report.json" {
		t.Fatalf("report id = %q", st.ReportID)
	}
	if st.FailedVerifyAttempts != 1 {
		t.Fatalf("failed verify attempts = %d, want 1", st.FailedVerifyAttempts)
	}
}

func TestDeriveBatchAttemptState_PassedVerifyDoesNotReplan(t *testing.T) {
	batch := types.WriteWorkflowBatch{
		ID:     "batch-1",
		Status: types.WriteWorkflowBatchReadyToPlan,
		Attempts: []types.WriteWorkflowAttempt{
			{Kind: "verify", Status: "failed", ReasonCode: "tests_failed"},
			{Kind: "verify", Status: "passed", ReasonCode: "tests_passed"},
		},
	}
	st := DeriveBatchAttemptState(batch)
	if st.Phase != BatchPhaseReadyToPlan {
		t.Fatalf("latest verify passed; phase should follow batch status, got %+v", st)
	}
	if st.FailedVerifyAttempts != 1 {
		t.Fatalf("failed verify attempts = %d, want 1", st.FailedVerifyAttempts)
	}
}

func TestFinishBlockedReason_BlocksOnLatestFailedVerify(t *testing.T) {
	run := types.WriteWorkflowRun{
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "verify", Status: "failed", ReasonCode: "tests_failed"},
			},
		}},
	}
	blocked := FinishBlockedReason(run, WriteWorkflowDecision{Action: ActionFinish})
	if blocked == "" {
		t.Fatal("finish must be blocked while latest verify attempt failed")
	}
	if !strings.Contains(blocked, "batch-1(tests_failed)") {
		t.Fatalf("blocked reason should name the batch and typed cause: %q", blocked)
	}
}

func TestFinishBlockedReason_TypedDispositionUnblocks(t *testing.T) {
	run := types.WriteWorkflowRun{
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "verify", Status: "failed", ReasonCode: "tests_failed"},
			},
		}},
	}
	decision := WriteWorkflowDecision{Action: ActionFinish, FinishDisposition: FinishDispositionAcceptUnverified}
	if blocked := FinishBlockedReason(run, decision); blocked != "" {
		t.Fatalf("accept_unverified must unblock finish, got %q", blocked)
	}
	if got := UnverifiedBatchSummaries(run); len(got) != 1 || !strings.Contains(got[0], "batch-1") {
		t.Fatalf("unverified summaries should name the batch: %+v", got)
	}
}

func TestFinishBlockedReason_CompleteAndPassedBatchesDoNotBlock(t *testing.T) {
	run := types.WriteWorkflowRun{
		Batches: []types.WriteWorkflowBatch{
			{
				ID:     "batch-1",
				Status: types.WriteWorkflowBatchComplete,
				Attempts: []types.WriteWorkflowAttempt{
					{Kind: "verify", Status: "failed", ReasonCode: "tests_failed"},
					{Kind: "verify", Status: "passed", ReasonCode: "tests_passed"},
				},
			},
			{ID: "batch-2", Status: types.WriteWorkflowBatchReadyToPlan},
		},
	}
	if blocked := FinishBlockedReason(run, WriteWorkflowDecision{Action: ActionFinish}); blocked != "" {
		t.Fatalf("no live failed verify; finish should pass, got %q", blocked)
	}
}

func TestWorkflowActionsForMode_PlanMasksApplyVerify(t *testing.T) {
	planActions := WorkflowActionsForMode(types.ModePlan)
	for _, a := range planActions {
		if a == ActionApplyPlan || a == ActionVerifyBatch {
			t.Fatalf("ModePlan action set must not contain %s", a)
		}
	}
	if len(planActions) != len(AllWorkflowActions())-2 {
		t.Fatalf("ModePlan should mask exactly two actions, got %v", planActions)
	}
	if got := WorkflowActionsForMode(types.ModeApply); len(got) != len(AllWorkflowActions()) {
		t.Fatalf("ModeApply must keep the full action set, got %v", got)
	}
	if WorkflowActionAllowedInMode(ActionApplyPlan, types.ModePlan) {
		t.Fatal("apply_plan must not be allowed in ModePlan")
	}
	if !WorkflowActionAllowedInMode(ActionApplyPlan, types.ModeApply) {
		t.Fatal("apply_plan must stay allowed in ModeApply")
	}
}

func TestWriteWorkflowDecisionSchemaForActions_RestrictsEnum(t *testing.T) {
	schema := string(WriteWorkflowDecisionSchemaForActions(WorkflowActionsForMode(types.ModePlan)))
	if strings.Contains(schema, `"apply_plan"`) || strings.Contains(schema, `"verify_batch"`) {
		t.Fatalf("ModePlan schema must not offer apply/verify actions: %s", schema)
	}
	if !strings.Contains(schema, `"plan_batch"`) || !strings.Contains(schema, `"finish"`) {
		t.Fatalf("ModePlan schema must keep planning actions: %s", schema)
	}
	full := string(WriteWorkflowDecisionSchema())
	if !strings.Contains(full, `"apply_plan"`) || !strings.Contains(full, `"finish_disposition"`) {
		t.Fatalf("full schema must keep apply_plan and finish_disposition: %s", full)
	}
}

func TestValidateWriteWorkflowDecision_FinishDisposition(t *testing.T) {
	bad := WriteWorkflowDecision{Action: ActionFinish, FinishDisposition: "whatever"}
	if errs := ValidateWriteWorkflowDecision(bad); len(errs) == 0 {
		t.Fatal("unknown finish_disposition must be rejected")
	}
	misplaced := WriteWorkflowDecision{Action: ActionPlanBatch, FinishDisposition: FinishDispositionAcceptUnverified,
		Batch: &WriteBatchPlan{ID: "b", Goal: "g"}}
	if errs := ValidateWriteWorkflowDecision(misplaced); len(errs) == 0 {
		t.Fatal("finish_disposition without action=finish must be rejected")
	}
	ok := WriteWorkflowDecision{Action: ActionFinish, FinishDisposition: FinishDispositionAcceptUnverified}
	if errs := ValidateWriteWorkflowDecision(ok); len(errs) != 0 {
		t.Fatalf("valid finish disposition rejected: %v", errs)
	}
}
