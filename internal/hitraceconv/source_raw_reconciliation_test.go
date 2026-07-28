package hitraceconv

import (
	"strings"
	"testing"
)

func TestTraceDBRawDecodeFormatWitnessNameBoundsUntrustedDescriptorText(t *testing.T) {
	unsafe := "event\t" + strings.Repeat("x", 256)
	got := traceDBRawDecodeFormatWitnessName(unsafe)
	if strings.Contains(got, unsafe) || !strings.HasPrefix(got, "name_sha256_") ||
		!strings.HasSuffix(got, "_bytes_262") {
		t.Fatalf("unsafe event-format name escaped bounded hash witness: %q", got)
	}
	if got := traceDBRawDecodeFormatWitnessName("sched_blocked_reason"); got != "sched_blocked_reason" {
		t.Fatalf("safe closed event name was not preserved: %q", got)
	}
}

func TestTraceDBRawDecodeReconciliationClosesBlockedReasonAndKeepsVSyncSeparate(t *testing.T) {
	captureMetrics := map[string]int64{}
	for _, name := range traceDBRawDecodeTargetNames() {
		captureMetrics["selected_"+traceDBRawDecodeMetricName(name)+"_status_mask"] =
			int64(traceDBCaptureAllStatTypes)
	}
	captureMetrics["selected_sched_blocked_reason_received"] = 1795
	captureMetrics["selected_sched_blocked_reason_not_match"] = 19771
	captureMetrics["selected_trace_vsync_received"] = 12
	captureMetrics["selected_trace_vsync_not_match"] = 79
	var absent []string
	for _, name := range traceDBRawDecodeTargetNames() {
		if name != "sched_blocked_reason" {
			absent = append(absent, name)
		}
	}
	items := []TraceDBCoverage{
		{
			Family: "source_rawtrace_decode", Table: "__raw_record_decode__", Role: "diagnostic_ledger",
			Found: true, RowsRead: 491411,
			Metrics: map[string]int64{
				"target_sched_blocked_reason_records": 21566,
			},
			Metadata: map[string]string{
				"decode_state":          "strict_target_ledger_complete",
				"target_formats_absent": strings.Join(absent, ","),
			},
		},
		{
			Family: "capture_completeness", Table: "stat", Role: "capture_completeness",
			Found: true, Metrics: captureMetrics,
			CaptureCompleteness: &TraceCaptureCompleteness{
				State: traceCaptureCompletenessDegraded,
			},
		},
		{
			Family: "scheduler", Table: "thread_state.arg_setid", Role: "query_ready_export",
			Found: true, RowsRead: 1795, RowsEmitted: 1795,
		},
		{
			Family: "slice", Table: "dma_fence", Role: "unsupported_input",
			Found: true, RowsRead: 1787,
		},
	}
	got := traceDBRawDecodeReconciliationCoverage(items)
	if !got.Found || got.RowsRead != 491411 || got.RowsEmitted != 0 ||
		got.Metadata["reconciliation_state"] != "complete_with_exact_closures" ||
		got.Metadata["sched_blocked_reason_db_received_closure"] != "exact" ||
		got.Metadata["trace_vsync_recovery_state"] == "" ||
		got.Metrics["raw_sched_blocked_reason_records"] != 21566 ||
		got.Metrics["trace_streamer_sched_blocked_reason_stat_sum"] != 21566 ||
		got.Metrics["db_sched_blocked_reason_rows_emitted"] != 1795 ||
		got.Metrics["exact_raw_stat_closures"] != 1 ||
		got.Metrics["raw_stat_closure_mismatches"] != 0 {
		t.Fatalf("raw/trace_streamer reconciliation mismatch: %+v", got)
	}
}
