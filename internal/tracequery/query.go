package tracequery

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

func Run(idx *Index, q Query) Result {
	q = normalizeQuery(idx, q)
	flavor, confidence, signals, flavorCaveats := resolveTraceFlavor(idx, q)
	q.TraceFlavor = flavor
	res := Result{
		View:              q.View,
		SourcePath:        idx.Path,
		TraceFlavor:       string(flavor),
		FlavorConfidence:  confidence,
		FlavorSignals:     signals,
		TimeUnit:          "seconds",
		PrioritySemantics: PrioritySemanticsForFlavor(flavor),
		LineCount:         idx.LineCount,
		EventCount:        len(idx.Events),
		TimeStart:         q.TimeStart,
		TimeEnd:           q.TimeEnd,
	}
	switch q.View {
	case "thread_timeline":
		tl := ThreadTimeline(idx, q)
		res.Timeline = &tl
		res.EvidencePack = evidenceFromTimeline(tl)
	case "window_stats":
		stats := ComputeWindowStats(idx, q)
		res.WindowStats = &stats
		res.EvidencePack = evidenceFromStats(stats)
	case "ipc_graph":
		ipc := BuildIPCGraph(idx, q)
		res.IPCGraph = &ipc
		res.EvidencePack = evidenceFromIPCGraph(ipc)
	case "wakeup_chain":
		chain := BuildWakeupChain(idx, q)
		res.WakeupChain = &chain
		if q.IncludeWindowStats {
			stats := ComputeWindowStats(idx, q)
			res.WindowStats = &stats
		}
		res.EvidencePack = append(evidenceFromChain(chain), evidenceFromIPCGraph(IPCGraphResult{Edges: chain.IPCEdges})...)
	case "evidence_pack":
		chain := BuildWakeupChain(idx, q)
		stats := ComputeWindowStats(idx, q)
		ipc := BuildIPCGraph(idx, q)
		res.WakeupChain = &chain
		res.WindowStats = &stats
		res.IPCGraph = &ipc
		res.EvidencePack = append(evidenceFromChain(chain), evidenceFromStats(stats)...)
		res.EvidencePack = append(res.EvidencePack, evidenceFromIPCGraph(ipc)...)
	default:
		res.View = "event_search"
		res.Events = EventSearch(idx, q)
		res.EvidencePack = evidenceFromEvents(res.Events)
	}
	res.Caveats = append(res.Caveats, flavorCaveats...)
	res.Caveats = append(res.Caveats, resultCaveats(idx, q, res)...)
	return res
}

func resolveTraceFlavor(idx *Index, q Query) (TraceFlavor, float64, []string, []string) {
	detected := TraceFlavorGenericFtrace
	confidence := 0.50
	var signals []string
	if idx != nil {
		if idx.TraceFlavor != "" {
			detected = idx.TraceFlavor
		}
		if idx.FlavorConfidence > 0 {
			confidence = idx.FlavorConfidence
		}
		signals = append(signals, idx.FlavorSignals...)
	}
	hint := q.TraceFlavorHint
	hintSource := q.TraceFlavorHintSource
	if (hint == "" || hint == TraceFlavorAuto) && q.TraceFlavor != "" && q.TraceFlavor != TraceFlavorAuto && q.TraceFlavor != TraceFlavorGenericFtrace {
		hint = q.TraceFlavor
		hintSource = "query"
	}
	if hint == "" || hint == TraceFlavorAuto {
		return detected, confidence, signals, nil
	}
	signals = append(signals, "flavor_hint_"+string(hint))
	switch hintSource {
	case "tool_param":
		caveats := []string{"trace flavor was selected from explicit trace_query parameter"}
		if detected != "" && detected != TraceFlavorGenericFtrace && confidence >= 0.75 && detected != hint {
			caveats = append(caveats, fmt.Sprintf("explicit trace flavor %s conflicts with content-detected %s (confidence %.2f); using explicit trace_query parameter and preserving detection signals for audit", hint, detected, confidence))
		}
		return hint, 1.0, signals, caveats
	default:
		if detected == TraceFlavorGenericFtrace || confidence < 0.75 || detected == hint {
			if confidence < 0.85 {
				confidence = 0.85
			}
			return hint, confidence, signals, nil
		}
		return detected, confidence, signals, []string{"attached trace source hint conflicted with stronger content signals; using content-detected trace flavor"}
	}
}

func normalizeQuery(idx *Index, q Query) Query {
	q.View = strings.TrimSpace(q.View)
	if q.View == "" {
		q.View = "event_search"
	}
	if strings.TrimSpace(q.ThreadInput) == "" {
		q.ThreadInput = q.Thread
	}
	if sel := parseThreadSelector(q.ThreadInput); strings.TrimSpace(sel.Raw) != "" {
		if q.PID <= 0 && sel.HasPID {
			q.PID = sel.PID
			q.ThreadPIDInferred = true
		}
		if sel.Name != "" {
			q.Thread = sel.Name
		} else if q.PID > 0 {
			q.Thread = ""
		}
	}
	if q.MaxDepth <= 0 {
		q.MaxDepth = 6
	}
	if q.MaxBranches <= 0 {
		q.MaxBranches = 8
	}
	if q.MinDurationMs <= 0 {
		q.MinDurationMs = 1
	}
	if q.Limit <= 0 {
		q.Limit = 40
	}
	if q.TimeStart == 0 && q.LineStart == 0 && idx != nil {
		q.TimeStart = idx.FirstTs
	}
	if q.TimeEnd == 0 && q.LineEnd == 0 && idx != nil {
		q.TimeEnd = idx.LastTs
	}
	if q.View == "wakeup_chain" && !q.IncludeWindowStats {
		q.IncludeWindowStats = true
	}
	if q.TraceFlavor == "" {
		q.TraceFlavor = TraceFlavorGenericFtrace
	}
	return q
}

func ensureQueryFlavor(idx *Index, q Query) Query {
	if q.TraceFlavor != "" && q.TraceFlavor != TraceFlavorAuto {
		return q
	}
	if idx != nil && idx.TraceFlavor != "" {
		q.TraceFlavor = idx.TraceFlavor
	} else {
		q.TraceFlavor = TraceFlavorGenericFtrace
	}
	return q
}

func EventSearch(idx *Index, q Query) []EventView {
	if idx == nil {
		return nil
	}
	q = ensureQueryFlavor(idx, q)
	typeSet := make(map[EventType]bool, len(q.EventTypes))
	for _, t := range q.EventTypes {
		if t != "" {
			typeSet[t] = true
		}
	}
	var events []Event
	for _, ev := range idx.Events {
		if !eventInQuery(ev, q, typeSet) {
			continue
		}
		events = append(events, ev)
		if len(events) >= q.Limit {
			break
		}
	}
	raw := loadRawLines(idx.Path, eventLines(events))
	out := make([]EventView, 0, len(events))
	for _, ev := range events {
		ev = applyPriorityFlavor(ev, q.TraceFlavor)
		out = append(out, EventView{Event: ev, Raw: raw[ev.Line]})
	}
	return out
}

func eventInQuery(ev Event, q Query, typeSet map[EventType]bool) bool {
	if q.LineStart > 0 && ev.Line < q.LineStart {
		return false
	}
	if q.LineEnd > 0 && ev.Line > q.LineEnd {
		return false
	}
	if q.LineStart == 0 && q.LineEnd == 0 {
		if q.TimeStart > 0 && ev.Ts < q.TimeStart {
			return false
		}
		if q.TimeEnd > 0 && ev.Ts > q.TimeEnd {
			return false
		}
	}
	if len(typeSet) > 0 && !typeSet[ev.Type] {
		return false
	}
	if q.PID > 0 && !eventMentionsPID(ev, q.PID) {
		return false
	}
	if q.PID <= 0 && strings.TrimSpace(q.Thread) != "" && !eventMentionsThread(ev, q.Thread) {
		return false
	}
	return true
}

func eventMentionsPID(ev Event, pid int) bool {
	return ev.PID == pid || ev.TGID == pid || ev.PrevPID == pid || ev.NextPID == pid || ev.WakeePID == pid || ev.BinderDestProc == pid || ev.BinderDestThread == pid
}

