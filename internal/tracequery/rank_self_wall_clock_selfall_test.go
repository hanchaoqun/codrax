package tracequery

// rank_self_wall_clock_selfall_test.go — SELF-ALL engine pins (§29.61.2 +
// §29.61.2a user rulings 2026-07-13, ledger
// docs/design/real_trace_campaign_20260705.md; SELF-LANE §29.58.3 连带).
//
// Ruling shape: the analysis TARGET's own WALL-CLOCK seat (blocked-state
// family / IO facet / runnable / running) with a typed interval inside the
// query window enters the ON-CHAIN channel on the typed self basis
// (OnChainBasis=self_wall_clock_interval, causality=self_wall_clock) — no
// wakeup edge, no fabricated overlap — and consumes the SAME effective
// attribution ladder as every on-chain row (零特判). Non-wall-clock calibers
// (V2-P0 composite/count) keep the ⌗ side rail; wait-symptom lanes keep the
// SYM demotion; §23.1 道别红线 stays byte-identical for non-target threads.
//
// Fixture red line (§29.53 产线实铸形): the positive witness family is
// engine-parsed and engine-minted from the VERBATIM real donghu.ftrace
// capture — the 133136 customer witness window (13762.791708..13763.024898,
// target .ugc.aweme.lite-17267) whose ◇ 内 io_latency seats this ruling
// promotes.
//
// MUTATION self-checks:
//   - M1 removing the enrich self arm (enrichRootCauseItemsWithChainContext)
//     reds TestSelfAllDonghuIOSeatEntersOnChainChannel — the io_latency
//     family falls back to the ◇ adjacent channel;
//   - M2 (突变实录 2026-07-13: SURVIVES by redundancy — recorded honestly):
//     removing the V2-P0 caliber gate from selfWallClockSeatTokenAdmits alone
//     does NOT red anything, because the ⌗ exclusion is structurally
//     triple-guarded (registry Additivity!=wall_clock blocks the count
//     family, the item arm's seat families block composite rows, and
//     adjacentIOFacetUnionFacet carries its own caliber carve) — a REAL
//     regression must break the behavior pin
//     TestSelfAllNonWallClockCalibersStayOnSideRail, which asserts the final
//     tiers/channels on the real capture;
//   - M3 removing the proof-basis lane dimension from
//     rootCauseFamilyFoldLaneKey reds
//     TestSelfAllOverlapProvenSeatStaysASeparateSeat — the 1.347ms
//     overlap-proven row would fold into the self-basis family (两把尺混折);
//   - M4 removing the keep arm (rootCauseOnChainBasisIsSelf in
//     rootCauseChainContextForItem) reds TestSelfAllKeepArmHoldsLaneOnReEnrich;
//   - M5 removing the critical_blocking face arm reds
//     TestSelfAllCriticalBlockingFaceMirrorsTheVerdict.

import (
	"context"
	"fmt"
	"testing"
)

func selfAllDonghuQuery() Query {
	return Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
}

