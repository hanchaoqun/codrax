package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

const TraceMeasurementsPredicate = "measurements_observation"

func traceQueryMeasurementsSchema(schema json.RawMessage) json.RawMessage {
	var root map[string]any
	if json.Unmarshal(schema, &root) != nil {
		return schema
	}
	props, _ := root["properties"].(map[string]any)
	if view, ok := props["view"].(map[string]any); ok {
		prior, _ := view["description"].(string)
		view["description"] = prior + " " + tracequery.MeasurementsTeaching
	}
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(root) != nil {
		return schema
	}
	return b.Bytes()
}
func traceQueryMeasurementsInput(p traceQueryParams) *types.ToolResult {
	if tracequery.CanonicalViewName(p.View) != tracequery.ViewMeasurements {
		return nil
	}
	if p.PID.Int() != 0 || p.Thread != "" || p.TargetScope != "" || p.SpanName != "" || p.Pattern != "" || len(p.Patterns) > 0 || len(p.EventTypes.Strings()) > 0 || len(p.EventNames) > 0 || len(p.EventFieldFilters) > 0 || len(p.TraceMarkActions.Strings()) > 0 {
		hint := "measurements accepts source, time/line bounds and limit only; raw resource ownership is unknown, so PID/thread/target_scope and event/span filters do not apply"
		return &types.ToolResult{ToolName: "trace_query", Summary: hint, Timestamp: time.Now(), Repair: &types.ToolRepair{Code: "trace_query_raw_measurements_view", Fields: []string{"pid", "thread", "target_scope", "span_name", "pattern", "patterns", "event_types", "event_names", "event_field_filters", "trace_mark_actions"}, Hint: hint}}
	}
	return nil
}

func (t *TraceQuery) streamMeasurements(ctx *types.BusContext, p traceQueryParams, path, sourceLabel, callCaveat string, window traceQueryNormalizedWindow) (types.ToolResult, bool) {
	if tracequery.CanonicalViewName(p.View) != tracequery.ViewMeasurements || tracequery.TracePathRequiresCompositeIndex(path) {
		return types.ToolResult{}, false
	}
	q := traceQueryBuildQuery(ctx, p, sourceLabel, path, window.RequestedStart, window.RequestedEnd)
	result, err := tracequery.StreamMeasurements(contextFromBus(ctx), path, q)
	if err != nil {
		if traceQueryIsCancellation(err) {
			return traceQueryCancellationResult(p.View, path, err), true
		}
		return types.ToolResult{ToolName: t.Name(), Summary: "Measurement query failed: " + err.Error(), Timestamp: time.Now()}, true
	}
	traceQueryAppendCallCaveats(&result, callCaveat)
	payload, failure := traceQueryMarshalPayload(t.Name(), result)
	if failure != nil {
		return *failure, true
	}
	payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-query-result.json", string(payload))
	preview, rawRef := StoreBlob(ctx, t.Name(), traceQuerySummary(result, p, sourceLabel, payloadRef))
	if rawRef == "" {
		rawRef = payloadRef
	}
	now := time.Now()
	return types.ToolResult{ToolName: t.Name(), Success: true, Summary: preview, RawRef: rawRef, Timestamp: now, Observations: traceQueryTypedObservations(result, sourceLabel, payloadRef, rawRef, "", now, q), TraceQuerySourceRead: traceQuerySourceReadCandidate(result), TraceEvidenceAuthority: traceQueryEvidenceAuthorityWithSource(result, sourceLabel, payloadRef, rawRef, "", now, q)}, true
}

func traceQueryMeasurementsObservations(p *tracequery.MeasurementsResult, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if p == nil || !tracequery.ValidMeasurements(*p) {
		return nil
	}
	r := types.ObservationRecord{ID: "trace_query:" + scope + "#measurements", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref, Span: types.ObservationSpan{StartTs: p.Window.StartTs, EndTs: p.Window.EndTs}, ClaimKey: TraceMeasurementsPredicate, Predicate: TraceMeasurementsPredicate, Subject: "raw measurements", Object: p.Status, Summary: "Source values, filter references and explicit intervals; units and physical resource identity unverified", ObservedAt: at, Confidence: 1}
	note := traceQueryMeasurementsReceipt(r, *p)
	if note == "" {
		return nil
	}
	r.RichNotes = []string{note}
	return []types.ObservationRecord{r}
}

