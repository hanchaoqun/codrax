package tracequery

// rank_p3_measure_test.go — P3MEASURE-1 unit pins (§29.169, 2026-07-20):
// the ruled counterfactual caliber on engine-real synthetic shapes.
//
//	pin① periodic consistency (校准 pin ②, 两机器同判): the berlin VS-1
//	     vsync shape — the SAME seat the periodic discount machine already
//	     discounts (eff = runnable+lateness) measures counterfactually
//	     INVALID in full (its closing edges are cadence-pinned), and the
//	     cadence-broken twin measures VALID in full (ruled-legal forms are
//	     never convicted by data — absence of a typed hit ⇒ valid).
//	pin② µs identity + per-window family-① precision: a mixed board where
//	     ONE of two anchor windows closes on a periodic edge — only that
//	     window's portion turns invalid (a pid-coarse judge mutation reds).
//	pin③ segment edge-witness tolerance: the audit-同源 closure slack
//	     (rspaIOCompletionClosureTolS) is load-bearing on both sides.
//	pin④ disposition closed set: self_ruled carries NO numbers (禁重诉既裁),
//	     honest-absence forms carry no numbers, the capped aggregate
//	     inventory refuses re-aggregation, the discounted-caliber seat
//	     withholds the structural dimension (edge_witnessed ≤ 席值 stays a
//	     wire invariant).
//	pin⑤ restamp idempotency (the A/B write-surface primitive).
//
// Mutation duties (cp-copy recovery only):
//	M-1 invert the family-① judge (invalid on !periodic) → pin① both arms red;
//	M-2 fold the per-window judge to pid level → pin② red (both windows turn);
//	M-3 drop the closure tolerance from the witness join → pin③ red;
//	M-4 publish numbers on the self lane → pin④ red;
//	M-5 let the discounted seat publish witnessed > 席值 → pin④ red.

import (
	"encoding/json"
	"math"
	"testing"
)

// p3mMicroTrace — a minimal REAL indexed inventory for the context builder:
// worker-200 emits two raw sched_wakeup rows toward app-100 (10.010 /
// 10.030), plus one third-party wakeup that must NOT enter the closure
// inventory. The chain windows are hand-built (engine-actual paired
// node+impact form, VS-1 fixture precedent).
const p3mMicroTrace = `
     worker-200 (200) [001] .... 10.010000: sched_wakeup: comm=app pid=100 prio=100 target_cpu=001
     worker-200 (200) [001] .... 10.020500: sched_wakeup: comm=other pid=900 prio=100 target_cpu=002
     worker-200 (200) [001] .... 10.030000: sched_wakeup: comm=app pid=100 prio=100 target_cpu=001
`

// p3mMicroChain — app-100 (target) waits twice on worker-200: window A
// [10.000,10.010] closes on the 10.010 edge, window B [10.020,10.030] on the
// 10.030 edge. periodicB flags the SECOND impact as a VS-1 periodic member.
func p3mMicroChain(periodicB bool) ChainResult {
	chain := ChainResult{Target: ThreadRef{PID: 100, Comm: "app"}}
	windows := []TimeWindow{
		{StartTs: 10.000, EndTs: 10.010},
		{StartTs: 10.020, EndTs: 10.030},
	}
	for i, w := range windows {
		chain.Nodes = append(chain.Nodes, ChainNode{
			ID: "n", Thread: ThreadRef{PID: 200, Comm: "worker"},
			Window: w, Dominant: StateSSleep, DurationMs: (w.EndTs - w.StartTs) * 1000, Depth: 1,
		})
		impact := WakeupCausalImpact{
			Thread: ThreadRef{PID: 200, Comm: "worker"}, ChainDepth: 1,
			DominantState: string(StateSSleep), Window: w, TotalMs: (w.EndTs - w.StartTs) * 1000,
		}
		if i == 1 && periodicB {
			impact.PeriodicSource = true
		}
		chain.CausalImpacts = append(chain.CausalImpacts, impact)
	}
	return chain
}

