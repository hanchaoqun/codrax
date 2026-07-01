package types

import (
	"fmt"
	"sort"
	"strings"
)

const (
	TraceObservationDimensionFrameTarget      = "frame_target_resolution"
	TraceObservationDimensionRootCauseRank    = "root_cause_rank"
	TraceObservationDimensionSemanticSpan     = "trace_semantic_span"
	TraceObservationDimensionWakeupChain      = "wakeup_chain"
	TraceObservationDimensionCriticalBlocking = "critical_blocking"
	TraceObservationDimensionStateChurn       = "state_churn"
	TraceObservationDimensionStateDrilldown   = "state_drilldown"
	TraceObservationDimensionThreadTimeline   = "thread_timeline"
	TraceObservationDimensionResourcePressure = "resource_pressure"
	TraceObservationDimensionEvidencePack     = "evidence_pack"
)

const (
	traceObservationMicroWindowProbeMs           = 50.0
	traceObservationRepresentativeWindowMinMs    = 80.0
	traceObservationRepresentativeMicroWindowMin = 2
)

type TraceObservationCoverage struct {
	Active                bool                                  `json:"active,omitempty"`
	TotalRecords          int                                   `json:"total_records,omitempty"`
	QueryCount            int                                   `json:"query_count,omitempty"`
	Dimensions            []TraceObservationDimensionCoverage   `json:"dimensions,omitempty"`
	TopObservations       []TraceObservationCoverageRecord      `json:"top_observations,omitempty"`
	Windows               []string                              `json:"windows,omitempty"`
	Filters               []string                              `json:"filters,omitempty"`
	SourceRefs            []string                              `json:"source_refs,omitempty"`
	SoftMissingDimensions []string                              `json:"soft_missing_dimensions,omitempty"`
	ShardAggregates       []TraceObservationShardAggregate      `json:"shard_aggregates,omitempty"`
	ShardStateAggregates  []TraceObservationShardStateAggregate `json:"shard_state_aggregates,omitempty"`
	CausalProjection      TraceCausalProjection                 `json:"causal_projection,omitempty"`
}

type TraceObservationDimensionCoverage struct {
	Dimension       string                           `json:"dimension,omitempty"`
	Count           int                              `json:"count,omitempty"`
	PrincipalCount  int                              `json:"principal_count,omitempty"`
	OnChainCount    int                              `json:"on_chain_count,omitempty"`
	AdjacentCount   int                              `json:"adjacent_count,omitempty"`
	BackgroundCount int                              `json:"background_count,omitempty"`
	Examples        []TraceObservationCoverageRecord `json:"examples,omitempty"`
}

type TraceObservationCoverageRecord struct {
	ID                 string   `json:"id,omitempty"`
	Dimension          string   `json:"dimension,omitempty"`
	Subject            string   `json:"subject,omitempty"`
	Predicate          string   `json:"predicate,omitempty"`
	Object             string   `json:"object,omitempty"`
	Value              string   `json:"value,omitempty"`
	Unit               string   `json:"unit,omitempty"`
	Summary            string   `json:"summary,omitempty"`
	ChainRelevance     string   `json:"chain_relevance,omitempty"`
	DrilldownSource    string   `json:"drilldown_source,omitempty"`
	RecommendedViews   []string `json:"recommended_views,omitempty"`
	ChainRequired      bool     `json:"chain_required,omitempty"`
	RecursiveDrilldown bool     `json:"recursive_drilldown,omitempty"`
	Window             string   `json:"window,omitempty"`
	Filter             string   `json:"filter,omitempty"`
	Source             string   `json:"source,omitempty"`
	Span               string   `json:"span,omitempty"`
	SupportRefs        []string `json:"support_refs,omitempty"`
	Significant        bool     `json:"significant,omitempty"`
}

func BuildTraceObservationCoverage(ledger ObservationLedger) TraceObservationCoverage {
	return TraceObservationCoverageFromObservationRecords(ledger.Records)
}

