package tracequery

import (
	"fmt"
	"sort"
)

func buildProcessProfile(idx *Index, q Query) *ProcessProfile {
	p := &ProcessProfile{Status: "unavailable", SourcePath: idx.Path, Window: queryResultTimeWindow(q),
		MembershipBasis: "observed_native_tgid_members", Threads: []ProcessProfileThread{}}
	if !finiteSleepInventoryTime(q.TimeStart) || !finiteSleepInventoryTime(q.TimeEnd) || q.TimeEnd <= q.TimeStart || q.LineStart > 0 || q.LineEnd > 0 {
		p.Reason = "bounded_time_window_required"
		return p
	}
	p.WindowMs = (q.TimeEnd - q.TimeStart) * 1000
	if idx.RelationScoped || len(idx.TraceArtifacts) != 1 || !idx.TraceArtifacts[0].CausalCompatible {
		p.Reason = "single_complete_source_scope_required"
		return p
	}
	resolution := resolveThreadSelection(idx, q)
	p.SourceThread = resolution.Thread
	if resolution.Ambiguous || resolution.Thread.PID <= 0 {
		p.Reason = "unique_source_thread_required"
		return p
	}
	roster, conflicting := processProfileNativeRoster(idx, q)
	target := roster[resolution.Thread.PID]
	if target.TGID <= 0 || conflicting[target.PID] || threadIncarnationConflictForQuery(idx, q, resolution.Thread.PID) != nil {
		p.Reason = "source_thread_native_process_membership_unavailable"
		return p
	}
	p.TGID, p.SourceThread = target.TGID, target
	var members []ThreadRef
	for pid, ref := range roster {
		if conflicting[pid] || ref.TGID <= 0 {
			p.UnknownMembershipThreads++
			continue
		}
		if ref.TGID == p.TGID {
			members = append(members, ref)
		}
	}
	p.ThreadCount = len(members)
	// Compute all member summaries before selecting display rows. Reuse the
	// existing per-owner indexed timeline cache, not N complete trace scans.
	sort.Slice(members, func(i, j int) bool { return members[i].PID < members[j].PID })
	cache := newTraceMarkerTreeStateCache(idx, q, true)
	hotspots := processProfileBusinessHotspots(idx, q, members)
	for _, member := range members {
		if q.runCancel.tick() {
			return nil
		}
		node := TraceMarkerTreeNode{Thread: member, SourcePath: idx.Path}
		account := cache.account(node, []TimeWindow{p.Window})
		row := ProcessProfileThread{Thread: member, States: account.States}
		row.BusinessHotspots = hotspots[member.PID]
		row.BusinessGroupCount = len(row.BusinessHotspots)
		if len(row.BusinessHotspots) > processProfileSleepGroupLimit {
			row.BusinessHotspots = row.BusinessHotspots[:processProfileSleepGroupLimit]
		}
		row.OmittedBusinessGroups = row.BusinessGroupCount - len(row.BusinessHotspots)
		if row.States.Values != nil {
			pct := row.States.Values.RunningMs / p.WindowMs * 100
			row.RunningWindowPct = &pct
			owner := q
			owner.PID, owner.Thread, owner.ThreadInput, owner.TargetScope = member.PID, "", "", TargetScopeThread
			row.SleepGroups, row.SleepGroupCount = processProfileSleepGroups(cache.cache.timeline(owner, member))
			row.OmittedSleepGroups = row.SleepGroupCount - len(row.SleepGroups)
		} else {
			p.UnavailableThreads++
		}
		p.Threads = append(p.Threads, row)
	}
	sort.SliceStable(p.Threads, func(i, j int) bool {
		a, b := p.Threads[i].RunningWindowPct, p.Threads[j].RunningWindowPct
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		return *a > *b
	})
	limit := ViewCapacityFor("process_profile").ClampLimit(q.Limit)
	if len(p.Threads) > limit {
		p.Threads = p.Threads[:limit]
	}
	p.EmittedThreads, p.OmittedThreads = len(p.Threads), p.ThreadCount-len(p.Threads)
	p.Status = "observed_members"
	p.Caveats = []string{
		"Thread count covers natively identified observed members, not every OS thread. Unknown membership is a source-wide census and is not assigned to this process; names and marker payload votes never establish membership here.",
		"Per-thread percentages use the full selected wall-clock window; cross-thread durations can overlap and are not process elapsed time. Unknown scheduler time is retained. Open state tails do not prove a completed wait.",
		"Sleep groups organize thread/state/blocked caller observations, not call paths or wakeup dependencies. Missing caller stays unknown; S, D and proven IO stay separate. IO flags on S refine S time rather than adding another interval. These rows do not rank root causes or establish device blame.",
		"Business hotspots group closed synchronous marker instances within each emitter; times are window-clipped inclusive elapsed time, not CPU running, self time or root-cause contribution. Nested names can overlap and must not be added. Empty groups do not prove absent work.",
	}
	return p
}

func processProfileSleepGroups(tl TimelineResult) ([]ProcessSleepGroup, int) {
	type key struct {
		state  ThreadState
		caller string
	}
	groups := map[key]*ProcessSleepGroup{}
	for _, it := range tl.Intervals {
		switch it.State {
		case StateSSleep, StateDSleep, StateIOWait:
		default:
			continue
		}
		if it.DurationMs <= 0 {
			continue
		}
		k := key{it.State, it.BlockedReasonCaller}
		g := groups[k]
		if g == nil {
			g = &ProcessSleepGroup{State: it.State, Caller: it.BlockedReasonCaller, CallerKnown: it.BlockedReasonCaller != "", LineStart: it.StartLine}
			groups[k] = g
		}
		g.IntervalCount++
		g.DurationMs += it.DurationMs
		g.LineEnd = it.EndLine
		if it.BlockedReasonIOWaitKnown {
			g.IOFlagKnownIntervals++
			if it.BlockedReasonIOWait > 0 {
				g.IOFlagPositiveMs += it.DurationMs
			}
		}
	}
	rows := make([]ProcessSleepGroup, 0, len(groups))
	for _, g := range groups {
		rows = append(rows, *g)
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.DurationMs != b.DurationMs {
			return a.DurationMs > b.DurationMs
		}
		if a.State != b.State {
			return a.State < b.State
		}
		return a.Caller < b.Caller
	})
	total := len(rows)
	if total > processProfileSleepGroupLimit {
		rows = rows[:processProfileSleepGroupLimit]
	}
	return rows, total
}

func evidenceFromProcessProfile(p *ProcessProfile) []EvidenceFact {
	if p == nil || p.Status != "observed_members" {
		return nil
	}
	out := []EvidenceFact{{Subject: fmt.Sprintf("process %d", p.TGID), Predicate: "process_profile", StartTs: p.Window.StartTs, EndTs: p.Window.EndTs, Confidence: 1,
		Summary: fmt.Sprintf("Observed native-TGID members=%d; displayed=%d; omitted=%d; unavailable state accounts=%d; window_ms=%g. This is not a complete OS thread census or causal ranking.", p.ThreadCount, p.EmittedThreads, p.OmittedThreads, p.UnavailableThreads, p.WindowMs)}}
	// Detailed rows travel through the source/window-bound typed profile,
	// not duplicate JSON strings in generic truncated evidence summaries.
	return out
}
