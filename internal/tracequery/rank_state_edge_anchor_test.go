package tracequery

// rank_state_edge_anchor_test.go — ONCHAIN-3c acceptance pins (bare-census-
// edge hosts' runnable / D-IO state seats on the R3 host-edge credential;
// mint audit 反向缺口5, eval docs/design/edge3be_eval_20260719.md §3c).
//
// Live-trace sentinel pins ride TestRSPATiebaWitnessBoard (the SCAN-3 61839
// 判例: io_wait 3.550 fully pre-edge + runnable 0.370⛓/0.075◇ = 0.445) and
// the donghu board evolutions (gpu-token-id4-2931 R4-mirror inversion form).
// This file pins the mechanism arms on the r3SyntheticChain universe
// (boundary for hostw-300 = its direct census edge at 6.006).
//
// MUTATION self-checks (突变电池对应): flipping the pre/post comparison in
// semanticEdgeAnchorSplit consumption, deleting the chain-member pid guard,
// deleting the Σ-identity fail-close, apportioning the D/IO per-state split
// off the union list, or dropping the ◇ remainder clone each reds a pin here.

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

// o3cSeatFullAccountHull — the per-(thread,cpu) group hull describing the
// PRE-SPLIT full account (the shape query.go's fold pass stamps). Both fixture
// seats carry it so the hull-clearing arms in convertStateSeatToEdgeAnchored /
// mintStateEdgeRemainderClone are asserted against a NON-empty input (a nil
// fixture hull would leave the clearing assertion vacuous — mutation M10).
func o3cSeatFullAccountHull() []foldInterval {
	return []foldInterval{{start: 6.001, end: 6.0085, valueMs: 5.0}}
}

func o3cRunnableSeat() RootCauseRankItem {
	item := RootCauseRankItem{
		Type: "runnable_wait", Source: "window_stats",
		Thread:     ThreadRef{Comm: "hostw", PID: 300},
		Confidence: 0.76, DominantState: string(StateRunnable),
		ChainRelevance: "background", Causality: "background",
		StartTs: 6.001, EndTs: 6.0085,
		ImpactMs: 5.0, CumulativeImpactMs: 5.0, EffectiveImpactMs: 5.0,
		RunnableMs: 5.0, LineStart: 10, LineEnd: 20,
		memberSegmentsProducerDisjoint: true,
		familyMemberIntervals:          o3cSeatFullAccountHull(),
		runnableIntervals: []foldInterval{
			{start: 6.001, end: 6.003},  // 2.0ms fully pre-edge
			{start: 6.004, end: 6.005},  // 1.0ms fully pre-edge
			{start: 6.0055, end: 6.007}, // 1.5ms straddles 6.006: 0.5 pre / 1.0 post
			{start: 6.008, end: 6.0085}, // 0.5ms fully post-edge
		},
	}
	return item
}

func o3cMixedDIOSeat() RootCauseRankItem {
	dSegs := []foldInterval{
		{start: 6.001, end: 6.002},   // 1.0ms D pre
		{start: 6.0055, end: 6.0065}, // 1.0ms D straddling: 0.5 pre / 0.5 post
	}
	ioSegs := []foldInterval{
		// 1.0ms IO straddling: 0.2 pre / 0.8 post — BOTH states must carry a
		// non-zero pre-edge share so a union-apportioned per-state split
		// (mutation M4) cannot be behaviorally equivalent to the exact split.
		{start: 6.0058, end: 6.0068},
	}
	union := append(append([]foldInterval(nil), dSegs...), ioSegs...)
	return RootCauseRankItem{
		Type: "d_state_or_io_wait", Source: "window_stats",
		Thread:     ThreadRef{Comm: "hostw", PID: 300},
		Confidence: 0.82, DominantState: string(StateDSleep),
		ChainRelevance: "background", Causality: "background",
		StartTs: 6.001, EndTs: 6.008,
		ImpactMs: 3.0, CumulativeImpactMs: 3.0, EffectiveImpactMs: 3.0,
		DStateMs: 2.0, IOWaitMs: 1.0, LineStart: 30, LineEnd: 40,
		memberSegmentsProducerDisjoint: true,
		familyMemberIntervals:          []foldInterval{{start: 6.001, end: 6.008, valueMs: 3.0}},
		dioSegmentIntervals:            union,
		dioSegmentIntervalsD:           dSegs,
		dioSegmentIntervalsIO:          ioSegs,
	}
}

