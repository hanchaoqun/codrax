package tracequery

import (
	"math"
	"sort"
	"strconv"
)

// priorityEvidenceCaliber is the closed proof lattice for scheduler priority.
// Only exact-at-point and closed-range-stable verdicts may feed a hard
// priority relation.  Nearest is retained deliberately as an advisory value:
// physical proximity is useful display context, but is not point-in-time
// evidence.
type priorityEvidenceCaliber string

const (
	priorityCaliberUnknown           priorityEvidenceCaliber = "unknown"
	priorityCaliberExactAtPoint      priorityEvidenceCaliber = "exact_at_point"
	priorityCaliberClosedRangeStable priorityEvidenceCaliber = "closed_range_stable"
	priorityCaliberAdvisoryNearest   priorityEvidenceCaliber = "advisory_nearest"
)

// Compatibility names used by the query integration. Keep the caliber value
// itself as the single authority; these are aliases, not a second enum.
const (
	priorityEvidenceUnknown           = priorityCaliberUnknown
	priorityEvidenceExactAtPoint      = priorityCaliberExactAtPoint
	priorityEvidenceClosedRangeStable = priorityCaliberClosedRangeStable
	priorityEvidenceAdvisoryNearest   = priorityCaliberAdvisoryNearest
)

// priorityPointSide makes same-timestamp physical ordering explicit. A
// sched_switch prev_prio is authoritative immediately before the transition;
// next_prio is authoritative immediately after it. Wakee priority is exact on
// the wake row and is the opening value on its after side.
type priorityPointSide uint8

const (
	priorityPointAt priorityPointSide = iota
	priorityPointBefore
	priorityPointAfter
)

type priorityPhysicalPoint struct {
	Source string
	Ts     float64
	Line   int
}

type priorityPoint = priorityPhysicalPoint

type priorityPointVerdict struct {
	Priority int
	Caliber  priorityEvidenceCaliber
	Source   string
	Role     string
	Start    priorityPhysicalPoint
	End      priorityPhysicalPoint
}

func (v priorityPointVerdict) hardEvidence() bool {
	return v.Priority > 0 && priorityEvidenceCaliberIsHard(string(v.Caliber))
}

// priorityEvidenceCaliberIsHard is the package-local closed authority gate.
// Derivative consumers (aggregate/family folds included) may preserve a hard
// caliber only when an input already carries one of these exact tokens; shape,
// duration arithmetic and a non-zero "proven" scalar can never mint it.
func priorityEvidenceCaliberIsHard(caliber string) bool {
	return caliber == string(priorityCaliberExactAtPoint) || caliber == string(priorityCaliberClosedRangeStable)
}

type priorityPointRelationVerdict struct {
	Target   priorityPointVerdict
	Subject  priorityPointVerdict
	Relation string
}

func (v priorityPointRelationVerdict) hardEvidence() bool {
	return v.Relation != "" && v.Target.hardEvidence() && v.Subject.hardEvidence()
}

type priorityRelationSlice struct {
	Interval           Interval
	TargetPriority     int
	DependencyPriority int
	Relation           string
	Source             string
}

type priorityLowerRelationWindow struct {
	Source               string
	Start                priorityPhysicalPoint
	End                  priorityPhysicalPoint
	TargetPriority       int
	DependencyPriority   int
	DependencyGeneration threadGenerationScope
}

type priorityEndpointSideMask uint8

const (
	priorityEndpointAt priorityEndpointSideMask = 1 << iota
	priorityEndpointBefore
	priorityEndpointAfter
)

type priorityEndpoint struct {
	PID        int
	Priority   int
	Point      priorityPhysicalPoint
	Sides      priorityEndpointSideMask
	Role       string
	Generation threadGenerationScope
}

type priorityMutation struct {
	// PID=0 means the mutation row could not be bound to exactly one subject.
	// Source="" means even its physical source could not be resolved. Both are
	// conservative poison, never an inferred identity.
	PID    int
	Point  priorityPhysicalPoint
	Reason string
	// GenerationScoped is true only when the malformed/mutation row has an
	// exact physical coordinate and one proven TID incarnation. A false value
	// is an intentional PID/source wildcard, never a guessed generation.
	Generation       threadGenerationScope
	GenerationScoped bool
}

type priorityMutationScope struct {
	Source           string
	PID              int
	Generation       threadGenerationScope
	GenerationScoped bool
}

type priorityStableRange struct {
	PID        int
	Priority   int
	Source     string
	Start      priorityPhysicalPoint
	End        priorityPhysicalPoint
	Generation threadGenerationScope
}

// priorityPointAuthority is immutable after construction. It is deliberately
// independent from rank/query publication: every hard consumer must ask this
// one object for a point/range proof instead of re-scanning for a convenient
// nearby sched sample.
//
// A RelationScoped index retains every priority mutation for its proven TID
// closure plus every PID-0 global mutation and malformed scheduler witness.
// Closed ranges are therefore available only for relationScopeTIDs; scheduler
// rows may incidentally retain an unrelated peer endpoint, but that peer can
// receive exact/advisory verdicts only because its mutation universe was
// deliberately pruned.
type priorityPointAuthority struct {
	idx            *Index
	cancel         *runCancelState
	endpointsByPID map[int][]priorityEndpoint
	stableByPID    map[int][]priorityStableRange
	mutations      []priorityMutation
	// mutationsByScope is the immutable search index for range construction.
	// Without it, one PID with N endpoints and M priority mutations would
	// perform an O(N*M) full mutation rescan. Four exact scopes are queried:
	// source+generation+PID, source+PID wildcard, source-global, and fully
	// global.
	mutationsByScope map[priorityMutationScope][]priorityPhysicalPoint
	// rangeSourceComplete is the positive, physical-source-local timestamp
	// ordering proof used by closed stable ranges. A tracebundle's merged event
	// stream is canonically sorted, so the composite TimestampOrder cannot
	// substitute for each child's original order proof.
	rangeSourceComplete map[string]bool
	rangeComplete       bool
	buildComplete       bool
	sourcesByPID        map[int][]string
}

func newPriorityPointAuthority(idx *Index) *priorityPointAuthority {
	return newPriorityPointAuthorityWithCancel(idx, nil)
}

func incompletePriorityPointAuthority(idx *Index) *priorityPointAuthority {
	return &priorityPointAuthority{
		idx: idx, endpointsByPID: map[int][]priorityEndpoint{},
		stableByPID: map[int][]priorityStableRange{}, mutationsByScope: map[priorityMutationScope][]priorityPhysicalPoint{},
		rangeSourceComplete: map[string]bool{}, sourcesByPID: map[int][]string{},
	}
}

