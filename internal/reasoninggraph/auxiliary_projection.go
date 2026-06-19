package reasoninggraph

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

func EventsFromToolResultObservations(graphID, nodeID string, result types.ToolResult) []ReasoningEvent {
	graphID = strings.TrimSpace(graphID)
	nodeID = firstNonEmpty(strings.TrimSpace(nodeID), "tool:"+strings.TrimSpace(result.ToolName), "tool:unknown")
	events := make([]ReasoningEvent, 0, len(result.Observations))
	for i, record := range result.Observations {
		record = normalizeObservationRecordForGraph(record, i)
		if record.ID == "" {
			continue
		}
		events = append(events, eventFromObservationRecord(graphID, nodeID, record, int64(i+1), "tool_result_observation"))
	}
	return NormalizeEvents(events)
}

func EventsFromMCPResponseObservations(graphID, nodeID string, resp types.MCPResponse) []ReasoningEvent {
	graphID = strings.TrimSpace(graphID)
	nodeID = firstNonEmpty(strings.TrimSpace(nodeID), "mcp:"+strings.TrimSpace(resp.ServerName), "mcp:unknown")
	events := make([]ReasoningEvent, 0, len(resp.Observations))
	for i, obs := range resp.Observations {
		record := observationRecordFromMCPResponse(resp, obs, i)
		if record.ID == "" {
			continue
		}
		event := eventFromObservationRecord(graphID, nodeID, record, int64(i+1), "mcp_observation")
		event.Kind = ReasoningEventMCPObservationProjected
		event.NodeKind = ReasoningNodeOperation
		events = append(events, NormalizeEvent(event))
	}
	return NormalizeEvents(events)
}

func eventFromObservationRecord(graphID, nodeID string, record types.ObservationRecord, sequence int64, sourceStage string) ReasoningEvent {
	nodeKind := auxiliaryNodeKindForObservation(record)
	reason := firstNonEmpty(string(record.Origin), string(record.SourceRef.Kind), "auxiliary_evidence_projected")
	return NormalizeEvent(ReasoningEvent{
		ID:           auxiliaryEventID(graphID, nodeID, record.ID),
		GraphID:      graphID,
		NodeID:       nodeID,
		NodeKind:     nodeKind,
		Sequence:     sequence,
		Kind:         ReasoningEventAuxiliaryEvidenceProjected,
		ReasonCode:   reason,
		ArtifactRefs: artifactRefsFromObservationSource(record.SourceRef),
		EvidenceRefs: evidenceRefsFromObservationRecord(record, nodeID, sourceStage, reason),
		Payload: eventPayload(ObservationPayload{
			ExternalOrigin:   string(record.Origin),
			SourceKind:       string(record.SourceRef.Kind),
			Producer:         record.Producer,
			ResourceURI:      record.SourceRef.ResourceURI,
			PayloadRef:       firstNonEmpty(record.SourceRef.PayloadRef, record.SourceRef.RawRef, record.SourceRef.ArtifactID),
			ObservationID:    record.ID,
			EvidenceRefCount: len(record.SupportRefs) + 1,
		}),
	})
}

func normalizeObservationRecordForGraph(record types.ObservationRecord, index int) types.ObservationRecord {
	record.ID = strings.TrimSpace(record.ID)
	record.Producer = strings.TrimSpace(record.Producer)
	record.ClaimKey = strings.TrimSpace(record.ClaimKey)
	record.SourceRef = normalizeObservationSourceRefForGraph(record.SourceRef)
	record.SupportRefs = dedupTrimStrings(record.SupportRefs)
	if record.ID == "" {
		record.ID = firstNonEmpty(record.ClaimKey, record.SourceRef.RawRef, record.SourceRef.PayloadRef, record.SourceRef.ArtifactID)
		if record.ID == "" && index >= 0 {
			record.ID = fmt.Sprintf("observation-%d", index+1)
		}
	}
	return record
}

func observationRecordFromMCPResponse(resp types.MCPResponse, obs types.MCPTypedObservation, index int) types.ObservationRecord {
	source := types.ObservationSourceRef{
		Kind:        types.ObservationSourceMCPResource,
		Server:      strings.TrimSpace(resp.ServerName),
		ResourceURI: firstNonEmpty(obs.ResourceURI, resp.ResourceURI),
		MIMEType:    firstNonEmpty(obs.MIMEType, resp.MIMEType),
		RawRef:      firstNonEmpty(obs.RawRef, resp.RawRef),
		PayloadRef:  firstNonEmpty(obs.PayloadRef, resp.PayloadRef),
		RowSetRef:   firstNonEmpty(obs.RowSetRef, resp.RowSetRef),
		PageRef:     firstNonEmpty(obs.PageRef, resp.PageRef),
	}
	record := types.ObservationRecord{
		ID:        firstNonEmpty(obs.RawRef, obs.PayloadRef, obs.RowSetRef, obs.PageRef, fmt.Sprintf("mcp-observation-%d", index+1)),
		Origin:    types.AnswerEvidenceOriginMCPResource,
		Producer:  firstNonEmpty(resp.ServerName, resp.Method, "mcp"),
		SourceRef: source,
		Span: types.ObservationSpan{
			LineStart:   obs.LineStart,
			LineEnd:     obs.LineEnd,
			JSONPointer: firstNonEmpty(obs.JSONPointer, resp.JSONPointer),
			Selector:    firstNonEmpty(obs.Selector, resp.Selector),
			Row:         firstPositive(obs.Row, resp.Row),
		},
		SupportRefs: []string{obs.RawRef, obs.PayloadRef, obs.RowSetRef, obs.PageRef},
	}
	return normalizeObservationRecordForGraph(record, index)
}

