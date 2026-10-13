package tool

import (
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This is a request-provenance check, not intent or operator classification.
// A native query can prove its own population complete without proving that
// an extra model-selected numeric predicate belongs to the user's population.
// Require a quoted source only under an accepted all/count fact contract.
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
	if rm == nil || !rm.RuntimeQuestionProfile.BoundedFactSet() ||
		(!rm.Predicates.IsCountQuestion && !rm.CompletenessObligation.IsActive()) {
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
	hint := "The requested all/count population cannot inherit extra filters from artifact previews or previously observed values. For each requested condition, provide source_quote copied verbatim from the current request stating that condition. Remove a filter only if the user did not request it; do not drop a requested threshold or alter time/line windows. Listing an identity column does not select one identity. The quote establishes request provenance only, not correctness of the field/operator/value: preserve legitimate number normalization and independently grounded identity resolution. No query ran and no count was produced."
	return &types.ToolResult{ToolName: "trace_query", Success: false, Summary: hint, Timestamp: time.Now(),
		Repair: &types.ToolRepair{Code: "trace_query_event_selection_source_required", Fields: fields, Hint: hint}}
}