func selfAllDonghuIndex(t *testing.T) *Index {
	t.Helper()
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

// TestSelfAllDonghuIOSeatEntersOnChainChannel is the positive witness pin
// (production-minted, the 133136 window verbatim): the target's own io_latency
// wall-clock family — pre-ruling a ◇ adjacent seat — rides the on-chain
// channel on the typed self basis with a chain-channel ordinal, the ordinary
// election ladder and the unmodified D/IO wall-clock effective (§29.61.2a
// 同一阶梯: published effective == the window-clipped wall-clock union, no
// re-pricing, no boost, no fabricated overlap).
func TestSelfAllDonghuIOSeatEntersOnChainChannel(t *testing.T) {
	rank := BuildRootCauseRank(selfAllDonghuIndex(t), selfAllDonghuQuery())
	var seat *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "io_latency" && rank.Items[i].OnChainBasis != "" {
			seat = &rank.Items[i]
			break
		}
	}
	if seat == nil {
		t.Fatalf("fixture drifted: no self-basis io_latency seat minted: %+v", rank.Items)
	}
	if !seat.SubjectIsAnalysisTarget || seat.Thread.PID != 17267 {
		t.Fatalf("the promoted seat must be the analysis target's own: %+v", seat)
	}
	if seat.ChainRelevance != "on_chain" || seat.OnChainBasis != RootCauseOnChainBasisSelfWallClockInterval {
		t.Fatalf("target self wall-clock seat must enter the on-chain channel on the typed self basis: %+v", seat)
	}
	if seat.Causality != RootCauseCausalitySelfWallClock {
		t.Fatalf("self seat must speak the honest causality token, never a wakeup-edge claim: %+v", seat)
	}
	if seat.OverlapMs != 0 {
		t.Fatalf("self basis must not fabricate a chain-window overlap: %+v", seat)
	}
	// §29.61.2a 同一阶梯: the family's published effective IS the wall-clock
	// window union the ladder gives every on-chain D/IO row (no special lane).
	if got := fmt.Sprintf("%.3f", rootCauseEffectiveImpactMs(*seat)); got != "3.264" {
		t.Fatalf("published effective must be the 3.264ms wall-clock union: %s (%+v)", got, seat)
	}
	if seat.EffectiveImpactMs != seat.CumulativeImpactMs || seat.MemberCount != 5 {
		t.Fatalf("witness family shape drifted (5 member IOs, eff==cum): %+v", seat)
	}
	// 佩序数 (witness acceptance): a chain-channel ordinal on the ordinary
	// election ladder — never the ◇ adjacent ordinal space, never Rank=0.
	if seat.Rank != 6 || seat.Tier != "tertiary" || seat.BackgroundRank != 0 {
		t.Fatalf("witness seat drifted (根因排序#6 · tertiary · no background seat): rank=%d tier=%s bg=%d",
			seat.Rank, seat.Tier, seat.BackgroundRank)
	}
}

// TestSelfAllOverlapProvenSeatStaysASeparateSeat (M3, 两把尺禁混折): the
// target's 1.347ms io_latency proved by genuine chain-window overlap keeps its
// OWN seat with the overlap basis ("" + on_wakeup_chain) — the self-basis
// family and the overlap-proven row never fold into one contender.
func TestSelfAllOverlapProvenSeatStaysASeparateSeat(t *testing.T) {
	rank := BuildRootCauseRank(selfAllDonghuIndex(t), selfAllDonghuQuery())
	var overlap *RootCauseRankItem
	for i := range rank.Items {
		item := &rank.Items[i]
		if item.Type == "io_latency" && item.SubjectIsAnalysisTarget && item.OnChainBasis == "" {
			overlap = item
			break
		}
	}
	if overlap == nil {
		t.Fatalf("fixture drifted: the overlap-proven io_latency seat vanished: %+v", rank.Items)
	}
	if overlap.ChainRelevance != "on_chain" || overlap.Causality != "on_wakeup_chain" {
		t.Fatalf("the overlap-proven row keeps the legacy lane byte-identically: %+v", overlap)
	}
	if got := fmt.Sprintf("%.3f", overlap.EffectiveImpactMs); got != "1.347" || overlap.MemberCount != 0 {
		t.Fatalf("the overlap-proven 1.347ms row must stay a single separate seat: eff=%s members=%d",
			got, overlap.MemberCount)
	}
}

// TestSelfAllNonWallClockCalibersStayOnSideRail (M2, V2-P0 口径纪律): the
// target's own count/composite rows never take the self basis — the
// page_cache_churn count row keeps the ◇ adjacent channel and the ⌗
// caliber-side tier; the on-chain composite block_io row keeps its legacy
// caliber-side seat with no basis.
func TestSelfAllNonWallClockCalibersStayOnSideRail(t *testing.T) {
	rank := BuildRootCauseRank(selfAllDonghuIndex(t), selfAllDonghuQuery())
	rows := append(append([]RootCauseRankItem{}, rank.Items...), rank.AbsorbedItems...)
	sawCount, sawComposite := false, false
	for _, item := range rows {
		switch item.Type {
		case "page_cache_churn":
			if !item.SubjectIsAnalysisTarget {
				continue
			}
			sawCount = true
			if item.OnChainBasis != "" || item.ChainRelevance != "adjacent" || item.Tier != RootCauseTierCaliberSide {
				t.Fatalf("count-caliber self row must keep the ◇ ⌗ side rail: %+v", item)
			}
		case "block_io_by_inode":
			if !item.SubjectIsAnalysisTarget {
				continue
			}
			sawComposite = true
			if item.OnChainBasis != "" || item.Tier != RootCauseTierCaliberSide {
				t.Fatalf("composite-score self row must keep its ⌗ side seat with no basis: %+v", item)
			}
		}
	}
	if !sawCount || !sawComposite {
		t.Fatalf("fixture drifted: count=%v composite=%v rows missing", sawCount, sawComposite)
	}
}

