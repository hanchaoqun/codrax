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

func TestBuildWorkflowStateViolationsProjectsTypedIssues(t *testing.T) {
	state := WorkflowStateView{
		MaterialCoverageSufficient: true,
		ContributionLedgerRequired: true,
		FieldContractViolations: []FieldContractIssue{{
			ActionID:          "filter",
			ActionKind:        string(dataquery.DataActionFilterRecords),
			InputAlias:        "records.json",
			MissingFields:     []string{"status"},
			RepairActionHints: []string{string(dataquery.DataActionDeriveFields)},
			Reason:            "status is missing",
		}},
		ZeroMatchFilterViolations: []ZeroMatchFilterIssue{{
			ArtifactID:        "filtered.json",
			InputPath:         "records.json",
			FilterFields:      []string{"status"},
			RepairActionHints: []string{string(dataquery.DataActionValueDistribution)},
			Reason:            "filter matched zero rows",
		}},
		UnmatchedResolutionViolations: []UnmatchedResolutionIssue{{
			ArtifactID:        "resolved.json",
			BasePath:          "base.json",
			TargetFields:      []string{"canonical_id"},
			RepairActionHints: []string{string(dataquery.DataActionApplyResolutions)},
			Reason:            "all rows unmatched",
		}},
		ZeroEligibleViolations: []ZeroEligibleIssue{{
			ArtifactID:        "eligible.json",
			InputPath:         "resolved.json",
			RepairActionHints: []string{string(dataquery.DataActionQualifyRecords)},
			Reason:            "no eligible rows",
		}},
	}
	violations := BuildWorkflowStateViolations(WorkflowStateViolationInput{State: state})
	var codes []string
	for _, violation := range violations {
		codes = append(codes, violation.Code)
	}
	want := []string{"field_contract_violation", "zero_match_filter", "unmatched_resolution", "zero_eligible_records"}
	if len(codes) != len(want) {
		t.Fatalf("codes=%v, want %v", codes, want)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("codes=%v, want %v", codes, want)
		}
	}
	if violations[0].ActionID != "filter" || violations[0].InputAlias != "records.json" || violations[0].MissingFields[0] != "status" {
		t.Fatalf("field violation=%+v, want projected field contract", violations[0])
	}
	if violations[1].ActionKind != string(dataquery.DataActionFilterRecords) || violations[2].ActionKind != string(dataquery.DataActionApplyResolutions) || violations[3].ActionKind != string(dataquery.DataActionQualifyRecords) {
		t.Fatalf("violation action kinds=%+v", violations)
	}
}
