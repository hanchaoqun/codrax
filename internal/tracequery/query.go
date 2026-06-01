package tracequery

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

func Run(idx *Index, q Query) Result {
	explicitTimeStart := q.TimeStart != 0
	explicitTimeEnd := q.TimeEnd != 0
	q = normalizeQuery(idx, q)
	flavor, confidence, signals, flavorCaveats := resolveTraceFlavor(idx, q)
	q.TraceFlavor = flavor
	frameworkSurfaces := detectFrameworkSurfaces(idx, q, TracePlatformAuto, 4)
	platform, platformCandidate, platformCandidateConfidence, platformCandidateSignals, platformCaveats := resolveTracePlatform(idx, q, flavor, frameworkSurfaces, signals)
	if platform == TracePlatformDonghu && q.TraceFlavorHintSource == "" && q.TraceFlavorHint != TraceFlavorAndroidAtrace {
		flavor = TraceFlavorHarmonyHitrace
		q.TraceFlavor = flavor
		if confidence < platformCandidateConfidence {
			confidence = platformCandidateConfidence
		}
	}
	q.TracePlatform = platform
	spanWindows, spanCaveats := resolveSpanWindowsForQuery(idx, &q, explicitTimeStart, explicitTimeEnd)
	res := Result{
		View:                        q.View,
		SourcePath:                  idx.Path,
		TraceFlavor:                 string(flavor),
		Platform:                    string(platform),
		PlatformCandidate:           platformCandidate,
		PlatformCandidateConfidence: platformCandidateConfidence,
		PlatformCandidateSignals:    platformCandidateSignals,
		FlavorConfidence:            confidence,
		FlavorSignals:               signals,
		FrameworkMode:               FrameworkModeForPlatform(platform),
		FrameworkSurfaces:           frameworkSurfaces,
		TimeUnit:                    "seconds",
		PrioritySemantics:           PrioritySemanticsForFlavor(flavor),
		LineCount:                   idx.LineCount,
		EventCount:                  len(idx.Events),
		TimeStart:                   q.TimeStart,
		TimeEnd:                     q.TimeEnd,
	}
	if len(spanWindows) > 0 {
		res.SpanWindows = spanWindows
	}
	switch q.View {
	case "span_window":
		if len(spanWindows) == 0 {
			spanWindows, spanCaveats = FindSpanWindows(idx, q, q.Limit)
			res.SpanWindows = spanWindows
		}
		res.EvidencePack = evidenceFromSpans(spanWindows)
	case "thread_timeline":
		tl := ThreadTimeline(idx, q)
		res.Timeline = &tl
		res.EvidencePack = evidenceFromTimeline(tl)
	case "window_stats":
		stats := ComputeWindowStats(idx, q)
		res.WindowStats = &stats
		res.EvidencePack = evidenceFromStats(stats)
	case "scheduler_latency_stats":
		latency := BuildSchedulerLatencyStats(idx, q)
		res.SchedulerLatency = &latency
		res.EvidencePack = evidenceFromSchedulerLatency(latency)
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
	case "root_cause_rank":
		var chain ChainResult
		if q.PID > 0 || q.Thread != "" {
			chain = BuildWakeupChain(idx, q)
			res.WakeupChain = &chain
		}
		stats := ComputeWindowStats(idx, q)
		res.WindowStats = &stats
		rank := buildRootCauseRankFrom(q, chain, stats)
		latency := BuildSchedulerLatencyStats(idx, q)
		res.SchedulerLatency = &latency
		rank = enrichRootCauseRankWithScheduler(q, rank, latency, stats)
		res.RootCauseRank = &rank
		res.EvidencePack = evidenceFromRootCauseRank(rank)
	case "interaction_stats":
		interactions := BuildInteractionStats(idx, q)
		res.InteractionStats = &interactions
		res.EvidencePack = evidenceFromInteractionStats(interactions)
	case "frame_window", "render_pipeline":
		frame := BuildFramePipeline(idx, q)
		res.FramePipeline = &frame
		res.SpanWindows = frameSpans(frame)
		res.EvidencePack = evidenceFromFramePipeline(frame)
	case "frame_timeline", "frame_flow":
		timeline := BuildFrameTimeline(idx, q)
		res.FrameTimeline = &timeline
		res.SpanWindows = frameTimelineSpans(timeline)
		res.EvidencePack = evidenceFromFrameTimeline(timeline)
	case "critical_blocking_calls":
		blocking := BuildCriticalBlockingCalls(idx, q)
		res.CriticalBlocking = &blocking
		res.EvidencePack = evidenceFromCriticalBlocking(blocking)
	case "recipe":
		recipe := BuildRecipe(idx, q)
		res.Recipe = &recipe
		if recipeHasView(recipe, "window_stats") {
			stats := ComputeWindowStats(idx, q)
			res.WindowStats = &stats
		}
		if recipeHasView(recipe, "scheduler_latency_stats") {
			latency := BuildSchedulerLatencyStats(idx, q)
			res.SchedulerLatency = &latency
			res.EvidencePack = append(res.EvidencePack, evidenceFromSchedulerLatency(latency)...)
		}
		if recipeHasView(recipe, "wakeup_chain") {
			chain := BuildWakeupChain(idx, q)
			res.WakeupChain = &chain
			res.EvidencePack = append(res.EvidencePack, evidenceFromChain(chain)...)
		}
		if recipeHasView(recipe, "ipc_graph") {
			ipc := BuildIPCGraph(idx, q)
			res.IPCGraph = &ipc
			res.EvidencePack = append(res.EvidencePack, evidenceFromIPCGraph(ipc)...)
		}
		if recipeHasView(recipe, "root_cause_rank") {
			rank := BuildRootCauseRank(idx, q)
			res.RootCauseRank = &rank
			res.EvidencePack = append(res.EvidencePack, evidenceFromRootCauseRank(rank)...)
		}
		if recipeHasView(recipe, "critical_blocking_calls") {
			blocking := BuildCriticalBlockingCalls(idx, q)
			res.CriticalBlocking = &blocking
			res.EvidencePack = append(res.EvidencePack, evidenceFromCriticalBlocking(blocking)...)
		}
		if recipeHasView(recipe, "frame_window") || recipeHasView(recipe, "render_pipeline") {
			frame := BuildFramePipeline(idx, q)
			res.FramePipeline = &frame
			res.SpanWindows = frameSpans(frame)
			res.EvidencePack = append(res.EvidencePack, evidenceFromFramePipeline(frame)...)
		}
		if recipeHasView(recipe, "frame_timeline") || recipeHasView(recipe, "frame_flow") {
			timeline := BuildFrameTimeline(idx, q)
			res.FrameTimeline = &timeline
			res.EvidencePack = append(res.EvidencePack, evidenceFromFrameTimeline(timeline)...)
		}
	case "evidence_pack":
		chain := BuildWakeupChain(idx, q)
		stats := ComputeWindowStats(idx, q)
		ipc := BuildIPCGraph(idx, q)
		latency := BuildSchedulerLatencyStats(idx, q)
		blocking := BuildCriticalBlockingCalls(idx, q)
		res.WakeupChain = &chain
		res.WindowStats = &stats
		res.IPCGraph = &ipc
		res.SchedulerLatency = &latency
		res.CriticalBlocking = &blocking
		res.EvidencePack = append(evidenceFromChain(chain), evidenceFromStats(stats)...)
		res.EvidencePack = append(res.EvidencePack, evidenceFromIPCGraph(ipc)...)
		res.EvidencePack = append(res.EvidencePack, evidenceFromSchedulerLatency(latency)...)
		res.EvidencePack = append(res.EvidencePack, evidenceFromCriticalBlocking(blocking)...)
	default:
		res.View = "event_search"
		res.Events = EventSearch(idx, q)
		res.EvidencePack = evidenceFromEvents(res.Events)
	}
	res.Caveats = append(res.Caveats, flavorCaveats...)
	res.Caveats = append(res.Caveats, platformCaveats...)
	res.Caveats = append(res.Caveats, spanCaveats...)
	res.Caveats = append(res.Caveats, resultCaveats(idx, q, res)...)
	return res
}

func resolveTracePlatform(idx *Index, q Query, flavor TraceFlavor, surfaces []FrameworkSurface, flavorSignals []string) (TracePlatform, string, float64, []string, []string) {
	if q.TracePlatformHint != "" && q.TracePlatformHint != TracePlatformAuto {
		return q.TracePlatformHint, "", 0, nil, nil
	}
	if q.TracePlatform != "" && q.TracePlatform != TracePlatformAuto {
		return q.TracePlatform, "", 0, nil, nil
	}
	platform := PlatformForFlavor(flavor)
	if q.TraceFlavorHint == TraceFlavorAndroidAtrace && (q.TraceFlavorHintSource == "tool_param" || q.TraceFlavorHintSource == "user_request") {
		return platform, "", 0, nil, nil
	}
	candidate, confidence, signals := inferPlatformCandidate(idx, q, flavor, platform, surfaces, flavorSignals)
	var caveats []string
	if candidate == "mixed_harmony_base" && platform != TracePlatformDonghu {
		platform = TracePlatformDonghu
		caveats = append(caveats, "auto platform candidate mixed_harmony_base was selected from Harmony-base signals plus Android/Harmony framework surfaces; using Harmony/OpenHarmony timestamp and priority semantics")
	}
	return platform, candidate, confidence, signals, caveats
}

