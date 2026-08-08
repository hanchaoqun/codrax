package repl

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/llm"
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
	if strings.Contains(string(tool.Parameters), `"reconcile_artifacts"`) || strings.Contains(string(tool.Parameters), `"assemble_answer"`) {
		t.Fatalf("current-rank tool schema leaked future executable kinds: %s", tool.Parameters)
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
