package tracequery

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func concurrencyPublicRun(t *testing.T, body string, start, end float64) *SchedulerConcurrencyStats {
	t.Helper()
	idx := buildTraceIndex(t, "scheduler.systrace", body)
	res := Run(idx, Query{View: "window_stats", TimeStart: start, TimeEnd: end, TimeStartSet: true, TimeEndSet: true})
	if res.WindowStats == nil {
		t.Fatalf("window result absent: %+v", res)
	}
	return res.WindowStats.SchedulerConcurrency
}

func concurrencyPublicGroup(t *testing.T, stats *SchedulerConcurrencyStats, state string) SchedulerConcurrencyGroup {
	t.Helper()
	if stats != nil {
		for _, g := range stats.Groups {
			if g.State == state {
				return g
			}
		}
	}
	t.Fatalf("%s group absent: %+v", state, stats)
	return SchedulerConcurrencyGroup{}
}

func concurrencyPublicValues(t *testing.T, g SchedulerConcurrencyGroup, peak int, mean, busy, area float64) {
	t.Helper()
	if g.Values == nil || g.Values.PeakThreads != peak {
		t.Fatalf("wrong values: %+v", g)
	}
	for key, p := range map[string][2]float64{"mean": {g.Values.MeanThreads, mean}, "busy": {g.Values.BusyMs, busy}, "area": {g.Values.ThreadMs, area}} {
		if math.IsNaN(p[0]) || math.IsInf(p[0], 0) || math.Abs(p[0]-p[1]) > 1e-6 {
			t.Errorf("%s=%g want=%g", key, p[0], p[1])
		}
	}
}

func TestSchedulerConcurrencyPublicAuthoredEvalFixture(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_scheduler_concurrency", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	stats := concurrencyPublicRun(t, string(body), 1, 1.01)
	concurrencyPublicValues(t, concurrencyPublicGroup(t, stats, "runnable"), 2, .8, 6, 8)
	concurrencyPublicValues(t, concurrencyPublicGroup(t, stats, "running"), 2, .5, 4, 5)
	if stats.Coverage.OpenEndedIntervals != 1 {
		t.Fatalf("open tail 104 not disclosed: %+v", stats.Coverage)
	}
}

func TestSchedulerConcurrencyPublicHalfOpenAndNativeZero(t *testing.T) {
	for _, tc := range []struct {
		name             string
		rows             []string
		peak             int
		mean, busy, area float64
	}{
		{"serial", []string{concurrencyPublicSwitch(1.001, 0, 0, 10, "S"), concurrencyPublicSwitch(1.003, 0, 10, 11, "S"), concurrencyPublicSwitch(1.005, 0, 11, 0, "S")}, 1, .4, 4, 4},
		{"native_zero", []string{concurrencyPublicSwitch(1.001, 0, 0, 10, "S"), concurrencyPublicSwitch(1.001, 0, 10, 0, "S")}, 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := concurrencyPublicGroup(t, concurrencyPublicRun(t, concurrencyPublicTrace(tc.rows...), 1, 1.01), "running")
			concurrencyPublicValues(t, g, tc.peak, tc.mean, tc.busy, tc.area)
			wire, _ := json.Marshal(g.Values)
			for _, key := range []string{"peak_threads", "mean_threads", "busy_ms", "thread_ms"} {
				if !strings.Contains(string(wire), `"`+key+`":`) {
					t.Errorf("zero field %s omitted: %s", key, wire)
				}
			}
		})
	}
	// Positive-width intervals merely ending at the left edge do not enter
	// the half-open selected population, unlike an in-window native zero.
	stats := concurrencyPublicRun(t, concurrencyPublicTrace(concurrencyPublicWake(.998, 10), concurrencyPublicSwitch(1, 0, 0, 10, "S")), 1, 1.01)
	if stats != nil {
		for _, g := range stats.Groups {
			if g.State == "runnable" && g.AcceptedIntervalCount > 0 {
				t.Fatalf("left-boundary-only interval counted: %+v", g)
			}
		}
	}
}

func TestSchedulerConcurrencyPublicOpenTailAndSameTimestampBackedge(t *testing.T) {
	for _, tc := range []struct {
		name, state string
		rows        []string
	}{
		{"runnable_tail", "runnable", []string{concurrencyPublicWake(1.001, 10)}},
		{"running_tail", "running", []string{concurrencyPublicSwitch(1.001, 0, 0, 10, "S")}},
		{"same_timestamp_last_switch", "running", []string{concurrencyPublicSwitch(1.001, 0, 10, 11, "S"), concurrencyPublicSwitch(1.001, 0, 11, 10, "S")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := concurrencyPublicGroup(t, concurrencyPublicRun(t, concurrencyPublicTrace(tc.rows...), 1, 1.01), tc.state)
			if g.Coverage.OpenEndedIntervals != 1 {
				t.Fatalf("tail closed by window/past row: %+v", g)
			}
			if tc.name != "same_timestamp_last_switch" && (g.Values != nil || g.AcceptedIntervalCount != 0) {
				t.Fatalf("open interval became measured zero: %+v", g)
			}
			if tc.name == "same_timestamp_last_switch" && g.AcceptedIntervalCount != 1 {
				t.Fatalf("same timestamp falsely closed last row: %+v", g)
			}
		})
	}
}

