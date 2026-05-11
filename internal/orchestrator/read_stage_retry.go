package orchestrator

import (
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// retryReadStageDispatchError converts transient read-mode stage
// dispatch errors into a normal scheduler retry instead of forcing the
// partially-completed pipeline straight into extract/finalize.
//
// Two decoupled budgets:
//   - transientRetryBudget (orchestrator-owned) caps THIS path so a
//     network blip during explore / finalize does not drain the
//     content-retry budget that contract-violation retries depend on
//     OR the pipeline step budget that should count productive stage
//     dispatches.
//   - retryBudget (graph-owned) is reserved for content retries
//     (contract violations, validation feedback, etc.).
//
// Stall plateau detection: when the previous transient-failed attempt
// produced an identical tool-call signature with no terminal emit
// progress, further retry would just re-hit the same wall (e.g. the
// LLM is structurally confused by the request shape). Surface the
// terminal failure with the actionable stallPlateauMessage instead.
func (o *Orchestrator) retryReadStageDispatchError(
	state *graphState,
	stage types.PipelineStage,
	window []*types.TaskNode,
	fin *types.TaskNode,
	err error,
) bool {
	if o == nil || state == nil || o.busCtx == nil || err == nil {
		return false
	}
	// L4 only retries on stream-level errors (EOF / stream stalled /
	// first-byte timeout / network). HTTP 429 / 5xx are L1's domain
	// — by the time we see one here, L1 already exhausted its
	// 6-attempt × 62-second budget; retrying at L4 would burn 2×
	// more wall-clock for the same persistent upstream condition.
	// The IsStreamLevelRetryable subset filters this.
	if o.busCtx.Mode != types.ModeRead || !llm.IsStreamLevelRetryable(err) {
		return false
	}

	// Transient retry uses its own dedicated budget so stream stalls
	// don't drain the content-retry budget.
	if state.transientRetryUsed >= o.transientRetryBudget {
		return false
	}

	// Stall plateau detection. Compute a signature for the just-failed
	// dispatch and compare against the previous transient-retry's
	// signature. Identical pair (with no terminal emit between) means
	// "stuck on the same wall"; surface terminal failure rather than
	// burn another retry. Use the first window node's ID as the key
	// for explore (any node in the failing window suffices since the
	// whole window stalls on the same dispatch error). Finalize uses
	// the finalize node's ID.
	var keyID string
	switch stage {
	case types.StageExplore:
		if len(window) == 0 {
			return false
		}
		keyID = window[0].ID
	case types.StageFinalize:
		if fin == nil {
			return false
		}
		keyID = fin.ID
	default:
		return false
	}
	sig := computeStallSignature(o.busCtx.Mutable.DispatchToolResults(), nil, stage)
	if state.transientStallPlateau(keyID, sig) {
		friendly := stallPlateauMessage(o.busCtx, stage, friendlyDispatchErr(err), o.autoInitRepo, o.scaffoldEnabled)
		logging.Warning("[orchestrator] %s read-mode transient stall plateau (sig=%q) — suppressing retry: %s",
			stage, sig, friendly)
		o.busCtx.TaskState.LastError = friendly
		return false
	}
	state.rememberTransientSignature(keyID, sig)

	switch stage {
	case types.StageExplore:
		for _, n := range window {
			state.requeue(n.ID)
		}
	case types.StageFinalize:
		state.requeue(fin.ID)
	}

	state.recordTransientRetry()
	logging.Warning("[orchestrator] retrying %s after transient dispatch error (%d/%d transient budget; pipeline step budget unchanged): %v",
		stage, state.transientRetryUsed, o.transientRetryBudget, err)
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: render.NoticeRetry,
		Reasoning:  softRetryHintMessage(o.busCtx.Language),
	})
	return true
}

// retryReadStandaloneDispatchError is the same transient read-mode
// retry lane for stages that are dispatched as scheduler side-effects
// rather than graph nodes. Today that means the pre-finalize extract
// calls. A dropped stream there used to fall through as "proceeding
// without extract", which let a transport blip erase Turn-B answer
// symbol / verdict work. The retry is capped by transientRetryBudget
// and deliberately does not consume the pipeline step budget.
func (o *Orchestrator) retryReadStandaloneDispatchError(
	state *graphState,
	stage types.PipelineStage,
	err error,
) bool {
	if o == nil || state == nil || o.busCtx == nil || err == nil {
		return false
	}
	if o.busCtx.Mode != types.ModeRead || !llm.IsStreamLevelRetryable(err) {
		return false
	}
	if state.transientRetryUsed >= o.transientRetryBudget {
		return false
	}
	state.recordTransientRetry()
	o.busCtx.TaskState.LastError = ""
	logging.Warning("[orchestrator] retrying standalone %s after transient dispatch error (%d/%d transient budget; pipeline step budget unchanged): %v",
		stage, state.transientRetryUsed, o.transientRetryBudget, err)
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: render.NoticeRetry,
		Reasoning:  softRetryHintMessage(o.busCtx.Language),
	})
	return true
}
