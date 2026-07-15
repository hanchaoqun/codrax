package tracequery

// rank_chain_anchor_rspahyg2_test.go — RSPA-HYG 残余批 pins (§29.83 残余清单,
// 2026-07-14): the follow-ups the §29.83 batch left on file.
//
//	残余① trace_perf_bundle / recipe 双 sweep — both lanes now pre-pull the
//	     shared stats memo through the anchored rank lane (chain gate known at
//	     case entry), so a chain-bearing bundle/recipe Run performs exactly ONE
//	     ComputeWindowStats instead of plain+anchored; the published
//	     window_stats face stays byte-identical (exported JSON) to a plain
//	     sweep's (件② isomorphism re-verified on these lanes). Wording-sweep
//	     verdict (同族发射点核查): the bundle/recipe lanes share every RSPA
//	     decomposition emission with root_cause_rank — the rank Summary
//	     sentences (rspaRemainderSummary / rspaAnchoredSummary), the typed
//	     chain_anchored/chain_anchor_full notes (direct field pairing) and the
//	     "## Root cause rank" banner face (35 slots individually re-checked);
//	     no lane-specific emission point exists and no new slot disease was
//	     found. The ⛓ sister sentence gains its own arithmetic pin below (the
//	     修复轮 pinned only the ◇ remainder form).
//	残余② ThreadInput-only 形 — the root_cause_rank case chain gate had
//	     dropped ThreadInput while getRootCause carried the triple: a
//	     degenerate selector (e.g. "-", unresolvable into Thread/PID by
//	     normalizeQuery) built the chain inside getRootCause without
//	     publishing it. The gate is now the ONE shared rankChainGate
//	     predicate; the chain face (with its honest resolution-ambiguity
//	     caveats) publishes exactly like trace_perf_bundle always did for the
//	     same input, and the Run stays single-sweep. (The HEAD double-sweep
//	     corner was structurally starved: anchors need an in-window PID>0
//	     waker, whose events make the match-all degenerate selector ambiguous,
//	     which kills the chain — probed 2026-07-14, sweeps==1 on every form.)
//	残余③ io facet 域外 facets audited per edge (§29.61.10d 逐边核非按类) —
//	     dispositions on stampResourceClosureEvaluation; unit arms live in the
//	     §29.83 件③ tests (extended); the production page_cache_churn 应豁免
//	     witness is pinned below.
//
// MUTATION self-checks (each verified red by hand during the batch):
//   - dropping the trace_perf_bundle pre-pull reds the bundle sweep pin
//     (probe counts 2);
//   - dropping the recipe pre-pull reds the recipe sweep pin (probe counts 2);
//   - reverting the root_cause_rank case gate to the PID/Thread pair reds the
//     ThreadInput-only pin (chain face vanishes);
//   - swapping rspaAnchoredSummary's full/remainder Sprintf args reds the
//     sister arithmetic pin;
//   - adding page_cache_churn to the direct-on-chain closed list reds the
//     production witness pin's structural arm.

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// --- 残余① trace_perf_bundle / recipe 单 Run 恰一次 sweep ---------------------

