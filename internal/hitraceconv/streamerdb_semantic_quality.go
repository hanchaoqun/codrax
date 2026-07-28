package hitraceconv

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	traceDBSemanticQualityFamily = "conversion_quality"
	traceDBSemanticQualityTable  = "__semantic_quality__"
)

func traceDBAddCoverageMetric(item *TraceDBCoverage, key string, delta int64) {
	if item == nil || key == "" || delta == 0 {
		return
	}
	if item.Metrics == nil {
		item.Metrics = map[string]int64{}
	}
	item.Metrics[key] += delta
}

func traceDBSemanticQualityCoverage(items []TraceDBCoverage) TraceDBCoverage {
	quality := TraceDBCoverage{
		Family: traceDBSemanticQualityFamily,
		Table:  traceDBSemanticQualityTable,
		Role:   "semantic_quality_summary",
		Found:  true,
		FieldSources: map[string]string{
			"authority": "exact counters copied from typed DB resolver/exporter decisions after shared sync-span reconciliation; replacement closure uses exact submitted/emitted/suppressed counters only; advisory only and never a conversion hard gate",
		},
		Metadata: map[string]string{},
	}
	copyMetric := func(target string, family string, table string, source string) {
		for _, item := range items {
			if item.Family != family || item.Table != table {
				continue
			}
			if value := item.Metrics[source]; value != 0 {
				traceDBAddCoverageMetric(&quality, target, value)
			}
		}
	}
	copyMetric("unnamed_threads", "resolver", "thread", "unnamed_threads")
	copyMetric("unresolved_thread_names", "resolver", "thread", "unresolved_thread_names")
	copyMetric("thread_names_recovered_main_process", "resolver", "thread", "thread_names_recovered_main_process")
	copyMetric("thread_names_recovered_unique_public_tid", "resolver", "thread", "thread_names_recovered_unique_public_tid")
	copyMetric("thread_names_recovered_source_cmdline", "resolver", "thread", "thread_names_recovered_source_cmdline")
	copyMetric("thread_names_recovered_source_wakeup_new", "resolver", "thread",
		"thread_names_recovered_source_wakeup_new")
	copyMetric("public_tids_with_multiple_itids", "resolver", "thread", "public_tids_with_multiple_itids")
	copyMetric("public_tids_with_multiple_owner_ipids", "resolver", "thread", "public_tids_with_multiple_owner_ipids")
	copyMetric("scheduler_boundaries_with_unknown_comm", "scheduler", "sched_slice", "boundaries_with_unknown_comm")
	copyMetric("callstack_source_rows_suppressed_pre_pairing", "slice", "callstack", "source_rows_suppressed_pre_pairing")
	copyMetric("callstack_async_source_rows_suppressed_post_pairing", "slice", "callstack", "async_source_rows_suppressed_post_pairing")
	copyMetric("callstack_source_rows_official_async_shaped", "slice", "callstack",
		"source_rows_official_async_shaped")
	copyMetric("callstack_source_rows_withheld_official_async_interval", "slice", "callstack",
		"source_rows_withheld_official_async_interval")
	copyMetric("callstack_source_rows_emitted_official_async_interval", "slice", "callstack",
		"source_rows_emitted_official_async_interval")
	copyMetric("callstack_source_rows_emitted_official_async_raw_pair", "slice", "callstack",
		"source_rows_emitted_official_async_raw_pair")
	copyMetric("callstack_source_rows_rejected_official_async_shape", "slice", "callstack",
		"source_rows_rejected_official_async_shape")
	copyMetric("callstack_source_rows_official_async_child_emitter_resolved", "slice", "callstack",
		"source_rows_official_async_child_emitter_resolved")
	copyMetric("callstack_source_rows_unfinished_duration_sentinel", "slice", "callstack",
		"source_rows_unfinished_duration_sentinel")
	copyMetric("callstack_source_rows_with_distributed_metadata", "slice", "callstack",
		"source_rows_with_distributed_metadata")
	copyMetric("callstack_source_rows_suppressed_cpu_unavailable", "slice", "callstack", "source_rows_suppressed_cpu_unavailable")
	copyMetric("callstack_source_rows_preserved_cpu_unavailable", "slice", "callstack", "source_rows_preserved_cpu_unavailable")
	copyMetric("callstack_source_rows_admitted_exact_name_pre_pairing", "slice", "callstack",
		"source_rows_admitted_exact_name_pre_pairing")
	copyMetric("callstack_source_rows_suppressed_identity", "slice", "callstack", "source_rows_suppressed_identity")
	copyMetric("callstack_sync_spans_suppressed", "slice", "callstack", "sync_spans_suppressed")
	copyMetric("callstack_sync_spans_suppressed_by_local_fence", "slice", "callstack",
		"sync_spans_suppressed_by_local_fence")
	copyMetric("raw_marker_pairs_unique_cpu_unavailable_callstack_candidate",
		"source_rawtrace_marker_sync", "__raw_marker_sync__",
		"raw_pairs_unique_cpu_unavailable_callstack_candidate")
	copyMetric("raw_marker_pairs_unique_cpu_known_callstack_candidate",
		"source_rawtrace_marker_sync", "__raw_marker_sync__",
		"raw_pairs_unique_cpu_known_callstack_candidate")
	copyMetric("raw_marker_pairs_ambiguous_candidate_set",
		"source_rawtrace_marker_sync", "__raw_marker_sync__",
		"raw_pairs_ambiguous_candidate_set")
	copyMetric("callstack_source_rows_recovered_same_public_tid_scheduler_alias", "slice", "callstack",
		"source_rows_recovered_same_public_tid_scheduler_alias")
	copyMetric("unclassified_nonempty_tables", "conversion_inventory", "__table_inventory__",
		"unclassified_nonempty_tables")
	copyMetric("unclassified_uninspectable_tables", "conversion_inventory", "__table_inventory__",
		"unclassified_uninspectable_tables")
	copyMetric("table_inventory_truncated", "conversion_inventory", "__table_inventory__",
		"inventory_truncated")
	traceDBAddRawMarkerReplacementClosure(&quality, items)
	return quality
}

