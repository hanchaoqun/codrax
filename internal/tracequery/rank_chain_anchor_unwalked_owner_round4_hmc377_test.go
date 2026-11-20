package tracequery

import (
	"fmt"
	"strings"
	"testing"
)

// HMC377 fix round 4 — the three findings review round 3 reproduced on the
// round-3 ledger, as real tests (the review's evidence tests, renamed; where
// an assertion encoded the defect's own premise, the property it guarded is
// asserted instead and the comment says so).

// hmc377R4Dump renders thread 200's rows for failure messages.
func hmc377R4Dump(items []RootCauseRankItem) string {
	var b strings.Builder
	for _, it := range items {
		if it.Thread.PID != 200 {
			continue
		}
		fmt.Fprintf(&b, "  %-30s src=%-28s rel=%-9s basis=%-26s run=%.3f d=%.3f io=%.3f gated=%.3f anch=%.3f unw=%.3f out=%.3f full=%.3f rem=%t own=%t held=%.3f owned=%t cpuk=%q conf=%.2f\n     %s\n",
			it.Type, it.Source, it.ChainRelevance, it.OnChainBasis, it.RunnableMs, it.DStateMs, it.IOWaitMs, it.GatedRunnableMs,
			it.ChainAnchoredMs, it.ChainAnchorUnwalkedMs, it.ChainAnchorOutsideMs, it.ChainAnchorFullMs, it.ChainAnchorRemainderSeat,
			it.ChainAnchorUnwalkedSeat, it.rspaUnwalkedHeldMs, it.rspaUnwalkedShareOwned, it.CPUConstraintKind, it.Confidence, it.Summary)
	}
	return b.String()
}

// hmc377R4DIOUniverse (review round 3, rv3DIOUniverse): thread 200 has D time
// inside the WALKED wait (2 ms, the D partition seat) and IO time inside the
// UNWALKED wait (3 ms, the io_wait seat, whose own walked share is ZERO), plus
// 1 ms D and 1 ms IO outside. Raw inside walked ∪ unwalked = 5 ms. chainDMs > 0
// publishes a chain-lane D seat of that value (case A' when it diverges from
// the 2 ms census anchored share).
func hmc377R4DIOUniverse(chainDMs float64) (ChainResult, WindowStats, []RootCauseRankItem) {
	chain, stats, _ := hmc377UnwalkedChain()
	member := ThreadRef{Comm: "dep", PID: 200}
	d := []foldInterval{{start: 1.020, end: 1.022, valueMs: 2}, {start: 1.070, end: 1.071, valueMs: 1}}
	io := []foldInterval{{start: 1.054, end: 1.057, valueMs: 3}, {start: 1.080, end: 1.081, valueMs: 1}}
	stats.runnableCensus = nil
	stats.dstateCensus = map[string]ThreadDuration{"200|0": {Thread: member, DurationMs: 3, anchoredMs: 2, dioIntervals: d}}
	stats.iowaitCensus = map[string]ThreadDuration{"200|0": {Thread: member, DurationMs: 4, anchoredMs: 0, dioIntervals: io}}
	chain.CausalImpacts = []WakeupCausalImpact{{Thread: member, ChainDepth: 1, DStateMs: chainDMs, Window: TimeWindow{StartTs: 1.000, EndTs: 1.030}}}
	dseat := RootCauseRankItem{Type: "d_state_or_io_wait", Thread: member, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
		DominantState: string(StateDSleep), DStateMs: 3, ImpactMs: 3, CumulativeImpactMs: 3, EffectiveImpactMs: 3,
		Source: "window_stats", Confidence: 0.8, LineStart: 20, LineEnd: 71, StartTs: 1.020, EndTs: 1.071,
		StatsWindowStartTs: 1.0, StatsWindowEndTs: 1.1, ledgerAnchorStamped: true, ledgerAnchoredDMs: 2,
		dioSegmentIntervalsD: d, dioSegmentIntervals: append([]foldInterval(nil), d...)}
	ioseat := RootCauseRankItem{Type: "io_wait", Thread: member, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
		DominantState: string(StateIOWait), IOWaitMs: 4, ImpactMs: 4, CumulativeImpactMs: 4, EffectiveImpactMs: 4,
		Source: "window_stats.io_wait_top", Confidence: 0.8, LineStart: 54, LineEnd: 81, StartTs: 1.054, EndTs: 1.081,
		StatsWindowStartTs: 1.0, StatsWindowEndTs: 1.1, ledgerAnchorStamped: true,
		dioSegmentIntervalsIO: io, dioSegmentIntervals: append([]foldInterval(nil), io...)}
	twin := RootCauseRankItem{Type: "fragmented_d_state_or_io_wait", Thread: member, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
		DominantState: string(StateDSleep), DStateMs: 3, IOWaitMs: 4, ImpactMs: 7, CumulativeImpactMs: 7, EffectiveImpactMs: 7,
		Source: "window_stats.state_churn", Confidence: 0.7}
	items := []RootCauseRankItem{dseat, ioseat, twin}
	if chainDMs > 0 {
		items = append([]RootCauseRankItem{hmc377R4DIOChainSeat(chainDMs)}, items...)
	}
	return chain, stats, items
}