func inferPlatformCandidate(idx *Index, q Query, flavor TraceFlavor, platform TracePlatform, surfaces []FrameworkSurface, flavorSignals []string) (string, float64, []string) {
	if idx == nil {
		return "", 0, nil
	}
	signalSet := map[string]bool{}
	addSignal := func(s string) {
		if strings.TrimSpace(s) != "" {
			signalSet[s] = true
		}
	}
	for _, s := range flavorSignals {
		addSignal(s)
	}
	harmonyBase := flavor == TraceFlavorHarmonyHitrace || platform == TracePlatformHarmony || platform == TracePlatformDonghu
	androidSurface := false
	harmonySurface := false
	for _, surface := range surfaces {
		switch surface.Surface {
		case "android_framework":
			if surface.ProcessCount > 0 {
				androidSurface = true
				addSignal("surface_android_framework")
			}
		case "harmony_framework":
			if surface.ProcessCount > 0 {
				harmonySurface = true
				addSignal("surface_harmony_framework")
			}
		}
		for _, s := range surface.Signals {
			addSignal(s)
		}
	}
	for _, s := range flavorSignals {
		if strings.Contains(s, "harmony") || strings.Contains(s, "hitrace") || strings.Contains(s, "ffrt") || strings.Contains(s, "ohos") {
			harmonyBase = true
		}
	}
	if harmonyBase && androidSurface {
		confidence := 0.78
		if harmonySurface {
			confidence = 0.88
		}
		if flavor == TraceFlavorHarmonyHitrace {
			confidence += 0.04
		}
		if confidence > 0.96 {
			confidence = 0.96
		}
		signals := make([]string, 0, len(signalSet)+1)
		for s := range signalSet {
			signals = append(signals, s)
		}
		signals = append(signals, "platform_candidate_mixed_harmony_base")
		sort.Strings(signals)
		return "mixed_harmony_base", confidence, signals
	}
	return "", 0, nil
}

func detectFrameworkSurfaces(idx *Index, q Query, platform TracePlatform, limit int) []FrameworkSurface {
	if idx == nil || limit <= 0 {
		return nil
	}
	type acc struct {
		count    int
		examples []ThreadRef
		signals  map[string]bool
		seen     map[string]bool
	}
	surfaces := map[string]*acc{}
	add := func(surface string, ref ThreadRef, signal string) {
		if surface == "" || ref.PID <= 0 {
			return
		}
		a := surfaces[surface]
		if a == nil {
			a = &acc{signals: map[string]bool{}, seen: map[string]bool{}}
			surfaces[surface] = a
		}
		a.count++
		if signal != "" {
			a.signals[signal] = true
		}
		key := fmt.Sprintf("%d/%s", ref.PID, ref.Comm)
		if !a.seen[key] && len(a.examples) < limit {
			a.examples = append(a.examples, ref)
			a.seen[key] = true
		}
	}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		for _, ref := range []ThreadRef{
			{Comm: ev.Comm, PID: ev.PID, TGID: ev.TGID},
			{Comm: ev.PrevComm, PID: ev.PrevPID},
			{Comm: ev.NextComm, PID: ev.NextPID},
			{Comm: ev.WakeeComm, PID: ev.WakeePID},
		} {
			surface, signal := classifyFrameworkSurface(ref.Comm, "")
			add(surface, ref, signal)
		}
	}
	order := []string{"android_framework", "harmony_framework", "unknown"}
	out := make([]FrameworkSurface, 0, len(surfaces))
	for _, surface := range order {
		a := surfaces[surface]
		if a == nil {
			continue
		}
		signals := make([]string, 0, len(a.signals))
		for signal := range a.signals {
			signals = append(signals, signal)
		}
		sort.Strings(signals)
		out = append(out, FrameworkSurface{
			Surface:        surface,
			ProcessCount:   a.count,
			ExampleThreads: a.examples,
			Signals:        signals,
		})
	}
	return out
}

