package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQuerySummaryLabelsIRQBurstAsInventorySpan(t *testing.T) {
	result := tracequery.Result{
		View: "window_stats",
		WindowStats: &tracequery.WindowStats{IRQBursts: []tracequery.IRQBurstSummary{{
			CPU: 3, IRQ: 17, Name: "timer", Count: 4, SpanMs: 2.5,
			// A legacy producer could still populate this compatibility field;
			// the publication face must never revive it as active duration.
			DurationMs: 99, LineStart: 10, LineEnd: 20,
		}}},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "/tmp/payload.json")
	if !strings.Contains(summary, "irq_burst cpu=3 irq=17 name=timer count=4 span=2.500ms duration_basis=inventory_not_active") {
		t.Fatalf("IRQ burst inventory semantics missing from summary:\n%s", summary)
	}
	if strings.Contains(summary, "duration=99.000ms") {
		t.Fatalf("legacy burst envelope leaked as active duration:\n%s", summary)
	}
}