func measurementScalar(s tracewire.MeasureScalar) string {
	if s.StorageClass == "null" {
		return "未知（NULL）"
	}
	if s.StorageClass == "absent" {
		return "未提供"
	}
	if s.StorageClass == "text" {
		if s.Encoding == "base64" {
			return "TEXT原字节(base64):" + s.Value
		}
		return strconv.Quote(s.Value) + " (text)"
	}
	if s.StorageClass == "blob" {
		return "base64:" + s.Value + " (blob)"
	}
	return s.Value + " (" + s.StorageClass + ")"
}

func measurementReferenceLabel(status string) string {
	switch status {
	case "observed_unique":
		return "唯一记录引用"
	case "ambiguous":
		return "引用歧义"
	default:
		return "引用未确认"
	}
}
func measurementSelectionLabel(selection string) string {
	switch selection {
	case "interval_overlap":
		return "源区间与查询窗交集"
	case "point":
		return "源时间点"
	default:
		return "窗内时间点，持续未知"
	}
}

func traceQueryMeasurementsReceipt(r types.ObservationRecord, p tracequery.MeasurementsResult) string {
	start, end, known := types.TraceObservationContinuousQueryWindow(r.SourceRef)
	if !tracequery.ValidMeasurements(p) || !known || p.SourcePath != r.SourceRef.Path || p.Window.StartTs != start || p.Window.EndTs != end || p.Status != r.Object || r.Span.StartTs != start || r.Span.EndTs != end || r.SourceRef.QueryTargetPID != 0 || r.SourceRef.QueryTargetThread != "" || r.SourceRef.QueryTargetScope == tracequery.TargetScopeProcess {
		return ""
	}
	right := ")"
	if p.Window.EndInclusive {
		right = "]"
	}
	base := []string{fmt.Sprintf("查询窗口 [%s,%s%s 秒；来源 %s。", traceQueryDisplaySeconds(start), traceQueryDisplaySeconds(end), right, p.SourcePath), "状态：" + p.Status, "原值单位、活动状态和物理资源身份未经确认；过滤器唯一引用不代表 GPU/CPU/进程/线程，source_arg_set_id 仅原始引用。NULL不补零；未知持续不延长，重叠记录不合并。这些观测不是执行、等待、整帧或响应根因。"}
	base = append(base, p.Caveats...)
	keep := len(p.Rows)
	for {
		notes := append([]string(nil), base...)
		notes = append(notes, fmt.Sprintf("窗口匹配%d条；引擎省略%d条；表格显示%d条，额外省略%d条；无法定位时间%d条（不计入窗口）。无记录不等于0，记录数不证明采集完整。", p.TotalRows, p.OmittedRows, keep, len(p.Rows)-keep, p.UnpositionedRows))
		summary := types.RuntimeMeasurementTable{ObservationID: r.ID, View: types.RuntimeMeasurementSummary, Label: "各过滤器的原值观测序列", Columns: []string{"来源", "过滤器ID", "过滤器名称", "引用状态", "保留记录数", "原起点(ns)→原值（单位未知）"}, Notes: notes}
		members := types.RuntimeMeasurementTable{ObservationID: r.ID, View: types.RuntimeMeasurementMembers, Label: "量测原始记录", Columns: []string{"来源/记录/行", "过滤器ID", "过滤器名称", "引用状态", "量测类型", "过滤器类型", "原始source_arg_set_id", "原始值（单位未知）", "原起点(ns)", "原持续(ns)"}, Notes: notes}
		timeline := types.RuntimeMeasurementTable{ObservationID: r.ID, View: types.RuntimeMeasurementTimeline, Label: "量测窗口内观测", Columns: []string{"来源/记录/行", "过滤器ID", "原始值（单位未知）", "窗口内起点(ns)", "窗口内终点(ns)", "选取依据"}, Notes: notes}
		positions := map[string]int{}
		var sequences [][]string
		for _, row := range p.Rows[:keep] {
			rec := row.Record
			name, kind, arg := "未确认", "未确认", "未确认"
			if f := rec.Filter; f != nil {
				name, kind, arg = measurementScalar(f.Name), measurementScalar(f.Type), measurementScalar(f.SourceArgSetID)
			}
			ref := fmt.Sprintf("%s / %d / %d", row.SourcePath, rec.RowID, row.SourceLine)
			members.Rows = append(members.Rows, []string{ref, measurementScalar(rec.FilterID), name, measurementReferenceLabel(rec.FilterStatus), measurementScalar(rec.MeasureType), kind, arg, measurementScalar(rec.Value), measurementScalar(rec.StartNS), measurementScalar(rec.DurationNS)})
			timeline.Rows = append(timeline.Rows, []string{ref, measurementScalar(rec.FilterID), measurementScalar(rec.Value), processMeasurementEndpoint(row.ClippedStartNS), processMeasurementEndpoint(row.ClippedEndNS), measurementSelectionLabel(row.Selection)})
			parts := []any{row.SourcePath, rec.FilterID, rec.FilterStatus, rec.Filter, rec.MeasureType}
			if rec.FilterStatus != "observed_unique" {
				parts = append(parts, rec.RowID)
			}
			key, _ := json.Marshal(parts)
			pos, exists := positions[string(key)]
			if !exists {
				pos = len(summary.Rows)
				positions[string(key)] = pos
				summary.Rows = append(summary.Rows, []string{row.SourcePath, measurementScalar(rec.FilterID), name, measurementReferenceLabel(rec.FilterStatus), "", ""})
				sequences = append(sequences, nil)
			}
			sequences[pos] = append(sequences[pos], measurementScalar(rec.StartNS)+"→"+measurementScalar(rec.Value))
		}
		for i := range summary.Rows {
			summary.Rows[i][4] = strconv.Itoa(len(sequences[i]))
			summary.Rows[i][5] = strings.Join(sequences[i], "; ")
		}
		raw, err := json.Marshal(types.RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef, Tables: []types.RuntimeMeasurementTable{summary, members, timeline}})
		if err != nil {
			return ""
		}
		if len(raw) <= 64<<10 {
			return types.TraceNoteKeyRuntimeMeasurement + "=" + string(raw)
		}
		if keep == 0 {
			return ""
		}
		keep--
	}
}