// TestSelfAllSymptomAndIdleLanesUntouched: the SYM wait-symptom demotion and
// the P9 idle-cadence context lane are OUTSIDE the promotion's closed set —
// the target's binder_wait row keeps tier=target_self_state and the
// pacing_idle row keeps adjacent/context_only, both basis-free.
func TestSelfAllSymptomAndIdleLanesUntouched(t *testing.T) {
	rank := BuildRootCauseRank(selfAllDonghuIndex(t), selfAllDonghuQuery())
	sawBinder, sawPacing := false, false
	for _, item := range rank.Items {
		switch item.Type {
		case "binder_wait":
			sawBinder = true
			if item.OnChainBasis != "" || item.Tier != RootCauseTierTargetSelfState || item.Rank != 0 {
				t.Fatalf("the wait-symptom self row must keep the SYM demotion: %+v", item)
			}
		case "pacing_idle":
			sawPacing = true
			if item.OnChainBasis != "" || item.ChainRelevance != "adjacent" || item.Tier != RootCauseTierContextOnly {
				t.Fatalf("the idle-cadence context row must keep its lane: %+v", item)
			}
		}
	}
	if !sawBinder || !sawPacing {
		t.Fatalf("fixture drifted: binder=%v pacing=%v rows missing", sawBinder, sawPacing)
	}
}

// TestSelfAllNonTargetLanesUntouched pins the §23.1 负向 forms on the real
// capture: NO non-target row carries the self basis or the self causality —
// keva/JankManager/CompThread rows keep their overlap-proven lanes and the
// background rows (wk:0/0/0/14, daily_control) stay background.
func TestSelfAllNonTargetLanesUntouched(t *testing.T) {
	rank := BuildRootCauseRank(selfAllDonghuIndex(t), selfAllDonghuQuery())
	for _, item := range append(append([]RootCauseRankItem{}, rank.Items...), rank.AbsorbedItems...) {
		if item.SubjectIsAnalysisTarget {
			continue
		}
		if item.OnChainBasis != "" || item.Causality == RootCauseCausalitySelfWallClock {
			t.Fatalf("§23.1: a non-target row must never carry the self basis: %+v", item)
		}
	}
	blocking := BuildCriticalBlockingCalls(selfAllDonghuIndex(t), selfAllDonghuQuery())
	sawBackground := false
	for _, cand := range blocking.Items {
		if cand.Thread.PID == 17267 {
			continue
		}
		if cand.OnChainBasis != "" {
			t.Fatalf("§23.1 (critical face): a non-target candidate must never carry the self basis: %+v", cand)
		}
		if cand.Thread.Comm == "wk:0/0/0/14" && cand.Type == "io_latency" {
			sawBackground = true
			if cand.ChainRelevance != "background" {
				t.Fatalf("the non-chain-member thread's IO row stays background: %+v", cand)
			}
		}
	}
	if !sawBackground {
		t.Fatalf("fixture drifted: the wk:0/0/0/14 background io_latency witness vanished")
	}
}

// TestSelfAllCriticalBlockingFaceMirrorsTheVerdict (M5): the display witness
// feeder — every target io_latency critical_blocking candidate with a typed
// in-window interval rides on_chain on the SAME self basis (one 道别
// predicate, two consumers); the wait-symptom binder row keeps its legacy
// on-chain-by-overlap lane with no basis.
func TestSelfAllCriticalBlockingFaceMirrorsTheVerdict(t *testing.T) {
	q := selfAllDonghuQuery()
	q.Limit = 24 // the 12-row cap tail-cuts two of the five witness IO rows
	blocking := BuildCriticalBlockingCalls(selfAllDonghuIndex(t), q)
	promoted := 0
	for _, cand := range blocking.Items {
		if cand.Thread.PID != 17267 {
			continue
		}
		switch cand.Type {
		case "io_latency":
			if cand.ChainRelevance != "on_chain" {
				t.Fatalf("target io_latency candidate must ride on_chain: %+v", cand)
			}
			if cand.OnChainBasis == RootCauseOnChainBasisSelfWallClockInterval {
				promoted++
				if cand.OverlapMs != 0 {
					t.Fatalf("self-basis candidate must not fabricate overlap: %+v", cand)
				}
			}
		case "binder_wait":
			if cand.OnChainBasis != "" {
				t.Fatalf("the symptom-lane binder candidate never takes the self basis: %+v", cand)
			}
		}
	}
	// The 133136 witness carries FIVE adjacent-era target io_latency rows
	// (1.248/1.058/1.056/0.884/0.865); the 1.347 row was already
	// overlap-proven and keeps its empty basis.
	if promoted != 5 {
		t.Fatalf("witness drifted: want 5 self-basis io_latency candidates, got %d", promoted)
	}
}

