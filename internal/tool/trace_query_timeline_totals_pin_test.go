package tool

// trace_query_timeline_totals_pin_test.go — E1-b + E1-a tool-face pins
// (RTC-R1 e1, 2026-07-05).
//
// E1-b: the thread_timeline text face truncates the interval listing at 12
// rows, so the per-state totals (Σms + segment count) MUST be published as a
// deterministic block computed from the FULL interval slice — the same slice
// the JSON payload serialises. A totals block computed from the truncated
// 12-row sample goes red here (the distinctive-duration intervals live past
// index 12 on purpose).
//
// E1-a tool face: a window-clamped interval row must show the clamped primary
// duration plus the actual_duration/actual_window dual-ledger tokens; an
// unclamped row must not carry them.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func rtcTimelineInterval(state tracequery.ThreadState, start, end float64) tracequery.Interval {
	durationMs := (end - start) * 1000
	return tracequery.Interval{
		Thread:           tracequery.ThreadRef{Comm: "app", PID: 20},
		State:            state,
		StartTs:          start,
		EndTs:            end,
		DurationMs:       durationMs,
		ActualStartTs:    start,
		ActualEndTs:      end,
		ActualDurationMs: durationMs,
		StartLine:        1,
		EndLine:          2,
		Summary:          fmt.Sprintf("%s for %.3f ms", state, durationMs),
	}
}

func TestTraceQuerySummaryThreadTimelineStateTotalsCoverFullSample(t *testing.T) {
	// 14 intervals: 6 running (1ms each) + 6 s_sleep (2ms each) interleaved in
	// the first 12 rows, then — PAST the 12-row truncation point — one running
	// 100ms and one runnable 7ms. Totals from the truncated sample would read
	// running=6ms/6 segments and miss runnable entirely.
	var intervals []tracequery.Interval
	ts := 10.0
	for i := 0; i < 6; i++ {
		intervals = append(intervals, rtcTimelineInterval(tracequery.StateRunning, ts, ts+0.001))
		ts += 0.001
		intervals = append(intervals, rtcTimelineInterval(tracequery.StateSSleep, ts, ts+0.002))
		ts += 0.002
	}
	intervals = append(intervals, rtcTimelineInterval(tracequery.StateRunning, ts, ts+0.100))
	ts += 0.100
	intervals = append(intervals, rtcTimelineInterval(tracequery.StateRunnable, ts, ts+0.007))

	// Independent per-state Σ over the SAME full slice the payload serialises.
	wantTotal := map[tracequery.ThreadState]float64{}
	wantSegments := map[tracequery.ThreadState]int{}
	for _, it := range intervals {
		wantTotal[it.State] += it.DurationMs
		wantSegments[it.State]++
	}

	result := tracequery.Result{
		View:     "thread_timeline",
		Timeline: &tracequery.TimelineResult{Thread: tracequery.ThreadRef{Comm: "app", PID: 20}, Intervals: intervals},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "thread_timeline"}, "path", "/tmp/payload.json")

	for _, state := range []tracequery.ThreadState{tracequery.StateRunning, tracequery.StateRunnable, tracequery.StateSSleep} {
		want := fmt.Sprintf("- state_total state=%s total=%.3fms segments=%d\n", state, wantTotal[state], wantSegments[state])
		if !strings.Contains(summary, want) {
			t.Fatalf("missing full-sample state_total line %q in summary:\n%s", want, summary)
		}
	}
	// Mutation trap: totals derived from the truncated 12-row sample.
	if strings.Contains(summary, "state_total state=running total=6.000ms") {
		t.Fatalf("running total computed from the truncated sample, not the full slice:\n%s", summary)
	}
	// Truncation disclosure still present, and totals stay full-sample.
	if !strings.Contains(summary, "... omitted 2 interval(s); see payload_ref") {
		t.Fatalf("expected the truncation disclosure to survive:\n%s", summary)
	}
	// The totals block precedes both the interval rows and the truncation line.
	totalsAt := strings.Index(summary, "- state_totals intervals=14 basis=single_thread_non_overlapping")
	firstRowAt := strings.Index(summary, "- running 10.000000..")
	omittedAt := strings.Index(summary, "... omitted 2 interval(s)")
	if totalsAt < 0 || firstRowAt < 0 || omittedAt < 0 || !(totalsAt < firstRowAt && firstRowAt < omittedAt) {
		t.Fatalf("totals block must precede interval rows and the truncation line (totals=%d firstRow=%d omitted=%d):\n%s", totalsAt, firstRowAt, omittedAt, summary)
	}
	// Deterministic state order = the authoritative ThreadState universe order.
	runningAt := strings.Index(summary, "- state_total state=running ")
	runnableAt := strings.Index(summary, "- state_total state=runnable ")
	sleepAt := strings.Index(summary, "- state_total state=s_sleep ")
	if !(totalsAt < runningAt && runningAt < runnableAt && runnableAt < sleepAt) {
		t.Fatalf("state_total lines must follow the ThreadState universe order (running=%d runnable=%d s_sleep=%d):\n%s", runningAt, runnableAt, sleepAt, summary)
	}
}

