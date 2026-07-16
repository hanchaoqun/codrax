package tracequery

// rank_chain_anchor_xlane_test.go — XLANE-1 件1/件2 pins (§29.104.1/§29.104.2,
// 2026-07-15; customer witness /Users/han/opt/customlogs/runnable2.txt).
//
// Disease (witness arithmetic): the shadowhook-task-64305 chain-lane runnable
// family published E11 (调度延迟 satellite, 23.471 FULL) beside E26/E27/E28/E32
// (chain seats, 17.635+3.608+8.608+0.195) — Σ 53.5ms of chain-tier runnable
// eff against a 26.725ms full-window physical account (2.0×). The satellite's
// fully-anchored path kept the whole diagnostic projection on the chain tier
// (`continue`), so one physical runnable held two-plus full seats.
//
// Fix layers pinned here:
//   件1 — a fully-anchored satellite whose same-pid chain-lane runnable seat
//        is in the pool AND physically intersects its intervals demotes WHOLE
//        to ◇ (typed ChainAnchorRepresentedByChainSeat, values untouched);
//        a pid with no chain seat (or no provable intersection) keeps the
//        chain lane byte-identically (禁把锚定份丢出链, negative pins).
//   件2 — the B4 recon table gains the causal_scheduler_latency pair: an
//        EXACT interval-twin satellite (witness E11↔E15 逐值同形) is absorbed
//        into the chain seat (single seat + E# merge); the 件1 lane move is
//        bridged for exactly that candidate shape and nothing else.

import (
	"math"
	"strings"
	"testing"
)

// xlaneUnitFixture builds the shared 件1 unit shape: chain thread 200 with a
// typed jump window [1.000, 1.030], census full 8ms / anchored 5ms, one
// chain-lane runnable seat over [1.010, 1.015] and one fully-anchored
// scheduler_latency satellite over the same segment.
func xlaneUnitFixture() (ChainResult, WindowStats, []RootCauseRankItem) {
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
	items := []RootCauseRankItem{
		// The chain-lane runnable seat (the anchored share's formal
		// representative on the chain tier).
		{Type: "runnable_wait", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 5.0, ImpactMs: 5.0, CumulativeImpactMs: 5.0, EffectiveImpactMs: 5.0,
			StartTs: 1.010, EndTs: 1.015, LineStart: 100, LineEnd: 120,
			Source: "wakeup_chain.causal_impacts", ChainDepth: 1},
		// Fully-anchored satellite over the SAME physical segment.
		{Type: "scheduler_latency", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 5.0, ImpactMs: 5.0, CumulativeImpactMs: 5.0, EffectiveImpactMs: 5.0,
			StartTs: 1.010, EndTs: 1.015, LineStart: 100, LineEnd: 120,
			Source: "scheduler_latency_stats"},
	}
	return chain, stats, items
}

// 件1 positive pin: chain seat present ∧ physical intersection → the whole
// satellite rides ◇ with every published value untouched and the honest
// represented-by-chain-seat sentence (never the R4 无凭证 word family).
func TestXLANEFullyAnchoredSatelliteDemotesWhenChainSeatRepresents(t *testing.T) {
	chain, stats, items := xlaneUnitFixture()
	// low_frequency rides the same arm (同族).
	items = append(items, RootCauseRankItem{
		Type: "low_frequency", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
		DominantState: string(StateRunnable), RunnableMs: 5.0, ImpactMs: 5.0, CumulativeImpactMs: 5.0, EffectiveImpactMs: 5.0,
		StartTs: 1.010, EndTs: 1.015, LineStart: 100, LineEnd: 120,
		Source: "scheduler_latency_stats"})
	items = reanchorOnChainStateSeats(chain, stats, items)
	if !strings.HasPrefix(items[0].Source, "wakeup_chain") || items[0].ChainRelevance != "on_chain" ||
		items[0].RunnableMs != 5.0 || items[0].ChainAnchorRepresentedByChainSeat {
		t.Fatalf("the chain seat itself must stay untouched: %+v", items[0])
	}
	for _, i := range []int{1, 2} {
		sat := items[i]
		if !sat.ChainAnchorRepresentedByChainSeat || sat.ChainRelevance != "adjacent" ||
			sat.Causality != "adjacent_to_wakeup_chain" {
			t.Fatalf("件1: the fully-anchored satellite must demote whole to ◇ with the typed marker: %+v", sat)
		}
		if sat.RunnableMs != 5.0 || sat.ImpactMs != 5.0 || sat.CumulativeImpactMs != 5.0 || sat.EffectiveImpactMs != 5.0 {
			t.Fatalf("件1: the demotion must never touch published values: %+v", sat)
		}
		if sat.ChainAnchorFullMs != 0 || sat.ChainAnchorRemainderSeat || sat.ChainCredentialLaneDemoted {
			t.Fatalf("件1: the represented demotion is its own typed form — no bipartition fields, no R4 marker: %+v", sat)
		}
		if !strings.Contains(sat.Summary, "anchored share represented by the chain seat") {
			t.Fatalf("件1: the honest sentence must ride the summary: %q", sat.Summary)
		}
		if strings.Contains(sat.Summary, "no chain credential") {
			t.Fatalf("件1: the 无凭证 word family is forbidden on a credential-anchored satellite: %q", sat.Summary)
		}
	}
	// Idempotency: the second (enrich) pass never re-processes or doubles the
	// sentence.
	before1, before2 := items[1].Summary, items[2].Summary
	items = reanchorOnChainStateSeats(chain, stats, items)
	if items[1].Summary != before1 || items[2].Summary != before2 {
		t.Fatalf("件1: the demotion must be idempotent across the build+enrich passes")
	}
}

