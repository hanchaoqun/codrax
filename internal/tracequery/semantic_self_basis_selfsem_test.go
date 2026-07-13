package tracequery

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// semantic_self_basis_selfsem_test.go — SELF-SEM engine pins (§29.61.1 user
// ruling 2026-07-13, RANK-U Stage 1 commit A; ledger
// docs/design/real_trace_campaign_20260705.md).
//
// Ruling shape: the analysis TARGET's own deterministic semantic span(s)
// inside the query window enter the ON-CHAIN channel on the typed self basis
// (OnChainBasis=self_deterministic_span, causality=self_deterministic) — no
// wakeup edge is minted and no cross-thread relation is claimed. §23.1 道别
// 红线 stays byte-identical for every NON-target thread (its protected
// witness is an other-process span — structurally different, ruling ③).
//
// Fixture red line (§29.53 产线实铸形): the positive witness family is
// engine-parsed and engine-minted from VERBATIM real donghu.ftrace rows (the
// Jit thread pool-17284 JIT-compile neighborhood, lines 5960-5990/12600-12660
// of eval/fixtures/real_traces/donghu.ftrace) — never hand-built rank items,
// never synthetic timestamps.
//
// MUTATION self-checks:
//   - M1 removing the self arm (selfDeterministicSemanticSpanLane consumers in
//     semanticSpanFamilyLane / the E2 fall-through, or the enrich keep arm in
//     rootCauseChainContextForItem) reds
//     TestSelfSemTargetOwnDeterministicSpanEntersOnChainChannel — the witness
//     family falls back to adjacent;
//   - M2 publishing the boosted effective (RankSortBoostedEffectiveMs) instead
//     of the window-projection union reds the same test's eff==cumulative
//     identity assertion (§29.7-2 乘子泄漏防线);
//   - re-deciding the lane at enrich time (deleting the keep arm) also reds
//     the positive pin, because enrichRootCauseItemsWithChainContext runs
//     inside BuildRootCauseRank on exactly this shape.

