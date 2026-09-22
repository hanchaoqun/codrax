package types

import (
	"math"
	"strconv"
	"strings"
)

const TraceEventSearchInventorySchemaVersion = 1
const TraceEventSearchInventoryPredicate = "event_search_inventory"
const TraceEventSearchInventoryRowLimit = 40
const TraceEventSearchInventoryRawLimit = 4096

// TraceJankSourceClock is the parser-defined clock of jank_event_sync's
// nanosecond payload fields. It refers only to the physical source Trace axis,
// not another capture, UTC, or a composite's independently mapped canonical axis.
const TraceJankSourceClock = "source_trace_clock"

// TraceJankLegacyUnverified preserves older receipts without silently upgrading
// them to the current parser's field contract.
const TraceJankLegacyUnverified = "unverified"

// TraceEventSearchInventory is a producer-owned snapshot of ONE event_search
// result. QueryScopeID binds it to its containing record's source receipt.
// Neither another query, model aggregate, nor a nearby timestamp can extend
// this census. This is reported inventory, never scheduler/causal authority.
type TraceEventSearchInventory struct {
	SchemaVersion      int                               `json:"schema_version"`
	QueryScopeID       string                            `json:"query_scope_id"`
	Query              TraceEventSearchInventoryQuery    `json:"query"`
	Coverage           TraceEventSearchInventoryCoverage `json:"coverage"`
	Rows               []TraceEventSearchInventoryRow    `json:"rows"`
	HandoffRowsOmitted int                               `json:"handoff_rows_omitted"`
	RowsComplete       bool                              `json:"rows_complete"`
	// Caveats are verbatim engine disclosures, NOT another typed count source.
	// In particular do not parse malformed-field counts out of this text.
	Caveats []string `json:"caveats,omitempty"`
}

type TraceEventSearchInventoryQuery struct {
	View              string                            `json:"view"`
	Pattern           string                            `json:"pattern,omitempty"`
	Patterns          []string                          `json:"patterns,omitempty"`
	EventTypes        []string                          `json:"event_types,omitempty"`
	TraceMarkActions  []string                          `json:"trace_mark_actions,omitempty"`
	EventFieldFilters []TraceEventSearchInventoryFilter `json:"event_field_filters,omitempty"`
	PID               int                               `json:"pid,omitempty"`
	Thread            string                            `json:"thread,omitempty"`
	ThreadInput       string                            `json:"thread_input,omitempty"`
	TargetScope       string                            `json:"target_scope,omitempty"`
	TimeStart         float64                           `json:"time_start"`
	TimeEnd           float64                           `json:"time_end"`
	TimeStartSet      bool                              `json:"time_start_set"`
	TimeEndSet        bool                              `json:"time_end_set"`
	LineStart         int                               `json:"line_start,omitempty"`
	LineEnd           int                               `json:"line_end,omitempty"`
	SpanName          string                            `json:"span_name,omitempty"`
	Limit             int                               `json:"limit"`
}

type TraceEventSearchInventoryFilter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

type TraceEventSearchInventoryCoverage struct {
	ScopeKind           string  `json:"scope_kind"`
	ScopeTimeStart      float64 `json:"scope_time_start"`
	ScopeTimeEnd        float64 `json:"scope_time_end"`
	ScopeTimestampRows  int     `json:"scope_timestamp_rows"`
	ScopeComplete       bool    `json:"scope_complete"`
	MatchedTimeStart    float64 `json:"matched_time_start"`
	MatchedTimeEnd      float64 `json:"matched_time_end"`
	MatchedTotal        int     `json:"matched_total"`
	Emitted             int     `json:"emitted"`
	EnumerationComplete bool    `json:"enumeration_complete"`
}

type TraceEventSearchInventoryRow struct {
	Line       int    `json:"line"`
	SourcePath string `json:"source_path,omitempty"`
	LocalLine  int    `json:"local_line,omitempty"`
	// TraceTimeSeconds follows the canonical result clock, not necessarily
	// the physical source header. Native payload nanoseconds stay separate.
	TraceTimeSeconds     float64                    `json:"trace_time_seconds"`
	SourceTimeSeconds    float64                    `json:"source_time_seconds,omitempty"`
	SourceTimeKnown      bool                       `json:"source_time_known"`
	TimeDomain           string                     `json:"time_domain,omitempty"`
	CanonicalTimeDomain  string                     `json:"canonical_time_domain,omitempty"`
	EventType            string                     `json:"event_type"`
	Comm                 string                     `json:"comm,omitempty"`
	EmitterTID           int                        `json:"emitter_tid"`
	EmitterTGID          int                        `json:"emitter_tgid"`
	MarkerPID            int                        `json:"marker_pid"`
	CPU                  int                        `json:"cpu"` // -1 stays unknown, never CPU zero.
	Raw                  string                     `json:"raw,omitempty"`
	RawTruncated         bool                       `json:"raw_truncated"`
	RawUnavailableReason string                     `json:"raw_unavailable_reason,omitempty"`
	JankEvent            *TraceEventSearchJankEvent `json:"jank_event,omitempty"`
}

type TraceEventSearchJankEvent struct {
	Values           *TraceEventSearchJankValues `json:"values,omitempty"`
	IssueReason      string                      `json:"issue_reason,omitempty"`
	TimeDomainStatus string                      `json:"time_domain_status"`
}

type TraceEventSearchJankValues struct {
	StartTSNS          int64 `json:"start_ts_ns,string"`
	EndTSNS            int64 `json:"end_ts_ns,string"`
	JankFrames         int64 `json:"jank_frames,string"`
	AppID              int64 `json:"appid,string"`
	ReportedDurationNS int64 `json:"reported_duration_ns,string"`
}

