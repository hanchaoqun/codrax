package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/types"
)

// repair_execution_plan_dispatch_test.go — §40.43 R1 (fold-in round three).
//
// F12 made the persisted RepairExecutionPlan reach the next failure's
// classification, which turned the never-before-live promote / stay arms
// into DISPATCH targets: promote popped RemainingOwners[0] without checking
// that owner's clusters were still open (an already-covered facet was sent
// back to explore), and stay copied FinalizerLocalDowngradeApplied verbatim
// so the R2.2 finalizer-local budget was consulted only at build time.
//
// Ruling: the persisted plan is used ONLY for cluster stability accounting
// and the stuck-cluster fail-loud exit; the dispatch target is always a
// fresh rebuild of the current actionable violations with the current
// finalizerLocalUsed — the pre-F12 production semantics, which agree with
// the legacy budget picker (FallbackTargetForViolationsWithBudget, still
// used by the yield-kill pre-check) on every round. StableAttempts carry
// over across rebuilds for clusters whose (kind, fingerprint) persists so
// stability keeps counting.

// dispatchRound is one failed finalize attempt: the fresh actionable
// violation set the validator returned.
type dispatchRound struct {
	fresh []types.Violation
	// want is the expected target for this round (empty = only the
	// picker-agreement property is asserted).
	want FallbackTarget
	// wantUsedAfter, when >= 0, pins finalizerLocalUsed after the round.
	wantUsedAfter int
	// wantBilled, when non-nil, pins whether the per-kind retry ledger is
	// billed for this round (shouldBillKindRetryLedger on the pair
	// AdvanceRepairExecutionPlan returned).
	wantBilled *bool
}

func boolPtr(b bool) *bool { return &b }

func contractResultOf(vs []types.Violation) contract.Result {
	return contract.Result{Violations: vs}
}

func vExtractAnchor(block string) types.Violation {
	return types.Violation{Kind: types.ViolSubjectAnchorMissing, Detail: `principal block "` + block + `" has no subject anchor`}
}

func vFinalizerContradiction(block string) types.Violation {
	return types.Violation{Kind: types.ViolSelfContradiction, Detail: `block id="` + block + `" contradicts itself`}
}

