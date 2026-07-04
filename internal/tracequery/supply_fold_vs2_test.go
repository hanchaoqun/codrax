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
// governed fmax is 2GHz — fold to ideal 5+10=15ms, deficit 5ms; a slice with
// no frequency data folds at ratio 1 (zero fabricated deficit, UNKNOWN
// basis); the deficit is clamped ≥ 0 even when a slice is governed above the
// big-cluster fmax; raw pre-window frequency history never participates.

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

// The ledger's synthetic pin: 10ms@1GHz small + 10ms@2GHz big, big fmax=2GHz
// → ideal = 10×(1/2) + 10×(2/2) = 15ms, deficit = 5ms, basis fully known.
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
	if dep.SupplyFoldDeficitMs < 4.7 || dep.SupplyFoldDeficitMs > 5.3 {
		t.Fatalf("deficit should be the ~5ms 1GHz-vs-2GHz fold, got %.3f", dep.SupplyFoldDeficitMs)
	}
	if dep.SupplyFoldIdealMs < 14.7 || dep.SupplyFoldIdealMs > 15.3 {
		t.Fatalf("ideal should be ~15ms, got %.3f", dep.SupplyFoldIdealMs)
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

// A slice on a CPU with NO frequency samples folds at ratio 1 — no fabricated
// deficit — and books as UNKNOWN basis (§7.10 无频点数据 rule).
func TestSupplyFoldUnknownFrequencySliceZeroDeficit(t *testing.T) {
	idx := buildTraceIndex(t, "vs2_unknown_slice.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
        dep-200 (100) [003] .... 5.010000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [003] .... 5.019900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [003] .... 5.020000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil {
		t.Fatalf("fold must run: %+v", dep)
	}
	// Only the 1GHz slice folds (10×0.5=5ms deficit); the no-sample CPU3
	// slice contributes ideal=wall and UNKNOWN basis.
	if dep.SupplyFoldDeficitMs < 4.7 || dep.SupplyFoldDeficitMs > 5.3 {
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
}

// CMP-10 F1 caliber: raw pre-window history MUST NOT participate — a 3GHz
// burst long before the window (superseded by the 2GHz head-governing sample)
// must not raise the big-cluster fmax. Deficit stays the 1GHz-vs-2GHz 5ms,
// not the 6.67ms a 3GHz fmax would fabricate.
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
	// ~10ms @1GHz against governed fmax 2GHz → ~5ms. A 3GHz fmax would give
	// ~6.67ms — the pre-window burst leaking in.
	if dep.SupplyFoldDeficitMs < 4.7 || dep.SupplyFoldDeficitMs > 5.3 {
		t.Fatalf("pre-window 3GHz burst must not participate in fmax, got deficit %.3f", dep.SupplyFoldDeficitMs)
	}
}

// Clamp: under explicit topology the big cluster's governed fmax can sit
// BELOW another cluster's governed frequency — the ratio clamps at 1 so a
// faster-than-big slice yields zero deficit, never a negative one. This is
// also the affirmative fourth-branch shape: deficit 0 on a fully-known basis.
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
	if itemWith.Score != itemWithout.Score || itemWith.ImpactMs != itemWithout.ImpactMs {
		t.Fatalf("fold must not move score/impact (deficit 不参赛): %.6f vs %.6f", itemWith.Score, itemWithout.Score)
	}

	// Aggregate-backed rank row mirror.
	aggItem := rootCauseItemFromCausalAggregate(agg)
	if aggItem.SupplyFoldBasis == nil || !floatNear(aggItem.SupplyFoldDeficitMs, 7.5) {
		t.Fatalf("aggregate rank row must mirror the fold accounting: %+v", aggItem)
	}
	noFoldAgg := agg
	noFoldAgg.SupplyFoldDeficitMs, noFoldAgg.SupplyFoldIdealMs, noFoldAgg.SupplyFoldBasis = 0, 0, nil
	if a, b := rootCauseItemFromCausalAggregate(agg).Score, rootCauseItemFromCausalAggregate(noFoldAgg).Score; a != b {
		t.Fatalf("aggregate fold must not move score: %.6f vs %.6f", a, b)
	}
}

// VS-2b (§7.10 fmax ladder): a window-governing cpu_frequency_limits row on
// the big cluster is the POLICY authority and outranks the observed
// governance samples — fmax 2.5GHz instead of the observed 2GHz, so both
// slices now carry deficit (10×(1−1/2.5) + 10×(1−2/2.5) = 8ms). The limits
// ceiling sits ABOVE everything observed, so no throttling finding.
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
	if dep.SupplyFoldDeficitMs < 7.7 || dep.SupplyFoldDeficitMs > 8.3 {
		t.Fatalf("deficit against the 2.5GHz policy ceiling should be ~8ms, got %.3f", dep.SupplyFoldDeficitMs)
	}
	if dep.SupplyFoldBasis.LimitThrottled {
		t.Fatalf("limits above every observed sample must NOT raise the throttling finding: %+v", dep.SupplyFoldBasis)
	}
	// Identity survives the ladder swap.
	if got, want := dep.SupplyFoldIdealMs+dep.SupplyFoldDeficitMs, dep.RunningMs; !floatNear(got, want) {
		t.Fatalf("ideal+deficit must reconstruct RunningMs: %.6f != %.6f", got, want)
	}
}

// VS-2b companion finding: the big cluster's governing limits.Max (1.5GHz)
// sits BELOW a frequency the same cluster demonstrably reached elsewhere in
// the trace (2GHz before the window) → typed LimitThrottled with the
// full-trace observed maximum recorded; the fold itself still divides by the
// policy ceiling (limits stay the window authority).
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
	if basis.FmaxKHz != 1500000 || basis.FmaxSource != SupplyFoldFmaxSourceLimit {
		t.Fatalf("throttled window must still fold against the 1.5GHz policy ceiling, got %+v", basis)
	}
	if !basis.LimitThrottled || basis.TraceObservedMaxKHz != 2000000 {
		t.Fatalf("limits below the cluster's full-trace 2GHz sample must raise the typed throttling finding: %+v", basis)
	}
	// ~10ms @1GHz against fmax 1.5GHz → ~3.33ms deficit.
	if dep.SupplyFoldDeficitMs < 3.0 || dep.SupplyFoldDeficitMs > 3.7 {
		t.Fatalf("deficit should fold against the policy ceiling (~3.33ms), got %.3f", dep.SupplyFoldDeficitMs)
	}
}

func floatNear(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 0.05
}
