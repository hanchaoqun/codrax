package tracequery

import (
	"strings"
	"testing"
)

// supply_fold_vs2_test.go — VS-2 (§7.10) engine pins for the supply-fold
// accounting of on-chain running-dominant wakeup-chain nodes.
//
// Synthetic numeric fixture (pinned in the design ledger): two 10ms running
// slices — one at 1GHz on a small core, one at 2GHz on the big core whose
// governed fmax is 2GHz. CAP (§26, real_trace_campaign_20260705.md,
// 2026-07-08). EVOLUTION RECORD: the fold now prices core-class capability
// (derived 2-cluster shape → 小=×1.0 / 大=×2.53 default table), so the small
// slice folds at (1/2)×(1/2.53)≈0.198 — ideal ≈1.98+10=11.98ms, deficit
// ≈8.02ms (pre-CAP pure frequency ratio: ideal 15ms, deficit 5ms; assertions
// evolved, none deleted). A slice with no frequency data still folds at
// ratio 1 (zero fabricated deficit, UNKNOWN basis); the deficit is clamped
// ≥ 0 even when a slice is governed above the big-cluster equivalent
// capacity; raw pre-window frequency history never participates.

func supplyFoldDepImpact(t *testing.T, chain ChainResult) *WakeupCausalImpact {
	t.Helper()
	for i := range chain.CausalImpacts {
		if chain.CausalImpacts[i].Thread.PID == 200 {
			return &chain.CausalImpacts[i]
		}
	}
	t.Fatalf("dependency causal impact missing: %+v", chain.CausalImpacts)
	return nil
}

// The ledger's synthetic pin: 10ms@1GHz small + 10ms@2GHz big, big fmax=2GHz.
// CAP (§26) evolution: ideal = 10×(1/2)×(1/2.53) + 10×(2/2) ≈ 11.98ms,
// deficit ≈ 8.02ms (pre-CAP: 15ms / 5ms — 小核 running 缺口变大, the ruling's
// direction witness; the big-core slice still folds to zero deficit).
func TestSupplyFoldTwoSliceNumericPin(t *testing.T) {
	idx := buildTraceIndex(t, "vs2_two_slice.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.010000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [007] .... 5.019900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [007] .... 5.020000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	if dep.DominantState != string(StateRunning) {
		t.Fatalf("fixture must be running-dominant, got %+v", dep)
	}
	if dep.SupplyFoldBasis == nil {
		t.Fatalf("running-dominant on-chain node must compute the supply fold: %+v", dep)
	}
	t.Logf("CAP §26 direction dump (two-slice ledger form): deficit pre-CAP≈5.000 → now %.3f, ideal pre-CAP≈15.000 → now %.3f", dep.SupplyFoldDeficitMs, dep.SupplyFoldIdealMs)
	if dep.SupplyFoldDeficitMs < 7.7 || dep.SupplyFoldDeficitMs > 8.3 {
		t.Fatalf("deficit should be the ~8.02ms capability fold (CAP §26), got %.3f", dep.SupplyFoldDeficitMs)
	}
	if dep.SupplyFoldIdealMs < 11.7 || dep.SupplyFoldIdealMs > 12.3 {
		t.Fatalf("ideal should be ~11.98ms (CAP §26), got %.3f", dep.SupplyFoldIdealMs)
	}
	// CAP (§26 C1): the derived 2-cluster structure judges → default table in
	// force, typed disclosure token stamped.
	if dep.SupplyFoldBasis.CapabilitySource != CoreCapabilitySourceDefault {
		t.Fatalf("judged 2-cluster fold must disclose the default capability table, got %+v", dep.SupplyFoldBasis)
	}
	// Identities: ideal + deficit == RunningMs, known + unknown == RunningMs.
	if got, want := dep.SupplyFoldIdealMs+dep.SupplyFoldDeficitMs, dep.RunningMs; !floatNear(got, want) {
		t.Fatalf("ideal+deficit must reconstruct RunningMs: %.6f != %.6f", got, want)
	}
	if got, want := dep.SupplyFoldBasis.KnownMs+dep.SupplyFoldBasis.UnknownMs, dep.RunningMs; !floatNear(got, want) {
		t.Fatalf("basis split must cover RunningMs: %.6f != %.6f", got, want)
	}
	if dep.SupplyFoldBasis.UnknownMs != 0 || !dep.SupplyFoldBasis.AllKnown() {
		t.Fatalf("both slices carry governed samples — basis must be fully known: %+v", dep.SupplyFoldBasis)
	}
	// VS-2b (additive extension of this pin, no pinned value changed): with
	// no cpu_frequency_limits rows the fmax ladder falls back to the
	// observed governance step and says so.
	if dep.SupplyFoldBasis.FmaxKHz != 2000000 || dep.SupplyFoldBasis.FmaxSource != SupplyFoldFmaxSourceObserved {
		t.Fatalf("limits-free trace must resolve fmax=2GHz from observed governance: %+v", dep.SupplyFoldBasis)
	}
	if dep.SupplyFoldBasis.LimitThrottled {
		t.Fatalf("no limits row → no throttling finding: %+v", dep.SupplyFoldBasis)
	}
	// The typed accounting reaches the summary surface, zeros included.
	for _, want := range []string{"supply_fold_deficit=", "supply_fold_ideal=", "fold_basis_known=", "fold_basis_unknown=0.000ms"} {
		if !strings.Contains(dep.Summary, want) {
			t.Fatalf("impact summary missing %q:\n%s", want, dep.Summary)
		}
	}
}