func newPriorityPointAuthorityWithCancel(idx *Index, cancel *runCancelState) *priorityPointAuthority {
	a := &priorityPointAuthority{
		idx:                 idx,
		cancel:              cancel,
		endpointsByPID:      map[int][]priorityEndpoint{},
		stableByPID:         map[int][]priorityStableRange{},
		mutationsByScope:    map[priorityMutationScope][]priorityPhysicalPoint{},
		rangeSourceComplete: map[string]bool{},
		sourcesByPID:        map[int][]string{},
	}
	if idx == nil || cancel.sample() {
		return a
	}
	// A closed-range absence claim requires a positive monotonic-order proof.
	// Unknown is not equivalent to monotonic: hand-built/partially parsed
	// indexes may simply lack the evidence needed to show that no mutation row
	// sits between two endpoints. Exact row-local priority remains valid.
	a.rangeComplete = !idx.RelationScoped || idx.relationScopePriorityComplete
	a.rangeSourceComplete = priorityRangeSourceCompleteness(idx, a.rangeComplete)
	raw := map[int][]priorityEndpoint{}
	for i := range idx.Events {
		if cancel.tick() {
			return incompletePriorityPointAuthority(idx)
		}
		ev := idx.Events[i]
		source, sourceKnown := prioritySourceForEvent(idx, ev)
		if ev.Type == EventPriorityMutation {
			pid := ev.WakeePID
			if pid < 0 {
				pid = 0
			}
			coordinateKnown := sourceKnown && ev.Line > 0 && finitePriorityTimestamp(ev.Ts)
			switch {
			case !sourceKnown:
				a.disablePriorityStableRanges("")
			case !coordinateKnown:
				a.disablePriorityStableRanges(source)
			default:
				mutation := priorityMutation{
					PID: pid, Point: priorityPhysicalPoint{Source: source, Ts: ev.Ts, Line: ev.Line}, Reason: ev.Name,
				}
				if pid > 0 {
					generation := threadGenerationScopeAt(idx, pid, ev.Ts, ev.Line)
					if generation.known {
						mutation.Generation = generation
						mutation.GenerationScoped = true
					} else {
						// A numeric PID without an incarnation proof cannot be
						// allowed to bridge a reuse boundary.
						a.disablePriorityStableRanges(source)
						break
					}
				}
				a.mutations = append(a.mutations, mutation)
			}
		}
		if !sourceKnown || ev.Line <= 0 || !finitePriorityTimestamp(ev.Ts) {
			continue
		}
		add := func(pid, priority int, sides priorityEndpointSideMask, role string, poisonNonPositive bool) {
			if pid <= 0 {
				return
			}
			// A sched_switch priority field is an explicit point observation.
			// For a real task, a non-positive value is therefore not "missing":
			// it invalidates priority authority at this physical point. Retain a
			// normalization sentinel so a second positive field for the same PID
			// on the same row cannot survive as an exact endpoint by iteration
			// order. Wake rows are different: eventWakeePriorityForHardUse returns
			// zero when the field is absent/untrusted, so that lane remains simple
			// absence rather than manufacturing a poison witness.
			if priority <= 0 && !poisonNonPositive {
				return
			}
			point := priorityPhysicalPoint{Source: source, Ts: ev.Ts, Line: ev.Line}
			raw[pid] = append(raw[pid], priorityEndpoint{
				PID: pid, Priority: priority, Point: point, Sides: sides, Role: role,
				Generation: threadGenerationScopeAt(idx, pid, ev.Ts, ev.Line),
			})
		}
		switch ev.Type {
		case EventSchedSwitch:
			add(ev.PrevPID, ev.PrevPrio, priorityEndpointAt|priorityEndpointBefore, "sched_switch_prev", true)
			add(ev.NextPID, ev.NextPrio, priorityEndpointAt|priorityEndpointAfter, "sched_switch_next", true)
		case EventSchedWakeup, EventSchedWaking:
			add(ev.WakeePID, eventWakeePriorityForHardUse(ev), priorityEndpointAt|priorityEndpointAfter, "sched_wakeup_wakee", false)
		}
	}
	if !a.ingestSchedulerPriorityPoisonsWithCancel(cancel) {
		return incompletePriorityPointAuthority(idx)
	}
	if cancel.sample() {
		return incompletePriorityPointAuthority(idx)
	}

	// Normalize same-row duplicate observations before forming ranges. Equal
	// observations merge side masks; conflicting values become a scoped
	// mutation poison rather than allowing iteration order to elect a winner.
	for pid, endpoints := range raw {
		if cancel.tick() {
			return incompletePriorityPointAuthority(idx)
		}
		sort.SliceStable(endpoints, func(i, j int) bool {
			if endpoints[i].Point.Source != endpoints[j].Point.Source {
				return endpoints[i].Point.Source < endpoints[j].Point.Source
			}
			if cmp := comparePriorityPhysicalPoint(endpoints[i].Point, endpoints[j].Point); cmp != 0 {
				return cmp < 0
			}
			if endpoints[i].Priority != endpoints[j].Priority {
				return endpoints[i].Priority < endpoints[j].Priority
			}
			return endpoints[i].Role < endpoints[j].Role
		})
		if cancel.sample() {
			return incompletePriorityPointAuthority(idx)
		}
		out := make([]priorityEndpoint, 0, len(endpoints))
		for first := 0; first < len(endpoints); {
			if cancel.tick() {
				return incompletePriorityPointAuthority(idx)
			}
			last := first + 1
			for last < len(endpoints) && samePriorityPhysicalPoint(endpoints[first].Point, endpoints[last].Point) {
				if cancel.tick() {
					return incompletePriorityPointAuthority(idx)
				}
				last++
			}
			candidate := endpoints[first]
			conflict := false
			generationConflict := false
			roles := map[string]bool{candidate.Role: true}
			for i := first + 1; i < last; i++ {
				if !samePriorityGeneration(endpoints[i].Generation, candidate.Generation) {
					generationConflict = true
				}
				if endpoints[i].Priority != candidate.Priority || generationConflict {
					conflict = true
					continue
				}
				candidate.Sides |= endpoints[i].Sides
				roles[endpoints[i].Role] = true
			}
			if conflict || candidate.Priority <= 0 {
				reason := "conflicting_exact_priority"
				if candidate.Priority <= 0 {
					reason = "nonpositive_sched_priority"
				}
				mutation := priorityMutation{PID: pid, Point: candidate.Point, Reason: reason}
				// Normal scheduler rows have a precise TID incarnation. Scope the
				// poison to that generation so a reused numeric TID remains
				// independent. If generation identity itself conflicts/is unknown,
				// the PID+source wildcard is intentionally more conservative.
				if !generationConflict && candidate.Generation.known {
					mutation.Generation = candidate.Generation
					mutation.GenerationScoped = true
				}
				a.mutations = append(a.mutations, mutation)
			} else {
				candidate.Role = joinedPriorityEndpointRoles(roles)
				out = append(out, candidate)
			}
			first = last
		}
		a.endpointsByPID[pid] = out
		a.sourcesByPID[pid] = priorityEndpointSources(out)
	}
	sort.SliceStable(a.mutations, func(i, j int) bool {
		if a.mutations[i].Point.Source != a.mutations[j].Point.Source {
			return a.mutations[i].Point.Source < a.mutations[j].Point.Source
		}
		if cmp := comparePriorityPhysicalPoint(a.mutations[i].Point, a.mutations[j].Point); cmp != 0 {
			return cmp < 0
		}
		if a.mutations[i].PID != a.mutations[j].PID {
			return a.mutations[i].PID < a.mutations[j].PID
		}
		return a.mutations[i].Reason < a.mutations[j].Reason
	})
	if cancel.sample() || !a.rebuildMutationIndexWithCancel(cancel) {
		return incompletePriorityPointAuthority(idx)
	}
	if a.rangeComplete {
		for pid, endpoints := range a.endpointsByPID {
			if cancel.tick() {
				return incompletePriorityPointAuthority(idx)
			}
			ranges, complete := a.buildStableRangesWithCancel(pid, endpoints, cancel)
			if !complete {
				return incompletePriorityPointAuthority(idx)
			}
			a.stableByPID[pid] = ranges
		}
	}
	if cancel.sample() {
		return incompletePriorityPointAuthority(idx)
	}
	a.buildComplete = true
	return a
}

