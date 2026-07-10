package tracequery

import (
	"reflect"
	"strings"
	"testing"
)

func TestThreadMatchesKnownPIDNeverFallsBackToComm(t *testing.T) {
	tests := []struct {
		name string
		ref  ThreadRef
		pid  int
		comm string
		want bool
	}{
		{
			name: "same pid survives rename",
			ref:  ThreadRef{Comm: "old-name", PID: 100},
			pid:  100,
			comm: "new-name",
			want: true,
		},
		{
			name: "same comm on another pid is not the target",
			ref:  ThreadRef{Comm: "worker", PID: 100},
			pid:  300,
			comm: "worker",
			want: false,
		},
		{
			name: "known target pid cannot match a pidless name row",
			ref:  ThreadRef{Comm: "worker", PID: 100},
			pid:  0,
			comm: "worker",
			want: false,
		},
		{
			name: "comm fallback remains for a genuinely pidless target",
			ref:  ThreadRef{Comm: "worker"},
			pid:  300,
			comm: "WORKER",
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := threadMatches(tc.ref, tc.pid, tc.comm); got != tc.want {
				t.Fatalf("threadMatches(%+v, %d, %q) = %v, want %v", tc.ref, tc.pid, tc.comm, got, tc.want)
			}
		})
	}
}

func TestThreadTimelineKnownPIDRejectsSameCommEventsFromAnotherPID(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1.000, Type: EventSchedSwitch, PrevComm: "worker", PrevPID: 100, PrevState: "S", NextComm: "idle/0"},
		// These three rows describe a different worker with the same comm. The
		// legacy PID-or-comm matcher closed and reopened pid=100's intervals on
		// these timestamps.
		{Line: 2, Ts: 1.005, Type: EventSchedWakeup, Comm: "waker-300", PID: 500, WakeeComm: "worker", WakeePID: 300},
		{Line: 3, Ts: 1.006, Type: EventSchedSwitch, PrevComm: "idle/1", NextComm: "worker", NextPID: 300},
		{Line: 4, Ts: 1.007, Type: EventSchedSwitch, PrevComm: "worker", PrevPID: 300, PrevState: "S", NextComm: "idle/1"},
		{Line: 5, Ts: 1.020, Type: EventSchedWakeup, Comm: "real-waker", PID: 501, WakeeComm: "worker", WakeePID: 100},
		{Line: 6, Ts: 1.030, Type: EventSchedSwitch, PrevComm: "idle/0", NextComm: "worker", NextPID: 100},
	}, FirstTs: 1.000, LastTs: 1.030}

	got := ThreadTimeline(idx, Query{PID: 100, TimeStart: 1.000, TimeEnd: 1.030})
	if got.Thread.PID != 100 {
		t.Fatalf("timeline target = %+v, want pid=100", got.Thread)
	}
	if len(got.Intervals) != 3 {
		t.Fatalf("pid=300 rows must not split pid=100's sleep/runnable interval; got %+v", got.Intervals)
	}
	if got.Intervals[0].State != StateSSleep || got.Intervals[0].StartTs != 1.000 || got.Intervals[0].EndTs != 1.020 ||
		got.Intervals[1].State != StateRunnable || got.Intervals[1].StartTs != 1.020 || got.Intervals[1].EndTs != 1.030 ||
		got.Intervals[2].State != StateRunning || got.Intervals[2].StartTs != 1.030 || got.Intervals[2].EndTs != 1.030 {
		t.Fatalf("timeline must be formed only from pid=100 rows, got %+v", got.Intervals)
	}
	for _, interval := range got.Intervals {
		if interval.Thread.PID != 100 || interval.StartLine == 2 || interval.StartLine == 3 || interval.StartLine == 4 ||
			interval.EndLine == 2 || interval.EndLine == 3 || interval.EndLine == 4 {
			t.Fatalf("same-comm pid=300 row contaminated target timeline: %+v", interval)
		}
	}
}

