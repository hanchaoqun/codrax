package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// accepted_closure_premise.go — the ONE premise every accepted-closure
// consumer reads before it treats an investigation closure as in force
// (§40.14 V7-2 / F14). Two scheduler consumers auto-complete work from an
// accepted closure: the explore window
// (shouldAutoCompleteExploreWindowFromAcceptedClosure) and the reconcile
// node (acceptedClosureCanSatisfyReconcileEnoughFacts). Before this file
// only the explore window consulted the explore-backtrack veto; the
// reconcile arm accepted on the retained pre-backtrack reason
// (StableInvestigationCompleteReason survives ResetInvestigationComplete by
// design) and — because the scheduler runs reconcile auto-complete BEFORE
// the veto-guarded explore-window checks — auto-completed a requeued
// reconcile node from the stale closure while the backtrack was in force.
//
// Premise (precise signals only, evaluated in this order):
//
//  1. no bound explore-contract backtrack is in force for the CURRENT
//     epoch / completion generation
//     (acceptedClosureHasActiveExploreContractBacktrack — the typed escape
//     lane is the explorer's next accepted completion decision, which
//     advances the generation and consumes the veto);
//  2. the investigation-complete policy is soft or override (strict never
//     auto-completes);
//  3. a completion mark exists: the live flag or the retained reason.
//
// Returns the effective policy so callers can apply the override
// short-circuit (override skips the origin-debt / pending-repair checks).
// Every MutableState read here is census-bound by
// hard_arm_mutable_carrier_census_test.go, and every consumer of this
// premise must be listed in that census (premise-consumer totality pin).
func (o *Orchestrator) acceptedClosurePremise() (policy string, ok bool) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return "", false
	}
	mut := o.busCtx.Mutable
	if acceptedClosureHasActiveExploreContractBacktrack(mut) {
		return "", false
	}
	policy = o.effectiveInvestigationCompletePolicy()
	if policy != types.ICPolicySoft && policy != types.ICPolicyOverride {
		return "", false
	}
	if !mut.IsInvestigationComplete() && strings.TrimSpace(mut.StableInvestigationCompleteReason()) == "" {
		return "", false
	}
	return policy, true
}