// newPriorityPointAuthorityForQuery adds the existing scheduler-head
// checkpoint as a real prefix endpoint. The carried coordinate is always the
// original PriorityTs/PriorityLine; a synthetic boundary coordinate would
// falsely turn "known before the window" into "observed at the window head".
// The returned authority is a fresh immutable value; the base authority and
// scheduler-head snapshot are never mutated.
func newPriorityPointAuthorityForQuery(idx *Index, q Query) *priorityPointAuthority {
	return newPriorityPointAuthorityWithCancel(idx, q.runCancel).withQueryHead(q)
}

func (a *priorityPointAuthority) withQueryHead(q Query) *priorityPointAuthority {
	if a == nil || a.idx == nil || !a.buildComplete {
		return a
	}
	if q.runCancel.sample() {
		return incompletePriorityPointAuthority(a.idx)
	}
	head := schedulerHeadForQuery(a.idx, q)
	if q.runCancel.fired() {
		return incompletePriorityPointAuthority(a.idx)
	}
	if head == nil || !head.Complete {
		return a
	}
	clone := &priorityPointAuthority{
		idx:                 a.idx,
		cancel:              q.runCancel,
		endpointsByPID:      make(map[int][]priorityEndpoint, len(a.endpointsByPID)),
		stableByPID:         make(map[int][]priorityStableRange, len(a.stableByPID)),
		mutations:           append([]priorityMutation(nil), a.mutations...),
		mutationsByScope:    make(map[priorityMutationScope][]priorityPhysicalPoint, len(a.mutationsByScope)),
		rangeSourceComplete: clonePrioritySourceCompleteness(a.rangeSourceComplete),
		rangeComplete:       a.rangeComplete,
		buildComplete:       true,
		sourcesByPID:        make(map[int][]string, len(a.sourcesByPID)),
	}
	for pid, endpoints := range a.endpointsByPID {
		copied, complete := clonePriorityEndpointsWithCancel(endpoints, q.runCancel)
		if !complete {
			return incompletePriorityPointAuthority(a.idx)
		}
		clone.endpointsByPID[pid] = copied
	}
	for pid, state := range head.Threads {
		if q.runCancel.tick() {
			return incompletePriorityPointAuthority(a.idx)
		}
		if pid <= 0 || state.Priority <= 0 || state.PriorityPoisoned || state.PriorityLine <= 0 || !finitePriorityTimestamp(state.PriorityTs) {
			continue
		}
		source, ok := prioritySourceForEvent(a.idx, Event{Line: state.PriorityLine})
		if !ok {
			continue
		}
		point := priorityPhysicalPoint{Source: source, Ts: state.PriorityTs, Line: state.PriorityLine}
		duplicate := false
		conflict := false
		for _, endpoint := range clone.endpointsByPID[pid] {
			if !samePriorityPhysicalPoint(endpoint.Point, point) {
				continue
			}
			if endpoint.Priority == state.Priority {
				duplicate = true
			} else {
				conflict = true
			}
		}
		if conflict {
			// A carried checkpoint is only another exact observation at its
			// original physical coordinate. It cannot overwrite a conflicting
			// in-trace endpoint, and iteration order must never elect one of the
			// two values. Remove that coordinate and poison it exactly as the
			// base-ledger normalization does.
			kept := clone.endpointsByPID[pid][:0]
			for _, endpoint := range clone.endpointsByPID[pid] {
				if !samePriorityPhysicalPoint(endpoint.Point, point) {
					kept = append(kept, endpoint)
				}
			}
			clone.endpointsByPID[pid] = kept
			clone.mutations = append(clone.mutations, priorityMutation{
				PID: pid, Point: point, Reason: "conflicting_head_priority",
			})
			continue
		}
		if duplicate {
			continue
		}
		clone.endpointsByPID[pid] = append(clone.endpointsByPID[pid], priorityEndpoint{
			PID: pid, Priority: state.Priority, Point: point,
			Sides: priorityEndpointAt | priorityEndpointAfter, Role: "scheduler_head_priority_carry",
			Generation: threadGenerationScopeAt(a.idx, pid, state.PriorityTs, state.PriorityLine),
		})
	}
	sort.SliceStable(clone.mutations, func(i, j int) bool {
		if clone.mutations[i].Point.Source != clone.mutations[j].Point.Source {
			return clone.mutations[i].Point.Source < clone.mutations[j].Point.Source
		}
		if cmp := comparePriorityPhysicalPoint(clone.mutations[i].Point, clone.mutations[j].Point); cmp != 0 {
			return cmp < 0
		}
		if clone.mutations[i].PID != clone.mutations[j].PID {
			return clone.mutations[i].PID < clone.mutations[j].PID
		}
		return clone.mutations[i].Reason < clone.mutations[j].Reason
	})
	if q.runCancel.sample() || !clone.rebuildMutationIndexWithCancel(q.runCancel) {
		return incompletePriorityPointAuthority(a.idx)
	}
	for pid, endpoints := range clone.endpointsByPID {
		if q.runCancel.tick() {
			return incompletePriorityPointAuthority(a.idx)
		}
		sort.SliceStable(endpoints, func(i, j int) bool {
			if endpoints[i].Point.Source != endpoints[j].Point.Source {
				return endpoints[i].Point.Source < endpoints[j].Point.Source
			}
			if cmp := comparePriorityPhysicalPoint(endpoints[i].Point, endpoints[j].Point); cmp != 0 {
				return cmp < 0
			}
			if endpoints[i].Priority != endpoints[j].Priority {
				return endpoints[i].Priority < endpoints[j].Priority
			}
			return endpoints[i].Role < endpoints[j].Role
		})
		clone.endpointsByPID[pid] = endpoints
		clone.sourcesByPID[pid] = priorityEndpointSources(endpoints)
		if clone.rangeComplete {
			ranges, complete := clone.buildStableRangesWithCancel(pid, endpoints, q.runCancel)
			if !complete {
				return incompletePriorityPointAuthority(a.idx)
			}
			clone.stableByPID[pid] = ranges
		}
	}
	if q.runCancel.sample() {
		return incompletePriorityPointAuthority(a.idx)
	}
	return clone
}

