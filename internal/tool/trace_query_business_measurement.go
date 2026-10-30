package tool

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Publish after physical provenance is applied. The business interval remains
// distinct from the query window: displaying its owner's state partition must
// neither nominate a causal focus nor borrow the broader window's quantities.
func traceQueryBusinessMeasurementPublications(result tracequery.Result, records []types.ObservationRecord) []types.ObservationRecord {
	if result.WindowStats == nil {
		return records
	}
	for i := range records {
		r := &records[i]
		if r.Predicate != types.TraceBusinessSpanPredicate {
			continue
		}
		for _, span := range result.WindowStats.TraceSpans {
			if span.Name != r.Object || traceThreadLabel(span.Thread) != r.Subject || span.SourcePath != r.SourceRef.Path ||
				span.StartTs != r.Span.StartTs || span.EndTs != r.Span.EndTs || span.StartLine != r.Span.LineStart || span.EndLine != r.Span.LineEnd {
				continue
			}
			if note := traceQueryBusinessMeasurementReceipt(*r, span); note != "" {
				r.RichNotes = append(r.RichNotes, note)
				if annotation := traceQueryBusinessDiagramAnnotationNote(*r, span); annotation != "" {
					r.RichNotes = append(r.RichNotes, annotation)
				}
				*r = types.PublishRuntimeMeasurementQueryWindow(*r, result.WindowStats.Window.StartTs, result.WindowStats.Window.EndTs, false)
			}
			break
		}
	}
	return records
}

func traceQueryBusinessMeasurementReceipt(r types.ObservationRecord, span tracequery.TraceSpanSummary) string {
	start, end, known := types.TraceObservationContinuousQueryWindow(r.SourceRef)
	states := span.SchedulerStates
	if !known || span.Kind != "sync" || span.StartTs < start || span.EndTs > end ||
		!states.MatchesOwner(r.SourceRef.Path, span.Thread, span.StartTs, span.EndTs) {
		return ""
	}
	value := func(v float64) string {
		if states.Coverage == "unavailable" {
			return "未知"
		}
		return strconv.FormatFloat(v, 'g', 9, 64)
	}
	coverage := map[string]string{"complete": "完整", "partial": "部分，未覆盖部分未知", "unavailable": "不可用，未知不是零"}[states.Coverage]
	table := types.RuntimeMeasurementTable{ObservationID: r.ID, View: types.RuntimeMeasurementSummary,
		Label: "业务区间量测 — " + span.Name, DefaultPresentation: true, Columns: []string{"量测", "原生观测值"},
		Rows: [][]string{
			{"业务", span.Name}, {"执行线程", r.Subject},
			{"业务区间开始（秒，含）", traceQueryDisplaySeconds(span.StartTs)}, {"业务区间结束（秒，不含）", traceQueryDisplaySeconds(span.EndTs)},
			{"窗口内业务耗时（ms）", strconv.FormatFloat(span.DurationMs, 'g', 9, 64)}, {"调度状态覆盖", coverage},
			{"运行（ms）", value(states.RunningMs)}, {"可运行但未上CPU（ms）", value(states.RunnableMs)},
			{"睡眠（ms）", value(states.SleepMs)}, {TraceStateNonIODStateWord(true) + "（ms）", value(states.DStateMs)},
			{"D态且标记IO等待（ms）", value(states.IOWaitMs)}, {"睡眠中标记IO等待（ms，已含于睡眠）", value(states.SleepIOWaitMs)},
			{"已分类状态合计（ms）", value(states.AccountedMs)},
		}, Notes: []string{
			"来源：" + r.SourceRef.Path,
			fmt.Sprintf("查询窗口 [%s, %s) 秒；表中数值仅属于该业务区间，不能用整个查询窗口的线程统计替代。", traceQueryDisplaySeconds(start), traceQueryDisplaySeconds(end)),
			"本表为含子层的总区间量测，不能跨嵌套业务相加以免重复计量；此限制不适用于已扣除子区间的自身分段，后者须按同源、同线程、同窗同单位及已证互斥范围另计，省略或未知不能补零或外推全集。调度状态不证明等待机制、唤醒依赖或响应根因，睡眠中的IO等待标记不是额外加项。",
		}}
	if span.ActualEndTs > span.ActualStartTs {
		table.Notes = append(table.Notes, fmt.Sprintf("原始配对业务区间 [%s, %s) 秒，完整耗时 %s ms；裁切时不把完整耗时计入当前业务区间。", traceQueryDisplaySeconds(span.ActualStartTs), traceQueryDisplaySeconds(span.ActualEndTs), strconv.FormatFloat(span.ActualDurationMs, 'g', 9, 64)))
	}
	table.Notes = append(table.Notes, states.Caveats...)
	p := types.RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef, Tables: []types.RuntimeMeasurementTable{table}}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > 64<<10 {
		return ""
	}
	return types.TraceNoteKeyRuntimeMeasurement + "=" + string(raw)
}
