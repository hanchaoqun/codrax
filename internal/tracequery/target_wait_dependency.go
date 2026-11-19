package tracequery

// target_wait_dependency.go — HMC377 E2: the dependency credit of a thread
// that woke the analysis target is bounded by the wait that wakeup ended.
//
// Ruling (product owner, 2026-11-18, batch 377 — "同意继续", given after being
// told it re-prices three previously accepted witness values to zero, e.g.
// tieba threads 61839 / 59566): R3/R4 "before the edge" means INSIDE the wait
// of the analysis target that the edge ended. A thread other than the target
// that holds a typed wakeup credential toward the target (R3, §29.88.1) is
// priced on the chain only for the part of its own inventory (ready, D, IO
// wait segments, deterministic semantic spans) that lies inside a wait of the
// target closed by ITS OWN recorded wakeup — the closed window
// [wait start (window-clipped), wakeup]. A wakeup that finds the target not
// waiting closes no wait and prices nothing; the rest of the inventory (before
// the wait began, between waits, after the wakeup) is supporting context on
// the adjacent remainder seat. Supersedes the [window start, latest edge]
// reading of R3-IMPL (§29.97), ONCHAIN-3c (§29.139), the §40.28 ① tripwire
// wording and the §40.30 residual lane.
//
// Why: the old left boundary (query-window start) is not a statement about
// the target. However early the host finishes, the target cannot be woken
// before it went to sleep, so the saving is capped by the wait itself. On the
// 924 ms Douyin reboot window the old reading published 844.739 ms of other
// threads' ready time on the chain against 315.621 ms of target waiting.
//
// The window is exactly the one the chain expansion already hands to every
// waker it walks (expandChainSleepSegment recurses over [seg start, wakeup]);
// this file builds the same window for EVERY typed wait of the target, so a
// host the expansion did not walk is priced by the same geometry. The closing
// wakeup is resolved by the chain's own edge finder
// (findWakeupForWithSelection — the single judgement of which row ended a
// wait; never a second edge rule).
//
// Invariants (structural tests in target_wait_dependency_test.go):
//   - row: every priced segment of a host-edge seat lies inside a wait of the
//     target closed by that thread;
//   - sum: one closing wakeup per wait and disjoint waits, so the shares
//     priced to all hosts never exceed the target's S + D + IO time;
//   - conservation: inside + outside == the seat's inventory, exactly.

import (
	"sort"
)

// targetWaitDependencyWindow is one typed wait of the analysis target closed
// by a recorded wakeup of a thread other than the target: [startTs, endTs] is
// the wait (clipped to the chain window; a boundary-tolerance wakeup match
// never extends it past the wait's own end); wakeupTs is the closing row.
type targetWaitDependencyWindow struct {
	startTs, endTs float64
	wakeupTs       float64
}

// attachTargetWaitDependencyWindows builds the carrier on a finished chain
// result (called beside attachWakeupEdgeCensus, after every expansion pass).
// Fail-closed: no index, unresolved/PID-less target, degenerate chain window,
// target timeline integrity failure, or no PID-indexed wakeup inventory ⇒ no
// carrier ⇒ the host-edge lanes price nothing (there is no fallback to the
// old reading).
func attachTargetWaitDependencyWindows(cache *chainQueryCache, q Query, res *ChainResult) {
	if cache == nil || cache.idx == nil || res == nil {
		return
	}
	target := res.Target
	if target.PID <= 0 || res.Window.EndTs <= res.Window.StartTs {
		return
	}
	ids, indexed := cache.wakeupsForThread(target)
	if !indexed {
		return
	}
	// The same target timeline the chain build expanded (same cache key).
	tq := q
	tq.PID = target.PID
	tq.Thread = target.Comm
	timeline := cache.timeline(tq, target)
	if timeline.IntegrityFailure != "" {
		return
	}
	// Candidate wakeup rows naming the target, ordered by timestamp, so each
	// wait hands the edge finder exactly the rows its own filter could accept
	// ([wait start, closing upper bound of the wait end]). The finder's
	// selection (type, new-incarnation guard, wakee identity, latest row) is
	// applied unchanged to that sub-range.
	type candidate struct {
		ts float64
		id int
	}
	lo, hi := res.Window.StartTs, wakeupClosingUpperBound(res.Window.EndTs)
	candidates := make([]candidate, 0, len(ids))
	for _, id := range ids {
		if id < 0 || id >= len(cache.idx.Events) {
			continue
		}
		ts := cache.idx.Events[id].Ts
		if ts >= lo && ts <= hi {
			candidates = append(candidates, candidate{ts: ts, id: id})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].ts < candidates[j].ts })
	out := map[int][]targetWaitDependencyWindow{}
	selection := make([]int, 0, 8)
	for _, iv := range timeline.Intervals {
		switch iv.State {
		case StateSSleep, StateDSleep, StateIOWait:
		default:
			continue
		}
		start, end := iv.StartTs, iv.EndTs
		if start < res.Window.StartTs {
			start = res.Window.StartTs
		}
		if end > res.Window.EndTs {
			end = res.Window.EndTs
		}
		if end <= start {
			continue
		}
		upper := wakeupClosingUpperBound(iv.EndTs)
		first := sort.Search(len(candidates), func(i int) bool { return candidates[i].ts >= iv.StartTs })
		selection = selection[:0]
		for i := first; i < len(candidates) && candidates[i].ts <= upper; i++ {
			selection = append(selection, candidates[i].id)
		}
		if len(selection) == 0 {
			continue
		}
		wakeup, _ := findWakeupForWithSelection(cache.idx, target, iv.StartTs, iv.EndTs, selection, true)
		if wakeup == nil || wakeup.PID <= 0 || wakeup.PID == target.PID {
			continue // no recorded wakeup, interrupt/idle context, or a self wakeup
		}
		closeTs := wakeup.Ts
		if closeTs > end {
			closeTs = end
		}
		if closeTs <= start {
			continue
		}
		out[wakeup.PID] = append(out[wakeup.PID], targetWaitDependencyWindow{startTs: start, endTs: closeTs, wakeupTs: wakeup.Ts})
	}
	for pid := range out {
		windows := out[pid]
		sort.SliceStable(windows, func(i, j int) bool { return windows[i].startTs < windows[j].startTs })
	}
	res.directDependencyWindows = out
	res.directDependencyWindowsBuilt = true
}