// 件1 negative pins (禁把锚定份丢出链): a satellite that is the anchored
// share's ONLY representative keeps the chain lane byte-identically — chain
// seat absent, chain seat of another state family, and chain seat physically
// disjoint from the satellite's own segment.
func TestXLANESoleRepresentativeSatelliteKeepsChainLane(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(items []RootCauseRankItem) []RootCauseRankItem
	}{
		{"chain seat absent", func(items []RootCauseRankItem) []RootCauseRankItem {
			return items[1:] // drop the chain seat, keep the satellite
		}},
		{"chain seat non-runnable family", func(items []RootCauseRankItem) []RootCauseRankItem {
			items[0].DominantState = string(StateDSleep)
			items[0].Type = "d_state_or_io_wait"
			items[0].DStateMs, items[0].RunnableMs = 5.0, 0
			return items
		}},
		{"chain seat physically disjoint", func(items []RootCauseRankItem) []RootCauseRankItem {
			items[0].StartTs, items[0].EndTs = 1.020, 1.025
			return items
		}},
		{"chain seat interval-less", func(items []RootCauseRankItem) []RootCauseRankItem {
			items[0].StartTs, items[0].EndTs = 0, 0
			return items
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chain, stats, items := xlaneUnitFixture()
			items = tc.mutate(items)
			satIdx := len(items) - 1
			want := items[satIdx]
			items = reanchorOnChainStateSeats(chain, stats, items)
			sat := items[satIdx]
			if sat.ChainAnchorRepresentedByChainSeat || sat.ChainRelevance != "on_chain" ||
				sat.Causality != want.Causality || sat.RunnableMs != want.RunnableMs ||
				sat.Summary != want.Summary {
				t.Fatalf("负向 pin (%s): the sole-representative satellite must keep the chain lane byte-identically: %+v", tc.name, sat)
			}
		})
	}
}

// 修复轮 P1-1 常驻负向 pin (对抗复核缺口形探针转正, 2026-07-16): an AGGREGATED
// chain seat's StartTs..EndTs is the FirstTs..LastTs ENVELOPE across its
// occurrence gaps — a satellite segment lying entirely inside such a gap has
// NO chain-seat coverage and must keep the chain lane (sole-representative
// protection; the typed OccurrenceWindows inventory is the only admissible
// segment source, hull/envelope timestamps never feed the hard gate).
func TestXLANEOccurrenceGapSatelliteKeepsChainLane(t *testing.T) {
	chain, stats, items := xlaneUnitFixture()
	// Aggregated chain seat: envelope [1.008, 1.022] with a typed occurrence
	// gap 1.012..1.020; the satellite [1.014, 1.016] sits fully in the gap.
	items[0].Source = "wakeup_chain.aggregated_impacts"
	items[0].StartTs, items[0].EndTs = 1.008, 1.022
	items[0].OccurrenceWindows = []WakeupCausalOccurrence{
		{Window: TimeWindow{StartTs: 1.008, EndTs: 1.012}},
		{Window: TimeWindow{StartTs: 1.020, EndTs: 1.022}},
	}
	items[1].StartTs, items[1].EndTs = 1.014, 1.016
	items[1].RunnableMs, items[1].ImpactMs, items[1].CumulativeImpactMs, items[1].EffectiveImpactMs = 2.0, 2.0, 2.0, 2.0
	want := items[1]
	items = reanchorOnChainStateSeats(chain, stats, items)
	sat := items[1]
	if sat.ChainAnchorRepresentedByChainSeat || sat.ChainRelevance != "on_chain" || sat.Summary != want.Summary {
		t.Fatalf("P1-1: a gap-resident satellite has no chain-seat coverage and must keep the chain lane: %+v", sat)
	}

	// Envelope-only multi-member seat (no member inventory, no occurrence
	// windows — the MAX-fallback fold shape) contributes NOTHING: the
	// satellite keeps the chain lane even when the envelope would cover it.
	chain, stats, items = xlaneUnitFixture()
	items[0].MemberCount = 3
	items[0].StartTs, items[0].EndTs = 1.008, 1.022
	want = items[1]
	items = reanchorOnChainStateSeats(chain, stats, items)
	if items[1].ChainAnchorRepresentedByChainSeat || items[1].ChainRelevance != "on_chain" || items[1].Summary != want.Summary {
		t.Fatalf("P1-1: an inventory-less multi-member seat must contribute no segments: %+v", items[1])
	}
}

