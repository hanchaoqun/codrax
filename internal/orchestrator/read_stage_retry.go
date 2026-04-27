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
	if o.busCtx.Mode != types.ModeRead || !llm.IsRetryableDispatchError(err) || state.retryBudgetExhausted() {
		return false
	}

	switch stage {
	case types.StageExplore:
		if len(window) == 0 {
			return false
		}
		for _, n := range window {
			state.requeue(n.ID)
		}
	case types.StageFinalize:
		if fin == nil {
			return false
		}
		state.requeue(fin.ID)
	default:
		return false
	}

	state.recordRetry()
	logging.Warning("[orchestrator] retrying %s after transient dispatch error (%d/%d): %v",
		stage, state.retryUsed, o.busCtx.AnalysisIR.TaskGraph.ExecutionPolicy.RetryBudget, err)
	o.emit(render.Event{
		Kind:      render.EventAgentReasoning,
		Timestamp: time.Now(),
		Agent:     "orchestrator",
		Reasoning: softRetryHintMessage(o.busCtx.Language),
	})
	return true
}
