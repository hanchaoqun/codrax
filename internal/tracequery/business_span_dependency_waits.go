package tracequery

// business_span_dependency_waits.go — HMC377 E3: the typed window inside
// which a thread that woke the analysed thread directly is on its chain.
//
// A direct wakeup is on the dependency chain only for the wait it ended: the
// analysed thread's sleep / uninterruptible / IO-wait interval that the
// recorded wakeup closed. Work the waker did before that wait began, or a
// wakeup that found the thread not waiting, is temporal proximity. The
// interval is read from the analysed thread's own scheduler timeline (the
// same exited-state rule as the wakeup-edge census, wakeupExitStateBucket);
// nothing is inferred from names or durations.

import "sort"

// targetEndedWaitWindowsByWaker returns, per waker thread id, the merged
// windows [wait start, wakeup] of the analysed thread's waits that waker's
// in-window wakeups ended. nil when there is no resolved target, no cache or
// no such wait.
func targetEndedWaitWindowsByWaker(idx *Index, cache *chainQueryCache, q Query, chain ChainResult) map[int][]TimeWindow {
	target := chain.Target
	if idx == nil || cache == nil || target.PID <= 0 || chain.Window.EndTs <= chain.Window.StartTs {
		return nil
	}
	tq := q
	tq.PID, tq.Thread = target.PID, target.Comm
	tq.TimeStart, tq.TimeEnd = chain.Window.StartTs, chain.Window.EndTs
	tl := cache.timeline(tq, target)
	if tl.IntegrityFailure != "" || len(tl.Intervals) == 0 {
		return nil
	}
	intervals := append([]Interval(nil), tl.Intervals...)
	sort.SliceStable(intervals, func(i, j int) bool { return intervals[i].StartTs < intervals[j].StartTs })
	ids, indexed := cache.wakeupsForThread(target)
	collect := func(eventType EventType) map[int][]TimeWindow {
		var out map[int][]TimeWindow
		visit := func(i int) {
			if i < 0 || i >= len(idx.Events) {
				return
			}
			ev := &idx.Events[i]
			if ev.Type != eventType || schedWakeupStartsNewIncarnation(*ev) || ev.PID <= 0 || ev.PID == target.PID {
				return
			}
			if ev.Ts < chain.Window.StartTs || ev.Ts > chain.Window.EndTs || !threadMatches(target, ev.WakeePID, ev.WakeeComm) {
				return
			}
			wait, ok := targetWaitEndedAt(intervals, ev.Ts)
			if !ok {
				return
			}
			start := maxFloat(wait.StartTs, chain.Window.StartTs)
			if ev.Ts <= start {
				return
			}
			if out == nil {
				out = map[int][]TimeWindow{}
			}
			out[ev.PID] = append(out[ev.PID], TimeWindow{StartTs: start, EndTs: ev.Ts})
		}
		if indexed {
			for _, id := range ids {
				visit(id)
			}
		} else {
			for i := range idx.Events {
				visit(i)
			}
		}
		return out
	}
	out := collect(EventSchedWakeup)
	if out == nil {
		// Single-ruler fallback, the census's own rule: a sched_waking-only
		// trace still records who woke the thread.
		out = collect(EventSchedWaking)
	}
	for pid, windows := range out {
		out[pid] = mergeAnchorTimeWindows(windows)
	}
	return out
}

// targetWaitEndedAt returns the analysed thread's interval a wakeup at ts
// ended when that interval is a wait (interruptible sleep, uninterruptible
// wait or IO wait): the interval containing ts with its start strictly before
// ts (wakeupExitStateBucket's rule). Stricter than the census: a wait that
// ended before ts (a timeline gap) was not ended by this wakeup.
func targetWaitEndedAt(intervals []Interval, ts float64) (Interval, bool) {
	i := sort.Search(len(intervals), func(i int) bool { return intervals[i].StartTs >= ts })
	for j := i - 1; j >= 0; j-- {
		it := intervals[j]
		if it.EndTs <= it.StartTs {
			continue
		}
		if ts > it.EndTs+1e-9 {
			return Interval{}, false
		}
		// One exited-state classifier for every wakeup consumer: the
		// census's own sleep / D-family buckets.
		if bucket := wakeupExitStateBucket([]Interval{it}, ts); bucket == "sleep" || bucket == "d" {
			return it, true
		}
		return Interval{}, false
	}
	return Interval{}, false
}
