package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// hmc376SelfRow is one target-own runnable account row wearing a rank board.
func hmc376SelfRow(id, fingerprint string, impactMS float64, coPublishedBy ...string) runtimeTraceProjTreeRow {
	node := types.TraceCausalProjectionNode{
		EvidenceID: id, Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-10", Predicate: "root_cause_tertiary",
		StateKind: "runnable", ImpactMS: impactMS, Rank: 1,
		RankBoardTarget: "worker-10", RankBoardParamsFingerprint: fingerprint,
		QueryWindowStartTs: 10, QueryWindowEndTs: 11,
	}
	for _, other := range coPublishedBy {
		node.SameFactRankBoards = append(node.SameFactRankBoards, types.TraceSameFactRankBoard{
			Target: "worker-10", ParamsFingerprint: other,
			QueryWindowStartTs: 10, QueryWindowEndTs: 11, AccountMS: impactMS,
		})
	}
	return runtimeTraceProjTreeRow{Node: node, HasData: true}
}

func hmc376Admitted(model runtimeTraceProjTreeModel) (float64, []bool) {
	total, admitted, _, _ := runtimeTraceProjTargetSymptomAdmissionWithEligibility(model, nil)
	return total, admitted
}

// The fine board published five accounts (171 ms); the coarse board published
// four of the same facts plus one aggregate of its own (148 ms). Whichever
// publication kept the four shared rows, the denominator is the fine board's
// own complete account and only the coarse board's own aggregate is left out.
func TestHMC376SymptomDenominatorIsTheLargestBoardsOwnAccount(t *testing.T) {
	type shape struct {
		name string
		rows []runtimeTraceProjTreeRow
		// index of the coarse board's own aggregate row
		aggregate int
	}
	shapes := []shape{
		{name: "fine publications kept the shared rows", aggregate: 5, rows: []runtimeTraceProjTreeRow{
			hmc376SelfRow("f1", "fine", 48, "coarse"), hmc376SelfRow("f2", "fine", 40),
			hmc376SelfRow("f3", "fine", 32, "coarse"), hmc376SelfRow("f4", "fine", 18, "coarse"),
			hmc376SelfRow("f5", "fine", 33, "coarse"), hmc376SelfRow("c4", "coarse", 17),
		}},
		{name: "coarse publications kept the shared rows", aggregate: 5, rows: []runtimeTraceProjTreeRow{
			hmc376SelfRow("c1", "coarse", 48, "fine"), hmc376SelfRow("f2", "fine", 40),
			hmc376SelfRow("c2", "coarse", 32, "fine"), hmc376SelfRow("c3", "coarse", 18, "fine"),
			hmc376SelfRow("c5", "coarse", 33, "fine"), hmc376SelfRow("c4", "coarse", 17),
		}},
		{name: "the shared rows are split between the publications", aggregate: 5, rows: []runtimeTraceProjTreeRow{
			hmc376SelfRow("c1", "coarse", 48, "fine"), hmc376SelfRow("f2", "fine", 40),
			hmc376SelfRow("f3", "fine", 32, "coarse"), hmc376SelfRow("f4", "fine", 18, "coarse"),
			hmc376SelfRow("c5", "coarse", 33, "fine"), hmc376SelfRow("c4", "coarse", 17),
		}},
	}
	for _, tc := range shapes {
		t.Run(tc.name, func(t *testing.T) {
			model := runtimeTraceProjTreeModel{SelfRows: tc.rows}
			total, admitted := hmc376Admitted(model)
			if total != 171 {
				t.Fatalf("the denominator must be the fine board's own account 48+40+32+18+33 = 171, got %.3f", total)
			}
			for i, ok := range admitted {
				if want := i != tc.aggregate; ok != want {
					t.Fatalf("row %d (%s) admitted=%v, want %v", i, tc.rows[i].Node.EvidenceID, ok, want)
				}
			}
			// The denominator names the board that owns it — never the
			// identity a folded row happens to wear.
			board, ok := runtimeTraceProjSymptomDenominatorBoard(model)
			if !ok || board.RankBoardParamsFingerprint != "fine" || board.RankBoardTarget != "worker-10" {
				t.Fatalf("the denominator names the fine board, got %q/%q ok=%v", board.RankBoardTarget, board.RankBoardParamsFingerprint, ok)
			}
			scope := runtimeTraceProjCoverageBoardScope{multi: true}
			scope.adoptBoardOf(board)
			if !scope.subjectOK || scope.fp != "fine" || !scope.winOK || scope.winStart != 10 || scope.winEnd != 11 {
				t.Fatalf("the coverage subject board must be the fine board with its own window, got %+v", scope)
			}
		})
	}
}

