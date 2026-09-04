package orchestrator

import (
	"fmt"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// repair_execution_plan_exact_carry_owner_test.go — §40.43 round-seven #0.
//
// carryClusterStability's exact-(PrimaryKind, PrimaryFingerprint) lane had no
// owner guard (only the Implies lane got one in round six), so an owner flip
// on the SAME kind + SAME cluster key inherited the previous owner's
// StableAttempts. Real producer: contract_check_block stamps
// required_diagram_edge_absent RepairLocusOverride=LocusExplore while no typed
// relation authority exists (round 1 dispatches back_to_explore — the kind is
// FixableByAgents=[explorer], so the R2.2 downgrade cannot fire); once the
// explorer supplied relation evidence the finalizer prompt compiler sets
// FinalizerTypedRelationRecipeAvailable and assignRequiredDiagramEdgeRepairOwner
// re-stamps the identical (kind, ClusterKey) to LocusFinalizer. Before the fix
// the fresh finalizer-owned cluster started at the explore cluster's count:
// at pipeline_cluster_stable_budget=1 round 2 fail-louded with the finalizer
// NEVER the dispatched owner for that root (red line: a never-dispatched root
// cause is always dispatched before fail-loud); at the default budget 2 the
// finalizer was denied one of its two attempts. The same flip is produced by
// the semantic-quality reviewer choosing a different repair_locus for one
// topic cluster key across rounds.
//
// Ruling: the owner guard applies to BOTH carry lanes — a cross-owner exact
// match starts at 0; a same-owner exact match keeps the count.
//
// EVOLUTION RECORD (red on 79ca2f98b via a scratch baseline copy of
// repair_execution_plan.go under go test -overlay): budget 1 → round 2
// `target=fail_loud ... (owner=finalizer ... stable=1)`; budget 2 → round 2
// finalizer cluster stable=1. Green after the exact lane gained
// `prev.Owner == st.Owner`.

// requiredDiagramEdgeAbsentViolation mirrors the contract_check_block producer
// (same kind, ClusterKey, SuspectedRoot and the round-1 LocusExplore stamp).
func requiredDiagramEdgeAbsentViolation() types.Violation {
	return types.Violation{
		Kind: types.ViolRequiredDiagramEdgeAbsent,
		Detail: fmt.Sprintf(
			"required diagram block id=%q has zero structural Mermaid edges while its typed relation contract requires at least one",
			"d1"),
		Repair:     types.FlowOperationEvidenceEmissionGuide,
		ClusterKey: blockClusterKey("d1", "required_diagram_edges"),
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "diagram_edges",
			Reason:     "required relation diagram has no structural edge",
			Confidence: 1,
		},
		Stage:               string(types.StageFinalize),
		RepairLocusOverride: types.LocusExplore,
	}
}

