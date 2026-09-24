package tracequery

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestSchedulerConcurrencyPublicCompleteDistributionAndBuckets(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_scheduler_concurrency", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	idx := buildTraceIndex(t, "distribution.systrace", string(body))
	res := Run(idx, Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true, BucketMs: 3})
	stats := res.WindowStats.SchedulerConcurrency
	for _, tc := range []struct {
		state     string
		durations []float64
		peaks     []int
		p50       int
	}{
		{"runnable", []float64{4, 4, 2}, []int{2, 2, 1, 0}, 1},
		{"running", []float64{6, 3, 1}, []int{1, 1, 2, 0}, 0},
	} {
		t.Run(tc.state, func(t *testing.T) {
			g := concurrencyPublicGroup(t, stats, tc.state)
			if g.Distribution == nil || len(g.Distribution.Depths) != 3 {
				t.Fatalf("complete distribution missing: %+v", g)
			}
			d := g.Distribution
			if d.DepthCount != 3 || d.OmittedDepths != 0 || d.P50Threads != tc.p50 || d.P95Threads != 2 || d.P99Threads != 2 {
				t.Fatalf("not whole-window time-weighted CDF: %+v", d)
			}
			for i, ms := range tc.durations {
				row := d.Depths[i]
				if row.Threads != i || math.Abs(row.DurationMs-ms) > 1e-8 || math.Abs(row.WindowShare-ms/10) > 1e-8 {
					t.Errorf("depth %d: %+v", i, row)
				}
			}
			if stats.BucketMs != 3 || g.BucketCount != 4 || len(g.Buckets) != 4 || g.OmittedBuckets != 0 {
				t.Fatalf("axis/short tail missing: %+v", g)
			}
			for i, peak := range tc.peaks {
				if g.Buckets[i].Values.PeakThreads != peak {
					t.Errorf("bucket %d: %+v", i, g.Buckets[i])
				}
			}
			if math.Abs((g.Buckets[3].Window.EndTs-g.Buckets[3].Window.StartTs)*1000-1) > 1e-8 {
				t.Fatal("tail width was rounded to nominal bucket width")
			}
			if len(g.Members) != g.AcceptedIntervalCount || g.MemberWitnessUnavailableCount != 0 || g.OmittedMembers != 0 {
				t.Fatalf("member census incomplete: %+v", g)
			}
			for _, m := range g.Members {
				if m.ID == "" || m.SourcePath != idx.Path || m.StartLine != m.StartLocalLine || m.EndLine != m.EndLocalLine || m.WindowContributionMs == nil || m.Thread.PID <= 0 {
					t.Fatalf("member identity missing: %+v", m)
				}
			}
		})
	}
}

func TestSchedulerConcurrencyPublicMemberRealCarryEndpoints(t *testing.T) {
	for _, state := range []string{"running", "runnable"} {
		t.Run(state, func(t *testing.T) {
			rows := []string{concurrencyPublicSwitch(.998, 0, 0, 10, "S"), concurrencyPublicSwitch(1.012, 0, 10, 0, "S")}
			if state == "runnable" {
				rows = []string{concurrencyPublicWake(.998, 10), concurrencyPublicSwitch(1.012, 0, 0, 10, "S")}
			}
			g := concurrencyPublicGroup(t, concurrencyPublicRun(t, concurrencyPublicTrace(rows...), 1, 1.01), state)
			if len(g.Members) != 1 {
				t.Fatalf("missing physical witness: %+v", g)
			}
			m := g.Members[0]
			if m.ActualStartTs != .998 || m.ActualEndTs != 1.012 || m.WindowContribution == nil || m.WindowContribution.StartTs != 1 || m.WindowContribution.EndTs != 1.01 || m.WindowContributionMs == nil || math.Abs(*m.WindowContributionMs-10) > 1e-8 {
				t.Fatalf("clipped head replaced actual physical endpoint: %+v", m)
			}
		})
	}
}

func TestSchedulerConcurrencyPublicExactCDFBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		start    float64
		quantile int
	}{
		{"p50", 1.001, 50}, {"p95", 1.0019, 95}, {"p99", 1.00198, 99},
	} {
		for _, offset := range []float64{0, -.000000001, .000000001} {
			t.Run(tc.name+"_"+strconv.FormatFloat(offset, 'g', -1, 64), func(t *testing.T) {
				g := concurrencyPublicGroup(t, concurrencyPublicRun(t, concurrencyPublicTrace(concurrencyPublicSwitch(tc.start+offset, 0, 0, 10, "S"), concurrencyPublicSwitch(1.002, 0, 10, 0, "S")), 1, 1.002), "running")
				if g.Distribution == nil {
					t.Fatal("missing CDF")
				}
				got := g.Distribution.P50Threads
				if tc.quantile == 95 {
					got = g.Distribution.P95Threads
				}
				if tc.quantile == 99 {
					got = g.Distribution.P99Threads
				}
				want := 0
				if offset < 0 {
					want = 1
				}
				if got != want {
					t.Fatalf("exact decimal CDF boundary=%v offset=%v p%d=%d want=%d", tc.start, offset, tc.quantile, got, want)
				}
			})
		}
	}
}