func eventMentionsThread(ev Event, thread string) bool {
	sel := parseThreadSelector(thread)
	if sel.HasPID && eventMentionsPID(ev, sel.PID) {
		return true
	}
	for _, v := range []string{ev.Comm, ev.PrevComm, ev.NextComm, ev.WakeeComm, ev.SpanName, ev.SpanValue, ev.Reason, ev.FieldText} {
		if threadSelectorMatchesName(sel, v) {
			return true
		}
	}
	return false
}

func ThreadTimeline(idx *Index, q Query) TimelineResult {
	q = ensureQueryFlavor(idx, q)
	target := resolveThread(idx, q)
	res := TimelineResult{
		Thread: target,
		Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
	}
	if target.PID == 0 && target.Comm == "" {
		res.Caveats = append(res.Caveats, "target thread not found; provide pid or a thread name visible in the trace")
		return res
	}
	var runningStart float64
	var runningLine int
	var offStart float64
	var offLine int
	var offState string
	var wake *Event
	for _, ev := range idx.Events {
		if q.LineStart > 0 || q.LineEnd > 0 {
			if q.LineStart > 0 && ev.Line < q.LineStart {
				continue
			}
			if q.LineEnd > 0 && ev.Line > q.LineEnd {
				continue
			}
		}
		switch ev.Type {
		case EventSchedWakeup, EventSchedWaking:
			if threadMatches(target, ev.WakeePID, ev.WakeeComm) && offStart > 0 && ev.Ts >= offStart {
				copy := ev
				wake = &copy
			}
		case EventSchedSwitch:
			if threadMatches(target, ev.NextPID, ev.NextComm) {
				if offStart > 0 {
					res.Intervals = append(res.Intervals, offCPUIntervals(target, offStart, ev.Ts, offLine, ev.Line, offState, wake)...)
				}
				offStart, offLine, offState, wake = 0, 0, "", nil
				runningStart = ev.Ts
				runningLine = ev.Line
			}
			if threadMatches(target, ev.PrevPID, ev.PrevComm) {
				if runningStart > 0 {
					res.Intervals = append(res.Intervals, makeInterval(target, StateRunning, runningStart, ev.Ts, runningLine, ev.Line, ""))
				}
				runningStart = 0
				offStart = ev.Ts
				offLine = ev.Line
				offState = ev.PrevState
				wake = nil
			}
		}
	}
	if runningStart > 0 {
		res.Intervals = append(res.Intervals, makeInterval(target, StateRunning, runningStart, q.TimeEnd, runningLine, 0, ""))
	}
	if offStart > 0 {
		res.Intervals = append(res.Intervals, offCPUIntervals(target, offStart, q.TimeEnd, offLine, 0, offState, wake)...)
	}
	enrichBlockedReasonIntervals(idx, target, res.Intervals)
	res.Intervals = clampIntervals(res.Intervals, q)
	if len(res.Intervals) == 0 {
		res.Caveats = append(res.Caveats, "no scheduler interval for the target thread was found in the selected window")
	}
	return res
}

func enrichBlockedReasonIntervals(idx *Index, target ThreadRef, intervals []Interval) {
	for i := range intervals {
		if intervals[i].State != StateDSleep {
			continue
		}
		reason := findBlockedReasonFor(idx, target, intervals[i].StartTs, intervals[i].EndTs)
		if reason == nil {
			continue
		}
		if reason.IOWait > 0 {
			intervals[i].State = StateIOWait
		}
		intervals[i].Summary = fmt.Sprintf("%s for %.3f ms; sched_blocked_reason iowait=%d caller=%s", intervals[i].State, intervals[i].DurationMs, reason.IOWait, firstNonEmpty(reason.Reason, reason.FieldText, "unknown"))
		if intervals[i].EndLine == 0 {
			intervals[i].EndLine = reason.Line
		}
	}
}

func offCPUIntervals(thread ThreadRef, start, end float64, startLine, endLine int, prevState string, wake *Event) []Interval {
	if end <= start {
		return nil
	}
	state := stateFromPrevState(prevState)
	if wake != nil && wake.Ts > start && wake.Ts < end && state != StateRunnable {
		return []Interval{
			makeInterval(thread, state, start, wake.Ts, startLine, wake.Line, prevState),
			makeIntervalWithWake(thread, StateRunnable, wake.Ts, end, wake.Line, endLine, prevState, wake.Line),
		}
	}
	return []Interval{makeInterval(thread, state, start, end, startLine, endLine, prevState)}
}

func makeInterval(thread ThreadRef, state ThreadState, start, end float64, startLine, endLine int, prevState string) Interval {
	return makeIntervalWithWake(thread, state, start, end, startLine, endLine, prevState, 0)
}

func makeIntervalWithWake(thread ThreadRef, state ThreadState, start, end float64, startLine, endLine int, prevState string, wakeLine int) Interval {
	if end < start {
		end = start
	}
	return Interval{
		Thread:       thread,
		State:        state,
		StartTs:      start,
		EndTs:        end,
		DurationMs:   (end - start) * 1000,
		StartLine:    startLine,
		EndLine:      endLine,
		WakeupLine:   wakeLine,
		PrevStateRaw: prevState,
		Summary:      fmt.Sprintf("%s for %.3f ms", state, (end-start)*1000),
	}
}

func stateFromPrevState(prev string) ThreadState {
	prev = strings.TrimSpace(prev)
	switch {
	case strings.HasPrefix(prev, "D"):
		return StateDSleep
	case strings.HasPrefix(prev, "S"):
		return StateSSleep
	case strings.HasPrefix(prev, "R"):
		return StateRunnable
	default:
		return StateUnknown
	}
}

func clampIntervals(in []Interval, q Query) []Interval {
	var out []Interval
	for _, it := range in {
		if q.TimeStart > 0 && it.EndTs < q.TimeStart {
			continue
		}
		if q.TimeEnd > 0 && it.StartTs > q.TimeEnd {
			continue
		}
		if q.TimeStart > 0 && it.StartTs < q.TimeStart {
			it.StartTs = q.TimeStart
		}
		if q.TimeEnd > 0 && it.EndTs > q.TimeEnd {
			it.EndTs = q.TimeEnd
		}
		it.DurationMs = (it.EndTs - it.StartTs) * 1000
		if it.DurationMs >= 0 {
			out = append(out, it)
		}
	}
	return out
}

func resolveThread(idx *Index, q Query) ThreadRef {
	if q.PID > 0 {
		name := ""
		tgid := 0
		for _, ev := range idx.Events {
			if ev.PID == q.PID {
				name, tgid = ev.Comm, ev.TGID
				break
			}
			if ev.PrevPID == q.PID {
				name = ev.PrevComm
				break
			}
			if ev.NextPID == q.PID {
				name = ev.NextComm
				break
			}
			if ev.WakeePID == q.PID {
				name = ev.WakeeComm
				break
			}
		}
		return ThreadRef{Comm: name, PID: q.PID, TGID: tgid}
	}
	sel := parseThreadSelector(firstNonEmpty(q.ThreadInput, q.Thread))
	if sel.HasPID {
		q.PID = sel.PID
		return resolveThread(idx, q)
	}
	if strings.TrimSpace(sel.Raw) == "" && strings.TrimSpace(sel.Name) == "" {
		return ThreadRef{}
	}
	for _, ev := range idx.Events {
		for _, candidate := range []struct {
			comm string
			pid  int
			tgid int
		}{{ev.Comm, ev.PID, ev.TGID}, {ev.PrevComm, ev.PrevPID, 0}, {ev.NextComm, ev.NextPID, 0}, {ev.WakeeComm, ev.WakeePID, 0}} {
			if candidate.pid > 0 && threadSelectorMatchesName(sel, candidate.comm) {
				return ThreadRef{Comm: candidate.comm, PID: candidate.pid, TGID: candidate.tgid}
			}
		}
	}
	return ThreadRef{Comm: q.Thread}
}

func threadMatches(ref ThreadRef, pid int, comm string) bool {
	if ref.PID > 0 && pid == ref.PID {
		return true
	}
	if ref.Comm != "" && comm != "" && strings.EqualFold(ref.Comm, comm) {
		return true
	}
	return false
}

