package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

// snapshotWorkflowVerificationResults attaches display-only facts to the exact
// attempt just recorded. It never changes that attempt's status or authority.
func snapshotWorkflowVerificationResults(batch *types.WriteWorkflowBatch, report *types.ChangeReport) {
	if batch == nil || len(batch.Attempts) == 0 {
		return
	}
	attempt := &batch.Attempts[len(batch.Attempts)-1]
	attempt.VerificationResults = types.BindWriteWorkflowVerificationResults(batch.PlanID, *attempt, report)
}
