package types

import "strings"

const (
	ReasoningGraphAuditObserved = "observed"
	ReasoningGraphAuditPartial  = "partial"
	ReasoningGraphAuditMissing  = "missing"
)

// ReasoningGraphAuditSummary is the compact user/support-facing view of the
// typed reasoning evidence ledger. It is intentionally a read model: producers
// fill it from typed graph events or compact graph summaries, and consumers must
// not infer control-flow decisions from prompt text, model prose, terminal
// narrative, or visible thinking logs.
type ReasoningGraphAuditSummary struct {
	Source           string                     `json:"source,omitempty"`
	Status           string                     `json:"status,omitempty"`
	ReasonCode       string                     `json:"reason_code,omitempty"`
	GraphID          string                     `json:"graph_id,omitempty"`
	EventCount       int                        `json:"event_count,omitempty"`
	NodeCount        int                        `json:"node_count,omitempty"`
	EvidenceRefCount int                        `json:"evidence_ref_count,omitempty"`
	LastEventKind    string                     `json:"last_event_kind,omitempty"`
	LastReasonCode   string                     `json:"last_reason_code,omitempty"`
	EventRefs        []string                   `json:"event_refs,omitempty"`
	Lanes            []ReasoningGraphAuditLane  `json:"lanes,omitempty"`
	RecentEvents     []ReasoningGraphAuditEvent `json:"recent_events,omitempty"`
	RepairEvents     []ReasoningGraphAuditEvent `json:"repair_events,omitempty"`
	LLMEvents        []ReasoningGraphAuditEvent `json:"llm_events,omitempty"`
	WorkerEvents     []ReasoningGraphAuditEvent `json:"worker_events,omitempty"`
	SubAgentEvents   []ReasoningGraphAuditEvent `json:"subagent_events,omitempty"`
	AuxiliaryEvents  []ReasoningGraphAuditEvent `json:"auxiliary_events,omitempty"`
	Missing          []ReasoningGraphAuditGap   `json:"missing,omitempty"`
}

type ReasoningGraphAuditLane struct {
	Name  string `json:"name,omitempty"`
	Count int    `json:"count,omitempty"`
}

type ReasoningGraphAuditEvent struct {
	EventID          string `json:"event_id,omitempty"`
	NodeID           string `json:"node_id,omitempty"`
	Kind             string `json:"kind,omitempty"`
	ReasonCode       string `json:"reason_code,omitempty"`
	ToolName         string `json:"tool_name,omitempty"`
	Agent            string `json:"agent,omitempty"`
	Stage            string `json:"stage,omitempty"`
	Model            string `json:"model,omitempty"`
	Attempt          int    `json:"attempt,omitempty"`
	MaxAttempts      int    `json:"max_attempts,omitempty"`
	ElapsedMillis    int64  `json:"elapsed_millis,omitempty"`
	RepairCode       string `json:"repair_code,omitempty"`
	ViolationKind    string `json:"violation_kind,omitempty"`
	RepairLocus      string `json:"repair_locus,omitempty"`
	WorkerKind       string `json:"worker_kind,omitempty"`
	WorkerStatus     string `json:"worker_status,omitempty"`
	WorkerRole       string `json:"worker_role,omitempty"`
	SubAgentName     string `json:"subagent_name,omitempty"`
	PermissionAction string `json:"permission_action,omitempty"`
	PermissionReason string `json:"permission_reason_code,omitempty"`
	ScopePathCount   int    `json:"scope_path_count,omitempty"`
	InputRefCount    int    `json:"input_ref_count,omitempty"`
	OutputRefCount   int    `json:"output_ref_count,omitempty"`
	EvidenceRefCount int    `json:"evidence_ref_count,omitempty"`
	ToolResultCount  int    `json:"tool_result_count,omitempty"`
	FactCount        int    `json:"fact_count,omitempty"`
	FlowFindingCount int    `json:"flow_finding_count,omitempty"`
	MCPResponseCount int    `json:"mcp_response_count,omitempty"`
	ExternalOrigin   string `json:"external_origin,omitempty"`
	SourceKind       string `json:"source_kind,omitempty"`
	Producer         string `json:"producer,omitempty"`
	ResourceURI      string `json:"resource_uri,omitempty"`
	PayloadRef       string `json:"payload_ref,omitempty"`
	ObservationID    string `json:"observation_id,omitempty"`
}

