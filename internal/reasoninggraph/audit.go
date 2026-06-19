package reasoninggraph

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const defaultAuditEventLimit = 5

func AuditSummaryFromEvents(events []ReasoningEvent, source string, limit int) *types.ReasoningGraphAuditSummary {
	return AuditSummaryFromView(ReduceEvents(events), source, limit)
}

func AuditSummaryFromView(view ReasoningGraphView, source string, limit int) *types.ReasoningGraphAuditSummary {
	if limit <= 0 {
		limit = defaultAuditEventLimit
	}
	out := types.ReasoningGraphAuditSummary{
		Source:           strings.TrimSpace(source),
		Status:           types.ReasoningGraphAuditObserved,
		ReasonCode:       "reasoning_graph_view",
		GraphID:          strings.TrimSpace(view.GraphID),
		EventCount:       view.EventCount,
		NodeCount:        len(view.Nodes),
		EvidenceRefCount: len(view.EvidenceRefs),
		LastEventKind:    string(view.LastEventKind),
		LastReasonCode:   strings.TrimSpace(view.LastReasonCode),
		Lanes: []types.ReasoningGraphAuditLane{
			{Name: "workflow", Count: len(view.WorkflowEvents)},
			{Name: "read", Count: len(view.ReadEvents)},
			{Name: "tool", Count: len(view.ToolEvents)},
			{Name: "repair", Count: len(view.RepairEvents)},
			{Name: "llm", Count: len(view.LLMEvents)},
		},
		RecentEvents: auditEventsFromSummaries(recentReasoningEvents(view, limit)),
		RepairEvents: auditEventsFromSummaries(limitReasoningEventSummaries(view.RepairEvents, limit)),
		LLMEvents:    auditEventsFromSummaries(limitReasoningEventSummaries(view.LLMEvents, limit)),
	}
	if out.Source == "" {
		out.Source = "reasoning_graph_view"
	}
	if out.EventCount == 0 {
		out.Missing = append(out.Missing, types.ReasoningGraphAuditGap{
			Code:     "reasoning_graph_no_events",
			Severity: "warning",
			Detail:   "No typed reasoning graph events were available for this artifact.",
		})
	}
	return types.NormalizeReasoningGraphAuditSummaryPtr(&out)
}

func recentReasoningEvents(view ReasoningGraphView, limit int) []ReasoningEventSummary {
	all := make([]ReasoningEventSummary, 0,
		len(view.WorkflowEvents)+len(view.ReadEvents)+len(view.ToolEvents)+len(view.RepairEvents)+len(view.LLMEvents))
	all = append(all, view.WorkflowEvents...)
	all = append(all, view.ReadEvents...)
	all = append(all, view.ToolEvents...)
	all = append(all, view.RepairEvents...)
	all = append(all, view.LLMEvents...)
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].At.Equal(all[j].At) {
			if all[i].At.IsZero() {
				return false
			}
			if all[j].At.IsZero() {
				return true
			}
			return all[i].At.Before(all[j].At)
		}
		return all[i].EventID < all[j].EventID
	})
	if len(all) <= limit {
		return all
	}
	return all[len(all)-limit:]
}

func limitReasoningEventSummaries(events []ReasoningEventSummary, limit int) []ReasoningEventSummary {
	if limit <= 0 || len(events) <= limit {
		return events
	}
	return events[len(events)-limit:]
}

func auditEventsFromSummaries(events []ReasoningEventSummary) []types.ReasoningGraphAuditEvent {
	out := make([]types.ReasoningGraphAuditEvent, 0, len(events))
	for _, event := range events {
		out = append(out, types.ReasoningGraphAuditEvent{
			EventID:       event.EventID,
			NodeID:        event.NodeID,
			Kind:          string(event.Kind),
			ReasonCode:    event.ReasonCode,
			ToolName:      event.ToolName,
			Agent:         event.Agent,
			Stage:         event.Stage,
			Model:         event.Model,
			Attempt:       event.Attempt,
			MaxAttempts:   event.MaxAttempts,
			ElapsedMillis: event.ElapsedMillis,
			RepairCode:    event.RepairCode,
			ViolationKind: event.ViolationKind,
			RepairLocus:   event.RepairLocus,
		})
	}
	return out
}
