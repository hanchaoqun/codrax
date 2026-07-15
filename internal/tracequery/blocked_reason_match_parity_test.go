package tracequery

import (
	"context"
	"math"
	"strconv"
	"strings"
	"testing"
)

func blockedMatchEvent(ts float64, line, pid int, io int32) Event {
	return Event{
		Type: EventSchedBlockedReason, Ts: ts, Line: line, WakeePID: pid,
		IOWait: io, BlockedReasonIOWaitKnown: true, Reason: "wait_object",
	}
}

func TestBlockedReasonMatcherOwnsOneInclusiveTolerance(t *testing.T) {
	thread := ThreadRef{Comm: "renamed-display-only", PID: 42}
	for _, tc := range []struct {
		name  string
		delta float64
		want  bool
	}{
		{name: "same", delta: 0, want: true},
		{name: "plus_1us", delta: 0.000001, want: true},
		{name: "plus_2us", delta: 0.000002, want: true},
		{name: "plus_5us_inclusive", delta: 0.000005, want: true},
		{name: "past_5us", delta: 0.000006, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := map[int][]Event{42: {blockedMatchEvent(2.0+tc.delta, 10, 42, 1)}}
			got := matchBlockedReasonForInterval(events, thread, 1.0, 2.0)
			if (got.Event != nil) != tc.want || got.Ambiguous {
				t.Fatalf("delta=%g match=%+v want=%t", tc.delta, got, tc.want)
			}
		})
	}

	wrongPID := map[int][]Event{42: {blockedMatchEvent(2.0, 11, 43, 1)}}
	if got := matchBlockedReasonForInterval(wrongPID, thread, 1, 2); got.Event != nil || got.Ambiguous {
		t.Fatalf("thread display/name must never override exact PID: %+v", got)
	}
	unknownIO := blockedMatchEvent(2.0, 12, 42, 1)
	unknownIO.BlockedReasonIOWaitKnown = false
	if got := matchBlockedReasonForInterval(map[int][]Event{42: {unknownIO}}, thread, 1, 2); got.Event != nil || got.Physical == nil || got.Ambiguous || got.Candidates != 1 {
		t.Fatalf("unknown iowait must not alias a typed marker: %+v", got)
	}
	validIO := blockedMatchEvent(2.000001, 13, 42, 1)
	if got := matchBlockedReasonForInterval(map[int][]Event{42: {unknownIO, validIO}}, thread, 1, 2); !got.Ambiguous || got.Event != nil || got.Candidates != 2 {
		t.Fatalf("opaque+valid physical markers must fail closed: %+v", got)
	}
}

func TestBlockedReasonMatcherTextualFiveMicrosecondBoundaryIsInclusive(t *testing.T) {
	intern := newStringInterner()
	end, ok := ParseLine(1, "waker-99 (99) [000] .... 13762.000003: sched_wakeup: comm=worker pid=42 prio=20 target_cpu=000", intern)
	if !ok {
		t.Fatal("failed to parse interval-end witness")
	}
	markerAtFive, ok := ParseLine(2, "waker-99 (99) [000] .... 13762.000008: sched_blocked_reason: pid=42 iowait=1 caller=wait_object delay=7", intern)
	if !ok {
		t.Fatal("failed to parse exact +5us marker")
	}
	markerAtSix, ok := ParseLine(3, "waker-99 (99) [000] .... 13762.000009: sched_blocked_reason: pid=42 iowait=1 caller=wait_object delay=7", intern)
	if !ok {
		t.Fatal("failed to parse +6us marker")
	}
	thread := ThreadRef{PID: 42}
	if got := matchBlockedReasonForInterval(map[int][]Event{42: {markerAtFive}}, thread, 13761, end.Ts); got.Event == nil || got.Ambiguous {
		t.Fatalf("independently parsed textual +5us boundary was rejected: end=%.15f marker=%.15f match=%+v", end.Ts, markerAtFive.Ts, got)
	}
	if got := matchBlockedReasonForInterval(map[int][]Event{42: {markerAtSix}}, thread, 13761, end.Ts); got.Event != nil || got.Ambiguous || got.Candidates != 0 {
		t.Fatalf("real textual +6us marker leaked through ULP allowance: end=%.15f marker=%.15f match=%+v", end.Ts, markerAtSix.Ts, got)
	}
}

