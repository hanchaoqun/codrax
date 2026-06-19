package writeflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestApprovalEffectBindingMatchesActiveRuntimeUnitScope(t *testing.T) {
	plan := approvalEffectPlanForTest()
	run := approvalEffectRunForTest("slice-1", "pkg/a.go")
	assessment := AssessWriteRisk(AssessmentInput{Plan: plan})
	decision := ApprovalDecision{Policy: ApprovalPolicyAutoSafe, Action: ApprovalActionManual, ReasonCode: "high_write_risk"}
	record := NewApprovalRecord(assessment, decision, "test", "approved", types.PlanFingerprint(plan), "operator")
	BindApprovalRecordEffect(record, plan, &run)
	if record.EffectFingerprint == "" {
		t.Fatalf("effect fingerprint should be stamped: %+v", record)
	}
	if !ApprovalRecordEffectMatches(record, plan, &run) {
		t.Fatalf("record should match same runtime unit scope: %+v", record)
	}
	otherUnit := approvalEffectRunForTest("slice-2", "pkg/a.go")
	if ApprovalRecordEffectMatches(record, plan, &otherUnit) {
		t.Fatalf("record must not match a different runtime unit scope")
	}
	otherPath := approvalEffectRunForTest("slice-1", "pkg/b.go")
	if ApprovalRecordEffectMatches(record, plan, &otherPath) {
		t.Fatalf("record must not match a different path scope")
	}
}

func TestApprovalEffectBindingPlanOnlyScope(t *testing.T) {
	plan := approvalEffectPlanForTest()
	assessment := AssessWriteRisk(AssessmentInput{Plan: plan})
	decision := ApprovalDecision{Policy: ApprovalPolicyAutoSafe, Action: ApprovalActionManual, ReasonCode: "high_write_risk"}
	record := NewApprovalRecord(assessment, decision, "test", "approved", types.PlanFingerprint(plan), "operator")
	BindApprovalRecordEffect(record, plan, nil)
	if record.EffectFingerprint == "" || len(record.EffectScope) == 0 {
		t.Fatalf("plan-only approval effect should be stamped: %+v", record)
	}
	if !ApprovalRecordEffectMatches(record, plan, nil) {
		t.Fatalf("plan-only effect should match without run: %+v", record)
	}
}

func approvalEffectPlanForTest() *types.ChangePlan {
	return &types.ChangePlan{
		ID:          "plan-effect",
		Summary:     "update file",
		TargetPaths: []string{"pkg/a.go"},
		Changes: []types.FileChange{{
			Path:       "pkg/a.go",
			Kind:       "modify",
			NewContent: "new",
			Rationale:  "exercise approval effect binding",
		}},
	}
}

func approvalEffectRunForTest(sliceID, path string) types.WriteWorkflowRun {
	return types.WriteWorkflowRun{
		RunID:         "wf-effect",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchPendingApproval,
			PlanID:        "plan-effect",
			ActiveSliceID: sliceID,
			Slices: []types.WriteWorkflowSlice{{
				ID:            sliceID,
				Status:        types.ChangePlanSlicePending,
				PlanID:        "plan-effect",
				Paths:         []string{path},
				ChangeIndexes: []int{0},
			}},
		}},
	}
}
