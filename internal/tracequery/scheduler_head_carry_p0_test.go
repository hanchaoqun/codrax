package tracequery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeSchedulerCarryTrace(t *testing.T, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	body := "# tracer: nop\n" + strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func buildSchedulerCarryWindow(t *testing.T, path string, start, end float64) *Index {
	t.Helper()
	idx, err := BuildIndexWithOptions(t.Context(), path, BuildOptions{
		AllowWindowedParse: true,
		TimeStart:          start,
		TimeEnd:            end,
		TimeStartSet:       true,
		TimeEndSet:         true,
		TimePaddingBefore:  0.05,
		TimePaddingAfter:   0.05,
	})
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestCrossSourceSchedulerHeadWithdrawsThreadAndCPUCarry(t *testing.T) {
	sourceAPath := writeSchedulerCarryTrace(t, "cross-source-a.systrace",
		" waker-7 (7) [000] .... 0.100000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=000",
	)
	sourceBPath := writeSchedulerCarryTrace(t, "cross-source-b.systrace",
		" idle-0 (0) [001] .... 0.200000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=9 next_prio=120",
		" worker-9 (9) [001] .... 1.050000: sched_switch: prev_comm=worker prev_pid=9 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120",
	)
	fullA, err := BuildIndex(t.Context(), sourceAPath)
	if err != nil {
		t.Fatal(err)
	}
	fullB, err := BuildIndex(t.Context(), sourceBPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(fullA.TraceArtifacts) != 1 || len(fullB.TraceArtifacts) != 1 {
		t.Fatalf("fixture source ledger drift: a=%+v b=%+v", fullA.TraceArtifacts, fullB.TraceArtifacts)
	}
	sourceA := fullA.TraceArtifacts[0]
	sourceB := fullB.TraceArtifacts[0]
	sourceB.VirtualLineBase = sourceA.LocalLineCount

	var inWindow []Event
	for _, ev := range fullB.Events {
		if ev.Ts < 1 || ev.Ts > 1.1 {
			continue
		}
		ev.Line += sourceB.VirtualLineBase
		inWindow = append(inWindow, ev)
	}
	if len(inWindow) != 1 || inWindow[0].Type != EventSchedSwitch {
		t.Fatalf("fixture must retain one in-window switch: %+v", inWindow)
	}
	idx := &Index{
		Path:             filepath.Join(t.TempDir(), "defensive-composite.tracebundle.json"),
		TraceArtifacts:   []TraceArtifactSource{sourceA, sourceB},
		LineCount:        sourceA.LocalLineCount + sourceB.LocalLineCount,
		ScannedLineCount: sourceA.LocalLineCount + sourceB.LocalLineCount,
		Windowed:         true,
		IndexTimeStart:   1,
		IndexTimeEnd:     1.1,
		Events:           inWindow,
		FirstTs:          inWindow[0].Ts,
		LastTs:           inWindow[len(inWindow)-1].Ts,
		ParsedKnown:      len(inWindow),
		TimestampOrder:   TraceTimestampOrderMonotonic,
	}
	if err := populateWindowSchedulerHead(t.Context(), idx, 1); err != nil {
		t.Fatal(err)
	}
	q := Query{PID: 42, TimeStart: 1, TimeEnd: 1.1, MinDurationMs: 0.001}
	head := schedulerHeadForQuery(idx, q)
	if head == nil || head.Complete || !head.CrossSourceCPUUnproven || head.Reason != schedulerHeadCrossSourceStateUnprovenReason {
		t.Fatalf("cross-source scheduler-state fail-close verdict missing: %+v", head)
	}
	if len(head.Threads) != 0 || len(head.CPUs) != 0 {
		t.Fatalf("cross-source latest-per-key merge published unproven carry: threads=%+v cpus=%+v", head.Threads, head.CPUs)
	}

	timeline := ThreadTimeline(idx, q)
	if timeline.HeadState == nil || timeline.HeadState.Status != "unknown" || timeline.HeadState.Reason != schedulerHeadCrossSourceStateUnprovenReason {
		t.Fatalf("cross-source head must remain explicitly unknown: %+v", timeline.HeadState)
	}
	if len(timeline.Intervals) != 1 || timeline.Intervals[0].State != StateRunning ||
		!near(timeline.Intervals[0].DurationMs, 50, 0.001) || !timeline.Intervals[0].CPUKnown || timeline.Intervals[0].CPU != 1 {
		t.Fatalf("only the proven in-window running suffix may survive: %+v", timeline.Intervals)
	}

	stats := ComputeWindowStats(idx, q)
	if targetRunnable := threadDurationForPID(stats.RunnableTop, 42); targetRunnable != nil {
		t.Fatalf("unproven cross-source runnable carry survived: %+v", stats.RunnableTop)
	}
	var cpu1Busy float64
	for _, cpu := range stats.CPU {
		if cpu.CPU == 1 {
			cpu1Busy = cpu.BusyMs
		}
	}
	if !near(cpu1Busy, 50, 0.001) {
		t.Fatalf("worker pre-window CPU carry leaked through cross-source merge: %+v", stats.CPU)
	}
	for _, running := range stats.TopRunning {
		if running.Thread.PID == 9 {
			t.Fatalf("worker was fabricated as running before the in-window switch: %+v", stats.TopRunning)
		}
	}
	for _, cpu := range stats.CPUPressure {
		if cpu.RunnableWaitMs != 0 || cpu.RunnableEvents != 0 || len(cpu.TopRunnable) != 0 {
			t.Fatalf("unknown-CPU runnable segments entered per-CPU pressure: %+v", stats.CPUPressure)
		}
	}
	if !containsSubstring(stats.Caveats, "scheduler_head_state_unknown=true") ||
		!containsSubstring(stats.Caveats, "cross_source_scheduler_state_unproven") {
		t.Fatalf("cross-source state fail-close caveat missing: %+v", stats.Caveats)
	}
	if stats.SchedulerHeadCoverage == nil || stats.SchedulerHeadCoverage.Status != "unknown" ||
		stats.SchedulerHeadCoverage.Reason != schedulerHeadCrossSourceStateUnprovenReason {
		t.Fatalf("withdrawn scheduler head was not exposed in coverage: %+v", stats.SchedulerHeadCoverage)
	}
}

func TestCrossSourceRunningHeadIsNotPublishedWithoutReplay(t *testing.T) {
	idx := &Index{Windowed: true, TimestampOrder: TraceTimestampOrderMonotonic, LastTs: 1.1}
	head := newSchedulerHeadSnapshot(1)
	head.Complete = true
	head.CrossSourceCPUUnproven = true
	head.Threads[42] = schedulerHeadThread{
		Thread: ThreadRef{Comm: "target", PID: 42}, State: StateRunning,
		StartTs: 0.2, LastEventTs: 0.2, Line: 7, CPU: 2, CPUKnown: true,
		CPUProvenance: runnableCPUProvenanceSchedSwitch,
	}
	head.CPUs[2] = schedulerHeadCPU{CPU: 2, Thread: ThreadRef{Comm: "target", PID: 42}, StartTs: 0.2, LastEventTs: 0.2, Line: 7}
	idx.setSchedulerHead(head)

	q := Query{PID: 42, TimeStart: 1, TimeEnd: 1.1, MinDurationMs: 0.001}
	timeline := ThreadTimeline(idx, q)
	if len(timeline.Intervals) != 0 {
		t.Fatalf("cross-source running carry survived without a replay: %+v", timeline.Intervals)
	}
	if timeline.HeadState == nil || timeline.HeadState.Status != "unknown" ||
		timeline.HeadState.Reason != schedulerHeadCrossSourceStateUnprovenReason ||
		!containsSubstring(timeline.Caveats, "scheduler_head_state_unknown=true") {
		t.Fatalf("standalone timeline omitted the scheduler-state disclosure: head=%+v caveats=%+v", timeline.HeadState, timeline.Caveats)
	}
	coverage := schedulerHeadCoverageForWindow(idx, q, schedulerHeadForQuery(idx, q))
	if coverage == nil || coverage.Status != "unknown" || coverage.Reason != schedulerHeadCrossSourceStateUnprovenReason ||
		coverage.MissingCPUCount != 0 || coverage.MissingThreadCount != 0 {
		t.Fatalf("cross-source state withdrawal was falsely reported recovered: %+v", coverage)
	}
	dirty := &schedulerHeadSnapshot{
		Complete: true, CrossSourceCPUUnproven: true,
		CPUs: map[int]schedulerHeadCPU{2: {CPU: 2, Thread: ThreadRef{PID: 42}}},
	}
	if got := schedulerHeadSortedCPUs(dirty); len(got) != 0 {
		t.Fatalf("typed cross-source verdict must mechanically gate every CPU-global consumer: %+v", got)
	}
}

func TestWindowedTimelineRecoversSchedulerHeadCarryIn(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "timeline.systrace",
		" target-42 (42) [000] .... 0.100000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=other next_pid=7 next_prio=120",
		"  other-7 (7) [000] .... 1.050000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=000",
		"  other-7 (7) [000] .... 1.080000: sched_switch: prev_comm=other prev_pid=7 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120",
		" target-42 (42) [000] .... 1.120000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=other next_pid=7 next_prio=120",
	)
	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	for _, ev := range idx.Events {
		if ev.Ts == 0.1 {
			t.Fatal("fixture drift: pre-padding switch must not be retained in the bounded event index")
		}
	}

	timeline := ThreadTimeline(idx, Query{PID: 42, TimeStart: 1.0, TimeEnd: 1.1, MinDurationMs: 1})
	if timeline.HeadState == nil || timeline.HeadState.Status != "recovered" || timeline.HeadState.State != StateSSleep {
		t.Fatalf("expected typed recovered sleep carry-in, got %+v", timeline.HeadState)
	}
	if len(timeline.Intervals) != 3 {
		t.Fatalf("expected sleep -> runnable -> running, got %+v", timeline.Intervals)
	}
	wantStates := []ThreadState{StateSSleep, StateRunnable, StateRunning}
	wantDurations := []float64{50, 30, 20}
	for i := range wantStates {
		if timeline.Intervals[i].State != wantStates[i] || !near(timeline.Intervals[i].DurationMs, wantDurations[i], 0.001) {
			t.Fatalf("interval %d mismatch: got %+v want state=%s duration=%.3f", i, timeline.Intervals[i], wantStates[i], wantDurations[i])
		}
	}
	if !near(timeline.Intervals[0].ActualStartTs, 0.1, 0.000001) {
		t.Fatalf("carry-in must preserve the physical segment start, got %+v", timeline.Intervals[0])
	}
}

func TestWindowedTimelineDropsCarryClosedExactlyAtWindowHead(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "timeline_exact_head.systrace",
		" target-42 (42) [000] .... 0.100000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=other next_pid=7 next_prio=120",
		"  other-7 (7) [000] .... 1.000000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=000",
		"  other-7 (7) [000] .... 1.050000: sched_switch: prev_comm=other prev_pid=7 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120",
	)
	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	timeline := ThreadTimeline(idx, Query{PID: 42, TimeStart: 1.0, TimeEnd: 1.1})
	for _, interval := range timeline.Intervals {
		if interval.DurationMs == 0 && interval.ActualStartTs < 1.0 && interval.ActualEndTs <= 1.0 {
			t.Fatalf("a pre-window carry closed at the exact head has no in-window support: %+v", interval)
		}
	}
	if len(timeline.Intervals) != 2 || timeline.Intervals[0].State != StateRunnable || !near(timeline.Intervals[0].DurationMs, 50, 0.001) {
		t.Fatalf("expected runnable then running without a stale zero-width head row, got %+v", timeline.Intervals)
	}
}

func TestWindowedStatsCarryCPUAndRunnableHeadState(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "stats.systrace",
		" <idle>-0 (0) [000] .... 0.100000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=120",
		" worker-42 (42) [000] .... 1.050000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" <idle>-0 (0) [000] .... 1.150000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=7 next_prio=120",
	)
	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.1, MinDurationMs: 1})
	if len(stats.CPU) == 0 || !near(stats.CPU[0].BusyMs, 50, 0.001) || !near(stats.CPU[0].IdleMs, 50, 0.001) {
		t.Fatalf("CPU head carry must account for the full window, got %+v", stats.CPU)
	}
	if len(stats.TopRunning) == 0 || stats.TopRunning[0].Thread.PID != 42 || !near(stats.TopRunning[0].DurationMs, 50, 0.001) {
		t.Fatalf("running head segment missing from TopRunning: %+v", stats.TopRunning)
	}
	if len(stats.SleepTop) == 0 || stats.SleepTop[0].Thread.PID != 42 || !near(stats.SleepTop[0].DurationMs, 50, 0.001) {
		t.Fatalf("post-switch sleep segment drifted: %+v", stats.SleepTop)
	}
}

