package tracequery

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/hanchaoqun/codrax/internal/types"
)

// unparsedLineCaveatRatio is the fraction of scanned lines that must fail to
// match any known trace format before the result carries a coverage caveat.
// Soft guidance only: the typed UnparsedLines counter gates nothing.
const unparsedLineCaveatRatio = 0.5

const wakeupMatchToleranceSec = 0.000005
const wakeupCausalAggregateOccurrenceCap = 8

// VS-1 periodic-source detection (§7.8, customer ruling): a sleep-dominant
// (waker→target) aggregate with at least wakeupPeriodicMinOccurrences
// occurrences is a periodic signal source (e.g. a VSync generator) when its
// adjacent actual-window start intervals hold a cadence around the robust
// period p. The occurrences come from branch top-K SELECTION, so the interval
// sequence is NOT a signal timeline (F1, adversarial review 2026-07-04) —
// selected occurrences need not be adjacent signal ticks — and every reading
// below is immune to that selection:
//   - p is the LOWER median (sorted[(n−1)/2], one uniform rule for odd and
//     even counts) of the intervals that remain after carving observation
//     gaps: an interval within ±tol·p of an integer multiple k·p (k≥2) is a
//     gap between non-adjacent selected occurrences — never lateness, never a
//     veto (F3: an even-count two-middle AVERAGE gets pulled between cadence
//     bands by one gap/extreme and lands on a period no real interval has);
//   - an interval below p×(1−tol) is an EARLY fire: a fixed-period source
//     never fires early, so detection vetoes;
//   - lateness is NOT an interval reading at all: per occurrence it is
//     max(0, target_blocked − p) — how much the target's wait for THIS signal
//     exceeded one period (the customer semantics "the signal arrived later
//     than expected"), independent of occurrence adjacency; the aggregate sum
//     is capped at raw − runnable so a fabricated amount can never reach the
//     Summary. Intervals are used ONLY to estimate p and drive the vetoes;
//   - the in-band ratio gate (F4): after the gap carve at least 2/3 of the
//     remaining intervals must sit inside [p×(1−tol), p×(1+tol)]. Late
//     intervals still never veto by themselves (their overage reaches
//     LatenessMs through the blocked caliber) — they just must not dominate
//     the sample.
//
// The 15% tolerance is a NOISE threshold and therefore drives ONLY soft
// surfaces (effective-impact accounting, labels, rank ordering) — never a
// hard structural gate (precise-signals red line). The runnable and lateness
// amounts that DO count are exact arithmetic.
//
// wakeupPeriodicMinOccurrences is 5 (F4, adversarial review 2026-07-04): the
// trace records WHEN a wakeup happened, never WHY — a demand-driven worker
// woken a couple of times at coincidentally similar spacing is unobservably
// different from a real generator, so at 3 occurrences (2 intervals) the
// discount could silence a genuine on-demand wait. Five occurrences with a
// 2/3 in-band majority make an accidental cadence unlikely while real polling
// / tick workers (berlin: 6 VSync occurrences) still clear the bar; the cost
// of the stricter gate is only that a short-lived periodic source keeps its
// raw (conservative, pre-VS-1) attribution.
const wakeupPeriodicMinOccurrences = 5
const wakeupPeriodicIntervalTolerance = 0.15
const microWindowProbeSeconds = 0.050
const preferredCoverageWindowMinSeconds = 0.080
const preferredCoverageWindowMaxSeconds = 0.150
const parentWindowStrategySeconds = 1.000

func traceMarkActionFilterInvalidResult(idx *Index, q Query, validationErr error) Result {
	res := Result{
		View:      CanonicalViewName(q.View),
		TimeUnit:  "seconds",
		TimeStart: q.TimeStart,
		TimeEnd:   q.TimeEnd,
		Caveats: []string{
			"trace_mark_action_filter_invalid=true; no trace rows were evaluated; " + validationErr.Error(),
		},
	}
	if idx == nil {
		return res
	}
	res.SourcePath = idx.Path
	res.TraceArtifacts = append([]TraceArtifactSource(nil), idx.TraceArtifacts...)
	res.LineCount = idx.LineCount
	res.ScannedLineCount = idx.ScannedLineCount
	res.IndexWindowed = idx.Windowed
	res.IndexTimeStart = idx.IndexTimeStart
	res.IndexTimeEnd = idx.IndexTimeEnd
	res.IndexLineStart = idx.IndexLineStart
	res.IndexLineEnd = idx.IndexLineEnd
	res.EventCount = len(idx.Events)
	res.UnparsedLineCount = idx.UnparsedLines
	res.ParseLinePanics = idx.ParseLinePanics
	res.ClockRegressions = idx.ClockRegressions
	return res
}

func Run(idx *Index, q Query) Result {
	explicitTimeStart := queryExplicitTimeStart(q)
	explicitTimeEnd := queryExplicitTimeEnd(q)
	if !explicitTimeStart && !explicitTimeEnd && strings.TrimSpace(q.Pattern) != "" {
		q.FrameWindowAutoDerived = true
	}
	// Freeze caller-provided boundedness before normalizeQuery fills missing
	// endpoints from index metadata.  A derived default is useful for view
	// execution but is not proof that the caller supplied a state-account
	// window; explicit [0,x] remains distinguishable through TimeStartSet.
	stateAccountTimeStartBounded := queryBoundedTimeStart(q)
	stateAccountTimeEndBounded := queryBoundedTimeEnd(q)
	boundedWindowOrSelector := stateAccountTimeStartBounded ||
		stateAccountTimeEndBounded ||
		q.LineStart != 0 ||
		q.LineEnd != 0 ||
		strings.TrimSpace(q.SpanName) != "" ||
		q.PID > 0 ||
		strings.TrimSpace(q.Thread) != "" ||
		strings.TrimSpace(q.ThreadInput) != "" ||
		len(q.EventTypes) > 0
	q = normalizeQuery(idx, q)
	if err := ValidateTraceMarkActionFilter(q.View, q.EventTypes, q.TraceMarkActions); err != nil {
		return traceMarkActionFilterInvalidResult(idx, q, err)
	}
	flavor, confidence, signals, flavorCaveats := resolveTraceFlavor(idx, q)
	q.TraceFlavor = flavor
	frameworkSurfaces := detectFrameworkSurfaces(idx, q, TracePlatformAuto, 4)
	platform, platformCandidate, platformCandidateConfidence, platformCandidateSignals, platformCaveats := resolveTracePlatform(idx, q, flavor, idx.platformDetectionSurfaces(), signals)
	if platform == TracePlatformDonghu && q.TraceFlavorHintSource == "" && q.TraceFlavorHint != TraceFlavorAndroidAtrace {
		flavor = TraceFlavorHarmonyHitrace
		q.TraceFlavor = flavor
		if confidence < platformCandidateConfidence {
			confidence = platformCandidateConfidence
		}
	}
	q.TracePlatform = platform
	spanWindows, spanCaveats, spanCompaction := resolveSpanWindowsForQuery(idx, &q, explicitTimeStart, explicitTimeEnd)
	res := Result{
		View:                        q.View,
		SourcePath:                  idx.Path,
		TraceArtifacts:              append([]TraceArtifactSource(nil), idx.TraceArtifacts...),
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
			rank := buildRootCauseRankFrom(idx, q, chain, stats)
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
			spanWindows, spanCaveats, spanCompaction = findSpanWindowsCompacted(idx, q, q.Limit)
			res.SpanWindows = spanWindows
		}
		res.EvidencePack = evidenceFromSpans(spanWindows)
	case "thread_timeline":
		tl := ThreadTimeline(idx, q)
		res.Timeline = &tl
		res.Caveats = append(res.Caveats, tl.Caveats...)
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
			if idx.RelationScoped {
				// The relation-pruned event set is complete for the target/waker
				// chain, not for all-thread CPU/off-CPU aggregates.  Publishing
				// WindowStats here would mix a subset of in-window transitions with
				// full-artifact head checkpoints and fabricate whole-window load.
				res.Caveats = append(res.Caveats, "relation_scoped_window_stats_unavailable=true; wakeup_chain is complete for the retained target/waker closure, but global window_stats are omitted because the index intentionally pruned unrelated scheduler events")
			} else {
				stats := getStats()
				res.WindowStats = &stats
			}
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
		// Q4-K 修1 (ledger §12.1): blocking evidence appends SECOND, right
		// after the chain — the tail of the legacy chain→frame→rank→blocking
		// order fell past the 16-fact pack cap and the lock evidence went
		// invisible on the pack face.
		if bundle.CriticalBlocking != nil {
			res.CriticalBlocking = bundle.CriticalBlocking
			res.EvidencePack = append(res.EvidencePack, evidenceFromCriticalBlocking(*bundle.CriticalBlocking)...)
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
		if bundle.windowStats != nil {
			res.WindowStats = bundle.windowStats
		} else {
			stats := getStats()
			res.WindowStats = &stats
		}
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
		if recipeShouldUseDiscoveryOnly(q, recipe, boundedWindowOrSelector) {
			recipe.IncludedViews = []string{"frame_window", "frame_timeline", "frame_flow"}
			recipe.Caveats = append(recipe.Caveats, "unbounded jank recipe ran in discovery mode because no time, line, span, pid/thread, or event filters were provided; select a frame/span/window before requesting full root-cause/resource ranking")
			res.Caveats = append(res.Caveats, "large recipe guard: unbounded jank analysis skips full-trace scheduler/resource/root-cause expansion until the query is narrowed")
		}
		res.Recipe = &recipe
		if recipeHasView(recipe, "event_search") {
			// span_locate step 1: bare-pattern locate. Reuse the span label
			// as the literal pattern and deliberately drop event_types so
			// nonstandard marker forms still match.
			locate := q
			if strings.TrimSpace(locate.Pattern) == "" {
				locate.Pattern = strings.TrimSpace(locate.SpanName)
			}
			locate.EventTypes = nil
			if strings.TrimSpace(locate.Pattern) != "" {
				res.Events = EventSearch(idx, locate)
				res.EvidencePack = append(res.EvidencePack, evidenceFromEvents(res.Events)...)
			}
		}
		if recipeHasView(recipe, "span_window") {
			// span_locate step 2: resolve the located span into its start/end
			// time and line window (already resolved once when span_name was
			// set on the query). A pattern-only invocation mirrors the label
			// into SpanName here — without a name filter this step would
			// return EVERY complete span (StartTs-sorted, Limit-truncated),
			// publishing unrelated or even truncating out the target window.
			// Both labels empty → skip the step entirely; the recipe caveat
			// already asks for one.
			windowQ := q
			if strings.TrimSpace(windowQ.SpanName) == "" {
				windowQ.SpanName = strings.TrimSpace(windowQ.Pattern)
			}
			if strings.TrimSpace(windowQ.SpanName) != "" {
				if len(spanWindows) == 0 {
					spanWindows, spanCaveats, spanCompaction = findSpanWindowsCompacted(idx, windowQ, windowQ.Limit)
					res.SpanWindows = spanWindows
				}
				res.EvidencePack = append(res.EvidencePack, evidenceFromSpans(spanWindows)...)
			}
		}
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
		matchedEvents := 0
		if len(q.TraceMarkActions) > 0 {
			res.Events, matchedEvents = eventSearchWithAccounting(idx, q)
		} else {
			res.Events = EventSearch(idx, q)
		}
		res.EvidencePack = evidenceFromEvents(res.Events)
		// RFC #71 (§8.2 c4): pre-truncation frequency tier census — nil
		// unless the chronological display cap actually hid matched
		// cpu_frequency rows (non-truncated results stay byte-identical).
		if census := ComputeCPUFrequencyCensus(idx, q, res.Events); census != nil {
			res.CPUFrequencyCensus = census
			res.EvidencePack = append([]EvidenceFact{census.EvidenceFact()}, res.EvidencePack...)
		}
		if matchedEvents > len(res.Events) {
			last := res.Events[len(res.Events)-1]
			res.Compactions = append(res.Compactions, ViewCompaction{
				View:            FallbackViewEventSearch,
				Dimension:       CompactionDimensionEvents,
				Total:           matchedEvents,
				Emitted:         len(res.Events),
				LastEmittedTs:   last.Ts,
				LastEmittedLine: last.Line,
			})
			res.Caveats = append(res.Caveats,
				fmt.Sprintf("event_search_index_compacted=true; matched %d row(s) but returned the first %d chronological match(es) only; omitted rows may contain later trace-mark actions, so do not infer absence without narrowing the query", matchedEvents, len(res.Events)))
		}
	}
	// G1 跨车道对账 (§27.2, 2026-07-09): every view shape that carries BOTH
	// lanes in one result envelope (recipe with rank+blocking, the frame
	// bundle's shared pointers) reconciles here; single-lane shapes
	// (critical_blocking_calls alone, evidence_pack without rank) are no-ops
	// — 负向保护 by construction. Reset-first idempotent, so re-running over
	// the frame bundle's already-reconciled backing slices converges.
	if res.RootCauseRank != nil && res.CriticalBlocking != nil {
		reconcileCriticalBlockingWithRankFamilies(res.RootCauseRank, res.CriticalBlocking)
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
	if idx.PaddingTruncated && strings.TrimSpace(idx.PaddingTruncatedNote) != "" {
		// Padding-tail degrade marker (berlin.systrace 2026-07-03): the
		// windowed index hit its event budget only inside the safety padding,
		// AFTER the requested [TimeStart,TimeEnd] was fully parsed. Surface
		// the typed marker as a verbatim note line so the model consumes the
		// complete core-window result with the caveat attached. Deliberately
		// NOT mirrored onto Result.Compactions: that typed channel marks
		// truncated RESULT rows and drives the result-compacted refinement
		// (narrow/split-the-window suggestions) — steering the model to
		// re-split an already fully-parsed window would recreate the retry
		// loop this degrade exists to break.
		res.Caveats = append(res.Caveats, idx.PaddingTruncatedNote)
	}
	res.Caveats = append(res.Caveats, spanCaveats...)
	res.Caveats = append(res.Caveats, resultCaveats(idx, q, res)...)
	res.Caveats = dedupStrings(res.Caveats)
	if spanCompaction != nil {
		res.Compactions = append(res.Compactions, *spanCompaction)
	}
	attachEvidenceFactProvenance(res.EvidencePack, res.TraceArtifacts)
	collectResultCompactions(&res, q)
	// §29.27② 常态发布 (SMR-1 修复轮 引擎件①, 2026-07-13): every
	// target-anchored bounded-window run publishes the four-state account.
	// The frame-bundle copy stays the authority when present (same builder,
	// same scan discipline); the generic arm reuses the ONE shared timeline
	// helper — no second event rescan beyond the target's own timeline.
	// Exactly ONE copy per run (dup-lane discipline the tracediag schema pin
	// reviews): the bundle path keeps its own slot; the top-level slot fills
	// only on non-bundle runs.
	if res.TargetWindowStates == nil && (q.PID > 0 || strings.TrimSpace(q.Thread) != "") &&
		(res.FrameRootCauseBundle == nil || res.FrameRootCauseBundle.TargetWindowStates == nil) {
		if stateAccountTimeStartBounded && stateAccountTimeEndBounded && q.TimeEnd > q.TimeStart {
			target := ThreadRef{PID: q.PID, Comm: strings.TrimSpace(q.Thread)}
			window := TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}
			tl, ok := targetWindowTimeline(idx, q, target, window)
			// targetWindowTimeline resolves a name-only selector to one precise
			// scheduler TID (or fails closed).  Carry that resolved identity into
			// every refinement; the raw selector's PID=0/comm is only an input
			// hint and may be stale after a thread rename.
			res.TargetWindowStates = buildTargetWindowStateAccount(idx, tl, ok, tl.Thread, window, res.WindowStats)
		}
	}
	return res
}

// collectResultCompactions mirrors sub-result truncation records onto the
// top-level Result so the tool refinement layer reads one typed surface:
// every " compacted " prose caveat that propagates upward keeps a typed twin.
// Records are deduplicated because composite views (recipe, bundles) can hold
// the same sub-result twice (e.g. frame pipeline + frame timeline).
func collectResultCompactions(res *Result, q Query) {
	if res == nil {
		return
	}
	if res.SchedulerLatency != nil {
		res.Compactions = append(res.Compactions, res.SchedulerLatency.Compactions...)
	}
	if res.RootCauseRank != nil {
		res.Compactions = append(res.Compactions, res.RootCauseRank.Compactions...)
	}
	if res.InteractionStats != nil {
		res.Compactions = append(res.Compactions, res.InteractionStats.Compactions...)
	}
	if res.FramePipeline != nil {
		res.Compactions = append(res.Compactions, res.FramePipeline.Compactions...)
	}
	if res.FrameTimeline != nil {
		res.Compactions = append(res.Compactions, res.FrameTimeline.Compactions...)
	}
	if res.CriticalBlocking != nil {
		res.Compactions = append(res.Compactions, res.CriticalBlocking.Compactions...)
	}
	if res.IPCGraph != nil {
		res.Compactions = append(res.Compactions, res.IPCGraph.Compactions...)
	}
	// Legacy indexed event_search stops scanning at the cap, so its true total
	// is unknown (Total=0). Exact trace-mark action search installs its counted
	// compaction before this fallback; never publish a conflicting second ruler.
	if res.View == "event_search" && q.Limit > 0 && len(res.Events) >= q.Limit && !hasEventSearchCompaction(res.Compactions) {
		last := res.Events[len(res.Events)-1]
		res.Compactions = append(res.Compactions, ViewCompaction{
			View:            "event_search",
			Dimension:       CompactionDimensionEvents,
			Emitted:         len(res.Events),
			LastEmittedTs:   last.Ts,
			LastEmittedLine: last.Line,
		})
	}
	res.Compactions = dedupeViewCompactions(res.Compactions)
}

func hasEventSearchCompaction(in []ViewCompaction) bool {
	for _, compaction := range in {
		if compaction.View == FallbackViewEventSearch && compaction.Dimension == CompactionDimensionEvents {
			return true
		}
	}
	return false
}

func dedupeViewCompactions(in []ViewCompaction) []ViewCompaction {
	if len(in) < 2 {
		return in
	}
	seen := make(map[ViewCompaction]bool, len(in))
	out := in[:0]
	for _, comp := range in {
		if seen[comp] {
			continue
		}
		seen[comp] = true
		out = append(out, comp)
	}
	return out
}

func queryExplicitTimeStart(q Query) bool {
	if q.FrameWindowAutoDerived {
		return false
	}
	if q.TimeStartSet {
		return true
	}
	return q.TimeStart != 0 && !q.FrameWindowAutoDerived
}

func queryExplicitTimeEnd(q Query) bool {
	if q.FrameWindowAutoDerived {
		return false
	}
	if q.TimeEndSet {
		return true
	}
	return q.TimeEnd != 0 && !q.FrameWindowAutoDerived
}

func queryBoundedTimeStart(q Query) bool {
	return q.TimeStartSet || q.TimeStart != 0
}

func queryBoundedTimeEnd(q Query) bool {
	return q.TimeEndSet || q.TimeEnd != 0
}

// resolveTracePlatform resolves the published platform label. W-1 修根
// (platform_surfaces.go, 2026-07-11): the detection input is the per-trace
// platformSurfaceScan record — ONE determination per trace consumed by every
// view — never a per-query window/filter-scoped surface enumeration (the
// witness flip: event_types=[unknown] matched android comms while
// [trace_mark]/[workqueue,dma_fence] did not, flipping harmony↔donghu inside
// one report). Explicit user/tool hints keep their short-circuits.
func resolveTracePlatform(idx *Index, q Query, flavor TraceFlavor, surfaces platformSurfaceScan, flavorSignals []string) (TracePlatform, string, float64, []string, []string) {
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
	// 复核 F3 (CLOSE-1, 2026-07-11): when no complete-coverage detection
	// record exists for this trace (every scan so far was filtered or
	// windowed), the label was inferred from a PARTIAL basis — disclose it
	// instead of silently publishing a per-query inference. A complete
	// record (Set ∧ !Scoped) publishes with zero disclosure.
	if !surfaces.Set || surfaces.Scoped {
		caveats = append(caveats, "platform_detection_basis=partial; the platform label was inferred from a filtered or windowed scan of this trace — a complete scan of the file has not confirmed it yet")
	}
	return platform, candidate, confidence, signals, caveats
}

func inferPlatformCandidate(idx *Index, q Query, flavor TraceFlavor, platform TracePlatform, surfaces platformSurfaceScan, flavorSignals []string) (string, float64, []string) {
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
	androidSurface := surfaces.Android
	harmonySurface := surfaces.Harmony
	for _, s := range surfaces.Signals {
		addSignal(s)
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
		key := threadKey(ref)
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
	q.View = CanonicalViewName(q.View)
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
		q.MaxDepth = wakeupChainDefaultMaxDepth
	}
	if q.MaxBranches <= 0 {
		q.MaxBranches = wakeupChainDefaultMaxBranches
	}
	if q.MinDurationMs <= 0 {
		q.MinDurationMs = 1
	}
	if q.Limit <= 0 {
		q.Limit = sharedDefaultResultLimit
	}
	if q.TimeStart == 0 && !q.TimeStartSet && q.LineStart == 0 && idx != nil {
		q.TimeStart = idx.FirstTs
	}
	if q.TimeEnd == 0 && !q.TimeEndSet && q.LineEnd == 0 && idx != nil {
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
	if err := ValidateTraceMarkActionFilter(q.View, q.EventTypes, q.TraceMarkActions); err != nil {
		return nil
	}
	q = ensureQueryFlavor(idx, q)
	typeSet := make(map[EventType]bool, len(q.EventTypes))
	for _, t := range q.EventTypes {
		if t != "" {
			typeSet[t] = true
		}
	}
	actionSet := traceMarkActionFilterSet(q.TraceMarkActions)
	var events []Event
	for _, ev := range idx.Events {
		if !eventInQuery(ev, q, typeSet, actionSet) {
			continue
		}
		events = append(events, ev)
		if len(events) >= q.Limit {
			break
		}
	}
	raw, rawIssues := loadRawArtifactLines(idx, events)
	out := make([]EventView, 0, len(events))
	for _, ev := range events {
		ev = applyPriorityFlavor(ev, q.TraceFlavor)
		view := idx.eventView(ev, raw[ev.Line])
		if issue := rawIssues[ev.Line]; issue != "" {
			view.RawUnavailableReason = issue
		}
		out = append(out, view)
	}
	return out
}

// eventSearchWithAccounting is the indexed twin of StreamEventSearch for the
// exact trace-mark action lane. It counts the complete matched set while
// retaining only the earliest limit rows, so both engines publish identical
// matched/emitted accounting without allocating an unbounded result slice.
func eventSearchWithAccounting(idx *Index, q Query) ([]EventView, int) {
	if idx == nil || ValidateTraceMarkActionFilter(q.View, q.EventTypes, q.TraceMarkActions) != nil {
		return nil, 0
	}
	q = ensureQueryFlavor(idx, q)
	typeSet := make(map[EventType]bool, len(q.EventTypes))
	for _, eventType := range q.EventTypes {
		if eventType != "" {
			typeSet[eventType] = true
		}
	}
	actionSet := traceMarkActionFilterSet(q.TraceMarkActions)
	matched := 0
	selected := make([]Event, 0, q.Limit)
	for _, event := range idx.Events {
		if !eventInQuery(event, q, typeSet, actionSet) {
			continue
		}
		matched++
		selected = insertEventChronological(selected, event, q.Limit)
	}
	raw, rawIssues := loadRawArtifactLines(idx, selected)
	out := make([]EventView, 0, len(selected))
	for _, event := range selected {
		event = applyPriorityFlavor(event, q.TraceFlavor)
		view := idx.eventView(event, raw[event.Line])
		if issue := rawIssues[event.Line]; issue != "" {
			view.RawUnavailableReason = issue
		}
		out = append(out, view)
	}
	return out, matched
}

func insertEventChronological(events []Event, candidate Event, limit int) []Event {
	if limit <= 0 {
		return events
	}
	position := sort.Search(len(events), func(i int) bool {
		if events[i].Ts != candidate.Ts {
			return events[i].Ts > candidate.Ts
		}
		return events[i].Line > candidate.Line
	})
	if len(events) >= limit && position >= limit {
		return events
	}
	events = append(events, Event{})
	copy(events[position+1:], events[position:])
	events[position] = candidate
	if len(events) > limit {
		events = events[:limit]
	}
	return events
}

func eventInQuery(ev Event, q Query, typeSet map[EventType]bool, actionSet map[string]bool) bool {
	if !eventInQueryWindow(ev, q) {
		return false
	}
	if len(typeSet) > 0 && !eventTypeMatches(ev, typeSet) {
		return false
	}
	if len(actionSet) > 0 && (ev.Type != EventTraceMark || !actionSet[ev.SpanAction]) {
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

// eventInQueryWindow is eventInQuery's line/time gate, extracted so the
// zero-match cross-type recount (crossTypePatternHits) applies the EXACT same
// window convention without drift: line bounds always apply; time bounds apply
// only when no line bounds are set.
func eventInQueryWindow(ev Event, q Query) bool {
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
	return true
}

func eventMatchesPattern(ev Event, pattern string) bool {
	needle := strings.ToLower(strings.TrimSpace(pattern))
	if needle == "" {
		return true
	}
	// SQL-perf source-only rows deliberately retain the producer's thread
	// coordinates as audit payload.  They are not proved trace identities, so
	// neither the raw field text nor any transport/header thread field may turn
	// them back into a name/PID-searchable event.  Keep this arm on the same
	// precise hard-negative predicate used by every typed thread selector.
	if perfSampleIsSourceOnlyIdentity(ev) {
		return perfSampleSourceOnlyInventoryMatchesPattern(ev, needle)
	}
	candidates := []string{
		string(ev.Type),
		ev.Name,
		ev.FieldText,
		ev.Comm,
		ev.PrevComm,
		ev.NextComm,
		ev.WakeeComm,
		ev.WakeePrioritySource(),
		ev.PrevState,
		ev.NextInfo,
		ev.NextInfoAffinity,
		ev.CGroup,
		ev.Reason,
		ev.SpanAction,
		ev.SpanName,
		ev.SpanValue,
		ev.ClockName,
		ev.IRQName,
		ev.IPITargetMask,
		ev.MemoryKind,
		ev.SubsystemKind,
	}
	ints := []int{
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
		ev.State,
		ev.Frequency,
		ev.FrequencyMin,
		ev.FrequencyMax,
		ev.CPUForField,
		ev.IOWait,
		ev.IRQID,
	}
	var int64s []int64
	if ss := ev.SchedStatFields; ss != nil {
		candidates = append(candidates, ss.Kind, ss.Comm)
		ints = append(ints, ss.PID)
		int64s = append(int64s, ss.DelayNs, ss.RunNs, ss.VRunNs)
	}
	if cf := ev.ConstraintFields; cf != nil {
		candidates = append(candidates, cf.Comm, cf.Kind, cf.Policy, cf.AllowedText, cf.CPUSetName)
		ints = append(ints, cf.PID, cf.CPU, cf.OrigCPU, cf.DestCPU)
	}
	if bf := ev.BinderFields; bf != nil {
		candidates = append(candidates, bf.Flags, bf.Code, bf.LockTag)
		ints = append(ints, bf.TransactionID, bf.DestProc, bf.DestThread, bf.Reply, bf.DebugID)
		int64s = append(int64s, bf.DataSize, bf.OffsetsSize, bf.ExtraSize)
	}
	if blk := ev.BlockIOFields; blk != nil {
		candidates = append(candidates, blk.Dev, blk.Op, blk.Error, blk.SrcDev)
		int64s = append(int64s, blk.Sector, blk.Len, blk.SrcSector, blk.RemapBios)
	}
	if rf := ev.ResourceFields; rf != nil {
		candidates = append(candidates, rf.Path, rf.Op, rf.Address, rf.Callstack)
		int64s = append(int64s, rf.Bytes)
	}
	if ff := ev.FileFields; ff != nil {
		candidates = append(candidates, ff.Dev, ff.Ino, ff.ParentIno, ff.Entry, ff.RW)
		int64s = append(int64s, ff.Offset, ff.Len, ff.Ret, ff.Size)
	}
	if pl := ev.PluginFields; pl != nil {
		candidates = append(candidates, pl.Domain, pl.EventName, pl.Metric, pl.Value, pl.Category, pl.SpanTrack)
	}
	if pf := ev.PerfFields; pf != nil {
		candidates = append(candidates, pf.Comm, pf.EventName, pf.Symbol, pf.DSO, pf.IP, pf.Callchain, pf.Source, pf.Resolution, pf.SourceComm, pf.SampleKindSource, pf.SymbolizationStatus, pf.Clock, pf.ClockConfidence, pf.CallchainStatus)
		ints = append(ints, pf.PID, pf.TID, pf.SourcePID, pf.SourceTID)
		int64s = append(int64s, pf.Period)
	}
	for _, candidate := range candidates {
		if strings.Contains(strings.ToLower(candidate), needle) {
			return true
		}
	}
	for _, value := range ints {
		if value != 0 && strings.Contains(fmt.Sprintf("%d", value), needle) {
			return true
		}
	}
	for _, value := range int64s {
		if value != 0 && strings.Contains(fmt.Sprintf("%d", value), needle) {
			return true
		}
	}
	if rf := ev.ResourceFields; rf != nil && rf.LatencyMs != 0 && strings.Contains(fmt.Sprintf("%.3f", rf.LatencyMs), needle) {
		return true
	}
	if ev.Ts != 0 && strings.Contains(fmt.Sprintf("%.6f", ev.Ts), needle) {
		return true
	}
	return false
}

// perfSampleSourceOnlyInventoryMatchesPattern is the closed searchable surface for
// an anonymous/source-only perf sample. It intentionally admits only perf
// workload inventory and provenance/quality dimensions. In particular it
// must not inspect Event.FieldText, Event.Comm/PID/TGID, PerfFields.Comm/PID/
// TID, or the perf_source_* audit coordinates.
//
// needle is already normalized by eventMatchesPattern. The caller must first
// establish perfSampleIsSourceOnlyIdentity(ev).
func perfSampleSourceOnlyInventoryMatchesPattern(ev Event, needle string) bool {
	pf := ev.PerfFields
	if pf == nil {
		return false
	}
	candidates := [...]string{
		string(ev.Type),
		pf.EventName,
		pf.Symbol,
		pf.DSO,
		pf.IP,
		pf.Addr,
		pf.Callchain,
		pf.Source,
		pf.Resolution,
		pf.SampleKind,
		pf.SampleKindSource,
		pf.SymbolizationStatus,
		pf.Clock,
		pf.ClockConfidence,
		pf.CallchainStatus,
	}
	for _, candidate := range candidates {
		if strings.Contains(strings.ToLower(candidate), needle) {
			return true
		}
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
	// Tool contract: pid is a thread id.  Process/TGID fields are deliberately
	// excluded; otherwise two same-name sibling TIDs in one process are merged
	// by event_search/perf_timeline despite an explicit numeric selector.
	if ev.Type == EventPerfSample && !perfSampleHasTypedThreadIdentity(ev) {
		return false
	}
	if ev.PID == pid || ev.PrevPID == pid || ev.NextPID == pid || ev.WakeePID == pid {
		return true
	}
	if ss := ev.SchedStatFields; ss != nil && ss.PID == pid {
		return true
	}
	if cf := ev.ConstraintFields; cf != nil && cf.PID == pid {
		return true
	}
	if bf := ev.BinderFields; bf != nil && bf.DestThread == pid {
		return true
	}
	if pf := ev.PerfFields; pf != nil && pf.TID == pid {
		return true
	}
	return false
}

func eventMentionsThread(ev Event, thread string) bool {
	sel := parseThreadSelector(thread)
	if sel.HasPID {
		// A pid-bearing selector is already precise.  Never fall back to comm,
		// symbols, span names or free-form fields after an exact-TID miss.
		return eventMentionsPID(ev, sel.PID)
	}
	if ev.Type == EventPerfSample && !perfSampleHasTypedThreadIdentity(ev) {
		// A bundle perf sample whose capability did not prove thread identity
		// is deliberately retained only as anonymous symbol/DSO inventory.
		// Do not resurrect its scrubbed identity through FieldText, symbol, DSO,
		// or callchain substring matching. The pattern field remains the proper
		// way to search those support dimensions.
		return false
	}
	names := []string{ev.Comm, ev.PrevComm, ev.NextComm, ev.WakeeComm, ev.SpanName, ev.SpanValue, ev.Reason, ev.IRQName, ev.FieldText}
	if ss := ev.SchedStatFields; ss != nil {
		names = append(names, ss.Comm)
	}
	if cf := ev.ConstraintFields; cf != nil {
		names = append(names, cf.Comm)
	}
	if pf := ev.PerfFields; pf != nil {
		names = append(names, pf.Comm, pf.Symbol, pf.DSO, pf.Callchain)
	}
	for _, v := range names {
		if threadSelectorMatchesName(sel, v) {
			return true
		}
	}
	return false
}

func perfSampleHasTypedThreadIdentity(ev Event) bool {
	if ev.Type != EventPerfSample || perfSampleIsSourceOnlyIdentity(ev) {
		return false
	}
	if ev.PID > 0 || ev.TGID > 0 || strings.TrimSpace(ev.Comm) != "" {
		return true
	}
	pf := ev.PerfFields
	return pf != nil && (pf.PID > 0 || pf.TID > 0 || strings.TrimSpace(pf.Comm) != "")
}

func ThreadTimeline(idx *Index, q Query) TimelineResult {
	q = ensureQueryFlavor(idx, q)
	resolution := resolveThreadSelection(idx, q)
	res := threadTimelineForTarget(idx, q, resolution.Thread, nil, nil, false)
	if resolution.Ambiguous {
		res.Caveats = append([]string{threadResolutionCaveat(idx, q)}, res.Caveats...)
	}
	return res
}

func threadTimelineForTarget(idx *Index, q Query, target ThreadRef, eventIDs []int, blockedReasonIDs []int, useIndexedEvents bool) TimelineResult {
	res := TimelineResult{
		Thread: target,
		Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
	}
	if target.PID == 0 && target.Comm == "" {
		res.Caveats = append(res.Caveats, "target thread not found; provide pid or a thread name visible in the trace")
		return res
	}
	if idx == nil {
		res.Caveats = append(res.Caveats, "trace index is empty")
		return res
	}
	if conflict := threadIncarnationConflictForQuery(idx, q, target.PID); conflict != nil {
		res.IntegrityFailure = "thread_incarnation_conflict"
		if q.TimeStart > 0 && q.LineStart == 0 && q.LineEnd == 0 {
			res.HeadState = &TimelineHeadState{Status: "unknown", BoundaryTs: q.TimeStart, Reason: "thread_selector_spans_incarnations"}
		}
		res.Caveats = append(res.Caveats, "thread_identity_fail_closed=true; "+conflict.reason()+"; one numeric TID denotes multiple tasks inside the selected window, so target intervals are omitted; split the query at boundary_ts/boundary_line")
		return res
	}
	if failure := schedulerStateIntegrityFailureForQuery(idx, q, target.PID); failure != nil {
		res.IntegrityFailure = failure.code
		if q.TimeStart > 0 && q.LineStart == 0 && q.LineEnd == 0 {
			res.HeadState = &TimelineHeadState{Status: "unknown", BoundaryTs: q.TimeStart, Reason: failure.code}
		}
		res.Caveats = append(res.Caveats, "scheduler_duration_fail_closed=true; "+failure.reason()+"; elapsed thread-state intervals are omitted because scheduler input completeness and same-lane ordering are not provable")
		return res
	}
	var runningStart float64
	var runningOpen bool
	var runningLine int
	var runningCPU int
	var offStart float64
	var offOpen bool
	var offLine int
	var offState string
	var offKnownState ThreadState
	var wake *Event
	boundaryObserved := false
	headExpected := idx.Windowed && q.TimeStart > 0 && q.LineStart == 0 && q.LineEnd == 0
	var head *schedulerHeadSnapshot
	if headExpected {
		head = schedulerHeadForQuery(idx, q)
	}
	if headExpected && head != nil && head.Complete {
		if state, ok := head.Threads[target.PID]; ok {
			if state.State == StateUnknown {
				res.HeadState = &TimelineHeadState{Status: "unknown", BoundaryTs: q.TimeStart, Reason: "prior_scheduler_state_unclassified", SourceLine: state.Line}
			} else {
				res.HeadState = &TimelineHeadState{
					Status:        "recovered",
					BoundaryTs:    q.TimeStart,
					State:         state.State,
					ActualStartTs: state.StartTs,
					SourceLine:    state.Line,
				}
				switch state.State {
				case StateRunning:
					runningStart, runningOpen = state.StartTs, true
					runningLine, runningCPU = state.Line, state.CPU
				default:
					offStart, offOpen = state.StartTs, true
					offLine, offKnownState, offState = state.Line, state.State, state.PrevStateRaw
				}
			}
		} else {
			res.HeadState = &TimelineHeadState{Status: "unknown", BoundaryTs: q.TimeStart, Reason: "no_prior_scheduler_state_for_target"}
		}
	} else if headExpected && head != nil && !head.Complete {
		res.HeadState = &TimelineHeadState{Status: "unknown", BoundaryTs: q.TimeStart, Reason: firstNonEmpty(head.Reason, "scheduler_head_snapshot_incomplete")}
	}
	visit := func(ev Event) {
		if q.LineStart > 0 || q.LineEnd > 0 {
			if q.LineStart > 0 && ev.Line < q.LineStart {
				return
			}
			if q.LineEnd > 0 && ev.Line > q.LineEnd {
				return
			}
		}
		if headExpected && head != nil && ev.Ts < q.TimeStart {
			// Any populated snapshot owns the prefix decision.  Incomplete
			// snapshots are fail-closed, so retained padding cannot sneak a
			// partial state back into the state machine.
			return
		}
		switch ev.Type {
		case EventSchedWakeup, EventSchedWaking:
			if !threadMatches(target, ev.WakeePID, ev.WakeeComm) {
				return
			}
			if schedWakeupStartsNewIncarnation(ev) {
				// A new incarnation terminates any state carried by the previous
				// occupant of this numeric TID.  Keep the old interval only up to
				// the reset boundary, then begin the new task as runnable.
				if runningOpen {
					iv := makeInterval(target, StateRunning, runningStart, ev.Ts, runningLine, ev.Line, "")
					iv.CPU, iv.CPUKnown = runningCPU, true
					res.Intervals = append(res.Intervals, iv)
				}
				if offOpen {
					res.Intervals = append(res.Intervals, offCPUIntervalsFromState(target, offStart, ev.Ts, offLine, ev.Line, offState, offKnownState, nil)...)
				}
				runningStart, runningOpen = 0, false
				offStart, offOpen = ev.Ts, true
				offLine, offKnownState, offState = ev.Line, StateRunnable, "R"
				copy := ev
				wake = &copy
				boundaryObserved = boundaryObserved || ev.Ts == q.TimeStart
				res.Caveats = append(res.Caveats, fmt.Sprintf("thread_generation_boundary=true; sched_wakeup_new reset tid=%d at line=%d", ev.WakeePID, ev.Line))
				return
			}
			if !offOpen && ev.Ts == q.TimeStart {
				offStart, offOpen = ev.Ts, true
				offLine, offKnownState, offState = ev.Line, StateRunnable, "R"
				boundaryObserved = true
			}
			if offOpen && ev.Ts >= offStart {
				copy := ev
				wake = &copy
			}
		case EventSchedSwitch:
			if threadMatches(target, ev.NextPID, ev.NextComm) {
				boundaryObserved = boundaryObserved || ev.Ts == q.TimeStart
				if offOpen {
					res.Intervals = append(res.Intervals, offCPUIntervalsFromState(target, offStart, ev.Ts, offLine, ev.Line, offState, offKnownState, wake)...)
				}
				offStart, offLine, offState, offKnownState, offOpen, wake = 0, 0, "", "", false, nil
				runningStart, runningOpen = ev.Ts, true
				runningLine = ev.Line
				runningCPU = ev.CPU
			}
			if threadMatches(target, ev.PrevPID, ev.PrevComm) {
				boundaryObserved = boundaryObserved || ev.Ts == q.TimeStart
				if runningOpen {
					iv := makeInterval(target, StateRunning, runningStart, ev.Ts, runningLine, ev.Line, "")
					iv.CPU, iv.CPUKnown = runningCPU, true
					res.Intervals = append(res.Intervals, iv)
				}
				runningStart, runningOpen = 0, false
				state := stateFromPrevState(ev.PrevState)
				if state == StateDead {
					offStart, offOpen = 0, false
					offLine, offState, offKnownState = 0, "", ""
				} else {
					offStart, offOpen = ev.Ts, true
					offLine = ev.Line
					offState = ev.PrevState
					offKnownState = ""
				}
				wake = nil
			}
		}
	}
	visitEventsInTimestampOrder(idx, eventIDs, useIndexedEvents, visit)
	if runningOpen {
		iv := makeInterval(target, StateRunning, runningStart, q.TimeEnd, runningLine, 0, "")
		iv.CPU, iv.CPUKnown = runningCPU, true
		res.Intervals = append(res.Intervals, iv)
	}
	if offOpen {
		res.Intervals = append(res.Intervals, offCPUIntervalsFromState(target, offStart, q.TimeEnd, offLine, 0, offState, offKnownState, wake)...)
	}
	enrichBlockedReasonIntervalsWithSelection(idx, target, res.Intervals, blockedReasonIDs, useIndexedEvents)
	res.Intervals = clampIntervals(res.Intervals, q)
	if headExpected {
		covered := false
		for _, interval := range res.Intervals {
			if interval.StartTs <= q.TimeStart && interval.EndTs >= q.TimeStart {
				covered = true
				break
			}
		}
		if covered {
			// Padding replay is authoritative only when the complete-file order
			// proof is monotonic.  It must never overwrite a fail-closed
			// timestamp_order_regressed/unproven snapshot decision.
			if (res.HeadState == nil && idx.TimestampOrder.AllowsTimeEndEarlyStop()) || boundaryObserved {
				res.HeadState = &TimelineHeadState{Status: "observed_in_index", BoundaryTs: q.TimeStart}
			}
			if res.HeadState != nil && res.HeadState.Status == "unknown" {
				res.Caveats = append(res.Caveats, "scheduler_head_state_unknown=true; retained padding cannot prove the governing window-head state without a monotonic complete-file timestamp order")
			}
		} else {
			reason := "scheduler_head_snapshot_unavailable"
			if res.HeadState != nil && res.HeadState.Reason != "" {
				reason = res.HeadState.Reason
			}
			res.HeadState = &TimelineHeadState{Status: "unknown", BoundaryTs: q.TimeStart, Reason: reason}
			res.Caveats = append(res.Caveats, "scheduler_head_state_unknown=true; the pre-window scheduler state could not be proven, so the window-head interval is typed unknown rather than omitted or assigned to a wait/running lane")
		}
	}
	if len(res.Intervals) == 0 {
		res.Caveats = append(res.Caveats, "no scheduler interval for the target thread was found in the selected window")
	}
	return res
}

func schedWakeupStartsNewIncarnation(ev Event) bool {
	return ev.Type == EventSchedWakeup && ev.Name == "sched_wakeup_new"
}

const schedulerHeadMissingSubjectDisplayCap = 32

func schedulerHeadCoverageForWindow(idx *Index, q Query, head *schedulerHeadSnapshot) *SchedulerHeadCoverage {
	if idx == nil || q.TimeStart <= 0 || q.LineStart > 0 || q.LineEnd > 0 {
		return nil
	}
	coverage := &SchedulerHeadCoverage{BoundaryTs: q.TimeStart}
	if head == nil || !head.Complete {
		coverage.Status = "unknown"
		coverage.Reason = "scheduler_head_snapshot_unavailable"
		if head != nil && head.Reason != "" {
			coverage.Reason = head.Reason
		}
		return coverage
	}
	knownCPUs := make(map[int]bool, len(head.CPUs))
	knownThreads := make(map[int]bool, len(head.Threads))
	for cpu := range head.CPUs {
		knownCPUs[cpu] = true
	}
	for pid, state := range head.Threads {
		if state.State != StateUnknown {
			knownThreads[pid] = true
		}
	}
	missingCPUs := map[int]bool{}
	missingThreads := map[int]bool{}
	markThreadBeforeEvent := func(pid int, ts float64) {
		if pid <= 0 {
			return
		}
		if ts > q.TimeStart && !knownThreads[pid] {
			missingThreads[pid] = true
		}
		knownThreads[pid] = true
	}
	visitEventsInTimestampOrder(idx, nil, false, func(ev Event) {
		if !eventLineInWindow(ev, q) || ev.Ts < q.TimeStart || (q.TimeEnd > 0 && ev.Ts > q.TimeEnd) {
			return
		}
		switch ev.Type {
		case EventSchedWakeup, EventSchedWaking:
			if schedWakeupStartsNewIncarnation(ev) {
				// The subject did not exist before this precise lifecycle edge.
				knownThreads[ev.WakeePID] = ev.WakeePID > 0
				return
			}
			markThreadBeforeEvent(ev.WakeePID, ev.Ts)
		case EventSchedSwitch:
			if ev.Ts > q.TimeStart && !knownCPUs[ev.CPU] {
				missingCPUs[ev.CPU] = true
			}
			knownCPUs[ev.CPU] = true
			markThreadBeforeEvent(ev.NextPID, ev.Ts)
			markThreadBeforeEvent(ev.PrevPID, ev.Ts)
			if stateFromPrevState(ev.PrevState) == StateDead {
				delete(knownThreads, ev.PrevPID)
			}
		}
	})
	coverage.MissingCPUCount = len(missingCPUs)
	coverage.MissingThreadCount = len(missingThreads)
	coverage.MissingCPUs = sortedBoundedIntSet(missingCPUs, schedulerHeadMissingSubjectDisplayCap)
	coverage.MissingThreadPIDs = sortedBoundedIntSet(missingThreads, schedulerHeadMissingSubjectDisplayCap)
	if coverage.MissingCPUCount > 0 || coverage.MissingThreadCount > 0 {
		coverage.Status = "partial_unknown"
		coverage.Reason = "subject_checkpoint_missing"
	} else {
		coverage.Status = "recovered"
	}
	return coverage
}

func sortedBoundedIntSet(values map[int]bool, limit int) []int {
	out := make([]int, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Ints(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func enrichBlockedReasonIntervals(idx *Index, target ThreadRef, intervals []Interval) {
	enrichBlockedReasonIntervalsWithSelection(idx, target, intervals, nil, false)
}

func enrichBlockedReasonIntervalsWithSelection(idx *Index, target ThreadRef, intervals []Interval, blockedReasonIDs []int, useIndexedEvents bool) {
	for i := range intervals {
		if intervals[i].State != StateDSleep {
			continue
		}
		reason := findBlockedReasonForWithSelection(idx, target, intervals[i].StartTs, intervals[i].EndTs, blockedReasonIDs, useIndexedEvents)
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
	return offCPUIntervalsFromState(thread, start, end, startLine, endLine, prevState, "", wake)
}

func offCPUIntervalsFromState(thread ThreadRef, start, end float64, startLine, endLine int, prevState string, knownState ThreadState, wake *Event) []Interval {
	if end <= start {
		return nil
	}
	state := stateFromPrevState(prevState)
	if knownState != "" {
		state = knownState
	}
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
	it := Interval{
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
	}
	it.Summary = intervalBaseSummary(it)
	return it
}

// intervalBaseSummary renders the canonical "<state> for <duration> ms"
// summary prefix from the interval's CURRENT typed DurationMs. Single
// renderer shared by construction (makeIntervalWithWake) and post-clamp
// regeneration (clampIntervals via clampedIntervalSummary) so the prose face
// can never publish a duration the typed fields no longer carry.
func intervalBaseSummary(it Interval) string {
	return fmt.Sprintf("%s for %.3f ms", it.State, it.DurationMs)
}

// clampedIntervalSummary rebuilds an interval's prose Summary from its
// clamped typed fields. Any enrichment detail appended before clamping
// (enrichBlockedReasonIntervalsWithSelection writes "<state> for <ms> ms;
// sched_blocked_reason ...") is preserved verbatim from the first "; " on —
// the base "<state> for <ms> ms" prefix never contains that separator.
// Segments cut by the query window additionally publish the dual ledger in
// the root_cause_rank projected/actual token form (actual_* key=value, cf.
// renderWakeupCausalImpactSummary): the leading duration stays the in-window
// (clamped) figure that timeline rows and per-state totals use, and
// actual_duration/actual_window carry the full scheduler segment so a
// cross-window wait is disclosed instead of silently shrunk.
func clampedIntervalSummary(it Interval) string {
	summary := intervalBaseSummary(it)
	if it.WindowClamped() {
		summary += fmt.Sprintf(" actual_duration=%.3fms actual_window=%.6f..%.6f",
			it.ActualDurationMsResolved(), it.ActualStartTs, it.ActualEndTs)
	}
	if idx := strings.Index(it.Summary, "; "); idx >= 0 {
		summary += it.Summary[idx:]
	}
	return summary
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
	// §7.11 B-1 (customer_dead_session_audit_20260703.md): precise
	// single-char extensions I/T/t/X/Z. Modifier suffixes ("I|K") keep the
	// existing HasPrefix drop semantics; unlisted codes stay Unknown.
	case strings.HasPrefix(prev, "I"):
		// TASK_IDLE — a kworker parked idle. Books to the interruptible-
		// sleep family (reference consumers classify I alongside sleep,
		// perfetto.rs:2865); NOT D-sleep, so I never inflates the
		// uninterruptible/IO pressure lanes.
		return StateSSleep
	case strings.HasPrefix(prev, "T"), strings.HasPrefix(prev, "t"):
		return StateStopped
	case strings.HasPrefix(prev, "X"), strings.HasPrefix(prev, "Z"):
		return StateDead
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
		// An interval that lived wholly before the selected window and closes
		// exactly at its left edge has no in-window support.  Suppress only that
		// stale carry row; zero-width rows at the right edge are retained because
		// their actual_* ledger is deliberate evidence of window clamping.
		zeroWidthStaleHeadCarry := it.DurationMs == 0 && q.TimeStart > 0 &&
			it.EndTs == q.TimeStart && it.ActualStartTs < q.TimeStart && it.ActualEndTs <= q.TimeStart
		if !zeroWidthStaleHeadCarry && it.DurationMs >= 0 {
			// E1-a (RTC-R1 e1, 2026-07-05): Summary is minted at construction
			// from the UNCLAMPED duration, and evidenceFromTimeline republishes
			// Interval.Summary verbatim into the evidence pack. Without
			// regeneration a window-cut segment renders "running ... 0.000ms"
			// on the timeline row (clamped, correct) while the pack row still
			// says "running for 0.987 ms" (stale) — and the model trusts the
			// pack. Rebuild the prose face from the clamped typed fields,
			// preserving enrichment suffixes, and disclose window-cut segments
			// with the established actual_* dual-ledger tokens.
			it.Summary = clampedIntervalSummary(it)
			out = append(out, it)
		}
	}
	return out
}

func resolveThread(idx *Index, q Query) ThreadRef {
	return resolveThreadSelection(idx, q).Thread
}

func threadMatches(ref ThreadRef, pid int, comm string) bool {
	// A positive scheduler PID is the precise identity signal. Once the
	// target carries one, comm is display metadata only: names can collide
	// across processes and can change during a thread's lifetime.
	if ref.PID > 0 {
		return pid > 0 && pid == ref.PID
	}
	// Some trace formats genuinely omit a usable PID. Preserve the explicit
	// comm-only fallback for that degraded shape, but never let it override a
	// known target PID above.
	return ref.Comm != "" && comm != "" && strings.EqualFold(ref.Comm, comm)
}

func ComputeWindowStats(idx *Index, q Query) WindowStats {
	q = ensureQueryFlavor(idx, q)
	stats := WindowStats{
		Window:      TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
		EventCounts: map[EventType]int{},
	}
	if idx == nil {
		stats.Caveats = append(stats.Caveats, "trace index is empty")
		return stats
	}
	if idx.RelationScoped {
		stats.SchedulerHeadCoverage = &SchedulerHeadCoverage{Status: "unknown", BoundaryTs: q.TimeStart, Reason: "relation_scoped_event_subset"}
		stats.Caveats = append(stats.Caveats, "relation_scoped_window_stats_unavailable=true; global scheduler aggregates are omitted because this index intentionally retains only a target/waker relation closure")
		return stats
	}
	durationFailures := durationOrderFailuresForQuery(idx, q)
	durationPairingIntegrities := durationPairingIntegritiesForQuery(idx, q)
	frequencyIntegrity := frequencyOrderIntegrityForQuery(idx, q)
	stats.Caveats = append(stats.Caveats, frequencyIntegrity.caveats()...)
	stats.Caveats = append(stats.Caveats, cpuInputIntegrityCaveats(idx, q)...)
	stats.Caveats = append(stats.Caveats, traceMarkIntegrityCaveats(idx, q)...)
	schedulerFailure := schedulerStateIntegrityFailureForQuery(idx, q, 0)
	identityConflict := threadIncarnationConflictForQuery(idx, q, 0)
	schedulerDurationsSafe := schedulerFailure == nil && identityConflict == nil
	if schedulerFailure != nil {
		stats.SchedulerHeadCoverage = &SchedulerHeadCoverage{Status: "unknown", BoundaryTs: q.TimeStart, Reason: schedulerFailure.code}
		stats.Caveats = append(stats.Caveats, "scheduler_duration_fail_closed=true; "+schedulerFailure.reason()+"; scheduler busy/off-CPU/latency/churn durations are omitted because scheduler input completeness and same-lane ordering are not provable")
	}
	if identityConflict != nil {
		stats.SchedulerHeadCoverage = &SchedulerHeadCoverage{Status: "unknown", BoundaryTs: q.TimeStart, Reason: "thread_incarnation_conflict"}
		// ENG audit #44 (§29.25 处置委托 2026-07-10): the caveat names the
		// process-domain census because that face is also withheld below — its
		// thread count and catalog TGID/comm attribution would otherwise seat a
		// reused TID's new task in the old task's process domain.
		stats.Caveats = append(stats.Caveats, "thread_identity_fail_closed=true; "+identityConflict.reason()+"; scheduler duration aggregates and the process-domain census are omitted because their PID-keyed rows cannot safely merge multiple task incarnations; split the window at the lifecycle boundary")
	}
	byCPU := map[int][]Event{}
	freqByCPU := map[int][]Event{}
	// CFC (§7.10 VS-2c 设计): governed limits timeline for the cluster-ceiling
	// snapshot — head-governing caliber needs pre-window rows, so this is
	// collected beside freqByCPU (upper bound only), NOT inside the strict
	// in-window switch that feeds stats.CPUFrequencyLimits (deliberate caliber
	// fork, pinned by TestComputeWindowStats_ClusterFrequencyCeilingsSnapshot).
	limitTimelineByCPU := map[int][]freqSample{}
	blockedReasons := map[string]BlockedReasonSummary{}
	freqLimits := map[int]CPUFrequencyLimit{}
	subsystems := map[string]SubsystemEventSummary{}
	bioResources := map[string]*RuntimeResourceSummary{}
	filesystemResources := map[string]*RuntimeResourceSummary{}
	pageFaultResources := map[string]*RuntimeResourceSummary{}
	bioResourceContributorPIDs := map[int]bool{}
	filesystemResourceContributorPIDs := map[int]bool{}
	pageFaultResourceContributorPIDs := map[int]bool{}
	fileIO := map[string]*FileIOSummary{}
	pageCache := map[string]*PageCacheSummary{}
	abilityEvents := map[string]*TracePluginSummary{}
	xpowerEvents := map[string]*TracePluginSummary{}
	hiSystemEvents := map[string]*TracePluginSummary{}
	abilityContributorPIDs := map[int]bool{}
	xpowerContributorPIDs := map[int]bool{}
	hiSystemContributorPIDs := map[int]bool{}
	// Workqueue/DMA and the direct resource/plugin summaries consume exactly
	// their selected in-window Event rows, so this scan can enumerate their
	// complete numeric identity dependencies without a second pass or a noisy
	// inference. File-IO/page-cache/storage composites retain the global
	// conservative gate until their multi-input completeness is propagated.
	workqueueContributorPIDs := map[int]bool{}
	dmaFenceContributorPIDs := map[int]bool{}
	// CR-4 引擎件 (2026-07-12, tieba witness: 1697 条 in-window wakeup 全部
	// target_cpu=000): the wakeup target_cpu ALL-ZERO census. A converter
	// degradation writes every wakeup's target_cpu as 0, silently funneling
	// the whole cross-thread runnable backlog into "CPU0" on every
	// target_cpu-keyed per-CPU face. Valid-parse events only (malformed
	// values are the cpu_input_integrity module's lane).
	wakeupTargetCPUTotal, wakeupTargetCPUZero := 0, 0
	wakeupHeaderCPUs := map[int]bool{}
	for _, ev := range idx.Events {
		// CFC P0 (§7.10 VS-2c): the per-CPU frequency basis admits only genuine
		// per-CPU samples — reclassified clock_set_rate lanes are excluded by
		// the SAME shared predicate the fold face uses (isPerCPUFrequencySample,
		// cluster_ceilings.go), closing the window-face pollution lane
		// (fabricated cpu0 residency / flipped topology / false low_frequency).
		if eventLineInWindow(ev, q) && isPerCPUFrequencySample(ev) {
			if q.TimeEnd == 0 || ev.Ts <= q.TimeEnd {
				cpu := eventCPUForStats(ev)
				if !frequencyIntegrity.frequencyUnsafe(cpu) {
					freqByCPU[cpu] = append(freqByCPU[cpu], ev)
				}
			}
		}
		// CFC F1: admission + CPU attribution via the shared limits predicate
		// (isPerCPULimitSample, cluster_ceilings.go). The line-window +
		// upper-bound-only filter is THIS face's window convention
		// (head-governing caliber needs pre-window rows — see the
		// limitTimelineByCPU declaration above).
		if cpu, ok := isPerCPULimitSample(ev); ok && eventLineInWindow(ev, q) {
			if q.TimeEnd == 0 || ev.Ts <= q.TimeEnd {
				if !frequencyIntegrity.limitUnsafe(cpu) {
					limitTimelineByCPU[cpu] = append(limitTimelineByCPU[cpu], freqSample{ts: ev.Ts, khz: ev.FrequencyMax})
				}
			}
		}
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		stats.EventCounts[ev.Type]++
		if ev.Type == EventSchedWakeup || ev.Type == EventSchedWaking {
			if cpu, ok := eventTargetCPU(ev); ok {
				wakeupTargetCPUTotal++
				if cpu == 0 {
					wakeupTargetCPUZero++
				}
				wakeupHeaderCPUs[ev.CPU] = true
			}
		}
		switch ev.Type {
		case EventSchedSwitch:
			if schedulerDurationsSafe {
				byCPU[ev.CPU] = append(byCPU[ev.CPU], ev)
			}
		case EventCPUFrequencyLimit:
			if !frequencyIntegrity.limitUnsafe(eventCPUForStats(ev)) {
				accumulateCPUFrequencyLimit(freqLimits, ev)
			}
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
			if ev.PID > 0 {
				workqueueContributorPIDs[ev.PID] = true
			}
		case EventDMAFence:
			stats.DMAFenceEventCount++
			if ev.PID > 0 {
				dmaFenceContributorPIDs[ev.PID] = true
			}
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
		resourceKind := accumulateRuntimeResource(bioResources, filesystemResources, pageFaultResources, ev)
		if ev.PID > 0 {
			switch resourceKind {
			case "bio":
				bioResourceContributorPIDs[ev.PID] = true
			case "filesystem":
				filesystemResourceContributorPIDs[ev.PID] = true
			case "page_fault":
				pageFaultResourceContributorPIDs[ev.PID] = true
			}
		}
		accumulateFileIO(fileIO, ev)
		accumulatePageCache(pageCache, ev)
		pluginKind := accumulateTracePluginEvent(abilityEvents, xpowerEvents, hiSystemEvents, ev)
		if ev.PID > 0 {
			switch pluginKind {
			case "ability_monitor":
				abilityContributorPIDs[ev.PID] = true
			case "xpower":
				xpowerContributorPIDs[ev.PID] = true
			case "hi_sysevent":
				hiSystemContributorPIDs[ev.PID] = true
			}
		}
	}
	// CR-4 引擎件: the all-zero verdict fires only on the PRECISE degraded
	// shape — a large in-window wakeup population, every valid target_cpu
	// exactly 0, while the wakeups themselves are EMITTED from ≥2 distinct
	// CPUs (a genuine single-core trace legitimately targets cpu0 and stays
	// silent). Typed fact, disclosure only.
	if wakeupTargetCPUTotal >= wakeupTargetCPUDegradedFloor &&
		wakeupTargetCPUZero == wakeupTargetCPUTotal && len(wakeupHeaderCPUs) >= 2 {
		stats.Caveats = append(stats.Caveats, fmt.Sprintf(
			"wakeup_target_cpu_degraded=true total=%d — every in-window sched_wakeup/sched_waking target_cpu is 0 while wakeups are emitted from %d CPUs (suspected converter degradation); per-CPU runnable/pressure accounting keyed on target_cpu is unreliable",
			wakeupTargetCPUTotal, len(wakeupHeaderCPUs)))
	}
	// Scheduler carry-in: an in-window sched_switch describes the CPU state
	// AFTER that instant, so without the last pre-window switch the first CPU
	// segment was silently absent. Seed one synthetic internal switch per known
	// CPU at the exact query head; it is never added to EventCounts/evidence.
	if schedulerDurationsSafe && q.TimeStart > 0 && q.LineStart == 0 && q.LineEnd == 0 {
		head := schedulerHeadForQuery(idx, q)
		stats.SchedulerHeadCoverage = schedulerHeadCoverageForWindow(idx, q, head)
		if head != nil && head.Complete {
			for _, state := range schedulerHeadSortedCPUs(head) {
				hasBoundarySwitch := false
				for _, ev := range byCPU[state.CPU] {
					if ev.Ts == q.TimeStart {
						hasBoundarySwitch = true
						break
					}
				}
				if hasBoundarySwitch {
					continue
				}
				byCPU[state.CPU] = append(byCPU[state.CPU], Event{
					Line:     state.Line,
					Ts:       q.TimeStart,
					CPU:      state.CPU,
					Type:     EventSchedSwitch,
					NextComm: state.Thread.Comm,
					NextPID:  state.Thread.PID,
					NextPrio: state.Priority,
				})
			}
		} else {
			reason := "scheduler_head_snapshot_unavailable"
			if head != nil && head.Reason != "" {
				reason = head.Reason
			}
			stats.Caveats = append(stats.Caveats, "scheduler_head_state_unknown=true; CPU/off-CPU totals may omit an unclassified window-head segment reason="+reason)
		}
		if coverage := stats.SchedulerHeadCoverage; coverage != nil && coverage.Status == "partial_unknown" {
			stats.Caveats = append(stats.Caveats, fmt.Sprintf("scheduler_head_subjects_unknown=true; complete prefix scan lacked governing state for %d in-window CPU(s) %v and %d in-window thread(s) %v, so their pre-first-event head segments are omitted rather than assigned to a state", coverage.MissingCPUCount, coverage.MissingCPUs, coverage.MissingThreadCount, coverage.MissingThreadPIDs))
		}
	}
	sortFrequencyTimeline(freqByCPU)
	for _, samples := range limitTimelineByCPU {
		sort.SliceStable(samples, func(i, j int) bool { return samples[i].ts < samples[j].ts })
	}
	// CFR (#75 簇共频, 客户硬件域裁定) + CFR-2 (#80 变化点推导): the
	// frequency-WEIGHTED faces (busy-loop thread frequency/weighting, off-CPU
	// frequency context, compute_supply per-CPU fmax) may read a same-cluster
	// sampled sibling's timeline for cores without their own samples — the
	// cluster shares one hardware frequency point. Single resolution
	// authority: cluster_freq_share.go — explicit core_topology first, and in
	// its absence the CFR-2 change-point derivation (identical emission
	// sequences merge; unsampled cores inherit toward higher core numbers;
	// never upward past the highest sampled core; a core with own samples
	// never takes a donor). CAP-3 (§29.11 拓扑全局基): the derivation reads
	// the INDEX-global sample stream — cluster topology is a trace attribute,
	// and the old window-cropped input moved the carve boundary per query and
	// forked the judgment between faces. hasSamples and the donor timelines'
	// VALUES stay the window collection (governance caliber untouched — only
	// MEMBERSHIP is global). The RAW collection map stays untouched:
	// FrequencyResidency, CPUStats.Frequency, topology inference and the
	// ceilings snapshot keep stating sampling FACTS; every reuse is disclosed
	// with its membership source via the window caveat + the per-CPU
	// compute_supply donor fields.
	freqDonors := newClusterFreqDonorResolver(
		resolveClusterFreqDomains(q.CoreTopology, func() map[int][]freqSample { return indexFreqSampleTimelines(idx) }),
		// Treat an unsafe recipient as owning an unavailable lane. donorFor's
		// own-sample guard then refuses to alias a healthy sibling into it.
		func(cpu int) bool { return frequencyIntegrity.frequencyUnsafe(cpu) || len(freqByCPU[cpu]) > 0 })
	freqTimelineFor := func(cpu int) []Event {
		if frequencyIntegrity.frequencyUnsafe(cpu) {
			return nil
		}
		if evs := freqByCPU[cpu]; len(evs) > 0 {
			return evs
		}
		if donor, ok := freqDonors.donorFor(cpu); ok {
			return freqByCPU[donor]
		}
		return nil
	}
	running := map[string]ThreadDuration{}
	pressure := map[int]*cpuPressureAcc{}
	// CMP-10 (§7.4): per-CPU frequency-weighted running accumulation for the
	// compute-supply ledger, fed by the SAME busy segments judged below.
	supplyByCPU := map[int]*cpuSupplyAcc{}
	for cpu, events := range byCPU {
		sort.SliceStable(events, func(i, j int) bool { return events[i].Ts < events[j].Ts })
		// CFR (#75 簇共频): own timeline first; donor timeline only when this
		// CPU has no samples AND the resolved domains name a sampled sibling.
		cpuFreqTimeline := freqTimelineFor(cpu)
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
			// Adversarial review 2026-07-04 F4: THE shared idle predicate —
			// the CMP-10 idle-mismatch pass calls the same schedNextIsIdle,
			// so the busy/idle split and the mismatch integration cannot
			// drift (structural pin: TestSchedNextIsIdlePredicateSharedPin
			// fails if either side re-inlines a diverging copy).
			if schedNextIsIdle(ev) {
				idle += dur
			} else {
				busy += dur
				freq := frequencyAt(cpuFreqTimeline, start)
				key := threadCPUKey(ThreadRef{Comm: ev.NextComm, PID: ev.NextPID}, cpu)
				td := running[key]
				candidateThread := ThreadRef{Comm: ev.NextComm, PID: ev.NextPID}
				if td.Thread.PID == 0 || end > td.EndTs || (end == td.EndTs && threadDisplayLess(candidateThread, td.Thread)) {
					td.Thread = candidateThread
				}
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
				fs := segmentFrequencyStats(cpuFreqTimeline, start, end)
				if fs.known {
					td.freqWeightKHzMs += fs.weightedKHz * dur
					td.freqKnownMs += dur
					td.freqObservedMaxKHz = max(td.freqObservedMaxKHz, fs.observedMaxKHz)
					td.freqInSegmentSamples += fs.inSegmentSamples
				}
				running[key] = td
				sacc := supplyByCPU[cpu]
				if sacc == nil {
					sacc = &cpuSupplyAcc{}
					supplyByCPU[cpu] = sacc
				}
				sacc.runningMs += dur
				if fs.known {
					sacc.freqWeightKHzMs += fs.weightedKHz * dur
					sacc.freqKnownMs += dur
				}
				acc := cpuPressure(pressure, cpu)
				acc.runningMs += dur
				acc.runningEvents++
				highPriority := isHighPriorityForPressure(q.TraceFlavor, ev.NextPrio, td.PriorityClass)
				if highPriority {
					acc.highPriorityRunningMs += dur
				}
				if isSystemOrKernelForPressure(q.TraceFlavor, td.PriorityClass) {
					acc.systemOrKernelRunningMs += dur
				}
				acc.runningSegs = append(acc.runningSegs, pressureSegment{
					thread:        td.Thread,
					cpu:           cpu,
					startTs:       start,
					endTs:         end,
					line:          ev.Line,
					priority:      ev.NextPrio,
					priorityClass: td.PriorityClass,
					highPriority:  highPriority,
				})
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
	// CFC P1 (§7.10 VS-2c 设计): single-point cluster frequency ceilings for
	// this window. Per-CPU observed fmax = the F1-governed residency timeline
	// built just above (computed ONCE here; computeComputeSupplyBalance reads
	// the same map instead of re-scanning residency); the limits rung uses the
	// head-governing + in-window caliber — a deliberate fork from
	// stats.CPUFrequencyLimits' strict in-window display caliber.
	observedFmaxByCPU := windowObservedFmaxByCPU(stats.CPU)
	stats.ClusterFrequencyCeilings = computeWindowClusterFrequencyCeilings(observedFmaxByCPU, coreByCPU, limitTimelineByCPU, q)
	offCPU := computeOffCPUStats(idx, q, freqTimelineFor, pressure)
	stats.RunnableTop, stats.DStateTop, stats.SleepTop, stats.IOWaitTop, stats.CPUPressure = offCPU.runnableTop, offCPU.dstateTop, offCPU.sleepTop, offCPU.iowaitTop, offCPU.pressure
	// 修复轮二 件A (2026-07-13): the D/IO family seats mint from the FULL
	// census (帽外泄漏修根 — tieba 60555 witness: fscache cause seat 4.739 vs
	// provable 7.386, hmfs_get_dnode 0.171 seatless); the capped top lists
	// stay the board display, with the per-lane overflow disclosed
	// explicitly (groups beyond the cap + their total ms — same-source
	// floats, subtraction-free: summed once over the census keys outside
	// the kept set).
	stats.dstateCensus, stats.iowaitCensus = offCPU.dstateCensus, offCPU.iowaitCensus
	stats.DStateTopOverflowGroups, stats.DStateTopOverflowMs = threadDurationCapOverflow(offCPU.dstateCensus, stats.DStateTop)
	stats.IOWaitTopOverflowGroups, stats.IOWaitTopOverflowMs = threadDurationCapOverflow(offCPU.iowaitCensus, stats.IOWaitTop)
	if stats.DStateTopOverflowGroups > 0 {
		stats.Caveats = append(stats.Caveats, fmt.Sprintf("top_d_state shows %d of %d (thread,cpu) groups; %d group(s) totalling %.3fms sit beyond the display cap — the D/IO family seats carry the full per-thread account", len(stats.DStateTop), len(stats.DStateTop)+stats.DStateTopOverflowGroups, stats.DStateTopOverflowGroups, stats.DStateTopOverflowMs))
	}
	if stats.IOWaitTopOverflowGroups > 0 {
		stats.Caveats = append(stats.Caveats, fmt.Sprintf("top_io_wait shows %d of %d (thread,cpu) groups; %d group(s) totalling %.3fms sit beyond the display cap — the D/IO family seats carry the full per-thread account", len(stats.IOWaitTop), len(stats.IOWaitTop)+stats.IOWaitTopOverflowGroups, stats.IOWaitTopOverflowGroups, stats.IOWaitTopOverflowMs))
	}
	// CMP-9 (§7.3): per-CPU runnable-wait density = value / wall window.
	stats.CPUPressure = applyCPUPressureDensity(stats.CPUPressure, queryWindowWallMs(q))
	applyThreadCoreClasses(stats.TopRunning, coreByCPU)
	applyThreadCoreClasses(stats.RunnableTop, coreByCPU)
	applyThreadCoreClasses(stats.SleepTop, coreByCPU)
	applyThreadCoreClasses(stats.DStateTop, coreByCPU)
	applyThreadCoreClasses(stats.IOWaitTop, coreByCPU)
	applyCPUPressureCoreClasses(stats.CPUPressure, coreByCPU)
	stats.CoreTopology = buildCoreClassStats(stats.CPU, stats.CPUPressure, coreByCPU, topologySource)
	if schedulerDurationsSafe {
		stats.CPUConstraints = computeCPUConstraintSummaries(idx, q, coreByCPU, stats.RunnableTop, stats.CPU, 8)
	}
	stats.ThreadCPULoad = computeThreadCPULoad(q, stats.TopRunning, stats.RunnableTop, running, offCPU.runnableCensus, 12)
	windowCatalog := buildThreadCatalog(idx, q)
	// B-3 (§7.11): tidTgidApplied is true when the span-pid vote actually
	// backfilled a TGID for some tid in this window's catalog — the process
	// rollups below then group on soft-derived attribution and must say so
	// (row marker + window caveat). False on any trace with a native TGID
	// column (vote table nil) and on span-less/tie-only windows.
	tidTgidApplied := false
	if derive := idx.derivedTidTgidForQuery(q); derive.enabled() {
		for pid := range windowCatalog {
			if derive.tgidFor(pid) > 0 {
				tidTgidApplied = true
				break
			}
		}
	}
	stats.ProcessCPULoad = computeProcessCPULoad(windowCatalog, stats.ThreadCPULoad, coreByCPU, 8, tidTgidApplied)
	// CMP-8 (§7.1): occupancy-side decomposition from the full running
	// buckets (pre-truncation) — who consumed the CPUs in this window.
	stats.CPUOccupancy = computeCPUOccupancyStats(q, queryWindowWallMs(q), running, pressure, coreByCPU, windowCatalog, stats.CPU, 8, tidTgidApplied)
	// WSR §8 b3 (real_trace_campaign_20260705 §8.1): pid-scoped process-domain
	// census lane over the SAME pre-truncation running buckets — the query's
	// pid/thread finally enters a rollup domain (event admission above has no
	// pid predicate by design; the global faces are population-wide). Additive
	// lane only: TopRunning / ThreadCPULoad / ProcessCPULoad stay byte-identical
	// whether or not a pid is present. Roster cap 8 = the shared up-to-8
	// subject roster convention (PTV5 wire-cap fold bound).
	// ENG audit #44 (§29.25 处置委托 2026-07-10): the census is PID-keyed on
	// both faces (thread count via catalog+observed PIDs, rollup via running
	// buckets) and buildThreadCatalog's TGID/comm merge bridges generations
	// for the creation-edge-less reappeared-after-dead reuse shape. Under a
	// declared incarnation conflict it fails closed with every sibling
	// PID-keyed face (BIO/filesystem/inode/storage gates below) instead of
	// publishing cross-incarnation process attribution beside the very caveat
	// that proves identity is unsafe.
	if identityConflict == nil {
		stats.ProcessDomainCensus = computeProcessDomainCensus(idx, q, running, windowCatalog, coreByCPU, 8, tidTgidApplied)
	}
	if tidTgidApplied {
		stats.Caveats = append(stats.Caveats, tidTgidDerivedCaveat)
	}
	// CMP-10 (§7.4): frequency-weighted delivered compute vs nominal
	// capacity, with the supply-gap decomposition. Nil on unbounded windows.
	// schedCPUs is the precise sched-observation signal (≥1 in-window
	// sched_switch = a byCPU bucket exists): CPUs surfaced only by
	// cpu_frequency samples must not inflate nominal capacity (F2).
	schedCPUs := make(map[int]bool, len(byCPU))
	for cpu := range byCPU {
		schedCPUs[cpu] = true
	}
	// headRunnablePIDs (F3): threads already runnable AT the window head,
	// read from the off-CPU pass's runnable segments (computed above) whose
	// clipped start sits exactly on the window head — pre-window
	// prev_state=R evidence the idle-mismatch event scan cannot see.
	headRunnablePIDs := map[int]bool{}
	if q.TimeStart > 0 {
		for _, acc := range pressure {
			if acc == nil {
				continue
			}
			for _, seg := range acc.runnableSegs {
				if seg.thread.PID > 0 && seg.startTs <= q.TimeStart && seg.endTs > q.TimeStart {
					headRunnablePIDs[seg.thread.PID] = true
				}
			}
		}
	}
	stats.ComputeSupplyBalance = computeComputeSupplyBalance(idx, q, queryWindowWallMs(q), supplyByCPU, schedCPUs, headRunnablePIDs, stats.CPU, coreByCPU, observedFmaxByCPU, freqDonors.donorFor, freqDonors.sourceToken())
	// CFR (#75 簇共频): one deterministic window-level disclosure covering
	// every frequency-weighted face that consumed a donor timeline in this
	// window (busy loop, off-CPU context, compute_supply fmax).
	if caveat := clusterFreqReuseCaveat(freqDonors.usedPairs(), freqDonors.sourceToken(), freqDonors.primeCPUs(), freqDonors.explicitIgnored()); caveat != "" {
		stats.Caveats = append(stats.Caveats, caveat)
	}
	blockPairing := computeBlockIOLatencies(idx, q, 8, durationPairingIntegrities[durationOrderBlockIO])
	stats.IOLatencies = blockPairing.latencies
	stats.Caveats = append(stats.Caveats, blockPairing.caveats...)
	stats.CPUFrequencyLimits = sortedCPUFrequencyLimits(freqLimits, 8)
	stats.SubsystemEvents = sortedSubsystemEvents(subsystems, 12)
	// Direct resource/plugin summaries consume only their own accepted
	// in-window rows. Their complete numeric identity dependencies are the
	// positive event PIDs collected by that same admission call above, so an
	// unrelated task-incarnation boundary must not erase them. Keep each
	// family independent: a reused contributor suppresses that whole family
	// (never a partial cross-generation aggregate), while an empty/PID-less
	// family has no numeric identity dependency. The inode/storage composites
	// below retain the global guard until their multi-input completeness can be
	// propagated end-to-end.
	publishRuntimeResources := func(name string, contributors map[int]bool, items map[string]*RuntimeResourceSummary) []RuntimeResourceSummary {
		// The already-computed global result is a cheap proof that every
		// contributor set is clean on the common path. Re-scan a family only
		// when some lifecycle conflict exists in the window and we must decide
		// whether that conflict actually intersects this family.
		if identityConflict != nil {
			if conflict := threadIncarnationConflictForPIDSet(idx, q, contributors); conflict != nil {
				stats.Caveats = append(stats.Caveats, "thread_identity_"+name+"_resource_fail_closed=true; "+conflict.reason()+"; "+name+" resource summaries are omitted because a contributing PID spans task incarnations")
				return nil
			}
		}
		return sortedRuntimeResources(items, 8)
	}
	stats.BIOResources = publishRuntimeResources("bio", bioResourceContributorPIDs, bioResources)
	stats.FilesystemResources = publishRuntimeResources("filesystem", filesystemResourceContributorPIDs, filesystemResources)
	stats.PageFaultResources = publishRuntimeResources("page_fault", pageFaultResourceContributorPIDs, pageFaultResources)

	publishPluginEvents := func(name string, contributors map[int]bool, items map[string]*TracePluginSummary) []TracePluginSummary {
		if identityConflict != nil {
			if conflict := threadIncarnationConflictForPIDSet(idx, q, contributors); conflict != nil {
				stats.Caveats = append(stats.Caveats, "thread_identity_"+name+"_plugin_fail_closed=true; "+conflict.reason()+"; "+name+" plugin summaries are omitted because a contributing PID spans task incarnations")
				return nil
			}
		}
		return sortedTracePluginSummaries(items, 8)
	}
	stats.AbilityEvents = publishPluginEvents("ability_monitor", abilityContributorPIDs, abilityEvents)
	stats.XPowerEvents = publishPluginEvents("xpower", xpowerContributorPIDs, xpowerEvents)
	stats.HiSystemEvents = publishPluginEvents("hi_sysevent", hiSystemContributorPIDs, hiSystemEvents)

	if identityConflict == nil {
		// INODE (§28.6): fold the FULL accumulator maps BEFORE the top-8
		// truncations below — the whole-window (dev,inode) carrier must never be
		// built on truncated inputs (the block_io_by_inode second-aggregation
		// lesson).
		stats.TopIOInodes = computeTopIOInodes(fileIO, pageCache, topIOInodeGroupLimit)
		stats.FileIOByInode = sortedFileIOSummaries(fileIO, 8)
		stats.PageCacheByInode = sortedPageCacheSummaries(pageCache, 8)
		storageLatencies, storagePairingCaveats := computeStorageLatencyByLayer(idx, q, blockPairing.summaries, 8, durationPairingIntegrities[durationOrderStorage])
		stats.StorageLatencyByLayer = append(stats.StorageLatencyByLayer, storageLatencies...)
		stats.Caveats = append(stats.Caveats, storagePairingCaveats...)
	} else {
		stats.Caveats = append(stats.Caveats, "thread_identity_resource_fail_closed=true; PID-keyed inode/file-IO/page-cache/storage composite aggregates are omitted because the selected window crosses a task-incarnation boundary")
	}
	if schedulerDurationsSafe {
		// CR-3 修复轮 P2 (2026-07-12): fold the FULL accumulator BEFORE the
		// top-8 truncation (INODE §28.6 precedent) — the P10 residual count
		// must never be a second aggregation over a truncated inventory.
		stats.blockedReasonFullByPID = foldBlockedReasonFullByPID(blockedReasons)
		stats.BlockedReasons = topBlockedReasons(blockedReasons, 8)
	}
	var traceMarkCaveats []string
	if schedulerDurationsSafe {
		traceSpans, traceCounters, candidateCaveats := computeTraceMarks(idx, q, 8)
		stats.TraceCounters = traceCounters
		if failure := durationFailures[durationOrderTraceSpan]; failure != nil {
			stats.Caveats = append(stats.Caveats, durationOrderFailClosedCaveat(failure, "trace_spans/trace_mark_categories/async_file_work"))
		} else {
			stats.TraceSpans = traceSpans
			traceMarkCaveats = candidateCaveats
		}
		counterDeltas, counterQuality := computeCounterDeltas(idx, q, 8)
		stats.CounterQuality = counterQuality
		if failure := durationFailures[durationOrderTraceCounter]; failure != nil {
			stats.Caveats = append(stats.Caveats, durationOrderFailClosedCaveat(failure, "counter_deltas"))
		} else {
			stats.CounterDeltas = counterDeltas
		}
		if quality := stats.CounterQuality; quality != nil && (quality.InvalidRows > 0 || quality.NonNumericRows > 0 || quality.DerivedInvalidSeries > 0 || quality.SeriesBudgetExceeded) {
			stats.Caveats = append(stats.Caveats, fmt.Sprintf(
				"trace_counter_quality_degraded=true; invalid_rows=%d non_numeric_rows=%d derived_invalid_series=%d suppressed_series=%d series_budget=%d budget_exceeded=%t overflow_rows=%d; malformed/non-finite/over-budget series are omitted from counter_deltas and retained in trace_counters/counter_quality",
				quality.InvalidRows, quality.NonNumericRows, quality.DerivedInvalidSeries, quality.SuppressedSeries,
				quality.SeriesBudget, quality.SeriesBudgetExceeded, quality.OverflowRows))
		}
	}
	stats.Caveats = append(stats.Caveats, traceMarkCaveats...)
	// Android G/H track spans and I/N instants use logical payload ownership,
	// not emitter-thread ownership. Publish them on their isolated typed faces;
	// their own source/generation/order gates fail closed independently.
	stats.TraceTrackSpans, stats.TraceInstants, traceMarkCaveats = computeTraceTrackMarks(idx, q, 8)
	stats.Caveats = append(stats.Caveats, traceMarkCaveats...)
	stats.TraceMarkCategories = computeTraceMarkCategories(stats.TraceSpans, 8)
	stats.AsyncFileWork = computeAsyncFileWorkSummaries(stats.TraceSpans, 8)
	var tracePairingCaveats []string
	if failure := durationFailures[durationOrderIRQ]; failure != nil {
		stats.Caveats = append(stats.Caveats, durationOrderFailClosedCaveat(failure, "irq_bursts/irq_activity"))
	} else {
		stats.IRQBursts = computeIRQBursts(idx, q, 8)
		stats.IRQActivity, tracePairingCaveats = computeInterruptActivity(idx, q, EventIRQ, coreByCPU, 8)
		stats.Caveats = append(stats.Caveats, tracePairingCaveats...)
	}
	if failure := durationFailures[durationOrderSoftIRQ]; failure != nil {
		stats.Caveats = append(stats.Caveats, durationOrderFailClosedCaveat(failure, "softirq_activity"))
	} else {
		stats.SoftIRQActivity, tracePairingCaveats = computeInterruptActivity(idx, q, EventSoftIRQ, coreByCPU, 8)
		stats.Caveats = append(stats.Caveats, tracePairingCaveats...)
	}
	if failure := durationFailures[durationOrderIPI]; failure != nil {
		stats.Caveats = append(stats.Caveats, durationOrderFailClosedCaveat(failure, "ipi_activity"))
	} else {
		stats.IPIActivity, tracePairingCaveats = computeInterruptActivity(idx, q, EventIPI, coreByCPU, 8)
		stats.Caveats = append(stats.Caveats, tracePairingCaveats...)
	}
	workqueueIdentityConflict := threadIncarnationConflictForPIDSet(idx, q, workqueueContributorPIDs)
	if workqueueIdentityConflict != nil {
		stats.Caveats = append(stats.Caveats, "thread_identity_workqueue_fail_closed=true; "+workqueueIdentityConflict.reason()+"; workqueue activity is omitted because a contributing PID spans task incarnations")
	}
	if workqueueIdentityConflict == nil {
		stats.WorkqueueActivity, tracePairingCaveats = computeWorkqueueActivity(idx, q, 8, durationPairingIntegrities[durationOrderWorkqueue])
		stats.Caveats = append(stats.Caveats, tracePairingCaveats...)
	}

	dmaIdentityConflict := threadIncarnationConflictForPIDSet(idx, q, dmaFenceContributorPIDs)
	if dmaIdentityConflict != nil {
		stats.Caveats = append(stats.Caveats, "thread_identity_dma_fence_fail_closed=true; "+dmaIdentityConflict.reason()+"; DMA fence activity is omitted because a contributing PID spans task incarnations")
	}
	if dmaIdentityConflict == nil {
		stats.DMAFenceActivity, tracePairingCaveats = computeDMAFenceActivity(idx, q, 8, durationPairingIntegrities[durationOrderDMAFence])
		stats.Caveats = append(stats.Caveats, tracePairingCaveats...)
	}
	if schedulerDurationsSafe {
		stats.SchedStatAccounting = computeSchedStatAccounting(idx, q, 8)
	}
	stats.MemoryKinds = computeMemoryKinds(idx, q, 8)
	stats.ThreadDrifts = detectThreadDrifts(idx, q, 8)
	for _, drift := range stats.ThreadDrifts {
		if drift.Caveat != "" {
			stats.Caveats = append(stats.Caveats, drift.Caveat)
		}
	}
	stats.ComputeSupply = computeSupplySummaries(stats, 8)
	stats.StateChurn = enrichStateChurnWithCPUPressure(computeStateChurnSummaries(idx, q, 8), stats.CPUPressure)
	stats.StateDrilldownPlan, stats.IdleWholeWindowSleepers = buildStateDrilldownPlanForTarget(stats, 12, q.PID, q.Thread)
	latency := buildSchedulerLatencyStatsFromStats(idx, q, stats)
	stats.RunnableContext = computeRunnableContextSummaries(latency.Items, stats.ThreadCPULoad, stats.ProcessCPULoad, stats.CPUConstraints, 8)
	stats.IOPressureSummary = computeIOPressureSummary(stats)
	stats.BlockIOByInode = computeBlockIOByInode(stats, 8)
	stats.IOBurstEpisodes = computeIOBurstEpisodes(stats, 8)
	stats.SupplyPressureSummary = computeSupplyPressureSummary(idx, q, stats, 8)
	if schedulerDurationsSafe {
		stats.PerfSamples = computePerfContext(idx, q, 8)
	}
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
		pf := ev.PerfFields
		if pf == nil {
			pf = &PerfFields{}
		}
		period := pf.Period
		if period <= 0 {
			period = 1
		}
		weightUnit := perfSampleWeightUnit(ev)
		ctx.SampleCount++
		ctx.TotalPeriod += period
		quality.add(ev, period)
		thread := perfSampleThread(ev)
		cpu := -1
		if executionCPU, ok := perfSampleOnCPUExecutionCPU(ev); ok {
			cpu = executionCPU
		}
		example := perfSampleExample(ev)
		addPerfHotspot(bySymbol, firstNonEmpty(pf.Symbol, pf.IP, "unknown"), PerfHotspot{
			Symbol:              pf.Symbol,
			DSO:                 pf.DSO,
			Event:               pf.EventName,
			WeightUnit:          weightUnit,
			Source:              pf.Source,
			SymbolizationStatus: pf.SymbolizationStatus,
		}, thread, cpu, ev.Line, period, example, &ctx.TotalPeriod)
		addPerfHotspot(byDSO, firstNonEmpty(pf.DSO, "unknown"), PerfHotspot{
			DSO:                 pf.DSO,
			Event:               pf.EventName,
			WeightUnit:          weightUnit,
			Source:              pf.Source,
			SymbolizationStatus: pf.SymbolizationStatus,
		}, thread, cpu, ev.Line, period, example, &ctx.TotalPeriod)
		addPerfHotspot(byCallchain, firstNonEmpty(pf.Callchain, pf.Symbol, pf.IP, "unknown"), PerfHotspot{
			Symbol:              pf.Symbol,
			DSO:                 pf.DSO,
			Callchain:           pf.Callchain,
			Event:               pf.EventName,
			WeightUnit:          weightUnit,
			Source:              pf.Source,
			SymbolizationStatus: pf.SymbolizationStatus,
		}, thread, cpu, ev.Line, period, example, &ctx.TotalPeriod)
		addPerfHotspot(byEvent, firstNonEmpty(pf.EventName, "unknown"), PerfHotspot{
			Event:               pf.EventName,
			WeightUnit:          weightUnit,
			Source:              pf.Source,
			SymbolizationStatus: pf.SymbolizationStatus,
		}, thread, cpu, ev.Line, period, example, &ctx.TotalPeriod)
		if perfThreadRefHasRosterIdentity(thread) {
			addPerfThread(byThread, thread, cpu, ev.Line, period, example, &ctx.TotalPeriod)
		}
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

func perfContextForExecutionThread(idx *Index, q Query, thread ThreadRef, start, end float64, max int) *PerfContext {
	if thread.PID <= 0 && strings.TrimSpace(thread.Comm) == "" {
		return nil
	}
	sub := queryForPerfContextWindow(q, start, end)
	return computePerfContextFiltered(idx, sub, max, func(ev Event) bool {
		return perfSampleMatchesExecutionThread(ev, thread)
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

func perfContextForExecutionThreads(idx *Index, q Query, threads map[int]ThreadRef, max int) *PerfContext {
	if len(threads) == 0 {
		return nil
	}
	return computePerfContextFiltered(idx, q, max, func(ev Event) bool {
		for _, thread := range threads {
			if perfSampleMatchesExecutionThread(ev, thread) {
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
		cpu, ok := perfSampleOnCPUExecutionCPU(ev)
		return ok && cpus[cpu]
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
	if !perfSampleHasTypedThreadIdentity(ev) {
		return false
	}
	pf := ev.PerfFields
	if pf == nil {
		pf = &PerfFields{}
	}
	if thread.PID > 0 {
		// ThreadRef.PID is a TID.  pf.PID/ev.TGID are process identities and
		// comm is not an authorized fallback once a TID is known.
		return pf.TID == thread.PID || ev.PID == thread.PID
	}
	if thread.Comm != "" {
		if strings.EqualFold(thread.Comm, pf.Comm) || strings.EqualFold(thread.Comm, ev.Comm) {
			return true
		}
	}
	return false
}

func perfSampleThread(ev Event) ThreadRef {
	if !perfSampleHasTypedThreadIdentity(ev) {
		return ThreadRef{}
	}
	pf := ev.PerfFields
	if pf == nil {
		pf = &PerfFields{}
	}
	return ThreadRef{
		Comm: firstNonEmpty(pf.Comm, ev.Comm),
		PID:  firstNonZero(pf.TID, ev.PID),
		TGID: firstNonZero(pf.PID, ev.TGID),
	}
}

func perfThreadRefHasRosterIdentity(thread ThreadRef) bool {
	return thread.PID > 0 || strings.TrimSpace(thread.Comm) != ""
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
	pf := ev.PerfFields
	if pf == nil {
		pf = &PerfFields{}
	}
	parts := []string{}
	if pf.Symbol != "" {
		parts = append(parts, "symbol="+pf.Symbol)
	}
	if pf.DSO != "" {
		parts = append(parts, "dso="+pf.DSO)
	}
	if pf.EventName != "" {
		parts = append(parts, "event="+pf.EventName)
	}
	if pf.Period > 0 {
		parts = append(parts, fmt.Sprintf("sample_weight=%d", pf.Period))
	}
	if unit := perfSampleWeightUnit(ev); unit != "" {
		parts = append(parts, "weight_unit="+unit)
	}
	if pf.IP != "" {
		parts = append(parts, "ip="+pf.IP)
	}
	if pf.Addr != "" && pf.Addr != pf.IP {
		parts = append(parts, "addr="+pf.Addr)
	}
	if pf.SampleID != "" {
		parts = append(parts, "sample_id="+pf.SampleID)
	}
	if pf.StreamID != "" {
		parts = append(parts, "stream_id="+pf.StreamID)
	}
	if pf.RawWeight > 0 {
		parts = append(parts, fmt.Sprintf("perf_weight=%d", pf.RawWeight))
	}
	if pf.DataSrc != "" {
		parts = append(parts, "data_src="+pf.DataSrc)
	}
	if pf.Transaction != "" {
		parts = append(parts, "transaction="+pf.Transaction)
	}
	if pf.PhysAddr != "" {
		parts = append(parts, "phys_addr="+pf.PhysAddr)
	}
	if pf.CGroupID != "" {
		parts = append(parts, "cgroup_id="+pf.CGroupID)
	}
	if pf.DataPageSize > 0 {
		parts = append(parts, fmt.Sprintf("data_page_size=%d", pf.DataPageSize))
	}
	if pf.CodePageSize > 0 {
		parts = append(parts, fmt.Sprintf("code_page_size=%d", pf.CodePageSize))
	}
	if pf.RawSize > 0 {
		parts = append(parts, fmt.Sprintf("raw_size=%d", pf.RawSize))
	}
	if pf.BranchCount > 0 {
		parts = append(parts, fmt.Sprintf("branch_count=%d", pf.BranchCount))
	}
	if pf.UserRegsABI != "" {
		parts = append(parts, "user_regs_abi="+pf.UserRegsABI)
	}
	if pf.UserRegsCount > 0 {
		parts = append(parts, fmt.Sprintf("user_regs_count=%d", pf.UserRegsCount))
	}
	if pf.UserStackSize > 0 {
		parts = append(parts, fmt.Sprintf("user_stack_size=%d", pf.UserStackSize))
	}
	if pf.AuxSize > 0 {
		parts = append(parts, fmt.Sprintf("aux_size=%d", pf.AuxSize))
	}
	if pf.Source != "" {
		parts = append(parts, "source="+pf.Source)
	}
	if pf.ThreadIdentityKnown != nil {
		parts = append(parts, fmt.Sprintf("thread_identity_known=%t", *pf.ThreadIdentityKnown))
	}
	if pf.Resolution != "" {
		parts = append(parts, "resolution="+pf.Resolution)
	}
	if pf.LifecycleUnverified != nil {
		parts = append(parts, fmt.Sprintf("lifecycle_unverified=%t", *pf.LifecycleUnverified))
	}
	if pf.SourcePID > 0 {
		parts = append(parts, fmt.Sprintf("perf_source_pid=%d", pf.SourcePID))
	}
	if pf.SourceTID > 0 {
		parts = append(parts, fmt.Sprintf("perf_source_tid=%d", pf.SourceTID))
	}
	if pf.SourceComm != "" {
		parts = append(parts, "perf_source_comm="+pf.SourceComm)
	}
	if pf.SymbolizationStatus != "" {
		parts = append(parts, "symbolization_status="+pf.SymbolizationStatus)
	}
	if pf.SampleKind != "" {
		parts = append(parts, "sample_kind="+pf.SampleKind)
	}
	if pf.SampleKindSource != "" {
		parts = append(parts, "sample_kind_source="+pf.SampleKindSource)
	}
	if pf.CallchainStatus != "" {
		parts = append(parts, "callchain_status="+pf.CallchainStatus)
	}
	if pf.ClockConfidence != "" {
		parts = append(parts, "clock_confidence="+pf.ClockConfidence)
	}
	if pf.CPUKnown != nil {
		parts = append(parts, fmt.Sprintf("cpu_known=%t", *pf.CPUKnown))
	}
	if pf.Callchain != "" {
		parts = append(parts, "callchain="+pf.Callchain)
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
	pf := ev.PerfFields
	if pf == nil {
		pf = &PerfFields{}
	}
	addPerfValueCount(acc.sources, firstNonEmpty(pf.Source, "unknown"), period)
	addPerfValueCount(acc.symbolizationStatuses, firstNonEmpty(pf.SymbolizationStatus, "unknown"), period)
	addPerfValueCount(acc.sampleKinds, firstNonEmpty(pf.SampleKind, "unknown"), period)
	addPerfValueCount(acc.weightUnits, perfSampleWeightUnit(ev), period)
	addPerfValueCount(acc.clocks, firstNonEmpty(pf.Clock, "unknown"), period)
	addPerfValueCount(acc.clockConfidences, firstNonEmpty(pf.ClockConfidence, "unknown"), period)
	addPerfValueCount(acc.callchainStatuses, firstNonEmpty(pf.CallchainStatus, "unknown"), period)
	if perfSampleHasKnownCPU(ev) {
		acc.cpuKnownCount++
	} else {
		acc.cpuUnknownCount++
	}
	if perfCallchainKnownForQuality(pf.CallchainStatus) {
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
	pf := ev.PerfFields
	if pf == nil {
		pf = &PerfFields{}
	}
	event := strings.ToLower(strings.TrimSpace(pf.EventName))
	sampleKind := strings.ToLower(strings.TrimSpace(pf.SampleKind))
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
	if perfThreadRefHasRosterIdentity(thread) {
		label := threadLabel(thread)
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
	start, end, count, contributorPIDs := perfTimelineWindow(idx, q)
	res := PerfTimelineResult{Window: TimeWindow{StartTs: start, EndTs: end}}
	if conflict := threadIncarnationConflictForPIDSet(idx, q, contributorPIDs); conflict != nil {
		res.Caveats = append(res.Caveats, "thread_identity_fail_closed=true; "+conflict.reason()+"; PID-keyed perf timeline buckets are omitted because the selected window spans task incarnations")
		return res
	}
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
		// ENG audit #46 (§29.25 处置委托 2026-07-10): single shared admission
		// predicate with perfTimelineWindow — the contributor-PID guard is
		// sound only if every bucketed sample went through the same filter, so
		// the two loops must never drift apart.
		if !perfTimelineAdmits(ev, q) {
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
		pf := ev.PerfFields
		if pf == nil {
			pf = &PerfFields{}
		}
		period := pf.Period
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
		if pf.Symbol != "" {
			acc.symbols[pf.Symbol] += period
		}
		if pf.DSO != "" {
			acc.dsos[pf.DSO] += period
		}
		if pf.EventName != "" {
			acc.events[pf.EventName] += period
		}
		if thread := perfSampleThread(ev); perfThreadRefHasRosterIdentity(thread) {
			acc.threadSet[threadLabel(thread)] = thread
		}
		if cpu, ok := perfSampleOnCPUExecutionCPU(ev); ok {
			acc.cpuSet[cpu] = true
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

// perfTimelineAdmits is the ONE admission predicate for perf timeline
// samples. ENG audit #46 (§29.25 处置委托 2026-07-10): BuildPerfTimeline's
// bucket loop and perfTimelineWindow's contributor loop used to carry
// byte-identical copy-pasted filters; the incarnation guard consumes the
// contributor set, so any one-sided edit would have created samples that are
// bucketed but never guard-checked. Both loops now call this predicate.
func perfTimelineAdmits(ev Event, q Query) bool {
	if ev.Type != EventPerfSample || !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
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

func perfTimelineWindow(idx *Index, q Query) (float64, float64, int, map[int]bool) {
	start, end := q.TimeStart, q.TimeEnd
	count := 0
	contributorPIDs := map[int]bool{}
	for _, ev := range idx.Events {
		if !perfTimelineAdmits(ev, q) {
			continue
		}
		count++
		if pid := perfSampleThread(ev).PID; pid > 0 {
			contributorPIDs[pid] = true
		}
		if start == 0 || ev.Ts < start {
			start = ev.Ts
		}
		if end == 0 || ev.Ts > end {
			end = ev.Ts
		}
	}
	// ENG audit #46 residual gap: a sample can be admitted for the pid
	// selector via the row-header ev.PID while its own thread identity
	// (pf.TID>0≠q.PID) is what enters the contributor set — leaving the
	// addressed subject itself unguarded against incarnation conflicts. When
	// q.PID selected samples, the subject's identity is load-bearing for the
	// published timeline and always joins the guarded set.
	if q.PID > 0 && count > 0 {
		contributorPIDs[q.PID] = true
	}
	if end < start {
		end = start
	}
	if end == start && count > 0 {
		end = start + 0.001
	}
	return start, end, count, contributorPIDs
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
	runnableWaitMs          float64
	runnableEvents          int
	runningMs               float64
	runningEvents           int
	highPriorityRunningMs   float64
	systemOrKernelRunningMs float64
	runnable                map[string]ThreadDuration
	running                 map[string]ThreadDuration
	runningSegs             []pressureSegment
	runnableSegs            []pressureSegment
}

// pressureSegment is one contiguous scheduling interval (running or runnable)
// of a single thread on a single CPU. Kept raw so competition claims can be
// backed by time-displacement evidence — the target waiting runnable WHILE the
// competitor runs — instead of window-total co-residency (methodology audit
// §7.30.2 R5g).
type pressureSegment struct {
	thread        ThreadRef
	cpu           int
	startTs       float64
	endTs         float64
	line          int
	priority      int
	priorityClass string
	highPriority  bool
}

// timeInterval is a [start, end) window in trace seconds.
type timeInterval struct {
	start float64
	end   float64
}

type priorityPressureOverlap struct {
	highPriorityMs                float64
	systemOrKernelMs              float64
	systemOrKernelCompetitorCount int
	competitors                   []ThreadDuration
}

// overlapCompetitorsForIntervals keeps only the running time that overlapped
// the target's runnable interval(s) on one CPU (§7.30.2 R5g displacement
// evidence). It returns the high-priority-only overlapped total plus the
// per-competitor overlapped durations (any priority; DurationMs is the
// overlapped portion, not the window running total) sorted by overlap
// descending. Serial hand-offs where a peer only runs outside the target's
// waits contribute nothing. runningSegs must be sorted by start and disjoint
// (single-CPU running segments are).
func overlapCompetitorsForIntervals(runningSegs []pressureSegment, target ThreadRef, intervals []timeInterval, maxCompetitors int) priorityPressureOverlap {
	if len(runningSegs) == 0 || len(intervals) == 0 {
		return priorityPressureOverlap{}
	}
	highPriorityMs := 0.0
	systemOrKernelMs := 0.0
	systemOrKernelCompetitors := map[string]bool{}
	competitors := map[string]ThreadDuration{}
	for _, interval := range intervals {
		if interval.end <= interval.start {
			continue
		}
		first := sort.Search(len(runningSegs), func(i int) bool { return runningSegs[i].endTs > interval.start })
		for i := first; i < len(runningSegs) && runningSegs[i].startTs < interval.end; i++ {
			seg := runningSegs[i]
			if sameThreadRef(seg.thread, target) {
				continue
			}
			start, end, ok := overlapTimeWindow(seg.startTs, seg.endTs, interval.start, interval.end)
			if !ok {
				continue
			}
			overlapMs := (end - start) * 1000
			if seg.highPriority {
				highPriorityMs += overlapMs
			}
			key := threadKey(seg.thread)
			if seg.priorityClass == "system_or_kernel" {
				systemOrKernelMs += overlapMs
				systemOrKernelCompetitors[key] = true
			}
			td := competitors[key]
			td.Thread = seg.thread
			td.DurationMs += overlapMs
			td.CPU = seg.cpu
			td.Priority = seg.priority
			td.PriorityClass = seg.priorityClass
			if td.StartTs == 0 || start < td.StartTs {
				td.StartTs = start
			}
			if end > td.EndTs {
				td.EndTs = end
			}
			if td.LineStart == 0 || (seg.line > 0 && seg.line < td.LineStart) {
				td.LineStart = seg.line
			}
			if seg.line > td.LineEnd {
				td.LineEnd = seg.line
			}
			competitors[key] = td
		}
	}
	out := make([]ThreadDuration, 0, len(competitors))
	for _, td := range competitors {
		out = append(out, td)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DurationMs != out[j].DurationMs {
			return out[i].DurationMs > out[j].DurationMs
		}
		return out[i].LineStart < out[j].LineStart
	})
	if maxCompetitors > 0 && len(out) > maxCompetitors {
		out = out[:maxCompetitors]
	}
	return priorityPressureOverlap{
		highPriorityMs:                highPriorityMs,
		systemOrKernelMs:              systemOrKernelMs,
		systemOrKernelCompetitorCount: len(systemOrKernelCompetitors),
		competitors:                   out,
	}
}

// sortPressureSegments orders segments by start time (line as tiebreak) so
// overlap consumers can binary-search them.
func sortPressureSegments(segs []pressureSegment) {
	sort.SliceStable(segs, func(i, j int) bool {
		if segs[i].startTs != segs[j].startTs {
			return segs[i].startTs < segs[j].startTs
		}
		return segs[i].line < segs[j].line
	})
}

// runnableIntervalsForThread extracts the target's runnable wait intervals on
// this CPU from the raw pressure segments.
func runnableIntervalsForThread(pressure CPUPressureStats, target ThreadRef) []timeInterval {
	var out []timeInterval
	for _, seg := range pressure.runnableSegments {
		if sameThreadRef(seg.thread, target) {
			out = append(out, timeInterval{start: seg.startTs, end: seg.endTs})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].start < out[j].start })
	return out
}

// runnableDisplacementOverlap reports how much running time from OTHER threads
// overlapped the target's runnable waits on this CPU: the high-priority
// overlapped total plus the per-competitor overlapped list (§7.30.2 R5g).
// Zero when the target never waited runnable on this CPU — co-residency
// without displacement is not competition.
func runnableDisplacementOverlap(pressure CPUPressureStats, target ThreadRef, maxCompetitors int) priorityPressureOverlap {
	return overlapCompetitorsForIntervals(pressure.runningSegments, target, runnableIntervalsForThread(pressure, target), maxCompetitors)
}

// cpuDisplacementAggregate computes the per-CPU displacement aggregate: how
// much high-priority running time overlapped at least one OTHER thread's
// runnable wait on this CPU, plus the per-competitor overlapped durations
// (any priority). Running segments on one CPU are disjoint, so each instant
// contributes at most once (§7.30.2 R5g). Both inputs must be sorted by start.
func cpuDisplacementAggregate(runningSegs, runnableSegs []pressureSegment, maxCompetitors int) priorityPressureOverlap {
	if len(runningSegs) == 0 || len(runnableSegs) == 0 {
		return priorityPressureOverlap{}
	}
	type boundary struct {
		ts    float64
		delta int
		key   string
	}
	bounds := make([]boundary, 0, len(runnableSegs)*2)
	for _, seg := range runnableSegs {
		if seg.endTs <= seg.startTs {
			continue
		}
		key := threadKey(seg.thread)
		bounds = append(bounds, boundary{ts: seg.startTs, delta: 1, key: key})
		bounds = append(bounds, boundary{ts: seg.endTs, delta: -1, key: key})
	}
	sort.SliceStable(bounds, func(i, j int) bool { return bounds[i].ts < bounds[j].ts })
	counts := map[string]int{}
	total := 0
	b := 0
	apply := func(ts float64) {
		for b < len(bounds) && bounds[b].ts <= ts {
			counts[bounds[b].key] += bounds[b].delta
			total += bounds[b].delta
			b++
		}
	}
	highPriorityMs := 0.0
	systemOrKernelMs := 0.0
	systemOrKernelCompetitors := map[string]bool{}
	competitors := map[string]ThreadDuration{}
	for _, seg := range runningSegs {
		if seg.endTs <= seg.startTs {
			continue
		}
		apply(seg.startTs)
		segKey := threadKey(seg.thread)
		cursor := seg.startTs
		flush := func(to float64) {
			if to <= cursor {
				return
			}
			if total-counts[segKey] > 0 {
				overlapMs := (to - cursor) * 1000
				if seg.highPriority {
					highPriorityMs += overlapMs
				}
				if seg.priorityClass == "system_or_kernel" {
					systemOrKernelMs += overlapMs
					systemOrKernelCompetitors[segKey] = true
				}
				td := competitors[segKey]
				td.Thread = seg.thread
				td.DurationMs += overlapMs
				td.CPU = seg.cpu
				td.Priority = seg.priority
				td.PriorityClass = seg.priorityClass
				if td.StartTs == 0 || cursor < td.StartTs {
					td.StartTs = cursor
				}
				if to > td.EndTs {
					td.EndTs = to
				}
				if td.LineStart == 0 || (seg.line > 0 && seg.line < td.LineStart) {
					td.LineStart = seg.line
				}
				if seg.line > td.LineEnd {
					td.LineEnd = seg.line
				}
				competitors[segKey] = td
			}
			cursor = to
		}
		for b < len(bounds) && bounds[b].ts < seg.endTs {
			flush(bounds[b].ts)
			counts[bounds[b].key] += bounds[b].delta
			total += bounds[b].delta
			b++
		}
		flush(seg.endTs)
	}
	out := make([]ThreadDuration, 0, len(competitors))
	for _, td := range competitors {
		out = append(out, td)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DurationMs != out[j].DurationMs {
			return out[i].DurationMs > out[j].DurationMs
		}
		return out[i].LineStart < out[j].LineStart
	})
	if maxCompetitors > 0 && len(out) > maxCompetitors {
		out = out[:maxCompetitors]
	}
	return priorityPressureOverlap{
		highPriorityMs:                highPriorityMs,
		systemOrKernelMs:              systemOrKernelMs,
		systemOrKernelCompetitorCount: len(systemOrKernelCompetitors),
		competitors:                   out,
	}
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
	key := threadCPUKey(thread, cpu)
	td := bucket[key]
	firstSegment := td.DurationMs <= 0
	if td.Thread.PID == 0 || endTs > td.EndTs || (endTs == td.EndTs && threadDisplayLess(thread, td.Thread)) {
		td.Thread = thread
	}
	td.DurationMs += dur
	td.CPU = cpu
	if freq > 0 {
		td.Frequency = freq
	}
	td.Priority = priority
	td.PriorityClass = priorityClass
	if firstSegment || startTs < td.StartTs {
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

// computeOffCPUStats takes the CFR (#75) cluster-shared frequency accessor
// (own timeline first, explicit-topology donor fallback) instead of the raw
// map, so the off-CPU frequency context reads the SAME caliber as the busy
// loop and the two faces cannot fork on donor-covered CPUs.
func computeOffCPUStats(idx *Index, q Query, freqTimelineFor func(int) []Event, pressure map[int]*cpuPressureAcc) offCPUStatsResult {
	if idx == nil {
		return offCPUStatsResult{}
	}
	if schedulerStateIntegrityFailureForQuery(idx, q, 0) != nil {
		return offCPUStatsResult{}
	}
	if threadIncarnationConflictForQuery(idx, q, 0) != nil {
		return offCPUStatsResult{}
	}
	blockedReasons := blockedReasonsByPID(idx, q)
	open := map[int]offCPUStart{}
	runnable := map[string]ThreadDuration{}
	sleep := map[string]ThreadDuration{}
	dstate := map[string]ThreadDuration{}
	iowait := map[string]ThreadDuration{}
	head := schedulerHeadForQuery(idx, q)
	headOwnsPrefix := idx.Windowed && q.TimeStart > 0 && q.LineStart == 0 && q.LineEnd == 0 && head != nil
	headComplete := headOwnsPrefix && head.Complete
	if headComplete {
		for _, state := range schedulerHeadSortedThreads(head) {
			switch state.State {
			case StateRunnable, StateSSleep, StateDSleep, StateIOWait:
				open[state.Thread.PID] = offCPUStart{
					thread:        state.Thread,
					state:         state.State,
					ts:            state.StartTs,
					line:          state.Line,
					cpu:           state.CPU,
					priority:      state.Priority,
					priorityClass: classifyTracePriority(q.TraceFlavor, state.Priority),
				}
			}
		}
	}
	// §29.50.5 证明分区 (v5 P1 批 件②, 2026-07-13; 修复轮 h1 ∿ 回归): the
	// D/IO segments accumulate a per-PROVEN-wait-object SLICE inventory on
	// their (thread,cpu) ledger group (逐片段证明门 — a fragment joins a cause
	// slice only when ITS OWN typed sched_blocked_reason marker names the
	// semantic symbol; "" = the unproven slice). The GROUP KEY itself is
	// untouched — keying the ledger on the cause inflated the capped
	// DStateTop/IOWaitTop entry counts and downstream wire caps evicted
	// unrelated rows (the pacing ∿ seat). The runnable/sleep buckets never
	// track slices (hypothesis lanes never partition — 假设永不并). Segment
	// sums are unchanged; §29.19 sum_disjoint semantics carry through per
	// slice.
	addDurationCause := func(bucket map[string]ThreadDuration, start offCPUStart, endTs float64, endLine int, cause string, trackSlices bool) {
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
		freq := frequencyAt(freqTimelineFor(start.cpu), startTs)
		key := threadCPUKey(start.thread, start.cpu)
		td := bucket[key]
		if trackSlices {
			if td.causeSlices == nil {
				td.causeSlices = map[string]offCPUCauseSlice{}
			}
			slice := td.causeSlices[cause]
			slice.durMs += dur
			slice.segCount++
			if slice.segMinMs == 0 || dur < slice.segMinMs {
				slice.segMinMs = dur
			}
			if dur > slice.segMaxMs {
				slice.segMaxMs = dur
			}
			if slice.startTs == 0 || startTs < slice.startTs {
				slice.startTs = startTs
			}
			if endTs > slice.endTs {
				slice.endTs = endTs
			}
			if slice.lineStart == 0 || (start.line > 0 && start.line < slice.lineStart) {
				slice.lineStart = start.line
			}
			if line := firstPositive(endLine, start.line); line > slice.lineEnd {
				slice.lineEnd = line
			}
			td.causeSlices[cause] = slice
		}
		firstSegment := td.DurationMs <= 0
		if td.Thread.PID == 0 || endTs > td.EndTs || (endTs == td.EndTs && threadDisplayLess(start.thread, td.Thread)) {
			td.Thread = start.thread
		}
		td.DurationMs += dur
		// F-1 (修复轮, 2026-07-12): true per-segment stats ride the
		// aggregation so folds can speak segment truth (never group sums
		// dressed as 段).
		td.segCount++
		if td.segMinMs == 0 || dur < td.segMinMs {
			td.segMinMs = dur
		}
		if dur > td.segMaxMs {
			td.segMaxMs = dur
		}
		td.CPU = start.cpu
		if freq > 0 {
			td.Frequency = freq
		}
		td.Priority = start.priority
		td.PriorityClass = start.priorityClass
		if firstSegment || startTs < td.StartTs {
			td.StartTs = startTs
		}
		if endTs > td.EndTs {
			td.EndTs = endTs
		}
		if td.LineStart == 0 {
			td.LineStart = start.line
		}
		td.LineEnd = firstPositive(endLine, start.line)
		if fs := segmentFrequencyStats(freqTimelineFor(start.cpu), startTs, endTs); fs.known {
			td.freqWeightKHzMs += fs.weightedKHz * dur
			td.freqKnownMs += dur
			td.freqObservedMaxKHz = max(td.freqObservedMaxKHz, fs.observedMaxKHz)
			td.freqInSegmentSamples += fs.inSegmentSamples
		}
		bucket[key] = td
		if start.state == StateRunnable {
			acc := cpuPressure(pressure, start.cpu)
			acc.runnableWaitMs += dur
			acc.runnableEvents++
			acc.runnableSegs = append(acc.runnableSegs, pressureSegment{
				thread:        start.thread,
				cpu:           start.cpu,
				startTs:       startTs,
				endTs:         endTs,
				line:          firstPositive(endLine, start.line),
				priority:      start.priority,
				priorityClass: start.priorityClass,
				highPriority:  isHighPriorityForPressure(q.TraceFlavor, start.priority, start.priorityClass),
			})
			accumulateThreadDuration(acc.runnable, start.thread, dur, start.cpu, freq, startTs, endTs, firstPositive(endLine, start.line), start.priority, start.priorityClass)
		}
	}
	addDuration := func(bucket map[string]ThreadDuration, start offCPUStart, endTs float64, endLine int) {
		addDurationCause(bucket, start, endTs, endLine, "", false)
	}
	// DSTATE-REFINE arm a (件③): stamp the D-ledger coverage verdict onto the
	// aggregated duration (same key/clamp derivation as addDuration; a
	// zero-length clamped segment that addDuration dropped is skipped the
	// same way).
	markDStateCoverage := func(start offCPUStart, endTs float64, marked bool, caller string) {
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
		key := threadCPUKey(start.thread, start.cpu)
		td := dstate[key]
		td.dFamilySegments++
		if marked {
			td.dFamilyNonIOMarked++
		}
		if caller != "" && caller != "unknown" && len(td.dFamilyCallers) < 4 {
			seen := false
			for _, c := range td.dFamilyCallers {
				if c == caller {
					seen = true
					break
				}
			}
			if !seen {
				td.dFamilyCallers = append(td.dFamilyCallers, caller)
			}
		}
		dstate[key] = td
	}
	visit := func(ev Event) {
		if !eventLineInWindow(ev, q) {
			return
		}
		if headOwnsPrefix && ev.Ts < q.TimeStart {
			return
		}
		if pid, destCPU, comm, migrated := schedMigrationTarget(ev); migrated {
			if !timeInWindow(ev.Ts, q) {
				return
			}
			start, exists := open[pid]
			if !exists || start.state != StateRunnable {
				return
			}
			// A runnable migration ends CPU attribution on the old lane at the
			// exact event timestamp and immediately continues the same state on
			// the destination. At time_start the old clamped segment is zero and
			// addDuration intentionally emits nothing.
			addDuration(runnable, start, ev.Ts, ev.Line)
			start.ts = ev.Ts
			start.line = ev.Line
			start.cpu = destCPU
			if start.thread.Comm == "" && comm != "" {
				start.thread.Comm = comm
			}
			open[pid] = start
			return
		}
		if ev.Type == EventSchedWakeup || ev.Type == EventSchedWaking {
			if ev.WakeePID <= 0 || !timeInWindow(ev.Ts, q) {
				return
			}
			if start, ok := open[ev.WakeePID]; ok {
				if !schedWakeupStartsNewIncarnation(ev) && start.state == StateRunnable {
					return
				}
				switch start.state {
				case StateRunnable:
					addDuration(runnable, start, ev.Ts, ev.Line)
				case StateSSleep:
					addDuration(sleep, start, ev.Ts, ev.Line)
				case StateDSleep, StateIOWait:
					if io, marked, caller := offCPUDStateVerdict(start, ev.Ts, blockedReasons); io {
						addDurationCause(iowait, start, ev.Ts, ev.Line, offCPUCauseSymbol(caller), true)
					} else {
						cause := ""
						if marked {
							cause = offCPUCauseSymbol(caller)
						}
						addDurationCause(dstate, start, ev.Ts, ev.Line, cause, true)
						markDStateCoverage(start, ev.Ts, marked, caller)
					}
				}
			}
			targetCPU, _ := eventTargetCPU(ev)
			open[ev.WakeePID] = offCPUStart{
				thread:        ThreadRef{Comm: ev.WakeeComm, PID: ev.WakeePID},
				state:         StateRunnable,
				ts:            ev.Ts,
				line:          ev.Line,
				cpu:           targetCPU,
				priority:      eventWakeePriorityForHardUse(ev),
				priorityClass: classifyTracePriority(q.TraceFlavor, eventWakeePriorityForHardUse(ev)),
			}
			return
		}
		if ev.Type != EventSchedSwitch {
			return
		}
		if ev.NextPID > 0 {
			if start, ok := open[ev.NextPID]; ok {
				switch start.state {
				case StateRunnable:
					addDuration(runnable, start, ev.Ts, ev.Line)
				case StateSSleep:
					addDuration(sleep, start, ev.Ts, ev.Line)
				case StateDSleep, StateIOWait:
					if io, marked, caller := offCPUDStateVerdict(start, ev.Ts, blockedReasons); io {
						addDurationCause(iowait, start, ev.Ts, ev.Line, offCPUCauseSymbol(caller), true)
					} else {
						cause := ""
						if marked {
							cause = offCPUCauseSymbol(caller)
						}
						addDurationCause(dstate, start, ev.Ts, ev.Line, cause, true)
						markDStateCoverage(start, ev.Ts, marked, caller)
					}
				}
				delete(open, ev.NextPID)
			}
		}
		if ev.PrevPID > 0 {
			state := stateFromPrevState(ev.PrevState)
			if state == StateRunnable || state == StateSSleep || state == StateDSleep || state == StateIOWait {
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
	visitEventsInTimestampOrder(idx, nil, false, visit)
	if q.TimeEnd > 0 {
		for _, start := range open {
			switch start.state {
			case StateRunnable:
				addDuration(runnable, start, q.TimeEnd, 0)
			case StateSSleep:
				addDuration(sleep, start, q.TimeEnd, 0)
			case StateDSleep, StateIOWait:
				if io, marked, caller := offCPUDStateVerdict(start, q.TimeEnd, blockedReasons); io {
					addDurationCause(iowait, start, q.TimeEnd, 0, offCPUCauseSymbol(caller), true)
				} else {
					cause := ""
					if marked {
						cause = offCPUCauseSymbol(caller)
					}
					addDurationCause(dstate, start, q.TimeEnd, 0, cause, true)
					markDStateCoverage(start, q.TimeEnd, marked, caller)
				}
			}
		}
	}
	// ENG-1 (复核冷读 F2-1, 2026-07-12): the FULL pre-cap runnable census
	// rides back beside the capped display lists so thread_cpu_load totals
	// can be true full-window sums (see computeThreadCPULoad).
	return offCPUStatsResult{
		runnableTop:    topThreadDurations(runnable, 8),
		dstateTop:      topThreadDurations(dstate, 8),
		sleepTop:       topThreadDurations(sleep, 8),
		iowaitTop:      topThreadDurations(iowait, 8),
		pressure:       buildCPUPressureStats(pressure, 8),
		runnableCensus: runnable,
		// 修复轮二 件A (ENG-1 补完, 2026-07-13): the FULL pre-cap D/IO census
		// rides back beside the capped display lists so the formal family
		// seats can carry true full-window accounts (see
		// rootCauseDIOStateFamilyItems) and the board face can disclose the
		// per-lane cap overflow honestly.
		dstateCensus: dstate,
		iowaitCensus: iowait,
	}
}

// offCPUStatsResult carries computeOffCPUStats' capped display lists plus
// the full pre-cap runnable census (ENG-1: totals never truncate; caps only
// limit display rows).
type offCPUStatsResult struct {
	runnableTop    []ThreadDuration
	dstateTop      []ThreadDuration
	sleepTop       []ThreadDuration
	iowaitTop      []ThreadDuration
	pressure       []CPUPressureStats
	runnableCensus map[string]ThreadDuration
	dstateCensus   map[string]ThreadDuration
	iowaitCensus   map[string]ThreadDuration
}

func offCPUStateIsIOWait(start offCPUStart, endTs float64, blockedReasons map[int][]Event) bool {
	io, _, _ := offCPUDStateVerdict(start, endTs, blockedReasons)
	return io
}

// offCPUDStateVerdict — DSTATE-REFINE arm a (CAL-1 件③, 2026-07-12): the
// D-family segment verdict in one lookup — whether the segment is
// iowait-proven (rides the IOWaitTop ledger), whether a sched_blocked_reason
// marker COVERED the segment at all (coverage proof for the refined
// 「D-state」 word — absence of a marker proves nothing), and the marker's
// semantic caller symbol (the 等待对象族 disclosure; hex/opaque callers were
// already collapsed to "unknown" by blockedReasonSemanticCaller at parse).
// offCPUCauseSymbol reduces a sched_blocked_reason caller to its semantic
// wait-object symbol (§29.50.5 逐片段证明门, v5 P1 批 件②): the symbol before
// '+' — same symbol at different offsets is ONE wait object (the offset/
// module detail stays on the raw evidence lines). ""/unknown → no proof
// (absence never guesses). Single mint point for the partition key AND the
// cause-seat 等待对象 word.
func offCPUCauseSymbol(caller string) string {
	c := strings.TrimSpace(caller)
	if c == "" || c == "unknown" {
		return ""
	}
	if i := strings.IndexByte(c, '+'); i > 0 {
		c = c[:i]
	}
	return c
}

func offCPUDStateVerdict(start offCPUStart, endTs float64, blockedReasons map[int][]Event) (isIOWait, marked bool, caller string) {
	if start.state == StateIOWait {
		return true, true, ""
	}
	if start.state != StateDSleep {
		return false, false, ""
	}
	// The kernel emits sched_blocked_reason immediately AFTER the wakeup that
	// ends the D segment — donghu witness: same-ts (keva rows) OR trailing by
	// ~1µs (CompThread rows 13762.793064→.793065). The lookup end widens by
	// the ONE established wakeup-match tolerance (wakeupMatchToleranceSec,
	// 5µs — target_window_state_account.go is the sibling consumer), so a
	// trailing marker classifies its own segment instead of silently
	// vanishing (a missed iowait=1 marker left the segment on the D ledger).
	reason := blockedReasonForInterval(blockedReasons, start.thread, start.ts, endTs+wakeupMatchToleranceSec)
	if reason == nil {
		return false, false, ""
	}
	return reason.IOWait > 0, true, strings.TrimSpace(reason.Reason)
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
	if failure := schedulerStateIntegrityFailureForQuery(idx, q, 0); failure != nil {
		res.Caveats = append(res.Caveats, "scheduler_duration_fail_closed=true; "+failure.reason()+"; runnable latency intervals are omitted")
		res.Caveats = append(res.Caveats, stats.Caveats...)
		return res
	}
	if conflict := threadIncarnationConflictForQuery(idx, q, 0); conflict != nil {
		res.Caveats = append(res.Caveats, "thread_identity_fail_closed=true; "+conflict.reason()+"; runnable latency rows are omitted because the selected window spans task incarnations")
		res.Caveats = append(res.Caveats, stats.Caveats...)
		return res
	}
	var target ThreadRef
	if q.PID > 0 || q.Thread != "" || q.ThreadInput != "" {
		resolution := resolveThreadSelection(idx, q)
		target = resolution.Thread
		res.Target = target
		if resolution.Ambiguous {
			res.Caveats = append(res.Caveats, threadResolutionCaveat(idx, q))
			return res
		}
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
	frequencyIntegrity := frequencyOrderIntegrityForQuery(idx, q)
	freqByCPU := map[int][]Event{}
	for _, ev := range idx.Events {
		// CFC P0: same shared admission predicate as ComputeWindowStats — the
		// two window-face collections must stay member-identical.
		if eventLineInWindow(ev, q) && isPerCPUFrequencySample(ev) {
			if q.TimeEnd == 0 || ev.Ts <= q.TimeEnd {
				cpu := eventCPUForStats(ev)
				if !frequencyIntegrity.frequencyUnsafe(cpu) {
					freqByCPU[cpu] = append(freqByCPU[cpu], ev)
				}
			}
		}
	}
	sortFrequencyTimeline(freqByCPU)
	// CFR (#75 簇共频): same explicit-topology donor fallback as
	// ComputeWindowStats (single authority: cluster_freq_share.go) so the
	// latency rows' frequency context cannot fork from the window face.
	// Reuse is disclosed via the result caveat appended after the scan.
	// CAP-3 (§29.11): derivation over the Index-global stream, mirroring
	// ComputeWindowStats — membership global, values window-collected.
	schedFreqDonors := newClusterFreqDonorResolver(
		resolveClusterFreqDomains(q.CoreTopology, func() map[int][]freqSample { return indexFreqSampleTimelines(idx) }),
		func(cpu int) bool { return frequencyIntegrity.frequencyUnsafe(cpu) || len(freqByCPU[cpu]) > 0 })
	schedFreqTimelineFor := func(cpu int) []Event {
		if frequencyIntegrity.frequencyUnsafe(cpu) {
			return nil
		}
		if evs := freqByCPU[cpu]; len(evs) > 0 {
			return evs
		}
		if donor, ok := schedFreqDonors.donorFor(cpu); ok {
			return freqByCPU[donor]
		}
		return nil
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
	head := schedulerHeadForQuery(idx, q)
	headOwnsPrefix := idx.Windowed && q.TimeStart > 0 && q.LineStart == 0 && q.LineEnd == 0 && head != nil
	headComplete := headOwnsPrefix && head.Complete
	if headComplete {
		for _, state := range schedulerHeadSortedThreads(head) {
			if state.State != StateRunnable {
				continue
			}
			open[state.Thread.PID] = startInfo{
				thread:        state.Thread,
				ts:            state.StartTs,
				line:          state.Line,
				cpu:           state.CPU,
				priority:      state.Priority,
				priorityClass: classifyTracePriority(q.TraceFlavor, state.Priority),
			}
		}
	}
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
		freq := frequencyAt(schedFreqTimelineFor(start.cpu), startTs)
		freqStats := segmentFrequencyStats(schedFreqTimelineFor(start.cpu), startTs, endTs)
		overlap := overlapCompetitorsForIntervals(p.runningSegments, start.thread, []timeInterval{{start: startTs, end: endTs}}, 8)
		item := SchedulerLatencyItem{
			Thread:                         start.thread,
			StartTs:                        startTs,
			EndTs:                          endTs,
			DurationMs:                     duration,
			CPU:                            start.cpu,
			CoreClass:                      cpu.CoreClass,
			Frequency:                      freq,
			WeightedFrequency:              int(math.Round(freqStats.weightedKHz)),
			ObservedMaxFrequency:           freqStats.observedMaxKHz,
			Priority:                       start.priority,
			PriorityClass:                  start.priorityClass,
			StartLine:                      start.line,
			EndLine:                        firstPositive(endLine, start.line),
			SameCPUBusyMs:                  cpu.BusyMs,
			SameCPUIdleMs:                  cpu.IdleMs,
			OtherCPUIdleMs:                 otherIdle,
			HighPriorityRunningMs:          p.HighPriorityRunningMs,
			HighPriorityRunningOverlapMs:   overlap.highPriorityMs,
			SystemOrKernelRunningMs:        p.SystemOrKernelRunningMs,
			SystemOrKernelRunningOverlapMs: overlap.systemOrKernelMs,
			SystemOrKernelCompetitorCount:  overlap.systemOrKernelCompetitorCount,
			SameCPUTopRunning:              overlap.competitors,
		}
		if freqStats.known && freqStats.inSegmentSamples == 0 {
			item.FrequencySample = FrequencySampleNearestFallback
		}
		item.Summary = fmt.Sprintf("%s waited runnable for %.3fms on cpu=%d", threadLabel(item.Thread), item.DurationMs, item.CPU)
		if item.CoreClass != "" {
			item.Summary = fmt.Sprintf("%s core_class=%s", item.Summary, item.CoreClass)
		}
		if item.Frequency > 0 {
			item.Summary = fmt.Sprintf("%s freq=%dkHz", item.Summary, item.Frequency)
		}
		if item.WeightedFrequency > 0 {
			item.Summary = fmt.Sprintf("%s weighted_freq=%dkHz observed_max_freq=%dkHz", item.Summary, item.WeightedFrequency, item.ObservedMaxFrequency)
		}
		if item.FrequencySample != "" {
			item.Summary = fmt.Sprintf("%s frequency_sample=%s", item.Summary, item.FrequencySample)
		}
		if item.HighPriorityRunningOverlapMs > 0 {
			item.Summary = fmt.Sprintf("%s high_prio_overlap=%.3fms", item.Summary, item.HighPriorityRunningOverlapMs)
		}
		if item.SystemOrKernelRunningOverlapMs > 0 {
			item.Summary = fmt.Sprintf("%s system_or_kernel_overlap=%.3fms system_or_kernel_competitors=%d",
				item.Summary, item.SystemOrKernelRunningOverlapMs, item.SystemOrKernelCompetitorCount)
		}
		if item.OtherCPUIdleMs > 0 {
			item.Summary = fmt.Sprintf("%s other_cpu_idle=%.3fms", item.Summary, item.OtherCPUIdleMs)
		}
		res.Items = append(res.Items, item)
	}
	visitLatency := func(ev Event) {
		if headOwnsPrefix && ev.Ts < q.TimeStart {
			return
		}
		if pid, destCPU, comm, migrated := schedMigrationTarget(ev); migrated {
			if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
				return
			}
			start, exists := open[pid]
			if !exists {
				return
			}
			// Emit the old-CPU wait fragment through the same close authority,
			// then continue the still-runnable wait on the destination CPU.
			closeWait(pid, ev.Ts, ev.Line)
			start.ts = ev.Ts
			start.line = ev.Line
			start.cpu = destCPU
			if start.thread.Comm == "" && comm != "" {
				start.thread.Comm = comm
			}
			open[pid] = start
			return
		}
		if !eventLineInWindow(ev, q) || ev.Type != EventSchedSwitch {
			if ev.Type == EventSchedWakeup || ev.Type == EventSchedWaking {
				if ev.WakeePID > 0 {
					if target.PID > 0 || target.Comm != "" {
						if !threadMatches(target, ev.WakeePID, ev.WakeeComm) {
							return
						}
					}
					if q.TimeEnd > 0 && ev.Ts > q.TimeEnd {
						return
					}
					if q.TimeStart > 0 && ev.Ts < q.TimeStart {
						return
					}
					if existing, ok := open[ev.WakeePID]; schedWakeupStartsNewIncarnation(ev) || !ok || ev.Ts < existing.ts {
						targetCPU, _ := eventTargetCPU(ev)
						open[ev.WakeePID] = startInfo{
							thread:        catalogThreadRef(catalog, ev.WakeePID, ev.WakeeComm),
							ts:            ev.Ts,
							line:          ev.Line,
							cpu:           targetCPU,
							priority:      eventWakeePriorityForHardUse(ev),
							priorityClass: classifyTracePriority(q.TraceFlavor, eventWakeePriorityForHardUse(ev)),
						}
					}
				}
			}
			return
		}
		if q.TimeEnd > 0 && ev.Ts > q.TimeEnd {
			return
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
	visitEventsInTimestampOrder(idx, nil, false, visitLatency)
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
	limit := ViewCapacityFor("scheduler_latency_stats").ClampLimit(q.Limit)
	if len(res.Items) > limit {
		last := res.Items[limit-1]
		res.Compactions = append(res.Compactions, ViewCompaction{
			View:            "scheduler_latency_stats",
			Dimension:       CompactionDimensionIntervals,
			Total:           len(res.Items),
			Emitted:         limit,
			LastEmittedTs:   last.EndTs,
			LastEmittedLine: last.EndLine,
		})
		res.Caveats = append(res.Caveats, fmt.Sprintf("scheduler_latency_stats compacted from %d to %d runnable wait interval(s)", len(res.Items), limit))
		res.Items = res.Items[:limit]
	}
	if res.Count == 0 {
		res.Caveats = append(res.Caveats, "no runnable wait intervals matched the selected filters")
	}
	// CFR (#75 簇共频): disclose donor consumption of THIS face's scan; the
	// window-stats face's own disclosure rides in via stats.Caveats below —
	// skip an exact duplicate line.
	if caveat := clusterFreqReuseCaveat(schedFreqDonors.usedPairs(), schedFreqDonors.sourceToken(), schedFreqDonors.primeCPUs(), schedFreqDonors.explicitIgnored()); caveat != "" && !caveatListContains(stats.Caveats, caveat) {
		res.Caveats = append(res.Caveats, caveat)
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
		sortPressureSegments(acc.runningSegs)
		sortPressureSegments(acc.runnableSegs)
		overlap := cpuDisplacementAggregate(acc.runningSegs, acc.runnableSegs, max)
		out = append(out, CPUPressureStats{
			CPU:                            cpu,
			RunnableWaitMs:                 acc.runnableWaitMs,
			RunnableEvents:                 acc.runnableEvents,
			RunningMs:                      acc.runningMs,
			HighPriorityRunningMs:          acc.highPriorityRunningMs,
			HighPriorityRunningOverlapMs:   overlap.highPriorityMs,
			SystemOrKernelRunningMs:        acc.systemOrKernelRunningMs,
			SystemOrKernelRunningOverlapMs: overlap.systemOrKernelMs,
			SystemOrKernelCompetitorCount:  overlap.systemOrKernelCompetitorCount,
			OverlapCompetitors:             overlap.competitors,
			TopRunnable:                    topThreadDurations(acc.runnable, max),
			TopRunning:                     topThreadDurations(acc.running, max),
			runningSegments:                acc.runningSegs,
			runnableSegments:               acc.runnableSegs,
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
	// Window-head scheduler state is the governing generation when a bounded
	// index legitimately omitted the creation/rename row from its retained
	// event slice. Seed its own comm/TGID before in-window enrichment.
	if q.TimeStart > 0 && q.LineStart == 0 && q.LineEnd == 0 {
		if head := schedulerHeadForQuery(idx, q); head != nil && head.Complete {
			for pid, state := range head.Threads {
				if pid > 0 {
					ref := state.Thread
					ref.PID = pid
					out[pid] = ref
				}
			}
		}
	}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		if schedWakeupStartsNewIncarnation(ev) && ev.WakeePID > 0 {
			// Exact lifecycle reset: old comm/TGID must not survive into the new
			// occupant even when both are present in a padded index.
			out[ev.WakeePID] = ThreadRef{PID: ev.WakeePID, Comm: ev.WakeeComm}
		}
		add(ev.PID, ev.Comm, ev.TGID)
		add(ev.PrevPID, ev.PrevComm, 0)
		add(ev.NextPID, ev.NextComm, 0)
		add(ev.WakeePID, ev.WakeeComm, 0)
		if cf := ev.ConstraintFields; cf != nil {
			add(cf.PID, cf.Comm, 0)
		}
	}
	// B-3 (§7.11): on TGID-column-less traces (hmtrace) every native TGID
	// above is 0 — backfill the catalog from the per-index trace_mark
	// span-pid vote so the per-process display rollups
	// (computeProcessCPULoad, occupancy TopProcesses, runnable-context
	// same-process matching) can group threads again. Single wiring point:
	// every process-attribution consumer converges on this catalog via
	// processRefForThread/catalogThreadRef. Soft display enrichment only —
	// the vote table is nil whenever the index has any native TGID, so the
	// with-TGID path is untouched.
	if derive := idx.derivedTidTgidForQuery(q); derive.enabled() {
		usedTgids := map[int]bool{}
		for pid, ref := range out {
			if ref.TGID == 0 {
				if tgid := derive.tgidFor(pid); tgid > 0 {
					ref.TGID = tgid
					out[pid] = ref
				}
			}
			if out[pid].TGID > 0 {
				usedTgids[out[pid].TGID] = true
			}
		}
		// Display-name backfill for derived process rows whose main thread
		// has no catalogued line in this scope: the label comes from the
		// self-registration span name (B|tgid|process_name emitted by
		// tid==tgid). Display only — never a matching key.
		for tgid := range usedTgids {
			ref := out[tgid]
			if ref.PID == 0 {
				ref.PID = tgid
			}
			if ref.TGID == 0 {
				if tg := derive.tgidFor(tgid); tg > 0 {
					ref.TGID = tg
				}
			}
			if strings.TrimSpace(ref.Comm) == "" {
				if name := derive.commFor(tgid); name != "" {
					ref.Comm = name
				}
			}
			out[tgid] = ref
		}
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

func threadCPUKey(thread ThreadRef, cpu int) string {
	return fmt.Sprintf("%s/cpu:%d", threadKey(thread), cpu)
}

// threadDisplayLess is only a deterministic tie-breaker for soft display
// metadata. Numeric TID remains the sole hard identity whenever it is known.
func threadDisplayLess(left, right ThreadRef) bool {
	leftComm := strings.TrimSpace(left.Comm)
	rightComm := strings.TrimSpace(right.Comm)
	if leftComm != rightComm {
		if leftComm == "" {
			return false
		}
		if rightComm == "" {
			return true
		}
		return leftComm < rightComm
	}
	return left.TGID > 0 && (right.TGID <= 0 || left.TGID < right.TGID)
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
			cf := ev.ConstraintFields
			if cf == nil {
				cf = &ConstraintFields{}
			}
			thread := catalogThreadRef(catalog, cf.PID, cf.Comm)
			if thread.PID <= 0 && thread.Comm == "" {
				continue
			}
			acc := ensure(thread)
			acc.item.ConstraintCount++
			acc.item.Kind = firstNonEmpty(acc.item.Kind, ev.Name, string(ev.Type))
			acc.item.Policy = firstNonEmpty(acc.item.Policy, cf.Policy)
			acc.item.CPUSet = firstNonEmpty(acc.item.CPUSet, cf.CPUSetName)
			acc.item.CGroup = firstNonEmpty(acc.item.CGroup, ev.CGroup)
			addAllowed(acc, cf.Allowed)
			if cf.DestCPUSet {
				acc.item.ObservedCPU = cf.DestCPU
				acc.item.ObservedCPUKnown = true
				acc.item.ObservedCoreClass = coreByCPU[cf.DestCPU]
			} else if cf.CPUValid {
				acc.item.ObservedCPU = cf.CPU
				acc.item.ObservedCPUKnown = true
				acc.item.ObservedCoreClass = coreByCPU[cf.CPU]
			}
			if cf.OrigCPUSet && cf.DestCPUSet {
				acc.item.MigrationCount++
			} else if cf.DestCPUSet {
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

// computeThreadCPULoad builds the per-thread load rollup. EVOLUTION RECORD
// (ENG-1, 复核冷读 F2-1, 2026-07-12): the totals used to be summed from the
// CAPPED display lists (global top-8 per-(thread,cpu) slices), so a thread
// whose running fragmented across many CPUs published a partial sum as its
// total — the donghu witness reported running=132.041ms (cpu12 96.081 +
// cpu4 35.960, the only two surviving slices) while the full-window all-core
// truth is 157.248ms. Totals now sum from the FULL pre-cap censuses
// (runningCensus / runnableCensus); the capped lists decide only the display
// ROSTER (帽只限逐核行数, never the arithmetic). Display fields (dominant
// CPU / core class / priority) also read the full census — for a rostered
// thread its largest slice survived the cap, so those fields are unchanged.
func computeThreadCPULoad(q Query, running []ThreadDuration, runnable []ThreadDuration, runningCensus, runnableCensus map[string]ThreadDuration, max int) []ThreadCPULoadSummary {
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
			if isSystemOrKernelForPressure(q.TraceFlavor, td.PriorityClass) {
				acc.item.SystemOrKernelRunningMs += td.DurationMs
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
	// Roster membership: exactly the capped display lists (unchanged cap
	// semantics). Arithmetic: the full censuses via the rostered-thread
	// filter below; the legacy capped-list fallback keeps hand-built test
	// fixtures (which pass nil censuses) byte-identical.
	roster := map[string]bool{}
	for _, td := range running {
		if td.DurationMs > 0 && (td.Thread.PID > 0 || td.Thread.Comm != "") {
			roster[threadKey(td.Thread)] = true
		}
	}
	for _, td := range runnable {
		if td.DurationMs > 0 && (td.Thread.PID > 0 || td.Thread.Comm != "") {
			roster[threadKey(td.Thread)] = true
		}
	}
	addCensus := func(census map[string]ThreadDuration, capped []ThreadDuration, state string) {
		if census == nil {
			for _, td := range capped {
				add(td, state)
			}
			return
		}
		keys := make([]string, 0, len(census))
		for key := range census {
			keys = append(keys, key)
		}
		sort.Strings(keys) // deterministic accumulation order
		for _, key := range keys {
			td := census[key]
			if roster[threadKey(td.Thread)] {
				add(td, state)
			}
		}
	}
	addCensus(runningCensus, running, "running")
	addCensus(runnableCensus, runnable, "runnable")
	out := make([]ThreadCPULoadSummary, 0, len(accs))
	for _, acc := range accs {
		item := acc.item
		item.LineStart = acc.lineStart
		item.LineEnd = acc.lineEnd
		item.Summary = fmt.Sprintf("%s thread load running=%.3fms runnable=%.3fms high_prio_running=%.3fms system_or_kernel_running=%.3fms cpu=%d",
			threadLabel(item.Thread), item.RunningMs, item.RunnableWaitMs, item.HighPriorityRunningMs, item.SystemOrKernelRunningMs, item.CPU)
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
	derived   bool
}

// computeProcessCPULoad rolls thread loads up to tgid-level processes.
// catalog is the pid→process catalog built once per window (buildThreadCatalog)
// and shared with the CMP-8 occupancy rollup. derivedAttribution is the B-3
// (§7.11) window flag: when true, catalog TGIDs were soft-derived from the
// trace_mark span-pid vote and every row grouped through such a TGID carries
// the self-explaining marker in its Summary.
func computeProcessCPULoad(catalog map[int]ThreadRef, loads []ThreadCPULoadSummary, coreByCPU map[int]string, max int, derivedAttribution bool) []ProcessCPULoadSummary {
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
		if derivedAttribution && catalog[load.Thread.PID].TGID > 0 {
			acc.derived = true
		}
		if load.Thread.PID > 0 && !acc.threadSet[load.Thread.PID] {
			acc.threadSet[load.Thread.PID] = true
			acc.item.ThreadCount++
		}
		acc.item.RunningMs += load.RunningMs
		acc.item.RunnableWaitMs += load.RunnableWaitMs
		acc.item.HighPriorityRunningMs += load.HighPriorityRunningMs
		acc.item.SystemOrKernelRunningMs += load.SystemOrKernelRunningMs
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
		if acc.derived {
			acc.item.Summary += tidTgidDerivedRowMarker
		}
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
			Thread:                         item.Thread,
			RunnableWaitMs:                 item.DurationMs,
			CPU:                            item.CPU,
			CoreClass:                      item.CoreClass,
			Frequency:                      item.Frequency,
			Priority:                       item.Priority,
			PriorityClass:                  item.PriorityClass,
			SameCPUBusyMs:                  item.SameCPUBusyMs,
			SameCPUIdleMs:                  item.SameCPUIdleMs,
			OtherCPUIdleMs:                 item.OtherCPUIdleMs,
			HighPriorityRunningMs:          item.HighPriorityRunningMs,
			HighPriorityRunningOverlapMs:   item.HighPriorityRunningOverlapMs,
			SystemOrKernelRunningMs:        item.SystemOrKernelRunningMs,
			SystemOrKernelRunningOverlapMs: item.SystemOrKernelRunningOverlapMs,
			SystemOrKernelCompetitorCount:  item.SystemOrKernelCompetitorCount,
			SameCPUTopRunning:              item.SameCPUTopRunning,
			LineStart:                      item.StartLine,
			LineEnd:                        item.EndLine,
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
	// R5g (§7.30.2): the high-priority pressure term reads only the running
	// time that overlapped this thread's wait — window-total high-priority
	// running is background and must not create a competition verdict.
	if ctx.HighPriorityRunningOverlapMs > 0 || ctx.SameCPUBusyMs > ctx.SameCPUIdleMs {
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
	parts = append(parts, fmt.Sprintf("same_cpu_busy=%.3fms same_cpu_idle=%.3fms other_cpu_idle=%.3fms high_prio_overlap=%.3fms high_prio_running_window=%.3fms system_or_kernel_running_window=%.3fms system_or_kernel_overlap=%.3fms system_or_kernel_competitors=%d",
		ctx.SameCPUBusyMs, ctx.SameCPUIdleMs, ctx.OtherCPUIdleMs, ctx.HighPriorityRunningOverlapMs, ctx.HighPriorityRunningMs,
		ctx.SystemOrKernelRunningMs, ctx.SystemOrKernelRunningOverlapMs, ctx.SystemOrKernelCompetitorCount))
	if len(ctx.SameCPUTopRunning) > 0 {
		parts = append(parts, fmt.Sprintf("same_cpu_top_running=%s overlap=%.3fms", threadLabel(ctx.SameCPUTopRunning[0].Thread), ctx.SameCPUTopRunning[0].DurationMs))
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
	add := func(td ThreadDuration, state string) {
		if td.DurationMs <= 0 {
			return
		}
		cpu := cpus[td.CPU]
		p := pressure[td.CPU]
		weighted := td.weightedFrequencyKHz()
		// R5g (§7.30.2): the pressure term only counts high-priority running
		// that overlapped THIS thread's runnable waits on this CPU.
		overlap := runnableDisplacementOverlap(p, td.Thread, 8)
		verdict, conf := computeSupplyVerdict(td.DurationMs, weighted, td.freqObservedMaxKHz, overlap.highPriorityMs, cpu)
		frequencySample := ""
		if weighted > 0 && td.freqInSegmentSamples == 0 {
			frequencySample = FrequencySampleNearestFallback
		}
		summary := fmt.Sprintf("%s %s for %.3fms on cpu=%d", threadLabel(td.Thread), state, td.DurationMs, td.CPU)
		if cpu.CoreClass != "" {
			summary = fmt.Sprintf("%s core_class=%s", summary, cpu.CoreClass)
		}
		if td.Frequency > 0 {
			summary = fmt.Sprintf("%s freq=%dkHz", summary, td.Frequency)
		}
		if weighted > 0 {
			summary = fmt.Sprintf("%s weighted_freq=%dkHz observed_max_freq=%dkHz", summary, weighted, td.freqObservedMaxKHz)
		}
		if frequencySample != "" {
			summary = fmt.Sprintf("%s frequency_sample=%s", summary, frequencySample)
		}
		if cpu.BusyMs > 0 || cpu.IdleMs > 0 {
			summary = fmt.Sprintf("%s busy=%.3fms idle=%.3fms", summary, cpu.BusyMs, cpu.IdleMs)
		}
		if overlap.highPriorityMs > 0 {
			summary = fmt.Sprintf("%s high_prio_overlap=%.3fms", summary, overlap.highPriorityMs)
		}
		if overlap.systemOrKernelMs > 0 {
			summary = fmt.Sprintf("%s system_or_kernel_overlap=%.3fms system_or_kernel_competitors=%d", summary, overlap.systemOrKernelMs, overlap.systemOrKernelCompetitorCount)
		}
		out = append(out, ComputeSupplySummary{
			Thread:                         td.Thread,
			State:                          state,
			CPU:                            td.CPU,
			CoreClass:                      cpu.CoreClass,
			DurationMs:                     td.DurationMs,
			Frequency:                      td.Frequency,
			WeightedFrequency:              weighted,
			ObservedMaxFrequency:           td.freqObservedMaxKHz,
			FrequencySample:                frequencySample,
			CPUBusyMs:                      cpu.BusyMs,
			CPUIdleMs:                      cpu.IdleMs,
			RunnableWaitMs:                 p.RunnableWaitMs,
			HighPriorityRunningMs:          p.HighPriorityRunningMs,
			HighPriorityRunningOverlapMs:   overlap.highPriorityMs,
			SystemOrKernelRunningMs:        p.SystemOrKernelRunningMs,
			SystemOrKernelRunningOverlapMs: overlap.systemOrKernelMs,
			SystemOrKernelCompetitorCount:  overlap.systemOrKernelCompetitorCount,
			Verdict:                        verdict,
			Confidence:                     conf,
			LineStart:                      td.LineStart,
			LineEnd:                        td.LineEnd,
			Summary:                        summary + " verdict=" + verdict,
		})
	}
	// TSH review F4: the ComputeSupplySummary.State words minted here are
	// dominant-lane wire tokens — computeSupplyDominantState passes them
	// through verbatim into RootCauseRankItem.DominantState (root_cause_rank
	// compute-supply lane), so they are typed constants, never raw literals.
	// The mint set is pinned by the production-witness test.
	for _, td := range stats.RunnableTop {
		add(td, string(StateRunnable))
	}
	for _, td := range stats.TopRunning {
		add(td, string(StateRunning))
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

// computeSupplyCauseSubjectAllowed is the RN-15 (§7.4 demand/supply
// separation ruling, §7.9) hard guard for the compute_supply causal token:
// the delivery-side ledger (supply ratio, low-frequency loss, idle mismatch,
// core-limited) is aggregate-level by definition, so a compute_supply rank
// candidate or observation must never carry a concrete per-thread subject —
// a per-thread runnable/running wait is demand-side evidence and belongs to
// the runnable_wait / scheduling-pressure token family instead. Precise
// signal (pid + comm emptiness), never a prose/keyword check.
func computeSupplyCauseSubjectAllowed(thread ThreadRef) bool {
	return thread.PID <= 0 && strings.TrimSpace(thread.Comm) == ""
}

// computeSupplyVerdict classifies compute supply for one judged thread
// duration from PRECISE per-target signals (methodology audit §7.30.2
// R5e/R5g): weightedFreqKHz is the duration-weighted frequency across the
// judged segments (never a single point sample), observedMaxKHz the max
// frequency observed inside/nearest those segments (never the window residency
// max), and highPrioOverlapMs only the high-priority running that overlapped
// this thread's runnable waits on the same CPU (never the window-total
// high-priority running).
func computeSupplyVerdict(durationMs float64, weightedFreqKHz, observedMaxKHz int, highPrioOverlapMs float64, cpu CPUStats) (string, float64) {
	total := cpu.BusyMs + cpu.IdleMs
	busyRatio := 0.0
	if total > 0 {
		busyRatio = cpu.BusyMs / total
	}
	lowFreq := weightedFrequencyIsLow(weightedFreqKHz, observedMaxKHz)
	cpuPressure := busyRatio >= 0.80 || (durationMs > 0 && highPrioOverlapMs >= durationMs*0.50)
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
		summary.SystemOrKernelRunningMs += pressure.SystemOrKernelRunningMs
		summary.SystemOrKernelRunningOverlapMs += pressure.SystemOrKernelRunningOverlapMs
		summary.SystemOrKernelCompetitorCount += pressure.SystemOrKernelCompetitorCount
		if pressure.RunnableWaitMs > 0 {
			// PRESSURE-ONE-SEAT (2026-07-10 customer witness): this field is
			// the demand backlog numerator used as an average runnable-queue
			// depth. HighPriorityRunningMs is occupancy context; unless its
			// interval overlaps a runnable wait it is not displacement, and even
			// an overlap is evidence ABOUT the same wait rather than additional
			// queue time. Adding it here made app-100's unrelated 1.2ms running
			// inflate a 0.8ms runnable backlog to 2.0ms.
			summary.CPUPressureMs += pressure.RunnableWaitMs
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
		// R5e: judge low frequency from the RESIDENCY-WEIGHTED average over
		// the window, never from the single latest sample (CPUStats.Frequency)
		// — a CPU that ramped up mid-window used to read as "low" from a
		// stale early sample, producing the customer's false
		// supply-insufficient verdicts.
		if weighted := residencyWeightedFrequency(cpu); weighted > 0 && frequencyIsLowForCPU(weighted, cpu) {
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
	if summary.CPUPressureMs == 0 && summary.HighPriorityRunningMs == 0 && summary.SystemOrKernelRunningMs == 0 && summary.SchedStatWaitMs == 0 && summary.SchedStatIOWaitMs == 0 && summary.SchedStatBlockedMs == 0 && summary.IPIEventCount == 0 && len(summary.LowFrequencyCPUs) == 0 && summary.ClockSetRateCount == 0 && summary.ThermalEventCount == 0 && summary.DDREventCount == 0 && summary.L3EventCount == 0 && summary.ThroughputEventCount == 0 {
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
	case summary.SystemOrKernelRunningMs > 0:
		// Raw >159 Harmony scheduler tokens are disclosed as activity only.
		// They never enter the high-priority account or mint a priority claim.
		summary.Signal = "system_or_kernel_activity"
	default:
		summary.Signal = "supply_activity"
	}
	// CMP-9 (§7.3): the cross-thread sum is only cross-window comparable as a
	// density (value / wall window ≈ average runnable queue depth). Window
	// unbounded → no density, never an estimate.
	if windowMs := queryWindowWallMs(q); windowMs > 0 {
		summary.WindowMs = windowMs
		if summary.CPUPressureMs > 0 {
			summary.PressureDensity = summary.CPUPressureMs / windowMs
		}
	}
	summary.Summary = fmt.Sprintf("supply_pressure signal=%s cpu_pressure=%.3fms runnable=%.3fms high_prio=%.3fms system_or_kernel_running=%.3fms system_or_kernel_overlap=%.3fms system_or_kernel_competitors=%d sched_stat_wait=%.3fms sched_stat_iowait=%.3fms sched_stat_blocked=%.3fms ipi_events=%d ipi_active=%.3fms low_freq_cpus=%v clock_set_rate=%d thermal=%d ddr=%d l3=%d throughput=%d",
		summary.Signal, summary.CPUPressureMs, summary.RunnableWaitMs, summary.HighPriorityRunningMs,
		summary.SystemOrKernelRunningMs, summary.SystemOrKernelRunningOverlapMs, summary.SystemOrKernelCompetitorCount,
		summary.SchedStatWaitMs, summary.SchedStatIOWaitMs, summary.SchedStatBlockedMs, summary.IPIEventCount, summary.IPIActiveMs, summary.LowFrequencyCPUs, summary.ClockSetRateCount, summary.ThermalEventCount, summary.DDREventCount, summary.L3EventCount, summary.ThroughputEventCount)
	if summary.WindowMs > 0 {
		summary.Summary += fmt.Sprintf(" window_ms=%.3f pressure_density=%.2f (≈avg queue depth; cpu_pressure is a cross-thread cpu·ms sum, not wall clock)", summary.WindowMs, summary.PressureDensity)
	}
	return &summary
}

// residencyWeightedFrequency is the duration-weighted average frequency of a
// CPU over the window, derived from its cpu_frequency residency segments —
// the per-segment truth R5e demands, with zero extra plumbing. Returns 0 when
// the window exposes no residency.
func residencyWeightedFrequency(cpu CPUStats) int {
	totalMs, weighted := 0.0, 0.0
	for _, res := range cpu.FrequencyResidency {
		if res.Frequency <= 0 || res.DurationMs <= 0 {
			continue
		}
		totalMs += res.DurationMs
		weighted += float64(res.Frequency) * res.DurationMs
	}
	if totalMs <= 0 {
		return 0
	}
	return int(weighted / totalMs)
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
	if schedulerStateIntegrityFailureForQuery(idx, q, 0) != nil {
		return nil
	}
	if threadIncarnationConflictForQuery(idx, q, 0) != nil {
		return nil
	}
	minDurationMs := q.MinDurationMs
	if minDurationMs <= 0 {
		minDurationMs = 1
	}
	blockedReasons := blockedReasonsByPID(idx, q)
	open := map[int]stateChurnOpen{}
	accs := map[string]*stateChurnAcc{}
	head := schedulerHeadForQuery(idx, q)
	headOwnsPrefix := idx.Windowed && q.TimeStart > 0 && q.LineStart == 0 && q.LineEnd == 0 && head != nil
	headComplete := headOwnsPrefix && head.Complete
	closeState := func(pid int, endTs float64, endLine int) {
		start, ok := open[pid]
		if !ok {
			return
		}
		delete(open, pid)
		addStateChurnInterval(accs, start, endTs, endLine, q, blockedReasons)
	}
	openState := func(thread ThreadRef, state ThreadState, ts float64, line int) {
		// §7.11 B-1 sequel (2026-07-04 review): churn tracks ACTIVE scheduling
		// states only. B-1 typed T/t→stopped and X/Z→dead as non-Unknown so
		// interval-level faces book them honestly, but the churn accumulator's
		// five per-state lanes (and thus total) intentionally skip them — an
		// open stopped/dead segment therefore contributed ONLY fragments/
		// switches/maxSegment. A thread's exit tail (dead until window end)
		// then trips the maxSegment>=70%-of-total suppression gate and kills a
		// REAL churn row (two-window overlay counterexample: churn row present
		// with the window cut before the Z exit, gone once the dead tail
		// entered). Precise typed-state gate: stopped/dead are rejected exactly
		// like Unknown and never open a churn segment (shared authority:
		// stateChurnOpenIneligible, thread_state_universe.go — same gate as the
		// streaming face).
		if stateChurnOpenIneligible(thread, state) {
			return
		}
		open[thread.PID] = stateChurnOpen{thread: thread, state: state, ts: ts, line: line}
	}
	if headComplete {
		for _, state := range schedulerHeadSortedThreads(head) {
			openState(state.Thread, state.State, state.StartTs, state.Line)
		}
	}
	visitChurn := func(ev Event) {
		if !eventLineInWindow(ev, q) {
			return
		}
		if headOwnsPrefix && ev.Ts < q.TimeStart {
			return
		}
		switch ev.Type {
		case EventSchedWakeup, EventSchedWaking:
			if ev.WakeePID <= 0 {
				return
			}
			start, ok := open[ev.WakeePID]
			// Shared wakeup-reopen guard (thread_state_universe.go) — same
			// gate as the streaming face. EVOLUTION RECORD (headless-wakeup
			// alignment ruling, 主会话裁定 2026-07-10, §29.26 待落账): a
			// normal wakeup for a thread with no governing open state now
			// MINTS a runnable segment from the wakeup timestamp instead of
			// being dropped (the old `!ok` reject arm) — aligning this churn
			// face with the offCPU face (computeOffCPUStats), which has
			// always minted it, and with the streaming state-cluster face.
			// The wakeup is a witnessed typed transition; only the prefix
			// before it stays unknown, and the head-coverage face keeps
			// disclosing that prefix independently.
			if !schedWakeupStartsNewIncarnation(ev) && ok && (stateChurnWakeupReopenIneligible(start.state) || ev.Ts < start.ts) {
				return
			}
			if ok {
				closeState(ev.WakeePID, ev.Ts, ev.Line)
			}
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
	visitEventsInTimestampOrder(idx, nil, false, visitChurn)
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
	key := threadKey(start.thread)
	acc := accs[key]
	if acc == nil {
		acc = &stateChurnAcc{thread: start.thread}
		accs[key] = acc
	}
	candidateEndLine := firstPositive(endLine, start.line)
	if candidateEndLine > acc.lineEnd || (candidateEndLine == acc.lineEnd && threadDisplayLess(start.thread, acc.thread)) {
		acc.thread = start.thread
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
	if candidateEndLine > acc.lineEnd {
		acc.lineEnd = candidateEndLine
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
		if best == nil || eventLaterThan(*ev, *best) {
			best = ev
		}
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
		NextStepKind:     stateChurnNextStepKind(dominantState),
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
		competitor, overlapMs, windowRunningMs := stateChurnTopCompetitor(items[i].Thread, pressure)
		items[i].RunnableCPU = cpu
		items[i].RunnableCPUKnown = true
		items[i].RunnableCoreClass = pressure.CoreClass
		if competitor != "" {
			items[i].TopCompetitor = competitor
			items[i].TopCompetitorOverlapMs = overlapMs
			items[i].TopCompetitorRunningMs = windowRunningMs
			items[i].NextStep = fmt.Sprintf("inspect %s on same CPU cpu=%d for CPU pressure/time-slice competition (its running overlaps this thread's runnable wait by %.3fms), then validate wake_latency with sched_wakeup", competitor, cpu, overlapMs)
		} else {
			items[i].NextStep = fmt.Sprintf("inspect same-CPU pressure on cpu=%d, top running competitors, priority, CPU frequency, and sched_wakeup wake_latency", cpu)
		}
		// Both dynamic variants are runnable-state same-CPU competition
		// guidance; the loop above only reaches runnable-dominant rows.
		items[i].NextStepKind = NextStepKindRunnable
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

// stateChurnTopCompetitor names a competitor only when its running time
// actually overlapped the target's runnable waits on this CPU (§7.30.2 R5g).
// Threads that merely ran on the same CPU outside those waits are serial
// hand-offs, not competitors, and yield no competitor. It returns the label,
// the overlapped ms (displacement evidence), and the competitor's window
// running total on this CPU (background context).
func stateChurnTopCompetitor(target ThreadRef, pressure CPUPressureStats) (string, float64, float64) {
	overlap := runnableDisplacementOverlap(pressure, target, 8)
	competitors := overlap.competitors
	for _, running := range competitors {
		label := threadLabel(running.Thread)
		if label == "" {
			continue
		}
		windowRunningMs := 0.0
		for _, td := range pressure.TopRunning {
			if sameThreadRef(td.Thread, running.Thread) {
				windowRunningMs = td.DurationMs
				break
			}
		}
		return label, running.DurationMs, windowRunningMs
	}
	return "", 0, 0
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
		summary = fmt.Sprintf("%s; top_competitor=%s overlap=%.3fms", summary, item.TopCompetitor, item.TopCompetitorOverlapMs)
		if item.TopCompetitorRunningMs > 0 {
			summary = fmt.Sprintf("%s running_window=%.3fms", summary, item.TopCompetitorRunningMs)
		}
	}
	return summary
}

func dominantChurnState(acc *stateChurnAcc) (string, float64) {
	// Shared 5-lane pick (thread_state_universe.go) — same priority order the
	// inline candidates array always used.
	return dominantStateFromLanes(acc.runningMs, acc.runnableMs, acc.sleepMs, acc.dStateMs, acc.ioWaitMs)
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

// stateChurnNextStepKind is the typed counterpart of stateChurnNextStep (and
// of the dynamic same-CPU competitor variants, which are runnable-state
// guidance). Must stay in lockstep with the switch above so renderers can
// localize the guidance without parsing English prose.
func stateChurnNextStepKind(state string) string {
	switch state {
	case string(StateRunnable):
		return NextStepKindRunnable
	case string(StateSSleep):
		return NextStepKindSSleep
	case string(StateDSleep), string(StateIOWait):
		return NextStepKindDSleepIO
	case string(StateRunning):
		return NextStepKindRunning
	default:
		return NextStepKindGeneric
	}
}

// stateDrilldownSignificantFloor is the fraction of the selected window a
// state must occupy (in addition to always keeping the top state) to be
// marked Significant for R3 per-layer root-cause prioritization. 0.05 = 5%
// keeps genuinely material secondary states while flagging long-tail
// low-share states (the §2.1 gap) as coverage-only. There is no pre-existing
// proportion constant in this package, so no compatibility baggage; kept as a
// single constant for later codrax.yaml-ification (cf. O7 direction).
const stateDrilldownSignificantFloor = 0.05

// stateDrilldownSignificantTopRatio marks a lower-ranked state significant
// when its impact is at least this fraction of the top state's impact, so a
// second state that is large relative to the leader (but small vs a huge
// window) is still surfaced as worth root-causing.
const stateDrilldownSignificantTopRatio = 0.25

// stateDrilldownIdleWholeWindowRatio is the whole-window threshold for the
// top_sleep idle fold: a candidate whose cumulative sleep covers at least
// this fraction of the selected window slept through effectively the entire
// window. Such threads (audio output sinks, DNS watchers, FFRT workers
// parked between jobs) are idle by construction — impact == window carries
// zero root-cause information — yet a customer's 101ms state_drilldown
// surface (berlin.systrace 2026-07-03) was flooded with 15+ impact=101.000
// whole-window sleeper rows that drowned the real candidates. Precise signal
// (one float comparison on the published physical ms) driving a display-side
// fold only; the fold summary stays visible so absence is auditable.
const stateDrilldownIdleWholeWindowRatio = 0.99

// stateDrilldownIdleSleeperThreadListCap bounds the folded thread-label list
// carried on the fold summary (the count stays exact beyond the cap).
const stateDrilldownIdleSleeperThreadListCap = 8

// stateDrilldownPinnedTarget reports whether a drilldown candidate is the
// query's explicitly pinned target thread: exact pid equality or verbatim
// comm equality against the typed query selector — precise signals only,
// per the hard-gate signal discipline (the fold suppresses a row, which is
// hard behavior). A pinned target that slept through the whole window is the
// investigation subject (e.g. a UI thread lock-blocked across a 101ms jank
// window, QF2 2026-07-03), not an idle service thread: folding it would tell
// the model its own target did nothing and misdirect the root-cause hunt.
func stateDrilldownPinnedTarget(thread ThreadRef, pinnedPID int, pinnedComm string) bool {
	if pinnedPID > 0 {
		return thread.PID == pinnedPID
	}
	if pinnedComm != "" && thread.Comm == pinnedComm {
		return true
	}
	return false
}

// buildStateDrilldownPlan is the unpinned-compatibility entry (no explicit
// query target); production call sites route through
// buildStateDrilldownPlanForTarget so the idle fold can exempt the pinned
// target thread.
func buildStateDrilldownPlan(stats WindowStats, max int) ([]StateDrilldownStep, *IdleWholeWindowSleeperFold) {
	return buildStateDrilldownPlanForTarget(stats, max, 0, "")
}

func buildStateDrilldownPlanForTarget(stats WindowStats, max int, pinnedPID int, pinnedThread string) ([]StateDrilldownStep, *IdleWholeWindowSleeperFold) {
	var candidates []StateDrilldownStep
	fragmentedSleep := fragmentedSleepChurnByThread(stats.StateChurn)
	pinnedComm := strings.TrimSpace(pinnedThread)
	// windowMs feeds both the idle whole-window fold below and the
	// Significant proportion computation on the emitted steps.
	windowMs := (stats.Window.EndTs - stats.Window.StartTs) * 1000
	var idleFold *IdleWholeWindowSleeperFold
	addDuration := func(source, state string, items []ThreadDuration) {
		for _, td := range items {
			if td.Thread.PID <= 0 || td.DurationMs <= 0 {
				continue
			}
			if source == "top_sleep" && state == string(StateSSleep) {
				// Whole-window sleepers are folded, not ranked: a Top-N sleep
				// row that spans (>=99% of) the window and is NOT the query's
				// pinned target is an idle service thread, and publishing it
				// as a drilldown candidate invites per-thread root-cause work
				// on threads that did nothing. The pinned target is exempt
				// (QF2): a victim thread blocked for the whole window shows
				// the same whole-window-sleep signal and must keep its ranked
				// drilldown row. Runs before the fragmented-sleep filter
				// (which stays untouched): a whole-window sleeper is counted
				// here even in the rare case its churn shape also looks
				// fragmented.
				if windowMs > 0 && td.DurationMs >= windowMs*stateDrilldownIdleWholeWindowRatio &&
					!stateDrilldownPinnedTarget(td.Thread, pinnedPID, pinnedComm) {
					if idleFold == nil {
						idleFold = &IdleWholeWindowSleeperFold{}
					}
					idleFold.Count++
					if len(idleFold.Threads) < stateDrilldownIdleSleeperThreadListCap {
						idleFold.Threads = append(idleFold.Threads, threadLabel(td.Thread))
					}
					continue
				}
				if _, ok := fragmentedSleep[stateDrilldownThreadKey(td.Thread)]; ok {
					continue
				}
			}
			candidates = append(candidates, StateDrilldownStep{
				Thread:           td.Thread,
				State:            state,
				ImpactMs:         td.DurationMs,
				TotalMs:          td.DurationMs,
				Source:           source,
				RecommendedViews: stateDrilldownRecommendedViewsForSource(state, source),
				ChainRequired:    stateDrilldownNeedsWakeupChainForSource(state, source),
				Recursive:        stateDrilldownNeedsRecursiveChainForSource(state, source),
				StartTs:          td.StartTs,
				EndTs:            td.EndTs,
				LineStart:        td.LineStart,
				LineEnd:          td.LineEnd,
			})
		}
	}
	addDuration("top_sleep", string(StateSSleep), stats.SleepTop)
	addDuration("top_runnable", string(StateRunnable), stats.RunnableTop)
	addDuration("top_running", string(StateRunning), stats.TopRunning)
	addDuration("top_io_wait", string(StateIOWait), stats.IOWaitTop)
	addDuration("top_d_state", string(StateDSleep), stats.DStateTop)
	for _, churn := range stats.StateChurn {
		if churn.Thread.PID <= 0 || strings.TrimSpace(churn.DominantState) == "" {
			continue
		}
		rankImpact := stateChurnRankImpactMs(churn)
		if rankImpact <= 0 {
			continue
		}
		// ImpactMs must stay the PHYSICAL dominant-state duration: the step
		// is published as a hard ms observation and must reconcile with the
		// churn totals (§7.30 S1 — the composite leaked into a customer
		// report as a 119%-of-window running time). The fragmentation boost
		// lives only in the ranking-only RankImpactMs channel.
		candidates = append(candidates, StateDrilldownStep{
			Thread:           churn.Thread,
			State:            churn.DominantState,
			ImpactMs:         churn.DominantImpactMs,
			TotalMs:          churn.TotalMs,
			RankImpactMs:     rankImpact,
			Source:           "state_churn",
			RecommendedViews: stateDrilldownRecommendedViewsForSource(churn.DominantState, "state_churn"),
			ChainRequired:    stateDrilldownNeedsWakeupChainForSource(churn.DominantState, "state_churn"),
			Recursive:        stateDrilldownNeedsRecursiveChainForSource(churn.DominantState, "state_churn"),
			LineStart:        churn.LineStart,
			LineEnd:          churn.LineEnd,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		ci, cj := stateDrilldownPriority(candidates[i]), stateDrilldownPriority(candidates[j])
		if ci != cj {
			return ci > cj
		}
		wi, wj := stateDrilldownRankWeight(candidates[i]), stateDrilldownRankWeight(candidates[j])
		if wi != wj {
			return wi > wj
		}
		if candidates[i].TotalMs != candidates[j].TotalMs {
			return candidates[i].TotalMs > candidates[j].TotalMs
		}
		return candidates[i].LineStart < candidates[j].LineStart
	})
	var topImpact float64
	seen := map[string]bool{}
	out := make([]StateDrilldownStep, 0, len(candidates))
	for _, step := range candidates {
		key := threadKey(step.Thread) + "/state:" + step.State
		if seen[key] {
			continue
		}
		seen[key] = true
		step.Rank = len(out) + 1
		if step.Rank == 1 {
			topImpact = step.ImpactMs
		}
		if windowMs > 0 && step.ImpactMs > 0 {
			proportion := step.ImpactMs / windowMs
			if proportion > 1 {
				proportion = 1
			}
			step.WindowProportion = proportion
			step.Significant = proportion >= stateDrilldownSignificantFloor ||
				(topImpact > 0 && step.ImpactMs/topImpact >= stateDrilldownSignificantTopRatio)
		}
		if step.Rank == 1 && step.ImpactMs > 0 {
			step.Significant = true
		}
		step.Summary = renderStateDrilldownStep(step)
		out = append(out, step)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out, idleFold
}

func fragmentedSleepChurnByThread(churns []ThreadStateChurnSummary) map[string]ThreadStateChurnSummary {
	out := map[string]ThreadStateChurnSummary{}
	for _, churn := range churns {
		if !isFragmentedSleepChurn(churn) {
			continue
		}
		out[stateDrilldownThreadKey(churn.Thread)] = churn
	}
	return out
}

func isFragmentedSleepChurn(churn ThreadStateChurnSummary) bool {
	return churn.Thread.PID > 0 &&
		churn.DominantState == string(StateSSleep) &&
		churn.SleepMs > 0 &&
		churn.FragmentCount >= 4 &&
		churn.StateSwitches >= 3 &&
		churn.MaxSegmentMs > 0 &&
		churn.MaxSegmentMs < churn.SleepMs*0.70
}

func stateDrilldownThreadKey(thread ThreadRef) string {
	return threadKey(thread)
}

func stateDrilldownPriority(step StateDrilldownStep) float64 {
	// Rank weight, not published ImpactMs: fragmented churn rows keep their
	// ranking composite here while the published impact stays physical
	// (§7.30 S1).
	score := stateDrilldownRankWeight(step)
	if step.ChainRequired {
		score *= 1.25
	}
	return score
}

func stateDrilldownRecommendedViewsForSource(state, source string) []string {
	if source == "state_churn" && state == string(StateSSleep) {
		return []string{"thread_timeline", "interaction_stats", "window_stats"}
	}
	return stateDrilldownRecommendedViews(state)
}

func stateDrilldownRecommendedViews(state string) []string {
	switch state {
	case string(StateSSleep):
		return []string{"wakeup_chain", "root_cause_rank"}
	case string(StateRunnable):
		// RN-11 (§7.9): a runnable-dominant row has no sleep edge to chase — its
		// drilldown surfaces are CPU competition ones: scheduler latency,
		// ranked competitors, and window_stats (cpu_occupancy top occupiers +
		// compute_supply_balance). wakeup_chain is NOT required for it.
		return []string{"scheduler_latency_stats", "root_cause_rank", "window_stats"}
	case string(StateRunning):
		return []string{"trace_perf_bundle", "perf_stats", "root_cause_rank"}
	case string(StateDSleep), string(StateIOWait):
		return []string{"critical_blocking_calls", "window_stats", "root_cause_rank"}
	default:
		return []string{"thread_timeline", "window_stats"}
	}
}

func stateDrilldownNeedsWakeupChainForSource(state, source string) bool {
	if source == "state_churn" && state == string(StateSSleep) {
		return false
	}
	return stateDrilldownNeedsWakeupChain(state)
}

func stateDrilldownNeedsWakeupChain(state string) bool {
	switch state {
	// RN-11 (§7.9): StateRunnable is deliberately absent — a runnable-dominant
	// row is CPU competition, not a wakeup dependency; forcing chain_required
	// on it drove the customer session (cust_runnable round 10) into a
	// wakeup_chain drilldown the model correctly pushed back on, and set the
	// projection's "recommended wakeup chain not run" warning on windows with
	// no wakeup edge at all. Sleep/D/IO behavior is unchanged.
	case string(StateSSleep), string(StateDSleep), string(StateIOWait):
		return true
	default:
		return false
	}
}

func stateDrilldownNeedsRecursiveChainForSource(state, source string) bool {
	// RN-11: runnable rows drop the wakeup-chain requirement above but REMAIN
	// recursive root-cause candidates (occupancy/scheduler-latency drilldown)
	// — the tool-description contract "fragmented runnable or D/IO waits
	// remain recursive root-cause candidates" is unchanged.
	if state == string(StateRunnable) {
		return true
	}
	return stateDrilldownNeedsWakeupChainForSource(state, source)
}

func renderStateDrilldownStep(step StateDrilldownStep) string {
	return fmt.Sprintf("state_drilldown rank=%d thread=%s state=%s impact=%.3fms total=%.3fms source=%s recommended_views=%s chain_required=%t recursive=%t window_proportion=%.4f significant=%t lines=%d-%d",
		step.Rank, threadLabel(step.Thread), step.State, step.ImpactMs, step.TotalMs, step.Source,
		strings.Join(step.RecommendedViews, ","), step.ChainRequired, step.Recursive, step.WindowProportion, step.Significant, step.LineStart, step.LineEnd)
}

// threadDurationCapOverflow reports how many census groups sit beyond the
// capped display list plus their summed account (修复轮二 件A, 2026-07-13).
// Kept-set identity = the exact (thread,cpu) accumulation key; the overflow
// total sums each evicted group's own float ONCE (never census−top
// subtraction, never a re-aggregation of kept values).
func threadDurationCapOverflow(census map[string]ThreadDuration, top []ThreadDuration) (int, float64) {
	if len(census) <= len(top) {
		return 0, 0
	}
	kept := make(map[string]bool, len(top))
	for _, td := range top {
		kept[threadCPUKey(td.Thread, td.CPU)] = true
	}
	groups, total := 0, 0.0
	for key, td := range census {
		if kept[key] {
			continue
		}
		groups++
		total += td.DurationMs
	}
	return groups, total
}

func topThreadDurations(in map[string]ThreadDuration, max int) []ThreadDuration {
	out := make([]ThreadDuration, 0, len(in))
	for _, td := range in {
		out = append(out, td)
	}
	// 修复轮二 件A (DET 纪律, 2026-07-13): the stable sort used to preserve
	// MAP order among equal durations — a tie was elected by map iteration
	// (the DET-1 disease class). Typed constant tie chain instead; the family
	// mint reuses THIS comparator uncapped (单一值源), so member order and
	// the display top lists can never diverge.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DurationMs != out[j].DurationMs {
			return out[i].DurationMs > out[j].DurationMs
		}
		if out[i].LineStart != out[j].LineStart {
			return out[i].LineStart < out[j].LineStart
		}
		if out[i].Thread.PID != out[j].Thread.PID {
			return out[i].Thread.PID < out[j].Thread.PID
		}
		if out[i].CPU != out[j].CPU {
			return out[i].CPU < out[j].CPU
		}
		// 修复轮三 F6: pid==0 kernel-thread groups (comm-keyed identities)
		// can tie on every scalar above — the comm key closes the last
		// map-order fallback.
		return out[i].Thread.Comm < out[j].Thread.Comm
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

// blockedReasonPIDTotal is one pid's FULL-window blocked_reason account
// (CR-3 修复轮 P2): total marker count plus up to two distinct semantic
// caller symbols in deterministic (count desc, first line asc) order.
type blockedReasonPIDTotal struct {
	count   int
	callers []string
}

// foldBlockedReasonFullByPID folds the full (pid, iowait, reason)
// accumulator into per-pid totals BEFORE any truncation (INODE §28.6
// discipline). Symbol hygiene mirrors the unanimous-caller lane: '+'
// offset carve, opaque/hex/unknown never surface a name.
func foldBlockedReasonFullByPID(in map[string]BlockedReasonSummary) map[int]blockedReasonPIDTotal {
	byPID := map[int][]BlockedReasonSummary{}
	for _, s := range in {
		if s.Thread.PID <= 0 || s.Count <= 0 {
			continue
		}
		byPID[s.Thread.PID] = append(byPID[s.Thread.PID], s)
	}
	out := make(map[int]blockedReasonPIDTotal, len(byPID))
	for pid, rows := range byPID {
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Count != rows[j].Count {
				return rows[i].Count > rows[j].Count
			}
			return rows[i].Line < rows[j].Line
		})
		total := blockedReasonPIDTotal{}
		for _, s := range rows {
			total.count += s.Count
			c := s.Reason
			if i := strings.IndexByte(c, '+'); i > 0 {
				c = c[:i]
			}
			c = strings.TrimSpace(c)
			if c == "" || c == "unknown" || isPureHexAddressToken(c) {
				continue
			}
			dup := false
			for _, have := range total.callers {
				if have == c {
					dup = true
					break
				}
			}
			if !dup && len(total.callers) < 2 {
				total.callers = append(total.callers, c)
			}
		}
		out[pid] = total
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
	if idx == nil || ev.WakeePID == 0 {
		return ThreadRef{PID: ev.WakeePID}
	}
	// Resolve metadata from the physical lifecycle prefix ending at this exact
	// evidence row. A full-index first match can belong to an earlier occupant
	// of the same numeric TID and leak its comm/TGID into the blocked reason.
	return resolvePIDThread(idx, ev.WakeePID, Query{LineEnd: ev.Line})
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
	if ev.CPUForFieldValid && validTraceCPUIndex(ev.CPUForField) {
		return ev.CPUForField
	}
	if ev.CPUForFieldPresent || !validTraceCPUIndex(ev.CPU) {
		return -1
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
	cpus, valid, _ := parseCPURangeListStrict(raw)
	if !valid {
		return nil
	}
	return cpus
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
		applyThreadCoreClasses(items[i].OverlapCompetitors, byCPU)
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
		a.item.SystemOrKernelRunningMs += p.SystemOrKernelRunningMs
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

// sortFrequencyTimeline sorts each CPU's cpu_frequency samples by timestamp so
// segmentFrequencyStats can binary-search change points.
func sortFrequencyTimeline(freqByCPU map[int][]Event) {
	for _, events := range freqByCPU {
		sort.SliceStable(events, func(i, j int) bool {
			if events[i].Ts != events[j].Ts {
				return events[i].Ts < events[j].Ts
			}
			return events[i].Line < events[j].Line
		})
	}
}

// frequencySegmentStats summarizes the cpu_frequency timeline over one judged
// scheduling segment (methodology audit §7.30.2 R5e): weightedKHz integrates
// the frequency across cpu_frequency change points inside [startTs, endTs);
// observedMaxKHz is the max sample observed inside the segment or at its
// nearest bracketing samples (the low-frequency benchmark, never the
// window-wide residency max); inSegmentSamples counts change points strictly
// inside the segment — zero means the value rests entirely on nearest samples.
type frequencySegmentStats struct {
	weightedKHz      float64
	observedMaxKHz   int
	inSegmentSamples int
	known            bool
}

// segmentFrequencyStats integrates the sorted cpu_frequency timeline over
// [startTs, endTs). A single point sample never represents the whole segment:
// the segment is split at every in-segment change point and duration-weighted.
// When no sample falls inside the segment, the nearest sample rules it —
// preceding first, following as last resort — instead of defaulting to
// zero/low/high (§7.30.2 R5e). samples must be sorted by Ts (see
// sortFrequencyTimeline); only a fully empty timeline yields known=false.
func segmentFrequencyStats(samples []Event, startTs, endTs float64) frequencySegmentStats {
	var out frequencySegmentStats
	if len(samples) == 0 || endTs <= startTs {
		return out
	}
	// A sample sitting exactly AT the segment start is an in-segment
	// observation (half-open [startTs, endTs)), not a preceding one —
	// classifying it as preceding falsely raised the nearest_fallback
	// provenance marker on perfectly-sampled segments.
	first := sort.Search(len(samples), func(i int) bool { return samples[i].Ts >= startTs })
	current := 0
	if first > 0 {
		current = samples[first-1].Frequency
		out.observedMaxKHz = max(out.observedMaxKHz, current)
	}
	cursor := startTs
	weighted := 0.0
	for i := first; i < len(samples) && samples[i].Ts < endTs; i++ {
		if current > 0 {
			weighted += float64(current) * (samples[i].Ts - cursor)
		} else {
			// Head portion before the first known sample: the first in-segment
			// sample is the nearest available observation for it.
			weighted += float64(samples[i].Frequency) * (samples[i].Ts - cursor)
		}
		current = samples[i].Frequency
		cursor = samples[i].Ts
		out.inSegmentSamples++
		out.observedMaxKHz = max(out.observedMaxKHz, current)
	}
	following := sort.Search(len(samples), func(i int) bool { return samples[i].Ts >= endTs })
	if current == 0 {
		// No preceding and no in-segment sample: fall back to the nearest
		// following sample (last resort, never a default).
		if following >= len(samples) {
			return frequencySegmentStats{}
		}
		current = samples[following].Frequency
	}
	weighted += float64(current) * (endTs - cursor)
	// The nearest following sample participates in the observed-max benchmark
	// ("inside or nearby"), even when unused for coverage.
	if following < len(samples) {
		out.observedMaxKHz = max(out.observedMaxKHz, samples[following].Frequency)
	}
	out.weightedKHz = weighted / (endTs - startTs)
	out.known = out.weightedKHz > 0
	return out
}

// weightedFrequencyIsLow is the R5e low-frequency test: the duration-weighted
// frequency across the judged segments sits at or below 65% of the max
// frequency observed inside or nearest those segments. The window-wide
// residency max is NOT the benchmark (§7.30.2 R5e).
func weightedFrequencyIsLow(weightedKHz, observedMaxKHz int) bool {
	return weightedKHz > 0 && observedMaxKHz > 0 && weightedKHz < observedMaxKHz &&
		float64(weightedKHz) <= float64(observedMaxKHz)*0.65
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

func computeTraceMarks(idx *Index, q Query, max int) ([]TraceSpanSummary, []TraceCounterSummary, []string) {
	if idx == nil {
		return nil, nil, nil
	}
	type counterInventoryKey struct {
		emitterPID  int
		ownerPID    int
		ownerRaw    string
		ownerScope  string
		name        string
		value       string
		metadataRaw string
		issueReason string
	}
	unknownEmitter := traceMarkUnknownEmitterFailureForQuery(idx, q)
	stacks := map[string][]Event{}
	var spans []TraceSpanSummary
	asyncPairer := newTraceMarkAsyncPairer(q, nil, func(pair traceMarkAsyncPair) {
		if span, ok := clipTraceMarkSpanToQueryWindow(traceSpanFromEvents(pair.start, pair.end, "async", pair.source), q); ok {
			spans = append(spans, span)
		}
	})
	unresolvedPairingRows := 0
	counters := map[counterInventoryKey]TraceCounterSummary{}
	// DCS E4 (ledger §23/§23.1 H1, 2026-07-08): B/E and S/F pairing runs over
	// the WHOLE trace-mark stream instead of the query window — a compile span
	// riding the window boundary used to lose one end to the window filter and
	// vanish with zero warning (the pair never formed). Minting still admits
	// only window-overlapping spans, clipped to the window
	// (clipTraceMarkSpanToQueryWindow); C| counter rows keep the strict
	// window filter unchanged.
	for _, ev := range idx.Events {
		if resetPID, reset := schedulerLifecycleResetPID(ev); reset {
			source, ok := tracePairingSourceIdentity(idx, ev)
			if !ok {
				unresolvedPairingRows++
				continue
			}
			resetTraceMarkSyncPairingState(source, resetPID, stacks)
			asyncPairer.observeLifecycle(source, resetPID, ev)
			continue
		}
		if ev.Type != EventTraceMark {
			continue
		}
		if traceMarkEventMalformed(ev) {
			source, ok := tracePairingSourceIdentity(idx, ev)
			if !ok {
				unresolvedPairingRows++
				continue
			}
			resetTraceMarkSyncPairingState(source, ev.PID, stacks)
			asyncPairer.observeMalformed(source, ev.PID, ev)
			continue
		}
		var source string
		if ev.SpanAction == "B" || ev.SpanAction == "E" || ev.SpanAction == "S" || ev.SpanAction == "F" {
			var ok bool
			source, ok = tracePairingSourceIdentity(idx, ev)
			if !ok {
				unresolvedPairingRows++
				continue
			}
		}
		switch ev.SpanAction {
		case "B":
			key := traceMarkSyncPairingKey(source, ev.PID)
			stacks[key] = append(stacks[key], ev)
		case "E":
			key := traceMarkSyncPairingKey(source, ev.PID)
			stack := stacks[key]
			if len(stack) == 0 {
				continue
			}
			start := stack[len(stack)-1]
			stacks[key] = stack[:len(stack)-1]
			if ev.Ts < start.Ts {
				continue
			}
			if span, ok := clipTraceMarkSpanToQueryWindow(traceSpanFromEvents(start, ev, "sync", source), q); ok {
				spans = append(spans, span)
			}
		case "S":
			asyncPairer.observeEndpoint(source, ev)
		case "F":
			asyncPairer.observeEndpoint(source, ev)
		case "C":
			if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
				continue
			}
			// Use the same closed C| parser as CounterDeltas. In particular,
			// Pipe-containing names and terminal OpenHarmony metadata must not
			// acquire a second, contradictory legacy interpretation. Invalid /
			// non-numeric rows stay in this compatibility inventory;
			// CounterQuality carries the typed reason and suppresses only derived
			// numeric claims.
			sample := parseTraceCounterSample(ev)
			name, value := sample.name, sample.valueRaw
			if name == "" {
				name = strings.TrimSpace(ev.SpanName)
			}
			if value == "" {
				value = strings.TrimSpace(ev.SpanValue)
			}
			key := counterInventoryKey{
				emitterPID: ev.PID, ownerPID: sample.ownerPID, ownerRaw: sample.ownerRaw,
				ownerScope: sample.ownerScope, name: name, value: value,
				metadataRaw: sample.metadataRaw, issueReason: sample.issueReason,
			}
			counter, exists := counters[key]
			if !exists {
				counter = TraceCounterSummary{
					Thread: threadRefFromEvent(ev), OwnerPID: sample.ownerPID,
					OwnerRaw: sample.ownerRaw, OwnerScope: sample.ownerScope,
					Name: name, Value: value, TrailingTag: sample.metadataRaw,
					OutputLevel: sample.outputLevel, TagBits: sample.tagBits,
				}
			}
			counter.Count++
			if counter.Line == 0 || ev.Line < counter.Line {
				counter.Thread = threadRefFromEvent(ev)
				counter.Line = ev.Line
				counter.Ts = ev.Ts
			}
			counters[key] = counter
		}
	}
	asyncOpenStarts := asyncPairer.openStarts()
	asyncPairer.finishEOF()
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].DurationMs != spans[j].DurationMs {
			return spans[i].DurationMs > spans[j].DurationMs
		}
		return spans[i].StartLine < spans[j].StartLine
	})
	var semanticBound traceMarkSemanticBoundInfo
	spans, semanticBound = boundTraceMarkSpansWithInfo(spans, max)
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
	caveats := traceMarkIntegrityCaveats(idx, q)
	if unresolvedPairingRows > 0 {
		spans = nil
		caveats = append(caveats, fmt.Sprintf("trace_mark_pairing_provenance_unresolved=true; rows=%d; trace span durations were omitted because an endpoint or reset could not be mapped to exactly one physical source artifact", unresolvedPairingRows))
	}
	if unknownEmitter {
		spans = nil
		caveats = append(caveats, "trace_mark_span_pairing_fail_closed=true; a malformed trace_mark endpoint has an unknown emitter, could not materialize as an Event, or overflowed the bounded witness ledger, so trace_spans/trace_mark_categories/async_file_work are omitted; trace counter inventory remains available")
		return spans, counterList, caveats
	}
	if unresolvedPairingRows > 0 {
		return spans, counterList, caveats
	}
	if boundCaveat := semanticBound.caveat(); boundCaveat != "" {
		caveats = append(caveats, boundCaveat)
	}
	caveats = append(caveats, asyncPairer.caveats()...)
	caveats = append(caveats, incompleteSemanticTraceMarkCaveats(q, stacks, asyncOpenStarts)...)
	return spans, counterList, caveats
}

// incompleteSemanticTraceMarkCaveats (DCS E4 caveat half, ledger §23/§23.1
// H1): after full-stream pairing, a leftover B|/S| whose name carries a
// SEMANTIC word surface (classified or near-miss compile/verify/shader/texture-upload
// vocabulary) is an incomplete pair the window projection could not mint —
// e.g. its end marker fell outside the captured trace. Fail-loud instead of
// fail-silent: name up to three such spans. Bare sync E| orphans carry no
// name and stay silent (nothing to report without inventing an identity).
// Only starts at or before the window end can still overlap the window; later
// starts are irrelevant to this query and stay out.
func incompleteSemanticTraceMarkCaveats(q Query, stacks map[string][]Event, asyncStarts []Event) []string {
	var names []string
	seen := map[string]bool{}
	note := func(ev Event) {
		name := strings.TrimSpace(ev.SpanName)
		if name == "" || seen[name] {
			return
		}
		if q.TimeEnd > 0 && ev.Ts > q.TimeEnd {
			return
		}
		if _, classified := traceSpanSemanticWorkClass(name); !classified && !traceSpanNearMissesSemanticWorkClassification(name) {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, stack := range stacks {
		for _, ev := range stack {
			note(ev)
		}
	}
	for _, ev := range asyncStarts {
		note(ev)
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	if len(names) > 3 {
		names = names[:3]
	}
	return []string{fmt.Sprintf("trace mark span(s) with compile/verify/shader/texture-upload-like names never closed inside the captured trace (e.g. %s); their duration cannot be projected into the window and they are absent from trace_spans and root_cause_rank", strings.Join(names, ", "))}
}

// clipTraceMarkSpanToQueryWindow (DCS E4, ledger §23/§23.1 H1) admits a fully
// paired span only when it overlaps the query window and clips the published
// extent to that window: StartTs/EndTs/DurationMs become the in-window
// projection (preserving the raw≡in-window invariant every downstream
// consumer relies on) while Actual* keeps the physical B/E extent for
// cross-window disclosure. Line-window queries keep overlap admission on line
// numbers; durations are time-based and are never "clipped by lines".
func clipTraceMarkSpanToQueryWindow(span TraceSpanSummary, q Query) (TraceSpanSummary, bool) {
	if q.LineStart > 0 && span.EndLine > 0 && span.EndLine < q.LineStart {
		return span, false
	}
	if q.LineEnd > 0 && span.StartLine > 0 && span.StartLine > q.LineEnd {
		return span, false
	}
	start, end := span.StartTs, span.EndTs
	if end <= start {
		// Zero-duration pairs (E at the same timestamp) keep their legacy
		// point-in-window admission — there is nothing to clip.
		return span, timeInWindow(start, q)
	}
	clipStart, clipEnd := start, end
	if q.TimeStart > 0 && clipStart < q.TimeStart {
		clipStart = q.TimeStart
	}
	if q.TimeEnd > 0 && clipEnd > q.TimeEnd {
		clipEnd = q.TimeEnd
	}
	if clipEnd <= clipStart {
		return span, false
	}
	if clipStart != start || clipEnd != end {
		span.ActualStartTs, span.ActualEndTs, span.ActualDurationMs = start, end, span.DurationMs
		span.StartTs, span.EndTs = clipStart, clipEnd
		span.DurationMs = (clipEnd - clipStart) * 1000
	}
	return span, true
}

// traceMarkSemanticSpanCap bounds semantic-class spans (JIT compile, class
// verification, shader/runtime compile, texture upload, GC pause) separately from the generic
// duration-ranked cap in boundTraceMarkSpans, so a short compile/verify span
// is not silently evicted by longer unrelated spans before
// traceSpanSemanticWorkClass ever gets to classify it. Matches
// traceCausalProjectionSemanticSpanLimit so a semantic span that survives
// here is not re-truncated by a smaller cap further down the handoff chain.
const traceMarkSemanticSpanCap = 16

// boundTraceMarkSpans truncates spans (already duration-sorted by the
// caller) to at most max entries, but gives semantic-class spans their own
// reserved slots instead of letting them compete for the generic max slots
// purely by raw DurationMs. Without this, a 2ms JIT-compile span sitting
// behind 8+ longer generic spans (Choreographer, RenderFrame, etc.) would
// never reach traceSpanSemanticWorkClass, root_cause_rank, or the
// independent trace_semantic_span typed-observation channel at all.
func boundTraceMarkSpans(spans []TraceSpanSummary, max int) []TraceSpanSummary {
	out, _ := boundTraceMarkSpansWithInfo(spans, max)
	return out
}

type traceMarkSemanticBoundInfo struct {
	totalSpans      int
	keptSpans       int
	totalFamilies   int
	omittedFamilies int
	omittedRoster   []string
}

func (info traceMarkSemanticBoundInfo) caveat() string {
	omitted := info.totalSpans - info.keptSpans
	if omitted <= 0 {
		return ""
	}
	roster := "none (omissions only reduce totals of represented families)"
	if len(info.omittedRoster) > 0 {
		roster = strings.Join(info.omittedRoster, ", ")
	}
	return fmt.Sprintf("semantic span bound kept %d/%d classified span(s) across %d family/families; omitted %d span(s) from %d whole family/families (bounded omitted-family roster, first 8: %s). Published semantic family totals are lower bounds; every wholly omitted family is counted, and up to the first 8 are named here instead of the overflow disappearing silently", info.keptSpans, info.totalSpans, info.totalFamilies, omitted, info.omittedFamilies, roster)
}

// boundTraceMarkSpansWithInfo preserves one representative of every semantic
// (thread,class) family before filling remaining semantic seats by duration.
// Thus a high-frequency VerifyClass family cannot crowd a rarer GC/JIT/shader
// family out of the independent mention channel. If more than the bounded 16
// families exist, the exact omitted count and a bounded family roster travel
// in the caveat above; loss is never silent.
func boundTraceMarkSpansWithInfo(spans []TraceSpanSummary, max int) ([]TraceSpanSummary, traceMarkSemanticBoundInfo) {
	if max <= 0 || len(spans) <= max {
		return spans, traceMarkSemanticBoundInfo{}
	}
	var generic, semantic []TraceSpanSummary
	for _, span := range spans {
		if _, ok := traceSpanSemanticWorkClass(span.Name); ok {
			semantic = append(semantic, span)
		} else {
			generic = append(generic, span)
		}
	}
	if len(generic) > max {
		generic = generic[:max]
	}
	info := traceMarkSemanticBoundInfo{totalSpans: len(semantic)}
	if len(semantic) > traceMarkSemanticSpanCap {
		type semanticSeat struct {
			span     TraceSpanSummary
			family   string
			selected bool
		}
		seats := make([]semanticSeat, len(semantic))
		familySeen := map[string]bool{}
		familyKept := map[string]bool{}
		for i, span := range semantic {
			class := firstNonEmpty(strings.TrimSpace(span.SemanticClass), traceSpanSemanticClass(span.Name))
			family := threadKey(span.Thread) + "\x00" + class
			seats[i] = semanticSeat{span: span, family: family}
			familySeen[family] = true
			if familyKept[family] || info.keptSpans >= traceMarkSemanticSpanCap {
				continue
			}
			seats[i].selected = true
			familyKept[family] = true
			info.keptSpans++
		}
		// When fewer than 16 families exist, use the spare seats for the next
		// largest members so family totals remain as complete as the cap allows.
		for i := range seats {
			if info.keptSpans >= traceMarkSemanticSpanCap {
				break
			}
			if seats[i].selected {
				continue
			}
			seats[i].selected = true
			info.keptSpans++
		}
		semantic = semantic[:0]
		omittedSeen := map[string]bool{}
		for _, seat := range seats {
			if seat.selected {
				semantic = append(semantic, seat.span)
				continue
			}
			if familyKept[seat.family] || omittedSeen[seat.family] {
				continue
			}
			omittedSeen[seat.family] = true
			info.omittedFamilies++
			if len(info.omittedRoster) < 8 {
				info.omittedRoster = append(info.omittedRoster,
					fmt.Sprintf("%s@%s", firstNonEmpty(strings.TrimSpace(seat.span.SemanticClass), traceSpanSemanticClass(seat.span.Name), "unknown_class"), threadLabel(seat.span.Thread)))
			}
		}
		info.totalFamilies = len(familySeen)
	} else {
		families := map[string]bool{}
		for _, span := range semantic {
			class := firstNonEmpty(strings.TrimSpace(span.SemanticClass), traceSpanSemanticClass(span.Name))
			families[threadKey(span.Thread)+"\x00"+class] = true
		}
		info.totalFamilies = len(families)
		info.keptSpans = len(semantic)
	}
	out := make([]TraceSpanSummary, 0, len(generic)+len(semantic))
	out = append(out, generic...)
	out = append(out, semantic...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DurationMs != out[j].DurationMs {
			return out[i].DurationMs > out[j].DurationMs
		}
		return out[i].StartLine < out[j].StartLine
	})
	return out, info
}

func traceSpanFromEvents(start, end Event, kind, source string) TraceSpanSummary {
	return TraceSpanSummary{
		SourcePath: source,
		Thread:     threadRefFromEvent(start),
		Kind:       kind,
		Name:       start.SpanName,
		// LCK-2 (§18.E): the opening B row's payload pid — the emitter's OWN
		// pid-namespace process id — travels with the span so the rung-②
		// ns-span owner derivation can key a contention span to its container.
		SpanPID:       start.SpanPID,
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

func resolveSpanWindowsForQuery(idx *Index, q *Query, explicitStart, explicitEnd bool) ([]TraceSpanSummary, []string, *ViewCompaction) {
	if idx == nil || q == nil || strings.TrimSpace(q.SpanName) == "" {
		return nil, nil, nil
	}
	spans, caveats, compaction := findSpanWindowsCompacted(idx, *q, q.Limit)
	if len(spans) == 0 {
		return nil, append(caveats, fmt.Sprintf("span_name=%q matched no complete trace span in the selected filters; synchronous B/E spans close with unnamed E|<pid> or bare E on the same ftrace thread stack, and async S/F spans close by name+cookie, so do not search for E|<pid>|<span_name> as proof of an end marker", q.SpanName)), compaction
	}
	if len(spans) != 1 {
		if explicitStart && explicitEnd {
			return spans, caveats, compaction
		}
		return spans, append(caveats, fmt.Sprintf("span_name=%q matched %d span window(s); refine with pid/thread/line_start/line_end/time filters before deriving a root-cause window; for a specific frame id or marker, first run trace_query(view=\"event_search\", pattern=\"<frame id or exact label>\", event_types=[\"trace_mark\"]) and then rerun with the selected line/time window; do not narrow by searching E|<pid>|<span_name> because B/E end rows are unnamed", q.SpanName, len(spans))), compaction
	}
	span := spans[0]
	if explicitStart && explicitEnd {
		explicitWindow := TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}
		unioned := unionTimeWindows(explicitWindow, TimeWindow{StartTs: span.StartTs, EndTs: span.EndTs})
		if unioned.StartTs != explicitWindow.StartTs || unioned.EndTs != explicitWindow.EndTs {
			q.TimeStart, q.TimeEnd = unioned.StartTs, unioned.EndTs
			return spans, append(caveats, fmt.Sprintf("selected_window preserved explicit query window %.6f..%.6f and unioned it with matched span %q window %.6f..%.6f lines=%d-%d instead of shrinking to the explicit bounds", explicitWindow.StartTs, explicitWindow.EndTs, span.Name, span.StartTs, span.EndTs, span.StartLine, span.EndLine)), compaction
		}
		return spans, caveats, compaction
	}
	if !explicitStart {
		q.TimeStart = span.StartTs
	}
	if !explicitEnd {
		q.TimeEnd = span.EndTs
	}
	return spans, append(caveats, fmt.Sprintf("selected_window derived from unique trace span %q lines=%d-%d", span.Name, span.StartLine, span.EndLine)), compaction
}

func FindSpanWindows(idx *Index, q Query, max int) ([]TraceSpanSummary, []string) {
	spans, caveats, _ := findSpanWindowsCompacted(idx, q, max)
	return spans, caveats
}

// findSpanWindowsCompacted is FindSpanWindows plus the typed truncation
// record for the span cap, so Run/BuildFramePipeline can publish the
// compaction on their results instead of relying on caveat prose.
func findSpanWindowsCompacted(idx *Index, q Query, max int) ([]TraceSpanSummary, []string, *ViewCompaction) {
	if idx == nil {
		return nil, []string{"trace index is empty"}, nil
	}
	if failure := durationOrderFailureForFamily(idx, q, durationOrderTraceSpan); failure != nil {
		return nil, []string{durationOrderFailClosedCaveat(failure, "span_window")}, nil
	}
	if traceMarkUnknownEmitterFailureForQuery(idx, q) {
		caveats := traceMarkIntegrityCaveats(idx, q)
		caveats = append(caveats, "trace_mark_span_pairing_fail_closed=true; span_window is omitted because a malformed trace_mark endpoint has an unknown emitter, could not materialize as an Event, or overflowed the bounded witness ledger")
		return nil, caveats, nil
	}
	if max <= 0 {
		max = ViewCapacityFor("span_window").FloorLimit
	}
	var target ThreadRef
	if q.PID > 0 || strings.TrimSpace(q.Thread) != "" || strings.TrimSpace(q.ThreadInput) != "" {
		resolution := resolveThreadSelection(idx, q)
		if resolution.Ambiguous {
			return nil, []string{threadResolutionCaveat(idx, q)}, nil
		}
		target = resolution.Thread
		if target.PID > 0 {
			if conflict := threadIncarnationConflictForQuery(idx, q, target.PID); conflict != nil {
				return nil, []string{"thread_identity_fail_closed=true; " + conflict.reason() + "; target-scoped spans are omitted because the numeric TID spans task incarnations"}, nil
			}
		}
	}
	stacks := map[string][]Event{}
	var spans []TraceSpanSummary
	asyncPairer := newTraceMarkAsyncPairer(q, func(start Event) bool {
		return traceSpanStartMatchesQuery(start, target, q)
	}, func(pair traceMarkAsyncPair) {
		span := traceSpanFromEvents(pair.start, pair.end, "async", pair.source)
		if traceSpanMatchesQuery(span, target, q) {
			spans = append(spans, span)
		}
	})
	unresolvedPairingRows := 0
	for _, ev := range idx.Events {
		if resetPID, reset := schedulerLifecycleResetPID(ev); reset {
			source, ok := tracePairingSourceIdentity(idx, ev)
			if !ok {
				unresolvedPairingRows++
				continue
			}
			resetTraceMarkSyncPairingState(source, resetPID, stacks)
			asyncPairer.observeLifecycle(source, resetPID, ev)
			continue
		}
		if ev.Type != EventTraceMark {
			continue
		}
		if !eventLineInWindow(ev, q) {
			continue
		}
		if traceMarkEventMalformed(ev) {
			source, ok := tracePairingSourceIdentity(idx, ev)
			if !ok {
				unresolvedPairingRows++
				continue
			}
			resetTraceMarkSyncPairingState(source, ev.PID, stacks)
			asyncPairer.observeMalformed(source, ev.PID, ev)
			continue
		}
		var source string
		if ev.SpanAction == "B" || ev.SpanAction == "E" || ev.SpanAction == "S" || ev.SpanAction == "F" {
			var ok bool
			source, ok = tracePairingSourceIdentity(idx, ev)
			if !ok {
				unresolvedPairingRows++
				continue
			}
		}
		switch ev.SpanAction {
		case "B":
			key := traceMarkSyncPairingKey(source, ev.PID)
			stacks[key] = append(stacks[key], ev)
		case "E":
			key := traceMarkSyncPairingKey(source, ev.PID)
			stack := stacks[key]
			if len(stack) == 0 {
				continue
			}
			start := stack[len(stack)-1]
			stacks[key] = stack[:len(stack)-1]
			if ev.Ts < start.Ts {
				continue
			}
			span := traceSpanFromEvents(start, ev, "sync", source)
			if !traceSpanMatchesQuery(span, target, q) {
				continue
			}
			spans = append(spans, span)
		case "S":
			asyncPairer.observeEndpoint(source, ev)
		case "F":
			asyncPairer.observeEndpoint(source, ev)
		}
	}
	asyncPairer.finishEOF()
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].StartTs != spans[j].StartTs {
			return spans[i].StartTs < spans[j].StartTs
		}
		return spans[i].StartLine < spans[j].StartLine
	})
	caveats := traceMarkIntegrityCaveats(idx, q)
	caveats = append(caveats, asyncPairer.caveats()...)
	if unresolvedPairingRows > 0 {
		spans = nil
		caveats = append(caveats, fmt.Sprintf("trace_mark_pairing_provenance_unresolved=true; rows=%d; span_window durations were omitted because an endpoint or reset could not be mapped to exactly one physical source artifact", unresolvedPairingRows))
	}
	var compaction *ViewCompaction
	if len(spans) > max {
		last := spans[max-1]
		compaction = &ViewCompaction{
			View:            "span_window",
			Dimension:       CompactionDimensionSpans,
			Total:           len(spans),
			Emitted:         max,
			LastEmittedTs:   last.StartTs,
			LastEmittedLine: last.StartLine,
		}
		caveats = append(caveats, fmt.Sprintf("span_window compacted from %d to %d span(s)", len(spans), max))
		spans = spans[:max]
	}
	if len(spans) == 0 {
		caveats = append(caveats, "no complete trace spans matched the selected filters; B/E ends are unnamed E|<pid> or bare E on the same ftrace thread stack, and async S/F spans pair by name+cookie")
	}
	return spans, caveats, compaction
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

func traceSpanStartMatchesQuery(start Event, target ThreadRef, q Query) bool {
	if q.SpanName != "" && !strings.Contains(strings.ToLower(start.SpanName), strings.ToLower(strings.TrimSpace(q.SpanName))) {
		return false
	}
	if target.PID > 0 || target.Comm != "" {
		return threadMatches(target, start.PID, start.Comm)
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
			// Inventory only. This envelope says how widely the observed rows are
			// spread; it is never interrupt active time and never enters rank.
			burst.SpanMs = (burst.EndTs - burst.StartTs) * 1000
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
		// CPU + vector is the exact interrupt lane; exit rows often omit the
		// display name. An unrelated CPU's future timestamp must never flush or
		// split this lane's burst.
		key := fmt.Sprintf("%d/%d", ev.CPU, ev.IRQID)
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
		} else if burst.Name == "" || burst.Name == "irq" {
			burst.Name = name
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
		if bursts[i].SpanMs != bursts[j].SpanMs {
			return bursts[i].SpanMs > bursts[j].SpanMs
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

// interruptPairingLane carries the block-pairing cohort discipline for one
// exact interrupt lane (ENG audit #9, §29.25 处置委托 2026-07-10): one CPU's
// exact IRQ/softirq/IPI lane cannot physically self-nest, so depth>=2 can
// only come from event loss. LIFO-guessing such a lane minted OVERLAPPING
// pairs whose ActiveMs sum double-counted the overlap and booked unproven
// wall clock as hard active time (feeding root_cause_rank seats and
// supply_pressure). The whole cohort is withheld and disclosed instead.
type interruptPairingLane struct {
	open      []interruptOpen
	ambiguous bool
	// cohortStarts counts entry rows of the current cohort for the
	// suppression disclosure; inWindow marks window relevance of the cohort.
	cohortStarts int
	inWindow     bool
}

func computeInterruptActivity(idx *Index, q Query, typ EventType, coreByCPU map[int]string, max int) ([]InterruptActivity, []string) {
	if idx == nil {
		return nil, nil
	}
	accs := map[string]*InterruptActivity{}
	lanes := map[string]*interruptPairingLane{}
	unresolvedSourceRows := 0
	ambiguousCohorts, suppressedPairs, unpairedEntries := 0, 0, 0
	ensure := func(ev Event, baseName, key, source string) *InterruptActivity {
		name := firstNonEmpty(baseName, ev.IRQName, ev.Name, string(typ))
		item := accs[key]
		if item == nil {
			item = &InterruptActivity{
				SourcePath: source,
				Kind:       string(typ),
				CPU:        ev.CPU,
				CoreClass:  coreByCPU[ev.CPU],
				Vector:     ev.IRQID,
				Name:       name,
				LineStart:  ev.Line,
				LineEnd:    ev.Line,
				StartTs:    ev.Ts,
				EndTs:      ev.Ts,
			}
			accs[key] = item
		} else if (item.Name == "" || item.Name == string(typ) || strings.Contains(item.Name, "_exit")) && name != "" {
			item.Name = name
		}
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
	observe := func(ev Event, baseName, key, source string) *InterruptActivity {
		item := ensure(ev, baseName, key, source)
		item.Count++
		return item
	}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || ev.Type != typ {
			continue
		}
		baseName, phase := interruptBaseAndPhase(ev)
		lane, ok := interruptLaneKey(ev)
		if !ok || phase == "" {
			continue
		}
		source, sourceOK := tracePairingSourceIdentity(idx, ev)
		if !sourceOK {
			unresolvedSourceRows++
			continue
		}
		key := source + "\x00" + lane
		switch phase {
		case "instant":
			// Raise rows are point inventory. Unlike entry/exit pairs they have
			// no interval that can overlap the selected window.
			if timeInWindow(ev.Ts, q) {
				observe(ev, baseName, key, source)
			}
		case "entry":
			if timeInWindow(ev.Ts, q) {
				observe(ev, baseName, key, source)
			}
			lane := lanes[key]
			if lane == nil {
				lane = &interruptPairingLane{}
				lanes[key] = lane
			}
			lane.open = append(lane.open, interruptOpen{ev: ev})
			lane.cohortStarts++
			if timeInWindow(ev.Ts, q) {
				lane.inWindow = true
			}
			// ENG audit #9: this exact lane cannot physically self-nest —
			// depth>=2 proves event loss and the whole cohort becomes
			// ambiguous (block-pairing discipline) instead of LIFO-guessed.
			if len(lane.open) > 1 {
				lane.ambiguous = true
			}
		case "exit":
			var item *InterruptActivity
			if timeInWindow(ev.Ts, q) {
				item = observe(ev, baseName, key, source)
			}
			lane := lanes[key]
			if lane == nil || len(lane.open) == 0 {
				continue
			}
			if timeInWindow(ev.Ts, q) {
				lane.inWindow = true
			}
			if lane.ambiguous {
				lane.open = lane.open[:len(lane.open)-1]
				if len(lane.open) == 0 {
					if lane.inWindow {
						ambiguousCohorts++
						suppressedPairs += lane.cohortStarts
					}
					*lane = interruptPairingLane{}
				}
				continue
			}
			// Keying by exact type+CPU+vector first prevents a different lane
			// from closing this one; a non-ambiguous lane holds at most one
			// open entry.
			start := lane.open[len(lane.open)-1].ev
			*lane = interruptPairingLane{}
			if ev.Ts < start.Ts {
				continue
			}
			clipStart, clipEnd := start.Ts, ev.Ts
			if q.TimeEnd > q.TimeStart {
				var overlaps bool
				clipStart, clipEnd, overlaps = overlapTimeWindow(start.Ts, ev.Ts, q.TimeStart, q.TimeEnd)
				if !overlaps {
					continue
				}
			}
			// A malformed identity-less endpoint inside the physical pair can
			// change its LIFO nesting. Suppress only that affected pair; a bad
			// row before a later fully-contained exact pair does not poison the
			// unrelated future window.
			if interruptPairHasValidationFailure(idx, interruptDurationFamily(typ), start.Ts, ev.Ts, start.Line, ev.Line) {
				continue
			}
			if item == nil {
				item = ensure(start, baseName, key, source)
			}
			applyLineRange(&item.LineStart, &item.LineEnd, start.Line)
			applyLineRange(&item.LineStart, &item.LineEnd, ev.Line)
			if item.StartTs == 0 || start.Ts < item.StartTs {
				item.StartTs = start.Ts
			}
			if ev.Ts > item.EndTs {
				item.EndTs = ev.Ts
			}
			dur := (clipEnd - clipStart) * 1000
			item.PairedCount++
			item.ActiveMs += dur
			if dur > item.MaxActiveMs {
				item.MaxActiveMs = dur
			}
		}
	}
	// Drain: leftover open entries are either an ambiguous cohort that never
	// resolved (withhold + disclose) or window-relevant unpaired entries
	// (disclose, mirroring block's unpaired_start lane).
	for _, lane := range lanes {
		if len(lane.open) == 0 {
			continue
		}
		if lane.ambiguous {
			if lane.inWindow {
				ambiguousCohorts++
				suppressedPairs += lane.cohortStarts
			}
			continue
		}
		for _, open := range lane.open {
			if q.TimeEnd <= 0 || open.ev.Ts <= q.TimeEnd {
				unpairedEntries++
			}
		}
	}
	windowMs := (q.TimeEnd - q.TimeStart) * 1000
	out := make([]InterruptActivity, 0, len(accs))
	for _, item := range accs {
		// Only a typed entry/exit pair can mint active time. The old
		// first-to-last envelope fallback turned unrelated or incomplete rows
		// into a hard duration.
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
	var caveats []string
	if unresolvedSourceRows > 0 {
		caveats = append(caveats, fmt.Sprintf("interrupt_pairing_provenance_unresolved=true; family=%s rows=%d; endpoints without exactly one physical source artifact were excluded", typ, unresolvedSourceRows))
	}
	if ambiguousCohorts > 0 {
		caveats = append(caveats, fmt.Sprintf("interrupt_pairing_ambiguous=true; family=%s cohorts=%d pairing_suppressed=%d; same-lane nested entry/exit endpoints are physically impossible for one exact type+cpu+vector lane (event loss), so whole cohorts were withheld instead of LIFO-guessed", typ, ambiguousCohorts, suppressedPairs))
	}
	if unpairedEntries > 0 {
		caveats = append(caveats, fmt.Sprintf("interrupt_pairing_unpaired=true; family=%s unpaired_entry=%d; active time was emitted only for complete exact-lane pairs", typ, unpairedEntries))
	}
	return out, caveats
}

func interruptDurationFamily(typ EventType) durationOrderFamily {
	switch typ {
	case EventIRQ:
		return durationOrderIRQ
	case EventSoftIRQ:
		return durationOrderSoftIRQ
	case EventIPI:
		return durationOrderIPI
	default:
		return ""
	}
}

func interruptPairHasValidationFailure(idx *Index, family durationOrderFamily, startTs, endTs float64, startLine, endLine int) bool {
	if idx == nil || family == "" || endTs < startTs {
		return false
	}
	if idx.durationOrderFailuresCapped[family] {
		return true
	}
	for i := range idx.durationOrderFailures {
		failure := &idx.durationOrderFailures[i]
		if failure.Family != family || failure.Issue != "endpoint_parse_incomplete" {
			continue
		}
		// ENG audit #4b (§29.25 处置委托 2026-07-10): a malformed endpoint whose
		// timestamp itself did not parse (TsUnknown, CurrentTs=0) can never hit
		// the [startTs,endTs] interval — match it by its known physical line
		// instead (same-lane physical order tracks time), keeping the
		// suppression scoped to pairs it can actually corrupt.
		if failure.TsUnknown {
			if startLine > 0 && endLine >= startLine && failure.Line >= startLine && failure.Line <= endLine {
				return true
			}
			continue
		}
		if failure.CurrentTs >= startTs && failure.CurrentTs <= endTs {
			return true
		}
	}
	return false
}

func interruptBaseAndPhase(ev Event) (baseName, phase string) {
	name := strings.ToLower(strings.TrimSpace(ev.Name))
	baseName = firstNonEmpty(ev.IRQName, ev.Name)
	// Only the kernel's closed endpoint families are duration-bearing. Generic
	// irq_* / softirq_* observations stay inventory and cannot accidentally
	// become a pair merely because their name happens to share a suffix.
	switch ev.Type {
	case EventIRQ:
		switch name {
		case "irq_handler_entry":
			return baseName, "entry"
		case "irq_handler_exit":
			return baseName, "exit"
		}
	case EventSoftIRQ:
		switch name {
		case "softirq_raise":
			return baseName, "instant"
		case "softirq_entry":
			return baseName, "entry"
		case "softirq_exit":
			return baseName, "exit"
		}
	case EventIPI:
		switch name {
		case "ipi_raise":
			return baseName, "instant"
		case "ipi_entry":
			return baseName, "entry"
		case "ipi_exit":
			return baseName, "exit"
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
		ss := ev.SchedStatFields
		if ss == nil {
			ss = &SchedStatFields{}
		}
		thread := ThreadRef{Comm: firstNonEmpty(ss.Comm, ev.Comm), PID: firstNonZero(ss.PID, ev.PID), TGID: ev.TGID}
		kind := firstNonEmpty(ss.Kind, strings.TrimPrefix(ev.Name, "sched_stat_"), "unknown")
		key := threadKey(thread) + "/kind:" + kind
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
		delayMs := float64(ss.DelayNs) / 1e6
		runtimeMs := float64(ss.RunNs) / 1e6
		vruntimeMs := float64(ss.VRunNs) / 1e6
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

// selectedPairingCohortState keeps complete physical topology separate from
// query publication. Every endpoint participates in the shared cohort FSM;
// only selected endpoints contribute Count/unpaired/pair metrics. This makes a
// carry-in or window-external overlap able to suppress a false pair without
// importing its endpoint into the selected-window activity roster.
type selectedPairingCohortState struct {
	cohort         pairingCohortState
	startSelected  bool
	selectedStarts int
	selectedEvents int
}

type selectedPairingCohortTransition struct {
	pairingCohortTransition
	startSelected  bool
	doneSelected   bool
	selectedStarts int
	selectedEvents int
}

func (s *selectedPairingCohortState) observeStart(ev Event, selected bool) selectedPairingCohortTransition {
	if s == nil {
		return selectedPairingCohortTransition{}
	}
	if s.cohort.depth == 0 {
		s.startSelected = false
		s.selectedStarts = 0
		s.selectedEvents = 0
	}
	if selected {
		if s.cohort.depth == 0 {
			s.startSelected = true
		}
		s.selectedStarts++
		s.selectedEvents++
	}
	return selectedPairingCohortTransition{pairingCohortTransition: s.cohort.observeStart(ev)}
}

func (s *selectedPairingCohortState) observeDone(ev Event, selected bool) selectedPairingCohortTransition {
	if s == nil {
		return selectedPairingCohortTransition{}
	}
	if selected {
		s.selectedEvents++
	}
	transition := s.cohort.observeDone(ev)
	out := selectedPairingCohortTransition{
		pairingCohortTransition: transition,
		startSelected:           s.startSelected,
		doneSelected:            selected,
		selectedStarts:          s.selectedStarts,
		selectedEvents:          s.selectedEvents,
	}
	if transition.cohortClosed || transition.unpairedDone {
		s.startSelected = false
		s.selectedStarts = 0
		s.selectedEvents = 0
	}
	return out
}

func (s *selectedPairingCohortState) finishEOF() selectedPairingCohortTransition {
	if s == nil {
		return selectedPairingCohortTransition{}
	}
	out := selectedPairingCohortTransition{
		pairingCohortTransition: s.cohort.finishEOF(),
		startSelected:           s.startSelected,
		selectedStarts:          s.selectedStarts,
		selectedEvents:          s.selectedEvents,
	}
	s.startSelected = false
	s.selectedStarts = 0
	s.selectedEvents = 0
	return out
}

type durationPairingReplayEndpoint struct {
	eventIndex     int
	source         string
	verdict        PairingEndpointVerdict
	lifecycleReset bool
	resetPID       int
	work           string
	function       string
	driver         string
	timeline       string
	context        string
	seqno          string
}

type durationPairingReplayOwner struct {
	source string
	pid    int
}

func addDurationPairingReplayLane(owner durationPairingReplayOwner, key string, laneOwners map[string]durationPairingReplayOwner, ownerLanes map[durationPairingReplayOwner]map[string]struct{}) {
	laneOwners[key] = owner
	keys := ownerLanes[owner]
	if keys == nil {
		keys = map[string]struct{}{}
		ownerLanes[owner] = keys
	}
	keys[key] = struct{}{}
}

func dropDurationPairingReplayLane(key string, laneOwners map[string]durationPairingReplayOwner, ownerLanes map[durationPairingReplayOwner]map[string]struct{}) {
	owner, ok := laneOwners[key]
	if !ok {
		return
	}
	delete(laneOwners, key)
	keys := ownerLanes[owner]
	delete(keys, key)
	if len(keys) == 0 {
		delete(ownerLanes, owner)
	}
}

func durationPairingReplayOwnerLaneKeys(owner durationPairingReplayOwner, ownerLanes map[durationPairingReplayOwner]map[string]struct{}) []string {
	set := ownerLanes[owner]
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortDurationPairingReplayEndpoints(idx *Index, endpoints []durationPairingReplayEndpoint) {
	sort.SliceStable(endpoints, func(i, j int) bool {
		if endpoints[i].source != endpoints[j].source {
			return endpoints[i].source < endpoints[j].source
		}
		left, right := idx.Events[endpoints[i].eventIndex], idx.Events[endpoints[j].eventIndex]
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		return left.Ts < right.Ts
	})
}

func pairingEndpointSelected(ev Event, q Query) bool {
	return pairingEventInsideQuery(ev, q)
}

func computeWorkqueueActivity(idx *Index, q Query, max int, providedIntegrity ...*durationPairingIntegrity) ([]WorkqueueActivity, []string) {
	if idx == nil {
		return nil, nil
	}
	integrity := selectedDurationPairingIntegrity(idx, q, durationOrderWorkqueue, providedIntegrity)
	if integrity.familyGlobal {
		caveats := integrity.caveats("workqueue_activity")
		if integrity.unresolvedSources > 0 {
			caveats = append(caveats, fmt.Sprintf("workqueue_pairing_provenance_unresolved=true; rows=%d; rows without exactly one physical source artifact were excluded", integrity.unresolvedSources))
		}
		return nil, caveats
	}
	endpoints := make([]durationPairingReplayEndpoint, 0, 64)
	relevantPIDs := map[int]bool{}
	unresolvedSourceRows := 0
	unresolvedEndpointRows := 0
	unresolvedLifecycleResets := 0
	lifecycleResetLanes := 0
	invalidEndpointRows := 0
	var invalidSamples []string
	// Completeness pre-pass: exact endpoints are audited across the complete
	// retained topology. Query bounds must not delete a malformed barrier or a
	// carry-in start before the cohort state machine sees it.
	for eventIndex, ev := range idx.Events {
		if ev.Type != EventWorkqueue {
			continue
		}
		if _, phase := workqueueBaseAndPhase(ev.Name); phase == "" {
			continue
		}
		if ev.PID > 0 {
			relevantPIDs[ev.PID] = true
		}
		verdict, decoded := decodePairingEndpointWire(ev.Name, ev.FieldText, int64(ev.PID))
		work := decoded.work
		if verdict.Family != PairingEndpointWorkqueue || !verdict.KeyKnown || !verdict.PayloadAdmitted || !verdict.EmitterAdmitted {
			missing := workqueueEndpointMissingFields(ev, work)
			if len(missing) == 0 {
				missing = []string{"canonical_pairing_identity"}
			}
			invalidEndpointRows++
			if len(invalidSamples) < 4 {
				invalidSamples = append(invalidSamples, fmt.Sprintf("line=%d event=%s missing=%s", ev.Line, ev.Name, strings.Join(missing, ",")))
			}
			integrity.rejectEvent(idx, ev, verdict)
			continue
		}
		source, sourceOK := tracePairingSourceIdentity(idx, ev)
		if !sourceOK {
			unresolvedSourceRows++
			unresolvedEndpointRows++
			integrity.rejectEvent(idx, ev, verdict)
			continue
		}
		endpoints = append(endpoints, durationPairingReplayEndpoint{
			eventIndex: eventIndex, source: source, verdict: verdict,
			work: decoded.work, function: decoded.function,
		})
	}
	if !integrity.familyGlobal {
		for eventIndex, ev := range idx.Events {
			resetPID, reset := schedulerLifecycleResetPID(ev)
			if !reset || !relevantPIDs[resetPID] {
				continue
			}
			source, sourceOK := tracePairingSourceIdentity(idx, ev)
			if !sourceOK {
				unresolvedLifecycleResets++
				integrity.familyGlobal = true
				integrity.globalWitnesses++
				integrity.unresolvedSources++
				continue
			}
			endpoints = append(endpoints, durationPairingReplayEndpoint{eventIndex: eventIndex, source: source, lifecycleReset: true, resetPID: resetPID})
		}
	}
	if integrity.familyGlobal {
		caveats := integrity.caveats("workqueue_activity")
		if unresolvedSourceRows > 0 {
			caveats = append(caveats, fmt.Sprintf("workqueue_pairing_provenance_unresolved=true; rows=%d; rows without exactly one physical source artifact were excluded", unresolvedSourceRows))
		}
		if invalidEndpointRows > 0 || unresolvedEndpointRows > 0 {
			caveats = append(caveats, fmt.Sprintf("workqueue_pairing_invalid_endpoints=true; invalid_endpoints=%d unresolved_source_endpoints=%d samples=[%s]", invalidEndpointRows, unresolvedEndpointRows, strings.Join(invalidSamples, "; ")))
		}
		if unresolvedLifecycleResets > 0 {
			caveats = append(caveats, fmt.Sprintf("workqueue_pairing_lifecycle_reset_provenance_unresolved=true; rows=%d; workqueue elapsed pairing was fail-closed because a relevant task-incarnation reset could not be assigned to exactly one physical source artifact", unresolvedLifecycleResets))
		}
		return nil, caveats
	}
	sortDurationPairingReplayEndpoints(idx, endpoints)
	accs := map[string]*WorkqueueActivity{}
	lanes := map[string]*selectedPairingCohortState{}
	laneOwners := map[string]durationPairingReplayOwner{}
	ownerLanes := map[durationPairingReplayOwner]map[string]struct{}{}
	functionVariants := map[string]map[string]struct{}{}
	endpointKeys := map[string]bool{}
	observeSelected := func(ev Event, source, key, work, function string) *WorkqueueActivity {
		item := accs[key]
		if item == nil {
			item = &WorkqueueActivity{
				SourcePath: source,
				Thread:     threadRefFromEvent(ev),
				Work:       work,
				Function:   function,
				LineStart:  ev.Line,
				LineEnd:    ev.Line,
				StartTs:    ev.Ts,
				EndTs:      ev.Ts,
			}
			accs[key] = item
		}
		if function != "" {
			variants := functionVariants[key]
			if variants == nil {
				variants = map[string]struct{}{}
				functionVariants[key] = variants
			}
			variants[function] = struct{}{}
			if len(variants) == 1 {
				item.Function = function
			} else {
				item.Function = "multiple"
			}
		}
		item.Count++
		applyLineRange(&item.LineStart, &item.LineEnd, ev.Line)
		if item.StartTs == 0 || ev.Ts < item.StartTs {
			item.StartTs = ev.Ts
		}
		if ev.Ts > item.EndTs {
			item.EndTs = ev.Ts
		}
		return item
	}
	accountTransition := func(key string, transition selectedPairingCohortTransition) {
		if transition.unpairedDone {
			if transition.doneSelected {
				if item := accs[key]; item != nil {
					item.UnpairedDoneCount++
				}
			}
			return
		}
		if !transition.cohortClosed || transition.selectedEvents == 0 {
			return
		}
		item := accs[key]
		if item == nil {
			return
		}
		if transition.ambiguous {
			item.AmbiguousCohortCount++
			item.PairingSuppressedCount += transition.selectedStarts
			return
		}
		if transition.startSelected && transition.doneSelected {
			start, done := transition.pairStart, transition.last
			if done.Ts < start.Ts {
				item.PairingSuppressedCount++
				return
			}
			dur := (done.Ts - start.Ts) * 1000
			item.PairedCount++
			item.DurationMs += dur
			if dur > item.MaxLatencyMs {
				item.MaxLatencyMs = dur
			}
		} else if transition.startSelected {
			item.UnpairedStartCount++
		} else if transition.doneSelected {
			item.UnpairedDoneCount++
		}
	}
	// Inventory-only workqueue rows remain selected-window observations and
	// never enter elapsed pairing topology.
	for _, ev := range idx.Events {
		if !pairingEventInsideQuery(ev, q) || ev.Type != EventWorkqueue {
			continue
		}
		base, phase := workqueueBaseAndPhase(ev.Name)
		if phase != "" {
			continue
		}
		source, sourceOK := tracePairingSourceIdentity(idx, ev)
		if !sourceOK {
			unresolvedSourceRows++
			continue
		}
		if integrity.poisonedSources[source] {
			continue
		}
		work, function := workqueueFields(ev)
		key := encodePairingKey(source, strconv.Itoa(ev.PID), firstNonEmpty(work, "-"), base)
		key = encodePairingKey(key, "meta", firstNonEmpty(function, "-"))
		if integrity.poisonedLanes[key] {
			continue
		}
		observeSelected(ev, source, key, work, function)
	}
	for _, observation := range endpoints {
		if observation.lifecycleReset {
			owner := durationPairingReplayOwner{source: observation.source, pid: observation.resetPID}
			for _, key := range durationPairingReplayOwnerLaneKeys(owner, ownerLanes) {
				lane := lanes[key]
				accountTransition(key, lane.finishEOF())
				lifecycleResetLanes++
				delete(lanes, key)
				dropDurationPairingReplayLane(key, laneOwners, ownerLanes)
			}
			continue
		}
		ev := idx.Events[observation.eventIndex]
		key, keyOK := observation.verdict.LaneKey(observation.source)
		if !keyOK || integrity.poisonedSources[observation.source] || integrity.poisonedLanes[key] {
			continue
		}
		_, phase := workqueueBaseAndPhase(ev.Name)
		work, function := observation.work, observation.function
		selected := pairingEndpointSelected(ev, q)
		if selected {
			observeSelected(ev, observation.source, key, work, function)
			endpointKeys[key] = true
		}
		lane := lanes[key]
		if lane == nil {
			lane = &selectedPairingCohortState{}
			lanes[key] = lane
			addDurationPairingReplayLane(durationPairingReplayOwner{source: observation.source, pid: ev.PID}, key, laneOwners, ownerLanes)
		}
		var transition selectedPairingCohortTransition
		if phase == "start" {
			transition = lane.observeStart(ev, selected)
		} else {
			transition = lane.observeDone(ev, selected)
		}
		accountTransition(key, transition)
		if transition.unpairedDone || transition.cohortClosed {
			delete(lanes, key)
			dropDurationPairingReplayLane(key, laneOwners, ownerLanes)
		}
	}
	out := make([]WorkqueueActivity, 0, len(accs))
	var ambiguous, suppressed, unpairedStart, unpairedDone int
	for key, lane := range lanes {
		accountTransition(key, lane.finishEOF())
	}
	for _, item := range accs {
		// Count-only/unpaired rows stay visible with duration=0. A first/last
		// envelope is not a deterministic workqueue execution interval.
		item.Summary = fmt.Sprintf("workqueue thread=%s work=%s function=%s count=%d paired=%d unpaired_start=%d unpaired_done=%d ambiguous_cohorts=%d pairing_suppressed=%d duration=%.3fms max=%.3fms",
			threadLabel(item.Thread), item.Work, item.Function, item.Count, item.PairedCount, item.UnpairedStartCount, item.UnpairedDoneCount, item.AmbiguousCohortCount, item.PairingSuppressedCount, item.DurationMs, item.MaxLatencyMs)
		ambiguous += item.AmbiguousCohortCount
		suppressed += item.PairingSuppressedCount
		unpairedStart += item.UnpairedStartCount
		unpairedDone += item.UnpairedDoneCount
		out = append(out, *item)
	}
	functionVariantRows := 0
	for key, variants := range functionVariants {
		if len(variants) > 1 && endpointKeys[key] {
			functionVariantRows++
		}
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
	var caveats []string
	caveats = append(caveats, integrity.caveats("workqueue_activity")...)
	if unresolvedSourceRows > 0 {
		caveats = append(caveats, fmt.Sprintf("workqueue_pairing_provenance_unresolved=true; rows=%d; rows without exactly one physical source artifact were excluded", unresolvedSourceRows))
	}
	if ambiguous > 0 {
		caveats = append(caveats, fmt.Sprintf("workqueue_pairing_ambiguous=true; cohorts=%d pairing_suppressed=%d; overlapping identical work identities were withheld as whole cohorts instead of FIFO-guessed", ambiguous, suppressed))
	}
	if unpairedStart > 0 || unpairedDone > 0 {
		caveats = append(caveats, fmt.Sprintf("workqueue_pairing_unpaired=true; unpaired_start=%d unpaired_done=%d; elapsed time was emitted only for complete exact execute pairs", unpairedStart, unpairedDone))
	}
	if functionVariantRows > 0 {
		caveats = append(caveats, fmt.Sprintf("workqueue_function_variants=true; identities=%d; function=multiple means the same typed work pointer was observed with more than one function label in the selected window", functionVariantRows))
	}
	if invalidEndpointRows > 0 || unresolvedEndpointRows > 0 {
		caveats = append(caveats, fmt.Sprintf("workqueue_pairing_invalid_endpoints=true; invalid_endpoints=%d unresolved_source_endpoints=%d samples=[%s]", invalidEndpointRows, unresolvedEndpointRows, strings.Join(invalidSamples, "; ")))
	}
	if lifecycleResetLanes > 0 {
		caveats = append(caveats, fmt.Sprintf("workqueue_pairing_lifecycle_reset=true; lanes=%d; open workqueue lanes were closed as unpaired at exact task-generation boundaries instead of crossing TID reuse", lifecycleResetLanes))
	}
	return out, caveats
}

func workqueueFields(ev Event) (work, function string) {
	kv := parseKV(ev.FieldText)
	work = firstNonEmpty(kv["work"], kv["addr"], kv["address"])
	function = firstNonEmpty(kv["function"], kv["func"], kv["name"])
	if work == "" {
		work = valueAfterLabel(ev.FieldText, "work struct")
	}
	if function == "" {
		function = valueAfterLabel(ev.FieldText, "function")
	}
	work = strings.TrimRight(cleanTraceValue(work), ":")
	return work, cleanTraceValue(function)
}

func workqueueBaseAndPhase(name string) (base, phase string) {
	raw := strings.TrimSpace(name)
	if profile, ok := pairingEndpointProfileForName(raw); ok && profile.Family == PairingEndpointWorkqueue {
		return profile.SemanticBase, string(profile.Phase)
	}
	return strings.ToLower(raw), ""
}

func computeDMAFenceActivity(idx *Index, q Query, max int, providedIntegrity ...*durationPairingIntegrity) ([]DMAFenceActivity, []string) {
	if idx == nil {
		return nil, nil
	}
	integrity := selectedDurationPairingIntegrity(idx, q, durationOrderDMAFence, providedIntegrity)
	if integrity.familyGlobal {
		caveats := integrity.caveats("dma_fence_activity")
		if integrity.unresolvedSources > 0 {
			caveats = append(caveats, fmt.Sprintf("dma_fence_pairing_provenance_unresolved=true; rows=%d; rows without exactly one physical source artifact were excluded", integrity.unresolvedSources))
		}
		return nil, caveats
	}
	endpoints := make([]durationPairingReplayEndpoint, 0, 64)
	relevantPIDs := map[int]bool{}
	unresolvedSourceRows := 0
	unresolvedEndpointRows := 0
	unresolvedLifecycleResets := 0
	lifecycleResetLanes := 0
	invalidEndpointRows := 0
	var invalidSamples []string
	for eventIndex, ev := range idx.Events {
		if ev.Type != EventDMAFence {
			continue
		}
		if _, phase := dmaFenceBaseAndPhase(ev.Name); phase == "" {
			continue
		}
		if ev.PID > 0 {
			relevantPIDs[ev.PID] = true
		}
		verdict, decoded := decodePairingEndpointWire(ev.Name, ev.FieldText, int64(ev.PID))
		driver, timeline, context, seqno := decoded.driver, decoded.timeline, decoded.context, decoded.seqno
		if verdict.Family != PairingEndpointDMAFence || !verdict.KeyKnown || !verdict.PayloadAdmitted || !verdict.EmitterAdmitted {
			missing := dmaFenceEndpointMissingFields(ev, driver, timeline, context, seqno)
			if len(missing) == 0 {
				missing = []string{"canonical_pairing_identity"}
			}
			invalidEndpointRows++
			if len(invalidSamples) < 4 {
				invalidSamples = append(invalidSamples, fmt.Sprintf("line=%d event=%s missing=%s", ev.Line, ev.Name, strings.Join(missing, ",")))
			}
			integrity.rejectEvent(idx, ev, verdict)
			continue
		}
		source, sourceOK := tracePairingSourceIdentity(idx, ev)
		if !sourceOK {
			unresolvedSourceRows++
			unresolvedEndpointRows++
			integrity.rejectEvent(idx, ev, verdict)
			continue
		}
		endpoints = append(endpoints, durationPairingReplayEndpoint{
			eventIndex: eventIndex, source: source, verdict: verdict,
			driver: decoded.driver, timeline: decoded.timeline, context: decoded.context, seqno: decoded.seqno,
		})
	}
	if !integrity.familyGlobal {
		for eventIndex, ev := range idx.Events {
			resetPID, reset := schedulerLifecycleResetPID(ev)
			if !reset || !relevantPIDs[resetPID] {
				continue
			}
			source, sourceOK := tracePairingSourceIdentity(idx, ev)
			if !sourceOK {
				unresolvedLifecycleResets++
				integrity.familyGlobal = true
				integrity.globalWitnesses++
				integrity.unresolvedSources++
				continue
			}
			endpoints = append(endpoints, durationPairingReplayEndpoint{eventIndex: eventIndex, source: source, lifecycleReset: true, resetPID: resetPID})
		}
	}
	if integrity.familyGlobal {
		caveats := integrity.caveats("dma_fence_activity")
		if unresolvedSourceRows > 0 {
			caveats = append(caveats, fmt.Sprintf("dma_fence_pairing_provenance_unresolved=true; rows=%d; rows without exactly one physical source artifact were excluded", unresolvedSourceRows))
		}
		if invalidEndpointRows > 0 || unresolvedEndpointRows > 0 {
			caveats = append(caveats, fmt.Sprintf("dma_fence_pairing_invalid_endpoints=true; invalid_endpoints=%d unresolved_source_endpoints=%d samples=[%s]", invalidEndpointRows, unresolvedEndpointRows, strings.Join(invalidSamples, "; ")))
		}
		if unresolvedLifecycleResets > 0 {
			caveats = append(caveats, fmt.Sprintf("dma_fence_pairing_lifecycle_reset_provenance_unresolved=true; rows=%d; DMA fence elapsed pairing was fail-closed because a relevant task-incarnation reset could not be assigned to exactly one physical source artifact", unresolvedLifecycleResets))
		}
		return nil, caveats
	}
	sortDurationPairingReplayEndpoints(idx, endpoints)
	accs := map[string]*DMAFenceActivity{}
	lanes := map[string]*selectedPairingCohortState{}
	laneOwners := map[string]durationPairingReplayOwner{}
	ownerLanes := map[durationPairingReplayOwner]map[string]struct{}{}
	observeSelected := func(ev Event, source, key, driver, timeline, context, seqno string) *DMAFenceActivity {
		item := accs[key]
		if item == nil {
			item = &DMAFenceActivity{
				SourcePath: source,
				Thread:     threadRefFromEvent(ev),
				Driver:     driver,
				Timeline:   timeline,
				Context:    context,
				Seqno:      seqno,
				LineStart:  ev.Line,
				LineEnd:    ev.Line,
				StartTs:    ev.Ts,
				EndTs:      ev.Ts,
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
		return item
	}
	accountTransition := func(key string, transition selectedPairingCohortTransition) {
		if transition.unpairedDone {
			if transition.doneSelected {
				if item := accs[key]; item != nil {
					item.UnpairedDoneCount++
				}
			}
			return
		}
		if !transition.cohortClosed || transition.selectedEvents == 0 {
			return
		}
		item := accs[key]
		if item == nil {
			return
		}
		if transition.ambiguous {
			item.AmbiguousCohortCount++
			item.PairingSuppressedCount += transition.selectedStarts
			return
		}
		if transition.startSelected && transition.doneSelected {
			start, done := transition.pairStart, transition.last
			if done.Ts < start.Ts {
				item.PairingSuppressedCount++
				return
			}
			dur := (done.Ts - start.Ts) * 1000
			item.PairedCount++
			item.WaitMs += dur
			if dur > item.MaxWaitMs {
				item.MaxWaitMs = dur
			}
		} else if transition.startSelected {
			item.UnpairedStartCount++
		} else if transition.doneSelected {
			item.UnpairedDoneCount++
		}
	}
	// Non-endpoint DMA inventory remains query-scoped and cannot mint waits.
	for _, ev := range idx.Events {
		if !pairingEventInsideQuery(ev, q) || ev.Type != EventDMAFence {
			continue
		}
		base, phase := dmaFenceBaseAndPhase(ev.Name)
		if phase != "" {
			continue
		}
		source, sourceOK := tracePairingSourceIdentity(idx, ev)
		if !sourceOK {
			unresolvedSourceRows++
			continue
		}
		if integrity.poisonedSources[source] {
			continue
		}
		driver, timeline, context, seqno := dmaFenceFields(ev)
		key := encodePairingKey(source, strconv.Itoa(ev.PID), firstNonEmpty(driver, "-"), firstNonEmpty(timeline, "-"), firstNonEmpty(context, "-"), firstNonEmpty(seqno, "-"), base)
		if integrity.poisonedLanes[key] {
			continue
		}
		observeSelected(ev, source, key, driver, timeline, context, seqno)
	}
	for _, observation := range endpoints {
		if observation.lifecycleReset {
			owner := durationPairingReplayOwner{source: observation.source, pid: observation.resetPID}
			for _, key := range durationPairingReplayOwnerLaneKeys(owner, ownerLanes) {
				lane := lanes[key]
				accountTransition(key, lane.finishEOF())
				lifecycleResetLanes++
				delete(lanes, key)
				dropDurationPairingReplayLane(key, laneOwners, ownerLanes)
			}
			continue
		}
		ev := idx.Events[observation.eventIndex]
		key, keyOK := observation.verdict.LaneKey(observation.source)
		if !keyOK || integrity.poisonedSources[observation.source] || integrity.poisonedLanes[key] {
			continue
		}
		_, phase := dmaFenceBaseAndPhase(ev.Name)
		driver, timeline, context, seqno := observation.driver, observation.timeline, observation.context, observation.seqno
		selected := pairingEndpointSelected(ev, q)
		if selected {
			observeSelected(ev, observation.source, key, driver, timeline, context, seqno)
		}
		lane := lanes[key]
		if lane == nil {
			lane = &selectedPairingCohortState{}
			lanes[key] = lane
			addDurationPairingReplayLane(durationPairingReplayOwner{source: observation.source, pid: ev.PID}, key, laneOwners, ownerLanes)
		}
		var transition selectedPairingCohortTransition
		if phase == "start" {
			transition = lane.observeStart(ev, selected)
		} else {
			transition = lane.observeDone(ev, selected)
		}
		accountTransition(key, transition)
		if transition.unpairedDone || transition.cohortClosed {
			delete(lanes, key)
			dropDurationPairingReplayLane(key, laneOwners, ownerLanes)
		}
	}
	out := make([]DMAFenceActivity, 0, len(accs))
	var ambiguous, suppressed, unpairedStart, unpairedDone int
	for key, lane := range lanes {
		accountTransition(key, lane.finishEOF())
	}
	for _, item := range accs {
		// Count-only/unpaired rows stay visible with wait=0. Only a typed
		// start/end pair can mint fence wait time.
		item.Summary = fmt.Sprintf("dma_fence thread=%s driver=%s timeline=%s context=%s seqno=%s count=%d paired=%d unpaired_start=%d unpaired_done=%d ambiguous_cohorts=%d pairing_suppressed=%d wait=%.3fms max=%.3fms",
			threadLabel(item.Thread), item.Driver, item.Timeline, item.Context, item.Seqno, item.Count, item.PairedCount, item.UnpairedStartCount, item.UnpairedDoneCount, item.AmbiguousCohortCount, item.PairingSuppressedCount, item.WaitMs, item.MaxWaitMs)
		ambiguous += item.AmbiguousCohortCount
		suppressed += item.PairingSuppressedCount
		unpairedStart += item.UnpairedStartCount
		unpairedDone += item.UnpairedDoneCount
		out = append(out, *item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].WaitMs != out[j].WaitMs {
			return out[i].WaitMs > out[j].WaitMs
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	var caveats []string
	caveats = append(caveats, integrity.caveats("dma_fence_activity")...)
	if unresolvedSourceRows > 0 {
		caveats = append(caveats, fmt.Sprintf("dma_fence_pairing_provenance_unresolved=true; rows=%d; rows without exactly one physical source artifact were excluded", unresolvedSourceRows))
	}
	if ambiguous > 0 {
		caveats = append(caveats, fmt.Sprintf("dma_fence_pairing_ambiguous=true; cohorts=%d pairing_suppressed=%d; overlapping identical fence waits were withheld as whole cohorts instead of FIFO-guessed", ambiguous, suppressed))
	}
	if unpairedStart > 0 || unpairedDone > 0 {
		caveats = append(caveats, fmt.Sprintf("dma_fence_pairing_unpaired=true; unpaired_start=%d unpaired_done=%d; wait time was emitted only for complete exact wait pairs", unpairedStart, unpairedDone))
	}
	if invalidEndpointRows > 0 || unresolvedEndpointRows > 0 {
		caveats = append(caveats, fmt.Sprintf("dma_fence_pairing_invalid_endpoints=true; invalid_endpoints=%d unresolved_source_endpoints=%d samples=[%s]", invalidEndpointRows, unresolvedEndpointRows, strings.Join(invalidSamples, "; ")))
	}
	if lifecycleResetLanes > 0 {
		caveats = append(caveats, fmt.Sprintf("dma_fence_pairing_lifecycle_reset=true; lanes=%d; open DMA fence lanes were closed as unpaired at exact task-generation boundaries instead of crossing TID reuse", lifecycleResetLanes))
	}
	return out, caveats
}

func dmaFenceFields(ev Event) (driver, timeline, context, seqno string) {
	kv := parseKV(ev.FieldText)
	driver = firstNonEmpty(kv["driver"], kv["drv"], kv["name"])
	timeline = firstNonEmpty(kv["timeline"], kv["tl"], kv["fence_timeline"])
	context = firstNonEmpty(kv["context"], kv["ctx"], kv["fence_context"])
	seqno = firstNonEmpty(kv["seqno"], kv["sequence"], kv["fence_seqno"], kv["id"])
	return cleanTraceValue(driver), cleanTraceValue(timeline), cleanTraceValue(context), cleanTraceValue(seqno)
}

func dmaFenceBaseAndPhase(name string) (base, phase string) {
	raw := strings.TrimSpace(name)
	if profile, ok := pairingEndpointProfileForName(raw); ok && profile.Family == PairingEndpointDMAFence {
		return profile.SemanticBase, string(profile.Phase)
	}
	return strings.ToLower(raw), ""
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
		// Unadmitted mm_filemap names are searchable inventory, not a memory
		// cause category.  In particular mm_filemap_fault, suffix/case drift and
		// malformed exact add/delete rows must not regain rank/evidence through
		// the broad MemoryKind classifier after the typed mutation gate rejects
		// them.
		if strings.HasPrefix(strings.ToLower(ev.Name), "mm_filemap_") && pageCacheMutationKindForEvent(ev) == pageCacheMutationNone {
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
	// The two writeback rows are searchable EventFilesystem observations with
	// an explicit SubsystemKind, but they are not causal evidence or an IO
	// activity family.  Suppress the derived subsystem evidence carrier.
	if isWritebackObservation(ev) {
		return
	}
	if EROFSCoverageOnlyNameCandidate(ev.Name) {
		return
	}
	// MMC prefix/case/suffix drift and exact-but-malformed payloads remain raw
	// inventory/search observations. They must not be republished as generic
	// subsystem evidence after the exact pairing/IO gate rejected them.
	if !mmcSemanticPayloadAdmitted(ev) {
		return
	}
	if !f2fsSemanticPayloadAdmitted(ev) {
		return
	}
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

func accumulateRuntimeResource(bio, filesystem, pageFault map[string]*RuntimeResourceSummary, ev Event) string {
	kind := runtimeResourceKind(ev)
	if kind == "" {
		return ""
	}
	target := bio
	switch kind {
	case "filesystem":
		target = filesystem
	case "page_fault":
		target = pageFault
	}
	rf := ev.ResourceFields
	if rf == nil {
		rf = &ResourceFields{}
	}
	blk := ev.BlockIOFields
	if blk == nil {
		blk = &BlockIOFields{}
	}
	op := firstNonEmpty(rf.Op, blk.Op, ev.MemoryKind, ev.Name)
	path := firstNonEmpty(rf.Path, blk.Dev, rf.Address, "unknown")
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
			Callstack: rf.Callstack,
		}
		target[key] = item
	}
	item.Count++
	item.TotalLatencyMs += rf.LatencyMs
	if rf.LatencyMs > item.MaxLatencyMs {
		item.MaxLatencyMs = rf.LatencyMs
	}
	bytes := rf.Bytes
	if profile, exact := exactF2FSPairingProfile(ev.Name); exact && profile.Phase != PairingEndpointStart {
		// F2FS start/done both carry the request length. RuntimeResources is an
		// aggregate request account, so the completion row must not count the
		// same bytes a second time; copied/ret remain disclosure only.
		bytes = 0
	}
	item.Bytes = addSaturatedBytes(item.Bytes, bytes)
	if item.Line == 0 || ev.Line < item.Line {
		item.Line = ev.Line
		item.Ts = ev.Ts
	}
	if item.Example == "" {
		item.Example = clampString(ev.FieldText, 160)
	}
	if item.Callstack == "" {
		item.Callstack = rf.Callstack
	}
	if item.Address == "" {
		item.Address = rf.Address
	}
	return kind
}

func runtimeResourceKind(ev Event) string {
	if isWritebackObservation(ev) {
		return ""
	}
	if EROFSCoverageOnlyNameCandidate(ev.Name) {
		return ""
	}
	if !f2fsSemanticPayloadAdmitted(ev) {
		return ""
	}
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
	ff := ev.FileFields
	if ff == nil {
		ff = &FileFields{}
	}
	rf := ev.ResourceFields
	if rf == nil {
		rf = &ResourceFields{}
	}
	blk := ev.BlockIOFields
	if blk == nil {
		blk = &BlockIOFields{}
	}
	inode := firstNonEmpty(ff.Ino, "unknown")
	dev := firstNonEmpty(ff.Dev, blk.SrcDev, blk.Dev, "unknown")
	op := firstNonEmpty(ff.RW, rf.Op, fileOperationFromEventName(ev.Name), "io")
	key := strings.Join([]string{dev, inode, op, fmt.Sprintf("%d", ev.PID)}, "/")
	item := out[key]
	if item == nil {
		item = &FileIOSummary{
			Dev:         dev,
			Inode:       inode,
			ParentInode: ff.ParentIno,
			EntryName:   ff.Entry,
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
		if ff.Len > 0 {
			item.Bytes = addSaturatedBytes(item.Bytes, ff.Len)
		} else if rf.Bytes > 0 {
			item.Bytes = addSaturatedBytes(item.Bytes, rf.Bytes)
		}
	} else {
		item.CompletionCount++
	}
	item.TotalLatencyMs += rf.LatencyMs
	if rf.LatencyMs > item.MaxLatencyMs {
		item.MaxLatencyMs = rf.LatencyMs
	}
	if item.EntryName == "" && ff.Entry != "" {
		item.EntryName = ff.Entry
	}
	if item.ParentInode == "" && ff.ParentIno != "" {
		item.ParentInode = ff.ParentIno
	}
	if ff.Ret != 0 {
		item.Ret = ff.Ret
	}
	applyOffsetRange(&item.MinOffset, &item.MaxOffset, ff.Offset)
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
	kind := pageCacheMutationKindForEvent(ev)
	if kind == pageCacheMutationNone {
		return
	}
	ff := ev.FileFields
	if ff == nil {
		ff = &FileFields{}
	}
	rf := ev.ResourceFields
	if rf == nil {
		rf = &ResourceFields{}
	}
	blk := ev.BlockIOFields
	if blk == nil {
		blk = &BlockIOFields{}
	}
	inode := firstNonEmpty(ff.Ino, "unknown")
	dev := firstNonEmpty(ff.Dev, blk.SrcDev, blk.Dev, "unknown")
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
	switch kind {
	case pageCacheMutationDelete:
		item.Deletes++
	case pageCacheMutationAdd:
		item.Adds++
	default:
		// pageCacheMutationKind is a closed parse-time enum.  Future values
		// must acquire an explicit accounting ruling instead of defaulting to
		// an add mutation.
		return
	}
	item.Churn = item.Adds + item.Deletes
	if ff.Len > 0 {
		item.Bytes = addSaturatedBytes(item.Bytes, ff.Len)
	} else if rf.Bytes > 0 {
		item.Bytes = addSaturatedBytes(item.Bytes, rf.Bytes)
	}
	applyOffsetRange(&item.MinOffset, &item.MaxOffset, ff.Offset)
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

// topIOInodeGroupLimit caps the published TopIOInodes group rows; the fold
// itself always covers every group and TotalGroups discloses the full count
// (§28.6 ④ — truncation is never silent).
const topIOInodeGroupLimit = 10

// topIOInodeThreadContributorLimit caps the per-group per-thread latency
// contributor roster.
const topIOInodeThreadContributorLimit = 3

type topIOInodeAcc struct {
	item TopIOInodeSummary
	// perThread accumulates WITHIN-thread latency sums keyed by PID (the only
	// latency aggregation the wall-clock red line allows).
	perThread map[int]*TopIOInodeThreadLatency
	// threadPIDs is the distinct-thread census across both member families.
	threadPIDs map[int]bool
	// entryLine remembers which member donated EntryName so the earliest
	// (event-order) non-empty label wins deterministically.
	entryLine int
}

func (acc *topIOInodeAcc) observeThread(thread ThreadRef) {
	if thread.PID > 0 {
		acc.threadPIDs[thread.PID] = true
	}
}

func (acc *topIOInodeAcc) observeEntryName(name string, line int) {
	if name == "" {
		return
	}
	if acc.item.EntryName == "" || line < acc.entryLine {
		acc.item.EntryName = name
		acc.entryLine = line
	}
}

func (acc *topIOInodeAcc) observeEnvelope(lineStart, lineEnd int, startTs, endTs float64) {
	applyLineRange(&acc.item.LineStart, &acc.item.LineEnd, lineStart)
	applyLineRange(&acc.item.LineStart, &acc.item.LineEnd, lineEnd)
	if startTs > 0 && (acc.item.StartTs == 0 || startTs < acc.item.StartTs) {
		acc.item.StartTs = startTs
	}
	if endTs > acc.item.EndTs {
		acc.item.EndTs = endTs
	}
}

// computeTopIOInodes (INODE §28.6, 2026-07-09) folds the FULL pre-truncation
// fileIO/pageCache accumulator maps by (dev,inode): the PID and op key
// dimensions that shard the three legacy carriers are collapsed so "which
// inodes see the most IO" gets one whole-window frequency row per inode.
// MUST be fed the un-truncated maps (never the top-8 slices) — the
// block_io_by_inode carrier's built-on-truncated-input flaw is the audited
// anti-pattern this replaces for enumeration questions.
//
// Additivity (CLAUDE.md red line — wall clock is not additive across
// threads): event counts and byte counts sum across threads; latency only
// publishes the max single member event plus per-thread within-thread sums.
// Ordering: Count (frequency caliber, the customer's "高频" question) →
// Bytes → MaxLatencyMs → line/identity tie-breaks.
func computeTopIOInodes(fileIO map[string]*FileIOSummary, pageCache map[string]*PageCacheSummary, max int) *TopIOInodeStats {
	if len(fileIO) == 0 && len(pageCache) == 0 {
		return nil
	}
	// Deterministic fold order: map iteration is randomized, so members are
	// folded in event (line) order — EntryName donation and per-thread
	// ThreadRef identity never depend on map order.
	fileMembers := make([]*FileIOSummary, 0, len(fileIO))
	for _, member := range fileIO {
		fileMembers = append(fileMembers, member)
	}
	sort.SliceStable(fileMembers, func(i, j int) bool { return fileMembers[i].LineStart < fileMembers[j].LineStart })
	cacheMembers := make([]*PageCacheSummary, 0, len(pageCache))
	for _, member := range pageCache {
		cacheMembers = append(cacheMembers, member)
	}
	sort.SliceStable(cacheMembers, func(i, j int) bool { return cacheMembers[i].LineStart < cacheMembers[j].LineStart })

	accs := map[string]*topIOInodeAcc{}
	unidentified := 0
	get := func(dev, inode string) *topIOInodeAcc {
		key := dev + "\x00" + inode
		acc := accs[key]
		if acc == nil {
			acc = &topIOInodeAcc{
				item:       TopIOInodeSummary{Dev: dev, Inode: inode},
				perThread:  map[int]*TopIOInodeThreadLatency{},
				threadPIDs: map[int]bool{},
			}
			accs[key] = acc
		}
		return acc
	}
	for _, member := range fileMembers {
		events := member.Count + member.CompletionCount
		if member.Inode == "" || member.Inode == "unknown" {
			unidentified += events
			continue
		}
		acc := get(member.Dev, member.Inode)
		acc.item.Count += events
		acc.item.FileIOCount += member.Count
		acc.item.CompletionCount += member.CompletionCount
		// Closed-set op classification only (exact normalized tokens); every
		// other op stays in the total (the op domain is open-ended — raw rwbs
		// values and syscall names pass through the accumulator unmapped).
		switch member.Operation {
		case "read", "read_bio":
			acc.item.ReadCount += member.Count
		case "write", "write_bio":
			acc.item.WriteCount += member.Count
		}
		acc.item.Bytes = addSaturatedBytes(acc.item.Bytes, member.Bytes)
		if member.MaxLatencyMs > acc.item.MaxLatencyMs {
			acc.item.MaxLatencyMs = member.MaxLatencyMs
		}
		acc.observeThread(member.Thread)
		if member.TotalLatencyMs > 0 && member.Thread.PID > 0 {
			tl := acc.perThread[member.Thread.PID]
			if tl == nil {
				tl = &TopIOInodeThreadLatency{Thread: member.Thread}
				acc.perThread[member.Thread.PID] = tl
			}
			// Within-thread sum only: this member's events all belong to ONE
			// thread (the accumulator key carries the PID).
			tl.TotalLatencyMs += member.TotalLatencyMs
			tl.Count += events
		}
		acc.observeEntryName(member.EntryName, member.LineStart)
		acc.observeEnvelope(member.LineStart, member.LineEnd, member.StartTs, member.EndTs)
	}
	for _, member := range cacheMembers {
		events := member.Adds + member.Deletes
		if member.Inode == "" || member.Inode == "unknown" {
			unidentified += events
			continue
		}
		acc := get(member.Dev, member.Inode)
		acc.item.Count += events
		acc.item.PageCacheAdds += member.Adds
		acc.item.PageCacheDeletes += member.Deletes
		acc.observeThread(member.Thread)
		acc.observeEnvelope(member.LineStart, member.LineEnd, member.StartTs, member.EndTs)
	}
	if len(accs) == 0 && unidentified == 0 {
		return nil
	}
	groups := make([]TopIOInodeSummary, 0, len(accs))
	for _, acc := range accs {
		acc.item.PageCacheChurn = acc.item.PageCacheAdds + acc.item.PageCacheDeletes
		acc.item.ThreadCount = len(acc.threadPIDs)
		contributors := make([]TopIOInodeThreadLatency, 0, len(acc.perThread))
		for _, tl := range acc.perThread {
			contributors = append(contributors, *tl)
		}
		sort.SliceStable(contributors, func(i, j int) bool {
			if contributors[i].TotalLatencyMs != contributors[j].TotalLatencyMs {
				return contributors[i].TotalLatencyMs > contributors[j].TotalLatencyMs
			}
			return contributors[i].Thread.PID < contributors[j].Thread.PID
		})
		if len(contributors) > topIOInodeThreadContributorLimit {
			contributors = contributors[:topIOInodeThreadContributorLimit]
		}
		if len(contributors) > 0 {
			acc.item.TopThreadLatencies = contributors
		}
		acc.item.Summary = fmt.Sprintf("inode=%s dev=%s events=%d reads=%d writes=%d completions=%d bytes=%d page_cache_adds=%d page_cache_deletes=%d max_latency=%.3fms threads=%d",
			acc.item.Inode, acc.item.Dev, acc.item.Count, acc.item.ReadCount, acc.item.WriteCount, acc.item.CompletionCount, acc.item.Bytes, acc.item.PageCacheAdds, acc.item.PageCacheDeletes, acc.item.MaxLatencyMs, acc.item.ThreadCount)
		if acc.item.EntryName != "" {
			acc.item.Summary += " name=" + acc.item.EntryName
		}
		groups = append(groups, acc.item)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Count != groups[j].Count {
			return groups[i].Count > groups[j].Count
		}
		if groups[i].Bytes != groups[j].Bytes {
			return groups[i].Bytes > groups[j].Bytes
		}
		if groups[i].MaxLatencyMs != groups[j].MaxLatencyMs {
			return groups[i].MaxLatencyMs > groups[j].MaxLatencyMs
		}
		if groups[i].LineStart != groups[j].LineStart {
			return groups[i].LineStart < groups[j].LineStart
		}
		if groups[i].Dev != groups[j].Dev {
			return groups[i].Dev < groups[j].Dev
		}
		return groups[i].Inode < groups[j].Inode
	})
	total := len(groups)
	if max > 0 && len(groups) > max {
		groups = groups[:max]
	}
	return &TopIOInodeStats{Groups: groups, TotalGroups: total, UnidentifiedEvents: unidentified}
}

type storageLatencyAcc struct {
	item           StorageLatencySummary
	totalLatencyMs float64
}

type storageLatencyLane struct {
	cohort     pairingCohortState
	pairingKey string
	source     string
	identity   genericStorageIdentity
	eventCount int
	startBytes int64
	doneBytes  int64
	inode      string
	entryName  string
}

func computeStorageLatencyByLayer(idx *Index, q Query, blockSummaries []StorageLatencySummary, max int, providedIntegrity ...*durationPairingIntegrity) ([]StorageLatencySummary, []string) {
	integrity := selectedDurationPairingIntegrity(idx, q, durationOrderStorage, providedIntegrity)
	accs := map[string]*storageLatencyAcc{}
	lanes := map[string]*storageLatencyLane{}
	laneOwners := map[string]durationPairingReplayOwner{}
	ownerLanes := map[durationPairingReplayOwner]map[string]struct{}{}
	unresolvedSourceRows := 0
	lifecycleResetLanes := 0
	unresolvedLifecycleResets := 0
	var decodedByEvent map[int]genericStoragePairingDecoded
	if idx != nil && !integrity.familyGlobal {
		decodedByEvent = make(map[int]genericStoragePairingDecoded)
		for eventIndex, ev := range idx.Events {
			if !pairingReplayAuditEvent(ev, q) {
				continue
			}
			decoded := decodeGenericStoragePairingEvent(idx, ev)
			if !decoded.endpoint {
				continue
			}
			verdict := decoded.verdict
			if verdict.Family != PairingEndpointStorage || !verdict.KeyKnown || !verdict.PayloadAdmitted || !verdict.EmitterAdmitted {
				integrity.rejectEvent(idx, ev, verdict)
				continue
			}
			if !decoded.sourceKnown {
				integrity.rejectEvent(idx, ev, verdict)
				continue
			}
			if decoded.keyAdmitted {
				decodedByEvent[eventIndex] = decoded
			}
		}
	}
	replay := genericStorageReplayPlan{}
	if idx != nil && !integrity.familyGlobal {
		replay = buildGenericStorageReplayPlan(idx, q)
		unresolvedLifecycleResets = replay.unresolvedLifecycleResets
		if unresolvedLifecycleResets > 0 {
			integrity.familyGlobal = true
			integrity.globalWitnesses += unresolvedLifecycleResets
			integrity.unresolvedSources += unresolvedLifecycleResets
		}
	}
	if idx != nil && !integrity.familyGlobal {
		for _, eventIndex := range replay.eventIndexes {
			ev := idx.Events[eventIndex]
			if resetPID, reset := schedulerLifecycleResetPID(ev); reset {
				resetSource, _ := tracePairingSourceIdentity(idx, ev)
				owner := durationPairingReplayOwner{source: resetSource, pid: resetPID}
				for _, key := range durationPairingReplayOwnerLaneKeys(owner, ownerLanes) {
					lane := lanes[key]
					if lane == nil || lane.cohort.depth == 0 {
						delete(lanes, key)
						dropDurationPairingReplayLane(key, laneOwners, ownerLanes)
						continue
					}
					transition := lane.cohort.finishEOF()
					if pairingIntervalIntersectsQuery(transition.first, ev, q) {
						accountGenericStorageOpen(accs, lane, transition)
						lifecycleResetLanes++
					}
					delete(lanes, key)
					dropDurationPairingReplayLane(key, laneOwners, ownerLanes)
				}
			}
			decoded, keyOK := decodedByEvent[eventIndex]
			if !keyOK {
				_, _, endpoint := genericStorageEndpoint(ev)
				if !endpoint {
					continue
				}
				if pairingEventInsideQuery(ev, q) {
					unresolvedSourceRows++
				}
				continue
			}
			key, source, identity, phase := decoded.key, decoded.source, decoded.identity, decoded.phase
			if integrity.sourcePoisoned(source, ev.Name) {
				continue
			}
			if integrity.poisonedLanes[key] {
				continue
			}
			lane := lanes[key]
			if lane == nil {
				lane = &storageLatencyLane{pairingKey: key, source: source, identity: identity}
				lanes[key] = lane
				addDurationPairingReplayLane(durationPairingReplayOwner{source: source, pid: identity.PID}, key, laneOwners, ownerLanes)
			}
			if phase == "start" && lane.cohort.depth == 0 {
				lane.eventCount = 0
				lane.startBytes = 0
				lane.doneBytes = 0
				lane.inode = ""
				lane.entryName = ""
			}
			lane.eventCount++
			if phase == "start" {
				lane.startBytes = addSaturatedBytes(lane.startBytes, genericStorageEndpointBytes(ev))
			} else {
				lane.doneBytes = addSaturatedBytes(lane.doneBytes, genericStorageEndpointBytes(ev))
			}
			if ff := ev.FileFields; ff != nil {
				if lane.inode == "" {
					lane.inode = ff.Ino
				}
				if lane.entryName == "" {
					lane.entryName = ff.Entry
				}
			}
			var transition pairingCohortTransition
			switch phase {
			case "start":
				transition = lane.cohort.observeStart(ev)
			case "done":
				transition = lane.cohort.observeDone(ev)
			}
			accountGenericStorageTransition(accs, lane, transition, q)
			// A closed/idle cohort left its zero state behind; drop the lane
			// so map residency (and the lifecycle-reset scan above) tracks
			// CONCURRENT opens, not distinct identities seen (perf audit
			// #25). A later same-identity start recreates an identical fresh
			// lane, matching the old depth==0 metadata reset.
			if lane.cohort.depth == 0 {
				delete(lanes, key)
				dropDurationPairingReplayLane(key, laneOwners, ownerLanes)
			}
		}
	}
	out := append(make([]StorageLatencySummary, 0, len(blockSummaries)+len(accs)), blockSummaries...)
	var ambiguous, suppressed, unpairedStart, unpairedDone int
	for key, lane := range lanes {
		transition := lane.cohort.finishEOF()
		if pairingOpenCohortIntersectsIndex(transition.first, idx, q) {
			accountGenericStorageOpen(accs, lane, transition)
		}
		delete(lanes, key)
		dropDurationPairingReplayLane(key, laneOwners, ownerLanes)
	}
	for _, acc := range accs {
		if acc.item.PairedCount > 0 {
			acc.item.AvgLatencyMs = acc.totalLatencyMs / float64(acc.item.PairedCount)
		}
		acc.item.Summary = storageLatencySummaryText(acc.item)
		ambiguous += acc.item.AmbiguousCohortCount
		suppressed += acc.item.PairingSuppressedCount
		unpairedStart += acc.item.UnpairedStartCount
		unpairedDone += acc.item.UnpairedDoneCount
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
	var caveats []string
	caveats = append(caveats, integrity.caveats("storage_latency_by_layer(non_block)")...)
	if unresolvedEndpoints := integrity.unresolvedSources - unresolvedLifecycleResets; unresolvedEndpoints > 0 {
		caveats = append(caveats, fmt.Sprintf("storage_latency_pairing_provenance_unresolved=true; rows=%d; endpoints without exactly one physical source artifact were excluded", unresolvedEndpoints))
	}
	if unresolvedLifecycleResets > 0 {
		caveats = append(caveats, fmt.Sprintf("storage_latency_lifecycle_reset_provenance_unresolved=true; rows=%d; generic storage latency pairing was fail-closed because a relevant task-incarnation reset could not be assigned to exactly one physical source artifact", unresolvedLifecycleResets))
	}
	if unresolvedSourceRows > 0 {
		caveats = append(caveats, fmt.Sprintf("storage_latency_pairing_provenance_unresolved=true; rows=%d; endpoints without exactly one physical source artifact were excluded", unresolvedSourceRows))
	}
	if lifecycleResetLanes > 0 {
		caveats = append(caveats, fmt.Sprintf("storage_latency_pairing_lifecycle_reset=true; lanes=%d; open generic-storage lanes were closed as unpaired at exact task-generation boundaries instead of crossing TID reuse", lifecycleResetLanes))
	}
	if ambiguous > 0 {
		caveats = append(caveats, fmt.Sprintf("storage_latency_pairing_ambiguous=true; cohorts=%d pairing_suppressed=%d; overlapping identical coarse lanes were withheld as whole cohorts instead of FIFO-guessed", ambiguous, suppressed))
	}
	if unpairedStart > 0 || unpairedDone > 0 {
		caveats = append(caveats, fmt.Sprintf("storage_latency_pairing_unpaired=true; unpaired_start=%d unpaired_done=%d; elapsed latency was emitted only for complete exact-lane pairs", unpairedStart, unpairedDone))
	}
	return out, caveats
}

func addSaturatedBytes(total, value int64) int64 {
	if value <= 0 {
		return total
	}
	if total > math.MaxInt64-value {
		return math.MaxInt64
	}
	return total + value
}

func genericStorageCohortBytes(lane *storageLatencyLane) int64 {
	if lane == nil {
		return 0
	}
	// The six exact F2FS endpoints repeat one request length on both sides.
	// Count that request once. Other generic-storage profiles retain their
	// established per-physical-endpoint accounting; changing them here would be
	// an unrelated semantic migration hidden inside the F2FS authority batch.
	switch lane.identity.Base {
	case "f2fs_sync_file", "f2fs_direct_io", "f2fs_write":
		if lane.startBytes >= lane.doneBytes {
			return lane.startBytes
		}
		return lane.doneBytes
	}
	return addSaturatedBytes(lane.startBytes, lane.doneBytes)
}

func storageLatencyAccumulatorForIdentity(accs map[string]*storageLatencyAcc, lane *storageLatencyLane, ev Event) *storageLatencyAcc {
	if lane == nil || lane.pairingKey == "" {
		return nil
	}
	acc := storageLatencyAccumulator(accs, lane.pairingKey, lane.source, lane.identity.Layer, lane.identity.Base, lane.identity.Dev, lane.identity.Op, threadRefFromEvent(ev), ev.Line, ev.Ts, ev.FieldText)
	if acc.item.Inode == "" && lane.inode != "" {
		acc.item.Inode = lane.inode
	}
	if acc.item.EntryName == "" && lane.entryName != "" {
		acc.item.EntryName = lane.entryName
	}
	return acc
}

func observeGenericStorageEnvelope(acc *storageLatencyAcc, first, last Event) {
	if acc == nil {
		return
	}
	for _, ev := range []Event{first, last} {
		applyLineRange(&acc.item.LineStart, &acc.item.LineEnd, ev.Line)
		if acc.item.StartTs == 0 || ev.Ts < acc.item.StartTs {
			acc.item.StartTs = ev.Ts
		}
		if ev.Ts > acc.item.EndTs {
			acc.item.EndTs = ev.Ts
		}
	}
}

func accountGenericStorageTransition(accs map[string]*storageLatencyAcc, lane *storageLatencyLane, transition pairingCohortTransition, q Query) {
	if lane == nil {
		return
	}
	if transition.unpairedDone {
		if !pairingEventInsideQuery(transition.last, q) {
			return
		}
		acc := storageLatencyAccumulatorForIdentity(accs, lane, transition.last)
		if acc == nil {
			return
		}
		acc.item.Count += lane.eventCount
		acc.item.Bytes = addSaturatedBytes(acc.item.Bytes, genericStorageCohortBytes(lane))
		acc.item.UnpairedDoneCount++
		observeGenericStorageEnvelope(acc, transition.last, transition.last)
		return
	}
	if !transition.cohortClosed || !pairingIntervalIntersectsQuery(transition.first, transition.last, q) {
		return
	}
	acc := storageLatencyAccumulatorForIdentity(accs, lane, transition.first)
	if acc == nil {
		return
	}
	acc.item.Count += lane.eventCount
	acc.item.Bytes = addSaturatedBytes(acc.item.Bytes, genericStorageCohortBytes(lane))
	observeGenericStorageEnvelope(acc, transition.first, transition.last)
	if transition.ambiguous {
		acc.item.AmbiguousCohortCount++
		acc.item.PairingSuppressedCount += transition.cohortStarts
		return
	}
	if transition.last.Ts < transition.pairStart.Ts {
		acc.item.PairingSuppressedCount++
		return
	}
	dur := (transition.last.Ts - transition.pairStart.Ts) * 1000
	acc.item.PairedCount++
	acc.totalLatencyMs += dur
	if dur > acc.item.MaxLatencyMs {
		acc.item.MaxLatencyMs = dur
	}
}

func accountGenericStorageOpen(accs map[string]*storageLatencyAcc, lane *storageLatencyLane, transition pairingCohortTransition) {
	if lane == nil || !transition.cohortClosed || transition.cohortStarts == 0 {
		return
	}
	acc := storageLatencyAccumulatorForIdentity(accs, lane, transition.first)
	if acc == nil {
		return
	}
	acc.item.Count += lane.eventCount
	acc.item.Bytes = addSaturatedBytes(acc.item.Bytes, genericStorageCohortBytes(lane))
	observeGenericStorageEnvelope(acc, transition.first, transition.last)
	if transition.ambiguous {
		acc.item.AmbiguousCohortCount++
		acc.item.PairingSuppressedCount += transition.cohortStarts
		return
	}
	acc.item.UnpairedStartCount++
}

func storageLatencyAccumulator(accs map[string]*storageLatencyAcc, key, source, layer, event, dev, op string, thread ThreadRef, line int, ts float64, example string) *storageLatencyAcc {
	acc := accs[key]
	if acc != nil {
		return acc
	}
	acc = &storageLatencyAcc{item: StorageLatencySummary{
		SourcePath: source,
		Layer:      layer,
		Event:      event,
		Dev:        dev,
		Operation:  op,
		Thread:     thread,
		LineStart:  line,
		LineEnd:    line,
		StartTs:    ts,
		EndTs:      ts,
		Example:    clampString(example, 160),
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
		fileBytes = addSaturatedBytes(fileBytes, item.Bytes)
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
	var ioWaitMs float64
	for _, item := range stats.IOWaitTop {
		ioWaitMs += item.DurationMs
	}
	if blockMax == 0 && storageMax == 0 && fileBytes == 0 && fileEvents == 0 && pageChurn == 0 && stats.IOWaitBlockedCount == 0 && dStateMs == 0 && ioWaitMs == 0 {
		return nil
	}
	score := firstPositiveFloat(blockMax, storageMax) +
		float64(stats.IOWaitBlockedCount)*5 +
		dStateMs + ioWaitMs +
		float64(pageChurn)*0.2 +
		float64(fileEvents)*0.1 +
		float64(fileBytes)/(1024*1024)*2
	signal := "io_activity"
	switch {
	case (stats.IOWaitBlockedCount > 0 || ioWaitMs > 0) && (blockMax > 0 || storageMax > 0):
		signal = "scheduler_iowait_with_storage_latency"
	case ioWaitMs > 0:
		signal = "scheduler_iowait"
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
		IOWaitMs:            ioWaitMs,
		TopInode:            topInode,
		TopDev:              topDev,
		TopEntryName:        topName,
		LineStart:           lineStart,
		LineEnd:             lineEnd,
		Summary:             fmt.Sprintf("io pressure signal=%s score=%.3f block_max=%.3fms storage_max=%.3fms file_bytes=%d file_events=%d page_cache_churn=%d iowait_blocked=%d d_state=%.3fms io_wait=%.3fms top_inode=%s", signal, score, blockMax, storageMax, fileBytes, fileEvents, pageChurn, stats.IOWaitBlockedCount, dStateMs, ioWaitMs, firstNonEmpty(topInode, "unknown")),
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
		// Pairing identities canonicalize dev_t as major,minor while canonical
		// F2FS/file rows display major:minor. They are the same physical device;
		// normalize only the fold key so the storage latency rejoins its inode
		// row without rewriting the user-visible producer spelling.
		deviceKey := canonicalGenericStorageDevice(firstNonEmpty(dev, "unknown"))
		key := strings.Join([]string{deviceKey, firstNonEmpty(inode, "unknown"), fmt.Sprintf("%d", thread.PID)}, "/")
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
		acc.item.FileIOBytes = addSaturatedBytes(acc.item.FileIOBytes, file.Bytes)
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
	// DET-1: the output walk is sorted-key too (a map-order walk fed the
	// stable sort a random tie order), and the sort's tie chain ends on the
	// typed constant (dev,inode) identity — never map order.
	outKeys := make([]string, 0, len(accs))
	for key := range accs {
		outKeys = append(outKeys, key)
	}
	sort.Strings(outKeys)
	out := make([]BlockIOByInodeSummary, 0, len(accs))
	for _, key := range outKeys {
		item := accs[key].item
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
		if out[i].LineStart != out[j].LineStart {
			return out[i].LineStart < out[j].LineStart
		}
		if out[i].Dev != out[j].Dev {
			return out[i].Dev < out[j].Dev
		}
		return out[i].Inode < out[j].Inode
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func nearestBlockInodeForThread(accs map[string]*blockInodeAcc, thread ThreadRef, start, end float64) *blockInodeAcc {
	// DET-1 (v5 P1 批 追加件, 2026-07-13; 噪音从源头消除): the election walks
	// the accumulator map in SORTED-KEY order, so a distance TIE lands on the
	// same typed constant key (dev+inode) every run — the former map-order
	// walk flipped the storage_max/block_max attribution between inodes
	// run-to-run (donghu 0x14088d↔0x25a01 witness) and the caliber_side
	// member election / io_burst top-8 census / a rank tertiary subject
	// flipped with it. 帽/选举前确定性次序; tie-break = the sorted walk itself
	// (first key wins under strict `<`), never map order.
	keys := make([]string, 0, len(accs))
	for key := range accs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var best *blockInodeAcc
	bestDistance := 0.0
	found := false
	for _, key := range keys {
		acc := accs[key]
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
	for _, td := range stats.IOWaitTop {
		item := IOBurstEpisodeSummary{
			Thread:         td.Thread,
			DominantSignal: "scheduler_iowait",
			IOWaitMs:       td.DurationMs,
			DurationMs:     td.DurationMs,
			StartTs:        td.StartTs,
			EndTs:          td.EndTs,
			LineStart:      td.LineStart,
			LineEnd:        td.LineEnd,
			Confidence:     0.82,
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

func accumulateTracePluginEvent(ability, xpower, hiSystem map[string]*TracePluginSummary, ev Event) string {
	kind := tracePluginKind(ev.Type)
	if kind == "" {
		return ""
	}
	target := ability
	switch kind {
	case "xpower":
		target = xpower
	case "hi_sysevent":
		target = hiSystem
	}
	pl := ev.PluginFields
	if pl == nil {
		pl = &PluginFields{}
	}
	eventName := firstNonEmpty(pl.EventName, ev.Name)
	metric := firstNonEmpty(pl.Metric, ev.SubsystemKind, ev.Name)
	domain := firstNonEmpty(pl.Domain, ev.Comm)
	key := fmt.Sprintf("%s/%s/%s/%s/%s/%d", kind, domain, eventName, metric, pl.Value, ev.PID)
	item := target[key]
	if item == nil {
		item = &TracePluginSummary{
			Kind:      kind,
			Domain:    domain,
			EventName: eventName,
			Metric:    metric,
			Value:     pl.Value,
			Category:  pl.Category,
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
	return kind
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
	return buildWakeupChainWithCache(idx, q, newChainQueryCache(idx))
}

func buildWakeupChainWithCache(idx *Index, q Query, cache *chainQueryCache) ChainResult {
	q = normalizeQuery(idx, q)
	q = ensureQueryFlavor(idx, q)
	resolution := cache.resolveThreadSelection(q)
	target := resolution.Thread
	res := ChainResult{Target: target, Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}}
	res.Caveats = append(res.Caveats, cache.frequencyOrderCaveats()...)
	if target.PID == 0 && target.Comm == "" {
		if resolution.Ambiguous {
			res.Caveats = append(res.Caveats, threadResolutionCaveat(idx, q))
		}
		res.Caveats = append(res.Caveats, "target thread not found")
		return res
	}
	if conflict := threadIncarnationConflictForQuery(idx, q, 0); conflict != nil {
		res.Caveats = append(res.Caveats, "wakeup_chain_fail_closed=true; thread_identity_fail_closed=true; "+conflict.reason()+"; causal aggregation is omitted because a target/waker PID may denote multiple task incarnations")
		return res
	}
	tq := q
	tq.PID = target.PID
	tq.Thread = target.Comm
	targetTimeline := cache.timeline(tq, target)
	res.Caveats = dedupStrings(append(res.Caveats, targetTimeline.Caveats...))
	if targetTimeline.IntegrityFailure != "" {
		res.Caveats = append(res.Caveats, "wakeup_chain_fail_closed=true; target timeline integrity failure="+targetTimeline.IntegrityFailure+"; no trace_gap or causal edge is inferred from the discarded interval set")
		return res
	}
	branches, qualifyingBranches := interestingIntervals(targetTimeline.Intervals, q.MinDurationMs, q.MaxBranches)
	viaRaw := strings.TrimSpace(q.ViaThread)
	if qualifyingBranches > len(branches) && viaRaw == "" {
		res.Caveats = append(res.Caveats, fmt.Sprintf("target thread had %d candidate state segment(s) in the selected window; only the top %d (by duration and state priority) were expanded into the wakeup chain, %d lower-ranked segment(s) were not recursed into — widen max_branches, narrow the window, or re-run scoped to a specific sub-window if a dropped segment could be the real root cause", qualifyingBranches, len(branches), qualifyingBranches-len(branches)))
	}
	if len(branches) == 0 {
		visited := map[int]bool{}
		targetBlockedMs := (q.TimeEnd - q.TimeStart) * 1000
		expandChain(idx, q, cache, target, q.TimeStart, q.TimeEnd, 0, targetBlockedMs, visited, &res, "", nil, 1)
		res.AggregatedImpacts = aggregateWakeupCausalImpacts(&res)
		attachIPCGraphToChain(idx, q, &res)
		attachChainViaThreadReport(viaRaw, &res)
		return res
	}
	for i, branch := range branches {
		visited := map[int]bool{}
		targetBlockedMs := (branch.EndTs - branch.StartTs) * 1000
		expandChain(idx, q, cache, target, branch.StartTs, branch.EndTs, 0, targetBlockedMs, visited, &res, "", nil, i+1)
	}
	expandViaImmuneBranches(idx, q, cache, target, targetTimeline.Intervals, branches, qualifyingBranches, viaRaw, &res)
	res.AggregatedImpacts = aggregateWakeupCausalImpacts(&res)
	attachIPCGraphToChain(idx, q, &res)
	attachChainViaThreadReport(viaRaw, &res)
	return res
}

// expandViaImmuneBranches implements the RN-14a (§7.9) via_thread branch-cap
// immunity: target segments dropped by the MaxBranches top-N cap are expanded
// anyway, and each expansion is KEPT only when its wakeup subtree contains the
// via thread (canonical-exact match) — otherwise the expansion is rolled back
// wholesale so non-via results stay byte-identical to the capped chain. The
// dropped-segment caveat is re-issued here with the post-immunity count so it
// never claims a via-expanded segment "was not recursed into".
func expandViaImmuneBranches(idx *Index, q Query, cache *chainQueryCache, target ThreadRef, intervals []Interval, branches []Interval, qualifyingBranches int, viaRaw string, res *ChainResult) {
	if viaRaw == "" || qualifyingBranches <= len(branches) {
		return
	}
	sel := parseThreadSelector(viaRaw)
	all, _ := interestingIntervals(intervals, q.MinDurationMs, qualifyingBranches)
	overflow := all[len(branches):]
	viaKept := 0
	// P0-E CHAIN-PATH: via-immune expansions continue the branch numbering
	// after the capped top-N set; a rolled-back expansion leaves a gap in the
	// ordinals (branch identity is an id, not a dense index).
	nextBranch := len(branches)
	for _, branch := range overflow {
		nodesMark, edgesMark := len(res.Nodes), len(res.Edges)
		impactsMark, rootsMark, caveatsMark := len(res.CausalImpacts), len(res.RootEvidence), len(res.Caveats)
		visited := map[int]bool{}
		targetBlockedMs := (branch.EndTs - branch.StartTs) * 1000
		nextBranch++
		expandChain(idx, q, cache, target, branch.StartTs, branch.EndTs, 0, targetBlockedMs, visited, res, "", nil, nextBranch)
		contains := false
		for _, node := range res.Nodes[nodesMark:] {
			if chainThreadMatchesViaSelector(sel, node.Thread) {
				contains = true
				break
			}
		}
		if !contains {
			res.Nodes = res.Nodes[:nodesMark]
			res.Edges = res.Edges[:edgesMark]
			res.CausalImpacts = res.CausalImpacts[:impactsMark]
			res.RootEvidence = res.RootEvidence[:rootsMark]
			res.Caveats = res.Caveats[:caveatsMark]
			continue
		}
		viaKept++
	}
	if remaining := qualifyingBranches - len(branches) - viaKept; remaining > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("target thread had %d candidate state segment(s) in the selected window; only the top %d (by duration and state priority) were expanded into the wakeup chain, %d lower-ranked segment(s) were not recursed into — widen max_branches, narrow the window, or re-run scoped to a specific sub-window if a dropped segment could be the real root cause", qualifyingBranches, len(branches)+viaKept, remaining))
	}
	if viaKept > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("via_thread=%s branch-cap immunity: expanded %d additional segment(s) beyond max_branches because their wakeup subtree contains the via thread", viaRaw, viaKept))
	}
}

// chainThreadMatchesViaSelector is the RN-14a canonical-exact via matcher:
// a parsed pid (bare integer, pid=N token, bracket form, or trailing -N label
// segment) must equal the thread pid exactly; otherwise the cleaned selector
// name must equal the thread comm verbatim. No substring or fuzzy matching.
func chainThreadMatchesViaSelector(sel threadSelector, thread ThreadRef) bool {
	if sel.HasPID {
		return thread.PID > 0 && thread.PID == sel.PID
	}
	name := strings.TrimSpace(sel.Name)
	return name != "" && thread.Comm == name
}

// attachChainViaThreadReport publishes the RN-14a via verdict onto the chain.
// Both outcomes are affirmative statements (§7.4 wording discipline): ON path
// reports depth and per-hop wakeup latency from the existing edges; NOT on
// path states that the via thread's influence in this window is scheduling
// contention (runnable queuing), not a wakeup dependency.
func attachChainViaThreadReport(viaRaw string, res *ChainResult) {
	if viaRaw == "" || res == nil {
		return
	}
	sel := parseThreadSelector(viaRaw)
	report := &ChainViaThreadReport{Requested: viaRaw}
	var viaThread ThreadRef
	found := false
	for _, node := range res.Nodes {
		if chainThreadMatchesViaSelector(sel, node.Thread) {
			viaThread = node.Thread
			found = true
			break
		}
	}
	if !found {
		report.OnChain = false
		report.Summary = fmt.Sprintf("via_thread %s NOT on any wakeup path to %s in this window; its influence is scheduling contention only (runnable queuing), not a wakeup dependency", viaRaw, threadLabel(res.Target))
		res.ViaThread = report
		return
	}
	report.OnChain = true
	report.Thread = viaThread
	depth := -1
	for _, impact := range res.CausalImpacts {
		if impact.Thread.PID == viaThread.PID && (depth < 0 || impact.ChainDepth < depth) {
			depth = impact.ChainDepth
		}
	}
	hops, complete := viaMonotonicHops(res, viaThread)
	report.Hops = hops
	if depth < 0 {
		depth = len(report.Hops)
	}
	report.Depth = depth
	hopParts := make([]string, 0, len(report.Hops))
	for _, hop := range report.Hops {
		hopParts = append(hopParts, fmt.Sprintf("%s->%s=%.3fms", threadLabel(hop.Waker), threadLabel(hop.Wakee), hop.LatencyMs))
	}
	perHop := "n/a"
	if len(hopParts) > 0 {
		perHop = strings.Join(hopParts, ", ")
	}
	report.Summary = fmt.Sprintf("via_thread %s ON wakeup path: depth=%d, per-hop latency %s", threadLabel(viaThread), depth, perHop)
	if !complete {
		report.Summary += ";跨分支,逐跳序不可得 — no time-consistent (non-decreasing wakeup_ts) edge sequence reaches the target, hop list shows the reachable prefix only"
	}
	res.ViaThread = report
}

// viaMonotonicHops walks res.Edges from the via thread down to the target
// under the 2026-07-04 review rules:
//
//   - F4 (time order): every hop's WakeupTs must be >= the previous hop's
//     (exact float comparison, no tolerance). On a REAL single branch wakeup
//     causality flows toward the target in non-decreasing time (a waker's own
//     wakeup precedes the wakeup it delivers), so a walk that steps backwards
//     in time is a cross-branch stitch — an IMPOSSIBLE sequence the old
//     "first matching edge per hop" greedy walk happily fabricated
//     (counterexample: hop at t=7.9 followed by a hop at t=1.0 from another
//     branch expansion).
//   - F5 (depth consistency): among the time-consistent paths the SHORTEST
//     one is returned (BFS by hop count), so the walk descends the same
//     branch that produced report.Depth = min ChainDepth — the old walk took
//     the expansion-order first edge and could pair depth=1 with a 2-hop
//     walk from a lower branch.
//
// When no time-consistent complete path exists, the fallback is a greedy
// monotonic walk (each hop: the EARLIEST candidate edge not before the
// previous hop) truncated at the first dead end, and complete=false — the
// caller annotates the truncation instead of stitching an impossible order.
func viaMonotonicHops(res *ChainResult, via ThreadRef) ([]ChainViaHop, bool) {
	if via.PID <= 0 || res.Target.PID <= 0 || via.PID == res.Target.PID {
		return nil, true
	}
	// Adjacency: waker pid → edge indices, each list sorted by (WakeupTs,
	// original index) so both walks pick candidates deterministically.
	adj := map[int][]int{}
	for i := range res.Edges {
		if pid := res.Edges[i].Waker.PID; pid > 0 && res.Edges[i].Wakee.PID > 0 {
			adj[pid] = append(adj[pid], i)
		}
	}
	for pid := range adj {
		idxs := adj[pid]
		sort.SliceStable(idxs, func(a, b int) bool { return res.Edges[idxs[a]].WakeupTs < res.Edges[idxs[b]].WakeupTs })
	}
	hopFromEdge := func(e *WakeupEdge) ChainViaHop {
		return ChainViaHop{
			Waker:      e.Waker,
			Wakee:      e.Wakee,
			LatencyMs:  e.LatencyMs,
			WakeupTs:   e.WakeupTs,
			WakeupLine: e.WakeupLine,
		}
	}
	// BFS over (pid, lastTs) states. Dominance: reaching a pid again is only
	// useful with a strictly SMALLER lastTs (a smaller lastTs admits a
	// superset of monotonic continuations; BFS order already guarantees the
	// hop count is not better), which also bounds the search — bestTs per pid
	// only ever decreases through the finite set of edge timestamps.
	type viaPathState struct {
		pid    int
		lastTs float64
		hops   []int
	}
	queue := []viaPathState{{pid: via.PID, lastTs: math.Inf(-1)}}
	bestTs := map[int]float64{via.PID: math.Inf(-1)}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.pid == res.Target.PID {
			hops := make([]ChainViaHop, 0, len(cur.hops))
			for _, ei := range cur.hops {
				hops = append(hops, hopFromEdge(&res.Edges[ei]))
			}
			return hops, true
		}
		for _, ei := range adj[cur.pid] {
			e := &res.Edges[ei]
			if e.WakeupTs < cur.lastTs {
				continue
			}
			if prev, ok := bestTs[e.Wakee.PID]; ok && prev <= e.WakeupTs {
				continue
			}
			bestTs[e.Wakee.PID] = e.WakeupTs
			next := viaPathState{pid: e.Wakee.PID, lastTs: e.WakeupTs, hops: append(append([]int(nil), cur.hops...), ei)}
			queue = append(queue, next)
		}
	}
	// No monotonic path reaches the target: truncated greedy monotonic walk.
	var hops []ChainViaHop
	current := via.PID
	lastTs := math.Inf(-1)
	seen := map[int]bool{}
	for current > 0 && current != res.Target.PID && !seen[current] {
		seen[current] = true
		next := -1
		for _, ei := range adj[current] {
			if res.Edges[ei].WakeupTs >= lastTs {
				next = ei
				break
			}
		}
		if next < 0 {
			break
		}
		e := &res.Edges[next]
		hops = append(hops, hopFromEdge(e))
		lastTs = e.WakeupTs
		current = e.Wakee.PID
	}
	return hops, false
}

// wakeupCausalAggregateGroupKey is the ONE aggregation-group identity —
// aggregateWakeupCausalImpacts groups member occurrences by it and the ORD-A
// member-suppression arm in buildRootCauseRankFrom tests membership against
// it (single source: the two sides can never drift apart).
func wakeupCausalAggregateGroupKey(pid int, dominantState string) string {
	return fmt.Sprintf("%d/%s", pid, dominantState)
}

// chainRankAggregateCensus returns the FULL pre-trim aggregate census when the
// chain was built in-process (the only production path — all three rank build
// sites consume the ChainResult the aggregation pass just wrote); a chain
// without it (e.g. JSON-roundtripped in a test harness) degrades to the
// trimmed view, i.e. exactly the pre-ORD behavior.
func chainRankAggregateCensus(chain ChainResult) []WakeupCausalAggregate {
	if len(chain.rankAggregateCensus) > 0 {
		return chain.rankAggregateCensus
	}
	return chain.AggregatedImpacts
}

// rankSeatAggregates resolves which aggregates hold rank seats (ORD, ledger
// §29.8 P2③/§29.11 补充, 2026-07-10):
//   - the FULL census feeds seat allocation — the AggregatedImpacts top-8
//     trim is a view-capacity measure (PTS derived view) and never a seat
//     gate (aggregate top-8 折叠吞携榜席成员 修根);
//   - a typed PeriodicSource aggregate BYPASSES the intermediate-sleep skip:
//     a periodic signal source competes with its DISCOUNTED attribution
//     (runnable + lateness, §7.8 VS-1; §28.7 G9 复核纠偏 "周期源保留席位,
//     恢复 VS-1 参赛形" — huadong_792 E12 VSyncGenerator held no seat at all
//     while every board/lead/❶ gate keys on Rank>0). Non-periodic
//     intermediate sleeps stay seatless — chain plumbing whose wait is
//     explained by ITS upstream.
func rankSeatAggregates(chain ChainResult) []WakeupCausalAggregate {
	var out []WakeupCausalAggregate
	for _, aggregate := range chainRankAggregateCensus(chain) {
		if aggregate.ChainDepth <= 0 || aggregate.DominantImpactMs <= 0 {
			continue
		}
		if isIntermediateSleepAggregate(chain, aggregate) && !aggregate.PeriodicSource {
			continue
		}
		out = append(out, aggregate)
	}
	return out
}

func aggregateWakeupCausalImpacts(chain *ChainResult) []WakeupCausalAggregate {
	type acc struct {
		item      WakeupCausalAggregate
		prioVotes map[string]int
		invCount  int
		// members indexes this aggregate's occurrences in chain.CausalImpacts
		// so VS-1 periodic detection can stamp the member rows in place.
		members []int
	}
	accs := map[string]*acc{}
	for idx := range chain.CausalImpacts {
		impact := chain.CausalImpacts[idx]
		if impact.Thread.PID <= 0 || impact.ChainDepth <= 0 || impact.TotalMs <= 0 || strings.TrimSpace(impact.DominantState) == "" {
			continue
		}
		key := wakeupCausalAggregateGroupKey(impact.Thread.PID, impact.DominantState)
		a := accs[key]
		if a == nil {
			a = &acc{prioVotes: map[string]int{}}
			a.item.Thread = impact.Thread
			a.item.ChainDepth = impact.ChainDepth
			// P0-E CHAIN-PATH (ledger §22.1): the aggregate carries a branch
			// identity only when EVERY member was measured in the same branch;
			// a cross-branch aggregate stays 0 (absence never guesses).
			a.item.ChainBranch = impact.ChainBranch
			a.item.DominantState = impact.DominantState
			a.item.Path = wakeupChainPathFromThread(*chain, impact.Thread)
			accs[key] = a
		}
		a.members = append(a.members, idx)
		a.item.OccurrenceCount++
		if impact.ChainDepth < a.item.ChainDepth {
			a.item.ChainDepth = impact.ChainDepth
		}
		if a.item.ChainBranch != impact.ChainBranch {
			a.item.ChainBranch = 0
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
		// VS-2 (§7.10): the shared merge authority keeps the fold's numeric
		// vector and permitted aggregate provenance together. Overlap
		// reconciliation invokes the same helper after selecting one complete
		// physical occurrence vector per overlap-connected cohort.
		addWakeupAggregateSupplyFold(&a.item, impact)
		a.item.OccurrenceWindows = append(a.item.OccurrenceWindows, wakeupCausalOccurrenceFromImpact(impact))
	}
	var out []WakeupCausalAggregate
	for _, a := range accs {
		if a.item.OccurrenceCount < 2 {
			continue
		}
		// Periodic-source detection works from recurring occurrence starts, but
		// cadence does not prove the projected windows are disjoint. Detect on
		// the raw members, then reconcile every aggregate through the same
		// overlap-safe path and recompute the derived periodic account.
		a.item.DominantState, a.item.DominantImpactMs = dominantAggregateState(a.item)
		detectPeriodicWakeupSource(chain, &a.item, a.members)
		reconcileWakeupAggregateOccurrenceOverlap(chain, &a.item, a.members)
		a.item.DominantState, a.item.DominantImpactMs = dominantAggregateState(a.item)
		if a.item.PeriodicSource {
			totalLateness := reconciledWakeupAggregatePeriodicLateness(chain, a.members)
			rawBlocking := aggregateBlockingMs(a.item)
			maxLateness := math.Max(rawBlocking-a.item.RunnableMs, 0)
			if totalLateness > maxLateness {
				totalLateness = maxLateness
			}
			a.item.LatenessMs = totalLateness
			a.item.EffectivePeriodicImpactMs = minPositiveCapFloat(a.item.RunnableMs+totalLateness, rawBlocking)
		}
		a.item.ProjectedImpactMs = aggregateBlockingMs(a.item)
		a.item.ActualImpactMs = actualAggregateBlockingMs(a.item)
		a.item.PriorityRelation = mostFrequentString(a.prioVotes)
		a.item.PriorityInversion = a.invCount > 0
		applyAggregateGatedInversion(chain, &a.item, a.members)
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
	// ORD (§29.8 P2③, 2026-07-10): the FULL census survives on the
	// engine-internal chain field BEFORE the view trim below — seat
	// allocation reads it so the top-8 trim stays a pure display-capacity
	// measure and never swallows a family's rank seat.
	chain.rankAggregateCensus = out
	if len(out) > 8 {
		// PTS (#68 用户裁定 2026-07-05, 零静默丢弃披露): the aggregate list is a
		// DERIVED view over CausalImpacts (the per-hop rows remain complete);
		// the top-8 trim states its count instead of vanishing. Per-pair
		// occurrence WINDOWS are already fold+count by construction:
		// OccurrenceCount keeps the full count while OccurrenceWindows keeps
		// the top wakeupCausalAggregateOccurrenceCap spans.
		chain.Caveats = append(chain.Caveats, fmt.Sprintf(
			"aggregated_impacts kept top 8 of %d (derived view; per-hop causal impact rows remain complete)", len(out)))
		// PTS-2 (#69 用户条件裁定 2026-07-06, 评估无险后实施 — 账本 §7.1): the
		// rank>8 overflow additionally folds into ONE bounded synthetic member
		// so the projection tree can render the engine-level fold row (count +
		// range + roster) instead of a caveat-only disclosure. O(1) by
		// construction: scalars + ≤8 labels; the headline value is the member
		// MAX, never a sum (wall clock never sums across threads). ≤8 groups
		// never reach this branch — the field stays nil (anti-noise).
		chain.AggregatedImpactsFold = foldWakeupCausalAggregateOverflow(out[8:])
		out = out[:8]
	}
	return out
}

// applyAggregateGatedInversion (P0-E §20 E-Gap②, 2026-07-07) fills the R5d
// gated caliber on the aggregate face from its member occurrences, matching
// the per-occurrence lane's caliber (rootCauseItemFromCausalImpact ranks
// inversion candidates by PriorityInversionGatedMs — the aggregate lane used
// to compete with its RAW blocking magnitude, E-Gap②).
//
// Additivity argument (墙钟不可加和 red line respected): each member's gated
// components are wall-clock SUBSETS of that member's own occurrence window
// (runnable intervals in full + weak-core deficit shares of running
// intervals), all on ONE thread. When the gated members' windows are pairwise
// non-overlapping, the underlying intervals are disjoint, so summing them is
// per-thread wall-additive (same disjointness that legitimizes the
// cpu_occupancy per-thread cross-CPU merge and the N2 distinct-fact union
// ruling). Any overlap — branch windows can project the same physical segment
// twice (the PTV6 envelope-overlap veto shape) — makes the sum a potential
// double count, so the value honestly degrades to the strongest member (MAX,
// a lower bound) and the typed caliber field discloses the degradation. A
// gated member without a valid window cannot prove disjointness and also
// degrades to MAX.
func applyAggregateGatedInversion(chain *ChainResult, item *WakeupCausalAggregate, members []int) {
	type gatedMember struct {
		window     TimeWindow
		total      float64
		runnable   float64
		deficit    float64
		capability string
		topology   string
	}
	var gated []gatedMember
	for _, idx := range members {
		if idx < 0 || idx >= len(chain.CausalImpacts) {
			continue
		}
		impact := chain.CausalImpacts[idx]
		if !impact.PriorityInversionCandidate || impact.PriorityInversionGatedMs <= 0 {
			continue
		}
		gated = append(gated, gatedMember{
			window:     impact.Window,
			total:      impact.PriorityInversionGatedMs,
			runnable:   impact.GatedRunnableMs,
			deficit:    impact.GatedRunningDeficitMs,
			capability: impact.GatedCapabilitySource,
			topology:   impact.GatedClusterTopology,
		})
	}
	if len(gated) == 0 {
		return
	}
	disjoint := true
	for i := 0; i < len(gated) && disjoint; i++ {
		if gated[i].window.EndTs <= gated[i].window.StartTs {
			disjoint = false
			break
		}
		for j := i + 1; j < len(gated); j++ {
			if windowOverlapMs(gated[i].window.StartTs, gated[i].window.EndTs, gated[j].window.StartTs, gated[j].window.EndTs) > 0 {
				disjoint = false
				break
			}
		}
	}
	if disjoint {
		for _, member := range gated {
			item.GatedRunnableMs += member.runnable
			item.GatedRunningDeficitMs += member.deficit
			// CAP (§26 C3): members share one per-query capability judgment;
			// first non-empty wins. CAP-2: the topology token travels the
			// same way.
			if item.GatedCapabilitySource == "" {
				item.GatedCapabilitySource = member.capability
			}
			if item.GatedClusterTopology == "" {
				item.GatedClusterTopology = member.topology
			}
		}
		// F4 (§20.2 absorption, RCX² F2 same family): the total is derived
		// from the summed components — never a parallel Σ of member totals,
		// whose floating-point grouping could drift a last-ulp away from
		// components-sum and split the single-source identity
		// (total == runnable + deficit) every downstream face relies on.
		item.PriorityInversionGatedMs = item.GatedRunnableMs + item.GatedRunningDeficitMs
		item.GatedAggregationCaliber = GatedCaliberSumDisjointOccurrences
		return
	}
	strongest := gated[0]
	for _, member := range gated[1:] {
		if member.total > strongest.total {
			strongest = member
		}
	}
	item.PriorityInversionGatedMs = strongest.total
	item.GatedRunnableMs = strongest.runnable
	item.GatedRunningDeficitMs = strongest.deficit
	item.GatedCapabilitySource = strongest.capability
	item.GatedClusterTopology = strongest.topology
	item.GatedAggregationCaliber = GatedCaliberMaxOverlapFallback
}

// foldWakeupCausalAggregateOverflow synthesizes the PTS-2 bounded fold member
// from the trimmed aggregate overflow: one linear pass collecting group count,
// DominantImpactMs min–max, an up-to-8 subject-label roster (mirror of the
// PTV5 wire-cap fold roster bound) and the line/ts envelope. Returns nil on an
// empty overflow so callers can assign unconditionally.
//
// P2-1 (DIAG A1 第四取最大点, G12-ENG batch, ledger §29.6, 2026-07-09): the
// fold now ALSO retains the µs-tie member roster — members whose
// DominantImpactMs ties the published MAX inside the shared strict band
// (types.TraceCausalProjectionSameValueTieMS) are kept as (label, line-range)
// entries (cap 4, ≥2 labeled ties or none), so the record builder can emit
// the EXISTING same_value_members note from this take-MAX point too (zero new
// note keys; consumer chain built by DIAG A1). Disclosure only — every
// published fold figure above is final before the roster is computed.
func foldWakeupCausalAggregateOverflow(overflow []WakeupCausalAggregate) *WakeupCausalAggregateFold {
	if len(overflow) == 0 {
		return nil
	}
	fold := &WakeupCausalAggregateFold{Groups: len(overflow)}
	// PTS-2 F2 (复核 2026-07-06): roster dedupe mirrors the wire-cap fold —
	// aggregate groups are keyed (PID, state), so one thread overflowing with
	// TWO dominant states occupies ONE roster slot (Groups still counts both
	// groups).
	seen := map[string]bool{}
	for _, member := range overflow {
		v := member.DominantImpactMs
		if fold.MinImpactMs == 0 || (v > 0 && v < fold.MinImpactMs) {
			fold.MinImpactMs = v
		}
		if v > fold.MaxImpactMs {
			fold.MaxImpactMs = v
		}
		if len(fold.Subjects) < 8 {
			if label := strings.TrimSpace(threadLabel(member.Thread)); label != "" && !seen[label] {
				seen[label] = true
				fold.Subjects = append(fold.Subjects, label)
			}
		}
		if member.LineStart > 0 && (fold.LineStart <= 0 || member.LineStart < fold.LineStart) {
			fold.LineStart = member.LineStart
		}
		if member.LineEnd > fold.LineEnd {
			fold.LineEnd = member.LineEnd
		}
		if member.FirstTs > 0 && (fold.FirstTs == 0 || member.FirstTs < fold.FirstTs) {
			fold.FirstTs = member.FirstTs
		}
		if member.LastTs > fold.LastTs {
			fold.LastTs = member.LastTs
		}
	}
	fold.SameValueMembers = wakeupCausalAggregateFoldTies(overflow, fold.MaxImpactMs)
	return fold
}

// wakeupCausalAggregateFoldTies collects the P2-1 µs-tie roster over one
// trimmed overflow: members with a non-empty thread label whose
// DominantImpactMs ties maxMS inside the strict shared band. Cap 4; fewer
// than TWO ties return nil (one member at the max is just the max, not a
// suspected double). Pure read — callers pass the already-final maxMS.
func wakeupCausalAggregateFoldTies(overflow []WakeupCausalAggregate, maxMS float64) []WakeupCausalAggregateFoldTieMember {
	if maxMS <= 0 {
		return nil
	}
	var ties []WakeupCausalAggregateFoldTieMember
	for _, member := range overflow {
		label := strings.TrimSpace(threadLabel(member.Thread))
		if label == "" {
			continue
		}
		v := member.DominantImpactMs
		if v <= 0 || math.Abs(v-maxMS) >= types.TraceCausalProjectionSameValueTieMS {
			continue
		}
		if len(ties) < 4 {
			ties = append(ties, WakeupCausalAggregateFoldTieMember{
				Label:     label,
				LineStart: member.LineStart,
				LineEnd:   member.LineEnd,
			})
		}
	}
	if len(ties) < 2 {
		return nil
	}
	return ties
}

// detectPeriodicWakeupSource implements the VS-1 (§7.8) periodic-signal-source
// detection on one (waker→target) aggregate: with ≥wakeupPeriodicMinOccurrences
// sleep-dominant occurrences whose actual-window start intervals hold the
// robust cadence (see the wakeupPeriodicIntervalTolerance doc: gap carve +
// lower-median p, early fires veto, ≥2/3 in-band ratio), the pair is a
// periodic source — its in-period sleep is normal cadence. Lateness is the
// per-occurrence blocked caliber max(0, target_blocked − p), never an
// interval reading (F1: occurrence selection makes intervals a non-timeline).
// The aggregate AND its member impacts (stamped in place via the members
// indices) get PeriodicSource/DetectedPeriodMs/LatenessMs/
// EffectivePeriodicImpactMs; every raw field stays untouched. Deterministic
// interval arithmetic only — thread names never participate. Restricted to
// sleep-dominant aggregates: the customer ruling discounts in-period SLEEP;
// runnable/running/D/IO rows already count precisely and keep their bytes.
func detectPeriodicWakeupSource(chain *ChainResult, item *WakeupCausalAggregate, members []int) {
	if item.DominantState != string(StateSSleep) || len(members) < wakeupPeriodicMinOccurrences {
		return
	}
	ordered := append([]int(nil), members...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return chain.CausalImpacts[ordered[i]].ActualWindow.StartTs < chain.CausalImpacts[ordered[j]].ActualWindow.StartTs
	})
	starts := make([]float64, 0, len(ordered))
	for _, idx := range ordered {
		ts := chain.CausalImpacts[idx].ActualWindow.StartTs
		if ts <= 0 {
			return // an occurrence without an actual-window start: no cadence basis
		}
		starts = append(starts, ts)
	}
	intervals := make([]float64, 0, len(starts)-1)
	for i := 1; i < len(starts); i++ {
		interval := (starts[i] - starts[i-1]) * 1000
		if interval <= 0 {
			return // coincident/unordered starts: not a cadence
		}
		intervals = append(intervals, interval)
	}
	cadence, ok := wakeupPeriodicCadenceFromIntervals(intervals)
	if !ok || cadence.EarlyVeto {
		return // no cadence basis, or fired early beyond tolerance
	}
	// F4 in-band ratio gate (integer arithmetic, ≥2/3): after the gap carve
	// the cadence must be the MAJORITY reading of the observed intervals —
	// out-of-band late intervals never veto by themselves, they just must not
	// dominate the sample.
	if cadence.InBand*3 < cadence.Kept*2 {
		return
	}
	period := cadence.Period
	item.PeriodicSource = true
	item.DetectedPeriodMs = period
	totalLateness := 0.0
	for _, idx := range ordered {
		impact := &chain.CausalImpacts[idx]
		// F1(b) blocked-caliber lateness: the amount the target's wait for
		// THIS signal exceeded one period. Independent of whether the selected
		// occurrences are adjacent ticks; a missing target-blocked reading
		// accrues nothing (never fabricate).
		lateness := 0.0
		if impact.TargetBlockedMs > period {
			lateness = impact.TargetBlockedMs - period
		}
		impact.PeriodicSource = true
		impact.DetectedPeriodMs = period
		impact.LatenessMs = lateness
		// A DISCOUNT can never inflate: runnable + lateness is capped at the
		// raw blocking value the row published before VS-1 (a target wait far
		// beyond the waker's own sleep would otherwise push the "effective"
		// above the raw sleep).
		impact.EffectivePeriodicImpactMs = minPositiveCapFloat(impact.RunnableMs+lateness, causalImpactBlockingMs(*impact))
		impact.Summary = renderWakeupCausalImpactSummary(*impact)
		totalLateness += lateness
	}
	// F1(c) fabrication cap at the SUM site: the aggregate's published
	// lateness can never exceed raw blocking − runnable (occurrences sharing
	// one branch window would otherwise double-count the same target wait and
	// push an invented number into the Summary). Effective stays ≤ raw.
	rawBlocking := aggregateBlockingMs(*item)
	maxLateness := rawBlocking - item.RunnableMs
	if maxLateness < 0 {
		maxLateness = 0
	}
	if totalLateness > maxLateness {
		totalLateness = maxLateness
	}
	item.LatenessMs = totalLateness
	item.EffectivePeriodicImpactMs = minPositiveCapFloat(item.RunnableMs+totalLateness, rawBlocking)
}

// wakeupPeriodicCadence is the branch-selection-immune cadence reading of one
// aggregate's adjacent actual-window start intervals (F1/F3, adversarial
// review 2026-07-04). Gap marks observation gaps (≈k·p, k≥2): intervals
// between selected occurrences that skipped ticks — excluded from the period
// estimate, never lateness, never a veto.
type wakeupPeriodicCadence struct {
	Period    float64 // robust period p: lower median of the non-gap intervals
	Gap       []bool  // per input interval: observation gap (≈k·p, k≥2 integer)
	EarlyVeto bool    // some non-gap interval fired earlier than p×(1−tol)
	InBand    int     // non-gap intervals inside [p×(1−tol), p×(1+tol)]
	Kept      int     // non-gap interval count
}

// wakeupPeriodicCadenceFromIntervals estimates the robust period and
// classifies every interval. Two deterministic passes:
//   - pass 1 carves observation gaps against the SMALLEST interval (the
//     smallest observed interval can never itself be a k≥2 multiple of the
//     period, so it anchors the carve before any median is taken) and takes
//     the lower median of what remains as p;
//   - pass 2 re-classifies every interval against that final p: gap / early
//     (veto) / in-band / late.
//
// ok is false when there is no positive interval to read a cadence from.
func wakeupPeriodicCadenceFromIntervals(intervals []float64) (wakeupPeriodicCadence, bool) {
	if len(intervals) == 0 {
		return wakeupPeriodicCadence{}, false
	}
	anchor := intervals[0]
	for _, interval := range intervals {
		if interval <= 0 {
			return wakeupPeriodicCadence{}, false
		}
		if interval < anchor {
			anchor = interval
		}
	}
	kept := make([]float64, 0, len(intervals))
	for _, interval := range intervals {
		if !wakeupPeriodicIsObservationGap(interval, anchor) {
			kept = append(kept, interval)
		}
	}
	period := lowerMedianOfFloats(kept)
	if period <= 0 {
		return wakeupPeriodicCadence{}, false
	}
	out := wakeupPeriodicCadence{Period: period, Gap: make([]bool, len(intervals))}
	for i, interval := range intervals {
		if wakeupPeriodicIsObservationGap(interval, period) {
			out.Gap[i] = true
			continue
		}
		out.Kept++
		if interval < period*(1-wakeupPeriodicIntervalTolerance) {
			out.EarlyVeto = true
			continue
		}
		if interval <= period*(1+wakeupPeriodicIntervalTolerance) {
			out.InBand++
		}
	}
	return out, true
}

// wakeupPeriodicIsObservationGap reports whether the interval sits within
// ±tol·reference of an integer multiple k·reference (k≥2) — an observation
// gap between non-adjacent selected occurrences. The tolerance is relative to
// ONE period, not to k·reference: a relative-to-multiple band widens with k
// and starts absorbing chaotic interval sets whose smallest member is noise
// (everything becomes "some multiple"), while accumulated in-band jitter over
// the realistic k=2..4 gaps stays well inside ±tol of a single period.
func wakeupPeriodicIsObservationGap(interval, reference float64) bool {
	if reference <= 0 {
		return false
	}
	k := math.Round(interval / reference)
	if k < 2 {
		return false
	}
	return math.Abs(interval-k*reference) <= wakeupPeriodicIntervalTolerance*reference
}

// minPositiveCapFloat caps value at cap when cap is positive; a non-positive
// cap leaves the value untouched.
func minPositiveCapFloat(value, cap float64) float64 {
	if cap > 0 && value > cap {
		return cap
	}
	return value
}

// lowerMedianOfFloats returns the lower median — sorted[(n−1)/2], one UNIFORM
// rule for odd and even counts (F3: averaging the two middle values of an
// even-count sample gets pulled between cadence bands by a single extreme and
// returns a period no real interval has). The input slice is not mutated.
func lowerMedianOfFloats(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	return sorted[(len(sorted)-1)/2]
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
	// Shared 5-lane pick (thread_state_universe.go) — same priority order the
	// inline candidates array always used (fourth copy, truth-table compared
	// against the shared pick before the swap, TSH review F1).
	return dominantStateFromLanes(item.RunningMs, item.RunnableMs, item.SleepMs, item.DStateMs, item.IOWaitMs)
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
	if item.AggregationCaliber != "" {
		summary += " aggregation_caliber=" + item.AggregationCaliber
	}
	if item.PriorityRelation != "" {
		summary += " priority_relation=" + item.PriorityRelation
	}
	if item.PriorityInversion {
		summary += " priority_inversion_candidate=true"
	}
	if item.PeriodicSource {
		// VS-1 (§7.8): a periodic signal source's in-period sleep is normal
		// cadence; only runnable time and signal lateness count as attribution.
		summary += fmt.Sprintf(" periodic_source=true detected_period=%.3fms lateness=%.3fms effective_impact=%.3fms", item.DetectedPeriodMs, item.LatenessMs, item.EffectivePeriodicImpactMs)
	}
	if item.SupplyFoldBasis != nil {
		// VS-2 (§7.10): folded-member sum, zeros load-bearing (see the
		// per-occurrence renderer).
		summary += fmt.Sprintf(" supply_fold_deficit=%.3fms supply_fold_ideal=%.3fms fold_basis_known=%.3fms fold_basis_unknown=%.3fms",
			item.SupplyFoldDeficitMs, item.SupplyFoldIdealMs, item.SupplyFoldBasis.KnownMs, item.SupplyFoldBasis.UnknownMs)
	}
	return summary
}

func attachIPCGraphToChain(idx *Index, q Query, res *ChainResult) {
	if res == nil {
		return
	}
	ipc := BuildIPCGraph(idx, q)
	res.IPCEdges = ipc.Edges
	waits, pacingIdles, writeOffCaveat := findBinderWaitsForChain(idx, *res, ipc.Edges, ipc.BinderEvents)
	res.BinderWaits, res.PacingIdles = waits, pacingIdles
	if writeOffCaveat != "" {
		res.Caveats = append(res.Caveats, writeOffCaveat)
	}
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
	// P9 arm c (§29.42 案1): idle-cadence segments publish on their own
	// semantic lane — a rank row that never competes (RootCauseTierContextOnly
	// via the dedicated type arm in assignRootCauseRanksAndTiers). The Type
	// token is the typed Kind fork (pacing_idle / periodic_idle, 复核 P2-1).
	for _, p := range res.PacingIdles {
		res.RootEvidence = append(res.RootEvidence, RootEvidence{
			Type:       firstNonEmpty(p.Kind, "pacing_idle"),
			Thread:     p.Thread,
			DurationMs: p.DurationMs,
			// ENG-2 追修 (2026-07-12): the published span aligns to the
			// segment's causal impact record (same-fact fold by
			// construction); the raw sleep/wakeup lines stay on the summary.
			LineStart:  firstPositive(p.EvidenceLineStart, p.SleepLine),
			LineEnd:    firstPositive(p.EvidenceLineEnd, p.WakeupLine, p.SleepLine),
			Summary:    p.Summary,
			Confidence: 0.85,
		})
	}
	res.Caveats = append(res.Caveats, ipc.Caveats...)
}

func findBinderWaitsForChain(idx *Index, chain ChainResult, edges []IPCEdge, aux []BinderEventSummary) ([]BinderWaitSummary, []PacingIdleSummary, string) {
	if len(chain.Nodes) == 0 || len(edges) == 0 {
		return nil, nil, ""
	}
	var out []BinderWaitSummary
	var pacing []PacingIdleSummary
	seen := map[string]bool{}
	// P9 (§29.42 案1 BINDER-MISATTR): the reply-completion / waker-identity /
	// pacing indexes the three write-off arms read (binder_attribution.go).
	audit := buildBinderAttributionAudit(idx, chain)
	writtenOffReply := 0
	writtenOffWaker := 0
	for nodeIdx, node := range chain.Nodes {
		if node.Thread.PID == 0 {
			continue
		}
		if node.Dominant != StateSSleep && node.Dominant != StateDSleep && node.Dominant != StateIOWait {
			continue
		}
		// EVOLUTION RECORD (P9, 2026-07-12): the segment's wakeup edge used to
		// be resolved per accepted candidate as the FIRST in-window edge; it is
		// now resolved once per node as the segment-ENDING edge (latest
		// in-window wakeup) because arm b compares the terminating waker's
		// process against the attributed peer. Single-wakeup segments are
		// byte-identical.
		wakeEdge, hasWake := audit.wakeEdgeByNode[nodeIdx]
		minted := false
		var rejectedTxns []int
		for _, edge := range edges {
			if edge.Oneway {
				// Oneway/async transactions are never "the waited-on
				// transaction" (P9 note ②; flags bit 0x1 covers 0x01/0x11).
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
			// P9 arm a — reply write-off: a transaction whose reply completed
			// strictly BEFORE the segment start is already finished and cannot
			// explain this sleep (donghu witness: txn 12145963, reply ~97ms
			// before the blamed 15.758ms frame-pacing segment).
			if audit.replyCompletedBeforeSegment(node.Thread.PID, edge.SendTs, node.Window.StartTs) {
				writtenOffReply++
				rejectedTxns = append(rejectedTxns, edge.TransactionID)
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
			if hasWake {
				wait.WakeupLine = wakeEdge.WakeupLine
				wait.WakeupTs = wakeEdge.WakeupTs
			}
			// §11 N8: Android BC_TRANSACTION to a process pool leaves
			// dest_thread=0, so ipc.go could not name a receiver — recover the
			// server-side counterpart from the waiter's direct wakeup edge (the
			// thread that eventually replied woke it). A dest_thread HINT that
			// names no thread this trace scheduled (cross-ns phantom) is also
			// treated as unresolved via threadRefPresent, symmetric to the lock
			// lane — only a genuinely-present receiver keeps the payload source.
			if idx.threadRefPresent(wait.Peer) {
				wait.PeerSource = CounterpartSourceContentionPayload
			} else if fb := resolveCounterpartViaWakeupEdge(idx, node.Thread, node.Window.StartTs, node.Window.EndTs); fb.OK {
				if wait.Peer.PID > 0 {
					// Preserve the phantom dest tid for audit before swapping.
					wait.Caveats = append(wait.Caveats, fmt.Sprintf("binder dest_thread %d is not present in this trace (endpoint-only send)", wait.Peer.PID))
				}
				wait.Peer = fb.Waker
				wait.PeerSource = CounterpartSourceWakeupEdge
				// Visible confidence downgrade for the inferred counterpart.
				wait.Confidence = counterpartDemotedConfidence(wait.Confidence)
				wait.Caveats = append(wait.Caveats, "binder receiver inferred from the waiter's wakeup edge (dest_thread=0/absent endpoint); counterpart is the thread that woke the waiter, not a confirmed binder receive row")
			} else if wait.Peer.PID > 0 {
				// A phantom dest_thread with no usable wakeup edge: drop the ghost
				// PID so drill/peer-state read "unresolved" instead of pointing at
				// an id this trace never scheduled (symmetric with the lock lane).
				wait.Caveats = append(wait.Caveats, fmt.Sprintf("binder dest_thread %d is not present in this trace and no wakeup edge resolved the counterpart", wait.Peer.PID))
				wait.Peer = ThreadRef{}
			}
			// P9 arm b — peer-process consistency: the segment-ending waker
			// must belong to the attributed peer's process. A mismatch is a
			// write-off UNLESS the reply verifiably arrived inside the segment
			// (genuine wait woken through another thread — attribution stands
			// with a typed disclosure caveat). Unknown tgids skip the
			// comparison (precise signals only).
			if hasWake && wakeEdge.Waker.TGID > 0 && wait.Peer.TGID > 0 && wakeEdge.Waker.TGID != wait.Peer.TGID {
				if audit.replyInsideSegment(node.Thread.PID, edge.SendTs, node.Window.StartTs, node.Window.EndTs) {
					wait.Caveats = append(wait.Caveats, fmt.Sprintf("segment-ending waker %s belongs to a different process than binder peer %s; the reply arrived inside the segment, so the binder wait stands with this disclosure", threadLabel(wakeEdge.Waker), threadLabel(wait.Peer)))
				} else {
					writtenOffWaker++
					rejectedTxns = append(rejectedTxns, edge.TransactionID)
					delete(seen, key)
					continue
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
			minted = true
			break
		}
		// P9 arm c — idle-cadence rerouting: every binder candidate of this
		// segment was written off; when the segment reads as idle cadence
		// (length ≈ one plausible period, typed period evidence), re-mint it
		// on the pacing_idle / periodic_idle semantic lane (复核 P2-1 fork:
		// the frame words render only for frame-chain wakers).
		if !minted && len(rejectedTxns) > 0 && hasWake {
			if periodMs, source, kind, ok := audit.pacingVerdict(chain, node, wakeEdge); ok {
				p := PacingIdleSummary{
					Thread:                 node.Thread,
					Waker:                  wakeEdge.Waker,
					WindowStartTs:          node.Window.StartTs,
					WindowEndTs:            node.Window.EndTs,
					DurationMs:             node.DurationMs,
					FramePeriodMs:          periodMs,
					PeriodSource:           source,
					Kind:                   kind,
					SleepLine:              node.EvidenceLine,
					WakeupLine:             wakeEdge.WakeupLine,
					WakeupTs:               wakeEdge.WakeupTs,
					RejectedTransactionIDs: rejectedTxns,
				}
				// ENG-2 追修 (2026-07-12): publish under the segment's causal
				// impact evidence span so the display same-fact fold engages
				// by construction (see chainCausalImpactLinesForNode).
				if ls, le, ok := chainCausalImpactLinesForNode(chain, node); ok {
					p.EvidenceLineStart, p.EvidenceLineEnd = ls, le
				}
				p.Summary = renderPacingIdleSummary(p)
				pacing = append(pacing, p)
			}
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
	sort.SliceStable(pacing, func(i, j int) bool {
		if pacing[i].DurationMs != pacing[j].DurationMs {
			return pacing[i].DurationMs > pacing[j].DurationMs
		}
		return pacing[i].SleepLine < pacing[j].SleepLine
	})
	if len(pacing) > 8 {
		pacing = pacing[:8]
	}
	return out, pacing, binderWriteOffCaveat(writtenOffReply, writtenOffWaker)
}

// binderWriteOffCaveat renders the bounded P9 write-off disclosure appended
// once per chain (honest accounting, never per-segment noise).
func binderWriteOffCaveat(writtenOffReply, writtenOffWaker int) string {
	if writtenOffReply == 0 && writtenOffWaker == 0 {
		return ""
	}
	var parts []string
	if writtenOffReply > 0 {
		parts = append(parts, fmt.Sprintf("%d candidate(s) whose reply had already completed before the sleep segment started", writtenOffReply))
	}
	if writtenOffWaker > 0 {
		parts = append(parts, fmt.Sprintf("%d candidate(s) whose segment-ending waker belongs to a different process than the binder peer", writtenOffWaker))
	}
	return "binder wait attribution wrote off " + strings.Join(parts, " and ") + "; those sleep segments stay on their scheduler-state lanes (or the frame-pacing idle lane) instead"
}

func binderAuxCaveatsForWait(wait BinderWaitSummary, aux []BinderEventSummary) []string {
	if wait.ReceiveLine <= 0 || wait.SendLine <= 0 {
		return nil
	}
	var out []string
	for _, item := range aux {
		if item.Line < wait.SendLine || item.Line > wait.ReceiveLine {
			continue
		}
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
	rank := buildRootCauseRankFrom(idx, q, chain, stats)
	latency := buildSchedulerLatencyStatsFromStats(idx, q, stats)
	rank = enrichRootCauseRankWithScheduler(q, rank, latency, stats, chain)
	return attachPerfContextToRootCauseRank(idx, q, rank, stats)
}

// stampRootCauseProcessIdentity (CR-3 件③ P11, 2026-07-12; 冷读案8 关键角色
// 裸线程名无 tgid witness): every rank row gains its process attribution —
// the TGID the trace's second column published for the thread (catalog
// backfill when the row's ThreadRef missed it) and the owning process comm
// (the catalog's tgid==tid main-thread entry). Identity enrichment only:
// values are verbatim catalog facts, rank/score/order untouched. Called
// from attachPerfContextToRootCauseRank — the ONE shared finalize tail of
// every rank build lane (Run getRootCause / BuildRootCauseRank /
// BuildFrameRootCauseBundle); a lane-local stamp would drift (首放实证:
// the frame-bundle lane shipped bare tgid-less rows while the direct lane
// was stamped).
func stampRootCauseProcessIdentity(idx *Index, q Query, rank *RootCauseRankResult) {
	if idx == nil || rank == nil || (len(rank.Items) == 0 && len(rank.AbsorbedItems) == 0) {
		return
	}
	catalog := buildThreadCatalog(idx, q)
	stamp := func(items []RootCauseRankItem) {
		for i := range items {
			thread := &items[i].Thread
			if thread.PID <= 0 {
				continue
			}
			if thread.TGID == 0 {
				if cat := catalog[thread.PID]; cat.TGID > 0 {
					thread.TGID = cat.TGID
				}
			}
			if thread.TGID > 0 {
				if proc := catalog[thread.TGID]; strings.TrimSpace(proc.Comm) != "" {
					items[i].ProcessComm = strings.TrimSpace(proc.Comm)
				}
			}
		}
	}
	stamp(rank.Items)
	stamp(rank.AbsorbedItems)
}

func buildRootCauseRankFrom(idx *Index, q Query, chain ChainResult, stats WindowStats) RootCauseRankResult {
	res := RootCauseRankResult{
		Target: chain.Target,
		Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
	}
	var items []RootCauseRankItem
	chainThreads := wakeupChainThreadSet(chain)
	hasCausalChain := len(chainThreads) > 1
	// ORD (ledger §29.8 P2②/P2③ + §29.11 补充 观察②, 2026-07-10): one
	// occurrence set = ONE seat. The seated aggregates are resolved first
	// (full census, periodic bypass — see rankSeatAggregates) and their
	// member occurrences are suppressed from the per-occurrence rank mint
	// below: pre-ORD the aggregate AND its members all seated (cap2 #4/#5/#6
	// triple seat), and the display ×N merge then summed the aggregate row
	// (=Σ member gateds) with its own members — the cmp_792 E7 "窗口投影恰=
	// 2×有效归因 / occurrence 集不可调和" double-count. The VIEW lane
	// (chain.CausalImpacts wire records) stays lossless — only rank seats
	// deduplicate.
	seatedAggregates := rankSeatAggregates(chain)
	seatedAggregateGroups := map[string]bool{}
	for _, aggregate := range seatedAggregates {
		seatedAggregateGroups[wakeupCausalAggregateGroupKey(aggregate.Thread.PID, aggregate.DominantState)] = true
	}
	// One physical causal occurrence may be published on both lossless
	// wakeup-chain faces: CausalImpacts and RootEvidence.  Only the former owns
	// the active rank seat when it (or its aggregate) was admitted.  Record the
	// exact derived RootEvidence identity for each admitted occurrence so the
	// later RootEvidence loop can preserve every non-twin lane (missing wakeup,
	// trace gap, depth-0 fallback, binder evidence) without double minting the
	// same interval.
	seatedCausalRootEvidence := map[string]bool{}
	// ENG audit #65 (§29.25 处置委托 2026-07-10): seed BOTH the exact seat key
	// (unmutated constructor output) and the mutation-invariant d_state/io_wait
	// family key — expandChain mutates the D-state twin in place when a
	// sched_blocked_reason resolves (Type/LineEnd), so the exact key alone let
	// that twin escape the single-seat gate and mint a second rank row for the
	// same physical occurrence.
	seatRootEvidenceTwin := func(impact WakeupCausalImpact) {
		seed := rootEvidenceFromCausalImpact(impact, "", 0)
		seatedCausalRootEvidence[rootEvidenceRankSeatKey(seed)] = true
		if key, ok := rootEvidenceDStateTwinFamilyKey(seed); ok {
			seatedCausalRootEvidence[key] = true
		}
	}
	for _, impact := range chain.CausalImpacts {
		if impact.TotalMs <= 0 {
			continue
		}
		if impact.ChainDepth <= 0 && impact.DominantState != string(StateRunning) {
			// Depth-0 (target-own) impacts stay out of the rank pool — their
			// wait states are the SYMPTOM being explained, and their
			// RootEvidence lanes below keep publishing as before. EXCEPT the
			// target's own RUNNING work (§20 A-fix(3), 2026-07-07): that row
			// used to be rank-carried ONLY by the impoverished RootEvidence
			// running twin (no fold/window/state typed fields); the twin no
			// longer mints rank rows (§20.1 ruling ①), so the CausalImpacts
			// lane now mints the depth-0 running row with the full typed set.
			continue
		}
		if isIntermediateSleepImpact(chain, impact) {
			continue
		}
		// ORD-A member suppression: EXACTLY the aggregation admission
		// predicate (aggregateWakeupCausalImpacts: PID>0 ∧ ChainDepth>0 ∧
		// TotalMs>0 ∧ DominantState nonempty) + the shared group key — a
		// depth-0 running row is NOT a member (the aggregate never grouped
		// it) and keeps minting.
		if impact.Thread.PID > 0 && impact.ChainDepth > 0 && strings.TrimSpace(impact.DominantState) != "" &&
			seatedAggregateGroups[wakeupCausalAggregateGroupKey(impact.Thread.PID, impact.DominantState)] {
			seatRootEvidenceTwin(impact)
			continue
		}
		items = append(items, rootCauseItemFromCausalImpact(impact))
		seatRootEvidenceTwin(impact)
	}
	for _, aggregate := range seatedAggregates {
		items = append(items, rootCauseItemFromCausalAggregate(aggregate))
	}
	for _, root := range chain.RootEvidence {
		// §20 headline + §20.1 ruling ① (2026-07-07, RKC): a RootEvidence twin
		// of an ADMITTED CausalImpact no longer mints a rank row — the SAME
		// WakeupCausalImpact already publishes its rank row via the
		// CausalImpacts lane above, and the running twin's raw
		// DominantImpactMs used to outrank the segment's own gated inversion
		// row in the same pool (q6: raw 58.919 over gated 37.410, a
		// co-primary double mint that bypassed all three fold generations).
		// One segment, one rank row; the CausalImpacts row is the single rank
		// carrier. RootEvidence itself is untouched — it stays a wakeup_chain
		// view evidence row.
		// EVOLUTION RECORD (5d91b433 + ENG audit #65, §29.25 处置委托
		// 2026-07-10): the original §20.1 gate was the precise typed token
		// `root.Type == "running"`. 5d91b433 widened it to the exact
		// occurrence identity key (Type|thread|LineStart|LineEnd|DurationMs),
		// so EVERY exact twin of an admitted impact — running, runnable,
		// D-state — is suppressed, while non-twin lanes (missing_wakeup /
		// binder_wait / trace_gap / depth-0 fallback / unknown) keep minting
		// because their identities are never seeded. This batch closed the
		// remaining escape: the blocked-reason-mutated D-state twin no longer
		// matches the exact key and is folded by the mutation-invariant
		// family key below (rootEvidenceDStateTwinFamilyKey).
		// §15.B revocation note (§20.1): the formerly-planned separate
		// "runnable independent effective-attribution row" is WITHDRAWN — the
		// merged row's gated composition already counts the runnable share in
		// full; re-publishing it would recreate the double count.
		if seatedCausalRootEvidence[rootEvidenceRankSeatKey(root)] {
			continue
		}
		if key, ok := rootEvidenceDStateTwinFamilyKey(root); ok && seatedCausalRootEvidence[key] {
			continue
		}
		if sameThreadRef(root.Thread, res.Target) && rootEvidenceStateOwnedByWindowStats(root, stats) {
			// The formal WindowStats state account is richer (typed interval,
			// CPU/window identity and priority-inversion refinement) and owns the
			// single self-cause seat. RootEvidence remains lossless on the
			// wakeup_chain view but must not mint a second rank vote.
			continue
		}
		item := rootCauseItem(root.Type, root.Thread, root.DurationMs, root.Confidence, root.LineStart, root.LineEnd, "wakeup_chain", root.Summary)
		stampRootEvidenceRankCaliber(root, &item)
		if sameThreadRef(root.Thread, res.Target) &&
			(root.Type == "runnable_wait" || root.Type == "io_wait" || root.Type == "d_state_or_io_wait") {
			item.Causality = "on_wakeup_chain"
			item.ChainRelevance = "on_chain"
		}
		if root.Type == "trace_gap" {
			// G2 判据 typed 化 (§27.2/§28.1, 2026-07-09): the precise blind-spot
			// criterion travels typed on the rank row (trace_gap_kind wire note);
			// the row itself demotes to RootCauseTierDataGap in
			// assignRootCauseRanksAndTiers — 数据盲区非成因.
			item.TraceGapKind = root.GapKind
		}
		items = append(items, item)
	}
	// PRESSURE-ONE-SEAT: when SupplyPressureSummary is the exact sum of this
	// WindowStats cohort's per-CPU runnable-wait components, the aggregate owns
	// the single rank seat. Per-CPU components remain fully typed in
	// WindowStats.CPUPressure (banner/JSON/perf drilldown); seating them beside
	// their total would split one demand account across N+1 ranks. Any absent or
	// mismatched aggregate fails open to the per-CPU seats.
	aggregateOwnsCPUPressure := rootCauseSupplyPressureOwnsCPURankSeat(stats)
	for _, pressure := range stats.CPUPressure {
		if pressure.RunnableWaitMs <= 0 {
			continue
		}
		if aggregateOwnsCPUPressure {
			continue
		}
		// R5g (§7.30.2): confidence and the competition wording key off the
		// overlap-evidenced share, not the window-total high-priority running.
		conf := 0.72
		if pressure.HighPriorityRunningOverlapMs > 0 {
			conf = 0.80
		}
		summary := fmt.Sprintf("cpu=%d had %.3fms runnable wait and %.3fms running time in the selected window", pressure.CPU, pressure.RunnableWaitMs, pressure.RunningMs)
		if pressure.HighPriorityRunningOverlapMs > 0 {
			summary = fmt.Sprintf("%s; high-priority running overlapping other threads' runnable waits %.3fms (window total %.3fms)", summary, pressure.HighPriorityRunningOverlapMs, pressure.HighPriorityRunningMs)
		} else if pressure.HighPriorityRunningMs > 0 {
			summary = fmt.Sprintf("%s; high-priority running time %.3fms in window without runnable-wait overlap (background, not displacement)", summary, pressure.HighPriorityRunningMs)
		}
		if pressure.SystemOrKernelRunningOverlapMs > 0 {
			summary = fmt.Sprintf("%s; raw system/kernel competition overlap %.3fms across %d competitor(s) (window total %.3fms; not high-priority evidence)",
				summary, pressure.SystemOrKernelRunningOverlapMs, pressure.SystemOrKernelCompetitorCount, pressure.SystemOrKernelRunningMs)
		} else if pressure.SystemOrKernelRunningMs > 0 {
			summary = fmt.Sprintf("%s; raw system/kernel running time %.3fms in window (not high-priority evidence)", summary, pressure.SystemOrKernelRunningMs)
		}
		item := rootCauseItem("cpu_pressure", ThreadRef{}, backgroundImpactMs(q, pressure.RunnableWaitMs, hasCausalChain, false), conf, firstThreadLine(pressure.TopRunnable), lastThreadLine(pressure.TopRunning), "window_stats", summary)
		item.CumulativeImpactMs = pressure.RunnableWaitMs
		if hasCausalChain {
			item.Causality = "background"
		}
		items = append(items, item)
	}
	for _, io := range stats.IOLatencies {
		projectedStart, projectedEnd := io.IssueTs, io.CompleteTs
		projectedMs := io.DurationMs
		if q.TimeEnd > q.TimeStart && io.CompleteTs > io.IssueTs {
			if start, end, ok := overlapTimeWindow(io.IssueTs, io.CompleteTs, q.TimeStart, q.TimeEnd); ok {
				projectedStart, projectedEnd = start, end
				projectedMs = (end - start) * 1000
			} else {
				continue
			}
		}
		if projectedMs <= 0 {
			continue
		}
		onChain := threadInSet(chainThreads, io.IssueThread) || threadInSet(chainThreads, io.CompleteThread)
		item := rootCauseItem("io_latency", io.IssueThread, backgroundImpactMs(q, projectedMs, hasCausalChain, onChain), 0.86, io.IssueLine, io.CompleteLine, "window_stats", fmt.Sprintf("block IO %s %s sector=%d len=%d projected %.3fms inside the selected window (physical %.3fms)", io.Dev, io.Op, io.Sector, io.Len, projectedMs, io.DurationMs))
		item.PhysicalSourcePath = io.SourcePath
		item.ProjectedImpactMs = projectedMs
		item.CumulativeImpactMs = projectedMs
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.StartTs = projectedStart
		item.EndTs = projectedEnd
		if io.CompleteTs > io.IssueTs && (projectedStart != io.IssueTs || projectedEnd != io.CompleteTs) {
			item.ActualStartTs = io.IssueTs
			item.ActualEndTs = io.CompleteTs
			item.ActualImpactMs = io.DurationMs
			item.ActualTotalMs = io.DurationMs
		}
		// RCM §24.7.1 typed member identity (never a Summary re-parse).
		item.Dev = io.Dev
		item.MemberKey = fmt.Sprintf("dev=%s op=%s sector=%d", io.Dev, io.Op, io.Sector)
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
		// RCM §24.7.1/§24.9-B F3: the real distinguishing keys ride typed
		// fields (the Summary prose was their ONLY carrier before).
		item.Inode = file.Inode
		item.Dev = file.Dev
		item.MemberKey = fmt.Sprintf("inode=%s dev=%s op=%s", file.Inode, file.Dev, file.Operation)
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
		// RCM §24.7.1/§24.9-B F3 typed distinguishing keys.
		item.Inode = cache.Inode
		item.Dev = cache.Dev
		item.MemberKey = fmt.Sprintf("inode=%s dev=%s", cache.Inode, cache.Dev)
		items = append(items, item)
	}
	if stats.IOPressureSummary != nil && stats.IOPressureSummary.Score > 0 {
		// Aggregate pressure has no single causal thread. Borrowing the top
		// inode thread promoted a composite (whose constituents already have
		// their own seats) onto the wakeup chain. Keep it as rank-0 context.
		item := rootCauseItem("io_pressure", ThreadRef{}, backgroundImpactMs(q, stats.IOPressureSummary.Score, hasCausalChain, false), 0.70, stats.IOPressureSummary.LineStart, stats.IOPressureSummary.LineEnd, "window_stats.io_pressure_summary", stats.IOPressureSummary.Summary)
		item.CumulativeImpactMs = stats.IOPressureSummary.Score
		if hasCausalChain {
			item.Causality = "background"
			item.ChainRelevance = "background"
		}
		items = append(items, item)
	}
	for _, episode := range stats.IOBurstEpisodes {
		if episode.DominantSignal == "d_state_or_io_wait" || episode.DominantSignal == "scheduler_iowait" {
			// Scheduler-derived episodes are a diagnostic projection of the
			// formal mutually-exclusive D/IO state account above. They remain in
			// WindowStats/typed observations and never mint a second rank seat.
			continue
		}
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
		// RCM §24.7.1/§24.9-B F3 (opendir_78 gap③): inode/dev were carried by
		// the free-text Summary ONLY and every display face dropped them —
		// typed fields now, and the family fold folds two different-inode rows
		// of one thread into ONE contender with both keys in the roster.
		item.Inode = inode.Inode
		item.Dev = inode.Dev
		item.MemberKey = fmt.Sprintf("inode=%s dev=%s", inode.Inode, inode.Dev)
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
	// Q4-A 修1 (ledger §12.1/§12.3-5) + P2-3: structured lock-contention spans
	// get their own typed rank lane (type=blocking_span, registry RowToken
	// already true — zero new registration) instead of degrading into a
	// generic trace_span row that can never leave the adjacent tier. The lane
	// is fed from the SAME folded carve as critical_blocking, so dual print
	// forms of one lock publish exactly one rank row.
	for _, row := range collectBlockingSpanRows(idx, stats) {
		if row.cand.BlockingKind == "" {
			continue
		}
		items = append(items, rootCauseItemFromLockContentionCandidate(q, chainThreads, hasCausalChain, row))
	}
	var semanticNearMisses []string
	// RCM §24.10 (user ruling 2026-07-08): semantic spans participate as
	// (thread, semantic class, chain lane) FAMILIES — one contender per family
	// carrying the window-projection total (合并量参赛). The fold is the ONE
	// shared function the typed observation channel also consumes (§24.12: 两
	// 消费方同一函数); stats.TraceSpans itself stays untouched (span_window /
	// detail roster source). A family of one routes through the single-span
	// constructor verbatim — degenerate families stay byte-identical to the
	// pre-RCM rows (退化不变体).
	semanticConsumed := map[int]bool{}
	for _, fam := range FoldSemanticSpanFamilies(&chain, stats.TraceSpans) {
		if len(fam.Members) == 1 {
			if item, ok := rootCauseItemFromSemanticTraceSpan(q, chain, fam.Members[0], hasCausalChain); ok {
				items = append(items, item)
				markSemanticSpanConsumed(semanticConsumed, stats.TraceSpans, fam.Members[0])
			}
			continue
		}
		if item, ok := rootCauseItemFromSemanticSpanFamily(q, fam, hasCausalChain); ok {
			items = append(items, item)
			for _, member := range fam.Members {
				markSemanticSpanConsumed(semanticConsumed, stats.TraceSpans, member)
			}
		}
	}
	for i, span := range stats.TraceSpans {
		if semanticConsumed[i] {
			continue
		}
		if _, isContention := parseLockContentionPayload(span.Name); isContention {
			// Carved into the folded typed lock lane above — never a generic
			// trace_span row.
			continue
		}
		if span.SemanticClass == "" && traceSpanNearMissesSemanticWorkClassification(span.Name) && len(semanticNearMisses) < 3 {
			semanticNearMisses = append(semanticNearMisses, span.Name)
		}
		item := rootCauseItem("trace_span", span.Thread, span.DurationMs, 0.74, span.StartLine, span.EndLine, "window_stats", fmt.Sprintf("trace span %q lasted %.3fms", span.Name, span.DurationMs))
		item.PhysicalSourcePath = span.SourcePath
		item.StartTs = span.StartTs
		item.EndTs = span.EndTs
		if span.ActualDurationMs > 0 {
			// DCS E4: window-clipped boundary span — the published duration is
			// the in-window projection; the physical extent rides actual_*.
			item.ActualStartTs = span.ActualStartTs
			item.ActualEndTs = span.ActualEndTs
			item.ActualImpactMs = span.ActualDurationMs
			item.ActualTotalMs = span.ActualDurationMs
		}
		item.SpanName = span.Name
		item.SpanKind = span.Kind
		item.SpanCategory = span.Category
		item.SpanSubcategory = span.Subcategory
		item.SemanticClass = span.SemanticClass
		items = append(items, item)
	}
	if len(semanticNearMisses) > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("span name(s) mention compile/verify/shader/texture-upload/GC-like vocabulary but did not match a known jit_compile/class_verification/shader_compile/runtime_compile/texture_upload/gc_pause pattern (e.g. %s); the app/ArkCompiler/ROM naming convention may have changed — treat as generic trace_span context, not a confirmed semantic root cause", strings.Join(semanticNearMisses, ", ")))
	}
	// ORD 复核 P3-1 (2026-07-10): the producer-disjointness proof PREMISE is a
	// globally time-ordered event stream — the single open-segment state
	// machine in computeOffCPUStats can only partition a thread's timeline
	// when timestamps never move backwards. The engine itself admits the
	// out-of-order shape via the ClockRegressions counter (parse.go; the Q1
	// degrade criterion lane), so the proof mints ONLY on regression-free
	// indexes; a regressed trace conservatively degrades the family fold to
	// the member MAX (honest lower bound, never an over-count Σ).
	offCPUProducerDisjoint := idx != nil && idx.TimestampOrder == TraceTimestampOrderMonotonic
	for _, td := range stats.RunnableTop {
		onChain := threadInSet(chainThreads, td.Thread)
		item := rootCauseItem("runnable_wait", td.Thread, backgroundImpactMs(q, td.DurationMs, hasCausalChain, onChain), 0.76, td.LineStart, td.LineEnd, "window_stats", fmt.Sprintf("%s was runnable for %.3fms%s", threadLabel(td.Thread), td.DurationMs, durationCPUDetail(td)))
		item.CumulativeImpactMs = td.DurationMs
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.StartTs = td.StartTs
		item.EndTs = td.EndTs
		item.DominantState = string(StateRunnable)
		item.RunnableMs = td.DurationMs
		// ORD (§29.11 补充 观察①/§24.7.1, 2026-07-10): the per-CPU bucket key
		// is the row's 区分键 (roster face), and the single open-segment
		// state machine of computeOffCPUStats proves same-thread member
		// segments pairwise disjoint (Σ caliber; envelopes interleave).
		item.MemberKey = fmt.Sprintf("cpu=%d", td.CPU)
		item.memberSegmentsProducerDisjoint = offCPUProducerDisjoint
		applyRunnableTopPriorityInversion(idx, q, stats, td, &item)
		items = append(items, item)
	}
	fragmentedSleep := fragmentedSleepChurnByThread(stats.StateChurn)
	for _, td := range stats.SleepTop {
		if _, ok := fragmentedSleep[stateDrilldownThreadKey(td.Thread)]; ok {
			continue
		}
		onChain := threadInSet(chainThreads, td.Thread)
		item := rootCauseItem("sleep_wait", td.Thread, backgroundImpactMs(q, td.DurationMs, hasCausalChain, onChain), 0.74, td.LineStart, td.LineEnd, "window_stats.sleep_top", fmt.Sprintf("%s slept for %.3fms before wakeup%s", threadLabel(td.Thread), td.DurationMs, durationCPUDetail(td)))
		item.CumulativeImpactMs = td.DurationMs
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.StartTs = td.StartTs
		item.EndTs = td.EndTs
		item.DominantState = string(StateSSleep)
		item.SleepMs = td.DurationMs
		// ORD: same per-CPU 区分键 + producer-disjointness as the runnable
		// top above (one off-CPU state machine feeds all four buckets;
		// 复核 P3-1: same ordered-stream premise gate).
		item.MemberKey = fmt.Sprintf("cpu=%d", td.CPU)
		item.memberSegmentsProducerDisjoint = offCPUProducerDisjoint
		items = append(items, item)
	}
	items = append(items, rootCauseDIOStateFamilyItems(q, stats, chainThreads, hasCausalChain, offCPUProducerDisjoint)...)
	for _, churn := range stats.StateChurn {
		onChain := threadInSet(chainThreads, churn.Thread)
		// Physical dominant-state duration stays on the display lane. Root-rank
		// participation is derived only from the matching typed state caliber:
		// runnable in full, D/IO in full, sleep in full, and running only from
		// an independently computed CAP/supply deficit. StateChurn carries no
		// such running fold, so fragmented_running is authoritatively zero here.
		// In particular, the former "dominant + half the remaining states"
		// heuristic is a drilldown-priority hint, not a causal duration, and may
		// never enter EffectiveImpactMs/Score.
		item := rootCauseItem(stateChurnRootCauseType(churn.DominantState), churn.Thread, backgroundImpactMs(q, churn.DominantImpactMs, hasCausalChain, onChain), churn.Confidence, churn.LineStart, churn.LineEnd, "window_stats.state_churn", churn.Summary)
		item.CumulativeImpactMs = churn.TotalMs
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.DominantState = churn.DominantState
		item.RunningMs = churn.RunningMs
		item.RunnableMs = churn.RunnableMs
		item.SleepMs = churn.SleepMs
		item.DStateMs = churn.DStateMs
		item.IOWaitMs = churn.IOWaitMs
		item.EffectiveImpactMs = rootCauseFragmentedStateEffectiveImpactMs(item)
		item.Score = item.EffectiveImpactMs * item.Confidence * rootCauseItemScoreWeight(item)
		items = append(items, item)
	}
	// IRQBursts is count/span inventory only. It intentionally consumes no
	// root-rank seat: paired IRQActivity.ActiveMs is the sole elapsed-time
	// contender for the same physical interrupt rows.
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
		item.PhysicalSourcePath = work.SourcePath
		item.CumulativeImpactMs = work.DurationMs
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.StartTs = work.StartTs
		item.EndTs = work.EndTs
		// RCM §24.7.1 typed member identity.
		item.MemberKey = fmt.Sprintf("work=%s fn=%s", work.Work, work.Function)
		items = append(items, item)
	}
	for _, fence := range stats.DMAFenceActivity {
		impact := firstPositiveFloat(fence.WaitMs, fence.MaxWaitMs)
		if impact <= 0 {
			continue
		}
		onChain := threadInSet(chainThreads, fence.Thread)
		item := rootCauseItem("dma_fence_activity", fence.Thread, backgroundImpactMs(q, impact, hasCausalChain, onChain), 0.63, fence.LineStart, fence.LineEnd, "window_stats.dma_fence_activity", fence.Summary)
		item.PhysicalSourcePath = fence.SourcePath
		item.CumulativeImpactMs = impact
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.StartTs = fence.StartTs
		item.EndTs = fence.EndTs
		// RCM §24.7.1 typed member identity.
		item.MemberKey = fmt.Sprintf("driver=%s timeline=%s ctx=%s seqno=%s", fence.Driver, fence.Timeline, fence.Context, fence.Seqno)
		items = append(items, item)
	}
	if supply := stats.SupplyPressureSummary; supply != nil {
		// supply_pressure is the demand/capacity aggregate. Paired IPI active
		// time already owns the typed ipi_activity seat above; using the same
		// duration (or an event-count proxy) here let one physical interrupt
		// occupy two board seats. Keep IPI in SupplyPressureSummary as context,
		// but mint this rank row only from an independent demand-side duration.
		impact := firstPositiveFloat(supply.CPUPressureMs, (supply.SchedStatWaitMs+supply.SchedStatIOWaitMs+supply.SchedStatBlockedMs)*0.35)
		if impact > 0 {
			item := rootCauseItem("supply_pressure", ThreadRef{}, backgroundImpactMs(q, impact, hasCausalChain, false), 0.58, supply.LineStart, supply.LineEnd, "window_stats.supply_pressure_summary", supply.Summary)
			item.CumulativeImpactMs = impact
			items = append(items, item)
		}
	}
	// §21.1 CWD-2 ② (cmp_01 C7 产端半场): every rank row minted from a
	// window_stats summary carries the typed query-window identity the stats
	// were computed over. enrichRootCauseRankWithScheduler stamps its own
	// window_stats-family additions the same way.
	stampWindowStatsRankQueryWindow(items, stats.Window)
	demoteLockDominatedInversionCandidates(chain, stats, items)
	// RCX① (§12.3 ruling 1) + §20 E-Gap⑥: counterpart-lane rank rows
	// (blocking_span holders, binder_wait peers, io_latency completers) carry
	// the typed drill-debt verdict.
	stampRootCauseRankDrillStatus(idx, items, buildDrillSubjectUniverse(&chain, &stats), &chain, &stats)
	items = enrichRootCauseItemsWithChainContext(chain, items)
	attributeOnChainResourceItemsToWakeupDependency(chain, items)
	normalizeRootCauseChainRelevance(items, hasCausalChain)
	normalizeRootCauseSubjectKind(items)
	// SYM (§24.13 裁定一): typed self-subject identity for the election-ladder
	// skip arm — tid-first against the rank's own resolved target.
	stampRootCauseRankAnalysisTargetSubject(items, res.Target)
	// SYM-2 (§24.17 R2): the self runnable rows that now compete carry the
	// typed below-RT-preempted disclosure when the scheduling data proves it.
	stampRunnableSelfBelowRTPreempted(items, stats.RunnableContext)
	// RCM §24.7.1 (user ruling 2026-07-08): same-(thread,type) per-instance
	// rows merge into ONE contender per family BEFORE the sort, so the family
	// competes with its combined magnitude instead of splitting its own vote
	// (opendir_78 E5/E6: 1.136 rank#3 + 0.462 rank#8 → 1.598 one seat). Keyed
	// additionally on chain lane and typed selected window (M3 跨窗纪律).
	items = foldSameThreadTypeRankFamilies(q, hasCausalChain, items)
	// B4 (2026-07-10): the d_state_or_io_wait source row and its
	// io_burst_episode resource projection may be the exact same physical
	// segment. Reconcile only the adjudicated exact-match shape before sort /
	// capacity / ordinal assignment, so one segment owns one board seat while
	// the absorbed observation remains lossless on res.AbsorbedItems.
	res.Items = items
	reconcileExactCrossTypeRankSeats(&res)
	// UXR-1 §29.36③ (2026-07-11): the ◇ adjacent IO facet family folds into
	// ONE interval-union seat (成员 absorbed 明细不占席) — after the exact B4
	// recon, before sort/capacity/ordinals.
	reconcileAdjacentIOFacetFamilySeats(&res)
	items = res.Items
	normalizeRootCauseCumulativeImpact(items)
	normalizeRootCauseEffectiveImpact(items)
	sortRootCauseRankItems(items, hasCausalChain)
	limit := ViewCapacityFor("root_cause_rank").ClampLimit(q.Limit)
	items, candidateTotal, candidateEmitted, sideTotal, sideEmitted := truncateRootCauseRankCandidatesAndSideRows(items, limit)
	if candidateTotal > candidateEmitted {
		last := items[candidateEmitted-1]
		res.Compactions = append(res.Compactions, ViewCompaction{
			View:            "root_cause_rank",
			Dimension:       CompactionDimensionCandidates,
			Total:           candidateTotal,
			Emitted:         candidateEmitted,
			LastEmittedTs:   last.EndTs,
			LastEmittedLine: last.LineEnd,
		})
		res.Caveats = append(res.Caveats, fmt.Sprintf("root_cause_rank compacted from %d to %d competing candidate(s); rank-0 diagnostics do not consume candidate seats", candidateTotal, candidateEmitted))
	}
	if sideTotal > sideEmitted {
		res.Caveats = append(res.Caveats, fmt.Sprintf("root_cause_rank kept %d of %d rank-0 diagnostic/target-self disclosure row(s); these rows do not consume candidate seats", sideEmitted, sideTotal))
	}
	assignRootCauseRanksAndTiers(items)
	if caveat, ok := semanticSpanRankFailLoudCaveat(stats, items); ok {
		res.Caveats = append(res.Caveats, caveat)
	}
	if caveat, ok := semanticSpanCapLowerBoundCaveat(stats); ok {
		res.Caveats = append(res.Caveats, caveat)
	}
	if len(items) == 0 {
		res.Caveats = append(res.Caveats, "no deterministic root-cause candidates were found in the selected window")
	}
	res.Items = items
	res.Caveats = append(res.Caveats, stats.Caveats...)
	res.Caveats = append(res.Caveats, chain.Caveats...)
	return res
}

// stampRootEvidenceRankCaliber preserves the exact state scalar carried by a
// RootEvidence-only rank row. The closed effective-impact matrix deliberately
// refuses to infer runnable or D/IO attribution from a generic raw DurationMs;
// these exact root tokens therefore must populate the same typed channel as
// their WindowStats/CausalImpact counterparts. Sleep/missing/unknown remain
// context-only and are intentionally absent from this switch.
func stampRootEvidenceRankCaliber(root RootEvidence, item *RootCauseRankItem) {
	if item == nil || root.DurationMs <= 0 {
		return
	}
	switch strings.TrimSpace(root.Type) {
	case "runnable_wait":
		item.DominantState = string(StateRunnable)
		item.RunnableMs = root.DurationMs
	case "io_wait":
		item.DominantState = string(StateIOWait)
		item.IOWaitMs = root.DurationMs
	case "d_state_or_io_wait":
		item.DominantState = string(StateDSleep)
		item.DStateMs = root.DurationMs
	}
}

func rootEvidenceStateOwnedByWindowStats(root RootEvidence, stats WindowStats) bool {
	containsThread := func(items []ThreadDuration) bool {
		for _, td := range items {
			if td.DurationMs > 0 && sameThreadRef(td.Thread, root.Thread) {
				return true
			}
		}
		return false
	}
	switch strings.TrimSpace(root.Type) {
	case "runnable_wait":
		return containsThread(stats.RunnableTop)
	case "io_wait":
		return containsThread(stats.IOWaitTop)
	case "d_state_or_io_wait":
		return containsThread(stats.DStateTop) || containsThread(stats.IOWaitTop)
	default:
		return false
	}
}

// rootCauseDIOStateFamilyItems publishes one formal D/IO blocking account per
// numeric thread for the selected WindowStats run. computeOffCPUStats feeds
// mutually-exclusive DStateTop and IOWaitTop ledgers; their same-thread
// surviving bounded segments are therefore wall-clock additive when the
// ordered-stream proof is present. Keeping them in one contender prevents D
// and IO fragments from splitting their vote and gives StateChurn one formal
// account to reconcile. The upstream Top8-per-lane capacity remains disclosed
// separately; this helper never claims unseen members.
// dioStateFamilyMember is one accounting member feeding a formal D/IO
// blocking seat (state = the owning ledger's state token). A member is
// either the WHOLE (thread,cpu) ledger group (wholeTd — the pre-§29.50.5
// shape, byte-stable accounting straight off the ThreadDuration) or ONE
// proof-partition SLICE of an internally-split group (§29.50.5 逐片段证明门:
// the group held fragments proving different wait objects). The slice-view
// fields are the member's own accounting either way.
type dioStateFamilyMember struct {
	state string
	td    ThreadDuration
	cause string
	// slice view (whole-td members copy the group fields verbatim).
	durMs     float64
	segCount  int
	segMinMs  float64
	segMaxMs  float64
	startTs   float64
	endTs     float64
	lineStart int
	lineEnd   int
	wholeTd   bool
}

// dioStateMemberFromTd builds the whole-group member (pre-§29.50.5 accounting
// verbatim; cause = the group's single proven wait object, or "").
func dioStateMemberFromTd(state string, td ThreadDuration, cause string) dioStateFamilyMember {
	return dioStateFamilyMember{
		state: state, td: td, cause: cause, wholeTd: true,
		durMs: td.DurationMs, segCount: td.segCount,
		segMinMs: td.segMinMs, segMaxMs: td.segMaxMs,
		startTs: td.StartTs, endTs: td.EndTs,
		lineStart: td.LineStart, lineEnd: td.LineEnd,
	}
}

// dioStateMemberFromSlice builds one proof-partition slice member of an
// internally-split group.
func dioStateMemberFromSlice(state string, td ThreadDuration, cause string, slice offCPUCauseSlice) dioStateFamilyMember {
	return dioStateFamilyMember{
		state: state, td: td, cause: cause,
		durMs: slice.durMs, segCount: slice.segCount,
		segMinMs: slice.segMinMs, segMaxMs: slice.segMaxMs,
		startTs: slice.startTs, endTs: slice.endTs,
		lineStart: slice.lineStart, lineEnd: slice.lineEnd,
	}
}

func rootCauseDIOStateFamilyItems(q Query, stats WindowStats, chainThreads map[int]bool, hasCausalChain, producerDisjoint bool) []RootCauseRankItem {
	groups := map[string][]dioStateFamilyMember{}
	threads := map[string]ThreadRef{}
	var order []string
	add := func(state string, td ThreadDuration) {
		if td.Thread.PID <= 0 || td.DurationMs <= 0 {
			return
		}
		key := threadKey(td.Thread)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		// §29.50.5: a group whose slice inventory holds ONE cause key is
		// whole (accounting verbatim off the group — the pre-partition
		// shape, incl. CASE-1 same-source floats); an internally-split
		// group contributes one member per slice (deterministic order:
		// sorted keys, "" first). The unproven-overlap fallback
		// (producerDisjoint=false) never partitions — the lower-bound
		// election needs whole groups (宁漏勿猜).
		if !producerDisjoint || len(td.causeSlices) <= 1 {
			cause := ""
			if producerDisjoint {
				for c := range td.causeSlices {
					cause = c
				}
			}
			groups[key] = append(groups[key], dioStateMemberFromTd(state, td, cause))
		} else {
			causes := make([]string, 0, len(td.causeSlices))
			for c := range td.causeSlices {
				causes = append(causes, c)
			}
			sort.Strings(causes)
			for _, c := range causes {
				groups[key] = append(groups[key], dioStateMemberFromSlice(state, td, c, td.causeSlices[c]))
			}
		}
		current := threads[key]
		if current.PID == 0 || threadDisplayLess(td.Thread, current) {
			threads[key] = td.Thread
		}
	}
	// 修复轮二 件A (2026-07-13): the formal family accounts read the FULL
	// pre-cap census — 席位按全量账铸值 (tieba 60555 witness: the capped
	// basis under-reported the fscache cause seat 4.739 vs provable 7.386
	// and left hmfs_get_dnode 0.171 seatless). Selection reuses the display
	// lists' OWN comparator uncapped (topThreadDurations — 单一值源: when
	// nothing is evicted the member sequence is byte-identical to the
	// pre-census mint). A WindowStats built without the census (legacy
	// fixtures/direct literals) fails open to the capped lists verbatim.
	ioMembers := stats.IOWaitTop
	if stats.iowaitCensus != nil {
		ioMembers = topThreadDurations(stats.iowaitCensus, len(stats.iowaitCensus))
	}
	dMembers := stats.DStateTop
	if stats.dstateCensus != nil {
		dMembers = topThreadDurations(stats.dstateCensus, len(stats.dstateCensus))
	}
	for _, td := range ioMembers {
		add(string(StateIOWait), td)
	}
	for _, td := range dMembers {
		add(string(StateDSleep), td)
	}

	out := make([]RootCauseRankItem, 0, len(order))
	for _, key := range order {
		allMembers := groups[key]
		if len(allMembers) == 0 {
			continue
		}
		thread := threads[key]
		onChain := threadInSet(chainThreads, thread)
		// §29.50.5 证明分区 (v5 P1 批 件②, 2026-07-13): the participation
		// aggregation key refines to (thread, state family, root-cause
		// identity). Fragments whose typed markers proved a wait object were
		// ledger-grouped under that cause (addDurationCause) — each proven
		// cause mints its OWN seat (跨 token 合并: D and iowait fragments of
		// one wait object join ONE seat); the unproven fragments keep the
		// generic seat, wearing the honest-remainder marker when a sibling
		// cause seat exists (绝不灌根因席). The unproven-overlap fallback
		// (producerDisjoint=false) never partitions — the lower-bound
		// election needs the whole member set (宁漏勿猜).
		partitions := map[string][]dioStateFamilyMember{}
		var partOrder []string
		if producerDisjoint {
			for _, m := range allMembers {
				c := m.cause
				if _, ok := partitions[c]; !ok {
					partOrder = append(partOrder, c)
				}
				partitions[c] = append(partitions[c], m)
			}
		} else {
			partitions[""] = allMembers
			partOrder = []string{""}
		}
		hasCauseSeat := false
		for _, c := range partOrder {
			if c != "" {
				hasCauseSeat = true
			}
		}
		for _, cause := range partOrder {
			members := partitions[cause]
			if len(members) == 0 {
				continue
			}
			out = append(out, mintRootCauseDIOStateSeat(q, stats, hasCausalChain, producerDisjoint,
				thread, onChain, members, cause, cause == "" && hasCauseSeat))
		}
	}
	return out
}

// mintRootCauseDIOStateSeat mints ONE formal D/IO blocking seat from the
// given proof partition of a thread's ledger groups (§29.50.5; the whole
// pre-partition body of rootCauseDIOStateFamilyItems, factored per seat).
// cause != "" ⇒ every fragment proved that wait object (the seat carries it
// as BlockedReasonCaller); unprovenRemainder ⇒ the generic seat sits beside
// ≥1 sibling cause seat and wears the typed honest-remainder marker.
func mintRootCauseDIOStateSeat(q Query, stats WindowStats, hasCausalChain, producerDisjoint bool,
	thread ThreadRef, onChain bool, members []dioStateFamilyMember,
	cause string, unprovenRemainder bool) RootCauseRankItem {
	{
		total, dStateMs, ioWaitMs := 0.0, 0.0, 0.0
		maxMs, minMs := 0.0, 0.0
		startTs, endTs := 0.0, 0.0
		hasStart := false
		lineStart, lineEnd := 0, 0
		var roster []string
		// CASE-1 gap (b) (§29.52 立案, v5 P1 批, 2026-07-13): the validated
		// per-member inventory (same-source interval + value floats per
		// member) — the G1 cross-lane reconciliation's eligibility gate and
		// same-source identity input. Engine-internal, never serialized
		// (same carrier as the io_latency family fold). Whole-td members
		// carry the exact ThreadDuration floats the chain lane copies; slice
		// members carry slice extents (an internally-split group's chain twin
		// carries the WHOLE-group floats, so absorption fails open there —
		// honest dual beats wrong absorption).
		var memberIntervals []foldInterval
		strongest := members[0]
		for _, m := range members {
			total += m.durMs
			if m.state == string(StateIOWait) {
				ioWaitMs += m.durMs
			} else {
				dStateMs += m.durMs
			}
			if m.durMs > strongest.durMs {
				strongest = m
			}
			// F-1 (冷读 P1): the published a–b range is the TRUE single-
			// segment range (P8 规格) — the engine holds the per-segment
			// inventory; group sums must never masquerade as 单段. Members
			// without the inventory (defensive) fall back to their sum.
			memberMin, memberMax := m.segMinMs, m.segMaxMs
			if m.segCount == 0 {
				memberMin, memberMax = m.durMs, m.durMs
			}
			if maxMs == 0 || memberMax > maxMs {
				maxMs = memberMax
			}
			if minMs == 0 || (memberMin > 0 && memberMin < minMs) {
				minMs = memberMin
			}
			if m.endTs > m.startTs && (!hasStart || m.startTs < startTs) {
				startTs = m.startTs
				hasStart = true
			}
			if m.endTs > endTs {
				endTs = m.endTs
			}
			if m.endTs > m.startTs {
				memberIntervals = append(memberIntervals,
					foldInterval{start: m.startTs, end: m.endTs, valueMs: m.durMs})
			}
			applyLineRange(&lineStart, &lineEnd, m.lineStart)
			applyLineRange(&lineStart, &lineEnd, m.lineEnd)
			if len(roster) < rootCauseFamilyRosterCap {
				// F-1 (冷读 P1, 2026-07-12): a multi-segment member is a
				// per-CPU group SUM — say so ("合计…(N段)"); only a true
				// single segment keeps the bare form. 计数当量-precedent:
				// zh caliber words already live on engine roster faces.
				if m.segCount > 1 {
					roster = append(roster, fmt.Sprintf("%s cpu=%d 合计%.3fms(%d段)", m.state, m.td.CPU, m.durMs, m.segCount))
				} else {
					roster = append(roster, fmt.Sprintf("%s cpu=%d %.3fms", m.state, m.td.CPU, m.durMs))
				}
			}
		}
		caliber := RootCauseMemberFoldCaliberSumDisjoint
		memberSum := 0.0
		if len(members) > 1 && !producerDisjoint {
			// Clock regression removes the producer disjointness proof. Publish
			// the strongest exact member as an honest lower bound, never Σ.
			memberSum = total
			total = strongest.durMs
			dStateMs, ioWaitMs = 0, 0
			if strongest.state == string(StateIOWait) {
				ioWaitMs = total
			} else {
				dStateMs = total
			}
			caliber = RootCauseMemberFoldCaliberMaxOverlapFallback
		}
		typ := "d_state_or_io_wait"
		dominant := string(StateDSleep)
		confidence := 0.82
		if dStateMs == 0 && ioWaitMs > 0 {
			typ = "io_wait"
			dominant = string(StateIOWait)
			confidence = 0.84
		} else if ioWaitMs > dStateMs {
			dominant = string(StateIOWait)
		}
		source := "window_stats"
		if typ == "io_wait" {
			source = "window_stats.io_wait_top"
		}
		summary := fmt.Sprintf("%s had a mutually-exclusive D/IO blocking account of %.3fms (d_state=%.3fms io_wait=%.3fms)", threadLabel(thread), total, dStateMs, ioWaitMs)
		if caliber == RootCauseMemberFoldCaliberMaxOverlapFallback {
			summary = fmt.Sprintf("%s had an unproven-overlap D/IO lower bound of %.3fms (member_max; raw_member_sum=%.3fms)", threadLabel(thread), total, memberSum)
		}
		item := rootCauseItem(typ, thread, backgroundImpactMs(q, total, hasCausalChain, onChain), confidence,
			lineStart, lineEnd, source, summary)
		item.CumulativeImpactMs = total
		item.Causality = causalityLabel(hasCausalChain, onChain)
		item.StartTs, item.EndTs = startTs, endTs
		item.DominantState = dominant
		item.DStateMs, item.IOWaitMs = dStateMs, ioWaitMs
		// DSTATE-REFINE arm a (件③, 2026-07-12): the refined 「D-state」 proof
		// + the unanimous caller disclosure — computed ONLY on the exact
		// sum_disjoint account (the unproven-overlap fallback rewrote the
		// D/IO split and keeps the honest merged word).
		if caliber == RootCauseMemberFoldCaliberSumDisjoint && ioWaitMs == 0 && dStateMs > 0 {
			segments, marked := 0, 0
			callerUnanimous, callerConflict := "", false
			for _, m := range members {
				if m.state != string(StateDSleep) || !m.wholeTd {
					// §29.50.5: coverage counters live on the WHOLE group; a
					// slice member of a split group cannot re-prove them
					// (its cause identity is stamped directly below).
					continue
				}
				segments += m.td.dFamilySegments
				marked += m.td.dFamilyNonIOMarked
				for _, c := range m.td.dFamilyCallers {
					// The kernel caller form is symbol+offset[module]
					// (dma_fence_default_wait+0x74/0x160[sysmgr.elf]) — the
					// 等待对象 word is the SYMBOL (same wait object across
					// offsets; single reducer = offCPUCauseSymbol, 件②).
					c = offCPUCauseSymbol(c)
					if callerUnanimous == "" {
						callerUnanimous = c
					} else if callerUnanimous != c {
						callerConflict = true
					}
				}
			}
			if segments > 0 && marked == segments {
				item.DStateAllNonIOProven = true
				if !callerConflict {
					item.BlockedReasonCaller = callerUnanimous
				}
			}
		}
		// §29.50.5 (v5 P1 批 件②): a proof-partition cause seat carries its
		// proven wait object regardless of the D/IO mix — 跨 token 合并: the
		// mixed D+iowait cause seat keeps the merged type token AND the
		// 等待对象 caller word (the refined non-IO proof above stays the
		// pure-D arm's business); every fragment in this partition proved the
		// symbol at the ledger (addDurationCause), so this is never a guess.
		if cause != "" && item.BlockedReasonCaller == "" {
			item.BlockedReasonCaller = cause
		}
		if cause != "" && caliber == RootCauseMemberFoldCaliberSumDisjoint &&
			ioWaitMs == 0 && dStateMs > 0 && !item.DStateAllNonIOProven {
			// A pure-D cause partition is refined-proven BY CONSTRUCTION:
			// the D-bucket cause slices only mint from fragments whose own
			// iowait=0 marker named the symbol (the whole-group coverage
			// counters are unreachable on slice members).
			item.DStateAllNonIOProven = true
		}
		item.DStateCauseUnprovenRemainder = unprovenRemainder
		// CR-3 件② P10 (2026-07-12): the unconsumed-marker residual. When the
		// unanimous lane minted NO caller (partial coverage, conflicting or
		// opaque symbols, or a non-sum_disjoint fold), the window account may
		// still hold sched_blocked_reason records for this thread — the 冷读
		// 案7 shape: the root-cause row reads 未解析 while the GPU-fence
		// marker sits in hand. Mint the typed residual (count + distinct
		// semantic symbols) so the display can DISCLOSE; never a synthesized
		// upstream identity, never a rank/score input.
		// 修复轮 P2 (2026-07-12): the count reads the FULL pre-truncation
		// accumulator (INODE §28.6 precedent) — the first cut aggregated the
		// top-8 inventory and said 17 while the window held 19 (冷读直核).
		// 件② guard: the honest remainder beside a sibling cause seat never
		// re-discloses markers the sibling already consumed (双说防线).
		if item.BlockedReasonCaller == "" && !unprovenRemainder {
			if total, ok := stats.blockedReasonFullByPID[thread.PID]; ok && total.count > 0 {
				item.BlockedReasonWindowCount = total.count
				item.BlockedReasonWindowCaller = strings.Join(total.callers, "/")
			}
		}
		item.memberSegmentsProducerDisjoint = producerDisjoint
		item.EffectiveImpactMs = rootCauseEffectiveImpactMs(item)
		item.Score = item.EffectiveImpactMs * item.Confidence * rootCauseItemScoreWeight(item)
		if len(members) > 1 {
			item.MemberCount = len(members)
			item.MemberRoster = roster
			item.MemberMaxMs, item.MemberMinMs = maxMs, minMs
			item.MemberFoldCaliber = caliber
			item.MemberSumMs = memberSum
			item.familyMemberIntervals = memberIntervals
		}
		return item
	}
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
		// R5g (§7.30.2): only displacement-evidenced high-priority overlap
		// raises confidence; window-total high-priority running is background.
		if item.HighPriorityRunningOverlapMs > 0 {
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
		// R5e (§7.30.2): the low-frequency judgement integrates the frequency
		// over the whole wait interval and benchmarks against the max observed
		// inside/nearest that interval — a stale point sample at the wait
		// start compared against the window residency max is forbidden.
		if weightedFrequencyIsLow(item.WeightedFrequency, item.ObservedMaxFrequency) {
			lowSummary := fmt.Sprintf("%s runnable wait ran with weighted_freq=%dkHz on cpu=%d, at or below 65%% of the nearby observed max %dkHz", threadLabel(item.Thread), item.WeightedFrequency, item.CPU, item.ObservedMaxFrequency)
			if item.FrequencySample != "" {
				lowSummary = fmt.Sprintf("%s frequency_sample=%s", lowSummary, item.FrequencySample)
			}
			low := rootCauseItem("low_frequency", item.Thread, backgroundImpactMs(q, item.DurationMs, hasCausalChain, onChain), 0.70, item.StartLine, item.EndLine, "scheduler_latency_stats", lowSummary)
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
			// RN-15 (§7.4 demand/supply separation, §7.9): a per-thread
			// runnable/running wait judged verdict=cpu_pressure is DEMAND-side
			// scheduling-pressure evidence and already rides the
			// runnable_wait / scheduler_latency / cpu_pressure token family —
			// publishing it again as compute_supply double-counted the same
			// wait (customer cust_large_3s: the identical 2.661/2.908ms
			// runnable waits appeared once as type=runnable_wait,
			// source=window_stats and once as type=compute_supply,
			// source=window_stats.compute_supply, and the projection grew a
			// phantom "影响点 compute_supply" row). The compute_supply token is
			// reserved for the aggregate delivery-side ledger (supply ratio /
			// low-frequency loss / idle mismatch / core-limited,
			// compute_supply_balance) and must never carry a concrete thread
			// subject; per-thread low-frequency verdicts keep their own
			// low_frequency token unchanged.
			if typ == "compute_supply" && !computeSupplyCauseSubjectAllowed(supply.Thread) {
				continue
			}
			onChain := threadInSet(chainThreads, supply.Thread)
			dominantState := computeSupplyDominantState(supply)
			// A running low-frequency verdict is already represented by the
			// causal running row's CAP/supply-deficit attribution. Publishing the
			// same raw running DurationMs as a second low_frequency contender both
			// bypasses the fold and consumes a duplicate seat. Keep the verdict in
			// WindowStats for diagnosis, but do not mint an independent rank row.
			if typ == "low_frequency" && dominantState == string(StateRunning) {
				continue
			}
			candidate := rootCauseItem(typ, supply.Thread, backgroundImpactMs(q, supply.DurationMs, hasCausalChain, onChain), supply.Confidence, supply.LineStart, supply.LineEnd, "window_stats.compute_supply", supply.Summary)
			candidate.CumulativeImpactMs = supply.DurationMs
			candidate.Causality = causalityLabel(hasCausalChain, onChain)
			candidate.ChainRelevance = chainRelevanceFromCausality(candidate.Causality)
			candidate.DominantState = dominantState
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
	// §21.1 CWD-2 ②: the compute_supply / cpu_constraints additions above are
	// window_stats-derived too — same typed window stamp as the build pass
	// (idempotent on rows already stamped there).
	stampWindowStatsRankQueryWindow(rank.Items, stats.Window)
	normalizeRootCauseChainRelevance(rank.Items, hasCausalChain)
	normalizeRootCauseSubjectKind(rank.Items)
	// SYM (§24.13 裁定一): the scheduler/compute additions above are new rows —
	// re-stamp the whole slice with the same typed target (idempotent).
	stampRootCauseRankAnalysisTargetSubject(rank.Items, rank.Target)
	// SYM-2 (§24.17 R2): the enrich-minted scheduler_latency self rows are the
	// highest-frequency runnable-family additions — same typed RT disclosure.
	stampRunnableSelfBelowRTPreempted(rank.Items, stats.RunnableContext)
	attributeOnChainResourceItemsToWakeupDependency(chain, rank.Items)
	// RCM §24.7.1: the enrich pass mints new per-instance rows (multiple
	// scheduler_latency segments per thread) — same family merge before its
	// re-sort. Idempotent over the build pass: already-merged families arrive
	// as single rows and pass through untouched.
	rank.Items = foldSameThreadTypeRankFamilies(q, hasCausalChain, rank.Items)
	// Idempotent B4 recomputation after enrichment: scheduler additions cannot
	// silently resurrect the absorbed cross-type seat, and the dedicated
	// lossless carrier is rejoined to the same exact engine key.
	reconcileExactCrossTypeRankSeats(&rank)
	// UXR-1 §29.36③: idempotent adjacent IO facet family recomputation (the
	// pass reset-first restores its own absorbed members before refolding).
	reconcileAdjacentIOFacetFamilySeats(&rank)
	normalizeRootCauseCumulativeImpact(rank.Items)
	normalizeRootCauseEffectiveImpact(rank.Items)
	sortRootCauseRankItems(rank.Items, hasCausalChain)
	limit := ViewCapacityFor("root_cause_rank").ClampLimit(q.Limit)
	var candidateTotal, candidateEmitted, sideTotal, sideEmitted int
	rank.Items, candidateTotal, candidateEmitted, sideTotal, sideEmitted = truncateRootCauseRankCandidatesAndSideRows(rank.Items, limit)
	if candidateTotal > candidateEmitted {
		last := rank.Items[candidateEmitted-1]
		rank.Compactions = append(rank.Compactions, ViewCompaction{
			View:            "root_cause_rank",
			Dimension:       CompactionDimensionCandidates,
			Total:           candidateTotal,
			Emitted:         candidateEmitted,
			LastEmittedTs:   last.EndTs,
			LastEmittedLine: last.LineEnd,
		})
		rank.Caveats = append(rank.Caveats, fmt.Sprintf("root_cause_rank compacted after scheduler/compute enrichment from %d to %d competing candidate(s); rank-0 diagnostics do not consume candidate seats", candidateTotal, candidateEmitted))
	}
	if sideTotal > sideEmitted {
		rank.Caveats = append(rank.Caveats, fmt.Sprintf("root_cause_rank kept %d of %d rank-0 diagnostic/target-self disclosure row(s) after enrichment; these rows do not consume candidate seats", sideEmitted, sideTotal))
	}
	assignRootCauseRanksAndTiers(rank.Items)
	rank.Caveats = append(rank.Caveats, latency.Caveats...)
	return rank
}

func attachPerfContextToRootCauseRank(idx *Index, q Query, rank RootCauseRankResult, stats WindowStats) RootCauseRankResult {
	// CR-3 件③ P11: the process-identity stamp rides the shared finalize
	// tail so EVERY rank lane ships it (see stampRootCauseProcessIdentity).
	stampRootCauseProcessIdentity(idx, q, &rank)
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
			contexts = appendRootCausePerfRoleContext(contexts, "target_running", item.Thread, -1, window, "running-state CPU work", perfContextForExecutionThread(idx, q, item.Thread, start, end, 4))
		}
	}
	return contexts
}

func appendRootCauseCPUPressurePerfContexts(idx *Index, q Query, pressure CPUPressureStats, window TimeWindow, contexts []RootCausePerfRoleContext) []RootCausePerfRoleContext {
	start, end := window.StartTs, window.EndTs
	contexts = appendRootCausePerfRoleContext(contexts, "cpu_pressure_cpu", ThreadRef{}, pressure.CPU, window, "CPU pressure scope", perfContextForCPU(idx, q, pressure.CPU, start, end, 4))
	// R5g: competitor perf contexts prefer the displacement-overlap set —
	// full-window TopRunning includes serially-pipelined threads that never
	// displaced anyone; those stay background pressure, not competitors.
	if len(pressure.OverlapCompetitors) > 0 {
		return appendRootCauseRunnableCompetitorPerfContexts(idx, q, pressure.OverlapCompetitors, ThreadRef{}, window, "cpu_pressure_top_running", "displacement-overlap running thread on pressure CPU", contexts)
	}
	return appendRootCauseRunnableCompetitorPerfContexts(idx, q, pressure.TopRunning, ThreadRef{}, window, "cpu_pressure_top_running", "top running thread on pressure CPU (window background, no displacement overlap)", contexts)
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
		ctx := perfContextForExecutionThread(idx, q, td.Thread, start, end, 4)
		roleWindow := TimeWindow{StartTs: start, EndTs: end}
		if ctx == nil && !sameTimeWindow(roleWindow, window) {
			ctx = perfContextForExecutionThread(idx, q, td.Thread, window.StartTs, window.EndTs, 4)
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
		executionCPU, ok := perfSampleOnCPUExecutionCPU(ev)
		return ok && executionCPU == cpu
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

// rootCauseSupplyPressureOwnsCPURankSeat proves that the window-level demand
// backlog is exactly the sum of the per-CPU runnable-wait components carried
// by the same WindowStats value. This is a hard seat-reconciliation gate, so
// it uses exact typed arithmetic in the same iteration order as
// computeSupplyPressureSummary; no tolerance or summary text participates.
// The component rows remain available in WindowStats for drilldown.
func rootCauseSupplyPressureOwnsCPURankSeat(stats WindowStats) bool {
	supply := stats.SupplyPressureSummary
	if supply == nil || supply.CPUPressureMs <= 0 {
		return false
	}
	componentCount := 0
	componentSum := 0.0
	for _, pressure := range stats.CPUPressure {
		if pressure.RunnableWaitMs <= 0 {
			continue
		}
		componentCount++
		componentSum += pressure.RunnableWaitMs
	}
	return componentCount > 0 && componentSum == supply.CPUPressureMs
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
	if end <= start {
		return fallback.StartTs, fallback.EndTs
	}
	if fallback.EndTs > fallback.StartTs && start < fallback.StartTs {
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
		TargetRunningPerf: perfContextForExecutionThread(idx, q, target, q.TimeStart, q.TimeEnd, 6),
	}
	if chain != nil {
		out.OnChainPerf = perfContextForThreads(idx, q, chainThreadRefs(*chain), 6)
	}
	out.BinderPeerPerf = perfContextForThreads(idx, q, binderPeerThreadRefs(blocking), 6)
	if cpuCtx := perfContextForCPUs(idx, q, sameCPUCompetitorCPUs(stats), 6); cpuCtx != nil {
		out.SameCPUCompetitorPerf = cpuCtx
	} else {
		out.SameCPUCompetitorPerf = perfContextForExecutionThreads(idx, q, sameCPUCompetitorThreadRefs(stats), 6)
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
		// R5g (§7.30.2): only threads whose running overlapped another
		// thread's runnable wait qualify as same-CPU competitors; window
		// top-running co-residents without overlap are serial hand-offs.
		for _, td := range pressure.OverlapCompetitors {
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

// rootCauseAggregateMetricTypes lists the root-cause row types whose subject
// is a window/CPU-scoped aggregate metric rather than a resolvable thread.
// Kept in lockstep with the construction sites that pass ThreadRef{} (or a
// merely representative thread) for these types.
var rootCauseAggregateMetricTypes = map[string]bool{
	"cpu_pressure":        true,
	"io_pressure":         true,
	"cpu_frequency_limit": true,
	"irq_burst":           true,
	"irq_activity":        true,
	"ipi_activity":        true,
	"supply_pressure":     true,
}

// normalizeRootCauseSubjectKind marks aggregate-metric rows whose ThreadRef is
// structurally empty so renderers stop presenting them as an "unknown thread":
// there is no thread to resolve — the subject IS the metric (§7.30 complaint
// 1/6/8). Rows that borrowed a representative thread keep the thread shape.
func normalizeRootCauseSubjectKind(items []RootCauseRankItem) {
	for i := range items {
		if items[i].SubjectKind != "" || !rootCauseAggregateMetricTypes[items[i].Type] {
			continue
		}
		if items[i].Thread.PID > 0 || strings.TrimSpace(items[i].Thread.Comm) != "" {
			continue
		}
		items[i].SubjectKind = RootCauseSubjectKindAggregateMetric
	}
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

func normalizeRootCauseEffectiveImpact(items []RootCauseRankItem) {
	for i := range items {
		if items[i].PeriodicSource {
			// VS-1 (§7.8): a periodic-source row's effective impact IS its
			// discounted value, even when exactly 0 (pure in-period cadence) —
			// the cumulative fallback would resurrect the raw sleep this lane
			// exists to discount. Precise boolean gate.
			continue
		}
		if items[i].EffectiveImpactMs > 0 {
			continue
		}
		if spec, ok := CausalTokenSpecFor(items[i].Type); ok && spec.Additivity == CausalAdditivityCount {
			// G3 count-family identity (§27.2, 2026-07-09): a count-class
			// advisory scalar's EFFECTIVE attribution is the PUBLISHED (window
			// -capped) ImpactMs — the same value the Score/sort lanes already
			// consume — never the raw count-equivalent CumulativeImpactMs the
			// generic fallback below would resurrect (opendir_79 页缓存抖动:
			// effective=198.300 count-equivalent "ms" against a 41.671 tree
			// face broke the Σ计入==V==发布值 identity engine-side). The raw
			// count-equivalent stays disclosed on the cumulative / member_sum
			// channels. Conservative on both sides: the effective face never
			// exceeds the capped published value, and the raw disclosure
			// channel keeps the full uncapped magnitude.
			items[i].EffectiveImpactMs = items[i].ImpactMs
			continue
		}
		items[i].EffectiveImpactMs = rootCauseEffectiveImpactMs(items[i])
	}
	for i := range items {
		previous := items[i].EffectiveImpactMs
		canonical := rootCauseEffectiveImpactMs(items[i])
		items[i].EffectiveImpactMs = canonical
		if canonical > 0 {
			// Preserve specialized score weights while bringing any pre-cap score
			// onto the same published effective scalar. This matters for
			// off-chain inversion/state rows whose mint-time score used raw ms.
			if previous > 0 && canonical < previous && items[i].Score > 0 {
				items[i].Score *= canonical / previous
			}
			continue
		}
		// A typed zero attribution is rank-0 context. It cannot retain a
		// positive score/boost minted earlier from the raw wall-clock display
		// lane: that would publish "not participating" and a positive ranking
		// score on the same row, and could perturb zero-row order.
		items[i].Score = 0
		items[i].RankSortBoostedEffectiveMs = 0
	}
}

func sortRootCauseRankItems(items []RootCauseRankItem, chainAware bool) {
	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := 0, 0
		if chainAware {
			ri = rootCauseChainRelevanceSortRank(items[i])
			rj = rootCauseChainRelevanceSortRank(items[j])
			if ri != rj {
				return ri < rj
			}
		}
		// Root-cause ordinals and capacity are decided by the published typed
		// effective attribution for every lane. Score/weights are tie-breakers
		// only; they may not let a confidence heuristic or semantic boost evict
		// a row with a larger measured effective impact.
		ci := rootCauseEffectiveImpactMs(items[i])
		cj := rootCauseEffectiveImpactMs(items[j])
		if ci != cj {
			return ci > cj
		}
		if !chainAware || ri == chainRelevanceRank("on_chain") {
			if chainAware {
				// …已解析对端行优先 — at equal measured impact the row with a
				// RESOLVED contention counterpart outranks the one without…
				pi := rootCauseItemHasResolvedBlockingPeer(items[i])
				pj := rootCauseItemHasResolvedBlockingPeer(items[j])
				if pi != pj {
					return pi
				}
				// …承自行让位 — and a row additionally banking on an inherited
				// dependency-window annotation yields to a purely measured one.
				ii := items[i].InheritedTargetBlockedMs > 0
				ij := items[j].InheritedTargetBlockedMs > 0
				if ii != ij {
					return ij
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

// rootCauseItemHasResolvedBlockingPeer is the §12.3 ruling-2 tie-break signal:
// a typed contention row whose counterpart is resolved (same precise pair as
// the direct-on-chain admission gate).
func rootCauseItemHasResolvedBlockingPeer(item RootCauseRankItem) bool {
	return item.BlockingKind != "" && threadRefResolved(item.BlockingPeer)
}

// truncateRootCauseRankItemsStrict applies capacity to the already sorted
// candidate board without type-specific displacement. Semantic work competes
// by the same published effective value as every other candidate; when it is
// below the cut, the independent semantic-optimization disclosure channel is
// responsible for mentioning it. This keeps Rank, capacity, and effective
// attribution on one deterministic order.
func truncateRootCauseRankItemsStrict(items []RootCauseRankItem, limit int) []RootCauseRankItem {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

// semanticSpanRankFailLoudCaveat (DCS E3, ledger §23 义务② fail-loud gap;
// family grain per RCM §24.12: classified>0 ∧ published semantic FAMILY==0):
// window_stats classified semantic optimization spans but the published rank
// carries ZERO semantic rows — a structurally silent loss (the near-miss
// caveat only covers UNclassified vocabulary). Precise counting comparison on
// typed fields only; any published semantic row (each row IS one family after
// the §24.10 fold) silences it. The counted unit is the (thread, semantic
// class) family, matching what the rank can actually publish post-fold.
func semanticSpanRankFailLoudCaveat(stats WindowStats, items []RootCauseRankItem) (string, bool) {
	classifiedFamilies := map[string]bool{}
	classifiedSpans := 0
	for _, span := range stats.TraceSpans {
		if strings.TrimSpace(span.SemanticClass) != "" {
			classifiedSpans++
			classifiedFamilies[threadKey(span.Thread)+"\x00"+strings.TrimSpace(span.SemanticClass)] = true
		}
	}
	if len(classifiedFamilies) == 0 {
		return "", false
	}
	for _, item := range items {
		if rootCauseItemIsSemanticSpanWork(item) {
			return "", false
		}
	}
	return fmt.Sprintf("window_stats.trace_spans holds %d classified semantic optimization span(s) across %d same-thread famil(ies) but root_cause_rank published 0 semantic rows (%d row(s) published in total); the ranked causes are incomplete for deterministic-optimization accounting — inspect window_stats.trace_spans directly", classifiedSpans, len(classifiedFamilies), len(items)), true
}

// semanticSpanCapLowerBoundCaveat (RCM §24.12 复核漏项补充④): the semantic
// span reservation (traceMarkSemanticSpanCap) arriving EXACTLY full is the
// precise witness that the engine bound MAY have dropped further classified
// spans — every family total computed from the surviving spans is then a
// LOWER BOUND and must say so instead of shipping a silently short 合计
// (cmp_78_01: the 7.0 side hit precisely 16). Soft disclosure only (a caveat,
// never a gate): the exact-cap signal is precise but cannot count the drops.
func semanticSpanCapLowerBoundCaveat(stats WindowStats) (string, bool) {
	classified := 0
	for _, span := range stats.TraceSpans {
		if strings.TrimSpace(span.SemanticClass) != "" {
			classified++
		}
	}
	if classified < traceMarkSemanticSpanCap {
		return "", false
	}
	return fmt.Sprintf("the semantic span capacity (%d) is exactly full: additional classified spans may have been dropped at the window-stats bound, so every semantic family total is a lower bound (>=) of the window's true optimization volume", traceMarkSemanticSpanCap), true
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

func rootCauseEffectiveImpactMs(item RootCauseRankItem) float64 {
	effective := rootCauseEffectiveImpactMsUncapped(item)
	// Off-chain rows live on the background board. When a causal chain exists,
	// backgroundImpactMs applies the long-standing selected-window cap on the
	// published ImpactMs; the effective channel must honor that same cap for a
	// singleton exactly as foldRankFamily already does for a family. Otherwise
	// splitting one physical background account into two members changed its
	// value from raw to capped and reordered the board.
	if !rootCauseItemIsOnChain(item) && item.ImpactMs > 0 && effective > item.ImpactMs {
		return item.ImpactMs
	}
	return effective
}

func rootCauseEffectiveImpactMsUncapped(item RootCauseRankItem) float64 {
	if item.PeriodicSource {
		// VS-1 (§7.8): the discounted value is authoritative even at 0.
		return item.EffectiveImpactMs
	}
	// Semantic deterministic work participates by its measured intersection
	// projection, even when that work happened while the host thread was
	// running. It is not a generic running-state row and must never be replaced
	// by the host CPU supply deficit.
	if rootCauseItemIsSemanticSpanWork(item) {
		return item.EffectiveImpactMs
	}
	// A typed priority-inversion row owns its gated algorithm result
	// authoritatively, including zero. Falling through at zero would resurrect
	// the raw runnable/running/D-IO wall clock under an inversion label even
	// though the gate proved no inversion-attributable impact.
	if rootCauseTypeIsPriorityInversion(item.Type) {
		return item.EffectiveImpactMs
	}
	// The family fold has already applied its typed disjoint/union/MAX ruler
	// across member effective values. Interval-union families deliberately
	// clear per-state scalar sums when a channel split cannot be reconstructed;
	// the folded effective field is therefore the authoritative family scalar.
	if item.MemberCount > 1 && strings.TrimSpace(item.MemberFoldCaliber) != "" {
		return item.EffectiveImpactMs
	}
	// EVOLUTION RECORD (SEM-LEAD 复核 P1-1, §29.22 修向(a), 2026-07-10): the
	// batch's first cut returned RankSortBoostedEffectiveMs here, making the
	// ON-CHAIN ORDINAL key the boosted heuristic while the display board /
	// ❶❷❸ badges order by the published EffectiveImpactMS — the same page
	// showed ❶ on rank#2 and ❷ on rank#1 (序值倒挂, zero disclosure). §7.30
	// S1: a synthetic ranking score must never publish as an ms hard fact —
	// and the rank ordinal IS a published face. The accessor therefore reads
	// the PUBLISHED value only; the boost survives solely as the
	// rootCauseRankScoreBasisMs same-effective tie-break and never affects the
	// strict capacity prefix.
	if rootCauseItemIsRunningCaliber(item) {
		// Running participates only through its ELIMINABLE CAP/supply-fold
		// deficit. Missing fold data and a fully supplied interval both mean
		// effective=0; raw RunningMs/ImpactMs remain display evidence only.
		// This includes fragmented_running and any running compute-delivery
		// observation, closing the raw-Duration low_frequency bypass.
		return item.EffectiveImpactMs
	}
	if rootCauseItemIsRunnableCaliber(item) {
		// Scheduling demand is the measured runnable lane in full. TotalMs may
		// include running/sleep and is never a fallback for this caliber.
		return item.RunnableMs
	}
	if rootCauseItemIsDStateOrIOCaliber(item) {
		// D-state and I/O are one blocking family and participate by the exact
		// typed union-of-lanes scalar carried by the row.
		return item.DStateMs + item.IOWaitMs
	}
	if item.Type == "fragmented_sleep_wait" {
		// Non-periodic S-sleep is a dependency symptom/context, not one of the
		// closed root-cause participation calibers. Periodic sources returned
		// above through VS-1; D/IO has its own exact branch.
		return 0
	}
	switch strings.TrimSpace(item.Type) {
	case "sleep_wait", "missing_wakeup", "unknown_state", "state_churn",
		"trace_span", "io_pressure", "cpu_pressure", "supply_pressure",
		"cpu_frequency_limit", "sched_stat_accounting", "irq_activity", "ipi_activity":
		return 0
	}
	if item.EffectiveImpactMs > 0 {
		return item.EffectiveImpactMs
	}
	return rootCauseCumulativeImpactMs(item)
}

func rootCauseItemIsRunningCaliber(item RootCauseRankItem) bool {
	switch strings.TrimSpace(item.Type) {
	case "running", "fragmented_running":
		return true
	case "low_frequency", "compute_supply":
		return item.DominantState == string(StateRunning)
	default:
		return item.DominantState == string(StateRunning) && item.RunningMs > 0
	}
}

func rootCauseItemIsRunnableCaliber(item RootCauseRankItem) bool {
	switch strings.TrimSpace(item.Type) {
	case "runnable_wait", "scheduler_latency", "fragmented_runnable_wait":
		return true
	case "low_frequency", "compute_supply", "cpu_affinity_or_cpuset":
		return item.DominantState == string(StateRunnable)
	default:
		return item.DominantState == string(StateRunnable) && item.RunnableMs > 0
	}
}

func rootCauseItemIsDStateOrIOCaliber(item RootCauseRankItem) bool {
	switch strings.TrimSpace(item.Type) {
	case "io_wait", "d_state_or_io_wait", "fragmented_d_state_or_io_wait":
		return true
	default:
		return dominantStateIsDStateOrIOWait(item.DominantState) && (item.DStateMs > 0 || item.IOWaitMs > 0)
	}
}

func rootCauseFragmentedStateEffectiveImpactMs(item RootCauseRankItem) float64 {
	switch strings.TrimSpace(item.Type) {
	case "fragmented_running":
		// StateChurn has no CAP/supply-fold basis, so absence is authoritative.
		return 0
	case "fragmented_runnable_wait":
		return item.RunnableMs
	case "fragmented_d_state_or_io_wait":
		return item.DStateMs + item.IOWaitMs
	case "fragmented_sleep_wait":
		return 0
	default:
		return 0
	}
}

// rootCauseRankScoreBasisMs is the Score-channel basis (SEM-LEAD 复核 P1-1,
// §29.22 修向(a), 2026-07-10): the published effective attribution, lifted to
// the engine-internal semantic boost when present. Score is a SECONDARY sort
// key on the on-chain tier (the published-effective comparison decides
// first), so the boost acts exactly as a same-effective TIE-BREAK — a noisy
// heuristic driving a soft decision, never the published ordinal
// (精确信号硬门/嘈声信号软引导 red line).
func rootCauseRankScoreBasisMs(item RootCauseRankItem) float64 {
	if item.RankSortBoostedEffectiveMs > 0 {
		return item.RankSortBoostedEffectiveMs
	}
	return rootCauseEffectiveImpactMs(item)
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
	// RN-16 (§7.9): every rank row passes the causal-token registry guard —
	// unregistered tokens and aggregate-only tokens carrying a concrete
	// thread subject panic under test / WARN in prod.
	assertCausalTokenRow(typ, thread, "root_cause_rank")
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

// stampWindowStatsRankQueryWindow (§21.1 CWD-2 ②, cmp_01 C7 witness,
// real_trace_campaign_20260705.md): every rank row whose typed Source names
// the window_stats family carries the query-window identity the backing
// summary was computed over, so the tool observation face can emit the
// row-level selected_window note even when the result envelope carries no
// window of its own. Identity carriage only — the rank/score/impact lanes
// never read the stamp (rank 排序/权重/Value 零触碰); an unbounded stats
// window stamps nothing (absence never guesses a window base). Idempotent:
// build and enrich both call it over the same window.
func stampWindowStatsRankQueryWindow(items []RootCauseRankItem, window TimeWindow) {
	if window.EndTs <= window.StartTs {
		return
	}
	for i := range items {
		if !strings.HasPrefix(items[i].Source, "window_stats") {
			continue
		}
		items[i].StatsWindowStartTs = window.StartTs
		items[i].StatsWindowEndTs = window.EndTs
	}
}

// WakeupCausalImpactEffectiveImpactMs is the PTV5 Q1 effective-attribution
// value of one causal-impact row — EXACTLY the rank lane's published
// semantics (rootCauseItemFromCausalImpact + rootCauseEffectiveImpactMs),
// exported so the typed-note emission in internal/tool publishes the SAME
// number the rank rows publish (复核 Med 真镜像, 2026-07-06; the former
// tool-side re-implementation drifted: plain rows fell to DominantImpactMs
// while the rank lane backfills cumulative=TotalMs, and a gated-0 inversion
// row — the rank assignment has no >0 guard — also lands on the TotalMs
// backfill). Branches:
//   - periodic → the VS-1 discounted attribution (authoritative at 0);
//   - inversion candidate with gated>0 → the R5d gated composite;
//   - running-dominant (non-inversion) → the ELIMINABLE supply-fold deficit
//     (SupplyFoldDeficitMs; §20.2 user ruling 2026-07-07, authoritative at 0
//     — 能算作影响的永远是折算后能消除的那部分);
//   - otherwise → TotalMs, then the per-state total, then the row's own
//     blocking impact (gated for inversion rows) — the rootCause cumulative
//     backfill chain verbatim.
//
// Pinned two-lane-equal by TestWakeupCausalImpactEffectiveMirrorsRankLane.
func WakeupCausalImpactEffectiveImpactMs(impact WakeupCausalImpact) float64 {
	if impact.PeriodicSource {
		return impact.EffectivePeriodicImpactMs
	}
	if impact.PriorityInversionCandidate {
		return impact.PriorityInversionGatedMs
	}
	switch impact.DominantState {
	case string(StateRunning):
		// §20.2 (user ruling 2026-07-07, OVERTURNS the §20.1甲 raw-participates
		// side clause — EVOLUTION, not regression): a NON-inversion running
		// segment's ATTRIBUTION is its ELIMINABLE supply-fold deficit — never
		// the raw wall clock, never the folded ideal. deficit==0 with a
		// computed fold IS the §7.10 fourth branch stated correctly:
		// full-frequency/full-core running is real workload → attribution ≈ 0
		// → chain context, not a root cause. Frequency data missing (fold
		// gate unmet → nil basis, or an unknown-frequency basis) folds the
		// deficit to 0 the same conservative way (§7.10 不伪造) — the row
		// keeps its raw DISPLAY facts (cumulative / state split / projection
		// bar) but un-folded raw must never drive ranking (precise signals
		// for hard gates; noisy raw stays soft display). Authoritative
		// INCLUDING at 0 — the TotalMs backfill below must not resurrect it.
		// (Inversion running segments already returned above: their running
		// share is GatedRunningDeficitMs by construction — same principle.)
		return impact.SupplyFoldDeficitMs
	case string(StateRunnable):
		// Runnable is scheduling demand and participates in full. TotalMs may
		// include unrelated running/sleep intervals and is forbidden here.
		return impact.RunnableMs
	case string(StateDSleep), string(StateIOWait):
		// D/IO is one blocking caliber; both typed lanes participate in full.
		return impact.DStateMs + impact.IOWaitMs
	case string(StateSSleep):
		// Ordinary S-sleep is the dependency symptom being drilled through. Only
		// a proved periodic source (handled above) converts it into a VS-1
		// effective contender; missing wakeup remains context/data quality.
		return 0
	default:
		// stopped/dead/unknown own no causal-duration lane.
		return 0
	}
}

// WakeupCausalAggregateEffectiveImpactMs is the aggregate face of the same
// closed participation matrix. It is exported so rank construction and tool
// observations cannot drift on authoritative zeros (plain running without a
// CAP deficit, ordinary sleep, unknown state) or resurrect raw totals.
func WakeupCausalAggregateEffectiveImpactMs(aggregate WakeupCausalAggregate) float64 {
	if aggregate.PeriodicSource {
		return aggregate.EffectivePeriodicImpactMs
	}
	if WakeupCausalAggregateInversionTyped(aggregate) {
		return aggregate.PriorityInversionGatedMs
	}
	switch aggregate.DominantState {
	case string(StateRunning):
		return aggregate.SupplyFoldDeficitMs
	case string(StateRunnable):
		return aggregate.RunnableMs
	case string(StateDSleep), string(StateIOWait):
		return aggregate.DStateMs + aggregate.IOWaitMs
	default:
		return 0
	}
}

// RootCauseRankItemEffectiveImpactMs exports the single ranking-caliber
// authority to tool/report layers. Consumers must not reimplement fallback
// rules: authoritative zeros (running without CAP deficit, non-periodic
// sleep, context-only inversion, unknown state) are load-bearing facts.
func RootCauseRankItemEffectiveImpactMs(item RootCauseRankItem) float64 {
	return rootCauseEffectiveImpactMs(item)
}

func rootCauseItemFromCausalImpact(impact WakeupCausalImpact) RootCauseRankItem {
	effectiveMs := WakeupCausalImpactEffectiveImpactMs(impact)
	typ := causalImpactRootType(impact)
	if impact.PriorityInversionCandidate {
		typ = "priority_inversion_candidate"
	}
	conf := 0.86
	if impact.PriorityInversionCandidate {
		conf = 0.91
	}
	impactMs := causalImpactBlockingMs(impact)
	if impact.PriorityInversionCandidate {
		// R5d: an inversion row publishes and ranks with its gated impact,
		// not the dependency's whole dominant/blocked duration.
		impactMs = impact.PriorityInversionGatedMs
	}
	item := rootCauseItem(typ, impact.Thread, impactMs, conf, impact.LineStart, impact.LineEnd, "wakeup_chain.causal_impacts", impact.Summary)
	item.EffectiveImpactMs = effectiveMs
	if impact.PriorityInversionCandidate {
		// §7.30.3 D3: the composite's composition travels with the row so the
		// renderer can split it instead of claiming a single state.
		item.GatedRunnableMs = impact.GatedRunnableMs
		item.GatedRunningDeficitMs = impact.GatedRunningDeficitMs
		item.GatedCapabilitySource = impact.GatedCapabilitySource
		item.GatedClusterTopology = impact.GatedClusterTopology
	} else if impact.DominantState == string(StateRunning) {
		// §20.2 (2026-07-07, overturns §20.1甲 side clause — EVOLUTION): the
		// merged non-inversion RUNNING row's ATTRIBUTION channels (effective /
		// sort / Score) carry its ELIMINABLE supply-fold deficit — 0 when the
		// fold found none or frequency data was missing (§7.10 不伪造), and
		// authoritative at 0 (rootCauseEffectiveImpactMs type=="running"
		// branch). Raw stays on the DISPLAY channels only: ImpactMs /
		// ProjectedImpactMs (projection bar) keep causalImpactBlockingMs and
		// CumulativeImpactMs keeps TotalMs below. Mirror of
		// WakeupCausalImpactEffectiveImpactMs — pinned two-lane-equal by
		// TestWakeupCausalImpactEffectiveMirrorsRankLane. (PeriodicSource is
		// only ever stamped on sleep-dominant rows, structurally exclusive.)
		item.EffectiveImpactMs = effectiveMs
	}
	item.Causality = "on_wakeup_chain"
	item.ChainRelevance = "on_chain"
	item.ChainDepth = impact.ChainDepth
	item.ChainBranch = impact.ChainBranch
	item.CumulativeImpactMs = impact.TotalMs
	if impact.PriorityInversionCandidate && impact.DominantState == string(StateRunning) {
		// §20.1 ruling ①甲 (raw 墙钟保留): the merged running-dominant
		// inversion row keeps the dead twin's raw caliber visible — the raw
		// RUNNING wall clock rides the cumulative face (q6: gated 37.410
		// ranks, raw 58.919 stays cumulative) while the state fields keep the
		// full split. TotalMs (which includes the segment's own sleep) never
		// was the twin's caliber and would misstate the raw ruler here.
		item.CumulativeImpactMs = impact.DominantImpactMs
	}
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
	item.Score = effectiveMs * conf * rootCauseScoreWeightChainImpact
	if impact.PeriodicSource {
		// VS-1 (§7.8): a periodic source's sleep-dominant occurrence ranks and
		// scores by its DISCOUNTED attribution (runnable in full + lateness) —
		// in-period sleep is cadence, not cause. Raw ImpactMs/CumulativeImpactMs
		// stay untouched so the window projection remains lossless.
		// (PeriodicSource is only ever stamped on sleep-dominant rows, so it is
		// structurally exclusive with the inversion override above.)
		item.PeriodicSource = true
		item.DetectedPeriodMs = impact.DetectedPeriodMs
		item.LatenessMs = impact.LatenessMs
		item.EffectiveImpactMs = effectiveMs
		item.Score = effectiveMs * conf * rootCauseScoreWeightChainImpact
	}
	// VS-2 (§7.10) fold accounting travels with the rank row for the display
	// decision table. §20.2 EVOLUTION (2026-07-07): for non-inversion RUNNING
	// rows the deficit now IS the attribution (effective/sort/Score above) —
	// the former "deficit 不参赛" clause is overturned for that lane; ideal
	// remains display-only everywhere (ideal 零 impact 化).
	if impact.SupplyFoldBasis != nil {
		basis := *impact.SupplyFoldBasis
		item.SupplyFoldDeficitMs = impact.SupplyFoldDeficitMs
		item.SupplyFoldIdealMs = impact.SupplyFoldIdealMs
		item.SupplyFoldBasis = &basis
	}
	return item
}

func rootCauseItemFromCausalAggregate(aggregate WakeupCausalAggregate) RootCauseRankItem {
	inversionTyped := WakeupCausalAggregateInversionTyped(aggregate)
	effectiveMs := WakeupCausalAggregateEffectiveImpactMs(aggregate)
	typ := aggregateRootCauseType(aggregate)
	if inversionTyped {
		typ = "priority_inversion_candidate"
	}
	conf := 0.82
	if inversionTyped {
		conf = 0.88
	}
	impactMs := aggregateBlockingMs(aggregate)
	if inversionTyped {
		// R5d aggregate half (§20 E-Gap②, 2026-07-07): an inversion-typed
		// aggregate row publishes and ranks with its gated caliber — the same
		// R5d rule the per-occurrence lane has carried since §7.30.1; the raw
		// blocking magnitude stays on the cumulative/state faces. The gated
		// value's cross-occurrence additivity/fallback is decided at
		// aggregation time (applyAggregateGatedInversion — sum only over
		// disjoint occurrence windows, honest MAX otherwise).
		impactMs = aggregate.PriorityInversionGatedMs
	}
	item := rootCauseItem(typ, aggregate.Thread, impactMs, conf, aggregate.LineStart, aggregate.LineEnd, "wakeup_chain.aggregated_impacts", aggregate.Summary)
	item.EffectiveImpactMs = effectiveMs
	if inversionTyped {
		// §7.30.3 D3 mirror: composition travels with the row.
		item.GatedRunnableMs = aggregate.GatedRunnableMs
		item.GatedRunningDeficitMs = aggregate.GatedRunningDeficitMs
		item.GatedCapabilitySource = aggregate.GatedCapabilitySource
		item.GatedClusterTopology = aggregate.GatedClusterTopology
	} else if aggregate.DominantState == string(StateRunning) {
		// §20.2 mirror (2026-07-07, overturns §20.1甲 side clause): a
		// non-inversion running-dominant aggregate's attribution channels
		// carry the ELIMINABLE member-deficit value (VS-2 fold sum over the
		// folded members; 0 when no member folded or frequency data was
		// missing — authoritative at 0 via the type=="running" branch in
		// rootCauseEffectiveImpactMs). Raw ΣRunning stays display-only
		// (ImpactMs/ProjectedImpactMs bar + TotalMs cumulative below). The
		// The typed inversion gate above is authoritative. The raw aggregate
		// flag is an audit census bit ("some member was flagged") and cannot
		// suppress a valid plain-running CAP deficit when no typed gated
		// inversion survived aggregation.
		item.EffectiveImpactMs = effectiveMs
	}
	item.Causality = "on_wakeup_chain"
	item.ChainRelevance = "on_chain"
	item.ChainDepth = aggregate.ChainDepth
	item.ChainBranch = aggregate.ChainBranch
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
	item.Score = effectiveMs * conf * rootCauseScoreWeightChainAggregate
	if aggregate.PeriodicSource {
		// VS-1 (§7.8): same discounted ranking as the per-occurrence face —
		// see rootCauseItemFromCausalImpact. Raw impact/cumulative unchanged.
		item.PeriodicSource = true
		item.DetectedPeriodMs = aggregate.DetectedPeriodMs
		item.LatenessMs = aggregate.LatenessMs
		item.EffectiveImpactMs = effectiveMs
		item.Score = effectiveMs * conf * rootCauseScoreWeightChainAggregate
	}
	// VS-2 (§7.10): same mirror as rootCauseItemFromCausalImpact — display
	// decision-table input only, never a rank/score participant.
	if aggregate.SupplyFoldBasis != nil {
		basis := *aggregate.SupplyFoldBasis
		item.SupplyFoldDeficitMs = aggregate.SupplyFoldDeficitMs
		item.SupplyFoldIdealMs = aggregate.SupplyFoldIdealMs
		item.SupplyFoldBasis = &basis
	}
	return item
}

// rootCauseItemFromLockContentionCandidate (Q4-A 修1, ledger §12.1/§12.3-5)
// turns one FOLDED lock-contention candidate (collectBlockingSpanRows) into a
// root_cause_rank row: the row SUBJECT is the parsed lock HOLDER (falls back
// to the blocked waiter when the payload carried no resolvable owner) and the
// impact is the MEASURED contention duration. Chain relevance couples through
// either side (the span lives on the waiter's critical path; see the
// contention-peer lane in enrichRootCauseItemsWithChainContext), while DIRECT
// on-chain admission stays gated on the precise typed pair BlockingKind +
// resolved counterpart (rootCauseItemCanBeDirectOnChain) — never on span-name
// text. monitor_contention / lock_contention remain BlockingKind refinements
// and are NOT promoted to row tokens (registry ruling; blocking_span
// RowToken=true).
func rootCauseItemFromLockContentionCandidate(q Query, chainThreads map[int]bool, hasCausalChain bool, row blockingSpanRow) RootCauseRankItem {
	cand := row.cand
	waiter := cand.Thread
	holder := cand.Peer
	holderResolved := threadRefResolved(holder)
	subject := waiter
	if holderResolved {
		subject = holder
	}
	onChain := threadInSet(chainThreads, waiter) || threadInSet(chainThreads, holder)
	suffix := lockContentionSummarySuffix(lockContentionInfo{Kind: cand.BlockingKind, Owner: holder, Waiters: cand.Waiters, HolderSite: cand.HolderSite, BlockingFromSite: cand.BlockingFromSite})
	summary := fmt.Sprintf("%s blocked %.3fms on lock contention with unresolved owner%s", threadLabel(waiter), cand.DurationMs, suffix)
	if holderResolved {
		summary = fmt.Sprintf("lock holder %s blocked %s for %.3fms%s", threadLabel(holder), threadLabel(waiter), cand.DurationMs, suffix)
		if len(cand.HolderHandoff) >= 2 {
			// P0-E 锁车道修2: the payload recorded a hand-over — the subject
			// is the FINAL holder; the whole-span figure is the waiter's wait
			// envelope, never one thread's tenure.
			summary = fmt.Sprintf("final lock holder %s (hand-over chain %s) last held the lock %s waited %.3fms on%s", threadLabel(holder), strings.Join(cand.HolderHandoff, " --> "), threadLabel(waiter), cand.DurationMs, suffix)
		}
	}
	if cand.ActualDurationMs > 0 {
		// F-1 (ledger §23.2): window-clipped contention — the rank summary
		// discloses the dual basis exactly like the semantic lane does.
		summary += fmt.Sprintf("; window-clipped, actual_span=%.3fms window=%.6f..%.6f", cand.ActualDurationMs, cand.ActualStartTs, cand.ActualEndTs)
	}
	// P0-E2a: the rank confidence tracks the folded candidate's confidence, so
	// the wakeup-edge holder-source demotion (0.62) flows into rank Score = impact
	// × conf × weight — an inferred holder never scores as a payload-direct one.
	rowConfidence := cand.Confidence
	if rowConfidence <= 0 {
		rowConfidence = 0.72
	}
	item := rootCauseItem("blocking_span", subject, backgroundImpactMs(q, cand.DurationMs, hasCausalChain, onChain), rowConfidence, cand.LineStart, cand.LineEnd, "window_stats.trace_spans.lock_contention", summary)
	item.CumulativeImpactMs = cand.DurationMs
	item.Causality = causalityLabel(hasCausalChain, onChain)
	item.StartTs = cand.StartTs
	item.EndTs = cand.EndTs
	if cand.ActualDurationMs > 0 {
		// F-1: the physical extent rides the SAME actual_* rank lanes the
		// semantic rows use — the ⚠实际 display marker and the
		// projected/actual note pair engage without a second implementation.
		item.ActualStartTs = cand.ActualStartTs
		item.ActualEndTs = cand.ActualEndTs
		item.ActualImpactMs = cand.ActualDurationMs
		item.ActualTotalMs = cand.ActualDurationMs
	}
	item.SpanName = row.spanName
	item.BlockingKind = cand.BlockingKind
	item.HolderSite = cand.HolderSite
	// BLOCKFROM (§27.4 G13): the waiter-side blocking call site rides the rank
	// row verbatim (typed 等待点 face; display half is DISP-2's).
	item.BlockingFromSite = cand.BlockingFromSite
	// P0-E2a: carry the typed holder-source origin and the phantom payload tid
	// (if the wakeup-edge fallback fired) onto the rank row.
	item.HolderSource = cand.HolderSource
	item.OwnerTidRaw = cand.OwnerTidRaw
	// LCK-2 (§18.E/§18.E.1): the ②×③ identity-unification declaration and the
	// process-level ns-span identity ride the rank row verbatim.
	item.HolderNsUnification = cand.HolderNsUnification
	item.HolderHostProcess = cand.HolderHostProcess
	// P0-E 锁车道修2: the hand-off witness and the self-contradiction
	// demotion witness ride the rank row verbatim (display disclosure faces).
	item.HolderHandoff = append([]string(nil), cand.HolderHandoff...)
	item.HolderSelfContradiction = cand.HolderSelfContradiction
	if holderResolved {
		// BlockingPeer = the contention counterpart of the row subject: the
		// blocked waiter. Deliberately left EMPTY when the holder is
		// unresolved (the subject stays the waiter), so the typed admission
		// pair (BlockingKind + resolved BlockingPeer) reads false and the row
		// can never take a direct on-chain slot unresolved (§12.3 未解析不准入).
		item.BlockingPeer = waiter
		// BLK §15.C: the subject IS the holder here — the display face must read
		// this row as a HOLD ("持锁 X ms 阻塞了 waiter"), never as the reversed
		// lock-WAIT the waiter-subject critical_blocking row already carries for
		// the SAME physical span. Only set when the holder resolved (subject
		// swapped to the holder); the unresolved shape keeps the waiter subject
		// and this stays false.
		item.SubjectIsLockHolder = true
	}
	// §20.1 ruling ② (2026-07-07): re-derive Score AFTER the typed contention
	// fields are set — a RESOLVED lock row weighs 1.35 with the
	// priority_inversion family (decisive-evidence class) instead of the 0.8
	// type-table default rootCauseItem used at construction, so a measured
	// lock hold no longer scores below a generic trace_span (0.9) in the
	// Score-only (non-chain-aware) sort tiers. Unresolved rows keep 0.8.
	item.Score = item.ImpactMs * item.Confidence * rootCauseItemScoreWeight(item)
	return item
}

// rootCauseTypeIsPriorityInversion (P2-2 per-CLASS): the full inversion
// row-type family — the causal-impact/aggregate lane (priority_inversion_
// candidate) AND the RunnableTop lane stamped by
// applyRunnableTopPriorityInversion (priority_inversion_runnable_wait).
//
// UXG-1 M4 (2026-07-12): THE engine-side family single point — the literal
// token pair may appear together only here and in the causal-token registry
// rows (source-scan pinned by
// internal/tool/uxg1_family_predicate_tripwire_test.go; the display-side
// single point runtimeTracePriorityInversionCandidateType is interlocked with
// this one through the exported wrapper below).
func rootCauseTypeIsPriorityInversion(typ string) bool {
	switch typ {
	case "priority_inversion_candidate", "priority_inversion_runnable_wait":
		return true
	default:
		return false
	}
}

// RootCauseTypeIsPriorityInversion exposes the inversion row-type family
// predicate for cross-package consumers and the M4 interlock pin. No token
// literals here — the family bytes live only at the single point above.
func RootCauseTypeIsPriorityInversion(typ string) bool {
	return rootCauseTypeIsPriorityInversion(typ)
}

// demoteLockDominatedInversionCandidates (Q4-D, ledger §12.1/§12.2 P0-E) —
// inversion cross-check against resolved lock evidence: when the TARGET's own
// window carries a structured monitor/lock contention span (BlockingKind
// parsed ∧ owner resolved) whose interval COVERS an inversion row's whole
// wait interval, the wait is lock-holder dominated — the priority-inversion
// reading demotes to an annotation (typed flag + summary note; the
// observation itself is preserved, 不删观测). Applies to the whole inversion
// row-type family (P2-2, rootCauseTypeIsPriorityInversion). Every gate input
// is typed: parsed kind, resolved owner ThreadRef, interval containment —
// never span-name text or waker/wakee prio prose. q4 witness:
// RxComputationT's inversion candidate vs the target's 112.223ms
// monitor_contention span.
func demoteLockDominatedInversionCandidates(chain ChainResult, stats WindowStats, items []RootCauseRankItem) {
	if len(items) == 0 || chain.Target.PID <= 0 {
		return
	}
	type contentionCover struct {
		startTs, endTs float64
		kind           string
		owner          ThreadRef
	}
	var covers []contentionCover
	for _, span := range stats.TraceSpans {
		if span.Thread.PID != chain.Target.PID || span.EndTs <= span.StartTs {
			continue
		}
		cand, ok := blockingSpanCandidateFromTraceSpan(span)
		if !ok || cand.BlockingKind == "" || !threadRefResolved(cand.Peer) {
			continue
		}
		covers = append(covers, contentionCover{startTs: span.StartTs, endTs: span.EndTs, kind: cand.BlockingKind, owner: cand.Peer})
	}
	if len(covers) == 0 {
		return
	}
	for i := range items {
		item := &items[i]
		if !rootCauseTypeIsPriorityInversion(item.Type) || item.StartTs <= 0 || item.EndTs <= item.StartTs {
			continue
		}
		for _, cover := range covers {
			if cover.startTs > item.StartTs || cover.endTs < item.EndTs {
				continue
			}
			item.PriorityInversionLockDominated = true
			item.Summary = appendRootCauseSummaryDetail(item.Summary,
				fmt.Sprintf("inversion candidate demoted to note: resolved %s held by %s covers this wait interval %.6f..%.6f — 锁等待主导 (lock-wait dominated)",
					cover.kind, threadLabel(cover.owner), item.StartTs, item.EndTs))
			break
		}
	}
}

type semanticTraceSpanProjection struct {
	StartTs       float64
	EndTs         float64
	ImpactMs      float64
	DominantState string
	ChainDepth    int
	// ChainBranch mirrors the overlapped chain node/impact's branch identity
	// (P0-E CHAIN-PATH, ledger §22.1) — 0 when no overlap won.
	ChainBranch int
	OnChain     bool
}

func rootCauseItemFromSemanticTraceSpan(q Query, chain ChainResult, span TraceSpanSummary, hasCausalChain bool) (RootCauseRankItem, bool) {
	work, ok := traceSpanSemanticWorkClass(span.Name)
	if !ok || span.DurationMs <= 0 || span.EndTs <= span.StartTs {
		return RootCauseRankItem{}, false
	}
	projection, dominantStateImpactMs := semanticTraceSpanProjectionForRootCause(q, chain, span)
	if projection.ImpactMs <= 0 {
		return RootCauseRankItem{}, false
	}
	// SEM-LEAD (§29.7-2 ②, ledger real_trace_campaign_20260705.md, 2026-07-10).
	// EVOLUTION RECORD: the deterministic hidden-cost boost (ImpactMultiplier ×
	// window projection) used to publish AS EffectiveImpactMs and leak onto
	// every consumer face (792-textup witness: 有效归因 214.561ms 表值 =
	// 102.172 × 2.10, plus the semantic_multiplier=/hidden_cost_boost= internal
	// tokens escaping into answer prose via this Summary). The published
	// effective attribution is now the REAL window projection (家族真实合计);
	// the boost stays ENGINE-INTERNAL on the sort/score channel
	// (RankSortBoostedEffectiveMs → rootCauseEffectiveImpactMs / Score), so the
	// row's competitive strength is unchanged while no boosted ms ever leaves
	// the engine as a value or a token.
	sortBoostedMs := semanticTraceSpanEffectiveImpactMs(work, projection, span)
	// DCS E4: a boundary-straddling span was minted from its window-clipped
	// extent; the actual_* lanes carry the physical B/E extent when present.
	actualStartTs, actualEndTs, actualMs := span.StartTs, span.EndTs, span.DurationMs
	if span.ActualDurationMs > 0 {
		actualStartTs, actualEndTs, actualMs = span.ActualStartTs, span.ActualEndTs, span.ActualDurationMs
	}
	summary := fmt.Sprintf("%s span %q overlapped %s for %.3fms; effective_impact=%.3fms; actual_span=%.3fms window=%.6f..%.6f",
		work.Label, span.Name, semanticTraceSpanProjectionScope(projection, hasCausalChain), projection.ImpactMs, projection.ImpactMs, actualMs, actualStartTs, actualEndTs)
	if projection.DominantState != "" {
		summary = fmt.Sprintf("%s; overlapped_chain_state=%s", summary, projection.DominantState)
	}
	item := rootCauseItem(work.RootCauseType, span.Thread, projection.ImpactMs, work.Confidence, span.StartLine, span.EndLine, "window_stats.trace_spans.semantic", summary)
	item.PhysicalSourcePath = span.SourcePath
	item.StartTs = projection.StartTs
	item.EndTs = projection.EndTs
	item.ActualStartTs = actualStartTs
	item.ActualEndTs = actualEndTs
	item.ActualImpactMs = actualMs
	item.ActualTotalMs = actualMs
	item.ProjectedImpactMs = projection.ImpactMs
	item.CumulativeImpactMs = projection.ImpactMs
	// SEM-LEAD (§29.7-2 ②): published effective = real projection; boost stays
	// on the internal sort channel only.
	item.EffectiveImpactMs = projection.ImpactMs
	if sortBoostedMs > projection.ImpactMs {
		item.RankSortBoostedEffectiveMs = sortBoostedMs
	}
	item.SpanName = span.Name
	item.SpanKind = span.Kind
	item.SpanCategory = firstNonEmpty(span.Category, work.Category)
	item.SpanSubcategory = firstNonEmpty(span.Subcategory, work.Subcategory)
	item.SemanticClass = work.SemanticClass
	item.DominantState = projection.DominantState
	item.ChainDepth = projection.ChainDepth
	item.ChainBranch = projection.ChainBranch
	if projection.OnChain {
		item.Causality = "on_wakeup_chain"
		item.ChainRelevance = "on_chain"
	}
	applySemanticTraceSpanState(&item, projection.DominantState, dominantStateImpactMs)
	// F6 (§20.2 absorption): item-aware weight helper — behavior-identical
	// here (semantic work tokens are never blocking_span). SEM-LEAD P1-1: the
	// Score consumes the boosted basis as a same-effective tie-break only —
	// the on-chain ordinal key is the published effective.
	item.Score = rootCauseRankScoreBasisMs(item) * item.Confidence * rootCauseItemScoreWeight(item)
	return item, true
}

func semanticTraceSpanEffectiveImpactMs(work traceSpanSemanticWork, projection semanticTraceSpanProjection, span TraceSpanSummary) float64 {
	impact := projection.ImpactMs
	if impact <= 0 {
		return 0
	}
	if !projection.OnChain {
		return impact
	}
	multiplier := work.ImpactMultiplier
	if multiplier <= 0 {
		multiplier = 1
	}
	effective := impact * multiplier
	if work.MinOnChainImpactMs > 0 && span.DurationMs >= 0.5 && effective < work.MinOnChainImpactMs {
		effective = work.MinOnChainImpactMs
	}
	capMs := maxFloat(span.DurationMs*4, impact*4)
	if work.MinOnChainImpactMs > capMs {
		capMs = work.MinOnChainImpactMs
	}
	if capMs > 0 && effective > capMs {
		effective = capMs
	}
	if effective < impact {
		return impact
	}
	return effective
}

func semanticTraceSpanProjectionForRootCause(q Query, chain ChainResult, span TraceSpanSummary) (semanticTraceSpanProjection, float64) {
	start, end, ok := selectedTraceSpanWindow(q, span)
	if !ok {
		return semanticTraceSpanProjection{}, 0
	}
	if len(chain.Nodes) > 0 || len(chain.Edges) > 0 || len(chain.CausalImpacts) > 0 {
		// Single spans and semantic families share one exact interval algebra:
		// union(span members) ∩ union(same-thread chain windows). This prevents
		// the node and its mirrored causal-impact window (or two overlapping
		// branch nodes) from counting the same physical span twice.
		intersection := semanticTraceSpanChainIntersection(&chain, span.Thread, []foldInterval{{start: start, end: end}})
		if intersection.projection.ImpactMs > 0 {
			return intersection.projection, intersection.dominantStateImpactMs
		}
		// DCS E2 fall-through (ledger §23.1 rulings ①/②, 2026-07-08): a chain
		// exists but NO same-thread chain-node/impact window overlap — the row
		// used to degrade into a generic trace_span (and could never leave the
		// adjacent tier). It now mints the window-clipped typed projection with
		// OnChain=false: 窗内即可铸,链上/非链上由重叠谓词定道. The non-chain
		// lane (E1b) ranks it on the background composite board.
		return semanticTraceSpanProjection{
			StartTs:  start,
			EndTs:    end,
			ImpactMs: (end - start) * 1000,
		}, 0
	}
	// DCS E2 PID-gate removal (ledger §23.1 ruling ②; cmp_01 E2 witness: the
	// 83.893ms JIT span's host com.huawei.hwid is NOT the query target, and
	// the old q.PID gate erased the whole other-process compile family from
	// the rank pool). In-window is the only minting condition; the host
	// process never gates minting — the overlap predicate above decides the
	// lane, and the non-chain lane competes only on the background board.
	return semanticTraceSpanProjection{
		StartTs:  start,
		EndTs:    end,
		ImpactMs: (end - start) * 1000,
	}, 0
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
	if dominantStateIsDStateOrIOWait(item.DominantState) {
		return item.DStateMs + item.IOWaitMs
	}
	return item.DominantImpactMs
}

func actualAggregateBlockingMs(item WakeupCausalAggregate) float64 {
	if dominantStateIsDStateOrIOWait(item.DominantState) {
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
	// Inversion gating describes scheduling supply only: runnable in full plus
	// CAP-discounted running deficit. D/IO-dominant aggregates retain their
	// own full blocking caliber even when their members also carry a lower-
	// priority relation; retyping them as inversion would replace real D/IO
	// with the smaller scheduling-only gated scalar.
	case string(StateRunnable), string(StateRunning):
		return aggregateBlockingMs(item) > 0 && item.PriorityInversionGatedMs > 0
	default:
		return false
	}
}

// WakeupCausalAggregateInversionTyped (F1/F2, §20.2 absorption 2026-07-07) is
// the SINGLE typed determination of whether an aggregate row is
// inversion-TYPED on the rank face (type==priority_inversion_candidate).
// PriorityInversion alone (any member was a candidate, invCount>0) is a
// WEAKER claim: a sleep/D/IO-dominant aggregate with one inversion member
// must not light inversion labels or gated composition prose while its bar
// shows the raw dominant value (the F2 contradiction row). Exported so the
// internal/tool aggregate note face gates on exactly this determination —
// never on the raw flag.
func WakeupCausalAggregateInversionTyped(aggregate WakeupCausalAggregate) bool {
	return aggregate.PriorityInversion && aggregateRootCauseIsPrioritySensitive(aggregate)
}

func aggregateRootCauseType(item WakeupCausalAggregate) string {
	// Shared root-type authority (thread_state_universe.go) — byte-identical
	// twin of the causal-impact mapping.
	return rootTypeForDominantState(item.DominantState, WakeupCausalAggregateInversionTyped(item), item.IOWaitMs)
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
		// Q4-A 修1: a RESOLVED lock-contention rank row's subject is the
		// HOLDER, which may sit off-chain while the blocked WAITER
		// (BlockingPeer) is the on-chain thread — the contention couples to
		// the chain through the waiter's critical path. Take the better of
		// the two typed contexts; the gate stays the typed field pair.
		if items[i].Type == "blocking_span" && items[i].BlockingKind != "" && threadRefResolved(items[i].BlockingPeer) {
			peerCtx := chainContextForCandidate(chain, items[i].BlockingPeer, items[i].StartTs, items[i].EndTs)
			if chainRelevanceRank(peerCtx.relevance) < chainRelevanceRank(ctx.relevance) {
				ctx = peerCtx
			}
		}
		if ctx.relevance == "" {
			if hasChain {
				ctx.relevance = "background"
			} else {
				ctx.relevance = ""
			}
		}
		ctx = rootCauseChainContextForItem(items[i], ctx)
		// §20 E-Gap③ (2026-07-07, aligns the §17 tool-half demotion): an
		// aggregate-metric row (registry Subject==aggregate_only — the
		// subject IS the window/CPU-scoped metric, no thread) must never be
		// promoted to the adjacent tier on the strength of NOISY window
		// overlap with a chain node; beside a causal chain it is ALWAYS
		// background context. Precise typed registry signal (one enum read),
		// never row prose. Rows that may borrow a representative thread
		// (Subject==either, e.g. io_pressure) are not touched.
		if ctx.relevance == "adjacent" {
			if spec, ok := CausalTokenSpecFor(items[i].Type); ok && spec.Subject == CausalSubjectAggregateOnly {
				ctx.relevance = "background"
				ctx.overlapMs = 0
			}
		}
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
	// DCS 道别红线 (ledger §23.1 ruling ①, 2026-07-08): a semantic compile
	// span row's LANE is decided ONCE at mint time by the typed chain-node/
	// impact WINDOW-OVERLAP predicate. Thread membership alone (this ctx path)
	// must never flip a mint-time non-chain span onto the on-chain lane —
	// same-thread-without-overlap is exactly the shape the E2 fall-through
	// mints as non-chain. It keeps the chain-proximity context honestly as
	// adjacent (the huadong E21 precedent tier), which is still 非链上 for the
	// tier/mention double gate (rootCauseItemIsOnChain stays false).
	if ctx.relevance == "on_chain" && rootCauseItemIsSemanticSpanWork(item) && !rootCauseItemIsOnChain(item) {
		ctx.relevance = "adjacent"
		return ctx
	}
	if ctx.relevance == "on_chain" && rootCauseTypeIsResourceAttribution(item.Type) && item.EndTs <= item.StartTs {
		// Resource rows need their own typed interval to prove intersection
		// with a chain window. Same-TID membership alone is not causality.
		ctx.relevance = "adjacent"
		ctx.overlapMs = 0
		return ctx
	}
	if ctx.relevance != "on_chain" || rootCauseItemCanBeDirectOnChain(item) {
		return ctx
	}
	ctx.relevance = "adjacent"
	return ctx
}

func attributeOnChainResourceItemsToWakeupDependency(chain ChainResult, items []RootCauseRankItem) {
	if len(items) == 0 || len(chain.CausalImpacts) == 0 {
		return
	}
	impactByThread := strongestWakeupDependencyImpactByThread(chain)
	if len(impactByThread) == 0 {
		return
	}
	for i := range items {
		item := &items[i]
		if item == nil || !rootCauseTypeIsResourceAttribution(item.Type) || !rootCauseItemIsOnChain(*item) || item.Thread.PID <= 0 {
			continue
		}
		impact, ok := impactByThread[item.Thread.PID]
		if !ok || impact.TargetBlockedMs <= 0 {
			continue
		}
		overlap := item.OverlapMs
		if overlap <= 0 {
			overlap = windowOverlapMs(item.StartTs, item.EndTs, impact.Window.StartTs, impact.Window.EndTs)
		}
		// Q4-B (§12.3 ruling 2): the material gate reads the row's OWN measured
		// resource duration only — aggregate-window overlap is a NOISY signal
		// and may no longer admit a row by itself (it stays an advisory
		// note/OverlapMs input below).
		resourceMs := firstPositiveFloat(item.CumulativeImpactMs, item.ImpactMs, item.ProjectedImpactMs)
		if !onChainResourceAttributionIsMaterial(resourceMs, impact.TargetBlockedMs) {
			continue
		}
		// Q4-B (§12.3 ruling 2): the dependency window's target-blocked value
		// is INHERITED context — it rides the typed annotation field and the
		// summary note only. The ranking channels (EffectiveImpactMs /
		// TargetImpactMs / Score / sort keys) stay on the row's own
		// measurement: 承自只作注记,永不作硬排序键.
		item.InheritedTargetBlockedMs = impact.TargetBlockedMs
		if item.ChainDepth == 0 && impact.ChainDepth > 0 {
			item.ChainDepth = impact.ChainDepth
			// P0-E CHAIN-PATH: the coupled impact's branch travels with the
			// coupled depth — the display attach domain must never pair a
			// depth from one branch with the elected trunk of another.
			item.ChainBranch = impact.ChainBranch
		}
		if item.NearestChainThread.PID == 0 && strings.TrimSpace(item.NearestChainThread.Comm) == "" {
			item.NearestChainThread = impact.Thread
			item.NearestChainWindow = impact.Window
		}
		if item.OverlapMs <= 0 && overlap > 0 {
			item.OverlapMs = overlap
		}
		// state_churn precedent (§7.30 S1, rank re-scoring at :8677): re-derive
		// Score from the ranking channel after attribution so the published
		// score can never drift from the sort key again (the pre-Q4-B shape —
		// effective raised, score stale — was the q4 rank1-score-0.932
		// dissonance).
		// F6 (§20.2 absorption): item-aware weight helper — behavior-identical
		// here (resource-attribution tokens are never blocking_span).
		item.Score = rootCauseEffectiveImpactMs(*item) * item.Confidence * rootCauseItemScoreWeight(*item)
		item.Summary = appendRootCauseSummaryDetail(item.Summary,
			fmt.Sprintf("on-chain resource overlapped wakeup dependency window %.6f..%.6f; inherited target_blocked=%.3fms (annotation only, not a ranking key)",
				impact.Window.StartTs, impact.Window.EndTs, impact.TargetBlockedMs))
	}
}

func strongestWakeupDependencyImpactByThread(chain ChainResult) map[int]WakeupCausalImpact {
	out := map[int]WakeupCausalImpact{}
	for _, impact := range chain.CausalImpacts {
		if impact.Thread.PID <= 0 || impact.ChainDepth <= 0 || impact.TargetBlockedMs <= 0 {
			continue
		}
		existing, ok := out[impact.Thread.PID]
		if !ok || impact.TargetBlockedMs > existing.TargetBlockedMs ||
			(impact.TargetBlockedMs == existing.TargetBlockedMs && rootCauseEffectiveWakeupImpactMs(impact) > rootCauseEffectiveWakeupImpactMs(existing)) {
			out[impact.Thread.PID] = impact
		}
	}
	return out
}

func rootCauseEffectiveWakeupImpactMs(impact WakeupCausalImpact) float64 {
	return firstPositiveFloat(impact.ProjectedImpactMs, impact.DominantImpactMs, impact.TotalMs, impact.TargetBlockedMs)
}

func rootCauseTypeIsResourceAttribution(typ string) bool {
	switch strings.TrimSpace(typ) {
	case "io_latency", "io_burst_episode", "block_io_by_inode", "file_io_hot_inode", "page_cache_churn", "workqueue_activity", "dma_fence_activity":
		return true
	default:
		return false
	}
}

// onChainResourceAttributionIsMaterial (Q4-B, §12.3 ruling 2): the row's OWN
// measured resource duration must clear the floor. The pre-Q4-B shape took
// max(resourceMs, overlapMs) — an aggregate-window overlap (noisy signal)
// could admit a 1.1ms row into the wakeup-dependency attribution and, with the
// effective raise, hand it rank1 (q4 block_io 1.136ms case). Precise signals
// for hard gates: overlap now only feeds the advisory note/OverlapMs field.
func onChainResourceAttributionIsMaterial(resourceMs, targetBlockedMs float64) bool {
	if resourceMs <= 0 || targetBlockedMs <= 0 {
		return false
	}
	minMaterialMs := maxFloat(16, targetBlockedMs*0.35)
	return resourceMs >= minMaterialMs
}

func appendRootCauseSummaryDetail(summary, detail string) string {
	summary = strings.TrimSpace(summary)
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return summary
	}
	if summary == "" {
		return detail
	}
	if strings.Contains(summary, detail) {
		return summary
	}
	return summary + "; " + detail
}

func rootCauseItemCanBeDirectOnChain(item RootCauseRankItem) bool {
	// Q4-A 修1 (ledger §12.3-5): a blocking_span row is admissible as a DIRECT
	// on-chain cause only in its RESOLVED lock-contention form — typed
	// BlockingKind present AND the contention counterpart resolved. Unresolved
	// blocking spans keep the legacy demote-to-adjacent behavior (precise
	// signals for hard gates: two typed fields, never span-name text).
	if item.Type == "blocking_span" {
		return item.BlockingKind != "" && threadRefResolved(item.BlockingPeer)
	}
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
		"jit_compile", "class_verification", "shader_compile", "runtime_compile", "texture_upload", "gc_pause",
		"io_wait", "d_state_or_io_wait", "io_latency", "io_burst_episode", "block_io_by_inode", "file_io_hot_inode", "fragmented_d_state_or_io_wait",
		"workqueue_activity", "dma_fence_activity",
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
	hasCandidateWindow := end > start
	if start > end {
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
	sameThreadSeen := false
	for _, node := range chain.Nodes {
		if thread.PID > 0 && node.Thread.PID == thread.PID {
			sameThreadSeen = true
			ctx.nearest = node.Thread
			ctx.window = node.Window
			if hasCandidateWindow {
				overlap := windowOverlapMs(start, end, node.Window.StartTs, node.Window.EndTs)
				if overlap > 0 {
					ctx.relevance = "on_chain"
					ctx.overlapMs = maxFloat(ctx.overlapMs, overlap)
				}
			} else {
				ctx.relevance = "on_chain"
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
		// A bounded interval on a chain member that does not intersect any of
		// that member's chain windows is typed proximity, not causality. Keep
		// it adjacent: on_chain would fabricate an overlap, while background
		// would erase the useful same-thread relationship.
		if sameThreadSeen && hasCandidateWindow {
			ctx.relevance = "adjacent"
		} else {
			ctx.relevance = "background"
		}
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

// Rank-lane Score multipliers (§20 A-Gap④ Score 权重单源化, 2026-07-07):
// every Score channel reads THIS table — no construction site may hardcode a
// lane multiplier again (the pre-§20 shape carried three divergent hardcoded
// weights and the same segment's two rows could sort against their own
// effective ordering).
const (
	// rootCauseScoreWeightChainImpact — the wakeup_chain.causal_impacts lane
	// (per-occurrence on-chain rows, VS-1 periodic discount included).
	rootCauseScoreWeightChainImpact = 2.0
	// rootCauseScoreWeightChainAggregate — the wakeup_chain.aggregated_impacts
	// lane (multi-occurrence merged rows, VS-1 periodic discount included).
	rootCauseScoreWeightChainAggregate = 2.05
	// rootCauseWeightResolvedBlockingSpan (§20.1 ruling ②, 2026-07-07): a
	// blocking_span rank row whose lock counterpart is RESOLVED (typed pair
	// BlockingKind + resolved BlockingPeer — the same precise pair as the
	// direct-on-chain admission gate) carries decisive contention evidence of
	// the same class as the priority_inversion family and weighs 1.35 with
	// it. An UNRESOLVED blocking span keeps the type-table default (0.8,
	// deliberately below generic trace_span 0.9) so a span-name shell can
	// never outscore measured rows.
	rootCauseWeightResolvedBlockingSpan = 1.35
)

// rootCauseItemScoreWeight is the item-aware Score weight: typed-field
// refinements first (resolved blocking_span — §20.1 ruling ②), then the
// type-table weight. Score recomputation sites must use this, not
// rootCauseTypeWeight directly, whenever the item is in hand.
func rootCauseItemScoreWeight(item RootCauseRankItem) float64 {
	if item.Type == "blocking_span" && rootCauseItemHasResolvedBlockingPeer(item) {
		return rootCauseWeightResolvedBlockingSpan
	}
	return rootCauseTypeWeight(item.Type)
}

func rootCauseTypeWeight(typ string) float64 {
	// UXG-1 M4: the inversion row-type family reads its ONE predicate — the
	// literal token pair may appear only at the family single point
	// (rootCauseTypeIsPriorityInversion; source-scan pinned in
	// internal/tool/uxg1_family_predicate_tripwire_test.go).
	if rootCauseTypeIsPriorityInversion(typ) {
		return 1.35
	}
	switch typ {
	case "io_wait", "d_state_or_io_wait", "binder_wait":
		return 1.25
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
	case "jit_compile", "class_verification", "shader_compile", "runtime_compile", "texture_upload", "gc_pause":
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
	case "dma_fence_activity":
		return 0.82
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

// stateDrilldownRankWeight orders drilldown candidates: fragmented churn rows
// compete with their ranking-only composite, everything else with its physical
// duration. Never published as a duration value.
func stateDrilldownRankWeight(step StateDrilldownStep) float64 {
	if step.RankImpactMs > 0 {
		return step.RankImpactMs
	}
	return step.ImpactMs
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

// rootCauseItemIsSemanticSpanWork is the PRECISE typed identity of a semantic
// span-work rank row (DCS/SEM-LEAD): these closed-set type tokens are
// minted ONLY by rootCauseItemFromSemanticTraceSpan — never a name/substring
// heuristic.
func rootCauseItemIsSemanticSpanWork(item RootCauseRankItem) bool {
	switch item.Type {
	case "jit_compile", "class_verification", "shader_compile", "runtime_compile", "texture_upload", "gc_pause":
		return true
	default:
		return false
	}
}

// stampRootCauseRankAnalysisTargetSubject mints the SYM (§24.13 裁定一,
// real_trace_campaign_20260705.md, 2026-07-08) typed self-subject identity on
// every rank row: SubjectIsAnalysisTarget = the row's subject thread IS the
// analysis target the rank was computed for. The comparator is the engine's
// existing tid-first identity lane (sameThreadRef): PID equality decides
// whenever both sides carry a tid, and the comm arm engages only when a side
// has none — never a label/prose heuristic. An unresolved target (no PID, no
// comm — e.g. an untargeted window-stats rank) stamps nothing: absence never
// guesses, and every row keeps competing exactly as before. Idempotent: the
// enrich pass re-stamps the grown slice with the same target.
//
// SYM-2 (§24.17, 2026-07-08): the stamp stays FULL-population — it is the
// identity FACT (身份事实, audit face). Only the tier consequence narrowed:
// rootCauseItemIsTargetWaitSymptomType decides which stamped rows demote.
func stampRootCauseRankAnalysisTargetSubject(items []RootCauseRankItem, target ThreadRef) {
	if target.PID <= 0 && strings.TrimSpace(target.Comm) == "" {
		return
	}
	for i := range items {
		items[i].SubjectIsAnalysisTarget = sameThreadRef(items[i].Thread, target)
	}
}

// rootCauseItemIsTargetWaitSymptomType is the SYM-2 (ledger §24.17, user
// ruling 2026-07-08, revising the §24.13 裁定一 scope) typed 等待症状族 closed
// set: the target's own row demotes to the symptom lane ONLY when its cause
// token says the root cause lives at a counterpart/upstream — sleep-before-
// wakeup family (sleep_wait / fragmented_sleep_wait / missing_wakeup),
// binder wait-on-peer (binder_wait) and lock contention (blocking_span, the
// self-held-lock opendir_78 form and the waiter form alike). The closed set is
// the causal-token registry's OWN lane column (CausalLaneWakeupChain +
// CausalLaneLockContention — single semantic source, §7.2.1 red line), never a
// prose/substring heuristic. Everything else — the 自因可拆解族 (runnable /
// running / IO / D-state and any other registered family) — keeps competing:
// the target is itself an on-chain node (depth 0) and those states decompose
// into actionable system causes (调度压力 / 算力供给 / IO阻塞 / D状态), not a
// counterpart's behavior. Unregistered tokens compete (absence never demotes).
func rootCauseItemIsTargetWaitSymptomType(item RootCauseRankItem) bool {
	spec, ok := CausalTokenSpecFor(strings.TrimSpace(item.Type))
	if !ok {
		return false
	}
	switch spec.Lane {
	case CausalLaneWakeupChain, CausalLaneLockContention:
		return true
	default:
		return false
	}
}

// stampRunnableSelfBelowRTPreempted mints the SYM-2 (§24.17 R2, 2026-07-08)
// typed 「优先级低于RT」 disclosure on SELF runnable-family rank rows: the
// target's own runnable wait re-enters the election as a 调度压力候选, and
// when the typed scheduling data says the target's priority class is below RT
// (Harmony ohos_cfs) while an RT-class competitor's running overlapped this
// very wait on the same CPU, the display appends the disclosure tail. Every
// input is a precise typed signal: the SubjectIsAnalysisTarget stamp, the
// closed runnable token set, the RunnableContext priority class and the R5g
// SameCPUTopRunning displacement-overlap roster (window-total background load
// never participates). Non-Harmony flavors carry no RT class → nothing stamps
// (absence never guesses). Display/wording input only; rank/score/sort lanes
// never read the flag. Idempotent — the enrich pass re-stamps the grown slice.
func stampRunnableSelfBelowRTPreempted(items []RootCauseRankItem, contexts []RunnableContextSummary) {
	if len(contexts) == 0 {
		return
	}
	for i := range items {
		if !items[i].SubjectIsAnalysisTarget {
			continue
		}
		switch strings.TrimSpace(items[i].Type) {
		case "runnable_wait", "fragmented_runnable_wait", "scheduler_latency":
		default:
			continue
		}
		ctx, ok := runnableContextForThread(items[i].Thread, contexts)
		if !ok || ctx.PriorityClass != "ohos_cfs" {
			continue
		}
		for _, competitor := range ctx.SameCPUTopRunning {
			if competitor.PriorityClass == "ohos_rt" && competitor.DurationMs > 0 {
				items[i].RunnableBelowRTPreempted = true
				break
			}
		}
	}
}

// assignRootCauseRanksAndTiers assigns the per-window rank ordinals and tier
// words. It runs once per RootCauseRankResult (after the final sort and
// truncation, in both the build and the enrich pass — idempotent full
// recompute), so multi-window reports number every query window independently
// and contiguously.
//
// G9 engine renumbering (§27.3 + §28.1 user ruling 2026-07-09,
// real_trace_campaign_20260705.md; three faces one source). EVOLUTION RECORD:
// pre-G9 every row pre-consumed an ordinal (Rank=i+1) before the demotion
// arms ran and no face ever renumbered — a multi-window report's visible
// board read #6/#7/#12 while #1-#5 were silently eaten by rows the display
// never shows a seat for (demoted self-symptom rows, data blind spots;
// huadong_79/opendir_79 witness). Ordinals now go ONLY to rows carrying a
// rank-board display identity:
//   - competing election-ladder rows (自因四态 self-cause rows, on-chain
//     semantic span-work rows and VS-1
//     PeriodicSource discounted rows included — 复核 P1-2: a discounted row's
//     ordinal IS its competition identity) plus off-chain semantic background
//     rows keep incrementing;
//   - wait-symptom target_self_state rows and trace_gap data-blind-spot rows
//     carry Rank=0 — no seat, no hole.
//
// Election slots (tier words), BackgroundRank counting and every
// score/sort lane are UNTOUCHED — only the ordinal channel moved. The display
// badge gate is Rank>0, so Rank=0 rows drop their badge by construction.
//
// EVOLUTION RECORD (UXR-1, §29.36.2/§29.36.3 user rulings 2026-07-11, ledger
// real_trace_campaign_20260705.md — 3+1 通道终形): the former SINGLE rankPos
// ordinal space over every competing row is SPLIT per chain-relevance channel.
// Witness (real_trace_a5 4165): 根因排序#1/#2 landed on ▒ background rows
// whose own caliber note says 不计入链上归因 — a same-page contradiction, and
// the ▒ board mixes cross-thread cpu·ms aggregates with wall-clock rows (两把
// 尺红线的序数版). Ordinals now allocate per channel, keyed on the SAME typed
// chain-relevance single source the display stanzas read (chain_relevance /
// causality wire notes — rootCauseOrdinalChannel; never a prose judgment):
//   - channel 1 (链上 根因排序#N): on-chain rows only — the CLOSE-1 §29.30.1
//     valid-seat population keeps consuming exactly this ordinal;
//   - channel 2 (◇ 邻近影响#N): an INDEPENDENT ordinal space; ordering is the
//     published-eff sort order restricted to the channel (§29.22.1 序数键==
//     发布 eff preserved; adjacent rows are same-thread wall-clock caliber);
//   - channel 3 (▒ 背景): NO ordinal — caliber-grouped display; the §23.1
//     mention gate (BackgroundRank) stays an internal filter, never a chip;
//   - channel 4 (提及义务, §29.36.3): not an ordinal channel — an on-chain
//     semantic row either takes a channel-1 ordinal (TOP N) or renders via
//     the ✦ mention-floor lane with no ordinal (no silent-disappearance path).
//
// The wire carries NO new key: (rank, chain_relevance) jointly denote
// (channel, ordinal) — both already hard-consumer members of the causal_rank
// note family; display chip words fork on the relevance (禁裸 #N).
func assignRootCauseRanksAndTiers(items []RootCauseRankItem) {
	electionPos := 0
	backgroundPos := 0
	rankPos := 0
	adjacentPos := 0
	takeOrdinal := func(i int) {
		switch rootCauseOrdinalChannel(items[i]) {
		case rootCauseOrdinalChannelChain:
			rankPos++
			items[i].Rank = rankPos
		case rootCauseOrdinalChannelAdjacent:
			adjacentPos++
			items[i].Rank = adjacentPos
		default:
			// §29.36.2 channel 3: background rows publish NO ordinal — the
			// row stays visible with its tier/eff, but never wears a seat
			// number (口径混杂板不发序数; election slots untouched).
		}
	}
	for i := range items {
		items[i].Rank = 0
		items[i].BackgroundRank = 0
		if !rootCauseItemIsOnChain(items[i]) {
			// DCS E1b/E6 (ledger §23.1 rulings ②/③): typed 榜位 on the
			// non-on-chain composite board — the mention-obligation gate reads
			// background_rank<=3, never a prose position guess. The POSITION
			// counts every published non-on-chain row; the FIELD (复核 F-2) is
			// stamped on semantic compile span rows only, matching the two
			// output faces (text line / typed note) so the JSON payload of a
			// semantic-free trace stays byte-stable.
			backgroundPos++
			if rootCauseItemIsSemanticSpanWork(items[i]) {
				items[i].BackgroundRank = backgroundPos
			}
		}
		if items[i].Type == "trace_gap" {
			// G2 降道跳臂 (§27.2 + §28.1 user ruling 2026-07-09): a data blind
			// spot is a diagnostic FACT, never a cause — the row wears the
			// independent data_gap tier, takes no election slot, shifts nothing
			// below it and carries no rank ordinal (G9: no board seat for a row
			// the board never shows; pre-G2 the opendir_79/huadong_79 boards
			// seated blind-spot rows at #6-#12). The observation itself keeps
			// publishing unchanged — the ◇ display arm consumes the tier and
			// the typed trace_gap_kind criterion in the follow-up tool batch.
			// Precise mint-time type token only (single expandChain mint site).
			items[i].Tier = RootCauseTierDataGap
			continue
		}
		if items[i].Type == "pacing_idle" || items[i].Type == "periodic_idle" {
			// P9 arm c (§29.42 案1 BINDER-MISATTR, 2026-07-12): an idle-cadence
			// segment (frame-pacing or generic periodic — 复核 P2-1 fork) is
			// causal-analysis CONTEXT — the thread waiting for its next tick,
			// never a cause. The row keeps its display/evidence seat on the
			// context-only tier, takes no election slot and no rank ordinal.
			// Precise mint-time type tokens only (single attachIPCGraphToChain
			// mint site); deliberately placed BEFORE the self-symptom arm so an
			// upstream node's idle row demotes the same way as the target's own.
			items[i].Tier = RootCauseTierContextOnly
			continue
		}
		if items[i].SubjectIsAnalysisTarget && rootCauseItemIsTargetWaitSymptomType(items[i]) {
			// SYM (§24.13 裁定一, real_trace_campaign_20260705.md, 2026-07-08):
			// the analysis target's OWN wait-symptom rows are the symptom being
			// explained — same election-ladder transparency as the semantic
			// compile-span arm above: the row neither takes a
			// primary/secondary/tertiary slot nor shifts the slots of the
			// causal rows below it, and it never enters the ranked election
			// (opendir_78 witness: the target's self-held
			// AssetManager lock, a resolved blocking_span, wore rank#1
			// tier=primary and was crowned 主根因 for the target's own jank).
			// The sort/Score lanes never read the flag, and the
			// BackgroundRank counting above is
			// untouched. Judged on typed SUBJECT identity — the counterpart
			// side of the same contention (subject != target, e.g. a peer's
			// binder_wait row) keeps competing through the ladder below. A
			// Semantic span-work is not a wait-on-counterpart symptom and falls
			// through to the ordinary election before this arm is relevant.
			//
			// EVOLUTION RECORD (SYM-2, ledger §24.17, 2026-07-08): the arm
			// narrowed from ALL stamped self rows to subject==target ∧
			// 等待症状族 (rootCauseItemIsTargetWaitSymptomType — the registry
			// wakeup_chain + lock_contention lanes). The 自因可拆解族 (self
			// runnable / running / IO / D-state …) fell through to the normal
			// ladder below: those rows take election slots and may be crowned
			// lead by their strict position — the target's own
			// scheduling pressure / compute-supply shortfall / IO block is an
			// actionable system cause, not a counterpart symptom (cmp_78
			// witness: both sides crowned the self binder wait while the
			// decomposable self causes were locked out of the election).
			//
			// EVOLUTION RECORD (G9, §28.1 user ruling 2026-07-09): the §24.13
			// "榜位照发" clause is superseded — the demoted symptom row now
			// carries Rank=0 instead of pre-consuming an ordinal the display
			// never shows (see the function header). Tier semantics unchanged.
			items[i].Tier = RootCauseTierTargetSelfState
			continue
		}
		if rootCauseOrdinalChannel(items[i]) != rootCauseOrdinalChannelBackground &&
			CausalTokenCaliberSideClass(items[i].Type) != CausalCaliberSideNone {
			// V2-P0 行级尺守卫 (rank_order_v2_design_20260712.md §6.1 新裁定 A,
			// GREENLIT 2026-07-12): count-additivity / composite-score rows never
			// occupy ordinal space on the chain/◇ channels — a 计数当量 0.600
			// must not outrank wall-clock 0.198 in one ordinal sequence (4165
			// 自违序列 witness). The row keeps its channel seat as the ⌗ 口径旁栏
			// (rendered, caliber-worded, Rank=0, no badge, no election slot);
			// ▒ background rows are untouched (no ordinals there to guard, and
			// BackgroundRank above stays as assigned). Typed criterion = the
			// SHARED registry arm (Additivity==count OR composite-score marker);
			// the sort/Score lanes never read this arm.
			items[i].Tier = RootCauseTierCaliberSide
			continue
		}
		if rootCauseEffectiveImpactMs(items[i]) <= 0 {
			// A zero effective attribution is typed chain/background context, not
			// a competing cause. Keep the row visible on the bounded rank-0 side
			// lane without consuming an ordinal or election position.
			items[i].Tier = RootCauseTierContextOnly
			continue
		}
		if rootCauseItemIsSemanticSpanWork(items[i]) && !rootCauseItemIsOnChain(items[i]) {
			// SEM-LEAD-P0 (2026-07-10): only OFF-CHAIN semantic work belongs
			// to the background board. It is transparent to the causal election,
			// always wears the supporting tier and carries BackgroundRank above.
			// ON-CHAIN semantic rows deliberately fall through to the ordinary
			// primary/secondary/tertiary ladder below: their measured interval is
			// part of the causal chain and must be allowed to win the root cause.
			items[i].Tier = "tertiary"
			takeOrdinal(i)
			continue
		}
		tier := rootCauseTier(electionPos)
		electionPos++
		items[i].Tier = tier
		// EVOLUTION RECORD (复核 P1-2, 2026-07-09): the batch's initial
		// PeriodicSource no-ordinal arm here was DELETED — its premise ("the
		// display already suppresses their board row") was falsified by the
		// adversarial review: the shared board (runtimeTraceProjRankBoard) has
		// no PeriodicSource filter arm and every board/lead/❶/成因-grammar
		// gate keys on Rank>0, so withholding the ordinal stripped a
		// discounted periodic row of its §24 裁定① competition identity
		// (成因行身份=根因排序参赛身份) and killed the VS-1 late-period form —
		// a 30ms-late VSync ranks by its discounted eff≈30ms and may
		// legitimately be crowned. Periodic rows take ordinals like every
		// competing row; only the wait-symptom and data_gap arms above skip.
		takeOrdinal(i)
	}
}

func rootCauseItemIsOnChain(item RootCauseRankItem) bool {
	if strings.TrimSpace(item.ChainRelevance) == "on_chain" {
		return true
	}
	return strings.TrimSpace(item.Causality) == "on_wakeup_chain"
}

// rootCauseOrdinalChannel values (UXR-1, §29.36.2 三通道裁定 2026-07-11).
// Closed set — the ordinal allocator switches on it and the display chip
// word forks on the SAME chain-relevance signal (single source, three faces:
// glyph lane / stanza membership / ordinal channel).
const (
	rootCauseOrdinalChannelChain      = "chain"      // 通道1 根因排序#N
	rootCauseOrdinalChannelAdjacent   = "adjacent"   // 通道2 邻近影响#N
	rootCauseOrdinalChannelBackground = "background" // 通道3 无序数
)

// rootCauseOrdinalChannel resolves a rank row's ordinal-allocation channel
// from the typed chain-relevance single source (ChainRelevance field, with
// the same causality fallback the sort lane uses — never a prose judgment).
//
// EMPTY relevance stays on the chain channel (fail-open): a chainless trace
// never runs normalizeRootCauseChainRelevance, so its whole board carries no
// relevance and remains ONE caliber-uniform ordinal space rendered in the
// display's chain universe (flat fallback) — robbing it of ordinals would be
// a §29.36.2 over-reach (the ruling splits the ◇/▒ stanza channels, which
// only exist as EXPLICIT typed relevance). With a causal chain present the
// normalize pass stamps every row before sorting, so background rows always
// reach this switch with their explicit token.
func rootCauseOrdinalChannel(item RootCauseRankItem) string {
	if rootCauseItemIsOnChain(item) {
		return rootCauseOrdinalChannelChain
	}
	relevance := strings.TrimSpace(item.ChainRelevance)
	if relevance == "" {
		relevance = chainRelevanceFromCausality(item.Causality)
	}
	switch relevance {
	case "adjacent":
		return rootCauseOrdinalChannelAdjacent
	case "background":
		return rootCauseOrdinalChannelBackground
	default:
		return rootCauseOrdinalChannelChain
	}
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
	resolution := resolveThreadSelection(idx, q)
	target := resolution.Thread
	direction := normalizeInteractionDirection(q.InteractionDirection)
	res := InteractionStatsResult{
		Target:    target,
		Window:    TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
		Direction: direction,
	}
	if target.PID == 0 && target.Comm == "" {
		if resolution.Ambiguous {
			res.Caveats = append(res.Caveats, threadResolutionCaveat(idx, q))
		}
		res.Caveats = append(res.Caveats, "target thread not found; provide pid or a thread name visible in the trace")
		return res
	}
	if conflict := threadIncarnationConflictForQuery(idx, q, 0); conflict != nil {
		res.Caveats = append(res.Caveats, "thread_identity_fail_closed=true; "+conflict.reason()+"; interaction rows are omitted because the target numeric TID spans task incarnations")
		return res
	}
	acc := map[string]*InteractionSummary{}
	add := func(peer ThreadRef, ts float64, line int, kind string) {
		if peer.PID == 0 && peer.Comm == "" {
			return
		}
		key := threadKey(peer)
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
		if schedWakeupStartsNewIncarnation(ev) {
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
	limit := ViewCapacityFor("interaction_stats").ClampLimit(q.Limit)
	if len(res.Items) > limit {
		last := res.Items[limit-1]
		res.Compactions = append(res.Compactions, ViewCompaction{
			View:            "interaction_stats",
			Dimension:       CompactionDimensionPeers,
			Total:           len(res.Items),
			Emitted:         limit,
			LastEmittedTs:   last.LastTs,
			LastEmittedLine: last.LastLine,
		})
		res.Caveats = append(res.Caveats, fmt.Sprintf("interaction_stats compacted from %d to %d peer(s)", len(res.Items), limit))
		res.Items = res.Items[:limit]
	}
	if len(res.Items) == 0 {
		res.Caveats = append(res.Caveats, "no wakeup or binder interactions with the target were found in the selected window")
	}
	res.Caveats = append(res.Caveats, ipc.Caveats...)
	res.Compactions = append(res.Compactions, ipc.Compactions...)
	return res
}

func BuildFrameRootCauseBundle(idx *Index, q Query) FrameRootCauseBundle {
	q = normalizeQuery(idx, q)
	frame := BuildFrameTimeline(idx, q)
	targetResolution := ResolveFrameTarget(idx, q, frame)
	analysisQ := applyFrameTargetResolution(q, targetResolution)
	if frameTargetResolutionWindowChanged(q, targetResolution) {
		frame = BuildFrameTimeline(idx, analysisQ)
	}
	cache := newChainQueryCache(idx)
	stats := ComputeWindowStats(idx, analysisQ)
	var chain ChainResult
	if analysisQ.PID > 0 || analysisQ.Thread != "" || analysisQ.ThreadInput != "" {
		chain = buildWakeupChainWithCache(idx, analysisQ, cache)
	}
	rank := buildRootCauseRankFrom(idx, analysisQ, chain, stats)
	latency := buildSchedulerLatencyStatsFromStats(idx, analysisQ, stats)
	rank = enrichRootCauseRankWithScheduler(analysisQ, rank, latency, stats, chain)
	rank = attachPerfContextToRootCauseRank(idx, analysisQ, rank, stats)
	var chainPtr *ChainResult
	if len(chain.Nodes) > 0 || len(chain.Edges) > 0 || len(chain.CausalImpacts) > 0 || chain.Target.PID > 0 || chain.Target.Comm != "" {
		chainPtr = &chain
	}
	target := firstNonEmptyThread(chain.Target, targetResolution.Target, safeResolveThread(idx, analysisQ))
	blocking := buildCriticalBlockingCallsFromStats(idx, analysisQ, stats, chainPtr)
	// G1 跨车道对账 (§27.2, 2026-07-09): the bundle carries BOTH lanes of one
	// result — stamp the typed absorption markers here so every downstream
	// face (result JSON, typed observations, projection, tree) reads one
	// engine verdict. Marker-only writes; both lanes' rows keep publishing.
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	perfContexts := buildFramePerfContexts(idx, analysisQ, stats, chainPtr, blocking, target)
	bundle := FrameRootCauseBundle{
		Target:                target,
		TargetResolution:      frameTargetResolutionPtr(targetResolution),
		Window:                TimeWindow{StartTs: analysisQ.TimeStart, EndTs: analysisQ.TimeEnd},
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
		DMAFenceActivity:      stats.DMAFenceActivity,
		SupplyPressureSummary: stats.SupplyPressureSummary,
		TraceMarkCategories:   stats.TraceMarkCategories,
		AsyncFileWork:         stats.AsyncFileWork,
		windowStats:           &stats,
	}
	if chainPtr != nil {
		bundle.IOBurstEpisodes = enrichIOBurstEpisodesWithChainContext(*chainPtr, bundle.IOBurstEpisodes)
	}
	// RCX③ (§12.3-1 ③): the typed model-facing causal skeleton, built from the
	// components above so the head region carries a structured spine the model
	// narrates. F3: the target_state layer reads the target's OWN timeline
	// decomposition, never rank rows. §29.27② (COV-4, 2026-07-11): the ONE
	// timeline scan is shared with the full-window state account (PERF #21
	// discipline — no second event rescan).
	targetTimeline, targetTimelineOK := targetWindowTimeline(idx, analysisQ, target, bundle.Window)
	bundle.TargetWindowStates = buildTargetWindowStateAccount(idx, targetTimeline, targetTimelineOK, target, bundle.Window, &stats)
	bundle.Skeleton = buildCausalSkeleton(targetTimeline, targetTimelineOK, target, bundle.Window, &blocking, chainPtr, stats.SupplyPressureSummary)
	bundle.Caveats = append(bundle.Caveats, targetResolution.Caveats...)
	bundle.Caveats = append(bundle.Caveats, stats.Caveats...)
	bundle.Caveats = append(bundle.Caveats, frame.Caveats...)
	bundle.Caveats = append(bundle.Caveats, rank.Caveats...)
	bundle.Caveats = append(bundle.Caveats, blocking.Caveats...)
	bundle.Caveats = dedupStrings(bundle.Caveats)
	return bundle
}

func ResolveFrameTarget(idx *Index, q Query, frame FrameTimelineResult) FrameTargetResolution {
	q = normalizeQuery(idx, q)
	if q.PID > 0 || strings.TrimSpace(q.Thread) != "" || strings.TrimSpace(q.ThreadInput) != "" {
		target := safeResolveThread(idx, q)
		res := FrameTargetResolution{
			Target:       firstNonEmptyThread(target, ThreadRef{Comm: q.Thread, PID: q.PID}),
			Source:       "explicit_query_target",
			Confidence:   1,
			Window:       TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
			WindowSource: "query_window",
		}
		if res.Target.PID == 0 && strings.TrimSpace(res.Target.Comm) == "" {
			res.Caveats = append(res.Caveats, "frame_target_resolution explicit target could not be resolved from trace events")
		}
		return res
	}
	candidates := frameTargetResolutionCandidates(q, frame)
	res := FrameTargetResolution{
		Source:       "frame_timeline_ui_candidate",
		Window:       TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
		WindowSource: "query_window",
		Candidates:   frameTargetResolutionLimitCandidates(candidates, 6),
	}
	if len(candidates) == 0 {
		res.Source = "frame_timeline_no_ui_candidate"
		if selector := strings.TrimSpace(firstNonEmpty(q.Pattern, q.SpanName)); selector != "" {
			res.Caveats = append(res.Caveats, fmt.Sprintf("frame_target_resolution found no UI/main-like frame item matching selector %q; preserve query window and require explicit pid/thread for wakeup-chain target locking", selector))
		} else {
			res.Caveats = append(res.Caveats, "frame_target_resolution did not find a unique UI/main-like frame thread; preserve query window and require explicit pid/thread for wakeup-chain target locking")
		}
		return res
	}
	threads := map[string]FrameTargetCandidate{}
	for _, candidate := range candidates {
		key := frameTargetThreadKey(candidate.Thread)
		if key == "" {
			continue
		}
		if prev, ok := threads[key]; !ok || frameTargetCandidateLess(candidate, prev) {
			threads[key] = candidate
		}
	}
	if len(threads) != 1 {
		res.Source = "frame_timeline_ambiguous_ui_candidate"
		res.Caveats = append(res.Caveats, fmt.Sprintf("frame_target_resolution found %d UI/main-like frame thread candidates; not auto-locking target without explicit pid/thread", len(threads)))
		return res
	}
	var selected FrameTargetCandidate
	for _, candidate := range threads {
		selected = candidate
	}
	res.Target = selected.Thread
	res.Source = "frame_timeline_ui_unique"
	res.Confidence = 0.86
	res.SelectedFrame = &selected
	if frameTargetShouldDeriveFrameWindow(q) {
		prevEnd, ok := previousFrameEndForTarget(frame, selected)
		if !ok && frameTargetQueryHasExplicitSelectorWindow(q) {
			expandedFrame := BuildFrameTimeline(idx, frameTargetPreviousFrameSearchQuery(q, selected))
			prevEnd, ok = previousFrameEndForTarget(expandedFrame, selected)
		}
		if ok && prevEnd < selected.Window.EndTs {
			derived := TimeWindow{StartTs: prevEnd, EndTs: selected.Window.EndTs}
			if frameTargetQueryHasExplicitSelectorWindow(q) {
				res.Window = unionTimeWindows(TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}, derived)
				res.WindowSource = "explicit_query_union_previous_frame_end_to_current_frame_end"
				res.Caveats = append(res.Caveats, fmt.Sprintf("frame_target_resolution preserved explicit query window %.6f..%.6f and unioned it with frame-derived window %.6f..%.6f", q.TimeStart, q.TimeEnd, derived.StartTs, derived.EndTs))
			} else {
				res.Window = derived
				res.WindowSource = "previous_frame_end_to_current_frame_end"
			}
		} else {
			res.Caveats = append(res.Caveats, "frame_target_resolution could not find previous frame end; preserving query window")
		}
	}
	return res
}

func frameTargetShouldDeriveFrameWindow(q Query) bool {
	if q.FrameWindowAutoDerived {
		return true
	}
	return frameTargetQueryHasExplicitSelectorWindow(q)
}

func frameTargetQueryHasExplicitSelectorWindow(q Query) bool {
	return strings.TrimSpace(firstNonEmpty(q.Pattern, q.SpanName)) != "" &&
		queryExplicitTimeStart(q) &&
		queryExplicitTimeEnd(q)
}

func frameTargetPreviousFrameSearchQuery(q Query, selected FrameTargetCandidate) Query {
	searchQ := q
	searchQ.TimeStart = selected.Window.StartTs - 0.250
	if searchQ.TimeStart < 0 {
		searchQ.TimeStart = 0
	}
	searchQ.TimeEnd = selected.Window.EndTs
	searchQ.TimeStartSet = false
	searchQ.TimeEndSet = false
	searchQ.LineStart = 0
	searchQ.LineEnd = 0
	return searchQ
}

// unionTimeWindows merges two windows into the widest span that covers both,
// used whenever a caller supplies an explicit time window alongside a
// separately-derived window (frame target resolution, span_name lookups)
// so neither side's coverage is silently dropped. Both callers must pass
// determined bounds for `a` (the explicit user window) — this function uses
// pure geometric min/max and does NOT treat StartTs==0 as "unset", so an
// explicit user time_start of 0 is preserved rather than being replaced by
// the derived window's start (that replacement was a real R8 regression).
// The `b.StartTs > 0` guard only prevents a non-positive derived start from
// spuriously widening the window below zero; it never narrows `a`.
func unionTimeWindows(a, b TimeWindow) TimeWindow {
	out := a
	if b.StartTs > 0 && b.StartTs < out.StartTs {
		out.StartTs = b.StartTs
	}
	if b.EndTs > out.EndTs {
		out.EndTs = b.EndTs
	}
	return out
}

func frameTargetResolutionCandidates(q Query, frame FrameTimelineResult) []FrameTargetCandidate {
	selector := strings.TrimSpace(firstNonEmpty(q.Pattern, q.SpanName))
	var out []FrameTargetCandidate
	for _, item := range frame.Items {
		if item.Thread.PID == 0 && strings.TrimSpace(item.Thread.Comm) == "" {
			continue
		}
		roleScore := frameTargetRoleScore(item.Role, item.Phase)
		if roleScore <= 0 {
			continue
		}
		score := roleScore
		reason := "ui_or_main_like_frame_role"
		if selector != "" && frameTimelineItemMatchesSelector(item, selector) {
			score += 1000
			reason = "exact_frame_selector_and_ui_role"
		}
		out = append(out, FrameTargetCandidate{
			Thread:    item.Thread,
			Role:      item.Role,
			Phase:     item.Phase,
			Name:      item.Name,
			FrameID:   item.FrameID,
			Window:    TimeWindow{StartTs: item.StartTs, EndTs: item.EndTs},
			StartLine: item.StartLine,
			EndLine:   item.EndLine,
			Score:     score,
			Reason:    reason,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return frameTargetCandidateLess(out[i], out[j])
	})
	if selector == "" {
		return out
	}
	var exact []FrameTargetCandidate
	for _, candidate := range out {
		if strings.Contains(candidate.Reason, "exact_frame_selector") {
			exact = append(exact, candidate)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return nil
}

func frameTargetRoleScore(role, phase string) float64 {
	switch strings.TrimSpace(role) {
	case "ui":
		return 100
	}
	switch strings.TrimSpace(phase) {
	case "frame_schedule", "ui_traversal":
		return 90
	default:
		return 0
	}
}

func frameTimelineItemMatchesSelector(item FrameTimelineItem, selector string) bool {
	selector = strings.ToLower(strings.TrimSpace(selector))
	if selector == "" {
		return false
	}
	for _, value := range []string{item.Name, item.FrameID, item.Summary} {
		if strings.Contains(strings.ToLower(value), selector) {
			return true
		}
	}
	return false
}

func frameTargetCandidateLess(a, b FrameTargetCandidate) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.Window.StartTs != b.Window.StartTs {
		return a.Window.StartTs > b.Window.StartTs
	}
	return a.StartLine > b.StartLine
}

func previousFrameEndForTarget(frame FrameTimelineResult, selected FrameTargetCandidate) (float64, bool) {
	var prevEnd float64
	for _, item := range frame.Items {
		if item.EndTs <= 0 || item.EndTs >= selected.Window.StartTs {
			continue
		}
		if !threadMatches(selected.Thread, item.Thread.PID, item.Thread.Comm) {
			continue
		}
		if item.EndTs > prevEnd {
			prevEnd = item.EndTs
		}
	}
	return prevEnd, prevEnd > 0
}

func applyFrameTargetResolution(q Query, resolution FrameTargetResolution) Query {
	if resolution.Target.PID > 0 {
		q.PID = resolution.Target.PID
	}
	if strings.TrimSpace(resolution.Target.Comm) != "" {
		q.Thread = resolution.Target.Comm
		q.ThreadInput = resolution.Target.Comm
	}
	if frameTargetWindowValid(resolution.Window) {
		q.TimeStart = resolution.Window.StartTs
		q.TimeEnd = resolution.Window.EndTs
	}
	return q
}

func frameTargetResolutionWindowChanged(q Query, resolution FrameTargetResolution) bool {
	return frameTargetWindowValid(resolution.Window) &&
		(resolution.Window.StartTs != q.TimeStart || resolution.Window.EndTs != q.TimeEnd)
}

func frameTargetWindowValid(window TimeWindow) bool {
	return window.StartTs > 0 && window.EndTs > window.StartTs
}

func frameTargetResolutionPtr(resolution FrameTargetResolution) *FrameTargetResolution {
	if resolution.Target.PID == 0 && strings.TrimSpace(resolution.Target.Comm) == "" &&
		len(resolution.Candidates) == 0 && len(resolution.Caveats) == 0 && strings.TrimSpace(resolution.Source) == "" {
		return nil
	}
	return &resolution
}

func frameTargetResolutionLimitCandidates(candidates []FrameTargetCandidate, limit int) []FrameTargetCandidate {
	if limit <= 0 || len(candidates) <= limit {
		return append([]FrameTargetCandidate(nil), candidates...)
	}
	return append([]FrameTargetCandidate(nil), candidates[:limit]...)
}

func frameTargetThreadKey(thread ThreadRef) string {
	if thread.PID > 0 {
		return fmt.Sprintf("pid:%d", thread.PID)
	}
	comm := strings.ToLower(strings.TrimSpace(thread.Comm))
	if comm == "" {
		return ""
	}
	return "comm:" + comm
}

func safeResolveThread(idx *Index, q Query) ThreadRef {
	if idx == nil {
		return ThreadRef{Comm: q.Thread, PID: q.PID}
	}
	return resolveThread(idx, q)
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
	spans, caveats, spanCompaction := findSpanWindowsCompacted(idx, q, q.Limit)
	if spanCompaction != nil {
		res.Compactions = append(res.Compactions, *spanCompaction)
	}
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
	limit := ViewCapacityFor("frame_window").ClampLimit(q.Limit)
	if len(res.Items) > limit {
		last := res.Items[limit-1]
		res.Compactions = append(res.Compactions, ViewCompaction{
			View:            "frame_window",
			Dimension:       CompactionDimensionPhaseSpans,
			Total:           len(res.Items),
			Emitted:         limit,
			LastEmittedTs:   last.StartTs,
			LastEmittedLine: last.StartLine,
		})
		res.Caveats = append(res.Caveats, fmt.Sprintf("frame pipeline compacted from %d to %d phase span(s)", len(res.Items), limit))
		res.Items = res.Items[:limit]
	}
	if len(res.Items) == 0 {
		res.Caveats = append(res.Caveats, "no complete frame/render-like trace spans matched the selected filters")
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
	res.Compactions = append(res.Compactions, frame.Compactions...)
	if len(res.Items) == 0 {
		res.Caveats = append(res.Caveats, "no frame timeline items were built; need complete frame-like trace spans")
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
	RootCauseType      string
	Category           string
	Subcategory        string
	SemanticClass      string
	Label              string
	Confidence         float64
	ImpactMultiplier   float64
	MinOnChainImpactMs float64
}

// SemanticSpanPattern extends the built-in trace_mark semantic span
// classifier with deployment-specific naming conventions. It is an
// operator-provided classifier input, not a user-intent signal or hard gate.
type SemanticSpanPattern struct {
	SemanticClass string
	Contains      []string
	Tokens        []string
}

var customTraceSemanticSpanPatterns atomic.Value // stores []SemanticSpanPattern

// SetSemanticSpanPatterns installs deployment-specific semantic span patterns.
// Unknown classes and empty patterns are ignored. Built-in patterns always win,
// so config can add missing ROM/app names without weakening stable defaults.
func SetSemanticSpanPatterns(patterns []SemanticSpanPattern) {
	customTraceSemanticSpanPatterns.Store(normalizeSemanticSpanPatterns(patterns))
}

func traceSpanSemanticClass(name string) string {
	work, ok := traceSpanSemanticWorkClass(name)
	if !ok {
		return ""
	}
	return work.SemanticClass
}

// TraceSpanSemanticClass is the exported thin wrapper over the engine's
// semantic span classification (TDIAG B3, §28.13, 2026-07-09): the canonical
// semantic-class token for a span name (jit_compile / class_verification /
// shader_compile / runtime_compile / texture_upload / gc_pause /
// config-added classes),
// "" when unclassified. Same single classifier as root_cause_rank — never a
// second pattern set (anti-parallel-subsystem red line); deterministic
// consumers (tracediag census face ⑦) annotate span rosters with it.
func TraceSpanSemanticClass(name string) string {
	return traceSpanSemanticClass(name)
}

// TraceSpanNearMissesSemanticWork is the exported thin wrapper over the
// advisory near-miss signal (TDIAG B3): the span name mentions
// compile/verify/shader/texture-upload/GC-ish vocabulary but matched no known
// semantic pattern — the naming-drift blind-spot list. Advisory-only
// semantics travel with the export: it must never gate or promote anything.
func TraceSpanNearMissesSemanticWork(name string) bool {
	return traceSpanNearMissesSemanticWorkClassification(name)
}

// traceSpanNearMissesSemanticWorkClassification flags a span name that
// mentions compile/verify/shader/texture-upload-ish vocabulary but did not match any of the
// specific traceSpanLooksLike* patterns consumed by traceSpanSemanticWorkClass.
// This is a low-cost, advisory-only signal (a caveat, never a candidate or a
// tier) that the underlying app/ArkCompiler/ROM naming convention may have
// drifted from the hardcoded patterns; it must never gate or promote a
// candidate on its own.
func traceSpanNearMissesSemanticWorkClassification(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	tokens := traceSpanNameTokenSet(lower)
	return strings.Contains(lower, "shader") ||
		tokens["verify"] || tokens["verifier"] || tokens["verification"] ||
		strings.Contains(lower, "compile") || tokens["compiler"] || tokens["compilation"] ||
		traceSpanNearMissesGCPause(lower, tokens) ||
		// TEX (§28.1/§28.2, 2026-07-09): texture-upload naming drift — a span
		// that mentions BOTH words but did not match the strict
		// traceSpanLooksLikeTextureUpload prefix shape (e.g. "UploadTexture",
		// "GLES texture upload path"). Advisory-only, like every arm here.
		(strings.Contains(lower, "texture") && strings.Contains(lower, "upload"))
}

func traceSpanSemanticWorkClass(name string) (traceSpanSemanticWork, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return traceSpanSemanticWork{}, false
	}
	tokens := traceSpanNameTokenSet(lower)
	switch {
	case traceSpanLooksLikeJITCompile(lower, tokens):
		return traceSpanSemanticWorkForClass("jit_compile")
	case traceSpanLooksLikeClassVerification(lower, tokens):
		return traceSpanSemanticWorkForClass("class_verification")
	case traceSpanLooksLikeShaderCompile(lower, tokens):
		return traceSpanSemanticWorkForClass("shader_compile")
	case traceSpanLooksLikeRuntimeCompile(lower, tokens):
		return traceSpanSemanticWorkForClass("runtime_compile")
	case traceSpanLooksLikeTextureUpload(lower):
		return traceSpanSemanticWorkForClass("texture_upload")
	case traceSpanLooksLikeGCPause(lower, tokens):
		return traceSpanSemanticWorkForClass("gc_pause")
	default:
		if class, ok := customTraceSpanSemanticClass(lower, tokens); ok {
			return traceSpanSemanticWorkForClass(class)
		}
		return traceSpanSemanticWork{}, false
	}
}

func traceSpanSemanticWorkForClass(class string) (traceSpanSemanticWork, bool) {
	switch strings.TrimSpace(class) {
	case "jit_compile":
		return traceSpanSemanticWork{
			RootCauseType:      "jit_compile",
			Category:           "runtime_compile",
			Subcategory:        "jit",
			SemanticClass:      "jit_compile",
			Label:              "JIT compilation",
			Confidence:         0.82,
			ImpactMultiplier:   2.60,
			MinOnChainImpactMs: 4.0,
		}, true
	case "class_verification":
		return traceSpanSemanticWork{
			RootCauseType:      "class_verification",
			Category:           "runtime_verification",
			Subcategory:        "class_verification",
			SemanticClass:      "class_verification",
			Label:              "class verification",
			Confidence:         0.82,
			ImpactMultiplier:   2.40,
			MinOnChainImpactMs: 4.0,
		}, true
	case "shader_compile":
		return traceSpanSemanticWork{
			RootCauseType:      "shader_compile",
			Category:           "shader_compile",
			Subcategory:        "shader",
			SemanticClass:      "shader_compile",
			Label:              "shader compilation",
			Confidence:         0.80,
			ImpactMultiplier:   2.40,
			MinOnChainImpactMs: 4.0,
		}, true
	case "runtime_compile":
		return traceSpanSemanticWork{
			RootCauseType:      "runtime_compile",
			Category:           "runtime_compile",
			Subcategory:        "compile",
			SemanticClass:      "runtime_compile",
			Label:              "runtime compilation",
			Confidence:         0.78,
			ImpactMultiplier:   2.10,
			MinOnChainImpactMs: 3.5,
		}, true
	case "texture_upload":
		// TEX (§28.1 user ruling 2026-07-09, real_trace_campaign_20260705.md):
		// "Texture upload" is the FIFTH semantic span class, with exactly the
		// same treatment as VerifyClass/Shader/JIT — on-chain rows ride the
		// deterministic_optimization reserved-seat tier, off-chain rows enter
		// the background composite board (mention gate background_rank<=3), and
		// same-(thread,class) spans family-fold by window-projection interval
		// union (§24.10). The class label stays English (§22.2.1 专名尺子).
		// Confidence mirrors shader_compile (prefix-precise matcher);
		// multiplier/floor mirror runtime_compile (conservative bottom of the
		// existing band — texture uploads are tiny high-frequency spans whose
		// magnitude story is the FAMILY total, never a per-span boost).
		return traceSpanSemanticWork{
			RootCauseType:      "texture_upload",
			Category:           "texture_upload",
			Subcategory:        "texture",
			SemanticClass:      "texture_upload",
			Label:              "Texture upload",
			Confidence:         0.80,
			ImpactMultiplier:   2.10,
			MinOnChainImpactMs: 3.5,
		}, true
	case "gc_pause":
		// GC semantic work is intentionally narrower than memory_gc inventory:
		// only explicit pause/collection trace-span names reach this class. The
		// interval is therefore a measured, same-thread wall-clock candidate,
		// not a count-derived memory-pressure claim.
		return traceSpanSemanticWork{
			RootCauseType:      "gc_pause",
			Category:           "garbage_collection",
			Subcategory:        "gc_pause",
			SemanticClass:      "gc_pause",
			Label:              "GC pause",
			Confidence:         0.82,
			ImpactMultiplier:   2.40,
			MinOnChainImpactMs: 4.0,
		}, true
	default:
		return traceSpanSemanticWork{}, false
	}
}

func normalizeSemanticSpanPatterns(patterns []SemanticSpanPattern) []SemanticSpanPattern {
	if len(patterns) == 0 {
		return nil
	}
	out := make([]SemanticSpanPattern, 0, len(patterns))
	for _, p := range patterns {
		class := normalizeTraceSemanticSpanClass(p.SemanticClass)
		if _, ok := traceSpanSemanticWorkForClass(class); !ok {
			continue
		}
		contains := normalizeSemanticSpanContains(p.Contains)
		tokens := normalizeSemanticSpanTokens(p.Tokens)
		if len(contains) == 0 && len(tokens) == 0 {
			continue
		}
		out = append(out, SemanticSpanPattern{
			SemanticClass: class,
			Contains:      contains,
			Tokens:        tokens,
		})
	}
	return out
}

func normalizeTraceSemanticSpanClass(class string) string {
	class = strings.ToLower(strings.TrimSpace(class))
	class = strings.ReplaceAll(class, "-", "_")
	class = strings.ReplaceAll(class, " ", "_")
	return class
}

func normalizeSemanticSpanContains(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func normalizeSemanticSpanTokens(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		for token := range traceSpanNameTokenSet(strings.ToLower(strings.TrimSpace(item))) {
			if token == "" || seen[token] {
				continue
			}
			seen[token] = true
			out = append(out, token)
		}
	}
	sort.Strings(out)
	return out
}

func customTraceSpanSemanticClass(lower string, tokens map[string]bool) (string, bool) {
	raw := customTraceSemanticSpanPatterns.Load()
	if raw == nil {
		return "", false
	}
	patterns, _ := raw.([]SemanticSpanPattern)
	for _, p := range patterns {
		if semanticSpanPatternMatches(p, lower, tokens) {
			return p.SemanticClass, true
		}
	}
	return "", false
}

func semanticSpanPatternMatches(p SemanticSpanPattern, lower string, tokens map[string]bool) bool {
	for _, needle := range p.Contains {
		if needle != "" && strings.Contains(lower, needle) {
			return true
		}
	}
	if len(p.Tokens) == 0 {
		return false
	}
	for _, token := range p.Tokens {
		if !tokens[token] {
			return false
		}
	}
	return true
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

// traceSpanLooksLikeTextureUpload (TEX §28.1/§28.2, 2026-07-09) matches the
// customer's GPU texture-upload span family: the normalized name STARTS WITH
// "texture upload" (case-insensitive; "_"-joined and fully-joined spellings
// accepted like the JIT arm), tolerating the "(id)" and "WxH" suffixes of the
// real shape ("Texture upload(15283) 512x194" — trace_texture_upload.txt:8/34/53).
// Deliberately conservative, matching the other arms' precision bar:
//   - prefix-anchored, so "upload texture"/"TextureCache" never match
//     (word order and the second word are both load-bearing);
//   - the character right after the prefix must NOT be alphanumeric, so
//     "texture uploads"/"texture uploader" stay out — the observed suffixes
//     begin with '(' or ' '.
//
// The raw span name (id + dimensions) stays on the roster as the member
// distinguishing key; only the CLASS is normalized.
//
// F1 (对抗复核 SHIP-WITH-FIXES, 2026-07-09): hitrace prefixes user-space span
// names with "H:" — prefix anchoring alone rejected "H:Texture upload(…)"
// while the four substring-matched classmates pass straight through an H:
// prefix, breaking 完全同待遇 on the dual-stack platform (§28.5 T11). ONE
// case-insensitive "h:" prefix is stripped INSIDE this matcher only
// (single-point widening: the other arms and the near-miss advisory are
// untouched); the boundary rule then applies to the stripped name unchanged.
func traceSpanLooksLikeTextureUpload(lower string) bool {
	lower = strings.TrimPrefix(lower, "h:")
	for _, prefix := range []string{"texture upload", "texture_upload", "textureupload"} {
		if !strings.HasPrefix(lower, prefix) {
			continue
		}
		rest := lower[len(prefix):]
		if rest == "" {
			return true
		}
		next := rest[0]
		if (next >= 'a' && next <= 'z') || (next >= '0' && next <= '9') {
			continue
		}
		return true
	}
	return false
}

// traceSpanLooksLikeGCPause classifies only explicit garbage-collection work
// names. A bare substring check for "gc" is intentionally forbidden: it
// would turn unrelated names such as "GCLockerMetrics" or "gc_cache" into
// causal candidates. One hitrace H: prefix is accepted, then one of these
// precise shapes must hold:
//   - the exact token GC;
//   - GC followed by a delimiter and an explicit collection/pause phase;
//   - a garbage-collection prefix with a non-alphanumeric boundary;
//   - a small closed set of ART/runtime compound pause names.
func traceSpanLooksLikeGCPause(lower string, _ map[string]bool) bool {
	lower = strings.TrimSpace(strings.TrimPrefix(lower, "h:"))
	if lower == "gc" {
		return true
	}
	for _, prefix := range []string{"garbage collection", "garbage_collection", "garbagecollection"} {
		if semanticSpanHasPrefixBoundary(lower, prefix) {
			return true
		}
	}
	for _, exactPrefix := range []string{
		"suspendallforgc", "suspend_all_for_gc", "suspend all for gc",
		"waitforgctocomplete", "wait_for_gc_to_complete", "wait for gc to complete",
		"collectgarbage", "collect_garbage", "collect garbage",
	} {
		if semanticSpanHasPrefixBoundary(lower, exactPrefix) {
			return true
		}
	}
	if !semanticSpanHasPrefixBoundary(lower, "gc") {
		return false
	}
	rest := strings.TrimLeftFunc(lower[len("gc"):], func(r rune) bool {
		return r == ':' || r == '_' || r == '-' || r == '/' || r == ' ' || r == '\t' || r == '(' || r == '['
	})
	if rest == "" {
		return true
	}
	restTokens := traceSpanNameTokenSet(rest)
	for _, token := range []string{
		"pause", "paused", "collect", "collection", "collector", "young",
		"minor", "major", "full", "concurrent", "mark", "sweep", "compact",
	} {
		if restTokens[token] {
			return true
		}
	}
	return false
}

func semanticSpanHasPrefixBoundary(lower, prefix string) bool {
	if !strings.HasPrefix(lower, prefix) {
		return false
	}
	if len(lower) == len(prefix) {
		return true
	}
	next := lower[len(prefix)]
	return !((next >= 'a' && next <= 'z') || (next >= '0' && next <= '9'))
}

func traceSpanNearMissesGCPause(lower string, tokens map[string]bool) bool {
	lower = strings.TrimSpace(strings.TrimPrefix(lower, "h:"))
	return tokens["gc"] ||
		(strings.Contains(lower, "garbage") &&
			(tokens["collect"] || tokens["collection"] || tokens["collector"]))
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

// converterSyntheticLaneClass maps the CLOSED set of machine-generated
// synthetic lane/span names emitted by the hmtrace db2systrace.py converter
// (and byte-identically by codrax's own trace-db exporter,
// hitraceconv/streamerdb_export_extended.go) to soft semantic category
// labels (§7.11 B-5). These are machine tokens (exact converter format
// strings, file:line pinned per entry), so exact-case prefix + closed-suffix
// matching (all-digit run, or the converters' literal "None" null-identity
// fallback) is a precise signal here — but the OUTPUT is classification
// vocabulary and MUST stay soft guidance:
//
//	RED LINE (§7.11 B-5 / feedback_precise_signals_for_hard_gates): this
//	vocabulary is consumed ONLY by traceSpanCategory (display/suggestion
//	classification: span category fields, trace_mark category grouping).
//	It must never feed a hard gate, completion/emit reject, candidate
//	promotion, or ranking score. Structural pin:
//	TestConverterSyntheticLaneVocabConsumersPinnedB5.
//
// Near-miss safety: human-authored lookalikes ("TaskPool-Manager",
// "sys_read", lowercase variants) intentionally do NOT match — the numeric
// suffix and exact case are part of the machine shape.
func converterSyntheticLaneClass(name string) (string, bool) {
	switch {
	// Trace Streamer frame slices → "Frame{Actual|Expected}-{vsync}".  The
	// hitrace converter emits independent async S/F pairs keyed by the stable
	// frame_slice row identity; vsync remains display metadata, not pairing
	// identity. NULL vsync uses the verbatim token "None"
	// (hitraceconv/streamerdb_export_frame.go) — the same closed machine-name
	// set, matched by exact equality only.
	case hasMachineDigitSuffix(name, "FrameActual-"),
		hasMachineDigitSuffix(name, "FrameExpected-"),
		name == "FrameActual-None",
		name == "FrameExpected-None":
		return "frame_pacing", true
	// db2systrace.py:649-654: syscall slices → "sys_{syscall_number}"; NULL
	// syscall number falls back to the verbatim "sys_None"
	// (hitraceconv/streamerdb_export_extended.go:535).
	case hasMachineDigitSuffix(name, "sys_"), name == "sys_None":
		return "syscall", true
	// db2systrace.py:679-685: task_pool rows → async S/F "TaskPool-{task_id}";
	// the NULL fallback is "0" (hitraceconv/streamerdb_export_extended.go:579)
	// and is already covered by the digit-suffix shape.
	case hasMachineDigitSuffix(name, "TaskPool-"):
		return "task_pool", true
	// db2systrace.py:700-705: app_startup phases → "AppStartup:{sname}".
	case strings.HasPrefix(name, "AppStartup:"):
		return "app_startup", true
	// db2systrace.py:719-724: static_initalize (SO init) → "SoInit:{so_name}".
	case strings.HasPrefix(name, "SoInit:"):
		return "so_init", true
	default:
		return "", false
	}
}

// hasMachineDigitSuffix reports whether name is exactly prefix followed by a
// non-empty ASCII digit run — the converter's numeric-identity lane shape.
func hasMachineDigitSuffix(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	rest := name[len(prefix):]
	if rest == "" {
		return false
	}
	for i := 0; i < len(rest); i++ {
		if rest[i] < '0' || rest[i] > '9' {
			return false
		}
	}
	return true
}

func traceSpanCategory(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if work, ok := traceSpanSemanticWorkClass(name); ok {
		return work.Category
	}
	// §7.11 B-5: converter-synthetic lane names are a closed machine set;
	// label them before the generic Contains chain so accidental substring
	// hits stop mislabeling them — e.g. "SoInit:libaudio.so" read "audio"
	// (the audio case runs early in the chain) and "SoInit:libfileshare.so"
	// read file_io; both now read so_init. Soft classification only — the
	// built-in semantic work classes above keep priority, so root-cause
	// ranking surfaces are untouched.
	if class, ok := converterSyntheticLaneClass(strings.TrimSpace(name)); ok {
		return class
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
		// RN-16 (§7.9): critical-blocking rows ride the same causal-token
		// registry guard as root-cause rank rows.
		assertCausalTokenRow(item.Type, item.Thread, "critical_blocking")
		if item.Confidence <= 0 {
			item.Confidence = 0.6
		}
		if item.PeerState == nil {
			item.PeerState = buildCriticalBlockingPeerState(idx, q, item)
		}
		// A1 bounded continuation (§12.3-5): take the resolved counterpart ONE
		// sub-goal hop further. sourceIsInferred inherits presumptive confidence
		// when the counterpart itself was only wakeup-edge-resolved.
		if item.PeerChain == nil && item.Peer.PID > 0 {
			// LCK-2: set membership instead of the single-value wakeup_edge
			// comparison — a ns-span-derived counterpart is also an inference
			// and inherits presumptive confidence the same way.
			sourceIsInferred := counterpartSourceIsInferred(item.HolderSource) || counterpartSourceIsInferred(item.PeerSource)
			item.PeerChain = buildCriticalBlockingPeerChain(idx, q, item, sourceIsInferred)
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
				PeerSource:        wait.PeerSource,
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
	// CASE-1 gap (a) (§29.52 立案, v5 P1 批, 2026-07-13; 修复轮 h1 ∿ 回归):
	// the D/IO chain-lane candidates carry their typed per-(thread,cpu)
	// group interval ENGINE-INTERNALLY (reconStartTs/reconEndTs) — the
	// same-source floats the G1 cross-lane reconciliation proves membership
	// against (the recon hard-skips interval-less rows; before this the
	// whole D/IO pair set was structurally unreachable). The interval is a
	// segment HULL and deliberately stays OFF the published StartTs/EndTs
	// wire: publishing it let the projection's span-overlap fold arms fire
	// on hull noise (the h1 ∿ pacing seat regression) and a hull is NOT an
	// occurrence segment (a 发生段 word minted from it would be false) —
	// the same hull-noise reasoning the CASE-1 ruling used to reject
	// containment membership. Identity carriage only: score/sort read
	// DurationMs·Confidence unchanged; the published wire is byte-identical
	// to pre-CASE-1.
	for _, td := range stats.DStateTop {
		add(CriticalBlockingCandidate{
			Type:         "d_state_or_io_wait",
			Thread:       td.Thread,
			DurationMs:   td.DurationMs,
			reconStartTs: td.StartTs,
			reconEndTs:   td.EndTs,
			LineStart:    td.LineStart,
			LineEnd:      td.LineEnd,
			Confidence:   0.80,
			// 修复轮二 件B: the per-group proof rides the chain-lane candidate
			// from the SAME ThreadDuration donor the family seats read.
			proofRefined: td.DStateAllNonIOProvenGroup(),
			proofCaller:  td.UnanimousCauseSymbol(),
			Summary:      fmt.Sprintf("%s spent %.3fms in non-IO D-state wait%s", threadLabel(td.Thread), td.DurationMs, durationCPUDetail(td)),
		})
	}
	for _, td := range stats.IOWaitTop {
		add(CriticalBlockingCandidate{
			Type:         "io_wait",
			Thread:       td.Thread,
			DurationMs:   td.DurationMs,
			reconStartTs: td.StartTs,
			reconEndTs:   td.EndTs,
			LineStart:    td.LineStart,
			LineEnd:      td.LineEnd,
			Confidence:   0.84,
			proofCaller:  td.UnanimousCauseSymbol(),
			Summary:      fmt.Sprintf("%s spent %.3fms in scheduler IO wait%s", threadLabel(td.Thread), td.DurationMs, durationCPUDetail(td)),
		})
	}
	// P2-3 (Q4-F root fold): the span lane arrives pre-folded — dual print
	// forms of the same lock publish exactly one candidate here. P0-E2a: the
	// counterpart is resolved (payload-direct vs wakeup-edge fallback) inside
	// collectBlockingSpanRows before this add.
	for _, row := range collectBlockingSpanRows(idx, stats) {
		add(row.cand)
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
	// RCX① (§12.3 ruling 1): stamp the typed drill-debt verdict for every
	// counterpart-lane row against THIS report's observation universe.
	stampCriticalBlockingDrillStatus(idx, res.Items, buildDrillSubjectUniverse(chainForContext, &stats))
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
	limit := ViewCapacityFor("critical_blocking_calls").ClampLimit(q.Limit)
	if len(res.Items) > limit {
		last := res.Items[limit-1]
		res.Compactions = append(res.Compactions, ViewCompaction{
			View:            "critical_blocking_calls",
			Dimension:       CompactionDimensionCandidates,
			Total:           len(res.Items),
			Emitted:         limit,
			LastEmittedTs:   last.EndTs,
			LastEmittedLine: last.LineEnd,
		})
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
	// Shared 5-lane pick (thread_state_universe.go) — same priority order the
	// inline candidates array always used.
	dominant, _ := dominantStateFromLanes(item.RunningMs, item.RunnableMs, item.SleepMs, item.DStateMs, item.IOWaitMs)
	return dominant
}

// buildCriticalBlockingPeerChain (A1 bounded continuation, ledger §12.3-5 ruling
// 5) takes a resolved blocking counterpart ONE sub-goal hop further: it
// decomposes the peer's OWN state over the parent blocking window and, when the
// peer was itself sleep-dominated, names the peer's single DIRECT (1-hop)
// blocker — the thread that woke the peer.
//
// Boundedness (hard, precise): the continuation is EXACTLY one hop. The direct
// blocker's own state is recorded as a bare dominant-state word only
// (DirectBlockerState); its blocker is NEVER resolved. There is no recursion,
// no depth parameter, no loop — the peer's peer cannot blow the chain up (q1
// L31-33 lesson). Called at most once per critical-blocking row.
//
// Presumption inheritance (§12.3-5, LCK-2 set form): sourceIsInferred is true
// when the parent counterpart itself came from ANY inference lane —
// counterpartSourceIsInferred(HolderSource/PeerSource), i.e. wakeup_edge or
// ns_span_derivation. The whole step then rides Presumptive=true.
//
// Two hardening rules (P0-E2b review F1/F2):
//   - F1 self-loop discard: in a sync request-reply shape the waiter wakes the
//     peer INSIDE its own blocking window, so the peer's "last waker" is the
//     waiter itself — publishing "the blocker of A's blocker is A (and A is
//     sleeping)" is a causal inversion loop the model would narrate as a fake
//     deadlock. Such an edge is discarded outright (PID-level identity check
//     against the parent row's thread); the inverted-loop name carries zero
//     information, so no annotation is kept.
//   - F2 hop-2 is ALWAYS an inference: DirectBlocker has exactly one resolution
//     lane (the peer's wakeup edge), so it always carries
//     DirectBlockerSource=wakeup_edge and the step confidence is demoted to the
//     counterpartDemotedConfidence ceiling whenever a blocker is aboard — a
//     payload-direct PEER never lends its direct-evidence confidence to an
//     inferred hop-2 name.
func buildCriticalBlockingPeerChain(idx *Index, q Query, item CriticalBlockingCandidate, sourceIsInferred bool) *PeerChainStep {
	if idx == nil || item.Peer.PID <= 0 {
		return nil
	}
	// F5: the add funnel computes item.PeerState (same window derivation, same
	// builder) immediately before this call — reuse it instead of re-scanning
	// the peer timeline; compute only when a caller passes a bare item.
	state := item.PeerState
	if state == nil {
		state = buildCriticalBlockingPeerState(idx, q, item)
	}
	if state == nil {
		return nil
	}
	step := &PeerChainStep{
		Peer:        item.Peer,
		State:       state,
		Presumptive: sourceIsInferred,
		Confidence:  1.0,
	}
	start, end := item.StartTs, item.EndTs
	if start <= 0 {
		start = q.TimeStart
	}
	if end <= start {
		end = q.TimeEnd
	}
	// One hop only: if the peer was itself sleep-dominated it was blocked on
	// someone — name that single direct blocker via the peer's OWN wakeup edge.
	// A running/runnable/D-state-dominant peer was not itself sleep-blocked, so
	// there is no upstream blocker to name (the chain legitimately terminates
	// here, not because we refused to look).
	if state.DominantState == string(StateSSleep) {
		// F1: an edge whose waker IS the parent waiter is a causal inversion
		// (sync request-reply wakeup), never a blocker — discard, don't annotate.
		if fb := resolveCounterpartViaWakeupEdge(idx, item.Peer, start, end); fb.OK && fb.Waker.PID != item.Thread.PID {
			step.DirectBlocker = fb.Waker
			// F2: the hop-2 name is structurally a wakeup-edge inference — always
			// stamped, regardless of how the PEER itself was resolved.
			step.DirectBlockerSource = CounterpartSourceWakeupEdge
			// Depth cap 1: the blocker's dominant state is a bare word, NOT a
			// recursive breakdown (that would be a second continuation hop).
			step.DirectBlockerState = peerDirectBlockerDominantState(idx, q, fb.Waker, start, end)
		}
	}
	if sourceIsInferred || threadRefResolved(step.DirectBlocker) {
		// Any inference aboard — parent counterpart inferred (presumption
		// inheritance) or a hop-2 blocker named (always an inference, F2) —
		// demotes the step to the wakeup-edge ceiling.
		step.Confidence = counterpartDemotedConfidence(step.Confidence)
	}
	step.Summary = peerChainStepSummary(step)
	return step
}

// peerDirectBlockerDominantState returns ONLY the dominant-state word of the
// peer's direct blocker over the same window — a bare label, deliberately not a
// full ThreadStateBreakdown, so the continuation stays at depth 1 (naming the
// blocker's second-hop blocker would be depth 2). Empty when the blocker has no
// timeline in the window.
func peerDirectBlockerDominantState(idx *Index, q Query, blocker ThreadRef, start, end float64) string {
	if idx == nil || blocker.PID <= 0 || end <= start {
		return ""
	}
	tq := q
	tq.View = ""
	tq.PID = blocker.PID
	tq.Thread = blocker.Comm
	tq.ThreadInput = ""
	tq.TimeStart = start
	tq.TimeEnd = end
	bd := summarizeThreadStateBreakdown(ThreadTimeline(idx, tq))
	if bd == nil {
		return ""
	}
	return bd.DominantState
}

func peerChainStepSummary(step *PeerChainStep) string {
	if step == nil || step.State == nil {
		return ""
	}
	// The origin label describes the PEER's resolution only; a named blocker is
	// always separately labelled as inferred (F2) — a payload-direct peer never
	// extends its direct-evidence label over the hop-2 name.
	origin := "peer payload-direct"
	if step.Presumptive {
		origin = "peer inferred (counterpart itself only wakeup-edge-resolved)"
	}
	base := fmt.Sprintf("continuation off %s (%s): peer dominant=%s over %.3fms",
		threadLabel(step.Peer), origin, step.State.DominantState, step.State.TotalMs)
	if threadRefResolved(step.DirectBlocker) {
		base += fmt.Sprintf("; its own direct blocker is %s (dominant=%s, via wakeup edge — inferred) — one hop only, not expanded further",
			threadLabel(step.DirectBlocker), firstNonEmpty(step.DirectBlockerState, "unknown"))
	}
	return base
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

// blockingSpanCandidateFromTraceSpan is the ONE blocking-span carve shared by
// the critical_blocking view (row candidates) and the root_cause_rank
// lock-contention candidate source (Q4-A 修1, ledger §12.1/§12.2 P0-E): the
// blocking-like text screen and the structured lock-contention payload parse
// (§7.30.3 D1) live here so the two faces can never drift. ok=false for spans
// that are not blocking-like at all.
func blockingSpanCandidateFromTraceSpan(span TraceSpanSummary) (CriticalBlockingCandidate, bool) {
	// BLIND-2 (§29.7-1): the generalized `owner tid[:=]<N>` key form admits on
	// its own — the key is the carrying signal (keyed precise form); the
	// blocking-vocabulary screen keeps admitting the free-text family exactly
	// as before, so a vendor prefix without lock/wait tokens no longer hides
	// its contention span from this lane.
	if !isBlockingLikeText(span.Name) && !spanNameCarriesOwnerTidKey(span.Name) {
		return CriticalBlockingCandidate{}, false
	}
	cand := CriticalBlockingCandidate{
		Type:       "blocking_span",
		Thread:     span.Thread,
		DurationMs: span.DurationMs,
		StartTs:    span.StartTs,
		EndTs:      span.EndTs,
		// DCS E4 复核 F-1 (ledger §23.2): a boundary-straddling blocking span
		// was minted from its window-clipped extent — port the physical B/E
		// extent so the lock lane keeps the same dual-basis disclosure
		// discipline as the semantic lane (⚠实际 display lane included).
		ActualStartTs:    span.ActualStartTs,
		ActualEndTs:      span.ActualEndTs,
		ActualDurationMs: span.ActualDurationMs,
		LineStart:        span.StartLine,
		LineEnd:          span.EndLine,
		Confidence:       0.72,
		Summary:          fmt.Sprintf("blocking-like trace span %q lasted %.3fms", span.Name, span.DurationMs),
	}
	if span.ActualDurationMs > 0 {
		// F-1 dual-basis disclosure: the published duration is the in-window
		// projection; the physical extent is named right next to it (same
		// actual_span vocabulary as the semantic rank lane).
		cand.Summary += fmt.Sprintf(" (window-clipped; actual_span=%.3fms window=%.6f..%.6f)", span.ActualDurationMs, span.ActualStartTs, span.ActualEndTs)
	}
	// §7.30.3 D1: structured ART/OHOS contention payloads carry the lock
	// owner — parse deterministically and publish it as the peer so the
	// projection renders the holder instead of an unattributed duration.
	if info, ok := parseLockContentionPayload(span.Name); ok {
		cand.BlockingKind = info.Kind
		cand.Peer = info.Owner
		cand.Waiters = info.Waiters
		cand.HolderSite = info.HolderSite
		// BLOCKFROM (§27.4 G13): the waiter's own blocking call site travels
		// next to the holder site, verbatim.
		cand.BlockingFromSite = info.BlockingFromSite
		// P0-E 锁车道修2 (§24.9-C F2): the payload hand-off chain is a typed
		// witness that the holder CHANGED during this wait — the parsed final
		// owner is the last holder, never the whole-span holder.
		cand.HolderHandoff = info.OwnerHandoff
		// §19 清点②: preserve the parsed lock-object name so an ownerless
		// contention row can still say WHAT it blocked on. Never overwrites a
		// wait_object the caller already set.
		if info.WaitObject != "" && cand.WaitObject == "" {
			cand.WaitObject = info.WaitObject
		}
		cand.Summary += lockContentionSummarySuffix(info)
	}
	return cand, true
}

// blockingSpanRow pairs one blocking-span candidate with its raw span name so
// both consumer faces (critical_blocking rows, root_cause_rank lock lane) are
// fed from the SAME folded lane. spanNsPID (LCK-2, §18.E) is the span's own
// trace-mark payload pid — the emitting waiter's pid-namespace process id —
// carried so the rung-② ns-span owner derivation can key the contention to
// its container namespace.
type blockingSpanRow struct {
	cand      CriticalBlockingCandidate
	spanName  string
	spanNsPID int
	// payloadOwnerTid preserves the PRE-RESOLUTION payload owner tid (the
	// fold/attribution key) — after resolveBlockingSpanRowCounterpart the
	// inferred lanes clear cand.Peer to the recovered thread and rung ①
	// leaves the payload tid in Peer.PID, so the self-contradiction guard
	// (P0-E 锁车道修2) needs the original claim in one stable place.
	payloadOwnerTid int
}

// collectBlockingSpanRows (P2-3, absorbs Q4-F/Q5-D at the root): carves every
// blocking-like span and folds SAME-LOCK duplicate print forms — the ART
// runtime emits the same contention once as the rich "monitor contention with
// owner …" form and once as the "Lock contention on a monitor lock (owner
// tid: N)" form. Fold gate is fully typed: equal BlockingKind ∧ both owners
// resolved to the SAME PID ∧ overlapping intervals (never display-string
// comparison). The information-richer form survives (owner comm +
// holder_site), the value takes the larger measured duration, and the folded
// duplicates are recorded on MergedLines — every downstream face (blocking
// rows, rank candidates, drill stamps, next-step synthesis) is naturally
// single.
//
// P0-E2a (§10 A2 / §12 Q4-C): the counterpart resolve pass runs AFTER the fold
// so each folded row is resolved exactly once (the fold key = the payload
// owner tid, unaffected by the fallback). resolveBlockingSpanRowCounterpart
// stamps HolderSource and, when the payload owner tid is a cross-namespace
// phantom, swaps the phantom for the waiter's real wakeup-edge waker (and gives
// payload-less blocking spans a wait_object).
func collectBlockingSpanRows(idx *Index, stats WindowStats) []blockingSpanRow {
	var rows []blockingSpanRow
	for _, span := range stats.TraceSpans {
		cand, ok := blockingSpanCandidateFromTraceSpan(span)
		if !ok {
			continue
		}
		row := blockingSpanRow{cand: cand, spanName: span.Name, spanNsPID: span.SpanPID, payloadOwnerTid: cand.Peer.PID}
		if cand.BlockingKind != "" && cand.Peer.PID > 0 {
			merged := false
			for i := range rows {
				if sameLockContention(rows[i].cand, cand) {
					rows[i] = foldLockContentionRow(rows[i], row)
					merged = true
					break
				}
			}
			if merged {
				continue
			}
		}
		rows = append(rows, row)
	}
	for i := range rows {
		resolveBlockingSpanRowCounterpart(idx, &rows[i])
	}
	guardLockHolderSelfContradiction(rows)
	return rows
}

// lockHolderSelfContradictionCoverage is the overlap-coverage threshold of the
// P0-E 锁车道修2 same-lock self-contradiction guard (§24.9-C F2): the guard
// fires only when the inferred holder's OWN same-owner contention span covers
// the MAJORITY of the span being attributed — it was queued on the same lock
// for most of the claimed tenure, so the whole-span holder claim is falsified
// by two typed rows (opendir_78: 112.223/115.944 ≈ 97% coverage). Sub-majority
// overlap stays untouched: a holder can legitimately re-queue on the lock
// after releasing it (hand-over adjacency), and demoting on a sliver would
// hard-gate on an ambiguous shape.
const lockHolderSelfContradictionCoverage = 0.5

// guardLockHolderSelfContradiction (P0-E 锁车道修2, ledger §24.9-C F2,
// 2026-07-09) cross-checks every INFERRED holder attribution against the
// other contention rows of the SAME payload owner: if the thread we inferred
// as the holder was ITSELF waiting on that same owner for ≥ the coverage
// threshold of the attributed span, the inference is self-contradictory —
// the closing-wake "last releaser" was a fellow waiter in the hand-over
// relay, not the whole-span holder. The row demotes back to UNRESOLVED
// (typed-pair gates read false → no direct on-chain slot, no 1.35 weight —
// §12.3 未解析不准入, unchanged machinery) and carries the typed
// HolderSelfContradiction witness + a summary disclosure. Payload-direct
// (rung ①) holders are exempt: their identity is a direct payload claim, not
// a closing-wake inference (the guard's precise scope).
func guardLockHolderSelfContradiction(rows []blockingSpanRow) {
	for i := range rows {
		cand := &rows[i].cand
		if !counterpartSourceIsInferred(cand.HolderSource) || cand.Peer.PID <= 0 || rows[i].payloadOwnerTid <= 0 {
			continue
		}
		spanMs := (cand.EndTs - cand.StartTs) * 1000
		if spanMs <= 0 {
			continue
		}
		for j := range rows {
			if j == i {
				continue
			}
			peer := &rows[j].cand
			if rows[j].payloadOwnerTid != rows[i].payloadOwnerTid || peer.Thread.PID != cand.Peer.PID {
				continue
			}
			overlap := windowOverlapMs(cand.StartTs, cand.EndTs, peer.StartTs, peer.EndTs)
			if overlap < spanMs*lockHolderSelfContradictionCoverage {
				continue
			}
			// G10 (§27.4 + §28.1 收口 2026-07-09): the witness travels typed
			// (HolderSelfContradiction → projection BlockingHolderContradiction)
			// straight onto the zh 明细 face, so it is minted in Chinese —
			// §22.2.1 词条尺子 (trace 专有名词 payload/tid 不翻译; number and
			// line formats byte-preserved: %.3fms, 行 %d-%d).
			witness := fmt.Sprintf("推断持有者 %s 自身在同一 payload 持有者 tid %d 上排队 %.3fms(本段共 %.3fms;行 %d-%d)",
				threadLabel(cand.Peer), rows[i].payloadOwnerTid, overlap, spanMs,
				peer.LineStart, peer.LineEnd)
			cand.HolderSelfContradiction = witness
			cand.Summary = appendRootCauseSummaryDetail(cand.Summary,
				"holder attribution withdrawn (same-lock self-contradiction): "+witness+" — a thread queued on the lock for most of the span cannot have held it for the whole span; the holder stays unresolved")
			cand.Peer = ThreadRef{}
			cand.HolderSource = ""
			break
		}
	}
}

// resolveBlockingSpanRowCounterpart (P0-E2a) runs once per folded blocking-span
// row. Priority chain, all typed:
//
//  1. Structured lock contention with a payload owner tid that IS present in
//     this trace → unchanged: stamp HolderSource=contention_payload. On the
//     information-rich monitor form (owner comm carried), a comm cross-check
//     guards against a tid collision — if a thread with that pid exists in the
//     trace but its observed comm never equals the payload's owner comm, the
//     payload id is a coincidental collision, so fall through to the fallback.
//     1b. LCK-2 rung ② (§18.E, BEFORE the wakeup-edge fallback): when the owner
//     tid is a container-namespace id on an ns-divergent contention span
//     (span SpanPID ≠ waiter host TGID), derive the host identity from the
//     trace's own trace_mark emission pairs — thread-level via ②a
//     self-reported ns-tid samples or the ②b main-thread special case
//     (HolderSource=ns_span_derivation, 0.67), process-level otherwise
//     (EXPLICIT downgrade disclosure, host tgid never enters Peer.PID). The
//     ②×③ cross-check against the closing wakeup runs inside
//     applyNsSpanOwnerResolution (identity unification / corroboration /
//     divergence disclosure).
//  2. Structured lock contention whose owner tid is NOT present (cross-ns
//     phantom or absent) and rung ② had no mapping material → recover the
//     holder from the waiter's direct 1-hop wakeup edge; stamp
//     HolderSource=wakeup_edge, keep the phantom on OwnerTidRaw, drop the
//     phantom PID off the Peer.
//     2b. Structured lock contention that is TYPED-OWNERLESS (§19 S1): the payload
//     printed an EXPLICIT no-holder sentinel (`owner tid: 0` or the uint64(-1)
//     form) or carried no owner tid at all → BlockingKind stays set but no real
//     owner exists. Keep the typed contention semantics + the parsed wait_object
//     (lock-object name), and route to the SAME payload-less wakeup-edge
//     fallback: the waiter's closing wake IS the thread that released the lock.
//     Never a phantom PID / OwnerTidRaw garbage number.
//  3. Payload-less blocking span (no BlockingKind) → publish wait_object=span
//     name and try the same wakeup-edge fallback for a counterpart candidate.
func resolveBlockingSpanRowCounterpart(idx *Index, row *blockingSpanRow) {
	if row == nil {
		return
	}
	cand := &row.cand
	// P0-E 锁车道修2 (§24.9-C F2): a payload hand-off chain proves the holder
	// CHANGED during the wait — every resolution lane below (payload-direct
	// included) is naming the FINAL holder only, and the whole-span duration
	// is a conservative envelope, never one thread's tenure. The disclosure
	// rides the row unconditionally; tenure segmentation is impossible from
	// the payload (no boundaries recorded), so segmenting would invent data.
	if len(cand.HolderHandoff) >= 2 {
		cand.Summary = appendRootCauseSummaryDetail(cand.Summary,
			fmt.Sprintf("payload owner segment records a hand-off chain (%s): the lock changed hands during this wait — the resolved holder is the FINAL holder in the chain, not the whole-span holder; per-holder tenure boundaries are not recorded, so the span duration stays a conservative whole-wait envelope", strings.Join(cand.HolderHandoff, " --> ")))
	}
	if cand.BlockingKind != "" && cand.Peer.PID > 0 {
		if idx.tidPresent(cand.Peer.PID) && !lockOwnerCommCollides(idx, cand.Peer) {
			cand.HolderSource = CounterpartSourceContentionPayload
			return
		}
		// The payload printed an owner id this trace cannot resolve (or a
		// colliding pid): try the ns-span derivation (rung ②) first, then fall
		// back to the waiter's wakeup-edge waker (rung ③).
		rawTid := cand.Peer.PID
		if ns := resolveOwnerViaNsSpan(idx, row.spanNsPID, cand.Thread.TGID, rawTid); ns.OK {
			applyNsSpanOwnerResolution(idx, cand, ns, rawTid)
			return
		}
		if fb := resolveCounterpartViaWakeupEdge(idx, cand.Thread, cand.StartTs, cand.EndTs); fb.OK {
			cand.Peer = fb.Waker
			cand.HolderSource = CounterpartSourceWakeupEdge
			cand.OwnerTidRaw = rawTid
			// Visible confidence downgrade: an inferred holder never scores as a
			// payload-direct one (P2 review, ipc.go receiver-inferred precedent).
			cand.Confidence = counterpartDemotedConfidence(cand.Confidence)
			cand.Summary = appendRootCauseSummaryDetail(cand.Summary, counterpartWakeupEdgeCaveat(rawTid))
			return
		}
		// No usable wakeup edge: leave the row unresolved. Drop the phantom
		// PID so the P0-E1 typed-pair gates correctly read "unresolved" instead
		// of pointing at a ghost id; preserve the raw tid for audit.
		cand.OwnerTidRaw = rawTid
		cand.Peer = ThreadRef{}
		return
	}
	if cand.BlockingKind != "" && cand.Peer.PID <= 0 && strings.TrimSpace(cand.Peer.Comm) == "" {
		// §19 S1 branch 2b: a recognised contention payload with NO resolvable
		// owner — an explicit ownerless sentinel (`owner tid: 0` / uint64(-1)) or
		// a payload that carried no owner tid slot at all. This is the typed
		// post-condition of the sentinel-safe parse (BlockingKind set, no Peer).
		// It previously fell through BOTH branches into the E2a dead corner (§19
		// 病灶). Keep the typed contention semantics + the parsed wait_object and
		// route to the payload-less wakeup-edge fallback: for an ownerless
		// contention the waiter's CLOSING wake is the thread that dropped the lock.
		// Crucially NO OwnerTidRaw is set (the sentinel is not a real tid → no
		// garbage 9223372036854775807 disclosure, pin④).
		if fb := resolveCounterpartViaWakeupEdge(idx, cand.Thread, cand.StartTs, cand.EndTs); fb.OK {
			cand.Peer = fb.Waker
			cand.HolderSource = CounterpartSourceWakeupEdge
			cand.Confidence = counterpartDemotedConfidence(cand.Confidence)
			cand.Summary = appendRootCauseSummaryDetail(cand.Summary, counterpartOwnerlessCaveat())
		}
		return
	}
	if cand.BlockingKind == "" {
		// §10 A2: payload-less blocking span — at least name the wait object,
		// and offer a wakeup-edge counterpart candidate when the row has no peer
		// yet.
		if cand.WaitObject == "" {
			cand.WaitObject = row.spanName
		}
		if cand.Peer.PID <= 0 && strings.TrimSpace(cand.Peer.Comm) == "" {
			if fb := resolveCounterpartViaWakeupEdge(idx, cand.Thread, cand.StartTs, cand.EndTs); fb.OK {
				cand.Peer = fb.Waker
				cand.PeerSource = CounterpartSourceWakeupEdge
				cand.Confidence = counterpartDemotedConfidence(cand.Confidence)
				cand.Summary = appendRootCauseSummaryDetail(cand.Summary, counterpartWakeupEdgeCaveat(0))
			}
		}
	}
}

// counterpartDemotedConfidence lowers a candidate's confidence to the
// wakeup-edge inference ceiling, never RAISING an already-lower value (a
// dest_thread=0 binder edge may already sit below it). Rank Score is monotone
// in Confidence, so this is the sole (no-gate) demotion of an inferred
// counterpart relative to a payload-direct row of equal impact.
func counterpartDemotedConfidence(current float64) float64 {
	if current > 0 && current < counterpartWakeupEdgeConfidence {
		return current
	}
	return counterpartWakeupEdgeConfidence
}

// counterpartWakeupEdgeCaveat is the symmetric advisory sentence for a
// wakeup-edge-inferred counterpart (mirrors the binder-side caveat at the
// findBinderWaitsForChain fallback). rawTid>0 names the phantom payload tid the
// current trace could not resolve; rawTid==0 is the payload-less blocking-span
// form.
func counterpartWakeupEdgeCaveat(rawTid int) string {
	if rawTid > 0 {
		return fmt.Sprintf("counterpart inferred from the waiter's wakeup edge because the payload owner tid %d is not present in this trace; it is the thread that woke the waiter, not a payload-confirmed holder", rawTid)
	}
	return "counterpart inferred from the waiter's wakeup edge (no structured owner in the payload); it is the thread that woke the waiter, not a payload-confirmed holder"
}

// counterpartOwnerlessCaveat is the advisory sentence for a typed-ownerless
// contention row (§19 S1 branch 2b): the payload named a contention lock but
// printed an EXPLICIT no-holder sentinel (owner tid 0 or the uint64(-1) form) or
// carried no owner tid at all, so there is no payload owner to name — the
// counterpart is recovered from the waiter's closing wakeup edge. Deliberately
// prints NO owner tid (the sentinel is not a real id), so no garbage number ever
// reaches the disclosure (pin④).
func counterpartOwnerlessCaveat() string {
	return "contention payload named no holder (ownerless: owner tid 0 or uint64(-1) sentinel); counterpart inferred from the waiter's closing wakeup edge — the thread that released the lock, not a payload-confirmed holder"
}

// lockOwnerCommCollides guards the payload-direct holder path against a tid
// collision: the rich monitor-contention form carries the owner's COMM, so if
// that pid is present in the trace but the trace never observed it under the
// payload's owner comm, the pid is a coincidental collision with an unrelated
// thread and the payload-direct resolution would misattribute. Returns false
// (no collision) when the payload carried no owner comm (nothing to cross-check
// — the tid-only "Lock contention on … (owner tid: N)" form) or when the comms
// match (truncation-aware, E2a correction ②).
func lockOwnerCommCollides(idx *Index, owner ThreadRef) bool {
	payloadComm := strings.TrimSpace(owner.Comm)
	if payloadComm == "" || owner.PID <= 0 || idx == nil {
		return false
	}
	sawPID := false
	for i := range idx.Events {
		ev := &idx.Events[i]
		for _, pair := range [...]struct {
			pid  int
			comm string
		}{{ev.PID, ev.Comm}, {ev.PrevPID, ev.PrevComm}, {ev.NextPID, ev.NextComm}, {ev.WakeePID, ev.WakeeComm}} {
			if pair.pid != owner.PID {
				continue
			}
			sawPID = true
			if lockOwnerCommMatches(payloadComm, strings.TrimSpace(pair.comm)) {
				return false
			}
		}
	}
	return sawPID
}

// schedCommMaxLen is the kernel TASK_COMM_LEN-1 truncation width: every comm a
// scheduler event carries is at most 15 bytes, while a contention payload
// prints the thread's FULL name.
const schedCommMaxLen = 15

// lockOwnerCommMatches (P0-E2b, E2a correction ②) is the truncation-aware comm
// equality for the collision cross-check. The payload's owner comm is the full
// Java/native thread name; the observed sched comm is truncated to 15 bytes —
// requiring full EqualFold equality made every >15-char owner name a FALSE
// collision, wrongly discarding a payload-direct holder for a wakeup-edge
// inference. A full name therefore also matches its own 15-byte truncation.
// The truncated compare fires ONLY in the exact kernel-truncation shape
// (observed exactly 15 bytes ∧ payload longer) — never a general prefix match,
// so "Thread-1" can not match "Thread-10".
func lockOwnerCommMatches(payloadComm, observedComm string) bool {
	if strings.EqualFold(observedComm, payloadComm) {
		return true
	}
	if len(observedComm) == schedCommMaxLen && len(payloadComm) > schedCommMaxLen &&
		strings.EqualFold(observedComm, payloadComm[:schedCommMaxLen]) {
		return true
	}
	return false
}

func sameLockContention(a, b CriticalBlockingCandidate) bool {
	// P0-E 锁车道修1 (ledger §24.9-C F1, 2026-07-09): the fold's design intent
	// is the SAME contention printed twice (rich monitor form + tid-only
	// form) — i.e. one WAITER's dual print. The key therefore pins the
	// emitting waiter identity: two PHYSICALLY DIFFERENT contention spans
	// (two different blocked threads queued on the same owner) must never
	// fold into one chimera row (opendir_78: the target's 112.2ms victim row
	// was swallowed whole by LegoHandler's 115.9ms span — the direction-flip
	// lesion's first half).
	return a.BlockingKind != "" && a.BlockingKind == b.BlockingKind &&
		a.Peer.PID > 0 && a.Peer.PID == b.Peer.PID &&
		a.Thread.PID > 0 && a.Thread.PID == b.Thread.PID &&
		windowOverlapMs(a.StartTs, a.EndTs, b.StartTs, b.EndTs) > 0
}

func foldLockContentionRow(a, b blockingSpanRow) blockingSpanRow {
	survivor, folded := a, b
	if lockContentionInfoRichness(b.cand) > lockContentionInfoRichness(a.cand) {
		survivor, folded = b, a
	}
	if folded.cand.DurationMs > survivor.cand.DurationMs {
		survivor.cand.DurationMs = folded.cand.DurationMs
		// F-1: the actual extent travels with the value that won the fold —
		// zeroed when the winning form was not clipped (absence stays the
		// precise "not clipped" signal for the surviving value).
		survivor.cand.ActualStartTs = folded.cand.ActualStartTs
		survivor.cand.ActualEndTs = folded.cand.ActualEndTs
		survivor.cand.ActualDurationMs = folded.cand.ActualDurationMs
	}
	merged := append([]int(nil), survivor.cand.MergedLines...)
	merged = append(merged, folded.cand.MergedLines...)
	merged = append(merged, firstPositive(folded.cand.LineStart, folded.cand.LineEnd))
	survivor.cand.MergedLines = merged
	// P0-E 锁车道修1 附带口径洞 (§24.9-C F5): the dual print forms of ONE
	// waiter's contention may disagree on waiters= — keep the MAX instead of
	// silently dropping the folded form's count.
	if folded.cand.Waiters > survivor.cand.Waiters {
		survivor.cand.Waiters = folded.cand.Waiters
	}
	// The hand-off witness travels with the fold (richer-form survivor may be
	// the tid-only print that carried no chain).
	if len(survivor.cand.HolderHandoff) == 0 {
		survivor.cand.HolderHandoff = folded.cand.HolderHandoff
	}
	// LCK-2: keep the container ns pid across the fold (both print forms of
	// the same lock come from the same emitting process; take the folded
	// form's pid only when the survivor carried none).
	if survivor.spanNsPID <= 0 {
		survivor.spanNsPID = folded.spanNsPID
	}
	return survivor
}

// lockContentionInfoRichness orders dual print forms of the same lock: the
// owner-comm form beats the tid-only form, a holder site breaks remaining
// ties.
func lockContentionInfoRichness(cand CriticalBlockingCandidate) int {
	score := 0
	if strings.TrimSpace(cand.Peer.Comm) != "" {
		score += 2
	}
	if cand.HolderSite != "" {
		score++
	}
	return score
}

// isBlockingLikeText screens a trace-span name for blocking-like vocabulary so
// a payload-less span can still be carved as a blocking_span candidate. This is
// a NOISY soft screen (free substring match), so its output only ever nominates
// a candidate — every hard consumer downstream re-gates on typed fields
// (BlockingKind, resolved Peer, drill status). Two §19 F1 de-noise rulings are
// baked in (§1 precise-signal red line,误判实证):
//
//   - `io` REMOVED: it fired on animation#action / TimerIteration /
//     AudioRenderSink / AudioVolume (the whole Audio DSP family) / H:…Context —
//     none of them a block. Real IO blocking has its OWN typed lanes
//     (io_latency / blocked_reason / d_state); it never needs a span-name
//     fallback, so the token bought nothing but false positives.
//   - `sync` KEPT but the VSync display family is EXCLUDED: bare `sync` matched
//     every Choreographer#onVsync / requestNextVsync / jank_event_sync frame
//     span (pure UI cadence, not a block). isVsyncCadenceText carves those out
//     while still admitting genuine sync-primitive waits.
//
// `lock` is kept as-is: it does catch AudioRunningLock (a wakelock ACCOUNTING
// span, not a contention wait) and UnlockMainThread, but a simple word boundary
// cannot cleanly separate "holding/contention lock" from "wakelock accounting"
// without a context model — over-engineering for a display-tier soft screen, so
// it is left as a known low-signal admission (§19 F1 observation, not fixed
// here; the typed BlockingKind gate keeps these out of every hard consumer).
func isBlockingLikeText(name string) bool {
	lower := strings.ToLower(name)
	for _, token := range []string{"lock", "futex", "wait", "blocked", "binder", "sync", "mutex", "semaphore", "contention"} {
		if !strings.Contains(lower, token) {
			continue
		}
		if token == "sync" && isVsyncCadenceText(lower) {
			// The only `sync` hit is the VSync cadence family — not blocking-like.
			continue
		}
		return true
	}
	return false
}

// isVsyncCadenceText reports whether an already-lowercased span name owes its
// `sync` substring solely to the VSync frame-cadence family (Choreographer#
// onVsync, requestNextVsync, jank_event_sync, …) and carries no OTHER
// blocking-like token. Such spans are UI pacing, never a lock/IO wait, so the
// `sync` screen must not admit them (§19 F1). If the name ALSO contains a real
// blocking token (e.g. a hypothetical "vsync … lock wait"), this returns false
// and the span is still admitted by that other token.
func isVsyncCadenceText(lower string) bool {
	hasVsyncFamily := strings.Contains(lower, "vsync") || strings.Contains(lower, "jank_event_sync")
	if !hasVsyncFamily {
		return false
	}
	for _, other := range []string{"lock", "futex", "wait", "blocked", "binder", "mutex", "semaphore", "contention"} {
		if strings.Contains(lower, other) {
			return false
		}
	}
	return true
}

// threadRefResolved reports whether a ThreadRef names a concrete thread
// (typed resolution check shared by the Q4-A admission gates).
func threadRefResolved(t ThreadRef) bool {
	return t.PID > 0 || strings.TrimSpace(t.Comm) != ""
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
	case "span_locate":
		// §16-R golden path: unlike the six analysis recipes above, this
		// recipe does NOT assume a window is already selected — it IS the
		// window-selection step. Locate first with a bare literal pattern
		// (deliberately no event_types filter, so nonstandard marker forms
		// still match), then pair the span into its start/end time and line
		// window for follow-up views.
		res.IncludedViews = []string{"event_search", "span_window"}
		res.Summary = "span-locate recipe: locate the named span's marker rows with a bare literal pattern search (event_search with the span label as pattern and no event_types filter), then resolve the paired span into its start/end time and line window (span_window with span_name) for follow-up views over the selected window"
		if strings.TrimSpace(q.SpanName) == "" && strings.TrimSpace(q.Pattern) == "" {
			res.Caveats = append(res.Caveats, "span_locate needs span_name (or pattern) naming the span label to locate; provide one and rerun")
		}
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
	case "span_locate", "locate_span", "span_locator":
		return "span_locate"
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

type chainQueryCache struct {
	idx             *Index
	eventsByPID     map[int][]int
	wakeupsByPID    map[int][]int
	blockedByPID    map[int][]int
	priorityByPID   map[int][]prioritySample
	timelineByKey   map[string]TimelineResult
	resolvedByQuery map[string]threadResolution
	// freqByCPU is the lazily-built per-CPU cpu_frequency sample timeline
	// (ts-ordered, kHz). Empty slice per CPU when the trace carries no
	// frequency events — consumers must treat "no sample" as unknown, not
	// as zero supply (R5d weak-core gate stays conservative).
	freqByCPU     map[int][]freqSample
	freqIndexOnce bool
	// freqOrder is the full-index physical-order verdict used by trace-global
	// topology/capability and chain-fold consumers. A per-CPU poison removes
	// only that CPU and also blocks same-cluster donor/rail substitution.
	freqOrder     frequencyOrderIntegrity
	freqOrderOnce bool
	// freqLimitByCPU is the lazily-built per-CPU cpu_frequency_limits Max
	// timeline (ts-ordered, kHz) — the VS-2b fmax ladder's step-2 source
	// (policy ceiling; see supply_fold.go).
	freqLimitByCPU map[int][]freqSample
	freqLimitOnce  bool
	// clockLaneSamples is the lazily-built cpu-freq-NAMED clock_set_rate
	// lane sample list (VS-2c corroboration caveat ONLY — never a fold
	// basis; see supply_fold.go).
	clockLaneSamples []clockLaneSample
	clockLaneOnce    bool
	// switchInByPID is the lazily-built per-thread switch-in (ts, cpu)
	// timeline so threadCPUNear stays O(log n) per lookup — a linear rescan
	// per RUNNING interval was O(intervals × pid-events) and hung on
	// GB-scale traces.
	switchInByPID map[int][]cpuSample
	// capabilityByTopo memoizes the CAP (§26) core-class capability judgment
	// per core_topology input (core_capability.go coreCapability) — one
	// cluster resolution per query, shared by the VS-2 fold and the R5d
	// weak-core gate.
	capabilityByTopo map[string]coreCapabilityMap
	// CAP-2 (§28.4/§28.5) memoized lanes (cluster_rail_evidence.go): the
	// six-gate keyed-rail scan, the scheduler-observed CPU set (gate ⑤ +
	// membership presumption bound) and the THERM thermal-rail timelines.
	railScanOnce    bool
	railScan        clusterRailScan
	schedCPUOnce    bool
	schedCPUs       map[int]bool
	thermalRailOnce bool
	thermalRails    []thermalRailTimeline
}

type cpuSample struct {
	ts   float64
	line int
	cpu  int
}

type freqSample struct {
	ts  float64
	khz int
}

func (c *chainQueryCache) buildFrequencyOrderIntegrity() {
	if c == nil || c.freqOrderOnce {
		return
	}
	c.freqOrderOnce = true
	c.freqOrder = frequencyOrderIntegrityForQuery(c.idx, Query{})
}

func (c *chainQueryCache) frequencyLaneUnsafe(cpu int) bool {
	if c == nil {
		return false
	}
	c.buildFrequencyOrderIntegrity()
	return c.freqOrder.frequencyUnsafe(cpu)
}

func (c *chainQueryCache) frequencyLimitLaneUnsafe(cpu int) bool {
	if c == nil {
		return false
	}
	c.buildFrequencyOrderIntegrity()
	return c.freqOrder.limitUnsafe(cpu)
}

func (c *chainQueryCache) frequencyOrderCaveats() []string {
	if c == nil {
		return nil
	}
	c.buildFrequencyOrderIntegrity()
	return c.freqOrder.caveats()
}

// buildFreqIndexLocked scans the index once for cpu_frequency events. The
// cache is used single-goroutine per query, mirroring the other lazy maps.
func (c *chainQueryCache) buildFreqIndex() {
	if c.freqIndexOnce {
		return
	}
	c.freqIndexOnce = true
	c.buildFrequencyOrderIntegrity()
	// VS-2c 终局裁定 (§7.10): clock_set_rate lanes reclassified as
	// cpu_frequency by the isCPUFrequencyClock NAME HEURISTIC carry
	// vendor-free-vocabulary names and emitting-CPU attribution (hmtrace
	// hardcodes cpu 0) — they MUST NOT enter the chain/fold per-CPU
	// frequency basis (neither fmax nor slice governance). Their max
	// surfaces only as the SupplyFoldBasis cluster-lane corroboration
	// caveat. Precise signal: verbatim event-name match, via the CFC
	// shared predicate (cluster_ceilings.go) so the window face cannot
	// drift from this exclusion. Pinned in semantic_ruling_pins_test.go.
	// CAP-3 (§29.11): collection now lives in the SHARED Index-global
	// collector (indexFreqSampleTimelines, cluster_freq_share.go) — same
	// admission, same cpu_id attribution, same event order; the window
	// faces' domain derivation reads the same function, so the fold and
	// window lanes share ONE topology basis by construction.
	c.freqByCPU = indexFreqSampleTimelines(c.idx)
}

// frequencyAt returns the cpu_frequency sample in effect on cpu at ts: the
// last sample at or before ts, falling back to the NEAREST later sample when
// no earlier one exists (R5e: a window that starts before the first frequency
// event must not be treated as unknown/zero/low supply — traces commonly emit
// the first cpu_frequency event only on the first change after tracing
// starts). Returns 0 only when the trace carries no samples for that CPU at
// all.
func (c *chainQueryCache) frequencyAt(cpu int, ts float64) int {
	if c.frequencyLaneUnsafe(cpu) {
		return 0
	}
	c.buildFreqIndex()
	samples := c.freqByCPU[cpu]
	if len(samples) == 0 {
		return 0
	}
	lo, hi, best := 0, len(samples)-1, 0
	for lo <= hi {
		mid := (lo + hi) / 2
		if samples[mid].ts <= ts {
			best = samples[mid].khz
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	if best == 0 {
		best = samples[0].khz
	}
	return best
}

// threadCPUNear returns the CPU the thread most recently switched IN on at or
// before ts. The per-thread switch-in timeline is built once per thread and
// binary-searched afterwards. ok=false when no switch-in precedes ts.
func (c *chainQueryCache) threadCPUNear(thread ThreadRef, ts float64) (int, bool) {
	if c.idx == nil || thread.PID <= 0 {
		return 0, false
	}
	if c.switchInByPID == nil {
		c.switchInByPID = map[int][]cpuSample{}
	}
	samples, built := c.switchInByPID[thread.PID]
	if !built {
		for _, id := range c.eventsByPID[thread.PID] {
			if id < 0 || id >= len(c.idx.Events) {
				continue
			}
			ev := c.idx.Events[id]
			if ev.Type == EventSchedSwitch && ev.NextPID == thread.PID {
				samples = append(samples, cpuSample{ts: ev.Ts, line: ev.Line, cpu: ev.CPU})
			}
		}
		sort.SliceStable(samples, func(i, j int) bool {
			if samples[i].ts != samples[j].ts {
				return samples[i].ts < samples[j].ts
			}
			return samples[i].line < samples[j].line
		})
		c.switchInByPID[thread.PID] = samples
	}
	scope := threadGenerationScopeAt(c.idx, thread.PID, ts, 0)
	if !scope.known {
		return 0, false
	}
	lo, hi := 0, len(samples)-1
	best := -1
	for lo <= hi {
		mid := (lo + hi) / 2
		if samples[mid].ts <= ts {
			best = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	if best < 0 || !scope.contains(samples[best].ts, samples[best].line) {
		return 0, false
	}
	return samples[best].cpu, true
}

type prioritySample struct {
	ts   float64
	line int
	prio int
}

func newChainQueryCache(idx *Index) *chainQueryCache {
	cache := &chainQueryCache{
		idx:             idx,
		eventsByPID:     map[int][]int{},
		wakeupsByPID:    map[int][]int{},
		blockedByPID:    map[int][]int{},
		priorityByPID:   map[int][]prioritySample{},
		timelineByKey:   map[string]TimelineResult{},
		resolvedByQuery: map[string]threadResolution{},
	}
	if idx == nil {
		return cache
	}
	for i := range idx.Events {
		ev := idx.Events[i]
		addEventPID := func(pid int) {
			if pid <= 0 {
				return
			}
			ids := cache.eventsByPID[pid]
			if len(ids) > 0 && ids[len(ids)-1] == i {
				return
			}
			cache.eventsByPID[pid] = append(ids, i)
		}
		addPriority := func(pid, prio int) {
			if pid <= 0 || prio <= 0 {
				return
			}
			cache.priorityByPID[pid] = append(cache.priorityByPID[pid], prioritySample{ts: ev.Ts, line: ev.Line, prio: prio})
		}
		switch ev.Type {
		case EventSchedSwitch:
			addEventPID(ev.PrevPID)
			addEventPID(ev.NextPID)
			addPriority(ev.PrevPID, ev.PrevPrio)
			addPriority(ev.NextPID, ev.NextPrio)
		case EventSchedWakeup, EventSchedWaking:
			addEventPID(ev.WakeePID)
			if ev.WakeePID > 0 {
				cache.wakeupsByPID[ev.WakeePID] = append(cache.wakeupsByPID[ev.WakeePID], i)
			}
			addPriority(ev.WakeePID, eventWakeePriorityForHardUse(ev))
		case EventSchedBlockedReason:
			if ev.WakeePID > 0 {
				cache.blockedByPID[ev.WakeePID] = append(cache.blockedByPID[ev.WakeePID], i)
			}
		}
	}
	for pid, samples := range cache.priorityByPID {
		sort.SliceStable(samples, func(i, j int) bool {
			if samples[i].ts != samples[j].ts {
				return samples[i].ts < samples[j].ts
			}
			return samples[i].line < samples[j].line
		})
		cache.priorityByPID[pid] = samples
	}
	return cache
}

func (c *chainQueryCache) resolveThreadSelection(q Query) threadResolution {
	if c == nil || c.idx == nil {
		return threadResolution{}
	}
	key := fmt.Sprintf("pid=%d/thread=%s/input=%s", q.PID, q.Thread, q.ThreadInput)
	if resolution, ok := c.resolvedByQuery[key]; ok {
		return resolution
	}
	resolution := resolveThreadSelection(c.idx, q)
	c.resolvedByQuery[key] = resolution
	return resolution
}

func (c *chainQueryCache) timeline(q Query, thread ThreadRef) TimelineResult {
	if c == nil || c.idx == nil {
		return TimelineResult{
			Thread:  thread,
			Window:  TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
			Caveats: []string{"trace index is empty"},
		}
	}
	key := fmt.Sprintf("%d/%s/%.9f/%.9f/%d/%d/%s", thread.PID, strings.ToLower(thread.Comm), q.TimeStart, q.TimeEnd, q.LineStart, q.LineEnd, q.TraceFlavor)
	if tl, ok := c.timelineByKey[key]; ok {
		return cloneTimelineResult(tl)
	}
	eventIDs, eventIndexed := c.eventsForThread(thread)
	blockedIDs, blockedIndexed := c.blockedReasonsForThread(thread)
	tl := threadTimelineForTarget(c.idx, q, thread, eventIDs, blockedIDs, eventIndexed && blockedIndexed)
	c.timelineByKey[key] = cloneTimelineResult(tl)
	return tl
}

func cloneTimelineResult(in TimelineResult) TimelineResult {
	out := in
	if len(in.Intervals) > 0 {
		out.Intervals = append([]Interval(nil), in.Intervals...)
	}
	if len(in.Caveats) > 0 {
		out.Caveats = append([]string(nil), in.Caveats...)
	}
	return out
}

func (c *chainQueryCache) eventsForThread(thread ThreadRef) ([]int, bool) {
	if c == nil || thread.PID <= 0 {
		return nil, false
	}
	ids := c.eventsByPID[thread.PID]
	if ids == nil {
		return []int{}, true
	}
	return ids, true
}

func (c *chainQueryCache) wakeupsForThread(thread ThreadRef) ([]int, bool) {
	if c == nil || thread.PID <= 0 {
		return nil, false
	}
	ids := c.wakeupsByPID[thread.PID]
	if ids == nil {
		return []int{}, true
	}
	return ids, true
}

func (c *chainQueryCache) blockedReasonsForThread(thread ThreadRef) ([]int, bool) {
	if c == nil || thread.PID <= 0 {
		return nil, false
	}
	ids := c.blockedByPID[thread.PID]
	if ids == nil {
		return []int{}, true
	}
	return ids, true
}

func (c *chainQueryCache) findWakeup(thread ThreadRef, start, end float64) (*Event, bool) {
	if c == nil || c.idx == nil {
		return nil, false
	}
	ids, indexed := c.wakeupsForThread(thread)
	return findWakeupForWithSelection(c.idx, thread, start, end, ids, indexed)
}

func (c *chainQueryCache) findBlockedReason(thread ThreadRef, start, end float64) *Event {
	if c == nil || c.idx == nil {
		return nil
	}
	ids, indexed := c.blockedReasonsForThread(thread)
	return findBlockedReasonForWithSelection(c.idx, thread, start, end, ids, indexed)
}

func (c *chainQueryCache) priorityNear(flavor TraceFlavor, thread ThreadRef, ts float64) (int, string) {
	if c == nil || c.idx == nil {
		return 0, ""
	}
	if thread.PID <= 0 {
		return threadPriorityNear(c.idx, flavor, thread, ts)
	}
	samples := c.priorityByPID[thread.PID]
	if len(samples) == 0 {
		return threadPriorityNear(c.idx, flavor, thread, ts)
	}
	scope := threadGenerationScopeAt(c.idx, thread.PID, ts, 0)
	if !scope.known {
		return 0, ""
	}
	first, last := 0, len(samples)
	if scope.hasStart {
		first = sort.Search(len(samples), func(i int) bool {
			return lifecyclePointAtOrAfter(samples[i].ts, samples[i].line, scope.start)
		})
	}
	if scope.hasEnd {
		last = sort.Search(len(samples), func(i int) bool {
			return lifecyclePointAtOrAfter(samples[i].ts, samples[i].line, scope.end)
		})
	}
	if first >= last {
		return 0, ""
	}
	pos := first + sort.Search(last-first, func(i int) bool {
		return samples[first+i].ts >= ts
	})
	bestPrio := 0
	bestDist := 0.0
	consider := func(i int) {
		if i < first || i >= last || samples[i].prio <= 0 || !scope.contains(samples[i].ts, samples[i].line) {
			return
		}
		dist := samples[i].ts - ts
		if dist < 0 {
			dist = -dist
		}
		if bestPrio == 0 || dist < bestDist {
			bestPrio = samples[i].prio
			bestDist = dist
		}
	}
	consider(pos)
	consider(pos - 1)
	return bestPrio, classifyTracePriority(flavor, bestPrio)
}

func expandChain(idx *Index, q Query, cache *chainQueryCache, thread ThreadRef, start, end float64, depth int, targetBlockedMs float64, visited map[int]bool, res *ChainResult, parentID string, consumers []ThreadRef, branch int) string {
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
	tl := cache.timeline(tq, thread)
	res.Caveats = dedupStrings(append(res.Caveats, tl.Caveats...))
	if tl.IntegrityFailure != "" {
		res.Caveats = append(res.Caveats, fmt.Sprintf("wakeup_chain_dependency_fail_closed=true; %s timeline integrity failure=%s; this branch is not converted into trace_gap evidence", threadLabel(thread), tl.IntegrityFailure))
		return ""
	}
	impact := summarizeWakeupCausalImpact(idx, q, cache, thread, tl.Intervals, start, end, depth, targetBlockedMs, res.Target, consumers)
	interesting := mostInterestingInterval(tl.Intervals, q.MinDurationMs)
	nodeID := fmt.Sprintf("n%d", len(res.Nodes)+1)
	// P0-E CHAIN-PATH (ledger §22.1): every node — nil-impact transits
	// included — carries its TRUE recursion depth and its owning branch, so
	// the serialization layer never has to guess either from a flat walk.
	node := ChainNode{ID: nodeID, Thread: thread, Window: TimeWindow{StartTs: start, EndTs: end}, Depth: depth, Branch: branch}
	if interesting != nil {
		node.Dominant = interesting.State
		node.DurationMs = interesting.DurationMs
		node.EvidenceLine = firstPositive(interesting.StartLine, interesting.WakeupLine, interesting.EndLine)
		node.Summary = interesting.Summary
	} else {
		// G2 判据 typed 化 (§27.2/§28.1, 2026-07-09): the two nil-interesting
		// shapes are DIFFERENT facts and must never share the over-strong
		// "no scheduler data" claim — a thread with a 0.051ms running interval
		// below the floor HAS scheduler data (the depth-0 exception channel may
		// even rank it in the same window). Precise typed split on the
		// thread's own timeline; the summary states the matching form only.
		node.Dominant = StateUnknown
		switch traceGapKindForTimeline(tl.Intervals) {
		case TraceGapKindNoSchedData:
			node.Summary = "no scheduler intervals for this thread inside the aligned window"
		default:
			// 复核 P3-5 wording narrowing: nil-interesting with intervals
			// present ⟺ EVERY interval sits below min_duration_ms (the
			// fallback admits any state at/above the floor, so a running
			// interval at/above it never reaches this arm) — say exactly that.
			node.Summary = "scheduler intervals exist in the aligned window but all sit below min_duration_ms (no eligible wait candidate)"
		}
	}
	impact.ChainBranch = branch
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
		res.RootEvidence = append(res.RootEvidence, RootEvidence{Type: "trace_gap", Thread: thread, Summary: node.Summary, Confidence: 0.6, GapKind: traceGapKindForTimeline(tl.Intervals)})
		return nodeID
	}
	switch interesting.State {
	case StateSSleep:
		wakeup, usedTolerance := cache.findWakeup(thread, interesting.StartTs, interesting.EndTs)
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
		childConsumers := append(append([]ThreadRef{}, consumers...), thread)
		childID := expandChain(idx, q, cache, waker, interesting.StartTs, wakeup.Ts, depth+1, targetBlockedMs, visited, res, nodeID, childConsumers, branch)
		if childID != "" {
			wakerPrio, wakerClass := cache.priorityNear(q.TraceFlavor, waker, wakeup.Ts)
			wakeePrio := eventWakeePriorityForHardUse(*wakeup)
			if wakeePrio <= 0 && wakeup.WakeePrioritySource() == "" {
				wakeePrio, _ = cache.priorityNear(q.TraceFlavor, thread, wakeup.Ts)
			}
			wakeeClass := classifyTracePriority(q.TraceFlavor, wakeePrio)
			relation := priorityRelation(q.TraceFlavor, wakeePrio, wakerPrio)
			res.Edges = append(res.Edges, WakeupEdge{
				From:                       childID,
				To:                         nodeID,
				Branch:                     branch,
				Waker:                      waker,
				Wakee:                      thread,
				WakeupTs:                   wakeup.Ts,
				WakeupLine:                 wakeup.Line,
				LatencyMs:                  (wakeup.Ts - interesting.StartTs) * 1000,
				WakerPriority:              wakerPrio,
				WakerPriorityClass:         wakerClass,
				WakeePriority:              wakeePrio,
				WakeePriorityClass:         wakeeClass,
				WakeePrioritySource:        wakeup.WakeePrioritySource(),
				PriorityRelation:           relation,
				PriorityInversionCandidate: relation == "lower_priority_waker",
				EvidenceLine:               wakeup.Line,
			})
		}
	case StateRunnable:
		res.RootEvidence = append(res.RootEvidence, rootEvidenceFromCausalImpact(impact, "thread was runnable but not running; inspect CPU pressure and priority context", 0.8))
	case StateDSleep, StateIOWait:
		root := rootEvidenceFromCausalImpact(impact, "thread slept in D state; IO or uninterruptible wait is a root-cause candidate", 0.88)
		if reason := cache.findBlockedReason(thread, interesting.StartTs, interesting.EndTs); reason != nil {
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

func summarizeWakeupCausalImpact(idx *Index, q Query, cache *chainQueryCache, thread ThreadRef, intervals []Interval, start, end float64, depth int, targetBlockedMs float64, target ThreadRef, consumers []ThreadRef) WakeupCausalImpact {
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
		actualDuration := it.ActualDurationMsResolved()
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
	item.Priority, item.PriorityClass = cache.priorityNear(q.TraceFlavor, thread, (start+end)/2)
	item.TargetPriority, item.TargetPriorityClass = cache.priorityNear(q.TraceFlavor, target, start)
	item.PriorityRelation = dependencyPriorityRelation(q.TraceFlavor, item.TargetPriority, item.Priority, depth)
	gateConsumers := consumers
	if len(gateConsumers) == 0 && thread.PID != target.PID {
		gateConsumers = []ThreadRef{target}
	}
	// CAP (§26): one memoized capability resolution serves both running folds
	// (R5d weak-core gate here, VS-2 supply fold below).
	capability := cache.coreCapability(q.CoreTopology)
	item.GatedRunnableMs, item.GatedRunningDeficitMs = priorityInversionGatedMs(cache, capability, gateConsumers, intervals)
	item.PriorityInversionGatedMs = item.GatedRunnableMs + item.GatedRunningDeficitMs
	if item.GatedRunningDeficitMs > 0 {
		// CAP (§26 C3): the discounted running component discloses its
		// capability caliber (typed three-state; wording input only).
		// CAP-2 (§28.4/§28.5): the cluster-topology source rides along
		// (empty on explicit/legacy — byte-preserving absence).
		item.GatedCapabilitySource = capability.source
		item.GatedClusterTopology = capability.topologySource
	}
	// R5d (§7.30.1): the inversion flag and its published impact key on the
	// GATED duration only — runnable time plus weak-core running time of the
	// dependency. Its own sleep/D/IO time never qualifies: that is the
	// dependency's own upstream problem and previously inflated inversion
	// rows into the top rank. A D/IO- or sleep-dominant dependency therefore
	// keeps its own root identity (io_wait / d_state / drilldown) at full
	// blocking magnitude; only rows whose story IS scheduling supply
	// (runnable- or running-dominant) surface as inversion candidates.
	item.PriorityInversionCandidate = item.PriorityRelation == "lower_priority_dependency" &&
		item.PriorityInversionGatedMs > 0 &&
		(item.DominantState == string(StateRunnable) || item.DominantState == string(StateRunning))
	// VS-2 (§7.10): an on-chain RUNNING-dominant node (typed triple gate —
	// never a heuristic) gets its running wall clock folded to the
	// big-cluster governed fmax so the report can separate running-SLOW
	// (supply-fold deficit, lower bound) from running-MUCH (true workload).
	// Non-nil SupplyFoldBasis is the presence signal; a deficit of exactly 0
	// with a fully-known basis IS the affirmative "ran at full frequency"
	// fact and must survive.
	if item.OnChain && item.DominantState == string(StateRunning) && item.RunningMs > item.RunnableMs {
		ideal, basis := cache.supplyFoldRunningIntervals(q, start, end, intervals)
		item.SupplyFoldIdealMs = ideal
		if deficit := item.RunningMs - ideal; deficit > 0 {
			item.SupplyFoldDeficitMs = deficit
		}
		item.SupplyFoldBasis = &basis
	}
	item.NextStep = causalImpactNextStep(item)
	item.NextStepKind = causalImpactNextStepKind(item)
	item.Summary = renderWakeupCausalImpactSummary(item)
	return item
}

// intervalActualDurationMs moved to Interval.ActualDurationMsResolved
// (types.go) — the single exported fallback authority shared with the
// tool-side timeline row face (PTV4 review finding, RTC-R1 2026-07-05).

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
	// Shared 5-lane pick (thread_state_universe.go) — same priority order the
	// inline candidates array always used.
	return dominantStateFromLanes(item.RunningMs, item.RunnableMs, item.SleepMs, item.DStateMs, item.IOWaitMs)
}

// priorityInversionGatedMs computes the R5d-gated inversion impact from the
// dependency thread's state intervals: RUNNABLE intervals count in full;
// RUNNING intervals count only when the dependency ran on a CPU whose
// equivalent capacity at that moment was below the capacity of ANY downstream
// chain consumer's CPU — the immediate wakee and every hop back to the focus
// thread (§7.30.1 rule 2: "被唤醒线程或者逐级回溯到用户关注线程任意一个").
// Frequency is sampled at the interval midpoint, so a DVFS ramp inside one
// interval attributes the whole interval to the midpoint state. Missing data
// — unknown interval CPU, no frequency samples, no locatable consumer CPU —
// contributes zero: the gate is conservative, never guessed.
//
// §7.30.3 D3: the two components return SEPARATELY (runnable full amount vs
// capacity-discounted weak-core running deficit) so the published composite
// can show its composition; the gated total is their sum.
//
// CAP (§26): capacity = frequency × class capability (core_capability.go);
// under freq_only fallback every coefficient is 1 and the comparison is the
// pre-CAP pure frequency ratio.
func priorityInversionGatedMs(cache *chainQueryCache, capability coreCapabilityMap, consumers []ThreadRef, intervals []Interval) (runnableMs, runningDeficitMs float64) {
	if cache == nil || len(consumers) == 0 {
		return 0, 0
	}
	for _, it := range intervals {
		if it.DurationMs <= 0 {
			continue
		}
		switch it.State {
		case StateRunnable:
			runnableMs += it.DurationMs
		case StateRunning:
			if !it.CPUKnown {
				continue
			}
			runningDeficitMs += cache.weakCoreDeficitMs(capability, consumers, it)
		}
	}
	return runnableMs, runningDeficitMs
}

// weakCoreDeficitMs integrates the R5d-2 capacity-proportional inversion
// impact of one RUNNING interval. The interval is sliced at every
// cpu_frequency change point of its own CPU (R5e: in-window frequency changes
// must be honored segment by segment, never one sample for the whole
// interval), and each slice contributes
//
//	sliceMs × max(0, 1 − (f_waker × cap_waker) / (f_consumerMax × cap_consumer))
//
// — the EXTRA time the same work would not have needed on the strongest known
// downstream consumer core, priced in equivalent capacity (CAP §26:
// cap = the CPU's core-class capability coefficient, 1 under the freq_only
// fallback — the pre-CAP pure frequency comparison). The strongest consumer is
// the one with the highest f × cap product. Counting the whole running slice
// would inflate the inversion exactly like counting whole sleeps did (§7.30.2
// R5d-2). Slices with unknown waker or consumer supply contribute zero.
//
// 复核 F2 (2026-07-08): under a usable capability map, UNKNOWN class
// membership on EITHER participating side degrades the whole slice to the
// PURE frequency comparison on BOTH sides. A silent cap=1 on the waker side
// alone is the AGGRESSIVE direction — an explicit topology that failed to
// declare the fastest core would understate that waker's equivalent capacity
// and mint deficit against a slower-but-declared big consumer (witness:
// 9.988ms fabricated vs a true 0), violating "missing data contributes zero,
// never a guess".
func (c *chainQueryCache) weakCoreDeficitMs(capability coreCapabilityMap, consumers []ThreadRef, it Interval) float64 {
	c.buildFreqIndex()
	boundaries := []float64{it.StartTs}
	for _, sample := range c.freqByCPU[it.CPU] {
		if sample.ts > it.StartTs && sample.ts < it.EndTs {
			boundaries = append(boundaries, sample.ts)
		}
	}
	boundaries = append(boundaries, it.EndTs)
	deficit := 0.0
	for i := 0; i+1 < len(boundaries); i++ {
		s0, s1 := boundaries[i], boundaries[i+1]
		if s1 <= s0 {
			continue
		}
		mid := (s0 + s1) / 2
		wakerFreq := c.frequencyAt(it.CPU, mid)
		if wakerFreq <= 0 {
			continue
		}
		type consumerSupply struct {
			freq float64
			cap  float64
		}
		wakerCap, classKnown := capability.capabilityForKnown(it.CPU)
		var supplies []consumerSupply
		for _, consumer := range consumers {
			cpu, ok := c.threadCPUNear(consumer, mid)
			if !ok {
				continue
			}
			f := c.frequencyAt(cpu, mid)
			if f <= 0 {
				continue
			}
			cap, known := capability.capabilityForKnown(cpu)
			classKnown = classKnown && known
			supplies = append(supplies, consumerSupply{freq: float64(f), cap: cap})
		}
		// 复核 F2: class pricing engages only when EVERY participating side's
		// membership is known (a freq_only map degrades wholesale by
		// construction — capabilityForKnown is then never true).
		wakerEquiv := float64(wakerFreq)
		if classKnown {
			wakerEquiv *= wakerCap
		}
		maxConsumerEquiv := 0.0
		for _, supply := range supplies {
			equiv := supply.freq
			if classKnown {
				equiv *= supply.cap
			}
			if equiv > maxConsumerEquiv {
				maxConsumerEquiv = equiv
			}
		}
		if maxConsumerEquiv <= 0 || wakerEquiv >= maxConsumerEquiv {
			continue
		}
		deficit += (s1 - s0) * 1000 * (1 - wakerEquiv/maxConsumerEquiv)
	}
	return deficit
}

func causalImpactBlockingMs(item WakeupCausalImpact) float64 {
	if dominantStateIsDStateOrIOWait(item.DominantState) {
		return item.DStateMs + item.IOWaitMs
	}
	return item.DominantImpactMs
}

func actualCausalImpactBlockingMs(item WakeupCausalImpact) float64 {
	if dominantStateIsDStateOrIOWait(item.DominantState) {
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

// causalImpactNextStepKind is the typed counterpart of causalImpactNextStep.
// Must stay in lockstep with the branch above.
func causalImpactNextStepKind(item WakeupCausalImpact) string {
	if item.PriorityInversionCandidate {
		return NextStepKindPriorityInversion
	}
	return stateChurnNextStepKind(item.DominantState)
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
	if item.PeriodicSource {
		// VS-1 (§7.8): periodic-source occurrences publish their cadence and
		// discounted attribution inline; in-period sleep is normal cadence.
		summary += fmt.Sprintf(" periodic_source=true detected_period=%.3fms lateness=%.3fms effective_impact=%.3fms", item.DetectedPeriodMs, item.LatenessMs, item.EffectivePeriodicImpactMs)
	}
	if item.SupplyFoldBasis != nil {
		// VS-2 (§7.10): the fold accounting prints explicitly, zeros included
		// — deficit 0 with a fully-known basis IS the affirmative fact.
		summary += fmt.Sprintf(" supply_fold_deficit=%.3fms supply_fold_ideal=%.3fms fold_basis_known=%.3fms fold_basis_unknown=%.3fms",
			item.SupplyFoldDeficitMs, item.SupplyFoldIdealMs, item.SupplyFoldBasis.KnownMs, item.SupplyFoldBasis.UnknownMs)
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
	// Shared root-type authority (thread_state_universe.go) — byte-identical
	// twin of the aggregate mapping.
	return rootTypeForDominantState(item.DominantState, item.PriorityInversionCandidate, item.IOWaitMs)
}

func threadPriorityNear(idx *Index, flavor TraceFlavor, thread ThreadRef, ts float64) (int, string) {
	if idx == nil || (thread.PID <= 0 && thread.Comm == "") {
		return 0, ""
	}
	scope := threadGenerationScope{known: true}
	if thread.PID > 0 {
		scope = threadGenerationScopeAt(idx, thread.PID, ts, 0)
		if !scope.known {
			return 0, ""
		}
	}
	bestPrio := 0
	bestDist := 0.0
	consider := func(ev Event, pid int, comm string, prio int) {
		if prio <= 0 || !threadMatches(thread, pid, comm) || (thread.PID > 0 && !scope.contains(ev.Ts, ev.Line)) {
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
			consider(ev, ev.WakeePID, ev.WakeeComm, eventWakeePriorityForHardUse(ev))
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
		// Only the documented Harmony userspace bands share an ordering
		// semantics. Values outside 1..159 are explicitly classified as
		// system_or_kernel/raw; comparing either side numerically would turn an
		// opaque scheduler token into a hard lower/higher dependency claim.
		if !harmonyUserPriorityComparable(targetPrio) || !harmonyUserPriorityComparable(dependencyPrio) {
			return "raw_priority_uninterpreted"
		}
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
		// The lower-priority-waker token is the precise gate that mints a
		// priority-inversion candidate. Require both endpoints to belong to the
		// documented, comparable ohos_cfs/ohos_rt closed set; a raw/system
		// priority may still be displayed, but can never mint the candidate.
		if !harmonyUserPriorityComparable(wakeePrio) || !harmonyUserPriorityComparable(wakerPrio) {
			return "raw_priority_uninterpreted"
		}
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

func harmonyUserPriorityComparable(prio int) bool {
	switch classifyTracePriority(TraceFlavorHarmonyHitrace, prio) {
	case "ohos_cfs", "ohos_rt":
		return true
	default:
		return false
	}
}

func mostInterestingInterval(intervals []Interval, minDurationMs float64) *Interval {
	candidates, _ := interestingIntervals(intervals, minDurationMs, 1)
	if len(candidates) == 0 {
		return nil
	}
	return &candidates[0]
}

// traceGapKindForTimeline (G2 判据 typed 化, ledger §27.2 + §28.1 user ruling
// 2026-07-09, real_trace_campaign_20260705.md) is the PRECISE criterion behind
// a trace_gap mint, decided from the thread's own timeline inside the aligned
// window. mostInterestingInterval returns nil ⟺ no interval reaches the
// MinDurationMs floor (its fallback arm admits ANY state at/above the floor,
// Running included), so exactly two nil shapes exist:
//   - the timeline holds NO interval at all      → TraceGapKindNoSchedData
//     (the only shape the legacy "窗内无调度数据" wording was true for);
//   - intervals exist but all sit below the floor → TraceGapKindNoEligibleWait
//     (sub-threshold fragments of any state — real scheduler data; the same
//     (thread, window) may legitimately carry a depth-0 running rank row, the
//     §27.2 OS_FFRT self-contradiction witness).
//
// Precise integer signal (len) only — never a prose/summary heuristic.
func traceGapKindForTimeline(intervals []Interval) string {
	if len(intervals) == 0 {
		return TraceGapKindNoSchedData
	}
	return TraceGapKindNoEligibleWait
}

// interestingIntervals returns up to max intervals worth recursing into
// (highest duration/state priority first, Running intervals excluded since
// they are handled by the independent compute-supply candidate stream), and
// the total qualifying count before truncation so callers that recurse on
// the result (buildWakeupChainWithCache) can surface a caveat when
// candidates were silently dropped rather than expanded.
func interestingIntervals(intervals []Interval, minDurationMs float64, max int) ([]Interval, int) {
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
	qualifying := len(out)
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
	return out, qualifying
}

func findWakeupFor(idx *Index, thread ThreadRef, start, end float64) (*Event, bool) {
	return findWakeupForWithSelection(idx, thread, start, end, nil, false)
}

func findWakeupForWithSelection(idx *Index, thread ThreadRef, start, end float64, eventIDs []int, useIndexedEvents bool) (*Event, bool) {
	if idx == nil {
		return nil, false
	}
	var best *Event
	usedTolerance := false
	visit := func(i int) {
		if i < 0 || i >= len(idx.Events) {
			return
		}
		ev := &idx.Events[i]
		if ev.Type != EventSchedWakeup && ev.Type != EventSchedWaking {
			return
		}
		// sched_wakeup_new creates a new occupant of the numeric TID.  It is
		// never evidence that the creator woke a sleep interval belonging to the
		// previous occupant, and must not mint a cross-generation causal edge or
		// counterpart fallback.
		if schedWakeupStartsNewIncarnation(*ev) {
			return
		}
		if ev.Ts < start {
			return
		}
		inStrict := ev.Ts <= end
		if !inStrict && ev.Ts > end+wakeupMatchToleranceSec {
			return
		}
		if threadMatches(thread, ev.WakeePID, ev.WakeeComm) {
			if best == nil || eventLaterThan(*ev, *best) {
				best = ev
				usedTolerance = !inStrict
			}
		}
	}
	if useIndexedEvents {
		for _, id := range eventIDs {
			visit(id)
		}
	} else {
		for i := range idx.Events {
			visit(i)
		}
	}
	return best, usedTolerance
}

func findBlockedReasonFor(idx *Index, thread ThreadRef, start, end float64) *Event {
	return findBlockedReasonForWithSelection(idx, thread, start, end, nil, false)
}

func findBlockedReasonForWithSelection(idx *Index, thread ThreadRef, start, end float64, eventIDs []int, useIndexedEvents bool) *Event {
	if idx == nil {
		return nil
	}
	var best *Event
	visit := func(i int) {
		if i < 0 || i >= len(idx.Events) {
			return
		}
		ev := &idx.Events[i]
		if ev.Type != EventSchedBlockedReason {
			return
		}
		if ev.Ts < start || ev.Ts > end {
			return
		}
		if threadMatches(thread, ev.WakeePID, "") {
			if best == nil || eventLaterThan(*ev, *best) {
				best = ev
			}
		}
	}
	if useIndexedEvents {
		for _, id := range eventIDs {
			visit(id)
		}
	} else {
		for i := range idx.Events {
			visit(i)
		}
	}
	return best
}

func eventLaterThan(candidate, current Event) bool {
	return candidate.Ts > current.Ts || (candidate.Ts == current.Ts && candidate.Line > current.Line)
}

// visitEventsInTimestampOrder is the shared state-machine iterator.  Callers
// first require schedulerStateOrderViolationForQuery == nil; with every
// consumed PID/CPU lane proven monotonic, physical interleaving across
// independent lanes is safe and preserves zero-allocation trace order.  We do
// not globally sort a clock rollback into a fabricated elapsed timeline.
func visitEventsInTimestampOrder(idx *Index, eventIDs []int, useIndexedEvents bool, visit func(Event)) {
	if idx == nil || visit == nil {
		return
	}
	if useIndexedEvents {
		for _, id := range eventIDs {
			if id >= 0 && id < len(idx.Events) {
				visit(idx.Events[id])
			}
		}
		return
	}
	for _, ev := range idx.Events {
		visit(ev)
	}
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

func loadRawArtifactLines(idx *Index, events []Event) (map[int]string, map[int]string) {
	out := map[int]string{}
	issues := map[int]string{}
	if idx == nil || len(events) == 0 {
		return out, issues
	}
	// source path -> local line -> virtual line(s). Virtual lines are unique,
	// while local lines deliberately are not across bundle children.
	wants := map[string]map[int][]int{}
	for _, ev := range events {
		spans := idx.ResolveArtifactSpans(ev.Line, ev.Line)
		if len(spans) != 1 {
			// Compatibility for hand-built/legacy Index values that predate the
			// source ledger. Production parsers always populate TraceArtifacts.
			if len(idx.TraceArtifacts) == 0 && strings.TrimSpace(idx.Path) != "" && ev.Line > 0 {
				if wants[idx.Path] == nil {
					wants[idx.Path] = map[int][]int{}
				}
				wants[idx.Path][ev.Line] = append(wants[idx.Path][ev.Line], ev.Line)
			}
			continue
		}
		span := spans[0]
		if wants[span.SourcePath] == nil {
			wants[span.SourcePath] = map[int][]int{}
		}
		wants[span.SourcePath][span.LocalLineStart] = append(wants[span.SourcePath][span.LocalLineStart], ev.Line)
	}
	for path, want := range wants {
		markSourceIssue := func(reason string, discard bool) {
			for _, virtualLines := range want {
				for _, virtualLine := range virtualLines {
					if discard {
						delete(out, virtualLine)
					}
					issues[virtualLine] = reason
				}
			}
		}
		f, err := os.Open(path)
		if err != nil {
			markSourceIssue("artifact_open_failed", false)
			continue
		}
		// Validate the exact opened descriptor, not a pathname stat performed
		// before Open. This closes the stat->open replacement race: the identity
		// we approve is the same file object from which raw lines are scanned.
		source, sourceOK := traceArtifactSourceForPath(idx.TraceArtifacts, path)
		openedInfo, statErr := f.Stat()
		if statErr != nil || (sourceOK && !source.identityMatchesInfo(openedInfo)) {
			_ = f.Close()
			markSourceIssue("artifact_identity_changed", true)
			continue
		}
		openedIdentity := traceFileIdentityFromInfo(openedInfo)
		sc := bufioNewScanner(f)
		lineNo := 0
		found := 0
		for sc.Scan() {
			lineNo++
			virtualLines := want[lineNo]
			if len(virtualLines) == 0 {
				continue
			}
			for _, virtualLine := range virtualLines {
				out[virtualLine] = sc.Text()
			}
			found++
			if found == len(want) {
				break
			}
		}
		finalInfo, finalStatErr := f.Stat()
		identityChanged := finalStatErr != nil || !openedIdentity.matchesInfo(finalInfo)
		if !identityChanged && sourceOK {
			identityChanged = !source.identityMatchesInfo(finalInfo)
		}
		scanErr := sc.Err()
		_ = f.Close()
		if identityChanged {
			markSourceIssue("artifact_identity_changed", true)
			continue
		}
		if scanErr != nil {
			markSourceIssue("artifact_read_failed", true)
			continue
		}
		for _, virtualLines := range want {
			for _, virtualLine := range virtualLines {
				if _, ok := out[virtualLine]; !ok {
					issues[virtualLine] = "artifact_line_unavailable"
				}
			}
		}
	}
	return out, issues
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
	for _, td := range stats.SleepTop {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(td.Thread),
			Predicate:  "sleep_wait",
			Summary:    fmt.Sprintf("%s spent %.3f ms sleeping before wakeup in the selected window%s", threadLabel(td.Thread), td.DurationMs, durationCPUDetail(td)),
			LineStart:  td.LineStart,
			LineEnd:    td.LineEnd,
			StartTs:    td.StartTs,
			EndTs:      td.EndTs,
			Confidence: 0.76,
		})
		if len(out) >= 16 {
			break
		}
	}
	for _, td := range stats.IOWaitTop {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(td.Thread),
			Predicate:  "io_wait",
			Summary:    fmt.Sprintf("%s spent %.3f ms in IO wait in the selected window%s", threadLabel(td.Thread), td.DurationMs, durationCPUDetail(td)),
			LineStart:  td.LineStart,
			LineEnd:    td.LineEnd,
			StartTs:    td.StartTs,
			EndTs:      td.EndTs,
			Confidence: 0.82,
		})
		if len(out) >= 18 {
			break
		}
	}
	for _, td := range stats.DStateTop {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(td.Thread),
			Predicate:  "d_state_or_io_wait",
			Summary:    fmt.Sprintf("%s spent %.3f ms in non-IO D-state wait in the selected window%s", threadLabel(td.Thread), td.DurationMs, durationCPUDetail(td)),
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
	for _, fence := range stats.DMAFenceActivity {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(fence.Thread),
			Predicate:  "dma_fence_activity",
			Object:     firstNonEmpty(fence.Timeline, fence.Driver, fence.Seqno),
			Summary:    fence.Summary,
			LineStart:  fence.LineStart,
			LineEnd:    fence.LineEnd,
			StartTs:    fence.StartTs,
			EndTs:      fence.EndTs,
			Confidence: 0.64,
		})
		if len(out) >= 47 {
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
			Summary:    fmt.Sprintf("IRQ burst inventory %s irq=%d on cpu=%d had %d event(s) across a %.3f ms span; span is not active duration", burst.Name, burst.IRQ, burst.CPU, burst.Count, burst.SpanMs),
			LineStart:  burst.LineStart,
			LineEnd:    burst.LineEnd,
			StartTs:    burst.StartTs,
			EndTs:      burst.EndTs,
			Confidence: 0.58,
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
	// RN-15 (§7.4/§7.9): per-thread stats.ComputeSupply verdict rows are NOT
	// republished as Predicate=compute_supply evidence facts — the same
	// runnable/running durations already ride the runnable_wait/running_time
	// facts above, and the compute_supply observation family is reserved for
	// the aggregate delivery-side ledger (compute_supply_balance). The
	// per-thread verdict surface stays visible in the window_stats section.
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
	rows := make([]RootCauseRankItem, 0, len(rank.Items)+len(rank.AbsorbedItems))
	rows = append(rows, rank.Items...)
	rows = append(rows, rank.AbsorbedItems...)
	for _, item := range rows {
		summary := item.Summary
		if perf := rootCausePerfSummary(item.PerfContext); perf != "" {
			summary = fmt.Sprintf("%s; %s", summary, perf)
		}
		if perfRoles := rootCausePerfRoleSummary(item.PerfContexts, 3); perfRoles != "" {
			summary = fmt.Sprintf("%s; perf_role_contexts=%s", summary, perfRoles)
		}
		// G9 (2026-07-09, 复核 P1-2 narrowed): demoted rows (target_self_state
		// / data_gap) carry no board ordinal — the evidence face says so
		// instead of fabricating a "#0" seat.
		// UXR-1 (§29.36.2): the ordinal is channel-scoped — an adjacent row's
		// seat is the 邻近影响 channel's #N, never the root-cause board's; the
		// evidence face names the channel so two channels' #1 cannot collide.
		position := fmt.Sprintf("%s cause #%d", item.Tier, item.Rank)
		if item.Rank > 0 && rootCauseOrdinalChannel(item) == rootCauseOrdinalChannelAdjacent {
			position = fmt.Sprintf("%s adjacent-impact #%d", item.Tier, item.Rank)
		}
		if item.Rank <= 0 {
			position = fmt.Sprintf("%s row (no rank seat)", item.Tier)
		}
		if item.AbsorbedByRankFamily {
			position = fmt.Sprintf("absorbed row (no rank seat; absorbed_into=%s)", item.AbsorbedIntoFamily)
		}
		out = append(out, EvidenceFact{
			Subject:    threadLabel(item.Thread),
			Predicate:  "root_cause_" + item.Tier,
			Object:     item.Type,
			Summary:    fmt.Sprintf("%s (%s): %s", position, item.Type, summary),
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

type runnableInversionOverlapWitness struct {
	competitor     ThreadDuration
	targetPrio     int
	competitorPrio int
	durationMs     float64
	interval       timeInterval
	intervalExact  bool
}

// runnableInversionWitnessInterval returns a union-safe interval only when
// the typed envelope and typed overlap duration prove one contiguous overlap.
// SameCPUTopRunning may aggregate fragmented runs of one competitor; its
// StartTs..EndTs is then only an envelope and must not be treated as elapsed
// overlap. Those rows remain usable through the conservative MAX lane below.
func runnableInversionWitnessInterval(td, competitor ThreadDuration, durationMs float64) (timeInterval, bool) {
	if durationMs <= 0 || competitor.EndTs <= competitor.StartTs {
		return timeInterval{}, false
	}
	start, end := competitor.StartTs, competitor.EndTs
	if td.EndTs > td.StartTs {
		var overlaps bool
		start, end, overlaps = overlapTimeWindow(start, end, td.StartTs, td.EndTs)
		if !overlaps {
			return timeInterval{}, false
		}
	}
	envelopeMs := (end - start) * 1000
	tolerance := math.Max(1e-6, math.Max(envelopeMs, durationMs)*1e-9)
	if math.Abs(envelopeMs-durationMs) > tolerance {
		return timeInterval{}, false
	}
	return timeInterval{start: start, end: end}, true
}

// conservativeRunnableInversionOverlap uses a true interval union only for
// contiguous overlap witnesses. Any aggregated/unknown-time witness is mixed
// in by MAX, never by summation: without a disjointness proof, adding it to
// another competitor would manufacture wall-clock time. The result is always
// bounded by the raw runnable wait represented by the rank row.
func conservativeRunnableInversionOverlap(witnesses []runnableInversionOverlapWitness, rawRunnableMs float64) (float64, string) {
	if len(witnesses) == 0 || rawRunnableMs <= 0 {
		return 0, ""
	}
	maxObservedMs := 0.0
	intervals := make([]timeInterval, 0, len(witnesses))
	for _, witness := range witnesses {
		if witness.durationMs > maxObservedMs {
			maxObservedMs = witness.durationMs
		}
		if witness.intervalExact {
			intervals = append(intervals, witness.interval)
		}
	}
	unionMs := 0.0
	if len(intervals) > 0 {
		sort.SliceStable(intervals, func(i, j int) bool {
			if intervals[i].start != intervals[j].start {
				return intervals[i].start < intervals[j].start
			}
			return intervals[i].end < intervals[j].end
		})
		start, end := intervals[0].start, intervals[0].end
		for _, interval := range intervals[1:] {
			if interval.start <= end {
				if interval.end > end {
					end = interval.end
				}
				continue
			}
			unionMs += (end - start) * 1000
			start, end = interval.start, interval.end
		}
		unionMs += (end - start) * 1000
	}
	impactMs := math.Max(maxObservedMs, unionMs)
	caliber := "typed_overlap_max"
	if unionMs > maxObservedMs {
		caliber = "typed_interval_union"
	} else if len(witnesses) == 1 {
		caliber = "typed_overlap_single"
	}
	if impactMs > rawRunnableMs {
		impactMs = rawRunnableMs
		caliber += "_bounded"
	}
	return impactMs, caliber
}

// applyRunnableTopPriorityInversion reclassifies a direct runnable_wait
// candidate only when typed same-CPU running overlap belongs to a strictly
// lower-priority dependency. The raw runnable wait remains on ImpactMs /
// CumulativeImpactMs / RunnableMs for disclosure; EffectiveImpactMs and Score
// carry only the provable inversion overlap. This prevents a 2ms lower-prio
// displacement from turning an unrelated 30ms runnable envelope into 30ms of
// priority-inversion impact.
func applyRunnableTopPriorityInversion(idx *Index, q Query, stats WindowStats, td ThreadDuration, item *RootCauseRankItem) {
	if item == nil || td.DurationMs <= 0 {
		return
	}
	var witnesses []runnableInversionOverlapWitness
	competitorKeys := map[string]bool{}
	for _, ctx := range stats.RunnableContext {
		if !sameThreadRef(ctx.Thread, td.Thread) || ctx.CPU != td.CPU {
			continue
		}
		for _, competitor := range ctx.SameCPUTopRunning {
			if competitor.DurationMs <= 0 || sameThreadRef(competitor.Thread, td.Thread) {
				continue
			}
			ts := td.StartTs
			if competitor.EndTs > competitor.StartTs {
				ts = (competitor.StartTs + competitor.EndTs) / 2
			} else if td.EndTs > td.StartTs {
				ts = (td.StartTs + td.EndTs) / 2
			}
			targetPrio := ctx.Priority
			if targetPrio <= 0 {
				targetPrio = td.Priority
			}
			if targetPrio <= 0 {
				targetPrio, _ = threadPriorityNear(idx, q.TraceFlavor, td.Thread, ts)
			}
			competitorPrio := competitor.Priority
			if competitorPrio <= 0 {
				competitorPrio, _ = threadPriorityNear(idx, q.TraceFlavor, competitor.Thread, ts)
			}
			if dependencyPriorityRelation(q.TraceFlavor, targetPrio, competitorPrio, 1) != "lower_priority_dependency" {
				continue
			}
			durationMs := math.Min(competitor.DurationMs, td.DurationMs)
			interval, exact := runnableInversionWitnessInterval(td, competitor, durationMs)
			witnesses = append(witnesses, runnableInversionOverlapWitness{
				competitor: competitor, targetPrio: targetPrio, competitorPrio: competitorPrio,
				durationMs: durationMs, interval: interval, intervalExact: exact,
			})
			competitorKeys[threadKey(competitor.Thread)] = true
		}
	}
	impactMs, caliber := conservativeRunnableInversionOverlap(witnesses, td.DurationMs)
	if impactMs <= 0 {
		return
	}
	sort.SliceStable(witnesses, func(i, j int) bool {
		if witnesses[i].durationMs != witnesses[j].durationMs {
			return witnesses[i].durationMs > witnesses[j].durationMs
		}
		if witnesses[i].competitor.LineStart != witnesses[j].competitor.LineStart {
			return witnesses[i].competitor.LineStart < witnesses[j].competitor.LineStart
		}
		return threadKey(witnesses[i].competitor.Thread) < threadKey(witnesses[j].competitor.Thread)
	})
	primary := witnesses[0]
	item.Type = "priority_inversion_runnable_wait"
	item.EffectiveImpactMs = impactMs
	// This retype's inversion algorithm is pure runnable overlap; publish the
	// same scalar on the gated component lane so family folding and report
	// decomposition preserve total == runnable + running_deficit.
	item.GatedRunnableMs = impactMs
	item.GatedRunningDeficitMs = 0
	// §20 B/D-Gap④ (state_churn precedent §7.30 S1, 2026-07-07): a retyped
	// row re-passes the causal-token registry guard and re-derives Score from
	// the measured inversion-overlap ranking channel — never the raw wait.
	assertCausalTokenRow(item.Type, item.Thread, "root_cause_rank")
	item.Score = item.EffectiveImpactMs * item.Confidence * rootCauseItemScoreWeight(*item)
	item.Summary = fmt.Sprintf("%s; same_cpu_competitor=%s has lower priority (target_prio=%d competitor_prio=%d); raw_runnable_wait=%.3fms inversion_overlap=%.3fms overlap_caliber=%s lower_priority_competitors=%d — priority inversion candidate",
		item.Summary, threadLabel(primary.competitor.Thread), primary.targetPrio, primary.competitorPrio,
		td.DurationMs, item.EffectiveImpactMs, caliber, len(competitorKeys))
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
		// Writeback error-sequence rows are searchable observations, not causal
		// evidence.  Keep the returned EventView intact while preventing both
		// indexed and streaming event_search (which share this function) from
		// minting a generic filesystem EvidenceFact for the same observation.
		if isWritebackObservation(ev.Event) {
			continue
		}
		if EROFSCoverageOnlyNameCandidate(ev.Name) {
			continue
		}
		// MMC near names and exact-name rows whose closed body profile failed
		// remain searchable inventory. They must not regain semantic authority
		// through event_search's generic EvidenceFact publisher.
		if mmcPairingNameCandidate(ev.Name) && !mmcSemanticPayloadAdmitted(ev.Event) {
			continue
		}
		if F2FSClosedEndpointNameCandidate(ev.Name) && !f2fsSemanticPayloadAdmitted(ev.Event) {
			continue
		}
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
	// Streaming event_search deliberately does not materialize rows into the
	// index.  A non-empty result is therefore positive parse evidence even when
	// idx.ParsedKnown remains zero; do not claim the trace has no parsed rows.
	if idx != nil && idx.ParsedKnown == 0 && len(res.Events) == 0 {
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
		out = append(out, cpuInputIntegrityCaveats(idx, q)...)
		out = append(out, traceMarkIntegrityCaveats(idx, q)...)
	}
	if idx != nil && idx.Windowed {
		// QF5: a padding-tail-truncated build (PaddingTruncated) stopped
		// parsing at PaddingTruncatedLastTs, strictly inside the padded
		// bound — this caveat must report the REAL parse boundary, or it
		// contradicts the PaddingTruncatedNote caveat emitted alongside and
		// claims unparsed padding as parsed coverage.
		parsedEnd := idx.IndexTimeEnd
		truncated := ""
		if idx.PaddingTruncated && idx.PaddingTruncatedLastTs > 0 && idx.PaddingTruncatedLastTs < parsedEnd {
			parsedEnd = idx.PaddingTruncatedLastTs
			truncated = "; padding tail truncated at the event budget — nothing after that boundary was parsed"
		}
		out = append(out, fmt.Sprintf("windowed_index_parse=true; parsed a bounded trace slice before running the view (time %.6f..%.6f seconds, lines %d..%d)%s. If the answer needs state far before this window, rerun with a wider time/line window or omit the window to build the full index.", idx.IndexTimeStart, parsedEnd, idx.IndexLineStart, idx.IndexLineEnd, truncated))
	}
	if selector := threadSelectorSummary(firstNonEmpty(q.ThreadInput, q.Thread)); selector != "" {
		if q.ThreadPIDInferred {
			out = append(out, "thread selector normalized from model/customer text: "+selector+"; pid-bearing scheduler fields are used for matching")
		}
	}
	if caveat := threadResolutionCaveat(idx, q); caveat != "" {
		out = append(out, caveat)
	}
	if caveat := threadSelectorSpanNameCaveat(idx, q, res); caveat != "" {
		out = append(out, caveat)
	}
	if res.View == "event_search" && len(res.Events) == 0 {
		out = append(out, "matched_events=0 for the selected filters; this is not absence proof if the thread label, time window, event types, or line window are too narrow")
		if pattern := strings.TrimSpace(q.Pattern); pattern != "" {
			out = append(out, fmt.Sprintf("pattern_no_match_hint=pattern %q is a literal substring, not a regex; try one shorter exact frame id/span label/marker token/symbol/DSO/callchain/source/symbolization_status/callchain_status/clock_confidence/cpu_known fragment, add event_types=[\"trace_mark\"] for B/E/C/S/F/G/H/N/I marker rows or event_types=[\"perf_sample\"] for CPU sample rows, or remove over-narrow pid/thread/time filters before falling back to grep/read_file; for B/E spans, do not search E|<pid>|<span_name> because end rows are unnamed E|<pid> or bare E on the same ftrace thread stack", pattern))
			if len(q.EventTypes) == 0 {
				out = append(out, fmt.Sprintf("next_pattern_call_hint=try trace_query(view=\"event_search\", pattern=%q, event_types=[\"trace_mark\"], time_start=%.6f, time_end=%.6f, limit=40), trace_query(view=\"event_search\", pattern=%q, event_types=[\"perf_sample\"], time_start=%.6f, time_end=%.6f, limit=40), or trace_query(view=\"span_window\", span_name=\"<span label>\", line_start=<line>, line_end=<line>) after selecting a line window", pattern, q.TimeStart, q.TimeEnd, pattern, q.TimeStart, q.TimeEnd))
			} else {
				// D-diag B-1 (§16): gating INVERSION. The recovery hint used to
				// fire only when event_types was EMPTY — exactly backwards: a
				// model that narrowed event_types and got zero matches is the
				// caller most in need of "the type filter itself may be wrong"
				// guidance (q7 burned five rounds on a pattern whose rows parse
				// under a different type than the one it selected). Gate is
				// precise: non-empty event_types AND zero matched rows. The
				// bounded cross-type recount below turns the generic advice
				// into a counted fact when the pattern does match rows of other
				// types under the same window/thread filters.
				if hits := crossTypePatternHits(idx, q); hits.total > 0 {
					out = append(out, fmt.Sprintf("cross_type_pattern_hint=pattern %q does match %s row(s) in the same window and filters whose event type is outside event_types=%s (top types: %s); the event_types filter is what excluded them — drop event_types and rerun, or use trace_query(view=\"span_window\", span_name=\"<span label>\") when the match is a span label", pattern, hits.countLabel(), formatEventTypesFilter(q.EventTypes), hits.topLabel()))
				}
				out = append(out, fmt.Sprintf("next_pattern_call_hint=event_types=%s matched nothing for this pattern and the type filter itself may be excluding the rows; retry without event_types: trace_query(view=\"event_search\", pattern=%q, time_start=%.6f, time_end=%.6f, limit=40), or trace_query(view=\"span_window\", span_name=\"<span label>\", line_start=<line>, line_end=<line>) after selecting a line window", formatEventTypesFilter(q.EventTypes), pattern, q.TimeStart, q.TimeEnd))
			}
		}
		if q.PID > 0 {
			out = append(out, fmt.Sprintf("next_call_hint=try trace_query(view=\"thread_timeline\", pid=%d, time_start=%.6f, time_end=%.6f) or trace_query(view=\"wakeup_chain\", pid=%d, time_start=%.6f, time_end=%.6f)", q.PID, q.TimeStart, q.TimeEnd, q.PID, q.TimeStart, q.TimeEnd))
		} else if strings.TrimSpace(q.Thread) != "" {
			out = append(out, fmt.Sprintf("next_call_hint=try trace_query(view=\"event_search\", thread=%q, time_start=%.6f, time_end=%.6f, event_types=[\"sched_switch\",\"sched_wakeup\"]) or use pid if visible in the trace row", q.Thread, q.TimeStart, q.TimeEnd))
		}
	}
	if res.View == "event_search" {
		// Audit #37 (§29.25 处置委托 2026-07-10): RawUnavailableReason is a typed
		// five-value enum; collapsing every value into the identity-mismatch
		// prose misdescribed open/read/line failures and the clock_inverse
		// precision case (whose raw text is not even withheld). Emit one
		// reason-keyed caveat per distinct reason present.
		seenRawIssues := map[string]bool{}
		for _, event := range res.Events {
			reason := event.RawUnavailableReason
			if reason == "" || seenRawIssues[reason] {
				continue
			}
			seenRawIssues[reason] = true
			switch reason {
			case "artifact_identity_changed":
				out = append(out, "raw_artifact_identity_mismatch=true; raw source text was withheld because the physical artifact no longer matches the index-time size/mtime identity; rebuild the trace index before auditing raw rows")
			case "artifact_open_failed":
				out = append(out, "raw_artifact_open_failed=true; raw source text was withheld because the physical artifact could not be opened (moved/deleted/permission); restore the artifact at its indexed path or rebuild the trace index before auditing raw rows")
			case "artifact_read_failed":
				out = append(out, "raw_artifact_read_failed=true; raw source text was withheld because reading the physical artifact failed (I/O error or oversized line); the indexed event fields remain valid")
			case "artifact_line_unavailable":
				out = append(out, "raw_artifact_line_unavailable=true; raw source text was withheld because the recorded line number lies beyond the current artifact content; rebuild the trace index before auditing raw rows")
			case "clock_inverse_unsafe":
				out = append(out, "raw_source_ts_inverse_unsafe=true; this artifact's affine clock mapping cannot losslessly invert the canonical timestamp for the affected rows, so no artifact-local source_ts could be derived: their source_ts field carries the CANONICAL value and clock_aligned stays false; raw text and canonical timestamps remain valid")
			default:
				out = append(out, fmt.Sprintf("raw_source_unavailable=true; reason=%s; raw source text for the affected rows could not be served from the physical artifact", reason))
			}
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
	out = append(out, traceWindowStrategyCaveats(q, res)...)
	out = append(out, traceCompletenessCaveats(idx, q, res)...)
	if caveat := capabilitySplitAuditCaveat(res); caveat != "" {
		out = append(out, caveat)
	}
	return out
}

// capabilitySplitAuditCaveat (CAP-3 复核 P2, §29.11) lifts the FIRST
// fragmentation split-audit found on any fold basis of this result into ONE
// engine caveat line, so tracediag replays and tool consumers see WHERE the
// co-movement criterion split behind a freq_only degrade. Disclosure only —
// the wording says so explicitly and no gate reads it; "" when no basis
// carries an audit (non-freq_only results and the non-fragmentation freq_only
// arms stay caveat-silent, absence preserves every existing byte).
func capabilitySplitAuditCaveat(res Result) string {
	audit := ""
	scanBasis := func(basis *SupplyFoldBasis) {
		if audit == "" && basis != nil && basis.CapabilitySplitAudit != "" {
			audit = basis.CapabilitySplitAudit
		}
	}
	scanChain := func(chain *ChainResult) {
		if chain == nil {
			return
		}
		for i := range chain.CausalImpacts {
			scanBasis(chain.CausalImpacts[i].SupplyFoldBasis)
		}
		for i := range chain.AggregatedImpacts {
			scanBasis(chain.AggregatedImpacts[i].SupplyFoldBasis)
		}
	}
	scanRank := func(rank *RootCauseRankResult) {
		if rank == nil {
			return
		}
		for i := range rank.Items {
			scanBasis(rank.Items[i].SupplyFoldBasis)
		}
	}
	scanChain(res.WakeupChain)
	scanRank(res.RootCauseRank)
	if res.FrameRootCauseBundle != nil {
		scanChain(res.FrameRootCauseBundle.WakeupChain)
		scanRank(res.FrameRootCauseBundle.RootCauseRank)
	}
	if audit == "" {
		return ""
	}
	return "capability_freq_only_split_audit=" + audit +
		" — 簇结构不可判(freq_only)降级的首个共动分裂点定位;仅披露/审计用,不参与任何判定 (first co-movement split behind the freq_only capability degrade; disclosure/audit only, never a gate)"
}

// crossTypeRescan bounds (D-diag B-1, §16). The zero-match query already paid
// one full filtered pass over the SAME in-window rows, so the recount can never
// exceed the cost class of the search that triggered it; the explicit in-window
// row budget plus the match cap additionally keep it a bounded sample on giant
// windows — never an unbounded second full-trace scan. Rows outside the
// line/time window cost only the cheap eventInQueryWindow comparisons and do
// not consume the budget.
const crossTypeRescanInWindowEventBudget = 250000
const crossTypeRescanMatchCap = 200

type crossTypeHitCount struct {
	Type  EventType
	Count int
}

type crossTypeHitSummary struct {
	total     int
	truncated bool
	top       []crossTypeHitCount
}

func (s crossTypeHitSummary) countLabel() string {
	if s.truncated {
		return fmt.Sprintf("at least %d (count stopped early)", s.total)
	}
	return fmt.Sprintf("%d", s.total)
}

func (s crossTypeHitSummary) topLabel() string {
	limit := len(s.top)
	if limit > 3 {
		limit = 3
	}
	parts := make([]string, 0, limit)
	for _, h := range s.top[:limit] {
		parts = append(parts, fmt.Sprintf("%s:%d", string(h.Type), h.Count))
	}
	return strings.Join(parts, ", ")
}

// crossTypePatternHits recounts a zero-match pattern search WITHOUT the
// event_types filter, keeping every other filter (window, pid, thread)
// identical, and reports how many rows of OTHER event types would have
// matched — the precise counterfactual for "would dropping event_types have
// found the pattern?". Same-type rows are skipped through the same
// eventTypeMatches predicate the failed search used (including its
// compatibility aliases), so a row the original filter admitted is never
// counted as cross-type evidence. Counts drive a soft hint only.
func crossTypePatternHits(idx *Index, q Query) crossTypeHitSummary {
	var sum crossTypeHitSummary
	if idx == nil || len(q.EventTypes) == 0 || strings.TrimSpace(q.Pattern) == "" {
		return sum
	}
	typeSet := make(map[EventType]bool, len(q.EventTypes))
	for _, t := range q.EventTypes {
		if t != "" {
			typeSet[t] = true
		}
	}
	actionSet := traceMarkActionFilterSet(q.TraceMarkActions)
	if len(typeSet) == 0 {
		return sum
	}
	counts := map[EventType]int{}
	examined := 0
	for _, ev := range idx.Events {
		if !eventInQueryWindow(ev, q) {
			continue
		}
		if examined >= crossTypeRescanInWindowEventBudget {
			sum.truncated = true
			break
		}
		examined++
		if eventTypeMatches(ev, typeSet) {
			// Rows the original type filter admitted were already searched by
			// the zero-match query; only OTHER types are counterfactual hits.
			continue
		}
		if !eventInQuery(ev, q, nil, actionSet) {
			continue
		}
		counts[ev.Type]++
		sum.total++
		if sum.total >= crossTypeRescanMatchCap {
			sum.truncated = true
			break
		}
	}
	for typ, count := range counts {
		sum.top = append(sum.top, crossTypeHitCount{Type: typ, Count: count})
	}
	sort.Slice(sum.top, func(i, j int) bool {
		if sum.top[i].Count != sum.top[j].Count {
			return sum.top[i].Count > sum.top[j].Count
		}
		return sum.top[i].Type < sum.top[j].Type
	})
	return sum
}

// formatEventTypesFilter renders q.EventTypes exactly as the tool parameter is
// written (["trace_mark","perf_sample"]) so hint text stays copy-pasteable.
func formatEventTypesFilter(types []EventType) string {
	parts := make([]string, 0, len(types))
	for _, t := range types {
		if t != "" {
			parts = append(parts, fmt.Sprintf("%q", string(t)))
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// threadSelectorSpanNameCaveat is the D-diag B-2 (§16) thread=<span name>
// silent-zero diagnosis. resolveThread returns ThreadRef{Comm: <selector>,
// PID: 0} when a comm-less selector matches no scheduled thread, and every
// thread-scoped face downstream (threadMatches' comm lane, window-stats
// drill targets, timelines) then renders structural zeros without saying why —
// q7 burned rounds treating the span label "bindApplication" as a thread.
// Preconditions are all PRECISE signals:
//   - a thread selector is present and carries no pid (pid selectors resolve
//     on the pid lane and already have their own caveat);
//   - resolveThread found no scheduled thread (PID == 0);
//   - no in-window scheduler row's comm face equals the selector (residual
//     pid==0 comm lane — e.g. swapper — mirroring threadMatches' EqualFold);
//   - the selector text occurs verbatim (case-folded) as a span name in the
//     parsed rows.
//
// Soft guidance only: the caveat never blocks or alters the query result.
//
// Cost discipline: the scan below runs only after resolveThread already missed
// (itself a linear scan), adds at most ONE more allocation-free linear pass of
// EqualFold comparisons over parsed events, and returns early the moment a
// scheduler comm match disproves the caveat. Streamed results are excluded via
// their streamed_* marker caveat because a streaming pass retains only rows
// matched by the current filters — its event set says nothing about what the
// trace schedules, so the "no scheduled thread" claim would be unsound there.
func threadSelectorSpanNameCaveat(idx *Index, q Query, res Result) string {
	if idx == nil || len(idx.Events) == 0 || q.PID > 0 {
		return ""
	}
	selector := strings.TrimSpace(firstNonEmpty(q.ThreadInput, q.Thread))
	if selector == "" {
		return ""
	}
	for _, c := range res.Caveats {
		if strings.HasPrefix(c, "streamed_") {
			return ""
		}
	}
	sel := parseThreadSelector(selector)
	if sel.HasPID {
		return ""
	}
	resolution := resolveThreadSelection(idx, q)
	if resolution.Ambiguous || resolution.Thread.PID != 0 {
		return ""
	}
	spanName := ""
	for _, ev := range idx.Events {
		if spanName == "" && ev.SpanName != "" &&
			(strings.EqualFold(ev.SpanName, sel.Raw) || (sel.Name != "" && strings.EqualFold(ev.SpanName, sel.Name))) {
			spanName = ev.SpanName
		}
		switch ev.Type {
		case EventSchedSwitch, EventSchedWakeup, EventSchedWaking:
			if eventInQueryWindow(ev, q) && selectorFoldEqualsSchedComm(sel, ev) {
				// The selector does have in-window scheduler material after
				// all (pid==0 comm lane); thread-scoped results are not
				// structurally zero, so stay silent.
				return ""
			}
		}
	}
	if spanName == "" {
		return ""
	}
	return fmt.Sprintf("thread_selector_is_span_name=thread=%q matched no scheduled thread in the parsed trace rows, so thread-scoped scheduler numbers for it are structurally zero; the same text does occur as span label %q — use trace_query(view=\"span_window\", span_name=%q) to get that span's time window, or trace_query(view=\"event_search\", pattern=%q) to see its rows, then rerun thread-scoped views with a pid or thread name taken from those rows", selector, spanName, spanName, spanName)
}

// selectorFoldEqualsSchedComm mirrors threadMatches' comm lane (EqualFold on
// the resolved Comm) across the four scheduler comm faces — the exact residual
// lane that could still produce non-zero thread-scoped results when
// resolveThread returned PID==0 (comm rows with pid 0, e.g. swapper).
func selectorFoldEqualsSchedComm(sel threadSelector, ev Event) bool {
	name := sel.Name
	if name == "" {
		name = sel.Raw
	}
	if name == "" {
		return false
	}
	return (ev.Comm != "" && strings.EqualFold(ev.Comm, name)) ||
		(ev.PrevComm != "" && strings.EqualFold(ev.PrevComm, name)) ||
		(ev.NextComm != "" && strings.EqualFold(ev.NextComm, name)) ||
		(ev.WakeeComm != "" && strings.EqualFold(ev.WakeeComm, name))
}

func traceWindowStrategyCaveats(q Query, res Result) []string {
	view := strings.TrimSpace(firstNonEmpty(res.View, q.View))
	if !traceHeavyViewWindowStrategyApplies(view) {
		return nil
	}
	duration := selectedWindowDurationSeconds(q.TimeStart, q.TimeEnd)
	if duration <= 0 {
		return nil
	}
	if duration >= parentWindowStrategySeconds {
		return []string{fmt.Sprintf("trace window strategy: selected_window_duration=%.3fms is a parent/transaction window. Preserve the full window as parent coverage, summarize phase/span/marker boundaries first, then drill into the heaviest phase windows; do not present arbitrary micro-window samples as exhaustive coverage of the parent window.", duration*1000)}
	}
	if duration >= microWindowProbeSeconds {
		return nil
	}
	return []string{fmt.Sprintf("trace window strategy: selected_window_duration=%.3fms is a micro-window probe. For broader jank/stall root-cause claims, prefer frame/span-derived windows or %.0f-%.0fms coverage windows first; use this sub-%.0fms result only as local evidence unless neighboring coverage windows corroborate it.",
		duration*1000,
		preferredCoverageWindowMinSeconds*1000,
		preferredCoverageWindowMaxSeconds*1000,
		microWindowProbeSeconds*1000)}
}

func traceHeavyViewWindowStrategyApplies(view string) bool {
	switch strings.TrimSpace(view) {
	case "window_stats", "scheduler_latency_stats", "wakeup_chain", "root_cause_rank", "frame_root_cause_bundle",
		"critical_blocking_calls", "recipe", "evidence_pack", "trace_perf_bundle", "thread_timeline":
		return true
	default:
		return false
	}
}

func selectedWindowDurationSeconds(start, end float64) float64 {
	if end <= start || start < 0 {
		return 0
	}
	return end - start
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
		// CFC P0: a reclassified clock lane sample must not silence the
		// "no initial frequency" completeness caveat.
		if !eventLineInWindow(ev, q) || !isPerCPUFrequencySample(ev) || eventCPUForStats(ev) != cpu {
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
		if !eventLineInWindow(ev, q) || !isPerCPUFrequencySample(ev) || eventCPUForStats(ev) != cpu {
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
	if isWritebackObservation(ev) {
		return false
	}
	if EROFSCoverageOnlyNameCandidate(ev.Name) {
		return false
	}
	if !f2fsSemanticPayloadAdmitted(ev) {
		return false
	}
	name := strings.ToLower(ev.Name)
	ff := ev.FileFields
	if ff == nil || (ff.Ino == "" && ff.Entry == "" && ff.Dev == "") {
		return false
	}
	if isPageCacheEvent(ev) {
		return false
	}
	for _, token := range []string{
		"android_fs_dataread", "android_fs_datawrite", "f2fs_direct_io",
		"f2fs_sync_file", "f2fs_submit_read_bio", "f2fs_submit_write_bio",
		"ext4_", "file_system", "filesystem", "ebpf_file",
	} {
		if strings.Contains(name, token) {
			return true
		}
	}
	return ev.Type == EventFilesystem && (ff.Ino != "" || ff.Entry != "")
}

func fileIOCountsAsActivity(ev Event) bool {
	if profile, exact := exactF2FSPairingProfile(ev.Name); exact {
		return profile.Phase == PairingEndpointStart && f2fsSemanticPayloadAdmitted(ev)
	}
	if F2FSClosedEndpointNameCandidate(ev.Name) {
		return false
	}
	name := strings.ToLower(ev.Name)
	if strings.HasSuffix(name, "_end") || strings.HasSuffix(name, "_exit") || strings.HasSuffix(name, "_done") {
		return false
	}
	return true
}

func isPageCacheEvent(ev Event) bool {
	return pageCacheMutationKindForEvent(ev) != pageCacheMutationNone
}

func isStorageLatencyEvent(ev Event) bool {
	if isWritebackObservation(ev) {
		return false
	}
	if EROFSCoverageOnlyNameCandidate(ev.Name) {
		return false
	}
	if !f2fsSemanticPayloadAdmitted(ev) {
		return false
	}
	if ev.Type == EventBlockIssue || ev.Type == EventBlockComplete {
		_, _, endpoint := blockLatencyEndpoint(ev)
		return endpoint
	}
	layer := storageLatencyLayer(ev)
	if layer == "" {
		return false
	}
	_, phase := storageLatencyBaseAndPhase(ev)
	return phase != ""
}

func storageLatencyLayer(ev Event) string {
	if EROFSCoverageOnlyNameCandidate(ev.Name) {
		return ""
	}
	if profile, exact := exactMMCPairingProfile(ev.Name); exact {
		return profile.Layer
	}
	if mmcPairingNameCandidate(ev.Name) {
		return ""
	}
	if profile, exact := exactF2FSPairingProfile(ev.Name); exact {
		return profile.Layer
	}
	if f2fsElapsedPairingNameCandidate(ev.Name) {
		return ""
	}
	name := strings.ToLower(ev.Name)
	switch {
	case ev.Type == EventBlockIssue || ev.Type == EventBlockComplete:
		if _, _, endpoint := blockLatencyEndpoint(ev); endpoint {
			return "block"
		}
		return ""
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
	if EROFSCoverageOnlyNameCandidate(ev.Name) {
		return "", ""
	}
	if family, endpointPhase, endpoint := blockLatencyEndpoint(ev); endpoint {
		if endpointPhase == blockEndpointStart {
			return family, "start"
		}
		return family, "done"
	}
	if profile, exact := exactMMCPairingProfile(ev.Name); exact {
		return profile.SemanticBase, string(profile.Phase)
	}
	if mmcPairingNameCandidate(ev.Name) {
		return "", ""
	}
	if profile, exact := exactF2FSPairingProfile(ev.Name); exact {
		return profile.SemanticBase, string(profile.Phase)
	}
	if f2fsElapsedPairingNameCandidate(ev.Name) {
		return "", ""
	}
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
