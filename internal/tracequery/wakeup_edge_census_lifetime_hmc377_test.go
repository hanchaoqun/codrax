package tracequery

import "testing"

// HMC377 D3-R3: the name-free census pair key keeps one pair per physical
// waker→wakee relation across a RENAME, but a REUSED TID (exit, then a new
// incarnation of the same number) is two threads and stays two pairs.
func TestHMC377CensusKeepsTwoLifetimesOfAReusedTIDApart(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "reuse_vs_rename.ftrace",
		" ui-100 (100) [000] .... 1.000000: sched_switch: prev_comm=ui prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=old next_pid=55 next_prio=120",
		" old-55 (55) [000] .... 1.010000: sched_wakeup: comm=ui pid=100 prio=120 target_cpu=001",
		" <idle>-0 (0) [001] .... 1.011000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=ui next_pid=100 next_prio=120",
		" ui-100 (100) [001] .... 1.012000: sched_switch: prev_comm=ui prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120",
		" old-55 (55) [000] .... 1.020000: sched_switch: prev_comm=old prev_pid=55 prev_prio=120 prev_state=X ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" creator-7 (7) [002] .... 1.030000: sched_wakeup_new: comm=creator pid=55 prio=120 target_cpu=000",
		" <idle>-0 (0) [000] .... 1.031000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=creator next_pid=55 next_prio=120",
		" fresh-55 (7) [000] .... 1.050000: sched_wakeup: comm=ui pid=100 prio=120 target_cpu=001",
		" <idle>-0 (0) [001] .... 1.051000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=ui next_pid=100 next_prio=120",
		" ui-100 (100) [001] .... 1.052000: sched_switch: prev_comm=ui prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120",
		" fresh-55 (7) [000] .... 1.055000: sched_switch: prev_comm=fresh prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=w-a next_pid=60 next_prio=120",
		" w-a-60 (100) [000] .... 1.060000: sched_wakeup: comm=ui pid=100 prio=120 target_cpu=001",
		" <idle>-0 (0) [001] .... 1.061000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=ui next_pid=100 next_prio=120",
		" ui-100 (100) [001] .... 1.062000: sched_switch: prev_comm=ui prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120",
		" w-b-60 (100) [000] .... 1.070000: sched_wakeup: comm=ui pid=100 prio=120 target_cpu=001",
		" <idle>-0 (0) [001] .... 1.071000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=ui next_pid=100 next_prio=120",
		" ui-100 (100) [001] .... 1.072000: sched_switch: prev_comm=ui prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120",
		" w-b-60 (100) [000] .... 1.080000: sched_switch: prev_comm=w-b prev_pid=60 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" <idle>-0 (0) [000] .... 1.100000: cpu_idle: state=0 cpu_id=0",
	)
	full, err := BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	for name, idx := range map[string]*Index{"full": full, "windowed": buildSchedulerCarryWindow(t, path, 1.0, 1.1)} {
		res := Run(idx, Query{View: "wakeup_chain", PID: 100, TimeStart: 1.0, TimeEnd: 1.1, TimeStartSet: true, TimeEndSet: true})
		if res.WakeupChain == nil {
			t.Fatalf("%s: no chain", name)
		}
		pairs55, count60 := 0, 0
		for _, p := range res.WakeupChain.WakeupEdgeCensus {
			if p.Wakee.PID != 100 {
				continue
			}
			switch p.Waker.PID {
			case 55:
				pairs55++
				if p.Count != 1 {
					t.Fatalf("%s: each lifetime of TID 55 woke ui once: %+v", name, p)
				}
			case 60:
				count60 += p.Count
			}
		}
		if pairs55 != 2 {
			t.Fatalf("%s: a reused TID is two threads — want two census pairs for TID 55, got %d: %+v", name, pairs55, res.WakeupChain.WakeupEdgeCensus)
		}
		if count60 != 2 {
			t.Fatalf("%s: the renamed TID 60 is one thread with one pair of count 2: %+v", name, res.WakeupChain.WakeupEdgeCensus)
		}
	}
}
