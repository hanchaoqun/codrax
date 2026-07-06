package tracequery

import (
	"strings"
	"testing"
)

// D-diag B-1 (§16): the zero-match recovery hint GATING is inverted. The old
// next_pattern_call_hint fired only when event_types was EMPTY; the caller who
// most needs "your type filter may be wrong" guidance is the one that DID set a
// narrow event_types and got zero matches. These pins lock the inverted gate,
// the same-window cross-type recount, and the empty-event_types byte-preserved
// path.

func firstCaveatWithPrefix(caveats []string, prefix string) string {
	for _, c := range caveats {
		if strings.HasPrefix(c, prefix) {
			return c
		}
	}
	return ""
}

func anyCaveatHasPrefix(caveats []string, prefix string) bool {
	return firstCaveatWithPrefix(caveats, prefix) != ""
}

// TestZeroMatchHintFiresWhenEventTypesNonEmpty pins the gating INVERSION: with
// a non-empty event_types and zero matches, the type-filter recovery hint must
// fire (the exact q7 five-round-spin shape).
func TestZeroMatchHintFiresWhenEventTypesNonEmpty(t *testing.T) {
	idx := &Index{
		Events: []Event{
			// The pattern lives on a print/trace_mark row, NOT on the
			// trace_mark event_type the query narrowed to being absent —
			// here it is a trace_mark row but the query asked for
			// perf_sample, so it is a cross-type hit.
			{Line: 10, Ts: 1.10, Type: EventTraceMark, Comm: "app", SpanName: "AnimationTick", SpanAction: "B"},
			{Line: 11, Ts: 1.11, Type: EventTraceMark, Comm: "app", SpanName: "AnimationTick", SpanAction: "E"},
		},
	}
	q := Query{
		View:       "event_search",
		Pattern:    "AnimationTick",
		EventTypes: []EventType{EventPerfSample},
	}
	res := Result{View: "event_search"} // zero events
	caveats := resultCaveats(idx, q, res)

	next := firstCaveatWithPrefix(caveats, "next_pattern_call_hint=")
	if next == "" {
		t.Fatalf("non-empty event_types zero-match must emit next_pattern_call_hint: %v", caveats)
	}
	if !strings.Contains(next, "the type filter itself may be excluding the rows") {
		t.Fatalf("inverted hint must call out the type filter as the suspect:\n%s", next)
	}
	if !strings.Contains(next, `retry without event_types`) {
		t.Fatalf("inverted hint must advise dropping event_types:\n%s", next)
	}

	// Cross-type recount turns the generic advice into a counted fact.
	cross := firstCaveatWithPrefix(caveats, "cross_type_pattern_hint=")
	if cross == "" {
		t.Fatalf("pattern present in another event type must produce cross_type_pattern_hint: %v", caveats)
	}
	if !strings.Contains(cross, "trace_mark:2") {
		t.Fatalf("cross-type hint must count the trace_mark rows the filter excluded:\n%s", cross)
	}
	if !strings.Contains(cross, `["perf_sample"]`) {
		t.Fatalf("cross-type hint must echo the excluding filter verbatim:\n%s", cross)
	}
}

// TestZeroMatchHintEmptyEventTypesUnchanged pins the byte-preserved empty
// event_types path: the original next_pattern_call_hint (event_types="trace_mark"
// / "perf_sample" suggestions) must still fire, and the inverted-gate wording
// must NOT appear.
func TestZeroMatchHintEmptyEventTypesUnchanged(t *testing.T) {
	idx := &Index{
		Events: []Event{
			{Line: 10, Ts: 1.10, Type: EventTraceMark, Comm: "app", SpanName: "AnimationTick", SpanAction: "B"},
		},
	}
	q := Query{
		View:    "event_search",
		Pattern: "NoSuchToken",
	}
	res := Result{View: "event_search"}
	caveats := resultCaveats(idx, q, res)

	next := firstCaveatWithPrefix(caveats, "next_pattern_call_hint=")
	if next == "" {
		t.Fatalf("empty event_types zero-match must still emit next_pattern_call_hint: %v", caveats)
	}
	if !strings.Contains(next, `event_types=["trace_mark"]`) {
		t.Fatalf("empty-event_types hint must keep the trace_mark suggestion:\n%s", next)
	}
	if strings.Contains(next, "the type filter itself may be excluding") {
		t.Fatalf("empty-event_types path must NOT use the inverted-gate wording:\n%s", next)
	}
	if anyCaveatHasPrefix(caveats, "cross_type_pattern_hint=") {
		t.Fatalf("no event_types filter means nothing to recount cross-type: %v", caveats)
	}
}

