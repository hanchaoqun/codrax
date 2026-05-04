// Package orchestrator — repair_cluster_closure.go
//
// B1 v3 (2026-05-04). Cluster-state closure detection for
// RepairExecutionPlan. Replaces the pre-v3 global kind-set strict-
// subset rule (shouldRebuildExecutionPlan) with per-cluster
// fingerprinted closure tracking:
//
//   1. cluster identity = (PrimaryKind, PrimaryFingerprint) — runtime
//      typed signal extracted from violation.Detail at build time.
//      The fingerprint distinguishes "two ViolFacetUncovered clusters
//      for facet=X and facet=Y" so that resolving one of them is
//      observed as progress, not "kind unchanged".
//
//   2. ClusterState tracks PrimaryResolved / DerivedResolved /
//      StableAttempts per cluster, mirroring prev.Clusters 1:1.
//
//   3. shouldRebuildExecutionPlan reads cluster state to decide
//      rebuild / promote / same-owner-stable++:
//        - any (kind, fingerprint) in fresh missing from prev   → rebuild
//        - any current-owner cluster Primary cleared but Derived
//          still present                                          → rebuild
//          (cooccurrence theory broken — Derived is now an
//          independent root cause).
//        - all current-owner clusters PrimaryResolved AND
//          DerivedResolved                                        → promote next
//        - any current-owner cluster StableAttempts >= budget AND
//          !PrimaryResolved                                       → promote next /
//                                                                    FailLoud
//        - else                                                  → same owner,
//                                                                    stable++
//
// Iteration savings: in multi-facet "two ViolFacetUncovered" runs
// the pre-v3 rule rebuilt the whole plan whenever the kind-set
// stayed the same (because both clusters share the kind). The new
// rule observes "X cleared, Y still here" and stays on the same
// Finalizer owner to fix Y — saving 1-2 retry rounds.

package orchestrator

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RepairClusterExecutionState is one row in
// RepairExecutionPlan.ClusterStates — 1:1 with prev.Clusters[i] at
// plan-build time. Carries the per-cluster typed signals the closure
// detector reads on the next attempt.
type RepairClusterExecutionState struct {
	// Owner mirrors RepairCluster.Owner — the locus that owns the
	// fix for this cluster's Primary.
	Owner RepairLocus

	// PrimaryKind is RepairCluster.Primary.Kind at build time.
	PrimaryKind types.ViolationKind

	// PrimaryFingerprint is the runtime typed signal extracted from
	// RepairCluster.Primary.Detail (block id / facet kind / etc.).
	// Used together with PrimaryKind as the cross-attempt cluster
	// identity. See clusterFingerprintOf for derivation rules.
	PrimaryFingerprint string

	// DerivedKinds lists RepairCluster.Derived[].Kind for closure
	// matching. Order is preserved verbatim from build time.
	DerivedKinds []types.ViolationKind

	// PrimaryResolved is set on the next attempt: true when no fresh
	// violation matches (PrimaryKind, PrimaryFingerprint).
	PrimaryResolved bool

	// DerivedResolved is set on the next attempt: true when no fresh
	// violation matches any DerivedKinds value.
	DerivedResolved bool

	// StableAttempts counts consecutive attempts where the same
	// owner ran without resolving Primary. Resets to 0 when Primary
	// resolves OR the owner advances. Drives the stuck-owner
	// promote / FailLoud path.
	StableAttempts int
}

// ClusterStateForCluster returns the initial state for a freshly-
// built cluster (PrimaryResolved=false, DerivedResolved=false,
// StableAttempts=0). Called by BuildRepairExecutionPlan to seed
// ClusterStates 1:1 with prev.Clusters.
func ClusterStateForCluster(c RepairCluster) RepairClusterExecutionState {
	derivedKinds := make([]types.ViolationKind, 0, len(c.Derived))
	for _, d := range c.Derived {
		derivedKinds = append(derivedKinds, d.Kind)
	}
	return RepairClusterExecutionState{
		Owner:              c.Owner,
		PrimaryKind:        c.Primary.Kind,
		PrimaryFingerprint: clusterFingerprintOf(c.Primary),
		DerivedKinds:       derivedKinds,
	}
}

