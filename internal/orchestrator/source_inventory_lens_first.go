package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) prioritizeSourceInventoryLensWindow(window []*types.TaskNode, env criterion.Env) []*types.TaskNode {
	lensWindow := sourceInventoryLensFirstWindow(window, env.SourceInventoryProfileActive, env.SourceInventoryLensExecuted)
	if len(lensWindow) == 0 {
		followupWindow := sourceInventoryFollowupFirstWindow(window, env.SourceInventoryFollowupDebt.IsActive())
		if len(followupWindow) == 0 {
			o.busCtx.SourceInventoryFollowupDebt = types.SourceInventoryFollowupDebt{}
			return window
		}
		o.busCtx.SourceInventoryFollowupDebt = types.NormalizeSourceInventoryFollowupDebt(env.SourceInventoryFollowupDebt)
		logging.Info("[orchestrator] prioritizing source-inventory follow-up probe from typed debt")
		return followupWindow
	}
	o.busCtx.SourceInventoryFollowupDebt = types.SourceInventoryFollowupDebt{}
	logging.Info("[orchestrator] prioritizing source-inventory lens probe before broad evidence reads")
	return lensWindow
}

func (o *Orchestrator) seedRequiredFileHintForcedReadsBeforeExploreForWindow(window []*types.TaskNode) int {
	if sourceInventoryLensProbeOnlyWindow(window) || sourceInventoryFollowupProbeOnlyWindow(window) {
		logging.Info("[CGEC] pre-dispatch required-file forced-read skipped for source-inventory probe")
		return 0
	}
	return o.seedRequiredFileHintForcedReadsBeforeExplore()
}