func prioritySourceForEvent(idx *Index, ev Event) (string, bool) {
	if idx == nil {
		return "", false
	}
	if len(idx.TraceArtifacts) > 0 {
		i, ok := resolveTraceArtifactSourceIndexForLine(idx.TraceArtifacts, ev.Line)
		if !ok {
			return "", false
		}
		return "artifact:" + strconv.Itoa(i), true
	}
	// Path-less hand-built indexes are a supported compatibility universe. A
	// single source sentinel keeps their tests useful without confusing them
	// with production artifact identities.
	return "compat:index", true
}

func priorityRangeSourceCompleteness(idx *Index, rangeComplete bool) map[string]bool {
	complete := make(map[string]bool)
	if idx == nil || !rangeComplete {
		return complete
	}
	if len(idx.TraceArtifacts) == 0 {
		complete["compat:index"] = idx.TimestampOrder == TraceTimestampOrderMonotonic && idx.ClockRegressions == 0
		return complete
	}
	for i := range idx.TraceArtifacts {
		source := idx.TraceArtifacts[i]
		complete["artifact:"+strconv.Itoa(i)] = source.CausalCompatible &&
			source.timestampOrder == TraceTimestampOrderMonotonic && source.clockRegressions == 0
	}
	return complete
}

func clonePrioritySourceCompleteness(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for source, complete := range in {
		out[source] = complete
	}
	return out
}

func (a *priorityPointAuthority) disablePriorityStableRanges(source string) {
	if a == nil {
		return
	}
	if source == "" {
		for candidate := range a.rangeSourceComplete {
			a.rangeSourceComplete[candidate] = false
		}
		return
	}
	a.rangeSourceComplete[source] = false
}

func schedulerFailureCarriesPriority(eventName string) bool {
	switch eventName {
	case "sched_switch", "sched_wakeup", "sched_wakeup_new", "sched_waking", "sched_pi_setprio", "binder_set_priority":
		return true
	default:
		return false
	}
}

func uniqueSchedulerFailurePID(failure schedulerRowIntegrityFailure) (int, bool) {
	if failure.AffectsAllPIDs {
		return 0, false
	}
	pid := 0
	for _, candidate := range failure.PIDs {
		if candidate <= 0 {
			continue
		}
		if pid == 0 {
			pid = candidate
			continue
		}
		if candidate != pid {
			return 0, false
		}
	}
	return pid, pid > 0
}

func prioritySourceForSchedulerFailure(idx *Index, failure schedulerRowIntegrityFailure) (string, bool) {
	if idx == nil {
		return "", false
	}
	lineSource, lineKnown := "", false
	if failure.Line > 0 {
		lineSource, lineKnown = prioritySourceForEvent(idx, Event{Line: failure.Line})
	}
	pathSource, pathKnown := "", false
	if failure.SourcePath != "" && len(idx.TraceArtifacts) > 0 {
		for i := range idx.TraceArtifacts {
			if idx.TraceArtifacts[i].SourcePath != failure.SourcePath {
				continue
			}
			if pathKnown {
				return "", false
			}
			pathSource, pathKnown = "artifact:"+strconv.Itoa(i), true
		}
		// An explicit physical-source claim is a precise provenance input. If
		// it does not bind to exactly one bundle child, a coincidentally valid
		// virtual line must not silently override the contradiction: doing so
		// could preserve hard authority in the actual malformed child.
		if !pathKnown {
			return "", false
		}
	}
	if lineKnown && pathKnown && lineSource != pathSource {
		return "", false
	}
	if lineKnown {
		return lineSource, true
	}
	if pathKnown {
		return pathSource, true
	}
	if len(idx.TraceArtifacts) == 0 {
		return "compat:index", true
	}
	return "", false
}

func prioritySchedulerFailureCoordinateKnown(idx *Index, failure schedulerRowIntegrityFailure, source string) bool {
	if idx == nil || source == "" || failure.Line <= 0 || !finitePriorityTimestamp(failure.Ts) {
		return false
	}
	lineSource, ok := prioritySourceForEvent(idx, Event{Line: failure.Line})
	return ok && lineSource == source
}

