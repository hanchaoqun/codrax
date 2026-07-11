package hitraceconv

import "sort"

const maxTraceDBLifecycleMalformedPoints = 1 << 16

// traceDBLifecycleBoundary is a hard, event-backed generation begin.  The
// public TID/PID is the lane key; NewITID/NewIPID identify the left-closed side
// of the boundary.  ITID/IPID themselves are canonical row identities, not
// generations, so the same values may legitimately appear on both sides.
type traceDBLifecycleBoundary struct {
	TS      int64
	NewITID int64
	NewIPID int64
}

type traceDBLifecycleLane struct {
	Cuts          []traceDBLifecycleBoundary
	Terminals     []traceDBLifecycleBoundary
	PoisonPoints  []int64
	UnknownStarts []int64
	Tainted       bool
}

type traceDBLifecycleIndex struct {
	ByTID        map[int64]traceDBLifecycleLane
	ByPID        map[int64]traceDBLifecycleLane
	GlobalPoison []int64
	GlobalTaint  bool
}

type traceDBLifecycleTerminal struct {
	TS                int64
	Kind              string
	Activity          traceDBLifecycleBoundary
	Conflicted        bool
	Restart           traceDBLifecycleBoundary
	RestartKnown      bool
	RestartConflicted bool
	RestartBlocked    bool
}

type traceDBLifecycleBuildLane struct {
	terminalsByTS map[int64]*traceDBLifecycleTerminal
	terminals     []*traceDBLifecycleTerminal
	cuts          map[int64]traceDBLifecycleBoundary
	directCuts    map[int64]traceDBLifecycleBoundary
	cutConflicts  map[int64]bool
	poison        map[int64]bool
	unknown       map[int64]bool
	effectiveEnds []traceDBLifecycleBoundary
	tainted       bool
}

type traceDBLifecycleBuilder struct {
	identities                   traceDBThreadIndex
	lanes                        map[int64]*traceDBLifecycleBuildLane
	globalPoison                 map[int64]bool
	globalTaint                  bool
	malformedPoints              int
	malformedPointLimit          int
	malformedPointBudgetExceeded bool
	frozen                       bool
}

// A cursor is scoped to one physical activity source. Sources may arrive in
// arbitrary physical order: every activity binary-searches the latest prior
// terminal and merges the earliest candidate. This avoids temp-sorting large
// callstack/native/frame tables and remains O(rows*log(terminals)).
type traceDBLifecycleActivityCursor struct {
	builder *traceDBLifecycleBuilder
}

func newTraceDBLifecycleBuilder(identities traceDBThreadIndex) *traceDBLifecycleBuilder {
	return &traceDBLifecycleBuilder{
		identities:          identities,
		lanes:               map[int64]*traceDBLifecycleBuildLane{},
		globalPoison:        map[int64]bool{},
		malformedPointLimit: maxTraceDBLifecycleMalformedPoints,
	}
}

func (builder *traceDBLifecycleBuilder) lane(tid int64) *traceDBLifecycleBuildLane {
	lane := builder.lanes[tid]
	if lane == nil {
		lane = &traceDBLifecycleBuildLane{
			terminalsByTS: map[int64]*traceDBLifecycleTerminal{},
			cuts:          map[int64]traceDBLifecycleBoundary{},
			directCuts:    map[int64]traceDBLifecycleBoundary{},
			cutConflicts:  map[int64]bool{},
			poison:        map[int64]bool{},
			unknown:       map[int64]bool{},
		}
		builder.lanes[tid] = lane
	}
	return lane
}

func (builder *traceDBLifecycleBuilder) thread(itid int64) (traceDBThread, bool) {
	if itid <= 0 || builder.identities.AmbiguousITID[itid] {
		return traceDBThread{}, false
	}
	thread, ok := builder.identities.ByITID[itid]
	return thread, ok && thread.TID > 0
}

