package writeflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// WorkflowTransitionValidation is the typed result of checking a controller
// decision against the assembled workflow execution state. The write controller
// enforces this before effect execution; tests lock down the state-kernel rules
// so future loop-kernel adapters can reuse the same typed contract.
type WorkflowTransitionValidation struct {
	Allowed           bool           `json:"allowed"`
	ReasonCode        string         `json:"reason_code,omitempty"`
	Reason            string         `json:"reason,omitempty"`
	RecommendedAction WorkflowAction `json:"recommended_action,omitempty"`
}

// ValidateWorkflowTransition validates one controller decision against a
// WorkflowExecutionView. It reads only typed state and schema-normalized action
// fields; natural-language reason text never drives routing.
func ValidateWorkflowTransition(view WorkflowExecutionView, decision WriteWorkflowDecision) WorkflowTransitionValidation {
	decision = NormalizeWriteWorkflowDecision(decision)
	if errs := ValidateWriteWorkflowDecision(decision); len(errs) > 0 {
		return workflowTransitionDenied("invalid_decision", strings.Join(errs, "; "), "")
	}
	if !WorkflowActionAllowedInMode(decision.Action, view.Mode) {
		return workflowTransitionDenied("action_not_allowed_in_mode",
			fmt.Sprintf("action %s is not allowed in mode %s", decision.Action, view.Mode), "")
	}
	if localization := validateLocalizationAuthorityTransition(view, decision.Action); !localization.Allowed {
		return localization
	}
	switch view.State {
	case WorkflowExecutionApplyReady:
		return validateApplyReadyTransition(decision.Action)
	case WorkflowExecutionPendingApproval:
		return validatePendingApprovalTransition(decision.Action)
	case WorkflowExecutionApprovalInvalid:
		return validateApprovalInvalidTransition(decision.Action)
	case WorkflowExecutionObserveRequired:
		return validateObserveRequiredTransition(decision.Action)
	case WorkflowExecutionNeedsReplan:
		return validateNeedsReplanTransition(view, decision)
	case WorkflowExecutionComplete:
		return validateCompleteTransition(view, decision.Action)
	case WorkflowExecutionBlocked:
		return validateBlockedTransition(decision.Action)
	default:
		return WorkflowTransitionValidation{Allowed: true}
	}
}

func validateLocalizationAuthorityTransition(view WorkflowExecutionView, action WorkflowAction) WorkflowTransitionValidation {
	if !workflowTransitionNeedsOwnerLocalization(view) {
		return WorkflowTransitionValidation{Allowed: true}
	}
	switch action {
	case ActionExploreCode, ActionAskUser, ActionBlock:
		return WorkflowTransitionValidation{Allowed: true}
	case ActionPlanBatch, ActionAppendBatch, ActionSplitBatch, ActionReplanBatch, ActionFinish:
		return workflowTransitionDenied("localization_authority_requires_exploration",
			"typed localization authority requires bounded owner localization before planning or finishing this batch", ActionExploreCode)
	default:
		return WorkflowTransitionValidation{Allowed: true}
	}
}

func workflowTransitionNeedsOwnerLocalization(view WorkflowExecutionView) bool {
	if !view.LocalizationGateEligible || !view.Localization.RequiresMoreContext || view.ExploreAttempts > 0 {
		return false
	}
	if len(view.Localization.SourcePaths) == 0 && len(view.Localization.OwnerMissingPaths) == 0 {
		return false
	}
	switch view.State {
	case WorkflowExecutionReadyToPlan, WorkflowExecutionNeedsReplan:
		return true
	default:
		return false
	}
}

func validateApplyReadyTransition(action WorkflowAction) WorkflowTransitionValidation {
	switch action {
	case ActionApplyPlan, ActionBlock:
		return WorkflowTransitionValidation{Allowed: true}
	default:
		return workflowTransitionDenied("apply_ready_requires_apply",
			"active plan is executable; apply_plan is the next deterministic action", ActionApplyPlan)
	}
}

func validatePendingApprovalTransition(action WorkflowAction) WorkflowTransitionValidation {
	switch action {
	case ActionAskUser, ActionBlock:
		return WorkflowTransitionValidation{Allowed: true}
	default:
		return workflowTransitionDenied("manual_approval_required",
			"active plan requires explicit approval before mutation", ActionAskUser)
	}
}

func validateApprovalInvalidTransition(action WorkflowAction) WorkflowTransitionValidation {
	switch action {
	case ActionAskUser, ActionReplanBatch, ActionBlock:
		return WorkflowTransitionValidation{Allowed: true}
	default:
		return workflowTransitionDenied("approval_authority_invalid",
			"approval record is missing, stale, tampered, or unsupported", ActionAskUser)
	}
}

func validateObserveRequiredTransition(action WorkflowAction) WorkflowTransitionValidation {
	switch action {
	case ActionVerifyBatch, ActionBlock:
		return WorkflowTransitionValidation{Allowed: true}
	default:
		return workflowTransitionDenied("post_apply_observe_required",
			"active applied plan requires typed observation before more planning", ActionVerifyBatch)
	}
}

func validateNeedsReplanTransition(view WorkflowExecutionView, decision WriteWorkflowDecision) WorkflowTransitionValidation {
	switch decision.Action {
	case ActionReplanBatch, ActionExploreCode, ActionFinish, ActionBlock:
		return WorkflowTransitionValidation{Allowed: true}
	case ActionAppendBatch, ActionSplitBatch:
		if workflowTransitionTargetsNewBatch(view, decision) {
			return WorkflowTransitionValidation{Allowed: true}
		}
	default:
		return workflowTransitionDenied("failed_verify_requires_replan",
			"failed post-apply verification requires a new bounded plan or exploration", ActionReplanBatch)
	}
	return workflowTransitionDenied("failed_verify_requires_replan",
		"failed post-apply verification requires a new bounded plan or exploration", ActionReplanBatch)
}

func workflowTransitionTargetsNewBatch(view WorkflowExecutionView, decision WriteWorkflowDecision) bool {
	if decision.Batch == nil {
		return false
	}
	target := strings.TrimSpace(decision.Batch.ID)
	if target == "" {
		return false
	}
	active := strings.TrimSpace(view.BatchID)
	return active == "" || target != active
}

func validateCompleteTransition(view WorkflowExecutionView, action WorkflowAction) WorkflowTransitionValidation {
	if view.RunStatus == "" || view.RunStatus == types.WriteWorkflowRunInProgress {
		switch action {
		case ActionAppendBatch, ActionSplitBatch, ActionFinish, ActionBlock:
			return WorkflowTransitionValidation{Allowed: true}
		}
	}
	switch action {
	case ActionFinish, ActionBlock:
		return WorkflowTransitionValidation{Allowed: true}
	default:
		return workflowTransitionDenied("workflow_already_complete",
			"completed workflow should finish or stay complete", ActionFinish)
	}
}

func validateBlockedTransition(action WorkflowAction) WorkflowTransitionValidation {
	switch action {
	case ActionBlock, ActionAskUser:
		return WorkflowTransitionValidation{Allowed: true}
	default:
		return workflowTransitionDenied("workflow_blocked",
			"blocked workflow cannot continue without explicit operator recovery", ActionAskUser)
	}
}

func workflowTransitionDenied(reasonCode, reason string, recommended WorkflowAction) WorkflowTransitionValidation {
	return WorkflowTransitionValidation{
		Allowed:           false,
		ReasonCode:        strings.TrimSpace(reasonCode),
		Reason:            strings.TrimSpace(reason),
		RecommendedAction: recommended,
	}
}
