// Package orchestrator — repair_cluster_closure.go
//
// B1 v3 (2026-05-04). Cluster-state closure detection for
// RepairExecutionPlan:
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
//   3. §40.43 R1 (fold-in round three): the closure feeds ONLY the
//      stability accounting of AdvanceRepairExecutionPlan — the
//      per-cluster StableAttempts count (owner-attributed, §40.43 finding
//      U: advanced only for clusters owned by the owner the previous
//      round dispatched; carried over the fresh rebuild for every
//      persisting cluster key, W2.7 sibling rotation included)
//      and the stuck-owner fail-loud exit
//        - any current-owner cluster StableAttempts >= budget AND
//          !PrimaryResolved, no remaining owners, escalation allowed
//                                                          → FailLoud.
//      The dispatch target itself is always the fresh rebuild
//      (repair_execution_plan.go); the pre-R1 rebuild / promote / stay
//      classification over this state no longer exists.

package orchestrator

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
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

	// StableAttempts counts the finalize attempts in which THIS
	// cluster's owner was the dispatched owner and the Primary stayed
	// unresolved. It is owner-attributed (§40.43 F-orch 四轮复核 finding
	// U): computeClusterClosure advances it only in a round whose
	// dispatched owner (prev.CurrentOwner — the target actually
	// dispatched last round, after the R2.2 downgrade) equals this
	// cluster's Owner; a cluster whose owner did not run keeps its
	// count, so a never-dispatched root stays at 0 and is always
	// dispatched (anyClusterNeverAttempted). Resets to 0 when Primary
	// resolves. Drives the stuck-cluster FailLoud exit.
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
//  1. producer-side ClusterKey when present
//  2. SuspectedRoot.IRField when present (typed producer-side root
//     field, independent of Detail wording)
//  3. EvidenceRefs fingerprint when present (stable references the
//     producer attached to the violation)
//  4. typed primary tokens from Detail when present:
//     - block id        (`block id="<X>"`)
//     - facet kind      (`facet "<X>"`)
//     - relation kind   (`relation kind=<X>`)
//  5. fallback — sha1 hash of the first 80 bytes of v.Detail
//
// The result is a joined typed identity (e.g.
// "block:summary|root:block_claim_use") when multiple signals are
// available. Producer-side typed signals (ClusterKey / IRField /
// EvidenceRefs) outrank Detail parsing so closure remains stable even
// when validator wording changes.
//
// All branches read **runtime current-dispatch typed substrings /
// fields** — no static dictionary, no fuzzy matching. R3 invariant.
func clusterFingerprintOf(v types.Violation) string {
	if key := strings.TrimSpace(v.ClusterKey); key != "" {
		return key
	}
	if refs := evidenceRefsFingerprint(v.EvidenceRefs); refs != "" {
		return refs
	}
	parts := make([]string, 0, 4)
	if id := extractBlockIDFromDetail(v.Detail); id != "" {
		parts = append(parts, "block:"+id)
	}
	if facet := extractFacetKindFromDetail(v.Detail); facet != "" {
		parts = append(parts, "facet:"+facet)
	}
	if rel := extractRelationKindFromDetail(v.Detail); rel != "" {
		parts = append(parts, "relation:"+rel)
	}
	if root := strings.TrimSpace(v.SuspectedRoot.IRField); root != "" {
		parts = append(parts, "root:"+root)
	}
	if len(parts) > 0 {
		return strings.Join(parts, "|")
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

func evidenceRefsFingerprint(refs []string) string {
	if len(refs) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(refs))
	norm := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		norm = append(norm, ref)
	}
	if len(norm) == 0 {
		return ""
	}
	if len(norm) == 1 {
		return "ref:" + norm[0]
	}
	sort.Strings(norm)
	joined := strings.Join(norm, "|")
	if len(joined) <= 96 {
		return "refs:" + joined
	}
	sum := sha1.Sum([]byte(joined))
	return "refs:h:" + hex.EncodeToString(sum[:6])
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
// advanced — owner-attributed (§40.43 F-orch 四轮复核 finding U): only
// for a cluster whose Owner equals prev.CurrentOwner, the owner the
// previous round actually dispatched (after the R2.2 finalizer-local
// downgrade). A cluster whose owner did not run this round keeps its
// count: with fresh {must_include (finalizer), facet_uncovered
// (explore)} and two downgraded finalizer_only rounds, the explore
// cluster stays at 0 and is dispatched when the finalizer clears its
// own cluster — the earlier unattributed count reached the stable
// budget for an owner that never ran and fail-louded instead.
//
// Pre-condition: prev.ClusterStates is 1:1 with prev.Clusters
// (BuildRepairExecutionPlan guarantees this).
func computeClusterClosure(prev RepairExecutionPlan, fresh []types.Violation) []RepairClusterExecutionState {
	if len(prev.ClusterStates) == 0 {
		return nil
	}
	dispatched := prev.CurrentOwner
	freshIdent := make(map[clusterIdent]bool, len(fresh))
	freshKinds := make(map[types.ViolationKind]bool, len(fresh))
	// W2.7 (2026-05-05): build a fp -> set of fresh kinds map so
	// rotation detection can ask "did the same fp persist with a
	// SIBLING kind that PrimaryKind implies?". Without this, the
	// qfa-mr3 forensic rotation (DiagramEdgeUnsupported -> Edge-
	// LabelMismatch on fp=block:d1) looked like "Primary resolved
	// + new cluster" (stable=0) instead of "rotating sibling
	// (stable++)".
	freshKindsByFp := make(map[string]map[types.ViolationKind]bool, len(fresh))
	for _, v := range fresh {
		fp := clusterFingerprintOf(v)
		freshIdent[clusterIdent{Kind: v.Kind, Fingerprint: fp}] = true
		freshKinds[v.Kind] = true
		set := freshKindsByFp[fp]
		if set == nil {
			set = make(map[types.ViolationKind]bool, 2)
			freshKindsByFp[fp] = set
		}
		set[v.Kind] = true
	}

	out := make([]RepairClusterExecutionState, len(prev.ClusterStates))
	for i, st := range prev.ClusterStates {
		ident := clusterIdent{Kind: st.PrimaryKind, Fingerprint: st.PrimaryFingerprint}
		exactMatch := freshIdent[ident]
		// W2.7 sibling-on-same-fp check: when PrimaryKind is gone
		// from fresh BUT a kind that PrimaryKind.Implies appears on
		// the same fp, treat as exact-match-equivalent. The
		// cooccurrence theory holds; a sibling validator just took
		// over reporting the same root cause. Without this branch,
		// the LLM "fixing" one validator only to trip another on
		// the same fp resets stable=0 every round.
		impliedSiblingOnSameFp := false
		if !exactMatch {
			if siblings := primaryImpliesSiblingsOnFp(st.PrimaryKind, freshKindsByFp[st.PrimaryFingerprint]); siblings {
				impliedSiblingOnSameFp = true
			}
		}
		st.PrimaryResolved = !(exactMatch || impliedSiblingOnSameFp)
		st.DerivedResolved = derivedAllResolved(st.DerivedKinds, freshKinds)
		switch {
		case st.PrimaryResolved:
			st.StableAttempts = 0
		case st.Owner == dispatched:
			// Only the owner that ran can have failed to resolve it.
			st.StableAttempts++
		}
		out[i] = st
	}
	return out
}

// primaryImpliesSiblingsOnFp returns true when at least one
// freshKind is declared by primary.Implies in the registry.
// Empty / unregistered primary returns false.
func primaryImpliesSiblingsOnFp(primary types.ViolationKind, freshKinds map[types.ViolationKind]bool) bool {
	if len(freshKinds) == 0 {
		return false
	}
	spec, ok := types.ViolKindSpecFor(primary)
	if !ok {
		return false
	}
	for _, implied := range spec.Implies {
		if freshKinds[implied] {
			return true
		}
	}
	return false
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
// clusters in ClusterStates. Telemetry only (SummarizeRepairExecutionPlan
// `closed=`) since §40.43 R1 — it no longer routes a dispatch.
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

// currentOwnerStuck returns true when any current-owner cluster has
// reached or exceeded the per-cluster stable-attempt budget without
// resolving its Primary. Read by the stuck-cluster fail-loud exit
// (stuckClusterExit: RemainingOwners empty AND EscalationAllowed) and
// by the telemetry summary.
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
