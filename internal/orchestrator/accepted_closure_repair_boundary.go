package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) repairBlocksAcceptedClosure(r types.RepairDirective) bool {
	if !types.RepairBlocksAcceptedClosure(r) {
		return false
	}
	if o.acceptedClosureHasCompletionCaveatForRepair(r) {
		logging.Info("[orchestrator] treating repair as advisory after downgrade convergence: kind=%s lane=%s origin=%q subject=%q",
			r.Kind, r.DowngradeLane, r.Origin, r.Subject)
		return false
	}
	if o.sourceInventoryClosureMakesSubjectRebindAdvisory(r) {
		logging.Info("[orchestrator] treating source-inventory subject rebind as advisory after exact-universe accepted closure: subject=%q origin=%q",
			r.Subject, r.Origin)
		return false
	}
	return true
}

func (o *Orchestrator) acceptedClosureHasCompletionCaveatForRepair(r types.RepairDirective) bool {
	if r.DowngradeLane == "" || o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return false
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	if closure == nil {
		return false
	}
	for _, caveat := range closure.CompletionCaveats() {
		if caveat.Lane == r.DowngradeLane {
			return true
		}
	}
	return false
}
