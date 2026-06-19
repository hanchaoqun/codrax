package reasoninggraph

import "sort"

func ReduceEvents(events []ReasoningEvent) ReasoningGraphView {
	events = NormalizeEvents(events)
	view := ReasoningGraphView{EventCount: len(events)}
	nodes := map[string]ReasoningNodeView{}
	counts := map[ReasoningEventKind]int{}
	for _, event := range events {
		if view.GraphID == "" && event.GraphID != "" {
			view.GraphID = event.GraphID
		}
		if event.GraphID != "" {
			view.GraphID = event.GraphID
		}
		view.LastEventKind = event.Kind
		view.LastReasonCode = event.ReasonCode
		counts[event.Kind]++
		if event.NodeID != "" {
			node := nodes[event.NodeID]
			node.NodeID = event.NodeID
			if event.NodeKind != "" {
				node.Kind = event.NodeKind
			} else if node.Kind == "" {
				node.Kind = nodeKindForEvent(event.Kind)
			}
			node.LastEventKind = event.Kind
			node.LastReasonCode = event.ReasonCode
			node.UpdatedAt = event.At
			nodes[event.NodeID] = node
		}
		view.EvidenceRefs = append(view.EvidenceRefs, event.EvidenceRefs...)
		summary := eventSummary(event)
		switch {
		case IsToolObservationKind(event.Kind):
			view.ToolEvents = append(view.ToolEvents, summary)
		case IsRepairObservationKind(event.Kind):
			view.RepairEvents = append(view.RepairEvents, summary)
		case IsLLMObservationKind(event.Kind):
			view.LLMEvents = append(view.LLMEvents, summary)
		}
	}
	view.Nodes = sortedNodeViews(nodes)
	view.EvidenceRefs = NormalizeEvidenceRefs(view.EvidenceRefs)
	view.EventKindCounts = eventKindCounts(counts)
	return view
}

func nodeKindForEvent(kind ReasoningEventKind) ReasoningNodeKind {
	switch {
	case IsToolObservationKind(kind):
		return ReasoningNodeTool
	case IsRepairObservationKind(kind):
		return ReasoningNodeRepair
	case IsLLMObservationKind(kind):
		return ReasoningNodeLLM
	default:
		return ""
	}
}

func eventSummary(event ReasoningEvent) ReasoningEventSummary {
	payload := observationPayloadFromEvent(event)
	return ReasoningEventSummary{
		EventID:       event.ID,
		NodeID:        event.NodeID,
		Kind:          event.Kind,
		ReasonCode:    event.ReasonCode,
		ToolName:      payload.ToolName,
		Agent:         payload.Agent,
		Stage:         payload.Stage,
		Model:         payload.Model,
		Attempt:       payload.Attempt,
		MaxAttempts:   payload.MaxAttempts,
		ElapsedMillis: payload.ElapsedMillis,
		RepairCode:    firstNonEmpty(payload.RepairCode, payload.RepairReasonCode),
		ViolationKind: payload.ViolationKind,
		RepairLocus:   payload.RepairLocus,
		At:            event.At,
	}
}

func sortedNodeViews(nodes map[string]ReasoningNodeView) []ReasoningNodeView {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]ReasoningNodeView, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, node)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].NodeID < out[j].NodeID
	})
	return out
}

func eventKindCounts(counts map[ReasoningEventKind]int) []EventKindCount {
	if len(counts) == 0 {
		return nil
	}
	kinds := make([]ReasoningEventKind, 0, len(counts))
	for kind := range counts {
		if kind == "" {
			continue
		}
		kinds = append(kinds, kind)
	}
	sort.SliceStable(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })
	out := make([]EventKindCount, 0, len(kinds))
	for _, kind := range kinds {
		out = append(out, EventKindCount{Kind: kind, Count: counts[kind]})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
