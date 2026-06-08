package dataworkflow

import (
	"strings"
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

func TestActiveGuardViolationsPreserveInitialBlockedSuffix(t *testing.T) {
	action := dataquery.DataAction{
		ID:             "filter_missing",
		Kind:           dataquery.DataActionFilterRecords,
		InputPaths:     []string{"records.json"},
		OutputArtifact: "filtered.json",
	}
	guard := ActionInputContractGuardResult(ActionInputContractGuardInput{
		Code:       "missing_action_inputs",
		Action:     action,
		InputAlias: "records.json",
		Message:    "filter needs records",
	})
	records := []WorkflowRecord{{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{action}},
		Err:  guard.ErrorText(),
		Admission: &ActionDAGAdmissionDecision{
			Plan:          dataquery.TaskPlan{Actions: []dataquery.DataAction{action}},
			FinalGuard:    guard,
			FinalGuardErr: guard.ErrorText(),
		},
	}}
	violations := ActiveGuardViolationsFromRecords(records)
	if len(violations) != 1 || violations[0].Code != "missing_action_inputs" {
		t.Fatalf("violations=%+v, want active admission guard", violations)
	}
}

func TestActiveGuardViolationsDropsGuardsBeforeSuccessfulProgress(t *testing.T) {
	staleAction := dataquery.DataAction{
		ID:             "filter_raw_text",
		Kind:           dataquery.DataActionFilterRecords,
		InputPaths:     []string{"notes.txt"},
		OutputArtifact: "filtered.json",
	}
	staleGuard := ActionInputContractGuardResult(ActionInputContractGuardInput{
		Code:       "artifact_schema_incompatible",
		Action:     staleAction,
		InputAlias: "notes.txt",
		Message:    "notes.txt is not a record artifact",
	})
	activeAction := dataquery.DataAction{
		ID:             "join_missing",
		Kind:           dataquery.DataActionJoinRecords,
		InputPaths:     []string{"left.json", "right.json"},
		OutputArtifact: "joined.json",
	}
	activeGuard := ActionInputContractGuardResult(ActionInputContractGuardInput{
		Code:       "missing_join_field",
		Action:     activeAction,
		InputAlias: "right.json",
		Message:    "right.json is missing join field",
	})
	records := []WorkflowRecord{{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{staleAction}},
		Err:  staleGuard.ErrorText(),
		Admission: &ActionDAGAdmissionDecision{
			Plan:          dataquery.TaskPlan{Actions: []dataquery.DataAction{staleAction}},
			FinalGuard:    staleGuard,
			FinalGuardErr: staleGuard.ErrorText(),
		},
	}, {
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:             "extract_records",
			Kind:           dataquery.DataActionExtractRecords,
			OutputArtifact: "records.json",
		}}},
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:      "records.json",
			Kind:    "records",
			Headers: []string{"id", "status"},
			Fields:  map[string]string{"id": "string", "status": "string"},
		}}},
	}, {
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{activeAction}},
		Err:  activeGuard.ErrorText(),
		Admission: &ActionDAGAdmissionDecision{
			Plan:          dataquery.TaskPlan{Actions: []dataquery.DataAction{activeAction}},
			FinalGuard:    activeGuard,
			FinalGuardErr: activeGuard.ErrorText(),
		},
	}}
	violations := ActiveGuardViolationsFromRecords(records)
	if len(violations) != 1 || violations[0].Code != "missing_join_field" {
		t.Fatalf("violations=%+v, want only guard after latest successful progress", violations)
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
			Role:                 "left",
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
		got.Severity != "error" ||
		got.Repairability != RepairNeedsTypedAction {
		t.Fatalf("violation=%+v, want typed workflow action/input shape", got)
	}
	if got.IdempotencyKey == "" || len(got.InputAliases) != 2 || got.InputAliases[0] != "orders.json" || got.InputAliases[1] != "customers.json" {
		t.Fatalf("violation=%+v, want action-derived idempotency and inputs", got)
	}
	if len(got.MissingFields) != 1 || got.MissingFields[0] != "customer_id" {
		t.Fatalf("MissingFields=%v, want customer_id", got.MissingFields)
	}
	if got.Role != "left" {
		t.Fatalf("Role=%q, want preserved role", got.Role)
	}
	if len(got.AvailableFieldSample) != 2 || got.AvailableFieldSample[0] != "order_id" {
		t.Fatalf("AvailableFieldSample=%v, want preserved sample", got.AvailableFieldSample)
	}
	if len(got.RepairActionHints) != 1 || got.RepairActionHints[0] != string(dataquery.DataActionDeriveFields) {
		t.Fatalf("RepairActionHints=%v, want derive_fields", got.RepairActionHints)
	}
}

