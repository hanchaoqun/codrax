// Package orchestrator — repair_plan.go
//
// Phase 1-A1 (V2 runtime consolidation, 2026-05-04). RepairPlan is
// the root-cause-aware successor to FallbackTargetForViolations.
//
// Pre-Phase 1, fallback routing was: bucket violations by RepairLocus
// (the stage that owns the fix), pick the locus with the most
// violations, on tie pick deepest. The model treats every violation
// as independent, which over-pulls when a single root cause emits 3+
// derived violations: count + deepest-tiebreak picks a deeper locus
// even though the root cause is finalize-local.
//
// Phase 1 model:
//   1. Group violations by DispatchID (same emit / same validator
//      pass clusters together).
//   2. Within each group, apply typed cooccurrence rules
//      (repair_cooccurrence.go) to identify Primary + Derived.
//   3. Each cluster's repair owner = Primary's RepairLocus (Derived
//      do NOT contribute their own owner, since fixing Primary
//      necessarily clears them).
//   4. PrimaryOwner of the whole RepairPlan = deepest cluster owner
//      (because fixing a finalize-local cluster cannot remediate
//      an explore-local cluster — they are independent root causes).
//
// Compatibility: existing callers of FallbackTargetForViolations and
// FallbackTargetForViolationsWithBudget remain valid; A3 will
// rewire them to consume RepairPlan internally. A2 wires
// RetryState to track LastPrimaryOwner / OwnerStableAttempts so
// the orchestrator can detect ping-pong (same owner picked N times,
// no progress → escalate to FailLoud).

package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RepairCluster groups one Primary violation with its typed-Derived
// consequences. Derived may be empty (a cluster of size 1 means the
// violation is its own Primary, with no rule matching).
type RepairCluster struct {
	// Primary is the root-cause violation. Its kind drives Owner.
	Primary types.Violation

	// Derived are the violations whose validators fire as a
	// consequence of Primary's failure mode. Empty when no
	// cooccurrence rule applies (singleton cluster).
	Derived []types.Violation

	// Owner is the RepairLocus that owns Primary's fix
	// (LocusOfTarget(FallbackTargetForViolation(Primary))). Derived
	// do not contribute their own owner.
	Owner RepairLocus

	// Reason names the typed invariant from the matched
	// CooccurrenceRule, or "<no rule applied>" for singleton clusters.
	// Surfaced in repair-plan telemetry for operator audit.
	Reason string
}

// RepairPlan is the dispatch-ready output of BuildRepairPlan. The
// orchestrator consumes PrimaryOwner to choose its requeue target;
// EscalationAllowed gates the optional "after N stable attempts,
// escalate to FailLoud" path.
type RepairPlan struct {
	// Clusters is the partition of input violations into
	// root-cause-aware groups. Order is stable: deepest-Owner first,
	// breaking ties by Primary's ViolationKind alphabetical order.
	Clusters []RepairCluster

	// PrimaryOwner is the deepest RepairLocus among Clusters[].Owner.
	// This is the orchestrator's dispatch target (translated to
	// FallbackTarget via targetForLocus).
	//
	// Empty input ⇒ LocusFinalizer (the safest no-op when the
	// validator returned no violations).
	PrimaryOwner RepairLocus

	// HasFailLoud is true when any cluster's Owner is
	// LocusTerminal — the FailLoud safety net (an unrecoverable
	// violation kind in the input forces the whole plan to halt
	// regardless of cluster majority).
	HasFailLoud bool
}

// BuildRepairPlan groups violations into root-cause-aware clusters
// using the typed cooccurrence rules in repair_cooccurrence.go and
// chooses the deepest cluster owner as the dispatch target.
//
// Algorithm (deterministic):
//
//   1. groupByDispatch(vs) → map[DispatchID][]Violation.
//   2. For each group:
//      a. matchRule(group) → (rule, ok). On match, the violation
//         whose Kind == rule.Primary is the cluster Primary; every
//         violation whose Kind ∈ rule.Derived joins as Derived.
//      b. Violations not covered by any rule become singleton
//         clusters (their own Primary, empty Derived).
//   3. PrimaryOwner = deepest of the per-cluster Owners. FailLoud
//      (LocusTerminal) wins outright if present (failure-of-last-
//      resort red line, mirrors FallbackTargetForViolations).
//
// Returned plan is safe for telemetry serialization (no internal
// pointers).
func BuildRepairPlan(vs []types.Violation) RepairPlan {
	if len(vs) == 0 {
		return RepairPlan{PrimaryOwner: LocusFinalizer}
	}

	groups := groupByDispatch(vs)
	var clusters []RepairCluster
	for _, group := range groups {
		clusters = append(clusters, clusterizeGroup(group)...)
	}

	plan := RepairPlan{Clusters: clusters}

	// Compute PrimaryOwner: deepest among clusters; LocusTerminal
	// wins outright (failure-of-last-resort red line).
	//
	// Phase 1-C (V2 runtime eval followup, 2026-05-04): SOFT-
	// Terminal clusters DO NOT trigger HasFailLoud. Pre-fix, the
	// FailLoud safety-net mapping for SOFT-default kinds (e.g.
	// ViolRichnessRegression → FallbackFailLoud, intended only
	// when operator promotes to STRICT) leaked into BuildRepairPlan
	// via LocusOfTarget(FallbackFailLoud) = LocusTerminal,
	// forcing the entire plan to fail_loud whenever the SOFT
	// violation appeared. The m1a-2 / u3a-1 outlier traces were
	// the symptom: 4-cluster plans with one SOFT richness
	// regression flipped routing to terminal.
	//
	// Filter scope: SOFT-Terminal ONLY. SOFT clusters with a
	// non-Terminal owner (LocusExtract / LocusExplore /
	// LocusFinalizer) still drive PrimaryOwner, because most
	// violation kinds default to SOFT classification — gating all
	// SOFT clusters out of routing would break extract/explore
	// fallback for the common case (SubjectAnchorMissing →
	// extract, FacetUncovered → explore, etc.). The narrow filter
	// only blocks the FailLoud-promotion path for SOFT kinds.
	plan.PrimaryOwner = LocusFinalizer
	maxDepth := -1
	for _, c := range clusters {
		ownerIsTerminal := c.Owner == LocusTerminal
		isSoft := isSoftViolationKind(c.Primary.Kind)
		if ownerIsTerminal && isSoft {
			continue
		}
		if ownerIsTerminal {
			plan.HasFailLoud = true
		}
		if d := locusDepth(c.Owner); d > maxDepth {
			maxDepth = d
			plan.PrimaryOwner = c.Owner
		}
	}
	if plan.HasFailLoud {
		plan.PrimaryOwner = LocusTerminal
	}

	// Stable order: deepest first, then by Primary.Kind for ties.
	sortClustersDeepestFirst(plan.Clusters)
	return plan
}

