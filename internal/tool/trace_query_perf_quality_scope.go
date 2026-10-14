package tool

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A display-only envelope over the existing native cohort, not a new source
// identity, member metric, or evidence authority. No counts are apportioned to
// hotspots: the native member has only its own explicitly recorded fields.
type perfQualityScopeDisplay struct {
	Version         int                            `json:"version"`
	QuerySourcePath string                         `json:"query_source_path"`
	QueryScopeID    string                         `json:"query_scope_id"`
	PayloadRef      string                         `json:"payload_ref"`
	Scope           *tracequery.PerfCohortScope    `json:"scope"`
	Event           string                         `json:"event"`
	WeightUnit      string                         `json:"weight_unit"`
	Samples         int                            `json:"samples"`
	Weight          int64                          `json:"weight"`
	Quality         *tracequery.PerfQualitySummary `json:"quality"`
}

func traceQueryPerfQualityScopeNote(cohort tracequery.PerfCohort, ref types.ObservationSourceRef) string {
	if cohort.Scope == nil || cohort.Scope.ID == "" || cohort.Quality == nil {
		return ""
	}
	raw, err := json.Marshal(perfQualityScopeDisplay{Version: 1, QuerySourcePath: ref.Path, QueryScopeID: ref.QueryScopeID, PayloadRef: ref.PayloadRef, Scope: cohort.Scope, Event: cohort.Event, WeightUnit: cohort.WeightUnit, Samples: cohort.SampleCount, Weight: cohort.TotalPeriod, Quality: cohort.Quality})
	if err != nil {
		return ""
	}
	return string(raw)
}

type perfQualityDimension struct {
	name   string
	values []tracequery.PerfValueCount
}

func traceQueryPerfQualityDimensions(q *tracequery.PerfQualitySummary) []perfQualityDimension {
	return []perfQualityDimension{
		{"source", q.Sources}, {"input_integrity", q.InputIntegrityIssues},
		{"symbolization", q.SymbolizationStatuses}, {"sample_kind", q.SampleKinds},
		{"weight_unit", q.WeightUnits}, {"clock", q.Clocks},
		{"clock_confidence", q.ClockConfidences}, {"callchain_status", q.CallchainStatuses},
		{"frame_status", q.FrameStatuses},
	}
}

func traceQueryPerfQualityDistribution(values []tracequery.PerfValueCount) string {
	copyValues := append([]tracequery.PerfValueCount(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i].Value < copyValues[j].Value })
	raw, _ := json.Marshal(copyValues)
	return string(raw)
}

func runtimeTracePerfQualityScope(record types.ObservationRecord) (*perfQualityScopeDisplay, string) {
	raw := runtimeTraceObservationRichNoteValue(record.RichNotes, types.TraceNoteKeyPerfQualityScope)
	var scope perfQualityScopeDisplay
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if raw == "" || !traceRootCauseReportHasUniqueNestedKeys(json.RawMessage(raw)) || decoder.Decode(&scope) != nil || decoder.Decode(new(any)) != io.EOF || scope.Version != 1 || scope.Scope == nil || scope.Scope.ID == "" ||
		scope.Scope.SourcePath == "" || scope.Scope.SourceGeneration == "" || scope.Quality == nil || scope.Samples < 0 || scope.Weight < 0 {
		return nil, ""
	}
	// Query carrier and physical cohort may legitimately differ (prepared or
	// composite captures). Bind the publisher's query receipt without treating
	// their path equality as an authority or requiring model reconstruction.
	if scope.QuerySourcePath == "" || scope.QuerySourcePath != record.SourceRef.Path || scope.QueryScopeID != record.SourceRef.QueryScopeID || scope.PayloadRef != record.SourceRef.PayloadRef {
		return nil, ""
	}
	// Only identity fields form the grouping key. Source, generation, window,
	// event and unit must all match; matching names or an ID alone never join.
	key, _ := json.Marshal(struct {
		Scope       *tracequery.PerfCohortScope
		Event, Unit string
	}{scope.Scope, scope.Event, scope.WeightUnit})
	return &scope, string(key)
}

// RuntimeTracePerfQualityScopedItems shares the exact display projection with
// the finalizer's legacy metric supplement. Unscoped legacy notes stay on that
// consumer's existing compatibility path; they cannot join a scoped cohort.
func RuntimeTracePerfQualityScopedItems(records []types.ObservationRecord) []types.AnswerBlockItem {
	var scoped []types.ObservationRecord
	for _, record := range records {
		if runtimeTraceObservationRichNoteValue(record.RichNotes, types.TraceNoteKeyPerfQualityScope) != "" {
			scoped = append(scoped, record)
		}
	}
	return runtimeTraceScopedPerfQualityItems(scoped)
}