func o3cFindSeats(items []RootCauseRankItem, typ string) (seat, rem *RootCauseRankItem) {
	for i := range items {
		if items[i].Type != typ {
			continue
		}
		if items[i].ChainAnchorRemainderSeat {
			rem = &items[i]
		} else {
			seat = &items[i]
		}
	}
	return seat, rem
}

// TestBareCensusEdgeStateSeatRunnableBisect — the runnable seat straddling the
// credential boundary bisects: ⛓ pre-edge seat + ◇ post-edge remainder clone,
// exact partition, clipped inventories, typed basis/causality/edge pair, and
// the RSPA verbatim twin anchors on both sentences.
func TestBareCensusEdgeStateSeatRunnableBisect(t *testing.T) {
	chain := r3SyntheticChain()
	items := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{o3cRunnableSeat()})
	if len(items) != 2 {
		t.Fatalf("bisection must append exactly the ◇ remainder twin: %d rows", len(items))
	}
	seat, rem := o3cFindSeats(items, "runnable_wait")
	if seat == nil || rem == nil {
		t.Fatalf("both halves must publish: %+v", items)
	}
	// ⛓ half: lane/basis/causality + the typed edge pair.
	if seat.ChainRelevance != "on_chain" || seat.Causality != "on_wakeup_chain" ||
		seat.OnChainBasis != RootCauseOnChainBasisHostWakeupEdgeState {
		t.Fatalf("⛓ half lane/basis drifted: %+v", seat)
	}
	if seat.HostWakeupEdgeAnchorTs != 6.006 || seat.HostWakeupEdgeAnchorVia != HostWakeupEdgeAnchorViaDirect {
		t.Fatalf("⛓ half edge pair drifted: %+v", seat)
	}
	// Hand-computed values: pre = 2.0 + 1.0 + 0.5 = 3.5; post = 1.0 + 0.5 = 1.5.
	if math.Abs(seat.RunnableMs-3.5) > 0.0005 || math.Abs(seat.CumulativeImpactMs-3.5) > 0.0005 ||
		math.Abs(seat.EffectiveImpactMs-3.5) > 0.0005 {
		t.Fatalf("⛓ half value drifted from the hand-computed 3.5: %+v", seat)
	}
	if math.Abs(seat.ChainAnchoredMs-3.5) > 0.0005 || math.Abs(seat.ChainAnchorFullMs-5.0) > 0.0005 {
		t.Fatalf("⛓ half bipartition trio drifted: %+v", seat)
	}
	if math.Abs(rem.CumulativeImpactMs-1.5) > 0.0005 || math.Abs(rem.RunnableMs-1.5) > 0.0005 {
		t.Fatalf("◇ half value drifted from the hand-computed 1.5: %+v", rem)
	}
	if !rem.ChainAnchorRemainderSeat || rem.ChainRelevance != "adjacent" ||
		rem.Causality != "adjacent_to_wakeup_chain" || rem.OnChainBasis != "" {
		t.Fatalf("◇ half lane drifted: %+v", rem)
	}
	if rem.HostWakeupEdgeAnchorTs != 6.006 {
		t.Fatalf("◇ half must keep the boundary disclosure: %+v", rem)
	}
	// Partition identity: full = pre + post, exactly.
	if math.Abs(seat.CumulativeImpactMs+rem.CumulativeImpactMs-seat.ChainAnchorFullMs) > 0.0005 {
		t.Fatalf("partition identity broken: %.3f + %.3f != %.3f",
			seat.CumulativeImpactMs, rem.CumulativeImpactMs, seat.ChainAnchorFullMs)
	}
	// Clipped inventories: each half's segment union reproduces ITS value
	// (the AXIOM-V2 support==claim identity on both halves).
	preLen, _ := foldIntervalsLengthMs(seat.runnableIntervals)
	postLen, _ := foldIntervalsLengthMs(rem.runnableIntervals)
	if math.Abs(preLen-3.5) > 0.0005 || math.Abs(postLen-1.5) > 0.0005 {
		t.Fatalf("clipped inventories drifted: pre=%.3f post=%.3f", preLen, postLen)
	}
	if len(seat.runnableIntervals) != 3 || len(rem.runnableIntervals) != 2 {
		t.Fatalf("clipped segment counts drifted: %d/%d", len(seat.runnableIntervals), len(rem.runnableIntervals))
	}
	// Extents follow each half's own segments.
	if seat.StartTs != 6.001 || seat.EndTs != 6.006 || rem.StartTs != 6.006 || rem.EndTs != 6.0085 {
		t.Fatalf("half extents drifted: seat=%.4f..%.4f rem=%.4f..%.4f",
			seat.StartTs, seat.EndTs, rem.StartTs, rem.EndTs)
	}
	// The group hulls are cleared on both halves (they describe the pre-split
	// full account; the fixture carries a NON-empty hull so this bites —
	// mutation M10 self-check).
	if len(seat.familyMemberIntervals) != 0 || len(rem.familyMemberIntervals) != 0 {
		t.Fatalf("group hulls must be cleared on both halves")
	}
	// Sentences: the R4 family language + the RSPA verbatim twin anchors (the
	// existing twin-visibility pass owns their downgrade).
	if !strings.Contains(seat.Summary, "edge=credential") || !strings.Contains(seat.Summary, rspaSummaryRemainderTwinPublished) {
		t.Fatalf("⛓ sentence must speak edge=credential + the twin anchor: %q", seat.Summary)
	}
	if !strings.Contains(rem.Summary, "edge=credential") || !strings.Contains(rem.Summary, rspaSummaryOwnedByChainSeat) {
		t.Fatalf("◇ sentence must speak edge=credential + the ownership anchor: %q", rem.Summary)
	}
	// Score re-derives from the published value (§7.30 S1).
	if math.Abs(seat.Score-3.5*seat.Confidence*rootCauseItemScoreWeight(*seat)) > 0.0005 {
		t.Fatalf("⛓ Score must re-derive from the published value: %+v", seat)
	}
}