func (a *priorityPointAuthority) ingestSchedulerPriorityPoisonsWithCancel(cancel *runCancelState) bool {
	if a == nil || a.idx == nil {
		return true
	}
	for i := range a.idx.schedulerRowIntegrityFailures {
		if cancel.tick() {
			return false
		}
		failure := a.idx.schedulerRowIntegrityFailures[i]
		if !schedulerFailureCarriesPriority(failure.EventName) {
			continue
		}
		source, sourceKnown := prioritySourceForSchedulerFailure(a.idx, failure)
		coordinateKnown := sourceKnown && prioritySchedulerFailureCoordinateKnown(a.idx, failure, source)
		pid, uniquePID := uniqueSchedulerFailurePID(failure)
		switch {
		case sourceKnown && coordinateKnown && uniquePID:
			generation := threadGenerationScopeAt(a.idx, pid, failure.Ts, failure.Line)
			if !generation.known {
				a.disablePriorityStableRanges(source)
				continue
			}
			a.mutations = append(a.mutations, priorityMutation{
				PID: pid,
				Point: priorityPhysicalPoint{
					Source: source, Ts: failure.Ts, Line: failure.Line,
				},
				Reason:     "malformed_" + failure.EventName,
				Generation: generation, GenerationScoped: true,
			})
		case sourceKnown && coordinateKnown:
			// The row belongs to one physical source and point but not one
			// exact subject. Poison every PID range crossing that point only.
			a.mutations = append(a.mutations, priorityMutation{
				Point:  priorityPhysicalPoint{Source: source, Ts: failure.Ts, Line: failure.Line},
				Reason: "malformed_" + failure.EventName,
			})
		case sourceKnown:
			// Without a physical coordinate the row cannot be placed between
			// endpoints, so the smallest safe absence claim is source-global.
			a.disablePriorityStableRanges(source)
		default:
			a.disablePriorityStableRanges("")
		}
	}
	for _, overflow := range []struct {
		capped  bool
		global  bool
		sources []string
	}{
		{a.idx.schedulerRowIntegrityFailuresCapped, a.idx.schedulerRowIntegrityOverflowGlobal, a.idx.schedulerRowIntegrityOverflowSources},
		{a.idx.priorityMutationIntegrityFailuresCapped, a.idx.priorityMutationIntegrityOverflowGlobal, a.idx.priorityMutationIntegrityOverflowSources},
	} {
		if !overflow.capped {
			continue
		}
		if overflow.global || len(overflow.sources) == 0 {
			a.disablePriorityStableRanges("")
			continue
		}
		for _, sourcePath := range overflow.sources {
			if cancel.tick() {
				return false
			}
			source, ok := prioritySourceForSchedulerFailure(a.idx, schedulerRowIntegrityFailure{SourcePath: sourcePath})
			if !ok {
				a.disablePriorityStableRanges("")
				break
			}
			a.disablePriorityStableRanges(source)
		}
	}
	return true
}

func finitePriorityTimestamp(ts float64) bool {
	return !math.IsNaN(ts) && !math.IsInf(ts, 0)
}

func comparePriorityPhysicalPoint(a, b priorityPhysicalPoint) int {
	if a.Ts < b.Ts {
		return -1
	}
	if a.Ts > b.Ts {
		return 1
	}
	if a.Line < b.Line {
		return -1
	}
	if a.Line > b.Line {
		return 1
	}
	return 0
}

func samePriorityPhysicalPoint(a, b priorityPhysicalPoint) bool {
	return a.Source == b.Source && comparePriorityPhysicalPoint(a, b) == 0
}

func samePriorityGeneration(a, b threadGenerationScope) bool {
	if !a.known || !b.known || a.lineMode != b.lineMode || a.hasStart != b.hasStart || a.hasEnd != b.hasEnd {
		return false
	}
	if a.hasStart && (a.start.ts != b.start.ts || a.start.line != b.start.line) {
		return false
	}
	if a.hasEnd && (a.end.ts != b.end.ts || a.end.line != b.end.line) {
		return false
	}
	return true
}

func joinedPriorityEndpointRoles(roles map[string]bool) string {
	values := make([]string, 0, len(roles))
	for role := range roles {
		values = append(values, role)
	}
	sort.Strings(values)
	if len(values) == 0 {
		return ""
	}
	out := values[0]
	for _, value := range values[1:] {
		out += "+" + value
	}
	return out
}

func priorityEndpointSources(endpoints []priorityEndpoint) []string {
	var sources []string
	for _, endpoint := range endpoints {
		if len(sources) == 0 || sources[len(sources)-1] != endpoint.Point.Source {
			sources = append(sources, endpoint.Point.Source)
		}
	}
	return sources
}

func (a *priorityPointAuthority) buildStableRangesWithCancel(pid int, endpoints []priorityEndpoint, cancel *runCancelState) ([]priorityStableRange, bool) {
	if a == nil || len(endpoints) < 2 {
		return nil, true
	}
	if a.idx != nil && a.idx.RelationScoped && !a.idx.relationScopeTIDs[pid] {
		return nil, true
	}
	var ranges []priorityStableRange
	for i := 0; i+1 < len(endpoints); i++ {
		if cancel.tick() {
			return nil, false
		}
		left, right := endpoints[i], endpoints[i+1]
		if !a.rangeSourceComplete[left.Point.Source] || left.Point.Source != right.Point.Source || comparePriorityPhysicalPoint(left.Point, right.Point) >= 0 ||
			left.Priority != right.Priority || !samePriorityGeneration(left.Generation, right.Generation) ||
			a.mutationBetween(pid, left.Point.Source, left.Generation, left.Point, right.Point) {
			continue
		}
		candidate := priorityStableRange{
			PID: pid, Priority: left.Priority, Source: left.Point.Source,
			Start: left.Point, End: right.Point, Generation: left.Generation,
		}
		if len(ranges) > 0 {
			last := &ranges[len(ranges)-1]
			if last.PID == candidate.PID && last.Priority == candidate.Priority && last.Source == candidate.Source &&
				samePriorityGeneration(last.Generation, candidate.Generation) && samePriorityPhysicalPoint(last.End, candidate.Start) {
				last.End = candidate.End
				continue
			}
		}
		ranges = append(ranges, candidate)
	}
	return ranges, true
}

func (a *priorityPointAuthority) rebuildMutationIndexWithCancel(cancel *runCancelState) bool {
	if a == nil {
		return false
	}
	a.mutationsByScope = make(map[priorityMutationScope][]priorityPhysicalPoint)
	for _, mutation := range a.mutations {
		if cancel.tick() {
			return false
		}
		scope := priorityMutationScope{
			Source: mutation.Point.Source, PID: mutation.PID,
			Generation: mutation.Generation, GenerationScoped: mutation.GenerationScoped,
		}
		a.mutationsByScope[scope] = append(a.mutationsByScope[scope], mutation.Point)
	}
	return true
}

func clonePriorityEndpointsWithCancel(endpoints []priorityEndpoint, cancel *runCancelState) ([]priorityEndpoint, bool) {
	if len(endpoints) == 0 {
		return nil, true
	}
	copyOf := make([]priorityEndpoint, len(endpoints))
	for i := range endpoints {
		if cancel.tick() {
			return nil, false
		}
		copyOf[i] = endpoints[i]
	}
	return copyOf, true
}

