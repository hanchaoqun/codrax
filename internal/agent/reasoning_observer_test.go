package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/reasoninggraph"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type observedRuntimeTool struct {
	toolpkg.ReadOnly
	toolpkg.NonEvidenceTool
	name   string
	result types.ToolResult
	err    error
}

func (t *observedRuntimeTool) Name() string { return t.name }
func (t *observedRuntimeTool) Description() string {
	return "test tool for runtime observation"
}
func (t *observedRuntimeTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}
func (t *observedRuntimeTool) Execute(_ *types.BusContext, _ json.RawMessage) (types.ToolResult, error) {
	result := t.result
	if result.ToolName == "" {
		result.ToolName = t.name
	}
	return result, t.err
}

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
	if view.ToolEvents[0].Kind == reasoninggraph.ReasoningEventToolCallObserved {
		t.Fatalf("malformed params must not be reported as executed: %+v", view.ToolEvents[0])
	}
}

func TestReasoningObserverCapturesLocalToolObservedSuccess(t *testing.T) {
	collector := reasoninggraph.NewEventCollector("graph-agent")
	reg := toolpkg.NewRegistry()
	reg.Register(&observedRuntimeTool{
		name: "observed_success",
		result: types.ToolResult{
			ToolName: "observed_success",
			RawRef:   "blob://tool/raw-success",
			Observations: []types.ObservationRecord{{
				ID:       "obs-1",
				Producer: "observed_success",
			}},
			Success:   true,
			Timestamp: time.Now(),
		},
	})
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{
		Tools:             reg,
		ReasoningObserver: collector,
	}, nil)
	ctx := &types.AgentContext{AgentName: types.AgentExplorer, Stage: types.StageExplore}

	res, _ := base.executeTool(ctx, llm.ToolCall{
		ID:     "call-observed-success",
		Name:   "observed_success",
		Params: json.RawMessage(`{}`),
	})
	if res == nil || !res.Success {
		t.Fatalf("expected successful ToolResult, got %+v", res)
	}

	view := collector.View()
	if len(view.ToolEvents) != 1 {
		t.Fatalf("tool events=%+v", view.ToolEvents)
	}
	ev := view.ToolEvents[0]
	if ev.Kind != reasoninggraph.ReasoningEventToolCallObserved ||
		ev.ReasonCode != "tool_call_ok" ||
		ev.ActionStatus != "success" ||
		ev.ToolName != "observed_success" ||
		ev.Agent != string(types.AgentExplorer) ||
		ev.Stage != string(types.StageExplore) ||
		ev.ToolResultCount != 1 ||
		ev.FactCount != 1 ||
		ev.OutputRefCount != 1 ||
		ev.PayloadRef != "blob://tool/raw-success" ||
		ev.ElapsedMillis < 0 {
		t.Fatalf("unexpected observed success event: %+v", ev)
	}
}

func TestToolInvocationStampedInProduction(t *testing.T) {
	collector := reasoninggraph.NewEventCollector("graph-agent")
	reg := toolpkg.NewRegistry()
	reg.Register(&observedRuntimeTool{
		name: "observed_artifact",
		result: types.ToolResult{
			ToolName:  "observed_artifact",
			Summary:   "small result",
			Success:   true,
			Timestamp: time.Now(),
		},
	})
	workDir := t.TempDir()
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{
		Tools:             reg,
		ReasoningObserver: collector,
	}, nil)
	ctx := &types.AgentContext{
		AgentName: types.AgentExplorer,
		Stage:     types.StageExplore,
		WorkDir:   workDir,
	}

	res, _ := base.executeTool(ctx, llm.ToolCall{
		ID:     "call-artifact",
		Name:   "observed_artifact",
		Params: json.RawMessage(`{}`),
	})
	if res == nil || !res.Success {
		t.Fatalf("expected successful ToolResult, got %+v", res)
	}

	view := collector.View()
	if len(view.ToolEvents) != 1 {
		t.Fatalf("tool events=%+v", view.ToolEvents)
	}
	ev := view.ToolEvents[0]
	if ev.InvocationID != "call-artifact" ||
		ev.ParamsRef == "" ||
		ev.ResultRef == "" ||
		ev.PayloadRef != ev.ResultRef {
		t.Fatalf("invocation refs missing: %+v", ev)
	}
	if _, err := os.Stat(ev.ParamsRef); err != nil {
		t.Fatalf("params ref not persisted: %v", err)
	}
	if _, err := os.Stat(ev.ResultRef); err != nil {
		t.Fatalf("result ref not persisted: %v", err)
	}
}