func TraceObservationCoverageFromObservationRecords(records []ObservationRecord) TraceObservationCoverage {
	if len(records) == 0 {
		return TraceObservationCoverage{}
	}
	byDimension := map[string][]TraceObservationCoverageRecord{}
	queryIDs := map[string]bool{}
	var coverageRecords []TraceObservationCoverageRecord
	var windows []string
	var filters []string
	var sources []string
	for _, record := range records {
		if !traceObservationCoverageRecord(record) {
			continue
		}
		dimension := traceObservationDimension(record)
		if dimension == "" {
			continue
		}
		view := traceObservationCoverageRecordView(record, dimension)
		coverageRecords = append(coverageRecords, view)
		byDimension[dimension] = append(byDimension[dimension], view)
		if id := strings.TrimSpace(record.SourceRef.ToolCallID); id != "" {
			queryIDs[id] = true
		} else if id := traceObservationToolCallIDFromRecord(record); id != "" {
			queryIDs[id] = true
		}
		windows = appendUniqueTraceObservationString(windows, view.Window)
		filters = appendUniqueTraceObservationString(filters, view.Filter)
		sources = appendUniqueTraceObservationString(sources, view.Source)
	}
	if len(coverageRecords) == 0 {
		return TraceObservationCoverage{}
	}
	sort.SliceStable(coverageRecords, func(i, j int) bool {
		return traceObservationCoverageRecordLess(coverageRecords[i], coverageRecords[j])
	})
	dimensions := traceObservationDimensionCoverageRows(byDimension)
	out := TraceObservationCoverage{
		Active:                true,
		TotalRecords:          len(coverageRecords),
		QueryCount:            len(queryIDs),
		Dimensions:            dimensions,
		TopObservations:       traceObservationCoverageLimitRecords(coverageRecords, 8),
		Windows:               traceObservationCoverageLimitStrings(windows, 6),
		Filters:               traceObservationCoverageLimitStrings(filters, 8),
		SourceRefs:            traceObservationCoverageLimitStrings(sources, 6),
		SoftMissingDimensions: traceObservationSoftMissingDimensions(byDimension, coverageRecords),
		ShardAggregates:       traceObservationShardAggregates(coverageRecords),
		ShardStateAggregates:  traceObservationShardStateAggregates(coverageRecords),
		CausalProjection:      TraceCausalProjectionFromObservationRecords(records),
	}
	return out
}

func traceObservationCoverageRecord(record ObservationRecord) bool {
	return record.Origin == AnswerEvidenceOriginRuntimeArtifact &&
		RuntimeObservationProducerIsDeterministicQuery(record.Producer) &&
		record.GroundingPolicy == ClaimGroundingHard
}

func traceObservationDimension(record ObservationRecord) string {
	predicate := strings.TrimSpace(record.Predicate)
	claimKey := strings.TrimSpace(record.ClaimKey)
	switch {
	case predicate == "frame_target_resolution" ||
		strings.HasPrefix(claimKey, "frame_target_resolution"):
		return TraceObservationDimensionFrameTarget
	case predicate == "wakeup_chain" ||
		strings.HasPrefix(predicate, "wakeup_causal_") ||
		strings.HasPrefix(claimKey, "wakeup_chain") ||
		strings.HasPrefix(claimKey, "wakeup_causal_"):
		return TraceObservationDimensionWakeupChain
	case predicate == "trace_semantic_span" ||
		strings.HasPrefix(claimKey, "trace_semantic_span:"):
		return TraceObservationDimensionSemanticSpan
	case predicate == "critical_blocking" ||
		strings.HasPrefix(claimKey, "critical_blocking"):
		return TraceObservationDimensionCriticalBlocking
	case predicate == "state_churn" ||
		strings.HasPrefix(claimKey, "state_churn"):
		return TraceObservationDimensionStateChurn
	case predicate == "state_drilldown" ||
		strings.HasPrefix(claimKey, "state_drilldown"):
		return TraceObservationDimensionStateDrilldown
	case traceObservationRecordIsThreadTimeline(predicate, claimKey):
		return TraceObservationDimensionThreadTimeline
	case traceObservationRecordIsResourcePressure(predicate, claimKey):
		return TraceObservationDimensionResourcePressure
	case strings.HasPrefix(predicate, "root_cause_") ||
		strings.HasPrefix(claimKey, "root_cause_"):
		return TraceObservationDimensionRootCauseRank
	case strings.HasPrefix(claimKey, "evidence_fact:") ||
		strings.HasPrefix(claimKey, "root_evidence:"):
		return TraceObservationDimensionEvidencePack
	default:
		return ""
	}
}