// TestSelfAllKeepArmHoldsLaneOnReEnrich (M4): the lane is decided ONCE — a
// re-enrich (the scheduler pass re-runs the candidate context) must keep the
// self verdict instead of demoting the row back through the same-thread
// no-overlap arm.
func TestSelfAllKeepArmHoldsLaneOnReEnrich(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	chain := ChainResult{
		Target: target,
		Window: TimeWindow{StartTs: 5.0, EndTs: 5.1},
		Nodes:  []ChainNode{{Thread: target, Window: TimeWindow{StartTs: 5.05, EndTs: 5.08}}},
	}
	item := rootCauseItem("io_latency", target, 1.0, 0.86, 3, 4, "window_stats", "probe")
	item.StartTs, item.EndTs = 5.01, 5.02 // in window, no chain-node overlap
	items := enrichRootCauseItemsWithChainContext(chain, []RootCauseRankItem{item})
	if items[0].ChainRelevance != "on_chain" || items[0].OnChainBasis != RootCauseOnChainBasisSelfWallClockInterval {
		t.Fatalf("first enrich must promote the self wall-clock seat: %+v", items[0])
	}
	items = enrichRootCauseItemsWithChainContext(chain, items)
	if items[0].ChainRelevance != "on_chain" || items[0].OnChainBasis != RootCauseOnChainBasisSelfWallClockInterval ||
		items[0].OverlapMs != 0 || items[0].Causality != RootCauseCausalitySelfWallClock {
		t.Fatalf("keep arm must hold the verdict on re-enrich: %+v", items[0])
	}
	// The keep arm's LOAD-BEARING shape (突变实录 2026-07-13: the plain
	// re-enrich above re-decides identically, so the M4 knockout survives it):
	// a promoted row whose typed interval was later CLEARED (family-fold /
	// carrier reshaping) re-enriches through the no-candidate-window
	// same-thread arm, which would fabricate a whole-node-window OverlapMs on
	// the on-chain verdict — the keep arm zeroes it (不伪造重叠).
	held := items[0]
	held.StartTs, held.EndTs = 0, 0
	held.OverlapMs = 0
	kept := enrichRootCauseItemsWithChainContext(chain, []RootCauseRankItem{held})
	if kept[0].ChainRelevance != "on_chain" || kept[0].OnChainBasis != RootCauseOnChainBasisSelfWallClockInterval {
		t.Fatalf("keep arm must hold the lane on the interval-less shape: %+v", kept[0])
	}
	if kept[0].OverlapMs != 0 {
		t.Fatalf("keep arm must zero the fabricated overlap on the interval-less shape (M4): %+v", kept[0])
	}
}

