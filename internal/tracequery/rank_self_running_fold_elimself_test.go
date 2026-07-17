package tracequery

// rank_self_running_fold_elimself_test.go — ELIM-SELF-FIX acceptance pins
// (§29.93 R8 / §29.93.1 修向①② / §29.93.3 全族收编, ledger
// docs/design/real_trace_campaign_20260705.md, 2026-07-15).
//
// Witness anchors (V-2, recomputed under the RNB-4 §29.94 single basis and
// INDEPENDENTLY hand-verified by a Python probe reading the raw fixtures —
// running-interval window projection + governed frequency slicing + the R5
// global-max-core-max-frequency fold; all four windows matched the engine to
// the µs):
//
//	donghu 17267 flagship  [13762.791708..13763.024898]:
//	    running 157.248  ideal  98.928  deficit 58.320  (fmax 2750000
//	    observed, class big, cap 2.53 — the would-be gap 8.2× the board's
//	    former #1 seat 7.081)
//	donghu 2955  same win:  running  74.915  ideal   9.003  deficit 65.912
//	tieba 59566  head win  [34579.450627..34579.472865] (退化窗):
//	    running  19.795  ideal  10.430  deficit  9.365  (freq_only basis
//	    fmax 2189000, CFR donor lane for the curve-less cpu0-2)
//	tieba 59566  h1 win    [34579.450627..34579.522905]:
//	    running  25.319  ideal  15.773  deficit  9.546

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func elimSelfIndex(t *testing.T, path string) *Index {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func elimSelfQuery(pid int, ws, we float64) Query {
	return Query{PID: pid, TimeStart: ws, TimeEnd: we,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
}

func findSelfRunningSeat(items []RootCauseRankItem, pid int) *RootCauseRankItem {
	for i := range items {
		if items[i].Type == "running" && items[i].Thread.PID == pid {
			return &items[i]
		}
	}
	return nil
}

// 件1 Form-1 修根: the ordinary donghu flagship window mints the target's own
// running supply-fold deficit seat — full typed shape + exact witness values.
func TestElimSelfRunningFoldSeatDonghuFlagship(t *testing.T) {
	idx := elimSelfIndex(t, "../../eval/fixtures/real_traces/donghu.ftrace")
	rank := BuildRootCauseRank(idx, elimSelfQuery(17267, 13762.791708, 13763.024898))
	seat := findSelfRunningSeat(rank.Items, 17267)
	if seat == nil {
		t.Fatalf("Form-1 修根: the self running fold seat must mint on an ordinary window: %+v", rank.Items)
	}
	if got := fmt.Sprintf("%.3f/%.3f/%.3f", seat.RunningMs, seat.SupplyFoldIdealMs, seat.SupplyFoldDeficitMs); got != "157.248/98.928/58.320" {
		t.Fatalf("witness values drifted (hand-verified 157.248/98.928/58.320): %s", got)
	}
	if got := fmt.Sprintf("%.3f", rootCauseEffectiveImpactMs(*seat)); got != "58.320" {
		t.Fatalf("eff must be the fold deficit ONLY (0 权威不伪造): %s", got)
	}
	// R8 channel identity: on_chain on the SELF basis with the honest token.
	if seat.ChainRelevance != "on_chain" || seat.OnChainBasis != RootCauseOnChainBasisSelfWallClockInterval ||
		seat.Causality != RootCauseCausalitySelfWallClock || !seat.SubjectIsAnalysisTarget {
		t.Fatalf("R8 seat identity drifted: rel=%s basis=%s causality=%s self=%v",
			seat.ChainRelevance, seat.OnChainBasis, seat.Causality, seat.SubjectIsAnalysisTarget)
	}
	// Ordinary election ladder (no special tier): the 58.320 deficit wins #1.
	if seat.Rank != 1 || seat.Tier != "primary" {
		t.Fatalf("the seat rides the ordinary ladder (witness #1 primary): rank=%d tier=%s", seat.Rank, seat.Tier)
	}
	// RNB-4 single-basis provenance rides the seat.
	if seat.SupplyFoldBasis == nil {
		t.Fatalf("the seat must carry its fold basis")
	}
	if seat.SupplyFoldBasis.FmaxKHz != 2750000 || seat.SupplyFoldBasis.FmaxSource != SupplyFoldFmaxSourceObserved ||
		seat.SupplyFoldBasis.ReferenceClass != "big" || !seat.SupplyFoldBasis.AllKnown() {
		t.Fatalf("R5 basis drifted: %+v", *seat.SupplyFoldBasis)
	}
	// ORD single-seat closure: exactly ONE running rank row for the target
	// across the published wire (candidates + side lanes + absorbed).
	seen := 0
	for _, it := range append(append([]RootCauseRankItem{}, rank.Items...), rank.AbsorbedItems...) {
		if it.Type == "running" && it.Thread.PID == 17267 {
			seen++
		}
	}
	if seen != 1 {
		t.Fatalf("ORD 单席闭合: exactly one self running seat, got %d", seen)
	}
}

// 件1 witness (donghu 2955): the CompThread window mints 65.912 and the
// existing self D-state chain seat keeps publishing beside it.
func TestElimSelfRunningFoldSeatDonghu2955(t *testing.T) {
	idx := elimSelfIndex(t, "../../eval/fixtures/real_traces/donghu.ftrace")
	rank := BuildRootCauseRank(idx, elimSelfQuery(2955, 13762.791708, 13763.024898))
	seat := findSelfRunningSeat(rank.Items, 2955)
	if seat == nil {
		t.Fatalf("2955 window must mint the self running fold seat: %+v", rank.Items)
	}
	if got := fmt.Sprintf("%.3f/%.3f", rootCauseEffectiveImpactMs(*seat), seat.SupplyFoldDeficitMs); got != "65.912/65.912" {
		t.Fatalf("2955 witness deficit drifted (hand-verified 65.912): %s", got)
	}
	var dSeat *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Thread.PID == 2955 && rootCauseItemIsDStateOrIOCaliber(rank.Items[i]) &&
			rootCauseEffectiveImpactMs(rank.Items[i]) > 30 {
			dSeat = &rank.Items[i]
		}
	}
	if dSeat == nil {
		t.Fatalf("the 36.757 self D seat must keep publishing beside the running seat: %+v", rank.Items)
	}
}

// 件2 Form-2 修根 + §29.93.3: the tieba head window (退化窗, channel-blind
// build sort, candidate cap 12 filled by background rows) publishes ALL of
// the target's own on-chain seats — running (new lane) + runnable + io_wait
// — instead of silently killing them at the cap (排查实录: pre-truncation
// positions 18/20/24, published 0).
func TestElimSelfSeatsSurviveTiebaDegenerateWindow(t *testing.T) {
	idx := elimSelfIndex(t, "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	rank := BuildRootCauseRank(idx, elimSelfQuery(59566, 34579.450627, 34579.472865))
	seat := findSelfRunningSeat(rank.Items, 59566)
	if seat == nil {
		t.Fatalf("the degenerate window must mint + publish the self running fold seat: %+v", rank.Items)
	}
	if got := fmt.Sprintf("%.3f/%.3f/%.3f", seat.RunningMs, seat.SupplyFoldIdealMs, seat.SupplyFoldDeficitMs); got != "19.795/10.430/9.365" {
		t.Fatalf("tieba head witness drifted (hand-verified 19.795/10.430/9.365): %s", got)
	}
	// The freq_only basis prices at pure frequency ratio (cap 1, no class).
	if seat.SupplyFoldBasis == nil || seat.SupplyFoldBasis.FmaxKHz != 2189000 ||
		seat.SupplyFoldBasis.ReferenceClass != "" || seat.SupplyFoldBasis.CapabilitySource != CoreCapabilitySourceFreqOnly {
		t.Fatalf("tieba freq_only basis drifted: %+v", seat.SupplyFoldBasis)
	}
	wantEff := map[string]string{"running": "9.365", "io_wait": "0.635"}
	for typ, want := range wantEff {
		found := false
		for _, it := range rank.Items {
			if it.Thread.PID != 59566 || it.Type != typ {
				continue
			}
			if got := fmt.Sprintf("%.3f", rootCauseEffectiveImpactMs(it)); got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("全族收编: the self %s seat (%s ms) must survive to the published wire: %+v", typ, want, rank.Items)
		}
	}
	var inversion *RootCauseRankItem
	for i := range rank.Items {
		it := &rank.Items[i]
		if it.Thread.PID == 59566 && it.Type == "priority_inversion_runnable_wait" {
			inversion = it
			break
		}
	}
	if inversion == nil || fmt.Sprintf("%.3f/%.3f/%.3f", inversion.RunnableMs, inversion.EffectiveImpactMs, inversion.PriorityRelationUnknownOrNonLowerMs) != "1.575/0.214/1.361" ||
		inversion.PriorityRelationCaliber != "closed_range_stable" {
		t.Fatalf("self runnable disclosure/proven priority partition drifted: %+v", inversion)
	}
	// ORD 单席闭合 (修复轮 P1-1, 对抗官实锤 2026-07-15): THIS window is where
	// the depth-0 exception arm genuinely fires (the chain expansion carries a
	// 1.871ms running subset impact) — with the closure removed the published
	// wire seats BOTH the window-projection seat (9.365) and the depth-0
	// subset twin (1.871), double-counting the same physical running time.
	// The donghu flagship count pin cannot catch that (its depth-0 arm never
	// fires); this one turns red on the closure's removal (mutation-verified).
	seen := 0
	for _, it := range append(append([]RootCauseRankItem{}, rank.Items...), rank.AbsorbedItems...) {
		if it.Type == "running" && it.Thread.PID == 59566 {
			seen++
		}
	}
	if seen != 1 {
		t.Fatalf("ORD 单席闭合: exactly one published self running seat on the depth-0-firing window, got %d", seen)
	}
}

// 件3 全族在榜恒等 pin (§29.93.3 补钉②): every self family seat holding value
// (eff>0, on-chain) present in the engine pre-truncation pool has a published
// wire form — the self specialization of the 零静默消失 invariant, pinned on
// a normal window AND a degenerate window.
func TestElimSelfPoolSeatsNeverSilentlyVanish(t *testing.T) {
	cases := []struct {
		name  string
		trace string
		pid   int
		ws    float64
		we    float64
	}{
		{"normal_donghu_flagship", "../../eval/fixtures/real_traces/donghu.ftrace", 17267, 13762.791708, 13763.024898},
		{"degenerate_tieba_head", "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace", 59566, 34579.450627, 34579.472865},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			idx := elimSelfIndex(t, c.trace)
			rank := BuildRootCauseRank(idx, elimSelfQuery(c.pid, c.ws, c.we))
			published := map[string]bool{}
			for _, it := range append(append([]RootCauseRankItem{}, rank.Items...), rank.AbsorbedItems...) {
				published[strings.TrimSpace(it.Type)+"\x00"+threadKey(it.Thread)] = true
			}
			if len(rank.preTruncationItems) == 0 {
				t.Fatalf("fixture drifted: empty pre-truncation pool")
			}
			for _, it := range rank.preTruncationItems {
				if !it.SubjectIsAnalysisTarget || !rootCauseItemIsOnChain(it) {
					continue
				}
				if rootCauseEffectiveImpactMs(it) <= 0 {
					continue
				}
				key := strings.TrimSpace(it.Type) + "\x00" + threadKey(it.Thread)
				if !published[key] {
					t.Fatalf("自身持值席静默消失 (type=%s eff=%.3f): pool seat has no published wire form",
						it.Type, rootCauseEffectiveImpactMs(it))
				}
			}
		})
	}
}

