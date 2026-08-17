package tracequery

import (
	"crypto/sha256"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func WindowDiscoveryPairingStatusNames() []string {
	return []string{
		string(WindowDiscoveryPairingCompleteExact),
		string(WindowDiscoveryPairingAmbiguousDuplicate),
		string(WindowDiscoveryPairingIncompleteOpen),
		string(WindowDiscoveryPairingOrphanEnd),
		string(WindowDiscoveryPairingLifecycleCut),
		string(WindowDiscoveryPairingMalformedEndpoint),
		string(WindowDiscoveryPairingTimestampRollback),
		string(WindowDiscoveryPairingSourceUnresolved),
		string(WindowDiscoveryPairingBudgetExceeded),
	}
}

func WindowDiscoveryCarryClassNames() []string {
	return []string{
		string(WindowDiscoveryCarryIn),
		string(WindowDiscoveryCarryOut),
		string(WindowDiscoveryCarryThrough),
		string(WindowDiscoveryInsidePair),
	}
}

type traceMarkCarrySyncLane struct {
	source     string
	emitterPID int
	generation int
	stack      []Event
	lastTs     float64
	lastTsSet  bool
	maxDepth   int
}

type traceMarkCarryCohortLane struct {
	cohort         pairingCohortState
	family         WindowDiscoveryFamily
	source         string
	payloadPID     int
	generation     int
	identity       string
	key            string
	events         []Event
	eventsOverflow bool
	lastTs         float64
	lastTsSet      bool
}

type traceMarkCarryDiscovery struct {
	req      WindowDiscoveryRequest
	scope    Query
	source   string
	families map[WindowDiscoveryFamily]bool
	stats    map[WindowDiscoveryFamily]*WindowDiscoveryFamilyStats

	generations map[string]int
	ownerSeen   map[string]bool
	syncLanes   map[string]*traceMarkCarrySyncLane
	cohortLanes map[string]*traceMarkCarryCohortLane

	candidates         []*WindowDiscoveryCandidate
	malformedSeen      map[string]bool
	familyUnsafe       map[WindowDiscoveryFamily]WindowDiscoveryPairingStatus
	endpointCount      int
	budgetStopped      bool
	poolTruncated      bool
	identityIncomplete bool
	// spanSelector is an internal, typed narrowing lane used by the streaming
	// span locator.  It is deliberately not part of WindowDiscoveryRequest's
	// public wire surface: ordinary discovery keeps its historical all-family
	// census, while span location may retain only one requested identity without
	// teaching or parsing user prose.  The complete per-emitter stack is still
	// tracked for matching lanes, so unnamed synchronous E endpoints retain LIFO
	// semantics.
	spanSelector *traceMarkCarrySpanSelector
	// Exact emitter TIDs whose lifecycle boundary intersects the selector's
	// parent or cuts an open selected lane.  The streaming span publisher uses
	// this typed set to withhold only the affected member (important for process
	// scope) instead of poisoning unrelated process members.
	spanSelectorLifecycleConflicts map[int]Event
}

type traceMarkCarrySpanSelector struct {
	name        string
	targetScope string
	pid         int
}

func (s *traceMarkCarrySpanSelector) coarseEventMatches(family WindowDiscoveryFamily, ev Event) bool {
	if s == nil || s.pid <= 0 {
		return true
	}
	if s.targetScope == TargetScopeProcess {
		return ev.TGID == s.pid || ev.SpanPID == s.pid
	}
	// A thread selector is the physical row emitter.  This also preserves
	// namespace separation: SpanPID remains the trace-marker payload process id
	// and is never substituted for the host/emitter TID.
	return ev.PID == s.pid
}

func (s *traceMarkCarrySpanSelector) startMatches(ev Event) bool {
	if s == nil {
		return true
	}
	if name := strings.TrimSpace(s.name); name != "" &&
		!strings.Contains(strings.ToLower(ev.SpanName), strings.ToLower(name)) {
		return false
	}
	return s.coarseEventMatches(WindowDiscoveryFamilyTraceSync, ev)
}

func (d *traceMarkCarryDiscovery) spanIssueRelevant(start, end Event) bool {
	if d.spanSelector == nil {
		return true
	}
	if start.Line > 0 {
		return d.spanSelector.startMatches(start)
	}
	if end.Line > 0 {
		// An unnamed sync E cannot be name-filtered, but its typed emitter can.
		// Retain it when the emitter/process selector matches because it may be
		// the missing counterpart of the requested named B.
		family, _, ok := traceMarkCarryFamily(end.SpanAction)
		if !ok {
			family = WindowDiscoveryFamilyTraceSync
		}
		return d.spanSelector.coarseEventMatches(family, end)
	}
	return true
}

func (d *traceMarkCarryDiscovery) spanEndpointRelevant(source string, family WindowDiscoveryFamily, ev Event) bool {
	if d.spanSelector == nil {
		return true
	}
	if family != WindowDiscoveryFamilyTraceSync {
		// Async S/F endpoints repeat their typed name/cookie identity, so they
		// can be narrowed before consuming the endpoint budget.
		return d.spanSelector.startMatches(ev)
	}
	// A bare sync E never repeats the B name and, in a pid namespace, need not
	// repeat B's payload process id.  Start a lane only from a selected named B;
	// once open, retain every nested B/E on that exact physical emitter until
	// the stack closes.  This preserves LIFO while preventing unrelated marker
	// traffic from exhausting the streaming endpoint budget.
	key := traceMarkCarrySyncKey(strings.TrimSpace(source), ev.PID, d.generation(strings.TrimSpace(source), ev.PID))
	if d.syncLanes[key] != nil {
		return true
	}
	return ev.SpanAction == "B" && d.spanSelector.startMatches(ev)
}

func newTraceMarkCarryDiscovery(req WindowDiscoveryRequest, source string) *traceMarkCarryDiscovery {
	d := &traceMarkCarryDiscovery{
		req: req,
		scope: Query{
			TimeStart: req.TimeStart, TimeEnd: req.TimeEnd,
			TimeStartSet: req.TimeStartSet, TimeEndSet: req.TimeEndSet,
			LineStart: req.LineStart, LineEnd: req.LineEnd,
		},
		source:        strings.TrimSpace(source),
		families:      map[WindowDiscoveryFamily]bool{},
		stats:         map[WindowDiscoveryFamily]*WindowDiscoveryFamilyStats{},
		generations:   map[string]int{},
		ownerSeen:     map[string]bool{},
		syncLanes:     map[string]*traceMarkCarrySyncLane{},
		cohortLanes:   map[string]*traceMarkCarryCohortLane{},
		malformedSeen: map[string]bool{},
		familyUnsafe:  map[WindowDiscoveryFamily]WindowDiscoveryPairingStatus{},
	}
	for _, family := range req.Families {
		d.families[family] = true
		d.stats[family] = &WindowDiscoveryFamilyStats{Family: family}
	}
	return d
}

func traceMarkCarryFamily(action string) (WindowDiscoveryFamily, string, bool) {
	switch action {
	case "B":
		return WindowDiscoveryFamilyTraceSync, "start", true
	case "E":
		return WindowDiscoveryFamilyTraceSync, "done", true
	case "S":
		return WindowDiscoveryFamilyTraceAsync, "start", true
	case "F":
		return WindowDiscoveryFamilyTraceAsync, "done", true
	case "G":
		return WindowDiscoveryFamilyTraceTrack, "start", true
	case "H":
		return WindowDiscoveryFamilyTraceTrack, "done", true
	default:
		return "", "", false
	}
}

func traceMarkCarryFamilyForInvalid(action traceMarkInvalidAction) (WindowDiscoveryFamily, bool) {
	switch action {
	case traceMarkActionB, traceMarkActionE:
		return WindowDiscoveryFamilyTraceSync, true
	case traceMarkActionS, traceMarkActionF:
		return WindowDiscoveryFamilyTraceAsync, true
	case traceMarkActionG, traceMarkActionH:
		return WindowDiscoveryFamilyTraceTrack, true
	default:
		return "", false
	}
}

func traceMarkCarryGenerationKey(source string, pid int) string {
	return source + "\x00" + strconv.Itoa(pid)
}

func traceMarkCarrySourceLabel(source string) string {
	if base := strings.TrimSpace(filepath.Base(source)); base != "" && base != "." {
		return base
	}
	return "<unresolved>"
}

func (d *traceMarkCarryDiscovery) generation(source string, pid int) int {
	return d.generations[traceMarkCarryGenerationKey(source, pid)]
}

func (d *traceMarkCarryDiscovery) markOwner(source string, pid int) {
	if source != "" && pid > 0 {
		d.ownerSeen[traceMarkCarryGenerationKey(source, pid)] = true
	}
}

func traceMarkCarrySyncKey(source string, emitterPID, generation int) string {
	return strings.Join([]string{source, strconv.Itoa(emitterPID), strconv.Itoa(generation)}, "\x00")
}

func traceMarkCarryCohortKey(source string, family WindowDiscoveryFamily, ev Event, generation int) (string, string, bool) {
	switch family {
	case WindowDiscoveryFamilyTraceAsync:
		if ev.SpanPID <= 0 || strings.TrimSpace(ev.SpanName) == "" || strings.TrimSpace(ev.SpanValue) == "" {
			return "", "", false
		}
		key := strings.Join([]string{source, string(family), strconv.Itoa(ev.SpanPID), strconv.Itoa(generation), ev.SpanName, ev.SpanValue}, "\x00")
		identity := fmt.Sprintf("source_artifact=%s payload_pid=%d generation=%d name=%q cookie=%q", traceMarkCarrySourceLabel(source), ev.SpanPID, generation, ev.SpanName, ev.SpanValue)
		return key, identity, true
	case WindowDiscoveryFamilyTraceTrack:
		track := traceTrackNameFromEvent(ev)
		if ev.SpanPID <= 0 || track == "" || strings.TrimSpace(ev.SpanValue) == "" {
			return "", "", false
		}
		key := strings.Join([]string{source, string(family), strconv.Itoa(ev.SpanPID), strconv.Itoa(generation), track, ev.SpanValue}, "\x00")
		identity := fmt.Sprintf("source_artifact=%s payload_pid=%d generation=%d track=%q cookie=%q", traceMarkCarrySourceLabel(source), ev.SpanPID, generation, track, ev.SpanValue)
		return key, identity, true
	default:
		return "", "", false
	}
}

func (d *traceMarkCarryDiscovery) observe(source string, ev Event) bool {
	if resetPID, reset := schedulerLifecycleResetPID(ev); reset && resetPID > 0 {
		d.observeLifecycleBoundary(strings.TrimSpace(source), resetPID, ev)
	}
	if ev.Type == EventTraceAsyncInterval {
		return d.observeCompletedAsyncInterval(strings.TrimSpace(source), ev)
	}
	if ev.Type != EventTraceMark {
		return true
	}
	if ev.SpanAction == "" {
		action, reason := traceMarkEventInvalidCodes(ev)
		family, recognized := traceMarkCarryFamilyForInvalid(action)
		if !recognized || !d.families[family] {
			return true
		}
		if !d.reserveEndpoint(family) {
			return false
		}
		phase := "done"
		if action == traceMarkActionB || action == traceMarkActionS || action == traceMarkActionG {
			phase = "start"
		}
		d.observeStatsEndpoint(family, phase, ev)
		failure := traceMarkIntegrityFailure{
			Action: action.String(), Reason: reason.String(), Line: ev.Line, LocalLine: ev.Line,
			Ts: ev.Ts, TimestampKnown: true, RowPID: ev.PID, EmitterKnown: ev.PID >= 0,
			SourcePath: strings.TrimSpace(source),
		}
		if failure.Reason == "" {
			failure.Reason = "invalid_endpoint_payload"
		}
		d.recordMalformedFailure(failure)
		return true
	}
	family, phase, recognized := traceMarkCarryFamily(ev.SpanAction)
	if !recognized || !d.families[family] {
		return true
	}
	if !d.spanEndpointRelevant(source, family, ev) {
		return true
	}
	if !d.reserveEndpoint(family) {
		return false
	}
	d.observeStatsEndpoint(family, phase, ev)
	source = strings.TrimSpace(source)
	if source == "" {
		d.recordSourceUnresolved(family, ev)
		return true
	}
	if math.IsNaN(ev.Ts) || math.IsInf(ev.Ts, 0) || ev.Ts < 0 {
		d.recordRollback(family, Event{}, ev, source, 0, "invalid_endpoint_timestamp")
		return true
	}
	switch family {
	case WindowDiscoveryFamilyTraceSync:
		d.markOwner(source, ev.PID)
		return d.observeSync(source, phase, ev)
	case WindowDiscoveryFamilyTraceAsync, WindowDiscoveryFamilyTraceTrack:
		d.markOwner(source, ev.SpanPID)
		return d.observeCohort(source, family, phase, ev)
	default:
		return true
	}
}

func (d *traceMarkCarryDiscovery) observeCompletedAsyncInterval(source string, ev Event) bool {
	const family = WindowDiscoveryFamilyTraceAsync
	if !d.families[family] {
		return true
	}
	if d.spanSelector != nil && !d.spanSelector.startMatches(ev) {
		return true
	}
	if !d.reserveEndpoint(family) {
		return false
	}
	stats := d.stats[family]
	if stats != nil {
		stats.EndpointCount++
	}
	span, ok := traceSpanFromCompletedAsyncInterval(ev, source)
	if !ok || source == "" {
		d.identityIncomplete = true
		if source == "" {
			d.recordSourceUnresolved(family, ev)
		}
		return true
	}
	end := Event{Line: ev.Line, Ts: span.EndTs}
	if !pairingIntervalIntersectsQuery(ev, end, d.scope) {
		return true
	}
	if stats != nil {
		stats.ScopedEndpointCount++
		stats.CompletedIntervalCount++
	}
	identity := fmt.Sprintf(
		"source_artifact=%s source_row=%d payload_pid=%d name=%q cookie=%q",
		traceMarkCarrySourceLabel(source), ev.PluginFields.AsyncInterval.SourceRow,
		ev.SpanPID, ev.SpanName, ev.SpanValue,
	)
	exactIdentity := strings.Join([]string{
		source,
		strconv.FormatUint(ev.PluginFields.AsyncInterval.SourceRow, 10),
		strconv.Itoa(ev.SpanPID),
		ev.SpanName,
		ev.SpanValue,
	}, "\x00")
	candidate := &WindowDiscoveryCandidate{
		Family: family, Kind: "typed_interval", Identity: identity,
		IdentityFingerprint: traceMarkCarryFingerprint(exactIdentity, ev.Line, ev.Line),
		FirstLine:           ev.Line, LastLine: ev.Line,
		CoreStartTs: span.StartTs, CoreEndTs: span.EndTs,
		EndpointCount: 1, MaxDepth: 1, Closed: true,
		PairingStatus: WindowDiscoveryPairingCompleteExact,
		CarryClass:    traceMarkCarryClassify(ev, end, d.scope),
		SemanticClass: TraceSpanSemanticClass(ev.SpanName),
		StartEndpoint: traceMarkCarryEndpointProvenance(source, ev, 0),
		events:        []Event{ev, end},
	}
	candidate.windows, candidate.CollectionComplete, candidate.RequiredWindowCount =
		buildTraceMarkCarryWindows(candidate, d.req)
	candidate.FitsSingleWindow = len(candidate.windows) == 1
	if !candidate.CollectionComplete {
		candidate.CollectionBlockedReason = "typed_interval_window_exceeded_hard_budget"
	}
	d.retainCandidate(candidate)
	return true
}

func (d *traceMarkCarryDiscovery) reserveEndpoint(family WindowDiscoveryFamily) bool {
	if d.endpointCount >= d.req.EndpointLimit {
		d.budgetStopped = true
		d.identityIncomplete = true
		d.setFamilyUnsafe(family, WindowDiscoveryPairingBudgetExceeded)
		return false
	}
	d.endpointCount++
	return true
}

func (d *traceMarkCarryDiscovery) observeStatsEndpoint(family WindowDiscoveryFamily, phase string, ev Event) {
	stats := d.stats[family]
	if stats == nil {
		return
	}
	stats.EndpointCount++
	if phase == "start" {
		stats.StartCount++
	} else {
		stats.DoneCount++
	}
	if pairingEventInsideQuery(ev, d.scope) {
		stats.ScopedEndpointCount++
		if phase == "start" {
			stats.ScopedStartCount++
		} else {
			stats.ScopedDoneCount++
		}
	}
}

func (d *traceMarkCarryDiscovery) activeLaneAvailable(family WindowDiscoveryFamily) bool {
	if len(d.syncLanes)+len(d.cohortLanes) < d.req.ActiveLaneLimit {
		return true
	}
	d.budgetStopped = true
	d.identityIncomplete = true
	d.setFamilyUnsafe(family, WindowDiscoveryPairingBudgetExceeded)
	return false
}

func (d *traceMarkCarryDiscovery) observeSync(source, phase string, ev Event) bool {
	generation := d.generation(source, ev.PID)
	key := traceMarkCarrySyncKey(source, ev.PID, generation)
	lane := d.syncLanes[key]
	if phase == "start" && lane == nil {
		if !d.activeLaneAvailable(WindowDiscoveryFamilyTraceSync) {
			return false
		}
		lane = &traceMarkCarrySyncLane{source: source, emitterPID: ev.PID, generation: generation}
		d.syncLanes[key] = lane
	}
	if lane == nil {
		if pairingEventInsideQuery(ev, d.scope) {
			d.stats[WindowDiscoveryFamilyTraceSync].UnpairedDoneCount++
			d.recordIssue(WindowDiscoveryFamilyTraceSync, WindowDiscoveryPairingOrphanEnd, Event{}, ev, source, generation,
				fmt.Sprintf("source_artifact=%s emitter_pid=%d generation=%d", traceMarkCarrySourceLabel(source), ev.PID, generation), "end_without_open_sync_stack")
		}
		return true
	}
	if lane.lastTsSet && ev.Ts < lane.lastTs {
		var first Event
		if len(lane.stack) > 0 {
			first = lane.stack[0]
		}
		d.recordRollback(WindowDiscoveryFamilyTraceSync, first, ev, source, generation, "same_lane_timestamp_rollback")
		delete(d.syncLanes, key)
		return true
	}
	lane.lastTs, lane.lastTsSet = ev.Ts, true
	if phase == "start" {
		if len(lane.stack) >= d.req.CohortEventLimit {
			d.stats[WindowDiscoveryFamilyTraceSync].CohortEventOverflowCount++
			d.identityIncomplete = true
			d.setFamilyUnsafe(WindowDiscoveryFamilyTraceSync, WindowDiscoveryPairingBudgetExceeded)
			d.recordIssue(WindowDiscoveryFamilyTraceSync, WindowDiscoveryPairingBudgetExceeded, lane.stack[0], ev, source, generation,
				fmt.Sprintf("source_artifact=%s emitter_pid=%d generation=%d", traceMarkCarrySourceLabel(source), ev.PID, generation), "sync_stack_depth_exceeded_budget")
			delete(d.syncLanes, key)
			return true
		}
		lane.stack = append(lane.stack, ev)
		if len(lane.stack) > lane.maxDepth {
			lane.maxDepth = len(lane.stack)
		}
		return true
	}
	start := lane.stack[len(lane.stack)-1]
	lane.stack = lane.stack[:len(lane.stack)-1]
	identity := fmt.Sprintf("source_artifact=%s emitter_pid=%d generation=%d nested_lifo=true", traceMarkCarrySourceLabel(source), ev.PID, generation)
	d.recordExactPair(WindowDiscoveryFamilyTraceSync, start, ev, source, generation, identity, key, lane.maxDepth)
	if len(lane.stack) == 0 {
		delete(d.syncLanes, key)
	}
	return true
}

func (d *traceMarkCarryDiscovery) observeCohort(source string, family WindowDiscoveryFamily, phase string, ev Event) bool {
	generation := d.generation(source, ev.SpanPID)
	key, identity, valid := traceMarkCarryCohortKey(source, family, ev, generation)
	if !valid {
		d.recordSourceUnresolved(family, ev)
		return true
	}
	lane := d.cohortLanes[key]
	if phase == "done" && lane == nil {
		if pairingEventInsideQuery(ev, d.scope) {
			d.stats[family].UnpairedDoneCount++
			d.recordIssue(family, WindowDiscoveryPairingOrphanEnd, Event{}, ev, source, generation, identity, "end_without_open_exact_lane")
		}
		return true
	}
	if lane == nil {
		if !d.activeLaneAvailable(family) {
			return false
		}
		lane = &traceMarkCarryCohortLane{
			family: family, source: source, payloadPID: ev.SpanPID, generation: generation,
			identity: identity, key: key,
		}
		d.cohortLanes[key] = lane
	}
	if lane.lastTsSet && ev.Ts < lane.lastTs {
		d.recordRollback(family, lane.cohort.first, ev, source, generation, "same_lane_timestamp_rollback")
		delete(d.cohortLanes, key)
		return true
	}
	lane.lastTs, lane.lastTsSet = ev.Ts, true
	if len(lane.events) < d.req.CohortEventLimit {
		lane.events = append(lane.events, ev)
	} else if !lane.eventsOverflow {
		lane.eventsOverflow = true
		d.stats[family].CohortEventOverflowCount++
		d.identityIncomplete = true
		d.setFamilyUnsafe(family, WindowDiscoveryPairingBudgetExceeded)
	}
	var transition pairingCohortTransition
	if phase == "start" {
		transition = lane.cohort.observeStart(ev)
	} else {
		transition = lane.cohort.observeDone(ev)
	}
	if !transition.cohortClosed {
		return true
	}
	delete(d.cohortLanes, key)
	if !pairingIntervalIntersectsQuery(transition.first, transition.last, d.scope) {
		return true
	}
	if lane.eventsOverflow {
		d.recordIssue(family, WindowDiscoveryPairingBudgetExceeded, transition.first, transition.last, source, generation, identity, "cohort_endpoint_roster_exceeded_budget")
		return true
	}
	if transition.ambiguous {
		d.stats[family].ClosedAmbiguousCount++
		d.recordIssue(family, WindowDiscoveryPairingAmbiguousDuplicate, transition.first, transition.last, source, generation, identity, "duplicate_start_cohort_withheld")
		return true
	}
	if transition.pairReady {
		d.recordExactPair(family, transition.pairStart, transition.last, source, generation, identity, lane.key, 1)
	}
	return true
}

func (d *traceMarkCarryDiscovery) observeLifecycleBoundary(source string, pid int, boundary Event) {
	if source == "" || pid <= 0 {
		return
	}
	generationKey := traceMarkCarryGenerationKey(source, pid)
	if !d.ownerSeen[generationKey] {
		return
	}
	if d.spanSelector != nil && pairingEventInsideQuery(boundary, d.scope) {
		if d.spanSelectorLifecycleConflicts == nil {
			d.spanSelectorLifecycleConflicts = map[int]Event{}
		}
		d.spanSelectorLifecycleConflicts[pid] = boundary
		d.identityIncomplete = true
	}
	for key, lane := range d.syncLanes {
		if lane.source != source || lane.emitterPID != pid {
			continue
		}
		relevant := false
		for _, start := range lane.stack {
			if pairingIntervalIntersectsQuery(start, boundary, d.scope) {
				relevant = true
				status := WindowDiscoveryPairingLifecycleCut
				reason := "emitter_generation_boundary_before_sync_stack_closed"
				if boundary.Ts < start.Ts {
					status, reason = WindowDiscoveryPairingTimestampRollback, "lifecycle_boundary_timestamp_rollback"
					d.setFamilyUnsafe(WindowDiscoveryFamilyTraceSync, status)
					d.stats[WindowDiscoveryFamilyTraceSync].TimestampRollbackCount++
				}
				d.recordIssue(WindowDiscoveryFamilyTraceSync, status, start, boundary, source, lane.generation,
					fmt.Sprintf("source_artifact=%s emitter_pid=%d generation=%d", traceMarkCarrySourceLabel(source), pid, lane.generation), reason)
			}
		}
		if relevant {
			d.stats[WindowDiscoveryFamilyTraceSync].LifecycleResetLaneCount++
			d.identityIncomplete = true
			if d.spanSelector != nil {
				if d.spanSelectorLifecycleConflicts == nil {
					d.spanSelectorLifecycleConflicts = map[int]Event{}
				}
				d.spanSelectorLifecycleConflicts[pid] = boundary
			}
		}
		delete(d.syncLanes, key)
	}
	for key, lane := range d.cohortLanes {
		if lane.source != source || lane.payloadPID != pid {
			continue
		}
		transition := lane.cohort.finishEOF()
		if pairingIntervalIntersectsQuery(transition.first, boundary, d.scope) {
			status := WindowDiscoveryPairingLifecycleCut
			reason := "payload_owner_generation_boundary_before_cohort_closed"
			if boundary.Ts < transition.first.Ts {
				status, reason = WindowDiscoveryPairingTimestampRollback, "lifecycle_boundary_timestamp_rollback"
				d.setFamilyUnsafe(lane.family, status)
				d.stats[lane.family].TimestampRollbackCount++
			}
			d.stats[lane.family].LifecycleResetLaneCount++
			d.identityIncomplete = true
			if d.spanSelector != nil {
				if d.spanSelectorLifecycleConflicts == nil {
					d.spanSelectorLifecycleConflicts = map[int]Event{}
				}
				d.spanSelectorLifecycleConflicts[pid] = boundary
			}
			d.recordIssue(lane.family, status, transition.first, boundary, source, lane.generation, lane.identity, reason)
		}
		delete(d.cohortLanes, key)
	}
	d.generations[generationKey]++
}

func (d *traceMarkCarryDiscovery) recordExactPair(family WindowDiscoveryFamily, start, end Event, source string, generation int, identity, exactIdentity string, maxDepth int) {
	if end.Ts < start.Ts {
		d.recordRollback(family, start, end, source, generation, "pair_end_before_start")
		return
	}
	if !pairingIntervalIntersectsQuery(start, end, d.scope) {
		return
	}
	if d.spanSelector != nil && !d.spanSelector.startMatches(start) {
		return
	}
	d.stats[family].CompletedPairCount++
	carry := traceMarkCarryClassify(start, end, d.scope)
	semanticClass := TraceSpanSemanticClass(start.SpanName)
	fingerprint := traceMarkCarryFingerprint(exactIdentity, start.Line, end.Line)
	candidate := &WindowDiscoveryCandidate{
		Family: family, Kind: "exact_pair", Identity: identity, IdentityFingerprint: fingerprint,
		FirstLine: minPositiveLine(start.Line, end.Line), LastLine: maxInt(start.Line, end.Line),
		CoreStartTs: start.Ts, CoreEndTs: end.Ts,
		EndpointCount: 2, StartCount: 1, DoneCount: 1, MaxDepth: maxInt(1, maxDepth), Closed: true,
		PairingStatus: WindowDiscoveryPairingCompleteExact, CarryClass: carry, SemanticClass: semanticClass,
		StartEndpoint: traceMarkCarryEndpointProvenance(source, start, generation),
		EndEndpoint:   traceMarkCarryEndpointProvenance(source, end, generation),
		events:        []Event{start, end},
	}
	candidate.windows, candidate.CollectionComplete, candidate.RequiredWindowCount = buildTraceMarkCarryWindows(candidate, d.req)
	candidate.FitsSingleWindow = len(candidate.windows) == 1
	if !candidate.CollectionComplete {
		candidate.CollectionBlockedReason = "exact_pair_endpoint_windows_exceeded_hard_budget"
	}
	d.retainCandidate(candidate)
}

func traceMarkCarryClassify(start, end Event, q Query) WindowDiscoveryCarryClass {
	startInside := pairingEventInsideQuery(start, q)
	endInside := pairingEventInsideQuery(end, q)
	switch {
	case startInside && endInside:
		return WindowDiscoveryInsidePair
	case !startInside && endInside:
		return WindowDiscoveryCarryIn
	case startInside && !endInside:
		return WindowDiscoveryCarryOut
	default:
		return WindowDiscoveryCarryThrough
	}
}

func traceMarkCarryEndpointProvenance(source string, ev Event, generation int) *WindowDiscoveryEndpointProvenance {
	if ev.Line <= 0 {
		return nil
	}
	action := ev.SpanAction
	if action == "" {
		action = ev.Name
	}
	return &WindowDiscoveryEndpointProvenance{
		Action: action, SourcePath: source, Line: ev.Line, Ts: ev.Ts,
		EmitterPID: ev.PID, PayloadPID: ev.SpanPID, Generation: generation,
		Name: ev.SpanName, Track: traceTrackNameFromEvent(ev), Cookie: ev.SpanValue,
		RawEvent: ev.FieldText,
	}
}

func buildTraceMarkCarryWindows(candidate *WindowDiscoveryCandidate, req WindowDiscoveryRequest) ([]DiscoveredWindow, bool, int) {
	windows, complete, required := buildCandidateWindows(candidate, req)
	if !complete || len(windows) == 0 || len(windows) > 2 {
		return nil, false, required
	}
	for i := range windows {
		windows[i].WindowOrigin = string(WindowDiscoveryTraceMarkCarry)
		windows[i].RankBasis = "soft_collection_priority:deterministic_semantic_class;carry_class;family;physical_line; no causal claim"
		windows[i].PairingStatus = candidate.PairingStatus
		windows[i].CarryClass = candidate.CarryClass
		windows[i].SemanticClass = candidate.SemanticClass
		windows[i].StartEndpoint = candidate.StartEndpoint
		windows[i].EndEndpoint = candidate.EndEndpoint
	}
	return windows, true, required
}

func (d *traceMarkCarryDiscovery) recordIssue(family WindowDiscoveryFamily, status WindowDiscoveryPairingStatus, start, end Event, source string, generation int, identity, reason string) {
	if !d.spanIssueRelevant(start, end) {
		return
	}
	if start.Line > 0 && end.Line > 0 && !pairingIntervalIntersectsQuery(start, end, d.scope) {
		return
	}
	firstLine, lastLine := minPositiveLine(start.Line, end.Line), maxInt(start.Line, end.Line)
	coreStart, coreEnd := start.Ts, end.Ts
	if start.Line == 0 {
		coreStart = end.Ts
	}
	if end.Line == 0 {
		coreEnd = start.Ts
	}
	if coreEnd < coreStart {
		coreStart, coreEnd = coreEnd, coreStart
	}
	candidate := &WindowDiscoveryCandidate{
		Family: family, Kind: "pairing_issue", Identity: identity,
		IdentityFingerprint: traceMarkCarryFingerprint(string(status)+"\x00"+source+"\x00"+identity, firstLine, lastLine),
		FirstLine:           firstLine, LastLine: lastLine, CoreStartTs: coreStart, CoreEndTs: coreEnd,
		EndpointCount: boolCount(start.Line > 0) + boolCount(end.Line > 0), Closed: end.Line > 0,
		CollectionComplete: false, PairingStatus: status, CollectionBlockedReason: reason,
		StartEndpoint: traceMarkCarryEndpointProvenance(source, start, generation),
		EndEndpoint:   traceMarkCarryEndpointProvenance(source, end, generation),
	}
	if start.Line > 0 && end.Line > 0 && status != WindowDiscoveryPairingTimestampRollback {
		candidate.CarryClass = traceMarkCarryClassify(start, end, d.scope)
	}
	d.retainCandidate(candidate)
}

func (d *traceMarkCarryDiscovery) recordRollback(family WindowDiscoveryFamily, start, end Event, source string, generation int, reason string) {
	if !d.spanIssueRelevant(start, end) {
		return
	}
	d.stats[family].TimestampRollbackCount++
	d.identityIncomplete = true
	d.setFamilyUnsafe(family, WindowDiscoveryPairingTimestampRollback)
	d.recordIssue(family, WindowDiscoveryPairingTimestampRollback, start, end, source, generation, "timestamp_order", reason)
}

func (d *traceMarkCarryDiscovery) recordSourceUnresolved(family WindowDiscoveryFamily, ev Event) {
	d.stats[family].InvalidIdentityCount++
	d.identityIncomplete = true
	d.setFamilyUnsafe(family, WindowDiscoveryPairingSourceUnresolved)
	if ev.SpanAction == "E" || ev.SpanAction == "F" || ev.SpanAction == "H" {
		d.recordIssue(family, WindowDiscoveryPairingSourceUnresolved, Event{}, ev, "", 0, "physical_source_unresolved", "endpoint_could_not_be_mapped_to_one_physical_source")
		return
	}
	d.recordIssue(family, WindowDiscoveryPairingSourceUnresolved, ev, Event{}, "", 0, "physical_source_unresolved", "endpoint_could_not_be_mapped_to_one_physical_source")
}

func (d *traceMarkCarryDiscovery) recordMalformedFailure(failure traceMarkIntegrityFailure) {
	family, _, recognized := traceMarkCarryFamily(failure.Action)
	if !recognized || !d.families[family] || !traceMarkIntegrityFailureRelevantToQuery(failure, d.scope) {
		return
	}
	if d.spanSelector != nil && d.spanSelector.targetScope != TargetScopeProcess &&
		d.spanSelector.pid > 0 && failure.EmitterKnown && failure.RowPID != d.spanSelector.pid {
		return
	}
	key := strings.Join([]string{failure.SourcePath, strconv.Itoa(failure.Line), failure.Action, failure.Reason}, "\x00")
	if d.malformedSeen[key] {
		return
	}
	if len(d.malformedSeen) < traceMarkIntegrityFailureCap {
		d.malformedSeen[key] = true
	}
	d.stats[family].InvalidIdentityCount++
	d.identityIncomplete = true
	d.setFamilyUnsafe(family, WindowDiscoveryPairingMalformedEndpoint)
	generationPID := failure.RowPID
	generation := d.generation(failure.SourcePath, generationPID)
	endpoint := &WindowDiscoveryEndpointProvenance{
		Action: failure.Action, SourcePath: failure.SourcePath, Line: failure.Line, Ts: failure.Ts,
		EmitterPID: failure.RowPID, Generation: generation,
	}
	candidate := &WindowDiscoveryCandidate{
		Family: family, Kind: "pairing_issue", Identity: "malformed_trace_mark_endpoint",
		IdentityFingerprint: traceMarkCarryFingerprint(key, failure.Line, failure.Line),
		FirstLine:           failure.Line, LastLine: failure.Line, CoreStartTs: failure.Ts, CoreEndTs: failure.Ts,
		EndpointCount: 1, CollectionComplete: false, PairingStatus: WindowDiscoveryPairingMalformedEndpoint,
		CollectionBlockedReason: failure.Reason,
	}
	if failure.Action == "B" || failure.Action == "S" || failure.Action == "G" {
		candidate.StartEndpoint = endpoint
	} else {
		candidate.EndEndpoint = endpoint
	}
	d.retainCandidate(candidate)
}

func (d *traceMarkCarryDiscovery) setFamilyUnsafe(family WindowDiscoveryFamily, status WindowDiscoveryPairingStatus) {
	if current := d.familyUnsafe[family]; current == "" || traceMarkPairingStatusPriority(status) < traceMarkPairingStatusPriority(current) {
		d.familyUnsafe[family] = status
	}
}

func traceMarkPairingStatusPriority(status WindowDiscoveryPairingStatus) int {
	switch status {
	case WindowDiscoveryPairingBudgetExceeded:
		return 0
	case WindowDiscoveryPairingSourceUnresolved:
		return 1
	case WindowDiscoveryPairingMalformedEndpoint:
		return 2
	case WindowDiscoveryPairingTimestampRollback:
		return 3
	case WindowDiscoveryPairingAmbiguousDuplicate:
		return 4
	case WindowDiscoveryPairingLifecycleCut:
		return 5
	case WindowDiscoveryPairingIncompleteOpen:
		return 6
	case WindowDiscoveryPairingOrphanEnd:
		return 7
	case WindowDiscoveryPairingCompleteExact:
		return 8
	default:
		return 9
	}
}

func traceMarkCarryFingerprint(identity string, firstLine, lastLine int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d", identity, firstLine, lastLine)))
	return fmt.Sprintf("sha256:%x", sum[:8])
}

func (d *traceMarkCarryDiscovery) retainCandidate(candidate *WindowDiscoveryCandidate) {
	if candidate == nil {
		return
	}
	key := strings.Join([]string{
		string(candidate.Family), string(candidate.PairingStatus), candidate.IdentityFingerprint,
		strconv.Itoa(candidate.FirstLine), strconv.Itoa(candidate.LastLine),
	}, "\x00")
	for _, existing := range d.candidates {
		if existing == nil {
			continue
		}
		existingKey := strings.Join([]string{
			string(existing.Family), string(existing.PairingStatus), existing.IdentityFingerprint,
			strconv.Itoa(existing.FirstLine), strconv.Itoa(existing.LastLine),
		}, "\x00")
		if existingKey == key {
			return
		}
	}
	d.candidates = append(d.candidates, candidate)
	sort.SliceStable(d.candidates, func(i, j int) bool { return traceMarkCarryCandidateLess(d.candidates[i], d.candidates[j]) })
	if len(d.candidates) > windowDiscoveryCandidatePoolLimit {
		d.trimCandidatePool()
		d.poolTruncated = true
	}
}

// trimCandidatePool preserves one collectible exact-pair seat per requested
// family before filling the remaining bounded pool by the global comparator.
// Without this reservation, a malformed-marker storm in one family could
// crowd a healthy exact pair from another family out of the 128-entry pool,
// accidentally widening a family-scoped fail-close decision to every lane.
func (d *traceMarkCarryDiscovery) trimCandidatePool() {
	if len(d.candidates) <= windowDiscoveryCandidatePoolLimit {
		return
	}
	reserved := map[*WindowDiscoveryCandidate]bool{}
	for _, family := range d.req.Families {
		for _, candidate := range d.candidates {
			if candidate != nil && candidate.Family == family && candidate.PairingStatus == WindowDiscoveryPairingCompleteExact &&
				candidate.CollectionComplete && len(candidate.windows) > 0 {
				reserved[candidate] = true
				break
			}
		}
	}
	capacity := windowDiscoveryCandidatePoolLimit - len(reserved)
	if capacity < 0 {
		capacity = 0
	}
	trimmed := make([]*WindowDiscoveryCandidate, 0, windowDiscoveryCandidatePoolLimit)
	for _, candidate := range d.candidates {
		if reserved[candidate] || len(trimmed) >= capacity {
			continue
		}
		trimmed = append(trimmed, candidate)
	}
	for _, candidate := range d.candidates {
		if reserved[candidate] {
			trimmed = append(trimmed, candidate)
		}
	}
	sort.SliceStable(trimmed, func(i, j int) bool { return traceMarkCarryCandidateLess(trimmed[i], trimmed[j]) })
	d.candidates = trimmed
}

func traceMarkCarryCandidateLess(a, b *WindowDiscoveryCandidate) bool {
	if a == nil || b == nil {
		return b != nil
	}
	criticalA := traceMarkPairingStatusPriority(a.PairingStatus) <= traceMarkPairingStatusPriority(WindowDiscoveryPairingTimestampRollback)
	criticalB := traceMarkPairingStatusPriority(b.PairingStatus) <= traceMarkPairingStatusPriority(WindowDiscoveryPairingTimestampRollback)
	if criticalA != criticalB {
		return criticalA
	}
	if exactA, exactB := a.PairingStatus == WindowDiscoveryPairingCompleteExact, b.PairingStatus == WindowDiscoveryPairingCompleteExact; exactA != exactB {
		return exactA
	}
	if a.PairingStatus == WindowDiscoveryPairingCompleteExact {
		if (a.SemanticClass != "") != (b.SemanticClass != "") {
			return a.SemanticClass != ""
		}
		if pa, pb := traceMarkCarryClassPriority(a.CarryClass), traceMarkCarryClassPriority(b.CarryClass); pa != pb {
			return pa < pb
		}
		if a.FitsSingleWindow != b.FitsSingleWindow {
			return a.FitsSingleWindow
		}
	} else if pa, pb := traceMarkPairingStatusPriority(a.PairingStatus), traceMarkPairingStatusPriority(b.PairingStatus); pa != pb {
		return pa < pb
	}
	if a.Family != b.Family {
		return a.Family < b.Family
	}
	if a.FirstLine != b.FirstLine {
		return a.FirstLine < b.FirstLine
	}
	if a.LastLine != b.LastLine {
		return a.LastLine < b.LastLine
	}
	return a.IdentityFingerprint < b.IdentityFingerprint
}

func traceMarkCarryClassPriority(class WindowDiscoveryCarryClass) int {
	switch class {
	case WindowDiscoveryCarryThrough:
		return 0
	case WindowDiscoveryCarryIn:
		return 1
	case WindowDiscoveryCarryOut:
		return 2
	case WindowDiscoveryInsidePair:
		return 3
	default:
		return 4
	}
}

func (d *traceMarkCarryDiscovery) finalizeOpen(shell *Index) {
	for key, lane := range d.syncLanes {
		for _, start := range lane.stack {
			if !pairingOpenCohortIntersectsIndex(start, shell, d.scope) {
				continue
			}
			d.stats[WindowDiscoveryFamilyTraceSync].OpenSingleCount++
			d.recordIssue(WindowDiscoveryFamilyTraceSync, WindowDiscoveryPairingIncompleteOpen, start, Event{}, lane.source, lane.generation,
				fmt.Sprintf("source_artifact=%s emitter_pid=%d generation=%d", traceMarkCarrySourceLabel(lane.source), lane.emitterPID, lane.generation), "sync_begin_open_at_eof")
		}
		delete(d.syncLanes, key)
	}
	for key, lane := range d.cohortLanes {
		transition := lane.cohort.finishEOF()
		if !pairingOpenCohortIntersectsIndex(transition.first, shell, d.scope) {
			delete(d.cohortLanes, key)
			continue
		}
		if transition.ambiguous {
			d.stats[lane.family].OpenAmbiguousCount++
			d.recordIssue(lane.family, WindowDiscoveryPairingAmbiguousDuplicate, transition.first, transition.last, lane.source, lane.generation, lane.identity, "duplicate_start_cohort_open_at_eof")
		} else {
			d.stats[lane.family].OpenSingleCount++
			d.recordIssue(lane.family, WindowDiscoveryPairingIncompleteOpen, transition.first, Event{}, lane.source, lane.generation, lane.identity, "begin_open_at_eof")
		}
		delete(d.cohortLanes, key)
	}
}

func (d *traceMarkCarryDiscovery) applyIntegrityShell(shell *Index) {
	if shell == nil {
		d.identityIncomplete = true
		return
	}
	for _, failure := range shell.traceMarkIntegrityFailures {
		d.recordMalformedFailure(failure)
	}
	if shell.traceMarkIntegrityDroppedGlobalPoison {
		for _, family := range []WindowDiscoveryFamily{WindowDiscoveryFamilyTraceSync, WindowDiscoveryFamilyTraceAsync} {
			if d.families[family] {
				d.identityIncomplete = true
				d.setFamilyUnsafe(family, WindowDiscoveryPairingMalformedEndpoint)
			}
		}
	}
	if shell.traceTrackIntegrityDroppedPoison && d.families[WindowDiscoveryFamilyTraceTrack] {
		d.identityIncomplete = true
		d.setFamilyUnsafe(WindowDiscoveryFamilyTraceTrack, WindowDiscoveryPairingMalformedEndpoint)
	}
}

func (d *traceMarkCarryDiscovery) finalize(shell *Index, version TraceSourceVersion) WindowDiscoveryResult {
	d.finalizeOpen(shell)
	d.applyIntegrityShell(shell)
	if d.budgetStopped {
		for family := range d.families {
			d.setFamilyUnsafe(family, WindowDiscoveryPairingBudgetExceeded)
		}
		d.recordIssue(firstTraceMarkCarryFamily(d.req.Families), WindowDiscoveryPairingBudgetExceeded, Event{}, Event{}, d.source, 0, "discovery_budget", "endpoint_or_active_lane_budget_reached")
	}
	pool := append([]*WindowDiscoveryCandidate(nil), d.candidates...)
	sort.SliceStable(pool, func(i, j int) bool { return traceMarkCarryCandidateLess(pool[i], pool[j]) })
	for _, candidate := range pool {
		if candidate.PairingStatus != WindowDiscoveryPairingCompleteExact {
			continue
		}
		if d.budgetStopped {
			candidate.CollectionComplete = false
			candidate.CollectionBlockedReason = "discovery_budget_exceeded_before_full_file_completion"
			candidate.windows = nil
			continue
		}
		if unsafe := d.familyUnsafe[candidate.Family]; unsafe != "" {
			candidate.CollectionComplete = false
			candidate.CollectionBlockedReason = "family_fail_closed:" + string(unsafe)
			candidate.windows = nil
		}
	}
	for i, candidate := range pool {
		candidate.Rank = i + 1
		for j := range candidate.windows {
			candidate.windows[j].CandidateRank = candidate.Rank
		}
	}
	d.selectWindows(pool)
	result := WindowDiscoveryResult{
		Strategy: d.req.Strategy, SourcePath: d.source, SourceFingerprint: version.Fingerprint(),
		Complete: !d.budgetStopped, IdentityComplete: !d.budgetStopped && !d.identityIncomplete,
		ParseComplete: shell != nil && shell.UnparsedLines == 0 && shell.ParseLinePanics == 0,
		EndpointCount: d.endpointCount, BudgetStopped: d.budgetStopped,
		CandidatePoolTruncated: d.poolTruncated, RetainedCandidateCount: len(pool),
		SelectionBasis: "complete exact endpoint pairs or producer-completed typed intervals intersecting the parent only; soft priority deterministic semantic class then carry class then stable family/physical-line order; no causal claim",
		Candidates:     selectedAndTopCandidates(pool, windowDiscoveryCandidateReportLimit),
	}
	if shell != nil {
		result.ScannedLineCount = shell.ScannedLineCount
		result.ParsedKnown = shell.ParsedKnown
		result.UnparsedLineCount = shell.UnparsedLines
		result.ParseLinePanics = shell.ParseLinePanics
		result.ClockRegressions = shell.ClockRegressions
	}
	for _, family := range []WindowDiscoveryFamily{WindowDiscoveryFamilyTraceSync, WindowDiscoveryFamilyTraceAsync, WindowDiscoveryFamilyTraceTrack} {
		if stats := d.stats[family]; stats != nil {
			result.Families = append(result.Families, *stats)
			result.ScopedEndpointCount += stats.ScopedEndpointCount
		}
	}
	for _, candidate := range pool {
		if !candidate.Selected {
			continue
		}
		for _, window := range candidate.windows {
			window.Ordinal = len(result.Windows) + 1
			result.Windows = append(result.Windows, window)
		}
	}
	if d.budgetStopped {
		result.Caveats = append(result.Caveats, fmt.Sprintf("trace_mark_carry_complete=false; endpoint_or_active_lane_budget_reached endpoints=%d/%d active_lane_limit=%d; no candidate window was published", d.endpointCount, d.req.EndpointLimit, d.req.ActiveLaneLimit))
	}
	if !result.ParseComplete {
		result.Caveats = append(result.Caveats, fmt.Sprintf("parse_complete=false; unparsed_lines=%d parse_panics=%d; ordinary unparsed rows are quality observations, while trace-mark endpoint poison is gated separately", result.UnparsedLineCount, result.ParseLinePanics))
	}
	if !result.IdentityComplete {
		result.Caveats = append(result.Caveats, "trace_mark_carry_identity_complete=false; malformed endpoint, source resolution, lifecycle cut, timestamp rollback, or bounded-state failure prevented a complete exact pairing claim")
	}
	if d.poolTruncated {
		result.Caveats = append(result.Caveats, fmt.Sprintf("candidate_pool_truncated=true; retained=%d under stable soft comparator with one collectible exact-pair seat reserved per requested family", windowDiscoveryCandidatePoolLimit))
	}
	if len(result.Windows) == 0 {
		result.Caveats = append(result.Caveats, "generated_windows=0; no complete exact trace-mark pair or producer-completed typed interval intersecting the parent fit the atomic fan-out budget; dependent collection must fail explicit")
	}
	return result
}

func (d *traceMarkCarryDiscovery) selectWindows(pool []*WindowDiscoveryCandidate) {
	remaining := d.req.MaxWindows
	selectedFamily := map[WindowDiscoveryFamily]bool{}
	selectCandidate := func(candidate *WindowDiscoveryCandidate, reason string) bool {
		if candidate == nil || candidate.Selected || candidate.PairingStatus != WindowDiscoveryPairingCompleteExact ||
			!candidate.CollectionComplete || len(candidate.windows) == 0 || len(candidate.windows) > remaining {
			return false
		}
		candidate.Selected = true
		candidate.SelectionReason = reason
		remaining -= len(candidate.windows)
		selectedFamily[candidate.Family] = true
		return true
	}
	for _, candidate := range pool {
		if selectCandidate(candidate, "highest_soft_priority_complete_exact_pair") {
			break
		}
	}
	for _, candidate := range pool {
		if remaining == 0 {
			break
		}
		if selectedFamily[candidate.Family] {
			continue
		}
		selectCandidate(candidate, "cross_family_complete_pair_seat")
	}
	for _, candidate := range pool {
		if remaining == 0 {
			break
		}
		selectCandidate(candidate, "stable_soft_priority_budget_fill")
	}
	for _, candidate := range pool {
		if candidate.Selected || candidate.SelectionReason != "" {
			continue
		}
		switch {
		case candidate.PairingStatus != WindowDiscoveryPairingCompleteExact:
			candidate.SelectionReason = "not_collectible:" + string(candidate.PairingStatus)
		case !candidate.CollectionComplete:
			candidate.SelectionReason = "not_collectible:" + candidate.CollectionBlockedReason
		case len(candidate.windows) > remaining:
			candidate.SelectionReason = "generated_window_budget_exhausted_pair_atomic"
		default:
			candidate.SelectionReason = "lower_soft_priority_than_selected_budget"
		}
	}
}

func firstTraceMarkCarryFamily(families []WindowDiscoveryFamily) WindowDiscoveryFamily {
	if len(families) > 0 {
		return families[0]
	}
	return WindowDiscoveryFamilyTraceSync
}

func minPositiveLine(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