// TestSelfAllSharedPredicateConditions unit-pins the typed admission
// conditions of the shared predicate + token arm — removing any condition
// reds its arm.
func TestSelfAllSharedPredicateConditions(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	chain := &ChainResult{
		Target: target,
		Window: TimeWindow{StartTs: 5.0, EndTs: 5.1},
		Nodes:  []ChainNode{{Thread: target, Window: TimeWindow{StartTs: 5.05, EndTs: 5.08}}},
	}
	if !selfWallClockSeatLane(chain, target, 5.01, 5.02) {
		t.Fatalf("full condition set must admit")
	}
	if selfWallClockSeatLane(nil, target, 5.01, 5.02) {
		t.Fatalf("nil chain must never admit")
	}
	if selfWallClockSeatLane(&ChainResult{Target: target, Window: chain.Window}, target, 5.01, 5.02) {
		t.Fatalf("an EMPTY chain universe must never admit")
	}
	if selfWallClockSeatLane(&ChainResult{Nodes: chain.Nodes, Window: chain.Window}, target, 5.01, 5.02) {
		t.Fatalf("an unresolved target must never admit (absence never guesses)")
	}
	if selfWallClockSeatLane(chain, ThreadRef{Comm: "peer", PID: 200}, 5.01, 5.02) {
		t.Fatalf("a non-target thread must never admit (§23.1 道别红线)")
	}
	if selfWallClockSeatLane(chain, target, 5.02, 5.02) {
		t.Fatalf("a zero-extent interval must never admit (真无区间 residual class)")
	}
	if selfWallClockSeatLane(chain, target, 4.0, 4.5) {
		t.Fatalf("an out-of-window interval must never admit")
	}
	// Token arm: wall-clock seat families only.
	for token, want := range map[string]bool{
		"io_latency":         true,  // IO facet union member
		"io_wait":            true,  // D/IO caliber + IO facet
		"d_state_or_io_wait": true,  // D/IO caliber
		"runnable_wait":      true,  // runnable caliber
		"running":            true,  // running caliber (供给折算阶梯照旧)
		"scheduler_latency":  true,  // runnable caliber
		"io_burst_episode":   true,  // IO facet union member
		"page_cache_churn":   false, // V2-P0 count caliber — ⌗ 旁栏
		"block_io_by_inode":  false, // V2-P0 composite score — ⌗ 旁栏
		"file_io_hot_inode":  false, // count advisory
		"sleep_wait":         false, // wait-symptom lane (根因在对端)
		"binder_wait":        false, // wait-symptom lane
		"pacing_idle":        false, // idle-cadence context (wakeup_chain lane)
		"jit_compile":        false, // SELF-SEM owns the deterministic classes
		"trace_span":         false, // generic span context — not a seat family
		"cpu_pressure":       false, // aggregate-only subject
		"io_pressure":        false, // cross-thread pressure score
		"not_a_token":        false, // unregistered — absence never promotes
	} {
		if got := selfWallClockSeatTokenIsSeatFamily(token); got != want {
			t.Fatalf("token arm drifted for %q: got %v want %v", token, got, want)
		}
	}
}

// TestSelfAllEffectiveLadderIsUntouched (§29.61.2a 零特判): the promotion
// never re-prices a row — the per-state effective ladder reads the SAME
// fields before and after the basis stamp (runnable=full, D/IO=wall-clock
// sum, running=supply-fold deficit, raw running never resurrects).
func TestSelfAllEffectiveLadderIsUntouched(t *testing.T) {
	runnable := RootCauseRankItem{Type: "runnable_wait", DominantState: string(StateRunnable), RunnableMs: 2.5,
		ChainRelevance: "on_chain", OnChainBasis: RootCauseOnChainBasisSelfWallClockInterval}
	if got := rootCauseEffectiveImpactMs(runnable); got != 2.5 {
		t.Fatalf("runnable ladder must stay 全额: %v", got)
	}
	dio := RootCauseRankItem{Type: "d_state_or_io_wait", DominantState: string(StateDSleep), DStateMs: 3.0, IOWaitMs: 1.0,
		ChainRelevance: "on_chain", OnChainBasis: RootCauseOnChainBasisSelfWallClockInterval}
	if got := rootCauseEffectiveImpactMs(dio); got != 4.0 {
		t.Fatalf("D/IO ladder must stay 墙钟合计: %v", got)
	}
	running := RootCauseRankItem{Type: "running", DominantState: string(StateRunning), RunningMs: 3.0,
		EffectiveImpactMs: 0.5, ChainRelevance: "on_chain", OnChainBasis: RootCauseOnChainBasisSelfWallClockInterval}
	if got := rootCauseEffectiveImpactMs(running); got != 0.5 {
		t.Fatalf("running ladder must stay 供给折算 (raw 3.0 must never resurrect): %v", got)
	}
}

