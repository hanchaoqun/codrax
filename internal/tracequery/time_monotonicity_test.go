package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func writeTimestampRegressionTrace(t *testing.T, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	body := strings.Join(append([]string{"# tracer: nop"}, lines...), "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func wakeupAt(ts string, waker, wakee int) string {
	return "  waker-" + strconv.Itoa(waker) + " (" + strconv.Itoa(waker) + ") [000] .... " + ts + ": sched_wakeup: comm=target pid=" + strconv.Itoa(wakee) + " prio=120 target_cpu=000"
}

func TestWindowedIndexDoesNotStopBeforeTimestampRegressionBackIntoWindow(t *testing.T) {
	path := writeTimestampRegressionTrace(t, "regressed.systrace",
		wakeupAt("1.000000", 10, 20),
		wakeupAt("2.000000", 10, 20),
		wakeupAt("4.000000", 10, 20), // first row beyond time_end
		wakeupAt("2.500000", 30, 20), // later physical row regresses into window
		wakeupAt("5.000000", 10, 20),
	)
	resetAnchorCaches()
	opts := BuildOptions{
		AllowWindowedParse: true,
		TimeStart:          1.5,
		TimeStartSet:       true,
		TimeEnd:            3.0,
		TimeEndSet:         true,
	}
	idx, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got := eventTimestamps(idx.Events); !reflect.DeepEqual(got, []float64{2.0, 2.5}) {
		t.Fatalf("windowed index silently lost the post-regression in-window row: %v", got)
	}
	if idx.TimestampOrder != TraceTimestampOrderRegressed || idx.ClockRegressions == 0 {
		t.Fatalf("complete scan must publish regressed order and diagnostic count: order=%v regressions=%d", idx.TimestampOrder, idx.ClockRegressions)
	}
	if idx.ScannedLineCount != 6 {
		t.Fatalf("regressed order must scan to EOF, scanned through line %d", idx.ScannedLineCount)
	}

	// A cached NON-monotonic proof must never accidentally unlock the fast
	// stop. The warm result and EOF coverage stay identical.
	warm, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(idx.Events, warm.Events) || warm.ScannedLineCount != idx.ScannedLineCount {
		t.Fatalf("warm regressed scan changed coverage: cold=%+v warm=%+v", idx.Events, warm.Events)
	}
}

func TestStreamEventSearchRetainsChronologicalMatchesAfterTimestampRegression(t *testing.T) {
	path := writeTimestampRegressionTrace(t, "event_search_regressed.systrace",
		wakeupAt("1.000000", 10, 20),
		wakeupAt("2.800000", 10, 20),
		wakeupAt("4.000000", 10, 20),
		wakeupAt("2.100000", 30, 20), // physically late, chronologically first
		wakeupAt("5.000000", 10, 20),
	)
	resetAnchorCaches()
	q := Query{TimeStart: 1.5, TimeEnd: 3.0, EventTypes: []EventType{EventSchedWakeup}, Limit: 1}
	res, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 1 || res.Events[0].Ts != 2.1 {
		t.Fatalf("bounded stream result must keep the earliest chronological match, got %+v", res.Events)
	}
	if res.ClockRegressions == 0 || res.ScannedLineCount != 6 {
		t.Fatalf("regressed stream must disclose regression and scan EOF: %+v", res)
	}

	warm, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Events, warm.Events) || warm.ScannedLineCount != res.ScannedLineCount {
		t.Fatalf("non-monotonic proof must not enable early stop: cold=%+v warm=%+v", res.Events, warm.Events)
	}
}