func TestCreationAtWindowHeadInvalidatesOldOccupantCPUCarry(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "creation_at_head.systrace",
		" old-42 (700) [000] .... 0.500000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=42 next_prio=20",
		" creator-7 (7) [000] .... 1.000000: sched_wakeup_new: comm=new pid=42 prio=30 target_cpu=000",
		" idle-0 (0) [000] .... 1.050000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=9 next_prio=20",
	)
	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.1})
	for _, td := range stats.TopRunning {
		if td.Thread.PID == 42 {
			t.Fatalf("old occupant CPU checkpoint leaked through creation at the window head: %+v", stats.TopRunning)
		}
	}
	if len(stats.CPU) != 1 || !near(stats.CPU[0].BusyMs, 50, 0.001) {
		t.Fatalf("only the post-switch 1.05..1.10 CPU segment is proven: %+v", stats.CPU)
	}
	if stats.SchedulerHeadCoverage == nil || stats.SchedulerHeadCoverage.Status != "partial_unknown" || stats.SchedulerHeadCoverage.MissingCPUCount == 0 {
		t.Fatalf("invalidated CPU head must be disclosed as partial unknown: %+v", stats.SchedulerHeadCoverage)
	}
}

func TestPreWindowCreationInvalidatesOldOccupantCPUCarryWithoutSwitchOut(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "creation_before_head.systrace",
		" old-42 (700) [000] .... 0.100000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=42 next_prio=20",
		" creator-7 (7) [001] .... 0.300000: sched_wakeup_new: comm=new pid=42 prio=30 target_cpu=000",
	)
	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	head := schedulerHeadForQuery(idx, Query{TimeStart: 1.0, TimeEnd: 1.1})
	if head == nil || !head.Complete {
		t.Fatalf("expected complete scheduler head: %+v", head)
	}
	if state, ok := head.Threads[42]; !ok || state.State != StateRunnable || state.Thread.Comm != "new" || state.Thread.TGID != 0 {
		t.Fatalf("new generation runnable checkpoint missing: %+v", state)
	}
	for cpu, state := range head.CPUs {
		if state.Thread.PID == 42 {
			t.Fatalf("CPU%d retained the old tid=42 occupant after sched_wakeup_new: %+v", cpu, state)
		}
	}

	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.1})
	for _, running := range stats.TopRunning {
		if running.Thread.PID == 42 {
			t.Fatalf("old occupant was fabricated as running in the query window: %+v", stats.TopRunning)
		}
	}
	if runnable := threadDurationForPID(stats.RunnableTop, 42); runnable == nil || !near(runnable.DurationMs, 100, 0.001) {
		t.Fatalf("new occupant runnable carry must remain: %+v", stats.RunnableTop)
	}
}