func classifyFrameworkSurface(comm, fields string) (surface, signal string) {
	text := strings.ToLower(strings.TrimSpace(comm + " " + fields))
	if text == "" {
		return "", ""
	}
	switch {
	case strings.Contains(text, "com.") || strings.Contains(text, "choreographer") ||
		strings.Contains(text, "surfaceflinger") || strings.Contains(text, "system_server") ||
		strings.Contains(text, "android."):
		return "android_framework", "android_process_marker"
	case strings.Contains(text, "os_ffrt") || strings.Contains(text, "ffrt") ||
		strings.Contains(text, "render_service") || strings.Contains(text, "rsunirender") ||
		strings.Contains(text, "foundation") || strings.Contains(text, "ohos") ||
		strings.Contains(text, "h:renderframe"):
		return "harmony_framework", "harmony_process_marker"
	default:
		return "", ""
	}
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
	if (hint == "" || hint == TraceFlavorAuto) && q.TracePlatformHint != "" && q.TracePlatformHint != TracePlatformAuto {
		hint = FlavorForPlatform(q.TracePlatformHint)
		hintSource = q.TracePlatformSource
	}
	if (hint == "" || hint == TraceFlavorAuto) && q.TraceFlavor != "" && q.TraceFlavor != TraceFlavorAuto && q.TraceFlavor != TraceFlavorGenericFtrace {
		hint = q.TraceFlavor
		hintSource = "query"
	}
	if hint == "" || hint == TraceFlavorAuto {
		return detected, confidence, signals, nil
	}
	signals = append(signals, "flavor_hint_"+string(hint))
	switch hintSource {
	case "tool_param", "user_request":
		caveatSource := "explicit trace_query parameter"
		if hintSource == "user_request" {
			caveatSource = "explicit user request"
		}
		caveats := []string{"trace flavor was selected from " + caveatSource}
		if detected != "" && detected != TraceFlavorGenericFtrace && confidence >= 0.75 && detected != hint {
			caveats = append(caveats, fmt.Sprintf("explicit trace flavor %s conflicts with content-detected %s (confidence %.2f); using %s and preserving detection signals for audit", hint, detected, confidence, caveatSource))
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
	q.RecipeName = strings.TrimSpace(q.RecipeName)
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
	if len(typeSet) > 0 && !eventTypeMatches(ev.Type, typeSet) {
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

func eventTypeMatches(typ EventType, typeSet map[EventType]bool) bool {
	if typeSet[typ] {
		return true
	}
	// Compatibility: older prompts use cpu_frequency to discover both concrete
	// frequency residency rows and frequency-limit rows. Residency math still
	// only consumes EventCPUFrequency.
	return typ == EventCPUFrequencyLimit && typeSet[EventCPUFrequency]
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
	freqLimits := map[int]CPUFrequencyLimit{}
	subsystems := map[string]SubsystemEventSummary{}
	bioResources := map[string]*RuntimeResourceSummary{}
	filesystemResources := map[string]*RuntimeResourceSummary{}
	pageFaultResources := map[string]*RuntimeResourceSummary{}
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
		case EventCPUFrequencyLimit:
			accumulateCPUFrequencyLimit(freqLimits, ev)
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
		case EventBinderAllocBuf, EventBinderLock, EventBinderLocked, EventBinderUnlock, EventBinderReply:
			stats.BinderAuxCount++
		case EventIRQ:
			stats.IRQCount++
		case EventSoftIRQ:
			stats.SoftIRQCount++
		case EventMemory:
			stats.MemoryEventCount++
		case EventStorage:
			stats.StorageEventCount++
		case EventFilesystem:
			stats.FilesystemEventCount++
		case EventPower:
			stats.PowerEventCount++
		case EventWorkqueue:
			stats.WorkqueueEventCount++
		case EventDMAFence:
			stats.DMAFenceEventCount++
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
		accumulateSubsystemEvent(subsystems, ev)
		accumulateRuntimeResource(bioResources, filesystemResources, pageFaultResources, ev)
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
	stats.CPUFrequencyLimits = sortedCPUFrequencyLimits(freqLimits, 8)
	stats.SubsystemEvents = sortedSubsystemEvents(subsystems, 12)
	stats.BIOResources = sortedRuntimeResources(bioResources, 8)
	stats.FilesystemResources = sortedRuntimeResources(filesystemResources, 8)
	stats.PageFaultResources = sortedRuntimeResources(pageFaultResources, 8)
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
	stats.ComputeSupply = computeSupplySummaries(stats, 8)
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

func BuildSchedulerLatencyStats(idx *Index, q Query) SchedulerLatencyResult {
	q = normalizeQuery(idx, q)
	q = ensureQueryFlavor(idx, q)
	res := SchedulerLatencyResult{Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}}
	if idx == nil {
		res.Caveats = append(res.Caveats, "trace index is empty")
		return res
	}
	var target ThreadRef
	if q.PID > 0 || q.Thread != "" || q.ThreadInput != "" {
		target = resolveThread(idx, q)
		res.Target = target
	}
	stats := ComputeWindowStats(idx, q)
	cpus := map[int]CPUStats{}
	for _, cpu := range stats.CPU {
		cpus[cpu.CPU] = cpu
	}
	pressure := map[int]CPUPressureStats{}
	for _, p := range stats.CPUPressure {
		pressure[p.CPU] = p
	}
	freqByCPU := map[int][]Event{}
	for _, ev := range idx.Events {
		if eventLineInWindow(ev, q) && ev.Type == EventCPUFrequency && ev.Frequency > 0 {
			if q.TimeEnd == 0 || ev.Ts <= q.TimeEnd {
				freqByCPU[eventCPUForStats(ev)] = append(freqByCPU[eventCPUForStats(ev)], ev)
			}
		}
	}
	type startInfo struct {
		thread        ThreadRef
		ts            float64
		line          int
		cpu           int
		priority      int
		priorityClass string
	}
	open := map[int]startInfo{}
	closeWait := func(pid int, endTs float64, endLine int) {
		start, ok := open[pid]
		if !ok {
			return
		}
		delete(open, pid)
		if target.PID > 0 || target.Comm != "" {
			if !threadMatches(target, start.thread.PID, start.thread.Comm) {
				return
			}
		}
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
		duration := (endTs - startTs) * 1000
		if duration < q.MinDurationMs {
			return
		}
		cpu := cpus[start.cpu]
		p := pressure[start.cpu]
		otherIdle := 0.0
		for cpuID, item := range cpus {
			if cpuID != start.cpu {
				otherIdle += item.IdleMs
			}
		}
		freq := frequencyAt(freqByCPU[start.cpu], startTs)
		item := SchedulerLatencyItem{
			Thread:                start.thread,
			StartTs:               startTs,
			EndTs:                 endTs,
			DurationMs:            duration,
			CPU:                   start.cpu,
			Frequency:             freq,
			Priority:              start.priority,
			PriorityClass:         start.priorityClass,
			StartLine:             start.line,
			EndLine:               firstPositive(endLine, start.line),
			SameCPUBusyMs:         cpu.BusyMs,
			SameCPUIdleMs:         cpu.IdleMs,
			OtherCPUIdleMs:        otherIdle,
			HighPriorityRunningMs: p.HighPriorityRunningMs,
			SameCPUTopRunning:     p.TopRunning,
		}
		item.Summary = fmt.Sprintf("%s waited runnable for %.3fms on cpu=%d", threadLabel(item.Thread), item.DurationMs, item.CPU)
		if item.Frequency > 0 {
			item.Summary = fmt.Sprintf("%s freq=%dkHz", item.Summary, item.Frequency)
		}
		if item.HighPriorityRunningMs > 0 {
			item.Summary = fmt.Sprintf("%s high_prio_running=%.3fms", item.Summary, item.HighPriorityRunningMs)
		}
		if item.OtherCPUIdleMs > 0 {
			item.Summary = fmt.Sprintf("%s other_cpu_idle=%.3fms", item.Summary, item.OtherCPUIdleMs)
		}
		res.Items = append(res.Items, item)
	}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || ev.Type != EventSchedSwitch {
			continue
		}
		if q.TimeEnd > 0 && ev.Ts > q.TimeEnd {
			break
		}
		if ev.NextPID > 0 {
			closeWait(ev.NextPID, ev.Ts, ev.Line)
		}
		if ev.PrevPID > 0 && stateFromPrevState(ev.PrevState) == StateRunnable {
			open[ev.PrevPID] = startInfo{
				thread:        ThreadRef{Comm: ev.PrevComm, PID: ev.PrevPID},
				ts:            ev.Ts,
				line:          ev.Line,
				cpu:           ev.CPU,
				priority:      ev.PrevPrio,
				priorityClass: classifyTracePriority(q.TraceFlavor, ev.PrevPrio),
			}
		}
	}
	if q.TimeEnd > 0 {
		for pid := range open {
			closeWait(pid, q.TimeEnd, 0)
		}
	}
	sort.SliceStable(res.Items, func(i, j int) bool {
		if res.Items[i].DurationMs != res.Items[j].DurationMs {
			return res.Items[i].DurationMs > res.Items[j].DurationMs
		}
		return res.Items[i].StartLine < res.Items[j].StartLine
	})
	durations := make([]float64, 0, len(res.Items))
	for _, item := range res.Items {
		durations = append(durations, item.DurationMs)
	}
	res.Count = len(res.Items)
	res.MeanMs = meanFloat64(durations)
	res.P50Ms = percentileFloat64(durations, 0.50)
	res.P95Ms = percentileFloat64(durations, 0.95)
	res.P99Ms = percentileFloat64(durations, 0.99)
	if len(durations) > 0 {
		res.MaxMs = durations[0]
	}
	limit := q.Limit
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	if len(res.Items) > limit {
		res.Caveats = append(res.Caveats, fmt.Sprintf("scheduler_latency_stats compacted from %d to %d runnable wait interval(s)", len(res.Items), limit))
		res.Items = res.Items[:limit]
	}
	if res.Count == 0 {
		res.Caveats = append(res.Caveats, "no runnable wait intervals matched the selected filters")
	}
	res.Caveats = append(res.Caveats, stats.Caveats...)
	return res
}

func meanFloat64(in []float64) float64 {
	if len(in) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range in {
		sum += v
	}
	return sum / float64(len(in))
}

func percentileFloat64(in []float64, p float64) float64 {
	if len(in) == 0 {
		return 0
	}
	cp := append([]float64(nil), in...)
	sort.Float64s(cp)
	if p <= 0 {
		return cp[0]
	}
	if p >= 1 {
		return cp[len(cp)-1]
	}
	idx := int(float64(len(cp)-1)*p + 0.999999)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
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

func computeSupplySummaries(stats WindowStats, max int) []ComputeSupplySummary {
	cpus := map[int]CPUStats{}
	for _, cpu := range stats.CPU {
		cpus[cpu.CPU] = cpu
	}
	pressure := map[int]CPUPressureStats{}
	for _, item := range stats.CPUPressure {
		pressure[item.CPU] = item
	}
	var out []ComputeSupplySummary
	add := func(thread ThreadRef, state string, duration float64, cpuID int, frequency int, lineStart, lineEnd int) {
		if duration <= 0 {
			return
		}
		cpu := cpus[cpuID]
		p := pressure[cpuID]
		verdict, conf := computeSupplyVerdict(duration, frequency, cpu, p)
		summary := fmt.Sprintf("%s %s for %.3fms on cpu=%d", threadLabel(thread), state, duration, cpuID)
		if frequency > 0 {
			summary = fmt.Sprintf("%s freq=%dkHz", summary, frequency)
		}
		if cpu.BusyMs > 0 || cpu.IdleMs > 0 {
			summary = fmt.Sprintf("%s busy=%.3fms idle=%.3fms", summary, cpu.BusyMs, cpu.IdleMs)
		}
		if p.HighPriorityRunningMs > 0 {
			summary = fmt.Sprintf("%s high_prio_running=%.3fms", summary, p.HighPriorityRunningMs)
		}
		out = append(out, ComputeSupplySummary{
			Thread:                thread,
			State:                 state,
			CPU:                   cpuID,
			DurationMs:            duration,
			Frequency:             frequency,
			CPUBusyMs:             cpu.BusyMs,
			CPUIdleMs:             cpu.IdleMs,
			RunnableWaitMs:        p.RunnableWaitMs,
			HighPriorityRunningMs: p.HighPriorityRunningMs,
			Verdict:               verdict,
			Confidence:            conf,
			LineStart:             lineStart,
			LineEnd:               lineEnd,
			Summary:               summary + " verdict=" + verdict,
		})
	}
	for _, td := range stats.RunnableTop {
		add(td.Thread, "runnable", td.DurationMs, td.CPU, td.Frequency, td.LineStart, td.LineEnd)
	}
	for _, td := range stats.TopRunning {
		add(td.Thread, "running", td.DurationMs, td.CPU, td.Frequency, td.LineStart, td.LineEnd)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		if out[i].DurationMs != out[j].DurationMs {
			return out[i].DurationMs > out[j].DurationMs
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func computeSupplyVerdict(durationMs float64, frequency int, cpu CPUStats, pressure CPUPressureStats) (string, float64) {
	total := cpu.BusyMs + cpu.IdleMs
	busyRatio := 0.0
	if total > 0 {
		busyRatio = cpu.BusyMs / total
	}
	lowFreq := frequency > 0 && frequencyIsLowForCPU(frequency, cpu)
	cpuPressure := busyRatio >= 0.80 || pressure.HighPriorityRunningMs >= durationMs*0.50
	switch {
	case cpuPressure && lowFreq:
		return "mixed_cpu_pressure_and_low_frequency", 0.78
	case cpuPressure:
		return "cpu_pressure", 0.76
	case lowFreq:
		return "low_frequency_signal", 0.68
	case cpu.IdleMs > cpu.BusyMs && durationMs > 0:
		return "idle_available_check_wakeup_or_affinity", 0.62
	default:
		return "insufficient_signal", 0.50
	}
}

func frequencyIsLowForCPU(frequency int, cpu CPUStats) bool {
	maxFreq := 0
	for _, res := range cpu.FrequencyResidency {
		if res.Frequency > maxFreq {
			maxFreq = res.Frequency
		}
	}
	if maxFreq <= 0 || frequency <= 0 || frequency >= maxFreq {
		return false
	}
	return float64(frequency) <= float64(maxFreq)*0.65
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

func resolveSpanWindowsForQuery(idx *Index, q *Query, explicitStart, explicitEnd bool) ([]TraceSpanSummary, []string) {
	if idx == nil || q == nil || strings.TrimSpace(q.SpanName) == "" {
		return nil, nil
	}
	spans, caveats := FindSpanWindows(idx, *q, q.Limit)
	if len(spans) == 0 {
		return nil, append(caveats, fmt.Sprintf("span_name=%q matched no complete B/E trace span in the selected filters", q.SpanName))
	}
	if explicitStart && explicitEnd {
		return spans, caveats
	}
	if len(spans) != 1 {
		return spans, append(caveats, fmt.Sprintf("span_name=%q matched %d span window(s); refine with pid/thread/line_start/line_end/time filters before deriving a root-cause window", q.SpanName, len(spans)))
	}
	span := spans[0]
	if !explicitStart {
		q.TimeStart = span.StartTs
	}
	if !explicitEnd {
		q.TimeEnd = span.EndTs
	}
	return spans, append(caveats, fmt.Sprintf("selected_window derived from unique trace span %q lines=%d-%d", span.Name, span.StartLine, span.EndLine))
}

func FindSpanWindows(idx *Index, q Query, max int) ([]TraceSpanSummary, []string) {
	if idx == nil {
		return nil, []string{"trace index is empty"}
	}
	if max <= 0 {
		max = 8
	}
	var target ThreadRef
	if q.PID > 0 || strings.TrimSpace(q.Thread) != "" || strings.TrimSpace(q.ThreadInput) != "" {
		target = resolveThread(idx, q)
	}
	stacks := map[int][]Event{}
	var spans []TraceSpanSummary
	for _, ev := range idx.Events {
		if ev.Type != EventTraceMark {
			continue
		}
		if !eventLineInWindow(ev, q) {
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
			span := TraceSpanSummary{
				Thread:     threadRefFromEvent(start),
				Name:       start.SpanName,
				StartTs:    start.Ts,
				EndTs:      ev.Ts,
				DurationMs: (ev.Ts - start.Ts) * 1000,
				StartLine:  start.Line,
				EndLine:    ev.Line,
			}
			if !traceSpanMatchesQuery(span, target, q) {
				continue
			}
			spans = append(spans, span)
		}
	}
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].StartTs != spans[j].StartTs {
			return spans[i].StartTs < spans[j].StartTs
		}
		return spans[i].StartLine < spans[j].StartLine
	})
	var caveats []string
	if len(spans) > max {
		caveats = append(caveats, fmt.Sprintf("span_window compacted from %d to %d span(s)", len(spans), max))
		spans = spans[:max]
	}
	if len(spans) == 0 {
		caveats = append(caveats, "no complete trace spans matched the selected filters")
	}
	return spans, caveats
}

func traceSpanMatchesQuery(span TraceSpanSummary, target ThreadRef, q Query) bool {
	if q.SpanName != "" && !strings.Contains(strings.ToLower(span.Name), strings.ToLower(strings.TrimSpace(q.SpanName))) {
		return false
	}
	if target.PID > 0 || target.Comm != "" {
		if !threadMatches(target, span.Thread.PID, span.Thread.Comm) {
			return false
		}
	}
	if q.TimeStart > 0 && span.EndTs < q.TimeStart {
		return false
	}
	if q.TimeEnd > 0 && span.StartTs > q.TimeEnd {
		return false
	}
	return true
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

func accumulateCPUFrequencyLimit(byCPU map[int]CPUFrequencyLimit, ev Event) {
	cpu := eventCPUForStats(ev)
	if cpu < 0 {
		return
	}
	item := byCPU[cpu]
	item.CPU = cpu
	item.Count++
	// Keep the most restrictive max-frequency row for quick capacity diagnosis,
	// while Count preserves how many limit rows were seen in the window.
	if item.Line == 0 || (ev.FrequencyMax > 0 && (item.MaxFrequency == 0 || ev.FrequencyMax < item.MaxFrequency)) {
		item.MinFrequency = ev.FrequencyMin
		item.MaxFrequency = ev.FrequencyMax
		item.Line = ev.Line
		item.Ts = ev.Ts
	}
	byCPU[cpu] = item
}

func sortedCPUFrequencyLimits(in map[int]CPUFrequencyLimit, max int) []CPUFrequencyLimit {
	out := make([]CPUFrequencyLimit, 0, len(in))
	for _, item := range in {
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].MaxFrequency != out[j].MaxFrequency {
			if out[i].MaxFrequency == 0 {
				return false
			}
			if out[j].MaxFrequency == 0 {
				return true
			}
			return out[i].MaxFrequency < out[j].MaxFrequency
		}
		return out[i].CPU < out[j].CPU
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func accumulateSubsystemEvent(byKind map[string]SubsystemEventSummary, ev Event) {
	kind := firstNonEmpty(ev.SubsystemKind, subsystemKindForEventType(ev.Type))
	if kind == "" {
		return
	}
	key := fmt.Sprintf("%s/%s", kind, ev.Type)
	item := byKind[key]
	item.Kind = kind
	item.EventType = ev.Type
	item.Count++
	if item.Line == 0 || ev.Line < item.Line {
		item.Line = ev.Line
		item.Ts = ev.Ts
		item.Example = clampString(ev.FieldText, 140)
	}
	byKind[key] = item
}

func subsystemKindForEventType(typ EventType) string {
	switch typ {
	case EventCPUFrequencyLimit:
		return "cpu_frequency_limits"
	case EventSoftIRQ:
		return "softirq"
	case EventStorage:
		return "storage"
	case EventFilesystem:
		return "filesystem"
	case EventPower:
		return "power"
	case EventWorkqueue:
		return "workqueue"
	case EventDMAFence:
		return "dma_fence"
	default:
		return ""
	}
}

func sortedSubsystemEvents(in map[string]SubsystemEventSummary, max int) []SubsystemEventSummary {
	out := make([]SubsystemEventSummary, 0, len(in))
	for _, item := range in {
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

func accumulateRuntimeResource(bio, filesystem, pageFault map[string]*RuntimeResourceSummary, ev Event) {
	kind := runtimeResourceKind(ev)
	if kind == "" {
		return
	}
	target := bio
	switch kind {
	case "filesystem":
		target = filesystem
	case "page_fault":
		target = pageFault
	}
	op := firstNonEmpty(ev.ResourceOp, ev.BlockOp, ev.MemoryKind, ev.Name)
	path := firstNonEmpty(ev.ResourcePath, ev.BlockDev, ev.ResourceAddress, "unknown")
	key := fmt.Sprintf("%s/%s/%s/%d", kind, op, path, ev.PID)
	item := target[key]
	if item == nil {
		item = &RuntimeResourceSummary{
			Kind:      kind,
			Operation: op,
			Path:      path,
			Thread:    threadRefFromEvent(ev),
			Line:      ev.Line,
			Ts:        ev.Ts,
			Example:   clampString(ev.FieldText, 160),
			Callstack: ev.ResourceCallstack,
		}
		target[key] = item
	}
	item.Count++
	item.TotalLatencyMs += ev.ResourceLatencyMs
	if ev.ResourceLatencyMs > item.MaxLatencyMs {
		item.MaxLatencyMs = ev.ResourceLatencyMs
	}
	item.Bytes += ev.ResourceBytes
	if item.Line == 0 || ev.Line < item.Line {
		item.Line = ev.Line
		item.Ts = ev.Ts
	}
	if item.Example == "" {
		item.Example = clampString(ev.FieldText, 160)
	}
	if item.Callstack == "" {
		item.Callstack = ev.ResourceCallstack
	}
	if item.Address == "" {
		item.Address = ev.ResourceAddress
	}
}

func runtimeResourceKind(ev Event) string {
	text := strings.ToLower(ev.Name + " " + ev.SubsystemKind + " " + ev.FieldText)
	switch {
	case ev.Type == EventStorage && strings.Contains(text, "bio"):
		return "bio"
	case ev.Type == EventFilesystem:
		return "filesystem"
	case ev.Type == EventMemory && (ev.MemoryKind == "page_fault" || strings.Contains(text, "page_fault") || strings.Contains(text, "fault")):
		return "page_fault"
	default:
		return ""
	}
}

func sortedRuntimeResources(in map[string]*RuntimeResourceSummary, max int) []RuntimeResourceSummary {
	out := make([]RuntimeResourceSummary, 0, len(in))
	for _, item := range in {
		out = append(out, *item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TotalLatencyMs != out[j].TotalLatencyMs {
			return out[i].TotalLatencyMs > out[j].TotalLatencyMs
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
	res.BinderWaits = findBinderWaitsForChain(*res, ipc.Edges, ipc.BinderEvents)
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

func findBinderWaitsForChain(chain ChainResult, edges []IPCEdge, aux []BinderEventSummary) []BinderWaitSummary {
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
			wait.Caveats = append(wait.Caveats, binderAuxCaveatsForWait(wait, aux)...)
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

func binderAuxCaveatsForWait(wait BinderWaitSummary, aux []BinderEventSummary) []string {
	var out []string
	for _, item := range aux {
		if item.Thread.PID > 0 && wait.Thread.PID > 0 && item.Thread.PID != wait.Thread.PID {
			continue
		}
		if wait.TransactionID > 0 && item.TransactionID > 0 && item.TransactionID != wait.TransactionID {
			continue
		}
		if item.Ts > 0 && wait.SendTs > 0 {
			if item.Ts < wait.SendTs-0.010 || (wait.SleepStartTs > 0 && item.Ts > wait.SleepStartTs+0.010) {
				continue
			}
		}
		switch item.Type {
		case EventBinderAllocBuf:
			out = append(out, fmt.Sprintf("binder alloc buffer line %d data_size=%d offsets_size=%d extra_buffers_size=%d", item.Line, item.DataSize, item.OffsetsSize, item.ExtraBuffersSize))
		case EventBinderLock, EventBinderLocked, EventBinderUnlock:
			out = append(out, fmt.Sprintf("%s line %d tag=%s", item.Type, item.Line, item.Tag))
		case EventBinderReply:
			out = append(out, fmt.Sprintf("binder reply line %d", item.Line))
		}
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func BuildRootCauseRank(idx *Index, q Query) RootCauseRankResult {
	q = normalizeQuery(idx, q)
	var chain ChainResult
	if q.PID > 0 || q.Thread != "" {
		chain = BuildWakeupChain(idx, q)
	}
	stats := ComputeWindowStats(idx, q)
	rank := buildRootCauseRankFrom(q, chain, stats)
	latency := BuildSchedulerLatencyStats(idx, q)
	return enrichRootCauseRankWithScheduler(q, rank, latency, stats)
}

func buildRootCauseRankFrom(q Query, chain ChainResult, stats WindowStats) RootCauseRankResult {
	res := RootCauseRankResult{
		Target: chain.Target,
		Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
	}
	var items []RootCauseRankItem
	for _, root := range chain.RootEvidence {
		items = append(items, rootCauseItem(root.Type, root.Thread, root.DurationMs, root.Confidence, root.LineStart, root.LineEnd, "wakeup_chain", root.Summary))
	}
	for _, pressure := range stats.CPUPressure {
		if pressure.RunnableWaitMs <= 0 {
			continue
		}
		conf := 0.72
		if pressure.HighPriorityRunningMs > 0 {
			conf = 0.80
		}
		summary := fmt.Sprintf("cpu=%d had %.3fms runnable wait and %.3fms running time in the selected window", pressure.CPU, pressure.RunnableWaitMs, pressure.RunningMs)
		if pressure.HighPriorityRunningMs > 0 {
			summary = fmt.Sprintf("%s; high-priority running time %.3fms", summary, pressure.HighPriorityRunningMs)
		}
		items = append(items, rootCauseItem("cpu_pressure", ThreadRef{}, pressure.RunnableWaitMs, conf, firstThreadLine(pressure.TopRunnable), lastThreadLine(pressure.TopRunning), "window_stats", summary))
	}
	for _, io := range stats.IOLatencies {
		items = append(items, rootCauseItem("io_latency", io.IssueThread, io.DurationMs, 0.86, io.IssueLine, io.CompleteLine, "window_stats", fmt.Sprintf("block IO %s %s sector=%d len=%d took %.3fms", io.Dev, io.Op, io.Sector, io.Len, io.DurationMs)))
	}
	windowImpactMs := (q.TimeEnd - q.TimeStart) * 1000
	if windowImpactMs < 0 {
		windowImpactMs = 0
	}
	for _, limit := range stats.CPUFrequencyLimits {
		if limit.MaxFrequency <= 0 {
			continue
		}
		items = append(items, rootCauseItem("cpu_frequency_limit", ThreadRef{}, windowImpactMs, 0.58, limit.Line, limit.Line, "window_stats", fmt.Sprintf("cpu=%d had frequency limit min=%dkHz max=%dkHz in the selected window (count=%d)", limit.CPU, limit.MinFrequency, limit.MaxFrequency, limit.Count)))
	}
	for _, span := range stats.TraceSpans {
		items = append(items, rootCauseItem("trace_span", span.Thread, span.DurationMs, 0.74, span.StartLine, span.EndLine, "window_stats", fmt.Sprintf("trace span %q lasted %.3fms", span.Name, span.DurationMs)))
	}
	for _, td := range stats.RunnableTop {
		items = append(items, rootCauseItem("runnable_wait", td.Thread, td.DurationMs, 0.76, td.LineStart, td.LineEnd, "window_stats", fmt.Sprintf("%s was runnable for %.3fms%s", threadLabel(td.Thread), td.DurationMs, durationCPUDetail(td))))
	}
	for _, td := range stats.DStateTop {
		items = append(items, rootCauseItem("d_state_or_io_wait", td.Thread, td.DurationMs, 0.82, td.LineStart, td.LineEnd, "window_stats", fmt.Sprintf("%s was in D-state/IO-like wait for %.3fms%s", threadLabel(td.Thread), td.DurationMs, durationCPUDetail(td))))
	}
	for _, burst := range stats.IRQBursts {
		items = append(items, rootCauseItem("irq_burst", ThreadRef{}, burst.DurationMs, 0.66, burst.LineStart, burst.LineEnd, "window_stats", fmt.Sprintf("IRQ burst %s irq=%d on cpu=%d had %d event(s) over %.3fms", burst.Name, burst.IRQ, burst.CPU, burst.Count, burst.DurationMs)))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		if items[i].ImpactMs != items[j].ImpactMs {
			return items[i].ImpactMs > items[j].ImpactMs
		}
		return items[i].LineStart < items[j].LineStart
	})
	limit := q.Limit
	if limit <= 0 || limit > 12 {
		limit = 12
	}
	if len(items) > limit {
		res.Caveats = append(res.Caveats, fmt.Sprintf("root_cause_rank compacted from %d to %d candidate(s)", len(items), limit))
		items = items[:limit]
	}
	for i := range items {
		items[i].Rank = i + 1
		items[i].Tier = rootCauseTier(i)
	}
	if len(items) == 0 {
		res.Caveats = append(res.Caveats, "no deterministic root-cause candidates were found in the selected window")
	}
	res.Items = items
	res.Caveats = append(res.Caveats, stats.Caveats...)
	res.Caveats = append(res.Caveats, chain.Caveats...)
	return res
}

func enrichRootCauseRankWithScheduler(q Query, rank RootCauseRankResult, latency SchedulerLatencyResult, stats WindowStats) RootCauseRankResult {
	cpus := map[int]CPUStats{}
	for _, cpu := range stats.CPU {
		cpus[cpu.CPU] = cpu
	}
	for _, item := range latency.Items {
		conf := 0.78
		if item.HighPriorityRunningMs > 0 {
			conf = 0.84
		}
		summary := item.Summary
		if len(item.SameCPUTopRunning) > 0 {
			summary = fmt.Sprintf("%s; same_cpu_top_running=%s", summary, threadLabel(item.SameCPUTopRunning[0].Thread))
		}
		rank.Items = append(rank.Items, rootCauseItem("scheduler_latency", item.Thread, item.DurationMs, conf, item.StartLine, item.EndLine, "scheduler_latency_stats", summary))
		if frequencyIsLowForCPU(item.Frequency, cpus[item.CPU]) {
			rank.Items = append(rank.Items, rootCauseItem("low_frequency", item.Thread, item.DurationMs, 0.70, item.StartLine, item.EndLine, "scheduler_latency_stats", fmt.Sprintf("%s runnable wait began at %dkHz on cpu=%d, below the CPU's observed max frequency in the selected window", threadLabel(item.Thread), item.Frequency, item.CPU)))
		}
	}
	for _, supply := range stats.ComputeSupply {
		switch supply.Verdict {
		case "cpu_pressure", "mixed_cpu_pressure_and_low_frequency", "low_frequency_signal":
			typ := "compute_supply"
			if strings.Contains(supply.Verdict, "low_frequency") {
				typ = "low_frequency"
			}
			rank.Items = append(rank.Items, rootCauseItem(typ, supply.Thread, supply.DurationMs, supply.Confidence, supply.LineStart, supply.LineEnd, "window_stats.compute_supply", supply.Summary))
		}
	}
	sort.SliceStable(rank.Items, func(i, j int) bool {
		if rank.Items[i].Score != rank.Items[j].Score {
			return rank.Items[i].Score > rank.Items[j].Score
		}
		if rank.Items[i].ImpactMs != rank.Items[j].ImpactMs {
			return rank.Items[i].ImpactMs > rank.Items[j].ImpactMs
		}
		return rank.Items[i].LineStart < rank.Items[j].LineStart
	})
	limit := q.Limit
	if limit <= 0 || limit > 12 {
		limit = 12
	}
	if len(rank.Items) > limit {
		rank.Caveats = append(rank.Caveats, fmt.Sprintf("root_cause_rank compacted after scheduler/compute enrichment from %d to %d candidate(s)", len(rank.Items), limit))
		rank.Items = rank.Items[:limit]
	}
	for i := range rank.Items {
		rank.Items[i].Rank = i + 1
		rank.Items[i].Tier = rootCauseTier(i)
	}
	rank.Caveats = append(rank.Caveats, latency.Caveats...)
	return rank
}

func rootCauseItem(typ string, thread ThreadRef, impactMs float64, confidence float64, lineStart, lineEnd int, source, summary string) RootCauseRankItem {
	if confidence <= 0 {
		confidence = 0.5
	}
	return RootCauseRankItem{
		Type:       typ,
		Thread:     thread,
		ImpactMs:   impactMs,
		Score:      impactMs * confidence * rootCauseTypeWeight(typ),
		Confidence: confidence,
		LineStart:  lineStart,
		LineEnd:    lineEnd,
		Source:     source,
		Summary:    summary,
	}
}

func rootCauseTypeWeight(typ string) float64 {
	switch typ {
	case "io_wait", "d_state_or_io_wait", "binder_wait":
		return 1.25
	case "runnable_wait", "cpu_pressure", "scheduler_latency":
		return 1.15
	case "compute_supply", "low_frequency":
		return 0.95
	case "cpu_frequency_limit":
		return 0.7
	case "running":
		return 1.0
	case "trace_span":
		return 0.9
	case "irq_burst":
		return 0.75
	default:
		return 0.8
	}
}

func rootCauseTier(idx int) string {
	switch idx {
	case 0:
		return "primary"
	case 1:
		return "secondary"
	default:
		return "tertiary"
	}
}

func firstThreadLine(items []ThreadDuration) int {
	for _, item := range items {
		if item.LineStart > 0 {
			return item.LineStart
		}
	}
	return 0
}

func lastThreadLine(items []ThreadDuration) int {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].LineEnd > 0 {
			return items[i].LineEnd
		}
	}
	return 0
}

func BuildInteractionStats(idx *Index, q Query) InteractionStatsResult {
	q = normalizeQuery(idx, q)
	target := resolveThread(idx, q)
	direction := normalizeInteractionDirection(q.InteractionDirection)
	res := InteractionStatsResult{
		Target:    target,
		Window:    TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
		Direction: direction,
	}
	if target.PID == 0 && target.Comm == "" {
		res.Caveats = append(res.Caveats, "target thread not found; provide pid or a thread name visible in the trace")
		return res
	}
	acc := map[string]*InteractionSummary{}
	add := func(peer ThreadRef, ts float64, line int, kind string) {
		if peer.PID == 0 && peer.Comm == "" {
			return
		}
		key := fmt.Sprintf("%d/%s", peer.PID, peer.Comm)
		item := acc[key]
		if item == nil {
			item = &InteractionSummary{Peer: peer, FirstTs: ts, LastTs: ts, FirstLine: line, LastLine: line}
			acc[key] = item
		}
		switch kind {
		case "wake_to_target":
			item.WakeupsToTarget++
		case "wake_from_target":
			item.WakeupsFromTarget++
		case "binder_to_target":
			item.BinderToTarget++
		case "binder_from_target":
			item.BinderFromTarget++
		}
		item.TotalInteractions++
		if item.FirstTs == 0 || ts < item.FirstTs {
			item.FirstTs = ts
			item.FirstLine = line
		}
		if ts >= item.LastTs {
			item.LastTs = ts
			item.LastLine = line
		}
	}
	for _, ev := range idx.Events {
		if ev.Type != EventSchedWakeup && ev.Type != EventSchedWaking {
			continue
		}
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		if directionAllowsIncoming(direction) && threadMatches(target, ev.WakeePID, ev.WakeeComm) {
			add(ThreadRef{Comm: ev.Comm, PID: ev.PID, TGID: ev.TGID}, ev.Ts, ev.Line, "wake_to_target")
		}
		if directionAllowsOutgoing(direction) && threadMatches(target, ev.PID, ev.Comm) {
			add(ThreadRef{Comm: ev.WakeeComm, PID: ev.WakeePID}, ev.Ts, ev.Line, "wake_from_target")
		}
	}
	ipc := BuildIPCGraph(idx, q)
	for _, edge := range ipc.Edges {
		if directionAllowsOutgoing(direction) && threadMatches(target, edge.Sender.PID, edge.Sender.Comm) {
			add(firstNonEmptyThread(edge.Receiver, ThreadRef{PID: edge.DestThread, TGID: edge.DestProc}), edge.SendTs, edge.SendLine, "binder_from_target")
		}
		if directionAllowsIncoming(direction) && threadMatches(target, edge.Receiver.PID, edge.Receiver.Comm) {
			add(edge.Sender, firstPositiveFloat(edge.ReceiveTs, edge.SendTs), firstPositive(edge.ReceiveLine, edge.SendLine), "binder_to_target")
		}
	}
	for _, item := range acc {
		item.Summary = fmt.Sprintf("%s interacted with target %d time(s): wake_to_target=%d wake_from_target=%d binder_to_target=%d binder_from_target=%d",
			threadLabel(item.Peer), item.TotalInteractions, item.WakeupsToTarget, item.WakeupsFromTarget, item.BinderToTarget, item.BinderFromTarget)
		res.Items = append(res.Items, *item)
	}
	sort.SliceStable(res.Items, func(i, j int) bool {
		if res.Items[i].TotalInteractions != res.Items[j].TotalInteractions {
			return res.Items[i].TotalInteractions > res.Items[j].TotalInteractions
		}
		return res.Items[i].FirstLine < res.Items[j].FirstLine
	})
	limit := q.Limit
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	if len(res.Items) > limit {
		res.Caveats = append(res.Caveats, fmt.Sprintf("interaction_stats compacted from %d to %d peer(s)", len(res.Items), limit))
		res.Items = res.Items[:limit]
	}
	if len(res.Items) == 0 {
		res.Caveats = append(res.Caveats, "no wakeup or binder interactions with the target were found in the selected window")
	}
	res.Caveats = append(res.Caveats, ipc.Caveats...)
	return res
}

func BuildFramePipeline(idx *Index, q Query) FramePipelineResult {
	q = normalizeQuery(idx, q)
	res := FramePipelineResult{Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}}
	if idx == nil {
		res.Caveats = append(res.Caveats, "trace index is empty")
		return res
	}
	spans, caveats := FindSpanWindows(idx, q, q.Limit)
	for _, span := range spans {
		if !isFrameLikeSpan(span.Name) && strings.TrimSpace(q.SpanName) == "" {
			continue
		}
		phase := classifyFramePhase(span.Name)
		res.Items = append(res.Items, FramePhaseSummary{
			Thread:     span.Thread,
			Phase:      phase,
			Name:       span.Name,
			StartTs:    span.StartTs,
			EndTs:      span.EndTs,
			DurationMs: span.DurationMs,
			StartLine:  span.StartLine,
			EndLine:    span.EndLine,
			Summary:    fmt.Sprintf("%s phase %s span %q lasted %.3fms", threadLabel(span.Thread), phase, span.Name, span.DurationMs),
		})
	}
	sort.SliceStable(res.Items, func(i, j int) bool {
		if res.Items[i].StartTs != res.Items[j].StartTs {
			return res.Items[i].StartTs < res.Items[j].StartTs
		}
		return res.Items[i].StartLine < res.Items[j].StartLine
	})
	limit := q.Limit
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	if len(res.Items) > limit {
		res.Caveats = append(res.Caveats, fmt.Sprintf("frame pipeline compacted from %d to %d phase span(s)", len(res.Items), limit))
		res.Items = res.Items[:limit]
	}
	if len(res.Items) == 0 {
		res.Caveats = append(res.Caveats, "no frame/render-like complete B/E trace spans matched the selected filters")
	}
	res.Caveats = append(res.Caveats, caveats...)
	return res
}

func BuildFrameTimeline(idx *Index, q Query) FrameTimelineResult {
	q = normalizeQuery(idx, q)
	res := FrameTimelineResult{Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}}
	frame := BuildFramePipeline(idx, q)
	for i, phase := range frame.Items {
		item := FrameTimelineItem{
			Index:      i + 1,
			Thread:     phase.Thread,
			Phase:      phase.Phase,
			Role:       classifyFrameTimelineRole(phase.Name, phase.Phase),
			Name:       phase.Name,
			FrameID:    frameIDFromName(phase.Name),
			StartTs:    phase.StartTs,
			EndTs:      phase.EndTs,
			DurationMs: phase.DurationMs,
			StartLine:  phase.StartLine,
			EndLine:    phase.EndLine,
		}
		item.Summary = fmt.Sprintf("frame_timeline item #%d role=%s phase=%s %s span %q lasted %.3fms", item.Index, item.Role, item.Phase, threadLabel(item.Thread), item.Name, item.DurationMs)
		res.Items = append(res.Items, item)
	}
	for i := 0; i+1 < len(res.Items); i++ {
		from := res.Items[i]
		to := res.Items[i+1]
		latency := (to.StartTs - from.EndTs) * 1000
		if latency < 0 {
			latency = 0
		}
		res.Flows = append(res.Flows, FrameFlowEdge{
			FromIndex: from.Index,
			ToIndex:   to.Index,
			From:      from.Thread,
			To:        to.Thread,
			FromPhase: from.Phase,
			ToPhase:   to.Phase,
			LatencyMs: latency,
			LineStart: firstPositive(from.EndLine, from.StartLine),
			LineEnd:   firstPositive(to.StartLine, to.EndLine),
			Summary:   fmt.Sprintf("frame flow #%d->#%d %s/%s to %s/%s latency=%.3fms", from.Index, to.Index, threadLabel(from.Thread), from.Phase, threadLabel(to.Thread), to.Phase, latency),
		})
	}
	res.Caveats = append(res.Caveats, frame.Caveats...)
	if len(res.Items) == 0 {
		res.Caveats = append(res.Caveats, "no frame timeline items were built; need complete B/E frame-like trace spans")
	}
	if len(res.Flows) == 0 && len(res.Items) > 1 {
		res.Caveats = append(res.Caveats, "frame timeline had items but no flow edges were emitted")
	}
	return res
}

func frameSpans(frame FramePipelineResult) []TraceSpanSummary {
	out := make([]TraceSpanSummary, 0, len(frame.Items))
	for _, item := range frame.Items {
		out = append(out, TraceSpanSummary{
			Thread:     item.Thread,
			Name:       item.Name,
			StartTs:    item.StartTs,
			EndTs:      item.EndTs,
			DurationMs: item.DurationMs,
			StartLine:  item.StartLine,
			EndLine:    item.EndLine,
		})
	}
	return out
}

func frameTimelineSpans(frame FrameTimelineResult) []TraceSpanSummary {
	out := make([]TraceSpanSummary, 0, len(frame.Items))
	for _, item := range frame.Items {
		out = append(out, TraceSpanSummary{
			Thread:     item.Thread,
			Name:       item.Name,
			StartTs:    item.StartTs,
			EndTs:      item.EndTs,
			DurationMs: item.DurationMs,
			StartLine:  item.StartLine,
			EndLine:    item.EndLine,
		})
	}
	return out
}

func isFrameLikeSpan(name string) bool {
	lower := strings.ToLower(name)
	for _, token := range []string{"frame", "timeline", "expected", "actual", "jank", "deadline", "vsync", "choreographer", "render", "draw", "traversal", "measure", "layout", "present", "gpu", "surface", "compose"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func classifyFramePhase(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "vsync") || strings.Contains(lower, "choreographer") || strings.Contains(lower, "doframe"):
		return "frame_schedule"
	case strings.Contains(lower, "measure") || strings.Contains(lower, "layout") || strings.Contains(lower, "traversal"):
		return "ui_traversal"
	case strings.Contains(lower, "render") || strings.Contains(lower, "draw"):
		return "render"
	case strings.Contains(lower, "present") || strings.Contains(lower, "surface") || strings.Contains(lower, "compose"):
		return "composition"
	case strings.Contains(lower, "gpu"):
		return "gpu"
	default:
		return "frame_related"
	}
}

func classifyFrameTimelineRole(name, phase string) string {
	lower := strings.ToLower(name + " " + phase)
	switch {
	case strings.Contains(lower, "expected"):
		return "expected"
	case strings.Contains(lower, "actual"):
		return "actual"
	case strings.Contains(lower, "jank") || strings.Contains(lower, "miss") || strings.Contains(lower, "deadline"):
		return "jank"
	case strings.Contains(lower, "gpu"):
		return "gpu"
	case strings.Contains(lower, "render_service") || strings.Contains(lower, "rsuni") || strings.Contains(lower, "rs ") || strings.Contains(lower, "h:render"):
		return "render_service"
	case strings.Contains(lower, "choreographer") || strings.Contains(lower, "doframe") || strings.Contains(lower, "traversal") || strings.Contains(lower, "measure") || strings.Contains(lower, "layout"):
		return "ui"
	case strings.Contains(lower, "present") || strings.Contains(lower, "surface") || strings.Contains(lower, "compose"):
		return "composition"
	default:
		return firstNonEmpty(phase, "frame")
	}
}

func frameIDFromName(name string) string {
	fields := strings.FieldsFunc(name, func(r rune) bool {
		return r == ' ' || r == '|' || r == ',' || r == ';' || r == '(' || r == ')' || r == '[' || r == ']'
	})
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		for _, prefix := range []string{"vsyncid=", "vsync=", "frameid=", "frame=", "id="} {
			if strings.HasPrefix(lower, prefix) && len(field) > len(prefix) {
				return strings.TrimSpace(field[len(prefix):])
			}
		}
	}
	return ""
}

func BuildCriticalBlockingCalls(idx *Index, q Query) CriticalBlockingResult {
	q = normalizeQuery(idx, q)
	res := CriticalBlockingResult{Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}}
	if idx == nil {
		res.Caveats = append(res.Caveats, "trace index is empty")
		return res
	}
	stats := ComputeWindowStats(idx, q)
	add := func(item CriticalBlockingCandidate) {
		if item.DurationMs <= 0 && item.LineStart == 0 {
			return
		}
		if item.Confidence <= 0 {
			item.Confidence = 0.6
		}
		res.Items = append(res.Items, item)
	}
	for _, br := range stats.BlockedReasons {
		add(CriticalBlockingCandidate{
			Type:       "blocked_reason",
			Thread:     br.Thread,
			StartTs:    br.Ts,
			EndTs:      br.Ts,
			LineStart:  br.Line,
			LineEnd:    br.Line,
			Confidence: 0.82,
			Summary:    fmt.Sprintf("%s sched_blocked_reason iowait=%d caller=%s count=%d", threadLabel(br.Thread), br.IOWait, firstNonEmpty(br.Reason, "unknown"), br.Count),
		})
	}
	for _, io := range stats.IOLatencies {
		add(CriticalBlockingCandidate{
			Type:       "io_latency",
			Thread:     io.IssueThread,
			Peer:       io.CompleteThread,
			DurationMs: io.DurationMs,
			StartTs:    io.IssueTs,
			EndTs:      io.CompleteTs,
			LineStart:  io.IssueLine,
			LineEnd:    io.CompleteLine,
			Confidence: 0.86,
			Summary:    fmt.Sprintf("block IO %s %s sector=%d len=%d took %.3fms", io.Dev, io.Op, io.Sector, io.Len, io.DurationMs),
		})
	}
	if q.PID > 0 || q.Thread != "" || q.ThreadInput != "" {
		chain := BuildWakeupChain(idx, q)
		for _, wait := range chain.BinderWaits {
			add(CriticalBlockingCandidate{
				Type:       "binder_wait",
				Thread:     wait.Thread,
				Peer:       wait.Peer,
				DurationMs: wait.DurationMs,
				StartTs:    wait.SendTs,
				EndTs:      firstPositiveFloat(wait.WakeupTs, wait.SleepStartTs),
				LineStart:  firstPositive(wait.SendLine, wait.SleepLine),
				LineEnd:    firstPositive(wait.WakeupLine, wait.ReceiveLine, wait.SleepLine),
				Confidence: wait.Confidence,
				Summary:    wait.Summary,
			})
		}
	}
	for _, td := range stats.DStateTop {
		add(CriticalBlockingCandidate{
			Type:       "d_state_or_io_wait",
			Thread:     td.Thread,
			DurationMs: td.DurationMs,
			LineStart:  td.LineStart,
			LineEnd:    td.LineEnd,
			Confidence: 0.80,
			Summary:    fmt.Sprintf("%s spent %.3fms in D-state/IO-like wait%s", threadLabel(td.Thread), td.DurationMs, durationCPUDetail(td)),
		})
	}
	for _, span := range stats.TraceSpans {
		if !isBlockingLikeText(span.Name) {
			continue
		}
		add(CriticalBlockingCandidate{
			Type:       "blocking_span",
			Thread:     span.Thread,
			DurationMs: span.DurationMs,
			StartTs:    span.StartTs,
			EndTs:      span.EndTs,
			LineStart:  span.StartLine,
			LineEnd:    span.EndLine,
			Confidence: 0.72,
			Summary:    fmt.Sprintf("blocking-like trace span %q lasted %.3fms", span.Name, span.DurationMs),
		})
	}
	for _, mem := range stats.MemoryKinds {
		if mem.Kind != "reclaim" && mem.Kind != "page_fault" && mem.Kind != "gc" {
			continue
		}
		add(CriticalBlockingCandidate{
			Type:       "memory_" + mem.Kind,
			LineStart:  mem.Line,
			LineEnd:    mem.Line,
			StartTs:    mem.Ts,
			EndTs:      mem.Ts,
			Confidence: 0.62,
			Summary:    fmt.Sprintf("memory category %s appeared %d time(s) in the selected window", mem.Kind, mem.Count),
		})
	}
	sort.SliceStable(res.Items, func(i, j int) bool {
		scoreI := res.Items[i].DurationMs * res.Items[i].Confidence
		scoreJ := res.Items[j].DurationMs * res.Items[j].Confidence
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		if res.Items[i].DurationMs != res.Items[j].DurationMs {
			return res.Items[i].DurationMs > res.Items[j].DurationMs
		}
		return res.Items[i].LineStart < res.Items[j].LineStart
	})
	limit := q.Limit
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	if len(res.Items) > limit {
		res.Caveats = append(res.Caveats, fmt.Sprintf("critical_blocking_calls compacted from %d to %d candidate(s)", len(res.Items), limit))
		res.Items = res.Items[:limit]
	}
	if len(res.Items) == 0 {
		res.Caveats = append(res.Caveats, "no critical blocking candidates matched the selected filters")
	}
	res.Caveats = append(res.Caveats, stats.Caveats...)
	return res
}

func isBlockingLikeText(name string) bool {
	lower := strings.ToLower(name)
	for _, token := range []string{"lock", "futex", "wait", "blocked", "binder", "sync", "mutex", "semaphore", "contention", "io"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func BuildRecipe(idx *Index, q Query) RecipeResult {
	q = normalizeQuery(idx, q)
	name := normalizeRecipeName(q.RecipeName, q)
	res := RecipeResult{Name: name}
	switch name {
	case "jank":
		res.IncludedViews = []string{"frame_window", "frame_timeline", "frame_flow", "scheduler_latency_stats", "window_stats", "root_cause_rank", "critical_blocking_calls"}
		res.Summary = "jank recipe: derive frame/render spans, frame timeline/flows, scheduler latency, same-window resources, ranked causes, and blocking candidates"
	case "runnable_delay":
		res.IncludedViews = []string{"scheduler_latency_stats", "window_stats", "root_cause_rank"}
		res.Summary = "runnable-delay recipe: quantify runnable waits, CPU pressure, compute supply, and ranked causes"
	case "binder_wait":
		res.IncludedViews = []string{"ipc_graph", "wakeup_chain", "critical_blocking_calls", "root_cause_rank"}
		res.Summary = "binder-wait recipe: combine binder IPC edges with scheduler sleep/wakeup evidence"
	case "io_wait":
		res.IncludedViews = []string{"window_stats", "critical_blocking_calls", "root_cause_rank"}
		res.Summary = "IO-wait recipe: pair block IO latencies, D-state/blocked reasons, and ranked causes"
	case "cpu_supply":
		res.IncludedViews = []string{"scheduler_latency_stats", "window_stats", "root_cause_rank"}
		res.Summary = "CPU-supply recipe: join runnable/running intervals with CPU busy/idle/frequency context"
	default:
		res.IncludedViews = []string{"wakeup_chain", "scheduler_latency_stats", "window_stats", "critical_blocking_calls", "root_cause_rank"}
		res.Summary = "sleep-root-cause recipe: trace wakeup chain, scheduler latency, resource stats, blocking candidates, and ranked causes"
	}
	if idx == nil {
		res.Caveats = append(res.Caveats, "trace index is empty")
	}
	return res
}

func normalizeRecipeName(raw string, q Query) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.ReplaceAll(name, "-", "_")
	switch name {
	case "jank", "frame", "frame_jank", "render", "render_pipeline":
		return "jank"
	case "runnable", "runnable_delay", "scheduler_latency", "scheduler_latency_stats":
		return "runnable_delay"
	case "binder", "binder_wait", "ipc":
		return "binder_wait"
	case "io", "io_wait", "d_state":
		return "io_wait"
	case "cpu", "cpu_supply", "frequency", "freq":
		return "cpu_supply"
	case "auto", "":
		if strings.TrimSpace(q.SpanName) != "" && isFrameLikeSpan(q.SpanName) {
			return "jank"
		}
		if len(q.EventTypes) > 0 {
			for _, typ := range q.EventTypes {
				if typ == EventBinderTransaction || typ == EventBinderReceived {
					return "binder_wait"
				}
				if typ == EventBlockIssue || typ == EventBlockComplete || typ == EventSchedBlockedReason {
					return "io_wait"
				}
			}
		}
		return "sleep_root_cause"
	default:
		return name
	}
}

func recipeHasView(recipe RecipeResult, view string) bool {
	for _, item := range recipe.IncludedViews {
		if item == view {
			return true
		}
	}
	return false
}

func normalizeInteractionDirection(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "incoming", "in", "to_target", "to":
		return "incoming"
	case "outgoing", "out", "from_target", "from":
		return "outgoing"
	default:
		return "both"
	}
}

func directionAllowsIncoming(direction string) bool {
	return direction == "" || direction == "both" || direction == "incoming"
}

func directionAllowsOutgoing(direction string) bool {
	return direction == "" || direction == "both" || direction == "outgoing"
}

func firstNonEmptyThread(items ...ThreadRef) ThreadRef {
	for _, item := range items {
		if item.PID > 0 || item.Comm != "" {
			return item
		}
	}
	return ThreadRef{}
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
	for _, limit := range stats.CPUFrequencyLimits {
		out = append(out, EvidenceFact{
			Subject:    fmt.Sprintf("cpu=%d", limit.CPU),
			Predicate:  "cpu_frequency_limit",
			Summary:    fmt.Sprintf("cpu=%d frequency limit min=%dkHz max=%dkHz appeared %d time(s) in the selected window", limit.CPU, limit.MinFrequency, limit.MaxFrequency, limit.Count),
			LineStart:  limit.Line,
			LineEnd:    limit.Line,
			StartTs:    limit.Ts,
			EndTs:      limit.Ts,
			Confidence: 0.68,
		})
		if len(out) >= 26 {
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
	for _, subsystem := range stats.SubsystemEvents {
		out = append(out, EvidenceFact{
			Subject:    subsystem.Kind,
			Predicate:  string(subsystem.EventType),
			Summary:    fmt.Sprintf("subsystem category %s (%s) appeared %d time(s) in the selected window", subsystem.Kind, subsystem.EventType, subsystem.Count),
			LineStart:  subsystem.Line,
			LineEnd:    subsystem.Line,
			StartTs:    subsystem.Ts,
			EndTs:      subsystem.Ts,
			Confidence: 0.62,
		})
		if len(out) >= 38 {
			break
		}
	}
	for _, supply := range stats.ComputeSupply {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(supply.Thread),
			Predicate:  "compute_supply",
			Object:     supply.Verdict,
			Summary:    supply.Summary,
			LineStart:  supply.LineStart,
			LineEnd:    supply.LineEnd,
			Confidence: supply.Confidence,
		})
		if len(out) >= 40 {
			break
		}
	}
	return out
}

func evidenceFromSchedulerLatency(latency SchedulerLatencyResult) []EvidenceFact {
	var out []EvidenceFact
	for _, item := range latency.Items {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(item.Thread),
			Predicate:  "scheduler_latency",
			Summary:    item.Summary,
			LineStart:  item.StartLine,
			LineEnd:    item.EndLine,
			StartTs:    item.StartTs,
			EndTs:      item.EndTs,
			Confidence: 0.78,
		})
		if len(out) >= 16 {
			break
		}
	}
	return out
}

func evidenceFromFramePipeline(frame FramePipelineResult) []EvidenceFact {
	var out []EvidenceFact
	for _, item := range frame.Items {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(item.Thread),
			Predicate:  "frame_phase",
			Object:     item.Phase,
			Summary:    item.Summary,
			LineStart:  item.StartLine,
			LineEnd:    item.EndLine,
			StartTs:    item.StartTs,
			EndTs:      item.EndTs,
			Confidence: 0.78,
		})
		if len(out) >= 16 {
			break
		}
	}
	return out
}

func evidenceFromFrameTimeline(frame FrameTimelineResult) []EvidenceFact {
	var out []EvidenceFact
	for _, item := range frame.Items {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(item.Thread),
			Predicate:  "frame_timeline_" + item.Role,
			Object:     item.Phase,
			Summary:    item.Summary,
			LineStart:  item.StartLine,
			LineEnd:    item.EndLine,
			StartTs:    item.StartTs,
			EndTs:      item.EndTs,
			Confidence: 0.78,
		})
		if len(out) >= 12 {
			break
		}
	}
	for _, flow := range frame.Flows {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(flow.From),
			Predicate:  "frame_flow",
			Object:     threadLabel(flow.To),
			Summary:    flow.Summary,
			LineStart:  flow.LineStart,
			LineEnd:    flow.LineEnd,
			Confidence: 0.72,
		})
		if len(out) >= 16 {
			break
		}
	}
	return out
}

func evidenceFromCriticalBlocking(blocking CriticalBlockingResult) []EvidenceFact {
	var out []EvidenceFact
	for _, item := range blocking.Items {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(item.Thread),
			Predicate:  item.Type,
			Object:     threadLabel(item.Peer),
			Summary:    item.Summary,
			LineStart:  item.LineStart,
			LineEnd:    item.LineEnd,
			StartTs:    item.StartTs,
			EndTs:      item.EndTs,
			Confidence: item.Confidence,
		})
		if len(out) >= 16 {
			break
		}
	}
	return out
}

