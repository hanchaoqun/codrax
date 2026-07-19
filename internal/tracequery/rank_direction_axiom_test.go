package tracequery

// rank_direction_axiom_test.go — AXIOM-V2 批 pins (user rulings 2026-07-18):
//
//   pin① fix-direction census: every registry token declares EXACTLY one
//        direction from the closed set (每 token 恰一方向或 unresolved), both
//        key sets equal, anchor rows pinned (the E5/E26 witness directions).
//   pin③ pair table identity: overlap ≤ min(two support unions); symmetric
//        wire entries; population exclusions (◇/count/rank-0/unresolved/
//        cross-thread/envelope-level) publish nothing.
//   pin④ conservation checker: the violating form stamps the typed finding +
//        caveat on every member seat while EMISSION PROCEEDS and every value
//        channel stays byte-identical (永不硬拦); the compliant form
//        publishes zero disclosure bytes (负臂).
//   witness shape: the cust_span_runnable E26(类校验, self_workload 向) ×
//        E5(running supply-fold, frequency_thermal 向) same-thread pair mints
//        the symmetric cross-direction entries (件2 验收 witness的引擎半).

import (
	"strings"
	"testing"
)

func TestCausalTokenFixDirectionCensus(t *testing.T) {
	closed := map[CausalTokenFixDirection]bool{
		CausalFixDirectionSchedulingSupply: true,
		CausalFixDirectionLockPriority:     true,
		CausalFixDirectionIODependency:     true,
		CausalFixDirectionMemory:           true,
		CausalFixDirectionFrequencyThermal: true,
		CausalFixDirectionSelfWorkload:     true,
		CausalFixDirectionUnresolved:       true,
	}
	// 每 token 恰一方向或 unresolved — both key sets equal, values closed.
	for _, token := range CausalTokenUniverse() {
		direction, ok := causalTokenFixDirections[token]
		if !ok {
			t.Errorf("registry token %q has no fix-direction declaration (件1: 每 token 恰一方向或 unresolved)", token)
			continue
		}
		if !closed[direction] {
			t.Errorf("token %q declares direction %q outside the closed set", token, direction)
		}
	}
	universe := map[string]bool{}
	for _, token := range CausalTokenUniverse() {
		universe[token] = true
	}
	for token := range causalTokenFixDirections {
		if !universe[token] {
			t.Errorf("fix-direction declaration %q names a token absent from the registry (ghost row)", token)
		}
	}
	// Anchor rows (the adjudicated defaults — see the declaration header):
	anchors := map[string]CausalTokenFixDirection{
		"runnable_wait":                    CausalFixDirectionSchedulingSupply,
		"scheduler_latency":                CausalFixDirectionSchedulingSupply,
		"priority_inversion_runnable_wait": CausalFixDirectionLockPriority,
		"blocking_span":                    CausalFixDirectionLockPriority,
		"sleep_wait":                       CausalFixDirectionIODependency,
		"binder_wait":                      CausalFixDirectionIODependency,
		"d_state_or_io_wait":               CausalFixDirectionIODependency,
		"gc_pause":                         CausalFixDirectionMemory, // 与 memory_gc 同向防假跨向对
		"memory_gc":                        CausalFixDirectionMemory,
		"running":                          CausalFixDirectionFrequencyThermal, // E5 witness: eff=供给折算缺口
		"low_frequency":                    CausalFixDirectionFrequencyThermal,
		"class_verification":               CausalFixDirectionSelfWorkload, // E26 witness
		"jit_compile":                      CausalFixDirectionSelfWorkload,
		"pacing_idle":                      CausalFixDirectionUnresolved,
		"missing_wakeup":                   CausalFixDirectionUnresolved, // 无边不认向
		"trace_gap":                        CausalFixDirectionUnresolved,
	}
	for token, want := range anchors {
		if got := CausalTokenFixDirectionFor(token); got != want {
			t.Errorf("anchor token %q: direction %q, want %q", token, got, want)
		}
	}
	// Unregistered tokens fail open to unresolved (never guess).
	if got := CausalTokenFixDirectionFor("no_such_token"); got != CausalFixDirectionUnresolved {
		t.Errorf("unregistered token must resolve unresolved, got %q", got)
	}
}

// axiomv2SelfRunningSeat mimics the cust_span_runnable E5 shape: the target's
// running supply-fold deficit seat (frequency_thermal direction) with a typed
// running-interval inventory.
func axiomv2SelfRunningSeat(rank int) RootCauseRankItem {
	return RootCauseRankItem{
		Rank: rank, Tier: "tertiary", Type: "running",
		Thread:   ThreadRef{Comm: "ease.cloudmusic", PID: 63993},
		ImpactMs: 107.084, ProjectedImpactMs: 107.084, CumulativeImpactMs: 107.084,
		EffectiveImpactMs: 4.843, Score: 4.2, Confidence: 0.86,
		LineStart: 26424, LineEnd: 61077,
		Source:    "thread_timeline.self_running_fold",
		Causality: RootCauseCausalitySelfWallClock, ChainRelevance: "on_chain",
		OnChainBasis:  RootCauseOnChainBasisSelfWallClockInterval,
		DominantState: "running", RunningMs: 107.084,
		SupplyFoldDeficitMs: 4.843, SupplyFoldIdealMs: 102.241,
		SubjectIsAnalysisTarget: true,
		selfGapRunningIntervals: []foldInterval{
			{start: 17729.480, end: 17729.520}, // 40ms
			{start: 17729.540, end: 17729.600}, // 60ms
		},
	}
}

// axiomv2SemanticFamilySeat mimics the E26 shape: the target's own semantic
// class_verification family seat (self_workload direction) with the complete
// typed member interval inventory.
func axiomv2SemanticFamilySeat(rank int) RootCauseRankItem {
	return RootCauseRankItem{
		Rank: rank, Tier: "primary", Type: "class_verification",
		Thread:   ThreadRef{Comm: "ease.cloudmusic", PID: 63993},
		ImpactMs: 9.586, ProjectedImpactMs: 9.586, CumulativeImpactMs: 9.586,
		EffectiveImpactMs: 9.586, Score: 8.1, Confidence: 0.82,
		LineStart: 40424, LineEnd: 40925,
		Source:    "trace_semantic_span",
		Causality: RootCauseCausalitySelfDeterministic, ChainRelevance: "on_chain",
		OnChainBasis:  RootCauseOnChainBasisSelfDeterministicSpan,
		SemanticClass: "class_verification", MemberCount: 2,
		semanticMemberIntervals: []foldInterval{
			{start: 17729.510, end: 17729.515},  // 5ms, 全部落在 running 区间内
			{start: 17729.550, end: 17729.5546}, // 4.6ms ⊂ running
		},
	}
}

