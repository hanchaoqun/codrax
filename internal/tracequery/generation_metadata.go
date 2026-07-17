package tracequery

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

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

// seedPerfGenerationHeadsFromFull derives the exact lifecycle state strictly
// before a bounded index's inclusive left edge. Only numeric TIDs that own an
// admitted perf candidate in the bounded index are retained, so a warm-cache
// window does not turn the full scheduler roster into hidden per-query state.
// The scope plan is the same authority used by perfThreadKey: verified V2
// children share one canonical replay, while unbound artifacts replay in
// independent physical-line domains.
func seedPerfGenerationHeadsFromFull(window, full *Index) {
	if window == nil || full == nil || !window.Windowed {
		return
	}
	candidates := perfGenerationCandidateTIDsByScope(window)
	if len(candidates) == 0 {
		return
	}
	if window.IndexLineStart <= 0 && window.IndexTimeStart > 0 && full.TimestampOrder != TraceTimestampOrderMonotonic && len(full.TraceArtifacts) <= 1 {
		window.perfGenerationHeads = map[string]*threadIncarnationTracker{}
		window.perfGenerationHeadInvalid = map[perfThreadScopeTID]string{}
		for scope, pids := range candidates {
			for tid := range pids {
				window.perfGenerationHeadInvalid[perfThreadScopeTID{scope: scope, tid: tid}] = perfGenerationInvalidNonmonotonic
			}
		}
		return
	}
	plan := buildPerfIdentityScopePlan(full)
	heads := perfGenerationCandidateHeadCoordinates(window, plan)
	byScope := make(map[string][]perfGenerationPrefixEvent, len(candidates))
	invalid := map[perfThreadScopeTID]string{}
	cappedScope := map[string]bool{}
	for ordinal := range full.Events {
		ev := full.Events[ordinal]
		scope, sourceIndex, ok := plan.scopeForEvent(full, ev)
		if !ok {
			continue
		}
		pids := candidates[scope]
		if len(pids) == 0 {
			continue
		}
		touched := perfGenerationEventCandidateTIDs(ev, pids)
		if touched.count > 0 && perfGenerationSourceOrderUnproven(full, sourceIndex) {
			for i := 0; i < touched.count; i++ {
				invalid[perfThreadScopeTID{scope: scope, tid: touched.values[i]}] = perfGenerationInvalidNonmonotonic
			}
		}
		if plan.sharedCapture {
			markPerfGenerationCrossSourceHeadTie(scope, sourceIndex, ev.Ts, touched, heads, invalid)
		}
		if !perfGenerationEventQualifiesPrefix(ev, window, scope, pids, heads, plan.sharedCapture) {
			continue
		}
		if cappedScope[scope] {
			continue
		}
		if len(byScope[scope]) >= perfGenerationPrefixEventCap {
			cappedScope[scope] = true
			byScope[scope] = nil
			for tid := range pids {
				invalid[perfThreadScopeTID{scope: scope, tid: tid}] = perfGenerationInvalidBudget
			}
			continue
		}
		item := compactPerfGenerationPrefixEvent(ev)
		item.sourceIndex = sourceIndex
		byScope[scope] = append(byScope[scope], item)
	}
	markWarmPerfGenerationPrefixFailures(window, full, plan, candidates, heads, invalid)
	window.perfGenerationHeads = make(map[string]*threadIncarnationTracker, len(candidates))
	window.perfGenerationHeadInvalid = invalid
	for scope, pids := range candidates {
		events := byScope[scope]
		if plan.sharedCapture {
			sort.SliceStable(events, func(i, j int) bool {
				if events[i].ts != events[j].ts {
					return events[i].ts < events[j].ts
				}
				return events[i].line < events[j].line
			})
			markSharedPerfGenerationSimultaneity(scope, events, pids, heads, invalid)
		} else {
			sort.SliceStable(events, func(i, j int) bool { return events[i].line < events[j].line })
			lastTs, lastSet := 0.0, false
			for _, item := range events {
				if lastSet && item.ts < lastTs {
					for tid := range pids {
						invalid[perfThreadScopeTID{scope: scope, tid: tid}] = perfGenerationInvalidNonmonotonic
					}
					break
				}
				lastTs, lastSet = item.ts, true
			}
		}
		tracker := newPerfGenerationTracker()
		for _, item := range events {
			ev := item.asEvent()
			touched := perfGenerationEventCandidateTIDs(ev, pids)
			for i := 0; i < touched.count; i++ {
				tid := touched.values[i]
				head, ok := heads[perfThreadScopeTID{scope: scope, tid: tid}]
				if ok && perfGenerationEventTouchesPID(ev, tid) && perfGenerationCoordinateBeforeHead(item.ts, item.line, head, plan.sharedCapture) {
					tracker.observeAll(ev, tid)
				}
			}
		}
		window.perfGenerationHeads[scope] = tracker.cloneForPIDs(pids)
	}
}

