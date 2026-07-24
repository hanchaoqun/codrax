package tool

// NW-04 接应 (P3, 2026-07-24) — the count-only IO activity index gets one
// actionable next-step row (upgrade counts to wall-clock evidence on the SAME
// window). Precise trigger = the typed io_pressure_evidence_quality note the
// projection caliber text already gates on; guidance-only effect.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeTraceNextStepCountOnlyIOStep(t *testing.T) {
	marker := types.TraceNoteKeyIOPressureEvidenceQuality + "=" + tracequery.IOPressureEvidenceQualityActivityMarkerOnly
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{{
		Predicate: "io_pressure",
		RichNotes: []string{"tier=background", marker},
	}}}
	zh := runtimeTraceNextStepCountOnlyIOStep(ledger, true)
	if !strings.Contains(zh, "纯计数口径") || !strings.Contains(zh, "critical_blocking_calls") ||
		!strings.Contains(zh, "同一分析窗") {
		t.Fatalf("zh count-only IO next-step drifted: %q", zh)
	}
	en := runtimeTraceNextStepCountOnlyIOStep(ledger, false)
	if !strings.Contains(en, "count-only") || !strings.Contains(en, "critical_blocking_calls") ||
		!strings.Contains(en, "same analysis window") {
		t.Fatalf("en count-only IO next-step drifted: %q", en)
	}
	// Wall-clock corroborated (or note-free) ledgers stay silent.
	quiet := types.ObservationLedger{Records: []types.ObservationRecord{{
		Predicate: "io_pressure",
		RichNotes: []string{types.TraceNoteKeyIOPressureEvidenceQuality + "=wall_clock_or_latency_corroborated"},
	}}}
	if got := runtimeTraceNextStepCountOnlyIOStep(quiet, true); got != "" {
		t.Fatalf("corroborated ledger must not mint the count-only follow-up: %q", got)
	}
	if got := runtimeTraceNextStepCountOnlyIOStep(types.ObservationLedger{}, true); got != "" {
		t.Fatalf("empty ledger must stay silent: %q", got)
	}
}