func (builder *traceDBLifecycleBuilder) addCreation(itid, timestamp int64) bool {
	thread, ok := builder.thread(itid)
	if !ok || timestamp < 0 {
		return false
	}
	builder.addDirectCut(thread.TID, traceDBLifecycleBoundary{TS: timestamp, NewITID: thread.ITID, NewIPID: thread.IPID})
	return true
}

func (builder *traceDBLifecycleBuilder) addTerminal(itid, timestamp int64, kind string) bool {
	thread, ok := builder.thread(itid)
	if !ok || timestamp < 0 {
		return false
	}
	lane := builder.lane(thread.TID)
	if builder.frozen {
		lane.tainted = true
		return false
	}
	if kind != "X" && kind != "Z" {
		builder.addPoison(thread.TID, timestamp)
		return false
	}
	terminal := lane.terminalsByTS[timestamp]
	activity := traceDBLifecycleBoundary{TS: timestamp, NewITID: thread.ITID, NewIPID: thread.IPID}
	if terminal == nil {
		terminal = &traceDBLifecycleTerminal{TS: timestamp, Kind: kind, Activity: activity}
		lane.terminalsByTS[timestamp] = terminal
		if lane.poison[timestamp] {
			terminal.Conflicted = true
			builder.invalidateGeneration(thread.TID, timestamp)
			return false
		}
		return true
	}
	if terminal.Kind != kind || terminal.Activity.NewITID != activity.NewITID || terminal.Activity.NewIPID != activity.NewIPID {
		terminal.Conflicted = true
		builder.invalidateGeneration(thread.TID, timestamp)
		return false
	}
	return true
}

func (builder *traceDBLifecycleBuilder) addPoisonForITID(itid, timestamp int64) {
	thread, ok := builder.thread(itid)
	if !ok {
		builder.addGlobalPoison(timestamp)
		return
	}
	builder.addPoison(thread.TID, timestamp)
}

func (builder *traceDBLifecycleBuilder) taintITID(itid int64) {
	thread, ok := builder.thread(itid)
	if !ok {
		builder.globalTaint = true
		return
	}
	builder.lane(thread.TID).tainted = true
}

func (builder *traceDBLifecycleBuilder) addGlobalPoison(timestamp int64) {
	if timestamp < 0 {
		builder.globalTaint = true
		return
	}
	if builder.globalTaint || builder.globalPoison[timestamp] {
		return
	}
	if !builder.reserveMalformedPoint() {
		return
	}
	builder.globalPoison[timestamp] = true
}

func (builder *traceDBLifecycleBuilder) taintGlobal() {
	builder.globalTaint = true
}

func (builder *traceDBLifecycleBuilder) addPoison(tid, timestamp int64) {
	if timestamp < 0 {
		builder.lane(tid).tainted = true
		return
	}
	if builder.globalTaint {
		return
	}
	lane := builder.lane(tid)
	if lane.poison[timestamp] {
		return
	}
	if !builder.reserveMalformedPoint() {
		return
	}
	lane.poison[timestamp] = true
	if terminal := lane.terminalsByTS[timestamp]; terminal != nil {
		terminal.Conflicted = true
	}
}

func (builder *traceDBLifecycleBuilder) reserveMalformedPoint() bool {
	if builder.globalTaint {
		return false
	}
	if builder.malformedPointLimit <= 0 || builder.malformedPoints >= builder.malformedPointLimit {
		builder.globalTaint = true
		builder.malformedPointBudgetExceeded = true
		builder.malformedPoints = 0
		builder.globalPoison = map[int64]bool{}
		for _, lane := range builder.lanes {
			lane.poison = map[int64]bool{}
		}
		return false
	}
	builder.malformedPoints++
	return true
}

// invalidateGeneration records that the public lane's subject becomes
// unknowable at timestamp. Unlike a standalone malformed-row poison (which is
// only an exact point/range barrier), this uncertainty persists until a later
// trusted direct begin selects a new subject. Inferred restarts require a
// previously known terminal subject and therefore cannot repair Unknown.
func (builder *traceDBLifecycleBuilder) invalidateGeneration(tid, timestamp int64) {
	if timestamp < 0 {
		builder.lane(tid).tainted = true
		return
	}
	lane := builder.lane(tid)
	lane.poison[timestamp] = true
	lane.unknown[timestamp] = true
}