func perfGenerationSourceOrderUnproven(idx *Index, sourceIndex int) bool {
	if idx == nil {
		return true
	}
	if sourceIndex >= 0 && sourceIndex < len(idx.TraceArtifacts) {
		source := idx.TraceArtifacts[sourceIndex]
		return source.clockRegressions > 0 || source.timestampOrder == TraceTimestampOrderRegressed
	}
	return len(idx.TraceArtifacts) <= 1 && (idx.ClockRegressions > 0 || idx.TimestampOrder == TraceTimestampOrderRegressed)
}

func markPerfGenerationCrossSourceHeadTie(scope string, sourceIndex int, ts float64, touched perfGenerationCandidateTIDSet, heads map[perfThreadScopeTID]perfGenerationHeadCoordinate, invalid map[perfThreadScopeTID]string) {
	if math.IsNaN(ts) || math.IsInf(ts, 0) {
		return
	}
	for i := 0; i < touched.count; i++ {
		tid := touched.values[i]
		head, ok := heads[perfThreadScopeTID{scope: scope, tid: tid}]
		if ok && ts == head.ts && sourceIndex != head.sourceIndex {
			invalid[perfThreadScopeTID{scope: scope, tid: tid}] = perfGenerationInvalidNonmonotonic
		}
	}
}

const perfGenerationPrefixEventCap = 64 * 1024

const (
	perfGenerationInvalidClock        = "clock_unmappable"
	perfGenerationInvalidMalformed    = "malformed_scheduler_identity"
	perfGenerationInvalidBudget       = "prefix_budget_exceeded"
	perfGenerationInvalidNonmonotonic = "nonmonotonic_order"
)

type perfGenerationHeadCoordinate struct {
	ts          float64
	line        int
	sourceIndex int
}

func perfGenerationCandidateHeadCoordinates(idx *Index, plan perfIdentityScopePlan) map[perfThreadScopeTID]perfGenerationHeadCoordinate {
	heads := map[perfThreadScopeTID]perfGenerationHeadCoordinate{}
	if idx == nil {
		return heads
	}
	for ordinal := range idx.Events {
		tid, admitted := perfIdentityCandidateTIDFromEvent(idx.Events[ordinal])
		if !admitted {
			continue
		}
		scope, sourceIndex, ok := plan.scopeForEvent(idx, idx.Events[ordinal])
		if !ok {
			continue
		}
		ev := idx.Events[ordinal]
		key := perfThreadScopeTID{scope: scope, tid: tid}
		current, exists := heads[key]
		earlier := !exists
		if plan.sharedCapture {
			earlier = earlier || ev.Ts < current.ts || ev.Ts == current.ts && ev.Line < current.line
		} else {
			earlier = earlier || ev.Line < current.line
		}
		if earlier {
			heads[key] = perfGenerationHeadCoordinate{ts: ev.Ts, line: ev.Line, sourceIndex: sourceIndex}
		}
	}
	return heads
}

