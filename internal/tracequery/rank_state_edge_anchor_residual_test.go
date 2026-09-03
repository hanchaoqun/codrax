package tracequery

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

// rank_state_edge_anchor_residual_test.go — STATERES-1 (user ruling §40.30
// V-STATE-1 plan A, 2026-09-02): "RSPA first, R3 residual" for chain-member
// hosts' runnable / D-IO seats, pinned on the r3SyntheticChain universe
// (target 100; relay 200 = chain member with node window 6.000..6.004 and a
// direct census edge toward the target at 6.0045; host 300 = bare-census host
// with a direct edge at 6.006).

// residualRelaySeat is the relay's window runnable seat whose ONLY segment
// lies outside its chain window and before its direct edge.
func residualRelaySeat() RootCauseRankItem {
	item := o3cRunnableSeat()
	item.Thread = ThreadRef{Comm: "relay", PID: 200}
	item.ChainRelevance, item.Causality = "adjacent", "adjacent_to_wakeup_chain"
	item.StartTs, item.EndTs = 6.0041, 6.0043
	item.runnableIntervals = []foldInterval{{start: 6.0041, end: 6.0043}} // 0.2ms, outside 6.000..6.004, before 6.0045
	item.RunnableMs, item.ImpactMs, item.CumulativeImpactMs, item.EffectiveImpactMs = 0.2, 0.2, 0.2, 0.2
	item.familyMemberIntervals = nil
	item.ledgerAnchorStamped = true
	item.ledgerAnchoredRunnableMs = 0
	return item
}

func TestChainMemberResidualStateSeatTakesDirectEdgeCredential(t *testing.T) {
	chain := r3SyntheticChain()
	items := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{residualRelaySeat()})
	if len(items) != 1 {
		t.Fatalf("a fully pre-edge residual converts whole (no ◇ clone): %+v", items)
	}
	seat := items[0]
	if !rootCauseItemIsOnChain(seat) || seat.OnChainBasis != RootCauseOnChainBasisHostWakeupEdgeState ||
		seat.HostWakeupEdgeAnchorVia != HostWakeupEdgeAnchorViaDirect || math.Abs(seat.HostWakeupEdgeAnchorTs-6.0045) > 1e-9 ||
		math.Abs(seat.EffectiveImpactMs-0.2) > 1e-9 {
		t.Fatalf("residual seat must take the DIRECT census edge credential whole: %+v", seat)
	}
	if !strings.Contains(seat.Summary, "fully pre-edge") || !strings.Contains(seat.Summary, "via=direct") {
		t.Fatalf("residual seat wears the R4 family sentence with via=direct: %s", seat.Summary)
	}
}

