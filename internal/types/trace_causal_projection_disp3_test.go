package types

// trace_causal_projection_disp3_test.go — DISP-3 显示面修复批 engine-half pins
// (docs/design/real_trace_campaign_20260705.md §29.8, 792 四场景回访, 2026-07-09):
//
//   item3 E22 ◇席窗标回归 — the §11-N2 merge zeroes a multi-window ×N row's
//         row-level QueryWindow; the rank ordinal's OWN window identity now
//         survives on RankQueryWindowStartTs/EndTs (verbatim member endpoints
//         of the rank-supplying member; absence never guesses).
//   item5 E19 跨窗折叠漏拒% — the on-chain overflow fold constructor now
//         carries its member query-window roster (same F-2 slot dedupe as the
//         R2 merge) so the §21.1 CWD-2 ① %-suppression gate can see the fold.
//
// Fixtures are cast through the REAL merge/fold authorities
// (traceCausalProjectionAggregateSameKind / TraceCausalProjectionMergeOccurrenceRows /
// traceCausalProjectionOverflowFoldRow) — never hand-written Merged* fields.
//
// Mutation self-checks (each verified RED during development, then restored):
//   M-A: dropping the rank-window capture in the merge loop →
//        TestDisp3MergeKeepsRankSupplyingMemberWindow red.
//   M-B: dropping the fold's MergedQueryWindows roster →
//        TestDisp3OverflowFoldCarriesMemberWindowRoster red (and the tool-side
//        %-suppression pin red).

import (
	"testing"
)

func disp3MergeMember(id string, rank int, impact, winStart, winEnd float64) TraceCausalProjectionNode {
	node := TraceCausalProjectionNode{
		Role: TraceCausalRoleRootCauseContext, EvidenceID: id,
		Subject: "oney.hmn.berlin-42591", Object: "trace_span",
		ImpactMS: impact, CumulativeImpactMS: impact,
		Rank: rank, Confidence: 0.7,
	}
	if winStart > 0 {
		node.QueryWindowStartTs, node.QueryWindowEndTs = winStart, winEnd
	}
	return node
}

// TestDisp3MergeKeepsRankSupplyingMemberWindow pins the E22 regression repair
// (§29.8 P2-⑧, huadong_792 E22 "根因排序#2·置信中" vs the huadong_79 chips):
// members from TWO query windows zero the row-level pair (unchanged §11-N2
// behavior), while the rank-supplying (smallest-Rank) member's window travels
// on the new RankQueryWindow pair verbatim.
func TestDisp3MergeKeepsRankSupplyingMemberWindow(t *testing.T) {
	nodes := []TraceCausalProjectionNode{
		disp3MergeMember("m1", 5, 8.611, 6793224.299, 6793224.501),
		disp3MergeMember("m2", 2, 9.169, 6793222.700, 6793222.901),
		disp3MergeMember("m3", 7, 8.633, 6793224.299, 6793224.501),
	}
	out := traceCausalProjectionAggregateSameKind(nodes)
	if len(out) != 1 {
		t.Fatalf("R2 must merge the ×3 same-(subject,object) group: %d rows", len(out))
	}
	merged := out[0]
	if merged.Rank != 2 {
		t.Fatalf("merged rank must be the member min: %d", merged.Rank)
	}
	if merged.QueryWindowStartTs != 0 || merged.QueryWindowEndTs != 0 {
		t.Fatalf("multi-window members must still zero the row-level window (§11-N2): %.3f–%.3f",
			merged.QueryWindowStartTs, merged.QueryWindowEndTs)
	}
	if merged.RankQueryWindowStartTs != 6793222.700 || merged.RankQueryWindowEndTs != 6793222.901 {
		t.Fatalf("the rank-supplying member's window must survive verbatim: %.3f–%.3f",
			merged.RankQueryWindowStartTs, merged.RankQueryWindowEndTs)
	}
}

// TestDisp3MergeRankWindowAbsenceNeverGuesses: a rank-supplying member WITHOUT
// a window identity leaves the pair zero even when other members carry windows.
func TestDisp3MergeRankWindowAbsenceNeverGuesses(t *testing.T) {
	nodes := []TraceCausalProjectionNode{
		disp3MergeMember("m1", 5, 8.611, 6793224.299, 6793224.501),
		disp3MergeMember("m2", 2, 9.169, 0, 0), // rank winner, no window identity
		disp3MergeMember("m3", 7, 8.633, 6793222.700, 6793222.901),
	}
	out := traceCausalProjectionAggregateSameKind(nodes)
	if len(out) != 1 {
		t.Fatalf("R2 must merge: %d rows", len(out))
	}
	if out[0].Rank != 2 {
		t.Fatalf("merged rank must be the member min: %d", out[0].Rank)
	}
	if out[0].RankQueryWindowStartTs != 0 || out[0].RankQueryWindowEndTs != 0 {
		t.Fatalf("absence never guesses a window: %.3f–%.3f",
			out[0].RankQueryWindowStartTs, out[0].RankQueryWindowEndTs)
	}
}

