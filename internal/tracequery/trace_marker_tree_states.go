package tracequery

import (
	"math"
	"path/filepath"
	"sort"
)

type traceMarkerTreeStateIndex struct {
	lanes       map[ThreadState]*traceMarkerTreeIntegral
	sleepIO     traceMarkerTreeIntegral
	unavailable string
}

type traceMarkerTreeStateCache struct {
	idx         *Index
	query       Query
	cache       *chainQueryCache
	owners      map[int]*traceMarkerTreeStateIndex
	unavailable string
}

func newTraceMarkerTreeStateCache(idx *Index, q Query, safe bool) *traceMarkerTreeStateCache {
	c := &traceMarkerTreeStateCache{idx: idx, query: q, owners: map[int]*traceMarkerTreeStateIndex{}}
	switch {
	case !safe:
		c.unavailable = "scheduler_integrity_unavailable"
	case idx.RelationScoped || len(idx.TraceArtifacts) != 1 || !idx.TraceArtifacts[0].CausalCompatible || idx.TraceArtifacts[0].VirtualLineBase != 0 || filepath.Clean(idx.TraceArtifacts[0].SourcePath) != filepath.Clean(idx.Path):
		c.unavailable = "single_source_owner_timeline_unavailable"
	case q.LineStart > 0 || q.LineEnd > 0:
		c.unavailable = "line_bounded_scheduler_coverage_unavailable"
	}
	return c
}

func (c *traceMarkerTreeStateCache) owner(n TraceMarkerTreeNode) *traceMarkerTreeStateIndex {
	if c.unavailable != "" {
		return &traceMarkerTreeStateIndex{unavailable: c.unavailable}
	}
	if n.Thread.PID <= 0 || filepath.Clean(n.SourcePath) != filepath.Clean(c.idx.Path) {
		return &traceMarkerTreeStateIndex{unavailable: "marker_owner_timeline_unavailable"}
	}
	if state, ok := c.owners[n.Thread.PID]; ok {
		return state
	}
	if c.cache == nil {
		c.cache = newChainQueryCache(c.idx, c.query.runCancel)
	}
	owner := c.query
	owner.PID, owner.Thread, owner.ThreadInput, owner.TargetScope = n.Thread.PID, "", "", TargetScopeThread
	tl := c.cache.timeline(owner, n.Thread)
	state := &traceMarkerTreeStateIndex{lanes: map[ThreadState]*traceMarkerTreeIntegral{}}
	c.owners[n.Thread.PID] = state
	if tl.IntegrityFailure != "" {
		state.unavailable = "owner_scheduler_integrity_failure"
		return state
	}
	// A known interval after an unknown head still carries local evidence.
	// Keep the gap unknown rather than requiring a whole-window domain proof.
	intervals := append([]Interval(nil), tl.Intervals...)
	sort.SliceStable(intervals, func(i, j int) bool { return intervals[i].StartTs < intervals[j].StartTs })
	lastEnd, seen := 0.0, false
	for _, it := range intervals {
		if c.query.runCancel.tick() {
			state.unavailable = "canceled"
			return state
		}
		if it.EndTs <= it.StartTs {
			continue
		}
		if math.IsNaN(it.StartTs) || math.IsNaN(it.EndTs) || math.IsInf(it.StartTs, 0) || math.IsInf(it.EndTs, 0) || seen && it.StartTs < lastEnd {
			state.unavailable = "owner_scheduler_intervals_conflict"
			return state
		}
		lastEnd, seen = it.EndTs, true
		switch it.State {
		case StateRunning, StateRunnable, StateSSleep, StateDSleep, StateIOWait, StateStopped, StateDead:
			lane := state.lanes[it.State]
			if lane == nil {
				lane = &traceMarkerTreeIntegral{}
				state.lanes[it.State] = lane
			}
			lane.append(it.StartTs, it.EndTs)
		case StateUnknown:
			// Deliberately unbooked: the account's unknown remainder retains
			// this interval instead of pretending it is zero or a known lane.
		}
		if it.State == StateSSleep && it.BlockedReasonIOWaitKnown && it.BlockedReasonIOWait > 0 {
			state.sleepIO.append(it.StartTs, it.EndTs)
		}
	}
	return state
}

func (c *traceMarkerTreeStateCache) account(n TraceMarkerTreeNode, segments []TimeWindow) *TraceMarkerTreeAccount {
	a := &TraceMarkerTreeAccount{}
	for _, segment := range segments {
		a.Segments = append(a.Segments, TraceMarkerTreeWindow{StartTs: segment.StartTs, EndTs: segment.EndTs})
		a.DurationMs += (segment.EndTs - segment.StartTs) * 1000
	}
	state := c.owner(n)
	a.States = &TraceMarkerTreeStates{Coverage: "unavailable", UnknownMs: a.DurationMs}
	if state.unavailable != "" {
		a.States.Reasons = []string{state.unavailable}
	} else {
		value := &TraceMarkerTreeStateValues{}
		targets := []struct {
			state ThreadState
			value *float64
		}{
			{StateRunning, &value.RunningMs}, {StateRunnable, &value.RunnableMs},
			{StateSSleep, &value.SleepMs}, {StateDSleep, &value.DStateMs}, {StateIOWait, &value.IOWaitMs},
			{StateStopped, &value.StoppedMs}, {StateDead, &value.DeadMs},
		}
		for _, target := range targets {
			if integral := state.lanes[target.state]; integral != nil {
				*target.value = integral.sum(segments)
				value.AccountedMs += *target.value
			}
		}
		value.SleepIOWaitMs = state.sleepIO.sum(segments)
		unknown := a.DurationMs - value.AccountedMs
		// Precision guard only: no negative duration clamp conceals overlap.
		epsilon := math.Max(1e-12, a.DurationMs*1e-10)
		if unknown < -epsilon || value.SleepIOWaitMs > value.SleepMs+epsilon {
			a.States.Reasons = []string{"owner_scheduler_account_conflict"}
		} else if value.AccountedMs > 0 || a.DurationMs == 0 {
			a.States.Values, a.States.Coverage = value, "partial"
			a.States.UnknownMs = maxFloat(unknown, 0)
			if unknown <= epsilon {
				a.States.Coverage, a.States.UnknownMs = "complete", 0
			}
		} else {
			a.States.Reasons = []string{"no_known_owner_scheduler_intervals"}
		}
	}
	if len(a.Segments) > TraceMarkerTreeSegmentLimit {
		a.OmittedSegments = len(a.Segments) - TraceMarkerTreeSegmentLimit
		a.Segments = a.Segments[:TraceMarkerTreeSegmentLimit]
	}
	return a
}
