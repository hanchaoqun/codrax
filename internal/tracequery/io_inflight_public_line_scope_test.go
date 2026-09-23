package tracequery

import (
	"math"
	"testing"
)

// A line selection is the existing public query authority. Time arguments
// alongside it must not become an unrelated denominator for that population.
func TestIOInflightPublicLineScopeDoesNotBorrowTimeDenominator(t *testing.T) {
	body := "# tracer: nop\n" + ioInflightPublicRQ(ioInflightPublicPair{1, 1.002})
	idx := buildTraceIndex(t, "line-scope.systrace", body)
	for _, tc := range []struct {
		name string
		q    Query
	}{
		{"line_with_conflicting_time", Query{LineStart: 2, LineEnd: 3, TimeStart: 9, TimeEnd: 10, TimeStartSet: true, TimeEndSet: true}},
		{"line_only", Query{LineStart: 2, LineEnd: 3}},
		{"time_only", Query{TimeStart: 1, TimeEnd: 1.010, TimeStartSet: true, TimeEndSet: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.q.View = "window_stats"
			result := Run(idx, tc.q)
			if result.WindowStats == nil || len(result.WindowStats.IOLatencies) != 1 {
				t.Fatalf("fixture did not preserve the original exact pair: %+v", result.WindowStats)
			}
			pair := result.WindowStats.IOLatencies[0]
			if pair.IssueLine != 2 || pair.CompleteLine != 3 || math.Abs(pair.DurationMs-2) > 1e-6 {
				t.Fatalf("original pairing coordinates/duration changed: %+v", pair)
			}
			wire := ioInflightPublicDecode(t, *result.WindowStats)
			if len(wire.Groups) != 1 || wire.Groups[0].AcceptedPairCount != 1 || wire.Groups[0].IssueCount != 1 {
				t.Fatalf("line-authoritative population was filtered by unrelated time: %+v", wire)
			}
			if tc.q.LineStart == 0 && tc.q.LineEnd == 0 {
				if wire.Window == nil || wire.Window.StartTs != 1 || wire.Window.EndTs != 1.010 {
					t.Fatalf("explicit time-only window lost its authority: %+v", wire)
				}
				ioInflightPublicValues(t, wire.Groups[0], 1, 1, 1, .2, 2, 2)
				return
			}
			if wire.LineStart != 2 || wire.LineEnd != 3 || wire.Window != nil || wire.WindowUnavailableReason != "line_bounds_take_precedence" {
				t.Fatalf("line-selected pairs borrowed an unproved time denominator: %+v", wire)
			}
			group := wire.Groups[0]
			if group.Values != nil || len(group.Segments) != 0 || group.ValuesUnavailableReason != wire.WindowUnavailableReason {
				t.Fatalf("unknown line-window occupancy became measured values: %+v", group)
			}
		})
	}
}