// clusterFingerprintOf extracts a stable, runtime-typed identifier
// from a violation that lets the closure detector match the same
// "logical cluster" across retry attempts. Strategy (priority order):
//
//  1. block id — `block id="<X>"` substring (V2 oracle violations).
//  2. facet kind — `facet "<X>"` substring (validateFacetCoverage).
//  3. relation kind — `relation kind=<X>` substring (diagram edge).
//  4. fallback — sha1 hash of the first 80 bytes of v.Detail.
//
// The fallback ensures a never-empty fingerprint even when Detail
// shape is unfamiliar; the hash is stable per Detail, so identical
// violations across attempts produce the same fingerprint.
//
// All branches read **runtime current-dispatch typed substrings** —
// no static dictionary, no fuzzy matching. R3 invariant.
func clusterFingerprintOf(v types.Violation) string {
	if id := extractBlockIDFromDetail(v.Detail); id != "" {
		return "block:" + id
	}
	if facet := extractFacetKindFromDetail(v.Detail); facet != "" {
		return "facet:" + facet
	}
	if rel := extractRelationKindFromDetail(v.Detail); rel != "" {
		return "relation:" + rel
	}
	// Fallback — opaque hash of the prefix. Stable across attempts
	// when the producer emits identical Detail.
	prefix := v.Detail
	if len(prefix) > 80 {
		prefix = prefix[:80]
	}
	sum := sha1.Sum([]byte(prefix))
	return "h:" + hex.EncodeToString(sum[:6])
}

// extractFacetKindFromDetail pulls the facet kind out of details
// shaped like `required facet "X" (...)` (validateFacetCoverage)
// or `optional richness facet "X" has N evidence ...`
// (validateRichnessRegression). Returns "" on no match.
func extractFacetKindFromDetail(detail string) string {
	for _, prefix := range []string{
		`required facet "`,
		`optional richness facet "`,
	} {
		idx := strings.Index(detail, prefix)
		if idx < 0 {
			continue
		}
		rest := detail[idx+len(prefix):]
		end := strings.Index(rest, `"`)
		if end < 0 {
			continue
		}
		return rest[:end]
	}
	return ""
}

// extractRelationKindFromDetail pulls the typed RelationKind out of
// details shaped like `relation kind=<X>` (validateDiagramRelationLegality).
func extractRelationKindFromDetail(detail string) string {
	const marker = `relation kind=`
	idx := strings.Index(detail, marker)
	if idx < 0 {
		return ""
	}
	rest := detail[idx+len(marker):]
	end := strings.IndexAny(rest, " \t,)\n")
	if end < 0 {
		end = len(rest)
	}
	return strings.TrimSpace(rest[:end])
}

// computeClusterClosure carries forward each prev cluster's state
// based on whether its (kind, fingerprint) and DerivedKinds appear in
// fresh. Returns a new []RepairClusterExecutionState with
// PrimaryResolved / DerivedResolved updated and StableAttempts
// incremented when Primary remains unresolved.
//
// Pre-condition: prev.ClusterStates is 1:1 with prev.Clusters
// (BuildRepairExecutionPlan guarantees this).
func computeClusterClosure(prev RepairExecutionPlan, fresh []types.Violation) []RepairClusterExecutionState {
	if len(prev.ClusterStates) == 0 {
		return nil
	}
	freshIdent := make(map[clusterIdent]bool, len(fresh))
	freshKinds := make(map[types.ViolationKind]bool, len(fresh))
	for _, v := range fresh {
		freshIdent[clusterIdent{Kind: v.Kind, Fingerprint: clusterFingerprintOf(v)}] = true
		freshKinds[v.Kind] = true
	}

	out := make([]RepairClusterExecutionState, len(prev.ClusterStates))
	for i, st := range prev.ClusterStates {
		ident := clusterIdent{Kind: st.PrimaryKind, Fingerprint: st.PrimaryFingerprint}
		st.PrimaryResolved = !freshIdent[ident]
		st.DerivedResolved = derivedAllResolved(st.DerivedKinds, freshKinds)
		if st.PrimaryResolved {
			st.StableAttempts = 0
		} else {
			st.StableAttempts++
		}
		out[i] = st
	}
	return out
}

// clusterIdent is the (kind, fingerprint) pair used as cross-attempt
// cluster identity.
type clusterIdent struct {
	Kind        types.ViolationKind
	Fingerprint string
}

