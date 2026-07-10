package tool

// trace_query_stream_denial_test.go — audit #55 (tool half, 2026-07-10
// STREAM batch): the index_event_limit denial prose must not give
// self-referential recovery guidance. When the failed request window already
// sits AT or BELOW the preferred 80-150ms coverage band, "Split toward
// 80-150ms coverage windows" tells the model to re-request the exact window
// that just failed; the fork swaps in non-time-splitting levers
// (pid/thread, event_types, line windows). Precise trigger signal: the typed
// request-window duration only. Longer or unset windows keep the historical
// sentence byte-compatible, and the >1s window_sweep_first steer (§4.7 W3)
// is untouched.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQueryIndexLimitSummarySkipsSelfReferentialSplitOnBandWindows(t *testing.T) {
	limitErr := &tracequery.IndexEventLimitError{
		Path:      "dense.ftrace",
		MaxEvents: 250000,
		Events:    250000,
		FirstTs:   1.0,
		LastTs:    1.05,
	}
	summaryFor := func(start, end float64, bounded bool) string {
		p := traceQueryParams{View: "root_cause_rank"}
		if bounded {
			p.TimeStart = traceSecondFromAutoWindow(start)
			p.TimeEnd = traceSecondFromAutoWindow(end)
		}
		return traceQueryIndexLimitSummary("dense.ftrace", "path", p, limitErr)
	}

	// 100ms window — the sweep default bucket width, the exact customer shape:
	// already inside the band, so no "Split toward" and the alternative levers
	// must be named.
	inBand := summaryFor(1.0, 1.1, true)
	if strings.Contains(inBand, "Split toward") {
		t.Fatalf("a window already inside the coverage band must not be told to split toward that band:\n%s", inBand)
	}
	for _, want := range []string{
		"already spans 100ms",
		"pid=<target pid>",
		"event_types",
		"line_start/line_end",
	} {
		if !strings.Contains(inBand, want) {
			t.Fatalf("in-band denial must offer non-time-splitting levers (%q missing):\n%s", want, inBand)
		}
	}

	// Exactly 150ms — the band's upper edge is still self-referential (<=).
	if edge := summaryFor(1.0, 1.15, true); strings.Contains(edge, "Split toward") {
		t.Fatalf("a window at the band's upper edge must not get the split sentence:\n%s", edge)
	}

	// 60ms — below the band: splitting time even further is still not the
	// lever; same fork.
	if below := summaryFor(1.0, 1.06, true); strings.Contains(below, "Split toward") {
		t.Fatalf("a sub-band window must not get the split sentence:\n%s", below)
	}

	// 500ms — above the band: the historical sentence stays byte-compatible.
	wide := summaryFor(1.0, 1.5, true)
	if !strings.Contains(wide, "Split toward 80-150ms coverage windows for jank/stall root-cause views") {
		t.Fatalf("above-band window keeps the historical split guidance:\n%s", wide)
	}

	// No explicit window: duration unknown — historical sentence stays.
	unbounded := summaryFor(0, 0, false)
	if !strings.Contains(unbounded, "Split toward 80-150ms coverage windows") {
		t.Fatalf("unbounded request keeps the historical split guidance:\n%s", unbounded)
	}

	// Micro-probe floor stays disclosed on both arms.
	for _, s := range []string{inBand, wide} {
		if !strings.Contains(s, "only as a local micro-probe") {
			t.Fatalf("micro-probe floor disclosure must survive the fork:\n%s", s)
		}
	}
}
