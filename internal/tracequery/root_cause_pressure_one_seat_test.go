package tracequery

import "testing"

func pressureOneSeatStats(supplyMS float64) WindowStats {
	return WindowStats{
		Window: TimeWindow{StartTs: 5.000, EndTs: 5.007},
		CPUPressure: []CPUPressureStats{
			{CPU: 1, RunnableWaitMs: 0.300, HighPriorityRunningMs: 1.200, RunningMs: 1.200},
			{CPU: 2, RunnableWaitMs: 0.500, RunningMs: 4.600},
		},
		SupplyPressureSummary: &SupplyPressureSummary{
			CPUPressureMs: supplyMS, RunnableWaitMs: 0.800,
			HighPriorityRunningMs: 1.200, WindowMs: 7.000,
			PressureDensity: supplyMS / 7.000, Summary: "typed pressure summary",
		},
	}
}

func TestRootCausePressureAggregateOwnsOneExactRankSeat(t *testing.T) {
	stats := pressureOneSeatStats(0.800)
	if !rootCauseSupplyPressureOwnsCPURankSeat(stats) {
		t.Fatal("exact aggregate must own the pressure rank seat")
	}
	rank := buildRootCauseRankFrom(nil, Query{TimeStart: 5.000, TimeEnd: 5.007}, ChainResult{}, stats)
	counts := map[string]int{}
	for _, item := range rank.Items {
		counts[item.Type]++
	}
	if counts["supply_pressure"] != 1 || counts["cpu_pressure"] != 0 {
		t.Fatalf("one demand account must own one aggregate rank seat: counts=%v items=%+v", counts, rank.Items)
	}
	if len(stats.CPUPressure) != 2 {
		t.Fatalf("per-CPU typed drilldown must remain lossless: %+v", stats.CPUPressure)
	}
}

func TestRootCausePressureSeatGateFailsOpenOnAggregateMismatch(t *testing.T) {
	stats := pressureOneSeatStats(0.801)
	if rootCauseSupplyPressureOwnsCPURankSeat(stats) {
		t.Fatal("a mismatched aggregate must not absorb per-CPU seats")
	}
	rank := buildRootCauseRankFrom(nil, Query{TimeStart: 5.000, TimeEnd: 5.007}, ChainResult{}, stats)
	counts := map[string]int{}
	for _, item := range rank.Items {
		counts[item.Type]++
	}
	if counts["supply_pressure"] != 1 || counts["cpu_pressure"] == 0 {
		t.Fatalf("mismatched accounts must remain separately auditable: counts=%v items=%+v", counts, rank.Items)
	}
}

func TestSupplyPressureDoesNotMintQueueTimeFromRunningOnly(t *testing.T) {
	stats := WindowStats{CPUPressure: []CPUPressureStats{{CPU: 1, HighPriorityRunningMs: 1.200, RunningMs: 1.200}}}
	supply := computeSupplyPressureSummary(nil, Query{TimeStart: 5.000, TimeEnd: 5.007}, stats, 4)
	if supply == nil {
		t.Fatal("running-only occupancy context must remain observable")
	}
	if supply.CPUPressureMs != 0 || supply.RunnableWaitMs != 0 || supply.HighPriorityRunningMs != 1.200 {
		t.Fatalf("running-only context must never mint queue time: %+v", supply)
	}
	rankStats := stats
	rankStats.SupplyPressureSummary = supply
	rank := buildRootCauseRankFrom(nil, Query{TimeStart: 5.000, TimeEnd: 5.007}, ChainResult{}, rankStats)
	for _, item := range rank.Items {
		if item.Type == "supply_pressure" || item.Type == "cpu_pressure" {
			t.Fatalf("running-only occupancy must not mint a pressure rank seat: %+v", rank.Items)
		}
	}
}
