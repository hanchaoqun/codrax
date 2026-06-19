// Package reasoninggraph contains Codrax's cross-mode typed reasoning evidence
// ledger. It is a projection layer over existing typed artifacts; hard control
// flow must not parse user prose, model rationale, prompt text, terminal
// narrative, or visible thinking logs from this package.
package reasoninggraph

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
)

type ReasoningNodeKind string

const (
	ReasoningNodeAnalyze   ReasoningNodeKind = "analyze"
	ReasoningNodeExplore   ReasoningNodeKind = "explore"
	ReasoningNodeExtract   ReasoningNodeKind = "extract"
	ReasoningNodeFinalize  ReasoningNodeKind = "finalize"
	ReasoningNodePlan      ReasoningNodeKind = "plan"
	ReasoningNodeApply     ReasoningNodeKind = "apply"
	ReasoningNodeVerify    ReasoningNodeKind = "verify"
	ReasoningNodeTool      ReasoningNodeKind = "tool"
	ReasoningNodeRepair    ReasoningNodeKind = "repair"
	ReasoningNodeWorker    ReasoningNodeKind = "worker"
	ReasoningNodeSubAgent  ReasoningNodeKind = "subagent"
	ReasoningNodeLogTrace  ReasoningNodeKind = "log_trace"
	ReasoningNodeData      ReasoningNodeKind = "data"
	ReasoningNodeOperation ReasoningNodeKind = "operation"
	ReasoningNodeEvidence  ReasoningNodeKind = "evidence"
	ReasoningNodeLLM       ReasoningNodeKind = "llm"
)

type ReasoningEdgeKind string

const (
	ReasoningEdgeDependsOn  ReasoningEdgeKind = "depends_on"
	ReasoningEdgeProduces   ReasoningEdgeKind = "produces"
	ReasoningEdgeConsumes   ReasoningEdgeKind = "consumes"
	ReasoningEdgeValidates  ReasoningEdgeKind = "validates"
	ReasoningEdgeRepairs    ReasoningEdgeKind = "repairs"
	ReasoningEdgeBranchesTo ReasoningEdgeKind = "branches_to"
	ReasoningEdgeMergesInto ReasoningEdgeKind = "merges_into"
	ReasoningEdgeCausalRef  ReasoningEdgeKind = "causal_ref"
)

type ReasoningEventKind string

const (
	ReasoningEventGraphSeeded                ReasoningEventKind = "graph_seeded"
	ReasoningEventToolCallObserved           ReasoningEventKind = "tool_call_observed"
	ReasoningEventToolCallRejected           ReasoningEventKind = "tool_call_rejected"
	ReasoningEventToolParamNormalized        ReasoningEventKind = "tool_param_normalized"
	ReasoningEventStructuredPayloadRecovered ReasoningEventKind = "structured_payload_recovered"
	ReasoningEventSchemaRejected             ReasoningEventKind = "schema_rejected"
	ReasoningEventRepairPackEmitted          ReasoningEventKind = "repair_pack_emitted"
	ReasoningEventFallbackRouted             ReasoningEventKind = "fallback_routed"
	ReasoningEventLLMRequestWaiting          ReasoningEventKind = "llm_request_waiting"
	ReasoningEventLLMRequestRetried          ReasoningEventKind = "llm_request_retried"
	ReasoningEventLLMRequestTimeout          ReasoningEventKind = "llm_request_timeout"
)

type ReasoningGraph struct {
	GraphID      string                   `json:"graph_id,omitempty"`
	Mode         string                   `json:"mode,omitempty"`
	Goal         string                   `json:"goal,omitempty"`
	Nodes        []ReasoningNode          `json:"nodes,omitempty"`
	Edges        []ReasoningEdge          `json:"edges,omitempty"`
	EvidenceRefs []ReasoningEvidenceRef   `json:"evidence_refs,omitempty"`
	EventRefs    []loopkernel.ArtifactRef `json:"event_refs,omitempty"`
	CreatedAt    time.Time                `json:"created_at,omitempty"`
	UpdatedAt    time.Time                `json:"updated_at,omitempty"`
}

type ReasoningNode struct {
	ID           string                   `json:"id,omitempty"`
	Kind         ReasoningNodeKind        `json:"kind,omitempty"`
	Role         string                   `json:"role,omitempty"`
	Stage        string                   `json:"stage,omitempty"`
	ReasonCode   string                   `json:"reason_code,omitempty"`
	ArtifactRefs []loopkernel.ArtifactRef `json:"artifact_refs,omitempty"`
	EvidenceRefs []ReasoningEvidenceRef   `json:"evidence_refs,omitempty"`
}