func ComputeWindowStats(idx *Index, q Query) WindowStats {
	q = ensureQueryFlavor(idx, q)
	stats := WindowStats{
		Window:      TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
		EventCounts: map[EventType]int{},
	}
	byCPU := map[int][]Event{}
	freqByCPU := map[int][]Event{}
	blockedReasons := map[string]BlockedReasonSummary{}
	for _, ev := range idx.Events {
		if eventLineInWindow(ev, q) && ev.Type == EventCPUFrequency && ev.Frequency > 0 {
			if q.TimeEnd == 0 || ev.Ts <= q.TimeEnd {
				cpu := eventCPUForStats(ev)
				freqByCPU[cpu] = append(freqByCPU[cpu], ev)
			}
		}
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		stats.EventCounts[ev.Type]++
		switch ev.Type {
		case EventSchedSwitch:
			byCPU[ev.CPU] = append(byCPU[ev.CPU], ev)
		case EventBlockIssue:
			stats.BlockIssueCount++
		case EventBlockRemap:
			stats.BlockRemapCount++
		case EventBlockComplete:
			stats.BlockCompleteCount++
		case EventBinderTransaction:
			stats.BinderCount++
		case EventBinderReceived:
			stats.BinderCount++
			stats.BinderReceivedCount++
		case EventIRQ:
			stats.IRQCount++
		case EventMemory:
			stats.MemoryEventCount++
		case EventSchedBlockedReason:
			stats.BlockedReasonCount++
			if ev.IOWait > 0 {
				stats.IOWaitBlockedCount++
			}
			key := fmt.Sprintf("%d/%d/%s", ev.WakeePID, ev.IOWait, ev.Reason)
			br := blockedReasons[key]
			br.Thread = resolveBlockedReasonThread(idx, ev)
			br.IOWait = ev.IOWait
			br.Reason = ev.Reason
			br.Count++
			if br.Line == 0 || ev.Line < br.Line {
				br.Line = ev.Line
				br.Ts = ev.Ts
			}
			blockedReasons[key] = br
		}
	}
	running := map[string]ThreadDuration{}
	pressure := map[int]*cpuPressureAcc{}
	for cpu, events := range byCPU {
		sort.SliceStable(events, func(i, j int) bool { return events[i].Ts < events[j].Ts })
		var busy, idle float64
		for i, ev := range events {
			end := q.TimeEnd
			if i+1 < len(events) {
				end = events[i+1].Ts
			}
			start := ev.Ts
			if q.TimeStart > 0 && start < q.TimeStart {
				start = q.TimeStart
			}
			if q.TimeEnd > 0 && end > q.TimeEnd {
				end = q.TimeEnd
			}
			if end <= start {
				continue
			}
			dur := (end - start) * 1000
			if ev.NextPID == 0 || strings.Contains(strings.ToLower(ev.NextComm), "idle") {
				idle += dur
			} else {
				busy += dur
				freq := frequencyAt(freqByCPU[cpu], start)
				key := fmt.Sprintf("%d/%s/%d", ev.NextPID, ev.NextComm, cpu)
				td := running[key]
				td.Thread = ThreadRef{Comm: ev.NextComm, PID: ev.NextPID}
				td.DurationMs += dur
				td.CPU = cpu
				if freq > 0 {
					td.Frequency = freq
				}
				td.Priority = ev.NextPrio
				td.PriorityClass = classifyTracePriority(q.TraceFlavor, ev.NextPrio)
				if td.LineStart == 0 {
					td.LineStart = ev.Line
				}
				td.LineEnd = ev.Line
				running[key] = td
				acc := cpuPressure(pressure, cpu)
				acc.runningMs += dur
				acc.runningEvents++
				if isHighPriorityForPressure(q.TraceFlavor, ev.NextPrio, td.PriorityClass) {
					acc.highPriorityRunningMs += dur
				}
				accumulateThreadDuration(acc.running, td.Thread, dur, cpu, freq, ev.Line, ev.NextPrio, td.PriorityClass)
			}
		}
		stats.CPU = upsertCPUBusyIdle(stats.CPU, cpu, busy, idle)
	}
	for _, td := range running {
		stats.TopRunning = append(stats.TopRunning, td)
	}
	sort.SliceStable(stats.TopRunning, func(i, j int) bool { return stats.TopRunning[i].DurationMs > stats.TopRunning[j].DurationMs })
	if len(stats.TopRunning) > 8 {
		stats.TopRunning = stats.TopRunning[:8]
	}
	stats.CPU = applyCPUFrequencyResidency(stats.CPU, freqByCPU, q)
	stats.RunnableTop, stats.DStateTop, stats.CPUPressure = computeOffCPUStats(idx, q, freqByCPU, pressure)
	stats.IOLatencies = computeIOLatencies(idx, q, 8)
	stats.Caveats = append(stats.Caveats, ioPairingCaveats(idx, q)...)
	stats.BlockedReasons = topBlockedReasons(blockedReasons, 8)
	stats.TraceSpans, stats.TraceCounters = computeTraceMarks(idx, q, 8)
	stats.IRQBursts = computeIRQBursts(idx, q, 8)
	stats.MemoryKinds = computeMemoryKinds(idx, q, 8)
	stats.ThreadDrifts = detectThreadDrifts(idx, q, 8)
	for _, drift := range stats.ThreadDrifts {
		if drift.Caveat != "" {
			stats.Caveats = append(stats.Caveats, drift.Caveat)
		}
	}
	return stats
}

type cpuPressureAcc struct {
	runnableWaitMs        float64
	runnableEvents        int
	runningMs             float64
	runningEvents         int
	highPriorityRunningMs float64
	runnable              map[string]ThreadDuration
	running               map[string]ThreadDuration
}

func cpuPressure(in map[int]*cpuPressureAcc, cpu int) *cpuPressureAcc {
	if acc := in[cpu]; acc != nil {
		return acc
	}
	acc := &cpuPressureAcc{
		runnable: map[string]ThreadDuration{},
		running:  map[string]ThreadDuration{},
	}
	in[cpu] = acc
	return acc
}

func accumulateThreadDuration(bucket map[string]ThreadDuration, thread ThreadRef, dur float64, cpu int, freq int, line int, priority int, priorityClass string) {
	if dur <= 0 {
		return
	}
	key := fmt.Sprintf("%d/%s/%d", thread.PID, thread.Comm, cpu)
	td := bucket[key]
	td.Thread = thread
	td.DurationMs += dur
	td.CPU = cpu
	if freq > 0 {
		td.Frequency = freq
	}
	td.Priority = priority
	td.PriorityClass = priorityClass
	if td.LineStart == 0 {
		td.LineStart = line
	}
	td.LineEnd = line
	bucket[key] = td
}

type offCPUStart struct {
	thread        ThreadRef
	state         ThreadState
	ts            float64
	line          int
	cpu           int
	priority      int
	priorityClass string
}

func computeOffCPUStats(idx *Index, q Query, freqByCPU map[int][]Event, pressure map[int]*cpuPressureAcc) ([]ThreadDuration, []ThreadDuration, []CPUPressureStats) {
	if idx == nil {
		return nil, nil, nil
	}
	open := map[int]offCPUStart{}
	runnable := map[string]ThreadDuration{}
	dstate := map[string]ThreadDuration{}
	addDuration := func(bucket map[string]ThreadDuration, start offCPUStart, endTs float64, endLine int) {
		startTs := start.ts
		if q.TimeStart > 0 && startTs < q.TimeStart {
			startTs = q.TimeStart
		}
		if q.TimeEnd > 0 && endTs > q.TimeEnd {
			endTs = q.TimeEnd
		}
		if endTs <= startTs {
			return
		}
		dur := (endTs - startTs) * 1000
		freq := frequencyAt(freqByCPU[start.cpu], startTs)
		key := fmt.Sprintf("%d/%s/%d", start.thread.PID, start.thread.Comm, start.cpu)
		td := bucket[key]
		td.Thread = start.thread
		td.DurationMs += dur
		td.CPU = start.cpu
		if freq > 0 {
			td.Frequency = freq
		}
		td.Priority = start.priority
		td.PriorityClass = start.priorityClass
		if td.LineStart == 0 {
			td.LineStart = start.line
		}
		td.LineEnd = firstPositive(endLine, start.line)
		bucket[key] = td
		if start.state == StateRunnable {
			acc := cpuPressure(pressure, start.cpu)
			acc.runnableWaitMs += dur
			acc.runnableEvents++
			accumulateThreadDuration(acc.runnable, start.thread, dur, start.cpu, freq, firstPositive(endLine, start.line), start.priority, start.priorityClass)
		}
	}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || ev.Type != EventSchedSwitch {
			continue
		}
		if ev.NextPID > 0 {
			if start, ok := open[ev.NextPID]; ok {
				switch start.state {
				case StateRunnable:
					addDuration(runnable, start, ev.Ts, ev.Line)
				case StateDSleep, StateIOWait:
					addDuration(dstate, start, ev.Ts, ev.Line)
				}
				delete(open, ev.NextPID)
			}
		}
		if ev.PrevPID > 0 {
			state := stateFromPrevState(ev.PrevState)
			if state == StateRunnable || state == StateDSleep || state == StateIOWait {
				open[ev.PrevPID] = offCPUStart{
					thread:        ThreadRef{Comm: ev.PrevComm, PID: ev.PrevPID},
					state:         state,
					ts:            ev.Ts,
					line:          ev.Line,
					cpu:           ev.CPU,
					priority:      ev.PrevPrio,
					priorityClass: classifyTracePriority(q.TraceFlavor, ev.PrevPrio),
				}
			}
		}
	}
	if q.TimeEnd > 0 {
		for _, start := range open {
			switch start.state {
			case StateRunnable:
				addDuration(runnable, start, q.TimeEnd, 0)
			case StateDSleep, StateIOWait:
				addDuration(dstate, start, q.TimeEnd, 0)
			}
		}
	}
	return topThreadDurations(runnable, 8), topThreadDurations(dstate, 8), buildCPUPressureStats(pressure, 8)
}

