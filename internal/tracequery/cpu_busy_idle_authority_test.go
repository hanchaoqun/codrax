package tracequery

import (
	"context"
	"testing"
)

func TestCPUBusyIdleSurvivesUnrelatedThreadIncarnationConflict(t *testing.T) {
	idx := buildTraceIndex(t, "cpu_busy_idle_unrelated_reuse.systrace", `
       worker-100 (  100) [000] .... 0.990000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=100 next_prio=20
          old-900 (  900) [001] .... 0.995000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=900 next_prio=20
       creator-7 (    7) [001] .... 1.005000: sched_wakeup_new: comm=new pid=900 prio=20 target_cpu=001
       worker-100 (  100) [000] .... 1.010000: sched_switch: prev_comm=worker prev_pid=100 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
      swapper-0 (    0) [000] .... 1.020000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=200 next_prio=20
        other-200 (  200) [000] .... 1.040000: sched_switch: prev_comm=other prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
	`)
	stats := ComputeWindowStats(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.05, Limit: 20})

	var cpu0 *CPUStats
	for i := range stats.CPU {
		if stats.CPU[i].CPU == 0 {
			cpu0 = &stats.CPU[i]
			break
		}
	}
	if cpu0 == nil {
		t.Fatalf("identity-independent CPU lane disappeared: %+v", stats.CPU)
	}
	if !near(cpu0.BusyMs, 30, 0.001) || !near(cpu0.IdleMs, 20, 0.001) ||
		cpu0.BusyIdleStatus != CPUBusyIdleStatusMeasured {
		t.Fatalf("CPU-global busy/idle must remain measured across unrelated TID reuse: %+v", cpu0)
	}
	if len(stats.TopRunning) != 2 || len(stats.ThreadCPULoad) != 2 || len(stats.ProcessCPULoad) != 0 {
		t.Fatalf("clean PID scheduler rows must survive while process composites stay closed: top=%+v thread=%+v process=%+v", stats.TopRunning, stats.ThreadCPULoad, stats.ProcessCPULoad)
	}
	for _, row := range stats.TopRunning {
		if row.Thread.PID == 900 {
			t.Fatalf("conflicting PID leaked into running rows: %+v", stats.TopRunning)
		}
	}
	if stats.SchedulerHeadCoverage == nil ||
		stats.SchedulerHeadCoverage.Status != "unknown" ||
		stats.SchedulerHeadCoverage.SubjectCensusStatus != "not_evaluated" {
		t.Fatalf("identity-dependent coverage must stay withdrawn: %+v", stats.SchedulerHeadCoverage)
	}
	if !containsSubstring(stats.Caveats, "cpu_busy_idle_identity_independent=true") ||
		!containsSubstring(stats.Caveats, "thread_identity_per_pid_fail_closed=true") ||
		!containsSubstring(stats.Caveats, "suppressed_pids=[900]") {
		t.Fatalf("split authority must be disclosed: %+v", stats.Caveats)
	}
	streamed, err := StreamStateCluster(context.Background(), idx.Path, Query{TimeStart: 1.0, TimeEnd: 1.05, Limit: 20}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if streamed.WindowStats == nil || len(streamed.WindowStats.TopRunning) != 2 ||
		!containsSubstring(streamed.Caveats, "thread_identity_per_pid_filtered=true") ||
		!containsSubstring(streamed.Caveats, "suppressed_pids=[900]") {
		t.Fatalf("streaming twin did not retain the clean PID rows: %+v", streamed.WindowStats)
	}
	for _, row := range streamed.WindowStats.TopRunning {
		if row.Thread.PID == 900 {
			t.Fatalf("streaming twin leaked conflicting PID: %+v", streamed.WindowStats.TopRunning)
		}
	}
}

func TestLifecycleAuditCapWithholdsEveryPIDDurationRow(t *testing.T) {
	idx := buildTraceIndex(t, "cpu_busy_idle_lifecycle_cap.systrace", `
       worker-100 (  100) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=100 next_prio=20
       worker-100 (  100) [000] .... 1.010000: sched_switch: prev_comm=worker prev_pid=100 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
	`)
	idx.threadIncarnationFailuresCapped = true
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.02, Limit: 20})
	if len(stats.TopRunning) != 0 || len(stats.ThreadCPULoad) != 0 || len(stats.StateChurn) != 0 {
		t.Fatalf("capped lifecycle audit cannot identify a safe PID contributor: %+v", stats)
	}
	if len(stats.CPU) != 1 || stats.CPU[0].BusyMs <= 0 {
		t.Fatalf("identity-independent CPU lane was suppressed by lifecycle audit cap: %+v", stats.CPU)
	}
	if !containsSubstring(stats.Caveats, "lifecycle_audit_truncated") ||
		!containsSubstring(stats.Caveats, "every PID-keyed scheduler duration row") {
		t.Fatalf("capped authority withdrawal was not disclosed: %+v", stats.Caveats)
	}
}

func TestCPUFrequencyOnlyRowMarksBusyIdleUnavailable(t *testing.T) {
	idx := buildTraceIndex(t, "cpu_frequency_only_busy_idle.systrace", `
          power-9 (    9) [003] .... 1.010000: cpu_frequency: state=1800000 cpu_id=3
	`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.02, Limit: 20})
	if len(stats.CPU) != 1 {
		t.Fatalf("expected one frequency-only CPU row: %+v", stats.CPU)
	}
	cpu := stats.CPU[0]
	if cpu.CPU != 3 || cpu.Frequency != 1800000 ||
		cpu.BusyIdleStatus != CPUBusyIdleStatusUnavailable ||
		cpu.BusyIdleReason != "no_sched_switch_observation" {
		t.Fatalf("frequency-only row must not mint measured zero busy/idle: %+v", cpu)
	}
}

