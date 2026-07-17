package tracequery

// rank_caliber_rkc_p0e_test.go — P0-E engine batch pins (ledger §20 RKC audit
// + §20.1 rulings, docs/design/real_trace_campaign_20260705.md, 2026-07-07).
//
//  ① De-double-publication: a running-dominant on-chain segment mints exactly
//     ONE rank row (the CausalImpacts lane); the RootEvidence running twin no
//     longer enters the rank funnel (q6 verbatim: gated 37.410 ranks, raw
//     58.919 rides the cumulative face, supply-fold 17.702 rides typed).
//  ② Depth-0 target-own running keeps its rank row — now minted by the
//     CausalImpacts lane WITH the full typed field set (§20 A-fix(3)).
//  ③ Merge caliber (§7.10 fourth branch): non-inversion running rows rank by
//     the raw RUNNING wall clock, never the sleep-inclusive TotalMs backfill.
//  ④ Aggregate gated (E-Gap②): inversion-typed aggregate rows rank by the
//     R5d gated caliber — SUM over disjoint occurrence windows, honest MAX
//     fallback on overlap.
//  ⑤ AggregateOnly rows never upgrade to the adjacent tier on noisy window
//     overlap (E-Gap③) — always background beside a chain.
//  ⑥ RunnableTop inversion retype re-passes the registry guard and re-derives
//     Score (B/D-Gap④, state_churn precedent); resolved blocking_span rows
//     weigh 1.35 (§20.1 ruling ②) so a measured lock hold never scores below
//     a generic trace_span.
//  ⑦ binder_wait / io_latency rank rows carry the RCX① drill verdict
//     (E-Gap⑥, symmetric with the critical_blocking face).

import (
	"fmt"
	"testing"
)

func rkcQ6InversionImpact() WakeupCausalImpact {
	return WakeupCausalImpact{
		Thread:                     ThreadRef{Comm: "dep", PID: 200},
		Window:                     TimeWindow{StartTs: 10.0, EndTs: 10.0821},
		ChainDepth:                 1,
		OnChain:                    true,
		DominantState:              string(StateRunning),
		DominantImpactMs:           58.919,
		RunningMs:                  58.919,
		RunnableMs:                 20.713,
		SleepMs:                    2.5,
		TotalMs:                    82.132,
		TargetBlockedMs:            82.132,
		PriorityInversionCandidate: true,
		PriorityInversionGatedMs:   37.410,
		GatedRunnableMs:            20.713,
		GatedRunningDeficitMs:      16.697,
		SupplyFoldDeficitMs:        17.702,
		SupplyFoldIdealMs:          41.217,
		SupplyFoldBasis:            &SupplyFoldBasis{KnownMs: 58.919},
		LineStart:                  100,
		LineEnd:                    180,
		Summary:                    "dep running-dominant inversion candidate",
	}
}

