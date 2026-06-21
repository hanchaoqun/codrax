package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) dispatchExploreWindow(window []*types.TaskNode) (*agent.StageOutput, error) {
	req := newExploreStageExecutionRequest(window)
	beforeArtifacts := captureExploreNodeArtifactProjectionSnapshot(o.busCtx, o.busCtx.Mutable)
	result := o.executeStageRequest(req)
	afterArtifacts := captureExploreNodeArtifactProjectionSnapshot(nil, o.busCtx.Mutable)
	o.ingestExploreNodeArtifactsForWindow(window, result.Output, beforeArtifacts, afterArtifacts)
	return result.Output, result.Err
}
