package reasoninggraph

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worker"
)

func EventsFromWorkerRequestResult(req worker.Request, result *worker.Result) []ReasoningEvent {
	req = worker.NormalizeRequest(req)
	graphID := workerProjectionGraphID(req, result)
	nodeID := workerProjectionNodeID(req.ID)
	permission := worker.RequestPermissionDecision(req)
	events := []ReasoningEvent{NormalizeEvent(ReasoningEvent{
		ID:           workerProjectionEventID(graphID, nodeID, "requested"),
		GraphID:      graphID,
		NodeID:       nodeID,
		NodeKind:     ReasoningNodeWorker,
		Sequence:     1,
		Kind:         ReasoningEventWorkerRequested,
		ReasonCode:   firstNonEmpty(permission.ReasonCode, "worker_requested"),
		ArtifactRefs: append([]loopkernel.ArtifactRef(nil), req.InputRefs...),
		EvidenceRefs: evidenceRefsFromIDs(req.Scope.EvidenceRefs, nodeID, "worker_scope", "p0", "high", "worker_request", "worker", "worker_scope_evidence"),
		Payload: eventPayload(ObservationPayload{
			WorkerKind:       string(req.Kind),
			WorkerRole:       string(req.Role),
			PermissionAction: string(permission.Action),
			PermissionReason: permission.ReasonCode,
			ScopePathCount:   len(req.Scope.Paths),
			InputRefCount:    len(req.InputRefs),
			EvidenceRefCount: len(req.Scope.EvidenceRefs),
		}),
		At: req.RequestedAt,
	})}
	if result == nil {
		return NormalizeEvents(events)
	}
	res := worker.NormalizeResult(*result)
	events = append(events, NormalizeEvent(ReasoningEvent{
		ID:           workerProjectionEventID(graphID, nodeID, "completed"),
		GraphID:      graphID,
		NodeID:       nodeID,
		NodeKind:     ReasoningNodeWorker,
		Sequence:     2,
		Kind:         ReasoningEventWorkerCompleted,
		ReasonCode:   firstNonEmpty(res.ReasonCode, string(res.Status), "worker_completed"),
		ArtifactRefs: append([]loopkernel.ArtifactRef(nil), res.OutputRefs...),
		EvidenceRefs: evidenceRefsFromIDs(res.EvidenceRefs, nodeID, "worker_result", "p1", workerStatusConfidence(res.Status), "worker_result", "controller", firstNonEmpty(res.ReasonCode, "worker_result_evidence")),
		Payload: eventPayload(ObservationPayload{
			WorkerKind:       string(res.Kind),
			WorkerStatus:     string(res.Status),
			WorkerRole:       string(res.Role),
			OutputRefCount:   len(res.OutputRefs),
			EvidenceRefCount: len(res.EvidenceRefs),
		}),
		At: res.CompletedAt,
	}))
	return NormalizeEvents(events)
}

func EventsFromSubAgentRequestResult(req *types.SubAgentRequest, result *types.SubAgentResult) []ReasoningEvent {
	if req == nil {
		return nil
	}
	graphID := subAgentProjectionGraphID(req, result)
	nodeID := subAgentProjectionNodeID(req.ID)
	events := []ReasoningEvent{NormalizeEvent(ReasoningEvent{
		ID:         workerProjectionEventID(graphID, nodeID, "requested"),
		GraphID:    graphID,
		NodeID:     nodeID,
		NodeKind:   ReasoningNodeSubAgent,
		Sequence:   1,
		Kind:       ReasoningEventSubAgentRequested,
		ReasonCode: "subagent_requested",
		Payload: eventPayload(ObservationPayload{
			SubAgentName:   strings.TrimSpace(req.SubAgent),
			ScopePathCount: len(req.Scope),
		}),
	})}
	if result == nil {
		return NormalizeEvents(events)
	}
	reason := "subagent_completed"
	if strings.TrimSpace(result.Error) != "" {
		reason = "subagent_failed"
	}
	evidenceIDs := subAgentEvidenceIDs(result)
	events = append(events, NormalizeEvent(ReasoningEvent{
		ID:           workerProjectionEventID(graphID, nodeID, "completed"),
		GraphID:      graphID,
		NodeID:       nodeID,
		NodeKind:     ReasoningNodeSubAgent,
		Sequence:     2,
		Kind:         ReasoningEventSubAgentCompleted,
		ReasonCode:   reason,
		ArtifactRefs: subAgentArtifactRefs(result),
		EvidenceRefs: evidenceRefsFromIDs(evidenceIDs, nodeID, "subagent_result", "p1", subAgentResultConfidence(result), "subagent_result", "extractor", reason),
		Payload: eventPayload(ObservationPayload{
			SubAgentName:     strings.TrimSpace(result.SubAgent),
			EvidenceRefCount: len(evidenceIDs),
			ToolResultCount:  len(result.Tools),
			FactCount:        len(result.Facts),
			FlowFindingCount: len(result.FlowFindings),
			MCPResponseCount: len(result.MCPResps),
		}),
	}))
	return NormalizeEvents(events)
}

