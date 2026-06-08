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

func TestWorkflowViolationsFromRecordExecutionProjectsDataTaskViolation(t *testing.T) {
	records := []WorkflowRecord{{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:             "join_customers",
			Kind:           dataquery.DataActionJoinRecords,
			InputPaths:     []string{"orders.json", "customers.json"},
			OutputArtifact: "joined.json",
		}}},
		Violations: []dataquery.DataTaskViolation{{
			Code:                 "field_contract_violation",
			Summary:              "orders.json is missing customer_id",
			ActionID:             "join_customers",
			ActionKind:           string(dataquery.DataActionJoinRecords),
			InputAlias:           "orders.json",
			MissingFields:        []string{"customer_id"},
			AvailableFieldSample: []string{"order_id", "amount"},
			Repairability:        dataquery.RepairabilityNeedsRecompute,
			RepairHint:           string(dataquery.DataActionDeriveFields),
		}},
	}}

	violations := WorkflowViolationsFromRecordExecution(records)
	if len(violations) != 1 {
		t.Fatalf("violations=%+v, want one projected execution violation", violations)
	}
	got := violations[0]
	if got.Code != "field_contract_violation" ||
		got.ActionID != "join_customers" ||
		got.ActionKind != string(dataquery.DataActionJoinRecords) ||
		got.InputAlias != "orders.json" ||
		got.OutputAlias != "joined.json" ||
		got.Repairability != RepairNeedsTypedAction {
		t.Fatalf("violation=%+v, want typed workflow action/input shape", got)
	}
	if got.IdempotencyKey == "" || len(got.InputAliases) != 2 || got.InputAliases[0] != "orders.json" || got.InputAliases[1] != "customers.json" {
		t.Fatalf("violation=%+v, want action-derived idempotency and inputs", got)
	}
	if len(got.MissingFields) != 1 || got.MissingFields[0] != "customer_id" {
		t.Fatalf("MissingFields=%v, want customer_id", got.MissingFields)
	}
	if len(got.AvailableFieldSample) != 2 || got.AvailableFieldSample[0] != "order_id" {
		t.Fatalf("AvailableFieldSample=%v, want preserved sample", got.AvailableFieldSample)
	}
	if len(got.RepairActionHints) != 1 || got.RepairActionHints[0] != string(dataquery.DataActionDeriveFields) {
		t.Fatalf("RepairActionHints=%v, want derive_fields", got.RepairActionHints)
	}
}

func TestBuildWorkflowStateViewProjectsRecordExecutionViolationIntoActionGraph(t *testing.T) {
	action := dataquery.DataAction{
		ID:             "filter_paid",
		Kind:           dataquery.DataActionFilterRecords,
		InputPaths:     []string{"records.json"},
		OutputArtifact: "paid.json",
	}
	state := BuildWorkflowStateView(WorkflowStateViewBuildInput{
		Records: []WorkflowRecord{{
			Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{action}},
			Violations: []dataquery.DataTaskViolation{{
				Code:          "field_contract_violation",
				Summary:       "status field is missing",
				ActionID:      "filter_paid",
				ActionKind:    string(dataquery.DataActionFilterRecords),
				InputAlias:    "records.json",
				MissingFields: []string{"status"},
			}},
		}},
		Current: dataquery.TaskPlan{Actions: []dataquery.DataAction{action}},
		WorkflowContract: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		HasMaterialProgress: true,
		NoProgressThreshold: 3,
	})

	var found *WorkflowViolation
	for i := range state.WorkflowViolations {
		if state.WorkflowViolations[i].Code == "field_contract_violation" &&
			state.WorkflowViolations[i].ActionID == "filter_paid" {
			found = &state.WorkflowViolations[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("WorkflowViolations=%+v, want execution field violation", state.WorkflowViolations)
	}
	if len(state.ActionGraph.Blocked) != 1 || state.ActionGraph.Blocked[0].ID != "filter_paid" {
		t.Fatalf("ActionGraph.Blocked=%+v, want failed action blocked from state violation", state.ActionGraph.Blocked)
	}
	if len(state.ActionGraph.Ready) != 0 {
		t.Fatalf("ActionGraph.Ready=%+v, want current failed action suppressed", state.ActionGraph.Ready)
	}
}
