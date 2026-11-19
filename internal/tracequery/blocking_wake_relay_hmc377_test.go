package tracequery

import (
	"strings"
	"testing"
)

// HMC377 D2 — a blocking row says what was proven. These synthetic captures
// need no external file. The shared wait is the batch-376 NetEase shape: one
// collector runs, the main thread and two workers park in the same recorded
// wait, the collector wakes the main thread and each woken waiter wakes the
// next one.
func hmc377RelayCapture(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

const hmc377RelayWait = "GC: Wait For Completion Alloc"

func hmc377SharedWaitCapture(workerWaitName string) string {
	return hmc377RelayCapture(
		"            main-100   (  100) [001] ....   1.000000: tracing_mark_write: B|100|"+hmc377RelayWait,
		"            main-100   (  100) [001] ....   1.000010: sched_switch: prev_comm=main prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120",
		"              w1-101   (  100) [002] ....   1.000100: tracing_mark_write: B|100|"+workerWaitName,
		"              w1-101   (  100) [002] ....   1.000110: sched_switch: prev_comm=w1 prev_pid=101 prev_prio=120 prev_state=S ==> next_comm=swapper/2 next_pid=0 next_prio=120",
		"              w2-102   (  100) [003] ....   1.000200: tracing_mark_write: B|100|"+workerWaitName,
		"              w2-102   (  100) [003] ....   1.000210: sched_switch: prev_comm=w2 prev_pid=102 prev_prio=120 prev_state=S ==> next_comm=swapper/3 next_pid=0 next_prio=120",
		"       collector-110   (  100) [004] ....   1.000300: sched_switch: prev_comm=swapper/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=collector next_pid=110 next_prio=120",
		"       collector-110   (  100) [004] ....   1.100000: sched_wakeup: comm=main pid=100 prio=120 target_cpu=001",
		"            main-100   (  100) [001] ....   1.100010: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=main next_pid=100 next_prio=120",
		"            main-100   (  100) [001] ....   1.100020: tracing_mark_write: E|100",
		"            main-100   (  100) [001] ....   1.100030: sched_wakeup: comm=w1 pid=101 prio=120 target_cpu=002",
		"              w1-101   (  100) [002] ....   1.100040: sched_switch: prev_comm=swapper/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=w1 next_pid=101 next_prio=120",
		"              w1-101   (  100) [002] ....   1.100050: tracing_mark_write: E|100",
		"              w1-101   (  100) [002] ....   1.100060: sched_wakeup: comm=w2 pid=102 prio=120 target_cpu=003",
		"              w2-102   (  100) [003] ....   1.100070: sched_switch: prev_comm=swapper/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=w2 next_pid=102 next_prio=120",
		"              w2-102   (  100) [003] ....   1.100080: tracing_mark_write: E|100",
		"       collector-110   (  100) [004] ....   1.150000: sched_switch: prev_comm=collector prev_pid=110 prev_prio=120 prev_state=S ==> next_comm=swapper/4 next_pid=0 next_prio=120",
	)
}

func hmc377BlockingRows(t *testing.T, content string) map[int]CriticalBlockingCandidate {
	t.Helper()
	idx := buildTraceIndex(t, "hmc377_relay.systrace", content)
	res := BuildCriticalBlockingCalls(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.2})
	rows := map[int]CriticalBlockingCandidate{}
	for _, item := range res.Items {
		if item.Type == "blocking_span" {
			rows[item.Thread.PID] = item
		}
	}
	return rows
}