// selfSemDonghuJitTrace — VERBATIM rows from eval/fixtures/real_traces/
// donghu.ftrace (real HarmonyOS capture): Jit thread pool-17284 runs two real
// JIT-compile spans on its own running segments, sleeps in between
// (13762.849064 → 13762.895268) and is woken by forest_report_t-18029 — so a
// chain universe exists for target 17284 while the spans never overlap any
// chain node/impact window (they sit on the RUNNING segments).
const selfSemDonghuJitTrace = ` .ugc.aweme.lite-17267 (17267) [007] .... 13762.847111: sched_wakeup: comm=Jit thread pool pid=17284 prio=11 target_cpu=000
 Jit thread pool-17284 (17267) [000] .... 13762.847202: sched_switch: prev_comm=SaInit0 prev_pid=3132 prev_prio=20 prev_state=S ==> next_comm=Jit thread pool next_pid=17284 next_prio=11 next_info=3fff,85,2,0,0,0
 Jit thread pool-17284 (17267) [000] .... 13762.847253: print: B|37722|JIT compiling void android.widget.TextView$TextAppearanceAttributes.<init>() (kind=Baseline) from /system/framework/framework.jar!classes4.dex, dex_location=/system/framework/framework.jar!classes4.dex)
 Jit thread pool-17284 (17267) [000] .... 13762.847272: print: B|37722|Compiling baseline
 Jit thread pool-17284 (17267) [000] .... 13762.848735: print: B|37722|ScopedCodeCacheWrite
 Jit thread pool-17284 (17267) [000] .... 13762.848780: print: E|37722
 Jit thread pool-17284 (17267) [000] .... 13762.848884: print: B|37722|ScopedCodeCacheWrite
 Jit thread pool-17284 (17267) [000] .... 13762.848914: print: E|37722
 Jit thread pool-17284 (17267) [000] .... 13762.848953: print: E|37722
 Jit thread pool-17284 (17267) [000] .... 13762.848962: print: B|37722|TrimMaps
 Jit thread pool-17284 (17267) [000] .... 13762.848965: print: B|37722|virtual void art::MemMapArenaPool::TrimMaps()
 Jit thread pool-17284 (17267) [000] .... 13762.849021: print: E|37722
 Jit thread pool-17284 (17267) [000] .... 13762.849028: print: E|37722
 Jit thread pool-17284 (17267) [000] .... 13762.849034: print: E|37722
        hilogcat-9503  ( 9503) [000] .... 13762.849064: sched_switch: prev_comm=Jit thread pool prev_pid=17284 prev_prio=11 prev_state=S ==> next_comm=hilogcat next_pid=9503 next_prio=10 next_info=ffb,7,0,0,0,0
 forest_report_t-18029 (17267) [003] .... 13762.895255: sched_wakeup: comm=Jit thread pool pid=17284 prio=11 target_cpu=002
 Jit thread pool-17284 (17267) [002] .... 13762.895268: sched_switch: prev_comm=tppmgr-idle-2 prev_pid=0 prev_prio=-2 prev_state=R+ ==> next_comm=Jit thread pool next_pid=17284 next_prio=11 next_info=3fff,86,2,0,0,0
 Jit thread pool-17284 (17267) [002] .... 13762.895303: print: B|37722|JIT compiling void android.icu.impl.number.DecimalQuantity_DualStorageBCD.readIntToBcd(int) (kind=Baseline) from /apex/com.android.i18n/javalib/core-icu4j.jar, dex_location=/apex/com.android.i18n/javalib/core-icu4j.jar)
 Jit thread pool-17284 (17267) [002] .... 13762.895317: print: B|37722|Compiling baseline
 Jit thread pool-17284 (17267) [002] .... 13762.895781: print: B|37722|ScopedCodeCacheWrite
 Jit thread pool-17284 (17267) [002] .... 13762.895785: print: E|37722
 Jit thread pool-17284 (17267) [002] .... 13762.895826: print: B|37722|ScopedCodeCacheWrite
 Jit thread pool-17284 (17267) [002] .... 13762.895832: print: E|37722
 Jit thread pool-17284 (17267) [002] .... 13762.895855: print: E|37722
 Jit thread pool-17284 (17267) [002] .... 13762.895862: print: B|37722|TrimMaps
 Jit thread pool-17284 (17267) [002] .... 13762.895864: print: B|37722|virtual void art::MemMapArenaPool::TrimMaps()
 Jit thread pool-17284 (17267) [002] .... 13762.895900: print: E|37722
 Jit thread pool-17284 (17267) [002] .... 13762.895905: print: E|37722
 Jit thread pool-17284 (17267) [002] .... 13762.895910: print: E|37722
   tppmgr-idle-2-272   (    2) [002] .... 13762.895993: sched_switch: prev_comm=Jit thread pool prev_pid=17284 prev_prio=11 prev_state=S ==> next_comm=tppmgr-idle-2 next_pid=0 next_prio=-2 next_info=4,0,0,0,0,0
`

func selfSemDonghuQuery() Query {
	return Query{
		PID: 17284, TimeStart: 13762.846, TimeEnd: 13762.897,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12,
	}
}

