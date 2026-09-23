package tracequery

import (
	"math"
	"sort"
)

type schedulerConcurrencyKey struct{ source, state string }
type schedulerConcurrencySource struct {
	source   string
	conflict bool
}
type schedulerConcurrencyAccumulator struct {
	group     SchedulerConcurrencyGroup
	intervals map[int][]timeInterval
}
type schedulerConcurrencyCollector struct {
	idx                    *Index
	q                      Query
	identity               *queryPIDIdentityFilter
	out                    SchedulerConcurrencyStats
	groups                 map[schedulerConcurrencyKey]*schedulerConcurrencyAccumulator
	cpuSources, tidSources map[int]schedulerConcurrencySource
	nextSwitch             map[int]int // physical start line -> actual next switch ordinal
	seenSwitchLines        map[int]bool
	switches               map[int][]int
}

func newSchedulerConcurrencyCollector(idx *Index, q Query, identity *queryPIDIdentityFilter) *schedulerConcurrencyCollector {
	c := &schedulerConcurrencyCollector{idx: idx, q: q, identity: identity,
		groups:     map[schedulerConcurrencyKey]*schedulerConcurrencyAccumulator{},
		cpuSources: map[int]schedulerConcurrencySource{}, tidSources: map[int]schedulerConcurrencySource{},
		nextSwitch: map[int]int{}, seenSwitchLines: map[int]bool{}, switches: map[int][]int{},
		out: SchedulerConcurrencyStats{Population: SchedulerConcurrencyPopulationClosedIntervals,
			ThreadScope: SchedulerConcurrencyThreadScopeAll, QueryPID: q.PID, LineStart: q.LineStart, LineEnd: q.LineEnd}}
	w := queryResultTimeWindow(q)
	if q.LineStart > 0 || q.LineEnd > 0 {
		c.out.WindowUnavailableReason = "line_bounds_take_precedence"
	} else if w.StartDetermined() && schedulerConcurrencyFinite(w.StartTs) && schedulerConcurrencyFinite(w.EndTs) && w.EndTs > w.StartTs && schedulerConcurrencyFinite((w.EndTs-w.StartTs)*1000) {
		c.out.Window = &SchedulerConcurrencyWindow{w.StartTs, w.EndTs}
	} else {
		c.out.WindowUnavailableReason = "finite_positive_time_window_not_determined"
	}
	if _, tail := schedulerMeasuredTimeEnd(idx, q); tail {
		c.out.Coverage.Reasons = append(c.out.Coverage.Reasons, "requested_window_exceeds_artifact")
	}
	if idx.Windowed {
		c.out.Coverage.Reasons = append(c.out.Coverage.Reasons, "source_audit_limited_to_retained_rows")
	}
	last := map[int]int{}
	for i, ev := range idx.Events {
		if q.runCancel.tick() {
			return c
		}
		_, _, _, _, migration := schedMigrationTransition(ev)
		if ev.Type != EventSchedSwitch && ev.Type != EventSchedWakeup && ev.Type != EventSchedWaking && !migration {
			continue
		}
		source, ok := tracePairingSourceIdentity(idx, ev)
		if !ok {
			source = ""
		}
		mark := func(m map[int]schedulerConcurrencySource, id int) {
			if old, seen := m[id]; seen {
				old.conflict = old.conflict || old.source != source || source == ""
				m[id] = old
			} else {
				m[id] = schedulerConcurrencySource{source: source, conflict: source == ""}
			}
		}
		if ev.Type == EventSchedSwitch {
			c.seenSwitchLines[ev.Line] = true
			mark(c.cpuSources, ev.CPU)
			for _, pid := range []int{ev.PrevPID, ev.NextPID} {
				if pid > 0 {
					mark(c.tidSources, pid)
				}
			}
			if prior, exists := last[ev.CPU]; exists {
				c.nextSwitch[idx.Events[prior].Line] = i
			}
			last[ev.CPU] = i
			c.switches[ev.CPU] = append(c.switches[ev.CPU], i)
		} else if ev.WakeePID > 0 {
			mark(c.tidSources, ev.WakeePID)
		} else if pid, _, _, _, yes := schedMigrationTransition(ev); yes && pid > 0 {
			mark(c.tidSources, pid)
		}
	}
	return c
}

func schedulerConcurrencyFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func (c *schedulerConcurrencyCollector) group(source, state string) *schedulerConcurrencyAccumulator {
	k := schedulerConcurrencyKey{source, state}
	if a := c.groups[k]; a != nil {
		return a
	}
	a := &schedulerConcurrencyAccumulator{group: SchedulerConcurrencyGroup{SourcePath: source, State: state}, intervals: map[int][]timeInterval{}}
	c.groups[k] = a
	return a
}