func (a *priorityPointAuthority) mutationBetween(pid int, source string, generation threadGenerationScope, start, end priorityPhysicalPoint) bool {
	if a == nil {
		return true
	}
	scopes := [...]priorityMutationScope{
		{Source: source, PID: pid, Generation: generation, GenerationScoped: true},
		{Source: source, PID: pid},
		{Source: source, PID: 0},
		{Source: "", PID: 0},
	}
	for _, scope := range scopes {
		points := a.mutationsByScope[scope]
		i := sort.Search(len(points), func(i int) bool {
			return comparePriorityPhysicalPoint(points[i], start) > 0
		})
		if i < len(points) && comparePriorityPhysicalPoint(points[i], end) < 0 {
			return true
		}
	}
	return false
}

func prioritySideMask(side priorityPointSide) priorityEndpointSideMask {
	switch side {
	case priorityPointBefore:
		return priorityEndpointBefore
	case priorityPointAfter:
		return priorityEndpointAfter
	default:
		return priorityEndpointAt
	}
}

func (a *priorityPointAuthority) pointForEvent(ev Event) (priorityPhysicalPoint, bool) {
	if a == nil || !a.buildComplete || a.idx == nil {
		return priorityPhysicalPoint{}, false
	}
	source, ok := prioritySourceForEvent(a.idx, ev)
	if !ok || ev.Line <= 0 || !finitePriorityTimestamp(ev.Ts) {
		return priorityPhysicalPoint{}, false
	}
	return priorityPhysicalPoint{Source: source, Ts: ev.Ts, Line: ev.Line}, true
}

// pointVerdict is a compatibility adapter for older package-local callers:
// subject may be a ThreadRef or positive PID and an optional side selects the
// physical transition face. New hard consumers should call pointVerdictAt so
// subject/side remain compile-time typed. A missing physical line may return
// nearest display context, but can never return hard evidence.
func (a *priorityPointAuthority) pointVerdict(subject any, point priorityPoint, sides ...priorityPointSide) priorityPointVerdict {
	pid := 0
	switch value := subject.(type) {
	case ThreadRef:
		pid = value.PID
	case int:
		pid = value
	}
	if a == nil || !a.buildComplete || a.idx == nil || pid <= 0 || !finitePriorityTimestamp(point.Ts) {
		return priorityPointVerdict{Caliber: priorityCaliberUnknown}
	}
	if point.Source == "" {
		sources := a.sourcesByPID[pid]
		if len(sources) != 1 {
			return priorityPointVerdict{Caliber: priorityCaliberUnknown}
		}
		point.Source = sources[0]
	}
	if point.Line <= 0 {
		scope := threadGenerationScopeAt(a.idx, pid, point.Ts, 0)
		if !scope.known {
			return priorityPointVerdict{Caliber: priorityCaliberUnknown}
		}
		point.Line = int(^uint(0) >> 1)
		return a.advisoryNearest(pid, point, scope)
	}
	side := priorityPointAt
	if len(sides) > 0 {
		side = sides[0]
	}
	return a.pointVerdictAt(pid, point, side)
}

func (a *priorityPointAuthority) pointVerdictAt(pid int, point priorityPhysicalPoint, side priorityPointSide) priorityPointVerdict {
	if a == nil || a.cancel.sample() || !a.buildComplete || a.idx == nil || pid <= 0 || point.Source == "" || point.Line <= 0 || !finitePriorityTimestamp(point.Ts) {
		return priorityPointVerdict{Caliber: priorityCaliberUnknown}
	}
	scope := threadGenerationScopeAt(a.idx, pid, point.Ts, point.Line)
	if !scope.known {
		return priorityPointVerdict{Caliber: priorityCaliberUnknown}
	}
	mask := prioritySideMask(side)
	if endpoint, ok := priorityEndpointAtPoint(a.endpointsByPID[pid], point); ok &&
		endpoint.Sides&mask != 0 && samePriorityGeneration(endpoint.Generation, scope) {
		return priorityPointVerdict{
			Priority: endpoint.Priority, Caliber: priorityCaliberExactAtPoint,
			Source: endpoint.Point.Source, Role: endpoint.Role, Start: endpoint.Point, End: endpoint.Point,
		}
	}
	if stable, ok := priorityStableRangeAtPoint(a.stableByPID[pid], point, side, scope); ok {
		return priorityPointVerdict{
			Priority: stable.Priority, Caliber: priorityCaliberClosedRangeStable,
			Source: stable.Source, Role: "equal_bracketing_endpoints", Start: stable.Start, End: stable.End,
		}
	}
	return a.advisoryNearest(pid, point, scope)
}

func priorityEndpointSourceBounds(endpoints []priorityEndpoint, source string) (int, int) {
	first := sort.Search(len(endpoints), func(i int) bool {
		return endpoints[i].Point.Source >= source
	})
	last := first + sort.Search(len(endpoints)-first, func(i int) bool {
		return endpoints[first+i].Point.Source > source
	})
	return first, last
}

func priorityEndpointAtPoint(endpoints []priorityEndpoint, point priorityPhysicalPoint) (priorityEndpoint, bool) {
	first, last := priorityEndpointSourceBounds(endpoints, point.Source)
	if first == last {
		return priorityEndpoint{}, false
	}
	i := first + sort.Search(last-first, func(i int) bool {
		return comparePriorityPhysicalPoint(endpoints[first+i].Point, point) >= 0
	})
	if i >= last || !samePriorityPhysicalPoint(endpoints[i].Point, point) {
		return priorityEndpoint{}, false
	}
	return endpoints[i], true
}

func priorityStableRangeAtPoint(ranges []priorityStableRange, point priorityPhysicalPoint, side priorityPointSide, scope threadGenerationScope) (priorityStableRange, bool) {
	// Ranges are immutable and sorted by source/start. Search the first range
	// whose start lies strictly after the query, then validate its predecessor.
	i := sort.Search(len(ranges), func(i int) bool {
		return ranges[i].Source > point.Source ||
			(ranges[i].Source == point.Source && comparePriorityPhysicalPoint(ranges[i].Start, point) > 0)
	})
	if i == 0 {
		return priorityStableRange{}, false
	}
	candidate := ranges[i-1]
	if candidate.Source != point.Source || !samePriorityGeneration(candidate.Generation, scope) || !priorityRangeContainsPoint(candidate, point, side) {
		return priorityStableRange{}, false
	}
	return candidate, true
}