// TestSelfSemTargetOwnDeterministicSpanEntersOnChainChannel is the positive
// witness pin (production-minted, donghu verbatim rows): the target's own
// JIT-compile family rides the on-chain channel on the typed self basis, with
// the window-projection union as BOTH the participation and the published
// effective (M2 arm), no fabricated overlap, and full election rights
// (ordinary ladder tier + a chain-channel ordinal, never BackgroundRank).
func TestSelfSemTargetOwnDeterministicSpanEntersOnChainChannel(t *testing.T) {
	idx := buildTraceIndex(t, "selfsem_donghu_jit.systrace", selfSemDonghuJitTrace)
	rank := BuildRootCauseRank(idx, selfSemDonghuQuery())
	var self *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "jit_compile" {
			self = &rank.Items[i]
			break
		}
	}
	if self == nil {
		// Fixture-activation fail-loud (§29.53 教训: 合成形静默失活):
		t.Fatalf("fixture drifted: no jit_compile rank row was minted: %+v", rank.Items)
	}
	if self.MemberCount != 2 {
		t.Fatalf("both real JIT spans must fold into ONE self family: %+v", self)
	}
	if self.ChainRelevance != "on_chain" || self.OnChainBasis != RootCauseOnChainBasisSelfDeterministicSpan {
		t.Fatalf("target self deterministic family must enter the on-chain channel on the typed self basis: %+v", self)
	}
	if self.Causality != RootCauseCausalitySelfDeterministic {
		t.Fatalf("self row must speak the honest causality token, never a wakeup-edge claim: %+v", self)
	}
	if self.OverlapMs != 0 {
		t.Fatalf("self basis must not fabricate a chain-window overlap: %+v", self)
	}
	// 口径 (§29.61.1 ②): participation == published effective == the complete
	// window-projection member union — and the deterministic hidden-cost boost
	// never publishes (M2 §29.7-2 乘子泄漏防线).
	if self.EffectiveImpactMs <= 0 || self.EffectiveImpactMs != self.CumulativeImpactMs {
		t.Fatalf("self family must publish the window-projection union as its effective: %+v", self)
	}
	if self.ImpactMs != self.EffectiveImpactMs {
		t.Fatalf("participation and published effective must be ONE value (no boost leak): %+v", self)
	}
	// 全权参赛 (§29.61.1 自然读法): ordinary election ladder + chain-channel
	// ordinal; never the background composite board.
	switch self.Tier {
	case "primary", "secondary", "tertiary":
	default:
		t.Fatalf("self row must walk the ordinary election ladder: %+v", self)
	}
	if self.Rank <= 0 || self.BackgroundRank != 0 {
		t.Fatalf("self row takes a chain-channel ordinal and no background seat: %+v", self)
	}
	// 冕格默认 pin (件5, 设计裁定④/SYM-2 延伸, 复核 F4 2026-07-13): in this
	// fixture the self family's eff tops the board — the row is CROWNABLE by
	// strict position (Rank=1, tier=primary; no demotion arm may eat a
	// board-topping self deterministic row). 主会话可改判默认形 — this pin
	// freezes today's ruling-④ default.
	if self.Rank != 1 || self.Tier != "primary" {
		t.Fatalf("冕格默认: the board-topping self family must hold Rank=1/primary: rank=%d tier=%s", self.Rank, self.Tier)
	}
	// 不铸唤醒边: the chain's edge set carries no edge minted BY this row —
	// the only edge is the real forest_report_t wakeup of the target.
	if strings.Contains(self.Summary, "wakeup-chain interval") {
		t.Fatalf("self row summary must not claim a wakeup-chain interval: %q", self.Summary)
	}
}

// selfSemNonTargetTrace — production-shape rows (donghu emission forms): the
// target app-100 runs, then sleeps 5.003→5.0065 and is woken by verify-200 (a
// REAL chain node whose chain-node window is the target's sleep interval);
// verify-200's own VerifyClass span (5.0005..5.0025) sits BEFORE that window
// (no overlap — the huadong E21 shape); other-300 belongs to a different
// process with a JIT span and never touches the chain.
const selfSemNonTargetTrace = `        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52 next_info=3fff,85,2,0,0,0
      verify-200 ( 200) [002] .... 5.000400: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=verify next_pid=200 next_prio=20 next_info=3fff,85,2,0,0,0
      verify-200 ( 200) [002] .... 5.000500: print: B|200|VerifyClass com.example.app.MainActivity
      verify-200 ( 200) [002] .... 5.002500: print: E|200
        app-100 (100) [001] .... 5.003000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      verify-200 ( 200) [002] .... 5.006000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.006500: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52 next_info=3fff,85,2,0,0,0
       other-300 ( 300) [003] .... 5.007000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=300 next_prio=20 next_info=3fff,85,2,0,0,0
       other-300 ( 300) [003] .... 5.007100: print: B|300|JIT compiling void com.other.Worker.run() (kind=Baseline) from /data/app/other.jar, dex_location=/data/app/other.jar)
       other-300 ( 300) [003] .... 5.008600: print: E|300
        app-100 (100) [001] .... 5.009000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`

