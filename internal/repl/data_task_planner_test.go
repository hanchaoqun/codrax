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
			dataTaskPlanResp(`{"status":"ready","inputPaths":"orders.csv, vendors.csv","outputContract":{"format":"csv_line","explanationAllowed":"false"},"script":"emit({\"answer\":\"ok,1\",\"output_contract\":{\"format\":\"csv_line\",\"explanation_allowed\":false}})",}` + "\ntrailing"),
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
	if !strings.Contains(plan.Script, "emit") {
		t.Fatalf("Script=%q", plan.Script)
	}
	system := adapter.calls[0].messages[0].Content
	for _, want := range []string{
		"not source-code analysis",
		"or computer operation",
		"output_contract",
		"common data standard libraries",
		"open(path) is read-only",
		"network/process libraries",
		"row-level audit",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("data planner system prompt missing %q:\n%s", want, system)
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
		{Path: "data/a.csv", Kind: "csv", Size: 123},
	})
	if err != nil {
		t.Fatalf("PlanDataTask: %v", err)
	}
	if len(plan.Questions) != 1 {
		t.Fatalf("Questions=%v", plan.Questions)
	}
	user := adapter.calls[0].messages[1].Content
	for _, want := range []string{"## candidate_data_files", "path=data/a.csv", "kind=csv"} {
		if !strings.Contains(user, want) {
			t.Fatalf("data planner prompt missing %q:\n%s", want, user)
		}
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