func buildCPUPressureStats(in map[int]*cpuPressureAcc, max int) []CPUPressureStats {
	out := make([]CPUPressureStats, 0, len(in))
	for cpu, acc := range in {
		if acc == nil {
			continue
		}
		out = append(out, CPUPressureStats{
			CPU:                   cpu,
			RunnableWaitMs:        acc.runnableWaitMs,
			RunnableEvents:        acc.runnableEvents,
			RunningMs:             acc.runningMs,
			HighPriorityRunningMs: acc.highPriorityRunningMs,
			TopRunnable:           topThreadDurations(acc.runnable, max),
			TopRunning:            topThreadDurations(acc.running, max),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RunnableWaitMs != out[j].RunnableWaitMs {
			return out[i].RunnableWaitMs > out[j].RunnableWaitMs
		}
		return out[i].CPU < out[j].CPU
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func topThreadDurations(in map[string]ThreadDuration, max int) []ThreadDuration {
	out := make([]ThreadDuration, 0, len(in))
	for _, td := range in {
		out = append(out, td)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DurationMs > out[j].DurationMs })
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func topBlockedReasons(in map[string]BlockedReasonSummary, max int) []BlockedReasonSummary {
	out := make([]BlockedReasonSummary, 0, len(in))
	for _, br := range in {
		out = append(out, br)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IOWait != out[j].IOWait {
			return out[i].IOWait > out[j].IOWait
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Line < out[j].Line
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func resolveBlockedReasonThread(idx *Index, ev Event) ThreadRef {
	ref := ThreadRef{PID: ev.WakeePID}
	if idx == nil || ev.WakeePID == 0 {
		return ref
	}
	for _, candidate := range idx.Events {
		if candidate.PID == ev.WakeePID {
			ref.Comm = candidate.Comm
			ref.TGID = candidate.TGID
			return ref
		}
		if candidate.PrevPID == ev.WakeePID {
			ref.Comm = candidate.PrevComm
			return ref
		}
		if candidate.NextPID == ev.WakeePID {
			ref.Comm = candidate.NextComm
			return ref
		}
		if candidate.WakeePID == ev.WakeePID {
			ref.Comm = candidate.WakeeComm
			return ref
		}
	}
	return ref
}

func upsertCPUBusyIdle(in []CPUStats, cpu int, busy, idle float64) []CPUStats {
	for i := range in {
		if in[i].CPU == cpu {
			in[i].BusyMs += busy
			in[i].IdleMs += idle
			return in
		}
	}
	return append(in, CPUStats{CPU: cpu, BusyMs: busy, IdleMs: idle})
}

func eventCPUForStats(ev Event) int {
	if ev.CPUForFieldValid {
		return ev.CPUForField
	}
	return ev.CPU
}

func applyCPUFrequencyResidency(in []CPUStats, byCPU map[int][]Event, q Query) []CPUStats {
	for cpu, events := range byCPU {
		residency, latest := computeCPUFrequencyResidency(events, q)
		if latest == 0 && len(residency) == 0 {
			continue
		}
		found := false
		for i := range in {
			if in[i].CPU == cpu {
				if latest > 0 {
					in[i].Frequency = latest
				}
				in[i].FrequencyResidency = residency
				found = true
				break
			}
		}
		if !found {
			in = append(in, CPUStats{CPU: cpu, Frequency: latest, FrequencyResidency: residency})
		}
	}
	sort.SliceStable(in, func(i, j int) bool { return in[i].CPU < in[j].CPU })
	return in
}

func computeCPUFrequencyResidency(events []Event, q Query) ([]CPUFrequencyResidency, int) {
	if len(events) == 0 {
		return nil, 0
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Ts != events[j].Ts {
			return events[i].Ts < events[j].Ts
		}
		return events[i].Line < events[j].Line
	})
	collapsed := make([]Event, 0, len(events))
	for _, ev := range events {
		if len(collapsed) > 0 && ev.Ts == collapsed[len(collapsed)-1].Ts {
			collapsed[len(collapsed)-1] = ev
			continue
		}
		collapsed = append(collapsed, ev)
	}
	startWindow := q.TimeStart
	endWindow := q.TimeEnd
	if endWindow == 0 {
		endWindow = collapsed[len(collapsed)-1].Ts
	}
	var current *Event
	currentStart := startWindow
	var out []CPUFrequencyResidency
	for i := range collapsed {
		ev := collapsed[i]
		if ev.Ts < startWindow {
			copy := ev
			current = &copy
			currentStart = startWindow
			continue
		}
		if ev.Ts > endWindow {
			break
		}
		if current != nil {
			out = appendFrequencyResidency(out, *current, currentStart, ev.Ts, ev.Line)
		}
		copy := ev
		current = &copy
		currentStart = ev.Ts
	}
	if current != nil && endWindow > currentStart {
		out = appendFrequencyResidency(out, *current, currentStart, endWindow, 0)
	}
	latest := 0
	if current != nil {
		latest = current.Frequency
	}
	return out, latest
}

func frequencyAt(events []Event, ts float64) int {
	if len(events) == 0 {
		return 0
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Ts != events[j].Ts {
			return events[i].Ts < events[j].Ts
		}
		return events[i].Line < events[j].Line
	})
	freq := 0
	for _, ev := range events {
		if ev.Ts > ts {
			break
		}
		if ev.Frequency > 0 {
			freq = ev.Frequency
		}
	}
	return freq
}

func appendFrequencyResidency(in []CPUFrequencyResidency, ev Event, start, end float64, endLine int) []CPUFrequencyResidency {
	if ev.Frequency <= 0 || end <= start {
		return in
	}
	res := CPUFrequencyResidency{
		Frequency:  ev.Frequency,
		DurationMs: (end - start) * 1000,
		StartTs:    start,
		EndTs:      end,
		LineStart:  ev.Line,
		LineEnd:    firstPositive(endLine, ev.Line),
	}
	if len(in) > 0 && in[len(in)-1].Frequency == res.Frequency {
		in[len(in)-1].DurationMs += res.DurationMs
		in[len(in)-1].EndTs = res.EndTs
		in[len(in)-1].LineEnd = res.LineEnd
		return in
	}
	return append(in, res)
}

func computeIOLatencies(idx *Index, q Query, max int) []IOLatencySummary {
	if idx == nil {
		return nil
	}
	open := map[string][]Event{}
	var out []IOLatencySummary
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) {
			continue
		}
		if q.TimeEnd > 0 && ev.Ts > q.TimeEnd {
			break
		}
		switch ev.Type {
		case EventBlockIssue:
			key := blockKey(ev)
			if key == "" {
				continue
			}
			open[key] = append(open[key], ev)
		case EventBlockComplete:
			if q.TimeStart > 0 && ev.Ts < q.TimeStart {
				continue
			}
			key := blockKey(ev)
			queue := open[key]
			if key == "" || len(queue) == 0 {
				continue
			}
			issue := queue[0]
			open[key] = queue[1:]
			if ev.Ts < issue.Ts {
				continue
			}
			out = append(out, IOLatencySummary{
				Dev:            ev.BlockDev,
				Op:             ev.BlockOp,
				Sector:         ev.BlockSector,
				Len:            ev.BlockLen,
				IssueThread:    threadRefFromEvent(issue),
				CompleteThread: threadRefFromEvent(ev),
				IssueTs:        issue.Ts,
				CompleteTs:     ev.Ts,
				DurationMs:     (ev.Ts - issue.Ts) * 1000,
				IssueLine:      issue.Line,
				CompleteLine:   ev.Line,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DurationMs != out[j].DurationMs {
			return out[i].DurationMs > out[j].DurationMs
		}
		return out[i].IssueLine < out[j].IssueLine
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func ioPairingCaveats(idx *Index, q Query) []string {
	if idx == nil {
		return nil
	}
	issues := 0
	completes := 0
	missingIdentity := 0
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		switch ev.Type {
		case EventBlockIssue:
			issues++
			if blockKey(ev) == "" {
				missingIdentity++
			}
		case EventBlockComplete:
			completes++
			if blockKey(ev) == "" {
				missingIdentity++
			}
		}
	}
	var out []string
	if issues > 0 && completes == 0 {
		out = append(out, "block_rq_issue rows were present but no matching block_rq_complete rows appeared in the selected window; IO latency may extend outside the window")
	}
	if completes > 0 && issues == 0 {
		out = append(out, "block_rq_complete rows were present but issue rows were outside the selected window or unavailable; IO latency cannot be paired deterministically")
	}
	if missingIdentity > 0 {
		out = append(out, fmt.Sprintf("%d block IO row(s) lacked parseable device/sector/length identity and were excluded from latency pairing", missingIdentity))
	}
	return out
}

func blockKey(ev Event) string {
	if ev.BlockDev == "" || ev.BlockSector == 0 || ev.BlockLen == 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s/%d/%d", ev.BlockDev, ev.BlockOp, ev.BlockSector, ev.BlockLen)
}

func computeTraceMarks(idx *Index, q Query, max int) ([]TraceSpanSummary, []TraceCounterSummary) {
	if idx == nil {
		return nil, nil
	}
	stacks := map[int][]Event{}
	var spans []TraceSpanSummary
	counters := map[string]TraceCounterSummary{}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) || ev.Type != EventTraceMark {
			continue
		}
		switch ev.SpanAction {
		case "B":
			stacks[ev.PID] = append(stacks[ev.PID], ev)
		case "E":
			stack := stacks[ev.PID]
			if len(stack) == 0 {
				continue
			}
			start := stack[len(stack)-1]
			stacks[ev.PID] = stack[:len(stack)-1]
			if ev.Ts < start.Ts {
				continue
			}
			spans = append(spans, TraceSpanSummary{
				Thread:     threadRefFromEvent(start),
				Name:       start.SpanName,
				StartTs:    start.Ts,
				EndTs:      ev.Ts,
				DurationMs: (ev.Ts - start.Ts) * 1000,
				StartLine:  start.Line,
				EndLine:    ev.Line,
			})
		case "C":
			key := fmt.Sprintf("%d/%s/%s", ev.PID, ev.SpanName, ev.SpanValue)
			counter := counters[key]
			counter.Thread = threadRefFromEvent(ev)
			counter.Name = ev.SpanName
			counter.Value = ev.SpanValue
			counter.Count++
			if counter.Line == 0 || ev.Line < counter.Line {
				counter.Line = ev.Line
				counter.Ts = ev.Ts
			}
			counters[key] = counter
		}
	}
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].DurationMs != spans[j].DurationMs {
			return spans[i].DurationMs > spans[j].DurationMs
		}
		return spans[i].StartLine < spans[j].StartLine
	})
	if max > 0 && len(spans) > max {
		spans = spans[:max]
	}
	counterList := make([]TraceCounterSummary, 0, len(counters))
	for _, c := range counters {
		counterList = append(counterList, c)
	}
	sort.SliceStable(counterList, func(i, j int) bool {
		if counterList[i].Count != counterList[j].Count {
			return counterList[i].Count > counterList[j].Count
		}
		return counterList[i].Line < counterList[j].Line
	})
	if max > 0 && len(counterList) > max {
		counterList = counterList[:max]
	}
	return spans, counterList
}

func computeIRQBursts(idx *Index, q Query, max int) []IRQBurstSummary {
	if idx == nil {
		return nil
	}
	const burstGapSeconds = 0.001
	var bursts []IRQBurstSummary
	active := map[string]IRQBurstSummary{}
	flush := func(key string) {
		burst := active[key]
		if burst.Count > 0 {
			burst.DurationMs = (burst.EndTs - burst.StartTs) * 1000
			bursts = append(bursts, burst)
		}
		delete(active, key)
	}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		if ev.Type != EventIRQ {
			continue
		}
		name := firstNonEmpty(ev.IRQName, ev.Name, "irq")
		key := fmt.Sprintf("%d/%d/%s", ev.CPU, ev.IRQID, name)
		for existing, burst := range active {
			if existing != key && ev.Ts-burst.EndTs > burstGapSeconds {
				flush(existing)
			}
		}
		burst := active[key]
		if burst.Count > 0 && ev.Ts-burst.EndTs > burstGapSeconds {
			flush(key)
			burst = IRQBurstSummary{}
		}
		if burst.Count == 0 {
			burst = IRQBurstSummary{
				CPU:       ev.CPU,
				Name:      name,
				IRQ:       ev.IRQID,
				StartTs:   ev.Ts,
				EndTs:     ev.Ts,
				LineStart: ev.Line,
				LineEnd:   ev.Line,
			}
		}
		burst.Count++
		burst.EndTs = ev.Ts
		burst.LineEnd = ev.Line
		active[key] = burst
	}
	for key := range active {
		flush(key)
	}
	sort.SliceStable(bursts, func(i, j int) bool {
		if bursts[i].Count != bursts[j].Count {
			return bursts[i].Count > bursts[j].Count
		}
		if bursts[i].DurationMs != bursts[j].DurationMs {
			return bursts[i].DurationMs > bursts[j].DurationMs
		}
		return bursts[i].LineStart < bursts[j].LineStart
	})
	if max > 0 && len(bursts) > max {
		bursts = bursts[:max]
	}
	return bursts
}

func computeMemoryKinds(idx *Index, q Query, max int) []MemoryKindSummary {
	if idx == nil {
		return nil
	}
	byKind := map[string]MemoryKindSummary{}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) || ev.Type != EventMemory {
			continue
		}
		kind := firstNonEmpty(ev.MemoryKind, "memory")
		item := byKind[kind]
		item.Kind = kind
		item.Count++
		if item.Line == 0 || ev.Line < item.Line {
			item.Line = ev.Line
			item.Ts = ev.Ts
		}
		byKind[kind] = item
	}
	out := make([]MemoryKindSummary, 0, len(byKind))
	for _, item := range byKind {
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Line < out[j].Line
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

type threadDriftAcc struct {
	names     map[string]bool
	tgids     map[int]bool
	lineStart int
	lineEnd   int
}

func detectThreadDrifts(idx *Index, q Query, max int) []ThreadDriftSummary {
	if idx == nil {
		return nil
	}
	accs := map[int]*threadDriftAcc{}
	add := func(pid int, comm string, tgid int, line int) {
		if pid <= 0 || line <= 0 {
			return
		}
		acc := accs[pid]
		if acc == nil {
			acc = &threadDriftAcc{names: map[string]bool{}, tgids: map[int]bool{}, lineStart: line, lineEnd: line}
			accs[pid] = acc
		}
		if strings.TrimSpace(comm) != "" {
			acc.names[comm] = true
		}
		if tgid > 0 {
			acc.tgids[tgid] = true
		}
		if acc.lineStart == 0 || line < acc.lineStart {
			acc.lineStart = line
		}
		if line > acc.lineEnd {
			acc.lineEnd = line
		}
	}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		add(ev.PID, ev.Comm, ev.TGID, ev.Line)
		add(ev.PrevPID, ev.PrevComm, 0, ev.Line)
		add(ev.NextPID, ev.NextComm, 0, ev.Line)
		add(ev.WakeePID, ev.WakeeComm, 0, ev.Line)
	}
	var out []ThreadDriftSummary
	for pid, acc := range accs {
		names := sortedStringSet(acc.names)
		tgids := sortedIntSet(acc.tgids)
		if len(names) <= 1 && len(tgids) <= 1 {
			continue
		}
		out = append(out, ThreadDriftSummary{
			PID:       pid,
			Names:     names,
			TGIDs:     tgids,
			LineStart: acc.lineStart,
			LineEnd:   acc.lineEnd,
			Caveat:    fmt.Sprintf("pid=%d has multiple names/TGIDs in the selected window; treat cross-row thread identity as lower confidence unless line context confirms continuity", pid),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func sortedStringSet(in map[string]bool) []string {
	out := make([]string, 0, len(in))
	for s := range in {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func sortedIntSet(in map[int]bool) []int {
	out := make([]int, 0, len(in))
	for n := range in {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

func BuildWakeupChain(idx *Index, q Query) ChainResult {
	q = normalizeQuery(idx, q)
	target := resolveThread(idx, q)
	res := ChainResult{Target: target, Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}}
	if target.PID == 0 && target.Comm == "" {
		res.Caveats = append(res.Caveats, "target thread not found")
		return res
	}
	tq := q
	tq.PID = target.PID
	tq.Thread = target.Comm
	targetTimeline := ThreadTimeline(idx, tq)
	branches := interestingIntervals(targetTimeline.Intervals, q.MinDurationMs, q.MaxBranches)
	if len(branches) == 0 {
		visited := map[int]bool{}
		expandChain(idx, q, target, q.TimeStart, q.TimeEnd, 0, visited, &res, "")
		attachIPCGraphToChain(idx, q, &res)
		return res
	}
	for _, branch := range branches {
		visited := map[int]bool{}
		expandChain(idx, q, target, branch.StartTs, branch.EndTs, 0, visited, &res, "")
	}
	attachIPCGraphToChain(idx, q, &res)
	return res
}

func attachIPCGraphToChain(idx *Index, q Query, res *ChainResult) {
	if res == nil {
		return
	}
	ipc := BuildIPCGraph(idx, q)
	res.IPCEdges = ipc.Edges
	res.BinderWaits = findBinderWaitsForChain(*res, ipc.Edges)
	for _, wait := range res.BinderWaits {
		res.RootEvidence = append(res.RootEvidence, RootEvidence{
			Type:       "binder_wait",
			Thread:     wait.Thread,
			DurationMs: wait.DurationMs,
			LineStart:  firstPositive(wait.SendLine, wait.SleepLine),
			LineEnd:    firstPositive(wait.WakeupLine, wait.ReceiveLine, wait.SleepLine),
			Summary:    wait.Summary,
			Confidence: wait.Confidence,
		})
	}
	res.Caveats = append(res.Caveats, ipc.Caveats...)
}

func findBinderWaitsForChain(chain ChainResult, edges []IPCEdge) []BinderWaitSummary {
	if len(chain.Nodes) == 0 || len(edges) == 0 {
		return nil
	}
	var out []BinderWaitSummary
	seen := map[string]bool{}
	for _, node := range chain.Nodes {
		if node.Thread.PID == 0 {
			continue
		}
		if node.Dominant != StateSSleep && node.Dominant != StateDSleep && node.Dominant != StateIOWait {
			continue
		}
		for _, edge := range edges {
			if edge.Oneway {
				continue
			}
			if edge.Sender.PID != node.Thread.PID {
				continue
			}
			if edge.SendTs <= 0 || edge.SendTs > node.Window.StartTs {
				continue
			}
			if node.Window.StartTs-edge.SendTs > 0.100 {
				continue
			}
			key := fmt.Sprintf("%d/%d/%d", node.Thread.PID, edge.TransactionID, node.EvidenceLine)
			if seen[key] {
				continue
			}
			seen[key] = true
			confidence := edge.Confidence
			if confidence == 0 {
				confidence = 0.65
			}
			if edge.ReceiveLine == 0 {
				confidence *= 0.85
			}
			wait := BinderWaitSummary{
				Thread:        node.Thread,
				Peer:          edge.Receiver,
				TransactionID: edge.TransactionID,
				SendLine:      edge.SendLine,
				ReceiveLine:   edge.ReceiveLine,
				SleepLine:     node.EvidenceLine,
				SendTs:        edge.SendTs,
				SleepStartTs:  node.Window.StartTs,
				DurationMs:    node.DurationMs,
				Confidence:    confidence,
			}
			for _, w := range chain.Edges {
				if w.Wakee.PID == node.Thread.PID && w.WakeupTs >= node.Window.StartTs && w.WakeupTs <= node.Window.EndTs {
					wait.WakeupLine = w.WakeupLine
					wait.WakeupTs = w.WakeupTs
					break
				}
			}
			peer := tracePeerLabel(wait.Peer, edge)
			wait.Summary = fmt.Sprintf("%s sent synchronous-looking binder transaction", threadLabel(wait.Thread))
			if edge.TransactionID > 0 {
				wait.Summary = fmt.Sprintf("%s transaction=%d", wait.Summary, edge.TransactionID)
			}
			if peer != "" {
				wait.Summary = fmt.Sprintf("%s to %s", wait.Summary, peer)
			}
			wait.Summary = fmt.Sprintf("%s before %.3fms %s interval", wait.Summary, node.DurationMs, node.Dominant)
			if edge.ReceiveLine == 0 {
				wait.Caveats = append(wait.Caveats, "receiver row was not matched; binder wait is a scheduler-correlated candidate, not standalone proof")
			}
			out = append(out, wait)
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DurationMs != out[j].DurationMs {
			return out[i].DurationMs > out[j].DurationMs
		}
		return out[i].SendLine < out[j].SendLine
	})
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

func tracePeerLabel(peer ThreadRef, edge IPCEdge) string {
	if peer.PID > 0 || peer.Comm != "" {
		return threadLabel(peer)
	}
	if edge.DestThread > 0 {
		return fmt.Sprintf("dest_thread=%d", edge.DestThread)
	}
	if edge.DestProc > 0 {
		return fmt.Sprintf("dest_proc=%d", edge.DestProc)
	}
	return ""
}

func expandChain(idx *Index, q Query, thread ThreadRef, start, end float64, depth int, visited map[int]bool, res *ChainResult, parentID string) string {
	if depth >= q.MaxDepth {
		res.Caveats = append(res.Caveats, fmt.Sprintf("max_depth=%d reached at pid=%d", q.MaxDepth, thread.PID))
		return ""
	}
	if thread.PID > 0 && visited[thread.PID] {
		res.Caveats = append(res.Caveats, fmt.Sprintf("cycle detected at pid=%d", thread.PID))
		return ""
	}
	if thread.PID > 0 {
		visited[thread.PID] = true
		defer delete(visited, thread.PID)
	}
	tq := q
	tq.PID = thread.PID
	tq.Thread = thread.Comm
	tq.TimeStart = start
	tq.TimeEnd = end
	tl := ThreadTimeline(idx, tq)
	interesting := mostInterestingInterval(tl.Intervals, q.MinDurationMs)
	nodeID := fmt.Sprintf("n%d", len(res.Nodes)+1)
	node := ChainNode{ID: nodeID, Thread: thread, Window: TimeWindow{StartTs: start, EndTs: end}}
	if interesting != nil {
		node.Dominant = interesting.State
		node.DurationMs = interesting.DurationMs
		node.EvidenceLine = firstPositive(interesting.StartLine, interesting.WakeupLine, interesting.EndLine)
		node.Summary = interesting.Summary
	} else {
		node.Dominant = StateUnknown
		node.Summary = "no decisive scheduler interval found in aligned window"
	}
	res.Nodes = append(res.Nodes, node)
	if parentID != "" {
		// Edge is added by the caller once it knows the wakeup row.
	}
	if interesting == nil {
		res.RootEvidence = append(res.RootEvidence, RootEvidence{Type: "trace_gap", Thread: thread, Summary: node.Summary, Confidence: 0.6})
		return nodeID
	}
	switch interesting.State {
	case StateSSleep:
		wakeup := findWakeupFor(idx, thread, interesting.StartTs, interesting.EndTs)
		if wakeup == nil {
			res.RootEvidence = append(res.RootEvidence, RootEvidence{
				Type:       "missing_wakeup",
				Thread:     thread,
				DurationMs: interesting.DurationMs,
				LineStart:  interesting.StartLine,
				LineEnd:    interesting.EndLine,
				Summary:    "sleep interval has no matching sched_wakeup row in the selected trace window",
				Confidence: 0.7,
			})
			return nodeID
		}
		waker := ThreadRef{Comm: wakeup.Comm, PID: wakeup.PID, TGID: wakeup.TGID}
		childID := expandChain(idx, q, waker, interesting.StartTs, wakeup.Ts, depth+1, visited, res, nodeID)
		if childID != "" {
			res.Edges = append(res.Edges, WakeupEdge{
				From:         childID,
				To:           nodeID,
				Waker:        waker,
				Wakee:        thread,
				WakeupTs:     wakeup.Ts,
				WakeupLine:   wakeup.Line,
				LatencyMs:    (wakeup.Ts - interesting.StartTs) * 1000,
				EvidenceLine: wakeup.Line,
			})
		}
	case StateRunnable:
		res.RootEvidence = append(res.RootEvidence, RootEvidence{Type: "runnable_wait", Thread: thread, DurationMs: interesting.DurationMs, LineStart: interesting.StartLine, LineEnd: interesting.EndLine, Summary: "thread was runnable but not running; inspect CPU pressure and priority context", Confidence: 0.8})
	case StateDSleep, StateIOWait:
		rootType := "d_state_or_io_wait"
		summary := "thread slept in D state; IO or uninterruptible wait is a root-cause candidate"
		lineEnd := interesting.EndLine
		if interesting.State == StateIOWait {
			rootType = "io_wait"
		}
		if reason := findBlockedReasonFor(idx, thread, interesting.StartTs, interesting.EndTs); reason != nil {
			if reason.IOWait > 0 {
				rootType = "io_wait"
			}
			lineEnd = firstPositive(reason.Line, lineEnd)
			summary = fmt.Sprintf("thread slept in D state; sched_blocked_reason iowait=%d caller=%s", reason.IOWait, firstNonEmpty(reason.Reason, reason.FieldText, "unknown"))
		}
		res.RootEvidence = append(res.RootEvidence, RootEvidence{Type: rootType, Thread: thread, DurationMs: interesting.DurationMs, LineStart: interesting.StartLine, LineEnd: lineEnd, Summary: summary, Confidence: 0.88})
	case StateRunning:
		res.RootEvidence = append(res.RootEvidence, RootEvidence{Type: "running", Thread: thread, DurationMs: interesting.DurationMs, LineStart: interesting.StartLine, LineEnd: interesting.EndLine, Summary: "thread was running in the aligned window; its own CPU work is root-cause evidence", Confidence: 0.75})
	default:
		res.RootEvidence = append(res.RootEvidence, RootEvidence{Type: "unknown_state", Thread: thread, DurationMs: interesting.DurationMs, LineStart: interesting.StartLine, LineEnd: interesting.EndLine, Summary: "thread state could not be classified from scheduler rows", Confidence: 0.5})
	}
	return nodeID
}

func mostInterestingInterval(intervals []Interval, minDurationMs float64) *Interval {
	candidates := interestingIntervals(intervals, minDurationMs, 1)
	if len(candidates) == 0 {
		return nil
	}
	return &candidates[0]
}

func interestingIntervals(intervals []Interval, minDurationMs float64, max int) []Interval {
	if max <= 0 {
		max = 1
	}
	var best *Interval
	score := func(s ThreadState) int {
		switch s {
		case StateIOWait:
			return 6
		case StateDSleep:
			return 5
		case StateSSleep:
			return 4
		case StateRunnable:
			return 3
		case StateRunning:
			return 2
		default:
			return 1
		}
	}
	var out []Interval
	for i := range intervals {
		if intervals[i].DurationMs < minDurationMs {
			continue
		}
		if intervals[i].State == StateRunning {
			continue
		}
		out = append(out, intervals[i])
	}
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := score(out[i].State), score(out[j].State)
		if si != sj {
			return si > sj
		}
		return out[i].DurationMs > out[j].DurationMs
	})
	if len(out) > max {
		out = out[:max]
	}
	if len(out) == 0 {
		for i := range intervals {
			if intervals[i].DurationMs >= minDurationMs {
				if best == nil || intervals[i].DurationMs > best.DurationMs {
					best = &intervals[i]
				}
			}
		}
		if best != nil {
			out = append(out, *best)
		}
	}
	return out
}

func findWakeupFor(idx *Index, thread ThreadRef, start, end float64) *Event {
	var best *Event
	for i := range idx.Events {
		ev := &idx.Events[i]
		if ev.Type != EventSchedWakeup && ev.Type != EventSchedWaking {
			continue
		}
		if ev.Ts < start || ev.Ts > end {
			continue
		}
		if threadMatches(thread, ev.WakeePID, ev.WakeeComm) {
			best = ev
		}
	}
	return best
}

func findBlockedReasonFor(idx *Index, thread ThreadRef, start, end float64) *Event {
	var best *Event
	for i := range idx.Events {
		ev := &idx.Events[i]
		if ev.Type != EventSchedBlockedReason {
			continue
		}
		if ev.Ts < start || ev.Ts > end {
			continue
		}
		if threadMatches(thread, ev.WakeePID, "") {
			best = ev
		}
	}
	return best
}

func timeInWindow(ts float64, q Query) bool {
	if q.TimeStart > 0 && ts < q.TimeStart {
		return false
	}
	if q.TimeEnd > 0 && ts > q.TimeEnd {
		return false
	}
	return true
}

func eventLineInWindow(ev Event, q Query) bool {
	if q.LineStart > 0 && ev.Line < q.LineStart {
		return false
	}
	if q.LineEnd > 0 && ev.Line > q.LineEnd {
		return false
	}
	return true
}

func eventLines(events []Event) []int {
	out := make([]int, 0, len(events))
	for _, ev := range events {
		if ev.Line > 0 {
			out = append(out, ev.Line)
		}
	}
	return out
}

func loadRawLines(path string, lines []int) map[int]string {
	out := map[int]string{}
	if len(lines) == 0 {
		return out
	}
	want := map[int]bool{}
	for _, n := range lines {
		want[n] = true
	}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufioNewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		if want[lineNo] {
			out[lineNo] = sc.Text()
			if len(out) == len(want) {
				break
			}
		}
	}
	return out
}

func bufioNewScanner(f *os.File) *bufio.Scanner {
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	return sc
}

func evidenceFromTimeline(tl TimelineResult) []EvidenceFact {
	var out []EvidenceFact
	for _, it := range tl.Intervals {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(it.Thread),
			Predicate:  string(it.State),
			Summary:    it.Summary,
			LineStart:  it.StartLine,
			LineEnd:    it.EndLine,
			StartTs:    it.StartTs,
			EndTs:      it.EndTs,
			Confidence: 0.75,
		})
		if len(out) >= 12 {
			break
		}
	}
	return out
}

func evidenceFromStats(stats WindowStats) []EvidenceFact {
	var out []EvidenceFact
	for _, td := range stats.TopRunning {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(td.Thread),
			Predicate:  "running_time",
			Summary:    fmt.Sprintf("%s ran for %.3f ms in the selected window", threadLabel(td.Thread), td.DurationMs),
			LineStart:  td.LineStart,
			LineEnd:    td.LineEnd,
			Confidence: 0.7,
		})
		if len(out) >= 8 {
			break
		}
	}
	for _, td := range stats.RunnableTop {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(td.Thread),
			Predicate:  "runnable_wait",
			Summary:    fmt.Sprintf("%s spent %.3f ms runnable but not running in the selected window%s", threadLabel(td.Thread), td.DurationMs, durationCPUDetail(td)),
			LineStart:  td.LineStart,
			LineEnd:    td.LineEnd,
			Confidence: 0.75,
		})
		if len(out) >= 12 {
			break
		}
	}
	for _, td := range stats.DStateTop {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(td.Thread),
			Predicate:  "d_state_or_io_wait",
			Summary:    fmt.Sprintf("%s spent %.3f ms in D-state or IO-like wait in the selected window%s", threadLabel(td.Thread), td.DurationMs, durationCPUDetail(td)),
			LineStart:  td.LineStart,
			LineEnd:    td.LineEnd,
			Confidence: 0.8,
		})
		if len(out) >= 16 {
			break
		}
	}
	for _, br := range stats.BlockedReasons {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(br.Thread),
			Predicate:  "blocked_reason",
			Summary:    fmt.Sprintf("%s sched_blocked_reason iowait=%d caller=%s (count=%d)", threadLabel(br.Thread), br.IOWait, firstNonEmpty(br.Reason, "unknown"), br.Count),
			LineStart:  br.Line,
			LineEnd:    br.Line,
			StartTs:    br.Ts,
			EndTs:      br.Ts,
			Confidence: 0.82,
		})
		if len(out) >= 20 {
			break
		}
	}
	for _, io := range stats.IOLatencies {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(io.IssueThread),
			Predicate:  "io_latency",
			Object:     threadLabel(io.CompleteThread),
			Summary:    fmt.Sprintf("block IO %s %s sector=%d len=%d took %.3f ms", io.Dev, io.Op, io.Sector, io.Len, io.DurationMs),
			LineStart:  io.IssueLine,
			LineEnd:    io.CompleteLine,
			StartTs:    io.IssueTs,
			EndTs:      io.CompleteTs,
			Confidence: 0.86,
		})
		if len(out) >= 24 {
			break
		}
	}
	for _, span := range stats.TraceSpans {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(span.Thread),
			Predicate:  "trace_span_duration",
			Object:     span.Name,
			Summary:    fmt.Sprintf("trace span %q on %s lasted %.3f ms", span.Name, threadLabel(span.Thread), span.DurationMs),
			LineStart:  span.StartLine,
			LineEnd:    span.EndLine,
			StartTs:    span.StartTs,
			EndTs:      span.EndTs,
			Confidence: 0.78,
		})
		if len(out) >= 28 {
			break
		}
	}
	for _, burst := range stats.IRQBursts {
		out = append(out, EvidenceFact{
			Subject:    fmt.Sprintf("cpu=%d", burst.CPU),
			Predicate:  "irq_burst",
			Object:     burst.Name,
			Summary:    fmt.Sprintf("IRQ burst %s irq=%d on cpu=%d had %d event(s) over %.3f ms", burst.Name, burst.IRQ, burst.CPU, burst.Count, burst.DurationMs),
			LineStart:  burst.LineStart,
			LineEnd:    burst.LineEnd,
			StartTs:    burst.StartTs,
			EndTs:      burst.EndTs,
			Confidence: 0.72,
		})
		if len(out) >= 32 {
			break
		}
	}
	for _, mem := range stats.MemoryKinds {
		out = append(out, EvidenceFact{
			Subject:    "memory",
			Predicate:  mem.Kind,
			Summary:    fmt.Sprintf("memory category %s appeared %d time(s) in the selected window", mem.Kind, mem.Count),
			LineStart:  mem.Line,
			LineEnd:    mem.Line,
			StartTs:    mem.Ts,
			EndTs:      mem.Ts,
			Confidence: 0.68,
		})
		if len(out) >= 36 {
			break
		}
	}
	return out
}

