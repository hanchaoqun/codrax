package tracequery

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

// unparsedLineCaveatRatio is the fraction of scanned lines that must fail to
// match any known trace format before the result carries a coverage caveat.
// Soft guidance only: the typed UnparsedLines counter gates nothing.
const unparsedLineCaveatRatio = 0.5

const wakeupMatchToleranceSec = 0.000005
const wakeupCausalAggregateOccurrenceCap = 8

func Run(idx *Index, q Query) Result {
	explicitTimeStart := q.TimeStart != 0
	explicitTimeEnd := q.TimeEnd != 0
	explicitWindowOrSelector := explicitTimeStart ||
		explicitTimeEnd ||
		q.LineStart != 0 ||
		q.LineEnd != 0 ||
		strings.TrimSpace(q.SpanName) != "" ||
		q.PID > 0 ||
		strings.TrimSpace(q.Thread) != "" ||
		strings.TrimSpace(q.ThreadInput) != "" ||
		len(q.EventTypes) > 0
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
		ScannedLineCount:            idx.ScannedLineCount,
		IndexWindowed:               idx.Windowed,
		IndexTimeStart:              idx.IndexTimeStart,
		IndexTimeEnd:                idx.IndexTimeEnd,
		IndexLineStart:              idx.IndexLineStart,
		IndexLineEnd:                idx.IndexLineEnd,
		EventCount:                  len(idx.Events),
		UnparsedLineCount:           idx.UnparsedLines,
		ParseLinePanics:             idx.ParseLinePanics,
		ClockRegressions:            idx.ClockRegressions,
		TimeStart:                   q.TimeStart,
		TimeEnd:                     q.TimeEnd,
	}
	if len(spanWindows) > 0 {
		res.SpanWindows = spanWindows
	}
	var cachedStats WindowStats
	var cachedStatsOK bool
	getStats := func() WindowStats {
		if !cachedStatsOK {
			cachedStats = ComputeWindowStats(idx, q)
			cachedStatsOK = true
		}
		return cachedStats
	}
	var cachedLatency SchedulerLatencyResult
	var cachedLatencyOK bool
	getLatency := func() SchedulerLatencyResult {
		if !cachedLatencyOK {
			cachedLatency = buildSchedulerLatencyStatsFromStats(idx, q, getStats())
			cachedLatencyOK = true
		}
		return cachedLatency
	}
	var cachedChain ChainResult
	var cachedChainOK bool
	getChain := func() ChainResult {
		if !cachedChainOK {
			cachedChain = BuildWakeupChain(idx, q)
			cachedChainOK = true
		}
		return cachedChain
	}
	var cachedIPC IPCGraphResult
	var cachedIPCOK bool
	getIPC := func() IPCGraphResult {
		if !cachedIPCOK {
			cachedIPC = BuildIPCGraph(idx, q)
			cachedIPCOK = true
		}
		return cachedIPC
	}
	var cachedRootCause RootCauseRankResult
	var cachedRootCauseOK bool
	getRootCause := func() RootCauseRankResult {
		if !cachedRootCauseOK {
			var chain ChainResult
			if q.PID > 0 || q.Thread != "" || q.ThreadInput != "" {
				chain = getChain()
			}
			stats := getStats()
			rank := buildRootCauseRankFrom(q, chain, stats)
			rank = enrichRootCauseRankWithScheduler(q, rank, getLatency(), stats, chain)
			rank = attachPerfContextToRootCauseRank(idx, q, rank, stats)
			cachedRootCause = rank
			cachedRootCauseOK = true
		}
		return cachedRootCause
	}
	var cachedBlocking CriticalBlockingResult
	var cachedBlockingOK bool
	getBlocking := func() CriticalBlockingResult {
		if !cachedBlockingOK {
			var chain *ChainResult
			if cachedChainOK {
				chain = &cachedChain
			}
			cachedBlocking = buildCriticalBlockingCallsFromStats(idx, q, getStats(), chain)
			cachedBlockingOK = true
		}
		return cachedBlocking
	}
	var cachedFrame FramePipelineResult
	var cachedFrameOK bool
	getFrame := func() FramePipelineResult {
		if !cachedFrameOK {
			cachedFrame = BuildFramePipeline(idx, q)
			cachedFrameOK = true
		}
		return cachedFrame
	}
	var cachedFrameTimeline FrameTimelineResult
	var cachedFrameTimelineOK bool
	getFrameTimeline := func() FrameTimelineResult {
		if !cachedFrameTimelineOK {
			cachedFrameTimeline = buildFrameTimelineFromPipeline(q, getFrame())
			cachedFrameTimelineOK = true
		}
		return cachedFrameTimeline
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
		stats := getStats()
		res.WindowStats = &stats
		res.EvidencePack = evidenceFromStats(stats)
	case "perf_stats":
		stats := getStats()
		res.WindowStats = &stats
		res.PerfStats = stats.PerfSamples
		if stats.PerfSamples == nil {
			res.Caveats = append(res.Caveats, "no perf_sample events matched the selected window or filters")
		}
		res.EvidencePack = evidenceFromPerfContext(stats.PerfSamples)
	case "perf_timeline":
		timeline := BuildPerfTimeline(idx, q)
		res.PerfTimeline = &timeline
		res.EvidencePack = evidenceFromPerfTimeline(timeline)
	case "scheduler_latency_stats":
		latency := getLatency()
		res.SchedulerLatency = &latency
		res.EvidencePack = evidenceFromSchedulerLatency(latency)
	case "ipc_graph":
		ipc := getIPC()
		res.IPCGraph = &ipc
		res.EvidencePack = evidenceFromIPCGraph(ipc)
	case "wakeup_chain":
		chain := getChain()
		res.WakeupChain = &chain
		if q.IncludeWindowStats {
			stats := getStats()
			res.WindowStats = &stats
		}
		res.EvidencePack = append(evidenceFromChain(chain), evidenceFromIPCGraph(IPCGraphResult{Edges: chain.IPCEdges})...)
	case "root_cause_rank":
		chain := ChainResult{}
		if q.PID > 0 || q.Thread != "" {
			chain = getChain()
			res.WakeupChain = &chain
		}
		stats := getStats()
		res.WindowStats = &stats
		latency := getLatency()
		res.SchedulerLatency = &latency
		rank := getRootCause()
		res.RootCauseRank = &rank
		res.EvidencePack = evidenceFromRootCauseRank(rank)
	case "frame_root_cause_bundle":
		bundle := BuildFrameRootCauseBundle(idx, q)
		res.FrameRootCauseBundle = &bundle
		if bundle.WakeupChain != nil {
			res.WakeupChain = bundle.WakeupChain
			res.EvidencePack = append(res.EvidencePack, evidenceFromChain(*bundle.WakeupChain)...)
		}
		if bundle.FrameTimeline != nil {
			res.FrameTimeline = bundle.FrameTimeline
			res.SpanWindows = frameTimelineSpans(*bundle.FrameTimeline)
			res.EvidencePack = append(res.EvidencePack, evidenceFromFrameTimeline(*bundle.FrameTimeline)...)
		}
		if bundle.RootCauseRank != nil {
			res.RootCauseRank = bundle.RootCauseRank
			res.EvidencePack = append(res.EvidencePack, evidenceFromRootCauseRank(*bundle.RootCauseRank)...)
		}
		if bundle.CriticalBlocking != nil {
			res.CriticalBlocking = bundle.CriticalBlocking
			res.EvidencePack = append(res.EvidencePack, evidenceFromCriticalBlocking(*bundle.CriticalBlocking)...)
		}
		stats := getStats()
		res.WindowStats = &stats
	case "trace_perf_bundle":
		stats := getStats()
		res.WindowStats = &stats
		res.PerfStats = stats.PerfSamples
		if q.PID > 0 || q.Thread != "" || q.ThreadInput != "" {
			chain := getChain()
			res.WakeupChain = &chain
			res.EvidencePack = append(res.EvidencePack, evidenceFromChain(chain)...)
			rank := getRootCause()
			res.RootCauseRank = &rank
			res.EvidencePack = append(res.EvidencePack, evidenceFromRootCauseRank(rank)...)
		}
		res.EvidencePack = append(res.EvidencePack, evidenceFromStats(stats)...)
	case "interaction_stats":
		interactions := BuildInteractionStats(idx, q)
		res.InteractionStats = &interactions
		res.EvidencePack = evidenceFromInteractionStats(interactions)
	case "frame_window", "render_pipeline":
		frame := getFrame()
		res.FramePipeline = &frame
		res.SpanWindows = frameSpans(frame)
		res.EvidencePack = evidenceFromFramePipeline(frame)
	case "frame_timeline", "frame_flow":
		timeline := getFrameTimeline()
		res.FrameTimeline = &timeline
		res.SpanWindows = frameTimelineSpans(timeline)
		res.EvidencePack = evidenceFromFrameTimeline(timeline)
	case "critical_blocking_calls":
		blocking := getBlocking()
		res.CriticalBlocking = &blocking
		res.EvidencePack = evidenceFromCriticalBlocking(blocking)
	case "recipe":
		recipe := BuildRecipe(idx, q)
		if recipeShouldUseDiscoveryOnly(q, recipe, explicitWindowOrSelector) {
			recipe.IncludedViews = []string{"frame_window", "frame_timeline", "frame_flow"}
			recipe.Caveats = append(recipe.Caveats, "unbounded jank recipe ran in discovery mode because no time, line, span, pid/thread, or event filters were provided; select a frame/span/window before requesting full root-cause/resource ranking")
			res.Caveats = append(res.Caveats, "large recipe guard: unbounded jank analysis skips full-trace scheduler/resource/root-cause expansion until the query is narrowed")
		}
		res.Recipe = &recipe
		if recipeHasView(recipe, "window_stats") {
			stats := getStats()
			res.WindowStats = &stats
		}
		if recipeHasView(recipe, "scheduler_latency_stats") {
			latency := getLatency()
			res.SchedulerLatency = &latency
			res.EvidencePack = append(res.EvidencePack, evidenceFromSchedulerLatency(latency)...)
		}
		if recipeHasView(recipe, "wakeup_chain") {
			chain := getChain()
			res.WakeupChain = &chain
			res.EvidencePack = append(res.EvidencePack, evidenceFromChain(chain)...)
		}
		if recipeHasView(recipe, "ipc_graph") {
			ipc := getIPC()
			res.IPCGraph = &ipc
			res.EvidencePack = append(res.EvidencePack, evidenceFromIPCGraph(ipc)...)
		}
		if recipeHasView(recipe, "root_cause_rank") {
			rank := getRootCause()
			res.RootCauseRank = &rank
			res.EvidencePack = append(res.EvidencePack, evidenceFromRootCauseRank(rank)...)
		}
		if recipeHasView(recipe, "critical_blocking_calls") {
			blocking := getBlocking()
			res.CriticalBlocking = &blocking
			res.EvidencePack = append(res.EvidencePack, evidenceFromCriticalBlocking(blocking)...)
		}
		if recipeHasView(recipe, "frame_window") || recipeHasView(recipe, "render_pipeline") {
			frame := getFrame()
			res.FramePipeline = &frame
			res.SpanWindows = frameSpans(frame)
			res.EvidencePack = append(res.EvidencePack, evidenceFromFramePipeline(frame)...)
		}
		if recipeHasView(recipe, "frame_timeline") || recipeHasView(recipe, "frame_flow") {
			timeline := getFrameTimeline()
			res.FrameTimeline = &timeline
			res.EvidencePack = append(res.EvidencePack, evidenceFromFrameTimeline(timeline)...)
		}
		if recipeHasView(recipe, "frame_root_cause_bundle") {
			bundle := BuildFrameRootCauseBundle(idx, q)
			res.FrameRootCauseBundle = &bundle
			if bundle.RootCauseRank != nil {
				res.EvidencePack = append(res.EvidencePack, evidenceFromRootCauseRank(*bundle.RootCauseRank)...)
			}
			if bundle.CriticalBlocking != nil {
				res.EvidencePack = append(res.EvidencePack, evidenceFromCriticalBlocking(*bundle.CriticalBlocking)...)
			}
		}
	case "evidence_pack":
		chain := getChain()
		stats := getStats()
		ipc := getIPC()
		latency := getLatency()
		blocking := getBlocking()
		res.WakeupChain = &chain
		res.WindowStats = &stats
		res.IPCGraph = &ipc
		res.SchedulerLatency = &latency
		res.CriticalBlocking = &blocking
		res.EvidencePack = append(evidenceFromChain(chain), evidenceFromStats(stats)...)
		res.EvidencePack = append(res.EvidencePack, evidenceFromIPCGraph(ipc)...)
		res.EvidencePack = append(res.EvidencePack, evidenceFromSchedulerLatency(latency)...)
		res.EvidencePack = append(res.EvidencePack, evidenceFromCriticalBlocking(blocking)...)
		bundle := BuildFrameRootCauseBundle(idx, q)
		res.FrameRootCauseBundle = &bundle
	default:
		res.View = "event_search"
		res.Events = EventSearch(idx, q)
		res.EvidencePack = evidenceFromEvents(res.Events)
	}
	res.Caveats = append(res.Caveats, flavorCaveats...)
	res.Caveats = append(res.Caveats, platformCaveats...)
	if idx.ParseLinePanics > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("%d trace line(s) could not be parsed and were skipped; results may undercount events near those lines", idx.ParseLinePanics))
	}
	if idx.ClockRegressions > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("%d timestamp regression(s) detected in the trace (clock moved backwards); duration and ordering metrics around those points are unreliable", idx.ClockRegressions))
	}
	if idx.UnparsedLines > 0 && idx.ScannedLineCount > 0 && float64(idx.UnparsedLines) > unparsedLineCaveatRatio*float64(idx.ScannedLineCount) {
		res.Caveats = append(res.Caveats, fmt.Sprintf("%d of %d scanned lines did not match any known trace format; coverage may be incomplete", idx.UnparsedLines, idx.ScannedLineCount))
	}
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
	switch q.View {
	case "frame_bundle", "frame_rootcause_bundle", "frame_root_cause":
		q.View = "frame_root_cause_bundle"
	}
	if q.TraceFlavor == "" {
		q.TraceFlavor = TraceFlavorGenericFtrace
	}
	return q
}

func ensureQueryFlavor(idx *Index, q Query) Query {
	if q.TraceFlavor != "" && q.TraceFlavor != TraceFlavorAuto && q.TraceFlavor != TraceFlavorGenericFtrace {
		return q
	}
	flavor, _, _, _ := resolveTraceFlavor(idx, q)
	if flavor == "" || flavor == TraceFlavorAuto {
		flavor = TraceFlavorGenericFtrace
	}
	q.TraceFlavor = flavor
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
	if len(typeSet) > 0 && !eventTypeMatches(ev, typeSet) {
		return false
	}
	if strings.TrimSpace(q.Pattern) != "" && !eventMatchesPattern(ev, q.Pattern) {
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

func eventMatchesPattern(ev Event, pattern string) bool {
	needle := strings.ToLower(strings.TrimSpace(pattern))
	if needle == "" {
		return true
	}
	for _, candidate := range []string{
		string(ev.Type),
		ev.Name,
		ev.FieldText,
		ev.Comm,
		ev.PrevComm,
		ev.NextComm,
		ev.WakeeComm,
		ev.PrevState,
		ev.NextInfo,
		ev.NextInfoAffinity,
		ev.CGroup,
		ev.SchedStatKind,
		ev.SchedStatComm,
		ev.ConstraintComm,
		ev.ConstraintKind,
		ev.ConstraintPolicy,
		ev.AllowedCPUsText,
		ev.CPUSet,
		ev.Reason,
		ev.SpanAction,
		ev.SpanName,
		ev.SpanValue,
		ev.ClockName,
		ev.BinderFlags,
		ev.BinderCode,
		ev.BinderLockTag,
		ev.BlockDev,
		ev.BlockOp,
		ev.BlockError,
		ev.BlockSrcDev,
		ev.IRQName,
		ev.IPITargetMask,
		ev.MemoryKind,
		ev.SubsystemKind,
		ev.ResourcePath,
		ev.ResourceOp,
		ev.ResourceAddress,
		ev.ResourceCallstack,
		ev.FSDev,
		ev.Inode,
		ev.ParentInode,
		ev.EntryName,
		ev.FileRW,
		ev.PluginDomain,
		ev.PluginEventName,
		ev.PluginMetric,
		ev.PluginValue,
		ev.PluginCategory,
		ev.PerfComm,
		ev.PerfEvent,
		ev.PerfSymbol,
		ev.PerfDSO,
		ev.PerfIP,
		ev.PerfCallchain,
		ev.PerfSource,
		ev.PerfSymbolizationStatus,
		ev.PerfClock,
		ev.PerfClockConfidence,
		ev.PerfCallchainStatus,
	} {
		if strings.Contains(strings.ToLower(candidate), needle) {
			return true
		}
	}
	for _, value := range []int{
		ev.Line,
		ev.CPU,
		ev.PID,
		ev.TGID,
		ev.PrevPID,
		ev.PrevPrio,
		ev.NextPID,
		ev.NextPrio,
		ev.NextInfoLoad,
		ev.NextInfoGroup,
		ev.NextInfoExpel,
		ev.NextInfoCGID,
		ev.WakeePID,
		ev.WakeePrio,
		ev.TargetCPU,
		ev.SchedStatPID,
		ev.ConstraintPID,
		ev.ConstraintCPU,
		ev.ConstraintOrigCPU,
		ev.ConstraintDestCPU,
		ev.State,
		ev.Frequency,
		ev.FrequencyMin,
		ev.FrequencyMax,
		ev.CPUForField,
		ev.IOWait,
		ev.BinderTransactionID,
		ev.BinderDestProc,
		ev.BinderDestThread,
		ev.BinderReply,
		ev.BinderDebugID,
		ev.IRQID,
		ev.PerfPID,
		ev.PerfTID,
	} {
		if value != 0 && strings.Contains(fmt.Sprintf("%d", value), needle) {
			return true
		}
	}
	for _, value := range []int64{
		ev.BinderDataSize,
		ev.BinderOffsetsSize,
		ev.BinderExtraSize,
		ev.BlockSector,
		ev.BlockLen,
		ev.BlockSrcSector,
		ev.SchedStatDelayNs,
		ev.SchedStatRunNs,
		ev.SchedStatVRunNs,
		ev.ResourceBytes,
		ev.FileOffset,
		ev.FileLen,
		ev.FileRet,
		ev.FileSize,
		ev.PerfPeriod,
	} {
		if value != 0 && strings.Contains(fmt.Sprintf("%d", value), needle) {
			return true
		}
	}
	if ev.ResourceLatencyMs != 0 && strings.Contains(fmt.Sprintf("%.3f", ev.ResourceLatencyMs), needle) {
		return true
	}
	if ev.Ts != 0 && strings.Contains(fmt.Sprintf("%.6f", ev.Ts), needle) {
		return true
	}
	return false
}

func eventTypeMatches(ev Event, typeSet map[EventType]bool) bool {
	typ := ev.Type
	if typeSet[typ] {
		return true
	}
	// Compatibility: older prompts use cpu_frequency to discover both concrete
	// frequency residency rows and frequency-limit rows. Residency math still
	// only consumes EventCPUFrequency.
	if typ == EventCPUFrequencyLimit && typeSet[EventCPUFrequency] {
		return true
	}
	name := strings.ToLower(ev.Name)
	switch {
	case typeSet["file_io"] && isFileIOEvent(ev):
		return true
	case typeSet["page_cache"] && isPageCacheEvent(ev):
		return true
	case typeSet["android_fs"] && strings.HasPrefix(name, "android_fs_"):
		return true
	case typeSet["f2fs"] && strings.HasPrefix(name, "f2fs_"):
		return true
	case typeSet["scsi"] && strings.HasPrefix(name, "scsi_"):
		return true
	case typeSet["mmc"] && strings.HasPrefix(name, "mmc_"):
		return true
	case typeSet["storage_latency"] && isStorageLatencyEvent(ev):
		return true
	case typeSet["io_pressure"] && (isStorageLatencyEvent(ev) || isFileIOEvent(ev) || isPageCacheEvent(ev) || ev.Type == EventSchedBlockedReason):
		return true
	case typeSet["cpu_constraint"] && isCPUConstraintEvidence(ev):
		return true
	default:
		return false
	}
}

func isCPUConstraintEvidence(ev Event) bool {
	return ev.Type == EventCPUConstraint || (ev.Type == EventSchedSwitch && (len(ev.NextInfoAllowedCPUs) > 0 || ev.NextInfoRestricted || strings.TrimSpace(ev.NextInfoAffinity) != ""))
}

func eventMentionsPID(ev Event, pid int) bool {
	return ev.PID == pid || ev.TGID == pid || ev.PrevPID == pid || ev.NextPID == pid || ev.WakeePID == pid || ev.SchedStatPID == pid || ev.ConstraintPID == pid || ev.BinderDestProc == pid || ev.BinderDestThread == pid || ev.PerfPID == pid || ev.PerfTID == pid
}

func eventMentionsThread(ev Event, thread string) bool {
	sel := parseThreadSelector(thread)
	if sel.HasPID && eventMentionsPID(ev, sel.PID) {
		return true
	}
	for _, v := range []string{ev.Comm, ev.PrevComm, ev.NextComm, ev.WakeeComm, ev.SchedStatComm, ev.ConstraintComm, ev.PerfComm, ev.PerfSymbol, ev.PerfDSO, ev.PerfCallchain, ev.SpanName, ev.SpanValue, ev.Reason, ev.IRQName, ev.FieldText} {
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
	durationMs := (end - start) * 1000
	return Interval{
		Thread:           thread,
		State:            state,
		StartTs:          start,
		EndTs:            end,
		DurationMs:       durationMs,
		ActualStartTs:    start,
		ActualEndTs:      end,
		ActualDurationMs: durationMs,
		StartLine:        startLine,
		EndLine:          endLine,
		WakeupLine:       wakeLine,
		PrevStateRaw:     prevState,
		Summary:          fmt.Sprintf("%s for %.3f ms", state, durationMs),
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
		if it.ActualStartTs == 0 && it.ActualEndTs == 0 {
			it.ActualStartTs = it.StartTs
			it.ActualEndTs = it.EndTs
			it.ActualDurationMs = it.DurationMs
		}
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
	fileIO := map[string]*FileIOSummary{}
	pageCache := map[string]*PageCacheSummary{}
	abilityEvents := map[string]*TracePluginSummary{}
	xpowerEvents := map[string]*TracePluginSummary{}
	hiSystemEvents := map[string]*TracePluginSummary{}
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
		case EventAbilityMonitor:
			stats.AbilityEventCount++
		case EventXPower:
			stats.XPowerEventCount++
		case EventHiSystemEvent:
			stats.HiSystemEventCount++
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
		case EventSchedStat:
			stats.SchedStatCount++
		case EventIPI:
			stats.IPICount++
		}
		accumulateSubsystemEvent(subsystems, ev)
		accumulateRuntimeResource(bioResources, filesystemResources, pageFaultResources, ev)
		accumulateFileIO(fileIO, ev)
		accumulatePageCache(pageCache, ev)
		accumulateTracePluginEvent(abilityEvents, xpowerEvents, hiSystemEvents, ev)
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
				if td.StartTs == 0 || start < td.StartTs {
					td.StartTs = start
				}
				if end > td.EndTs {
					td.EndTs = end
				}
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
				accumulateThreadDuration(acc.running, td.Thread, dur, cpu, freq, start, end, ev.Line, ev.NextPrio, td.PriorityClass)
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
	coreByCPU, topologySource := resolveCoreTopology(stats.CPU, q.CoreTopology)
	applyCPUCoreClasses(stats.CPU, coreByCPU)
	stats.RunnableTop, stats.DStateTop, stats.CPUPressure = computeOffCPUStats(idx, q, freqByCPU, pressure)
	applyThreadCoreClasses(stats.TopRunning, coreByCPU)
	applyThreadCoreClasses(stats.RunnableTop, coreByCPU)
	applyThreadCoreClasses(stats.DStateTop, coreByCPU)
	applyCPUPressureCoreClasses(stats.CPUPressure, coreByCPU)
	stats.CoreTopology = buildCoreClassStats(stats.CPU, stats.CPUPressure, coreByCPU, topologySource)
	stats.CPUConstraints = computeCPUConstraintSummaries(idx, q, coreByCPU, stats.RunnableTop, stats.CPU, 8)
	stats.ThreadCPULoad = computeThreadCPULoad(q, stats.TopRunning, stats.RunnableTop, 12)
	stats.ProcessCPULoad = computeProcessCPULoad(idx, q, stats.ThreadCPULoad, coreByCPU, 8)
	stats.IOLatencies = computeIOLatencies(idx, q, 8)
	stats.CPUFrequencyLimits = sortedCPUFrequencyLimits(freqLimits, 8)
	stats.SubsystemEvents = sortedSubsystemEvents(subsystems, 12)
	stats.BIOResources = sortedRuntimeResources(bioResources, 8)
	stats.FilesystemResources = sortedRuntimeResources(filesystemResources, 8)
	stats.PageFaultResources = sortedRuntimeResources(pageFaultResources, 8)
	stats.FileIOByInode = sortedFileIOSummaries(fileIO, 8)
	stats.PageCacheByInode = sortedPageCacheSummaries(pageCache, 8)
	stats.StorageLatencyByLayer = computeStorageLatencyByLayer(idx, q, stats.IOLatencies, 8)
	stats.AbilityEvents = sortedTracePluginSummaries(abilityEvents, 8)
	stats.XPowerEvents = sortedTracePluginSummaries(xpowerEvents, 8)
	stats.HiSystemEvents = sortedTracePluginSummaries(hiSystemEvents, 8)
	stats.Caveats = append(stats.Caveats, ioPairingCaveats(idx, q)...)
	stats.BlockedReasons = topBlockedReasons(blockedReasons, 8)
	stats.TraceSpans, stats.TraceCounters = computeTraceMarks(idx, q, 8)
	stats.TraceMarkCategories = computeTraceMarkCategories(stats.TraceSpans, 8)
	stats.AsyncFileWork = computeAsyncFileWorkSummaries(stats.TraceSpans, 8)
	stats.IRQBursts = computeIRQBursts(idx, q, 8)
	stats.IRQActivity = computeInterruptActivity(idx, q, EventIRQ, coreByCPU, 8)
	stats.SoftIRQActivity = computeInterruptActivity(idx, q, EventSoftIRQ, coreByCPU, 8)
	stats.IPIActivity = computeInterruptActivity(idx, q, EventIPI, coreByCPU, 8)
	stats.WorkqueueActivity = computeWorkqueueActivity(idx, q, 8)
	stats.SchedStatAccounting = computeSchedStatAccounting(idx, q, 8)
	stats.MemoryKinds = computeMemoryKinds(idx, q, 8)
	stats.ThreadDrifts = detectThreadDrifts(idx, q, 8)
	for _, drift := range stats.ThreadDrifts {
		if drift.Caveat != "" {
			stats.Caveats = append(stats.Caveats, drift.Caveat)
		}
	}
	stats.ComputeSupply = computeSupplySummaries(stats, 8)
	stats.StateChurn = enrichStateChurnWithCPUPressure(computeStateChurnSummaries(idx, q, 8), stats.CPUPressure)
	latency := buildSchedulerLatencyStatsFromStats(idx, q, stats)
	stats.RunnableContext = computeRunnableContextSummaries(latency.Items, stats.ThreadCPULoad, stats.ProcessCPULoad, stats.CPUConstraints, 8)
	stats.IOPressureSummary = computeIOPressureSummary(stats)
	stats.BlockIOByInode = computeBlockIOByInode(stats, 8)
	stats.IOBurstEpisodes = computeIOBurstEpisodes(stats, 8)
	stats.SupplyPressureSummary = computeSupplyPressureSummary(idx, q, stats, 8)
	stats.PerfSamples = computePerfContext(idx, q, 8)
	return stats
}

type perfHotspotAcc struct {
	item      PerfHotspot
	threadSet map[string]ThreadRef
	cpuSet    map[int]bool
	total     *int64
}

type perfThreadAcc struct {
	item   PerfThreadSummary
	cpuSet map[int]bool
	total  *int64
}

type perfValueCountAcc struct {
	Value       string
	SampleCount int
	Period      int64
}

type perfQualityAcc struct {
	sources               map[string]*perfValueCountAcc
	symbolizationStatuses map[string]*perfValueCountAcc
	sampleKinds           map[string]*perfValueCountAcc
	weightUnits           map[string]*perfValueCountAcc
	clocks                map[string]*perfValueCountAcc
	clockConfidences      map[string]*perfValueCountAcc
	callchainStatuses     map[string]*perfValueCountAcc
	cpuKnownCount         int
	cpuUnknownCount       int
	callchainKnownCount   int
	callchainUnknownCount int
}

type perfSampleFilter func(Event) bool

func computePerfContext(idx *Index, q Query, max int) *PerfContext {
	return computePerfContextFiltered(idx, q, max, nil)
}

func computePerfContextFiltered(idx *Index, q Query, max int, filter perfSampleFilter) *PerfContext {
	if idx == nil {
		return nil
	}
	ctx := &PerfContext{}
	bySymbol := map[string]*perfHotspotAcc{}
	byDSO := map[string]*perfHotspotAcc{}
	byCallchain := map[string]*perfHotspotAcc{}
	byEvent := map[string]*perfHotspotAcc{}
	byThread := map[string]*perfThreadAcc{}
	quality := newPerfQualityAcc()
	for _, ev := range idx.Events {
		if ev.Type != EventPerfSample || !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		if filter != nil && !filter(ev) {
			continue
		}
		period := ev.PerfPeriod
		if period <= 0 {
			period = 1
		}
		weightUnit := perfSampleWeightUnit(ev)
		ctx.SampleCount++
		ctx.TotalPeriod += period
		quality.add(ev, period)
		thread := perfSampleThread(ev)
		example := perfSampleExample(ev)
		addPerfHotspot(bySymbol, firstNonEmpty(ev.PerfSymbol, ev.PerfIP, "unknown"), PerfHotspot{
			Symbol:              ev.PerfSymbol,
			DSO:                 ev.PerfDSO,
			Event:               ev.PerfEvent,
			WeightUnit:          weightUnit,
			Source:              ev.PerfSource,
			SymbolizationStatus: ev.PerfSymbolizationStatus,
		}, thread, ev.CPU, ev.Line, period, example, &ctx.TotalPeriod)
		addPerfHotspot(byDSO, firstNonEmpty(ev.PerfDSO, "unknown"), PerfHotspot{
			DSO:                 ev.PerfDSO,
			Event:               ev.PerfEvent,
			WeightUnit:          weightUnit,
			Source:              ev.PerfSource,
			SymbolizationStatus: ev.PerfSymbolizationStatus,
		}, thread, ev.CPU, ev.Line, period, example, &ctx.TotalPeriod)
		addPerfHotspot(byCallchain, firstNonEmpty(ev.PerfCallchain, ev.PerfSymbol, ev.PerfIP, "unknown"), PerfHotspot{
			Symbol:              ev.PerfSymbol,
			DSO:                 ev.PerfDSO,
			Callchain:           ev.PerfCallchain,
			Event:               ev.PerfEvent,
			WeightUnit:          weightUnit,
			Source:              ev.PerfSource,
			SymbolizationStatus: ev.PerfSymbolizationStatus,
		}, thread, ev.CPU, ev.Line, period, example, &ctx.TotalPeriod)
		addPerfHotspot(byEvent, firstNonEmpty(ev.PerfEvent, "unknown"), PerfHotspot{
			Event:               ev.PerfEvent,
			WeightUnit:          weightUnit,
			Source:              ev.PerfSource,
			SymbolizationStatus: ev.PerfSymbolizationStatus,
		}, thread, ev.CPU, ev.Line, period, example, &ctx.TotalPeriod)
		addPerfThread(byThread, thread, ev.CPU, ev.Line, period, example, &ctx.TotalPeriod)
	}
	if ctx.SampleCount == 0 {
		return nil
	}
	ctx.TopSymbols = sortedPerfHotspots(bySymbol, max)
	ctx.TopDSO = sortedPerfHotspots(byDSO, max)
	ctx.TopCallchains = sortedPerfHotspots(byCallchain, max)
	ctx.TopThreads = sortedPerfThreads(byThread, max)
	ctx.TopEvents = sortedPerfHotspots(byEvent, max)
	ctx.Quality = quality.summary(ctx.TotalPeriod)
	return ctx
}

func perfContextForThread(idx *Index, q Query, thread ThreadRef, start, end float64, max int) *PerfContext {
	if thread.PID <= 0 && strings.TrimSpace(thread.Comm) == "" {
		return nil
	}
	sub := queryForPerfContextWindow(q, start, end)
	return computePerfContextFiltered(idx, sub, max, func(ev Event) bool {
		return perfSampleMatchesThread(ev, thread)
	})
}

func perfContextForThreads(idx *Index, q Query, threads map[int]ThreadRef, max int) *PerfContext {
	if len(threads) == 0 {
		return nil
	}
	return computePerfContextFiltered(idx, q, max, func(ev Event) bool {
		for _, thread := range threads {
			if perfSampleMatchesThread(ev, thread) {
				return true
			}
		}
		return false
	})
}

func perfContextForCPUs(idx *Index, q Query, cpus map[int]bool, max int) *PerfContext {
	if len(cpus) == 0 {
		return nil
	}
	return computePerfContextFiltered(idx, q, max, func(ev Event) bool {
		return ev.CPU >= 0 && cpus[ev.CPU]
	})
}

func queryForPerfContextWindow(q Query, start, end float64) Query {
	if start > 0 {
		q.TimeStart = start
	}
	if end > 0 && (q.TimeStart <= 0 || end > q.TimeStart) {
		q.TimeEnd = end
	}
	return q
}

func perfSampleMatchesThread(ev Event, thread ThreadRef) bool {
	if thread.PID > 0 {
		if ev.PerfTID == thread.PID || ev.PID == thread.PID {
			return true
		}
		if ev.PerfPID == thread.PID || ev.TGID == thread.PID {
			return true
		}
	}
	if thread.Comm != "" {
		if strings.EqualFold(thread.Comm, ev.PerfComm) || strings.EqualFold(thread.Comm, ev.Comm) {
			return true
		}
	}
	return false
}

func perfSampleThread(ev Event) ThreadRef {
	return ThreadRef{
		Comm: firstNonEmpty(ev.PerfComm, ev.Comm),
		PID:  firstNonZero(ev.PerfTID, ev.PID),
		TGID: firstNonZero(ev.PerfPID, ev.TGID),
	}
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func perfSampleExample(ev Event) string {
	parts := []string{}
	if ev.PerfSymbol != "" {
		parts = append(parts, "symbol="+ev.PerfSymbol)
	}
	if ev.PerfDSO != "" {
		parts = append(parts, "dso="+ev.PerfDSO)
	}
	if ev.PerfEvent != "" {
		parts = append(parts, "event="+ev.PerfEvent)
	}
	if ev.PerfPeriod > 0 {
		parts = append(parts, fmt.Sprintf("sample_weight=%d", ev.PerfPeriod))
	}
	if unit := perfSampleWeightUnit(ev); unit != "" {
		parts = append(parts, "weight_unit="+unit)
	}
	if ev.PerfIP != "" {
		parts = append(parts, "ip="+ev.PerfIP)
	}
	if ev.PerfAddr != "" && ev.PerfAddr != ev.PerfIP {
		parts = append(parts, "addr="+ev.PerfAddr)
	}
	if ev.PerfSampleID != "" {
		parts = append(parts, "sample_id="+ev.PerfSampleID)
	}
	if ev.PerfStreamID != "" {
		parts = append(parts, "stream_id="+ev.PerfStreamID)
	}
	if ev.PerfRawWeight > 0 {
		parts = append(parts, fmt.Sprintf("perf_weight=%d", ev.PerfRawWeight))
	}
	if ev.PerfDataSrc != "" {
		parts = append(parts, "data_src="+ev.PerfDataSrc)
	}
	if ev.PerfTransaction != "" {
		parts = append(parts, "transaction="+ev.PerfTransaction)
	}
	if ev.PerfPhysAddr != "" {
		parts = append(parts, "phys_addr="+ev.PerfPhysAddr)
	}
	if ev.PerfCGroupID != "" {
		parts = append(parts, "cgroup_id="+ev.PerfCGroupID)
	}
	if ev.PerfDataPageSize > 0 {
		parts = append(parts, fmt.Sprintf("data_page_size=%d", ev.PerfDataPageSize))
	}
	if ev.PerfCodePageSize > 0 {
		parts = append(parts, fmt.Sprintf("code_page_size=%d", ev.PerfCodePageSize))
	}
	if ev.PerfRawSize > 0 {
		parts = append(parts, fmt.Sprintf("raw_size=%d", ev.PerfRawSize))
	}
	if ev.PerfBranchCount > 0 {
		parts = append(parts, fmt.Sprintf("branch_count=%d", ev.PerfBranchCount))
	}
	if ev.PerfUserRegsABI != "" {
		parts = append(parts, "user_regs_abi="+ev.PerfUserRegsABI)
	}
	if ev.PerfUserRegsCount > 0 {
		parts = append(parts, fmt.Sprintf("user_regs_count=%d", ev.PerfUserRegsCount))
	}
	if ev.PerfUserStackSize > 0 {
		parts = append(parts, fmt.Sprintf("user_stack_size=%d", ev.PerfUserStackSize))
	}
	if ev.PerfAuxSize > 0 {
		parts = append(parts, fmt.Sprintf("aux_size=%d", ev.PerfAuxSize))
	}
	if ev.PerfSource != "" {
		parts = append(parts, "source="+ev.PerfSource)
	}
	if ev.PerfSymbolizationStatus != "" {
		parts = append(parts, "symbolization_status="+ev.PerfSymbolizationStatus)
	}
	if ev.PerfSampleKind != "" {
		parts = append(parts, "sample_kind="+ev.PerfSampleKind)
	}
	if ev.PerfCallchainStatus != "" {
		parts = append(parts, "callchain_status="+ev.PerfCallchainStatus)
	}
	if ev.PerfClockConfidence != "" {
		parts = append(parts, "clock_confidence="+ev.PerfClockConfidence)
	}
	if ev.PerfCPUKnown != nil {
		parts = append(parts, fmt.Sprintf("cpu_known=%t", *ev.PerfCPUKnown))
	}
	if ev.PerfCallchain != "" {
		parts = append(parts, "callchain="+ev.PerfCallchain)
	}
	if len(parts) == 0 {
		return ev.FieldText
	}
	return strings.Join(parts, " ")
}

func newPerfQualityAcc() *perfQualityAcc {
	return &perfQualityAcc{
		sources:               map[string]*perfValueCountAcc{},
		symbolizationStatuses: map[string]*perfValueCountAcc{},
		sampleKinds:           map[string]*perfValueCountAcc{},
		weightUnits:           map[string]*perfValueCountAcc{},
		clocks:                map[string]*perfValueCountAcc{},
		clockConfidences:      map[string]*perfValueCountAcc{},
		callchainStatuses:     map[string]*perfValueCountAcc{},
	}
}

func (acc *perfQualityAcc) add(ev Event, period int64) {
	if acc == nil {
		return
	}
	addPerfValueCount(acc.sources, firstNonEmpty(ev.PerfSource, "unknown"), period)
	addPerfValueCount(acc.symbolizationStatuses, firstNonEmpty(ev.PerfSymbolizationStatus, "unknown"), period)
	addPerfValueCount(acc.sampleKinds, firstNonEmpty(ev.PerfSampleKind, "unknown"), period)
	addPerfValueCount(acc.weightUnits, perfSampleWeightUnit(ev), period)
	addPerfValueCount(acc.clocks, firstNonEmpty(ev.PerfClock, "unknown"), period)
	addPerfValueCount(acc.clockConfidences, firstNonEmpty(ev.PerfClockConfidence, "unknown"), period)
	addPerfValueCount(acc.callchainStatuses, firstNonEmpty(ev.PerfCallchainStatus, "unknown"), period)
	if ev.PerfCPUKnown != nil && *ev.PerfCPUKnown {
		acc.cpuKnownCount++
	} else {
		acc.cpuUnknownCount++
	}
	if perfCallchainKnownForQuality(ev.PerfCallchainStatus) {
		acc.callchainKnownCount++
	} else {
		acc.callchainUnknownCount++
	}
}

func (acc *perfQualityAcc) summary(total int64) *PerfQualitySummary {
	if acc == nil {
		return nil
	}
	out := &PerfQualitySummary{
		Sources:               sortedPerfValueCounts(acc.sources, total),
		SymbolizationStatuses: sortedPerfValueCounts(acc.symbolizationStatuses, total),
		SampleKinds:           sortedPerfValueCounts(acc.sampleKinds, total),
		WeightUnits:           sortedPerfValueCounts(acc.weightUnits, total),
		Clocks:                sortedPerfValueCounts(acc.clocks, total),
		ClockConfidences:      sortedPerfValueCounts(acc.clockConfidences, total),
		CallchainStatuses:     sortedPerfValueCounts(acc.callchainStatuses, total),
		CPUKnownCount:         acc.cpuKnownCount,
		CPUUnknownCount:       acc.cpuUnknownCount,
		CallchainKnownCount:   acc.callchainKnownCount,
		CallchainUnknownCount: acc.callchainUnknownCount,
	}
	out.Caveats = perfQualityCaveats(*out)
	if len(out.Sources) == 0 && len(out.SymbolizationStatuses) == 0 && len(out.SampleKinds) == 0 && len(out.WeightUnits) == 0 && len(out.Clocks) == 0 && out.CPUKnownCount == 0 && out.CPUUnknownCount == 0 {
		return nil
	}
	return out
}

func perfSampleWeightUnit(ev Event) string {
	event := strings.ToLower(strings.TrimSpace(ev.PerfEvent))
	sampleKind := strings.ToLower(strings.TrimSpace(ev.PerfSampleKind))
	switch {
	case event == "":
		return "event_count"
	case strings.Contains(event, "cpu-cycles") || strings.Contains(event, "cycles"):
		return "cycles"
	case strings.Contains(event, "instructions"):
		return "instructions"
	case strings.Contains(event, "cpu-clock") || strings.Contains(event, "task-clock"):
		if sampleKind == "off_cpu" {
			return "ns_off_cpu_event"
		}
		if sampleKind == "on_cpu" {
			return "ns_on_cpu_event"
		}
		return "ns_clock_event"
	case strings.Contains(event, "sched_switch") && sampleKind == "off_cpu":
		return "ns_off_cpu_event"
	default:
		return "event_count"
	}
}

func addPerfValueCount(bucket map[string]*perfValueCountAcc, value string, period int64) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "unknown"
	}
	acc := bucket[value]
	if acc == nil {
		acc = &perfValueCountAcc{Value: value}
		bucket[value] = acc
	}
	acc.SampleCount++
	acc.Period += period
}

func sortedPerfValueCounts(in map[string]*perfValueCountAcc, total int64) []PerfValueCount {
	out := make([]PerfValueCount, 0, len(in))
	for _, acc := range in {
		item := PerfValueCount{Value: acc.Value, SampleCount: acc.SampleCount, Period: acc.Period}
		if total > 0 {
			item.Percent = float64(item.Period) * 100 / float64(total)
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Period != out[j].Period {
			return out[i].Period > out[j].Period
		}
		if out[i].SampleCount != out[j].SampleCount {
			return out[i].SampleCount > out[j].SampleCount
		}
		return out[i].Value < out[j].Value
	})
	return out
}

func perfQualityCaveats(q PerfQualitySummary) []string {
	var out []string
	if q.CPUUnknownCount > 0 {
		out = append(out, fmt.Sprintf("perf samples include %d CPU-unknown sample(s); CPU-scoped joins must keep them out of concrete CPU/core attribution", q.CPUUnknownCount))
	}
	if perfValueCountsContain(q.SymbolizationStatuses, "unsymbolized") {
		out = append(out, "perf samples include unsymbolized/IP-only rows; function-level conclusions should keep raw fallback caveats")
	}
	if perfValueCountsContain(q.SampleKinds, "off_cpu") {
		out = append(out, "perf samples include off_cpu rows; do not narrate those periods as running CPU execution")
	}
	if perfValueCountsContain(q.SampleKinds, "unknown") {
		out = append(out, "perf samples include unknown sample_kind rows; keep on-cpu/off-cpu interpretation as a caveat")
	}
	if perfValueCountsContain(q.CallchainStatuses, "missing") || perfValueCountsContain(q.CallchainStatuses, "ip_only") {
		out = append(out, "perf samples include missing or IP-only callchains; call-chain conclusions may be partial")
	}
	if perfValueCountsContain(q.ClockConfidences, "assumed") || perfValueCountsContain(q.ClockConfidences, "unknown") {
		out = append(out, "perf sample timestamps use assumed or unknown clock alignment; treat trace/perf overlap as supporting evidence unless calibrated")
	}
	if unit := perfQualityTopUnit(q.WeightUnits); unit != "" {
		out = append(out, "perf sample_weight unit hint is "+unit+"; keep it as an event weight unless the event definition explicitly defines a time unit")
	}
	out = append(out, "perf period/sample_weight values are event/sample weights, not elapsed duration; do not convert them to time or expected sample density without explicit sampling configuration and calibrated CPU frequency")
	return out
}

func perfQualityTopUnit(values []PerfValueCount) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0].Value)
}

