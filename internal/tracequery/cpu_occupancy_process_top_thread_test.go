package tracequery

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const b1623MigratingThreadTrace = `
          <idle>-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old_name next_pid=101 next_prio=20
          <idle>-0 (-----) [002] .... 1.000000: sched_switch: prev_comm=swapper/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=sibling next_pid=102 next_prio=20
          <idle>-0 (-----) [003] .... 1.000000: sched_switch: prev_comm=swapper/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=new_name next_pid=201 next_prio=20
     old_name-101 (  100) [000] .... 1.006000: sched_switch: prev_comm=old_name prev_pid=101 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
          <idle>-0 (-----) [001] .... 1.007000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=new_name next_pid=101 next_prio=20
      sibling-102 (  100) [002] .... 1.010000: sched_switch: prev_comm=sibling prev_pid=102 prev_prio=20 prev_state=S ==> next_comm=swapper/2 next_pid=0 next_prio=120
     new_name-201 (  200) [003] .... 1.010000: sched_switch: prev_comm=new_name prev_pid=201 prev_prio=20 prev_state=S ==> next_comm=swapper/3 next_pid=0 next_prio=120
     new_name-101 (  100) [001] .... 1.013000: sched_switch: prev_comm=new_name prev_pid=101 prev_prio=20 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
`

func b1623Process(t *testing.T, occ *CPUOccupancyStats, pid int) ProcessCPULoadSummary {
	t.Helper()
	if occ == nil {
		t.Fatal("CPU occupancy missing")
	}
	for _, row := range occ.TopProcesses {
		if row.Process.PID == pid {
			return row
		}
	}
	t.Fatalf("process %d missing from %+v", pid, occ.TopProcesses)
	return ProcessCPULoadSummary{}
}

func TestB1623ProcessLeaderUsesUntruncatedThreadPopulation(t *testing.T) {
	running := map[string]ThreadDuration{}
	catalog := map[int]ThreadRef{}
	add := func(pid, tgid, cpu int, ms float64) {
		thread := ThreadRef{PID: pid, Comm: fmt.Sprintf("task%d", pid)}
		catalog[pid] = ThreadRef{PID: pid, TGID: tgid, Comm: thread.Comm}
		running[threadCPUKey(thread, cpu)] = ThreadDuration{Thread: thread, CPU: cpu, DurationMs: ms, LineStart: 1, LineEnd: 2}
	}
	for pid := 301; pid <= 308; pid++ {
		add(pid, 300, pid, 20)
	}
	add(101, 100, 0, 6)
	add(101, 100, 1, 6)
	add(102, 100, 2, 10)
	occ := computeCPUOccupancyStats(Query{}, 0, running, nil, nil, catalog, nil, 8, false)
	if len(occ.TopThreads) != 8 || len(occ.TopProcesses) != 2 {
		t.Fatalf("existing independent display budgets changed: %+v", occ)
	}
	for _, row := range occ.TopThreads {
		if row.Thread.PID < 300 {
			t.Fatalf("test premise: process100 members should all be below global Top8: %+v", occ.TopThreads)
		}
	}
	proc := b1623Process(t, occ, 100)
	approxEq(t, "untruncated leader", proc.TopThreadMs, 12)
	approxEq(t, "untruncated process total", proc.RunningMs, 22)
	if proc.TopThread.PID != 101 || occ.WindowMs != 0 {
		t.Fatalf("leader or unbounded-window semantics drifted: %+v", proc)
	}
}

