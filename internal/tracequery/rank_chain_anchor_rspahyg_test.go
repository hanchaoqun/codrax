package tracequery

// rank_chain_anchor_rspahyg_test.go — RSPA-HYG batch pins (§29.77 立案六件,
// 2026-07-14): the mechanical hygiene follow-ups of the RSPA re-anchoring.
//
//	件②  单 Run 恰一次 sweep — a mixed-view root_cause_rank Run performs
//	     exactly ONE ComputeWindowStats (the anchored sweep backfeeds the
//	     shared memo; exported faces re-verified byte-identical against a
//	     plain window_stats Run).
//	件③  io facet host-form credential narrowing — io_burst_episode /
//	     block_io_by_inode keep the chain lane only when their typed interval
//	     is CONTAINED in the thread's anchor-window union; partial overlap
//	     demotes to ◇ (values untouched). Witness: tieba ThreadPoolForeg-60555
//	     block_io envelope 61.540ms with 24.568ms inside. io_burst_episode has
//	     NO on-chain partial production instance in the in-repo traces
//	     (donghu board carries only the target-self block_io row; tieba mints
//	     no io_burst rank row in the witness window) — the arm is covered by
//	     the synthetic unit fixture below, 如实注.
//	件④  reanchorOnChainStateSeats two-pass byte idempotence (the production
//	     pipeline runs the pass in BOTH the build and enrich lanes).
//	修复轮  rspaRemainderSummary arithmetic pin (anchored/full Sprintf slots
//	     were swapped since the RSPA batch; typed fields were always correct).
//
// MUTATION self-checks:
//   - dropping the getStatsForRank backfeed or reverting the root_cause_rank
//     case to getStats() reds 件② (probe counts 2);
//   - dropping the containment demote arm in rootCauseChainContextForItem
//     reds 件③ (tieba block_io rides the chain tier again);
//   - 件④ (witnessed 2026-07-14): removing the migrated-row skip guard ALONE
//     is absorbed — the lane exclusion (migrated rows leave the on-chain
//     lane) and the census value-identity gates independently block
//     re-processing (defense-in-depth, actually run); the pin reds under the
//     compound mutation skip-guard removal + satellite scalar-identity
//     removal + lane-gate bypass (actually run, DeepEqual red).

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

// --- 件② 单 Run 恰一次 sweep --------------------------------------------------

func TestRSPAHygSingleSweepPerRankRun(t *testing.T) {
	idx := rspaCaseATrace(t)
	sweeps := 0
	res := Run(idx, Query{View: "root_cause_rank", PID: 100, TimeStart: 1.0, TimeEnd: 1.07,
		MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12,
		statsSweepProbe: &sweeps})
	if res.RootCauseRank == nil || res.WindowStats == nil || res.SchedulerLatency == nil {
		t.Fatalf("mixed-view rank run must publish rank+stats+latency faces: %+v", res.View)
	}
	// Non-vacuous guard: the anchored lane actually ran (the fixture's worker
	// seat carries the re-anchoring decomposition).
	anchoredSeen := false
	for _, item := range append(append([]RootCauseRankItem{}, res.RootCauseRank.Items...), res.RootCauseRank.AbsorbedItems...) {
		if item.ChainAnchorFullMs > 0 {
			anchoredSeen = true
		}
	}
	if !anchoredSeen {
		t.Fatalf("fixture drifted: the rank run must exercise the anchored sweep")
	}
	if sweeps != 1 {
		t.Fatalf("件②: a mixed-view rank Run must perform exactly ONE ComputeWindowStats, got %d", sweeps)
	}
	// Exported-face isomorphism re-verification: the anchored sweep published
	// as the window_stats face must be byte-identical (exported JSON) to a
	// plain window_stats Run's face.
	plainSweeps := 0
	plain := Run(idx, Query{View: "window_stats", PID: 100, TimeStart: 1.0, TimeEnd: 1.07,
		MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12,
		statsSweepProbe: &plainSweeps})
	if plainSweeps != 1 {
		t.Fatalf("plain window_stats Run must stay a single sweep, got %d", plainSweeps)
	}
	rankFace, err := json.Marshal(res.WindowStats)
	if err != nil {
		t.Fatal(err)
	}
	plainFace, err := json.Marshal(plain.WindowStats)
	if err != nil {
		t.Fatal(err)
	}
	if string(rankFace) != string(plainFace) {
		t.Fatalf("anchored-sweep window_stats face must be byte-identical to the plain sweep's:\nrank:  %s\nplain: %s", rankFace, plainFace)
	}
}