type ReasoningEdge struct {
	ID         string            `json:"id,omitempty"`
	FromNodeID string            `json:"from_node_id,omitempty"`
	ToNodeID   string            `json:"to_node_id,omitempty"`
	Kind       ReasoningEdgeKind `json:"kind,omitempty"`
	ReasonCode string            `json:"reason_code,omitempty"`
}

type ReasoningEvidenceRef struct {
	ID          string                 `json:"id,omitempty"`
	Kind        string                 `json:"kind,omitempty"`
	ArtifactRef loopkernel.ArtifactRef `json:"artifact_ref,omitempty"`
	NodeID      string                 `json:"node_id,omitempty"`
	Priority    string                 `json:"priority,omitempty"`
	Confidence  string                 `json:"confidence,omitempty"`
	SourceStage string                 `json:"source_stage,omitempty"`
	Consumer    string                 `json:"consumer,omitempty"`
	ReasonCode  string                 `json:"reason_code,omitempty"`
}

type ReasoningEvent struct {
	ID           string                   `json:"id,omitempty"`
	GraphID      string                   `json:"graph_id,omitempty"`
	NodeID       string                   `json:"node_id,omitempty"`
	NodeKind     ReasoningNodeKind        `json:"node_kind,omitempty"`
	Sequence     int64                    `json:"sequence,omitempty"`
	Kind         ReasoningEventKind       `json:"kind,omitempty"`
	ReasonCode   string                   `json:"reason_code,omitempty"`
	ArtifactRefs []loopkernel.ArtifactRef `json:"artifact_refs,omitempty"`
	EvidenceRefs []ReasoningEvidenceRef   `json:"evidence_refs,omitempty"`
	Payload      json.RawMessage          `json:"payload,omitempty"`
	At           time.Time                `json:"at,omitempty"`
}

type ObservationPayload struct {
	ToolName          string `json:"tool_name,omitempty"`
	Agent             string `json:"agent,omitempty"`
	Stage             string `json:"stage,omitempty"`
	Model             string `json:"model,omitempty"`
	Attempt           int    `json:"attempt,omitempty"`
	MaxAttempts       int    `json:"max_attempts,omitempty"`
	ElapsedMillis     int64  `json:"elapsed_millis,omitempty"`
	RepairCode        string `json:"repair_code,omitempty"`
	RepairReasonCode  string `json:"repair_reason_code,omitempty"`
	ViolationKind     string `json:"violation_kind,omitempty"`
	RepairLocus       string `json:"repair_locus,omitempty"`
	SchemaName        string `json:"schema_name,omitempty"`
	FallbackTarget    string `json:"fallback_target,omitempty"`
	OriginalByteLen   int    `json:"original_byte_len,omitempty"`
	NormalizedByteLen int    `json:"normalized_byte_len,omitempty"`
	Message           string `json:"message,omitempty"`
}

type ObservationInput struct {
	GraphID      string
	NodeID       string
	NodeKind     ReasoningNodeKind
	Sequence     int64
	Kind         ReasoningEventKind
	ReasonCode   string
	ArtifactRefs []loopkernel.ArtifactRef
	EvidenceRefs []ReasoningEvidenceRef
	Payload      ObservationPayload
	At           time.Time
}

type ReasoningEventSummary struct {
	EventID       string             `json:"event_id,omitempty"`
	NodeID        string             `json:"node_id,omitempty"`
	Kind          ReasoningEventKind `json:"kind,omitempty"`
	ReasonCode    string             `json:"reason_code,omitempty"`
	ToolName      string             `json:"tool_name,omitempty"`
	Agent         string             `json:"agent,omitempty"`
	Stage         string             `json:"stage,omitempty"`
	Model         string             `json:"model,omitempty"`
	Attempt       int                `json:"attempt,omitempty"`
	MaxAttempts   int                `json:"max_attempts,omitempty"`
	ElapsedMillis int64              `json:"elapsed_millis,omitempty"`
	RepairCode    string             `json:"repair_code,omitempty"`
	ViolationKind string             `json:"violation_kind,omitempty"`
	RepairLocus   string             `json:"repair_locus,omitempty"`
	At            time.Time          `json:"at,omitempty"`
}