func evidenceFromSpans(spans []TraceSpanSummary) []EvidenceFact {
	var out []EvidenceFact
	for _, span := range spans {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(span.Thread),
			Predicate:  "trace_span_window",
			Object:     span.Name,
			Summary:    fmt.Sprintf("trace span %q on %s covers %.6f..%.6f seconds and lasts %.3f ms", span.Name, threadLabel(span.Thread), span.StartTs, span.EndTs, span.DurationMs),
			LineStart:  span.StartLine,
			LineEnd:    span.EndLine,
			StartTs:    span.StartTs,
			EndTs:      span.EndTs,
			Confidence: 0.9,
		})
		if len(out) >= 16 {
			break
		}
	}
	return out
}

func evidenceFromRootCauseRank(rank RootCauseRankResult) []EvidenceFact {
	var out []EvidenceFact
	for _, item := range rank.Items {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(item.Thread),
			Predicate:  "root_cause_" + item.Tier,
			Object:     item.Type,
			Summary:    fmt.Sprintf("%s cause #%d (%s): %s", item.Tier, item.Rank, item.Type, item.Summary),
			LineStart:  item.LineStart,
			LineEnd:    item.LineEnd,
			Confidence: item.Confidence,
		})
		if len(out) >= 16 {
			break
		}
	}
	return out
}

