package reasoninggraph

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const DefaultWriteFinalReasoningGraphEventRefLimit = 128

func WriteFinalReasoningGraphSummaryFromRun(run *types.WriteWorkflowRun, limit int) *types.WriteFinalReasoningGraphSummary {
	if run == nil {
		return nil
	}
	events := EventsFromWriteWorkflowRun(*run)
	if len(events) == 0 {
		return nil
	}
	view := ReduceEvents(events)
	out := &types.WriteFinalReasoningGraphSummary{
		GraphID:            strings.TrimSpace(view.GraphID),
		EventCount:         view.EventCount,
		LastEventKind:      string(view.LastEventKind),
		LastReasonCode:     strings.TrimSpace(view.LastReasonCode),
		EventRefs:          writeFinalReasoningGraphEventRefs(events, limit),
		WorkflowEventCount: len(view.WorkflowEvents),
		ToolEventCount:     len(view.ToolEvents),
		RepairEventCount:   len(view.RepairEvents),
		LLMEventCount:      len(view.LLMEvents),
		NodeCount:          len(view.Nodes),
	}
	normalized := types.NormalizeWriteFinalReport(types.WriteFinalReport{ReasoningGraph: out})
	return normalized.ReasoningGraph
}

func writeFinalReasoningGraphEventRefs(events []ReasoningEvent, limit int) []string {
	if limit <= 0 {
		limit = len(events)
	}
	refs := make([]string, 0, len(events))
	seen := map[string]bool{}
	for _, event := range events {
		ref := strings.TrimSpace(event.ID)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		refs = append(refs, ref)
		if len(refs) >= limit {
			break
		}
	}
	return refs
}