func hmc377R4DIOChainSeat(ms float64) RootCauseRankItem {
	return RootCauseRankItem{Type: "d_state_or_io_wait", Thread: ThreadRef{Comm: "dep", PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
		DominantState: string(StateDSleep), DStateMs: ms, ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms,
		Source: "wakeup_chain.causal_impacts", Confidence: 0.9, LineStart: 20, LineEnd: 30, StartTs: 1.0, EndTs: 1.03, ChainDepth: 1}
}

// hmc377R4DIOCheck: per thread, Σ on-chain priced D/IO ≤ the 5 ms raw inside
// the walked ∪ unwalked waits, and at most one on-chain seat prices the share
// inside the unwalked wait (the own seat or a residual-lane seat).
func hmc377R4DIOCheck(t *testing.T, board []RootCauseRankItem) {
	t.Helper()
	onChain, owners := 0.0, 0
	for _, it := range board {
		if it.Thread.PID != 200 || !rootCauseItemIsOnChain(it) {
			continue
		}
		onChain += it.DStateMs + it.IOWaitMs
		if it.ChainAnchorUnwalkedSeat || it.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState {
			owners++
		}
	}
	if onChain > 5+rspaAnchorIdentityTolMs {
		t.Errorf("on-chain D/IO Σ for thread 200 = %.3f ms > 5 ms inside the walked and unwalked waits it ended:\n%s", onChain, hmc377R4Dump(board))
	}
	if owners > 1 {
		t.Errorf("%d on-chain seats price the 3 ms IO inside the unwalked wait:\n%s", owners, hmc377R4Dump(board))
	}
}

// R4-1, case A' (review: TestRv3DIOCaseAPrimeZeroWalkedSeat). The chain lane
// keeps its own diverging 1.5 ms D seat; the planner makes the io_wait seat
// (walked share zero) give its 3 ms to the thread's own seat. Round 3's arm
// handed the share over only when the row's walked share was positive: the
// io_wait seat kept it, became a zero-anchored whole remainder and the
// STATERES-1 residual lane priced the same 3 ms again (Σ 7.5 ms vs 5 ms raw).
func TestHMC377ZeroWalkedDIOSeatGivesItsShareToTheOwnSeatBesideADivergingChainSeat(t *testing.T) {
	chain, stats, items := hmc377R4DIOUniverse(1.5)
	board := hmc377UnwalkedBoard(chain, stats, items)
	rspaPatchSummariesForTwinVisibility(board)
	hmc377R4DIOCheck(t, board)
	for _, it := range board {
		if it.Type == "io_wait" && it.ChainAnchorRemainderSeat {
			if !hmc377UwNear(it.IOWaitMs, 1) || !hmc377UwNear(it.ChainAnchorUnwalkedMs, 3) || !hmc377UwNear(it.ChainAnchorOutsideMs, 1) ||
				!hmc377UwNear(it.ChainAnchoredMs+it.ChainAnchorUnwalkedMs+it.ChainAnchorOutsideMs, it.ChainAnchorFullMs) {
				t.Errorf("the io_wait seat applies the planner's ruling: 1 ms outside, its 3 ms named as the own seat's (4 = 0 + 3 + 1): %+v", it)
			}
		}
	}
}

// R4-1, case B (review: TestRv3DIOCaseBZeroWalkedSeat). No chain-lane D seat:
// the clipped seats hold walked + unwalked as one object — the D seat the
// walked 2 ms, the io_wait seat (walked share zero) the unwalked 3 ms. The
// review's assertion read "the churn twin hands 5 ms to the clipped seats but
// no clipped seat holds the unwalked 3 ms" (the io_wait seat kept it and the
// residual lane priced it); with the planner's ruling applied the claim is
// true, so the property asserted is its truth: the share the twin hands to
// "the chain seat" is exactly what the on-chain clipped seats publish, and no
// residual-lane seat prices any of it.
func TestHMC377ZeroWalkedDIOSeatHoldsItsShareAsTheCaseBObject(t *testing.T) {
	chain, stats, items := hmc377R4DIOUniverse(0)
	chain.CausalImpacts[0].DStateMs = 2.0
	board := hmc377UnwalkedBoard(chain, stats, items)
	rspaPatchSummariesForTwinVisibility(board)
	hmc377R4DIOCheck(t, board)
	clipped := 0.0
	for _, it := range board {
		if it.Thread.PID != 200 {
			continue
		}
		if it.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState {
			t.Errorf("the residual lane priced a share the ledger gave to the clipped seat: %+v", it)
		}
		if rootCauseItemIsOnChain(it) && it.ChainAnchorFullMs > 0 && !it.ChainAnchorRemainderSeat {
			clipped += it.DStateMs + it.IOWaitMs
		}
	}
	if !hmc377UwNear(clipped, 5) {
		t.Errorf("the clipped seats hold walked 2 + unwalked 3 on the chain lane, got %.3f:\n%s", clipped, hmc377R4Dump(board))
	}
	for _, it := range board {
		if it.Type == "fragmented_d_state_or_io_wait" && strings.Contains(it.Summary, "owned by the chain seat") && !hmc377UwNear(it.ChainAnchoredMs, clipped) {
			t.Errorf("the churn twin hands %.3f ms to the clipped seats, which hold %.3f ms: %q", it.ChainAnchoredMs, clipped, it.Summary)
		}
	}
}

// R4-1 (review: TestRv3DIOChannels): the same zero-walked io_wait seat beside
// a chain D seat that holds the walked share exactly (case A). Round 3 priced
// the 3 ms IO on the own seat and again on the residual lane (Σ 8 ms).
func TestHMC377ZeroWalkedDIOSeatGivesItsShareToTheOwnSeatBesideAnIdentityChainSeat(t *testing.T) {
	chain, stats, items := hmc377R4DIOUniverse(2)
	var noTwin []RootCauseRankItem
	for _, it := range items {
		if it.Type != "fragmented_d_state_or_io_wait" {
			noTwin = append(noTwin, it)
		}
	}
	board := hmc377UnwalkedBoard(chain, stats, noTwin)
	hmc377R4DIOCheck(t, board)
}

// R4-1 belt: the residual lane recognises a share the ledger owns. A row of
// the thread that kept instants inside the unwalked wait (here an adjacent
// census-group seat the planner does not rule — it is not on the chain lane)
// is never priced again by the residual lane once a seat owns the share.
func TestHMC377ResidualLaneRefusesAShareTheLedgerOwns(t *testing.T) {
	chain, stats, s := hmc377UnwalkedChain()
	member := ThreadRef{Comm: "dep", PID: 200}
	walked := []foldInterval{s[0], s[2]} // 5 walked + 3 outside
	unwalked := []foldInterval{s[1]}     // 4 inside the unwalked wait
	stats.runnableCensus = map[string]ThreadDuration{
		"200|0": {Thread: member, DurationMs: 8, anchoredMs: 5, runnableIntervals: walked},
		"200|1": {Thread: member, DurationMs: 4, anchoredMs: 0, runnableIntervals: unwalked},
	}
	group0 := hmc377UnwalkedWindowSeat(walked)
	group0.RunnableMs, group0.ImpactMs, group0.CumulativeImpactMs, group0.EffectiveImpactMs = 8, 8, 8, 8
	group1 := hmc377UnwalkedWindowSeat(unwalked)
	group1.RunnableMs, group1.ImpactMs, group1.CumulativeImpactMs, group1.EffectiveImpactMs = 4, 4, 4, 4
	group1.ledgerAnchoredRunnableMs = 0
	group1.Causality, group1.ChainRelevance = "adjacent_to_wakeup_chain", "adjacent"
	board := hmc377UnwalkedBoard(chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), group0, group1})
	priced := 0
	for _, it := range board {
		if it.Thread.PID == 200 && rootCauseItemIsOnChain(it) && it.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState {
			priced++
		}
	}
	if priced != 1 {
		t.Fatalf("the 4 ms inside the unwalked wait is priced by exactly one seat, got %d:\n%s", priced, hmc377R4Dump(board))
	}
	// The gate itself, on the typed mark alone.
	row := group1
	row.rspaUnwalkedShareOwned = true
	if lane := bareCensusEdgeStateSeatEligible(&row, wakeupChainThreadSet(chain), chain.Target, chainAnchorWindowsByPID(chain)); lane != stateEdgeLaneRefused {
		t.Fatalf("a row whose share the ledger owns is refused by the residual lane, got lane %d", lane)
	}
	row.rspaUnwalkedShareOwned = false
	if lane := bareCensusEdgeStateSeatEligible(&row, wakeupChainThreadSet(chain), chain.Target, chainAnchorWindowsByPID(chain)); lane != stateEdgeLaneChainMemberResidual {
		t.Fatalf("without an owner the same row is the residual population, got lane %d", lane)
	}
}