func TestBlockedReasonMatcherFailsClosedOnAnyIntervalAmbiguity(t *testing.T) {
	thread := ThreadRef{PID: 42}
	events := map[int][]Event{42: {
		blockedMatchEvent(1.2, 10, 42, 0), // older legacy in-interval marker
		blockedMatchEvent(2.000001, 20, 42, 1),
		blockedMatchEvent(2.000002, 21, 42, 1),
	}}
	got := matchBlockedReasonForInterval(events, thread, 1, 2)
	if !got.Ambiguous || got.Event != nil || got.Candidates != 3 {
		t.Fatalf("two closing markers must fail closed, got %+v", got)
	}

	// A closing-boundary marker cannot erase an older interval-eligible marker.
	// Both are physical inventory; without typed request identity the causal
	// association is ambiguous and must fail closed.
	events[42] = events[42][:2]
	got = matchBlockedReasonForInterval(events, thread, 1, 2)
	if !got.Ambiguous || got.Event != nil || got.Candidates != 2 {
		t.Fatalf("older interval marker plus one closing marker must fail closed: %+v", got)
	}
}

func blockedReasonParityTrace(t *testing.T, extraMarker bool) string {
	t.Helper()
	lines := []string{
		"        idle-0 (    0) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
		"     worker-42 (   42) [001] .... 1.001000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=20 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120",
		"      waker-99 (   99) [000] .... 1.010000: sched_wakeup: comm=worker pid=42 prio=20 target_cpu=001",
		"      waker-99 (   99) [000] .... 1.010002: sched_blocked_reason: pid=42 iowait=1 caller=wait_object+0x1/0x2 delay=9",
	}
	if extraMarker {
		lines = append(lines, "      waker-99 (   99) [000] .... 1.010003: sched_blocked_reason: pid=42 iowait=1 caller=wait_object+0x1/0x2 delay=9")
	}
	lines = append(lines, "        idle-0 (    0) [001] .... 1.011000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20")
	return writeSchedulerIntegrityTrace(t, "blocked-marker-parity.ftrace", lines...)
}

func durationForPID(items []ThreadDuration, pid int) float64 {
	for _, item := range items {
		if item.Thread.PID == pid {
			return item.DurationMs
		}
	}
	return 0
}

