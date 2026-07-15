package types

// trace_causal_projection_rnb_context_bucket_test.go — RNB-1 D1 修复轮
// (§29.88 复核, 2026-07-14) pins for the ◇/▒ context-bucket capacity
// structure: value seats compete by value (in-path context rows may no
// longer unconditionally preempt), and overflow folds with a count instead
// of the former silent nodes[:8] discard (donghu 2955 witness: 8 in-path
// context rows ate the whole cap and every ◇ remainder seat — 47.660 down —
// silently vanished).

import (
	"strings"
	"testing"
)

func rnbContextNode(subject string, value float64, seat bool) TraceCausalProjectionNode {
	node := TraceCausalProjectionNode{
		Subject: subject, ChainRelevance: "adjacent", EvidenceID: "ev-" + subject,
		ImpactMS: value, CumulativeImpactMS: value,
	}
	if seat {
		node.Rank = 1
		node.ChainAnchorRemainderSeat = true
		node.ChainAnchoredMS = 0.1
		node.ChainAnchorFullMS = value + 0.1
	}
	return node
}

// The two-class order: value seats first (value order), context rows after.
func TestRNBContextBucketValueSeatsBeforeInPathContext(t *testing.T) {
	pathIndex := map[string]int{"waker-1": 0, "waker-2": 1}
	nodes := []TraceCausalProjectionNode{
		rnbContextNode("waker-1", 15.0, false), // in-path context row
		rnbContextNode("waker-2", 14.0, false), // in-path context row
		rnbContextNode("seatB", 17.2, true),
		rnbContextNode("seatA", 47.6, true),
	}
	sorted := traceCausalProjectionSortContextBucket(nodes, pathIndex)
	if sorted[0].Subject != "seatA" || sorted[1].Subject != "seatB" {
		t.Fatalf("value seats must lead the bucket in value order: %s, %s", sorted[0].Subject, sorted[1].Subject)
	}
	if sorted[2].Subject != "waker-1" || sorted[3].Subject != "waker-2" {
		t.Fatalf("context rows keep their legacy relative order behind the seats: %s, %s", sorted[2].Subject, sorted[3].Subject)
	}
}

// Overflow folds with a count — never a silent discard.
func TestRNBContextBucketOverflowFoldsWithCount(t *testing.T) {
	var nodes []TraceCausalProjectionNode
	for i := 0; i < 11; i++ {
		nodes = append(nodes, rnbContextNode(string(rune('a'+i)), float64(20-i), i < 4))
	}
	out := traceCausalProjectionLimitContextNodesFold(nodes, 8, "adjacent")
	if len(out) != 9 {
		t.Fatalf("cap must keep 8 rows + ONE counted fold row, got %d", len(out))
	}
	fold := out[8]
	if strings.TrimSpace(fold.Subject) != "" || fold.MergedCount != 3 || fold.OnChainOverflowFold {
		t.Fatalf("overflow must fold into the subjectless stanza-fold form (count=3, !OnChainOverflowFold): %+v", fold)
	}
	if fold.ChainRelevance != "adjacent" {
		t.Fatalf("the fold row stays in its bucket's channel: %q", fold.ChainRelevance)
	}
	if fold.MergedMaxMS != 12.0 {
		t.Fatalf("fold value is the member MAX (never a sum): %+v", fold.MergedMaxMS)
	}
	// ≤limit inputs stay byte-identical to the plain limiter.
	small := traceCausalProjectionLimitContextNodesFold(nodes[:5], 8, "adjacent")
	if len(small) != 5 {
		t.Fatalf("under-cap inputs must pass through: %d", len(small))
	}
}

// The post-aggregation resort keeps the two-class context order — the former
// classified re-sort silently restored in-path preemption right before the
// fold cap (donghu 2955: value seats 16.013/10.571 folded while 1.462/1.252
// kept individual rows).
func TestRNBContextBucketResortKeepsSeatOrder(t *testing.T) {
	out := TraceCausalProjection{
		WakeupPath: []string{"waker-1"},
		AdjacentCauses: []TraceCausalProjectionNode{
			rnbContextNode("waker-1", 15.0, false), // in-path context row
			rnbContextNode("seatSmall", 1.4, true),
			rnbContextNode("seatBig", 16.0, true),
		},
	}
	traceCausalProjectionResortAfterAggregation(&out)
	got := []string{out.AdjacentCauses[0].Subject, out.AdjacentCauses[1].Subject, out.AdjacentCauses[2].Subject}
	if got[0] != "seatBig" || got[1] != "seatSmall" || got[2] != "waker-1" {
		t.Fatalf("post-aggregation resort must keep value seats first in value order: %v", got)
	}
}