// R4-1, ready time: the zero-walked formal seat of a second census group gives
// its 4 ms to the own seat (the same predicate in the runnable family).
func TestHMC377ZeroWalkedReadySeatGivesItsShareToTheOwnSeat(t *testing.T) {
	chain, stats, items := hmc377R4TwoGroupUniverse()
	board := hmc377UnwalkedBoard(chain, stats, items)
	onChain, owners := 0.0, 0
	for _, it := range board {
		if it.Thread.PID == 200 && rootCauseItemIsOnChain(it) {
			onChain += it.RunnableMs
			if it.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState {
				owners++
			}
		}
	}
	if onChain > 9+rspaAnchorIdentityTolMs || owners != 1 {
		t.Fatalf("on-chain ready Σ %.3f ms (raw inside 9) with %d owners of the 4 ms:\n%s", onChain, owners, hmc377R4Dump(board))
	}
}

// hmc377R4TwoGroupUniverse: thread 200's census in two groups — group 0 holds
// the walked 5 ms and the outside 3 ms, group 1 only the 4 ms inside the
// unwalked wait (its walked share is zero). Both formal seats are on the chain
// lane beside the 5 ms chain seat.
func hmc377R4TwoGroupUniverse() (ChainResult, WindowStats, []RootCauseRankItem) {
	chain, stats, s := hmc377UnwalkedChain()
	member := ThreadRef{Comm: "dep", PID: 200}
	walked, unwalked := []foldInterval{s[0], s[2]}, []foldInterval{s[1]}
	stats.runnableCensus = map[string]ThreadDuration{
		"200|0": {Thread: member, DurationMs: 8, anchoredMs: 5, runnableIntervals: walked},
		"200|1": {Thread: member, DurationMs: 4, anchoredMs: 0, runnableIntervals: unwalked},
	}
	group0 := hmc377UnwalkedWindowSeat(walked)
	group0.RunnableMs, group0.ImpactMs, group0.CumulativeImpactMs, group0.EffectiveImpactMs = 8, 8, 8, 8
	group1 := hmc377UnwalkedWindowSeat(unwalked)
	group1.RunnableMs, group1.ImpactMs, group1.CumulativeImpactMs, group1.EffectiveImpactMs = 4, 4, 4, 4
	group1.ledgerAnchoredRunnableMs = 0
	return chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), group0, group1}
}

