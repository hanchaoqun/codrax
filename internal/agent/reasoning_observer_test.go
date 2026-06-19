package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/reasoninggraph"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestReasoningObserverCapturesToolParamNormalization(t *testing.T) {
	collector := reasoninggraph.NewEventCollector("graph-agent")
	base := &BaseAgent{
		name: types.AgentExplorer,
		deps: &Dependencies{
			ReasoningObserver: collector,
			ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
				types.AgentExplorer: {Mode: types.ToolParamCompatRepair},
			},
		},
	}
	ctx := &types.AgentContext{AgentName: types.AgentExplorer, Stage: types.StageExplore}
	calls := []llm.ToolCall{{
		ID:     "call-1",
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"internal/types/enums.go","offset":"146","limit":"25"}`),
	}}
	got := base.normalizeToolCallParamsWithContext(ctx, calls, []llm.ToolSchema{{
		Name:       "read_file",
		Parameters: readFileCompatTestSchema(),
	}})
	if string(got[0].Params) == string(calls[0].Params) {
		t.Fatalf("expected params to normalize")
	}

	view := collector.View()
	if len(view.ToolEvents) != 1 {
		t.Fatalf("tool events=%+v", view.ToolEvents)
	}
	ev := view.ToolEvents[0]
	if ev.Kind != reasoninggraph.ReasoningEventToolParamNormalized ||
		ev.ToolName != "read_file" ||
		ev.Agent != string(types.AgentExplorer) ||
		ev.Stage != string(types.StageExplore) ||
		ev.OriginalByteLen <= ev.NormalizedByteLen {
		t.Fatalf("unexpected normalized event: %+v", ev)
	}
}

func TestReasoningObserverCapturesMalformedToolParamReject(t *testing.T) {
	collector := reasoninggraph.NewEventCollector("graph-agent")
	base := &BaseAgent{
		name: types.AgentExplorer,
		deps: &Dependencies{ReasoningObserver: collector},
	}
	ctx := &types.AgentContext{AgentName: types.AgentExplorer, Stage: types.StageExplore}
	res, _ := base.executeTool(ctx, llm.ToolCall{
		ID:     "call-bad",
		Name:   "read_file",
		Params: json.RawMessage(`not-json`),
	})
	if res == nil || res.Success {
		t.Fatalf("expected rejected ToolResult, got %+v", res)
	}

	view := collector.View()
	if len(view.RepairEvents) != 1 || view.RepairEvents[0].Kind != reasoninggraph.ReasoningEventSchemaRejected {
		t.Fatalf("schema reject event missing: %+v", view.RepairEvents)
	}
	if len(view.ToolEvents) != 1 || view.ToolEvents[0].Kind != reasoninggraph.ReasoningEventToolCallRejected {
		t.Fatalf("tool rejected event missing: %+v", view.ToolEvents)
	}
	if view.ToolEvents[0].ViolationKind != "malformed_json" {
		t.Fatalf("violation kind = %q", view.ToolEvents[0].ViolationKind)
	}
}

func TestReasoningObserverCapturesRepairPackAndLLMEvents(t *testing.T) {
	collector := reasoninggraph.NewEventCollector("graph-agent")
	base := &BaseAgent{
		name: types.AgentPlanner,
		deps: &Dependencies{ReasoningObserver: collector},
	}
	ctx := &types.AgentContext{AgentName: types.AgentPlanner, Stage: types.StagePlan}
	base.observeToolRepairPackEmitted(ctx, &types.ToolResult{
		ToolName: "emit_change_plan",
		Repair: &types.ToolRepair{
			Code:   types.PlanRepairToolCode,
			Fields: []string{"changes[0].old_text"},
		},
	})
	base.observeLLMRequestWaiting(ctx, 1, llm.RequestTelemetry{ModelID: "planner-model"}, 31*time.Second)
	base.observeLLMRequestRetried(ctx, 2, 4*time.Second, "rate limit")
	base.observeLLMFallbackRouted(ctx, "primary-model", "fallback-model", "primary failed")

	view := collector.View()
	if len(view.LLMEvents) != 2 {
		t.Fatalf("LLM events=%+v", view.LLMEvents)
	}
	if view.LLMEvents[0].Kind != reasoninggraph.ReasoningEventLLMRequestWaiting ||
		view.LLMEvents[0].ElapsedMillis != 31000 {
		t.Fatalf("waiting event=%+v", view.LLMEvents[0])
	}
	if view.LLMEvents[1].Kind != reasoninggraph.ReasoningEventLLMRequestRetried ||
		view.LLMEvents[1].Attempt != 2 ||
		view.LLMEvents[1].Message != "rate limit" {
		t.Fatalf("retry event=%+v", view.LLMEvents[1])
	}
	if len(view.RepairEvents) != 2 {
		t.Fatalf("repair/fallback events=%+v", view.RepairEvents)
	}
	if view.RepairEvents[0].RepairCode != types.PlanRepairToolCode ||
		view.RepairEvents[0].ToolName != "emit_change_plan" {
		t.Fatalf("repair pack event=%+v", view.RepairEvents[0])
	}
	if view.RepairEvents[1].Kind != reasoninggraph.ReasoningEventFallbackRouted ||
		view.RepairEvents[1].FallbackTarget != "fallback-model" {
		t.Fatalf("fallback event=%+v", view.RepairEvents[1])
	}
}