func TestToolInvocationObserverSideEffectFree(t *testing.T) {
	collector := reasoninggraph.NewEventCollector("graph-agent")
	reg := toolpkg.NewRegistry()
	reg.Register(&observedRuntimeTool{
		name: "observed_side_effect_free",
		result: types.ToolResult{
			ToolName:  "observed_side_effect_free",
			Summary:   "model visible summary",
			Success:   true,
			Timestamp: time.Now(),
		},
	})
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{
		Tools:             reg,
		ReasoningObserver: collector,
	}, nil)
	ctx := &types.AgentContext{
		AgentName: types.AgentExplorer,
		Stage:     types.StageExplore,
		WorkDir:   t.TempDir(),
	}

	res, _ := base.executeTool(ctx, llm.ToolCall{
		ID:     "call-side-effect-free",
		Name:   "observed_side_effect_free",
		Params: json.RawMessage(`{}`),
	})
	if res == nil || res.Summary != "model visible summary" {
		t.Fatalf("observer changed model-visible result: %+v", res)
	}
	if view := collector.View(); len(view.ToolEvents) != 1 || view.ToolEvents[0].ResultRef == "" {
		t.Fatalf("expected side-effect-only graph event, got %+v", view.ToolEvents)
	}
}

func TestToolInvocationReplayable(t *testing.T) {
	collector := reasoninggraph.NewEventCollector("graph-agent")
	reg := toolpkg.NewRegistry()
	reg.Register(&observedRuntimeTool{
		name: "observed_replay",
		result: types.ToolResult{
			ToolName:  "observed_replay",
			Summary:   "replay result",
			Success:   true,
			Timestamp: time.Now(),
		},
	})
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{
		Tools:             reg,
		ReasoningObserver: collector,
	}, nil)
	ctx := &types.AgentContext{
		AgentName: types.AgentExplorer,
		Stage:     types.StageExplore,
		WorkDir:   t.TempDir(),
	}

	base.executeTool(ctx, llm.ToolCall{
		ID:     "call-replay",
		Name:   "observed_replay",
		Params: json.RawMessage(`{}`),
	})
	replay, err := (reasoninggraph.GraphReplayExecutor{}).Replay(context.Background(), reasoninggraph.GraphReplayRequest{
		Events: collector.Events(),
		Source: "agent_test",
	})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(replay.View.ToolEvents) != 1 ||
		replay.View.ToolEvents[0].InvocationID != "call-replay" ||
		replay.View.ToolEvents[0].ParamsRef == "" ||
		replay.View.ToolEvents[0].ResultRef == "" {
		t.Fatalf("replay lost invocation refs: %+v", replay.View.ToolEvents)
	}
}

func TestReasoningObserverCapturesLocalToolObservedFailure(t *testing.T) {
	collector := reasoninggraph.NewEventCollector("graph-agent")
	reg := toolpkg.NewRegistry()
	reg.Register(&observedRuntimeTool{
		name: "observed_failure",
		result: types.ToolResult{
			ToolName:  "observed_failure",
			Success:   false,
			Timestamp: time.Now(),
		},
		err: errors.New("boom"),
	})
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{
		Tools:             reg,
		ReasoningObserver: collector,
	}, nil)
	ctx := &types.AgentContext{AgentName: types.AgentExplorer, Stage: types.StageExplore}

	res, _ := base.executeTool(ctx, llm.ToolCall{
		ID:     "call-observed-failure",
		Name:   "observed_failure",
		Params: json.RawMessage(`{}`),
	})
	if res == nil || res.Success {
		t.Fatalf("expected failed ToolResult, got %+v", res)
	}

	view := collector.View()
	if len(view.ToolEvents) != 1 {
		t.Fatalf("tool events=%+v", view.ToolEvents)
	}
	ev := view.ToolEvents[0]
	if ev.Kind != reasoninggraph.ReasoningEventToolCallObserved ||
		ev.ReasonCode != "tool_call_failed" ||
		ev.ActionStatus != "failure" ||
		ev.ViolationKind != "tool_execution_error" ||
		ev.ToolName != "observed_failure" ||
		ev.ToolResultCount != 1 ||
		ev.ElapsedMillis < 0 {
		t.Fatalf("unexpected observed failure event: %+v", ev)
	}
}

