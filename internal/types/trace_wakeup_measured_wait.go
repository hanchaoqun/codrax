package types

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// TraceWakeupMeasuredWait preserves the operands used by the native producer.
// It is display-only: the selected segment may be clipped, and a wakeup may
// lie just beyond its end under the engine's existing matching tolerance.
// Neither this measurement nor SegmentState attributes the wait to the waker.
type TraceWakeupMeasuredWait struct {
	StartTs       float64  `json:"start_ts"`
	EndTs         float64  `json:"end_ts"`
	SegmentState  string   `json:"segment_state"`
	SegmentEndTs  float64  `json:"segment_end_ts"`
	StartLine     int      `json:"start_line"`
	ActualStartTs *float64 `json:"actual_start_ts,omitempty"`
}

func (w *TraceWakeupMeasuredWait) Valid() bool {
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	return w != nil && finite(w.StartTs) && finite(w.EndTs) && finite(w.SegmentEndTs) &&
		w.StartTs >= 0 && w.EndTs > w.StartTs && w.SegmentEndTs > w.StartTs && w.StartLine > 0 &&
		(w.SegmentState == "s_sleep" || w.SegmentState == "d_sleep" || w.SegmentState == "io_wait") &&
		(w.ActualStartTs == nil || finite(*w.ActualStartTs) && *w.ActualStartTs >= 0 && *w.ActualStartTs <= w.StartTs)
}

func TraceWakeupMeasuredWaitDisplay(w *TraceWakeupMeasuredWait) string {
	if !w.Valid() {
		return ""
	}
	text := fmt.Sprintf("measured_wait=[%.9f,%.9f)s selected_segment_state=%s start_line=%d", w.StartTs, w.EndTs, w.SegmentState, w.StartLine)
	if w.SegmentEndTs == w.EndTs {
		text += " selected_state_interval=same_as_measured_wait"
	} else {
		text += fmt.Sprintf(" selected_state_interval=[%.9f,%.9f)s wakeup_matches_selected_state_end=false", w.StartTs, w.SegmentEndTs)
	}
	if w.ActualStartTs != nil {
		text += fmt.Sprintf(" actual_segment_start=%.9f start_clipped=%t", *w.ActualStartTs, *w.ActualStartTs < w.StartTs)
	} else {
		text += " actual_segment_start=unknown start_clipped=unknown"
	}
	return text
}

// The note is bound to the existing physical wake event and source/query. It
// never participates in event fingerprints, relationship eligibility or IDs.
type TraceWakeupMeasuredWaitPublication struct {
	Path         string                  `json:"path"`
	QueryScopeID string                  `json:"query_scope_id"`
	PayloadRef   string                  `json:"payload_ref"`
	Wakee        RuntimeWakeupThread     `json:"wakee"`
	WakeupLine   int                     `json:"wakeup_line"`
	QueryStartTs float64                 `json:"query_start_ts"`
	QueryEndTs   float64                 `json:"query_end_ts"`
	Wait         TraceWakeupMeasuredWait `json:"wait"`
}

func DecodeTraceWakeupMeasuredWait(r ObservationRecord) (*TraceWakeupMeasuredWait, bool) {
	event, valid := DecodeRuntimeWakeupEvent(r)
	if !valid || r.Predicate != "wakeup_chain_edge" ||
		(r.ClaimAuthority != ObservationClaimAuthorityUnknown && r.ClaimAuthority != ObservationClaimAuthorityDirectObservation) {
		return nil, false
	}
	var p TraceWakeupMeasuredWaitPublication
	count, calibers := 0, 0
	for _, note := range r.RichNotes {
		if value, ok := strings.CutPrefix(note, TraceNoteKeyWakeupLatencyCaliber+"="); ok {
			if value != "sleep_start_to_sched_wakeup" {
				return nil, false
			}
			calibers++
		}
		if value, ok := strings.CutPrefix(note, TraceNoteKeyWakeupMeasuredWait+"="); ok {
			count++
			if json.Unmarshal([]byte(value), &p) != nil {
				return nil, false
			}
		}
	}
	if count != 1 || calibers != 1 || !p.Wait.Valid() || p.Path != r.SourceRef.Path || p.QueryScopeID != r.SourceRef.QueryScopeID ||
		p.PayloadRef != r.SourceRef.PayloadRef || p.Wakee != event.Wakee || p.WakeupLine != event.Line || p.Wait.EndTs != event.Timestamp ||
		p.QueryStartTs != r.SourceRef.QueryWindowStartTs || p.QueryEndTs != r.SourceRef.QueryWindowEndTs || p.Wait.StartTs < p.QueryStartTs ||
		fmt.Sprintf("%.3f", (p.Wait.EndTs-p.Wait.StartTs)*1000) != r.Value {
		return nil, false
	}
	return &p.Wait, true
}