func p3mMicroCtx(t *testing.T, chain ChainResult) *p3MeasureContext {
	t.Helper()
	idx := buildTraceIndex(t, "p3m_micro.systrace", p3mMicroTrace)
	ctx := buildP3MeasureContext(idx, nil, &chain)
	if ctx == nil {
		t.Fatalf("context must build from a live chain universe")
	}
	return ctx
}

func p3mUs(ms float64) int64 { return int64(math.Round(ms * 1000)) }

// TestP3MeasurePeriodicDiscountConsistency — pin① (校准 pin ②): the berlin
// VS-1 vsync aggregate — where the periodic discount machine is ON DUTY
// (eff collapses 36.256 → 0.105) — measures counterfactually invalid in
// FULL; the cadence-broken twin (irregular intervals, detection negative)
// measures valid in full. 两机器同判 on the same typed classification.
func TestP3MeasurePeriodicDiscountConsistency(t *testing.T) {
	measure := func(intervalsMs []float64) RootCauseRankItem {
		chain := buildPeriodicVSyncChain(intervalsMs, berlinVSyncSleepMs, berlinVSyncRunnableMs)
		chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
		if len(chain.AggregatedImpacts) != 1 {
			t.Fatalf("fixture: one aggregate expected, got %d", len(chain.AggregatedImpacts))
		}
		item := rootCauseItemFromCausalAggregate(chain.AggregatedImpacts[0])
		item.Rank = 1
		rank := RootCauseRankResult{Target: chain.Target, Items: []RootCauseRankItem{item}}
		rank.p3MeasureCtx = buildP3MeasureContext(buildTraceIndex(t, "p3m_vsync.systrace", p3mMicroTrace), nil, &chain)
		stampP3CounterfactualMeasure(&rank)
		return rank.Items[0]
	}

	// ── periodic arm: the discount machine is on duty ⇒ invalid in full ──
	seat := measure(berlinVSyncIntervalsMs)
	if !seat.PeriodicSource || !near(rootCauseEffectiveImpactMs(seat), 0.105, 0.001) {
		t.Fatalf("fixture drift: the berlin seat must ride the periodic discount (eff 0.105): %+v", seat)
	}
	if seat.P3MDisposition != p3mDispositionEdgeTerminatedWindow {
		t.Fatalf("chain aggregate seat must measure on the window form, got %q", seat.P3MDisposition)
	}
	baseUs := p3mUs(seat.P3MCounterfactualValidMs) + p3mUs(seat.P3MCounterfactualInvalidMs)
	if baseUs <= 0 || p3mUs(seat.P3MCounterfactualValidMs) != 0 {
		t.Fatalf("两机器同判: a periodic-discounted seat's anchor time is counterfactually invalid in FULL, got valid=%.3f invalid=%.3f",
			seat.P3MCounterfactualValidMs, seat.P3MCounterfactualInvalidMs)
	}
	// The six 6ms occurrence windows: base == Σ window time to the µs.
	wantUs := int64(0)
	for i := range berlinVSyncSleepMs {
		wantUs += p3mUs(berlinVSyncSleepMs[i] + berlinVSyncRunnableMs[i])
	}
	if baseUs != wantUs {
		t.Fatalf("µs identity: valid+invalid=%dµs want the merged occurrence time %dµs", baseUs, wantUs)
	}
	// edge_witnessed ≤ 席值 (the discounted eff IS the cap on the window form).
	if got, want := p3mUs(seat.P3MEdgeWitnessedMs), p3mUs(0.105); got != want {
		t.Fatalf("window-form witness must cap at the published value: got %dµs want %dµs", got, want)
	}

	// ── cadence-broken twin: no typed hit ⇒ valid in full (禁重诉既裁) ──
	broken := measure([]float64{8.302, 20.000, 12.760, 7.100, 9.444})
	if broken.PeriodicSource {
		t.Fatalf("fixture: the irregular twin must not detect as periodic")
	}
	if p3mUs(broken.P3MCounterfactualInvalidMs) != 0 ||
		p3mUs(broken.P3MCounterfactualValidMs) != wantUs {
		t.Fatalf("absence of a typed counterexample must read VALID in full: valid=%.3f invalid=%.3f",
			broken.P3MCounterfactualValidMs, broken.P3MCounterfactualInvalidMs)
	}
}

