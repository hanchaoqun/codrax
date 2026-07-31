package types

import (
	"strconv"
	"testing"
)

func traceValueOccurrenceAuthorityRequest() RequestModel {
	return RequestModel{RuntimeTargets: []RuntimeTarget{{
		Kind:   RuntimeTargetKindThread,
		PID:    17267,
		Thread: ".ugc.aweme.lite-17267",
		Source: "user_explicit",
	}}}
}

func traceValueOccurrenceAuthorityRecord(id, typ string, value, start, end float64) ObservationRecord {
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy: ClaimGroundingHard,
		SourceRef: ObservationSourceRef{
			Kind:       ObservationSourceRuntimeArtifact,
			Path:       "/tmp/attached_trace.txt",
			ArtifactID: "attached_trace.txt",
		},
		Span:      ObservationSpan{StartTs: start, EndTs: end},
		ClaimKey:  "root_cause_target_self_state",
		Predicate: "root_cause_target_self_state",
		Subject:   ".ugc.aweme.lite-17267",
		Object:    typ,
		Value:     strconv.FormatFloat(value, 'f', 3, 64),
		Unit:      "ms",
		RichNotes: []string{"type=" + typ},
	}
}

func TestBuildTraceValueOccurrenceAuthoritiesUsesSameFactSpanAndIgnoresAggregateTimestamp(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	root := traceValueOccurrenceAuthorityRecord("root", "binder_wait", 1.409, 13762.835861, 13762.837270)
	evidence := root
	evidence.ID = "root-evidence"
	evidence.ClaimKey = "root_evidence:binder_wait"
	evidence.Predicate = "root_evidence"
	aggregate := root
	aggregate.ID = "aggregate"
	aggregate.Origin = AnswerEvidenceOriginCommandMeasurement
	aggregate.Producer = "emit_investigation_complete"
	aggregate.Span = ObservationSpan{StartTs: 13762.834345, EndTs: 13762.835754}

	got := BuildTraceValueOccurrenceAuthorities(ObservationLedger{Records: []ObservationRecord{
		aggregate, evidence, root,
	}}, &rm)
	if len(got) != 1 {
		t.Fatalf("expected one deduplicated typed occurrence authority, got %+v", got)
	}
	if got[0].Status != "exact" || got[0].OccurrenceN != 1 ||
		got[0].StartTs != 13762.835861 || got[0].EndTs != 13762.837270 ||
		len(got[0].RecordIDs) != 2 {
		t.Fatalf("same-fact occurrence identity drifted: %+v", got[0])
	}
}

func TestBuildTraceValueOccurrenceAuthoritiesFailsClosedOnMultipleOccurrences(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	first := traceValueOccurrenceAuthorityRecord("first", "binder_wait", 1.409, 10.000000, 10.001409)
	second := traceValueOccurrenceAuthorityRecord("second", "binder_wait", 1.409, 20.000000, 20.001409)

	got := BuildTraceValueOccurrenceAuthorities(ObservationLedger{Records: []ObservationRecord{first, second}}, &rm)
	if len(got) != 1 || got[0].Status != "ambiguous_multiple_occurrences" ||
		got[0].OccurrenceN != 2 || got[0].StartTs != 0 || got[0].EndTs != 0 {
		t.Fatalf("multiple occurrences must not elect one timestamp: %+v", got)
	}
}

func TestBuildTraceValueOccurrenceAuthoritiesRejectsAggregateEnvelopeAndNonTarget(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	envelope := traceValueOccurrenceAuthorityRecord("envelope", "running", 15.758, 1.000000, 1.233190)
	other := traceValueOccurrenceAuthorityRecord("other", "binder_wait", 1.409, 2.000000, 2.001409)
	other.Subject = "other-99"

	if got := BuildTraceValueOccurrenceAuthorities(ObservationLedger{Records: []ObservationRecord{envelope, other}}, &rm); len(got) != 0 {
		t.Fatalf("aggregate envelope and non-target rows must not gain value-owner time authority: %+v", got)
	}
}
