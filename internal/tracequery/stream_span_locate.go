package tracequery

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// StreamSpanLocate resolves a named B/E or S/F span across one immutable
// physical trace without materializing a full event index.  The caller's
// bounded time/line window is a parent selector: an endpoint pair is eligible
// when its full interval intersects that parent, but the remote endpoint may be
// arbitrarily far outside it.  This is the long-span counterpart to the local
// indexed span_window lane; no fixed padding is used as a duration boundary.
//
// The implementation intentionally reuses trace_mark_carry's single endpoint
// state machine.  There is no second parser or pairing algorithm to drift, and
// only complete_exact pairs may mint TraceSpanSummary durations.
func StreamSpanLocate(ctx context.Context, path string, flavorHint TraceFlavor, q Query) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	path = canonicalTraceIndexPath(strings.TrimSpace(path))
	if path == "" {
		return Result{}, fmt.Errorf("stream_span_locate: trace path is empty")
	}
	requiresComposite, err := tracePathRequiresCompositeIndexContext(ctx, path)
	if err != nil {
		return Result{}, err
	}
	if requiresComposite {
		return Result{}, fmt.Errorf("stream_span_locate requires a single physical artifact; %s belongs to a composite artifact universe, so use the indexed path to preserve source and clock-domain provenance", path)
	}

	q.View = CanonicalViewName(q.View)
	if q.View == "recipe" && normalizeRecipeName(q.RecipeName, q) == "span_locate" && strings.TrimSpace(q.SpanName) == "" {
		q.SpanName = strings.TrimSpace(q.Pattern)
	}
	if q.View != "span_window" && !(q.View == "recipe" && normalizeRecipeName(q.RecipeName, q) == "span_locate") {
		return Result{}, fmt.Errorf("stream_span_locate: view=%q is not span_window or recipe/span_locate", q.View)
	}
	if strings.TrimSpace(q.SpanName) == "" {
		return Result{}, fmt.Errorf("stream_span_locate: span_name (or span_locate pattern) is required")
	}
	boundedTime := queryBoundedTimeStart(q) && queryBoundedTimeEnd(q)
	boundedLines := q.LineStart > 0 && q.LineEnd > 0
	if !boundedTime && !boundedLines {
		return Result{}, fmt.Errorf("stream_span_locate: a complete parent time or line window is required")
	}
	q.TargetScope = normalizedTargetScope(q.TargetScope)
	if q.TargetScope == TargetScopeProcess && q.PID <= 0 {
		return Result{}, fmt.Errorf("stream_span_locate: target_scope=process requires pid=<process_id>")
	}
	if q.TargetScope != TargetScopeProcess && q.PID <= 0 &&
		(strings.TrimSpace(q.Thread) != "" || strings.TrimSpace(q.ThreadInput) != "") {
		return Result{}, fmt.Errorf("stream_span_locate: a name-only thread selector needs indexed identity resolution; pass the exact pid or use the indexed path")
	}

	request := WindowDiscoveryRequest{
		Strategy:     WindowDiscoveryTraceMarkCarry,
		Families:     []WindowDiscoveryFamily{WindowDiscoveryFamilyTraceSync, WindowDiscoveryFamilyTraceAsync},
		TimeStart:    q.TimeStart,
		TimeEnd:      q.TimeEnd,
		TimeStartSet: boundedTime,
		TimeEndSet:   boundedTime,
		LineStart:    q.LineStart,
		LineEnd:      q.LineEnd,
		// A long pair may require two bounded endpoint windows.  Discovery
		// selection is not the span result cap; q.Limit is applied below.
		MaxWindows: HardWindowDiscoveryMaxWindows,
	}
	request, err = normalizeWindowDiscoveryRequest(request)
	if err != nil {
		return Result{}, err
	}
	version, err := CaptureTraceSourceVersion(path)
	if err != nil {
		return Result{}, err
	}
	discovery := newTraceMarkCarryDiscovery(request, path)
	discovery.spanSelector = &traceMarkCarrySpanSelector{
		name:        strings.TrimSpace(q.SpanName),
		targetScope: q.TargetScope,
		pid:         q.PID,
	}
	shell, err := StreamScan(ctx, path, flavorHint, func(ev Event) bool {
		return discovery.observe(path, ev)
	})
	if err != nil {
		return Result{}, err
	}
	if windowDiscoveryAfterStreamScanHook != nil {
		windowDiscoveryAfterStreamScanHook()
	}
	if err := version.Validate(path); err != nil {
		return Result{}, err
	}
	discoveryResult := discovery.finalize(shell, version)

	var target ThreadRef
	if q.TargetScope != TargetScopeProcess && q.PID > 0 {
		target.PID = q.PID
	}
	var spans []TraceSpanSummary
	var endpointEvents []EventView
	withheldLifecycle := map[int]Event{}
	for _, candidate := range discovery.candidates {
		if candidate == nil || candidate.PairingStatus != WindowDiscoveryPairingCompleteExact ||
			!candidate.CollectionComplete || len(candidate.events) < 1 {
			continue
		}
		var span TraceSpanSummary
		var ok bool
		if candidate.Kind == "typed_interval" {
			span, ok = traceSpanFromCompletedAsyncInterval(candidate.events[0], path)
		} else if len(candidate.events) >= 2 {
			kind := "sync"
			if candidate.Family == WindowDiscoveryFamilyTraceAsync {
				kind = "async"
			}
			span, ok = traceSpanFromEvents(candidate.events[0], candidate.events[1], kind, path), true
		}
		if !ok || !traceSpanMatchesQuery(span, target, q) {
			continue
		}
		if boundary, conflict := discovery.spanSelectorLifecycleConflicts[span.Thread.PID]; conflict &&
			(pairingEventInsideQuery(boundary, discovery.scope) || (boundary.Ts >= span.StartTs && boundary.Ts <= span.EndTs)) {
			withheldLifecycle[span.Thread.PID] = boundary
			continue
		}
		annotateTraceSpanTargetScope(&span, q)
		spans = append(spans, span)
		for _, ev := range candidate.events {
			if ev.Line <= 0 {
				continue
			}
			endpointEvents = append(endpointEvents, EventView{
				Event: ev, Raw: ev.FieldText, SourcePath: path, LocalLine: ev.Line,
				TimeDomain: "trace_seconds", CanonicalTimeDomain: "trace_seconds", SourceTs: ev.Ts, ClockAligned: true,
			})
		}
	}
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].StartTs != spans[j].StartTs {
			return spans[i].StartTs < spans[j].StartTs
		}
		return spans[i].StartLine < spans[j].StartLine
	})
	sort.SliceStable(endpointEvents, func(i, j int) bool {
		if endpointEvents[i].Ts != endpointEvents[j].Ts {
			return endpointEvents[i].Ts < endpointEvents[j].Ts
		}
		return endpointEvents[i].Line < endpointEvents[j].Line
	})

	limit := ViewCapacityFor("span_window").ClampLimit(q.Limit)
	totalSpans := len(spans)
	if len(spans) > limit {
		spans = spans[:limit]
	}
	// Endpoint rows are a recipe locate aid only.  Keep them aligned to the
	// published span roster instead of leaking endpoints of a compacted span.
	if len(spans) < totalSpans {
		allowedLines := map[int]bool{}
		for _, span := range spans {
			allowedLines[span.StartLine] = true
			allowedLines[span.EndLine] = true
		}
		filtered := endpointEvents[:0]
		for _, ev := range endpointEvents {
			if allowedLines[ev.Line] {
				filtered = append(filtered, ev)
			}
		}
		endpointEvents = filtered
	}

	flavor, confidence, signals, flavorCaveats := resolveTraceFlavor(shell, q)
	q.TraceFlavor = flavor
	platform, platformCandidate, platformCandidateConfidence, platformCandidateSignals, platformCaveats :=
		resolveTracePlatform(shell, q, flavor, shell.platformDetectionSurfaces(), signals)
	q.TracePlatform = platform
	start, end := q.TimeStart, q.TimeEnd
	if len(spans) == 1 {
		if boundedTime {
			parent := TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}
			unioned := unionTimeWindows(parent, TimeWindow{StartTs: spans[0].StartTs, EndTs: spans[0].EndTs})
			start, end = unioned.StartTs, unioned.EndTs
		} else {
			start, end = spans[0].StartTs, spans[0].EndTs
		}
	}
	artifact := TraceArtifactSource{
		SourcePath: path, Kind: inferTraceArtifactKind(path),
		TimeDomain: "trace_seconds", CanonicalTimeDomain: "trace_seconds",
		LocalLineCount: shell.LineCount, EventCount: shell.ParsedKnown,
		SourceBytes: version.SourceBytes(), SourceModUnixNano: shell.ModTime.UnixNano(),
		CausalCompatible: true, ClockAlignment: TraceClockAlignmentIdentity,
	}
	res := Result{
		View: q.View, SourcePath: path, TraceArtifacts: []TraceArtifactSource{artifact},
		TraceFlavor: string(flavor), Platform: string(platform),
		PlatformCandidate: platformCandidate, PlatformCandidateConfidence: platformCandidateConfidence,
		PlatformCandidateSignals: platformCandidateSignals,
		FlavorConfidence:         confidence, FlavorSignals: signals,
		FrameworkMode: FrameworkModeForPlatform(platform), TimeUnit: "seconds",
		PrioritySemantics: PrioritySemanticsForFlavor(flavor),
		LineCount:         shell.LineCount, ScannedLineCount: shell.ScannedLineCount,
		EventCount: shell.ParsedKnown, UnparsedLineCount: shell.UnparsedLines,
		ParseLinePanics: shell.ParseLinePanics, ClockRegressions: shell.ClockRegressions,
		TimeStart: start, TimeEnd: end, TargetScope: q.TargetScope,
		SpanWindows: spans,
	}
	if q.View == "recipe" {
		res.Recipe = &RecipeResult{
			Name: "span_locate", IncludedViews: []string{"event_search", "span_window"},
			Summary: "span-locate recipe resolved the named marker with exact whole-artifact endpoint pairing before publishing its start/end window",
		}
		res.Events = endpointEvents
	}
	res.EvidencePack = evidenceFromSpans(spans)
	if q.View == "recipe" {
		res.EvidencePack = append(res.EvidencePack, evidenceFromEvents(endpointEvents)...)
	}
	attachEvidenceFactProvenance(res.EvidencePack, res.TraceArtifacts)
	res.Caveats = append(res.Caveats, discoveryResult.Caveats...)
	withheldPIDs := make([]int, 0, len(withheldLifecycle))
	for pid := range withheldLifecycle {
		withheldPIDs = append(withheldPIDs, pid)
	}
	sort.Ints(withheldPIDs)
	for _, pid := range withheldPIDs {
		boundary := withheldLifecycle[pid]
		res.Caveats = append(res.Caveats, fmt.Sprintf("thread_identity_fail_closed=true; tid=%d lifecycle_boundary_line=%d lifecycle_boundary_ts=%.6f; complete span rows for this task incarnation were withheld", pid, boundary.Line, boundary.Ts))
	}
	res.Caveats = append(res.Caveats,
		fmt.Sprintf("streamed_span_locate=true; scanned %d line(s) across one immutable physical artifact; the parent window selected intersecting identities, while complete B/E or S/F endpoints were paired without a fixed time-padding ceiling", shell.ScannedLineCount))
	if len(spans) == 1 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("selected_window preserved the parent and unioned it with exact span %q window %.6f..%.6f lines=%d-%d",
			spans[0].Name, spans[0].StartTs, spans[0].EndTs, spans[0].StartLine, spans[0].EndLine))
	} else if len(spans) > 1 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("span_name=%q matched %d complete span(s) intersecting the parent; the parent window was preserved and follow-up causal analysis requires selecting one exact span", q.SpanName, totalSpans))
	} else {
		res.Caveats = append(res.Caveats, fmt.Sprintf("span_name=%q produced no complete exact pair; no duration or causal window was inferred from an unmatched endpoint", q.SpanName))
	}
	if totalSpans > len(spans) {
		last := spans[len(spans)-1]
		res.Compactions = append(res.Compactions, ViewCompaction{
			View: "span_window", Dimension: CompactionDimensionSpans,
			Total: totalSpans, Emitted: len(spans), LastEmittedTs: last.StartTs, LastEmittedLine: last.StartLine,
		})
		res.Caveats = append(res.Caveats, fmt.Sprintf("span_window compacted from %d to %d span(s)", totalSpans, len(spans)))
	}
	res.Caveats = append(res.Caveats, flavorCaveats...)
	res.Caveats = append(res.Caveats, platformCaveats...)
	res.Caveats = dedupStrings(res.Caveats)
	return res, nil
}