// TestBareCensusEdgeStateSeatMixedDIOPerState — the mixed D+IO seat splits
// PER STATE off the per-state carriers (exact, never an apportionment).
func TestBareCensusEdgeStateSeatMixedDIOPerState(t *testing.T) {
	chain := r3SyntheticChain()
	items := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{o3cMixedDIOSeat()})
	if len(items) != 2 {
		t.Fatalf("bisection must append exactly the ◇ remainder twin: %d rows", len(items))
	}
	seat, rem := o3cFindSeats(items, "d_state_or_io_wait")
	if seat == nil || rem == nil {
		t.Fatalf("both halves must publish: %+v", items)
	}
	// Hand-computed per-state split at 6.006: D pre 1.5 / post 0.5; IO pre 0.2
	// / post 0.8 → ⛓ 1.7 (1.5 D + 0.2 IO), ◇ 1.3 (0.5 D + 0.8 IO). Both state
	// channels carry non-zero pre shares, so a union-apportioned split can
	// never reproduce this pin (mutation M4 self-check).
	if math.Abs(seat.DStateMs-1.5) > 0.0005 || math.Abs(seat.IOWaitMs-0.2) > 0.0005 ||
		math.Abs(seat.CumulativeImpactMs-1.7) > 0.0005 {
		t.Fatalf("⛓ per-state split drifted: %+v", seat)
	}
	if math.Abs(rem.DStateMs-0.5) > 0.0005 || math.Abs(rem.IOWaitMs-0.8) > 0.0005 ||
		math.Abs(rem.CumulativeImpactMs-1.3) > 0.0005 {
		t.Fatalf("◇ per-state split drifted: %+v", rem)
	}
	if math.Abs(seat.ChainAnchoredMs-1.7) > 0.0005 || math.Abs(seat.ChainAnchorFullMs-3.0) > 0.0005 {
		t.Fatalf("⛓ trio drifted: %+v", seat)
	}
	if seat.OnChainBasis != RootCauseOnChainBasisHostWakeupEdgeState || rem.OnChainBasis != "" {
		t.Fatalf("basis stamps drifted: %+v / %+v", seat, rem)
	}
	// The rebuilt union carriers partition per half.
	preLen, _ := foldIntervalsLengthMs(seat.dioSegmentIntervals)
	postLen, _ := foldIntervalsLengthMs(rem.dioSegmentIntervals)
	if math.Abs(preLen-1.7) > 0.0005 || math.Abs(postLen-1.3) > 0.0005 {
		t.Fatalf("half union carriers drifted: pre=%.3f post=%.3f", preLen, postLen)
	}
	dPre, _ := foldIntervalsLengthMs(seat.dioSegmentIntervalsD)
	ioPre, _ := foldIntervalsLengthMs(seat.dioSegmentIntervalsIO)
	if math.Abs(dPre-1.5) > 0.0005 || math.Abs(ioPre-0.2) > 0.0005 {
		t.Fatalf("⛓ per-state carriers drifted: d=%.3f io=%.3f", dPre, ioPre)
	}
	// The group hulls are cleared on both halves here too (non-empty in the
	// fixture — mutation M10 self-check, D/IO arm).
	if len(seat.familyMemberIntervals) != 0 || len(rem.familyMemberIntervals) != 0 {
		t.Fatalf("group hulls must be cleared on both D/IO halves")
	}
}

