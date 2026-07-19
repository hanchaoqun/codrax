package repl

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
)

// E-2 pins (eval replay#4 run-1, 2026-07-19): the repair planner answered
// with prose instead of a tool call while the completion guard's precise
// typed repair instruction was already in the prompt, and the workflow died
// on "data task planner returned no tool_call" with zero retries — the
// evaluator lane already had a bounded no-tool reprompt, the planner lane
// did not. These tests pin the planner-side twin.

func plannerNoToolScriptedResponses(second llm.Response) *scriptedChatAdapter {
	return &scriptedChatAdapter{
		responses: []llm.Response{
			{Content: "I think we should re-run assemble_answer with complete_reference=true."},
			second,
		},
	}
}

// TestDataTaskPlannerNoToolCallRepromptsWithOriginalContext: one bounded
// reprompt fires, carries the original planning context (including the
// validator's precise repair hint) plus the previous prose, and a tool call
// on the retry is accepted.
func TestDataTaskPlannerNoToolCallRepromptsWithOriginalContext(t *testing.T) {
	adapter := plannerNoToolScriptedResponses(llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name:   dataTaskPlanTool.Name,
			Params: json.RawMessage(`{"goal":"project the grounded answer","status":"ready","actions":[{"id":"ground_output_reference_projection","kind":"assemble_answer","params":{"complete_reference":"true","reference_path":"targets.csv","reference_key_field":"canonical_label"}}]}`),
		}},
	})
	planner := &llmDataTaskPlanner{adapter: adapter}
	prompt := "repair context: data output grounding failed HINT-REF-XYZ; re-run assemble_answer with complete_reference=true"
	plan, err := planner.planDataTask(context.Background(), "data_task_repair_planner", prompt)
	if err != nil {
		t.Fatalf("planDataTask after no-tool reprompt: %v", err)
	}
	if len(plan.Actions) != 1 || string(plan.Actions[0].Kind) != "assemble_answer" {
		t.Fatalf("plan=%+v, want the retried assemble_answer plan accepted", plan)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("calls=%d, want exactly one bounded reprompt after the no-tool response", len(adapter.calls))
	}
	retryUser := adapter.calls[1].messages[len(adapter.calls[1].messages)-1].Content
	for _, want := range []string{
		"did not call the required tool",
		"emit_data_task_plan",
		"HINT-REF-XYZ",
		"previous_content_preview",
	} {
		if !strings.Contains(retryUser, want) {
			t.Fatalf("reprompt=%q, want %q carried (validator hint + previous prose must ride the retry)", retryUser, want)
		}
	}
}

// TestDataTaskPlannerNoToolCallBoundedRetryStaysHonest: a second no-tool
// response keeps the honest typed error — exactly one retry, no loop, and
// the system never fabricates a plan or an answer.
func TestDataTaskPlannerNoToolCallBoundedRetryStaysHonest(t *testing.T) {
	adapter := plannerNoToolScriptedResponses(llm.Response{Content: "still prose"})
	planner := &llmDataTaskPlanner{adapter: adapter}
	_, err := planner.planDataTask(context.Background(), "data_task_repair_planner", "repair context")
	if err == nil {
		t.Fatal("two no-tool responses must keep the honest planner error")
	}
	if !strings.Contains(err.Error(), "returned no tool_call") {
		t.Fatalf("err=%v, want the typed no-tool error", err)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("calls=%d, want exactly one bounded retry (no loop)", len(adapter.calls))
	}
}
