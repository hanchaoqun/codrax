package tracequery

// rank_self_runnable_two_ruler_test.go — RULER2-1 engine pins (§29.150② user
// ruling / R-19-b, 2026-07-19; ledger §29.136 cb_rework P3③ 备案).
//
// Witness (production, verbatim donghu capture): the donghu17267 flagship
// board (default chain caps) publishes the target's own runnable account as
// THREE seats on TWO closed rulers — 3.956 (#5) + 1.193 (#14) on the self
// wall-clock ruler and 1.648 (#11) on the wakeup-edge ruler. The harvested
// record must carry exactly that split, with the same-ruler subtotal
// 3.956+1.193=5.149 as a µs identity. NO cross-ruler total exists anywhere
// (M3 禁混尺: the struct deliberately has no such field, and Σ6.797 never
// prints on any face — display-side inverse-ban pin).
//
// MUTATION self-checks (cp copies, serial):
//   - M1 (sentence-missing): disabling the harvest (or the display composer)
//     reds TestRuler2Donghu17267WitnessRecord / the display witness pin;
//   - M2 (subtotal-identity): drifting WallSubtotalMs at the harvest site
//     reds TestRuler2Donghu17267WitnessRecord (µs identity assert) and the
//     display witness (strict parser drops the record → sentence gone);
//   - M3 (cross-ruler Σ): the display inverse-ban pin owns it;
//   - M4 (double render): the display idempotence pin owns it.

import (
	"fmt"
	"testing"
)

func ruler2DonghuDefaultQuery() Query {
	return Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
}

// TestRuler2Donghu17267WitnessRecord — the production witness pin: the
// three-seat two-ruler split lands on the typed record verbatim, and the
// same-ruler subtotal holds to the µs.
func TestRuler2Donghu17267WitnessRecord(t *testing.T) {
	rank := BuildRootCauseRank(selfAllDonghuIndex(t), ruler2DonghuDefaultQuery())
	record := rank.SelfRunnableTwoRuler
	if record == nil {
		t.Fatalf("witness drifted: the 17267 default board must mint the two-ruler accounting record")
	}
	if record.Thread.PID != 17267 {
		t.Fatalf("the record's thread must be the analysis target: %+v", record.Thread)
	}
	fmtSeats := func(seats []SelfRunnableTwoRulerSeat) string {
		out := ""
		for _, seat := range seats {
			out += fmt.Sprintf("#%d=%.3f;", seat.Rank, seat.EffMs)
		}
		return out
	}
	if got := fmtSeats(record.WallSeats); got != "#6=3.956;#15=1.193;" {
		t.Fatalf("self wall-clock ruler seats drifted (want 3.956 #6 + 1.193 #15 after the full causal IO seat): %s", got)
	}
	if got := fmtSeats(record.EdgeSeats); got != "#13=1.648;" {
		t.Fatalf("wakeup-edge ruler seat drifted (want 1.648 #13 after the full causal IO seat): %s", got)
	}
	// 同尺小计 µs 恒等 (spec 定形1): 3.956+1.193=5.149 — the subtotal IS the
	// sum of the published seat values (identity by construction, asserted
	// against the print face too).
	wallSum := 0.0
	for _, seat := range record.WallSeats {
		wallSum += seat.EffMs
	}
	if diff := wallSum - record.WallSubtotalMs; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("same-ruler subtotal must equal the seat sum to the µs: Σ=%.9f subtotal=%.9f", wallSum, record.WallSubtotalMs)
	}
	if got := fmt.Sprintf("%.3f", record.WallSubtotalMs); got != "5.149" {
		t.Fatalf("witness wall subtotal must print 5.149 (=3.956+1.193): %s", got)
	}
	if got := fmt.Sprintf("%.3f", record.EdgeSubtotalMs); got != "1.648" {
		t.Fatalf("witness edge subtotal must print 1.648: %s", got)
	}
	// The lead seat (#5) grounds the wire record.
	if record.LineStart <= 0 || record.LineEnd < record.LineStart {
		t.Fatalf("the lead seat's line span must ground the record: %d..%d", record.LineStart, record.LineEnd)
	}
}

// TestRuler2SingleRulerBoardStaysSilent — the §29.136 single-ruler precedent
// (TestSelfAllChainBudgetDefaultTierSingleRulerFold 同款既裁): under the
// EXPLICIT legacy chain caps the same window's runnable account is ONE
// self-wall-clock seat (5.604) — a single-ruler board never mints the record
// (the existing same-ruler faces own that shape).
func TestRuler2SingleRulerBoardStaysSilent(t *testing.T) {
	rank := BuildRootCauseRank(selfAllDonghuIndex(t), selfAllDonghuQuery())
	sawSingle := false
	for _, item := range rank.Items {
		if item.Type == "runnable_wait" && item.SubjectIsAnalysisTarget && item.ChainRelevance == "on_chain" {
			sawSingle = true
			if item.Causality != RootCauseCausalitySelfWallClock {
				t.Fatalf("legacy-cap board drifted: the single runnable seat must ride the self wall-clock ruler: %+v", item)
			}
		}
	}
	if !sawSingle {
		t.Fatalf("legacy-cap board drifted: the 5.604 single-ruler runnable seat vanished")
	}
	if rank.SelfRunnableTwoRuler != nil {
		t.Fatalf("单尺多席不发此句: a single-ruler board must never mint the record: %+v", rank.SelfRunnableTwoRuler)
	}
}