func TestChainMemberResidualDirectBoundaryIgnoresChainHopEdge(t *testing.T) {
	// The relay ALSO owns a later chain-hop edge (its own wakee on the path)
	// at 6.0048; the full credential would move the boundary there, the
	// residual lane must stay on the DIRECT census edge 6.0045.
	chain := r3SyntheticChain()
	chain.Edges = append(chain.Edges, WakeupEdge{Waker: ThreadRef{Comm: "relay", PID: 200}, Wakee: ThreadRef{Comm: "x", PID: 500}, WakeupTs: 6.0048, WakeupLine: 12, Branch: 1})
	if full, ok := hostSemanticSpanEdgeAnchor(&chain, ThreadRef{Comm: "relay", PID: 200}); !ok || math.Abs(full.boundaryTs-6.0048) > 1e-9 {
		t.Fatalf("fixture: the full credential must move to the chain-hop edge: %+v", full)
	}
	seat := residualRelaySeat()
	seat.runnableIntervals = []foldInterval{{start: 6.0041, end: 6.0047}} // straddles 6.0045: 0.4 pre / 0.2 post
	seat.StartTs, seat.EndTs = 6.0041, 6.0047
	seat.RunnableMs, seat.ImpactMs, seat.CumulativeImpactMs, seat.EffectiveImpactMs = 0.6, 0.6, 0.6, 0.6
	items := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{seat})
	if len(items) != 2 {
		t.Fatalf("a straddling residual bisects into ⛓ + ◇: %+v", items)
	}
	pre, post := items[0], items[1]
	if math.Abs(pre.HostWakeupEdgeAnchorTs-6.0045) > 1e-9 || pre.HostWakeupEdgeAnchorVia != HostWakeupEdgeAnchorViaDirect ||
		math.Abs(pre.EffectiveImpactMs-0.4) > 1e-9 || math.Abs(pre.ChainAnchorFullMs-0.6) > 1e-9 {
		t.Fatalf("the residual lane bisects at the DIRECT edge (6.0045), never the chain-hop edge: %+v", pre)
	}
	// The state-lane ◇ clone keeps the released share as its own adjacent
	// account (ONCHAIN-3c mint shape: value = post-edge share, lane adjacent,
	// never on the chain tier) and cites the same direct edge.
	if !post.ChainAnchorRemainderSeat || rootCauseItemIsOnChain(post) || math.Abs(post.ProjectedImpactMs-0.2) > 1e-9 ||
		math.Abs(post.HostWakeupEdgeAnchorTs-6.0045) > 1e-9 {
		t.Fatalf("the post-edge share rides the ◇ clone released at the direct edge: %+v", post)
	}
	// Wording: the residual lane's sentences name the DIRECT edge, never
	// "the host's latest typed wakeup edge" (its chain-hop edge is later).
	for _, summary := range []string{pre.Summary, post.Summary} {
		if !strings.Contains(summary, "DIRECT wakeup edge") || strings.Contains(summary, "typed wakeup edge") {
			t.Fatalf("residual-lane disclosure must speak the direct edge: %s", summary)
		}
	}
}

func TestChainMemberResidualRefusedForms(t *testing.T) {
	chain := r3SyntheticChain()
	chainHopOnly := r3SyntheticChain()
	chainHopOnly.WakeupEdgeCensus = chainHopOnly.WakeupEdgeCensus[:1] // relay keeps only its chain edge
	forms := map[string]func() (ChainResult, RootCauseRankItem){
		"chain-hop-only member (no direct census edge)": func() (ChainResult, RootCauseRankItem) {
			return chainHopOnly, residualRelaySeat()
		},
		"inventory overlaps the host's chain window (RSPA-owned)": func() (ChainResult, RootCauseRankItem) {
			item := residualRelaySeat()
			item.runnableIntervals = []foldInterval{{start: 6.0035, end: 6.0043}} // 0.5ms inside 6.000..6.004
			item.RunnableMs, item.ImpactMs, item.CumulativeImpactMs, item.EffectiveImpactMs = 0.8, 0.8, 0.8, 0.8
			return chain, item
		},
		"ledger stamp says RSPA priced part of the family": func() (ChainResult, RootCauseRankItem) {
			item := residualRelaySeat()
			item.ledgerAnchoredRunnableMs = 0.1
			return chain, item
		},
		"no ledger stamp (legacy / MAX-fallback fold)": func() (ChainResult, RootCauseRankItem) {
			item := residualRelaySeat()
			item.ledgerAnchorStamped = false
			return chain, item
		},
		"RSPA ◇ remainder is never re-judged": func() (ChainResult, RootCauseRankItem) {
			item := residualRelaySeat()
			item.ChainAnchorRemainderSeat = true
			item.ChainAnchoredMs, item.ChainAnchorFullMs = 0.3, 0.5
			return chain, item
		},
		"RSPA ⛓ clipped seat is never re-judged": func() (ChainResult, RootCauseRankItem) {
			item := residualRelaySeat()
			item.ChainRelevance, item.Causality = "on_chain", "on_wakeup_chain"
			item.ChainAnchoredMs, item.ChainAnchorFullMs = 0.2, 0.5
			return chain, item
		},
		"priority-inversion R4-mirror arm stays bare-host only (D1)": func() (ChainResult, RootCauseRankItem) {
			item := residualRelaySeat()
			item.Type = "priority_inversion_runnable_wait"
			item.GatedRunnableMs = 0.2
			return chain, item
		},
	}
	for name, build := range forms {
		universe, item := build()
		before := item
		items := anchorBareCensusEdgeStateSeats(universe, []RootCauseRankItem{item})
		if len(items) != 1 || !reflect.DeepEqual(items[0], before) {
			t.Fatalf("%s: the row must stay byte-identical (got %d rows, %+v)", name, len(items), items[0])
		}
	}
}

