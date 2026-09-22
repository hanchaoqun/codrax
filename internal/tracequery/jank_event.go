package tracequery

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// JankEventFields is reported marker metadata, not an independently measured
// frame interval. This grammar defines payload nanoseconds on the source Trace
// axis. A reporting header need not equal either endpoint, and source-clock
// membership does not establish a scheduler identity or a causal relation.
type JankEventFields struct {
	Values           *JankEventValues `json:"values,omitempty"`
	IssueReason      string           `json:"issue_reason,omitempty"`
	TimeDomainStatus string           `json:"time_domain_status"`
}

type JankEventValues struct {
	StartTSNS          int64 `json:"start_ts_ns"`
	EndTSNS            int64 `json:"end_ts_ns"`
	JankFrames         int64 `json:"jank_frames"`
	AppID              int64 `json:"appid"`
	ReportedDurationNS int64 `json:"reported_duration_ns"`
}

func attachJankEventFields(ev *Event) {
	// Only the actual synchronous marker grammar is admitted. A same-named
	// raw event, another marker's substring, or an invalid B envelope is not it.
	if ev.Type != EventTraceMark || ev.SpanAction != "B" || !strings.HasPrefix(ev.SpanName, "jank_event_sync:") {
		return
	}
	parsed := &JankEventFields{TimeDomainStatus: types.TraceJankSourceClock}
	if ev.PluginFields == nil {
		ev.PluginFields = &PluginFields{}
	}
	ev.PluginFields.JankEvent = parsed
	fields := strings.Split(strings.TrimPrefix(ev.SpanName, "jank_event_sync:"), ",")
	seen := make(map[string]int64, 4)
	for _, field := range fields {
		key, raw, ok := strings.Cut(strings.TrimSpace(field), "=")
		key, raw = strings.TrimSpace(key), strings.TrimSpace(raw)
		if !ok || key == "" {
			parsed.IssueReason = "invalid_field_syntax"
			return
		}
		switch key {
		case "start_ts", "end_ts", "jank_frames", "appid":
		default:
			// Extra producer metadata remains in the original SpanName/raw.
			// It creates neither another searchable typed field nor permission
			// to recover a missing required value from arbitrary text.
			continue
		}
		if _, exists := seen[key]; exists {
			parsed.IssueReason = "duplicate_field"
			return
		}
		value, err := parseEventFieldInteger(raw)
		if err != nil || value < 0 {
			parsed.IssueReason = "invalid_nonnegative_int64"
			return
		}
		seen[key] = value
	}
	if len(seen) != 4 {
		parsed.IssueReason = "missing_required_field"
		return
	}
	if seen["end_ts"] < seen["start_ts"] {
		parsed.IssueReason = "end_before_start"
		return
	}
	parsed.Values = &JankEventValues{
		StartTSNS: seen["start_ts"], EndTSNS: seen["end_ts"], JankFrames: seen["jank_frames"], AppID: seen["appid"],
		ReportedDurationNS: seen["end_ts"] - seen["start_ts"],
	}
}

// JankEventSummary renders only the typed native metadata; it cannot select a
// scheduler window, overwrite the raw marker, or claim a measured root cause.
func JankEventSummary(ev Event) string {
	plugin := ev.PluginFields
	if plugin == nil || plugin.JankEvent == nil {
		return ""
	}
	fields := plugin.JankEvent
	if fields.Values == nil {
		return fmt.Sprintf("jank_event_fields_invalid=%s native_time_domain=%s (raw marker retained; no numeric authority)", fields.IssueReason, fields.TimeDomainStatus)
	}
	v := fields.Values
	clockNote := "legacy parser clock status; not upgraded"
	if fields.TimeDomainStatus == types.TraceJankSourceClock {
		clockNote = "source Trace axis; ns to s only; header is report time"
	}
	return fmt.Sprintf("jank_event_sync start_ts_ns=%d end_ts_ns=%d jank_frames=%d appid=%d reported_duration_ns=%d native_time_domain=%s (reported metadata; appid is not a scheduler TID; %s)", v.StartTSNS, v.EndTSNS, v.JankFrames, v.AppID, v.ReportedDurationNS, fields.TimeDomainStatus, clockNote)
}

func jankEventInvalidInQuery(ev Event, q Query, typeSet map[EventType]bool, actionSet map[string]bool) bool {
	plugin := ev.PluginFields
	if plugin == nil || plugin.JankEvent == nil || plugin.JankEvent.Values != nil {
		return false
	}
	q.EventFieldFilters = nil
	return eventInQuery(ev, q, typeSet, actionSet)
}

func jankEventIntegrityCaveat(count int) string {
	return fmt.Sprintf("jank_event_fields_invalid=true rows=%d; raw markers retained as inventory but excluded by numeric predicates; missing/invalid fields are not zero; native_time_domain=%s", count, types.TraceJankSourceClock)
}