func durationCPUDetail(td ThreadDuration) string {
	parts := []string{}
	if td.CPU >= 0 {
		parts = append(parts, fmt.Sprintf("cpu=%d", td.CPU))
	}
	if td.Frequency > 0 {
		parts = append(parts, fmt.Sprintf("freq=%dkHz", td.Frequency))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, " ") + ")"
}

func evidenceFromChain(chain ChainResult) []EvidenceFact {
	var out []EvidenceFact
	for _, edge := range chain.Edges {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(edge.Waker),
			Predicate:  "wakes",
			Object:     threadLabel(edge.Wakee),
			Summary:    fmt.Sprintf("%s wakes %s at %.6f", threadLabel(edge.Waker), threadLabel(edge.Wakee), edge.WakeupTs),
			LineStart:  edge.WakeupLine,
			LineEnd:    edge.WakeupLine,
			StartTs:    edge.WakeupTs,
			EndTs:      edge.WakeupTs,
			Confidence: 0.85,
		})
	}
	for _, root := range chain.RootEvidence {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(root.Thread),
			Predicate:  root.Type,
			Summary:    root.Summary,
			LineStart:  root.LineStart,
			LineEnd:    root.LineEnd,
			Confidence: root.Confidence,
		})
	}
	for _, wait := range chain.BinderWaits {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(wait.Thread),
			Predicate:  "binder_wait",
			Object:     threadLabel(wait.Peer),
			Summary:    wait.Summary,
			LineStart:  firstPositive(wait.SendLine, wait.SleepLine),
			LineEnd:    firstPositive(wait.WakeupLine, wait.ReceiveLine, wait.SleepLine),
			StartTs:    wait.SendTs,
			EndTs:      firstPositiveFloat(wait.WakeupTs, wait.SleepStartTs),
			Confidence: wait.Confidence,
		})
	}
	return out
}

