package tool

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Publish the existing engine family's actual member intervals independently
// of its causal seat. The family envelope, union and member sum remain three
// distinct quantities. No parser, fold, chain credential or rank is changed.
// This is display-only: a complete retained family is not a complete capture.
func traceQuerySpanMemberMeasurementObservations(result tracequery.Result, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	start, end, known := types.TraceObservationContinuousQueryWindow(ref)
	stats := result.WindowStats
	if !known || stats == nil || stats.Window.StartTs != start || stats.Window.EndTs != end ||
		ref.Path == "" || ref.Path != result.SourcePath || ref.QueryScopeID == "" || ref.PayloadRef == "" {
		return nil
	}
	var out []types.ObservationRecord
	for i, family := range tracequery.FoldSemanticSpanFamilies(traceQueryResultWakeupChain(result), stats.TraceSpans) {
		if i >= traceQueryWidthTypedFamilyRowCap() {
			break
		}
		table, ok := traceQuerySpanMemberMeasurementTable(family, ref, start, end)
		if !ok {
			continue // Original native observations remain available unchanged.
		}
		id := fmt.Sprintf("trace_query:%s#span_members:%d", scope, i+1)
		table.ObservationID = id
		record := types.ObservationRecord{ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref,
			Span:     types.ObservationSpan{LineStart: family.StartLine, LineEnd: family.EndLine, StartTs: family.StartTs, EndTs: family.EndTs},
			ClaimKey: "trace_span_members_observation", Predicate: "trace_span_members_observation", Subject: traceThreadLabel(family.Thread),
			Object: "observed span members", Value: strconv.Itoa(len(family.Members)), Unit: "intervals", Summary: table.Label,
			SupportRefs: traceQueryObservationSupportRefs(ref, family.StartLine, family.EndLine), ObservedAt: at, Confidence: 1}
		publication := types.RuntimeMeasurementPublication{Version: 1, ObservationID: id, Source: ref, Tables: []types.RuntimeMeasurementTable{table}}
		raw, err := json.Marshal(publication)
		if err != nil || len(raw) > 64<<10 {
			continue // Never publish a partial member set as the retained set.
		}
		record.RichNotes = []string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(raw)}
		record = types.PublishRuntimeMeasurementQueryWindow(record, start, end, false)
		out = append(out, record)
	}
	return out
}

func traceQuerySpanMemberMeasurementTable(family tracequery.SemanticSpanFamily, ref types.ObservationSourceRef, start, end float64) (types.RuntimeMeasurementTable, bool) {
	table := types.RuntimeMeasurementTable{View: types.RuntimeMeasurementMembers,
		Label:   "逐段业务区间 — " + traceThreadLabel(family.Thread),
		Columns: []string{"执行线程", "业务打点", "开始（秒，含）", "结束（秒，不含）", "窗口内耗时（ms）", "原始配对开始（秒）", "原始配对结束（秒）", "完整配对耗时（ms）", "起点行", "终点行", "宿主进程ID", "打点声明进程ID"},
		Notes: []string{"来源：" + ref.Path,
			fmt.Sprintf("查询窗口 [%s, %s) 秒；本表保留该原生集合的 %d 个成员及其原顺序，不表示采集或窗口内全部业务完整。", traceQueryDisplaySeconds(start), traceQueryDisplaySeconds(end), len(family.Members)),
			"每行是独立的原生配对区间；集合首尾包络中的空档不属于这些成员，不能用包络代替逐成员相交、包含或先后关系。耗时保留原生窗口内量测，原始配对范围另列；成员可能重叠，不将逐行和改称区间并集。业务名称不是状态、调用、依赖或根因证明。"}}
	if len(family.Members) == 0 || family.SourcePath != ref.Path {
		return table, false
	}
	seen := map[[2]int]bool{}
	for _, member := range family.Members {
		lines := [2]int{member.StartLine, member.EndLine}
		if member.SourcePath != ref.Path || member.Thread != family.Thread || member.Name == "" ||
			member.StartLine <= 0 || member.EndLine < member.StartLine || seen[lines] ||
			!traceQueryNativeIntervalValid(member.StartTs, member.EndTs, member.DurationMs, start, end) || member.EndTs <= member.StartTs {
			return table, false
		}
		seen[lines] = true
		actualStart, actualEnd, actualMS := member.StartTs, member.EndTs, member.DurationMs
		if member.ActualStartTs != 0 || member.ActualEndTs != 0 || member.ActualDurationMs != 0 {
			actualStart, actualEnd, actualMS = member.ActualStartTs, member.ActualEndTs, member.ActualDurationMs
			if !traceQueryNativeIntervalValid(actualStart, actualEnd, actualMS, actualStart, actualEnd) || actualStart > member.StartTs || actualEnd < member.EndTs {
				return table, false
			}
		}
		table.Rows = append(table.Rows, []string{traceThreadLabel(member.Thread), member.Name,
			traceQueryDisplaySeconds(member.StartTs), traceQueryDisplaySeconds(member.EndTs), strconv.FormatFloat(member.DurationMs, 'g', 12, 64),
			traceQueryDisplaySeconds(actualStart), traceQueryDisplaySeconds(actualEnd), strconv.FormatFloat(actualMS, 'g', 12, 64),
			strconv.Itoa(member.StartLine), strconv.Itoa(member.EndLine), traceQueryNativeLine(member.Thread.TGID), traceQueryNativeLine(member.SpanPID)})
	}
	return table, true
}