// TestBareCensusEdgeStateSeatFullyPreEdge — every segment pre-edge: the seat
// converts whole (no trio, no clone), value channels re-published undiscounted.
func TestBareCensusEdgeStateSeatFullyPreEdge(t *testing.T) {
	chain := r3SyntheticChain()
	item := o3cRunnableSeat()
	item.runnableIntervals = []foldInterval{{start: 6.001, end: 6.003}, {start: 6.004, end: 6.005}}
	item.RunnableMs, item.CumulativeImpactMs, item.EffectiveImpactMs = 3.0, 3.0, 3.0
	item.ImpactMs = 1.2 // background-discounted mint value — must re-publish
	items := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{item})
	if len(items) != 1 {
		t.Fatalf("fully pre-edge conversion must mint NO remainder clone: %d rows", len(items))
	}
	seat := items[0]
	if seat.OnChainBasis != RootCauseOnChainBasisHostWakeupEdgeState || seat.ChainRelevance != "on_chain" {
		t.Fatalf("whole conversion drifted: %+v", seat)
	}
	if seat.ChainAnchorFullMs != 0 || seat.ChainAnchoredMs != 0 || seat.ChainAnchorRemainderSeat {
		t.Fatalf("whole conversion must carry no bipartition trio (span-basis fully-pre form): %+v", seat)
	}
	if math.Abs(seat.ImpactMs-3.0) > 0.0005 || math.Abs(seat.EffectiveImpactMs-3.0) > 0.0005 {
		t.Fatalf("whole conversion must publish the full account on every value channel: %+v", seat)
	}
	if !strings.Contains(seat.Summary, "fully pre-edge") {
		t.Fatalf("whole conversion sentence drifted: %q", seat.Summary)
	}
	if len(seat.familyMemberIntervals) != 0 {
		t.Fatalf("the whole-conversion seat must clear the group hull too (its inventory replaced the hull as the exact carrier)")
	}
}

