package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// This is transient call-stack context, not persisted/model-authored state.
// Capturing the batch's original run prevents a changed ambient snapshot from
// borrowing another run's completed source work during a later retry.
type controllerPlanResultScope struct {
	owner          *types.MutableState
	runID, batchID string
}

func newControllerPlanResultScope(mu *types.MutableState, batch *writeflow.WriteBatchPlan) *controllerPlanResultScope {
	if mu == nil || batch == nil || batch.ID == "" {
		return nil
	}
	run := mu.WriteWorkflowRun()
	if run == nil || run.RunID == "" || run.ActiveBatchID != batch.ID {
		return nil
	}
	return &controllerPlanResultScope{owner: mu, runID: run.RunID, batchID: batch.ID}
}

func (o *Orchestrator) runControllerPlanStageInResultScope(scope *controllerPlanResultScope, stepsUsed *int) (*agent.StageOutput, error) {
	prior := o.controllerPlanResultScope
	o.controllerPlanResultScope = scope
	defer func() { o.controllerPlanResultScope = prior }()
	return o.runControllerWriteStage(types.StagePlan, stepsUsed)
}

// Only suppresses an inapplicable result write. Errors, retries, terminal
// eligibility, proof and applied work continue through their existing owners.
// No prior answer text is inspected or rewritten.
func (o *Orchestrator) deferOptionalFollowupPlanErrorResult() bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mode != types.ModeApply ||
		o.busCtx.PipelineStage != types.StagePlan || o.busCtx.Mutable == nil {
		return false
	}
	scope := o.controllerPlanResultScope
	mu := o.busCtx.Mutable
	if scope == nil || scope.owner != mu || scope.runID == "" || scope.batchID == "" {
		return false
	}
	run := mu.WriteWorkflowRun()
	if run == nil || run.RunID != scope.runID || run.ActiveBatchID != scope.batchID ||
		run.Status != types.WriteWorkflowRunInProgress || !activeBatchOptionalFollowupPurpose(run) ||
		activeBatchHasVerifyFailureHandoff(mu, scope.batchID) ||
		writeWorkflowActiveBatchHasFailedVerify(run) || writeWorkflowActiveBatchHasAppliedAttempt(run) ||
		!workflowRunNonActiveBatchesComplete(run) {
		return false
	}
	active, ok := activeWriteWorkflowControllerBatch(run)
	if !ok || active.Status != types.WriteWorkflowBatchReadyToPlan {
		return false
	}
	// Completion labels alone do not establish delivered code. Require an
	// actual apply-attempt record for an identified completed source plan.
	for _, batch := range run.Batches {
		if batch.ID == scope.batchID || batch.Status != types.WriteWorkflowBatchComplete || batch.PlanID == "" {
			continue
		}
		for _, attempt := range batch.Attempts {
			if attempt.Kind == "apply" && attempt.Status == "applied" && attempt.PlanID == batch.PlanID {
				return true
			}
		}
	}
	return false
}