func priorityRangeContainsPoint(r priorityStableRange, point priorityPhysicalPoint, side priorityPointSide) bool {
	startCmp := comparePriorityPhysicalPoint(point, r.Start)
	endCmp := comparePriorityPhysicalPoint(point, r.End)
	switch side {
	case priorityPointBefore:
		return startCmp > 0 && endCmp <= 0
	case priorityPointAfter:
		return startCmp >= 0 && endCmp < 0
	default:
		return startCmp >= 0 && endCmp <= 0
	}
}

func (a *priorityPointAuthority) advisoryNearest(pid int, point priorityPhysicalPoint, scope threadGenerationScope) priorityPointVerdict {
	if a == nil || a.cancel.sample() {
		return priorityPointVerdict{Caliber: priorityCaliberUnknown}
	}
	endpoints := a.endpointsByPID[pid]
	first, last := priorityEndpointSourceBounds(endpoints, point.Source)
	if first == last {
		return priorityPointVerdict{Caliber: priorityCaliberUnknown}
	}
	position := first + sort.Search(last-first, func(i int) bool {
		return comparePriorityPhysicalPoint(endpoints[first+i].Point, point) >= 0
	})
	best := -1
	bestDistance := math.Inf(1)
	bestLineDistance := int(^uint(0) >> 1)
	for _, i := range [...]int{position - 1, position} {
		if i < first || i >= last {
			continue
		}
		endpoint := endpoints[i]
		if !samePriorityGeneration(endpoint.Generation, scope) {
			continue
		}
		distance := math.Abs(endpoint.Point.Ts - point.Ts)
		lineDistance := endpoint.Point.Line - point.Line
		if lineDistance < 0 {
			lineDistance = -lineDistance
		}
		if best < 0 || distance < bestDistance || (distance == bestDistance && lineDistance < bestLineDistance) ||
			(distance == bestDistance && lineDistance == bestLineDistance && comparePriorityPhysicalPoint(endpoint.Point, endpoints[best].Point) < 0) {
			best, bestDistance, bestLineDistance = i, distance, lineDistance
		}
	}
	if best < 0 {
		return priorityPointVerdict{Caliber: priorityCaliberUnknown}
	}
	endpoint := endpoints[best]
	return priorityPointVerdict{
		Priority: endpoint.Priority, Caliber: priorityCaliberAdvisoryNearest,
		Source: endpoint.Point.Source, Role: endpoint.Role, Start: endpoint.Point, End: endpoint.Point,
	}
}

func (a *priorityPointAuthority) rangeVerdict(pid int, start, end priorityPhysicalPoint) priorityPointVerdict {
	if a == nil || a.cancel.sample() || !a.buildComplete || a.idx == nil || pid <= 0 || start.Source == "" || start.Source != end.Source ||
		start.Line <= 0 || end.Line <= 0 || comparePriorityPhysicalPoint(start, end) > 0 {
		return priorityPointVerdict{Caliber: priorityCaliberUnknown}
	}
	if samePriorityPhysicalPoint(start, end) {
		return a.pointVerdictAt(pid, start, priorityPointAt)
	}
	startScope := threadGenerationScopeAt(a.idx, pid, start.Ts, start.Line)
	endScope := threadGenerationScopeAt(a.idx, pid, end.Ts, end.Line)
	if !samePriorityGeneration(startScope, endScope) {
		return priorityPointVerdict{Caliber: priorityCaliberUnknown}
	}
	if stable, ok := priorityStableRangeAtPoint(a.stableByPID[pid], start, priorityPointAfter, startScope); ok &&
		comparePriorityPhysicalPoint(end, stable.End) <= 0 {
		return priorityPointVerdict{
			Priority: stable.Priority, Caliber: priorityCaliberClosedRangeStable,
			Source: stable.Source, Role: "equal_bracketing_endpoints", Start: stable.Start, End: stable.End,
		}
	}
	// Preserve a nearby value only as explicitly advisory context.
	return a.advisoryNearest(pid, start, startScope)
}

func (a *priorityPointAuthority) wakeupRelationAtPoint(flavor TraceFlavor, wakeePID, wakerPID int, point priorityPhysicalPoint) priorityPointRelationVerdict {
	target := a.pointVerdictAt(wakeePID, point, priorityPointAt)
	subject := a.pointVerdictAt(wakerPID, point, priorityPointAt)
	verdict := priorityPointRelationVerdict{Target: target, Subject: subject}
	// A wake edge has a row-local wakee priority field. Only that exact-at-row
	// value may authorize the edge relation; a surrounding target range cannot
	// rescue a missing, inferred or explicitly untrusted wake field. The waker
	// has no row-local field and therefore requires exact or closed-range proof.
	if target.Caliber == priorityCaliberExactAtPoint && target.Priority > 0 && subject.hardEvidence() {
		verdict.Relation = priorityRelation(flavor, target.Priority, subject.Priority)
	}
	return verdict
}

func (a *priorityPointAuthority) dependencyRelationAtPoint(flavor TraceFlavor, targetPID, dependencyPID, depth int, point priorityPhysicalPoint) priorityPointRelationVerdict {
	target := a.pointVerdictAt(targetPID, point, priorityPointAt)
	subject := a.pointVerdictAt(dependencyPID, point, priorityPointAt)
	verdict := priorityPointRelationVerdict{Target: target, Subject: subject}
	if target.hardEvidence() && subject.hardEvidence() {
		verdict.Relation = dependencyPriorityRelation(flavor, target.Priority, subject.Priority, depth)
	}
	return verdict
}