// TestSelfAllSelfBasisEffectiveIdentity (修复轮 件5 F5, 2026-07-13): the
// §29.61.2a 零特判 identity with MUTATION discriminating power (the earlier
// hand-item pin survived a ×1.15 boost at the promotion site) — SELF-SEM M2
// 同款恒等式:
//   - real capture: EVERY self-basis row publishes eff==cum==imp (the proof
//     path flip carries the value; any boost forks the triple);
//   - normalize pass: a boosted arrival on a self-basis runnable/D-IO row is
//     REPLACED by the per-state ladder recomputation (canonical wins);
//   - a running self-basis row with a computed supply fold publishes
//     eff==SupplyFoldDeficitMs (§20.2 identity, print precision).
func TestSelfAllSelfBasisEffectiveIdentity(t *testing.T) {
	rank := BuildRootCauseRank(selfAllDonghuIndex(t), selfAllDonghuQuery())
	seen := 0
	for _, item := range rank.Items {
		if item.OnChainBasis != RootCauseOnChainBasisSelfWallClockInterval {
			continue
		}
		seen++
		if item.EffectiveImpactMs != item.CumulativeImpactMs || item.EffectiveImpactMs != item.ImpactMs {
			t.Fatalf("self-basis row must publish eff==cum==imp (零特判恒等式): %+v", item)
		}
	}
	if seen == 0 {
		t.Fatalf("fixture drifted: no self-basis row on the witness window")
	}
	// Ladder-recompute identity: a boosted arrival never publishes.
	boostedRunnable := RootCauseRankItem{Type: "runnable_wait", DominantState: string(StateRunnable),
		RunnableMs: 2.5, EffectiveImpactMs: 2.5 * 1.15,
		ChainRelevance: "on_chain", OnChainBasis: RootCauseOnChainBasisSelfWallClockInterval,
		Causality: RootCauseCausalitySelfWallClock}
	boostedDIO := RootCauseRankItem{Type: "d_state_or_io_wait", DominantState: string(StateDSleep),
		DStateMs: 3.0, IOWaitMs: 1.0, EffectiveImpactMs: 4.0 * 1.15,
		ChainRelevance: "on_chain", OnChainBasis: RootCauseOnChainBasisSelfWallClockInterval,
		Causality: RootCauseCausalitySelfWallClock}
	items := []RootCauseRankItem{boostedRunnable, boostedDIO}
	normalizeRootCauseEffectiveImpact(items)
	if items[0].EffectiveImpactMs != 2.5 {
		t.Fatalf("normalize must restore the runnable ladder over a boosted arrival: %v", items[0].EffectiveImpactMs)
	}
	if items[1].EffectiveImpactMs != 4.0 {
		t.Fatalf("normalize must restore the D/IO ladder over a boosted arrival: %v", items[1].EffectiveImpactMs)
	}
	// Running arm: the supply-fold deficit identity (the authoritative
	// running effective is the deficit — never the raw running wall clock).
	running := RootCauseRankItem{Type: "running", DominantState: string(StateRunning),
		RunningMs: 3.0, EffectiveImpactMs: 0.5, SupplyFoldDeficitMs: 0.5, SupplyFoldIdealMs: 2.5,
		SupplyFoldBasis: &SupplyFoldBasis{KnownMs: 3.0},
		ChainRelevance:  "on_chain", OnChainBasis: RootCauseOnChainBasisSelfWallClockInterval,
		Causality: RootCauseCausalitySelfWallClock}
	if got := rootCauseEffectiveImpactMs(running); got != running.SupplyFoldDeficitMs {
		t.Fatalf("running self-basis row must publish the supply-fold deficit identity: %v", got)
	}
}

// TestSelfAllFoldLaneKeyCarriesBasis (M3 unit half): the family fold's 道别
// key separates the proof bases — an overlap-proven on-chain row and a
// self-basis row never share a merge lane.
func TestSelfAllFoldLaneKeyCarriesBasis(t *testing.T) {
	overlap := RootCauseRankItem{Type: "io_latency", ChainRelevance: "on_chain"}
	selfBasis := RootCauseRankItem{Type: "io_latency", ChainRelevance: "on_chain",
		OnChainBasis: RootCauseOnChainBasisSelfWallClockInterval, Causality: RootCauseCausalitySelfWallClock}
	if k1, k2 := rootCauseFamilyFoldLaneKey(overlap), rootCauseFamilyFoldLaneKey(selfBasis); k1 == k2 {
		t.Fatalf("两把尺禁混折: overlap and self-basis lanes must never share a fold key: %q", k1)
	}
	if got := rootCauseFamilyFoldLaneKey(selfBasis); got != "on_chain|self_wall_clock_interval" {
		t.Fatalf("self lane key drifted: %q", got)
	}
}