func evidenceFromIPCGraph(ipc IPCGraphResult) []EvidenceFact {
	var out []EvidenceFact
	for _, edge := range ipc.Edges {
		summary := fmt.Sprintf("%s sends binder transaction", threadLabel(edge.Sender))
		if edge.TransactionID > 0 {
			summary = fmt.Sprintf("%s transaction=%d", summary, edge.TransactionID)
		}
		if edge.Receiver.PID > 0 || edge.Receiver.Comm != "" {
			summary = fmt.Sprintf("%s to %s", summary, threadLabel(edge.Receiver))
		} else if edge.DestThread > 0 {
			summary = fmt.Sprintf("%s to dest_thread=%d", summary, edge.DestThread)
		}
		if edge.ReceiveLine > 0 {
			summary = fmt.Sprintf("%s; receive row matched at line %d", summary, edge.ReceiveLine)
		}
		out = append(out, EvidenceFact{
			Subject:    threadLabel(edge.Sender),
			Predicate:  "binder_ipc",
			Object:     threadLabel(edge.Receiver),
			Summary:    summary,
			LineStart:  firstPositive(edge.SendLine, edge.ReceiveLine),
			LineEnd:    firstPositive(edge.ReceiveLine, edge.SendLine),
			StartTs:    edge.SendTs,
			EndTs:      firstPositiveFloat(edge.ReceiveTs, edge.SendTs),
			Confidence: edge.Confidence,
		})
		if len(out) >= 16 {
			break
		}
	}
	return out
}