func axiomv2Rank(items ...RootCauseRankItem) RootCauseRankResult {
	return RootCauseRankResult{
		Target: ThreadRef{Comm: "ease.cloudmusic", PID: 63993},
		Window: TimeWindow{StartTs: 17729.471126, EndTs: 17729.622508},
		Items:  items,
	}
}

// The witness shape (件2 验收 witness 引擎半): E26 工作量向 × E5 供给向 —
// symmetric entries, exact overlap, overlap ≤ min(两席支撑 union) identity.
func TestAXIOMV2CrossDirectionWitnessPairStamps(t *testing.T) {
	rank := axiomv2Rank(axiomv2SemanticFamilySeat(1), axiomv2SelfRunningSeat(4))
	valuesBefore := []float64{rank.Items[0].EffectiveImpactMs, rank.Items[1].EffectiveImpactMs,
		rank.Items[0].ImpactMs, rank.Items[1].ImpactMs}
	stampRootCauseFixDirections(&rank)
	stampCrossDirectionDisclosureAndConservation(&rank)
	sem, run := rank.Items[0], rank.Items[1]
	if sem.FixDirection != "self_workload" || run.FixDirection != "frequency_thermal" {
		t.Fatalf("件1 wire transit: directions %q/%q, want self_workload/frequency_thermal", sem.FixDirection, run.FixDirection)
	}
	if len(sem.CrossDirectionOverlaps) != 1 || len(run.CrossDirectionOverlaps) != 1 {
		t.Fatalf("件2: exactly one symmetric entry per side, got %d/%d", len(sem.CrossDirectionOverlaps), len(run.CrossDirectionOverlaps))
	}
	a, b := sem.CrossDirectionOverlaps[0], run.CrossDirectionOverlaps[0]
	// The semantic spans lie entirely inside the running union → overlap ==
	// the span union (5ms + 4.6ms = 9.6ms).
	wantOverlap := 9.6
	if diff := a.OverlapMs - wantOverlap; diff > 0.001 || diff < -0.001 {
		t.Fatalf("件2 overlap: got %.3f want %.3f", a.OverlapMs, wantOverlap)
	}
	if a.OverlapMs != b.OverlapMs {
		t.Fatalf("件2 symmetric overlap values diverged: %.3f vs %.3f", a.OverlapMs, b.OverlapMs)
	}
	// 三问 pin: overlap ≤ min(两席支撑区间 union) — running union 100ms,
	// span union 9.6ms.
	if a.OverlapMs > 9.6+0.001 {
		t.Fatalf("件2 identity: overlap %.3f exceeds the smaller support union", a.OverlapMs)
	}
	// Mutual pointers: each entry carries the PARTNER's envelope/direction/basis.
	if a.LineStart != 26424 || a.LineEnd != 61077 || a.Direction != "frequency_thermal" || a.Basis != RootCauseDirectionBasisSelfRunning {
		t.Fatalf("件2 entry on the semantic seat must describe the running partner: %+v", a)
	}
	if b.LineStart != 40424 || b.LineEnd != 40925 || b.Direction != "self_workload" || b.Basis != RootCauseDirectionBasisSemanticMembers {
		t.Fatalf("件2 entry on the running seat must describe the semantic partner: %+v", b)
	}
	// 值零动 (three-question / 主值零动 pin).
	valuesAfter := []float64{sem.EffectiveImpactMs, run.EffectiveImpactMs, sem.ImpactMs, run.ImpactMs}
	for i := range valuesBefore {
		if valuesBefore[i] != valuesAfter[i] {
			t.Fatalf("值通道零动 violated at %d: %.3f → %.3f", i, valuesBefore[i], valuesAfter[i])
		}
	}
	// Cross-direction pairs never mint the conservation finding (different keys).
	if sem.DirectionConservationExcess != nil || run.DirectionConservationExcess != nil {
		t.Fatalf("cross-direction pair must not mint a same-direction conservation finding")
	}
}

// Population exclusions (「链上」硬纪律): each excluded shape publishes NOTHING.
func TestAXIOMV2PopulationExclusions(t *testing.T) {
	base := func() (RootCauseRankItem, RootCauseRankItem) {
		return axiomv2SemanticFamilySeat(1), axiomv2SelfRunningSeat(4)
	}
	cases := []struct {
		name   string
		mutate func(sem, run *RootCauseRankItem)
	}{
		{"rank0 豁免席", func(sem, run *RootCauseRankItem) { run.Rank = 0 }},
		{"gated 归零", func(sem, run *RootCauseRankItem) { run.EffectiveImpactMs = 0 }},
		{"◇ credential demoted", func(sem, run *RootCauseRankItem) { run.ChainCredentialLaneDemoted = true }},
		{"◇ remainder seat", func(sem, run *RootCauseRankItem) { run.ChainAnchorRemainderSeat = true }},
		// 修补轮 件3 (②): the 14th excluded shape — the fully-anchored
		// satellite whose share is represented by the chain-lane seat (XLANE-1
		// ◇ lane) never re-enters as a full seat.
		{"◇ represented by chain seat", func(sem, run *RootCauseRankItem) { run.ChainAnchorRepresentedByChainSeat = true }},
		{"gated-share constituent", func(sem, run *RootCauseRankItem) { run.GatedShareConstituentSeat = true }},
		{"absorbed row", func(sem, run *RootCauseRankItem) { run.AbsorbedByRankFamily = true }},
		{"off-chain", func(sem, run *RootCauseRankItem) { run.ChainRelevance = "adjacent"; run.Causality = "" }},
		{"unknown basis", func(sem, run *RootCauseRankItem) { run.OnChainBasis = "made_up_basis" }},
		{"cross-thread", func(sem, run *RootCauseRankItem) { run.Thread.PID = 999 }},
		{"no thread identity", func(sem, run *RootCauseRankItem) { run.Thread.PID = 0 }},
		{"count caliber", func(sem, run *RootCauseRankItem) { run.Type = "page_cache_churn" }},
		{"unresolved direction", func(sem, run *RootCauseRankItem) { run.Type = "state_churn" }},
		{"envelope-level (no inventory)", func(sem, run *RootCauseRankItem) {
			run.selfGapRunningIntervals = nil // fragmented row: ImpactMs ≠ envelope → no basis
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sem, run := base()
			tc.mutate(&sem, &run)
			rank := axiomv2Rank(sem, run)
			stampRootCauseFixDirections(&rank)
			stampCrossDirectionDisclosureAndConservation(&rank)
			for i := range rank.Items {
				if len(rank.Items[i].CrossDirectionOverlaps) != 0 {
					t.Fatalf("%s: excluded shape minted a pair entry on item %d", tc.name, i)
				}
			}
		})
	}
}

