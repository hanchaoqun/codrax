package tracequery

import "fmt"

// A branch's measurement window is not a new source query. Preserve metadata
// already proved by that query, but only for the same complete physical wait.
// These keys/values are private to one chain cache and its immutable Index.
type schedulerIntervalMetadataKey struct {
	source                        string
	pid, tgid, startLine, endLine int
	actualStart, actualEnd        float64
	previousState                 string
	generationStart               threadLifecyclePoint
	hasGenerationStart            bool
}

type schedulerIntervalMetadataIndex struct {
	rows      map[schedulerIntervalMetadataKey]Interval
	duplicate map[schedulerIntervalMetadataKey]bool
}

func schedulerIntervalMetadataIdentity(idx *Index, interval Interval) (schedulerIntervalMetadataKey, bool) {
	if idx == nil || interval.Thread.PID <= 0 || interval.StartLine <= 0 || interval.EndLine <= 0 ||
		interval.ActualEndTs <= interval.ActualStartTs {
		return schedulerIntervalMetadataKey{}, false
	}
	// Reuse the existing native S/D family classifiers; runnable rows which
	// inherited a raw switch-out code may not become waiting instances.
	original := stateFromPrevState(interval.PrevStateRaw)
	if !blockIOIssuerWaitState(original) || (interval.State != original &&
		!(dominantStateIsDStateOrIOWait(string(original)) && dominantStateIsDStateOrIOWait(string(interval.State)))) {
		return schedulerIntervalMetadataKey{}, false
	}
	startSource, startOK := tracePairingSourceIdentity(idx, Event{Line: interval.StartLine})
	endSource, endOK := tracePairingSourceIdentity(idx, Event{Line: interval.EndLine})
	if !startOK || !endOK || startSource != endSource {
		return schedulerIntervalMetadataKey{}, false
	}
	generation := threadGenerationScopeAt(idx, interval.Thread.PID, interval.ActualStartTs, interval.StartLine)
	if !generation.contains(interval.ActualEndTs, interval.EndLine) {
		return schedulerIntervalMetadataKey{}, false
	}
	return schedulerIntervalMetadataKey{source: startSource, pid: interval.Thread.PID, tgid: interval.Thread.TGID,
		startLine: interval.StartLine, endLine: interval.EndLine,
		actualStart: interval.ActualStartTs, actualEnd: interval.ActualEndTs,
		previousState: interval.PrevStateRaw, generationStart: generation.start, hasGenerationStart: generation.hasStart}, true
}

func (c *chainQueryCache) projectOriginalIntervalMetadata(original, local Query, thread ThreadRef, current TimelineResult) TimelineResult {
	if c == nil || c.idx == nil || current.IntegrityFailure != "" ||
		original.TimeStart > local.TimeStart || original.TimeEnd < local.TimeEnd ||
		original.LineStart != local.LineStart || original.LineEnd != local.LineEnd {
		return current
	}
	owner := original
	owner.PID, owner.Thread, owner.ThreadInput = thread.PID, thread.Comm, ""
	owner.TargetScope = TargetScopeThread
	key := fmt.Sprintf("%d/%s/%.9f/%.9f/%d/%d/%s", thread.PID, thread.Comm,
		owner.TimeStart, owner.TimeEnd, owner.LineStart, owner.LineEnd, owner.TraceFlavor)
	if c.intervalMetadataBySourceQuery == nil {
		c.intervalMetadataBySourceQuery = make(map[string]*schedulerIntervalMetadataIndex)
	}
	donor, cached := c.intervalMetadataBySourceQuery[key]
	if !cached {
		base := c.timeline(owner, thread)
		if base.IntegrityFailure == "" && base.MeasurementDomain != nil {
			donor = &schedulerIntervalMetadataIndex{rows: make(map[schedulerIntervalMetadataKey]Interval), duplicate: make(map[schedulerIntervalMetadataKey]bool)}
			for _, row := range base.Intervals {
				if row.EndTs <= row.StartTs {
					continue
				}
				identity, known := schedulerIntervalMetadataIdentity(c.idx, row)
				if !known {
					continue
				}
				if _, exists := donor.rows[identity]; exists {
					donor.duplicate[identity] = true
				}
				donor.rows[identity] = row
			}
		}
		c.intervalMetadataBySourceQuery[key] = donor
	}
	if donor == nil {
		return current
	}
	// cache.timeline returns copies, but keep the ownership boundary explicit:
	// callers may retain current independently of this projected partition.
	projected := cloneTimelineResult(current)
	changed := false
	for i := range projected.Intervals {
		row := &projected.Intervals[i]
		if row.EndTs <= row.StartTs {
			continue
		}
		identity, known := schedulerIntervalMetadataIdentity(c.idx, *row)
		if !known || donor.duplicate[identity] {
			continue
		}
		source, exists := donor.rows[identity]
		if !exists || row.StartTs < source.StartTs || row.EndTs > source.EndTs {
			continue
		}
		// Empty/ambiguous source metadata is also authoritative about the lack
		// of a proof: narrowing must not hide its competing native candidates.
		row.State = source.State
		row.BlockedReasonCaller, row.BlockedReasonLine = source.BlockedReasonCaller, source.BlockedReasonLine
		row.BlockedReasonIOWait, row.BlockedReasonIOWaitKnown = source.BlockedReasonIOWait, source.BlockedReasonIOWaitKnown
		row.Summary = source.Summary
		row.Summary = clampedIntervalSummary(*row)
		changed = true
	}
	if changed {
		projected.MeasurementDomain = buildTimelineMeasurementDomain(local, projected)
	}
	return projected
}