// TestSelfSemNonTargetLanesUntouched pins the §23.1 negative forms N1/N2:
//   - N1 (huadong E21 形): a NON-target chain-node thread's deterministic span
//     without chain-window overlap stays ADJACENT — no self basis, no self
//     causality (道别红线原文不动);
//   - N2 (cmp_01 E2 形): an other-process compile span with no chain
//     membership keeps its non-chain lane byte-identically (background board,
//     BackgroundRank stamped).
func TestSelfSemNonTargetLanesUntouched(t *testing.T) {
	idx := buildTraceIndex(t, "selfsem_nontarget.systrace", selfSemNonTargetTrace)
	q := Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
	rank := BuildRootCauseRank(idx, q)
	var verify, other *RootCauseRankItem
	for i := range rank.Items {
		switch {
		case rank.Items[i].Type == "class_verification" && rank.Items[i].Thread.PID == 200:
			verify = &rank.Items[i]
		case rank.Items[i].Type == "jit_compile" && rank.Items[i].Thread.PID == 300:
			other = &rank.Items[i]
		}
	}
	if verify == nil || other == nil {
		t.Fatalf("fixture drifted: both semantic rows must mint: %+v", rank.Items)
	}
	// N1: chain-node thread, no overlap → adjacent, never the self basis.
	if verify.ChainRelevance != "adjacent" || verify.OnChainBasis != "" {
		t.Fatalf("N1: a NON-target chain-node span must stay adjacent with no self basis: %+v", verify)
	}
	if verify.Causality == RootCauseCausalitySelfDeterministic {
		t.Fatalf("N1: the self causality token must never mint on a non-target row: %+v", verify)
	}
	// N2: other-process span off the chain → background lane unchanged.
	if other.ChainRelevance != "background" || other.OnChainBasis != "" || other.BackgroundRank <= 0 {
		t.Fatalf("N2: an other-process compile span keeps the background lane: %+v", other)
	}
}

// selfSemSelfNonSemanticTrace — the target's own span whose name only
// NEAR-misses the deterministic closed set ("verify" vocabulary without a
// classifier match): the closed-set arm must discriminate (N3).
const selfSemSelfNonSemanticTrace = `        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52 next_info=3fff,85,2,0,0,0
        app-100 (100) [001] .... 5.000200: print: B|100|verify metadata cache refresh
        app-100 (100) [001] .... 5.002200: print: E|100
        app-100 (100) [001] .... 5.003000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-500 ( 500) [002] .... 5.008000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.008500: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52 next_info=3fff,85,2,0,0,0
`

// TestSelfSemSelfNonDeterministicSpanUntouched (N3): the target's OWN span
// outside the deterministic closed set never takes the self basis — it stays
// a generic trace_span row (the near-miss vocabulary earns the advisory
// caveat only, never a lane).
func TestSelfSemSelfNonDeterministicSpanUntouched(t *testing.T) {
	idx := buildTraceIndex(t, "selfsem_nonsemantic.systrace", selfSemSelfNonSemanticTrace)
	q := Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.009, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
	// Fixture-activation fail-loud: the span must reach the classifier as an
	// UNclassified span (near-miss vocabulary, closed set discriminates).
	stats := ComputeWindowStats(idx, q)
	sawSpan := false
	for _, span := range stats.TraceSpans {
		if span.Name == "verify metadata cache refresh" {
			sawSpan = true
			if span.SemanticClass != "" {
				t.Fatalf("N3: the near-miss name must stay unclassified: %+v", span)
			}
		}
	}
	if !sawSpan {
		t.Fatalf("fixture drifted: the self span must reach window stats: %+v", stats.TraceSpans)
	}
	rank := BuildRootCauseRank(idx, q)
	rows := append(append([]RootCauseRankItem{}, rank.Items...), rank.AbsorbedItems...)
	for _, item := range rows {
		if item.OnChainBasis != "" || item.Causality == RootCauseCausalitySelfDeterministic {
			t.Fatalf("N3: no row of this shape may carry the self basis: %+v", item)
		}
		if rootCauseItemIsSemanticSpanWork(item) {
			t.Fatalf("N3: a near-miss name must not classify into the semantic closed set: %+v", item)
		}
	}
	// The advisory near-miss caveat is the only vocabulary consequence.
	if !strings.Contains(strings.Join(rank.Caveats, "\n"), "verify metadata cache refresh") {
		t.Fatalf("N3: the near-miss advisory caveat must disclose the drifted name: %q", rank.Caveats)
	}
}