func traceDBAddRawMarkerReplacementClosure(quality *TraceDBCoverage, items []TraceDBCoverage) {
	if quality == nil {
		return
	}
	var raw *TraceDBCoverage
	for i := range items {
		if items[i].Family == "source_rawtrace_marker_sync" &&
			items[i].Table == "__raw_marker_sync__" {
			raw = &items[i]
			break
		}
	}
	if raw == nil {
		quality.Metadata["raw_marker_replacement_closure"] = "not_evaluated_raw_coverage_absent"
		return
	}
	replacementCandidates :=
		raw.Metrics["raw_pairs_existing_db_candidate_locally_suppressed"] +
			raw.Metrics["raw_pairs_interval_collision_locally_suppressed"] +
			raw.Metrics["raw_pairs_cpu_unavailable_callstack_replaced"]
	localFenceReplacements :=
		raw.Metrics["raw_pairs_existing_db_candidate_locally_suppressed"] +
			raw.Metrics["raw_pairs_interval_collision_locally_suppressed"]
	cpuUnavailableReplacements :=
		raw.Metrics["raw_pairs_cpu_unavailable_callstack_replaced"]
	submitted := raw.Metrics["sync_spans_submitted"]
	emittedEndpoints := raw.Metrics["sync_endpoints_emitted"]
	suppressed := raw.Metrics["sync_spans_suppressed"]
	traceDBAddCoverageMetric(quality, "raw_marker_db_suppressed_candidate_spans",
		replacementCandidates)
	traceDBAddCoverageMetric(quality,
		"raw_marker_cpu_unavailable_replacement_spans",
		cpuUnavailableReplacements)
	traceDBAddCoverageMetric(quality, "raw_marker_sync_spans_submitted", submitted)
	traceDBAddCoverageMetric(quality, "raw_marker_sync_endpoints_emitted", emittedEndpoints)
	traceDBAddCoverageMetric(quality, "raw_marker_sync_spans_suppressed", suppressed)

	switch {
	case replacementCandidates == 0:
		quality.Metadata["raw_marker_replacement_closure"] = "complete_no_replacement_candidate"
		return
	case submitted < replacementCandidates:
		quality.Metadata["raw_marker_replacement_closure"] = "not_evaluated_candidate_submission_mismatch"
		return
	case suppressed != 0:
		quality.Metadata["raw_marker_replacement_closure"] = "not_evaluated_raw_span_suppressed"
		return
	case submitted > math.MaxInt64/2 || emittedEndpoints != submitted*2:
		quality.Metadata["raw_marker_replacement_closure"] = "not_evaluated_raw_endpoint_census_mismatch"
		return
	}
	locallyFenced := quality.Metrics["callstack_sync_spans_suppressed_by_local_fence"]
	totalSuppressed := quality.Metrics["callstack_sync_spans_suppressed"]
	if locallyFenced < localFenceReplacements ||
		totalSuppressed < localFenceReplacements+cpuUnavailableReplacements {
		quality.Metadata["raw_marker_replacement_closure"] = "not_evaluated_replacement_exceeds_local_fence"
		return
	}
	quality.Metadata["raw_marker_replacement_closure"] = "complete"
	traceDBAddCoverageMetric(quality,
		"callstack_sync_spans_recovered_by_raw_marker", replacementCandidates)
	traceDBAddCoverageMetric(quality,
		"callstack_sync_spans_unrecovered_after_raw_marker",
		totalSuppressed-replacementCandidates)
	traceDBAddCoverageMetric(quality,
		"callstack_local_fence_spans_unrecovered_after_raw_marker",
		locallyFenced-localFenceReplacements)
	cpuUnavailableBefore :=
		quality.Metrics["callstack_source_rows_preserved_cpu_unavailable"]
	if cpuUnavailableBefore >= cpuUnavailableReplacements {
		traceDBAddCoverageMetric(quality,
			"callstack_source_rows_cpu_unavailable_after_raw_replacement",
			cpuUnavailableBefore-cpuUnavailableReplacements)
	} else {
		quality.Metadata["raw_marker_replacement_closure"] =
			"not_evaluated_cpu_replacement_exceeds_unavailable_source"
	}
}