// hmc377R4RankPasses runs the two re-anchoring passes of one rank build the
// way query.go does — build pass, the build cap (keep), the enrich additions,
// enrich pass — carrying the build pass's unwalked-share decisions on the
// rank result. It returns both passes' boards (pass 2's input is the build
// survivors + the additions).
func hmc377R4RankPasses(chain ChainResult, stats WindowStats, items []RootCauseRankItem,
	keep func(RootCauseRankItem) bool, additions []RootCauseRankItem) (pass1, input2, pass2 []RootCauseRankItem) {
	var rank RootCauseRankResult
	pass1 = reanchorOnChainStateSeatsCarried(chain, stats, items, &rank.unwalkedDecisions)
	pass1 = anchorBareCensusEdgeStateSeats(chain, pass1)
	for _, it := range pass1 {
		if keep(it) {
			input2 = append(input2, it)
		}
	}
	rspaPatchSummariesForTwinVisibility(input2)
	input2 = append(input2, additions...)
	pass2 = reanchorOnChainStateSeatsCarried(chain, stats, append([]RootCauseRankItem(nil), input2...), &rank.unwalkedDecisions)
	pass2 = anchorBareCensusEdgeStateSeats(chain, pass2)
	return pass1, input2, pass2
}

func hmc377R4UnwalkedSeats(items []RootCauseRankItem) int {
	n := 0
	for _, it := range items {
		if it.Thread.PID == 200 && it.ChainAnchorUnwalkedSeat {
			n++
		}
	}
	return n
}

