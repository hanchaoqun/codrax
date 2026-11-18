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

// hmc376BoardPart reads one board's recorded part of a merged row.
func hmc376BoardPart(t *testing.T, row TraceCausalProjectionNode, fingerprint string) TraceRankBoardPartAccount {
	t.Helper()
	for _, part := range row.PublishedRankBoardParts() {
		if part.ParamsFingerprint == fingerprint {
			return part
		}
	}
	t.Fatalf("no part recorded for board %q: %+v", fingerprint, row.RankBoardPartAccounts)
	return TraceRankBoardPartAccount{}
}

// Three boards measured one wait at their own thresholds. The ×3 row adds all
// three measurements, which is what the row displays and what no board
// published: each board keeps its own part, whichever member seeds the row.
func TestCrossBoardMergedRowKeepsEachBoardsOwnPart(t *testing.T) {
	member := func(id, fingerprint string, impactMS float64, line int) TraceCausalProjectionNode {
		node := hmc376BoardNode(id, fingerprint, impactMS)
		node.EffectiveImpactMS = impactMS
		node.LineStart, node.LineEnd = line, line+10
		return node
	}
	for _, order := range [][]int{{0, 1, 2}, {2, 1, 0}, {1, 2, 0}} {
		all := []TraceCausalProjectionNode{member("fine", "fine", 40.071, 100), member("mid", "mid", 33.489, 200), member("coarse", "coarse", 17.815, 300)}
		rows := make([]TraceCausalProjectionNode, 0, len(all))
		for _, i := range order {
			rows = append(rows, all[i])
		}
		merged := traceCausalProjectionMergeSameKindMembers(rows, 0, []int{0, 1, 2})
		if got := traceSameFactAccountKey(merged.ImpactMS); got != "91.375" {
			t.Fatalf("%v: control: the merged row displays the member sum, got %s", order, got)
		}
		for _, want := range []struct {
			board string
			ms    float64
		}{{"fine", 40.071}, {"mid", 33.489}, {"coarse", 17.815}} {
			part := hmc376BoardPart(t, merged, want.board)
			if part.AccountMS != want.ms || part.CumulativeMS != want.ms || part.EffectiveMS != want.ms {
				t.Fatalf("%v: board %s must keep its own %.3f, got %+v", order, want.board, want.ms, part)
			}
		}
		if len(merged.CoPublishedRankBoards()) != 0 {
			t.Fatalf("%v: no board published the merged total: %+v", order, merged.CoPublishedRankBoards())
		}
		// A later change of the row's value retires the split.
		merged.ImpactMS = 50
		if parts := merged.PublishedRankBoardParts(); len(parts) != 0 {
			t.Fatalf("%v: a split speaks only for the value it was recorded for: %+v", order, parts)
		}
	}
}

// A board's part of a merge goes through the merge's own value calibers, and a
// fact two boards share counts on both.
func TestCrossBoardMergedRowPartOfSharedAndOwnMembers(t *testing.T) {
	shared := hmc376BoardNode("shared", "fine", 10)
	shared.LineStart, shared.LineEnd = 100, 110
	twin := hmc376BoardNode("shared-twin", "coarse", 10)
	twin.LineStart, twin.LineEnd = 100, 110
	traceCausalProjectionAbsorbSameFactRankBoards(&shared, twin)
	fineOnly := hmc376BoardNode("fine-only", "fine", 11)
	fineOnly.LineStart, fineOnly.LineEnd = 200, 210
	coarseOnly := hmc376BoardNode("coarse-only", "coarse", 12)
	coarseOnly.LineStart, coarseOnly.LineEnd = 300, 310
	merged := traceCausalProjectionMergeSameKindMembers([]TraceCausalProjectionNode{shared, fineOnly, coarseOnly}, 0, []int{0, 1, 2})
	if merged.ImpactMS != 33 {
		t.Fatalf("control: the merged row displays the member sum, got %.3f", merged.ImpactMS)
	}
	if part := hmc376BoardPart(t, merged, "fine"); part.AccountMS != 21 {
		t.Fatalf("the fine board published 10 + 11, got %.3f", part.AccountMS)
	}
	if part := hmc376BoardPart(t, merged, "coarse"); part.AccountMS != 22 {
		t.Fatalf("the coarse board published 10 + 12, got %.3f", part.AccountMS)
	}
}