// Pin ① (§20 headline / §20.1 ruling ①甲, q6 verbatim numbers): one segment,
// one rank row — gated 37.410 is the sort key, raw running 58.919 stays on
// the cumulative face, and the supply-fold deficit 17.702 travels typed on
// the SAME row. The RootEvidence running twin (the raw-58.919 co-primary that
// bypassed all three fold generations) must not resurrect in the pool.
func TestRunningDominantInversionSegmentMintsSingleGatedRankRow(t *testing.T) {
	impact := rkcQ6InversionImpact()
	chain := ChainResult{
		Target: ThreadRef{Comm: "app", PID: 100},
		Nodes: []ChainNode{
			{ID: "n1", Thread: ThreadRef{Comm: "app", PID: 100}, Window: TimeWindow{StartTs: 10.0, EndTs: 10.0821}},
			{ID: "n2", Thread: ThreadRef{Comm: "dep", PID: 200}, Window: TimeWindow{StartTs: 10.0, EndTs: 10.0821}},
		},
		CausalImpacts: []WakeupCausalImpact{impact},
		// Exactly what expandChain's case StateRunning used to feed the
		// funnel: the running twin of the SAME impact.
		RootEvidence: []RootEvidence{
			rootEvidenceFromCausalImpact(impact, "thread was running in the aligned window; its own CPU work is root-cause evidence", 0.75),
		},
	}
	rank := buildRootCauseRankFrom(nil, Query{TimeStart: 10.0, TimeEnd: 10.0821}, chain, WindowStats{})
	var depRows []RootCauseRankItem
	for _, item := range rank.Items {
		if item.Thread.PID == 200 {
			depRows = append(depRows, item)
		}
	}
	if len(depRows) != 1 {
		t.Fatalf("一段一行: running-dominant inversion segment must mint exactly ONE rank row, got %d: %+v", len(depRows), depRows)
	}
	row := depRows[0]
	if row.Type != "priority_inversion_candidate" {
		t.Fatalf("merged row must keep the inversion type, got %q", row.Type)
	}
	if got := fmt.Sprintf("%.3f", rootCauseEffectiveImpactMs(row)); got != "37.410" {
		t.Fatalf("gated 参赛: sort/effective must be the R5d gated composite 37.410, got %s", got)
	}
	if got := fmt.Sprintf("%.3f", row.ImpactMs); got != "37.410" {
		t.Fatalf("published impact must be gated 37.410, got %s", got)
	}
	if got := fmt.Sprintf("%.3f", row.CumulativeImpactMs); got != "58.919" {
		t.Fatalf("raw 在 cumulative: the dead twin's raw running wall clock 58.919 must ride the cumulative face, got %s", got)
	}
	if got := fmt.Sprintf("%.3f", row.SupplyFoldDeficitMs); got != "17.702" {
		t.Fatalf("fold note: the supply-fold deficit 17.702 must travel typed on the merged row, got %s", got)
	}
	if got := fmt.Sprintf("%.3f", row.RunningMs); got != "58.919" {
		t.Fatalf("state face must keep the full split (running=58.919), got %s", got)
	}
	if wantScore := 37.410 * 0.91 * rootCauseScoreWeightChainImpact; !rkcFloatEq(row.Score, wantScore) {
		t.Fatalf("Score must be gated × conf × chain lane weight, got %f want %f", row.Score, wantScore)
	}
	// Mutation red: nothing in the pool ranks with the raw 58.919 for this
	// thread — the twin lane is dead.
	for _, item := range rank.Items {
		if item.Thread.PID == 200 && fmt.Sprintf("%.3f", rootCauseEffectiveImpactMs(item)) == "58.919" {
			t.Fatalf("raw running twin resurrected in the rank pool: %+v", item)
		}
	}
}

// Pin ② (§20 A-fix(3)): the depth-0/target-own running scenario REALLY exists
// (BuildWakeupChain enters expandChain at depth 0, and the CausalImpacts rank
// loop used to skip ChainDepth<=0 — the impoverished RootEvidence twin was
// the only rank carrier). After de-double-publication the CausalImpacts lane
// mints that row with the full typed field set (window + state totals +
// source), verified on the REAL pipeline (P0-A2 lesson: parse → chain → rank).
func TestDepthZeroTargetOwnRunningMintsTypedRankRow(t *testing.T) {
	idx := buildTraceIndex(t, "rkc_depth0_running.systrace", `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.050000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
	`)
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.050, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	var runningRows []RootCauseRankItem
	for _, item := range rank.Items {
		if item.Thread.PID == 100 && item.Type == "running" {
			runningRows = append(runningRows, item)
		}
	}
	if len(runningRows) != 1 {
		t.Fatalf("target-own running must keep exactly ONE rank row, got %d: %+v", len(runningRows), rank.Items)
	}
	row := runningRows[0]
	if row.Source != "wakeup_chain.causal_impacts" {
		t.Fatalf("depth-0 running row must be minted by the CausalImpacts lane (typed fields), got source=%q", row.Source)
	}
	if row.StartTs <= 0 || row.EndTs <= row.StartTs {
		t.Fatalf("depth-0 running row must carry its typed window, got %+v", row)
	}
	if row.DominantState != string(StateRunning) || row.RunningMs <= 0 {
		t.Fatalf("depth-0 running row must carry the typed state split, got %+v", row)
	}
	// EVOLUTION RECORD (§20.2 user ruling 2026-07-07, overturns the §20.1甲
	// raw side clause — not a regression): this pin originally asserted the
	// row ranks by its raw running wall clock; under §20.2 the attribution is
	// the ELIMINABLE deficit. This fixture carries no cpu_frequency data, so
	// the fold basis is all-unknown, the deficit folds to 0 (§7.10 不伪造)
	// and the attribution is authoritatively ≈0 — raw stays a display fact
	// (state split / cumulative) and never drives ranking.
	if !rkcFloatEq(rootCauseEffectiveImpactMs(row), 0) {
		t.Fatalf("§20.2: un-folded depth-0 running attribution must be 0 (raw never drives ranking), got eff=%f", rootCauseEffectiveImpactMs(row))
	}
	if row.RunningMs <= 0 || row.CumulativeImpactMs <= 0 {
		t.Fatalf("raw display facts (state split / cumulative) must survive on the row, got %+v", row)
	}
}

