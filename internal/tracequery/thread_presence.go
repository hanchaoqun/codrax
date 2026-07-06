package tracequery

// thread_presence.go — P0-E2a cross-thread blocking-counterpart resolution
// (ledger docs/design/real_trace_campaign_20260705.md §10 A2 / §11 N8 / §12
// Q4-C, user ruling §12.3-5). Two deterministic primitives back the three
// counterpart-fallback forms (lock holder / binder dest / blocking-span
// object):
//
//  1. tidPresenceSet — the precise "is this id resolvable in THIS trace?"
//     judgement. A structured contention/binder payload can print an owner or
//     dest tid drawn from a DIFFERENT pid namespace: threadRefResolved reports
//     it as resolved (PID>0), yet buildCriticalBlockingPeerState finds zero
//     timeline hits and the drill lane silently claims "drilled". The presence
//     set separates "payload printed an id" from "id names a thread this trace
//     actually scheduled/woke", exactly the four-field surface resolveThread
//     already scans (PID / PrevPID / NextPID / WakeePID).
//
//  2. resolveCounterpartViaWakeupEdge — when the payload tid is NOT present,
//     the direct 1-hop wakeup edge names a real in-trace counterpart: the
//     last sched_wakeup whose wakee is the waiter (findWakeupForWithSelection,
//     wakeupMatchToleranceSec) has a WAKER that IS host-namespace-resolvable
//     and drillable. The waker replaces the phantom peer so the six P0-E1
//     typed-pair gates read true again through a real thread instead of a
//     ghost id.
//
// Neither primitive touches the lock_contention.go payload parse: the `(owner
// tid: N)` extraction is retained verbatim per §12.3-5; the fallback fires only
// when that tid is not present in the current trace.
//
// counterpartWakeupEdgeConfidence is the visible confidence DOWNGRADE for any
// counterpart resolved via the wakeup-edge fallback (lock holder / binder peer /
// blocking-span object). A wakeup-edge peer is an INFERENCE ("the thread that
// woke the waiter"), not a payload-DIRECT resolution, so it rides the same
// receiver-inferred 0.62 confidence precedent ipc.go uses for a dest-hint edge.
// The rank Score = EffectiveImpactMs × Confidence × TypeWeight is monotone in
// Confidence, so an inferred counterpart naturally scores BELOW a payload-direct
// row of equal impact — no gate/tier logic reads the source, the demotion is
// entirely through the confidence channel (§1 precise-signal / soft-guidance).
const counterpartWakeupEdgeConfidence = 0.62

// tidPresenceSet lazily builds (exactly once per Index) the set of every tid
// that appears in any event's PID / PrevPID / NextPID / WakeePID field — the
// same four-field surface resolveThread scans. One O(N) pass, the same order
// of work as one buildThreadCatalog pass. Windowed builds get their own Index
// and therefore their own set (a payload tid outside the windowed slice reads
// "not present", which is correct: the fallback then anchors on an in-window
// wakeup edge). Safe for concurrent queries against a cached Index: sync.Once
// publishes an immutable map.
func (idx *Index) tidPresenceSet() map[int]bool {
	if idx == nil {
		return nil
	}
	idx.tidPresenceOnce.Do(func() {
		set := make(map[int]bool)
		scheduled := make(map[int]bool)
		for i := range idx.Events {
			ev := &idx.Events[i]
			if ev.PID > 0 {
				set[ev.PID] = true
			}
			if ev.PrevPID > 0 {
				set[ev.PrevPID] = true
			}
			if ev.NextPID > 0 {
				set[ev.NextPID] = true
			}
			if ev.WakeePID > 0 {
				set[ev.WakeePID] = true
			}
			// E2a correction ① (P0-E2b): the scheduled subset — only sched_switch
			// prev/next participation gives a thread actual context-switch
			// timeline material to drill. Built in the same single pass.
			if ev.Type == EventSchedSwitch {
				if ev.PrevPID > 0 {
					scheduled[ev.PrevPID] = true
				}
				if ev.NextPID > 0 {
					scheduled[ev.NextPID] = true
				}
			}
		}
		idx.tidPresence = set
		idx.tidScheduled = scheduled
	})
	return idx.tidPresence
}