// TestBareCensusEdgeStateSeatInversionR4Mirror — the inversion-rewritten seat
// is an indivisible gated composite: fully pre-edge → whole-seat lane change
// with every published value untouched; ANY post-edge share → untouched.
func TestBareCensusEdgeStateSeatInversionR4Mirror(t *testing.T) {
	chain := r3SyntheticChain()
	base := o3cRunnableSeat()
	base.Type = "priority_inversion_runnable_wait"
	base.GatedRunnableMs = 0.8
	base.EffectiveImpactMs = 0.8
	// ① fully pre-edge: lane changes whole, values untouched.
	full := base
	full.runnableIntervals = []foldInterval{{start: 6.001, end: 6.003}, {start: 6.004, end: 6.005}}
	full.RunnableMs, full.CumulativeImpactMs = 3.0, 3.0
	items := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{full})
	if len(items) != 1 {
		t.Fatalf("① no clone may mint for the indivisible composite")
	}
	got := items[0]
	if got.ChainRelevance != "on_chain" || got.OnChainBasis != RootCauseOnChainBasisHostWakeupEdgeState {
		t.Fatalf("① fully pre-edge inversion seat must change lane: %+v", got)
	}
	if math.Abs(got.EffectiveImpactMs-0.8) > 0.0005 || math.Abs(got.CumulativeImpactMs-3.0) > 0.0005 ||
		math.Abs(got.RunnableMs-3.0) > 0.0005 {
		t.Fatalf("① every published value must stay untouched (gated composite never split): %+v", got)
	}
	if !strings.Contains(got.Summary, "the gated composite is never split") {
		t.Fatalf("① disclosure sentence drifted: %q", got.Summary)
	}
	// ② any post-edge share: the seat's published authority stays untouched —
	// EVOLUTION RECORD (PARTSPLIT-1 §29.150④, 2026-07-19): the former
	// byte-identical assertion evolves DELIBERATELY into the 拒转+披露 form:
	// the refusal now goes on record as the four disclosure-only measure
	// fields (X=3.5 pre + Y=1.5 post == the 5.0 runnable account, boundary
	// 6.006, via=direct), while EVERY pre-existing field stays byte-identical
	// (value/lane/ordinal zero-motion — the R4 whole-seat floor holds).
	partial := base // segments straddle the 6.006 boundary
	before := partial
	items = anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{partial})
	if len(items) != 1 {
		t.Fatalf("② no clone may mint for the refused composite: %+v", items)
	}
	refused := items[0]
	if math.Abs(refused.GatedCompositeEdgePreShareMs-3.5) > 0.0005 ||
		math.Abs(refused.GatedCompositeEdgePostShareMs-1.5) > 0.0005 ||
		math.Abs(refused.GatedCompositeEdgeAnchorTs-6.006) > 0.0000005 ||
		refused.GatedCompositeEdgeAnchorVia != HostWakeupEdgeAnchorViaDirect {
		t.Fatalf("② the refusal record must carry the bisection measures: %+v", refused)
	}
	// µs identity: X + Y == the runnable census account, exactly.
	if math.Abs(refused.GatedCompositeEdgePreShareMs+refused.GatedCompositeEdgePostShareMs-refused.RunnableMs) > 0.0000005 {
		t.Fatalf("② X+Y==RunnableMs identity broken: %.6f + %.6f != %.6f",
			refused.GatedCompositeEdgePreShareMs, refused.GatedCompositeEdgePostShareMs, refused.RunnableMs)
	}
	// Published authority zero-motion: strip the four disclosure fields and
	// the row is byte-identical to its pre-pass copy.
	stripped := refused
	stripped.GatedCompositeEdgePreShareMs = 0
	stripped.GatedCompositeEdgePostShareMs = 0
	stripped.GatedCompositeEdgeAnchorTs = 0
	stripped.GatedCompositeEdgeAnchorVia = ""
	if !reflect.DeepEqual(stripped, before) {
		t.Fatalf("② beyond the disclosure fields the refused seat must stay byte-identical:\n got=%+v\nwant=%+v", stripped, before)
	}
	// Idempotency: the second pass re-stamps the same deterministic record.
	again := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{refused})
	if len(again) != 1 || !reflect.DeepEqual(again[0], refused) {
		t.Fatalf("② the refusal record must be idempotent across the double pass: %+v", again)
	}
}

