package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/types"
)

// §40.14 V7-2: the accepted-closure explore-backtrack veto reads a
// cross-generation carrier. A RetryState vetoes an accepted closure only
// while it is bound to the CURRENT explore-backtrack epoch AND the
// explore window has not produced a new completion decision in that
// epoch. Unbound states (finalizer-only / extract fallbacks, downgraded
// explore owners) never veto; one backtrack vetoes exactly once.

func exploreOwnedRetryState() *types.RetryState {
	return &types.RetryState{
		Attempt:          1,
		LastPrimaryOwner: string(LocusExplore),
		ActiveViolations: []types.ScoredViolation{{
			Kind:     types.ViolRequiredDiagramEdgeAbsent,
			Severity: types.SeverityHigh,
			Layer:    "v2_oracle",
		}},
	}
}

// productionExploreBacktrack replays the FallbackBackToExplore ordering of
// the scheduler: the retry state is populated first, then the explore
// reset binds it, then the completion latch is reset for the re-opened
// window (pendingCompletionReset).
func productionExploreBacktrack(t *testing.T, mut *types.MutableState, rs *types.RetryState) {
	t.Helper()
	mut.SetRetryState(rs)
	mut.ResetForFallback(types.FallbackResetTargetExplore)
	mut.ResetInvestigationComplete()
	if got := mut.RetryState(); got == nil || got.ExploreBacktrackEpoch != mut.ExploreBacktrackEpoch() {
		t.Fatalf("fixture: retry state must be bound to the opened epoch %d, got %+v", mut.ExploreBacktrackEpoch(), got)
	}
}

// ① red→green: after a finalize failure backtracked to explore, the
// explorer's FRESH completion decision must not be vetoed by the retry
// state left behind by the backtrack.
func TestShouldAutoCompleteExploreWindowFromAcceptedClosure_FreshCompletionAfterExploreBacktrackLiftsVeto(t *testing.T) {
	mut := types.NewMutableState("fresh completion after explore backtrack")
	mut.SetInvestigationComplete("first accepted closure")
	productionExploreBacktrack(t, mut, exploreOwnedRetryState())
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	// Sanity: the veto is armed until the model re-decides.
	if !acceptedClosureHasActiveExploreContractBacktrack(mut) {
		t.Fatal("fixture: the bound backtrack must veto before the fresh completion")
	}

	mut.SetInvestigationComplete("fresh completion after the backtrack")

	if acceptedClosureHasActiveExploreContractBacktrack(mut) {
		t.Fatal("a completion decided AFTER the backtrack lifts the stale veto; the arm must respect the model's fresh decision")
	}
	if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("remaining explore nodes must auto-complete from the fresh accepted closure instead of being re-dispatched")
	}
}

// ① (populate path): the same lift holds when the retry state comes from
// the production populator rather than a hand-built value.
func TestShouldAutoCompleteExploreWindowFromAcceptedClosure_FreshCompletionLiftsPopulatedVeto(t *testing.T) {
	mut := types.NewMutableState("populated explore backtrack")
	mut.SetInvestigationComplete("first accepted closure")
	populateRetryState(mut, contract.Result{Violations: []contract.Violation{{
		Kind: types.ViolRequiredDiagramEdgeAbsent, Detail: "required diagram edge absent",
	}}}, 0)
	rs := mut.RetryState()
	if rs == nil || rs.LastPrimaryOwner != string(types.LocusExplore) || len(rs.ActiveViolations) == 0 {
		t.Fatalf("fixture: populateRetryState must yield an explore-owned plan, got %+v", rs)
	}
	mut.ResetForFallback(types.FallbackResetTargetExplore)
	mut.ResetInvestigationComplete()
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}
	if !acceptedClosureHasActiveExploreContractBacktrack(mut) {
		t.Fatal("fixture: populated + bound backtrack must veto until re-decided")
	}

	mut.SetInvestigationComplete("fresh completion")

	if acceptedClosureHasActiveExploreContractBacktrack(mut) || !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("fresh completion must lift the populated backtrack veto")
	}
}

