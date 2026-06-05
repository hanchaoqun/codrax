package repl

import (
	"context"
	"fmt"
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
	for _, want := range []string{"actions", "material_inventory", "inspect_material", "custom_transform", "adaptive action workflow", "An action is atomic"} {
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

func TestDataTaskEvaluatorParsesTypedStatus(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{{
			ToolCalls: []llm.ToolCall{{
				Name:   dataTaskEvaluationTool.Name,
				Params: []byte(`{"status":"continue_data","reason":"needs final aggregation","confidence":"high","missingInputs":"final total, row audit"}`),
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
	if eval.Status != dataquery.EvalContinueData || eval.Confidence != "high" {
		t.Fatalf("eval=%+v", eval)
	}
	if len(eval.MissingInputs) != 2 || eval.MissingInputs[1] != "row audit" {
		t.Fatalf("MissingInputs=%v", eval.MissingInputs)
	}
	user := adapter.calls[0].messages[1].Content
	for _, want := range []string{"## data_workflow_rounds", "intermediate", "continue_after"} {
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

func TestPreserveDataTaskMaterialRepairCoverageAllowsExplicitReplacement(t *testing.T) {
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
	if strings.Contains(strings.Join(got.CoverageContract.RequiredPaths(), ","), "old-rules.txt") {
		t.Fatalf("RequiredPaths=%v, old material should not be forced back when replacement contract is explicit", got.CoverageContract.RequiredPaths())
	}
	if !got.CoverageContract.DecisionRecordsRequired {
		t.Fatalf("DecisionRecordsRequired=false")
	}
	if strings.Join(got.InputPaths, ",") != "orders.csv,new-rules.txt" {
		t.Fatalf("InputPaths=%v", got.InputPaths)
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
	if strings.Join(got.InputPaths, ",") != "orders.csv" {
		t.Fatalf("InputPaths=%v", got.InputPaths)
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
