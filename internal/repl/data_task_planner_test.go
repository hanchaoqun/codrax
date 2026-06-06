package repl

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
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

func TestNormalizeDataTaskPlanShapeMovesTopLevelScriptIntoCustomAction(t *testing.T) {
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
	if len(notes) != 1 {
		t.Fatalf("notes=%v, want one structural normalization note", notes)
	}
	if strings.TrimSpace(got.Script) != "" {
		t.Fatalf("top-level script not cleared: %q", got.Script)
	}
	if strings.TrimSpace(got.Actions[1].Script) == "" {
		t.Fatalf("custom_transform script not populated: %+v", got.Actions[1])
	}
	if strings.Join(got.Actions[1].InputPaths, ",") != "orders.csv" {
		t.Fatalf("custom_transform input paths=%v", got.Actions[1].InputPaths)
	}
}

func TestNormalizeDataTaskPlanShapeMergesTopLevelInputsIntoCustomAction(t *testing.T) {
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
	if len(notes) != 1 {
		t.Fatalf("notes=%v, want one structural normalization note", notes)
	}
	if strings.Join(got.Actions[0].InputPaths, ",") != "rules.md,orders.csv,queries.csv" {
		t.Fatalf("custom_transform input paths=%v", got.Actions[0].InputPaths)
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

func TestNormalizeDataTaskPlanShapeAppendsBoundedTopLevelScriptAfterTypedActions(t *testing.T) {
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
	if !strings.Contains(strings.Join(notes, "\n"), "appended bounded top-level script") {
		t.Fatalf("notes=%v", notes)
	}
	if strings.TrimSpace(got.Script) != "" {
		t.Fatalf("top-level script not cleared: %q", got.Script)
	}
	if len(got.Actions) != 3 || got.Actions[2].Kind != dataquery.DataActionCustomTransform {
		t.Fatalf("Actions=%+v, want appended custom_transform", got.Actions)
	}
	if strings.TrimSpace(got.Actions[2].Script) == "" {
		t.Fatalf("appended custom_transform missing script: %+v", got.Actions[2])
	}
	if strings.Join(got.Actions[2].InputPaths, ",") != "orders.csv,rules.md" {
		t.Fatalf("custom_transform input paths=%v", got.Actions[2].InputPaths)
	}
}

func TestNormalizeDataTaskPlanShapeAppendsTopLevelScriptWhenCustomActionsAlreadyHaveScripts(t *testing.T) {
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
	if !strings.Contains(strings.Join(notes, "\n"), "appended bounded top-level script") {
		t.Fatalf("notes=%v", notes)
	}
	if got.Script != "" {
		t.Fatalf("top-level script not cleared: %q", got.Script)
	}
	if len(got.Actions) != 4 || got.Actions[3].Kind != dataquery.DataActionCustomTransform {
		t.Fatalf("Actions=%+v, want appended final custom_transform", got.Actions)
	}
}

func TestNormalizeDataTaskPlanShapeAppendsAfterOneTypedAndExistingCustomScript(t *testing.T) {
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
	if !strings.Contains(strings.Join(notes, "\n"), "appended bounded top-level script") {
		t.Fatalf("notes=%v", notes)
	}
	if got.Script != "" {
		t.Fatalf("top-level script not cleared: %q", got.Script)
	}
	if len(got.Actions) != 3 || got.Actions[2].Kind != dataquery.DataActionCustomTransform {
		t.Fatalf("Actions=%+v, want appended final custom_transform", got.Actions)
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
	for _, want := range []string{"actions", "material_inventory", "inspect_material", "extract_records", "derive_rules", "derive_fields", "normalize_entities", "enrich_records", "join_records", "compute_contributions", "reconcile_artifacts", "custom_transform", "adaptive action workflow", "An action is atomic"} {
		if !strings.Contains(system, want) {
			t.Fatalf("data planner system prompt missing %q:\n%s", want, system)
		}
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
	if state.NextStage != "compute_contributions" {
		t.Fatalf("NextStage=%q, want compute_contributions; state=%+v", state.NextStage, state)
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
	if state.HasAnswer || state.NextStage != "emit_output_contract_answer" {
		t.Fatalf("state=%+v, want action artifact summary to remain intermediate", state)
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
		Actions: []dataquery.DataAction{
			{ID: "inspect", Kind: dataquery.DataActionInspectMaterial, InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv"}},
			{ID: "rules", Kind: dataquery.DataActionDeriveRules, InputPaths: []string{"rules.md"}},
			{
				ID:         "final_transform",
				Kind:       dataquery.DataActionCustomTransform,
				InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv"},
				Script:     strings.Join(lines, "\n"),
			},
		},
	}
	if errText := dataTaskPlanStagingGuardError(plan); errText != "" {
		t.Fatalf("final custom transform after typed context should pass, got %q", errText)
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

func TestDataTaskWorkflowStagingGuardRejectsCrossStageCustomTransform(t *testing.T) {
	required := []dataquery.CoverageMaterial{
		{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
		{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
	}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{RequiredMaterials: required, RuleCoverageRequired: true},
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
	if !strings.Contains(errText, "cross multiple unfinished data DAG stages") {
		t.Fatalf("errText=%q, want cross-stage custom_transform guard", errText)
	}

	plan.ContinueAfter = true
	plan.Actions = []dataquery.DataAction{{
		ID:             "classify",
		Kind:           dataquery.DataActionCustomTransform,
		InputPaths:     []string{"records.csv", "rules.md"},
		OutputArtifact: "classified_records.json",
		Script:         `emit({"artifact":"classified_records","rows":[{"id":"i1"}]})`,
	}}
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("errText=%q, want single intermediate custom_transform to pass", errText)
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
	if len(fallback.Actions) != 1 || fallback.Actions[0].Kind != dataquery.DataActionDeriveRules {
		t.Fatalf("Actions=%+v, want one derive_rules action", fallback.Actions)
	}
	if got := fallback.Actions[0].InputPaths; len(got) != 1 || got[0] != "rules.md" {
		t.Fatalf("InputPaths=%v, want rules.md", got)
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
	if strings.Join(paths, ",") != "lookup.json" {
		t.Fatalf("missing paths=%v, want lookup.json to remain scheduled before terminal transform", paths)
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

func TestDataTaskWorkflowStagingGuardAllowsBroadCustomTransformWithPrerequisites(t *testing.T) {
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
	if errText := dataTaskWorkflowStagingGuardError(records, plan); errText != "" {
		t.Fatalf("workflow-covered broad transform should pass, got %q", errText)
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
			Fields: map[string]string{
				"artifact_aliases": "vendor_category_map,vendor_category_map.json",
				"json_shape":       "array(len=2,item=object(keys=canonical_id,source_value,status))",
			},
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
		"json_records(alias)",
		"do not call .get()",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("continuation prompt missing %q:\n%s", want, prompt)
		}
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
