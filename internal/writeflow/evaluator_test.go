package writeflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEvaluateWriteWorkflowDangerDenied(t *testing.T) {
	got := EvaluateWriteWorkflow(EvaluationInput{
		RiskAssessment:   RiskAssessment{Level: RiskCritical},
		ApprovalDecision: ApprovalDecision{Action: ApprovalActionAutoExecute},
	})
	if got.Status != EvalDangerDenied {
		t.Fatalf("status = %s; want %s", got.Status, EvalDangerDenied)
	}
}

func TestEvaluateWriteWorkflowHighRiskNeedsApproval(t *testing.T) {
	got := EvaluateWriteWorkflow(EvaluationInput{
		Workflow: WriteWorkflowPlan{
			Goal:            "change public API",
			SuccessCriteria: []string{"tests pass"},
			Status:          WorkflowInProgress,
			NextBatch:       &WriteBatchPlan{Goal: "edit API", Status: BatchReadyForChangePlan},
		},
		RiskAssessment:   RiskAssessment{Level: RiskHigh},
		ApprovalDecision: ApprovalDecision{Action: ApprovalActionManual},
		Plan:             &types.ChangePlan{Status: types.PlanStatusPending},
	})
	if got.Status != EvalNeedsApproval {
		t.Fatalf("status = %s; want %s", got.Status, EvalNeedsApproval)
	}
}

func TestEvaluateWriteWorkflowBatchNeedsExploration(t *testing.T) {
	batch := &WriteBatchPlan{ID: "batch-1", Goal: "inspect planner", Status: BatchNeedsExploration}
	got := EvaluateWriteWorkflow(EvaluationInput{
		Workflow: WriteWorkflowPlan{
			Goal:            "make write mode staged",
			SuccessCriteria: []string{"tests pass"},
			Status:          WorkflowInProgress,
			NextBatch:       batch,
		},
		Batch:            batch,
		RiskAssessment:   RiskAssessment{Level: RiskMedium},
		ApprovalDecision: ApprovalDecision{Action: ApprovalActionAutoExecute},
	})
	if got.Status != EvalContinueExplore || got.NextBatchID != "batch-1" {
		t.Fatalf("evaluation = %+v; want continue_explore batch-1", got)
	}
}

func TestEvaluateWriteWorkflowVerifyFailureContinuesPlan(t *testing.T) {
	got := EvaluateWriteWorkflow(EvaluationInput{
		Workflow: WriteWorkflowPlan{
			Goal:            "fix bug",
			SuccessCriteria: []string{"test passes"},
			Status:          WorkflowInProgress,
		},
		Plan:             &types.ChangePlan{Status: types.PlanStatusVerifyFailed},
		RiskAssessment:   RiskAssessment{Level: RiskMedium},
		ApprovalDecision: ApprovalDecision{Action: ApprovalActionAutoExecute},
	})
	if got.Status != EvalContinuePlan || got.ReasonCode != "verify_failed" {
		t.Fatalf("evaluation = %+v; want verify_failed continue_plan", got)
	}
}

func TestEvaluateWriteWorkflowUnverifiedContinuesPlan(t *testing.T) {
	got := EvaluateWriteWorkflow(EvaluationInput{
		Workflow: WriteWorkflowPlan{
			Goal:            "modify source",
			SuccessCriteria: []string{"tests pass"},
			Status:          WorkflowInProgress,
		},
		Plan:             &types.ChangePlan{Status: types.PlanStatusUnverified},
		RiskAssessment:   RiskAssessment{Level: RiskMedium},
		ApprovalDecision: ApprovalDecision{Action: ApprovalActionAutoExecute},
	})
	if got.Status != EvalContinuePlan || got.ReasonCode != "unverified" {
		t.Fatalf("evaluation = %+v; want unverified continue_plan", got)
	}
}

func TestEvaluateWriteWorkflowAppliedAndVerifiedComplete(t *testing.T) {
	got := EvaluateWriteWorkflow(EvaluationInput{
		Workflow: WriteWorkflowPlan{
			Goal:            "fix bug",
			SuccessCriteria: []string{"test passes"},
			Status:          WorkflowInProgress,
		},
		Plan:             &types.ChangePlan{Status: types.PlanStatusApplied},
		Report:           &types.ChangeReport{Passed: true},
		RiskAssessment:   RiskAssessment{Level: RiskLow},
		ApprovalDecision: ApprovalDecision{Action: ApprovalActionAutoExecute},
	})
	if got.Status != EvalComplete || got.ReasonCode != "applied_and_verified" {
		t.Fatalf("evaluation = %+v; want applied_and_verified complete", got)
	}
}

func TestEvaluateWriteWorkflowBudgetExhausted(t *testing.T) {
	got := EvaluateWriteWorkflow(EvaluationInput{
		Workflow:         WriteWorkflowPlan{Goal: "fix", SuccessCriteria: []string{"done"}, Status: WorkflowInProgress},
		BatchCount:       3,
		MaxBatches:       3,
		RiskAssessment:   RiskAssessment{Level: RiskLow},
		ApprovalDecision: ApprovalDecision{Action: ApprovalActionAutoExecute},
	})
	if got.Status != EvalBudgetExhausted {
		t.Fatalf("status = %s; want %s", got.Status, EvalBudgetExhausted)
	}
}