// TestCrossTypeRescanPrintVariantFormShape pins the print/trace_mark variant
// mentioned in the batch pin: a pattern that yields 0 trace_mark hits but N
// EventUnknown/print rows must produce the cross-type count naming Unknown.
func TestCrossTypeRescanPrintVariantFormShape(t *testing.T) {
	idx := &Index{
		Events: []Event{
			// FieldText carries the token, row stayed EventUnknown (print
			// payload the trace_mark classifier did not carve).
			{Line: 5, Ts: 2.10, Type: EventUnknown, Comm: "app", FieldText: "customMarker vsync-42"},
			{Line: 6, Ts: 2.20, Type: EventUnknown, Comm: "app", FieldText: "customMarker vsync-43"},
			{Line: 7, Ts: 2.30, Type: EventUnknown, Comm: "app", FieldText: "customMarker vsync-44"},
		},
	}
	q := Query{
		View:       "event_search",
		Pattern:    "customMarker",
		EventTypes: []EventType{EventTraceMark},
	}
	res := Result{View: "event_search"}
	caveats := resultCaveats(idx, q, res)

	cross := firstCaveatWithPrefix(caveats, "cross_type_pattern_hint=")
	if cross == "" {
		t.Fatalf("pattern present only in Unknown/print rows must recount cross-type: %v", caveats)
	}
	if !strings.Contains(cross, "unknown:3") {
		t.Fatalf("cross-type hint must count the 3 Unknown rows:\n%s", cross)
	}
	if !strings.Contains(cross, `["trace_mark"]`) {
		t.Fatalf("cross-type hint must echo the excluding trace_mark filter:\n%s", cross)
	}
}

// TestCrossTypeRescanNoOtherTypeStaysQuiet pins that when the pattern truly
// appears nowhere else, no false cross-type count is minted (the hint still
// advises dropping event_types, but with no fabricated evidence).
func TestCrossTypeRescanNoOtherTypeStaysQuiet(t *testing.T) {
	idx := &Index{
		Events: []Event{
			{Line: 5, Ts: 2.10, Type: EventSchedSwitch, Comm: "app", PrevComm: "a", NextComm: "b"},
		},
	}
	q := Query{
		View:       "event_search",
		Pattern:    "GhostToken",
		EventTypes: []EventType{EventTraceMark},
	}
	res := Result{View: "event_search"}
	caveats := resultCaveats(idx, q, res)

	if anyCaveatHasPrefix(caveats, "cross_type_pattern_hint=") {
		t.Fatalf("pattern absent everywhere must NOT fabricate a cross-type count: %v", caveats)
	}
	if firstCaveatWithPrefix(caveats, "next_pattern_call_hint=") == "" {
		t.Fatalf("non-empty event_types zero-match must still steer off the type filter: %v", caveats)
	}
}

// TestCrossTypeRescanRespectsWindow pins the performance/correctness discipline:
// the recount honors the SAME line/time window as the failed search, so a
// cross-type row OUTSIDE the window is not counted.
func TestCrossTypeRescanRespectsWindow(t *testing.T) {
	idx := &Index{
		Events: []Event{
			{Line: 10, Ts: 5.00, Type: EventTraceMark, Comm: "app", SpanName: "phaseMark", SpanAction: "B"}, // in window
			{Line: 90, Ts: 9.00, Type: EventTraceMark, Comm: "app", SpanName: "phaseMark", SpanAction: "B"}, // out of window
		},
	}
	q := Query{
		View:       "event_search",
		Pattern:    "phaseMark",
		EventTypes: []EventType{EventPerfSample},
		TimeStart:  4.0,
		TimeEnd:    6.0,
	}
	res := Result{View: "event_search"}
	caveats := resultCaveats(idx, q, res)

	cross := firstCaveatWithPrefix(caveats, "cross_type_pattern_hint=")
	if cross == "" {
		t.Fatalf("in-window cross-type hit must produce a hint: %v", caveats)
	}
	if !strings.Contains(cross, "trace_mark:1") {
		t.Fatalf("only the in-window row must be counted (out-of-window row excluded):\n%s", cross)
	}
}