func TestB1623ProcessLeaderTieIsStableWithoutMergingSameNames(t *testing.T) {
	rows := []ThreadDuration{
		{Thread: ThreadRef{PID: 102, Comm: "same"}, CPU: 2, DurationMs: 12, LineStart: 10, LineEnd: 20},
		{Thread: ThreadRef{PID: 101, Comm: "same"}, CPU: 0, DurationMs: 6, LineStart: 10, LineEnd: 11},
		{Thread: ThreadRef{PID: 101, Comm: "same"}, CPU: 1, DurationMs: 6, LineStart: 12, LineEnd: 13},
		{Thread: ThreadRef{PID: 201, Comm: "same"}, CPU: 3, DurationMs: 30, LineStart: 10, LineEnd: 20},
	}
	catalog := map[int]ThreadRef{101: {PID: 101, TGID: 100}, 102: {PID: 102, TGID: 100}, 201: {PID: 201, TGID: 200}}
	var first ProcessCPULoadSummary
	for iteration := 0; iteration < 40; iteration++ {
		running := map[string]ThreadDuration{}
		for i := range rows {
			row := rows[(i+iteration)%len(rows)]
			running[threadCPUKey(row.Thread, row.CPU)] = row
		}
		before := make(map[string]ThreadDuration, len(running))
		for key, row := range running {
			before[key] = row
		}
		occ := computeCPUOccupancyStats(Query{}, 0, running, nil, nil, catalog, nil, 8, false)
		if !reflect.DeepEqual(running, before) {
			t.Fatal("leader selection mutated the original CPU buckets")
		}
		proc := b1623Process(t, occ, 100)
		approxEq(t, "same name distinct TID stays two threads", proc.RunningMs, 24)
		approxEq(t, "tie total is whole thread", proc.TopThreadMs, 12)
		if proc.ThreadCount != 2 || proc.TopThread.PID != 101 {
			t.Fatalf("equal totals/line ties must deterministically choose the lower TID, not CPU/map order: %+v", proc)
		}
		if iteration == 0 {
			first = proc
		} else if !reflect.DeepEqual(first, proc) {
			t.Fatalf("input order changed leader metadata: first=%+v current=%+v", first, proc)
		}
		approxEq(t, "other process remains independent", b1623Process(t, occ, 200).TopThreadMs, 30)
	}
}

func TestB1623UnknownIdentityAndCPUDoNotGainAssumedZeroOrEquality(t *testing.T) {
	t.Run("missing_tid_is_not_a_cross_cpu_identity", func(t *testing.T) {
		running := map[string]ThreadDuration{
			"observed0": {Thread: ThreadRef{PID: -1, Comm: "same"}, CPU: 0, DurationMs: 6},
			"observed1": {Thread: ThreadRef{PID: -1, Comm: "same"}, CPU: 1, DurationMs: 7},
		}
		occ := computeCPUOccupancyStats(Query{}, 0, running, nil, nil, nil, nil, 8, false)
		if len(occ.TopProcesses) != 1 {
			t.Fatalf("legacy observed process row lost: %+v", occ)
		}
		proc := occ.TopProcesses[0]
		// No new identity is minted from a shared comm: retain the previous
		// local bucket rather than making it an allegedly known 13ms thread.
		approxEq(t, "unknown TID retains local observation", proc.TopThreadMs, 7)
		if proc.TopThread.PID != -1 || proc.ThreadCount != 0 {
			t.Fatalf("unknown TID became a counted known thread: %+v", proc)
		}
	})
	t.Run("unknown_cpu_does_not_erase_known_thread_time_or_become_cpu0", func(t *testing.T) {
		thread := ThreadRef{PID: 101, Comm: "same"}
		running := map[string]ThreadDuration{
			"observed3": {Thread: thread, CPU: 3, DurationMs: 6},
			"unlocated": {Thread: thread, CPU: -1, DurationMs: 7},
		}
		occ := computeCPUOccupancyStats(Query{}, 0, running, nil, nil, nil, nil, 8, false)
		proc := b1623Process(t, occ, 101)
		approxEq(t, "known TID's observed time remains nonzero", proc.TopThreadMs, 13)
		if !reflect.DeepEqual(proc.CPUs, []int{3}) {
			t.Fatalf("unknown CPU was assigned to a real CPU: %+v", proc)
		}
	})
	t.Run("absent_measurements_do_not_publish_a_zero_leader", func(t *testing.T) {
		idx := buildTraceIndex(t, "wakeup_only.systrace", "waker-10 (10) [001] .... 1.005000: sched_wakeup: comm=target pid=101 prio=20 target_cpu=003\n")
		res := Run(idx, Query{View: "window_stats", PID: 101, TimeStart: 1, TimeEnd: 1.01})
		if res.WindowStats == nil || res.WindowStats.CPUOccupancy != nil {
			t.Fatalf("no running measurement cannot mean measured zero CPU/process occupancy: %+v", res.WindowStats)
		}
	})
}

