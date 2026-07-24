package tracequery

import "testing"

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
	if len(stats.TopRunning) != 0 || len(stats.ThreadCPULoad) != 0 || len(stats.ProcessCPULoad) != 0 {
		t.Fatalf("PID-keyed scheduler faces were accidentally reopened: top=%+v thread=%+v process=%+v", stats.TopRunning, stats.ThreadCPULoad, stats.ProcessCPULoad)
	}
	if stats.SchedulerHeadCoverage == nil ||
		stats.SchedulerHeadCoverage.Status != "unknown" ||
		stats.SchedulerHeadCoverage.SubjectCensusStatus != "not_evaluated" {
		t.Fatalf("identity-dependent coverage must stay withdrawn: %+v", stats.SchedulerHeadCoverage)
	}
	if !containsSubstring(stats.Caveats, "cpu_busy_idle_identity_independent=true") ||
		!containsSubstring(stats.Caveats, "thread_identity_fail_closed=true") {
		t.Fatalf("split authority must be disclosed: %+v", stats.Caveats)
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