func perfValueCountsContain(values []PerfValueCount, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value.Value)) == want && value.SampleCount > 0 {
			return true
		}
	}
	return false
}

func perfCallchainKnownForQuality(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "symbolized", "partial":
		return true
	default:
		return false
	}
}

func addPerfHotspot(bucket map[string]*perfHotspotAcc, key string, item PerfHotspot, thread ThreadRef, cpu int, line int, period int64, example string, total *int64) {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "unknown"
	}
	acc := bucket[key]
	if acc == nil {
		acc = &perfHotspotAcc{
			item:      item,
			threadSet: map[string]ThreadRef{},
			cpuSet:    map[int]bool{},
			total:     total,
		}
		bucket[key] = acc
	}
	acc.item.SampleCount++
	acc.item.Period += period
	if acc.item.LineStart == 0 || (line > 0 && line < acc.item.LineStart) {
		acc.item.LineStart = line
	}
	if line > acc.item.LineEnd {
		acc.item.LineEnd = line
	}
	if acc.item.Example == "" {
		acc.item.Example = example
	}
	if label := threadLabel(thread); label != "" {
		acc.threadSet[label] = thread
	}
	if cpu >= 0 {
		acc.cpuSet[cpu] = true
	}
}

func addPerfThread(bucket map[string]*perfThreadAcc, thread ThreadRef, cpu int, line int, period int64, example string, total *int64) {
	key := threadLabel(thread)
	if key == "" {
		key = "unknown"
	}
	acc := bucket[key]
	if acc == nil {
		acc = &perfThreadAcc{
			item:   PerfThreadSummary{Thread: thread},
			cpuSet: map[int]bool{},
			total:  total,
		}
		bucket[key] = acc
	}
	acc.item.SampleCount++
	acc.item.Period += period
	if acc.item.LineStart == 0 || (line > 0 && line < acc.item.LineStart) {
		acc.item.LineStart = line
	}
	if line > acc.item.LineEnd {
		acc.item.LineEnd = line
	}
	if acc.item.Example == "" {
		acc.item.Example = example
	}
	if cpu >= 0 {
		acc.cpuSet[cpu] = true
	}
}

