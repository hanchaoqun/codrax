package tracequery

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
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
			fmt.Sprintf("event_search stream compacted from %d to %d row(s); rerun with a narrower time/line/event filter for later matches", matchedTotal, len(events)))
	}
	res.Caveats = append(res.Caveats, flavorCaveats...)
	res.Caveats = append(res.Caveats, platformCaveats...)
	res.Caveats = append(res.Caveats, resultCaveats(idx, q, res)...)
	return res, nil
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
