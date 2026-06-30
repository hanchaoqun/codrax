package tracequery

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// StreamEventSearch scans a trace for event_search rows without materializing a
// full Index. It is intended for large unbounded discovery calls where the
// model is looking for a frame id, timestamp token, span label, or resource key
// before carrying a bounded window into heavier views.
func StreamEventSearch(ctx context.Context, path string, q Query) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return Result{}, fmt.Errorf("trace path is empty")
	}
	path = canonicalTraceIndexPath(path)
	info, err := os.Stat(path)
	if err != nil {
		return Result{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()

	q.View = "event_search"
	q = normalizeQuery(nil, q)
	typeSet := make(map[EventType]bool, len(q.EventTypes))
	for _, typ := range q.EventTypes {
		if typ != "" {
			typeSet[typ] = true
		}
	}

	idx := &Index{Path: path, Size: info.Size(), ModTime: info.ModTime()}
	intern := newStringInterner()
	flavor := newFlavorVote(path)
	reader := bufio.NewReaderSize(f, 256*1024)
	seenTimeWindow := false
	limit := q.Limit
	if limit <= 0 {
		limit = 40
	}
	matchedTotal := 0
	var events []EventView
	for lineNo := 1; ; lineNo++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			idx.LineCount = lineNo
			idx.ScannedLineCount = lineNo
			trimmed := strings.TrimRight(line, "\r\n")
			if q.LineEnd > 0 && lineNo > q.LineEnd {
				break
			}
			if q.LineStart > 0 && lineNo < q.LineStart {
				if lineNo <= 200 {
					flavor.observeRawLine(trimmed)
				}
				goto nextLine
			}
			if q.TimeStart > 0 || q.TimeEnd > 0 {
				ts, hasTS := parseLineTimestamp(trimmed)
				if hasTS {
					if q.TimeEnd > 0 && ts > q.TimeEnd {
						break
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
			flavor.observeRawLine(trimmed)
			if !streamEventSearchRawCandidate(trimmed, lineNo, q) {
				goto nextLine
			}
			ev, ok := ParseLine(lineNo, trimmed, intern)
			if !ok {
				goto nextLine
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
			flavor.observeEvent(ev)
			if !eventInQuery(ev, q, typeSet) {
				goto nextLine
			}
			matchedTotal++
			if len(events) < limit {
				idx.Events = append(idx.Events, ev)
				events = append(events, EventView{
					Event: applyPriorityFlavor(ev, q.TraceFlavor),
					Raw:   trimmed,
				})
			}
		}
	nextLine:
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return Result{}, readErr
		}
	}
	idx.TraceFlavor, idx.FlavorConfidence, idx.FlavorSignals = flavor.result()
	flavorValue, confidence, signals, flavorCaveats := resolveTraceFlavor(idx, q)
	q.TraceFlavor = flavorValue
	for i := range events {
		events[i].Event = applyPriorityFlavor(events[i].Event, flavorValue)
	}
	frameworkSurfaces := detectFrameworkSurfaces(idx, q, TracePlatformAuto, 4)
	platform, platformCandidate, platformCandidateConfidence, platformCandidateSignals, platformCaveats := resolveTracePlatform(idx, q, flavorValue, frameworkSurfaces, signals)
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
	res := Result{
		View:                        "event_search",
		SourcePath:                  idx.Path,
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
		EventCount:                  idx.ParsedKnown,
		TimeStart:                   start,
		TimeEnd:                     end,
		Events:                      events,
		EvidencePack:                evidenceFromEvents(events),
	}
	res.Caveats = append(res.Caveats,
		fmt.Sprintf("streamed_event_search=true; scanned %d line(s) without building or caching a full trace index", idx.ScannedLineCount))
	if matchedTotal > len(events) {
		res.Caveats = append(res.Caveats,
			fmt.Sprintf("event_search_stream_compacted=true; matched %d row(s) but returned the first %d chronological match(es) only; omitted rows may contain later frame/span ids, so do not infer absence without rerunning an exact literal token", matchedTotal, len(events)))
	}
	res.Caveats = append(res.Caveats, flavorCaveats...)
	res.Caveats = append(res.Caveats, platformCaveats...)
	res.Caveats = append(res.Caveats, resultCaveats(idx, q, res)...)
	return res, nil
}

// StreamStateCluster scans scheduler state boundaries without materializing the
// full trace index. It is the dense-window escape hatch for root-cause
// investigations: preserve the parent window and surface state priorities before
// asking a model to pick narrower windows.
func StreamStateCluster(ctx context.Context, path string, q Query, max int) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return Result{}, fmt.Errorf("trace path is empty")
	}
	path = canonicalTraceIndexPath(path)
	info, err := os.Stat(path)
	if err != nil {
		return Result{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	if max <= 0 {
		max = 8
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
	intern := newStringInterner()
	flavor := newFlavorVote(path)
	reader := bufio.NewReaderSize(f, 256*1024)
	open := map[int]stateChurnOpen{}
	accs := map[string]*stateChurnAcc{}
	running := map[string]ThreadDuration{}
	runnable := map[string]ThreadDuration{}
	dstate := map[string]ThreadDuration{}
	seenTimeWindow := false
	parsedEvents := 0

	closeState := func(pid int, endTs float64, endLine int) {
		start, ok := open[pid]
		if !ok {
			return
		}
		delete(open, pid)
		if !streamStateClusterThreadAllowed(q, start.thread) {
			return
		}
		addStreamStateClusterInterval(accs, running, runnable, dstate, start, endTs, endLine, q)
	}
	openState := func(thread ThreadRef, state ThreadState, ts float64, line int) {
		if thread.PID <= 0 || state == StateUnknown {
			return
		}
		open[thread.PID] = stateChurnOpen{thread: thread, state: state, ts: ts, line: line}
	}

	for lineNo := 1; ; lineNo++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			idx.LineCount = lineNo
			idx.ScannedLineCount = lineNo
			trimmed := strings.TrimRight(line, "\r\n")
			if q.LineEnd > 0 && lineNo > q.LineEnd {
				break
			}
			if q.LineStart > 0 && lineNo < q.LineStart {
				if lineNo <= 200 {
					flavor.observeRawLine(trimmed)
				}
				goto nextLine
			}
			if q.TimeStart > 0 || q.TimeEnd > 0 {
				ts, hasTS := parseLineTimestamp(trimmed)
				if hasTS {
					if q.TimeEnd > 0 && ts > q.TimeEnd {
						break
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
			flavor.observeRawLine(trimmed)
			ev, ok := ParseLine(lineNo, trimmed, intern)
			if !ok {
				goto nextLine
			}
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
			flavor.observeEvent(ev)
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
	nextLine:
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return Result{}, readErr
		}
	}
	endTs := q.TimeEnd
	if endTs == 0 && idx.LastTs > 0 {
		endTs = idx.LastTs
	}
	for pid := range open {
		closeState(pid, endTs, 0)
	}
	idx.TraceFlavor, idx.FlavorConfidence, idx.FlavorSignals = flavor.result()
	flavorValue, confidence, signals, flavorCaveats := resolveTraceFlavor(idx, q)
	q.TraceFlavor = flavorValue
	frameworkSurfaces := detectFrameworkSurfaces(idx, q, TracePlatformAuto, 4)
	platform, platformCandidate, platformCandidateConfidence, platformCandidateSignals, platformCaveats := resolveTracePlatform(idx, q, flavorValue, frameworkSurfaces, signals)

	stats := WindowStats{
		Window:      TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
		TopRunning:  streamStateTopDurations(running, max),
		RunnableTop: streamStateTopDurations(runnable, max),
		DStateTop:   streamStateTopDurations(dstate, max),
		StateChurn:  streamStateClusterSummaries(accs, max),
		Caveats: []string{
			"stream_state_cluster=true; derived without materializing the full trace index",
			"state_cluster is parent-window coverage for prioritizing drilldown; root_cause_rank/frame_root_cause_bundle may still be needed on bounded phase windows",
		},
	}
	if q.PID > 0 || strings.TrimSpace(q.ThreadInput) != "" || strings.TrimSpace(q.Thread) != "" {
		stats.Caveats = append(stats.Caveats, "state_cluster filter="+streamStateClusterFilterLabel(q))
	}
	if parsedEvents == 0 || len(stats.StateChurn) == 0 {
		stats.Caveats = append(stats.Caveats, "state_cluster produced no scheduler state intervals in the selected scope; verify time/line/thread filters before making absence claims")
	}
	caveats := append([]string{
		"mode=stream_state_cluster",
	}, flavorCaveats...)
	caveats = append(caveats, platformCaveats...)
	caveats = append(caveats, stats.Caveats...)
	return Result{
		View:                        "window_stats",
		SourcePath:                  path,
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
		IndexWindowed:               idx.Windowed,
		IndexTimeStart:              idx.IndexTimeStart,
		IndexTimeEnd:                idx.IndexTimeEnd,
		IndexLineStart:              idx.IndexLineStart,
		IndexLineEnd:                idx.IndexLineEnd,
		EventCount:                  parsedEvents,
		TimeStart:                   q.TimeStart,
		TimeEnd:                     q.TimeEnd,
		WindowStats:                 &stats,
		Caveats:                     dedupStrings(caveats),
	}, nil
}

func addStreamStateClusterInterval(accs map[string]*stateChurnAcc, running, runnable, dstate map[string]ThreadDuration, start stateChurnOpen, endTs float64, endLine int, q Query) {
	if endTs <= start.ts {
		return
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
	if acc.lastState != "" && acc.lastState != start.state {
		acc.stateSwitches++
	}
	acc.lastState = start.state
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
	td := ThreadDuration{
		Thread:     start.thread,
		DurationMs: durationMs,
		StartTs:    clampedStart,
		EndTs:      clampedEnd,
		LineStart:  start.line,
		LineEnd:    firstPositive(endLine, start.line),
		CPU:        -1,
	}
	switch start.state {
	case StateRunning:
		acc.runningMs += durationMs
		streamStateAccumulateDuration(running, td)
	case StateRunnable:
		acc.runnableMs += durationMs
		streamStateAccumulateDuration(runnable, td)
	case StateSSleep:
		acc.sleepMs += durationMs
	case StateDSleep:
		acc.dStateMs += durationMs
		streamStateAccumulateDuration(dstate, td)
	case StateIOWait:
		acc.ioWaitMs += durationMs
		streamStateAccumulateDuration(dstate, td)
	}
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

func streamStateAccumulateDuration(dst map[string]ThreadDuration, td ThreadDuration) {
	if td.DurationMs <= 0 || td.Thread.PID <= 0 {
		return
	}
	key := fmt.Sprintf("%d/%s", td.Thread.PID, td.Thread.Comm)
	existing := dst[key]
	if existing.Thread.PID == 0 {
		dst[key] = td
		return
	}
	existing.DurationMs += td.DurationMs
	if existing.Thread.Comm == "" && td.Thread.Comm != "" {
		existing.Thread.Comm = td.Thread.Comm
	}
	if existing.StartTs == 0 || (td.StartTs > 0 && td.StartTs < existing.StartTs) {
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

func streamStateClusterThreadAllowed(q Query, thread ThreadRef) bool {
	if q.PID > 0 {
		return thread.PID == q.PID || thread.TGID == q.PID
	}
	target := strings.ToLower(strings.TrimSpace(firstNonEmpty(q.ThreadInput, q.Thread)))
	if target == "" {
		return true
	}
	label := strings.ToLower(threadLabel(thread))
	return strings.Contains(label, target) || strings.Contains(target, label)
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
	pattern := strings.ToLower(strings.TrimSpace(q.Pattern))
	if pattern == "" {
		return true
	}
	lower := strings.ToLower(line)
	if strings.Contains(lower, pattern) {
		return true
	}
	return strings.Contains(strconv.Itoa(lineNo), pattern)
}