func runtimeTracePerfPopulationText(scope *perfQualityScopeDisplay) string {
	end := ")"
	if scope.Scope.Window.EndInclusive {
		end = "]"
	}
	q := scope.Quality
	parts := []string{
		fmt.Sprintf("来源=%q; 采样窗=[%.9f,%.9f%s", scope.Scope.SourcePath, scope.Scope.Window.StartTs, scope.Scope.Window.EndTs, end),
		fmt.Sprintf("event=%q; weight_unit=%q; 总体样本数=%d; 总体采样权重=%d", scope.Event, scope.WeightUnit, scope.Samples, scope.Weight),
		fmt.Sprintf("cpu_known=%d; cpu_unknown=%d; sample_cpu_scope=%s; 已验证帧边界=%d; 未知帧边界=%d", q.CPUKnownCount, q.CPUUnknownCount, perfQualitySampleCPUScope(q), q.CallchainKnownCount, q.CallchainUnknownCount),
	}
	if thread := scope.Scope.ThreadIdentity; thread != nil {
		parts = append(parts, fmt.Sprintf("TID=%d; 线程代次=%d", thread.TID, thread.Generation))
	}
	for _, dimension := range traceQueryPerfQualityDimensions(q) {
		if len(dimension.values) > 0 {
			parts = append(parts, dimension.name+"_distribution="+traceQueryPerfQualityDistribution(dimension.values))
		}
	}
	if len(q.ParserCaveats) > 0 {
		parts = append(parts, "采集告警="+traceQueryPerfQualityDistribution(q.ParserCaveats))
	}
	for _, caveat := range q.Caveats {
		parts = append(parts, "说明="+strconv.Quote(caveat))
	}
	parts = append(parts, "以上计数与分布只属于该采样总体，不是下列任一热点的质量或数量")
	return strings.Join(parts, "; ")
}

func runtimeTracePerfMemberQualityText(record types.ObservationRecord) string {
	parts := []string{"symbol=" + strconv.Quote(record.Subject)}
	for _, key := range []string{types.TraceNoteKeyDSO, types.TraceNoteKeySource, "event", "weight_unit", "symbolization_status", "samples", "sample_weight"} {
		value := runtimeTraceObservationRichNoteValue(record.RichNotes, key)
		if key == "symbolization_status" && value == "" {
			value = "unknown"
		}
		if value != "" {
			parts = append(parts, key+"="+sanitizeForBanner(value))
		}
	}
	return strings.Join(parts, "; ")
}

func runtimeTraceScopedPerfQualityItems(records []types.ObservationRecord) []types.AnswerBlockItem {
	type group struct {
		scope    *perfQualityScopeDisplay
		members  map[string]string
		conflict bool
	}
	groups := map[string]*group{}
	var legacy []types.AnswerBlockItem
	for _, record := range records {
		if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact || !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) || record.Predicate != "perf_sample_top_symbol" {
			continue
		}
		scope, key := runtimeTracePerfQualityScope(record)
		if scope == nil {
			if text := runtimeTracePerfQualityText(record); text != "" {
				// A legacy scalar has no verifiable population identity. Preserve
				// its raw caveat as unscoped, never label it as a hotspot property.
				legacy = append(legacy, types.AnswerBlockItem{Label: "采样总体（旧记录，范围未验证）", Text: text, CitationRef: -1})
			}
			continue
		}
		g := groups[key]
		if g == nil {
			g = &group{scope: scope, members: map[string]string{}}
			groups[key] = g
		}
		// Conflicting same-identity aggregates must not win by iteration order.
		if runtimeTracePerfPopulationText(g.scope) != runtimeTracePerfPopulationText(scope) {
			g.conflict = true
		}
		member := runtimeTracePerfMemberQualityText(record)
		g.members[member] = member
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out []types.AnswerBlockItem
	for i, key := range keys {
		g := groups[key]
		text := runtimeTracePerfPopulationText(g.scope)
		if g.conflict {
			text = "同来源、窗口与采样组的质量记录不一致；不选择其中一份总体数值。"
		}
		out = append(out, types.AnswerBlockItem{Label: fmt.Sprintf("采样总体 %d", i+1), Text: text, CitationRef: -1})
		members := make([]string, 0, len(g.members))
		for member := range g.members {
			members = append(members, member)
		}
		sort.Strings(members)
		for _, member := range members {
			out = append(out, types.AnswerBlockItem{Label: fmt.Sprintf("热点字段（采样组 %d）", i+1), Text: member, CitationRef: -1})
		}
	}
	return append(out, legacy...)
}
