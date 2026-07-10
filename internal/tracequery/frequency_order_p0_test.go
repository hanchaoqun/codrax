package tracequery

import (
	"context"
	"strings"
	"testing"
)

func TestCPUFrequencyRollbackFailsClosedPerCPUAndBlocksDonorReuse(t *testing.T) {
	idx := buildTraceIndex(t, "frequency-rollback.systrace", strings.Join([]string{
		`idle-0 (0) [001] .... 0.800000: cpu_frequency: state=1200000 cpu_id=1`,
		`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=800000 cpu_id=0`,
		`idle-0 (0) [000] .... 1.100000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=42 next_prio=120`,
		`idle-0 (0) [000] .... 2.000000: cpu_frequency: state=1000000 cpu_id=0`,
		`idle-0 (0) [001] .... 2.100000: cpu_frequency: state=1600000 cpu_id=1`,
		// Same physical cpu0 lane moves backwards; timestamp sorting must not
		// launder this into a plausible 1.0 -> 1.5 -> 2.0 timeline.
		`idle-0 (0) [000] .... 1.500000: cpu_frequency: state=900000 cpu_id=0`,
		`app-42 (42) [000] .... 2.500000: sched_switch: prev_comm=app prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
	}, "\n")+"\n")

	q := Query{TimeStart: 1, TimeEnd: 2.5, CoreTopology: "little=0-1"}
	stats := ComputeWindowStats(idx, q)
	if !containsSubstring(stats.Caveats, "frequency_timeline_fail_closed=true") ||
		!containsSubstring(stats.Caveats, "family=cpu_frequency affected_cpus=[0]") {
		t.Fatalf("missing per-CPU frequency fail-close caveat: %v", stats.Caveats)
	}
	for _, cpu := range stats.CPU {
		if cpu.CPU == 0 && (cpu.Frequency != 0 || len(cpu.FrequencyResidency) != 0) {
			t.Fatalf("regressed cpu0 frequency lane survived residency: %+v", cpu)
		}
	}
	if timelines := indexFreqSampleTimelines(idx); len(timelines[0]) != 0 || len(timelines[1]) == 0 {
		t.Fatalf("trace-global topology basis did not remove only cpu0: %+v", timelines)
	}
	if stats.ComputeSupplyBalance == nil {
		t.Fatal("bounded scheduler window must publish compute supply")
	}
	foundCPU0 := false
	for _, per := range stats.ComputeSupplyBalance.PerCPU {
		if per.CPU != 0 {
			continue
		}
		foundCPU0 = true
		if per.FrequencyKnown || per.FrequencyClusterDonorCPU != nil || per.MaxFrequencyKHz != 0 {
			t.Fatalf("same-cluster donor bypassed cpu0 fail-close: %+v", per)
		}
	}
	if !foundCPU0 {
		t.Fatalf("fixture did not produce cpu0 supply row: %+v", stats.ComputeSupplyBalance.PerCPU)
	}

	cache := newChainQueryCache(idx)
	_, basis := cache.supplyFoldRunningIntervals(q, 1.1, 2.5, []Interval{{
		State: StateRunning, StartTs: 1.1, EndTs: 2.5, DurationMs: 1400, CPU: 0, CPUKnown: true,
	}})
	if basis.UnknownMs != 1400 || len(basis.ClusterFreqReuse) != 0 {
		t.Fatalf("chain fold replaced a poisoned cpu0 lane with donor/rail data: %+v", basis)
	}
}

func TestCPUFrequencyLimitRollbackFailsClosedPerCPU(t *testing.T) {
	idx := buildTraceIndex(t, "frequency-limit-rollback.systrace", strings.Join([]string{
		`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1000000 cpu_id=0`,
		`idle-0 (0) [001] .... 1.010000: cpu_frequency: state=1200000 cpu_id=1`,
		`idle-0 (0) [000] .... 1.100000: cpu_frequency_limits: min=400000 max=1500000 cpu_id=0`,
		`idle-0 (0) [001] .... 1.200000: cpu_frequency_limits: min=400000 max=1800000 cpu_id=1`,
		`idle-0 (0) [000] .... 2.000000: cpu_frequency_limits: min=400000 max=1400000 cpu_id=0`,
		`idle-0 (0) [001] .... 2.100000: cpu_frequency_limits: min=400000 max=1700000 cpu_id=1`,
		`idle-0 (0) [000] .... 1.500000: cpu_frequency_limits: min=400000 max=1300000 cpu_id=0`,
	}, "\n")+"\n")

	q := Query{TimeStart: 1, TimeEnd: 2.5, CoreTopology: "little=0;big=1"}
	stats := ComputeWindowStats(idx, q)
	if !containsSubstring(stats.Caveats, "family=cpu_frequency_limits affected_cpus=[0]") {
		t.Fatalf("missing per-CPU limits fail-close caveat: %v", stats.Caveats)
	}
	if containsSubstring(stats.Caveats, "family=cpu_frequency affected_cpus=") {
		t.Fatalf("limits rollback poisoned the independent frequency family: %v", stats.Caveats)
	}
	for _, limit := range stats.CPUFrequencyLimits {
		if limit.CPU == 0 {
			t.Fatalf("regressed cpu0 policy ceiling survived display aggregation: %+v", stats.CPUFrequencyLimits)
		}
	}
	foundCPU1 := false
	for _, limit := range stats.CPUFrequencyLimits {
		foundCPU1 = foundCPU1 || limit.CPU == 1
	}
	if !foundCPU1 {
		t.Fatalf("healthy cpu1 limits were suppressed: %+v", stats.CPUFrequencyLimits)
	}
	cache := newChainQueryCache(idx)
	if got := cache.governedLimitMaxKHz(0, 1, 2.5); got != 0 {
		t.Fatalf("chain/fold limits index bypassed cpu0 fail-close: %d", got)
	}
	if got := cache.governedLimitMaxKHz(1, 1, 2.5); got != 1800000 {
		t.Fatalf("healthy cpu1 limit drifted: %d", got)
	}
}

func TestColdWindowFrequencyAuditKeepsOutOfWindowRollbackPoison(t *testing.T) {
	resetAnchorCaches()
	path := writeSchedulerCarryTrace(t, "window-frequency-rollback.systrace",
		`idle-0 (0) [000] .... 2.000000: cpu_frequency: state=1000000 cpu_id=0`,
		`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=800000 cpu_id=0`,
		`idle-0 (0) [001] .... 2.100000: cpu_frequency: state=1500000 cpu_id=1`,
	)
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		AllowWindowedParse: true,
		TimeStartSet:       true,
		TimeStart:          1.8,
		TimeEndSet:         true,
		TimeEnd:            2.2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !idx.Windowed {
		t.Fatal("fixture must exercise the cold windowed raw audit")
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.8, TimeEnd: 2.2})
	if !containsSubstring(stats.Caveats, "family=cpu_frequency affected_cpus=[0]") {
		t.Fatalf("window pruning lost the physical frequency rollback: %+v", stats.Caveats)
	}
	for _, cpu := range stats.CPU {
		if cpu.CPU == 0 && (cpu.Frequency != 0 || len(cpu.FrequencyResidency) != 0) {
			t.Fatalf("windowed face published the poisoned cpu0 timeline: %+v", cpu)
		}
	}
}
