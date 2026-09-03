// Package orchestrator — repair_execution_plan.go
//
// G1 (post_v2_runtime_gap_remediation, 2026-05-04). RepairExecutionPlan
// is the dispatch-ready execution view of a RepairPlan. Pre-G1 the
// orchestrator's retry-decision site consumed only RepairPlan.PrimaryOwner
// — a single owner, even when BuildRepairPlan identified multiple
// independent root-cause clusters.
//
// §40.43 R1 (fold-in round three, 2026-09-03) — the plan's two roles are
// now separated:
//
//   - DISPATCH: the target the orchestrator dispatches after a failed
//     finalize attempt is ALWAYS derived from a fresh rebuild of the
//     current actionable violation set with the current finalizer-local
//     budget counter (BuildRepairExecutionPlan). This is the pre-F12
//     production semantics and agrees with the legacy budget picker
//     FallbackTargetForViolationsWithBudget — which the scheduler's
//     yield-kill pre-check still reads — on every round (pinned by
//     TestAdvanceRepairExecutionPlan_DispatchAgreesWithBudgetPicker).
//     F12 (§40.43) made the persisted plan reach the next failure's
//     classification and thereby turned the never-before-live
//     promote / stay arms into dispatch targets: promote popped the queue
//     for an already-covered facet and stay froze the R2.2 downgrade
//     decision past its budget. Both arms are gone.
//
//   - STABILITY: the persisted plan (MutableState.RepairExecutionPlan)
//     carries per-cluster closure state across attempts. The next
//     failure's closure (repair_cluster_closure.go) increments
//     StableAttempts for every cluster whose Primary is still open, the
//     counts carry over the rebuild for every cluster whose
//     (PrimaryKind, PrimaryFingerprint) persists, and a stuck deepest
//     owner OF THE FRESH REBUILD (no shallower owner queued, escalation
//     allowed, no never-attempted cluster) exits through FallbackFailLoud
//     instead of cycling (§40.43 finding R: the exit is evaluated after
//     the rebuild, on the fresh carrier).
//
// Red-line invariants (must hold across every helper here):
//
//   - R3 (precise signals): every owner in OrderedOwners derives from
//     a RepairCluster.Owner — typed RepairLocus, no heuristic. The
//     stuck exit reads single integer comparisons on StableAttempts.
//
//   - R4 (no LLM-facing jargon): RepairExecutionPlan is internal
//     orchestrator routing state. It MUST NOT appear verbatim in any
//     LLM-facing string (skill prompt, retry hint summary, tool
//     description).
//
//   - L1 byte-identical: read-mode behaviour with write machinery absent
//     is unchanged; the retry-decision site is shared by both modes.

package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RepairExecutionPlan is the dispatch-ready execution view built on
// every failed finalize attempt and stashed on MutableState so the next
// attempt's closure detector can read the previous cluster states.
//
// Lifecycle:
//
//  1. Validator returns violations → AdvanceRepairExecutionPlan reads the
//     stashed previous plan (if any), computes the cluster closure
//     against the fresh set, rebuilds from the fresh set with the carried
//     StableAttempts, and exits through FallbackFailLoud when the FRESH
//     plan's deepest owner is stuck.
//  2. The resulting plan is stashed on MutableState. It PERSISTS across
//     ResetForFallback at every target (§40.43 F12); only ResetRetryState
//     (chain close at an accepted answer) clears it.
//  3. Orchestrator dispatches targetForLocus(CurrentOwner).
type RepairExecutionPlan struct {
	// Clusters is the partition BuildRepairPlan computed, preserved
	// verbatim for telemetry. Callers MUST treat as read-only.
	Clusters []RepairCluster

	// OrderedOwners is the deduplicated, deepest-first sequence of
	// distinct cluster owners. Length ≤ len(Clusters); duplicate
	// owners (two clusters owned by LocusFinalizer) collapse to one
	// entry — dispatching Finalizer once already addresses both
	// Finalizer-owned clusters since Finalizer-local fixes happen in
	// a single re-emit pass.
	OrderedOwners []RepairLocus

	// CurrentOwner is the locus the dispatch targets: OrderedOwners[0]
	// of the fresh rebuild.
	CurrentOwner RepairLocus

	// RemainingOwners is OrderedOwners[1:] — the shallower owners the
	// fresh set still names. It is telemetry plus one precise signal:
	// the stuck-cluster fail-loud exit fires only when it is empty
	// (a shallower owner still queued means the next rebuild can still
	// change the target once the finalizer-local budget is spent).
	RemainingOwners []RepairLocus

	// EscalationAllowed gates the "current owner stuck AND no remaining
	// owners → FallbackFailLoud" exit. Defaults to true; specific kinds
	// (e.g. SOFT-only telemetry) may flip via future hooks.
	EscalationAllowed bool

	// HasFailLoud preserves the legacy single-owner plan signal — a
	// LocusTerminal cluster forces the whole plan to fail_loud
	// regardless of remaining owners (CurrentOwner = LocusTerminal,
	// RemainingOwners empty). AdvanceRepairExecutionPlan also sets it on
	// the stuck-cluster exit.
	HasFailLoud bool

	// FinalizerLocalDowngradeApplied flags whether BuildRepairExecutionPlan
	// promoted LocusFinalizer to OrderedOwners[0] under the R2.2
	// budget rule (same cost-opt the legacy
	// FallbackTargetForViolationsWithBudget applies). Recomputed on every
	// rebuild from the current finalizerLocalUsed; surfaced for
	// telemetry and the preDowngrade return value.
	FinalizerLocalDowngradeApplied bool

	// ClusterStates is the per-cluster closure state, 1:1 with
	// Clusters at build time. v3 B1 (2026-05-04) — the cross-attempt
	// closure detector reads ClusterStates to count stable attempts and
	// detect the stuck owner. See repair_cluster_closure.go for
	// fingerprint extraction + closure rules.
	ClusterStates []RepairClusterExecutionState
}

