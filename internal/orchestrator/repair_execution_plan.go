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
// retry attempt closed the previous CurrentOwner's cluster (its
// violation kind set is a strict subset of prev.Clusters' kind set).
// The caller's classification is encoded by passing nil
// freshViolations OR by calling shouldRebuildExecutionPlan first.
//
// Behaviour:
//
//   - len(freshViolations) > 0: rebuild via BuildRepairExecutionPlan.
//     The fresh kind set may have grown / changed — re-clustering is
//     the only safe move.
//
//   - freshViolations == nil AND prev.RemainingOwners empty: returns
//     prev unchanged. CurrentOwner stays put; the orchestrator's
//     OwnerStableAttempts counter (RetryState) escalates upward.
//
//   - freshViolations == nil AND prev.RemainingOwners non-empty:
//     pops RemainingOwners[0] into CurrentOwner. Clusters /
//     OrderedOwners preserve verbatim for telemetry.
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
	return out
}

// shouldRebuildExecutionPlan classifies the fresh violation set
// against a previously-stashed plan. Returns true (rebuild) when:
//
//   - prev is nil
//   - prev.IsEmpty() OR prev.HasFailLoud (stale terminal state)
//   - prev.RemainingOwners is empty (queue exhausted; force re-eval)
//   - the fresh ViolationKind set is NOT a subset of prev.Clusters'
//     kind set (kinds appeared that weren't there before)
//
// Returns false (promote next) when the fresh kind set is a strict
// subset of prev.Clusters' kind set — i.e. the previous owner's
// dispatch closed at least one cluster's violations and only known
// downstream kinds remain.
//
// Equal kind sets (no violation closed) ALSO returns true (rebuild),
// because no progress means re-clustering is at least as informative
// as advancing through a queue that already failed once.
func shouldRebuildExecutionPlan(prev *RepairExecutionPlan, fresh []types.Violation) bool {
	if prev == nil || prev.IsEmpty() || prev.HasFailLoud {
		return true
	}
	if len(prev.RemainingOwners) == 0 {
		return true
	}
	if len(fresh) == 0 {
		// No fresh violations at all — orchestrator wouldn't be in
		// the retry path. Defensive: rebuild.
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

	// Any kind in fresh not in prev → rebuild (new root cause exposed).
	for k := range freshKinds {
		if _, ok := prevKinds[k]; !ok {
			return true
		}
	}
	// All fresh kinds were in prev. Strict-subset means progress was
	// made (some kinds are gone): promote next. Equal means zero
	// progress: rebuild — safer than advancing through a queue that
	// already failed.
	if len(freshKinds) >= len(prevKinds) {
		return true
	}
	return false
}

// AdvanceRepairExecutionPlan is the orchestrator-side entry point that
// reads the previously-stashed plan from MutableState, classifies the
// fresh violations against it, builds-or-promotes accordingly, and
// stashes the resulting plan back. Returns the resulting plan + the
// FallbackTarget the orchestrator should dispatch.
//
// Behaviour mirrors the legacy FallbackTargetForViolationsWithBudget
// + telemetry semantics:
//
//   - Empty fresh violations: returns FallbackFinalizerOnly (the
//     safest no-op when validator returned nothing) and an empty
//     plan; no MutableState write.
//   - HasFailLoud cluster: returns FallbackFailLoud; plan stashed.
//   - Otherwise: stashes the plan and returns
//     targetForLocus(plan.CurrentOwner).
//
// preDowngrade is the locus the legacy budget-less picker WOULD
// have chosen — used by the orchestrator's R2.2 telemetry to detect
// "downgrade fired" events. Equals plan.CurrentOwner when no
// downgrade applied; otherwise equals the deepest cluster's owner.
//
// nil MutableState is tolerated for tests / direct callers — the
// plan is built fresh every call (no persistence). Production
// callers always pass a non-nil MutableState.
func AdvanceRepairExecutionPlan(mut *types.MutableState, fresh []types.Violation, finalizerLocalUsed int) (RepairExecutionPlan, FallbackTarget, FallbackTarget) {
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
	if shouldRebuildExecutionPlan(prevPlan, fresh) {
		plan = BuildRepairExecutionPlan(fresh, finalizerLocalUsed)
	} else {
		plan = PromoteNextOwner(*prevPlan, nil, finalizerLocalUsed)
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

// SummarizeRepairExecutionPlan returns a one-line telemetry summary
// for trace emission. Format mirrors SummarizeRepairPlan but adds
// queue state. INTERNAL telemetry — never rendered into LLM-facing
// strings.
//
//	"current=<locus> ordered=[o1,o2,...] remaining=N stable=<bool>
//	 budget_downgrade=<bool> fail_loud=<bool>"
func SummarizeRepairExecutionPlan(plan RepairExecutionPlan) string {
	if plan.IsEmpty() {
		return "current=<empty> ordered=[] remaining=0"
	}
	owners := make([]string, 0, len(plan.OrderedOwners))
	for _, o := range plan.OrderedOwners {
		owners = append(owners, string(o))
	}
	return fmt.Sprintf(
		"current=%s ordered=[%s] remaining=%d budget_downgrade=%t fail_loud=%t",
		plan.CurrentOwner,
		strings.Join(owners, ","),
		len(plan.RemainingOwners),
		plan.FinalizerLocalDowngradeApplied,
		plan.HasFailLoud,
	)
}