// 修复轮 P1-2 负向 pin (对抗复核, 2026-07-16): the demotion gate is a COVERAGE
// proof, not bare intersection — a 1ms chain seat must never swallow a 5ms
// satellite whole (the uncovered 4ms has no other chain representative).
func TestXLANEPartialCoverageSatelliteKeepsChainLane(t *testing.T) {
	chain, stats, items := xlaneUnitFixture()
	items[0].StartTs, items[0].EndTs = 1.010, 1.011 // 1ms of the satellite's 5ms
	want := items[1]
	items = reanchorOnChainStateSeats(chain, stats, items)
	sat := items[1]
	if sat.ChainAnchorRepresentedByChainSeat || sat.ChainRelevance != "on_chain" || sat.Summary != want.Summary {
		t.Fatalf("P1-2: partial coverage must keep the satellite on the chain lane byte-identically: %+v", sat)
	}
}

// 修复轮 P1-2 不变式 pin: every demoted satellite satisfies
// Σ overlap(chain-seat segments, satellite intervals) ≈ its published value —
// the 「已由链上席全额代表」 sentence's arithmetic backing.
func TestXLANEDemotedSatelliteCoverageInvariant(t *testing.T) {
	chain, stats, items := xlaneUnitFixture()
	items = reanchorOnChainStateSeats(chain, stats, items)
	sat := items[1]
	if !sat.ChainAnchorRepresentedByChainSeat {
		t.Fatalf("fixture drifted: the covered satellite must demote: %+v", sat)
	}
	seatWindows := rspaChainRunnableSeatWindowsByPID(items)[sat.Thread.PID]
	covered := rspaIntervalsOverlapMs(seatWindows, []foldInterval{{start: sat.StartTs, end: sat.EndTs}})
	if !rspaWithinTol(covered, sat.RunnableMs) {
		t.Fatalf("P1-2 invariant: demoted satellite coverage %.6f must equal its account %.6f", covered, sat.RunnableMs)
	}
}