// TestSelfSemUntargetedRankMintsNoSelfBasis (N4): an untargeted window-stats
// rank has no chain universe and no resolved target — absence never guesses:
// the deterministic span keeps today's non-chain lane, zero self basis.
func TestSelfSemUntargetedRankMintsNoSelfBasis(t *testing.T) {
	idx := buildTraceIndex(t, "selfsem_untargeted.systrace", selfSemDonghuJitTrace)
	q := Query{TimeStart: 13762.846, TimeEnd: 13762.897, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
	rank := BuildRootCauseRank(idx, q)
	found := false
	for _, item := range rank.Items {
		if item.OnChainBasis != "" || item.Causality == RootCauseCausalitySelfDeterministic {
			t.Fatalf("N4: untargeted rank must never mint the self basis: %+v", item)
		}
		if item.Type == "jit_compile" {
			found = true
			if rootCauseItemIsOnChain(item) {
				t.Fatalf("N4: chainless deterministic work stays off the on-chain channel: %+v", item)
			}
		}
	}
	if !found {
		t.Fatalf("fixture drifted: the jit_compile family must still mint: %+v", rank.Items)
	}
}

// selfSemTrueOverlapTrace — the target's own GC-pause span BRACKETS its sleep
// segment (B before the S switch, E after the wakeup), so the span truly
// overlaps the target's chain-node window: the legacy intersection caliber
// must keep this row byte-identically (N5 — the self arm only accepts the
// no-overlap fall-through).
const selfSemTrueOverlapTrace = `        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52 next_info=3fff,85,2,0,0,0
        app-100 (100) [001] .... 5.000200: print: B|100|GC pause young
        app-100 (100) [001] .... 5.001000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-500 ( 500) [002] .... 5.006000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.006400: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52 next_info=3fff,85,2,0,0,0
        app-100 (100) [001] .... 5.007400: print: E|100
        app-100 (100) [001] .... 5.008000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`

// TestSelfSemTrueOverlapKeepsIntersectionCaliber (N5): a target self
// deterministic span WITH a genuine chain-window overlap walks the legacy
// intersection lane byte-identically — overlap basis (OnChainBasis empty),
// wakeup-chain causality, participation strictly below the full span union.
func TestSelfSemTrueOverlapKeepsIntersectionCaliber(t *testing.T) {
	idx := buildTraceIndex(t, "selfsem_overlap.systrace", selfSemTrueOverlapTrace)
	q := Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.009, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
	rank := BuildRootCauseRank(idx, q)
	var gc *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "gc_pause" {
			gc = &rank.Items[i]
			break
		}
	}
	if gc == nil {
		t.Fatalf("fixture drifted: the gc_pause row must mint: %+v", rank.Items)
	}
	if gc.ChainRelevance != "on_chain" || gc.Causality != "on_wakeup_chain" {
		t.Fatalf("N5: a truly overlapping self span keeps the legacy overlap lane: %+v", gc)
	}
	if gc.OnChainBasis != "" {
		t.Fatalf("N5: the self basis must never stamp an overlap-lane row: %+v", gc)
	}
	// Participation = the exact span∩chain-window intersection (the bracketed
	// sleep segment), strictly below the physical span extent — the self arm
	// never widens an overlap row back to the full union.
	if gc.EffectiveImpactMs <= 0 || gc.EffectiveImpactMs >= gc.ActualImpactMs {
		t.Fatalf("N5: participation must stay the exact intersection (< physical span extent): %+v", gc)
	}
}