// Pin ③ — EVOLUTION RECORD (§20.2 user ruling 2026-07-07, overturns the
// §20.1甲 "raw 参赛" side clause; the pre-§20.2 shape of this pin asserted
// effective==DominantImpactMs): a NON-inversion running row's ATTRIBUTION
// (effective / sort / Score) is its ELIMINABLE supply-fold deficit. §20.2
// three shapes:
//
//	① full-frequency running (computed fold, deficit=0) → attribution ≈ 0,
//	  not a root cause (sorts below any positive-attribution row);
//	② weak-core running → attribution = deficit (never raw, never Total);
//	③ frequency data missing (no fold / unknown basis) → deficit folds to 0,
//	  raw stays display-only and never drives the sort.
func TestNonInversionRunningAttributionIsEliminableDeficit(t *testing.T) {
	base := WakeupCausalImpact{
		Thread:           ThreadRef{Comm: "worker", PID: 300},
		Window:           TimeWindow{StartTs: 20.0, EndTs: 20.05},
		ChainDepth:       1,
		OnChain:          true,
		DominantState:    string(StateRunning),
		DominantImpactMs: 30,
		RunningMs:        30,
		SleepMs:          20,
		TotalMs:          50,
	}

	// ② weak-core: computed fold with a positive eliminable deficit.
	weak := base
	weak.SupplyFoldDeficitMs = 12
	weak.SupplyFoldIdealMs = 18
	weak.SupplyFoldBasis = &SupplyFoldBasis{KnownMs: 30}
	item := rootCauseItemFromCausalImpact(weak)
	if !rkcFloatEq(item.EffectiveImpactMs, 12) {
		t.Fatalf("§20.2: weak-core running attribution must be the eliminable deficit 12, got %f", item.EffectiveImpactMs)
	}
	if !rkcFloatEq(item.ImpactMs, 30) || !rkcFloatEq(item.CumulativeImpactMs, 50) {
		t.Fatalf("raw stays display-only (bar impact 30 / cumulative Total 50), got impact=%f cumulative=%f", item.ImpactMs, item.CumulativeImpactMs)
	}
	if got := WakeupCausalImpactEffectiveImpactMs(weak); !rkcFloatEq(got, 12) {
		t.Fatalf("exported helper must mirror the rank lane (12), got %f", got)
	}
	if wantScore := 12 * 0.86 * rootCauseScoreWeightChainImpact; !rkcFloatEq(item.Score, wantScore) {
		t.Fatalf("Score is an attribution channel — deficit-based, got %f want %f", item.Score, wantScore)
	}

	// ① full-frequency: computed fold, deficit exactly 0 — authoritative,
	// the cumulative fallback must not resurrect the raw.
	full := base
	full.SupplyFoldIdealMs = 30
	full.SupplyFoldBasis = &SupplyFoldBasis{KnownMs: 30}
	fullItem := rootCauseItemFromCausalImpact(full)
	if !rkcFloatEq(rootCauseEffectiveImpactMs(fullItem), 0) {
		t.Fatalf("§20.2: full-frequency running (deficit=0) attribution must be 0, got %f", rootCauseEffectiveImpactMs(fullItem))
	}
	if got := WakeupCausalImpactEffectiveImpactMs(full); !rkcFloatEq(got, 0) {
		t.Fatalf("exported helper must mirror the authoritative 0, got %f", got)
	}

	// ③ frequency data missing: no fold at all — deficit 0 the same
	// conservative way; raw 30/50 must not drive the sort. A modest
	// positive-attribution runnable row must outrank both zero-attribution
	// running shapes.
	unfolded := base
	unfoldedItem := rootCauseItemFromCausalImpact(unfolded)
	if !rkcFloatEq(rootCauseEffectiveImpactMs(unfoldedItem), 0) {
		t.Fatalf("§20.2: un-folded running attribution must be 0, got %f", rootCauseEffectiveImpactMs(unfoldedItem))
	}
	runnableRow := rootCauseItem("runnable_wait", ThreadRef{Comm: "peer", PID: 400}, 10, 0.76, 1, 2, "window_stats", "peer runnable")
	runnableRow.Causality = "on_wakeup_chain"
	runnableRow.ChainRelevance = "on_chain"
	items := []RootCauseRankItem{unfoldedItem, fullItem, runnableRow}
	sortRootCauseRankItems(items, true)
	if items[0].Type != "runnable_wait" {
		t.Fatalf("raw 不驱动排序: the positive-attribution row must outrank zero-deficit running rows, got head %+v", items[0])
	}
}