// clusterizeGroup applies the cooccurrence rules to one
// same-dispatch group of violations.
//
// Process:
//
//   Repeatedly try matchRule on the remaining (unassigned) group.
//   On match, peel off Primary + every present Derived into a
//   cluster, mark them assigned. When no rule matches, every
//   remaining violation becomes a singleton cluster.
//
// This handles multi-rule scenarios within one dispatch (e.g. a
// SubjectAnchorMissing cluster + an independent ChainDemoted
// cluster in the same emit pass).
func clusterizeGroup(group []types.Violation) []RepairCluster {
	if len(group) == 0 {
		return nil
	}
	remaining := append([]types.Violation(nil), group...)
	var clusters []RepairCluster

	for len(remaining) > 0 {
		rule, ok := matchRule(remaining, defaultCooccurrenceRules)
		if !ok {
			// No rule applies — every remaining violation is its own
			// Primary (singleton cluster).
			for _, v := range remaining {
				clusters = append(clusters, singletonCluster(v))
			}
			return clusters
		}

		// Found a rule. Peel off Primary + present Derived.
		var primary types.Violation
		var derived []types.Violation
		var keep []types.Violation
		primaryFound := false
		derivedSet := make(map[types.ViolationKind]bool, len(rule.Derived))
		for _, d := range rule.Derived {
			derivedSet[d] = true
		}
		for _, v := range remaining {
			switch {
			case !primaryFound && v.Kind == rule.Primary:
				primary = v
				primaryFound = true
			case derivedSet[v.Kind]:
				derived = append(derived, v)
			default:
				keep = append(keep, v)
			}
		}
		// Defensive: matchRule already verified Primary is present;
		// if for any reason it isn't (concurrent mutation, bug),
		// fall back to singleton clusters.
		if !primaryFound {
			for _, v := range remaining {
				clusters = append(clusters, singletonCluster(v))
			}
			return clusters
		}
		clusters = append(clusters, RepairCluster{
			Primary: primary,
			Derived: derived,
			Owner:   LocusOfTarget(FallbackTargetForViolation(primary)),
			Reason:  rule.Reason,
		})
		remaining = keep
	}
	return clusters
}

// singletonCluster wraps one violation as its own Primary, with no
// Derived. The Owner is the violation's standard FallbackTargetForViolation
// locus. Reason is "<no rule applied>" so telemetry can distinguish
// rule-driven from fallback clusters.
func singletonCluster(v types.Violation) RepairCluster {
	return RepairCluster{
		Primary: v,
		Owner:   LocusOfTarget(FallbackTargetForViolation(v)),
		Reason:  "<no cooccurrence rule applied — independent root cause>",
	}
}

// sortClustersDeepestFirst orders clusters by Owner depth descending,
// breaking ties on Primary.Kind alphabetical for determinism.
func sortClustersDeepestFirst(clusters []RepairCluster) {
	for i := 1; i < len(clusters); i++ {
		for j := i; j > 0; j-- {
			a, b := clusters[j-1], clusters[j]
			ad := locusDepth(a.Owner)
			bd := locusDepth(b.Owner)
			if bd > ad ||
				(bd == ad && string(b.Primary.Kind) < string(a.Primary.Kind)) {
				clusters[j-1], clusters[j] = clusters[j], clusters[j-1]
				continue
			}
			break
		}
	}
}

// locusDepth mirrors the value scale used inside
// FallbackTargetForViolationsWithBudget so RepairPlan and the legacy
// router agree on "deepest" semantics.
func locusDepth(l RepairLocus) int {
	switch l {
	case LocusFinalizer:
		return 1
	case LocusExtract:
		return 2
	case LocusExplore:
		return 3
	case LocusTerminal:
		return 4
	}
	return 0
}

// SummarizeRepairPlan returns a one-line telemetry summary for trace
// emission. Format: "primary=<locus> clusters=N kinds=[k1,k2,...]"
// — read by the orchestrator's per-dispatch trace writer.
func SummarizeRepairPlan(plan RepairPlan) string {
	if len(plan.Clusters) == 0 {
		return fmt.Sprintf("primary=%s clusters=0 kinds=[]", plan.PrimaryOwner)
	}
	var kinds []string
	for _, c := range plan.Clusters {
		kinds = append(kinds, string(c.Primary.Kind))
	}
	return fmt.Sprintf("primary=%s clusters=%d kinds=[%s]",
		plan.PrimaryOwner, len(plan.Clusters), strings.Join(kinds, ","))
}