// A slice on a CPU with NO resolvable frequency folds at ratio 1 — no
// fabricated deficit — and books as UNKNOWN basis (§7.10 无频点数据 rule).
//
// CFR-2 (#80, 用户裁定 2026-07-06) fixture evolution: the unknown slice
// originally sat on cpu3 with sampled cpu2/cpu7 below/above — under the
// change-point derivation cpu3 now legitimately inherits cpu7's cluster
// (向高核号就近继承), so the unknown slice moved ABOVE the highest sampled
// core (cpu8, 向上不外推 keeps it unresolvable). The §7.10 rule itself is
// unchanged: unresolvable frequency still folds at ratio 1 with UNKNOWN
// basis — this fixture is also the fold-face upward-extrapolation witness.
func TestSupplyFoldUnknownFrequencySliceZeroDeficit(t *testing.T) {
	idx := buildTraceIndex(t, "vs2_unknown_slice.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
        dep-200 (100) [008] .... 5.010000: sched_switch: prev_comm=idle/8 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [008] .... 5.019900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [008] .... 5.020000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/8 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil {
		t.Fatalf("fold must run: %+v", dep)
	}
	// Only the 1GHz slice folds (CAP §26 evolution: 10×(1−(1/2)/2.53)≈8.02ms
	// deficit — pre-CAP 10×0.5=5ms); the above-max-sampled CPU8 slice
	// contributes ideal=wall and UNKNOWN basis (no upward extrapolation —
	// derived groups here are {2} and {7}, count 2 < 3, so cpu8 is not even
	// declared a prime pseudo-domain, and either way no sampled member exists
	// above cpu7 to donate).
	if dep.SupplyFoldDeficitMs < 7.7 || dep.SupplyFoldDeficitMs > 8.3 {
		t.Fatalf("deficit must come from the known slice only, got %.3f", dep.SupplyFoldDeficitMs)
	}
	if dep.SupplyFoldBasis.UnknownMs < 9.7 || dep.SupplyFoldBasis.UnknownMs > 10.3 {
		t.Fatalf("the no-sample slice must book as unknown basis, got %+v", dep.SupplyFoldBasis)
	}
	if dep.SupplyFoldBasis.KnownMs < 9.7 || dep.SupplyFoldBasis.KnownMs > 10.3 {
		t.Fatalf("the governed slice must book as known basis, got %+v", dep.SupplyFoldBasis)
	}
	if dep.SupplyFoldBasis.AllKnown() {
		t.Fatalf("partial coverage must not read as fully known: %+v", dep.SupplyFoldBasis)
	}
	if len(dep.SupplyFoldBasis.ClusterFreqReuse) != 0 || dep.SupplyFoldBasis.ClusterFreqReuseSource != "" {
		t.Fatalf("no reuse happened → no disclosure: %+v", dep.SupplyFoldBasis)
	}
}

// R5/R6-规则4 (§29.88.3/§29.88.9, 2026-07-15) EVOLUTION RECORD — this pin
// INVERTED: it used to assert the CMP-10 F1 window caliber on the BASIS (the
// pre-window 3GHz burst must NOT raise fmax → deficit ≈7.94 vs the "leaked"
// ≈8.60). The R5 basis is now the 全域最大核最高频点 over the FULL-FILE
// curves — the 3GHz burst is exactly the capability evidence rule 4 demands
// (a window-local basis systematically under-states fmax; capability is a
// trace attribute, not a window attribute). CMP-10 F1 SURVIVES on the slice
// side: the slice's own frequency still reads window governance (the
// carry-in 2GHz never rewrites the 1GHz slice). Hand computation:
//
//	basis   = big {7} full-file fmax 3000000 × 2.53 = 7590000
//	slice   = 9.9ms on cpu2 governed 1000000 × cap(small)=1
//	deficit = 9.9 × (1 − 1000000/7590000) = 9.9 × 0.868248 ≈ 8.596ms
func TestSupplyFoldPreWindowHistoryExcluded(t *testing.T) {
	idx := buildTraceIndex(t, "vs2_prewindow.systrace", `
      <idle>-0 (-----) [007] .... 4.000000: cpu_frequency: state=3000000 cpu_id=7
      <idle>-0 (-----) [007] .... 4.500000: cpu_frequency: state=2000000 cpu_id=7
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [002] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil {
		t.Fatalf("fold must run: %+v", dep)
	}
	// ~9.9ms @1GHz small against the FULL-FILE big fmax 3GHz → ≈8.596ms (R5
	// global basis; the old window-governed 2GHz basis gave ≈7.94ms).
	if dep.SupplyFoldDeficitMs < 8.4 || dep.SupplyFoldDeficitMs > 8.8 {
		t.Fatalf("full-file 3GHz fmax must anchor the R5 basis, got deficit %.3f", dep.SupplyFoldDeficitMs)
	}
	if dep.SupplyFoldBasis.FmaxKHz != 3000000 {
		t.Fatalf("R5 basis must read the full-file big fmax 3GHz, got %+v", dep.SupplyFoldBasis)
	}
}

// Clamp: under explicit topology the LABELED big cluster's governed fmax can
// sit below another cluster's governed frequency. CAP 复核 F1 (2026-07-08).
// EVOLUTION RECORD: the fold reference is now the capability BIG-CLASS
// cluster resolved by fmax order (the label contributes membership only), so
// this mislabeled-topology shape folds the cpu7 slice against ITS OWN
// cluster's (2GHz, 2.53) basis — deficit 0 via ratio 1, no clamp engaged; the
// REAL above-reference clamp witness is the prime-slice pin
// (TestSupplyFoldPrimeSliceClampsAboveBigReference). Still the affirmative
// fourth-branch shape: deficit 0 on a fully-known basis.
func TestSupplyFoldClampAboveBigClusterFmax(t *testing.T) {
	idx := buildTraceIndex(t, "vs2_clamp.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.000000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [007] .... 5.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [007] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	// Explicit topology declares CPU2 the big cluster (governed fmax 1GHz);
	// the dep slice runs on small CPU7 governed at 2GHz — ABOVE big fmax.
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05,
		TraceFlavorHint: TraceFlavorHarmonyHitrace, CoreTopology: "big=2;small=7"})
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil {
		t.Fatalf("fold must run: %+v", dep)
	}
	if dep.SupplyFoldDeficitMs != 0 {
		t.Fatalf("slice above big-cluster fmax must clamp to zero deficit, got %.3f", dep.SupplyFoldDeficitMs)
	}
	if !floatNear(dep.SupplyFoldIdealMs, dep.RunningMs) {
		t.Fatalf("clamped ideal must equal RunningMs: %.6f != %.6f", dep.SupplyFoldIdealMs, dep.RunningMs)
	}
	if !dep.SupplyFoldBasis.AllKnown() {
		t.Fatalf("governed slice must be known basis: %+v", dep.SupplyFoldBasis)
	}
	// 复核 F1 coherence: the basis pair is the fmax-ordered big-class
	// cluster's own (cpu7, 2GHz) — never the 1GHz "big"-labeled cluster's
	// fmax under another cluster's cap.
	if dep.SupplyFoldBasis.ReferenceClass != "big" || dep.SupplyFoldBasis.FmaxKHz != 2000000 {
		t.Fatalf("same-cluster basis must be (2GHz, big-class): %+v", dep.SupplyFoldBasis)
	}
}

// No frequency data anywhere: the fold still runs (presence signal for the
// display layer's honest "频点数据不全" branch) but books everything as
// UNKNOWN basis with zero deficit — nothing is fabricated.
func TestSupplyFoldNoFrequencyDataAllUnknown(t *testing.T) {
	idx := buildTraceIndex(t, "vs2_nofreq.systrace", `
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [002] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil {
		t.Fatalf("fold presence signal must still publish: %+v", dep)
	}
	if dep.SupplyFoldDeficitMs != 0 {
		t.Fatalf("no frequency data must never fabricate a deficit: %.3f", dep.SupplyFoldDeficitMs)
	}
	if dep.SupplyFoldBasis.KnownMs != 0 || !floatNear(dep.SupplyFoldBasis.UnknownMs, dep.RunningMs) {
		t.Fatalf("everything must book as unknown basis: %+v vs running %.3f", dep.SupplyFoldBasis, dep.RunningMs)
	}
}

// The typed gate: a runnable-dominant node NEVER computes the fold — the
// mechanism belongs to running-slow stories only.
func TestSupplyFoldSkippedOnRunnableDominantNode(t *testing.T) {
	idx := buildTraceIndex(t, "vs2_runnable.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.018000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.019000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	if dep.DominantState != string(StateRunnable) {
		t.Fatalf("fixture must be runnable-dominant, got %+v", dep)
	}
	if dep.SupplyFoldBasis != nil || dep.SupplyFoldDeficitMs != 0 || dep.SupplyFoldIdealMs != 0 {
		t.Fatalf("runnable-dominant node must not compute the fold: %+v", dep)
	}
}

// Aggregate + rank-row mirrors: folded members SUM into the aggregate; the
// rootCauseItem builders copy the accounting verbatim; and the fold NEVER
// moves Score/rank (§7.10 red line: deficit 不参赛).
func TestSupplyFoldAggregateAndRankMirrors(t *testing.T) {
	member := func(startTs float64, deficit, ideal, known float64) WakeupCausalImpact {
		return WakeupCausalImpact{
			Thread:              ThreadRef{Comm: "dep", PID: 200},
			Window:              TimeWindow{StartTs: startTs, EndTs: startTs + 0.02},
			ChainDepth:          1,
			OnChain:             true,
			DominantState:       string(StateRunning),
			DominantImpactMs:    ideal + deficit,
			TotalMs:             ideal + deficit,
			RunningMs:           ideal + deficit,
			SupplyFoldDeficitMs: deficit,
			SupplyFoldIdealMs:   ideal,
			SupplyFoldBasis:     &SupplyFoldBasis{KnownMs: known, UnknownMs: ideal + deficit - known},
		}
	}
	chain := ChainResult{CausalImpacts: []WakeupCausalImpact{
		member(5.0, 5.0, 15.0, 20.0),
		member(6.0, 2.5, 7.5, 8.0),
	}}
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if len(aggregates) != 1 {
		t.Fatalf("expected one aggregate, got %+v", aggregates)
	}
	agg := aggregates[0]
	if agg.SupplyFoldBasis == nil ||
		!floatNear(agg.SupplyFoldDeficitMs, 7.5) || !floatNear(agg.SupplyFoldIdealMs, 22.5) ||
		!floatNear(agg.SupplyFoldBasis.KnownMs, 28.0) || !floatNear(agg.SupplyFoldBasis.UnknownMs, 2.0) {
		t.Fatalf("aggregate must sum the folded members: %+v basis=%+v", agg, agg.SupplyFoldBasis)
	}
	if !strings.Contains(agg.Summary, "supply_fold_deficit=7.500ms") {
		t.Fatalf("aggregate summary missing fold accounting:\n%s", agg.Summary)
	}

	// Rank-row mirror + score neutrality (per-occurrence face).
	withFold := chain.CausalImpacts[0]
	withoutFold := withFold
	withoutFold.SupplyFoldDeficitMs, withoutFold.SupplyFoldIdealMs, withoutFold.SupplyFoldBasis = 0, 0, nil
	itemWith := rootCauseItemFromCausalImpact(withFold)
	itemWithout := rootCauseItemFromCausalImpact(withoutFold)
	if itemWith.SupplyFoldBasis == nil || itemWith.SupplyFoldDeficitMs != 5.0 || itemWith.SupplyFoldIdealMs != 15.0 {
		t.Fatalf("rank row must mirror the fold accounting: %+v", itemWith)
	}
	if itemWithout.SupplyFoldBasis != nil {
		t.Fatalf("no fold on the impact → no fold on the rank row: %+v", itemWithout)
	}
	// EVOLUTION RECORD (§20.2 user ruling 2026-07-07, not a regression):
	// the original "deficit 不参赛" score-neutrality clause is OVERTURNED for
	// non-inversion running rows — the eliminable deficit now IS the
	// attribution (effective/sort/Score), while the raw display channels
	// (ImpactMs bar / cumulative) stay fold-independent. A row without a
	// fold carries attribution 0 (authoritative — raw never drives ranking).
	if itemWith.ImpactMs != itemWithout.ImpactMs {
		t.Fatalf("raw display impact must stay fold-independent: %.6f vs %.6f", itemWith.ImpactMs, itemWithout.ImpactMs)
	}
	if !floatNear(rootCauseEffectiveImpactMs(itemWith), 5.0) || !floatNear(itemWith.Score, 5.0*0.86*rootCauseScoreWeightChainImpact) {
		t.Fatalf("§20.2: running attribution/Score must be deficit-based, got eff=%.6f score=%.6f", rootCauseEffectiveImpactMs(itemWith), itemWith.Score)
	}
	if !floatNear(rootCauseEffectiveImpactMs(itemWithout), 0) || !floatNear(itemWithout.Score, 0) {
		t.Fatalf("§20.2: un-folded running attribution/Score must be 0, got eff=%.6f score=%.6f", rootCauseEffectiveImpactMs(itemWithout), itemWithout.Score)
	}

	// Aggregate-backed rank row mirror.
	aggItem := rootCauseItemFromCausalAggregate(agg)
	if aggItem.SupplyFoldBasis == nil || !floatNear(aggItem.SupplyFoldDeficitMs, 7.5) {
		t.Fatalf("aggregate rank row must mirror the fold accounting: %+v", aggItem)
	}
	noFoldAgg := agg
	noFoldAgg.SupplyFoldDeficitMs, noFoldAgg.SupplyFoldIdealMs, noFoldAgg.SupplyFoldBasis = 0, 0, nil
	if !floatNear(aggItem.Score, 7.5*0.82*rootCauseScoreWeightChainAggregate) {
		t.Fatalf("§20.2 aggregate mirror: Score must be deficit-based, got %.6f", aggItem.Score)
	}
	if b := rootCauseItemFromCausalAggregate(noFoldAgg).Score; !floatNear(b, 0) {
		t.Fatalf("§20.2 aggregate mirror: un-folded running aggregate Score must be 0, got %.6f", b)
	}
}

// VS-2b limit>observed shape (R5c 终判 §29.96.1: 两法同解 — under max() the
// 2.5GHz limits ceiling IS the maximum possible frequency point, so the
// retired priority ladder and the max() rule resolve identically here):
// fmax 2.5GHz instead of the observed 2GHz, so both slices carry deficit.
// The limits ceiling sits ABOVE everything observed, so no throttling
// finding.
func TestSupplyFoldFmaxLadderPrefersLimits(t *testing.T) {
	idx := buildTraceIndex(t, "vs2b_limits.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
      <idle>-0 (-----) [007] .... 4.950000: cpu_frequency_limits: min=500000 max=2500000 cpu_id=7
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.010000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [007] .... 5.019900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [007] .... 5.020000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil {
		t.Fatalf("fold must run: %+v", dep)
	}
	if dep.SupplyFoldBasis.FmaxKHz != 2500000 || dep.SupplyFoldBasis.FmaxSource != SupplyFoldFmaxSourceLimit {
		t.Fatalf("governing limits row must be the fmax authority (2.5GHz/limit), got %+v", dep.SupplyFoldBasis)
	}
	// CAP (§26) evolution: 10×(1−(1/2.5)/2.53) + 9.9×(1−2/2.5) ≈ 10.40ms
	// (pre-CAP pure ratio ≈ 8ms).
	if dep.SupplyFoldDeficitMs < 10.1 || dep.SupplyFoldDeficitMs > 10.7 {
		t.Fatalf("deficit against the 2.5GHz policy ceiling should be ~10.4ms (CAP §26), got %.3f", dep.SupplyFoldDeficitMs)
	}
	if dep.SupplyFoldBasis.LimitThrottled {
		t.Fatalf("limits above every observed sample must NOT raise the throttling finding: %+v", dep.SupplyFoldBasis)
	}
	// Identity survives the ladder swap.
	if got, want := dep.SupplyFoldIdealMs+dep.SupplyFoldDeficitMs, dep.RunningMs; !floatNear(got, want) {
		t.Fatalf("ideal+deficit must reconstruct RunningMs: %.6f != %.6f", got, want)
	}
}

// VS-2b companion finding + R5c 反向形 pin: the big cluster's limits.Max
// (1.5GHz) sits BELOW a frequency the same cluster demonstrably reached in
// the trace (2GHz before the window — the observed>limit / 整文件被压 shape).
// EVOLUTION RECORD (R5c 终判 §29.96.1, 2026-07-15): the fold basis now takes
// the OBSERVED peak via max() (最大可能频点 — the pressed policy ceiling no
// longer understates the basis), with the source token attributing the
// observed lane; the typed LimitThrottled DISCLOSURE (limits sat below the
// full-trace observed maximum) is value-independent and stays raised (照旧走
// 披露车道不改基准, R5b 族).
func TestSupplyFoldLimitThrottledFinding(t *testing.T) {
	idx := buildTraceIndex(t, "vs2b_throttled.systrace", `
      <idle>-0 (-----) [007] .... 4.000000: cpu_frequency: state=2000000 cpu_id=7
      <idle>-0 (-----) [007] .... 4.800000: cpu_frequency_limits: min=500000 max=1500000 cpu_id=7
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=1400000 cpu_id=7
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [002] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil {
		t.Fatalf("fold must run: %+v", dep)
	}
	basis := dep.SupplyFoldBasis
	// R5c max(): the observed 2GHz peak beats the pressed 1.5GHz ceiling —
	// basis value AND source attribution follow the winning lane.
	if basis.FmaxKHz != 2000000 || basis.FmaxSource != SupplyFoldFmaxSourceObserved {
		t.Fatalf("R5c 反向形: observed>limit must resolve the basis to the observed peak (2GHz/observed), got %+v", basis)
	}
	if !basis.LimitThrottled || basis.PolicyCeilingKHz != 1500000 || basis.TraceObservedMaxKHz != 2000000 {
		t.Fatalf("limits below the cluster's full-trace 2GHz sample must raise the typed throttling finding: %+v", basis)
	}
	// ~9.9ms @1GHz small against big fmax 2GHz (R5c max() basis):
	// 9.9×(1−(1/2)/2.53) ≈ 7.94ms (pre-R5c limit basis gave ≈7.29ms).
	if dep.SupplyFoldDeficitMs < 7.6 || dep.SupplyFoldDeficitMs > 8.3 {
		t.Fatalf("deficit should fold against the observed-peak basis (~7.94ms, R5c), got %.3f", dep.SupplyFoldDeficitMs)
	}
}

func floatNear(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 0.05
}
