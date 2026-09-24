package tool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TraceBusinessTreeFact retains the producer's instance identity and both
// independent omission layers. Line coordinates use the query index space;
// physical source-local citations come from the common SupportRefs mapper.
type TraceBusinessTreeFact struct {
	IndexPath               string                           `json:"index_path"`
	WindowUnavailableReason string                           `json:"window_unavailable_reason,omitempty"`
	Window                  tracequery.TraceMarkerTreeWindow `json:"window"`
	NodeCount               int                              `json:"node_count"`
	OmittedNodes            int                              `json:"omitted_nodes"`
	Coverage                string                           `json:"coverage"`
	Caveats                 []string                         `json:"caveats,omitempty"`
	Node                    tracequery.TraceMarkerTreeNode   `json:"node"`
}

func traceQueryTypedBusinessTreeObservations(tree *tracequery.TraceMarkerTreeStats, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if tree == nil {
		return nil
	}
	var out []types.ObservationRecord
	for _, node := range tree.Nodes {
		fact := TraceBusinessTreeFact{IndexPath: ref.Path, Window: tree.Window, WindowUnavailableReason: tree.WindowUnavailableReason,
			NodeCount: tree.NodeCount, OmittedNodes: tree.OmittedNodes, Coverage: tree.Coverage, Caveats: tree.Caveats, Node: node}
		data, err := json.Marshal(fact)
		if err != nil || node.ID == "" || node.SourcePath == "" || node.Thread.PID <= 0 || node.StartLine <= 0 {
			continue
		}
		span, value := traceBusinessTreeFactProjection(fact)
		notes := []string{types.TraceNoteKeyBusinessTreeNode + "=" + string(data)}
		if tree.WindowUnavailableReason == "" {
			notes = append(notes, types.TraceNoteKeySelectedWindow+"="+traceQuerySelectedWindowNoteValue(tracequery.TimeWindow{StartTs: tree.Window.StartTs, EndTs: tree.Window.EndTs, StartSet: true}))
		}
		out = append(out, types.ObservationRecord{
			ID:     fmt.Sprintf("trace_query:%s#trace_business_tree:%d", scope, len(out)+1),
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref,
			Span:     span,
			ClaimKey: types.TraceBusinessTreePredicate + ":" + node.ID,
			Subject:  traceThreadLabel(node.Thread), Predicate: types.TraceBusinessTreePredicate,
			Object: node.Name, Value: value, Unit: "ms",
			Summary:     "Observed synchronous business nesting; elapsed and self time are not causal contribution or removable time",
			RichNotes:   notes,
			SupportRefs: traceQueryObservationSupportRefs(ref, span.LineStart, span.LineEnd),
			ObservedAt:  at, Confidence: .95,
		})
	}
	return out
}

// DecodeTraceBusinessTreeFact is read-only factual display validation, not a
// causal or request-routing gate. Never borrow a tree from another query.
func DecodeTraceBusinessTreeFact(record types.ObservationRecord) (TraceBusinessTreeFact, bool) {
	var fact TraceBusinessTreeFact
	if record.Predicate != types.TraceBusinessTreePredicate || record.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
		!types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) || record.GroundingPolicy != types.ClaimGroundingHard ||
		record.SourceRef.Kind != types.ObservationSourceRuntimeArtifact || record.ProvenanceLane != types.ObservationProvenanceArtifactSpan ||
		record.SourceRef.QueryScopeID == "" || record.SourceRef.PayloadRef == "" {
		return fact, false
	}
	for _, note := range record.RichNotes {
		if strings.HasPrefix(note, types.TraceNoteKeyBusinessTreeNode+"=") {
			if json.Unmarshal([]byte(strings.TrimPrefix(note, types.TraceNoteKeyBusinessTreeNode+"=")), &fact) != nil {
				return fact, false
			}
			n := fact.Node
			span, value := traceBusinessTreeFactProjection(fact)
			physicalSupport := false
			for _, support := range record.SupportRefs {
				physicalSupport = physicalSupport || strings.HasPrefix(support, n.SourcePath+":")
			}
			windowMatches := fact.WindowUnavailableReason == "" && record.SourceRef.QueryWindowKnown &&
				fact.Window.StartTs == record.SourceRef.QueryWindowStartTs && fact.Window.EndTs == record.SourceRef.QueryWindowEndTs
			if fact.WindowUnavailableReason == "line_selected_time_window_undetermined" {
				windowMatches = !record.SourceRef.QueryWindowKnown && record.SourceRef.QueryLineRangeKnown
			}
			return fact, traceBusinessTreeFactValid(fact) && physicalSupport && fact.IndexPath == record.SourceRef.Path && n.Name == record.Object &&
				traceThreadLabel(n.Thread) == record.Subject && n.StartLine == record.Span.LineStart &&
				span.LineEnd == record.Span.LineEnd && span.StartTs == record.Span.StartTs && span.EndTs == record.Span.EndTs &&
				value == record.Value && record.Unit == "ms" && record.Role == types.AnswerAggregateRoleSupportingCoverage &&
				record.ClaimKey == types.TraceBusinessTreePredicate+":"+n.ID && windowMatches
		}
	}
	return fact, false
}

