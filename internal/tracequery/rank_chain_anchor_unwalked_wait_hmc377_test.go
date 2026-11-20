package tracequery

import (
	"math"
	"strings"
	"testing"
)

// HMC377 fix round 2, item F3. A thread on the analysed thread's chain is
// priced over the union of its chain windows and its own direct dependency
// windows — the waits of the analysed thread that its own recorded wakeup
// ended (the E2 carrier). The chain expansion walks only some of those waits;
// the thread's ready / D / IO time inside the ones it did not walk is still
// its time inside a wait its wakeup ended (TraceWakeupDependencyCreditTeaching)
// and must be priced, every instant once, and the remainder must be the time
// outside every such window.
//
// Universe: target 100 waits [1.000, 1.030] (walked: chain node of 200,
// closed by 200's wakeup) and [1.050, 1.060] (not walked, also closed by
// 200's wakeup). Thread 200's ready segments: [1.010, 1.015] inside the walked
// wait (5 ms, the chain seat's share), [1.052, 1.056] inside the unwalked
// wait (4 ms), [1.070, 1.073] outside both (3 ms). Full account 12 ms.
func hmc377UnwalkedChain() (ChainResult, WindowStats, []foldInterval) {
	target, member := ThreadRef{Comm: "app", PID: 100}, ThreadRef{Comm: "dep", PID: 200}
	segments := []foldInterval{{start: 1.010, end: 1.015, valueMs: 5}, {start: 1.052, end: 1.056, valueMs: 4}, {start: 1.070, end: 1.073, valueMs: 3}}
	chain := ChainResult{
		Target: target,
		Window: TimeWindow{StartTs: 1.0, EndTs: 1.1},
		Nodes: []ChainNode{
			{Thread: target, Depth: 0, Window: TimeWindow{StartTs: 1.000, EndTs: 1.030}, Dominant: StateSSleep},
			{Thread: member, Depth: 1, Window: TimeWindow{StartTs: 1.000, EndTs: 1.030}, Dominant: StateRunnable},
		},
		Edges:         []WakeupEdge{{Waker: member, Wakee: target, WakeupTs: 1.030, WakeupLine: 30}},
		CausalImpacts: []WakeupCausalImpact{{Thread: member, ChainDepth: 1, RunnableMs: 5.0, Window: TimeWindow{StartTs: 1.000, EndTs: 1.030}}},
		WakeupEdgeCensus: []WakeupEdgeCensusPair{
			{Waker: member, Wakee: target, Count: 2, FirstTs: 1.030, LastTs: 1.060},
		},
		directDependencyWindowsBuilt: true,
		directDependencyWindows: map[int][]targetWaitDependencyWindow{
			200: {{startTs: 1.000, endTs: 1.030, wakeupTs: 1.030}, {startTs: 1.050, endTs: 1.060, wakeupTs: 1.060}},
		},
	}
	stats := WindowStats{
		chainAnchorsByPID:      chainAnchorWindowsByPID(chain),
		offCPUProducerDisjoint: true,
		runnableCensus: map[string]ThreadDuration{
			"200|0": {Thread: member, DurationMs: 12.0, anchoredMs: 5.0, runnableIntervals: segments},
		},
	}
	return chain, stats, segments
}

func hmc377UnwalkedWindowSeat(segments []foldInterval) RootCauseRankItem {
	return RootCauseRankItem{Type: "runnable_wait", Thread: ThreadRef{Comm: "dep", PID: 200},
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
		RunnableMs: 12, ImpactMs: 12, CumulativeImpactMs: 12, EffectiveImpactMs: 12, Score: 12 * 0.8,
		Source: "window_stats", Confidence: 0.8, LineStart: 10, LineEnd: 73, StartTs: 1.010, EndTs: 1.073,
		StatsWindowStartTs: 1.0, StatsWindowEndTs: 1.1,
		ledgerAnchorStamped: true, ledgerAnchoredRunnableMs: 5.0,
		runnableIntervals: append([]foldInterval(nil), segments...), memberSegmentsProducerDisjoint: true}
}