func TestStreamWindowSweepCountsRowsAfterTimestampRegression(t *testing.T) {
	path := writeTimestampRegressionTrace(t, "sweep_regressed.systrace",
		sweepSwitchLine(2.000000, "a", 20, "S", "b", 30),
		sweepSwitchLine(4.000000, "a", 20, "S", "b", 30),
		sweepSwitchLine(2.500000, "a", 20, "S", "b", 30),
		sweepSwitchLine(5.000000, "a", 20, "S", "b", 30),
	)
	resetAnchorCaches()
	res, err := StreamWindowSweep(context.Background(), path, Query{
		TimeStart: 1.5, TimeStartSet: true,
		TimeEnd: 3.0, TimeEndSet: true,
		BucketMs: 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, row := range res.WindowSweep.Coverage {
		count += row.SchedSwitches
	}
	if count != 2 {
		t.Fatalf("window sweep missed a sched_switch after a future row: coverage=%+v", res.WindowSweep.Coverage)
	}
	if res.ClockRegressions == 0 || res.ScannedLineCount != 5 {
		t.Fatalf("regressed sweep must disclose regression and scan EOF: regressions=%d lines=%d", res.ClockRegressions, res.ScannedLineCount)
	}
}

func TestStateDurationFacesFailClosedWhenRollbackCrossesTimeEnd(t *testing.T) {
	path := writeTimestampRegressionTrace(t, "state_cluster_regressed.systrace",
		sweepSwitchLine(2.000000, "idle/0", 0, "R", "app", 20),
		sweepSwitchLine(4.000000, "other", 99, "S", "idle/0", 0),
		sweepSwitchLine(2.500000, "app", 20, "S", "idle/0", 0),
		sweepSwitchLine(5.000000, "other", 99, "S", "idle/0", 0),
	)
	resetAnchorCaches()
	q := Query{
		PID: 20, TimeStart: 1.5, TimeEnd: 3.0,
	}
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		AllowWindowedParse: true,
		TimeStart:          1.5, TimeStartSet: true,
		TimeEnd: 3.0, TimeEndSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	stats := ComputeWindowStats(idx, q)
	if len(stats.TopRunning) != 0 || len(stats.CPU) != 0 || !containsSubstring(stats.Caveats, "scheduler_duration_fail_closed=true") {
		t.Fatalf("indexed window sorted a t=4 -> t=2.5 same-lane rollback into a fabricated duration: %+v", stats)
	}
	res, err := StreamStateCluster(context.Background(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	if res.ClockRegressions == 0 || res.ScannedLineCount != 5 {
		t.Fatalf("regressed state scan must disclose regression and scan EOF: regressions=%d lines=%d", res.ClockRegressions, res.ScannedLineCount)
	}
	if res.WindowStats == nil || len(res.WindowStats.TopRunning) != 0 || !containsSubstring(res.Caveats, "stream_state_cluster_fail_closed=true") {
		t.Fatalf("streaming window fabricated elapsed time across an out-of-window same-lane rollback: %+v", res.WindowStats)
	}
}

func TestRelationScopeDiscoveryRetainsPostRegressionWaker(t *testing.T) {
	path := writeTimestampRegressionTrace(t, "relation_regressed.systrace",
		wakeupAt("2.000000", 10, 20),
		wakeupAt("4.000000", 99, 98),
		wakeupAt("2.500000", 30, 10), // transitive waker discovered after future row
		wakeupAt("5.000000", 99, 98),
	)
	resetAnchorCaches()
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		AllowWindowedParse: true,
		RelationScoped:     true,
		ScopePID:           20,
		ScopeMaxDepth:      4,
		TimeStart:          1.5,
		TimeStartSet:       true,
		TimeEnd:            3.0,
		TimeEndSet:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range idx.Events {
		if ev.Ts == 2.5 && ev.PID == 30 && ev.WakeePID == 10 {
			found = true
		}
	}
	if !found {
		t.Fatalf("relation discovery stopped before the transitive post-regression waker: %+v", idx.Events)
	}
}

func TestIOLatencyQueryDoesNotBreakBeforeTimestampRegression(t *testing.T) {
	path := writeTimestampRegressionTrace(t, "io_regressed.systrace",
		`  work-500 (500) [001] .... 2.000000: block_rq_issue: 8,0 R 4096 () 1000 + 8 [work]`,
		wakeupAt("4.000000", 99, 98),
		`  irq-71 (2) [000] .... 2.500000: block_rq_complete: 8,0 R () 1000 + 8 [0]`,
	)
	resetAnchorCaches()
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.5, TimeEnd: 3.0})
	if len(stats.IOLatencies) != 1 || stats.IOLatencies[0].DurationMs != 500 {
		t.Fatalf("query-side time_end break lost the regressed completion: %+v", stats.IOLatencies)
	}
}

func eventTimestamps(events []Event) []float64 {
	out := make([]float64, len(events))
	for i := range events {
		out[i] = events[i].Ts
	}
	return out
}