type ReasoningNodeView struct {
	NodeID         string             `json:"node_id,omitempty"`
	Kind           ReasoningNodeKind  `json:"kind,omitempty"`
	LastEventKind  ReasoningEventKind `json:"last_event_kind,omitempty"`
	LastReasonCode string             `json:"last_reason_code,omitempty"`
	UpdatedAt      time.Time          `json:"updated_at,omitempty"`
}

type EventKindCount struct {
	Kind  ReasoningEventKind `json:"kind,omitempty"`
	Count int                `json:"count,omitempty"`
}

type ReasoningGraphView struct {
	GraphID         string                  `json:"graph_id,omitempty"`
	EventCount      int                     `json:"event_count,omitempty"`
	LastEventKind   ReasoningEventKind      `json:"last_event_kind,omitempty"`
	LastReasonCode  string                  `json:"last_reason_code,omitempty"`
	Nodes           []ReasoningNodeView     `json:"nodes,omitempty"`
	EvidenceRefs    []ReasoningEvidenceRef  `json:"evidence_refs,omitempty"`
	ToolEvents      []ReasoningEventSummary `json:"tool_events,omitempty"`
	RepairEvents    []ReasoningEventSummary `json:"repair_events,omitempty"`
	LLMEvents       []ReasoningEventSummary `json:"llm_events,omitempty"`
	EventKindCounts []EventKindCount        `json:"event_kind_counts,omitempty"`
}

func NewObservationEvent(in ObservationInput) ReasoningEvent {
	payload := normalizeObservationPayload(in.Payload)
	return NormalizeEvent(ReasoningEvent{
		GraphID:      in.GraphID,
		NodeID:       in.NodeID,
		NodeKind:     in.NodeKind,
		Sequence:     in.Sequence,
		Kind:         in.Kind,
		ReasonCode:   in.ReasonCode,
		ArtifactRefs: in.ArtifactRefs,
		EvidenceRefs: in.EvidenceRefs,
		Payload:      eventPayload(payload),
		At:           in.At,
	})
}

func NormalizeEvent(in ReasoningEvent) ReasoningEvent {
	in.ID = strings.TrimSpace(in.ID)
	in.GraphID = strings.TrimSpace(in.GraphID)
	in.NodeID = strings.TrimSpace(in.NodeID)
	in.ReasonCode = strings.TrimSpace(in.ReasonCode)
	in.ArtifactRefs = normalizeArtifactRefs(in.ArtifactRefs)
	in.EvidenceRefs = NormalizeEvidenceRefs(in.EvidenceRefs)
	if len(in.Payload) == 0 || strings.TrimSpace(string(in.Payload)) == "" {
		in.Payload = nil
	}
	if in.ID == "" && in.GraphID != "" && in.Sequence > 0 && in.Kind != "" {
		in.ID = in.GraphID + ":" + formatSequence(in.Sequence) + ":" + string(in.Kind)
	}
	return in
}

func NormalizeEvents(events []ReasoningEvent) []ReasoningEvent {
	out := make([]ReasoningEvent, 0, len(events))
	for _, event := range events {
		event = NormalizeEvent(event)
		if event.GraphID == "" && event.ID == "" && event.NodeID == "" && event.Kind == "" {
			continue
		}
		out = append(out, event)
	}
	SortEvents(out)
	return out
}

func NormalizeEvidenceRefs(in []ReasoningEvidenceRef) []ReasoningEvidenceRef {
	out := make([]ReasoningEvidenceRef, 0, len(in))
	seen := map[string]bool{}
	for _, ref := range in {
		ref.ID = strings.TrimSpace(ref.ID)
		ref.Kind = strings.TrimSpace(ref.Kind)
		ref.NodeID = strings.TrimSpace(ref.NodeID)
		ref.Priority = strings.TrimSpace(strings.ToLower(ref.Priority))
		ref.Confidence = strings.TrimSpace(strings.ToLower(ref.Confidence))
		ref.SourceStage = strings.TrimSpace(ref.SourceStage)
		ref.Consumer = strings.TrimSpace(ref.Consumer)
		ref.ReasonCode = strings.TrimSpace(ref.ReasonCode)
		ref.ArtifactRef = normalizeArtifactRef(ref.ArtifactRef)
		if ref.ID == "" && ref.Kind == "" && ref.NodeID == "" && isEmptyArtifactRef(ref.ArtifactRef) {
			continue
		}
		key := ref.ID + "\x00" + ref.Kind + "\x00" + ref.NodeID + "\x00" + ref.ArtifactRef.Kind + "\x00" + ref.ArtifactRef.ID + "\x00" + ref.ArtifactRef.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ref)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].NodeID < out[j].NodeID
	})
	return out
}

