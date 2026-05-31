package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
      waker-10   (   10) [000] .... 2.040000: sched_blocked_reason: pid=30 iowait=1 caller=fscache_page_wait_on_page_bit
      waker-10   (   10) [000] .... 2.050000: cpu_idle: state=4294967295 cpu_id=0
      waker-10   (   10) [000] .... 2.060000: clock_set_rate: pid_freq state=1800000 cpu_id=0
      waker-10   (   10) [000] .... 2.070000: block_rq_issue: 8,0 R 4096 () 123 + 8 [worker]
      waker-10   (   10) [000] .... 2.080000: block_rq_complete: 8,0 R () 123 + 8 [0]
      waker-10   (   10) [000] .... 2.090000: binder_transaction: transaction=7 dest_node=0 dest_proc=40 dest_thread=41 reply=1 flags=0x0 code=0x1
      waker-10   (   10) [000] .... 2.095000: binder_transaction_received: transaction=7
      waker-10   (   10) [000] .... 2.100000: irq_handler_entry: irq=32 name=kirq
      waker-10   (   10) [000] .... 2.110000: mm_vmscan_direct_reclaim_begin: order=0 may_writepage=1
      waker-10   (   10) [000] .... 2.120000: print: B|20|Choreographer#doFrame
     worker-30   (   30) [001] .... 2.150000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=30 next_prio=20
       main-20   (   20) [001] .... 2.200000: sched_switch: prev_comm=worker prev_pid=30 prev_prio=20 prev_state=S ==> next_comm=main next_pid=20 next_prio=53
`

const ipcTrace = `
     client-20   (   20) [001] .... 3.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=client next_pid=20 next_prio=53
     client-20   (   20) [001] .... 3.010000: binder_transaction: transaction=42 dest_node=0 dest_proc=100 dest_thread=101 reply=1 flags=0x0 code=0x3
 binder:100_1-101 (  100) [002] .... 3.012000: binder_transaction_received: transaction=42
     client-20   (   20) [001] .... 3.015000: sched_switch: prev_comm=client prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
 binder:100_1-101 (  100) [002] .... 3.020000: sched_wakeup: comm=client pid=20 prio=53 target_cpu=001
     client-20   (   20) [001] .... 3.030000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=client next_pid=20 next_prio=53
`

const frequencyTrace = `
 arch_disk_io_2-33642 (33566) [001] .... 2940.180000: cpu_frequency: state=1800000 cpu_id=11
 arch_disk_io_2-33642 (33566) [001] .... 2940.190402: cpu_frequency: state=2228000 cpu_id=11
 arch_disk_io_2-33642 (33566) [001] .... 2940.190402: clock_set_rate: heca_info state=87047 cpu_id=11
	com.tencent.mm-36379 (36379) [010] .... 2940.190402: mm_filemap_add_to_page_cache: dev 260:84 ino 0x60ffe page=0000000000000000 pfn=3062260 ofs=0
 arch_disk_io_2-33642 (33566) [001] .... 2940.190402: clock_set_rate: heca_ddr_freq state=3744 cpu_id=0
 binder:31963_1-37072 (37047) [001] .... 2941.665123: cpu_frequency: state=1600000 cpu_id=0
 binder:31963_1-37072 (37047) [001] .... 2941.675123: cpu_frequency: state=1800000 cpu_id=0
