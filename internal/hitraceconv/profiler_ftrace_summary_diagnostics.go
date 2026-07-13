package hitraceconv

// P1-a2.2-B1-b: fixed-cardinality diagnostics for structured ftrace summary
// metadata. Numeric CPU-stat totals and detail overwrite values are snapshots
// at the TracePluginResult boundary; the cross-frame ledger records ranges and
// never adds those values together.

import (
	"fmt"
	"math/bits"
	"sort"
	"strconv"
	"strings"
)

type profilerFtraceSummaryIssueKind uint8

const (
	profilerFtraceSummaryIssueCPUStatsMalformed profilerFtraceSummaryIssueKind = iota
	profilerFtraceSummaryIssueStartStatsOverflow
	profilerFtraceSummaryIssueEndStatsOverflow
	profilerFtraceSummaryIssueDetailOverwriteOverflow
	profilerFtraceSummaryIssueSymbolMalformed
	profilerFtraceSummaryIssueClockMalformed
	profilerFtraceSummaryIssueCommMalformed
	profilerFtraceSummaryIssueVersionInvalid
	profilerFtraceSummaryIssueKindCount
)

func (kind profilerFtraceSummaryIssueKind) label() string {
	switch kind {
	case profilerFtraceSummaryIssueCPUStatsMalformed:
		return "ftrace_cpu_stats_malformed_wire"
	case profilerFtraceSummaryIssueStartStatsOverflow:
		return "ftrace_cpu_stats_start_aggregate_overflow"
	case profilerFtraceSummaryIssueEndStatsOverflow:
		return "ftrace_cpu_stats_end_aggregate_overflow"
	case profilerFtraceSummaryIssueDetailOverwriteOverflow:
		return "ftrace_cpu_detail_overwrite_aggregate_overflow"
	case profilerFtraceSummaryIssueSymbolMalformed:
		return "symbols_detail_malformed_wire"
	case profilerFtraceSummaryIssueClockMalformed:
		return "clocks_detail_malformed_wire"
	case profilerFtraceSummaryIssueCommMalformed:
		return "comm_dict_malformed_or_ambiguous"
	case profilerFtraceSummaryIssueVersionInvalid:
		return "trace_plugin_version_invalid"
	default:
		return "ftrace_summary_issue_invalid"
	}
}

type profilerFtraceSummaryIssueCensus struct {
	Occurrences    [profilerFtraceSummaryIssueKindCount]uint64
	AffectedFrames [profilerFtraceSummaryIssueKindCount]uint64
}

func (census *profilerFtraceSummaryIssueCensus) observe(kind profilerFtraceSummaryIssueKind, delta uint64) bool {
	if census == nil || kind >= profilerFtraceSummaryIssueKindCount || delta == 0 {
		return census != nil && kind < profilerFtraceSummaryIssueKindCount
	}
	index := int(kind)
	if census.Occurrences[index] == 0 && !checkedProfilerUint64AddTo(&census.AffectedFrames[index], 1) {
		return false
	}
	return checkedProfilerUint64AddTo(&census.Occurrences[index], delta)
}

func (census *profilerFtraceSummaryIssueCensus) merge(frame profilerFtraceSummaryIssueCensus) bool {
	if census == nil {
		return false
	}
	for index := range census.Occurrences {
		if !checkedProfilerUint64AddTo(&census.Occurrences[index], frame.Occurrences[index]) ||
			!checkedProfilerUint64AddTo(&census.AffectedFrames[index], frame.AffectedFrames[index]) {
			return false
		}
	}
	return true
}

func (census profilerFtraceSummaryIssueCensus) empty() bool {
	for _, count := range census.Occurrences {
		if count > 0 {
			return false
		}
	}
	return true
}

func (census profilerFtraceSummaryIssueCensus) has(kind profilerFtraceSummaryIssueKind) bool {
	return kind < profilerFtraceSummaryIssueKindCount && census.Occurrences[int(kind)] > 0
}

func (census profilerFtraceSummaryIssueCensus) totalOccurrences() (uint64, bool) {
	var total uint64
	for _, count := range census.Occurrences {
		if !checkedProfilerUint64AddTo(&total, count) {
			return 0, false
		}
	}
	return total, true
}

