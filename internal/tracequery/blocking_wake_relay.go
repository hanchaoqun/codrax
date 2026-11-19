package tracequery

import (
	"fmt"
	"sort"
	"strings"
)

// HMC377 D2 — a blocking row says what was proven.
//
// A payload-less blocking_span row (a recorded wait span with no lock or
// monitor payload) names as its counterpart the thread whose wakeup ended the
// wait (resolveCounterpartViaWakeupEdge). That thread is not necessarily the
// party the wait depended on. When several threads park in the same recorded
// wait (the NetEase cold-start witness: the main thread and four worker threads
// all inside "GC: Wait For Completion Alloc" while HeapTaskDaemon runs the
// collection), the collector wakes one waiter and each woken waiter wakes the
// next one: the wake is passed along. The thread that woke a waiter after its
// own wait in the same recorded wait had ended is a relay, never the blocker.
//
// stampBlockingWakeRelays marks those rows with typed comparisons only:
//
//   - both rows are payload-less blocking_span rows whose counterpart came
//     from a wakeup edge (typed peer source, the edge timestamp carried by the
//     resolver);
//   - the waker's own row records the SAME wait (verbatim span-name equality —
//     never a keyword test of the name);
//   - the waker's own wait began before it woke this waiter, and its own wake
//     edge lies inside this waiter's wait and no later than the wake it passed
//     on;
//   - HMC377 D2-R4: the waker's own recorded wait had ENDED when it woke this
//     waiter (a waker still inside its own span is not released from it), and
//     no wakeup named the waker between its own release and the wake it
//     passed on (it never slept again in between — a later sleep means the
//     wake it passed on came from other work, as the two real waitForever
//     fence pairs showed). The second check reads the same wakeup rows the
//     counterpart resolver reads (wakeup / waking rows naming the waker,
//     never a sched_wakeup_new incarnation row), indexed once per call by
//     the woken thread id; with no index the pair is not proven.
//
// The upstream party is the waker of the first non-relay wait along the relay
// chain. The verdict is wording input only: values, chain relevance, rank and
// every gate are untouched; rows without the proof keep the neutral
// "the thread that woke the waiter" reading.
func stampBlockingWakeRelays(idx *Index, items []CriticalBlockingCandidate) {
	byThread := map[int][]int{}
	for i := range items {
		if blockingRowWokenByEdge(items[i]) && items[i].Thread.PID > 0 {
			byThread[items[i].Thread.PID] = append(byThread[items[i].Thread.PID], i)
		}
	}
	if len(byThread) < 2 {
		return
	}
	wakes := newWakeeWakeupIndex(idx, items)
	next := make([]int, len(items))
	for i := range items {
		next[i] = -1
		if blockingRowWokenByEdge(items[i]) {
			next[i] = blockingWakeRelayRow(wakes, items, byThread, i)
		}
	}
	for i := range items {
		if next[i] < 0 {
			continue
		}
		j, seen := next[i], map[int]bool{i: true}
		for next[j] >= 0 && !seen[j] {
			seen[j] = true
			j = next[j]
		}
		if next[j] >= 0 {
			continue // a cycle of relays names no upstream party
		}
		upstream := items[j].Peer
		if !threadRefResolved(upstream) || sameThreadRef(upstream, items[i].Thread) {
			continue
		}
		items[i].WakeRelay = true
		items[i].WakeRelayUpstream = upstream
		items[i].Summary = appendRootCauseSummaryDetail(items[i].Summary, fmt.Sprintf(
			"the waking thread %s was itself parked in the same recorded wait %q and was woken before it woke this waiter: it passed the wakeup on after the awaited work finished and is not the party this wait depended on; the wake chain leads to %s",
			threadLabel(items[i].Peer), strings.TrimSpace(items[i].WaitObject), threadLabel(upstream)))
	}
}

// blockingRowWokenByEdge is the typed shape both rows of a relay pair share.
func blockingRowWokenByEdge(item CriticalBlockingCandidate) bool {
	return item.Type == "blocking_span" && item.BlockingKind == "" &&
		item.PeerSource == CounterpartSourceWakeupEdge && item.Peer.PID > 0 &&
		item.peerWakeTs > 0 && strings.TrimSpace(item.WaitObject) != ""
}

// blockingWakeRelayRow returns the index of the waker's own row that proves
// row i's waker was a relay, or -1.
func blockingWakeRelayRow(wakes *wakeeWakeupIndex, items []CriticalBlockingCandidate, byThread map[int][]int, i int) int {
	w := items[i]
	wait := strings.TrimSpace(w.WaitObject)
	for _, j := range byThread[w.Peer.PID] {
		if j == i {
			continue
		}
		x := items[j]
		if x.Thread.TGID > 0 && w.Peer.TGID > 0 && x.Thread.TGID != w.Peer.TGID {
			continue
		}
		if strings.TrimSpace(x.WaitObject) != wait || x.Peer.PID == w.Thread.PID {
			continue
		}
		if x.StartTs <= w.peerWakeTs && x.peerWakeTs >= w.StartTs && x.peerWakeTs <= w.peerWakeTs &&
			x.EndTs <= w.peerWakeTs && !wakes.wokenBetween(w.Peer, x.peerWakeTs, w.peerWakeTs) {
			return j
		}
	}
	return -1
}

// wakeeWakeupIndex holds, per woken thread id, the sorted timestamps of the
// wakeup and waking rows naming that thread (a sched_wakeup_new row starts a
// new incarnation and is never a wake of the waiting thread). It is built
// once per relay pass over the threads a relay check can ask about — the
// wakers of rows whose counterpart came from a wakeup edge — so each check
// is a binary search instead of a scan of every event. A nil index (no trace
// index) proves nothing.
type wakeeWakeupIndex struct {
	byPID map[int][]float64
}

func newWakeeWakeupIndex(idx *Index, items []CriticalBlockingCandidate) *wakeeWakeupIndex {
	if idx == nil {
		return nil
	}
	wanted := map[int]bool{}
	for i := range items {
		if blockingRowWokenByEdge(items[i]) {
			wanted[items[i].Peer.PID] = true
		}
	}
	out := &wakeeWakeupIndex{byPID: map[int][]float64{}}
	for i := range idx.Events {
		ev := &idx.Events[i]
		if ev.Type != EventSchedWakeup && ev.Type != EventSchedWaking {
			continue
		}
		if ev.WakeePID <= 0 || !wanted[ev.WakeePID] || schedWakeupStartsNewIncarnation(*ev) {
			continue
		}
		out.byPID[ev.WakeePID] = append(out.byPID[ev.WakeePID], ev.Ts)
	}
	for _, ts := range out.byPID {
		sort.Float64s(ts)
	}
	return out
}

// wokenBetween reports a wakeup row naming thread strictly after the first
// timestamp and strictly before the second (HMC377 D2-R4: the waker slept
// again between its own release and the wake it passed on). A missing index
// reports true, so the relay stays unproven (fail closed). The thread is
// matched by its positive id (blockingRowWokenByEdge admits only such
// wakers), exactly as threadMatches does for a positive id.
func (w *wakeeWakeupIndex) wokenBetween(thread ThreadRef, after, before float64) bool {
	if w == nil {
		return true
	}
	ts := w.byPID[thread.PID]
	first := sort.Search(len(ts), func(i int) bool { return ts[i] > after })
	return first < len(ts) && ts[first] < before
}
