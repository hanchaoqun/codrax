package tracequery

import (
	"fmt"
	"strings"
	"testing"
)

// VS-1 (§7.8, customer ruling, berlin vsync_cust case): a periodic signal
// source's in-period sleep is normal cadence — only runnable wait and signal
// lateness count as on-chain attribution. These pins use the berlin live
// numbers (VSyncGenerator→main, 6 occurrences, actual-window start intervals
// ≈8.3ms, E10 sleep total 36.256ms, runnable sum ≈0.105ms) plus the
// adversarial-review counterexamples (2026-07-04): branch top-K occurrence
// selection (F1), even-count median pull / gap carve (F3), and the
// min-occurrence + in-band ratio gates (F4).

// buildPeriodicVSyncChain builds the berlin-shaped chain: occurrence i starts
// at base + Σ intervals[0..i-1] (seconds), sleep-dominant, with the given
// per-occurrence sleep/runnable milliseconds. TargetBlockedMs defaults to the
// occurrence's own total (the target waited exactly as long as the waker's
// segment) — tests exercising the blocked-caliber lateness override it.
func buildPeriodicVSyncChain(intervalsMs, sleepMs, runnableMs []float64) ChainResult {
	base := 4520.100000
	start := base
	chain := ChainResult{Target: ThreadRef{PID: 4144, Comm: "main"}}
	for i := range sleepMs {
		if i > 0 {
			start += intervalsMs[i-1] / 1000
		}
		total := sleepMs[i] + runnableMs[i]
		end := start + total/1000
		chain.CausalImpacts = append(chain.CausalImpacts, WakeupCausalImpact{
			Thread:           ThreadRef{PID: 610, Comm: "VSyncGenerator"},
			ChainDepth:       1,
			DominantState:    string(StateSSleep),
			DominantImpactMs: sleepMs[i],
			TotalMs:          total,
			SleepMs:          sleepMs[i],
			RunnableMs:       runnableMs[i],
			TargetBlockedMs:  total,
			FragmentCount:    2,
			Window:           TimeWindow{StartTs: start, EndTs: end},
			ActualWindow:     TimeWindow{StartTs: start, EndTs: end},
			LineStart:        100 + i*10,
			LineEnd:          105 + i*10,
		})
	}
	return chain
}

var berlinVSyncIntervalsMs = []float64{8.302, 8.287, 8.315, 8.360, 8.298}
var berlinVSyncSleepMs = []float64{5.800, 6.400, 6.200, 5.900, 6.100, 5.856} // Σ = 36.256 (E10)
var berlinVSyncRunnableMs = []float64{0.018, 0.020, 0.015, 0.017, 0.016, 0.019}

