package tracequery

import "testing"

// HMC377 D3-R9: the kernel's "<...>" placeholder (comm not in the saved
// cmdline cache) is display metadata, never a second name — a thread whose
// only other "name" is the placeholder is not a drifted identity (Honeybird
// wakeup_chain face: OS_IPC_0_813 printed as "<...>-813" in a row header).
func TestHMC377PlaceholderCommIsNotAThreadDrift(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "placeholder_drift.ftrace",
		" <...>-813 (810) [000] .... 1.000000: sched_switch: prev_comm=OS_IPC_0_813 prev_pid=813 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" <idle>-0 (0) [000] .... 1.010000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=OS_IPC_0_813 next_pid=813 next_prio=120",
		" OS_IPC_0_813-813 (810) [000] .... 1.020000: sched_switch: prev_comm=OS_IPC_0_813 prev_pid=813 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" renamer-900 (900) [001] .... 1.030000: sched_switch: prev_comm=renamer prev_pid=900 prev_prio=120 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120",
		" <idle>-0 (0) [001] .... 1.040000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=900 next_prio=120",
		" worker-900 (901) [001] .... 1.050000: sched_switch: prev_comm=worker prev_pid=900 prev_prio=120 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120",
	)
	idx, err := BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	drifted := map[int]bool{}
	for _, d := range detectThreadDrifts(idx, Query{}, 8) {
		drifted[d.PID] = true
	}
	if drifted[813] {
		t.Fatal("the placeholder comm made OS_IPC_0_813 look like a renamed thread")
	}
	if !drifted[900] {
		t.Fatal("control: an unreconciled TID (two process numbers) stays a drift")
	}
}