func TestCarryOnlyEstablishedThreadPreservesNativeTGID(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "carry_tgid.systrace",
		" target-42 (700) [000] .... 0.500000: sched_switch: prev_comm=target prev_pid=42 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120",
	)
	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	timeline := ThreadTimeline(idx, Query{PID: 42, TimeStart: 1.0, TimeEnd: 1.1})
	if timeline.Thread.Comm != "target" || timeline.Thread.TGID != 700 {
		t.Fatalf("window-head metadata lost the established generation's TGID: %+v", timeline.Thread)
	}
}

func TestSchedulerHeadRejectsArtifactVersionDifferentFromIndexLedger(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "head_identity.systrace",
		" target-42 (42) [000] .... 0.500000: sched_switch: prev_comm=target prev_pid=42 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120",
	)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	source := singleTraceArtifactSource(path, info.Size(), info.ModTime().UnixNano(), 2, 1)
	if err := os.WriteFile(path, []byte("# replaced\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stamp := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := sourceSchedulerHeadSnapshot(t.Context(), source, 1.0); err == nil || !strings.Contains(err.Error(), "differs from parsed artifact ledger") {
		t.Fatalf("head scan accepted a different artifact version: %v", err)
	}
}

func TestWindowedSchedulerLatencyStartsAtRecoveredRunnableHead(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "latency.systrace",
		" target-42 (42) [000] .... 0.100000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=R ==> next_comm=other next_pid=7 next_prio=120",
		"  other-7 (7) [000] .... 1.040000: sched_switch: prev_comm=other prev_pid=7 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120",
		" target-42 (42) [000] .... 1.080000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=other next_pid=7 next_prio=120",
	)
	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	latency := BuildSchedulerLatencyStats(idx, Query{PID: 42, TimeStart: 1.0, TimeEnd: 1.1, MinDurationMs: 1})
	if len(latency.Items) == 0 || latency.Items[0].Thread.PID != 42 || !near(latency.Items[0].DurationMs, 40, 0.001) {
		t.Fatalf("recovered runnable head must feed scheduler latency, got %+v", latency.Items)
	}
}

