package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TraceSecond intentionally has no JSON marshal contract. Bind normalized
// coordinates explicitly, keeping a missing bound distinct from explicit 0.
func traceQueryEventSelectionScopeParams(p traceQueryParams) json.RawMessage {
	scope := map[string]any{"line_start": p.LineStart.Int(), "line_end": p.LineEnd.Int()}
	if p.TimeStart.Set() {
		scope["time_start"] = p.TimeStart.Seconds()
	}
	if p.TimeEnd.Set() {
		scope["time_end"] = p.TimeEnd.Seconds()
	}
	raw, _ := json.Marshal(scope)
	return raw
}

// This is a request-provenance check, not intent or operator classification.
// A native query can prove its own population complete without proving that
// an extra model-selected numeric predicate belongs to the user's population.
// Require a quoted source under every accepted bounded-fact contract. Listing
// records and counting them have the same population, even when an independent
// count/completeness classifier did not activate.
func traceQueryEventSelectionSourceRepair(ctx *types.BusContext, p traceQueryParams) *types.ToolResult {
	if ctx == nil || len(p.EventFieldFilters) == 0 || ctx.Mutable != nil && ctx.Mutable.SystemTraceSupplementInProgress() {
		return nil
	}
	var rm *types.RequestModel
	if ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	} else if ctx.Mutable != nil {
		rm = ctx.Mutable.RequestModel()
	}
	if rm == nil || !rm.RuntimeQuestionProfile.BoundedFactSet() {
		return nil
	}
	var fields []string
	for i, filter := range p.EventFieldFilters {
		if !sourceQuotePresentInCurrentRequest(rm.RawRequest, strings.TrimSpace(filter.SourceQuote)) {
			fields = append(fields, fmt.Sprintf("event_field_filters[%d].source_quote", i))
		}
	}
	if len(fields) == 0 {
		return nil
	}
	hint := "The requested bounded fact population cannot inherit extra filters from artifact previews or previously observed values. For each requested condition, provide source_quote copied verbatim from the current request stating that condition. Remove a filter only if the user did not request it; do not drop a requested threshold or alter time/line windows. Listing an identity column does not select one identity. A Trace report-time window is not an additional condition on business start/end fields, even on the same clock axis. The quote establishes request provenance only, not correctness of the field/operator/value: preserve legitimate number normalization and independently grounded identity resolution. No query ran and no count was produced."
	return &types.ToolResult{ToolName: "trace_query", Success: false, Summary: hint, Timestamp: time.Now(),
		Repair: &types.ToolRepair{Code: types.ToolRepairCodeTraceEventSelectionSourceRequired, Fields: fields, Hint: hint}}
}