func sortedPerfHotspots(in map[string]*perfHotspotAcc, max int) []PerfHotspot {
	out := make([]PerfHotspot, 0, len(in))
	for _, acc := range in {
		item := acc.item
		item.Threads = sortedThreadRefs(acc.threadSet)
		item.CPUs = sortedCPUs(acc.cpuSet)
		if acc.total != nil && *acc.total > 0 {
			item.Percent = float64(item.Period) * 100 / float64(*acc.total)
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Period != out[j].Period {
			return out[i].Period > out[j].Period
		}
		if out[i].SampleCount != out[j].SampleCount {
			return out[i].SampleCount > out[j].SampleCount
		}
		return perfHotspotLabel(out[i]) < perfHotspotLabel(out[j])
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func sortedPerfThreads(in map[string]*perfThreadAcc, max int) []PerfThreadSummary {
	out := make([]PerfThreadSummary, 0, len(in))
	for _, acc := range in {
		item := acc.item
		item.CPUs = sortedCPUs(acc.cpuSet)
		if acc.total != nil && *acc.total > 0 {
			item.Percent = float64(item.Period) * 100 / float64(*acc.total)
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Period != out[j].Period {
			return out[i].Period > out[j].Period
		}
		if out[i].SampleCount != out[j].SampleCount {
			return out[i].SampleCount > out[j].SampleCount
		}
		return threadLabel(out[i].Thread) < threadLabel(out[j].Thread)
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func sortedThreadRefs(in map[string]ThreadRef) []ThreadRef {
	out := make([]ThreadRef, 0, len(in))
	for _, thread := range in {
		out = append(out, thread)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return threadLabel(out[i]) < threadLabel(out[j])
	})
	return out
}

func sortedCPUs(in map[int]bool) []int {
	out := make([]int, 0, len(in))
	for cpu := range in {
		out = append(out, cpu)
	}
	sort.Ints(out)
	return out
}

func perfHotspotLabel(item PerfHotspot) string {
	return firstNonEmpty(item.Symbol, item.DSO, item.Callchain, item.Event, item.Example)
}

type perfTimelineBucketAcc struct {
	bucket    PerfTimelineBucket
	symbols   map[string]int64
	dsos      map[string]int64
	events    map[string]int64
	threadSet map[string]ThreadRef
	cpuSet    map[int]bool
}

func BuildPerfTimeline(idx *Index, q Query) PerfTimelineResult {
	q = ensureQueryFlavor(idx, q)
	start, end, count := perfTimelineWindow(idx, q)
	res := PerfTimelineResult{Window: TimeWindow{StartTs: start, EndTs: end}}
	if count == 0 {
		res.Caveats = append(res.Caveats, "no perf_sample events matched the selected window or filters")
		return res
	}
	bucketSec := q.MinDurationMs / 1000
	if bucketSec <= 0 {
		bucketSec = 0.001
	}
	if start < end {
		maxBuckets := 200.0
		if buckets := (end - start) / bucketSec; buckets > maxBuckets {
			bucketSec = (end - start) / maxBuckets
		}
	}
	if bucketSec <= 0 {
		bucketSec = 0.001
	}
	res.BucketMs = bucketSec * 1000
	buckets := map[int]*perfTimelineBucketAcc{}
	for _, ev := range idx.Events {
		if ev.Type != EventPerfSample || !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		if q.PID > 0 && !eventMentionsPID(ev, q.PID) {
			continue
		}
		if q.PID <= 0 && strings.TrimSpace(q.Thread) != "" && !eventMentionsThread(ev, q.Thread) {
			continue
		}
		idxBucket := 0
		if ev.Ts > start && bucketSec > 0 {
			idxBucket = int((ev.Ts - start) / bucketSec)
		}
		acc := buckets[idxBucket]
		if acc == nil {
			bStart := start + float64(idxBucket)*bucketSec
			acc = &perfTimelineBucketAcc{
				bucket:    PerfTimelineBucket{StartTs: bStart, EndTs: bStart + bucketSec},
				symbols:   map[string]int64{},
				dsos:      map[string]int64{},
				events:    map[string]int64{},
				threadSet: map[string]ThreadRef{},
				cpuSet:    map[int]bool{},
			}
			buckets[idxBucket] = acc
		}
		period := ev.PerfPeriod
		if period <= 0 {
			period = 1
		}
		acc.bucket.SampleCount++
		acc.bucket.Period += period
		if acc.bucket.LineStart == 0 || (ev.Line > 0 && ev.Line < acc.bucket.LineStart) {
			acc.bucket.LineStart = ev.Line
		}
		if ev.Line > acc.bucket.LineEnd {
			acc.bucket.LineEnd = ev.Line
		}
		if acc.bucket.Example == "" {
			acc.bucket.Example = perfSampleExample(ev)
		}
		if ev.PerfSymbol != "" {
			acc.symbols[ev.PerfSymbol] += period
		}
		if ev.PerfDSO != "" {
			acc.dsos[ev.PerfDSO] += period
		}
		if ev.PerfEvent != "" {
			acc.events[ev.PerfEvent] += period
		}
		if label := threadLabel(perfSampleThread(ev)); label != "" {
			acc.threadSet[label] = perfSampleThread(ev)
		}
		if ev.CPU >= 0 {
			acc.cpuSet[ev.CPU] = true
		}
	}
	keys := make([]int, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	for _, key := range keys {
		acc := buckets[key]
		acc.bucket.TopSymbol = topWeightedKey(acc.symbols)
		acc.bucket.TopDSO = topWeightedKey(acc.dsos)
		acc.bucket.TopEvent = topWeightedKey(acc.events)
		acc.bucket.Threads = sortedThreadRefs(acc.threadSet)
		acc.bucket.CPUs = sortedCPUs(acc.cpuSet)
		if acc.bucket.EndTs > end && end > start {
			acc.bucket.EndTs = end
		}
		res.Buckets = append(res.Buckets, acc.bucket)
	}
	return res
}

func perfTimelineWindow(idx *Index, q Query) (float64, float64, int) {
	start, end := q.TimeStart, q.TimeEnd
	count := 0
	for _, ev := range idx.Events {
		if ev.Type != EventPerfSample || !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		if q.PID > 0 && !eventMentionsPID(ev, q.PID) {
			continue
		}
		if q.PID <= 0 && strings.TrimSpace(q.Thread) != "" && !eventMentionsThread(ev, q.Thread) {
			continue
		}
		count++
		if start == 0 || ev.Ts < start {
			start = ev.Ts
		}
		if end == 0 || ev.Ts > end {
			end = ev.Ts
		}
	}
	if end < start {
		end = start
	}
	if end == start && count > 0 {
		end = start + 0.001
	}
	return start, end, count
}

func topWeightedKey(in map[string]int64) string {
	var best string
	var bestWeight int64
	for key, weight := range in {
		if best == "" || weight > bestWeight || (weight == bestWeight && key < best) {
			best = key
			bestWeight = weight
		}
	}
	return best
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

func accumulateThreadDuration(bucket map[string]ThreadDuration, thread ThreadRef, dur float64, cpu int, freq int, startTs, endTs float64, line int, priority int, priorityClass string) {
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
	if td.StartTs == 0 || (startTs > 0 && startTs < td.StartTs) {
		td.StartTs = startTs
	}
	if endTs > td.EndTs {
		td.EndTs = endTs
	}
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
		if td.StartTs == 0 || startTs < td.StartTs {
			td.StartTs = startTs
		}
		if endTs > td.EndTs {
			td.EndTs = endTs
		}
		if td.LineStart == 0 {
			td.LineStart = start.line
		}
		td.LineEnd = firstPositive(endLine, start.line)
		bucket[key] = td
		if start.state == StateRunnable {
			acc := cpuPressure(pressure, start.cpu)
			acc.runnableWaitMs += dur
			acc.runnableEvents++
			accumulateThreadDuration(acc.runnable, start.thread, dur, start.cpu, freq, startTs, endTs, firstPositive(endLine, start.line), start.priority, start.priorityClass)
		}
	}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) {
			continue
		}
		if ev.Type == EventSchedWakeup || ev.Type == EventSchedWaking {
			if ev.WakeePID <= 0 || !timeInWindow(ev.Ts, q) {
				continue
			}
			if start, ok := open[ev.WakeePID]; ok {
				switch start.state {
				case StateRunnable:
					continue
				case StateDSleep, StateIOWait:
					addDuration(dstate, start, ev.Ts, ev.Line)
				}
			}
			open[ev.WakeePID] = offCPUStart{
				thread:        ThreadRef{Comm: ev.WakeeComm, PID: ev.WakeePID},
				state:         StateRunnable,
				ts:            ev.Ts,
				line:          ev.Line,
				cpu:           ev.TargetCPU,
				priority:      ev.WakeePrio,
				priorityClass: classifyTracePriority(q.TraceFlavor, ev.WakeePrio),
			}
			continue
		}
		if ev.Type != EventSchedSwitch {
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
	stats := ComputeWindowStats(idx, q)
	return buildSchedulerLatencyStatsFromStats(idx, q, stats)
}

func buildSchedulerLatencyStatsFromStats(idx *Index, q Query, stats WindowStats) SchedulerLatencyResult {
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
	cpus := map[int]CPUStats{}
	for _, cpu := range stats.CPU {
		cpus[cpu.CPU] = cpu
	}
	pressure := map[int]CPUPressureStats{}
	for _, p := range stats.CPUPressure {
		pressure[p.CPU] = p
	}
	catalog := buildThreadCatalog(idx, q)
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
			CoreClass:             cpu.CoreClass,
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
		if item.CoreClass != "" {
			item.Summary = fmt.Sprintf("%s core_class=%s", item.Summary, item.CoreClass)
		}
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
			if ev.Type == EventSchedWakeup || ev.Type == EventSchedWaking {
				if ev.WakeePID > 0 {
					if target.PID > 0 || target.Comm != "" {
						if !threadMatches(target, ev.WakeePID, ev.WakeeComm) {
							continue
						}
					}
					if q.TimeEnd > 0 && ev.Ts > q.TimeEnd {
						continue
					}
					if q.TimeStart > 0 && ev.Ts < q.TimeStart {
						continue
					}
					if existing, ok := open[ev.WakeePID]; !ok || ev.Ts < existing.ts {
						open[ev.WakeePID] = startInfo{
							thread:        catalogThreadRef(catalog, ev.WakeePID, ev.WakeeComm),
							ts:            ev.Ts,
							line:          ev.Line,
							cpu:           ev.TargetCPU,
							priority:      ev.WakeePrio,
							priorityClass: classifyTracePriority(q.TraceFlavor, ev.WakeePrio),
						}
					}
				}
			}
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
				thread:        catalogThreadRef(catalog, ev.PrevPID, ev.PrevComm),
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

func buildThreadCatalog(idx *Index, q Query) map[int]ThreadRef {
	out := map[int]ThreadRef{}
	if idx == nil {
		return out
	}
	add := func(pid int, comm string, tgid int) {
		if pid <= 0 {
			return
		}
		ref := out[pid]
		if ref.PID == 0 {
			ref.PID = pid
		}
		if strings.TrimSpace(ref.Comm) == "" && strings.TrimSpace(comm) != "" {
			ref.Comm = strings.TrimSpace(comm)
		}
		if ref.TGID == 0 && tgid > 0 {
			ref.TGID = tgid
		}
		out[pid] = ref
	}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) {
			continue
		}
		add(ev.PID, ev.Comm, ev.TGID)
		add(ev.PrevPID, ev.PrevComm, 0)
		add(ev.NextPID, ev.NextComm, 0)
		add(ev.WakeePID, ev.WakeeComm, 0)
		add(ev.ConstraintPID, ev.ConstraintComm, 0)
	}
	return out
}

func catalogThreadRef(catalog map[int]ThreadRef, pid int, comm string) ThreadRef {
	ref := catalog[pid]
	if ref.PID == 0 {
		ref.PID = pid
	}
	if strings.TrimSpace(ref.Comm) == "" {
		ref.Comm = strings.TrimSpace(comm)
	}
	return ref
}

func threadKey(thread ThreadRef) string {
	if thread.PID > 0 {
		return fmt.Sprintf("pid:%d", thread.PID)
	}
	return "comm:" + strings.ToLower(strings.TrimSpace(thread.Comm))
}

type cpuConstraintAcc struct {
	item       CPUConstraintSummary
	allowedSet map[int]bool
}

func computeCPUConstraintSummaries(idx *Index, q Query, coreByCPU map[int]string, runnable []ThreadDuration, cpus []CPUStats, max int) []CPUConstraintSummary {
	if idx == nil {
		return nil
	}
	catalog := buildThreadCatalog(idx, q)
	runnableByPID := map[int]float64{}
	for _, td := range runnable {
		if td.Thread.PID > 0 {
			runnableByPID[td.Thread.PID] += td.DurationMs
		}
	}
	cpuByID := map[int]CPUStats{}
	for _, cpu := range cpus {
		cpuByID[cpu.CPU] = cpu
	}
	accs := map[string]*cpuConstraintAcc{}
	ensure := func(thread ThreadRef) *cpuConstraintAcc {
		key := threadKey(thread)
		if accs[key] == nil {
			accs[key] = &cpuConstraintAcc{item: CPUConstraintSummary{Thread: thread}, allowedSet: map[int]bool{}}
		}
		if accs[key].item.Thread.Comm == "" && thread.Comm != "" {
			accs[key].item.Thread.Comm = thread.Comm
		}
		if accs[key].item.Thread.TGID == 0 && thread.TGID > 0 {
			accs[key].item.Thread.TGID = thread.TGID
		}
		return accs[key]
	}
	addAllowed := func(acc *cpuConstraintAcc, cpus []int) {
		for _, cpu := range cpus {
			if cpu < 0 || acc.allowedSet[cpu] {
				continue
			}
			acc.allowedSet[cpu] = true
			acc.item.AllowedCPUs = append(acc.item.AllowedCPUs, cpu)
		}
	}
	updateLines := func(acc *cpuConstraintAcc, ev Event) {
		if acc.item.LineStart == 0 || (ev.Line > 0 && ev.Line < acc.item.LineStart) {
			acc.item.LineStart = ev.Line
		}
		if ev.Line > acc.item.LineEnd {
			acc.item.LineEnd = ev.Line
		}
		if acc.item.StartTs == 0 || (ev.Ts > 0 && ev.Ts < acc.item.StartTs) {
			acc.item.StartTs = ev.Ts
		}
		if ev.Ts > acc.item.EndTs {
			acc.item.EndTs = ev.Ts
		}
	}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		switch {
		case ev.Type == EventCPUConstraint:
			thread := catalogThreadRef(catalog, ev.ConstraintPID, ev.ConstraintComm)
			if thread.PID <= 0 && thread.Comm == "" {
				continue
			}
			acc := ensure(thread)
			acc.item.ConstraintCount++
			acc.item.Kind = firstNonEmpty(acc.item.Kind, ev.Name, string(ev.Type))
			acc.item.Policy = firstNonEmpty(acc.item.Policy, ev.ConstraintPolicy)
			acc.item.CPUSet = firstNonEmpty(acc.item.CPUSet, ev.CPUSet)
			acc.item.CGroup = firstNonEmpty(acc.item.CGroup, ev.CGroup)
			addAllowed(acc, ev.AllowedCPUs)
			if ev.ConstraintDestCPUSet {
				acc.item.ObservedCPU = ev.ConstraintDestCPU
				acc.item.ObservedCPUKnown = true
				acc.item.ObservedCoreClass = coreByCPU[ev.ConstraintDestCPU]
			} else if ev.ConstraintCPUValid {
				acc.item.ObservedCPU = ev.ConstraintCPU
				acc.item.ObservedCPUKnown = true
				acc.item.ObservedCoreClass = coreByCPU[ev.ConstraintCPU]
			}
			if ev.ConstraintOrigCPUSet && ev.ConstraintDestCPUSet {
				acc.item.MigrationCount++
			} else if ev.ConstraintDestCPUSet {
				acc.item.MigrationCount++
			}
			updateLines(acc, ev)
		case ev.Type == EventSchedSwitch && (len(ev.NextInfoAllowedCPUs) > 0 || ev.NextInfoRestricted || ev.NextInfoAffinity != ""):
			thread := catalogThreadRef(catalog, ev.NextPID, ev.NextComm)
			if thread.PID <= 0 && thread.Comm == "" {
				continue
			}
			acc := ensure(thread)
			acc.item.ConstraintCount++
			acc.item.Kind = firstNonEmpty(acc.item.Kind, "sched_switch_next_info")
			acc.item.CPUSet = firstNonEmpty(acc.item.CPUSet, ev.CGroup)
			acc.item.CGroup = firstNonEmpty(acc.item.CGroup, ev.CGroup)
			acc.item.ObservedCPU = ev.CPU
			acc.item.ObservedCPUKnown = true
			acc.item.ObservedCoreClass = coreByCPU[ev.CPU]
			addAllowed(acc, ev.NextInfoAllowedCPUs)
			policy := renderNextInfoPolicy(ev)
			if policy != "" {
				acc.item.Policy = policy
			}
			updateLines(acc, ev)
		}
	}
	out := make([]CPUConstraintSummary, 0, len(accs))
	for _, acc := range accs {
		item := acc.item
		sort.Ints(item.AllowedCPUs)
		item.AllowedCoreClasses = coreClassesForCPUs(item.AllowedCPUs, coreByCPU)
		if item.Thread.PID > 0 {
			item.RunnableWaitMs = runnableByPID[item.Thread.PID]
		}
		if item.ObservedCPUKnown {
			for cpuID, cpu := range cpuByID {
				if cpuID != item.ObservedCPU {
					item.OtherCPUIdleMs += cpu.IdleMs
				}
			}
		}
		item.Summary = renderCPUConstraintSummary(item)
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		scoreI := out[i].RunnableWaitMs + float64(out[i].ConstraintCount)*0.25 + float64(out[i].MigrationCount)*0.5
		scoreJ := out[j].RunnableWaitMs + float64(out[j].ConstraintCount)*0.25 + float64(out[j].MigrationCount)*0.5
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func renderNextInfoPolicy(ev Event) string {
	if strings.TrimSpace(ev.NextInfoAffinity) == "" && ev.NextInfoLoad == 0 && ev.NextInfoGroup == 0 && ev.NextInfoExpel == 0 && !ev.NextInfoRestricted {
		return ""
	}
	parts := []string{"next_info"}
	if ev.NextInfoAffinity != "" {
		parts = append(parts, "affinity="+ev.NextInfoAffinity)
	}
	if ev.NextInfoLoad > 0 {
		parts = append(parts, fmt.Sprintf("load=%d", ev.NextInfoLoad))
	}
	parts = append(parts, fmt.Sprintf("group=%d", ev.NextInfoGroup))
	parts = append(parts, fmt.Sprintf("restricted=%t", ev.NextInfoRestricted))
	if ev.NextInfoExpel > 0 {
		parts = append(parts, fmt.Sprintf("expel=%d", ev.NextInfoExpel))
	}
	if ev.NextInfoCGID > 0 {
		parts = append(parts, fmt.Sprintf("cgid=%d", ev.NextInfoCGID))
	}
	return strings.Join(parts, " ")
}

func coreClassesForCPUs(cpus []int, coreByCPU map[int]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, cpu := range cpus {
		class := coreByCPU[cpu]
		if class == "" || seen[class] {
			continue
		}
		seen[class] = true
		out = append(out, class)
	}
	sort.SliceStable(out, func(i, j int) bool { return coreClassRank(out[i]) < coreClassRank(out[j]) })
	return out
}

func renderCPUConstraintSummary(item CPUConstraintSummary) string {
	parts := []string{fmt.Sprintf("%s cpu constraint", threadLabel(item.Thread))}
	if item.Kind != "" {
		parts = append(parts, "kind="+item.Kind)
	}
	if len(item.AllowedCPUs) > 0 {
		parts = append(parts, fmt.Sprintf("allowed_cpus=%s", intListString(item.AllowedCPUs)))
	}
	if len(item.AllowedCoreClasses) > 0 {
		parts = append(parts, "allowed_core_classes="+strings.Join(item.AllowedCoreClasses, ","))
	}
	if item.CPUSet != "" {
		parts = append(parts, "cpuset="+item.CPUSet)
	}
	if item.ObservedCoreClass != "" {
		parts = append(parts, fmt.Sprintf("observed_cpu=%d/%s", item.ObservedCPU, item.ObservedCoreClass))
	} else if item.ObservedCPUKnown {
		parts = append(parts, fmt.Sprintf("observed_cpu=%d", item.ObservedCPU))
	}
	if item.RunnableWaitMs > 0 {
		parts = append(parts, fmt.Sprintf("runnable_wait=%.3fms", item.RunnableWaitMs))
	}
	if item.OtherCPUIdleMs > 0 {
		parts = append(parts, fmt.Sprintf("other_cpu_idle=%.3fms", item.OtherCPUIdleMs))
	}
	if item.Policy != "" {
		parts = append(parts, "policy="+item.Policy)
	}
	return strings.Join(parts, " ")
}

type threadLoadAcc struct {
	item       ThreadCPULoadSummary
	dominantMs float64
	lineStart  int
	lineEnd    int
}

func computeThreadCPULoad(q Query, running []ThreadDuration, runnable []ThreadDuration, max int) []ThreadCPULoadSummary {
	accs := map[string]*threadLoadAcc{}
	ensure := func(thread ThreadRef) *threadLoadAcc {
		key := threadKey(thread)
		acc := accs[key]
		if acc == nil {
			acc = &threadLoadAcc{item: ThreadCPULoadSummary{Thread: thread}}
			accs[key] = acc
		}
		if acc.item.Thread.Comm == "" && thread.Comm != "" {
			acc.item.Thread.Comm = thread.Comm
		}
		if acc.item.Thread.TGID == 0 && thread.TGID > 0 {
			acc.item.Thread.TGID = thread.TGID
		}
		return acc
	}
	add := func(td ThreadDuration, state string) {
		if td.DurationMs <= 0 || (td.Thread.PID <= 0 && td.Thread.Comm == "") {
			return
		}
		acc := ensure(td.Thread)
		if state == "running" {
			acc.item.RunningMs += td.DurationMs
			if isHighPriorityForPressure(q.TraceFlavor, td.Priority, td.PriorityClass) {
				acc.item.HighPriorityRunningMs += td.DurationMs
			}
		} else {
			acc.item.RunnableWaitMs += td.DurationMs
		}
		if td.DurationMs > acc.dominantMs {
			acc.dominantMs = td.DurationMs
			acc.item.CPU = td.CPU
			acc.item.CoreClass = td.CoreClass
			acc.item.Frequency = td.Frequency
			acc.item.Priority = td.Priority
			acc.item.PriorityClass = td.PriorityClass
		}
		if acc.lineStart == 0 || (td.LineStart > 0 && td.LineStart < acc.lineStart) {
			acc.lineStart = td.LineStart
		}
		if td.LineEnd > acc.lineEnd {
			acc.lineEnd = td.LineEnd
		}
	}
	for _, td := range running {
		add(td, "running")
	}
	for _, td := range runnable {
		add(td, "runnable")
	}
	out := make([]ThreadCPULoadSummary, 0, len(accs))
	for _, acc := range accs {
		item := acc.item
		item.LineStart = acc.lineStart
		item.LineEnd = acc.lineEnd
		item.Summary = fmt.Sprintf("%s thread load running=%.3fms runnable=%.3fms high_prio_running=%.3fms cpu=%d",
			threadLabel(item.Thread), item.RunningMs, item.RunnableWaitMs, item.HighPriorityRunningMs, item.CPU)
		if item.CoreClass != "" {
			item.Summary = fmt.Sprintf("%s core_class=%s", item.Summary, item.CoreClass)
		}
		if item.Priority > 0 {
			item.Summary = fmt.Sprintf("%s prio=%d/%s", item.Summary, item.Priority, item.PriorityClass)
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		scoreI := out[i].RunningMs + out[i].RunnableWaitMs
		scoreJ := out[j].RunningMs + out[j].RunnableWaitMs
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

type processLoadAcc struct {
	item      ProcessCPULoadSummary
	threadSet map[int]bool
	cpuSet    map[int]bool
	coreSet   map[string]bool
}

func computeProcessCPULoad(idx *Index, q Query, loads []ThreadCPULoadSummary, coreByCPU map[int]string, max int) []ProcessCPULoadSummary {
	catalog := buildThreadCatalog(idx, q)
	accs := map[string]*processLoadAcc{}
	ensure := func(thread ThreadRef) *processLoadAcc {
		proc := processRefForThread(thread, catalog)
		key := processKey(proc)
		acc := accs[key]
		if acc == nil {
			acc = &processLoadAcc{item: ProcessCPULoadSummary{Process: proc}, threadSet: map[int]bool{}, cpuSet: map[int]bool{}, coreSet: map[string]bool{}}
			accs[key] = acc
		}
		if proc.Comm != "" && acc.item.Process.Comm == "" {
			acc.item.Process.Comm = proc.Comm
		}
		return acc
	}
	for _, load := range loads {
		if load.RunningMs+load.RunnableWaitMs <= 0 || (load.Thread.PID <= 0 && load.Thread.Comm == "") {
			continue
		}
		acc := ensure(load.Thread)
		if load.Thread.PID > 0 && !acc.threadSet[load.Thread.PID] {
			acc.threadSet[load.Thread.PID] = true
			acc.item.ThreadCount++
		}
		acc.item.RunningMs += load.RunningMs
		acc.item.RunnableWaitMs += load.RunnableWaitMs
		acc.item.HighPriorityRunningMs += load.HighPriorityRunningMs
		loadTotal := load.RunningMs + load.RunnableWaitMs
		if loadTotal > acc.item.TopThreadMs {
			acc.item.TopThread = load.Thread
			acc.item.TopThreadMs = loadTotal
		}
		if load.CPU >= 0 && !acc.cpuSet[load.CPU] {
			acc.cpuSet[load.CPU] = true
			acc.item.CPUs = append(acc.item.CPUs, load.CPU)
		}
		class := firstNonEmpty(load.CoreClass, coreByCPU[load.CPU])
		if class != "" && !acc.coreSet[class] {
			acc.coreSet[class] = true
			acc.item.CoreClasses = append(acc.item.CoreClasses, class)
		}
		if acc.item.LineStart == 0 || (load.LineStart > 0 && load.LineStart < acc.item.LineStart) {
			acc.item.LineStart = load.LineStart
		}
		if load.LineEnd > acc.item.LineEnd {
			acc.item.LineEnd = load.LineEnd
		}
	}
	out := make([]ProcessCPULoadSummary, 0, len(accs))
	for _, acc := range accs {
		sort.Ints(acc.item.CPUs)
		sort.SliceStable(acc.item.CoreClasses, func(i, j int) bool {
			return coreClassRank(acc.item.CoreClasses[i]) < coreClassRank(acc.item.CoreClasses[j])
		})
		acc.item.Summary = fmt.Sprintf("%s process load running=%.3fms runnable=%.3fms threads=%d top_thread=%s %.3fms cpus=%s core_classes=%s",
			threadLabel(acc.item.Process), acc.item.RunningMs, acc.item.RunnableWaitMs, acc.item.ThreadCount,
			threadLabel(acc.item.TopThread), acc.item.TopThreadMs, intListString(acc.item.CPUs), strings.Join(acc.item.CoreClasses, ","))
		out = append(out, acc.item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		scoreI := out[i].RunningMs + out[i].RunnableWaitMs
		scoreJ := out[j].RunningMs + out[j].RunnableWaitMs
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func processRefForThread(thread ThreadRef, catalog map[int]ThreadRef) ThreadRef {
	ref := thread
	if thread.PID > 0 {
		if cat := catalog[thread.PID]; cat.PID > 0 {
			ref = cat
			if ref.Comm == "" {
				ref.Comm = thread.Comm
			}
		}
	}
	if ref.TGID > 0 {
		proc := catalog[ref.TGID]
		if proc.PID == 0 {
			proc.PID = ref.TGID
		}
		if proc.Comm == "" && ref.TGID == ref.PID {
			proc.Comm = ref.Comm
		}
		return proc
	}
	return ThreadRef{Comm: ref.Comm, PID: ref.PID}
}

func processKey(process ThreadRef) string {
	if process.PID > 0 {
		return fmt.Sprintf("pid:%d", process.PID)
	}
	return "comm:" + strings.ToLower(strings.TrimSpace(process.Comm))
}

func computeRunnableContextSummaries(items []SchedulerLatencyItem, threadLoads []ThreadCPULoadSummary, processes []ProcessCPULoadSummary, constraints []CPUConstraintSummary, max int) []RunnableContextSummary {
	if len(items) == 0 {
		return nil
	}
	var out []RunnableContextSummary
	for _, item := range items {
		ctx := RunnableContextSummary{
			Thread:                item.Thread,
			RunnableWaitMs:        item.DurationMs,
			CPU:                   item.CPU,
			CoreClass:             item.CoreClass,
			Frequency:             item.Frequency,
			Priority:              item.Priority,
			PriorityClass:         item.PriorityClass,
			SameCPUBusyMs:         item.SameCPUBusyMs,
			SameCPUIdleMs:         item.SameCPUIdleMs,
			OtherCPUIdleMs:        item.OtherCPUIdleMs,
			HighPriorityRunningMs: item.HighPriorityRunningMs,
			SameCPUTopRunning:     item.SameCPUTopRunning,
			LineStart:             item.StartLine,
			LineEnd:               item.EndLine,
		}
		if proc, ok := processLoadForThread(item.Thread, processes); ok {
			copy := proc
			ctx.SameProcessLoad = &copy
		}
		ctx.TopBackgroundThreads = topBackgroundThreads(item.Thread, threadLoads, 4)
		if proc, ok := topBackgroundProcess(item.Thread, processes); ok {
			copy := proc
			ctx.TopBackgroundProcess = &copy
		}
		if constraint, ok := constraintForThread(item.Thread, constraints); ok {
			copy := constraint
			ctx.CPUConstraint = &copy
		}
		ctx.Verdict, ctx.Confidence = runnableContextVerdict(ctx)
		ctx.Summary = renderRunnableContextSummary(ctx)
		out = append(out, ctx)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		if out[i].RunnableWaitMs != out[j].RunnableWaitMs {
			return out[i].RunnableWaitMs > out[j].RunnableWaitMs
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func processLoadForThread(thread ThreadRef, processes []ProcessCPULoadSummary) (ProcessCPULoadSummary, bool) {
	for _, proc := range processes {
		if proc.Process.PID > 0 && (thread.TGID == proc.Process.PID || thread.PID == proc.Process.PID) {
			return proc, true
		}
		if sameThreadRef(proc.TopThread, thread) {
			return proc, true
		}
	}
	return ProcessCPULoadSummary{}, false
}

func topBackgroundThreads(thread ThreadRef, loads []ThreadCPULoadSummary, max int) []ThreadCPULoadSummary {
	if max <= 0 {
		max = 4
	}
	var out []ThreadCPULoadSummary
	for _, load := range loads {
		if sameThreadRef(load.Thread, thread) {
			continue
		}
		out = append(out, load)
		if len(out) >= max {
			break
		}
	}
	return out
}

func topBackgroundProcess(thread ThreadRef, processes []ProcessCPULoadSummary) (ProcessCPULoadSummary, bool) {
	for _, proc := range processes {
		if proc.Process.PID > 0 && (thread.TGID == proc.Process.PID || thread.PID == proc.Process.PID) {
			continue
		}
		if sameThreadRef(proc.TopThread, thread) {
			continue
		}
		return proc, true
	}
	return ProcessCPULoadSummary{}, false
}

func constraintForThread(thread ThreadRef, constraints []CPUConstraintSummary) (CPUConstraintSummary, bool) {
	for _, item := range constraints {
		if sameThreadRef(item.Thread, thread) {
			return item, true
		}
		if item.Thread.PID > 0 && (thread.TGID == item.Thread.PID || thread.PID == item.Thread.PID) {
			return item, true
		}
	}
	return CPUConstraintSummary{}, false
}

func runnableContextVerdict(ctx RunnableContextSummary) (string, float64) {
	if ctx.CPUConstraint != nil && len(ctx.CPUConstraint.AllowedCPUs) > 0 {
		if !stringSliceContains(ctx.CPUConstraint.AllowedCoreClasses, "big") && ctx.OtherCPUIdleMs > 0 {
			return "restricted_to_busy_or_small_cores", 0.84
		}
		if ctx.CPUConstraint.RunnableWaitMs > 0 || ctx.CPUConstraint.ConstraintCount > 0 {
			return "cpu_affinity_or_cpuset_context", 0.78
		}
	}
	if ctx.HighPriorityRunningMs > 0 || ctx.SameCPUBusyMs > ctx.SameCPUIdleMs {
		return "cpu_pressure", 0.76
	}
	if ctx.OtherCPUIdleMs > 0 {
		return "other_cpu_idle_check_affinity_or_wakeup", 0.66
	}
	return "insufficient_signal", 0.50
}

func renderRunnableContextSummary(ctx RunnableContextSummary) string {
	parts := []string{fmt.Sprintf("%s runnable_context wait=%.3fms cpu=%d", threadLabel(ctx.Thread), ctx.RunnableWaitMs, ctx.CPU)}
	if ctx.CoreClass != "" {
		parts = append(parts, "core_class="+ctx.CoreClass)
	}
	if ctx.Frequency > 0 {
		parts = append(parts, fmt.Sprintf("freq=%dkHz", ctx.Frequency))
	}
	parts = append(parts, fmt.Sprintf("same_cpu_busy=%.3fms same_cpu_idle=%.3fms other_cpu_idle=%.3fms high_prio_running=%.3fms",
		ctx.SameCPUBusyMs, ctx.SameCPUIdleMs, ctx.OtherCPUIdleMs, ctx.HighPriorityRunningMs))
	if len(ctx.SameCPUTopRunning) > 0 {
		parts = append(parts, "same_cpu_top_running="+threadLabel(ctx.SameCPUTopRunning[0].Thread))
	}
	if len(ctx.TopBackgroundThreads) > 0 {
		top := ctx.TopBackgroundThreads[0]
		parts = append(parts, fmt.Sprintf("top_background_thread=%s load=%.3fms", threadLabel(top.Thread), top.RunningMs+top.RunnableWaitMs))
	}
	if ctx.TopBackgroundProcess != nil {
		parts = append(parts, fmt.Sprintf("top_background_process=%s load=%.3fms", threadLabel(ctx.TopBackgroundProcess.Process), ctx.TopBackgroundProcess.RunningMs+ctx.TopBackgroundProcess.RunnableWaitMs))
	}
	if ctx.CPUConstraint != nil {
		if len(ctx.CPUConstraint.AllowedCPUs) > 0 {
			parts = append(parts, "allowed_cpus="+intListString(ctx.CPUConstraint.AllowedCPUs))
		}
		if len(ctx.CPUConstraint.AllowedCoreClasses) > 0 {
			parts = append(parts, "allowed_core_classes="+strings.Join(ctx.CPUConstraint.AllowedCoreClasses, ","))
		}
		if ctx.CPUConstraint.CPUSet != "" {
			parts = append(parts, "cpuset="+ctx.CPUConstraint.CPUSet)
		}
		if ctx.CPUConstraint.Policy != "" {
			parts = append(parts, "constraint_policy="+ctx.CPUConstraint.Policy)
		}
	}
	parts = append(parts, "verdict="+ctx.Verdict)
	return strings.Join(parts, " ")
}

func intListString(in []int) string {
	if len(in) == 0 {
		return ""
	}
	parts := make([]string, 0, len(in))
	for _, v := range in {
		parts = append(parts, fmt.Sprintf("%d", v))
	}
	return strings.Join(parts, ",")
}

func stringSliceContains(in []string, value string) bool {
	for _, item := range in {
		if item == value {
			return true
		}
	}
	return false
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
		if cpu.CoreClass != "" {
			summary = fmt.Sprintf("%s core_class=%s", summary, cpu.CoreClass)
		}
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
			CoreClass:             cpu.CoreClass,
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

func computeSupplyPressureSummary(idx *Index, q Query, stats WindowStats, maxBackground int) *SupplyPressureSummary {
	var summary SupplyPressureSummary
	for _, pressure := range stats.CPUPressure {
		summary.RunnableWaitMs += pressure.RunnableWaitMs
		summary.HighPriorityRunningMs += pressure.HighPriorityRunningMs
		if pressure.RunnableWaitMs > 0 || pressure.HighPriorityRunningMs > 0 {
			summary.CPUPressureMs += pressure.RunnableWaitMs + pressure.HighPriorityRunningMs
		}
	}
	for _, accounting := range stats.SchedStatAccounting {
		switch accounting.Kind {
		case "wait", "sleep":
			summary.SchedStatWaitMs += accounting.TotalDelayMs
		case "iowait":
			summary.SchedStatIOWaitMs += accounting.TotalDelayMs
		case "blocked":
			summary.SchedStatBlockedMs += accounting.TotalDelayMs
		}
		applyLineRange(&summary.LineStart, &summary.LineEnd, accounting.LineStart)
		applyLineRange(&summary.LineStart, &summary.LineEnd, accounting.LineEnd)
	}
	for _, ipi := range stats.IPIActivity {
		summary.IPIEventCount += ipi.Count
		summary.IPIActiveMs += ipi.ActiveMs
		applyLineRange(&summary.LineStart, &summary.LineEnd, ipi.LineStart)
		applyLineRange(&summary.LineStart, &summary.LineEnd, ipi.LineEnd)
	}
	for _, cpu := range stats.CPU {
		if cpu.Frequency > 0 && frequencyIsLowForCPU(cpu.Frequency, cpu) {
			summary.LowFrequencyCPUs = append(summary.LowFrequencyCPUs, cpu.CPU)
		}
	}
	if idx != nil {
		for _, ev := range idx.Events {
			if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
				continue
			}
			text := strings.ToLower(ev.Name + " " + ev.ClockName + " " + ev.SubsystemKind + " " + ev.FieldText)
			if ev.Type == EventClockSetRate {
				summary.ClockSetRateCount++
				switch {
				case strings.Contains(text, "ddr") || strings.Contains(text, "mem"):
					summary.DDREventCount++
				case strings.Contains(text, "l3") || strings.Contains(text, "llc") || strings.Contains(text, "cache"):
					summary.L3EventCount++
				case strings.Contains(text, "throughput") || strings.Contains(text, "bw") || strings.Contains(text, "bandwidth"):
					summary.ThroughputEventCount++
				}
			}
			if ev.Type == EventPower && strings.Contains(text, "thermal") {
				summary.ThermalEventCount++
			}
			if summary.LineStart == 0 || (ev.Line > 0 && ev.Line < summary.LineStart) {
				summary.LineStart = ev.Line
			}
			if ev.Line > summary.LineEnd {
				summary.LineEnd = ev.Line
			}
		}
	}
	summary.TopBackgroundThreads = topBackgroundThreads(ThreadRef{}, stats.ThreadCPULoad, maxBackground)
	if maxBackground <= 0 {
		maxBackground = 8
	}
	for _, proc := range stats.ProcessCPULoad {
		summary.TopBackgroundProcesses = append(summary.TopBackgroundProcesses, proc)
		if len(summary.TopBackgroundProcesses) >= maxBackground {
			break
		}
	}
	if summary.CPUPressureMs == 0 && summary.SchedStatWaitMs == 0 && summary.SchedStatIOWaitMs == 0 && summary.SchedStatBlockedMs == 0 && summary.IPIEventCount == 0 && len(summary.LowFrequencyCPUs) == 0 && summary.ClockSetRateCount == 0 && summary.ThermalEventCount == 0 && summary.DDREventCount == 0 && summary.L3EventCount == 0 && summary.ThroughputEventCount == 0 {
		return nil
	}
	switch {
	case summary.CPUPressureMs > 0 && summary.IPIEventCount > 0:
		summary.Signal = "cpu_pressure_with_interrupt_activity"
	case summary.CPUPressureMs > 0 && len(summary.LowFrequencyCPUs) > 0:
		summary.Signal = "cpu_pressure_with_low_frequency"
	case summary.CPUPressureMs > 0:
		summary.Signal = "cpu_pressure"
	case summary.SchedStatIOWaitMs > 0 || summary.SchedStatBlockedMs > 0:
		summary.Signal = "scheduler_accounting_wait_signal"
	case summary.IPIEventCount > 0:
		summary.Signal = "interrupt_activity_signal"
	case summary.ThermalEventCount > 0 || len(summary.LowFrequencyCPUs) > 0:
		summary.Signal = "capacity_limit_signal"
	case summary.DDREventCount > 0 || summary.L3EventCount > 0 || summary.ThroughputEventCount > 0:
		summary.Signal = "memory_or_cache_supply_signal"
	default:
		summary.Signal = "supply_activity"
	}
	summary.Summary = fmt.Sprintf("supply_pressure signal=%s cpu_pressure=%.3fms runnable=%.3fms high_prio=%.3fms sched_stat_wait=%.3fms sched_stat_iowait=%.3fms sched_stat_blocked=%.3fms ipi_events=%d ipi_active=%.3fms low_freq_cpus=%v clock_set_rate=%d thermal=%d ddr=%d l3=%d throughput=%d",
		summary.Signal, summary.CPUPressureMs, summary.RunnableWaitMs, summary.HighPriorityRunningMs, summary.SchedStatWaitMs, summary.SchedStatIOWaitMs, summary.SchedStatBlockedMs, summary.IPIEventCount, summary.IPIActiveMs, summary.LowFrequencyCPUs, summary.ClockSetRateCount, summary.ThermalEventCount, summary.DDREventCount, summary.L3EventCount, summary.ThroughputEventCount)
	return &summary
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

type stateChurnOpen struct {
	thread ThreadRef
	state  ThreadState
	ts     float64
	line   int
}

type stateChurnAcc struct {
	thread        ThreadRef
	runningMs     float64
	runnableMs    float64
	sleepMs       float64
	dStateMs      float64
	ioWaitMs      float64
	fragmentCount int
	stateSwitches int
	maxSegmentMs  float64
	segments      []float64
	lineStart     int
	lineEnd       int
	lastState     ThreadState
}

func computeStateChurnSummaries(idx *Index, q Query, max int) []ThreadStateChurnSummary {
	if idx == nil {
		return nil
	}
	minDurationMs := q.MinDurationMs
	if minDurationMs <= 0 {
		minDurationMs = 1
	}
	blockedReasons := blockedReasonsByPID(idx, q)
	open := map[int]stateChurnOpen{}
	accs := map[string]*stateChurnAcc{}
	closeState := func(pid int, endTs float64, endLine int) {
		start, ok := open[pid]
		if !ok {
			return
		}
		delete(open, pid)
		addStateChurnInterval(accs, start, endTs, endLine, q, blockedReasons)
	}
	openState := func(thread ThreadRef, state ThreadState, ts float64, line int) {
		if thread.PID <= 0 || state == StateUnknown {
			return
		}
		open[thread.PID] = stateChurnOpen{thread: thread, state: state, ts: ts, line: line}
	}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) {
			continue
		}
		switch ev.Type {
		case EventSchedWakeup, EventSchedWaking:
			if ev.WakeePID <= 0 {
				continue
			}
			start, ok := open[ev.WakeePID]
			if !ok || start.state == StateRunning || start.state == StateRunnable || ev.Ts < start.ts {
				continue
			}
			closeState(ev.WakeePID, ev.Ts, ev.Line)
			openState(ThreadRef{Comm: ev.WakeeComm, PID: ev.WakeePID}, StateRunnable, ev.Ts, ev.Line)
		case EventSchedSwitch:
			if ev.NextPID > 0 {
				closeState(ev.NextPID, ev.Ts, ev.Line)
				openState(ThreadRef{Comm: ev.NextComm, PID: ev.NextPID}, StateRunning, ev.Ts, ev.Line)
			}
			if ev.PrevPID > 0 {
				closeState(ev.PrevPID, ev.Ts, ev.Line)
				openState(ThreadRef{Comm: ev.PrevComm, PID: ev.PrevPID}, stateFromPrevState(ev.PrevState), ev.Ts, ev.Line)
			}
		}
	}
	endTs := q.TimeEnd
	if endTs == 0 && idx.LastTs > 0 {
		endTs = idx.LastTs
	}
	for pid := range open {
		closeState(pid, endTs, 0)
	}
	out := make([]ThreadStateChurnSummary, 0, len(accs))
	for _, acc := range accs {
		if item, ok := buildStateChurnSummary(acc, minDurationMs); ok {
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		scoreI := out[i].DominantImpactMs * out[i].Confidence
		scoreJ := out[j].DominantImpactMs * out[j].Confidence
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		if out[i].StateSwitches != out[j].StateSwitches {
			return out[i].StateSwitches > out[j].StateSwitches
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func blockedReasonsByPID(idx *Index, q Query) map[int][]Event {
	out := map[int][]Event{}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || ev.Type != EventSchedBlockedReason || ev.WakeePID <= 0 {
			continue
		}
		out[ev.WakeePID] = append(out[ev.WakeePID], ev)
	}
	return out
}

func addStateChurnInterval(accs map[string]*stateChurnAcc, start stateChurnOpen, endTs float64, endLine int, q Query, blockedReasons map[int][]Event) {
	if endTs <= start.ts {
		return
	}
	state := start.state
	if state == StateDSleep {
		if reason := blockedReasonForInterval(blockedReasons, start.thread, start.ts, endTs); reason != nil && reason.IOWait > 0 {
			state = StateIOWait
			endLine = firstPositive(endLine, reason.Line)
		}
	}
	clampedStart := start.ts
	clampedEnd := endTs
	if q.TimeStart > 0 && clampedStart < q.TimeStart {
		clampedStart = q.TimeStart
	}
	if q.TimeEnd > 0 && clampedEnd > q.TimeEnd {
		clampedEnd = q.TimeEnd
	}
	if clampedEnd <= clampedStart {
		return
	}
	durationMs := (clampedEnd - clampedStart) * 1000
	key := fmt.Sprintf("%d/%s", start.thread.PID, start.thread.Comm)
	acc := accs[key]
	if acc == nil {
		acc = &stateChurnAcc{thread: start.thread}
		accs[key] = acc
	}
	if acc.thread.Comm == "" && start.thread.Comm != "" {
		acc.thread.Comm = start.thread.Comm
	}
	acc.fragmentCount++
	if acc.lastState != "" && acc.lastState != state {
		acc.stateSwitches++
	}
	acc.lastState = state
	if durationMs > acc.maxSegmentMs {
		acc.maxSegmentMs = durationMs
	}
	acc.segments = append(acc.segments, durationMs)
	if acc.lineStart == 0 || (start.line > 0 && start.line < acc.lineStart) {
		acc.lineStart = start.line
	}
	if candidateEnd := firstPositive(endLine, start.line); candidateEnd > acc.lineEnd {
		acc.lineEnd = candidateEnd
	}
	switch state {
	case StateRunning:
		acc.runningMs += durationMs
	case StateRunnable:
		acc.runnableMs += durationMs
	case StateSSleep:
		acc.sleepMs += durationMs
	case StateDSleep:
		acc.dStateMs += durationMs
	case StateIOWait:
		acc.ioWaitMs += durationMs
	}
}

func blockedReasonForInterval(in map[int][]Event, thread ThreadRef, start, end float64) *Event {
	if thread.PID <= 0 {
		return nil
	}
	var best *Event
	for i := range in[thread.PID] {
		ev := &in[thread.PID][i]
		if ev.Ts < start || ev.Ts > end {
			continue
		}
		best = ev
	}
	return best
}

func buildStateChurnSummary(acc *stateChurnAcc, minDurationMs float64) (ThreadStateChurnSummary, bool) {
	if acc == nil || acc.fragmentCount < 4 || acc.stateSwitches < 3 {
		return ThreadStateChurnSummary{}, false
	}
	total := acc.runningMs + acc.runnableMs + acc.sleepMs + acc.dStateMs + acc.ioWaitMs
	dominantState, dominantMs := dominantChurnState(acc)
	if total < minDurationMs*3 || dominantMs < minDurationMs {
		return ThreadStateChurnSummary{}, false
	}
	if acc.maxSegmentMs >= total*0.70 {
		return ThreadStateChurnSummary{}, false
	}
	p95 := percentileFloat64(acc.segments, 0.95)
	confidence := 0.70
	if acc.stateSwitches >= 8 {
		confidence += 0.05
	}
	if acc.fragmentCount >= 10 {
		confidence += 0.05
	}
	if acc.maxSegmentMs <= total*0.35 {
		confidence += 0.04
	}
	if dominantMs >= 5 {
		confidence += 0.03
	}
	if confidence > 0.88 {
		confidence = 0.88
	}
	item := ThreadStateChurnSummary{
		Thread:           acc.thread,
		DominantState:    dominantState,
		TotalMs:          total,
		DominantImpactMs: dominantMs,
		RunningMs:        acc.runningMs,
		RunnableMs:       acc.runnableMs,
		SleepMs:          acc.sleepMs,
		DStateMs:         acc.dStateMs,
		IOWaitMs:         acc.ioWaitMs,
		FragmentCount:    acc.fragmentCount,
		StateSwitches:    acc.stateSwitches,
		MaxSegmentMs:     acc.maxSegmentMs,
		P95SegmentMs:     p95,
		LineStart:        acc.lineStart,
		LineEnd:          acc.lineEnd,
		Confidence:       confidence,
		NextStep:         stateChurnNextStep(dominantState),
	}
	item.Summary = renderStateChurnSummary(item)
	return item, true
}

func enrichStateChurnWithCPUPressure(items []ThreadStateChurnSummary, pressures []CPUPressureStats) []ThreadStateChurnSummary {
	if len(items) == 0 || len(pressures) == 0 {
		return items
	}
	byCPU := map[int]CPUPressureStats{}
	for _, pressure := range pressures {
		byCPU[pressure.CPU] = pressure
	}
	for i := range items {
		if items[i].Thread.PID <= 0 || items[i].DominantState != string(StateRunnable) {
			continue
		}
		cpu, ok := stateChurnRunnableCPU(items[i], pressures)
		if !ok {
			continue
		}
		pressure := byCPU[cpu]
		competitor, competitorMs := stateChurnTopCompetitor(items[i].Thread, pressure)
		items[i].RunnableCPU = cpu
		items[i].RunnableCPUKnown = true
		items[i].RunnableCoreClass = pressure.CoreClass
		if competitor != "" {
			items[i].TopCompetitor = competitor
			items[i].TopCompetitorRunningMs = competitorMs
			items[i].NextStep = fmt.Sprintf("inspect %s on same CPU cpu=%d for CPU pressure/time-slice competition, then validate wake_latency with sched_wakeup", competitor, cpu)
		} else {
			items[i].NextStep = fmt.Sprintf("inspect same-CPU pressure on cpu=%d, top running competitors, priority, CPU frequency, and sched_wakeup wake_latency", cpu)
		}
		items[i].Summary = renderStateChurnSummary(items[i])
	}
	return items
}

func stateChurnRunnableCPU(item ThreadStateChurnSummary, pressures []CPUPressureStats) (int, bool) {
	bestCPU := 0
	bestMs := 0.0
	found := false
	for _, pressure := range pressures {
		for _, runnable := range pressure.TopRunnable {
			if !sameThreadRef(runnable.Thread, item.Thread) {
				continue
			}
			if !found || runnable.DurationMs > bestMs {
				bestCPU = pressure.CPU
				bestMs = runnable.DurationMs
				found = true
			}
		}
	}
	return bestCPU, found
}

func stateChurnTopCompetitor(target ThreadRef, pressure CPUPressureStats) (string, float64) {
	for _, running := range pressure.TopRunning {
		if sameThreadRef(running.Thread, target) {
			continue
		}
		label := threadLabel(running.Thread)
		if label == "" {
			continue
		}
		return label, running.DurationMs
	}
	return "", 0
}

func sameThreadRef(a, b ThreadRef) bool {
	if a.PID > 0 && b.PID > 0 {
		return a.PID == b.PID
	}
	return strings.TrimSpace(a.Comm) != "" && strings.TrimSpace(a.Comm) == strings.TrimSpace(b.Comm)
}

func renderStateChurnSummary(item ThreadStateChurnSummary) string {
	nextStep := strings.TrimSpace(item.NextStep)
	if nextStep == "" {
		nextStep = stateChurnNextStep(item.DominantState)
	}
	summary := fmt.Sprintf("%s had frequent state switching; dominant_state=%s impact=%.3fms total=%.3fms fragments=%d switches=%d max_segment=%.3fms p95_segment=%.3fms totals running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms; next_step=%s",
		threadLabel(item.Thread), item.DominantState, item.DominantImpactMs, item.TotalMs, item.FragmentCount, item.StateSwitches, item.MaxSegmentMs, item.P95SegmentMs, item.RunningMs, item.RunnableMs, item.SleepMs, item.DStateMs, item.IOWaitMs, nextStep)
	if item.RunnableCPUKnown {
		summary = fmt.Sprintf("%s; same_cpu=cpu=%d", summary, item.RunnableCPU)
		if item.RunnableCoreClass != "" {
			summary = fmt.Sprintf("%s/%s", summary, item.RunnableCoreClass)
		}
	}
	if item.TopCompetitor != "" {
		summary = fmt.Sprintf("%s; top_competitor=%s running=%.3fms", summary, item.TopCompetitor, item.TopCompetitorRunningMs)
	}
	return summary
}

func dominantChurnState(acc *stateChurnAcc) (string, float64) {
	type candidate struct {
		state string
		ms    float64
	}
	candidates := []candidate{
		{state: string(StateIOWait), ms: acc.ioWaitMs},
		{state: string(StateDSleep), ms: acc.dStateMs},
		{state: string(StateRunnable), ms: acc.runnableMs},
		{state: string(StateSSleep), ms: acc.sleepMs},
		{state: string(StateRunning), ms: acc.runningMs},
	}
	best := candidates[0]
	for _, item := range candidates[1:] {
		if item.ms > best.ms {
			best = item
		}
	}
	return best.state, best.ms
}

func stateChurnNextStep(state string) string {
	switch state {
	case string(StateRunnable):
		return "inspect same-CPU pressure, top running competitors, priority, and CPU frequency"
	case string(StateSSleep):
		return "inspect repeated wakeup peers, binder waits, locks, and condition waits"
	case string(StateDSleep), string(StateIOWait):
		return "inspect sched_blocked_reason, block IO, filesystem, page-fault, and reclaim evidence"
	case string(StateRunning):
		return "inspect trace spans/frame phases for own CPU work and preemption boundaries"
	default:
		return "inspect neighboring scheduler and resource events"
	}
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

func resolveCoreTopology(cpus []CPUStats, raw string) (map[int]string, string) {
	if explicit := parseCoreTopology(raw); len(explicit) > 0 {
		return explicit, "explicit"
	}
	inferred := inferCoreTopologyFromFrequency(cpus)
	if len(inferred) > 0 {
		return inferred, "inferred_frequency_tiers"
	}
	return map[int]string{}, "unknown"
}

func parseCoreTopology(raw string) map[int]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := map[int]string{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			k, v, ok = strings.Cut(part, ":")
		}
		if !ok {
			continue
		}
		class := normalizeCoreClass(k)
		if class == "" {
			continue
		}
		for _, cpu := range parseCPURangeList(v) {
			out[cpu] = class
		}
	}
	return out
}

func normalizeCoreClass(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "little", "small", "l":
		return "small"
	case "middle", "mid", "medium", "m":
		return "middle"
	case "big", "large", "prime", "b":
		return "big"
	default:
		return ""
	}
}

func parseCPURangeList(raw string) []int {
	var out []int
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == '|' || r == '/' || r == ' ' || r == ',' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if start, end, ok := strings.Cut(part, "-"); ok {
			a, b := atoi(start), atoi(end)
			if a > b {
				a, b = b, a
			}
			for cpu := a; cpu <= b; cpu++ {
				out = append(out, cpu)
			}
			continue
		}
		out = append(out, atoi(part))
	}
	return out
}

func inferCoreTopologyFromFrequency(cpus []CPUStats) map[int]string {
	type cpuFreq struct {
		cpu int
		max int
	}
	var items []cpuFreq
	for _, cpu := range cpus {
		maxFreq := cpu.Frequency
		for _, res := range cpu.FrequencyResidency {
			if res.Frequency > maxFreq {
				maxFreq = res.Frequency
			}
		}
		if maxFreq > 0 {
			items = append(items, cpuFreq{cpu: cpu.CPU, max: maxFreq})
		}
	}
	if len(items) < 2 {
		return nil
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].max != items[j].max {
			return items[i].max < items[j].max
		}
		return items[i].cpu < items[j].cpu
	})
	out := map[int]string{}
	for i, item := range items {
		class := "middle"
		switch {
		case len(items) == 2 && i == 0:
			class = "small"
		case len(items) == 2:
			class = "big"
		case i < len(items)/3:
			class = "small"
		case i >= (len(items)*2)/3:
			class = "big"
		}
		out[item.cpu] = class
	}
	return out
}

func applyCPUCoreClasses(cpus []CPUStats, byCPU map[int]string) {
	for i := range cpus {
		cpus[i].CoreClass = byCPU[cpus[i].CPU]
	}
}

func applyThreadCoreClasses(items []ThreadDuration, byCPU map[int]string) {
	for i := range items {
		items[i].CoreClass = byCPU[items[i].CPU]
	}
}

func applyCPUPressureCoreClasses(items []CPUPressureStats, byCPU map[int]string) {
	for i := range items {
		items[i].CoreClass = byCPU[items[i].CPU]
		applyThreadCoreClasses(items[i].TopRunnable, byCPU)
		applyThreadCoreClasses(items[i].TopRunning, byCPU)
	}
}

func buildCoreClassStats(cpus []CPUStats, pressure []CPUPressureStats, byCPU map[int]string, source string) []CoreClassStats {
	if len(byCPU) == 0 {
		return nil
	}
	type acc struct {
		item CoreClassStats
		seen map[int]bool
	}
	byClass := map[string]*acc{}
	ensure := func(class string) *acc {
		if class == "" {
			class = "unknown"
		}
		if byClass[class] == nil {
			byClass[class] = &acc{item: CoreClassStats{Class: class, TopologySource: source}, seen: map[int]bool{}}
		}
		return byClass[class]
	}
	for _, cpu := range cpus {
		class := byCPU[cpu.CPU]
		a := ensure(class)
		if !a.seen[cpu.CPU] {
			a.item.CPUs = append(a.item.CPUs, cpu.CPU)
			a.seen[cpu.CPU] = true
		}
		a.item.BusyMs += cpu.BusyMs
		a.item.IdleMs += cpu.IdleMs
		if cpu.Frequency > a.item.MaxFrequency {
			a.item.MaxFrequency = cpu.Frequency
		}
		for _, res := range cpu.FrequencyResidency {
			if res.Frequency > a.item.MaxFrequency {
				a.item.MaxFrequency = res.Frequency
			}
		}
	}
	for _, p := range pressure {
		a := ensure(byCPU[p.CPU])
		a.item.RunnableWaitMs += p.RunnableWaitMs
		a.item.HighPriorityRunMs += p.HighPriorityRunningMs
	}
	out := make([]CoreClassStats, 0, len(byClass))
	for _, a := range byClass {
		sort.Ints(a.item.CPUs)
		total := a.item.BusyMs + a.item.IdleMs
		if total > 0 && a.item.BusyMs/total >= 0.80 {
			a.item.ComputeSupplySignal = "class_cpu_pressure"
		} else if a.item.MaxFrequency > 0 {
			a.item.ComputeSupplySignal = "class_frequency_observed"
		} else {
			a.item.ComputeSupplySignal = "class_topology_only"
		}
		out = append(out, a.item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return coreClassRank(out[i].Class) < coreClassRank(out[j].Class)
	})
	return out
}

func coreClassRank(class string) int {
	switch class {
	case "small":
		return 0
	case "middle":
		return 1
	case "big":
		return 2
	default:
		return 3
	}
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
	asyncStarts := map[string][]Event{}
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
			spans = append(spans, traceSpanFromEvents(start, ev, "sync"))
		case "S":
			key := traceAsyncSpanKey(ev)
			if key == "" {
				continue
			}
			asyncStarts[key] = append(asyncStarts[key], ev)
		case "F":
			key := traceAsyncSpanKey(ev)
			stack := asyncStarts[key]
			if key == "" || len(stack) == 0 {
				continue
			}
			start := stack[len(stack)-1]
			asyncStarts[key] = stack[:len(stack)-1]
			if ev.Ts < start.Ts {
				continue
			}
			spans = append(spans, traceSpanFromEvents(start, ev, "async"))
		case "C":
			key := fmt.Sprintf("%d/%d/%s/%s", ev.PID, ev.SpanPID, ev.SpanName, ev.SpanValue)
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

func traceSpanFromEvents(start, end Event, kind string) TraceSpanSummary {
	return TraceSpanSummary{
		Thread:        threadRefFromEvent(start),
		Kind:          kind,
		Name:          start.SpanName,
		Category:      traceSpanCategory(start.SpanName),
		Subcategory:   traceSpanSubcategory(start.SpanName),
		SemanticClass: traceSpanSemanticClass(start.SpanName),
		StartTs:       start.Ts,
		EndTs:         end.Ts,
		DurationMs:    (end.Ts - start.Ts) * 1000,
		StartLine:     start.Line,
		EndLine:       end.Line,
	}
}

func traceAsyncSpanKey(ev Event) string {
	if ev.SpanName == "" || ev.SpanValue == "" {
		return ""
	}
	spanPID := ev.SpanPID
	if spanPID == 0 {
		spanPID = ev.TGID
	}
	if spanPID == 0 {
		spanPID = ev.PID
	}
	return fmt.Sprintf("%d/%s/%s", spanPID, ev.SpanName, ev.SpanValue)
}

func resolveSpanWindowsForQuery(idx *Index, q *Query, explicitStart, explicitEnd bool) ([]TraceSpanSummary, []string) {
	if idx == nil || q == nil || strings.TrimSpace(q.SpanName) == "" {
		return nil, nil
	}
	spans, caveats := FindSpanWindows(idx, *q, q.Limit)
	if len(spans) == 0 {
		return nil, append(caveats, fmt.Sprintf("span_name=%q matched no complete trace span in the selected filters; synchronous B/E spans close with unnamed E|<pid> or bare E on the same ftrace thread stack, and async S/F spans close by name+cookie, so do not search for E|<pid>|<span_name> as proof of an end marker", q.SpanName))
	}
	if explicitStart && explicitEnd {
		return spans, caveats
	}
	if len(spans) != 1 {
		return spans, append(caveats, fmt.Sprintf("span_name=%q matched %d span window(s); refine with pid/thread/line_start/line_end/time filters before deriving a root-cause window; for a specific frame id or marker, first run trace_query(view=\"event_search\", pattern=\"<frame id or exact label>\", event_types=[\"trace_mark\"]) and then rerun with the selected line/time window; do not narrow by searching E|<pid>|<span_name> because B/E end rows are unnamed", q.SpanName, len(spans)))
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
	asyncStarts := map[string][]Event{}
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
			span := traceSpanFromEvents(start, ev, "sync")
			if !traceSpanMatchesQuery(span, target, q) {
				continue
			}
			spans = append(spans, span)
		case "S":
			key := traceAsyncSpanKey(ev)
			if key == "" {
				continue
			}
			asyncStarts[key] = append(asyncStarts[key], ev)
		case "F":
			key := traceAsyncSpanKey(ev)
			stack := asyncStarts[key]
			if key == "" || len(stack) == 0 {
				continue
			}
			start := stack[len(stack)-1]
			asyncStarts[key] = stack[:len(stack)-1]
			if ev.Ts < start.Ts {
				continue
			}
			span := traceSpanFromEvents(start, ev, "async")
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
		caveats = append(caveats, "no complete trace spans matched the selected filters; B/E ends are unnamed E|<pid> or bare E on the same ftrace thread stack, and async S/F spans pair by name+cookie")
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

type interruptOpen struct {
	ev Event
}

func computeInterruptActivity(idx *Index, q Query, typ EventType, coreByCPU map[int]string, max int) []InterruptActivity {
	if idx == nil {
		return nil
	}
	accs := map[string]*InterruptActivity{}
	open := map[string][]interruptOpen{}
	ensure := func(ev Event, baseName string) *InterruptActivity {
		name := firstNonEmpty(baseName, ev.IRQName, ev.Name, string(typ))
		key := fmt.Sprintf("%s/%d/%d/%s", typ, ev.CPU, ev.IRQID, name)
		item := accs[key]
		if item == nil {
			item = &InterruptActivity{
				Kind:      string(typ),
				CPU:       ev.CPU,
				CoreClass: coreByCPU[ev.CPU],
				Vector:    ev.IRQID,
				Name:      name,
				LineStart: ev.Line,
				LineEnd:   ev.Line,
				StartTs:   ev.Ts,
				EndTs:     ev.Ts,
			}
			accs[key] = item
		}
		item.Count++
		if ev.IPITargetMask != "" {
			item.TargetMask = ev.IPITargetMask
		}
		item.TargetCPUs = appendUniqueInts(item.TargetCPUs, ev.IPITargetCPUs...)
		applyLineRange(&item.LineStart, &item.LineEnd, ev.Line)
		if item.StartTs == 0 || ev.Ts < item.StartTs {
			item.StartTs = ev.Ts
		}
		if ev.Ts > item.EndTs {
			item.EndTs = ev.Ts
		}
		return item
	}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) || ev.Type != typ {
			continue
		}
		baseName, phase := interruptBaseAndPhase(ev)
		item := ensure(ev, baseName)
		key := fmt.Sprintf("%s/%d/%d/%s", typ, ev.CPU, ev.IRQID, item.Name)
		switch phase {
		case "entry":
			open[key] = append(open[key], interruptOpen{ev: ev})
		case "exit":
			queue := open[key]
			if len(queue) == 0 {
				continue
			}
			start := queue[0].ev
			open[key] = queue[1:]
			if ev.Ts < start.Ts {
				continue
			}
			dur := (ev.Ts - start.Ts) * 1000
			item.PairedCount++
			item.ActiveMs += dur
			if dur > item.MaxActiveMs {
				item.MaxActiveMs = dur
			}
		}
	}
	windowMs := (q.TimeEnd - q.TimeStart) * 1000
	out := make([]InterruptActivity, 0, len(accs))
	for _, item := range accs {
		if item.ActiveMs == 0 && item.EndTs > item.StartTs && typ != EventIPI {
			item.ActiveMs = (item.EndTs - item.StartTs) * 1000
		}
		if windowMs > 0 {
			item.WindowOverlapMs = minFloat(item.ActiveMs, windowMs)
		}
		sort.Ints(item.TargetCPUs)
		item.Summary = fmt.Sprintf("%s cpu=%d core_class=%s vector=%d name=%s count=%d paired=%d active=%.3fms max=%.3fms",
			item.Kind, item.CPU, item.CoreClass, item.Vector, item.Name, item.Count, item.PairedCount, item.ActiveMs, item.MaxActiveMs)
		if item.TargetMask != "" {
			item.Summary = fmt.Sprintf("%s target_mask=%s target_cpus=%v", item.Summary, item.TargetMask, item.TargetCPUs)
		}
		out = append(out, *item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ActiveMs != out[j].ActiveMs {
			return out[i].ActiveMs > out[j].ActiveMs
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func interruptBaseAndPhase(ev Event) (baseName, phase string) {
	name := strings.ToLower(strings.TrimSpace(ev.Name))
	baseName = firstNonEmpty(ev.IRQName, ev.Name)
	if ev.Type == EventIPI && strings.HasSuffix(name, "_raise") {
		return strings.TrimSuffix(baseName, "_raise"), "instant"
	}
	for _, suffix := range []string{"_entry", "_raise", "_start", "_enter"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(baseName, suffix), "entry"
		}
	}
	for _, suffix := range []string{"_exit", "_done", "_end", "_complete"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(baseName, suffix), "exit"
		}
	}
	return baseName, ""
}

func computeSchedStatAccounting(idx *Index, q Query, max int) []SchedStatSummary {
	if idx == nil {
		return nil
	}
	accs := map[string]*SchedStatSummary{}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) || ev.Type != EventSchedStat {
			continue
		}
		thread := ThreadRef{Comm: firstNonEmpty(ev.SchedStatComm, ev.Comm), PID: firstNonZero(ev.SchedStatPID, ev.PID), TGID: ev.TGID}
		kind := firstNonEmpty(ev.SchedStatKind, strings.TrimPrefix(ev.Name, "sched_stat_"), "unknown")
		key := fmt.Sprintf("%d/%s/%s", thread.PID, thread.Comm, kind)
		item := accs[key]
		if item == nil {
			item = &SchedStatSummary{
				Thread:    thread,
				Kind:      kind,
				LineStart: ev.Line,
				LineEnd:   ev.Line,
				StartTs:   ev.Ts,
				EndTs:     ev.Ts,
			}
			accs[key] = item
		}
		item.Count++
		delayMs := float64(ev.SchedStatDelayNs) / 1e6
		runtimeMs := float64(ev.SchedStatRunNs) / 1e6
		vruntimeMs := float64(ev.SchedStatVRunNs) / 1e6
		item.TotalDelayMs += delayMs
		if delayMs > item.MaxDelayMs {
			item.MaxDelayMs = delayMs
		}
		item.TotalRuntimeMs += runtimeMs
		if runtimeMs > item.MaxRuntimeMs {
			item.MaxRuntimeMs = runtimeMs
		}
		item.TotalVRuntimeMs += vruntimeMs
		applyLineRange(&item.LineStart, &item.LineEnd, ev.Line)
		if item.StartTs == 0 || ev.Ts < item.StartTs {
			item.StartTs = ev.Ts
		}
		if ev.Ts > item.EndTs {
			item.EndTs = ev.Ts
		}
	}
	out := make([]SchedStatSummary, 0, len(accs))
	for _, item := range accs {
		item.Summary = fmt.Sprintf("sched_stat kind=%s thread=%s count=%d delay=%.3fms max_delay=%.3fms runtime=%.3fms max_runtime=%.3fms",
			item.Kind, threadLabel(item.Thread), item.Count, item.TotalDelayMs, item.MaxDelayMs, item.TotalRuntimeMs, item.MaxRuntimeMs)
		out = append(out, *item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		iImpact := schedStatImpactMs(out[i])
		jImpact := schedStatImpactMs(out[j])
		if iImpact != jImpact {
			return iImpact > jImpact
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func schedStatImpactMs(item SchedStatSummary) float64 {
	if item.TotalDelayMs > 0 {
		return item.TotalDelayMs
	}
	return item.TotalRuntimeMs
}

type workqueueOpen struct {
	ev Event
}

func computeWorkqueueActivity(idx *Index, q Query, max int) []WorkqueueActivity {
	if idx == nil {
		return nil
	}
	accs := map[string]*WorkqueueActivity{}
	open := map[string][]workqueueOpen{}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) || ev.Type != EventWorkqueue {
			continue
		}
		work, function := workqueueFields(ev)
		base, phase := workqueueBaseAndPhase(ev.Name)
		key := fmt.Sprintf("%d/%s/%s/%s", ev.PID, firstNonEmpty(work, "-"), firstNonEmpty(function, "-"), base)
		item := accs[key]
		if item == nil {
			item = &WorkqueueActivity{
				Thread:    threadRefFromEvent(ev),
				Work:      work,
				Function:  function,
				LineStart: ev.Line,
				LineEnd:   ev.Line,
				StartTs:   ev.Ts,
				EndTs:     ev.Ts,
			}
			accs[key] = item
		}
		item.Count++
		applyLineRange(&item.LineStart, &item.LineEnd, ev.Line)
		if item.StartTs == 0 || ev.Ts < item.StartTs {
			item.StartTs = ev.Ts
		}
		if ev.Ts > item.EndTs {
			item.EndTs = ev.Ts
		}
		switch phase {
		case "start":
			open[key] = append(open[key], workqueueOpen{ev: ev})
		case "done":
			queue := open[key]
			if len(queue) == 0 {
				continue
			}
			start := queue[0].ev
			open[key] = queue[1:]
			if ev.Ts < start.Ts {
				continue
			}
			dur := (ev.Ts - start.Ts) * 1000
			item.PairedCount++
			item.DurationMs += dur
			if dur > item.MaxLatencyMs {
				item.MaxLatencyMs = dur
			}
		}
	}
	out := make([]WorkqueueActivity, 0, len(accs))
	for _, item := range accs {
		if item.DurationMs == 0 && item.EndTs > item.StartTs {
			item.DurationMs = (item.EndTs - item.StartTs) * 1000
		}
		item.Summary = fmt.Sprintf("workqueue thread=%s work=%s function=%s count=%d paired=%d duration=%.3fms max=%.3fms",
			threadLabel(item.Thread), item.Work, item.Function, item.Count, item.PairedCount, item.DurationMs, item.MaxLatencyMs)
		out = append(out, *item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DurationMs != out[j].DurationMs {
			return out[i].DurationMs > out[j].DurationMs
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func workqueueFields(ev Event) (work, function string) {
	kv := parseKV(ev.FieldText)
	work = firstNonEmpty(kv["work"], kv["addr"], kv["address"])
	function = firstNonEmpty(kv["function"], kv["func"], kv["name"])
	if work == "" {
		work = valueAfterLabel(ev.FieldText, "work struct")
	}
	return cleanTraceValue(work), cleanTraceValue(function)
}

func workqueueBaseAndPhase(name string) (base, phase string) {
	lower := strings.ToLower(strings.TrimSpace(name))
	base = lower
	for _, suffix := range []string{"_start", "_enter", "_begin"} {
		if strings.HasSuffix(lower, suffix) {
			return strings.TrimSuffix(lower, suffix), "start"
		}
	}
	for _, suffix := range []string{"_end", "_exit", "_done", "_complete"} {
		if strings.HasSuffix(lower, suffix) {
			return strings.TrimSuffix(lower, suffix), "done"
		}
	}
	return base, ""
}

func valueAfterLabel(text, label string) string {
	lower := strings.ToLower(text)
	idx := strings.Index(lower, strings.ToLower(label))
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(text[idx+len(label):])
	rest = strings.TrimLeft(rest, "=: \t")
	if rest == "" {
		return ""
	}
	return strings.Fields(rest)[0]
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
	case EventSchedStat:
		return "sched_stat"
	case EventCPUFrequencyLimit:
		return "cpu_frequency_limits"
	case EventIPI:
		return "ipi"
	case EventSoftIRQ:
		return "softirq"
	case EventStorage:
		return "storage"
	case EventFilesystem:
		return "filesystem"
	case EventPower:
		return "power"
	case EventAbilityMonitor:
		return "ability_monitor"
	case EventXPower:
		return "xpower"
	case EventHiSystemEvent:
		return "hi_sysevent"
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

func accumulateFileIO(out map[string]*FileIOSummary, ev Event) {
	if !isFileIOEvent(ev) {
		return
	}
	inode := firstNonEmpty(ev.Inode, "unknown")
	dev := firstNonEmpty(ev.FSDev, ev.BlockSrcDev, ev.BlockDev, "unknown")
	op := firstNonEmpty(ev.FileRW, ev.ResourceOp, fileOperationFromEventName(ev.Name), "io")
	key := strings.Join([]string{dev, inode, op, fmt.Sprintf("%d", ev.PID)}, "/")
	item := out[key]
	if item == nil {
		item = &FileIOSummary{
			Dev:         dev,
			Inode:       inode,
			ParentInode: ev.ParentInode,
			EntryName:   ev.EntryName,
			Operation:   op,
			Thread:      threadRefFromEvent(ev),
			LineStart:   ev.Line,
			LineEnd:     ev.Line,
			StartTs:     ev.Ts,
			EndTs:       ev.Ts,
			Example:     clampString(ev.FieldText, 160),
		}
		out[key] = item
	}
	if fileIOCountsAsActivity(ev) {
		item.Count++
		if ev.FileLen > 0 {
			item.Bytes += ev.FileLen
		} else if ev.ResourceBytes > 0 {
			item.Bytes += ev.ResourceBytes
		}
	} else {
		item.CompletionCount++
	}
	item.TotalLatencyMs += ev.ResourceLatencyMs
	if ev.ResourceLatencyMs > item.MaxLatencyMs {
		item.MaxLatencyMs = ev.ResourceLatencyMs
	}
	if item.EntryName == "" && ev.EntryName != "" {
		item.EntryName = ev.EntryName
	}
	if item.ParentInode == "" && ev.ParentInode != "" {
		item.ParentInode = ev.ParentInode
	}
	if ev.FileRet != 0 {
		item.Ret = ev.FileRet
	}
	applyOffsetRange(&item.MinOffset, &item.MaxOffset, ev.FileOffset)
	applyLineRange(&item.LineStart, &item.LineEnd, ev.Line)
	if item.StartTs == 0 || ev.Ts < item.StartTs {
		item.StartTs = ev.Ts
	}
	if ev.Ts > item.EndTs {
		item.EndTs = ev.Ts
	}
	if item.Example == "" {
		item.Example = clampString(ev.FieldText, 160)
	}
}

func sortedFileIOSummaries(in map[string]*FileIOSummary, max int) []FileIOSummary {
	out := make([]FileIOSummary, 0, len(in))
	for _, item := range in {
		item.Summary = fmt.Sprintf("inode=%s dev=%s op=%s count=%d bytes=%d thread=%s", item.Inode, item.Dev, item.Operation, item.Count, item.Bytes, threadLabel(item.Thread))
		if item.CompletionCount > 0 {
			item.Summary = fmt.Sprintf("%s completions=%d", item.Summary, item.CompletionCount)
		}
		if item.EntryName != "" {
			item.Summary = fmt.Sprintf("%s name=%s", item.Summary, item.EntryName)
		}
		out = append(out, *item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TotalLatencyMs != out[j].TotalLatencyMs {
			return out[i].TotalLatencyMs > out[j].TotalLatencyMs
		}
		if out[i].Bytes != out[j].Bytes {
			return out[i].Bytes > out[j].Bytes
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func accumulatePageCache(out map[string]*PageCacheSummary, ev Event) {
	if !isPageCacheEvent(ev) {
		return
	}
	inode := firstNonEmpty(ev.Inode, "unknown")
	dev := firstNonEmpty(ev.FSDev, ev.BlockSrcDev, ev.BlockDev, "unknown")
	key := strings.Join([]string{dev, inode, fmt.Sprintf("%d", ev.PID)}, "/")
	item := out[key]
	if item == nil {
		item = &PageCacheSummary{
			Dev:       dev,
			Inode:     inode,
			Thread:    threadRefFromEvent(ev),
			LineStart: ev.Line,
			LineEnd:   ev.Line,
			StartTs:   ev.Ts,
			EndTs:     ev.Ts,
			Example:   clampString(ev.FieldText, 160),
		}
		out[key] = item
	}
	name := strings.ToLower(ev.Name)
	switch {
	case strings.Contains(name, "delete_from_page_cache"):
		item.Deletes++
	case strings.Contains(name, "add_to_page_cache"):
		item.Adds++
	default:
		item.Adds++
	}
	item.Churn = item.Adds + item.Deletes
	if ev.FileLen > 0 {
		item.Bytes += ev.FileLen
	} else if ev.ResourceBytes > 0 {
		item.Bytes += ev.ResourceBytes
	}
	applyOffsetRange(&item.MinOffset, &item.MaxOffset, ev.FileOffset)
	applyLineRange(&item.LineStart, &item.LineEnd, ev.Line)
	if item.StartTs == 0 || ev.Ts < item.StartTs {
		item.StartTs = ev.Ts
	}
	if ev.Ts > item.EndTs {
		item.EndTs = ev.Ts
	}
	if item.Example == "" {
		item.Example = clampString(ev.FieldText, 160)
	}
}

func sortedPageCacheSummaries(in map[string]*PageCacheSummary, max int) []PageCacheSummary {
	out := make([]PageCacheSummary, 0, len(in))
	for _, item := range in {
		item.Summary = fmt.Sprintf("inode=%s dev=%s page-cache adds=%d deletes=%d churn=%d thread=%s", item.Inode, item.Dev, item.Adds, item.Deletes, item.Churn, threadLabel(item.Thread))
		out = append(out, *item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Churn != out[j].Churn {
			return out[i].Churn > out[j].Churn
		}
		if out[i].Bytes != out[j].Bytes {
			return out[i].Bytes > out[j].Bytes
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

type storageLatencyAcc struct {
	item           StorageLatencySummary
	totalLatencyMs float64
	open           []Event
}

func computeStorageLatencyByLayer(idx *Index, q Query, blockLatencies []IOLatencySummary, max int) []StorageLatencySummary {
	accs := map[string]*storageLatencyAcc{}
	for _, io := range blockLatencies {
		key := strings.Join([]string{"block", "block_rq", io.Dev, io.Op}, "/")
		acc := storageLatencyAccumulator(accs, key, "block", "block_rq", io.Dev, io.Op, io.IssueThread, io.IssueLine, io.IssueTs, "")
		acc.item.Count++
		acc.item.PairedCount++
		acc.totalLatencyMs += io.DurationMs
		if io.DurationMs > acc.item.MaxLatencyMs {
			acc.item.MaxLatencyMs = io.DurationMs
		}
		acc.item.Bytes += io.Len * 512
		applyLineRange(&acc.item.LineStart, &acc.item.LineEnd, io.IssueLine)
		applyLineRange(&acc.item.LineStart, &acc.item.LineEnd, io.CompleteLine)
		if io.CompleteTs > acc.item.EndTs {
			acc.item.EndTs = io.CompleteTs
		}
	}
	if idx != nil {
		for _, ev := range idx.Events {
			if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) || !isStorageLatencyEvent(ev) {
				continue
			}
			layer := storageLatencyLayer(ev)
			base, phase := storageLatencyBaseAndPhase(ev)
			if layer == "" || base == "" {
				continue
			}
			dev := firstNonEmpty(ev.FSDev, ev.BlockDev, ev.BlockSrcDev, "unknown")
			op := firstNonEmpty(ev.FileRW, ev.ResourceOp, ev.BlockOp, fileOperationFromEventName(ev.Name), base)
			key := strings.Join([]string{layer, base, dev, firstNonEmpty(ev.Inode, "-"), op, fmt.Sprintf("%d", ev.PID)}, "/")
			acc := storageLatencyAccumulator(accs, key, layer, base, dev, op, threadRefFromEvent(ev), ev.Line, ev.Ts, ev.FieldText)
			if acc.item.Inode == "" && ev.Inode != "" {
				acc.item.Inode = ev.Inode
			}
			if acc.item.EntryName == "" && ev.EntryName != "" {
				acc.item.EntryName = ev.EntryName
			}
			acc.item.Count++
			if ev.FileLen > 0 {
				acc.item.Bytes += ev.FileLen
			} else if ev.ResourceBytes > 0 {
				acc.item.Bytes += ev.ResourceBytes
			}
			applyLineRange(&acc.item.LineStart, &acc.item.LineEnd, ev.Line)
			if ev.Ts > acc.item.EndTs {
				acc.item.EndTs = ev.Ts
			}
			switch phase {
			case "start":
				acc.open = append(acc.open, ev)
			case "done":
				if len(acc.open) == 0 {
					acc.item.UnpairedDoneCount++
					continue
				}
				start := acc.open[0]
				acc.open = acc.open[1:]
				if ev.Ts < start.Ts {
					continue
				}
				dur := (ev.Ts - start.Ts) * 1000
				acc.item.PairedCount++
				acc.totalLatencyMs += dur
				if dur > acc.item.MaxLatencyMs {
					acc.item.MaxLatencyMs = dur
				}
			}
		}
	}
	out := make([]StorageLatencySummary, 0, len(accs))
	for _, acc := range accs {
		acc.item.UnpairedStartCount += len(acc.open)
		if acc.item.PairedCount > 0 {
			acc.item.AvgLatencyMs = acc.totalLatencyMs / float64(acc.item.PairedCount)
		}
		acc.item.Summary = fmt.Sprintf("layer=%s event=%s dev=%s op=%s count=%d paired=%d max_latency=%.3fms", acc.item.Layer, acc.item.Event, acc.item.Dev, acc.item.Operation, acc.item.Count, acc.item.PairedCount, acc.item.MaxLatencyMs)
		out = append(out, acc.item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].MaxLatencyMs != out[j].MaxLatencyMs {
			return out[i].MaxLatencyMs > out[j].MaxLatencyMs
		}
		if out[i].PairedCount != out[j].PairedCount {
			return out[i].PairedCount > out[j].PairedCount
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func storageLatencyAccumulator(accs map[string]*storageLatencyAcc, key, layer, event, dev, op string, thread ThreadRef, line int, ts float64, example string) *storageLatencyAcc {
	acc := accs[key]
	if acc != nil {
		return acc
	}
	acc = &storageLatencyAcc{item: StorageLatencySummary{
		Layer:     layer,
		Event:     event,
		Dev:       dev,
		Operation: op,
		Thread:    thread,
		LineStart: line,
		LineEnd:   line,
		StartTs:   ts,
		EndTs:     ts,
		Example:   clampString(example, 160),
	}}
	accs[key] = acc
	return acc
}

func computeIOPressureSummary(stats WindowStats) *IOPressureSummary {
	var blockMax float64
	for _, io := range stats.IOLatencies {
		if io.DurationMs > blockMax {
			blockMax = io.DurationMs
		}
	}
	var storageMax float64
	for _, item := range stats.StorageLatencyByLayer {
		if item.MaxLatencyMs > storageMax {
			storageMax = item.MaxLatencyMs
		}
	}
	var fileBytes int64
	var fileEvents int
	var topFile FileIOSummary
	for _, item := range stats.FileIOByInode {
		fileBytes += item.Bytes
		fileEvents += fileIOEffectiveEventCount(item)
		if topFile.Inode == "" || item.Bytes > topFile.Bytes || (item.Bytes == topFile.Bytes && item.Count > topFile.Count) {
			topFile = item
		}
	}
	var pageChurn int
	var topCache PageCacheSummary
	for _, item := range stats.PageCacheByInode {
		pageChurn += item.Churn
		if topCache.Inode == "" || item.Churn > topCache.Churn {
			topCache = item
		}
	}
	var dStateMs float64
	for _, item := range stats.DStateTop {
		dStateMs += item.DurationMs
	}
	if blockMax == 0 && storageMax == 0 && fileBytes == 0 && fileEvents == 0 && pageChurn == 0 && stats.IOWaitBlockedCount == 0 && dStateMs == 0 {
		return nil
	}
	score := firstPositiveFloat(blockMax, storageMax) +
		float64(stats.IOWaitBlockedCount)*5 +
		dStateMs +
		float64(pageChurn)*0.2 +
		float64(fileEvents)*0.1 +
		float64(fileBytes)/(1024*1024)*2
	signal := "io_activity"
	switch {
	case stats.IOWaitBlockedCount > 0 && (blockMax > 0 || storageMax > 0):
		signal = "scheduler_iowait_with_storage_latency"
	case fileBytes > 0 || fileEvents > 0:
		signal = "file_io_hot_inode"
	case pageChurn > 0:
		signal = "page_cache_churn"
	case blockMax > 0 || storageMax > 0:
		signal = "storage_latency"
	}
	topInode := firstNonEmpty(topFile.Inode, topCache.Inode)
	topDev := firstNonEmpty(topFile.Dev, topCache.Dev)
	topName := topFile.EntryName
	lineStart := firstPositive(topFile.LineStart, topCache.LineStart)
	lineEnd := firstPositive(topFile.LineEnd, topCache.LineEnd)
	if lineStart == 0 {
		for _, item := range stats.StorageLatencyByLayer {
			lineStart = item.LineStart
			lineEnd = item.LineEnd
			break
		}
	}
	return &IOPressureSummary{
		Signal:              signal,
		Score:               score,
		BlockMaxLatencyMs:   blockMax,
		StorageMaxLatencyMs: storageMax,
		FileIOBytes:         fileBytes,
		FileIOEvents:        fileEvents,
		PageCacheChurn:      pageChurn,
		IOWaitBlockedCount:  stats.IOWaitBlockedCount,
		DStateMs:            dStateMs,
		TopInode:            topInode,
		TopDev:              topDev,
		TopEntryName:        topName,
		LineStart:           lineStart,
		LineEnd:             lineEnd,
		Summary:             fmt.Sprintf("io pressure signal=%s score=%.3f block_max=%.3fms storage_max=%.3fms file_bytes=%d file_events=%d page_cache_churn=%d iowait_blocked=%d d_state=%.3fms top_inode=%s", signal, score, blockMax, storageMax, fileBytes, fileEvents, pageChurn, stats.IOWaitBlockedCount, dStateMs, firstNonEmpty(topInode, "unknown")),
	}
}

type blockInodeAcc struct {
	item BlockIOByInodeSummary
}

func computeBlockIOByInode(stats WindowStats, max int) []BlockIOByInodeSummary {
	accs := map[string]*blockInodeAcc{}
	ensure := func(dev, inode, entry string, thread ThreadRef, op string, lineStart, lineEnd int, startTs, endTs float64) *blockInodeAcc {
		if inode == "" && entry == "" {
			return nil
		}
		key := strings.Join([]string{firstNonEmpty(dev, "unknown"), firstNonEmpty(inode, "unknown"), fmt.Sprintf("%d", thread.PID)}, "/")
		acc := accs[key]
		if acc == nil {
			acc = &blockInodeAcc{item: BlockIOByInodeSummary{
				Dev:       firstNonEmpty(dev, "unknown"),
				Inode:     inode,
				EntryName: entry,
				Thread:    thread,
				Operation: op,
				LineStart: lineStart,
				LineEnd:   lineEnd,
			}}
			accs[key] = acc
		}
		if acc.item.EntryName == "" && entry != "" {
			acc.item.EntryName = entry
		}
		if acc.item.Operation == "" && op != "" {
			acc.item.Operation = op
		}
		applyLineRange(&acc.item.LineStart, &acc.item.LineEnd, lineStart)
		applyLineRange(&acc.item.LineStart, &acc.item.LineEnd, lineEnd)
		if startTs > 0 && (acc.item.NearestBlockTs == 0 || startTs < acc.item.NearestBlockTs) {
			acc.item.NearestBlockTs = startTs
		}
		if acc.item.StartTs == 0 || (startTs > 0 && startTs < acc.item.StartTs) {
			acc.item.StartTs = startTs
		}
		if endTs > acc.item.EndTs {
			acc.item.EndTs = endTs
		}
		return acc
	}
	for _, file := range stats.FileIOByInode {
		acc := ensure(file.Dev, file.Inode, file.EntryName, file.Thread, file.Operation, file.LineStart, file.LineEnd, file.StartTs, file.EndTs)
		if acc == nil {
			continue
		}
		acc.item.FileIOBytes += file.Bytes
	}
	for _, cache := range stats.PageCacheByInode {
		acc := ensure(cache.Dev, cache.Inode, "", cache.Thread, "page_cache", cache.LineStart, cache.LineEnd, cache.StartTs, cache.EndTs)
		if acc == nil {
			continue
		}
		acc.item.PageCacheChurn += cache.Churn
	}
	for _, storage := range stats.StorageLatencyByLayer {
		acc := ensure(storage.Dev, storage.Inode, storage.EntryName, storage.Thread, storage.Operation, storage.LineStart, storage.LineEnd, storage.StartTs, storage.EndTs)
		if acc == nil {
			acc = nearestBlockInodeForThread(accs, storage.Thread, storage.StartTs, storage.EndTs)
		}
		if acc == nil {
			continue
		}
		if acc.item.BlockDev == "" {
			acc.item.BlockDev = storage.Dev
		}
		if storage.MaxLatencyMs > acc.item.StorageMaxLatencyMs {
			acc.item.StorageMaxLatencyMs = storage.MaxLatencyMs
		}
	}
	for _, io := range stats.IOLatencies {
		acc := nearestBlockInodeForThread(accs, io.IssueThread, io.IssueTs, io.CompleteTs)
		if acc == nil {
			continue
		}
		if acc.item.BlockDev == "" {
			acc.item.BlockDev = io.Dev
		}
		if io.DurationMs > acc.item.BlockMaxLatencyMs {
			acc.item.BlockMaxLatencyMs = io.DurationMs
			acc.item.NearestBlockThread = firstNonEmptyThread(io.IssueThread, io.CompleteThread)
			acc.item.NearestBlockTs = io.IssueTs
		}
		if acc.item.StartTs == 0 || (io.IssueTs > 0 && io.IssueTs < acc.item.StartTs) {
			acc.item.StartTs = io.IssueTs
		}
		if io.CompleteTs > acc.item.EndTs {
			acc.item.EndTs = io.CompleteTs
		}
		applyLineRange(&acc.item.LineStart, &acc.item.LineEnd, io.IssueLine)
		applyLineRange(&acc.item.LineStart, &acc.item.LineEnd, io.CompleteLine)
	}
	out := make([]BlockIOByInodeSummary, 0, len(accs))
	for _, acc := range accs {
		item := acc.item
		if item.BlockMaxLatencyMs > 0 || item.StorageMaxLatencyMs > 0 {
			item.Confidence = 0.76
		} else if item.FileIOBytes > 0 || item.PageCacheChurn > 0 {
			item.Confidence = 0.64
		} else {
			item.Confidence = 0.50
		}
		item.Summary = fmt.Sprintf("inode=%s dev=%s block_dev=%s op=%s file_bytes=%d page_cache_churn=%d block_max=%.3fms storage_max=%.3fms thread=%s",
			firstNonEmpty(item.Inode, "unknown"), item.Dev, item.BlockDev, item.Operation, item.FileIOBytes, item.PageCacheChurn, item.BlockMaxLatencyMs, item.StorageMaxLatencyMs, threadLabel(item.Thread))
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		scoreI := out[i].BlockMaxLatencyMs + out[i].StorageMaxLatencyMs + float64(out[i].FileIOBytes)/(1024*1024) + float64(out[i].PageCacheChurn)*0.2
		scoreJ := out[j].BlockMaxLatencyMs + out[j].StorageMaxLatencyMs + float64(out[j].FileIOBytes)/(1024*1024) + float64(out[j].PageCacheChurn)*0.2
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func nearestBlockInodeForThread(accs map[string]*blockInodeAcc, thread ThreadRef, start, end float64) *blockInodeAcc {
	var best *blockInodeAcc
	bestDistance := 0.0
	found := false
	for _, acc := range accs {
		if thread.PID > 0 && acc.item.Thread.PID > 0 && thread.PID != acc.item.Thread.PID {
			continue
		}
		if thread.PID == 0 && !sameThreadRef(thread, acc.item.Thread) {
			continue
		}
		distance := 0.0
		if acc.item.NearestBlockTs > 0 {
			distance = windowDistanceMs(start, end, acc.item.NearestBlockTs, acc.item.NearestBlockTs+0.000001)
		}
		if !found || distance < bestDistance {
			best = acc
			bestDistance = distance
			found = true
		}
	}
	return best
}

func computeIOBurstEpisodes(stats WindowStats, max int) []IOBurstEpisodeSummary {
	var out []IOBurstEpisodeSummary
	add := func(item IOBurstEpisodeSummary) {
		if item.DurationMs <= 0 {
			item.DurationMs = item.DStateMs + item.IOWaitMs + firstPositiveFloat(item.BlockMaxLatencyMs, item.StorageMaxLatencyMs)
		}
		if item.DurationMs <= 0 && item.FileIOBytes == 0 && item.PageCacheChurn == 0 {
			return
		}
		if item.Confidence <= 0 {
			item.Confidence = 0.68
		}
		item.Summary = fmt.Sprintf("%s io_burst signal=%s duration=%.3fms d_state=%.3fms io_wait=%.3fms block_max=%.3fms storage_max=%.3fms inode=%s file_bytes=%d page_cache_churn=%d",
			threadLabel(item.Thread), firstNonEmpty(item.DominantSignal, "io_activity"), item.DurationMs, item.DStateMs, item.IOWaitMs, item.BlockMaxLatencyMs, item.StorageMaxLatencyMs, firstNonEmpty(item.TopInode, "unknown"), item.FileIOBytes, item.PageCacheChurn)
		out = append(out, item)
	}
	topIO := stats.IOPressureSummary
	for _, td := range stats.DStateTop {
		item := IOBurstEpisodeSummary{
			Thread:         td.Thread,
			DominantSignal: "d_state_or_io_wait",
			DStateMs:       td.DurationMs,
			DurationMs:     td.DurationMs,
			StartTs:        td.StartTs,
			EndTs:          td.EndTs,
			LineStart:      td.LineStart,
			LineEnd:        td.LineEnd,
			Confidence:     0.74,
		}
		for _, br := range stats.BlockedReasons {
			if sameThreadRef(br.Thread, td.Thread) && br.IOWait > 0 {
				item.IOWaitMs = td.DurationMs
				item.DominantSignal = "scheduler_iowait"
				item.Confidence = 0.82
				break
			}
		}
		if topIO != nil {
			item.BlockMaxLatencyMs = topIO.BlockMaxLatencyMs
			item.StorageMaxLatencyMs = topIO.StorageMaxLatencyMs
			item.FileIOBytes = topIO.FileIOBytes
			item.PageCacheChurn = topIO.PageCacheChurn
			item.TopInode = topIO.TopInode
			item.TopDev = topIO.TopDev
			item.TopEntryName = topIO.TopEntryName
		}
		add(item)
	}
	for _, inode := range stats.BlockIOByInode {
		if inode.BlockMaxLatencyMs == 0 && inode.StorageMaxLatencyMs == 0 {
			continue
		}
		add(IOBurstEpisodeSummary{
			Thread:              inode.Thread,
			DominantSignal:      "inode_storage_latency",
			DurationMs:          firstPositiveFloat(inode.BlockMaxLatencyMs, inode.StorageMaxLatencyMs),
			BlockMaxLatencyMs:   inode.BlockMaxLatencyMs,
			StorageMaxLatencyMs: inode.StorageMaxLatencyMs,
			FileIOBytes:         inode.FileIOBytes,
			PageCacheChurn:      inode.PageCacheChurn,
			TopInode:            inode.Inode,
			TopDev:              inode.Dev,
			TopEntryName:        inode.EntryName,
			StartTs:             inode.StartTs,
			EndTs:               inode.EndTs,
			LineStart:           inode.LineStart,
			LineEnd:             inode.LineEnd,
			Confidence:          inode.Confidence,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		scoreI := out[i].DurationMs*maxFloat(out[i].Confidence, 0.5) + out[i].BlockMaxLatencyMs + out[i].StorageMaxLatencyMs
		scoreJ := out[j].DurationMs*maxFloat(out[j].Confidence, 0.5) + out[j].BlockMaxLatencyMs + out[j].StorageMaxLatencyMs
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func accumulateTracePluginEvent(ability, xpower, hiSystem map[string]*TracePluginSummary, ev Event) {
	kind := tracePluginKind(ev.Type)
	if kind == "" {
		return
	}
	target := ability
	switch kind {
	case "xpower":
		target = xpower
	case "hi_sysevent":
		target = hiSystem
	}
	eventName := firstNonEmpty(ev.PluginEventName, ev.Name)
	metric := firstNonEmpty(ev.PluginMetric, ev.SubsystemKind, ev.Name)
	domain := firstNonEmpty(ev.PluginDomain, ev.Comm)
	key := fmt.Sprintf("%s/%s/%s/%s/%s/%d", kind, domain, eventName, metric, ev.PluginValue, ev.PID)
	item := target[key]
	if item == nil {
		item = &TracePluginSummary{
			Kind:      kind,
			Domain:    domain,
			EventName: eventName,
			Metric:    metric,
			Value:     ev.PluginValue,
			Category:  ev.PluginCategory,
			Thread:    threadRefFromEvent(ev),
			Line:      ev.Line,
			Ts:        ev.Ts,
			Example:   clampString(ev.FieldText, 160),
		}
		target[key] = item
	}
	item.Count++
	if item.Line == 0 || ev.Line < item.Line {
		item.Line = ev.Line
		item.Ts = ev.Ts
	}
	if item.Example == "" {
		item.Example = clampString(ev.FieldText, 160)
	}
}

func tracePluginKind(typ EventType) string {
	switch typ {
	case EventAbilityMonitor:
		return "ability_monitor"
	case EventXPower:
		return "xpower"
	case EventHiSystemEvent:
		return "hi_sysevent"
	default:
		return ""
	}
}

func sortedTracePluginSummaries(in map[string]*TracePluginSummary, max int) []TracePluginSummary {
	out := make([]TracePluginSummary, 0, len(in))
	for _, item := range in {
		out = append(out, *item)
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
	q = ensureQueryFlavor(idx, q)
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
		targetBlockedMs := (q.TimeEnd - q.TimeStart) * 1000
		expandChain(idx, q, target, q.TimeStart, q.TimeEnd, 0, targetBlockedMs, visited, &res, "")
		res.AggregatedImpacts = aggregateWakeupCausalImpacts(res)
		attachIPCGraphToChain(idx, q, &res)
		return res
	}
	for _, branch := range branches {
		visited := map[int]bool{}
		targetBlockedMs := (branch.EndTs - branch.StartTs) * 1000
		expandChain(idx, q, target, branch.StartTs, branch.EndTs, 0, targetBlockedMs, visited, &res, "")
	}
	res.AggregatedImpacts = aggregateWakeupCausalImpacts(res)
	attachIPCGraphToChain(idx, q, &res)
	return res
}

func aggregateWakeupCausalImpacts(chain ChainResult) []WakeupCausalAggregate {
	type acc struct {
		item      WakeupCausalAggregate
		prioVotes map[string]int
		invCount  int
	}
	accs := map[string]*acc{}
	for _, impact := range chain.CausalImpacts {
		if impact.Thread.PID <= 0 || impact.ChainDepth <= 0 || impact.TotalMs <= 0 || strings.TrimSpace(impact.DominantState) == "" {
			continue
		}
		key := fmt.Sprintf("%d/%s", impact.Thread.PID, impact.DominantState)
		a := accs[key]
		if a == nil {
			a = &acc{prioVotes: map[string]int{}}
			a.item.Thread = impact.Thread
			a.item.ChainDepth = impact.ChainDepth
			a.item.DominantState = impact.DominantState
			a.item.Path = wakeupChainPathFromThread(chain, impact.Thread)
			accs[key] = a
		}
		a.item.OccurrenceCount++
		if impact.ChainDepth < a.item.ChainDepth {
			a.item.ChainDepth = impact.ChainDepth
		}
		a.item.TotalMs += impact.TotalMs
		a.item.ProjectedTotalMs += firstPositiveFloat(impact.ProjectedTotalMs, impact.TotalMs)
		a.item.ActualTotalMs += impact.ActualTotalMs
		a.item.RunningMs += impact.RunningMs
		a.item.RunnableMs += impact.RunnableMs
		a.item.SleepMs += impact.SleepMs
		a.item.DStateMs += impact.DStateMs
		a.item.IOWaitMs += impact.IOWaitMs
		a.item.ActualRunningMs += impact.ActualRunningMs
		a.item.ActualRunnableMs += impact.ActualRunnableMs
		a.item.ActualSleepMs += impact.ActualSleepMs
		a.item.ActualDStateMs += impact.ActualDStateMs
		a.item.ActualIOWaitMs += impact.ActualIOWaitMs
		a.item.TargetBlockedMs += impact.TargetBlockedMs
		a.item.FragmentCount += impact.FragmentCount
		a.item.StateSwitches += impact.StateSwitches
		if impact.MaxSegmentMs > a.item.MaxSegmentMs {
			a.item.MaxSegmentMs = impact.MaxSegmentMs
		}
		if impact.Window.StartTs > 0 && (a.item.FirstTs == 0 || impact.Window.StartTs < a.item.FirstTs) {
			a.item.FirstTs = impact.Window.StartTs
		}
		if impact.Window.EndTs > a.item.LastTs {
			a.item.LastTs = impact.Window.EndTs
		}
		if impact.ActualWindow.StartTs > 0 && (a.item.ActualFirstTs == 0 || impact.ActualWindow.StartTs < a.item.ActualFirstTs) {
			a.item.ActualFirstTs = impact.ActualWindow.StartTs
		}
		if impact.ActualWindow.EndTs > a.item.ActualLastTs {
			a.item.ActualLastTs = impact.ActualWindow.EndTs
		}
		if impact.LineStart > 0 && (a.item.LineStart == 0 || impact.LineStart < a.item.LineStart) {
			a.item.LineStart = impact.LineStart
		}
		if impact.LineEnd > a.item.LineEnd {
			a.item.LineEnd = impact.LineEnd
		}
		if impact.PriorityRelation != "" {
			a.prioVotes[impact.PriorityRelation]++
		}
		if impact.PriorityInversionCandidate {
			a.invCount++
		}
		a.item.OccurrenceWindows = append(a.item.OccurrenceWindows, wakeupCausalOccurrenceFromImpact(impact))
	}
	var out []WakeupCausalAggregate
	for _, a := range accs {
		if a.item.OccurrenceCount < 2 {
			continue
		}
		a.item.DominantState, a.item.DominantImpactMs = dominantAggregateState(a.item)
		a.item.ProjectedImpactMs = aggregateBlockingMs(a.item)
		a.item.ActualImpactMs = actualAggregateBlockingMs(a.item)
		a.item.PriorityRelation = mostFrequentString(a.prioVotes)
		a.item.PriorityInversion = a.invCount > 0
		a.item.OccurrenceWindows = trimWakeupCausalOccurrences(a.item.OccurrenceWindows, wakeupCausalAggregateOccurrenceCap)
		a.item.Summary = renderWakeupCausalAggregateSummary(a.item)
		out = append(out, a.item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DominantImpactMs != out[j].DominantImpactMs {
			return out[i].DominantImpactMs > out[j].DominantImpactMs
		}
		if out[i].OccurrenceCount != out[j].OccurrenceCount {
			return out[i].OccurrenceCount > out[j].OccurrenceCount
		}
		return out[i].LineStart < out[j].LineStart
	})
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

func wakeupCausalOccurrenceFromImpact(impact WakeupCausalImpact) WakeupCausalOccurrence {
	return WakeupCausalOccurrence{
		Window:            impact.Window,
		ActualWindow:      impact.ActualWindow,
		DominantState:     impact.DominantState,
		DominantImpactMs:  impact.DominantImpactMs,
		ProjectedImpactMs: impact.ProjectedImpactMs,
		TotalMs:           impact.TotalMs,
		ProjectedTotalMs:  firstPositiveFloat(impact.ProjectedTotalMs, impact.TotalMs),
		ActualImpactMs:    impact.ActualImpactMs,
		ActualTotalMs:     impact.ActualTotalMs,
		TargetBlockedMs:   impact.TargetBlockedMs,
		RunningMs:         impact.RunningMs,
		RunnableMs:        impact.RunnableMs,
		SleepMs:           impact.SleepMs,
		DStateMs:          impact.DStateMs,
		IOWaitMs:          impact.IOWaitMs,
		ActualRunningMs:   impact.ActualRunningMs,
		ActualRunnableMs:  impact.ActualRunnableMs,
		ActualSleepMs:     impact.ActualSleepMs,
		ActualDStateMs:    impact.ActualDStateMs,
		ActualIOWaitMs:    impact.ActualIOWaitMs,
		FragmentCount:     impact.FragmentCount,
		StateSwitches:     impact.StateSwitches,
		MaxSegmentMs:      impact.MaxSegmentMs,
		P95SegmentMs:      impact.P95SegmentMs,
		LineStart:         impact.LineStart,
		LineEnd:           impact.LineEnd,
		Summary:           impact.Summary,
	}
}

func trimWakeupCausalOccurrences(in []WakeupCausalOccurrence, limit int) []WakeupCausalOccurrence {
	if len(in) == 0 {
		return nil
	}
	out := append([]WakeupCausalOccurrence(nil), in...)
	if limit > 0 && len(out) > limit {
		sort.SliceStable(out, func(i, j int) bool {
			si := wakeupCausalOccurrenceSelectionScore(out[i])
			sj := wakeupCausalOccurrenceSelectionScore(out[j])
			if si != sj {
				return si > sj
			}
			if out[i].Window.StartTs != out[j].Window.StartTs {
				return out[i].Window.StartTs < out[j].Window.StartTs
			}
			return out[i].LineStart < out[j].LineStart
		})
		out = out[:limit]
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Window.StartTs != out[j].Window.StartTs {
			return out[i].Window.StartTs < out[j].Window.StartTs
		}
		if out[i].Window.EndTs != out[j].Window.EndTs {
			return out[i].Window.EndTs < out[j].Window.EndTs
		}
		return out[i].LineStart < out[j].LineStart
	})
	return out
}

func wakeupCausalOccurrenceSelectionScore(item WakeupCausalOccurrence) float64 {
	score := item.TotalMs
	if item.TargetBlockedMs > score {
		score = item.TargetBlockedMs
	}
	if item.DominantImpactMs > score {
		score = item.DominantImpactMs
	}
	return score
}

func dominantAggregateState(item WakeupCausalAggregate) (string, float64) {
	type candidate struct {
		state string
		ms    float64
	}
	candidates := []candidate{
		{state: string(StateIOWait), ms: item.IOWaitMs},
		{state: string(StateDSleep), ms: item.DStateMs},
		{state: string(StateRunnable), ms: item.RunnableMs},
		{state: string(StateSSleep), ms: item.SleepMs},
		{state: string(StateRunning), ms: item.RunningMs},
	}
	best := candidates[0]
	for _, cand := range candidates[1:] {
		if cand.ms > best.ms {
			best = cand
		}
	}
	return best.state, best.ms
}

func mostFrequentString(in map[string]int) string {
	best := ""
	bestCount := 0
	for value, count := range in {
		if count > bestCount || (count == bestCount && (best == "" || value < best)) {
			best = value
			bestCount = count
		}
	}
	return best
}

func wakeupChainPathFromThread(chain ChainResult, thread ThreadRef) string {
	if thread.PID <= 0 {
		return ""
	}
	var parts []string
	current := thread
	seen := map[int]bool{}
	for current.PID > 0 {
		if seen[current.PID] {
			break
		}
		seen[current.PID] = true
		parts = append(parts, threadLabel(current))
		if chain.Target.PID > 0 && current.PID == chain.Target.PID {
			break
		}
		next := ThreadRef{}
		for _, edge := range chain.Edges {
			if edge.Waker.PID == current.PID {
				next = edge.Wakee
				break
			}
		}
		if next.PID <= 0 {
			break
		}
		current = next
	}
	return strings.Join(parts, " -> ")
}

func renderWakeupCausalAggregateSummary(item WakeupCausalAggregate) string {
	summary := fmt.Sprintf("%s aggregated on wakeup chain occurrences=%d dominant_state=%s impact=%.3fms total=%.3fms projected_impact=%.3fms projected_total=%.3fms actual_impact=%.3fms actual_total=%.3fms actual_window=%.6f..%.6f target_blocked=%.3fms fragments=%d switches=%d max_segment=%.3fms totals running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms actual_totals running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms",
		threadLabel(item.Thread), item.OccurrenceCount, item.DominantState, item.DominantImpactMs, item.TotalMs, item.ProjectedImpactMs, item.ProjectedTotalMs, item.ActualImpactMs, item.ActualTotalMs, item.ActualFirstTs, item.ActualLastTs, item.TargetBlockedMs, item.FragmentCount, item.StateSwitches, item.MaxSegmentMs, item.RunningMs, item.RunnableMs, item.SleepMs, item.DStateMs, item.IOWaitMs, item.ActualRunningMs, item.ActualRunnableMs, item.ActualSleepMs, item.ActualDStateMs, item.ActualIOWaitMs)
	if item.Path != "" {
		summary += " path=" + item.Path
	}
	if item.PriorityRelation != "" {
		summary += " priority_relation=" + item.PriorityRelation
	}
	if item.PriorityInversion {
		summary += " priority_inversion_candidate=true"
	}
	return summary
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
				Thread:            node.Thread,
				Peer:              edge.Receiver,
				TransactionID:     edge.TransactionID,
				Flags:             edge.Flags,
				Oneway:            edge.Oneway,
				SyncLike:          edge.SyncLike,
				BlockingCandidate: edge.BlockingCandidate,
				SendLine:          edge.SendLine,
				ReceiveLine:       edge.ReceiveLine,
				SleepLine:         node.EvidenceLine,
				SendTs:            edge.SendTs,
				SleepStartTs:      node.Window.StartTs,
				DurationMs:        node.DurationMs,
				Confidence:        confidence,
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
			if edge.Flags != "" {
				wait.Summary = fmt.Sprintf("%s flags=%s oneway=%t sync_like=%t blocking_candidate=%t", wait.Summary, edge.Flags, edge.Oneway, edge.SyncLike, edge.BlockingCandidate)
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
	if q.PID > 0 || q.Thread != "" || q.ThreadInput != "" {
		chain = BuildWakeupChain(idx, q)
	}
	stats := ComputeWindowStats(idx, q)
	rank := buildRootCauseRankFrom(q, chain, stats)
	latency := buildSchedulerLatencyStatsFromStats(idx, q, stats)
	rank = enrichRootCauseRankWithScheduler(q, rank, latency, stats, chain)
	return attachPerfContextToRootCauseRank(idx, q, rank, stats)
}

func buildRootCauseRankFrom(q Query, chain ChainResult, stats WindowStats) RootCauseRankResult {
	res := RootCauseRankResult{
		Target: chain.Target,
		Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
	}
	var items []RootCauseRankItem
	chainThreads := wakeupChainThreadSet(chain)
	hasCausalChain := len(chainThreads) > 1
	for _, impact := range chain.CausalImpacts {
		if impact.ChainDepth <= 0 || impact.TotalMs <= 0 {
			continue
		}
		if isIntermediateSleepImpact(chain, impact) {
			continue
		}
		items = append(items, rootCauseItemFromCausalImpact(impact))
	}
	for _, aggregate := range chain.AggregatedImpacts {
		if aggregate.ChainDepth <= 0 || aggregate.DominantImpactMs <= 0 {
			continue
		}
		if isIntermediateSleepAggregate(chain, aggregate) {
			continue
		}
		items = append(items, rootCauseItemFromCausalAggregate(aggregate))
	}
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
		item := rootCauseItem("cpu_pressure", ThreadRef{}, backgroundImpactMs(q, pressure.RunnableWaitMs, hasCausalChain, false), conf, firstThreadLine(pressure.TopRunnable), lastThreadLine(pressure.TopRunning), "window_stats", summary)
		item.CumulativeImpactMs = pressure.RunnableWaitMs
		if hasCausalChain {
			item.Causality = "background"
		}
		items = append(items, item)
	}
	for _, io := range stats.IOLatencies {
		onChain := threadInSet(chainThreads, io.IssueThread) || threadInSet(chainThreads, io.CompleteThread)
		item := rootCauseItem("io_latency", io.IssueThread, backgroundImpactMs(q, io.DurationMs, hasCausalChain, onChain), 0.86, io.IssueLine, io.CompleteLine, "window_stats", fmt.Sprintf("block IO %s %s sector=%d len=%d took %.3fms", io.Dev, io.Op, io.Sector, io.Len, io.DurationMs))
		item.CumulativeImpactMs = io.DurationMs
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.StartTs = io.IssueTs
		item.EndTs = io.CompleteTs
		items = append(items, item)
	}
	for _, file := range stats.FileIOByInode {
		impact := file.TotalLatencyMs
		if impact <= 0 {
			impact = fileIOAdvisoryImpactMs(file)
		}
		if impact <= 0 {
			continue
		}
		summary := fmt.Sprintf("file IO inode=%s dev=%s op=%s count=%d bytes=%d", file.Inode, file.Dev, file.Operation, file.Count, file.Bytes)
		if file.EntryName != "" {
			summary = fmt.Sprintf("%s name=%s", summary, file.EntryName)
		}
		onChain := threadInSet(chainThreads, file.Thread)
		item := rootCauseItem("file_io_hot_inode", file.Thread, backgroundImpactMs(q, impact, hasCausalChain, onChain), 0.72, file.LineStart, file.LineEnd, "window_stats.file_io_by_inode", summary)
		item.CumulativeImpactMs = impact
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.StartTs = file.StartTs
		item.EndTs = file.EndTs
		items = append(items, item)
	}
	for _, cache := range stats.PageCacheByInode {
		impact := float64(cache.Churn) * 0.3
		if impact <= 0 {
			continue
		}
		onChain := threadInSet(chainThreads, cache.Thread)
		item := rootCauseItem("page_cache_churn", cache.Thread, backgroundImpactMs(q, impact, hasCausalChain, onChain), 0.66, cache.LineStart, cache.LineEnd, "window_stats.page_cache_by_inode", fmt.Sprintf("page cache churn inode=%s dev=%s adds=%d deletes=%d churn=%d", cache.Inode, cache.Dev, cache.Adds, cache.Deletes, cache.Churn))
		item.CumulativeImpactMs = impact
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.StartTs = cache.StartTs
		item.EndTs = cache.EndTs
		items = append(items, item)
	}
	if stats.IOPressureSummary != nil && stats.IOPressureSummary.Score > 0 {
		thread := ThreadRef{}
		if len(stats.FileIOByInode) > 0 {
			thread = stats.FileIOByInode[0].Thread
		}
		onChain := threadInSet(chainThreads, thread)
		item := rootCauseItem("io_pressure", thread, backgroundImpactMs(q, stats.IOPressureSummary.Score, hasCausalChain, onChain), 0.70, stats.IOPressureSummary.LineStart, stats.IOPressureSummary.LineEnd, "window_stats.io_pressure_summary", stats.IOPressureSummary.Summary)
		item.CumulativeImpactMs = stats.IOPressureSummary.Score
		item.Causality = causalityLabel(hasCausalChain, onChain)
		if len(stats.FileIOByInode) > 0 {
			item.StartTs = stats.FileIOByInode[0].StartTs
			item.EndTs = stats.FileIOByInode[0].EndTs
		}
		items = append(items, item)
	}
	for _, episode := range stats.IOBurstEpisodes {
		onChain := threadInSet(chainThreads, episode.Thread)
		impact := firstPositiveFloat(episode.DurationMs, episode.DStateMs+episode.IOWaitMs, episode.BlockMaxLatencyMs, episode.StorageMaxLatencyMs)
		if impact <= 0 {
			continue
		}
		item := rootCauseItem("io_burst_episode", episode.Thread, backgroundImpactMs(q, impact, hasCausalChain, onChain), episode.Confidence, episode.LineStart, episode.LineEnd, "window_stats.io_burst_episodes", episode.Summary)
		item.CumulativeImpactMs = impact
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.StartTs = episode.StartTs
		item.EndTs = episode.EndTs
		item.DominantState = ioBurstDominantState(episode)
		item.DStateMs = episode.DStateMs
		item.IOWaitMs = episode.IOWaitMs
		items = append(items, item)
	}
	for _, inode := range stats.BlockIOByInode {
		onChain := threadInSet(chainThreads, inode.Thread)
		impact := inode.BlockMaxLatencyMs + inode.StorageMaxLatencyMs + float64(inode.FileIOBytes)/(1024*1024)
		if impact <= 0 {
			continue
		}
		item := rootCauseItem("block_io_by_inode", inode.Thread, backgroundImpactMs(q, impact, hasCausalChain, onChain), inode.Confidence, inode.LineStart, inode.LineEnd, "window_stats.block_io_by_inode", inode.Summary)
		item.CumulativeImpactMs = impact
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.StartTs = inode.StartTs
		item.EndTs = inode.EndTs
		items = append(items, item)
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
		if item, ok := rootCauseItemFromSemanticTraceSpan(q, chain, span, hasCausalChain); ok {
			items = append(items, item)
			continue
		}
		item := rootCauseItem("trace_span", span.Thread, span.DurationMs, 0.74, span.StartLine, span.EndLine, "window_stats", fmt.Sprintf("trace span %q lasted %.3fms", span.Name, span.DurationMs))
		item.StartTs = span.StartTs
		item.EndTs = span.EndTs
		item.SpanName = span.Name
		item.SpanKind = span.Kind
		item.SpanCategory = span.Category
		item.SpanSubcategory = span.Subcategory
		item.SemanticClass = span.SemanticClass
		items = append(items, item)
	}
	for _, td := range stats.RunnableTop {
		onChain := threadInSet(chainThreads, td.Thread)
		item := rootCauseItem("runnable_wait", td.Thread, backgroundImpactMs(q, td.DurationMs, hasCausalChain, onChain), 0.76, td.LineStart, td.LineEnd, "window_stats", fmt.Sprintf("%s was runnable for %.3fms%s", threadLabel(td.Thread), td.DurationMs, durationCPUDetail(td)))
		item.CumulativeImpactMs = td.DurationMs
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.StartTs = td.StartTs
		item.EndTs = td.EndTs
		item.DominantState = string(StateRunnable)
		item.RunnableMs = td.DurationMs
		items = append(items, item)
	}
	for _, td := range stats.DStateTop {
		onChain := threadInSet(chainThreads, td.Thread)
		item := rootCauseItem("d_state_or_io_wait", td.Thread, backgroundImpactMs(q, td.DurationMs, hasCausalChain, onChain), 0.82, td.LineStart, td.LineEnd, "window_stats", fmt.Sprintf("%s was in D-state/IO-like wait for %.3fms%s", threadLabel(td.Thread), td.DurationMs, durationCPUDetail(td)))
		item.CumulativeImpactMs = td.DurationMs
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.StartTs = td.StartTs
		item.EndTs = td.EndTs
		item.DominantState = string(StateDSleep)
		item.DStateMs = td.DurationMs
		items = append(items, item)
	}
	for _, churn := range stats.StateChurn {
		onChain := threadInSet(chainThreads, churn.Thread)
		item := rootCauseItem(stateChurnRootCauseType(churn.DominantState), churn.Thread, backgroundImpactMs(q, stateChurnRankImpactMs(churn), hasCausalChain, onChain), churn.Confidence, churn.LineStart, churn.LineEnd, "window_stats.state_churn", churn.Summary)
		item.CumulativeImpactMs = churn.TotalMs
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.DominantState = churn.DominantState
		item.RunningMs = churn.RunningMs
		item.RunnableMs = churn.RunnableMs
		item.SleepMs = churn.SleepMs
		item.DStateMs = churn.DStateMs
		item.IOWaitMs = churn.IOWaitMs
		items = append(items, item)
	}
	for _, burst := range stats.IRQBursts {
		item := rootCauseItem("irq_burst", ThreadRef{}, burst.DurationMs, 0.66, burst.LineStart, burst.LineEnd, "window_stats", fmt.Sprintf("IRQ burst %s irq=%d on cpu=%d had %d event(s) over %.3fms", burst.Name, burst.IRQ, burst.CPU, burst.Count, burst.DurationMs))
		item.StartTs = burst.StartTs
		item.EndTs = burst.EndTs
		items = append(items, item)
	}
	for _, irq := range stats.IRQActivity {
		if irq.ActiveMs <= 0 {
			continue
		}
		item := rootCauseItem("irq_activity", ThreadRef{}, backgroundImpactMs(q, irq.ActiveMs, hasCausalChain, false), 0.60, irq.LineStart, irq.LineEnd, "window_stats.irq_activity", irq.Summary)
		item.CumulativeImpactMs = irq.ActiveMs
		item.StartTs = irq.StartTs
		item.EndTs = irq.EndTs
		items = append(items, item)
	}
	for _, ipi := range stats.IPIActivity {
		impact := ipi.ActiveMs
		if impact <= 0 {
			continue
		}
		item := rootCauseItem("ipi_activity", ThreadRef{}, backgroundImpactMs(q, impact, hasCausalChain, false), 0.56, ipi.LineStart, ipi.LineEnd, "window_stats.ipi_activity", ipi.Summary)
		item.CumulativeImpactMs = impact
		item.StartTs = ipi.StartTs
		item.EndTs = ipi.EndTs
		items = append(items, item)
	}
	for _, accounting := range stats.SchedStatAccounting {
		impact := schedStatImpactMs(accounting)
		if impact <= 0 {
			continue
		}
		onChain := threadInSet(chainThreads, accounting.Thread)
		item := rootCauseItem("sched_stat_accounting", accounting.Thread, backgroundImpactMs(q, impact*0.35, hasCausalChain, onChain), 0.54, accounting.LineStart, accounting.LineEnd, "window_stats.sched_stat_accounting", accounting.Summary+"; corroborating kernel accounting, not double-counted as a scheduler interval")
		item.CumulativeImpactMs = impact
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.StartTs = accounting.StartTs
		item.EndTs = accounting.EndTs
		items = append(items, item)
	}
	for _, work := range stats.WorkqueueActivity {
		onChain := threadInSet(chainThreads, work.Thread)
		item := rootCauseItem("workqueue_activity", work.Thread, backgroundImpactMs(q, work.DurationMs, hasCausalChain, onChain), 0.62, work.LineStart, work.LineEnd, "window_stats.workqueue_activity", work.Summary)
		item.CumulativeImpactMs = work.DurationMs
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.StartTs = work.StartTs
		item.EndTs = work.EndTs
		items = append(items, item)
	}
	if supply := stats.SupplyPressureSummary; supply != nil {
		impact := firstPositiveFloat(supply.CPUPressureMs, supply.IPIActiveMs, (supply.SchedStatWaitMs+supply.SchedStatIOWaitMs+supply.SchedStatBlockedMs)*0.35)
		if impact <= 0 {
			impact = float64(supply.IPIEventCount) * 0.05
		}
		if impact > 0 {
			item := rootCauseItem("supply_pressure", ThreadRef{}, backgroundImpactMs(q, impact, hasCausalChain, false), 0.58, supply.LineStart, supply.LineEnd, "window_stats.supply_pressure_summary", supply.Summary)
			item.CumulativeImpactMs = impact
			items = append(items, item)
		}
	}
	items = enrichRootCauseItemsWithChainContext(chain, items)
	normalizeRootCauseChainRelevance(items, hasCausalChain)
	normalizeRootCauseCumulativeImpact(items)
	sortRootCauseRankItems(items, hasCausalChain)
	limit := q.Limit
	if limit <= 0 || limit > 12 {
		limit = 12
	}
	if len(items) > limit {
		res.Caveats = append(res.Caveats, fmt.Sprintf("root_cause_rank compacted from %d to %d candidate(s)", len(items), limit))
		items = items[:limit]
	}
	assignRootCauseRanksAndTiers(items)
	if len(items) == 0 {
		res.Caveats = append(res.Caveats, "no deterministic root-cause candidates were found in the selected window")
	}
	res.Items = items
	res.Caveats = append(res.Caveats, stats.Caveats...)
	res.Caveats = append(res.Caveats, chain.Caveats...)
	return res
}

func enrichRootCauseRankWithScheduler(q Query, rank RootCauseRankResult, latency SchedulerLatencyResult, stats WindowStats, chain ChainResult) RootCauseRankResult {
	cpus := map[int]CPUStats{}
	for _, cpu := range stats.CPU {
		cpus[cpu.CPU] = cpu
	}
	chainThreads := wakeupChainThreadSet(chain)
	hasCausalChain := len(chainThreads) > 1
	if !hasCausalChain {
		chainThreads = rankCausalThreadSet(rank)
		hasCausalChain = len(chainThreads) > 0
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
		if ctx, ok := runnableContextForThread(item.Thread, stats.RunnableContext); ok {
			summary = appendRunnableContextToRootSummary(summary, ctx)
		}
		onChain := threadInSet(chainThreads, item.Thread)
		candidate := rootCauseItem("scheduler_latency", item.Thread, backgroundImpactMs(q, item.DurationMs, hasCausalChain, onChain), conf, item.StartLine, item.EndLine, "scheduler_latency_stats", summary)
		candidate.CumulativeImpactMs = item.DurationMs
		candidate.Causality = causalityLabel(hasCausalChain, onChain)
		candidate.ChainRelevance = chainRelevanceFromCausality(candidate.Causality)
		candidate.StartTs = item.StartTs
		candidate.EndTs = item.EndTs
		candidate.DominantState = string(StateRunnable)
		candidate.RunnableMs = item.DurationMs
		rank.Items = append(rank.Items, candidate)
		if frequencyIsLowForCPU(item.Frequency, cpus[item.CPU]) {
			low := rootCauseItem("low_frequency", item.Thread, backgroundImpactMs(q, item.DurationMs, hasCausalChain, onChain), 0.70, item.StartLine, item.EndLine, "scheduler_latency_stats", fmt.Sprintf("%s runnable wait began at %dkHz on cpu=%d, below the CPU's observed max frequency in the selected window", threadLabel(item.Thread), item.Frequency, item.CPU))
			low.CumulativeImpactMs = item.DurationMs
			low.Causality = causalityLabel(hasCausalChain, onChain)
			low.ChainRelevance = chainRelevanceFromCausality(low.Causality)
			low.StartTs = item.StartTs
			low.EndTs = item.EndTs
			low.DominantState = string(StateRunnable)
			low.RunnableMs = item.DurationMs
			rank.Items = append(rank.Items, low)
		}
	}
	for _, supply := range stats.ComputeSupply {
		switch supply.Verdict {
		case "cpu_pressure", "mixed_cpu_pressure_and_low_frequency", "low_frequency_signal":
			typ := "compute_supply"
			if strings.Contains(supply.Verdict, "low_frequency") {
				typ = "low_frequency"
			}
			onChain := threadInSet(chainThreads, supply.Thread)
			candidate := rootCauseItem(typ, supply.Thread, backgroundImpactMs(q, supply.DurationMs, hasCausalChain, onChain), supply.Confidence, supply.LineStart, supply.LineEnd, "window_stats.compute_supply", supply.Summary)
			candidate.CumulativeImpactMs = supply.DurationMs
			candidate.Causality = causalityLabel(hasCausalChain, onChain)
			candidate.ChainRelevance = chainRelevanceFromCausality(candidate.Causality)
			candidate.DominantState = computeSupplyDominantState(supply)
			if candidate.DominantState == string(StateRunning) {
				candidate.RunningMs = supply.DurationMs
			} else if candidate.DominantState == string(StateRunnable) {
				candidate.RunnableMs = supply.DurationMs
			}
			rank.Items = append(rank.Items, candidate)
		}
	}
	for _, constraint := range stats.CPUConstraints {
		if constraint.RunnableWaitMs <= 0 || (len(constraint.AllowedCPUs) == 0 && strings.TrimSpace(constraint.CPUSet) == "") {
			continue
		}
		conf := 0.64
		if strings.Contains(constraint.Policy, "restricted=true") || strings.TrimSpace(constraint.CPUSet) != "" {
			conf = 0.72
		}
		onChain := threadInSet(chainThreads, constraint.Thread)
		candidate := rootCauseItem("cpu_affinity_or_cpuset", constraint.Thread, backgroundImpactMs(q, constraint.RunnableWaitMs, hasCausalChain, onChain), conf, constraint.LineStart, constraint.LineEnd, "window_stats.cpu_constraints", constraint.Summary)
		candidate.CumulativeImpactMs = constraint.RunnableWaitMs
		candidate.Causality = causalityLabel(hasCausalChain, onChain)
		candidate.ChainRelevance = chainRelevanceFromCausality(candidate.Causality)
		candidate.DominantState = string(StateRunnable)
		candidate.RunnableMs = constraint.RunnableWaitMs
		rank.Items = append(rank.Items, candidate)
	}
	normalizeRootCauseChainRelevance(rank.Items, hasCausalChain)
	normalizeRootCauseCumulativeImpact(rank.Items)
	sortRootCauseRankItems(rank.Items, hasCausalChain)
	limit := q.Limit
	if limit <= 0 || limit > 12 {
		limit = 12
	}
	if len(rank.Items) > limit {
		rank.Caveats = append(rank.Caveats, fmt.Sprintf("root_cause_rank compacted after scheduler/compute enrichment from %d to %d candidate(s)", len(rank.Items), limit))
		rank.Items = rank.Items[:limit]
	}
	assignRootCauseRanksAndTiers(rank.Items)
	rank.Caveats = append(rank.Caveats, latency.Caveats...)
	return rank
}

func attachPerfContextToRootCauseRank(idx *Index, q Query, rank RootCauseRankResult, stats WindowStats) RootCauseRankResult {
	if idx == nil || len(rank.Items) == 0 {
		return rank
	}
	for i := range rank.Items {
		item := &rank.Items[i]
		start, end := rootCausePerfWindow(q, *item)
		window := TimeWindow{StartTs: start, EndTs: end}
		var contexts []RootCausePerfRoleContext
		if item.Thread.PID > 0 || strings.TrimSpace(item.Thread.Comm) != "" {
			contexts = appendRootCausePerfRoleContext(contexts, "candidate_thread", item.Thread, -1, window, "root-cause candidate thread", perfContextForThread(idx, q, item.Thread, start, end, 4))
		}
		if item.NearestChainThread.PID > 0 && !sameThreadRef(item.Thread, item.NearestChainThread) {
			contexts = appendRootCausePerfRoleContext(contexts, "nearest_chain_thread", item.NearestChainThread, -1, window, "nearest wakeup-chain thread", perfContextForThread(idx, q, item.NearestChainThread, start, end, 4))
		}
		if rootCauseItemIsOnChain(*item) && (item.Thread.PID > 0 || strings.TrimSpace(item.Thread.Comm) != "") {
			contexts = appendRootCausePerfRoleContext(contexts, "on_chain_dependency", item.Thread, -1, window, "on-chain root-cause dependency", perfContextForThread(idx, q, item.Thread, start, end, 4))
		}
		contexts = appendRootCauseStatsPerfContexts(idx, q, stats, *item, window, contexts)
		item.PerfContexts = contexts
		item.PerfContext = primaryRootCausePerfContext(contexts)
	}
	return rank
}

func appendRootCauseStatsPerfContexts(idx *Index, q Query, stats WindowStats, item RootCauseRankItem, window TimeWindow, contexts []RootCausePerfRoleContext) []RootCausePerfRoleContext {
	start, end := window.StartTs, window.EndTs
	if ctx, ok := runnableContextForThread(item.Thread, stats.RunnableContext); ok {
		contexts = appendRootCauseRunnableCompetitorPerfContexts(idx, q, ctx.SameCPUTopRunning, item.Thread, window, "same_cpu_competitor", "same-CPU top running competitor", contexts)
	}
	switch item.Type {
	case "cpu_pressure":
		for _, pressure := range matchingCPUPressuresForRootCauseItem(item, stats.CPUPressure, 1) {
			contexts = appendRootCauseCPUPressurePerfContexts(idx, q, pressure, window, contexts)
		}
	case "supply_pressure":
		for _, pressure := range matchingCPUPressuresForRootCauseItem(item, stats.CPUPressure, 3) {
			contexts = appendRootCauseCPUPressurePerfContexts(idx, q, pressure, window, contexts)
		}
	case "compute_supply", "low_frequency", "cpu_affinity_or_cpuset":
		if supply, ok := computeSupplyForRootCauseItem(item, stats.ComputeSupply); ok {
			cpuCtx := perfContextForCPU(idx, q, supply.CPU, start, end, 4)
			contexts = appendRootCausePerfRoleContext(contexts, "compute_supply_cpu", ThreadRef{}, supply.CPU, window, "compute-supply CPU scope", cpuCtx)
			for _, pressure := range stats.CPUPressure {
				if pressure.CPU == supply.CPU {
					contexts = appendRootCauseCPUPressurePerfContexts(idx, q, pressure, window, contexts)
					break
				}
			}
		}
	case "runnable_wait", "scheduler_latency", "fragmented_runnable_wait":
		if ctx, ok := runnableContextForThread(item.Thread, stats.RunnableContext); ok {
			cpuCtx := perfContextForCPU(idx, q, ctx.CPU, start, end, 4)
			contexts = appendRootCausePerfRoleContext(contexts, "runnable_cpu", ThreadRef{}, ctx.CPU, window, "runnable wait CPU scope", cpuCtx)
		}
	case "fragmented_running", "running":
		if item.Thread.PID > 0 || strings.TrimSpace(item.Thread.Comm) != "" {
			contexts = appendRootCausePerfRoleContext(contexts, "target_running", item.Thread, -1, window, "running-state CPU work", perfContextForThread(idx, q, item.Thread, start, end, 4))
		}
	}
	return contexts
}

func appendRootCauseCPUPressurePerfContexts(idx *Index, q Query, pressure CPUPressureStats, window TimeWindow, contexts []RootCausePerfRoleContext) []RootCausePerfRoleContext {
	start, end := window.StartTs, window.EndTs
	contexts = appendRootCausePerfRoleContext(contexts, "cpu_pressure_cpu", ThreadRef{}, pressure.CPU, window, "CPU pressure scope", perfContextForCPU(idx, q, pressure.CPU, start, end, 4))
	return appendRootCauseRunnableCompetitorPerfContexts(idx, q, pressure.TopRunning, ThreadRef{}, window, "cpu_pressure_top_running", "top running thread on pressure CPU", contexts)
}

func appendRootCauseRunnableCompetitorPerfContexts(idx *Index, q Query, threads []ThreadDuration, candidate ThreadRef, window TimeWindow, role, reason string, contexts []RootCausePerfRoleContext) []RootCausePerfRoleContext {
	limit := 2
	for _, td := range threads {
		if limit <= 0 {
			break
		}
		if sameThreadRef(td.Thread, candidate) {
			continue
		}
		start, end := rootCauseThreadDurationWindow(window, td)
		ctx := perfContextForThread(idx, q, td.Thread, start, end, 4)
		roleWindow := TimeWindow{StartTs: start, EndTs: end}
		if ctx == nil && !sameTimeWindow(roleWindow, window) {
			ctx = perfContextForThread(idx, q, td.Thread, window.StartTs, window.EndTs, 4)
			roleWindow = window
		}
		before := len(contexts)
		contexts = appendRootCausePerfRoleContext(contexts, role, td.Thread, td.CPU, roleWindow, reason, ctx)
		if len(contexts) > before {
			limit--
		}
	}
	return contexts
}

func appendRootCausePerfRoleContext(contexts []RootCausePerfRoleContext, role string, thread ThreadRef, cpu int, window TimeWindow, reason string, ctx *PerfContext) []RootCausePerfRoleContext {
	if ctx == nil || ctx.SampleCount == 0 {
		return contexts
	}
	for _, existing := range contexts {
		if existing.Role == role && existing.CPU == cpu && sameThreadRef(existing.Thread, thread) && sameTimeWindow(existing.Window, window) {
			return contexts
		}
	}
	return append(contexts, RootCausePerfRoleContext{
		Role:        role,
		Thread:      thread,
		CPU:         cpu,
		Window:      window,
		Reason:      reason,
		PerfContext: ctx,
	})
}

func primaryRootCausePerfContext(contexts []RootCausePerfRoleContext) *PerfContext {
	if len(contexts) == 0 {
		return nil
	}
	preferred := []string{"candidate_thread", "on_chain_dependency", "nearest_chain_thread", "target_running", "cpu_pressure_top_running", "same_cpu_competitor", "compute_supply_cpu", "cpu_pressure_cpu", "runnable_cpu"}
	for _, role := range preferred {
		for _, ctx := range contexts {
			if ctx.Role == role && ctx.PerfContext != nil {
				return ctx.PerfContext
			}
		}
	}
	return contexts[0].PerfContext
}

func perfContextForCPU(idx *Index, q Query, cpu int, start, end float64, max int) *PerfContext {
	if cpu < 0 {
		return nil
	}
	sub := queryForPerfContextWindow(q, start, end)
	return computePerfContextFiltered(idx, sub, max, func(ev Event) bool {
		return ev.CPU == cpu
	})
}

func matchingCPUPressuresForRootCauseItem(item RootCauseRankItem, pressures []CPUPressureStats, limit int) []CPUPressureStats {
	if limit <= 0 {
		limit = 1
	}
	if len(pressures) == 0 {
		return nil
	}
	if item.Type == "supply_pressure" {
		out := append([]CPUPressureStats(nil), pressures...)
		sort.SliceStable(out, func(i, j int) bool { return out[i].RunnableWaitMs > out[j].RunnableWaitMs })
		if len(out) > limit {
			out = out[:limit]
		}
		return out
	}
	type scored struct {
		pressure CPUPressureStats
		score    float64
	}
	var scoredItems []scored
	for _, pressure := range pressures {
		score := float64(lineOverlapLength(item.LineStart, item.LineEnd, firstThreadLine(pressure.TopRunnable), lastThreadLine(pressure.TopRunning)))
		if score <= 0 && item.CumulativeImpactMs > 0 && floatsClose(item.CumulativeImpactMs, pressure.RunnableWaitMs, 0.001) {
			score = pressure.RunnableWaitMs
		}
		if score > 0 {
			scoredItems = append(scoredItems, scored{pressure: pressure, score: score})
		}
	}
	if len(scoredItems) == 0 {
		out := append([]CPUPressureStats(nil), pressures...)
		sort.SliceStable(out, func(i, j int) bool { return out[i].RunnableWaitMs > out[j].RunnableWaitMs })
		if len(out) > limit {
			out = out[:limit]
		}
		return out
	}
	sort.SliceStable(scoredItems, func(i, j int) bool {
		if scoredItems[i].score != scoredItems[j].score {
			return scoredItems[i].score > scoredItems[j].score
		}
		return scoredItems[i].pressure.RunnableWaitMs > scoredItems[j].pressure.RunnableWaitMs
	})
	out := make([]CPUPressureStats, 0, minInt(limit, len(scoredItems)))
	for i := 0; i < len(scoredItems) && i < limit; i++ {
		out = append(out, scoredItems[i].pressure)
	}
	return out
}

func computeSupplyForRootCauseItem(item RootCauseRankItem, summaries []ComputeSupplySummary) (ComputeSupplySummary, bool) {
	var best ComputeSupplySummary
	bestScore := 0
	for _, summary := range summaries {
		if !sameThreadRef(item.Thread, summary.Thread) {
			continue
		}
		score := lineOverlapLength(item.LineStart, item.LineEnd, summary.LineStart, summary.LineEnd)
		if score <= 0 && item.CumulativeImpactMs > 0 && floatsClose(item.CumulativeImpactMs, summary.DurationMs, 0.001) {
			score = 1
		}
		if score > bestScore {
			best = summary
			bestScore = score
		}
	}
	return best, bestScore > 0
}

func rootCauseThreadDurationWindow(fallback TimeWindow, td ThreadDuration) (float64, float64) {
	start, end := td.StartTs, td.EndTs
	if start <= 0 || end <= start {
		return fallback.StartTs, fallback.EndTs
	}
	if fallback.StartTs > 0 && start < fallback.StartTs {
		start = fallback.StartTs
	}
	if fallback.EndTs > 0 && end > fallback.EndTs {
		end = fallback.EndTs
	}
	if end <= start {
		return fallback.StartTs, fallback.EndTs
	}
	return start, end
}

func lineOverlapLength(aStart, aEnd, bStart, bEnd int) int {
	if aStart <= 0 || bStart <= 0 {
		return 0
	}
	if aEnd <= 0 {
		aEnd = aStart
	}
	if bEnd <= 0 {
		bEnd = bStart
	}
	start := maxInt(aStart, bStart)
	end := minInt(aEnd, bEnd)
	if end < start {
		return 0
	}
	return end - start + 1
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func floatsClose(a, b, tolerance float64) bool {
	if tolerance <= 0 {
		tolerance = 0.001
	}
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

func sameTimeWindow(a, b TimeWindow) bool {
	return floatsClose(a.StartTs, b.StartTs, 0.000001) && floatsClose(a.EndTs, b.EndTs, 0.000001)
}

func rootCausePerfWindow(q Query, item RootCauseRankItem) (float64, float64) {
	start, end := item.StartTs, item.EndTs
	if (item.Thread.PID > 0 || strings.TrimSpace(item.Thread.Comm) != "") && (start <= 0 || end <= start) {
		start, end = item.NearestChainWindow.StartTs, item.NearestChainWindow.EndTs
	}
	if start <= 0 || end <= start {
		start, end = q.TimeStart, q.TimeEnd
	}
	return start, end
}

type framePerfContexts struct {
	PerfSamples           *PerfContext
	TargetRunningPerf     *PerfContext
	OnChainPerf           *PerfContext
	BinderPeerPerf        *PerfContext
	SameCPUCompetitorPerf *PerfContext
}

func buildFramePerfContexts(idx *Index, q Query, stats WindowStats, chain *ChainResult, blocking CriticalBlockingResult, target ThreadRef) framePerfContexts {
	out := framePerfContexts{
		PerfSamples:       stats.PerfSamples,
		TargetRunningPerf: perfContextForThread(idx, q, target, q.TimeStart, q.TimeEnd, 6),
	}
	if chain != nil {
		out.OnChainPerf = perfContextForThreads(idx, q, chainThreadRefs(*chain), 6)
	}
	out.BinderPeerPerf = perfContextForThreads(idx, q, binderPeerThreadRefs(blocking), 6)
	if cpuCtx := perfContextForCPUs(idx, q, sameCPUCompetitorCPUs(stats), 6); cpuCtx != nil {
		out.SameCPUCompetitorPerf = cpuCtx
	} else {
		out.SameCPUCompetitorPerf = perfContextForThreads(idx, q, sameCPUCompetitorThreadRefs(stats), 6)
	}
	return out
}

func chainThreadRefs(chain ChainResult) map[int]ThreadRef {
	out := map[int]ThreadRef{}
	addThreadRef(out, chain.Target)
	for _, node := range chain.Nodes {
		addThreadRef(out, node.Thread)
	}
	for _, impact := range chain.CausalImpacts {
		addThreadRef(out, impact.Thread)
	}
	for _, aggregate := range chain.AggregatedImpacts {
		addThreadRef(out, aggregate.Thread)
	}
	for _, edge := range chain.Edges {
		addThreadRef(out, edge.Waker)
		addThreadRef(out, edge.Wakee)
	}
	for _, wait := range chain.BinderWaits {
		addThreadRef(out, wait.Thread)
		addThreadRef(out, wait.Peer)
	}
	return out
}

func binderPeerThreadRefs(blocking CriticalBlockingResult) map[int]ThreadRef {
	out := map[int]ThreadRef{}
	for _, item := range blocking.Items {
		if item.Type == "binder_wait" || item.SyncLike != nil || item.BlockingCandidate != nil {
			addThreadRef(out, item.Peer)
		}
	}
	return out
}

func sameCPUCompetitorCPUs(stats WindowStats) map[int]bool {
	out := map[int]bool{}
	for _, ctx := range stats.RunnableContext {
		if ctx.RunnableWaitMs > 0 && ctx.CPU >= 0 {
			out[ctx.CPU] = true
		}
	}
	for _, pressure := range stats.CPUPressure {
		if pressure.RunnableWaitMs > 0 && pressure.CPU >= 0 {
			out[pressure.CPU] = true
		}
	}
	return out
}

func sameCPUCompetitorThreadRefs(stats WindowStats) map[int]ThreadRef {
	out := map[int]ThreadRef{}
	for _, ctx := range stats.RunnableContext {
		for _, td := range ctx.SameCPUTopRunning {
			addThreadRef(out, td.Thread)
		}
	}
	for _, pressure := range stats.CPUPressure {
		if pressure.RunnableWaitMs <= 0 {
			continue
		}
		for _, td := range pressure.TopRunning {
			addThreadRef(out, td.Thread)
		}
	}
	return out
}

func addThreadRef(out map[int]ThreadRef, thread ThreadRef) {
	if out == nil || thread.PID <= 0 {
		return
	}
	if existing, ok := out[thread.PID]; ok {
		if existing.Comm == "" && thread.Comm != "" {
			existing.Comm = thread.Comm
		}
		if existing.TGID == 0 && thread.TGID != 0 {
			existing.TGID = thread.TGID
		}
		out[thread.PID] = existing
		return
	}
	out[thread.PID] = thread
}

func normalizeRootCauseChainRelevance(items []RootCauseRankItem, hasCausalChain bool) {
	if !hasCausalChain {
		return
	}
	for i := range items {
		relevance := strings.TrimSpace(items[i].ChainRelevance)
		if relevance == "" {
			relevance = chainRelevanceFromCausality(items[i].Causality)
		}
		if relevance == "" {
			relevance = "background"
		}
		items[i].ChainRelevance = relevance
		if causality := causalityFromChainRelevance(relevance); causality != "" {
			items[i].Causality = causality
		}
	}
}

func normalizeRootCauseCumulativeImpact(items []RootCauseRankItem) {
	for i := range items {
		if items[i].CumulativeImpactMs > 0 {
			continue
		}
		items[i].CumulativeImpactMs = rootCauseCumulativeImpactMs(items[i])
	}
}

func sortRootCauseRankItems(items []RootCauseRankItem, chainAware bool) {
	sort.SliceStable(items, func(i, j int) bool {
		if chainAware {
			ri := rootCauseChainRelevanceSortRank(items[i])
			rj := rootCauseChainRelevanceSortRank(items[j])
			if ri != rj {
				return ri < rj
			}
			if ri == chainRelevanceRank("on_chain") {
				ci := rootCauseCumulativeImpactMs(items[i])
				cj := rootCauseCumulativeImpactMs(items[j])
				if ci != cj {
					return ci > cj
				}
			}
		}
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		if items[i].ImpactMs != items[j].ImpactMs {
			return items[i].ImpactMs > items[j].ImpactMs
		}
		return items[i].LineStart < items[j].LineStart
	})
}

func rootCauseChainRelevanceSortRank(item RootCauseRankItem) int {
	relevance := strings.TrimSpace(item.ChainRelevance)
	if relevance == "" {
		relevance = chainRelevanceFromCausality(item.Causality)
	}
	return chainRelevanceRank(relevance)
}

func rootCauseCumulativeImpactMs(item RootCauseRankItem) float64 {
	if item.CumulativeImpactMs > 0 {
		return item.CumulativeImpactMs
	}
	stateTotal := item.RunningMs + item.RunnableMs + item.SleepMs + item.DStateMs + item.IOWaitMs
	if stateTotal > 0 {
		return stateTotal
	}
	if item.ImpactMs > 0 {
		return item.ImpactMs
	}
	return item.TargetImpactMs
}

func runnableContextForThread(thread ThreadRef, contexts []RunnableContextSummary) (RunnableContextSummary, bool) {
	for _, ctx := range contexts {
		if sameThreadRef(ctx.Thread, thread) {
			return ctx, true
		}
	}
	return RunnableContextSummary{}, false
}

func appendRunnableContextToRootSummary(summary string, ctx RunnableContextSummary) string {
	if ctx.CoreClass != "" {
		summary = fmt.Sprintf("%s; core_class=%s", summary, ctx.CoreClass)
	}
	if len(ctx.TopBackgroundThreads) > 0 {
		top := ctx.TopBackgroundThreads[0]
		summary = fmt.Sprintf("%s; top_background_thread=%s load=%.3fms", summary, threadLabel(top.Thread), top.RunningMs+top.RunnableWaitMs)
	}
	if ctx.TopBackgroundProcess != nil {
		summary = fmt.Sprintf("%s; top_background_process=%s load=%.3fms", summary, threadLabel(ctx.TopBackgroundProcess.Process), ctx.TopBackgroundProcess.RunningMs+ctx.TopBackgroundProcess.RunnableWaitMs)
	}
	if ctx.CPUConstraint != nil {
		if len(ctx.CPUConstraint.AllowedCPUs) > 0 {
			summary = fmt.Sprintf("%s; allowed_cpus=%s", summary, intListString(ctx.CPUConstraint.AllowedCPUs))
		}
		if len(ctx.CPUConstraint.AllowedCoreClasses) > 0 {
			summary = fmt.Sprintf("%s; allowed_core_classes=%s", summary, strings.Join(ctx.CPUConstraint.AllowedCoreClasses, ","))
		}
		if ctx.CPUConstraint.CPUSet != "" {
			summary = fmt.Sprintf("%s; cpuset=%s", summary, ctx.CPUConstraint.CPUSet)
		}
	}
	if ctx.Verdict != "" {
		summary = fmt.Sprintf("%s; runnable_context_verdict=%s", summary, ctx.Verdict)
	}
	return summary
}

func rankCausalThreadSet(rank RootCauseRankResult) map[int]bool {
	out := map[int]bool{}
	for _, item := range rank.Items {
		if item.Causality == "on_wakeup_chain" && item.Thread.PID > 0 {
			out[item.Thread.PID] = true
		}
	}
	return out
}

func rootCauseItem(typ string, thread ThreadRef, impactMs float64, confidence float64, lineStart, lineEnd int, source, summary string) RootCauseRankItem {
	if confidence <= 0 {
		confidence = 0.5
	}
	return RootCauseRankItem{
		Type:               typ,
		Thread:             thread,
		ImpactMs:           impactMs,
		ProjectedImpactMs:  impactMs,
		CumulativeImpactMs: impactMs,
		Score:              impactMs * confidence * rootCauseTypeWeight(typ),
		Confidence:         confidence,
		LineStart:          lineStart,
		LineEnd:            lineEnd,
		Source:             source,
		Summary:            summary,
	}
}

func rootCauseItemFromCausalImpact(impact WakeupCausalImpact) RootCauseRankItem {
	typ := causalImpactRootType(impact)
	if impact.PriorityInversionCandidate {
		typ = "priority_inversion_candidate"
	}
	conf := 0.86
	if impact.PriorityInversionCandidate {
		conf = 0.91
	}
	impactMs := causalImpactBlockingMs(impact)
	item := rootCauseItem(typ, impact.Thread, impactMs, conf, impact.LineStart, impact.LineEnd, "wakeup_chain.causal_impacts", impact.Summary)
	item.Causality = "on_wakeup_chain"
	item.ChainRelevance = "on_chain"
	item.ChainDepth = impact.ChainDepth
	item.CumulativeImpactMs = impact.TotalMs
	item.TargetImpactMs = impact.TargetBlockedMs
	item.StartTs = impact.Window.StartTs
	item.EndTs = impact.Window.EndTs
	item.ActualStartTs = impact.ActualWindow.StartTs
	item.ActualEndTs = impact.ActualWindow.EndTs
	item.DominantState = impact.DominantState
	item.RunningMs = impact.RunningMs
	item.RunnableMs = impact.RunnableMs
	item.SleepMs = impact.SleepMs
	item.DStateMs = impact.DStateMs
	item.IOWaitMs = impact.IOWaitMs
	item.ProjectedImpactMs = firstPositiveFloat(impact.ProjectedImpactMs, impactMs)
	item.ActualImpactMs = impact.ActualImpactMs
	item.ActualTotalMs = impact.ActualTotalMs
	item.Score = impactMs * conf * 2.0
	return item
}

func rootCauseItemFromCausalAggregate(aggregate WakeupCausalAggregate) RootCauseRankItem {
	typ := aggregateRootCauseType(aggregate)
	if aggregate.PriorityInversion && aggregateRootCauseIsPrioritySensitive(aggregate) {
		typ = "priority_inversion_candidate"
	}
	conf := 0.82
	if aggregate.PriorityInversion {
		conf = 0.88
	}
	impactMs := aggregateBlockingMs(aggregate)
	item := rootCauseItem(typ, aggregate.Thread, impactMs, conf, aggregate.LineStart, aggregate.LineEnd, "wakeup_chain.aggregated_impacts", aggregate.Summary)
	item.Causality = "on_wakeup_chain"
	item.ChainRelevance = "on_chain"
	item.ChainDepth = aggregate.ChainDepth
	item.CumulativeImpactMs = aggregate.TotalMs
	item.TargetImpactMs = aggregate.TargetBlockedMs
	item.StartTs = aggregate.FirstTs
	item.EndTs = aggregate.LastTs
	item.ActualStartTs = aggregate.ActualFirstTs
	item.ActualEndTs = aggregate.ActualLastTs
	item.DominantState = aggregate.DominantState
	item.RunningMs = aggregate.RunningMs
	item.RunnableMs = aggregate.RunnableMs
	item.SleepMs = aggregate.SleepMs
	item.DStateMs = aggregate.DStateMs
	item.IOWaitMs = aggregate.IOWaitMs
	item.ProjectedImpactMs = firstPositiveFloat(aggregate.ProjectedImpactMs, impactMs)
	item.ActualImpactMs = aggregate.ActualImpactMs
	item.ActualTotalMs = aggregate.ActualTotalMs
	item.OccurrenceWindows = append([]WakeupCausalOccurrence(nil), aggregate.OccurrenceWindows...)
	item.Score = impactMs * conf * 2.05
	return item
}

type semanticTraceSpanProjection struct {
	StartTs       float64
	EndTs         float64
	ImpactMs      float64
	DominantState string
	ChainDepth    int
	OnChain       bool
}

func rootCauseItemFromSemanticTraceSpan(q Query, chain ChainResult, span TraceSpanSummary, hasCausalChain bool) (RootCauseRankItem, bool) {
	work, ok := traceSpanSemanticWorkClass(span.Name)
	if !ok || span.DurationMs <= 0 || span.EndTs <= span.StartTs {
		return RootCauseRankItem{}, false
	}
	projection := semanticTraceSpanProjectionForRootCause(q, chain, span)
	if projection.ImpactMs <= 0 {
		return RootCauseRankItem{}, false
	}
	summary := fmt.Sprintf("%s span %q overlapped %s for %.3fms; actual_span=%.3fms window=%.6f..%.6f",
		work.Label, span.Name, semanticTraceSpanProjectionScope(projection, hasCausalChain), projection.ImpactMs, span.DurationMs, span.StartTs, span.EndTs)
	if projection.DominantState != "" {
		summary = fmt.Sprintf("%s; overlapped_chain_state=%s", summary, projection.DominantState)
	}
	item := rootCauseItem(work.RootCauseType, span.Thread, projection.ImpactMs, work.Confidence, span.StartLine, span.EndLine, "window_stats.trace_spans.semantic", summary)
	item.StartTs = projection.StartTs
	item.EndTs = projection.EndTs
	item.ActualStartTs = span.StartTs
	item.ActualEndTs = span.EndTs
	item.ActualImpactMs = span.DurationMs
	item.ActualTotalMs = span.DurationMs
	item.ProjectedImpactMs = projection.ImpactMs
	item.CumulativeImpactMs = projection.ImpactMs
	item.SpanName = span.Name
	item.SpanKind = span.Kind
	item.SpanCategory = firstNonEmpty(span.Category, work.Category)
	item.SpanSubcategory = firstNonEmpty(span.Subcategory, work.Subcategory)
	item.SemanticClass = work.SemanticClass
	item.DominantState = projection.DominantState
	item.ChainDepth = projection.ChainDepth
	if projection.OnChain {
		item.Causality = "on_wakeup_chain"
		item.ChainRelevance = "on_chain"
	}
	applySemanticTraceSpanState(&item, projection.DominantState, projection.ImpactMs)
	item.Score = item.ImpactMs * item.Confidence * rootCauseTypeWeight(item.Type)
	return item, true
}

func semanticTraceSpanProjectionForRootCause(q Query, chain ChainResult, span TraceSpanSummary) semanticTraceSpanProjection {
	start, end, ok := selectedTraceSpanWindow(q, span)
	if !ok {
		return semanticTraceSpanProjection{}
	}
	if len(chain.Nodes) > 0 || len(chain.Edges) > 0 || len(chain.CausalImpacts) > 0 {
		var out semanticTraceSpanProjection
		bestImpactMs := 0.0
		for _, node := range chain.Nodes {
			if !sameThreadRef(node.Thread, span.Thread) {
				continue
			}
			overlapStart, overlapEnd, overlapOK := overlapTimeWindow(start, end, node.Window.StartTs, node.Window.EndTs)
			if !overlapOK {
				continue
			}
			impactMs := (overlapEnd - overlapStart) * 1000
			if impactMs <= 0 {
				continue
			}
			if out.ImpactMs == 0 || overlapStart < out.StartTs {
				out.StartTs = overlapStart
			}
			if overlapEnd > out.EndTs {
				out.EndTs = overlapEnd
			}
			out.ImpactMs += impactMs
			if impactMs > bestImpactMs || out.DominantState == "" {
				bestImpactMs = impactMs
				out.DominantState = string(node.Dominant)
				if node.Impact != nil {
					out.ChainDepth = node.Impact.ChainDepth
					if out.DominantState == "" {
						out.DominantState = node.Impact.DominantState
					}
				}
			}
			out.OnChain = true
		}
		if out.ImpactMs <= 0 {
			for _, impact := range chain.CausalImpacts {
				if !sameThreadRef(impact.Thread, span.Thread) {
					continue
				}
				overlapStart, overlapEnd, overlapOK := overlapTimeWindow(start, end, impact.Window.StartTs, impact.Window.EndTs)
				if !overlapOK {
					continue
				}
				impactMs := (overlapEnd - overlapStart) * 1000
				if impactMs <= 0 {
					continue
				}
				if out.ImpactMs == 0 || overlapStart < out.StartTs {
					out.StartTs = overlapStart
				}
				if overlapEnd > out.EndTs {
					out.EndTs = overlapEnd
				}
				out.ImpactMs += impactMs
				if impactMs > bestImpactMs || out.DominantState == "" {
					bestImpactMs = impactMs
					out.DominantState = impact.DominantState
					out.ChainDepth = impact.ChainDepth
				}
				out.OnChain = true
			}
		}
		return out
	}
	if q.PID > 0 && span.Thread.PID > 0 && q.PID != span.Thread.PID {
		return semanticTraceSpanProjection{}
	}
	return semanticTraceSpanProjection{
		StartTs:  start,
		EndTs:    end,
		ImpactMs: (end - start) * 1000,
	}
}

func selectedTraceSpanWindow(q Query, span TraceSpanSummary) (float64, float64, bool) {
	if span.EndTs <= span.StartTs {
		return 0, 0, false
	}
	start, end := span.StartTs, span.EndTs
	if q.TimeEnd > q.TimeStart {
		var ok bool
		start, end, ok = overlapTimeWindow(start, end, q.TimeStart, q.TimeEnd)
		if !ok {
			return 0, 0, false
		}
	}
	return start, end, end > start
}

func overlapTimeWindow(aStart, aEnd, bStart, bEnd float64) (float64, float64, bool) {
	if aEnd <= aStart || bEnd <= bStart {
		return 0, 0, false
	}
	start := maxFloat(aStart, bStart)
	end := minFloat(aEnd, bEnd)
	if end <= start {
		return 0, 0, false
	}
	return start, end, true
}

func semanticTraceSpanProjectionScope(projection semanticTraceSpanProjection, hasCausalChain bool) string {
	if projection.OnChain {
		return "direct wakeup-chain interval"
	}
	if hasCausalChain {
		return "selected non-chain interval"
	}
	return "selected window"
}

func applySemanticTraceSpanState(item *RootCauseRankItem, state string, impactMs float64) {
	if item == nil || impactMs <= 0 {
		return
	}
	switch state {
	case string(StateRunning):
		item.RunningMs = impactMs
	case string(StateRunnable):
		item.RunnableMs = impactMs
	case string(StateSSleep):
		item.SleepMs = impactMs
	case string(StateDSleep):
		item.DStateMs = impactMs
	case string(StateIOWait):
		item.IOWaitMs = impactMs
	}
}

func aggregateBlockingMs(item WakeupCausalAggregate) float64 {
	if item.DominantState == string(StateDSleep) || item.DominantState == string(StateIOWait) {
		return item.DStateMs + item.IOWaitMs
	}
	return item.DominantImpactMs
}

func actualAggregateBlockingMs(item WakeupCausalAggregate) float64 {
	if item.DominantState == string(StateDSleep) || item.DominantState == string(StateIOWait) {
		return item.ActualDStateMs + item.ActualIOWaitMs
	}
	switch item.DominantState {
	case string(StateRunning):
		return item.ActualRunningMs
	case string(StateRunnable):
		return item.ActualRunnableMs
	case string(StateSSleep):
		return item.ActualSleepMs
	case string(StateDSleep):
		return item.ActualDStateMs
	case string(StateIOWait):
		return item.ActualIOWaitMs
	default:
		return item.ActualTotalMs
	}
}

func aggregateRootCauseIsPrioritySensitive(item WakeupCausalAggregate) bool {
	switch item.DominantState {
	case string(StateRunnable), string(StateDSleep), string(StateIOWait):
		return aggregateBlockingMs(item) > 0
	default:
		return false
	}
}

func aggregateRootCauseType(item WakeupCausalAggregate) string {
	switch item.DominantState {
	case string(StateRunnable):
		if item.PriorityInversion {
			return "priority_inversion_runnable_wait"
		}
		return "runnable_wait"
	case string(StateDSleep):
		if item.IOWaitMs > 0 {
			return "io_wait"
		}
		return "d_state_or_io_wait"
	case string(StateIOWait):
		return "io_wait"
	case string(StateSSleep):
		return "sleep_wait"
	case string(StateRunning):
		return "running"
	default:
		return "unknown_state"
	}
}

func wakeupChainThreadSet(chain ChainResult) map[int]bool {
	out := map[int]bool{}
	if chain.Target.PID > 0 {
		out[chain.Target.PID] = true
	}
	for _, node := range chain.Nodes {
		if node.Thread.PID > 0 {
			out[node.Thread.PID] = true
		}
	}
	for _, impact := range chain.CausalImpacts {
		if impact.Thread.PID > 0 {
			out[impact.Thread.PID] = true
		}
	}
	for _, edge := range chain.Edges {
		if edge.Waker.PID > 0 {
			out[edge.Waker.PID] = true
		}
		if edge.Wakee.PID > 0 {
			out[edge.Wakee.PID] = true
		}
	}
	return out
}

func isIntermediateSleepImpact(chain ChainResult, impact WakeupCausalImpact) bool {
	if impact.ChainDepth <= 0 || impact.DominantState != string(StateSSleep) || impact.Thread.PID <= 0 {
		return false
	}
	for _, edge := range chain.Edges {
		if edge.Wakee.PID == impact.Thread.PID {
			return true
		}
	}
	return false
}

func isIntermediateSleepAggregate(chain ChainResult, aggregate WakeupCausalAggregate) bool {
	if aggregate.ChainDepth <= 0 || aggregate.DominantState != string(StateSSleep) || aggregate.Thread.PID <= 0 {
		return false
	}
	for _, edge := range chain.Edges {
		if edge.Wakee.PID == aggregate.Thread.PID {
			return true
		}
	}
	return false
}

func threadInSet(set map[int]bool, thread ThreadRef) bool {
	return thread.PID > 0 && set[thread.PID]
}

func causalityLabel(hasCausalChain, onChain bool) string {
	if !hasCausalChain {
		return ""
	}
	if onChain {
		return "on_wakeup_chain"
	}
	return "background"
}

type chainCandidateContext struct {
	relevance string
	nearest   ThreadRef
	window    TimeWindow
	overlapMs float64
	edgeCount int
}

func enrichRootCauseItemsWithChainContext(chain ChainResult, items []RootCauseRankItem) []RootCauseRankItem {
	hasChain := len(chain.Nodes) > 0 || len(chain.Edges) > 0 || len(chain.CausalImpacts) > 0
	for i := range items {
		ctx := chainContextForCandidate(chain, items[i].Thread, items[i].StartTs, items[i].EndTs)
		if ctx.relevance == "" {
			if hasChain {
				ctx.relevance = "background"
			} else {
				ctx.relevance = ""
			}
		}
		ctx = rootCauseChainContextForItem(items[i], ctx)
		items[i].ChainRelevance = ctx.relevance
		if causality := causalityFromChainRelevance(ctx.relevance); causality != "" {
			items[i].Causality = causality
		}
		if ctx.edgeCount > 0 {
			items[i].EdgeCount = ctx.edgeCount
		}
		if ctx.overlapMs > 0 {
			items[i].OverlapMs = ctx.overlapMs
		}
		if ctx.nearest.PID > 0 || ctx.nearest.Comm != "" {
			items[i].NearestChainThread = ctx.nearest
			items[i].NearestChainWindow = ctx.window
		}
	}
	return items
}

func rootCauseChainContextForItem(item RootCauseRankItem, ctx chainCandidateContext) chainCandidateContext {
	if ctx.relevance != "on_chain" || rootCauseItemCanBeDirectOnChain(item) {
		return ctx
	}
	ctx.relevance = "adjacent"
	return ctx
}

func rootCauseItemCanBeDirectOnChain(item RootCauseRankItem) bool {
	if !rootCauseTypeCanBeDirectOnChain(item.Type) {
		return false
	}
	if strings.HasPrefix(strings.TrimSpace(item.Source), "wakeup_chain") {
		return true
	}
	if item.Thread.PID <= 0 && strings.TrimSpace(item.Thread.Comm) == "" {
		return false
	}
	return true
}

func rootCauseTypeCanBeDirectOnChain(typ string) bool {
	switch typ {
	case "runnable_wait", "scheduler_latency", "priority_inversion_runnable_wait", "fragmented_runnable_wait",
		"running", "fragmented_running",
		"compute_supply", "low_frequency", "cpu_affinity_or_cpuset",
		"jit_compile", "class_verification", "shader_compile", "runtime_compile",
		"io_wait", "d_state_or_io_wait", "io_latency", "io_burst_episode", "block_io_by_inode", "file_io_hot_inode", "fragmented_d_state_or_io_wait",
		"priority_inversion_candidate", "binder_wait":
		return true
	default:
		return false
	}
}

func chainContextForCandidate(chain ChainResult, thread ThreadRef, start, end float64) chainCandidateContext {
	var ctx chainCandidateContext
	if len(chain.Nodes) == 0 && len(chain.Edges) == 0 && len(chain.CausalImpacts) == 0 {
		return ctx
	}
	hasCandidateWindow := start > 0 && end > start
	if end > 0 && start > end {
		start, end = end, start
		hasCandidateWindow = true
	}
	for _, edge := range chain.Edges {
		if thread.PID > 0 && (edge.Waker.PID == thread.PID || edge.Wakee.PID == thread.PID) {
			ctx.edgeCount++
		}
	}
	bestDistance := 0.0
	foundNearest := false
	for _, node := range chain.Nodes {
		if thread.PID > 0 && node.Thread.PID == thread.PID {
			ctx.relevance = "on_chain"
			ctx.nearest = node.Thread
			ctx.window = node.Window
			if hasCandidateWindow {
				ctx.overlapMs = maxFloat(ctx.overlapMs, windowOverlapMs(start, end, node.Window.StartTs, node.Window.EndTs))
			} else {
				ctx.overlapMs = maxFloat(ctx.overlapMs, (node.Window.EndTs-node.Window.StartTs)*1000)
			}
			if ctx.edgeCount == 0 {
				ctx.edgeCount = 1
			}
			continue
		}
		if !hasCandidateWindow {
			if !foundNearest {
				foundNearest = true
				ctx.nearest = node.Thread
				ctx.window = node.Window
			}
			continue
		}
		overlap := windowOverlapMs(start, end, node.Window.StartTs, node.Window.EndTs)
		distance := windowDistanceMs(start, end, node.Window.StartTs, node.Window.EndTs)
		if overlap > 0 && ctx.relevance == "" && thread.PID == 0 && thread.Comm == "" {
			ctx.relevance = "adjacent"
		}
		if overlap > ctx.overlapMs {
			ctx.overlapMs = overlap
		}
		if !foundNearest || overlap > 0 || distance < bestDistance {
			foundNearest = true
			bestDistance = distance
			ctx.nearest = node.Thread
			ctx.window = node.Window
		}
	}
	if ctx.relevance == "" {
		ctx.relevance = "background"
		ctx.overlapMs = 0
	}
	return ctx
}

func causalityFromChainRelevance(relevance string) string {
	switch relevance {
	case "on_chain":
		return "on_wakeup_chain"
	case "adjacent":
		return "adjacent_to_wakeup_chain"
	case "background":
		return "background"
	default:
		return ""
	}
}

func chainRelevanceFromCausality(causality string) string {
	switch causality {
	case "on_wakeup_chain":
		return "on_chain"
	case "adjacent_to_wakeup_chain":
		return "adjacent"
	case "background":
		return "background"
	default:
		return ""
	}
}

func chainRelevanceRank(relevance string) int {
	switch relevance {
	case "on_chain":
		return 0
	case "adjacent":
		return 1
	case "background":
		return 2
	default:
		return 3
	}
}

func windowOverlapMs(aStart, aEnd, bStart, bEnd float64) float64 {
	if aEnd <= aStart || bEnd <= bStart {
		return 0
	}
	start := maxFloat(aStart, bStart)
	end := minFloat(aEnd, bEnd)
	if end <= start {
		return 0
	}
	return (end - start) * 1000
}

func windowDistanceMs(aStart, aEnd, bStart, bEnd float64) float64 {
	if aEnd <= aStart || bEnd <= bStart {
		return 0
	}
	if aEnd < bStart {
		return (bStart - aEnd) * 1000
	}
	if bEnd < aStart {
		return (aStart - bEnd) * 1000
	}
	return 0
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func backgroundImpactMs(q Query, impact float64, hasCausalChain, onChain bool) float64 {
	if impact <= 0 || !hasCausalChain || onChain {
		return impact
	}
	windowMs := (q.TimeEnd - q.TimeStart) * 1000
	if windowMs <= 0 {
		return impact
	}
	capMs := windowMs * 0.35
	if capMs < 0.1 {
		capMs = 0.1
	}
	if impact > capMs {
		return capMs
	}
	return impact
}

func rootCauseTypeWeight(typ string) float64 {
	switch typ {
	case "io_wait", "d_state_or_io_wait", "binder_wait":
		return 1.25
	case "priority_inversion_candidate", "priority_inversion_runnable_wait":
		return 1.35
	case "io_pressure":
		return 1.12
	case "io_burst_episode":
		return 1.16
	case "block_io_by_inode":
		return 1.08
	case "file_io_hot_inode":
		return 1.06
	case "page_cache_churn":
		return 0.86
	case "runnable_wait", "cpu_pressure", "scheduler_latency":
		return 1.15
	case "fragmented_d_state_or_io_wait":
		return 1.18
	case "fragmented_runnable_wait":
		return 1.18
	case "fragmented_sleep_wait", "state_churn":
		return 1.05
	case "compute_supply", "low_frequency":
		return 0.95
	case "cpu_affinity_or_cpuset":
		return 0.92
	case "running", "fragmented_running":
		return 1.0
	case "jit_compile", "class_verification", "shader_compile", "runtime_compile":
		return 1.02
	case "cpu_frequency_limit":
		return 0.7
	case "trace_span":
		return 0.9
	case "irq_burst":
		return 0.75
	case "irq_activity":
		return 0.70
	case "workqueue_activity":
		return 0.80
	case "supply_pressure":
		return 0.72
	default:
		return 0.8
	}
}

func fileIOAdvisoryImpactMs(file FileIOSummary) float64 {
	return float64(fileIOEffectiveEventCount(file))*0.25 + float64(file.Bytes)/(1024*1024)*2
}

func fileIOEffectiveEventCount(file FileIOSummary) int {
	if file.Count >= file.CompletionCount {
		return file.Count
	}
	return file.CompletionCount
}

func stateChurnRankImpactMs(churn ThreadStateChurnSummary) float64 {
	if churn.TotalMs <= churn.DominantImpactMs {
		return churn.DominantImpactMs
	}
	return churn.DominantImpactMs + (churn.TotalMs-churn.DominantImpactMs)*0.5
}

func stateChurnRootCauseType(state string) string {
	switch state {
	case string(StateRunnable):
		return "fragmented_runnable_wait"
	case string(StateSSleep):
		return "fragmented_sleep_wait"
	case string(StateDSleep), string(StateIOWait):
		return "fragmented_d_state_or_io_wait"
	case string(StateRunning):
		return "fragmented_running"
	default:
		return "state_churn"
	}
}

func ioBurstDominantState(episode IOBurstEpisodeSummary) string {
	if episode.IOWaitMs >= episode.DStateMs && episode.IOWaitMs > 0 {
		return string(StateIOWait)
	}
	if episode.DStateMs > 0 {
		return string(StateDSleep)
	}
	return ""
}

func computeSupplyDominantState(item ComputeSupplySummary) string {
	switch strings.TrimSpace(item.State) {
	case string(StateRunning):
		return string(StateRunning)
	case string(StateRunnable):
		return string(StateRunnable)
	default:
		return strings.TrimSpace(item.State)
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

func assignRootCauseRanksAndTiers(items []RootCauseRankItem) {
	for i := range items {
		items[i].Rank = i + 1
		tier := rootCauseTier(i)
		if rootCauseShouldBeCoPrimary(items[i]) {
			tier = "primary"
		}
		items[i].Tier = tier
	}
}

func rootCauseShouldBeCoPrimary(item RootCauseRankItem) bool {
	if !rootCauseItemIsOnChain(item) || item.ImpactMs <= 0 {
		return false
	}
	switch item.Type {
	case "runnable_wait", "scheduler_latency", "priority_inversion_runnable_wait", "fragmented_runnable_wait",
		"running", "fragmented_running",
		"compute_supply", "low_frequency", "cpu_affinity_or_cpuset",
		"jit_compile", "class_verification", "shader_compile", "runtime_compile",
		"io_wait", "d_state_or_io_wait", "io_latency", "io_burst_episode", "block_io_by_inode", "file_io_hot_inode", "fragmented_d_state_or_io_wait":
		return true
	case "priority_inversion_candidate":
		return rootCauseItemHasDStateOrIO(item) || rootCauseItemHasRunnableOrRunning(item)
	default:
		return false
	}
}

func rootCauseItemIsOnChain(item RootCauseRankItem) bool {
	if strings.TrimSpace(item.ChainRelevance) == "on_chain" {
		return true
	}
	return strings.TrimSpace(item.Causality) == "on_wakeup_chain"
}

func rootCauseItemHasDStateOrIO(item RootCauseRankItem) bool {
	if item.DStateMs > 0 || item.IOWaitMs > 0 {
		return true
	}
	switch strings.TrimSpace(item.DominantState) {
	case string(StateDSleep), string(StateIOWait):
		return true
	default:
		return false
	}
}

func rootCauseItemHasRunnableOrRunning(item RootCauseRankItem) bool {
	if item.RunnableMs > 0 || item.RunningMs > 0 {
		return true
	}
	switch strings.TrimSpace(item.DominantState) {
	case string(StateRunnable), string(StateRunning):
		return true
	default:
		return false
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

func BuildFrameRootCauseBundle(idx *Index, q Query) FrameRootCauseBundle {
	q = normalizeQuery(idx, q)
	stats := ComputeWindowStats(idx, q)
	frame := BuildFrameTimeline(idx, q)
	var chain ChainResult
	if q.PID > 0 || q.Thread != "" || q.ThreadInput != "" {
		chain = BuildWakeupChain(idx, q)
	}
	rank := buildRootCauseRankFrom(q, chain, stats)
	latency := buildSchedulerLatencyStatsFromStats(idx, q, stats)
	rank = enrichRootCauseRankWithScheduler(q, rank, latency, stats, chain)
	rank = attachPerfContextToRootCauseRank(idx, q, rank, stats)
	var chainPtr *ChainResult
	if len(chain.Nodes) > 0 || len(chain.Edges) > 0 || len(chain.CausalImpacts) > 0 || chain.Target.PID > 0 || chain.Target.Comm != "" {
		chainPtr = &chain
	}
	blocking := buildCriticalBlockingCallsFromStats(idx, q, stats, chainPtr)
	perfContexts := buildFramePerfContexts(idx, q, stats, chainPtr, blocking, firstNonEmptyThread(chain.Target, resolveThread(idx, q)))
	bundle := FrameRootCauseBundle{
		Target:                firstNonEmptyThread(chain.Target, resolveThread(idx, q)),
		Window:                TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
		WakeupChain:           chainPtr,
		FrameTimeline:         &frame,
		RootCauseRank:         &rank,
		CriticalBlocking:      &blocking,
		PerfSamples:           perfContexts.PerfSamples,
		TargetRunningPerf:     perfContexts.TargetRunningPerf,
		OnChainPerf:           perfContexts.OnChainPerf,
		BinderPeerPerf:        perfContexts.BinderPeerPerf,
		SameCPUCompetitorPerf: perfContexts.SameCPUCompetitorPerf,
		IOBurstEpisodes:       stats.IOBurstEpisodes,
		BlockIOByInode:        stats.BlockIOByInode,
		IRQActivity:           stats.IRQActivity,
		SoftIRQActivity:       stats.SoftIRQActivity,
		WorkqueueActivity:     stats.WorkqueueActivity,
		SupplyPressureSummary: stats.SupplyPressureSummary,
		TraceMarkCategories:   stats.TraceMarkCategories,
		AsyncFileWork:         stats.AsyncFileWork,
	}
	if chainPtr != nil {
		bundle.IOBurstEpisodes = enrichIOBurstEpisodesWithChainContext(*chainPtr, bundle.IOBurstEpisodes)
	}
	bundle.Caveats = append(bundle.Caveats, stats.Caveats...)
	bundle.Caveats = append(bundle.Caveats, frame.Caveats...)
	bundle.Caveats = append(bundle.Caveats, rank.Caveats...)
	bundle.Caveats = append(bundle.Caveats, blocking.Caveats...)
	return bundle
}

func enrichIOBurstEpisodesWithChainContext(chain ChainResult, items []IOBurstEpisodeSummary) []IOBurstEpisodeSummary {
	for i := range items {
		ctx := chainContextForCandidate(chain, items[i].Thread, items[i].StartTs, items[i].EndTs)
		items[i].ChainRelevance = ctx.relevance
		items[i].OverlapMs = ctx.overlapMs
		items[i].NearestChainThread = ctx.nearest
		items[i].NearestChainWindow = ctx.window
	}
	sort.SliceStable(items, func(i, j int) bool {
		ri := chainRelevanceRank(items[i].ChainRelevance)
		rj := chainRelevanceRank(items[j].ChainRelevance)
		if ri != rj {
			return ri < rj
		}
		scoreI := items[i].DurationMs*maxFloat(items[i].Confidence, 0.5) + items[i].BlockMaxLatencyMs + items[i].StorageMaxLatencyMs
		scoreJ := items[j].DurationMs*maxFloat(items[j].Confidence, 0.5) + items[j].BlockMaxLatencyMs + items[j].StorageMaxLatencyMs
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return items[i].LineStart < items[j].LineStart
	})
	return items
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
	frame := BuildFramePipeline(idx, q)
	return buildFrameTimelineFromPipeline(q, frame)
}

func buildFrameTimelineFromPipeline(q Query, frame FramePipelineResult) FrameTimelineResult {
	q = normalizeQuery(nil, q)
	res := FrameTimelineResult{Window: frame.Window}
	if res.Window.StartTs == 0 && res.Window.EndTs == 0 {
		res.Window = TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}
	}
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
			Thread:        item.Thread,
			Name:          item.Name,
			Category:      traceSpanCategory(item.Name),
			Subcategory:   traceSpanSubcategory(item.Name),
			SemanticClass: traceSpanSemanticClass(item.Name),
			StartTs:       item.StartTs,
			EndTs:         item.EndTs,
			DurationMs:    item.DurationMs,
			StartLine:     item.StartLine,
			EndLine:       item.EndLine,
		})
	}
	return out
}

func frameTimelineSpans(frame FrameTimelineResult) []TraceSpanSummary {
	out := make([]TraceSpanSummary, 0, len(frame.Items))
	for _, item := range frame.Items {
		out = append(out, TraceSpanSummary{
			Thread:        item.Thread,
			Name:          item.Name,
			Category:      traceSpanCategory(item.Name),
			Subcategory:   traceSpanSubcategory(item.Name),
			SemanticClass: traceSpanSemanticClass(item.Name),
			StartTs:       item.StartTs,
			EndTs:         item.EndTs,
			DurationMs:    item.DurationMs,
			StartLine:     item.StartLine,
			EndLine:       item.EndLine,
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

type traceSpanSemanticWork struct {
	RootCauseType string
	Category      string
	Subcategory   string
	SemanticClass string
	Label         string
	Confidence    float64
}

func traceSpanSemanticClass(name string) string {
	work, ok := traceSpanSemanticWorkClass(name)
	if !ok {
		return ""
	}
	return work.SemanticClass
}

func traceSpanSemanticWorkClass(name string) (traceSpanSemanticWork, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return traceSpanSemanticWork{}, false
	}
	tokens := traceSpanNameTokenSet(lower)
	switch {
	case traceSpanLooksLikeJITCompile(lower, tokens):
		return traceSpanSemanticWork{
			RootCauseType: "jit_compile",
			Category:      "runtime_compile",
			Subcategory:   "jit",
			SemanticClass: "jit_compile",
			Label:         "JIT compilation",
			Confidence:    0.82,
		}, true
	case traceSpanLooksLikeClassVerification(lower, tokens):
		return traceSpanSemanticWork{
			RootCauseType: "class_verification",
			Category:      "runtime_verification",
			Subcategory:   "class_verification",
			SemanticClass: "class_verification",
			Label:         "class verification",
			Confidence:    0.82,
		}, true
	case traceSpanLooksLikeShaderCompile(lower, tokens):
		return traceSpanSemanticWork{
			RootCauseType: "shader_compile",
			Category:      "shader_compile",
			Subcategory:   "shader",
			SemanticClass: "shader_compile",
			Label:         "shader compilation",
			Confidence:    0.80,
		}, true
	case traceSpanLooksLikeRuntimeCompile(lower, tokens):
		return traceSpanSemanticWork{
			RootCauseType: "runtime_compile",
			Category:      "runtime_compile",
			Subcategory:   "compile",
			SemanticClass: "runtime_compile",
			Label:         "runtime compilation",
			Confidence:    0.78,
		}, true
	default:
		return traceSpanSemanticWork{}, false
	}
}

func traceSpanNameTokenSet(lower string) map[string]bool {
	tokens := map[string]bool{}
	for _, token := range strings.FieldsFunc(lower, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	}) {
		if token != "" {
			tokens[token] = true
		}
	}
	return tokens
}

func traceSpanLooksLikeJITCompile(lower string, tokens map[string]bool) bool {
	return tokens["jit"] ||
		strings.Contains(lower, "jitcompile") ||
		strings.Contains(lower, "jit_compile") ||
		strings.Contains(lower, "jit compile") ||
		strings.Contains(lower, "just-in-time")
}

func traceSpanLooksLikeClassVerification(lower string, tokens map[string]bool) bool {
	return strings.Contains(lower, "verifyclass") ||
		strings.Contains(lower, "verify_class") ||
		strings.Contains(lower, "classverification") ||
		strings.Contains(lower, "class_verification") ||
		strings.Contains(lower, "class verifier") ||
		strings.Contains(lower, "classverifier") ||
		((tokens["verify"] || tokens["verifier"] || tokens["verification"]) && tokens["class"])
}

func traceSpanLooksLikeShaderCompile(lower string, tokens map[string]bool) bool {
	if !strings.Contains(lower, "shader") {
		return false
	}
	return strings.Contains(lower, "compile") ||
		tokens["compilation"] ||
		tokens["compiler"] ||
		tokens["pipeline"] ||
		tokens["program"] ||
		tokens["link"] ||
		tokens["warmup"]
}

func traceSpanLooksLikeRuntimeCompile(lower string, tokens map[string]bool) bool {
	compileLike := strings.Contains(lower, "compile") ||
		tokens["compiler"] ||
		tokens["compilation"] ||
		strings.Contains(lower, "dex2oat") ||
		tokens["aot"]
	if !compileLike {
		return false
	}
	return tokens["ark"] ||
		tokens["arkts"] ||
		strings.Contains(lower, "arkts") ||
		tokens["art"] ||
		tokens["dex"] ||
		strings.Contains(lower, "dex2oat") ||
		tokens["bytecode"] ||
		tokens["wasm"] ||
		tokens["js"] ||
		tokens["runtime"]
}

func traceSpanCategory(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if work, ok := traceSpanSemanticWorkClass(name); ok {
		return work.Category
	}
	switch {
	case lower == "":
		return ""
	case strings.Contains(lower, "fence") || strings.Contains(lower, "dma_fence"):
		return "render_fence"
	case strings.Contains(lower, "bufferqueue") || strings.Contains(lower, "buffer_queue") || strings.Contains(lower, "queuebuffer") || strings.Contains(lower, "dequeuebuffer"):
		return "buffer_queue"
	case strings.Contains(lower, "audio"):
		return "audio"
	case strings.Contains(lower, "render_service") || strings.Contains(lower, "rsunirender") || strings.Contains(lower, "rs ") || strings.Contains(lower, "h:render"):
		return "frame_render"
	case isFrameLikeSpan(lower):
		return "frame_render"
	case strings.Contains(lower, "async") && (strings.Contains(lower, "file") || strings.Contains(lower, "read") || strings.Contains(lower, "write") || strings.Contains(lower, "io")):
		return "async_file"
	case strings.Contains(lower, "file") || strings.Contains(lower, "read") || strings.Contains(lower, "write") || strings.Contains(lower, "io"):
		return "file_io"
	case strings.Contains(lower, "binder"):
		return "binder"
	case strings.Contains(lower, "lock") || strings.Contains(lower, "mutex") || strings.Contains(lower, "futex") || strings.Contains(lower, "wait"):
		return "blocking_sync"
	case strings.Contains(lower, "workqueue") || strings.Contains(lower, "work queue"):
		return "workqueue"
	default:
		return "trace_span"
	}
}

func traceSpanSubcategory(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if work, ok := traceSpanSemanticWorkClass(name); ok {
		return work.Subcategory
	}
	switch {
	case strings.Contains(lower, "expected"):
		return "expected"
	case strings.Contains(lower, "fence"):
		return "fence"
	case strings.Contains(lower, "actual"):
		return "actual"
	case strings.Contains(lower, "jank") || strings.Contains(lower, "deadline"):
		return "jank"
	case strings.Contains(lower, "gpu"):
		return "gpu"
	case strings.Contains(lower, "present") || strings.Contains(lower, "surface") || strings.Contains(lower, "compose"):
		return "composition"
	case strings.Contains(lower, "buffer"):
		return "buffer"
	case strings.Contains(lower, "read"):
		return "read"
	case strings.Contains(lower, "write"):
		return "write"
	default:
		return ""
	}
}

func computeTraceMarkCategories(spans []TraceSpanSummary, max int) []TraceMarkCategory {
	accs := map[string]*TraceMarkCategory{}
	for _, span := range spans {
		category := firstNonEmpty(span.Category, traceSpanCategory(span.Name), "trace_span")
		sub := firstNonEmpty(span.Subcategory, traceSpanSubcategory(span.Name))
		key := category + "/" + sub
		item := accs[key]
		if item == nil {
			item = &TraceMarkCategory{Category: category, Subcategory: sub, LineStart: span.StartLine, LineEnd: span.EndLine}
			accs[key] = item
		}
		item.Count++
		item.TotalMs += span.DurationMs
		if span.DurationMs > item.MaxDurationMs {
			item.MaxDurationMs = span.DurationMs
			item.TopSpan = span.Name
			item.TopThread = span.Thread
		}
		applyLineRange(&item.LineStart, &item.LineEnd, span.StartLine)
		applyLineRange(&item.LineStart, &item.LineEnd, span.EndLine)
	}
	out := make([]TraceMarkCategory, 0, len(accs))
	for _, item := range accs {
		item.Summary = fmt.Sprintf("trace_mark_category category=%s subcategory=%s count=%d total=%.3fms max=%.3fms top_span=%s thread=%s",
			item.Category, item.Subcategory, item.Count, item.TotalMs, item.MaxDurationMs, item.TopSpan, threadLabel(item.TopThread))
		out = append(out, *item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TotalMs != out[j].TotalMs {
			return out[i].TotalMs > out[j].TotalMs
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func computeAsyncFileWorkSummaries(spans []TraceSpanSummary, max int) []AsyncFileWorkSummary {
	var out []AsyncFileWorkSummary
	for _, span := range spans {
		category := firstNonEmpty(span.Category, traceSpanCategory(span.Name))
		if category != "async_file" && category != "file_io" {
			continue
		}
		item := AsyncFileWorkSummary{
			Thread:     span.Thread,
			Name:       span.Name,
			Category:   category,
			StartTs:    span.StartTs,
			EndTs:      span.EndTs,
			DurationMs: span.DurationMs,
			LineStart:  span.StartLine,
			LineEnd:    span.EndLine,
		}
		item.Summary = fmt.Sprintf("async_file_work thread=%s category=%s span=%s duration=%.3fms", threadLabel(item.Thread), item.Category, item.Name, item.DurationMs)
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
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
	stats := ComputeWindowStats(idx, q)
	return buildCriticalBlockingCallsFromStats(idx, q, stats, nil)
}

func buildCriticalBlockingCallsFromStats(idx *Index, q Query, stats WindowStats, cachedChain *ChainResult) CriticalBlockingResult {
	q = normalizeQuery(idx, q)
	res := CriticalBlockingResult{Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}}
	var chainForContext *ChainResult
	if cachedChain != nil {
		chainForContext = cachedChain
	}
	if idx == nil {
		res.Caveats = append(res.Caveats, "trace index is empty")
		return res
	}
	add := func(item CriticalBlockingCandidate) {
		if item.DurationMs <= 0 && item.LineStart == 0 {
			return
		}
		if item.Confidence <= 0 {
			item.Confidence = 0.6
		}
		if item.PeerState == nil {
			item.PeerState = buildCriticalBlockingPeerState(idx, q, item)
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
		var chain ChainResult
		if cachedChain != nil {
			chain = *cachedChain
		} else {
			chain = BuildWakeupChain(idx, q)
		}
		chainForContext = &chain
		for _, wait := range chain.BinderWaits {
			add(CriticalBlockingCandidate{
				Type:              "binder_wait",
				Thread:            wait.Thread,
				Peer:              wait.Peer,
				Flags:             wait.Flags,
				Oneway:            traceBoolPtr(wait.Oneway),
				SyncLike:          traceBoolPtr(wait.SyncLike),
				BlockingCandidate: traceBoolPtr(wait.BlockingCandidate),
				DurationMs:        wait.DurationMs,
				StartTs:           wait.SendTs,
				EndTs:             firstPositiveFloat(wait.WakeupTs, wait.SleepStartTs),
				LineStart:         firstPositive(wait.SendLine, wait.SleepLine),
				LineEnd:           firstPositive(wait.WakeupLine, wait.ReceiveLine, wait.SleepLine),
				Confidence:        wait.Confidence,
				Summary:           wait.Summary,
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
	if chainForContext != nil {
		res.Items = enrichCriticalBlockingWithChainContext(*chainForContext, res.Items)
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

func buildCriticalBlockingPeerState(idx *Index, q Query, item CriticalBlockingCandidate) *ThreadStateBreakdown {
	if idx == nil || item.Peer.PID <= 0 {
		return nil
	}
	start := item.StartTs
	end := item.EndTs
	if start <= 0 {
		start = q.TimeStart
	}
	if end <= start {
		end = q.TimeEnd
	}
	if end <= start {
		return nil
	}
	tq := q
	tq.View = ""
	tq.PID = item.Peer.PID
	tq.Thread = item.Peer.Comm
	tq.ThreadInput = ""
	tq.TimeStart = start
	tq.TimeEnd = end
	tl := ThreadTimeline(idx, tq)
	return summarizeThreadStateBreakdown(tl)
}

func summarizeThreadStateBreakdown(tl TimelineResult) *ThreadStateBreakdown {
	if len(tl.Intervals) == 0 {
		return nil
	}
	out := ThreadStateBreakdown{
		Thread: tl.Thread,
		Window: tl.Window,
	}
	for _, it := range tl.Intervals {
		if it.DurationMs < 0 {
			continue
		}
		out.TotalMs += it.DurationMs
		out.FragmentCount++
		if it.DurationMs > out.MaxSegmentMs {
			out.MaxSegmentMs = it.DurationMs
		}
		if startLine := firstPositive(it.StartLine, it.WakeupLine, it.EndLine); startLine > 0 && (out.LineStart == 0 || startLine < out.LineStart) {
			out.LineStart = startLine
		}
		if endLine := firstPositive(it.EndLine, it.WakeupLine, it.StartLine); endLine > out.LineEnd {
			out.LineEnd = endLine
		}
		switch it.State {
		case StateRunning:
			out.RunningMs += it.DurationMs
		case StateRunnable:
			out.RunnableMs += it.DurationMs
		case StateSSleep:
			out.SleepMs += it.DurationMs
		case StateDSleep:
			out.DStateMs += it.DurationMs
		case StateIOWait:
			out.IOWaitMs += it.DurationMs
		}
	}
	if out.TotalMs <= 0 {
		return nil
	}
	out.DominantState = dominantThreadStateBreakdown(out)
	out.Summary = fmt.Sprintf("%s peer_state dominant_state=%s total=%.3fms running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms fragments=%d max_segment=%.3fms",
		threadLabel(out.Thread), out.DominantState, out.TotalMs, out.RunningMs, out.RunnableMs, out.SleepMs, out.DStateMs, out.IOWaitMs, out.FragmentCount, out.MaxSegmentMs)
	return &out
}

func dominantThreadStateBreakdown(item ThreadStateBreakdown) string {
	type candidate struct {
		state string
		ms    float64
	}
	candidates := []candidate{
		{state: string(StateIOWait), ms: item.IOWaitMs},
		{state: string(StateDSleep), ms: item.DStateMs},
		{state: string(StateRunnable), ms: item.RunnableMs},
		{state: string(StateSSleep), ms: item.SleepMs},
		{state: string(StateRunning), ms: item.RunningMs},
	}
	best := candidates[0]
	for _, cand := range candidates[1:] {
		if cand.ms > best.ms {
			best = cand
		}
	}
	return best.state
}

func enrichCriticalBlockingWithChainContext(chain ChainResult, items []CriticalBlockingCandidate) []CriticalBlockingCandidate {
	for i := range items {
		ctx := chainContextForCandidate(chain, items[i].Thread, items[i].StartTs, items[i].EndTs)
		items[i].ChainRelevance = ctx.relevance
		items[i].OverlapMs = ctx.overlapMs
		items[i].EdgeCount = ctx.edgeCount
		items[i].NearestChainThread = ctx.nearest
		items[i].NearestChainWindow = ctx.window
	}
	return items
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
		res.IncludedViews = []string{"frame_window", "frame_timeline", "frame_flow", "scheduler_latency_stats", "window_stats", "root_cause_rank", "critical_blocking_calls", "frame_root_cause_bundle"}
		res.Summary = "jank recipe: derive frame/render spans, frame timeline/flows, scheduler latency, same-window resources, ranked causes, blocking candidates, and a handoff-safe frame root-cause bundle"
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

func recipeShouldUseDiscoveryOnly(q Query, recipe RecipeResult, explicitWindowOrSelector bool) bool {
	if recipe.Name != "jank" {
		return false
	}
	if explicitWindowOrSelector {
		return false
	}
	return true
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

func expandChain(idx *Index, q Query, thread ThreadRef, start, end float64, depth int, targetBlockedMs float64, visited map[int]bool, res *ChainResult, parentID string) string {
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
	impact := summarizeWakeupCausalImpact(idx, q, thread, tl.Intervals, start, end, depth, targetBlockedMs, res.Target)
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
	if impact.TotalMs > 0 {
		node.Impact = &impact
		if impact.DominantState != "" {
			node.Dominant = ThreadState(impact.DominantState)
			node.DurationMs = impact.DominantImpactMs
			node.EvidenceLine = firstPositive(impact.LineStart, node.EvidenceLine, impact.LineEnd)
			node.Summary = impact.Summary
		}
		res.CausalImpacts = append(res.CausalImpacts, impact)
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
		wakeup, usedTolerance := findWakeupFor(idx, thread, interesting.StartTs, interesting.EndTs)
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
		if usedTolerance {
			res.Caveats = append(res.Caveats, fmt.Sprintf("matched sched_wakeup for %s %.6f outside strict sleep end %.6f by %.3fus boundary tolerance", threadLabel(thread), wakeup.Ts, interesting.EndTs, (wakeup.Ts-interesting.EndTs)*1000000))
		}
		waker := ThreadRef{Comm: wakeup.Comm, PID: wakeup.PID, TGID: wakeup.TGID}
		childID := expandChain(idx, q, waker, interesting.StartTs, wakeup.Ts, depth+1, targetBlockedMs, visited, res, nodeID)
		if childID != "" {
			wakerPrio, wakerClass := threadPriorityNear(idx, q.TraceFlavor, waker, wakeup.Ts)
			wakeePrio := wakeup.WakeePrio
			if wakeePrio <= 0 {
				wakeePrio, _ = threadPriorityNear(idx, q.TraceFlavor, thread, wakeup.Ts)
			}
			wakeeClass := classifyTracePriority(q.TraceFlavor, wakeePrio)
			relation := priorityRelation(q.TraceFlavor, wakeePrio, wakerPrio)
			res.Edges = append(res.Edges, WakeupEdge{
				From:                       childID,
				To:                         nodeID,
				Waker:                      waker,
				Wakee:                      thread,
				WakeupTs:                   wakeup.Ts,
				WakeupLine:                 wakeup.Line,
				LatencyMs:                  (wakeup.Ts - interesting.StartTs) * 1000,
				WakerPriority:              wakerPrio,
				WakerPriorityClass:         wakerClass,
				WakeePriority:              wakeePrio,
				WakeePriorityClass:         wakeeClass,
				PriorityRelation:           relation,
				PriorityInversionCandidate: relation == "lower_priority_waker",
				EvidenceLine:               wakeup.Line,
			})
		}
	case StateRunnable:
		res.RootEvidence = append(res.RootEvidence, rootEvidenceFromCausalImpact(impact, "thread was runnable but not running; inspect CPU pressure and priority context", 0.8))
	case StateDSleep, StateIOWait:
		root := rootEvidenceFromCausalImpact(impact, "thread slept in D state; IO or uninterruptible wait is a root-cause candidate", 0.88)
		if reason := findBlockedReasonFor(idx, thread, interesting.StartTs, interesting.EndTs); reason != nil {
			if reason.IOWait > 0 {
				root.Type = "io_wait"
			}
			root.LineEnd = firstPositive(reason.Line, root.LineEnd)
			root.Summary = fmt.Sprintf("thread slept in D state; sched_blocked_reason iowait=%d caller=%s", reason.IOWait, firstNonEmpty(reason.Reason, reason.FieldText, "unknown"))
		}
		res.RootEvidence = append(res.RootEvidence, root)
	case StateRunning:
		res.RootEvidence = append(res.RootEvidence, rootEvidenceFromCausalImpact(impact, "thread was running in the aligned window; its own CPU work is root-cause evidence", 0.75))
	default:
		res.RootEvidence = append(res.RootEvidence, rootEvidenceFromCausalImpact(impact, "thread state could not be classified from scheduler rows", 0.5))
	}
	return nodeID
}

func summarizeWakeupCausalImpact(idx *Index, q Query, thread ThreadRef, intervals []Interval, start, end float64, depth int, targetBlockedMs float64, target ThreadRef) WakeupCausalImpact {
	item := WakeupCausalImpact{
		Thread:          thread,
		Window:          TimeWindow{StartTs: start, EndTs: end},
		ChainDepth:      depth,
		OnChain:         true,
		TargetBlockedMs: targetBlockedMs,
	}
	var segments []float64
	for _, it := range intervals {
		if it.DurationMs < 0 {
			continue
		}
		item.TotalMs += it.DurationMs
		item.ProjectedTotalMs += it.DurationMs
		actualDuration := intervalActualDurationMs(it)
		item.ActualTotalMs += actualDuration
		extendActualWindow(&item.ActualWindow, it)
		item.FragmentCount++
		segments = append(segments, it.DurationMs)
		if it.DurationMs > item.MaxSegmentMs {
			item.MaxSegmentMs = it.DurationMs
		}
		if item.LineStart == 0 {
			item.LineStart = firstPositive(it.StartLine, it.WakeupLine, it.EndLine)
		}
		item.LineEnd = firstPositive(it.EndLine, it.WakeupLine, item.LineEnd)
		switch it.State {
		case StateRunning:
			item.RunningMs += it.DurationMs
			item.ActualRunningMs += actualDuration
		case StateRunnable:
			item.RunnableMs += it.DurationMs
			item.ActualRunnableMs += actualDuration
		case StateSSleep:
			item.SleepMs += it.DurationMs
			item.ActualSleepMs += actualDuration
		case StateDSleep:
			item.DStateMs += it.DurationMs
			item.ActualDStateMs += actualDuration
		case StateIOWait:
			item.IOWaitMs += it.DurationMs
			item.ActualIOWaitMs += actualDuration
		}
	}
	if item.FragmentCount > 0 {
		item.StateSwitches = item.FragmentCount - 1
	}
	item.P95SegmentMs = percentileFloat64(segments, 0.95)
	item.DominantState, item.DominantImpactMs = dominantCausalImpactState(item)
	item.ProjectedImpactMs = causalImpactBlockingMs(item)
	item.ActualImpactMs = actualCausalImpactBlockingMs(item)
	item.Priority, item.PriorityClass = threadPriorityNear(idx, q.TraceFlavor, thread, (start+end)/2)
	item.TargetPriority, item.TargetPriorityClass = threadPriorityNear(idx, q.TraceFlavor, target, start)
	item.PriorityRelation = dependencyPriorityRelation(q.TraceFlavor, item.TargetPriority, item.Priority, depth)
	item.PriorityInversionCandidate = item.PriorityRelation == "lower_priority_dependency" && causalImpactIsPrioritySensitiveRoot(item)
	item.NextStep = causalImpactNextStep(item)
	item.Summary = renderWakeupCausalImpactSummary(item)
	return item
}

func intervalActualDurationMs(it Interval) float64 {
	if it.ActualDurationMs > 0 {
		return it.ActualDurationMs
	}
	if it.ActualEndTs > it.ActualStartTs {
		return (it.ActualEndTs - it.ActualStartTs) * 1000
	}
	return it.DurationMs
}

func extendActualWindow(window *TimeWindow, it Interval) {
	if window == nil {
		return
	}
	start, end := it.ActualStartTs, it.ActualEndTs
	if start == 0 && end == 0 {
		start, end = it.StartTs, it.EndTs
	}
	if end < start {
		end = start
	}
	if start > 0 && (window.StartTs == 0 || start < window.StartTs) {
		window.StartTs = start
	}
	if end > window.EndTs {
		window.EndTs = end
	}
}

func dominantCausalImpactState(item WakeupCausalImpact) (string, float64) {
	type candidate struct {
		state string
		ms    float64
	}
	candidates := []candidate{
		{state: string(StateIOWait), ms: item.IOWaitMs},
		{state: string(StateDSleep), ms: item.DStateMs},
		{state: string(StateRunnable), ms: item.RunnableMs},
		{state: string(StateSSleep), ms: item.SleepMs},
		{state: string(StateRunning), ms: item.RunningMs},
	}
	best := candidates[0]
	for _, cand := range candidates[1:] {
		if cand.ms > best.ms {
			best = cand
		}
	}
	return best.state, best.ms
}

func causalImpactIsPrioritySensitiveRoot(item WakeupCausalImpact) bool {
	switch item.DominantState {
	case string(StateRunnable), string(StateDSleep), string(StateIOWait):
		return causalImpactBlockingMs(item) > 0
	default:
		return false
	}
}

func causalImpactBlockingMs(item WakeupCausalImpact) float64 {
	if item.DominantState == string(StateDSleep) || item.DominantState == string(StateIOWait) {
		return item.DStateMs + item.IOWaitMs
	}
	return item.DominantImpactMs
}

func actualCausalImpactBlockingMs(item WakeupCausalImpact) float64 {
	if item.DominantState == string(StateDSleep) || item.DominantState == string(StateIOWait) {
		return item.ActualDStateMs + item.ActualIOWaitMs
	}
	switch item.DominantState {
	case string(StateRunning):
		return item.ActualRunningMs
	case string(StateRunnable):
		return item.ActualRunnableMs
	case string(StateSSleep):
		return item.ActualSleepMs
	case string(StateDSleep):
		return item.ActualDStateMs
	case string(StateIOWait):
		return item.ActualIOWaitMs
	default:
		return item.ActualTotalMs
	}
}

func causalImpactNextStep(item WakeupCausalImpact) string {
	base := stateChurnNextStep(item.DominantState)
	if item.PriorityInversionCandidate {
		return "inspect lower-priority dependency scheduling delay and same-window CPU pressure; then " + base
	}
	return base
}

func renderWakeupCausalImpactSummary(item WakeupCausalImpact) string {
	summary := fmt.Sprintf("%s on wakeup chain depth=%d dominant_state=%s impact=%.3fms total=%.3fms projected_impact=%.3fms projected_total=%.3fms actual_impact=%.3fms actual_total=%.3fms actual_window=%.6f..%.6f target_blocked=%.3fms fragments=%d switches=%d max_segment=%.3fms p95_segment=%.3fms totals running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms actual_totals running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms",
		threadLabel(item.Thread), item.ChainDepth, item.DominantState, item.DominantImpactMs, item.TotalMs, item.ProjectedImpactMs, item.ProjectedTotalMs, item.ActualImpactMs, item.ActualTotalMs, item.ActualWindow.StartTs, item.ActualWindow.EndTs, item.TargetBlockedMs, item.FragmentCount, item.StateSwitches, item.MaxSegmentMs, item.P95SegmentMs, item.RunningMs, item.RunnableMs, item.SleepMs, item.DStateMs, item.IOWaitMs, item.ActualRunningMs, item.ActualRunnableMs, item.ActualSleepMs, item.ActualDStateMs, item.ActualIOWaitMs)
	if item.Priority > 0 || item.TargetPriority > 0 {
		summary = fmt.Sprintf("%s priority=%d/%s target_priority=%d/%s relation=%s", summary, item.Priority, item.PriorityClass, item.TargetPriority, item.TargetPriorityClass, item.PriorityRelation)
	}
	if item.PriorityInversionCandidate {
		summary += " priority_inversion_candidate=true"
	}
	if item.NextStep != "" {
		summary += "; next_step=" + item.NextStep
	}
	return summary
}

func rootEvidenceFromCausalImpact(item WakeupCausalImpact, fallback string, confidence float64) RootEvidence {
	typ := causalImpactRootType(item)
	duration := causalImpactBlockingMs(item)
	if duration <= 0 {
		duration = item.DominantImpactMs
	}
	summary := item.Summary
	if summary == "" {
		summary = fallback
	}
	return RootEvidence{
		Type:       typ,
		Thread:     item.Thread,
		DurationMs: duration,
		LineStart:  item.LineStart,
		LineEnd:    item.LineEnd,
		Summary:    summary,
		Confidence: confidence,
	}
}

func causalImpactRootType(item WakeupCausalImpact) string {
	switch item.DominantState {
	case string(StateRunnable):
		if item.PriorityInversionCandidate {
			return "priority_inversion_runnable_wait"
		}
		return "runnable_wait"
	case string(StateDSleep):
		if item.IOWaitMs > 0 {
			return "io_wait"
		}
		return "d_state_or_io_wait"
	case string(StateIOWait):
		return "io_wait"
	case string(StateSSleep):
		return "sleep_wait"
	case string(StateRunning):
		return "running"
	default:
		return "unknown_state"
	}
}

func threadPriorityNear(idx *Index, flavor TraceFlavor, thread ThreadRef, ts float64) (int, string) {
	if idx == nil || (thread.PID <= 0 && thread.Comm == "") {
		return 0, ""
	}
	bestPrio := 0
	bestDist := 0.0
	consider := func(ev Event, pid int, comm string, prio int) {
		if prio <= 0 || !threadMatches(thread, pid, comm) {
			return
		}
		dist := ev.Ts - ts
		if dist < 0 {
			dist = -dist
		}
		if bestPrio == 0 || dist < bestDist {
			bestPrio = prio
			bestDist = dist
		}
	}
	for _, ev := range idx.Events {
		switch ev.Type {
		case EventSchedSwitch:
			consider(ev, ev.PrevPID, ev.PrevComm, ev.PrevPrio)
			consider(ev, ev.NextPID, ev.NextComm, ev.NextPrio)
		case EventSchedWakeup, EventSchedWaking:
			consider(ev, ev.WakeePID, ev.WakeeComm, ev.WakeePrio)
		}
	}
	return bestPrio, classifyTracePriority(flavor, bestPrio)
}

func dependencyPriorityRelation(flavor TraceFlavor, targetPrio, dependencyPrio, depth int) string {
	if depth <= 0 || targetPrio <= 0 || dependencyPrio <= 0 {
		return ""
	}
	switch flavor {
	case TraceFlavorHarmonyHitrace:
		if dependencyPrio < targetPrio {
			return "lower_priority_dependency"
		}
		if dependencyPrio > targetPrio {
			return "higher_priority_dependency"
		}
		return "same_priority_dependency"
	default:
		return "raw_priority_uninterpreted"
	}
}

func priorityRelation(flavor TraceFlavor, wakeePrio, wakerPrio int) string {
	if wakeePrio <= 0 || wakerPrio <= 0 {
		return ""
	}
	switch flavor {
	case TraceFlavorHarmonyHitrace:
		if wakerPrio < wakeePrio {
			return "lower_priority_waker"
		}
		if wakerPrio > wakeePrio {
			return "higher_priority_waker"
		}
		return "same_priority"
	default:
		return "raw_priority_uninterpreted"
	}
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
		if diff := out[i].DurationMs - out[j].DurationMs; diff > 0.050 || diff < -0.050 {
			return out[i].DurationMs > out[j].DurationMs
		}
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

func findWakeupFor(idx *Index, thread ThreadRef, start, end float64) (*Event, bool) {
	var best *Event
	usedTolerance := false
	for i := range idx.Events {
		ev := &idx.Events[i]
		if ev.Type != EventSchedWakeup && ev.Type != EventSchedWaking {
			continue
		}
		if ev.Ts < start {
			continue
		}
		inStrict := ev.Ts <= end
		if !inStrict && ev.Ts > end+wakeupMatchToleranceSec {
			continue
		}
		if threadMatches(thread, ev.WakeePID, ev.WakeeComm) {
			best = ev
			usedTolerance = !inStrict
		}
	}
	return best, usedTolerance
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
	for _, file := range stats.FileIOByInode {
		out = append(out, EvidenceFact{
			Subject:    firstNonEmpty(file.EntryName, "inode="+file.Inode),
			Predicate:  "file_io_by_inode",
			Object:     file.Operation,
			Summary:    file.Summary,
			LineStart:  file.LineStart,
			LineEnd:    file.LineEnd,
			StartTs:    file.StartTs,
			EndTs:      file.EndTs,
			Confidence: 0.74,
		})
		if len(out) >= 27 {
			break
		}
	}
	for _, cache := range stats.PageCacheByInode {
		out = append(out, EvidenceFact{
			Subject:    "inode=" + cache.Inode,
			Predicate:  "page_cache_by_inode",
			Object:     cache.Dev,
			Summary:    cache.Summary,
			LineStart:  cache.LineStart,
			LineEnd:    cache.LineEnd,
			StartTs:    cache.StartTs,
			EndTs:      cache.EndTs,
			Confidence: 0.70,
		})
		if len(out) >= 30 {
			break
		}
	}
	for _, storage := range stats.StorageLatencyByLayer {
		out = append(out, EvidenceFact{
			Subject:    storage.Layer,
			Predicate:  "storage_latency_by_layer",
			Object:     storage.Event,
			Summary:    storage.Summary,
			LineStart:  storage.LineStart,
			LineEnd:    storage.LineEnd,
			StartTs:    storage.StartTs,
			EndTs:      storage.EndTs,
			Confidence: 0.72,
		})
		if len(out) >= 33 {
			break
		}
	}
	if stats.IOPressureSummary != nil {
		out = append(out, EvidenceFact{
			Subject:    "io_pressure",
			Predicate:  stats.IOPressureSummary.Signal,
			Object:     stats.IOPressureSummary.TopInode,
			Summary:    stats.IOPressureSummary.Summary,
			LineStart:  stats.IOPressureSummary.LineStart,
			LineEnd:    stats.IOPressureSummary.LineEnd,
			Confidence: 0.70,
		})
	}
	for _, episode := range stats.IOBurstEpisodes {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(episode.Thread),
			Predicate:  "io_burst_episode",
			Object:     episode.TopInode,
			Summary:    episode.Summary,
			LineStart:  episode.LineStart,
			LineEnd:    episode.LineEnd,
			StartTs:    episode.StartTs,
			EndTs:      episode.EndTs,
			Confidence: episode.Confidence,
		})
		if len(out) >= 36 {
			break
		}
	}
	for _, inode := range stats.BlockIOByInode {
		out = append(out, EvidenceFact{
			Subject:    firstNonEmpty(inode.EntryName, "inode="+inode.Inode),
			Predicate:  "block_io_by_inode",
			Object:     inode.BlockDev,
			Summary:    inode.Summary,
			LineStart:  inode.LineStart,
			LineEnd:    inode.LineEnd,
			Confidence: inode.Confidence,
		})
		if len(out) >= 39 {
			break
		}
	}
	for _, activity := range stats.IRQActivity {
		out = append(out, EvidenceFact{
			Subject:    fmt.Sprintf("cpu=%d", activity.CPU),
			Predicate:  "irq_activity",
			Object:     activity.Name,
			Summary:    activity.Summary,
			LineStart:  activity.LineStart,
			LineEnd:    activity.LineEnd,
			StartTs:    activity.StartTs,
			EndTs:      activity.EndTs,
			Confidence: 0.62,
		})
		if len(out) >= 42 {
			break
		}
	}
	for _, work := range stats.WorkqueueActivity {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(work.Thread),
			Predicate:  "workqueue_activity",
			Object:     firstNonEmpty(work.Function, work.Work),
			Summary:    work.Summary,
			LineStart:  work.LineStart,
			LineEnd:    work.LineEnd,
			StartTs:    work.StartTs,
			EndTs:      work.EndTs,
			Confidence: 0.64,
		})
		if len(out) >= 45 {
			break
		}
	}
	if stats.SupplyPressureSummary != nil {
		out = append(out, EvidenceFact{
			Subject:    "supply_pressure",
			Predicate:  stats.SupplyPressureSummary.Signal,
			Summary:    stats.SupplyPressureSummary.Summary,
			LineStart:  stats.SupplyPressureSummary.LineStart,
			LineEnd:    stats.SupplyPressureSummary.LineEnd,
			Confidence: 0.62,
		})
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
	if stats.PerfSamples != nil {
		for _, hot := range stats.PerfSamples.TopSymbols {
			out = append(out, EvidenceFact{
				Subject:    firstNonEmpty(hot.Symbol, hot.DSO, hot.Event, "perf_sample"),
				Predicate:  "perf_sample_top_symbol",
				Object:     hot.DSO,
				Summary:    fmt.Sprintf("perf samples: symbol=%s dso=%s event=%s weight_unit=%s sample_weight=%d samples=%d percent=%.2f%%", firstNonEmpty(hot.Symbol, "unknown"), firstNonEmpty(hot.DSO, "unknown"), firstNonEmpty(hot.Event, "unknown"), firstNonEmpty(hot.WeightUnit, "unknown"), hot.Period, hot.SampleCount, hot.Percent),
				LineStart:  hot.LineStart,
				LineEnd:    hot.LineEnd,
				Confidence: 0.72,
			})
			if len(out) >= 42 {
				break
			}
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
	for _, churn := range stats.StateChurn {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(churn.Thread),
			Predicate:  "state_churn",
			Object:     churn.DominantState,
			Summary:    churn.Summary,
			LineStart:  churn.LineStart,
			LineEnd:    churn.LineEnd,
			Confidence: churn.Confidence,
		})
		if len(out) >= 44 {
			break
		}
	}
	return out
}

func evidenceFromPerfContext(ctx *PerfContext) []EvidenceFact {
	if ctx == nil {
		return nil
	}
	var out []EvidenceFact
	for _, hot := range ctx.TopSymbols {
		out = append(out, EvidenceFact{
			Subject:    firstNonEmpty(hot.Symbol, hot.DSO, hot.Event, "perf_sample"),
			Predicate:  "perf_sample_top_symbol",
			Object:     hot.DSO,
			Summary:    fmt.Sprintf("perf samples: symbol=%s dso=%s event=%s weight_unit=%s sample_weight=%d samples=%d percent=%.2f%%", firstNonEmpty(hot.Symbol, "unknown"), firstNonEmpty(hot.DSO, "unknown"), firstNonEmpty(hot.Event, "unknown"), firstNonEmpty(hot.WeightUnit, "unknown"), hot.Period, hot.SampleCount, hot.Percent),
			LineStart:  hot.LineStart,
			LineEnd:    hot.LineEnd,
			Confidence: 0.72,
		})
		if len(out) >= 12 {
			break
		}
	}
	for _, hot := range ctx.TopCallchains {
		out = append(out, EvidenceFact{
			Subject:    firstNonEmpty(hot.Symbol, hot.Callchain, "perf_callchain"),
			Predicate:  "perf_sample_top_callchain",
			Object:     hot.Callchain,
			Summary:    fmt.Sprintf("perf callchain: %s weight_unit=%s sample_weight=%d samples=%d percent=%.2f%%", firstNonEmpty(hot.Callchain, "unknown"), firstNonEmpty(hot.WeightUnit, "unknown"), hot.Period, hot.SampleCount, hot.Percent),
			LineStart:  hot.LineStart,
			LineEnd:    hot.LineEnd,
			Confidence: 0.68,
		})
		if len(out) >= 16 {
			break
		}
	}
	return out
}

func evidenceFromPerfTimeline(timeline PerfTimelineResult) []EvidenceFact {
	var out []EvidenceFact
	for _, bucket := range timeline.Buckets {
		out = append(out, EvidenceFact{
			Subject:    firstNonEmpty(bucket.TopSymbol, bucket.TopDSO, bucket.TopEvent, "perf_timeline"),
			Predicate:  "perf_timeline_bucket",
			Object:     bucket.TopDSO,
			Summary:    fmt.Sprintf("perf timeline %.6f..%.6f sample_weight=%d samples=%d top_symbol=%s top_dso=%s event=%s", bucket.StartTs, bucket.EndTs, bucket.Period, bucket.SampleCount, firstNonEmpty(bucket.TopSymbol, "unknown"), firstNonEmpty(bucket.TopDSO, "unknown"), firstNonEmpty(bucket.TopEvent, "unknown")),
			LineStart:  bucket.LineStart,
			LineEnd:    bucket.LineEnd,
			StartTs:    bucket.StartTs,
			EndTs:      bucket.EndTs,
			Confidence: 0.68,
		})
		if len(out) >= 16 {
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
		summary := item.Summary
		if perf := rootCausePerfSummary(item.PerfContext); perf != "" {
			summary = fmt.Sprintf("%s; %s", summary, perf)
		}
		if perfRoles := rootCausePerfRoleSummary(item.PerfContexts, 3); perfRoles != "" {
			summary = fmt.Sprintf("%s; perf_role_contexts=%s", summary, perfRoles)
		}
		out = append(out, EvidenceFact{
			Subject:    threadLabel(item.Thread),
			Predicate:  "root_cause_" + item.Tier,
			Object:     item.Type,
			Summary:    fmt.Sprintf("%s cause #%d (%s): %s", item.Tier, item.Rank, item.Type, summary),
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

func rootCausePerfSummary(ctx *PerfContext) string {
	if ctx == nil || ctx.SampleCount == 0 {
		return ""
	}
	parts := []string{fmt.Sprintf("perf_samples=%d", ctx.SampleCount), fmt.Sprintf("sample_weight=%d", ctx.TotalPeriod)}
	if len(ctx.TopSymbols) > 0 {
		hot := ctx.TopSymbols[0]
		parts = append(parts, "top_symbol="+firstNonEmpty(hot.Symbol, "unknown"))
		if hot.DSO != "" {
			parts = append(parts, "dso="+hot.DSO)
		}
		parts = append(parts, fmt.Sprintf("top_sample_weight=%d", hot.Period))
	}
	if quality := perfQualitySummaryCompact(ctx.Quality); quality != "" {
		parts = append(parts, "perf_quality="+quality)
	}
	return strings.Join(parts, " ")
}

func rootCausePerfRoleSummary(contexts []RootCausePerfRoleContext, max int) string {
	if len(contexts) == 0 {
		return ""
	}
	if max <= 0 || max > len(contexts) {
		max = len(contexts)
	}
	parts := make([]string, 0, max)
	for _, role := range contexts {
		if len(parts) >= max || role.PerfContext == nil || role.PerfContext.SampleCount == 0 {
			continue
		}
		fields := []string{role.Role, fmt.Sprintf("samples=%d", role.PerfContext.SampleCount), fmt.Sprintf("sample_weight=%d", role.PerfContext.TotalPeriod)}
		if label := threadLabel(role.Thread); label != "" {
			fields = append(fields, "thread="+label)
		}
		if role.CPU >= 0 {
			fields = append(fields, fmt.Sprintf("cpu=%d", role.CPU))
		}
		if len(role.PerfContext.TopSymbols) > 0 {
			hot := role.PerfContext.TopSymbols[0]
			fields = append(fields, "top_symbol="+firstNonEmpty(hot.Symbol, "unknown"))
			if hot.DSO != "" {
				fields = append(fields, "dso="+hot.DSO)
			}
		}
		if quality := perfQualitySummaryCompact(role.PerfContext.Quality); quality != "" {
			fields = append(fields, "quality="+quality)
		}
		parts = append(parts, strings.Join(fields, " "))
	}
	return strings.Join(parts, " | ")
}

func perfQualitySummaryCompact(q *PerfQualitySummary) string {
	if q == nil {
		return ""
	}
	parts := []string{
		fmt.Sprintf("cpu_known=%d", q.CPUKnownCount),
		fmt.Sprintf("cpu_unknown=%d", q.CPUUnknownCount),
	}
	if source := perfQualityTopValue(q.Sources); source != "" {
		parts = append(parts, "source="+source)
	}
	if status := perfQualityTopValue(q.SymbolizationStatuses); status != "" {
		parts = append(parts, "symbolization="+status)
	}
	if sampleKind := perfQualityTopValue(q.SampleKinds); sampleKind != "" {
		parts = append(parts, "sample_kind="+sampleKind)
	}
	if unit := perfQualityTopValue(q.WeightUnits); unit != "" {
		parts = append(parts, "weight_unit="+unit)
	}
	if clock := perfQualityTopValue(q.Clocks); clock != "" {
		parts = append(parts, "clock="+clock)
	}
	if confidence := perfQualityTopValue(q.ClockConfidences); confidence != "" {
		parts = append(parts, "clock_confidence="+confidence)
	}
	if callchain := perfQualityTopValue(q.CallchainStatuses); callchain != "" {
		parts = append(parts, "callchain_status="+callchain)
	}
	return strings.Join(parts, ",")
}

func perfQualityTopValue(values []PerfValueCount) string {
	if len(values) == 0 {
		return ""
	}
	return values[0].Value
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
		if edge.Flags != "" {
			summary = fmt.Sprintf("%s flags=%s oneway=%t sync_like=%t blocking_candidate=%t", summary, edge.Flags, edge.Oneway, edge.SyncLike, edge.BlockingCandidate)
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
		switch {
		case len(idx.Events) == 0 && idx.Windowed:
			out = append(out, "no ftrace rows were parsed inside the selected bounded index window; ftrace-compatible text is supported, so verify time_start/time_end/line_start/line_end and timestamp units before concluding parser incompatibility")
		case len(idx.Events) == 0 && idx.ScannedLineCount > 0 && idx.UnparsedLines >= idx.ScannedLineCount:
			out = append(out, "no ftrace-compatible timestamped rows were parsed from the scanned lines; the input may be non-ftrace text, compressed/binary, converted incorrectly, or need a converter/parser adapter")
		case len(idx.Events) == 0:
			out = append(out, "trace index contains zero parsed rows; verify that the file contains ftrace-compatible timestamped text rows or pass a converted systrace/ftrace text artifact")
		default:
			out = append(out, "ftrace rows were parsed, but none mapped to known scheduler/resource event families; event_search can still inspect raw event labels, while structured root-cause views may need event-family support")
		}
	}
	if idx != nil {
		out = append(out, idx.Caveats...)
	}
	if idx != nil && idx.Windowed {
		out = append(out, fmt.Sprintf("windowed_index_parse=true; parsed a bounded trace slice before running the view (time %.6f..%.6f seconds, lines %d..%d). If the answer needs state far before this window, rerun with a wider time/line window or omit the window to build the full index.", idx.IndexTimeStart, idx.IndexTimeEnd, idx.IndexLineStart, idx.IndexLineEnd))
	}
	if selector := threadSelectorSummary(firstNonEmpty(q.ThreadInput, q.Thread)); selector != "" {
		if q.ThreadPIDInferred {
			out = append(out, "thread selector normalized from model/customer text: "+selector+"; pid-bearing scheduler fields are used for matching")
		}
	}
	if res.View == "event_search" && len(res.Events) == 0 {
		out = append(out, "matched_events=0 for the selected filters; this is not absence proof if the thread label, time window, event types, or line window are too narrow")
		if pattern := strings.TrimSpace(q.Pattern); pattern != "" {
			out = append(out, fmt.Sprintf("pattern_no_match_hint=pattern %q is a literal substring, not a regex; try one shorter exact frame id/span label/marker token/symbol/DSO/callchain/source/symbolization_status/callchain_status/clock_confidence/cpu_known fragment, add event_types=[\"trace_mark\"] for B/E/C/S/F marker rows or event_types=[\"perf_sample\"] for CPU sample rows, or remove over-narrow pid/thread/time filters before falling back to grep/read_file; for B/E spans, do not search E|<pid>|<span_name> because end rows are unnamed E|<pid> or bare E on the same ftrace thread stack", pattern))
			if len(q.EventTypes) == 0 {
				out = append(out, fmt.Sprintf("next_pattern_call_hint=try trace_query(view=\"event_search\", pattern=%q, event_types=[\"trace_mark\"], time_start=%.6f, time_end=%.6f, limit=40), trace_query(view=\"event_search\", pattern=%q, event_types=[\"perf_sample\"], time_start=%.6f, time_end=%.6f, limit=40), or trace_query(view=\"span_window\", span_name=\"<span label>\", line_start=<line>, line_end=<line>) after selecting a line window", pattern, q.TimeStart, q.TimeEnd, pattern, q.TimeStart, q.TimeEnd))
			}
		}
		if q.PID > 0 {
			out = append(out, fmt.Sprintf("next_call_hint=try trace_query(view=\"thread_timeline\", pid=%d, time_start=%.6f, time_end=%.6f) or trace_query(view=\"wakeup_chain\", pid=%d, time_start=%.6f, time_end=%.6f)", q.PID, q.TimeStart, q.TimeEnd, q.PID, q.TimeStart, q.TimeEnd))
		} else if strings.TrimSpace(q.Thread) != "" {
			out = append(out, fmt.Sprintf("next_call_hint=try trace_query(view=\"event_search\", thread=%q, time_start=%.6f, time_end=%.6f, event_types=[\"sched_switch\",\"sched_wakeup\"]) or use pid if visible in the trace row", q.Thread, q.TimeStart, q.TimeEnd))
		}
	}
	if res.View == "event_search" && q.Limit > 0 && len(res.Events) >= q.Limit {
		out = append(out, fmt.Sprintf("event_search_limit_reached=true; returned rows are the first %d chronological matches only, not an exhaustive result set; do not infer that a frame id/span label is absent from omitted rows", q.Limit))
		if strings.TrimSpace(q.Pattern) != "" {
			out = append(out, "event_search_exact_token_hint=for a requested frame id, jank id, span id, inode, or timestamp, rerun event_search or frame_window/span_window with that exact literal token before making any absence claim")
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
	if (res.View == "wakeup_chain" || res.View == "root_cause_rank" || res.View == "scheduler_latency_stats" || res.View == "trace_perf_bundle" || res.View == "recipe" || res.View == "evidence_pack") && (q.PID > 0 || q.Thread != "" || q.ThreadInput != "") {
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
				if wakeup, _ := findWakeupFor(idx, target, it.StartTs, it.EndTs); wakeup == nil {
					out = append(out, fmt.Sprintf("trace completeness: %s has %.3fms sleep interval lines=%d-%d without matching sched_wakeup/sched_waking in the selected window", threadLabel(target), it.DurationMs, it.StartLine, it.EndLine))
					break
				}
			}
		}
	}
	if q.TimeStart > 0 && (res.WindowStats != nil || res.SchedulerLatency != nil || res.RootCauseRank != nil || res.View == "recipe" || res.View == "evidence_pack" || res.View == "trace_perf_bundle") {
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

func appendUniqueInts(dst []int, values ...int) []int {
	seen := make(map[int]bool, len(dst)+len(values))
	for _, v := range dst {
		seen[v] = true
	}
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		dst = append(dst, v)
	}
	return dst
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

func traceBoolPtr(v bool) *bool {
	out := v
	return &out
}

func applyLineRange(start, end *int, line int) {
	if line <= 0 {
		return
	}
	if *start == 0 || line < *start {
		*start = line
	}
	if line > *end {
		*end = line
	}
}

func applyOffsetRange(minOffset, maxOffset *int64, offset int64) {
	if offset <= 0 && *minOffset != 0 && *maxOffset != 0 {
		return
	}
	if *minOffset == 0 || offset < *minOffset {
		*minOffset = offset
	}
	if offset > *maxOffset {
		*maxOffset = offset
	}
}

func isFileIOEvent(ev Event) bool {
	name := strings.ToLower(ev.Name)
	if ev.Inode == "" && ev.EntryName == "" && ev.FSDev == "" {
		return false
	}
	if isPageCacheEvent(ev) {
		return false
	}
	for _, token := range []string{
		"android_fs_dataread", "android_fs_datawrite", "f2fs_direct_io",
		"f2fs_sync_file", "f2fs_submit_read_bio", "f2fs_submit_write_bio",
		"ext4_", "erofs_", "file_system", "filesystem", "ebpf_file",
	} {
		if strings.Contains(name, token) {
			return true
		}
	}
	return ev.Type == EventFilesystem && (ev.Inode != "" || ev.EntryName != "")
}

func fileIOCountsAsActivity(ev Event) bool {
	name := strings.ToLower(ev.Name)
	if strings.HasSuffix(name, "_end") || strings.HasSuffix(name, "_exit") || strings.HasSuffix(name, "_done") {
		return false
	}
	return true
}

func isPageCacheEvent(ev Event) bool {
	name := strings.ToLower(ev.Name)
	return ev.MemoryKind == "page_cache" ||
		ev.SubsystemKind == "page_cache" ||
		strings.Contains(name, "filemap_add_to_page_cache") ||
		strings.Contains(name, "filemap_delete_from_page_cache")
}

func isStorageLatencyEvent(ev Event) bool {
	if ev.Type == EventBlockIssue || ev.Type == EventBlockComplete {
		return true
	}
	layer := storageLatencyLayer(ev)
	if layer == "" {
		return false
	}
	_, phase := storageLatencyBaseAndPhase(ev)
	return phase != ""
}

func storageLatencyLayer(ev Event) string {
	name := strings.ToLower(ev.Name)
	switch {
	case ev.Type == EventBlockIssue || ev.Type == EventBlockComplete:
		return "block"
	case strings.HasPrefix(name, "mmc_"):
		return "mmc"
	case strings.HasPrefix(name, "scsi_"):
		return "scsi"
	case strings.HasPrefix(name, "f2fs_"):
		return "f2fs"
	case strings.HasPrefix(name, "android_fs_"):
		return "android_fs"
	case strings.HasPrefix(name, "ext4_"):
		return "ext4"
	case ev.Type == EventStorage:
		return "storage"
	case ev.Type == EventFilesystem:
		return "filesystem"
	default:
		return ""
	}
}

func storageLatencyBaseAndPhase(ev Event) (base, phase string) {
	name := strings.ToLower(strings.TrimSpace(ev.Name))
	for _, suffix := range []string{"_start", "_enter", "_begin"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix), "start"
		}
	}
	for _, suffix := range []string{"_done", "_exit", "_end", "_complete"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix), "done"
		}
	}
	return "", ""
}

func firstPositiveFloat(values ...float64) float64 {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}
