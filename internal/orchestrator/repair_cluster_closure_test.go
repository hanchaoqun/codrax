package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// repair_cluster_closure_test.go — B1 v3 (2026-05-04). Cluster-state
// closure behavioural tests:
//
//   - cluster identity uses (kind, fingerprint) so same-kind clusters
//     for different facets / blocks are distinct.
//   - computeClusterClosure marks Primary / Derived resolution and
//     counts StableAttempts (W2.7 sibling rotation stays open).
//
// EVOLUTION RECORD (§40.43 R1, fold-in round three): the
// classifyNextPlanAction pins (rebuild / promote / stay / fail-loud) were
// deleted with the classifier. The closure state now feeds only the
// stability accounting and the stuck exit of AdvanceRepairExecutionPlan;
// those are pinned in repair_execution_plan_dispatch_test.go and
// repair_execution_plan_persistence_test.go.

// helper — synthesise a fresh violation pool. Detail strings are
// runtime-typed substrings the fingerprint extractor recognises.
func vFacet(facetKind string) types.Violation {
	return types.Violation{
		Kind:   types.ViolFacetUncovered,
		Detail: `required facet "` + facetKind + `" (essential) is not covered`,
	}
}

func vBlock(blockID string) types.Violation {
	return types.Violation{
		Kind:   types.ViolPrincipalClaimUseMissing,
		Detail: `block id="` + blockID + `" lacks claim_use`,
	}
}

func vClaimFormForBlock(blockID string) types.Violation {
	return types.Violation{
		Kind:   types.ViolClaimFormUnsupported,
		Detail: `block id="` + blockID + `" claim_form mismatch`,
	}
}

func TestClusterFingerprintOf_PrefersExplicitClusterKey(t *testing.T) {
	v := types.Violation{
		Kind:       types.ViolFacetUncovered,
		Detail:     `required facet "diagram_spine" not covered`,
		ClusterKey: "facet:diagram_spine|root:answer_facet_coverage",
		SuspectedRoot: types.SuspectedRoot{
			IRField: "answer_facet_coverage",
		},
	}
	got := clusterFingerprintOf(v)
	if got != "facet:diagram_spine|root:answer_facet_coverage" {
		t.Fatalf("clusterFingerprintOf should prefer explicit ClusterKey, got %q", got)
	}
}

func TestClusterFingerprintOf_PrefersTypedRootFieldWhenDetailIsGeneric(t *testing.T) {
	v := types.Violation{
		Kind:   types.ViolPrincipalProseUnderfilled,
		Detail: "principal block prose is too abstract",
		SuspectedRoot: types.SuspectedRoot{
			IRField: "answer_prose_density",
		},
	}
	got := clusterFingerprintOf(v)
	if got != "root:answer_prose_density" {
		t.Fatalf("clusterFingerprintOf should prefer typed root field, got %q", got)
	}
}

func TestClusterFingerprintOf_CombinesBlockAndTypedRoot(t *testing.T) {
	v := types.Violation{
		Kind:   types.ViolPrincipalClaimUseMissing,
		Detail: `principal block id="summary" has no claim_use`,
		SuspectedRoot: types.SuspectedRoot{
			IRField: "block_claim_use",
		},
	}
	got := clusterFingerprintOf(v)
	if got != "block:summary|root:block_claim_use" {
		t.Fatalf("clusterFingerprintOf should combine detail-derived and typed root identity, got %q", got)
	}
}

func TestClusterFingerprintOf_FallsBackToEvidenceRefsBeforeDetailHash(t *testing.T) {
	v := types.Violation{
		Kind:         types.ViolExternalArtifactUnderdecoded,
		Detail:       "generic underdecoded payload",
		EvidenceRefs: []string{"error_type:panic", "frame:buildAnalysisIR", "error_type:panic"},
	}
	got := clusterFingerprintOf(v)
	if got != "refs:error_type:panic|frame:buildAnalysisIR" {
		t.Fatalf("clusterFingerprintOf should prefer stable evidence refs before detail hash, got %q", got)
	}
}

func TestClusterFingerprintOf_PrefersEvidenceRefsOverDetailTokens(t *testing.T) {
	v := types.Violation{
		Kind:         types.ViolFacetUncovered,
		Detail:       `required facet "diagram_spine" (essential) is not covered`,
		EvidenceRefs: []string{"facet:diagram_spine", "coverage:block:summary"},
	}
	got := clusterFingerprintOf(v)
	if got != "refs:coverage:block:summary|facet:diagram_spine" {
		t.Fatalf("clusterFingerprintOf should prefer stable evidence refs over detail-derived tokens, got %q", got)
	}
}

