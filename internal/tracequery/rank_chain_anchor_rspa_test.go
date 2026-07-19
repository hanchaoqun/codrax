package tracequery

// rank_chain_anchor_rspa_test.go — RSPA (§29.61.10a/b/c user rulings,
// 2026-07-13/14; 处置矩阵 attr_bound_rca_20260714) engine pins.
//
// EVOLUTION RECORD (RSPA §29.61.10, 2026-07-14): the on-chain seat-value
// re-anchoring lands — a chain thread's window state seat no longer carries
// its FULL-window account on the chain tier. The credential-anchored portion
// (segments ∩ typed wakeup-dependency jump windows; µs-identical to the chain
// lane's own per-state value, gate-checked) stays ⛓; the remainder mints the
// ◇ adjacent seat. Witness numbers (matrix §2): donghu CompThread D-IO
// 36.757 = 3.598 anchored + 33.159 remainder (90.2% held no credential;
// ❶ 让位); donghu JankManager runnable census-full 31.191 = 1.759 anchored +
// 29.432 remainder; tieba CookieMonster 26.738 = 21.242 + 5.496; tieba
// NetworkService 20.342 = 14.700 + 5.642; tieba 60555 D-IO 18.135 = 17.819
// anchored (fscache 7.386 + "" 10.433 stay ⛓) + 0.316 (the two hmfs cause
// seats, zero-anchored, ◇).
//
// ELIM-1 收口验证 (matrix §4④): (a) per-thread bipartition identity
// 旧全窗席值 = ⛓锚定 + ◇余段 (µs tolerance); (b) population conservation —
// no seat value silently vanishes (every migrated account keeps both halves
// published or the fully-anchored value on one ⛓ seat / absorbed twin);
// (c) fail-open negatives (self exemption, anchor-less build, identity-gate
// divergence); (d) double-trace replay (donghu ❶ 换主 + tieba 近满锚定微变).

import (
	"context"
	"math"
	"strings"
	"testing"
)

// --- helper unit pins --------------------------------------------------------

func TestRSPAMergeAnchorTimeWindowsUnions(t *testing.T) {
	merged := mergeAnchorTimeWindows([]TimeWindow{
		{StartTs: 3.0, EndTs: 4.0},
		{StartTs: 1.0, EndTs: 2.0},
		{StartTs: 1.5, EndTs: 2.5},
	})
	if len(merged) != 2 || merged[0].StartTs != 1.0 || merged[0].EndTs != 2.5 || merged[1].StartTs != 3.0 {
		t.Fatalf("window union drifted: %+v", merged)
	}
	// Exact sum over a merged (pairwise disjoint) union — never double counts.
	if got := anchorWindowsOverlapMs(merged, 0.0, 10.0); math.Abs(got-2500.0) > 0.0001 {
		t.Fatalf("union overlap must be exact: %.6f", got)
	}
	if got := anchorWindowsOverlapMs(merged, 2.25, 3.5); math.Abs(got-750.0) > 0.0001 {
		t.Fatalf("partial overlap drifted: %.6f", got)
	}
}

func TestRSPAAnchorWindowsExcludeTarget(t *testing.T) {
	chain := ChainResult{
		Target: ThreadRef{PID: 100},
		Nodes: []ChainNode{
			{Thread: ThreadRef{PID: 100}, Depth: 0, Window: TimeWindow{StartTs: 1, EndTs: 2}},
			{Thread: ThreadRef{PID: 200}, Depth: 1, Window: TimeWindow{StartTs: 1, EndTs: 1.5}},
			{Thread: ThreadRef{PID: 200}, Depth: 2, Window: TimeWindow{StartTs: 1.4, EndTs: 1.8}},
		},
	}
	anchors := chainAnchorWindowsByPID(chain)
	if _, ok := anchors[100]; ok {
		t.Fatalf("self-causality: the target must never enter the anchor map (豁免重锚): %+v", anchors)
	}
	if windows := anchors[200]; len(windows) != 1 || windows[0].StartTs != 1 || windows[0].EndTs != 1.8 {
		t.Fatalf("depth>0 windows must union per pid: %+v", anchors[200])
	}
}

