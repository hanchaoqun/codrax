package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const TracePreferredFrameRatePredicate = "preferred_frame_rate_observation"

func traceQueryPreferredFrameRateSchema(schema json.RawMessage) json.RawMessage {
	var obj map[string]any
	if json.Unmarshal(schema, &obj) != nil {
		return schema
	}
	properties, _ := obj["properties"].(map[string]any)
	for _, key := range []string{"view", "pid", "target_scope"} {
		if field, ok := properties[key].(map[string]any); ok {
			description, _ := field["description"].(string)
			extra := " preferred_frame_rate defaults to process ownership with optional process pid; no thread scope."
			if key == "view" {
				extra = " " + tracequery.PreferredFrameRateTeaching
			}
			field["description"] = description + extra
		}
	}
	var out bytes.Buffer
	e := json.NewEncoder(&out)
	e.SetEscapeHTML(false)
	if e.Encode(obj) != nil {
		return schema
	}
	return out.Bytes()
}

func traceQueryPreferredFrameRateObservations(p *tracequery.PreferredFrameRateResult, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if p == nil || !tracequery.ValidPreferredFrameRate(*p) {
		return nil
	}
	r := types.ObservationRecord{ID: "trace_query:" + scope + "#preferred_frame_rate", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref, Span: types.ObservationSpan{StartTs: p.Window.StartTs, EndTs: p.Window.EndTs}, ClaimKey: TracePreferredFrameRatePredicate, Predicate: TracePreferredFrameRatePredicate, Subject: "preferred frame-rate observations", Object: p.Status, Summary: "Process/filter-owned expected-rate observations and coverage; not display refresh, thread execution or response causes", ObservedAt: at, Confidence: 1}
	note := traceQueryPreferredFrameRateReceipt(r, *p)
	if note == "" {
		return nil
	}
	r.RichNotes = []string{note}
	return []types.ObservationRecord{r}
}

func preferredFrameRateOwner(s tracequery.PreferredFrameRateSeries) string {
	if s.PID != nil {
		return fmt.Sprintf("%s (PID %d, ipid %s, filter %s)", s.ProcessName, *s.PID, s.IPID, s.FilterID)
	}
	return fmt.Sprintf("归属未确认 (%s, filter %s, 源记录%s)", s.OwnerStatus, s.FilterID, s.UnknownOwnerRowID)
}