`

const harmonyTrace = `
     OS_FFRT_0_0-49634 (48679) [000] .... 928.081774: block_rq_issue: 12,48 RS 4096 () 66637712 + 8 [OS_FFRT_0_0]
     OS_FFRT_0_0-49634 (48679) [000] .... 928.081786: mm_filemap_add_to_page_cache: dev 260:84 ino 0x1 page=0000000000000000 pfn=2477336 ofs=1211162624
     OS_FFRT_0_0-49634 (48679) [000] .... 928.081795: block_bio_remap: 12,48  66637568 + 8 <- (260,84) 14962432
     OS_FFRT_0_0-49634 (48679) [000] .... 928.081798: block_rq_issue: 12,48 RS 4096 () 66637568 + 8 [OS_FFRT_0_0]
     OS_FFRT_0_0-49634 (48679) [000] .... 928.081847: irq_handler_entry: irq=1507 name=kirq
     OS_FFRT_0_0-49634 (48679) [000] .... 928.081851: sched_wakeup: comm=udk-irq-0 pid=73 prio=301 target_cpu=000
       udk-irq-0-73    (    2) [000] .... 928.081861: sched_switch: prev_comm=OS_FFRT_0_0 prev_pid=49634 prev_prio=20 prev_state=R+ ==> next_comm=udk-irq-0 next_pid=73 next_prio=301
       udk-irq-0-73    (    2) [000] .... 928.081873: block_rq_complete: 12,48 RS () 66637712 + 8 [0]
       udk-irq-0-73    (    2) [000] .... 928.081896: irq_handler_exit: irq=1507 ret=handled
     OS_FFRT_0_0-49634 (48679) [000] .... 928.081903: sched_switch: prev_comm=udk-irq-0 prev_pid=73 prev_prio=301 prev_state=S ==> next_comm=OS_FFRT_0_0 next_pid=49634 next_prio=20
