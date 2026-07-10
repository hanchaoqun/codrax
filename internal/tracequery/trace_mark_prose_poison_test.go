package tracequery

import "testing"

// ENG audit #4a (P3, §29.25 处置委托 2026-07-10): a non-ftrace line that merely
// QUOTES a mark payload ("...tracing_mark_write: B|...") — the common shape in
// mixed log artifacts — used to mint an artifact-GLOBAL Unmaterialized span
// poison and kill trace_spans/trace_mark_categories/async_file_work plus the
// IPC interface join for every query. Global poison is a hard gate and now
// requires a precise corrupted-ftrace-row remnant (a seconds timestamp token
// or a bracketed CPU column before the marker); structural '#' comments never
// poison. Really corrupted rows keep their remnant and stay fail-closed.

func TestProseTraceMarkMentionDoesNotMintGlobalSpanPoison(t *testing.T) {
	// Free prose quoting a span payload: no poison.
	if failure := traceMarkValidationFailure(3, `W GoogleMap: dropped payload tracing_mark_write: B|123|H:frame`); failure != nil {
		t.Fatalf("prose mention of a mark payload is not a corrupted trace-mark row: %+v", failure)
	}
	// Structural comment carrying a timestamp-like remnant: comments never
	// poison even when the remnant heuristic would otherwise fire.
	if failure := traceMarkValidationFailure(4, `# dropped at 3.001000: tracing_mark_write: B|10|x`); failure != nil {
		t.Fatalf("comment lines must never poison span pairing: %+v", failure)
	}
	// A corrupted-header row keeps its timestamp remnant: poison retained.
	if failure := traceMarkValidationFailure(5, `?corrupt? 3.001000: tracing_mark_write: B|10|x`); failure == nil || !failure.Unmaterialized {
		t.Fatalf("corrupted ftrace row with a timestamp remnant must stay a global poison: %+v", failure)
	}
	// A corrupted-header row keeping only its CPU column: poison retained.
	if failure := traceMarkValidationFailure(6, `?corrupt? [003] tracing_mark_write: E`); failure == nil || !failure.Unmaterialized {
		t.Fatalf("corrupted ftrace row with a CPU-column remnant must stay a global poison: %+v", failure)
	}
}

func TestProseTraceMarkMentionKeepsSpanFacesAlive(t *testing.T) {
	idx := buildTraceIndex(t, "prose_mark.systrace", `
 app-10 (10) [000] .... 3.000000: tracing_mark_write: B|10|frame
log: replayed user text tracing_mark_write: B|999|bogus
 app-10 (10) [000] .... 3.002000: tracing_mark_write: E
`)
	spans, _, caveats := computeTraceMarks(idx, Query{TimeStart: 3, TimeEnd: 3.01}, 16)
	if len(spans) != 1 {
		t.Fatalf("a prose mention must not fail-close the whole artifact's span faces: spans=%+v caveats=%+v", spans, caveats)
	}
	if caveatsContain(caveats, "trace_mark_span_pairing_fail_closed=true") {
		t.Fatalf("no global poison exists in this artifact: %+v", caveats)
	}
}