// Negative controls: membership is the typed roster only.
func TestHMC376SymptomBoardMembershipIsTypedOnly(t *testing.T) {
	// No folded cross-board publication: the legacy split by worn identity is
	// unchanged (the larger board stays, the other board's rows leave).
	legacy := runtimeTraceProjTreeModel{SelfRows: []runtimeTraceProjTreeRow{
		hmc376SelfRow("f1", "fine", 48), hmc376SelfRow("f2", "fine", 40), hmc376SelfRow("c1", "coarse", 48), hmc376SelfRow("c4", "coarse", 17),
	}}
	if total, admitted := hmc376Admitted(legacy); total != 88 || !admitted[0] || !admitted[1] || admitted[2] || admitted[3] {
		t.Fatalf("legacy two-board split must stay: total=%.3f admitted=%v", total, admitted)
	}
	// An equal value on the other board is not a publication of this row:
	// without the roster entry nothing joins (equal numbers are never
	// identity).
	if total, _ := hmc376Admitted(runtimeTraceProjTreeModel{SelfRows: []runtimeTraceProjTreeRow{
		hmc376SelfRow("f1", "fine", 48), hmc376SelfRow("c1", "coarse", 48), hmc376SelfRow("c2", "coarse", 10),
	}}); total != 58 {
		t.Fatalf("the coarse board (58) wins on its own rows; the fine row is not lent to it: %.3f", total)
	}
	// A roster entry that vouched for another value is retired.
	stale := hmc376SelfRow("c1", "coarse", 48, "fine")
	stale.Node.SameFactRankBoards[0].AccountMS = 47
	if total, _ := hmc376Admitted(runtimeTraceProjTreeModel{SelfRows: []runtimeTraceProjTreeRow{
		stale, hmc376SelfRow("f2", "fine", 40), hmc376SelfRow("c4", "coarse", 17),
	}}); total != 65 {
		t.Fatalf("a stale entry must not lend the row to the fine board: %.3f", total)
	}
	// Another window is another board, even with equal target and params.
	otherWindow := hmc376SelfRow("c1", "coarse", 48, "fine")
	otherWindow.Node.SameFactRankBoards[0].QueryWindowStartTs, otherWindow.Node.SameFactRankBoards[0].QueryWindowEndTs = 20, 21
	model := runtimeTraceProjTreeModel{SelfRows: []runtimeTraceProjTreeRow{otherWindow, hmc376SelfRow("f2", "fine", 40), hmc376SelfRow("c4", "coarse", 17)}}
	if total, _ := hmc376Admitted(model); total != 65 {
		t.Fatalf("a publication on another window's board must not join this window's fine board: %.3f", total)
	}
	// A single named board never excludes anything.
	single := runtimeTraceProjTreeModel{SelfRows: []runtimeTraceProjTreeRow{hmc376SelfRow("f1", "fine", 48), hmc376SelfRow("f2", "fine", 40)}}
	if total, admitted := hmc376Admitted(single); total != 88 || !admitted[0] || !admitted[1] {
		t.Fatalf("single-board shape must stay whole: total=%.3f admitted=%v", total, admitted)
	}
	// A row whose every publication sits on the same board as the others is
	// not a second board.
	folded := runtimeTraceProjTreeModel{SelfRows: []runtimeTraceProjTreeRow{hmc376SelfRow("f1", "fine", 48, "fine"), hmc376SelfRow("f2", "fine", 40)}}
	if total, admitted := hmc376Admitted(folded); total != 88 || !admitted[0] || !admitted[1] {
		t.Fatalf("a redundant own-board entry must not split the board: total=%.3f admitted=%v", total, admitted)
	}
}

