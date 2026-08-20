package agent

// answer_document_evaluator_composite_unit_test.go — 终判⑧ (§29.96.2, ledger
// docs/design/real_trace_campaign_20260705.md, 2026-07-15) digest word-face
// pins: an observation record whose Unit carries the typed composite-score
// caliber token renders the established non-wall-clock caliber word (QH2-A
// 站①/站② family) instead of the retired 值=X ms lie; ordinary unit records
// keep the legacy suffix concatenation byte-identically.
//
// MUTATION self-check: removing the token arm in traceQueryObservationValue
// reds the composite assertions (值=61.540composite_score would render);
// widening the arm reds the legacy byte-identity arm.

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryObservationValueCompositeCaliberToken(t *testing.T) {
	composite := types.ObservationRecord{
		Value: "61.540",
		Unit:  types.TraceObservationUnitCompositeScore,
	}
	if got := traceQueryObservationValue(composite, true); got != "值=61.540(综合评分,非墙钟)" {
		t.Fatalf("zh digest must render the composite caliber word face, got %q", got)
	}
	if got := traceQueryObservationValue(composite, false); got != "value=61.540 (composite score, not wall clock)" {
		t.Fatalf("en digest must render the composite caliber word face, got %q", got)
	}

	// Legacy units keep the suffix concatenation byte-identically.
	wall := types.ObservationRecord{Value: "4.000", Unit: "ms"}
	if got := traceQueryObservationValue(wall, true); got != "值=4.000ms" {
		t.Fatalf("ms records keep the legacy zh form, got %q", got)
	}
	if got := traceQueryObservationValue(wall, false); got != "value=4.000ms" {
		t.Fatalf("ms records keep the legacy en form, got %q", got)
	}
	events := types.ObservationRecord{Value: "20", Unit: "events"}
	if got := traceQueryObservationValue(events, true); got != "值=20events" {
		t.Fatalf("events records keep the legacy zh form, got %q", got)
	}
}

func TestTraceQueryObservationValueIOPressureUsesDedicatedActivityIndex(t *testing.T) {
	record := types.ObservationRecord{
		Object: "io_pressure",
		Value:  "7.000",
		Unit:   types.TraceObservationUnitCompositeScore,
		RichNotes: []string{
			types.TraceNoteKeyType + "=io_pressure",
			types.TraceNoteKeyIOPressureActivityIndex + "=61.540",
		},
	}
	if got := traceQueryObservationValue(record, true); got != "值=61.540(IO活动综合指数,非墙钟)" {
		t.Fatalf("zh IO-pressure digest must use its dedicated activity-index authority, got %q", got)
	}
	if got := traceQueryObservationValue(record, false); got != "value=61.540 (IO activity index, not wall clock)" {
		t.Fatalf("en IO-pressure digest must use its dedicated activity-index authority, got %q", got)
	}

	legacy := record
	legacy.RichNotes = []string{types.TraceNoteKeyType + "=io_pressure"}
	if got := traceQueryObservationValue(legacy, true); got != "值=7.000(IO活动综合指数,非墙钟)" {
		t.Fatalf("legacy typed IO-pressure rows must keep the activity-index caliber, got %q", got)
	}
}
