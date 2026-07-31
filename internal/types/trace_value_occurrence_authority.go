package types

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// TraceValueOccurrenceAuthority binds one measured millisecond value to the
// occurrence interval owned by the same deterministic trace observation.
// It is answer-writing authority only: it does not participate in query
// selection, causal projection, ranking, supplementation, or scoring.
type TraceValueOccurrenceAuthority struct {
	ArtifactLabel string
	Subject       string
	Type          string
	ValueMS       float64
	Status        string
	StartTs       float64
	EndTs         float64
	OccurrenceN   int
	RecordIDs     []string
}

type traceValueOccurrenceCandidate struct {
	artifact string
	subject  string
	typ      string
	valueMS  float64
	startTs  float64
	endTs    float64
	recordID string
}

// BuildTraceValueOccurrenceAuthorities returns value/window identities only
// for deterministic, hard-grounded trace observations on a typed user target.
// The occurrence interval must numerically own the published millisecond value:
// its width and value must agree within trace timestamp/three-decimal display
// precision. Free-form aggregate facts and model prose never enter this lane.
//
// Multiple observations of the same artifact/subject/type/value and interval
// are deduplicated. If that identity has multiple distinct intervals, the
// result is marked ambiguous and publishes no chosen start/end.
func BuildTraceValueOccurrenceAuthorities(ledger ObservationLedger, rm *RequestModel) []TraceValueOccurrenceAuthority {
	if rm == nil || len(rm.RuntimeTargets) == 0 {
		return nil
	}
	groups := map[string][]traceValueOccurrenceCandidate{}
	for _, record := range ledger.Records {
		candidate, ok := traceValueOccurrenceCandidateFromRecord(record, rm)
		if !ok {
			continue
		}
		key := strings.Join([]string{
			strings.ToLower(candidate.artifact),
			strings.ToLower(candidate.subject),
			strings.ToLower(candidate.typ),
			fmt.Sprintf("%.6f", candidate.valueMS),
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
	out := make([]TraceValueOccurrenceAuthority, 0, len(keys))
	for _, key := range keys {
		candidates := groups[key]
		intervals := map[string]traceValueOccurrenceCandidate{}
		recordIDs := map[string]bool{}
		for _, candidate := range candidates {
			intervalKey := fmt.Sprintf("%.9f\x00%.9f", candidate.startTs, candidate.endTs)
			if _, exists := intervals[intervalKey]; !exists {
				intervals[intervalKey] = candidate
			}
			if id := strings.TrimSpace(candidate.recordID); id != "" {
				recordIDs[id] = true
			}
		}
		base := candidates[0]
		authority := TraceValueOccurrenceAuthority{
			ArtifactLabel: base.artifact,
			Subject:       base.subject,
			Type:          base.typ,
			ValueMS:       base.valueMS,
			OccurrenceN:   len(intervals),
		}
		for id := range recordIDs {
			authority.RecordIDs = append(authority.RecordIDs, id)
		}
		sort.Strings(authority.RecordIDs)
		if len(intervals) == 1 {
			authority.Status = "exact"
			for _, candidate := range intervals {
				authority.StartTs = candidate.startTs
				authority.EndTs = candidate.endTs
			}
		} else {
			authority.Status = "ambiguous_multiple_occurrences"
		}
		out = append(out, authority)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ArtifactLabel != out[j].ArtifactLabel {
			return out[i].ArtifactLabel < out[j].ArtifactLabel
		}
		if out[i].Subject != out[j].Subject {
			return out[i].Subject < out[j].Subject
		}
		if out[i].Status != out[j].Status {
			return out[i].Status == "exact"
		}
		if out[i].StartTs != out[j].StartTs {
			return out[i].StartTs < out[j].StartTs
		}
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].ValueMS > out[j].ValueMS
	})
	return out
}

func traceValueOccurrenceCandidateFromRecord(record ObservationRecord, rm *RequestModel) (traceValueOccurrenceCandidate, bool) {
	if record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
		!RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
		record.GroundingPolicy != ClaimGroundingHard ||
		!ObservationRecordMatchesUserRuntimeTarget(record, rm) {
		return traceValueOccurrenceCandidate{}, false
	}
	switch traceObservationDimension(record) {
	case TraceObservationDimensionRootCauseRank,
		TraceObservationDimensionCriticalBlocking,
		TraceObservationDimensionEvidencePack:
	default:
		return traceValueOccurrenceCandidate{}, false
	}
	if !strings.EqualFold(strings.TrimSpace(record.Unit), "ms") {
		return traceValueOccurrenceCandidate{}, false
	}
	valueMS, err := strconv.ParseFloat(strings.TrimSpace(record.Value), 64)
	if err != nil || valueMS <= 0 || math.IsNaN(valueMS) || math.IsInf(valueMS, 0) {
		return traceValueOccurrenceCandidate{}, false
	}
	startTs, endTs := record.Span.StartTs, record.Span.EndTs
	if endTs <= startTs || math.IsNaN(startTs) || math.IsNaN(endTs) ||
		math.IsInf(startTs, 0) || math.IsInf(endTs, 0) {
		return traceValueOccurrenceCandidate{}, false
	}
	durationMS := (endTs - startTs) * 1000
	toleranceMS := math.Max(0.002, valueMS*0.0005)
	if math.Abs(durationMS-valueMS) > toleranceMS {
		return traceValueOccurrenceCandidate{}, false
	}
	artifact := strings.TrimSpace(record.SourceRef.ArtifactID)
	if artifact == "" {
		artifact = strings.TrimSpace(record.SourceRef.Path)
	}
	subject := strings.TrimSpace(record.Subject)
	typ := traceValueOccurrenceType(record)
	if artifact == "" || subject == "" || typ == "" {
		return traceValueOccurrenceCandidate{}, false
	}
	return traceValueOccurrenceCandidate{
		artifact: artifact,
		subject:  subject,
		typ:      typ,
		valueMS:  valueMS,
		startTs:  startTs,
		endTs:    endTs,
		recordID: strings.TrimSpace(record.ID),
	}, true
}

func traceValueOccurrenceType(record ObservationRecord) string {
	for _, raw := range record.RichNotes {
		note := strings.TrimSpace(raw)
		if strings.HasPrefix(note, "type=") {
			if value := strings.TrimSpace(strings.TrimPrefix(note, "type=")); value != "" {
				return value
			}
		}
	}
	if value := strings.TrimSpace(record.Object); value != "" {
		return value
	}
	for _, prefix := range []string{"root_evidence:", "critical_blocking:"} {
		if claim := strings.TrimSpace(record.ClaimKey); strings.HasPrefix(claim, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(claim, prefix))
		}
	}
	return strings.TrimSpace(record.Predicate)
}