func hmc377UnwalkedChainSeat() RootCauseRankItem {
	return RootCauseRankItem{Type: "runnable_wait", Thread: ThreadRef{Comm: "dep", PID: 200},
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
		RunnableMs: 5.0, ImpactMs: 5.0, CumulativeImpactMs: 5.0, EffectiveImpactMs: 5.0,
		Source: "wakeup_chain.causal_impacts", Confidence: 0.9, LineStart: 10, LineEnd: 30, StartTs: 1.0, EndTs: 1.03,
		ChainDepth: 1}
}

// hmc377UnwalkedBoard runs the two state passes of a rank build in order.
func hmc377UnwalkedBoard(chain ChainResult, stats WindowStats, items []RootCauseRankItem) []RootCauseRankItem {
	items = reanchorOnChainStateSeats(chain, stats, items)
	return anchorBareCensusEdgeStateSeats(chain, items)
}

type hmc377UnwalkedRows struct {
	chainSeat, insideWait, remainder, clipped []RootCauseRankItem
}

func hmc377UnwalkedClassify(items []RootCauseRankItem) hmc377UnwalkedRows {
	var out hmc377UnwalkedRows
	for _, item := range items {
		if item.Thread.PID != 200 {
			continue
		}
		switch {
		case strings.HasPrefix(item.Source, "wakeup_chain"):
			out.chainSeat = append(out.chainSeat, item)
		case item.ChainAnchorRemainderSeat:
			out.remainder = append(out.remainder, item)
		case item.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState:
			out.insideWait = append(out.insideWait, item)
		case item.ChainAnchorFullMs > 0:
			out.clipped = append(out.clipped, item)
		}
	}
	return out
}

func hmc377UwNear(a, b float64) bool { return math.Abs(a-b) <= 0.0005 }

// Case A: the chain seat holds the walked share; the ready time inside the
// unwalked wait is its own on-chain seat; the remainder is only the time
// outside every wait the thread's wakeup ended.
func TestHMC377ChainMemberIsPricedInsideTheWaitsItsWakeupEndedThatTheChainDidNotWalk(t *testing.T) {
	chain, stats, segments := hmc377UnwalkedChain()
	rows := hmc377UnwalkedClassify(hmc377UnwalkedBoard(chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), hmc377UnwalkedWindowSeat(segments)}))
	if len(rows.chainSeat) != 1 || !hmc377UwNear(rows.chainSeat[0].RunnableMs, 5) {
		t.Fatalf("the chain seat keeps the walked share untouched: %+v", rows.chainSeat)
	}
	if len(rows.remainder) != 1 {
		t.Fatalf("one adjacent remainder: %+v", rows.remainder)
	}
	rem := rows.remainder[0]
	// HMC377 fix round 3: the three parts ride typed — 5 ms walked (the chain
	// seat), 4 ms unwalked (its own seat), 3 ms outside (this remainder).
	if !hmc377UwNear(rem.RunnableMs, 3) || !hmc377UwNear(rem.EffectiveImpactMs, 3) ||
		!hmc377UwNear(rem.ChainAnchoredMs, 5) || !hmc377UwNear(rem.ChainAnchorUnwalkedMs, 4) ||
		!hmc377UwNear(rem.ChainAnchorOutsideMs, 3) || !hmc377UwNear(rem.ChainAnchorFullMs, 12) {
		t.Fatalf("the remainder is the 3 ms outside every wait the thread's wakeup ended (full 12 = 5 walked + 4 unwalked + 3): %+v", rem)
	}
	if !strings.Contains(rem.Summary, "remainder 3.000ms outside") || strings.Contains(rem.Summary, "remainder 7.000ms") {
		t.Fatalf("the remainder sentence must state only the time outside the dependency windows: %q", rem.Summary)
	}
	if len(rows.insideWait) != 1 {
		t.Fatalf("the ready time inside the unwalked wait is priced on the chain: %+v", rows)
	}
	seat := rows.insideWait[0]
	if !hmc377UwNear(seat.RunnableMs, 4) || !hmc377UwNear(seat.EffectiveImpactMs, 4) || seat.ChainRelevance != "on_chain" ||
		seat.HostWakeupEdgeAnchorTs != 1.060 || seat.HostWakeupEdgeAnchorVia != HostWakeupEdgeAnchorViaDirect {
		t.Fatalf("4 ms inside the wait ended at 1.060, priced with the wakeup that ended it: %+v", seat)
	}
	if len(seat.runnableIntervals) != 1 || seat.runnableIntervals[0].start != 1.052 || seat.runnableIntervals[0].end != 1.056 {
		t.Fatalf("the priced seat's support is exactly the inside segment: %+v", seat.runnableIntervals)
	}
	if !strings.Contains(seat.Summary, "did not walk") || !seat.ChainAnchorUnwalkedSeat {
		t.Fatalf("the priced seat says these waits were not walked by the chain and wears the typed role: %+v", seat)
	}
	if got := rows.chainSeat[0].RunnableMs + seat.RunnableMs + rem.RunnableMs; !hmc377UwNear(got, 12) {
		t.Fatalf("every instant is counted once: %.3f != 12", got)
	}
}

