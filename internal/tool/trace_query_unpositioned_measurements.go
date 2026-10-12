package tool

import (
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const TraceUnpositionedMeasurementsPredicate = "unpositioned_measurements_observation"

// Reuse exactly the raw cells from the existing timed measurement projection.
// Timestamp storage types, integer precision, NULL and source references are
// formatted once; clipping and any window membership stay with the caller.
func traceQueryRawMeasurementCells(row tracequery.MeasurementRow) []string {
	rec := row.Record
	name, kind, arg := "未确认", "未确认", "未确认"
	if f := rec.Filter; f != nil {
		name, kind, arg = measurementScalar(f.Name), measurementScalar(f.Type), measurementScalar(f.SourceArgSetID)
	}
	ref := fmt.Sprintf("%s / %d / %d", row.SourcePath, rec.RowID, row.SourceLine)
	return []string{ref, measurementScalar(rec.FilterID), name, measurementReferenceLabel(rec.FilterStatus), measurementScalar(rec.MeasureType), kind, arg,
		measurementScalar(rec.Value), measurementScalar(rec.StartNS), measurementScalar(rec.DurationNS)}
}

// The event-search producer has already read and decoded these records. This
// projection neither scans again nor promotes the carrier's sorting coordinate
// or query scan window to a source timestamp. Only returned originals appear.
func traceQueryUnpositionedMeasurementsObservations(result tracequery.Result, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	c := result.EventSearchCoverage
	if result.View != "event_search" || c == nil || c.Emitted != len(result.Events) || c.MatchedTotal < c.Emitted ||
		(c.EnumerationComplete && c.MatchedTotal != c.Emitted) || ref.Kind != types.ObservationSourceRuntimeArtifact ||
		ref.Path == "" || ref.Path != result.SourcePath || ref.QueryScopeID == "" || ref.PayloadRef == "" {
		return nil
	}
	var rows [][]string
	seen := map[string]bool{}
	lineStart, lineEnd := 0, 0
	for _, event := range result.Events {
		if event.Type != tracequery.EventMeasureInterval {
			continue
		}
		if event.PluginFields == nil || event.PluginFields.Measure == nil || !event.PluginFields.Measure.Valid() || !traceQueryUnpositionedSourceMapped(result, event) {
			return nil
		}
		record := *event.PluginFields.Measure
		key, _ := json.Marshal([]any{event.SourcePath, record.RowID})
		if seen[string(key)] {
			return nil
		}
		seen[string(key)] = true
		if _, known := record.StartNS.Integer(); known {
			continue // Genuine zero and negative integer source times are known.
		}
		rows = append(rows, traceQueryRawMeasurementCells(tracequery.MeasurementRow{SourcePath: event.SourcePath, SourceLine: event.LocalLine, Record: record}))
		if lineStart == 0 || event.Line < lineStart {
			lineStart = event.Line
		}
		lineEnd = max(lineEnd, event.Line)
	}
	if len(rows) == 0 {
		return nil
	}
	r := types.ObservationRecord{ID: "trace_query:" + scope + "#unpositioned_measurements", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
		ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref, Span: types.ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		ClaimKey: TraceUnpositionedMeasurementsPredicate, Predicate: TraceUnpositionedMeasurementsPredicate, Subject: "unpositioned raw measurements", Object: "observed originals",
		Summary: "时间未定位的原始量测记录；不计入时间窗统计", ObservedAt: at, Confidence: 1,
		SupportRefs: traceQueryObservationSupportRefs(ref, lineStart, lineEnd)}
	keep := len(rows)
	for {
		table := types.RuntimeMeasurementTable{ObservationID: r.ID, View: types.RuntimeMeasurementMembers, Label: "时间未定位的量测原始记录", DefaultPresentation: keep > 0,
			Columns: []string{"来源/记录/行", "过滤器ID", "过滤器名称", "引用状态", "量测类型", "过滤器类型", "原始source_arg_set_id", "原始值（单位未知）", "原起点(ns)", "原持续(ns)"}, Rows: rows[:keep],
			Notes: []string{"来源：" + ref.Path,
				"原始起点不是可定位的整数时间；保留原值、原持续和原始类型，不补零、不构造终点、不计入任何时间窗。查询扫描范围和载体排序坐标不是这些记录的发生时间。",
				"本表只展示本次查询已返回的原件，不表示全部未定位记录或采集完整；不改变已定位量测统计，不证明硬件身份、线程等待或因果关系。",
				fmt.Sprintf("本次事件查询匹配%d条、返回%d条；已返回事件中的未定位量测%d条，表格显示%d条，省略%d条。字段不截断，省略记录仍保留在原查询载荷。", c.MatchedTotal, c.Emitted, len(rows), keep, len(rows)-keep)}}
		publication := types.RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: ref, Tables: []types.RuntimeMeasurementTable{table}}
		data, err := json.Marshal(publication)
		if err != nil {
			return nil
		}
		if len(data) <= 64<<10 {
			r.RichNotes = []string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(data)}
			return []types.ObservationRecord{r}
		}
		if keep == 0 {
			return nil
		}
		keep--
	}
}

func traceQueryUnpositionedSourceMapped(result tracequery.Result, event tracequery.EventView) bool {
	if event.SourcePath == "" || event.LocalLine <= 0 || event.Line <= 0 {
		return false
	}
	if len(result.TraceArtifacts) == 0 {
		return event.SourcePath == result.SourcePath && event.LocalLine == event.Line
	}
	matches := 0
	for _, source := range result.TraceArtifacts {
		if source.SourcePath == event.SourcePath && event.LocalLine == event.Line-source.VirtualLineBase && event.LocalLine <= source.LocalLineCount {
			matches++
		}
	}
	return matches == 1
}
