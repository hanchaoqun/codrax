package tracequery

import (
	"context"
	"testing"
)

const sameTIDRenameTrace = `
          idle-0 (    0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=alpha next_pid=42 next_prio=20
         alpha-42 (   42) [000] .... 1.002000: sched_switch: prev_comm=alpha prev_pid=42 prev_prio=20 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
        helper-7 (    7) [001] .... 1.004000: sched_wakeup: comm=beta pid=42 prio=20 target_cpu=000
          idle-0 (    0) [000] .... 1.006000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=beta next_pid=42 next_prio=20
          beta-42 (   42) [000] .... 1.008000: sched_switch: prev_comm=beta prev_pid=42 prev_prio=20 prev_state=D ==> next_comm=idle next_pid=0 next_prio=120
        helper-7 (    7) [001] .... 1.010000: sched_wakeup: comm=gamma pid=42 prio=20 target_cpu=000
          idle-0 (    0) [000] .... 1.012000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=gamma next_pid=42 next_prio=20
         gamma-42 (   42) [000] .... 1.014000: sched_switch: prev_comm=gamma prev_pid=42 prev_prio=20 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`

func TestSameTIDRenameDoesNotSplitIndexedHardAccounts(t *testing.T) {
	idx := buildTraceIndex(t, "same_tid_rename.systrace", sameTIDRenameTrace)
	q := Query{TimeStart: 1.0, TimeEnd: 1.016, MinDurationMs: 0.1, Limit: 20}
	stats := ComputeWindowStats(idx, q)

	assertSingleTIDDuration(t, "top running", stats.TopRunning, 42)
	assertSingleTIDDuration(t, "runnable", stats.RunnableTop, 42)
	assertSingleTIDDuration(t, "sleep", stats.SleepTop, 42)
	if stats.CPUOccupancy == nil {
		t.Fatal("expected CPU occupancy")
	}
	occupiers := 0
	for _, item := range stats.CPUOccupancy.TopThreads {
		if item.Thread.PID == 42 {
			occupiers++
		}
	}
	if occupiers != 1 {
		t.Fatalf("same numeric TID must occupy one CPU-account seat across comm rename, got %d: %+v", occupiers, stats.CPUOccupancy.TopThreads)
	}
	churn := 0
	for _, item := range stats.StateChurn {
		if item.Thread.PID == 42 {
			churn++
		}
	}
	if churn != 1 {
		t.Fatalf("same numeric TID must occupy one churn seat across comm rename, got %d: %+v", churn, stats.StateChurn)
	}
}

func TestSameTIDRenameDoesNotSplitStreamingHardAccounts(t *testing.T) {
	idx := buildTraceIndex(t, "same_tid_rename_stream.systrace", sameTIDRenameTrace)
	res, err := StreamStateCluster(context.Background(), idx.Path, Query{TimeStart: 1.0, TimeEnd: 1.016, MinDurationMs: 0.1, Limit: 20}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.WindowStats == nil {
		t.Fatal("expected streaming window stats")
	}
	assertSingleTIDDuration(t, "stream top running", res.WindowStats.TopRunning, 42)
	assertSingleTIDDuration(t, "stream runnable", res.WindowStats.RunnableTop, 42)
	assertSingleTIDDuration(t, "stream sleep", res.WindowStats.SleepTop, 42)
	churn := 0
	for _, item := range res.WindowStats.StateChurn {
		if item.Thread.PID == 42 {
			churn++
		}
	}
	if churn != 1 {
		t.Fatalf("streaming same numeric TID must occupy one churn seat, got %d: %+v", churn, res.WindowStats.StateChurn)
	}
}

func assertSingleTIDDuration(t *testing.T, label string, items []ThreadDuration, pid int) {
	t.Helper()
	count := 0
	for _, item := range items {
		if item.Thread.PID == pid {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("%s must have one seat for tid=%d across comm rename, got %d: %+v", label, pid, count, items)
	}
}
