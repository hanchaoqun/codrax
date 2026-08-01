package types

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// TraceBlockingWallClockOccurrence is one directly measured blocking interval
// on the typed analysis target. Transaction/reply phases that do not own a
// blocking interval never enter this carrier.
type TraceBlockingWallClockOccurrence struct {
	StartTs    float64
	EndTs      float64
	DurationMS float64
	Peer       string
	Flags      string
	RecordIDs  []string
}

// TraceBlockingWallClockAuthority publishes the union of directly measured
// target blocking intervals for one type and selected query window. It is an
// answer-writing authority only and changes no trace query, causal projection,
// rank, wakeup chain, supplementation, or measured value.
type TraceBlockingWallClockAuthority struct {
	ArtifactLabel  string
	SelectedWindow string
	Subject        string
	Type           string
	ObservedMS     float64
	CoverageStatus string
	Occurrences    []TraceBlockingWallClockOccurrence
}

type traceBlockingWallClockCandidate struct {
	artifact       string
	selectedWindow string
	subject        string
	typ            string
	startTs        float64
	endTs          float64
	peer           string
	flags          string
	recordID       string
	truncated      bool
}

// BuildTraceBlockingWallClockAuthorities admits only deterministic,
// hard-grounded critical-blocking observations or target-self-state rank rows
// whose own interval width agrees with their published millisecond value. The
// latter is the rank engine's exact no-seat symptom lane and is useful when a
// critical-blocking drill span includes a neighboring transaction phase. This
// keeps IPC transport latency, transaction send/reply phases, aggregate
// envelopes, and model-authored totals out of the target's measured
// blocking-wall-clock account.
//
// Exact duplicate publications are folded by interval identity. Overlapping
// intervals are unioned, so the authority never double-counts the same target
// wall clock. A capacity-truncated source publishes a lower bound rather than
// an exhaustive total.
func BuildTraceBlockingWallClockAuthorities(ledger ObservationLedger, rm *RequestModel) []TraceBlockingWallClockAuthority {
	if rm == nil || len(rm.RuntimeTargets) == 0 {
		return nil
	}
	groups := map[string][]traceBlockingWallClockCandidate{}
	for _, record := range ledger.Records {
		candidate, ok := traceBlockingWallClockCandidateFromRecord(record, rm)
		if !ok {
			continue
		}
		key := strings.Join([]string{
			strings.ToLower(candidate.artifact),
			candidate.selectedWindow,
			strings.ToLower(candidate.subject),
			strings.ToLower(candidate.typ),
		}, "\x00")
		groups[key] = append(groups[key], candidate)
	}
	if len(groups) == 0 {
		return nil
	}

	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]TraceBlockingWallClockAuthority, 0, len(keys))
	for _, key := range keys {
		candidates := groups[key]
		byInterval := map[string]traceBlockingWallClockCandidate{}
		idsByInterval := map[string]map[string]bool{}
		truncated := false
		for _, candidate := range candidates {
			intervalKey := fmt.Sprintf("%.9f\x00%.9f", candidate.startTs, candidate.endTs)
			existing, exists := byInterval[intervalKey]
			if !exists || traceBlockingWallClockCandidateRichness(candidate) > traceBlockingWallClockCandidateRichness(existing) {
				byInterval[intervalKey] = candidate
			}
			if idsByInterval[intervalKey] == nil {
				idsByInterval[intervalKey] = map[string]bool{}
			}
			if candidate.recordID != "" {
				idsByInterval[intervalKey][candidate.recordID] = true
			}
			truncated = truncated || candidate.truncated
		}

		occurrences := make([]TraceBlockingWallClockOccurrence, 0, len(byInterval))
		for intervalKey, candidate := range byInterval {
			occurrence := TraceBlockingWallClockOccurrence{
				StartTs:    candidate.startTs,
				EndTs:      candidate.endTs,
				DurationMS: (candidate.endTs - candidate.startTs) * 1000,
				Peer:       candidate.peer,
				Flags:      candidate.flags,
			}
			for id := range idsByInterval[intervalKey] {
				occurrence.RecordIDs = append(occurrence.RecordIDs, id)
			}
			sort.Strings(occurrence.RecordIDs)
			occurrences = append(occurrences, occurrence)
		}
		sort.SliceStable(occurrences, func(i, j int) bool {
			if occurrences[i].StartTs != occurrences[j].StartTs {
				return occurrences[i].StartTs < occurrences[j].StartTs
			}
			return occurrences[i].EndTs < occurrences[j].EndTs
		})

		base := candidates[0]
		authority := TraceBlockingWallClockAuthority{
			ArtifactLabel:  base.artifact,
			SelectedWindow: base.selectedWindow,
			Subject:        base.subject,
			Type:           base.typ,
			CoverageStatus: "complete",
			Occurrences:    occurrences,
		}
		if truncated {
			authority.CoverageStatus = "lower_bound_capacity_truncated"
		}
		authority.ObservedMS = traceBlockingWallClockIntervalUnionMS(occurrences)
		out = append(out, authority)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ArtifactLabel != out[j].ArtifactLabel {
			return out[i].ArtifactLabel < out[j].ArtifactLabel
		}
		if out[i].SelectedWindow != out[j].SelectedWindow {
			return out[i].SelectedWindow < out[j].SelectedWindow
		}
		if out[i].Subject != out[j].Subject {
			return out[i].Subject < out[j].Subject
		}
		return out[i].Type < out[j].Type
	})
	return out
}

