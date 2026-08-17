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
	"strings"
	"testing"
)

// selfAllDonghuQuery — EVOLUTION RECORD (CHAIN-BUDGET, 2026-07-18): the
// witness board is pinned under the EXPLICIT legacy chain caps
// (max_branches=8, max_chain_nodes=1 — the degenerate tier, byte-identical
// to the pre-CHAIN-BUDGET chain by TestChainBudgetDegenerateFloorIsLegacyShape).
// Under the new defaults this window's chain expands the target's full
// 13-segment qualifying set, ALL six target io_latency member IOs then
// genuinely overlap the target's own node windows at fold time, and the
// whole family rides the overlap ruler as ONE single-ruler fold — the
// adjacent→self-basis promotion shape this file pins simply does not occur
// in this window any more (the machinery is unchanged; the new-tier shape
// carries its own pin, TestSelfAllChainBudgetDefaultTierSingleRulerFold).
func selfAllDonghuQuery() Query {
	return Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MaxBranches: 8, MaxChainNodes: 1, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
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
// wall-clock family rides the on-chain channel only for exact requests whose
// completion directly woke the blocked issuer. The full exact-pair census,
// rather than the public Top-8, supplies that causal family.
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
	// The old 3.264ms/5-member value was the accidental public Top-8 slice.
	// The complete strict completion-wake account is 49 disjoint request waits
	// totalling 16.136ms in this fixed fixture.
	if got := fmt.Sprintf("%.3f", rootCauseEffectiveImpactMs(*seat)); got != "16.136" {
		t.Fatalf("published effective must use the full strict request-wait census: %s (%+v)", got, seat)
	}
	if seat.EffectiveImpactMs != seat.CumulativeImpactMs || seat.MemberCount != 49 || !seat.ResourceCompletionClosure {
		t.Fatalf("witness family shape drifted (49 direct completion-wake IOs, eff==cum): %+v", seat)
	}
	// 佩序数 (witness acceptance): a chain-channel ordinal on the ordinary
	// election ladder — never the ◇ adjacent ordinal space, never Rank=0.
	// EVOLUTION RECORD (runnable CPU continuity, 2026-07-14): #5→#6. The
	// target's runnable account is now intersected by exact disjoint segments,
	// not per-CPU aggregate hulls. None of its 5.604ms intersects its typed
	// chain-node windows, so the full per-thread account honestly folds onto
	// the self-wall-clock lane and ranks above this 3.264ms IO seat. The IO
	// seat's own value/basis/tier are untouched.
	// EVOLUTION RECORD (R5 §29.88.3/§29.88.12 单基准, 2026-07-15): #6→#7.
	// keva-3-17439's inversion seat unified its running component onto the
	// 全域最大核最高频点 fold (gated 2.160「按下游消费核」→ 2.286, exactly the
	// former supply-fold value — the §29.88.8 SCAN-4 keva-3 两车道互异 anchor
	// converging to one number); its eff 1.023+2.286=3.309 now sorts above
	// this 3.264ms IO seat. The IO seat's own value/basis/tier are untouched.
	// EVOLUTION RECORD (ELIM-SELF-FIX 件1 §29.93.1, 2026-07-15): #7→#8. The
	// target's own running supply-fold deficit seat (Form-1 修根: 157.248ms
	// window running, deficit 58.320ms on the R5 global-max basis) now mints
	// and ranks #1 on the same window, shifting every seat below it by one.
	// The IO seat's own value/basis/tier are untouched.
	if seat.Rank != 2 || seat.Tier != "secondary" || seat.BackgroundRank != 0 {
		t.Fatalf("witness seat drifted (根因排序#2 · secondary · no background seat): rank=%d tier=%s bg=%d",
			seat.Rank, seat.Tier, seat.BackgroundRank)
	}
}

// The default chain budget obeys the same completion-closure ruler: public
// Top-N and temporal overlap never define task wait. Its smaller chain shape
// admits 45 exact release-point requests totalling 12.208ms.
func TestSelfAllChainBudgetDefaultTierUsesCompletionClosureRuler(t *testing.T) {
	q := selfAllDonghuQuery()
	q.MaxBranches = 0   // default (capacity table)
	q.MaxChainNodes = 0 // default (capacity table)
	rank := BuildRootCauseRank(selfAllDonghuIndex(t), q)
	rows := append(append([]RootCauseRankItem{}, rank.Items...), rank.AbsorbedItems...)
	var fold *RootCauseRankItem
	for i := range rows {
		item := &rows[i]
		if item.Type != "io_latency" || !item.SubjectIsAnalysisTarget {
			continue
		}
		if item.OnChainBasis == RootCauseOnChainBasisSelfWallClockInterval && item.ResourceCompletionClosure {
			fold = item
		}
	}
	if fold == nil {
		t.Fatalf("fixture drifted: expected the strict completion-closure io_latency fold: %+v", rows)
	}
	if got := fmt.Sprintf("%.3f", fold.EffectiveImpactMs); got != "12.208" || fold.MemberCount != 45 {
		t.Fatalf("completion-closure fold must hold 45 exact waits totalling 12.208ms, got %s x%d", got, fold.MemberCount)
	}
	if fold.ChainRelevance != "on_chain" {
		t.Fatalf("the overlap-ruler family rides the on-chain channel: %+v", fold)
	}
}

