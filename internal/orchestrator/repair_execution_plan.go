// Package orchestrator — repair_execution_plan.go
//
// G1 (post_v2_runtime_gap_remediation, 2026-05-04). RepairExecutionPlan
// is the dispatch-ready execution view of a RepairPlan. Pre-G1 the
// orchestrator's retry-decision site consumed only RepairPlan.PrimaryOwner
// — a single owner, even when BuildRepairPlan identified multiple
// independent root-cause clusters. The shallower clusters then had to
// wait for a future retry to re-cluster their own root cause from
// scratch.
//
// G1 model: RepairExecutionPlan persists the deduplicated, deepest-
// first ORDERED queue of cluster owners across retry attempts.
// CurrentOwner drives the next dispatch; RemainingOwners is the queue
// of yet-to-execute owners. PromoteNextOwner consumes the queue when
// a retry produces no fresh violations (i.e. the previous owner
// closed its cluster) — the next-shallower owner activates without
// re-running BuildRepairPlan from scratch.
//
// Red-line invariants (must hold across every helper here):
//
//   - R3 (precise signals): every owner in OrderedOwners derives from
//     a RepairCluster.Owner — typed RepairLocus, no heuristic. The
//     queue is built ONCE per fresh violation set; per-retry queue
//     advances are pure pointer arithmetic.
//
//   - R4 (no LLM-facing jargon): RepairExecutionPlan is internal
//     orchestrator routing state. It MUST NOT appear verbatim in any
//     LLM-facing string (skill prompt, retry hint summary, tool
//     description). The retry hint render layer surfaces effects (the
//     current owner's stage-agnostic meaning) without naming the
//     queue.
//
//   - L1 byte-identical: this file adds NEW symbols only. Callers
//     that use BuildRepairPlan / FallbackTargetForViolations* remain
//     valid; G1 step 3 wires the orchestrator retry-decision site to
//     consume the execution plan, behind shouldRebuildExecutionPlan
//     so the legacy path stays exercised when no plan has been
//     stashed yet.

