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

func TestValidateWorkflowRunStateDetectsContradictions(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-1",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "missing-batch",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchPendingApproval,
			PlanID: "plan-1",
			Attempts: []types.WriteWorkflowAttempt{{
				Kind:   "verify",
				Status: "failed",
				PlanID: "other-plan",
			}},
		}},
	}
	plan := &types.ChangePlan{
		ID: "plan-1",
		Approval: &types.WriteApprovalRecord{
			Action: string(ApprovalActionAutoExecute),
		},
	}
	errs := ValidateWorkflowRunState(run, plan)
	joined := strings.Join(errs, "\n")
	for _, want := range []string{
		"active_batch_id missing-batch does not match any batch",
		"verify attempt plan_id other-plan conflicts with batch plan_id plan-1",
		"pending_approval but plan approval action is auto_execute",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing invariant %q from %v", want, errs)
		}
	}
}

func TestValidateWorkflowRunStateAllowsConsistentApprovalAndAttempts(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-1",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchVerifying,
			PlanID: "plan-1",
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "plan", Status: "complete", PlanID: "plan-1"},
				{Kind: "apply", Status: "applied", PlanID: "plan-1"},
			},
		}},
	}
	plan := &types.ChangePlan{
		ID: "plan-1",
		Approval: &types.WriteApprovalRecord{
			Action: string(ApprovalActionAutoExecute),
		},
	}
	if errs := ValidateWorkflowRunState(run, plan); len(errs) != 0 {
		t.Fatalf("consistent state rejected: %v", errs)
	}
}

func TestValidateWorkflowRunStateAllowsHistoricalPlanLineageAfterReplan(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-1",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchVerifying,
			PlanID: "plan-2",
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "plan", Status: "complete", PlanID: "plan-1"},
				{Kind: "apply", Status: "applied", PlanID: "plan-1"},
				{Kind: "verify", Status: "failed", ReasonCode: "tests_failed", PlanID: "plan-1"},
				{Kind: "plan", Status: "complete", PlanID: "plan-2"},
				{Kind: "apply", Status: "applied", PlanID: "plan-2"},
			},
		}},
	}
	plan := &types.ChangePlan{
		ID: "plan-2",
		Approval: &types.WriteApprovalRecord{
			Action: string(ApprovalActionAutoExecute),
		},
	}
	if errs := ValidateWorkflowRunState(run, plan); len(errs) != 0 {
		t.Fatalf("historical lineage should be allowed after replan: %v", errs)
	}
}