func workerProjectionGraphID(req worker.Request, result *worker.Result) string {
	if req.RunID != "" {
		return req.RunID
	}
	if result != nil {
		res := worker.NormalizeResult(*result)
		if res.RunID != "" {
			return res.RunID
		}
	}
	return "worker:" + firstNonEmpty(req.ID, string(req.Kind), "unknown")
}

func subAgentProjectionGraphID(req *types.SubAgentRequest, result *types.SubAgentResult) string {
	if req != nil && req.ReadView != nil && strings.TrimSpace(req.ReadView.TraceID) != "" {
		return strings.TrimSpace(req.ReadView.TraceID)
	}
	if req != nil && strings.TrimSpace(req.ID) != "" {
		return "subagent:" + strings.TrimSpace(req.ID)
	}
	if result != nil && strings.TrimSpace(result.RequestID) != "" {
		return "subagent:" + strings.TrimSpace(result.RequestID)
	}
	return "subagent:unknown"
}

func workerProjectionNodeID(id string) string {
	return "worker:" + firstNonEmpty(strings.TrimSpace(id), "unknown")
}

func subAgentProjectionNodeID(id string) string {
	return "subagent:" + firstNonEmpty(strings.TrimSpace(id), "unknown")
}

func workerProjectionEventID(graphID, nodeID, suffix string) string {
	parts := []string{strings.TrimSpace(graphID), strings.TrimSpace(nodeID), strings.TrimSpace(suffix)}
	for i, part := range parts {
		parts[i] = strings.ReplaceAll(part, " ", "_")
	}
	return strings.Join(parts, ":")
}

func evidenceRefsFromIDs(ids []string, nodeID, kind, priority, confidence, sourceStage, consumer, reasonCode string) []ReasoningEvidenceRef {
	refs := make([]ReasoningEvidenceRef, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		refs = append(refs, ReasoningEvidenceRef{
			ID:          id,
			Kind:        kind,
			NodeID:      nodeID,
			Priority:    priority,
			Confidence:  confidence,
			SourceStage: sourceStage,
			Consumer:    consumer,
			ReasonCode:  reasonCode,
		})
	}
	return refs
}

func workerStatusConfidence(status worker.Status) string {
	switch worker.NormalizeStatus(status) {
	case worker.StatusComplete:
		return "high"
	case worker.StatusBlocked:
		return "unverified"
	case worker.StatusFailed:
		return "low"
	default:
		return "medium"
	}
}

func subAgentResultConfidence(result *types.SubAgentResult) string {
	if result == nil {
		return "unverified"
	}
	if strings.TrimSpace(result.Error) != "" {
		return "low"
	}
	return "medium"
}

func subAgentEvidenceIDs(result *types.SubAgentResult) []string {
	if result == nil {
		return nil
	}
	var ids []string
	for _, item := range result.EvidenceItems {
		ids = append(ids, firstNonEmpty(item.EvidenceRef, item.ID))
	}
	for _, fact := range result.Facts {
		ids = append(ids, fact.EvidenceRef)
	}
	for _, flow := range result.FlowFindings {
		ids = append(ids, flow.EvidenceIDs...)
	}
	return dedupTrimStrings(ids)
}

func subAgentArtifactRefs(result *types.SubAgentResult) []loopkernel.ArtifactRef {
	if result == nil {
		return nil
	}
	var refs []loopkernel.ArtifactRef
	for _, tool := range result.Tools {
		if strings.TrimSpace(tool.RawRef) != "" {
			refs = append(refs, loopkernel.ArtifactRef{Kind: "tool_raw", ID: tool.RawRef})
		}
	}
	for _, resp := range result.MCPResps {
		if strings.TrimSpace(resp.PayloadRef) != "" {
			refs = append(refs, loopkernel.ArtifactRef{Kind: "mcp_payload", ID: resp.PayloadRef})
		}
		if strings.TrimSpace(resp.RowSetRef) != "" {
			refs = append(refs, loopkernel.ArtifactRef{Kind: "mcp_rowset", ID: resp.RowSetRef})
		}
		if strings.TrimSpace(resp.PageRef) != "" {
			refs = append(refs, loopkernel.ArtifactRef{Kind: "mcp_page", ID: resp.PageRef})
		}
	}
	return refs
}

func dedupTrimStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
