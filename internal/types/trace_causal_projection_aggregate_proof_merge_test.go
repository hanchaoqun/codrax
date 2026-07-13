package types

import "testing"

// trace_causal_projection_aggregate_proof_merge_test.go — 修复轮三 F1
// (2026-07-13): the ×N same-kind merge's proof-family semantics pinned at
// the merge authority itself (traceCausalProjectionMergeSameKindMembers is
// THE single merge body): refined = AND over members (distinct facts — one
// unproven member keeps the honest merged word), 等待对象 = unanimity (any
// diverging member vetoes the symbol). The tool-level word-face pin
// (TestProofDonorMixedMembersVetoBothLanes) discriminates the AND flip; THIS
// pin discriminates BOTH the AND flip and the unanimity-veto removal
// mechanically (the tool fixture's seed election can mask the veto arm).
func TestMergeSameKindProofFamilyANDAndUnanimity(t *testing.T) {
	nodes := []TraceCausalProjectionNode{
		{EvidenceID: "seed", Subject: "worker-200", Object: "unknown-thread",
			ImpactMS: 40, CumulativeImpactMS: 40, LineStart: 10, LineEnd: 20,
			DStateRefinedNonIO: true, BlockedReasonCaller: "dma_fence_default_wait"},
		{EvidenceID: "m2", Subject: "worker-200", Object: "unknown-thread",
			ImpactMS: 5, CumulativeImpactMS: 5, LineStart: 30, LineEnd: 40,
			DStateRefinedNonIO: false, BlockedReasonCaller: "dma_fence_default_wait"},
		{EvidenceID: "m3", Subject: "worker-200", Object: "unknown-thread",
			ImpactMS: 10, CumulativeImpactMS: 10, LineStart: 50, LineEnd: 60,
			DStateRefinedNonIO: false, BlockedReasonCaller: ""},
	}
	merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, []int{0, 1, 2})
	if merged.DStateRefinedNonIO {
		t.Fatalf("one unproven member must veto the family proof (AND, never OR): %+v", merged)
	}
	if merged.BlockedReasonCaller != "" {
		t.Fatalf("a callerless member must veto the family symbol (unanimity): %q", merged.BlockedReasonCaller)
	}
	// The unanimous all-proven family keeps both.
	for i := range nodes {
		nodes[i].DStateRefinedNonIO = true
		nodes[i].BlockedReasonCaller = "dma_fence_default_wait"
	}
	merged = traceCausalProjectionMergeSameKindMembers(nodes, 0, []int{0, 1, 2})
	if !merged.DStateRefinedNonIO || merged.BlockedReasonCaller != "dma_fence_default_wait" {
		t.Fatalf("an all-proven unanimous family keeps proof + symbol: %+v", merged)
	}
}