// The chainless shape keeps the plain shared memo (still exactly one sweep;
// no anchored lane, no decomposition).
func TestRSPAHygSingleSweepChainlessRankRun(t *testing.T) {
	idx := rspaCaseATrace(t)
	sweeps := 0
	res := Run(idx, Query{View: "root_cause_rank", TimeStart: 1.0, TimeEnd: 1.07,
		MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12,
		statsSweepProbe: &sweeps})
	if res.RootCauseRank == nil {
		t.Fatalf("chainless rank run must still publish the rank face")
	}
	if sweeps != 1 {
		t.Fatalf("chainless rank Run must also stay a single sweep, got %d", sweeps)
	}
	for _, item := range res.RootCauseRank.Items {
		if item.ChainAnchorFullMs > 0 || item.ChainAnchorRemainderSeat {
			t.Fatalf("chainless run must not mint decomposition: %+v", item)
		}
	}
}

// --- 件③ io facet host-form containment --------------------------------------

// Unit arm (mirrors TestRSPAResourceLaneClosureArm): a partially contained
// non-target io_burst/block_io row demotes to ◇; contained rows, target self
// rows and anchor-less (unevaluated) rows keep the chain lane.
//
// 如实注: io_burst_episode has no on-chain partial-overlap production instance
// in the in-repo traces — this synthetic fixture is its arm coverage; the
// block_io_by_inode half has the tieba production witness pin below.
func TestRSPAHygIOFacetContainmentArmUnit(t *testing.T) {
	target := ThreadRef{PID: 100}
	ctx := chainCandidateContext{relevance: "on_chain", overlapMs: 3}
	// EVOLUTION RECORD (RSPA-HYG 残余批, §29.83 残余③, 2026-07-14): the loop
	// originally covered the 立案③ pair only; file_io_hot_inode /
	// workqueue_activity / dma_fence_activity joined the containment arm after
	// the per-edge audit (dispositions on stampResourceClosureEvaluation).
	// 如实注: none of the three has an on-chain production instance in the
	// flagship windows (donghu 17267 / tieba 59566 probes, 2026-07-14) — this
	// synthetic loop is their arm coverage.
	for _, typ := range []string{"io_burst_episode", "block_io_by_inode",
		"file_io_hot_inode", "workqueue_activity", "dma_fence_activity"} {
		base := RootCauseRankItem{Type: typ, Thread: ThreadRef{PID: 200}, StartTs: 1, EndTs: 2}

		partial := base
		partial.resourceHostContainmentEvaluated = true
		if got := rootCauseChainContextForItem(partial, ctx, target); got.relevance != "adjacent" {
			t.Fatalf("%s: evaluated partially-contained row must demote to ◇: %+v", typ, got)
		}
		contained := partial
		contained.resourceHostWindowContained = true
		if got := rootCauseChainContextForItem(contained, ctx, target); got.relevance != "on_chain" {
			t.Fatalf("%s: contained interval keeps the host-form chain credential: %+v", typ, got)
		}
		self := partial
		self.Thread = target
		if got := rootCauseChainContextForItem(self, ctx, target); got.relevance != "on_chain" {
			t.Fatalf("%s: the target's own row is self-causality exempt: %+v", typ, got)
		}
		if got := rootCauseChainContextForItem(base, ctx, target); got.relevance != "on_chain" {
			t.Fatalf("%s: anchor-less (unevaluated) rows keep the legacy overlap lane: %+v", typ, got)
		}
	}
	// page_cache_churn — 应豁免 (§29.83 残余③ per-edge audit): structurally
	// excluded from the rootCauseTypeCanBeDirectOnChain closed list, so the
	// fall-through arm demotes it regardless of any containment bit (its value
	// is a synthetic churn-count score, never a wall-clock host account).
	churn := RootCauseRankItem{Type: "page_cache_churn", Thread: ThreadRef{PID: 200}, StartTs: 1, EndTs: 2}
	if got := rootCauseChainContextForItem(churn, ctx, target); got.relevance != "adjacent" {
		t.Fatalf("page_cache_churn can never hold the chain tier (closed direct-on-chain list): %+v", got)
	}
	if rootCauseTypeCanBeDirectOnChain("page_cache_churn") {
		t.Fatal("page_cache_churn must stay off the direct-on-chain closed list (应豁免 structural evidence)")
	}
}