func perfGenerationEventQualifiesPrefix(ev Event, window *Index, scope string, pids map[int]bool, heads map[perfThreadScopeTID]perfGenerationHeadCoordinate, canonical bool) bool {
	if !perfGenerationExcludedByLowerBound(window, ev.Ts, ev.Line) {
		return false
	}
	touched := perfGenerationEventCandidateTIDs(ev, pids)
	for i := 0; i < touched.count; i++ {
		tid := touched.values[i]
		head, ok := heads[perfThreadScopeTID{scope: scope, tid: tid}]
		if ok && perfGenerationEventTouchesPID(ev, tid) && perfGenerationCoordinateBeforeHead(ev.Ts, ev.Line, head, canonical) {
			return true
		}
	}
	return false
}

func perfGenerationExcludedByLowerBound(idx *Index, ts float64, line int) bool {
	if idx == nil {
		return false
	}
	if idx.IndexLineStart > 0 && line > 0 && line < idx.IndexLineStart {
		return true
	}
	if idx.IndexTimeStart > 0 && !math.IsNaN(ts) && !math.IsInf(ts, 0) {
		if traceClockRoundTripWithinULPs(idx.IndexTimeStart, ts) {
			ts = idx.IndexTimeStart
		}
		return ts < idx.IndexTimeStart
	}
	return false
}

func perfGenerationCoordinateBeforeHead(ts float64, line int, head perfGenerationHeadCoordinate, canonical bool) bool {
	if !canonical {
		return line > 0 && line < head.line
	}
	if math.IsNaN(ts) || math.IsInf(ts, 0) {
		return false
	}
	return ts < head.ts || ts == head.ts && line > 0 && line < head.line
}

func markSharedPerfGenerationSimultaneity(scope string, events []perfGenerationPrefixEvent, pids map[int]bool, heads map[perfThreadScopeTID]perfGenerationHeadCoordinate, invalid map[perfThreadScopeTID]string) {
	type owner struct {
		ts  uint64
		tid int
	}
	owners := map[owner]int{}
	for _, item := range events {
		ev := item.asEvent()
		touched := perfGenerationEventCandidateTIDs(ev, pids)
		for i := 0; i < touched.count; i++ {
			tid := touched.values[i]
			key := owner{ts: math.Float64bits(item.ts), tid: tid}
			if sourceIndex, exists := owners[key]; exists && sourceIndex != item.sourceIndex {
				invalid[perfThreadScopeTID{scope: scope, tid: tid}] = perfGenerationInvalidNonmonotonic
			} else {
				owners[key] = item.sourceIndex
			}
			head, ok := heads[perfThreadScopeTID{scope: scope, tid: tid}]
			if ok && item.ts == head.ts && item.sourceIndex != head.sourceIndex {
				invalid[perfThreadScopeTID{scope: scope, tid: tid}] = perfGenerationInvalidNonmonotonic
			}
		}
	}
}

func markWarmPerfGenerationPrefixFailures(window, full *Index, plan perfIdentityScopePlan, candidates map[string]map[int]bool, heads map[perfThreadScopeTID]perfGenerationHeadCoordinate, invalid map[perfThreadScopeTID]string) {
	if full == nil {
		return
	}
	if full.schedulerRowIntegrityFailuresCapped {
		for scope, pids := range candidates {
			for tid := range pids {
				invalid[perfThreadScopeTID{scope: scope, tid: tid}] = perfGenerationInvalidMalformed
			}
		}
	}
	for i := range full.schedulerRowIntegrityFailures {
		failure := &full.schedulerRowIntegrityFailures[i]
		scope, ok := perfGenerationScopeForSchedulerFailure(full, plan, failure)
		if !ok {
			for candidateScope, pids := range candidates {
				markWarmPerfGenerationFailureForScope(window, failure, candidateScope, pids, heads, plan.sharedCapture, invalid)
			}
			continue
		}
		markWarmPerfGenerationFailureForScope(window, failure, scope, candidates[scope], heads, plan.sharedCapture, invalid)
	}
}