func TestWorkflowViolationsFromRecordExecutionProjectsUnknownTextRuntimeFailureAsWarning(t *testing.T) {
	records := []WorkflowRecord{{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:   "script_1",
			Kind: dataquery.DataActionCustomTransform,
		}}},
		Violations: []dataquery.DataTaskViolation{dataquery.ClassifyExecutionError(`Traceback:
  File "<string>", line 4, in <module>
NameError: name 'status' is not defined`)},
	}}

	violations := WorkflowViolationsFromRecordExecution(records)
	if len(violations) != 1 {
		t.Fatalf("violations=%+v, want one diagnostic violation", violations)
	}
	got := violations[0]
	if got.Code != "runtime_failure" ||
		got.Severity != "warning" ||
		!strings.Contains(got.Reason, "NameError") {
		t.Fatalf("violation=%+v, want text fallback runtime diagnostic warning", got)
	}
}

func TestWorkflowViolationsFromRecordExecutionProjectsMaterialContractAsTypedAction(t *testing.T) {
	records := []WorkflowRecord{{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:         "profile_inputs",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"materials"},
		}}},
		Violations: []dataquery.DataTaskViolation{{
			Code:          "material_contract_violation",
			Summary:       "materials is a directory",
			ActionID:      "profile_inputs",
			ActionKind:    string(dataquery.DataActionCustomTransform),
			InputAlias:    "materials",
			InputAliases:  []string{"materials"},
			Operation:     "script_consumed",
			ExpectedShape: "concrete file input",
			Repairability: dataquery.RepairabilityNeedsRecompute,
			RepairHint:    string(dataquery.DataActionExtractRecords),
		}},
	}}

	violations := WorkflowViolationsFromRecordExecution(records)
	if len(violations) != 1 {
		t.Fatalf("violations=%+v, want one material contract violation", violations)
	}
	got := violations[0]
	if got.Code != "material_contract_violation" ||
		got.ActionID != "profile_inputs" ||
		got.ActionKind != string(dataquery.DataActionCustomTransform) ||
		got.InputAlias != "materials" ||
		got.Operation != "script_consumed" ||
		got.Repairability != RepairNeedsTypedAction {
		t.Fatalf("violation=%+v, want typed material contract projection", got)
	}
}

func TestWorkflowViolationsFromRecordExecutionProjectsActionDependencyAsTypedAction(t *testing.T) {
	records := []WorkflowRecord{{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:   "reconcile_now",
			Kind: dataquery.DataActionReconcile,
		}}},
		Violations: []dataquery.DataTaskViolation{{
			Code:          "action_dependency_violation",
			Summary:       "reconcile_artifacts requires prior contribution records",
			ActionID:      "reconcile_now",
			ActionKind:    string(dataquery.DataActionReconcile),
			Role:          "contributions",
			Operation:     "reconcile",
			ExpectedShape: "non-empty contribution ledger",
			Repairability: dataquery.RepairabilityNeedsRecompute,
			RepairHint:    string(dataquery.DataActionComputeContribs),
		}},
	}}

	violations := WorkflowViolationsFromRecordExecution(records)
	if len(violations) != 1 {
		t.Fatalf("violations=%+v, want one action dependency violation", violations)
	}
	got := violations[0]
	if got.Code != "action_dependency_violation" ||
		got.ActionID != "reconcile_now" ||
		got.ActionKind != string(dataquery.DataActionReconcile) ||
		got.Role != "contributions" ||
		got.Operation != "reconcile" ||
		got.Repairability != RepairNeedsTypedAction {
		t.Fatalf("violation=%+v, want typed dependency projection", got)
	}
	if len(got.RepairActionHints) != 1 || got.RepairActionHints[0] != string(dataquery.DataActionComputeContribs) {
		t.Fatalf("RepairActionHints=%v, want compute_contributions", got.RepairActionHints)
	}
}

