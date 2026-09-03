package orchestrator

// accepted_closure_reconcile.go — the reconcile-node auto-complete arm
// (split out of orchestrator.go under the IR delivery hot-file ratchet,
// §40.14 V7-2 / F14 fold-in). A ready NodeReconcile is auto-completed from
// existing evidence when the accepted-closure premise holds
// (accepted_closure_premise.go — shared with the explore-window consumer),
// the node's SuccessCriteria evaluate true, reconcile evidence context exists
// and no blocking reconcile repair is pending. The scheduler runs this arm
// BEFORE the explore-window checks, so the premise (including the
// explore-backtrack veto) MUST be read here too: before the fold-in a
// requeued reconcile node auto-completed from the retained pre-backtrack
// reason while the backtrack was still in force.

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) autoCompleteReadyReconcileNodes(state *graphState, window []*types.TaskNode, env criterion.Env) []*types.TaskNode {
	if len(window) == 0 {
		return window
	}
	remaining := make([]*types.TaskNode, 0, len(window))
	skipped := 0
	for _, n := range window {
		if !o.shouldAutoCompleteReadyReconcileNode(n, env) {
			remaining = append(remaining, n)
			continue
		}
		state.markRunning(n.ID)
		o.emitNodeStart(n.ID)
		state.markDone(n.ID)
		o.emitNodeEnd(n.ID, true, "skipped: reconciled from existing evidence")
		skipped++
		logging.Info("[orchestrator] auto-completed reconcile node %s from existing evidence", n.ID)
	}
	if skipped > 0 && len(remaining) == 0 {
		o.drainIgnorableReconcileRepairs()
	}
	return remaining
}

func (o *Orchestrator) shouldAutoCompleteReadyReconcileNode(n *types.TaskNode, env criterion.Env) bool {
	if n == nil || n.Type != types.NodeReconcile {
		return false
	}
	acceptedClosureEnough := o.acceptedClosureCanSatisfyReconcileEnoughFacts()
	if !acceptedClosureEnough && !env.Signals.HasEnoughFacts && (o.busCtx == nil || !o.busCtx.Signals.HasEnoughFacts) {
		return false
	}
	if len(n.SuccessCriteria) > 0 {
		evalEnv := env
		if acceptedClosureEnough {
			evalEnv.Signals.HasEnoughFacts = true
			evalEnv.InvestigationComplete = true
			if o.busCtx != nil && o.busCtx.Mutable != nil {
				evalEnv.AggregateFacts = o.busCtx.Mutable.StableInvestigationAggregateFacts()
			}
		}
		ok, _ := criterion.EvalAll(n.SuccessCriteria, evalEnv)
		if !ok {
			return false
		}
	}
	if !o.hasReconcileEvidenceContext() {
		return false
	}
	return !o.hasBlockingReconcileRepair()
}

// acceptedClosureCanSatisfyReconcileEnoughFacts is the reconcile-node
// accepted-closure consumer. It reads the SAME premise as the explore-window
// consumer (acceptedClosurePremise: backtrack veto + policy + completion
// mark) so a bound explore backtrack vetoes both arms until the explorer's
// fresh accepted completion advances the generation (§40.14 V7-2 / F14).
func (o *Orchestrator) acceptedClosureCanSatisfyReconcileEnoughFacts() bool {
	if !o.hasReconcileEvidenceContext() {
		return false
	}
	policy, ok := o.acceptedClosurePremise()
	if !ok {
		return false
	}
	if policy == types.ICPolicyOverride {
		return true
	}
	if missing := o.acceptedClosureMissingRequiredOriginsForAutoComplete(); len(missing) > 0 {
		logging.Info("[orchestrator] accepted investigation closure cannot auto-complete reconcile node; missing_origin_lanes=%s",
			formatAnswerEvidenceOriginsForLog(missing))
		return false
	}
	mut := o.busCtx.Mutable
	closure := mut.EvidenceClosure()
	if closure == nil {
		return true
	}
	for _, repair := range closure.PendingRepairs() {
		if o.repairBlocksAcceptedClosure(repair) {
			return false
		}
	}
	for _, pending := range closure.PendingReads() {
		if pendingReadBlocksAcceptedReconcileClosure(pending) {
			return false
		}
	}
	return true
}

func pendingReadBlocksAcceptedReconcileClosure(p types.PendingRead) bool {
	origin := strings.TrimSpace(p.Origin)
	if strings.HasPrefix(origin, "chain_promotion.") {
		return false
	}
	return types.PendingReadBlocksAcceptedClosure(p)
}