func markWarmPerfGenerationFailureForScope(window *Index, failure *schedulerRowIntegrityFailure, scope string, pids map[int]bool, heads map[perfThreadScopeTID]perfGenerationHeadCoordinate, canonical bool, invalid map[perfThreadScopeTID]string) {
	if failure == nil || len(pids) == 0 || !perfGenerationExcludedByLowerBound(window, failure.Ts, failure.Line) {
		return
	}
	if !failure.AffectsAllPIDs && len(failure.PIDs) > 0 {
		for _, tid := range failure.PIDs {
			if !pids[tid] {
				continue
			}
			head, ok := heads[perfThreadScopeTID{scope: scope, tid: tid}]
			if ok && perfGenerationCoordinateBeforeHead(failure.Ts, failure.Line, head, canonical) {
				invalid[perfThreadScopeTID{scope: scope, tid: tid}] = perfGenerationInvalidMalformed
			}
		}
		return
	}
	for tid := range pids {
		head, ok := heads[perfThreadScopeTID{scope: scope, tid: tid}]
		if ok && perfGenerationCoordinateBeforeHead(failure.Ts, failure.Line, head, canonical) {
			invalid[perfThreadScopeTID{scope: scope, tid: tid}] = perfGenerationInvalidMalformed
		}
	}
}

