package tool

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

type businessSelfMeasurement struct {
	source types.ObservationSourceRef
	node   tracequery.TraceMarkerTreeNode
}

type businessSelfMeasurementIndex map[string][]businessSelfMeasurement

// Both input populations have already received physical artifact provenance.
// Index existing typed accounts once; never re-open a source or derive self
// from the displayed children, which may be only a prefix of the topology.
func traceQueryBusinessSelfMeasurementIndex(records []types.ObservationRecord) businessSelfMeasurementIndex {
	out := businessSelfMeasurementIndex{}
	for _, r := range records {
		if r.Predicate != types.TraceBusinessTreePredicate || r.Negative {
			continue
		}
		notes := 0
		for _, note := range r.RichNotes {
			if strings.HasPrefix(note, types.TraceNoteKeyBusinessTreeNode+"=") {
				notes++
			}
		}
		f, ok := DecodeTraceBusinessTreeFact(r)
		if !ok || notes != 1 || f.WindowUnavailableReason != "" || f.Node.Closure != "closed" || f.Node.SourcePath != r.SourceRef.Path {
			continue
		}
		key := businessSelfMeasurementKey(r)
		if key == "" {
			continue
		}
		out[key] = append(out[key], businessSelfMeasurement{source: r.SourceRef, node: f.Node})
	}
	return out
}

func businessSelfMeasurementKey(r types.ObservationRecord) string {
	raw, err := json.Marshal(struct {
		Source          types.ObservationSourceRef
		Span            types.ObservationSpan
		Subject, Object string
	}{r.SourceRef, r.Span, r.Subject, r.Object})
	if err != nil {
		return ""
	}
	return string(raw)
}

func (index businessSelfMeasurementIndex) match(r types.ObservationRecord, span tracequery.TraceSpanSummary) *tracequery.TraceMarkerTreeNode {
	key := businessSelfMeasurementKey(r)
	if key == "" {
		return nil
	}
	rows := index[key]
	if len(rows) != 1 || !reflect.DeepEqual(rows[0].source, r.SourceRef) {
		return nil // Duplicate or cross-query accounts do not supply authority.
	}
	n := rows[0].node
	start, end := span.StartTs, span.EndTs
	if span.ActualDurationMs > 0 { // Existing typed signal that this pair was clipped.
		start, end = span.ActualStartTs, span.ActualEndTs
	}
	if n.SourcePath != span.SourcePath || n.Thread != span.Thread || n.Name != span.Name ||
		n.StartLine != span.StartLine || n.EndLine != span.EndLine || n.ActualEndTs == nil ||
		n.ActualStartTs != start || *n.ActualEndTs != end {
		return nil
	}
	return &n
}

// Keep the established 13-row summary and its row-reference coordinates.
// The whole existing account formatter is reused as one producer-owned note;
// summary/table/edgeless facts all carry the same source-bound note.
func appendTraceBusinessSelfMeasurement(table *types.RuntimeMeasurementTable, node *tracequery.TraceMarkerTreeNode) {
	if node == nil || node.Self == nil {
		table.Notes = append(table.Notes, "自身账户未验证：未匹配到同一查询、线程和物理打点的唯一自身账户；不能从已展示子行重算，也不能按零或完整实例自身耗时代替。")
		return
	}
	table.Notes = append(table.Notes, "自身账户（窗口内墙钟，不是运行时间）："+traceBusinessTreeAccountText(node.Self),
		"自身账户扣除完整直接子区间并集，并与本表同一查询裁窗、物理打点和线程绑定；状态只在自身离散区间内计量，不能套用整个业务区间或另一查询的状态。已展示分段及其省略数不改变已测总量；睡眠内IO标记是包含项，不重复相加；不增加完整成员、因果或调查完成权限。")
}