func auxiliaryNodeKindForObservation(record types.ObservationRecord) ReasoningNodeKind {
	switch record.Origin {
	case types.AnswerEvidenceOriginRuntimeArtifact,
		types.AnswerEvidenceOriginCommandMeasurement:
		return ReasoningNodeLogTrace
	case types.AnswerEvidenceOriginMCPResource,
		types.AnswerEvidenceOriginConnectorResource,
		types.AnswerEvidenceOriginExternalDocument,
		types.AnswerEvidenceOriginWebPage:
		return ReasoningNodeOperation
	default:
		switch record.SourceRef.Kind {
		case types.ObservationSourceRuntimeArtifact,
			types.ObservationSourceCommand:
			return ReasoningNodeLogTrace
		case types.ObservationSourceMCPResource,
			types.ObservationSourceConnector,
			types.ObservationSourceExternalDocument,
			types.ObservationSourceWebPage:
			return ReasoningNodeOperation
		default:
			return ReasoningNodeEvidence
		}
	}
}

func evidenceRefsFromObservationRecord(record types.ObservationRecord, nodeID, sourceStage, reasonCode string) []ReasoningEvidenceRef {
	ids := append([]string{record.ID}, record.SupportRefs...)
	return evidenceRefsFromIDs(ids, nodeID, "auxiliary_observation", "p1", observationConfidence(record), sourceStage, "reasoning_graph", reasonCode)
}

func observationConfidence(record types.ObservationRecord) string {
	switch {
	case record.Confidence >= 0.8:
		return "high"
	case record.Confidence > 0:
		return "medium"
	default:
		return "unverified"
	}
}

func artifactRefsFromObservationSource(ref types.ObservationSourceRef) []loopkernel.ArtifactRef {
	ref = normalizeObservationSourceRefForGraph(ref)
	var refs []loopkernel.ArtifactRef
	add := func(kind, id, path string) {
		kind = firstNonEmpty(kind, string(ref.Kind), "external_observation")
		id = strings.TrimSpace(id)
		path = strings.TrimSpace(path)
		if id == "" && path == "" {
			return
		}
		refs = append(refs, loopkernel.ArtifactRef{Kind: kind, ID: id, Path: path})
	}
	add(ref.ArtifactKind, ref.ArtifactID, ref.Path)
	add("raw_ref", ref.RawRef, "")
	add("payload_ref", ref.PayloadRef, "")
	add("rowset_ref", ref.RowSetRef, "")
	add("page_ref", ref.PageRef, "")
	if ref.ResourceURI != "" {
		add("mcp_resource", ref.ResourceURI, "")
	}
	if ref.URL != "" {
		add("web_page", ref.URL, "")
	}
	return refs
}

func normalizeObservationSourceRefForGraph(ref types.ObservationSourceRef) types.ObservationSourceRef {
	ref.Repo = strings.TrimSpace(ref.Repo)
	ref.Path = strings.TrimSpace(ref.Path)
	ref.Command = strings.TrimSpace(ref.Command)
	ref.ToolCallID = strings.TrimSpace(ref.ToolCallID)
	ref.RawRef = strings.TrimSpace(ref.RawRef)
	ref.PayloadRef = strings.TrimSpace(ref.PayloadRef)
	ref.RowSetRef = strings.TrimSpace(ref.RowSetRef)
	ref.PageRef = strings.TrimSpace(ref.PageRef)
	ref.ArtifactID = strings.TrimSpace(ref.ArtifactID)
	ref.ArtifactKind = strings.TrimSpace(ref.ArtifactKind)
	ref.URL = strings.TrimSpace(ref.URL)
	ref.Server = strings.TrimSpace(ref.Server)
	ref.ResourceURI = strings.TrimSpace(ref.ResourceURI)
	ref.MIMEType = strings.TrimSpace(ref.MIMEType)
	ref.Connector = strings.TrimSpace(ref.Connector)
	return ref
}

func auxiliaryEventID(graphID, nodeID, observationID string) string {
	return strings.Join([]string{
		strings.ReplaceAll(strings.TrimSpace(graphID), " ", "_"),
		strings.ReplaceAll(strings.TrimSpace(nodeID), " ", "_"),
		"observation",
		strings.ReplaceAll(strings.TrimSpace(observationID), " ", "_"),
	}, ":")
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