func TestSchedulerLatencyKnownPIDRejectsSameCommWaitFromAnotherPID(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 2.000, Type: EventSchedWakeup, Comm: "waker-a", PID: 500, WakeeComm: "worker", WakeePID: 300, WakeePrio: 50, TargetCPU: 1},
		{Line: 2, Ts: 2.010, Type: EventSchedSwitch, CPU: 1, PrevComm: "idle/1", NextComm: "worker", NextPID: 300, NextPrio: 50},
		{Line: 3, Ts: 2.020, Type: EventSchedWakeup, Comm: "waker-b", PID: 501, WakeeComm: "worker", WakeePID: 100, WakeePrio: 50, TargetCPU: 0},
		{Line: 4, Ts: 2.030, Type: EventSchedSwitch, CPU: 0, PrevComm: "idle/0", NextComm: "worker", NextPID: 100, NextPrio: 50},
	}, FirstTs: 2.000, LastTs: 2.030}

	got := BuildSchedulerLatencyStats(idx, Query{PID: 100, TimeStart: 2.000, TimeEnd: 2.030, MinDurationMs: 1})
	if len(got.Items) != 1 || got.Items[0].Thread.PID != 100 {
		t.Fatalf("target-specific latency must contain only pid=100, got %+v", got.Items)
	}
}

func TestInteractionStatsKnownPIDRejectsSameCommIncomingAndOutgoingRows(t *testing.T) {
	idx := &Index{Events: []Event{
		// Same comm, wrong emitter PID: not an outgoing interaction from target.
		{Line: 1, Ts: 3.001, Type: EventSchedWakeup, Comm: "worker", PID: 300, WakeeComm: "wrong-out", WakeePID: 400},
		// Same wakee comm, wrong wakee PID: not an incoming interaction to target.
		{Line: 2, Ts: 3.002, Type: EventSchedWakeup, Comm: "wrong-in", PID: 500, WakeeComm: "worker", WakeePID: 300},
		{Line: 3, Ts: 3.003, Type: EventSchedWakeup, Comm: "worker", PID: 100, WakeeComm: "true-out", WakeePID: 401},
		{Line: 4, Ts: 3.004, Type: EventSchedWakeup, Comm: "true-in", PID: 501, WakeeComm: "worker", WakeePID: 100},
	}, FirstTs: 3.001, LastTs: 3.004}

	got := BuildInteractionStats(idx, Query{PID: 100, TimeStart: 3.000, TimeEnd: 3.010, InteractionDirection: "both"})
	if len(got.Items) != 2 {
		t.Fatalf("same-comm pid=300 interactions must be excluded, got %+v", got.Items)
	}
	seen := map[int]InteractionSummary{}
	for _, item := range got.Items {
		seen[item.Peer.PID] = item
		if item.Peer.PID == 400 || item.Peer.PID == 500 {
			t.Fatalf("wrong-PID same-comm interaction leaked: %+v", item)
		}
	}
	if seen[401].WakeupsFromTarget != 1 || seen[501].WakeupsToTarget != 1 {
		t.Fatalf("true pid=100 interactions missing: %+v", got.Items)
	}
}

func TestIPCGraphNumericSelectorIsExactTIDNotProcessOrDestProc(t *testing.T) {
	idx := &Index{Events: []Event{{
		Line: 1, Ts: 4.0, Type: EventBinderTransaction,
		Comm: "sender", PID: 22, TGID: 100,
		BinderFields: &BinderFields{TransactionID: 7, DestProc: 200, DestThread: 33},
	}}, FirstTs: 4, LastTs: 4}
	for _, processLike := range []int{100, 200} {
		got := BuildIPCGraph(idx, Query{PID: processLike, TimeStart: 3.9, TimeEnd: 4.1})
		if len(got.Edges) != 0 {
			t.Fatalf("pid=%d is a process/TGID endpoint, not an exact binder TID: %+v", processLike, got.Edges)
		}
	}
	for _, exactTID := range []int{22, 33} {
		got := BuildIPCGraph(idx, Query{PID: exactTID, TimeStart: 3.9, TimeEnd: 4.1})
		if len(got.Edges) != 1 {
			t.Fatalf("exact binder tid=%d must retain the edge: %+v", exactTID, got)
		}
	}
}