func (census profilerFtraceSummaryIssueCensus) summary() string {
	parts := make([]string, 0, profilerFtraceSummaryIssueKindCount)
	for kind := profilerFtraceSummaryIssueKind(0); kind < profilerFtraceSummaryIssueKindCount; kind++ {
		if count := census.Occurrences[int(kind)]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", kind.label(), count))
		}
	}
	return strings.Join(parts, ",")
}

func (census profilerFtraceSummaryIssueCensus) appendFieldSources(fields map[string]string) {
	for kind := profilerFtraceSummaryIssueKind(0); kind < profilerFtraceSummaryIssueKindCount; kind++ {
		index := int(kind)
		if census.Occurrences[index] == 0 {
			continue
		}
		prefix := "issue_" + kind.label()
		fields[prefix+"_occurrences"] = strconv.FormatUint(census.Occurrences[index], 10)
		fields[prefix+"_affected_frames"] = strconv.FormatUint(census.AffectedFrames[index], 10)
	}
}

type profilerSummaryUint64Range struct {
	Observed uint64
	Min      uint64
	Max      uint64
}

func (snapshot *profilerSummaryUint64Range) observe(value uint64) bool {
	if snapshot == nil {
		return false
	}
	if snapshot.Observed == 0 {
		snapshot.Min, snapshot.Max = value, value
	}
	if !checkedProfilerUint64AddTo(&snapshot.Observed, 1) {
		return false
	}
	if value < snapshot.Min {
		snapshot.Min = value
	}
	if value > snapshot.Max {
		snapshot.Max = value
	}
	return true
}

type profilerFtraceCPUTotalsRange struct {
	Observed uint64
	Min      profilerFtraceCPUTotals
	Max      profilerFtraceCPUTotals
}

func (snapshot *profilerFtraceCPUTotalsRange) observe(value profilerFtraceCPUTotals) bool {
	if snapshot == nil {
		return false
	}
	if snapshot.Observed == 0 {
		snapshot.Min, snapshot.Max = value, value
	}
	if !checkedProfilerUint64AddTo(&snapshot.Observed, 1) {
		return false
	}
	profilerFtraceTotalsMin(&snapshot.Min, value)
	profilerFtraceTotalsMax(&snapshot.Max, value)
	return true
}

func profilerFtraceTotalsMin(current *profilerFtraceCPUTotals, value profilerFtraceCPUTotals) {
	current.Entries = min(current.Entries, value.Entries)
	current.Overrun = min(current.Overrun, value.Overrun)
	current.CommitOverrun = min(current.CommitOverrun, value.CommitOverrun)
	current.Bytes = min(current.Bytes, value.Bytes)
	current.DroppedEvents = min(current.DroppedEvents, value.DroppedEvents)
	current.ReadEvents = min(current.ReadEvents, value.ReadEvents)
}

func profilerFtraceTotalsMax(current *profilerFtraceCPUTotals, value profilerFtraceCPUTotals) {
	current.Entries = max(current.Entries, value.Entries)
	current.Overrun = max(current.Overrun, value.Overrun)
	current.CommitOverrun = max(current.CommitOverrun, value.CommitOverrun)
	current.Bytes = max(current.Bytes, value.Bytes)
	current.DroppedEvents = max(current.DroppedEvents, value.DroppedEvents)
	current.ReadEvents = max(current.ReadEvents, value.ReadEvents)
}

const profilerSummaryCPUWordCount = (maxTraceDBCPUIndex + 64) / 64

type profilerSummaryCPUSet [profilerSummaryCPUWordCount]uint64

func (set *profilerSummaryCPUSet) observe(cpu uint64) bool {
	if set == nil || cpu > uint64(maxTraceDBCPUIndex) {
		return false
	}
	set[cpu/64] |= uint64(1) << (cpu % 64)
	return true
}

func (set profilerSummaryCPUSet) count() int {
	total := 0
	for _, word := range set {
		total += bits.OnesCount64(word)
	}
	return total
}

