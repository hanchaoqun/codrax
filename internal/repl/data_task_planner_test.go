package repl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/dataworkflow"
	"github.com/hanchaoqun/codrax/internal/llm"
)

func TestDataTaskPlannerCompatJSON(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			dataTaskPlanResp(`{"status":"ready","inputPaths":"orders.csv, vendors.csv","outputContract":{"format":"csv_line","explanationAllowed":"false"},"coverageContract":{"requiredMaterials":[{"id":"m1","path":"orders.csv","purpose":"input rows","usage_mode":"script_consumed","text_evidence_path":"","distilled_notes":"read rows","required":"true"}],"validationRules":"all totals reconcile; rows cite source","decisionRecordsRequired":"true","ruleCoverageRequired":"true","contributionLedgerRequired":"true","entityResolutionRequired":"true","reconcileRequired":"true"},"goal":123,"knownConstraints":"read only; strict output","missingObservations":["invoice total",42],"successCriteria":"final total returned","nextBatch":true,"whyThisBatch":456,"continueAfter":"true","script":"emit({\"answer\":\"ok,1\",\"output_contract\":{\"format\":\"csv_line\",\"explanation_allowed\":false}})",}` + "\ntrailing"),
		},
	}
	planner := NewDataTaskPlanner(adapter)
	plan, err := planner.PlanDataTask(context.Background(), "汇总 CSV", "/repo", TurnPolicy{Route: RouteData, DataTaskKind: "data_aggregation"}, []dataquery.CandidateFile{
		{Path: "orders.csv", Kind: "csv", Size: 10},
		{Path: "vendors.csv", Kind: "csv", Size: 10},
	})
	if err != nil {
		t.Fatalf("PlanDataTask: %v", err)
	}
	if plan.Status != "ready" {
		t.Fatalf("Status=%q", plan.Status)
	}
	if strings.Join(plan.InputPaths, ",") != "orders.csv,vendors.csv" {
		t.Fatalf("InputPaths=%v", plan.InputPaths)
	}
	if plan.OutputContract.Format != dataquery.OutputCSVLine || plan.OutputContract.ExplanationAllowed {
		t.Fatalf("OutputContract=%+v", plan.OutputContract)
	}
	if len(plan.CoverageContract.RequiredMaterials) != 1 || plan.CoverageContract.RequiredMaterials[0].Path != "orders.csv" || !plan.CoverageContract.DecisionRecordsRequired {
		t.Fatalf("CoverageContract=%+v", plan.CoverageContract)
	}
	if plan.CoverageContract.RequiredMaterials[0].UsageMode != dataquery.MaterialUseScriptConsumed || len(plan.CoverageContract.RequiredMaterials[0].DistilledNotes) != 1 {
		t.Fatalf("RequiredMaterials[0]=%+v", plan.CoverageContract.RequiredMaterials[0])
	}
	if !plan.CoverageContract.RuleCoverageRequired || !plan.CoverageContract.ContributionLedgerRequired || !plan.CoverageContract.EntityResolutionRequired || !plan.CoverageContract.ReconcileRequired {
		t.Fatalf("CoverageContract validation flags=%+v, want all true", plan.CoverageContract)
	}
	if len(plan.CoverageContract.ValidationRules) != 2 {
		t.Fatalf("ValidationRules=%v", plan.CoverageContract.ValidationRules)
	}
	if !strings.Contains(plan.Script, "emit") {
		t.Fatalf("Script=%q", plan.Script)
	}
	if plan.Goal != "123" {
		t.Fatalf("Goal=%q", plan.Goal)
	}
	if len(plan.KnownConstraints) != 2 || plan.KnownConstraints[0] != "read only" || plan.KnownConstraints[1] != "strict output" {
		t.Fatalf("KnownConstraints=%v", plan.KnownConstraints)
	}
	if len(plan.MissingObservations) != 2 || plan.MissingObservations[1] != "42" {
		t.Fatalf("MissingObservations=%v", plan.MissingObservations)
	}
	if len(plan.SuccessCriteria) != 1 || plan.SuccessCriteria[0] != "final total returned" {
		t.Fatalf("SuccessCriteria=%v", plan.SuccessCriteria)
	}
	if plan.NextBatch != "true" || plan.WhyThisBatch != "456" || !plan.ContinueAfter {
		t.Fatalf("batch fields next=%q why=%q continue=%t", plan.NextBatch, plan.WhyThisBatch, plan.ContinueAfter)
	}
	system := adapter.calls[0].messages[0].Content
	for _, want := range []string{
		"not source-code analysis",
		"or computer operation",
		"output_contract",
		"common data standard libraries",
		"open(path) is read-only",
		"print(...) is allowed",
		"json_records(path)",
		"result.artifact_access",
		"access catalog",
		"field_samples",
		"operation pipeline",
		"coverage_contract",
		"material inventory",
		"usage_mode",
		"text_evidence_consumed",
		"planner_distilled",
		"decision_records_required",
		"rule_coverage_required",
		"contribution_ledger_required",
		"entity_resolution_required",
		"reconcile_required",
		"result.contributions",
		"result.entity_resolutions",
		"result.reconcile",
		"assemble_answer",
		"rule_refs",
		"canonical ledger field names",
		"network/process libraries",
		"item-level decision records",
		"Do not emit a giant one-shot script",
		"continue_after=true",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("data planner system prompt missing %q:\n%s", want, system)
		}
	}
}

func TestDataTaskDeterministicContinuationFallbackDeriveRules(t *testing.T) {
	current := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{{
				Path:      "data_rules.md",
				Required:  true,
				UsageMode: dataquery.MaterialUseScriptConsumed,
			}},
			RuleCoverageRequired:       true,
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
	}
	records := []dataTaskWorkflowRecord{{
		Plan: current,
		Result: &dataquery.Result{
			ConsumedPaths: []string{"data_rules.md"},
		},
	}}
	plan, reason, ok := dataTaskDeterministicContinuationFallback(records, current, errors.New("data task planner returned no tool_call"))
	if !ok {
		t.Fatalf("fallback ok=false")
	}
	if !strings.Contains(reason, "derive_rules") {
		t.Fatalf("reason=%q, want derive_rules", reason)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionDeriveRules {
		t.Fatalf("actions=%+v, want derive_rules action", plan.Actions)
	}
	if !plan.ContinueAfter {
		t.Fatalf("ContinueAfter=false, want true")
	}
}

func TestDataTaskApplyResolutionGuardRejectsDiagnosticResolutionInput(t *testing.T) {
	access := []dataTaskArtifactAccessPrompt{{
		ID:        "records",
		Kind:      string(dataquery.DataActionExtractRecords),
		NodeClass: dataworkflow.ArtifactNodeClassRecord,
		Aliases:   []string{"records.json"},
		JSONShape: "array(len=2)",
		Fields:    []string{"item_id", "name"},
	}, {
		ID:        "records.json#entity_resolution_source",
		Kind:      "apply_entity_resolutions/resolution",
		NodeClass: dataworkflow.ArtifactNodeClassDiagnosticChild,
		Aliases:   []string{"records.json#entity_resolution_source"},
		JSONShape: "array(len=2)",
		Fields:    []string{"item_id", "source_value", "canonical_id", "canonical_label"},
	}}
	action := dataquery.DataAction{
		ID:         "apply_diag",
		Kind:       dataquery.DataActionApplyResolutions,
		InputPaths: []string{"records.json", "records.json#entity_resolution_source"},
		Params: map[string]string{
			"base_path": "records.json",
			"resolution_specs": `[{
				"resolution_path":"records.json#entity_resolution_source",
				"resolution_key_fields":["item_id"],
				"target_id_field":"name_canonical_id"
			}]`,
		},
	}
	guard := dataTaskApplyResolutionActionFieldContractGuardResult(access, action, 0)
	if guard.Empty() {
		t.Fatalf("guard empty, want diagnostic input rejection")
	}
	if guard.Code != "apply_resolution_diagnostic_input" {
		t.Fatalf("guard code=%q, want apply_resolution_diagnostic_input", guard.Code)
	}
	if len(guard.Violations) != 1 || guard.Violations[0].InputAlias != "records.json#entity_resolution_source" {
		t.Fatalf("violations=%+v", guard.Violations)
	}
}

func TestDataTaskApplyResolutionGuardAcceptsMappingWhenBaseMatchesLaterSourceLineage(t *testing.T) {
	access := []dataTaskArtifactAccessPrompt{{
		ID:          "resolved_records.json",
		Kind:        string(dataquery.DataActionApplyResolutions),
		NodeClass:   dataworkflow.ArtifactNodeClassRecord,
		Aliases:     []string{"resolved_records.json"},
		SourcePaths: []string{"records.csv", "lookup_a.csv"},
		JSONShape:   "array(len=20)",
		Fields:      []string{"item_id", "category_raw", "lookup_id"},
	}, {
		ID:          "entity_mappings_category_raw.json",
		Kind:        string(dataquery.DataActionNormalizeEntities),
		NodeClass:   dataworkflow.ArtifactNodeClassArtifact,
		Aliases:     []string{"entity_mappings_category_raw.json"},
		SourcePaths: []string{"lookup_b.csv", "resolved_records.json"},
		JSONShape:   "array(len=8)",
		Fields:      []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
	}}
	action := dataquery.DataAction{
		ID:         "apply_category",
		Kind:       dataquery.DataActionApplyResolutions,
		InputPaths: []string{"resolved_records.json", "entity_mappings_category_raw.json"},
		Params: map[string]string{
			"base_path": "resolved_records.json",
			"resolution_specs": `[{
				"resolution_path":"entity_mappings_category_raw.json",
				"source_field":"category_raw",
				"resolution_key_fields":["item_id"],
				"target_id_field":"category_code",
				"target_label_field":"category_label"
			}]`,
		},
	}
	if guard := dataTaskApplyResolutionActionFieldContractGuardResult(access, action, 0); !guard.Empty() {
		t.Fatalf("guard=%+v, want later source-path lineage to be compatible", guard)
	}
}

func TestDataTaskApplyResolutionGuardPrefersSourceRecordLineageRole(t *testing.T) {
	access := []dataTaskArtifactAccessPrompt{{
		ID:          "resolved_records.json",
		Kind:        string(dataquery.DataActionApplyResolutions),
		NodeClass:   dataworkflow.ArtifactNodeClassRecord,
		Aliases:     []string{"resolved_records.json"},
		SourcePaths: []string{"records.csv", "lookup_a.csv"},
		JSONShape:   "array(len=20)",
		Fields:      []string{"item_id", "category_raw", "lookup_id"},
	}, {
		ID:                "entity_mappings_category_raw.json",
		Kind:              string(dataquery.DataActionNormalizeEntities),
		NodeClass:         dataworkflow.ArtifactNodeClassArtifact,
		Aliases:           []string{"entity_mappings_category_raw.json"},
		SourcePaths:       []string{"lookup_b.csv", "unrelated_reference.csv", "resolved_records.json"},
		SourceRecordPaths: []string{"resolved_records.json"},
		ReferencePaths:    []string{"lookup_b.csv"},
		JSONShape:         "array(len=8)",
		Fields:            []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
	}}
	action := dataquery.DataAction{
		ID:         "apply_category",
		Kind:       dataquery.DataActionApplyResolutions,
		InputPaths: []string{"resolved_records.json", "entity_mappings_category_raw.json"},
		Params: map[string]string{
			"base_path": "resolved_records.json",
			"resolution_specs": `[{
				"resolution_path":"entity_mappings_category_raw.json",
				"source_field":"category_raw",
				"resolution_key_fields":["item_id"],
				"target_id_field":"category_code"
			}]`,
		},
	}
	if guard := dataTaskApplyResolutionActionFieldContractGuardResult(access, action, 0); !guard.Empty() {
		t.Fatalf("guard=%+v, want source_record_paths role to be compatible", guard)
	}
	access[1].SourceRecordPaths = []string{"other_records.json"}
	if guard := dataTaskApplyResolutionActionFieldContractGuardResult(access, action, 0); guard.Empty() {
		t.Fatalf("guard empty, want source_record_paths role mismatch to reject even when mixed source_paths contains base")
	}
}

func TestDataTaskApplyResolutionGuardRejectsMappingWithNoBaseSourceLineage(t *testing.T) {
	access := []dataTaskArtifactAccessPrompt{{
		ID:          "resolved_records.json",
		Kind:        string(dataquery.DataActionApplyResolutions),
		NodeClass:   dataworkflow.ArtifactNodeClassRecord,
		Aliases:     []string{"resolved_records.json"},
		SourcePaths: []string{"records.csv", "lookup_a.csv"},
		JSONShape:   "array(len=20)",
		Fields:      []string{"item_id", "category_raw", "lookup_id"},
	}, {
		ID:          "entity_mappings_category_raw.json",
		Kind:        string(dataquery.DataActionNormalizeEntities),
		NodeClass:   dataworkflow.ArtifactNodeClassArtifact,
		Aliases:     []string{"entity_mappings_category_raw.json"},
		SourcePaths: []string{"lookup_b.csv", "other_records.json"},
		JSONShape:   "array(len=8)",
		Fields:      []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
	}}
	action := dataquery.DataAction{
		ID:         "apply_category",
		Kind:       dataquery.DataActionApplyResolutions,
		InputPaths: []string{"resolved_records.json", "entity_mappings_category_raw.json"},
		Params: map[string]string{
			"base_path": "resolved_records.json",
			"resolution_specs": `[{
				"resolution_path":"entity_mappings_category_raw.json",
				"source_field":"category_raw",
				"resolution_key_fields":["item_id"],
				"target_id_field":"category_code",
				"target_label_field":"category_label"
			}]`,
		},
	}
	guard := dataTaskApplyResolutionActionFieldContractGuardResult(access, action, 0)
	if guard.Empty() {
		t.Fatal("guard empty, want incompatible source-lineage rejection")
	}
	if guard.Code != "apply_resolution_lineage_contract" {
		t.Fatalf("guard code=%q, want apply_resolution_lineage_contract", guard.Code)
	}
}

func TestDataTaskDeterministicContinuationFallbackMaterializesRecords(t *testing.T) {
	current := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "vendors.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUsePlannerDistilled},
			},
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
	}
	records := []dataTaskWorkflowRecord{{
		Plan: current,
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "vendors.csv"},
			RuleCoverage: []dataquery.RuleCoverageRecord{{
				RuleID:   dataquery.LooseText("R1"),
				RuleText: dataquery.LooseText("include eligible rows"),
				Status:   dataquery.LooseText("derived"),
			}},
		},
	}}
	plan, reason, ok := dataTaskDeterministicContinuationFallback(records, current, errors.New("data task planner returned no tool_call"))
	if !ok {
		t.Fatalf("fallback ok=false")
	}
	if !strings.Contains(reason, "record materialization") {
		t.Fatalf("reason=%q, want record materialization", reason)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionExtractRecords {
		t.Fatalf("actions=%+v, want extract_records", plan.Actions)
	}
	if strings.Join(plan.Actions[0].InputPaths, ",") != "orders.csv,vendors.csv" {
		t.Fatalf("input_paths=%v, want executable data materials only", plan.Actions[0].InputPaths)
	}
	if plan.Actions[0].Params["limit"] == "" {
		t.Fatalf("params=%+v, want bounded full-record limit", plan.Actions[0].Params)
	}
}

func TestDataTaskWorkflowNextStageFallbackReconcilesContributions(t *testing.T) {
	current := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{{
				Path:      "orders.csv",
				Required:  true,
				UsageMode: dataquery.MaterialUseScriptConsumed,
			}},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
	}
	records := []dataTaskWorkflowRecord{{
		Plan: current,
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv"},
			Contributions: []dataquery.ContributionRecord{{
				ItemID:    dataquery.LooseText("row-1"),
				GroupKey:  dataquery.LooseText("Q001"),
				Metric:    dataquery.LooseText("total"),
				Value:     dataquery.LooseText("17"),
				Operation: dataquery.LooseText("add"),
			}},
		},
	}}
	plan, reason, ok := dataTaskWorkflowNextStageFallback(records, current, "batch result completed")
	if !ok {
		t.Fatalf("fallback ok=false")
	}
	if !strings.Contains(reason, "reconcile") {
		t.Fatalf("reason=%q, want reconcile", reason)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionReconcile {
		t.Fatalf("actions=%+v, want reconcile action", plan.Actions)
	}
	if !plan.ContinueAfter {
		t.Fatalf("ContinueAfter=false, want true")
	}
}

func TestDataTaskWorkflowNextStageFallbackNormalizesFromRecordArtifacts(t *testing.T) {
	current := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "vendors.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			EntityResolutionRequired:   true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
	}
	records := []dataTaskWorkflowRecord{{
		Plan: current,
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "vendors.csv"},
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "order_records.json",
					Kind:    string(dataquery.DataActionExtractRecords),
					Headers: []string{"order_id", "vendor_name_raw", "vendor_id", "amount"},
					Fields: map[string]string{
						"artifact_aliases": "order_records.json,order_records",
						"json_shape":       "array(len=2,item=object(keys=order_id,vendor_name_raw,vendor_id,amount))",
					},
				},
				{
					ID:      "vendor_records.json",
					Kind:    string(dataquery.DataActionExtractRecords),
					Headers: []string{"vendor_id", "legal_name", "brand_name", "status"},
					Fields: map[string]string{
						"artifact_aliases": "vendor_records.json,vendor_records",
						"json_shape":       "array(len=3,item=object(keys=vendor_id,legal_name,brand_name,status))",
					},
				},
			},
		},
	}}
	plan, reason, ok := dataTaskWorkflowNextStageFallback(records, current, "batch result completed")
	if !ok {
		t.Fatalf("fallback ok=false")
	}
	if !strings.Contains(reason, "normalize_or_enrich_entities") {
		t.Fatalf("reason=%q, want normalize stage reason", reason)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionNormalizeEntities {
		t.Fatalf("actions=%+v, want normalize_entities action", plan.Actions)
	}
	action := plan.Actions[0]
	if len(action.InputPaths) != 2 || action.InputPaths[0] != "order_records.json" || action.InputPaths[1] != "vendor_records.json" {
		t.Fatalf("input_paths=%v, want order/vendor artifacts", action.InputPaths)
	}
	if action.Params["canonical_id_field"] != "vendor_id" {
		t.Fatalf("params=%+v, want vendor_id canonical field", action.Params)
	}
	if action.Params["source_field"] == "" && action.Params["source_fields"] == "" {
		t.Fatalf("params=%+v, want concrete source field contract", action.Params)
	}
	if !plan.ContinueAfter {
		t.Fatalf("ContinueAfter=false, want true")
	}
}

func TestDataTaskWorkflowNextStageFallbackDoesNotJoinOnInternalLineageOnly(t *testing.T) {
	current := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "left.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "right.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
	}
	records := []dataTaskWorkflowRecord{{
		Plan: current,
		Result: &dataquery.Result{
			ConsumedPaths: []string{"left.csv", "right.csv"},
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "left_joined.json",
					Kind:    string(dataquery.DataActionJoinRecords),
					Headers: []string{"_source", "_left_index", "left_value"},
					Fields: map[string]string{
						"artifact_aliases": "left_joined.json,left_joined",
						"json_shape":       "array(len=2,item=object(keys=_source,_left_index,left_value))",
						"output_headers":   "_source,_left_index,left_value",
					},
				},
				{
					ID:      "right_joined.json",
					Kind:    string(dataquery.DataActionJoinRecords),
					Headers: []string{"_source", "_left_index", "right_value"},
					Fields: map[string]string{
						"artifact_aliases": "right_joined.json,right_joined",
						"json_shape":       "array(len=2,item=object(keys=_source,_left_index,right_value))",
						"output_headers":   "_source,_left_index,right_value",
					},
				},
			},
		},
	}}

	if plan, reason, ok := dataTaskWorkflowNextStageFallback(records, current, "batch result completed"); ok {
		if len(plan.Actions) == 0 {
			t.Fatalf("fallback=%+v reason=%q, want concrete diagnostic action when fallback exists", plan, reason)
		}
		action := plan.Actions[0]
		if action.Kind == dataquery.DataActionJoinRecords {
			t.Fatalf("fallback=%+v reason=%q, want no deterministic join fallback on internal lineage fields", plan, reason)
		}
		if action.Kind == dataquery.DataActionValueDistribution {
			if strings.Contains(action.Params["fields"], "<") || strings.Contains(action.Params["fields"], "_source") || strings.Contains(action.Params["fields"], "_left_index") {
				t.Fatalf("value_distribution params=%+v, want concrete non-internal fields", action.Params)
			}
		}
	}
}

func TestDataTaskWorkflowNextStageFallbackStopsRepeatedJoinNoProgress(t *testing.T) {
	current := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "items.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "lookup.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
	}
	joinPlan := func(id, left, right, out string) dataquery.TaskPlan {
		return dataquery.TaskPlan{
			Status:        "ready",
			ContinueAfter: true,
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials:          current.CoverageContract.RequiredMaterials,
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
			Actions: []dataquery.DataAction{{
				ID:             id,
				Kind:           dataquery.DataActionJoinRecords,
				InputPaths:     []string{left, right},
				OutputArtifact: out,
				Params: map[string]string{
					"left_fields":  `["shared_key"]`,
					"right_fields": `["shared_key"]`,
					"join_type":    "inner",
				},
			}},
		}
	}
	joinResult := func(id string) *dataquery.Result {
		return &dataquery.Result{
			ConsumedPaths: []string{"items.csv", "lookup.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:      id,
				Kind:    string(dataquery.DataActionJoinRecords),
				Headers: []string{"shared_key", "value", "label"},
				Fields: map[string]string{
					"artifact_aliases": id,
					"json_shape":       "array(len=2,item=object(keys=shared_key,value,label))",
					"output_headers":   "shared_key,value,label",
				},
			}},
		}
	}
	records := []dataTaskWorkflowRecord{
		{Plan: joinPlan("join_1", "items.csv", "lookup.csv", "joined_1.json"), Result: joinResult("joined_1.json")},
		{Plan: joinPlan("join_2", "joined_1.json", "lookup.csv", "joined_2.json"), Result: joinResult("joined_2.json")},
	}

	state := dataTaskWorkflowState(records, current)
	found := false
	for _, violation := range state.WorkflowViolations {
		if violation.Code == "stage_no_progress" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("WorkflowViolations=%+v, want stage_no_progress", state.WorkflowViolations)
	}
	if plan, reason, ok := dataTaskWorkflowNextStageFallback(records, current, "batch result completed"); ok {
		t.Fatalf("fallback=%+v reason=%q, want no automatic join after repeated no-progress joins", plan, reason)
	}
}

func TestDataTaskWorkflowNextStageFallbackStopsRepeatedRelationNoProgress(t *testing.T) {
	current := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "items.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "lookup.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
	}
	applyPlan := func(id, base, mapping, out string) dataquery.TaskPlan {
		return dataquery.TaskPlan{
			Status:        "ready",
			ContinueAfter: true,
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials:          current.CoverageContract.RequiredMaterials,
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
			Actions: []dataquery.DataAction{{
				ID:             id,
				Kind:           dataquery.DataActionApplyResolutions,
				InputPaths:     []string{base, mapping},
				OutputArtifact: out,
				Params: map[string]string{
					"base_path": base,
					"resolution_specs": `[{
						"resolution_path":"` + mapping + `",
						"resolution_key_fields":["item_id"],
						"target_id_field":"canonical_id"
					}]`,
				},
			}},
		}
	}
	applyResult := func(id string) *dataquery.Result {
		return &dataquery.Result{
			ConsumedPaths: []string{"items.csv", "lookup.csv"},
			EntityResolutions: []dataquery.EntityResolutionRecord{{
				SourceValue:    dataquery.LooseText("a"),
				CanonicalID:    dataquery.LooseText("A"),
				CanonicalLabel: dataquery.LooseText("A"),
				Status:         dataquery.LooseText("matched"),
			}},
			Artifacts: []dataquery.DataArtifact{{
				ID:      id,
				Kind:    string(dataquery.DataActionApplyResolutions),
				Headers: []string{"item_id", "canonical_id", "value"},
				Fields: map[string]string{
					"artifact_aliases": id,
					"json_shape":       "array(len=2,item=object(keys=item_id,canonical_id,value))",
					"output_headers":   "item_id,canonical_id,value",
				},
			}},
		}
	}
	records := []dataTaskWorkflowRecord{
		{Plan: applyPlan("apply_1", "items.csv", "mapping.json", "resolved_1.json"), Result: applyResult("resolved_1.json")},
		{Plan: applyPlan("apply_2", "resolved_1.json", "mapping.json", "resolved_2.json"), Result: applyResult("resolved_2.json")},
	}

	state := dataTaskWorkflowState(records, current)
	var found *dataworkflow.WorkflowViolation
	for i := range state.WorkflowViolations {
		if state.WorkflowViolations[i].Code == "stage_no_progress" {
			found = &state.WorkflowViolations[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("WorkflowViolations=%+v, want stage_no_progress", state.WorkflowViolations)
	}
	if found.ActionKind != string(dataquery.DataActionApplyResolutions) {
		t.Fatalf("stage_no_progress ActionKind=%q, want apply_entity_resolutions", found.ActionKind)
	}
	if plan, reason, ok := dataTaskWorkflowNextStageFallback(records, current, "batch result completed"); ok && len(plan.Actions) > 0 && dataTaskActionKindIsRelationMaterialization(plan.Actions[0].Kind) {
		t.Fatalf("fallback=%+v reason=%q, want no automatic relation materialization after repeated no-progress apply", plan, reason)
	}
}

func TestDataTaskActionStagingGuardRejectsEmptyCustomTransform(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:   "empty_transform",
			Kind: dataquery.DataActionCustomTransform,
		}},
	}
	errText := dataTaskActionStagingGuardError(plan)
	if !strings.Contains(errText, "custom_transform") || !strings.Contains(errText, "no script") {
		t.Fatalf("errText=%q, want empty custom_transform guard", errText)
	}
	errText = dataTaskWorkflowActionStagingGuardError(nil, plan)
	if !strings.Contains(errText, "custom_transform") || !strings.Contains(errText, "no script") {
		t.Fatalf("workflow errText=%q, want empty custom_transform guard", errText)
	}
}

func TestNormalizeDataTaskPlanShapeRemovesTopLevelScriptInsteadOfMovingIntoCustomAction(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv"},
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		Actions: []dataquery.DataAction{
			{ID: "inspect", Kind: dataquery.DataActionInspectMaterial, InputPaths: []string{"orders.csv"}},
			{ID: "compute", Kind: dataquery.DataActionCustomTransform},
		},
		Script: `rows = csv_rows("orders.csv")
emit_result(str(len(rows)), output_contract={"format":"plain_single_line","explanation_allowed":False})`,
	}
	got, notes := normalizeDataTaskPlanShape(plan)
	if strings.TrimSpace(got.Script) != "" {
		t.Fatalf("top-level script not cleared: %q", got.Script)
	}
	if strings.TrimSpace(got.Actions[1].Script) != "" {
		t.Fatalf("custom_transform script populated from top-level script: %+v", got.Actions[1])
	}
	if !strings.Contains(strings.Join(notes, "\n"), "removed stray top-level script") {
		t.Fatalf("notes=%v, want stray script removal", notes)
	}
}

func TestNormalizeDataTaskPlanShapeForPolicyAddsLedgersOnlyForComplexAggregation(t *testing.T) {
	policy := TurnPolicy{
		Route:           RouteData,
		NeedsDataAccess: true,
		DataTaskKind:    "data_aggregation",
		Operation:       "data_aggregation",
	}
	complex := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv", "queries.csv"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{{Path: "orders.csv", Required: true}, {Path: "queries.csv", Required: true}},
		},
		Script: `emit_result("0")`,
	}
	got, notes := normalizeDataTaskPlanShapeForPolicy(complex, policy)
	if !got.CoverageContract.ContributionLedgerRequired || !got.CoverageContract.ReconcileRequired {
		t.Fatalf("coverage contract=%+v, want contribution and reconcile required", got.CoverageContract)
	}
	if !strings.Contains(strings.Join(notes, "; "), "complex typed data aggregation") {
		t.Fatalf("notes=%v, want typed aggregation normalization note", notes)
	}

	simple := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv"},
		Script:     `emit_result("17")`,
	}
	got, notes = normalizeDataTaskPlanShapeForPolicy(simple, policy)
	if got.CoverageContract.ContributionLedgerRequired || got.CoverageContract.ReconcileRequired {
		t.Fatalf("simple coverage contract=%+v, want no forced ledgers", got.CoverageContract)
	}
	if strings.Contains(strings.Join(notes, "; "), "complex typed data aggregation") {
		t.Fatalf("notes=%v, should not contain complex aggregation normalization", notes)
	}
}

func TestNormalizeDataTaskPlanShapeForPolicyAddsLedgersForComplexStrictOutput(t *testing.T) {
	policy := TurnPolicy{
		Route:           RouteData,
		NeedsDataAccess: true,
		DataTaskKind:    "data_task",
		Operation:       "data_task",
	}
	complex := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv"},
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		Script: strings.Repeat("x = 1\n", 45) + `emit_result("1")`,
	}
	got, notes := normalizeDataTaskPlanShapeForPolicy(complex, policy)
	if !got.CoverageContract.DecisionRecordsRequired || !got.CoverageContract.ContributionLedgerRequired || !got.CoverageContract.ReconcileRequired {
		t.Fatalf("coverage contract=%+v, want strict-output ledgers", got.CoverageContract)
	}
	if !strings.Contains(strings.Join(notes, "; "), "complex strict-output data plan") {
		t.Fatalf("notes=%v, want strict-output normalization note", notes)
	}

	simple := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"a.csv"},
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		Script: `emit_result("1")`,
	}
	got, _ = normalizeDataTaskPlanShapeForPolicy(simple, policy)
	if got.CoverageContract.DecisionRecordsRequired || got.CoverageContract.ContributionLedgerRequired || got.CoverageContract.ReconcileRequired {
		t.Fatalf("simple coverage contract=%+v, want no strict-output ledgers", got.CoverageContract)
	}
}

func TestNormalizeDataTaskPlanShapeRemovesTopLevelScriptFromActionsPlan(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"rules.md", "orders.csv", "queries.csv"},
		Actions: []dataquery.DataAction{{
			ID:         "transform",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"rules.md"},
		}},
		Script: `rows = csv_rows("orders.csv")
emit_result(str(len(rows)), output_contract={"format":"plain_single_line","explanation_allowed":False})`,
	}
	got, notes := normalizeDataTaskPlanShape(plan)
	if strings.TrimSpace(got.Script) != "" {
		t.Fatalf("Script=%q, want cleared top-level script", got.Script)
	}
	if strings.Join(got.Actions[0].InputPaths, ",") != "rules.md" {
		t.Fatalf("custom_transform input paths=%v, want action-owned inputs preserved", got.Actions[0].InputPaths)
	}
	if !strings.Contains(strings.Join(notes, "\n"), "removed stray top-level script") {
		t.Fatalf("notes=%v, want stray script removal", notes)
	}
}

func TestNormalizeDataTaskPlanShapeRemovesDuplicateTopLevelScript(t *testing.T) {
	actionScript := `content = read_text('rules.md')
emit({'content': content, 'line_count': len(content.splitlines())})`
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "read_rules",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"rules.md"},
			Script:     actionScript,
		}},
		Script: "# Read the material\n" + actionScript,
	}
	got, notes := normalizeDataTaskPlanShape(plan)
	if strings.TrimSpace(got.Script) != "" {
		t.Fatalf("top-level script not cleared: %q", got.Script)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "duplicate top-level script") {
		t.Fatalf("notes=%v, want duplicate top-level script normalization", notes)
	}
	if got.Actions[0].Script != actionScript {
		t.Fatalf("action script changed: %q", got.Actions[0].Script)
	}
}

func TestNormalizeDataTaskPlanShapeDropsStrayTopLevelScriptFromActionsPlan(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv", "queries.csv"},
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "compute",
			Kind:       dataquery.DataActionComputeContribs,
			InputPaths: []string{"orders.csv"},
			Params: map[string]string{
				"group_key_field": "query_id",
				"value_field":     "amount",
				"operation":       "add",
			},
		}},
		Script: `rows = csv_rows("orders.csv")
emit_result("17", output_contract={"format":"plain_single_line","explanation_allowed":False})`,
	}
	got, notes := normalizeDataTaskPlanShape(plan)
	if strings.TrimSpace(got.Script) != "" {
		t.Fatalf("top-level script not cleared: %q", got.Script)
	}
	if len(got.Actions) != 1 || got.Actions[0].Kind != dataquery.DataActionComputeContribs {
		t.Fatalf("actions=%+v, want original typed action preserved", got.Actions)
	}
	if strings.Contains(strings.Join(notes, "\n"), "appended bounded top-level script") {
		t.Fatalf("notes=%v, should not append a stray script to a single-action plan", notes)
	}
	if !strings.Contains(strings.Join(notes, "\n"), "removed stray top-level script") {
		t.Fatalf("notes=%v, want stray script removal note", notes)
	}
}

func TestNormalizeDataTaskPlanShapeExecutesCompleteStatusWithPayload(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "complete",
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		Script: `emit_result("32", output_contract={"format":"plain_single_line","explanation_allowed":False})`,
	}
	got, notes := normalizeDataTaskPlanShape(plan)
	if got.Status != "ready" {
		t.Fatalf("Status=%q, want ready", got.Status)
	}
	if strings.TrimSpace(got.Script) == "" && len(got.Actions) == 0 {
		t.Fatalf("normalized plan lost executable payload: %+v", got)
	}
	if !strings.Contains(strings.Join(notes, "\n"), "status=complete to ready") {
		t.Fatalf("notes=%v, want complete status repair note", notes)
	}
}

func TestNormalizeDataTaskPlanShapeWrapsBoundedComplexTopLevelScript(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv", "rules.md", "queries.csv", "lookup.csv"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true},
				{Path: "rules.md", Required: true},
				{Path: "queries.csv", Required: true},
				{Path: "lookup.csv", Required: true},
			},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Script: `rows = csv_rows("orders.csv")
emit_result("ok", output_contract={"format":"plain_single_line","explanation_allowed":False})`,
	}
	got, notes := normalizeDataTaskPlanShape(plan)
	if len(notes) != 1 || !strings.Contains(notes[0], "wrapped bounded top-level script") {
		t.Fatalf("notes=%v", notes)
	}
	if strings.TrimSpace(got.Script) != "" {
		t.Fatalf("top-level script not cleared: %q", got.Script)
	}
	if len(got.Actions) != 1 || got.Actions[0].Kind != dataquery.DataActionCustomTransform {
		t.Fatalf("actions=%+v", got.Actions)
	}
	if got.Actions[0].Script != plan.Script {
		t.Fatalf("action script mismatch")
	}
	if strings.Join(got.Actions[0].InputPaths, ",") != "orders.csv,rules.md,queries.csv,lookup.csv" {
		t.Fatalf("custom_transform input paths=%v", got.Actions[0].InputPaths)
	}
}

func TestNormalizeDataTaskPlanShapeRemovesTopLevelScriptAfterTypedActions(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv", "rules.md"},
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true},
				{Path: "rules.md", Required: true},
			},
			DecisionRecordsRequired:    true,
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{
			{ID: "inspect", Kind: dataquery.DataActionInspectMaterial, InputPaths: []string{"orders.csv", "rules.md"}},
			{ID: "rules", Kind: dataquery.DataActionDeriveRules, InputPaths: []string{"rules.md"}},
		},
		Script: `rows = csv_rows("orders.csv")
emit_result("ok", output_contract={"format":"plain_single_line","explanation_allowed":False})`,
		WhyThisBatch: "final bounded transform",
	}
	got, notes := normalizeDataTaskPlanShape(plan)
	if strings.TrimSpace(got.Script) != "" {
		t.Fatalf("top-level script not cleared: %q", got.Script)
	}
	if len(got.Actions) != 2 {
		t.Fatalf("Actions=%+v, want original typed actions only", got.Actions)
	}
	if !strings.Contains(strings.Join(notes, "\n"), "removed stray top-level script") {
		t.Fatalf("notes=%v, want stray script removal", notes)
	}
}

func TestNormalizeDataTaskPlanShapeRemovesTopLevelScriptWhenCustomActionsAlreadyHaveScripts(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv", "rules.md"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true},
				{Path: "rules.md", Required: true},
			},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{
			{ID: "inspect", Kind: dataquery.DataActionInspectMaterial, InputPaths: []string{"orders.csv", "rules.md"}},
			{ID: "rules", Kind: dataquery.DataActionDeriveRules, InputPaths: []string{"rules.md"}},
			{
				ID:         "load",
				Kind:       dataquery.DataActionCustomTransform,
				InputPaths: []string{"orders.csv"},
				Script:     `emit_result("partial", output_contract={"format":"freeform","explanation_allowed":True})`,
			},
		},
		Script: `rows = csv_rows("orders.csv")
emit_result("ok", output_contract={"format":"plain_single_line","explanation_allowed":False})`,
	}
	got, notes := normalizeDataTaskPlanShape(plan)
	if got.Script != "" {
		t.Fatalf("top-level script not cleared: %q", got.Script)
	}
	if len(got.Actions) != 3 {
		t.Fatalf("Actions=%+v, want original actions only", got.Actions)
	}
	if !strings.Contains(strings.Join(notes, "\n"), "removed stray top-level script") {
		t.Fatalf("notes=%v, want stray script removal", notes)
	}
}

func TestNormalizeDataTaskPlanShapeRemovesTopLevelScriptAfterOneTypedAndExistingCustomScript(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv", "rules.md"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true},
				{Path: "rules.md", Required: true},
			},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{
			{ID: "rules", Kind: dataquery.DataActionDeriveRules, InputPaths: []string{"rules.md"}},
			{
				ID:         "transform",
				Kind:       dataquery.DataActionCustomTransform,
				InputPaths: []string{"orders.csv"},
				Script:     `emit_result("partial", output_contract={"format":"freeform","explanation_allowed":True})`,
			},
		},
		Script: `rows = csv_rows("orders.csv")
emit_result("ok", output_contract={"format":"plain_single_line","explanation_allowed":False})`,
	}
	got, notes := normalizeDataTaskPlanShape(plan)
	if got.Script != "" {
		t.Fatalf("top-level script not cleared: %q", got.Script)
	}
	if len(got.Actions) != 2 {
		t.Fatalf("Actions=%+v, want original actions only", got.Actions)
	}
	if !strings.Contains(strings.Join(notes, "\n"), "removed stray top-level script") {
		t.Fatalf("notes=%v, want stray script removal", notes)
	}
}

func TestNormalizeDataTaskPlanShapeTrimsOversizedActionBatch(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{
			{ID: "a1", Kind: dataquery.DataActionInspectMaterial, InputPaths: []string{"a.csv"}},
			{ID: "a2", Kind: dataquery.DataActionInspectMaterial, InputPaths: []string{"b.csv"}},
			{ID: "a3", Kind: dataquery.DataActionDeriveRules, InputPaths: []string{"rules.md"}},
			{ID: "a4", Kind: dataquery.DataActionNormalizeEntities, InputPaths: []string{"vendors.csv"}, OutputArtifact: "vendors.json"},
			{ID: "a5", Kind: dataquery.DataActionExtractRecords, InputPaths: []string{"queries.csv"}},
			{ID: "final", Kind: dataquery.DataActionCustomTransform, InputPaths: []string{"a.csv"}, Script: `emit_result("ok")`},
		},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "a.csv", Required: true},
				{Path: "b.csv", Required: true},
				{Path: "rules.md", Required: true},
			},
			RuleCoverageRequired: true,
		},
	}
	got, notes := normalizeDataTaskPlanShape(plan)
	if !strings.Contains(strings.Join(notes, "\n"), "trimmed oversized actions") {
		t.Fatalf("notes=%v", notes)
	}
	if len(got.Actions) != dataTaskMaxActionsPerBatch {
		t.Fatalf("len(Actions)=%d, want %d", len(got.Actions), dataTaskMaxActionsPerBatch)
	}
	if !got.ContinueAfter {
		t.Fatal("ContinueAfter=false, want true after deterministic batch trim")
	}
	if got.Actions[len(got.Actions)-1].ID != "a4" {
		t.Fatalf("last kept action=%q, want a4", got.Actions[len(got.Actions)-1].ID)
	}
	if len(got.CoverageContract.RequiredMaterials) != len(plan.CoverageContract.RequiredMaterials) {
		t.Fatalf("coverage contract changed: %+v", got.CoverageContract.RequiredMaterials)
	}
}

func TestWorkflowCoveredMaterialPathsIncludesEarlierActionOutputArtifacts(t *testing.T) {
	plan := dataquery.TaskPlan{
		Actions: []dataquery.DataAction{
			{
				ID:             "normalize_vendors",
				Kind:           dataquery.DataActionNormalizeEntities,
				InputPaths:     []string{"vendors.csv"},
				OutputArtifact: "vendor_resolutions.json",
			},
			{
				ID:         "final",
				Kind:       dataquery.DataActionCustomTransform,
				InputPaths: []string{"vendor_resolutions.json"},
				Script:     `emit_result("ok")`,
			},
		},
	}
	missing := dataTaskMissingCustomTransformPrerequisites(nil, plan, plan.Actions[1], 1)
	if len(missing) != 0 {
		t.Fatalf("missing=%v, want same-batch output_artifact covered", missing)
	}
}

func TestPreserveDataTaskCoverageForTerminalMissingMaterial(t *testing.T) {
	previous := dataquery.TaskPlan{
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
		},
	}
	repaired := dataquery.TaskPlan{
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
		},
	}
	errText := "data planning incomplete: terminal batch declares 1 required material(s) that are not scheduled for script/typed-action consumption: rules.md"
	got := preserveDataTaskMaterialRepairCoverageForError(previous, repaired, errText)
	var paths []string
	for _, material := range got.CoverageContract.RequiredMaterials {
		paths = append(paths, material.Path)
	}
	if strings.Join(paths, ",") != "rules.md,orders.csv" && strings.Join(paths, ",") != "orders.csv,rules.md" {
		t.Fatalf("required paths=%v, want orders.csv and rules.md preserved", paths)
	}
}

func TestDataTaskWorkflowCompletionGateRequiresCumulativeCoverage(t *testing.T) {
	records := []dataTaskWorkflowRecord{
		{
			Plan: dataquery.TaskPlan{
				CoverageContract: dataquery.CoverageContract{
					RequiredMaterials: []dataquery.CoverageMaterial{
						{Path: "a.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
						{Path: "b.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
					},
				},
			},
		},
	}
	current := dataquery.TaskPlan{
		ContinueAfter: true,
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "a.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
		},
	}
	result := dataquery.Result{
		Answer:         "10",
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
		ConsumedPaths:  []string{"a.csv"},
	}
	errText := dataTaskWorkflowCompletionGateError(records, current, result)
	if errText == "" || !strings.Contains(errText, "b.csv") {
		t.Fatalf("completion gate err=%q, want cumulative missing material", errText)
	}
	result.ConsumedPaths = []string{"a.csv", "b.csv"}
	if errText := dataTaskWorkflowCompletionGateError(records, current, result); errText != "" {
		t.Fatalf("completion gate err=%q, want pass after cumulative coverage", errText)
	}
}

func TestDataTaskWorkflowCompletionGateRequiresOutputProjection(t *testing.T) {
	current := dataquery.TaskPlan{
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false, Delimiter: ","},
		CoverageContract: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
	}
	result := dataquery.Result{
		OutputContract: current.OutputContract,
		Contributions: []dataquery.ContributionRecord{
			{
				ItemID:        dataquery.LooseText("row-1"),
				Source:        dataquery.LooseText("records.csv"),
				SourceLocator: dataquery.LooseText("line:2"),
				GroupKey:      dataquery.LooseText("Q1"),
				Metric:        dataquery.LooseText("amount"),
				Value:         dataquery.LooseText("10"),
				Operation:     dataquery.LooseText("add"),
				Role:          dataquery.LooseText("target"),
			},
			{
				ItemID:        dataquery.LooseText("row-2"),
				Source:        dataquery.LooseText("records.csv"),
				SourceLocator: dataquery.LooseText("line:3"),
				GroupKey:      dataquery.LooseText("Q2"),
				Metric:        dataquery.LooseText("amount"),
				Value:         dataquery.LooseText("20"),
				Operation:     dataquery.LooseText("add"),
				Role:          dataquery.LooseText("target"),
			},
		},
		Reconcile: &dataquery.ReconcileReport{
			Status: dataquery.LooseText("pass"),
			Groups: []dataquery.ReconcileGroup{
				{GroupKey: dataquery.LooseText("Q1"), Metric: dataquery.LooseText("amount"), Expected: dataquery.LooseText("10"), Actual: dataquery.LooseText("10"), Difference: dataquery.LooseText("0")},
				{GroupKey: dataquery.LooseText("Q2"), Metric: dataquery.LooseText("amount"), Expected: dataquery.LooseText("20"), Actual: dataquery.LooseText("20"), Difference: dataquery.LooseText("0")},
			},
		},
	}
	errText := dataTaskWorkflowCompletionGateError(nil, current, result)
	if errText == "" || !strings.Contains(errText, "final answer projection") {
		t.Fatalf("completion gate err=%q, want missing projection", errText)
	}
	plan, ok := dataTaskRequiredLedgerCompletionPlan(nil, current, result, errText)
	if !ok {
		t.Fatal("expected deterministic output projection plan")
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionAssembleAnswer {
		t.Fatalf("actions=%+v, want one assemble_answer action", plan.Actions)
	}
	if got := plan.OutputContract.Normalize().Format; got != dataquery.OutputPlainSingleLine {
		t.Fatalf("output format=%q, want plain_single_line", got)
	}
	if plan.ContinueAfter {
		t.Fatal("output projection should be terminal by default")
	}
	if got := plan.Actions[0].Params["value_field"]; got != "actual" {
		t.Fatalf("value_field=%q, want actual", got)
	}

	result.Answer = "Q1/amount=10; Q2/amount=20"
	errText = dataTaskWorkflowCompletionGateError(nil, current, result)
	if errText == "" || !strings.Contains(errText, "final answer projection") {
		t.Fatalf("completion gate err=%q, want projection for reconcile-rendered intermediate answer", errText)
	}

	result.Artifacts = append(result.Artifacts, dataquery.DataArtifact{ID: "final_answer", Kind: string(dataquery.DataActionAssembleAnswer)})
	errText = dataTaskWorkflowCompletionGateError(nil, current, result)
	if errText != "" {
		t.Fatalf("completion gate err=%q, want assembled answer accepted", errText)
	}
}

func TestDataTaskWorkflowCompletionGateRequiresReferenceCompleteProjection(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "targets.csv"), []byte("target\nA\nB\nC\n"), 0600); err != nil {
		t.Fatal(err)
	}
	current := dataquery.TaskPlan{
		InputPaths: []string{"targets.csv"},
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
			Delimiter:          ",",
		},
		CoverageContract: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
	}
	result := dataquery.Result{
		Answer:         "10,30",
		OutputContract: current.OutputContract,
		ConsumedPaths:  []string{"targets.csv"},
		Contributions: []dataquery.ContributionRecord{
			{
				ItemID:        dataquery.LooseText("row-1"),
				Source:        dataquery.LooseText("records.csv"),
				SourceLocator: dataquery.LooseText("line:2"),
				GroupKey:      dataquery.LooseText("A"),
				Metric:        dataquery.LooseText("amount"),
				Value:         dataquery.LooseText("10"),
				Operation:     dataquery.LooseText("add"),
				Role:          dataquery.LooseText("target"),
			},
			{
				ItemID:        dataquery.LooseText("row-2"),
				Source:        dataquery.LooseText("records.csv"),
				SourceLocator: dataquery.LooseText("line:3"),
				GroupKey:      dataquery.LooseText("C"),
				Metric:        dataquery.LooseText("amount"),
				Value:         dataquery.LooseText("30"),
				Operation:     dataquery.LooseText("add"),
				Role:          dataquery.LooseText("target"),
			},
		},
		Reconcile: &dataquery.ReconcileReport{
			Status: dataquery.LooseText("pass"),
			Groups: []dataquery.ReconcileGroup{
				{GroupKey: dataquery.LooseText("A"), Metric: dataquery.LooseText("amount"), Expected: dataquery.LooseText("10"), Actual: dataquery.LooseText("10"), Difference: dataquery.LooseText("0")},
				{GroupKey: dataquery.LooseText("C"), Metric: dataquery.LooseText("amount"), Expected: dataquery.LooseText("30"), Actual: dataquery.LooseText("30"), Difference: dataquery.LooseText("0")},
			},
		},
	}
	errText := dataTaskWorkflowCompletionGateErrorWithRepo(root, nil, current, result)
	if errText == "" || !strings.Contains(errText, "reference field") {
		t.Fatalf("completion gate err=%q, want reference universe gap", errText)
	}
	plan, ok := dataTaskRequiredLedgerCompletionPlanWithRepo(root, nil, current, result, errText)
	if !ok {
		t.Fatal("expected deterministic reference-complete projection plan")
	}
	if !plan.OutputContract.CompleteReference {
		t.Fatalf("CompleteReference=false, plan=%+v", plan.OutputContract)
	}
	if plan.OutputContract.ReferencePath != "targets.csv" || plan.OutputContract.ReferenceKeyField != "target" {
		t.Fatalf("OutputContract=%+v, want targets.csv.target reference", plan.OutputContract)
	}
	if got := plan.Actions[0].Params["complete_reference"]; got != "true" {
		t.Fatalf("complete_reference param=%q, want true", got)
	}
}

func TestDataTaskPlannerCompatJSONActions(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			dataTaskPlanResp(`{"status":"ready","output_contract":{"format":"markdown","explanation_allowed":true},"actions":[{"id":"inventory","kind":"material_inventory","purpose":"discover objective materials","params":{"limit":20},"success_criteria":"candidate inventory exists"},{"kind":"inspect_material","inputPaths":"orders.csv, rules.md","outputArtifact":"profiles"}],"continueAfter":"true"}`),
		},
	}
	planner := NewDataTaskPlanner(adapter)
	plan, err := planner.PlanDataTask(context.Background(), "先查看材料结构", "/repo", TurnPolicy{Route: RouteData, DataTaskKind: "data_task"}, []dataquery.CandidateFile{
		{Path: "orders.csv", Kind: "csv", Size: 10},
		{Path: "rules.md", Kind: "text", Size: 10},
	})
	if err != nil {
		t.Fatalf("PlanDataTask: %v", err)
	}
	if len(plan.Actions) != 2 {
		t.Fatalf("Actions=%+v, want 2 actions", plan.Actions)
	}
	if plan.Actions[0].Kind != dataquery.DataActionMaterialInventory || plan.Actions[0].Params["limit"] != "20" {
		t.Fatalf("action[0]=%+v", plan.Actions[0])
	}
	if strings.Join(plan.Actions[1].InputPaths, ",") != "orders.csv,rules.md" || plan.Actions[1].OutputArtifact != "profiles" {
		t.Fatalf("action[1]=%+v", plan.Actions[1])
	}
	if !plan.ContinueAfter {
		t.Fatal("ContinueAfter=false, want true for action workflow")
	}
	system := adapter.calls[0].messages[0].Content
	for _, want := range []string{"actions", "material_inventory", "inspect_material", "extract_records", "derive_rules", "derive_fields", "extract_fields", "group_records", "expand_records", "filter_records", "value_distribution", "normalize_entities", "enrich_records", "join_records", "compute_contributions", "reconcile_artifacts", "assemble_answer", "custom_transform", "adaptive action workflow", "An action is atomic"} {
		if !strings.Contains(system, want) {
			t.Fatalf("data planner system prompt missing %q:\n%s", want, system)
		}
	}
}

func TestDataTaskActionsToPlanPreservesStructuredParams(t *testing.T) {
	actions := dataTaskActionsToPlan([]dataTaskActionDraft{{
		ID:   "derive",
		Kind: "derive_fields",
		Params: map[string]any{
			"field_specs": []any{
				map[string]any{
					"source_field": "created_at",
					"target_field": "year",
					"operation":    "regex_extract",
					"pattern":      `^(\d{4})`,
				},
			},
			"filters": []any{
				map[string]any{"field": "status", "op": "in", "value": []any{"paid", "accepted"}},
			},
			"left_fields": []any{"vendor_id", "category_code"},
			"lookup_specs": []any{
				map[string]any{
					"lookup_path":        "category_mapping",
					"base_fields":        []any{"category_raw"},
					"lookup_fields":      []any{"source_value"},
					"lookup_value_field": "canonical_id",
					"target_field":       "category_code",
				},
			},
		},
	}})
	if len(actions) != 1 {
		t.Fatalf("actions=%+v, want one action", actions)
	}
	params := actions[0].Params
	if strings.Contains(params["field_specs_json"], "map[") || strings.Contains(params["filters_json"], "map[") || strings.Contains(params["left_fields"], " ") && !strings.Contains(params["left_fields"], `"`) {
		t.Fatalf("params=%+v, want JSON-serialized structured params", params)
	}
	var specs []map[string]any
	if err := json.Unmarshal([]byte(params["field_specs_json"]), &specs); err != nil {
		t.Fatalf("field_specs_json=%q unmarshal: %v", params["field_specs_json"], err)
	}
	if got := specs[0]["pattern"]; got != `^(\d{4})` {
		t.Fatalf("pattern=%q, want regex preserved", got)
	}
	var filters []map[string]any
	if err := json.Unmarshal([]byte(params["filters_json"]), &filters); err != nil {
		t.Fatalf("filters_json=%q unmarshal: %v", params["filters_json"], err)
	}
	var leftFields []string
	if err := json.Unmarshal([]byte(params["left_fields"]), &leftFields); err != nil {
		t.Fatalf("left_fields=%q unmarshal: %v", params["left_fields"], err)
	}
	if strings.Join(leftFields, ",") != "vendor_id,category_code" {
		t.Fatalf("left_fields=%v", leftFields)
	}
	var lookupSpecs []map[string]any
	if err := json.Unmarshal([]byte(params["lookup_specs_json"]), &lookupSpecs); err != nil {
		t.Fatalf("lookup_specs_json=%q unmarshal: %v", params["lookup_specs_json"], err)
	}
	if got := lookupSpecs[0]["lookup_value_field"]; got != "canonical_id" {
		t.Fatalf("lookup_value_field=%q, want canonical_id", got)
	}
}

func TestDataTaskPlannerRepairsMalformedToolParams(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			dataTaskPlanResp(`ä not json`),
			dataTaskPlanResp(`{"status":"ready","input_paths":["orders.csv"],"output_contract":{"format":"csv_line","explanation_allowed":false},"goal":"汇总订单","success_criteria":["输出最终总额"],"script":"emit({\"answer\":\"ok,1\",\"output_contract\":{\"format\":\"csv_line\",\"explanation_allowed\":false}})"}`),
		},
	}
	planner := NewDataTaskPlanner(adapter)
	plan, err := planner.PlanDataTask(context.Background(), "汇总 CSV", "/repo", TurnPolicy{Route: RouteData, DataTaskKind: "data_aggregation"}, []dataquery.CandidateFile{
		{Path: "orders.csv", Kind: "csv", Size: 10},
	})
	if err != nil {
		t.Fatalf("PlanDataTask: %v", err)
	}
	if plan.Status != "ready" || plan.Goal != "汇总订单" || !strings.Contains(plan.Script, "emit") {
		t.Fatalf("plan=%+v", plan)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("calls=%d", len(adapter.calls))
	}
	repairSystem := adapter.calls[1].messages[0].Content
	if !strings.Contains(repairSystem, "repair one malformed structured tool call") {
		t.Fatalf("repair system prompt missing repair role:\n%s", repairSystem)
	}
	repairUser := adapter.calls[1].messages[1].Content
	for _, want := range []string{"## parse_failure", "emit_data_task_plan", "## compact_data_context", "## schema"} {
		if !strings.Contains(repairUser, want) {
			t.Fatalf("repair prompt missing %q:\n%s", want, repairUser)
		}
	}
	trace := planner.(replLLMTraceProvider).LastReplLLMTrace()
	if trace.Scope != "data_task_structured_tool_repair" {
		t.Fatalf("trace.Scope=%q", trace.Scope)
	}
}

func TestDataTaskRepairPlannerPromptCarriesExecutionErrorAndPreviousPlan(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			dataTaskPlanResp(`{"status":"ready","input_paths":["orders.csv"],"output_contract":{"format":"csv_line","explanation_allowed":false},"script":"rows=csv_rows(\"orders.csv\"); emit({\"answer\":\"ok,1\",\"output_contract\":{\"format\":\"csv_line\",\"explanation_allowed\":false}})"}`),
		},
	}
	planner := NewDataTaskPlanner(adapter)
	repairer, ok := planner.(DataTaskRepairPlanner)
	if !ok {
		t.Fatal("planner does not implement DataTaskRepairPlanner")
	}
	previous := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials:       []dataquery.CoverageMaterial{{Path: "orders.csv", Purpose: "must read rows", Required: true}},
			DecisionRecordsRequired: true,
		},
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputCSVLine,
			ExplanationAllowed: false,
		},
		Script: strings.Join([]string{
			`rows = csv_rows("orders.csv")`,
			`total = 0`,
			`for row in rows:`,
			`    total += int(row["amount"])`,
			`print("debug")`,
			`emit_result(str(total), output_contract={"format": "csv_line", "explanation_allowed": False})`,
		}, "\n"),
	}
	executionErr := `execute data task: data task script failed: exit status 1
Traceback (most recent call last):
  File "/tmp/codrax-data/_runner.py", line 130, in <module>
    exec(code, env, env)
  File "<string>", line 5, in <module>
NameError: name 'print' is not defined`
	plan, err := repairer.RepairDataTask(context.Background(), "汇总 CSV", "/repo", TurnPolicy{Route: RouteData}, []dataquery.CandidateFile{{Path: "orders.csv", Kind: "csv", Size: 10}}, previous, executionErr)
	if err != nil {
		t.Fatalf("RepairDataTask: %v", err)
	}
	if plan.Status != "ready" || !strings.Contains(plan.Script, "emit") {
		t.Fatalf("repaired plan=%+v", plan)
	}
	user := adapter.calls[0].messages[1].Content
	for _, want := range []string{"## execution_error", "NameError", "## typed_repair_locus", `"script_line": 5`, `"runner_line": 130`, "1-based line number in the model-authored script", "helper wrapper line", "script_line_excerpt", "## previous_plan_compact_json", `"line": 5`, `print(\"debug\")`, "coverage_contract", "required_materials", "usage_mode", "text_evidence_consumed", "planner_distilled", "operation pipeline"} {
		if !strings.Contains(user, want) {
			t.Fatalf("repair prompt missing %q:\n%s", want, user)
		}
	}
	trace := planner.(replLLMTraceProvider).LastReplLLMTrace()
	if trace.Scope != "data_task_repair_planner" {
		t.Fatalf("trace.Scope=%q", trace.Scope)
	}
}

func TestDataTaskRepairPlannerPromptCarriesActionLocus(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			dataTaskPlanResp(`{"status":"ready","output_contract":{"format":"plain_single_line","explanation_allowed":false},"actions":[{"id":"fix_node","kind":"custom_transform","script":"emit_result(\"ok\", output_contract={\"format\":\"plain_single_line\",\"explanation_allowed\":false})"}]}`),
		},
	}
	planner := NewDataTaskPlanner(adapter)
	repairer := planner.(DataTaskRepairPlanner)
	previous := dataquery.TaskPlan{
		Status:         "ready",
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []dataquery.DataAction{{
			ID:   "transform_1",
			Kind: dataquery.DataActionCustomTransform,
			Script: strings.Join([]string{
				`rows = csv_rows("orders.csv")`,
				`value = rows[0]["missing"]`,
				`emit_result(value, output_contract={"format": "plain_single_line", "explanation_allowed": False})`,
			}, "\n"),
			InputPaths: []string{"orders.csv"},
		}},
	}
	executionErr := `execute data task: data action failed action_id="transform_1" action_kind="custom_transform": data task script failed: exit status 1
Traceback (most recent call last):
  File "/tmp/codrax-data/_runner.py", line 130, in <module>
    exec(code, env, env)
  File "<string>", line 2, in <module>
KeyError: 'missing'`
	_, err := repairer.RepairDataTask(context.Background(), "汇总 CSV", "/repo", TurnPolicy{Route: RouteData}, []dataquery.CandidateFile{{Path: "orders.csv", Kind: "csv", Size: 10}}, previous, executionErr)
	if err != nil {
		t.Fatalf("RepairDataTask: %v", err)
	}
	user := adapter.calls[0].messages[1].Content
	for _, want := range []string{`"action_id": "transform_1"`, `"action_kind": "custom_transform"`, `"line": 2`, `rows[0][\"missing\"]`, "repair that action/node first"} {
		if !strings.Contains(user, want) {
			t.Fatalf("repair prompt missing %q:\n%s", want, user)
		}
	}
}

func TestDataTaskEvaluatorParsesTypedStatus(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{{
			ToolCalls: []llm.ToolCall{{
				Name:   dataTaskEvaluationTool.Name,
				Params: []byte(`{"status":"repair_node","reason":"fix one transform node","confidence":"high","missingInputs":"final total, row audit","actionId":"normalize_1","actionKind":"custom_transform","repairLocus":"/artifacts/0"}`),
			}},
			StopReason: "tool_use",
		}},
	}
	planner := NewDataTaskPlanner(adapter)
	evaluator, ok := planner.(DataTaskEvaluator)
	if !ok {
		t.Fatal("planner does not implement DataTaskEvaluator")
	}
	eval, err := evaluator.EvaluateDataTask(context.Background(), "汇总 CSV", []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{Status: "ready", InputPaths: []string{"orders.csv"}, ContinueAfter: true},
		Result: &dataquery.Result{
			Answer:         "intermediate",
			OutputContract: dataquery.OutputContract{Format: dataquery.OutputMarkdown, ExplanationAllowed: true},
		},
	}}, "zh")
	if err != nil {
		t.Fatalf("EvaluateDataTask: %v", err)
	}
	if eval.Status != dataquery.EvalRepairNode || eval.Confidence != "high" {
		t.Fatalf("eval=%+v", eval)
	}
	if eval.ActionID != "normalize_1" || eval.ActionKind != "custom_transform" || eval.RepairLocus != "/artifacts/0" {
		t.Fatalf("eval repair locus not parsed: %+v", eval)
	}
	if len(eval.MissingInputs) != 2 || eval.MissingInputs[1] != "row audit" {
		t.Fatalf("MissingInputs=%v", eval.MissingInputs)
	}
	user := adapter.calls[0].messages[1].Content
	for _, want := range []string{"## data_workflow_rounds", "intermediate", "continue_after", "expand_graph", "repair_node", "continue_transform"} {
		if !strings.Contains(user, want) {
			t.Fatalf("evaluation prompt missing %q:\n%s", want, user)
		}
	}
}

func TestDataTaskEvaluatorRetriesTransientLLMError(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			{},
			{
				ToolCalls: []llm.ToolCall{{
					Name:   dataTaskEvaluationTool.Name,
					Params: []byte(`{"status":"continue_transform","reason":"continue after transient retry","confidence":"medium"}`),
				}},
				StopReason: "tool_use",
			},
		},
		errors: []error{
			fmt.Errorf("temporary stream failure: %w", llm.ErrStreamStalled),
			nil,
		},
	}
	planner := NewDataTaskPlanner(adapter)
	evaluator := planner.(DataTaskEvaluator)
	eval, err := evaluator.EvaluateDataTask(context.Background(), "汇总 CSV", []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{Status: "ready", ContinueAfter: true},
		Result: &dataquery.Result{
			Answer:         "intermediate",
			OutputContract: dataquery.OutputContract{Format: dataquery.OutputMarkdown, ExplanationAllowed: true},
			Artifacts:      []dataquery.DataArtifact{{ID: "a", Kind: "extract_records"}},
		},
	}}, "zh")
	if err != nil {
		t.Fatalf("EvaluateDataTask retry: %v", err)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("calls=%d, want transient retry", len(adapter.calls))
	}
	if eval.Status != dataquery.EvalContinueTransform || eval.Reason != "continue after transient retry" {
		t.Fatalf("eval=%+v", eval)
	}
}

func TestDataTaskEvaluatorFallsBackAfterTransientLLMErrorBudget(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{{}, {}},
		errors: []error{
			fmt.Errorf("temporary stream failure: %w", llm.ErrStreamStalled),
			fmt.Errorf("temporary stream failure: %w", llm.ErrStreamStalled),
		},
	}
	planner := NewDataTaskPlanner(adapter)
	evaluator := planner.(DataTaskEvaluator)
	eval, err := evaluator.EvaluateDataTask(context.Background(), "汇总 CSV", []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{Status: "ready", ContinueAfter: true},
		Result: &dataquery.Result{
			Answer:         "intermediate",
			OutputContract: dataquery.OutputContract{Format: dataquery.OutputMarkdown, ExplanationAllowed: true},
			Artifacts:      []dataquery.DataArtifact{{ID: "a", Kind: "extract_records"}},
		},
	}}, "zh")
	if err != nil {
		t.Fatalf("EvaluateDataTask fallback should not fail workflow: %v", err)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("calls=%d, want bounded retry budget", len(adapter.calls))
	}
	if eval.Status != dataquery.EvalContinueTransform || eval.Confidence != "low" {
		t.Fatalf("eval=%+v, want deterministic conservative fallback", eval)
	}
}

func TestRenderDataTaskRecordsForPromptCarriesReconcileTotals(t *testing.T) {
	groups := make([]dataquery.ReconcileGroup, 0, 12)
	for i := 1; i <= 12; i++ {
		groups = append(groups, dataquery.ReconcileGroup{
			GroupKey: dataquery.LooseText(fmt.Sprintf("Q%03d", i)),
			Metric:   dataquery.LooseText("amount"),
			Actual:   dataquery.LooseText("1"),
		})
	}
	prompt := renderDataTaskRecordsForPrompt([]dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{Status: "ready"},
		Result: &dataquery.Result{
			Answer:         "1,2,3,4,5,6,7,8,9,10,11,12",
			OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
			Reconcile: &dataquery.ReconcileReport{
				Status: dataquery.LooseText("pass"),
				Groups: groups,
			},
		},
	}})
	for _, want := range []string{`"answer_item_count": 12`, `"reconcile_group_count": 12`, `"reconcile_groups_truncated": true`, `"Q012/amount"`} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Count(prompt, `"group_key"`) > 8 {
		t.Fatalf("compact reconcile sample should stay bounded:\n%s", prompt)
	}
}

func TestDataTaskEvaluatorRepairsMalformedToolParams(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{{
			ToolCalls: []llm.ToolCall{{
				Name:   dataTaskEvaluationTool.Name,
				Params: []byte(`ä not json`),
			}},
			StopReason: "tool_use",
		}, {
			ToolCalls: []llm.ToolCall{{
				Name:   dataTaskEvaluationTool.Name,
				Params: []byte(`{"status":"complete","reason":"computed answer satisfies contract","confidence":"high"}`),
			}},
			StopReason: "tool_use",
		}},
	}
	planner := NewDataTaskPlanner(adapter)
	evaluator := planner.(DataTaskEvaluator)
	eval, err := evaluator.EvaluateDataTask(context.Background(), "汇总 CSV", []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{Status: "ready", InputPaths: []string{"orders.csv"}},
		Result: &dataquery.Result{
			Answer:         "ok,1",
			OutputContract: dataquery.OutputContract{Format: dataquery.OutputCSVLine, ExplanationAllowed: false},
		},
	}}, "zh")
	if err != nil {
		t.Fatalf("EvaluateDataTask: %v", err)
	}
	if eval.Status != dataquery.EvalComplete || eval.Confidence != "high" {
		t.Fatalf("eval=%+v", eval)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("calls=%d", len(adapter.calls))
	}
	repairUser := adapter.calls[1].messages[1].Content
	for _, want := range []string{"## parse_failure", "emit_data_task_evaluation", "## compact_data_context"} {
		if !strings.Contains(repairUser, want) {
			t.Fatalf("repair prompt missing %q:\n%s", want, repairUser)
		}
	}
}

func TestDataTaskEvaluatorRetriesNoToolCall(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{{
			Content:    "I should continue the transform.",
			StopReason: "end_turn",
		}, {
			ToolCalls: []llm.ToolCall{{
				Name:   dataTaskEvaluationTool.Name,
				Params: []byte(`{"status":"continue_transform","reason":"materials are ready","confidence":"medium"}`),
			}},
			StopReason: "tool_use",
		}},
	}
	planner := NewDataTaskPlanner(adapter)
	evaluator := planner.(DataTaskEvaluator)
	eval, err := evaluator.EvaluateDataTask(context.Background(), "汇总 CSV", []dataTaskWorkflowRecord{{
		Plan:   dataquery.TaskPlan{Status: "ready", InputPaths: []string{"orders.csv"}},
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{ID: "records", Kind: "extract_records"}}},
	}}, "zh")
	if err != nil {
		t.Fatalf("EvaluateDataTask: %v", err)
	}
	if eval.Status != dataquery.EvalContinueTransform {
		t.Fatalf("eval=%+v, want continue_transform", eval)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("calls=%d, want no-tool retry", len(adapter.calls))
	}
	if got := adapter.calls[1].messages[1].Content; !strings.Contains(got, "previous_content_preview") || !strings.Contains(got, "emit_data_task_evaluation") {
		t.Fatalf("retry prompt=%s", got)
	}
}

func TestDataTaskEvaluatorFallsBackAfterRepeatedNoToolCall(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{{
			Content:    "Need next step.",
			StopReason: "end_turn",
		}, {
			Content:    "Still no tool.",
			StopReason: "end_turn",
		}},
	}
	planner := NewDataTaskPlanner(adapter)
	evaluator := planner.(DataTaskEvaluator)
	eval, err := evaluator.EvaluateDataTask(context.Background(), "汇总 CSV", []dataTaskWorkflowRecord{{
		Plan:   dataquery.TaskPlan{Status: "ready", InputPaths: []string{"orders.csv"}},
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{ID: "records", Kind: "extract_records"}}},
	}}, "zh")
	if err != nil {
		t.Fatalf("EvaluateDataTask: %v", err)
	}
	if eval.Status != dataquery.EvalContinueTransform || eval.Confidence != "low" {
		t.Fatalf("eval=%+v, want conservative continue_transform fallback", eval)
	}
}

func TestDataTaskResultPatchPlannerParsesTypedPatch(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			dataTaskPatchResp(`{"status":"patch","patches":[{"target":"result","op":"replace","path":"/contributions/0/operation","value":"add","reason":"canonical structural operation"}],"reason":"safe structural patch","confidence":"high"}`),
		},
	}
	planner := NewDataTaskPlanner(adapter)
	patcher, ok := planner.(DataTaskResultPatchPlanner)
	if !ok {
		t.Fatal("planner does not implement DataTaskResultPatchPlanner")
	}
	plan, err := patcher.ProposeDataResultPatch(context.Background(), "汇总 CSV", dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv"},
	}, dataquery.Result{
		Answer: "10",
		Contributions: []dataquery.ContributionRecord{{
			ItemID: "row-1", GroupKey: "A", Metric: "amount", Value: "10", Operation: "totalize",
		}},
	}, []dataquery.DataTaskViolation{{
		Code:          "unsupported_contribution_operation",
		JSONPath:      "/contributions/0/operation",
		ExpectedShape: "add/sum/count/subtract/set/rank",
		ActualSnippet: "totalize",
		Repairability: dataquery.RepairabilityNeedsRecompute,
	}}, nil, "zh")
	if err != nil {
		t.Fatalf("ProposeDataResultPatch: %v", err)
	}
	if plan.Status != "patch" || len(plan.Patches) != 1 {
		t.Fatalf("patch plan=%+v", plan)
	}
	if plan.Patches[0].Path != "/contributions/0/operation" || string(plan.Patches[0].Value) != `"add"` {
		t.Fatalf("patch=%+v", plan.Patches[0])
	}
	system := adapter.calls[0].messages[0].Content
	for _, want := range []string{"STRUCTURE", "Do not change", "answer", "business", "emit_data_result_patch"} {
		if !strings.Contains(system, want) {
			t.Fatalf("patch system prompt missing %q:\n%s", want, system)
		}
	}
	user := adapter.calls[0].messages[1].Content
	for _, want := range []string{"## typed_violations", "unsupported_contribution_operation", "## partial_result_compact_json", "## patch_rules", "json_path points to the structured result JSON", "not to a script line"} {
		if !strings.Contains(user, want) {
			t.Fatalf("patch prompt missing %q:\n%s", want, user)
		}
	}
}

func TestDataTaskResultPatchPlannerRepairsMalformedToolParams(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			dataTaskPatchResp(`ä not json`),
			dataTaskPatchResp(`{"status":"needs_recompute","patches":[],"reason":"requires recomputation"}`),
		},
	}
	planner := NewDataTaskPlanner(adapter)
	patcher := planner.(DataTaskResultPatchPlanner)
	plan, err := patcher.ProposeDataResultPatch(context.Background(), "汇总 CSV", dataquery.TaskPlan{}, dataquery.Result{}, []dataquery.DataTaskViolation{{Code: "missing_required_ledger"}}, nil, "zh")
	if err != nil {
		t.Fatalf("ProposeDataResultPatch: %v", err)
	}
	if plan.Status != "needs_recompute" {
		t.Fatalf("Status=%q", plan.Status)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("calls=%d", len(adapter.calls))
	}
	repairUser := adapter.calls[1].messages[1].Content
	for _, want := range []string{"## parse_failure", "emit_data_result_patch", "## compact_data_context", "## schema"} {
		if !strings.Contains(repairUser, want) {
			t.Fatalf("repair prompt missing %q:\n%s", want, repairUser)
		}
	}
}

func TestDataTaskPlannerPromptIncludesCandidateFiles(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			dataTaskPlanResp(`{"status":"needs_clarification","output_contract":{"format":"freeform","explanation_allowed":true},"questions":[{"id":"q1","question":"缺哪个文件？","suggestions":["上传 CSV"]}]}`),
		},
	}
	planner := NewDataTaskPlanner(adapter)
	plan, err := planner.PlanDataTask(context.Background(), "算一下", "/repo", TurnPolicy{Route: RouteData}, []dataquery.CandidateFile{
		{Path: "data/a.csv", Kind: "csv", Size: 123, Delimiter: ",", Lines: 3, Headers: []string{"vendor", "amount"}, SampleRows: [][]string{{"A", "10"}}},
	})
	if err != nil {
		t.Fatalf("PlanDataTask: %v", err)
	}
	if len(plan.Questions) != 1 {
		t.Fatalf("Questions=%v", plan.Questions)
	}
	user := adapter.calls[0].messages[1].Content
	for _, want := range []string{"## candidate_data_files", "path=data/a.csv", "kind=csv", `delimiter=","`, `headers_json=["vendor","amount"]`, `sample_rows_json=[["A","10"]]`} {
		if !strings.Contains(user, want) {
			t.Fatalf("data planner prompt missing %q:\n%s", want, user)
		}
	}
	if strings.Contains(user, "headers=vendor|amount") || strings.Contains(user, "A | 10") {
		t.Fatalf("data planner prompt still contains ambiguous pipe-delimited display:\n%s", user)
	}
}

func TestPreserveDataTaskRepairCoverage(t *testing.T) {
	previous := dataquery.TaskPlan{
		InputPaths: []string{"orders.csv", "rules.txt"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Purpose: "input", Required: true},
				{Path: "rules.txt", Purpose: "rules", Required: true},
			},
			DecisionRecordsRequired: true,
		},
	}
	repaired := dataquery.TaskPlan{
		InputPaths: []string{"orders.csv"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Purpose: "input", Required: true},
				{Path: "./rules.txt", ID: "r2", Purpose: "same rules with updated purpose", Required: true},
			},
		},
	}
	got := preserveDataTaskRepairCoverage(previous, repaired)
	if strings.Join(got.InputPaths, ",") != "orders.csv,rules.txt" {
		t.Fatalf("InputPaths=%v", got.InputPaths)
	}
	if !got.CoverageContract.DecisionRecordsRequired {
		t.Fatalf("DecisionRecordsRequired=false")
	}
	if len(got.CoverageContract.RequiredPaths()) != 2 {
		t.Fatalf("RequiredMaterials=%+v", got.CoverageContract.RequiredMaterials)
	}
	ruleCount := 0
	for _, material := range got.CoverageContract.RequiredMaterials {
		if material.Path == "rules.txt" || material.Path == "./rules.txt" {
			ruleCount++
		}
	}
	if ruleCount != 1 {
		t.Fatalf("RequiredMaterials duplicate same path: %+v", got.CoverageContract.RequiredMaterials)
	}
}

func TestPreserveDataTaskMaterialRepairCoveragePreservesPriorRequiredMaterials(t *testing.T) {
	previous := dataquery.TaskPlan{
		InputPaths: []string{"orders.csv", "old-rules.txt"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Purpose: "input", Required: true},
				{Path: "old-rules.txt", Purpose: "old rules", Required: true},
			},
			DecisionRecordsRequired: true,
		},
	}
	repaired := dataquery.TaskPlan{
		InputPaths: []string{"orders.csv", "new-rules.txt"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Purpose: "input", Required: true},
				{Path: "new-rules.txt", Purpose: "replacement rules", Required: true},
			},
			ValidationRules: []string{"old-rules.txt was not part of the corrected material set"},
		},
	}
	got := preserveDataTaskMaterialRepairCoverage(previous, repaired)
	if strings.Join(got.CoverageContract.RequiredPaths(), ",") != "new-rules.txt,old-rules.txt,orders.csv" {
		t.Fatalf("RequiredPaths=%v", got.CoverageContract.RequiredPaths())
	}
	if !got.CoverageContract.DecisionRecordsRequired {
		t.Fatalf("DecisionRecordsRequired=false")
	}
	if strings.Join(got.InputPaths, ",") != "orders.csv,new-rules.txt,old-rules.txt" {
		t.Fatalf("InputPaths=%v", got.InputPaths)
	}
}

func TestNormalizeDataTaskPlanShapeRequiresRuleCoverageForDeriveRules(t *testing.T) {
	plan := dataquery.TaskPlan{
		Actions: []dataquery.DataAction{{
			ID:         "rules",
			Kind:       dataquery.DataActionDeriveRules,
			InputPaths: []string{"rules.md"},
		}},
	}
	got, reasons := normalizeDataTaskPlanShape(plan)
	if !got.CoverageContract.RuleCoverageRequired {
		t.Fatalf("RuleCoverageRequired=false")
	}
	if !strings.Contains(strings.Join(reasons, "\n"), "derive_rules") {
		t.Fatalf("reasons=%v, want derive_rules normalization reason", reasons)
	}
}

func TestNormalizeDataTaskPlanShapeFillsDeriveRulesInputsFromRequiredMaterials(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "notes.txt", Required: true, UsageMode: dataquery.MaterialUseTextEvidenceConsumed, TextEvidencePath: "notes.txt"},
			},
		},
		Actions: []dataquery.DataAction{{
			ID:   "rules",
			Kind: dataquery.DataActionDeriveRules,
		}},
	}
	got, reasons := normalizeDataTaskPlanShape(plan)
	if strings.Join(got.Actions[0].InputPaths, ",") != "rules.md,notes.txt" {
		t.Fatalf("InputPaths=%v, want text/rule materials only", got.Actions[0].InputPaths)
	}
	if !strings.Contains(strings.Join(reasons, "\n"), "filled derive_rules input_paths") {
		t.Fatalf("reasons=%v, want derive_rules input normalization reason", reasons)
	}
}

func TestNormalizeDataTaskPlanShapeNarrowsDeriveRulesInputsToRuleMaterials(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			OptionalMaterials: []dataquery.CoverageMaterial{{
				Path:           "rules.md",
				UsageMode:      dataquery.MaterialUsePlannerDistilled,
				DistilledNotes: []string{"include matching rows"},
			}},
			ValidationRules: []string{"include matching rows"},
		},
		Actions: []dataquery.DataAction{{
			ID:         "rules",
			Kind:       dataquery.DataActionDeriveRules,
			InputPaths: []string{"orders.csv", "rules.md", "text_evidence/invoice.txt"},
		}},
	}
	got, reasons := normalizeDataTaskPlanShape(plan)
	if strings.Join(got.Actions[0].InputPaths, ",") != "rules.md" {
		t.Fatalf("InputPaths=%v, want only declared rule material", got.Actions[0].InputPaths)
	}
	if !strings.Contains(strings.Join(reasons, "\n"), "filled derive_rules input_paths") {
		t.Fatalf("reasons=%v, want derive_rules input normalization reason", reasons)
	}
}

func TestDataTaskMaterialDiscoveryFallbackForBroadScriptPlan(t *testing.T) {
	var required []dataquery.CoverageMaterial
	var inputs []string
	for i := 0; i < dataTaskBroadMaterialDiscoveryLimit; i++ {
		path := fmt.Sprintf("input_%02d.csv", i+1)
		inputs = append(inputs, path)
		required = append(required, dataquery.CoverageMaterial{Path: path, Required: true})
	}
	plan := dataquery.TaskPlan{
		InputPaths: inputs,
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: required,
		},
		Script: `print("debug only")`,
	}
	fallback, ok := dataTaskMaterialDiscoveryFallback(nil, plan, "script has no result emitter")
	if !ok {
		t.Fatal("fallback=false, want broad material discovery")
	}
	if !fallback.ContinueAfter {
		t.Fatalf("ContinueAfter=false")
	}
	if len(fallback.Actions) != 1 || fallback.Actions[0].Kind != dataquery.DataActionMaterialInventory {
		t.Fatalf("Actions=%+v, want material_inventory", fallback.Actions)
	}
	if len(fallback.CoverageContract.RequiredMaterials) != 0 {
		t.Fatalf("fallback should not carry blocking required materials: %+v", fallback.CoverageContract.RequiredMaterials)
	}
}

func TestDataTaskMaterialDiscoveryFallbackDoesNotRepeatAfterInventory(t *testing.T) {
	var inputs []string
	for i := 0; i < dataTaskBroadMaterialDiscoveryLimit; i++ {
		inputs = append(inputs, fmt.Sprintf("input_%02d.csv", i+1))
	}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			Actions: []dataquery.DataAction{{
				ID:         "inventory",
				Kind:       dataquery.DataActionMaterialInventory,
				InputPaths: inputs,
			}},
		},
		Result: &dataquery.Result{Answer: "inventory complete"},
	}}
	plan := dataquery.TaskPlan{
		InputPaths: inputs,
		Actions: []dataquery.DataAction{{
			ID:         "broad_transform",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: inputs,
			Script:     `emit_result({"answer":"ok"})`,
		}},
	}
	if _, ok := dataTaskMaterialDiscoveryFallback(records, plan, "broad plan"); ok {
		t.Fatal("fallback=true after material_inventory already ran")
	}
}

func TestDataTaskMaterialDiscoveryFallbackDoesNotRunAfterCoverageSufficient(t *testing.T) {
	var inputs []string
	var required []dataquery.CoverageMaterial
	for i := 0; i < dataTaskBroadMaterialDiscoveryLimit; i++ {
		p := fmt.Sprintf("input_%02d.csv", i+1)
		inputs = append(inputs, p)
		required = append(required, dataquery.CoverageMaterial{Path: p, Required: true})
	}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{CoverageContract: dataquery.CoverageContract{RequiredMaterials: required}},
		Result: &dataquery.Result{
			ConsumedPaths: inputs,
		},
	}}
	plan := dataquery.TaskPlan{
		InputPaths: inputs,
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: required,
		},
		Actions: []dataquery.DataAction{{
			ID:         "broad_transform",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: inputs,
			Script:     `emit_result({"answer":"ok"})`,
		}},
	}
	if _, ok := dataTaskMaterialDiscoveryFallback(records, plan, "broad plan"); ok {
		t.Fatal("fallback=true after required materials were already covered")
	}
}

func TestDataTaskTerminalPlanCompletionGateRejectsDroppedWorkflowRequirement(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			InputPaths: []string{"orders.csv", "rules.md"},
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{
					{Path: "orders.csv", Purpose: "source rows", Required: true},
					{Path: "rules.md", Purpose: "rules", Required: true},
				},
			},
		},
		Result: &dataquery.Result{
			Answer:         "42",
			OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
			ConsumedPaths:  []string{"orders.csv"},
		},
	}}
	current := dataquery.TaskPlan{
		Status:     "complete",
		InputPaths: []string{"orders.csv"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{{Path: "orders.csv", Purpose: "source rows", Required: true}},
		},
	}
	errText := dataTaskTerminalPlanCompletionGateError(records, current)
	if errText == "" {
		t.Fatal("terminal complete gate returned empty error, want dropped rules.md rejection")
	}
	if !strings.Contains(errText, "rules.md") {
		t.Fatalf("errText=%q", errText)
	}
}

func TestPreserveDataTaskMaterialRepairCoverageForOversizedStagedBatch(t *testing.T) {
	previous := dataquery.TaskPlan{
		InputPaths: []string{"orders.csv", "vendors.csv", "rules.md"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Purpose: "source rows", Required: true},
				{Path: "vendors.csv", Purpose: "lookup", Required: true},
				{Path: "rules.md", Purpose: "rules", Required: true},
			},
			DecisionRecordsRequired:    true,
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			EntityResolutionRequired:   true,
			ReconcileRequired:          true,
		},
	}
	repaired := dataquery.TaskPlan{
		InputPaths:    []string{"vendors.csv"},
		ContinueAfter: true,
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "vendors.csv", Purpose: "staged lookup normalization", Required: true},
			},
			EntityResolutionRequired: true,
		},
	}
	errText := "data planning incomplete: plan is too large for one bounded data batch (script_lines=365 required_materials=6 validation_ledgers=5 continue_after=false). Emit a smaller bounded batch, set continue_after=true when further work remains, and let the workflow feed real results into later batches."
	got := preserveDataTaskMaterialRepairCoverageForError(previous, repaired, errText)
	if !got.ContinueAfter {
		t.Fatalf("ContinueAfter=false")
	}
	if got.CoverageContract.DecisionRecordsRequired || got.CoverageContract.RuleCoverageRequired || got.CoverageContract.ContributionLedgerRequired || got.CoverageContract.ReconcileRequired {
		t.Fatalf("staged oversized repair inherited final ledgers: %+v", got.CoverageContract)
	}
	if !got.CoverageContract.EntityResolutionRequired {
		t.Fatalf("stage-specific ledger was lost: %+v", got.CoverageContract)
	}
	if strings.Join(got.CoverageContract.RequiredPaths(), ",") != "vendors.csv" {
		t.Fatalf("RequiredPaths=%v, want only staged material", got.CoverageContract.RequiredPaths())
	}
}

func TestPreserveDataTaskMaterialRepairCoverageForScopedActionFailure(t *testing.T) {
	previous := dataquery.TaskPlan{
		InputPaths: []string{"orders.csv", "rules.md", "lookup.csv"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Purpose: "source rows", Required: true},
				{Path: "rules.md", Purpose: "rules", Required: true},
				{Path: "lookup.csv", Purpose: "lookup", Required: true},
			},
			DecisionRecordsRequired:    true,
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
	}
	repaired := dataquery.TaskPlan{
		InputPaths: []string{"rules.md"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials:    []dataquery.CoverageMaterial{{Path: "rules.md", Purpose: "derive current batch rules", Required: true}},
			RuleCoverageRequired: true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "derive_rules",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"rules.md"},
			Script:     `rules = read_text("rules.md")` + "\n" + `emit_result({"answer":"rules", "output_contract":{"format":"freeform","explanation_allowed":true}, "rule_coverage":[{"rule_id":"r1","rule_text":rules[:20],"status":"derived","notes":"batch rule"}]})`,
		}},
	}
	errText := `execute data task: data action failed action_id="derive_rules" action_kind="custom_transform": data coverage incomplete: required material "orders.csv" was not consumed by the script`
	got := preserveDataTaskMaterialRepairCoverageForError(previous, repaired, errText)
	if !got.ContinueAfter {
		t.Fatalf("ContinueAfter=false")
	}
	if strings.Join(got.CoverageContract.RequiredPaths(), ",") != "rules.md" {
		t.Fatalf("RequiredPaths=%v, want scoped rules.md only", got.CoverageContract.RequiredPaths())
	}
	if got.CoverageContract.DecisionRecordsRequired || got.CoverageContract.ContributionLedgerRequired || got.CoverageContract.ReconcileRequired {
		t.Fatalf("scoped action inherited final ledgers: %+v", got.CoverageContract)
	}
	if strings.Join(got.InputPaths, ",") != "rules.md" {
		t.Fatalf("InputPaths=%v", got.InputPaths)
	}
}

func TestPreserveDataTaskMaterialRepairCoverageForNonStagedRepairStillProtectsCoverage(t *testing.T) {
	previous := dataquery.TaskPlan{
		InputPaths: []string{"orders.csv", "rules.md"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Purpose: "source rows", Required: true},
				{Path: "rules.md", Purpose: "rules", Required: true},
			},
			RuleCoverageRequired: true,
		},
	}
	repaired := dataquery.TaskPlan{
		InputPaths: []string{"orders.csv"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{{Path: "orders.csv", Purpose: "source rows", Required: true}},
		},
	}
	got := preserveDataTaskMaterialRepairCoverageForError(previous, repaired, "execute data task: data task script failed")
	if !got.CoverageContract.RuleCoverageRequired {
		t.Fatalf("RuleCoverageRequired=false")
	}
	if strings.Join(got.InputPaths, ",") != "orders.csv,rules.md" {
		t.Fatalf("InputPaths=%v", got.InputPaths)
	}
}

func TestShouldValidateDataTaskWorkflowResultSkipsIntermediateBatch(t *testing.T) {
	if shouldValidateDataTaskWorkflowResult(dataquery.TaskPlan{ContinueAfter: true}) {
		t.Fatal("continue_after intermediate batch should skip workflow-final validation")
	}
	if !shouldValidateDataTaskWorkflowResult(dataquery.TaskPlan{}) {
		t.Fatal("final batch should run workflow validation")
	}
}

func TestNormalizeDataTaskPlanShapeFillsRolePathActionInputs(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:   "normalize",
			Kind: dataquery.DataActionNormalizeEntities,
			Params: map[string]string{
				"source_path":    "records.json",
				"reference_path": "lookup.json",
				"source_field":   "raw_name",
			},
		}},
	}
	got, notes := normalizeDataTaskPlanShape(plan)
	if strings.Join(got.Actions[0].InputPaths, ",") != "records.json,lookup.json" {
		t.Fatalf("InputPaths=%v, want role paths normalized into action inputs", got.Actions[0].InputPaths)
	}
	if !strings.Contains(strings.Join(notes, "; "), "role path") {
		t.Fatalf("notes=%v, want role path normalization note", notes)
	}

	plan = dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "join",
			Kind:       dataquery.DataActionJoinRecords,
			InputPaths: []string{"left.json"},
			Params: map[string]string{
				"left_path":  "left.json",
				"right_path": "right.json",
			},
		}},
	}
	got, _ = normalizeDataTaskPlanShape(plan)
	if strings.Join(got.Actions[0].InputPaths, ",") != "left.json,right.json" {
		t.Fatalf("InputPaths=%v, want missing right role path appended", got.Actions[0].InputPaths)
	}
}

func TestDataTaskWorkflowStateDoesNotTreatIntermediateAnswerAsFinal(t *testing.T) {
	contract := dataquery.CoverageContract{
		RequiredMaterials: []dataquery.CoverageMaterial{
			{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
		},
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	}
	current := dataquery.TaskPlan{
		OutputContract:   dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true},
		CoverageContract: contract,
	}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			OutputContract:   dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
			CoverageContract: contract,
		},
		Result: &dataquery.Result{
			Answer:         "coverage_records.json: extracted record samples",
			OutputContract: dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true},
			ConsumedPaths:  []string{"records.csv"},
		},
	}}
	state := dataTaskWorkflowState(records, current)
	if state.HasAnswer {
		t.Fatalf("HasAnswer=true for intermediate coverage summary; state=%+v", state)
	}
	if state.NextStage != "prepare_contribution_inputs" {
		t.Fatalf("NextStage=%q, want prepare_contribution_inputs; state=%+v", state.NextStage, state)
	}

	records = append(records, dataTaskWorkflowRecord{
		Result: &dataquery.Result{
			Answer:         "42",
			OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
			Contributions: []dataquery.ContributionRecord{{
				ItemID:        dataquery.LooseText("item-1"),
				Source:        dataquery.LooseText("records.csv"),
				SourceLocator: dataquery.LooseText("row=2"),
				GroupKey:      dataquery.LooseText("g1"),
				Metric:        dataquery.LooseText("count"),
				Value:         dataquery.LooseText("42"),
				Operation:     dataquery.LooseText("add"),
			}},
			Reconcile: &dataquery.ReconcileReport{
				Status:       dataquery.LooseText("pass"),
				ActualAnswer: dataquery.LooseText("42"),
			},
			ConsumedPaths: []string{"records.csv"},
		},
	})
	state = dataTaskWorkflowState(records, current)
	if !state.HasAnswer || state.NextStage != "complete" {
		t.Fatalf("state=%+v, want final answer complete", state)
	}
}

func TestDataTaskWorkflowStateDoesNotTreatActionSummaryAsFinalAnswer(t *testing.T) {
	current := dataquery.TaskPlan{
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
		},
	}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			OutputContract: current.OutputContract,
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: current.CoverageContract.RequiredMaterials,
			},
			Actions: []dataquery.DataAction{
				{ID: "inspect", Kind: dataquery.DataActionInspectMaterial, InputPaths: []string{"records.csv"}},
				{ID: "extract", Kind: dataquery.DataActionExtractRecords, InputPaths: []string{"records.csv"}},
			},
		},
		Result: &dataquery.Result{
			Answer:         "2 artifact(s)",
			OutputContract: current.OutputContract,
			Artifacts:      []dataquery.DataArtifact{{ID: "inspect"}, {ID: "extract"}},
			ConsumedPaths:  []string{"records.csv"},
		},
	}}
	state := dataTaskWorkflowState(records, current)
	if state.HasAnswer || state.NextStage != "compute_contributions" {
		t.Fatalf("state=%+v, want action artifact summary to remain intermediate compute stage", state)
	}
}

func TestDataTaskWorkflowStatePreservesDeferredRequiredMaterials(t *testing.T) {
	current := dataquery.TaskPlan{
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "orders.csv", Purpose: dataTaskUserExplicitMaterialPurpose, Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
		},
		Actions: []dataquery.DataAction{{
			ID:         "read_rules",
			Kind:       dataquery.DataActionDeriveRules,
			InputPaths: []string{"rules.md"},
		}},
	}
	constrained := dataTaskConstrainCurrentRequiredMaterials(dataquery.CoverageContract{}, current)
	if got := constrained.RequiredRunnerInputPaths(); strings.Join(got, ",") != "rules.md" {
		t.Fatalf("current batch required runner paths=%v, want only rules.md", got)
	}
	if len(constrained.OptionalMaterials) != 1 || !constrained.OptionalMaterials[0].Required || constrained.OptionalMaterials[0].Path != "orders.csv" {
		t.Fatalf("deferred optional materials=%+v, want orders.csv with workflow Required=true", constrained.OptionalMaterials)
	}
	state := dataTaskWorkflowState(nil, dataquery.TaskPlan{CoverageContract: constrained})
	if state.MaterialCoverageSufficient {
		t.Fatalf("state=%+v, want deferred required material to keep coverage incomplete", state)
	}
	if !slices.Contains(state.RequiredMaterials, "orders.csv") || !slices.Contains(state.MissingRequiredMaterials, "orders.csv") {
		t.Fatalf("state=%+v, want orders.csv preserved as missing workflow requirement", state)
	}
}

func TestDataTaskWorkflowStateExposesWorkflowAndCurrentBatchContracts(t *testing.T) {
	current := dataquery.TaskPlan{
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "orders.csv", Purpose: dataTaskUserExplicitMaterialPurpose, Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "read_rules",
			Kind:       dataquery.DataActionDeriveRules,
			InputPaths: []string{"rules.md"},
		}},
	}
	state := dataTaskWorkflowState(nil, current)
	if state.WorkflowContract.Layer != "workflow" {
		t.Fatalf("WorkflowContract.Layer=%q", state.WorkflowContract.Layer)
	}
	if got := state.WorkflowContract.RequiredRunnerInputPaths; strings.Join(got, ",") != "orders.csv" {
		t.Fatalf("workflow runner inputs=%v, want only durable user floor orders.csv", got)
	}
	if state.CurrentBatchContract.Layer != "current_batch" {
		t.Fatalf("CurrentBatchContract.Layer=%q", state.CurrentBatchContract.Layer)
	}
	if got := state.CurrentBatchContract.RequiredRunnerInputPaths; strings.Join(got, ",") != "rules.md" {
		t.Fatalf("current batch runner inputs=%v, want only rules.md", got)
	}
	if !state.CurrentBatchContract.ContributionLedgerRequired || !state.CurrentBatchContract.ReconcileRequired {
		t.Fatalf("current batch ledger flags=%+v, want durable ledger requirements visible", state.CurrentBatchContract)
	}
	if state.Decision.Status != "continue" || state.Decision.ReasonCode == "" {
		t.Fatalf("Decision=%+v, want continue decision projected into workflow state", state.Decision)
	}
}

func TestDataTaskWorkflowStateExposesActionGraphProjection(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:             "extract_records",
			Kind:           dataquery.DataActionExtractRecords,
			InputPaths:     []string{"records.csv"},
			OutputArtifact: "records.json",
		}}},
		Result: &dataquery.Result{ConsumedPaths: []string{"records.csv"}},
	}}
	current := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:             "compute",
		Kind:           dataquery.DataActionComputeContribs,
		InputPaths:     []string{"records.json"},
		OutputArtifact: "contribs.json",
		Params:         map[string]string{"value_field": "amount"},
	}}}
	state := dataTaskWorkflowState(records, current)
	if len(state.ActionGraph.Executed) != 1 || state.ActionGraph.Executed[0].Status != "executed" {
		t.Fatalf("executed action graph=%+v", state.ActionGraph.Executed)
	}
	if len(state.ActionGraph.Ready) != 1 || state.ActionGraph.Ready[0].DependencyRank != 4 {
		t.Fatalf("ready action graph=%+v, want compute_contributions rank 4", state.ActionGraph.Ready)
	}
	if state.ActionGraph.Ready[0].IdempotencyKey == "" {
		t.Fatalf("ready action node missing idempotency key: %+v", state.ActionGraph.Ready[0])
	}
}

func TestDataTaskWorkflowStatePromotesLatestEvaluationIntoDecision(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Evaluation: &dataquery.Evaluation{
			Status: dataquery.EvalRepairNode,
			Reason: "computed artifact needs another typed field derivation",
		},
	}}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if state.Decision.Status != string(dataquery.EvalRepairNode) || state.Decision.ReasonCode != string(dataquery.EvalRepairNode) {
		t.Fatalf("Decision=%+v, want evaluation status projected", state.Decision)
	}
	if state.Decision.Reason != "computed artifact needs another typed field derivation" {
		t.Fatalf("Decision.Reason=%q, want evaluation reason", state.Decision.Reason)
	}
}

func TestDataTaskWorkflowStateExposesTypedWorkflowViolations(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Err: `data planning incomplete: action 1 (filter_eligible) references field(s) [currency, status] that are not present on input records fields [id, amount]. Use an existing artifact from workflow_state_json.artifact_availability, or first materialize the missing field(s) with derive_fields, extract_fields, group_records, enrich_records, join_records, or a valid prior typed action before consuming them.`,
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:      "records",
			Kind:    string(dataquery.DataActionExtractRecords),
			Headers: []string{"id", "amount"},
			Fields:  map[string]string{"artifact_aliases": "records", "json_shape": "array(len=2,item=object(keys=id,amount))"},
		}}},
	}}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{CoverageContract: dataquery.CoverageContract{ContributionLedgerRequired: true}})
	if len(state.WorkflowViolations) == 0 {
		t.Fatalf("WorkflowViolations empty; field violations=%+v", state.FieldContractViolations)
	}
	got := state.WorkflowViolations[0]
	if got.Code != "field_contract_violation" || got.InputAlias != "records" {
		t.Fatalf("violation=%+v, want typed field contract violation for records", got)
	}
	if !slices.Contains(got.MissingFields, "currency") || !slices.Contains(got.MissingFields, "status") {
		t.Fatalf("MissingFields=%v", got.MissingFields)
	}
	if got.Repairability != "needs_typed_action" {
		t.Fatalf("Repairability=%q", got.Repairability)
	}
	if state.Decision.Status != "blocked" || state.Decision.ReasonCode != "field_contract_violation" {
		t.Fatalf("Decision=%+v, want blocked field-contract decision", state.Decision)
	}
	if len(state.ActionGraph.Blocked) == 0 || state.ActionGraph.Blocked[0].Status != dataworkflow.ActionStatusBlocked {
		t.Fatalf("blocked action graph=%+v, want typed violation projected as blocked node", state.ActionGraph.Blocked)
	}
}

func TestDataTaskMissingFieldGuardProducesTypedViolation(t *testing.T) {
	action := dataquery.DataAction{
		ID:         "filter_status",
		Kind:       dataquery.DataActionFilterRecords,
		InputPaths: []string{"records"},
	}
	violation, ok := dataTaskActionMissingFieldContractViolation(action, "records", []string{"status"}, []dataTaskArtifactAccessPrompt{{
		ID:      "records",
		Aliases: []string{"records"},
		Fields:  []string{"id", "amount"},
	}})
	if !ok {
		t.Fatalf("expected typed guard violation")
	}
	if violation.Code != "field_contract_violation" || violation.IdempotencyKey == "" || violation.DependencyRank == 0 {
		t.Fatalf("violation=%+v, want typed field contract guard violation", violation)
	}
	guard := dataTaskActionMissingFieldContractGuardResult(action, 0, "records", []string{"status"}, []dataTaskArtifactAccessPrompt{{
		ID:      "records",
		Aliases: []string{"records"},
		Fields:  []string{"id", "amount"},
	}})
	if guard.Code != "field_contract_violation" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want field_contract_violation with typed violation payload", guard)
	}
}

func TestDataTaskContinuationPromptIncludesDeferredActionGraph(t *testing.T) {
	deferred := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:         "join_next",
		Kind:       dataquery.DataActionJoinRecords,
		InputPaths: []string{"left.json", "right.json"},
	}}}
	prompt := dataTaskContinuationPromptWithDeferred(
		"继续计算",
		"/tmp/repo",
		TurnPolicy{Route: "data"},
		nil,
		nil,
		deferred,
	)
	if !strings.Contains(prompt, `"deferred"`) || !strings.Contains(prompt, `"join_next"`) || !strings.Contains(prompt, `"status": "deferred"`) {
		t.Fatalf("prompt missing deferred action graph:\n%s", prompt)
	}
}

func TestDataTaskCoverageExpansionFallbackDoesNotForgetHistoricalCoverage(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{
					{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				},
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"records.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:          "records_artifact",
				Kind:        string(dataquery.DataActionExtractRecords),
				SourcePaths: []string{"records.csv"},
			}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
		},
		Actions: []dataquery.DataAction{{
			ID:         "narrow_transform",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"records.csv"},
			Script:     `rows = csv_rows("records.csv"); emit({"answer": str(len(rows))})`,
		}},
	}
	if fallback, ok := dataTaskCoverageExpansionFallback(records, plan, "prior material scheduling check failed"); ok {
		t.Fatalf("fallback=%+v, want no coverage fallback after historical material coverage", fallback)
	}
}

func TestDataTaskResultPromptIncludesMaterialSetHandles(t *testing.T) {
	result := dataquery.Result{Artifacts: []dataquery.DataArtifact{{
		ID:   "inventory",
		Kind: string(dataquery.DataActionMaterialInventory),
		Children: []dataquery.DataArtifact{
			{
				ID:          "evidence/a.txt",
				Kind:        "text",
				SourcePaths: []string{"evidence/a.txt"},
			},
			{
				ID:          "evidence/b.txt",
				Kind:        "text",
				SourcePaths: []string{"evidence/b.txt"},
			},
			{
				ID:          "scan/a.png",
				Kind:        "image",
				SourcePaths: []string{"scan/a.png"},
				Fields:      map[string]string{"text_evidence_paths": "evidence/a.txt, evidence/b.txt"},
			},
		},
	}}}
	view := compactDataTaskResultPromptView(result, 100, 100, 1, 1, 1)
	if len(view.MaterialSetHandles) < 2 {
		t.Fatalf("MaterialSetHandles=%+v, want related text and directory handles", view.MaterialSetHandles)
	}
	var sawRelated, sawDir bool
	for _, handle := range view.MaterialSetHandles {
		if handle.Kind == "related_text_evidence" && strings.Join(handle.TextEvidencePaths, ",") == "evidence/a.txt,evidence/b.txt" {
			sawRelated = true
		}
		if handle.ID == "dir:evidence" && strings.Join(handle.MemberPaths, ",") == "evidence/a.txt,evidence/b.txt" {
			sawDir = true
		}
	}
	if !sawRelated || !sawDir {
		t.Fatalf("MaterialSetHandles=%+v, want related=%t dir=%t", view.MaterialSetHandles, sawRelated, sawDir)
	}
}

func TestDataTaskPlanStagingGuardRejectsBroadCustomTransform(t *testing.T) {
	lines := make([]string, 0, dataTaskComplexCustomScriptLineLimit+2)
	for i := 0; i < dataTaskComplexCustomScriptLineLimit; i++ {
		lines = append(lines, fmt.Sprintf("value_%d = %d", i, i))
	}
	lines = append(lines, `emit_result("ok", output_contract={"format":"plain_single_line","explanation_allowed":False})`)
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "a.csv", Required: true},
				{Path: "b.csv", Required: true},
				{Path: "c.csv", Required: true},
				{Path: "d.csv", Required: true},
			},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "whole_workflow",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv"},
			Script:     strings.Join(lines, "\n"),
		}},
	}
	errText := dataTaskPlanStagingGuardError(plan)
	if !strings.Contains(errText, "too broad for one bounded custom_transform") {
		t.Fatalf("errText=%q", errText)
	}
}

func TestDataTaskPlanStagingGuardAllowsFinalCustomTransformAfterTypedContext(t *testing.T) {
	lines := make([]string, 0, dataTaskOneShotScriptLineSoftLimit)
	for i := 0; i < dataTaskComplexCustomScriptLineLimit+30; i++ {
		lines = append(lines, fmt.Sprintf("value_%d = %d", i, i))
	}
	lines = append(lines, `emit_result("ok", output_contract={"format":"plain_single_line","explanation_allowed":False})`)
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"projection_source.json"},
		CoverageContract: dataquery.CoverageContract{
			ReconcileRequired: true,
		},
		Actions: []dataquery.DataAction{
			{ID: "inspect", Kind: dataquery.DataActionInspectMaterial, InputPaths: []string{"projection_source.json"}},
			{ID: "rules", Kind: dataquery.DataActionDeriveRules, InputPaths: []string{"rules.md"}},
			{
				ID:         "final_transform",
				Kind:       dataquery.DataActionCustomTransform,
				InputPaths: []string{"projection_source.json"},
				Script:     strings.Join(lines, "\n"),
			},
		},
	}
	if errText := dataTaskPlanStagingGuardError(plan); errText != "" {
		t.Fatalf("final custom transform after typed context should pass, got %q", errText)
	}
}

func TestDataTaskPlanStagingGuardRejectsBroadCustomTransformAfterTypedContext(t *testing.T) {
	lines := make([]string, 0, dataTaskOneShotScriptLineSoftLimit)
	for i := 0; i < dataTaskComplexCustomScriptLineLimit+5; i++ {
		lines = append(lines, fmt.Sprintf("value_%d = %d", i, i))
	}
	lines = append(lines, `emit({"answer":"0","rows":[{"row_id":"r1","source":"a.csv","source_locator":"line:2","decision":"include"}],"contributions":[{"item_id":"r1","source":"a.csv","source_locator":"line:2","group_key":"Q1","metric":"amount","value":"1","operation":"add"}],"reconcile":{"status":"pass","groups":[{"group_key":"Q1","metric":"amount","actual":"1","expected":"1"}]}})`)
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "a.csv", Required: true},
				{Path: "b.csv", Required: true},
				{Path: "c.csv", Required: true},
				{Path: "d.csv", Required: true},
			},
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{
			{ID: "rules", Kind: dataquery.DataActionDeriveRules, InputPaths: []string{"rules.md"}},
			{
				ID:         "compute_all",
				Kind:       dataquery.DataActionCustomTransform,
				InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv", "rules.md"},
				Script:     strings.Join(lines, "\n"),
			},
		},
	}
	errText := dataTaskPlanStagingGuardError(plan)
	if !strings.Contains(errText, "too broad for one bounded custom_transform") {
		t.Fatalf("errText=%q, want broad custom_transform rejected even after typed context", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsBroadCustomTransformWithoutPrerequisites(t *testing.T) {
	lines := make([]string, 0, dataTaskOneShotScriptLineSoftLimit)
	for i := 0; i < dataTaskComplexCustomScriptLineLimit+5; i++ {
		lines = append(lines, fmt.Sprintf("value_%d = %d", i, i))
	}
	lines = append(lines, `emit_result("ok", output_contract={"format":"plain_single_line","explanation_allowed":False})`)
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "rules.md", Required: true},
				{Path: "orders.csv", Required: true},
				{Path: "lookup.csv", Required: true},
				{Path: "evidence.csv", Required: true},
			},
			DecisionRecordsRequired:    true,
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{
			{ID: "rules", Kind: dataquery.DataActionDeriveRules, InputPaths: []string{"rules.md"}},
			{ID: "inspect_orders", Kind: dataquery.DataActionInspectMaterial, InputPaths: []string{"orders.csv"}},
			{
				ID:         "final_transform",
				Kind:       dataquery.DataActionCustomTransform,
				InputPaths: []string{"rules.md", "orders.csv", "lookup.csv", "evidence.csv"},
				Script:     strings.Join(lines, "\n"),
			},
		},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "not covered by prior typed actions/results") || !strings.Contains(errText, "lookup.csv") || !strings.Contains(errText, "evidence.csv") {
		t.Fatalf("errText=%q", errText)
	}
}

func TestDataTaskWorkflowStagingGuardAcceptsPriorArtifactAlias(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{Status: "ready"},
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:      "orders_records",
			Kind:    "extract_records",
			Summary: "records extracted",
			Fields: map[string]string{
				"artifact_path":    "/tmp/orders_records.json",
				"artifact_aliases": "orders_records,orders_records.json,artifacts/orders_records.json",
			},
		}}},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "final_transform",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"artifacts/orders_records.json", "orders.csv", "lookup.csv", "evidence.csv"},
			Script: strings.Repeat("x = 1\n", dataTaskComplexCustomScriptLineLimit) +
				`emit_result("ok", output_contract={"format":"plain_single_line","explanation_allowed":False})`,
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if strings.Contains(errText, "artifacts/orders_records.json") {
		t.Fatalf("errText=%q, prior artifact alias should be covered", errText)
	}
}

func TestNormalizeDataTaskPlanShapeSplitsCommaSeparatedPathLists(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv,queries.csv", "business note, not a path list"},
		Actions: []dataquery.DataAction{{
			ID:         "inspect",
			Kind:       dataquery.DataActionInspectMaterial,
			InputPaths: []string{"rules.md,vendors.csv"},
		}},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{{
				ID:               "contract_evidence",
				Path:             "contracts/A.txt,contracts/B.txt",
				TextEvidencePath: "text/A.txt,text/B.txt",
				Required:         true,
				UsageMode:        dataquery.MaterialUseTextEvidenceConsumed,
			}},
			OptionalMaterials: []dataquery.CoverageMaterial{{
				ID:       "optional",
				Path:     "plain prose, with comma",
				Required: false,
			}},
		},
	}
	got, reasons := normalizeDataTaskPlanShape(plan)
	if !strings.Contains(strings.Join(reasons, ","), "comma-separated path lists") {
		t.Fatalf("reasons=%v, want comma path normalization note", reasons)
	}
	if want := []string{"orders.csv", "queries.csv", "business note, not a path list"}; strings.Join(got.InputPaths, "|") != strings.Join(want, "|") {
		t.Fatalf("InputPaths=%v, want %v", got.InputPaths, want)
	}
	if want := []string{"rules.md", "vendors.csv"}; strings.Join(got.Actions[0].InputPaths, "|") != strings.Join(want, "|") {
		t.Fatalf("action InputPaths=%v, want %v", got.Actions[0].InputPaths, want)
	}
	materials := got.CoverageContract.RequiredMaterials
	if len(materials) != 2 {
		t.Fatalf("required materials=%+v, want 2 split materials", materials)
	}
	if materials[0].Path != "contracts/A.txt" || materials[0].TextEvidencePath != "text/A.txt" ||
		materials[1].Path != "contracts/B.txt" || materials[1].TextEvidencePath != "text/B.txt" {
		t.Fatalf("split materials=%+v", materials)
	}
	if len(got.CoverageContract.OptionalMaterials) != 1 || got.CoverageContract.OptionalMaterials[0].Path != "plain prose, with comma" {
		t.Fatalf("optional materials=%+v, prose should not be split", got.CoverageContract.OptionalMaterials)
	}
}

func TestDataTaskWorkflowStagingGuardRequiresRuleCoverageForTextConstraintMaterial(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{{
				Path:      "rules.md",
				Required:  true,
				UsageMode: dataquery.MaterialUseScriptConsumed,
			}},
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "final_transform",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"rules.md", "orders.csv"},
			Script:     `emit_result("10", output_contract={"format":"plain_single_line","explanation_allowed":False})`,
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "rule_coverage_required=false") || !strings.Contains(errText, "rules.md") {
		t.Fatalf("errText=%q, want text-rule coverage guard", errText)
	}
	plan.CoverageContract.RuleCoverageRequired = true
	if errText := dataTaskWorkflowStagingGuardError(nil, plan); strings.Contains(errText, "rule_coverage_required=false") {
		t.Fatalf("errText=%q, should not require rule coverage when already enabled", errText)
	}
}

func TestMergeDataTaskCoverageContractsLetsNextOptionalOverridePreviousRequired(t *testing.T) {
	previous := dataquery.CoverageContract{
		RequiredMaterials: []dataquery.CoverageMaterial{{
			Path:      "lookup.json",
			Required:  true,
			UsageMode: dataquery.MaterialUseScriptConsumed,
		}},
	}
	next := dataquery.CoverageContract{
		OptionalMaterials: []dataquery.CoverageMaterial{{
			Path:      "lookup.json",
			UsageMode: dataquery.MaterialUseReferenceOnly,
		}},
	}
	merged := mergeDataTaskCoverageContracts(previous, next)
	if len(merged.RequiredMaterials) != 0 {
		t.Fatalf("RequiredMaterials=%+v, want overridden by next optional role", merged.RequiredMaterials)
	}
	if len(merged.OptionalMaterials) != 1 || merged.OptionalMaterials[0].Path != "lookup.json" {
		t.Fatalf("OptionalMaterials=%+v, want lookup.json", merged.OptionalMaterials)
	}
}

func TestDataTaskPlanStagingGuardChecksContinueAfterTopLevelScript(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:        "ready",
		InputPaths:    []string{"orders.csv"},
		Script:        "print('debug only')",
		ContinueAfter: true,
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{{Path: "orders.csv", Required: true}},
		},
	}
	errText := dataTaskPlanStagingGuardError(plan)
	if !strings.Contains(errText, "script has no result emitter") {
		t.Fatalf("errText=%q, want no-result-emitter guard even when continue_after=true", errText)
	}
}

func TestDataTaskPlanStagingGuardRejectsReadyPlanWithoutExecutableBody(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{{Path: "orders.csv", Required: true}},
		},
	}
	errText := dataTaskPlanStagingGuardError(plan)
	if !strings.Contains(errText, "no executable body") {
		t.Fatalf("errText=%q, want no-executable-body guard", errText)
	}
}

func TestDataTaskCoverageExpansionFallbackCoversReadyPlanWithoutExecutableBody(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv", "rules.md"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			RuleCoverageRequired: true,
		},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	fallback, ok := dataTaskCoverageExpansionFallback(nil, plan, errText)
	if !ok {
		t.Fatal("expected empty ready plan to convert to deterministic coverage batch")
	}
	if !fallback.ContinueAfter {
		t.Fatal("coverage fallback must continue the workflow")
	}
	if got := len(fallback.Actions); got != 2 {
		t.Fatalf("Actions=%+v, want derive_rules plus extract_records", fallback.Actions)
	}
	if fallback.Actions[0].Kind != dataquery.DataActionDeriveRules || fallback.Actions[1].Kind != dataquery.DataActionExtractRecords {
		t.Fatalf("Actions=%+v, want derive_rules then extract_records", fallback.Actions)
	}
}

func TestDataTaskCoverageExpansionFallbackDerivesTextRulesForLedgerTask(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv", "rules.md"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	fallback, ok := dataTaskCoverageExpansionFallback(nil, plan, errText)
	if !ok {
		t.Fatal("expected deterministic coverage expansion fallback")
	}
	if got := fallback.CoverageContract.RuleCoverageRequired; got {
		t.Fatalf("fallback should not silently mutate workflow rule requirement, got %v", got)
	}
	if len(fallback.Actions) != 2 {
		t.Fatalf("Actions=%+v, want derive_rules plus extract_records", fallback.Actions)
	}
	if fallback.Actions[0].Kind != dataquery.DataActionDeriveRules {
		t.Fatalf("first action=%+v, want derive_rules for text constraint material", fallback.Actions[0])
	}
	if got := fallback.Actions[0].InputPaths; len(got) != 1 || got[0] != "rules.md" {
		t.Fatalf("rule InputPaths=%v, want rules.md", got)
	}
}

func TestDataTaskNoEmitterComplexScriptFallbackBuildsAtomicObservationBatch(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv", "queries.csv", "lookup.csv", "amounts.csv"},
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "queries.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "lookup.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Script: `
rows = csv_rows("orders.csv")
print(rows[:2])
`,
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	fallback, _, reason, ok := dataTaskWorkflowDeterministicFallback(nil, plan, errText)
	if !ok {
		t.Fatal("fallback=false, want deterministic no-emitter observation fallback")
	}
	if !strings.Contains(reason, "exploratory script") {
		t.Fatalf("reason=%q, want exploratory script fallback", reason)
	}
	if strings.TrimSpace(fallback.Script) != "" {
		t.Fatalf("fallback script=%q, want empty", fallback.Script)
	}
	if !fallback.ContinueAfter {
		t.Fatal("ContinueAfter=false, want workflow continuation")
	}
	if len(fallback.Actions) != 1 {
		t.Fatalf("Actions=%+v, want one extract_records action", fallback.Actions)
	}
	if fallback.Actions[0].Kind != dataquery.DataActionExtractRecords {
		t.Fatalf("Actions=%+v, want extract_records", fallback.Actions)
	}
	if fallback.OutputContract.Format != dataquery.OutputFreeform || !fallback.OutputContract.ExplanationAllowed {
		t.Fatalf("OutputContract=%+v, want freeform observation batch", fallback.OutputContract)
	}
}

func TestDataTaskCustomTransformDisabledFallbackInspectsGeneratedArtifacts(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				DecisionRecordsRequired:    true,
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
			Actions: []dataquery.DataAction{{
				ID:   "derive",
				Kind: dataquery.DataActionDeriveFields,
			}},
		},
		Result: &dataquery.Result{
			Artifacts: []dataquery.DataArtifact{{
				ID:     "po_enriched",
				Kind:   string(dataquery.DataActionJoinRecords),
				Fields: map[string]string{"json_shape": "array", "artifact_path": "po_enriched.json"},
			}},
			Rows: []dataquery.RowDecision{{RowID: "row1", Decision: "include"}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"po_enriched", "queries.csv"},
		CoverageContract: dataquery.CoverageContract{
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "rebuild",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"po_enriched", "queries.csv"},
			Script:     `emit_result("diagnostic")`,
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	fallback, _, reason, ok := dataTaskWorkflowDeterministicFallback(records, plan, errText)
	if !ok {
		t.Fatal("fallback=false, want generated artifact diagnostic")
	}
	if !strings.Contains(reason, "custom_transform disabled") {
		t.Fatalf("reason=%q, want custom_transform disabled fallback", reason)
	}
	if len(fallback.Actions) != 1 || fallback.Actions[0].Kind != dataquery.DataActionInspectMaterial {
		t.Fatalf("Actions=%+v, want inspect_material", fallback.Actions)
	}
	if got := fallback.Actions[0].InputPaths; len(got) != 1 || got[0] != "po_enriched" {
		t.Fatalf("InputPaths=%v, want generated po_enriched only", got)
	}
	if fallback.Actions[0].Script != "" {
		t.Fatalf("inspect action carried script: %q", fallback.Actions[0].Script)
	}
}

func TestDataTaskCustomTransformDisabledFallbackPrefersLatestRecordArtifact(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				DecisionRecordsRequired:    true,
				ContributionLedgerRequired: true,
				EntityResolutionRequired:   true,
				ReconcileRequired:          true,
			},
			Actions: []dataquery.DataAction{{
				ID:   "apply",
				Kind: dataquery.DataActionApplyResolutions,
			}},
		},
		Result: &dataquery.Result{
			Artifacts: []dataquery.DataArtifact{
				{
					ID:   "old_mapping_a",
					Kind: string(dataquery.DataActionNormalizeEntities),
					Fields: map[string]string{
						"artifact_aliases": "old_mapping_a",
						"json_shape":       "array",
						"fields":           "item_id,source_value,canonical_id,canonical_label,status",
					},
				},
				{
					ID:   "old_mapping_b",
					Kind: string(dataquery.DataActionNormalizeEntities),
					Fields: map[string]string{
						"artifact_aliases": "old_mapping_b",
						"json_shape":       "array",
						"fields":           "item_id,source_value,canonical_id,canonical_label,status",
					},
				},
				{
					ID:      "latest_records",
					Kind:    string(dataquery.DataActionApplyResolutions),
					Headers: []string{"id", "group", "amount", "status", "canonical_id", "canonical_label"},
					Fields: map[string]string{
						"artifact_aliases": "latest_records,latest_records.json",
						"json_shape":       "array(len=3,item=object(keys=id,group,amount,status,canonical_id,canonical_label))",
					},
				},
			},
			Rows: []dataquery.RowDecision{{RowID: "row1", Decision: "include"}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"old_mapping_a", "old_mapping_b"},
		CoverageContract: dataquery.CoverageContract{
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			EntityResolutionRequired:   true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "script_repair",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"old_mapping_a", "old_mapping_b"},
			Script:     `emit_result("diagnostic")`,
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	fallback, _, reason, ok := dataTaskWorkflowDeterministicFallback(records, plan, errText)
	if !ok {
		t.Fatal("fallback=false, want generated artifact diagnostic")
	}
	if !strings.Contains(reason, "custom_transform disabled") {
		t.Fatalf("reason=%q, want custom_transform disabled fallback", reason)
	}
	if got := fallback.Actions[0].InputPaths; len(got) == 0 || got[0] != "latest_records" {
		t.Fatalf("InputPaths=%v, want latest rich record artifact first", got)
	}
}

func TestDataTaskCustomTransformDisabledFallbackSkipsRuleArtifacts(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				DecisionRecordsRequired:    true,
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
			Actions: []dataquery.DataAction{{ID: "rules", Kind: dataquery.DataActionDeriveRules}},
		},
		Result: &dataquery.Result{
			Artifacts: []dataquery.DataArtifact{
				{
					ID:   "rules_artifact",
					Kind: string(dataquery.DataActionDeriveRules),
					Fields: map[string]string{
						"artifact_aliases": "rules_artifact",
						"json_shape":       "array(len=3,item=object(keys=rule_id,rule_text,status))",
					},
					Children: []dataquery.DataArtifact{
						{
							ID:   "rule_1",
							Kind: string(dataquery.DataActionDeriveRules),
							Fields: map[string]string{
								"artifact_aliases": "rule_1",
								"json_shape":       "object(keys=rule_id,rule_text,status)",
								"fields":           "rule_id,rule_text,status",
							},
						},
					},
				},
				{
					ID:      "po_enriched",
					Kind:    string(dataquery.DataActionApplyResolutions),
					Headers: []string{"po_id", "vendor_id", "category_code", "amount"},
					Fields: map[string]string{
						"artifact_aliases": "po_enriched,po_enriched.json",
						"json_shape":       "array(len=50,item=object(keys=po_id,vendor_id,category_code,amount))",
					},
				},
			},
			Rows: []dataquery.RowDecision{{RowID: "row1", Decision: "include"}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"rule_1", "po_enriched"},
		CoverageContract: dataquery.CoverageContract{
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "script_repair",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"rule_1", "po_enriched"},
			Script:     `emit_result("diagnostic")`,
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	fallback, _, reason, ok := dataTaskWorkflowDeterministicFallback(records, plan, errText)
	if !ok {
		t.Fatal("fallback=false, want generated record diagnostic")
	}
	if !strings.Contains(reason, "custom_transform disabled") {
		t.Fatalf("reason=%q, want custom_transform disabled fallback", reason)
	}
	got := fallback.Actions[0].InputPaths
	if len(got) != 1 || got[0] != "po_enriched" {
		t.Fatalf("InputPaths=%v, want only executable record artifact po_enriched", got)
	}
}

func TestDataTaskCustomTransformDisabledFallbackUsesConcreteNormalizeScaffold(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				DecisionRecordsRequired:    true,
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
			Actions: []dataquery.DataAction{{ID: "extract", Kind: dataquery.DataActionExtractRecords}},
		},
		Result: &dataquery.Result{
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "orders",
					Kind:    string(dataquery.DataActionExtractRecords),
					Headers: []string{"order_id", "vendor_name", "amount"},
					Fields: map[string]string{
						"artifact_aliases": "orders,orders.json",
						"json_shape":       "array(len=2,item=object(keys=order_id,vendor_name,amount))",
						"output_headers":   "order_id,vendor_name,amount",
					},
				},
				{
					ID:      "vendors",
					Kind:    string(dataquery.DataActionExtractRecords),
					Headers: []string{"vendor_id", "vendor_name"},
					Fields: map[string]string{
						"artifact_aliases": "vendors,vendors.json",
						"json_shape":       "array(len=2,item=object(keys=vendor_id,vendor_name))",
						"output_headers":   "vendor_id,vendor_name",
					},
				},
			},
			Rows: []dataquery.RowDecision{{RowID: "row1", Decision: "include"}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "script_repair",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"orders", "vendors"},
			Script:     `emit_result("diagnostic")`,
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	fallback, _, reason, ok := dataTaskWorkflowDeterministicFallback(records, plan, errText)
	if !ok {
		t.Fatal("fallback=false, want concrete normalize scaffold fallback")
	}
	if !strings.Contains(reason, "custom_transform disabled") {
		t.Fatalf("reason=%q, want custom_transform disabled fallback", reason)
	}
	if len(fallback.Actions) != 1 {
		t.Fatalf("Actions=%+v, want one concrete action", fallback.Actions)
	}
	action := fallback.Actions[0]
	if action.Kind != dataquery.DataActionNormalizeEntities {
		t.Fatalf("Action kind=%s, want normalize_entities; action=%+v", action.Kind, action)
	}
	if got := action.Params["match_mode"]; got != "exact" {
		t.Fatalf("match_mode=%q, want executable exact default", got)
	}
	for _, value := range action.Params {
		if strings.Contains(value, "<") || strings.Contains(value, "|") {
			t.Fatalf("param value %q is not concrete", value)
		}
	}
	if strings.TrimSpace(action.OutputArtifact) == "" {
		t.Fatalf("OutputArtifact empty for action=%+v", action)
	}
	if fallback.Actions[0].Kind == dataquery.DataActionInspectMaterial {
		t.Fatal("fallback used inspect_material instead of concrete scaffold")
	}
}

func TestDataTaskCustomTransformDisabledFallbackAppliesExistingResolutionScaffold(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				DecisionRecordsRequired:    true,
				ContributionLedgerRequired: true,
				EntityResolutionRequired:   true,
				ReconcileRequired:          true,
			},
			Actions: []dataquery.DataAction{{ID: "normalize", Kind: dataquery.DataActionNormalizeEntities}},
		},
		Result: &dataquery.Result{
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "orders",
					Kind:    string(dataquery.DataActionExtractRecords),
					Headers: []string{"row_id", "name_raw", "amount"},
					Fields: map[string]string{
						"artifact_aliases": "orders,orders.json",
						"json_shape":       "array(len=2,item=object(keys=row_id,name_raw,amount))",
						"output_headers":   "row_id,name_raw,amount",
					},
				},
				{
					ID:      "vendor_resolutions",
					Kind:    string(dataquery.DataActionNormalizeEntities),
					Headers: []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
					Fields: map[string]string{
						"artifact_aliases": "vendor_resolutions,vendor_resolutions.json",
						"json_shape":       "array(len=2,item=object(keys=item_id,source_value,canonical_id,canonical_label,status))",
						"output_headers":   "item_id,source_value,canonical_id,canonical_label,status",
					},
				},
			},
			EntityResolutions: []dataquery.EntityResolutionRecord{{
				ItemID:         dataquery.LooseText("orders#1:name_raw"),
				SourceValue:    dataquery.LooseText("raw"),
				CanonicalID:    dataquery.LooseText("canonical"),
				CanonicalLabel: dataquery.LooseText("Canonical"),
				Status:         dataquery.LooseText("resolved"),
			}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			EntityResolutionRequired:   true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "script_repair",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"orders", "vendor_resolutions"},
			Script:     `emit_result("diagnostic")`,
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	fallback, _, reason, ok := dataTaskWorkflowDeterministicFallback(records, plan, errText)
	if !ok {
		t.Fatal("fallback=false, want concrete apply_entity_resolutions scaffold fallback")
	}
	if !strings.Contains(reason, "custom_transform disabled") {
		t.Fatalf("reason=%q, want custom_transform disabled fallback", reason)
	}
	if len(fallback.Actions) != 1 {
		t.Fatalf("Actions=%+v, want one concrete action", fallback.Actions)
	}
	action := fallback.Actions[0]
	if action.Kind != dataquery.DataActionApplyResolutions {
		state := dataTaskWorkflowState(records, plan)
		t.Fatalf("Action kind=%s, want apply_entity_resolutions; action=%+v state_next=%s allowed=%v scaffolds=%+v",
			action.Kind, action, state.NextStage, state.AllowedNextActions, state.ActionScaffold)
	}
	if got := action.Params["base_path"]; got != "orders" {
		t.Fatalf("base_path=%q, want orders", got)
	}
	if got := action.Params["preserve_base_rows"]; got != "true" {
		t.Fatalf("preserve_base_rows=%q, want true", got)
	}
	spec := action.Params["resolution_specs"]
	for _, want := range []string{"vendor_canonical_id", "vendor_canonical_label", "vendor_resolution_status"} {
		if !strings.Contains(spec, want) {
			t.Fatalf("resolution_specs=%s, want %s", spec, want)
		}
	}
	for _, value := range action.Params {
		if strings.Contains(value, "<") || strings.Contains(value, "|") {
			t.Fatalf("param value %q is not concrete", value)
		}
	}
}

func TestDataTaskWorkflowStagingGuardRejectsMultipleCustomTransformsInOneBatch(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{
			{
				ID:     "transform_a",
				Kind:   dataquery.DataActionCustomTransform,
				Script: `emit_result("1", output_contract={"format":"plain_single_line","explanation_allowed":False})`,
			},
			{
				ID:     "transform_b",
				Kind:   dataquery.DataActionCustomTransform,
				Script: `emit_result("2", output_contract={"format":"plain_single_line","explanation_allowed":False})`,
			},
		},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "at most one bounded custom_transform") {
		t.Fatalf("errText=%q, want multiple custom transform guard", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsOversizedActionBatch(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{
			{ID: "a1", Kind: dataquery.DataActionInspectMaterial, InputPaths: []string{"a.csv"}},
			{ID: "a2", Kind: dataquery.DataActionInspectMaterial, InputPaths: []string{"b.csv"}},
			{ID: "a3", Kind: dataquery.DataActionExtractRecords, InputPaths: []string{"c.csv"}},
			{ID: "a4", Kind: dataquery.DataActionDeriveRules, InputPaths: []string{"rules.md"}},
			{ID: "a5", Kind: dataquery.DataActionNormalizeEntities, InputPaths: []string{"lookup.csv"}},
		},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "above the atomic batch limit") {
		t.Fatalf("errText=%q, want oversized action batch guard", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsReconcileWithoutContributionProducer(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{
			{ID: "rules", Kind: dataquery.DataActionDeriveRules, InputPaths: []string{"rules.md"}},
			{ID: "reconcile", Kind: dataquery.DataActionReconcile},
		},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "requires contribution records") {
		t.Fatalf("errText=%q, want reconcile prerequisite guard", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsEmptyExtractRecordsAction(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:   "read_rules",
			Kind: dataquery.DataActionExtractRecords,
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "requires input_paths") {
		t.Fatalf("errText=%q, want input_paths guard", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsDeriveFieldsWithoutSpec(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "derive",
			Kind:       dataquery.DataActionDeriveFields,
			InputPaths: []string{"records.csv"},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "derive_fields") || !strings.Contains(errText, "field specification") {
		t.Fatalf("errText=%q, want derive_fields field-spec guard", errText)
	}

	plan.Actions[0].Params = map[string]string{
		"field_specs_json": `[{"source_field":"value","target_field":"value_number","operation":"parse_number"}]`,
	}
	if errText := dataTaskWorkflowStagingGuardError(nil, plan); errText != "" {
		t.Fatalf("errText=%q, want derive_fields with spec to pass", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsDeriveFieldsWithMultipleInputs(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "derive",
			Kind:       dataquery.DataActionDeriveFields,
			InputPaths: []string{"records.csv", "lookup.csv"},
			Params: map[string]string{
				"field_specs_json": `[{"source_field":"value","target_field":"value_number","operation":"parse_number"}]`,
			},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "single-record-set") || !strings.Contains(errText, "2 input_paths") {
		t.Fatalf("errText=%q, want derive_fields multi-input guard", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsExtractFieldsWithoutSpec(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "extract",
			Kind:       dataquery.DataActionExtractFields,
			InputPaths: []string{"records.json"},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "extract_fields") || !strings.Contains(errText, "field specification") {
		t.Fatalf("errText=%q, want extract_fields field-spec guard", errText)
	}

	plan.Actions[0].Params = map[string]string{
		"extract_specs": `[{"source_field":"text","target_field":"value","operation":"regex_extract","pattern":"value=(\\d+)"}]`,
	}
	if errText := dataTaskWorkflowStagingGuardError(nil, plan); errText != "" {
		t.Fatalf("errText=%q, want extract_fields with spec to pass", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsGroupRecordsWithoutSpec(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "group",
			Kind:       dataquery.DataActionGroupRecords,
			InputPaths: []string{"records.json"},
			Params: map[string]string{
				"text_fields": `["text"]`,
			},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "group_records") || !strings.Contains(errText, "grouping/text specification") {
		t.Fatalf("errText=%q, want group_records grouping guard", errText)
	}

	plan.Actions[0].Params["group_field"] = "doc_id"
	if errText := dataTaskWorkflowStagingGuardError(nil, plan); errText != "" {
		t.Fatalf("errText=%q, want group_records with group/text spec to pass", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsJoinRecordsWithThreeInputs(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "join_three",
			Kind:       dataquery.DataActionJoinRecords,
			InputPaths: []string{"a.json", "b.json", "c.json"},
			Params: map[string]string{
				"left_fields":  `["id"]`,
				"right_fields": `["id"]`,
			},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "at most 2") || !strings.Contains(errText, "atomic DAG ranks") {
		t.Fatalf("errText=%q, want exact join arity guard", errText)
	}
}

func TestDataTaskWorkflowStagingRelationFieldGuardsCarryTypedPayloads(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "orders",
					Kind:    string(dataquery.DataActionExtractRecords),
					Headers: []string{"order_id", "raw_category"},
					Fields:  map[string]string{"artifact_aliases": "orders", "json_shape": "array(len=2,item=object(keys=order_id,raw_category))"},
				},
				{
					ID:      "reference",
					Kind:    string(dataquery.DataActionExtractRecords),
					Headers: []string{"category_code", "category_name"},
					Fields:  map[string]string{"artifact_aliases": "reference", "json_shape": "array(len=2,item=object(keys=category_code,category_name))"},
				},
			},
		},
	}}
	cases := []struct {
		name         string
		action       dataquery.DataAction
		wantInput    string
		wantMissing  string
		wantActionID string
	}{
		{
			name: "join",
			action: dataquery.DataAction{
				ID:         "join_missing",
				Kind:       dataquery.DataActionJoinRecords,
				InputPaths: []string{"orders", "reference"},
				Params: map[string]string{
					"left_fields":  `["missing_key"]`,
					"right_fields": `["category_code"]`,
				},
			},
			wantInput:    "orders",
			wantMissing:  "missing_key",
			wantActionID: "join_missing",
		},
		{
			name: "enrich",
			action: dataquery.DataAction{
				ID:         "enrich_missing",
				Kind:       dataquery.DataActionEnrichRecords,
				InputPaths: []string{"orders", "reference"},
				Params: map[string]string{
					"base_path": "orders",
					"lookup_specs_json": `[{
						"lookup_path":"reference",
						"base_fields":["raw_category"],
						"lookup_fields":["missing_lookup"],
						"mapping_value_field":"category_code",
						"target_field":"category_code"
					}]`,
				},
			},
			wantInput:    "reference",
			wantMissing:  "missing_lookup",
			wantActionID: "enrich_missing",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			guard := dataTaskWorkflowStagingGuardResult(records, dataquery.TaskPlan{
				Status:  "ready",
				Actions: []dataquery.DataAction{tc.action},
			})
			if guard.Code != "field_contract_violation" || len(guard.Violations) != 1 {
				t.Fatalf("guard=%+v, want typed field-contract violation", guard)
			}
			violation := guard.Violations[0]
			if violation.ActionID != tc.wantActionID || violation.InputAlias != tc.wantInput || !slices.Contains(violation.MissingFields, tc.wantMissing) {
				t.Fatalf("violation=%+v, want action=%s input=%s missing=%s", violation, tc.wantActionID, tc.wantInput, tc.wantMissing)
			}
		})
	}
}

func TestDataTaskWorkflowStateProjectsFieldContractViolations(t *testing.T) {
	records := []dataTaskWorkflowRecord{
		{
			Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
				ID:      "po_with_year",
				Kind:    string(dataquery.DataActionDeriveFields),
				Headers: []string{"po_id", "year", "status", "currency"},
				Fields:  map[string]string{"json_shape": "array[object]"},
			}}},
		},
		{
			Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
				ID:         "filter_eligible",
				Kind:       dataquery.DataActionFilterRecords,
				InputPaths: []string{"po_with_year"},
			}}},
			Err: `data planning incomplete: action 1 (filter_eligible) references field(s) [attachment_currency] that are not present on input po_with_year fields [po_id, year, status, currency]. Use an existing artifact from workflow_state_json.artifact_availability, or first materialize the missing field(s) with derive_fields, extract_fields, enrich_records, join_records, or a valid prior typed action before consuming them.`,
		},
	}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{CoverageContract: dataquery.CoverageContract{ContributionLedgerRequired: true}})
	if len(state.FieldContractViolations) != 1 {
		t.Fatalf("FieldContractViolations=%+v, want one violation", state.FieldContractViolations)
	}
	got := state.FieldContractViolations[0]
	if got.ActionID != "filter_eligible" || got.ActionKind != string(dataquery.DataActionFilterRecords) || got.InputAlias != "po_with_year" {
		t.Fatalf("violation=%+v, want typed action/input identity", got)
	}
	if strings.Join(got.MissingFields, ",") != "attachment_currency" || !strings.Contains(strings.Join(got.AvailableFieldSample, ","), "currency") {
		t.Fatalf("violation=%+v, want missing and available fields", got)
	}
	if len(got.RepairActionHints) == 0 {
		t.Fatalf("violation=%+v, want repair hints from allowed next actions", got)
	}
}

func TestDataTaskWorkflowStateProjectsExtractFieldsZeroMatchViolation(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:      "amount_extract",
			Kind:    string(dataquery.DataActionExtractFields),
			Headers: []string{"id", "text", "amount"},
			Fields: map[string]string{
				"input_path":              "documents",
				"input_rows":              "3",
				"output_rows":             "0",
				"source_fields":           "id,text,_source_locator",
				"extracted_fields":        "amount",
				"required_fields":         "amount",
				"zero_match_diagnostics":  `{"input_rows":3,"output_rows":0,"required_fields":["amount"]}`,
				"source_field_samples":    `{"text":["total: 123"]}`,
				"required_missing_counts": `{"amount":3}`,
			},
		}}},
	}}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{CoverageContract: dataquery.CoverageContract{
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	}})
	if len(state.FieldContractViolations) != 1 {
		t.Fatalf("FieldContractViolations=%+v, want one extract violation", state.FieldContractViolations)
	}
	got := state.FieldContractViolations[0]
	if got.ActionKind != string(dataquery.DataActionExtractFields) || got.ArtifactID != "amount_extract" || got.InputAlias != "documents" {
		t.Fatalf("violation=%+v, want extract artifact identity", got)
	}
	if !slices.Equal(got.MissingFields, []string{"amount"}) || !strings.Contains(got.Reason, "source_field_samples") {
		t.Fatalf("violation=%+v, want missing amount with samples", got)
	}
	if len(got.RepairActionHints) == 0 || !strings.Contains(strings.Join(got.RepairActionHints, " "), "source_field") {
		t.Fatalf("violation=%+v, want extraction repair hints", got)
	}
}

func TestDataTaskWorkflowStateProjectsZeroMatchFilterViolations(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:      "po_filtered",
			Kind:    string(dataquery.DataActionFilterRecords),
			Headers: []string{"po_id", "year", "status_norm"},
			Fields: map[string]string{
				"artifact_aliases":    "po_filtered,po_filtered.json",
				"input_path":          "po_qualified_base",
				"input_rows":          "50",
				"output_rows":         "0",
				"filter_fields":       "year,status_norm",
				"filter_diagnostics":  `{"total":50,"combined_match":0,"filters":[{"field":"year","matched":50},{"field":"status_norm","matched":0,"sample_values":["paid","completed"]}]}`,
				"json_shape":          "object(keys=diagnostics,headers,records)",
				"record_completeness": "complete",
			},
		}}},
	}}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{CoverageContract: dataquery.CoverageContract{
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	}})
	if len(state.ZeroMatchFilterViolations) != 1 {
		t.Fatalf("ZeroMatchFilterViolations=%+v, want one violation", state.ZeroMatchFilterViolations)
	}
	got := state.ZeroMatchFilterViolations[0]
	if got.ArtifactID != "po_filtered" || got.InputPath != "po_qualified_base" || got.InputRows != 50 || got.OutputRows != 0 {
		t.Fatalf("violation=%+v, want zero-match filter artifact identity and counts", got)
	}
	if !strings.Contains(got.FilterDiagnostics, "combined_match") || len(got.RepairActionHints) == 0 {
		t.Fatalf("violation=%+v, want diagnostics and repair hints", got)
	}
	if len(state.WorkflowViolations) == 0 || state.WorkflowViolations[0].Code != "zero_match_filter" || state.WorkflowViolations[0].IdempotencyKey == "" {
		t.Fatalf("WorkflowViolations=%+v, want typed zero-match violation with idempotency", state.WorkflowViolations)
	}
	if len(state.ActionGraph.Blocked) == 0 || state.ActionGraph.Blocked[0].Status != dataworkflow.ActionStatusBlocked || state.ActionGraph.Blocked[0].DependencyRank == 0 {
		t.Fatalf("blocked graph=%+v, want typed blocked zero-match node", state.ActionGraph.Blocked)
	}
}

func TestDataTaskWorkflowStateSuppressesStaleZeroMatchFilterViolation(t *testing.T) {
	zeroArtifact := dataquery.DataArtifact{
		ID:      "po_eligible.json",
		Kind:    string(dataquery.DataActionFilterRecords),
		Headers: []string{"po_id", "query_id", "amount"},
		Fields: map[string]string{
			"artifact_aliases":   "po_eligible,po_eligible.json",
			"input_path":         "po_old",
			"input_rows":         "50",
			"output_rows":        "0",
			"filter_fields":      "status",
			"filter_diagnostics": `{"total":50,"combined_match":0}`,
		},
	}
	positiveArtifact := dataquery.DataArtifact{
		ID:      "po_eligible.json",
		Kind:    string(dataquery.DataActionFilterRecords),
		Headers: []string{"po_id", "query_id", "amount"},
		Fields: map[string]string{
			"artifact_aliases":   "po_eligible,po_eligible.json",
			"input_path":         "po_fixed",
			"input_rows":         "50",
			"output_rows":        "49",
			"filter_fields":      "status",
			"filter_diagnostics": `{"total":50,"combined_match":49}`,
		},
		RowCount: 49,
	}
	records := []dataTaskWorkflowRecord{
		{Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{zeroArtifact}}},
		{Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{positiveArtifact}}},
	}
	contract := dataquery.CoverageContract{ContributionLedgerRequired: true, ReconcileRequired: true}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{CoverageContract: contract})
	if len(state.ZeroMatchFilterViolations) != 0 {
		t.Fatalf("ZeroMatchFilterViolations=%+v, want stale zero-match alias suppressed by newer non-empty artifact", state.ZeroMatchFilterViolations)
	}
	plan := dataquery.TaskPlan{
		Status:           "ready",
		CoverageContract: contract,
		Actions: []dataquery.DataAction{{
			ID:         "join_nonempty",
			Kind:       dataquery.DataActionJoinRecords,
			InputPaths: []string{"po_eligible", "queries"},
			Params: map[string]string{
				"left_fields":  `["query_id"]`,
				"right_fields": `["query_id"]`,
			},
		}},
	}
	records = append(records, dataTaskWorkflowRecord{Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
		ID:      "queries",
		Kind:    string(dataquery.DataActionExtractRecords),
		Headers: []string{"query_id"},
		Fields:  map[string]string{"artifact_aliases": "queries"},
	}}}})
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if strings.Contains(errText, "zero-match filter artifact") {
		t.Fatalf("errText=%q, want stale zero-match guard suppressed", errText)
	}
}

func TestDataTaskWorkflowStagingGuardBlocksZeroMatchFilterInputs(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{
			{
				ID:      "queries",
				Kind:    string(dataquery.DataActionExtractRecords),
				Headers: []string{"query_id"},
				Fields:  map[string]string{"artifact_aliases": "queries"},
			},
			{
				ID:      "po_filtered",
				Kind:    string(dataquery.DataActionFilterRecords),
				Headers: []string{"po_id", "query_id", "amount"},
				Fields: map[string]string{
					"artifact_aliases":   "po_filtered,po_filtered.json",
					"input_path":         "po_qualified_base",
					"input_rows":         "50",
					"output_rows":        "0",
					"filter_fields":      "year,status_norm",
					"filter_diagnostics": `{"total":50,"combined_match":0}`,
				},
			},
		}},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "join_empty",
			Kind:       dataquery.DataActionJoinRecords,
			InputPaths: []string{"po_filtered", "queries"},
			Params: map[string]string{
				"left_fields":  `["query_id"]`,
				"right_fields": `["query_id"]`,
			},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if !strings.Contains(errText, "zero-match filter artifact") || !strings.Contains(errText, "po_qualified_base") {
		t.Fatalf("errText=%q, want zero-match input guard", errText)
	}
	guard := dataTaskWorkflowStagingGuardResult(records, plan)
	if guard.Code != "zero_match_filter" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want typed zero-match guard with violation payload", guard)
	}
	if guard.Violations[0].ActionID != "join_empty" || guard.Violations[0].InputAlias != "po_filtered" {
		t.Fatalf("violation=%+v, want consuming action and blocked input alias", guard.Violations[0])
	}
}

func TestDataTaskWorkflowStateProjectsUnmatchedResolutionViolations(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:      "records_resolved",
			Kind:    string(dataquery.DataActionApplyResolutions),
			Headers: []string{"id", "canonical_id", "canonical_id_status"},
			Fields: map[string]string{
				"artifact_aliases":       "records_resolved,records_resolved.json",
				"base_path":              "records",
				"base_rows":              "12",
				"output_rows":            "12",
				"target_fields":          "canonical_id",
				"matched_canonical_id":   "0",
				"unmatched_canonical_id": "12",
				"json_shape":             "array(len=12,item=object(keys=id,canonical_id,canonical_id_status))",
			},
		}}},
	}}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{CoverageContract: dataquery.CoverageContract{
		EntityResolutionRequired:   true,
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	}})
	if len(state.UnmatchedResolutionViolations) != 1 {
		t.Fatalf("UnmatchedResolutionViolations=%+v, want one violation", state.UnmatchedResolutionViolations)
	}
	got := state.UnmatchedResolutionViolations[0]
	if got.ArtifactID != "records_resolved" || got.BaseRows != 12 || strings.Join(got.TargetFields, ",") != "canonical_id" {
		t.Fatalf("violation=%+v, want resolution artifact identity and target field", got)
	}
	if len(state.WorkflowViolations) == 0 || state.WorkflowViolations[0].Code != "unmatched_resolution" || state.WorkflowViolations[0].IdempotencyKey == "" {
		t.Fatalf("WorkflowViolations=%+v, want typed unmatched-resolution violation with idempotency", state.WorkflowViolations)
	}
}

func TestDataTaskWorkflowStagingGuardBlocksUnmatchedResolutionInputs(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:      "records_resolved",
			Kind:    string(dataquery.DataActionApplyResolutions),
			Headers: []string{"id", "canonical_id", "canonical_id_status"},
			Fields: map[string]string{
				"artifact_aliases":       "records_resolved,records_resolved.json",
				"base_path":              "records",
				"base_rows":              "12",
				"output_rows":            "12",
				"target_fields":          "canonical_id",
				"matched_canonical_id":   "0",
				"unmatched_canonical_id": "12",
				"json_shape":             "array(len=12,item=object(keys=id,canonical_id,canonical_id_status))",
			},
		}}},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			EntityResolutionRequired:   true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "qualify_bad_resolution",
			Kind:       dataquery.DataActionFilterRecords,
			InputPaths: []string{"records_resolved"},
			Params: map[string]string{
				"filters_json": `[{"field":"canonical_id_status","op":"eq","value":"resolved"}]`,
			},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if !strings.Contains(errText, "all-unmatched resolution artifact") || !strings.Contains(errText, "apply_entity_resolutions") {
		t.Fatalf("errText=%q, want all-unmatched resolution guard", errText)
	}
	guard := dataTaskWorkflowStagingGuardResult(records, plan)
	if guard.Code != "unmatched_resolution" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want typed unmatched-resolution guard with violation payload", guard)
	}
	if guard.Violations[0].ActionID != "qualify_bad_resolution" || guard.Violations[0].InputAlias != "records_resolved" {
		t.Fatalf("violation=%+v, want consuming action and blocked input alias", guard.Violations[0])
	}
}

func TestDataTaskWorkflowStateProjectsZeroEligibleQualificationViolations(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:      "qualified",
			Kind:    string(dataquery.DataActionQualifyRecords),
			Headers: []string{"id", "eligible"},
			Fields: map[string]string{
				"artifact_aliases":      "qualified,qualified.json",
				"input_path":            "records",
				"input_rows":            "12",
				"eligible_rows":         "0",
				"include_filters":       `{"total":12,"combined_match":0}`,
				"qualification_reasons": `{"include_filter_failed":12}`,
				"json_shape":            "object(keys=diagnostics,headers,records)",
			},
		}}},
	}}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{CoverageContract: dataquery.CoverageContract{
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	}})
	if len(state.ZeroEligibleViolations) != 1 {
		t.Fatalf("ZeroEligibleViolations=%+v, want one violation", state.ZeroEligibleViolations)
	}
	got := state.ZeroEligibleViolations[0]
	if got.ArtifactID != "qualified" || got.InputRows != 12 || got.EligibleRows != 0 {
		t.Fatalf("violation=%+v, want zero-eligible artifact identity and counts", got)
	}
}

func TestDataTaskWorkflowStagingGuardBlocksZeroEligibleInputs(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:      "qualified",
			Kind:    string(dataquery.DataActionQualifyRecords),
			Headers: []string{"id", "query_id", "amount"},
			Fields: map[string]string{
				"artifact_aliases":      "qualified,qualified.json",
				"input_path":            "records",
				"input_rows":            "12",
				"eligible_rows":         "0",
				"include_filters":       `{"total":12,"combined_match":0}`,
				"qualification_reasons": `{"include_filter_failed":12}`,
				"json_shape":            "object(keys=diagnostics,headers,records)",
			},
		}}},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "compute_empty",
			Kind:       dataquery.DataActionComputeContribs,
			InputPaths: []string{"qualified"},
			Params: map[string]string{
				"group_key_field": "query_id",
				"value_field":     "amount",
			},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if !strings.Contains(errText, "zero-eligible qualification artifact") || !strings.Contains(errText, "compute_contributions") {
		t.Fatalf("errText=%q, want zero-eligible guard", errText)
	}
	guard := dataTaskWorkflowStagingGuardResult(records, plan)
	if guard.Code != "zero_eligible_records" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want typed zero-eligible guard with violation payload", guard)
	}
	if guard.Violations[0].ActionID != "compute_empty" || guard.Violations[0].InputAlias != "qualified" {
		t.Fatalf("violation=%+v, want consuming action and blocked input alias", guard.Violations[0])
	}
}

func TestDataTaskStagingGuardRejectsScriptOnTypedAction(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "enrich_with_script",
			Kind:       dataquery.DataActionEnrichRecords,
			InputPaths: []string{"base.json", "mapping.csv"},
			Script:     `emit({"answer":"bad"})`,
		}},
	}
	errText := dataTaskPlanStagingGuardError(plan)
	if !strings.Contains(errText, "typed data action") || !strings.Contains(errText, "Only custom_transform may carry a script") {
		t.Fatalf("errText=%q, want typed-action script guard", errText)
	}
	errText = dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "typed data action") || !strings.Contains(errText, "Only custom_transform may carry a script") {
		t.Fatalf("workflow errText=%q, want typed-action script guard", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsExpandRecordsWithoutSpec(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "expand",
			Kind:       dataquery.DataActionExpandRecords,
			InputPaths: []string{"records.csv"},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "expand_records") || !strings.Contains(errText, "source_field") {
		t.Fatalf("errText=%q, want expand_records source-field guard", errText)
	}

	plan.Actions[0].Params = map[string]string{
		"source_field": "tags",
		"target_field": "tag",
	}
	if errText := dataTaskWorkflowStagingGuardError(nil, plan); errText != "" {
		t.Fatalf("errText=%q, want expand_records with source_field to pass", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsExpandRecordsWithMultipleInputs(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "expand",
			Kind:       dataquery.DataActionExpandRecords,
			InputPaths: []string{"records.csv", "lookup.csv"},
			Params: map[string]string{
				"source_field": "tags",
				"target_field": "tag",
			},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "single-record-set") || !strings.Contains(errText, "2 input_paths") {
		t.Fatalf("errText=%q, want expand_records multi-input guard", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsBroadCustomTransformRepairPlan(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		InputPaths:    []string{"a.csv", "b.csv", "c.csv", "d.csv", "rules.md"},
		CoverageContract: dataquery.CoverageContract{
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:             "repair_whole_workflow",
			Kind:           dataquery.DataActionCustomTransform,
			InputPaths:     []string{"a.csv", "b.csv", "c.csv", "d.csv", "rules.md"},
			OutputArtifact: "result",
			Script:         strings.Repeat("x = 1\n", dataTaskComplexCustomScriptLineLimit) + "emit({'answer': '0'})\n",
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "too broad") || !strings.Contains(errText, "custom_transform") {
		t.Fatalf("errText=%q, want broad custom_transform guard", errText)
	}
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: plan.Actions[0].InputPaths,
		},
	}}
	errText = dataTaskWorkflowStagingGuardError(records, plan)
	if !strings.Contains(errText, "custom_transform") ||
		(!strings.Contains(errText, "allowed_next_actions") && !strings.Contains(errText, "too broad")) {
		t.Fatalf("errText=%q, want covered-input whole-workflow custom_transform guard", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsCoverageOnlyAfterCoverageSufficient(t *testing.T) {
	required := []dataquery.CoverageMaterial{
		{Path: "orders.csv", Required: true},
		{Path: "rules.md", Required: true},
	}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{CoverageContract: dataquery.CoverageContract{RequiredMaterials: required}},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "rules.md"},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: required,
		},
		Actions: []dataquery.DataAction{{
			ID:         "inspect_again",
			Kind:       dataquery.DataActionInspectMaterial,
			InputPaths: []string{"orders.csv"},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if !strings.Contains(errText, "material coverage is already sufficient") {
		t.Fatalf("errText=%q, want coverage-loop guard", errText)
	}
}

func TestDataTaskWorkflowStagingAllowsRecordMaterializationAfterCoverageSufficient(t *testing.T) {
	required := []dataquery.CoverageMaterial{
		{Path: "orders.csv", Required: true},
	}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{CoverageContract: dataquery.CoverageContract{RequiredMaterials: required}},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv"},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials:          required,
			ContributionLedgerRequired: true,
		},
		Actions: []dataquery.DataAction{{
			ID:             "extract_records_for_compute",
			Kind:           dataquery.DataActionExtractRecords,
			InputPaths:     []string{"orders.csv"},
			OutputArtifact: "orders_record_set",
		}},
		ContinueAfter: true,
	}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); strings.Contains(errText, "material coverage is already sufficient") {
		t.Fatalf("errText=%q, record materialization should not be treated as coverage-only", errText)
	}
}

func TestDataTaskWorkflowStagingGuardAllowsGeneratedArtifactDiagnostics(t *testing.T) {
	required := []dataquery.CoverageMaterial{
		{Path: "orders.csv", Required: true},
		{Path: "rules.md", Required: true},
	}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{CoverageContract: dataquery.CoverageContract{RequiredMaterials: required}},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "rules.md"},
			Artifacts: []dataquery.DataArtifact{{
				ID:   "orders_clean",
				Kind: string(dataquery.DataActionDeriveFields),
			}},
			RuleCoverage: []dataquery.RuleCoverageRecord{{RuleID: dataquery.LooseText("r1"), Status: dataquery.LooseText("applied")}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials:          required,
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "inspect_generated",
			Kind:       dataquery.DataActionInspectMaterial,
			InputPaths: []string{"orders_clean.json"},
		}},
		ContinueAfter: true,
	}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("errText=%q, want generated artifact diagnostic inspect allowed", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsCrossStageCustomTransform(t *testing.T) {
	required := []dataquery.CoverageMaterial{
		{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
		{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
	}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials:          required,
				RuleCoverageRequired:       true,
				EntityResolutionRequired:   true,
				DecisionRecordsRequired:    true,
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"records.csv", "rules.md"},
			RuleCoverage:  []dataquery.RuleCoverageRecord{{RuleID: dataquery.LooseText("r1"), Status: dataquery.LooseText("applied")}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials:          required,
			RuleCoverageRequired:       true,
			EntityResolutionRequired:   true,
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "all_in_one",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"records.csv", "rules.md"},
			Script:     `emit_result("42", output_contract={"format":"plain_single_line","explanation_allowed":False}, contributions=[{"item_id":"i1","source":"records.csv","source_locator":"row=1","group_key":"g","metric":"m","value":"42","operation":"add"}], reconcile={"status":"pass"})`,
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if !strings.Contains(errText, "allowed_next_actions") || strings.Contains(errText, "custom_transform]") {
		t.Fatalf("errText=%q, want typed-stage allowed_next_actions guard", errText)
	}

	plan.ContinueAfter = true
	plan.Actions = []dataquery.DataAction{{
		ID:             "classify",
		Kind:           dataquery.DataActionCustomTransform,
		InputPaths:     []string{"records.csv", "rules.md"},
		OutputArtifact: "classified_records.json",
		Script:         `emit({"artifact":"classified_records","rows":[{"id":"i1"}]})`,
	}}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); !strings.Contains(errText, "allowed_next_actions") || strings.Contains(errText, "custom_transform]") {
		t.Fatalf("errText=%q, want single intermediate custom_transform rejected by typed-stage guard", errText)
	}

	narrowPlan := plan
	narrowPlan.CoverageContract = dataquery.CoverageContract{
		RequiredMaterials:    required,
		RuleCoverageRequired: true,
	}
	narrowPlan.Actions = []dataquery.DataAction{{
		ID:             "classify",
		Kind:           dataquery.DataActionCustomTransform,
		InputPaths:     []string{"records.csv", "rules.md"},
		OutputArtifact: "classified_records.json",
		Script:         `emit({"artifact":"classified_records","rows":[{"id":"i1"}]})`,
	}}
	if errText := dataTaskWorkflowStagingGuardError(records, narrowPlan); !strings.Contains(errText, "allowed_next_actions") || strings.Contains(errText, "custom_transform]") {
		t.Fatalf("errText=%q, want narrowed candidate contract to still be rejected by prior typed-stage guard", errText)
	}

	plan.Actions = []dataquery.DataAction{
		{
			ID:             "classify",
			Kind:           dataquery.DataActionCustomTransform,
			InputPaths:     []string{"records.csv", "rules.md"},
			OutputArtifact: "classified_records.json",
			Script:         `emit({"artifact":"classified_records","rows":[{"id":"i1"}]})`,
		},
		{
			ID:         "compute",
			Kind:       dataquery.DataActionComputeContribs,
			InputPaths: []string{"classified_records.json"},
			Params: map[string]string{
				"value_field": "value",
				"group_key":   "group",
			},
		},
	}
	errText = dataTaskWorkflowStagingGuardError(records, plan)
	if errText == "" {
		t.Fatalf("errText empty, want multi-action custom_transform batch to remain blocked")
	}

	plan.ContinueAfter = false
	plan.Actions = []dataquery.DataAction{{
		ID:         "normalize",
		Kind:       dataquery.DataActionNormalizeEntities,
		InputPaths: []string{"records.csv"},
	}}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("errText=%q, want next typed stage to pass", errText)
	}
}

func TestDataTaskWorkflowStagePrefixFallbackTrimsBeforeScriptedCustomTransform(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			Status:        "ready",
			ContinueAfter: true,
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{{Path: "records.csv", Required: true}},
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"records.csv"},
			Artifacts:     []dataquery.DataArtifact{{ID: "records", Kind: "extract_records", Headers: []string{"amount"}}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: false,
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials:          []dataquery.CoverageMaterial{{Path: "records.csv", Required: true}},
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{
			{
				ID:             "derive_amount",
				Kind:           dataquery.DataActionDeriveFields,
				InputPaths:     []string{"records"},
				OutputArtifact: "records_derived",
				Params: map[string]string{
					"field_specs_json": `[{"source_field":"amount","target_field":"amount_number","operation":"parse_number"}]`,
				},
			},
			{
				ID:         "whole_workflow",
				Kind:       dataquery.DataActionCustomTransform,
				InputPaths: []string{"records_derived"},
				Script:     `emit_result("0")`,
			},
		},
	}
	errText := "data planning incomplete: workflow next_stage=prepare_contribution_inputs still has unfinished validation stages"
	got, ok := dataTaskWorkflowStagePrefixFallback(records, plan, errText)
	if !ok {
		state := dataTaskWorkflowState(records, plan)
		t.Fatalf("expected stage prefix fallback for %q; state=%+v missing=%v", errText, state, dataTaskWorkflowMissingValidationStages(state))
	}
	if len(got.Actions) != 1 || got.Actions[0].ID != "derive_amount" {
		t.Fatalf("actions=%+v, want only typed prefix", got.Actions)
	}
	if !got.ContinueAfter {
		t.Fatalf("ContinueAfter=false, want true")
	}
	if !got.CoverageContract.DecisionRecordsRequired || !got.CoverageContract.ContributionLedgerRequired || !got.CoverageContract.ReconcileRequired {
		t.Fatalf("coverage contract not preserved: %+v", got.CoverageContract)
	}
}

func TestDataTaskWorkflowStagePrefixFallbackTrimsCrossRankTypedPlan(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				DecisionRecordsRequired:    true,
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "mapping.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:          "orders",
				Kind:        string(dataquery.DataActionExtractRecords),
				SourcePaths: []string{"orders.csv"},
				Headers:     []string{"id", "raw_category", "amount"},
			}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: false,
		CoverageContract: dataquery.CoverageContract{
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{
			{
				ID:             "derive_amount",
				Kind:           dataquery.DataActionDeriveFields,
				InputPaths:     []string{"orders"},
				OutputArtifact: "orders_derived",
				Params: map[string]string{
					"field_specs_json": `[{"source_field":"amount","target_field":"amount_number","operation":"parse_number"}]`,
				},
			},
			{
				ID:             "enrich_category",
				Kind:           dataquery.DataActionEnrichRecords,
				InputPaths:     []string{"orders_derived", "mapping.csv"},
				OutputArtifact: "orders_enriched",
				Params: map[string]string{
					"source_field":         "raw_category",
					"mapping_source_field": "source_value",
					"mapping_value_field":  "canonical_id",
					"target_field":         "category_id",
				},
			},
			{
				ID:         "contrib",
				Kind:       dataquery.DataActionComputeContribs,
				InputPaths: []string{"orders_enriched"},
				Params: map[string]string{
					"value_field":     "amount_number",
					"group_key_field": "category_id",
					"operation":       "add",
				},
			},
		},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if errText == "" || !strings.Contains(errText, "crosses multiple dependent DAG ranks") {
		t.Fatalf("errText=%q, want cross-rank guard", errText)
	}
	got, ok := dataTaskWorkflowStagePrefixFallback(records, plan, errText)
	if !ok {
		t.Fatalf("expected prefix fallback for cross-rank typed plan")
	}
	if len(got.Actions) != 1 || got.Actions[0].ID != "derive_amount" {
		t.Fatalf("actions=%+v, want first typed rank only", got.Actions)
	}
	if !got.ContinueAfter {
		t.Fatalf("ContinueAfter=false, want true")
	}
	prefix, remainder, ok := dataTaskWorkflowStagePrefixFallbackWithRemainder(records, plan, errText)
	if !ok {
		t.Fatalf("expected prefix fallback with remainder")
	}
	if len(prefix.Actions) != 1 || len(remainder.Actions) != 2 {
		t.Fatalf("prefix=%+v remainder=%+v, want 1 + 2 actions", prefix.Actions, remainder.Actions)
	}
	records = append(records, dataTaskWorkflowRecord{
		Plan: prefix,
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:      "orders_derived",
			Kind:    string(dataquery.DataActionDeriveFields),
			Headers: []string{"id", "raw_category", "amount_number"},
		}}},
	})
	next, remaining, ok := dataTaskPopDeferredActionBatch(records, remainder)
	if !ok {
		t.Fatalf("expected deferred action batch after prefix result")
	}
	if len(next.Actions) != 1 || next.Actions[0].ID != "enrich_category" {
		t.Fatalf("next actions=%+v, want enrich_category", next.Actions)
	}
	if len(remaining.Actions) != 1 || remaining.Actions[0].ID != "contrib" {
		t.Fatalf("remaining actions=%+v, want contrib", remaining.Actions)
	}
}

func TestDataTaskDeferredActionWaitsForGeneratedInputs(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:      "orders_derived",
				Kind:    string(dataquery.DataActionDeriveFields),
				Headers: []string{"id", "amount_number"},
			}},
		},
	}}
	deferred := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		Actions: []dataquery.DataAction{{
			ID:         "contrib",
			Kind:       dataquery.DataActionComputeContribs,
			InputPaths: []string{"orders_joined"},
			Params: map[string]string{
				"value_field":     "amount_number",
				"group_key_field": "id",
				"operation":       "add",
			},
		}},
	}
	if _, _, ok := dataTaskPopDeferredActionBatch(records, deferred); ok {
		t.Fatalf("deferred action popped before generated input orders_joined exists")
	}
	records = append(records, dataTaskWorkflowRecord{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:             "join_orders",
			Kind:           dataquery.DataActionJoinRecords,
			OutputArtifact: "orders_joined",
		}}},
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:      "orders_joined",
			Kind:    string(dataquery.DataActionJoinRecords),
			Headers: []string{"id"},
		}}},
	})
	if _, _, ok := dataTaskPopDeferredActionBatch(records, deferred); ok {
		t.Fatalf("deferred action popped before generated input has required fields")
	}
	records[len(records)-1].Result.Artifacts[0].Headers = []string{"id", "amount_number"}
	next, _, ok := dataTaskPopDeferredActionBatch(records, deferred)
	if !ok || len(next.Actions) != 1 || next.Actions[0].ID != "contrib" {
		t.Fatalf("next=%+v ok=%v, want contrib after generated input exists with required fields", next.Actions, ok)
	}
}

func TestDataTaskDeferredActionSkipsBlockedIndependentPrefix(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:      "orders_ready",
				Kind:    string(dataquery.DataActionDeriveFields),
				Headers: []string{"id", "status", "amount"},
				Fields:  map[string]string{"artifact_aliases": "orders_ready", "json_shape": "array(len=2,item=object(keys=id,status,amount))"},
			}},
		},
	}}
	deferred := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		Actions: []dataquery.DataAction{
			{
				ID:             "blocked_derive",
				Kind:           dataquery.DataActionDeriveFields,
				InputPaths:     []string{"missing_input"},
				OutputArtifact: "missing_output",
				Params: map[string]string{
					"field_specs_json": `[{"source_field":"amount","target_field":"amount_num","operation":"parse_number"}]`,
				},
			},
			{
				ID:             "ready_filter",
				Kind:           dataquery.DataActionFilterRecords,
				InputPaths:     []string{"orders_ready"},
				OutputArtifact: "orders_filtered",
				Params: map[string]string{
					"filters_json": `[{"field":"status","op":"not_empty"}]`,
				},
			},
		},
	}
	next, remaining, ok := dataTaskPopDeferredActionBatch(records, deferred)
	if !ok {
		t.Fatalf("deferred ready action was not dispatched: status=%+v", dataTaskDeferredQueueStatus(records, deferred))
	}
	if len(next.Actions) != 1 || next.Actions[0].ID != "ready_filter" {
		t.Fatalf("next actions=%+v, want ready_filter", next.Actions)
	}
	if len(remaining.Actions) != 1 || remaining.Actions[0].ID != "blocked_derive" {
		t.Fatalf("remaining actions=%+v, want blocked_derive preserved", remaining.Actions)
	}
}

func TestDataTaskDeferredActionRechecksFullWorkflowGuard(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{
					{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				},
				DecisionRecordsRequired:    true,
				RuleCoverageRequired:       true,
				ContributionLedgerRequired: true,
				EntityResolutionRequired:   true,
				ReconcileRequired:          true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "queries.csv"},
			RuleCoverage: []dataquery.RuleCoverageRecord{{
				RuleID:   dataquery.LooseText("R1"),
				RuleText: dataquery.LooseText("eligible rows must be qualified before contribution"),
				Status:   dataquery.LooseText("applied"),
			}},
			EntityResolutions: []dataquery.EntityResolutionRecord{{
				ItemID:      dataquery.LooseText("orders.csv#1:entity"),
				CanonicalID: dataquery.LooseText("E1"),
				Status:      dataquery.LooseText("resolved"),
			}},
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "po_filter_fields",
					Kind:    string(dataquery.DataActionDeriveFields),
					Headers: []string{"po_id", "year", "currency", "status", "amount_num", "resolved_vendor_id", "resolved_category_code"},
					Fields:  map[string]string{"artifact_aliases": "po_filter_fields", "json_shape": "array(len=2,item=object(keys=po_id,year,currency,status,amount_num,resolved_vendor_id,resolved_category_code))"},
				},
				{
					ID:      "queries.csv",
					Kind:    string(dataquery.DataActionExtractRecords),
					Headers: []string{"query_id", "vendor_id", "category_code", "po_id", "year", "amount_num"},
					Fields:  map[string]string{"artifact_aliases": "queries.csv", "json_shape": "array(len=1,item=object(keys=query_id,vendor_id,category_code,po_id,year,amount_num))"},
				},
			},
		},
	}}
	deferred := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		Actions: []dataquery.DataAction{{
			ID:             "qualify_eligible",
			Kind:           dataquery.DataActionQualifyRecords,
			InputPaths:     []string{"po_filter_fields", "queries.csv"},
			OutputArtifact: "eligible_pos",
			Params: map[string]string{
				"filters_json":    `[{"field":"year","op":"eq","value":"2026"}]`,
				"required_fields": `["po_id","amount_num"]`,
			},
		}},
	}
	if next, _, ok := dataTaskPopDeferredActionBatch(records, deferred); ok {
		t.Fatalf("deferred next=%+v, want full staging guard to block multi-input qualify_records", next.Actions)
	}
	status := dataTaskDeferredQueueStatus(records, deferred)
	if status.Ready {
		t.Fatalf("status=%+v, want not ready", status)
	}
	if !strings.Contains(status.Reason, "qualify_records") ||
		!strings.Contains(status.Reason, "single-record-set") ||
		!strings.Contains(status.Reason, "2 input_paths") {
		t.Fatalf("reason=%q, want full staging guard reason", status.Reason)
	}
}

func TestDataTaskDeferredActionNarrowsSingleRecordSetByFieldContract(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{
					{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				},
				DecisionRecordsRequired:    true,
				RuleCoverageRequired:       true,
				ContributionLedgerRequired: true,
				EntityResolutionRequired:   true,
				ReconcileRequired:          true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "queries.csv"},
			RuleCoverage: []dataquery.RuleCoverageRecord{{
				RuleID:   dataquery.LooseText("R1"),
				RuleText: dataquery.LooseText("eligible rows must be qualified before contribution"),
				Status:   dataquery.LooseText("applied"),
			}},
			EntityResolutions: []dataquery.EntityResolutionRecord{{
				ItemID:      dataquery.LooseText("orders.csv#1:entity"),
				CanonicalID: dataquery.LooseText("E1"),
				Status:      dataquery.LooseText("resolved"),
			}},
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "po_for_qualification.json",
					Kind:    string(dataquery.DataActionJoinRecords),
					Headers: []string{"po_id", "status", "vendor_id_resolved_status", "category_code_resolved_status"},
					Fields:  map[string]string{"artifact_aliases": "po_for_qualification.json", "json_shape": "array(len=2,item=object(keys=po_id,status,vendor_id_resolved_status,category_code_resolved_status))"},
				},
				{
					ID:      "rules_artifacts.json",
					Kind:    string(dataquery.DataActionDeriveRules),
					Headers: []string{"rule_id", "rule_text"},
					Fields:  map[string]string{"artifact_aliases": "rules_artifacts.json", "json_shape": "array(len=1,item=object(keys=rule_id,rule_text))"},
				},
			},
		},
	}}
	deferred := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		Actions: []dataquery.DataAction{{
			ID:             "qualify_eligible",
			Kind:           dataquery.DataActionQualifyRecords,
			InputPaths:     []string{"po_for_qualification.json", "rules_artifacts.json"},
			OutputArtifact: "eligible_pos",
			Params: map[string]string{
				"filters_json":    `[{"field":"vendor_id_resolved_status","op":"eq","value":"resolved"},{"field":"category_code_resolved_status","op":"eq","value":"resolved"}]`,
				"required_fields": `["po_id","status","vendor_id_resolved_status","category_code_resolved_status"]`,
				"status_fields":   `["status"]`,
			},
		}},
	}
	next, _, ok := dataTaskPopDeferredActionBatch(records, deferred)
	if !ok {
		t.Fatalf("deferred action was not dispatched after field-contract narrowing: status=%+v", dataTaskDeferredQueueStatus(records, deferred))
	}
	if len(next.Actions) != 1 {
		t.Fatalf("next actions=%+v, want one action", next.Actions)
	}
	if got := strings.Join(next.Actions[0].InputPaths, ","); got != "po_for_qualification.json" {
		t.Fatalf("InputPaths=%q, want narrowed executable artifact", got)
	}
	if errText := dataTaskWorkflowStagingGuardError(records, next); errText != "" {
		t.Fatalf("staging guard after narrowing: %s", errText)
	}
}

func TestDataTaskDeferredActionDoesNotDropOnSideRuleCoverageGap(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{
					{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				},
				DecisionRecordsRequired:    true,
				RuleCoverageRequired:       true,
				EntityResolutionRequired:   true,
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv"},
			EntityResolutions: []dataquery.EntityResolutionRecord{{
				ItemID:      dataquery.LooseText("orders.csv#1:entity"),
				CanonicalID: dataquery.LooseText("E1"),
				Status:      dataquery.LooseText("resolved"),
			}},
			Artifacts: []dataquery.DataArtifact{{
				ID:      "orders_ready",
				Kind:    string(dataquery.DataActionJoinRecords),
				Headers: []string{"id", "year", "currency", "status", "amount"},
				Fields:  map[string]string{"artifact_aliases": "orders_ready", "json_shape": "array(len=2,item=object(keys=id,year,currency,status,amount))"},
			}},
		},
	}}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if state.NextStage != dataworkflow.StagePrepareContributionInputs {
		t.Fatalf("NextStage=%q, want prepare_contribution_inputs", state.NextStage)
	}
	kinds := strings.Join(state.AllowedNextActions, ",")
	if !strings.Contains(kinds, string(dataquery.DataActionDeriveRules)) || !strings.Contains(kinds, string(dataquery.DataActionQualifyRecords)) {
		t.Fatalf("AllowedNextActions=%v, want side derive_rules plus qualify_records", state.AllowedNextActions)
	}
	deferred := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		Actions: []dataquery.DataAction{{
			ID:             "qualify_orders",
			Kind:           dataquery.DataActionQualifyRecords,
			InputPaths:     []string{"orders_ready"},
			OutputArtifact: "eligible_orders",
			Params: map[string]string{
				"filters_json":    `[{"field":"year","op":"eq","value":"2026"},{"field":"currency","op":"eq","value":"CNY"}]`,
				"required_fields": `["id","status","amount"]`,
			},
		}},
	}
	next, _, ok := dataTaskPopDeferredActionBatch(records, deferred)
	if !ok {
		t.Fatalf("deferred qualify_records was not dispatched with side rule gap: status=%+v", dataTaskDeferredQueueStatus(records, deferred))
	}
	if len(next.Actions) != 1 || next.Actions[0].ID != "qualify_orders" {
		t.Fatalf("next actions=%+v, want qualify_orders", next.Actions)
	}
}

func TestDataTaskDeferredActionResumesApplyResolutionsAfterNormalizeRank(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: []string{"purchase_orders_raw.csv", "vendors.csv", "category_taxonomy.csv"},
			Artifacts: []dataquery.DataArtifact{
				{ID: "po_records", Kind: string(dataquery.DataActionExtractRecords), Headers: []string{"po_id", "vendor_name_raw", "category_raw"}},
				{ID: "vendor_master", Kind: string(dataquery.DataActionExtractRecords), Headers: []string{"vendor_id", "legal_name", "status"}},
				{ID: "category_taxonomy", Kind: string(dataquery.DataActionExtractRecords), Headers: []string{"category_code", "category_name"}},
			},
		},
	}}
	plan := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		CoverageContract: dataquery.CoverageContract{
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{
			{ID: "N1", Kind: dataquery.DataActionNormalizeEntities, InputPaths: []string{"po_records", "vendor_master"}, OutputArtifact: "vendor_normalization"},
			{ID: "N2", Kind: dataquery.DataActionNormalizeEntities, InputPaths: []string{"po_records", "category_taxonomy"}, OutputArtifact: "category_normalization"},
			{
				ID:             "A1",
				Kind:           dataquery.DataActionApplyResolutions,
				InputPaths:     []string{"po_records", "vendor_normalization", "category_normalization"},
				OutputArtifact: "po_enriched",
				Params: map[string]string{
					"resolution_specs_json": `[{"base_path":"po_records","resolution_key_fields":["source_locator"],"resolution_path":"vendor_normalization","target_id_field":"canonical_vendor_id","target_label_field":"canonical_vendor_name"},{"base_path":"po_records","resolution_key_fields":["source_locator"],"resolution_path":"category_normalization","target_id_field":"canonical_category_code","target_label_field":"canonical_category_name"}]`,
				},
			},
			{ID: "A2", Kind: dataquery.DataActionDeriveFields, InputPaths: []string{"po_enriched"}, OutputArtifact: "po_with_year"},
		},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	fallback, remainder, reason, ok := dataTaskWorkflowDeterministicFallback(records, plan, errText)
	if !ok {
		t.Fatalf("fallback=false errText=%q", errText)
	}
	if !strings.Contains(reason, "split") {
		t.Fatalf("reason=%q, want split fallback", reason)
	}
	if len(fallback.Actions) != 2 || len(remainder.Actions) != 2 {
		t.Fatalf("fallback=%+v remainder=%+v, want 2 + 2 actions", fallback.Actions, remainder.Actions)
	}
	records = append(records, dataTaskWorkflowRecord{
		Plan: fallback,
		Result: &dataquery.Result{
			ConsumedPaths: []string{"po_records", "vendor_master", "category_taxonomy"},
			EntityResolutions: []dataquery.EntityResolutionRecord{{
				ItemID:      dataquery.LooseText("row1"),
				SourceValue: dataquery.LooseText("vendor"),
				CanonicalID: dataquery.LooseText("V1"),
				Status:      dataquery.LooseText("resolved"),
			}},
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "vendor_normalization",
					Kind:    string(dataquery.DataActionNormalizeEntities),
					Headers: []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
					Fields:  map[string]string{"count": "1", "artifact_aliases": "vendor_normalization"},
				},
				{
					ID:      "category_normalization",
					Kind:    string(dataquery.DataActionNormalizeEntities),
					Headers: []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
					Fields:  map[string]string{"count": "1", "artifact_aliases": "category_normalization"},
				},
			},
		},
	})
	next, remaining, ok := dataTaskPopDeferredActionBatch(records, remainder)
	if !ok {
		t.Fatalf("deferred apply_entity_resolutions did not resume: status=%+v", dataTaskDeferredQueueStatus(records, remainder))
	}
	if len(next.Actions) != 1 || next.Actions[0].ID != "A1" {
		t.Fatalf("next=%+v, want A1", next.Actions)
	}
	if len(remaining.Actions) != 1 || remaining.Actions[0].ID != "A2" {
		t.Fatalf("remaining=%+v, want A2", remaining.Actions)
	}
}

func TestDataTaskDeriveFieldGuardAllowsPartialMultiSourceFields(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			Artifacts: []dataquery.DataArtifact{{
				ID:      "records",
				Kind:    string(dataquery.DataActionDeriveFields),
				Headers: []string{"id", "vendor_id", "category_code"},
			}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		Actions: []dataquery.DataAction{{
			ID:             "derive_fallback_fields",
			Kind:           dataquery.DataActionDeriveFields,
			InputPaths:     []string{"records"},
			OutputArtifact: "records_projected",
			Params: map[string]string{
				"field_specs_json": `[
					{"source_fields":["vendor_id_resolved","vendor_id"],"target_field":"vendor_key","operation":"coalesce"},
					{"source_fields":["category_code","category_resolved_id"],"target_field":"category_key","operation":"coalesce"}
				]`,
			},
		}},
	}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("errText=%q, want partial coalesce sources accepted", errText)
	}

	plan.Actions[0].Params["field_specs_json"] = `[{"source_fields":["missing_a","missing_b"],"target_field":"missing_key","operation":"coalesce"}]`
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if !strings.Contains(errText, "missing_a") || !strings.Contains(errText, "missing_b") {
		t.Fatalf("errText=%q, want all-missing coalesce fields reported", errText)
	}
}

func TestDataTaskFieldContractGuardReportsCandidateArtifacts(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: []string{"attachment_manifest.csv"},
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "po_entity_resolved",
					Kind:    string(dataquery.DataActionFilterRecords),
					Headers: []string{"po_id", "attachment_ids", "amount"},
				},
				{
					ID:      "po_attachment_rows",
					Kind:    string(dataquery.DataActionExpandRecords),
					Headers: []string{"po_id", "attachment_ids", "attachment_id_single", "amount"},
				},
			},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "join_evidence",
			Kind:       dataquery.DataActionJoinRecords,
			InputPaths: []string{"po_entity_resolved", "attachment_manifest.csv"},
			Params: map[string]string{
				"left_fields":  `["attachment_id_single"]`,
				"right_fields": `["attachment_id"]`,
			},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	for _, want := range []string{"attachment_id_single", "po_attachment_rows", "Candidate artifact"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("errText=%q, want %q", errText, want)
		}
	}
}

func TestDataTaskArtifactAccessAliasDoesNotLetParentShadowChild(t *testing.T) {
	access := sampleDataTaskArtifactAccess([]dataquery.DataArtifact{{
		ID:      "material_inventory",
		Kind:    string(dataquery.DataActionMaterialInventory),
		Headers: []string{"id", "kind", "summary", "fields", "children"},
		Children: []dataquery.DataArtifact{{
			ID:      "attachment_manifest.csv",
			Kind:    "csv",
			Headers: []string{"attachment_id", "po_id", "attachment_type"},
		}},
	}}, 10)
	got, ok := dataTaskArtifactAccessByAlias(access, "attachment_manifest.csv")
	if !ok {
		t.Fatalf("attachment_manifest.csv not found in access=%+v", access)
	}
	if got.ID != "attachment_manifest.csv" {
		t.Fatalf("got ID=%q fields=%v, want child artifact", got.ID, got.Fields)
	}
	if !slices.Contains(got.Fields, "attachment_id") {
		t.Fatalf("fields=%v, want child record fields", got.Fields)
	}
	if slices.Contains(got.Fields, "children") {
		t.Fatalf("fields=%v, parent metadata fields should not shadow child", got.Fields)
	}
}

func TestDataTaskApplyResolutionGuardAcceptsItemIDForSourceLocatorKey(t *testing.T) {
	access := []dataTaskArtifactAccessPrompt{
		{
			ID:      "base_records",
			Aliases: []string{"base_records"},
			Fields:  []string{"_source_index", "raw_name"},
		},
		{
			ID:      "entity_resolution_records",
			Aliases: []string{"entity_resolution_records"},
			Fields:  []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
		},
	}
	action := dataquery.DataAction{
		ID:         "apply_resolution",
		Kind:       dataquery.DataActionApplyResolutions,
		InputPaths: []string{"base_records", "entity_resolution_records"},
		Params: map[string]string{
			"base_path":       "base_records",
			"resolution_path": "entity_resolution_records",
			"resolution_specs_json": `[{
				"base_key_fields":["_source_index"],
				"resolution_key_fields":["source_locator"],
				"target_id_field":"canonical_id_applied"
			}]`,
		},
	}
	if errText := dataTaskApplyResolutionActionFieldContractGuardError(access, action, 0); errText != "" {
		t.Fatalf("guard error=%q, want source_locator accepted through item_id equivalent", errText)
	}
}

func TestDataTaskWorkflowStagingRejectsUnavailableTypedActionInputs(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:      "orders_derived",
				Kind:    string(dataquery.DataActionDeriveFields),
				Headers: []string{"id", "amount_number"},
			}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "contrib",
			Kind:       dataquery.DataActionComputeContribs,
			InputPaths: []string{"orders_joined"},
			Params: map[string]string{
				"value_field":     "amount_number",
				"group_key_field": "id",
				"operation":       "add",
			},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if errText == "" || !strings.Contains(errText, "unavailable input_path") || !strings.Contains(errText, "orders_joined") {
		t.Fatalf("errText=%q, want unavailable generated input guard", errText)
	}

	plan.Actions = []dataquery.DataAction{
		{
			ID:             "join_orders",
			Kind:           dataquery.DataActionJoinRecords,
			InputPaths:     []string{"orders_derived", "orders.csv"},
			OutputArtifact: "orders_joined",
			Params: map[string]string{
				"left_fields":  `["id"]`,
				"right_fields": `["id"]`,
			},
		},
		{
			ID:         "contrib",
			Kind:       dataquery.DataActionComputeContribs,
			InputPaths: []string{"orders_joined"},
			Params: map[string]string{
				"value_field":     "amount_number",
				"group_key_field": "id",
				"operation":       "add",
			},
		},
	}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); strings.Contains(errText, "unavailable input_path") {
		t.Fatalf("errText=%q, same-batch output alias should be available to later action", errText)
	}

	plan = dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"left.csv", "right.csv"},
		Actions: []dataquery.DataAction{{
			ID:         "join_current_inputs",
			Kind:       dataquery.DataActionJoinRecords,
			InputPaths: []string{"left.csv", "right.csv"},
			Params: map[string]string{
				"left_fields":  `["id"]`,
				"right_fields": `["id"]`,
			},
		}},
	}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); strings.Contains(errText, "unavailable input_path") {
		t.Fatalf("errText=%q, current-batch top-level input_paths should be available to actions", errText)
	}
}

func TestDataTaskWorkflowStagingRejectsMissingArtifactFields(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:      "entity_resolutions",
				Kind:    string(dataquery.DataActionNormalizeEntities),
				Headers: []string{"source_value", "canonical_id", "status"},
				Fields:  map[string]string{"artifact_aliases": "entity_resolutions", "json_shape": "array(len=4,item=object(keys=source_value,canonical_id,status))"},
			}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "filter_business_records",
			Kind:       dataquery.DataActionFilterRecords,
			InputPaths: []string{"entity_resolutions"},
			Params: map[string]string{
				"filters_json": `[{"field":"amount_number","op":"gt","value":"0"}]`,
			},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if errText == "" || !strings.Contains(errText, "amount_number") || !strings.Contains(errText, "not present on input entity_resolutions") {
		t.Fatalf("errText=%q, want missing field contract guard", errText)
	}
}

func TestDataTaskWorkflowStagingUsesFullArtifactContractFields(t *testing.T) {
	headers := make([]string, 0, 46)
	for i := 0; i < 40; i++ {
		headers = append(headers, fmt.Sprintf("field_%02d", i))
	}
	headers = append(headers, "status_is_valid", "is_cny", "amount_system", "query_id", "po_id")
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:      "joined_records",
				Kind:    string(dataquery.DataActionJoinRecords),
				Headers: headers,
				Fields: map[string]string{
					"artifact_aliases": "joined_records,joined_records.json",
					"json_shape":       "array(len=9,item=object(keys=...))",
					"output_headers":   strings.Join(headers, ","),
				},
			}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "compute_totals",
			Kind:       dataquery.DataActionComputeContribs,
			InputPaths: []string{"joined_records"},
			Params: map[string]string{
				"value_field":     "amount_system",
				"group_key_field": "query_id",
				"item_id_field":   "po_id",
				"operation":       "sum",
				"filters_json":    `[{"field":"status_is_valid","op":"eq","value":"true"},{"field":"is_cny","op":"eq","value":"true"}]`,
			},
		}},
	}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("errText=%q, hard gate should use full artifact fields instead of compact prompt fields", errText)
	}
}

func TestDataTaskWorkflowStagingApplyEntityResolutionsHonorsSpecBasePath(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv"},
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "orders",
					Kind:    string(dataquery.DataActionDeriveFields),
					Headers: []string{"id", "raw_category", "amount"},
					Fields:  map[string]string{"artifact_aliases": "orders,orders.json", "json_shape": "array(len=2,item=object(keys=id,raw_category,amount))"},
				},
				{
					ID:      "category_resolution",
					Kind:    string(dataquery.DataActionNormalizeEntities),
					Headers: []string{"item_id", "source_value", "canonical_id", "status"},
					Fields:  map[string]string{"artifact_aliases": "category_resolution,category_resolution.json", "json_shape": "array(len=2,item=object(keys=item_id,source_value,canonical_id,status))"},
				},
			},
		},
	}}
	plan := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		Actions: []dataquery.DataAction{{
			ID:         "orders_with_category",
			Kind:       dataquery.DataActionApplyResolutions,
			InputPaths: []string{"category_resolution", "orders"},
			Params: map[string]string{
				"resolution_specs_json": `[{
					"base_path":"orders",
					"resolution_path":"category_resolution",
					"base_key_fields":["_source_index"],
					"resolution_key_fields":["item_id"],
					"target_id_field":"category_id"
				}]`,
			},
		}},
	}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("apply_entity_resolutions guard err=%q, want pass", errText)
	}
	plan.Actions[0].Params["resolution_specs_json"] = `[{
		"resolution_path":"category_resolution",
		"base_key_fields":["_source_index"],
		"resolution_key_fields":["item_id"],
		"target_id_field":"category_id"
	}]`
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("apply_entity_resolutions guard err=%q, want role inference to validate against orders base", errText)
	}
	plan.Actions[0].Params["resolution_specs_json"] = `[{
		"base_path":"orders",
		"resolution_path":"category_resolution",
		"base_key_fields":["_source_index"],
		"resolution_key_fields":["_source_index"],
		"target_id_field":"category_id"
	}]`
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("apply_entity_resolutions guard err=%q, want virtual resolution locator to be compatible with item_id", errText)
	}
	plan.Actions[0].Params["resolution_specs_json"] = `[{
		"base_path":"orders",
		"resolution_path":"category_resolution",
		"base_key_fields":["missing_base_field"],
		"resolution_key_fields":["item_id"],
		"target_id_field":"category_id"
	}]`
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if errText == "" || !strings.Contains(errText, "missing_base_field") || !strings.Contains(errText, "not present on input orders") {
		t.Fatalf("errText=%q, want missing base field on orders", errText)
	}
	guard := dataTaskWorkflowStagingGuardResult(records, plan)
	if guard.Code != "field_contract_violation" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want typed apply_entity_resolutions field-contract violation", guard)
	}
	if guard.Violations[0].ActionID != "orders_with_category" || guard.Violations[0].InputAlias != "orders" || !slices.Contains(guard.Violations[0].MissingFields, "missing_base_field") {
		t.Fatalf("violation=%+v, want missing base field on orders", guard.Violations[0])
	}
}

func TestDataTaskWorkflowStagingTreatsCanonicalRecordAsApplyResolutionBase(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "taxonomy.csv"},
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "orders_with_vendor",
					Kind:    string(dataquery.DataActionApplyResolutions),
					Headers: []string{"_source_index", "_source_locator", "order_id", "raw_category", "vendor_id_canonical", "vendor_resolution_status"},
					Fields: map[string]string{
						"artifact_aliases": "orders_with_vendor,orders_with_vendor.json",
						"json_shape":       "array(len=2,item=object(keys=_source_index,_source_locator,order_id,raw_category,vendor_id_canonical,vendor_resolution_status))",
					},
				},
				{
					ID:      "category_resolution",
					Kind:    string(dataquery.DataActionNormalizeEntities),
					Headers: []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
					Fields:  map[string]string{"artifact_aliases": "category_resolution,category_resolution.json", "json_shape": "array(len=2,item=object(keys=item_id,source_value,canonical_id,canonical_label,status))"},
				},
			},
		},
	}}
	plan := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		Actions: []dataquery.DataAction{{
			ID:         "orders_with_category",
			Kind:       dataquery.DataActionApplyResolutions,
			InputPaths: []string{"orders_with_vendor", "category_resolution"},
			Params: map[string]string{
				"base_path":       "orders_with_vendor",
				"resolution_path": "category_resolution",
				"resolution_specs_json": `[{
					"base_key_fields":["_source_index"],
					"resolution_key_fields":["item_id"],
					"target_id_field":"category_id_canonical",
					"target_label_field":"category_label_canonical",
					"target_status_field":"category_resolution_status"
				}]`,
			},
		}},
	}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("apply_entity_resolutions guard err=%q, canonical-enriched record artifact should remain a base record", errText)
	}
}

func TestDataTaskWorkflowStagingRejectsRepeatedApplyResolutionEdge(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			Artifacts: []dataquery.DataArtifact{
				{
					ID:          "orders_with_category",
					Kind:        string(dataquery.DataActionApplyResolutions),
					SourcePaths: []string{"orders", "category_resolution"},
					Headers:     []string{"_source_index", "order_id", "category_id", "category_id_status"},
					Fields: map[string]string{
						"artifact_aliases": "orders_with_category,orders_with_category.json",
						"json_shape":       "array(len=2,item=object(keys=_source_index,order_id,category_id,category_id_status))",
					},
				},
				{
					ID:      "category_resolution",
					Kind:    string(dataquery.DataActionNormalizeEntities),
					Headers: []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
					Fields: map[string]string{
						"artifact_aliases": "category_resolution,category_resolution.json",
						"json_shape":       "array(len=2,item=object(keys=item_id,source_value,canonical_id,canonical_label,status))",
					},
				},
			},
		},
	}}
	plan := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		Actions: []dataquery.DataAction{{
			ID:         "repeat_category_resolution",
			Kind:       dataquery.DataActionApplyResolutions,
			InputPaths: []string{"orders_with_category", "category_resolution"},
			Params: map[string]string{
				"base_path": "orders_with_category",
				"resolution_specs_json": `[{
					"resolution_path":"category_resolution",
					"base_key_fields":["_source_index"],
					"resolution_key_fields":["item_id"],
					"target_id_field":"category_id",
					"target_status_field":"category_id_status"
				}]`,
			},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if errText == "" || !strings.Contains(errText, "repeats apply_entity_resolutions") || !strings.Contains(errText, "idempotent") {
		t.Fatalf("errText=%q, want repeated apply-resolution edge rejection", errText)
	}
	guard := dataTaskWorkflowStagingGuardResult(records, plan)
	if guard.Code != "apply_resolution_no_progress" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want typed no-progress apply-resolution guard", guard)
	}
	if guard.Violations[0].ActionID != "repeat_category_resolution" || guard.Violations[0].InputAlias != "category_resolution" {
		t.Fatalf("violation=%+v, want repeated resolution input handle", guard.Violations[0])
	}
}

func TestDataTaskWorkflowStagingRejectsApplyResolutionIncompatibleSourceLineage(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			Artifacts: []dataquery.DataArtifact{
				{
					ID:          "rules_records",
					Kind:        string(dataquery.DataActionExtractRecords),
					SourcePaths: []string{"data_rules.md"},
					Headers:     []string{"_source_index", "text"},
					Fields: map[string]string{
						"artifact_aliases": "rules_records,rules_records.json",
						"json_shape":       "array(len=3,item=object(keys=_source_index,text))",
					},
				},
				{
					ID:          "vendor_resolution",
					Kind:        string(dataquery.DataActionNormalizeEntities),
					SourcePaths: []string{"orders_cleaned", "vendors.csv"},
					Headers:     []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
					Fields: map[string]string{
						"artifact_aliases": "vendor_resolution,vendor_resolution.json",
						"json_shape":       "array(len=2,item=object(keys=item_id,source_value,canonical_id,canonical_label,status))",
					},
				},
			},
		},
	}}
	plan := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		Actions: []dataquery.DataAction{{
			ID:         "apply_vendor_to_rules",
			Kind:       dataquery.DataActionApplyResolutions,
			InputPaths: []string{"rules_records", "vendor_resolution"},
			Params: map[string]string{
				"base_path": "rules_records",
				"resolution_specs_json": `[{
					"resolution_path":"vendor_resolution",
					"base_key_fields":["_source_index"],
					"resolution_key_fields":["item_id"],
					"target_id_field":"vendor_id"
				}]`,
			},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if errText == "" || !strings.Contains(errText, "source lineage") || !strings.Contains(errText, "not compatible") {
		t.Fatalf("errText=%q, want incompatible source-lineage rejection", errText)
	}
}

func TestDataTaskWorkflowStagingRejectsNormalizeEntitiesMissingSourceFields(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "taxonomy.csv"},
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "entity_mapping",
					Kind:    string(dataquery.DataActionNormalizeEntities),
					Headers: []string{"source_value", "canonical_id", "status"},
					Fields:  map[string]string{"artifact_aliases": "entity_mapping", "json_shape": "array(len=3,item=object(keys=source_value,canonical_id,status))"},
				},
				{
					ID:      "taxonomy.csv",
					Kind:    string(dataquery.DataActionExtractRecords),
					Headers: []string{"category_code", "category_name", "terms"},
					Fields:  map[string]string{"artifact_aliases": "taxonomy.csv", "json_shape": "array(len=3,item=object(keys=category_code,category_name,terms))"},
				},
			},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "normalize_category",
			Kind:       dataquery.DataActionNormalizeEntities,
			InputPaths: []string{"entity_mapping", "taxonomy.csv"},
			Params: map[string]string{
				"source_fields":         `["category_raw","description"]`,
				"reference_fields":      `["terms"]`,
				"canonical_id_field":    "category_code",
				"canonical_label_field": "category_name",
			},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if errText == "" || !strings.Contains(errText, "category_raw") || !strings.Contains(errText, "not present on input entity_mapping") {
		t.Fatalf("errText=%q, want normalize_entities source field contract guard", errText)
	}
	guard := dataTaskWorkflowStagingGuardResult(records, plan)
	if guard.Code != "field_contract_violation" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want typed normalize_entities field-contract violation", guard)
	}
	if guard.Violations[0].ActionID != "normalize_category" || guard.Violations[0].InputAlias != "entity_mapping" || !slices.Contains(guard.Violations[0].MissingFields, "category_raw") {
		t.Fatalf("violation=%+v, want missing source field on entity_mapping", guard.Violations[0])
	}
}

func TestDataTaskWorkflowStagingAllowsNormalizeEntitiesImplicitRoleSwap(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "taxonomy.csv"},
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "taxonomy.csv",
					Kind:    string(dataquery.DataActionExtractRecords),
					Headers: []string{"category_code", "category_name", "terms"},
					Fields:  map[string]string{"artifact_aliases": "taxonomy.csv", "json_shape": "array(len=3,item=object(keys=category_code,category_name,terms))"},
				},
				{
					ID:      "orders",
					Kind:    string(dataquery.DataActionExtractRecords),
					Headers: []string{"po_id", "category_raw", "description"},
					Fields:  map[string]string{"artifact_aliases": "orders", "json_shape": "array(len=3,item=object(keys=po_id,category_raw,description))"},
				},
			},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "normalize_category",
			Kind:       dataquery.DataActionNormalizeEntities,
			InputPaths: []string{"taxonomy.csv", "orders"},
			Params: map[string]string{
				"source_fields":         `["category_raw","description"]`,
				"reference_fields":      `["terms"]`,
				"canonical_id_field":    "category_code",
				"canonical_label_field": "category_name",
			},
		}},
	}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("errText=%q, want implicit source/reference role swap to pass", errText)
	}
}

func TestDataTaskWorkflowStagingSplitsIntraBatchArtifactDependency(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{
			{
				ID:             "normalize_source",
				Kind:           dataquery.DataActionNormalizeEntities,
				InputPaths:     []string{"records.csv", "reference.csv"},
				OutputArtifact: "source_resolutions",
				Params: map[string]string{
					"source_field":          "name_raw",
					"reference_name_fields": `["name","aliases"]`,
					"canonical_id_field":    "code",
				},
			},
			{
				ID:             "filter_source",
				Kind:           dataquery.DataActionFilterRecords,
				InputPaths:     []string{"source_resolutions"},
				OutputArtifact: "filtered_source",
				Params: map[string]string{
					"filters_json": `[{"field":"status","op":"eq","value":"ok"}]`,
				},
			},
		},
		ContinueAfter: true,
	}
	errText := dataTaskPlanStagingGuardError(plan)
	if !strings.Contains(errText, "consumes prior action output") {
		t.Fatalf("errText=%q, want intra-batch dependency guard", errText)
	}
	fallback, remainder, reason, ok := dataTaskWorkflowDeterministicFallback(nil, plan, errText)
	if !ok {
		t.Fatalf("deterministic fallback not produced")
	}
	if !strings.Contains(reason, "intra-batch artifact dependency") {
		t.Fatalf("reason=%q, want dependency split reason", reason)
	}
	if len(fallback.Actions) != 1 || fallback.Actions[0].ID != "normalize_source" {
		t.Fatalf("fallback actions=%+v, want producer prefix only", fallback.Actions)
	}
	if !fallback.ContinueAfter {
		t.Fatalf("fallback ContinueAfter=false, want true")
	}
	if len(remainder.Actions) != 1 || remainder.Actions[0].ID != "filter_source" {
		t.Fatalf("remainder actions=%+v, want dependent suffix", remainder.Actions)
	}
}

func TestDataTaskWorkflowFallbackExecutesValidPrefixAndDropsInvalidSuffix(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		Actions: []dataquery.DataAction{
			{
				ID:             "inspect_text",
				Kind:           dataquery.DataActionInspectMaterial,
				InputPaths:     []string{"evidence.txt"},
				OutputArtifact: "text_samples",
			},
			{
				ID:             "extract_fields_without_spec",
				Kind:           dataquery.DataActionExtractFields,
				InputPaths:     []string{"evidence.txt"},
				OutputArtifact: "extracted_fields",
			},
		},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if errText == "" || !strings.Contains(errText, "has no field specification") {
		t.Fatalf("errText=%q, want invalid extract_fields suffix", errText)
	}
	fallback, remainder, reason, ok := dataTaskWorkflowDeterministicFallback(nil, plan, errText)
	if !ok {
		t.Fatalf("deterministic fallback not produced")
	}
	if !strings.Contains(reason, "valid typed prefix") {
		t.Fatalf("reason=%q, want valid-prefix reason", reason)
	}
	if len(fallback.Actions) != 1 || fallback.Actions[0].ID != "inspect_text" {
		t.Fatalf("fallback actions=%+v, want only executable prefix", fallback.Actions)
	}
	if !fallback.ContinueAfter {
		t.Fatalf("fallback ContinueAfter=false, want true")
	}
	if len(remainder.Actions) != 0 {
		t.Fatalf("remainder=%+v, invalid suffix must not enter deferred queue", remainder.Actions)
	}
}

func TestDataTaskPreflightWorkflowPlanRewritesBeforeAudit(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{
			{
				ID:             "extract_records",
				Kind:           dataquery.DataActionExtractRecords,
				InputPaths:     []string{"records.csv"},
				OutputArtifact: "records",
			},
			{
				ID:             "filter_records",
				Kind:           dataquery.DataActionFilterRecords,
				InputPaths:     []string{"records"},
				OutputArtifact: "filtered",
				Params: map[string]string{
					"filters_json": `[{"field":"status","op":"eq","value":"ok"}]`,
				},
			},
		},
		ContinueAfter: true,
	}
	preflight := dataTaskPreflightWorkflowPlan(nil, plan, func(p dataquery.TaskPlan) dataquery.TaskPlan { return p })
	if !preflight.Rewritten {
		t.Fatalf("preflight=%+v, want deterministic rewrite before audit/display", preflight)
	}
	if !strings.Contains(preflight.Reason, "intra-batch artifact dependency") {
		t.Fatalf("reason=%q, want dependency split", preflight.Reason)
	}
	if len(preflight.Plan.Actions) != 1 || preflight.Plan.Actions[0].ID != "extract_records" {
		t.Fatalf("plan actions=%+v, want executable prefix", preflight.Plan.Actions)
	}
	if len(preflight.Remainder.Actions) != 1 || preflight.Remainder.Actions[0].ID != "filter_records" {
		t.Fatalf("remainder=%+v, want deferred suffix", preflight.Remainder.Actions)
	}
}

func TestDataTaskPreflightInitialPlanSplitsDependencyRanks(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{
			{ID: "inventory", Kind: dataquery.DataActionMaterialInventory},
			{ID: "extract", Kind: dataquery.DataActionExtractRecords, InputPaths: []string{"orders.csv"}, OutputArtifact: "orders"},
			{ID: "rules", Kind: dataquery.DataActionDeriveRules, InputPaths: []string{"rules.md"}},
			{ID: "normalize", Kind: dataquery.DataActionNormalizeEntities},
		},
		ContinueAfter: true,
	}
	preflight := dataTaskPreflightWorkflowPlan(nil, plan, func(p dataquery.TaskPlan) dataquery.TaskPlan { return p })
	if !preflight.Rewritten {
		t.Fatalf("preflight=%+v, want initial rank split before audit/display", preflight)
	}
	if !strings.Contains(preflight.Reason, "typed dependency rank") {
		t.Fatalf("reason=%q, want rank split", preflight.Reason)
	}
	if len(preflight.Plan.Actions) != 3 || preflight.Plan.Actions[2].ID != "rules" {
		t.Fatalf("prefix actions=%+v, want discovery plus first rank", preflight.Plan.Actions)
	}
	if len(preflight.Remainder.Actions) != 1 || preflight.Remainder.Actions[0].ID != "normalize" {
		t.Fatalf("remainder=%+v, want downstream rank", preflight.Remainder.Actions)
	}
}

func TestDataTaskPreflightRevalidatesFallbackPlan(t *testing.T) {
	inputs := []string{
		"a.csv", "b.csv", "c.csv", "d.csv", "e.csv", "f.csv",
		"g.csv", "h.csv", "i.csv", "j.csv", "k.csv", "l.csv",
	}
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: inputs,
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "a.csv", Required: true},
				{Path: "b.csv", Required: true},
				{Path: "c.csv", Required: true},
				{Path: "d.csv", Required: true},
			},
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{
			{
				ID:             "load_everything",
				Kind:           dataquery.DataActionCustomTransform,
				InputPaths:     inputs,
				OutputArtifact: "records",
				Script:         `emit_result({"records": []})`,
			},
			{
				ID:             "filter_records",
				Kind:           dataquery.DataActionFilterRecords,
				InputPaths:     []string{"records"},
				OutputArtifact: "filtered",
				Params: map[string]string{
					"filters_json": `[{"field":"status","op":"eq","value":"ok"}]`,
				},
			},
		},
		ContinueAfter: true,
	}
	preflight := dataTaskPreflightWorkflowPlan(nil, plan, func(p dataquery.TaskPlan) dataquery.TaskPlan { return p })
	if !preflight.Rewritten {
		t.Fatalf("preflight=%+v, want deterministic rewrite", preflight)
	}
	if !strings.Contains(preflight.Reason, "intra-batch artifact dependency") || !strings.Contains(preflight.Reason, "material discovery") {
		t.Fatalf("reason=%q, want chained fallback reasons", preflight.Reason)
	}
	if len(preflight.Plan.Actions) != 1 || preflight.Plan.Actions[0].Kind != dataquery.DataActionMaterialInventory {
		t.Fatalf("plan actions=%+v, want revalidated material_inventory fallback", preflight.Plan.Actions)
	}
	if len(preflight.Remainder.Actions) != 1 || preflight.Remainder.Actions[0].ID != "filter_records" {
		t.Fatalf("remainder=%+v, want original dependent suffix preserved", preflight.Remainder.Actions)
	}
	if preflight.FinalGuardErr != "" {
		t.Fatalf("FinalGuardErr=%q, want successful fallback plan to be displayable", preflight.FinalGuardErr)
	}
}

func TestDataTaskPreflightMarksRejectedCandidate(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:   "derive_without_input",
			Kind: dataquery.DataActionDeriveFields,
			Params: map[string]string{
				"field_specs_json": `[{"source_field":"value","target_field":"value_number","operation":"parse_number"}]`,
			},
		}},
		ContinueAfter: true,
	}
	preflight := dataTaskPreflightWorkflowPlan(nil, plan, func(p dataquery.TaskPlan) dataquery.TaskPlan { return p })
	if preflight.FinalGuardErr == "" {
		t.Fatalf("preflight=%+v, want final guard error for non-displayable candidate", preflight)
	}
	if !strings.Contains(preflight.FinalGuardErr, "requires input_paths") {
		t.Fatalf("FinalGuardErr=%q, want input path contract failure", preflight.FinalGuardErr)
	}
}

func TestDataTaskDeferredActionRedirectsToCompatibleArtifactSchema(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{Status: "ready", ContinueAfter: true},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"purchase_orders_raw.csv", "category_taxonomy.csv"},
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "po_vendor",
					Kind:    string(dataquery.DataActionEnrichRecords),
					Headers: []string{"po_id", "vendor_id_canonical"},
					Fields: map[string]string{
						"artifact_aliases": "po_vendor",
						"json_shape":       "array(len=50)",
					},
				},
				{
					ID:      "category_reference",
					Kind:    string(dataquery.DataActionExtractRecords),
					Headers: []string{"category_code", "common_raw_terms"},
					Fields: map[string]string{
						"artifact_aliases": "category_reference",
						"json_shape":       "array(len=12)",
					},
				},
				{
					ID:      "po_records",
					Kind:    string(dataquery.DataActionExtractRecords),
					Headers: []string{"po_id", "category_raw"},
					Fields: map[string]string{
						"artifact_aliases": "po_records",
						"json_shape":       "array(len=50)",
					},
				},
			},
		},
	}}
	deferred := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:             "category_resolution",
			Kind:           dataquery.DataActionNormalizeEntities,
			InputPaths:     []string{"po_vendor", "category_reference"},
			OutputArtifact: "category_resolutions",
			Params: map[string]string{
				"source_field":          "category_raw",
				"reference_fields_json": `["category_code","common_raw_terms"]`,
				"canonical_id_field":    "category_code",
			},
		}},
		ContinueAfter: true,
	}
	next, _, ok := dataTaskPopDeferredActionBatch(records, deferred)
	if !ok {
		t.Fatalf("deferred action was not dispatched")
	}
	if got := next.Actions[0].InputPaths[0]; got != "po_records" {
		t.Fatalf("first input = %q, want compatible artifact po_records", got)
	}
}

func TestDataTaskWorkflowStagingAllowsSequentialDeriveFieldSpecs(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:      "orders",
				Kind:    string(dataquery.DataActionExtractRecords),
				Headers: []string{"id", "status_raw"},
			}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "derive_status",
			Kind:       dataquery.DataActionDeriveFields,
			InputPaths: []string{"orders"},
			Params: map[string]string{
				"field_specs_json": `[
					{"source_field":"status_raw","target_field":"status_normalized","operation":"lower"},
					{"source_field":"status_normalized","target_field":"is_valid_status","operation":"map","mapping":{"paid":"true"}}
				]`,
			},
		}},
	}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("errText=%q, want sequential derive fields to pass", errText)
	}
	plan.Actions[0].Params["field_specs_json"] = `[{"source_field":"missing_status","target_field":"status_normalized","operation":"lower"}]`
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if errText == "" || !strings.Contains(errText, "missing_status") {
		t.Fatalf("errText=%q, want missing derive source field guard", errText)
	}
}

func TestDataTaskCLIRepairNoToolFallsBackToContinuation(t *testing.T) {
	planner := &stubDataTaskPlanner{
		repairErr: errors.New("data task planner returned no tool_call"),
		continuePlan: dataquery.TaskPlan{
			Status:        "ready",
			ContinueAfter: true,
			Actions: []dataquery.DataAction{{
				ID:             "derive_next_fields",
				Kind:           dataquery.DataActionDeriveFields,
				InputPaths:     []string{"orders.csv"},
				OutputArtifact: "orders_derived.json",
				Params: map[string]string{
					"field_specs_json": `[{"source_field":"raw","target_field":"clean","operation":"trim"}]`,
				},
			}},
		},
	}
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{ConsumedPaths: []string{"orders.csv"}},
	}}
	plan, rounds, ok, err := repairDataTaskPlanForCLI(
		context.Background(),
		planner,
		"continue data task",
		"/repo",
		TurnPolicy{Route: RouteData, DataTaskKind: "data_aggregation"},
		nil,
		dataquery.TaskPlan{Status: "ready"},
		"data planning incomplete: material coverage is already sufficient",
		records,
		0,
		6,
	)
	if err != nil {
		t.Fatalf("repairDataTaskPlanForCLI: %v", err)
	}
	if !ok || rounds != 1 || planner.repairCalls != 1 || planner.continueCalls != 1 {
		t.Fatalf("ok=%v rounds=%d repair=%d continue=%d", ok, rounds, planner.repairCalls, planner.continueCalls)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].ID != "derive_next_fields" {
		t.Fatalf("plan=%+v, want continuation plan", plan)
	}
}

func TestDataTaskWorkflowStagingAllowsCoverageActionsToReadSourceMaterials(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv"},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RuleCoverageRequired: true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "derive_rules",
			Kind:       dataquery.DataActionDeriveRules,
			InputPaths: []string{"data_rules.md"},
		}},
	}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); strings.Contains(errText, "unavailable input_path") {
		t.Fatalf("errText=%q, source material coverage action should be allowed", errText)
	}
}

func TestDataTaskFailedPlanDoesNotPolluteWorkflowRequiredMaterials(t *testing.T) {
	records := []dataTaskWorkflowRecord{
		{
			Plan: dataquery.TaskPlan{CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{{
					Path:      "orders.csv",
					Purpose:   dataTaskUserExplicitMaterialPurpose,
					Required:  true,
					UsageMode: dataquery.MaterialUseScriptConsumed,
				}},
				ContributionLedgerRequired: true,
			}},
			Result: &dataquery.Result{ConsumedPaths: []string{"orders.csv"}},
		},
		{
			Plan: dataquery.TaskPlan{CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{{
					Path:      "speculative_joined_artifact",
					Required:  true,
					UsageMode: dataquery.MaterialUseScriptConsumed,
				}},
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			}},
			Err: "data planning incomplete: speculative generated artifact was not ready",
		},
	}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if !state.MaterialCoverageSufficient {
		t.Fatalf("state=%+v, want source-material coverage to remain sufficient", state)
	}
	if slices.Contains(state.RequiredMaterials, "speculative_joined_artifact") || slices.Contains(state.MissingRequiredMaterials, "speculative_joined_artifact") {
		t.Fatalf("state=%+v, failed plan generated artifact polluted required materials", state)
	}
	if state.NextStage != "prepare_contribution_inputs" {
		t.Fatalf("next_stage=%q, want prepare_contribution_inputs", state.NextStage)
	}
}

func TestDataTaskInvalidRecordActionFallbackMaterializesBadDeriveFields(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:             "derive_all",
			Kind:           dataquery.DataActionDeriveFields,
			InputPaths:     []string{"a.csv", "b.csv"},
			OutputArtifact: "derived_all",
		}},
	}
	fallback, ok := dataTaskInvalidRecordActionFallback(nil, plan, "data planning incomplete: derive_fields with 2 input_paths")
	if !ok {
		t.Fatalf("fallback not produced")
	}
	if len(fallback.Actions) != 1 || fallback.Actions[0].Kind != dataquery.DataActionExtractRecords {
		t.Fatalf("Actions=%+v, want extract_records fallback", fallback.Actions)
	}
	if got := strings.Join(fallback.Actions[0].InputPaths, ","); got != "a.csv,b.csv" {
		t.Fatalf("InputPaths=%q, want original inputs", got)
	}
	if !fallback.ContinueAfter {
		t.Fatalf("ContinueAfter=false, want continuation")
	}
}

func TestPrepareDataTaskWorkflowPlanNarrowsSingleRecordSetActionByFieldContract(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{
			{
				ID:      "purchase_orders_raw.csv",
				Kind:    "extract_records",
				Headers: []string{"po_id", "created_at", "status"},
				Fields:  map[string]string{"artifact_aliases": "purchase_orders_raw.csv"},
			},
			{
				ID:      "queries.csv",
				Kind:    "extract_records",
				Headers: []string{"query_id", "vendor_id", "category_code"},
				Fields:  map[string]string{"artifact_aliases": "queries.csv"},
			},
		}},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:             "derive_year",
			Kind:           dataquery.DataActionDeriveFields,
			InputPaths:     []string{"purchase_orders_raw.csv", "queries.csv"},
			OutputArtifact: "orders_with_year",
			Params: map[string]string{
				"field_specs_json": `[{"source_field":"created_at","target_field":"year","operation":"substring","start":0,"length":4}]`,
			},
		}},
	}
	prepared := prepareDataTaskWorkflowPlanForExecution("", nil, records, plan)
	if len(prepared.Actions) != 1 {
		t.Fatalf("Actions=%+v", prepared.Actions)
	}
	if got := strings.Join(prepared.Actions[0].InputPaths, ","); got != "purchase_orders_raw.csv" {
		t.Fatalf("InputPaths=%q, want field-backed single input", got)
	}
	if errText := dataTaskWorkflowStagingGuardError(records, prepared); errText != "" {
		t.Fatalf("prepared plan guard error=%q", errText)
	}
}

func TestPrepareDataTaskWorkflowPlanKeepsAmbiguousSingleRecordSetInputs(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{
			{ID: "left.csv", Headers: []string{"id", "created_at"}, Fields: map[string]string{"artifact_aliases": "left.csv"}},
			{ID: "right.csv", Headers: []string{"id", "created_at"}, Fields: map[string]string{"artifact_aliases": "right.csv"}},
		}},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:         "derive_year",
			Kind:       dataquery.DataActionDeriveFields,
			InputPaths: []string{"left.csv", "right.csv"},
			Params: map[string]string{
				"field_specs_json": `[{"source_field":"created_at","target_field":"year","operation":"substring","start":0,"length":4}]`,
			},
		}},
	}
	prepared := prepareDataTaskWorkflowPlanForExecution("", nil, records, plan)
	if got := strings.Join(prepared.Actions[0].InputPaths, ","); got != "left.csv,right.csv" {
		t.Fatalf("InputPaths=%q, want ambiguous inputs preserved", got)
	}
	if errText := dataTaskWorkflowStagingGuardError(records, prepared); !strings.Contains(errText, "single-record-set") {
		t.Fatalf("errText=%q, want normal guard for ambiguous inputs", errText)
	}
}

func TestDataTaskInvalidRecordActionFallbackConvertsLookupDeriveToEnrich(t *testing.T) {
	records := []dataTaskWorkflowRecord{
		{
			Result: &dataquery.Result{ConsumedPaths: []string{"orders.csv", "lookup.csv"}},
		},
		{
			Err: `execute data task: data action failed action_id="join_records" action_kind="join_records": join_records left field "canonical_code" was not found in left input orders.csv`,
		},
	}
	plan := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "lookup.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:             "derive_lookup_field",
			Kind:           dataquery.DataActionDeriveFields,
			InputPaths:     []string{"orders.csv", "lookup.csv"},
			OutputArtifact: "orders_with_canonical_code.json",
			Params: map[string]string{
				"source_field":     "raw_label",
				"field_specs_json": `[{"source_field":"raw_label","target_field":"placeholder","operation":"trim"}]`,
			},
		}},
	}
	fallback, ok := dataTaskInvalidRecordActionFallback(records, plan, "data planning incomplete: derive_fields with 2 input_paths")
	if !ok {
		t.Fatalf("fallback not generated")
	}
	if len(fallback.Actions) != 1 || fallback.Actions[0].Kind != dataquery.DataActionEnrichRecords {
		t.Fatalf("fallback actions=%+v, want enrich_records", fallback.Actions)
	}
	action := fallback.Actions[0]
	if got := strings.Join(action.InputPaths, ","); got != "orders.csv,lookup.csv" {
		t.Fatalf("InputPaths=%v", action.InputPaths)
	}
	spec := action.Params["lookup_specs_json"]
	for _, want := range []string{`"target_field":"canonical_code"`, `"base_fields":["raw_label"]`, `"lookup_value_field":"canonical_code"`, `"match_mode":"contains"`} {
		if !strings.Contains(spec, want) {
			t.Fatalf("lookup_specs_json missing %s: %s", want, spec)
		}
	}
	if !fallback.ContinueAfter {
		t.Fatalf("ContinueAfter=false, want continuation")
	}
}

func TestDataTaskMissingJoinFieldFallbackMaterializesEnrichment(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{Status: "ready"},
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{
			{
				ID:      "po_enriched_vendor",
				Kind:    string(dataquery.DataActionEnrichRecords),
				Headers: []string{"po_id", "vendor_name_raw", "category_raw", "canonical_vendor_id", "year", "currency"},
				Fields: map[string]string{
					"artifact_aliases": "po_enriched_vendor.json",
					"json_shape":       "array[50]",
				},
			},
			{
				ID:      "category_resolutions",
				Kind:    string(dataquery.DataActionNormalizeEntities),
				Headers: []string{"source_value", "canonical_id", "canonical_label", "status"},
				Fields: map[string]string{
					"artifact_aliases": "category_resolutions.json",
					"json_shape":       "array[27]",
				},
			},
		}},
	}}
	plan := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:   "join_po_query",
			Kind: dataquery.DataActionJoinRecords,
			Params: map[string]string{
				"left_path":    "po_enriched_vendor.json",
				"right_path":   "queries.csv",
				"left_fields":  `["canonical_vendor_id","category_code","year","currency"]`,
				"right_fields": `["vendor_id","category_code","year","currency"]`,
			},
		}},
	}
	fallback, ok := dataTaskMissingJoinFieldFallback(records, plan, `execute data task: data action failed action_id="join_po_query" action_kind="join_records": join_records left field "category_code" was not found in left input po_enriched_vendor.json`)
	if !ok {
		t.Fatalf("fallback not generated")
	}
	if len(fallback.Actions) != 1 {
		t.Fatalf("actions=%+v, want one enrichment action", fallback.Actions)
	}
	action := fallback.Actions[0]
	if action.Kind != dataquery.DataActionEnrichRecords {
		t.Fatalf("kind=%q, want enrich_records", action.Kind)
	}
	if got := strings.Join(action.InputPaths, ","); got != "po_enriched_vendor.json,category_resolutions" {
		t.Fatalf("input_paths=%v", action.InputPaths)
	}
	raw := action.Params["lookup_specs_json"]
	for _, want := range []string{`"target_field":"category_code"`, `"category_raw"`, `"lookup_value_field":"canonical_id"`, `"source_value"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("lookup_specs_json missing %s: %s", want, raw)
		}
	}
	if !fallback.ContinueAfter {
		t.Fatalf("ContinueAfter=false, want true")
	}
}

func TestDataTaskHistoricalMissingJoinFieldFallbackUsesLaterMappingArtifact(t *testing.T) {
	records := []dataTaskWorkflowRecord{
		{
			Plan: dataquery.TaskPlan{Status: "ready"},
			Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
				ID:      "orders_records",
				Kind:    string(dataquery.DataActionExtractRecords),
				Headers: []string{"order_id", "raw_label", "amount"},
				Fields: map[string]string{
					"artifact_aliases": "orders.json",
					"json_shape":       "array[2]",
				},
			}}},
			Err: `execute data task: data action failed action_id="join_orders_ref" action_kind="join_records": join_records left field "label_code" was not found in left input orders.json`,
		},
		{
			Plan: dataquery.TaskPlan{Status: "ready"},
			Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
				ID:      "label_resolution",
				Kind:    string(dataquery.DataActionNormalizeEntities),
				Headers: []string{"source_value", "canonical_id", "status"},
				Fields: map[string]string{
					"artifact_aliases": "label_resolution.json",
					"json_shape":       "array[2]",
				},
			}}},
		},
	}
	fallback, ok := dataTaskHistoricalMissingJoinFieldFallback(records, dataquery.TaskPlan{Status: "ready", ContinueAfter: true})
	if !ok {
		t.Fatalf("historical fallback not generated")
	}
	if len(fallback.Actions) != 1 || fallback.Actions[0].Kind != dataquery.DataActionEnrichRecords {
		t.Fatalf("fallback actions=%+v, want one enrich_records action", fallback.Actions)
	}
	action := fallback.Actions[0]
	if got := strings.Join(action.InputPaths, ","); got != "orders.json,label_resolution" {
		t.Fatalf("input_paths=%v", action.InputPaths)
	}
	raw := action.Params["lookup_specs_json"]
	for _, want := range []string{`"target_field":"label_code"`, `"raw_label"`, `"lookup_value_field":"canonical_id"`, `"source_value"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("lookup_specs_json missing %s: %s", want, raw)
		}
	}
}

func TestDataTaskHistoricalMissingJoinFieldFallbackDoesNotRepeatExistingOutput(t *testing.T) {
	records := []dataTaskWorkflowRecord{
		{
			Plan: dataquery.TaskPlan{Status: "ready"},
			Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
				ID:      "orders_records",
				Kind:    string(dataquery.DataActionExtractRecords),
				Headers: []string{"order_id", "raw_label", "amount"},
				Fields: map[string]string{
					"artifact_aliases": "orders.json",
					"json_shape":       "array[2]",
				},
			}}},
			Err: `execute data task: data action failed action_id="join_orders_ref" action_kind="join_records": join_records left field "label_code" was not found in left input orders.json`,
		},
		{
			Plan: dataquery.TaskPlan{Status: "ready"},
			Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
				ID:      "label_resolution",
				Kind:    string(dataquery.DataActionNormalizeEntities),
				Headers: []string{"source_value", "canonical_id", "status"},
				Fields: map[string]string{
					"artifact_aliases": "label_resolution.json",
					"json_shape":       "array[2]",
				},
			}}},
		},
		{
			Plan: dataquery.TaskPlan{Status: "ready"},
			Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
				ID:      "orders_with_label_code",
				Kind:    string(dataquery.DataActionEnrichRecords),
				Headers: []string{"order_id", "raw_label", "amount", "label_code"},
				Fields: map[string]string{
					"artifact_aliases": "orders_with_label_code.json",
					"json_shape":       "array[2]",
				},
			}}},
		},
	}
	if fallback, ok := dataTaskHistoricalMissingJoinFieldFallback(records, dataquery.TaskPlan{Status: "ready", ContinueAfter: true}); ok {
		t.Fatalf("fallback=%+v, want no repeated fallback after output artifact already exists", fallback)
	}
}

func TestDataTaskHistoricalMissingJoinFieldFallbackDoesNotRepeatSamePlan(t *testing.T) {
	records := []dataTaskWorkflowRecord{
		{
			Plan: dataquery.TaskPlan{Status: "ready"},
			Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
				ID:      "orders_records",
				Kind:    string(dataquery.DataActionExtractRecords),
				Headers: []string{"order_id", "raw_label", "amount"},
				Fields: map[string]string{
					"artifact_aliases": "orders.json",
					"json_shape":       "array[2]",
				},
			}}},
			Err: `execute data task: data action failed action_id="join_orders_ref" action_kind="join_records": join_records left field "label_code" was not found in left input orders.json`,
		},
		{
			Plan: dataquery.TaskPlan{Status: "ready"},
			Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
				ID:      "label_resolution",
				Kind:    string(dataquery.DataActionNormalizeEntities),
				Headers: []string{"source_value", "canonical_id", "status"},
				Fields: map[string]string{
					"artifact_aliases": "label_resolution.json",
					"json_shape":       "array[2]",
				},
			}}},
		},
	}
	first, ok := dataTaskHistoricalMissingJoinFieldFallback(records, dataquery.TaskPlan{Status: "ready", ContinueAfter: true})
	if !ok {
		t.Fatalf("historical fallback not generated")
	}
	records = append(records, dataTaskWorkflowRecord{Plan: first, Err: "historical missing join fallback already attempted"})
	if fallback, ok := dataTaskHistoricalMissingJoinFieldFallback(records, dataquery.TaskPlan{Status: "ready", ContinueAfter: true}); ok {
		t.Fatalf("fallback=%+v, want no duplicate fallback plan", fallback)
	}
}

func TestDataTaskMaterialDiscoveryFallbackPreservesValidationContract(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv", "e.csv", "f.csv", "g.csv", "h.csv", "i.csv", "j.csv", "k.csv", "l.csv"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "a.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "b.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "too_broad",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv", "e.csv", "f.csv", "g.csv", "h.csv", "i.csv", "j.csv", "k.csv", "l.csv"},
			Script:     `emit_result("0")`,
		}},
	}
	got, ok := dataTaskMaterialDiscoveryFallback(nil, plan, "broad transform")
	if !ok {
		t.Fatalf("expected material discovery fallback")
	}
	if !got.CoverageContract.DecisionRecordsRequired || !got.CoverageContract.ContributionLedgerRequired || !got.CoverageContract.ReconcileRequired {
		t.Fatalf("CoverageContract=%+v, want validation flags preserved", got.CoverageContract)
	}
	if len(got.CoverageContract.RequiredMaterials) != 0 {
		t.Fatalf("RequiredMaterials=%+v, want discovery fallback to avoid blocking required-material loop", got.CoverageContract.RequiredMaterials)
	}
}

func TestDataTaskWorkflowStateIncludesAllowedNextActions(t *testing.T) {
	required := []dataquery.CoverageMaterial{
		{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
		{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
	}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials:    required,
				RuleCoverageRequired: true,
			},
		},
		Result: &dataquery.Result{ConsumedPaths: []string{"records.csv", "rules.md"}},
	}}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if state.NextStage != "derive_rules" {
		t.Fatalf("NextStage=%q, want derive_rules", state.NextStage)
	}
	if strings.Join(state.AllowedNextActions, ",") != string(dataquery.DataActionDeriveRules) {
		t.Fatalf("AllowedNextActions=%v, want only derive_rules", state.AllowedNextActions)
	}
}

func TestDataTaskWorkflowStateDisablesCustomTransformAfterScriptFailure(t *testing.T) {
	required := []dataquery.CoverageMaterial{
		{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
	}
	records := []dataTaskWorkflowRecord{
		{
			Plan: dataquery.TaskPlan{
				CoverageContract: dataquery.CoverageContract{
					RequiredMaterials:          required,
					RuleCoverageRequired:       true,
					ContributionLedgerRequired: true,
					ReconcileRequired:          true,
				},
			},
			Result: &dataquery.Result{
				ConsumedPaths: []string{"records.csv"},
				RuleCoverage:  []dataquery.RuleCoverageRecord{{RuleID: dataquery.LooseText("R1"), RuleText: dataquery.LooseText("rule"), Status: dataquery.LooseText("applied")}},
			},
		},
		{
			Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
				ID:     "wide_script",
				Kind:   dataquery.DataActionCustomTransform,
				Script: "emit_result('x')",
			}}},
			Err: `data planning incomplete: action 1 (custom_transform) is too large for one atomic data action`,
		},
	}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if !state.CustomTransformDisabled {
		t.Fatalf("CustomTransformDisabled=false, state=%+v", state)
	}
	if strings.Contains(strings.Join(state.AllowedNextActions, ","), string(dataquery.DataActionCustomTransform)) {
		t.Fatalf("AllowedNextActions=%v, want custom_transform removed after script failure", state.AllowedNextActions)
	}
	if state.NextStage != "prepare_contribution_inputs" {
		t.Fatalf("NextStage=%q, want prepare_contribution_inputs", state.NextStage)
	}
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:         "retry_script",
		Kind:       dataquery.DataActionCustomTransform,
		InputPaths: []string{"records.csv"},
		Script:     "emit_result('x')",
	}}}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if !strings.Contains(errText, "allowed_next_actions") || strings.Contains(errText, "custom_transform]") {
		t.Fatalf("errText=%q, want custom_transform rejected by filtered allowed_next_actions", errText)
	}
}

func TestDataTaskWorkflowStateKeepsCustomTransformDisabledAfterTypedProgressWithPendingLedgers(t *testing.T) {
	records := []dataTaskWorkflowRecord{
		{
			Plan: dataquery.TaskPlan{
				CoverageContract: dataquery.CoverageContract{
					ContributionLedgerRequired: true,
					ReconcileRequired:          true,
				},
				Actions: []dataquery.DataAction{{
					ID:     "wide_script",
					Kind:   dataquery.DataActionCustomTransform,
					Script: "emit_result('x')",
				}},
			},
			Err: `execute data task: data action failed action_id="wide_script" action_kind="custom_transform": data task script failed: exit status 1`,
		},
		{
			Plan: dataquery.TaskPlan{
				CoverageContract: dataquery.CoverageContract{
					ContributionLedgerRequired: true,
					ReconcileRequired:          true,
				},
				Actions: []dataquery.DataAction{{
					ID:   "derive_fields",
					Kind: dataquery.DataActionDeriveFields,
				}},
			},
			Result: &dataquery.Result{
				ConsumedPaths: []string{"records.csv"},
				Artifacts: []dataquery.DataArtifact{{
					ID:      "records_derived",
					Kind:    string(dataquery.DataActionDeriveFields),
					Headers: []string{"id", "amount"},
					Fields: map[string]string{
						"artifact_aliases": "records_derived.json",
						"json_shape":       "array[2]",
					},
				}},
			},
		},
	}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if !state.CustomTransformDisabled {
		t.Fatalf("CustomTransformDisabled=false, state=%+v", state)
	}
	if strings.Contains(strings.Join(state.AllowedNextActions, ","), string(dataquery.DataActionCustomTransform)) {
		t.Fatalf("AllowedNextActions=%v, want no custom_transform while ledgers remain pending", state.AllowedNextActions)
	}
}

func TestDataTaskWorkflowStateAdvancesAfterJoinedArtifactMaterializesEntityStage(t *testing.T) {
	contract := dataquery.CoverageContract{
		RequiredMaterials: []dataquery.CoverageMaterial{
			{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			{Path: "lookup.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
		},
		EntityResolutionRequired:   true,
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{CoverageContract: contract},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"records.csv", "lookup.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:          "joined_records",
				Kind:        string(dataquery.DataActionJoinRecords),
				SourcePaths: []string{"records.csv", "lookup.csv"},
				Headers:     []string{"record_id", "lookup_id", "amount"},
				Fields: map[string]string{
					"artifact_aliases": "joined_records,joined_records.json",
					"json_shape":       "array(len=2,item=object(keys=record_id,lookup_id,amount))",
				},
			}},
		},
	}}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{CoverageContract: contract})
	if !state.EntityStageMaterialized {
		t.Fatalf("EntityStageMaterialized=false, state=%+v", state)
	}
	if state.NextStage != "prepare_contribution_inputs" {
		t.Fatalf("NextStage=%q, want prepare_contribution_inputs; state=%+v", state.NextStage, state)
	}
	if !slices.Contains(state.AllowedNextActions, string(dataquery.DataActionComputeContribs)) {
		t.Fatalf("AllowedNextActions=%v, want compute_contributions after joined artifact", state.AllowedNextActions)
	}
}

func TestDataTaskWorkflowStateCoversMaterialGroupByConsumedMembers(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{
					{Path: "evidence/group", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
					{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				},
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"evidence/group/a.txt", "rules.md"},
			Artifacts: []dataquery.DataArtifact{{
				ID:          "evidence_records",
				SourcePaths: []string{"evidence/group/a.txt"},
			}},
		},
	}}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if !state.MaterialCoverageSufficient || state.MissingRequiredMaterialCount != 0 {
		t.Fatalf("state=%+v, want directory-like required material covered by consumed members", state)
	}
	if dataTaskCoveragePathCovered(dataTaskWorkflowCoveredMaterialPaths(records, dataquery.TaskPlan{}, 0), "rules") {
		t.Fatalf("extensionless path rules should not be covered by unrelated rules.md exact file")
	}
}

func TestDataTaskActionInputAvailabilityDoesNotTrustCurrentPlanInputs(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: []string{"source.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID: "known_records",
				Fields: map[string]string{
					"artifact_aliases": "known_records,known_records.json",
				},
			}},
		},
	}}
	plan := dataquery.TaskPlan{
		InputPaths: []string{"future_alias"},
		Actions: []dataquery.DataAction{{
			ID:         "sum_future",
			Kind:       dataquery.DataActionComputeContribs,
			InputPaths: []string{"future_alias"},
		}},
	}
	errText := dataTaskActionInputAvailabilityGuardError(records, plan, plan.Actions[0], 0)
	if !strings.Contains(errText, "future_alias") || !strings.Contains(errText, "unavailable input_path") {
		t.Fatalf("errText=%q, want future alias rejected before execution", errText)
	}
}

func TestDataTaskActionInputAvailabilityUsesNestedArtifactAliases(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			Artifacts: []dataquery.DataArtifact{{
				ID: "inventory",
				Children: []dataquery.DataArtifact{{
					ID:          "child_records",
					SourcePaths: []string{"source.csv"},
					Fields: map[string]string{
						"artifact_aliases": "child_records,child_records.json",
					},
				}},
			}},
		},
	}}
	plan := dataquery.TaskPlan{
		Actions: []dataquery.DataAction{{
			ID:         "sum_child",
			Kind:       dataquery.DataActionComputeContribs,
			InputPaths: []string{"child_records"},
		}},
	}
	if errText := dataTaskActionInputAvailabilityGuardError(records, plan, plan.Actions[0], 0); errText != "" {
		t.Fatalf("errText=%q, want nested artifact alias available", errText)
	}
}

func TestDataTaskWorkflowCoverageContractDemotesNewRequiredMaterialsOutsideCurrentBatch(t *testing.T) {
	previous := dataquery.CoverageContract{
		RequiredMaterials: []dataquery.CoverageMaterial{
			{Path: "core_a.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			{Path: "core_b.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
		},
		ContributionLedgerRequired: true,
	}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{CoverageContract: previous},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"core_a.csv"},
		},
	}}
	current := dataquery.TaskPlan{
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "core_a.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "core_b.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "next_batch.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "candidate_1.txt", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "candidate_2.txt", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			ContributionLedgerRequired: true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "read_next",
			Kind:       dataquery.DataActionExtractRecords,
			InputPaths: []string{"next_batch.csv"},
		}},
	}
	contract := dataTaskWorkflowCoverageContract(records, current)
	required := dataTaskCoverageMaterialKeySet(contract.RequiredMaterials)
	requiredKey := func(path string) string {
		return dataTaskCoverageMaterialKey(dataquery.CoverageMaterial{Path: path, UsageMode: dataquery.MaterialUseScriptConsumed})
	}
	for _, want := range []string{"core_a.csv", "core_b.csv", "next_batch.csv"} {
		if !required[requiredKey(want)] {
			t.Fatalf("required=%v, missing %q", contract.RequiredMaterials, want)
		}
	}
	for _, notWant := range []string{"candidate_1.txt", "candidate_2.txt"} {
		if required[requiredKey(notWant)] {
			t.Fatalf("required=%v, %q should have been demoted to optional", contract.RequiredMaterials, notWant)
		}
	}
	optional := dataTaskCoverageMaterialKeySet(contract.OptionalMaterials)
	for _, want := range []string{"candidate_1.txt", "candidate_2.txt"} {
		if !optional[requiredKey(want)] {
			t.Fatalf("optional=%v, missing demoted candidate %q", contract.OptionalMaterials, want)
		}
	}
}

func TestDataTaskWorkflowCoverageContractDoesNotPromoteAuxiliaryEvidenceWhenUserMaterialsArePinned(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "rules.md", Purpose: dataTaskUserExplicitMaterialPurpose, Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "orders.csv", Purpose: dataTaskUserExplicitMaterialPurpose, Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			ContributionLedgerRequired: true,
		}},
		Result: &dataquery.Result{ConsumedPaths: []string{"rules.md", "orders.csv"}},
	}}
	current := dataquery.TaskPlan{
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "rules.md", Purpose: dataTaskUserExplicitMaterialPurpose, Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "orders.csv", Purpose: dataTaskUserExplicitMaterialPurpose, Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "evidence/a.txt", Purpose: "auxiliary evidence discovered while resolving entities", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			ContributionLedgerRequired: true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "read_aux",
			Kind:       dataquery.DataActionExtractRecords,
			InputPaths: []string{"evidence/a.txt"},
		}},
	}
	contract := dataTaskWorkflowCoverageContract(records, current)
	required := dataTaskCoverageMaterialKeySet(contract.RequiredMaterials)
	keyFor := func(path string) string {
		return dataTaskCoverageMaterialKey(dataquery.CoverageMaterial{Path: path, UsageMode: dataquery.MaterialUseScriptConsumed})
	}
	for _, want := range []string{"rules.md", "orders.csv"} {
		if !required[keyFor(want)] {
			t.Fatalf("required=%+v, missing user-pinned material %q", contract.RequiredMaterials, want)
		}
	}
	if required[keyFor("evidence/a.txt")] {
		t.Fatalf("required=%+v, auxiliary current-batch evidence should not become a workflow hard gate", contract.RequiredMaterials)
	}
	optional := dataTaskCoverageMaterialKeySet(contract.OptionalMaterials)
	if !optional[keyFor("evidence/a.txt")] {
		t.Fatalf("optional=%+v, want auxiliary evidence preserved as optional material", contract.OptionalMaterials)
	}
}

func TestDataTaskScopePlanToCurrentBatchDoesNotCarryHistoricalRequiredMaterials(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "a.csv", Purpose: dataTaskUserExplicitMaterialPurpose, Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "b.csv", Purpose: dataTaskUserExplicitMaterialPurpose, Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			ReconcileRequired: true,
		}},
		Result: &dataquery.Result{ConsumedPaths: []string{"a.csv", "b.csv"}},
	}}
	plan := dataquery.TaskPlan{
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "a.csv", Purpose: dataTaskUserExplicitMaterialPurpose, Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "b.csv", Purpose: dataTaskUserExplicitMaterialPurpose, Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "c.txt", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
		},
		Actions: []dataquery.DataAction{{
			ID:         "read_c",
			Kind:       dataquery.DataActionExtractRecords,
			InputPaths: []string{"c.txt"},
		}},
	}
	scoped := dataTaskScopePlanToCurrentBatch(records, plan)
	requiredPaths := scoped.CoverageContract.RequiredRunnerInputPaths()
	if strings.Join(requiredPaths, ",") != "c.txt" {
		t.Fatalf("current batch required paths=%v, want only c.txt", requiredPaths)
	}
	if !scoped.CoverageContract.ReconcileRequired {
		t.Fatalf("workflow ledger flags were not carried into current batch contract: %+v", scoped.CoverageContract)
	}
}

func TestPrepareDataTaskWorkflowPlanForExecutionScopesInitialActionBatch(t *testing.T) {
	candidates := []dataquery.CandidateFile{
		{Path: "rules.md", Kind: "text"},
		{Path: "orders.csv", Kind: "csv"},
		{Path: "attachments/a.txt", Kind: "text"},
		{Path: "attachments/b.txt", Kind: "text"},
	}
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"rules.md", "orders.csv", "attachments/a.txt", "attachments/b.txt"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "attachments/a.txt", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "attachments/b.txt", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			EntityResolutionRequired:   true,
			ReconcileRequired:          true,
			DecisionRecordsRequired:    true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "derive_rules",
			Kind:       dataquery.DataActionDeriveRules,
			InputPaths: []string{"rules.md"},
		}, {
			ID:         "derive_orders",
			Kind:       dataquery.DataActionDeriveFields,
			InputPaths: []string{"orders.csv"},
		}},
		ContinueAfter: true,
	}
	got := prepareDataTaskWorkflowPlanForExecution("根据 rules.md、orders.csv 和附件计算", candidates, nil, plan)
	if strings.Join(got.InputPaths, ",") != "rules.md,orders.csv" {
		t.Fatalf("InputPaths=%v, want only current action inputs", got.InputPaths)
	}
	required := dataTaskCoverageMaterialKeySet(got.CoverageContract.RequiredMaterials)
	requiredKey := func(path string) string {
		return dataTaskCoverageMaterialKey(dataquery.CoverageMaterial{Path: path, UsageMode: dataquery.MaterialUseScriptConsumed})
	}
	for _, want := range []string{"rules.md", "orders.csv"} {
		if !required[requiredKey(want)] {
			t.Fatalf("required=%+v, missing current batch material %q", got.CoverageContract.RequiredMaterials, want)
		}
	}
	for _, notWant := range []string{"attachments/a.txt", "attachments/b.txt"} {
		if required[requiredKey(notWant)] {
			t.Fatalf("required=%+v, %q should be optional until a later action consumes it", got.CoverageContract.RequiredMaterials, notWant)
		}
	}
	optional := dataTaskCoverageMaterialKeySet(got.CoverageContract.OptionalMaterials)
	for _, want := range []string{"attachments/a.txt", "attachments/b.txt"} {
		if !optional[requiredKey(want)] {
			t.Fatalf("optional=%+v, missing deferred material %q", got.CoverageContract.OptionalMaterials, want)
		}
	}
	if !got.CoverageContract.RuleCoverageRequired || !got.CoverageContract.ContributionLedgerRequired || !got.CoverageContract.EntityResolutionRequired || !got.CoverageContract.ReconcileRequired || !got.CoverageContract.DecisionRecordsRequired {
		t.Fatalf("validation ledgers were lost: %+v", got.CoverageContract)
	}
}

func TestPrepareDataTaskWorkflowPlanForExecutionPreservesScriptOnlyInputs(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv", "rules.md"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
		},
		Script: "result = {'answer': 'ok'}",
	}
	got := prepareDataTaskWorkflowPlanForExecution("", nil, nil, plan)
	if strings.Join(got.InputPaths, ",") != "orders.csv,rules.md" {
		t.Fatalf("InputPaths=%v, want script-only inputs preserved", got.InputPaths)
	}
	if strings.Join(got.CoverageContract.RequiredPaths(), ",") != "orders.csv,rules.md" {
		t.Fatalf("RequiredPaths=%v, want script-only requirements preserved", got.CoverageContract.RequiredPaths())
	}
}

func TestDataTaskWorkflowStateDoesNotRegressCoverageWhenNextPlanOmitsRequiredMaterials(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{
					{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				},
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"records.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:          "records_artifact",
				Kind:        string(dataquery.DataActionExtractRecords),
				SourcePaths: []string{"records.csv"},
			}},
		},
	}}
	current := dataquery.TaskPlan{
		CoverageContract: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
	}
	state := dataTaskWorkflowState(records, current)
	if !state.MaterialCoverageSufficient {
		t.Fatalf("state=%+v, want historical material progress to keep coverage sufficient", state)
	}
	if state.NextStage != "prepare_contribution_inputs" {
		t.Fatalf("NextStage=%q, want prepare_contribution_inputs; state=%+v", state.NextStage, state)
	}
	if !slices.Contains(state.AllowedNextActions, string(dataquery.DataActionComputeContribs)) {
		t.Fatalf("AllowedNextActions=%v, want compute_contributions after material progress", state.AllowedNextActions)
	}
}

func TestDataTaskWorkflowStateIncludesTypedActionScaffold(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials:          []dataquery.CoverageMaterial{{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed}},
				ContributionLedgerRequired: true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"records.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:      "records",
				Kind:    "extract_records",
				Headers: []string{"id", "amount", "status"},
				Fields: map[string]string{
					"artifact_aliases": "records,records.json",
					"json_shape":       "array(len=2,item=object(keys=id,amount,status))",
				},
			}},
		},
	}}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if state.NextStage != "prepare_contribution_inputs" {
		t.Fatalf("NextStage=%q, want prepare_contribution_inputs", state.NextStage)
	}
	if !slices.Contains(state.AllowedNextActions, string(dataquery.DataActionFilterRecords)) {
		t.Fatalf("AllowedNextActions=%v, want filter_records available before contribution calculation", state.AllowedNextActions)
	}
	var sawCompute bool
	var sawFilter bool
	for _, scaffold := range state.ActionScaffold {
		if scaffold.Kind == string(dataquery.DataActionFilterRecords) {
			sawFilter = true
			if scaffold.InputPath != "records" || scaffold.ParamsTemplate["filters_json"] == "" {
				t.Fatalf("filter scaffold=%+v, want input path and filter template", scaffold)
			}
		}
		if scaffold.Kind != string(dataquery.DataActionComputeContribs) {
			continue
		}
		sawCompute = true
		if scaffold.InputPath != "records" || !strings.Contains(strings.Join(scaffold.Fields, ","), "amount") || scaffold.ParamsTemplate["value_field"] == "" {
			t.Fatalf("compute scaffold=%+v, want input path, fields, and params template", scaffold)
		}
	}
	if !sawCompute {
		t.Fatalf("ActionScaffold=%+v, want compute_contributions scaffold", state.ActionScaffold)
	}
	if !sawFilter {
		t.Fatalf("ActionScaffold=%+v, want filter_records scaffold", state.ActionScaffold)
	}
}

func TestDataTaskWorkflowGuardAcceptsFilterRecordsWithSpec(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials:          []dataquery.CoverageMaterial{{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed}},
				ContributionLedgerRequired: true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"records.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:      "records",
				Kind:    "extract_records",
				Headers: []string{"id", "amount", "status"},
				Fields:  map[string]string{"artifact_aliases": "records", "json_shape": "array(len=3,item=object(keys=id,amount,status))"},
			}},
		},
	}}
	plan := dataquery.TaskPlan{ContinueAfter: true, Actions: []dataquery.DataAction{{
		ID:             "filter_paid",
		Kind:           dataquery.DataActionFilterRecords,
		InputPaths:     []string{"records"},
		OutputArtifact: "paid_records",
		Params:         map[string]string{"filters_json": `[{"field":"status","op":"eq","value":"paid"}]`},
	}}}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("filter_records guard err=%q, want pass", errText)
	}
	plan.Actions[0].Params = nil
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if !strings.Contains(errText, "filter_records") || !strings.Contains(errText, "filters") {
		t.Fatalf("errText=%q, want missing filters guard", errText)
	}
}

func TestDataTaskWorkflowStagingAllowsRecordMaterializationAtEntityStage(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{
					{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
					{Path: "vendors.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				},
				RuleCoverageRequired:     true,
				EntityResolutionRequired: true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "vendors.csv"},
			RuleCoverage:  []dataquery.RuleCoverageRecord{{RuleID: "R1", RuleText: "normalize rows", Status: "applied"}},
		},
	}}
	plan := dataquery.TaskPlan{ContinueAfter: true, Actions: []dataquery.DataAction{{
		ID:             "extract_orders",
		Kind:           dataquery.DataActionExtractRecords,
		InputPaths:     []string{"orders.csv"},
		OutputArtifact: "order_records",
		Params:         map[string]string{"limit": "1000"},
	}}}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("extract_records materialization guard err=%q, want pass", errText)
	}
	plan.Actions[0].InputPaths = []string{"attachments.csv"}
	plan.Actions[0].OutputArtifact = "attachment_records"
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("current-batch extract_records guard err=%q, want pass", errText)
	}
	plan.Actions[0].OutputArtifact = ""
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if !strings.Contains(errText, "extract_records") || !strings.Contains(errText, "output_artifact") {
		t.Fatalf("errText=%q, want output_artifact guard", errText)
	}
}

func TestDataTaskWorkflowGuardRejectsBroadCustomTransformEvenWithEmitter(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "a.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "b.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "c.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "d.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "wide",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv"},
			Script:     "emit_result('ok')",
		}},
	}
	errText := dataTaskActionStagingGuardError(plan)
	if !strings.Contains(errText, "too broad for one custom_transform data DAG node") {
		t.Fatalf("errText=%q, want broad custom_transform guard", errText)
	}
}

func TestDataTaskTerminalWorkflowFallbackDoesNotInventEntityResolutionStage(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{
					{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
					{Path: "lookup.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				},
				EntityResolutionRequired:   true,
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"records.csv", "lookup.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:          "records",
				Kind:        string(dataquery.DataActionExtractRecords),
				SourcePaths: []string{"records.csv"},
			}},
		},
	}}
	current := dataquery.TaskPlan{
		Status: "blocked",
		CoverageContract: dataquery.CoverageContract{
			EntityResolutionRequired:   true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
	}
	fallback, reason, ok := dataTaskTerminalWorkflowFallback(records, current)
	if ok {
		t.Fatalf("fallback=%+v reason=%q, want no invented normalize_entities fallback", fallback, reason)
	}
}

func TestDataTaskTerminalWorkflowGuardRejectsPrematureBlockedPlan(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials:          []dataquery.CoverageMaterial{{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed}},
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"records.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:      "records",
				Kind:    string(dataquery.DataActionExtractRecords),
				Headers: []string{"id", "amount", "group"},
				Fields:  map[string]string{"artifact_aliases": "records"},
			}},
		},
	}}
	current := dataquery.TaskPlan{
		Status: "blocked",
		CoverageContract: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
	}
	guard := dataTaskTerminalWorkflowGuardResult(records, current)
	if guard.Empty() {
		t.Fatal("typed terminal guard should not be empty")
	}
	if guard.Code != "unfinished_validation_stage" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want unfinished_validation_stage with one violation", guard)
	}
	errText := dataTaskTerminalWorkflowGuardError(records, current)
	for _, want := range []string{"terminal status", "compute_contributions", "legal next actions"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("errText=%q, want %q", errText, want)
		}
	}
}

func TestDataTaskWorkflowStagingGuardResultCarriesTypedCode(t *testing.T) {
	guard := dataTaskWorkflowStagingGuardResult(nil, dataquery.TaskPlan{Status: "ready"})
	if guard.Empty() {
		t.Fatal("guard should reject ready plan without executable body")
	}
	if guard.Code != "missing_executable_body" {
		t.Fatalf("guard.Code=%q, want missing_executable_body", guard.Code)
	}
	if len(guard.Violations) != 1 || guard.Violations[0].Code != "missing_executable_body" {
		t.Fatalf("guard.Violations=%+v, want typed missing_executable_body payload", guard.Violations)
	}
	if !strings.Contains(guard.ErrorText(), "no executable body") {
		t.Fatalf("guard text=%q, want executable body guidance", guard.ErrorText())
	}
}

func TestDataTaskActionDependencyGuardResultCarriesTypedCode(t *testing.T) {
	action := dataquery.DataAction{
		ID:         "derive_status",
		Kind:       dataquery.DataActionDeriveFields,
		InputPaths: []string{"records"},
	}
	guard := dataTaskActionDependencyGuardResult(nil, dataquery.TaskPlan{Actions: []dataquery.DataAction{action}}, action, 0)
	if guard.Empty() {
		t.Fatal("guard should reject derive_fields without specs")
	}
	if guard.Code != "missing_action_spec" {
		t.Fatalf("guard.Code=%q, want missing_action_spec", guard.Code)
	}
	if len(guard.Violations) != 1 || guard.Violations[0].ActionID != "derive_status" || guard.Violations[0].ActionKind != string(dataquery.DataActionDeriveFields) || guard.Violations[0].IdempotencyKey == "" {
		t.Fatalf("guard.Violations=%+v, want action-shaped typed violation", guard.Violations)
	}
	if !strings.Contains(guard.ErrorText(), "field specification") {
		t.Fatalf("guard text=%q, want field specification guidance", guard.ErrorText())
	}
}

func TestDataTaskWorkflowStagingGuardResultPropagatesActionGuard(t *testing.T) {
	action := dataquery.DataAction{
		ID:         "derive_status",
		Kind:       dataquery.DataActionDeriveFields,
		InputPaths: []string{"records"},
	}
	guard := dataTaskWorkflowStagingGuardResult(nil, dataquery.TaskPlan{
		Status:  "ready",
		Actions: []dataquery.DataAction{action},
	})
	if guard.Empty() {
		t.Fatal("guard should reject derive_fields without specs")
	}
	if guard.Code != "missing_action_spec" {
		t.Fatalf("guard.Code=%q, want child action code missing_action_spec", guard.Code)
	}
	if len(guard.Violations) != 1 || guard.Violations[0].ActionID != "derive_status" {
		t.Fatalf("guard.Violations=%+v, want propagated child action violation", guard.Violations)
	}
	if !strings.Contains(guard.ErrorText(), "field specification") {
		t.Fatalf("guard text=%q, want field specification guidance", guard.ErrorText())
	}
}

func TestNormalizeDataTaskEvaluationUsesTypedGraphWhenCustomDisabled(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials:          []dataquery.CoverageMaterial{{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed}},
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
			Actions: []dataquery.DataAction{{
				ID:         "failed_script",
				Kind:       dataquery.DataActionCustomTransform,
				InputPaths: []string{"records.csv"},
				Script:     "emit_result('x')",
			}},
		},
		Err: "execute data task: data task script failed: exit status 1",
	}, {
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials:          []dataquery.CoverageMaterial{{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed}},
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
			Actions: []dataquery.DataAction{{
				ID:         "records",
				Kind:       dataquery.DataActionExtractRecords,
				InputPaths: []string{"records.csv"},
			}},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"records.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:      "records",
				Kind:    string(dataquery.DataActionExtractRecords),
				Headers: []string{"id", "amount", "group"},
				Fields:  map[string]string{"artifact_aliases": "records"},
			}},
		},
	}}
	eval := normalizeDataTaskEvaluationForWorkflow(records, dataquery.Evaluation{
		Status:     dataquery.EvalContinueTransform,
		Confidence: "medium",
		Reason:     "continue with bounded transform",
	})
	if eval.Status != dataquery.EvalExpandGraph {
		t.Fatalf("Status=%q, want expand_graph", eval.Status)
	}
	if !strings.Contains(eval.Reason, "custom_transform_disabled=true") || !strings.Contains(eval.Reason, "compute_contributions") {
		t.Fatalf("Reason=%q, want typed graph note", eval.Reason)
	}
}

func TestDataTaskWorkflowStateIncludesEnrichRecordsScaffold(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials:          []dataquery.CoverageMaterial{{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed}},
				EntityResolutionRequired:   true,
				ContributionLedgerRequired: true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"records.csv"},
			EntityResolutions: []dataquery.EntityResolutionRecord{{
				SourceValue:    dataquery.LooseText("Acme"),
				CanonicalID:    dataquery.LooseText("V1"),
				CanonicalLabel: dataquery.LooseText("Acme Inc"),
				Status:         dataquery.LooseText("resolved"),
			}},
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "records",
					Kind:    "extract_records",
					Headers: []string{"id", "vendor_name", "amount"},
					Fields: map[string]string{
						"artifact_aliases": "records,records.json",
						"json_shape":       "array(len=1,item=object(keys=id,vendor_name,amount))",
					},
				},
				{
					ID:      "vendor_map",
					Kind:    "normalize_entities",
					Headers: []string{"source_value", "canonical_id", "canonical_label", "status"},
					Fields: map[string]string{
						"artifact_aliases": "vendor_map,vendor_map.json",
						"json_shape":       "array(len=1,item=object(keys=source_value,canonical_id,canonical_label,status))",
					},
				},
			},
		},
	}}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if state.NextStage != "prepare_contribution_inputs" {
		t.Fatalf("NextStage=%q, want prepare_contribution_inputs", state.NextStage)
	}
	var sawEnrich bool
	for _, scaffold := range state.ActionScaffold {
		if scaffold.Kind != string(dataquery.DataActionEnrichRecords) {
			continue
		}
		sawEnrich = true
		if len(scaffold.InputPaths) != 2 || scaffold.InputPaths[0] != "records" || scaffold.InputPaths[1] != "vendor_map" {
			t.Fatalf("enrich scaffold=%+v, want base and mapping aliases", scaffold)
		}
		spec := scaffold.ParamsTemplate["lookup_specs"]
		for _, want := range []string{"source_value", "canonical_id", "base_fields", "lookup_fields"} {
			if !strings.Contains(spec, want) {
				t.Fatalf("lookup_specs=%q, missing %q", spec, want)
			}
		}
	}
	if !sawEnrich {
		t.Fatalf("ActionScaffold=%+v, want enrich_records scaffold", state.ActionScaffold)
	}
}

func TestDataTaskWorkflowStateIncludesReferenceTableEnrichScaffold(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials:          []dataquery.CoverageMaterial{{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed}},
				EntityResolutionRequired:   true,
				ContributionLedgerRequired: true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"records.csv", "reference.csv"},
			EntityResolutions: []dataquery.EntityResolutionRecord{{
				SourceValue: dataquery.LooseText("anything"),
				CanonicalID: dataquery.LooseText("R1"),
				Status:      dataquery.LooseText("resolved"),
			}},
			Artifacts: []dataquery.DataArtifact{
				{
					ID:      "records",
					Kind:    "derive_fields",
					Headers: []string{"id", "raw_label", "description", "amount"},
					Fields: map[string]string{
						"artifact_aliases": "records",
						"json_shape":       "array(len=2,item=object(keys=id,raw_label,description,amount))",
					},
				},
				{
					ID:      "reference",
					Kind:    "derive_fields",
					Headers: []string{"ref_code", "ref_name", "raw_terms", "keywords"},
					Fields: map[string]string{
						"artifact_aliases": "reference",
						"json_shape":       "array(len=2,item=object(keys=ref_code,ref_name,raw_terms,keywords))",
					},
				},
			},
		},
	}}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	var sawReferenceEnrich bool
	for _, scaffold := range state.ActionScaffold {
		if scaffold.Kind != string(dataquery.DataActionEnrichRecords) || len(scaffold.InputPaths) != 2 || scaffold.InputPaths[1] != "reference" {
			continue
		}
		sawReferenceEnrich = true
		spec := scaffold.ParamsTemplate["lookup_specs"]
		for _, want := range []string{`"base_fields"`, `"lookup_fields"`, "raw_label", "description", "raw_terms", "keywords", "ref_code"} {
			if !strings.Contains(spec, want) {
				t.Fatalf("lookup_specs=%q, missing %q", spec, want)
			}
		}
	}
	if !sawReferenceEnrich {
		t.Fatalf("ActionScaffold=%+v, want reference-table enrich scaffold", state.ActionScaffold)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsActionOutsideAllowedNextStage(t *testing.T) {
	required := []dataquery.CoverageMaterial{
		{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
		{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
	}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials:    required,
				RuleCoverageRequired: true,
			},
		},
		Result: &dataquery.Result{ConsumedPaths: []string{"records.csv", "rules.md"}},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials:          required,
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "too_early",
			Kind:       dataquery.DataActionComputeContribs,
			InputPaths: []string{"records.csv"},
			Params: map[string]string{
				"value_field": "amount",
				"group_key":   "all",
				"operation":   "add",
			},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if !strings.Contains(errText, "allowed_next_actions") || !strings.Contains(errText, "derive_rules") {
		t.Fatalf("errText=%q, want allowed_next_actions derive_rules guard", errText)
	}

	plan.Actions = []dataquery.DataAction{{
		ID:         "rules",
		Kind:       dataquery.DataActionDeriveRules,
		InputPaths: []string{"rules.md"},
	}}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("errText=%q, want derive_rules stage action to pass", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsAssembleWithoutReconcile(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:   "answer",
			Kind: dataquery.DataActionAssembleAnswer,
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "prior reconcile report") {
		t.Fatalf("errText=%q, want prior reconcile report guard", errText)
	}

	plan.Actions = []dataquery.DataAction{
		{ID: "compute", Kind: dataquery.DataActionComputeContribs, InputPaths: []string{"records.json"}},
		{ID: "reconcile", Kind: dataquery.DataActionReconcile},
		{ID: "answer", Kind: dataquery.DataActionAssembleAnswer},
	}
	if errText := dataTaskWorkflowStagingGuardError(nil, plan); errText != "" {
		t.Fatalf("errText=%q, want same-batch reconcile producer to satisfy assemble guard", errText)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsUnscheduledTerminalRequiredMaterial(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv", "rules.md"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true},
				{Path: "rules.md", Required: true},
			},
		},
		Actions: []dataquery.DataAction{{
			ID:         "inspect_orders",
			Kind:       dataquery.DataActionInspectMaterial,
			InputPaths: []string{"orders.csv"},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	if !strings.Contains(errText, "not scheduled") || !strings.Contains(errText, "rules.md") {
		t.Fatalf("errText=%q, want terminal required-material scheduling guard", errText)
	}
}

func TestDataTaskCoverageExpansionFallbackCoversTerminalRequiredMaterials(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			RuleCoverageRequired: true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "inspect_orders",
			Kind:       dataquery.DataActionInspectMaterial,
			InputPaths: []string{"orders.csv"},
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	fallback, ok := dataTaskCoverageExpansionFallback(nil, plan, errText)
	if !ok {
		t.Fatal("expected deterministic coverage expansion fallback")
	}
	if !fallback.ContinueAfter {
		t.Fatal("coverage expansion batch must continue the workflow")
	}
	if len(fallback.Actions) != 2 {
		t.Fatalf("Actions=%+v, want extract_records plus derive_rules", fallback.Actions)
	}
	if fallback.Actions[0].Kind != dataquery.DataActionDeriveRules || fallback.Actions[1].Kind != dataquery.DataActionExtractRecords {
		t.Fatalf("Actions=%+v, want derive_rules then extract_records", fallback.Actions)
	}
	if got := fallback.Actions[0].InputPaths; len(got) != 1 || got[0] != "rules.md" {
		t.Fatalf("rule InputPaths=%v, want rules.md", got)
	}
	if got := fallback.Actions[1].InputPaths; len(got) != 1 || got[0] != "orders.csv" {
		t.Fatalf("record InputPaths=%v, want orders.csv", got)
	}
}

func TestDataTaskCoverageExpansionFallbackDoesNotStealNarrowExecutablePlan(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv"},
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputCSVLine,
			ExplanationAllowed: false,
		},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
		},
		Script: `rows = csv_rows("orders.csv")
total = sum(int(r["amount"]) for r in rows)
emit({"answer": "A," + str(total), "output_contract": {"format": "csv_line", "explanation_allowed": False}})`,
	}
	if fallback, ok := dataTaskCoverageExpansionFallback(nil, plan, "missing material coverage before execution"); ok {
		t.Fatalf("fallback=%+v, want narrow executable plan to run directly", fallback)
	}
}

func TestDataTaskCoverageExpansionFallbackCoversBroadCustomPrerequisites(t *testing.T) {
	lines := make([]string, 0, dataTaskComplexCustomScriptLineLimit+1)
	for i := 0; i < dataTaskComplexCustomScriptLineLimit; i++ {
		lines = append(lines, "x = 1")
	}
	lines = append(lines, `emit_result("ok", output_contract={"format":"plain_single_line","explanation_allowed":False})`)
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "final",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"orders.csv", "lookup.json", "notes.bin", "queries.tsv"},
			Script:     strings.Join(lines, "\n"),
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	fallback, ok := dataTaskCoverageExpansionFallback(nil, plan, errText)
	if !ok {
		t.Fatal("expected deterministic coverage expansion fallback")
	}
	if len(fallback.Actions) != 2 {
		t.Fatalf("Actions=%+v, want extract_records plus inspect_material", fallback.Actions)
	}
	if fallback.Actions[0].Kind != dataquery.DataActionExtractRecords {
		t.Fatalf("first action=%+v, want extract_records", fallback.Actions[0])
	}
	if got := fallback.Actions[0].InputPaths; strings.Join(got, ",") != "lookup.json,orders.csv,queries.tsv" {
		t.Fatalf("extract InputPaths=%v", got)
	}
	if fallback.Actions[1].Kind != dataquery.DataActionInspectMaterial {
		t.Fatalf("second action=%+v, want inspect_material", fallback.Actions[1])
	}
	if got := fallback.Actions[1].InputPaths; len(got) != 1 || got[0] != "notes.bin" {
		t.Fatalf("inspect InputPaths=%v", got)
	}
}

func TestDataTaskUserMentionedCandidateMaterialsMatchesExactCandidateNames(t *testing.T) {
	candidates := []dataquery.CandidateFile{
		{Path: "purchase_orders_raw.csv", Kind: "csv"},
		{Path: "rules/data_rules.md", Kind: "text"},
		{Path: "evidence/photo.png", Kind: "image", TextEvidencePaths: []string{"evidence/photo.txt"}},
		{Path: "unmentioned.csv", Kind: "csv"},
	}
	materials := dataTaskUserMentionedCandidateMaterials(
		`请根据 data_rules.md 清洗 purchase_orders_raw.csv，并参考 evidence/photo.png。`,
		candidates,
	)
	var got []string
	modes := map[string]dataquery.CoverageMaterialUseMode{}
	evidence := map[string]string{}
	for _, material := range materials {
		got = append(got, material.Path)
		modes[material.Path] = material.UsageMode
		evidence[material.Path] = material.TextEvidencePath
	}
	sort.Strings(got)
	if strings.Join(got, ",") != "evidence/photo.png,purchase_orders_raw.csv,rules/data_rules.md" {
		t.Fatalf("materials=%v", got)
	}
	if modes["purchase_orders_raw.csv"] != dataquery.MaterialUseScriptConsumed {
		t.Fatalf("csv mode=%q", modes["purchase_orders_raw.csv"])
	}
	if modes["evidence/photo.png"] != dataquery.MaterialUseTextEvidenceConsumed || evidence["evidence/photo.png"] != "evidence/photo.txt" {
		t.Fatalf("image mode=%q evidence=%q", modes["evidence/photo.png"], evidence["evidence/photo.png"])
	}
}

func TestDataTaskUserMentionedCandidateMaterialsRequiresExactFileSignal(t *testing.T) {
	candidates := []dataquery.CandidateFile{
		{Path: "orders.csv", Kind: "csv"},
		{Path: "notes.md", Kind: "text"},
	}
	materials := dataTaskUserMentionedCandidateMaterials("读取一些 csv 和说明材料后计算结果", candidates)
	if len(materials) != 0 {
		t.Fatalf("materials=%+v, want no fuzzy material match", materials)
	}
}

func TestDataTaskCoverageExpansionFallbackCoversRequiredMaterialsForIntermediateBroadPlan(t *testing.T) {
	lines := make([]string, 0, dataTaskComplexCustomScriptLineLimit+1)
	for i := 0; i < dataTaskComplexCustomScriptLineLimit; i++ {
		lines = append(lines, "x = 1")
	}
	lines = append(lines, `emit_result("ok", output_contract={"format":"plain_single_line","explanation_allowed":False})`)
	plan := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "broad_compute",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"orders.csv", "rules.md"},
			Script:     strings.Join(lines, "\n"),
		}},
	}
	fallback, ok := dataTaskCoverageExpansionFallback(nil, plan, "missing material coverage before execution")
	if !ok {
		t.Fatal("expected deterministic coverage expansion fallback")
	}
	if len(fallback.Actions) != 2 {
		t.Fatalf("Actions=%+v, want derive_rules plus extract_records", fallback.Actions)
	}
	if fallback.Actions[0].Kind != dataquery.DataActionDeriveRules || strings.Join(fallback.Actions[0].InputPaths, ",") != "rules.md" {
		t.Fatalf("first action=%+v, want rules.md derive_rules", fallback.Actions[0])
	}
	if fallback.Actions[1].Kind != dataquery.DataActionExtractRecords || strings.Join(fallback.Actions[1].InputPaths, ",") != "orders.csv" {
		t.Fatalf("second action=%+v, want orders.csv extract_records", fallback.Actions[1])
	}
}

func TestApplyDataTaskUserMaterialFloorPreservesExplicitMaterialsAcrossPlanShape(t *testing.T) {
	candidates := []dataquery.CandidateFile{
		{Path: "orders.csv", Kind: "csv"},
		{Path: "lookup.json", Kind: "json"},
	}
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv"},
		CoverageContract: dataquery.CoverageContract{
			OptionalMaterials: []dataquery.CoverageMaterial{
				{Path: "lookup.json", Required: false, UsageMode: dataquery.MaterialUseReferenceOnly},
			},
		},
		Actions: []dataquery.DataAction{{
			ID:         "inspect_orders",
			Kind:       dataquery.DataActionInspectMaterial,
			InputPaths: []string{"orders.csv"},
		}},
	}
	got := applyDataTaskUserMaterialFloor("使用 orders.csv 和 lookup.json 计算", candidates, plan)
	required := got.CoverageContract.RequiredMaterials
	if len(required) != 2 {
		t.Fatalf("required=%+v, want two user-explicit materials", required)
	}
	paths := dataTaskCoverageExpansionMissingPaths(nil, got)
	if strings.Join(paths, ",") != "lookup.json,orders.csv" {
		t.Fatalf("missing paths=%v, want required materials covered by replacement fallback", paths)
	}
	if len(got.CoverageContract.OptionalMaterials) != 0 {
		t.Fatalf("optional=%+v, want explicit required material removed from optional list", got.CoverageContract.OptionalMaterials)
	}
	foundInput := false
	for _, input := range got.InputPaths {
		if input == "lookup.json" {
			foundInput = true
			break
		}
	}
	if !foundInput {
		t.Fatalf("InputPaths=%v, want lookup.json", got.InputPaths)
	}
}

func TestDataTaskMaterialDiscoveryFallbackDoesNotStealTypedActionPlan(t *testing.T) {
	lines := make([]string, 0, dataTaskComplexCustomScriptLineLimit)
	for i := 0; i < dataTaskComplexCustomScriptLineLimit+5; i++ {
		lines = append(lines, fmt.Sprintf("value_%d = %d", i, i))
	}
	lines = append(lines, `emit_result("ok", output_contract={"format":"plain_single_line","explanation_allowed":False})`)
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv"},
		Actions: []dataquery.DataAction{
			{ID: "inspect_a", Kind: dataquery.DataActionInspectMaterial, InputPaths: []string{"a.csv"}},
			{ID: "final_transform", Kind: dataquery.DataActionCustomTransform, InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv"}, Script: strings.Join(lines, "\n")},
		},
	}
	if _, ok := dataTaskMaterialDiscoveryFallback(nil, plan, "broad material custom action requires objective material discovery before execution"); ok {
		t.Fatal("typed action plan was unexpectedly converted to material discovery")
	}
}

func TestDataTaskWorkflowStagingGuardRejectsComputeStageCustomTransformWithPrerequisites(t *testing.T) {
	lines := make([]string, 0, dataTaskOneShotScriptLineSoftLimit)
	for i := 0; i < dataTaskComplexCustomScriptLineLimit+5; i++ {
		lines = append(lines, fmt.Sprintf("value_%d = %d", i, i))
	}
	lines = append(lines, `emit_result("ok", output_contract={"format":"plain_single_line","explanation_allowed":False})`)
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "rules.md", Required: true},
				{Path: "orders.csv", Required: true},
				{Path: "lookup.csv", Required: true},
				{Path: "evidence.csv", Required: true},
			},
		},
		Actions: []dataquery.DataAction{{
			ID:         "final_transform",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"rules.md", "orders.csv", "lookup.csv", "evidence.csv"},
			Script:     strings.Join(lines, "\n"),
		}},
	}
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{
			ConsumedPaths: []string{"rules.md", "orders.csv", "lookup.csv", "evidence.csv"},
		},
	}}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	for _, want := range []string{
		"workflow next_stage=compute_contributions",
		"allows only next actions",
		"custom_transform",
	} {
		if !strings.Contains(errText, want) {
			t.Fatalf("errText=%q, want %q", errText, want)
		}
	}
}

func TestDataTaskPlanStagingGuardAllowsBoundedCustomTransformBelowBroadLimit(t *testing.T) {
	lines := make([]string, 0, dataTaskComplexCustomScriptLineLimit)
	for i := 0; i < dataTaskComplexCustomScriptLineLimit-20; i++ {
		lines = append(lines, fmt.Sprintf("value_%d = %d", i, i))
	}
	lines = append(lines, `emit_result("ok", output_contract={"format":"plain_single_line","explanation_allowed":False})`)
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "a.csv", Required: true},
				{Path: "b.csv", Required: true},
				{Path: "c.csv", Required: true},
				{Path: "d.csv", Required: true},
			},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "bounded_transform",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv"},
			Script:     strings.Join(lines, "\n"),
		}},
	}
	if errText := dataTaskPlanStagingGuardError(plan); errText != "" {
		t.Fatalf("bounded action-level transform should pass staging guard, got %q", errText)
	}
}

func TestPreserveDataTaskWorkflowMaterialCoverageCarriesEarlierRequirements(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			InputPaths: []string{"orders.csv", "rules.md"},
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{
					{Path: "orders.csv", Purpose: "source rows", Required: true},
					{Path: "rules.md", Purpose: "task rules", Required: true},
				},
				RuleCoverageRequired: true,
			},
		},
		Result: &dataquery.Result{
			Answer:        "partial",
			ConsumedPaths: []string{"orders.csv", "rules.md"},
		},
	}}
	current := dataquery.TaskPlan{
		InputPaths: []string{"orders.csv"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{{Path: "orders.csv", Purpose: "source rows", Required: true}},
		},
	}
	next := dataquery.TaskPlan{
		InputPaths: []string{"orders.csv"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{{Path: "orders.csv", Purpose: "source rows", Required: true}},
		},
	}
	got := preserveDataTaskWorkflowMaterialCoverage(records, current, next)
	if strings.Join(got.CoverageContract.RequiredPaths(), ",") != "orders.csv,rules.md" {
		t.Fatalf("RequiredPaths=%v", got.CoverageContract.RequiredPaths())
	}
	if !got.CoverageContract.RuleCoverageRequired {
		t.Fatalf("RuleCoverageRequired=false")
	}
	if strings.Join(got.InputPaths, ",") != "orders.csv,rules.md" {
		t.Fatalf("InputPaths=%v", got.InputPaths)
	}
}

func TestPreserveDataTaskWorkflowMaterialCoverageDropsGeneratedRequiredArtifacts(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			Actions: []dataquery.DataAction{{
				ID:             "extract_orders",
				Kind:           dataquery.DataActionExtractRecords,
				InputPaths:     []string{"orders.csv"},
				OutputArtifact: "orders_records",
			}},
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{{
					Path:      "orders.csv",
					Purpose:   "source rows",
					Required:  true,
					UsageMode: dataquery.MaterialUseScriptConsumed,
				}},
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:   "orders_records",
				Kind: "extract_records",
				Fields: map[string]string{
					"artifact_path":    "/tmp/codrax-data-actions/orders_records.json",
					"artifact_aliases": "orders_records,orders_records.json,artifacts/orders_records,artifacts/orders_records.json",
				},
			}},
		},
	}}
	current := dataquery.TaskPlan{
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{{
				Path:      "orders.csv",
				Purpose:   "source rows",
				Required:  true,
				UsageMode: dataquery.MaterialUseScriptConsumed,
			}},
		},
	}
	next := dataquery.TaskPlan{
		InputPaths: []string{"artifacts/orders_records.json"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Purpose: "source rows", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "orders_records", Purpose: "generated record artifact", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "artifacts/orders_records.json", Purpose: "generated record artifact alias", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
				{Path: "/tmp/codrax-data-actions/orders_records.json", Purpose: "materialized generated record artifact", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			},
		},
	}
	got := preserveDataTaskWorkflowMaterialCoverage(records, current, next)
	if strings.Join(got.CoverageContract.RequiredRunnerInputPaths(), ",") != "orders.csv" {
		t.Fatalf("RequiredRunnerInputPaths=%v, want only original source material", got.CoverageContract.RequiredRunnerInputPaths())
	}
	if !strings.Contains(strings.Join(got.InputPaths, ","), "artifacts/orders_records.json") {
		t.Fatalf("InputPaths=%v, generated artifact should remain available to the next action", got.InputPaths)
	}
	candidates := dataTaskCandidatesWithWorkflowArtifacts(nil, records)
	var found bool
	for _, candidate := range candidates {
		if candidate.Path == "orders_records" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("generated artifact alias missing from candidates: %+v", candidates)
	}
}

func TestDataTaskWorkflowStagingGuardStopsRepeatedCustomTransformNode(t *testing.T) {
	records := []dataTaskWorkflowRecord{
		{Err: `execute data task: data action failed action_id="compute" action_kind="custom_transform": data task script failed: exit status 1`},
		{Err: `execute data task: data action failed action_id="compute" action_kind="custom_transform": data coverage incomplete: result.rows is empty`},
	}
	plan := dataquery.TaskPlan{
		Actions: []dataquery.DataAction{{
			ID:         "compute",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"orders.csv"},
			Script:     `emit_result("ok", output_contract={"format":"plain_single_line","explanation_allowed":False})`,
		}},
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if !strings.Contains(errText, `custom_transform node "compute" already failed 2 time`) ||
		!strings.Contains(errText, "replace it with smaller typed atomic actions") {
		t.Fatalf("errText=%q", errText)
	}
}

func TestDataTaskWorkflowStagingGuardStopsRepeatedCustomTransformClass(t *testing.T) {
	failedPlan := func(id string) dataquery.TaskPlan {
		return dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:         id,
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"orders.csv", "rules.md", "queries.csv", "lookup.csv"},
			Script:     `emit_result("bad", output_contract={"format":"plain_single_line","explanation_allowed":False})`,
		}}}
	}
	records := []dataTaskWorkflowRecord{
		{
			Plan: failedPlan("compute_query_totals"),
			Err:  `execute data task: data action failed action_id="compute_query_totals" action_kind="custom_transform": custom_transform field contract failed: orders.csv line 12 references missing field "derived_total"`,
		},
		{
			Plan: failedPlan("build_clean_rows"),
			Err:  `execute data task: data action failed action_id="build_clean_rows" action_kind="custom_transform": custom_transform field contract failed: orders.csv line 18 references missing field "derived_total"`,
		},
		{
			Plan: failedPlan("compute_totals"),
			Err:  `execute data task: data action failed action_id="compute_totals" action_kind="custom_transform": data task script failed: exit status 1`,
		},
	}
	plan := dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true},
				{Path: "rules.md", Required: true},
				{Path: "queries.csv", Required: true},
				{Path: "lookup.csv", Required: true},
			},
			DecisionRecordsRequired:    true,
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "fresh_action_name",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"orders.csv", "rules.md", "queries.csv", "lookup.csv"},
			Script:     `emit_result("ok", output_contract={"format":"plain_single_line","explanation_allowed":False})`,
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	if !strings.Contains(errText, "workflow already has 3 custom_transform failure") ||
		!strings.Contains(errText, "Do not bypass this by changing action_id") {
		t.Fatalf("errText=%q", errText)
	}
}

func TestDataTaskContinuationPromptIncludesArtifactAccessCatalog(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{Status: "ready"},
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:          "vendor_category_map",
			Kind:        "normalize_entities",
			SourcePaths: []string{"vendors.csv"},
			Headers:     []string{"source_value", "canonical_id", "status"},
			Fields: map[string]string{
				"artifact_aliases": "vendor_category_map,vendor_category_map.json",
				"json_shape":       "array(len=2,item=object(keys=canonical_id,source_value,status))",
			},
			Sample: []string{`{"source_value":"vendor a","canonical_id":"V001","status":"resolved"}`},
		}}},
	}}
	prompt := dataTaskContinuationPrompt(
		"继续计算",
		"/repo",
		TurnPolicy{Route: RouteData, DataTaskKind: "data_aggregation"},
		nil,
		records,
	)
	for _, want := range []string{
		"artifact_access",
		"vendor_category_map",
		"array-shaped",
		"fields",
		"field_samples",
		"vendor a",
		"canonical_id",
		"json_records(alias)",
		"do not call .get()",
		"only reference fields",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("continuation prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestDataTaskContinuationPromptIncludesAuthoritativeCoverageTruth(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{
					{Path: "orders.csv", Required: true},
					{Path: "rules.md", Required: true},
				},
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "rules.md"},
			Artifacts: []dataquery.DataArtifact{{
				ID:          "orders.csv#records",
				Kind:        "extract_records/csv",
				SourcePaths: []string{"orders.csv"},
				Fields: map[string]string{
					"artifact_aliases": "orders.csv#records",
					"json_shape":       "array(len=2,item=object(keys=id,total))",
				},
			}},
		},
	}}
	prompt := dataTaskContinuationPrompt(
		"继续计算",
		"/repo",
		TurnPolicy{Route: RouteData, DataTaskKind: "data_aggregation"},
		nil,
		records,
	)
	for _, want := range []string{
		`"material_coverage_authoritative": true`,
		`"material_coverage_sufficient": true`,
		`"required_materials": [`,
		`"covered_required_materials": [`,
		"deterministic material coverage truth",
		"compact samples and may omit covered artifacts",
		"Do not ask for coverage expansion",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("continuation prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestDataTaskContinuationPromptIncludesCumulativeArtifactAvailability(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:          "old_records",
			Kind:        string(dataquery.DataActionExtractRecords),
			SourcePaths: []string{"old.csv"},
			Headers:     []string{"id", "amount"},
			Fields: map[string]string{
				"artifact_aliases": "old_records,old_records.json",
				"json_shape":       "array(len=2,item=object(keys=id,amount))",
			},
			Sample: []string{`{"id":"1","amount":"42"}`},
		}}},
	}, {
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:          "new_records",
			Kind:        string(dataquery.DataActionExtractRecords),
			SourcePaths: []string{"new.csv"},
			Headers:     []string{"id", "group"},
			Fields: map[string]string{
				"artifact_aliases": "new_records,new_records.json",
				"json_shape":       "array(len=2,item=object(keys=id,group))",
			},
			Sample: []string{`{"id":"1","group":"A"}`},
		}}},
	}}
	prompt := dataTaskContinuationPrompt(
		"继续计算",
		"/repo",
		TurnPolicy{Route: RouteData, DataTaskKind: "data_aggregation"},
		nil,
		records,
	)
	for _, want := range []string{
		`"artifact_availability_count": 2`,
		`"artifact_availability": [`,
		`"artifact_graph"`,
		"old_records",
		"new_records",
		"durable generated-artifact graph",
		"Do not re-extract a covered material",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("continuation prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestDataTaskWorkflowArtifactAvailabilityPrioritizesNewestMaterializedArtifacts(t *testing.T) {
	var oldArtifacts []dataquery.DataArtifact
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("old_%02d", i)
		oldArtifacts = append(oldArtifacts, dataquery.DataArtifact{
			ID:      id,
			Kind:    string(dataquery.DataActionExtractRecords),
			Headers: []string{"id"},
			Fields: map[string]string{
				"artifact_aliases": id + "," + id + ".json",
				"json_shape":       "array(item=object(keys=id))",
			},
		})
	}
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{Artifacts: oldArtifacts},
	}, {
		Result: &dataquery.Result{Artifacts: append(append([]dataquery.DataArtifact{}, oldArtifacts...), dataquery.DataArtifact{
			ID:      "fresh_joined_records",
			Kind:    string(dataquery.DataActionJoinRecords),
			Headers: []string{"query_id", "amount"},
			Fields: map[string]string{
				"artifact_aliases": "fresh_joined_records,fresh_joined_records.json",
				"json_shape":       "array(item=object(keys=query_id,amount))",
			},
		})},
	}}
	availability, total, truncated := dataTaskWorkflowArtifactAvailability(records, 4)
	if total <= 20 {
		t.Fatalf("total=%d, want unique artifacts from both records counted", total)
	}
	if !truncated {
		t.Fatal("truncated=false, want compact availability to report truncation")
	}
	if len(availability) == 0 || availability[0].ID != "fresh_joined_records" {
		t.Fatalf("availability=%+v, want newest materialized artifact first", availability)
	}
}

func TestDataTaskWorkflowStateIncludesArtifactGraph(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:                "eligible_records",
			Kind:              string(dataquery.DataActionFilterRecords),
			Headers:           []string{"id", "amount", "status"},
			SourceRecordPaths: []string{"source_records.json"},
			ReferencePaths:    []string{"reference.json"},
			RowCount:          3,
			Fields: map[string]string{
				"artifact_aliases":   "eligible_records,eligible_records.json",
				"json_shape":         "array(len=3,item=object(keys=id,amount,status))",
				"input_rows":         "5",
				"output_rows":        "3",
				"filter_diagnostics": `{"total":5,"combined_match":3}`,
			},
		}}},
	}}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if state.ArtifactGraph.NodeCount != 1 || len(state.ArtifactGraph.Nodes) != 1 {
		t.Fatalf("artifact graph=%+v, want one node", state.ArtifactGraph)
	}
	node := state.ArtifactGraph.Nodes[0]
	if node.PrimaryAlias == "" || !node.ExecutableRecordInput || node.RowCount != 3 {
		t.Fatalf("node=%+v, want executable record node with row count", node)
	}
	if !slices.Contains(node.Lineage.SourceRecordPaths, "source_records.json") || !slices.Contains(node.Lineage.ReferencePaths, "reference.json") {
		t.Fatalf("lineage=%+v, want role-specific lineage", node.Lineage)
	}
	if node.Diagnostics["filter_diagnostics"] == "" {
		t.Fatalf("diagnostics=%+v, want filter diagnostics", node.Diagnostics)
	}
	prompt := dataTaskContinuationPrompt(
		"继续计算",
		"/repo",
		TurnPolicy{Route: RouteData, DataTaskKind: "data_aggregation"},
		nil,
		records,
	)
	for _, want := range []string{
		`"artifact_graph"`,
		`"executable_record_aliases"`,
		`"filter_diagnostics"`,
		"durable generated-artifact graph",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("continuation prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestDataTaskWorkflowStateIncludesProgressSignatures(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{Kind: dataquery.DataActionExtractRecords}}},
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:       "records",
			Kind:     string(dataquery.DataActionExtractRecords),
			Headers:  []string{"id", "amount"},
			RowCount: 2,
			Fields:   map[string]string{"artifact_aliases": "records", "json_shape": "array(len=2)"},
		}}},
	}, {
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{Kind: dataquery.DataActionDeriveFields}}},
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:       "records_ready",
			Kind:     string(dataquery.DataActionDeriveFields),
			Headers:  []string{"id", "amount", "amount_num"},
			RowCount: 2,
			Fields:   map[string]string{"artifact_aliases": "records_ready", "json_shape": "array(len=2)"},
		}}},
	}}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if state.ProgressSignatures.Latest.Round != 2 || state.ProgressSignatures.LatestDelta.ArtifactRowsDelta != 0 {
		t.Fatalf("progress=%+v, want latest round 2 with row delta 0", state.ProgressSignatures)
	}
	if !slices.Contains(state.ProgressSignatures.LatestDelta.AddedFields, "amount_num") {
		t.Fatalf("progress delta=%+v, want added derived field", state.ProgressSignatures.LatestDelta)
	}
	prompt := dataTaskContinuationPrompt(
		"继续计算",
		"/repo",
		TurnPolicy{Route: RouteData, DataTaskKind: "data_aggregation"},
		nil,
		records,
	)
	for _, want := range []string{`"progress_signatures"`, `"latest_delta"`, `"added_fields"`, "structural progress"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("continuation prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestDataTaskPromptsExplainCustomTransformDisabledStillAllowsTypedActions(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{{Path: "records.csv", Required: true}},
			},
			Actions: []dataquery.DataAction{{
				ID:         "bad_script",
				Kind:       dataquery.DataActionCustomTransform,
				InputPaths: []string{"records.csv"},
				Script:     "emit({'answer':'x'})",
			}},
		},
		Err: "execute data task: data task script failed: exit status 1",
	}, {
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials:          []dataquery.CoverageMaterial{{Path: "records.csv", Required: true}},
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
			Actions: []dataquery.DataAction{{
				ID:         "records",
				Kind:       dataquery.DataActionExtractRecords,
				InputPaths: []string{"records.csv"},
			}},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"records.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:      "records",
				Kind:    string(dataquery.DataActionExtractRecords),
				Headers: []string{"id", "amount", "group"},
				Fields: map[string]string{
					"artifact_aliases": "records",
					"json_shape":       "array(len=2,item=object(keys=id,amount,group))",
				},
			}},
		},
	}}
	continuation := dataTaskContinuationPrompt(
		"继续计算",
		"/repo",
		TurnPolicy{Route: RouteData, DataTaskKind: "data_aggregation"},
		nil,
		records,
	)
	for _, want := range []string{
		`"custom_transform_disabled": true`,
		`"custom_transform_disabled_note": "free-form custom_transform scripts are disabled`,
		"disables only free-form scripts",
		"not a compute-capability failure",
		"typed actions",
	} {
		if !strings.Contains(continuation, want) {
			t.Fatalf("continuation prompt missing %q:\n%s", want, continuation)
		}
	}
	evaluation := dataTaskEvaluationPrompt("继续计算", records, "zh-CN")
	for _, want := range []string{
		"disables only free-form scripts",
		"does not mean the task is blocked",
		"compute_contributions",
		"reconcile_artifacts",
	} {
		if !strings.Contains(evaluation, want) {
			t.Fatalf("evaluation prompt missing %q:\n%s", want, evaluation)
		}
	}
}

func TestDataTaskContinuationPromptUsesCompactHistory(t *testing.T) {
	records := make([]dataTaskWorkflowRecord, 0, 7)
	largeParam := strings.Repeat("field_a|field_b|field_c|", 400)
	for i := 0; i < 7; i++ {
		records = append(records, dataTaskWorkflowRecord{
			Plan: dataquery.TaskPlan{
				Status: "ready",
				Actions: []dataquery.DataAction{{
					ID:             fmt.Sprintf("A%d", i),
					Kind:           dataquery.DataActionDeriveFields,
					InputPaths:     []string{"records"},
					OutputArtifact: fmt.Sprintf("records_%d", i),
					Params: map[string]string{
						"field_specs_json": largeParam,
					},
					Script: strings.Repeat("print('x')\n", 200),
				}},
			},
			Result: &dataquery.Result{
				Answer: strings.Repeat("answer,", 500),
				Artifacts: []dataquery.DataArtifact{{
					ID:      fmt.Sprintf("records_%d", i),
					Kind:    "derive_fields",
					Headers: []string{"field_a", "field_b", "field_c"},
					Fields: map[string]string{
						"artifact_aliases": fmt.Sprintf("records_%d", i),
						"json_shape":       "array(len=100,item=object(keys=field_a,field_b,field_c))",
					},
					Sample: []string{`{"field_a":"a","field_b":"b","field_c":"c"}`},
				}},
			},
		})
	}
	prompt := dataTaskContinuationPrompt(
		"继续计算",
		"/repo",
		TurnPolicy{Route: RouteData, DataTaskKind: "data_aggregation"},
		nil,
		records,
	)
	if !strings.Contains(prompt, "omitted 4 older data rounds") {
		t.Fatalf("continuation prompt did not omit old rounds:\n%s", prompt)
	}
	if !strings.Contains(prompt, "field_specs_json") || !strings.Contains(prompt, "truncated") {
		t.Fatalf("continuation prompt lost compact param preview/truncation marker:\n%s", prompt)
	}
	if len(prompt) > 60000 {
		t.Fatalf("continuation prompt too large after compaction: %d bytes", len(prompt))
	}
}

func TestDataTaskContinuationPromptUsesCompactCandidateFiles(t *testing.T) {
	prompt := dataTaskContinuationPrompt(
		"继续计算",
		"/repo",
		TurnPolicy{Route: RouteData, DataTaskKind: "data_aggregation"},
		[]dataquery.CandidateFile{{
			Path:              "records.csv",
			Kind:              "csv",
			Size:              123,
			Lines:             10,
			Headers:           []string{"id", "amount", "status"},
			SampleRows:        [][]string{{"id", "amount"}, {"1", "42"}},
			Sample:            []string{"very long sample line"},
			TextEvidencePaths: []string{"evidence/a.txt", "evidence/b.txt"},
		}},
		nil,
	)
	for _, want := range []string{"records.csv", "headers_json", "text_evidence_count=2", "compact continuation view"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("continuation prompt missing compact candidate marker %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{"sample_rows_json", "sample_lines_json", "very long sample line", "evidence/a.txt"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("continuation prompt leaked full candidate material %q:\n%s", forbidden, prompt)
		}
	}
}

func TestDataTaskWorkflowStagingGuardRejectsSingleCustomTransformAcrossValidationStages(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{
					{Path: "orders.csv", Required: true},
					{Path: "rules.md", Required: true},
				},
				RuleCoverageRequired:       true,
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "rules.md"},
			RuleCoverage: []dataquery.RuleCoverageRecord{{
				RuleID:   "R1",
				RuleText: "Only include valid rows",
				Status:   "applied",
				Notes:    "derived from rules.md",
			}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true},
				{Path: "rules.md", Required: true},
			},
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "compute_all",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"orders.csv#records"},
			Script:     `emit({"artifacts":[{"id":"partial"}]})`,
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	for _, want := range []string{
		"custom_transform_disabled=true",
		"next_stage=prepare_contribution_inputs",
		"allowed_next_actions",
		"custom_transform",
	} {
		if !strings.Contains(errText, want) {
			t.Fatalf("errText=%q, want %q", errText, want)
		}
	}
}

func TestDataTaskWorkflowAllowedActionGuardUsesPriorStateNotCandidatePlan(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{
					{Path: "orders.csv", Required: true},
					{Path: "queries.csv", Required: true},
					{Path: "rules.md", Required: true},
				},
				RuleCoverageRequired:       true,
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "queries.csv", "rules.md"},
			RuleCoverage:  []dataquery.RuleCoverageRecord{{RuleID: "R1", RuleText: "include matching rows", Status: "applied"}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status:        "ready",
		ContinueAfter: true,
		CoverageContract: dataquery.CoverageContract{
			// The candidate plan is intentionally narrower than the accepted
			// workflow. It must not redefine the next-stage contract used to
			// validate its own actions.
			RequiredMaterials: []dataquery.CoverageMaterial{{Path: "rules.md", Required: true}},
		},
		Actions: []dataquery.DataAction{{
			ID:         "reparse_rules",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"rules.md"},
			Script:     `emit({"rules":["x"]})`,
		}},
	}
	errText := dataTaskWorkflowAllowedNextActionGuardError(records, plan)
	for _, want := range []string{
		"workflow next_stage=prepare_contribution_inputs",
		"allows only next actions",
		"custom_transform",
	} {
		if !strings.Contains(errText, want) {
			t.Fatalf("errText=%q, want %q", errText, want)
		}
	}
}

func TestDataTaskWorkflowStagingGuardRejectsTerminalRawMaterialCustomTransform(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{
					{Path: "orders.csv", Required: true},
					{Path: "rules.md", Required: true},
					{Path: "queries.csv", Required: true},
				},
				RuleCoverageRequired:       true,
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "rules.md", "queries.csv"},
			RuleCoverage:  []dataquery.RuleCoverageRecord{{RuleID: "R1", RuleText: "include matching rows", Status: "applied"}},
			Contributions: []dataquery.ContributionRecord{{
				ItemID:    "orders.csv#1",
				GroupKey:  "Q1",
				Metric:    "total",
				Value:     "42",
				Operation: "add",
			}},
			Reconcile: &dataquery.ReconcileReport{
				Status:         "pass",
				ExpectedAnswer: "42",
				ActualAnswer:   "42",
				Groups: []dataquery.ReconcileGroup{{
					GroupKey:   "Q1",
					Metric:     "total",
					Expected:   "42",
					Actual:     "42",
					Difference: "0",
				}},
			},
			Artifacts: []dataquery.DataArtifact{{
				ID:   "reconcile_totals",
				Kind: string(dataquery.DataActionReconcile),
				Fields: map[string]string{
					"artifact_aliases": "reconcile_totals,reconcile_totals.json",
					"json_shape":       "object(keys=groups,status)",
				},
			}},
		},
	}}
	lines := make([]string, 0, dataTaskComplexCustomScriptLineLimit+2)
	for i := 0; i < dataTaskComplexCustomScriptLineLimit; i++ {
		lines = append(lines, "x = 1")
	}
	lines = append(lines, `emit_result("42", output_contract={"format":"plain_single_line","explanation_allowed":False})`)
	plan := dataquery.TaskPlan{
		Status: "ready",
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "orders.csv", Required: true},
				{Path: "rules.md", Required: true},
				{Path: "queries.csv", Required: true},
			},
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "final_projection",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"orders.csv", "rules.md", "queries.csv"},
			Script:     strings.Join(lines, "\n"),
		}},
	}
	errText := dataTaskWorkflowStagingGuardError(records, plan)
	for _, want := range []string{"terminal custom_transform", "original material", "generated artifact aliases", "assemble_answer"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("errText=%q, want %q", errText, want)
		}
	}
}

func TestDataTaskWorkflowStagingGuardAllowsTerminalGeneratedArtifactProjection(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{
				RuleCoverageRequired:       true,
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "rules.md"},
			RuleCoverage:  []dataquery.RuleCoverageRecord{{RuleID: "R1", RuleText: "include matching rows", Status: "applied"}},
			Contributions: []dataquery.ContributionRecord{{
				ItemID:    "orders.csv#1",
				GroupKey:  "Q1",
				Metric:    "total",
				Value:     "42",
				Operation: "add",
			}},
			Reconcile: &dataquery.ReconcileReport{
				Status:         "pass",
				ExpectedAnswer: "42",
				ActualAnswer:   "42",
				Groups: []dataquery.ReconcileGroup{{
					GroupKey:   "Q1",
					Metric:     "total",
					Expected:   "42",
					Actual:     "42",
					Difference: "0",
				}},
			},
			Artifacts: []dataquery.DataArtifact{{
				ID:   "reconcile_totals",
				Kind: string(dataquery.DataActionReconcile),
				Fields: map[string]string{
					"artifact_aliases": "reconcile_totals,reconcile_totals.json",
					"json_shape":       "object(keys=groups,status)",
				},
			}},
		},
	}}
	plan := dataquery.TaskPlan{
		Status: "ready",
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: dataquery.CoverageContract{
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:         "format_answer",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"reconcile_totals"},
			Script:     `emit_result("42", output_contract={"format":"plain_single_line","explanation_allowed":False})`,
		}},
	}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("errText=%q, want generated artifact projection to pass", errText)
	}
}

func TestValidateDataTaskWorkflowResultRejectsDroppedEarlierRequirements(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			InputPaths: []string{"orders.csv", "rules.md"},
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials: []dataquery.CoverageMaterial{
					{Path: "orders.csv", Purpose: "source rows", Required: true},
					{Path: "rules.md", Purpose: "task rules", Required: true},
				},
			},
		},
	}}
	current := dataquery.TaskPlan{
		InputPaths: []string{"orders.csv"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{{Path: "orders.csv", Purpose: "source rows", Required: true}},
		},
	}
	err := validateDataTaskWorkflowResult(records, current, dataquery.Result{
		Answer:         "42",
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
		ConsumedPaths:  []string{"orders.csv"},
	})
	if err == nil {
		t.Fatal("validateDataTaskWorkflowResult returned nil, want missing earlier required material")
	}
	if !strings.Contains(err.Error(), "rules.md") {
		t.Fatalf("err=%v", err)
	}
	var validationErr dataquery.DataValidationError
	if !errors.As(err, &validationErr) || len(validationErr.Violations) == 0 {
		t.Fatalf("err=%T %[1]v, want typed data validation violation", err)
	}
	if validationErr.Violations[0].Code != "workflow_material_coverage_incomplete" {
		t.Fatalf("violation=%+v", validationErr.Violations[0])
	}
}

func TestDataTaskActionRunnerSeedAccumulatesArtifactsAcrossRecords(t *testing.T) {
	records := []dataTaskWorkflowRecord{
		{
			Result: &dataquery.Result{
				Artifacts: []dataquery.DataArtifact{{
					ID:          "orders.csv#records",
					Kind:        "extract_records/csv",
					SourcePaths: []string{"orders.csv"},
					Fields: map[string]string{
						"artifact_path":    "/tmp/orders-records.json",
						"artifact_aliases": "orders.csv#records",
					},
				}},
				ConsumedPaths: []string{"orders.csv"},
				RuleCoverage: []dataquery.RuleCoverageRecord{{
					RuleID:   dataquery.LooseText("r1"),
					RuleText: dataquery.LooseText("rule one"),
				}},
			},
		},
		{
			Result: &dataquery.Result{
				Artifacts: []dataquery.DataArtifact{{
					ID:          "queries.csv#records",
					Kind:        "extract_records/csv",
					SourcePaths: []string{"queries.csv"},
					Fields: map[string]string{
						"artifact_path":    "/tmp/queries-records.json",
						"artifact_aliases": "queries.csv#records",
					},
				}},
				ConsumedPaths: []string{"queries.csv"},
			},
		},
	}
	seed := dataTaskActionRunnerSeed(records)
	if len(seed.Artifacts) != 2 {
		t.Fatalf("Artifacts=%+v, want both prior batch artifacts", seed.Artifacts)
	}
	if got := strings.Join(seed.ConsumedPaths, ","); got != "orders.csv,queries.csv" {
		t.Fatalf("ConsumedPaths=%q, want accumulated paths", got)
	}
	if len(seed.RuleCoverage) != 1 {
		t.Fatalf("RuleCoverage=%+v, want prior rule coverage", seed.RuleCoverage)
	}
}

func TestPrepareDataTaskWorkflowPlanRaisesExtractLimitForExactWorkflow(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:             "extract_orders",
			Kind:           dataquery.DataActionExtractRecords,
			InputPaths:     []string{"orders.csv"},
			OutputArtifact: "orders",
		}},
	}
	got := prepareDataTaskWorkflowPlanForExecution("", nil, nil, plan)
	if len(got.Actions) != 1 {
		t.Fatalf("Actions=%+v, want one action", got.Actions)
	}
	if got.Actions[0].Params["limit"] != fmt.Sprintf("%d", dataTaskExactExtractRecordLimit) {
		t.Fatalf("limit=%q, want exact extraction limit", got.Actions[0].Params["limit"])
	}
	if got.Actions[0].Params["record_completeness_required"] != "true" {
		t.Fatalf("params=%+v, want record completeness marker", got.Actions[0].Params)
	}
}

func TestDataTaskWorkflowStagingGuardRejectsNonNumericConstantUsedAsNumeric(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{
			{
				ID:             "derive_flag",
				Kind:           dataquery.DataActionDeriveFields,
				InputPaths:     []string{"records.json"},
				OutputArtifact: "derived_records.json",
				Params: map[string]string{
					"field_specs": `[{"target_field":"eligible_value","operation":"constant","value":true}]`,
				},
			},
			{
				ID:         "filter_numeric",
				Kind:       dataquery.DataActionFilterRecords,
				InputPaths: []string{"derived_records.json"},
				Params: map[string]string{
					"filters": `[{"field":"eligible_value","op":"gt","value":0}]`,
				},
			},
		},
	}
	errText := dataTaskWorkflowStagingGuardError(nil, plan)
	for _, want := range []string{"non-numeric constant", "eligible_value", "parse_number"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("guard=%q, want substring %q", errText, want)
		}
	}
}

func TestDataTaskFieldContractViolationsParseNumericContract(t *testing.T) {
	rec := dataTaskWorkflowRecord{
		Err: `execute data task: data action failed action_id="filter" action_kind="filter_records": numeric field contract failed: filter_records input records.json field "eligible_value" op "gt" requires numeric values but sampled non-empty values are non-numeric`,
	}
	access := []dataTaskArtifactAccessPrompt{{
		ID:      "records.json",
		Aliases: []string{"records.json"},
		Fields:  []string{"eligible_value", "amount_text", "amount_number"},
		FieldSamples: map[string][]string{
			"eligible_value": []string{"true"},
			"amount_text":    []string{"12,300"},
			"amount_number":  []string{"12300"},
		},
	}}
	got := dataTaskFieldContractViolationsFromRecord(3, rec, access, map[string]bool{
		string(dataquery.DataActionDeriveFields): true,
	})
	if len(got) != 1 {
		t.Fatalf("violations=%+v, want one numeric contract violation", got)
	}
	if got[0].ActionKind != "filter_records" || got[0].InputAlias != "records.json" || !slices.Equal(got[0].MissingFields, []string{"eligible_value"}) {
		t.Fatalf("violation=%+v", got[0])
	}
	if len(got[0].CandidateArtifacts) == 0 || !strings.Contains(got[0].CandidateArtifacts[0], "amount_number") {
		t.Fatalf("candidates=%+v, want numeric-like field candidate", got[0].CandidateArtifacts)
	}
}

func TestPrepareDataTaskWorkflowPlanPreservesSampleOnlyExtract(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		CoverageContract: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:             "preview_orders",
			Kind:           dataquery.DataActionExtractRecords,
			InputPaths:     []string{"orders.csv"},
			OutputArtifact: "orders_preview",
			Params: map[string]string{
				"limit":       "5",
				"sample_only": "true",
			},
		}},
	}
	got := prepareDataTaskWorkflowPlanForExecution("", nil, nil, plan)
	if got.Actions[0].Params["limit"] != "5" {
		t.Fatalf("limit=%q, want explicit sample limit preserved", got.Actions[0].Params["limit"])
	}
	if got.Actions[0].Params["record_completeness_required"] != "" {
		t.Fatalf("params=%+v, want no exact completeness marker for sample_only", got.Actions[0].Params)
	}
}

func TestSampleDataTaskArtifactsCompactsChildSamples(t *testing.T) {
	var parentSample []string
	for i := 0; i < 10; i++ {
		parentSample = append(parentSample, fmt.Sprintf(`{"parent":%d}`, i))
	}
	var childSample []string
	for i := 0; i < 20; i++ {
		childSample = append(childSample, fmt.Sprintf(`{"id":%d,"value":"row-%d"}`, i, i))
	}
	var grandchildSample []string
	for i := 0; i < 8; i++ {
		grandchildSample = append(grandchildSample, fmt.Sprintf(`{"grandchild":%d}`, i))
	}
	artifacts := []dataquery.DataArtifact{{
		ID:      "records",
		Kind:    string(dataquery.DataActionExtractRecords),
		Summary: strings.Repeat("summary ", 100),
		Sample:  parentSample,
		Fields: map[string]string{
			"json_shape": strings.Repeat("shape ", 100),
		},
		Children: []dataquery.DataArtifact{{
			ID:      "records.csv#records",
			Kind:    "extract_records/csv",
			Headers: []string{"id", "value"},
			Sample:  childSample,
			Fields: map[string]string{
				"record_completeness": "complete",
				"total_rows":          "20",
				"huge_field_catalog":  strings.Repeat("field ", 100),
			},
			Children: []dataquery.DataArtifact{{
				ID:     "records.csv#nested",
				Kind:   "diagnostic",
				Sample: grandchildSample,
				Children: []dataquery.DataArtifact{{
					ID:     "records.csv#too-deep",
					Kind:   "diagnostic",
					Sample: []string{"must not be visible"},
				}},
			}},
		}},
	}}
	got := sampleDataTaskArtifacts(artifacts, 4)
	if len(got) != 1 || len(got[0].Children) != 1 {
		t.Fatalf("got=%+v, want parent and one child", got)
	}
	if len(got[0].Sample) != 5 {
		t.Fatalf("parent sample len=%d sample=%v, want compacted sample with truncation marker", len(got[0].Sample), got[0].Sample)
	}
	if len(got[0].Children[0].Sample) != 3 {
		t.Fatalf("child sample len=%d sample=%v, want compacted sample with truncation marker", len(got[0].Children[0].Sample), got[0].Children[0].Sample)
	}
	if len(got[0].Children[0].Children) != 1 || len(got[0].Children[0].Children[0].Sample) != 3 {
		t.Fatalf("grandchild=%+v, want one compacted nested diagnostic sample", got[0].Children[0].Children)
	}
	if len(got[0].Children[0].Children[0].Children) != 0 {
		t.Fatalf("too deep children should be removed from prompt projection: %+v", got[0].Children[0].Children[0].Children)
	}
	if len(got[0].Fields["json_shape"]) > 260 {
		t.Fatalf("json_shape was not clamped: %q", got[0].Fields["json_shape"])
	}
	if len(got[0].Children[0].Fields["huge_field_catalog"]) > 260 {
		t.Fatalf("child field catalog was not clamped: %q", got[0].Children[0].Fields["huge_field_catalog"])
	}
}

func TestCompactDataTaskResultPromptViewProjectsLedgers(t *testing.T) {
	result := dataquery.Result{
		Rows: []dataquery.RowDecision{
			{Decision: "include"},
			{Decision: "exclude"},
			{Decision: "include"},
		},
		EntityResolutions: []dataquery.EntityResolutionRecord{
			{Status: dataquery.LooseText("resolved"), SourceValue: dataquery.LooseText("a")},
			{Status: dataquery.LooseText("unresolved"), SourceValue: dataquery.LooseText("b")},
			{Status: dataquery.LooseText("resolved"), SourceValue: dataquery.LooseText("c")},
		},
		Contributions: []dataquery.ContributionRecord{
			{GroupKey: dataquery.LooseText("g1"), Metric: dataquery.LooseText("amount"), Role: dataquery.LooseText("target")},
			{GroupKey: dataquery.LooseText("g2"), Metric: dataquery.LooseText("amount"), Role: dataquery.LooseText("target")},
		},
	}
	view := compactDataTaskResultPromptView(result, 100, 100, 1, 1, 1)
	if view == nil {
		t.Fatalf("view=nil")
	}
	if len(view.EntityResolutionSamples) != 2 {
		t.Fatalf("EntityResolutionSamples=%+v, want compact bounded samples", view.EntityResolutionSamples)
	}
	projections := map[string]dataTaskLedgerProjection{}
	for _, projection := range view.LedgerProjection {
		projections[projection.Kind] = projection
	}
	if projections["decision_records"].DecisionCount["include"] != 2 || projections["decision_records"].DecisionCount["exclude"] != 1 {
		t.Fatalf("decision projection=%+v", projections["decision_records"])
	}
	if projections["entity_resolutions"].StatusCounts["resolved"] != 2 || projections["entity_resolutions"].StatusCounts["unresolved"] != 1 {
		t.Fatalf("entity projection=%+v", projections["entity_resolutions"])
	}
	if projections["contributions"].Count != 2 || !slices.Contains(projections["contributions"].GroupKeys, "g1/amount") {
		t.Fatalf("contribution projection=%+v", projections["contributions"])
	}
}

func TestSampleDataTaskArtifactAccessUsesSmallFieldSamples(t *testing.T) {
	var samples []string
	for i := 0; i < 4; i++ {
		samples = append(samples, fmt.Sprintf(`{"a":"a%d","b":"b%d","c":"c%d","d":"d%d","e":"e%d","f":"f%d","g":"g%d"}`, i, i, i, i, i, i, i))
	}
	access := sampleDataTaskArtifactAccess([]dataquery.DataArtifact{{
		ID:      "records",
		Kind:    "record_artifact",
		Headers: []string{"a", "b", "c", "d", "e", "f", "g"},
		Fields: map[string]string{
			"artifact_aliases": "records",
			"json_shape":       "array(len=4,item=object(keys=a,b,c,d,e,f,g))",
		},
		Sample: samples,
	}}, 10)
	if len(access) != 1 {
		t.Fatalf("access=%+v, want one artifact", access)
	}
	if len(access[0].FieldSamples) > 6 {
		t.Fatalf("FieldSamples keys=%v, want at most 6 fields", access[0].FieldSamples)
	}
	for field, values := range access[0].FieldSamples {
		if len(values) > 2 {
			t.Fatalf("field %s values=%v, want at most 2 values", field, values)
		}
	}
}

func TestNormalizeDataTaskPlanShapeAddsTopLevelInputsReferencedByCustomScript(t *testing.T) {
	plan := dataquery.TaskPlan{
		InputPaths: []string{"orders.csv", "rules.md", "unused.csv"},
		Actions: []dataquery.DataAction{{
			ID:         "bounded_transform",
			Kind:       dataquery.DataActionCustomTransform,
			InputPaths: []string{"orders.csv"},
			Script: `
rules = read_text("rules.md")
rows = csv_rows("orders.csv")
emit_result(str(len(rows)), output_contract={"format":"plain_single_line","explanation_allowed":False})
`,
		}},
	}
	got, reasons := normalizeDataTaskPlanShape(plan)
	if len(reasons) == 0 || !strings.Contains(strings.Join(reasons, ";"), "top-level declared inputs") {
		t.Fatalf("reasons=%v, want custom action input normalization", reasons)
	}
	inputs := strings.Join(got.Actions[0].InputPaths, ",")
	if inputs != "orders.csv,rules.md" {
		t.Fatalf("InputPaths=%q, want only referenced declared inputs", inputs)
	}
}

func TestNormalizeDataTaskPlanShapeCopiesTopLevelInputsToMaterializationAction(t *testing.T) {
	plan := dataquery.TaskPlan{
		InputPaths: []string{"orders.csv", "vendors.csv"},
		Actions: []dataquery.DataAction{{
			ID:             "materialize_records",
			Kind:           dataquery.DataActionExtractRecords,
			OutputArtifact: "source_records",
		}},
	}
	got, reasons := normalizeDataTaskPlanShape(plan)
	if !strings.Contains(strings.Join(reasons, ";"), "materialization actions") {
		t.Fatalf("reasons=%v, want materialization input inheritance", reasons)
	}
	if strings.Join(got.Actions[0].InputPaths, ",") != "orders.csv,vendors.csv" {
		t.Fatalf("InputPaths=%v, want copied top-level inputs", got.Actions[0].InputPaths)
	}
	if errText := dataTaskActionStagingGuardError(got); errText != "" {
		t.Fatalf("guard error after normalization: %s", errText)
	}
}

func TestNormalizeDataTaskPlanShapeDoesNotCopyTopLevelInputsToRoleActions(t *testing.T) {
	plan := dataquery.TaskPlan{
		InputPaths: []string{"left.csv", "right.csv"},
		Actions: []dataquery.DataAction{{
			ID:             "join_records",
			Kind:           dataquery.DataActionJoinRecords,
			OutputArtifact: "joined_records",
		}},
	}
	got, reasons := normalizeDataTaskPlanShape(plan)
	if strings.Contains(strings.Join(reasons, ";"), "materialization actions") {
		t.Fatalf("reasons=%v, did not want role-action input inheritance", reasons)
	}
	if len(got.Actions[0].InputPaths) != 0 {
		t.Fatalf("InputPaths=%v, want role action to keep explicit empty inputs", got.Actions[0].InputPaths)
	}
	errText := dataTaskActionStagingGuardError(got)
	if !strings.Contains(errText, "requires input_paths") {
		t.Fatalf("errText=%q, want explicit input-path guard", errText)
	}
}

func TestPrepareDataTaskWorkflowPlanBootstrapsCandidateInventoryForComplexTask(t *testing.T) {
	candidates := []dataquery.CandidateFile{
		{Path: "orders.csv", Kind: "csv"},
		{Path: "queries.csv", Kind: "csv"},
		{Path: "rules.md", Kind: "text"},
		{Path: "attachments/invoice.txt", Kind: "text"},
	}
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv", "queries.csv"},
		CoverageContract: dataquery.CoverageContract{
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []dataquery.DataAction{{
			ID:             "extract",
			Kind:           dataquery.DataActionExtractRecords,
			InputPaths:     []string{"orders.csv", "queries.csv"},
			OutputArtifact: "records.json",
		}},
		ContinueAfter: true,
	}
	got := prepareDataTaskWorkflowPlanForExecution("汇总 orders.csv", candidates, nil, plan)
	if len(got.Actions) != 1 || got.Actions[0].Kind != dataquery.DataActionMaterialInventory {
		t.Fatalf("Actions=%+v, want material_inventory bootstrap", got.Actions)
	}
	if strings.Join(got.Actions[0].InputPaths, ",") != "orders.csv,queries.csv,rules.md,attachments/invoice.txt" {
		t.Fatalf("InputPaths=%v, want all candidate paths", got.Actions[0].InputPaths)
	}
	if len(got.CoverageContract.OptionalMaterials) < len(candidates) {
		t.Fatalf("OptionalMaterials=%+v, want candidate materials represented", got.CoverageContract.OptionalMaterials)
	}
}

func TestPrepareDataTaskWorkflowPlanDoesNotBootstrapCandidateInventoryForSimpleTask(t *testing.T) {
	candidates := []dataquery.CandidateFile{
		{Path: "orders.csv", Kind: "csv"},
		{Path: "notes.txt", Kind: "text"},
	}
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv"},
		Actions: []dataquery.DataAction{{
			ID:             "extract",
			Kind:           dataquery.DataActionExtractRecords,
			OutputArtifact: "records.json",
		}},
		ContinueAfter: true,
	}
	got := prepareDataTaskWorkflowPlanForExecution("查看 orders.csv", candidates, nil, plan)
	if len(got.Actions) != 1 || got.Actions[0].Kind != dataquery.DataActionExtractRecords {
		t.Fatalf("Actions=%+v, want original extract_records action", got.Actions)
	}
}

func dataTaskPlanResp(raw string) llm.Response {
	return llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name:   dataTaskPlanTool.Name,
			Params: []byte(raw),
		}},
		StopReason: "tool_use",
	}
}

func dataTaskPatchResp(raw string) llm.Response {
	return llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name:   dataTaskResultPatchTool.Name,
			Params: []byte(raw),
		}},
		StopReason: "tool_use",
	}
}