// (1) property pin: over a table of violation mixes × budgets, the target
// AdvanceRepairExecutionPlan dispatches equals
// FallbackTargetForViolationsWithBudget(fresh, finalizerLocalUsed) on every
// round whose result is not FailLoud. Red on 0d9a142e4: the promote arm
// dispatched back_to_explore for an already-covered facet, the stay arm
// kept finalizer_only past the R2.2 budget, and the W3.5 fixable-by rule
// was applied by the picker but not by the rebuild. The downgraded-rounds
// shape (finding U) is censused exhaustively in
// repair_execution_plan_owner_attributed_test.go.
func TestAdvanceRepairExecutionPlan_DispatchAgreesWithBudgetPicker(t *testing.T) {
	threeOwners := []types.Violation{vFinalizerContradiction("summary"), vExtractAnchor("summary"), vFacet("diagram_spine")}
	finAndExtract := []types.Violation{vFinalizerContradiction("summary"), vExtractAnchor("summary")}
	finAndExplore := []types.Violation{vFinalizerContradiction("summary"), vFacet("diagram_spine")}
	cases := []struct {
		name          string
		finBudget     int
		clusterBudget int
		rounds        []dispatchRound
	}{
		{
			// Finding A: plan [finalizer, explore, extract]; round 2 only the
			// extract residual remains. The promote arm popped the explore
			// owner for an already-covered facet; the fresh rebuild says
			// back_to_extract.
			name:      "three owners, round-2-only-extract residual",
			finBudget: 2, clusterBudget: 2,
			rounds: []dispatchRound{
				{fresh: threeOwners, want: FallbackFinalizerOnly, wantUsedAfter: 1, wantBilled: boolPtr(false)},
				{fresh: []types.Violation{vExtractAnchor("summary")}, want: FallbackBackToExtract, wantUsedAfter: 1, wantBilled: boolPtr(true)},
			},
		},
		{
			// Finding B: finalizer-local budget 1. Round 2 must escalate to
			// back_to_extract with finalizerLocalUsed still 1 and the kind
			// ledger billed (the stay arm kept dispatching finalizer_only,
			// used 3/1, and shouldBillKindRetryLedger stayed false).
			name:      "finalizer budget 1 escalates on round 2",
			finBudget: 1, clusterBudget: 2,
			rounds: []dispatchRound{
				{fresh: finAndExtract, want: FallbackFinalizerOnly, wantUsedAfter: 1, wantBilled: boolPtr(false)},
				{fresh: finAndExtract, want: FallbackBackToExtract, wantUsedAfter: 1, wantBilled: boolPtr(true)},
				{fresh: finAndExtract, want: FallbackBackToExtract, wantUsedAfter: 1, wantBilled: boolPtr(true)},
			},
		},
		{
			// Finding B (reverse mis-exemption): finalizer budget 3, cluster
			// budget 2 — the yield-kill pre-check (legacy picker) and Advance
			// must agree on every round, including the rounds after the
			// finalizer cluster's stable budget is spent.
			name:      "finalizer budget 3 / cluster budget 2 agree with the yield pre-check",
			finBudget: 3, clusterBudget: 2,
			rounds: []dispatchRound{
				{fresh: finAndExplore, want: FallbackFinalizerOnly, wantUsedAfter: 1},
				{fresh: finAndExplore, want: FallbackFinalizerOnly, wantUsedAfter: 2},
				{fresh: finAndExplore, want: FallbackFinalizerOnly, wantUsedAfter: 3},
				{fresh: finAndExplore, want: FallbackBackToExplore, wantUsedAfter: 3},
				{fresh: finAndExplore, want: FallbackBackToExplore, wantUsedAfter: 3},
			},
		},
		{
			// W3.5: a kind only the explorer can fix must not be downgraded to
			// a finalizer_only rewrite even when the budget permits.
			name:      "W3.5 fixable-by rule",
			finBudget: 2, clusterBudget: 2,
			rounds: []dispatchRound{
				{fresh: []types.Violation{
					{Kind: types.ViolRequiredDiagramEdgeAbsent, Detail: "required diagram edge absent", DispatchID: "a"},
					{Kind: types.ViolMustInclude, Detail: "must include sentinel", DispatchID: "b"},
				}, want: FallbackBackToExplore, wantUsedAfter: 0, wantBilled: boolPtr(true)},
			},
		},
		{
			name:      "same dispatch id, mixed owners",
			finBudget: 2, clusterBudget: 2,
			rounds: []dispatchRound{
				{fresh: []types.Violation{
					{Kind: types.ViolSelfContradiction, Detail: "a", DispatchID: "d"},
					{Kind: types.ViolSubjectAnchorMissing, Detail: `principal block "x" has no subject anchor`, DispatchID: "d"},
				}, want: FallbackFinalizerOnly, wantUsedAfter: 1},
				{fresh: []types.Violation{
					{Kind: types.ViolSubjectAnchorMissing, Detail: `principal block "x" has no subject anchor`, DispatchID: "d"},
				}, want: FallbackBackToExtract, wantUsedAfter: 1},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restoreFin := FinalizerLocalRetryBudget()
			restoreCluster := ClusterStableBudget()
			t.Cleanup(func() {
				SetFinalizerLocalRetryBudget(restoreFin)
				SetClusterStableBudget(restoreCluster)
			})
			SetFinalizerLocalRetryBudget(tc.finBudget)
			SetClusterStableBudget(tc.clusterBudget)

			mut := types.NewMutableState(tc.name)
			used := 0
			for i, round := range tc.rounds {
				usedBefore := used
				plan, target, preDowngrade := AdvanceRepairExecutionPlan(mut, round.fresh, used)
				if target == FallbackFinalizerOnly && preDowngrade != FallbackFinalizerOnly {
					used++
				}
				if target != FallbackFailLoud {
					if want := FallbackTargetForViolationsWithBudget(round.fresh, usedBefore); target != want {
						t.Fatalf("round %d: Advance dispatches %v but the budget picker says %v (used=%d) — the dispatch target must be a fresh rebuild that agrees with the legacy picker; plan=%s",
							i+1, target, want, usedBefore, SummarizeRepairExecutionPlan(plan))
					}
				}
				if round.want != "" && target != round.want {
					t.Fatalf("round %d: target=%v, want %v; plan=%s", i+1, target, round.want, SummarizeRepairExecutionPlan(plan))
				}
				if round.wantUsedAfter >= 0 && used != round.wantUsedAfter {
					t.Fatalf("round %d: finalizerLocalUsed=%d after the round, want %d", i+1, used, round.wantUsedAfter)
				}
				if round.wantBilled != nil {
					if got := shouldBillKindRetryLedger(target, preDowngrade); got != *round.wantBilled {
						t.Fatalf("round %d: kind ledger billed=%t, want %t (target=%v preDowngrade=%v)", i+1, got, *round.wantBilled, target, preDowngrade)
					}
				}
				// The persisted plan is the current rebuild plus carried
				// stability — it must never be a stale promote/stay copy:
				// its CurrentOwner is the dispatched target's locus.
				stashed, ok := mut.RepairExecutionPlan().(RepairExecutionPlan)
				if !ok {
					t.Fatalf("round %d: plan not persisted (%T)", i+1, mut.RepairExecutionPlan())
				}
				if target != FallbackFailLoud && targetForLocus(stashed.CurrentOwner) != target {
					t.Fatalf("round %d: persisted CurrentOwner=%v does not match the dispatched target %v", i+1, stashed.CurrentOwner, target)
				}
				// Replay the scheduler's post-Advance state sequence.
				populateRetryState(mut, contractResultOf(round.fresh), i)
				switch target {
				case FallbackFinalizerOnly:
					mut.ResetForFallback(types.FallbackResetTargetFinalizer)
				case FallbackBackToExtract:
					mut.ResetForFallback(types.FallbackResetTargetExtract)
				case FallbackBackToExplore:
					mut.ResetForFallback(types.FallbackResetTargetExplore)
					mut.ResetInvestigationComplete()
				}
			}
		})
	}
}