type profilerFtraceSummaryDiagnosticLedger struct {
	Frames              uint64
	FirstOffset         int64
	LastOffset          int64
	DegradedFrames      uint64
	FirstDegradedOffset int64
	LastDegradedOffset  int64
	Issues              profilerFtraceSummaryIssueCensus

	VersionObservations uint64
	VersionSamples      profilerStableSampleSet
	StatsMessages       uint64
	StartStats          uint64
	EndStats            uint64
	TraceClockObserved  uint64
	TraceClockSamples   profilerStableSampleSet
	StatsCPUs           profilerSummaryCPUSet
	StartTotals         profilerFtraceCPUTotalsRange
	EndTotals           profilerFtraceCPUTotalsRange

	DetailMessages           uint64
	DetailEventCount         uint64
	DetailCPUs               profilerSummaryCPUSet
	DetailOverwrite          profilerSummaryUint64Range
	DetailOverwriteUnusable  uint64
	KnownEventCounts         [len(profilerFtraceEventDescriptorList)]uint64
	UnknownEventCount        uint64
	UnknownEventFieldSamples profilerStableSampleSet

	SymbolCount           uint64
	SymbolSamples         profilerStableSampleSet
	SymbolTruncatedFrames uint64
	ClockDetailCount      uint64
	ClockDetailSamples    profilerStableSampleSet
	ClockTruncatedFrames  uint64
}

func (ledger *profilerFtraceSummaryDiagnosticLedger) observe(summary profilerFtraceSummary, offset int64) bool {
	if ledger == nil || !summary.recognizedMessage || summary.IssueOverflow {
		return false
	}
	if ledger.Frames == 0 {
		ledger.FirstOffset = offset
	}
	if !checkedProfilerUint64AddTo(&ledger.Frames, 1) {
		return false
	}
	ledger.LastOffset = offset
	if !summary.Issues.empty() {
		if ledger.DegradedFrames == 0 {
			ledger.FirstDegradedOffset = offset
		}
		if !checkedProfilerUint64AddTo(&ledger.DegradedFrames, 1) || !ledger.Issues.merge(summary.Issues) {
			return false
		}
		ledger.LastDegradedOffset = offset
	}
	if summary.Version != "" {
		if !checkedProfilerUint64AddTo(&ledger.VersionObservations, 1) {
			return false
		}
		ledger.VersionSamples.observe("profiler-ftrace-summary-version", []byte(summary.Version))
	}
	if !profilerSummaryAddInt(&ledger.StatsMessages, summary.StatsMessages) ||
		!profilerSummaryAddInt(&ledger.StartStats, summary.StartStats) ||
		!profilerSummaryAddInt(&ledger.EndStats, summary.EndStats) ||
		!profilerSummaryAddInt(&ledger.DetailMessages, summary.DetailMessages) ||
		!profilerSummaryAddInt(&ledger.DetailEventCount, summary.DetailEventCount) ||
		!profilerSummaryAddInt(&ledger.SymbolCount, summary.SymbolCount) ||
		!profilerSummaryAddInt(&ledger.ClockDetailCount, summary.ClockDetailCount) {
		return false
	}
	for clock, count := range summary.TraceClocks {
		if !profilerSummaryAddInt(&ledger.TraceClockObserved, count) {
			return false
		}
		ledger.TraceClockSamples.observe("profiler-ftrace-summary-trace-clock", []byte(clock))
	}
	for cpu := range summary.StatsCPUs {
		if !ledger.StatsCPUs.observe(cpu) {
			return false
		}
	}
	if summary.StartTotalsSeen && summary.StartTotalsValid && !ledger.StartTotals.observe(summary.StartTotals) {
		return false
	}
	if summary.EndTotalsSeen && summary.EndTotalsValid && !ledger.EndTotals.observe(summary.EndTotals) {
		return false
	}
	for cpu := range summary.DetailCPUs {
		if !ledger.DetailCPUs.observe(cpu) {
			return false
		}
	}
	if summary.DetailMessages > 0 {
		if summary.DetailOverwriteOK {
			if !ledger.DetailOverwrite.observe(summary.DetailOverwrite) {
				return false
			}
		} else if !checkedProfilerUint64AddTo(&ledger.DetailOverwriteUnusable, 1) {
			return false
		}
	}
	for field, count := range summary.EventFieldCounts {
		if count < 0 {
			return false
		}
		if slot, ok := profilerFtraceEventDescriptorSlot(field); ok {
			if !profilerSummaryAddInt(&ledger.KnownEventCounts[slot], count) {
				return false
			}
			continue
		}
		if !profilerSummaryAddInt(&ledger.UnknownEventCount, count) {
			return false
		}
		ledger.UnknownEventFieldSamples.observe("profiler-ftrace-summary-unknown-event-field", []byte(strconv.Itoa(field)))
	}
	for _, sample := range summary.SymbolExamples {
		ledger.SymbolSamples.observe("profiler-ftrace-summary-symbol", []byte(sample))
	}
	if summary.SymbolTruncated && !checkedProfilerUint64AddTo(&ledger.SymbolTruncatedFrames, 1) {
		return false
	}
	for _, sample := range summary.ClockDetails {
		ledger.ClockDetailSamples.observe("profiler-ftrace-summary-clock-detail", []byte(sample))
	}
	if summary.ClockTruncated && !checkedProfilerUint64AddTo(&ledger.ClockTruncatedFrames, 1) {
		return false
	}
	return true
}