// TestRSPAIdentityGateFailsOpenOnDivergence — §6.3 精确臂.
//
// EVOLUTION RECORD (RNB-1, §29.88 R2 user ruling, 2026-07-14): the µs
// identity is NO LONGER a mint gate — fail-open on divergence kept the FULL
// window value on the chain tier (the customer runnable.txt W1/W2 disease;
// donghu keva-1 Δ0.085 production witness). The census bipartition is
// self-sufficiently exact, so a divergent pid still mints its decision with
// identityHolds=false (the case-A ownership qualification + typed double-Σ
// disclosure inputs). The anchor-less and clock-regressed negatives keep
// deciding nothing byte-identically.
func TestRSPAIdentityGateFailsOpenOnDivergence(t *testing.T) {
	chain := ChainResult{
		Target: ThreadRef{PID: 100},
		Nodes: []ChainNode{
			{Thread: ThreadRef{PID: 200}, Depth: 1, Window: TimeWindow{StartTs: 1, EndTs: 2}},
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
	runnable, _ := buildRSPAFamilyDecisions(chain, stats)
	if decision := runnable[200]; !decision.migrate || math.Abs(decision.anchoredMs-5.0) > 0.0001 || !decision.identityHolds {
		t.Fatalf("matching sums must migrate with the ownership identity: %+v", decision)
	}
	// Divergent chain value (e.g. overlapping occurrence windows double-count
	// on the chain Σ) → the decision still mints (census bipartition is
	// self-sufficient) but the case-A ownership identity is withdrawn and
	// both Σs ride the decision for the typed double-account disclosure.
	chain.CausalImpacts[0].RunnableMs = 5.2
	runnable, _ = buildRSPAFamilyDecisions(chain, stats)
	if decision, ok := runnable[200]; !ok || !decision.migrate || decision.identityHolds ||
		!decision.chainLanePresent || math.Abs(decision.chainLaneMs-5.2) > 0.0001 || math.Abs(decision.anchoredMs-5.0) > 0.0001 {
		t.Fatalf("µs-identity divergence must mint a divergent decision (RNB-1): %+v", decision)
	}
	// Anchor-less sweep (stats built without the chain basis) → nothing.
	statsNoAnchor := stats
	statsNoAnchor.chainAnchorsByPID = nil
	runnable, dio := buildRSPAFamilyDecisions(chain, statsNoAnchor)
	if runnable != nil || dio != nil {
		t.Fatalf("anchor-less stats must decide nothing: %+v %+v", runnable, dio)
	}
	// 件6 (修复轮, 2026-07-14): a clock-regressed index removes the ordered-
	// stream premise — the MAX-fallback window seats stay unclipped, so NO
	// decision may mint (and hence no chain D-IO seat suppression): the
	// board keeps the full legacy publication (板面失链值行 negative pin).
	chain.CausalImpacts[0].RunnableMs = 5.0
	statsRegressed := stats
	statsRegressed.offCPUProducerDisjoint = false
	runnable, dio = buildRSPAFamilyDecisions(chain, statsRegressed)
	if runnable != nil || dio != nil {
		t.Fatalf("a regressed (non-producer-disjoint) sweep must decide nothing: %+v %+v", runnable, dio)
	}
}

func TestRSPACompletionClosureToleranceBounds(t *testing.T) {
	stats := WindowStats{anchoredDIOWakeups: []anchoredDIOWakeupRecord{{wakerPID: 92, ts: 10.0005}}}
	if !resourceCompletionClosureProven(stats, 92, 10.0000, 10.0004) {
		t.Fatal("wakeup within the µs handler slack after completion must prove closure")
	}
	if resourceCompletionClosureProven(stats, 92, 10.0000, 9.9990) {
		t.Fatal("degenerate interval must never prove closure")
	}
	if resourceCompletionClosureProven(stats, 93, 10.0000, 10.0004) {
		t.Fatal("a different waker pid is not this IO's completion")
	}
	if resourceCompletionClosureProven(stats, 92, 10.0010, 10.0020) {
		t.Fatal("a wakeup before the IO issue is not its completion closure")
	}
}

// TestRSPAResourceLaneClosureArm — M-IO (10c 判据: 证据面非类别面): with the
// anchor basis present, a NON-target io_latency row's on-chain verdict needs
// the typed completion-closure credential; pure overlap demotes to ◇. The
// target's own rows and anchor-less builds keep the legacy lane.
func TestRSPAResourceLaneClosureArm(t *testing.T) {
	target := ThreadRef{PID: 100}
	base := RootCauseRankItem{Type: "io_latency", Thread: ThreadRef{PID: 200}, StartTs: 1, EndTs: 2}
	ctx := chainCandidateContext{relevance: "on_chain", overlapMs: 3}

	evaluated := base
	evaluated.resourceClosureEvaluated = true
	if got := rootCauseChainContextForItem(evaluated, ctx, target); got.relevance != "adjacent" {
		t.Fatalf("evaluated overlap-only io row must demote to ◇: %+v", got)
	}
	proven := evaluated
	proven.ResourceCompletionClosure = true
	if got := rootCauseChainContextForItem(proven, ctx, target); got.relevance != "on_chain" {
		t.Fatalf("completion-closure credential keeps the chain lane: %+v", got)
	}
	self := evaluated
	self.Thread = target
	if got := rootCauseChainContextForItem(self, ctx, target); got.relevance != "on_chain" {
		t.Fatalf("the target's own row is self-causality exempt: %+v", got)
	}
	if got := rootCauseChainContextForItem(base, ctx, target); got.relevance != "on_chain" {
		t.Fatalf("anchor-less (unevaluated) rows keep the legacy overlap lane: %+v", got)
	}
}

// --- synthetic end-to-end bipartition pins ------------------------------------

// rspaCaseATrace: app-100 (target) sleeps 1.000→1.029 woken by worker-200;
// worker is runnable 1.001→1.028 INSIDE its dependency window (anchored) and
// runnable again 1.040→1.060 OUTSIDE it (no credential). One physical
// runnable account 47ms = 27ms anchored + 20ms remainder.
func rspaCaseATrace(t *testing.T) *Index {
	t.Helper()
	return buildTraceIndex(t, "rspa_case_a.systrace", `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 1.000500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-300 (300) [002] .... 1.001000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.028000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.029000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 1.030000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.031000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
      other-300 (300) [002] .... 1.040000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.060000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.061000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
	`)
}

func TestRSPACaseABipartitionIdentityAndLanes(t *testing.T) {
	idx := rspaCaseATrace(t)
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.07, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	var chainSeat, remainderSeat *RootCauseRankItem
	for i := range rank.Items {
		item := &rank.Items[i]
		if item.Thread.PID != 200 || item.RunnableMs <= 0 {
			continue
		}
		if strings.HasPrefix(item.Source, "wakeup_chain") && rootCauseItemIsOnChain(*item) {
			chainSeat = item
		}
		if item.ChainAnchorRemainderSeat {
			remainderSeat = item
		}
	}
	if chainSeat == nil {
		t.Fatalf("the chain lane must own the anchored runnable account: %+v", rank.Items)
	}
	if remainderSeat == nil {
		t.Fatalf("the ◇ remainder half must stay published (零静默消失): %+v", rank.Items)
	}
	if math.Abs(chainSeat.RunnableMs-27.0) > 0.01 {
		t.Fatalf("chain seat must carry the anchored 27ms: %+v", chainSeat)
	}
	if remainderSeat.ChainRelevance != "adjacent" || remainderSeat.Causality != "adjacent_to_wakeup_chain" {
		t.Fatalf("the remainder half rides the ◇ adjacent lane, never the chain tier: %+v", remainderSeat)
	}
	// ELIM-1 (a): the bipartition identity — anchored + remainder == full.
	if math.Abs(remainderSeat.ChainAnchoredMs+remainderSeat.RunnableMs-remainderSeat.ChainAnchorFullMs) > rspaAnchorIdentityTolMs {
		t.Fatalf("bipartition identity broken: anchored=%.6f remainder=%.6f full=%.6f",
			remainderSeat.ChainAnchoredMs, remainderSeat.RunnableMs, remainderSeat.ChainAnchorFullMs)
	}
	if math.Abs(remainderSeat.ChainAnchorFullMs-47.0) > 0.01 || math.Abs(remainderSeat.RunnableMs-20.0) > 0.01 {
		t.Fatalf("witness split drifted (want 47 = 27 + 20): %+v", remainderSeat)
	}
	// The remainder's published value channels all speak the remainder.
	if remainderSeat.EffectiveImpactMs != remainderSeat.RunnableMs || remainderSeat.CumulativeImpactMs != remainderSeat.RunnableMs {
		t.Fatalf("remainder value channels must agree: %+v", remainderSeat)
	}
	// 跨车道双席收口: no OTHER on-chain runnable seat for the pid remains
	// beside the chain seat (the pre-RSPA full-window double is gone).
	for i := range rank.Items {
		item := &rank.Items[i]
		if item.Thread.PID == 200 && item != chainSeat && rootCauseItemIsOnChain(*item) && item.RunnableMs > 0 {
			t.Fatalf("cross-lane double seat must dissolve: %+v", item)
		}
	}
	// 件5(a) end-to-end half: the fragmented (state_churn) satellite of the
	// migrated thread rides the same ◇ remainder form as the formal seat.
	var fragmentedRemainder bool
	for _, item := range rank.Items {
		if item.Thread.PID == 200 && item.Type == "fragmented_runnable_wait" && item.ChainAnchorRemainderSeat {
			fragmentedRemainder = true
			if math.Abs(item.RunnableMs-20.0) > 0.01 || item.ChainRelevance != "adjacent" {
				t.Fatalf("fragmented satellite ◇ rewrite drifted: %+v", item)
			}
		}
	}
	if !fragmentedRemainder {
		t.Fatalf("件5(a): fragmented satellite remainder missing: %+v", rank.Items)
	}
}

// TestRSPASchedulerSatelliteArmUnit — 件5(a) unit half (M7 突变实锤补覆盖):
// the scheduler_latency/low_frequency per-row interval arm — a fully
// anchored satellite keeps its lane byte-identically; a partially anchored
// one rewrites to the ◇ remainder.
//
// EVOLUTION RECORD (RNB-1, §29.88 R4/§29.88.8 case 4, 2026-07-14): the
// hull-mismatch/interval-less form no longer fails OPEN (it kept the FULL
// value on the chain tier — the donghu 2955 低频运行 22.408/tieba 59953
// 10.776 live escapes on interval-less compute_supply verdict rows). A
// satellite that cannot prove its own anchored share now lane-demotes WHOLE
// to ◇ with values untouched; a fully-anchored pid keeps the chain lane.
func TestRSPASchedulerSatelliteArmUnit(t *testing.T) {
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
		// fully anchored satellite: [1.010, 1.015] ⊆ window → untouched.
		{Type: "scheduler_latency", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 5.0, ImpactMs: 5.0, CumulativeImpactMs: 5.0,
			StartTs: 1.010, EndTs: 1.015, Source: "scheduler_latency_stats"},
		// partial: [1.025, 1.045] → anchored 5ms, remainder 15ms → ◇.
		{Type: "scheduler_latency", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 20.0, ImpactMs: 20.0, CumulativeImpactMs: 20.0,
			StartTs: 1.025, EndTs: 1.045, Source: "scheduler_latency_stats"},
		// low_frequency partial rides the same arm.
		{Type: "low_frequency", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 20.0, ImpactMs: 20.0, CumulativeImpactMs: 20.0,
			StartTs: 1.025, EndTs: 1.045, Source: "scheduler_latency_stats"},
		// hull mismatch: interval 20ms but scalar 12ms → RNB-1 R4 lane
		// demotion (whole row ◇, values untouched — the hull cannot prove the
		// row's own anchored share and the pid census is not fully anchored).
		{Type: "scheduler_latency", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 12.0, ImpactMs: 12.0, CumulativeImpactMs: 12.0,
			StartTs: 1.025, EndTs: 1.045, Source: "scheduler_latency_stats"},
		// interval-less compute_supply low_frequency verdict (donghu 2955
		// 低频运行 live shape) → same R4 lane demotion.
		{Type: "low_frequency", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 22.408, ImpactMs: 22.408, CumulativeImpactMs: 22.408,
			Source: "window_stats.compute_supply"},
	}
	items = reanchorOnChainStateSeats(chain, stats, items)
	if items[0].ChainAnchorFullMs != 0 || items[0].ChainRelevance != "on_chain" {
		t.Fatalf("fully anchored satellite must stay untouched: %+v", items[0])
	}
	for _, i := range []int{1, 2} {
		if !items[i].ChainAnchorRemainderSeat || items[i].ChainRelevance != "adjacent" ||
			math.Abs(items[i].RunnableMs-15.0) > rspaAnchorIdentityTolMs ||
			math.Abs(items[i].ChainAnchoredMs-5.0) > rspaAnchorIdentityTolMs {
			t.Fatalf("partial satellite must rewrite to the ◇ remainder: %+v", items[i])
		}
	}
	for _, i := range []int{3, 4} {
		if !items[i].ChainCredentialLaneDemoted || items[i].ChainRelevance != "adjacent" ||
			items[i].ChainAnchorFullMs != 0 || items[i].RunnableMs != items[i].CumulativeImpactMs {
			t.Fatalf("unprovable satellite must lane-demote whole with values untouched: %+v", items[i])
		}
	}
	if items[3].RunnableMs != 12.0 || items[4].RunnableMs != 22.408 {
		t.Fatalf("lane demotion must never touch the published values: %+v %+v", items[3], items[4])
	}
}

// TestRSPARegressedTraceFailsOpenEndToEnd — 件6 fixture pin: one backwards
// timestamp removes the ordered-stream premise; the whole re-anchoring
// (decisions, clipping, chain-seat suppression) must fail open — the board
// keeps the legacy full-window on-chain publication byte-shape.
func TestRSPARegressedTraceFailsOpenEndToEnd(t *testing.T) {
	idx := buildTraceIndex(t, "rspa_regressed.systrace", `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 1.000500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-300 (300) [002] .... 1.001000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
      other-300 (300) [002] .... 1.000900: sched_wakeup: comm=other pid=300 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.028000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.029000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 1.030000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.031000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
      other-300 (300) [002] .... 1.040000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.060000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.061000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
	`)
	if idx.TimestampOrder == TraceTimestampOrderMonotonic {
		t.Fatal("fixture drifted: the regression line must break monotonic order")
	}
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.07, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	for _, item := range append(append([]RootCauseRankItem{}, rank.Items...), rank.AbsorbedItems...) {
		if item.ChainAnchorFullMs != 0 || item.ChainAnchoredMs != 0 || item.ChainAnchorRemainderSeat {
			t.Fatalf("件6: a regressed trace must never re-anchor any seat: %+v", item)
		}
	}
}

// TestRSPAFullyAnchoredWindowSeatAbsorbedIntoChainSeat — the §6.3 锚定分解吸收
// arm: a fully-anchored window seat (remainder ≈ 0) folds into its chain seat
// losslessly even though the interval sets are not byte-equal (the pre-RSPA
// exact-match recon failed open on exactly this shape, leaving the §1.3
// double seat).
func TestRSPAFullyAnchoredWindowSeatAbsorbedIntoChainSeat(t *testing.T) {
	idx := buildTraceIndex(t, "rspa_absorb.systrace", `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 1.000500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-300 (300) [002] .... 1.001000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.028000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.029000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 1.030000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.031000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.04, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	active := 0
	for _, item := range rank.Items {
		if item.Thread.PID == 200 && item.RunnableMs > 0 && rootCauseItemIsOnChain(item) {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("one occurrence set one seat: fully-anchored window twin must be absorbed, got %d active on-chain runnable rows: %+v", active, rank.Items)
	}
	absorbed := false
	for _, item := range rank.AbsorbedItems {
		if item.Thread.PID == 200 && item.ChainAnchorRemainderSeat {
			absorbed = true
			// The lossless carrier keeps the decomposition: the full account
			// stays reconstructible (零静默消失).
			if math.Abs(item.ChainAnchoredMs-item.ChainAnchorFullMs) > rspaAnchorIdentityTolMs {
				t.Fatalf("absorbed twin must be the fully-anchored half: %+v", item)
			}
		}
	}
	if !absorbed {
		t.Fatalf("the absorbed window twin must survive on the lossless carrier: %+v", rank.AbsorbedItems)
	}
}

// TestRSPATargetSelfSeatsExempt — the self-causality negative pin (matrix
// 处置矩阵 SELF rows; the h2/h3 golden zero-perturbation criterion): the
// analysis TARGET's own window seats never migrate — no decomposition
// fields, no lane change, full-window value intact.
func TestRSPATargetSelfSeatsExempt(t *testing.T) {
	idx := rspaCaseATrace(t)
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.07, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	for _, item := range rank.Items {
		if item.Thread.PID != 100 {
			continue
		}
		if item.ChainAnchorFullMs != 0 || item.ChainAnchoredMs != 0 || item.ChainAnchorRemainderSeat {
			t.Fatalf("target self seat must be exempt from re-anchoring: %+v", item)
		}
	}
}

// --- double-trace replay (matrix §2 witnesses, in-repo fixtures) --------------

func TestRSPADonghuWitnessBoard(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	rank := BuildRootCauseRank(idx, Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	var compThreadD, compThreadDRem, jankRunnable, jankRunnableRem *RootCauseRankItem
	for i := range rank.Items {
		item := &rank.Items[i]
		switch {
		case item.Thread.PID == 2955 && item.Type == "d_state_or_io_wait" && strings.HasPrefix(item.Source, "window_stats"):
			if item.ChainAnchorRemainderSeat {
				compThreadDRem = item
			} else {
				compThreadD = item
			}
		case item.Thread.PID == 9655 && item.Type == "runnable_wait" && strings.HasPrefix(item.Source, "window_stats"):
			if item.ChainAnchorRemainderSeat {
				jankRunnableRem = item
			} else {
				jankRunnable = item
			}
		}
	}
	if compThreadD == nil || compThreadDRem == nil || jankRunnable == nil || jankRunnableRem == nil {
		t.Fatalf("witness rows missing: ⛓D=%v ◇D=%v ⛓R=%v ◇R=%v", compThreadD != nil, compThreadDRem != nil, jankRunnable != nil, jankRunnableRem != nil)
	}
	// ❶ 换主 (matrix §2.1): CompThread's D account re-anchors 36.757 → 3.598
	// on the chain tier; 33.159 (90.2%) held no credential and rides ◇.
	if math.Abs(compThreadD.CumulativeImpactMs-3.598) > 0.002 || compThreadD.ChainRelevance != "on_chain" {
		t.Fatalf("CompThread ⛓ anchored seat drifted: %+v", compThreadD)
	}
	if math.Abs(compThreadDRem.CumulativeImpactMs-33.159) > 0.002 || compThreadDRem.ChainRelevance != "adjacent" {
		t.Fatalf("CompThread ◇ remainder drifted: %+v", compThreadDRem)
	}
	if math.Abs(compThreadD.CumulativeImpactMs+compThreadDRem.CumulativeImpactMs-36.757) > 0.002 {
		t.Fatalf("CompThread bipartition identity broken: %.3f + %.3f != 36.757",
			compThreadD.CumulativeImpactMs, compThreadDRem.CumulativeImpactMs)
	}
	// The refined-D proof words ride the ⛓ half unchanged (词面按凭证如实).
	if !compThreadD.DStateAllNonIOProven || compThreadD.BlockedReasonCaller != "dma_fence_default_w" {
		t.Fatalf("CompThread ⛓ seat must keep the refined proof + 等待对象: %+v", compThreadD)
	}
	// JankManager runnable: census full account 31.191 = 1.759 anchored +
	// 29.432 remainder (帽基→census 顺带修: the capped basis said 16.687).
	if math.Abs(jankRunnable.RunnableMs-1.759) > 0.002 || jankRunnable.ChainRelevance != "on_chain" {
		t.Fatalf("JankManager ⛓ anchored runnable drifted: %+v", jankRunnable)
	}
	if math.Abs(jankRunnableRem.RunnableMs-29.432) > 0.002 || !jankRunnableRem.ChainAnchorRemainderSeat {
		t.Fatalf("JankManager ◇ remainder drifted: %+v", jankRunnableRem)
	}
	if math.Abs(jankRunnableRem.ChainAnchorFullMs-31.191) > 0.002 {
		t.Fatalf("JankManager census-full account drifted: %+v", jankRunnableRem)
	}
	rspaAssertBoardBipartitionInvariants(t, rank)
}

func TestRSPATiebaWitnessBoard(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatal(err)
	}
	rank := BuildRootCauseRank(idx, Query{PID: 59566, TimeStart: 34579.472865, TimeEnd: 34579.587805,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	// 近满锚定微变 (matrix §2.2): 60555's D-IO partition stays nearly intact —
	// the fully-anchored cause seats keep their FULL values on the chain tier
	// (fscache 7.386, "" 10.433 — Σ 17.819 == the chain lane's own value) and
	// the zero-anchored hmfs seats (Σ 0.316) sit honestly off-chain. The
	// cause words never leave their seats (§29.50.5 分区机械零改).
	dioByCause := map[string]float64{}
	dioOnChain := map[string]bool{}
	var cookieInversion, networkInversion *RootCauseRankItem
	for i := range rank.Items {
		item := &rank.Items[i]
		if item.Thread.PID == 60555 && (item.Type == "d_state_or_io_wait" || item.Type == "io_wait") && strings.HasPrefix(item.Source, "window_stats") {
			dioByCause[item.BlockedReasonCaller] += item.CumulativeImpactMs
			dioOnChain[item.BlockedReasonCaller] = rootCauseItemIsOnChain(*item)
		}
		if item.Type == "priority_inversion_runnable_wait" && item.ChainCredentialLaneDemoted {
			switch item.Thread.PID {
			case 59843:
				cookieInversion = item
			case 60595:
				networkInversion = item
			}
		}
	}
	want := map[string]struct {
		ms      float64
		onChain bool
	}{
		"fscache_page_wait_o": {7.386, true},
		"":                    {10.433, true},
		"hmfs_get_dnode":      {0.171, false},
		"hmfs_read":           {0.145, false},
	}
	sum := 0.0
	for cause, expect := range want {
		if math.Abs(dioByCause[cause]-expect.ms) > 0.002 {
			t.Fatalf("tieba 60555 %q seat drifted: got %.3f want %.3f (all=%v)", cause, dioByCause[cause], expect.ms, dioByCause)
		}
		if dioOnChain[cause] != expect.onChain {
			t.Fatalf("tieba 60555 %q lane drifted (want onChain=%v)", cause, expect.onChain)
		}
		sum += dioByCause[cause]
	}
	if math.Abs(sum-18.135) > 0.002 {
		t.Fatalf("partition Σ must reconstruct the full window truth 18.135, got %.3f", sum)
	}
	// Priority-point authority keeps each giant's full runnable census on the
	// disclosure lane, but only the closed-range-stable lower-priority overlap
	// participates in ranking. RNB's indivisible-overlap rule therefore demotes
	// the full direct seat beside the chain-owned aggregate instead of inventing
	// a synthetic runnable remainder.
	// EVOLUTION RECORD (CHAIN-BUDGET, 2026-07-18): the ledger-anchored
	// runnable shares rose 21.242→26.001 (CookieMonster) and 14.700→18.979
	// (NetworkService) because the chain now expands the target's full
	// qualifying depth-0 segment set (max_branches 8→16; tieba had 12) — the
	// audited 20.7%/26.0% reverse-form leak (real edge-bearing segments
	// outside the walked anchor windows, onchain_segment_audit_20260718.md)
	// anchored honestly. Every published value (full runnable census,
	// proven-lower overlap, unknown split) is unchanged — only the anchored/
	// remainder decomposition moved.
	if cookieInversion == nil || math.Abs(cookieInversion.RunnableMs-26.738) > 0.002 ||
		math.Abs(cookieInversion.EffectiveImpactMs-1.847) > 0.002 ||
		math.Abs(cookieInversion.PriorityRelationUnknownOrNonLowerMs-24.891) > 0.002 ||
		math.Abs(cookieInversion.ledgerAnchoredRunnableMs-26.001) > 0.002 {
		t.Fatalf("CookieMonster proven/full inversion account drifted: %+v", cookieInversion)
	}
	if networkInversion == nil || math.Abs(networkInversion.RunnableMs-20.342) > 0.002 ||
		math.Abs(networkInversion.EffectiveImpactMs-0.683) > 0.002 ||
		math.Abs(networkInversion.PriorityRelationUnknownOrNonLowerMs-19.659) > 0.002 ||
		math.Abs(networkInversion.ledgerAnchoredRunnableMs-18.979) > 0.002 {
		t.Fatalf("NetworkService proven/full inversion account drifted: %+v", networkInversion)
	}
	rspaAssertBoardBipartitionInvariants(t, rank)
}

// rspaAssertBoardBipartitionInvariants — ELIM-1 (a)/(b) sweep over one
// published board: every decomposed row satisfies the additive identity and
// carries a consistent lane; every ◇ remainder has a ⛓ counterpart account
// on the board (population conservation — the anchored value never vanishes).
func rspaAssertBoardBipartitionInvariants(t *testing.T, rank RootCauseRankResult) {
	t.Helper()
	rows := append(append([]RootCauseRankItem{}, rank.Items...), rank.AbsorbedItems...)
	for _, item := range rows {
		if item.ChainAnchorFullMs == 0 {
			if item.ChainAnchoredMs != 0 || item.ChainAnchorRemainderSeat {
				t.Fatalf("decomposition fields must travel together: %+v", item)
			}
			continue
		}
		published := item.RunnableMs + item.DStateMs + item.IOWaitMs
		if item.ChainAnchorRemainderSeat {
			if math.Abs(item.ChainAnchoredMs+published-item.ChainAnchorFullMs) > 0.002 {
				t.Fatalf("◇ identity broken: anchored=%.3f + published=%.3f != full=%.3f (%+v)",
					item.ChainAnchoredMs, published, item.ChainAnchorFullMs, item)
			}
			if !item.AbsorbedByRankFamily && item.ChainRelevance != "adjacent" {
				t.Fatalf("◇ remainder must ride the adjacent lane: %+v", item)
			}
			// Population conservation: the anchored complement is on the board
			// — either the same thread's clipped ⛓ window seat or a chain-lane
			// seat of the same state family.
			//
			// RSPA-HYG 件⑤ (§29.77 立案⑤) — EVOLUTION RECORD: the former
			// release arm read "board compacted ∧ anchored<1ms" (a magnitude
			// heuristic — a noisy signal in a hard assertion). It is replaced
			// by the TYPED direct assertion: a counterpart absent from the
			// published board must be found in the pre-truncation pool
			// (capacity-truncated is the only lane between the two), and the
			// truncation must be disclosed through a candidates Compaction.
			if item.ChainAnchoredMs > rspaAnchorIdentityTolMs {
				matches := func(peer RootCauseRankItem) bool {
					if peer.Thread.PID != item.Thread.PID || peer.ChainAnchorRemainderSeat {
						return false
					}
					if peer.ChainAnchorFullMs > 0 && rootCauseItemIsOnChain(peer) {
						return true
					}
					if strings.HasPrefix(peer.Source, "wakeup_chain") && rootCauseItemIsOnChain(peer) {
						return true
					}
					return rootCauseItemIsOnChain(peer) && (peer.Type == "d_state_or_io_wait" || peer.Type == "io_wait") && item.DStateMs+item.IOWaitMs > 0
				}
				found := false
				for _, peer := range rank.Items {
					if matches(peer) {
						found = true
						break
					}
				}
				if !found {
					// Typed release arm: the counterpart must exist in the
					// pre-truncation pool AND the truncation must be disclosed.
					inPool := false
					for _, peer := range rank.preTruncationItems {
						if matches(peer) {
							inPool = true
							break
						}
					}
					compactionDisclosed := false
					for _, compaction := range rank.Compactions {
						if compaction.Dimension == CompactionDimensionCandidates && compaction.Total > compaction.Emitted {
							compactionDisclosed = true
						}
					}
					if !inPool || !compactionDisclosed {
						t.Fatalf("◇ remainder without a ⛓ counterpart on the board or in the disclosed pre-truncation pool (锚定值静默消失; inPool=%v compactionDisclosed=%v): %+v",
							inPool, compactionDisclosed, item)
					}
				}
			}
		} else {
			if math.Abs(published-item.ChainAnchoredMs) > 0.002 {
				t.Fatalf("⛓ clipped seat must publish exactly its anchored value: %+v", item)
			}
			if !rootCauseItemIsOnChain(item) {
				t.Fatalf("⛓ clipped seat must stay on the chain tier: %+v", item)
			}
		}
	}
}

// TestRSPAH2GoldenSelfSeatZeroPerturbation — the h2 golden-oracle proxy
// (先验后动 acceptance, matrix blast-radius clause): with the analysis TARGET
// = CompThread_0-2955 (the h2 eval case's exact shape), the target's OWN
// window D seat is self-causality (fully anchored by definition) and must
// stay byte-level untouched by the re-anchoring — full 36.757 account, the
// unanimous dma_fence caller word, no decomposition fields, chain tier.
func TestRSPAH2GoldenSelfSeatZeroPerturbation(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	rank := BuildRootCauseRank(idx, Query{PID: 2955, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	found := false
	for _, item := range rank.Items {
		if item.Thread.PID != 2955 || item.Type != "d_state_or_io_wait" || !strings.HasPrefix(item.Source, "window_stats") {
			continue
		}
		found = true
		if math.Abs(item.CumulativeImpactMs-36.757) > 0.002 || item.BlockedReasonCaller != "dma_fence_default_w" {
			t.Fatalf("h2 self D seat perturbed: %+v", item)
		}
		if item.ChainAnchorFullMs != 0 || item.ChainAnchoredMs != 0 || item.ChainAnchorRemainderSeat {
			t.Fatalf("h2 self D seat must carry no re-anchoring decomposition: %+v", item)
		}
		if !rootCauseItemIsOnChain(item) {
			t.Fatalf("h2 self D seat must keep the chain tier: %+v", item)
		}
	}
	if !found {
		t.Fatalf("h2 witness seat missing: %+v", rank.Items)
	}
}

// TestRSPAExemptFamiliesNeverMigrate — the 处置矩阵 negative pins (负向 pin
// 固化豁免判据): the re-anchoring switch is a CLOSED type set — the already-
// credential-anchored families (lock blocking_span, binder_wait, semantic
// span work, running supply-fold, sleep) pass through byte-identically even
// when the pid holds a migrable decision.
func TestRSPAExemptFamiliesNeverMigrate(t *testing.T) {
	chain := ChainResult{
		Target: ThreadRef{PID: 100},
		Nodes: []ChainNode{
			{Thread: ThreadRef{PID: 200}, Depth: 1, Window: TimeWindow{StartTs: 1, EndTs: 2}},
		},
		CausalImpacts: []WakeupCausalImpact{
			{Thread: ThreadRef{PID: 200}, ChainDepth: 1, RunnableMs: 5.0, DStateMs: 3.0},
		},
	}
	stats := WindowStats{
		chainAnchorsByPID:      chainAnchorWindowsByPID(chain),
		offCPUProducerDisjoint: true,
		runnableCensus: map[string]ThreadDuration{
			"200|0": {Thread: ThreadRef{PID: 200}, DurationMs: 8.0, anchoredMs: 5.0},
		},
		dstateCensus: map[string]ThreadDuration{
			"200|0": {Thread: ThreadRef{PID: 200}, DurationMs: 4.0, anchoredMs: 3.0},
		},
	}
	exempt := []RootCauseRankItem{
		{Type: "blocking_span", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain", BlockingKind: "mutex", ImpactMs: 9, CumulativeImpactMs: 9},
		{Type: "binder_wait", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ImpactMs: 9, CumulativeImpactMs: 9},
		{Type: "jit_compile", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ImpactMs: 9, CumulativeImpactMs: 9, SemanticClass: "jit_compile", SpanName: "JIT compiling x"},
		{Type: "running", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunning), RunningMs: 9, ImpactMs: 9, CumulativeImpactMs: 9, Source: "wakeup_chain.causal_impacts"},
		{Type: "sleep_wait", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateSSleep), SleepMs: 9, ImpactMs: 9, CumulativeImpactMs: 9, Source: "window_stats.sleep_top"},
		// EVOLUTION RECORD (RNB-1, §29.88 R4 排他通则, 2026-07-14): the
		// priority_inversion_runnable_wait row LEFT this exempt set — the
		// former "owns its gated algebra → fail-open by construction" carve
		// was an R4 escape (full-window runnable on the chain tier with no
		// edge credential). Its gated algebra stays untouched (values zero-
		// touch), but an unanchored share now demotes the LANE — pinned by
		// TestRSPAInversionRetypeLaneArm.
	}
	items := append([]RootCauseRankItem(nil), exempt...)
	items = reanchorOnChainStateSeats(chain, stats, items)
	if len(items) != len(exempt) {
		t.Fatalf("exempt families must not split: %d rows", len(items))
	}
	for i, item := range items {
		if item.ChainAnchorFullMs != 0 || item.ChainAnchoredMs != 0 || item.ChainAnchorRemainderSeat {
			t.Fatalf("exempt family %q gained decomposition fields: %+v", exempt[i].Type, item)
		}
		if item.ChainRelevance != "on_chain" || item.CumulativeImpactMs != exempt[i].CumulativeImpactMs {
			t.Fatalf("exempt family %q perturbed: %+v", exempt[i].Type, item)
		}
	}
}
