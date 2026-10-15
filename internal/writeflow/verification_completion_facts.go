package writeflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// appendWorkflowFinishProgress records facts separately from the model's reason.
// It does not claim workflow verification authority: that remains Completion,
// which the scheduler can still downgrade after checking cumulative obligations.
func appendWorkflowFinishProgress(run *types.WriteWorkflowRun, decision WriteWorkflowDecision) {
	var facts []string
	for _, batch := range run.Batches {
		if batch.Status != types.WriteWorkflowBatchComplete {
			continue
		}
		counts := types.CurrentWorkflowVerificationResults(batch)
		if counts == nil {
			facts = append(facts, fmt.Sprintf("Batch %s: verification result count unavailable (no current bound report snapshot).", batch.ID))
			continue
		}
		facts = append(facts, fmt.Sprintf("Batch %s: %s (plan %s; report %s).", batch.ID,
			counts.Results.RecordedResultsText(false), counts.PlanID, counts.ReportID))
	}
	message := "Workflow completed."
	if len(facts) == 0 {
		facts = append(facts, "Verification result count unavailable (no current bound report snapshot).")
	}
	message += " " + strings.Join(facts, " ") + " Recorded results are not a count of independent tests; terminal verification status is recorded separately."
	appendWorkflowProgress(run, run.ActiveBatchID, "controller", string(decision.Action), decision.ReasonCode, message)
	run.ProgressLedger[len(run.ProgressLedger)-1].AdvisoryMessage = decision.Reason
}