// TestSelfAllOverlapProvenSeatStaysASeparateSeat (M3, 两把尺禁混折): the
// target's 1.347ms io_latency proved by genuine chain-window overlap never
// folds into the self-basis family — the two proof lanes stay two rulers.
//
// EVOLUTION RECORD (RSPA §29.61.10, 2026-07-14): the witness board's tail
// re-composed — the re-anchored giants (36.757→3.598 anchored, 16.687→
// census-full→1.759 anchored) left the top and the chain threads' census-
// basis seats entered, so the 1.347 overlap-proven row now sorts at candidate
// #13 and the hard root_cause_rank capacity (MaxLimit=12) truncates it with
// the disclosed compaction caveat — a capacity cut, never a fold. The M3
// property under pin is NON-FOLDING: the self family's shape is byte-stable
// (exactly the five self-basis member IOs, window-union 3.264) — had the
// overlap row folded in, the family would count six members and its union
// would exceed 3.264. The truncated row's own lane behavior stays covered by
// the synthetic keep-arm pins (TestSelfAllKeepArmHoldsLaneOnReEnrich).
func TestSelfAllOverlapProvenSeatStaysASeparateSeat(t *testing.T) {
	rank := BuildRootCauseRank(selfAllDonghuIndex(t), selfAllDonghuQuery())
	var family *RootCauseRankItem
	for i := range rank.Items {
		item := &rank.Items[i]
		if item.Type == "io_latency" && item.SubjectIsAnalysisTarget {
			if item.OnChainBasis == "" {
				// The overlap-proven row, when on the board, must never wear
				// the self basis nor a family fold with the self members.
				if item.MemberCount != 0 {
					t.Fatalf("the overlap-proven row must stay a single separate seat: %+v", item)
				}
				continue
			}
			family = item
		}
	}
	if family == nil {
		t.Fatalf("fixture drifted: no self-basis io_latency family: %+v", rank.Items)
	}
	if got := fmt.Sprintf("%.3f", family.EffectiveImpactMs); got != "16.136" || family.MemberCount != 49 || !family.ResourceCompletionClosure {
		t.Fatalf("M3 两把尺禁混折: the self family must hold only the 49 strict completion-wake IOs; non-credential overlap rows stay out: eff=%s members=%d",
			got, family.MemberCount)
	}
}

// TestSelfAllNonWallClockCalibersStayOnSideRail (M2, V2-P0 口径纪律): the
// target's own count/composite rows never take the self basis — the
// page_cache_churn count row rides the ⌗ side rail (EVOLUTION RECORD,
// RNB-5B 件② §29.96.2 终判② 2026-07-15: its wire ChainRelevance is now the
// NON-CHANNEL self_caliber_side token — the former "adjacent" proximity
// verdict violated R8 自身恒为链上 while the count caliber keeps it off the
// wall-clock chain lanes); the on-chain composite block_io row keeps its
// legacy caliber-side seat with no basis.
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
			if item.OnChainBasis != "" || item.ChainRelevance != RootCauseChainRelevanceSelfCaliberSide ||
				item.Tier != RootCauseTierCaliberSide {
				t.Fatalf("count-caliber self row must ride the ⌗ side rail with the non-channel token: %+v", item)
			}
		case "block_io_by_inode":
			if !item.SubjectIsAnalysisTarget {
				continue
			}
			sawComposite = true
			// RNB-5B 修复轮 P2-3: the composite-score self row's LANE is pinned
			// too — the 件② token is COUNT-class-only by ruling, so an
			// engine-level widening (Mut-D) must red here, not pass silently.
			if item.OnChainBasis != "" || item.Tier != RootCauseTierCaliberSide ||
				item.ChainRelevance != "on_chain" {
				t.Fatalf("composite-score self row must keep its ⌗ side seat with no basis and its legacy on_chain lane: %+v", item)
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
			// EVOLUTION RECORD (ELIM-SELF-FIX 件1③, R8 §29.93 2026-07-15):
			// the published CHANNEL flipped — a self symptom/context row
			// never wears ◇ on the wire (P-4 残口收口); it speaks the honest
			// self causality (no fabricated wakeup edge), keeps no basis, and
			// its tier/eff/ordinal demotion is untouched (channel identity,
			// not an eliminability or election change).
			if item.OnChainBasis != "" || item.ChainRelevance != "on_chain" ||
				item.Causality != RootCauseCausalitySelfWallClock || item.Tier != RootCauseTierContextOnly || item.Rank != 0 {
				t.Fatalf("the idle-cadence context row must keep its demotion on the R8 chain channel: %+v", item)
			}
		}
	}
	if !sawBinder || !sawPacing {
		t.Fatalf("fixture drifted: binder=%v pacing=%v rows missing", sawBinder, sawPacing)
	}
}