// Red before the batch: the fellow waiters' rows named the previous waiter as
// their counterpart with no typed distinction from a worker.
func TestHMC377FellowWaiterWakeIsAPassedOnRelay(t *testing.T) {
	rows := hmc377BlockingRows(t, hmc377SharedWaitCapture(hmc377RelayWait))
	main, w1, w2 := rows[100], rows[101], rows[102]
	if main.Peer.PID != 110 || w1.Peer.PID != 100 || w2.Peer.PID != 101 {
		t.Fatalf("wakeup-edge counterparts drifted: main->%d w1->%d w2->%d", main.Peer.PID, w1.Peer.PID, w2.Peer.PID)
	}
	// The collector ran the awaited work: the main thread's waker is no relay.
	if main.WakeRelay {
		t.Fatalf("the collector that ended the main thread's wait was marked a relay: %+v", main)
	}
	for _, row := range []CriticalBlockingCandidate{w1, w2} {
		if !row.WakeRelay || row.WakeRelayUpstream.PID != 110 {
			t.Fatalf("%s-%d: waker %d only passed the wake on; want relay with upstream collector-110, got relay=%v upstream=%+v",
				row.Thread.Comm, row.Thread.PID, row.Peer.PID, row.WakeRelay, row.WakeRelayUpstream)
		}
		if !strings.Contains(row.Summary, "is not the party this wait depended on") {
			t.Fatalf("the model-facing summary does not say the waker only passed the wake on: %q", row.Summary)
		}
		// Wording input only: the value, lane and counterpart stay untouched.
		if row.BlockingKind != "" || row.PeerSource != CounterpartSourceWakeupEdge || row.BlockingValueBasis != BlockingValueBasisWaitSegments {
			t.Fatalf("relay verdict moved a typed lane: %+v", row)
		}
	}
}

// A waker parked in a DIFFERENT recorded wait is not proven to be a fellow
// waiter of the same work: the neutral reading stays.
func TestHMC377DifferentRecordedWaitIsNoRelay(t *testing.T) {
	rows := hmc377BlockingRows(t, hmc377SharedWaitCapture("futex wait for work item"))
	if rows[101].WakeRelay {
		t.Fatalf("a waker in a different recorded wait was marked a relay: %+v", rows[101])
	}
	// w2's waker (w1) does record the same wait as w2, and w1's own wake came
	// first: that pair is a relay whose chain ends at w1's own waker.
	if !rows[102].WakeRelay || rows[102].WakeRelayUpstream.PID != 100 {
		t.Fatalf("same-wait pair lost its relay verdict: %+v", rows[102])
	}
}

// A waker whose own wait ended only AFTER it woke the waiter did not pass a
// wake on: no relay.
func TestHMC377WakerStillWaitingIsNoRelay(t *testing.T) {
	items := []CriticalBlockingCandidate{
		{Type: "blocking_span", Thread: ThreadRef{PID: 2}, Peer: ThreadRef{PID: 1}, PeerSource: CounterpartSourceWakeupEdge,
			WaitObject: hmc377RelayWait, StartTs: 1.0, EndTs: 1.2, peerWakeTs: 1.10},
		{Type: "blocking_span", Thread: ThreadRef{PID: 1}, Peer: ThreadRef{PID: 9}, PeerSource: CounterpartSourceWakeupEdge,
			WaitObject: hmc377RelayWait, StartTs: 1.0, EndTs: 1.2, peerWakeTs: 1.15},
	}
	stampBlockingWakeRelays(nil, items)
	if items[0].WakeRelay {
		t.Fatalf("waker woken after it woke the waiter was marked a relay: %+v", items[0])
	}
}

// Real lock evidence is never re-read as a relay: a typed monitor/lock row
// whose holder was itself parked keeps its holder reading (a blocked holder
// is still the holder — the Douyin reboot capture's nested monitor shape).
func TestHMC377TypedLockRowIsNeverARelay(t *testing.T) {
	items := []CriticalBlockingCandidate{
		{Type: "blocking_span", BlockingKind: "monitor_contention", Thread: ThreadRef{PID: 2}, Peer: ThreadRef{PID: 1},
			HolderSource: CounterpartSourceWakeupEdge, WaitObject: "", StartTs: 1.0, EndTs: 1.2, peerWakeTs: 1.10},
		{Type: "blocking_span", BlockingKind: "monitor_contention", Thread: ThreadRef{PID: 1}, Peer: ThreadRef{PID: 9},
			HolderSource: CounterpartSourceWakeupEdge, StartTs: 1.0, EndTs: 1.2, peerWakeTs: 1.05},
	}
	stampBlockingWakeRelays(nil, items)
	for _, item := range items {
		if item.WakeRelay {
			t.Fatalf("typed lock row re-read as a relay: %+v", item)
		}
	}
}