func (builder *traceDBLifecycleBuilder) addCut(tid int64, cut traceDBLifecycleBoundary) {
	lane := builder.lane(tid)
	if lane.cutConflicts[cut.TS] {
		return
	}
	existing, exists := lane.cuts[cut.TS]
	if !exists {
		lane.cuts[cut.TS] = cut
		return
	}
	if existing.NewITID == cut.NewITID && existing.NewIPID == cut.NewIPID {
		return
	}
	delete(lane.cuts, cut.TS)
	lane.cutConflicts[cut.TS] = true
	builder.invalidateGeneration(tid, cut.TS)
}

func (builder *traceDBLifecycleBuilder) addDirectCut(tid int64, cut traceDBLifecycleBoundary) {
	builder.addCut(tid, cut)
	lane := builder.lane(tid)
	if lane.cutConflicts[cut.TS] {
		delete(lane.directCuts, cut.TS)
		return
	}
	if accepted, ok := lane.cuts[cut.TS]; ok {
		lane.directCuts[cut.TS] = accepted
	}
}

func (builder *traceDBLifecycleBuilder) freezeTerminals() {
	if builder.frozen {
		return
	}
	for _, lane := range builder.lanes {
		lane.terminals = make([]*traceDBLifecycleTerminal, 0, len(lane.terminalsByTS))
		for _, terminal := range lane.terminalsByTS {
			lane.terminals = append(lane.terminals, terminal)
		}
		sort.Slice(lane.terminals, func(i, j int) bool { return lane.terminals[i].TS < lane.terminals[j].TS })
	}
	builder.frozen = true
}

func (builder *traceDBLifecycleBuilder) newActivityCursor() *traceDBLifecycleActivityCursor {
	builder.freezeTerminals()
	return &traceDBLifecycleActivityCursor{
		builder: builder,
	}
}

func (cursor *traceDBLifecycleActivityCursor) observe(itid, timestamp int64) bool {
	thread, ok := cursor.builder.thread(itid)
	if !ok || timestamp < 0 {
		return false
	}
	tid := thread.TID
	activity := traceDBLifecycleBoundary{TS: timestamp, NewITID: thread.ITID, NewIPID: thread.IPID}
	lane := cursor.builder.lane(tid)
	position := sort.Search(len(lane.terminals), func(i int) bool { return lane.terminals[i].TS >= activity.TS })
	if position < len(lane.terminals) && lane.terminals[position].TS == activity.TS {
		// A terminal is an exact right boundary, never evidence of a new
		// generation at the same point. This applies to every activity source,
		// not only the table that supplied the terminal.
		return true
	}
	position--
	if position >= 0 && !lane.terminals[position].Conflicted {
		mergeTraceDBLifecycleRestart(lane.terminals[position], activity)
	}
	return true
}

func mergeTraceDBLifecycleRestart(terminal *traceDBLifecycleTerminal, activity traceDBLifecycleBoundary) {
	if !terminal.RestartKnown || activity.TS < terminal.Restart.TS {
		terminal.Restart = activity
		terminal.RestartKnown = true
		terminal.RestartConflicted = false
		return
	}
	if activity.TS > terminal.Restart.TS {
		return
	}
	if terminal.RestartConflicted {
		return
	}
	if activity.NewITID == terminal.Restart.NewITID && activity.NewIPID == terminal.Restart.NewIPID {
		return
	}
	terminal.RestartConflicted = true
}