// TestPeriodicSourceDetectionBerlinVSyncCadence pins the berlin detection: the
// ≈8.3ms cadence (lower median 8.302, all intervals in band) marks the pair
// periodic; the target waits (≈6ms each) never exceed one period, so lateness
// is exactly 0 — sub-tolerance interval jitter is NOISE and must not fabricate
// lateness (F1: 无假 lateness) — and the effective attribution collapses from
// the raw 36.256ms sleep to the runnable sum 0.105ms; every raw field is
// untouched.
func TestPeriodicSourceDetectionBerlinVSyncCadence(t *testing.T) {
	chain := buildPeriodicVSyncChain(berlinVSyncIntervalsMs, berlinVSyncSleepMs, berlinVSyncRunnableMs)
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if len(aggregates) != 1 {
		t.Fatalf("expected one VSyncGenerator aggregate, got %+v", aggregates)
	}
	agg := aggregates[0]
	if !agg.PeriodicSource {
		t.Fatalf("berlin ≈8.3ms cadence must detect as periodic source: %+v", agg)
	}
	if !near(agg.DetectedPeriodMs, 8.302, 0.001) {
		t.Fatalf("detected period should be the lower-median interval 8.302ms, got %.6f", agg.DetectedPeriodMs)
	}
	// Blocked caliber: every target wait ≈6ms < p=8.302 → zero lateness. The
	// pre-review interval-jitter reading (0.071ms) was noise, not lateness.
	if agg.LatenessMs != 0 {
		t.Fatalf("in-band jitter must not fabricate lateness, got %.6f", agg.LatenessMs)
	}
	if !near(agg.EffectivePeriodicImpactMs, 0.105, 0.001) {
		t.Fatalf("effective impact should be the runnable sum 0.105ms, got %.6f", agg.EffectivePeriodicImpactMs)
	}
	// Raw fields stay lossless: the discount NEVER rewrites the raw accounting.
	if !near(agg.SleepMs, 36.256, 0.001) || !near(agg.DominantImpactMs, 36.256, 0.001) ||
		!near(agg.ProjectedImpactMs, 36.256, 0.001) || !near(agg.RunnableMs, 0.105, 0.001) {
		t.Fatalf("raw sleep/dominant/projected/runnable sums must stay untouched: %+v", agg)
	}
	wantTail := "periodic_source=true detected_period=8.302ms lateness=0.000ms effective_impact=0.105ms"
	if !strings.Contains(agg.Summary, wantTail) {
		t.Fatalf("aggregate summary should publish the periodic accounting %q, got %q", wantTail, agg.Summary)
	}
	// Member occurrences are stamped in place; per-occurrence lateness is the
	// blocked caliber (all zero here), raw sleeps untouched.
	for i, impact := range chain.CausalImpacts {
		if !impact.PeriodicSource {
			t.Fatalf("member %d should carry PeriodicSource", i)
		}
		if !near(impact.DetectedPeriodMs, 8.302, 0.001) {
			t.Fatalf("member %d period mismatch: %.6f", i, impact.DetectedPeriodMs)
		}
		if impact.LatenessMs != 0 {
			t.Fatalf("member %d: sub-period target wait must carry zero lateness: %+v", i, impact)
		}
		if !near(impact.SleepMs, berlinVSyncSleepMs[i], 0.0001) {
			t.Fatalf("member %d raw sleep must stay untouched: %+v", i, impact)
		}
	}
	if !strings.Contains(chain.CausalImpacts[4].Summary, "periodic_source=true") {
		t.Fatalf("stamped member summary should republish the periodic accounting: %q", chain.CausalImpacts[4].Summary)
	}
}

// TestPeriodicSourceBranchSelectionImmunity pins the F1 counterexample: 8
// occurrences selected out of 12 segments (branch top-K), so the adjacent
// intervals mix p, 2p and 3p. The gap carve keeps p robust at 8.3ms, the
// multi-period gaps are observation gaps — no veto, no lateness — and the
// effective attribution is the runnable sum only (no fake lateness).
func TestPeriodicSourceBranchSelectionImmunity(t *testing.T) {
	intervals := []float64{8.3, 16.6, 8.3, 24.9, 8.3, 16.6, 8.3}
	sleeps := []float64{5.9, 6.1, 6.0, 6.2, 5.8, 6.3, 6.0, 5.9}
	runnables := []float64{0.020, 0.015, 0.018, 0.017, 0.016, 0.019, 0.014, 0.021}
	chain := buildPeriodicVSyncChain(intervals, sleeps, runnables)
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if len(aggregates) != 1 {
		t.Fatalf("expected one aggregate, got %+v", aggregates)
	}
	agg := aggregates[0]
	if !agg.PeriodicSource {
		t.Fatalf("non-adjacent occurrence selection must not break detection: %+v", agg)
	}
	if !near(agg.DetectedPeriodMs, 8.3, 0.001) {
		t.Fatalf("period must stay robust against 2p/3p gaps, got %.6f", agg.DetectedPeriodMs)
	}
	if agg.LatenessMs != 0 {
		t.Fatalf("observation gaps must not convert to lateness, got %.6f", agg.LatenessMs)
	}
	runnableSum := 0.0
	for _, r := range runnables {
		runnableSum += r
	}
	if !near(agg.EffectivePeriodicImpactMs, runnableSum, 0.001) {
		t.Fatalf("effective must be the runnable sum %.3f (no fake lateness), got %.6f", runnableSum, agg.EffectivePeriodicImpactMs)
	}
}

