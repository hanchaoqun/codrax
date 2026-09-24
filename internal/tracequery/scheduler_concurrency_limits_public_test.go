package tracequery

import (
	"encoding/json"
	"math"
	"testing"
)

func TestSchedulerConcurrencyPublicIndependentCensusCaps(t *testing.T) {
	var rows []string
	for i := 0; i < 40; i++ {
		rows = append(rows, concurrencyPublicSwitch(1, i, 0, 100+i, "S"), concurrencyPublicSwitch(float64(i+2), i, 100+i, 0, "S"))
	}
	idx := buildTraceIndex(t, "depths.systrace", concurrencyPublicTrace(rows...))
	q := Query{View: "window_stats", PID: 100, TimeStart: 1, TimeEnd: 41, TimeStartSet: true, TimeEndSet: true, BucketMs: 1000}
	g := concurrencyPublicGroup(t, Run(idx, q).WindowStats.SchedulerConcurrency, "running")
	concurrencyPublicValues(t, g, 40, 20.5, 40000, 820000)
	concurrencyWitnessPartition(t, g)
	if g.AcceptedIntervalCount != 40 || g.ThreadCount != 40 || len(g.Members) != 16 || g.OmittedMembers != 24 || g.MemberWitnessUnavailableCount != 0 {
		t.Fatalf("target/TopN became the population: %+v", g)
	}
	d := g.Distribution
	if d == nil || d.DepthCount != 40 || len(d.Depths) != 32 || d.OmittedDepths != 8 || d.P50Threads != 20 || d.P95Threads != 38 || d.P99Threads != 40 {
		t.Fatalf("CDF reconstructed from displayed values: %+v", d)
	}
	for i, row := range d.Depths {
		if row.Threads != i+1 || row.DurationMs != 1000 || row.WindowShare != .025 {
			t.Errorf("depth=%+v", row)
		}
	}
	if g.BucketCount != 40 || len(g.Buckets) != 32 || g.OmittedBuckets != 8 {
		t.Fatalf("bucket count omitted suffix not disclosed: %+v", g)
	}
	for i, b := range g.Buckets {
		if b.Values.PeakThreads != 40-i || b.Values.MeanThreads != float64(40-i) || b.Values.BusyMs != 1000 {
			t.Errorf("bucket %d not independent maximum/integral: %+v", i, b)
		}
	}
	if len(g.Segments) != 16 || g.OmittedSegments != 24 {
		t.Fatal("old logical-segment cap changed")
	}
}

func TestSchedulerConcurrencyPublicSameTIDWitnessesDoNotDoubleCount(t *testing.T) {
	body := concurrencyPublicTrace(concurrencyPublicSwitch(1.001, 0, 0, 10, "S"), concurrencyPublicSwitch(1.005, 0, 10, 0, "S"), concurrencyPublicSwitch(1.003, 1, 0, 10, "S"), concurrencyPublicSwitch(1.007, 1, 10, 0, "S"))
	g := concurrencyPublicGroup(t, concurrencyPublicRun(t, body, 1, 1.01), "running")
	concurrencyPublicValues(t, g, 1, .6, 6, 6)
	if len(g.Members) != 2 || g.Distribution == nil || g.Distribution.DepthCount != 2 {
		t.Fatalf("physical witnesses/union distribution lost: %+v", g)
	}
	ms := 0.0
	for _, m := range g.Members {
		ms += *m.WindowContributionMs
	}
	if math.Abs(ms-8) > 1e-8 || math.Abs(g.Distribution.Depths[1].DurationMs-6) > 1e-8 {
		t.Fatal("physical interval sum replaced per-TID union")
	}
}

func TestSchedulerConcurrencyPublicBucketWidthsAndUnavailable(t *testing.T) {
	idx := buildTraceIndex(t, "width.systrace", concurrencyPublicTrace(concurrencyPublicSwitch(0, 0, 0, 10, "S"), concurrencyPublicSwitch(.1, 0, 10, 0, "S")))
	for _, tc := range []struct {
		input, want float64
		count       uint64
	}{{0, 100, 3}, {.1, 1, 300}, {1e6, 60000, 1}, {math.NaN(), 100, 3}} {
		q := Query{View: "window_stats", TimeStartSet: true, TimeEndSet: true, TimeEnd: .3, BucketMs: tc.input}
		s := Run(idx, q).WindowStats.SchedulerConcurrency
		g := concurrencyPublicGroup(t, s, "running")
		if s.BucketMs != tc.want || g.BucketCount != tc.count || g.OmittedBuckets+uint64(len(g.Buckets)) != tc.count {
			t.Fatalf("effective width/decimal axis wrong: %+v %+v", s, g)
		}
	}
	q := Query{View: "window_stats", TimeStartSet: true, TimeEndSet: true, TimeEnd: 1e20, BucketMs: 1}
	g := concurrencyPublicGroup(t, Run(idx, q).WindowStats.SchedulerConcurrency, "running")
	if g.Values == nil || g.Distribution == nil || g.BucketsUnavailableReason != "bucket_count_overflow" || len(g.Buckets) != 0 {
		t.Fatalf("huge axis corrupted independent summary: %+v", g)
	}
	// One real, open switch has neither a distribution of confirmed duration
	// nor measured idle buckets, even though the requested axis is known.
	open := concurrencyPublicGroup(t, concurrencyPublicRun(t, concurrencyPublicTrace(concurrencyPublicSwitch(1.001, 0, 0, 10, "S")), 1, 1.01), "running")
	if open.Distribution != nil || len(open.Buckets) != 0 || len(open.Members) != 0 || open.BucketsUnavailableReason != "no_accepted_closed_intervals" {
		t.Fatalf("unknown tail synthesized zero/closure: %+v", open)
	}
}

func TestSchedulerConcurrencyPublicBucketParameterPreservesOldFaces(t *testing.T) {
	body := concurrencyPublicTrace(concurrencyPublicWake(1.001, 10), concurrencyPublicSwitch(1.004, 0, 0, 10, "S"), concurrencyPublicSwitch(1.007, 0, 10, 0, "S"))
	idx := buildTraceIndex(t, "oldfaces.systrace", body)
	q := Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true, BucketMs: 1}
	a := Run(idx, q)
	q.BucketMs = 3
	b := Run(idx, q)
	// Only the independent scheduler projection changes. No target scheduler
	// account, semantic/root-cause candidate, IO matcher or trace inventory does.
	a.WindowStats.SchedulerConcurrency, b.WindowStats.SchedulerConcurrency = nil, nil
	aw, _ := json.Marshal(a.WindowStats)
	bw, _ := json.Marshal(b.WindowStats)
	if string(aw) != string(bw) {
		t.Fatalf("new bucket parameter changed old statistics\na=%s\nb=%s", aw, bw)
	}
}
