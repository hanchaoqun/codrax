package tracequery

// target_window_state_boundary_fold_test.go — ANSWERFACE-1 件2 (§29.140 G6,
// 2026-07-19): the four-state account's window-boundary fold disclosure. Two
// PRECISE typed detections only: the head prefix carried from a RECOVERED
// pre-window scheduler state (HeadState.Status=="recovered", earliest
// interval opening at the window edge) and the tail suffix flushed from the
// final open interval (EndLine==0, no in-window closing event). Both values
// are already inside their lane totals — disclosure only, never addends
// (禁静默折入); everything else stamps nothing.
//
// Real-fixture pin (a3/c2 形): on donghu_tieba_frame.systrace the whole-trace
// window's only boundary-extrapolated component is the 1.016ms tail-open
// running suffix (switch-in at 34579.594168, no closing event before the
// window end). Investigation record (ANSWERFACE-1, 2026-07-19): the audit's
// 1.954ms "coverage gap" (§29.140 G6; PROFILE §1.7's then-recorded probe
// running 50.524 vs published 52.478 — §1.7 has since been re-based to
// 52.478 by PROFREBASE-1, 2026-07-20, §29.150⑦) decomposes as 1.954 = the
// probe-era engine's identity bug — the 2026-07-05 engine misattributed
// tid 61847's sched_wakeup_new +
// switch-in (comm com.baidu.tieba, later pass_net_thread) to target 59566
// and dropped the true running span 34579.555953..557907 — which the current
// identity-strict engine books correctly as event-backed running. The only
// remaining un-event-closed component is the tail flush pinned here.

import (
	"context"
	"math"
	"testing"
)

func boundaryFoldInterval(state ThreadState, start, end float64, startLine, endLine int) Interval {
	return Interval{
		Thread: ThreadRef{Comm: "ui", PID: 61}, State: state,
		StartTs: start, EndTs: end, DurationMs: (end - start) * 1000,
		StartLine: startLine, EndLine: endLine,
	}
}

func TestBoundaryFoldTailOpenFlushStamped(t *testing.T) {
	window := TimeWindow{StartTs: 100.0, EndTs: 100.1}
	tl := cov4Timeline([]Interval{
		boundaryFoldInterval(StateSSleep, 100.000, 100.060, 3, 9),
		boundaryFoldInterval(StateRunning, 100.060, 100.100, 9, 0), // flushed open to the window end
	})
	account := buildTargetWindowStateAccount(nil, tl, true, tl.Thread, window, nil)
	if account == nil {
		t.Fatalf("account must build")
	}
	if math.Abs(account.TailOpenMs-40) > 1e-6 || account.TailOpenState != "running" {
		t.Fatalf("tail-open flush must stamp lane running 40ms: %+v", account)
	}
	if account.HeadCarryMs != 0 || account.HeadCarryState != "" {
		t.Fatalf("no recovered head state → no head-carry stamp: %+v", account)
	}
	// Disclosure only: the lane totals are untouched by the stamp.
	if math.Abs(account.RunningMs-40) > 1e-6 || math.Abs(account.SleepMs-60) > 1e-6 {
		t.Fatalf("lane totals must not change: %+v", account)
	}
}

func TestBoundaryFoldHeadCarryRecoveredStamped(t *testing.T) {
	window := TimeWindow{StartTs: 100.0, EndTs: 100.1}
	tl := cov4Timeline([]Interval{
		// Clamped head-carry prefix: opened from the pre-window snapshot,
		// closed by the first in-window event at 100.030.
		boundaryFoldInterval(StateSSleep, 100.000, 100.030, 2, 7),
		boundaryFoldInterval(StateRunning, 100.030, 100.100, 7, 12),
	})
	tl.HeadState = &TimelineHeadState{Status: "recovered", BoundaryTs: 100.0, State: StateSSleep, ActualStartTs: 99.2}
	account := buildTargetWindowStateAccount(nil, tl, true, tl.Thread, window, nil)
	if account == nil {
		t.Fatalf("account must build")
	}
	if math.Abs(account.HeadCarryMs-30) > 1e-6 || account.HeadCarryState != "sleep" {
		t.Fatalf("recovered head carry must stamp lane sleep 30ms: %+v", account)
	}
	if account.TailOpenMs != 0 || account.TailOpenState != "" {
		t.Fatalf("closed tail → no tail-open stamp: %+v", account)
	}
}