// Case B: no chain-lane seat holds the walked share — the window seat itself
// carries the whole inside share (walked + unwalked waits) as one object.
func TestHMC377ChainMemberWithoutChainSeatCarriesTheWholeInsideShare(t *testing.T) {
	chain, stats, segments := hmc377UnwalkedChain()
	chain.CausalImpacts = nil
	rows := hmc377UnwalkedClassify(hmc377UnwalkedBoard(chain, stats, []RootCauseRankItem{hmc377UnwalkedWindowSeat(segments)}))
	if len(rows.clipped) != 1 || !hmc377UwNear(rows.clipped[0].RunnableMs, 9) || rows.clipped[0].ChainRelevance != "on_chain" {
		t.Fatalf("one on-chain seat with the 9 ms inside the thread's dependency windows: %+v", rows)
	}
	if len(rows.insideWait) != 0 {
		t.Fatalf("no second object for the same thread's inside time: %+v", rows.insideWait)
	}
	if len(rows.remainder) != 1 || !hmc377UwNear(rows.remainder[0].RunnableMs, 3) {
		t.Fatalf("the remainder is the 3 ms outside: %+v", rows.remainder)
	}
}

// The indivisible inversion account (R-377-4) joins its chain seat as lossless
// detail; the ready time inside the unwalked wait is its own seat and the
// detail sentence names all three shares.
func TestHMC377InversionAccountNamesTheUnwalkedWaitShare(t *testing.T) {
	chain, stats, segments := hmc377UnwalkedChain()
	inversion := hmc377UnwalkedWindowSeat(segments)
	inversion.Type = "priority_inversion_runnable_wait"
	inversion.EffectiveImpactMs, inversion.GatedRunnableMs, inversion.PriorityRelationProvenLowerMs = 2.0, 2.0, 2.0
	seat := hmc377UnwalkedChainSeat()
	seat.Type = "priority_inversion_candidate"
	seat.GatedRunnableMs = 5.0
	items := hmc377UnwalkedBoard(chain, stats, []RootCauseRankItem{seat, inversion})
	rows := hmc377UnwalkedClassify(items)
	if len(rows.remainder) != 1 || !hmc377UwNear(rows.remainder[0].RunnableMs, 3) {
		t.Fatalf("the plain ready-time remainder is the 3 ms outside: %+v", rows.remainder)
	}
	if len(rows.insideWait) != 1 || !hmc377UwNear(rows.insideWait[0].RunnableMs, 4) || rows.insideWait[0].Type != "runnable_wait" {
		t.Fatalf("the ready time inside the unwalked wait is a plain ready-time seat: %+v", rows.insideWait)
	}
	for _, item := range items {
		if item.Type == "priority_inversion_runnable_wait" {
			if !strings.Contains(item.Summary, "5.000ms of this 12.000ms ready-time account") ||
				!strings.Contains(item.Summary, "4.000ms") || !strings.Contains(item.Summary, "3.000ms lies outside") {
				t.Fatalf("the detail sentence states the three shares: %q", item.Summary)
			}
			if item.RunnableMs != 12 || item.EffectiveImpactMs != 2 {
				t.Fatalf("the inversion account keeps every value: %+v", item)
			}
		}
	}
}

