package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) prioritizeSourceInventoryLensWindow(window []*types.TaskNode, env criterion.Env) []*types.TaskNode {
	lensWindow := sourceInventoryLensFirstWindow(window, env.SourceInventoryProfileActive, env.SourceInventoryLensExecuted)
	if len(lensWindow) == 0 {
		return window
	}
	logging.Info("[orchestrator] prioritizing source-inventory lens probe before broad evidence reads")
	return lensWindow
}

func (o *Orchestrator) seedRequiredFileHintForcedReadsBeforeExploreForWindow(window []*types.TaskNode) int {
	if sourceInventoryLensProbeOnlyWindow(window) {
		logging.Info("[CGEC] pre-dispatch required-file forced-read skipped for source-inventory lens-first probe")
		return 0
	}
	return o.seedRequiredFileHintForcedReadsBeforeExplore()
}