// Stamp unit: the mint-time containment verdict — interval ⊆ anchor-window
// union within µs tolerance; interval-less rows stay unevaluated; io_latency
// keeps its own closure credential and never carries the containment bits.
func TestRSPAHygIOFacetContainmentStamp(t *testing.T) {
	stats := WindowStats{
		chainAnchorsByPID: map[int][]TimeWindow{
			200: {{StartTs: 1.0, EndTs: 1.05}},
		},
	}
	items := []RootCauseRankItem{
		{Type: "io_burst_episode", Thread: ThreadRef{PID: 200}, StartTs: 1.01, EndTs: 1.04},  // contained
		{Type: "block_io_by_inode", Thread: ThreadRef{PID: 200}, StartTs: 1.02, EndTs: 1.30}, // partial
		{Type: "block_io_by_inode", Thread: ThreadRef{PID: 200}},                             // interval-less
		{Type: "io_latency", Thread: ThreadRef{PID: 200}, StartTs: 1.01, EndTs: 1.04},        // closure lane only
		{Type: "io_burst_episode", Thread: ThreadRef{PID: 999}, StartTs: 1.01, EndTs: 1.04},  // no windows → not contained
	}
	stampResourceClosureEvaluation(stats, items)
	if !items[0].resourceHostContainmentEvaluated || !items[0].resourceHostWindowContained {
		t.Fatalf("contained interval must stamp contained=true: %+v", items[0])
	}
	if !items[1].resourceHostContainmentEvaluated || items[1].resourceHostWindowContained {
		t.Fatalf("partial interval must stamp contained=false: %+v", items[1])
	}
	if items[2].resourceHostContainmentEvaluated {
		t.Fatalf("interval-less row must stay unevaluated (typed-interval arm owns it): %+v", items[2])
	}
	if items[3].resourceHostContainmentEvaluated {
		t.Fatalf("io_latency owns the closure credential, never the containment bits: %+v", items[3])
	}
	if !items[4].resourceHostContainmentEvaluated || items[4].resourceHostWindowContained {
		t.Fatalf("a pid without anchor windows can never prove containment: %+v", items[4])
	}
	// §29.83 残余③ facets: the three newly covered host-form facets carry the
	// same mint-time verdict; page_cache_churn (应豁免) never wears the bits.
	extended := []RootCauseRankItem{
		{Type: "file_io_hot_inode", Thread: ThreadRef{PID: 200}, StartTs: 1.01, EndTs: 1.04},  // contained
		{Type: "workqueue_activity", Thread: ThreadRef{PID: 200}, StartTs: 1.02, EndTs: 1.30}, // partial
		{Type: "dma_fence_activity", Thread: ThreadRef{PID: 200}, StartTs: 1.02, EndTs: 1.30}, // partial
		{Type: "page_cache_churn", Thread: ThreadRef{PID: 200}, StartTs: 1.01, EndTs: 1.04},   // exempt facet
		{Type: "workqueue_activity", Thread: ThreadRef{PID: 200}},                             // interval-less
	}
	stampResourceClosureEvaluation(stats, extended)
	if !extended[0].resourceHostContainmentEvaluated || !extended[0].resourceHostWindowContained {
		t.Fatalf("contained file_io_hot_inode must stamp contained=true: %+v", extended[0])
	}
	if !extended[1].resourceHostContainmentEvaluated || extended[1].resourceHostWindowContained {
		t.Fatalf("partial workqueue_activity must stamp contained=false: %+v", extended[1])
	}
	if !extended[2].resourceHostContainmentEvaluated || extended[2].resourceHostWindowContained {
		t.Fatalf("partial dma_fence_activity must stamp contained=false: %+v", extended[2])
	}
	if extended[3].resourceHostContainmentEvaluated {
		t.Fatalf("page_cache_churn is 应豁免 — the exempt facet never wears the containment bits: %+v", extended[3])
	}
	if extended[4].resourceHostContainmentEvaluated {
		t.Fatalf("interval-less workqueue row must stay unevaluated: %+v", extended[4])
	}
	// Anchor-less sweep: nothing stamps.
	bare := []RootCauseRankItem{{Type: "io_burst_episode", Thread: ThreadRef{PID: 200}, StartTs: 1.01, EndTs: 1.04}}
	stampResourceClosureEvaluation(WindowStats{}, bare)
	if bare[0].resourceClosureEvaluated || bare[0].resourceHostContainmentEvaluated {
		t.Fatalf("anchor-less build must not stamp: %+v", bare[0])
	}
}

