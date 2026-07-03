package tool

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// TestTraceQueryWindowedIndexTimePaddingFixedFirstBuild pins the QF4
// two-level padding policy, level 1: the FIRST windowed build always uses
// the historical fixed per-view padding, regardless of the request window
// duration. Pin changed from the one-level proportional policy
// (2026-07-03): unconditionally proportional padding silently shrank
// pre-window visibility (open scheduler states, wakeup edges) for every
// healthy build that never approached the event budget; the proportional
// value now applies only on the budget-exhaustion retry.
func TestTraceQueryWindowedIndexTimePaddingFixedFirstBuild(t *testing.T) {
	cases := []struct {
		view string
		want float64
	}{
		{"frame_window", 0.500},
		{"window_stats", 0.500},
		{"root_cause_rank", 0.500},
		{"", 0.500},
		{"thread_timeline", 0.250},
		{"scheduler_latency_stats", 0.250},
		{"event_search", 0.050},
	}
	for _, tc := range cases {
		if got := traceQueryWindowedIndexTimePadding(tc.view); got != tc.want {
			t.Fatalf("view=%q first-build padding=%.6f want %.6f", tc.view, got, tc.want)
		}
	}
	// The customer's 101ms frame_window gets the FULL fixed padding on the
	// first build — zero information loss vs pre-2026-07-03 behavior; the
	// options builder consumes the fixed value verbatim.
	p := traceQueryParams{View: "frame_window"}
	p.TimeStart = traceSecondFromAutoWindow(2.000)
	p.TimeEnd = traceSecondFromAutoWindow(2.101)
	opts := traceQueryWindowedIndexOptions(p, 2.000, 2.101)
	if opts.TimePaddingBefore != 0.500 || opts.TimePaddingAfter != 0.500 {
		t.Fatalf("first build must use the fixed view padding, got ±%.6f/±%.6f", opts.TimePaddingBefore, opts.TimePaddingAfter)
	}
}

// TestTraceQueryReducedIndexTimePaddingRetryValues pins the retry-only
// proportional numbers: min(viewCap, max(0.050, window*0.5)), and ok=false
// whenever the proportional value is not strictly smaller than the padding
// that just failed (retrying at unchanged padding would deterministically
// fail again) or there is no complete time window to scale against.
func TestTraceQueryReducedIndexTimePaddingRetryValues(t *testing.T) {
	reduced := func(view string, start, end, current float64) (float64, bool) {
		p := traceQueryParams{View: view}
		if start != 0 || end != 0 {
			p.TimeStart = traceSecondFromAutoWindow(start)
			p.TimeEnd = traceSecondFromAutoWindow(end)
		}
		return traceQueryReducedIndexTimePadding(p, start, end, current)
	}

	// Customer case: 101ms window, default-cap view → 0.0505 per side.
	got, ok := reduced("frame_window", 2.000, 2.101, 0.500)
	if !ok || math.Abs(got-0.0505) > 1e-9 {
		t.Fatalf("101ms retry padding=%.6f ok=%t want 0.0505 true", got, ok)
	}
	// Tiny windows floor at the 50ms wakeup-context minimum.
	got, ok = reduced("frame_window", 1.000, 1.010, 0.500)
	if !ok || math.Abs(got-0.050) > 1e-9 {
		t.Fatalf("10ms retry padding=%.6f ok=%t want 0.050 true", got, ok)
	}
	// thread_timeline 100ms window → floor 0.050 < 0.250 cap → ok.
	got, ok = reduced("thread_timeline", 5.000, 5.100, 0.250)
	if !ok || math.Abs(got-0.050) > 1e-9 {
		t.Fatalf("thread_timeline retry padding=%.6f ok=%t want 0.050 true", got, ok)
	}
	// Large window: proportional saturates at the view cap == current →
	// NOT strictly smaller → no retry.
	if _, ok := reduced("frame_window", 10.0, 13.3, 0.500); ok {
		t.Fatalf("saturated proportional padding must not qualify for a retry")
	}
	// event_search: cap equals the floor, proportional can never go below
	// the fixed 0.050 → never a retry.
	if _, ok := reduced("event_search", 1.0, 3.0, 0.050); ok {
		t.Fatalf("event_search must never qualify for a reduced-padding retry")
	}
	// Half-open / line-only / degenerate windows have nothing to scale
	// against.
	halfOpen := traceQueryParams{View: "frame_window"}
	halfOpen.TimeStart = traceSecondFromAutoWindow(2.0)
	if _, ok := traceQueryReducedIndexTimePadding(halfOpen, 2.0, 0, 0.500); ok {
		t.Fatalf("half-open window must not qualify for a retry")
	}
	if _, ok := traceQueryReducedIndexTimePadding(traceQueryParams{View: "thread_timeline"}, 0, 0, 0.250); ok {
		t.Fatalf("line-only window must not qualify for a retry")
	}
	if _, ok := reduced("frame_window", 3.0, 2.0, 0.500); ok {
		t.Fatalf("degenerate window must not qualify for a retry")
	}
}