func TestTraceQuerySummaryThreadTimelineClampedRowDualLedger(t *testing.T) {
	inside := rtcTimelineInterval(tracequery.StateSSleep, 1.10, 1.18)
	clamped := tracequery.Interval{
		Thread:           tracequery.ThreadRef{Comm: "app", PID: 20},
		State:            tracequery.StateRunning,
		StartTs:          1.22,
		EndTs:            1.22,
		DurationMs:       0,
		ActualStartTs:    1.22,
		ActualEndTs:      1.26,
		ActualDurationMs: 40,
		StartLine:        5,
		EndLine:          6,
		Summary:          "running for 0.000 ms actual_duration=40.000ms actual_window=1.220000..1.260000",
	}
	result := tracequery.Result{
		View:     "thread_timeline",
		Timeline: &tracequery.TimelineResult{Thread: tracequery.ThreadRef{Comm: "app", PID: 20}, Intervals: []tracequery.Interval{inside, clamped}},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "thread_timeline"}, "path", "/tmp/payload.json")
	wantRow := "- running 1.220000..1.220000 0.000ms lines=5-6 wake_line=0 actual_duration=40.000ms actual_window=1.220000..1.260000\n"
	if !strings.Contains(summary, wantRow) {
		t.Fatalf("clamped timeline row must publish the dual ledger, want %q in:\n%s", wantRow, summary)
	}
	wantInsideRow := "- s_sleep 1.100000..1.180000 80.000ms lines=1-2 wake_line=0\n"
	if !strings.Contains(summary, wantInsideRow) {
		t.Fatalf("unclamped timeline row must stay in the historical form, want %q in:\n%s", wantInsideRow, summary)
	}
}

// Bounds-only shape (PTV4 review finding, 2026-07-05): the tool row must
// publish the actual duration through the SAME
// tracequery.Interval.ActualDurationMsResolved fallback authority the Summary
// face uses. A bare it.ActualDurationMs read prints actual_duration=0.000ms
// for a bounds-only interval (actual bounds set, ActualDurationMs zero) while
// the Summary token carries the bounds-derived value — mutating the row back
// to the bare read goes red on both assertions below.
func TestTraceQuerySummaryThreadTimelineBoundsOnlyRowUsesResolvedActualDuration(t *testing.T) {
	boundsOnly := tracequery.Interval{
		Thread:  tracequery.ThreadRef{Comm: "app", PID: 20},
		State:   tracequery.StateRunning,
		StartTs: 1.22, EndTs: 1.22, DurationMs: 0,
		ActualStartTs: 1.22, ActualEndTs: 1.26, // ActualDurationMs deliberately 0
		StartLine: 5, EndLine: 6,
		// Summary face as tracequery regenerates it (clampedIntervalSummary
		// via ActualDurationMsResolved) — the row token must match it.
		Summary: "running for 0.000 ms actual_duration=40.000ms actual_window=1.220000..1.260000",
	}
	result := tracequery.Result{
		View:     "thread_timeline",
		Timeline: &tracequery.TimelineResult{Thread: boundsOnly.Thread, Intervals: []tracequery.Interval{boundsOnly}},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "thread_timeline"}, "path", "/tmp/payload.json")
	wantRow := "- running 1.220000..1.220000 0.000ms lines=5-6 wake_line=0 actual_duration=40.000ms actual_window=1.220000..1.260000\n"
	if !strings.Contains(summary, wantRow) {
		t.Fatalf("bounds-only row must publish the resolved actual duration, want %q in:\n%s", wantRow, summary)
	}
	if strings.Contains(summary, "actual_duration=0.000ms") {
		t.Fatalf("bounds-only row published a bare zero actual duration (row/Summary faces diverge):\n%s", summary)
	}
}