// IsEmpty reports whether the plan carries no actionable owner. True
// when no clusters were produced (input violation set was empty).
func (p RepairExecutionPlan) IsEmpty() bool {
	return len(p.Clusters) == 0 && p.CurrentOwner == ""
}

// BuildRepairExecutionPlan groups violations into root-cause clusters
// (via BuildRepairPlan) and produces an execution plan whose
// OrderedOwners is the deduplicated deepest-first sequence of
// cluster owners. The R2.2 finalize-local downgrade rule (same as
// FallbackTargetForViolationsWithBudget, including the W3.5 fixable-by
// guard) is applied here so the legacy cost-opt semantic is preserved
// verbatim — when budget permits AND any cluster has Finalizer owner
// AND the natural deepest pick is deeper than Finalizer AND every
// violation with a declared FixableByAgents set lists the finalizer,
// Finalizer is promoted to OrderedOwners[0].
//
// finalizerLocalUsed is the per-Run counter of how many times the
// downgrade has fired so far; the caller increments it after each
// downgrade-eligible dispatch. When used >= budget, no downgrade.
func BuildRepairExecutionPlan(vs []types.Violation, finalizerLocalUsed int) RepairExecutionPlan {
	vs = FilterActionableRootViolations(vs)
	if len(vs) == 0 {
		// Empty input: no clusters, nothing to dispatch. Caller
		// (orchestrator) treats CurrentOwner == "" as "no plan".
		return RepairExecutionPlan{
			EscalationAllowed: true,
		}
	}

	plan := BuildRepairPlan(vs)

	if plan.HasFailLoud {
		return RepairExecutionPlan{
			Clusters:          plan.Clusters,
			CurrentOwner:      LocusTerminal,
			HasFailLoud:       true,
			EscalationAllowed: true,
		}
	}

	// Dedupe-preserving-order over plan.Clusters. plan.Clusters is
	// already deepest-first (sortClustersDeepestFirst runs inside
	// BuildRepairPlan), so a simple seen-map preserves the
	// deepest-first invariant. SOFT-Terminal clusters are skipped
	// exactly as BuildRepairPlan skips them for PrimaryOwner (Phase
	// 1-C): they never route the dispatch, so the rebuild and the
	// legacy picker read the same owner sequence.
	seen := make(map[RepairLocus]bool, len(plan.Clusters))
	ordered := make([]RepairLocus, 0, len(plan.Clusters))
	for _, c := range plan.Clusters {
		if c.Owner == "" {
			continue
		}
		if c.Owner == LocusTerminal && isSoftViolationKind(c.Primary.Kind) {
			continue
		}
		if seen[c.Owner] {
			continue
		}
		seen[c.Owner] = true
		ordered = append(ordered, c.Owner)
	}

	if len(ordered) == 0 {
		// Defensive — clusters with empty Owner shouldn't occur but
		// fall back to LocusFinalizer (the safe no-op) so the
		// orchestrator always has a target.
		ordered = append(ordered, LocusFinalizer)
	}

	// R2.2 finalize-local downgrade. Mirrors the legacy
	// FallbackTargetForViolationsWithBudget logic. When budget permits
	// AND OrderedOwners[0] is deeper than Finalizer AND Finalizer is
	// somewhere in the queue AND (W3.5) the finalizer can fix every
	// violation that declares a fixable-by set, promote Finalizer to
	// OrderedOwners[0] so the cheap re-emit gets up to budget attempts
	// before paying for upstream rework.
	downgradeApplied := false
	if budget := FinalizerLocalRetryBudget(); budget > 0 && finalizerLocalUsed < budget {
		if !seen[LocusFinalizer] {
			// no-op — Finalizer not in queue
		} else if ordered[0] != LocusFinalizer && violationsFixableByAgent(vs, types.AgentFinalizer) {
			ordered = promoteFinalizerLocal(ordered)
			downgradeApplied = true
		}
	}

	out := RepairExecutionPlan{
		Clusters:                       plan.Clusters,
		OrderedOwners:                  ordered,
		CurrentOwner:                   ordered[0],
		EscalationAllowed:              true,
		FinalizerLocalDowngradeApplied: downgradeApplied,
	}
	if len(ordered) > 1 {
		out.RemainingOwners = append([]RepairLocus(nil), ordered[1:]...)
	}
	// v3 B1: seed ClusterStates 1:1 with plan.Clusters so the next
	// attempt's closure detector can compare per-cluster state
	// across retries.
	if len(plan.Clusters) > 0 {
		out.ClusterStates = make([]RepairClusterExecutionState, 0, len(plan.Clusters))
		for _, c := range plan.Clusters {
			out.ClusterStates = append(out.ClusterStates, ClusterStateForCluster(c))
		}
	}
	return out
}