func (builder *traceDBLifecycleBuilder) finalize() traceDBLifecycleIndex {
	builder.freezeTerminals()
	globalPoison := sortedTraceDBLifecyclePoints(builder.globalPoison)
	for tid, lane := range builder.lanes {
		builder.finalizeLane(tid, lane, globalPoison)
	}

	result := traceDBLifecycleIndex{
		ByTID:        map[int64]traceDBLifecycleLane{},
		ByPID:        map[int64]traceDBLifecycleLane{},
		GlobalPoison: globalPoison,
		GlobalTaint:  builder.globalTaint,
	}
	for tid, buildLane := range builder.lanes {
		lane := freezeTraceDBLifecycleLane(buildLane)
		result.ByTID[tid] = lane
		if !traceDBLifecycleTIDMayBeMain(builder.identities, tid) {
			continue
		}
		processLane := traceDBLifecycleLane{
			PoisonPoints:  append([]int64(nil), lane.PoisonPoints...),
			UnknownStarts: append([]int64(nil), lane.UnknownStarts...),
			Tainted:       lane.Tainted,
		}
		// Once this public TID is proven to be a process main lane, every later
		// reuse cut is also the old process's tombstone. A worker NewSubject
		// therefore remains in the PID lane so the old IPID cannot survive it.
		processLane.Cuts = append(processLane.Cuts, lane.Cuts...)
		processLane.Terminals = append(processLane.Terminals, lane.Terminals...)
		result.ByPID[tid] = processLane
	}
	return result
}