func TestReanchorOnChainStateSeatsKeepsHostEdgeStateBasisSeat(t *testing.T) {
	// The ENRICH pass runs RSPA before the state-edge pass: a whole-account
	// residual seat (on_chain, ChainAnchorFullMs==0) with an RSPA decision
	// (anchored 0 < full) must pass through byte-identically — lane decided
	// once (§40.30).
	chain := r3SyntheticChain()
	converted := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{residualRelaySeat()})[0]
	if converted.OnChainBasis != RootCauseOnChainBasisHostWakeupEdgeState || converted.ChainAnchorFullMs != 0 {
		t.Fatalf("fixture: expected a whole-account converted residual seat: %+v", converted)
	}
	stats := WindowStats{
		chainAnchorsByPID:      chainAnchorWindowsByPID(chain),
		offCPUProducerDisjoint: true,
		runnableCensus: map[string]ThreadDuration{
			"200|0": {Thread: ThreadRef{Comm: "relay", PID: 200}, DurationMs: 0.2, anchoredMs: 0},
		},
	}
	before := converted
	items := reanchorOnChainStateSeats(chain, stats, []RootCauseRankItem{converted})
	if len(items) != 1 || !reflect.DeepEqual(items[0], before) {
		t.Fatalf("RSPA must not re-judge a seat that already wears a typed on-chain basis: %+v", items)
	}
}

// residualRelayDIOSeat is the relay's D-state window seat whose only segment
// lies outside its chain window and before its direct edge.
func residualRelayDIOSeat() RootCauseRankItem {
	item := RootCauseRankItem{
		Type: "d_state_or_io_wait", Source: "window_stats",
		Thread: ThreadRef{Comm: "relay", PID: 200}, Confidence: 0.7,
		DominantState: string(StateDSleep), ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain",
		StartTs: 6.0041, EndTs: 6.0043,
		ImpactMs: 0.2, CumulativeImpactMs: 0.2, EffectiveImpactMs: 0.2, DStateMs: 0.2,
		memberSegmentsProducerDisjoint: true,
		dioSegmentIntervalsD:           []foldInterval{{start: 6.0041, end: 6.0043}},
		dioSegmentIntervals:            []foldInterval{{start: 6.0041, end: 6.0043}},
		ledgerAnchorStamped:            true,
	}
	return item
}