// TestPeriodicSourceTrueLatenessFromBlockedCaliber pins the F1 true-late pin:
// an occurrence whose TARGET waited 25ms against the 8.3ms cadence carries
// lateness 25−8.3=16.7ms — the customer semantics "the signal arrived later
// than expected" — regardless of the interval sequence, and the lateness
// enters the account without vetoing detection.
func TestPeriodicSourceTrueLatenessFromBlockedCaliber(t *testing.T) {
	intervals := []float64{8.3, 8.3, 8.3, 8.3, 8.3}
	sleeps := []float64{5.9, 6.1, 6.0, 24.9, 6.2, 5.8}
	runnables := []float64{0.020, 0.015, 0.018, 0.017, 0.016, 0.014}
	chain := buildPeriodicVSyncChain(intervals, sleeps, runnables)
	chain.CausalImpacts[3].TargetBlockedMs = 25.0 // the target's true wait for tick 3
	aggregates := aggregateWakeupCausalImpacts(&chain)
	agg := aggregates[0]
	if !agg.PeriodicSource {
		t.Fatalf("a late signal must not veto periodic detection: %+v", agg)
	}
	if !near(agg.DetectedPeriodMs, 8.3, 0.001) {
		t.Fatalf("period should stay 8.3ms, got %.6f", agg.DetectedPeriodMs)
	}
	if !near(chain.CausalImpacts[3].LatenessMs, 16.7, 0.001) {
		t.Fatalf("blocked=25ms at p=8.3 must carry lateness 16.7ms: %+v", chain.CausalImpacts[3])
	}
	if !near(agg.LatenessMs, 16.7, 0.001) {
		t.Fatalf("aggregate lateness should be the 16.7ms overage, got %.6f", agg.LatenessMs)
	}
	runnableSum := 0.0
	for _, r := range runnables {
		runnableSum += r
	}
	if !near(agg.EffectivePeriodicImpactMs, runnableSum+16.7, 0.001) {
		t.Fatalf("effective should be runnable %.3f + lateness 16.7, got %.6f", runnableSum, agg.EffectivePeriodicImpactMs)
	}
}

// TestPeriodicSourceRankConsumesDiscountedValue pins the rank face: the
// periodic row keeps its RAW window projection (ImpactMs/CumulativeImpactMs)
// but ranks/scores by the discounted attribution, so the real impact rows
// (berlin: RSUniRender running 4.115ms) outrank the cadence sleep and the
// primary root cause is no longer the in-period sleep row.
func TestPeriodicSourceRankConsumesDiscountedValue(t *testing.T) {
	chain := buildPeriodicVSyncChain(berlinVSyncIntervalsMs, berlinVSyncSleepMs, berlinVSyncRunnableMs)
	aggregates := aggregateWakeupCausalImpacts(&chain)
	periodic := rootCauseItemFromCausalAggregate(aggregates[0])
	if !periodic.PeriodicSource {
		t.Fatalf("rank item should carry the periodic flag: %+v", periodic)
	}
	if !near(periodic.ImpactMs, 36.256, 0.001) {
		t.Fatalf("raw window projection must stay on ImpactMs (lossless), got %.6f", periodic.ImpactMs)
	}
	if !near(periodic.EffectiveImpactMs, 0.105, 0.001) || !near(periodic.DetectedPeriodMs, 8.302, 0.001) || periodic.LatenessMs != 0 {
		t.Fatalf("rank item should carry the discounted attribution + cadence: %+v", periodic)
	}
	wantScore := periodic.EffectiveImpactMs * periodic.Confidence * 2.05
	if !near(periodic.Score, wantScore, 0.001) {
		t.Fatalf("periodic row must SCORE by the discounted value: got %.6f want %.6f", periodic.Score, wantScore)
	}

	running := rootCauseItem("running", ThreadRef{PID: 1946, Comm: "RSUniRenderThread"}, 4.115, 0.86, 300, 320, "wakeup_chain.causal_impacts", "RSUniRender running")
	running.Causality = "on_wakeup_chain"
	running.ChainRelevance = "on_chain"
	running.DominantState = string(StateRunning)
	items := []RootCauseRankItem{periodic, running}
	normalizeRootCauseEffectiveImpact(items)
	sortRootCauseRankItems(items, true)
	assignRootCauseRanksAndTiers(items)
	if items[0].Type != "running" || items[0].Rank != 1 {
		t.Fatalf("the real running row must outrank the periodic cadence sleep: %+v", items)
	}
	if !items[1].PeriodicSource || items[1].Rank != 2 {
		t.Fatalf("the periodic row must rank below by its discounted value: %+v", items)
	}
	if !near(items[1].EffectiveImpactMs, 0.105, 0.001) {
		t.Fatalf("normalize must NOT resurrect the raw sleep as effective impact: %+v", items[1])
	}
}

