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

func TestMaterializeRuntimeTraceVsyncAuthorityCaveat(t *testing.T) {
	// GAP-B3 (§13.3): census typed 观测在账本时,答案面出周期权威注——
	// 消费者回调间距不得冒充信号周期/信号丢失证据。
	censusResult := types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:census#1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "vsync_generator_census",
			ClaimKey:        "vsync_generator_census:VSyncGenerator",
			Subject:         "VSyncGenerator-611",
			Object:          "generator",
			RichNotes:       []string{"vsync_generator_census_period_prints=3"},
			Confidence:      0.9,
		}},
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	ctx := &types.BusContext{ToolResults: []types.ToolResult{censusResult}}
	if !materializeRuntimeTraceVsyncAuthorityCaveat(doc, ctx) {
		t.Fatal("census presence must mint the vsync authority caveat")
	}
	got := strings.Join(doc.Caveats, "\n")
	for _, want := range []string{
		"VSync 周期权威：帧节拍发生器普查=1 个发生器（period_prints=3）",
		"不得当作 vsync 信号周期或「信号丢失」的证据",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("vsync authority caveat missing %q:\n%s", want, got)
		}
	}
	quiet := &types.AnswerDocumentV2{DocumentModel: "v2"}
	if materializeRuntimeTraceVsyncAuthorityCaveat(quiet, &types.BusContext{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true}}}) {
		t.Fatalf("census-free ledger must stay silent: %v", quiet.Caveats)
	}
}
