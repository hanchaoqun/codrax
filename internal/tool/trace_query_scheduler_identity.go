package tool

import (
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Reuse the exact display semantics later published to inventory/selectors.
// Never recover identities by parsing Raw or by borrowing neighboring rows.
func traceEventSchedulerIdentityDetail(event tracequery.Event) string {
	switch event.Type {
	case tracequery.EventSchedWakeup, tracequery.EventSchedWaking, tracequery.EventSchedSwitch:
	default:
		return ""
	}
	semantics := tracequery.ProjectTraceEventSemantics(event)
	if semantics == nil {
		return ""
	}
	var fields []string
	for _, field := range semantics.Fields {
		descriptor, ok := types.LookupTraceEventSemanticDescriptor(field.Key)
		if !ok || descriptor.Family != "scheduler" {
			continue
		}
		value := "未知"
		if field.Status == "known" && field.Value != nil {
			value = *field.Value
			if field.Type == "text" {
				value = strconv.Quote(value)
			}
		} else if field.Status == "invalid" {
			value = "无效"
		} else if field.Status == "omitted" {
			value = "已省略"
		}
		fields = append(fields, descriptor.Label+"="+value)
	}
	if len(fields) == 0 {
		return ""
	}
	return " " + strings.Join(fields, " ")
}
