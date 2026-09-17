package tool

import (
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func traceQueryEventFieldFilterSchema() string {
	fields, _ := json.Marshal(tracequery.EventFieldFilterFields())
	ops, _ := json.Marshal(tracequery.EventFieldFilterOps())
	description, _ := json.Marshal(skill.TraceJankQueryContract)
	return fmt.Sprintf(`{"type":"array","maxItems":%d,"items":{"type":"object","additionalProperties":false,"required":["field","op","value"],"properties":{"field":{"type":"string","enum":%s},"op":{"type":"string","enum":%s},"value":{"oneOf":[{"type":"string","pattern":"^-?[0-9]+$"},{"type":"integer"}],"description":"Exact decimal int64; prefer a string for native nanosecond timestamps. Missing, fractional and overflowing values are rejected."}}},"description":%s}`,
		tracequery.EventFieldFilterLimit, fields, ops, description)
}

func traceQueryEventFieldFiltersJSON(filters []tracequery.EventFieldFilter) string {
	if len(filters) == 0 {
		return ""
	}
	raw, _ := json.Marshal(filters)
	return string(raw)
}

func traceQueryJankEventDetail(event tracequery.Event) string {
	if summary := tracequery.JankEventSummary(event); summary != "" {
		return " " + summary
	}
	return ""
}
