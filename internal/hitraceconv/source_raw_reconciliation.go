package hitraceconv

import (
	"fmt"
	"sort"
	"strings"
)

func traceDBRawDecodeReconciliationCoverage(items []TraceDBCoverage) TraceDBCoverage {
	out := TraceDBCoverage{
		Family: "source_rawtrace_reconciliation",
		Table:  "__raw_vs_trace_streamer__",
		Role:   "diagnostic_reconciliation",
		FieldSources: map[string]string{
			"raw":            "exact per-format physical record counts and strict-body decisions from source_rawtrace_decode",
			"trace_streamer": "exact selected event/status counters from the complete official stat table; explicit zero is preserved",
			"db":             "typed Codrax exporter rows from the normalized SQLite DB; only the blocked-reason exporter has an exact event-family comparison in RPD-1",
			"effect":         "diagnostic equality/closure checks only; comparisons never publish, suppress, deduplicate, or fabricate a trace row",
		},
		Metadata: map[string]string{
			"reconciliation_state":  "unavailable",
			"publication_authority": "withheld_requires_rpd2_deduplication",
		},
	}
	raw, rawOK := traceDBCoverageByIdentity(items,
		"source_rawtrace_decode", "__raw_record_decode__", "diagnostic_ledger")
	if !rawOK {
		out.Skipped = "raw decode ledger absent"
		return out
	}
	out.Found = raw.Found
	out.RowsRead = raw.RowsRead
	if !raw.Found || raw.Metadata["decode_state"] != "strict_target_ledger_complete" {
		out.Metadata["reconciliation_state"] = "withheld_raw_decode_incomplete"
		out.Skipped = "raw/trace_streamer reconciliation withheld: strict raw decode ledger is incomplete"
		return out
	}
	capture, captureOK := traceDBCoverageByIdentity(items,
		"capture_completeness", "stat", "capture_completeness")
	if !captureOK || capture.CaptureCompleteness == nil ||
		capture.CaptureCompleteness.State == traceCaptureCompletenessUnknown {
		out.Metadata["reconciliation_state"] = "withheld_trace_streamer_stat_unavailable"
		out.Skipped = "raw/trace_streamer reconciliation withheld: complete trace_streamer stat authority is unavailable"
		return out
	}

	absent := traceDBRawDecodeCSVSet(raw.Metadata["target_formats_absent"])
	var witnesses []string
	exactClosures := 0
	mismatches := 0
	for _, name := range traceDBRawDecodeTargetNames() {
		metricName := traceDBRawDecodeMetricName(name)
		rawMetric := "target_" + metricName + "_records"
		rawCount := raw.Metrics[rawMetric]
		formatPresent := !absent[name]
		statValues, statComplete := traceDBRawDecodeSelectedStats(capture, metricName)
		if !formatPresent && !statComplete {
			continue
		}
		if out.Metrics == nil {
			out.Metrics = map[string]int64{}
		}
		if formatPresent {
			out.Metrics["raw_"+metricName+"_records"] = rawCount
		}
		parts := []string{name}
		if formatPresent {
			parts = append(parts, fmt.Sprintf("raw=%d", rawCount))
		} else {
			parts = append(parts, "raw_format=absent")
		}
		if statComplete {
			statSum := int64(0)
			for _, status := range traceDBRawDecodeStatTypes() {
				value := statValues[status]
				out.Metrics["trace_streamer_"+metricName+"_"+status] = value
				statSum += value
			}
			out.Metrics["trace_streamer_"+metricName+"_stat_sum"] = statSum
			parts = append(parts, fmt.Sprintf(
				"received=%d/data_lost=%d/not_match=%d/not_supported=%d/invalid_data=%d",
				statValues["received"], statValues["data_lost"], statValues["not_match"],
				statValues["not_supported"], statValues["invalid_data"]))
			if formatPresent {
				if rawCount == statSum {
					out.Metrics["exact_raw_stat_closures"]++
					exactClosures++
					parts = append(parts, "raw_stat_closure=exact")
				} else {
					out.Metrics["raw_stat_closure_mismatches"]++
					mismatches++
					parts = append(parts, "raw_stat_closure=mismatch")
				}
			}
		} else {
			parts = append(parts, "trace_streamer_stat=absent")
		}
		witnesses = append(witnesses, strings.Join(parts, "/"))
	}

	blocked, blockedOK := traceDBCoverageByIdentity(items,
		"scheduler", "thread_state.arg_setid", "query_ready_export")
	if blockedOK && blocked.Found && blocked.Error == "" {
		out.Metrics["db_sched_blocked_reason_rows_emitted"] = int64(blocked.RowsEmitted)
		blockedStats, receivedOK := traceDBRawDecodeSelectedStats(capture, "sched_blocked_reason")
		received := blockedStats["received"]
		if receivedOK && int64(blocked.RowsEmitted) == received {
			out.Metadata["sched_blocked_reason_db_received_closure"] = "exact"
		} else if receivedOK {
			out.Metadata["sched_blocked_reason_db_received_closure"] = "mismatch"
		}
	} else {
		out.Metadata["sched_blocked_reason_db_received_closure"] = "unavailable"
	}
	dma, dmaOK := traceDBCoverageByIdentity(items, "slice", "dma_fence", "unsupported_input")
	if dmaOK && dma.Found && dma.Error == "" {
		out.Metrics["db_dma_fence_high_level_rows_withheld"] = int64(dma.RowsRead)
		out.Metadata["dma_fence_db_comparison"] =
			"non_equivalent_high_level_activity_rows_vs_raw_event_family_mix"
	}
	if absent["trace_vsync"] {
		out.Metadata["trace_vsync_recovery_state"] =
			"not_present_in_raw_ftrace_event_format; requires_other_source_segment_or_upstream_retained_row"
	}
	sort.Strings(witnesses)
	if len(witnesses) > 0 {
		out.Metadata["family_reconciliation_witnesses"] = strings.Join(witnesses, ";")
	}
	out.RowsEmitted = 0
	switch {
	case mismatches > 0:
		out.Metadata["reconciliation_state"] = "complete_with_exact_count_mismatch"
	case exactClosures > 0:
		out.Metadata["reconciliation_state"] = "complete_with_exact_closures"
	default:
		out.Metadata["reconciliation_state"] = "complete_without_comparable_family"
	}
	return out
}

func traceDBCoverageByIdentity(
	items []TraceDBCoverage,
	family, table, role string,
) (TraceDBCoverage, bool) {
	for _, item := range items {
		if item.Family == family && item.Table == table && item.Role == role {
			return item, true
		}
	}
	return TraceDBCoverage{}, false
}

func traceDBRawDecodeCSVSet(value string) map[string]bool {
	out := map[string]bool{}
	for _, item := range strings.Split(value, ",") {
		if item != "" {
			out[item] = true
		}
	}
	return out
}

func traceDBRawDecodeStatTypes() []string {
	return []string{"received", "data_lost", "not_match", "not_supported", "invalid_data"}
}

func traceDBRawDecodeSelectedStats(
	capture TraceDBCoverage,
	metricName string,
) (map[string]int64, bool) {
	if capture.Metrics["selected_"+metricName+"_status_mask"] != int64(traceDBCaptureAllStatTypes) {
		return nil, false
	}
	out := make(map[string]int64, len(traceDBRawDecodeStatTypes()))
	for _, status := range traceDBRawDecodeStatTypes() {
		out[status] = capture.Metrics["selected_"+metricName+"_"+status]
	}
	return out, true
}
