package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func (e *explorerEvaluator) traceStatisticsAvailability(results []types.ToolResult) types.TraceStatisticsAvailability {
	if e == nil || e.mutable == nil {
		return types.TraceStatisticsAvailability{}
	}
	return e.mutable.TraceStatisticsAvailability(results)
}

func explorerStatisticsAvailabilitySignal(availability types.TraceStatisticsAvailability) LoopSignal {
	if !availability.HasPending() {
		return LoopSignal{}
	}
	return LoopSignal{HintRequested: true, HintKey: "explorer.mid-loop.statistics-available-navigation",
		Hint:     fmt.Sprintf("Native observations are addressable raw records, not yet computed coverage, duration or distribution for this exact source/window/selection. Optional derived views: %s. If the question only asks for source values, finish using those values without another query. If statistics are needed, use a listed view with the same selection, equivalent validated computation, or explicitly report the unavailable statistic; do not fill gaps/unknowns or infer totals from displayed rows. This is not a mandatory query or a new source-code obligation, and computed statistics alone do not resolve causal questions.\n", strings.Join(availability.PendingViews, ", ")),
		Progress: true, BypassThrottle: true, BypassBudget: true}
}