func rkcGatedMember(startTs, endTs, runnable, deficit float64, line int) WakeupCausalImpact {
	return WakeupCausalImpact{
		Thread:                          ThreadRef{Comm: "dep", PID: 300},
		Window:                          TimeWindow{StartTs: startTs, EndTs: endTs},
		ChainDepth:                      1,
		OnChain:                         true,
		DominantState:                   string(StateRunnable),
		DominantImpactMs:                runnable,
		RunnableMs:                      runnable,
		TotalMs:                         runnable + deficit + 1,
		PriorityRelation:                "lower_priority_dependency",
		PriorityRelationCaliber:         string(priorityCaliberClosedRangeStable),
		PriorityRelationProvenLowerMs:   runnable + deficit,
		PriorityRelationArtifactSources: []string{"compat:index"},
		PriorityInversionCandidate:      true,
		PriorityInversionGatedMs:        runnable + deficit,
		GatedRunnableMs:                 runnable,
		GatedRunningDeficitMs:           deficit,
		LineStart:                       line,
		LineEnd:                         line + 5,
	}
}

// Pin ④a (§20 E-Gap②): disjoint member occurrence windows — the aggregate
// gated caliber is the SUM (per-thread wall additive over disjoint
// intervals), and the inversion-typed aggregate rank row publishes and ranks
// with it (per-occurrence caliber parity).
func TestAggregateInversionGatedSumOverDisjointOccurrences(t *testing.T) {
	chain := &ChainResult{CausalImpacts: []WakeupCausalImpact{
		rkcGatedMember(10.000, 10.010, 2.0, 0.5, 10),
		rkcGatedMember(10.020, 10.030, 3.0, 0.5, 40),
	}}
	aggs := aggregateWakeupCausalImpacts(chain)
	if len(aggs) != 1 {
		t.Fatalf("expected one aggregate, got %+v", aggs)
	}
	agg := aggs[0]
	if !rkcFloatEq(agg.PriorityInversionGatedMs, 6.0) || !rkcFloatEq(agg.GatedRunnableMs, 5.0) || !rkcFloatEq(agg.GatedRunningDeficitMs, 1.0) {
		t.Fatalf("disjoint occurrences must SUM the gated components, got %+v", agg)
	}
	if agg.GatedAggregationCaliber != GatedCaliberSumDisjointOccurrences {
		t.Fatalf("caliber must disclose sum_disjoint_occurrences, got %q", agg.GatedAggregationCaliber)
	}
	item := rootCauseItemFromCausalAggregate(agg)
	if item.Type != "priority_inversion_candidate" {
		t.Fatalf("aggregate must type as inversion candidate, got %q", item.Type)
	}
	if !rkcFloatEq(item.ImpactMs, 6.0) || !rkcFloatEq(item.EffectiveImpactMs, 6.0) {
		t.Fatalf("inversion aggregate must publish/rank with the gated caliber, got impact=%f effective=%f", item.ImpactMs, item.EffectiveImpactMs)
	}
	if !rkcFloatEq(item.GatedRunnableMs, 5.0) || !rkcFloatEq(item.GatedRunningDeficitMs, 1.0) {
		t.Fatalf("gated composition must travel with the aggregate rank row, got %+v", item)
	}
	if wantScore := 6.0 * 0.88 * rootCauseScoreWeightChainAggregate; !rkcFloatEq(item.Score, wantScore) {
		t.Fatalf("aggregate Score must be gated × conf × aggregate lane weight, got %f want %f", item.Score, wantScore)
	}
	if !rkcFloatEq(item.CumulativeImpactMs, agg.TotalMs) {
		t.Fatalf("raw cross-occurrence total stays on the cumulative face, got %f want %f", item.CumulativeImpactMs, agg.TotalMs)
	}
}