// promoteFinalizerLocal moves LocusFinalizer to ordered[0] preserving
// the relative order of the rest. Pre-condition: LocusFinalizer is
// in ordered AND ordered[0] != LocusFinalizer.
func promoteFinalizerLocal(ordered []RepairLocus) []RepairLocus {
	out := make([]RepairLocus, 0, len(ordered))
	out = append(out, LocusFinalizer)
	for _, l := range ordered {
		if l == LocusFinalizer {
			continue
		}
		out = append(out, l)
	}
	return out
}

// AdvanceRepairExecutionPlan is the orchestrator-side entry point that
// reads the previously-stashed plan from MutableState, computes the
// cluster closure of the fresh violations against it, and returns the
// plan to persist plus the FallbackTarget the orchestrator dispatches.
//
// §40.43 R1: the persisted plan is used ONLY for cluster stability
// accounting and the stuck-cluster fail-loud exit; the dispatch target is
// always a fresh rebuild (BuildRepairExecutionPlan) of the current
// actionable violations with the current finalizerLocalUsed, so it
// agrees with FallbackTargetForViolationsWithBudget on every round.
//
// Behaviour (§40.43 F-orch 三轮复核 finding R: rebuild FIRST, then the
// stuck exit reads the FRESH carrier — never the previous round's
// CurrentOwner / RemainingOwners):
//
//   - Empty fresh violations: returns FallbackFinalizerOnly + empty
//     plan; no MutableState write.
//   - Rebuild from fresh with the current finalizerLocalUsed; carry the
//     closure-updated StableAttempts / DerivedResolved of every previous
//     cluster whose key persists (carryClusterStability).
//   - The fresh plan is stuck (stuckClusterExit: a cluster owned by the
//     FRESH CurrentOwner reached ClusterStableBudget(), the fresh plan
//     names no remaining owner, escalation allowed, and no fresh cluster
//     is at StableAttempts 0 — a never-attempted root always dispatches):
//     the fresh plan is stashed with HasFailLoud=true; returns
//     FallbackFailLoud.
//   - Otherwise: stashes the plan and returns
//     targetForLocus(plan.CurrentOwner) — or FallbackFailLoud when the
//     rebuild itself has a LocusTerminal cluster.
//
// preDowngrade is the locus the legacy budget-less picker WOULD
// have chosen — used by the orchestrator's R2.2 telemetry to detect
// "downgrade fired" events.
//
// nil MutableState is tolerated for tests / direct callers — the
// plan is built fresh every call (no persistence).
func AdvanceRepairExecutionPlan(mut *types.MutableState, fresh []types.Violation, finalizerLocalUsed int) (RepairExecutionPlan, FallbackTarget, FallbackTarget) {
	fresh = FilterActionableRootViolations(fresh)
	if len(fresh) == 0 {
		return RepairExecutionPlan{EscalationAllowed: true},
			FallbackFinalizerOnly, FallbackFinalizerOnly
	}

	var carried []RepairClusterExecutionState
	if prev := persistedRepairExecutionPlan(mut); prev != nil && !prev.IsEmpty() && !prev.HasFailLoud && len(prev.ClusterStates) > 0 {
		carried = computeClusterClosure(*prev, fresh)
	}

	plan := BuildRepairExecutionPlan(fresh, finalizerLocalUsed)
	carryClusterStability(&plan, carried)
	if !plan.HasFailLoud && stuckClusterExit(plan, ClusterStableBudget()) {
		plan.HasFailLoud = true
	}

	if mut != nil {
		mut.SetRepairExecutionPlan(plan)
	}

	if plan.HasFailLoud {
		return plan, FallbackFailLoud, FallbackFailLoud
	}

	target := targetForLocus(plan.CurrentOwner)

	// Compute preDowngrade — the FallbackTarget the legacy budget-
	// less picker would have chosen. When the budget downgrade fired
	// in this build, preDowngrade is the deepest cluster owner (i.e.
	// the locus we WOULD have picked without the cost-opt). When no
	// downgrade fired, preDowngrade == target.
	preDowngrade := target
	if plan.FinalizerLocalDowngradeApplied {
		// The deepest cluster owner is the second entry of OrderedOwners
		// (Finalizer was promoted to [0]; the natural deepest pick is
		// at [1] when downgrade applied).
		if len(plan.OrderedOwners) >= 2 {
			preDowngrade = targetForLocus(plan.OrderedOwners[1])
		}
	}
	return plan, target, preDowngrade
}

