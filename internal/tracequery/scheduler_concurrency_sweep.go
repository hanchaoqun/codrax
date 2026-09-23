package tracequery

import "sort"

func finishSchedulerConcurrencyGroup(q Query, a *schedulerConcurrencyAccumulator, window SchedulerConcurrencyWindow) bool {
	deltas := map[float64]int{window.StartTs: 0, window.EndTs: 0}
	for _, intervals := range a.intervals {
		if q.runCancel.tick() {
			return false
		}
		a.group.ThreadCount++
		sort.Slice(intervals, func(i, j int) bool {
			if intervals[i].start != intervals[j].start {
				return intervals[i].start < intervals[j].start
			}
			return intervals[i].end < intervals[j].end
		})
		var merged []timeInterval
		for _, iv := range intervals {
			if q.runCancel.tick() {
				return false
			}
			if iv.end <= iv.start {
				continue
			}
			if n := len(merged); n > 0 && iv.start <= merged[n-1].end {
				if iv.end > merged[n-1].end {
					merged[n-1].end = iv.end
				}
			} else {
				merged = append(merged, iv)
			}
		}
		for _, iv := range merged {
			deltas[iv.start]++
			deltas[iv.end]--
		}
	}
	points := make([]float64, 0, len(deltas))
	for ts := range deltas {
		points = append(points, ts)
	}
	sort.Float64s(points)
	v := &SchedulerConcurrencyValues{}
	depth, segmentCount := 0, 0
	lastDepth, hasLogicalSegment := 0, false
	for i, ts := range points {
		if q.runCancel.tick() {
			return false
		}
		// Combine all simultaneous deltas BEFORE the next positive-width span;
		// an ending interval cannot overlap a starting one at one timestamp.
		depth += deltas[ts]
		if i+1 == len(points) || points[i+1] <= ts {
			continue
		}
		end := points[i+1]
		ms := (end - ts) * 1000
		if depth > v.PeakThreads {
			v.PeakThreads = depth
		}
		v.ThreadMs += float64(depth) * ms
		if depth > 0 {
			v.BusyMs += ms
		}
		if hasLogicalSegment && lastDepth == depth {
			if segmentCount == len(a.group.Segments) {
				a.group.Segments[len(a.group.Segments)-1].EndTs = end
			}
			continue
		}
		lastDepth, hasLogicalSegment = depth, true
		segmentCount++
		if len(a.group.Segments) < 16 {
			a.group.Segments = append(a.group.Segments, SchedulerConcurrencySegment{StartTs: ts, EndTs: end, Threads: depth})
		}
	}
	v.MeanThreads = v.ThreadMs / ((window.EndTs - window.StartTs) * 1000)
	if !schedulerConcurrencyFinite(v.ThreadMs) || !schedulerConcurrencyFinite(v.MeanThreads) || !schedulerConcurrencyFinite(v.BusyMs) {
		a.group.ValuesUnavailableReason = "non_finite_aggregate"
		return true
	}
	a.group.Values = v
	a.group.OmittedSegments = segmentCount - len(a.group.Segments)
	return true
}

func addSchedulerConcurrencyCoverage(dst *SchedulerConcurrencyCoverage, src SchedulerConcurrencyCoverage) {
	dst.CandidateIntervals += src.CandidateIntervals
	dst.AcceptedIntervals += src.AcceptedIntervals
	dst.OpenEndedIntervals += src.OpenEndedIntervals
	dst.SourceConflictIntervals += src.SourceConflictIntervals
	dst.UnresolvedSourceIntervals += src.UnresolvedSourceIntervals
	dst.IdentityExcludedIntervals += src.IdentityExcludedIntervals
	dst.InvalidIntervals += src.InvalidIntervals
}

func finishSchedulerConcurrencyCoverage(c *SchedulerConcurrencyCoverage) {
	if c.OpenEndedIntervals > 0 {
		c.Reasons = append(c.Reasons, "open_intervals_excluded")
	}
	if c.SourceConflictIntervals > 0 {
		c.Reasons = append(c.Reasons, "cross_source_members_excluded")
	}
	if c.UnresolvedSourceIntervals > 0 {
		c.Reasons = append(c.Reasons, "physical_source_unresolved")
	}
	if c.IdentityExcludedIntervals+c.IdentityExcludedTIDs > 0 {
		c.Reasons = append(c.Reasons, "ambiguous_thread_lifecycle_excluded")
	}
	if c.InvalidIntervals > 0 {
		c.Reasons = append(c.Reasons, "invalid_or_contradictory_endpoints_excluded")
	}
	c.Status = "available"
	if c.AcceptedIntervals == 0 {
		c.Status = "unavailable"
	} else if len(c.Reasons) > 0 {
		c.Status = "partial"
	}
	// This disclaimer is unconditional and does not relabel valid closed
	// intervals as unavailable. Status only describes this interval population.
	c.Reasons = append(c.Reasons, "capture_completeness_not_established")
}