func traceObservationRecordIsThreadTimeline(predicate, claimKey string) bool {
	switch predicate {
	case "thread_cpu_load", "runnable_context", "process_cpu_load", "cpu_constraint", "sched_stat_accounting":
		return true
	default:
		return strings.HasPrefix(claimKey, "thread_cpu_load:") ||
			strings.HasPrefix(claimKey, "runnable_context:") ||
			strings.HasPrefix(claimKey, "process_cpu_load:") ||
			strings.HasPrefix(claimKey, "cpu_constraint:") ||
			strings.HasPrefix(claimKey, "sched_stat_accounting:")
	}
}

func traceObservationRecordIsResourcePressure(predicate, claimKey string) bool {
	switch predicate {
	case "file_io_by_inode", "page_cache_by_inode", "storage_latency_by_layer",
		"io_burst_episode", "block_io_by_inode", "irq_activity", "softirq_activity",
		"ipi_activity", "workqueue_activity", "trace_mark_category", "async_file_work":
		return true
	}
	for _, prefix := range []string{
		"io_pressure:", "supply_pressure:", "file_io:", "page_cache:", "storage_latency:",
		"io_burst_episode:", "block_io_by_inode:", "bio_resource:", "filesystem_resource:",
		"page_fault_resource:", "irq_activity:", "softirq_activity:", "ipi_activity:",
		"workqueue_activity:", "trace_mark_category:", "async_file_work:",
	} {
		if strings.HasPrefix(claimKey, prefix) {
			return true
		}
	}
	return false
}

func traceObservationCoverageRecordView(record ObservationRecord, dimension string) TraceObservationCoverageRecord {
	value := strings.TrimSpace(record.Value)
	if unit := strings.TrimSpace(record.Unit); value != "" && unit != "" && !strings.HasSuffix(strings.ToLower(value), strings.ToLower(unit)) {
		value += unit
	}
	return TraceObservationCoverageRecord{
		ID:                 strings.TrimSpace(record.ID),
		Dimension:          dimension,
		Subject:            strings.TrimSpace(record.Subject),
		Predicate:          strings.TrimSpace(record.Predicate),
		Object:             strings.TrimSpace(record.Object),
		Value:              value,
		Unit:               strings.TrimSpace(record.Unit),
		Summary:            strings.TrimSpace(record.Summary),
		ChainRelevance:     traceObservationChainRelevance(record),
		DrilldownSource:    traceObservationRichNoteValue(record.RichNotes, "source"),
		RecommendedViews:   traceObservationRecommendedViews(record.RichNotes),
		ChainRequired:      traceObservationRichNoteBool(record.RichNotes, "chain_required"),
		RecursiveDrilldown: traceObservationRichNoteBool(record.RichNotes, "recursive"),
		Window:             traceObservationWindow(record),
		Filter:             traceObservationFilter(record),
		Source:             FormatObservationSourceRef(record.SourceRef, 120),
		Span:               FormatObservationSpan(record.Span, 80),
		SupportRefs:        traceObservationCoverageLimitStrings(record.SupportRefs, 3),
		Significant:        traceObservationRichNoteBool(record.RichNotes, "significant"),
	}
}

func traceObservationDimensionCoverageRows(byDimension map[string][]TraceObservationCoverageRecord) []TraceObservationDimensionCoverage {
	dimensions := make([]string, 0, len(byDimension))
	for dimension := range byDimension {
		dimensions = append(dimensions, dimension)
	}
	sort.SliceStable(dimensions, func(i, j int) bool {
		return traceObservationDimensionRank(dimensions[i]) < traceObservationDimensionRank(dimensions[j])
	})
	out := make([]TraceObservationDimensionCoverage, 0, len(dimensions))
	for _, dimension := range dimensions {
		records := append([]TraceObservationCoverageRecord(nil), byDimension[dimension]...)
		sort.SliceStable(records, func(i, j int) bool {
			return traceObservationCoverageRecordLess(records[i], records[j])
		})
		row := TraceObservationDimensionCoverage{
			Dimension: dimension,
			Count:     len(records),
			Examples:  traceObservationCoverageLimitRecords(records, 3),
		}
		for _, record := range records {
			if record.Dimension == TraceObservationDimensionRootCauseRank {
				row.PrincipalCount++
			}
			switch record.ChainRelevance {
			case "on_chain":
				row.OnChainCount++
			case "adjacent":
				row.AdjacentCount++
			case "background":
				row.BackgroundCount++
			}
		}
		out = append(out, row)
	}
	return out
}