func TestMeasuredZeroBusyRemainsDistinctFromUnavailable(t *testing.T) {
	idx := buildTraceIndex(t, "cpu_measured_zero_busy.systrace", `
      swapper-0 (    0) [000] .... 1.000000: sched_switch: prev_comm=worker prev_pid=100 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
	`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.05, Limit: 20})
	if len(stats.CPU) != 1 {
		t.Fatalf("expected one measured CPU row: %+v", stats.CPU)
	}
	cpu := stats.CPU[0]
	if cpu.BusyMs != 0 || !near(cpu.IdleMs, 50, 0.001) ||
		cpu.BusyIdleStatus != CPUBusyIdleStatusMeasured {
		t.Fatalf("a real measured zero must remain numeric and typed measured: %+v", cpu)
	}
}

func TestCoreClassFrequencyOnlyBusyIdleStaysUnavailable(t *testing.T) {
	stats := buildCoreClassStats([]CPUStats{{
		CPU: 3, Frequency: 1800000,
		BusyIdleStatus: CPUBusyIdleStatusUnavailable,
		BusyIdleReason: "no_sched_switch_observation",
	}}, nil, map[int]string{3: "big"}, "explicit")
	if len(stats) != 1 {
		t.Fatalf("expected one core class: %+v", stats)
	}
	core := stats[0]
	if core.Class != "big" || core.BusyIdleStatus != CPUBusyIdleStatusUnavailable ||
		core.BusyIdleReason != "no_measured_cpu_busy_idle" ||
		core.BusyMs != 0 || core.IdleMs != 0 ||
		core.ComputeSupplySignal != "class_frequency_observed" {
		t.Fatalf("frequency-only class must preserve frequency but withdraw busy/idle: %+v", core)
	}
}

func TestCoreClassMixedBusyIdleCoverageIsPartial(t *testing.T) {
	stats := buildCoreClassStats([]CPUStats{
		{CPU: 0, BusyMs: 0, IdleMs: 10, BusyIdleStatus: CPUBusyIdleStatusMeasured},
		{CPU: 1, Frequency: 1000, BusyIdleStatus: CPUBusyIdleStatusUnavailable, BusyIdleReason: "no_sched_switch_observation"},
	}, nil, map[int]string{0: "small", 1: "small"}, "explicit")
	if len(stats) != 1 {
		t.Fatalf("expected one core class: %+v", stats)
	}
	core := stats[0]
	if core.BusyIdleStatus != CPUBusyIdleStatusPartial ||
		core.BusyIdleReason != "partial_cpu_busy_idle_coverage" ||
		core.BusyMs != 0 || !near(core.IdleMs, 10, 0.001) {
		t.Fatalf("mixed measured/unavailable class must sum measured contributors only: %+v", core)
	}
}

func TestCoreClassMeasuredZeroRemainsMeasured(t *testing.T) {
	stats := buildCoreClassStats([]CPUStats{{
		CPU: 0, BusyMs: 0, IdleMs: 10, BusyIdleStatus: CPUBusyIdleStatusMeasured,
	}}, nil, map[int]string{0: "small"}, "explicit")
	if len(stats) != 1 || stats[0].BusyIdleStatus != CPUBusyIdleStatusMeasured ||
		stats[0].BusyMs != 0 || !near(stats[0].IdleMs, 10, 0.001) {
		t.Fatalf("measured zero class drifted: %+v", stats)
	}
}

func TestComputeSupplySummariesPropagateBusyIdleAvailability(t *testing.T) {
	// SUPPLYAVAIL (2026-07-24): the compute_supply row mirrors the source
	// CPU's busy/idle authority — a frequency-only unavailable CPU must not
	// hand the face a measured-looking zero.
	stats := WindowStats{
		CPU: []CPUStats{{
			CPU: 2, CoreClass: "big",
			BusyIdleStatus: CPUBusyIdleStatusUnavailable,
			BusyIdleReason: "no_sched_switch_observation",
		}},
		RunnableTop: []ThreadDuration{{
			Thread: ThreadRef{Comm: "worker", PID: 7}, CPU: 2, DurationMs: 5,
		}},
	}
	out := computeSupplySummaries(stats, 4)
	if len(out) != 1 {
		t.Fatalf("expected one supply row: %+v", out)
	}
	if out[0].CPUBusyIdleStatus != CPUBusyIdleStatusUnavailable ||
		out[0].CPUBusyIdleReason != "no_sched_switch_observation" ||
		out[0].CPUBusyMs != 0 || out[0].CPUIdleMs != 0 {
		t.Fatalf("unavailable CPU authority did not propagate: %+v", out[0])
	}
	stats.CPU[0] = CPUStats{CPU: 2, CoreClass: "big", BusyMs: 3, IdleMs: 2, BusyIdleStatus: CPUBusyIdleStatusMeasured}
	out = computeSupplySummaries(stats, 4)
	if len(out) != 1 || out[0].CPUBusyIdleStatus != CPUBusyIdleStatusMeasured ||
		out[0].CPUBusyMs != 3 || out[0].CPUIdleMs != 2 {
		t.Fatalf("measured CPU authority drifted: %+v", out[0])
	}
}