package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RepairExecutionPlan is the dispatch-ready execution view built once
// per fresh validator failure. The orchestrator stashes it on
// MutableState (G1 step 2) so subsequent retries can advance through
// OrderedOwners without re-clustering on every attempt.
//
// Lifecycle:
//
//  1. Validator returns violations → BuildRepairExecutionPlan(vs, budget).
//  2. Plan stashed on MutableState (next commit).
//  3. Orchestrator dispatches CurrentOwner via targetForLocus.
//  4. After agent completes:
//     - Fresh violation set whose ViolationKind set differs from
//     prev.Clusters' kind set → BuildRepairExecutionPlan rebuilds
//     (the shallower owner's fix exposed a new root cause).
//     - Fresh violation set whose kind set is a subset of prev →
//     PromoteNextOwner pops RemainingOwners[0] into CurrentOwner.
//     - No fresh violations → success, plan discarded.
//  5. RemainingOwners empty AND oracles still failing →
//     EscalationAllowed gates the upper-layer FailLoud decision.
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

	// CurrentOwner is the locus the next dispatch targets. Equals
	// OrderedOwners[0] right after Build; advances via
	// PromoteNextOwner.
	CurrentOwner RepairLocus

	// RemainingOwners is OrderedOwners[1:] at Build time; shrinks as
	// PromoteNextOwner consumes them.
	RemainingOwners []RepairLocus

	// EscalationAllowed gates the "after exhausting RemainingOwners
	// AND OwnerStableAttempts >= budget, escalate to FailLoud" path.
	// Defaults to true; specific kinds (e.g. SOFT-only telemetry)
	// may flip via future hooks.
	EscalationAllowed bool

	// HasFailLoud preserves the legacy single-owner plan signal — a
	// LocusTerminal cluster forces the whole plan to fail_loud
	// regardless of remaining owners. CurrentOwner is set to
	// LocusTerminal in that case and RemainingOwners stays empty.
	HasFailLoud bool

	// FinalizerLocalDowngradeApplied flags whether BuildRepairExecutionPlan
	// promoted LocusFinalizer to OrderedOwners[0] under the R2.2
	// budget rule (same cost-opt the legacy
	// FallbackTargetForViolationsWithBudget applied). Surfaced for
	// telemetry only.
	FinalizerLocalDowngradeApplied bool

	// ClusterStates is the per-cluster closure state, 1:1 with
	// Clusters at build time. v3 B1 (2026-05-04) — the cross-attempt
	// closure detector reads ClusterStates to decide rebuild /
	// promote / same-owner-stable++. See repair_cluster_closure.go
	// for fingerprint extraction + decision rules. Older callers
	// that examine RepairExecutionPlan but ignore this field continue
	// to function unchanged (it is additive).
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
// FallbackTargetForViolationsWithBudget) is applied here so the
// legacy cost-opt semantic is preserved verbatim — when budget
// permits AND any cluster has Finalizer owner AND the natural
// deepest pick is deeper than Finalizer, Finalizer is promoted to
// OrderedOwners[0].
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
	// deepest-first invariant.
	seen := make(map[RepairLocus]bool, len(plan.Clusters))
	ordered := make([]RepairLocus, 0, len(plan.Clusters))
	for _, c := range plan.Clusters {
		if c.Owner == "" {
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
	// FallbackTargetForViolationsWithBudget logic (fallback_policy.go
	// line ~573). When budget permits AND OrderedOwners[0] is deeper
	// than Finalizer AND Finalizer is somewhere in the queue,
	// promote Finalizer to OrderedOwners[0] so the cheap re-emit
	// gets up to budget attempts before paying for upstream rework.
	downgradeApplied := false
	if budget := FinalizerLocalRetryBudget(); budget > 0 && finalizerLocalUsed < budget {
		if !seen[LocusFinalizer] {
			// no-op — Finalizer not in queue
		} else if ordered[0] != LocusFinalizer {
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

// PromoteNextOwner advances the plan when the orchestrator decides a
// retry attempt closed the previous CurrentOwner's cluster.
//
// v3 B1 (2026-05-04): the caller chooses between rebuild / promote /
// same-owner via shouldRebuildExecutionPlan; this helper handles only
// the promote-or-stay paths. Rebuild is invoked directly via
// BuildRepairExecutionPlan(freshViolations, ...) at the
// AdvanceRepairExecutionPlan layer.
//
// closureUpdate (when non-nil) carries the next-attempt cluster
// closure state so the promoted plan inherits up-to-date PrimaryResolved
// / DerivedResolved / StableAttempts entries for any cluster owners
// that retain their slot. Pass nil to keep prev.ClusterStates verbatim
// (e.g. on the same-owner-stable++ path where the caller already
// updated state in place).
//
// Behaviour:
//
//   - len(freshViolations) > 0: rebuild via BuildRepairExecutionPlan.
//     The fresh kind set may have grown / changed — re-clustering is
//     the only safe move.
//
//   - freshViolations == nil AND prev.RemainingOwners empty: returns
//     prev unchanged (caller decides between FailLoud and same-owner).
//
//   - freshViolations == nil AND prev.RemainingOwners non-empty:
//     pops RemainingOwners[0] into CurrentOwner. Clusters /
//     OrderedOwners / ClusterStates preserve for telemetry; the
//     promoted owner's clusters reset StableAttempts to 0.
//
// finalizerLocalUsed is forwarded only in the rebuild branch (the
// promote-next branch does not re-apply the downgrade — it has
// already fired in the original Build call).
func PromoteNextOwner(prev RepairExecutionPlan, freshViolations []types.Violation, finalizerLocalUsed int) RepairExecutionPlan {
	if len(freshViolations) > 0 {
		return BuildRepairExecutionPlan(freshViolations, finalizerLocalUsed)
	}
	if len(prev.RemainingOwners) == 0 {
		return prev
	}
	out := RepairExecutionPlan{
		Clusters:                       append([]RepairCluster(nil), prev.Clusters...),
		OrderedOwners:                  append([]RepairLocus(nil), prev.OrderedOwners...),
		CurrentOwner:                   prev.RemainingOwners[0],
		EscalationAllowed:              prev.EscalationAllowed,
		HasFailLoud:                    prev.HasFailLoud,
		FinalizerLocalDowngradeApplied: prev.FinalizerLocalDowngradeApplied,
	}
	if len(prev.RemainingOwners) > 1 {
		out.RemainingOwners = append([]RepairLocus(nil), prev.RemainingOwners[1:]...)
	}
	// v3 B1: carry forward ClusterStates with the promoted owner's
	// stable counter reset (every cluster keeps its PrimaryResolved /
	// DerivedResolved verdict so the next attempt's closure check
	// has a starting baseline; promoted-owner clusters reset
	// StableAttempts to 0 so their budget refreshes on activation).
	if len(prev.ClusterStates) > 0 {
		out.ClusterStates = make([]RepairClusterExecutionState, len(prev.ClusterStates))
		for i, st := range prev.ClusterStates {
			if st.Owner == out.CurrentOwner {
				st.StableAttempts = 0
			}
			out.ClusterStates[i] = st
		}
	}
	return out
}

// nextPlanAction classifies what the caller should do given a
// previously-stashed plan and fresh violations. v3 B1 (2026-05-04)
// replaces the pre-v3 global kind-set strict-subset rule with
// per-cluster closure state. See repair_cluster_closure.go for
// fingerprint extraction + decision detail.
//
// Returns one of:
//
//	planActionRebuild    — drop the prev plan, BuildRepairExecutionPlan
//	                        from fresh
//	planActionPromote    — pop RemainingOwners[0] into CurrentOwner
//	                        (current owner's clusters all closed)
//	planActionStay       — same owner, stable++ (in-place state
//	                        update at AdvanceRepairExecutionPlan)
//	planActionFailLoud   — owner stuck AND no remaining owners AND
//	                        EscalationAllowed (caller maps to
//	                        FallbackFailLoud)
type nextPlanAction int

const (
	planActionRebuild nextPlanAction = iota
	planActionPromote
	planActionStay
	planActionFailLoud
)

// classifyNextPlanAction reads prev (with its ClusterStates updated
// by computeClusterClosure for the current attempt) and chooses the
// next action.
//
// Order of checks:
//
//  1. prev nil / IsEmpty / HasFailLoud / no fresh / no clusters →
//     rebuild (defensive — closure reasoning has nothing to read).
//  2. fresh introduces a new (kind, fingerprint) → rebuild.
//  3. any current-owner cluster: PrimaryResolved AND !DerivedResolved
//     → rebuild (cooccurrence theory broken).
//  4. all current-owner clusters: PrimaryResolved AND DerivedResolved
//     → promote next (or FailLoud when queue empty + EscalationAllowed).
//  5. any current-owner cluster: stuck (StableAttempts >= budget AND
//     !PrimaryResolved) → promote next (or FailLoud when queue empty
//     + EscalationAllowed).
//  6. else → stay (same owner; closure detector already
//     incremented StableAttempts).
func classifyNextPlanAction(prev *RepairExecutionPlan, fresh []types.Violation, stableBudget int) nextPlanAction {
	if prev == nil || prev.IsEmpty() || prev.HasFailLoud {
		return planActionRebuild
	}
	if len(fresh) == 0 {
		// No fresh violations at all — orchestrator wouldn't be in
		// the retry path. Defensive: rebuild so caller sees an empty
		// plan and the success branch fires.
		return planActionRebuild
	}
	if len(prev.ClusterStates) == 0 {
		// Pre-v3 plans (no ClusterStates seeded). Conservative: rebuild
		// so v3 closure semantics start fresh.
		return planActionRebuild
	}
	if freshIntroducesNewClusterIdent(*prev, fresh) {
		return planActionRebuild
	}
	if currentOwnerHasPrimaryResolvedDerivedPersists(*prev) {
		return planActionRebuild
	}
	if currentOwnerClustersAllClosed(*prev) {
		return promoteOrFailLoud(*prev)
	}
	if currentOwnerStuck(*prev, stableBudget) {
		return promoteOrFailLoud(*prev)
	}
	return planActionStay
}

// promoteOrFailLoud decides between promote-next and FailLoud when
// the current owner is "done with the cluster" (closed) or stuck.
// FailLoud requires an empty RemainingOwners queue AND
// EscalationAllowed; otherwise the next owner activates.
func promoteOrFailLoud(prev RepairExecutionPlan) nextPlanAction {
	if len(prev.RemainingOwners) > 0 {
		return planActionPromote
	}
	if prev.EscalationAllowed {
		return planActionFailLoud
	}
	return planActionStay
}

// shouldRebuildExecutionPlan is the legacy boolean wrapper preserved
// for back-compat with callers / tests that distinguished only
// "rebuild vs promote". v3 B1 callers should use classifyNextPlanAction
// directly to discriminate stay / FailLoud from rebuild / promote.
func shouldRebuildExecutionPlan(prev *RepairExecutionPlan, fresh []types.Violation) bool {
	// computeClusterClosure needs prev.ClusterStates already-set for
	// the current attempt; for back-compat callers we synthesise the
	// closure once here so the boolean answer reflects the new rules.
	if prev != nil && len(prev.ClusterStates) > 0 && len(fresh) > 0 {
		updatedStates := computeClusterClosure(*prev, fresh)
		clone := *prev
		clone.ClusterStates = updatedStates
		switch classifyNextPlanAction(&clone, fresh, ClusterStableBudget()) {
		case planActionPromote, planActionStay, planActionFailLoud:
			return false
		default:
			return true
		}
	}
	// No ClusterStates → legacy semantics (rebuild on any kind change /
	// equal kind set). Preserved for migration safety.
	return shouldRebuildExecutionPlanLegacy(prev, fresh)
}

// shouldRebuildExecutionPlanLegacy is the pre-v3 global kind-set
// strict-subset rule. Retained as a fallback for callers / tests
// that pre-date ClusterStates seeding. Behaviour byte-identical to
// the pre-B1 implementation.
func shouldRebuildExecutionPlanLegacy(prev *RepairExecutionPlan, fresh []types.Violation) bool {
	if prev == nil || prev.IsEmpty() || prev.HasFailLoud {
		return true
	}
	if len(prev.RemainingOwners) == 0 {
		return true
	}
	if len(fresh) == 0 {
		return true
	}

	prevKinds := make(map[types.ViolationKind]struct{}, len(prev.Clusters))
	for _, c := range prev.Clusters {
		prevKinds[c.Primary.Kind] = struct{}{}
		for _, d := range c.Derived {
			prevKinds[d.Kind] = struct{}{}
		}
	}

	freshKinds := make(map[types.ViolationKind]struct{}, len(fresh))
	for _, v := range fresh {
		freshKinds[v.Kind] = struct{}{}
	}

	for k := range freshKinds {
		if _, ok := prevKinds[k]; !ok {
			return true
		}
	}
	if len(freshKinds) >= len(prevKinds) {
		return true
	}
	return false
}

// AdvanceRepairExecutionPlan is the orchestrator-side entry point that
// reads the previously-stashed plan from MutableState, classifies the
// fresh violations against it, builds-or-promotes-or-stays
// accordingly, and stashes the resulting plan back. Returns the
// resulting plan + the FallbackTarget the orchestrator should
// dispatch.
//
// v3 B1 (2026-05-04): the rebuild/promote/stay/fail-loud decision now
// reads per-cluster closure state (computeClusterClosure +
// classifyNextPlanAction). The pre-v3 global kind-set strict-subset
// rule is retained as a back-compat fallback (shouldRebuildExecutionPlanLegacy)
// for plans without ClusterStates seeded.
//
// Behaviour:
//
//   - Empty fresh violations: returns FallbackFinalizerOnly + empty
//     plan; no MutableState write.
//   - HasFailLoud cluster: returns FallbackFailLoud; plan stashed.
//   - Otherwise: stashes the plan and returns
//     targetForLocus(plan.CurrentOwner). When the closure detector
//     diagnoses "current owner stuck AND no remaining owners AND
//     EscalationAllowed" the result is FallbackFailLoud (escalate
//     instead of cycling on the same stuck owner).
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

	var prevPlan *RepairExecutionPlan
	if mut != nil {
		if raw := mut.RepairExecutionPlan(); raw != nil {
			if p, ok := raw.(RepairExecutionPlan); ok {
				prevPlan = &p
			}
		}
	}

	var plan RepairExecutionPlan
	switch chooseNextPlanAction(prevPlan, fresh) {
	case planActionRebuild:
		plan = BuildRepairExecutionPlan(fresh, finalizerLocalUsed)
	case planActionPromote:
		plan = PromoteNextOwner(*prevPlan, nil, finalizerLocalUsed)
	case planActionStay:
		// Same owner — keep prev verbatim but persist updated
		// ClusterStates (StableAttempts incremented inside the
		// closure detector).
		plan = stayWithClusterClosure(*prevPlan, fresh)
	case planActionFailLoud:
		plan = stayWithClusterClosure(*prevPlan, fresh)
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
	// downgrade fired, preDowngrade == target. Promote-next path
	// inherits the original build's downgrade flag, so preDowngrade
	// is recomputed from the cluster set verbatim.
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

// chooseNextPlanAction wraps classifyNextPlanAction with the
// closure-state computation. Returns the action; pre-v3 plans
// without ClusterStates fall through to legacy rebuild-or-promote.
func chooseNextPlanAction(prev *RepairExecutionPlan, fresh []types.Violation) nextPlanAction {
	if prev == nil || prev.IsEmpty() || prev.HasFailLoud || len(prev.ClusterStates) == 0 {
		// Legacy fallback path so callers without ClusterStates
		// behave byte-identically to pre-v3.
		if shouldRebuildExecutionPlanLegacy(prev, fresh) {
			return planActionRebuild
		}
		return planActionPromote
	}
	updated := computeClusterClosure(*prev, fresh)
	clone := *prev
	clone.ClusterStates = updated
	return classifyNextPlanAction(&clone, fresh, ClusterStableBudget())
}

// stayWithClusterClosure preserves prev but updates ClusterStates
// with the latest closure verdict so the next attempt sees current
// PrimaryResolved / DerivedResolved / StableAttempts. Used by the
// stay and FailLoud paths in AdvanceRepairExecutionPlan.
func stayWithClusterClosure(prev RepairExecutionPlan, fresh []types.Violation) RepairExecutionPlan {
	out := RepairExecutionPlan{
		Clusters:                       append([]RepairCluster(nil), prev.Clusters...),
		OrderedOwners:                  append([]RepairLocus(nil), prev.OrderedOwners...),
		CurrentOwner:                   prev.CurrentOwner,
		EscalationAllowed:              prev.EscalationAllowed,
		HasFailLoud:                    prev.HasFailLoud,
		FinalizerLocalDowngradeApplied: prev.FinalizerLocalDowngradeApplied,
	}
	if len(prev.RemainingOwners) > 0 {
		out.RemainingOwners = append([]RepairLocus(nil), prev.RemainingOwners...)
	}
	out.ClusterStates = computeClusterClosure(prev, fresh)
	return out
}

// ── B1 v3 stable-budget knob ─────────────────────────────────────

// clusterStableBudget is the per-cluster cap on how many attempts
// the same owner may run without resolving its Primary before the
// closure detector promotes/escalates. Default 2 — matches the
// pre-v3 finalizerLocalRetryBudget intuition (give the owner two
// shots; if no progress, advance).
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
