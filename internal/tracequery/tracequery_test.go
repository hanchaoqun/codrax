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

const resourceTrace = `
       main-20   (   20) [001] .... 2.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=main next_pid=20 next_prio=53
       main-20   (   20) [001] .... 2.010000: sched_switch: prev_comm=main prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=worker next_pid=30 next_prio=20
     worker-30   (   30) [001] .... 2.030000: sched_switch: prev_comm=worker prev_pid=30 prev_prio=20 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-10   (   10) [000] .... 2.040000: sched_blocked_reason: pid=30 caller=fscache_page_wait_on_page_bit
      waker-10   (   10) [000] .... 2.050000: cpu_idle: state=4294967295 cpu_id=0
      waker-10   (   10) [000] .... 2.060000: clock_set_rate: pid_freq state=1800000 cpu_id=0
      waker-10   (   10) [000] .... 2.070000: block_rq_issue: 8,0 R 4096 () 123 + 8 [worker]
      waker-10   (   10) [000] .... 2.080000: block_rq_complete: 8,0 R () 123 + 8 [0]
      waker-10   (   10) [000] .... 2.090000: binder_transaction: transaction=7 dest_node=0 dest_proc=40 dest_thread=41 reply=1 flags=0x0 code=0x1
      waker-10   (   10) [000] .... 2.100000: irq_handler_entry: irq=32 name=kirq
      waker-10   (   10) [000] .... 2.110000: mm_vmscan_direct_reclaim_begin: order=0 may_writepage=1
      waker-10   (   10) [000] .... 2.120000: print: B|20|Choreographer#doFrame
     worker-30   (   30) [001] .... 2.150000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=30 next_prio=20
       main-20   (   20) [001] .... 2.200000: sched_switch: prev_comm=worker prev_pid=30 prev_prio=20 prev_state=S ==> next_comm=main next_pid=20 next_prio=53
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

func TestParseLineSupportedResourceEvents(t *testing.T) {
	intern := newStringInterner()
	cases := []struct {
		name  string
		line  string
		want  EventType
		check func(Event) bool
	}{
		{
			name:  "blocked reason",
			line:  `      waker-10   (   10) [000] .... 2.040000: sched_blocked_reason: pid=30 caller=fscache_page_wait_on_page_bit`,
			want:  EventSchedBlockedReason,
			check: func(ev Event) bool { return ev.WakeePID == 30 && ev.Reason == "fscache_page_wait_on_page_bit" },
		},
		{
			name:  "cpu idle",
			line:  `      waker-10   (   10) [000] .... 2.050000: cpu_idle: state=4294967295 cpu_id=0`,
			want:  EventCPUIdle,
			check: func(ev Event) bool { return ev.CPUForField == 0 },
		},
		{
			name:  "cpu frequency",
			line:  `      waker-10   (   10) [000] .... 2.060000: clock_set_rate: pid_freq state=1800000 cpu_id=0`,
			want:  EventCPUFrequency,
			check: func(ev Event) bool { return ev.Frequency == 1800000 },
		},
		{
			name:  "block issue",
			line:  `      waker-10   (   10) [000] .... 2.070000: block_rq_issue: 8,0 R 4096 () 123 + 8 [worker]`,
			want:  EventBlockIssue,
			check: func(ev Event) bool { return ev.FieldText != "" },
		},
		{
			name:  "block complete",
			line:  `      waker-10   (   10) [000] .... 2.080000: block_rq_complete: 8,0 R () 123 + 8 [0]`,
			want:  EventBlockComplete,
			check: func(ev Event) bool { return ev.FieldText != "" },
		},
		{
			name:  "binder",
			line:  `      waker-10   (   10) [000] .... 2.090000: binder_transaction: transaction=7 dest_node=0 dest_proc=40 dest_thread=41 reply=1 flags=0x0 code=0x1`,
			want:  EventBinderTransaction,
			check: func(ev Event) bool { return ev.PID == 10 },
		},
		{
			name:  "irq",
			line:  `      waker-10   (   10) [000] .... 2.100000: irq_handler_entry: irq=32 name=kirq`,
			want:  EventIRQ,
			check: func(ev Event) bool { return ev.CPU == 0 },
		},
		{
			name:  "trace mark",
			line:  `      waker-10   (   10) [000] .... 2.120000: print: B|20|Choreographer#doFrame`,
			want:  EventTraceMark,
			check: func(ev Event) bool { return ev.SpanAction == "B" && ev.SpanName == "Choreographer#doFrame" },
		},
		{
			name:  "memory",
			line:  `      waker-10   (   10) [000] .... 2.110000: mm_vmscan_direct_reclaim_begin: order=0 may_writepage=1`,
			want:  EventMemory,
			check: func(ev Event) bool { return ev.Type == EventMemory },
		},
		{
			name:  "unknown ftrace row",
			line:  `      waker-10   (   10) [000] .... 2.130000: vendor_private_event: alpha=1`,
			want:  EventUnknown,
			check: func(ev Event) bool { return ev.FieldText == "" },
		},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, ok := ParseLine(i+10, tc.line, intern)
			if !ok || ev.Type != tc.want || !tc.check(ev) {
				t.Fatalf("ParseLine() = %+v ok=%v, want type %s", ev, ok, tc.want)
			}
		})
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