// TestSelfSemDonghuJitRealTraceWitness — the W1 proxy witness on the REAL
// in-repo customer capture (donghu.ftrace; the VerifyClass customer trace is
// not at hand — 主会话裁定: donghu's JIT semantic span form is the proxy, the
// ◇ 区 JIT row must rise on-chain). Channel-flip 实录 (2026-07-13, baseline
// e7ec21bd vs this batch, same fixture/query):
//
//	BASELINE: jit_compile ×2 family → ChainRelevance=adjacent,
//	          causality=adjacent_to_wakeup_chain, tier=tertiary,
//	          邻近影响#1 (adjacent-channel ordinal), background_rank=1,
//	          eff=2.388 (window union)
//	SELF-SEM: → ChainRelevance=on_chain, on_chain_basis=self_deterministic_span,
//	          causality=self_deterministic, tier=secondary, 根因排序#2,
//	          background_rank=0, eff=2.388 — the VALUE is unchanged (R13 携值
//	          合法: the union is the target thread's own in-window wall clock),
//	          only the channel identity flipped.
func TestSelfSemDonghuJitRealTraceWitness(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	rank := BuildRootCauseRank(idx, Query{PID: 17284, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	var jit *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "jit_compile" {
			jit = &rank.Items[i]
			break
		}
	}
	if jit == nil {
		t.Fatalf("real-trace drifted: the Jit thread pool family must mint: %+v", rank.Items)
	}
	if jit.MemberCount != 2 || !jit.SubjectIsAnalysisTarget {
		t.Fatalf("witness family shape drifted (2 real JIT spans on the target): %+v", jit)
	}
	if jit.ChainRelevance != "on_chain" || jit.OnChainBasis != RootCauseOnChainBasisSelfDeterministicSpan ||
		jit.Causality != RootCauseCausalitySelfDeterministic {
		t.Fatalf("the ◇ 区 JIT family must rise to the on-chain channel on the self basis: %+v", jit)
	}
	if jit.Rank != 2 || jit.Tier != "secondary" || jit.BackgroundRank != 0 {
		t.Fatalf("witness seat drifted (根因排序#2 · secondary · no background seat): rank=%d tier=%s bg=%d", jit.Rank, jit.Tier, jit.BackgroundRank)
	}
	// R13 携值: the published effective IS the baseline's window union — the
	// channel flip carries the value, never re-prices it (and never leaks the
	// internal boost).
	if got := fmt.Sprintf("%.3f", jit.EffectiveImpactMs); got != "2.388" || jit.EffectiveImpactMs != jit.CumulativeImpactMs {
		t.Fatalf("published effective must stay the 2.388ms window union: eff=%s cum=%.3f", got, jit.CumulativeImpactMs)
	}
	if jit.OverlapMs != 0 {
		t.Fatalf("no fabricated chain-window overlap on the self basis: %+v", jit)
	}
}

// TestSelfSemSharedPredicateConditions unit-pins the four typed admission
// conditions of the ONE shared predicate (§2.1 单一实现): chain universe,
// resolved target, tid-first self identity, deterministic closed set +
// positive extent. Removing any condition reds its arm.
func TestSelfSemSharedPredicateConditions(t *testing.T) {
	span := TraceSpanSummary{
		Name: "VerifyClass com.example.Foo", Thread: ThreadRef{Comm: "app", PID: 100},
		StartTs: 5.0, EndTs: 5.002, DurationMs: 2.0,
	}
	chain := &ChainResult{
		Target: ThreadRef{Comm: "app", PID: 100},
		Nodes:  []ChainNode{{Thread: ThreadRef{Comm: "app", PID: 100}, Window: TimeWindow{StartTs: 5.003, EndTs: 5.006}}},
	}
	if !selfDeterministicSemanticSpanLane(chain, span) {
		t.Fatalf("full condition set must admit the self lane")
	}
	if selfDeterministicSemanticSpanLane(nil, span) {
		t.Fatalf("nil chain must never admit")
	}
	if selfDeterministicSemanticSpanLane(&ChainResult{Target: chain.Target}, span) {
		t.Fatalf("an EMPTY chain universe must never admit (no chain, no on-chain identity)")
	}
	unresolved := &ChainResult{Nodes: chain.Nodes}
	if selfDeterministicSemanticSpanLane(unresolved, span) {
		t.Fatalf("an unresolved target must never admit (absence never guesses)")
	}
	otherThread := span
	otherThread.Thread = ThreadRef{Comm: "peer", PID: 200}
	if selfDeterministicSemanticSpanLane(chain, otherThread) {
		t.Fatalf("a non-target thread must never admit (§23.1 道别红线)")
	}
	nonSemantic := span
	nonSemantic.Name = "verify metadata cache refresh"
	if selfDeterministicSemanticSpanLane(chain, nonSemantic) {
		t.Fatalf("a near-miss name outside the closed set must never admit")
	}
	zero := span
	zero.DurationMs = 0
	zero.EndTs = zero.StartTs
	if selfDeterministicSemanticSpanLane(chain, zero) {
		t.Fatalf("a zero-extent span must never admit")
	}
}
