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

func (b *BaseAgent) observeToolCallObserved(ctx *types.AgentContext, call llm.ToolCall, elapsed time.Duration, result *types.ToolResult, resp *types.MCPResponse, execErr error) {
	payload := reasoninggraph.ObservationPayload{
		ToolName:      call.Name,
		ElapsedMillis: elapsed.Milliseconds(),
	}
	if result != nil {
		payload.ToolResultCount = 1
		payload.FactCount += len(result.Observations)
		if strings.TrimSpace(result.RawRef) != "" {
			payload.OutputRefCount++
			payload.PayloadRef = strings.TrimSpace(result.RawRef)
		}
	}
	if resp != nil {
		payload.MCPResponseCount = 1
		payload.FactCount += len(resp.Observations)
		payload.ResourceURI = strings.TrimSpace(resp.ResourceURI)
		for _, ref := range []string{resp.PayloadRef, resp.RawRef, resp.RowSetRef, resp.PageRef} {
			if strings.TrimSpace(ref) == "" {
				continue
			}
			payload.OutputRefCount++
			if payload.PayloadRef == "" {
				payload.PayloadRef = strings.TrimSpace(ref)
			}
		}
	}
	reasonCode := "tool_call_ok"
	payload.ActionStatus = "success"
	if violationKind := observedToolFailureKind(result, resp, execErr); violationKind != "" {
		reasonCode = "tool_call_failed"
		payload.ActionStatus = "failure"
		payload.ViolationKind = violationKind
	}
	b.observeReasoningObservation(ctx, reasoninggraph.ReasoningEventToolCallObserved, reasonCode, reasoninggraph.ReasoningNodeTool, payload)
	if result != nil {
		b.observeToolRuntimeTimings(ctx, call, result.RuntimeTimings)
	}
}

func (b *BaseAgent) observeToolRuntimeTimings(ctx *types.AgentContext, call llm.ToolCall, timings []types.ToolRuntimeTiming) {
	for _, timing := range timings {
		phase := strings.TrimSpace(timing.Phase)
		if phase == "" {
			continue
		}
		status := strings.TrimSpace(timing.Status)
		if status == "" {
			status = "success"
		}
		elapsed := timing.ElapsedMillis
		if elapsed < 0 {
			elapsed = 0
		}
		count := timing.Count
		if count < 0 {
			count = 0
		}
		reasonCode := "tool_phase_observed"
		violationKind := ""
		if status != "success" {
			reasonCode = "tool_phase_failed"
			violationKind = "tool_phase_failed"
		}
		b.observeReasoningObservation(ctx, reasoninggraph.ReasoningEventToolCallObserved, reasonCode, reasoninggraph.ReasoningNodeTool, reasoninggraph.ObservationPayload{
			ToolName:      call.Name,
			ToolPhase:     phase,
			ElapsedMillis: elapsed,
			ActionStatus:  status,
			ResultCount:   count,
			ViolationKind: violationKind,
		})
	}
}

func observedToolFailureKind(result *types.ToolResult, resp *types.MCPResponse, execErr error) string {
	if execErr != nil {
		return "tool_execution_error"
	}
	if result != nil && !result.Success {
		return "tool_result_failed"
	}
	if resp != nil && !resp.Success {
		return "mcp_response_failed"
	}
	return ""
}

func (b *BaseAgent) observeSchemaRejected(ctx *types.AgentContext, call llm.ToolCall, reasonCode, violationKind string) {
	b.observeReasoningObservation(ctx, reasoninggraph.ReasoningEventSchemaRejected, reasonCode, reasoninggraph.ReasoningNodeRepair, reasoninggraph.ObservationPayload{
		ToolName:      call.Name,
		ViolationKind: violationKind,
		RepairLocus:   "tool_arguments",
	})
}

