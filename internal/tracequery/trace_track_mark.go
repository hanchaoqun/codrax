package tracequery

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// traceTrackWireKey is the exact AOSP ASYNC_FOR_TRACK logical identity. The
// begin's display name is intentionally absent because H does not carry it.
func traceTrackWireKey(ev Event) string {
	track := traceTrackNameFromEvent(ev)
	if ev.SpanPID <= 0 || track == "" || strings.TrimSpace(ev.SpanValue) == "" {
		return ""
	}
	return strings.Join([]string{strconv.Itoa(ev.SpanPID), track, ev.SpanValue}, "\x00")
}

// traceTrackNameFromEvent recovers the already-validated G/H/N track without
// adding another string to the hot Event core (every sched_switch pays for
// that struct). SpanAction was minted from the complete raw payload; this
// helper only reads the bounded inventory copy and requires the track's right
// delimiter to remain present. A pathologically long/truncated track therefore
// fails closed instead of pairing on a partial identity.
func traceTrackNameFromEvent(ev Event) string {
	if ev.Type != EventTraceMark || (ev.SpanAction != "G" && ev.SpanAction != "H" && ev.SpanAction != "N") {
		return ""
	}
	if plugin := ev.PluginFields; plugin != nil && strings.TrimSpace(plugin.SpanTrack) != "" {
		return strings.TrimSpace(plugin.SpanTrack)
	}
	// Compatibility fallback for hand-built Events and older cached fixtures.
	// Production ParserVersion v22 rows take the side-table branch above.
	parts := strings.Split(normalizeTraceMarkPayload(ev.FieldText), "|")
	if len(parts) < 4 || strings.TrimSpace(parts[0]) != ev.SpanAction {
		return ""
	}
	pid, ok := parseUnsignedTraceInt(parts[1])
	if !ok || pid != ev.SpanPID {
		return ""
	}
	return strings.TrimSpace(parts[2])
}

func traceTrackPairingKey(source string, ev Event, generation string) (string, bool) {
	wire := traceTrackWireKey(ev)
	if wire == "" {
		return "", false
	}
	if generation == "" {
		generation = "initial"
	}
	return strings.Join([]string{source, generation, wire}, "\x00"), true
}

type traceTrackOpenLane struct {
	first     Event
	depth     int
	ambiguous bool
}

func traceTrackIntervalMayReachQuery(start, end Event, q Query) bool {
	if q.LineEnd > 0 && start.Line > q.LineEnd {
		return false
	}
	if end.Line > 0 && q.LineStart > 0 && end.Line < q.LineStart {
		return false
	}
	if q.LineStart == 0 && q.LineEnd == 0 {
		if q.TimeEnd > 0 && start.Ts > q.TimeEnd {
			return false
		}
		if end.Ts > 0 && q.TimeStart > 0 && end.Ts < q.TimeStart {
			return false
		}
	}
	return true
}

func traceTrackStartMayCarryIntoQuery(start Event, q Query) bool {
	return traceTrackIntervalMayReachQuery(start, Event{}, q)
}

func traceTrackSpanFromEvents(start, end Event, source string) TraceTrackSpanSummary {
	return TraceTrackSpanSummary{
		SourcePath:   source,
		OwnerPID:     start.SpanPID,
		TrackName:    traceTrackNameFromEvent(start),
		Name:         start.SpanName,
		Cookie:       start.SpanValue,
		BeginEmitter: threadRefFromEvent(start),
		EndEmitter:   threadRefFromEvent(end),
		BeginPayload: start.FieldText,
		EndPayload:   end.FieldText,
		StartTs:      start.Ts,
		EndTs:        end.Ts,
		DurationMs:   (end.Ts - start.Ts) * 1000,
		StartLine:    start.Line,
		EndLine:      end.Line,
	}
}

func clipTraceTrackSpanToQueryWindow(span TraceTrackSpanSummary, q Query) (TraceTrackSpanSummary, bool) {
	if q.LineStart > 0 && span.EndLine > 0 && span.EndLine < q.LineStart {
		return span, false
	}
	if q.LineEnd > 0 && span.StartLine > 0 && span.StartLine > q.LineEnd {
		return span, false
	}
	start, end := span.StartTs, span.EndTs
	if end <= start {
		return span, end == start && timeInWindow(start, q)
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
		span.ActualStartTs = start
		span.ActualEndTs = end
		span.ActualDurationMs = span.DurationMs
		span.StartTs = clipStart
		span.EndTs = clipEnd
		span.DurationMs = (clipEnd - clipStart) * 1000
	}
	return span, true
}