// 复核收编 (batch-two adversarial review): the D/IO residual arm carries its
// own positive and belt pins (the runnable-only forms left the D/IO ledger
// belts unpinned).
func TestChainMemberResidualDIOSeatTakesDirectEdgeCredentialAndBelts(t *testing.T) {
	chain := r3SyntheticChain()
	items := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{residualRelayDIOSeat()})
	if len(items) != 1 {
		t.Fatalf("a fully pre-edge D/IO residual converts whole: %+v", items)
	}
	seat := items[0]
	if seat.OnChainBasis != RootCauseOnChainBasisHostWakeupEdgeState || seat.HostWakeupEdgeAnchorVia != HostWakeupEdgeAnchorViaDirect ||
		math.Abs(seat.EffectiveImpactMs-0.2) > 1e-9 || math.Abs(seat.HostWakeupEdgeAnchorTs-6.0045) > 1e-9 {
		t.Fatalf("D/IO residual seat must take the direct-edge credential whole: %+v", seat)
	}
	forms := map[string]func() RootCauseRankItem{
		"ledger anchored IO share > 0 (RSPA priced part of the family)": func() RootCauseRankItem {
			item := residualRelayDIOSeat()
			item.ledgerAnchoredIOMs = 0.05
			return item
		},
		"ledger anchored D share > 0": func() RootCauseRankItem {
			item := residualRelayDIOSeat()
			item.ledgerAnchoredDMs = 0.05
			return item
		},
		"no ledger stamp (MAX-fallback fold)": func() RootCauseRankItem {
			item := residualRelayDIOSeat()
			item.ledgerAnchorStamped = false
			return item
		},
		"segment overlaps the chain window": func() RootCauseRankItem {
			item := residualRelayDIOSeat()
			item.dioSegmentIntervalsD = []foldInterval{{start: 6.0035, end: 6.0043}}
			item.dioSegmentIntervals = item.dioSegmentIntervalsD
			item.DStateMs, item.ImpactMs, item.CumulativeImpactMs, item.EffectiveImpactMs = 0.8, 0.8, 0.8, 0.8
			return item
		},
	}
	for name, build := range forms {
		item := build()
		before := item
		out := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{item})
		if len(out) != 1 || !reflect.DeepEqual(out[0], before) {
			t.Fatalf("%s: the row must stay byte-identical (got %d rows, %+v)", name, len(out), out[0])
		}
	}
}

// 复核收编: RSPA's ZERO-anchored whole-account ◇ rewrite (a D/IO seat whose
// hull crossed the chain window while every segment lay outside it) is the
// residual population itself and converts; a genuine RSPA partition
// (anchored share > 0) is never re-judged.
func TestChainMemberResidualAdmitsRSPAZeroAnchoredWholeRemainder(t *testing.T) {
	chain := r3SyntheticChain()
	chain.Nodes[1].Window = TimeWindow{StartTs: 6.002, EndTs: 6.003} // relay's chain window: both segments lie outside it
	zero := residualRelayDIOSeat()
	zero.dioSegmentIntervalsD = []foldInterval{{start: 6.0005, end: 6.0010}, {start: 6.0041, end: 6.0043}}
	zero.dioSegmentIntervals = zero.dioSegmentIntervalsD
	zero.StartTs, zero.EndTs = 6.0005, 6.0043 // hull crosses the relay's chain window 6.000..6.004
	zero.DStateMs, zero.ImpactMs, zero.CumulativeImpactMs, zero.EffectiveImpactMs = 0.7, 0.7, 0.7, 0.7
	// The shape RSPA leaves behind for anchored==0 (rspaRewriteSeatToRemainder).
	zero.ChainAnchorRemainderSeat, zero.ChainAnchoredMs, zero.ChainAnchorFullMs = true, 0, 0.7
	items := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{zero})
	if len(items) != 1 {
		t.Fatalf("both segments lie before the direct edge — whole conversion, no ◇ twin: %+v", items)
	}
	seat := items[0]
	if seat.ChainAnchorRemainderSeat || seat.OnChainBasis != RootCauseOnChainBasisHostWakeupEdgeState ||
		seat.HostWakeupEdgeAnchorVia != HostWakeupEdgeAnchorViaDirect || math.Abs(seat.EffectiveImpactMs-0.7) > 1e-9 {
		t.Fatalf("the zero-anchored whole remainder must convert into the residual ⛓ seat: %+v", seat)
	}
	partition := residualRelayDIOSeat()
	partition.ChainAnchorRemainderSeat, partition.ChainAnchoredMs, partition.ChainAnchorFullMs = true, 0.3, 0.5
	before := partition
	items = anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{partition})
	if len(items) != 1 || !reflect.DeepEqual(items[0], before) {
		t.Fatalf("an RSPA partition remainder (anchored share > 0) must stay byte-identical: %+v", items)
	}
}
