package orchestrator

import (
	"context"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// dispatchStage adds an execution-only settlement boundary around the existing
// stage runner. Ordinary closure and partial-output recovery stay in the core;
// only a new business focus waits for a successful, uncanceled worker result.
// This is deliberately outside the byte-preserved read scheduler loop.
func (o *Orchestrator) dispatchStage(stage types.PipelineStage) (*agent.StageOutput, error) {
	if stage != types.StageExplore || o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return o.dispatchStageCore(stage)
	}
	ticket := o.busCtx.Mutable.BeginTraceBusinessFocusDispatch()
	out, err := o.dispatchStageCore(stage)
	o.busCtx.Mutable.SettleTraceBusinessFocusDispatch(ticket,
		traceBusinessFocusWorkerSucceeded(out, err, o.CancelContext(), o.busCtx.Context()))
	return out, err
}

func traceBusinessFocusWorkerSucceeded(out *agent.StageOutput, err error, contexts ...context.Context) bool {
	if err != nil || out == nil || out.Error != "" {
		return false
	}
	for _, ctx := range contexts {
		if ctx != nil && ctx.Err() != nil {
			return false
		}
	}
	return true
}