func traceDBSemanticQualityCaveats(coverage []TraceDBCoverage) []string {
	var quality *TraceDBCoverage
	for i := range coverage {
		if coverage[i].Family == traceDBSemanticQualityFamily && coverage[i].Table == traceDBSemanticQualityTable {
			quality = &coverage[i]
			break
		}
	}
	if quality == nil {
		return nil
	}
	degradedKeys := []string{
		"unresolved_thread_names",
		"scheduler_boundaries_with_unknown_comm",
		"callstack_source_rows_suppressed_pre_pairing",
		"callstack_source_rows_unfinished_duration_sentinel",
		"callstack_async_source_rows_suppressed_post_pairing",
		"callstack_source_rows_withheld_official_async_interval",
		"callstack_source_rows_suppressed_cpu_unavailable",
		"callstack_source_rows_suppressed_identity",
	}
	if quality.Metadata["raw_marker_replacement_closure"] == "complete" {
		degradedKeys = append(degradedKeys,
			"callstack_sync_spans_unrecovered_after_raw_marker")
	} else {
		degradedKeys = append(degradedKeys, "callstack_sync_spans_suppressed")
	}
	identityKeys := []string{
		"public_tids_with_multiple_itids",
		"public_tids_with_multiple_owner_ipids",
	}
	var caveats []string
	if summary := traceDBSemanticQualityMetricSummary(quality.Metrics, degradedKeys); summary != "" {
		caveats = append(caveats, "trace_streamer semantic quality is degraded: "+summary+
			"; the systrace is query-ready but name/span completeness is not proven")
	}
	if quality.Metadata["raw_marker_replacement_closure"] == "complete" {
		recovered := quality.Metrics["callstack_sync_spans_recovered_by_raw_marker"]
		cpuRecovered :=
			quality.Metrics["raw_marker_cpu_unavailable_replacement_spans"]
		residual := quality.Metrics["callstack_sync_spans_unrecovered_after_raw_marker"]
		localResidual :=
			quality.Metrics["callstack_local_fence_spans_unrecovered_after_raw_marker"]
		caveats = append(caveats, fmt.Sprintf(
			"trace_streamer raw marker recovery published %d exact replacement span(s), including %d unique CPU-unavailable callstack span(s) replaced by authoritative raw B/E envelopes; %d callstack sync span(s) remain unpublished after replacement closure, including %d locally fenced span(s) without a published raw replacement",
			recovered, cpuRecovered, residual, localResidual))
	}
	if summary := traceDBSemanticQualityMetricSummary(quality.Metrics, identityKeys); summary != "" {
		caveats = append(caveats, "trace_streamer identity audit observed: "+summary+
			"; multiple internal identities may be lifecycle generations or host/namespace PID splits and require retained-DB review")
	}
	cpuUnavailableKey := "callstack_source_rows_preserved_cpu_unavailable"
	if quality.Metadata["raw_marker_replacement_closure"] == "complete" {
		cpuUnavailableKey =
			"callstack_source_rows_cpu_unavailable_after_raw_replacement"
	}
	if count := quality.Metrics[cpuUnavailableKey]; count > 0 {
		caveats = append(caveats, fmt.Sprintf(
			"trace_streamer callstack CPU placement is unavailable for %d source row(s); span identity and duration were preserved in a typed trace span/interval lane, but those spans have no CPU/core attribution",
			count))
	}
	if count := quality.Metrics["callstack_source_rows_emitted_official_async_interval"]; count > 0 {
		caveats = append(caveats, fmt.Sprintf(
			"trace_streamer preserved %d completed async interval(s) as Codrax versioned comment rows because a physical finish emitter/CPU was not proven; Codrax trace_query reads these intervals, but generic systrace viewers may omit them, and no synthetic S/F endpoint was emitted",
			count))
	}
	if count := quality.Metrics["unclassified_nonempty_tables"]; count > 0 {
		detail := ""
		for _, item := range coverage {
			if item.Family == "conversion_inventory" && item.Table == "__table_inventory__" {
				detail = strings.TrimSpace(item.Skipped)
				break
			}
		}
		caveat := fmt.Sprintf(
			"trace_streamer DB contains %d nonempty table(s) with no Codrax exporter/resolver classification; their rows were not converted",
			count)
		if detail != "" {
			caveat += ": " + detail
		}
		caveats = append(caveats, caveat)
	}
	if quality.Metrics["table_inventory_truncated"] > 0 ||
		quality.Metrics["unclassified_uninspectable_tables"] > 0 {
		caveats = append(caveats,
			"trace_streamer table inventory is incomplete; retained-DB review is required before claiming conversion-family completeness")
	}
	return caveats
}

func traceDBSemanticQualityMetricSummary(metrics map[string]int64, keys []string) string {
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if value := metrics[key]; value > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", key, value))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