func TestIPCGraphNameSelectorFailsClosedOnSameNameTIDs(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 5.0, Type: EventBinderTransaction, Comm: "worker", PID: 22, BinderFields: &BinderFields{TransactionID: 1, DestThread: 40}},
		{Line: 2, Ts: 5.1, Type: EventBinderTransaction, Comm: "worker", PID: 23, BinderFields: &BinderFields{TransactionID: 2, DestThread: 41}},
	}, FirstTs: 5, LastTs: 5.1}
	got := BuildIPCGraph(idx, Query{Thread: "worker", ThreadInput: "worker", TimeStart: 4.9, TimeEnd: 5.2})
	if len(got.Edges) != 0 || !containsSubstring(got.Caveats, "thread_selector_ambiguous") {
		t.Fatalf("same-name binder TIDs must not merge under a name selector: %+v", got)
	}
}

func TestSpanWindowKnownPIDRejectsSameCommSpanFromAnotherPID(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 4.000, Type: EventTraceMark, Comm: "worker", PID: 300, SpanAction: "B", SpanName: "doWork"},
		{Line: 2, Ts: 4.010, Type: EventTraceMark, Comm: "worker", PID: 300, SpanAction: "E"},
		{Line: 3, Ts: 4.020, Type: EventTraceMark, Comm: "worker", PID: 100, SpanAction: "B", SpanName: "doWork"},
		{Line: 4, Ts: 4.030, Type: EventTraceMark, Comm: "worker", PID: 100, SpanAction: "E"},
	}, FirstTs: 4.000, LastTs: 4.030}

	spans, caveats := FindSpanWindows(idx, Query{PID: 100, SpanName: "doWork", TimeStart: 4.000, TimeEnd: 4.040}, 8)
	if len(caveats) != 0 {
		t.Fatalf("unexpected caveats: %v", caveats)
	}
	if len(spans) != 1 || spans[0].Thread.PID != 100 || spans[0].StartLine != 3 {
		t.Fatalf("same-comm pid=300 span must not match pid=100 selector, got %+v", spans)
	}
}

func TestResolveThreadNameSelectorFailsClosedOnMultiplePIDs(t *testing.T) {
	idx := &Index{Path: "ambiguous.systrace", Events: []Event{
		{Line: 1, Ts: 5.000, Type: EventSchedSwitch, Comm: "worker", PID: 100, TGID: 10, PrevComm: "worker", PrevPID: 100, PrevState: "S"},
		{Line: 2, Ts: 5.010, Type: EventSchedSwitch, Comm: "worker", PID: 300, TGID: 10, PrevComm: "worker", PrevPID: 300, PrevState: "S"},
	}, FirstTs: 5.000, LastTs: 5.010, LineCount: 2, ScannedLineCount: 2, ParsedKnown: 2}
	q := Query{View: "thread_timeline", Thread: "worker", ThreadInput: "worker", TimeStart: 5.000, TimeEnd: 5.020}

	resolution := resolveThreadSelection(idx, q)
	if !resolution.Ambiguous || resolution.Thread != (ThreadRef{}) || !reflect.DeepEqual(resolution.CandidatePIDs, []int{100, 300}) {
		t.Fatalf("same name on two tids (even inside one TGID) must be ambiguous, got %+v", resolution)
	}
	timeline := ThreadTimeline(idx, q)
	if timeline.Thread != (ThreadRef{}) || len(timeline.Intervals) != 0 || !containsSubstring(timeline.Caveats, "thread_selector_ambiguous") {
		t.Fatalf("ambiguous name must not choose the first row or merge tids, got %+v", timeline)
	}
	result := Run(idx, q)
	if !containsSubstring(result.Caveats, `thread_selector_ambiguous selector="worker" candidate_pids=100,300`) {
		t.Fatalf("top-level result must publish the deterministic ambiguity caveat, got %v", result.Caveats)
	}

	explicit := ThreadTimeline(idx, Query{Thread: "worker-300", ThreadInput: "worker-300", TimeStart: 5.000, TimeEnd: 5.020})
	if explicit.Thread.PID != 300 {
		t.Fatalf("pid-bearing selector must resolve the requested tid exactly, got %+v", explicit.Thread)
	}
}

