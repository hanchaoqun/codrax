package types

import (
	"fmt"
	"sort"
	"strings"
)

const (
	traceObservationShardAggregateWindowMaxMs = 200.0
	traceObservationShardAggregateLimit       = 6
	traceObservationShardAggregateWindowLimit = 4
)

// TraceObservationShardAggregate summarizes repeated bounded trace_query shards
// that surfaced the same root-cause candidate. It is a soft handoff projection:
// consumers may use it to write parent-window summaries, but it never becomes a
// completion hard gate.
type TraceObservationShardAggregate struct {
	Key              string   `json:"key,omitempty"`
	Subject          string   `json:"subject,omitempty"`
	Predicate        string   `json:"predicate,omitempty"`
	Object           string   `json:"object,omitempty"`
	ChainRelevance   string   `json:"chain_relevance,omitempty"`
	ShardCount       int      `json:"shard_count,omitempty"`
	Window           string   `json:"window,omitempty"`
	TotalImpactMS    float64  `json:"total_impact_ms,omitempty"`
	MaxShardImpactMS float64  `json:"max_shard_impact_ms,omitempty"`
	ExampleWindows   []string `json:"example_windows,omitempty"`
	SourceRefs       []string `json:"source_refs,omitempty"`
	SupportRefs      []string `json:"support_refs,omitempty"`
}

// TraceObservationShardStateAggregate summarizes repeated bounded
// state/window-stats rows across trace_query shards. Like
// TraceObservationShardAggregate, it is soft handoff only: it helps later
// stages explain parent-window state coverage without reopening exploration.
type TraceObservationShardStateAggregate struct {
	Key                   string   `json:"key,omitempty"`
	Dimension             string   `json:"dimension,omitempty"`
	Subject               string   `json:"subject,omitempty"`
	Object                string   `json:"object,omitempty"`
	DrilldownSource       string   `json:"drilldown_source,omitempty"`
	ChainRelevance        string   `json:"chain_relevance,omitempty"`
	ShardCount            int      `json:"shard_count,omitempty"`
	SignificantShardCount int      `json:"significant_shard_count,omitempty"`
	ChainRequired         bool     `json:"chain_required,omitempty"`
	RecursiveDrilldown    bool     `json:"recursive_drilldown,omitempty"`
	Window                string   `json:"window,omitempty"`
	TotalImpactMS         float64  `json:"total_impact_ms,omitempty"`
	MaxShardImpactMS      float64  `json:"max_shard_impact_ms,omitempty"`
	RecommendedViews      []string `json:"recommended_views,omitempty"`
	ExampleWindows        []string `json:"example_windows,omitempty"`
	SourceRefs            []string `json:"source_refs,omitempty"`
	SupportRefs           []string `json:"support_refs,omitempty"`
}