// (2) stability pin: StableAttempts carry over across fresh rebuilds for a
// cluster whose (kind, fingerprint) persists — the count follows the cluster
// key across the rebuild, and it advances only in rounds whose dispatched
// owner is that cluster's owner.
//
// EVOLUTION RECORD (§40.43 F-orch 四轮复核 finding U): the original pin
// asserted BOTH clusters at round-1 on every round ("the finalizer-owned
// cluster keeps counting while the dispatched owner changes") — that was the
// unattributed count which let a root whose owner never ran reach the stuck
// exit. Owner-attributed: round 1 dispatches finalizer (finalizer 0 → 1 on
// round 2, extract stays 0), rounds 2+ dispatch extract (extract 0 → 1 on
// round 3, finalizer stays 1).
func TestAdvanceRepairExecutionPlan_StableAttemptsCarryOverAcrossRebuilds(t *testing.T) {
	restoreFin := FinalizerLocalRetryBudget()
	restoreCluster := ClusterStableBudget()
	t.Cleanup(func() {
		SetFinalizerLocalRetryBudget(restoreFin)
		SetClusterStableBudget(restoreCluster)
	})
	SetFinalizerLocalRetryBudget(1)
	SetClusterStableBudget(4)

	mut := types.NewMutableState("carry over")
	fresh := []types.Violation{vFinalizerContradiction("summary"), vExtractAnchor("summary")}
	used := 0
	stableOf := func(plan RepairExecutionPlan, owner RepairLocus) int {
		for _, st := range plan.ClusterStates {
			if st.Owner == owner {
				return st.StableAttempts
			}
		}
		t.Fatalf("no cluster owned by %v in %s", owner, SummarizeRepairExecutionPlan(plan))
		return -1
	}
	for round := 1; round <= 3; round++ {
		plan, target, preDowngrade := AdvanceRepairExecutionPlan(mut, fresh, used)
		if target == FallbackFinalizerOnly && preDowngrade != FallbackFinalizerOnly {
			used++
		}
		wantTarget := FallbackFinalizerOnly
		if round > 1 {
			wantTarget = FallbackBackToExtract
		}
		if target != wantTarget {
			t.Fatalf("round %d: target=%v, want %v", round, target, wantTarget)
		}
		// Owner-attributed: the finalizer ran in round 1 only; extract ran
		// in rounds 2+.
		wantFinalizer, wantExtract := 0, 0
		if round >= 2 {
			wantFinalizer = 1
			wantExtract = round - 2
		}
		if got := stableOf(plan, LocusFinalizer); got != wantFinalizer {
			t.Fatalf("round %d: finalizer cluster StableAttempts=%d, want %d — the count carries over the rebuild by cluster key and advances only when its owner ran", round, got, wantFinalizer)
		}
		if got := stableOf(plan, LocusExtract); got != wantExtract {
			t.Fatalf("round %d: extract cluster StableAttempts=%d, want %d", round, got, wantExtract)
		}
		populateRetryState(mut, contractResultOf(fresh), round-1)
		if target == FallbackFinalizerOnly {
			mut.ResetForFallback(types.FallbackResetTargetFinalizer)
		} else {
			mut.ResetForFallback(types.FallbackResetTargetExtract)
		}
	}

	// A cluster whose key disappears from fresh restarts at zero when it
	// reappears: stability is per persisting key, never per locus.
	AdvanceRepairExecutionPlan(mut, []types.Violation{vFacet("diagram_spine")}, used)
	plan, _, _ := AdvanceRepairExecutionPlan(mut, fresh, used)
	if got := stableOf(plan, LocusFinalizer); got != 0 {
		t.Fatalf("a cluster key absent from the previous round restarts at 0, got %d", got)
	}
}