// Members without a rank board belong to every board's account, including a
// board that published nothing else on this row.
func TestCrossBoardMergedRowUnclaimedPartReachesEveryBoard(t *testing.T) {
	named := hmc376BoardNode("named", "fine", 10)
	named.LineStart, named.LineEnd = 100, 110
	bare := hmc376BoardNode("bare", "", 5)
	bare.RankBoardTarget = ""
	bare.Rank = 0
	bare.LineStart, bare.LineEnd = 200, 210
	merged := traceCausalProjectionMergeSameKindMembers([]TraceCausalProjectionNode{named, bare}, 0, []int{0, 1})
	if part := hmc376BoardPart(t, merged, "fine"); part.AccountMS != 15 {
		t.Fatalf("the named board keeps the unclaimed member beside its own, got %.3f", part.AccountMS)
	}
	unclaimed := false
	for _, part := range merged.PublishedRankBoardParts() {
		if part.Target == "" {
			unclaimed = true
			if part.AccountMS != 5 {
				t.Fatalf("the unclaimed part is the identity-less member alone, got %.3f", part.AccountMS)
			}
		}
	}
	if !unclaimed {
		t.Fatalf("the part no board claims must be recorded: %+v", merged.PublishedRankBoardParts())
	}
}

// One board publishing every member keeps the legacy shape: no split at all.
func TestSingleBoardMergedRowRecordsNoParts(t *testing.T) {
	a, b, c := hmc376BoardNode("a", "fine", 10), hmc376BoardNode("b", "fine", 11), hmc376BoardNode("c", "fine", 12)
	a.LineStart, a.LineEnd = 100, 110
	b.LineStart, b.LineEnd = 200, 210
	c.LineStart, c.LineEnd = 300, 310
	merged := traceCausalProjectionMergeSameKindMembers([]TraceCausalProjectionNode{a, b, c}, 0, []int{0, 1, 2})
	if len(merged.RankBoardPartAccounts) != 0 {
		t.Fatalf("a single-board merge records no split: %+v", merged.RankBoardPartAccounts)
	}
}

// The duplicate-publication fold joins publications whose line spans overlap
// without being equal. The absorbed board keeps its membership in both arrival
// orders; the near-value lane re-values the row and stays out.
func TestDuplicatePublicationFoldKeepsTheAbsorbedBoard(t *testing.T) {
	publication := func(id, fingerprint string, impactMS float64, start, end int) TraceCausalProjectionNode {
		node := hmc376BoardNode(id, fingerprint, impactMS)
		node.LineStart, node.LineEnd = start, end
		node.Object = "peer-5" // the near-value lane needs a resolved peer
		return node
	}
	for _, order := range [][2]string{{"fine", "coarse"}, {"coarse", "fine"}} {
		rows := traceCausalProjectionDedupDuplicatePublications([]TraceCausalProjectionNode{
			publication("a", order[0], 30, 100, 200), publication("b", order[1], 30, 120, 220),
		})
		if len(rows) != 1 || rows[0].DuplicatePublications == 0 {
			t.Fatalf("%v: control: overlapping equal publications fold into one row, got %d rows", order, len(rows))
		}
		boards := rows[0].CoPublishedRankBoards()
		if len(boards) != 1 || boards[0].ParamsFingerprint != order[1] {
			t.Fatalf("%v: the absorbed board must stay a publisher of this value, got %+v", order, boards)
		}
	}
	near := traceCausalProjectionDedupDuplicatePublications([]TraceCausalProjectionNode{
		publication("a", "fine", 30, 100, 200), publication("b", "coarse", 30.2, 120, 220),
	})
	if len(near) != 1 || near[0].ImpactMS != 30.2 {
		t.Fatalf("control: near values fold onto the larger estimate, got %d rows", len(near))
	}
	if boards := near[0].CoPublishedRankBoards(); len(boards) != 1 || boards[0].ParamsFingerprint != "coarse" {
		t.Fatalf("only the board that published the row's displayed value is a publisher of it, got %+v", boards)
	}
}
