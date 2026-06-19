package agent

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/reasoninggraph"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (b *BaseAgent) observeReasoningObservation(ctx *types.AgentContext, kind reasoninggraph.ReasoningEventKind, reasonCode string, nodeKind reasoninggraph.ReasoningNodeKind, payload reasoninggraph.ObservationPayload) {
	if b == nil || b.deps == nil || b.deps.ReasoningObserver == nil || kind == "" {
		return
	}
	agentName := string(b.name)
	stage := ""
	if ctx != nil {
		if ctx.AgentName != "" {
			agentName = string(ctx.AgentName)
		}
		stage = string(ctx.Stage)
	}
	if payload.Agent == "" {
		payload.Agent = agentName
	}
	if payload.Stage == "" {
		payload.Stage = stage
	}
	nodeID := reasoningNodeID(nodeKind, payload)
	b.deps.ReasoningObserver.ObserveReasoningEvent(reasoninggraph.NewObservationEvent(reasoninggraph.ObservationInput{
		NodeID:     nodeID,
		NodeKind:   nodeKind,
		Kind:       kind,
		ReasonCode: strings.TrimSpace(reasonCode),
		Payload:    payload,
		At:         time.Now(),
	}))
}

func (b *BaseAgent) observeToolParamsRecovered(ctx *types.AgentContext, call llm.ToolCall, before, after json.RawMessage) {
	b.observeReasoningObservation(ctx, reasoninggraph.ReasoningEventStructuredPayloadRecovered, "tool_params_json_recovered", reasoninggraph.ReasoningNodeRepair, reasoninggraph.ObservationPayload{
		ToolName:          call.Name,
		RepairCode:        "tool_params_json_recovered",
		ViolationKind:     "malformed_json",
		RepairLocus:       "tool_arguments",
		OriginalByteLen:   len(before),
		NormalizedByteLen: len(after),
	})
}

func (b *BaseAgent) observeToolParamsNormalized(ctx *types.AgentContext, call llm.ToolCall, before, after json.RawMessage, reasonCode string) {
	b.observeReasoningObservation(ctx, reasoninggraph.ReasoningEventToolParamNormalized, reasonCode, reasoninggraph.ReasoningNodeTool, reasoninggraph.ObservationPayload{
		ToolName:          call.Name,
		RepairCode:        reasonCode,
		RepairLocus:       "tool_arguments",
		OriginalByteLen:   len(before),
		NormalizedByteLen: len(after),
	})
}

func (b *BaseAgent) observeToolRejected(ctx *types.AgentContext, call llm.ToolCall, reasonCode, violationKind string) {
	b.observeReasoningObservation(ctx, reasoninggraph.ReasoningEventToolCallRejected, reasonCode, reasoninggraph.ReasoningNodeTool, reasoninggraph.ObservationPayload{
		ToolName:      call.Name,
		ViolationKind: violationKind,
		RepairLocus:   "tool_call",
	})
}

func (b *BaseAgent) observeSchemaRejected(ctx *types.AgentContext, call llm.ToolCall, reasonCode, violationKind string) {
	b.observeReasoningObservation(ctx, reasoninggraph.ReasoningEventSchemaRejected, reasonCode, reasoninggraph.ReasoningNodeRepair, reasoninggraph.ObservationPayload{
		ToolName:      call.Name,
		ViolationKind: violationKind,
		RepairLocus:   "tool_arguments",
	})
}

func (b *BaseAgent) observeToolRepairPackEmitted(ctx *types.AgentContext, result *types.ToolResult) {
	if result == nil || result.Repair == nil || strings.TrimSpace(result.Repair.Code) == "" {
		return
	}
	payload := reasoninggraph.ObservationPayload{
		ToolName:    result.ToolName,
		RepairCode:  result.Repair.Code,
		RepairLocus: "tool_result",
		Message:     strings.Join(result.Repair.Fields, ","),
	}
	b.observeReasoningObservation(ctx, reasoninggraph.ReasoningEventRepairPackEmitted, result.Repair.Code, reasoninggraph.ReasoningNodeRepair, payload)
}

func (b *BaseAgent) observeLLMRequestWaiting(ctx *types.AgentContext, iter int, telemetry llm.RequestTelemetry, elapsed time.Duration) {
	b.observeReasoningObservation(ctx, reasoninggraph.ReasoningEventLLMRequestWaiting, "llm_request_waiting", reasoninggraph.ReasoningNodeLLM, reasoninggraph.ObservationPayload{
		Model:         telemetry.ModelID,
		Attempt:       iter + 1,
		ElapsedMillis: elapsed.Milliseconds(),
	})
}

func (b *BaseAgent) observeLLMRequestRetried(ctx *types.AgentContext, attempt int, delay time.Duration, reason string) {
	b.observeReasoningObservation(ctx, reasoninggraph.ReasoningEventLLMRequestRetried, "llm_request_retried", reasoninggraph.ReasoningNodeLLM, reasoninggraph.ObservationPayload{
		Attempt:       attempt,
		ElapsedMillis: delay.Milliseconds(),
		Message:       reason,
	})
}

func (b *BaseAgent) observeLLMFallbackRouted(ctx *types.AgentContext, from, to, reason string) {
	b.observeReasoningObservation(ctx, reasoninggraph.ReasoningEventFallbackRouted, "llm_fallback_routed", reasoninggraph.ReasoningNodeLLM, reasoninggraph.ObservationPayload{
		Model:          from,
		FallbackTarget: to,
		Message:        reason,
	})
}

func reasoningNodeID(kind reasoninggraph.ReasoningNodeKind, payload reasoninggraph.ObservationPayload) string {
	parts := []string{string(kind)}
	if payload.Agent != "" {
		parts = append(parts, payload.Agent)
	}
	if payload.Stage != "" {
		parts = append(parts, payload.Stage)
	}
	if payload.ToolName != "" {
		parts = append(parts, payload.ToolName)
	}
	if payload.Model != "" && payload.ToolName == "" {
		parts = append(parts, payload.Model)
	}
	return strings.Join(parts, ":")
}
