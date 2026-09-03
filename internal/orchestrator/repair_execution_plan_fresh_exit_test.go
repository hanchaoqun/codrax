package orchestrator

import (
	"fmt"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// repair_execution_plan_fresh_exit_test.go — §40.43 F-orch 三轮复核
// findings R and T(ii)/T(iii).
//
// R: the stuck-cluster fail-loud exit used to be evaluated on
// `closed := *prev` BEFORE the fresh rebuild, so its CurrentOwner /
// RemainingOwners operands were the previous round's. With a stuck
// finalizer cluster and a brand-new explore-owned root in the fresh set it
// fired fail-loud while the fresh rebuild (and the budget picker) said
// back_to_explore — a never-attempted root cause was never dispatched.
// Ruling: rebuild first (with carryClusterStability), then the exit reads
// only the FRESH carrier: its current-owner cluster has StableAttempts >=
// budget, it names no remaining owner, and no fresh cluster is at
// StableAttempts 0.

// PIN (red on 0139bca6b): rounds [finalizer block] ×2, round 3 adds a
// facet_uncovered root → back_to_explore (the picker's answer), never
// fail-loud; the plain stuck case (the same single cluster ×3) still
// fail-louds; a never-attempted sibling of the SAME owner also keeps the
// exit closed.
func TestAdvanceRepairExecutionPlan_StuckExitReadsFreshCarrier(t *testing.T) {
	restoreFin := FinalizerLocalRetryBudget()
	restoreCluster := ClusterStableBudget()
	t.Cleanup(func() {
		SetFinalizerLocalRetryBudget(restoreFin)
		SetClusterStableBudget(restoreCluster)
	})
	SetFinalizerLocalRetryBudget(2)
	SetClusterStableBudget(2)
	// finalizerLocalUsed at the budget: the fresh rebuild never downgrades
	// the deeper explore owner, exactly like the picker.
	const used = 2

	type round struct {
		fresh        []types.Violation
		want         FallbackTarget
		wantFailLoud bool
	}
	cases := []struct {
		name   string
		rounds []round
	}{
		{
			name: "finding R: new explore-owned root on failure 3 dispatches back_to_explore",
			rounds: []round{
				{fresh: []types.Violation{vBlock("summary")}, want: FallbackFinalizerOnly},
				{fresh: []types.Violation{vBlock("summary")}, want: FallbackFinalizerOnly},
				{fresh: []types.Violation{vBlock("summary"), vFacet("diagram_spine")}, want: FallbackBackToExplore},
			},
		},
		{
			name: "plain stuck cluster still fail-louds on failure budget+1",
			rounds: []round{
				{fresh: []types.Violation{vBlock("summary")}, want: FallbackFinalizerOnly},
				{fresh: []types.Violation{vBlock("summary")}, want: FallbackFinalizerOnly},
				{fresh: []types.Violation{vBlock("summary")}, want: FallbackFailLoud, wantFailLoud: true},
			},
		},
		{
			name: "never-attempted sibling of the same owner keeps the exit closed",
			rounds: []round{
				{fresh: []types.Violation{vBlock("summary")}, want: FallbackFinalizerOnly},
				{fresh: []types.Violation{vBlock("summary")}, want: FallbackFinalizerOnly},
				// Failure 3: the primary is stuck (stable 2) but the sibling is
				// a never-attempted root (stable 0) — it dispatches.
				{fresh: []types.Violation{vBlock("summary"), vBlock("other")}, want: FallbackFinalizerOnly},
				// Failure 4: every fresh cluster has been attempted (sibling at
				// 1, primary at 3) and the owner is still stuck — the exit fires.
				{fresh: []types.Violation{vBlock("summary"), vBlock("other")}, want: FallbackFailLoud, wantFailLoud: true},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mut := types.NewMutableState(tc.name)
			for i, r := range tc.rounds {
				plan, target, _ := AdvanceRepairExecutionPlan(mut, r.fresh, used)
				if target != r.want || plan.HasFailLoud != r.wantFailLoud {
					t.Fatalf("round %d: target=%v fail_loud=%t, want %v / %t — the stuck exit must read the FRESH rebuild; plan=%s",
						i+1, target, plan.HasFailLoud, r.want, r.wantFailLoud, SummarizeRepairExecutionPlan(plan))
				}
				if target != FallbackFailLoud {
					if want := FallbackTargetForViolationsWithBudget(r.fresh, used); target != want {
						t.Fatalf("round %d: Advance dispatches %v but the budget picker says %v", i+1, target, want)
					}
				}
				stashed, ok := mut.RepairExecutionPlan().(RepairExecutionPlan)
				if !ok || stashed.HasFailLoud != r.wantFailLoud {
					t.Fatalf("round %d: persisted plan fail_loud=%t (present=%t), want %t", i+1, stashed.HasFailLoud, ok, r.wantFailLoud)
				}
				if r.wantFailLoud {
					// The fail-loud plan IS the fresh rebuild with carried stability.
					if stashed.CurrentOwner != LocusFinalizer || len(stashed.RemainingOwners) != 0 || len(stashed.ClusterStates) != len(BuildRepairPlan(r.fresh).Clusters) {
						t.Fatalf("round %d: the fail-loud plan must be the fresh carrier, got %s", i+1, SummarizeRepairExecutionPlan(stashed))
					}
					continue
				}
				populateRetryState(mut, contractResultOf(r.fresh), i)
				mut.ResetForFallback(types.FallbackResetTargetFinalizer)
			}
		})
	}
}

// T(ii) PIN: carryClusterStability's W2.7 sibling-rotation carry-over. The
// previous cluster (BlockCoverageMissing on block "summary") is replaced in
// the fresh set by a retry-eligible kind it Implies on the SAME fingerprint
// (PrincipalClaimUseMissing on block "summary"): the closure counts the
// rotation as "still open" (StableAttempts 0→1) and the carry-over must land
// that count on the fresh cluster under its new kind — so the rotating
// cluster reaches the stuck exit on failure budget+1 like a plain stuck
// one. With the Implies branch of carryClusterStability disabled the fresh
// cluster restarts at 0 and never exits (every other test stayed green
// under that variant).
func TestCarryClusterStability_SiblingRotationCarriesOver(t *testing.T) {
	restoreCluster := ClusterStableBudget()
	t.Cleanup(func() { SetClusterStableBudget(restoreCluster) })
	SetClusterStableBudget(2)
	spec, ok := types.ViolKindSpecFor(types.ViolBlockCoverageMissing)
	implied := false
	for _, k := range spec.Implies {
		implied = implied || k == types.ViolPrincipalClaimUseMissing
	}
	if !ok || !implied {
		t.Fatalf("fixture: BlockCoverageMissing must imply PrincipalClaimUseMissing, got %+v", spec.Implies)
	}
	missing := types.Violation{Kind: types.ViolBlockCoverageMissing, Detail: `block id="summary" of required kind is missing`}
	rotated := vBlock("summary")
	if clusterFingerprintOf(missing) != clusterFingerprintOf(rotated) {
		t.Fatalf("fixture: both kinds must share the fingerprint, got %q vs %q", clusterFingerprintOf(missing), clusterFingerprintOf(rotated))
	}

	mut := types.NewMutableState("sibling rotation")
	plan, _, _ := AdvanceRepairExecutionPlan(mut, []types.Violation{missing}, 1<<30)
	if len(plan.ClusterStates) != 1 || plan.ClusterStates[0].StableAttempts != 0 {
		t.Fatalf("round 1 seeds the cluster at 0, got %+v", plan.ClusterStates)
	}
	plan, target, _ := AdvanceRepairExecutionPlan(mut, []types.Violation{rotated}, 1<<30)
	if target == FallbackFailLoud || len(plan.ClusterStates) != 1 {
		t.Fatalf("round 2 (rotation) must dispatch, got target=%v plan=%s", target, SummarizeRepairExecutionPlan(plan))
	}
	if got := plan.ClusterStates[0]; got.PrimaryKind != types.ViolPrincipalClaimUseMissing || got.StableAttempts != 1 {
		t.Fatalf("round 2: the rotated sibling must carry StableAttempts 1 under its new kind, got %+v", got)
	}
	plan, target, _ = AdvanceRepairExecutionPlan(mut, []types.Violation{rotated}, 1<<30)
	if target != FallbackFailLoud || !plan.HasFailLoud || plan.ClusterStates[0].StableAttempts != 2 {
		t.Fatalf("round 3: the rotating cluster must reach the stuck exit through the carried count, got target=%v plan=%s", target, SummarizeRepairExecutionPlan(plan))
	}
}

// T(iii) exhaustive census (the property the round-three author ran by
// hand, now pinned): for EVERY registered violation kind — as a singleton
// and paired with every finalizer-owned kind (same and distinct dispatch
// ids), with finalizerLocalUsed 0 and exhausted, and with a persisted plan
// whose EscalationAllowed is true or false — the target
// AdvanceRepairExecutionPlan dispatches equals
// FallbackTargetForViolationsWithBudget. A new kind (or a new guard in one
// picker only) that diverges is red here, naming the mix.
func TestAdvanceRepairExecutionPlan_DispatchAgreesWithBudgetPicker_ExhaustiveKindCensus(t *testing.T) {
	restoreFin := FinalizerLocalRetryBudget()
	restoreCluster := ClusterStableBudget()
	t.Cleanup(func() {
		SetFinalizerLocalRetryBudget(restoreFin)
		SetClusterStableBudget(restoreCluster)
	})
	SetFinalizerLocalRetryBudget(2)
	SetClusterStableBudget(4) // stability never reaches the exit inside this census

	kinds := types.AllViolationKinds()
	if len(kinds) < 60 {
		t.Fatalf("registry lists only %d kinds — the census lost its subject", len(kinds))
	}
	violationOf := func(kind types.ViolationKind, dispatch string) types.Violation {
		return types.Violation{Kind: kind, Detail: "census " + string(kind), DispatchID: dispatch}
	}
	var finalizerKinds []types.ViolationKind
	for _, k := range kinds {
		if LocusOfTarget(FallbackTargetForViolation(violationOf(k, "a"))) == LocusFinalizer {
			finalizerKinds = append(finalizerKinds, k)
		}
	}
	if len(finalizerKinds) < 10 {
		t.Fatalf("only %d finalizer-owned kinds — the census lost its subject", len(finalizerKinds))
	}
	type mix struct {
		name string
		vs   []types.Violation
	}
	var mixes []mix
	for _, k := range kinds {
		mixes = append(mixes, mix{name: string(k), vs: []types.Violation{violationOf(k, "a")}})
		for _, f := range finalizerKinds {
			if f == k {
				continue
			}
			mixes = append(mixes,
				mix{name: fmt.Sprintf("%s+%s/same-dispatch", k, f), vs: []types.Violation{violationOf(k, "a"), violationOf(f, "a")}},
				mix{name: fmt.Sprintf("%s+%s/distinct-dispatch", k, f), vs: []types.Violation{violationOf(k, "a"), violationOf(f, "b")}},
			)
		}
	}
	checked := 0
	for _, m := range mixes {
		for _, used := range []int{0, 1 << 30} {
			for _, escalation := range []bool{true, false} {
				mut := types.NewMutableState("census")
				prev := BuildRepairExecutionPlan(m.vs, used)
				prev.EscalationAllowed = escalation
				mut.SetRepairExecutionPlan(prev)
				plan, target, _ := AdvanceRepairExecutionPlan(mut, m.vs, used)
				want := FallbackTargetForViolationsWithBudget(m.vs, used)
				if target != want {
					t.Fatalf("mix %s used=%d escalation=%t: Advance dispatches %v but the budget picker says %v; plan=%s",
						m.name, used, escalation, target, want, SummarizeRepairExecutionPlan(plan))
				}
				checked++
			}
		}
	}
	if checked < 4*len(mixes) || len(mixes) < len(kinds) {
		t.Fatalf("census checked %d dispatches over %d mixes", checked, len(mixes))
	}
}