// TestPeriodicSourceNotDetectedForAperiodicIntervals pins the negative shape:
// chaotic high-variance intervals never set the periodic fields — the gap
// carve anchored on a noisy minimum must not absorb everything as "some
// multiple" — and the aggregate/rank rows keep their pre-VS-1 bytes.
func TestPeriodicSourceNotDetectedForAperiodicIntervals(t *testing.T) {
	chain := buildPeriodicVSyncChain([]float64{8.3, 12.9, 3.1, 9.9, 24.0}, berlinVSyncSleepMs, berlinVSyncRunnableMs)
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if len(aggregates) != 1 {
		t.Fatalf("expected one aggregate, got %+v", aggregates)
	}
	agg := aggregates[0]
	if agg.PeriodicSource || agg.DetectedPeriodMs != 0 || agg.LatenessMs != 0 || agg.EffectivePeriodicImpactMs != 0 {
		t.Fatalf("aperiodic intervals must not set any periodic field: %+v", agg)
	}
	if strings.Contains(agg.Summary, "periodic_source") {
		t.Fatalf("aperiodic aggregate summary must stay byte-identical (no periodic tail): %q", agg.Summary)
	}
	for i, impact := range chain.CausalImpacts {
		if impact.PeriodicSource || impact.DetectedPeriodMs != 0 || impact.LatenessMs != 0 || impact.EffectivePeriodicImpactMs != 0 {
			t.Fatalf("member %d must stay unstamped: %+v", i, impact)
		}
	}
	item := rootCauseItemFromCausalAggregate(agg)
	if item.PeriodicSource {
		t.Fatalf("rank item must not carry the periodic flag: %+v", item)
	}
	if !near(item.Score, item.ImpactMs*item.Confidence*2.05, 0.0001) {
		t.Fatalf("aperiodic rank score must keep the raw formula: %+v", item)
	}
}

// TestWakeupPeriodicCadenceLowerMedianAndGapCarve pins the F3 estimator
// counterexamples at the cadence level (the full gate additionally demands
// wakeupPeriodicMinOccurrences occurrences — see the F4 pins):
//   - [8.3, 25.0]: a two-interval sample whose plain even-count median
//     (16.65) sits between cadence bands and used to early-veto the real
//     8.3ms tick. Lower median anchors p=8.3 and 25.0≈3p carves out as an
//     observation gap — cadence reads clean, no veto.
//   - [8.3, 8.3, 25.0, 25.0]: TWO multi-period gaps (the "double late" shape)
//     — both carve out, p stays 8.3, no veto.
//   - [7.0, 8.3, 8.3]: a genuine early fire below p×0.85 → veto.
func TestWakeupPeriodicCadenceLowerMedianAndGapCarve(t *testing.T) {
	c, ok := wakeupPeriodicCadenceFromIntervals([]float64{8.3, 25.0})
	if !ok || c.EarlyVeto {
		t.Fatalf("[8.3,25.0] must read a clean cadence: %+v ok=%v", c, ok)
	}
	if !near(c.Period, 8.3, 0.001) {
		t.Fatalf("[8.3,25.0] lower-median period must be 8.3, got %.6f", c.Period)
	}
	if c.Gap[0] || !c.Gap[1] {
		t.Fatalf("25.0 ≈ 3p must carve out as an observation gap: %+v", c)
	}
	if c.Kept != 1 || c.InBand != 1 || c.InBand*3 < c.Kept*2 {
		t.Fatalf("[8.3,25.0] band accounting off: %+v", c)
	}

	c, ok = wakeupPeriodicCadenceFromIntervals([]float64{8.3, 8.3, 25.0, 25.0})
	if !ok || c.EarlyVeto {
		t.Fatalf("[8.3,8.3,25.0,25.0] must read a clean cadence: %+v ok=%v", c, ok)
	}
	if !near(c.Period, 8.3, 0.001) || !c.Gap[2] || !c.Gap[3] || c.Kept != 2 || c.InBand != 2 {
		t.Fatalf("double gaps must both carve out around p=8.3: %+v", c)
	}

	c, ok = wakeupPeriodicCadenceFromIntervals([]float64{7.0, 8.3, 8.3})
	if !ok {
		t.Fatalf("[7.0,8.3,8.3] must still read a cadence basis")
	}
	if !near(c.Period, 8.3, 0.001) || !c.EarlyVeto {
		t.Fatalf("a genuine early fire must veto (p=8.3): %+v", c)
	}
}

