package tracequery

// §29.183 G8 复核修 pins (AUDITFIX-C merge review, 2026-07-21): a line-anchored
// query leaves q.TimeStart at its 0=unset sentinel through normalizeQuery, so
// ResolveFrameTarget's published window becomes (0, end>0) — bytes identical to
// a REAL rebased [0,end] window. The typed window_source suffix
// "query_window_line_anchored_unbounded_start" keeps that ambiguous pair out
// of the projection's anchor gate (宁漏勿假) while explicit time_start=0
// (TimeStartSet) and the plain backfilled whole-trace window keep their
// anchorable "query_window" word.

import (
	"context"
	"testing"
)

func frameWindowSourceProbeTrace(t *testing.T) *Index {
	t.Helper()
	lines := []string{
		"        idle-0 (    0) [001] .... 500.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
		"     worker-42 (   42) [001] .... 500.001000: sched_switch: prev_comm=worker prev_pid=42 prev_prio=20 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120",
		"      waker-99 (   99) [000] .... 500.010000: sched_wakeup: comm=worker pid=42 prio=20 target_cpu=001",
		"        idle-0 (    0) [001] .... 500.011000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=42 next_prio=20",
	}
	path := writeSchedulerIntegrityTrace(t, "frame-window-source-g8.ftrace", lines...)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if idx.FirstTs <= 0 {
		t.Fatalf("probe trace must be non-rebased (FirstTs>0), got %v", idx.FirstTs)
	}
	return idx
}

// The fabrication shape: line_start only (no line_end, no time window) with an
// explicit target. TimeStart stays the 0=unset sentinel while TimeEnd
// backfills to idx.LastTs — the published pair (0, LastTs) must NOT wear the
// anchorable query_window word on a trace whose real time base starts later.
func TestFrameTargetResolutionLineAnchoredUnsetStartWindowSource(t *testing.T) {
	idx := frameWindowSourceProbeTrace(t)
	q := Query{PID: 42, LineStart: 2}
	frame := BuildFrameTimeline(idx, normalizeQuery(idx, q))
	res := ResolveFrameTarget(idx, q, frame)
	if res.Window.StartTs != 0 || res.Window.EndTs <= 0 {
		t.Fatalf("precondition: the ambiguous (0, end) window shape, got %v..%v", res.Window.StartTs, res.Window.EndTs)
	}
	if res.WindowSource != "query_window_line_anchored_unbounded_start" {
		t.Fatalf("line-anchored unset-start window must not claim query_window, got %q", res.WindowSource)
	}
}

// Explicit time_start=0 (TimeStartSet) is the REAL rebased [0,end] anchor G8
// exists for — it keeps the anchorable word even beside a line anchor.
func TestFrameTargetResolutionExplicitZeroStartKeepsQueryWindow(t *testing.T) {
	idx := frameWindowSourceProbeTrace(t)
	q := Query{PID: 42, LineStart: 2, TimeStart: 0, TimeStartSet: true, TimeEnd: 500.011, TimeEndSet: true}
	frame := BuildFrameTimeline(idx, normalizeQuery(idx, q))
	res := ResolveFrameTarget(idx, q, frame)
	if res.WindowSource != "query_window" {
		t.Fatalf("explicit time_start=0 must keep query_window, got %q", res.WindowSource)
	}
	if res.Window.StartTs != 0 || res.Window.EndTs != 500.011 {
		t.Fatalf("explicit [0,end] window must publish verbatim, got %v..%v", res.Window.StartTs, res.Window.EndTs)
	}
}

// The plain whole-trace backfill (no lines, no times) publishes the real
// (FirstTs, LastTs) window under the anchorable word — untouched behavior.
func TestFrameTargetResolutionBackfilledWindowKeepsQueryWindow(t *testing.T) {
	idx := frameWindowSourceProbeTrace(t)
	q := Query{PID: 42}
	frame := BuildFrameTimeline(idx, normalizeQuery(idx, q))
	res := ResolveFrameTarget(idx, q, frame)
	if res.WindowSource != "query_window" {
		t.Fatalf("backfilled whole-trace window must keep query_window, got %q", res.WindowSource)
	}
	if res.Window.StartTs != idx.FirstTs || res.Window.EndTs != idx.LastTs {
		t.Fatalf("backfilled window must be (FirstTs, LastTs), got %v..%v", res.Window.StartTs, res.Window.EndTs)
	}
}