// TestSelfAllPartitionSeatsPromoteWithoutRemerge (修复轮 件6 AS4 形式化,
// §29.50.5 相容核, 2026-07-13; the adversarial review's AS4 补形 PASS frozen
// as a pin): the target's own D/IO PROOF-PARTITION pair — a proven cause seat
// (BlockedReasonCaller) beside its honest-remainder sibling
// (DStateCauseUnprovenRemainder) — promotes on the self basis like any
// wall-clock seat, and the partition identities NEVER re-merge through the
// promotion (the fold participation key keeps its §29.50.5 root-cause-identity
// dimension beside the new basis dimension; 假设永不并).
func TestSelfAllPartitionSeatsPromoteWithoutRemerge(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	chain := ChainResult{
		Target: target,
		Window: TimeWindow{StartTs: 5.0, EndTs: 5.2},
		Nodes:  []ChainNode{{Thread: target, Window: TimeWindow{StartTs: 5.15, EndTs: 5.18}}},
	}
	cause := rootCauseItem("d_state_or_io_wait", target, 3.0, 0.8, 10, 20, "window_stats", "proven cause slice")
	cause.StartTs, cause.EndTs = 5.01, 5.04
	cause.DominantState = string(StateDSleep)
	cause.DStateMs = 3.0
	cause.BlockedReasonCaller = "dma_fence_default_w"
	remainder := rootCauseItem("d_state_or_io_wait", target, 2.0, 0.8, 30, 40, "window_stats", "unproven remainder")
	remainder.StartTs, remainder.EndTs = 5.05, 5.07
	remainder.DominantState = string(StateDSleep)
	remainder.DStateMs = 2.0
	remainder.DStateCauseUnprovenRemainder = true
	items := enrichRootCauseItemsWithChainContext(chain, []RootCauseRankItem{cause, remainder})
	for i := range items {
		if items[i].ChainRelevance != "on_chain" || items[i].OnChainBasis != RootCauseOnChainBasisSelfWallClockInterval {
			t.Fatalf("AS4: partition seat %d must promote on the self basis: %+v", i, items[i])
		}
	}
	folded := foldSameThreadTypeRankFamilies(Query{}, true, items)
	if len(folded) != 2 {
		t.Fatalf("AS4 相容核: the proven cause seat and its remainder sibling must NEVER re-merge (假设永不并), got %d rows: %+v", len(folded), folded)
	}
	sawCause, sawRemainder := false, false
	for _, item := range folded {
		if item.BlockedReasonCaller == "dma_fence_default_w" && item.DStateMs == 3.0 {
			sawCause = true
		}
		if item.DStateCauseUnprovenRemainder && item.DStateMs == 2.0 {
			sawRemainder = true
		}
	}
	if !sawCause || !sawRemainder {
		t.Fatalf("AS4: both partition identities must survive verbatim: cause=%v remainder=%v", sawCause, sawRemainder)
	}
}

// TestSelfAllIOBurstFaceMirrorsTheVerdict: the io_burst episode context
// enrich promotes the target's own adjacent episode on the same shared
// predicate (three consumers, one implementation); non-target episodes keep
// their lanes byte-identically.
func TestSelfAllIOBurstFaceMirrorsTheVerdict(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	chain := ChainResult{
		Target: target,
		Window: TimeWindow{StartTs: 5.0, EndTs: 5.1},
		Nodes:  []ChainNode{{Thread: target, Window: TimeWindow{StartTs: 5.05, EndTs: 5.08}}},
	}
	episodes := enrichIOBurstEpisodesWithChainContext(chain, []IOBurstEpisodeSummary{
		{Thread: target, StartTs: 5.01, EndTs: 5.02, DurationMs: 10},
		{Thread: ThreadRef{Comm: "peer", PID: 200}, StartTs: 5.01, EndTs: 5.02, DurationMs: 20},
	})
	for _, ep := range episodes {
		if ep.Thread.PID == 100 {
			if ep.ChainRelevance != "on_chain" || ep.OnChainBasis != RootCauseOnChainBasisSelfWallClockInterval {
				t.Fatalf("target self episode must promote on the self basis: %+v", ep)
			}
			continue
		}
		if ep.OnChainBasis != "" || ep.ChainRelevance == "on_chain" {
			t.Fatalf("non-target episode keeps its lane (§23.1): %+v", ep)
		}
	}
}