func TestBlockedReasonTrailingMarkerIndexedTimelineAndStreamParity(t *testing.T) {
	path := blockedReasonParityTrace(t, false)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{PID: 42, TimeStart: 1.0, TimeEnd: 1.010, MinDurationMs: 0.01, Limit: 8}
	stats := ComputeWindowStats(idx, q)
	if got := durationForPID(stats.IOWaitTop, 42); math.Abs(got-9) > 0.001 {
		t.Fatalf("indexed trailing marker must classify 9ms as IO wait, got %.6f stats=%+v", got, stats)
	}
	if got := durationForPID(stats.DStateTop, 42); got != 0 {
		t.Fatalf("indexed D/IO lanes are mutually exclusive, D got %.6f", got)
	}
	timeline := ThreadTimeline(idx, q)
	foundIO := false
	for _, interval := range timeline.Intervals {
		foundIO = foundIO || interval.State == StateIOWait
	}
	if !foundIO {
		t.Fatalf("timeline must consume the same trailing marker: %+v", timeline.Intervals)
	}

	streamed, err := StreamStateCluster(context.Background(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	if streamed.WindowStats == nil {
		t.Fatal("stream state-cluster did not publish stats")
	}
	if got := durationForPID(streamed.WindowStats.IOWaitTop, 42); math.Abs(got-9) > 0.001 {
		t.Fatalf("stream trailing marker must classify 9ms as IO wait, got %.6f", got)
	}
	if got := durationForPID(streamed.WindowStats.DStateTop, 42); got != 0 {
		t.Fatalf("stream must not double-book IO wait into D-state, got %.6f", got)
	}
	if streamed.EventCount != 3 {
		t.Fatalf("the +2us closing marker is carry, not an in-window event: count=%d", streamed.EventCount)
	}
}

func TestBlockedReasonAmbiguityKeepsGenericDAndDiscloses(t *testing.T) {
	path := blockedReasonParityTrace(t, true)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	stats := ComputeWindowStats(idx, Query{PID: 42, TimeStart: 1, TimeEnd: 1.010, Limit: 8})
	if got := durationForPID(stats.IOWaitTop, 42); got != 0 {
		t.Fatalf("ambiguous markers must not mint IO wait, got %.6f", got)
	}
	if got := durationForPID(stats.DStateTop, 42); math.Abs(got-9) > 0.001 {
		t.Fatalf("ambiguous markers retain the generic D account, got %.6f", got)
	}
	if !containsSubstring(stats.Caveats, "blocked_reason_marker_ambiguous=true") {
		t.Fatalf("ambiguity must be disclosed: %+v", stats.Caveats)
	}
	timeline := ThreadTimeline(idx, Query{PID: 42, TimeStart: 1, TimeEnd: 1.010, Limit: 8})
	if !containsSubstring(timeline.Caveats, "blocked_reason_marker_ambiguous=true") {
		t.Fatalf("timeline ambiguity must be disclosed: %+v", timeline.Caveats)
	}
	streamed, err := StreamStateCluster(context.Background(), path, Query{PID: 42, TimeStart: 1, TimeEnd: 1.010, Limit: 8}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSubstring(streamed.Caveats, "blocked_reason_marker_ambiguous=true") {
		t.Fatalf("stream ambiguity must be disclosed: %+v", streamed.Caveats)
	}
}

func TestBlockedReasonMalformedPIDBindingParity(t *testing.T) {
	for _, tc := range []struct {
		name          string
		malformed     string
		wantIO        bool
		wantAmbiguous bool
	}{
		{name: "mixed exact and bad PID contends", malformed: "pid=42 pid=bad iowait=1 caller=opaque_peer delay=7", wantAmbiguous: true},
		{name: "pure bad PID is audit only", malformed: "pid=bad iowait=1 caller=opaque_peer delay=7", wantIO: true},
		{name: "overflow PID is audit only", malformed: "pid=2147483648 iowait=1 caller=opaque_peer delay=7", wantIO: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeSchedulerIntegrityTrace(t, "blocked-pid-binding.ftrace",
				"idle-0 (0) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
				"worker-42 (42) [001] .... 1.001000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=20 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120",
				"waker-99 (99) [000] .... 1.010000: sched_wakeup: comm=worker pid=42 prio=20 target_cpu=001",
				"waker-99 (99) [000] .... 1.010002: sched_blocked_reason: pid=42 iowait=1 caller=wait_object delay=7",
				"waker-99 (99) [000] .... 1.010003: sched_blocked_reason: "+tc.malformed,
				"idle-0 (0) [001] .... 1.011000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
			)
			idx, err := BuildIndex(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			q := Query{PID: 42, TimeStart: 1, TimeEnd: 1.011, Limit: 8}
			stats := ComputeWindowStats(idx, q)
			gotIO := durationForPID(stats.IOWaitTop, 42) > 0
			if gotIO != tc.wantIO || (durationForPID(stats.DStateTop, 42) > 0) == tc.wantIO {
				t.Fatalf("indexed D/IO binding drifted: io=%+v d=%+v caveats=%v", stats.IOWaitTop, stats.DStateTop, stats.Caveats)
			}
			if containsSubstring(stats.Caveats, "blocked_reason_marker_ambiguous=true") != tc.wantAmbiguous {
				t.Fatalf("indexed ambiguity drifted: %v", stats.Caveats)
			}
			timeline := ThreadTimeline(idx, q)
			timelineIO := false
			for _, interval := range timeline.Intervals {
				timelineIO = timelineIO || interval.State == StateIOWait
			}
			if timelineIO != tc.wantIO || containsSubstring(timeline.Caveats, "blocked_reason_marker_ambiguous=true") != tc.wantAmbiguous {
				t.Fatalf("timeline binding drifted: intervals=%+v caveats=%v", timeline.Intervals, timeline.Caveats)
			}
			streamed, err := StreamStateCluster(context.Background(), path, q, 8)
			if err != nil {
				t.Fatal(err)
			}
			streamIO := streamed.WindowStats != nil && durationForPID(streamed.WindowStats.IOWaitTop, 42) > 0
			if streamIO != tc.wantIO || containsSubstring(streamed.Caveats, "blocked_reason_marker_ambiguous=true") != tc.wantAmbiguous {
				t.Fatalf("stream binding drifted: stats=%+v caveats=%v", streamed.WindowStats, streamed.Caveats)
			}
		})
	}
}

func TestBlockedReasonSyntheticWindowTailCannotMintClosure(t *testing.T) {
	path := writeSchedulerIntegrityTrace(t, "blocked-synthetic-tail.ftrace",
		"idle-0 (0) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
		"worker-42 (42) [001] .... 1.001000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=20 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120",
		"peer-77 (77) [002] .... 2.000001: tracing_mark_write: C|77|tail|1",
		"waker-99 (99) [000] .... 2.000002: sched_blocked_reason: pid=42 iowait=1 caller=wait_object delay=7",
		"peer-77 (77) [002] .... 2.100000: tracing_mark_write: C|77|after-tail|1",
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{PID: 42, TimeStart: 1, TimeEnd: 2, Limit: 8}
	stats := ComputeWindowStats(idx, q)
	if durationForPID(stats.IOWaitTop, 42) != 0 || durationForPID(stats.DStateTop, 42) <= 0 {
		t.Fatalf("open indexed tail acquired a synthetic marker closure: IO=%+v D=%+v", stats.IOWaitTop, stats.DStateTop)
	}
	timeline := ThreadTimeline(idx, q)
	for _, interval := range timeline.Intervals {
		if interval.State == StateIOWait {
			t.Fatalf("open timeline tail acquired a synthetic marker closure: %+v", timeline.Intervals)
		}
	}
	streamed, err := StreamStateCluster(context.Background(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	if streamed.WindowStats == nil || durationForPID(streamed.WindowStats.IOWaitTop, 42) != 0 || durationForPID(streamed.WindowStats.DStateTop, 42) <= 0 {
		t.Fatalf("open stream tail acquired a synthetic marker closure: %+v", streamed.WindowStats)
	}
}

func TestBlockedReasonOpeningSideMarkerSurvivesSyntheticWindowEnd(t *testing.T) {
	path := writeSchedulerIntegrityTrace(t, "blocked-opening-side.ftrace",
		"idle-0 (0) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
		"worker-42 (42) [001] .... 1.001000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=20 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120",
		"worker-42 (42) [001] .... 1.001001: sched_blocked_reason: pid=42 iowait=1 caller=perfetto_opening_side delay=7",
		"peer-77 (77) [002] .... 1.100000: tracing_mark_write: C|77|after-window|1",
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{PID: 42, TimeStart: 1, TimeEnd: 1.01, Limit: 8}
	stats := ComputeWindowStats(idx, q)
	if durationForPID(stats.IOWaitTop, 42) <= 0 || durationForPID(stats.DStateTop, 42) != 0 {
		t.Fatalf("opening-side indexed marker was lost at a synthetic window end: IO=%+v D=%+v", stats.IOWaitTop, stats.DStateTop)
	}
	streamed, err := StreamStateCluster(context.Background(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	if streamed.WindowStats == nil || durationForPID(streamed.WindowStats.IOWaitTop, 42) <= 0 || durationForPID(streamed.WindowStats.DStateTop, 42) != 0 {
		t.Fatalf("opening-side stream marker was lost at a synthetic window end: %+v", streamed.WindowStats)
	}
}

func TestBlockedReasonLineWindowOwnsMarkerDomain(t *testing.T) {
	path := writeSchedulerIntegrityTrace(t, "blocked-line-window.ftrace",
		"idle-0 (0) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
		"worker-42 (42) [001] .... 1.001000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=20 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120",
		"waker-99 (99) [000] .... 1.010000: sched_wakeup: comm=worker pid=42 prio=20 target_cpu=001",
		"peer-77 (77) [002] .... 1.010000: tracing_mark_write: C|77|between|1",
		"idle-0 (0) [001] .... 1.010001: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
		"waker-99 (99) [000] .... 1.010002: sched_blocked_reason: pid=42 iowait=1 caller=line_boundary delay=7",
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		lineEnd int
		wantIO  bool
	}{
		{name: "marker at LineEnd plus one excluded", lineEnd: 5},
		{name: "marker on LineEnd included", lineEnd: 6, wantIO: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := Query{PID: 42, LineStart: 1, LineEnd: tc.lineEnd, Limit: 8}
			stats := ComputeWindowStats(idx, q)
			if (durationForPID(stats.IOWaitTop, 42) > 0) != tc.wantIO {
				t.Fatalf("indexed line marker domain drifted: IO=%+v D=%+v", stats.IOWaitTop, stats.DStateTop)
			}
			timeline := ThreadTimeline(idx, q)
			foundIO := false
			for _, interval := range timeline.Intervals {
				foundIO = foundIO || interval.State == StateIOWait
			}
			if foundIO != tc.wantIO {
				t.Fatalf("timeline line marker domain drifted: %+v", timeline.Intervals)
			}
			streamed, err := StreamStateCluster(context.Background(), path, q, 8)
			if err != nil {
				t.Fatal(err)
			}
			if streamed.WindowStats == nil || (durationForPID(streamed.WindowStats.IOWaitTop, 42) > 0) != tc.wantIO {
				t.Fatalf("stream line marker domain drifted: %+v", streamed.WindowStats)
			}
		})
	}
}

func TestBlockedReasonPreWindowMarkerCannotClassifyCarryIn(t *testing.T) {
	path := writeSchedulerIntegrityTrace(t, "blocked-prewindow-marker.ftrace",
		"idle-0 (0) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
		"worker-42 (42) [001] .... 1.100000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=20 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120",
		"waker-99 (99) [000] .... 1.500000: sched_blocked_reason: pid=42 iowait=1 caller=old_wait delay=7",
		"waker-99 (99) [000] .... 3.000000: sched_wakeup: comm=worker pid=42 prio=20 target_cpu=001",
		"idle-0 (0) [001] .... 3.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{PID: 42, TimeStart: 2, TimeEnd: 3.01, Limit: 8}
	stats := ComputeWindowStats(idx, q)
	if durationForPID(stats.IOWaitTop, 42) != 0 || durationForPID(stats.DStateTop, 42) <= 0 {
		t.Fatalf("pre-window marker classified carry-in: IO=%+v D=%+v", stats.IOWaitTop, stats.DStateTop)
	}
	streamed, err := StreamStateCluster(context.Background(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	if streamed.WindowStats == nil || durationForPID(streamed.WindowStats.IOWaitTop, 42) != 0 || durationForPID(streamed.WindowStats.DStateTop, 42) <= 0 {
		t.Fatalf("stream pre-window marker classified carry-in: %+v", streamed.WindowStats)
	}
}

func TestBlockedReasonOpeningSideMarkerCarriesWithOpenDState(t *testing.T) {
	path := writeSchedulerIntegrityTrace(t, "blocked-opening-carry.ftrace",
		"idle-0 (0) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
		"worker-42 (42) [001] .... 1.100000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=20 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120",
		"worker-42 (42) [001] .... 1.100001: sched_blocked_reason: pid=42 iowait=1 caller=perfetto_opening_carry delay=7",
		"waker-99 (99) [000] .... 3.000000: sched_wakeup: comm=worker pid=42 prio=20 target_cpu=001",
		"idle-0 (0) [001] .... 3.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
	)
	q := Query{PID: 42, TimeStart: 2, TimeEnd: 3.01, Limit: 8}
	assert := func(t *testing.T, idx *Index, lane string) {
		t.Helper()
		stats := ComputeWindowStats(idx, q)
		if durationForPID(stats.IOWaitTop, 42) <= 0 || durationForPID(stats.DStateTop, 42) != 0 {
			t.Fatalf("%s lost opening-side carry: IO=%+v D=%+v caveats=%v", lane, stats.IOWaitTop, stats.DStateTop, stats.Caveats)
		}
		timeline := ThreadTimeline(idx, q)
		foundIO := false
		for _, interval := range timeline.Intervals {
			foundIO = foundIO || interval.State == StateIOWait
		}
		if !foundIO {
			t.Fatalf("%s timeline lost opening-side carry: %+v caveats=%v", lane, timeline.Intervals, timeline.Caveats)
		}
	}
	full, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	assert(t, full, "full")
	windowed, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: q.TimeStart, TimeEnd: q.TimeEnd, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert(t, windowed, "windowed")
	streamed, err := StreamStateCluster(context.Background(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	if streamed.WindowStats == nil || durationForPID(streamed.WindowStats.IOWaitTop, 42) <= 0 || durationForPID(streamed.WindowStats.DStateTop, 42) != 0 {
		t.Fatalf("stream lost opening-side carry: %+v caveats=%v", streamed.WindowStats, streamed.Caveats)
	}
}

func TestBlockedReasonHeadCarryAdmissionRequiresCrossWindowInterval(t *testing.T) {
	q := Query{TimeStart: 2, TimeEnd: 3}
	for _, tc := range []struct {
		name       string
		start, end float64
		query      Query
		want       bool
	}{
		{name: "crosses head", start: 1, end: 2, query: q, want: true},
		{name: "starts on head", start: 2, end: 2.5, query: q},
		{name: "ordinary in-window interval", start: 2.1, end: 2.5, query: q},
		{name: "closed before head", start: 1, end: 1.9, query: q},
		{name: "line window has no time-head carry", start: 1, end: 2.5, query: Query{TimeStart: 2, LineStart: 1, LineEnd: 10}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := blockedReasonIntervalCanConsumeHeadCarry(tc.query, tc.start, tc.end); got != tc.want {
				t.Fatalf("head-carry admission=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestBlockedReasonTruncatedOpeningCandidateFailsClosedBeyondRetainedPIDs(t *testing.T) {
	var declarations []string
	for pid := 100; pid < 132; pid++ {
		declarations = append(declarations, "pid="+strconv.Itoa(pid))
	}
	// The target is deliberately the 33rd declaration, beyond the bounded
	// candidate inventory retained from the malformed physical row.
	declarations = append(declarations, "pid=42")
	path := writeSchedulerIntegrityTrace(t, "blocked-opening-truncated-candidate.ftrace",
		"idle-0 (0) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
		"worker-42 (42) [001] .... 1.100000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=20 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120",
		"worker-42 (42) [001] .... 1.100001: sched_blocked_reason: pid=42 iowait=1 caller=valid_opening delay=7",
		"worker-42 (42) [001] .... 1.100002: sched_blocked_reason: "+strings.Join(declarations, " ")+" iowait=1 caller=truncated_opening delay=7",
		"waker-99 (99) [000] .... 3.000000: sched_wakeup: comm=worker pid=42 prio=20 target_cpu=001",
		"idle-0 (0) [001] .... 3.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
	)
	q := Query{PID: 42, TimeStart: 2, TimeEnd: 3.01, Limit: 8}
	assertGenericD := func(t *testing.T, stats *WindowStats, caveats []string, lane string) {
		t.Helper()
		if stats == nil || durationForPID(stats.IOWaitTop, 42) != 0 || durationForPID(stats.DStateTop, 42) <= 0 {
			t.Fatalf("%s let a truncated opening candidate mint IO: stats=%+v caveats=%v", lane, stats, caveats)
		}
		if !containsSubstring(caveats, "blocked_reason_pid_candidate_set_truncated=true") {
			t.Fatalf("%s hid truncated opening identity inventory: %v", lane, caveats)
		}
	}
	full, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	fullStats := ComputeWindowStats(full, q)
	assertGenericD(t, &fullStats, fullStats.Caveats, "full")
	windowed, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: q.TimeStart, TimeEnd: q.TimeEnd, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	windowedStats := ComputeWindowStats(windowed, q)
	assertGenericD(t, &windowedStats, windowedStats.Caveats, "windowed")
	streamed, err := StreamStateCluster(context.Background(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	assertGenericD(t, streamed.WindowStats, streamed.Caveats, "stream")
}

func TestBlockedReasonAdjacentIntervalStartIsOpen(t *testing.T) {
	thread := ThreadRef{PID: 42}
	markers := map[int][]Event{42: {blockedMatchEvent(2, 10, 42, 1)}}
	if got := matchBlockedReasonForInterval(markers, thread, 1, 2); got.Event == nil || got.Ambiguous {
		t.Fatalf("marker on first physical close was not admitted: %+v", got)
	}
	if got := matchBlockedReasonForInterval(markers, thread, 2, 3); got.Event != nil || got.Physical != nil || got.Candidates != 0 {
		t.Fatalf("same marker was reused at the adjacent interval start: %+v", got)
	}
}

func TestBlockedReasonPIDCandidateSetTruncationFailsClosedAndDiscloses(t *testing.T) {
	var declarations []string
	for pid := 42; pid < 75; pid++ {
		declarations = append(declarations, "pid="+strconv.Itoa(pid))
	}
	path := writeSchedulerIntegrityTrace(t, "blocked-candidate-truncated.ftrace",
		"idle-0 (0) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
		"worker-42 (42) [001] .... 1.001000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=20 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120",
		"waker-99 (99) [000] .... 1.010000: sched_wakeup: comm=worker pid=42 prio=20 target_cpu=001",
		"waker-99 (99) [000] .... 1.010001: sched_blocked_reason: pid=42 iowait=1 caller=valid_wait delay=7",
		"waker-99 (99) [000] .... 1.010002: sched_blocked_reason: "+strings.Join(declarations, " ")+" iowait=1 caller=ambiguous_set delay=7",
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{PID: 42, TimeStart: 1, TimeEnd: 1.011, Limit: 8}
	stats := ComputeWindowStats(idx, q)
	if durationForPID(stats.IOWaitTop, 42) != 0 || durationForPID(stats.DStateTop, 42) <= 0 ||
		!containsSubstring(stats.Caveats, "blocked_reason_pid_candidate_set_truncated=true") {
		t.Fatalf("candidate truncation did not fail closed/disclose: IO=%+v D=%+v caveats=%v", stats.IOWaitTop, stats.DStateTop, stats.Caveats)
	}
	streamed, err := StreamStateCluster(context.Background(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	if streamed.WindowStats == nil || durationForPID(streamed.WindowStats.IOWaitTop, 42) != 0 ||
		!containsSubstring(streamed.Caveats, "blocked_reason_pid_candidate_set_truncated=true") {
		t.Fatalf("stream candidate truncation did not fail closed/disclose: stats=%+v caveats=%v", streamed.WindowStats, streamed.Caveats)
	}
}

func TestBlockedReasonBadIOWaitKeepsCallerAndDelayCensus(t *testing.T) {
	path := writeSchedulerIntegrityTrace(t, "blocked-bad-iowait-census.ftrace",
		"idle-0 (0) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
		"worker-42 (42) [001] .... 1.001000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=20 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120",
		"waker-99 (99) [000] .... 1.010000: sched_wakeup: comm=worker pid=42 prio=20 target_cpu=001",
		"waker-99 (99) [000] .... 1.010002: sched_blocked_reason: pid=42 iowait=2 caller=f2fs_wait_on_block delay=7",
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{PID: 42, TimeStart: 1, TimeEnd: 1.011, Limit: 8}
	stats := ComputeWindowStats(idx, q)
	if stats.BlockedReasonCount != 1 || stats.IOWaitBlockedCount != 0 || durationForPID(stats.IOWaitTop, 42) != 0 || durationForPID(stats.DStateTop, 42) <= 0 {
		t.Fatalf("bad iowait aliased a classification: %+v", stats)
	}
	if len(stats.BlockedReasonCensus) != 1 || len(stats.BlockedReasonCensus[0].Callers) != 1 ||
		stats.BlockedReasonCensus[0].Callers[0].Caller != "f2fs_wait_on_block" ||
		math.Abs(stats.BlockedReasonCensus[0].Callers[0].DelayTotalMs-0.007) > 0.000001 {
		t.Fatalf("bad iowait erased independent caller/delay evidence: %+v", stats.BlockedReasonCensus)
	}
	timeline := ThreadTimeline(idx, q)
	foundUnknown := false
	for _, interval := range timeline.Intervals {
		foundUnknown = foundUnknown || (interval.State == StateDSleep && strings.Contains(interval.Summary, "iowait=unknown") && strings.Contains(interval.Summary, "caller=f2fs_wait_on_block"))
	}
	if !foundUnknown {
		t.Fatalf("timeline lost independent caller proof: %+v", timeline.Intervals)
	}
}

func TestStreamBlockedReasonWaitsForEOFWhenGlobalTimestampRegresses(t *testing.T) {
	path := writeSchedulerIntegrityTrace(t, "blocked-marker-global-regression.ftrace",
		"idle-0 (0) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
		"worker-42 (42) [001] .... 2.000000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=20 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120",
		"waker-99 (99) [000] .... 10.000000: sched_wakeup: comm=worker pid=42 prio=20 target_cpu=001",
		"other-77 (77) [002] .... 20.000000: sched_switch: prev_comm=other prev_pid=77 prev_prio=120 prev_state=R ==> next_comm=peer next_pid=78 next_prio=120",
		"waker-99 (99) [000] .... 10.000002: sched_blocked_reason: pid=42 iowait=1 caller=wait_object delay=7",
		"idle-0 (0) [001] .... 20.100000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if idx.TimestampOrder != TraceTimestampOrderRegressed {
		t.Fatalf("fixture must prove a global physical-order regression: %v", idx.TimestampOrder)
	}
	q := Query{PID: 42, TimeStart: 1, TimeEnd: 21, Limit: 8}
	indexed := ComputeWindowStats(idx, q)
	if got := durationForPID(indexed.IOWaitTop, 42); math.Abs(got-8000) > 0.001 {
		t.Fatalf("indexed lane lost trailing marker after unrelated future row: %.6f %+v", got, indexed.Caveats)
	}
	streamed, err := StreamStateCluster(context.Background(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	if streamed.WindowStats == nil {
		t.Fatal("stream state-cluster did not publish stats")
	}
	if got := durationForPID(streamed.WindowStats.IOWaitTop, 42); math.Abs(got-8000) > 0.001 {
		t.Fatalf("stream flushed D before a physically later regressed marker: %.6f caveats=%v", got, streamed.Caveats)
	}
}

func TestStreamStateClusterEffectiveStateDrivesSwitchesAndExclusiveLane(t *testing.T) {
	accs := map[string]*stateChurnAcc{}
	running, runnable := map[string]ThreadDuration{}, map[string]ThreadDuration{}
	sleep, dstate, iowait := map[string]ThreadDuration{}, map[string]ThreadDuration{}, map[string]ThreadDuration{}
	thread := ThreadRef{Comm: "worker", PID: 42}
	markers := map[int][]Event{42: {blockedMatchEvent(2, 2, 42, 1)}}
	addStreamStateClusterInterval(nil, accs, running, runnable, sleep, dstate, iowait,
		stateChurnOpen{thread: thread, state: StateDSleep, ts: 1, line: 1}, 2, 2, Query{}, markers)
	addStreamStateClusterInterval(nil, accs, running, runnable, sleep, dstate, iowait,
		stateChurnOpen{thread: thread, state: StateDSleep, ts: 3, line: 3}, 4, 4, Query{}, nil)
	acc := accs[threadKey(thread)]
	if acc == nil || acc.stateSwitches != 1 || acc.lastState != StateDSleep {
		t.Fatalf("effective IO→D transition must drive churn state, got %+v", acc)
	}
	if durationForPID(iowaitMapValues(iowait), 42) != 1000 || durationForPID(iowaitMapValues(dstate), 42) != 1000 {
		t.Fatalf("effective states must book one exclusive second each: D=%+v IO=%+v", dstate, iowait)
	}
}

func iowaitMapValues(in map[string]ThreadDuration) []ThreadDuration {
	out := make([]ThreadDuration, 0, len(in))
	for _, item := range in {
		out = append(out, item)
	}
	return out
}