func ruler2SyntheticSeat(pid, rank int, causality string, runnableMs float64) RootCauseRankItem {
	return RootCauseRankItem{
		Type: "runnable_wait", Thread: ThreadRef{Comm: "app", PID: pid},
		SubjectIsAnalysisTarget: true, ChainRelevance: "on_chain",
		Causality: causality, Rank: rank,
		DominantState: string(StateRunnable), RunnableMs: runnableMs,
		LineStart: 10 * rank, LineEnd: 10*rank + 1,
	}
}

// TestRuler2HarvestAdmissionConditions unit-pins the typed admission — each
// broken condition silences the WHOLE record (宁漏勿假指).
func TestRuler2HarvestAdmissionConditions(t *testing.T) {
	wall := ruler2SyntheticSeat(100, 2, RootCauseCausalitySelfWallClock, 3.0)
	wall2 := ruler2SyntheticSeat(100, 5, RootCauseCausalitySelfWallClock, 1.0)
	edge := ruler2SyntheticSeat(100, 4, rootCauseCausalityOnWakeupChain, 2.0)

	record := harvestSelfRunnableTwoRulerAccounting([]RootCauseRankItem{wall2, edge, wall})
	if record == nil {
		t.Fatalf("both rulers occupied must admit")
	}
	// Board order restored rank-asc regardless of scan order.
	if len(record.WallSeats) != 2 || record.WallSeats[0].Rank != 2 || record.WallSeats[1].Rank != 5 {
		t.Fatalf("wall seats must sort rank asc: %+v", record.WallSeats)
	}
	if record.WallSubtotalMs != 4.0 || record.EdgeSubtotalMs != 2.0 {
		t.Fatalf("same-ruler subtotals drifted: wall=%v edge=%v", record.WallSubtotalMs, record.EdgeSubtotalMs)
	}
	// Lead = the lowest ordinal across BOTH rulers (#2, the wall seat) —
	// its line span grounds the record.
	if record.LineStart != 20 || record.LineEnd != 21 {
		t.Fatalf("the lead seat's line span must ground the record: %d..%d", record.LineStart, record.LineEnd)
	}

	if harvestSelfRunnableTwoRulerAccounting([]RootCauseRankItem{wall, wall2}) != nil {
		t.Fatalf("单尺多席不发此句: a single-ruler multi-seat board must stay silent")
	}
	if harvestSelfRunnableTwoRulerAccounting([]RootCauseRankItem{edge}) != nil {
		t.Fatalf("a lone edge-ruler seat must stay silent")
	}
	zero := ruler2SyntheticSeat(100, 3, RootCauseCausalitySelfWallClock, 0)
	if harvestSelfRunnableTwoRulerAccounting([]RootCauseRankItem{wall, edge, zero}) != nil {
		t.Fatalf("各席发布值在场: a zero-value family seat must silence the whole record")
	}
	rankless := ruler2SyntheticSeat(100, 0, RootCauseCausalitySelfWallClock, 1.5)
	if harvestSelfRunnableTwoRulerAccounting([]RootCauseRankItem{wall, edge, rankless}) != nil {
		t.Fatalf("an ordinal-less family seat must silence the whole record")
	}
	foreign := ruler2SyntheticSeat(100, 6, "adjacent_to_wakeup_chain", 1.5)
	if harvestSelfRunnableTwoRulerAccounting([]RootCauseRankItem{wall, edge, foreign}) != nil {
		t.Fatalf("closed set: a family seat outside the two rulers must silence the whole record")
	}
	otherPid := ruler2SyntheticSeat(200, 7, rootCauseCausalityOnWakeupChain, 1.5)
	if harvestSelfRunnableTwoRulerAccounting([]RootCauseRankItem{wall, edge, otherPid}) != nil {
		t.Fatalf("family seats across two pids must silence the whole record")
	}
	// Non-family rows never participate and never abort: a NON-target
	// runnable row and an adjacent-lane self runnable row are both outside
	// the family (the two rulers are on-chain proof lanes of the target).
	nonTarget := ruler2SyntheticSeat(300, 8, "on_wakeup_chain", 9.0)
	nonTarget.SubjectIsAnalysisTarget = false
	adjacentSelf := ruler2SyntheticSeat(100, 9, "adjacent_to_wakeup_chain", 9.0)
	adjacentSelf.ChainRelevance = "adjacent"
	record = harvestSelfRunnableTwoRulerAccounting([]RootCauseRankItem{wall, edge, nonTarget, adjacentSelf})
	if record == nil || len(record.WallSeats) != 1 || len(record.EdgeSeats) != 1 {
		t.Fatalf("non-family rows must neither join nor abort: %+v", record)
	}
}