// TestComputeClusterClosure_PrimaryFingerprintDistinguishesSameKindClusters
// — two ViolFacetUncovered clusters for facet=X and facet=Y are
// recognised as DISTINCT identities. Resolving X (fresh contains only
// Y) marks cluster_X.PrimaryResolved=true, cluster_Y.PrimaryResolved=false.
func TestComputeClusterClosure_PrimaryFingerprintDistinguishesSameKindClusters(t *testing.T) {
	prev := RepairExecutionPlan{
		ClusterStates: []RepairClusterExecutionState{
			{Owner: LocusFinalizer, PrimaryKind: types.ViolFacetUncovered,
				PrimaryFingerprint: "facet:X"},
			{Owner: LocusFinalizer, PrimaryKind: types.ViolFacetUncovered,
				PrimaryFingerprint: "facet:Y"},
		},
		CurrentOwner: LocusFinalizer,
	}
	fresh := []types.Violation{vFacet("Y")}

	got := computeClusterClosure(prev, fresh)
	if len(got) != 2 {
		t.Fatalf("expected 2 cluster states, got %d", len(got))
	}
	if !got[0].PrimaryResolved {
		t.Errorf("cluster facet=X should be PrimaryResolved=true (X not in fresh)")
	}
	if got[1].PrimaryResolved {
		t.Errorf("cluster facet=Y should be PrimaryResolved=false (Y in fresh)")
	}
}

// TestComputeClusterClosure_KindShiftSameFpStaysUnresolved is the
// W2.7 (2026-05-05) qfa-mr3 forensic case: Round 0 cluster with
// PrimaryKind=DiagramEdgeUnsupported, fp=block:d1; Round 1 fresh
// has DiagramEdgeLabelMismatch with fp=block:d1 (LLM "fixed"
// unsupported by adding edge_anchors but introduced label
// mismatch). Pre-W2.7 this looked like "cluster resolved + new
// cluster" (stable=0). Post-W2.7 the fp match keeps the cluster
// alive and stable increments — cycle detection works.
func TestComputeClusterClosure_KindShiftSameFpStaysUnresolved(t *testing.T) {
	prev := RepairExecutionPlan{
		ClusterStates: []RepairClusterExecutionState{
			{
				Owner:              LocusFinalizer,
				PrimaryKind:        types.ViolDiagramEdgeUnsupported,
				PrimaryFingerprint: "block:d1|root:diagram_edges",
				StableAttempts:     1,
			},
		},
		CurrentOwner: LocusFinalizer,
	}
	// Round 1 violation: DIFFERENT kind, SAME fingerprint.
	fresh := []types.Violation{
		{
			Kind:          types.ViolDiagramEdgeLabelMismatch,
			Detail:        "block id=\"d1\" edge label drifts",
			SuspectedRoot: types.SuspectedRoot{IRField: "diagram_edges"},
		},
	}
	got := computeClusterClosure(prev, fresh)
	if len(got) != 1 {
		t.Fatalf("expected 1 cluster state, got %d", len(got))
	}
	if got[0].PrimaryResolved {
		t.Errorf("Kind shift on same fp must keep cluster unresolved (rotation case); got PrimaryResolved=true")
	}
	if got[0].StableAttempts != 2 {
		t.Errorf("StableAttempts = %d, want 2 (incremented from 1 because primary stayed unresolved)",
			got[0].StableAttempts)
	}
}

// TestSummarizeClusterStates_NamesClosure verifies the telemetry
// surface includes per-cluster closure flags. INTERNAL telemetry —
// guards against drift from "rebuild reason" debugging.
func TestSummarizeClusterStates_NamesClosure(t *testing.T) {
	states := []RepairClusterExecutionState{
		{
			Owner:              LocusFinalizer,
			PrimaryKind:        types.ViolFacetUncovered,
			PrimaryFingerprint: "facet:X",
			PrimaryResolved:    true,
			DerivedResolved:    false,
			StableAttempts:     2,
		},
	}
	summary := SummarizeClusterStates(states)
	for _, want := range []string{
		"primary_resolved=true",
		"derived_resolved=false",
		"stable=2",
		"facet:X",
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary missing %q: %s", want, summary)
		}
	}
}

// TestBuildRepairExecutionPlan_SeedsClusterStates — fresh plan build
// must populate ClusterStates 1:1 with Clusters.
func TestBuildRepairExecutionPlan_SeedsClusterStates(t *testing.T) {
	vs := []types.Violation{vFacet("X"), vFacet("Y")}
	plan := BuildRepairExecutionPlan(vs, 0)
	if len(plan.ClusterStates) != len(plan.Clusters) {
		t.Fatalf("ClusterStates length %d != Clusters length %d",
			len(plan.ClusterStates), len(plan.Clusters))
	}
	for i, st := range plan.ClusterStates {
		if st.Owner != plan.Clusters[i].Owner {
			t.Errorf("ClusterStates[%d].Owner = %q, Clusters[%d].Owner = %q",
				i, st.Owner, i, plan.Clusters[i].Owner)
		}
		if st.PrimaryFingerprint == "" {
			t.Errorf("ClusterStates[%d].PrimaryFingerprint is empty", i)
		}
	}
}
