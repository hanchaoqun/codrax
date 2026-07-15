package types

// trace_causal_projection_rnb2_test.go — RNB-2 件2 engine-side pins (§29.88 W3
// 病①, 2026-07-15; witness customer runnable.txt E32: 3 same-(subject,object)
// ◇ D-state rows merged to 10.643 = 9.272 + 0.478 + 0.893 while the seed
// remainder's 行2 kept speaking 「全窗9.272…本行其余9.272」):
//
//	(a) the R2 group key forks on the typed anchorForm (remainder / clipped /
//	    lane-demoted rows never merge with plain rows of one (subject,object));
//	(b) a homogeneous merge of bipartition seats clears the per-seat triple and
//	    stamps MergedChainAnchorMemberAccounts (seed 三元组不得冒充合并行账 —
//	    CASE3-D4 对 eff 的同款处置精神).

import "testing"

func rnb2AggNode(id string, impact float64) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Role: TraceCausalRoleRootCauseContext, EvidenceID: id,
		Subject: "workShark-6666", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
		StateKind: "d_sleep", ChainRelevance: "adjacent",
		ImpactMS: impact, CumulativeImpactMS: impact,
		LineStart: 10, LineEnd: 20, StartTs: 100.01, EndTs: 100.02,
	}
}

// (a) E32 shape: the remainder seat buckets APART from its two plain siblings
// — the plain pair stays unmerged (below the ≥3 threshold) and the remainder
// keeps its own exact triple.
func TestRNB2AnchorFormForksR2GroupKey(t *testing.T) {
	remainder := rnb2AggNode("e-rem", 9.272)
	remainder.ChainAnchoredMS = 0
	remainder.ChainAnchorFullMS = 9.272
	remainder.ChainAnchorRemainderSeat = true
	nodes := []TraceCausalProjectionNode{remainder, rnb2AggNode("e-p1", 0.478), rnb2AggNode("e-p2", 0.893)}
	out := traceCausalProjectionAggregateSameKind(nodes)
	if len(out) != 3 {
		t.Fatalf("anchorForm fork must keep the remainder apart from plain rows (no ×3 merge): got %d rows", len(out))
	}
	for _, node := range out {
		if node.MergedCount > 1 {
			t.Fatalf("no merge may happen across anchor forms: %+v", node)
		}
	}
	if out[0].ChainAnchorFullMS != 9.272 || !out[0].ChainAnchorRemainderSeat {
		t.Fatalf("the unmerged remainder must keep its exact triple: %+v", out[0])
	}
}

// (a) negative control: three PLAIN rows of one (subject,object) keep the
// pre-RNB-2 ×3 SUM byte-identically ("" form never forks any key).
func TestRNB2PlainRowsStillMerge(t *testing.T) {
	nodes := []TraceCausalProjectionNode{rnb2AggNode("e-1", 1.0), rnb2AggNode("e-2", 2.0), rnb2AggNode("e-3", 3.0)}
	out := traceCausalProjectionAggregateSameKind(nodes)
	if len(out) != 1 || out[0].MergedCount != 3 || out[0].ImpactMS != 6.0 {
		t.Fatalf("plain ×3 merge must stay byte-identical: %+v", out)
	}
	if out[0].MergedChainAnchorMemberAccounts {
		t.Fatalf("plain merges never stamp the member-accounts marker")
	}
}

// (b) homogeneous bipartition merge: the seed triple must NOT impersonate the
// merged row — cleared fields + the typed member-accounts marker.
func TestRNB2HomogeneousRemainderMergeClearsTriple(t *testing.T) {
	mk := func(id string, impact, anchored, full float64) TraceCausalProjectionNode {
		n := rnb2AggNode(id, impact)
		n.ChainAnchoredMS = anchored
		n.ChainAnchorFullMS = full
		n.ChainAnchorRemainderSeat = true
		return n
	}
	nodes := []TraceCausalProjectionNode{
		mk("e-r1", 9.272, 0, 9.272), mk("e-r2", 2.0, 1.0, 3.0), mk("e-r3", 4.0, 0.5, 4.5),
	}
	out := traceCausalProjectionAggregateSameKind(nodes)
	if len(out) != 1 || out[0].MergedCount != 3 {
		t.Fatalf("homogeneous remainder rows share one anchorForm and must merge ×3: %+v", out)
	}
	merged := out[0]
	if merged.ChainAnchorFullMS != 0 || merged.ChainAnchoredMS != 0 ||
		merged.ChainAnchorOwnershipDivergent || merged.ChainAnchorChainLaneMS != 0 || merged.ChainAnchorCensusMS != 0 {
		t.Fatalf("the merged row must clear the per-seat bipartition account (禁单成员值冒充): %+v", merged)
	}
	if !merged.MergedChainAnchorMemberAccounts {
		t.Fatalf("the merged row must carry the typed seed-member-accounts marker")
	}
	if !merged.ChainAnchorRemainderSeat {
		t.Fatalf("the channel-identity flag stays (all members are remainder seats): %+v", merged)
	}
}