func TestBuildWorkflowViolationSummaryCountsTypedBlockers(t *testing.T) {
	summary := BuildWorkflowViolationSummary([]WorkflowViolation{
		{
			Code:              "value_contract_violation",
			Severity:          "error",
			Repairability:     RepairNeedsTypedAction,
			ActionID:          "compute",
			ActionKind:        string(dataquery.DataActionComputeContribs),
			InputAlias:        "records.json",
			Field:             "amount",
			Operation:         "numeric_parse",
			Role:              "target",
			RepairActionHints: []string{string(dataquery.DataActionDeriveFields)},
			Reason:            "amount is not numeric",
		},
		{Code: "zero_match_filter", Severity: "warning"},
		{Code: "value_contract_violation", Severity: "error"},
	})
	if summary.Total != 3 || summary.ErrorCount != 2 || summary.WarningCount != 1 {
		t.Fatalf("summary=%+v, want total/error/warning counts", summary)
	}
	if len(summary.ByCode) != 2 || summary.ByCode[0].Code != "value_contract_violation" || summary.ByCode[0].Count != 2 || summary.ByCode[1].Code != "zero_match_filter" {
		t.Fatalf("ByCode=%+v, want sorted code counts", summary.ByCode)
	}
	if summary.FirstActionID != "compute" ||
		summary.FirstActionKind != string(dataquery.DataActionComputeContribs) ||
		summary.FirstInputAlias != "records.json" ||
		summary.FirstField != "amount" ||
		summary.FirstOperation != "numeric_parse" ||
		summary.FirstRole != "target" ||
		summary.FirstRepairActionHints[0] != string(dataquery.DataActionDeriveFields) {
		t.Fatalf("summary=%+v, want first blocker projection", summary)
	}
	clone := CloneWorkflowViolationSummary(summary)
	clone.ByCode[0].Code = "mutated"
	clone.FirstRepairActionHints[0] = "mutated"
	if summary.ByCode[0].Code != "value_contract_violation" || summary.FirstRepairActionHints[0] != string(dataquery.DataActionDeriveFields) {
		t.Fatalf("summary clone leaked mutation: %+v", summary)
	}
}

func TestWorkflowViolationProjectionCarriesParamAndLimit(t *testing.T) {
	violation := WorkflowViolationFromDataTaskViolation(dataquery.DataTaskViolation{
		Code:          "action_limit_violation",
		Summary:       "too many rows",
		ActionID:      "filter_limit",
		ActionKind:    string(dataquery.DataActionFilterRecords),
		Param:         "max_output_records",
		Limit:         1,
		Observed:      2,
		Repairability: dataquery.RepairabilityNeedsRecompute,
		RepairHint:    string(dataquery.DataActionFilterRecords),
	}, dataquery.DataAction{
		ID:             "filter_limit",
		Kind:           dataquery.DataActionFilterRecords,
		InputPaths:     []string{"records.csv"},
		OutputArtifact: "filtered.json",
	})
	if violation.Code != "action_limit_violation" ||
		violation.ActionID != "filter_limit" ||
		violation.Param != "max_output_records" ||
		violation.Limit != 1 ||
		violation.Observed != 2 ||
		violation.InputAlias != "records.csv" {
		t.Fatalf("violation=%+v, want param/limit projection", violation)
	}
	summary := BuildWorkflowViolationSummary([]WorkflowViolation{violation})
	if summary.FirstParam != "max_output_records" || summary.FirstLimit != 1 || summary.FirstObserved != 2 {
		t.Fatalf("summary=%+v, want first param/limit fields", summary)
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
	if state.WorkflowViolationSummary.Total != 1 ||
		state.WorkflowViolationSummary.FirstCode != "field_contract_violation" ||
		state.WorkflowViolationSummary.FirstActionID != "filter_paid" {
		t.Fatalf("WorkflowViolationSummary=%+v, want projected execution violation summary", state.WorkflowViolationSummary)
	}
	snapshot := state.Snapshot()
	state.WorkflowViolationSummary.FirstActionID = "mutated"
	if snapshot.WorkflowViolationSummary.FirstActionID != "filter_paid" {
		t.Fatalf("snapshot summary leaked mutation: %+v", snapshot.WorkflowViolationSummary)
	}
}