func TestThreadTimelineClassifiesDState(t *testing.T) {
	idx := buildTraceIndex(t, "resource.systrace", resourceTrace)
	tl := ThreadTimeline(idx, Query{PID: 30, TimeStart: 2.02, TimeEnd: 2.16, MinDurationMs: 1})
	if !timelineHasState(tl, StateDSleep) {
		t.Fatalf("expected D-state interval, got %+v", tl.Intervals)
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

func TestWakeupChainReportsMissingWakeup(t *testing.T) {
	idx := buildSampleIndex(t)
	chain := BuildWakeupChain(idx, Query{PID: 10, TimeStart: 1.05, TimeEnd: 1.17, MaxDepth: 4, MinDurationMs: 1})
	if len(chain.RootEvidence) == 0 || chain.RootEvidence[0].Type != "missing_wakeup" {
		t.Fatalf("expected missing wakeup root, got %+v", chain.RootEvidence)
	}
}

func TestWakeupChainReportsDStateRoot(t *testing.T) {
	idx := buildTraceIndex(t, "resource.systrace", resourceTrace)
	chain := BuildWakeupChain(idx, Query{PID: 30, TimeStart: 2.03, TimeEnd: 2.15, MaxDepth: 4, MinDurationMs: 1})
	if len(chain.RootEvidence) == 0 || chain.RootEvidence[0].Type != "d_state_or_io_wait" {
		t.Fatalf("expected D-state root, got %+v", chain.RootEvidence)
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

func TestWindowStatsCountsRuntimeResourcesAndOffCPU(t *testing.T) {
	idx := buildTraceIndex(t, "resource.systrace", resourceTrace)
	stats := ComputeWindowStats(idx, Query{TimeStart: 2.0, TimeEnd: 2.2})
	if stats.BlockIssueCount != 1 || stats.BlockCompleteCount != 1 || stats.BinderCount != 1 || stats.IRQCount != 1 || stats.MemoryEventCount != 1 {
		t.Fatalf("resource counts not preserved: %+v", stats)
	}
	if len(stats.RunnableTop) == 0 || stats.RunnableTop[0].Thread.PID != 20 {
		t.Fatalf("expected runnable top for main thread: %+v", stats.RunnableTop)
	}
	if len(stats.DStateTop) == 0 || stats.DStateTop[0].Thread.PID != 30 {
		t.Fatalf("expected D-state top for worker thread: %+v", stats.DStateTop)
	}
	foundFreq := false
	for _, cpu := range stats.CPU {
		if cpu.CPU == 0 && cpu.Frequency == 1800000 {
			foundFreq = true
		}
	}
	if !foundFreq {
		t.Fatalf("expected cpu frequency to be summarized: %+v", stats.CPU)
	}
}

func TestWindowStatsHonorsLineWindow(t *testing.T) {
	idx := buildTraceIndex(t, "resource.systrace", resourceTrace)
	stats := ComputeWindowStats(idx, Query{TimeStart: 2.0, TimeEnd: 2.2, LineStart: 1, LineEnd: 4})
	if stats.BlockIssueCount != 0 || stats.BinderCount != 0 || stats.IRQCount != 0 || stats.MemoryEventCount != 0 {
		t.Fatalf("line window should exclude later resource rows: %+v", stats)
	}
}

func buildSampleIndex(t *testing.T) *Index {
	t.Helper()
	return buildTraceIndex(t, "sample.systrace", sampleTrace)
}

func timelineHasState(tl TimelineResult, state ThreadState) bool {
	for _, it := range tl.Intervals {
		if it.State == state {
			return true
		}
	}
	return false
}

func buildTraceIndex(t *testing.T, name, content string) *Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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