// TestPeriodicSourceGapCarveFullGate pins the F3 shape end-to-end at the full
// detection gate: [8.3, 8.3, 25.0, 25.0] = 5 occurrences with two observation
// gaps detects successfully, and a late target wait still enters the account
// (迟到入账不 veto) instead of reverting the row to raw-sleep attribution.
func TestPeriodicSourceGapCarveFullGate(t *testing.T) {
	sleeps := []float64{5.9, 6.1, 6.0, 6.2, 5.8}
	runnables := []float64{0.020, 0.015, 0.018, 0.017, 0.016}
	chain := buildPeriodicVSyncChain([]float64{8.3, 8.3, 25.0, 25.0}, sleeps, runnables)
	chain.CausalImpacts[2].TargetBlockedMs = 25.0 // the target's true wait for this tick
	aggregates := aggregateWakeupCausalImpacts(&chain)
	agg := aggregates[0]
	if !agg.PeriodicSource {
		t.Fatalf("gap-mixed cadence must detect (double gaps carve out): %+v", agg)
	}
	if !near(agg.DetectedPeriodMs, 8.3, 0.001) {
		t.Fatalf("period must be the carved lower median 8.3, got %.6f", agg.DetectedPeriodMs)
	}
	if !near(agg.LatenessMs, 16.7, 0.001) {
		t.Fatalf("the 25ms target wait must enter as 16.7ms lateness, got %.6f", agg.LatenessMs)
	}
}

// TestPeriodicSourceMinOccurrenceGate pins the F4 gate: a worker observed only
// 3 times at equal spacing no longer takes the discount — the trace records
// WHEN wakeups happen, never WHY, so a demand-driven worker woken twice at a
// coincidentally similar spacing is unobservably different from a generator.
// Five occurrences with a 2/3 in-band majority clear the bar.
func TestPeriodicSourceMinOccurrenceGate(t *testing.T) {
	sleeps3 := []float64{5.9, 6.1, 6.0}
	runnables3 := []float64{0.020, 0.015, 0.018}
	chain := buildPeriodicVSyncChain([]float64{8.3, 8.3}, sleeps3, runnables3)
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if aggregates[0].PeriodicSource {
		t.Fatalf("3 equal-spaced occurrences must NOT mark a periodic source: %+v", aggregates[0])
	}

	sleeps5 := []float64{5.9, 6.1, 6.0, 6.2, 5.8}
	runnables5 := []float64{0.020, 0.015, 0.018, 0.017, 0.016}
	chain = buildPeriodicVSyncChain([]float64{8.3, 8.3, 8.3, 12.0}, sleeps5, runnables5)
	aggregates = aggregateWakeupCausalImpacts(&chain)
	if !aggregates[0].PeriodicSource {
		t.Fatalf("5 occurrences with 3/4 in-band intervals must mark: %+v", aggregates[0])
	}
}

// TestPeriodicSourceBandRatioGate pins the F4 in-band ratio: when fewer than
// 2/3 of the carved intervals sit inside ±15% of p, the cadence is not the
// majority reading and the row keeps its raw attribution (no early fire and
// no gap involved — this is purely the ratio gate).
func TestPeriodicSourceBandRatioGate(t *testing.T) {
	sleeps := []float64{5.9, 6.1, 6.0, 6.2, 5.8, 6.3, 6.0}
	runnables := []float64{0.020, 0.015, 0.018, 0.017, 0.016, 0.019, 0.014}
	chain := buildPeriodicVSyncChain([]float64{8.3, 8.3, 8.3, 10.0, 10.0, 10.0}, sleeps, runnables)
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if aggregates[0].PeriodicSource {
		t.Fatalf("3/6 in-band intervals must fail the 2/3 ratio gate: %+v", aggregates[0])
	}
}