func (builder *traceDBLifecycleBuilder) finalizeLane(tid int64, lane *traceDBLifecycleBuildLane, globalPoison []int64) {
	direct := sortedTraceDBLifecycleCuts(lane.directCuts)
	lanePoison := sortedTraceDBLifecyclePoints(lane.poison)
	lane.cuts = map[int64]traceDBLifecycleBoundary{}
	lane.effectiveEnds = nil

	directPos, terminalPos, poisonPos := 0, 0, 0
	live, unknown, subjectKnown := true, false, false
	var subject traceDBLifecycleBoundary
	var pending *traceDBLifecycleTerminal

	for {
		var next int64
		haveNext := false
		if directPos < len(direct) {
			next = direct[directPos].TS
			haveNext = true
		}
		if terminalPos < len(lane.terminals) && (!haveNext || lane.terminals[terminalPos].TS < next) {
			next = lane.terminals[terminalPos].TS
			haveNext = true
		}
		if poisonPos < len(lanePoison) && (!haveNext || lanePoison[poisonPos] < next) {
			next = lanePoison[poisonPos]
			haveNext = true
		}
		candidateAt := false
		if pending != nil && pending.RestartKnown && !pending.RestartBlocked {
			if !haveNext || pending.Restart.TS < next {
				next = pending.Restart.TS
				haveNext = true
			}
			candidateAt = pending.Restart.TS == next
		}
		if !haveNext {
			break
		}
		// Global barriers are queried, never merged into every public-TID
		// lane. A point strictly inside (terminal,candidate) blocks that
		// proposal in O(log global-points); an exact candidate collision stays
		// visible below and invalidates the generation.
		if candidateAt && traceDBLifecycleOpenRangeHasPoint(globalPoison, pending.TS, next) {
			pending.RestartBlocked = true
			candidateAt = false
		}

		directAt := directPos < len(direct) && direct[directPos].TS == next
		terminalAt := terminalPos < len(lane.terminals) && lane.terminals[terminalPos].TS == next
		lanePoisonAt := poisonPos < len(lanePoison) && lanePoison[poisonPos] == next
		globalPoisonAt := traceDBLifecycleHasPoint(globalPoison, next)

		// A standalone malformed row is only an exact point/range barrier. It
		// becomes generation-invalidating only when it collides with a hard
		// lifecycle transition. A poison strictly between a terminal and its
		// proposed restart blocks that inference but leaves the known dead gap.
		invalidatingAt := lane.unknown[next] || lanePoisonAt && (directAt || candidateAt || terminalAt)
		if invalidatingAt || candidateAt && pending.RestartConflicted {
			builder.invalidateGeneration(tid, next)
			delete(lane.cuts, next)
			live, unknown, subjectKnown, pending = false, true, false, nil
			directAt, candidateAt, terminalAt = false, false, false
		} else if lanePoisonAt && pending != nil {
			pending.RestartBlocked = true
		}

		// A global point is a cross-lane barrier, not a permanent global taint.
		// It invalidates a same-time lifecycle transition. Strictly interior
		// points were handled by the binary-search proposal guard above.
		if globalPoisonAt {
			if directAt || candidateAt || terminalAt {
				builder.invalidateGeneration(tid, next)
				delete(lane.cuts, next)
				live, unknown, subjectKnown, pending = false, true, false, nil
				directAt, candidateAt, terminalAt = false, false, false
			}
		}

		if directAt || candidateAt {
			var cut traceDBLifecycleBoundary
			if directAt {
				cut = direct[directPos]
			} else {
				cut = pending.Restart
			}
			if directAt && candidateAt && (cut.NewITID != pending.Restart.NewITID || cut.NewIPID != pending.Restart.NewIPID) {
				builder.invalidateGeneration(tid, next)
				delete(lane.cuts, next)
				live, unknown, subjectKnown, pending = false, true, false, nil
				terminalAt = false
			} else {
				lane.cuts[next] = cut
				live, unknown, subjectKnown, subject, pending = true, false, true, cut, nil
			}
		}

		if terminalAt {
			terminal := lane.terminals[terminalPos]
			switch {
			case terminal.Conflicted:
				builder.invalidateGeneration(tid, next)
				live, unknown, subjectKnown, pending = false, true, false, nil
			case unknown:
				// A terminal is exact as a row, but cannot prove that it belongs
				// to the current generation once that generation is Unknown.
				// Only an independent direct begin may restore the lane.
			case pending != nil || !live:
				if subjectKnown && (terminal.Activity.NewITID != subject.NewITID || terminal.Activity.NewIPID != subject.NewIPID) {
					builder.invalidateGeneration(tid, next)
					live, unknown, subjectKnown, pending = false, true, false, nil
				} else {
					// A later exact terminal replaces a blocked/older anchor. This
					// permits a fresh (terminal,activity] proof without reviving the
					// already dead generation at the terminal itself.
					pending = terminal
				}
			case subjectKnown && (terminal.Activity.NewITID != subject.NewITID || terminal.Activity.NewIPID != subject.NewIPID):
				builder.invalidateGeneration(tid, next)
				live, unknown, subjectKnown, pending = false, true, false, nil
			default:
				lane.effectiveEnds = append(lane.effectiveEnds, terminal.Activity)
				live, unknown, subjectKnown, subject, pending = false, false, true, terminal.Activity, terminal
			}
		}

		if directPos < len(direct) && direct[directPos].TS == next {
			directPos++
		}
		if terminalPos < len(lane.terminals) && lane.terminals[terminalPos].TS == next {
			terminalPos++
		}
		if poisonPos < len(lanePoison) && lanePoison[poisonPos] == next {
			poisonPos++
		}
	}
}

func sortedTraceDBLifecycleCuts(cuts map[int64]traceDBLifecycleBoundary) []traceDBLifecycleBoundary {
	out := make([]traceDBLifecycleBoundary, 0, len(cuts))
	for _, cut := range cuts {
		out = append(out, cut)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TS < out[j].TS })
	return out
}

func freezeTraceDBLifecycleLane(build *traceDBLifecycleBuildLane) traceDBLifecycleLane {
	for timestamp := range build.poison {
		delete(build.cuts, timestamp)
	}
	lane := traceDBLifecycleLane{
		Cuts:          make([]traceDBLifecycleBoundary, 0, len(build.cuts)),
		Terminals:     append([]traceDBLifecycleBoundary(nil), build.effectiveEnds...),
		PoisonPoints:  sortedTraceDBLifecyclePoints(build.poison),
		UnknownStarts: sortedTraceDBLifecyclePoints(build.unknown),
		Tainted:       build.tainted,
	}
	for _, cut := range build.cuts {
		lane.Cuts = append(lane.Cuts, cut)
	}
	sort.Slice(lane.Cuts, func(i, j int) bool { return lane.Cuts[i].TS < lane.Cuts[j].TS })
	return lane
}

