package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) dispatchExploreWindow(window []*types.TaskNode) (*agent.StageOutput, error) {
	prevDispatchKey := o.busCtx.ExploreDispatchKey
	prevDispatchKind := o.busCtx.ExploreDispatchKind
	prevToolSurface := o.busCtx.ExploreToolSurface
	o.busCtx.ExploreDispatchKey = exploreDispatchKeyForWindow(window)
	o.busCtx.ExploreDispatchKind = exploreDispatchKindForWindow(window)
	o.busCtx.ExploreToolSurface = exploreToolSurfaceForWindow(window)
	beforeArtifacts := captureExploreNodeArtifactProjectionSnapshot(o.busCtx, o.busCtx.Mutable)
	out, dispatchErr := o.dispatchStage(types.StageExplore)
	afterArtifacts := captureExploreNodeArtifactProjectionSnapshot(nil, o.busCtx.Mutable)
	o.ingestExploreNodeArtifactsForWindow(window, out, beforeArtifacts, afterArtifacts)
	o.busCtx.ExploreDispatchKey = prevDispatchKey
	o.busCtx.ExploreDispatchKind = prevDispatchKind
	o.busCtx.ExploreToolSurface = prevToolSurface
	return out, dispatchErr
}
