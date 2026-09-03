package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/types"
)

// accepted_closure_reconcile_backtrack_test.go — F14 (§40.14 V7-2 fold-in):
// the reconcile-node auto-complete is an accepted-closure consumer and must
// read the SAME premise as the explore-window auto-complete — including the
// explore-backtrack veto. Before this fold-in
// acceptedClosureCanSatisfyReconcileEnoughFacts accepted on the retained
// pre-backtrack reason without consulting the bound RetryState, and the
// scheduler runs autoCompleteReadyReconcileNodes BEFORE the veto-guarded
// explore-window checks, so a requeued reconcile node auto-completed from
// the stale closure while the backtrack was still in force.

func reconcileBacktrackFixture(t *testing.T) (*Orchestrator, *types.MutableState, *graphState, *types.TaskNode) {
	t.Helper()
	mut := types.NewMutableState("reconcile after explore backtrack")
	mut.SetInvestigationComplete("first accepted closure")
	productionExploreBacktrack(t, mut, exploreOwnedRetryState())
	if mut.StableInvestigationCompleteReason() == "" {
		t.Fatal("fixture: the retained pre-backtrack reason is what the reconcile arm used to accept on")
	}
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable:       mut,
		EvidenceItems: []types.EvidenceItem{{ID: "ev", Source: "src.go", LineStart: 1}},
	}}
	graph := types.TaskGraph{Nodes: []types.TaskNode{{
		ID:              "n_reconcile",
		Type:            types.NodeReconcile,
		EntryConditions: []types.Criterion{{Kind: types.CritHasEnoughFacts}},
	}}}
	state := newGraphState(graph)
	return o, mut, state, &state.graph.Nodes[0]
}

// red→green: with a bound backtrack (epoch stamped on the active RetryState,
// completion generation unchanged) a ready reconcile node is NOT
// auto-completed; after the explorer's fresh accepted completion (generation
// advanced) it is.
func TestReconcileAutoComplete_BoundExploreBacktrackVetoesUntilFreshCompletion(t *testing.T) {
	o, mut, state, reconcile := reconcileBacktrackFixture(t)
	if !acceptedClosureHasActiveExploreContractBacktrack(mut) {
		t.Fatal("fixture: the bound backtrack must be in force")
	}
	if o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		t.Fatal("fixture: the explore-window consumer honours the veto; the reconcile consumer must read the same premise")
	}

	if o.acceptedClosureCanSatisfyReconcileEnoughFacts() {
		t.Fatal("the retained pre-backtrack closure must not satisfy the reconcile node while the explore backtrack is in force")
	}
	if o.shouldAutoCompleteReadyReconcileNode(reconcile, criterion.Env{}) {
		t.Fatal("a ready reconcile node must not auto-complete from the stale closure while the backtrack veto holds")
	}
	remaining := o.autoCompleteReadyReconcileNodes(state, []*types.TaskNode{reconcile}, criterion.Env{})
	if len(remaining) != 1 || state.nodeStatus(reconcile.ID) != nodePending {
		t.Fatalf("the reconcile node must stay in the dispatch window (remaining=%d status=%v)", len(remaining), state.nodeStatus(reconcile.ID))
	}

	// The explorer re-decides completion → generation advances → the veto is
	// consumed and the reconcile node auto-completes from the FRESH closure.
	mut.SetInvestigationComplete("fresh completion after the backtrack")
	if acceptedClosureHasActiveExploreContractBacktrack(mut) {
		t.Fatal("fixture: the fresh completion must consume the veto")
	}
	if !o.acceptedClosureCanSatisfyReconcileEnoughFacts() || !o.shouldAutoCompleteReadyReconcileNode(reconcile, criterion.Env{}) {
		t.Fatal("after the fresh accepted completion the reconcile node must auto-complete")
	}
	remaining = o.autoCompleteReadyReconcileNodes(state, []*types.TaskNode{reconcile}, criterion.Env{})
	if len(remaining) != 0 || state.nodeStatus(reconcile.ID) != nodeDone {
		t.Fatalf("the reconcile node must be auto-completed once the fresh closure is in force (remaining=%d status=%v)", len(remaining), state.nodeStatus(reconcile.ID))
	}
}
