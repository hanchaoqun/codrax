package tool

import (
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceQueryUnknownViewRejection(view string, err error) types.ToolResult {
	return types.ToolResult{
		ToolName: "trace_query",
		Success:  false,
		Summary:  "trace_query rejected view: " + err.Error() + ". No trace query was executed. Re-call with a listed view, preserving the intended explicit window and business_span_ref; omit view only when event_search is intended.",
		Repair: &types.ToolRepair{
			Code:   "tool_param_invalid_enum_value",
			Fields: []string{"view"},
			Hint:   "Choose a supported view from allowed; output/statistic field names are not query views. Existing documented aliases remain accepted. Do not substitute an unrelated view or claim the requested statistics were computed.",
			Metadata: map[string]string{
				"field":   "view",
				"value":   strings.TrimSpace(view),
				"allowed": strings.Join(tracequery.CanonicalViewNames(), ", "),
			},
		},
		Timestamp: time.Now(),
	}
}
