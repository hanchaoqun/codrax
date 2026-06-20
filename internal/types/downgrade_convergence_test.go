package types

import "testing"

func TestComputeDowngradeBlockerKey_StableAndOrderIndependent(t *testing.T) {
	pendA := []PendingRead{{Origin: "phase1_unread", File: "a.go"}, {Origin: "primary_anchor", File: "b.go"}}
	pendB := []PendingRead{{Origin: "primary_anchor", File: "b.go"}, {Origin: "phase1_unread", File: "a.go"}} // reordered
	if ComputeDowngradeBlockerKey(pendA, nil, nil) != ComputeDowngradeBlockerKey(pendB, nil, nil) {
		t.Fatal("blocker key must be order-independent over the same typed blocker set")
	}
	// A genuinely changed blocker (one pending read drained) changes the key.
	pendShrunk := []PendingRead{{Origin: "phase1_unread", File: "a.go"}}
	if ComputeDowngradeBlockerKey(pendA, nil, nil) == ComputeDowngradeBlockerKey(pendShrunk, nil, nil) {
		t.Fatal("draining a pending read must change the blocker key (real progress resets convergence)")
	}
	// Repairs and unverified findings participate typed-only.
	withRepair := ComputeDowngradeBlockerKey(pendA, []UnverifiedFinding{{Kind: "path", Token: "x.go"}}, []RepairDirective{{Kind: RepairExpandSearch, Subject: "x"}})
	if withRepair == ComputeDowngradeBlockerKey(pendA, nil, nil) {
		t.Fatal("unverified/repairs must contribute to the blocker key")
	}
}

func TestAppendDowngradeFingerprint_ConvergesAndResets(t *testing.T) {
	c := NewEvidenceClosure("")
	fpA := DowngradeFingerprint{Lane: DowngradeLaneContractChain, BlockerKey: 111}
	if got := c.AppendDowngradeFingerprint(fpA); got != 1 {
		t.Fatalf("first append consecutive = %d, want 1", got)
	}
	if got := c.AppendDowngradeFingerprint(fpA); got != 2 {
		t.Fatalf("second identical = %d, want 2", got)
	}
	if got := c.AppendDowngradeFingerprint(fpA); got != 3 {
		t.Fatalf("third identical = %d, want 3", got)
	}
	// A changed blocker (or lane) resets the consecutive count.
	fpB := DowngradeFingerprint{Lane: DowngradeLaneContractChain, BlockerKey: 222}
	if got := c.AppendDowngradeFingerprint(fpB); got != 1 {
		t.Fatalf("changed blocker resets to 1, got %d", got)
	}
	// Different lane, same key also resets.
	fpC := DowngradeFingerprint{Lane: DowngradeLaneCurrentSourceLane, BlockerKey: 222}
	if got := c.AppendDowngradeFingerprint(fpC); got != 1 {
		t.Fatalf("changed lane resets to 1, got %d", got)
	}
}

func TestProgressDeltaConvergesLikeDowngradeFingerprint(t *testing.T) {
	c := NewEvidenceClosure("")
	first := c.RecordDowngradeProgressDelta(DowngradeLaneContractChain, 111, 3)
	if !first.ShouldReplan || first.ReasonCode != ProgressReasonContinue || first.Delta.Consecutive != 1 {
		t.Fatalf("first decision = %+v, want continue with consecutive=1", first)
	}
	second := c.RecordDowngradeProgressDelta(DowngradeLaneContractChain, 111, 3)
	if !second.ShouldReplan || second.Delta.Consecutive != 2 {
		t.Fatalf("second decision = %+v, want continue with consecutive=2", second)
	}
	third := c.RecordDowngradeProgressDelta(DowngradeLaneContractChain, 111, 3)
	if third.ShouldReplan || third.ReasonCode != ProgressReasonConverged || third.Delta.Consecutive != 3 {
		t.Fatalf("third decision = %+v, want converged at consecutive=3", third)
	}
	reset := c.RecordDowngradeProgressDelta(DowngradeLaneContractChain, 222, 3)
	if !reset.ShouldReplan || reset.Delta.Consecutive != 1 {
		t.Fatalf("changed blocker should reset progress decision: %+v", reset)
	}
}

func TestAppendCompletionCaveat_DedupByLane(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AppendCompletionCaveat(CompletionCaveat{Lane: DowngradeLaneContractChain, ReasonCode: ProgressReasonConverged, Reason: "r1"})
	c.AppendCompletionCaveat(CompletionCaveat{Lane: DowngradeLaneContractChain, Reason: "r2"})
	c.AppendCompletionCaveat(CompletionCaveat{Lane: DowngradeLaneCurrentSourceLane, Reason: "r3"})
	c.AppendCompletionCaveat(CompletionCaveat{Lane: ""}) // ignored
	got := c.CompletionCaveats()
	if len(got) != 2 {
		t.Fatalf("caveats deduped by lane: got %d, want 2 (%+v)", len(got), got)
	}
	if got[0].Reason != "r1" {
		t.Fatalf("first-write-wins per lane, got %q", got[0].Reason)
	}
	if got[0].ReasonCode != ProgressReasonConverged {
		t.Fatalf("typed reason code lost: %+v", got[0])
	}
}