func profilerSummaryAddInt(target *uint64, value int) bool {
	return value >= 0 && checkedProfilerUint64AddTo(target, uint64(value))
}

func profilerFtraceEventDescriptorSlot(field int) (int, bool) {
	for index, descriptor := range profilerFtraceEventDescriptorList {
		if descriptor.Field == field {
			return index, true
		}
	}
	return 0, false
}

func (ledger *profilerFtraceSummaryDiagnosticLedger) materialize(out *profilerContainerExtraction) bool {
	if ledger == nil || out == nil {
		return false
	}
	if ledger.Frames == 0 {
		return true
	}
	caveat, ok := ledger.caveat()
	if !ok {
		return false
	}
	out.Caveats = append(out.Caveats, caveat)
	if ledger.Issues.empty() {
		return true
	}
	total, ok := ledger.Issues.totalOccurrences()
	if !ok {
		return false
	}
	rowsRead, ok := profilerContainerCountToInt(total)
	if !ok {
		return false
	}
	fields := map[string]string{
		"schema_profile":             "TracePluginResult CPU stats/detail, symbols, clocks, comm dictionary, and version metadata",
		"aggregation_policy":         "fixed_summary_issue_census",
		"snapshot_numeric_policy":    "never_sum_across_trace_plugin_results",
		"sample_policy":              "sha256_min_k8_domain_separated_prefix96_bounded_examples",
		"structured_metadata_frames": strconv.FormatUint(ledger.Frames, 10),
		"degraded_frames":            strconv.FormatUint(ledger.DegradedFrames, 10),
		"first_offset":               strconv.FormatInt(ledger.FirstOffset, 10),
		"last_offset":                strconv.FormatInt(ledger.LastOffset, 10),
		"first_degraded_offset":      strconv.FormatInt(ledger.FirstDegradedOffset, 10),
		"last_degraded_offset":       strconv.FormatInt(ledger.LastDegradedOffset, 10),
	}
	ledger.Issues.appendFieldSources(fields)
	out.TraceCoverage = append(out.TraceCoverage, TraceDBCoverage{
		Family: "builtin_modern_ftrace:trace_plugin_metadata", Table: "__trace_plugin_metadata__",
		Role: "unsupported_input", Found: true, RowsRead: rowsRead,
		Skipped: ledger.Issues.summary(), FieldSources: fields,
	})
	return true
}