`

const p1ResourceTrace = `
       main-20   (   20) [001] .... 4.000000: print: B|20|Choreographer#doFrame
       main-20   (   20) [001] .... 4.010000: print: C|20|JNI Weak Global Refs|198
       main-20   (   20) [001] .... 4.015000: irq_handler_entry: irq=32 name=kirq
       main-20   (   20) [001] .... 4.015400: irq_handler_exit: irq=32 ret=handled
       main-20   (   20) [001] .... 4.015700: irq_handler_entry: irq=32 name=kirq
       main-20   (   20) [001] .... 4.016000: mm_filemap_add_to_page_cache: dev 260:84 ino 0x1 page=0 pfn=1 ofs=0
       main-20   (   20) [001] .... 4.017000: mm_vmscan_direct_reclaim_begin: order=0 may_writepage=1
       main-20   (   20) [001] .... 4.020000: print: E|20
      other-20   (   21) [002] .... 4.025000: sched_wakeup: comm=main pid=20 prio=53 target_cpu=001
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
			name: "blocked reason",
			line: `      waker-10   (   10) [000] .... 2.040000: sched_blocked_reason: pid=30 iowait=1 caller=fscache_page_wait_on_page_bit`,
			want: EventSchedBlockedReason,
			check: func(ev Event) bool {
				return ev.WakeePID == 30 && ev.IOWait == 1 && ev.Reason == "fscache_page_wait_on_page_bit"
			},
		},
		{
			name:  "cpu idle",
			line:  `      waker-10   (   10) [000] .... 2.050000: cpu_idle: state=4294967295 cpu_id=0`,
			want:  EventCPUIdle,
			check: func(ev Event) bool { return ev.CPUForField == 0 && ev.CPUForFieldValid },
		},
		{
			name:  "cpu frequency",
			line:  `      waker-10   (   10) [000] .... 2.060000: clock_set_rate: pid_freq state=1800000 cpu_id=0`,
			want:  EventCPUFrequency,
			check: func(ev Event) bool { return ev.Frequency == 1800000 && ev.ClockName == "pid_freq" },
		},
		{
			name:  "cpu_frequency tracepoint",
			line:  `      waker-10   (   10) [000] .... 2.061000: cpu_frequency: state=1600000 cpu_id=0`,
			want:  EventCPUFrequency,
			check: func(ev Event) bool { return ev.Frequency == 1600000 && ev.ClockName == "cpu_frequency" },
		},
		{
			name:  "non CPU clock set rate",
			line:  `      waker-10   (   10) [000] .... 2.062000: clock_set_rate: heca_ddr_freq state=3744 cpu_id=0`,
			want:  EventClockSetRate,
			check: func(ev Event) bool { return ev.Frequency == 3744 && ev.ClockName == "heca_ddr_freq" },
		},
		{
			name: "block issue",
			line: `      waker-10   (   10) [000] .... 2.070000: block_rq_issue: 8,0 R 4096 () 123 + 8 [worker]`,
			want: EventBlockIssue,
			check: func(ev Event) bool {
				return ev.BlockDev == "8,0" && ev.BlockOp == "R" && ev.BlockSector == 123 && ev.BlockLen == 8
			},
		},
		{
			name: "block complete",
			line: `      waker-10   (   10) [000] .... 2.080000: block_rq_complete: 8,0 R () 123 + 8 [0]`,
			want: EventBlockComplete,
			check: func(ev Event) bool {
				return ev.BlockDev == "8,0" && ev.BlockOp == "R" && ev.BlockSector == 123 && ev.BlockLen == 8
			},
		},
		{
			name:  "block remap",
			line:  `      waker-10   (   10) [000] .... 2.085000: block_bio_remap: 12,48  66637568 + 8 <- (260,84) 14962432`,
			want:  EventBlockRemap,
			check: func(ev Event) bool { return ev.BlockDev == "12,48" && ev.BlockSector == 66637568 && ev.BlockLen == 8 },
		},
		{
			name: "binder",
			line: `      waker-10   (   10) [000] .... 2.090000: binder_transaction: transaction=7 dest_node=0 dest_proc=40 dest_thread=41 reply=1 flags=0x0 code=0x1`,
			want: EventBinderTransaction,
			check: func(ev Event) bool {
				return ev.PID == 10 && ev.BinderTransactionID == 7 && ev.BinderDestProc == 40 && ev.BinderDestThread == 41 && ev.BinderReply == 1 && ev.BinderFlags == "0x0" && ev.BinderCode == "0x1"
			},
		},
		{
			name:  "binder received",
			line:  `      waker-10   (   10) [000] .... 2.095000: binder_transaction_received: transaction=7`,
			want:  EventBinderReceived,
			check: func(ev Event) bool { return ev.FieldText == "transaction=7" && ev.BinderTransactionID == 7 },
		},
		{
			name:  "irq",
			line:  `      waker-10   (   10) [000] .... 2.100000: irq_handler_entry: irq=32 name=kirq`,
			want:  EventIRQ,
			check: func(ev Event) bool { return ev.CPU == 0 && ev.IRQID == 32 && ev.IRQName == "kirq" },
		},
		{
			name:  "trace mark",
			line:  `      waker-10   (   10) [000] .... 2.120000: print: B|20|Choreographer#doFrame`,
			want:  EventTraceMark,
			check: func(ev Event) bool { return ev.SpanAction == "B" && ev.SpanName == "Choreographer#doFrame" },
		},
		{
			name: "trace counter",
			line: `      waker-10   (   10) [000] .... 2.125000: print: C|31963|JNI Weak Global Refs|198`,
			want: EventTraceMark,
			check: func(ev Event) bool {
				return ev.SpanAction == "C" && ev.SpanName == "JNI Weak Global Refs" && ev.SpanValue == "198"
			},
		},
		{
			name:  "memory",
			line:  `      waker-10   (   10) [000] .... 2.110000: mm_vmscan_direct_reclaim_begin: order=0 may_writepage=1`,
			want:  EventMemory,
			check: func(ev Event) bool { return ev.MemoryKind == "reclaim" },
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

func TestParseHarmonyHitraceSnippetKeepsSecondsAndPrioritySemantics(t *testing.T) {
	idx := buildTraceIndex(t, "harmony.systrace", harmonyTrace)
	if idx.FirstTs != 928.081774 {
		t.Fatalf("timestamp should stay in trace seconds, got %.6f", idx.FirstTs)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 928.081774, TimeEnd: 928.081903})
	if stats.BlockIssueCount != 2 || stats.BlockRemapCount != 1 || stats.BlockCompleteCount != 1 || stats.IRQCount != 2 || stats.MemoryEventCount != 1 {
		t.Fatalf("Harmony resource rows not summarized: %+v", stats)
	}
	foundCFS := false
	foundSystem := false
	for _, td := range stats.RunnableTop {
		if td.Thread.PID == 49634 && td.Priority == 20 && td.PriorityClass == "ohos_cfs" {
			foundCFS = true
		}
	}
	for _, td := range stats.TopRunning {
		if td.Thread.PID == 73 && td.Priority == 301 && td.PriorityClass == "system_or_kernel" {
			foundSystem = true
		}
	}
	if !foundCFS || !foundSystem {
		t.Fatalf("priority classes not preserved: running=%+v runnable=%+v", stats.TopRunning, stats.RunnableTop)
	}
	res := Run(idx, Query{View: "window_stats", TimeStart: 928.081774, TimeEnd: 928.081903})
	if res.TimeUnit != "seconds" || !strings.Contains(res.PrioritySemantics, "1-40=CFS") {
		t.Fatalf("result should describe trace time unit and Harmony priority semantics: %+v", res)
	}
}

