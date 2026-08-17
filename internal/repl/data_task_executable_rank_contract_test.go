package repl

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/dataworkflow"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/toolparam"
)

func dataTaskToolActionKindEnum(t *testing.T, tool llm.ToolSchema) []string {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
		t.Fatalf("tool schema JSON: %v", err)
	}
	properties, _ := schema["properties"].(map[string]any)
	actions, _ := properties["actions"].(map[string]any)
	items, _ := actions["items"].(map[string]any)
	itemProperties, _ := items["properties"].(map[string]any)
	kind, _ := itemProperties["kind"].(map[string]any)
	raw, _ := kind["enum"].([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if value, ok := item.(string); ok {
			out = append(out, value)
		}
	}
	return out
}

func deriveRulesRuntimeView() dataTaskWorkflowRuntimeView {
	contract := dataquery.CoverageContract{
		RequiredMaterials: []dataquery.CoverageMaterial{
			{Path: "records.csv", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
			{Path: "rules.md", Required: true, UsageMode: dataquery.MaterialUseScriptConsumed},
		},
		RuleCoverageRequired: true,
	}
	return dataTaskWorkflowRuntimeView{
		Records: []dataTaskWorkflowRecord{{
			Plan: dataquery.TaskPlan{CoverageContract: contract},
			Result: &dataquery.Result{
				ConsumedPaths: []string{"records.csv", "rules.md"},
			},
		}},
		CurrentPlan: dataquery.TaskPlan{CoverageContract: contract},
	}
}

func TestDataTaskExecutableRankNarrowsToolAndPromptFromSameState(t *testing.T) {
	view := deriveRulesRuntimeView()
	rank := dataTaskExecutableRankContractFromRuntimeView("", view)
	if rank.NextStage != "derive_rules" || !slices.Equal(rank.AllowedActionKinds, []string{"derive_rules"}) {
		t.Fatalf("rank=%+v, want derive_rules only", rank)
	}
	tool, err := dataTaskPlanToolForExecutableRank(rank)
	if err != nil {
		t.Fatal(err)
	}
	if got := dataTaskToolActionKindEnum(t, tool); !slices.Equal(got, rank.AllowedActionKinds) {
		t.Fatalf("tool action enum=%v, rank=%v", got, rank.AllowedActionKinds)
	}
	for _, futureKind := range []string{`"join_records"`, `"compute_contributions"`, `"reconcile_artifacts"`, `"assemble_answer"`, `"custom_transform"`} {
		if !strings.Contains(string(tool.Parameters), futureKind) {
			continue
		}
		t.Fatalf("current-rank tool schema leaked future executable kind %s: %s", futureKind, tool.Parameters)
	}

	prompt := dataTaskContinuationPromptWithRuntimeViewAndRank("continue", "/repo", TurnPolicy{Route: RouteData}, nil, view, rank)
	wantJSON, _ := json.Marshal(rank)
	if !strings.Contains(prompt, "## executable_next_rank\n"+string(wantJSON)) {
		t.Fatalf("continuation prompt missing exact rank carrier:\n%s", prompt)
	}
	if strings.Contains(prompt, "After that, emit compute_contributions") {
		t.Fatalf("continuation prompt presents future ranks as executable imperatives:\n%s", prompt)
	}
	if strings.LastIndex(prompt, "## executable_next_rank") < strings.LastIndex(prompt, "## current_plan_emission_contract") {
		t.Fatalf("current-rank carrier must be the final copyable contract")
	}
}

func TestDataTaskContinuationUsesNarrowedRankTool(t *testing.T) {
	adapter := &scriptedChatAdapter{responses: []llm.Response{dataTaskPlanResp(`{
		"status":"ready",
		"actions":[{"id":"rules","kind":"derive_rules","input_paths":["rules.md"],"output_artifact":"rules.json"}],
		"continue_after":true,
		"next_batch":"continue from the next typed rank"
	}`)}}
	planner := &llmDataTaskPlanner{adapter: adapter}
	plan, err := planner.ContinueDataTaskWithRuntimeView(context.Background(), "continue", "", TurnPolicy{Route: RouteData}, nil, deriveRulesRuntimeView())
	if err != nil {
		t.Fatalf("ContinueDataTaskWithRuntimeView: %v", err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionDeriveRules {
		t.Fatalf("plan=%+v, want one derive_rules action", plan)
	}
	if len(adapter.calls) != 1 || len(adapter.calls[0].tools) != 1 {
		t.Fatalf("calls=%d tools=%v", len(adapter.calls), adapter.calls)
	}
	if got := dataTaskToolActionKindEnum(t, adapter.calls[0].tools[0]); !slices.Equal(got, []string{"derive_rules"}) {
		t.Fatalf("continuation tool action enum=%v, want current rank only", got)
	}
}

func TestDataTaskRepairUsesSameNarrowedRankToolAndPrompt(t *testing.T) {
	adapter := &scriptedChatAdapter{responses: []llm.Response{dataTaskPlanResp(`{
		"status":"ready",
		"actions":[{"id":"rules-repair","kind":"derive_rules","input_paths":["rules.md"],"output_artifact":"rules.json"}],
		"continue_after":true,
		"next_batch":"continue from the next typed rank"
	}`)}}
	planner := &llmDataTaskPlanner{adapter: adapter}
	view := deriveRulesRuntimeView()
	plan, err := planner.RepairDataTaskWithRuntimeView(
		context.Background(),
		"continue",
		"",
		TurnPolicy{Route: RouteData},
		nil,
		view.CurrentPlan,
		"typed action requires repair",
		dataquery.DataTaskViolation{Code: "action_param_violation", ActionKind: "derive_rules"},
		view,
	)
	if err != nil {
		t.Fatalf("RepairDataTaskWithRuntimeView: %v", err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionDeriveRules {
		t.Fatalf("plan=%+v, want one derive_rules action", plan)
	}
	if got := dataTaskToolActionKindEnum(t, adapter.calls[0].tools[0]); !slices.Equal(got, []string{"derive_rules"}) {
		t.Fatalf("repair tool action enum=%v, want current rank only", got)
	}
	userPrompt := adapter.calls[0].messages[len(adapter.calls[0].messages)-1].Content
	if !strings.Contains(userPrompt, `"allowed_action_kinds":["derive_rules"]`) || !strings.Contains(userPrompt, "read-only until allowed") {
		t.Fatalf("repair prompt and schema do not share current rank:\n%s", userPrompt)
	}
}

func TestDataTaskMalformedParamRepairKeepsNarrowedRankTool(t *testing.T) {
	rank := dataTaskExecutableRankContract{NextStage: "derive_rules", AllowedActionKinds: []string{"derive_rules"}, FutureRanks: "read_only_roadmap"}
	tool, err := dataTaskPlanToolForExecutableRank(rank)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &scriptedChatAdapter{responses: []llm.Response{
		{ToolCalls: []llm.ToolCall{{Name: tool.Name, Params: json.RawMessage(`{"status":`)}}},
		dataTaskPlanResp(`{"status":"ready","actions":[{"kind":"derive_rules","input_paths":["rules.md"],"output_artifact":"rules.json"}],"continue_after":true,"next_batch":"continue"}`),
	}}
	planner := &llmDataTaskPlanner{adapter: adapter}
	plan, err := planner.planDataTaskWithTool(context.Background(), "data_task_continuation_planner", "typed current rank", tool)
	if err != nil {
		t.Fatalf("planDataTaskWithTool: %v", err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionDeriveRules {
		t.Fatalf("plan=%+v, want repaired derive_rules plan", plan)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("calls=%d, want planner plus compact structured repair", len(adapter.calls))
	}
	for i, call := range adapter.calls {
		if len(call.tools) != 1 {
			t.Fatalf("call %d tools=%v", i, call.tools)
		}
		if got := dataTaskToolActionKindEnum(t, call.tools[0]); !slices.Equal(got, []string{"derive_rules"}) {
			t.Fatalf("call %d action enum=%v, want same narrowed repair schema", i, got)
		}
	}
	if !strings.Contains(adapter.calls[1].messages[0].Content, "single valid JSON object") {
		t.Fatalf("structured repair lost minimal JSON teaching: %+v", adapter.calls[1].messages)
	}
}

func TestDataTaskStringifiedActionsCannotBypassCurrentRankEnum(t *testing.T) {
	rank := dataTaskExecutableRankContract{NextStage: "derive_rules", AllowedActionKinds: []string{"derive_rules"}, FutureRanks: "read_only_roadmap"}
	tool, err := dataTaskPlanToolForExecutableRank(rank)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &scriptedChatAdapter{responses: []llm.Response{
		dataTaskPlanResp(`{"status":"ready","actions":"[{\"kind\":\"derive_rules\"},{\"kind\":\"assemble_answer\"}]","continue_after":true,"next_batch":"continue"}`),
		dataTaskPlanResp(`{"status":"ready","actions":[{"kind":"derive_rules"}],"continue_after":true,"next_batch":"continue"}`),
	}}
	planner := &llmDataTaskPlanner{adapter: adapter}
	plan, err := planner.planDataTaskWithTool(context.Background(), "data_task_continuation_planner", "typed current rank", tool)
	if err != nil {
		t.Fatalf("planDataTaskWithTool: %v", err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionDeriveRules {
		t.Fatalf("future-rank member survived same-schema repair: %+v", plan.Actions)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("calls=%d, want planner plus one bounded same-schema repair", len(adapter.calls))
	}
	for i, call := range adapter.calls {
		if got := dataTaskToolActionKindEnum(t, call.tools[0]); !slices.Equal(got, []string{"derive_rules"}) {
			t.Fatalf("call %d action enum=%v, want current rank only", i, got)
		}
	}
	if !strings.Contains(adapter.calls[1].messages[1].Content, "$.actions[1].kind") {
		t.Fatalf("compact repair did not receive the precise nested schema violation: %+v", adapter.calls[1].messages)
	}
}

func TestDataTaskNativeActionsCannotBypassCurrentRankEnum(t *testing.T) {
	rank := dataTaskExecutableRankContract{NextStage: "derive_rules", AllowedActionKinds: []string{"derive_rules"}, FutureRanks: "read_only_roadmap", NativeSchemaAuthoritative: true}
	tool, err := dataTaskPlanToolForExecutableRank(rank)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &scriptedChatAdapter{responses: []llm.Response{
		dataTaskPlanResp(`{"status":"ready","actions":[{"kind":"assemble_answer"}],"continue_after":true,"next_batch":"continue"}`),
		dataTaskPlanResp(`{"status":"ready","actions":[{"kind":"derive_rules"}],"continue_after":true,"next_batch":"continue"}`),
	}}
	planner := &llmDataTaskPlanner{adapter: adapter}
	plan, err := planner.planDataTaskWithTool(context.Background(), "data_task_continuation_planner", "typed current rank", tool)
	if err != nil {
		t.Fatalf("planDataTaskWithTool: %v", err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionDeriveRules {
		t.Fatalf("future-rank native member survived same-schema repair: %+v", plan.Actions)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("calls=%d, want planner plus one bounded same-schema repair", len(adapter.calls))
	}
	if !strings.Contains(adapter.calls[1].messages[1].Content, "$.actions[0].kind") {
		t.Fatalf("compact repair did not receive the precise native schema violation: %+v", adapter.calls[1].messages)
	}
}

func TestDataTaskCurrentPlanOnlyRankRemainsNonAuthoritative(t *testing.T) {
	rank := dataTaskExecutableRankContractFromRuntimeView("", dataTaskWorkflowRuntimeView{
		CurrentPlan: dataquery.TaskPlan{
			Status:  "ready",
			Actions: []dataquery.DataAction{{Kind: dataquery.DataActionCustomTransform}},
		},
	})
	if rank.NativeSchemaAuthoritative {
		t.Fatalf("CurrentPlan-only compatibility view must not mint hard rank authority: %+v", rank)
	}
	tool, err := dataTaskPlanToolForExecutableRank(rank)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
		t.Fatal(err)
	}
	if _, ok := schema[replNativeValidationPropertiesSchemaKey]; ok {
		t.Fatalf("non-authoritative rank must not inherit hard action validation: %s", tool.Parameters)
	}
}

func TestDataTaskReducerRuntimeRankIsAuthoritativeBeforeFirstRecord(t *testing.T) {
	rt := dataworkflow.NewWorkflowRuntime(dataquery.TaskPlan{
		Status:  "ready",
		Actions: []dataquery.DataAction{{Kind: dataquery.DataActionCustomTransform}},
	})
	view := dataTaskWorkflowRuntimeViewFrom(rt, nil, dataquery.TaskPlan{}, dataquery.TaskPlan{}, 0, 0)
	rank := dataTaskExecutableRankContractFromRuntimeView("", view)
	if !rank.NativeSchemaAuthoritative {
		t.Fatalf("live reducer runtime must carry hard rank authority before the first record: view=%+v rank=%+v", view, rank)
	}
	tool, err := dataTaskPlanToolForExecutableRank(rank)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tool.Parameters), replNativeValidationPropertiesSchemaKey) {
		t.Fatalf("authoritative rank did not opt actions into native validation: %s", tool.Parameters)
	}
}

func TestDataTaskInitialPlannerKeepsFullDiscoveryVocabulary(t *testing.T) {
	adapter := &scriptedChatAdapter{responses: []llm.Response{dataTaskPlanResp(`{"status":"ready","actions":[{"kind":"material_inventory"}],"continue_after":true,"next_batch":"inspect materials"}`)}}
	planner := &llmDataTaskPlanner{adapter: adapter}
	if _, err := planner.PlanDataTask(context.Background(), "inspect data", "", TurnPolicy{Route: RouteData}, nil); err != nil {
		t.Fatalf("PlanDataTask: %v", err)
	}
	got := dataTaskToolActionKindEnum(t, adapter.calls[0].tools[0])
	for _, want := range []string{"material_inventory", "derive_rules", "compute_contributions", "reconcile_artifacts", "assemble_answer"} {
		if !slices.Contains(got, want) {
			t.Fatalf("initial tool action enum=%v, missing %q", got, want)
		}
	}
}

func TestDataTaskToolProjectsRuntimeActionParamContracts(t *testing.T) {
	base := `{"status":"ready","output_contract":{"format":"json_only","explanation_allowed":false,"complete_reference":false},"actions":[%s]}`
	tests := []struct {
		name    string
		action  string
		wantErr string
	}{
		{
			name:   "join canonical structured keys",
			action: `{"kind":"join_records","input_paths":["left.json","right.json"],"params":{"left_fields":["id"],"right_fields":["id"],"join_type":"inner"}}`,
		},
		{
			name:   "join compatibility alias remains admitted",
			action: `{"kind":"join_records","input_paths":["left.json","right.json"],"params":{"left_fields_json":"[\"id\"]","right_key":"id","type":"left"}}`,
		},
		{
			name:    "join rejects enrich-only lookup specs",
			action:  `{"kind":"join_records","input_paths":["left.json","right.json"],"params":{"lookup_specs":[{"lookup_path":"labels.csv"}]}}`,
			wantErr: "lookup_specs",
		},
		{
			name:    "filter rejects invented field selector",
			action:  `{"kind":"filter_records","input_paths":["records.json"],"params":{"source_filter_field":"active"}}`,
			wantErr: "source_filter_field",
		},
		{
			name:   "filter native structured carrier remains admitted",
			action: `{"kind":"filter_records","input_paths":["records.json"],"params":{"filters":[{"field":"active","op":"eq","value":true}]}}`,
		},
		{
			name:   "assemble external output field remains admitted",
			action: `{"kind":"assemble_answer","params":{"projection":"json_object","output_field":"ids"}}`,
		},
		{
			name:    "assemble rejects output rename in internal group carrier",
			action:  `{"kind":"assemble_answer","params":{"projection":"json_object","group_key":"ids"}}`,
			wantErr: "group_key",
		},
		{
			name:    "compute rejects phantom include key",
			action:  `{"kind":"compute_contributions","input_paths":["records.json"],"params":{"include":"id"}}`,
			wantErr: "include",
		},
		{
			name:    "compute count rejects member field before execution",
			action:  `{"kind":"compute_contributions","input_paths":["records.json"],"params":{"operation":"count","value_field":"id"}}`,
			wantErr: "value_field",
		},
		{
			name:   "compute include keeps member field",
			action: `{"kind":"compute_contributions","input_paths":["records.json"],"params":{"operation":"include","value_field":"id"}}`,
		},
		{
			name:   "uncontracted action remains fail open",
			action: `{"kind":"derive_fields","input_paths":["records.json"],"params":{"future_runtime_owned_key":{"nested":true}}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage([]byte(fmt.Sprintf(base, tc.action)))
			err := toolparam.Validate(raw, dataTaskPlanTool.Parameters)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("schema rejected runtime-admitted action params: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("schema err=%v, want rejected key %q", err, tc.wantErr)
			}
		})
	}
}

func TestDataTaskInitialPlannerExecutesNativeActionSchema(t *testing.T) {
	adapter := &scriptedChatAdapter{responses: []llm.Response{
		dataTaskPlanResp(`{"status":"ready","output_contract":{"format":"plain_single_line","explanation_allowed":false,"complete_reference":false},"actions":[{"kind":"extract_records","input_paths":["records.json"],"script":"emit_result(1)"}]}`),
		dataTaskPlanResp(`{"status":"ready","output_contract":{"format":"plain_single_line","explanation_allowed":false,"complete_reference":false},"actions":[{"kind":"extract_records","input_paths":["records.json"]}]}`),
	}}
	planner := NewDataTaskPlanner(adapter)
	plan, err := planner.PlanDataTask(context.Background(), "inspect data", t.TempDir(), TurnPolicy{Route: RouteData}, []dataquery.CandidateFile{{Path: "records.json", Kind: "json"}})
	if err != nil {
		t.Fatalf("PlanDataTask: %v", err)
	}
	if len(adapter.calls) != 2 || len(plan.Actions) != 1 || plan.Actions[0].Kind != dataquery.DataActionExtractRecords || plan.Actions[0].Script != "" {
		t.Fatalf("calls=%d plan=%+v, want one compact schema repair before workflow admission", len(adapter.calls), plan)
	}
}

func TestDataTaskInitialPlannerRepairsProjectionOutputFormatConflict(t *testing.T) {
	adapter := &scriptedChatAdapter{responses: []llm.Response{
		dataTaskPlanResp(`{"status":"ready","output_contract":{"format":"plain_single_line","explanation_allowed":false,"complete_reference":true,"reference_path":"targets.csv","reference_key_field":"canonical_label"},"actions":[{"kind":"assemble_answer","params":{"projection":"json_object","output_field":"count","reference_path":"targets.csv","reference_key_field":"canonical_label"}}]}`),
		dataTaskPlanResp(`{"status":"ready","output_contract":{"format":"plain_single_line","explanation_allowed":false,"complete_reference":true,"reference_path":"targets.csv","reference_key_field":"canonical_label"},"actions":[{"kind":"assemble_answer","params":{"projection":"values","reference_path":"targets.csv","reference_key_field":"canonical_label"}}]}`),
	}}
	planner := NewDataTaskPlanner(adapter)
	plan, err := planner.PlanDataTask(context.Background(), "produce one plain value", t.TempDir(), TurnPolicy{Route: RouteData}, nil)
	if err != nil {
		t.Fatalf("PlanDataTask: %v", err)
	}
	if len(adapter.calls) != 2 || len(plan.Actions) != 1 || plan.Actions[0].Params["projection"] != "values" {
		t.Fatalf("calls=%d plan=%+v, want one compact same-tool repair to a compatible projection", len(adapter.calls), plan)
	}
	if !strings.Contains(adapter.calls[1].messages[1].Content, "output_contract.format=plain_single_line") {
		t.Fatalf("compact repair lost typed conflict: %+v", adapter.calls[1].messages)
	}
	repairPrompt := adapter.calls[1].messages[1].Content
	for _, want := range []string{
		"## previous_tool_params",
		`"complete_reference":true`,
		`"reference_path":"targets.csv"`,
		`"output_field":"count"`,
		"preserve every unrelated valid field",
	} {
		if !strings.Contains(repairPrompt, want) {
			t.Fatalf("compact repair prompt omitted prior structural context %q:\n%s", want, repairPrompt)
		}
	}
	if !plan.OutputContract.CompleteReference || plan.OutputContract.ReferencePath != "targets.csv" || plan.OutputContract.ReferenceKeyField != "canonical_label" {
		t.Fatalf("compact projection repair lost unrelated complete-reference authority: %+v", plan.OutputContract)
	}
}

func TestDataTaskInitialPlannerConvergesTwoIndependentStructuredErrors(t *testing.T) {
	initial := `{"status":"ready","output_contract":{"format":"plain_single_line","explanation_allowed":false,"complete_reference":false},"actions":[{"id":"compute","kind":"custom_transform","input_paths":["records.json"],"output_artifact":"counts"},{"id":"publish","kind":"assemble_answer","input_paths":["counts"],"output_artifact":"answer","params":{"projection":"values","output_field":"count"}}]}`
	firstRepair := `{"status":"ready","output_contract":{"format":"plain_single_line","explanation_allowed":false,"complete_reference":false},"actions":[{"id":"compute","kind":"custom_transform","input_paths":["records.json"],"output_artifact":"counts","script":"emit_result(1)"},{"id":"publish","kind":"assemble_answer","input_paths":["counts"],"output_artifact":"answer","params":{"projection":"values","output_field":"count"}}]}`
	secondRepair := `{"status":"ready","output_contract":{"format":"plain_single_line","explanation_allowed":false,"complete_reference":false},"actions":[{"id":"compute","kind":"custom_transform","input_paths":["records.json"],"output_artifact":"counts","script":"emit_result(1)"},{"id":"publish","kind":"assemble_answer","input_paths":["counts"],"output_artifact":"answer","params":{"projection":"values"}}]}`
	adapter := &scriptedChatAdapter{responses: []llm.Response{
		dataTaskPlanResp(initial),
		dataTaskPlanResp(firstRepair),
		dataTaskPlanResp(secondRepair),
	}}
	planner := NewDataTaskPlanner(adapter)
	plan, err := planner.PlanDataTask(context.Background(), "aggregate records", t.TempDir(), TurnPolicy{Route: RouteData}, nil)
	if err != nil {
		t.Fatalf("PlanDataTask: %v", err)
	}
	if len(adapter.calls) != 3 {
		t.Fatalf("calls=%d, want planner plus two bounded structured repairs", len(adapter.calls))
	}
	if len(plan.Actions) != 2 || plan.Actions[0].Script == "" {
		t.Fatalf("first structured correction did not survive the second pass: %+v", plan.Actions)
	}
	if _, exists := plan.Actions[1].Params["output_field"]; exists {
		t.Fatalf("second structured correction did not remove the incompatible output_field: %+v", plan.Actions[1])
	}
	secondPrompt := adapter.calls[2].messages[1].Content
	for _, want := range []string{
		"## previous_tool_params",
		`"script":"emit_result(1)"`,
		"output_field",
		"preserve every unrelated valid field",
	} {
		if !strings.Contains(secondPrompt, want) {
			t.Fatalf("second repair prompt lost prior correction or latest typed locus %q:\n%s", want, secondPrompt)
		}
	}
}

func TestDataTaskToolTeachesAssembleOutputFieldFromRuntimeContract(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(dataTaskPlanTool.Parameters, &schema); err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	actions := properties["actions"].(map[string]any)
	items := actions["items"].(map[string]any)
	for _, raw := range items["allOf"].([]any) {
		if kind, ok := dataTaskActionConditionalKind(raw); !ok || kind != string(dataquery.DataActionAssembleAnswer) {
			continue
		}
		branch := raw.(map[string]any)
		then := branch["then"].(map[string]any)
		thenProperties := then["properties"].(map[string]any)
		params := thenProperties["params"].(map[string]any)
		paramProperties := params["properties"].(map[string]any)
		outputField := paramProperties["output_field"].(map[string]any)
		description, _ := outputField["description"].(string)
		if !strings.Contains(description, "External JSON object field") || !strings.Contains(description, "group_key") {
			t.Fatalf("output_field schema=%+v, want executor-owned external/internal distinction", outputField)
		}
		return
	}
	t.Fatal("assemble_answer runtime parameter schema branch missing")
}

func TestDataTaskToolProjectsRuntimeActionInputContracts(t *testing.T) {
	base := `{"status":"ready","output_contract":{"format":"plain_single_line","explanation_allowed":false,"complete_reference":false},"actions":[%s]}`
	tests := []struct {
		name    string
		action  string
		wantErr string
	}{
		{name: "compute exact input", action: `{"kind":"compute_contributions","input_paths":["records.json"]}`},
		{name: "compute missing input", action: `{"kind":"compute_contributions"}`, wantErr: "input_paths"},
		{name: "compute empty input", action: `{"kind":"compute_contributions","input_paths":[]}`, wantErr: "minimum is 1"},
		{name: "single record action rejects two", action: `{"kind":"compute_contributions","input_paths":["a.json","b.json"]}`, wantErr: "maximum is 1"},
		{name: "join exact pair", action: `{"kind":"join_records","input_paths":["left.json","right.json"]}`},
		{name: "join rejects one", action: `{"kind":"join_records","input_paths":["left.json"]}`, wantErr: "minimum is 2"},
		{name: "inventory needs no input", action: `{"kind":"material_inventory"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage([]byte(fmt.Sprintf(base, tc.action)))
			err := toolparam.Validate(raw, dataTaskPlanTool.Parameters)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("schema rejected capability-valid action inputs: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("schema err=%v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestDataTaskNarrowedAndRepairToolsKeepSameParamContract(t *testing.T) {
	rank := dataTaskExecutableRankContract{
		NextStage:          "normalize_or_enrich_entities",
		AllowedActionKinds: []string{"join_records"},
		FutureRanks:        "read_only_roadmap",
	}
	tool, err := dataTaskPlanToolForExecutableRank(rank)
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"status":"ready","output_contract":{"format":"plain_single_line","explanation_allowed":false,"complete_reference":false},"actions":[{"kind":"join_records","input_paths":["left.json","right.json"],"params":{"lookup_specs":[]}}]}`)
	if err := toolparam.Validate(raw, tool.Parameters); err == nil || !strings.Contains(err.Error(), "lookup_specs") {
		t.Fatalf("narrow/repair tool lost runtime parameter contract: %v", err)
	}
	for _, foreign := range []string{`"filter_records"`, `"compute_contributions"`, `"custom_transform"`} {
		if strings.Contains(string(tool.Parameters), foreign) {
			t.Fatalf("join-only schema still teaches foreign action %s: %s", foreign, tool.Parameters)
		}
	}
}
