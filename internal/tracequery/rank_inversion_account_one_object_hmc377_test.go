package tracequery

import (
	"math"
	"strings"
	"testing"
)

// HMC377 R-377-4 (owner ruling, §376 A audit failure 1) and E2-R1: one
// physical thread, one object. An indivisible same-CPU inversion window
// account (priority_inversion_runnable_wait: the raw ready account plus an
// overlap measured over all of it) keeps every value and is never split, but
// it must not ride as a separate time-adjacent "no chain credential"
// candidate when the same thread's ready time inside the analysed thread's
// waits is published on the chain. Real witnesses: Honeybird [11343.248,
// 11343.310) TID 64156 (Signal Catcher-64176 chain seat 36.311 beside a
// 45.878ms ◇ "same-core runnable overlap" row) and Yuanbao bindApplication
// TID 10600 (DefaultDispatch-10729 ready 7.220ms inside an 8.163ms wait it
// ended, on no face).

func hmc377InversionWindowRow(pid int, runnable, anchored, overlap float64) RootCauseRankItem {
	return RootCauseRankItem{Type: "priority_inversion_runnable_wait", Thread: ThreadRef{Comm: "dep", PID: pid},
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
		RunnableMs: runnable, ImpactMs: runnable, CumulativeImpactMs: runnable,
		EffectiveImpactMs: overlap, GatedRunnableMs: overlap, PriorityRelationProvenLowerMs: overlap,
		Score: overlap * 0.76, Source: "window_stats", Confidence: 0.76, LineStart: 50, LineEnd: 90,
		StatsWindowStartTs: 1.0, StatsWindowEndTs: 1.1,
		ledgerAnchorStamped: true, ledgerAnchoredRunnableMs: anchored}
}

func hmc377InversionChain() (ChainResult, WindowStats) {
	chain := ChainResult{
		Target: ThreadRef{PID: 100},
		Nodes: []ChainNode{
			{Thread: ThreadRef{PID: 200}, Depth: 1, Window: TimeWindow{StartTs: 1.0, EndTs: 1.03}},
		},
		CausalImpacts: []WakeupCausalImpact{
			{Thread: ThreadRef{PID: 200}, ChainDepth: 1, RunnableMs: 5.0},
		},
	}
	stats := WindowStats{
		chainAnchorsByPID:      chainAnchorWindowsByPID(chain),
		offCPUProducerDisjoint: true,
		runnableCensus: map[string]ThreadDuration{
			"200|0": {Thread: ThreadRef{PID: 200}, DurationMs: 8.0, anchoredMs: 5.0},
		},
	}
	return chain, stats
}

func hmc377InversionChainSeat(pid int) RootCauseRankItem {
	return RootCauseRankItem{Type: "priority_inversion_candidate", Thread: ThreadRef{Comm: "dep", PID: pid},
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
		RunnableMs: 5.0, GatedRunnableMs: 4.0, ImpactMs: 4.5, CumulativeImpactMs: 6.0, EffectiveImpactMs: 4.5,
		Source: "wakeup_chain.causal_impacts", Confidence: 0.91, LineStart: 52, LineEnd: 80,
		StartTs: 1.0, EndTs: 1.03}
}

func TestHMC377InversionWindowAccountJoinsItsChainSeat(t *testing.T) {
	chain, stats := hmc377InversionChain()
	window := hmc377InversionWindowRow(200, 8.0, 5.0, 2.0)
	before := window
	items := reanchorOnChainStateSeats(chain, stats, []RootCauseRankItem{hmc377InversionChainSeat(200), window})
	rank := RootCauseRankResult{Items: items, Window: TimeWindow{StartTs: 1.0, EndTs: 1.1}}
	reconcileExactCrossTypeRankSeats(&rank)
	var twin *RootCauseRankItem
	for i, item := range rank.Items {
		if item.Thread.PID == 200 && item.Type == "priority_inversion_runnable_wait" {
			t.Fatalf("the same thread's inversion window account must not stay a separate board object beside its chain seat: %+v", item)
		}
		if item.Thread.PID == 200 && item.Type == "runnable_wait" && item.Source == "window_stats" {
			twin = &rank.Items[i]
		}
	}
	// The ready time outside the dependency windows is different time: it is
	// the thread's plain ◇ ready-time remainder (the un-retyped case A/A'
	// form), never an inversion claim.
	if twin == nil || !twin.ChainAnchorRemainderSeat || twin.ChainRelevance != "adjacent" ||
		math.Abs(twin.RunnableMs-3.0) > 0.0005 || math.Abs(twin.EffectiveImpactMs-3.0) > 0.0005 ||
		math.Abs(twin.ChainAnchoredMs-5.0) > 0.0005 || math.Abs(twin.ChainAnchorFullMs-8.0) > 0.0005 ||
		twin.GatedRunnableMs != 0 || twin.PriorityRelationProvenLowerMs != 0 {
		t.Fatalf("outside share must be the plain ready-time remainder 3.000 of 8.000: %+v", twin)
	}
	if len(rank.AbsorbedItems) != 1 {
		t.Fatalf("the window account must ride as the chain seat's lossless detail: absorbed=%+v", rank.AbsorbedItems)
	}
	detail := rank.AbsorbedItems[0]
	if detail.ChainCredentialLaneDemoted {
		t.Fatalf("a represented account must not wear the no-chain-credential demotion: %+v", detail)
	}
	if detail.RunnableMs != before.RunnableMs || detail.EffectiveImpactMs != before.EffectiveImpactMs ||
		detail.GatedRunnableMs != before.GatedRunnableMs || detail.ImpactMs != before.ImpactMs || detail.Score != before.Score {
		t.Fatalf("the indivisible inversion measurement keeps every value: %+v", detail)
	}
	if !strings.Contains(detail.Summary, "represented by this thread's chain seat: 5.000ms of this 8.000ms ready-time account") {
		t.Fatalf("the detail must state the ready-account split in its own words: %q", detail.Summary)
	}
	var seat *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Thread.PID == 200 && rank.Items[i].Type == "priority_inversion_candidate" {
			seat = &rank.Items[i]
		}
	}
	if seat == nil || seat.AbsorbedRankRows != 1 || seat.EffectiveImpactMs != 4.5 {
		t.Fatalf("the chain seat stays the one object with its own value and the detail joined: %+v", seat)
	}
	// Idempotent over the second rank pass (reset-first recon).
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.AbsorbedItems) != 1 || seat.AbsorbedRankRows != 1 {
		t.Fatalf("second pass must rejoin the same pair: items=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
}

// Without exactly one chain-lane seat of the thread there is no proven
// representative: the former whole-seat demotion stands (no absorption).
func TestHMC377InversionWindowAccountWithoutUniqueChainSeatKeepsItsDemotion(t *testing.T) {
	chain, stats := hmc377InversionChain()
	second := hmc377InversionChainSeat(200)
	second.Type = "runnable_wait"
	second.LineStart, second.LineEnd = 81, 89
	items := reanchorOnChainStateSeats(chain, stats, []RootCauseRankItem{hmc377InversionChainSeat(200), second,
		hmc377InversionWindowRow(200, 8.0, 5.0, 2.0)})
	rank := RootCauseRankResult{Items: items, Window: TimeWindow{StartTs: 1.0, EndTs: 1.1}}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.AbsorbedItems) != 0 {
		t.Fatalf("an ambiguous representative must absorb nothing: %+v", rank.AbsorbedItems)
	}
	found := false
	for _, item := range rank.Items {
		if item.Type == "priority_inversion_runnable_wait" {
			found = true
			if !item.ChainCredentialLaneDemoted || item.ChainRelevance != "adjacent" {
				t.Fatalf("fallback keeps the former whole-seat demotion: %+v", item)
			}
		}
	}
	if !found {
		t.Fatal("the window account vanished")
	}
}