func positiveIntSliceContains(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// perfGenerationPrefixEvent is intentionally compact: Event is roughly 1KiB,
// while a lifecycle replay needs only these exact scheduler/header fields.
// The fixed cap therefore bounds a hostile hot-thread prefix without retaining
// a second full trace beside Index.Events.
type perfGenerationPrefixEvent struct {
	sourceIndex int
	line        int
	ts          float64
	typeID      EventType
	name        string
	pid         int
	prevPID     int
	nextPID     int
	prevState   string
	wakeePID    int
	migratePID  int
}

func compactPerfGenerationPrefixEvent(ev Event) perfGenerationPrefixEvent {
	item := perfGenerationPrefixEvent{
		line: ev.Line, ts: ev.Ts, typeID: ev.Type, name: ev.Name, pid: ev.PID,
		prevPID: ev.PrevPID, nextPID: ev.NextPID, prevState: ev.PrevState, wakeePID: ev.WakeePID,
	}
	if ev.ConstraintFields != nil && ev.ConstraintFields.Kind == "sched_migrate_task" {
		item.migratePID = ev.ConstraintFields.PID
	}
	return item
}

func (item perfGenerationPrefixEvent) asEvent() Event {
	ev := Event{
		Line: item.line, Ts: item.ts, Type: item.typeID, Name: item.name, PID: item.pid,
		PrevPID: item.prevPID, NextPID: item.nextPID, PrevState: item.prevState, WakeePID: item.wakeePID,
	}
	if item.migratePID > 0 {
		ev.ConstraintFields = &ConstraintFields{Kind: "sched_migrate_task", PID: item.migratePID}
	}
	return ev
}

// populateWindowPerfGenerationHeads is the cold-build twin of
// seedPerfGenerationHeadsFromFull. It runs only when the retained window has
// an admitted numeric perf candidate, scans every causal physical child from
// its held generation, and retains only rows that can affect those candidate
// TIDs. Verified V2 children are replayed once in canonical (ts,virtual-line)
// order; unbound artifacts retain independent physical-line histories.
func populateWindowPerfGenerationHeads(ctx context.Context, idx *Index) error {
	if idx == nil || !idx.Windowed {
		return nil
	}
	candidates := perfGenerationCandidateTIDsByScope(idx)
	if len(candidates) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	plan := buildPerfIdentityScopePlan(idx)
	candidateHeads := perfGenerationCandidateHeadCoordinates(idx, plan)
	if idx.IndexLineStart <= 0 && idx.IndexTimeStart > 0 && idx.TimestampOrder != TraceTimestampOrderMonotonic && len(idx.TraceArtifacts) <= 1 {
		idx.perfGenerationHeads = map[string]*threadIncarnationTracker{}
		idx.perfGenerationHeadInvalid = map[perfThreadScopeTID]string{}
		for scope, pids := range candidates {
			for tid := range pids {
				idx.perfGenerationHeadInvalid[perfThreadScopeTID{scope: scope, tid: tid}] = perfGenerationInvalidNonmonotonic
			}
		}
		return nil
	}
	sources := idx.TraceArtifacts
	if len(sources) == 0 {
		sources = []TraceArtifactSource{singleTraceArtifactSource(idx.Path, idx.Size, idx.ModTime.UnixNano(), idx.LineCount, len(idx.Events))}
	}
	byScope := make(map[string][]perfGenerationPrefixEvent, len(candidates))
	invalid := map[perfThreadScopeTID]string{}
	cappedScope := map[string]bool{}
	for sourceIndex := range sources {
		source := sources[sourceIndex]
		if !source.CausalCompatible {
			continue
		}
		scope, ok := perfGenerationScopeForSource(plan, sourceIndex)
		if !ok || len(candidates[scope]) == 0 {
			continue
		}
		events, sourceInvalid, err := scanPerfGenerationPrefixSource(ctx, idx, source, sourceIndex, scope, candidates[scope], candidateHeads, plan.sharedCapture)
		if err != nil {
			return err
		}
		if !cappedScope[scope] {
			if len(byScope[scope])+len(events) > perfGenerationPrefixEventCap {
				cappedScope[scope] = true
				byScope[scope] = nil
				for tid := range candidates[scope] {
					invalid[perfThreadScopeTID{scope: scope, tid: tid}] = perfGenerationInvalidBudget
				}
			} else {
				byScope[scope] = append(byScope[scope], events...)
			}
		}
		for tid, reason := range sourceInvalid {
			invalid[perfThreadScopeTID{scope: scope, tid: tid}] = reason
		}
	}

	heads := make(map[string]*threadIncarnationTracker, len(candidates))
	for scope, pids := range candidates {
		events := byScope[scope]
		if plan.sharedCapture {
			sort.SliceStable(events, func(i, j int) bool {
				left, right := events[i], events[j]
				if left.ts != right.ts {
					return left.ts < right.ts
				}
				return left.line < right.line
			})
			markSharedPerfGenerationSimultaneity(scope, events, pids, candidateHeads, invalid)
		} else {
			// A non-shared scope belongs to one physical source. Sorting by
			// virtual line preserves that source's exact physical order even if
			// its timestamp regressed.
			sort.SliceStable(events, func(i, j int) bool { return events[i].line < events[j].line })
		}
		tracker := newPerfGenerationTracker()
		for _, item := range events {
			ev := item.asEvent()
			touched := perfGenerationEventCandidateTIDs(ev, pids)
			for i := 0; i < touched.count; i++ {
				tid := touched.values[i]
				head, ok := candidateHeads[perfThreadScopeTID{scope: scope, tid: tid}]
				if ok && perfGenerationEventTouchesPID(ev, tid) && perfGenerationCoordinateBeforeHead(item.ts, item.line, head, plan.sharedCapture) {
					tracker.observeAll(ev, tid)
				}
			}
		}
		heads[scope] = tracker.cloneForPIDs(pids)
	}
	idx.perfGenerationHeads = heads
	idx.perfGenerationHeadInvalid = invalid
	return nil
}

func perfGenerationScopeForSource(plan perfIdentityScopePlan, sourceIndex int) (string, bool) {
	if plan.sharedScope != "" && !plan.unboundSources {
		return plan.sharedScope, true
	}
	scope, ok := plan.sourceScopes[sourceIndex]
	return scope, ok
}

func scanPerfGenerationPrefixSource(ctx context.Context, idx *Index, source TraceArtifactSource, sourceIndex int, scope string, pids map[int]bool, heads map[perfThreadScopeTID]perfGenerationHeadCoordinate, canonical bool) (events []perfGenerationPrefixEvent, invalid map[int]string, err error) {
	invalid = map[int]string{}
	f, openedIdentity, err := openTraceSourceRegularContext(ctx, canonicalTraceIndexPath(source.SourcePath))
	if err != nil {
		return nil, invalid, fmt.Errorf("build perf generation prefix: open source: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close perf generation prefix source: %w", closeErr))
		}
	}()
	if !schedulerHeadSourceIdentityMatches(source, openedIdentity) {
		return nil, invalid, fmt.Errorf("build perf generation prefix: source generation differs from parsed artifact ledger")
	}
	frozen, err := frozenTraceSectionAtCurrentOffset(f, openedIdentity)
	if err != nil {
		return nil, invalid, err
	}
	reader := bufio.NewReaderSize(frozen, 256*1024)
	intern := newStringInterner()
	scratch := &Index{}
	if idx.IndexTimeStart > 0 {
		if _, boundaryOK := source.toSourceTsChecked(idx.IndexTimeStart); !boundaryOK {
			for tid := range pids {
				invalid[tid] = perfGenerationInvalidClock
			}
			return nil, invalid, nil
		}
	}
	prefixCapped := false
	// A windowed child may report TimestampOrder=unknown simply because the
	// main parser stopped at line_end. This dedicated scan reads the held
	// physical child to EOF, so establish the missing source-local order proof
	// here instead of letting canonical cross-child sorting repair a clock
	// regression. Retain only the candidate TIDs actually touched by this
	// source; unrelated siblings stay usable.
	sourceOrderRegressed := perfGenerationSourceOrderUnproven(idx, sourceIndex)
	var lastSourceTs float64
	lastSourceTsSet := false
	sourceTouched := map[int]struct{}{}
	appendEvent := func(ev Event) {
		if prefixCapped {
			return
		}
		if len(events) >= perfGenerationPrefixEventCap {
			prefixCapped = true
			events = nil
			for tid := range pids {
				invalid[tid] = perfGenerationInvalidBudget
			}
			return
		}
		item := compactPerfGenerationPrefixEvent(ev)
		item.sourceIndex = sourceIndex
		events = append(events, item)
	}
	markSchedulerFailure := func(failure *schedulerRowIntegrityFailure, mapped float64, globalLine int) {
		relevant := func(tid int) bool {
			head, ok := heads[perfThreadScopeTID{scope: scope, tid: tid}]
			if !ok {
				return false
			}
			if math.IsNaN(mapped) || math.IsInf(mapped, 0) {
				// In a shared canonical capture, a malformed scheduler row with
				// no usable timestamp cannot be ordered against another child's
				// candidate at all. In a single physical source, line order is
				// sufficient and only a preceding row can govern the head.
				return canonical || globalLine > 0 && globalLine < head.line
			}
			return perfGenerationExcludedByLowerBound(idx, mapped, globalLine) && perfGenerationCoordinateBeforeHead(mapped, globalLine, head, canonical)
		}
		if failure == nil {
			for tid := range pids {
				if relevant(tid) {
					invalid[tid] = perfGenerationInvalidMalformed
				}
			}
			return
		}
		if failure.AffectsAllPIDs || len(failure.PIDs) == 0 {
			for tid := range pids {
				if relevant(tid) {
					invalid[tid] = perfGenerationInvalidMalformed
				}
			}
			return
		}
		for _, pid := range failure.PIDs {
			if pids[pid] && relevant(pid) {
				invalid[pid] = perfGenerationInvalidMalformed
			}
		}
	}
	for lineNo := 1; ; lineNo++ {
		if err := ctx.Err(); err != nil {
			return nil, invalid, err
		}
		line, readErr := readStreamScanPhysicalLine(reader, attachment.TracePhysicalLineMaxBytes)
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			var scan lineScan
			scan.reset(lineNo, trimmed)
			globalLine := lineNo + source.VirtualLineBase
			schedulerCandidate := schedulerHeadRawCandidate(trimmed) && !strings.EqualFold(source.Kind, "perftrace")
			match := scan.match()
			if len(match) < 7 {
				if schedulerCandidate {
					// Raw scheduler tokens outside the strict ftrace envelope are
					// integrity evidence, not ignorable text. With no usable header
					// or timestamp the row cannot identify a narrower TID set, so
					// the relevance predicate above fails closed for this scope.
					markSchedulerFailure(nil, math.NaN(), globalLine)
				}
				goto nextPrefixLine
			}
			if len(match) >= 7 {
				headerPID, headerOK := parseFtraceHeaderTID(match[2])
				rawTs, tsOK := scan.timestamp()
				if tsOK {
					if lastSourceTsSet && rawTs < lastSourceTs {
						sourceOrderRegressed = true
					}
					lastSourceTs, lastSourceTsSet = rawTs, true
				}
				headerRelevant := headerOK && pids[headerPID]
				if !tsOK && (headerRelevant || schedulerCandidate) {
					if headerRelevant {
						invalid[headerPID] = perfGenerationInvalidClock
					}
					if schedulerCandidate {
						failure := schedulerRowValidationFailureScan(&scan)
						if failure == nil {
							failure = schedulerRejectedRowFailureScan(&scan)
						}
						markSchedulerFailure(failure, math.NaN(), globalLine)
					}
					continue
				}
				mapped, mapOK := source.toCanonicalTsChecked(rawTs)
				ev, evOK := safeParseLineScan(&scan, intern, scratch)
				if !mapOK {
					if headerRelevant {
						invalid[headerPID] = perfGenerationInvalidClock
					}
					if evOK {
						touched := perfGenerationEventCandidateTIDs(ev, pids)
						for i := 0; i < touched.count; i++ {
							tid := touched.values[i]
							invalid[tid] = perfGenerationInvalidClock
						}
					}
					continue
				}
				if idx.IndexTimeStart > 0 && traceClockRoundTripWithinULPs(idx.IndexTimeStart, mapped) {
					mapped = idx.IndexTimeStart
				}
				if schedulerCandidate && !evOK {
					failure := schedulerRowValidationFailureScan(&scan)
					if failure == nil {
						failure = schedulerRejectedRowFailureScan(&scan)
					}
					markSchedulerFailure(failure, mapped, globalLine)
					continue
				}
				var proof Event
				proofOK := false
				if strings.EqualFold(source.Kind, "perftrace") {
					if tid, admitted := perfIdentityCandidateTIDFromEvent(ev); admitted && pids[tid] {
						proof = Event{Line: globalLine, Ts: mapped, PID: tid}
						proofOK = true
					}
				} else if schedulerCandidate && evOK {
					ev.Line, ev.Ts = globalLine, mapped
					proof, proofOK = ev, perfGenerationEventTouchesPIDSet(ev, pids)
				} else if headerRelevant {
					// A valid non-perf ftrace header is exact task existence
					// evidence even when its non-scheduler payload is unknown.
					proof = Event{Line: globalLine, Ts: mapped, PID: headerPID}
					proofOK = true
				}
				if proofOK {
					touched := perfGenerationEventCandidateTIDs(proof, pids)
					for i := 0; i < touched.count; i++ {
						tid := touched.values[i]
						sourceTouched[tid] = struct{}{}
						if sourceOrderRegressed {
							invalid[tid] = perfGenerationInvalidNonmonotonic
						}
					}
					if canonical {
						for i := 0; i < touched.count; i++ {
							tid := touched.values[i]
							head, ok := heads[perfThreadScopeTID{scope: scope, tid: tid}]
							if ok && proof.Ts == head.ts && sourceIndex != head.sourceIndex {
								invalid[tid] = perfGenerationInvalidNonmonotonic
							}
						}
					}
					if perfGenerationEventQualifiesPrefix(proof, idx, scope, pids, heads, canonical) {
						appendEvent(proof)
					}
				}
			}
		}
	nextPrefixLine:
		if prefixCapped {
			break
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return nil, invalid, traceReadErrorAfterIdentity(f, openedIdentity, "perf generation prefix physical read", readErr)
		}
	}
	if sourceOrderRegressed {
		// The regression may occur after an earlier candidate proof; poison the
		// complete touched set once the full physical scan has established it.
		for tid := range sourceTouched {
			invalid[tid] = perfGenerationInvalidNonmonotonic
		}
	}
	if err := validateTraceFileIdentityAfterRead(f, openedIdentity, "perf generation prefix scan"); err != nil {
		return nil, invalid, err
	}
	return events, invalid, nil
}