func (b *BaseAgent) observeToolRepairPackEmitted(ctx *types.AgentContext, result *types.ToolResult) {
	if result == nil {
		return
	}
	if result.Repair != nil && strings.TrimSpace(result.Repair.Code) != "" {
		payload := reasoninggraph.ObservationPayload{
			ToolName:    result.ToolName,
			RepairCode:  result.Repair.Code,
			RepairLocus: "tool_result",
			Message:     strings.Join(result.Repair.Fields, ","),
		}
		b.observeReasoningObservation(ctx, reasoninggraph.ReasoningEventRepairPackEmitted, result.Repair.Code, reasoninggraph.ReasoningNodeRepair, payload)
	}
	if result.Handoff != nil {
		b.observeToolHandoffCarrierProjected(ctx, result)
	}
}

func (b *BaseAgent) observeToolHandoffCarrierProjected(ctx *types.AgentContext, result *types.ToolResult) {
	if result == nil || result.Handoff == nil {
		return
	}
	carrier := types.NormalizeToolHandoffCarrier(*result.Handoff)
	if carrier.Empty() {
		return
	}
	payload := reasoninggraph.ObservationPayload{
		ToolName:            result.ToolName,
		RepairCode:          carrier.RepairCode,
		RepairReasonCode:    carrier.ReasonCode,
		RepairLocus:         "tool_handoff_carrier",
		EvidenceRefCount:    len(carrier.AcceptedEvidence),
		FactCount:           len(carrier.ObservationRefs),
		JSONFieldPaths:      toolHandoffJSONFieldPaths(carrier),
		EnumFieldPaths:      toolHandoffEnumFieldPaths(carrier),
		AcceptedEvidenceIDs: toolHandoffAcceptedEvidenceIDs(carrier),
		ObservationIDs:      toolHandoffObservationIDs(carrier),
	}
	if payload.RepairCode == "" && carrier.Repair != nil {
		payload.RepairCode = carrier.Repair.Code
	}
	reason := firstNonEmptyString(carrier.ReasonCode, carrier.RepairCode, "tool_handoff_projected")
	b.observeReasoningObservation(ctx, reasoninggraph.ReasoningEventToolHandoffProjected, reason, reasoninggraph.ReasoningNodeRepair, payload)
}

func (b *BaseAgent) observeLLMRequestWaiting(ctx *types.AgentContext, iter int, telemetry llm.RequestTelemetry, elapsed time.Duration) {
	b.observeReasoningObservation(ctx, reasoninggraph.ReasoningEventLLMRequestWaiting, "llm_request_waiting", reasoninggraph.ReasoningNodeLLM, reasoninggraph.ObservationPayload{
		Model:         telemetry.ModelID,
		Attempt:       iter + 1,
		ElapsedMillis: elapsed.Milliseconds(),
	})
}

func toolHandoffJSONFieldPaths(carrier types.ToolHandoffCarrier) []string {
	if carrier.SupportedJSON == nil {
		return nil
	}
	return append([]string(nil), carrier.SupportedJSON.FailingFieldPaths...)
}

func toolHandoffEnumFieldPaths(carrier types.ToolHandoffCarrier) []string {
	if carrier.SupportedJSON == nil || len(carrier.SupportedJSON.AcceptedEnums) == 0 {
		return nil
	}
	out := make([]string, 0, len(carrier.SupportedJSON.AcceptedEnums))
	for key := range carrier.SupportedJSON.AcceptedEnums {
		out = append(out, key)
	}
	return out
}

func toolHandoffAcceptedEvidenceIDs(carrier types.ToolHandoffCarrier) []string {
	out := make([]string, 0, len(carrier.AcceptedEvidence))
	for _, ref := range carrier.AcceptedEvidence {
		if ref.ID != "" {
			out = append(out, ref.ID)
		}
	}
	return out
}

func toolHandoffObservationIDs(carrier types.ToolHandoffCarrier) []string {
	out := make([]string, 0, len(carrier.ObservationRefs))
	for _, ref := range carrier.ObservationRefs {
		if ref.ID != "" {
			out = append(out, ref.ID)
		}
	}
	return out
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