// persistedRepairExecutionPlan reads the stashed plan from MutableState.
// Returns nil when nothing is stashed (fresh chain) or the stashed value
// is not a RepairExecutionPlan.
func persistedRepairExecutionPlan(mut *types.MutableState) *RepairExecutionPlan {
	if mut == nil {
		return nil
	}
	raw := mut.RepairExecutionPlan()
	if raw == nil {
		return nil
	}
	p, ok := raw.(RepairExecutionPlan)
	if !ok {
		return nil
	}
	return &p
}

// stuckClusterExit is the cluster-closure fail-loud exit predicate over
// the FRESH rebuilt plan after carryClusterStability (§40.43 finding R):
// a cluster owned by the fresh CurrentOwner reached the stable budget
// without resolving its Primary, the fresh plan names no shallower owner,
// escalation is allowed, and no fresh cluster is at StableAttempts 0 — a
// never-attempted root cause always dispatches. Every operand is a
// single integer or boolean (precise signals). With a shallower owner
// still queued the fresh rebuild keeps deciding the target (the
// finalizer-local budget escalates it upstream); only the last owner
// exits here.
func stuckClusterExit(plan RepairExecutionPlan, stableBudget int) bool {
	return currentOwnerStuck(plan, stableBudget) &&
		len(plan.RemainingOwners) == 0 &&
		plan.EscalationAllowed &&
		!anyClusterNeverAttempted(plan)
}

// anyClusterNeverAttempted reports whether the plan carries a cluster at
// StableAttempts 0 — a root cause the chain has not dispatched for yet.
func anyClusterNeverAttempted(plan RepairExecutionPlan) bool {
	for _, st := range plan.ClusterStates {
		if st.StableAttempts == 0 {
			return true
		}
	}
	return false
}