func perfGenerationCandidateTIDsByScope(idx *Index) map[string]map[int]bool {
	out := map[string]map[int]bool{}
	if idx == nil {
		return out
	}
	plan := buildPerfIdentityScopePlan(idx)
	for ordinal := range idx.Events {
		tid, admitted := perfIdentityCandidateTIDFromEvent(idx.Events[ordinal])
		if !admitted {
			continue
		}
		scope, _, ok := plan.scopeForEvent(idx, idx.Events[ordinal])
		if !ok {
			continue
		}
		set := out[scope]
		if set == nil {
			set = map[int]bool{}
			out[scope] = set
		}
		set[tid] = true
	}
	return out
}

func perfGenerationEventTouchesPIDSet(ev Event, pids map[int]bool) bool {
	return perfGenerationEventCandidateTIDs(ev, pids).count > 0
}

// perfGenerationEventCandidateTIDs is the O(event roles) inverse of scanning
// every candidate TID. Scheduler/perf rows carry only a handful of exact TID
// coordinates; intersect those with the candidate set instead of turning a
// 250k-thread profile into O(events*candidates) generation work.
type perfGenerationCandidateTIDSet struct {
	values [4]int
	count  int
}

func (s *perfGenerationCandidateTIDSet) add(pid int, candidates map[int]bool) {
	if s == nil || pid <= 0 || !candidates[pid] {
		return
	}
	for i := 0; i < s.count; i++ {
		if s.values[i] == pid {
			return
		}
	}
	if s.count < len(s.values) {
		s.values[s.count] = pid
		s.count++
	}
}