// TestP3MeasurePerWindowPeriodicPrecision — pin②: only the anchor window
// whose CLOSING edge carries the typed periodic flag turns invalid; the
// sibling window of the SAME pid stays valid (a pid-coarse judge reds here).
// Also the µs identity and the segment edge-witness join on the same seat.
func TestP3MeasurePerWindowPeriodicPrecision(t *testing.T) {
	chain := p3mMicroChain(true) // window B (10.020..10.030) closes periodic
	ctx := p3mMicroCtx(t, chain)
	item := RootCauseRankItem{
		Type: "runnable_wait", Thread: ThreadRef{PID: 200, Comm: "worker"},
		ChainRelevance: "on_chain", Rank: 1, RunnableMs: 14.0,
		// Four typed segments: 4ms in window A (witnessed by the in-window
		// 10.010 edge? — no: the edge sits at the WINDOW end; segment A1
		// [10.000,10.004] ends 6ms before it → NOT witnessed; A2
		// [10.0096,10.010] ends at the edge → witnessed), 6ms in window B.
		runnableIntervals: []foldInterval{
			{start: 10.000, end: 10.004},   // A1: 4ms, no edge within +0.5ms
			{start: 10.0096, end: 10.0100}, // A2: 0.4ms, edge at 10.010 → witnessed
			{start: 10.020, end: 10.026},   // B1: 6ms, no edge within +0.5ms
			{start: 10.0296, end: 10.0300}, // B2: 0.4ms, edge at 10.030 → witnessed
		},
	}
	rank := RootCauseRankResult{Target: chain.Target, Items: []RootCauseRankItem{item}}
	rank.p3MeasureCtx = ctx
	stampP3CounterfactualMeasure(&rank)
	seat := rank.Items[0]
	if seat.P3MDisposition != p3mDispositionSegmentJoin {
		t.Fatalf("typed-inventory state seat must measure on the segment join, got %q", seat.P3MDisposition)
	}
	// Base = 4 + 0.4 + 6 + 0.4 = 10.8ms; invalid = window B's share 6.4ms.
	validUs, invalidUs := p3mUs(seat.P3MCounterfactualValidMs), p3mUs(seat.P3MCounterfactualInvalidMs)
	if validUs+invalidUs != p3mUs(10.8) {
		t.Fatalf("µs identity: %d+%d != %d", validUs, invalidUs, p3mUs(10.8))
	}
	if invalidUs != p3mUs(6.4) || validUs != p3mUs(4.4) {
		t.Fatalf("per-window family-① precision: want invalid=6.400 (window B only) valid=4.400, got valid=%.3f invalid=%.3f",
			seat.P3MCounterfactualValidMs, seat.P3MCounterfactualInvalidMs)
	}
	// Witness join: exactly the two edge-closed micro segments (0.8ms).
	if got := p3mUs(seat.P3MEdgeWitnessedMs); got != p3mUs(0.8) {
		t.Fatalf("segment witness join: want 0.800ms (the two edge-closed segments), got %.3f", seat.P3MEdgeWitnessedMs)
	}
}

// TestP3MeasureWitnessClosureTolerance — pin③: the +0.5ms closure slack
// (rspaIOCompletionClosureTolS, audit-同源) is load-bearing on both sides of
// the boundary: a segment ending 0.4ms before its pid's edge IS witnessed, a
// segment ending 0.6ms before it is NOT.
func TestP3MeasureWitnessClosureTolerance(t *testing.T) {
	chain := p3mMicroChain(false)
	ctx := p3mMicroCtx(t, chain)
	item := RootCauseRankItem{
		Type: "runnable_wait", Thread: ThreadRef{PID: 200, Comm: "worker"},
		ChainRelevance: "on_chain", Rank: 1, RunnableMs: 14.0,
		runnableIntervals: []foldInterval{
			{start: 10.000, end: 10.0096}, // ends 0.4ms before the 10.010 edge → witnessed
			{start: 10.020, end: 10.0294}, // ends 0.6ms before the 10.030 edge → not witnessed
		},
	}
	rank := RootCauseRankResult{Target: chain.Target, Items: []RootCauseRankItem{item}}
	rank.p3MeasureCtx = ctx
	stampP3CounterfactualMeasure(&rank)
	seat := rank.Items[0]
	if got := p3mUs(seat.P3MEdgeWitnessedMs); got != p3mUs(9.6) {
		t.Fatalf("closure tolerance ±: want exactly the 9.6ms tolerant segment witnessed, got %.3f", seat.P3MEdgeWitnessedMs)
	}
}