// Pin ④b (§20 E-Gap② honesty fallback): overlapping member windows may
// double-count one physical segment — the aggregate gated value degrades to
// the strongest member (MAX, a lower bound) and the typed caliber says so.
func TestAggregateInversionGatedMaxFallbackOnOverlap(t *testing.T) {
	chain := &ChainResult{CausalImpacts: []WakeupCausalImpact{
		rkcGatedMember(10.000, 10.015, 2.0, 0.5, 10),
		rkcGatedMember(10.010, 10.030, 3.0, 0.5, 40), // overlaps the first
	}}
	aggs := aggregateWakeupCausalImpacts(chain)
	if len(aggs) != 1 {
		t.Fatalf("expected one aggregate, got %+v", aggs)
	}
	agg := aggs[0]
	if !rkcFloatEq(agg.PriorityInversionGatedMs, 3.5) || !rkcFloatEq(agg.GatedRunnableMs, 3.0) || !rkcFloatEq(agg.GatedRunningDeficitMs, 0.5) {
		t.Fatalf("overlapping occurrences must fall back to the strongest member (MAX), got %+v", agg)
	}
	if agg.GatedAggregationCaliber != GatedCaliberMaxOverlapFallback {
		t.Fatalf("caliber must disclose max_overlap_fallback, got %q", agg.GatedAggregationCaliber)
	}
}