func TestSchedulerConcurrencyPublicWindowProjectionAndPrecision(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		start, end, open, close float64
		mean, busy              float64
	}{
		{"carry_in_and_out", 1, 1.01, .998, 1.012, 1, 10},
		{"zero_origin", 0, .01, 0, .004, .4, 4},
		{"nanosecond_width", 1, 1.000000010, 1.000000001, 1.000000003, .2, .000002},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := concurrencyPublicGroup(t, concurrencyPublicRun(t, concurrencyPublicTrace(concurrencyPublicSwitch(tc.open, 0, 0, 10, "S"), concurrencyPublicSwitch(tc.close, 0, 10, 0, "S")), tc.start, tc.end), "running")
			concurrencyPublicValues(t, g, 1, tc.mean, tc.busy, tc.busy)
			if len(g.Segments) == 0 {
				t.Fatal("precise segments missing")
			}
		})
	}
	t.Run("runnable_real_tail_close", func(t *testing.T) {
		stats := concurrencyPublicRun(t, concurrencyPublicTrace(concurrencyPublicWake(1.008, 10), concurrencyPublicSwitch(1.012, 0, 0, 10, "S")), 1, 1.01)
		concurrencyPublicValues(t, concurrencyPublicGroup(t, stats, "runnable"), 1, .2, 2, 2)
	})
	t.Run("runnable_real_head_and_tail", func(t *testing.T) {
		stats := concurrencyPublicRun(t, concurrencyPublicTrace(concurrencyPublicWake(.998, 10), concurrencyPublicSwitch(1.012, 0, 0, 10, "S")), 1, 1.01)
		concurrencyPublicValues(t, concurrencyPublicGroup(t, stats, "runnable"), 1, 1, 10, 10)
	})
}

func TestSchedulerConcurrencyPublicFullPopulationAndDisplayCap(t *testing.T) {
	var rows []string
	for i := 0; i < 10; i++ {
		rows = append(rows, concurrencyPublicWake(1.001, 100+i), concurrencyPublicSwitch(1.003, i, 0, 100+i, "S"), concurrencyPublicSwitch(1.004, i, 100+i, 0, "S"))
	}
	g := concurrencyPublicGroup(t, concurrencyPublicRun(t, concurrencyPublicTrace(rows...), 1, 1.01), "runnable")
	concurrencyPublicValues(t, g, 10, 2, 2, 20)
	if g.ThreadCount != 10 {
		t.Fatalf("Top8 became population: %+v", g)
	}
	rows = nil
	for i := 0; i < 20; i++ {
		s := 1 + float64(2*i+1)/1000
		rows = append(rows, concurrencyPublicSwitch(s, 0, 0, 10, "S"), concurrencyPublicSwitch(s+.001, 0, 10, 0, "S"))
	}
	g = concurrencyPublicGroup(t, concurrencyPublicRun(t, concurrencyPublicTrace(rows...), 1, 1.05), "running")
	concurrencyPublicValues(t, g, 1, .4, 20, 20)
	if len(g.Segments) != 16 || g.OmittedSegments < 1 {
		t.Fatalf("unbounded/missing timeline omission: %+v", g)
	}
}

func TestSchedulerConcurrencyPublicUnknownCPUStillCountsThread(t *testing.T) {
	// Conflicting wake targets withdraw per-CPU authority, not a closed
	// thread-level wait. The new count must not restore either CPU claim.
	body := concurrencyPublicTrace(concurrencyPublicWake(1.001, 10), strings.Replace(concurrencyPublicWake(1.002, 10), "target_cpu=000", "target_cpu=001", 1), concurrencyPublicSwitch(1.005, 2, 0, 10, "S"))
	idx := buildTraceIndex(t, "unknowncpu.systrace", body)
	res := Run(idx, Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true})
	concurrencyPublicValues(t, concurrencyPublicGroup(t, res.WindowStats.SchedulerConcurrency, "runnable"), 1, .4, 4, 4)
	if res.WindowStats.RunnableCPUContinuity == nil || res.WindowStats.RunnableCPUContinuity.UnknownSegments != 1 {
		t.Fatal("fixture did not retain unknown CPU authority")
	}
}