func SortEvents(events []ReasoningEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Sequence != events[j].Sequence {
			return events[i].Sequence < events[j].Sequence
		}
		if !events[i].At.Equal(events[j].At) {
			if events[i].At.IsZero() {
				return false
			}
			if events[j].At.IsZero() {
				return true
			}
			return events[i].At.Before(events[j].At)
		}
		if events[i].ID != events[j].ID {
			return events[i].ID < events[j].ID
		}
		return events[i].Kind < events[j].Kind
	})
}

func IsToolObservationKind(kind ReasoningEventKind) bool {
	switch kind {
	case ReasoningEventToolCallObserved,
		ReasoningEventToolCallRejected,
		ReasoningEventToolParamNormalized:
		return true
	default:
		return false
	}
}

func IsRepairObservationKind(kind ReasoningEventKind) bool {
	switch kind {
	case ReasoningEventStructuredPayloadRecovered,
		ReasoningEventSchemaRejected,
		ReasoningEventRepairPackEmitted,
		ReasoningEventFallbackRouted:
		return true
	default:
		return false
	}
}

func IsLLMObservationKind(kind ReasoningEventKind) bool {
	switch kind {
	case ReasoningEventLLMRequestWaiting,
		ReasoningEventLLMRequestRetried,
		ReasoningEventLLMRequestTimeout:
		return true
	default:
		return false
	}
}

func normalizeObservationPayload(in ObservationPayload) ObservationPayload {
	in.ToolName = strings.TrimSpace(in.ToolName)
	in.Agent = strings.TrimSpace(in.Agent)
	in.Stage = strings.TrimSpace(in.Stage)
	in.Model = strings.TrimSpace(in.Model)
	in.RepairCode = strings.TrimSpace(in.RepairCode)
	in.RepairReasonCode = strings.TrimSpace(in.RepairReasonCode)
	in.ViolationKind = strings.TrimSpace(in.ViolationKind)
	in.RepairLocus = strings.TrimSpace(in.RepairLocus)
	in.SchemaName = strings.TrimSpace(in.SchemaName)
	in.FallbackTarget = strings.TrimSpace(in.FallbackTarget)
	in.Message = trimText(in.Message, 512)
	if in.Attempt < 0 {
		in.Attempt = 0
	}
	if in.MaxAttempts < 0 {
		in.MaxAttempts = 0
	}
	if in.ElapsedMillis < 0 {
		in.ElapsedMillis = 0
	}
	if in.OriginalByteLen < 0 {
		in.OriginalByteLen = 0
	}
	if in.NormalizedByteLen < 0 {
		in.NormalizedByteLen = 0
	}
	return in
}

func observationPayloadFromEvent(event ReasoningEvent) ObservationPayload {
	if len(event.Payload) == 0 {
		return ObservationPayload{}
	}
	var payload ObservationPayload
	if json.Unmarshal(event.Payload, &payload) != nil {
		return ObservationPayload{}
	}
	return normalizeObservationPayload(payload)
}

func eventPayload(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	if strings.TrimSpace(string(data)) == "{}" {
		return nil
	}
	return json.RawMessage(data)
}

func normalizeArtifactRefs(in []loopkernel.ArtifactRef) []loopkernel.ArtifactRef {
	out := make([]loopkernel.ArtifactRef, 0, len(in))
	seen := map[string]bool{}
	for _, ref := range in {
		ref = normalizeArtifactRef(ref)
		if isEmptyArtifactRef(ref) {
			continue
		}
		key := ref.Kind + "\x00" + ref.ID + "\x00" + ref.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ref)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func normalizeArtifactRef(in loopkernel.ArtifactRef) loopkernel.ArtifactRef {
	in.Kind = strings.TrimSpace(in.Kind)
	in.ID = strings.TrimSpace(in.ID)
	in.Path = strings.TrimSpace(in.Path)
	return in
}

func isEmptyArtifactRef(ref loopkernel.ArtifactRef) bool {
	return ref.Kind == "" && ref.ID == "" && ref.Path == ""
}

func trimText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "..."
}

func formatSequence(seq int64) string {
	if seq <= 0 {
		return "000000"
	}
	return fmt.Sprintf("%06d", seq)
}