// Fail closed: an inventory that does not reproduce the stamped account, or a
// thread whose wakeups ended only walked waits, keeps today's form exactly.
func TestHMC377UnwalkedWaitPricingFailsClosed(t *testing.T) {
	chain, stats, segments := hmc377UnwalkedChain()
	walkedOnly := chain
	walkedOnly.directDependencyWindows = map[int][]targetWaitDependencyWindow{200: {{startTs: 1.000, endTs: 1.030, wakeupTs: 1.030}}}
	rows := hmc377UnwalkedClassify(hmc377UnwalkedBoard(walkedOnly, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), hmc377UnwalkedWindowSeat(segments)}))
	if len(rows.insideWait) != 0 || len(rows.remainder) != 1 || !hmc377UwNear(rows.remainder[0].RunnableMs, 7) ||
		!hmc377UwNear(rows.remainder[0].ChainAnchoredMs, 5) {
		t.Fatalf("with every ended wait walked the partition is the chain windows' alone: %+v", rows)
	}
	broken := hmc377UnwalkedWindowSeat(segments[:2])
	rows = hmc377UnwalkedClassify(hmc377UnwalkedBoard(chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), broken}))
	if len(rows.insideWait) != 0 || len(rows.remainder) != 1 || !hmc377UwNear(rows.remainder[0].RunnableMs, 7) {
		t.Fatalf("an inventory that does not reproduce its account proves no inside share: %+v", rows)
	}
}