// TestP3MeasureDispositionClosedSet — pin④: every honest-absence lane plus
// the self red line (恒链上 is a RULING — a self seat records the typed
// disposition and NO numbers) and the discounted-caliber withhold.
func TestP3MeasureDispositionClosedSet(t *testing.T) {
	chain := p3mMicroChain(false)
	ctx := p3mMicroCtx(t, chain)
	stamp := func(item RootCauseRankItem) RootCauseRankItem {
		rank := RootCauseRankResult{Target: chain.Target, Items: []RootCauseRankItem{item}}
		rank.p3MeasureCtx = ctx
		stampP3CounterfactualMeasure(&rank)
		return rank.Items[0]
	}
	noNumbers := func(name string, seat RootCauseRankItem) {
		t.Helper()
		if seat.P3MCounterfactualValidMs != 0 || seat.P3MCounterfactualInvalidMs != 0 || seat.P3MEdgeWitnessedMs != 0 {
			t.Fatalf("%s must carry NO numbers: %+v", name, seat)
		}
	}

	// self by pid (RootEvidence legacy self arm).
	seat := stamp(RootCauseRankItem{Type: "runnable_wait", Thread: ThreadRef{PID: 100}, ChainRelevance: "on_chain",
		runnableIntervals: []foldInterval{{start: 10.001, end: 10.002}}})
	if seat.P3MDisposition != p3mDispositionSelfRuled {
		t.Fatalf("target pid seat must be self_ruled, got %q", seat.P3MDisposition)
	}
	noNumbers("self_ruled (pid)", seat)

	// self by basis.
	seat = stamp(RootCauseRankItem{Type: "running", Thread: ThreadRef{PID: 100}, ChainRelevance: "on_chain",
		OnChainBasis: RootCauseOnChainBasisSelfWallClockInterval})
	if seat.P3MDisposition != p3mDispositionSelfRuled {
		t.Fatalf("self basis seat must be self_ruled, got %q", seat.P3MDisposition)
	}
	noNumbers("self_ruled (basis)", seat)

	// envelope-only row: no typed inventory.
	seat = stamp(RootCauseRankItem{Type: "io_latency", Thread: ThreadRef{PID: 200}, ChainRelevance: "on_chain",
		StartTs: 10.000, EndTs: 10.009, ImpactMs: 4.0}) // envelope 9ms ≠ 4ms → no µs-identity arm
	if seat.P3MDisposition != p3mDispositionNoTypedInventory {
		t.Fatalf("envelope-only row must record no_typed_inventory, got %q", seat.P3MDisposition)
	}
	noNumbers("no_typed_inventory", seat)

	// typed inventory but the pid holds no anchor window.
	seat = stamp(RootCauseRankItem{Type: "runnable_wait", Thread: ThreadRef{PID: 900}, ChainRelevance: "on_chain",
		runnableIntervals: []foldInterval{{start: 10.001, end: 10.002}}})
	if seat.P3MDisposition != p3mDispositionNoAnchorWindows {
		t.Fatalf("anchor-less pid must record no_anchor_windows, got %q", seat.P3MDisposition)
	}
	noNumbers("no_anchor_windows", seat)

	// aggregate at the occurrence-window wire cap: 禁截断库存二次聚合.
	capped := RootCauseRankItem{Type: "sleep_wait", Thread: ThreadRef{PID: 200}, ChainRelevance: "on_chain",
		Source: "wakeup_chain.aggregated_impacts"}
	for i := 0; i < wakeupCausalAggregateOccurrenceCap; i++ {
		capped.OccurrenceWindows = append(capped.OccurrenceWindows,
			WakeupCausalOccurrence{Window: TimeWindow{StartTs: 10.0 + float64(i)*0.01, EndTs: 10.005 + float64(i)*0.01}})
	}
	seat = stamp(capped)
	if seat.P3MDisposition != p3mDispositionOccurrenceCapped {
		t.Fatalf("cap-full occurrence inventory must refuse re-aggregation, got %q", seat.P3MDisposition)
	}
	noNumbers("occurrence_inventory_capped", seat)

	// discounted caliber: published value below the seat's own witnessed
	// segment measure → the structural dimension is withheld (pin M-5 target).
	seat = stamp(RootCauseRankItem{Type: "runnable_wait", Thread: ThreadRef{PID: 200}, ChainRelevance: "on_chain",
		RunnableMs:        0.001, // 1µs published (runnable caliber), segments measure 9.6ms witnessed
		runnableIntervals: []foldInterval{{start: 10.000, end: 10.0096}}})
	if seat.P3MDisposition != p3mDispositionCounterfactualOnly {
		t.Fatalf("discounted-caliber seat must withhold the structural dimension, got %q", seat.P3MDisposition)
	}
	if seat.P3MEdgeWitnessedMs != 0 {
		t.Fatalf("edge_witnessed ≤ 席值 is a wire invariant — the withheld dimension publishes 0, got %.3f", seat.P3MEdgeWitnessedMs)
	}
	if p3mUs(seat.P3MCounterfactualValidMs)+p3mUs(seat.P3MCounterfactualInvalidMs) != p3mUs(9.6) {
		t.Fatalf("the counterfactual dimension stays measured on the withheld form: %+v", seat)
	}

	// non-on-chain and unknown-basis rows: not in the population, all zero.
	seat = stamp(RootCauseRankItem{Type: "runnable_wait", Thread: ThreadRef{PID: 200}, ChainRelevance: "adjacent",
		runnableIntervals: []foldInterval{{start: 10.001, end: 10.002}}})
	if seat.P3MDisposition != "" {
		t.Fatalf("a non-on-chain row is outside the population, got %q", seat.P3MDisposition)
	}
	seat = stamp(RootCauseRankItem{Type: "runnable_wait", Thread: ThreadRef{PID: 200}, ChainRelevance: "on_chain",
		OnChainBasis: "some_future_basis"})
	if seat.P3MDisposition != "" {
		t.Fatalf("an unknown basis never enters the measurement, got %q", seat.P3MDisposition)
	}
}