func sortedTraceDBLifecyclePoints(points map[int64]bool) []int64 {
	out := make([]int64, 0, len(points))
	for point := range points {
		out = append(out, point)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func traceDBLifecycleTIDMayBeMain(index traceDBThreadIndex, tid int64) bool {
	for _, thread := range index.ByTIDCandidates[tid] {
		process, ok := index.Processes[thread.IPID]
		if ok && thread.IsMainThread && process.PID == tid && thread.TID == process.PID {
			return true
		}
	}
	return false
}

func traceDBLifecycleThreadPointAllows(lifecycle traceDBLifecycleIndex, identities traceDBThreadIndex, itid, timestamp int64) bool {
	if timestamp < 0 || lifecycle.GlobalTaint || traceDBLifecycleHasPoint(lifecycle.GlobalPoison, timestamp) {
		return false
	}
	thread, ok := identities.ByITID[itid]
	if itid <= 0 || !ok || identities.AmbiguousITID[itid] || thread.TID <= 0 {
		return false
	}
	return traceDBLifecycleLanePointAllows(lifecycle.ByTID[thread.TID], timestamp, itid, false)
}

func traceDBLifecycleProcessPointAllows(lifecycle traceDBLifecycleIndex, identities traceDBThreadIndex, ipid, timestamp int64) bool {
	if timestamp < 0 || lifecycle.GlobalTaint || traceDBLifecycleHasPoint(lifecycle.GlobalPoison, timestamp) {
		return false
	}
	process, ok := identities.Processes[ipid]
	if !ok || identities.AmbiguousIPID[ipid] || process.PID <= 0 {
		return false
	}
	return traceDBLifecycleLanePointAllows(lifecycle.ByPID[process.PID], timestamp, ipid, true)
}

func traceDBLifecycleLanePointAllows(lane traceDBLifecycleLane, timestamp, identity int64, process bool) bool {
	if lane.Tainted || traceDBLifecycleHasPoint(lane.PoisonPoints, timestamp) {
		return false
	}
	cutPosition := sort.Search(len(lane.Cuts), func(i int) bool { return lane.Cuts[i].TS > timestamp }) - 1
	terminalPosition := sort.Search(len(lane.Terminals), func(i int) bool { return lane.Terminals[i].TS > timestamp }) - 1
	unknownPosition := sort.Search(len(lane.UnknownStarts), func(i int) bool { return lane.UnknownStarts[i] > timestamp }) - 1
	cutKnown := cutPosition >= 0
	terminalKnown := terminalPosition >= 0
	unknownKnown := unknownPosition >= 0
	cutTS := int64(-1)
	if cutKnown {
		cutTS = lane.Cuts[cutPosition].TS
	}
	if unknownKnown && (!cutKnown || lane.UnknownStarts[unknownPosition] >= cutTS) {
		return false
	}
	if terminalKnown && (!cutKnown || lane.Terminals[terminalPosition].TS >= cutTS) {
		terminal := lane.Terminals[terminalPosition]
		if timestamp != terminal.TS {
			return false
		}
		if process {
			return terminal.NewIPID == identity
		}
		return terminal.NewITID == identity
	}
	if !cutKnown {
		return true
	}
	if process {
		return lane.Cuts[cutPosition].NewIPID == identity
	}
	return lane.Cuts[cutPosition].NewITID == identity
}

func traceDBLifecycleThreadSourceIntervalAllows(lifecycle traceDBLifecycleIndex, identities traceDBThreadIndex, itid, start, end int64) bool {
	return traceDBLifecycleThreadIntervalAllows(lifecycle, identities, itid, start, end, false)
}

func traceDBLifecycleThreadClosedEndpointAllows(lifecycle traceDBLifecycleIndex, identities traceDBThreadIndex, itid, start, end int64) bool {
	return traceDBLifecycleThreadIntervalAllows(lifecycle, identities, itid, start, end, true)
}

func traceDBLifecycleThreadIntervalAllows(lifecycle traceDBLifecycleIndex, identities traceDBThreadIndex, itid, start, end int64, closedEnd bool) bool {
	if end <= start || !traceDBLifecycleThreadPointAllows(lifecycle, identities, itid, start) ||
		closedEnd && !traceDBLifecycleThreadPointAllows(lifecycle, identities, itid, end) {
		return false
	}
	thread := identities.ByITID[itid]
	return traceDBLifecycleIntervalAllows(lifecycle, lifecycle.ByTID[thread.TID], start, end, closedEnd)
}

func traceDBLifecycleProcessSourceIntervalAllows(lifecycle traceDBLifecycleIndex, identities traceDBThreadIndex, ipid, start, end int64) bool {
	return traceDBLifecycleProcessIntervalAllows(lifecycle, identities, ipid, start, end, false)
}

func traceDBLifecycleProcessClosedEndpointAllows(lifecycle traceDBLifecycleIndex, identities traceDBThreadIndex, ipid, start, end int64) bool {
	return traceDBLifecycleProcessIntervalAllows(lifecycle, identities, ipid, start, end, true)
}

func traceDBLifecycleProcessIntervalAllows(lifecycle traceDBLifecycleIndex, identities traceDBThreadIndex, ipid, start, end int64, closedEnd bool) bool {
	if end <= start || !traceDBLifecycleProcessPointAllows(lifecycle, identities, ipid, start) ||
		closedEnd && !traceDBLifecycleProcessPointAllows(lifecycle, identities, ipid, end) {
		return false
	}
	process := identities.Processes[ipid]
	return traceDBLifecycleIntervalAllows(lifecycle, lifecycle.ByPID[process.PID], start, end, closedEnd)
}

func traceDBLifecycleIntervalAllows(lifecycle traceDBLifecycleIndex, lane traceDBLifecycleLane, start, end int64, closedEnd bool) bool {
	if lifecycle.GlobalTaint || lane.Tainted || traceDBLifecycleRangeHasPoint(lifecycle.GlobalPoison, start, end, closedEnd) ||
		traceDBLifecycleRangeHasPoint(lane.PoisonPoints, start, end, closedEnd) ||
		traceDBLifecycleRangeHasPoint(lane.UnknownStarts, start, end, closedEnd) ||
		traceDBLifecycleRangeHasTerminal(lane.Terminals, start, end) {
		return false
	}
	position := sort.Search(len(lane.Cuts), func(i int) bool { return lane.Cuts[i].TS > start })
	if position >= len(lane.Cuts) {
		return true
	}
	if closedEnd {
		return lane.Cuts[position].TS > end
	}
	return lane.Cuts[position].TS >= end
}

func traceDBLifecycleRangeHasTerminal(terminals []traceDBLifecycleBoundary, start, end int64) bool {
	position := sort.Search(len(terminals), func(i int) bool { return terminals[i].TS >= start })
	return position < len(terminals) && terminals[position].TS < end
}

func traceDBLifecycleHasPoint(points []int64, timestamp int64) bool {
	position := sort.Search(len(points), func(i int) bool { return points[i] >= timestamp })
	return position < len(points) && points[position] == timestamp
}

func traceDBLifecycleRangeHasPoint(points []int64, start, end int64, closedEnd bool) bool {
	position := sort.Search(len(points), func(i int) bool { return points[i] >= start })
	if position >= len(points) {
		return false
	}
	if closedEnd {
		return points[position] <= end
	}
	return points[position] < end
}

// traceDBLifecycleOpenRangeHasPoint checks the strict interior (start,end).
// Exact end collisions are handled as transition conflicts by the caller.
func traceDBLifecycleOpenRangeHasPoint(points []int64, start, end int64) bool {
	position := sort.Search(len(points), func(i int) bool { return points[i] > start })
	return position < len(points) && points[position] < end
}