func TestReasoningObserverProjectsToolRuntimeTimings(t *testing.T) {
	collector := reasoninggraph.NewEventCollector("graph-agent")
	reg := toolpkg.NewRegistry()
	reg.Register(&observedRuntimeTool{
		name: "observed_phases",
		result: types.ToolResult{
			ToolName: "observed_phases",
			Success:  true,
			RuntimeTimings: []types.ToolRuntimeTiming{{
				Phase:         "ground_context",
				ElapsedMillis: 12,
				Status:        "success",
				Count:         3,
			}},
			Timestamp: time.Now(),
		},
	})
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{
		Tools:             reg,
		ReasoningObserver: collector,
	}, nil)
	ctx := &types.AgentContext{AgentName: types.AgentExplorer, Stage: types.StageExplore}

	res, _ := base.executeTool(ctx, llm.ToolCall{
		ID:     "call-observed-phases",
		Name:   "observed_phases",
		Params: json.RawMessage(`{}`),
	})
	if res == nil || !res.Success {
		t.Fatalf("expected successful ToolResult, got %+v", res)
	}

	view := collector.View()
	if len(view.ToolEvents) != 2 {
		t.Fatalf("tool events=%+v", view.ToolEvents)
	}
	phase := view.ToolEvents[1]
	if phase.Kind != reasoninggraph.ReasoningEventToolCallObserved ||
		phase.ReasonCode != "tool_phase_observed" ||
		phase.ToolName != "observed_phases" ||
		phase.ToolPhase != "ground_context" ||
		phase.ElapsedMillis != 12 ||
		phase.ActionStatus != "success" ||
		phase.ResultCount != 3 {
		t.Fatalf("unexpected phase event: %+v", phase)
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

func TestObserveToolHandoffCarrierProjected(t *testing.T) {
	collector := reasoninggraph.NewEventCollector("test")
	base := &BaseAgent{
		name: types.AgentExplorer,
		deps: &Dependencies{ReasoningObserver: collector},
	}
	result := &types.ToolResult{
		ToolName: "emit_change_plan",
		Success:  false,
		Handoff: &types.ToolHandoffCarrier{
			Version:    types.ToolHandoffCarrierVersion,
			ToolName:   "emit_change_plan",
			ReasonCode: "invalid_enum",
			RepairCode: types.PlanRepairToolCode,
			SupportedJSON: &types.ToolJSONSurfaceDescriptor{
				ToolName:          "emit_change_plan",
				ReasonCode:        "invalid_enum",
				FailingFieldPaths: []string{"$.changes[0].edits[0].kind"},
				AcceptedEnums: map[string][]string{
					"$.changes[].edits[].kind": {"replace"},
				},
			},
			AcceptedEvidence: []types.AcceptedEvidenceRef{{ID: "ev-1"}},
			ObservationRefs:  []types.ToolObservationRef{{ID: "obs-1"}},
		},
	}

	base.observeToolRepairPackEmitted(&types.AgentContext{
		AgentName: types.AgentExplorer,
		Stage:     types.StageExplore,
	}, result)

	events := collector.Events()
	if len(events) != 1 {
		t.Fatalf("events = %+v, want one handoff projection", events)
	}
	event := events[0]
	if event.Kind != reasoninggraph.ReasoningEventToolHandoffProjected {
		t.Fatalf("event kind = %s", event.Kind)
	}
	payload := reasoninggraphPayload(t, event)
	if payload.RepairReasonCode != "invalid_enum" ||
		payload.RepairCode != types.PlanRepairToolCode ||
		payload.EvidenceRefCount != 1 ||
		payload.FactCount != 1 ||
		len(payload.JSONFieldPaths) != 1 ||
		len(payload.EnumFieldPaths) != 1 ||
		len(payload.AcceptedEvidenceIDs) != 1 ||
		len(payload.ObservationIDs) != 1 {
		t.Fatalf("payload mismatch: %+v", payload)
	}
}

func reasoninggraphPayload(t *testing.T, event reasoninggraph.ReasoningEvent) reasoninggraph.ObservationPayload {
	t.Helper()
	payload := reasoninggraph.ObservationPayload{}
	if len(event.Payload) == 0 {
		t.Fatal("missing event payload")
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	return payload
}