// R4-2 (review: TestRv3PassTwoReMint). The build cap kills the own seat, the
// three-way remainder and every formal row; only the chain seat survives. The
// enrich pass adds the thread's CPU-constraint row. Round 3 inferred the
// earlier decision from the survivors, found none and minted a second own
// seat — a copy of the CPU-constraint row (its typed constraint fields and
// confidence on a ready-time seat). The decision now rides typed on the rank
// result: the enrich pass adopts it and mints nothing.
func TestHMC377UnwalkedDecisionIsCarriedAcrossTheRankPasses(t *testing.T) {
	chain, stats, s := hmc377UnwalkedChain()
	aff := hmc377OwnerAffinity(12)
	aff.CPUConstraintKind = "sched_switch_next_info"
	aff.CPUConstraintAllowedCPUs = []int{7}
	aff.FixDirection = "scheduling_supply"
	pass1, input2, pass2 := hmc377R4RankPasses(chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), hmc377UnwalkedWindowSeat(s)},
		func(it RootCauseRankItem) bool { return strings.HasPrefix(it.Source, "wakeup_chain") },
		[]RootCauseRankItem{aff, hmc377OwnerSatellite("scheduler_latency", s, 12)})
	if n := hmc377R4UnwalkedSeats(pass1); n != 1 {
		t.Fatalf("the build pass mints the one own seat, got %d:\n%s", n, hmc377R4Dump(pass1))
	}
	if minted := hmc377R4UnwalkedSeats(pass2) - hmc377R4UnwalkedSeats(input2); minted != 0 {
		t.Fatalf("the enrich pass minted %d more own seat(s) for a share the build pass decided:\n%s", minted, hmc377R4Dump(pass2))
	}
	for _, it := range pass2 {
		if it.Type == "scheduler_latency" && it.ChainAnchorRemainderSeat {
			if !hmc377UwNear(it.RunnableMs, 3) || !hmc377UwNear(it.ChainAnchorUnwalkedMs, 4) || !strings.Contains(it.Summary, rspaSummaryUnwalkedOwnSeat) {
				t.Errorf("the enrich-pass satellite reads the carried owner (3 ms outside, its 4 ms named as the own seat's): %+v", it)
			}
		}
	}
}

// R4-2: an own seat minted in a pass with no formal window seat holding the
// share is the census account's seat — never a copy of a row of another kind
// (the review's re-minted seat carried the affinity row's CPU-constraint kind,
// allowed CPUs, fix direction and 0.70 confidence).
func TestHMC377UnwalkedSeatIsNeverCopiedFromARowOfAnotherKind(t *testing.T) {
	chain, stats, s := hmc377UnwalkedChain()
	aff := hmc377OwnerAffinity(12)
	aff.CPUConstraintKind = "sched_switch_next_info"
	aff.CPUConstraintAllowedCPUs = []int{7}
	aff.FixDirection = "scheduling_supply"
	stats.Window = TimeWindow{StartTs: 1.0, EndTs: 1.1}
	board := hmc377UnwalkedBoard(chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), aff, hmc377OwnerSatellite("scheduler_latency", s, 12)})
	var seat *RootCauseRankItem
	for i := range board {
		if board[i].ChainAnchorUnwalkedSeat {
			if seat != nil {
				t.Fatalf("two own seats:\n%s", hmc377R4Dump(board))
			}
			seat = &board[i]
		}
	}
	if seat == nil || !hmc377UwNear(seat.RunnableMs, 4) {
		t.Fatalf("the census mints the one own seat holding 4 ms:\n%s", hmc377R4Dump(board))
	}
	if seat.Type != "runnable_wait" || seat.Source != "window_stats" || seat.CPUConstraintKind != "" || len(seat.CPUConstraintAllowedCPUs) != 0 ||
		seat.FixDirection != "" || seat.Confidence != rootCauseWindowRunnableSeatConfidence || len(seat.familyMemberIntervals) != 0 ||
		seat.StatsWindowStartTs != 1.0 || seat.StatsWindowEndTs != 1.1 {
		t.Fatalf("the own seat carries ready-time fields only (the census account's type, source, confidence and window): %+v", *seat)
	}
}