// TestBareCensusEdgeStateSeatFailClosedForms — every fail-closed roster form
// keeps the row byte-identical and mints nothing.
func TestBareCensusEdgeStateSeatFailClosedForms(t *testing.T) {
	chain := r3SyntheticChain()
	forms := map[string]func() (ChainResult, RootCauseRankItem){
		"no credential (edge-less host)": func() (ChainResult, RootCauseRankItem) {
			item := o3cRunnableSeat()
			item.Thread = ThreadRef{Comm: "bystander", PID: 400}
			return chain, item
		},
		"chain-member pid (RSPA vocabulary ownership)": func() (ChainResult, RootCauseRankItem) {
			item := o3cRunnableSeat()
			item.Thread = ThreadRef{Comm: "relay", PID: 200}
			return chain, item
		},
		"analysis target (self-causality carve)": func() (ChainResult, RootCauseRankItem) {
			item := o3cRunnableSeat()
			item.Thread = ThreadRef{Comm: "app", PID: 100}
			return chain, item
		},
		"degenerate chain window (fail closed)": func() (ChainResult, RootCauseRankItem) {
			degenerate := r3SyntheticChain()
			degenerate.Window = TimeWindow{}
			return degenerate, o3cRunnableSeat()
		},
		"Σ-broken inventory": func() (ChainResult, RootCauseRankItem) {
			item := o3cRunnableSeat()
			item.RunnableMs = 9.0
			return chain, item
		},
		"producer disjointness lost (MAX-fallback premise)": func() (ChainResult, RootCauseRankItem) {
			item := o3cRunnableSeat()
			item.memberSegmentsProducerDisjoint = false
			return chain, item
		},
		"missing per-state D/IO carriers": func() (ChainResult, RootCauseRankItem) {
			item := o3cMixedDIOSeat()
			item.dioSegmentIntervalsD, item.dioSegmentIntervalsIO = nil, nil
			return chain, item
		},
		"all segments post-edge (边后=解除 grants nothing)": func() (ChainResult, RootCauseRankItem) {
			item := o3cRunnableSeat()
			item.runnableIntervals = []foldInterval{{start: 6.007, end: 6.009}}
			item.RunnableMs, item.CumulativeImpactMs = 2.0, 2.0
			return chain, item
		},
		"already adjudicated (self basis)": func() (ChainResult, RootCauseRankItem) {
			item := o3cRunnableSeat()
			item.OnChainBasis = RootCauseOnChainBasisSelfWallClockInterval
			return chain, item
		},
		"already a remainder clone": func() (ChainResult, RootCauseRankItem) {
			item := o3cRunnableSeat()
			item.ChainAnchorRemainderSeat = true
			return chain, item
		},
		"periodic-discounted account": func() (ChainResult, RootCauseRankItem) {
			item := o3cRunnableSeat()
			item.PeriodicSource = true
			return chain, item
		},
		"already on the chain tier": func() (ChainResult, RootCauseRankItem) {
			item := o3cRunnableSeat()
			item.ChainRelevance = "on_chain"
			item.Causality = "on_wakeup_chain"
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

// TestBareCensusEdgeStateSeatIdempotent — the converted board re-enters the
// pass unchanged (the build+enrich double-pass discipline).
func TestBareCensusEdgeStateSeatIdempotent(t *testing.T) {
	chain := r3SyntheticChain()
	once := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{o3cRunnableSeat(), o3cMixedDIOSeat()})
	twice := anchorBareCensusEdgeStateSeats(chain, append([]RootCauseRankItem(nil), once...))
	if !reflect.DeepEqual(once, twice) {
		t.Fatalf("the pass must be idempotent:\nonce=%+v\ntwice=%+v", once, twice)
	}
}

// TestBareCensusEdgeStateSeatDirectionPopulation — the state basis enters the
// AXIOM-V2 strict full-seat population with its clipped inventory (support
// union == published value by construction).
func TestBareCensusEdgeStateSeatDirectionPopulation(t *testing.T) {
	chain := r3SyntheticChain()
	items := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{o3cRunnableSeat()})
	seat, rem := o3cFindSeats(items, "runnable_wait")
	seat.Rank = 3
	if !rootCauseItemDirectionPopulationEligible(seat) {
		t.Fatalf("the ⛓ state seat must enter the direction population: %+v", seat)
	}
	intervals, basis := rootCauseItemDirectionSupport(seat)
	if basis != RootCauseDirectionBasisRunnable {
		t.Fatalf("support basis drifted: %q", basis)
	}
	supportMs, _ := foldIntervalsLengthMs(intervals)
	if math.Abs(supportMs-seat.CumulativeImpactMs) > 0.0005 {
		t.Fatalf("support union must reproduce the published value: %.3f vs %.3f", supportMs, seat.CumulativeImpactMs)
	}
	// The ◇ remainder clone never re-enters as a full seat.
	rem.Rank = 4
	if rootCauseItemDirectionPopulationEligible(rem) {
		t.Fatalf("the ◇ remainder must stay out of the population: %+v", rem)
	}
}

// TestBareCensusEdgeStateSeatTwinVisibility — the truncation-killed twin
// downgrades through the EXISTING rspaPatchSummariesForTwinVisibility pass
// (verbatim anchors shared by design).
func TestBareCensusEdgeStateSeatTwinVisibility(t *testing.T) {
	chain := r3SyntheticChain()
	items := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{o3cRunnableSeat()})
	seat, rem := o3cFindSeats(items, "runnable_wait")
	// ① clone truncated: the ⛓ sentence's co-publication claim downgrades.
	onlySeat := []RootCauseRankItem{*seat}
	rspaPatchSummariesForTwinVisibility(onlySeat)
	if !strings.Contains(onlySeat[0].Summary, rspaSummaryRemainderTwinUnpublished) {
		t.Fatalf("① the ⛓ sentence must downgrade honestly: %q", onlySeat[0].Summary)
	}
	// ② ⛓ half truncated: the ◇ ownership claim downgrades.
	onlyRem := []RootCauseRankItem{*rem}
	rspaPatchSummariesForTwinVisibility(onlyRem)
	if !strings.Contains(onlyRem[0].Summary, rspaSummaryOwnedByChainSeatUnpublished) {
		t.Fatalf("② the ◇ sentence must downgrade honestly: %q", onlyRem[0].Summary)
	}
	// ③ both published: both sentences keep their claims byte-identically.
	both := []RootCauseRankItem{*seat, *rem}
	rspaPatchSummariesForTwinVisibility(both)
	if !strings.Contains(both[0].Summary, rspaSummaryRemainderTwinPublished) ||
		!strings.Contains(both[1].Summary, rspaSummaryOwnedByChainSeat) {
		t.Fatalf("③ co-published twins must keep their claims")
	}
}