func TestTraceFlavorDetectionAndPrioritySemantics(t *testing.T) {
	harmony := buildTraceIndex(t, "sample.htrace", harmonyTrace)
	harmonyRes := Run(harmony, Query{View: "window_stats", TimeStart: 928.081774, TimeEnd: 928.081903})
	if harmonyRes.TraceFlavor != string(TraceFlavorHarmonyHitrace) || !strings.Contains(harmonyRes.PrioritySemantics, "1-40=CFS") {
		t.Fatalf("Harmony trace should use Harmony priority semantics: %+v", harmonyRes)
	}

	android := buildTraceIndex(t, "sample.atrace", strings.Join([]string{
		` system_server-1000 (1000) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=system_server next_pid=1000 next_prio=98`,
		` SurfaceFlinger-2000 (2000) [001] .... 1.010000: sched_wakeup: comm=system_server pid=1000 prio=98 target_cpu=000`,
	}, "\n"))
	androidRes := Run(android, Query{View: "window_stats", TimeStart: 1.0, TimeEnd: 1.02})
	if androidRes.TraceFlavor != string(TraceFlavorAndroidAtrace) || strings.Contains(androidRes.PrioritySemantics, "1-40=CFS") {
		t.Fatalf("Android trace should not use Harmony priority semantics: %+v", androidRes)
	}
	if len(androidRes.WindowStats.TopRunning) == 0 || androidRes.WindowStats.TopRunning[0].PriorityClass != "android_raw_scheduler_prio" {
		t.Fatalf("Android priorities should stay raw: %+v", androidRes.WindowStats.TopRunning)
	}

	generic := buildTraceIndex(t, "sample.systrace", sampleTrace)
	genericRes := Run(generic, Query{View: "window_stats", TimeStart: 1.0, TimeEnd: 1.3})
	if genericRes.TraceFlavor != string(TraceFlavorGenericFtrace) || !strings.Contains(genericRes.PrioritySemantics, "raw scheduler priority") {
		t.Fatalf("ambiguous systrace should fall back to generic semantics: %+v", genericRes)
	}
}

