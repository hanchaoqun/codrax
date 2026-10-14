package agent

import (
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Aggregate selection is a validated thread query, not ownership by a process
// whose numeric ID happens to equal that TID. Keep the kind through this
// display-only bridge; the original record and causal projection are unchanged.
func answerDocAggregateSelectionMatchesTarget(record types.ObservationRecord, rm *types.RequestModel) bool {
	if rm == nil {
		return false
	}
	subject, ok := tool.TraceQueryAggregateSelectionSubject(record)
	if !ok {
		return false
	}
	selected := record
	selected.Subject = subject
	threadRequest := &types.RequestModel{}
	for _, target := range rm.RuntimeTargets {
		if target.Kind != types.RuntimeTargetKindProcess {
			threadRequest.RuntimeTargets = append(threadRequest.RuntimeTargets, target)
		}
	}
	return types.ObservationRecordMatchesUserRuntimeTarget(selected, threadRequest)
}
