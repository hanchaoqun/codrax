package repl

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
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
	base := `{"status":"ready","output_contract":{"format":"plain_single_line","explanation_allowed":false,"complete_reference":false},"actions":[%s]}`
	tests := []struct {
		name    string
		action  string
		wantErr string
	}{
		{
			name:   "join canonical structured keys",
			action: `{"kind":"join_records","params":{"left_fields":["id"],"right_fields":["id"],"join_type":"inner"}}`,
		},
		{
			name:   "join compatibility alias remains admitted",
			action: `{"kind":"join_records","params":{"left_fields_json":"[\"id\"]","right_key":"id","type":"left"}}`,
		},
		{
			name:    "join rejects enrich-only lookup specs",
			action:  `{"kind":"join_records","params":{"lookup_specs":[{"lookup_path":"labels.csv"}]}}`,
			wantErr: "lookup_specs",
		},
		{
			name:    "filter rejects invented field selector",
			action:  `{"kind":"filter_records","params":{"source_filter_field":"active"}}`,
			wantErr: "source_filter_field",
		},
		{
			name:   "filter native structured carrier remains admitted",
			action: `{"kind":"filter_records","params":{"filters":[{"field":"active","op":"eq","value":true}]}}`,
		},
		{
			name:    "compute rejects phantom include key",
			action:  `{"kind":"compute_contributions","params":{"include":"id"}}`,
			wantErr: "include",
		},
		{
			name:   "uncontracted action remains fail open",
			action: `{"kind":"derive_fields","params":{"future_runtime_owned_key":{"nested":true}}}`,
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
	raw := json.RawMessage(`{"status":"ready","output_contract":{"format":"plain_single_line","explanation_allowed":false,"complete_reference":false},"actions":[{"kind":"join_records","params":{"lookup_specs":[]}}]}`)
	if err := toolparam.Validate(raw, tool.Parameters); err == nil || !strings.Contains(err.Error(), "lookup_specs") {
		t.Fatalf("narrow/repair tool lost runtime parameter contract: %v", err)
	}
	for _, foreign := range []string{`"filter_records"`, `"compute_contributions"`, `"custom_transform"`} {
		if strings.Contains(string(tool.Parameters), foreign) {
			t.Fatalf("join-only schema still teaches foreign action %s: %s", foreign, tool.Parameters)
		}
	}
}