// 件2/件3 closure: a genuine pair whose partner lacks a line envelope reports
// through the type-token lane (no fake [E#]) + caveat.
func TestAXIOMV2UndisclosedPairFallsToTypeTokenLane(t *testing.T) {
	sem, run := axiomv2SemanticFamilySeat(1), axiomv2SelfRunningSeat(4)
	run.LineStart, run.LineEnd = 0, 0 // carrier absent
	rank := axiomv2Rank(sem, run)
	stampRootCauseFixDirections(&rank)
	stampCrossDirectionDisclosureAndConservation(&rank)
	if len(rank.Items[0].CrossDirectionOverlaps) != 0 || len(rank.Items[1].CrossDirectionOverlaps) != 0 {
		t.Fatalf("宁漏勿假指: a pointer-less pair must mint no [E#] entries")
	}
	if got := rank.Items[0].CrossDirectionOverlapUndisclosed; len(got) != 1 || got[0] != "running" {
		t.Fatalf("undisclosed lane on the semantic seat: %v", got)
	}
	if got := rank.Items[1].CrossDirectionOverlapUndisclosed; len(got) != 1 || got[0] != "class_verification" {
		t.Fatalf("undisclosed lane on the running seat: %v", got)
	}
	found := false
	for _, caveat := range rank.Caveats {
		// 件8: the carrier-absent caveat wears the cross_direction_overlap:
		// family prefix — never direction_conservation: (件3's family).
		if strings.HasPrefix(caveat, "cross_direction_overlap: overlap") {
			found = true
		}
		if strings.Contains(caveat, "direction_conservation:") {
			t.Fatalf("件8 词面卫生: carrier-absent caveat must not wear the 件3 family prefix: %q", caveat)
		}
	}
	if !found {
		t.Fatalf("立案素材 caveat missing: %v", rank.Caveats)
	}
}

// pin④: the violating same-direction form stamps the finding on every member
// seat + the caveat, EMISSION PROCEEDS, values untouched; the compliant form
// publishes zero disclosure bytes.
func TestAXIOMV2DirectionConservationChecker(t *testing.T) {
	// Two same-thread self_workload semantic seats whose span inventories
	// overlap heavily: union Σ = 9.6 + 9.6 > window? Window is 151ms — use a
	// SMALL window so Σ exceeds it: seats of 100ms + 60ms unions on a 151ms
	// window → Σ 160ms > 151.382ms (违宪形: nested spans double-billed).
	seatA := axiomv2SemanticFamilySeat(1)
	seatA.semanticMemberIntervals = []foldInterval{{start: 17729.480, end: 17729.580}} // 100ms
	seatA.LineStart, seatA.LineEnd = 40424, 40925
	seatB := axiomv2SemanticFamilySeat(2)
	seatB.Type = "jit_compile" // same direction (self_workload), different token
	seatB.SemanticClass = "jit_compile"
	seatB.semanticMemberIntervals = []foldInterval{{start: 17729.500, end: 17729.560}} // 60ms ⊂ A
	seatB.LineStart, seatB.LineEnd = 50000, 50100
	rank := axiomv2Rank(seatA, seatB)
	effBefore := []float64{rank.Items[0].EffectiveImpactMs, rank.Items[1].EffectiveImpactMs}
	stampRootCauseFixDirections(&rank)
	stampCrossDirectionDisclosureAndConservation(&rank)
	for i := range rank.Items {
		finding := rank.Items[i].DirectionConservationExcess
		if finding == nil {
			t.Fatalf("违宪形: member seat %d must carry the finding", i)
		}
		if finding.Direction != "self_workload" || finding.SeatCount != 2 {
			t.Fatalf("finding content: %+v", finding)
		}
		if diff := finding.SumMs - 160.0; diff > 0.001 || diff < -0.001 {
			t.Fatalf("finding Σ: got %.3f want 160.000", finding.SumMs)
		}
		if diff := finding.WindowMs - 151.382; diff > 0.001 || diff < -0.001 {
			t.Fatalf("finding window: got %.3f want 151.382", finding.WindowMs)
		}
	}
	found := false
	for _, caveat := range rank.Caveats {
		if strings.Contains(caveat, "direction_conservation: thread") &&
			strings.Contains(caveat, "self_workload") {
			found = true
		}
	}
	if !found {
		t.Fatalf("立案素材 caveat missing: %v", rank.Caveats)
	}
	// 永不硬拦 + 值零动: the pass returned normally, seats keep competing
	// values and ordinals.
	if rank.Items[0].Rank != 1 || rank.Items[1].Rank != 2 {
		t.Fatalf("ordinals must stay untouched")
	}
	for i := range effBefore {
		if rank.Items[i].EffectiveImpactMs != effBefore[i] {
			t.Fatalf("value channel changed on seat %d", i)
		}
	}
	// Same-direction seats never mint cross-direction pair entries.
	if len(rank.Items[0].CrossDirectionOverlaps) != 0 {
		t.Fatalf("same-direction overlap must not enter the 件2 pair lane")
	}

	// 负臂 (合规形): disjoint supports summing under the window → zero bytes.
	seatB2 := seatB
	seatB2.semanticMemberIntervals = []foldInterval{{start: 17729.590, end: 17729.600}} // 10ms disjoint
	clean := axiomv2Rank(seatA, seatB2)
	stampRootCauseFixDirections(&clean)
	stampCrossDirectionDisclosureAndConservation(&clean)
	for i := range clean.Items {
		if clean.Items[i].DirectionConservationExcess != nil {
			t.Fatalf("合规形 seat %d must publish no finding", i)
		}
	}
	for _, caveat := range clean.Caveats {
		if strings.Contains(caveat, "direction_conservation") {
			t.Fatalf("合规形 must append no caveat: %q", caveat)
		}
	}
}

// ── 修补轮 件1 (P1): occurrence_windows µs-identity 守卫 ──────────────────────