// ② same-generation veto holds: before the re-decision the retained
// pre-backtrack reason must not auto-complete; a second backtrack after
// a completion re-arms the veto until the next completion.
func TestShouldAutoCompleteExploreWindowFromAcceptedClosure_SameGenerationVetoHoldsAndRearms(t *testing.T) {
	mut := types.NewMutableState("same generation veto")
	mut.SetInvestigationComplete("first accepted closure")
	productionExploreBacktrack(t, mut, exploreOwnedRetryState())
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	if mut.StableInvestigationCompleteReason() == "" {
		t.Fatal("fixture: the retained pre-backtrack reason is what the veto must hold against")
	}
	if !acceptedClosureHasActiveExploreContractBacktrack(mut) {
		t.Fatal("bound backtrack in the current epoch must veto")
	}
	if o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("the retained pre-backtrack closure must not auto-complete the re-opened window before the model re-decides")
	}

	// Model re-decides → veto consumed.
	mut.SetInvestigationComplete("fresh completion")
	if acceptedClosureHasActiveExploreContractBacktrack(mut) {
		t.Fatal("re-decision must consume the veto")
	}

	// Second finalize failure → second backtrack re-populates + re-binds.
	productionExploreBacktrack(t, mut, &types.RetryState{
		Attempt:          2,
		LastPrimaryOwner: string(LocusExplore),
		ActiveViolations: exploreOwnedRetryState().ActiveViolations,
	})
	if mut.ExploreBacktrackEpoch() != 2 {
		t.Fatalf("second backtrack must open epoch 2, got %d", mut.ExploreBacktrackEpoch())
	}
	if !acceptedClosureHasActiveExploreContractBacktrack(mut) || o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("a second backtrack must re-arm the veto until the next completion")
	}
	mut.SetInvestigationComplete("second fresh completion")
	if acceptedClosureHasActiveExploreContractBacktrack(mut) || !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("the next completion must consume the re-armed veto")
	}
}

// ③ unbound never vetoes: an explore-owned retry state that no explore
// backtrack bound (epoch 0 — finalizer-only / extract fallbacks, or a
// downgraded explore owner) leaves the accepted closure in charge.
func TestShouldAutoCompleteExploreWindowFromAcceptedClosure_UnboundExploreRetryStateNeverVetoes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target types.FallbackResetTarget
	}{
		{name: "no fallback reset"},
		{name: "finalizer fallback", target: types.FallbackResetTargetFinalizer},
		{name: "extract fallback", target: types.FallbackResetTargetExtract},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mut := types.NewMutableState(tc.name)
			mut.SetInvestigationComplete("accepted closure")
			mut.SetRetryState(exploreOwnedRetryState())
			if tc.target != "" {
				mut.ResetForFallback(tc.target)
			}
			if got := mut.RetryState(); got == nil || got.ExploreBacktrackEpoch != 0 {
				t.Fatalf("fixture: retry state must stay unbound, got %+v", got)
			}
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

			if acceptedClosureHasActiveExploreContractBacktrack(mut) {
				t.Fatal("an unbound explore-owned retry state never re-opened exploration and must not veto")
			}
			if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
				t.Fatal("accepted closure must stay in charge when no explore backtrack bound the retry state")
			}
		})
	}
}

// ③' a bound state from an EARLIER epoch (stale binding after a later
// backtrack re-opened without a retry state) does not veto either.
// 复核 note: a bound-but-stale epoch cannot arise from production writers
// today (the only bind site re-stamps the live carrier on every explore
// backtrack); this pins the DEFENSIVE single-integer comparison so a future
// second writer cannot make a stale binding veto silently.
func TestAcceptedClosureHasActiveExploreContractBacktrack_StaleEpochDoesNotVeto(t *testing.T) {
	mut := types.NewMutableState("stale epoch")
	mut.SetInvestigationComplete("accepted closure")
	mut.SetRetryState(&types.RetryState{
		Attempt:                         1,
		LastPrimaryOwner:                string(LocusExplore),
		ActiveViolations:                exploreOwnedRetryState().ActiveViolations,
		ExploreBacktrackEpoch:           1,
		CompletionGenerationAtBacktrack: 1,
	})
	// Live epoch is 0 here: the carrier claims an epoch that was never
	// opened on this state — precise mismatch, no veto.
	if acceptedClosureHasActiveExploreContractBacktrack(mut) {
		t.Fatal("a retry state bound to a different epoch than the live one must not veto")
	}
}

// Production reset pair: closing the finalize retry chain at an accepted
// answer clears the carrier so nothing downstream reads a shipped chain's
// retry state.
func TestCloseFinalizeRetryChain_ClearsRetryStateAndPlan(t *testing.T) {
	mut := types.NewMutableState("close chain")
	mut.SetRetryState(exploreOwnedRetryState())
	mut.SetRepairExecutionPlan(struct{ x int }{1})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	o.closeFinalizeRetryChain()

	if mut.RetryState() != nil || mut.RepairExecutionPlan() != nil {
		t.Fatal("closing the finalize retry chain must clear the retry state and its paired execution plan")
	}
	// nil-safe on an orchestrator without a bus.
	(&Orchestrator{}).closeFinalizeRetryChain()
}
