package agent

import (
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

const explorerCompletionNarrativeOwnership = "Completion narrative ownership for emit_investigation_complete: use native_fact_handoff.interpretation when the current completion schema requires that handoff or you select it; otherwise use reason. This rule also applies after later tool results change the available facts. With the separated handoff, reason is lifecycle rationale only; preserve independent diagnoses, causal projections, source/business clues and missing-evidence boundaries in interpretation."

// Completion navigation and schema share current structured availability.
// This changes teaching only, never findings, evidence or stopping authority.
func explorerCompletionNarrativeGuidance(ctx *types.AgentContext, legacy string) string {
	if current := tool.NativeFactHandoffCompletionTeaching(types.ToolBusContext(ctx, types.AgentExplorer)); current != "" {
		return current
	}
	if legacy == "" {
		return ""
	}
	return "While native_fact_handoff is neither required nor selected: " + legacy
}

func explorerCompletionNarrativeField(ctx *types.AgentContext) string {
	bus := types.ToolBusContext(ctx, types.AgentExplorer)
	if tool.ManagedDeliveryRequiresNativeFactHandoff(bus) {
		return "`native_fact_handoff.interpretation`"
	}
	if len(tool.NativeFactHandoffChoices(bus)) > 0 {
		return "the completion narrative (`native_fact_handoff.interpretation` when selected, otherwise `reason`)"
	}
	return "the completion narrative"
}

func firstExplorerCompletionContext(contexts []*types.AgentContext) *types.AgentContext {
	if len(contexts) > 0 {
		return contexts[0]
	}
	return nil
}
