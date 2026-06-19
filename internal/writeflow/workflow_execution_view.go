package writeflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

// WorkflowExecutionState is the scheduler-facing, typed interpretation of the
// active write workflow. It is derived from durable run state, batch attempts,
// the active plan, and approval authority. It is not inferred from progress
// prose or model output.
type WorkflowExecutionState string

const (
	WorkflowExecutionNoActive         WorkflowExecutionState = "no_active_batch"
	WorkflowExecutionNeedsExploration WorkflowExecutionState = "needs_exploration"
	WorkflowExecutionReadyToPlan      WorkflowExecutionState = "ready_to_plan"
	WorkflowExecutionPlanReady        WorkflowExecutionState = "plan_ready"
	WorkflowExecutionApplyReady       WorkflowExecutionState = "apply_ready"
	WorkflowExecutionPendingApproval  WorkflowExecutionState = "pending_approval"
	WorkflowExecutionApprovalInvalid  WorkflowExecutionState = "approval_invalid"
	WorkflowExecutionApplying         WorkflowExecutionState = "applying"
	WorkflowExecutionObserveRequired  WorkflowExecutionState = "observe_required"
	WorkflowExecutionNeedsReplan      WorkflowExecutionState = "needs_replan"
	WorkflowExecutionComplete         WorkflowExecutionState = "complete"
	WorkflowExecutionBlocked          WorkflowExecutionState = "blocked"
)

// WorkflowExecutionView is the read-only state-kernel input for future
// controller transition validation and effect chaining. Consumers may render it
// for UX/eval, but hard routing should use the booleans/enums here rather than
// plan lifecycle strings alone.
type WorkflowExecutionView struct {
	Mode types.PipelineMode `json:"mode,omitempty"`

	State WorkflowExecutionState `json:"state"`

	RunID       string                         `json:"run_id,omitempty"`
	RunStatus   types.WriteWorkflowRunStatus   `json:"run_status,omitempty"`
	BatchID     string                         `json:"batch_id,omitempty"`
	BatchStatus types.WriteWorkflowBatchStatus `json:"batch_status,omitempty"`
	PlanID      string                         `json:"plan_id,omitempty"`

	ActiveSliceID     string                      `json:"active_slice_id,omitempty"`
	ActiveSliceStatus types.ChangePlanSliceStatus `json:"active_slice_status,omitempty"`

	BatchAttempt BatchAttemptState     `json:"batch_attempt,omitempty"`
	Approval     ApprovalExecutionView `json:"approval,omitempty"`
	Budget       types.WriteWorkflowBudget

	Localization             loopkernel.LocalizationAuthorityView `json:"localization,omitempty"`
	LocalizationGateEligible bool                                 `json:"localization_gate_eligible,omitempty"`
	ExploreAttempts          int                                  `json:"explore_attempts,omitempty"`
	LatestExploreStatus      string                               `json:"latest_explore_status,omitempty"`
	LatestExploreReason      string                               `json:"latest_explore_reason,omitempty"`
	LatestVerifyStatus       string                               `json:"latest_verify_status,omitempty"`
	LatestVerifyReasonCode   string                               `json:"latest_verify_reason_code,omitempty"`
	LatestVerifyFailureCode  string                               `json:"latest_verify_failure_code,omitempty"`
	Observation              ObservationAuthorityView             `json:"observation,omitempty"`

	RequiresUser bool `json:"requires_user,omitempty"`
	CanExplore   bool `json:"can_explore,omitempty"`
	CanPlan      bool `json:"can_plan,omitempty"`
	CanApply     bool `json:"can_apply,omitempty"`
	CanAutoApply bool `json:"can_auto_apply,omitempty"`
	MustObserve  bool `json:"must_observe,omitempty"`
	MustReplan   bool `json:"must_replan,omitempty"`
	Terminal     bool `json:"terminal,omitempty"`
}