func TestPreWindowMigrationMovesRunnableCarryCPUWithoutRestartingInterval(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "runnable_migrate.systrace",
		" creator-7 (7) [000] .... 0.100000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=000",
		" target-42 (42) [000] .... 0.800000: sched_migrate_task: comm=target pid=42 prio=120 orig_cpu=0 dest_cpu=1",
		"  other-9 (9) [001] .... 1.040000: sched_switch: prev_comm=other prev_pid=9 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120",
		" target-42 (42) [001] .... 1.080000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=other next_pid=9 next_prio=120",
	)
	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	head := schedulerHeadForQuery(idx, Query{TimeStart: 1.0, TimeEnd: 1.1})
	if head == nil || !head.Complete {
		t.Fatalf("expected complete scheduler head: %+v", head)
	}
	state, ok := head.Threads[42]
	if !ok || state.State != StateRunnable || !state.CPUKnown || state.CPU != 1 {
		t.Fatalf("pre-window migration did not move runnable carry to cpu1: %+v", state)
	}
	if state.StartTs != 0.1 || state.LastEventTs != 0.8 {
		t.Fatalf("migration restarted the governing runnable interval: %+v", state)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.1})
	runnable := threadDurationForPID(stats.RunnableTop, 42)
	if runnable == nil || runnable.CPU != 1 || !near(runnable.DurationMs, 40, 0.001) {
		t.Fatalf("runnable carry used stale pre-migration CPU: %+v", stats.RunnableTop)
	}
}