// lowerPriorityRelationSlices intersects two threads' closed-range stable
// priority ledgers with the caller's exact scheduler intervals. It performs no
// CPU equality check: causal dependency/supply inversion is intentionally
// cross-CPU. A direct runnable-displacement caller remains responsible for its
// independent same-CPU overlap gate before invoking this method.
func (a *priorityPointAuthority) lowerPriorityRelationSlices(flavor TraceFlavor, targetPID, dependencyPID, depth int, intervals []Interval) []priorityRelationSlice {
	if a == nil || a.cancel.sample() || !a.buildComplete || depth <= 0 || targetPID <= 0 || dependencyPID <= 0 || targetPID == dependencyPID || len(intervals) == 0 {
		return nil
	}
	targetRanges := a.stableByPID[targetPID]
	dependencyRanges := a.stableByPID[dependencyPID]
	if len(targetRanges) == 0 || len(dependencyRanges) == 0 {
		return nil
	}
	// Intersect the two immutable, source/start-sorted range ledgers once. The
	// prior interval x target-range x dependency-range walk was quadratic in
	// both retained ledgers and then repeated that work for every interval.
	windows := make([]priorityLowerRelationWindow, 0, min(len(targetRanges), len(dependencyRanges)))
	for targetIndex, dependencyIndex := 0, 0; targetIndex < len(targetRanges) && dependencyIndex < len(dependencyRanges); {
		if a.cancel.tick() {
			return nil
		}
		target := targetRanges[targetIndex]
		dependency := dependencyRanges[dependencyIndex]
		if target.Source < dependency.Source {
			targetIndex++
			continue
		}
		if target.Source > dependency.Source {
			dependencyIndex++
			continue
		}
		start := maxPriorityPhysicalPoint(target.Start, dependency.Start)
		end := minPriorityPhysicalPoint(target.End, dependency.End)
		if comparePriorityPhysicalPoint(start, end) < 0 && end.Ts > start.Ts &&
			dependencyPriorityRelation(flavor, target.Priority, dependency.Priority, depth) == "lower_priority_dependency" {
			windows = append(windows, priorityLowerRelationWindow{
				Source: target.Source, Start: start, End: end,
				TargetPriority: target.Priority, DependencyPriority: dependency.Priority,
				DependencyGeneration: dependency.Generation,
			})
		}
		endOrder := comparePriorityPhysicalPoint(target.End, dependency.End)
		if endOrder <= 0 {
			targetIndex++
		}
		if endOrder >= 0 {
			dependencyIndex++
		}
	}
	if a.cancel.sample() || len(windows) == 0 {
		return nil
	}

	var out []priorityRelationSlice
	for _, interval := range intervals {
		if a.cancel.tick() {
			return nil
		}
		if interval.EndTs <= interval.StartTs || interval.DurationMs <= 0 || interval.Thread.PID != dependencyPID {
			continue
		}
		intervalSource, intervalGeneration, intervalProven := a.intervalPriorityProvenance(interval, dependencyPID)
		if !intervalProven {
			continue
		}
		first, last := priorityLowerRelationWindowSourceBounds(windows, intervalSource)
		first += sort.Search(last-first, func(i int) bool {
			return windows[first+i].End.Ts > interval.StartTs
		})
		for i := first; i < last && windows[i].Start.Ts < interval.EndTs; i++ {
			if a.cancel.tick() {
				return nil
			}
			window := windows[i]
			if !samePriorityGeneration(window.DependencyGeneration, intervalGeneration) {
				continue
			}
			start := math.Max(interval.StartTs, window.Start.Ts)
			end := math.Min(interval.EndTs, window.End.Ts)
			if end <= start {
				continue
			}
			clipped := interval
			wall := interval.EndTs - interval.StartTs
			clipped.StartTs = start
			clipped.EndTs = end
			clipped.DurationMs = interval.DurationMs * (end - start) / wall
			clipped.ActualStartTs = start
			clipped.ActualEndTs = end
			clipped.ActualDurationMs = clipped.DurationMs
			if start != interval.StartTs {
				clipped.StartLine = 0
			}
			if end != interval.EndTs {
				clipped.EndLine = 0
			}
			out = append(out, priorityRelationSlice{
				Interval: clipped, TargetPriority: window.TargetPriority, DependencyPriority: window.DependencyPriority,
				Relation: "lower_priority_dependency", Source: window.Source,
			})
		}
	}
	if a.cancel.sample() {
		return nil
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Interval.StartTs != out[j].Interval.StartTs {
			return out[i].Interval.StartTs < out[j].Interval.StartTs
		}
		if out[i].Interval.EndTs != out[j].Interval.EndTs {
			return out[i].Interval.EndTs < out[j].Interval.EndTs
		}
		if out[i].TargetPriority != out[j].TargetPriority {
			return out[i].TargetPriority < out[j].TargetPriority
		}
		return out[i].DependencyPriority < out[j].DependencyPriority
	})
	if a.cancel.sample() {
		return nil
	}
	return out
}

func maxPriorityPhysicalPoint(a, b priorityPhysicalPoint) priorityPhysicalPoint {
	if comparePriorityPhysicalPoint(a, b) >= 0 {
		return a
	}
	return b
}

func minPriorityPhysicalPoint(a, b priorityPhysicalPoint) priorityPhysicalPoint {
	if comparePriorityPhysicalPoint(a, b) <= 0 {
		return a
	}
	return b
}

func priorityLowerRelationWindowSourceBounds(windows []priorityLowerRelationWindow, source string) (int, int) {
	first := sort.Search(len(windows), func(i int) bool {
		return windows[i].Source >= source
	})
	last := first + sort.Search(len(windows)-first, func(i int) bool {
		return windows[first+i].Source > source
	})
	return first, last
}

// intervalPriorityProvenance binds a scheduler interval back to exactly one
// physical artifact and one TID generation. Timestamps alone are not an
// artifact identity: two tracebundle children may legitimately share the
// same clock range. A range from child B must therefore never authorize an
// interval emitted by child A merely because their numeric times overlap.
func (a *priorityPointAuthority) intervalPriorityProvenance(interval Interval, expectedPID int) (string, threadGenerationScope, bool) {
	if a == nil || a.idx == nil || expectedPID <= 0 || interval.Thread.PID != expectedPID {
		return "", threadGenerationScope{}, false
	}
	type lineObservation struct {
		ts   float64
		line int
	}
	observations := []lineObservation{
		{ts: interval.StartTs, line: interval.StartLine},
		{ts: interval.EndTs, line: interval.EndLine},
		// WakeupLine is the opening coordinate of a RUNNABLE interval (the
		// same physical row makeIntervalWithWake uses as StartLine), not an
		// observation at the closing sched_switch timestamp.
		{ts: interval.StartTs, line: interval.WakeupLine},
	}
	var source string
	var generation threadGenerationScope
	seen := false
	for _, observation := range observations {
		if observation.line <= 0 || !finitePriorityTimestamp(observation.ts) {
			continue
		}
		candidateSource, ok := prioritySourceForEvent(a.idx, Event{Line: observation.line})
		if !ok || candidateSource == "" {
			return "", threadGenerationScope{}, false
		}
		candidateGeneration := threadGenerationScopeAt(a.idx, expectedPID, observation.ts, observation.line)
		if !candidateGeneration.known {
			return "", threadGenerationScope{}, false
		}
		if !seen {
			source, generation, seen = candidateSource, candidateGeneration, true
			continue
		}
		if candidateSource != source || !samePriorityGeneration(candidateGeneration, generation) {
			return "", threadGenerationScope{}, false
		}
	}
	return source, generation, seen
}