func TestRSPAHyg2SingleSweepBundleRun(t *testing.T) {
	idx := rspaCaseATrace(t)
	sweeps := 0
	res := Run(idx, Query{View: "trace_perf_bundle", PID: 100, TimeStart: 1.0, TimeEnd: 1.07,
		MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12,
		statsSweepProbe: &sweeps})
	if res.WindowStats == nil || res.WakeupChain == nil || res.RootCauseRank == nil {
		t.Fatalf("bundle run must publish stats+chain+rank faces: %+v", res.View)
	}
	// Non-vacuous guard: the anchored lane actually ran.
	anchoredSeen := false
	for _, item := range append(append([]RootCauseRankItem{}, res.RootCauseRank.Items...), res.RootCauseRank.AbsorbedItems...) {
		if item.ChainAnchorFullMs > 0 {
			anchoredSeen = true
		}
	}
	if !anchoredSeen {
		t.Fatalf("fixture drifted: the bundle run must exercise the anchored sweep")
	}
	if sweeps != 1 {
		t.Fatalf("残余①: a chain-bearing trace_perf_bundle Run must perform exactly ONE ComputeWindowStats, got %d", sweeps)
	}
	// Exported-face isomorphism on this lane: the bundle's published
	// window_stats face is byte-identical to a plain window_stats Run's.
	plain := Run(idx, Query{View: "window_stats", PID: 100, TimeStart: 1.0, TimeEnd: 1.07,
		MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	bundleFace, err := json.Marshal(res.WindowStats)
	if err != nil {
		t.Fatal(err)
	}
	plainFace, err := json.Marshal(plain.WindowStats)
	if err != nil {
		t.Fatal(err)
	}
	if string(bundleFace) != string(plainFace) {
		t.Fatalf("bundle anchored-sweep window_stats face must be byte-identical to the plain sweep's:\nbundle: %s\nplain:  %s", bundleFace, plainFace)
	}
}

func TestRSPAHyg2SingleSweepRecipeRun(t *testing.T) {
	idx := rspaCaseATrace(t)
	q := Query{View: "recipe", PID: 100, TimeStart: 1.0, TimeEnd: 1.07,
		MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
	sweeps := 0
	pq := q
	pq.statsSweepProbe = &sweeps
	res := Run(idx, pq)
	if res.Recipe == nil || !recipeHasView(*res.Recipe, "root_cause_rank") || !recipeHasView(*res.Recipe, "window_stats") {
		t.Fatalf("fixture must select a rank+stats recipe (sleep_root_cause), got %+v", res.Recipe)
	}
	if res.WindowStats == nil || res.WakeupChain == nil || res.RootCauseRank == nil || res.SchedulerLatency == nil {
		t.Fatalf("recipe run must publish stats+chain+rank+latency step faces")
	}
	anchoredSeen := false
	for _, item := range append(append([]RootCauseRankItem{}, res.RootCauseRank.Items...), res.RootCauseRank.AbsorbedItems...) {
		if item.ChainAnchorFullMs > 0 {
			anchoredSeen = true
		}
	}
	if !anchoredSeen {
		t.Fatalf("fixture drifted: the recipe run must exercise the anchored sweep")
	}
	if sweeps != 1 {
		t.Fatalf("残余①: a chain-bearing recipe Run must perform exactly ONE ComputeWindowStats, got %d", sweeps)
	}
	// Exported window_stats step face stays isomorphic to the plain sweep.
	plain := Run(idx, Query{View: "window_stats", PID: 100, TimeStart: 1.0, TimeEnd: 1.07,
		MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	recipeFace, err := json.Marshal(res.WindowStats)
	if err != nil {
		t.Fatal(err)
	}
	plainFace, err := json.Marshal(plain.WindowStats)
	if err != nil {
		t.Fatal(err)
	}
	if string(recipeFace) != string(plainFace) {
		t.Fatalf("recipe anchored-sweep window_stats face must be byte-identical to the plain sweep's:\nrecipe: %s\nplain:  %s", recipeFace, plainFace)
	}
	// The recipe view is not in the DET-1 view sweep — pin the armed-but-
	// unfired byte identity for this lane here (the pre-pull must be a pure
	// read on the untriggered path).
	armed, stop := context.WithCancel(context.Background())
	defer stop()
	plainRun, err := json.Marshal(Run(idx, q))
	if err != nil {
		t.Fatal(err)
	}
	armedRun, err := json.Marshal(Run(idx, q.WithRunContext(armed)))
	if err != nil {
		t.Fatal(err)
	}
	if string(plainRun) != string(armedRun) {
		t.Fatalf("armed-but-unfired context changed the recipe result:\nplain: %s\narmed: %s", plainRun, armedRun)
	}
}

// The chainless recipe keeps the plain shared memo (still exactly one sweep;
// the pre-pull gate is closed, no chain build, no decomposition).
func TestRSPAHyg2ChainlessRecipeSingleSweep(t *testing.T) {
	idx := rspaCaseATrace(t)
	sweeps := 0
	res := Run(idx, Query{View: "recipe", TimeStart: 1.0, TimeEnd: 1.07, TimeStartSet: true, TimeEndSet: true,
		MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12,
		statsSweepProbe: &sweeps})
	if res.RootCauseRank == nil || res.WindowStats == nil {
		t.Fatalf("chainless recipe must still publish its rank and stats step faces")
	}
	if sweeps != 1 {
		t.Fatalf("chainless recipe Run must stay a single sweep, got %d", sweeps)
	}
	for _, item := range res.RootCauseRank.Items {
		if item.ChainAnchorFullMs > 0 || item.ChainAnchorRemainderSeat {
			t.Fatalf("chainless run must not mint decomposition: %+v", item)
		}
	}
}

// --- 残余② ThreadInput-only 形 -------------------------------------------------

func TestRSPAHyg2ThreadInputOnlyRankRun(t *testing.T) {
	idx := rspaCaseATrace(t)
	sweeps := 0
	res := Run(idx, Query{View: "root_cause_rank", ThreadInput: "-", TimeStart: 1.0, TimeEnd: 1.07,
		MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12,
		statsSweepProbe: &sweeps})
	if sweeps != 1 {
		t.Fatalf("残余②: a ThreadInput-only rank Run must stay a single sweep, got %d", sweeps)
	}
	if res.RootCauseRank == nil || res.WindowStats == nil {
		t.Fatalf("ThreadInput-only rank run must publish its rank and stats faces")
	}
	// Gate alignment witness: the chain face (built by getRootCause at HEAD
	// but silently dropped) now publishes with its honest resolution caveats —
	// the same face trace_perf_bundle always published for this input.
	if res.WakeupChain == nil {
		t.Fatalf("aligned rankChainGate must publish the chain face for a ThreadInput-only query")
	}
	found := false
	for _, caveat := range res.WakeupChain.Caveats {
		if strings.Contains(caveat, "target thread not found") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the degenerate selector's chain face must carry the honest resolution caveat: %+v", res.WakeupChain.Caveats)
	}
	// The degenerate match-all selector is ambiguous on this trace — no
	// anchors, no decomposition (the structural starvation argument).
	for _, item := range res.RootCauseRank.Items {
		if item.ChainAnchorFullMs > 0 || item.ChainAnchorRemainderSeat {
			t.Fatalf("ambiguous degenerate selector must not mint decomposition: %+v", item)
		}
	}
}

// --- 残余① wording sweep — ⛓ sister sentence arithmetic pin -------------------

// The 修复轮 pinned rspaRemainderSummary after its anchored/full slots proved
// swapped; the ⛓ clipped-seat sister sentence shares the value-triplet family
// and gains the same arithmetic pin (slots: anchored, full, full-anchored).
func TestRSPAHyg2AnchoredSummaryArithmetic(t *testing.T) {
	got := rspaAnchoredSummary(ThreadRef{Comm: "worker", PID: 200}, "runnable", 27.0, 47.0)
	if !strings.Contains(got, "27.000ms anchored inside") {
		t.Fatalf("anchored slot must carry the anchored value: %q", got)
	}
	if !strings.Contains(got, "full-window account 47.000ms = this anchored portion + 20.000ms remainder") {
		t.Fatalf("full/remainder slots must carry full then full-anchored:\ngot %q", got)
	}
	if strings.Contains(got, "full-window account 27.000ms") || strings.Contains(got, "47.000ms anchored") {
		t.Fatalf("the swapped form must be dead: %q", got)
	}
}

// --- 残余③ page_cache_churn 应豁免 production witness --------------------------

// tieba 59566 window: the ThreadPoolForeg-60555 page_cache_churn envelope
// (61.540ms, 24.568ms inside the dependency windows — the SAME partial-overlap
// envelope as the §29.83 件③ block_io witness) publishes on the ◇ adjacent
// lane with its synthetic churn score untouched: the direct-on-chain closed
// list already owns the exemption, no containment arm engages.
func TestRSPAHyg2TiebaPageCacheChurnStaysAdjacent(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatal(err)
	}
	rank := BuildRootCauseRank(idx, Query{PID: 59566, TimeStart: 34579.472865, TimeEnd: 34579.587805,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	found := false
	for _, item := range append(append([]RootCauseRankItem{}, rank.Items...), rank.AbsorbedItems...) {
		if item.Thread.PID != 60555 || item.Type != "page_cache_churn" {
			continue
		}
		found = true
		if item.ChainRelevance != "adjacent" || rootCauseItemIsOnChain(item) {
			t.Fatalf("page_cache_churn must ride the ◇ adjacent lane (closed-list exemption): %+v", item)
		}
		if math.Abs(item.CumulativeImpactMs-7.200) > 0.002 {
			t.Fatalf("the exemption must never touch the published score: %+v", item)
		}
	}
	if !found {
		t.Fatalf("tieba page_cache_churn witness row missing")
	}
}
