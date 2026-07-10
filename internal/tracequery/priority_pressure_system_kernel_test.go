package tracequery

import (
	"strings"
	"testing"
)

// Berlin-scale pressure fixture: app-100 waits runnable for 102s while three
// legal Harmony RT priorities (139/140/159) run for 12s and two opaque raw
// system/kernel priorities (160/301) run for 90s. The raw 90s was previously
// swallowed by the high-priority account because the pressure gate used
// `prio>=41`; it must now remain observable only in its own typed bucket.
const berlinPriorityPressureTrace = `
        app-100 (100) [000] .... 1000.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=R ==> next_comm=rt139 next_pid=139 next_prio=139
      rt139-139 (139) [000] .... 1004.000000: sched_switch: prev_comm=rt139 prev_pid=139 prev_prio=139 prev_state=S ==> next_comm=rt140 next_pid=140 next_prio=140
      rt140-140 (140) [000] .... 1008.000000: sched_switch: prev_comm=rt140 prev_pid=140 prev_prio=140 prev_state=S ==> next_comm=rt159 next_pid=159 next_prio=159
      rt159-159 (159) [000] .... 1012.000000: sched_switch: prev_comm=rt159 prev_pid=159 prev_prio=159 prev_state=S ==> next_comm=sys160 next_pid=160 next_prio=160
     sys160-160 (160) [000] .... 1057.000000: sched_switch: prev_comm=sys160 prev_pid=160 prev_prio=160 prev_state=S ==> next_comm=irq301 next_pid=301 next_prio=301
     irq301-301 (301) [000] .... 1102.000000: sched_switch: prev_comm=irq301 prev_pid=301 prev_prio=301 prev_state=S ==> next_comm=app next_pid=100 next_prio=159
`

func berlinPriorityPressureQuery() Query {
	return Query{
		PID: 100, TimeStart: 1000, TimeEnd: 1102, MinDurationMs: 0.001,
		TraceFlavor: TraceFlavorHarmonyHitrace, Limit: 32,
	}
}

func TestBerlinSystemOrKernelPressureStaysOutOfHighPriorityBucket(t *testing.T) {
	idx := buildTraceIndex(t, "berlin_priority_pressure.systrace", berlinPriorityPressureTrace)
	q := berlinPriorityPressureQuery()
	stats := ComputeWindowStats(idx, q)
	if len(stats.CPUPressure) != 1 {
		t.Fatalf("expected one CPU pressure row, got %+v", stats.CPUPressure)
	}
	pressure := stats.CPUPressure[0]
	if !near(pressure.HighPriorityRunningMs, 12000, 0.001) ||
		!near(pressure.HighPriorityRunningOverlapMs, 12000, 0.001) {
		t.Fatalf("only 139/140/159 RT time may enter high-priority accounting: %+v", pressure)
	}
	if !near(pressure.SystemOrKernelRunningMs, 90000, 0.001) ||
		!near(pressure.SystemOrKernelRunningOverlapMs, 90000, 0.001) ||
		pressure.SystemOrKernelCompetitorCount != 2 {
		t.Fatalf("160/301 must occupy the separate raw competition bucket: %+v", pressure)
	}
	if pressure.HighPriorityRunningMs+pressure.SystemOrKernelRunningMs != pressure.RunningMs {
		t.Fatalf("RT/raw split must conserve the fixture's running account: %+v", pressure)
	}

	latency := BuildSchedulerLatencyStats(idx, q)
	if len(latency.Items) != 1 {
		t.Fatalf("expected the target's one 102s runnable wait, got %+v", latency.Items)
	}
	wait := latency.Items[0]
	if !near(wait.DurationMs, 102000, 0.001) ||
		!near(wait.HighPriorityRunningOverlapMs, 12000, 0.001) ||
		!near(wait.SystemOrKernelRunningOverlapMs, 90000, 0.001) ||
		wait.SystemOrKernelCompetitorCount != 2 {
		t.Fatalf("per-wait typed pressure split drifted: %+v", wait)
	}
	if strings.Contains(wait.Summary, "high_prio_overlap=102000") {
		t.Fatalf("raw system/kernel time leaked into high-priority wording: %s", wait.Summary)
	}

	if stats.SupplyPressureSummary == nil {
		t.Fatalf("Berlin pressure fixture must publish a supply summary")
	}
	supply := stats.SupplyPressureSummary
	if !near(supply.HighPriorityRunningMs, 12000, 0.001) ||
		!near(supply.SystemOrKernelRunningMs, 90000, 0.001) ||
		!near(supply.SystemOrKernelRunningOverlapMs, 90000, 0.001) ||
		supply.SystemOrKernelCompetitorCount != 2 {
		t.Fatalf("aggregate typed pressure split drifted: %+v", supply)
	}
	// CPUPressureMs retains its existing ruler (runnable + genuine RT); raw
	// system/kernel activity is disclosed but cannot inflate that hard account.
	if !near(supply.CPUPressureMs, 114000, 0.001) {
		t.Fatalf("raw time must not inflate cpu_pressure_ms, got %.3f", supply.CPUPressureMs)
	}

	if stats.CPUOccupancy == nil {
		t.Fatalf("priority bands must be published")
	}
	bands := map[string]CPUOccupancyPriorityBand{}
	for _, band := range stats.CPUOccupancy.PriorityBands {
		bands[band.Band] = band
	}
	rt, raw := bands["ohos_rt"], bands["system_or_kernel"]
	if !rt.HighPriority || !near(rt.RunningMs, 12000, 0.001) || rt.ThreadCount != 3 {
		t.Fatalf("139/140/159 RT occupancy band wrong: %+v", rt)
	}
	if raw.HighPriority || !near(raw.RunningMs, 90000, 0.001) || raw.ThreadCount != 2 {
		t.Fatalf("160/301 raw occupancy band wrong: %+v", raw)
	}
}