func TestBoundaryFoldClosedTimelineStampsNothing(t *testing.T) {
	window := TimeWindow{StartTs: 100.0, EndTs: 100.1}
	tl := cov4Timeline([]Interval{
		boundaryFoldInterval(StateRunning, 100.000, 100.060, 1, 5),
		boundaryFoldInterval(StateSSleep, 100.060, 100.100, 5, 11),
	})
	// An observed_in_index head (boundary event at the window edge) is NOT a
	// recovered carry — the head arm must stay silent.
	tl.HeadState = &TimelineHeadState{Status: "observed_in_index", BoundaryTs: 100.0}
	account := buildTargetWindowStateAccount(nil, tl, true, tl.Thread, window, nil)
	if account == nil {
		t.Fatalf("account must build")
	}
	if account.HeadCarryMs != 0 || account.HeadCarryState != "" || account.TailOpenMs != 0 || account.TailOpenState != "" {
		t.Fatalf("fully event-closed timeline must stamp no boundary fold: %+v", account)
	}
}

func TestBoundaryFoldSingleCarriedIntervalCountsOnce(t *testing.T) {
	window := TimeWindow{StartTs: 100.0, EndTs: 100.1}
	// One interval covers the whole window: head-carried AND flushed open.
	tl := cov4Timeline([]Interval{
		boundaryFoldInterval(StateSSleep, 100.000, 100.100, 2, 0),
	})
	tl.HeadState = &TimelineHeadState{Status: "recovered", BoundaryTs: 100.0, State: StateSSleep, ActualStartTs: 99.0}
	account := buildTargetWindowStateAccount(nil, tl, true, tl.Thread, window, nil)
	if account == nil {
		t.Fatalf("account must build")
	}
	if math.Abs(account.HeadCarryMs-100) > 1e-6 || account.HeadCarryState != "sleep" {
		t.Fatalf("head arm owns the single carried interval: %+v", account)
	}
	if account.TailOpenMs != 0 || account.TailOpenState != "" {
		t.Fatalf("the same span must not double-book on the tail arm: %+v", account)
	}
}

func TestBoundaryFoldMidWindowOpenLineStaysSilent(t *testing.T) {
	window := TimeWindow{StartTs: 100.0, EndTs: 100.1}
	// A zero-EndLine interval that does NOT reach the window end is not the
	// tail flush shape — the tail arm requires the window-edge endpoint.
	tl := cov4Timeline([]Interval{
		boundaryFoldInterval(StateRunning, 100.000, 100.040, 1, 0),
		boundaryFoldInterval(StateSSleep, 100.040, 100.100, 6, 9),
	})
	account := buildTargetWindowStateAccount(nil, tl, true, tl.Thread, window, nil)
	if account == nil {
		t.Fatalf("account must build")
	}
	if account.TailOpenMs != 0 || account.TailOpenState != "" {
		t.Fatalf("mid-window open interval must not stamp the tail arm: %+v", account)
	}
}

// TestBoundaryFoldRealFixtureA3Shape pins the a3/c2 whole-trace shape on the
// real fixture: the current engine covers the window fully by event pairs
// except the single 1.016ms tail-open running suffix (switch-in at
// 34579.594168 with no closing event before the window end). See the file
// header for the investigation record on the audit's probe-era 1.954ms.
func TestBoundaryFoldRealFixtureA3Shape(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	window := TimeWindow{StartTs: 34579.450627, EndTs: 34579.595184}
	q := Query{PID: 59566, TimeStart: window.StartTs, TimeEnd: window.EndTs, TimeStartSet: true, TimeEndSet: true}
	tl, ok := targetWindowTimeline(idx, q, ThreadRef{PID: 59566}, window)
	if !ok {
		t.Fatalf("fixture timeline must resolve")
	}
	account := buildTargetWindowStateAccount(idx, tl, ok, ThreadRef{PID: 59566}, window, nil)
	if account == nil {
		t.Fatalf("fixture account must build")
	}
	if math.Abs(account.RunningMs-52.478) > 0.001 {
		t.Fatalf("published running total drifted: %+v", account)
	}
	if account.TailOpenState != "running" || math.Abs(account.TailOpenMs-1.016) > 0.001 {
		t.Fatalf("the 34579.594168 switch-in tail flush must stamp running ~1.016ms: %+v", account)
	}
	if account.HeadCarryMs != 0 {
		t.Fatalf("the window head is event-backed (line 1 switch-in at the window edge): %+v", account)
	}
}
