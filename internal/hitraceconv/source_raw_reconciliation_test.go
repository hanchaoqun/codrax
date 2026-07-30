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
	captureMetrics["selected_sched_blocked_reason_not_match"] = 19771
	captureMetrics["selected_sched_blocked_reason_received"] = 23361
	captureMetrics["selected_trace_vsync_received"] = 12
	captureMetrics["selected_trace_vsync_not_match"] = 79
	captureMetrics["selected_tracing_mark_write_received"] = 182683
	captureMetrics["selected_tracing_mark_write_invalid_data"] = 2
	var absent []string
	for _, name := range traceDBRawDecodeTargetNames() {
		if name != "print" && name != "sched_blocked_reason" && name != "tracing_mark_write" {
			absent = append(absent, name)
		}
	}
	items := []TraceDBCoverage{
		{
			Family: "source_rawtrace_decode", Table: "__raw_record_decode__", Role: "diagnostic_ledger",
			Found: true, RowsRead: 491411,
			Metrics: map[string]int64{
				"target_sched_blocked_reason_records": 21566,
				"target_print_records":                175165,
				"target_tracing_mark_write_records":   7518,
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
		got.Metadata["reconciliation_state"] != "complete_with_exact_relations" ||
		got.Metadata["sched_blocked_reason_counter_profile"] !=
			"exact_raw_equals_db_plus_not_match_and_received_equals_raw_plus_db" ||
		got.Metadata["tracing_marker_group_received_relation"] !=
			"exact_print_plus_tracing_mark_write_equals_received" ||
		got.Metadata["tracing_marker_invalid_data_relation"] !=
			"overlapping_subset_within_received_not_additive" ||
		got.Metadata["trace_vsync_recovery_state"] == "" ||
		got.Metrics["raw_sched_blocked_reason_records"] != 21566 ||
		got.Metrics["trace_streamer_sched_blocked_reason_received"] != 23361 ||
		got.Metrics["db_sched_blocked_reason_rows_emitted"] != 1795 ||
		got.Metrics["sched_blocked_reason_exact_counter_equations"] != 2 ||
		got.Metrics["tracing_marker_exact_group_relations"] != 1 ||
		got.Metrics["raw_received_relation_mismatches"] != 0 ||
		got.Metadata["publication_authority"] !=
			"withheld_requires_rpd2_exact_record_deduplication" {
		t.Fatalf("raw/trace_streamer reconciliation mismatch: %+v", got)
	}
	for metric := range got.Metrics {
		if strings.HasSuffix(metric, "_stat_sum") {
			t.Fatalf("overlapping TraceStreamer statuses were still added: %s=%d", metric, got.Metrics[metric])
		}
	}
}

func TestTraceDBRawDecodeReconciliationUsesSuppressedDBCounterUnderRawFamilyAuthority(t *testing.T) {
	captureMetrics := map[string]int64{}
	for _, name := range traceDBRawDecodeTargetNames() {
		captureMetrics["selected_"+traceDBRawDecodeMetricName(name)+"_status_mask"] =
			int64(traceDBCaptureAllStatTypes)
	}
	captureMetrics["selected_sched_blocked_reason_not_match"] = 19771
	captureMetrics["selected_sched_blocked_reason_received"] = 23361
	var absent []string
	for _, name := range traceDBRawDecodeTargetNames() {
		if name != "sched_blocked_reason" {
			absent = append(absent, name)
		}
	}
	got := traceDBRawDecodeReconciliationCoverage([]TraceDBCoverage{
		{
			Family: "source_rawtrace_decode",
			Table:  "__raw_record_decode__", Role: "diagnostic_ledger",
			Found: true,
			Metrics: map[string]int64{
				"target_sched_blocked_reason_records": 21566,
			},
			Metadata: map[string]string{
				"decode_state":          "strict_target_ledger_complete",
				"target_formats_absent": strings.Join(absent, ","),
			},
		},
		{
			Family: "capture_completeness", Table: "stat",
			Role: "capture_completeness", Found: true,
			Metrics: captureMetrics,
			CaptureCompleteness: &TraceCaptureCompleteness{
				State: traceCaptureCompletenessDegraded,
			},
		},
		{
			Family: "scheduler", Table: "thread_state.arg_setid",
			Role: "query_ready_export", Found: true, RowsEmitted: 0,
		},
		{
			Family: "source_rawtrace_blocked_key",
			Table:  "__raw_vs_db_blocked_key__",
			Role:   "diagnostic_deduplication",
			Metadata: map[string]string{
				"ledger_state": "exact_raw_family_authority",
			},
			Metrics: map[string]int64{
				"db_rows_suppressed_by_raw_family": 1795,
			},
		},
		{
			Family: "source_rawtrace_blocked_recovery",
			Table:  "__raw_only_blocked_reason__",
			Role:   "query_ready_export", RowsEmitted: 21565,
		},
	})
	if got.Metadata["sched_blocked_reason_counter_profile"] !=
		"exact_raw_equals_db_plus_not_match_and_received_equals_raw_plus_db" ||
		got.Metadata["sched_blocked_reason_db_counter_source"] !=
			"validated_DB_projection_candidates_suppressed_by_complete_raw_family_authority" ||
		got.Metadata["publication_authority"] !=
			"source_rawtrace_blocked_family_authority" ||
		got.Metrics["db_sched_blocked_reason_rows_emitted"] != 0 ||
		got.Metrics["db_sched_blocked_reason_rows_suppressed_by_raw_family"] != 1795 ||
		got.Metrics["raw_sched_blocked_reason_rows_published"] != 21565 ||
		got.Metrics["sched_blocked_reason_exact_counter_equations"] != 2 {
		t.Fatalf("raw family authority corrupted upstream blocked counter reconciliation: %+v",
			got)
	}
}

func TestTraceDBRawDecodeTargetGeometryWitnessesBoundsFields(t *testing.T) {
	fields := make([]eventField, maxTraceDBRawDecodeFieldsPerFormat+2)
	for index := range fields {
		fields[index] = eventField{
			Name:   "field_" + string(rune('a'+index%26)),
			Offset: index * 2,
			Size:   2,
			Signed: index%2 == 0,
		}
	}
	catalog := eventFormatCatalog{Formats: map[int]eventFormat{
		7: {ID: 7, Name: "sched_wakeup_new", Fields: fields},
		8: {ID: 8, Name: "unrelated", Fields: fields},
	}}
	witnesses, omitted := traceDBRawDecodeTargetGeometryWitnesses(catalog)
	if len(witnesses) != 1 || omitted != 2 ||
		!strings.HasPrefix(witnesses[0], "sched_wakeup_new#7[") ||
		strings.Contains(witnesses[0], "unrelated") {
		t.Fatalf("bounded target geometry mismatch: witnesses=%q omitted=%d", witnesses, omitted)
	}
}

func TestTraceDBRawCountAddRejectsOverflowAndNegativeCounts(t *testing.T) {
	if _, ok := traceDBRawCountAdd(-1, 1); ok {
		t.Fatal("negative count admitted")
	}
	if _, ok := traceDBRawCountAdd(int64(^uint64(0)>>1), 1); ok {
		t.Fatal("overflowing count addition admitted")
	}
	if got, ok := traceDBRawCountAdd(175165, 7518); !ok || got != 182683 {
		t.Fatalf("exact marker group addition failed: got=%d ok=%t", got, ok)
	}
}