// derivedAllResolved returns true when no DerivedKinds value appears
// in freshKinds. Empty derived list trivially returns true.
func derivedAllResolved(derivedKinds []types.ViolationKind, freshKinds map[types.ViolationKind]bool) bool {
	for _, k := range derivedKinds {
		if freshKinds[k] {
			return false
		}
	}
	return true
}

// currentOwnerClustersAllClosed returns true when every cluster
// owned by plan.CurrentOwner has BOTH PrimaryResolved=true AND
// DerivedResolved=true. Returns false when CurrentOwner has zero
// clusters in ClusterStates (defensive — orphan owner indicates
// a build error, fall back to "not closed" so the rebuild check
// fires and re-derives the plan).
func currentOwnerClustersAllClosed(plan RepairExecutionPlan) bool {
	if plan.CurrentOwner == "" {
		return false
	}
	any := false
	for _, st := range plan.ClusterStates {
		if st.Owner != plan.CurrentOwner {
			continue
		}
		any = true
		if !st.PrimaryResolved || !st.DerivedResolved {
			return false
		}
	}
	return any
}

// currentOwnerHasPrimaryResolvedDerivedPersists returns true when
// any current-owner cluster has Primary resolved but Derived still
// present. This signals the cooccurrence theory was wrong — fixing
// Primary should have cleared Derived but didn't, so the residual
// Derived is now an independent root cause and the plan should
// rebuild from scratch.
func currentOwnerHasPrimaryResolvedDerivedPersists(plan RepairExecutionPlan) bool {
	if plan.CurrentOwner == "" {
		return false
	}
	for _, st := range plan.ClusterStates {
		if st.Owner != plan.CurrentOwner {
			continue
		}
		if st.PrimaryResolved && !st.DerivedResolved {
			return true
		}
	}
	return false
}

// currentOwnerStuck returns true when any current-owner cluster has
// reached or exceeded the per-cluster stable-attempt budget without
// resolving its Primary. Used by promotion logic to advance to the
// next owner (or to FailLoud when RemainingOwners is empty and
// EscalationAllowed).
func currentOwnerStuck(plan RepairExecutionPlan, budget int) bool {
	if plan.CurrentOwner == "" || budget <= 0 {
		return false
	}
	for _, st := range plan.ClusterStates {
		if st.Owner != plan.CurrentOwner {
			continue
		}
		if st.StableAttempts >= budget && !st.PrimaryResolved {
			return true
		}
	}
	return false
}

// freshIntroducesNewClusterIdent returns true when fresh contains
// any (kind, fingerprint) not already in prev.ClusterStates. This is
// the "new root cause exposed" signal that mandates rebuild.
func freshIntroducesNewClusterIdent(prev RepairExecutionPlan, fresh []types.Violation) bool {
	prevIdents := make(map[clusterIdent]bool, len(prev.ClusterStates))
	for _, st := range prev.ClusterStates {
		prevIdents[clusterIdent{Kind: st.PrimaryKind, Fingerprint: st.PrimaryFingerprint}] = true
		// Also tolerate fresh violations matching a previously-
		// classified Derived kind by the same fingerprint — those
		// are NOT new root causes; they're already part of an
		// existing cluster.
		for _, dk := range st.DerivedKinds {
			prevIdents[clusterIdent{Kind: dk, Fingerprint: st.PrimaryFingerprint}] = true
		}
	}
	for _, v := range fresh {
		ident := clusterIdent{Kind: v.Kind, Fingerprint: clusterFingerprintOf(v)}
		if !prevIdents[ident] {
			return true
		}
	}
	return false
}

// SummarizeClusterStates renders a one-line trace string describing
// every cluster's closure state. Used by SummarizeRepairExecutionPlan
// for operator-facing telemetry. Format:
//
//	[(kind=A primary_resolved=true derived_resolved=false stable=2), ...]
//
// INTERNAL telemetry only — never rendered into LLM-facing strings.
func SummarizeClusterStates(states []RepairClusterExecutionState) string {
	if len(states) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(states))
	for _, st := range states {
		parts = append(parts, fmt.Sprintf(
			"(owner=%s kind=%s fp=%s primary_resolved=%t derived_resolved=%t stable=%d)",
			st.Owner, st.PrimaryKind, st.PrimaryFingerprint,
			st.PrimaryResolved, st.DerivedResolved, st.StableAttempts))
	}
	return "[" + strings.Join(parts, " ") + "]"
}
