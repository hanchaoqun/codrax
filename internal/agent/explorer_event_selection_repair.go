package agent

import "github.com/hanchaoqun/codrax/internal/types"

func (e *explorerEvaluator) pendingTraceEventSelectionRepair(results []types.ToolResult) *types.ToolRepair {
	if e == nil || e.mutable == nil {
		return nil
	}
	// The dispatch buffer retains failed calls even when observation handoffs
	// omit them. Do not let a compact answer-facing slice erase execution debt.
	if dispatched := e.mutable.DispatchToolResults(); len(dispatched) != 0 {
		results = dispatched
	}
	return e.mutable.PendingTraceEventSelectionRepair(results)
}