func TestExplicitTraceFlavorOverrideWins(t *testing.T) {
	idx := buildTraceIndex(t, "sample.htrace", harmonyTrace)
	res := Run(idx, Query{
		View:                  "window_stats",
		TimeStart:             928.081774,
		TimeEnd:               928.081903,
		TraceFlavorHint:       TraceFlavorAndroidAtrace,
		TraceFlavorHintSource: "tool_param",
	})
	if res.TraceFlavor != string(TraceFlavorAndroidAtrace) || res.FlavorConfidence != 1.0 {
		t.Fatalf("explicit tool flavor should win: %+v", res)
	}
	if strings.Contains(res.PrioritySemantics, "1-40=CFS") {
		t.Fatalf("explicit Android flavor must not render Harmony semantics: %s", res.PrioritySemantics)
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
	if !timelineHasState(tl, StateIOWait) {
		t.Fatalf("expected IO-wait interval enriched from sched_blocked_reason, got %+v", tl.Intervals)
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

func TestIPCGraphMatchesBinderSendAndReceive(t *testing.T) {
	idx := buildTraceIndex(t, "ipc.systrace", ipcTrace)
	ipc := BuildIPCGraph(idx, Query{PID: 20, TimeStart: 3.0, TimeEnd: 3.04, Limit: 10})
	if len(ipc.Edges) != 1 {
		t.Fatalf("expected one IPC edge, got %+v", ipc)
	}
	edge := ipc.Edges[0]
	if edge.TransactionID != 42 || edge.Sender.PID != 20 || edge.Receiver.PID != 101 || edge.Receiver.TGID != 100 || edge.SendLine == 0 || edge.ReceiveLine == 0 {
		t.Fatalf("bad IPC edge: %+v", edge)
	}
	if edge.Confidence < 0.9 || edge.LatencyMs <= 0 {
		t.Fatalf("expected matched receive confidence and latency: %+v", edge)
	}
}

func TestWakeupChainCarriesRelevantIPCEdges(t *testing.T) {
	idx := buildTraceIndex(t, "ipc.systrace", ipcTrace)
	chain := BuildWakeupChain(idx, Query{PID: 20, TimeStart: 3.0, TimeEnd: 3.04, MaxDepth: 4, MinDurationMs: 1})
	if len(chain.IPCEdges) != 1 || chain.IPCEdges[0].TransactionID != 42 {
		t.Fatalf("expected wakeup chain to carry target IPC edge, got %+v", chain.IPCEdges)
	}
}

func TestWakeupChainReportsBinderWaitCandidate(t *testing.T) {
	idx := buildTraceIndex(t, "ipc.systrace", ipcTrace)
	chain := BuildWakeupChain(idx, Query{PID: 20, TimeStart: 3.0, TimeEnd: 3.04, MaxDepth: 4, MinDurationMs: 1})
	if len(chain.BinderWaits) != 1 {
		t.Fatalf("expected binder wait candidate, got %+v", chain.BinderWaits)
	}
	wait := chain.BinderWaits[0]
	if wait.TransactionID != 42 || wait.SendLine == 0 || wait.SleepLine == 0 || wait.Confidence <= 0 {
		t.Fatalf("bad binder wait: %+v", wait)
	}
	foundRoot := false
	for _, root := range chain.RootEvidence {
		if root.Type == "binder_wait" {
			foundRoot = true
		}
	}
	if !foundRoot {
		t.Fatalf("binder wait should be carried as root evidence candidate: %+v", chain.RootEvidence)
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
	if len(chain.RootEvidence) == 0 || chain.RootEvidence[0].Type != "io_wait" || !strings.Contains(chain.RootEvidence[0].Summary, "fscache_page_wait_on_page_bit") {
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
	if stats.BlockIssueCount != 1 || stats.BlockCompleteCount != 1 || stats.BinderCount != 2 || stats.BinderReceivedCount != 1 || stats.IRQCount != 1 || stats.MemoryEventCount != 1 || stats.BlockedReasonCount != 1 || stats.IOWaitBlockedCount != 1 {
		t.Fatalf("resource counts not preserved: %+v", stats)
	}
	if len(stats.BlockedReasons) == 0 || stats.BlockedReasons[0].Thread.PID != 30 || stats.BlockedReasons[0].IOWait != 1 || !strings.Contains(stats.BlockedReasons[0].Reason, "fscache") {
		t.Fatalf("blocked reason not summarized: %+v", stats.BlockedReasons)
	}
	if len(stats.RunnableTop) == 0 || stats.RunnableTop[0].Thread.PID != 20 {
		t.Fatalf("expected runnable top for main thread: %+v", stats.RunnableTop)
	}
	if stats.RunnableTop[0].CPU != 1 {
		t.Fatalf("expected runnable top to keep CPU context: %+v", stats.RunnableTop[0])
	}
	if len(stats.DStateTop) == 0 || stats.DStateTop[0].Thread.PID != 30 {
		t.Fatalf("expected D-state top for worker thread: %+v", stats.DStateTop)
	}
	if len(stats.IOLatencies) != 1 || stats.IOLatencies[0].DurationMs <= 0 || stats.IOLatencies[0].IssueLine == 0 || stats.IOLatencies[0].CompleteLine == 0 {
		t.Fatalf("expected paired IO latency: %+v", stats.IOLatencies)
	}
	if len(stats.CPUPressure) == 0 {
		t.Fatalf("expected CPU pressure stats: %+v", stats.CPUPressure)
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

func TestWindowStatsComputesCPUFrequencyResidency(t *testing.T) {
	idx := buildTraceIndex(t, "freq.systrace", frequencyTrace)
	stats := ComputeWindowStats(idx, Query{TimeStart: 2940.185000, TimeEnd: 2941.680123})
	cpu11 := findCPUStats(stats, 11)
	if cpu11 == nil || cpu11.Frequency != 2228000 {
		t.Fatalf("expected CPU11 latest frequency from cpu_frequency row, got %+v", cpu11)
	}
	if len(cpu11.FrequencyResidency) != 2 {
		t.Fatalf("expected CPU11 residency before and after switch, got %+v", cpu11.FrequencyResidency)
	}
	if cpu11.FrequencyResidency[0].Frequency != 1800000 || !near(cpu11.FrequencyResidency[0].DurationMs, 5.402, 0.001) {
		t.Fatalf("bad pre-window carried residency: %+v", cpu11.FrequencyResidency[0])
	}
	if cpu11.FrequencyResidency[1].Frequency != 2228000 || cpu11.FrequencyResidency[1].DurationMs <= 0 {
		t.Fatalf("bad switched residency: %+v", cpu11.FrequencyResidency[1])
	}
	cpu0 := findCPUStats(stats, 0)
	if cpu0 == nil || cpu0.Frequency != 1800000 || len(cpu0.FrequencyResidency) != 2 {
		t.Fatalf("expected CPU0 residency from real cpu_frequency rows only, got %+v", cpu0)
	}
	for _, item := range cpu0.FrequencyResidency {
		if item.Frequency == 3744 {
			t.Fatalf("DDR clock_set_rate leaked into CPU frequency residency: %+v", cpu0.FrequencyResidency)
		}
	}
}

func TestWindowStatsComputesP1ResourceSummaries(t *testing.T) {
	idx := buildTraceIndex(t, "p1.systrace", p1ResourceTrace)
	stats := ComputeWindowStats(idx, Query{TimeStart: 4.0, TimeEnd: 4.03})
	if len(stats.TraceSpans) != 1 || stats.TraceSpans[0].Name != "Choreographer#doFrame" || !near(stats.TraceSpans[0].DurationMs, 20.0, 0.001) {
		t.Fatalf("expected span duration: %+v", stats.TraceSpans)
	}
	if len(stats.TraceCounters) != 1 || stats.TraceCounters[0].Name != "JNI Weak Global Refs" || stats.TraceCounters[0].Value != "198" {
		t.Fatalf("expected trace counter: %+v", stats.TraceCounters)
	}
	if len(stats.IRQBursts) == 0 || stats.IRQBursts[0].IRQ != 32 || stats.IRQBursts[0].Count < 2 {
		t.Fatalf("expected IRQ burst: %+v", stats.IRQBursts)
	}
	if len(stats.MemoryKinds) != 2 {
		t.Fatalf("expected memory kinds: %+v", stats.MemoryKinds)
	}
	kinds := map[string]bool{}
	for _, item := range stats.MemoryKinds {
		kinds[item.Kind] = true
	}
	if !kinds["page_cache"] || !kinds["reclaim"] {
		t.Fatalf("expected page_cache and reclaim kinds: %+v", stats.MemoryKinds)
	}
	if len(stats.ThreadDrifts) == 0 || stats.ThreadDrifts[0].PID != 20 {
		t.Fatalf("expected pid/name drift caveat: %+v", stats.ThreadDrifts)
	}
}

func TestWindowStatsHonorsLineWindow(t *testing.T) {
	idx := buildTraceIndex(t, "resource.systrace", resourceTrace)
	stats := ComputeWindowStats(idx, Query{TimeStart: 2.0, TimeEnd: 2.2, LineStart: 1, LineEnd: 4})
	if stats.BlockIssueCount != 0 || stats.BinderCount != 0 || stats.IRQCount != 0 || stats.MemoryEventCount != 0 {
		t.Fatalf("line window should exclude later resource rows: %+v", stats)
	}
}

func findCPUStats(stats WindowStats, cpu int) *CPUStats {
	for i := range stats.CPU {
		if stats.CPU[i].CPU == cpu {
			return &stats.CPU[i]
		}
	}
	return nil
}

func near(got, want, delta float64) bool {
	if got < want {
		return want-got <= delta
	}
	return got-want <= delta
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
