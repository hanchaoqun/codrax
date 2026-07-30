package types

import "testing"

// FABGATE-2 pins (runtime_witness_floor.go): the witness floor and the
// factory-floor tripwire condition. The honest pipeline can no longer
// reach the witnessless-waiver state (FABGATE-1 + both minting lanes
// imply the floor), so these pin the invariant surface directly.

func runtimeWitnessRecord(producer string, artifactKind string) ObservationRecord {
	return ObservationRecord{
		Origin:   AnswerEvidenceOriginRuntimeArtifact,
		Producer: producer,
		SourceRef: ObservationSourceRef{
			Kind:         ObservationSourceRuntimeArtifact,
			ArtifactKind: artifactKind,
		},
	}
}

func TestRuntimeSourceOptionalWaiverWitnessFloor(t *testing.T) {
	empty := ObservationLedger{}
	if RuntimeSourceOptionalWaiverWitnessFloor(false, empty) {
		t.Fatal("witnessless run must fail the floor")
	}
	if !RuntimeSourceOptionalWaiverWitnessFloor(true, empty) {
		t.Fatal("an attached runtime artifact passes the floor")
	}
	trace := ObservationLedger{Records: []ObservationRecord{runtimeWitnessRecord("perf_triage", "trace")}}
	if !RuntimeSourceOptionalWaiverWitnessFloor(false, trace) {
		t.Fatal("a runtime trace artifact record passes the floor")
	}
	logRec := ObservationLedger{Records: []ObservationRecord{runtimeWitnessRecord("log_triage", "log")}}
	if !RuntimeSourceOptionalWaiverWitnessFloor(false, logRec) {
		t.Fatal("a runtime log artifact record passes the floor")
	}
}

func TestRuntimeGroundingClaimsWitnessless(t *testing.T) {
	poisoned := &AnswerSurfacePlan{RuntimeGroundingDisposition: &RuntimeGroundingDisposition{
		Source:         RuntimeGroundingSystemDetected,
		Reason:         EvidenceFloorWaiverExternalTrace,
		Rationale:      "runtime trace observations were available without a required current-source lane",
		CitationPolicy: RuntimeGroundingCitationRuntimeObservation,
	}}
	if !RuntimeGroundingClaimsWitnessless(poisoned, false, ObservationLedger{}) {
		t.Fatal("active runtime-observation disposition over a witnessless run must trip")
	}
	// Any witness clears it.
	witnessed := ObservationLedger{Records: []ObservationRecord{runtimeWitnessRecord("perf_triage", "trace")}}
	if RuntimeGroundingClaimsWitnessless(poisoned, false, witnessed) {
		t.Fatal("a runtime witness must clear the tripwire")
	}
	if RuntimeGroundingClaimsWitnessless(poisoned, true, ObservationLedger{}) {
		t.Fatal("an attached artifact must clear the tripwire")
	}
	// Inactive or non-runtime-citation dispositions never trip.
	if RuntimeGroundingClaimsWitnessless(nil, false, ObservationLedger{}) {
		t.Fatal("nil plan never trips")
	}
	if RuntimeGroundingClaimsWitnessless(&AnswerSurfacePlan{}, false, ObservationLedger{}) {
		t.Fatal("plan without disposition never trips")
	}
}