// TestBareCensusEdgeHostRunnableMintSet — the mint-domain widening admits
// exactly the hosts the pass will adjudicate.
func TestBareCensusEdgeHostRunnableMintSet(t *testing.T) {
	chain := r3SyntheticChain()
	chainThreads := wakeupChainThreadSet(chain)
	census := map[string]ThreadDuration{
		"hostw/300/2": {Thread: ThreadRef{Comm: "hostw", PID: 300}, CPU: 2, DurationMs: 3.0,
			runnableIntervals: []foldInterval{{start: 6.001, end: 6.003}, {start: 6.004, end: 6.005}}},
		"relay/200/1": {Thread: ThreadRef{Comm: "relay", PID: 200}, CPU: 1, DurationMs: 1.0,
			runnableIntervals: []foldInterval{{start: 6.001, end: 6.002}}},
	}
	set := bareCensusEdgeHostRunnableMintSet(chain, chainThreads, census, true)
	if len(set) != 1 || !set[300] {
		t.Fatalf("mint set must admit exactly the qualifying edge host: %v", set)
	}
	// Ordered-stream premise lost → nothing mints (fail closed).
	if set := bareCensusEdgeHostRunnableMintSet(chain, chainThreads, census, false); set != nil {
		t.Fatalf("a regressed trace must mint nothing: %v", set)
	}
	// A host whose whole account is post-edge never mints (边后=解除).
	postCensus := map[string]ThreadDuration{
		"hostw/300/2": {Thread: ThreadRef{Comm: "hostw", PID: 300}, CPU: 2, DurationMs: 1.0,
			runnableIntervals: []foldInterval{{start: 6.007, end: 6.008}}},
	}
	if set := bareCensusEdgeHostRunnableMintSet(chain, chainThreads, postCensus, true); set != nil {
		t.Fatalf("an all-post-edge host must not enter the mint set: %v", set)
	}
}

// TestMintDIOStateSeatPerStateCarriers — the mint stamps the per-state
// carriers in the same all-or-nothing block as the union (present together;
// each bucket single-state by construction).
func TestMintDIOStateSeatPerStateCarriers(t *testing.T) {
	dTd := ThreadDuration{Thread: ThreadRef{Comm: "w", PID: 9}, DurationMs: 4.0,
		dioIntervals: []foldInterval{{start: 1.0, end: 1.002}, {start: 1.01, end: 1.012}}}
	ioTd := ThreadDuration{Thread: ThreadRef{Comm: "w", PID: 9}, DurationMs: 2.0,
		dioIntervals: []foldInterval{{start: 1.02, end: 1.022}}}
	seat := mintRootCauseDIOStateSeat(Query{}, WindowStats{}, false, true,
		dTd.Thread, false,
		[]dioStateFamilyMember{
			dioStateMemberFromTd(string(StateDSleep), dTd, ""),
			dioStateMemberFromTd(string(StateIOWait), ioTd, ""),
		}, "", false)
	if len(seat.dioSegmentIntervals) != 3 || len(seat.dioSegmentIntervalsD) != 2 || len(seat.dioSegmentIntervalsIO) != 1 {
		t.Fatalf("per-state carriers must partition the union: %d/%d/%d",
			len(seat.dioSegmentIntervals), len(seat.dioSegmentIntervalsD), len(seat.dioSegmentIntervalsIO))
	}
	dLen, _ := foldIntervalsLengthMs(seat.dioSegmentIntervalsD)
	ioLen, _ := foldIntervalsLengthMs(seat.dioSegmentIntervalsIO)
	if math.Abs(dLen-seat.DStateMs) > rspaAnchorIdentityTolMs || math.Abs(ioLen-seat.IOWaitMs) > rspaAnchorIdentityTolMs {
		t.Fatalf("per-state Σ must reproduce the state channels: d=%.3f/%.3f io=%.3f/%.3f",
			dLen, seat.DStateMs, ioLen, seat.IOWaitMs)
	}
	// Fail-open together with the union (overflowed ledger).
	overflowTd := dTd
	overflowTd.dioIntervalsOverflow = true
	seat = mintRootCauseDIOStateSeat(Query{}, WindowStats{}, false, true,
		dTd.Thread, false,
		[]dioStateFamilyMember{dioStateMemberFromTd(string(StateDSleep), overflowTd, "")}, "", false)
	if len(seat.dioSegmentIntervals) != 0 || len(seat.dioSegmentIntervalsD) != 0 || len(seat.dioSegmentIntervalsIO) != 0 {
		t.Fatalf("the carriers must be absent together on an overflowed ledger")
	}
}