// DeriveWorkflowExecutionView assembles the active workflow state without
// executing any transition. The plan parameter should be the current active
// ChangePlan when available; it is ignored when its ID conflicts with the
// active batch's PlanID.
func DeriveWorkflowExecutionView(mode types.PipelineMode, run types.WriteWorkflowRun, plan *types.ChangePlan) WorkflowExecutionView {
	run = types.NormalizeWriteWorkflowRun(run)
	view := WorkflowExecutionView{
		Mode:      mode.Normalize(),
		State:     WorkflowExecutionNoActive,
		RunID:     strings.TrimSpace(run.RunID),
		RunStatus: run.Status,
		Budget:    run.Budget,
	}
	batch, ok := workflowExecutionActiveBatch(run)
	if !ok {
		return view
	}
	view.BatchID = strings.TrimSpace(batch.ID)
	view.BatchStatus = batch.Status
	view.PlanID = strings.TrimSpace(batch.PlanID)
	view.ActiveSliceID, view.ActiveSliceStatus = workflowExecutionActiveSlice(batch)
	view.BatchAttempt = DeriveBatchAttemptState(batch)
	view.ExploreAttempts, view.LatestExploreStatus, view.LatestExploreReason = workflowExecutionExploreAttempts(batch)
	if review := loopkernel.LocalizationReviewFromWriteWorkflowRun(run, view.BatchID); review != nil {
		view.Localization = loopkernel.DeriveLocalizationAuthority(review)
		view.LocalizationGateEligible = workflowExecutionLocalizationGateEligible(*review)
	}
	if latestVerify := latestAttemptOfKind(batch, "verify"); latestVerify != nil {
		view.LatestVerifyStatus = strings.TrimSpace(latestVerify.Status)
		view.LatestVerifyReasonCode = strings.TrimSpace(latestVerify.ReasonCode)
		view.LatestVerifyFailureCode = strings.TrimSpace(latestVerify.FailureReasonCode)
		view.Observation = DeriveObservationAuthorityFromAttempt(latestVerify)
	}
	activePlan := workflowExecutionActivePlan(batch, plan)
	if activePlan != nil {
		view.Approval = DeriveApprovalExecutionView(activePlan)
	}
	if view.BatchAttempt.Phase == BatchPhaseNeedsReplan {
		view.State = WorkflowExecutionNeedsReplan
		view.MustReplan = true
		view.CanPlan = true
		return view
	}
	switch batch.Status {
	case types.WriteWorkflowBatchNeedsExploration:
		view.State = WorkflowExecutionNeedsExploration
		view.CanExplore = true
	case types.WriteWorkflowBatchReadyToPlan:
		view.State = WorkflowExecutionReadyToPlan
		view.CanPlan = true
	case types.WriteWorkflowBatchPlanned:
		if view.Mode == types.ModePlan {
			view.State = WorkflowExecutionPlanReady
			view.RequiresUser = true
			return view
		}
		applyApprovalExecutionToView(&view, activePlan)
	case types.WriteWorkflowBatchPendingApproval:
		applyApprovalExecutionToView(&view, activePlan)
		if view.State == WorkflowExecutionPlanReady {
			view.State = WorkflowExecutionPendingApproval
			view.RequiresUser = true
		}
	case types.WriteWorkflowBatchApplying:
		view.State = WorkflowExecutionApplying
	case types.WriteWorkflowBatchVerifying:
		view.State = WorkflowExecutionObserveRequired
		view.MustObserve = true
	case types.WriteWorkflowBatchComplete:
		view.State = WorkflowExecutionComplete
		view.Terminal = true
	case types.WriteWorkflowBatchBlocked:
		view.State = WorkflowExecutionBlocked
		view.Terminal = true
	default:
		view.State = WorkflowExecutionReadyToPlan
		view.CanPlan = true
	}
	return view
}

func applyApprovalExecutionToView(view *WorkflowExecutionView, activePlan *types.ChangePlan) {
	if view == nil {
		return
	}
	if activePlan == nil || activePlan.Approval == nil {
		view.State = WorkflowExecutionPlanReady
		return
	}
	switch view.Approval.State {
	case ApprovalExecutionAutoAllowed:
		view.State = WorkflowExecutionApplyReady
		view.CanApply = true
		view.CanAutoApply = true
	case ApprovalExecutionManualApproved:
		view.State = WorkflowExecutionApplyReady
		view.CanApply = true
	case ApprovalExecutionManualRequired:
		view.State = WorkflowExecutionPendingApproval
		view.RequiresUser = true
	case ApprovalExecutionDenied:
		view.State = WorkflowExecutionBlocked
		view.Terminal = true
	case ApprovalExecutionFingerprintMismatch, ApprovalExecutionIntegrityFailed, ApprovalExecutionUnknownAction:
		view.State = WorkflowExecutionApprovalInvalid
		view.RequiresUser = true
	default:
		view.State = WorkflowExecutionPlanReady
	}
}

func workflowExecutionActivePlan(batch types.WriteWorkflowBatch, plan *types.ChangePlan) *types.ChangePlan {
	if plan == nil {
		return nil
	}
	batchPlanID := strings.TrimSpace(batch.PlanID)
	planID := strings.TrimSpace(plan.ID)
	if batchPlanID != "" && planID != "" && batchPlanID != planID {
		return nil
	}
	return plan
}

func workflowExecutionActiveBatch(run types.WriteWorkflowRun) (types.WriteWorkflowBatch, bool) {
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" && len(run.Batches) > 0 {
		return run.Batches[0], true
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) == activeID {
			return batch, true
		}
	}
	return types.WriteWorkflowBatch{}, false
}

func workflowExecutionActiveSlice(batch types.WriteWorkflowBatch) (string, types.ChangePlanSliceStatus) {
	activeID := strings.TrimSpace(batch.ActiveSliceID)
	if activeID != "" {
		for _, slice := range batch.Slices {
			if strings.TrimSpace(slice.ID) == activeID {
				return strings.TrimSpace(slice.ID), slice.Status
			}
		}
	}
	for _, slice := range batch.Slices {
		if !workflowExecutionSliceTerminal(slice.Status) {
			return strings.TrimSpace(slice.ID), slice.Status
		}
	}
	return "", ""
}

func workflowExecutionExploreAttempts(batch types.WriteWorkflowBatch) (int, string, string) {
	count := 0
	latestStatus := ""
	latestReason := ""
	for _, attempt := range batch.Attempts {
		if strings.TrimSpace(attempt.Kind) != "explore" {
			continue
		}
		count++
		latestStatus = strings.TrimSpace(attempt.Status)
		latestReason = strings.TrimSpace(attempt.ReasonCode)
	}
	return count, latestStatus, latestReason
}

func workflowExecutionLocalizationGateEligible(review types.SourceLocalizationReview) bool {
	review = types.NormalizeSourceLocalizationReview(review)
	for _, anchor := range review.Anchors {
		if types.SourcePathRoleIsAuxiliary(anchor.Role) {
			continue
		}
		switch anchor.Kind {
		case types.SourceLocalizationAnchorReadFile,
			types.SourceLocalizationAnchorGroundedEvidence,
			types.SourceLocalizationAnchorRecoveredEvidence,
			types.SourceLocalizationAnchorEvidence:
			return true
		}
	}
	return false
}

func workflowExecutionSliceTerminal(status types.ChangePlanSliceStatus) bool {
	switch status {
	case types.ChangePlanSliceVerified, types.ChangePlanSliceUnverified, types.ChangePlanSliceFailed, types.ChangePlanSliceBlocked:
		return true
	default:
		return false
	}
}
