package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestBuildWorkflowViolationsCombinesGuardNoProgressAndAdditional(t *testing.T) {
	guard := WorkflowViolation{Code: "guard_blocked"}
	additional := WorkflowViolation{Code: "field_contract_violation"}
	violations := BuildWorkflowViolations(WorkflowViolationInput{
		Facts: StageFacts{
			MaterialCoverageSufficient: true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		ProgressEvents: []ProgressEvent{
			{
				ResultPresent: true,
				Actions: []dataquery.DataAction{{
					Kind: dataquery.DataActionJoinRecords,
				}},
			},
			{
				ResultPresent: true,
				Actions: []dataquery.DataAction{{
					Kind: dataquery.DataActionJoinRecords,
				}},
			},
		},
		NoProgressThreshold: 2,
		GuardViolations:     []WorkflowViolation{guard},
		Additional:          []WorkflowViolation{additional},
	})
	var codes []string
	for _, violation := range violations {
		codes = append(codes, violation.Code)
	}
	want := []string{"guard_blocked", ViolationStageNoProgress, "field_contract_violation"}
	if len(codes) != len(want) {
		t.Fatalf("codes=%v, want %v", codes, want)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("codes=%v, want %v", codes, want)
		}
	}
}
