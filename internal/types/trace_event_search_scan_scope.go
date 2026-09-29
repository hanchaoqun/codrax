package types

import (
	"fmt"
	"math"
)

// TraceEventSearchScanScope describes the executed selector, not capture
// completeness or a rate denominator. A nil record is legacy/unknown. Time
// bounds here are the engine's applied inclusive bounds; line bounds take
// precedence. Inventory.Query retains the publication query, including bounds
// supplied by normalization/lookup tolerance or ignored by the legacy engine.
type TraceEventSearchScanScope struct {
	TimeStart        float64 `json:"time_start"`
	TimeEnd          float64 `json:"time_end"`
	TimeStartApplied bool    `json:"time_start_applied"`
	TimeEndApplied   bool    `json:"time_end_applied"`
	LineStart        int     `json:"line_start"`
	LineEnd          int     `json:"line_end"`
	IndexRestricted  bool    `json:"index_restricted"`
	ObservedBasis    string  `json:"observed_basis"`
	ObservedCount    int     `json:"observed_count"`
}

func ValidateTraceEventSearchScanScope(s *TraceEventSearchScanScope) bool {
	if s == nil {
		return true
	}
	if s.ObservedBasis != "parsed_events" && s.ObservedBasis != "physical_timestamp_rows" {
		return false
	}
	return !math.IsNaN(s.TimeStart) && !math.IsInf(s.TimeStart, 0) &&
		!math.IsNaN(s.TimeEnd) && !math.IsInf(s.TimeEnd, 0) &&
		s.ObservedCount >= 0 && s.LineStart >= 0 && s.LineEnd >= 0 &&
		(!(s.LineStart > 0 || s.LineEnd > 0) || !s.TimeStartApplied && !s.TimeEndApplied) &&
		(s.TimeStartApplied || s.TimeStart == 0) && (s.TimeEndApplied || s.TimeEnd == 0)
}

func CloneTraceEventSearchScanScope(s *TraceEventSearchScanScope) *TraceEventSearchScanScope {
	if s == nil {
		return nil
	}
	out := *s
	return &out
}

// FormatTraceEventSearchScanScope is shared by tool and report surfaces. It is
// explanatory only; no consumer should parse it to admit or reject evidence.
func FormatTraceEventSearchScanScope(s *TraceEventSearchScanScope) string {
	if s == nil {
		return "scan_selector=legacy_unknown; scope_time is an observed timestamp envelope, not the requested window or recording coverage"
	}
	selector := "unbounded"
	if s.LineStart > 0 || s.LineEnd > 0 {
		selector = fmt.Sprintf("lines(%d..%d; zero=unbounded)", s.LineStart, s.LineEnd)
	} else if s.TimeStartApplied || s.TimeEndApplied {
		start, end := "unbounded", "unbounded"
		if s.TimeStartApplied {
			start = fmt.Sprintf("%.9f", s.TimeStart)
		}
		if s.TimeEndApplied {
			end = fmt.Sprintf("%.9f", s.TimeEnd)
		}
		selector = "time[" + start + "," + end + "]_inclusive_seconds"
	}
	return fmt.Sprintf("scan_selector=%s index_restricted=%t observed_basis=%s observed_count=%d; observed and matched timestamp envelopes are not requested windows, rate denominators or recording coverage; completeness applies only to this scan and its filters", selector, s.IndexRestricted, s.ObservedBasis, s.ObservedCount)
}
