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
	if view.Proof.State != loopkernel.ProofCoverageFailed || view.Proof.RecommendedAction != loopkernel.LoopActionRepair {
		t.Fatalf("proof authority not surfaced from failed verify: %+v", view.Proof)
	}
}

func TestDeriveWorkflowExecutionViewUnverifiedVerifySurfacesUnavailableProof(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchComplete, plan.ID)
	run.Batches[0].Attempts = append(run.Batches[0].Attempts, types.WriteWorkflowAttempt{
		Kind:       "verify",
		Status:     "unverified",
		ReasonCode: "runner_missing",
		PlanID:     plan.ID,
	})

	view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
	if view.Proof.State != loopkernel.ProofCoverageUnavailable {
		t.Fatalf("proof = %+v, want unavailable", view.Proof)
	}
	if view.Proof.RecommendedAction == loopkernel.LoopActionRepair {
		t.Fatalf("unavailable proof should not request repair: %+v", view.Proof)
	}
}

func TestDeriveWorkflowExecutionViewWithReportDowngradesPassedAttemptWhenProofWeak(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	plan.TargetPaths = []string{"pkg/owner.py"}
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchComplete, plan.ID)
	run.Batches[0].Attempts = append(run.Batches[0].Attempts, types.WriteWorkflowAttempt{
		Kind:       "verify",
		Status:     "passed",
		ReasonCode: "tests_passed",
		PlanID:     plan.ID,
	})
	report := &types.ChangeReport{
		PlanID:             plan.ID,
		Channel:            types.ChangeReportChannelPostApplyVerify,
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
		VerificationConfidence: []types.VerificationConfidenceRecord{{
			Source:       "proof_auditor",
			Category:     "probe_contract_refs",
			Status:       "missing",
			ReasonCode:   "verification_probe_missing_contract_ref",
			ContractRefs: []string{"contract:render-format"},
		}},
	}

	view := DeriveWorkflowExecutionViewWithReport(types.ModeApply, run, plan, report)
	if view.Proof.State != loopkernel.ProofCoverageWeak {
		t.Fatalf("proof = %+v, want weak", view.Proof)
	}
	if view.Proof.RecommendedAction != loopkernel.LoopActionAddProof || !view.Proof.RequiresProof {
		t.Fatalf("weak proof should request proof follow-up: %+v", view.Proof)
	}
	if view.Proof.LedgerState != types.VerificationProofLedgerLowConfidence ||
		view.Proof.ProfileStatus != types.VerificationProofWeak {
		t.Fatalf("ledger/profile state not surfaced: %+v", view.Proof)
	}
}

func TestDeriveWorkflowExecutionViewWithReportIgnoresPlannerProbeForTerminalProof(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchComplete, plan.ID)
	run.Batches[0].Attempts = append(run.Batches[0].Attempts, types.WriteWorkflowAttempt{
		Kind:       "verify",
		Status:     "passed",
		ReasonCode: "tests_passed",
		PlanID:     plan.ID,
	})
	report := &types.ChangeReport{
		PlanID:             plan.ID,
		Channel:            types.ChangeReportChannelPlannerProbe,
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
		VerificationConfidence: []types.VerificationConfidenceRecord{{
			Source:       "planner_probe",
			Category:     "probe_contract_refs",
			Status:       "missing",
			ReasonCode:   "planner_probe_missing_contract_ref",
			ContractRefs: []string{"contract:preview"},
		}},
	}

	view := DeriveWorkflowExecutionViewWithReport(types.ModeApply, run, plan, report)
	if view.Proof.State != loopkernel.ProofCoverageCovered {
		t.Fatalf("planner probe report must not downgrade terminal proof: %+v", view.Proof)
	}
}

func TestDeriveWorkflowExecutionViewWithReportPreservesUnavailableVerifier(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchComplete, plan.ID)
	run.Batches[0].Attempts = append(run.Batches[0].Attempts, types.WriteWorkflowAttempt{
		Kind:       "verify",
		Status:     "unverified",
		ReasonCode: "runner_missing",
		PlanID:     plan.ID,
	})
	report := &types.ChangeReport{
		PlanID:             plan.ID,
		Channel:            types.ChangeReportChannelPostApplyVerify,
		Passed:             false,
		VerificationStatus: types.VerificationStatusUnavailable,
		FailureKind:        types.FailureKindRunnerMissing,
		VerificationConfidence: []types.VerificationConfidenceRecord{{
			Source:       "proof_auditor",
			Category:     "probe_contract_refs",
			Status:       "missing",
			ReasonCode:   "verification_probe_missing_contract_ref",
			ContractRefs: []string{"contract:render-format"},
		}},
	}

	view := DeriveWorkflowExecutionViewWithReport(types.ModeApply, run, plan, report)
	if view.Proof.State != loopkernel.ProofCoverageUnavailable {
		t.Fatalf("unavailable verifier should remain unavailable proof: %+v", view.Proof)
	}
	if view.Proof.RecommendedAction == loopkernel.LoopActionRepair || !view.Proof.AllowsUnverified {
		t.Fatalf("unavailable proof should not force repair: %+v", view.Proof)
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

func TestDeriveWorkflowExecutionViewNoLocalizationSignalIsVisibleButNotGateEligible(t *testing.T) {
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchReadyToPlan, "")

	view := DeriveWorkflowExecutionView(types.ModeApply, run, nil)
	if view.Localization.State != loopkernel.LocalizationAuthorityMissing {
		t.Fatalf("localization = %+v, want missing", view.Localization)
	}
	if !view.Localization.RequiresMoreContext || view.Localization.ReasonCode != "localization_missing" {
		t.Fatalf("missing localization should be visible as soft context need: %+v", view.Localization)
	}
	if view.LocalizationGateEligible {
		t.Fatalf("no-signal localization must not become a hard gate: %+v", view)
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
