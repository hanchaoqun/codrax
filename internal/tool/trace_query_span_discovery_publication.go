package tool

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Span discovery returns paired intervals through SpanWindows, not WindowStats.
// Preserve that result in the same native measured-fact channel; neither a
// model's investigation summary nor a later scheduler query is a prerequisite.
// Other views may use SpanWindows for synthetic frame navigation. Their
// established producers remain authoritative; this does not mint extra roots.
func traceQueryTypedDiscoveredSpanObservations(result tracequery.Result, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if tracequery.CanonicalViewName(result.View) != "span_window" {
		return nil
	}
	stats := tracequery.WindowStats{
		Window:     tracequery.TimeWindow{StartTs: result.TimeStart, EndTs: result.TimeEnd},
		TraceSpans: result.SpanWindows,
	}
	// Classified names are still just observed pairs here: discovery alone
	// supplies no runtime optimization mechanism, rank or causal membership.
	return traceQueryBusinessSpanObservationRows(stats, ref, scope+":paired", at, true)
}

func traceQueryDiscoveredSpanMeasurementPublications(result tracequery.Result, records []types.ObservationRecord, queries ...tracequery.Query) []types.ObservationRecord {
	if tracequery.CanonicalViewName(result.View) != "span_window" {
		return records
	}
	for i := range records {
		r := &records[i]
		if r.Predicate != types.TraceBusinessSpanPredicate {
			continue
		}
		for _, span := range result.SpanWindows {
			if span.Name != r.Object || traceThreadLabel(span.Thread) != r.Subject || span.SourcePath != r.SourceRef.Path ||
				span.StartTs != r.Span.StartTs || span.EndTs != r.Span.EndTs || span.StartLine != r.Span.LineStart || span.EndLine != r.Span.LineEnd {
				continue
			}
			if note := traceQueryDiscoveredSpanMeasurementReceipt(*r, span, queries...); note != "" {
				r.RichNotes = append(r.RichNotes, note)
				start, end, _ := types.TraceObservationContinuousQueryWindow(r.SourceRef)
				*r = types.PublishRuntimeMeasurementQueryWindow(*r, start, end, false)
			}
			break
		}
	}
	return records
}

func traceQueryDiscoveredSpanMeasurementReceipt(r types.ObservationRecord, span tracequery.TraceSpanSummary, queries ...tracequery.Query) string {
	start, end, known := types.TraceObservationContinuousQueryWindow(r.SourceRef)
	if !known || span.StartTs < start || span.EndTs > end || span.EndTs <= span.StartTs {
		return ""
	}
	kind := map[string]string{"sync": "同步配对区间", "async": "异步配对区间"}[span.Kind]
	if kind == "" {
		kind = "已观测配对区间"
	}
	table := types.RuntimeMeasurementTable{ObservationID: r.ID, View: types.RuntimeMeasurementSummary,
		Label: "业务配对区间 — " + span.Name, DefaultPresentation: true, Columns: []string{"量测", "原生观测值"},
		Rows: [][]string{
			{"业务", span.Name}, {"记录所属线程", r.Subject}, {"区间类型", kind},
			{"业务区间开始（秒，含）", traceQueryDisplaySeconds(span.StartTs)}, {"业务区间结束（秒，不含）", traceQueryDisplaySeconds(span.EndTs)},
			{"已配对业务耗时（ms）", strconv.FormatFloat(span.DurationMs, 'g', 9, 64)},
		}, Notes: []string{
			"来源：" + r.SourceRef.Path,
			fmt.Sprintf("本次定位结果覆盖 [%s, %s) 秒；配对行 %d–%d。查找范围用于选择相交业务，完整配对区间可以超出查找范围，不是仅在原始筛选窗内裁切后的量测。", traceQueryDisplaySeconds(start), traceQueryDisplaySeconds(end), span.StartLine, span.EndLine),
			"本表仅证明已配对的业务区间及其耗时；未查询自身分段和调度状态，不是零，不能把墙钟耗时当CPU执行、等待机制或根因贡献。不同嵌套区间可能重叠，不能将其总量直接相加。",
		}}
	if len(queries) > 0 {
		q := queries[0]
		if (q.TimeStartSet || q.TimeStart > 0) && (q.TimeEndSet || q.TimeEnd > 0) && q.TimeEnd > q.TimeStart {
			table.Notes = append(table.Notes, fmt.Sprintf("原始查找条件：time_start=%s、time_end=%s 秒；按原生查找边界选择业务，不将查找条件改写为裁切量测窗。它与上面的业务配对区间和定位结果覆盖范围分别保留。", traceQueryDisplaySeconds(q.TimeStart), traceQueryDisplaySeconds(q.TimeEnd)))
		}
	}
	if span.ActualEndTs > span.ActualStartTs {
		table.Notes = append(table.Notes, fmt.Sprintf("原始配对业务区间 [%s, %s) 秒，完整耗时 %s ms；裁切时不把完整耗时计入当前业务区间。", traceQueryDisplaySeconds(span.ActualStartTs), traceQueryDisplaySeconds(span.ActualEndTs), strconv.FormatFloat(span.ActualDurationMs, 'g', 9, 64)))
	}
	p := types.RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef, Tables: []types.RuntimeMeasurementTable{table}}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > 64<<10 {
		return ""
	}
	return types.TraceNoteKeyRuntimeMeasurement + "=" + string(raw)
}