// hostDirectDependencyWindows returns the host's closed windows (sorted,
// pairwise disjoint — each is a distinct interval of the target's own state
// partition). nil when the carrier is absent or holds nothing for the host.
func hostDirectDependencyWindows(chain *ChainResult, host ThreadRef) []targetWaitDependencyWindow {
	if chain == nil || !chain.directDependencyWindowsBuilt || host.PID <= 0 {
		return nil
	}
	return chain.directDependencyWindows[host.PID]
}

// dependencyWindowSplit is one inventory's exact partition against a host's
// dependency windows: pre* = the pieces INSIDE the windows (priced on the
// chain), post* = the pieces OUTSIDE them (supporting context). The two lists
// are disjoint and reproduce the inventory exactly (preMs + postMs == Σ).
// lastWakeupTs is the closing wakeup of the latest window that holds a
// positive inside piece (the disclosed wakeup of the priced share);
// waitCount / waitMs describe those windows.
type dependencyWindowSplit struct {
	stateEdgeSegmentSplit
	lastWakeupTs float64
	waitCount    int
	waitMs       float64
}

// splitFoldIntervalsByDependencyWindows partitions a validated segment
// inventory (or a span member list) against the host's windows.
func splitFoldIntervalsByDependencyWindows(intervals []foldInterval, windows []targetWaitDependencyWindow) dependencyWindowSplit {
	var out dependencyWindowSplit
	addInside := func(s, e float64) {
		ms := (e - s) * 1000
		out.preMs += ms
		out.pre = append(out.pre, foldInterval{start: s, end: e, valueMs: ms})
		if out.preStart == 0 || s < out.preStart {
			out.preStart = s
		}
		if e > out.preEnd {
			out.preEnd = e
		}
	}
	addOutside := func(s, e float64) {
		if e <= s {
			return
		}
		ms := (e - s) * 1000
		out.postMs += ms
		out.post = append(out.post, foldInterval{start: s, end: e, valueMs: ms})
		if out.postStart == 0 || s < out.postStart {
			out.postStart = s
		}
		if e > out.postEnd {
			out.postEnd = e
		}
	}
	priced := make([]bool, len(windows))
	for _, iv := range intervals {
		if iv.end <= iv.start {
			continue
		}
		cursor := iv.start
		for wi, w := range windows {
			if w.endTs <= cursor {
				continue
			}
			if w.startTs >= iv.end {
				break
			}
			s := maxFloat(cursor, w.startTs)
			e := minFloat(iv.end, w.endTs)
			if e <= s {
				continue
			}
			addOutside(cursor, s)
			addInside(s, e)
			priced[wi] = true
			cursor = e
			if cursor >= iv.end {
				break
			}
		}
		addOutside(cursor, iv.end)
	}
	for wi, w := range windows {
		if !priced[wi] {
			continue
		}
		out.waitCount++
		out.waitMs += (w.endTs - w.startTs) * 1000
		if w.wakeupTs > out.lastWakeupTs {
			out.lastWakeupTs = w.wakeupTs
		}
	}
	return out
}

// pricedBy returns the disclosure form of a host credential for one priced
// share: the wakeup that closed the latest priced wait, and the direct word —
// a dependency window is by construction a wait of the target ended by the
// host's own direct wakeup (a chain-hop edge never opens one; its window is
// the chain node window the overlap / RSPA lanes price).
func (a semanticHostEdgeAnchor) pricedBy(split dependencyWindowSplit) semanticHostEdgeAnchor {
	return semanticHostEdgeAnchor{boundaryTs: split.lastWakeupTs, direct: true, windows: a.windows}
}

// hostSemanticSpanDependencySplit is the span-side consumer of the bound: the
// host's typed edge is the credential, and the span [start, end] is priced
// only for its pieces inside the host's dependency windows. ok=false when the
// host holds no credential or the span has no positive share inside a window
// (the span then keeps its non-chain lane byte-identically).
func hostSemanticSpanDependencySplit(chain *ChainResult, host ThreadRef, start, end float64) (semanticHostEdgeAnchor, dependencyWindowSplit, bool) {
	anchor, ok := hostSemanticSpanEdgeAnchor(chain, host)
	if !ok || end <= start {
		return semanticHostEdgeAnchor{}, dependencyWindowSplit{}, false
	}
	split := splitFoldIntervalsByDependencyWindows([]foldInterval{{start: start, end: end}}, anchor.windows)
	if split.preMs <= 0 {
		return semanticHostEdgeAnchor{}, dependencyWindowSplit{}, false
	}
	return anchor.pricedBy(split), split, true
}

// spanInsideOneDependencyWindow reports whether [start, end] lies entirely
// inside one window (the verbatim-member discipline of the business-clue
// mention face: a member is either wholly inside a wait the host ended or not
// mentioned on this basis at all — no bisected values on that face).
func spanInsideOneDependencyWindow(windows []targetWaitDependencyWindow, start, end float64) bool {
	for _, w := range windows {
		if start >= w.startTs && end <= w.endTs {
			return true
		}
	}
	return false
}