// The registry's lock_contention lane / lock_priority direction for the
// blocking_span token is honest only because no kind-less blocking row ever
// becomes a rank row: the rank lane admits typed contention payloads only.
func TestHMC377RankBoardNeverMintsAKindlessBlockingRow(t *testing.T) {
	idx := buildTraceIndex(t, "hmc377_relay.systrace", hmc377SharedWaitCapture(hmc377RelayWait))
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.2})
	for _, item := range rank.Items {
		if item.Type == "blocking_span" && strings.TrimSpace(item.BlockingKind) == "" {
			t.Fatalf("a recorded wait without a lock payload became a rank row on the lock lane: %+v", item)
		}
	}
	spec, ok := CausalTokenSpecFor("blocking_span")
	if !ok || spec.Lane != CausalLaneLockContention || CausalTokenFixDirectionFor("blocking_span") != CausalFixDirectionLockPriority {
		t.Fatalf("blocking_span registry row drifted: %+v", spec)
	}
}

// HMC377 D2-R4 — the relay predicate needs the waker to have LEFT the shared
// recorded wait and to have passed the wake on without sleeping again. Two
// real false relays (unseen Douyin captures, "waitForever" on two different
// fences) broke each half: the waker was still inside its own span when it
// woke the waiter, or its matched span had ended long before and it slept and
// was woken by other work before the wake it passed on.
func TestHMC377RelayNeedsTheWakerReleasedAndNotSleepingAgain(t *testing.T) {
	head := []string{
		"            main-100   (  100) [001] ....   1.000000: tracing_mark_write: B|100|" + hmc377RelayWait,
		"            main-100   (  100) [001] ....   1.000010: sched_switch: prev_comm=main prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120",
		"              w1-101   (  100) [002] ....   1.000100: tracing_mark_write: B|100|" + hmc377RelayWait,
		"              w1-101   (  100) [002] ....   1.000110: sched_switch: prev_comm=w1 prev_pid=101 prev_prio=120 prev_state=S ==> next_comm=swapper/2 next_pid=0 next_prio=120",
		"       collector-110   (  100) [004] ....   1.000300: sched_switch: prev_comm=swapper/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=collector next_pid=110 next_prio=120",
		"       collector-110   (  100) [004] ....   1.050000: sched_wakeup: comm=w1 pid=101 prio=120 target_cpu=002",
		"              w1-101   (  100) [002] ....   1.050010: sched_switch: prev_comm=swapper/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=w1 next_pid=101 next_prio=120",
	}
	tail := []string{
		"            main-100   (  100) [001] ....   1.070010: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=main next_pid=100 next_prio=120",
		"            main-100   (  100) [001] ....   1.070020: tracing_mark_write: E|100",
		"       collector-110   (  100) [004] ....   1.150000: sched_switch: prev_comm=collector prev_pid=110 prev_prio=120 prev_state=S ==> next_comm=swapper/4 next_pid=0 next_prio=120",
	}
	// Still inside its own span: w1 wakes main at 1.070000 and only ends its
	// own wait at 1.080000.
	stillInside := append(append(append([]string(nil), head...),
		"              w1-101   (  100) [002] ....   1.070000: sched_wakeup: comm=main pid=100 prio=120 target_cpu=001",
		"              w1-101   (  100) [002] ....   1.080000: tracing_mark_write: E|100",
	), tail...)
	if rows := hmc377BlockingRows(t, hmc377RelayCapture(stillInside...)); rows[100].Peer.PID != 101 || rows[100].WakeRelay {
		t.Fatalf("a waker still inside its own recorded wait passed nothing on: %+v", rows[100])
	}
	// Released, then slept again and was woken by other work before waking
	// main: the wake it passed on is not the shared wait's.
	sleptAgain := append(append(append([]string(nil), head...),
		"              w1-101   (  100) [002] ....   1.050020: tracing_mark_write: E|100",
		"              w1-101   (  100) [002] ....   1.050030: sched_switch: prev_comm=w1 prev_pid=101 prev_prio=120 prev_state=S ==> next_comm=swapper/2 next_pid=0 next_prio=120",
		"           other-120   (  120) [005] ....   1.065000: sched_wakeup: comm=w1 pid=101 prio=120 target_cpu=002",
		"              w1-101   (  100) [002] ....   1.065010: sched_switch: prev_comm=swapper/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=w1 next_pid=101 next_prio=120",
		"              w1-101   (  100) [002] ....   1.070000: sched_wakeup: comm=main pid=100 prio=120 target_cpu=001",
	), tail...)
	if rows := hmc377BlockingRows(t, hmc377RelayCapture(sleptAgain...)); rows[100].Peer.PID != 101 || rows[100].WakeRelay {
		t.Fatalf("a waker that slept again before passing the wake on is not a relay: %+v", rows[100])
	}
	// Control: released and passing the wake straight on is a relay.
	relay := append(append(append([]string(nil), head...),
		"              w1-101   (  100) [002] ....   1.050020: tracing_mark_write: E|100",
		"              w1-101   (  100) [002] ....   1.070000: sched_wakeup: comm=main pid=100 prio=120 target_cpu=001",
	), tail...)
	if rows := hmc377BlockingRows(t, hmc377RelayCapture(relay...)); !rows[100].WakeRelay || rows[100].WakeRelayUpstream.PID != 110 {
		t.Fatalf("a released waker passing the wake straight on is a relay: %+v", rows[100])
	}
}