// axiomv2RunnableOccurrenceSeat mimics an aggregated on-chain runnable seat
// (legacy OnChainBasis "" lane) whose only support inventory is its
// occurrence-window list.
func axiomv2RunnableOccurrenceSeat(rank int) RootCauseRankItem {
	return RootCauseRankItem{
		Rank: rank, Tier: "secondary", Type: "runnable_wait",
		Thread:   ThreadRef{Comm: "ease.cloudmusic", PID: 63993},
		ImpactMs: 8.282, ProjectedImpactMs: 8.282, CumulativeImpactMs: 8.282,
		EffectiveImpactMs: 8.282, Score: 2.0, Confidence: 0.8,
		LineStart: 30000, LineEnd: 30500,
		Source:    "wakeup_chain.aggregated_impacts",
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainDepth: 1,
		DominantState: "runnable", RunnableMs: 8.282,
		StartTs: 17729.480, EndTs: 17729.488632,
	}
}

// axiomv2MixedOccurrence is the tieba 59566 live geometry (synthesized): one
// occurrence window of 8.632ms filled by a STATE MIXTURE — run 0.185 +
// runnable 8.282 + sleep 0.113. Its envelope is a node-level hull, never a
// segment (DominantImpactMs 8.282 ≠ window 8.632).
func axiomv2MixedOccurrence() WakeupCausalOccurrence {
	return WakeupCausalOccurrence{
		Window:           TimeWindow{StartTs: 17729.480, EndTs: 17729.488632},
		DominantState:    "runnable",
		DominantImpactMs: 8.282,
		RunningMs:        0.185, RunnableMs: 8.282, SleepMs: 0.113,
		FragmentCount: 3, StateSwitches: 4,
	}
}

// axiomv2PureOccurrence is the single-state-filled shape: the dominant state
// fills the window to the µs (DominantImpactMs == window length) — the window
// IS its own segment.
func axiomv2PureOccurrence() WakeupCausalOccurrence {
	return WakeupCausalOccurrence{
		Window:           TimeWindow{StartTs: 17729.480, EndTs: 17729.488632},
		DominantState:    "runnable",
		DominantImpactMs: 8.632,
		RunnableMs:       8.632,
	}
}

// 件1 basis-level pin: the µs-identity guard is PER WINDOW — a state-mixture
// window never enters the support closed set; a single-state-filled window
// does; a seat whose every window is mixed resolves NO basis (包络级凭证行
// 出圈 — the union of such hulls exceeded the seat's eff on the tieba live
// board: 21.42 > 20.15).
func TestAXIOMV2OccurrenceWindowMixtureGuardBasis(t *testing.T) {
	mixed := axiomv2RunnableOccurrenceSeat(2)
	mixed.OccurrenceWindows = []WakeupCausalOccurrence{axiomv2MixedOccurrence()}
	if intervals, basis := rootCauseItemDirectionSupport(&mixed); basis != "" || intervals != nil {
		t.Fatalf("州混合窗 must resolve no basis (hull ≠ segment), got basis=%q n=%d", basis, len(intervals))
	}

	pure := axiomv2RunnableOccurrenceSeat(2)
	pure.OccurrenceWindows = []WakeupCausalOccurrence{axiomv2PureOccurrence()}
	intervals, basis := rootCauseItemDirectionSupport(&pure)
	if basis != RootCauseDirectionBasisOccurrences || len(intervals) != 1 {
		t.Fatalf("单态填满窗 must resolve as its own segment: basis=%q n=%d", basis, len(intervals))
	}

	// Per-window admission: mixed + pure in one inventory → only the pure
	// window enters (宁漏勿假指: the mixed hull is dropped, the proven
	// segment is kept).
	both := axiomv2RunnableOccurrenceSeat(2)
	pureSmall := WakeupCausalOccurrence{
		Window:           TimeWindow{StartTs: 17729.520, EndTs: 17729.522},
		DominantState:    "runnable",
		DominantImpactMs: 2.0, RunnableMs: 2.0,
	}
	both.OccurrenceWindows = []WakeupCausalOccurrence{axiomv2MixedOccurrence(), pureSmall}
	intervals, basis = rootCauseItemDirectionSupport(&both)
	if basis != RootCauseDirectionBasisOccurrences || len(intervals) != 1 {
		t.Fatalf("per-window guard: exactly the pure window must survive, got basis=%q n=%d", basis, len(intervals))
	}
	if intervals[0].start != 17729.520 || intervals[0].end != 17729.522 {
		t.Fatalf("per-window guard admitted the wrong window: %+v", intervals[0])
	}
}

// 件1 population-level pin (四板对照的引擎半): the all-mixed occurrence seat
// steps OUT of the 件2/件3 population entirely — no pair entry, no
// undisclosed token, no conservation finding on either side; the
// single-state-filled twin stays IN and mints the symmetric pair.
func TestAXIOMV2OccurrenceMixtureStepsOutOfPopulation(t *testing.T) {
	partner := func() RootCauseRankItem {
		sem := axiomv2SemanticFamilySeat(1)
		// Overlap the occurrence envelope: 2ms inside 17729.480..488632.
		sem.semanticMemberIntervals = []foldInterval{{start: 17729.482, end: 17729.484}}
		return sem
	}

	mixedSeat := axiomv2RunnableOccurrenceSeat(2)
	mixedSeat.OccurrenceWindows = []WakeupCausalOccurrence{axiomv2MixedOccurrence()}
	rank := axiomv2Rank(partner(), mixedSeat)
	stampRootCauseFixDirections(&rank)
	stampCrossDirectionDisclosureAndConservation(&rank)
	for i := range rank.Items {
		if len(rank.Items[i].CrossDirectionOverlaps) != 0 ||
			len(rank.Items[i].CrossDirectionOverlapUndisclosed) != 0 ||
			rank.Items[i].DirectionConservationExcess != nil {
			t.Fatalf("州混合形 seat %d must publish nothing (out of the population): %+v", i, rank.Items[i])
		}
	}
	if rank.Items[1].directionSupportBasis != "" {
		t.Fatalf("州混合形 must carry no support basis, got %q", rank.Items[1].directionSupportBasis)
	}

	pureSeat := axiomv2RunnableOccurrenceSeat(2)
	pureSeat.OccurrenceWindows = []WakeupCausalOccurrence{axiomv2PureOccurrence()}
	clean := axiomv2Rank(partner(), pureSeat)
	stampRootCauseFixDirections(&clean)
	stampCrossDirectionDisclosureAndConservation(&clean)
	if len(clean.Items[0].CrossDirectionOverlaps) != 1 || len(clean.Items[1].CrossDirectionOverlaps) != 1 {
		t.Fatalf("单态填满形 must mint the symmetric pair: %d/%d",
			len(clean.Items[0].CrossDirectionOverlaps), len(clean.Items[1].CrossDirectionOverlaps))
	}
	entry := clean.Items[0].CrossDirectionOverlaps[0]
	if diff := entry.OverlapMs - 2.0; diff > 0.001 || diff < -0.001 {
		t.Fatalf("单态填满形 overlap: got %.3f want 2.000", entry.OverlapMs)
	}
	if entry.Basis != RootCauseDirectionBasisOccurrences {
		t.Fatalf("partner entry must carry the occurrence basis, got %q", entry.Basis)
	}
}

