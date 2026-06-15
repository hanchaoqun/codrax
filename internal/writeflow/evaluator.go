package writeflow

import "github.com/hanchaoqun/codrax/internal/types"

// EvaluationStatus is the deterministic next action for the write workflow
// controller.
type EvaluationStatus string

const (
	EvalContinueExplore  EvaluationStatus = "continue_explore"
	EvalContinuePlan     EvaluationStatus = "continue_plan"
	EvalNeedsApproval    EvaluationStatus = "needs_approval"
	EvalComplete         EvaluationStatus = "complete"
	EvalBlocked          EvaluationStatus = "blocked"
	EvalDangerDenied     EvaluationStatus = "danger_denied"
	EvalRollbackRequired EvaluationStatus = "rollback_required"
	EvalBudgetExhausted  EvaluationStatus = "budget_exhausted"
)

// EvaluationInput is the deterministic evidence set for one workflow
// evaluation. Nil pointers mean the artifact has not been produced yet.
type EvaluationInput struct {
	Workflow         WriteWorkflowPlan
	Batch            *WriteBatchPlan
	Plan             *types.ChangePlan
	Report           *types.ChangeReport
	RiskAssessment   RiskAssessment
	ApprovalDecision ApprovalDecision
	BatchCount       int
	MaxBatches       int
}

// WriteEvaluation is the typed verdict consumed by the future workflow loop.
type WriteEvaluation struct {
	Status         EvaluationStatus `json:"status"`
	ReasonCode     string           `json:"reason_code"`
	Reason         string           `json:"reason,omitempty"`
	NextBatchID    string           `json:"next_batch_id,omitempty"`
	RiskLevel      RiskLevel        `json:"risk_level,omitempty"`
	ApprovalAction ApprovalAction   `json:"approval_action,omitempty"`
}

// EvaluateWriteWorkflow returns a deterministic next action. It does not
// inspect request prose, model explanations, or test output text; those can
// guide LLM planning, but hard state transitions come from typed artifacts.
func EvaluateWriteWorkflow(input EvaluationInput) WriteEvaluation {
	risk := input.RiskAssessment
	approval := input.ApprovalDecision
	if risk.Level == RiskCritical || approval.Action == ApprovalActionDeny {
		return input.verdict(EvalDangerDenied, "critical_risk_denied", "critical write risk is denied")
	}
	if input.MaxBatches > 0 && input.BatchCount >= input.MaxBatches {
		return input.verdict(EvalBudgetExhausted, "max_batches_reached", "write workflow reached its batch budget")
	}
	workflow := NormalizeWorkflowPlan(input.Workflow)
	switch workflow.Status {
	case WorkflowBlocked, WorkflowNeedsClarification:
		return input.verdict(EvalBlocked, string(workflow.Status), "workflow cannot continue without external input")
	case WorkflowComplete:
		return input.verdict(EvalComplete, "workflow_complete", "workflow is marked complete")
	}
	if approval.Action == ApprovalActionManual {
		return input.verdict(EvalNeedsApproval, "manual_approval_required", "approval decision requires manual review")
	}
	if input.Batch != nil {
		batch := NormalizeBatchPlan(*input.Batch)
		switch batch.Status {
		case BatchNeedsExploration:
			return input.verdictWithBatch(EvalContinueExplore, "batch_needs_exploration", "next batch needs targeted source exploration", batch.ID)
		case BatchBlocked:
			return input.verdictWithBatch(EvalBlocked, "batch_blocked", "batch is blocked", batch.ID)
		case BatchNeedsRepair:
			return input.verdictWithBatch(EvalContinuePlan, "batch_needs_repair", "batch needs a narrower repair plan", batch.ID)
		case BatchComplete:
			if workflow.NextBatch == nil {
				return input.verdictWithBatch(EvalComplete, "last_batch_complete", "last batch is complete", batch.ID)
			}
		}
	}
	if input.Plan == nil {
		return input.verdict(EvalContinuePlan, "missing_change_plan", "no change plan has been produced for the current batch")
	}
	switch input.Plan.Status {
	case types.PlanStatusPartiallyApplied:
		return input.verdict(EvalRollbackRequired, "partial_apply", "plan is partially applied and needs retry or rollback")
	case types.PlanStatusApplyFailed:
		return input.verdict(EvalContinuePlan, "apply_failed", "apply failed before verification")
	case types.PlanStatusVerifyFailed:
		return input.verdict(EvalContinuePlan, "verify_failed", "verification failed and needs a repair batch")
	case types.PlanStatusUnverified:
		return input.verdict(EvalContinuePlan, "unverified", "plan changed code without a validating test signal")
	case types.PlanStatusPending:
		if approval.Action == ApprovalActionAutoExecute {
			return input.verdict(EvalContinuePlan, "pending_auto_plan", "pending plan is eligible for automatic execution")
		}
		return input.verdict(EvalNeedsApproval, "pending_manual_plan", "pending plan requires approval")
	}
	if input.Report != nil {
		status := input.Report.NormalizeVerificationStatus()
		if status == types.VerificationStatusUnavailable {
			return input.verdict(EvalContinuePlan, "unverified", "plan changed code without a validating test signal")
		}
		if input.Report.BuildFailed {
			return input.verdict(EvalContinuePlan, "build_failed", "build failed and needs a repair batch")
		}
		if status == types.VerificationStatusFailed {
			return input.verdict(EvalContinuePlan, "tests_failed", "tests failed and need a repair batch")
		}
		if input.Plan.Status == types.PlanStatusApplied && workflow.NextBatch == nil {
			return input.verdict(EvalComplete, "applied_and_verified", "plan applied and verification passed")
		}
	}
	if input.Plan.Status == types.PlanStatusApplied && workflow.NextBatch == nil {
		return input.verdict(EvalComplete, "applied_no_more_batches", "plan applied and no further batch is queued")
	}
	return input.verdict(EvalContinuePlan, "continue_next_batch", "workflow still has work to plan")
}

func (input EvaluationInput) verdict(status EvaluationStatus, code, reason string) WriteEvaluation {
	return input.verdictWithBatch(status, code, reason, "")
}

func (input EvaluationInput) verdictWithBatch(status EvaluationStatus, code, reason, batchID string) WriteEvaluation {
	return WriteEvaluation{
		Status:         status,
		ReasonCode:     code,
		Reason:         reason,
		NextBatchID:    batchID,
		RiskLevel:      input.RiskAssessment.Level,
		ApprovalAction: input.ApprovalDecision.Action,
	}
}