func traceQueryPreferredFrameRateReceipt(r types.ObservationRecord, p tracequery.PreferredFrameRateResult) string {
	start, end, known := types.TraceObservationContinuousQueryWindow(r.SourceRef)
	if !tracequery.ValidPreferredFrameRate(p) || !known || p.SourcePath != r.SourceRef.Path || p.Window.StartTs != start || p.Window.EndTs != end || p.Status != r.Object || r.Span.StartTs != start || r.Span.EndTs != end || p.TargetPID != r.SourceRef.QueryTargetPID || r.SourceRef.QueryTargetThread != "" || p.TargetScope != r.SourceRef.QueryTargetScope {
		return ""
	}
	base := []string{fmt.Sprintf("查询窗口 [%s,%s) 秒。", traceQueryDisplaySeconds(start), traceQueryDisplaySeconds(end)), "Source: " + p.SourcePath, "状态：" + p.Status, "这是进程/过滤器的期望帧率观测（Hz），不代表屏幕实际刷新、实际执行、帧预算或投票因果；不默认60Hz，不回退到别的进程。", "同值重叠只算一次，异值重叠列为冲突；无有效值与没有覆盖分开，百分比分母为每条序列的完整查询墙钟。不可跨进程或过滤器相加。"}
	base = append(base, p.Caveats...)
	keep := len(p.Series)
	for {
		notes := append([]string(nil), base...)
		notes = append(notes, fmt.Sprintf("匹配序列%d，查询展示%d、省略%d；表格保留%d、另省略%d。窗口内原始记录%d（载荷保留%d、省略%d），另有%d条原始时间未知，未分配到查询窗口。", p.TotalSeries, len(p.Series), p.OmittedSeries, keep, len(p.Series)-keep, p.TotalObservations, len(p.Observations), p.OmittedObservations, p.UnpositionedRows))
		summary := types.RuntimeMeasurementTable{ObservationID: r.ID, View: types.RuntimeMeasurementSummary, Label: "期望帧率覆盖（逐进程与过滤器）", Columns: []string{"来源", "进程与过滤器", "窗内记录", "时间未知记录", "已知覆盖(ns)", "冲突(ns)", "无有效值(ns)", "无观测覆盖(ns)"}, Notes: notes}
		distribution := types.RuntimeMeasurementTable{ObservationID: r.ID, View: types.RuntimeMeasurementDistribution, Label: "期望帧率已知覆盖分布", Columns: []string{"来源", "进程与过滤器", "期望帧率(Hz)", "已知覆盖(ns)", "合并区间数", "占完整窗口(%)"}, Notes: notes}
		timeline := types.RuntimeMeasurementTable{ObservationID: r.ID, View: types.RuntimeMeasurementTimeline, Label: "期望帧率覆盖时序", Columns: []string{"来源", "进程与过滤器", "起点(ns)", "终点(ns)", "状态", "期望帧率(Hz)", "示例源行/涉及行数"}, Notes: notes}
		for _, s := range p.Series[:keep] {
			owner := preferredFrameRateOwner(s)
			summary.Rows = append(summary.Rows, []string{s.SourcePath, owner, strconv.Itoa(s.Observations), strconv.Itoa(s.UnpositionedRows), strconv.FormatInt(s.KnownDurationNS, 10), strconv.FormatInt(s.ConflictDurationNS, 10), strconv.FormatInt(s.UnknownValueDurationNS, 10), strconv.FormatInt(s.UnobservedDurationNS, 10)})
			for _, d := range s.Distribution {
				distribution.Rows = append(distribution.Rows, []string{s.SourcePath, owner, d.RateHz, strconv.FormatInt(d.DurationNS, 10), strconv.Itoa(d.Intervals), strconv.FormatFloat(d.WindowPercent, 'f', 6, 64)})
			}
			for _, w := range s.Timeline {
				state := map[string]string{"known": "已知", "conflict": "冲突", "unknown_value": "无有效值", "unobserved": "无观测覆盖"}[w.State]
				rate := w.RateHz
				if rate == "" {
					rate = "未知"
				}
				refs := fmt.Sprintf("%v / %d（省略%d）", w.SourceLines, w.TotalSourceLines, w.OmittedSourceLines)
				timeline.Rows = append(timeline.Rows, []string{s.SourcePath, owner, strconv.FormatInt(w.StartNS, 10), strconv.FormatInt(w.EndNS, 10), state, rate, refs})
			}
			extra := fmt.Sprintf("%s：分布展示%d/总%d，时序展示%d/总%d；省略不改变覆盖总体。", owner, len(s.Distribution), s.TotalDistribution, len(s.Timeline), s.TotalIntervals)
			distribution.Notes = append(distribution.Notes, extra)
			timeline.Notes = append(timeline.Notes, extra)
		}
		raw, err := json.Marshal(types.RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef, Tables: []types.RuntimeMeasurementTable{summary, distribution, timeline}})
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

func TracePreferredFrameRateText(p tracequery.PreferredFrameRateResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "期望帧率观测：[%s,%s)秒，状态=%s；序列%d、保留%d、省略%d；未知时间记录%d。\n", traceQueryDisplaySeconds(p.Window.StartTs), traceQueryDisplaySeconds(p.Window.EndTs), p.Status, p.TotalSeries, len(p.Series), p.OmittedSeries, p.UnpositionedRows)
	b.WriteString("按进程与过滤器独立统计，非屏幕实际刷新/投票因果；冲突与缺测不补60Hz。\n")
	for i, s := range p.Series {
		if i == 8 {
			fmt.Fprintf(&b, "预览另省略%d条序列；完整表由runtime_measurement选择。\n", len(p.Series)-i)
			break
		}
		fmt.Fprintf(&b, "- %s：已知=%dns；冲突=%dns；无有效值=%dns；未覆盖=%dns。\n", preferredFrameRateOwner(s), s.KnownDurationNS, s.ConflictDurationNS, s.UnknownValueDurationNS, s.UnobservedDurationNS)
	}
	for _, c := range p.Caveats {
		fmt.Fprintf(&b, "- %s\n", c)
	}
	return b.String()
}
