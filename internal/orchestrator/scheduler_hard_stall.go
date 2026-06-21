package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

func (o *Orchestrator) prepareHardStallFinalize(state *graphState) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return
	}
	o.busCtx.Mutable.SetTerminationProfile(types.TerminationProfile{Kind: types.TerminationHardStall})
	o.injectInconclusiveForStuckHypotheses("scheduler_hard_stall")
	o.drainHypothesisVerdicts()
	if state != nil {
		state.forceCloseExploreWindow()
	}
}