func TestHarmonyPriorityBoundaryAndMicrokernelInversionRemainGated(t *testing.T) {
	for _, prio := range []int{139, 140, 159} {
		class := classifyTracePriority(TraceFlavorHarmonyHitrace, prio)
		if class != "ohos_rt" || !isHighPriorityForPressure(TraceFlavorHarmonyHitrace, prio, class) {
			t.Fatalf("prio=%d must be typed/high-priority ohos_rt, class=%q", prio, class)
		}
	}
	for _, prio := range []int{160, 301} {
		class := classifyTracePriority(TraceFlavorHarmonyHitrace, prio)
		if class != "system_or_kernel" || isHighPriorityForPressure(TraceFlavorHarmonyHitrace, prio, class) {
			t.Fatalf("prio=%d must stay raw and outside high-priority pressure, class=%q", prio, class)
		}
	}

	// The newly corrected microkernel band remains fully comparable and can
	// mint an inversion candidate; the rank lane must still consume the gated
	// effective caliber, never the raw dominant duration.
	if got := dependencyPriorityRelation(TraceFlavorHarmonyHitrace, 159, 140, 1); got != "lower_priority_dependency" {
		t.Fatalf("140 dependency below 159 target must remain comparable, got %q", got)
	}
	impact := WakeupCausalImpact{
		Thread:     ThreadRef{Comm: "microkernel-dep", PID: 140},
		Window:     TimeWindow{StartTs: 5, EndTs: 5.020},
		ChainDepth: 1, OnChain: true, DominantState: string(StateRunning),
		DominantImpactMs: 20, RunningMs: 20, TotalMs: 20,
		PriorityInversionCandidate: true, PriorityInversionGatedMs: 7,
		GatedRunnableMs: 2, GatedRunningDeficitMs: 5,
	}
	item := rootCauseItemFromCausalImpact(impact)
	if item.Type != "priority_inversion_candidate" || item.ChainRelevance != "on_chain" ||
		!near(item.EffectiveImpactMs, 7, 0.001) || !near(rootCauseEffectiveImpactMs(item), 7, 0.001) {
		t.Fatalf("legal microkernel inversion must keep gated on-chain ranking: %+v", item)
	}
}