func TestResolveThreadNameUsesWindowAndExactNameBeforeSubstringFallback(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1.000, Type: EventSchedSwitch, Comm: "app", PID: 100, TGID: 100},
		{Line: 2, Ts: 1.010, Type: EventSchedSwitch, Comm: "app-helper", PID: 200, TGID: 100},
		// Exact duplicate outside the selected window must not poison a
		// bounded query whose local identity is unique.
		{Line: 3, Ts: 9.000, Type: EventSchedSwitch, Comm: "app", PID: 300, TGID: 300},
	}, FirstTs: 1.000, LastTs: 9.000}

	got := resolveThreadSelection(idx, Query{Thread: "app", TimeStart: 0.900, TimeEnd: 1.100})
	if got.Ambiguous || got.Thread.PID != 100 {
		t.Fatalf("window-local exact comm must beat substring and out-of-window candidates, got %+v", got)
	}
}

func TestHyphenNumericSelectorFailsClosedOnLiteralNameConflict(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1.000, Type: EventSchedSwitch, Comm: "worker-300", PID: 100, PrevComm: "worker-300", PrevPID: 100},
		{Line: 2, Ts: 1.010, Type: EventSchedSwitch, Comm: "other", PID: 300, PrevComm: "other", PrevPID: 300},
	}, FirstTs: 1.000, LastTs: 1.010}
	q := Query{Thread: "worker-300", ThreadInput: "worker-300", TimeStart: 0.900, TimeEnd: 1.100}
	got := resolveThreadSelection(idx, q)
	if !got.Ambiguous || !reflect.DeepEqual(got.CandidatePIDs, []int{100, 300}) {
		t.Fatalf("literal comm and rendered-label interpretations must fail closed, got %+v", got)
	}

	idx.Events = idx.Events[:1]
	got = resolveThreadSelection(idx, q)
	if got.Ambiguous || got.Thread.PID != 100 || got.Thread.Comm != "worker-300" {
		t.Fatalf("an observed literal numeric-suffix comm must win when rendered TID is absent, got %+v", got)
	}
}

func TestHyphenRenderedSelectorRemainsCompatible(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1.000, Type: EventSchedSwitch, Comm: "app", PID: 100, PrevComm: "app", PrevPID: 100},
	}, FirstTs: 1.000, LastTs: 1.000}
	got := resolveThreadSelection(idx, Query{Thread: "app-100", ThreadInput: "app-100", TimeStart: 0.900, TimeEnd: 1.100})
	if got.Ambiguous || got.Thread.PID != 100 {
		t.Fatalf("rendered comm-TID selector must retain compatibility, got %+v", got)
	}
}

func TestResolvePIDThreadKeepsExactPIDAndBackfillsTGID(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1.000, Type: EventSchedSwitch, PrevComm: "old-name", PrevPID: 100},
		{Line: 2, Ts: 1.010, Type: EventTraceMark, Comm: "new-name", PID: 100, TGID: 10},
	}}
	got := resolveThread(idx, Query{PID: 100})
	if got.PID != 100 || got.TGID != 10 || strings.TrimSpace(got.Comm) == "" {
		t.Fatalf("pid resolution must keep the exact tid while filling typed metadata, got %+v", got)
	}
	if threadKey(ThreadRef{Comm: "worker", PID: 100}) == threadKey(ThreadRef{Comm: "worker", PID: 300}) {
		t.Fatal("cycle/family keys must not merge same-comm threads with different positive PIDs")
	}
}