// TestTraceQueryReducedPaddingRetryTrigger pins the retry trigger: a typed
// IndexEventLimitError whose parse boundary never reached the bounded
// request TimeEnd (the parse-side padding-tail degrade cannot save that
// build) plus a strictly smaller proportional padding → exactly one retry
// with both paddings reduced and every other option preserved.
func TestTraceQueryReducedPaddingRetryTrigger(t *testing.T) {
	p := traceQueryParams{View: "frame_window"}
	p.TimeStart = traceSecondFromAutoWindow(2.000)
	p.TimeEnd = traceSecondFromAutoWindow(2.101)
	opts := traceQueryWindowedIndexOptions(p, 2.000, 2.101)
	opts.MaxEvents = 123456
	buildErr := error(&tracequery.IndexEventLimitError{LastTs: 2.050})

	retry, ok := traceQueryReducedPaddingRetryOptions(p, opts, buildErr, 2.000, 2.101)
	if !ok {
		t.Fatalf("limit error short of TimeEnd with shrinkable padding must trigger the retry")
	}
	if math.Abs(retry.TimePaddingBefore-0.0505) > 1e-9 || math.Abs(retry.TimePaddingAfter-0.0505) > 1e-9 {
		t.Fatalf("retry paddings=±%.6f/±%.6f want ±0.0505 both sides", retry.TimePaddingBefore, retry.TimePaddingAfter)
	}
	if retry.MaxEvents != opts.MaxEvents || retry.TimeStart != opts.TimeStart || retry.TimeEnd != opts.TimeEnd ||
		retry.RelationScoped != opts.RelationScoped || retry.LinePaddingBefore != opts.LinePaddingBefore {
		t.Fatalf("retry must preserve every non-padding option: %+v vs %+v", retry, opts)
	}
	// The wrapped form (errors.As path) triggers identically.
	if _, ok := traceQueryReducedPaddingRetryOptions(p, opts, fmt.Errorf("build: %w", buildErr), 2.000, 2.101); !ok {
		t.Fatalf("wrapped limit error must trigger the retry")
	}
	// Caveat note pin: the reduced margin is stated with the proportional
	// value so the model knows how much context survived.
	if got := traceQueryReducedPaddingCaveat(retry.TimePaddingBefore); got != "index rebuilt with reduced padding ±0.0505s (window-proportional) after budget exhaustion" {
		t.Fatalf("reduced-padding caveat drifted: %q", got)
	}
}