// 件2 positive pin: the chain-lane seat absorbs an EXACT interval-twin
// satellite (single seat + lossless carrier), including after the 件1 lane
// demotion (the marker bridges exactly that lane mismatch) and for the
// inversion-typed chain seat (witness E28 form).
func TestXLANEChainSeatAbsorbsIntervalTwinSatellite(t *testing.T) {
	for _, tc := range []struct {
		name         string
		absorberType string
		source       string
		demoted      bool
	}{
		{"causal_impacts runnable, demoted candidate", "runnable_wait", "wakeup_chain.causal_impacts", true},
		{"aggregated_impacts runnable, on-chain candidate", "runnable_wait", "wakeup_chain.aggregated_impacts", false},
		{"inversion chain seat, demoted candidate", "priority_inversion_candidate", "wakeup_chain.causal_impacts", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			window := TimeWindow{StartTs: 1.0, EndTs: 1.04}
			thread := ThreadRef{Comm: "shadowhook-task", PID: 200, TGID: 20}
			absorber := RootCauseRankItem{
				Type: tc.absorberType, Thread: thread,
				StartTs: 1.010, EndTs: 1.015, LineStart: 100, LineEnd: 120,
				Source: tc.source, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
				DominantState: string(StateRunnable), RunnableMs: 5.0,
				ImpactMs: 5.0, CumulativeImpactMs: 5.0, EffectiveImpactMs: 5.0,
			}
			candidate := RootCauseRankItem{
				Type: "scheduler_latency", Thread: thread,
				StartTs: 1.010, EndTs: 1.015, LineStart: 100, LineEnd: 120,
				StatsWindowStartTs: window.StartTs, StatsWindowEndTs: window.EndTs,
				Source: "scheduler_latency_stats",
				DominantState: string(StateRunnable), RunnableMs: 5.0,
				ImpactMs: 5.0, CumulativeImpactMs: 5.0, EffectiveImpactMs: 5.0,
			}
			if tc.demoted {
				candidate.ChainAnchorRepresentedByChainSeat = true
				candidate.Causality = "adjacent_to_wakeup_chain"
				candidate.ChainRelevance = "adjacent"
			} else {
				candidate.Causality = "on_wakeup_chain"
				candidate.ChainRelevance = "on_chain"
			}
			rank := RootCauseRankResult{Window: window, Items: []RootCauseRankItem{absorber, candidate}}
			reconcileExactCrossTypeRankSeats(&rank)
			if len(rank.Items) != 1 || len(rank.AbsorbedItems) != 1 {
				t.Fatalf("件2: one exact physical account must own one seat: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
			}
			owner, supporting := rank.Items[0], rank.AbsorbedItems[0]
			if owner.Type != tc.absorberType || owner.Source != tc.source || owner.AbsorbedRankRows != 1 {
				t.Fatalf("件2: the chain seat must own the active seat: %+v", owner)
			}
			if supporting.Type != "scheduler_latency" || !supporting.AbsorbedByRankFamily ||
				supporting.Tier != RootCauseTierAbsorbed || supporting.AbsorbedIntoFamily != owner.RankFamilyKey {
				t.Fatalf("件2: the satellite must survive on the lossless carrier: %+v", supporting)
			}
		})
	}
}