// TestThreadSelectorSpanNameCaveat pins D-diag B-2: thread=<span name> that
// matches no scheduled thread but IS a span label must emit the typed
// redirect caveat instead of silently returning structural zeros (q7
// bindApplication shape).
func TestThreadSelectorSpanNameCaveat(t *testing.T) {
	idx := &Index{
		Events: []Event{
			// The span "bindApplication" was emitted by a real thread "app"
			// (comm=app), but no thread is NAMED bindApplication.
			{Line: 10, Ts: 1.10, Type: EventTraceMark, Comm: "app", PID: 20, SpanName: "bindApplication", SpanAction: "B"},
			{Line: 11, Ts: 1.20, Type: EventTraceMark, Comm: "app", PID: 20, SpanName: "bindApplication", SpanAction: "E"},
			{Line: 12, Ts: 1.30, Type: EventSchedSwitch, Comm: "app", PrevComm: "app", PrevPID: 20, NextComm: "kworker", NextPID: 5},
		},
	}
	q := Query{
		View:   "window_stats",
		Thread: "bindApplication",
	}
	res := Result{View: "window_stats"}
	caveats := resultCaveats(idx, q, res)

	c := firstCaveatWithPrefix(caveats, "thread_selector_is_span_name=")
	if c == "" {
		t.Fatalf("thread=span-name with no scheduled thread must emit the redirect caveat: %v", caveats)
	}
	if !strings.Contains(c, "span_name=\"bindApplication\"") {
		t.Fatalf("caveat must redirect to span_window with the span label:\n%s", c)
	}
	if !strings.Contains(c, "structurally zero") {
		t.Fatalf("caveat must disclose why the thread-scoped numbers are zero:\n%s", c)
	}
}

// TestThreadSelectorSpanNameCaveatSilentWhenThreadScheduled pins the precise
// gate: a selector that DOES match a scheduled thread's comm (even one that is
// also a span label) must stay silent — the thread-scoped result is real.
func TestThreadSelectorSpanNameCaveatSilentWhenThreadScheduled(t *testing.T) {
	idx := &Index{
		Events: []Event{
			// "render" is both a span label AND a scheduled thread comm.
			{Line: 10, Ts: 1.10, Type: EventTraceMark, Comm: "app", PID: 20, SpanName: "render", SpanAction: "B"},
			{Line: 12, Ts: 1.30, Type: EventSchedSwitch, Comm: "render", PrevComm: "render", PrevPID: 33, NextComm: "kworker", NextPID: 5},
		},
	}
	q := Query{
		View:   "window_stats",
		Thread: "render",
	}
	res := Result{View: "window_stats"}
	caveats := resultCaveats(idx, q, res)

	if anyCaveatHasPrefix(caveats, "thread_selector_is_span_name=") {
		t.Fatalf("selector matching a scheduled thread comm must stay silent: %v", caveats)
	}
}

// TestThreadSelectorSpanNameCaveatSilentOnStreamedResult pins that the caveat
// is suppressed on streamed results, whose mini-index only retains matched
// rows and therefore cannot support a "no scheduled thread" claim.
func TestThreadSelectorSpanNameCaveatSilentOnStreamedResult(t *testing.T) {
	idx := &Index{
		Events: []Event{
			{Line: 10, Ts: 1.10, Type: EventTraceMark, Comm: "app", PID: 20, SpanName: "bindApplication", SpanAction: "B"},
		},
	}
	q := Query{
		View:   "event_search",
		Thread: "bindApplication",
	}
	res := Result{
		View:    "event_search",
		Caveats: []string{"streamed_event_search=true; scanned 100 line(s) without building or caching a full trace index"},
	}
	caveats := resultCaveats(idx, q, res)

	if anyCaveatHasPrefix(caveats, "thread_selector_is_span_name=") {
		t.Fatalf("streamed result must not assert no-scheduled-thread: %v", caveats)
	}
}