// TestTraceQueryReducedPaddingRetryNotTriggered pins every non-trigger
// condition: window already covered (LastTs >= TimeEnd — the parse-side
// PaddingTruncated degrade owns that case), non-limit errors, unbounded
// TimeEnd, and non-shrinkable (event_search / saturated) paddings.
func TestTraceQueryReducedPaddingRetryNotTriggered(t *testing.T) {
	p := traceQueryParams{View: "frame_window"}
	p.TimeStart = traceSecondFromAutoWindow(2.000)
	p.TimeEnd = traceSecondFromAutoWindow(2.101)
	opts := traceQueryWindowedIndexOptions(p, 2.000, 2.101)

	// Budget hit only after the request window was covered.
	covered := &tracequery.IndexEventLimitError{LastTs: 2.101}
	if _, ok := traceQueryReducedPaddingRetryOptions(p, opts, covered, 2.000, 2.101); ok {
		t.Fatalf("LastTs >= TimeEnd must not trigger the retry")
	}
	// Non-limit failures never retry.
	if _, ok := traceQueryReducedPaddingRetryOptions(p, opts, errors.New("boom"), 2.000, 2.101); ok {
		t.Fatalf("non-limit error must not trigger the retry")
	}
	// No bounded TimeEnd → nothing to prove coverage against.
	open := traceQueryParams{View: "frame_window"}
	open.TimeStart = traceSecondFromAutoWindow(2.000)
	openOpts := traceQueryWindowedIndexOptions(open, 2.000, 0)
	if _, ok := traceQueryReducedPaddingRetryOptions(open, openOpts, &tracequery.IndexEventLimitError{LastTs: 1.0}, 2.000, 0); ok {
		t.Fatalf("unbounded TimeEnd must not trigger the retry")
	}
	// event_search: fixed padding already at the floor.
	es := traceQueryParams{View: "event_search"}
	es.TimeStart = traceSecondFromAutoWindow(1.0)
	es.TimeEnd = traceSecondFromAutoWindow(3.0)
	esOpts := traceQueryWindowedIndexOptions(es, 1.0, 3.0)
	if _, ok := traceQueryReducedPaddingRetryOptions(es, esOpts, &tracequery.IndexEventLimitError{LastTs: 1.5}, 1.0, 3.0); ok {
		t.Fatalf("event_search must never trigger the reduced-padding retry")
	}
}

// TestWriteTraceStateDrilldownSummaryFoldsIdleSleepers pins the display fold
// for whole-window sleepers (berlin.systrace 2026-07-03): the typed
// WindowStats summary renders as exactly one line naming the folded threads,
// and stays absent when nothing was folded. Wording pin updated for QF2: the
// line is a neutral fact statement ("no in-window scheduling activity") —
// the signal cannot distinguish an idle service thread from an unpinned
// whole-window-blocked victim, so it must not assert "not root-cause
// evidence".
func TestWriteTraceStateDrilldownSummaryFoldsIdleSleepers(t *testing.T) {
	steps := []tracequery.StateDrilldownStep{{
		Rank:   1,
		Thread: tracequery.ThreadRef{Comm: "worker", PID: 13},
		State:  "s_sleep",
		Source: "top_sleep",
	}}
	fold := &tracequery.IdleWholeWindowSleeperFold{
		Count:   15,
		Threads: []string{"AudioOut-11", "DNSResolver-12"},
	}
	var b strings.Builder
	writeTraceStateDrilldownSummary(&b, steps, fold)
	out := b.String()
	if !strings.Contains(out, "- state_drilldown_idle_folded count=15 threads=AudioOut-11,DNSResolver-12 — whole-window sleepers (no in-window scheduling activity)\n") {
		t.Fatalf("fold line missing or malformed:\n%s", out)
	}
	if strings.Contains(out, "not root-cause") || strings.Contains(out, "idle,") {
		t.Fatalf("fold line must stay a neutral fact statement:\n%s", out)
	}
	if strings.Count(out, "state_drilldown_idle_folded") != 1 {
		t.Fatalf("fold must render as exactly one line:\n%s", out)
	}

	b.Reset()
	writeTraceStateDrilldownSummary(&b, steps, nil)
	if strings.Contains(b.String(), "state_drilldown_idle_folded") {
		t.Fatalf("no fold summary must render nothing:\n%s", b.String())
	}

	// The fold line must survive the summary-cap overflow path too: the
	// omitted marker no longer early-returns past it.
	overflow := make([]tracequery.StateDrilldownStep, 0, traceQueryWidthStateDrilldownSummaryCap()+2)
	for i := 0; i < traceQueryWidthStateDrilldownSummaryCap()+2; i++ {
		overflow = append(overflow, tracequery.StateDrilldownStep{
			Rank:   i + 1,
			Thread: tracequery.ThreadRef{Comm: fmt.Sprintf("t%d", i), PID: 100 + i},
			State:  "s_sleep",
			Source: "top_sleep",
		})
	}
	b.Reset()
	writeTraceStateDrilldownSummary(&b, overflow, fold)
	if !strings.Contains(b.String(), "state_drilldown_omitted count=2") {
		t.Fatalf("overflow marker missing:\n%s", b.String())
	}
	if !strings.Contains(b.String(), "state_drilldown_idle_folded count=15") {
		t.Fatalf("fold line must render even when the step list overflows the cap:\n%s", b.String())
	}
}
