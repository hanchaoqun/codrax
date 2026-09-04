package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runtime_accepted_closure_restamp_test.go — §40.43 round-six #0.
//
// The scheduler's repair site stashes the plan inside
// AdvanceRepairExecutionPlan (DispatchedOwner = CurrentOwner) and only THEN
// runs downgradeRuntimeAcceptedClosureExploreFallback, which may rewrite
// back_to_explore → finalizer_only. Before the fix nothing re-stashed the
// corrected owner, so computeClusterClosure billed the next failed round to
// explore although the finalizer ran: the never-attempted veto was defeated
// and, with pipeline_cluster_stable_budget 1, a fail-loud fired with the
// explore root cause NEVER dispatched (violating the "a never-dispatched
// root cause is always dispatched before fail-loud" red line).
//
// PIN (red on e02828718): the pure explore soft source-debt set goes through
// the REAL downgrade function (the orchestrator.go repair-site callee); the
// downgrade re-stamps the persisted plan's DispatchedOwner, the explore
// cluster's stability stays 0, the never-attempted veto holds, and the
// budget-1 config does NOT fail-loud.
func TestRuntimeAcceptedClosureDowngradeRestampsDispatchedOwner(t *testing.T) {
	restoreCluster := ClusterStableBudget()
	t.Cleanup(func() { SetClusterStableBudget(restoreCluster) })
	SetClusterStableBudget(1) // the finding's supported knob: must NOT fail-loud

	mut := types.NewMutableState("accepted runtime closure restamp")
	mut.SetInvestigationComplete("trace_query already answered the runtime-only root cause")
	mut.AppendDispatchToolResult(tier1TraceQueryRuntimeToolResult())
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioPerformanceBottleneck,
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
				CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
				Confidence:           0.9,
			},
		}},
	}}

	// Pure explore-owned soft source-debt set — the downgrade guard's exact
	// allowlist shape (facet:diagram_spine | root:answer_facet_coverage).
	violations := []types.Violation{{
		Kind:       types.ViolFacetUncovered,
		ClusterKey: types.FacetClusterKey(string(types.FacetDiagramSpine), "answer_facet_coverage"),
	}}

	// Round 1 — replay the scheduler's repair site in order: Advance stashes
	// the plan, then the real downgrade function rewrites the target.
	plan, target, preDowngrade := AdvanceRepairExecutionPlan(mut, violations, 0)
	if target != FallbackBackToExplore || preDowngrade != FallbackBackToExplore {
		t.Fatalf("fixture: Advance must pick back_to_explore, got %v/%v plan=%s", target, preDowngrade, SummarizeRepairExecutionPlan(plan))
	}
	if got := persistedRepairExecutionPlan(mut); got == nil || got.DispatchedOwner != LocusExplore {
		t.Fatalf("Advance stamps the dispatched owner on the stash: %+v", got)
	}
	target = o.downgradeRuntimeAcceptedClosureExploreFallback(target, violations)
	if target != FallbackFinalizerOnly {
		t.Fatalf("fixture: the runtime accepted-closure downgrade must fire, got %v", target)
	}
	if got := persistedRepairExecutionPlan(mut); got == nil || got.DispatchedOwner != LocusFinalizer || got.CurrentOwner != LocusExplore {
		t.Fatalf("the downgrade must re-stamp DispatchedOwner=finalizer while CurrentOwner keeps the plan semantics (deepest fresh owner): %+v", got)
	}
	populateRetryState(mut, contractResultOf(violations), 0)
	mut.ResetForFallback(types.FallbackResetTargetFinalizer)

	// Round 2 — the finalizer ran, the explore root did not: stability stays
	// 0 and the never-attempted veto holds, so even at stable budget 1 the
	// cluster exit never fail-louds. What this asserts is Advance's PRE-
	// downgrade pick (back_to_explore); §40.43 round-seven #1: in production
	// the repair site runs downgradeRuntimeAcceptedClosureExploreFallback
	// again on every round with the authority state unchanged
	// (ResetForFallback(Finalizer) clears only the answer document), so the
	// target is rewritten to finalizer_only again, DispatchedOwner is
	// re-stamped finalizer again, and the explore root is never dispatched
	// while the runtime-authority shape holds — the chain ends through the
	// P6 finalize repair hard cap (ships with the residual-concerns caveat),
	// not through the cluster exit. The stability outcome pinned here (0 /
	// veto holds / no cluster fail-loud at budget 1) is what keeps that loop
	// from fail-louding a root whose owner never ran.
	plan, target, _ = AdvanceRepairExecutionPlan(mut, violations, 0)
	if got := stableOfKind(plan, types.ViolFacetUncovered); got != 0 {
		t.Fatalf("the explore cluster's owner never ran, StableAttempts=%d want 0 (plan=%s)", got, SummarizeRepairExecutionPlan(plan))
	}
	if target != FallbackBackToExplore || plan.HasFailLoud {
		t.Fatalf("a never-dispatched explore root must never reach the cluster fail-loud exit (Advance's pre-downgrade pick stays back_to_explore), got %v fail_loud=%t plan=%s", target, plan.HasFailLoud, SummarizeRepairExecutionPlan(plan))
	}
}