func traceObservationSoftMissingDimensions(byDimension map[string][]TraceObservationCoverageRecord, records []TraceObservationCoverageRecord) []string {
	if len(byDimension) == 0 {
		return nil
	}
	hasRoot := len(byDimension[TraceObservationDimensionRootCauseRank]) > 0
	hasWakeup := len(byDimension[TraceObservationDimensionWakeupChain]) > 0
	hasBlocking := len(byDimension[TraceObservationDimensionCriticalBlocking]) > 0
	hasTimeline := len(byDimension[TraceObservationDimensionThreadTimeline]) > 0 ||
		len(byDimension[TraceObservationDimensionStateChurn]) > 0 ||
		len(byDimension[TraceObservationDimensionStateDrilldown]) > 0
	hasResource := len(byDimension[TraceObservationDimensionResourcePressure]) > 0
	var missing []string
	if hasRoot && !hasWakeup {
		missing = append(missing, "wakeup_chain")
	}
	if hasRoot && !hasBlocking {
		missing = append(missing, "critical_blocking_calls")
	}
	if hasRoot && !hasTimeline {
		missing = append(missing, "thread_timeline_or_window_stats")
	}
	if hasRoot && !hasResource {
		missing = append(missing, "window_stats_resource_pressure")
	}
	if traceObservationNeedsRepresentativeWindowCoverage(records) {
		missing = append(missing, "representative_window_coverage")
	}
	return missing
}

func traceObservationNeedsRepresentativeWindowCoverage(records []TraceObservationCoverageRecord) bool {
	microWindows := map[string]bool{}
	hasRepresentative := false
	for _, record := range records {
		if record.Dimension != TraceObservationDimensionRootCauseRank {
			continue
		}
		durationMs, ok := traceObservationCoverageWindowDurationMs(record.Window)
		if !ok {
			continue
		}
		switch {
		case durationMs >= traceObservationRepresentativeWindowMinMs:
			hasRepresentative = true
		case durationMs > 0 && durationMs < traceObservationMicroWindowProbeMs:
			microWindows[strings.TrimSpace(record.Window)] = true
		}
	}
	return !hasRepresentative && len(microWindows) >= traceObservationRepresentativeMicroWindowMin
}

func traceObservationCoverageWindowDurationMs(window string) (float64, bool) {
	window = strings.TrimSpace(window)
	if window == "" {
		return 0, false
	}
	parts := strings.Split(window, "..")
	if len(parts) != 2 {
		return 0, false
	}
	start, startMS, ok := traceObservationCoverageParseWindowEndpoint(parts[0])
	if !ok {
		return 0, false
	}
	end, endMS, ok := traceObservationCoverageParseWindowEndpoint(parts[1])
	if !ok || end <= start {
		return 0, false
	}
	if startMS || endMS {
		return end - start, true
	}
	return (end - start) * 1000, true
}

func traceObservationCoverageParseWindowEndpoint(raw string) (float64, bool, bool) {
	value := strings.TrimSpace(raw)
	isMS := false
	if strings.HasSuffix(value, "ms") {
		isMS = true
		value = strings.TrimSpace(strings.TrimSuffix(value, "ms"))
	}
	var parsed float64
	if _, err := fmt.Sscanf(value, "%f", &parsed); err != nil {
		return 0, isMS, false
	}
	return parsed, isMS, true
}

func traceObservationCoverageRecordLess(a, b TraceObservationCoverageRecord) bool {
	if traceObservationDimensionRank(a.Dimension) != traceObservationDimensionRank(b.Dimension) {
		return traceObservationDimensionRank(a.Dimension) < traceObservationDimensionRank(b.Dimension)
	}
	if traceObservationChainRelevanceRank(a.ChainRelevance) != traceObservationChainRelevanceRank(b.ChainRelevance) {
		return traceObservationChainRelevanceRank(a.ChainRelevance) < traceObservationChainRelevanceRank(b.ChainRelevance)
	}
	return strings.TrimSpace(a.ID) < strings.TrimSpace(b.ID)
}