type ReasoningGraphAuditGap struct {
	Code     string `json:"code,omitempty"`
	Severity string `json:"severity,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

func ReasoningGraphAuditFromWriteSummary(in *WriteFinalReasoningGraphSummary) *ReasoningGraphAuditSummary {
	if in == nil {
		return nil
	}
	out := ReasoningGraphAuditSummary{
		Source:         "write_final_report_summary",
		Status:         ReasoningGraphAuditObserved,
		ReasonCode:     "write_reasoning_graph_summary",
		GraphID:        in.GraphID,
		EventCount:     in.EventCount,
		NodeCount:      in.NodeCount,
		LastEventKind:  in.LastEventKind,
		LastReasonCode: in.LastReasonCode,
		EventRefs:      append([]string(nil), in.EventRefs...),
		Lanes: []ReasoningGraphAuditLane{
			{Name: "workflow", Count: in.WorkflowEventCount},
			{Name: "tool", Count: in.ToolEventCount},
			{Name: "repair", Count: in.RepairEventCount},
			{Name: "llm", Count: in.LLMEventCount},
		},
	}
	return NormalizeReasoningGraphAuditSummaryPtr(&out)
}

func ReasoningGraphAuditFromAnswerSummary(in *AnswerReasoningGraphSummary) *ReasoningGraphAuditSummary {
	if in == nil {
		return nil
	}
	out := ReasoningGraphAuditSummary{
		Source:           "read_answer_summary",
		Status:           ReasoningGraphAuditObserved,
		ReasonCode:       "read_reasoning_graph_summary",
		GraphID:          in.GraphID,
		EventCount:       in.EventCount,
		NodeCount:        in.NodeCount,
		EvidenceRefCount: in.EvidenceRefCount,
		LastEventKind:    in.LastEventKind,
		LastReasonCode:   in.LastReasonCode,
		EventRefs:        append([]string(nil), in.EventRefs...),
		Lanes: []ReasoningGraphAuditLane{
			{Name: "read", Count: in.ReadEventCount},
			{Name: "tool", Count: in.ToolEventCount},
			{Name: "repair", Count: in.RepairEventCount},
			{Name: "llm", Count: in.LLMEventCount},
		},
	}
	return NormalizeReasoningGraphAuditSummaryPtr(&out)
}

func NormalizeReasoningGraphAuditSummaryPtr(in *ReasoningGraphAuditSummary) *ReasoningGraphAuditSummary {
	if in == nil {
		return nil
	}
	out := NormalizeReasoningGraphAuditSummary(*in)
	if reasoningGraphAuditSummaryEmpty(out) {
		return nil
	}
	return &out
}

func NormalizeReasoningGraphAuditSummary(in ReasoningGraphAuditSummary) ReasoningGraphAuditSummary {
	in.Source = strings.TrimSpace(in.Source)
	in.Status = normalizeReasoningGraphAuditStatus(in.Status)
	in.ReasonCode = strings.TrimSpace(in.ReasonCode)
	in.GraphID = strings.TrimSpace(in.GraphID)
	in.LastEventKind = strings.TrimSpace(in.LastEventKind)
	in.LastReasonCode = strings.TrimSpace(in.LastReasonCode)
	in.EventRefs = dedupTrimWriteWorkflowRunStrings(in.EventRefs)
	in.Lanes = normalizeReasoningGraphAuditLanes(in.Lanes)
	in.RecentEvents = normalizeReasoningGraphAuditEvents(in.RecentEvents)
	in.RepairEvents = normalizeReasoningGraphAuditEvents(in.RepairEvents)
	in.LLMEvents = normalizeReasoningGraphAuditEvents(in.LLMEvents)
	in.WorkerEvents = normalizeReasoningGraphAuditEvents(in.WorkerEvents)
	in.SubAgentEvents = normalizeReasoningGraphAuditEvents(in.SubAgentEvents)
	in.AuxiliaryEvents = normalizeReasoningGraphAuditEvents(in.AuxiliaryEvents)
	in.Missing = normalizeReasoningGraphAuditGaps(in.Missing)
	if in.EventCount < 0 {
		in.EventCount = 0
	}
	if in.NodeCount < 0 {
		in.NodeCount = 0
	}
	if in.EvidenceRefCount < 0 {
		in.EvidenceRefCount = 0
	}
	if in.EventCount == 0 && len(in.EventRefs) == 0 && in.GraphID == "" {
		in.Status = ReasoningGraphAuditMissing
		if in.ReasonCode == "" {
			in.ReasonCode = "reasoning_graph_missing"
		}
	} else if len(in.RecentEvents) == 0 && len(in.EventRefs) > 0 {
		if in.Status == "" || in.Status == ReasoningGraphAuditObserved {
			in.Status = ReasoningGraphAuditPartial
		}
	}
	if in.ReasonCode == "" {
		in.ReasonCode = "reasoning_graph_observed"
	}
	return in
}

func normalizeReasoningGraphAuditStatus(status string) string {
	switch strings.TrimSpace(status) {
	case ReasoningGraphAuditObserved, ReasoningGraphAuditPartial, ReasoningGraphAuditMissing:
		return strings.TrimSpace(status)
	default:
		return ""
	}
}

func normalizeReasoningGraphAuditLanes(in []ReasoningGraphAuditLane) []ReasoningGraphAuditLane {
	out := make([]ReasoningGraphAuditLane, 0, len(in))
	seen := map[string]bool{}
	for _, lane := range in {
		name := strings.TrimSpace(lane.Name)
		if name == "" || lane.Count <= 0 || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, ReasoningGraphAuditLane{Name: name, Count: lane.Count})
	}
	return out
}

func normalizeReasoningGraphAuditEvents(in []ReasoningGraphAuditEvent) []ReasoningGraphAuditEvent {
	out := make([]ReasoningGraphAuditEvent, 0, len(in))
	seen := map[string]bool{}
	for _, event := range in {
		event.EventID = strings.TrimSpace(event.EventID)
		event.NodeID = strings.TrimSpace(event.NodeID)
		event.Kind = strings.TrimSpace(event.Kind)
		event.ReasonCode = strings.TrimSpace(event.ReasonCode)
		event.ToolName = strings.TrimSpace(event.ToolName)
		event.Agent = strings.TrimSpace(event.Agent)
		event.Stage = strings.TrimSpace(event.Stage)
		event.Model = strings.TrimSpace(event.Model)
		event.RepairCode = strings.TrimSpace(event.RepairCode)
		event.ViolationKind = strings.TrimSpace(event.ViolationKind)
		event.RepairLocus = strings.TrimSpace(event.RepairLocus)
		event.WorkerKind = strings.TrimSpace(event.WorkerKind)
		event.WorkerStatus = strings.TrimSpace(event.WorkerStatus)
		event.WorkerRole = strings.TrimSpace(event.WorkerRole)
		event.SubAgentName = strings.TrimSpace(event.SubAgentName)
		event.PermissionAction = strings.TrimSpace(event.PermissionAction)
		event.PermissionReason = strings.TrimSpace(event.PermissionReason)
		if event.Attempt < 0 {
			event.Attempt = 0
		}
		if event.MaxAttempts < 0 {
			event.MaxAttempts = 0
		}
		if event.ElapsedMillis < 0 {
			event.ElapsedMillis = 0
		}
		if event.ScopePathCount < 0 {
			event.ScopePathCount = 0
		}
		if event.InputRefCount < 0 {
			event.InputRefCount = 0
		}
		if event.OutputRefCount < 0 {
			event.OutputRefCount = 0
		}
		if event.EvidenceRefCount < 0 {
			event.EvidenceRefCount = 0
		}
		if event.ToolResultCount < 0 {
			event.ToolResultCount = 0
		}
		if event.FactCount < 0 {
			event.FactCount = 0
		}
		if event.FlowFindingCount < 0 {
			event.FlowFindingCount = 0
		}
		if event.MCPResponseCount < 0 {
			event.MCPResponseCount = 0
		}
		event.ExternalOrigin = strings.TrimSpace(event.ExternalOrigin)
		event.SourceKind = strings.TrimSpace(event.SourceKind)
		event.Producer = strings.TrimSpace(event.Producer)
		event.ResourceURI = strings.TrimSpace(event.ResourceURI)
		event.PayloadRef = strings.TrimSpace(event.PayloadRef)
		event.ObservationID = strings.TrimSpace(event.ObservationID)
		if event.EventID == "" && event.Kind == "" && event.ReasonCode == "" {
			continue
		}
		key := event.EventID + "\x00" + event.Kind + "\x00" + event.ReasonCode + "\x00" + event.NodeID
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, event)
	}
	return out
}

func normalizeReasoningGraphAuditGaps(in []ReasoningGraphAuditGap) []ReasoningGraphAuditGap {
	out := make([]ReasoningGraphAuditGap, 0, len(in))
	seen := map[string]bool{}
	for _, gap := range in {
		gap.Code = strings.TrimSpace(gap.Code)
		gap.Severity = strings.TrimSpace(gap.Severity)
		gap.Detail = strings.TrimSpace(gap.Detail)
		if gap.Code == "" {
			continue
		}
		key := gap.Code + "\x00" + gap.Severity + "\x00" + gap.Detail
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, gap)
	}
	return out
}

func reasoningGraphAuditSummaryEmpty(in ReasoningGraphAuditSummary) bool {
	return in.Source == "" &&
		in.Status == "" &&
		in.ReasonCode == "" &&
		in.GraphID == "" &&
		in.EventCount == 0 &&
		in.NodeCount == 0 &&
		in.EvidenceRefCount == 0 &&
		in.LastEventKind == "" &&
		in.LastReasonCode == "" &&
		len(in.EventRefs) == 0 &&
		len(in.Lanes) == 0 &&
		len(in.RecentEvents) == 0 &&
		len(in.RepairEvents) == 0 &&
		len(in.LLMEvents) == 0 &&
		len(in.WorkerEvents) == 0 &&
		len(in.SubAgentEvents) == 0 &&
		len(in.AuxiliaryEvents) == 0 &&
		len(in.Missing) == 0
}