// F1 pin (§20.2 absorption, 2026-07-07): a RUNNING-dominant inversion
// aggregate (2 occurrences) types as priority_inversion_candidate and ranks
// by its gated caliber — the pre-F1 shape fell through the priority-sensitive
// gate (runnable/D/IO only), typed as "running" and ranked with raw ΣRunning
// (the raw punch-through the whole §20 batch exists to close).
func TestRunningDominantInversionAggregateRanksGatedNotRawSum(t *testing.T) {
	member := func(startTs, endTs float64, line int) WakeupCausalImpact {
		return WakeupCausalImpact{
			Thread:                          ThreadRef{Comm: "dep", PID: 300},
			Window:                          TimeWindow{StartTs: startTs, EndTs: endTs},
			ChainDepth:                      1,
			OnChain:                         true,
			DominantState:                   string(StateRunning),
			DominantImpactMs:                25,
			RunningMs:                       25,
			RunnableMs:                      2,
			TotalMs:                         27,
			PriorityRelation:                "lower_priority_dependency",
			PriorityRelationCaliber:         string(priorityCaliberClosedRangeStable),
			PriorityRelationProvenLowerMs:   10,
			PriorityRelationArtifactSources: []string{"compat:index"},
			PriorityInversionCandidate:      true,
			PriorityInversionGatedMs:        10,
			GatedRunnableMs:                 2,
			GatedRunningDeficitMs:           8,
			LineStart:                       line,
			LineEnd:                         line + 5,
		}
	}
	chain := &ChainResult{CausalImpacts: []WakeupCausalImpact{
		member(10.000, 10.030, 10),
		member(10.040, 10.070, 40),
	}}
	aggs := aggregateWakeupCausalImpacts(chain)
	if len(aggs) != 1 {
		t.Fatalf("expected one aggregate, got %+v", aggs)
	}
	agg := aggs[0]
	if agg.DominantState != string(StateRunning) {
		t.Fatalf("fixture must be running-dominant, got %q", agg.DominantState)
	}
	if !WakeupCausalAggregateInversionTyped(agg) {
		t.Fatalf("F1: running-dominant inversion aggregate must be inversion-TYPED (per-occurrence gate parity)")
	}
	item := rootCauseItemFromCausalAggregate(agg)
	if item.Type != "priority_inversion_candidate" {
		t.Fatalf("F1: aggregate must type as inversion candidate, got %q", item.Type)
	}
	if !rkcFloatEq(item.ImpactMs, 20) || !rkcFloatEq(item.EffectiveImpactMs, 20) {
		t.Fatalf("F1: aggregate must rank with the gated caliber 20 (2+8 per member, disjoint sum), got impact=%f effective=%f", item.ImpactMs, item.EffectiveImpactMs)
	}
	if rkcFloatEq(rootCauseEffectiveImpactMs(item), 50) {
		t.Fatalf("mutation red: raw ΣRunning 50 punched through the gated caliber")
	}
	// F4: the total is derived from the summed components — the identity
	// total == runnable + deficit holds on the aggregate face.
	if !rkcFloatEq(agg.PriorityInversionGatedMs, agg.GatedRunnableMs+agg.GatedRunningDeficitMs) {
		t.Fatalf("F4: gated total must equal its components sum, got %f vs %f+%f", agg.PriorityInversionGatedMs, agg.GatedRunnableMs, agg.GatedRunningDeficitMs)
	}
}

// Pin ⑤ (§20 E-Gap③, mutation red): an AggregateOnly row (registry Subject)
// overlapping a chain node window stays BACKGROUND — the noisy overlap no
// longer buys the adjacent tier. A Subject==either row with the same shape
// keeps the legacy adjacent upgrade, pinning that the demotion reads the
// PRECISE registry signal, not a blanket thread-emptiness heuristic.
func TestAggregateMetricRowsNeverUpgradeToAdjacent(t *testing.T) {
	chain := ChainResult{
		Target: ThreadRef{Comm: "app", PID: 100},
		Nodes: []ChainNode{
			{ID: "n1", Thread: ThreadRef{Comm: "app", PID: 100}, Window: TimeWindow{StartTs: 10.0, EndTs: 10.1}},
			{ID: "n2", Thread: ThreadRef{Comm: "dep", PID: 200}, Window: TimeWindow{StartTs: 10.0, EndTs: 10.1}},
		},
		CausalImpacts: []WakeupCausalImpact{{Thread: ThreadRef{Comm: "dep", PID: 200}, ChainDepth: 1, TotalMs: 5}},
	}
	items := []RootCauseRankItem{
		{Type: "cpu_pressure", SubjectKind: RootCauseSubjectKindAggregateMetric, ImpactMs: 12, StartTs: 10.01, EndTs: 10.09, Confidence: 0.72},
		{Type: "io_pressure", ImpactMs: 8, StartTs: 10.01, EndTs: 10.09, Confidence: 0.70},
	}
	items = enrichRootCauseItemsWithChainContext(chain, items)
	if items[0].ChainRelevance != "background" {
		t.Fatalf("AggregateOnly row must stay background despite window overlap (恒 background), got %q", items[0].ChainRelevance)
	}
	if items[1].ChainRelevance != "adjacent" {
		t.Fatalf("Subject==either row keeps the legacy adjacent behavior (precise-signal contrast), got %q", items[1].ChainRelevance)
	}
}

