package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// accepted_closure_premise_test.go — F14: the single accepted-closure premise
// shared by the explore-window and reconcile-node auto-complete consumers.

// The two accepted-closure consumers share one premise: every configuration
// of (policy, veto, completion mark) yields the same premise verdict for the
// explore window and the reconcile node. Divergence between the two arms is
// exactly the class F14 closed.
func TestAcceptedClosurePremise_SharedByExploreWindowAndReconcileConsumers(t *testing.T) {
	type step struct {
		name string
		arm  func(mut *types.MutableState)
		want bool
	}
	steps := []step{
		{name: "no completion mark", arm: func(*types.MutableState) {}, want: false},
		{name: "accepted completion", arm: func(m *types.MutableState) { m.SetInvestigationComplete("accepted") }, want: true},
		{name: "bound explore backtrack", arm: func(m *types.MutableState) {
			m.SetRetryState(exploreOwnedRetryState())
			m.ResetForFallback(types.FallbackResetTargetExplore)
			m.ResetInvestigationComplete()
		}, want: false},
		{name: "fresh completion consumes the veto", arm: func(m *types.MutableState) { m.SetInvestigationComplete("fresh") }, want: true},
		// §40.43 R3 F: live flag false, retained reason present, NO veto —
		// the premise holds on the retained mark. A consumer gating on
		// IsInvestigationComplete() alone reads false here; a consumer that
		// ignores the premise result reads true at the bound-backtrack step.
		{name: "retained reason without veto", arm: func(m *types.MutableState) { m.ResetInvestigationComplete() }, want: true},
		{name: "unbound finalizer retry state never vetoes", arm: func(m *types.MutableState) {
			m.SetRetryState(&types.RetryState{Attempt: 2, LastPrimaryOwner: string(LocusFinalizer),
				ActiveViolations: exploreOwnedRetryState().ActiveViolations})
			m.ResetForFallback(types.FallbackResetTargetFinalizer)
		}, want: true},
	}
	mut := types.NewMutableState("shared premise")
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable:       mut,
		EvidenceItems: []types.EvidenceItem{{ID: "ev", Source: "src.go", LineStart: 1}},
	}}
	for _, s := range steps {
		s.arm(mut)
		if s.name == "retained reason without veto" && (mut.IsInvestigationComplete() || mut.StableInvestigationCompleteReason() == "" || acceptedClosureHasActiveExploreContractBacktrack(mut)) {
			t.Fatal("fixture: the step must present flag=false, retained reason, no veto")
		}
		_, premise := o.acceptedClosurePremise()
		window := o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "")
		reconcile := o.acceptedClosureCanSatisfyReconcileEnoughFacts()
		if premise != s.want || window != s.want || reconcile != s.want {
			t.Fatalf("%s: premise=%t window=%t reconcile=%t, want all %t", s.name, premise, window, reconcile, s.want)
		}
	}
	// nil-safety of the shared premise.
	if _, ok := (*Orchestrator)(nil).acceptedClosurePremise(); ok {
		t.Fatal("nil orchestrator must not hold an accepted-closure premise")
	}
	if _, ok := (&Orchestrator{busCtx: &types.BusContext{}}).acceptedClosurePremise(); ok {
		t.Fatal("missing Mutable must not hold an accepted-closure premise")
	}
}