// ── 修补轮 件2 (①): PID<=0 臂负例 ────────────────────────────────────────────

// Two identity-less seats (PID==0) with same-direction supports summing far
// past the window must NEVER group for conservation — the axiom keys on
// thread identity, and pid 0 is not an identity (剥 PID 臂 → the two seats
// would collide under key {0, self_workload} and mint a finding).
func TestAXIOMV2PidlessSeatsNeverGroupForConservation(t *testing.T) {
	seatA := axiomv2SemanticFamilySeat(1)
	seatA.Thread = ThreadRef{Comm: "<unknown>", PID: 0}
	seatA.semanticMemberIntervals = []foldInterval{{start: 17729.480, end: 17729.580}} // 100ms
	seatB := axiomv2SemanticFamilySeat(2)
	seatB.Type = "jit_compile"
	seatB.SemanticClass = "jit_compile"
	seatB.Thread = ThreadRef{Comm: "<unknown>", PID: 0}
	seatB.semanticMemberIntervals = []foldInterval{{start: 17729.490, end: 17729.590}} // 100ms, Σ 200 > 151.382
	rank := axiomv2Rank(seatA, seatB)
	stampRootCauseFixDirections(&rank)
	stampCrossDirectionDisclosureAndConservation(&rank)
	for i := range rank.Items {
		if rank.Items[i].DirectionConservationExcess != nil {
			t.Fatalf("PID==0 seat %d grouped for conservation (no identity, no claim)", i)
		}
		if len(rank.Items[i].CrossDirectionOverlaps) != 0 || len(rank.Items[i].CrossDirectionOverlapUndisclosed) != 0 {
			t.Fatalf("PID==0 seat %d entered the pair lane", i)
		}
	}
	for _, caveat := range rank.Caveats {
		if strings.Contains(caveat, "direction_conservation") || strings.Contains(caveat, "cross_direction_overlap") {
			t.Fatalf("PID==0 population must append no audit caveat: %q", caveat)
		}
	}
}

// ── 修补轮 件4 (③): cap6 对称行为 (8席扇形) ─────────────────────────────────

// One running seat overlaps SEVEN disjoint semantic partners (overlaps 7ms
// down to 1ms). The bounded roster keeps the top 6 pairs; the 7th pair drops
// on BOTH sides (both-or-neither wire symmetry) and reports through the
// undisclosed type-token lane on BOTH sides. 单边化突变 (capacity checked on
// one side only) → the hub publishes 7 or the dropped partner publishes 1 →
// red.
func TestAXIOMV2PartnerCapDropsPairOnBothSides(t *testing.T) {
	hub := axiomv2SelfRunningSeat(1)
	hub.selfGapRunningIntervals = []foldInterval{{start: 17729.480, end: 17729.580}} // 100ms
	items := []RootCauseRankItem{hub}
	for k := 1; k <= 7; k++ {
		partner := axiomv2SemanticFamilySeat(1 + k)
		partner.Type = "jit_compile"
		partner.SemanticClass = "jit_compile"
		partner.MemberCount = 1
		partner.semanticMemberIntervals = nil
		// Partner k: a (8−k)ms span at 480ms+k·10ms — disjoint from each
		// other, nested inside the hub union → overlap = own length.
		partner.StartTs = 17729.480 + float64(k)*0.010
		partner.EndTs = partner.StartTs + float64(8-k)*0.001
		partner.LineStart = 50000 + k*100
		partner.LineEnd = partner.LineStart + 10
		items = append(items, partner)
	}
	rank := axiomv2Rank(items...)
	stampRootCauseFixDirections(&rank)
	stampCrossDirectionDisclosureAndConservation(&rank)
	hubOut := rank.Items[0]
	if len(hubOut.CrossDirectionOverlaps) != RootCauseCrossDirectionOverlapPartnerCap {
		t.Fatalf("hub roster must stop at the cap: got %d want %d",
			len(hubOut.CrossDirectionOverlaps), RootCauseCrossDirectionOverlapPartnerCap)
	}
	for k := 1; k <= 6; k++ {
		if got := len(rank.Items[k].CrossDirectionOverlaps); got != 1 {
			t.Fatalf("disclosed partner %d must carry exactly one entry, got %d", k, got)
		}
		if got := len(rank.Items[k].CrossDirectionOverlapUndisclosed); got != 0 {
			t.Fatalf("disclosed partner %d must not ride the undisclosed lane", k)
		}
	}
	// The smallest-overlap pair (partner 7, 1ms) dropped on BOTH sides.
	dropped := rank.Items[7]
	if len(dropped.CrossDirectionOverlaps) != 0 {
		t.Fatalf("both-or-neither: the over-cap pair minted a one-sided entry on the partner: %+v",
			dropped.CrossDirectionOverlaps)
	}
	if got := dropped.CrossDirectionOverlapUndisclosed; len(got) != 1 || got[0] != "running" {
		t.Fatalf("over-cap partner must report the hub by type token: %v", got)
	}
	if got := hubOut.CrossDirectionOverlapUndisclosed; len(got) != 1 || got[0] != "jit_compile" {
		t.Fatalf("hub must report the dropped partner by type token: %v", got)
	}
	found := false
	for _, caveat := range rank.Caveats {
		if strings.HasPrefix(caveat, "cross_direction_overlap: overlap") {
			found = true
		}
	}
	if !found {
		t.Fatalf("roster-capacity drop must leave the 立案素材 caveat: %v", rank.Caveats)
	}
}

// ── 修补轮 件5 (④): 交集是真区间测度,非 min(unions) ─────────────────────────

