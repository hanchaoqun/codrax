package tool

// trace_query_top_overflow_banner_test.go — PIN-1 B2 (§29.65 回归口, 2026-07-13):
// the ENG-1 修复轮二 件A cap-overflow disclosure has engine-field pins
// (rank_dio_fullaccount_a_test.go) but the TOOL WIRE face — the two
// top_*_overflow banner lines traceQuerySummary emits into the model-visible
// window_stats view — had none: deleting either fmt.Fprintf line stayed
// green. These pins hold the wire wording verbatim (the model consumes this
// exact text to learn the top lists are a display cap, not the full
// account) and the zero-overflow silence.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQuerySummaryDisclosesTopCapOverflowOnWire(t *testing.T) {
	result := tracequery.Result{
		View: "window_stats",
		WindowStats: &tracequery.WindowStats{
			DStateTopOverflowGroups: 2,
			DStateTopOverflowMs:     21.0,
			IOWaitTopOverflowGroups: 1,
			IOWaitTopOverflowMs:     7.5,
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "/tmp/payload.json")
	// Verbatim wire lines (删行应红): groups + total + the family-seat
	// full-account pointer.
	for _, want := range []string{
		"- top_d_state_overflow groups=2 total=21.000ms (beyond the display cap; D/IO family seats carry the full per-thread account)\n",
		"- top_io_wait_overflow groups=1 total=7.500ms (beyond the display cap; D/IO family seats carry the full per-thread account)\n",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("wire summary must carry the cap-overflow disclosure %q:\n%s", want, summary)
		}
	}
}

func TestTraceQuerySummaryZeroTopCapOverflowStaysSilent(t *testing.T) {
	result := tracequery.Result{
		View:        "window_stats",
		WindowStats: &tracequery.WindowStats{},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "/tmp/payload.json")
	for _, banned := range []string{"top_d_state_overflow", "top_io_wait_overflow"} {
		if strings.Contains(summary, banned) {
			t.Fatalf("zero eviction must emit zero disclosure (%q leaked):\n%s", banned, summary)
		}
	}
}