func TestCarryClusterStability_ExactMatchIsOwnerAttributed(t *testing.T) {
	restoreCluster := ClusterStableBudget()
	restoreFin := FinalizerLocalRetryBudget()
	t.Cleanup(func() {
		SetClusterStableBudget(restoreCluster)
		SetFinalizerLocalRetryBudget(restoreFin)
	})
	SetFinalizerLocalRetryBudget(2)

	// The fixture is the REAL owner flip: round 1 carries the producer's
	// LocusExplore stamp; round 2 runs the same violation through
	// assignRequiredDiagramEdgeRepairOwner once the typed relation recipe
	// receipt is set, which re-stamps LocusFinalizer on the identical
	// (kind, ClusterKey).
	flipped := func(mut *types.MutableState) []types.Violation {
		mut.SetFinalizerTypedRelationRecipeAvailable(true)
		vs := assignRequiredDiagramEdgeRepairOwner([]types.Violation{requiredDiagramEdgeAbsentViolation()}, &types.BusContext{Mutable: mut})
		if vs[0].RepairLocusOverride != types.LocusFinalizer || vs[0].ClusterKey != requiredDiagramEdgeAbsentViolation().ClusterKey {
			t.Fatalf("fixture: the producer must re-stamp the same cluster key to LocusFinalizer, got %+v", vs[0])
		}
		return vs
	}

	for _, budget := range []int{1, 2} {
		t.Run(fmt.Sprintf("explore→finalizer flip on the same (kind, cluster key) starts at 0 at budget %d", budget), func(t *testing.T) {
			SetClusterStableBudget(budget)
			mut := types.NewMutableState("exact carry owner flip")
			round1 := []types.Violation{requiredDiagramEdgeAbsentViolation()}
			plan, target, _ := AdvanceRepairExecutionPlan(mut, round1, 0)
			if target != FallbackBackToExplore || len(plan.ClusterStates) != 1 || plan.ClusterStates[0].Owner != LocusExplore {
				t.Fatalf("fixture: round 1 dispatches the explore-owned root, got %v plan=%s", target, SummarizeRepairExecutionPlan(plan))
			}
			fp := plan.ClusterStates[0].PrimaryFingerprint
			populateRetryState(mut, contractResultOf(round1), 0)
			mut.ResetForFallback(types.FallbackResetTargetExplore)
			mut.ResetInvestigationComplete()

			// Round 2: the explorer ran and the same root persists, now
			// finalizer-owned. The finalizer never dispatched for this root.
			round2 := flipped(mut)
			plan, target, _ = AdvanceRepairExecutionPlan(mut, round2, 0)
			if len(plan.ClusterStates) != 1 || plan.ClusterStates[0].PrimaryFingerprint != fp || plan.ClusterStates[0].Owner != LocusFinalizer {
				t.Fatalf("fixture: round 2 is the same cluster key under the finalizer, got plan=%s", SummarizeRepairExecutionPlan(plan))
			}
			if target != FallbackFinalizerOnly || plan.HasFailLoud {
				t.Fatalf("round 2: the finalizer never dispatched for this root — it must be dispatched, got %v fail_loud=%t plan=%s", target, plan.HasFailLoud, SummarizeRepairExecutionPlan(plan))
			}
			if got := stableOfKind(plan, types.ViolRequiredDiagramEdgeAbsent); got != 0 {
				t.Fatalf("round 2: a cross-owner exact match starts at 0 (the explore count must not transfer), got %d plan=%s", got, SummarizeRepairExecutionPlan(plan))
			}
			populateRetryState(mut, contractResultOf(round2), 1)
			mut.ResetForFallback(types.FallbackResetTargetFinalizer)

			// Round 3+: the finalizer is now the dispatched owner; the SAME-owner
			// exact carry keeps counting and the stuck exit fires only after
			// the finalizer itself ran `budget` times for the root.
			for round := 3; ; round++ {
				fresh := flipped(mut)
				plan, target, _ = AdvanceRepairExecutionPlan(mut, fresh, 0)
				finalizerRuns := round - 2
				if finalizerRuns < budget {
					if target != FallbackFinalizerOnly || stableOfKind(plan, types.ViolRequiredDiagramEdgeAbsent) != finalizerRuns {
						t.Fatalf("round %d: same-owner exact carry must count the finalizer's %d run(s), got target=%v plan=%s", round, finalizerRuns, target, SummarizeRepairExecutionPlan(plan))
					}
					populateRetryState(mut, contractResultOf(fresh), round-1)
					mut.ResetForFallback(types.FallbackResetTargetFinalizer)
					continue
				}
				if target != FallbackFailLoud || !plan.HasFailLoud || stableOfKind(plan, types.ViolRequiredDiagramEdgeAbsent) != budget {
					t.Fatalf("round %d: the finalizer ran %d time(s) without progress — stuck exit expected, got %v plan=%s", round, finalizerRuns, target, SummarizeRepairExecutionPlan(plan))
				}
				break
			}
		})
	}

	t.Run("same-owner exact match keeps the count", func(t *testing.T) {
		SetClusterStableBudget(2)
		mut := types.NewMutableState("exact carry same owner")
		fresh := []types.Violation{requiredDiagramEdgeAbsentViolation()}
		for round := 1; round <= 2; round++ {
			plan, target, _ := AdvanceRepairExecutionPlan(mut, fresh, 0)
			if target != FallbackBackToExplore || stableOfKind(plan, types.ViolRequiredDiagramEdgeAbsent) != round-1 {
				t.Fatalf("round %d: the explore owner ran %d time(s), got target=%v plan=%s", round, round-1, target, SummarizeRepairExecutionPlan(plan))
			}
			populateRetryState(mut, contractResultOf(fresh), round-1)
			mut.ResetForFallback(types.FallbackResetTargetExplore)
			mut.ResetInvestigationComplete()
		}
	})
}