// TestDisp3MergeSingleWindowKeepsRowWindow: the single-window merge keeps the
// row-level pair (legacy) — the chip lane then never needs the fallback.
func TestDisp3MergeSingleWindowKeepsRowWindow(t *testing.T) {
	nodes := []TraceCausalProjectionNode{
		disp3MergeMember("m1", 5, 8.611, 6793222.700, 6793222.901),
		disp3MergeMember("m2", 2, 9.169, 6793222.700, 6793222.901),
		disp3MergeMember("m3", 7, 8.633, 6793222.700, 6793222.901),
	}
	out := traceCausalProjectionAggregateSameKind(nodes)
	if len(out) != 1 {
		t.Fatalf("R2 must merge: %d rows", len(out))
	}
	if out[0].QueryWindowStartTs != 6793222.700 || out[0].QueryWindowEndTs != 6793222.901 {
		t.Fatalf("single-window merge keeps the row-level window: %.3f–%.3f",
			out[0].QueryWindowStartTs, out[0].QueryWindowEndTs)
	}
	if out[0].RankQueryWindowStartTs != 6793222.700 || out[0].RankQueryWindowEndTs != 6793222.901 {
		t.Fatalf("the rank window mirrors the supplying member: %.3f–%.3f",
			out[0].RankQueryWindowStartTs, out[0].RankQueryWindowEndTs)
	}
}

// TestDisp3OccurrenceMergeSharesRankWindowAuthority: the display trunk's ×2
// occurrence fold rides the SAME merge body (single authority — no drift).
func TestDisp3OccurrenceMergeSharesRankWindowAuthority(t *testing.T) {
	merged := TraceCausalProjectionMergeOccurrenceRows([]TraceCausalProjectionNode{
		disp3MergeMember("m1", 4, 8.611, 6793224.299, 6793224.501),
		disp3MergeMember("m2", 2, 9.169, 6793222.700, 6793222.901),
	})
	if merged.Rank != 2 {
		t.Fatalf("occurrence merge rank must be the member min: %d", merged.Rank)
	}
	if merged.RankQueryWindowStartTs != 6793222.700 || merged.RankQueryWindowEndTs != 6793222.901 {
		t.Fatalf("occurrence merge must keep the rank member's window: %.3f–%.3f",
			merged.RankQueryWindowStartTs, merged.RankQueryWindowEndTs)
	}
}

func disp3FoldMember(id, subject string, impact, winStart, winEnd float64) TraceCausalProjectionNode {
	node := TraceCausalProjectionNode{
		Role: TraceCausalRoleRootCauseContext, EvidenceID: id, Subject: subject,
		Object: "sleep_wait", ImpactMS: impact, CumulativeImpactMS: impact, Confidence: 0.6,
	}
	if winStart > 0 {
		node.QueryWindowStartTs, node.QueryWindowEndTs = winStart, winEnd
	}
	return node
}

// TestDisp3OverflowFoldCarriesMemberWindowRoster pins the E19 repair half in
// the constructor (§29.8 P3 "E19 跨窗折叠漏拒%", huadong_792 E19: an 11-member
// cross-window fold published "24%" against the single anchor window): the
// fold row carries the distinct member query windows so the display's
// runtimeTraceProjMultiWindowMergedRow gate (MergedCount>1 ∧ >1 roster
// windows) can suppress the share. Single-window and windowless folds keep an
// ≤1 roster — their legacy % stays byte-identical.
func TestDisp3OverflowFoldCarriesMemberWindowRoster(t *testing.T) {
	fold := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{
		disp3FoldMember("f1", "OS_mmi_EventHdr-43103", 5.335, 6793224.299, 6793224.501),
		disp3FoldMember("f2", "VSyncGenerator-2270", 48.518, 6793222.031, 6793225.370),
		disp3FoldMember("f3", "dh-irq-bind-0-89", 6.100, 6793222.700, 6793222.901),
	})
	if fold.MergedCount != 3 || fold.MergedMaxMS != 48.518 {
		t.Fatalf("fold accounting unchanged: count=%d max=%.3f", fold.MergedCount, fold.MergedMaxMS)
	}
	if len(fold.MergedQueryWindows) != 3 {
		t.Fatalf("the fold must carry the distinct member window roster: %+v", fold.MergedQueryWindows)
	}
	if fold.MergedQueryWindows[0].StartTs != 6793222.031 {
		t.Fatalf("roster must be ascending-start sorted: %+v", fold.MergedQueryWindows)
	}
	// Single-window members → roster of exactly one (the display % gate stays
	// closed and the legacy share renders).
	single := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{
		disp3FoldMember("s1", "a-1", 5.0, 100.0, 100.2),
		disp3FoldMember("s2", "b-2", 6.0, 100.0, 100.2),
	})
	if len(single.MergedQueryWindows) != 1 {
		t.Fatalf("single-window fold roster must have exactly one entry: %+v", single.MergedQueryWindows)
	}
	// Windowless members → empty roster (absence never guesses).
	bare := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{
		disp3FoldMember("w1", "a-1", 5.0, 0, 0),
		disp3FoldMember("w2", "b-2", 6.0, 0, 0),
	})
	if len(bare.MergedQueryWindows) != 0 {
		t.Fatalf("windowless fold roster must stay empty: %+v", bare.MergedQueryWindows)
	}
	// An absorbed member that is ITSELF a merged row contributes its own
	// roster (row-level pair already zeroed by the §11-N2 merge).
	preMerged := disp3FoldMember("p1", "c-3", 7.0, 0, 0)
	preMerged.MergedCount = 2
	preMerged.MergedQueryWindows = []TraceCausalProjectionQueryWindow{
		{StartTs: 200.0, EndTs: 200.1}, {StartTs: 201.0, EndTs: 201.1},
	}
	viaRoster := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{
		disp3FoldMember("s1", "a-1", 5.0, 200.0, 200.1),
		preMerged,
	})
	if len(viaRoster.MergedQueryWindows) != 2 {
		t.Fatalf("absorbed merged member's roster must fold in (F-2 dedupe): %+v", viaRoster.MergedQueryWindows)
	}
}