func (ledger profilerFtraceSummaryDiagnosticLedger) caveat() (string, bool) {
	parts := []string{
		fmt.Sprintf("summary_frames=%d", ledger.Frames),
		"metadata_completeness=recognized_members_only",
	}
	compacted := false
	parts, compacted = profilerAppendSummarySamples(parts, "version", ledger.VersionSamples, compacted)
	if ledger.VersionObservations > 1 {
		parts = append(parts, fmt.Sprintf("version_observations=%d", ledger.VersionObservations))
	}
	parts = append(parts, fmt.Sprintf("stats_messages=%d", ledger.StatsMessages))
	if ledger.StartStats > 0 || ledger.EndStats > 0 {
		parts = append(parts, fmt.Sprintf("stats_start=%d", ledger.StartStats), fmt.Sprintf("stats_end=%d", ledger.EndStats))
	}
	parts, compacted = profilerAppendSummarySamples(parts, "trace_clock", ledger.TraceClockSamples, compacted)
	if ledger.TraceClockObserved > 1 {
		parts = append(parts, fmt.Sprintf("trace_clock_observations=%d", ledger.TraceClockObserved))
	}
	if cpuCount := ledger.StatsCPUs.count(); cpuCount > 0 {
		parts = append(parts, fmt.Sprintf("stats_cpus=%d", cpuCount), "stats_cpus_scope=distinct_observed_cpu_ids")
	}
	if ledger.Frames == 1 {
		if ledger.EndStats > 0 && ledger.EndTotals.Observed == 1 {
			parts = profilerAppendExactTotals(parts, "end", ledger.EndTotals.Min)
		} else if ledger.StartTotals.Observed == 1 {
			parts = profilerAppendExactTotals(parts, "observed", ledger.StartTotals.Min)
		}
	} else {
		parts = profilerAppendTotalsRange(parts, "start", ledger.StartTotals)
		parts = profilerAppendTotalsRange(parts, "end", ledger.EndTotals)
	}
	if ledger.StartTotals.Observed > 0 || ledger.EndTotals.Observed > 0 {
		parts = append(parts, "stats_snapshot_values_not_summed=true")
	}
	if ledger.DetailMessages > 0 {
		parts = append(parts,
			fmt.Sprintf("detail_messages=%d", ledger.DetailMessages),
			fmt.Sprintf("detail_cpus=%d", ledger.DetailCPUs.count()),
			fmt.Sprintf("structured_event_records=%d", ledger.DetailEventCount))
		if ledger.Frames == 1 && ledger.DetailOverwrite.Observed == 1 {
			parts = append(parts, fmt.Sprintf("detail_overwrite=%d", ledger.DetailOverwrite.Min))
		} else if ledger.DetailOverwrite.Observed > 0 {
			parts = append(parts,
				fmt.Sprintf("detail_overwrite_snapshot_count=%d", ledger.DetailOverwrite.Observed),
				fmt.Sprintf("detail_overwrite_snapshot_min=%d", ledger.DetailOverwrite.Min),
				fmt.Sprintf("detail_overwrite_snapshot_max=%d", ledger.DetailOverwrite.Max),
				"detail_overwrite_snapshot_values_not_summed=true")
		}
		if ledger.DetailOverwriteUnusable > 0 {
			parts = append(parts, fmt.Sprintf("detail_overwrite_unusable_frames=%d", ledger.DetailOverwriteUnusable))
		}
	}
	familyCounts, nameCounts, ok := ledger.eventCounts()
	if !ok {
		return "", false
	}
	if len(familyCounts) > 0 {
		parts = append(parts, "event_families="+joinProfilerSummaryUint64Counts(familyCounts), "event_names="+joinProfilerSummaryUint64Counts(nameCounts))
	}
	if ledger.UnknownEventCount > 0 {
		parts = append(parts, fmt.Sprintf("unknown_event_records=%d", ledger.UnknownEventCount))
		parts, compacted = profilerAppendSummarySamples(parts, "unknown_event_field", ledger.UnknownEventFieldSamples, compacted)
	}
	if ledger.SymbolCount > 0 {
		parts = append(parts, fmt.Sprintf("symbols=%d", ledger.SymbolCount))
		parts, compacted = profilerAppendSummarySamples(parts, "symbol_examples", ledger.SymbolSamples, compacted)
		if ledger.SymbolTruncatedFrames > 0 {
			parts = append(parts, fmt.Sprintf("symbol_example_truncated_frames=%d", ledger.SymbolTruncatedFrames))
		}
	}
	if ledger.ClockDetailCount > 0 {
		parts = append(parts, fmt.Sprintf("clock_detail_records=%d", ledger.ClockDetailCount))
		parts, compacted = profilerAppendSummarySamples(parts, "clock_details", ledger.ClockDetailSamples, compacted)
		if ledger.ClockTruncatedFrames > 0 {
			parts = append(parts, fmt.Sprintf("clock_detail_truncated_frames=%d", ledger.ClockTruncatedFrames))
		}
	}
	if compacted {
		parts = append(parts, "sample_policy=sha256_min_k8_prefix96_bounded_examples_not_complete_inventory")
	}
	if issueSummary := ledger.Issues.summary(); issueSummary != "" {
		parts = append(parts, "degraded="+issueSummary)
	}
	return "ftrace-plugin structured metadata: " + strings.Join(parts, "; "), true
}