// 件2 negative pins: the exact arm stays exact — a near-miss keeps two honest
// rows, and the lane bridge never opens without the 件1 marker.
func TestXLANEChainSeatTwinNearMissesFailOpen(t *testing.T) {
	build := func() (TimeWindow, RootCauseRankItem, RootCauseRankItem) {
		window := TimeWindow{StartTs: 1.0, EndTs: 1.04}
		thread := ThreadRef{Comm: "shadowhook-task", PID: 200, TGID: 20}
		absorber := RootCauseRankItem{
			Type: "runnable_wait", Thread: thread,
			StartTs: 1.010, EndTs: 1.015, LineStart: 100, LineEnd: 120,
			Source: "wakeup_chain.causal_impacts", Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 5.0,
			ImpactMs: 5.0, CumulativeImpactMs: 5.0, EffectiveImpactMs: 5.0,
		}
		candidate := RootCauseRankItem{
			Type: "scheduler_latency", Thread: thread,
			StartTs: 1.010, EndTs: 1.015, LineStart: 100, LineEnd: 120,
			StatsWindowStartTs: window.StartTs, StatsWindowEndTs: window.EndTs,
			Source: "scheduler_latency_stats",
			Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent",
			ChainAnchorRepresentedByChainSeat: true,
			DominantState:                     string(StateRunnable), RunnableMs: 5.0,
			ImpactMs: 5.0, CumulativeImpactMs: 5.0, EffectiveImpactMs: 5.0,
		}
		return window, absorber, candidate
	}
	tests := []struct {
		name   string
		mutate func(absorber, candidate *RootCauseRankItem)
	}{
		{"interval differs", func(_, c *RootCauseRankItem) { c.EndTs += 0.000001 }},
		{"line span differs", func(_, c *RootCauseRankItem) { c.LineEnd++ }},
		{"window differs", func(_, c *RootCauseRankItem) { c.StatsWindowEndTs += 0.000001 }},
		{"pid differs", func(_, c *RootCauseRankItem) { c.Thread.PID = 201 }},
		{"lane moved WITHOUT the 件1 marker", func(_, c *RootCauseRankItem) {
			c.ChainAnchorRepresentedByChainSeat = false
		}},
		{"marker rides but absorber is not a chain seat", func(a, _ *RootCauseRankItem) {
			a.Source = "window_stats"
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			window, absorber, candidate := build()
			tc.mutate(&absorber, &candidate)
			rank := RootCauseRankResult{Window: window, Items: []RootCauseRankItem{absorber, candidate}}
			reconcileExactCrossTypeRankSeats(&rank)
			if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 0 {
				t.Fatalf("件2 near miss must fail open to two honest rows: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
			}
		})
	}
}

// Witness-arithmetic end state (runnable2 §29.104.1 三车道同段形): after the
// 件1 demotion and the 件2 recon, the pid's chain tier holds EXACTLY ONE
// full-value runnable representative — the interval-twin satellite is
// absorbed, a non-twin fully-anchored satellite rides ◇, and Σ(on-chain
// runnable eff) can never exceed the census full-window account again.
func TestXLANEWitnessShapeOneFullSeatPerPhysicalTime(t *testing.T) {
	chain, stats, items := xlaneUnitFixture()
	// A second, NON-twin fully-anchored satellite (different segment inside
	// the same jump window; the low_frequency family form).
	items = append(items, RootCauseRankItem{
		Type: "low_frequency", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
		DominantState: string(StateRunnable), RunnableMs: 3.0, ImpactMs: 3.0, CumulativeImpactMs: 3.0, EffectiveImpactMs: 3.0,
		StartTs: 1.012, EndTs: 1.015, LineStart: 111, LineEnd: 115,
		Source: "scheduler_latency_stats"})
	items = reanchorOnChainStateSeats(chain, stats, items)
	rank := RootCauseRankResult{Window: TimeWindow{StartTs: 1.0, EndTs: 1.04}, Items: items}
	for i := range rank.Items {
		if rank.Items[i].Source == "scheduler_latency_stats" {
			rank.Items[i].StatsWindowStartTs, rank.Items[i].StatsWindowEndTs = 1.0, 1.04
		}
	}
	reconcileExactCrossTypeRankSeats(&rank)

	onChainRunnableEff := 0.0
	onChainSeats := 0
	for _, item := range rank.Items {
		if item.Thread.PID != 200 || item.DominantState != string(StateRunnable) {
			continue
		}
		if rootCauseItemIsOnChain(item) {
			onChainSeats++
			onChainRunnableEff += item.EffectiveImpactMs
		}
	}
	if onChainSeats != 1 {
		t.Fatalf("同段物理时间恰一全额席: want exactly one on-chain runnable seat, got %d: %+v", onChainSeats, rank.Items)
	}
	censusFull := 8.0
	if onChainRunnableEff > censusFull+rspaAnchorIdentityTolMs {
		t.Fatalf("chain-tier runnable Σ %.3f must never exceed the census full-window account %.3f", onChainRunnableEff, censusFull)
	}
	// The interval twin is absorbed (E# merge), the non-twin satellite rides
	// ◇ with the represented marker and its value untouched.
	if len(rank.AbsorbedItems) != 1 || rank.AbsorbedItems[0].Type != "scheduler_latency" {
		t.Fatalf("the interval-twin satellite must be absorbed into the chain seat: %+v", rank.AbsorbedItems)
	}
	foundNonTwin := false
	for _, item := range rank.Items {
		if item.Type == "low_frequency" && item.Thread.PID == 200 {
			foundNonTwin = true
			if !item.ChainAnchorRepresentedByChainSeat || item.ChainRelevance != "adjacent" ||
				math.Abs(item.RunnableMs-3.0) > 1e-9 {
				t.Fatalf("the non-twin satellite must ride ◇ with the marker and its value untouched: %+v", item)
			}
		}
	}
	if !foundNonTwin {
		t.Fatalf("the non-twin satellite must stay published on the ◇ lane: %+v", rank.Items)
	}
}

// The represented-demoted satellite is a DIFFERENT account form from a plain
// or R4-demoted row of the same (thread, type, lane) — the family-fold anchor
// key forks on the marker.
func TestXLANEAnchorFormKeyForksOnRepresentedMarker(t *testing.T) {
	plain := RootCauseRankItem{Type: "scheduler_latency"}
	represented := RootCauseRankItem{Type: "scheduler_latency", ChainAnchorRepresentedByChainSeat: true}
	r4 := RootCauseRankItem{Type: "scheduler_latency", ChainCredentialLaneDemoted: true}
	if rootCauseFamilyFoldAnchorFormKey(plain) != "" ||
		rootCauseFamilyFoldAnchorFormKey(represented) != "anchor_represented" ||
		rootCauseFamilyFoldAnchorFormKey(r4) != "lane_demoted" {
		t.Fatalf("anchor form keys drifted: plain=%q represented=%q r4=%q",
			rootCauseFamilyFoldAnchorFormKey(plain), rootCauseFamilyFoldAnchorFormKey(represented), rootCauseFamilyFoldAnchorFormKey(r4))
	}
}
