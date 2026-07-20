package types

import "testing"

// trace_causal_projection_clusterfix2_test.go — CLUSTER-FIX-2 件1 (S1)
// note→node parse pin: the typed freq_only cause token rides the SAME
// fold_basis presence gate as its CAP siblings; absence stays empty so every
// pre-batch record keeps its wording byte-identically.

func TestTraceCausalProjectionClusterFix2FreqOnlyReasonParse(t *testing.T) {
	node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext,
		cap2FoldRecord("fold_capability_freq_only_reason=single_cluster"))
	if node.SupplyFoldCapabilityFreqOnlyReason != "single_cluster" {
		t.Fatalf("fold_capability_freq_only_reason must reach the node verbatim: %q", node.SupplyFoldCapabilityFreqOnlyReason)
	}
	// Absence = pre-batch/judged record: no reason claim.
	if bare := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, cap2FoldRecord()); bare.SupplyFoldCapabilityFreqOnlyReason != "" {
		t.Fatalf("absent note must stay empty (byte-preserving legacy): %q", bare.SupplyFoldCapabilityFreqOnlyReason)
	}
	// Without the fold, a stray reason note claims nothing (fold_basis
	// presence gate).
	stray := ObservationRecord{
		ID: "fix2-1", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Predicate: "root_cause_context", Subject: "worker-9", Object: "running",
		RichNotes: []string{"type=running", "fold_capability_freq_only_reason=single_cluster"},
	}
	if node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, stray); node.SupplyFoldComputed ||
		node.SupplyFoldCapabilityFreqOnlyReason != "" {
		t.Fatalf("a reason claim without a fold is meaningless: %+v", node)
	}
}