func evidenceFromInteractionStats(stats InteractionStatsResult) []EvidenceFact {
	var out []EvidenceFact
	for _, item := range stats.Items {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(stats.Target),
			Predicate:  "interacts_with",
			Object:     threadLabel(item.Peer),
			Summary:    item.Summary,
			LineStart:  item.FirstLine,
			LineEnd:    item.LastLine,
			StartTs:    item.FirstTs,
			EndTs:      item.LastTs,
			Confidence: 0.78,
		})
		if len(out) >= 16 {
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
	for _, item := range ipc.BinderEvents {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(item.Thread),
			Predicate:  string(item.Type),
			Summary:    item.Summary,
			LineStart:  item.Line,
			LineEnd:    item.Line,
			StartTs:    item.Ts,
			EndTs:      item.Ts,
			Confidence: 0.78,
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
	out = append(out, traceCompletenessCaveats(idx, q, res)...)
	return out
}

func traceCompletenessCaveats(idx *Index, q Query, res Result) []string {
	if idx == nil {
		return nil
	}
	var out []string
	if (res.View == "wakeup_chain" || res.View == "root_cause_rank" || res.View == "scheduler_latency_stats" || res.View == "recipe" || res.View == "evidence_pack") && (q.PID > 0 || q.Thread != "" || q.ThreadInput != "") {
		target := resolveThread(idx, q)
		if target.PID > 0 || target.Comm != "" {
			tq := q
			tq.PID = target.PID
			tq.Thread = target.Comm
			tl := ThreadTimeline(idx, tq)
			for _, it := range tl.Intervals {
				if it.State != StateSSleep || it.DurationMs < q.MinDurationMs {
					continue
				}
				if findWakeupFor(idx, target, it.StartTs, it.EndTs) == nil {
					out = append(out, fmt.Sprintf("trace completeness: %s has %.3fms sleep interval lines=%d-%d without matching sched_wakeup/sched_waking in the selected window", threadLabel(target), it.DurationMs, it.StartLine, it.EndLine))
					break
				}
			}
		}
	}
	if q.TimeStart > 0 && (res.WindowStats != nil || res.SchedulerLatency != nil || res.RootCauseRank != nil || res.View == "recipe" || res.View == "evidence_pack") {
		for _, cpu := range cpusMentionedInResult(res) {
			if cpu < 0 {
				continue
			}
			if !hasFrequencyAtOrBefore(idx, q, cpu, q.TimeStart) && hasFrequencyAfter(idx, q, cpu, q.TimeStart) {
				out = append(out, fmt.Sprintf("trace completeness: cpu=%d has cpu_frequency rows after selected start %.6f but no initial frequency at/before the window; low-frequency inference for early intervals is lower confidence", cpu, q.TimeStart))
			}
		}
	}
	return dedupStrings(out)
}

func cpusMentionedInResult(res Result) []int {
	seen := map[int]bool{}
	add := func(cpu int) {
		if cpu >= 0 {
			seen[cpu] = true
		}
	}
	if res.WindowStats != nil {
		for _, cpu := range res.WindowStats.CPU {
			add(cpu.CPU)
		}
		for _, td := range res.WindowStats.RunnableTop {
			add(td.CPU)
		}
		for _, td := range res.WindowStats.TopRunning {
			add(td.CPU)
		}
		for _, item := range res.WindowStats.ComputeSupply {
			add(item.CPU)
		}
	}
	if res.SchedulerLatency != nil {
		for _, item := range res.SchedulerLatency.Items {
			add(item.CPU)
		}
	}
	out := make([]int, 0, len(seen))
	for cpu := range seen {
		out = append(out, cpu)
	}
	sort.Ints(out)
	return out
}

func hasFrequencyAtOrBefore(idx *Index, q Query, cpu int, ts float64) bool {
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || ev.Type != EventCPUFrequency || ev.Frequency <= 0 || eventCPUForStats(ev) != cpu {
			continue
		}
		if ev.Ts <= ts {
			return true
		}
		if ev.Ts > ts {
			return false
		}
	}
	return false
}

func hasFrequencyAfter(idx *Index, q Query, cpu int, ts float64) bool {
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || ev.Type != EventCPUFrequency || ev.Frequency <= 0 || eventCPUForStats(ev) != cpu {
			continue
		}
		if ev.Ts > ts && (q.TimeEnd == 0 || ev.Ts <= q.TimeEnd) {
			return true
		}
	}
	return false
}

func dedupStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		if strings.TrimSpace(item) == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
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