func TestMigrationAtWindowHeadMovesRunnableCarryBeforeAttribution(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "runnable_migrate_at_head.systrace",
		" creator-7 (7) [000] .... 0.100000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=000",
		" target-42 (42) [000] .... 1.000000: sched_migrate_task: comm=target pid=42 prio=120 orig_cpu=0 dest_cpu=1",
		"  other-9 (9) [001] .... 1.040000: sched_switch: prev_comm=other prev_pid=9 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120",
	)
	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	q := Query{PID: 42, TimeStart: 1.0, TimeEnd: 1.1, MinDurationMs: 0.001}
	stats := ComputeWindowStats(idx, q)
	var cpu1Ms float64
	for _, td := range stats.RunnableTop {
		if td.Thread.PID != 42 {
			continue
		}
		if td.CPU == 0 && td.DurationMs > 0 {
			t.Fatalf("left-boundary migration left a positive old-CPU fragment: %+v", stats.RunnableTop)
		}
		if td.CPU == 1 {
			cpu1Ms += td.DurationMs
		}
	}
	if !near(cpu1Ms, 40, 0.001) {
		t.Fatalf("left-boundary destination carry mismatch: %+v", stats.RunnableTop)
	}
	latency := BuildSchedulerLatencyStats(idx, q)
	if len(latency.Items) != 1 || latency.Items[0].CPU != 1 || !near(latency.Items[0].DurationMs, 40, 0.001) {
		t.Fatalf("scheduler latency did not consume the boundary migration: %+v", latency.Items)
	}
}

func TestInWindowMigrationSplitsRunnableCPUAttribution(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "runnable_migrate_in_window.systrace",
		" creator-7 (7) [000] .... 0.100000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=000",
		" target-42 (42) [000] .... 1.020000: sched_migrate_task: comm=target pid=42 prio=120 orig_cpu=0 dest_cpu=1",
		"  other-9 (9) [001] .... 1.060000: sched_switch: prev_comm=other prev_pid=9 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120",
	)
	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	q := Query{PID: 42, TimeStart: 1.0, TimeEnd: 1.1, MinDurationMs: 0.001}
	stats := ComputeWindowStats(idx, q)
	byCPU := map[int]float64{}
	for _, td := range stats.RunnableTop {
		if td.Thread.PID == 42 {
			byCPU[td.CPU] += td.DurationMs
		}
	}
	if !near(byCPU[0], 20, 0.001) || !near(byCPU[1], 40, 0.001) {
		t.Fatalf("in-window migration did not split old/destination CPU waits: %+v", stats.RunnableTop)
	}
	latency := BuildSchedulerLatencyStats(idx, q)
	latencyByCPU := map[int]float64{}
	for _, item := range latency.Items {
		if item.Thread.PID == 42 {
			latencyByCPU[item.CPU] += item.DurationMs
		}
	}
	if !near(latencyByCPU[0], 20, 0.001) || !near(latencyByCPU[1], 40, 0.001) {
		t.Fatalf("scheduler latency did not split the migrated wait: %+v", latency.Items)
	}
}

