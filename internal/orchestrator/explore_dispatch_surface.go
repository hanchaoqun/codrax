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
	out, dispatchErr := o.dispatchStage(types.StageExplore)
	o.busCtx.ExploreDispatchKey = prevDispatchKey
	o.busCtx.ExploreDispatchKind = prevDispatchKind
	o.busCtx.ExploreToolSurface = prevToolSurface
	return out, dispatchErr
}
