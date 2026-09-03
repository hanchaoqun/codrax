package orchestrator

import (
	"fmt"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// repair_execution_plan_owner_attributed_test.go — §40.43 F-orch 四轮复核
// finding U.
//
// computeClusterClosure used to advance StableAttempts for EVERY persisting
// cluster regardless of which owner the round dispatched, carryClusterStability
// copied that count, and the fresh-carrier stuck exit's only never-attempted
// operand is StableAttempts == 0. With fresh {must_include (finalizer),
// facet_uncovered (explore)} and FinalizerLocalRetryBudget 2, rounds 1–2 are
// R2.2-downgraded finalizer_only; the finalizer fixes must_include; on round
// 3 the explore cluster stood at 2 although the explorer was NEVER dispatched
// for that root, and Advance returned FallbackFailLoud while the budget
// picker said back_to_explore.
//
// Ruling: stability is owner-attributed — a cluster's StableAttempts advances
// only in a round whose dispatched owner (prev.CurrentOwner, the target
// actually dispatched last round after the R2.2 downgrade) equals that
// cluster's owner; clusters whose owner did not run keep their count, so a
// never-dispatched root stays at 0 and is always dispatched.

func vMustInclude() types.Violation {
	return types.Violation{Kind: types.ViolMustInclude, Detail: "must include sentinel"}
}

// stableOfKind returns the StableAttempts of the cluster whose Primary has
// the given kind; -1 when absent.
func stableOfKind(plan RepairExecutionPlan, kind types.ViolationKind) int {
	for _, st := range plan.ClusterStates {
		if st.PrimaryKind == kind {
			return st.StableAttempts
		}
	}
	return -1
}

// PIN (red on 64ceb5b06): the finding's scenario dispatches back_to_explore
// on round 3 with the explore cluster still at 0; the plain same-owner stuck
// chain still fail-louds on failure budget+1; the W2.7 rotation carry-over
// counts only for the owner that ran.
func TestAdvanceRepairExecutionPlan_StabilityIsOwnerAttributed(t *testing.T) {
	restoreFin := FinalizerLocalRetryBudget()
	restoreCluster := ClusterStableBudget()
	t.Cleanup(func() {
		SetFinalizerLocalRetryBudget(restoreFin)
		SetClusterStableBudget(restoreCluster)
	})
	SetFinalizerLocalRetryBudget(2)
	SetClusterStableBudget(2)

	t.Run("finding U: two downgraded finalizer rounds never attempt the explore root", func(t *testing.T) {
		mut := types.NewMutableState("owner attributed")
		pair := []types.Violation{vMustInclude(), vFacet("diagram_spine")}
		used := 0
		advance := func(round int, fresh []types.Violation) (RepairExecutionPlan, FallbackTarget) {
			t.Helper()
			usedBefore := used
			plan, target, preDowngrade := AdvanceRepairExecutionPlan(mut, fresh, used)
			if target == FallbackFinalizerOnly && preDowngrade != FallbackFinalizerOnly {
				used++
			}
			if target != FallbackFailLoud {
				if want := FallbackTargetForViolationsWithBudget(fresh, usedBefore); target != want {
					t.Fatalf("round %d: Advance dispatches %v but the budget picker says %v; plan=%s", round, target, want, SummarizeRepairExecutionPlan(plan))
				}
			}
			populateRetryState(mut, contractResultOf(fresh), round-1)
			mut.ResetForFallback(types.FallbackResetTargetFinalizer)
			return plan, target
		}
		plan, target := advance(1, pair)
		if target != FallbackFinalizerOnly || !plan.FinalizerLocalDowngradeApplied {
			t.Fatalf("fixture: round 1 must be the R2.2-downgraded finalizer_only, got %v plan=%s", target, SummarizeRepairExecutionPlan(plan))
		}
		plan, target = advance(2, pair)
		if target != FallbackFinalizerOnly || !plan.FinalizerLocalDowngradeApplied {
			t.Fatalf("fixture: round 2 must be the R2.2-downgraded finalizer_only, got %v", target)
		}
		if got := stableOfKind(plan, types.ViolMustInclude); got != 1 {
			t.Fatalf("round 2: the finalizer cluster ran once, StableAttempts=%d want 1", got)
		}
		if got := stableOfKind(plan, types.ViolFacetUncovered); got != 0 {
			t.Fatalf("round 2: the explore cluster's owner never ran, StableAttempts=%d want 0", got)
		}
		// Round 3: the finalizer fixed must_include; the residual root is
		// explore-owned and has never been dispatched.
		plan, target = advance(3, []types.Violation{vFacet("diagram_spine")})
		if target != FallbackBackToExplore || plan.HasFailLoud {
			t.Fatalf("round 3: a never-dispatched explore root must be dispatched (back_to_explore), got %v fail_loud=%t plan=%s",
				target, plan.HasFailLoud, SummarizeRepairExecutionPlan(plan))
		}
		if got := stableOfKind(plan, types.ViolFacetUncovered); got != 0 {
			t.Fatalf("round 3: the explore cluster must still be at 0, got %d", got)
		}
		// Rounds 4–5: now the explorer runs and fails to clear it — the
		// owner-attributed count advances and the plain stuck exit fires
		// on the explorer's third attempt.
		plan, target = advance(4, []types.Violation{vFacet("diagram_spine")})
		if target != FallbackBackToExplore || stableOfKind(plan, types.ViolFacetUncovered) != 1 {
			t.Fatalf("round 4: explore ran once, got target=%v plan=%s", target, SummarizeRepairExecutionPlan(plan))
		}
		plan, target = advance(5, []types.Violation{vFacet("diagram_spine")})
		if target != FallbackFailLoud || !plan.HasFailLoud || stableOfKind(plan, types.ViolFacetUncovered) != 2 {
			t.Fatalf("round 5: the explorer ran twice without progress — stuck exit, got target=%v plan=%s", target, SummarizeRepairExecutionPlan(plan))
		}
	})

	t.Run("plain same-owner stuck chain still fail-louds on failure budget+1", func(t *testing.T) {
		mut := types.NewMutableState("plain stuck")
		for round := 1; round <= 3; round++ {
			plan, target, _ := AdvanceRepairExecutionPlan(mut, []types.Violation{vBlock("summary")}, 1<<30)
			if round < 3 {
				if target != FallbackFinalizerOnly || stableOfKind(plan, types.ViolPrincipalClaimUseMissing) != round-1 {
					t.Fatalf("round %d: target=%v plan=%s", round, target, SummarizeRepairExecutionPlan(plan))
				}
			} else if target != FallbackFailLoud || !plan.HasFailLoud {
				t.Fatalf("round 3: the dispatched owner ran twice — stuck exit expected, got %v", target)
			}
			populateRetryState(mut, contractResultOf([]types.Violation{vBlock("summary")}), round-1)
			mut.ResetForFallback(types.FallbackResetTargetFinalizer)
		}
	})

	t.Run("W2.7 rotation carries only for the owner that ran", func(t *testing.T) {
		// Distinct dispatch ids keep the finalizer-owned block cluster and the
		// explore-owned facet cluster apart (same-dispatch violations are
		// clustered by the cooccurrence rules).
		missing := types.Violation{Kind: types.ViolBlockCoverageMissing, Detail: `block id="summary" of required kind is missing`, DispatchID: "a"}
		rotated := vBlock("summary")
		rotated.DispatchID = "a"
		facet := vFacet("diagram_spine")
		facet.DispatchID = "b"
		mut := types.NewMutableState("rotation attribution")
		// used at the budget: the explore owner is dispatched every round.
		plan, target, _ := AdvanceRepairExecutionPlan(mut, []types.Violation{missing, facet}, 1<<30)
		if target != FallbackBackToExplore || len(plan.ClusterStates) != 2 {
			t.Fatalf("fixture: round 1 dispatches explore over two clusters, got %v plan=%s", target, SummarizeRepairExecutionPlan(plan))
		}
		populateRetryState(mut, contractResultOf([]types.Violation{missing, facet}), 0)
		mut.ResetForFallback(types.FallbackResetTargetExplore)
		mut.ResetInvestigationComplete()
		plan, target, _ = AdvanceRepairExecutionPlan(mut, []types.Violation{rotated, facet}, 1<<30)
		if target != FallbackBackToExplore {
			t.Fatalf("round 2 dispatches explore, got %v", target)
		}
		if got := stableOfKind(plan, types.ViolPrincipalClaimUseMissing); got != 0 {
			t.Fatalf("the finalizer-owned cluster rotated while the explorer ran: its count must stay 0, got %d (plan=%s)", got, SummarizeRepairExecutionPlan(plan))
		}
		if got := stableOfKind(plan, types.ViolFacetUncovered); got != 1 {
			t.Fatalf("the explore-owned cluster's owner ran once, got %d", got)
		}
		// Same rotation under the finalizer as dispatched owner still
		// carries (TestCarryClusterStability_SiblingRotationCarriesOver).
		single := types.NewMutableState("rotation same owner")
		AdvanceRepairExecutionPlan(single, []types.Violation{missing}, 1<<30)
		plan, _, _ = AdvanceRepairExecutionPlan(single, []types.Violation{rotated}, 1<<30)
		if got := stableOfKind(plan, types.ViolPrincipalClaimUseMissing); got != 1 {
			t.Fatalf("same-owner rotation must carry 1, got %d", got)
		}
	})
}

// Exhaustive downgraded-rounds census (finding U, extending the T(iii)
// census whose persisted plan was always built at the same budget as the
// round under test): for every non-finalizer-owned kind paired with every
// finalizer-owned kind (same and distinct dispatch ids), replay the R2.2
// shape — rounds 1–2 at FinalizerLocalRetryBudget 2 (downgraded when the
// pair permits), round 3 with the finalizer-owned kind cleared. On every
// round the target must equal the budget picker's unless the residual
// root's owner has itself been dispatched ClusterStableBudget() times (the
// legitimate stuck exit). Red on 64ceb5b06 for every downgraded pair: the
// residual root fail-louded with an owner that never ran.
func TestAdvanceRepairExecutionPlan_DispatchAgreesWithBudgetPicker_DowngradedRoundsCensus(t *testing.T) {
	restoreFin := FinalizerLocalRetryBudget()
	restoreCluster := ClusterStableBudget()
	t.Cleanup(func() {
		SetFinalizerLocalRetryBudget(restoreFin)
		SetClusterStableBudget(restoreCluster)
	})
	SetFinalizerLocalRetryBudget(2)
	SetClusterStableBudget(2)

	kinds := types.AllViolationKinds()
	if len(kinds) < 60 {
		t.Fatalf("registry lists only %d kinds — the census lost its subject", len(kinds))
	}
	violationOf := func(kind types.ViolationKind, dispatch string) types.Violation {
		return types.Violation{Kind: kind, Detail: "census " + string(kind), DispatchID: dispatch}
	}
	ownerOf := func(v types.Violation) RepairLocus { return LocusOfTarget(FallbackTargetForViolation(v)) }
	var finalizerKinds, otherKinds []types.ViolationKind
	for _, k := range kinds {
		if ownerOf(violationOf(k, "a")) == LocusFinalizer {
			finalizerKinds = append(finalizerKinds, k)
		} else {
			otherKinds = append(otherKinds, k)
		}
	}
	if len(finalizerKinds) < 10 || len(otherKinds) < 5 {
		t.Fatalf("census lost its subject: %d finalizer-owned / %d other kinds", len(finalizerKinds), len(otherKinds))
	}
	checked, downgraded := 0, 0
	for _, k := range otherKinds {
		for _, f := range finalizerKinds {
			for _, dispatch := range []string{"a", "b"} {
				residual := violationOf(k, "a")
				pair := []types.Violation{residual, violationOf(f, dispatch)}
				name := fmt.Sprintf("%s+%s/%s", k, f, map[string]string{"a": "same-dispatch", "b": "distinct-dispatch"}[dispatch])
				mut := types.NewMutableState(name)
				used := 0
				residualOwner := ownerOf(residual)
				residualRuns := 0
				rounds := []struct {
					fresh []types.Violation
				}{{pair}, {pair}, {[]types.Violation{residual}}}
				for i, r := range rounds {
					usedBefore := used
					plan, target, preDowngrade := AdvanceRepairExecutionPlan(mut, r.fresh, used)
					if target == FallbackFinalizerOnly && preDowngrade != FallbackFinalizerOnly {
						used++
						if i == 0 {
							downgraded++
						}
					}
					want := FallbackTargetForViolationsWithBudget(r.fresh, usedBefore)
					if target == FallbackFailLoud {
						if plan.HasFailLoud && BuildRepairPlan(r.fresh).HasFailLoud {
							break // a terminal kind: both pickers say fail-loud
						}
						if residualRuns < ClusterStableBudget() {
							t.Fatalf("mix %s round %d: Advance fail-louds while the residual root's owner %v ran only %d time(s) (picker says %v); plan=%s",
								name, i+1, residualOwner, residualRuns, want, SummarizeRepairExecutionPlan(plan))
						}
					} else if target != want {
						t.Fatalf("mix %s round %d (used=%d): Advance dispatches %v but the budget picker says %v; plan=%s",
							name, i+1, usedBefore, target, want, SummarizeRepairExecutionPlan(plan))
					}
					if LocusOfTarget(target) == residualOwner {
						residualRuns++
					}
					checked++
					populateRetryState(mut, contractResultOf(r.fresh), i)
					switch target {
					case FallbackBackToExplore:
						mut.ResetForFallback(types.FallbackResetTargetExplore)
						mut.ResetInvestigationComplete()
					case FallbackBackToExtract:
						mut.ResetForFallback(types.FallbackResetTargetExtract)
					default:
						mut.ResetForFallback(types.FallbackResetTargetFinalizer)
					}
				}
			}
		}
	}
	if checked < 2*len(otherKinds)*len(finalizerKinds) || downgraded == 0 {
		t.Fatalf("census checked %d dispatches, %d downgraded pairs — the downgraded-rounds shape must be exercised", checked, downgraded)
	}
}