// A closing blocked marker refines only the D/IO subsegment that owns it. The
// CompThread production branch is running-dominant over its aligned impact;
// rewriting that root's physical identity with the nested D marker used to
// evade the CausalImpact twin gate, mint a duplicate zero-effective running
// row and evict the pacing disclosure from the bounded side lane.
func TestSelfAllBlockedMarkerCannotRewriteRunningDominantRootIdentity(t *testing.T) {
	idx, q := selfAllDonghuIndex(t), selfAllDonghuQuery()
	chain := BuildWakeupChain(idx, q)
	var seed *RootEvidence
	for i := range chain.CausalImpacts {
		impact := chain.CausalImpacts[i]
		if impact.Thread.PID != 2955 || impact.DominantState != string(StateRunning) {
			continue
		}
		candidate := rootEvidenceFromCausalImpact(impact, "", 0)
		seed = &candidate
		break
	}
	if seed == nil {
		t.Fatal("fixture drifted: CompThread running-dominant causal impact missing")
	}
	matchedRoot := false
	for _, root := range chain.RootEvidence {
		if root.Type != "running" || root.Thread.PID != 2955 || root.DurationMs != seed.DurationMs {
			continue
		}
		matchedRoot = true
		if root.LineStart != seed.LineStart || root.LineEnd != seed.LineEnd {
			t.Fatalf("a nested D marker must not rewrite the running root identity: root=%+v seed=%+v", root, *seed)
		}
		if strings.Contains(root.Summary, "sched_blocked_reason") {
			t.Fatalf("a running root must not speak D/IO marker semantics: %+v", root)
		}
	}
	if !matchedRoot {
		t.Fatal("fixture drifted: matching running RootEvidence missing")
	}

	rank := BuildRootCauseRank(idx, q)
	for _, item := range rank.Items {
		if item.Type == "running" && item.Thread.PID == 2955 {
			t.Fatalf("the running RootEvidence twin must remain suppressed, got duplicate %+v", item)
		}
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
	promoted, contextual := 0, 0
	for _, cand := range blocking.Items {
		if cand.Thread.PID != 17267 {
			continue
		}
		switch cand.Type {
		case "io_latency":
			if cand.ResourceCompletionClosure {
				if cand.ChainRelevance != "on_chain" ||
					(cand.OnChainBasis != RootCauseOnChainBasisSelfWallClockInterval && cand.OverlapMs <= 0) {
					t.Fatalf("direct completion-wake IO must ride either its genuine overlap lane or the typed self lane: %+v", cand)
				}
				promoted++
				if cand.OnChainBasis == RootCauseOnChainBasisSelfWallClockInterval && cand.OverlapMs != 0 {
					t.Fatalf("self-basis candidate must not fabricate overlap: %+v", cand)
				}
			} else {
				contextual++
				if cand.ChainRelevance == "on_chain" || cand.OnChainBasis != "" {
					t.Fatalf("request residence without completion-wake proof must stay context: %+v", cand)
				}
			}
		case "binder_wait":
			if cand.OnChainBasis != "" {
				t.Fatalf("the symptom-lane binder candidate never takes the self basis: %+v", cand)
			}
		}
	}
	if promoted == 0 || contextual == 0 {
		t.Fatalf("witness must exercise both direct-causal and request-only IO lanes: promoted=%d contextual=%d", promoted, contextual)
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
	item.ResourceCompletionClosure = true
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
		if rootCauseItemIsRunningCaliber(item) {
			// ELIM-SELF-FIX 件1 (§29.93.1, 2026-07-15): the self running
			// fold-deficit seat consumes the SAME per-state ladder as every
			// on-chain running row — eff IS the supply-fold deficit (§20.2),
			// raw wall clock stays on the display channels (cum/imp). The
			// 零特判 identity for this caliber is eff==deficit, never
			// eff==raw.
			if item.EffectiveImpactMs != item.SupplyFoldDeficitMs || item.EffectiveImpactMs > item.CumulativeImpactMs {
				t.Fatalf("self-basis running row must publish eff==deficit<=cum: %+v", item)
			}
			continue
		}
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
