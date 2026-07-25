package tracequery

import "testing"

func TestPerPIDIdentityNarrowingRetainsCleanOffCPUAndChurnRows(t *testing.T) {
	idx := buildTraceIndex(t, "per_pid_identity_offcpu.systrace", `
          old-900 (  900) [001] .... 0.995000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=900 next_prio=20
          app-200 (  200) [000] .... 1.000000: sched_wakeup: comm=worker pid=100 prio=20 target_cpu=000
          old-900 (  900) [001] .... 1.005000: sched_switch: prev_comm=old prev_pid=900 prev_prio=20 prev_state=X ==> next_comm=swapper/1 next_pid=0 next_prio=120
        swapper-0 (    0) [000] .... 1.010000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=100 next_prio=20
       worker-100 (  100) [000] .... 1.020000: sched_switch: prev_comm=worker prev_pid=100 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
        creator-7 (    7) [001] .... 1.025000: sched_wakeup_new: comm=new pid=900 prio=20 target_cpu=001
          app-200 (  200) [000] .... 1.030000: sched_wakeup: comm=worker pid=100 prio=20 target_cpu=000
        swapper-1 (    0) [001] .... 1.035000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=new next_pid=900 next_prio=20
        swapper-0 (    0) [000] .... 1.040000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=100 next_prio=20
          new-900 (  900) [001] .... 1.045000: sched_switch: prev_comm=new prev_pid=900 prev_prio=20 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
       worker-100 (  100) [000] .... 1.050000: sched_switch: prev_comm=worker prev_pid=100 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
	`)
	stats := ComputeWindowStats(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.06, MinDurationMs: 0.1, Limit: 20})

	if !threadDurationRowsContainPID(stats.TopRunning, 100) ||
		!threadDurationRowsContainPID(stats.RunnableTop, 100) ||
		!threadDurationRowsContainPID(stats.SleepTop, 100) {
		t.Fatalf("clean target scheduler value channels were erased: running=%+v runnable=%+v sleep=%+v", stats.TopRunning, stats.RunnableTop, stats.SleepTop)
	}
	for _, rows := range [][]ThreadDuration{stats.TopRunning, stats.RunnableTop, stats.SleepTop, stats.DStateTop, stats.IOWaitTop} {
		if threadDurationRowsContainPID(rows, 900) {
			t.Fatalf("conflicting PID leaked into scheduler value channels: %+v", rows)
		}
	}
	if !threadCPULoadRowsContainPID(stats.ThreadCPULoad, 100) {
		t.Fatalf("clean thread CPU-load input did not survive: %+v", stats.ThreadCPULoad)
	}
	for _, row := range stats.ThreadCPULoad {
		if row.Thread.PID == 900 {
			t.Fatalf("conflicting PID leaked into thread CPU-load: %+v", stats.ThreadCPULoad)
		}
	}
	if !stateChurnRowsContainPID(stats.StateChurn, 100) {
		t.Fatalf("clean churn row did not survive: %+v", stats.StateChurn)
	}
	for _, row := range stats.StateChurn {
		if row.Thread.PID == 900 {
			t.Fatalf("conflicting PID leaked into churn: %+v", stats.StateChurn)
		}
	}
	if len(stats.ProcessCPULoad) != 0 || stats.CPUOccupancy != nil || stats.ProcessDomainCensus != nil {
		t.Fatalf("process composites reopened before contributor-completeness proof: process=%+v occupancy=%+v census=%+v", stats.ProcessCPULoad, stats.CPUOccupancy, stats.ProcessDomainCensus)
	}
}

func threadDurationRowsContainPID(rows []ThreadDuration, pid int) bool {
	for _, row := range rows {
		if row.Thread.PID == pid {
			return true
		}
	}
	return false
}

func threadCPULoadRowsContainPID(rows []ThreadCPULoadSummary, pid int) bool {
	for _, row := range rows {
		if row.Thread.PID == pid {
			return true
		}
	}
	return false
}

func stateChurnRowsContainPID(rows []ThreadStateChurnSummary, pid int) bool {
	for _, row := range rows {
		if row.Thread.PID == pid {
			return true
		}
	}
	return false
}
