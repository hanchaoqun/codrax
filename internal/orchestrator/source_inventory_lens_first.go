package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) prioritizeSourceInventoryLensWindow(window []*types.TaskNode, env criterion.Env) []*types.TaskNode {
	if sourceInventoryFollowupSuppressedByCompletionCaveat(o.busCtx) {
		o.busCtx.SourceInventoryFollowupDebt = types.SourceInventoryFollowupDebt{}
		return dropSourceInventoryFollowupNodes(window)
	}
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

func dropSourceInventoryFollowupNodes(window []*types.TaskNode) []*types.TaskNode {
	if len(window) == 0 {
		return window
	}
	out := make([]*types.TaskNode, 0, len(window))
	for _, node := range window {
		if sourceInventoryFollowupProbeNode(node) {
			continue
		}
		out = append(out, node)
	}
	return out
}

func sourceInventoryFollowupSuppressedByCompletionCaveat(ctx *types.BusContext) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return false
	}
	for _, caveat := range closure.CompletionCaveats() {
		if caveat.Lane == types.DowngradeLaneSourceInventoryCompletion {
			return true
		}
	}
	return false
}
