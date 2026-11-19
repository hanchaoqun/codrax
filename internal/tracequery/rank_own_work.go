package tracequery

import "strings"

// rank_own_work.go — own-work pricing (HMC377 E1+E6).
//
// A row of the analysed thread's own work (OnChainBasis
// self_deterministic_span, a marker that does not record a wait) publishes:
//   - EffectiveImpactMs / ImpactMs: the priced own work — the nested-safe
//     union of every member clipped to the window, minus the instants the
//     thread spends in a wait of its own dependency chain (those are priced by
//     the wait row of the same spans);
//   - CumulativeImpactMs: the complete member union in the window (waits
//     included), the disclosure total;
//   - MemberCount: every member in the window, never the listed sample;
//     MemberNotListedCount: how many the bounded roster does not list;
//   - OwnWorkOnCPUMs: the part of the priced own work during which the thread
//     was on a CPU.
//
// The ranked value is wall time, not CPU time: removing the work removes the
// whole interval the thread spent inside it (on a CPU, waiting for a CPU, or
// blocked by the work itself), which is the eliminable magnitude the
// self-basis ruling prices (§29.61.1 ②). The on-CPU share is published beside
// it so the two are never confused. The thread's own running row prices the
// same running time on its supply-fold caliber; the exact shared instants are
// disclosed on that row (stampSelfGapSemanticOverlapDisclosure), never
// subtracted.

// targetRunningIntervalsForRank returns the analysed thread's running
// intervals projected into the query window, from the same cached timeline
// the thread's own running row is minted from. nil when the window or the
// target is not resolved: absence never claims an on-CPU share.
func targetRunningIntervalsForRank(idx *Index, q Query, chain ChainResult, cache *chainQueryCache) func() []foldInterval {
	return func() []foldInterval {
		if idx == nil || cache == nil || q.TimeEnd <= q.TimeStart {
			return nil
		}
		target := chain.Target
		if target.PID <= 0 && strings.TrimSpace(target.Comm) == "" {
			return nil
		}
		tq := q
		tq.View = ""
		tq.PID = target.PID
		tq.Thread = target.Comm
		tq.ThreadInput = ""
		tl := cache.timeline(tq, target)
		var running []foldInterval
		for _, it := range tl.Intervals {
			if it.State != StateRunning || it.EndTs <= it.StartTs {
				continue
			}
			running = append(running, foldInterval{start: it.StartTs, end: it.EndTs})
		}
		merged, _ := foldIntervalUnionWithDisjoint(running)
		return merged
	}
}

// stampOwnWorkOnCPU sets OwnWorkOnCPUMs on every own-work row of the analysed
// thread among items: |own-work intervals ∩ running intervals|. The own-work
// intervals are the row's own support inventory (family rows) or its priced
// window-clipped extent (one-member rows).
func stampOwnWorkOnCPU(items []RootCauseRankItem, target ThreadRef, runningOf func() []foldInterval) {
	var running []foldInterval
	loaded := false
	for i := range items {
		item := &items[i]
		if item.OnChainBasis != RootCauseOnChainBasisSelfDeterministicSpan || strings.TrimSpace(item.SemanticClass) == "" ||
			!sameThreadRef(item.Thread, target) || semanticSpanRecordsWait(item.SpanName) {
			continue
		}
		own := item.semanticMemberIntervals
		if len(own) == 0 {
			if item.MemberCount > 1 || item.EndTs <= item.StartTs {
				continue
			}
			own = []foldInterval{{start: item.StartTs, end: item.EndTs}}
		}
		if !loaded {
			running, loaded = runningOf(), true
		}
		if len(running) == 0 {
			return
		}
		merged, _ := foldIntervalUnionWithDisjoint(own)
		var hits []foldInterval
		for _, span := range merged {
			for _, run := range running {
				lo, hi := maxFloat(span.start, run.start), minFloat(span.end, run.end)
				if hi > lo {
					hits = append(hits, foldInterval{start: lo, end: hi})
				}
			}
		}
		if ms, _ := foldIntervalUnionMs(hits); ms > 0 {
			item.OwnWorkOnCPUMs = ms
		}
	}
}
