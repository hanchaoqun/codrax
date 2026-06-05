package repl

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/llm"
)

func TestDataTaskPlannerCompatJSON(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			dataTaskPlanResp(`{"status":"ready","inputPaths":"orders.csv, vendors.csv","outputContract":{"format":"csv_line","explanationAllowed":"false"},"coverageContract":{"requiredMaterials":[{"id":"m1","path":"orders.csv","purpose":"input rows","required":"true"}],"validationRules":"all totals reconcile; rows cite source","decisionRecordsRequired":"true","ruleCoverageRequired":"true","contributionLedgerRequired":"true","entityResolutionRequired":"true","reconcileRequired":"true"},"goal":123,"knownConstraints":"read only; strict output","missingObservations":["invoice total",42],"successCriteria":"final total returned","nextBatch":true,"whyThisBatch":456,"continueAfter":"true","script":"emit({\"answer\":\"ok,1\",\"output_contract\":{\"format\":\"csv_line\",\"explanation_allowed\":false}})",}` + "\ntrailing"),
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
	} {
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
		Script: `print("debug")`,
	}
	plan, err := repairer.RepairDataTask(context.Background(), "汇总 CSV", "/repo", TurnPolicy{Route: RouteData}, []dataquery.CandidateFile{{Path: "orders.csv", Kind: "csv", Size: 10}}, previous, `NameError: name 'print' is not defined`)
	if err != nil {
		t.Fatalf("RepairDataTask: %v", err)
	}
	if plan.Status != "ready" || !strings.Contains(plan.Script, "emit") {
		t.Fatalf("repaired plan=%+v", plan)
	}
	user := adapter.calls[0].messages[1].Content
	for _, want := range []string{"## execution_error", "NameError", "## previous_plan_json", `print(\"debug\")`, "coverage_contract", "required_materials", "input_paths alone is not material consumption", "actually read", "operation pipeline"} {
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

func dataTaskPlanResp(raw string) llm.Response {
	return llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name:   dataTaskPlanTool.Name,
			Params: []byte(raw),
		}},
		StopReason: "tool_use",
	}
}