func traceObservationShardAggregates(records []TraceObservationCoverageRecord) []TraceObservationShardAggregate {
	if len(records) == 0 {
		return nil
	}
	type bucket struct {
		TraceObservationShardAggregate
		windows map[string]bool
		parent  bool
		start   float64
		end     float64
		have    bool
	}
	buckets := map[string]*bucket{}
	for _, record := range records {
		if record.Dimension != TraceObservationDimensionRootCauseRank {
			continue
		}
		window := strings.TrimSpace(record.Window)
		duration, ok := traceObservationCoverageWindowDurationMs(window)
		if !ok {
			continue
		}
		key := traceObservationShardAggregateKey(record)
		if key == "" {
			continue
		}
		b := buckets[key]
		if b == nil {
			b = &bucket{
				TraceObservationShardAggregate: TraceObservationShardAggregate{
					Key:            key,
					Subject:        strings.TrimSpace(record.Subject),
					Predicate:      strings.TrimSpace(record.Predicate),
					Object:         strings.TrimSpace(record.Object),
					ChainRelevance: strings.TrimSpace(record.ChainRelevance),
				},
				windows: map[string]bool{},
			}
			buckets[key] = b
		}
		if duration > traceObservationShardAggregateWindowMaxMs {
			b.parent = true
			continue
		}
		if duration < traceObservationRepresentativeWindowMinMs {
			continue
		}
		if b.windows[window] {
			continue
		}
		start, end, ok := traceObservationCoverageWindowEndpointsSeconds(window)
		if !ok {
			continue
		}
		b.windows[window] = true
		b.ShardCount++
		if !b.have || start < b.start {
			b.start = start
		}
		if end > b.end {
			b.end = end
		}
		b.have = true
		impact := traceObservationCoverageImpactMS(record.Value)
		b.TotalImpactMS += impact
		if impact > b.MaxShardImpactMS {
			b.MaxShardImpactMS = impact
		}
		b.ExampleWindows = appendUniqueTraceObservationString(b.ExampleWindows, window)
		if record.Source != "" {
			b.SourceRefs = appendUniqueTraceObservationString(b.SourceRefs, record.Source)
		}
		for _, ref := range record.SupportRefs {
			b.SupportRefs = appendUniqueTraceObservationString(b.SupportRefs, ref)
		}
	}
	out := make([]TraceObservationShardAggregate, 0, len(buckets))
	for _, b := range buckets {
		if b.parent || b.ShardCount < 2 {
			continue
		}
		agg := b.TraceObservationShardAggregate
		agg.Window = traceObservationCoverageFormatWindow(b.start, b.end)
		agg.ExampleWindows = traceObservationCoverageLimitStrings(agg.ExampleWindows, traceObservationShardAggregateWindowLimit)
		agg.SourceRefs = traceObservationCoverageLimitStrings(agg.SourceRefs, 3)
		agg.SupportRefs = traceObservationCoverageLimitStrings(agg.SupportRefs, 3)
		out = append(out, agg)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if traceObservationChainRelevanceRank(out[i].ChainRelevance) != traceObservationChainRelevanceRank(out[j].ChainRelevance) {
			return traceObservationChainRelevanceRank(out[i].ChainRelevance) < traceObservationChainRelevanceRank(out[j].ChainRelevance)
		}
		if out[i].TotalImpactMS != out[j].TotalImpactMS {
			return out[i].TotalImpactMS > out[j].TotalImpactMS
		}
		if out[i].ShardCount != out[j].ShardCount {
			return out[i].ShardCount > out[j].ShardCount
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > traceObservationShardAggregateLimit {
		out = out[:traceObservationShardAggregateLimit]
	}
	return out
}

func traceObservationShardStateAggregates(records []TraceObservationCoverageRecord) []TraceObservationShardStateAggregate {
	if len(records) == 0 {
		return nil
	}
	type bucket struct {
		TraceObservationShardStateAggregate
		windows map[string]bool
		parent  bool
		start   float64
		end     float64
		have    bool
	}
	buckets := map[string]*bucket{}
	for _, record := range records {
		if !traceObservationShardStateAggregateDimension(record.Dimension) {
			continue
		}
		window := strings.TrimSpace(record.Window)
		duration, ok := traceObservationCoverageWindowDurationMs(window)
		if !ok {
			continue
		}
		key := traceObservationShardStateAggregateKey(record)
		if key == "" {
			continue
		}
		b := buckets[key]
		if b == nil {
			b = &bucket{
				TraceObservationShardStateAggregate: TraceObservationShardStateAggregate{
					Key:                key,
					Dimension:          strings.TrimSpace(record.Dimension),
					Subject:            strings.TrimSpace(record.Subject),
					Object:             strings.TrimSpace(record.Object),
					DrilldownSource:    strings.TrimSpace(record.DrilldownSource),
					ChainRelevance:     strings.TrimSpace(record.ChainRelevance),
					ChainRequired:      record.ChainRequired,
					RecursiveDrilldown: record.RecursiveDrilldown,
					RecommendedViews:   append([]string(nil), record.RecommendedViews...),
				},
				windows: map[string]bool{},
			}
			buckets[key] = b
		}
		if duration > traceObservationShardAggregateWindowMaxMs {
			b.parent = true
			continue
		}
		if duration < traceObservationRepresentativeWindowMinMs {
			continue
		}
		if b.windows[window] {
			continue
		}
		start, end, ok := traceObservationCoverageWindowEndpointsSeconds(window)
		if !ok {
			continue
		}
		b.windows[window] = true
		b.ShardCount++
		if record.Significant {
			b.SignificantShardCount++
		}
		if !b.have || start < b.start {
			b.start = start
		}
		if end > b.end {
			b.end = end
		}
		b.have = true
		impact := traceObservationCoverageImpactMS(record.Value)
		b.TotalImpactMS += impact
		if impact > b.MaxShardImpactMS {
			b.MaxShardImpactMS = impact
		}
		b.ExampleWindows = appendUniqueTraceObservationString(b.ExampleWindows, window)
		if record.Source != "" {
			b.SourceRefs = appendUniqueTraceObservationString(b.SourceRefs, record.Source)
		}
		for _, ref := range record.SupportRefs {
			b.SupportRefs = appendUniqueTraceObservationString(b.SupportRefs, ref)
		}
	}
	out := make([]TraceObservationShardStateAggregate, 0, len(buckets))
	for _, b := range buckets {
		if b.parent || b.ShardCount < 2 {
			continue
		}
		agg := b.TraceObservationShardStateAggregate
		agg.Window = traceObservationCoverageFormatWindow(b.start, b.end)
		agg.ExampleWindows = traceObservationCoverageLimitStrings(agg.ExampleWindows, traceObservationShardAggregateWindowLimit)
		agg.SourceRefs = traceObservationCoverageLimitStrings(agg.SourceRefs, 3)
		agg.SupportRefs = traceObservationCoverageLimitStrings(agg.SupportRefs, 3)
		out = append(out, agg)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if traceObservationChainRelevanceRank(out[i].ChainRelevance) != traceObservationChainRelevanceRank(out[j].ChainRelevance) {
			return traceObservationChainRelevanceRank(out[i].ChainRelevance) < traceObservationChainRelevanceRank(out[j].ChainRelevance)
		}
		if traceObservationDimensionRank(out[i].Dimension) != traceObservationDimensionRank(out[j].Dimension) {
			return traceObservationDimensionRank(out[i].Dimension) < traceObservationDimensionRank(out[j].Dimension)
		}
		if out[i].SignificantShardCount != out[j].SignificantShardCount {
			return out[i].SignificantShardCount > out[j].SignificantShardCount
		}
		if out[i].TotalImpactMS != out[j].TotalImpactMS {
			return out[i].TotalImpactMS > out[j].TotalImpactMS
		}
		if out[i].ShardCount != out[j].ShardCount {
			return out[i].ShardCount > out[j].ShardCount
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > traceObservationShardAggregateLimit {
		out = out[:traceObservationShardAggregateLimit]
	}
	return out
}

func traceObservationShardAggregateKey(record TraceObservationCoverageRecord) string {
	parts := []string{
		traceObservationCanonicalKeyPart(record.Subject),
		traceObservationCanonicalKeyPart(record.Predicate),
		traceObservationCanonicalKeyPart(record.Object),
		traceObservationCanonicalKeyPart(record.ChainRelevance),
	}
	if parts[0] == "" || parts[1] == "" {
		return ""
	}
	return strings.Join(parts, "|")
}

func traceObservationShardStateAggregateDimension(dimension string) bool {
	switch strings.TrimSpace(dimension) {
	case TraceObservationDimensionStateDrilldown,
		TraceObservationDimensionStateChurn,
		TraceObservationDimensionThreadTimeline,
		TraceObservationDimensionResourcePressure:
		return true
	default:
		return false
	}
}

func traceObservationShardStateAggregateKey(record TraceObservationCoverageRecord) string {
	parts := []string{
		traceObservationCanonicalKeyPart(record.Dimension),
		traceObservationCanonicalKeyPart(record.Subject),
		traceObservationCanonicalKeyPart(record.Object),
		traceObservationCanonicalKeyPart(record.DrilldownSource),
		traceObservationCanonicalKeyPart(record.ChainRelevance),
		traceObservationCanonicalKeyPart(strings.Join(record.RecommendedViews, ",")),
		fmt.Sprintf("chain=%t", record.ChainRequired),
		fmt.Sprintf("recursive=%t", record.RecursiveDrilldown),
	}
	if parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return ""
	}
	return strings.Join(parts, "|")
}

func traceObservationCanonicalKeyPart(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func traceObservationCoverageWindowEndpointsSeconds(window string) (float64, float64, bool) {
	window = strings.TrimSpace(window)
	parts := strings.Split(window, "..")
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, startMS, ok := traceObservationCoverageParseWindowEndpoint(parts[0])
	if !ok {
		return 0, 0, false
	}
	end, endMS, ok := traceObservationCoverageParseWindowEndpoint(parts[1])
	if !ok || end <= start {
		return 0, 0, false
	}
	if startMS || endMS {
		return start / 1000, end / 1000, true
	}
	return start, end, true
}

func traceObservationCoverageImpactMS(value string) float64 {
	value = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(value), "ms"))
	if value == "" {
		return 0
	}
	var parsed float64
	if _, err := fmt.Sscanf(value, "%f", &parsed); err != nil {
		return 0
	}
	if parsed < 0 {
		return 0
	}
	return parsed
}

func traceObservationCoverageFormatWindow(start, end float64) string {
	if start < 0 || end <= start {
		return ""
	}
	return fmt.Sprintf("%.6f..%.6f", start, end)
}
