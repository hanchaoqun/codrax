package types

import (
	"math"
	"testing"
)

func TestTraceObservationContinuousQueryWindowUsesReceipt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ref   ObservationSourceRef
		known bool
	}{
		{"positive", ObservationSourceRef{QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 2}, true},
		{"explicit_zero", ObservationSourceRef{QueryWindowKnown: true, QueryWindowEndTs: 2}, true},
		{"unknown_stale_endpoints", ObservationSourceRef{QueryWindowStartTs: 1, QueryWindowEndTs: 2}, false},
		{"point", ObservationSourceRef{QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 1}, false},
		{"line_over_time", ObservationSourceRef{QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 2, QueryLineRangeKnown: true, QueryLineStart: 5, QueryLineEnd: 6}, false},
		{"one_line_bound", ObservationSourceRef{QueryWindowKnown: true, QueryWindowEndTs: 2, QueryLineRangeKnown: true, QueryLineEnd: 6}, false},
		{"unknown_stale_lines", ObservationSourceRef{QueryWindowKnown: true, QueryWindowEndTs: 2, QueryLineEnd: 6}, true},
		{"infinity", ObservationSourceRef{QueryWindowKnown: true, QueryWindowEndTs: math.Inf(1)}, false},
		{"nan", ObservationSourceRef{QueryWindowKnown: true, QueryWindowEndTs: math.NaN()}, false},
		{"reversed", ObservationSourceRef{QueryWindowKnown: true, QueryWindowStartTs: 2, QueryWindowEndTs: 1}, false},
		{"negative", ObservationSourceRef{QueryWindowKnown: true, QueryWindowStartTs: -1, QueryWindowEndTs: 1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end, known := TraceObservationContinuousQueryWindow(tc.ref)
			if known != tc.known {
				t.Fatalf("known=%t want=%t", known, tc.known)
			}
			if known && (start != tc.ref.QueryWindowStartTs || end != tc.ref.QueryWindowEndTs) {
				t.Fatal("receipt endpoints changed")
			}
			if !known && (start != 0 || end != 0) {
				t.Fatal("absent scope leaked stale endpoints")
			}
			requestedStart, requestedEnd := 1.0, 2.0
			requested := &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: &requestedStart, TimeEnd: &requestedEnd, SourceQuote: "1..2 seconds"}
			scope := ResolveTraceObservationQueryWindowScope(requested, tc.ref)
			if !known && scope.Role != TraceQueryWindowScopeUnknownQueryWindow {
				t.Fatalf("unknown query borrowed a requested interval: %+v", scope)
			}
		})
	}
}