// R4-3, engine half (review: TestRv3PartialSatellite). Two unwalked waits; the
// satellite's inventory holds only the first one's ready time (4 of the
// thread's 6 ms). The review asserted the satellite's typed unwalked part
// equal the seat's value — the renderer's round-3 lookup assumption; the
// part is row-level by design (the row's three parts add up to the row's
// account), so the properties asserted are: the one owner seat publishes the
// thread's whole share, the row's part lies inside it, the row's parts add
// up, and its sentence says which part it covers.
func TestHMC377PartialSatelliteNamesItsPartOfTheOwnSeatShare(t *testing.T) {
	chain, stats, s := hmc377R4TwoUnwalkedChain()
	window := hmc377UnwalkedWindowSeat(s)
	window.RunnableMs, window.ImpactMs, window.CumulativeImpactMs, window.EffectiveImpactMs, window.EndTs = 14, 14, 14, 14, 1.084
	sat := hmc377OwnerSatellite("scheduler_latency", []foldInterval{s[0], s[1], s[2]}, 12)
	board := hmc377UnwalkedBoard(chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), window, sat})
	var seat *RootCauseRankItem
	for i := range board {
		if board[i].ChainAnchorUnwalkedSeat {
			seat = &board[i]
		}
	}
	if seat == nil || !hmc377UwNear(seat.RunnableMs, 6) {
		t.Fatalf("the own seat holds the thread's whole 6 ms:\n%s", hmc377R4Dump(board))
	}
	found := false
	for _, it := range board {
		if it.Type != "scheduler_latency" || it.ChainAnchorUnwalkedMs <= 0 {
			continue
		}
		found = true
		if !hmc377UwNear(it.ChainAnchoredMs, 5) || !hmc377UwNear(it.ChainAnchorUnwalkedMs, 4) || !hmc377UwNear(it.ChainAnchorOutsideMs, 3) ||
			!hmc377UwNear(it.ChainAnchorFullMs, 12) || !hmc377UwNear(it.RunnableMs, 3) {
			t.Errorf("the satellite's typed parts are its own account's (12 = 5 + 4 + 3): %+v", it)
		}
		if !strings.Contains(it.Summary, "this row's 4.000ms is part of the 6.000ms that seat holds for this thread") {
			t.Errorf("the satellite's sentence says which part of the seat's share it covers: %q", it.Summary)
		}
	}
	if !found {
		t.Fatalf("the satellite carries the three-way split:\n%s", hmc377R4Dump(board))
	}
	for _, it := range board {
		if it.Type == "runnable_wait" && it.ChainAnchorRemainderSeat && strings.Contains(it.Summary, "is part of the") {
			t.Errorf("the formal remainder holds the thread's whole share — no partial clause: %q", it.Summary)
		}
	}
}

// hmc377R4TwoUnwalkedChain (review round 3, rv3TwoUnwalked): TWO unwalked
// waits, [1.050,1.060] and [1.080,1.090], both ended by 200. Ready segments:
// walked 5 ms, unwalked 4 ms + 2 ms, outside 3 ms. Full 14 = 5 + 6 + 3.
func hmc377R4TwoUnwalkedChain() (ChainResult, WindowStats, []foldInterval) {
	chain, stats, _ := hmc377UnwalkedChain()
	segments := []foldInterval{{start: 1.010, end: 1.015, valueMs: 5}, {start: 1.052, end: 1.056, valueMs: 4}, {start: 1.070, end: 1.073, valueMs: 3}, {start: 1.082, end: 1.084, valueMs: 2}}
	chain.directDependencyWindows = map[int][]targetWaitDependencyWindow{
		200: {{startTs: 1.000, endTs: 1.030, wakeupTs: 1.030}, {startTs: 1.050, endTs: 1.060, wakeupTs: 1.060}, {startTs: 1.080, endTs: 1.090, wakeupTs: 1.090}},
	}
	chain.WakeupEdgeCensus = []WakeupEdgeCensusPair{{Waker: ThreadRef{Comm: "dep", PID: 200}, Wakee: chain.Target, Count: 3, FirstTs: 1.030, LastTs: 1.090}}
	stats.runnableCensus = map[string]ThreadDuration{"200|0": {Thread: ThreadRef{Comm: "dep", PID: 200}, DurationMs: 14.0, anchoredMs: 5.0, runnableIntervals: segments}}
	return chain, stats, segments
}