// Pin ⑥a (§20 B/D-Gap④, state_churn precedent): the RunnableTop inversion
// retype re-derives Score with the retyped weight — the stale runnable_wait
// Score (weight 1.15) must not survive the retype to 1.35.
func TestRunnableTopInversionRetypeRecomputesScore(t *testing.T) {
	idx := buildTraceIndex(t, "rkc_retype.systrace", `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=R ==> next_comm=bg next_pid=300 next_prio=20
        bg-300 (300) [001] .... 5.030000: sched_switch: prev_comm=bg prev_pid=300 prev_prio=20 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	rank := BuildRootCauseRank(idx, Query{
		PID: 100, TimeStart: 5.0, TimeEnd: 5.03, Limit: 16, MinDurationMs: 0.001,
		TraceFlavorHint: TraceFlavorHarmonyHitrace,
	})
	var item *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Thread.PID == 100 && rank.Items[i].Type == "priority_inversion_runnable_wait" {
			item = &rank.Items[i]
			break
		}
	}
	if item == nil {
		t.Fatalf("physical scheduler endpoints must retype the target runnable seat: %+v", rank.Items)
	}
	want := item.EffectiveImpactMs * item.Confidence * rootCauseItemScoreWeight(*item)
	if !rkcFloatEq(item.Score, want) {
		t.Fatalf("retype must re-derive Score from proven impact with the inversion weight, got %f want %f", item.Score, want)
	}
	staleScore := item.EffectiveImpactMs * item.Confidence * rootCauseTypeWeight("runnable_wait")
	if rkcFloatEq(item.Score, staleScore) {
		t.Fatalf("mutation red: retyped row kept the stale runnable_wait Score %f", staleScore)
	}
}

// Pin ⑥b (§20.1 ruling ②): a RESOLVED lock-contention rank row weighs 1.35
// (priority_inversion family — decisive evidence class); unresolved keeps the
// 0.8 default. Score-only ordering: a measured resolved lock hold never
// scores below a generic trace_span of the same magnitude.
func TestResolvedBlockingSpanScoreWeightRuling(t *testing.T) {
	resolved := RootCauseRankItem{Type: "blocking_span", BlockingKind: "monitor_contention", BlockingPeer: ThreadRef{Comm: "waiter", PID: 400}}
	if got := rootCauseItemScoreWeight(resolved); !rkcFloatEq(got, 1.35) {
		t.Fatalf("resolved blocking_span weight must be 1.35 (§20.1 ruling ②), got %f", got)
	}
	unresolved := RootCauseRankItem{Type: "blocking_span"}
	if got := rootCauseItemScoreWeight(unresolved); !rkcFloatEq(got, rootCauseTypeWeight("blocking_span")) {
		t.Fatalf("unresolved blocking_span keeps the type-table default, got %f", got)
	}
	row := blockingSpanRow{
		spanName: "monitor contention with owner holder",
		cand: CriticalBlockingCandidate{
			Type:         "blocking_span",
			Thread:       ThreadRef{Comm: "waiter", PID: 400},
			Peer:         ThreadRef{Comm: "holder", PID: 500},
			BlockingKind: "monitor_contention",
			DurationMs:   10,
			Confidence:   0.72,
			LineStart:    10,
			LineEnd:      20,
		},
	}
	lockItem := rootCauseItemFromLockContentionCandidate(Query{}, map[int]bool{}, false, row)
	genericSpan := rootCauseItem("trace_span", ThreadRef{Comm: "other", PID: 600}, 10, lockItem.Confidence, 30, 40, "window_stats", "trace span")
	if lockItem.Score <= genericSpan.Score {
		t.Fatalf("Score-only 形态: resolved lock row (%f) must not score below a generic trace_span (%f)", lockItem.Score, genericSpan.Score)
	}
	if want := lockItem.ImpactMs * lockItem.Confidence * 1.35; !rkcFloatEq(lockItem.Score, want) {
		t.Fatalf("resolved lock row Score must use weight 1.35, got %f want %f", lockItem.Score, want)
	}
}

// Pin ⑦ (§20 E-Gap⑥): binder_wait and io_latency rank rows carry the RCX①
// drill verdict for their counterpart (binder peer / IO completer), symmetric
// with the critical_blocking face. Three verdicts covered: drilled (peer is a
// chain-examined thread with a scheduled timeline), undrilled_peer_known
// (resolved but never examined), peer_unknown (unresolved binder peer).
func TestBinderAndIOLatencyRankRowsCarryDrillStamp(t *testing.T) {
	idx := buildTraceIndex(t, "rkc_drill.systrace", `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        peer-500 (500) [002] .... 5.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=peer next_pid=500 next_prio=98
        peer-500 (500) [002] .... 5.004000: sched_switch: prev_comm=peer prev_pid=500 prev_prio=98 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
	`)
	chain := ChainResult{
		Target: ThreadRef{Comm: "app", PID: 100},
		Nodes: []ChainNode{
			{ID: "n1", Thread: ThreadRef{Comm: "app", PID: 100}, Window: TimeWindow{StartTs: 5.0, EndTs: 5.01}},
			{ID: "n2", Thread: ThreadRef{Comm: "peer", PID: 500}, Window: TimeWindow{StartTs: 5.0, EndTs: 5.005}},
		},
		BinderWaits: []BinderWaitSummary{
			{Thread: ThreadRef{Comm: "app", PID: 100}, Peer: ThreadRef{Comm: "peer", PID: 500}, SendLine: 10, SleepLine: 11, ReceiveLine: 15, WakeupLine: 20, DurationMs: 5, Confidence: 0.8},
			{Thread: ThreadRef{Comm: "app", PID: 100}, SendLine: 50, SleepLine: 51, WakeupLine: 60, DurationMs: 2, Confidence: 0.6},
		},
		RootEvidence: []RootEvidence{
			{Type: "binder_wait", Thread: ThreadRef{Comm: "app", PID: 100}, DurationMs: 5, LineStart: 10, LineEnd: 20, Summary: "binder wait resolved peer", Confidence: 0.8},
			{Type: "binder_wait", Thread: ThreadRef{Comm: "app", PID: 100}, DurationMs: 2, LineStart: 50, LineEnd: 60, Summary: "binder wait unresolved peer", Confidence: 0.6},
		},
	}
	stats := WindowStats{IOLatencies: []IOLatencySummary{{
		Dev: "sda", Op: "R", IssueThread: ThreadRef{Comm: "worker", PID: 200},
		CompleteThread: ThreadRef{Comm: "kworker", PID: 600},
		IssueLine:      30, CompleteLine: 40, DurationMs: 3,
	}}}
	rank := buildRootCauseRankFrom(idx, Query{TimeStart: 5.0, TimeEnd: 5.01}, chain, stats)
	verdicts := map[string]string{}
	for _, item := range rank.Items {
		switch {
		case item.Type == "binder_wait" && item.LineStart == 10:
			verdicts["binder_resolved"] = item.DrillStatus
		case item.Type == "binder_wait" && item.LineStart == 50:
			verdicts["binder_unresolved"] = item.DrillStatus
		case item.Type == "io_latency":
			verdicts["io"] = item.DrillStatus
		}
	}
	if verdicts["binder_resolved"] != DrillStatusDrilled {
		t.Fatalf("binder peer 500 is a chain-examined thread with a scheduled timeline — want drilled, got %q (all=%v)", verdicts["binder_resolved"], verdicts)
	}
	if verdicts["binder_unresolved"] != DrillStatusPeerUnknown {
		t.Fatalf("unresolved binder peer must stamp peer_unknown, got %q", verdicts["binder_unresolved"])
	}
	if verdicts["io"] != DrillStatusUndrilledPeerKnown {
		t.Fatalf("IO completer 600 is resolved but never examined — want undrilled_peer_known, got %q", verdicts["io"])
	}
}

func rkcFloatEq(a, b float64) bool {
	diff := a - b
	return diff < 1e-6 && diff > -1e-6
}
