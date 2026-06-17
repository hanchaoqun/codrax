package writeflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestValidateWorkflowTransitionApplyReadyRequiresApply(t *testing.T) {
	view := WorkflowExecutionView{Mode: types.ModeApply, State: WorkflowExecutionApplyReady}

	allowed := ValidateWorkflowTransition(view, WriteWorkflowDecision{Action: ActionApplyPlan})
	if !allowed.Allowed {
		t.Fatalf("apply_plan should be allowed: %+v", allowed)
	}
	denied := ValidateWorkflowTransition(view, WriteWorkflowDecision{
		Action:           ActionAskUser,
		QuestionsForUser: []string{"approve?"},
		MissingFacts: []MissingUserFact{{
			Kind:        "approval_boundary",
			Description: "approval requested",
			Consumer:    "controller",
		}},
	})
	if denied.Allowed || denied.ReasonCode != "apply_ready_requires_apply" || denied.RecommendedAction != ActionApplyPlan {
		t.Fatalf("ask_user should be redirected to apply_plan: %+v", denied)
	}
}

func TestValidateWorkflowTransitionPendingApprovalBlocksApply(t *testing.T) {
	view := WorkflowExecutionView{Mode: types.ModeApply, State: WorkflowExecutionPendingApproval}

	denied := ValidateWorkflowTransition(view, WriteWorkflowDecision{Action: ActionApplyPlan})
	if denied.Allowed || denied.ReasonCode != "manual_approval_required" || denied.RecommendedAction != ActionAskUser {
		t.Fatalf("apply should be blocked while approval is pending: %+v", denied)
	}
}

func TestValidateWorkflowTransitionInvalidApprovalBlocksApply(t *testing.T) {
	view := WorkflowExecutionView{Mode: types.ModeApply, State: WorkflowExecutionApprovalInvalid}

	denied := ValidateWorkflowTransition(view, WriteWorkflowDecision{Action: ActionApplyPlan})
	if denied.Allowed || denied.ReasonCode != "approval_authority_invalid" || denied.RecommendedAction != ActionAskUser {
		t.Fatalf("apply should be blocked while approval authority is invalid: %+v", denied)
	}
}

func TestValidateWorkflowTransitionObserveRequiredBlocksPlanning(t *testing.T) {
	view := WorkflowExecutionView{Mode: types.ModeApply, State: WorkflowExecutionObserveRequired}

	denied := ValidateWorkflowTransition(view, WriteWorkflowDecision{
		Action: ActionPlanBatch,
		Batch:  &WriteBatchPlan{ID: "batch-1", Goal: "plan more work"},
	})
	if denied.Allowed || denied.ReasonCode != "post_apply_observe_required" || denied.RecommendedAction != ActionVerifyBatch {
		t.Fatalf("planning should be blocked until observation: %+v", denied)
	}
}

func TestValidateWorkflowTransitionNeedsReplanBlocksStaleApplyVerify(t *testing.T) {
	view := WorkflowExecutionView{Mode: types.ModeApply, State: WorkflowExecutionNeedsReplan, BatchID: "batch-1"}

	for _, action := range []WorkflowAction{ActionApplyPlan, ActionVerifyBatch, ActionPlanBatch} {
		decision := WriteWorkflowDecision{Action: action}
		if action == ActionPlanBatch {
			decision.Batch = &WriteBatchPlan{ID: "batch-1", Goal: "reuse old plan"}
		}
		got := ValidateWorkflowTransition(view, decision)
		if got.Allowed || got.ReasonCode != "failed_verify_requires_replan" || got.RecommendedAction != ActionReplanBatch {
			t.Fatalf("%s should be blocked after failed verify: %+v", action, got)
		}
	}
	if got := ValidateWorkflowTransition(view, WriteWorkflowDecision{
		Action: ActionReplanBatch,
		Batch:  &WriteBatchPlan{ID: "batch-1", Goal: "repair failed point"},
	}); !got.Allowed {
		t.Fatalf("replan should be allowed after failed verify: %+v", got)
	}
	if got := ValidateWorkflowTransition(view, WriteWorkflowDecision{
		Action: ActionAppendBatch,
		Batch:  &WriteBatchPlan{ID: "batch-2", Goal: "follow-up work"},
	}); !got.Allowed {
		t.Fatalf("append to a new batch should be allowed after failed verify: %+v", got)
	}
	if got := ValidateWorkflowTransition(view, WriteWorkflowDecision{
		Action: ActionAppendBatch,
		Batch:  &WriteBatchPlan{ID: "batch-1", Goal: "reuse failed batch"},
	}); got.Allowed || got.ReasonCode != "failed_verify_requires_replan" {
		t.Fatalf("append targeting the failed batch should be blocked: %+v", got)
	}
}

func TestValidateWorkflowTransitionHonorsModeActionSet(t *testing.T) {
	view := WorkflowExecutionView{Mode: types.ModePlan, State: WorkflowExecutionPlanReady}

	got := ValidateWorkflowTransition(view, WriteWorkflowDecision{Action: ActionApplyPlan})
	if got.Allowed || got.ReasonCode != "action_not_allowed_in_mode" {
		t.Fatalf("apply must be rejected in plan mode: %+v", got)
	}
}

func TestValidateWorkflowTransitionRejectsInvalidDecisionShape(t *testing.T) {
	view := WorkflowExecutionView{Mode: types.ModeApply, State: WorkflowExecutionReadyToPlan}

	got := ValidateWorkflowTransition(view, WriteWorkflowDecision{Action: ActionPlanBatch})
	if got.Allowed || got.ReasonCode != "invalid_decision" {
		t.Fatalf("invalid decision should be rejected structurally: %+v", got)
	}
}
