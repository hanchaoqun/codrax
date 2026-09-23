package tracequery

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestIOInFlightReducerKeepsUnknownSeparateFromZero(t *testing.T) {
	key := ioInFlightGroupKey{"capture", "storage", "read", "8,0", "read"}
	pair := ioInFlightInterval{key, 1, 1.002}
	for _, tc := range []struct {
		name   string
		query  Query
		pairs  []ioInFlightInterval
		starts ioInFlightStarts
		reason string
	}{
		{"line_only_has_no_time_denominator", Query{LineStart: 2, LineEnd: 4}, []ioInFlightInterval{pair}, nil, "line_bounds_take_precedence"},
		{"zero_width_window", Query{TimeStart: 1, TimeEnd: 1, TimeStartSet: true, TimeEndSet: true}, []ioInFlightInterval{pair}, nil, "finite_positive_time_window_not_determined"},
		{"unpaired_start_has_no_measured_zero", Query{TimeStart: 1, TimeEnd: 2}, nil, ioInFlightStarts{key: 1}, "no_accepted_complete_pairs"},
		{"infinite_window", Query{TimeStart: 1, TimeEnd: math.Inf(1)}, []ioInFlightInterval{pair}, nil, "finite_positive_time_window_not_determined"},
		{"finite_bounds_overflow_ms", Query{TimeStart: 1, TimeEnd: math.MaxFloat64}, []ioInFlightInterval{pair}, nil, "finite_positive_time_window_not_determined"},
		{"finite_window_overflow_request_area", Query{TimeStart: 0, TimeEnd: 1e305, TimeStartSet: true}, []ioInFlightInterval{{key, 0, 1e305}, {key, 0, 1e305}}, nil, "non_finite_statistic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := buildIOInFlightStats(tc.query, blockPairingResult{}, storagePairingResult{intervals: tc.pairs, starts: tc.starts})
			if got == nil || len(got.Groups) != 1 || got.Groups[0].Values != nil || len(got.Groups[0].Segments) != 0 || got.Groups[0].ValuesUnavailableReason != tc.reason {
				t.Fatalf("unknown statistics became measured values: %+v", got)
			}
			if _, err := json.Marshal(got); err != nil {
				t.Fatalf("unavailable result must remain valid JSON: %v", err)
			}
		})
	}
}

func TestIOInFlightReducerTupleIdentityAndDeterminism(t *testing.T) {
	keys := []ioInFlightGroupKey{
		{"a|b", "c", "read", "8,0", "read"},
		{"a", "b|c", "read", "8,0", "read"},
		{"a", "c", "read", "", "read"},
		{"a", "c", "read", "unknown", "read"},
	}
	pairs := make([]ioInFlightInterval, len(keys))
	for i, key := range keys {
		pairs[i] = ioInFlightInterval{key, 1.001, 1.003}
	}
	q := Query{PID: 999, TimeStart: 1, TimeEnd: 1.010}
	forward := buildIOInFlightStats(q, blockPairingResult{}, storagePairingResult{intervals: pairs})
	for i, j := 0, len(pairs)-1; i < j; i, j = i+1, j-1 {
		pairs[i], pairs[j] = pairs[j], pairs[i]
	}
	reverse := buildIOInFlightStats(q, blockPairingResult{}, storagePairingResult{intervals: pairs})
	if forward.GroupCount != len(keys) || !reflect.DeepEqual(forward, reverse) || forward.QueryPID != 999 || forward.IssuerScope != IOInFlightIssuerScopeAll {
		t.Fatalf("tuple collided, order drifted, or target PID became a filter: %+v / %+v", forward, reverse)
	}
}

func TestIOInFlightReducerStatisticsPrecedeSegmentCap(t *testing.T) {
	key := ioInFlightGroupKey{"capture", "storage", "read", "8,0", "read"}
	var pairs []ioInFlightInterval
	for i := 0; i < 20; i++ {
		start := 1 + float64(i*2+1)/1000
		pairs = append(pairs, ioInFlightInterval{key, start, start + .001})
	}
	// The maximum occurs strictly beyond the retained chronological prefix.
	for i := 0; i < 3; i++ {
		pairs = append(pairs, ioInFlightInterval{key, 1.070, 1.080})
	}
	got := buildIOInFlightStats(Query{TimeStart: 1, TimeEnd: 1.100}, blockPairingResult{}, storagePairingResult{intervals: pairs})
	g := got.Groups[0]
	if g.Values == nil || g.Values.PeakRequests != 3 || math.Abs(g.Values.MeanRequests-.5) > 1e-9 || math.Abs(g.Values.RequestMs-50) > 1e-6 || math.Abs(g.Values.BusyMs-30) > 1e-6 || len(g.Segments) != ioInFlightSegmentLimit || g.OmittedSegments == 0 {
		t.Fatalf("display cap changed numeric population or hid truncation: %+v", g)
	}
	for _, segment := range g.Segments {
		if segment.Requests == 3 {
			t.Fatal("fixture peak unexpectedly entered the retained prefix")
		}
	}
}

func TestIOInFlightReducerCancellationPublishesNoPartialFace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q := Query{TimeStart: 1, TimeEnd: 2, runCancel: newRunCancelState(ctx)}
	got := buildIOInFlightStats(q, blockPairingResult{census: []IOLatencySummary{{SourcePath: "capture", EndpointFamily: blockEndpointFamilyRQ, IssueTs: 1, CompleteTs: 2}}}, storagePairingResult{})
	if got != nil || !q.runCancel.fired() {
		t.Fatalf("cancelled reduction published a partial face: %+v", got)
	}
}

func TestIOInFlightCoverageUsesFullTypedFamilyCounters(t *testing.T) {
	idx := &Index{}
	integrity := newDurationPairingIntegrity(durationOrderStorage)
	integrity.poisonedLanes["private-source-lane"] = true
	integrity.rejectedEndpointRows = 2
	coverage := ioInFlightPairingCoverage(idx, "storage", integrity, []StorageLatencySummary{
		{Layer: "block", PairedCount: 999, UnpairedStartCount: 999},
		{Layer: "mmc", PairedCount: 10, UnpairedStartCount: 2},
		{Layer: "f2fs", PairedCount: 12, UnpairedDoneCount: 3, AmbiguousCohortCount: 1, PairingSuppressedCount: 2},
	})
	if coverage.Status != IOInFlightCoveragePartial || !coverage.TopologyComplete || coverage.AcceptedPairCount != 22 || coverage.UnpairedStartCount != 2 || coverage.UnpairedDoneCount != 3 || coverage.AmbiguousCohortCount != 1 || coverage.PairingSuppressedCount != 2 || coverage.RejectedEndpointRows != 2 {
		t.Fatalf("coverage borrowed block counts or lost unshown groups: %+v", coverage)
	}
	idx.Windowed = true
	failDurationPairingTopology(integrity)
	coverage = ioInFlightPairingCoverage(idx, "storage", integrity, nil)
	if coverage.Status != IOInFlightCoverageUnavailable || coverage.TopologyComplete {
		t.Fatalf("cropped topology became complete: %+v", coverage)
	}
}