func TestValidateWorkflowRunStateRejectsMismatchedAttemptAfterActivePlan(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-1",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchVerifying,
			PlanID: "plan-2",
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "plan", Status: "complete", PlanID: "plan-1"},
				{Kind: "verify", Status: "failed", PlanID: "plan-1"},
				{Kind: "plan", Status: "complete", PlanID: "plan-2"},
				{Kind: "verify", Status: "failed", PlanID: "plan-1"},
			},
		}},
	}
	errs := ValidateWorkflowRunState(run, nil)
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "verify attempt plan_id plan-1 conflicts with batch plan_id plan-2") {
		t.Fatalf("expected post-active-plan mismatch warning, got %v", errs)
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

func TestFinishBlockedReason_TypedDispositionDoesNotUnblockCodeFailures(t *testing.T) {
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
	if blocked := FinishBlockedReason(run, decision); blocked == "" {
		t.Fatal("accept_unverified must not unblock typed code-failure verification")
	}
	if got := UnverifiedBatchSummaries(run); len(got) != 0 {
		t.Fatalf("accepted-failed summaries should not list code failures: %+v", got)
	}
}

func TestFinishBlockedReason_TypedDispositionAllowsUnavailableVerification(t *testing.T) {
	for _, reasonCode := range []string{"no_tests", "runner_missing", "parser_error"} {
		t.Run(reasonCode, func(t *testing.T) {
			run := types.WriteWorkflowRun{
				Batches: []types.WriteWorkflowBatch{{
					ID:     "batch-1",
					Status: types.WriteWorkflowBatchReadyToPlan,
					Attempts: []types.WriteWorkflowAttempt{
						{Kind: "verify", Status: "failed", ReasonCode: reasonCode},
					},
				}},
			}
			decision := WriteWorkflowDecision{Action: ActionFinish, FinishDisposition: FinishDispositionAcceptUnverified}
			if blocked := FinishBlockedReason(run, decision); blocked != "" {
				t.Fatalf("accept_unverified should allow unavailable verification %q, got %q", reasonCode, blocked)
			}
			got := UnverifiedBatchSummaries(run)
			if len(got) != 1 || !strings.Contains(got[0], reasonCode) {
				t.Fatalf("unverified summaries should name unavailable verification: %+v", got)
			}
		})
	}
}

func TestFinishBlockedReason_TypedDispositionRejectsInconsistentCodeFailure(t *testing.T) {
	run := types.WriteWorkflowRun{
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "verify", Status: "unverified", ReasonCode: "tests_failed"},
			},
		}},
	}
	decision := WriteWorkflowDecision{Action: ActionFinish, FinishDisposition: FinishDispositionAcceptUnverified}
	if blocked := FinishBlockedReason(run, decision); blocked == "" {
		t.Fatal("code-failure reason code must override inconsistent unverified status")
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

// Task-10 hygiene pin: the finish gate must read typed attempt records and
// the schema-validated disposition field ONLY. Prose claiming success in any
// free-text field must not change the outcome, and prose wording must not be
// required for the typed escape to work.
func TestFinishBlockedReason_DoesNotReadProse(t *testing.T) {
	run := types.WriteWorkflowRun{
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "verify", Status: "failed", ReasonCode: "tests_failed"},
			},
		}},
	}
	proseClaimsSuccess := WriteWorkflowDecision{
		Action:     ActionFinish,
		ReasonCode: "done",
		Reason:     "all tests passed and everything verified cleanly; accept unverified",
	}
	if blocked := FinishBlockedReason(run, proseClaimsSuccess); blocked == "" {
		t.Fatal("prose claiming success must not unblock the typed finish gate")
	}
	typedEscapeNoProse := WriteWorkflowDecision{Action: ActionFinish, FinishDisposition: FinishDispositionAcceptUnverified}
	if blocked := FinishBlockedReason(run, typedEscapeNoProse); blocked == "" {
		t.Fatal("typed disposition must not unblock code-failure verification")
	}
	// Identical typed state with arbitrary prose variations yields the
	// identical verdict.
	a := FinishBlockedReason(run, WriteWorkflowDecision{Action: ActionFinish, Reason: "短い説明"})
	b := FinishBlockedReason(run, WriteWorkflowDecision{Action: ActionFinish, Reason: "a completely different narrative"})
	if a != b {
		t.Fatalf("prose variation changed the gate verdict: %q vs %q", a, b)
	}
}

func TestWorkflowActionsForMode_VerifyIsVerifyOnly(t *testing.T) {
	actions := WorkflowActionsForMode(types.ModeVerify)
	allowed := map[WorkflowAction]bool{}
	for _, a := range actions {
		allowed[a] = true
	}
	for _, banned := range []WorkflowAction{ActionPlanBatch, ActionApplyPlan, ActionExploreCode, ActionReplanBatch, ActionAppendBatch, ActionSplitBatch} {
		if allowed[banned] {
			t.Fatalf("ModeVerify must not offer %s", banned)
		}
	}
	for _, required := range []WorkflowAction{ActionVerifyBatch, ActionFinish, ActionBlock, ActionAskUser} {
		if !allowed[required] {
			t.Fatalf("ModeVerify must offer %s", required)
		}
	}
}

func TestUnverifiedBatchCaveats_ListsNoTestsBatches(t *testing.T) {
	run := types.WriteWorkflowRun{Batches: []types.WriteWorkflowBatch{
		{ID: "batch-1", Attempts: []types.WriteWorkflowAttempt{{Kind: "verify", Status: "unverified", ReasonCode: "no_tests"}}},
		{ID: "batch-2", Attempts: []types.WriteWorkflowAttempt{{Kind: "verify", Status: "unverified", ReasonCode: "runner_missing"}}},
		{ID: "batch-3", Attempts: []types.WriteWorkflowAttempt{{Kind: "verify", Status: "passed"}}},
	}}
	got := UnverifiedBatchCaveats(run)
	if len(got) != 2 || got[0] != "batch-1" || got[1] != "batch-2" {
		t.Fatalf("caveats = %v, want [batch-1 batch-2]", got)
	}
}