// TestP3MeasureRestampIdempotent — pin⑤: clearing the four fields and
// re-stamping reproduces them exactly; a second stamp over a stamped board
// is a byte no-op (the flagship A/B write-surface primitive).
func TestP3MeasureRestampIdempotent(t *testing.T) {
	chain := p3mMicroChain(true)
	ctx := p3mMicroCtx(t, chain)
	rank := RootCauseRankResult{Target: chain.Target, Items: []RootCauseRankItem{{
		Type: "runnable_wait", Thread: ThreadRef{PID: 200, Comm: "worker"},
		ChainRelevance: "on_chain", Rank: 1, RunnableMs: 14.0,
		runnableIntervals: []foldInterval{{start: 10.000, end: 10.004}, {start: 10.020, end: 10.026}},
	}}}
	rank.p3MeasureCtx = ctx
	stampP3CounterfactualMeasure(&rank)
	firstJSON, err := json.Marshal(rank.Items[0])
	if err != nil {
		t.Fatal(err)
	}
	stampP3CounterfactualMeasure(&rank)
	secondJSON, _ := json.Marshal(rank.Items[0])
	if string(secondJSON) != string(firstJSON) {
		t.Fatalf("restamp must be a byte no-op:\n first=%s\nsecond=%s", firstJSON, secondJSON)
	}
	p3mClearSeat(&rank.Items[0])
	stampP3CounterfactualMeasure(&rank)
	thirdJSON, _ := json.Marshal(rank.Items[0])
	if string(thirdJSON) != string(firstJSON) {
		t.Fatalf("clear+restamp must reproduce the measurement exactly:\n first=%s\n third=%s", firstJSON, thirdJSON)
	}
}