func TestSchedulerConcurrencyPublicLogicalSegmentsBeyondDisplayCap(t *testing.T) {
	var rows []string
	for i := 0; i < 8; i++ {
		start := 1 + float64(2*i+1)/1000
		rows = append(rows, concurrencyPublicSwitch(start, 0, 0, 10, "S"), concurrencyPublicSwitch(start+.001, 0, 10, 0, "S"))
	}
	rows = append(rows, concurrencyPublicSwitch(1.017, 0, 0, 50, "S"), concurrencyPublicSwitch(1.018, 0, 50, 51, "S"), concurrencyPublicSwitch(1.019, 0, 51, 52, "S"), concurrencyPublicSwitch(1.020, 0, 52, 0, "S"))
	g := concurrencyPublicGroup(t, concurrencyPublicRun(t, concurrencyPublicTrace(rows...), 1, 1.024), "running")
	concurrencyPublicValues(t, g, 1, 11.0/24, 11, 11)
	if len(g.Segments) != 16 || g.OmittedSegments != 3 {
		t.Fatalf("cap split identical-depth logical segments: %+v", g)
	}
}

func TestSchedulerConcurrencyPublicLineCancellationIntegrityAndIdentity(t *testing.T) {
	body := concurrencyPublicTrace(concurrencyPublicSwitch(1.001, 0, 0, 10, "S"), concurrencyPublicSwitch(1.004, 0, 10, 0, "S"))
	idx := buildTraceIndex(t, "guards.systrace", body)
	q := Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true}
	lineQ := q
	lineQ.LineStart = 1
	lineQ.LineEnd = 2
	line := Run(idx, lineQ).WindowStats.SchedulerConcurrency
	if line == nil || line.Window != nil || line.WindowUnavailableReason != "line_bounds_take_precedence" {
		t.Fatalf("line query invented denominator: %+v", line)
	}
	for _, g := range line.Groups {
		if g.Values != nil {
			t.Fatal("line-selected values received time denominator")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := Run(idx, q.WithRunContext(ctx))
	if res.WindowStats != nil || res.ViewCancellation == nil {
		t.Fatalf("cancellation published partial account: %+v", res)
	}
	for name, bad := range map[string]string{
		"malformed": strings.Replace(body, "prev_pid=10", "prev_pid=bad", 1),
		"rollback":  concurrencyPublicSwitch(1.004, 0, 0, 10, "S") + "\n" + concurrencyPublicSwitch(1.001, 0, 10, 0, "S") + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			stats := concurrencyPublicRun(t, bad, 1, 1.01)
			if stats != nil {
				for _, g := range stats.Groups {
					if g.Values != nil {
						t.Fatalf("invalid scheduler obtained duration: %+v", g)
					}
				}
			}
		})
	}
	reused := concurrencyPublicTrace(concurrencyPublicSwitch(1.001, 0, 0, 10, "S"), concurrencyPublicSwitch(1.003, 0, 10, 0, "X"), strings.Replace(concurrencyPublicWake(1.004, 10), "sched_wakeup:", "sched_wakeup_new:", 1), concurrencyPublicSwitch(1.005, 0, 0, 10, "S"), concurrencyPublicSwitch(1.006, 0, 10, 0, "S"), concurrencyPublicSwitch(1.001, 1, 0, 20, "S"), concurrencyPublicSwitch(1.003, 1, 20, 0, "S"))
	stats := concurrencyPublicRun(t, reused, 1, 1.01)
	g := concurrencyPublicGroup(t, stats, "running")
	concurrencyPublicValues(t, g, 1, .2, 2, 2)
	if stats.Coverage.IdentityExcludedTIDs != 1 {
		t.Fatalf("ambiguous reused thread not disclosed: %+v", stats.Coverage)
	}
}

func TestSchedulerConcurrencyPublicRelationScopedAndNativeZeroJSON(t *testing.T) {
	body := concurrencyPublicTrace(concurrencyPublicWake(.998, 10), concurrencyPublicSwitch(1.002, 0, 0, 10, "S"), concurrencyPublicSwitch(1.004, 0, 10, 0, "S"))
	path := filepath.Join(t.TempDir(), "relation.systrace")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{AllowWindowedParse: true, TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true, RelationScoped: true, ScopePID: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !idx.RelationScoped {
		t.Fatal("fixture not relation-scoped")
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true})
	if stats.SchedulerConcurrency != nil {
		t.Fatal("relation subset acquired global concurrency")
	}
	if got := fmt.Sprint(SchedulerConcurrencyPopulationClosedIntervals); got != "accepted_closed_intervals" {
		t.Fatal(got)
	}
}