// Called only at the existing running producer. The old running arithmetic is
// untouched; this additional face requires a real next switch, including one
// outside the query window. A synthetic window boundary is never an endpoint.
func (c *schedulerConcurrencyCollector) running(ev Event) {
	if c == nil || ev.NextPID <= 0 || schedNextIsIdle(ev) {
		return
	}
	end, line := ev.Ts, 0
	ordinal, found := c.nextSwitch[ev.Line]
	if !found && !c.seenSwitchLines[ev.Line] {
		// A bounded index may carry an actual prefix endpoint only in its head
		// snapshot. The first subsequent retained switch can close that row.
		for _, i := range c.switches[ev.CPU] {
			candidate := c.idx.Events[i]
			if candidate.Line > ev.Line && candidate.Ts >= ev.Ts {
				ordinal, found = i, true
				break
			}
		}
	}
	valid := true
	if found {
		next := c.idx.Events[ordinal]
		end, line = next.Ts, next.Line
		valid = next.PrevPID == ev.NextPID
	}
	c.add(SchedulerConcurrencyStateRunning, ev.NextPID, ev.CPU, ev.Ts, end, ev.Line, line, found, valid)
}

func (c *schedulerConcurrencyCollector) runnable(start offCPUStart, end float64, endLine int, closure string) {
	if c == nil {
		return
	}
	closed := closure == runnableCPUContinuityBoundarySchedIn || closure == runnableCPUContinuityBoundaryMigration
	c.add(SchedulerConcurrencyStateRunnable, start.thread.PID, start.cpu, start.ts, end, start.line, endLine, closed, true)
}

func (c *schedulerConcurrencyCollector) add(state string, pid, cpu int, start, end float64, startLine, endLine int, closed, valid bool) {
	if c.q.runCancel.tick() || pid <= 0 {
		return
	}
	// Producer rows can cross the selection. Half-open overlap, not endpoint
	// containment, decides whether they contribute. Open rows are diagnostic.
	if w := c.out.Window; w != nil && (start >= w.EndTs || (closed && (end < w.StartTs || (end == w.StartTs && end > start)))) {
		return
	}
	source, ok := tracePairingSourceIdentity(c.idx, Event{Line: startLine})
	a := c.group(source, state)
	a.group.Coverage.CandidateIntervals++
	if !ok {
		a.group.Coverage.UnresolvedSourceIntervals++
		return
	}
	if c.tidSources[pid].conflict || (state == SchedulerConcurrencyStateRunning && c.cpuSources[cpu].conflict) {
		a.group.Coverage.SourceConflictIntervals++
		return
	}
	if !c.identity.allows(pid) {
		a.group.Coverage.IdentityExcludedIntervals++
		return
	}
	if !closed || endLine <= 0 {
		a.group.Coverage.OpenEndedIntervals++
		return
	}
	endSource, endOK := tracePairingSourceIdentity(c.idx, Event{Line: endLine})
	if !endOK {
		a.group.Coverage.UnresolvedSourceIntervals++
		return
	}
	if endSource != source {
		a.group.Coverage.SourceConflictIntervals++
		return
	}
	if !valid || !schedulerConcurrencyFinite(start) || !schedulerConcurrencyFinite(end) || end < start {
		a.group.Coverage.InvalidIntervals++
		return
	}
	a.group.Coverage.AcceptedIntervals++
	a.group.AcceptedIntervalCount++
	if w := c.out.Window; w != nil {
		start, end = math.Max(start, w.StartTs), math.Min(end, w.EndTs)
		if end >= start {
			a.intervals[pid] = append(a.intervals[pid], timeInterval{start: start, end: end})
		}
	}
}

func (c *schedulerConcurrencyCollector) finish(head *SchedulerHeadCoverage) *SchedulerConcurrencyStats {
	if c == nil || c.q.runCancel.sample() {
		return nil
	}
	if head != nil && head.Status != "recovered" {
		c.out.Coverage.Reasons = append(c.out.Coverage.Reasons, "window_head_not_fully_classified")
	}
	c.out.Coverage.IdentityExcludedTIDs = len(c.identity.suppressedPIDs())
	for _, a := range c.groups {
		if c.q.runCancel.tick() {
			return nil
		}
		if c.out.Window == nil {
			a.group.ValuesUnavailableReason = c.out.WindowUnavailableReason
		} else if a.group.AcceptedIntervalCount == 0 {
			a.group.ValuesUnavailableReason = "no_accepted_closed_intervals"
		} else if !finishSchedulerConcurrencyGroup(c.q, a, *c.out.Window) {
			return nil
		}
		finishSchedulerConcurrencyCoverage(&a.group.Coverage)
		addSchedulerConcurrencyCoverage(&c.out.Coverage, a.group.Coverage)
		c.out.Groups = append(c.out.Groups, a.group)
	}
	if len(c.out.Groups) == 0 && c.out.Coverage.IdentityExcludedTIDs == 0 {
		return nil
	}
	sort.Slice(c.out.Groups, func(i, j int) bool {
		a, b := c.out.Groups[i], c.out.Groups[j]
		if a.SourcePath != b.SourcePath {
			return a.SourcePath < b.SourcePath
		}
		return a.State < b.State
	})
	c.out.GroupCount = len(c.out.Groups)
	if len(c.out.Groups) > 8 {
		c.out.OmittedGroups = len(c.out.Groups) - 8
		c.out.Groups = c.out.Groups[:8]
	}
	finishSchedulerConcurrencyCoverage(&c.out.Coverage)
	if c.q.runCancel.sample() {
		return nil
	}
	return &c.out
}