func traceBusinessTreeFactProjection(f TraceBusinessTreeFact) (types.ObservationSpan, string) {
	n := f.Node
	s := types.ObservationSpan{LineStart: n.StartLine, LineEnd: max(n.StartLine, n.EndLine), StartTs: f.Window.StartTs, EndTs: f.Window.EndTs}
	if f.WindowUnavailableReason != "" {
		s.StartTs, s.EndTs = n.ActualStartTs, n.ActualStartTs
	}
	value := ""
	if n.Inclusive != nil && n.ActualEndTs != nil {
		if len(n.Inclusive.Segments) == 1 {
			s.StartTs, s.EndTs = n.Inclusive.Segments[0].StartTs, n.Inclusive.Segments[0].EndTs
		}
		value = traceQueryObservationMSValue(n.Inclusive.DurationMs)
	}
	return s, value
}

// TraceBusinessTreeNodeText names relationships by physical instance ID, never
// by ambiguous display names. Keep exact numeric values; format is not a gate.
func TraceBusinessTreeNodeText(n tracequery.TraceMarkerTreeNode) string {
	parent := "父层未确定（此前打点栈不可见）"
	switch n.ParentStatus {
	case "observed_parent":
		parent = fmt.Sprintf("父实例=%q（真实同步嵌套；父可能未在本次展示）", n.ParentID)
	case "observed_root":
		parent = "当前观察到的最外层（不保证是程序调用根）"
	}
	closure := "未配对闭合，不提供完整耗时，也不证明仍在占用窗口"
	if n.Closure == "closed" {
		closure = "已配对闭合"
	} else if n.Closure == "invalidated" {
		closure = "配对已失效，不提供完整耗时"
	}
	end := "未知"
	if n.ActualEndTs != nil {
		end = traceQueryDisplaySeconds(*n.ActualEndTs)
	}
	return fmt.Sprintf("业务=%q；线程=%q；实例=%q；%s；完整打点区间（当前Trace时钟）=%s..%s 秒；%s；完整实例中直接子数量=%d；总耗时{%s}；自身耗时{%s}",
		n.Name, traceThreadLabel(n.Thread), n.ID, parent, traceQueryDisplaySeconds(n.ActualStartTs), end, closure, n.DirectChildCount,
		traceBusinessTreeAccountText(n.Inclusive), traceBusinessTreeAccountText(n.Self))
}

func traceBusinessTreeAccountText(a *tracequery.TraceMarkerTreeAccount) string {
	if a == nil {
		return "不可计量，不是零"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%.9g 毫秒；计量分段=", a.DurationMs)
	for i, s := range a.Segments {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "[%s,%s)", traceQueryDisplaySeconds(s.StartTs), traceQueryDisplaySeconds(s.EndTs))
	}
	fmt.Fprintf(&b, " 秒；未展示分段=%d", a.OmittedSegments)
	s := a.States
	if s == nil || s.Values == nil {
		b.WriteString("；线程状态未能计量，不按零处理")
		return b.String()
	}
	v := s.Values
	fmt.Fprintf(&b, "；状态交集（毫秒）：运行=%.9g、等待调度=%.9g、睡眠=%.9g、%s=%.9g、调度标记IO等待=%.9g、停止=%.9g、退出=%.9g、已计量=%.9g、未知=%.9g；睡眠中IO等待=%.9g（睡眠包含项）",
		v.RunningMs, v.RunnableMs, v.SleepMs, TraceStateNonIODStateWord(true), v.DStateMs, v.IOWaitMs, v.StoppedMs, v.DeadMs, v.AccountedMs, s.UnknownMs, v.SleepIOWaitMs)
	return b.String()
}

func writeTraceBusinessTree(b *strings.Builder, tree *tracequery.TraceMarkerTreeStats, limit int) {
	if tree == nil {
		return
	}
	shown := min(limit, len(tree.Nodes))
	fmt.Fprintf(b, "- 同步业务层级：%s；全线程观察库存按起始行展示（不是目标过滤或耗时排名）；观察到 %d 个实例，本摘要展示 %d；原生未展示 %d、摘要另省略 %d。总耗时含子层；自身耗时排除完整直接子区间并集。不是唤醒链或根因树。\n",
		TraceBusinessTreeWindowText(TraceBusinessTreeFact{Window: tree.Window, WindowUnavailableReason: tree.WindowUnavailableReason}), tree.NodeCount, shown, tree.OmittedNodes, len(tree.Nodes)-shown)
	for _, n := range tree.Nodes[:shown] {
		fmt.Fprintf(b, "  - %s；来源=%q；索引行=%d..%d\n", TraceBusinessTreeNodeText(n), n.SourcePath, n.StartLine, n.EndLine)
	}
}

func TraceBusinessTreeWindowText(f TraceBusinessTreeFact) string {
	if f.WindowUnavailableReason != "" {
		return "按行范围选择，未建立完整查询时间窗；各业务片段有独立的已配对区间"
	}
	return fmt.Sprintf("所选窗口=%s..%s 秒", traceQueryDisplaySeconds(f.Window.StartTs), traceQueryDisplaySeconds(f.Window.EndTs))
}