// D/IO: the same union per channel — blocked time inside the unwalked wait is
// priced on the chain (D part), the remainder is the time outside.
func TestHMC377ChainMemberBlockedTimeInsideTheUnwalkedWaitIsPriced(t *testing.T) {
	chain, stats, _ := hmc377UnwalkedChain()
	d := []foldInterval{{start: 1.020, end: 1.022, valueMs: 2}, {start: 1.054, end: 1.057, valueMs: 3}}
	io := []foldInterval{{start: 1.080, end: 1.081, valueMs: 1}}
	member := ThreadRef{Comm: "dep", PID: 200}
	stats.dstateCensus = map[string]ThreadDuration{"200|0": {Thread: member, DurationMs: 5, anchoredMs: 2, dioIntervals: d}}
	stats.iowaitCensus = map[string]ThreadDuration{"200|0": {Thread: member, DurationMs: 1, anchoredMs: 0, dioIntervals: io}}
	chain.CausalImpacts = []WakeupCausalImpact{{Thread: member, ChainDepth: 1, DStateMs: 2.0, Window: TimeWindow{StartTs: 1.000, EndTs: 1.030}}}
	window := RootCauseRankItem{Type: "d_state_or_io_wait", Thread: member, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
		DominantState: string(StateDSleep), DStateMs: 5, IOWaitMs: 1, ImpactMs: 6, CumulativeImpactMs: 6, EffectiveImpactMs: 6,
		Source: "window_stats", Confidence: 0.8, LineStart: 20, LineEnd: 81, StartTs: 1.020, EndTs: 1.081,
		StatsWindowStartTs: 1.0, StatsWindowEndTs: 1.1, ledgerAnchorStamped: true, ledgerAnchoredDMs: 2, ledgerAnchoredIOMs: 0,
		dioSegmentIntervalsD: d, dioSegmentIntervalsIO: io, dioSegmentIntervals: append(append([]foldInterval(nil), d...), io...)}
	chainSeat := RootCauseRankItem{Type: "d_state_or_io_wait", Thread: member, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
		DominantState: string(StateDSleep), DStateMs: 2, ImpactMs: 2, CumulativeImpactMs: 2, EffectiveImpactMs: 2,
		Source: "wakeup_chain.causal_impacts", Confidence: 0.9, LineStart: 20, LineEnd: 30, StartTs: 1.0, EndTs: 1.03, ChainDepth: 1}
	rows := hmc377UnwalkedClassify(hmc377UnwalkedBoard(chain, stats, []RootCauseRankItem{chainSeat, window}))
	if len(rows.insideWait) != 1 || !hmc377UwNear(rows.insideWait[0].DStateMs, 3) || !hmc377UwNear(rows.insideWait[0].IOWaitMs, 0) ||
		rows.insideWait[0].HostWakeupEdgeAnchorTs != 1.060 {
		t.Fatalf("the 3 ms of D time inside the wait ended at 1.060 is priced on the chain: %+v", rows.insideWait)
	}
	if len(rows.remainder) != 1 || !hmc377UwNear(rows.remainder[0].DStateMs, 0) || !hmc377UwNear(rows.remainder[0].IOWaitMs, 1) ||
		!hmc377UwNear(rows.remainder[0].ChainAnchoredMs, 2) || !hmc377UwNear(rows.remainder[0].ChainAnchorUnwalkedMs, 3) ||
		!hmc377UwNear(rows.remainder[0].ChainAnchorOutsideMs, 1) || !hmc377UwNear(rows.remainder[0].ChainAnchorFullMs, 6) {
		t.Fatalf("the remainder is the 1 ms of IO wait outside every dependency window (6 = 2 walked + 3 unwalked + 1): %+v", rows.remainder)
	}
}

// A satellite's time inside the unwalked waits leaves its remainder only to
// the one seat the ledger mints for the thread (HMC377 fix round 3: with no
// formal window seat in the pass the satellite lends its presentation
// identity, never its value — the seat carries the ledger's census share).
func TestHMC377SatelliteRemainderExcludesTheUnwalkedWaitShare(t *testing.T) {
	chain, stats, segments := hmc377UnwalkedChain()
	satellite := RootCauseRankItem{Type: "scheduler_latency", Thread: ThreadRef{Comm: "dep", PID: 200},
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
		RunnableMs: 9, ImpactMs: 9, CumulativeImpactMs: 9, EffectiveImpactMs: 9, Source: "scheduler_latency_stats", Confidence: 0.78,
		familyMemberIntervals: []foldInterval{segments[0], segments[1]}, MemberCount: 2}
	items := reanchorOnChainStateSeats(chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), satellite})
	for _, item := range items {
		if item.Type != "scheduler_latency" {
			continue
		}
		if item.ChainAnchorRemainderSeat && !hmc377UwNear(item.RunnableMs, 0) {
			t.Fatalf("a satellite fully inside the walked and unwalked waits has no outside remainder: %+v", item)
		}
		if strings.Contains(item.Summary, "remainder 4.000ms outside") {
			t.Fatalf("the unwalked share must not be called outside the dependency windows: %q", item.Summary)
		}
	}
	owners := 0
	for _, item := range items {
		if item.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState {
			owners++
			if !item.ChainAnchorUnwalkedSeat || !hmc377UwNear(item.RunnableMs, 4) || item.Type != "runnable_wait" || item.Source != "window_stats" {
				t.Fatalf("the one owner of the unwalked share is a plain ready-time seat holding the ledger's 4 ms: %+v", item)
			}
		}
	}
	if owners != 1 {
		t.Fatalf("exactly one seat owns the unwalked share: %d", owners)
	}
}
