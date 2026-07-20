package tracequery

import "testing"

func TestRankZeroDisclosureCapacityReservesBothTypedLanes(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	items := []RootCauseRankItem{
		{Type: "sleep_wait", Thread: target, SubjectIsAnalysisTarget: true},
		{Type: "binder_wait", Thread: target, SubjectIsAnalysisTarget: true},
		{Type: "missing_wakeup", Thread: target, SubjectIsAnalysisTarget: true},
		{Type: "blocking_span", Thread: target, SubjectIsAnalysisTarget: true},
		{Type: "fragmented_sleep_wait", Thread: target, SubjectIsAnalysisTarget: true},
		{Type: "trace_gap", Thread: ThreadRef{Comm: "peer-a", PID: 201}},
		{Type: "trace_gap", Thread: ThreadRef{Comm: "peer-b", PID: 202}},
		{Type: "trace_gap", Thread: ThreadRef{Comm: "peer-c", PID: 203}},
		{Type: "runnable_wait", Thread: ThreadRef{Comm: "worker", PID: 300}, ImpactMs: 1, RunnableMs: 1},
	}

	got, _, candidateTotal, candidateEmitted, sideTotal, sideEmitted := truncateRootCauseRankCandidatesAndSideRows(items, 1)
	if candidateTotal != 1 || candidateEmitted != 1 || sideTotal != 8 || sideEmitted != rootCauseRankZeroSeatDisclosureCap {
		t.Fatalf("unexpected capacity census candidates=%d/%d side=%d/%d rows=%+v", candidateEmitted, candidateTotal, sideEmitted, sideTotal, got)
	}
	var targetRows, gapRows int
	for _, item := range got {
		if item.Type == "trace_gap" {
			gapRows++
		} else if rootCauseRankItemIsZeroSeatDisclosure(item) {
			targetRows++
		}
	}
	if targetRows != rootCauseRankZeroSeatPerLaneReservedCap || gapRows != rootCauseRankZeroSeatPerLaneReservedCap {
		t.Fatalf("both populated rank-0 lanes must retain reserved seats, target=%d gap=%d rows=%+v", targetRows, gapRows, got)
	}
}

func TestRankZeroDisclosureCapacityLoansUnusedLaneSeats(t *testing.T) {
	items := make([]RootCauseRankItem, 0, 6)
	for i := 0; i < 6; i++ {
		items = append(items, RootCauseRankItem{Type: "trace_gap", Thread: ThreadRef{PID: 200 + i}})
	}
	got, _, _, _, sideTotal, sideEmitted := truncateRootCauseRankCandidatesAndSideRows(items, 1)
	if sideTotal != 6 || sideEmitted != rootCauseRankZeroSeatDisclosureCap || len(got) != rootCauseRankZeroSeatDisclosureCap {
		t.Fatalf("an unopposed rank-0 lane should use the full disclosure cap, total=%d emitted=%d rows=%+v", sideTotal, sideEmitted, got)
	}
}

// TestRankSelfSideLaneSixSeatFullFamilyOverflow is the 终判① §29.96.2
// structural pin (2026-07-15, selfSide cap 4 → 6): SIX of the analysis
// target's own value-holding on-chain seats — the four state families
// (running / runnable / D-state / IO) + a semantic span seat + a sixth seat
// from any family (fragmented_running here) — ALL overflow the candidate cap
// at once, and NONE may silently disappear (§29.93.3 全族收编 zero-silent-
// disappearance promise, self specialization): every one of the six must
// hold a published wire form, either a candidate board seat or a selfSide
// side-lane seat. The side census (sideTotal/sideEmitted — the caveat
// counts) must account for all six. Under the retired cap 4 this exact
// shape silently dropped two seats (sideEmitted 4 < sideTotal 6) — the
// mutation arm this pin turns red on.
func TestRankSelfSideLaneSixSeatFullFamilyOverflow(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	background := func(pid int, ms float64) RootCauseRankItem {
		return RootCauseRankItem{
			Type: "runnable_wait", Thread: ThreadRef{Comm: "bg", PID: pid},
			ImpactMs: ms, RunnableMs: ms,
		}
	}
	selfSeats := []RootCauseRankItem{
		{Type: "running", Thread: target, SubjectIsAnalysisTarget: true,
			ChainRelevance: "on_chain", Causality: RootCauseCausalitySelfWallClock,
			RunningMs: 9, EffectiveImpactMs: 5, SupplyFoldDeficitMs: 5},
		{Type: "runnable_wait", Thread: target, SubjectIsAnalysisTarget: true,
			ChainRelevance: "on_chain", RunnableMs: 4, ImpactMs: 4},
		{Type: "d_state_or_io_wait", Thread: target, SubjectIsAnalysisTarget: true,
			ChainRelevance: "on_chain", DStateMs: 3, ImpactMs: 3},
		{Type: "io_wait", Thread: target, SubjectIsAnalysisTarget: true,
			ChainRelevance: "on_chain", IOWaitMs: 2, ImpactMs: 2},
		{Type: "class_verification", Thread: target, SubjectIsAnalysisTarget: true,
			ChainRelevance: "on_chain", EffectiveImpactMs: 1.5, ImpactMs: 1.5},
		{Type: "fragmented_running", Thread: target, SubjectIsAnalysisTarget: true,
			ChainRelevance: "on_chain", RunningMs: 1, EffectiveImpactMs: 1},
	}
	for _, seat := range selfSeats {
		if rootCauseEffectiveImpactMs(seat) <= 0 || !rootCauseItemIsOnChain(seat) {
			t.Fatalf("fixture drifted: every self seat must hold value on-chain: %+v", seat)
		}
		if rootCauseRankItemIsZeroSeatDisclosure(seat) {
			t.Fatalf("fixture drifted: a self seat fell into the rank-0 disclosure lane: %+v", seat)
		}
	}
	// Board filled by background candidates; every self seat sorts BEYOND the
	// candidate cap (the tieba death shape, all-families form).
	items := []RootCauseRankItem{background(300, 20), background(301, 19), background(302, 18)}
	items = append(items, selfSeats...)
	limit := 3
	got, _, candidateTotal, candidateEmitted, sideTotal, sideEmitted := truncateRootCauseRankCandidatesAndSideRows(items, limit)
	if candidateTotal != 9 || candidateEmitted != 3 {
		t.Fatalf("candidate census drifted: %d/%d rows=%+v", candidateEmitted, candidateTotal, got)
	}
	// Caveat counts: all six self seats are side-accounted AND side-published.
	if sideTotal != 6 || sideEmitted != 6 {
		t.Fatalf("终判①: six overflowing self seats must all ride the selfSide lane (cap 6), side=%d/%d rows=%+v", sideEmitted, sideTotal, got)
	}
	// Zero silent disappearance: every self seat has a published wire form.
	published := map[string]bool{}
	for _, item := range got {
		published[item.Type+"\x00"+threadKey(item.Thread)] = true
	}
	for _, seat := range selfSeats {
		if !published[seat.Type+"\x00"+threadKey(seat.Thread)] {
			t.Fatalf("自身持值席静默消失 (type=%s): no published wire form, rows=%+v", seat.Type, got)
		}
	}
}