func traceObservationDimensionRank(dimension string) int {
	switch strings.TrimSpace(dimension) {
	case TraceObservationDimensionFrameTarget:
		return 0
	case TraceObservationDimensionRootCauseRank:
		return 1
	case TraceObservationDimensionSemanticSpan:
		return 2
	case TraceObservationDimensionWakeupChain:
		return 3
	case TraceObservationDimensionCriticalBlocking:
		return 4
	case TraceObservationDimensionStateChurn:
		return 5
	case TraceObservationDimensionStateDrilldown:
		return 6
	case TraceObservationDimensionThreadTimeline:
		return 7
	case TraceObservationDimensionResourcePressure:
		return 8
	case TraceObservationDimensionEvidencePack:
		return 9
	default:
		return 100
	}
}

func traceObservationChainRelevanceRank(relevance string) int {
	switch strings.TrimSpace(relevance) {
	case "on_chain":
		return 0
	case "adjacent":
		return 1
	case "background":
		return 2
	default:
		return 3
	}
}

func traceObservationChainRelevance(record ObservationRecord) string {
	switch value := traceObservationRichNoteValue(record.RichNotes, "chain_relevance"); value {
	case "on_chain", "adjacent", "background":
		return value
	}
	switch value := traceObservationRichNoteValue(record.RichNotes, "causality"); value {
	case "on_wakeup_chain", "on_dependency_chain":
		return "on_chain"
	case "adjacent_to_wakeup_chain", "adjacent_to_dependency_chain":
		return "adjacent"
	case "background", "off_chain":
		return "background"
	}
	return ""
}

func traceObservationWindow(record ObservationRecord) string {
	switch {
	case record.Span.StartTs >= 0 && record.Span.EndTs > record.Span.StartTs:
		return fmt.Sprintf("%.6f..%.6f", record.Span.StartTs, record.Span.EndTs)
	case record.Span.StartTsMs >= 0 && record.Span.EndTsMs > record.Span.StartTsMs:
		return fmt.Sprintf("%.3fms..%.3fms", record.Span.StartTsMs, record.Span.EndTsMs)
	}
	for _, key := range []string{"actual_window", "nearest_chain_window", "occurrence_windows"} {
		if value := traceObservationRichNoteValue(record.RichNotes, key); value != "" {
			if before, _, ok := strings.Cut(value, ";"); ok {
				value = before
			}
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func traceObservationFilter(record ObservationRecord) string {
	var parts []string
	if subject := strings.TrimSpace(record.Subject); subject != "" {
		parts = append(parts, "subject="+subject)
	}
	if object := strings.TrimSpace(record.Object); object != "" {
		parts = append(parts, "object="+object)
	}
	if relevance := traceObservationChainRelevance(record); relevance != "" {
		parts = append(parts, "chain_relevance="+relevance)
	}
	if tool := strings.TrimSpace(record.SourceRef.ToolCallID); tool != "" {
		parts = append(parts, "tool_call="+tool)
	}
	return strings.Join(parts, " ")
}

func traceObservationToolCallIDFromRecord(record ObservationRecord) string {
	id := strings.TrimSpace(record.ID)
	if id == "" {
		return ""
	}
	if before, _, ok := strings.Cut(id, "#"); ok {
		return strings.TrimSpace(before)
	}
	return ""
}

func traceObservationRichNoteValue(notes []string, key string) string {
	prefix := strings.TrimSpace(key) + "="
	if prefix == "=" {
		return ""
	}
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if strings.HasPrefix(note, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(note, prefix))
		}
	}
	return ""
}

func traceObservationRichNoteBool(notes []string, key string) bool {
	switch strings.ToLower(traceObservationRichNoteValue(notes, key)) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

func traceObservationRecommendedViews(notes []string) []string {
	raw := traceObservationRichNoteValue(notes, "recommended_views")
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', '|':
			return true
		default:
			return false
		}
	})
	var out []string
	for _, field := range fields {
		out = appendUniqueTraceObservationString(out, field)
	}
	return traceObservationCoverageLimitStrings(out, 5)
}

func traceObservationCoverageLimitRecords(records []TraceObservationCoverageRecord, limit int) []TraceObservationCoverageRecord {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if len(records) > limit {
		records = records[:limit]
	}
	return append([]TraceObservationCoverageRecord(nil), records...)
}

func traceObservationCoverageLimitStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) == 0 {
		return nil
	}
	if len(values) > limit {
		values = values[:limit]
	}
	return append([]string(nil), values...)
}

func appendUniqueTraceObservationString(dst []string, value string) []string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return dst
	}
	for _, existing := range dst {
		if existing == value {
			return dst
		}
	}
	return append(dst, value)
}