func TestB1623ActualQueryKeepsIncarnationAndProcessIdentityGuards(t *testing.T) {
	idx := buildTraceIndex(t, "reused_tid.systrace", `
       old-42 (100) [000] .... 0.995000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=42 next_prio=20
       old-42 (100) [000] .... 1.005000: sched_switch: prev_comm=old prev_pid=42 prev_prio=20 prev_state=X ==> next_comm=swapper/0 next_pid=0 next_prio=120
     creator-7 (7) [001] .... 1.007000: sched_wakeup_new: comm=new pid=42 prio=20 target_cpu=001
       new-42 (200) [001] .... 1.010000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=new next_pid=42 next_prio=20
       new-42 (200) [001] .... 1.018000: sched_switch: prev_comm=new prev_pid=42 prev_prio=20 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
`)
	res := Run(idx, Query{View: "window_stats", PID: 42, TimeStart: 1, TimeEnd: 1.02})
	if res.WindowStats == nil || res.WindowStats.CPUOccupancy != nil || len(res.WindowStats.ProcessCPULoad) != 0 {
		t.Fatalf("numeric TID reuse must not join generations/processes: %+v", res.WindowStats)
	}
	if !containsSubstring(res.WindowStats.Caveats, "thread_incarnation_conflict") {
		t.Fatalf("test premise: exact lifecycle conflict must be identified: %v", res.WindowStats.Caveats)
	}
	res = Run(idx, Query{View: "window_stats", PID: 42, TimeStart: 1.007, TimeEnd: 1.02})
	if res.WindowStats == nil {
		t.Fatal("new incarnation-only window missing")
	}
	proc := b1623Process(t, res.WindowStats.CPUOccupancy, 200)
	approxEq(t, "only new incarnation's running", proc.TopThreadMs, 8)
	if proc.TopThread.PID != 42 || proc.TopThread.Comm != "new" || len(res.WindowStats.CPUOccupancy.TopProcesses) != 1 {
		t.Fatalf("prior process/name borrowed into new generation: %+v", res.WindowStats.CPUOccupancy)
	}
}

func TestB1623ActualQueryChoosesWholeThreadNotLargestCPUBucket(t *testing.T) {
	idx := buildTraceIndex(t, "migrating.systrace", b1623MigratingThreadTrace)
	res := Run(idx, Query{View: "window_stats", PID: 101, TimeStart: 1, TimeEnd: 1.014})
	if res.WindowStats == nil {
		t.Fatalf("window entry did not publish stats: %+v", res)
	}
	occ := res.WindowStats.CPUOccupancy
	proc := b1623Process(t, occ, 100)
	approxEq(t, "process total remains sum of thread CPU time", proc.RunningMs, 22)
	if proc.ThreadCount != 2 {
		t.Fatalf("process/thread identity confused: %+v", proc)
	}
	if proc.TopThread.PID != 101 || proc.TopThread.Comm != "new_name" {
		t.Errorf("6+6 ms migrating thread must beat single-CPU 10 ms sibling and keep latest same-TID name: %+v", proc)
	}
	approxEq(t, "process top thread is cross-CPU total", proc.TopThreadMs, 12)
	other := b1623Process(t, occ, 200)
	approxEq(t, "same-name other process not merged", other.RunningMs, 10)
	if other.TopThread.PID != 201 || other.ThreadCount != 1 {
		t.Fatalf("same-name distinct TID confused: %+v", other)
	}
	seen := 0
	for _, cpu := range occ.PerCPUTop {
		for _, row := range cpu.Top {
			if row.Thread.PID == 101 {
				seen++
				approxEq(t, "per-CPU bucket remains local", row.DurationMs, 6)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("expected two unchanged CPU buckets, got %d: %+v", seen, occ.PerCPUTop)
	}
}

func TestB1623RealD4QueryProcessLeaderMatchesTargetRunning(t *testing.T) {
	idx, err := BuildIndex(context.Background(), evalcaseTiebaFixture)
	if err != nil {
		t.Fatal(err)
	}
	res := Run(idx, Query{View: "window_stats", PID: 59566, TimeStart: 34579.472865, TimeEnd: 34579.587805})
	if res.WindowStats == nil || res.TargetWindowStates == nil {
		t.Fatal("real window must publish both occupancy and target states")
	}
	proc := b1623Process(t, res.WindowStats.CPUOccupancy, 59566)
	approxEq(t, "real process CPU time", proc.RunningMs, 108.356)
	approxEq(t, "real target Running", res.TargetWindowStates.RunningMs, 26.946)
	approxEq(t, "real process top thread", proc.TopThreadMs, 26.946)
	var rosterTotal, largestBucket float64
	for _, row := range res.TargetWindowStates.RunningByCPU {
		rosterTotal += row.RunningMs
		if row.RunningMs > largestBucket {
			largestBucket = row.RunningMs
		}
	}
	approxEq(t, "same target CPU roster remains conserved", rosterTotal, 26.946)
	approxEq(t, "original largest CPU bucket remains a local value", largestBucket, 11.487)
	if proc.TopThread.PID != 59566 || !strings.Contains(proc.Summary, "26.946ms") {
		t.Fatalf("process summary lost exact leader identity or sum: %+v", proc)
	}
}