// E2-R1: a host that only delivered a wakeup, whose ready time partly lies
// inside the analysed thread's wait it ended, gets that inside share priced as
// its own on-chain scheduling-delay seat; the inversion row keeps every value
// and rides as that seat's detail (no refusal-only disclosure).
func TestHMC377HostEdgeInversionInsideShareIsPricedOnChain(t *testing.T) {
	chain := r3SyntheticChain()
	row := o3cRunnableSeat()
	row.Type = "priority_inversion_runnable_wait"
	row.GatedRunnableMs = 0.8
	row.EffectiveImpactMs = 0.8
	row.StatsWindowStartTs, row.StatsWindowEndTs = 6.000, 6.010
	before := row
	items := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{row})
	if len(items) != 3 {
		t.Fatalf("the inside share and the outside remainder must be minted as their own rows: %+v", items)
	}
	home, seat, rem := items[0], items[1], items[2]
	if rem.Type != "runnable_wait" || !rem.ChainAnchorRemainderSeat || rem.ChainRelevance != "adjacent" ||
		math.Abs(rem.RunnableMs-1.5) > 0.0005 || rem.GatedRunnableMs != 0 {
		t.Fatalf("the outside share is the plain ◇ ready-time remainder 1.500: %+v", rem)
	}
	if home.GatedCompositeEdgePreShareMs != 0 || home.GatedCompositeEdgePostShareMs != 0 {
		t.Fatalf("a priced inside share is not a refusal record: %+v", home)
	}
	if home.RunnableMs != before.RunnableMs || home.EffectiveImpactMs != before.EffectiveImpactMs ||
		home.ChainRelevance != before.ChainRelevance || home.OnChainBasis != "" {
		t.Fatalf("the inversion row keeps every value and its lane: %+v", home)
	}
	if seat.Type != "runnable_wait" || seat.ChainRelevance != "on_chain" ||
		seat.OnChainBasis != RootCauseOnChainBasisHostWakeupEdgeState {
		t.Fatalf("the inside share is an on-chain scheduling-delay seat: %+v", seat)
	}
	if math.Abs(seat.RunnableMs-3.5) > 0.0005 || math.Abs(seat.EffectiveImpactMs-3.5) > 0.0005 ||
		seat.GatedRunnableMs != 0 || seat.PriorityOverlap != nil || seat.PriorityRelationProvenLowerMs != 0 {
		t.Fatalf("the seat is the Σ of true ready segments inside the wait, with no inversion claim of its own: %+v", seat)
	}
	inside, _ := foldIntervalsLengthMs(seat.runnableIntervals)
	if math.Abs(inside-3.5) > 0.0005 {
		t.Fatalf("support inventory must reproduce the published value: %.3f", inside)
	}
	if !strings.Contains(seat.Summary, "overlap 0.800ms was measured over the whole 5.000ms ready-time account and is never split") {
		t.Fatalf("the seat must name the overlap with its own word: %q", seat.Summary)
	}
	rank := RootCauseRankResult{Items: items, Window: TimeWindow{StartTs: 6.000, EndTs: 6.010}}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 2 || rank.Items[0].Type != "runnable_wait" || rank.Items[1].Type != "runnable_wait" ||
		len(rank.AbsorbedItems) != 1 || rank.AbsorbedItems[0].Type != "priority_inversion_runnable_wait" {
		t.Fatalf("one thread, one object per time: items=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
	// Double pass: nothing re-mints from the already paired rows.
	again := anchorBareCensusEdgeStateSeats(chain, items)
	if len(again) != 3 {
		t.Fatalf("the second pass must not mint another seat: %d rows", len(again))
	}
}
