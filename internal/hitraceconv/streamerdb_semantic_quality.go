package hitraceconv

import (
	"fmt"
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
			"authority": "exact counters copied from typed DB resolver/exporter decisions after shared sync-span reconciliation; advisory only and never a conversion hard gate",
		},
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
	copyMetric("public_tids_with_multiple_itids", "resolver", "thread", "public_tids_with_multiple_itids")
	copyMetric("public_tids_with_multiple_owner_ipids", "resolver", "thread", "public_tids_with_multiple_owner_ipids")
	copyMetric("scheduler_boundaries_with_unknown_comm", "scheduler", "sched_slice", "boundaries_with_unknown_comm")
	copyMetric("callstack_source_rows_suppressed_pre_pairing", "slice", "callstack", "source_rows_suppressed_pre_pairing")
	copyMetric("callstack_async_source_rows_suppressed_post_pairing", "slice", "callstack", "async_source_rows_suppressed_post_pairing")
	copyMetric("callstack_source_rows_suppressed_cpu_unavailable", "slice", "callstack", "source_rows_suppressed_cpu_unavailable")
	copyMetric("callstack_source_rows_suppressed_identity", "slice", "callstack", "source_rows_suppressed_identity")
	copyMetric("callstack_sync_spans_suppressed", "slice", "callstack", "sync_spans_suppressed")
	return quality
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
		"callstack_async_source_rows_suppressed_post_pairing",
		"callstack_source_rows_suppressed_cpu_unavailable",
		"callstack_source_rows_suppressed_identity",
		"callstack_sync_spans_suppressed",
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
	if summary := traceDBSemanticQualityMetricSummary(quality.Metrics, identityKeys); summary != "" {
		caveats = append(caveats, "trace_streamer identity audit observed: "+summary+
			"; multiple internal identities may be lifecycle generations or host/namespace PID splits and require retained-DB review")
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
