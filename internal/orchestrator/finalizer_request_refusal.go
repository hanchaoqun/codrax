package orchestrator

import (
	"errors"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// finalizer_request_refusal.go — what the scheduler does when the answer
// stage refuses its own request before sending it.
//
// A refusal is deterministic: the request is built by local planning over the
// run's typed state, so a second dispatch over the same state builds the same
// request and is refused again. The scheduler therefore records each refusal
// with a content digest of that state (types.SampleAnswerRequestInputs) and
// does not run the forced dispatch when the digest is unchanged. The digest
// is the whole argument: nothing here trusts a list of mutation points, and a
// state the digest cannot read is treated as changed.

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

// answerRequestRefusedUnsent reports whether the run's last answer dispatch
// was refused with nothing of the answer stage sent to the model.
func (o *Orchestrator) answerRequestRefusedUnsent() bool {
	return finalizeRefusalOf(o.busCtx).NothingSent()
}

func finalizeRefusalOf(bus *types.BusContext) *types.FinalizerRequestRefusal {
	if bus == nil {
		return nil
	}
	return bus.Mutable.FinalizerRequestRefusal()
}

// repeatedAnswerRequestRefusal decides the forced dispatch after a refusal
// that sent nothing. It returns the refusal to report when the dispatch would
// rebuild the request from the same inputs, and nil when it must run: the
// inputs changed (a drained forced read, an ingested extract), the state
// could not be sampled, or the refusal was of a repair or follow-up request,
// whose fresh first request is a different one.
func (o *Orchestrator) repeatedAnswerRequestRefusal() error {
	refusal := finalizeRefusalOf(o.busCtx)
	if !refusal.NothingSent() {
		return nil
	}
	now := o.sampleAnswerRequestInputs()
	changed, same := refusal.InputsChangedSince(now)
	if !same {
		logging.Info("[orchestrator] forced finalize: the refused answer request would be rebuilt from different inputs (%s); dispatching",
			describeChangedAnswerInputs(changed, now))
		return nil
	}
	return &agent.AnswerContextByteBudgetError{ActualBytes: refusal.RequestBytes, AllocatedBytes: refusal.AllocationBytes}
}

func describeChangedAnswerInputs(changed []string, now types.TypedInputSample) string {
	if reason := now.Unknown(); reason != "" {
		return "state not sampled: " + reason
	}
	const shown = 12
	if len(changed) > shown {
		return strings.Join(changed[:shown], ",") + ",…"
	}
	if len(changed) == 0 {
		return "refused state not sampled"
	}
	return strings.Join(changed, ",")
}

// logForcedFinalizeFailure reports the forced dispatch truthfully: how many
// attempts were dispatched, and that none was when the refusal stood.
func logForcedFinalizeFailure(dispatched int, err error) {
	if dispatched == 0 {
		logging.Error("[orchestrator] forced finalize not dispatched (0 attempts): the answer request was refused before sending and its inputs are unchanged, so a rebuild would be refused again: %v", err)
		return
	}
	logging.Error("[orchestrator] forced finalize failed after %d dispatched attempt(s): %v", dispatched, err)
}

// refusedWithNothingSent reports whether err is the byte guard's refusal and
// the run's refusal record says that nothing of the answer stage reached the
// model. It is the one derivation of the reader's texts for a refused request,
// read from the same record as the root-cause side file's reason
// (outputdump.RootCauseReasonForUnformedAnswer). A refused repair or
// follow-up request — its dispatch had already sent a request, or an earlier
// answer dispatch of the run was not refused before sending — is not a
// request that was never sent.
func refusedWithNothingSent(err error, refusal *types.FinalizerRequestRefusal) bool {
	var refused *agent.AnswerContextByteBudgetError
	return errors.As(err, &refused) && refusal.NothingSent()
}

// finalizeNodeFailureText is the detail of the answer step's failure line.
// A request that was never sent is not a failed model response: the reader
// gets the notice for that case, and the raw error stays in the log. Every
// other failure keeps its own text, a refused repair request included: a
// request of the answer stage had been sent, and the forced path that follows
// composes the answer again, so the step reads as recoverable.
func finalizeNodeFailureText(err error, refusal *types.FinalizerRequestRefusal, lang string) string {
	if refusedWithNothingSent(err, refusal) {
		return answerRequestNotSentMessage(lang)
	}
	return err.Error()
}

func (o *Orchestrator) emitFinalizingNotice() {
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: render.NoticeFinalizing,
		Reasoning:  softFinalizingMessage(o.busCtx.Language),
	})
}

// UnformedAnswerRootCauseReason names, for a frontend that writes its own
// root-cause side file, why the last Run formed no answer. It reads the record
// the default side file was written from, so the two cannot disagree.
func (o *Orchestrator) UnformedAnswerRootCauseReason() string {
	if o == nil {
		return outputdump.RootCauseReasonForUnformedAnswer(nil)
	}
	return outputdump.RootCauseReasonForUnformedAnswer(o.unformedAnswerRefusal)
}