// Production witness (tieba 59566 window): the ThreadPoolForeg-60555
// block_io_by_inode envelope (61.540ms, 24.568ms inside its dependency
// windows) demotes to the ◇ adjacent lane with its composite value untouched.
func TestRSPAHygTiebaBlockIOPartialOverlapDemotes(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatal(err)
	}
	rank := BuildRootCauseRank(idx, Query{PID: 59566, TimeStart: 34579.472865, TimeEnd: 34579.587805,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	found := false
	for _, item := range append(append([]RootCauseRankItem{}, rank.Items...), rank.AbsorbedItems...) {
		if item.Thread.PID != 60555 || item.Type != "block_io_by_inode" {
			continue
		}
		found = true
		if item.ChainRelevance != "adjacent" || rootCauseItemIsOnChain(item) {
			t.Fatalf("partial-overlap block_io must ride the ◇ adjacent lane: %+v", item)
		}
		if math.Abs(item.CumulativeImpactMs-4.262) > 0.002 {
			t.Fatalf("the demotion must never touch the published value: %+v", item)
		}
	}
	if !found {
		t.Fatalf("tieba witness row missing: %+v", rank.Items)
	}
}

// Production negative witness (donghu 17267 window): the analysis TARGET's own
// block_io_by_inode row is self-causality exempt and keeps the chain lane.
func TestRSPAHygDonghuTargetSelfBlockIOKeepsChainLane(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	rank := BuildRootCauseRank(idx, Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	found := false
	for _, item := range append(append([]RootCauseRankItem{}, rank.Items...), rank.AbsorbedItems...) {
		if item.Thread.PID != 17267 || item.Type != "block_io_by_inode" {
			continue
		}
		found = true
		if !rootCauseItemIsOnChain(item) {
			t.Fatalf("target-self block_io must keep the chain lane (self exemption): %+v", item)
		}
		if math.Abs(item.CumulativeImpactMs-2.694) > 0.002 {
			t.Fatalf("target-self block_io value drifted: %+v", item)
		}
	}
	if !found {
		t.Fatalf("donghu target-self witness row missing")
	}
}

// --- 件④ two-pass byte idempotence --------------------------------------------

// The production pipeline runs reanchorOnChainStateSeats in BOTH the build and
// enrich lanes; migrated rows carry the typed decomposition fields and must
// pass through the second run byte-identically (no re-migration, no second ◇
// twin, no value drift).
func TestRSPAHygReanchorTwoPassByteIdempotent(t *testing.T) {
	chain := ChainResult{
		Target: ThreadRef{PID: 100},
		Nodes: []ChainNode{
			{Thread: ThreadRef{PID: 200}, Depth: 1, Window: TimeWindow{StartTs: 1.0, EndTs: 1.005}},
			{Thread: ThreadRef{PID: 300}, Depth: 1, Window: TimeWindow{StartTs: 2.0, EndTs: 2.004}},
			{Thread: ThreadRef{PID: 400}, Depth: 1, Window: TimeWindow{StartTs: 3.0, EndTs: 3.006}},
		},
		CausalImpacts: []WakeupCausalImpact{
			{Thread: ThreadRef{PID: 200}, ChainDepth: 1, RunnableMs: 5.0},
			{Thread: ThreadRef{PID: 300}, ChainDepth: 1, RunnableMs: 4.0},
			{Thread: ThreadRef{PID: 400}, ChainDepth: 1, DStateMs: 2.0, IOWaitMs: 1.0},
		},
	}
	stats := WindowStats{
		chainAnchorsByPID:      chainAnchorWindowsByPID(chain),
		offCPUProducerDisjoint: true,
		runnableCensus: map[string]ThreadDuration{
			"200|0": {Thread: ThreadRef{PID: 200}, DurationMs: 8.0, anchoredMs: 5.0},
			"300|0": {Thread: ThreadRef{PID: 300}, DurationMs: 10.0, anchoredMs: 4.0},
		},
		dstateCensus: map[string]ThreadDuration{
			"400|0": {Thread: ThreadRef{PID: 400}, DurationMs: 2.5, anchoredMs: 2.0},
		},
		iowaitCensus: map[string]ThreadDuration{
			"400|0": {Thread: ThreadRef{PID: 400}, DurationMs: 1.5, anchoredMs: 1.0},
		},
	}
	items := []RootCauseRankItem{
		// case B split (no chain seat for 200): clip 5 + ◇ twin 3.
		{Type: "runnable_wait", Thread: ThreadRef{PID: 200}, Source: "window_stats",
			Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
			RunnableMs: 8.0, ImpactMs: 8.0, CumulativeImpactMs: 8.0, EffectiveImpactMs: 8.0, Confidence: 0.8},
		// chain seat for 300 (case A presence).
		{Type: "runnable_wait", Thread: ThreadRef{PID: 300}, Source: "wakeup_chain.causal_impacts",
			Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
			RunnableMs: 4.0, ImpactMs: 4.0, CumulativeImpactMs: 4.0, EffectiveImpactMs: 4.0, Confidence: 0.8},
		// case A window seat for 300: rewrites to the ◇ remainder 6.
		{Type: "runnable_wait", Thread: ThreadRef{PID: 300}, Source: "window_stats",
			Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
			RunnableMs: 10.0, ImpactMs: 10.0, CumulativeImpactMs: 10.0, EffectiveImpactMs: 10.0, Confidence: 0.8},
		// partially anchored scheduler satellite for 300: ◇ rewrite.
		{Type: "scheduler_latency", Thread: ThreadRef{PID: 300}, Source: "scheduler_latency_stats",
			Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
			RunnableMs: 20.0, ImpactMs: 20.0, CumulativeImpactMs: 20.0, EffectiveImpactMs: 20.0,
			StartTs: 2.002, EndTs: 2.022, Confidence: 0.8},
		// fragmented churn twin for 300: ◇ rewrite.
		{Type: "fragmented_runnable_wait", Thread: ThreadRef{PID: 300}, Source: "window_stats.state_churn",
			Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
			RunnableMs: 10.0, ImpactMs: 10.0, CumulativeImpactMs: 10.0, EffectiveImpactMs: 10.0, Confidence: 0.8},
		// D-IO case B split for 400 (ledger-stamped).
		{Type: "d_state_or_io_wait", Thread: ThreadRef{PID: 400}, Source: "window_stats",
			Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateDSleep),
			DStateMs: 2.5, IOWaitMs: 1.5, ImpactMs: 4.0, CumulativeImpactMs: 4.0, EffectiveImpactMs: 4.0,
			ledgerAnchorStamped: true, ledgerAnchoredDMs: 2.0, ledgerAnchoredIOMs: 1.0, Confidence: 0.8},
	}
	pass1 := reanchorOnChainStateSeats(chain, stats, append([]RootCauseRankItem(nil), items...))
	if len(pass1) != len(items)+2 {
		t.Fatalf("fixture drifted: two case-B ◇ twins expected (200 runnable + 400 D-IO), got %d rows from %d", len(pass1), len(items))
	}
	migrated := 0
	for _, item := range pass1 {
		if item.ChainAnchorFullMs > 0 {
			migrated++
		}
	}
	if migrated < 6 {
		t.Fatalf("fixture drifted: expected ≥6 decomposition-carrying rows, got %d", migrated)
	}
	pass2 := reanchorOnChainStateSeats(chain, stats, append([]RootCauseRankItem(nil), pass1...))
	if len(pass2) != len(pass1) {
		t.Fatalf("件④: the second pass must append nothing, got %d rows from %d", len(pass2), len(pass1))
	}
	if !reflect.DeepEqual(pass1, pass2) {
		t.Fatalf("件④: two-pass output must be deeply identical")
	}
	if b1, b2 := fmt.Sprintf("%#v", pass1), fmt.Sprintf("%#v", pass2); b1 != b2 {
		t.Fatalf("件④: two-pass output must be byte-identical:\npass1: %s\npass2: %s", b1, b2)
	}
}

// --- 件⑤ pre-truncation pool + 件⑥ three-class caveat -------------------------

// 件⑤ production witness (donghu): udk-irq-12-92's ⛓ clipped seat (anchored
// 0.039ms) dies in the build-lane candidates cap while its ◇ remainder
// survives on the side lane — the typed release arm must find the counterpart
// in the pre-truncation pool with the truncation disclosed (the former
// "compacted ∧ anchored<1ms" magnitude release waved this shape through
// without ever proving the counterpart existed).
// 件⑥: the side-row caveat enumerates its THREE row classes — rank-0
// diagnostics, rank-0 target-self disclosures, and ◇ chain-remainder seats
// (which wear an adjacent ordinal, not rank-0).
func TestRSPAHygPoolReleaseArmAndThreeClassCaveat(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	rank := BuildRootCauseRank(idx, Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	// 件⑤: the ◇ remainder is on the board; its ⛓ counterpart is capacity-
	// truncated off the board but held by the pool.
	var remainder *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Thread.PID == 92 && rank.Items[i].ChainAnchorRemainderSeat {
			remainder = &rank.Items[i]
		}
	}
	if remainder == nil {
		t.Fatalf("donghu witness drifted: udk-irq-12-92 ◇ remainder missing from the board")
	}
	onBoard, inPool := false, false
	for _, peer := range rank.Items {
		if peer.Thread.PID == 92 && !peer.ChainAnchorRemainderSeat && peer.ChainAnchorFullMs > 0 && rootCauseItemIsOnChain(peer) {
			onBoard = true
		}
	}
	for _, peer := range rank.preTruncationItems {
		if peer.Thread.PID == 92 && !peer.ChainAnchorRemainderSeat && peer.ChainAnchorFullMs > 0 && rootCauseItemIsOnChain(peer) {
			inPool = true
		}
	}
	if onBoard {
		t.Fatalf("donghu witness drifted: the pid-92 ⛓ counterpart survived the cap (the release arm is no longer exercised) — refresh the witness")
	}
	if !inPool {
		t.Fatalf("件⑤: the pre-truncation pool must hold the capacity-truncated ⛓ counterpart")
	}
	compactionDisclosed := false
	for _, compaction := range rank.Compactions {
		if compaction.Dimension == CompactionDimensionCandidates && compaction.Total > compaction.Emitted {
			compactionDisclosed = true
		}
	}
	if !compactionDisclosed {
		t.Fatalf("件⑤: the counterpart's truncation must be disclosed through a candidates compaction: %+v", rank.Compactions)
	}
	// 件⑥: the sentence face enumerates the side-lane classes and the old
	// two-class form is dead.
	//
	// EVOLUTION RECORD (RNB-1 D1 修复轮, 2026-07-14): the side lane gained the
	// FOURTH class — R4 credential-demoted seats ("chain-remainder and
	// credential-demoted seats"); the three-class token "chain-remainder
	// seats," died with the two-class form.
	sentence := ""
	for _, caveat := range rank.Caveats {
		if strings.Contains(caveat, "side disclosure row(s)") {
			sentence = caveat
		}
		if strings.Contains(caveat, "rank-0 diagnostic/target-self disclosure row(s)") {
			t.Fatalf("件⑥: the two-class sentence form must be dead: %q", caveat)
		}
	}
	if sentence == "" {
		t.Fatalf("件⑥: the side-row caveat must fire on the donghu board: %+v", rank.Caveats)
	}
	// ELIM-SELF-FIX 件2 (2026-07-15): the side lane gained the FIFTH class —
	// cap-preserved target self seats ("plus cap-preserved target self
	// seats"); the four-class "plus chain-remainder" joiner died with it.
	// LEVELMERGE-1 件2 (2026-07-18): SIXTH class — the gated-share
	// constituent rows join the adjacent-ordinal enumeration.
	for _, token := range []string{
		"rank-0 diagnostic/target-self rows",
		"chain-remainder, credential-demoted and gated-share constituent seats",
		"adjacent ordinal rather than rank-0",
		"plus cap-preserved target self seats keeping their chain ordinal",
		"do not consume candidate seats",
	} {
		if !strings.Contains(sentence, token) {
			t.Fatalf("件⑥: sentence must enumerate the six classes (missing %q): %q", token, sentence)
		}
	}
}

// --- 修复轮: rspaRemainderSummary arithmetic ----------------------------------

// The engine-side remainder sentence must state full = anchored + remainder
// with each number in its named slot (the RSPA batch shipped the anchored and
// full Sprintf args swapped; live donghu specimen udk-irq-12-92 read
// "full-window account 0.039ms = 0.307ms anchored").
func TestRSPAHygRemainderSummaryArithmetic(t *testing.T) {
	got := rspaRemainderSummary(ThreadRef{Comm: "udk-irq-12", PID: 92}, "runnable (scheduling-pressure candidate)", 0.268, 0.039, 0.307)
	want := "full-window account 0.307ms = 0.039ms anchored"
	if !strings.Contains(got, want) {
		t.Fatalf("remainder summary slots must carry full then anchored:\nwant substring %q\ngot %q", want, got)
	}
	if strings.Contains(got, "full-window account 0.039ms") {
		t.Fatalf("the swapped form must be dead: %q", got)
	}
}