// carryClusterStability copies the closure-updated StableAttempts /
// DerivedResolved of the previous plan's clusters onto the freshly
// rebuilt plan for every cluster whose identity persists:
//
//   - exact (PrimaryKind, PrimaryFingerprint) match, or
//   - same PrimaryFingerprint with the previous PrimaryKind declaring the
//     fresh PrimaryKind in its Implies set (W2.7 sibling rotation — the
//     closure already counted that rotation as "still open").
//
// Each previous cluster is consumed at most once. Clusters the previous
// plan never saw start at zero (a new root cause), and a cluster whose
// key was absent from the previous round restarts at zero when it
// reappears — stability is per persisting key, never per locus.
func carryClusterStability(plan *RepairExecutionPlan, carried []RepairClusterExecutionState) {
	if plan == nil || len(carried) == 0 || len(plan.ClusterStates) == 0 {
		return
	}
	consumed := make([]bool, len(carried))
	match := func(st RepairClusterExecutionState) int {
		for i, prev := range carried {
			if !consumed[i] && prev.PrimaryKind == st.PrimaryKind && prev.PrimaryFingerprint == st.PrimaryFingerprint {
				return i
			}
		}
		for i, prev := range carried {
			if consumed[i] || prev.PrimaryFingerprint != st.PrimaryFingerprint {
				continue
			}
			if primaryImpliesSiblingsOnFp(prev.PrimaryKind, map[types.ViolationKind]bool{st.PrimaryKind: true}) {
				return i
			}
		}
		return -1
	}
	for i := range plan.ClusterStates {
		j := match(plan.ClusterStates[i])
		if j < 0 {
			continue
		}
		consumed[j] = true
		plan.ClusterStates[i].StableAttempts = carried[j].StableAttempts
		plan.ClusterStates[i].DerivedResolved = carried[j].DerivedResolved
	}
}

// ── B1 v3 stable-budget knob ─────────────────────────────────────

// clusterStableBudget is the per-cluster cap on how many attempts
// the same owner may run without resolving its Primary before the
// closure detector escalates. Default 2 — matches the pre-v3
// finalizerLocalRetryBudget intuition (give the owner two shots; if
// no progress, advance). NOTE: at the default caps the finalize
// repair hard cap (P6, 2) pre-empts this exit on the third failure —
// see clusterClosureExitReachability in finalize_repair_hard_cap.go.
//
// Wired via cmd/root.go from yaml `pipeline_cluster_stable_budget`.
// Tests may override directly via SetClusterStableBudget.
var clusterStableBudget = 2

// ClusterStableBudget exposes the current budget for the closure
// detector.
func ClusterStableBudget() int { return clusterStableBudget }

// SetClusterStableBudget overrides the budget. Non-positive values
// reset to default (2). Call from cmd/root.go after merging
// codrax.yaml.
func SetClusterStableBudget(n int) {
	if n <= 0 {
		clusterStableBudget = 2
		return
	}
	clusterStableBudget = n
}

// SummarizeRepairExecutionPlan returns a one-line telemetry summary
// for trace emission. Format mirrors SummarizeRepairPlan but adds
// queue + per-cluster closure state. INTERNAL telemetry — never
// rendered into LLM-facing strings.
//
//	"current=<locus> ordered=[o1,o2,...] remaining=N
//	 closed=<bool> stuck=<bool> stable_max=<int>
//	 budget_downgrade=<bool> fail_loud=<bool> clusters=[(...)]"
//
// closed reports whether all current-owner clusters have both
// Primary AND Derived resolved on the latest closure update;
// stuck reports whether any current-owner cluster has reached
// the stable-attempt budget without resolving. stable_max is the
// max StableAttempts across current-owner clusters.
func SummarizeRepairExecutionPlan(plan RepairExecutionPlan) string {
	if plan.IsEmpty() {
		return "current=<empty> ordered=[] remaining=0 clusters=[]"
	}
	owners := make([]string, 0, len(plan.OrderedOwners))
	for _, o := range plan.OrderedOwners {
		owners = append(owners, string(o))
	}
	closed := currentOwnerClustersAllClosed(plan)
	stuck := currentOwnerStuck(plan, ClusterStableBudget())
	stableMax := 0
	for _, st := range plan.ClusterStates {
		if st.Owner != plan.CurrentOwner {
			continue
		}
		if st.StableAttempts > stableMax {
			stableMax = st.StableAttempts
		}
	}
	return fmt.Sprintf(
		"current=%s ordered=[%s] remaining=%d closed=%t stuck=%t stable_max=%d budget_downgrade=%t fail_loud=%t clusters=%s",
		plan.CurrentOwner,
		strings.Join(owners, ","),
		len(plan.RemainingOwners),
		closed,
		stuck,
		stableMax,
		plan.FinalizerLocalDowngradeApplied,
		plan.HasFailLoud,
		SummarizeClusterStates(plan.ClusterStates),
	)
}
