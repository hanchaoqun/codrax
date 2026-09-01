package tracequery

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// StreamEventSearch scans a trace for event_search rows without materializing a
// full Index. It is intended for large unbounded discovery calls where the
// model is looking for a frame id, timestamp token, span label, or resource key
// before carrying a bounded window into heavier views.
func StreamEventSearch(ctx context.Context, path string, q Query) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return Result{}, fmt.Errorf("trace path is empty")
	}
	path = canonicalTraceIndexPath(path)
	requiresComposite, err := tracePathRequiresCompositeIndexContext(ctx, path)
	if err != nil {
		return Result{}, err
	}
	if requiresComposite {
		return Result{}, fmt.Errorf("stream_event_search requires a single physical artifact; %s is a tracebundle, so use the indexed path to preserve artifact and clock-domain provenance", path)
	}
	initialIdentity, err := filegeneration.FromPath(path)
	if err != nil {
		return Result{}, err
	}
	f, openedIdentity, err := openTraceSourceRegularContext(ctx, path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	openedInfo, err := f.Stat()
	if err != nil {
		return Result{}, err
	}
	if !openedIdentity.SameVersion(initialIdentity) {
		return Result{}, fmt.Errorf("trace source identity changed before stream_event_search opened the artifact")
	}
	info := openedInfo

	if err := ValidateTraceMarkActionFilter(q.View, q.EventTypes, q.TraceMarkActions); err != nil {
		return Result{}, fmt.Errorf("stream_event_search: %w", err)
	}
	q.View = "event_search"
	q = normalizeQuery(nil, q)
	typeSet := make(map[EventType]bool, len(q.EventTypes))
	for _, typ := range q.EventTypes {
		if typ != "" {
			typeSet[typ] = true
		}
	}
	actionSet := traceMarkActionFilterSet(q.TraceMarkActions)
	_, identityAddressed := perfTimelineThreadSelector(q)
	if identityAddressed && typeSet[EventPerfSample] {
		// Perf thread selection requires the full lifecycle/provenance ledger.
		// A streaming row has no stable event ordinal and cannot prove generation,
		// alias ambiguity or withheld identity, so route an EXPLICIT perf_sample
		// request through the indexed authority instead of resurrecting raw
		// comm/TID matching. An untyped mixed discovery remains streaming and
		// withholds only perf rows below; otherwise one possible perf row would
		// silently turn a dense discovery call into a full-index build.
		indexed, buildErr := BuildIndex(ctx, path)
		if buildErr != nil {
			return Result{}, buildErr
		}
		return Run(indexed, q.WithRunContext(ctx)), nil
	}

	idx := &Index{Path: path, Size: info.Size(), ModTime: info.ModTime()}
	artifactSource := singleTraceArtifactSourceWithIdentity(path, openedIdentity, 0, 0)
	anchorKey := traceAnchorKeyForIdentity(path, openedIdentity)
	anchorSet := anchorCache.load(anchorKey)
	if anchorSet != nil {
		idx.TimestampOrder = anchorSet.TimestampOrder
	}
	// Mixed line+time window authority (streaming-consistency audit #50): the
	// indexed lane's eventInQueryWindow convention — "time bounds apply only
	// when no line bounds are set" — governs both lanes. The streaming raw
	// pre-gate used to intersect line∩time while the zero-match fallback to
	// the indexed path applied line-only, silently flipping window semantics
	// mid-call. timeGateActive is the single switch for the time gate, the
	// time-end early stop, and the time side of the anchor seek below.
	timeGateActive := q.LineStart == 0 && q.LineEnd == 0
	startLine := 1
	seeked := false
	var seekAnchor traceAnchor
	if anchorSet != nil && anchorSet.FlavorSet {
		if a, ok := anchorSet.seekAnchorFor(timeGateActive && q.TimeStart > 0, q.TimeStart, q.LineStart); ok {
			if _, seekErr := f.Seek(a.ByteOffset, io.SeekStart); seekErr == nil {
				seekAnchor = a
				startLine = a.LineNo + 1
				seeked = true
			}
		}
	}
	recorder := newAnchorRecorder(anchorSet, seekAnchor, seeked)
	recording := recorder.canExtend(startLine)
	intern := newStringInterner()
	flavor := newFlavorVote(path)
	// W-1 修根 (platform_surfaces.go): every parsed event feeds the platform
	// surface vote BEFORE the result filter — the detection input must not
	// drift with event_types/pattern (the witness flip lane).
	platformVote := newPlatformSurfaceVote()
	frozenSource, err := frozenTraceSectionAtCurrentOffset(f, openedIdentity)
	if err != nil {
		return Result{}, err
	}
	reader := bufio.NewReaderSize(frozenSource, 256*1024)
	seenTimeWindow := false
	limit := ViewCapacityFor(q.View).ClampLimit(q.Limit)
	matchedTotal := 0
	scopeTimestampRows := 0
	scopeTimeStart := 0.0
	scopeTimeEnd := 0.0
	matchedTimeStart := 0.0
	matchedTimeEnd := 0.0
	reachedEOF := false
	lastParsedTs := 0.0
	// RFC #71 (§8.2 c4): the census accumulates in this same match pass —
	// the scan already runs past the display limit to count matchedTotal,
	// so the full tier ladder costs O(distinct tiers) extra memory only.
	census := newCPUFrequencyCensusAcc(typeSet)
	// SA-F2 (DISPATCH-IND 批4, 2026-07-14): the generator census accumulates
	// in this same pass on the TARGET-FREE matched set (pattern/type/window
	// arms only — the pid/thread arms are deliberately not applied, see the
	// caliber note in vsync_generator_census.go: the generator lives outside
	// the queried thread's process by construction, and the tool-injected
	// runtime target blinded the census on the tieba run-2 witness). The raw
	// prefilter is pattern-only, so generator rows are parsed regardless of
	// the thread filter. O(generators) memory.
	vsyncCensus := newVsyncGeneratorCensusAcc()
	vsyncCensusQ := vsyncGeneratorCensusQuery(q)
	vsyncCensusTargetFree := vsyncCensusQ.PID != q.PID || vsyncCensusQ.Thread != q.Thread
	perfIdentityRowsWithheld := false
	var events []EventView
	var scan lineScan
	for lineNo := startLine; ; lineNo++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		line, readErr := readStreamScanPhysicalLine(reader, attachment.TracePhysicalLineMaxBytes)
		if len(line) > 0 {
			idx.LineCount = lineNo
			// Actual scanned volume, not the absolute line number: after an
			// anchor seek the skipped prefix was never read, so counting it as
			// "scanned" both misstates the caveat and dilutes the
			// unparsed-ratio denominator (audit #51). LineCount above keeps
			// the absolute physical line number.
			idx.ScannedLineCount = lineNo - startLine + 1
			trimmed := strings.TrimRight(line, "\r\n")
			scan.reset(lineNo, trimmed)
			lineTs, lineHasTS := scan.timestamp()
			if recording {
				recorder.observe(lineNo, len(line), lineTs, lineHasTS, trimmed)
			}
			if q.LineEnd > 0 && lineNo > q.LineEnd {
				break
			}
			if q.LineStart > 0 && lineNo < q.LineStart {
				if lineNo <= 200 {
					flavor.observeRawLine(trimmed)
				}
				goto nextLine
			}
			// blocked_reason is an optional D/IO refinement, with its own
			// field-local integrity ledger. Audit before the result prefilter and
			// before the strict time gate so a closing marker in (end,end+5µs]
			// cannot disappear from the streaming parity face.
			if failure := blockedReasonValidationFailureScan(&scan); failure != nil && blockedReasonIntegrityFailureRelevantToQuery(failure, q, q.PID) {
				failure.SourcePath = path
				appendBlockedReasonIntegrityFailure(idx, *failure)
			}
			if timeGateActive && (q.TimeStart > 0 || q.TimeEnd > 0) {
				ts, hasTS := lineTs, lineHasTS
				if hasTS {
					if q.TimeEnd > 0 && ts > q.TimeEnd {
						// Keep scanning the blocked-reason closing tail even when a
						// different event is the first row beyond TimeEnd. The field
						// audit above consumes malformed markers in this tail; no tail
						// row is an event_search result or parse-quality member.
						if ts <= wakeupClosingUpperBound(q.TimeEnd) {
							goto nextLine
						}
						if idx.TimestampOrder.AllowsTimeEndEarlyStop() {
							break
						}
						goto nextLine
					}
					if q.TimeStart > 0 && ts < q.TimeStart {
						if lineNo <= 200 {
							flavor.observeRawLine(trimmed)
						}
						goto nextLine
					}
					seenTimeWindow = true
				} else if q.TimeStart > 0 && !seenTimeWindow {
					if lineNo <= 200 {
						flavor.observeRawLine(trimmed)
					}
					goto nextLine
				}
			}
			// B5-T2: account the selected physical timestamp domain before
			// the raw pattern/type prefilter. A pattern may parse only a tiny
			// candidate subset, but it must not shrink the artifact envelope
			// to those matches.
			if lineHasTS {
				scopeTimestampRows++
				if scopeTimestampRows == 1 || lineTs < scopeTimeStart {
					scopeTimeStart = lineTs
				}
				if lineTs > scopeTimeEnd {
					scopeTimeEnd = lineTs
				}
			}
			flavor.observeRawLine(trimmed)
			// Keep trace-mark integrity auditing independent of the result filter.
			// In particular, an exact action filter must not hide a malformed S/F
			// or G/H row merely because its parser-validated SpanAction is empty.
			// Audit before the pattern prefilter; raw prefixes remain diagnostic
			// witnesses only and are never re-admitted as typed matches below.
			// The shared lineScan memo keeps this audit plus the parse below at
			// ONE header match per line (perf audit #21).
			if failure := traceMarkValidationFailureScan(&scan); failure != nil && traceMarkIntegrityFailureRelevantToQuery(*failure, q) {
				failure.SourcePath = path
				appendTraceMarkIntegrityFailure(idx, *failure)
			}
			if !streamEventSearchRawCandidate(trimmed, lineNo, q) {
				goto nextLine
			}
			// Parse-quality counters mirror the indexed path (safeParseLine +
			// UnparsedLines + ClockRegressions): since event_search now ALWAYS
			// streams, the coverage/quality caveats the indexed engine surfaced
			// must not silently disappear. With a pattern set the counters cover
			// the candidate lines only (the prefilter skip is load-bearing for
			// GB traces) — a conservative undercount, never an overclaim.
			panicsBefore := idx.ParseLinePanics
			ev, ok := safeParseLineScan(&scan, intern, idx)
			if !ok {
				if trimmed != "" {
					if idx.ParseLinePanics == panicsBefore {
						idx.UnparsedLines++
					}
					// TDIAG B4: same typed sample face as the indexed build
					// (with a pattern set this covers candidate lines only —
					// the counters' existing conservative scope).
					idx.recordUnparsedSample(lineNo, trimmed)
				}
				goto nextLine
			}
			if prev := lastParsedTs; prev > 0 && ev.Ts > 0 && ev.Ts < prev {
				idx.ClockRegressions++
			}
			if ev.Ts > 0 {
				lastParsedTs = ev.Ts
			}
			if idx.FirstTs == 0 || ev.Ts < idx.FirstTs {
				idx.FirstTs = ev.Ts
			}
			if ev.Ts > idx.LastTs {
				idx.LastTs = ev.Ts
			}
			if ev.Type != EventUnknown {
				idx.ParsedKnown++
			}
			if countTraceDBTextRecord(idx, ev) {
				goto nextLine
			}
			flavor.observeEvent(ev)
			platformVote.observe(ev)
			if identityAddressed && ev.Type == EventPerfSample {
				// The base gates prove that this row belonged to the requested
				// inventory/window/pattern, but streaming has no stable ordinal or
				// lifecycle ledger with which to join it to a thread generation.
				// Keep other event families streaming and disclose this local
				// withdrawal instead of either raw-matching or materializing the
				// entire trace merely because event_types was omitted.
				if eventInQueryBase(ev, q, typeSet, actionSet) {
					perfIdentityRowsWithheld = true
				}
				goto nextLine
			}
			if !eventInQuery(ev, q, typeSet, actionSet) {
				// SA-F2 target-free census arm: a row excluded ONLY by the
				// pid/thread filter still feeds the generator census.
				if vsyncCensusTargetFree && eventInQuery(ev, vsyncCensusQ, typeSet, actionSet) {
					vsyncCensus.observe(ev)
				}
				goto nextLine
			}
			matchedTotal++
			if matchedTotal == 1 || ev.Ts < matchedTimeStart {
				matchedTimeStart = ev.Ts
			}
			if ev.Ts > matchedTimeEnd {
				matchedTimeEnd = ev.Ts
			}
			census.observe(ev)
			vsyncCensus.observe(ev)
			events = insertEventViewChronological(events, eventViewFromSource(
				applyPriorityFlavor(ev, q.TraceFlavor), trimmed, artifactSource, ev.Line), limit)
		}
	nextLine:
		if readErr != nil {
			if readErr == io.EOF {
				reachedEOF = true
				if recording {
					recorder.finishEOF()
				}
				break
			}
			return Result{}, traceReadErrorAfterIdentity(f, openedIdentity, "stream_event_search physical read", readErr)
		}
	}
	if err := validateTraceFileIdentityAfterRead(f, openedIdentity, "stream_event_search"); err != nil {
		return Result{}, err
	}
	if seeked && anchorSet != nil && anchorSet.FlavorSet {
		idx.TraceFlavor, idx.FlavorConfidence, idx.FlavorSignals = anchorSet.Flavor, anchorSet.FlavorConf, append([]string(nil), anchorSet.FlavorSignals...)
	} else {
		idx.TraceFlavor, idx.FlavorConfidence, idx.FlavorSignals = flavor.result()
	}
	if recording {
		if !seeked && !recorder.set.FlavorSet {
			recorder.set.FlavorSet = true
			recorder.set.Flavor = idx.TraceFlavor
			recorder.set.FlavorConf = idx.FlavorConfidence
			recorder.set.FlavorSignals = append([]string(nil), idx.FlavorSignals...)
		}
		// W-1 修根 + 复核 F2: only a from-0 scan with COMPLETE parsed-event
		// coverage (no pattern prefilter / no window, reached EOF) may mint
		// the per-file record — once, flavor discipline; an ineligible scan
		// leaves minting to the next eligible one.
		if !seeked && !recorder.set.PlatformSurfaces.Set && platformSurfaceMintEligible(q, reachedEOF) {
			recorder.set.PlatformSurfaces = platformVote.result(false)
		}
		anchorCache.store(anchorKey, recorder.set)
	}
	if anchorSet != nil && anchorSet.PlatformSurfaces.Set {
		idx.platformSurfaces = anchorSet.PlatformSurfaces.clone()
	} else if recorder.set.PlatformSurfaces.Set {
		idx.platformSurfaces = recorder.set.PlatformSurfaces.clone()
	} else {
		idx.platformSurfaces = platformVote.result(!platformSurfaceMintEligible(q, reachedEOF))
	}
	idx.TimestampOrder = recorder.set.TimestampOrder
	if idx.TimestampOrder != TraceTimestampOrderUnknown {
		idx.ClockRegressions = recorder.set.CoveredClockRegressions
	}
	idx.Events = make([]Event, len(events))
	for i := range events {
		idx.Events[i] = events[i].Event
	}
	artifactSource.LocalLineCount = idx.LineCount
	artifactSource.EventCount = idx.ParsedKnown
	idx.TraceArtifacts = []TraceArtifactSource{artifactSource}
	flavorValue, confidence, signals, flavorCaveats := resolveTraceFlavor(idx, q)
	q.TraceFlavor = flavorValue
	for i := range events {
		events[i].Event = applyPriorityFlavor(events[i].Event, flavorValue)
	}
	frameworkSurfaces := detectFrameworkSurfaces(idx, q, TracePlatformAuto, 4)
	platform, platformCandidate, platformCandidateConfidence, platformCandidateSignals, platformCaveats := resolveTracePlatform(idx, q, flavorValue, idx.platformDetectionSurfaces(), signals)
	if platform == TracePlatformDonghu && q.TraceFlavorHintSource == "" && q.TraceFlavorHint != TraceFlavorAndroidAtrace {
		flavorValue = TraceFlavorHarmonyHitrace
		q.TraceFlavor = flavorValue
		if confidence < platformCandidateConfidence {
			confidence = platformCandidateConfidence
		}
		for i := range events {
			events[i].Event = applyPriorityFlavor(events[i].Event, flavorValue)
		}
	}
	start, end := q.TimeStart, q.TimeEnd
	if start == 0 && len(events) > 0 {
		start = events[0].Ts
	}
	if end == 0 && len(events) > 0 {
		end = events[len(events)-1].Ts
	}
	scopeKind := EventSearchScopeSelectedWindow
	if q.TimeStart == 0 && q.TimeEnd == 0 && q.LineStart == 0 && q.LineEnd == 0 {
		scopeKind = EventSearchScopeScanSegment
		if !seeked && startLine == 1 && reachedEOF {
			scopeKind = EventSearchScopeArtifact
		}
	}
	res := Result{
		View:                        "event_search",
		SourcePath:                  idx.Path,
		TraceArtifacts:              append([]TraceArtifactSource(nil), idx.TraceArtifacts...),
		TraceFlavor:                 string(flavorValue),
		Platform:                    string(platform),
		PlatformCandidate:           platformCandidate,
		PlatformCandidateConfidence: platformCandidateConfidence,
		PlatformCandidateSignals:    platformCandidateSignals,
		FlavorConfidence:            confidence,
		FlavorSignals:               signals,
		FrameworkMode:               FrameworkModeForPlatform(platform),
		FrameworkSurfaces:           frameworkSurfaces,
		TimeUnit:                    "seconds",
		PrioritySemantics:           PrioritySemanticsForFlavor(flavorValue),
		LineCount:                   idx.LineCount,
		ScannedLineCount:            idx.ScannedLineCount,
		UnparsedLineCount:           idx.UnparsedLines,
		ParseLinePanics:             idx.ParseLinePanics,
		ClockRegressions:            idx.ClockRegressions,
		EventCount:                  idx.ParsedKnown,
		TimeStart:                   start,
		TimeEnd:                     end,
		EventSearchCoverage: &EventSearchCoverage{
			ScopeKind:           scopeKind,
			ScopeTimeStart:      scopeTimeStart,
			ScopeTimeEnd:        scopeTimeEnd,
			ScopeTimestampRows:  scopeTimestampRows,
			ScopeComplete:       true,
			MatchedTimeStart:    matchedTimeStart,
			MatchedTimeEnd:      matchedTimeEnd,
			MatchedTotal:        matchedTotal,
			Emitted:             len(events),
			EnumerationComplete: true,
		},
		Events:       events,
		EvidencePack: evidenceFromEvents(events),
	}
	// RFC #71 (§8.2 c4): publish the pre-truncation tier ladder when the
	// display cap hid matched cpu_frequency rows, and lead the evidence pack
	// with it so the boundary truth reaches the typed observation face.
	if c := census.finalize(events); c != nil {
		res.CPUFrequencyCensus = c
		res.EvidencePack = append([]EvidenceFact{c.EvidenceFact()}, res.EvidencePack...)
	}
	// SA-F2 (DISPATCH-IND 批4, 2026-07-14): matched-rows generator census.
	res.VsyncGeneratorCensus = vsyncCensus.finalize(VsyncGeneratorCensusCaliberMatched)
	attachEvidenceFactProvenance(res.EvidencePack, res.TraceArtifacts)
	res.Caveats = append(res.Caveats,
		fmt.Sprintf("streamed_event_search=true; scanned %d line(s) without building or caching a full trace index", idx.ScannedLineCount))
	if perfIdentityRowsWithheld {
		res.Caveats = append(res.Caveats, "perf_thread_selector_withheld=true; reason=streaming_event_search_has_no_generation_ledger; perf_rows_withheld=true; retry with event_types=[perf_sample] to use the indexed typed identity authority")
	}
	// Same parse-quality caveat wording as the indexed Run() path: event_search
	// now always streams, so these coverage/quality signals must keep surfacing.
	if idx.ParseLinePanics > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("%d trace line(s) could not be parsed and were skipped; results may undercount events near those lines", idx.ParseLinePanics))
	}
	if idx.ClockRegressions > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("%d timestamp regression(s) detected in the trace (clock moved backwards); duration and ordering metrics around those points are unreliable", idx.ClockRegressions))
	}
	if idx.UnparsedLines > 0 && idx.ScannedLineCount > 0 && float64(idx.UnparsedLines) > unparsedLineCaveatRatio*float64(idx.ScannedLineCount) {
		res.Caveats = append(res.Caveats, fmt.Sprintf("%d of %d scanned lines did not match any known trace format; coverage may be incomplete", idx.UnparsedLines, idx.ScannedLineCount))
	}
	if matchedTotal > len(events) {
		last := events[len(events)-1]
		res.Compactions = append(res.Compactions, ViewCompaction{
			View:            "event_search",
			Dimension:       CompactionDimensionEvents,
			Total:           matchedTotal,
			Emitted:         len(events),
			LastEmittedTs:   last.Ts,
			LastEmittedLine: last.Line,
		})
		res.Caveats = append(res.Caveats,
			fmt.Sprintf("event_search_stream_compacted=true; matched %d row(s) but returned the first %d chronological match(es) only; omitted rows may contain later frame/span ids, so do not infer absence without rerunning an exact literal token", matchedTotal, len(events)))
	}
	res.Caveats = append(res.Caveats, flavorCaveats...)
	res.Caveats = append(res.Caveats, platformCaveats...)
	res.Caveats = append(res.Caveats, resultCaveats(idx, q, res)...)
	return res, nil
}

