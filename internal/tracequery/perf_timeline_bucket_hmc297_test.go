package tracequery

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestHMC297PerfTimelineDirectAndNormalizedWidth(t *testing.T) {
	idx := buildTraceIndex(t, "bucket.perftrace", "worker-101 (100) [000] .... 1.001000: perf_sample: pid=100 tid=101 cpu=0 sample_weight=3 event=cycles weight_unit=cycles\nworker-101 (100) [000] .... 1.011000: perf_sample: pid=100 tid=101 cpu=0 sample_weight=5 event=cycles weight_unit=cycles\n")
	for _, tc := range []struct {
		name                  string
		bucket, legacy, width float64
		source                string
	}{
		{"default", 0, 0, 1, "default"}, {"legacy", 0, 20, 20, "min_duration_ms"}, {"formal", 20, 10, 20, "bucket_ms"},
		{"zero_falls_back", 0, 10, 10, "min_duration_ms"}, {"negative_falls_back", -1, 10, 10, "min_duration_ms"},
		{"invalid_direct", math.NaN(), math.Inf(1), 1, "default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := Query{View: "perf_timeline", PID: 101, TimeStart: 1, TimeEnd: 1.1, TimeStartSet: true, TimeEndSet: true, BucketMs: tc.bucket, MinDurationMs: tc.legacy}
			direct := BuildPerfTimeline(idx, q)
			normalized := normalizeQuery(idx, q)
			twice := normalizeQuery(idx, normalized)
			got := BuildPerfTimeline(idx, twice)
			if !reflect.DeepEqual(direct, got) || got.BucketMs != tc.width || got.RequestedBucketMs != tc.width || got.BucketWidthSource != tc.source {
				t.Fatalf("direct/repeated normalization drift: direct=%+v normalized=%+v", direct, got)
			}
			twice.BucketMs = 25
			if r := BuildPerfTimeline(idx, twice); r.BucketWidthSource != "bucket_ms" || r.BucketMs != 25 {
				t.Fatalf("stale default stole explicit width: %+v", r)
			}
		})
	}
}

func TestHMC297PerfTimelineDirectObservedEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name      string
		times     []float64
		q         Query
		count     int
		inclusive bool
	}{
		{"observed_two_points", []float64{1.001, 1.011}, Query{BucketMs: 20}, 2, true},
		{"observed_single_point", []float64{1.001}, Query{BucketMs: 20}, 1, false},
		{"explicit_half_open", []float64{1.001, 1.011}, Query{BucketMs: 20, TimeStart: 1, TimeEnd: 1.011, TimeStartSet: true, TimeEndSet: true}, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var text strings.Builder
			for _, ts := range tc.times {
				fmt.Fprintf(&text, "worker-101 (100) [000] .... %.6f: perf_sample: pid=100 tid=101 cpu=0 sample_weight=1 event=cycles weight_unit=cycles\n", ts)
			}
			idx := buildTraceIndex(t, "direct.perftrace", text.String())
			r := BuildPerfTimeline(idx, tc.q)
			if len(r.Buckets) != 1 || r.Buckets[0].SampleCount != tc.count || r.Buckets[0].EndInclusive != tc.inclusive {
				t.Fatalf("direct observed endpoint disclosure: %+v", r)
			}
		})
	}
}