// 件1 修向① suppression-arm narrowing: the running-dominant low_frequency
// verdict suppression keys on the PRECISE seat-presence bit for the target —
// represented ⇒ suppressed (double-seat prevention), unrepresented target ⇒
// the verdict is the lane's only outlet and mints as a rank-0 disclosure;
// non-target threads keep the historic suppression byte-identically.
func TestElimSelfLowFrequencySuppressionNarrowedToRepresentedTarget(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	other := ThreadRef{Comm: "bg", PID: 200}
	stats := WindowStats{ComputeSupply: []ComputeSupplySummary{
		{Thread: target, Verdict: "low_frequency_signal", State: string(StateRunning),
			DurationMs: 3.0, Confidence: 0.7, Summary: "target low-frequency running"},
		{Thread: other, Verdict: "low_frequency_signal", State: string(StateRunning),
			DurationMs: 2.0, Confidence: 0.7, Summary: "bg low-frequency running"},
	}}
	baseRank := func(withRunningSeat bool) RootCauseRankResult {
		rank := RootCauseRankResult{Target: target}
		if withRunningSeat {
			rank.Items = append(rank.Items, RootCauseRankItem{
				Type: "running", Thread: target, DominantState: string(StateRunning),
				RunningMs: 5, EffectiveImpactMs: 1.2, SupplyFoldDeficitMs: 1.2,
				ChainRelevance: "on_chain", Causality: RootCauseCausalitySelfWallClock,
				OnChainBasis: RootCauseOnChainBasisSelfWallClockInterval,
			})
		}
		return rank
	}
	count := func(rank RootCauseRankResult, pid int) int {
		n := 0
		for _, it := range rank.Items {
			if it.Type == "low_frequency" && it.Thread.PID == pid && it.DominantState == string(StateRunning) {
				n++
			}
		}
		return n
	}
	q := Query{PID: 100, TimeStart: 1, TimeEnd: 1.1, Limit: 12}
	// Represented target: suppressed (premise completed by the seat).
	enriched := enrichRootCauseRankWithScheduler(q, baseRank(true), SchedulerLatencyResult{}, stats, ChainResult{})
	if count(enriched, 100) != 0 {
		t.Fatalf("a represented target's running low_frequency verdict must stay suppressed (double-seat prevention)")
	}
	// Unrepresented target: the verdict is the only outlet — it mints, and
	// its running caliber keeps effective=0 (rank-0 disclosure, never a
	// competing seat).
	enriched = enrichRootCauseRankWithScheduler(q, baseRank(false), SchedulerLatencyResult{}, stats, ChainResult{})
	if count(enriched, 100) != 1 {
		t.Fatalf("an unrepresented target's running low_frequency verdict must mint as the lane outlet: %+v", enriched.Items)
	}
	for _, it := range enriched.Items {
		if it.Type == "low_frequency" && it.Thread.PID == 100 {
			if rootCauseEffectiveImpactMs(it) != 0 || it.Rank != 0 {
				t.Fatalf("the outlet row must stay a rank-0 zero-effective disclosure: %+v", it)
			}
		}
	}
	// Non-target threads: historic suppression byte-identical.
	if count(enriched, 200) != 0 {
		t.Fatalf("non-target running low_frequency verdicts keep the historic suppression")
	}
}