func traceInstantFromEvent(ev Event, source string) TraceInstantSummary {
	return TraceInstantSummary{
		SourcePath: source,
		Action:     ev.SpanAction,
		OwnerPID:   ev.SpanPID,
		TrackName:  traceTrackNameFromEvent(ev),
		Name:       ev.SpanName,
		Emitter:    threadRefFromEvent(ev),
		Payload:    ev.FieldText,
		Ts:         ev.Ts,
		Line:       ev.Line,
	}
}

// computeTraceTrackMarks publishes G/H and N/I on an isolated inventory face.
// In particular, it never calls traceSpanFromEvents, never assigns a Thread
// owner, and never feeds semantic/root-cause ranking.
func computeTraceTrackMarks(idx *Index, q Query, max int) ([]TraceTrackSpanSummary, []TraceInstantSummary, []string) {
	if idx == nil {
		return nil, nil, nil
	}
	if max <= 0 {
		max = 8
	}
	lanes := map[string]*traceTrackOpenLane{}
	var spans []TraceTrackSpanSummary
	var instants []TraceInstantSummary
	duplicateCohorts, incompleteStarts, orphanEnds := 0, 0, 0
	unresolvedTrackEndpoints, unresolvedInstants := 0, 0
	unresolvedTrackIdentity, unresolvedInstantIdentity := 0, 0
	pairingUnsafe := durationOrderFailureForFamily(idx, q, durationOrderTraceTrack)
	trackIntegrityFailure := traceTrackIntegrityFailureForQuery(idx, q)
	trackOwnerPIDs := map[int]bool{}
	for _, ev := range idx.Events {
		if ev.Type == EventTraceMark && (ev.SpanAction == "G" || ev.SpanAction == "H") && ev.SpanPID > 0 {
			trackOwnerPIDs[ev.SpanPID] = true
		}
	}
	// Exact scheduler lifecycle boundaries partition the payload owner's
	// numeric pid inside each physical source. The marker payload may live in a
	// container namespace, so these boundaries are used only as conservative
	// non-crossing cuts; they never assert host-thread ownership.
	generations := map[string]string{}

	for _, ev := range idx.Events {
		if resetPID, reset := schedulerLifecycleResetPID(ev); reset && resetPID > 0 {
			if source, ok := tracePairingSourceIdentity(idx, ev); ok {
				generations[source+"\x00"+strconv.Itoa(resetPID)] = fmt.Sprintf("line=%d,ts=%.9f", ev.Line, ev.Ts)
			} else if trackOwnerPIDs[resetPID] {
				unresolvedTrackEndpoints++
			}
		}
		if ev.Type != EventTraceMark {
			continue
		}
		switch ev.SpanAction {
		case "I", "N":
			if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
				continue
			}
			source, ok := tracePairingSourceIdentity(idx, ev)
			if !ok {
				unresolvedInstants++
				continue
			}
			if ev.SpanAction == "N" && traceTrackNameFromEvent(ev) == "" {
				unresolvedInstantIdentity++
				continue
			}
			instants = append(instants, traceInstantFromEvent(ev, source))
		case "G", "H":
			source, sourceOK := tracePairingSourceIdentity(idx, ev)
			if !sourceOK {
				unresolvedTrackEndpoints++
				continue
			}
			generation := generations[source+"\x00"+strconv.Itoa(ev.SpanPID)]
			key, ok := traceTrackPairingKey(source, ev, generation)
			if !ok {
				unresolvedTrackIdentity++
				continue
			}
			lane := lanes[key]
			if ev.SpanAction == "G" {
				if lane == nil {
					lanes[key] = &traceTrackOpenLane{first: ev, depth: 1}
					continue
				}
				// The same pid+track+cookie cannot be paired uniquely while an
				// earlier begin is still open. Suppress the entire overlapping
				// cohort instead of LIFO-guessing.
				lane.depth++
				lane.ambiguous = true
				continue
			}
			if lane == nil || lane.depth == 0 {
				if eventLineInWindow(ev, q) && timeInWindow(ev.Ts, q) {
					orphanEnds++
				}
				continue
			}
			if lane.ambiguous {
				lane.depth--
				if lane.depth == 0 {
					if traceTrackIntervalMayReachQuery(lane.first, ev, q) {
						duplicateCohorts++
					}
					delete(lanes, key)
				}
				continue
			}
			delete(lanes, key)
			if ev.Ts < lane.first.Ts || pairingUnsafe != nil || trackIntegrityFailure != nil {
				continue
			}
			if span, admitted := clipTraceTrackSpanToQueryWindow(traceTrackSpanFromEvents(lane.first, ev, source), q); admitted {
				spans = append(spans, span)
			}
		}
	}

	for _, lane := range lanes {
		if !traceTrackStartMayCarryIntoQuery(lane.first, q) {
			continue
		}
		if lane.ambiguous {
			duplicateCohorts++
		} else {
			incompleteStarts++
		}
	}

	var caveats []string
	if pairingUnsafe != nil {
		spans = nil
		caveats = append(caveats, durationOrderFailClosedCaveat(pairingUnsafe, "trace_track_spans"))
	}
	if trackIntegrityFailure != nil {
		spans = nil
		caveats = append(caveats, "trace_track_pairing_fail_closed=true; "+trackIntegrityFailure.reason()+"; trace_track_spans are omitted because a malformed G/H endpoint cannot be assigned to one exact payload-owner track lane")
	}
	if unresolvedTrackEndpoints > 0 {
		spans = nil
		caveats = append(caveats, fmt.Sprintf("trace_track_provenance_unresolved=true; rows=%d; trace_track_spans are omitted because a G/H endpoint or relevant lifecycle boundary could not be mapped to exactly one physical source artifact", unresolvedTrackEndpoints))
	}
	if unresolvedTrackIdentity > 0 {
		spans = nil
		caveats = append(caveats, fmt.Sprintf("trace_track_identity_unavailable=true; rows=%d; trace_track_spans are omitted because a validated G/H track identity did not fit in the bounded Event inventory copy", unresolvedTrackIdentity))
	}
	if unresolvedInstants > 0 {
		instants = nil
		caveats = append(caveats, fmt.Sprintf("trace_instant_provenance_unresolved=true; rows=%d; trace_instants are omitted because an I/N row could not be mapped to exactly one physical source artifact", unresolvedInstants))
	}
	if unresolvedInstantIdentity > 0 {
		instants = nil
		caveats = append(caveats, fmt.Sprintf("trace_instant_identity_unavailable=true; rows=%d; trace_instants are omitted because a validated N track identity did not fit in the bounded Event inventory copy", unresolvedInstantIdentity))
	}
	if duplicateCohorts > 0 {
		caveats = append(caveats, fmt.Sprintf("trace_track_duplicate_key_fail_closed=true; ambiguous_cohorts=%d; concurrent/repeated G endpoints with the same payload_pid+track_name+cookie were withheld instead of LIFO-paired", duplicateCohorts))
	}
	if incompleteStarts > 0 || orphanEnds > 0 {
		caveats = append(caveats, fmt.Sprintf("trace_track_pairing_incomplete=true; open_begins=%d orphan_ends=%d; incomplete endpoints remain searchable trace_mark inventory and mint no duration", incompleteStarts, orphanEnds))
	}

	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].DurationMs != spans[j].DurationMs {
			return spans[i].DurationMs > spans[j].DurationMs
		}
		return spans[i].StartLine < spans[j].StartLine
	})
	sort.SliceStable(instants, func(i, j int) bool {
		if instants[i].Ts != instants[j].Ts {
			return instants[i].Ts < instants[j].Ts
		}
		return instants[i].Line < instants[j].Line
	})
	if len(spans) > max {
		total := len(spans)
		spans = spans[:max]
		caveats = append(caveats, fmt.Sprintf("trace_track_spans compacted from %d to %d row(s)", total, max))
	}
	if len(instants) > max {
		total := len(instants)
		instants = instants[:max]
		caveats = append(caveats, fmt.Sprintf("trace_instants compacted from %d to %d row(s)", total, max))
	}
	return spans, instants, caveats
}