// insertEventViewChronological keeps the bounded display set equal to the
// earliest matches by (timestamp,line), even when physical trace lines regress.
// Memory remains O(limit); matchedTotal outside this helper still counts the
// complete pre-truncation set.
func insertEventViewChronological(events []EventView, candidate EventView, limit int) []EventView {
	if limit <= 0 {
		return events
	}
	pos := sort.Search(len(events), func(i int) bool {
		if events[i].Ts != candidate.Ts {
			return events[i].Ts > candidate.Ts
		}
		return events[i].Line > candidate.Line
	})
	if len(events) >= limit && pos >= limit {
		return events
	}
	events = append(events, EventView{})
	copy(events[pos+1:], events[pos:])
	events[pos] = candidate
	if len(events) > limit {
		events = events[:limit]
	}
	return events
}

// StreamStateCluster scans scheduler state boundaries without materializing the
// full trace index. It is the dense-window escape hatch for root-cause
// investigations: preserve the parent window and surface state priorities before
// asking a model to pick narrower windows.
func StreamStateCluster(ctx context.Context, path string, q Query, max int) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return Result{}, fmt.Errorf("trace path is empty")
	}
	path = canonicalTraceIndexPath(path)
	requiresComposite, err := tracePathRequiresCompositeIndexContext(ctx, path)
	if err != nil {
		return Result{}, err
	}
	if requiresComposite {
		return Result{}, fmt.Errorf("stream_state_cluster requires a single physical artifact; %s is a tracebundle, so use the indexed path to preserve artifact and clock-domain provenance", path)
	}
	initialIdentity, err := filegeneration.FromPath(path)
	if err != nil {
		return Result{}, err
	}
	f, openedIdentity, err := openTraceSourceRegularContext(ctx, path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	openedInfo, err := f.Stat()
	if err != nil {
		return Result{}, err
	}
	if !openedIdentity.SameVersion(initialIdentity) {
		return Result{}, fmt.Errorf("trace source identity changed before stream_state_cluster opened the artifact")
	}
	info := openedInfo
	if max <= 0 {
		max = StreamStateClusterDefaultMax
	}
	q.View = "window_stats"
	q = normalizeQuery(nil, q)
	idx := &Index{Path: path, Size: info.Size(), ModTime: info.ModTime()}
	if q.TimeStart > 0 || q.TimeEnd > 0 || q.LineStart > 0 || q.LineEnd > 0 {
		idx.Windowed = true
		idx.IndexTimeStart = q.TimeStart
		idx.IndexTimeEnd = q.TimeEnd
		idx.IndexLineStart = q.LineStart
		idx.IndexLineEnd = q.LineEnd
	}
	anchorKey := traceAnchorKeyForIdentity(path, openedIdentity)
	anchorSet := anchorCache.load(anchorKey)
	if anchorSet != nil {
		idx.TimestampOrder = anchorSet.TimestampOrder
	}
	recorder := newAnchorRecorder(anchorSet, traceAnchor{}, false)
	recording := recorder.canExtend(1)
	intern := newStringInterner()
	flavor := newFlavorVote(path)
	// W-1 修根 (platform_surfaces.go): all parsed events feed the platform
	// surface vote — same per-file single-authority lane as event_search.
	platformVote := newPlatformSurfaceVote()
	reachedEOF := false
	frozenSource, err := frozenTraceSectionAtCurrentOffset(f, openedIdentity)
	if err != nil {
		return Result{}, err
	}
	reader := bufio.NewReaderSize(frozenSource, 256*1024)
	open := map[int]stateChurnOpen{}
	accs := map[string]*stateChurnAcc{}
	running := map[string]ThreadDuration{}
	runnable := map[string]ThreadDuration{}
	sleep := map[string]ThreadDuration{}
	dstate := map[string]ThreadDuration{}
	iowait := map[string]ThreadDuration{}
	blockedReasons := map[int][]Event{}
	blockedReasonAmbiguousIntervals := 0
	blockedReasonCarryIntegrityDegraded := 0
	blockedReasonCarryCandidateSetTruncated := 0
	type pendingStateClusterInterval struct {
		start   stateChurnOpen
		endTs   float64
		endLine int
	}
	var pendingIntervals []pendingStateClusterInterval
	// Only a PID whose first unresolved D-family segment may need a physically
	// later blocked_reason row is deferred. Other PIDs and pre-D running/
	// runnable/S churn are independent ledgers and must not consume this budget.
	pendingBlockedPIDs := map[int]bool{}
	const pendingBlockedReasonIntervalCap = 4096
	const pendingBlockedReasonMarkerCap = 4096
	pendingBlockedReasonOverflow := false
	pendingBlockedReasonOverflowReason := ""
	blockedReasonMarkerCount := 0
	missingHeadThreads := map[int]bool{}
	orderTracker := newSchedulerOrderTracker()
	orderPIDTracker := newSchedulerOrderTracker()
	var schedulerViolation *schedulerOrderViolation
	var schedulerRowFailure *schedulerRowIntegrityFailure
	incarnationTracker := newThreadIncarnationTracker()
	var identityConflict *threadIncarnationConflict
	identityConflictPIDs := map[int]bool{}
	auditIntern := newStringInterner()
	auditScratch := &Index{}
	seenTimeWindow := false
	parsedEvents := 0
	lastObservedTs := 0.0
	lastObservedTsSet := false
	localClockRegressions := 0

	addPendingInterval := func(pending pendingStateClusterInterval, refine bool) {
		reasons := blockedReasons
		if !refine || blockedReasonRefinementUnavailableForInterval(idx, q, pending.start.thread.PID, pending.start.ts, pending.endTs, pending.endLine > 0) {
			reasons = nil
		}
		if addStreamStateClusterInterval(idx, accs, running, runnable, sleep, dstate, iowait,
			pending.start, pending.endTs, pending.endLine, q, reasons) {
			blockedReasonAmbiguousIntervals++
		}
	}
	flushPendingIntervals := func(refine bool) {
		for _, pending := range pendingIntervals {
			addPendingInterval(pending, refine)
		}
		pendingIntervals = pendingIntervals[:0]
	}
	triggerBlockedReasonOverflow := func(reason string) {
		if pendingBlockedReasonOverflow {
			return
		}
		pendingBlockedReasonOverflow = true
		pendingBlockedReasonOverflowReason = reason
		flushPendingIntervals(false)
		clear(blockedReasons)
		blockedReasonMarkerCount = 0
	}
	intervalOverlapsQuery := func(start stateChurnOpen, endTs float64) bool {
		if q.LineStart > 0 || q.LineEnd > 0 {
			return true
		}
		clampedStart, clampedEnd := start.ts, endTs
		if q.TimeStart > 0 && clampedStart < q.TimeStart {
			clampedStart = q.TimeStart
		}
		if q.TimeEnd > 0 && clampedEnd > q.TimeEnd {
			clampedEnd = q.TimeEnd
		}
		return clampedEnd > clampedStart
	}
	closeState := func(pid int, endTs float64, endLine int) {
		start, ok := open[pid]
		if !ok {
			return
		}
		delete(open, pid)
		pending := pendingStateClusterInterval{start: start, endTs: endTs, endLine: endLine}
		if !intervalOverlapsQuery(start, endTs) {
			return
		}
		if pendingBlockedReasonOverflow {
			addPendingInterval(pending, false)
			return
		}
		if !pendingBlockedPIDs[pid] && start.state != StateDSleep {
			addPendingInterval(pending, true)
			return
		}
		pendingBlockedPIDs[pid] = true
		if len(pendingIntervals) >= pendingBlockedReasonIntervalCap {
			// Cache state must never decide classification: cold and warm scans
			// use the same bounded full-window queue. On overflow, replay every
			// queued segment in closure order with generic D refinement.
			triggerBlockedReasonOverflow("interval_cap")
			addPendingInterval(pending, false)
			return
		}
		pendingIntervals = append(pendingIntervals, pending)
	}
	appendBlockedReasonMarker := func(ev Event, openingCarry bool) {
		if pendingBlockedReasonOverflow || ev.Type != EventSchedBlockedReason || ev.WakeePID <= 0 {
			return
		}
		markerRelevant := pendingBlockedPIDs[ev.WakeePID]
		current, openKnown := open[ev.WakeePID]
		if openKnown && current.state == StateDSleep {
			markerRelevant = true
		}
		inQuery := blockedReasonMarkerInQuery(ev, q)
		if !inQuery {
			inQuery = openingCarry && openKnown && current.state == StateDSleep &&
				ev.Ts > current.ts && ev.Ts <= wakeupClosingUpperBound(current.ts)
		}
		if !markerRelevant || !inQuery {
			return
		}
		if blockedReasonMarkerCount >= pendingBlockedReasonMarkerCap {
			triggerBlockedReasonOverflow("marker_cap")
			return
		}
		blockedReasons[ev.WakeePID] = append(blockedReasons[ev.WakeePID], ev)
		blockedReasonMarkerCount++
	}
	openState := func(thread ThreadRef, state ThreadState, ts float64, line int) {
		// §7.11 B-1 sequel (2026-07-04 review): same gate as the indexed twin
		// (computeStateChurnSummaries) — stopped/dead segments never open. The
		// five-state lanes skip them, so they would inflate ONLY fragments/
		// switches/maxSegment and let a dead exit tail suppress a real churn
		// row via the maxSegment>=70% gate downstream. Shared authority:
		// stateChurnOpenIneligible (thread_state_universe.go).
		if stateChurnOpenIneligible(thread, state) {
			return
		}
		open[thread.PID] = stateChurnOpen{thread: thread, state: state, ts: ts, line: line}
	}

	var scan lineScan
	for lineNo := 1; ; lineNo++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		line, readErr := readStreamScanPhysicalLine(reader, attachment.TracePhysicalLineMaxBytes)
		if len(line) > 0 {
			idx.LineCount = lineNo
			idx.ScannedLineCount = lineNo
			trimmed := strings.TrimRight(line, "\r\n")
			scan.reset(lineNo, trimmed)
			lineTs, lineHasTS := scan.timestamp()
			preWindowCarry := false
			closingMarkerCarry := false
			if recording {
				recorder.observe(lineNo, len(line), lineTs, lineHasTS, trimmed)
			}
			if q.LineEnd > 0 && lineNo > q.LineEnd {
				break
			}
			if failure := blockedReasonValidationFailureScan(&scan); failure != nil {
				openingCarry := false
				if q.TimeStart > 0 && failure.Ts < q.TimeStart && blockedReasonFailureHasIdentityIssue(*failure) {
					if failure.PIDCandidateSetTruncated {
						for _, current := range open {
							if current.state == StateDSleep && failure.Ts > current.ts && failure.Ts <= wakeupClosingUpperBound(current.ts) {
								openingCarry = true
								break
							}
						}
					} else {
						for _, pid := range failure.PIDs {
							current, ok := open[pid]
							if pid > 0 && ok && current.state == StateDSleep && failure.Ts > current.ts && failure.Ts <= wakeupClosingUpperBound(current.ts) {
								openingCarry = true
								break
							}
						}
					}
				}
				if blockedReasonIntegrityFailureRelevantToQuery(failure, q, q.PID) || openingCarry {
					failure.SourcePath = path
					appendBlockedReasonIntegrityFailure(idx, *failure)
					if openingCarry {
						blockedReasonCarryIntegrityDegraded++
						if failure.PIDCandidateSetTruncated {
							blockedReasonCarryCandidateSetTruncated++
						}
					}
					if blockedReasonFailureHasIdentityIssue(*failure) {
						if openingCarry && failure.PIDCandidateSetTruncated {
							// The retained PID candidates are only a prefix of this
							// physical row. Match indexed head semantics: every D slice
							// that was open within the opening-side tolerance is a
							// possible owner and must receive an opaque contender. The
							// shared marker cap bounds this fan-out and fails the whole
							// stream refinement closed on exhaustion.
							for pid, current := range open {
								if pid > 0 && current.state == StateDSleep && failure.Ts > current.ts && failure.Ts <= wakeupClosingUpperBound(current.ts) {
									appendBlockedReasonMarker(blockedReasonOpaqueIdentityCandidate(*failure, pid), true)
								}
							}
						} else {
							for _, pid := range failure.PIDs {
								if pid > 0 {
									appendBlockedReasonMarker(blockedReasonOpaqueIdentityCandidate(*failure, pid), openingCarry)
								}
							}
						}
					}
				}
			}
			if schedulerIntegrityRawCandidate(trimmed) {
				rowFailure := schedulerRowValidationFailureScan(&scan)
				if rowFailure != nil && schedulerRowFailure == nil && schedulerRowIntegrityFailureRelevantToQuery(rowFailure, q, q.PID) {
					copy := *rowFailure
					copy.SourcePath = path
					schedulerRowFailure = &copy
				}
				if rowFailure == nil && schedulerHeadRawCandidate(trimmed) {
					if auditEv, auditOK := safeParseLineScan(&scan, auditIntern, auditScratch); auditOK {
						// observeAll contract (ENG audit #42): iterate the FULL
						// conflict slice and apply the window-relevance
						// predicate per conflict — the tracker permanently
						// consumes lifecycle evidence for every conflict it
						// emits, so keeping only the first element would
						// silently discard siblings with different relevance.
						for _, conflict := range incarnationTracker.observeAll(auditEv, 0) {
							if incarnationBoundaryInsideQuery(&conflict, q) {
								identityConflictPIDs[conflict.PID] = true
								if identityConflict == nil {
									copy := conflict
									identityConflict = &copy
								}
							}
						}
						lineInOrderDomain := q.LineStart <= 0 || lineNo >= q.LineStart
						if schedulerViolation == nil && lineInOrderDomain {
							for _, violation := range auditSchedulerOrderEvent(orderTracker, orderPIDTracker, auditEv) {
								if schedulerOrderViolationRelevantToQuery(&violation, q, 0) {
									copy := violation
									schedulerViolation = &copy
									break
								}
							}
						}
					} else if schedulerRowFailure == nil {
						if rejected := schedulerRejectedRowFailure(lineNo, trimmed); rejected != nil && schedulerRowIntegrityFailureRelevantToQuery(rejected, q, q.PID) {
							rejected.SourcePath = path
							schedulerRowFailure = rejected
						}
					}
				}
			}
			if q.LineStart > 0 && lineNo < q.LineStart {
				if lineNo <= 200 {
					flavor.observeRawLine(trimmed)
				}
				goto nextLine
			}
			if lineHasTS {
				if lastObservedTsSet && lineTs < lastObservedTs {
					localClockRegressions++
				}
				lastObservedTs, lastObservedTsSet = lineTs, true
			}
			if q.TimeStart > 0 || q.TimeEnd > 0 {
				ts, hasTS := lineTs, lineHasTS
				if hasTS {
					if q.TimeEnd > 0 && ts > q.TimeEnd {
						// A Donghu blocked marker may be emitted during wakeup
						// processing just after the physical sleep boundary. Admit
						// only that exact event family through the matcher's one
						// 5µs closing tail; it is refinement carry, not an in-window
						// event and must not advance any scheduler state or count.
						if ts <= wakeupClosingUpperBound(q.TimeEnd) {
							name, exact := ProbeLeadingExactEventNamePrefix(trimmed)
							if !exact || name != "sched_blocked_reason" {
								goto nextLine
							}
							closingMarkerCarry = true
						} else {
							if idx.TimestampOrder.AllowsTimeEndEarlyStop() {
								break
							}
							goto nextLine
						}
					}
					if q.TimeStart > 0 && ts < q.TimeStart {
						if lineNo <= 200 {
							flavor.observeRawLine(trimmed)
						}
						// State-cluster is itself the streaming escape hatch, so it
						// already reads the prefix. Parse only scheduler-boundary rows
						// there to carry the governing state into the selected window;
						// all other pre-window rows keep the cheap skip path.
						if !schedulerHeadRawCandidate(trimmed) {
							goto nextLine
						}
						preWindowCarry = true
					} else {
						// Only rows actually AT/inside the time window arm the
						// "seen window" latch. A carry row falling through here
						// used to set it too, which stripped every later
						// pre-window no-timestamp line of the cheap skip path
						// below (audit #53).
						seenTimeWindow = true
					}
				} else if q.TimeStart > 0 && !seenTimeWindow {
					if lineNo <= 200 {
						flavor.observeRawLine(trimmed)
					}
					goto nextLine
				}
			}
			flavor.observeRawLine(trimmed)
			// Parse-quality counters mirror the sibling streaming lanes
			// (StreamEventSearch / StreamWindowSweep) and the indexed Run()
			// path: this face is the index_event_limit fallback for exactly
			// the dense/degraded traces where unparseable rows mean
			// systematically undercounted five-state durations, so the
			// coverage disclosure must not silently disappear here
			// (audit #52).
			panicsBefore := idx.ParseLinePanics
			ev, ok := safeParseLineScan(&scan, intern, idx)
			if !ok {
				if closingMarkerCarry {
					goto nextLine
				}
				if trimmed != "" {
					if idx.ParseLinePanics == panicsBefore {
						idx.UnparsedLines++
					}
					idx.recordUnparsedSample(lineNo, trimmed)
				}
				if schedulerRowFailure == nil {
					if rejected := schedulerRejectedRowFailure(lineNo, trimmed); rejected != nil && schedulerRowIntegrityFailureRelevantToQuery(rejected, q, q.PID) {
						rejected.SourcePath = path
						schedulerRowFailure = rejected
					}
				}
				goto nextLine
			}
			if !preWindowCarry && !closingMarkerCarry {
				parsedEvents++
				if idx.FirstTs == 0 || ev.Ts < idx.FirstTs {
					idx.FirstTs = ev.Ts
				}
				if ev.Ts > idx.LastTs {
					idx.LastTs = ev.Ts
				}
				if ev.Type != EventUnknown {
					idx.ParsedKnown++
				}
				if countTraceDBTextRecord(idx, ev) || sourceRawVisibilityAdvisory(ev) {
					goto nextLine
				}
			}
			if !closingMarkerCarry {
				flavor.observeEvent(ev)
				platformVote.observe(ev)
			}
			appendBlockedReasonMarker(ev, preWindowCarry)
			if closingMarkerCarry {
				goto nextLine
			}
			switch ev.Type {
			case EventSchedWakeup, EventSchedWaking:
				if ev.WakeePID <= 0 {
					continue
				}
				start, ok := open[ev.WakeePID]
				if !preWindowCarry && q.TimeStart > 0 && ev.Ts > q.TimeStart && !ok && !schedWakeupStartsNewIncarnation(ev) {
					// Disclosure retained alongside the mint below: the mark
					// says the PREFIX before this wakeup is un-witnessed,
					// which stays true — minting the witnessed suffix segment
					// is orthogonal (headless-wakeup alignment ruling).
					missingHeadThreads[ev.WakeePID] = true
				}
				// Shared wakeup-reopen guard (thread_state_universe.go) —
				// same gate as the indexed churn face. A normal wakeup for a
				// thread with NO governing open state mints a runnable
				// segment from the wakeup timestamp on BOTH lanes:
				// pre-window carry rows mirror the indexed head authority
				// (applySchedulerHeadEvent — audit #48), and in-window rows
				// follow the headless-wakeup alignment ruling (主会话裁定
				// 2026-07-10, §29.26 待落账): the wakeup is a witnessed typed
				// transition, not a guess, and the indexed offCPU face
				// (computeOffCPUStats) has always minted it — the
				// index_event_limit fallback face must give the same numbers.
				// The un-witnessed prefix stays disclosed via
				// missingHeadThreads above (partial_unknown). Convergence is
				// bounded: any later sched_switch naming the pid closes the
				// segment, and repeat wakeups are rejected by the
				// reopen-ineligible arm.
				if !schedWakeupStartsNewIncarnation(ev) && ok && (stateChurnWakeupReopenIneligible(start.state) || ev.Ts < start.ts) {
					continue
				}
				if ok {
					closeState(ev.WakeePID, ev.Ts, ev.Line)
				}
				openState(ThreadRef{Comm: ev.WakeeComm, PID: ev.WakeePID}, StateRunnable, ev.Ts, ev.Line)
			case EventSchedSwitch:
				if !preWindowCarry && q.TimeStart > 0 && ev.Ts > q.TimeStart {
					if ev.NextPID > 0 {
						if _, known := open[ev.NextPID]; !known {
							missingHeadThreads[ev.NextPID] = true
						}
					}
					if ev.PrevPID > 0 {
						if _, known := open[ev.PrevPID]; !known {
							missingHeadThreads[ev.PrevPID] = true
						}
					}
				}
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
	nextLine:
		if readErr != nil {
			if readErr == io.EOF {
				reachedEOF = true
				if recording {
					recorder.finishEOF()
				}
				break
			}
			return Result{}, traceReadErrorAfterIdentity(f, openedIdentity, "stream_state_cluster physical read", readErr)
		}
	}
	if err := validateTraceFileIdentityAfterRead(f, openedIdentity, "stream_state_cluster"); err != nil {
		return Result{}, err
	}
	endTs := q.TimeEnd
	if endTs == 0 && idx.LastTs > 0 {
		endTs = idx.LastTs
	}
	streamArtifactTailUncovered := q.TimeEndSet && reachedEOF && idx.Size > 0 && idx.LastTs > 0 && q.TimeEnd > idx.LastTs
	if streamArtifactTailUncovered {
		endTs = idx.LastTs
	}
	for pid := range open {
		closeState(pid, endTs, 0)
	}
	flushPendingIntervals(!pendingBlockedReasonOverflow)
	idx.TraceFlavor, idx.FlavorConfidence, idx.FlavorSignals = flavor.result()
	if recording {
		if !recorder.set.FlavorSet {
			recorder.set.FlavorSet = true
			recorder.set.Flavor = idx.TraceFlavor
			recorder.set.FlavorConf = idx.FlavorConfidence
			recorder.set.FlavorSignals = append([]string(nil), idx.FlavorSignals...)
		}
		// W-1 修根 + 复核 F2: complete-coverage from-0 scans only, write-once.
		if !recorder.set.PlatformSurfaces.Set && platformSurfaceMintEligible(q, reachedEOF) {
			recorder.set.PlatformSurfaces = platformVote.result(false)
		}
		anchorCache.store(anchorKey, recorder.set)
	}
	if anchorSet != nil && anchorSet.PlatformSurfaces.Set {
		idx.platformSurfaces = anchorSet.PlatformSurfaces.clone()
	} else if recorder.set.PlatformSurfaces.Set {
		idx.platformSurfaces = recorder.set.PlatformSurfaces.clone()
	} else {
		idx.platformSurfaces = platformVote.result(!platformSurfaceMintEligible(q, reachedEOF))
	}
	idx.TimestampOrder = recorder.set.TimestampOrder
	if idx.TimestampOrder != TraceTimestampOrderUnknown {
		idx.ClockRegressions = recorder.set.CoveredClockRegressions
	} else {
		idx.ClockRegressions = localClockRegressions
	}
	source := singleTraceArtifactSourceWithIdentity(path, openedIdentity, idx.LineCount, parsedEvents)
	source.timestampOrder = idx.TimestampOrder
	source.clockRegressions = idx.ClockRegressions
	idx.TraceArtifacts = []TraceArtifactSource{source}
	flavorValue, confidence, signals, flavorCaveats := resolveTraceFlavor(idx, q)
	q.TraceFlavor = flavorValue
	frameworkSurfaces := detectFrameworkSurfaces(idx, q, TracePlatformAuto, 4)
	platform, platformCandidate, platformCandidateConfidence, platformCandidateSignals, platformCaveats := resolveTracePlatform(idx, q, flavorValue, idx.platformDetectionSurfaces(), signals)
	blockedIntegrityCaveats := blockedReasonIntegrityCaveats(idx, q, q.PID)
	filterCaveats, selectorRejection := streamStateClusterApplyThreadFilter(q, accs, running, runnable, sleep, dstate, iowait, missingHeadThreads)
	var stateIntegrityCaveats []string
	if schedulerViolation != nil {
		clearStreamStateCluster(accs, running, runnable, sleep, dstate, iowait)
		for pid := range missingHeadThreads {
			delete(missingHeadThreads, pid)
		}
		stateIntegrityCaveats = append(stateIntegrityCaveats, "stream_state_cluster_fail_closed=true; "+schedulerViolation.reason()+"; scheduler durations were discarded because same-lane clock rollback has no provable elapsed-time ordering")
	}
	if schedulerRowFailure != nil {
		clearStreamStateCluster(accs, running, runnable, sleep, dstate, iowait)
		for pid := range missingHeadThreads {
			delete(missingHeadThreads, pid)
		}
		stateIntegrityCaveats = append(stateIntegrityCaveats, "stream_state_cluster_fail_closed=true; "+schedulerRowFailure.reason()+"; scheduler durations were discarded because a critical scheduler row was incomplete")
	}
	if identityConflict != nil {
		for key, acc := range accs {
			if acc != nil && identityConflictPIDs[acc.thread.PID] {
				delete(accs, key)
			}
		}
		for _, bucket := range []map[string]ThreadDuration{running, runnable, sleep, dstate, iowait} {
			for key, td := range bucket {
				if identityConflictPIDs[td.Thread.PID] {
					delete(bucket, key)
				}
			}
		}
		for pid := range identityConflictPIDs {
			delete(missingHeadThreads, pid)
		}
		stateIntegrityCaveats = append(stateIntegrityCaveats, fmt.Sprintf(
			"stream_state_cluster_per_pid_fail_closed=true; thread_identity_fail_closed=true; thread_identity_per_pid_filtered=true; %s; suppressed_pids=%v; conflicting PID state durations were discarded while clean PID rows were retained",
			identityConflict.reason(), sortedIntSet(identityConflictPIDs)))
	}
	var headCoverage *SchedulerHeadCoverage
	if q.TimeStart > 0 && q.LineStart == 0 && q.LineEnd == 0 {
		headCoverage = &SchedulerHeadCoverage{BoundaryTs: q.TimeStart}
		if schedulerRowFailure != nil {
			headCoverage.Status = "unknown"
			headCoverage.Reason = "scheduler_row_parse_incomplete"
			headCoverage.SubjectCensusStatus = "not_evaluated"
		} else if schedulerViolation != nil {
			headCoverage.Status = "unknown"
			headCoverage.Reason = "scheduler_lane_timestamp_regressed"
			headCoverage.SubjectCensusStatus = "not_evaluated"
		} else if identityConflict != nil {
			headCoverage.Status = "unknown"
			headCoverage.Reason = "thread_incarnation_conflict"
			headCoverage.SubjectCensusStatus = "not_evaluated"
		} else if selectorRejection != "" {
			// A selector rejection (ambiguous / unresolved thread name)
			// clears every bucket AND the missingHeadThreads set, so the
			// empty set must not fall through to the "recovered" arm: the
			// typed head-coverage face would then contradict the rejection
			// caveat on the same result (audit #49). Same shape as the three
			// integrity arms above: unknown + typed reason.
			headCoverage.Status = "unknown"
			headCoverage.Reason = selectorRejection
			headCoverage.SubjectCensusStatus = "not_evaluated"
		} else if len(missingHeadThreads) > 0 {
			headCoverage.Status = "partial_unknown"
			headCoverage.Reason = "subject_checkpoint_missing"
			headCoverage.SubjectCensusStatus = "evaluated"
			headCoverage.MissingThreadCount = len(missingHeadThreads)
			headCoverage.MissingThreadPIDs = sortedBoundedIntSet(missingHeadThreads, schedulerHeadMissingSubjectDisplayCap)
			stateIntegrityCaveats = append(stateIntegrityCaveats, fmt.Sprintf("scheduler_head_subjects_unknown=true; stream prefix had no governing state for %d in-window thread(s) %v, so their pre-first-event head segments are omitted", headCoverage.MissingThreadCount, headCoverage.MissingThreadPIDs))
		} else {
			headCoverage.Status = "recovered"
			headCoverage.SubjectCensusStatus = "evaluated"
		}
	}

	stats := WindowStats{
		Window:                queryResultTimeWindow(q),
		SchedulerHeadCoverage: headCoverage,
		TopRunning:            streamStateTopDurations(running, max),
		RunnableTop:           streamStateTopDurations(runnable, max),
		SleepTop:              streamStateTopDurations(sleep, max),
		DStateTop:             streamStateTopDurations(dstate, max),
		IOWaitTop:             streamStateTopDurations(iowait, max),
		StateChurn:            streamStateClusterSummaries(accs, max),
		Caveats: []string{
			"stream_state_cluster=true; derived without materializing the full trace index",
			"state_cluster is parent-window coverage for prioritizing drilldown; root_cause_rank/frame_root_cause_bundle may still be needed on bounded phase windows",
		},
	}
	stats.Caveats = append(stats.Caveats, stateIntegrityCaveats...)
	if streamArtifactTailUncovered {
		stats.Caveats = append(stats.Caveats, schedulerArtifactTailCaveat(q, endTs))
	}
	stats.Caveats = append(stats.Caveats, filterCaveats...)
	stats.Caveats = append(stats.Caveats, blockedIntegrityCaveats...)
	if blockedReasonAmbiguousIntervals > 0 {
		stats.Caveats = append(stats.Caveats, fmt.Sprintf(
			"blocked_reason_marker_ambiguous=true ambiguous_intervals=%d; no I/O/non-I/O classification or caller proof was minted for those intervals",
			blockedReasonAmbiguousIntervals))
	}
	if blockedReasonCarryIntegrityDegraded > 0 {
		stats.Caveats = append(stats.Caveats, fmt.Sprintf("blocked_reason_integrity_degraded=true; opening_side_carry_identity_failures=%d; malformed pre-window marker identity was retained as opaque interval inventory and cannot grant I/O/non-I/O or caller classification", blockedReasonCarryIntegrityDegraded))
	}
	if blockedReasonCarryCandidateSetTruncated > 0 {
		stats.Caveats = append(stats.Caveats, fmt.Sprintf("blocked_reason_pid_candidate_set_truncated=true; opening_side_carry_truncated_rows=%d affected_pid_scope=all_open_d; canonical PID candidate inventory exceeded its deterministic bound, so every contemporaneous open D interval stays generic", blockedReasonCarryCandidateSetTruncated))
	}
	if pendingBlockedReasonOverflow {
		stats.Caveats = append(stats.Caveats, fmt.Sprintf(
			"blocked_reason_pending_interval_audit_truncated=true; reason=%s interval_cap=%d marker_cap=%d; marker-based D/IO refinement was withdrawn deterministically for this stream while generic D wall-clock accounting and closure-order churn were preserved",
			pendingBlockedReasonOverflowReason, pendingBlockedReasonIntervalCap, pendingBlockedReasonMarkerCap))
	}
	stats.StateDrilldownPlan, stats.IdleWholeWindowSleepers = buildStateDrilldownPlanForTarget(stats, max, q.PID, q.Thread)
	if q.PID > 0 || strings.TrimSpace(q.ThreadInput) != "" || strings.TrimSpace(q.Thread) != "" {
		stats.Caveats = append(stats.Caveats, "state_cluster filter="+streamStateClusterFilterLabel(q))
	}
	if len(stats.StateChurn) == 0 && len(stats.TopRunning) == 0 && len(stats.RunnableTop) == 0 && len(stats.SleepTop) == 0 && len(stats.DStateTop) == 0 && len(stats.IOWaitTop) == 0 {
		stats.Caveats = append(stats.Caveats, "state_cluster produced no scheduler state intervals in the selected scope; verify time/line/thread filters before making absence claims")
	}
	caveats := append([]string{
		"mode=stream_state_cluster",
	}, flavorCaveats...)
	caveats = append(caveats, platformCaveats...)
	caveats = append(caveats, stats.Caveats...)
	// Same parse-quality caveat wording as the sibling streaming lanes and the
	// indexed Run() path (audit #52): this fallback face serves the densest /
	// most degraded traces, so parse-coverage disclosure must not be the one
	// lane that stays silent.
	if idx.ParseLinePanics > 0 {
		caveats = append(caveats, fmt.Sprintf("%d trace line(s) could not be parsed and were skipped; results may undercount events near those lines", idx.ParseLinePanics))
	}
	if idx.ClockRegressions > 0 {
		caveats = append(caveats, fmt.Sprintf("%d timestamp regression(s) detected in the trace (clock moved backwards); duration and ordering metrics around those points are unreliable", idx.ClockRegressions))
	}
	if idx.UnparsedLines > 0 && idx.ScannedLineCount > 0 && float64(idx.UnparsedLines) > unparsedLineCaveatRatio*float64(idx.ScannedLineCount) {
		caveats = append(caveats, fmt.Sprintf("%d of %d scanned lines did not match any known trace format; coverage may be incomplete", idx.UnparsedLines, idx.ScannedLineCount))
	}
	return Result{
		View:                        "window_stats",
		SourcePath:                  path,
		TraceArtifacts:              append([]TraceArtifactSource(nil), idx.TraceArtifacts...),
		TraceFlavor:                 string(flavorValue),
		Platform:                    string(platform),
		PlatformCandidate:           string(platformCandidate),
		PlatformCandidateConfidence: platformCandidateConfidence,
		PlatformCandidateSignals:    platformCandidateSignals,
		FlavorConfidence:            confidence,
		FlavorSignals:               signals,
		FrameworkMode:               FrameworkModeForPlatform(platform),
		FrameworkSurfaces:           frameworkSurfaces,
		TimeUnit:                    "seconds",
		PrioritySemantics:           PrioritySemanticsForFlavor(flavorValue),
		LineCount:                   idx.LineCount,
		ScannedLineCount:            idx.ScannedLineCount,
		UnparsedLineCount:           idx.UnparsedLines,
		ParseLinePanics:             idx.ParseLinePanics,
		IndexWindowed:               idx.Windowed,
		IndexTimeStart:              idx.IndexTimeStart,
		IndexTimeEnd:                idx.IndexTimeEnd,
		IndexLineStart:              idx.IndexLineStart,
		IndexLineEnd:                idx.IndexLineEnd,
		EventCount:                  parsedEvents,
		ClockRegressions:            idx.ClockRegressions,
		TimeStart:                   q.TimeStart,
		TimeEnd:                     q.TimeEnd,
		WindowStats:                 &stats,
		Caveats:                     dedupStrings(caveats),
	}, nil
}

func addStreamStateClusterInterval(idx *Index, accs map[string]*stateChurnAcc, running, runnable, sleep, dstate, iowait map[string]ThreadDuration, start stateChurnOpen, endTs float64, endLine int, q Query, blockedReasons map[int][]Event) (ambiguous bool) {
	if endTs <= start.ts {
		return false
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
		return false
	}
	state := start.state
	if state == StateDSleep {
		match := blockedReasonIntervalMatch{}
		if !blockedReasonRefinementUnavailableForInterval(idx, q, start.thread.PID, start.ts, endTs, endLine > 0) {
			match = matchBlockedReasonForIntervalAtClosure(blockedReasons, start.thread, start.ts, endTs, endLine > 0)
		}
		ambiguous = match.Ambiguous
		if reason := match.Event; reason != nil && reason.IOWait > 0 {
			state = StateIOWait
			endLine = firstPositive(endLine, reason.Line)
		}
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
	td := ThreadDuration{
		Thread:     start.thread,
		DurationMs: durationMs,
		StartTs:    clampedStart,
		EndTs:      clampedEnd,
		LineStart:  start.line,
		LineEnd:    firstPositive(endLine, start.line),
		CPU:        -1,
	}
	zeroStartReal := queryWindowStartsAtDeterminedZero(q)
	switch state {
	case StateRunning:
		acc.runningMs += durationMs
		streamStateAccumulateDuration(running, td, zeroStartReal)
	case StateRunnable:
		acc.runnableMs += durationMs
		streamStateAccumulateDuration(runnable, td, zeroStartReal)
	case StateSSleep:
		acc.sleepMs += durationMs
		streamStateAccumulateDuration(sleep, td, zeroStartReal)
	case StateDSleep:
		acc.dStateMs += durationMs
		streamStateAccumulateDuration(dstate, td, zeroStartReal)
	case StateIOWait:
		acc.ioWaitMs += durationMs
		streamStateAccumulateDuration(iowait, td, zeroStartReal)
	}
	return ambiguous
}

func streamStateClusterSummaries(accs map[string]*stateChurnAcc, max int) []ThreadStateChurnSummary {
	out := make([]ThreadStateChurnSummary, 0, len(accs))
	for _, acc := range accs {
		if acc == nil {
			continue
		}
		total := acc.runningMs + acc.runnableMs + acc.sleepMs + acc.dStateMs + acc.ioWaitMs
		if total <= 0 {
			continue
		}
		dominantState, dominantMs := dominantChurnState(acc)
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
			P95SegmentMs:     percentileFloat64(acc.segments, 0.95),
			LineStart:        acc.lineStart,
			LineEnd:          acc.lineEnd,
			Confidence:       streamStateClusterConfidence(acc, total, dominantMs),
			NextStep:         stateChurnNextStep(dominantState),
		}
		item.Summary = fmt.Sprintf("state_cluster %s dominant_state=%s impact=%.3fms total=%.3fms running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms io_wait=%.3fms; next_step=%s",
			threadLabel(item.Thread), item.DominantState, item.DominantImpactMs, item.TotalMs, item.RunningMs, item.RunnableMs, item.SleepMs, item.DStateMs, item.IOWaitMs, item.NextStep)
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DominantImpactMs != out[j].DominantImpactMs {
			return out[i].DominantImpactMs > out[j].DominantImpactMs
		}
		return out[i].LineStart < out[j].LineStart
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func streamStateClusterConfidence(acc *stateChurnAcc, total, dominant float64) float64 {
	conf := 0.68
	if dominant >= 5 {
		conf += 0.06
	}
	if total >= 20 {
		conf += 0.05
	}
	if acc.fragmentCount >= 4 {
		conf += 0.04
	}
	if acc.lineStart > 0 && acc.lineEnd >= acc.lineStart {
		conf += 0.03
	}
	if conf > 0.86 {
		return 0.86
	}
	return conf
}

// WINFLAG-1 (c) (§29.190④): zeroStartReal is the flagged [0,end]-run gate.
// Every ThreadDuration minted by addStreamStateClusterInterval carries real
// clamped endpoints, so under the flag the start envelope is a plain min —
// the legacy arm reads StartTs==0 as "unset yet" and would both let a later
// positive segment RAISE a real 0 envelope and refuse to let a real
// 0-starting segment lower it. Without the flag the merge is byte-identical
// to the legacy form.
func streamStateAccumulateDuration(dst map[string]ThreadDuration, td ThreadDuration, zeroStartReal bool) {
	if td.DurationMs <= 0 || td.Thread.PID <= 0 {
		return
	}
	key := threadKey(td.Thread)
	existing := dst[key]
	if existing.Thread.PID == 0 {
		dst[key] = td
		return
	}
	existing.DurationMs += td.DurationMs
	if td.EndTs > existing.EndTs || (td.EndTs == existing.EndTs && threadDisplayLess(td.Thread, existing.Thread)) {
		existing.Thread = td.Thread
	}
	if zeroStartReal {
		if td.StartTs < existing.StartTs {
			existing.StartTs = td.StartTs
		}
	} else if existing.StartTs == 0 || (td.StartTs > 0 && td.StartTs < existing.StartTs) {
		existing.StartTs = td.StartTs
	}
	if td.EndTs > existing.EndTs {
		existing.EndTs = td.EndTs
	}
	if existing.LineStart == 0 || (td.LineStart > 0 && td.LineStart < existing.LineStart) {
		existing.LineStart = td.LineStart
	}
	if td.LineEnd > existing.LineEnd {
		existing.LineEnd = td.LineEnd
	}
	dst[key] = existing
}

func streamStateTopDurations(in map[string]ThreadDuration, max int) []ThreadDuration {
	out := make([]ThreadDuration, 0, len(in))
	for _, td := range in {
		out = append(out, td)
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

func clearStreamStateCluster(accs map[string]*stateChurnAcc, buckets ...map[string]ThreadDuration) {
	for key := range accs {
		delete(accs, key)
	}
	for _, bucket := range buckets {
		for key := range bucket {
			delete(bucket, key)
		}
	}
}

// streamStateClusterApplyThreadFilter prunes the accumulated buckets to the
// selected thread. It filters by exact TID only — stream ThreadRefs carry no
// TGID, so pid=<process pid> deliberately selects nothing here (the
// relation-scope pass in parse_relation_scope.go is the face that expands a
// process id into its member threads). The second return value is the typed
// rejection reason ("thread_selector_ambiguous" / "thread_selector_unresolved",
// "" when the selector resolved): rejections clear every bucket AND the
// missingHeadThreads set, so headCoverage must consume this reason instead of
// reading the cleared set as "recovered" (audit #49).
func streamStateClusterApplyThreadFilter(q Query, accs map[string]*stateChurnAcc, running, runnable, sleep, dstate, iowait map[string]ThreadDuration, missingHeadThreads map[int]bool) ([]string, string) {
	selector := strings.TrimSpace(firstNonEmpty(q.ThreadInput, q.Thread))
	targetPID := q.PID
	var resolution threadResolution
	if targetPID <= 0 && selector != "" {
		refs := make([]ThreadRef, 0, len(accs))
		for _, acc := range accs {
			if acc != nil {
				refs = append(refs, acc.thread)
			}
		}
		resolution = resolveThreadRefs(selector, refs)
		if resolution.Ambiguous {
			clearStreamStateCluster(accs, running, runnable, sleep, dstate, iowait)
			for pid := range missingHeadThreads {
				delete(missingHeadThreads, pid)
			}
			return []string{fmt.Sprintf("thread_selector_ambiguous selector=%q candidate_pids=%s; stream_state_cluster refuses to merge same-name scheduler threads — rerun with pid=<tid>", selector, joinThreadResolutionPIDs(resolution.CandidatePIDs))}, "thread_selector_ambiguous"
		}
		targetPID = resolution.Thread.PID
		if targetPID <= 0 {
			clearStreamStateCluster(accs, running, runnable, sleep, dstate, iowait)
			for pid := range missingHeadThreads {
				delete(missingHeadThreads, pid)
			}
			return []string{fmt.Sprintf("thread_selector_unresolved selector=%q; stream_state_cluster found no unique scheduler TID in the selected window", selector)}, "thread_selector_unresolved"
		}
	}
	if targetPID <= 0 {
		return nil, ""
	}
	for key, acc := range accs {
		if acc == nil || acc.thread.PID != targetPID {
			delete(accs, key)
		}
	}
	for _, bucket := range []map[string]ThreadDuration{running, runnable, sleep, dstate, iowait} {
		for key, td := range bucket {
			if td.Thread.PID != targetPID {
				delete(bucket, key)
			}
		}
	}
	for pid := range missingHeadThreads {
		if pid != targetPID {
			delete(missingHeadThreads, pid)
		}
	}
	return nil, ""
}

func streamStateClusterFilterLabel(q Query) string {
	if q.PID > 0 {
		return fmt.Sprintf("pid=%d", q.PID)
	}
	if thread := strings.TrimSpace(firstNonEmpty(q.ThreadInput, q.Thread)); thread != "" {
		return "thread=" + thread
	}
	return ""
}

func streamEventSearchRawCandidate(line string, lineNo int, q Query) bool {
	if !eventSearchHasLiteralPatterns(q) {
		return true
	}
	lower := strings.ToLower(line)
	lineNumber := strconv.Itoa(lineNo)
	matches := func(raw string) bool {
		pattern := strings.ToLower(raw)
		return pattern != "" && (strings.Contains(lower, pattern) || strings.Contains(lineNumber, pattern))
	}
	if matches(strings.TrimSpace(q.Pattern)) {
		return true
	}
	for _, raw := range q.Patterns {
		if matches(strings.TrimSpace(raw)) {
			return true
		}
	}
	return false
}