// HMC377 fix round 2, item F7: the "slept again in between" check reads a
// per-thread index of the wakeup rows built once per relay pass instead of
// scanning every event for each candidate pair. The verdict must be exactly
// the row scan's: wakeup and waking rows naming the thread by id, strictly
// inside (after, before), never a sched_wakeup_new incarnation row, rows of
// other threads ignored, unsorted input handled, and no trace index proves
// nothing (fail closed).
func TestHMC377WakeRelayWakeupIndexMatchesTheRowScan(t *testing.T) {
	scan := func(idx *Index, thread ThreadRef, after, before float64) bool {
		if idx == nil {
			return true
		}
		for i := range idx.Events {
			ev := &idx.Events[i]
			if ev.Type != EventSchedWakeup && ev.Type != EventSchedWaking {
				continue
			}
			if ev.Ts <= after || ev.Ts >= before || schedWakeupStartsNewIncarnation(*ev) {
				continue
			}
			if threadMatches(thread, ev.WakeePID, ev.WakeeComm) {
				return true
			}
		}
		return false
	}
	idx := &Index{Events: []Event{
		{Type: EventSchedWakeup, Name: "sched_wakeup", Ts: 1.300, WakeePID: 7},
		{Type: EventSchedWaking, Name: "sched_waking", Ts: 1.100, WakeePID: 7},
		{Type: EventSchedWakeup, Name: "sched_wakeup_new", Ts: 1.500, WakeePID: 7},
		{Type: EventSchedWakeup, Name: "sched_wakeup", Ts: 1.200, WakeePID: 8},
		{Type: EventSchedSwitch, Ts: 1.250, PrevPID: 7, NextPID: 8},
		{Type: EventSchedWakeup, Name: "sched_wakeup", Ts: 1.700, WakeePID: 7},
	}}
	items := []CriticalBlockingCandidate{
		{Type: "blocking_span", PeerSource: CounterpartSourceWakeupEdge, Peer: ThreadRef{PID: 7, Comm: "w"}, peerWakeTs: 1, WaitObject: hmc377RelayWait},
		{Type: "blocking_span", PeerSource: CounterpartSourceWakeupEdge, Peer: ThreadRef{PID: 8, Comm: "v"}, peerWakeTs: 1, WaitObject: hmc377RelayWait},
	}
	wakes := newWakeeWakeupIndex(idx, items)
	bounds := []float64{1.000, 1.100, 1.150, 1.200, 1.250, 1.300, 1.400, 1.500, 1.600, 1.700, 1.800}
	trues := 0
	for _, thread := range []ThreadRef{{PID: 7, Comm: "w"}, {PID: 8, Comm: "v"}} {
		for _, after := range bounds {
			for _, before := range bounds {
				want := scan(idx, thread, after, before)
				if got := wakes.wokenBetween(thread, after, before); got != want {
					t.Fatalf("thread %d (%.3f, %.3f): index says %v, the row scan says %v", thread.PID, after, before, got, want)
				}
				if want {
					trues++
				}
			}
		}
	}
	if trues == 0 {
		t.Fatal("the grid must exercise woken verdicts")
	}
	if !(*wakeeWakeupIndex)(nil).wokenBetween(ThreadRef{PID: 7}, 1.0, 2.0) || newWakeeWakeupIndex(nil, items) != nil {
		t.Fatal("no trace index must prove nothing (fail closed)")
	}
}
