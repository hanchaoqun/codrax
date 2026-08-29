package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryTypedObservationsPublishesWindowScopedWakeupTargetCPUIntegrity(t *testing.T) {
	result := tracequery.Result{
		View: "window_stats", SourcePath: "/tmp/customer.systrace", TimeStart: 10, TimeEnd: 11,
		WindowStats: &tracequery.WindowStats{
			Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11, StartSet: true},
			WakeupTargetCPUIntegrity: &tracequery.WakeupTargetCPUIntegritySummary{
				Status:        tracequery.WakeupTargetCPUIntegritySuspectedDegradedAllZero,
				ObservedCount: 1697, ZeroCount: 1697, EmitterCPUCount: 6,
			},
		},
	}
	records := traceQueryTypedObservations(result, "customer.systrace", "payload", "raw", "", time.Unix(0, 0).UTC())
	var got *types.ObservationRecord
	for i := range records {
		if records[i].Predicate == "wakeup_target_cpu_integrity" {
			got = &records[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("typed integrity observation missing: %+v", records)
	}
	if got.Value != "1697" || got.Unit != "events" || got.Span.StartTs != 10 || got.Span.EndTs != 11 {
		t.Fatalf("integrity census/window mismatch: %+v", got)
	}
	joined := strings.Join(got.RichNotes, " ")
	for _, want := range []string{
		types.TraceNoteKeyWakeupTargetCPUObservedCount + "=1697",
		types.TraceNoteKeyWakeupTargetCPUZeroCount + "=1697",
		types.TraceNoteKeyWakeupTargetCPUEmitterCPUCount + "=6",
		"raw_rows_preserved",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("typed integrity note missing %q: %+v", want, got.RichNotes)
		}
	}

	result.WindowStats.WakeupTargetCPUIntegrity.ZeroCount = 1696
	for _, record := range traceQueryTypedObservations(result, "customer.systrace", "payload2", "raw2", "", time.Unix(0, 0).UTC()) {
		if record.Predicate == "wakeup_target_cpu_integrity" {
			t.Fatalf("an inconsistent census must not publish typed integrity: %+v", record)
		}
	}
}
