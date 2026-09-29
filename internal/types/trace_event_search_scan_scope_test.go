package types

import (
	"math"
	"strings"
	"testing"
)

func TestTraceEventSearchScanScopeValidationAndClone(t *testing.T) {
	base := TraceEventSearchScanScope{TimeStart: .5, TimeEnd: 2.5, TimeStartApplied: true, TimeEndApplied: true, ObservedBasis: "physical_timestamp_rows", ObservedCount: 3}
	if !ValidateTraceEventSearchScanScope(nil) || !ValidateTraceEventSearchScanScope(&base) {
		t.Fatal("valid/legacy scope rejected")
	}
	for _, change := range []func(*TraceEventSearchScanScope){
		func(s *TraceEventSearchScanScope) { s.TimeStart = math.NaN() },
		func(s *TraceEventSearchScanScope) { s.TimeEnd = math.Inf(1) },
		func(s *TraceEventSearchScanScope) { s.ObservedCount = -1 },
		func(s *TraceEventSearchScanScope) { s.ObservedBasis = "guessed" },
		func(s *TraceEventSearchScanScope) { s.LineStart = 1 },
		func(s *TraceEventSearchScanScope) { s.LineEnd = -1 },
		func(s *TraceEventSearchScanScope) { s.TimeEndApplied = false },
	} {
		bad := base
		change(&bad)
		if ValidateTraceEventSearchScanScope(&bad) {
			t.Fatalf("invalid scope accepted: %+v", bad)
		}
	}
	clone := CloneTraceEventSearchScanScope(&base)
	clone.ObservedCount = 0
	if base.ObservedCount != 3 || CloneTraceEventSearchScanScope(nil) != nil {
		t.Fatal("clone aliases source")
	}
	for _, tc := range []struct {
		scope *TraceEventSearchScanScope
		want  string
	}{
		{nil, "legacy_unknown"}, {&base, "time[0.500000000,2.500000000]_inclusive_seconds"},
		{&TraceEventSearchScanScope{ObservedBasis: "parsed_events"}, "scan_selector=unbounded"},
		{&TraceEventSearchScanScope{LineStart: 2, ObservedBasis: "parsed_events"}, "lines(2..0; zero=unbounded)"},
	} {
		if !strings.Contains(FormatTraceEventSearchScanScope(tc.scope), tc.want) {
			t.Fatalf("missing %s", tc.want)
		}
	}
}
