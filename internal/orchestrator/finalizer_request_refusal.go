package orchestrator

import (
	"errors"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// finalizer_request_refusal.go — what the scheduler keeps when the answer
// stage refuses its own request before sending it.
//
// A refusal is deterministic: the request is built by local planning over the
// run's typed state. The scheduler records each refusal with a content digest
// of that state (types.SampleAnswerRequestInputs) and with whether anything
// of the answer stage had reached the model.

// executeFinalizeStageRequest runs one answer dispatch and keeps the run's
// refusal record in step with how it returned.
func (o *Orchestrator) executeFinalizeStageRequest(node *types.TaskNode, reasonCode string) StageExecutionResult {
	result := o.executeStageRequest(newFinalizeStageExecutionRequest(node, reasonCode))
	o.recordFinalizeDispatchResult(result.Err)
	return result
}

func (o *Orchestrator) recordFinalizeDispatchResult(err error) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return
	}
	var refused *agent.AnswerContextByteBudgetError
	if !errors.As(err, &refused) {
		o.busCtx.Mutable.RecordFinalizeDispatchNotRefused()
		return
	}
	inputs := o.sampleAnswerRequestInputs()
	record := o.busCtx.Mutable.RecordFinalizeRequestRefusal(refused.ActualBytes, refused.AllocatedBytes, refused.RequestsSent, inputs)
	digest := record.InputDigest
	if digest == "" {
		digest = "unavailable (" + inputs.Unknown() + ")"
	}
	logging.Info("[orchestrator] answer request refused before sending: kind=%s request_bytes=%d allocation_bytes=%d requests_sent=%d input_digest=%s",
		record.Kind, record.RequestBytes, record.AllocationBytes, record.RequestsSent, digest)
}

func (o *Orchestrator) sampleAnswerRequestInputs() types.TypedInputSample {
	return types.SampleAnswerRequestInputs(o.busCtx, types.AnswerDispatchStamp{EmitRetryAttempt: o.emitStageRetryAttempt})
}

func finalizeRefusalOf(bus *types.BusContext) *types.FinalizerRequestRefusal {
	if bus == nil {
		return nil
	}
	return bus.Mutable.FinalizerRequestRefusal()
}