func perfGenerationEventCandidateTIDs(ev Event, pids map[int]bool) perfGenerationCandidateTIDSet {
	var out perfGenerationCandidateTIDSet
	if len(pids) == 0 {
		return out
	}
	if ev.Type == EventPerfSample {
		tid, admitted := perfIdentityCandidateTIDFromEvent(ev)
		if admitted {
			out.add(tid, pids)
		}
		return out
	}
	out.add(ev.PID, pids)
	switch ev.Type {
	case EventSchedSwitch:
		out.add(ev.PrevPID, pids)
		out.add(ev.NextPID, pids)
	case EventSchedWakeup, EventSchedWaking, EventSchedBlockedReason:
		out.add(ev.WakeePID, pids)
	case EventCPUConstraint:
		if ev.ConstraintFields != nil && ev.ConstraintFields.Kind == "sched_migrate_task" {
			out.add(ev.ConstraintFields.PID, pids)
		}
	}
	return out
}

func perfGenerationEventTouchesPID(ev Event, pid int) bool {
	if pid <= 0 {
		return false
	}
	if ev.Type == EventPerfSample {
		tid, admitted := perfIdentityCandidateTIDFromEvent(ev)
		return admitted && tid == pid
	}
	if ev.PID == pid {
		return true
	}
	switch ev.Type {
	case EventSchedSwitch:
		return ev.PrevPID == pid || ev.NextPID == pid
	case EventSchedWakeup, EventSchedWaking, EventSchedBlockedReason:
		return ev.WakeePID == pid
	case EventCPUConstraint:
		return ev.ConstraintFields != nil && ev.ConstraintFields.Kind == "sched_migrate_task" && ev.ConstraintFields.PID == pid
	default:
		return false
	}
}