// TestZeroTraceGapRankRowDropsWhenThreadHoldsValuedSeat is the CHAIN-BUDGET
// 返工 P1-2① 同窗去重 pin (2026-07-18): a zero-value trace_gap rank row whose
// thread ALSO holds a valued seat on the same board window is a
// self-contradictory blind-spot claim (the thread demonstrably has data in
// this window) and stops minting a rank seat; a thread with no valued row
// keeps its gap disclosure byte-identically (G2 real blind spots stay).
// Dropping the pass reds the tieba regression pin (the 59843 gap row
// re-floods the gap side lane and halves the target-side cap).
func TestZeroTraceGapRankRowDropsWhenThreadHoldsValuedSeat(t *testing.T) {
	valued := ThreadRef{Comm: "busy", PID: 100}
	blind := ThreadRef{Comm: "idle-helper", PID: 200}
	items := []RootCauseRankItem{
		{Type: "runnable_wait", Thread: valued, RunnableMs: 5, EffectiveImpactMs: 5, CumulativeImpactMs: 5},
		{Type: "trace_gap", Thread: valued},
		{Type: "trace_gap", Thread: blind},
	}
	got := dropZeroTraceGapRankRowsForValuedThreads(items)
	if len(got) != 2 {
		t.Fatalf("exactly the valued thread's zero gap row must drop, got %+v", got)
	}
	for _, item := range got {
		if item.Type == "trace_gap" && item.Thread.PID == valued.PID {
			t.Fatalf("the valued thread's contradictory blind-spot row survived: %+v", got)
		}
	}
	var blindKept bool
	for _, item := range got {
		if item.Type == "trace_gap" && item.Thread.PID == blind.PID {
			blindKept = true
		}
	}
	if !blindKept {
		t.Fatalf("a thread WITHOUT a valued seat must keep its blind-spot disclosure, got %+v", got)
	}
}

// TestSideLaneCapKeepsValuedDisclosuresFirst is the CHAIN-BUDGET 返工 P1-2②
// 帽内先保真值行 pin (2026-07-18): the bounded target-side disclosure cap
// fills TRUE-VALUE rows before zero-value rows regardless of the blind
// channel-first arrival order (donghu 17267 witness: the valued pacing_idle
// 15.758 context row died at the halved cap behind zero rows and a smaller
// newcomer). Zero rows keep their relative order AFTER every valued row.
func TestSideLaneCapKeepsValuedDisclosuresFirst(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	zero1 := RootCauseRankItem{Type: "trace_span", Thread: ThreadRef{Comm: "spanner-a", PID: 301}, CumulativeImpactMs: 9}
	zero2 := RootCauseRankItem{Type: "trace_span", Thread: ThreadRef{Comm: "spanner-b", PID: 302}, CumulativeImpactMs: 8}
	valued := RootCauseRankItem{Type: "pacing_idle", Thread: target, SubjectIsAnalysisTarget: true, EffectiveImpactMs: 15.758, CumulativeImpactMs: 15.758, PeriodicSource: true}
	gap := RootCauseRankItem{Type: "trace_gap", Thread: ThreadRef{Comm: "idle-helper", PID: 400}}
	// Blind arrival order: both zero rows ahead of the valued disclosure.
	items := []RootCauseRankItem{zero1, zero2, valued, gap}
	got, _, _, _, _, _ := truncateRootCauseRankCandidatesAndSideRows(items, 12)
	var kept []string
	var valuedKept bool
	for _, item := range got {
		if item.Type == "trace_gap" {
			continue
		}
		kept = append(kept, item.Thread.Comm)
		if item.Type == "pacing_idle" {
			valuedKept = true
		}
	}
	if !valuedKept {
		t.Fatalf("the valued disclosure must never die at the cap behind zero rows, kept=%v rows=%+v", kept, got)
	}
	if len(kept) != rootCauseRankZeroSeatPerLaneReservedCap || kept[0] != "app" || kept[1] != "spanner-a" {
		t.Fatalf("cap fill must be valued-first then stable zero order, kept=%v", kept)
	}
}