func TestPreWindowMigrationDoesNotMoveRunningCarry(t *testing.T) {
	snapshot := newSchedulerHeadSnapshot(1.0)
	snapshot.Threads[42] = schedulerHeadThread{
		Thread: ThreadRef{Comm: "target", PID: 42}, State: StateRunning,
		StartTs: 0.1, LastEventTs: 0.1, Line: 1, CPU: 0, CPUKnown: true,
	}
	applySchedulerHeadEvent(snapshot, Event{
		Type: EventCPUConstraint, Ts: 0.8, Line: 2,
		ConstraintFields: &ConstraintFields{Kind: "sched_migrate_task", PID: 42, OrigCPU: 0, OrigCPUSet: true, DestCPU: 1, DestCPUSet: true},
	})
	state := snapshot.Threads[42]
	if state.CPU != 0 || state.LastEventTs != 0.1 || state.StartTs != 0.1 {
		t.Fatalf("migration row mutated a non-runnable checkpoint: %+v", state)
	}
}

func TestPreWindowMigrationTimestampRollbackFailsHeadClosed(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "runnable_migrate_rollback.systrace",
		" creator-7 (7) [000] .... 2.000000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=000",
		" target-42 (42) [000] .... 1.000000: sched_migrate_task: comm=target pid=42 prio=120 orig_cpu=0 dest_cpu=1",
	)
	idx := buildSchedulerCarryWindow(t, path, 3.0, 3.1)
	head := schedulerHeadForQuery(idx, Query{TimeStart: 3.0, TimeEnd: 3.1})
	if head == nil || head.Complete || !strings.Contains(head.Reason, "scheduler_lane_timestamp_regressed") {
		t.Fatalf("migrate rollback was sorted into a plausible head: %+v", head)
	}
}

func TestWindowedTimelineDisclosesUnknownHeadOnRegressedTrace(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "regressed.systrace",
		" target-42 (42) [000] .... 0.100000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=other next_pid=7 next_prio=120",
		"  other-7 (7) [000] .... 1.200000: sched_wakeup: comm=other pid=7 prio=120 target_cpu=000",
		"  other-7 (7) [000] .... 0.900000: sched_wakeup: comm=other pid=7 prio=120 target_cpu=000",
		"  other-7 (7) [000] .... 1.050000: sched_switch: prev_comm=other prev_pid=7 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120",
		" target-42 (42) [000] .... 1.080000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=other next_pid=7 next_prio=120",
	)
	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	if idx.TimestampOrder != TraceTimestampOrderRegressed {
		t.Fatalf("fixture must carry a complete regressed proof, got %v", idx.TimestampOrder)
	}
	timeline := ThreadTimeline(idx, Query{PID: 42, TimeStart: 1.0, TimeEnd: 1.1, MinDurationMs: 1})
	if timeline.HeadState == nil || timeline.HeadState.Status != "unknown" || !strings.Contains(timeline.HeadState.Reason, "scheduler_lane_timestamp_regressed") {
		t.Fatalf("regressed source must disclose an unknown head, got %+v", timeline.HeadState)
	}
	if !containsSubstring(timeline.Caveats, "scheduler_head_state_unknown=true") {
		t.Fatalf("unknown head must reach the consumer caveat surface: %+v", timeline.Caveats)
	}
}

func TestStreamStateClusterConsumesPreWindowSchedulerCarry(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "stream.systrace",
		" target-42 (42) [000] .... 0.100000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=other next_pid=7 next_prio=120",
		"  other-7 (7) [000] .... 1.050000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=000",
		"  other-7 (7) [000] .... 1.080000: sched_switch: prev_comm=other prev_pid=7 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120",
		" target-42 (42) [000] .... 1.100000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=other next_pid=7 next_prio=120",
	)
	result, err := StreamStateCluster(t.Context(), path, Query{PID: 42, TimeStart: 1.0, TimeEnd: 1.1}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if result.WindowStats == nil || len(result.WindowStats.SleepTop) == 0 || !near(result.WindowStats.SleepTop[0].DurationMs, 50, 0.001) {
		t.Fatalf("streaming state cluster lost the pre-window sleep carry: %+v", result.WindowStats)
	}
	if len(result.WindowStats.RunnableTop) == 0 || !near(result.WindowStats.RunnableTop[0].DurationMs, 30, 0.001) {
		t.Fatalf("streaming state cluster lost the wakeup-to-switch runnable interval: %+v", result.WindowStats.RunnableTop)
	}
}