// tidHasScheduledTimeline (E2a correction ①) reports whether a tid has actual
// context-switch timeline material in THIS trace — it appears as sched_switch
// prev/next, i.e. a subject==tid timeline/drilldown query would return real
// intervals. Strictly stronger than tidPresent: a thread only ever seen as a
// waker/wakee or event-header row is PRESENT (resolvable) but has nothing to
// drill — a "drilled" verdict for it would assert an examination that
// structurally cannot have decomposed anything.
func (idx *Index) tidHasScheduledTimeline(tid int) bool {
	if idx == nil || tid <= 0 {
		return false
	}
	idx.tidPresenceSet()
	return idx.tidScheduled[tid]
}

// tidPresent reports whether a tid names a thread this trace actually observed
// (scheduled, woke, or was woken) — as opposed to an id a payload merely
// printed. A non-positive tid is never "present". This is the precise signal
// the counterpart fallbacks branch on; it is intentionally distinct from
// threadRefResolved (which only asks "does the ThreadRef carry a pid or comm",
// not "is that pid in this trace").
func (idx *Index) tidPresent(tid int) bool {
	if idx == nil || tid <= 0 {
		return false
	}
	return idx.tidPresenceSet()[tid]
}

// threadRefPresent reports whether a resolved-looking ThreadRef actually names
// a thread this trace observed. A comm-only ThreadRef (PID<=0) is trusted (name
// resolution already went through the trace's own thread catalog); a
// PID-bearing ref must have that pid in the presence set.
func (idx *Index) threadRefPresent(t ThreadRef) bool {
	if t.PID > 0 {
		return idx.tidPresent(t.PID)
	}
	return threadRefResolved(t)
}

// counterpartWakeupResult is the typed result of a direct-1-hop wakeup-edge
// counterpart resolution: the waker thread that signalled the waiter, the
// wakeup timestamp and source line, and whether a usable edge was found.
type counterpartWakeupResult struct {
	Waker    ThreadRef
	WakeupTs float64
	Line     int
	OK       bool
}

// resolveCounterpartViaWakeupEdge finds the direct 1-hop counterpart of a
// blocked waiter: the WAKER of the last sched_wakeup(wakee=waiter) inside
// [startTs, endTs] (findWakeupForWithSelection already applies the
// wakeupMatchToleranceSec window slack). Guards, all precise:
//
//   - no wakeup row for the waiter in the window → OK=false (nothing to
//     fall back to);
//   - waker PID==0 (swapper/idle timeout wakeup) → OK=false: an idle-thread
//     wakeup names no real counterpart (ownerless, §12.3 延伸), and promoting
//     the idle thread as the peer would fabricate a bogus holder.
//
// The waker is by construction host-namespace-resolvable (it is a real event's
// row-header thread), so a peer built from it satisfies threadRefPresent and
// the P0-E1 typed-pair gates through a genuinely drillable thread.
func resolveCounterpartViaWakeupEdge(idx *Index, waiter ThreadRef, startTs, endTs float64) counterpartWakeupResult {
	if idx == nil || startTs <= 0 || endTs < startTs {
		return counterpartWakeupResult{}
	}
	ev, _ := findWakeupForWithSelection(idx, waiter, startTs, endTs, nil, false)
	if ev == nil {
		return counterpartWakeupResult{}
	}
	if ev.PID <= 0 {
		// swapper/idle (pid 0) timeout wakeup — no real counterpart.
		return counterpartWakeupResult{}
	}
	return counterpartWakeupResult{
		Waker:    ThreadRef{Comm: ev.Comm, PID: ev.PID, TGID: ev.TGID},
		WakeupTs: ev.Ts,
		Line:     ev.Line,
		OK:       true,
	}
}