func evidenceFromEvents(events []EventView) []EvidenceFact {
	var out []EvidenceFact
	for _, ev := range events {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(ThreadRef{Comm: ev.Comm, PID: ev.PID, TGID: ev.TGID}),
			Predicate:  string(ev.Type),
			Summary:    strings.TrimSpace(ev.Raw),
			LineStart:  ev.Line,
			LineEnd:    ev.Line,
			StartTs:    ev.Ts,
			EndTs:      ev.Ts,
			Confidence: 0.7,
		})
	}
	return out
}

func resultCaveats(idx *Index, q Query, res Result) []string {
	var out []string
	if idx != nil && idx.ParsedKnown == 0 {
		out = append(out, "no known ftrace scheduler/resource events were parsed; the file may need a future parser adapter")
	}
	if selector := threadSelectorSummary(firstNonEmpty(q.ThreadInput, q.Thread)); selector != "" {
		if q.ThreadPIDInferred {
			out = append(out, "thread selector normalized from model/customer text: "+selector+"; pid-bearing scheduler fields are used for matching")
		}
	}
	if res.View == "event_search" && len(res.Events) == 0 {
		out = append(out, "matched_events=0 for the selected filters; this is not absence proof if the thread label, time window, event types, or line window are too narrow")
		if q.PID > 0 {
			out = append(out, fmt.Sprintf("next_call_hint=try trace_query(view=\"thread_timeline\", pid=%d, time_start=%.6f, time_end=%.6f) or trace_query(view=\"wakeup_chain\", pid=%d, time_start=%.6f, time_end=%.6f)", q.PID, q.TimeStart, q.TimeEnd, q.PID, q.TimeStart, q.TimeEnd))
		} else if strings.TrimSpace(q.Thread) != "" {
			out = append(out, fmt.Sprintf("next_call_hint=try trace_query(view=\"event_search\", thread=%q, time_start=%.6f, time_end=%.6f, event_types=[\"sched_switch\",\"sched_wakeup\"]) or use pid if visible in the trace row", q.Thread, q.TimeStart, q.TimeEnd))
		}
	}
	if q.LineStart > 0 || q.LineEnd > 0 {
		out = append(out, "line-window filtering was used; time-window statistics only cover parsed rows inside that line window")
	}
	return out
}

func threadLabel(t ThreadRef) string {
	switch {
	case t.Comm != "" && t.PID > 0:
		return fmt.Sprintf("%s-%d", t.Comm, t.PID)
	case t.Comm != "":
		return t.Comm
	case t.PID > 0:
		return fmt.Sprintf("pid=%d", t.PID)
	default:
		return "unknown-thread"
	}
}

func firstPositive(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func firstPositiveFloat(values ...float64) float64 {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}