// Partially interleaved (non-nested) supports distinguish the TRUE interval
// intersection from min(two union lengths): the witness pair's nested shape
// satisfies both formulas (overlap == smaller union), so only a crossing
// shape catches a min() impostor (改 min 突变红).
func TestAXIOMV2IntersectionTrueMeasureNotMinUnion(t *testing.T) {
	// Direct unit arm: [0,10ms] × [5ms,20ms] → ∩ 5ms; min(unions) = 10ms.
	a := []foldInterval{{start: 100.000, end: 100.010}}
	b := []foldInterval{{start: 100.005, end: 100.020}}
	if got := foldIntervalIntersectionMs(a, b); got > 5.0+1e-6 || got < 5.0-1e-6 {
		t.Fatalf("partial crossing ∩: got %.6f want 5.000000 (min-union impostor returns 10)", got)
	}
	// Multi-interval arm: {[0,4],[8,12]} × {[3,9]} → ∩ 1+1 = 2ms;
	// min(unions) = min(8, 6) = 6ms.
	a = []foldInterval{{start: 100.000, end: 100.004}, {start: 100.008, end: 100.012}}
	b = []foldInterval{{start: 100.003, end: 100.009}}
	if got := foldIntervalIntersectionMs(a, b); got > 2.0+1e-6 || got < 2.0-1e-6 {
		t.Fatalf("multi-interval ∩: got %.6f want 2.000000", got)
	}

	// Full-pass arm: running union [480..530ms] × semantic span [500..560ms]
	// (crossing, not nested) → published OverlapMs must be 30ms, never
	// min(50, 60) = 50.
	run := axiomv2SelfRunningSeat(2)
	run.selfGapRunningIntervals = []foldInterval{{start: 17729.480, end: 17729.530}}
	sem := axiomv2SemanticFamilySeat(1)
	sem.semanticMemberIntervals = []foldInterval{{start: 17729.500, end: 17729.560}}
	rank := axiomv2Rank(sem, run)
	stampRootCauseFixDirections(&rank)
	stampCrossDirectionDisclosureAndConservation(&rank)
	if len(rank.Items[0].CrossDirectionOverlaps) != 1 || len(rank.Items[1].CrossDirectionOverlaps) != 1 {
		t.Fatalf("crossing pair must mint symmetric entries")
	}
	got := rank.Items[0].CrossDirectionOverlaps[0].OverlapMs
	if got > 30.0+0.001 || got < 30.0-0.001 {
		t.Fatalf("published overlap: got %.3f want 30.000 (真 ∩, 非 min(unions))", got)
	}
}

// ── 修补轮 件6 (⑤): 窗内裁剪 ────────────────────────────────────────────────

// Support intervals extending past the board window (the actual_* audit
// extent) must be CLIPPED before the conservation Σ: the published SumMs uses
// in-window lengths only, and a seat entirely outside the window leaves the
// population (剥 clip → Σ 充胀 350/3席 → red).
func TestAXIOMV2ConservationClipsToBoardWindow(t *testing.T) {
	seatA := axiomv2SemanticFamilySeat(1)
	// 150ms raw, the first 50ms BEFORE the window start → clips to 100ms.
	seatA.semanticMemberIntervals = []foldInterval{{start: 17729.421126, end: 17729.571126}}
	seatB := axiomv2SemanticFamilySeat(2)
	seatB.Type = "jit_compile"
	seatB.SemanticClass = "jit_compile"
	seatB.LineStart, seatB.LineEnd = 50000, 50100
	seatB.semanticMemberIntervals = []foldInterval{{start: 17729.510, end: 17729.610}} // 100ms in-window
	seatC := axiomv2SemanticFamilySeat(3)
	seatC.Type = "shader_compile"
	seatC.SemanticClass = "shader_compile"
	seatC.LineStart, seatC.LineEnd = 51000, 51100
	seatC.semanticMemberIntervals = []foldInterval{{start: 17729.700, end: 17729.800}} // fully OUT of window
	rank := axiomv2Rank(seatA, seatB, seatC)
	stampRootCauseFixDirections(&rank)
	stampCrossDirectionDisclosureAndConservation(&rank)
	for i := 0; i < 2; i++ {
		finding := rank.Items[i].DirectionConservationExcess
		if finding == nil {
			t.Fatalf("clipped 违宪形: seat %d must carry the finding", i)
		}
		// Σ = 100 (clipped from 150) + 100 = 200 — never the raw 350.
		if diff := finding.SumMs - 200.0; diff > 0.001 || diff < -0.001 {
			t.Fatalf("finding Σ must use in-window lengths: got %.3f want 200.000", finding.SumMs)
		}
		if finding.SeatCount != 2 {
			t.Fatalf("out-of-window seat must not count: SeatCount=%d want 2", finding.SeatCount)
		}
	}
	if rank.Items[2].DirectionConservationExcess != nil {
		t.Fatalf("a seat entirely outside the window must leave the population")
	}
}

// ── 修补轮 件7 (⑥): unresolved stamp 守卫 wire 空值 ──────────────────────────

// A rank containing unresolved-direction tokens passes through the REAL stamp
// and publishes NOTHING for them: FixDirection stays "", which the emit site
// copies verbatim into the zero-dropped fix_direction note — wire 零发射
// (剥守卫 → FixDirection=="unresolved" leaks onto the wire → red). The
// absorbed lane obeys the same guard.
func TestAXIOMV2UnresolvedStampGuardWireEmpty(t *testing.T) {
	churn := axiomv2SelfRunningSeat(3)
	churn.Type = "state_churn" // unresolved direction
	rank := axiomv2Rank(axiomv2SelfRunningSeat(1), churn)
	rank.AbsorbedItems = []RootCauseRankItem{{
		Rank: 0, Tier: RootCauseTierAbsorbed, Type: "pacing_idle", // unresolved
		Thread:   ThreadRef{Comm: "ease.cloudmusic", PID: 63993},
		ImpactMs: 1.0, EffectiveImpactMs: 1.0,
	}}
	stampRootCauseFixDirections(&rank)
	if got := rank.Items[0].FixDirection; got != "frequency_thermal" {
		t.Fatalf("resolved control seat must stamp: got %q", got)
	}
	if got := rank.Items[1].FixDirection; got != "" {
		t.Fatalf("unresolved token must publish NOTHING on the wire field, got %q", got)
	}
	if got := rank.AbsorbedItems[0].FixDirection; got != "" {
		t.Fatalf("absorbed unresolved token must publish NOTHING, got %q", got)
	}
}

// Support-basis resolution: the single-segment µs-identity arm admits an
// unfragmented row (envelope == measured value) and refuses a fragmented one
// (envelope-level rows step out — HULL-CRED).
func TestAXIOMV2SingleSegmentIdentityArm(t *testing.T) {
	item := RootCauseRankItem{
		Type: "blocking_span", ImpactMs: 12.0,
		StartTs: 100.0, EndTs: 100.012, MemberCount: 1,
	}
	intervals, basis := rootCauseItemDirectionSupport(&item)
	if basis != RootCauseDirectionBasisSingleSegment || len(intervals) != 1 {
		t.Fatalf("µs-identity single segment must resolve: basis=%q n=%d", basis, len(intervals))
	}
	fragmented := item
	fragmented.ImpactMs = 5.0 // measured ≪ envelope → hull across gaps
	if intervals, basis := rootCauseItemDirectionSupport(&fragmented); basis != "" || intervals != nil {
		t.Fatalf("envelope-level row must resolve nothing, got basis=%q", basis)
	}
}