// hmc376SplitRow is a merged row of several boards: it displays the sum of the
// members and records what each board published of it.
func hmc376SplitRow(id, worn string, parts map[string]float64) runtimeTraceProjTreeRow {
	row := hmc376SelfRow(id, worn, 0)
	total := 0.0
	for _, ms := range parts {
		total += ms
	}
	row.Node.ImpactMS, row.Node.EffectiveImpactMS, row.Node.MergedCount = total, total, len(parts)
	for fingerprint, ms := range parts {
		row.Node.RankBoardPartAccounts = append(row.Node.RankBoardPartAccounts, types.TraceRankBoardPartAccount{
			TraceSameFactRankBoard: types.TraceSameFactRankBoard{Target: "worker-10", ParamsFingerprint: fingerprint,
				QueryWindowStartTs: 10, QueryWindowEndTs: 11, AccountMS: ms},
			EffectiveMS: ms, RowMS: total,
		})
	}
	return row
}

// A merge across boards counts for the winning board's own part. The row's total
// is what no board published, whichever board's identity the row wears.
func TestHMC376SymptomDenominatorReadsTheWinningBoardsPartOfAMerge(t *testing.T) {
	for _, worn := range []string{"fine", "mid", "coarse"} {
		model := runtimeTraceProjTreeModel{SelfRows: []runtimeTraceProjTreeRow{
			hmc376SelfRow("shared", "fine", 48, "mid", "coarse"),
			hmc376SplitRow("merged", worn, map[string]float64{"fine": 40, "mid": 33, "coarse": 17}),
		}}
		total, admitted := hmc376Admitted(model)
		if total != 88 || !admitted[0] || !admitted[1] {
			t.Fatalf("worn by %s: the denominator is the fine board's 48 + 40, got %.3f admitted=%v", worn, total, admitted)
		}
		board, ok := runtimeTraceProjSymptomDenominatorBoard(model)
		if !ok || board.RankBoardParamsFingerprint != "fine" {
			t.Fatalf("worn by %s: the fine board owns the denominator, got %q ok=%v", worn, board.RankBoardParamsFingerprint, ok)
		}
	}
	// A split recorded for another row value no longer speaks for this row.
	stale := hmc376SplitRow("merged", "fine", map[string]float64{"fine": 40, "coarse": 17})
	stale.Node.ImpactMS = 60
	if total, _ := hmc376Admitted(runtimeTraceProjTreeModel{SelfRows: []runtimeTraceProjTreeRow{stale}}); total != 60 {
		t.Fatalf("a retired split falls back to the row's own value, got %.3f", total)
	}
}

// The over-window sum and the coverage scope read the same per-board accounts.
func TestHMC376BoardEffectiveSumsReadEachBoardsOwnAccount(t *testing.T) {
	for _, worn := range []string{"fine", "coarse"} {
		shared := hmc376SelfRow("shared", worn, 30, map[string]string{"fine": "coarse", "coarse": "fine"}[worn])
		shared.Node.EffectiveImpactMS = 30
		split := hmc376SplitRow("merged", worn, map[string]float64{"fine": 40, "coarse": 17})
		rows := []*runtimeTraceProjTreeRow{&shared, &split}
		sums := runtimeTraceProjBoardEffectiveSums(rows, runtimeTraceProjStableRankBoardIDs(rows))
		if len(sums) != 2 {
			t.Fatalf("worn by %s: two boards published these seats, got %v", worn, sums)
		}
		largest := sums[0]
		if sums[1] > largest {
			largest = sums[1]
		}
		if largest != 70 {
			t.Fatalf("worn by %s: the largest board's own sum is 30 + 40, got %v", worn, sums)
		}
	}
}