func traceBlockingWallClockCandidateFromRecord(record ObservationRecord, rm *RequestModel) (traceBlockingWallClockCandidate, bool) {
	if record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
		!RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
		record.GroundingPolicy != ClaimGroundingHard ||
		!ObservationRecordMatchesUserRuntimeTarget(record, rm) ||
		TraceObservationIsEvidenceBoundary(record) ||
		!strings.EqualFold(strings.TrimSpace(record.Unit), "ms") {
		return traceBlockingWallClockCandidate{}, false
	}
	dimension := traceObservationDimension(record)
	criticalBlocking := dimension == TraceObservationDimensionCriticalBlocking &&
		strings.EqualFold(strings.TrimSpace(record.Predicate), "critical_blocking")
	targetSelfState := dimension == TraceObservationDimensionRootCauseRank &&
		strings.EqualFold(strings.TrimSpace(record.Predicate), "root_cause_target_self_state") &&
		strings.EqualFold(strings.TrimSpace(traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyTier)), TraceCausalTierTargetSelfState)
	if !criticalBlocking && !targetSelfState {
		return traceBlockingWallClockCandidate{}, false
	}
	valueMS, err := strconv.ParseFloat(strings.TrimSpace(record.Value), 64)
	if err != nil || valueMS <= 0 || math.IsNaN(valueMS) || math.IsInf(valueMS, 0) {
		return traceBlockingWallClockCandidate{}, false
	}
	startTs, endTs := record.Span.StartTs, record.Span.EndTs
	if endTs <= startTs || math.IsNaN(startTs) || math.IsNaN(endTs) ||
		math.IsInf(startTs, 0) || math.IsInf(endTs, 0) {
		return traceBlockingWallClockCandidate{}, false
	}
	durationMS := (endTs - startTs) * 1000
	toleranceMS := math.Max(0.002, valueMS*0.0005)
	if math.Abs(durationMS-valueMS) > toleranceMS {
		return traceBlockingWallClockCandidate{}, false
	}
	artifact := strings.TrimSpace(record.SourceRef.ArtifactID)
	if artifact == "" {
		artifact = RuntimeArtifactCaptureIdentityPath(record.SourceRef)
	}
	selectedWindow := strings.TrimSpace(traceObservationRichNoteValue(record.RichNotes, TraceNoteKeySelectedWindow))
	subject := strings.TrimSpace(record.Subject)
	typ := strings.TrimSpace(traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyType))
	if typ == "" {
		typ = traceValueOccurrenceType(record)
	}
	if artifact == "" || selectedWindow == "" || subject == "" || typ == "" {
		return traceBlockingWallClockCandidate{}, false
	}
	return traceBlockingWallClockCandidate{
		artifact:       artifact,
		selectedWindow: selectedWindow,
		subject:        subject,
		typ:            typ,
		startTs:        startTs,
		endTs:          endTs,
		peer:           strings.TrimSpace(traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyPeer)),
		flags:          strings.TrimSpace(traceObservationRichNoteValue(record.RichNotes, "flags")),
		recordID:       strings.TrimSpace(record.ID),
		truncated:      traceObservationRichNoteBool(record.RichNotes, TraceNoteKeyCapacityTruncated),
	}, true
}

func traceBlockingWallClockCandidateRichness(candidate traceBlockingWallClockCandidate) int {
	score := 0
	if candidate.peer != "" {
		score++
	}
	if candidate.flags != "" {
		score++
	}
	return score
}

func traceBlockingWallClockIntervalUnionMS(occurrences []TraceBlockingWallClockOccurrence) float64 {
	if len(occurrences) == 0 {
		return 0
	}
	start, end := occurrences[0].StartTs, occurrences[0].EndTs
	totalSeconds := 0.0
	for _, occurrence := range occurrences[1:] {
		if occurrence.StartTs <= end {
			if occurrence.EndTs > end {
				end = occurrence.EndTs
			}
			continue
		}
		totalSeconds += end - start
		start, end = occurrence.StartTs, occurrence.EndTs
	}
	totalSeconds += end - start
	return totalSeconds * 1000
}
