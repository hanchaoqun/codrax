package types

import "testing"

func hmc376BoardNode(id, fingerprint string, impactMS float64) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		EvidenceID: id, Role: TraceCausalRoleRootCauseContext, Subject: "worker-10", Predicate: "root_cause_tertiary",
		StateKind: "runnable", ImpactMS: impactMS, CumulativeImpactMS: impactMS, Confidence: 0.6, Rank: 2,
		LineStart: 100, LineEnd: 900, ChainRelevance: "on_chain",
		RankBoardTarget: "worker-10", RankBoardParamsFingerprint: fingerprint,
		QueryWindowStartTs: 10, QueryWindowEndTs: 11,
	}
}

func hmc376MergeOneBucket(nodes ...TraceCausalProjectionNode) []TraceCausalProjectionNode {
	out := TraceCausalProjection{OnChainCauses: nodes}
	traceCausalProjectionMergeSameFacts(&out)
	return out.OnChainCauses
}

// A fact two rank boards published folds into one row; the absorbed
// publication's board keeps its membership of that account, whichever
// publication survives.
func TestSameFactMergeKeepsTheAbsorbedPublicationsRankBoard(t *testing.T) {
	for _, order := range [][2]string{{"fine", "coarse"}, {"coarse", "fine"}} {
		rows := hmc376MergeOneBucket(hmc376BoardNode("a", order[0], 31.728), hmc376BoardNode("b", order[1], 31.728))
		if len(rows) != 1 {
			t.Fatalf("%v: one fact, one row; got %d", order, len(rows))
		}
		row := rows[0]
		if row.RankBoardParamsFingerprint != order[0] {
			t.Fatalf("%v: the survivor keeps its own board identity, got %q", order, row.RankBoardParamsFingerprint)
		}
		boards := row.CoPublishedRankBoards()
		if len(boards) != 1 || boards[0].ParamsFingerprint != order[1] || boards[0].Target != "worker-10" ||
			boards[0].QueryWindowStartTs != 10 || boards[0].QueryWindowEndTs != 11 || boards[0].AccountMS != 31.728 {
			t.Fatalf("%v: the absorbed board must stay a publisher of this account, got %+v", order, boards)
		}
	}
}

func TestSameFactMergeRankBoardMembershipFailsClosed(t *testing.T) {
	// Same board on both publications: nothing to add.
	if rows := hmc376MergeOneBucket(hmc376BoardNode("a", "fine", 31.728), hmc376BoardNode("b", "fine", 31.728)); len(rows) != 1 || len(rows[0].CoPublishedRankBoards()) != 0 {
		t.Fatalf("a row never lists its own board: %+v", rows)
	}
	// An identity-less publication names no board (absence never claims one).
	bare := hmc376BoardNode("b", "", 31.728)
	bare.RankBoardTarget = ""
	if rows := hmc376MergeOneBucket(hmc376BoardNode("a", "fine", 31.728), bare); len(rows) != 1 || len(rows[0].CoPublishedRankBoards()) != 0 {
		t.Fatalf("an identity-less twin must not mint a board: %+v", rows)
	}
	// A twin that describes the fact with ANOTHER account (a re-anchored
	// half: the identity reads its full account, the row publishes the
	// anchored one) published a different number on its board.
	anchored := hmc376BoardNode("a", "fine", 12.000)
	anchored.ChainAnchorFullMS, anchored.ChainAnchoredMS = 31.728, 12.000
	rows := hmc376MergeOneBucket(anchored, hmc376BoardNode("b", "coarse", 31.728))
	if len(rows) != 1 {
		t.Fatalf("control: the anchored half and its full-account twin are one fact, got %d rows", len(rows))
	}
	if boards := rows[0].CoPublishedRankBoards(); len(boards) != 0 {
		t.Fatalf("a board that published 31.728 must not be lent the row's 12.000: %+v", boards)
	}
	// A later change of the row's value retires every inherited entry.
	rows = hmc376MergeOneBucket(hmc376BoardNode("a", "fine", 31.728), hmc376BoardNode("b", "coarse", 31.728))
	rows[0].ImpactMS = 40
	if boards := rows[0].CoPublishedRankBoards(); len(boards) != 0 {
		t.Fatalf("an entry speaks only for the value it vouched for: %+v", boards)
	}
	// Equal value on different evidence lines is not the same fact: no fold,
	// no membership.
	other := hmc376BoardNode("b", "coarse", 31.728)
	other.LineStart, other.LineEnd = 2000, 2100
	if rows := hmc376MergeOneBucket(hmc376BoardNode("a", "fine", 31.728), other); len(rows) != 2 ||
		len(rows[0].CoPublishedRankBoards()) != 0 || len(rows[1].CoPublishedRankBoards()) != 0 {
		t.Fatalf("equal numbers alone are never one fact: %+v", rows)
	}
}

// A merged ×N row is published by a board only when that board published
// every member; the seed member's membership is never inherited as it stands.
func TestMergedRowRankBoardMembershipNeedsEveryMember(t *testing.T) {
	member := func(id string, impactMS float64, lines int, coPublished bool) TraceCausalProjectionNode {
		node := hmc376BoardNode(id, "fine", impactMS)
		node.LineStart, node.LineEnd = lines, lines+10
		if coPublished {
			twin := hmc376BoardNode(id+"-twin", "coarse", impactMS)
			twin.LineStart, twin.LineEnd = lines, lines+10
			traceCausalProjectionAbsorbSameFactRankBoards(&node, twin)
		}
		return node
	}
	all := []TraceCausalProjectionNode{member("a", 10, 100, true), member("b", 11, 200, true), member("c", 12, 300, true)}
	merged := traceCausalProjectionMergeSameKindMembers(all, 0, []int{0, 1, 2})
	if merged.ImpactMS != 33 {
		t.Fatalf("control: the merged row carries the member sum, got %.3f", merged.ImpactMS)
	}
	coarse := 0
	for _, board := range merged.CoPublishedRankBoards() {
		if board.ParamsFingerprint == "coarse" {
			coarse++
			if board.AccountMS != 33 {
				t.Fatalf("the membership vouches for the merged account, got %.3f", board.AccountMS)
			}
		}
	}
	if coarse != 1 {
		t.Fatalf("a board that published every member publishes the merged account: %+v", merged.CoPublishedRankBoards())
	}

	partial := []TraceCausalProjectionNode{member("a", 10, 100, true), member("b", 11, 200, false), member("c", 12, 300, true)}
	merged = traceCausalProjectionMergeSameKindMembers(partial, 0, []int{0, 1, 2})
	for _, board := range merged.CoPublishedRankBoards() {
		if board.ParamsFingerprint == "coarse" {
			t.Fatalf("one member the board never published breaks its claim on the total: %+v", merged.CoPublishedRankBoards())
		}
	}
}