// IsValidTraceEventSearchInventoryRecord admits only an internally consistent
// typed producer receipt. It never upgrades a model claim via prose or refs.
func IsValidTraceEventSearchInventoryRecord(r ObservationRecord) bool {
	i := r.EventSearchInventory
	if i == nil || i.SchemaVersion != TraceEventSearchInventorySchemaVersion ||
		!RuntimeObservationProducerIsDeterministicQuery(r.Producer) ||
		r.Origin != AnswerEvidenceOriginRuntimeArtifact || r.Role != AnswerAggregateRoleSupportingCoverage ||
		r.GroundingPolicy != ClaimGroundingHard || r.ProvenanceLane != ObservationProvenanceArtifactSpan ||
		r.ClaimAuthority != ObservationClaimAuthorityDirectObservation ||
		r.Predicate != TraceEventSearchInventoryPredicate || r.SourceRef.Kind != ObservationSourceRuntimeArtifact ||
		strings.TrimSpace(r.SourceRef.Path) == "" || strings.TrimSpace(r.SourceRef.QueryScopeID) == "" ||
		i.QueryScopeID != r.SourceRef.QueryScopeID || strings.TrimSpace(r.ObservedAt) == "" ||
		(strings.TrimSpace(r.SourceRef.PayloadRef) == "" && strings.TrimSpace(r.SourceRef.RawRef) == "") ||
		i.Query.View != "event_search" {
		return false
	}
	c := i.Coverage
	if c.ScopeKind != "artifact" && c.ScopeKind != "selected_window" && c.ScopeKind != "scan_segment" {
		return false
	}
	for _, n := range []float64{c.ScopeTimeStart, c.ScopeTimeEnd, c.MatchedTimeStart, c.MatchedTimeEnd, i.Query.TimeStart, i.Query.TimeEnd} {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return false
		}
	}
	if c.MatchedTotal < 0 || c.Emitted < 0 || c.Emitted > c.MatchedTotal || c.ScopeTimestampRows < 0 ||
		c.ScopeTimeEnd < c.ScopeTimeStart || c.MatchedTimeEnd < c.MatchedTimeStart ||
		len(i.Rows) > TraceEventSearchInventoryRowLimit || len(i.Rows) > c.Emitted ||
		i.HandoffRowsOmitted != c.Emitted-len(i.Rows) ||
		i.RowsComplete != (c.ScopeComplete && c.EnumerationComplete && len(i.Rows) == c.MatchedTotal) {
		return false
	}
	for _, f := range i.Query.EventFieldFilters {
		if !traceEventSearchInventoryFilterValid(f) {
			return false
		}
	}
	for _, row := range i.Rows {
		if row.Line <= 0 || row.LocalLine < 0 || row.EventType == "" || len(row.Raw) > TraceEventSearchInventoryRawLimit ||
			math.IsNaN(row.TraceTimeSeconds) || math.IsInf(row.TraceTimeSeconds, 0) ||
			math.IsNaN(row.SourceTimeSeconds) || math.IsInf(row.SourceTimeSeconds, 0) {
			return false
		}
		if row.SourceTimeKnown && (row.SourcePath == "" || row.LocalLine <= 0) {
			return false
		}
		if j := row.JankEvent; j != nil {
			if j.TimeDomainStatus != TraceJankSourceClock && j.TimeDomainStatus != TraceJankLegacyUnverified {
				return false
			}
			if j.Values == nil {
				if j.IssueReason == "" {
					return false
				}
			} else {
				v := j.Values
				if j.IssueReason != "" || v.StartTSNS < 0 || v.EndTSNS < v.StartTSNS || v.JankFrames < 0 || v.AppID < 0 ||
					v.ReportedDurationNS != v.EndTSNS-v.StartTSNS {
					return false
				}
			}
		}
	}
	return true
}

func traceEventSearchInventoryFilterValid(f TraceEventSearchInventoryFilter) bool {
	switch f.Field {
	case "start_ts", "end_ts", "jank_frames", "appid":
	default:
		return false
	}
	switch f.Op {
	case "eq", "ne", "gt", "gte", "lt", "lte":
	default:
		return false
	}
	if f.Value == "" {
		return false
	}
	for n, ch := range f.Value {
		if ch == '-' && n == 0 {
			continue
		}
		if ch < '0' || ch > '9' {
			return false
		}
	}
	_, err := strconv.ParseInt(f.Value, 10, 64)
	return err == nil
}

func CloneTraceEventSearchInventory(in *TraceEventSearchInventory) *TraceEventSearchInventory {
	if in == nil {
		return nil
	}
	out := *in
	out.Query.Patterns = append([]string(nil), in.Query.Patterns...)
	out.Query.EventTypes = append([]string(nil), in.Query.EventTypes...)
	out.Query.TraceMarkActions = append([]string(nil), in.Query.TraceMarkActions...)
	out.Query.EventFieldFilters = append([]TraceEventSearchInventoryFilter(nil), in.Query.EventFieldFilters...)
	out.Caveats = append([]string(nil), in.Caveats...)
	out.Rows = append([]TraceEventSearchInventoryRow{}, in.Rows...)
	for n := range out.Rows {
		if j := in.Rows[n].JankEvent; j != nil {
			copy := *j
			if j.Values != nil {
				values := *j.Values
				copy.Values = &values
			}
			out.Rows[n].JankEvent = &copy
		}
	}
	return &out
}