// TestBareCensusEdgeStateSeatKeepArms — the enrich keep arms hold the
// converted rows' lanes on a hypothetical re-enrich (lane-decided-once).
func TestBareCensusEdgeStateSeatKeepArms(t *testing.T) {
	chain := r3SyntheticChain()
	items := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{o3cRunnableSeat()})
	seat, rem := o3cFindSeats(items, "runnable_wait")
	ctx := rootCauseChainContextForItem(*seat, chainCandidateContext{relevance: "background"}, chain.Target)
	if ctx.relevance != "on_chain" || ctx.overlapMs != 0 {
		t.Fatalf("the ⛓ state seat must keep its lane on re-enrich: %+v", ctx)
	}
	ctx = rootCauseChainContextForItem(*rem, chainCandidateContext{relevance: "background"}, chain.Target)
	if ctx.relevance != "adjacent" || ctx.overlapMs != 0 {
		t.Fatalf("the ◇ state remainder must keep its lane on re-enrich: %+v", ctx)
	}
}

// TestBareCensusEdgeStateSeatHalfNeverReconsOnFullAccountHull — the dangerous
// cross-type recon shape the hull-clearing exists for (偏离备案① made load-
// bearing): a diagnostic projection whose interval identity is the PRE-SPLIT
// full-account hull, with EVERY other precise recon dimension (numeric TID /
// typed window / lane+basis / line span) aligned with the ⛓ half seat. Were
// the half seat still wearing the group hull (mutation M10), the B4 interval-
// twin arm would certify "one physical account" between the 3.5ms half seat
// and this 5.0ms full-account row — a value-halved seat absorbing a full-
// account observation. The cleared hull makes the interval dimension fail
// open, so BOTH rows must stay published.
func TestBareCensusEdgeStateSeatHalfNeverReconsOnFullAccountHull(t *testing.T) {
	chain := r3SyntheticChain()
	item := o3cRunnableSeat()
	item.StatsWindowStartTs, item.StatsWindowEndTs = 6.0, 6.01
	items := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{item})
	seat, rem := o3cFindSeats(items, "runnable_wait")
	if seat == nil || rem == nil {
		t.Fatalf("bisection must publish both halves: %+v", items)
	}
	twin := RootCauseRankItem{
		Type: "scheduler_latency", Source: "scheduler_latency_stats",
		Thread:         ThreadRef{Comm: "hostw", PID: 300},
		Confidence:     0.7,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
		OnChainBasis:       RootCauseOnChainBasisHostWakeupEdgeState,
		StatsWindowStartTs: 6.0, StatsWindowEndTs: 6.01,
		StartTs: 6.001, EndTs: 6.0085,
		ImpactMs: 5.0, CumulativeImpactMs: 5.0, EffectiveImpactMs: 5.0,
		RunnableMs: 5.0, LineStart: 10, LineEnd: 20,
		familyMemberIntervals: o3cSeatFullAccountHull(),
	}
	rank := RootCauseRankResult{Items: []RootCauseRankItem{*seat, *rem, twin}}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.AbsorbedItems) != 0 || len(rank.Items) != 3 {
		t.Fatalf("a half seat must never absorb a full-account interval twin by hull (值已减半而 hull 描述全账): absorbed=%d active=%d",
			len(rank.AbsorbedItems), len(rank.Items))
	}
}
