package tracequery

import "sort"

// threadGenerationScope is the exact lifecycle-bounded incarnation that owns
// a metadata lookup point. Numeric TIDs are reusable kernel handles, so
// priority, CPU and derived process metadata may be reused only inside this
// half-open physical interval. A boundary event belongs to the new
// incarnation; the next boundary does not.
type threadGenerationScope struct {
	start    threadLifecyclePoint
	end      threadLifecyclePoint
	hasStart bool
	hasEnd   bool
	known    bool
	lineMode bool
}

func lifecyclePointLess(ts float64, line int, point threadLifecyclePoint) bool {
	return ts < point.ts || (ts == point.ts && line < point.line)
}

func lifecyclePointAtOrAfter(ts float64, line int, point threadLifecyclePoint) bool {
	return !lifecyclePointLess(ts, line, point)
}

func (s threadGenerationScope) contains(ts float64, line int) bool {
	if !s.known {
		return false
	}
	if s.lineMode {
		if s.hasStart && line < s.start.line {
			return false
		}
		if s.hasEnd && line >= s.end.line {
			return false
		}
		return true
	}
	if s.hasStart && !lifecyclePointAtOrAfter(ts, line, s.start) {
		return false
	}
	if s.hasEnd && lifecyclePointAtOrAfter(ts, line, s.end) {
		return false
	}
	return true
}

// threadGenerationScopeAt resolves the incarnation containing (ts,line).
// A zero line means "after all physical rows at this timestamp", matching
// point-in-time consumers such as interval midpoints. If lifecycle proofs were
// truncated, no generation is guessed: metadata becomes unknown.
func threadGenerationScopeAt(idx *Index, pid int, ts float64, line int) threadGenerationScope {
	if idx == nil || pid <= 0 {
		return threadGenerationScope{}
	}
	if line <= 0 {
		line = int(^uint(0) >> 1)
	}
	boundaries, capped := threadGenerationBoundaries(idx, pid)
	if capped {
		return threadGenerationScope{}
	}
	scope := threadGenerationScope{known: true}
	for _, boundary := range boundaries {
		point := threadLifecyclePoint{ts: boundary.BoundaryTs, line: boundary.BoundaryLine}
		if lifecyclePointAtOrAfter(ts, line, point) {
			scope.start, scope.hasStart = point, true
			continue
		}
		scope.end, scope.hasEnd = point, true
		break
	}
	return scope
}

// threadGenerationScopeForQuery returns the sole incarnation selected by q.
// Queries spanning a lifecycle boundary are deliberately unknown even if a
// downstream caller forgot to consult the global identity fail-closed gate.
func threadGenerationScopeForQuery(idx *Index, pid int, q Query) threadGenerationScope {
	if idx == nil || pid <= 0 {
		return threadGenerationScope{}
	}
	boundaries, capped := threadGenerationBoundaries(idx, pid)
	if capped {
		return threadGenerationScope{}
	}
	if len(boundaries) == 0 {
		return threadGenerationScope{known: true}
	}
	for i := range boundaries {
		if incarnationBoundaryInsideQuery(&boundaries[i], q) {
			return threadGenerationScope{}
		}
	}
	if q.LineStart > 0 || q.LineEnd > 0 {
		line := q.LineStart
		if line <= 0 {
			line = q.LineEnd
		}
		if line <= 0 {
			return threadGenerationScope{}
		}
		// A task may die before the selected line range and a new occupant
		// appear inside it. The global identity gate correctly treats that as
		// new-only (the dead occupant has no selected row); choose the creation
		// boundary, not the empty dead gap at line_start, as metadata owner.
		for _, boundary := range boundaries {
			if boundary.PriorDead && q.LineStart > 0 && boundary.PriorDeadLine < q.LineStart &&
				boundary.BoundaryLine >= q.LineStart && (q.LineEnd <= 0 || boundary.BoundaryLine <= q.LineEnd) {
				line = boundary.BoundaryLine
			}
		}
		// Line-scoped queries use the exact physical coordinate. Timestamp is
		// intentionally ignored because line order is their governing domain.
		return threadGenerationScopeAtLine(idx, pid, line)
	}
	point := q.TimeStart
	if point <= 0 {
		point = q.TimeEnd
	}
	for _, boundary := range boundaries {
		if boundary.PriorDead && q.TimeStart > 0 && boundary.PriorDeadTs < q.TimeStart &&
			boundary.BoundaryTs >= q.TimeStart && (q.TimeEnd <= 0 || boundary.BoundaryTs <= q.TimeEnd) {
			point = boundary.BoundaryTs
		}
	}
	if point <= 0 && len(boundaries) > 0 {
		// An unbounded query selects every incarnation; there is no sole owner.
		return threadGenerationScope{}
	}
	return threadGenerationScopeAt(idx, pid, point, 0)
}

func threadGenerationScopeAtLine(idx *Index, pid, line int) threadGenerationScope {
	if idx == nil || pid <= 0 || line <= 0 {
		return threadGenerationScope{}
	}
	boundaries, capped := threadGenerationBoundaries(idx, pid)
	if capped {
		return threadGenerationScope{}
	}
	boundaries = append([]threadIncarnationConflict(nil), boundaries...)
	sort.SliceStable(boundaries, func(i, j int) bool {
		return boundaries[i].BoundaryLine < boundaries[j].BoundaryLine
	})
	scope := threadGenerationScope{known: true, lineMode: true}
	for _, boundary := range boundaries {
		point := threadLifecyclePoint{ts: boundary.BoundaryTs, line: boundary.BoundaryLine}
		if boundary.BoundaryLine <= line {
			scope.start, scope.hasStart = point, true
			continue
		}
		scope.end, scope.hasEnd = point, true
		break
	}
	return scope
}

func threadGenerationBoundaries(idx *Index, pid int) ([]threadIncarnationConflict, bool) {
	if idx == nil || pid <= 0 {
		return nil, false
	}
	all, capped := ensureThreadGenerationMetadata(idx)
	return all[pid], capped
}

func ensureThreadGenerationMetadata(idx *Index) (map[int][]threadIncarnationConflict, bool) {
	if idx == nil {
		return nil, false
	}
	idx.generationMetadataOnce.Do(func() {
		failures := append([]threadIncarnationConflict(nil), idx.threadIncarnationFailures...)
		capped := idx.threadIncarnationFailuresCapped
		scanned, scannedCapped := threadIncarnationConflictsFromEvents(idx.Events, Query{}, threadIncarnationFailureCap)
		failures, capped = mergeThreadIncarnationFailures(failures, capped, scanned, scannedCapped, threadIncarnationFailureCap)
		idx.generationMetadataBoundaries = map[int][]threadIncarnationConflict{}
		for _, failure := range failures {
			if failure.PID <= 0 || failure.BoundaryLine <= 0 {
				continue
			}
			idx.generationMetadataBoundaries[failure.PID] = append(idx.generationMetadataBoundaries[failure.PID], failure)
		}
		for generationPID := range idx.generationMetadataBoundaries {
			items := idx.generationMetadataBoundaries[generationPID]
			sort.SliceStable(items, func(i, j int) bool {
				if items[i].BoundaryTs != items[j].BoundaryTs {
					return items[i].BoundaryTs < items[j].BoundaryTs
				}
				return items[i].BoundaryLine < items[j].BoundaryLine
			})
			idx.generationMetadataBoundaries[generationPID] = items
		}
		idx.generationMetadataCapped = capped
	})
	return idx.generationMetadataBoundaries, idx.generationMetadataCapped
}