// TestPeriodicSourceEarlyFireVetoesDetection pins the early-fire veto at the
// full gate: an interval below p×0.85 means the pair is not a fixed-period
// source.
func TestPeriodicSourceEarlyFireVetoesDetection(t *testing.T) {
	sleeps := []float64{5.9, 6.1, 6.0, 6.2, 5.8}
	runnables := []float64{0.020, 0.015, 0.018, 0.017, 0.016}
	chain := buildPeriodicVSyncChain([]float64{8.3, 6.0, 8.3, 8.3}, sleeps, runnables)
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if aggregates[0].PeriodicSource {
		t.Fatalf("an early fire beyond tolerance must veto detection: %+v", aggregates[0])
	}
}

// TestPeriodicSourceRestrictedToSleepDominantRows pins the state gate: the
// customer ruling discounts in-period SLEEP; a runnable-dominant pair with a
// perfect cadence keeps its bytes (runnable already counts precisely).
func TestPeriodicSourceRestrictedToSleepDominantRows(t *testing.T) {
	chain := buildPeriodicVSyncChain(berlinVSyncIntervalsMs, berlinVSyncSleepMs, berlinVSyncRunnableMs)
	for i := range chain.CausalImpacts {
		impact := &chain.CausalImpacts[i]
		impact.DominantState = string(StateRunnable)
		impact.RunnableMs, impact.SleepMs = impact.SleepMs, impact.RunnableMs
		impact.DominantImpactMs = impact.RunnableMs
	}
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if aggregates[0].PeriodicSource {
		t.Fatalf("runnable-dominant rows must never take the periodic discount: %+v", aggregates[0])
	}
}

// TestPeriodicSourceLatenessCapAtRawMinusRunnable pins the F1(c) fabrication
// cap at the aggregate SUM site: occurrences sharing one branch window can
// each report the same huge target wait, but the published aggregate lateness
// can never exceed raw blocking − runnable, and the effective attribution can
// never exceed the raw blocking value — no invented number reaches the
// Summary.
func TestPeriodicSourceLatenessCapAtRawMinusRunnable(t *testing.T) {
	sleeps := []float64{5.9, 6.1, 6.0, 6.2, 5.8, 6.3}
	runnables := []float64{0.020, 0.015, 0.018, 0.017, 0.016, 0.019}
	chain := buildPeriodicVSyncChain([]float64{8.3, 8.3, 8.3, 8.3, 8.3}, sleeps, runnables)
	for i := range chain.CausalImpacts {
		chain.CausalImpacts[i].TargetBlockedMs = 100.0 // shared-branch double count shape
	}
	aggregates := aggregateWakeupCausalImpacts(&chain)
	agg := aggregates[0]
	if !agg.PeriodicSource {
		t.Fatalf("late ticks must still detect: %+v", agg)
	}
	raw := aggregateBlockingMs(agg)
	if agg.LatenessMs > raw-agg.RunnableMs+0.0001 {
		t.Fatalf("aggregate lateness must cap at raw−runnable (%.3f), got %.3f", raw-agg.RunnableMs, agg.LatenessMs)
	}
	if agg.EffectivePeriodicImpactMs > raw+0.0001 {
		t.Fatalf("a discount can never inflate: effective %.3f > raw %.3f", agg.EffectivePeriodicImpactMs, raw)
	}
	if fmt.Sprintf("%.3f", agg.EffectivePeriodicImpactMs) != fmt.Sprintf("%.3f", raw) {
		t.Fatalf("the capped effective should equal the raw blocking value %.3f, got %.3f", raw, agg.EffectivePeriodicImpactMs)
	}
	wantTail := fmt.Sprintf("lateness=%.3fms effective_impact=%.3fms", agg.LatenessMs, agg.EffectivePeriodicImpactMs)
	if !strings.Contains(agg.Summary, wantTail) {
		t.Fatalf("the Summary must publish the CAPPED numbers %q: %q", wantTail, agg.Summary)
	}
}
