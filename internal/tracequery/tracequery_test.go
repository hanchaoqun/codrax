package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const sampleTrace = `
      waker-10   (   10) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=waker next_pid=10 next_prio=20
      waker-10   (   10) [000] .... 1.050000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
      waker-10   (   10) [000] .... 1.060000: sched_switch: prev_comm=waker prev_pid=10 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
        app-20   (   20) [001] .... 1.100000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-10   (   10) [000] .... 1.180000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
        app-20   (   20) [001] .... 1.220000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
        app-20   (   20) [001] .... 1.260000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-10   (   10) [000] .... 1.300000: cpu_idle: state=4294967295 cpu_id=0
      waker-10   (   10) [000] .... 1.310000: clock_set_rate: pid_freq state=1200000 cpu_id=0
      waker-10   (   10) [000] .... 1.320000: block_rq_issue: 8,0 R 4096 () 123 + 8 [waker]
`

func TestParseLineSchedulerEvents(t *testing.T) {
	intern := newStringInterner()
	ev, ok := ParseLine(4, `        app-20   (   20) [001] .... 1.100000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`, intern)
	if !ok {
		t.Fatal("ParseLine did not parse sched_switch")
	}
	if ev.Type != EventSchedSwitch || ev.PrevPID != 20 || ev.PrevState != "S" || ev.NextPID != 0 || ev.CPU != 1 {
		t.Fatalf("unexpected event: %+v", ev)
	}
	wake, ok := ParseLine(5, `      waker-10   (   10) [000] .... 1.180000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`, intern)
	if !ok || wake.Type != EventSchedWakeup || wake.PID != 10 || wake.WakeePID != 20 || wake.TargetCPU != 1 {
		t.Fatalf("unexpected wake event: %+v ok=%v", wake, ok)
	}
}

func TestThreadTimelineSplitsSleepAndRunnable(t *testing.T) {
	idx := buildSampleIndex(t)
	tl := ThreadTimeline(idx, Query{PID: 20, TimeStart: 1.09, TimeEnd: 1.24, MinDurationMs: 1})
	if len(tl.Intervals) < 2 {
		t.Fatalf("expected sleep+runnable intervals, got %+v", tl.Intervals)
	}
	if tl.Intervals[0].State != StateSSleep || tl.Intervals[1].State != StateRunnable {
		t.Fatalf("unexpected interval states: %+v", tl.Intervals)
	}
	if tl.Intervals[0].WakeupLine == 0 && tl.Intervals[1].WakeupLine == 0 {
		t.Fatalf("expected wakeup line to be preserved: %+v", tl.Intervals)
	}
}

func TestWakeupChainFindsWakerAndRoot(t *testing.T) {
	idx := buildSampleIndex(t)
	chain := BuildWakeupChain(idx, Query{PID: 20, TimeStart: 1.10, TimeEnd: 1.22, MaxDepth: 4, MinDurationMs: 1})
	if len(chain.Edges) != 1 {
		t.Fatalf("expected one wakeup edge, got %+v", chain)
	}
	if chain.Edges[0].Waker.PID != 10 || chain.Edges[0].Wakee.PID != 20 || chain.Edges[0].WakeupLine == 0 {
		t.Fatalf("bad edge: %+v", chain.Edges[0])
	}
	if len(chain.RootEvidence) == 0 {
		t.Fatalf("expected root evidence: %+v", chain)
	}
}

func TestWindowStatsComputesCPUAndResourceCounts(t *testing.T) {
	idx := buildSampleIndex(t)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.35})
	if stats.BlockIssueCount != 1 {
		t.Fatalf("BlockIssueCount=%d", stats.BlockIssueCount)
	}
	if len(stats.CPU) == 0 || len(stats.TopRunning) == 0 {
		t.Fatalf("expected cpu stats and top running threads: %+v", stats)
	}
}

func buildSampleIndex(t *testing.T) *Index {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.systrace")
	if err := os.WriteFile(path, []byte(sampleTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if idx.ParsedKnown == 0 {
		t.Fatal("expected known events")
	}
	return idx
}