// ── INTERSECT-FIX (§29.143 INTERSECT-REG 归因, 2026-07-19): family-segments
// basis 臂 — the fold-pass exact-caliber family re-enters the direction
// population; hull inventories keep failing the µs identity (偏离④ 原保护). ──

// axiomv2IoFamilySeat mimics the donghu 17267 io_latency fold family (the
// §29.136 regression shape): interval_union caliber, an OVERLAPPING member
// inventory whose union equals the published value to the µs (twin-print
// segments dedup under the union exactly — union 5+2=7ms of 3 members whose
// raw Σ is 8ms).
func axiomv2IoFamilySeat(rank int) RootCauseRankItem {
	return RootCauseRankItem{
		Rank: rank, Tier: "tertiary", Type: "io_latency",
		Thread:   ThreadRef{Comm: "ease.cloudmusic", PID: 63993},
		ImpactMs: 7.0, ProjectedImpactMs: 7.0, CumulativeImpactMs: 7.0,
		EffectiveImpactMs: 7.0, Score: 1.2, Confidence: 0.8,
		LineStart: 61000, LineEnd: 61400,
		Source:    "critical_blocking.io_latency",
		Causality: RootCauseCausalitySelfWallClock, ChainRelevance: "on_chain",
		OnChainBasis:  RootCauseOnChainBasisSelfWallClockInterval,
		DominantState: "iowait",
		StartTs:       17729.510, EndTs: 17729.552,
		MemberCount: 3, MemberFoldCaliber: RootCauseMemberFoldCaliberIntervalUnion,
		familyMemberIntervals: []foldInterval{
			{start: 17729.510, end: 17729.514}, // 4ms ⊂ running[480..520]
			{start: 17729.513, end: 17729.515}, // 2ms, overlaps the twin above
			{start: 17729.550, end: 17729.552}, // 2ms ⊂ running[540..600]
		},
	}
}

// Basis-level pins: exact calibers with the µs identity enter; hull shapes
// (identity broken) and non-exact calibers never do; the arm respects the
// closed-set ORDER (dio wins) and the single-segment arm's degenerate
// identity is untouched.
func TestAXIOMV2FamilySegmentInventoryArmBasis(t *testing.T) {
	fam := axiomv2IoFamilySeat(2)
	intervals, basis := rootCauseItemDirectionSupport(&fam)
	if basis != RootCauseDirectionBasisFamilySegments || len(intervals) != 3 {
		t.Fatalf("interval_union family with µs identity must resolve the family basis: basis=%q n=%d", basis, len(intervals))
	}

	// sum_disjoint caliber with a disjoint inventory summing to the value.
	disjoint := axiomv2IoFamilySeat(2)
	disjoint.MemberFoldCaliber = RootCauseMemberFoldCaliberSumDisjoint
	disjoint.familyMemberIntervals = []foldInterval{
		{start: 17729.510, end: 17729.514}, // 4ms
		{start: 17729.550, end: 17729.553}, // 3ms
	}
	disjoint.ImpactMs = 7.0
	if _, basis := rootCauseItemDirectionSupport(&disjoint); basis != RootCauseDirectionBasisFamilySegments {
		t.Fatalf("sum_disjoint family with µs identity must resolve the family basis, got %q", basis)
	}

	// The census-hull shape: same caliber word, but the inventory holds
	// per-(thread,cpu) group HULLS — internal gaps break the µs identity
	// (union 7ms ≠ published 5ms). 偏离④ 原保护: refuses, seat steps out.
	hull := axiomv2IoFamilySeat(2)
	hull.ImpactMs = 5.0
	if intervals, basis := rootCauseItemDirectionSupport(&hull); basis != "" || intervals != nil {
		t.Fatalf("hull inventory (union ≠ published value) must resolve nothing, got basis=%q", basis)
	}

	// Non-exact calibers never enter, even with a coincidental identity.
	maxFallback := axiomv2IoFamilySeat(2)
	maxFallback.MemberFoldCaliber = RootCauseMemberFoldCaliberMaxOverlapFallback
	if _, basis := rootCauseItemDirectionSupport(&maxFallback); basis != "" {
		t.Fatalf("max_overlap_fallback caliber must never enter, got %q", basis)
	}
	countSum := axiomv2IoFamilySeat(2)
	countSum.MemberFoldCaliber = RootCauseMemberFoldCaliberCountSum
	if _, basis := rootCauseItemDirectionSupport(&countSum); basis != "" {
		t.Fatalf("count_sum caliber must never enter, got %q", basis)
	}

	// Tolerance boundary (all-or-nothing at directionConservationToleranceMs):
	// inside the µs tolerance enters, beyond it steps out.
	within := axiomv2IoFamilySeat(2)
	within.ImpactMs = 7.0 - 0.0005
	if _, basis := rootCauseItemDirectionSupport(&within); basis != RootCauseDirectionBasisFamilySegments {
		t.Fatalf("within-tolerance identity must enter, got %q", basis)
	}
	beyond := axiomv2IoFamilySeat(2)
	beyond.ImpactMs = 7.0 - 0.002
	if _, basis := rootCauseItemDirectionSupport(&beyond); basis != "" {
		t.Fatalf("beyond-tolerance identity must step out, got %q", basis)
	}

	// Closed-set order: a validated dio carrier outranks the family lane.
	withDio := axiomv2IoFamilySeat(2)
	withDio.dioSegmentIntervals = []foldInterval{{start: 17729.510, end: 17729.517}}
	if _, basis := rootCauseItemDirectionSupport(&withDio); basis != RootCauseDirectionBasisDioSegments {
		t.Fatalf("dio carrier must outrank the family lane, got %q", basis)
	}

	// 退化恒等 (pin c): the UNFOLDED single-segment row keeps resolving
	// through single_segment_identity exactly as before the family arm.
	single := RootCauseRankItem{
		Type: "io_latency", ImpactMs: 1.347, MemberCount: 1,
		StartTs: 13762.872568, EndTs: 13762.873915,
	}
	if _, basis := rootCauseItemDirectionSupport(&single); basis != RootCauseDirectionBasisSingleSegment {
		t.Fatalf("unfolded single-segment row must keep the single_segment_identity basis, got %q", basis)
	}
}

