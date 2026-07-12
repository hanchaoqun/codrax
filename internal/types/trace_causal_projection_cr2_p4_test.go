package types

import "testing"

// CR-2 组① P4 徽章-图例闭合门 (ledger §29.42 P4 / §29.49 F-2 移交, 2026-07-12;
// witness 冷读 F-6, donghu 20260712-133933: the rank #2 seat row (JankManager
// runnable_wait 16.687ms, root_cause_secondary) sat beyond the on-chain bucket
// cap and was folded into 「其余 7 项(链上折叠)」 — ❷ appeared 0 times in the
// whole report while the legend promised ❶..❺ and the engine had published a
// contiguous seat ordinal. 判据: ❶..❺ 承诺 ⇒ typed rank 前五持席者各存在带徽章
// 独立行. 修向 (工单默认): 持席行(席位 1..5)豁免进折叠 — v5 E.3 「携席位行
// 不可折」白名单精神的 compile-layer 落点; the fold row itself stays a counted
// roster and still never wears a badge.

// seat-holder node helper: a typed rank seat (1..TopN) beyond the cap.
func cr2P4Node(id, subject string, impact float64, rank int) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Role: TraceCausalRoleCausalHop, EvidenceID: id, Subject: subject,
		Predicate: "wakeup_causal_impact", ChainRelevance: "on_chain",
		ImpactMS: impact, CumulativeImpactMS: impact, Rank: rank,
	}
}

// F-6 形 pin: a rank-2 seat row inside the fold candidate set stays an
// independent row (available to wear ❷ downstream); the fold count honestly
// excludes it.
func TestCR2P4OnChainFoldExemptsSeatHolderRows(t *testing.T) {
	nodes := []TraceCausalProjectionNode{
		cr2P4Node("e-1", "keeper-a", 100, 1),
		cr2P4Node("e-2", "keeper-b", 90, 0),
		cr2P4Node("e-3", "seat-2", 16.687, 2), // the F-6 witness shape: seat #2 beyond the cap
		cr2P4Node("e-4", "plain-a", 9, 0),
		cr2P4Node("e-5", "plain-b", 8, 0),
	}
	out := traceCausalProjectionLimitNodesOnChainFold(nodes, 2, nil)
	if len(out) != 4 {
		t.Fatalf("want kept(2) + exempted seat row + fold row = 4 rows, got %d: %+v", len(out), out)
	}
	seat := out[2]
	if seat.Subject != "seat-2" || seat.Rank != 2 || seat.OnChainOverflowFold {
		t.Fatalf("seat #2 row must survive the fold as an independent row: %+v", seat)
	}
	fold := out[3]
	if !fold.OnChainOverflowFold {
		t.Fatalf("last row must be the fold roster: %+v", fold)
	}
	if fold.MergedCount != 2 {
		t.Fatalf("fold count must honestly exclude the exempted seat row (want 2, got %d)", fold.MergedCount)
	}
	for _, s := range fold.MergedSubjects {
		if s == "seat-2" {
			t.Fatalf("exempted seat row must not double-publish inside the fold roster: %v", fold.MergedSubjects)
		}
	}
}

// Boundary: seats beyond the badge population (rank > TopN) keep folding —
// the exemption mirrors the ❶..❺ promise exactly, not every positive rank.
func TestCR2P4OnChainFoldExemptionBoundary(t *testing.T) {
	nodes := []TraceCausalProjectionNode{
		cr2P4Node("e-1", "keeper-a", 100, 1),
		cr2P4Node("e-2", "keeper-b", 90, 0),
		cr2P4Node("e-3", "seat-5", 5, TraceCausalProjectionSeatFoldExemptTopN),
		cr2P4Node("e-4", "rank-6", 4, TraceCausalProjectionSeatFoldExemptTopN+1),
	}
	out := traceCausalProjectionLimitNodesOnChainFold(nodes, 2, nil)
	if len(out) != 4 {
		t.Fatalf("want kept(2) + exempted rank-5 + fold(rank-6) = 4 rows, got %d", len(out))
	}
	if out[2].Subject != "seat-5" || out[2].OnChainOverflowFold {
		t.Fatalf("rank-5 seat must be exempt: %+v", out[2])
	}
	fold := out[3]
	if !fold.OnChainOverflowFold || fold.MergedCount != 1 {
		t.Fatalf("rank-6 row must keep folding (no seat promise beyond TopN): %+v", fold)
	}
}

// Control: an overflow set without any seat holder folds byte-identically to
// the pre-CR2 shape (no behavior drift on the dominant form).
func TestCR2P4OnChainFoldNoSeatHolderControl(t *testing.T) {
	nodes := []TraceCausalProjectionNode{
		cr2P4Node("e-1", "keeper-a", 100, 1),
		cr2P4Node("e-2", "keeper-b", 90, 0),
		cr2P4Node("e-3", "plain-a", 9, 0),
		cr2P4Node("e-4", "plain-b", 8, 0),
	}
	out := traceCausalProjectionLimitNodesOnChainFold(nodes, 2, nil)
	if len(out) != 3 {
		t.Fatalf("want kept(2) + fold row = 3 rows, got %d", len(out))
	}
	if !out[2].OnChainOverflowFold || out[2].MergedCount != 2 {
		t.Fatalf("control fold must keep the legacy shape: %+v", out[2])
	}
}

// The SupportingHops fold shares the promise: a seat row never disappears into
// the hop fold either (same white-list, one predicate).
func TestCR2P4HopsFoldExemptsSeatHolderRows(t *testing.T) {
	hops := []TraceCausalProjectionNode{
		cr2P4Node("h-1", "keeper-a", 100, 0),
		cr2P4Node("h-2", "keeper-b", 90, 0),
		cr2P4Node("h-3", "seat-3", 12, 3),
		cr2P4Node("h-4", "plain-a", 9, 0),
		cr2P4Node("h-5", "plain-b", 8, 0),
	}
	out := traceCausalProjectionLimitHopsFold(hops, nil, 2)
	if len(out) != 4 {
		t.Fatalf("want kept(2) + exempted seat row + fold row = 4 rows, got %d", len(out))
	}
	if out[2].Subject != "seat-3" || out[2].OnChainOverflowFold {
		t.Fatalf("seat #3 hop must survive the fold: %+v", out[2])
	}
	if !out[3].OnChainOverflowFold || out[3].MergedCount != 2 {
		t.Fatalf("hop fold count must exclude the exempted seat row: %+v", out[3])
	}
}