// (2b) the stuck exit reads the carried count: a single stuck cluster
// reaches FailLoud on attempt budget+1 for every owner locus (kept from
// TestRepairExecutionPlan_StuckClusterReachesFailLoudAcrossResets), and a
// multi-owner plan never fail-louds through the cluster exit while a
// shallower owner remains — it keeps agreeing with the picker instead.
func TestAdvanceRepairExecutionPlan_StuckExitRequiresEmptyQueue(t *testing.T) {
	restoreCluster := ClusterStableBudget()
	t.Cleanup(func() { SetClusterStableBudget(restoreCluster) })
	SetClusterStableBudget(2)

	mut := types.NewMutableState("stuck with queue")
	fresh := []types.Violation{vFinalizerContradiction("summary"), vFacet("diagram_spine")}
	for round := 1; round <= 5; round++ {
		_, target, _ := AdvanceRepairExecutionPlan(mut, fresh, 1<<30)
		if target == FallbackFailLoud {
			t.Fatalf("round %d: a stuck deepest owner with a shallower owner still queued must not fail-loud through the cluster exit", round)
		}
		if want := FallbackTargetForViolationsWithBudget(fresh, 1<<30); target != want {
			t.Fatalf("round %d: target=%v, want %v", round, target, want)
		}
		populateRetryState(mut, contractResultOf(fresh), round-1)
		mut.ResetForFallback(types.FallbackResetTargetExplore)
		mut.ResetInvestigationComplete()
	}
}