// Full-pass pin (the regression witness in unit form): the exact-caliber
// family seat and the self running seat mint the symmetric cross-direction
// pair; the hull twin steps out of the population entirely and leaves only
// the P3 soft-disclosure caveat (no pair, no undisclosed token, values
// untouched).
func TestAXIOMV2FamilySegmentPopulationPair(t *testing.T) {
	rank := axiomv2Rank(axiomv2IoFamilySeat(2), axiomv2SelfRunningSeat(4))
	valuesBefore := []float64{rank.Items[0].ImpactMs, rank.Items[1].ImpactMs,
		rank.Items[0].EffectiveImpactMs, rank.Items[1].EffectiveImpactMs}
	stampRootCauseFixDirections(&rank)
	stampCrossDirectionDisclosureAndConservation(&rank)
	fam, run := rank.Items[0], rank.Items[1]
	if len(fam.CrossDirectionOverlaps) != 1 || len(run.CrossDirectionOverlaps) != 1 {
		t.Fatalf("family × running must mint exactly one symmetric pair, got %d/%d",
			len(fam.CrossDirectionOverlaps), len(run.CrossDirectionOverlaps))
	}
	a, b := fam.CrossDirectionOverlaps[0], run.CrossDirectionOverlaps[0]
	// The family union (7ms across [510..515]+[550..552]) sits entirely
	// inside the running union → overlap == 7ms exactly.
	if diff := a.OverlapMs - 7.0; diff > 0.001 || diff < -0.001 {
		t.Fatalf("pair overlap: got %.3f want 7.000", a.OverlapMs)
	}
	if a.OverlapMs != b.OverlapMs {
		t.Fatalf("symmetric overlap values diverged: %.3f vs %.3f", a.OverlapMs, b.OverlapMs)
	}
	if b.Basis != RootCauseDirectionBasisFamilySegments || b.Direction != "io_dependency" {
		t.Fatalf("entry on the running seat must describe the family partner: %+v", b)
	}
	if a.Basis != RootCauseDirectionBasisSelfRunning || a.Direction != "frequency_thermal" {
		t.Fatalf("entry on the family seat must describe the running partner: %+v", a)
	}
	if fam.directionSupportBasis != RootCauseDirectionBasisFamilySegments {
		t.Fatalf("family seat support basis: got %q", fam.directionSupportBasis)
	}
	valuesAfter := []float64{fam.ImpactMs, run.ImpactMs, fam.EffectiveImpactMs, run.EffectiveImpactMs}
	for i := range valuesBefore {
		if valuesBefore[i] != valuesAfter[i] {
			t.Fatalf("值通道零动 violated at %d", i)
		}
	}

	// Hull twin: the µs identity is broken → the seat leaves the population
	// (no pair on either side, no undisclosed token — the partner was never
	// probed), and the exit is DISCLOSED through the P3 caveat.
	hull := axiomv2IoFamilySeat(2)
	hull.ImpactMs = 5.0
	hull.EffectiveImpactMs = 5.0
	hullRank := axiomv2Rank(hull, axiomv2SelfRunningSeat(4))
	stampRootCauseFixDirections(&hullRank)
	stampCrossDirectionDisclosureAndConservation(&hullRank)
	for i := range hullRank.Items {
		if len(hullRank.Items[i].CrossDirectionOverlaps) != 0 || len(hullRank.Items[i].CrossDirectionOverlapUndisclosed) != 0 {
			t.Fatalf("hull twin population: seat %d must publish no pair face", i)
		}
	}
	found := false
	for _, caveat := range hullRank.Caveats {
		if strings.HasPrefix(caveat, "cross_direction_population: ") && strings.Contains(caveat, "io_latency") {
			found = true
		}
	}
	if !found {
		t.Fatalf("hull exit must leave the population soft-disclosure caveat: %v", hullRank.Caveats)
	}
}

// ── INTERSECT-FIX P3 (pin d): the population-exit soft-disclosure arm ───────

// An ELIGIBLE seat that resolves no per-segment support inventory leaves one
// counting caveat under its own family prefix; discipline-excluded seats
// (rank-0 etc.) never count; a fully-credentialed population leaves nothing.
func TestAXIOMV2PopulationExitSoftDisclosure(t *testing.T) {
	hull := axiomv2IoFamilySeat(2)
	hull.ImpactMs = 5.0 // identity broken → eligible but credential-less
	hull.EffectiveImpactMs = 5.0
	rank := axiomv2Rank(hull, axiomv2SelfRunningSeat(4))
	stampRootCauseFixDirections(&rank)
	stampCrossDirectionDisclosureAndConservation(&rank)
	var caveat string
	for _, existing := range rank.Caveats {
		if strings.HasPrefix(existing, "cross_direction_population: ") {
			if caveat != "" {
				t.Fatalf("exactly one population caveat per board, got a second: %q", existing)
			}
			caveat = existing
		}
	}
	if caveat == "" {
		t.Fatalf("eligible credential-less exit must be disclosed: %v", rank.Caveats)
	}
	if !strings.Contains(caveat, "1 on-chain full seat(s)") || !strings.Contains(caveat, "io_latency") {
		t.Fatalf("caveat must carry the exit count and type token: %q", caveat)
	}
	// 词面卫生: never the 件2/件3 family prefixes.
	if strings.Contains(caveat, "cross_direction_overlap:") || strings.Contains(caveat, "direction_conservation:") {
		t.Fatalf("population caveat must wear its own prefix only: %q", caveat)
	}

	// A discipline-excluded seat (rank-0 豁免席) without an inventory never
	// counts — the caveat discloses CREDENTIAL absence, not 纪律 exclusion.
	exempt := axiomv2IoFamilySeat(0)
	exempt.ImpactMs = 5.0
	clean := axiomv2Rank(exempt, axiomv2SelfRunningSeat(4))
	stampRootCauseFixDirections(&clean)
	stampCrossDirectionDisclosureAndConservation(&clean)
	for _, existing := range clean.Caveats {
		if strings.HasPrefix(existing, "cross_direction_population: ") {
			t.Fatalf("rank-0 exemption must not count as a population exit: %q", existing)
		}
	}

	// 负臂: every eligible seat credentialed → zero disclosure bytes.
	full := axiomv2Rank(axiomv2IoFamilySeat(2), axiomv2SelfRunningSeat(4))
	stampRootCauseFixDirections(&full)
	stampCrossDirectionDisclosureAndConservation(&full)
	for _, existing := range full.Caveats {
		if strings.HasPrefix(existing, "cross_direction_population: ") {
			t.Fatalf("fully-credentialed population must append no exit caveat: %q", existing)
		}
	}
}