func TraceMeasurementsText(p tracequery.MeasurementsResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "原始量测：状态=%s；窗口匹配%d条，保留%d条，省略%d条；时间未知%d条（不分配到窗口）。单位及物理资源身份未知。\n", p.Status, p.TotalRows, len(p.Rows), p.OmittedRows, p.UnpositionedRows)
	for _, row := range p.Rows {
		r := row.Record
		name, kind, arg := "未确认", "未确认", "未确认"
		if f := r.Filter; f != nil {
			name, kind, arg = measurementScalar(f.Name), measurementScalar(f.Type), measurementScalar(f.SourceArgSetID)
		}
		fmt.Fprintf(&b, "- 记录%d/源行%d；过滤器=%s（%s），名称=%s，过滤器类型=%s，原始source_arg_set_id=%s；量测类型=%s；原值=%s；原ts=%s ns，dur=%s ns；窗内=%s..%s ns（%s）。\n", r.RowID, row.SourceLine, measurementScalar(r.FilterID), measurementReferenceLabel(r.FilterStatus), name, kind, arg, measurementScalar(r.MeasureType), measurementScalar(r.Value), measurementScalar(r.StartNS), measurementScalar(r.DurationNS), processMeasurementEndpoint(row.ClippedStartNS), processMeasurementEndpoint(row.ClippedEndNS), measurementSelectionLabel(row.Selection))
	}
	for _, c := range p.Caveats {
		b.WriteString("- " + c + "\n")
	}
	return b.String()
}