func profilerAppendSummarySamples(parts []string, label string, samples profilerStableSampleSet, compacted bool) ([]string, bool) {
	if samples.Used == 0 {
		return parts, compacted
	}
	if samples.Used == 1 {
		item := samples.Items[0]
		if item.InputLen == uint64(item.PrefixLen) {
			value := strings.ToValidUTF8(string(item.Prefix[:item.PrefixLen]), "�")
			return append(parts, label+"="+value), compacted
		}
	}
	return append(parts, label+"_samples="+samples.render()), true
}

func profilerAppendExactTotals(parts []string, label string, totals profilerFtraceCPUTotals) []string {
	return append(parts,
		fmt.Sprintf("%s_entries=%d", label, totals.Entries),
		fmt.Sprintf("%s_dropped=%d", label, totals.DroppedEvents),
		fmt.Sprintf("%s_overrun=%d", label, totals.Overrun),
		fmt.Sprintf("%s_commit_overrun=%d", label, totals.CommitOverrun),
		fmt.Sprintf("%s_read=%d", label, totals.ReadEvents),
		fmt.Sprintf("%s_bytes=%d", label, totals.Bytes))
}

func profilerAppendTotalsRange(parts []string, label string, totals profilerFtraceCPUTotalsRange) []string {
	if totals.Observed == 0 {
		return parts
	}
	parts = append(parts, fmt.Sprintf("%s_snapshot_count=%d", label, totals.Observed))
	parts = profilerAppendMetricRange(parts, label+"_entries", totals.Min.Entries, totals.Max.Entries)
	parts = profilerAppendMetricRange(parts, label+"_dropped", totals.Min.DroppedEvents, totals.Max.DroppedEvents)
	parts = profilerAppendMetricRange(parts, label+"_overrun", totals.Min.Overrun, totals.Max.Overrun)
	parts = profilerAppendMetricRange(parts, label+"_commit_overrun", totals.Min.CommitOverrun, totals.Max.CommitOverrun)
	parts = profilerAppendMetricRange(parts, label+"_read", totals.Min.ReadEvents, totals.Max.ReadEvents)
	return profilerAppendMetricRange(parts, label+"_bytes", totals.Min.Bytes, totals.Max.Bytes)
}

func profilerAppendMetricRange(parts []string, label string, minValue, maxValue uint64) []string {
	return append(parts, fmt.Sprintf("%s_snapshot_min=%d", label, minValue), fmt.Sprintf("%s_snapshot_max=%d", label, maxValue))
}

func (ledger profilerFtraceSummaryDiagnosticLedger) eventCounts() (map[string]uint64, map[string]uint64, bool) {
	families := make(map[string]uint64)
	names := make(map[string]uint64)
	for index, count := range ledger.KnownEventCounts {
		if count == 0 {
			continue
		}
		descriptor := profilerFtraceEventDescriptorList[index]
		familyCount := families[descriptor.Family]
		nameCount := names[descriptor.Name]
		if !checkedProfilerUint64AddTo(&familyCount, count) || !checkedProfilerUint64AddTo(&nameCount, count) {
			return nil, nil, false
		}
		families[descriptor.Family] = familyCount
		names[descriptor.Name] = nameCount
	}
	return families, names, true
}

func joinProfilerSummaryUint64Counts(values map[string]uint64) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if values[key] == 1 {
			parts = append(parts, key)
		} else {
			parts = append(parts, fmt.Sprintf("%s:%d", key, values[key]))
		}
	}
	return strings.Join(parts, ",")
}
