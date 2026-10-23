package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The source-bound selector is shared with native tables/diagrams. This
// formatter never derives a new population or reclassifies window exclusions
// as missing owners, and preserves whole events with their original frames.
func TraceResourceStackProjectedText(p tracequery.ResourceStackResult, projection types.RuntimeResourceDisplayProjection, limit int) string {
	rows := make([]tracequery.ResourceStackEvent, 0, len(projection.SelectedEventIndexes))
	last := -1
	for _, index := range projection.SelectedEventIndexes {
		if index <= last || index < 0 || index >= len(p.Events) {
			return ""
		}
		rows = append(rows, p.Events[index])
		last = index
	}
	var b strings.Builder
	fmt.Fprintf(&b, "原查询目标：scope=%s，id=%d，name=%q；原查询匹配事件=%d，原交接省略事件=%d。\n", p.TargetScope, p.TargetPID, p.TargetThread, p.MatchedEvents, p.OmittedEvents)
	b.WriteString(projection.BoundaryNote)
	b.WriteByte('\n')
	copy := p
	copy.Events = rows
	b.WriteString(traceResourceStackText(copy, limit, &projection))
	return b.String()
}

// TraceResourceStackSelectedText displays source-indexed members without
// rewriting the original query census. Selection is presentation only: it is
// neither a new query result nor completeness or causal authority.
func TraceResourceStackSelectedText(p tracequery.ResourceStackResult, selected []int, limit int) string {
	rows := make([]tracequery.ResourceStackEvent, 0, len(selected))
	last := -1
	for _, index := range selected {
		if index <= last || index < 0 || index >= len(p.Events) {
			return ""
		}
		rows = append(rows, p.Events[index])
		last = index
	}
	var b strings.Builder
	fmt.Fprintf(&b, "原查询目标：scope=%s，id=%d，name=%q；原查询匹配事件=%d，原交接省略事件=%d。\n", p.TargetScope, p.TargetPID, p.TargetThread, p.MatchedEvents, p.OmittedEvents)
	fmt.Fprintf(&b, "按请求目标展示成员=%d；另排除未匹配owner的已保留事件=%d；原查询总体未改，未显示或未查询成员不能判为不存在。\n", len(rows), len(p.Events)-len(rows))
	copy := p
	copy.Events = rows
	b.WriteString(TraceResourceStackText(copy, limit))
	return b.String()
}
