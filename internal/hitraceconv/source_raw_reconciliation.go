package hitraceconv

import (
	"fmt"
	"math"
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
			"trace_streamer": "exact selected event/status counters from the complete official stat table; statuses are event-specific stage counters which may overlap and are never generically summed",
			"db":             "typed Codrax exporter rows from the normalized SQLite DB; blocked-reason equations use only exact emitted rows",
			"effect":         "diagnostic equality/closure checks only; comparisons never publish, suppress, deduplicate, or fabricate a trace row",
		},
		Metadata: map[string]string{
			"reconciliation_state":    "unavailable",
			"stat_counter_semantics":  "event_specific_stage_counters_may_overlap_never_sum",
			"publication_authority":   "withheld_requires_rpd2_exact_record_deduplication",
			"blocked_duplicate_state": "not_evaluated_requires_exact_raw_db_record_keys",
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
	if !raw.Found || !traceDBRawDecodeCensusComplete(raw) {
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
	exactRelations := 0
	relationMismatches := 0
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
			for _, status := range traceDBRawDecodeStatTypes() {
				value := statValues[status]
				out.Metrics["trace_streamer_"+metricName+"_"+status] = value
			}
			parts = append(parts, fmt.Sprintf(
				"received=%d/data_lost=%d/not_match=%d/not_supported=%d/invalid_data=%d",
				statValues["received"], statValues["data_lost"], statValues["not_match"],
				statValues["not_supported"], statValues["invalid_data"]))
			if formatPresent && traceDBRawReceivedDirectlyComparable(name) {
				if rawCount == statValues["received"] {
					out.Metrics["exact_raw_received_relations"]++
					exactRelations++
					parts = append(parts, "raw_received_relation=exact")
				} else {
					out.Metrics["raw_received_relation_mismatches"]++
					relationMismatches++
					parts = append(parts, "raw_received_relation=mismatch")
				}
			} else if formatPresent {
				parts = append(parts, "raw_received_relation=family_specific")
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
		dbCounterCount := int64(blocked.RowsEmitted)
		if blockedKey, ok := traceDBCoverageByIdentity(
			items, "source_rawtrace_blocked_key",
			"__raw_vs_db_blocked_key__", "diagnostic_deduplication"); ok &&
			blockedKey.Metadata["ledger_state"] ==
				"exact_raw_family_authority" {
			dbCounterCount =
				blockedKey.Metrics["db_rows_suppressed_by_raw_family"]
			out.Metrics["db_sched_blocked_reason_rows_suppressed_by_raw_family"] =
				dbCounterCount
			out.Metadata["sched_blocked_reason_db_counter_source"] =
				"validated_DB_projection_candidates_suppressed_by_complete_raw_family_authority"
			out.Metadata["publication_authority"] =
				"source_rawtrace_blocked_family_authority"
			if rawPublished, present := traceDBCoverageByIdentity(
				items, "source_rawtrace_blocked_recovery",
				"__raw_only_blocked_reason__", "query_ready_export"); present {
				out.Metrics["raw_sched_blocked_reason_rows_published"] =
					int64(rawPublished.RowsEmitted)
			}
		} else {
			out.Metadata["sched_blocked_reason_db_counter_source"] =
				"published_DB_projection_rows"
		}
		blockedStats, statsOK := traceDBRawDecodeSelectedStats(capture, "sched_blocked_reason")
		rawCount := raw.Metrics["target_sched_blocked_reason_records"]
		rawPresent := !absent["sched_blocked_reason"]
		dbCount := dbCounterCount
		if statsOK && rawPresent {
			rawFromDBAndNotMatch, firstOK := traceDBRawCountAdd(dbCount, blockedStats["not_match"])
			receivedFromRawAndDB, secondOK := traceDBRawCountAdd(rawCount, dbCount)
			firstExact := firstOK && rawCount == rawFromDBAndNotMatch
			secondExact := secondOK && blockedStats["received"] == receivedFromRawAndDB
			switch {
			case firstExact && secondExact:
				out.Metadata["sched_blocked_reason_counter_profile"] =
					"exact_raw_equals_db_plus_not_match_and_received_equals_raw_plus_db"
				out.Metrics["sched_blocked_reason_exact_counter_equations"] = 2
				exactRelations += 2
			default:
				out.Metadata["sched_blocked_reason_counter_profile"] =
					"exact_equation_mismatch_publication_withheld"
				if !firstExact {
					out.Metrics["sched_blocked_reason_raw_db_not_match_mismatches"] = 1
					relationMismatches++
				}
				if !secondExact {
					out.Metrics["sched_blocked_reason_received_raw_db_mismatches"] = 1
					relationMismatches++
				}
			}
			out.Metadata["sched_blocked_reason_upstream_semantics"] =
				"rawtrace parser increments received for every physical event and again after successful thread-state attachment; failures increment not_match"
		} else {
			out.Metadata["sched_blocked_reason_counter_profile"] = "unavailable"
		}
	} else {
		out.Metadata["sched_blocked_reason_counter_profile"] = "unavailable"
	}
	printPresent := !absent["print"]
	tracingMarkerPresent := !absent["tracing_mark_write"]
	markerStats, markerStatsOK := traceDBRawDecodeSelectedStats(capture, "tracing_mark_write")
	if printPresent && tracingMarkerPresent && markerStatsOK {
		rawMarkerTotal, addOK := traceDBRawCountAdd(
			raw.Metrics["target_print_records"],
			raw.Metrics["target_tracing_mark_write_records"])
		if addOK && rawMarkerTotal == markerStats["received"] {
			out.Metadata["tracing_marker_group_received_relation"] =
				"exact_print_plus_tracing_mark_write_equals_received"
			out.Metrics["tracing_marker_exact_group_relations"] = 1
			exactRelations++
		} else {
			out.Metadata["tracing_marker_group_received_relation"] = "mismatch"
			out.Metrics["tracing_marker_group_relation_mismatches"] = 1
			relationMismatches++
		}
		if markerStats["invalid_data"] <= markerStats["received"] {
			out.Metadata["tracing_marker_invalid_data_relation"] =
				"overlapping_subset_within_received_not_additive"
		} else {
			out.Metadata["tracing_marker_invalid_data_relation"] =
				"invalid_subset_bound_mismatch"
			relationMismatches++
		}
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
	case relationMismatches > 0:
		out.Metadata["reconciliation_state"] = "complete_with_exact_relation_mismatch"
	case exactRelations > 0:
		out.Metadata["reconciliation_state"] = "complete_with_exact_relations"
	default:
		out.Metadata["reconciliation_state"] = "complete_without_comparable_family"
	}
	return out
}

func traceDBRawReceivedDirectlyComparable(name string) bool {
	switch name {
	case "dma_fence_destroy", "dma_fence_enable_signal", "dma_fence_init",
		"dma_fence_signaled", "sched_wakeup_new":
		return true
	default:
		return false
	}
}

func traceDBRawCountAdd(left, right int64) (int64, bool) {
	if left < 0 || right < 0 || left > math.MaxInt64-right {
		return 0, false
	}
	return left + right, true
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
