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

// externalDocRecord builds an addressable external-document observation
// (the shape the authority lane's sufficiency arm counts).
func externalDocRecord() ObservationRecord {
	return ObservationRecord{
		Origin:      AnswerEvidenceOriginExternalDocument,
		Summary:     "spec section 3 defines the retry budget",
		SupportRefs: []string{"doc:spec.pdf#p3"},
		SourceRef: ObservationSourceRef{
			Kind: ObservationSourceExternalDocument,
		},
	}
}

// TestRuntimeWitnessSplit pins the round-4 split: external observations
// clear the WIDE flag-flip floor but never the NARROW runtime predicate
// — so a runtime-grounding claim can never ride an external-document
// witness, and a model-authored SupportRef cannot silence the tripwire.
func TestRuntimeWitnessSplit(t *testing.T) {
	ext := ObservationLedger{Records: []ObservationRecord{externalDocRecord()}}
	if !RuntimeSourceOptionalWaiverWitnessFloor(false, ext) {
		t.Fatal("external-document candidates must clear the wide flag-flip floor (§10 R8 ruling)")
	}
	if RuntimeWitnessPresent(false, ext) {
		t.Fatal("an external document is NOT a runtime witness")
	}

	poisoned := &AnswerSurfacePlan{RuntimeGroundingDisposition: &RuntimeGroundingDisposition{
		Source:         RuntimeGroundingSystemDetected,
		Reason:         EvidenceFloorWaiverExternalTrace,
		Rationale:      "runtime trace observations were available without a required current-source lane",
		CitationPolicy: RuntimeGroundingCitationRuntimeObservation,
	}}
	if !RuntimeGroundingClaimsWitnessless(poisoned, false, ext) {
		t.Fatal("round-4: the tripwire must fire on a runtime-grounding claim backed only by external documents")
	}
}

// TestSourceOptionalMint_ExternalOnlyFlipsFlagsWithoutRuntimeClaim pins
// the round-4 mint split at the choke point: an external-observation
// waiver flips the source-optional flags but must NOT mint the
// runtime-grounding disposition whose rationale would falsely claim
// "runtime … observations were available".
func TestSourceOptionalMint_ExternalOnlyFlipsFlagsWithoutRuntimeClaim(t *testing.T) {
	ir := &AnalysisIR{}
	ir.RequestModel.ExternalObservationPolicy = &ExternalObservationPolicy{
		CurrentSourceMode:    ExternalObservationCurrentSourceExclude,
		ExclusionKind:        ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:         []string{"只按我贴的文档回答"},
		ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
	}
	plan := &AnswerSurfacePlan{CurrentStatusDiagnosticRequired: true, CurrentSourceEvidenceOrigin: true}
	ledger := ObservationLedger{Records: []ObservationRecord{externalDocRecord()}}
	applyRuntimeTraceSourceOptionalSurfacePlan(plan, ir, false, ledger, TurnRouteHint{})
	if plan.CurrentStatusDiagnosticRequired || plan.CurrentSourceEvidenceOrigin {
		t.Fatal("precondition: the external-observation waiver must actually flip the flags (else this test proves nothing about the split)")
	}
	if plan.RuntimeGroundingDisposition != nil {
		t.Fatalf("external-only run must not mint a runtime-grounding disposition; got %+v", plan.RuntimeGroundingDisposition)
	}

	// With a real runtime witness the disposition still mints.
	planRT := &AnswerSurfacePlan{CurrentStatusDiagnosticRequired: true, CurrentSourceEvidenceOrigin: true}
	rtLedger := ObservationLedger{Records: []ObservationRecord{{
		Origin:   AnswerEvidenceOriginRuntimeArtifact,
		Producer: "perf_triage",
		SourceRef: ObservationSourceRef{
			Kind:         ObservationSourceRuntimeArtifact,
			ArtifactKind: "trace",
		},
	}}}
	applyRuntimeTraceSourceOptionalSurfacePlan(planRT, ir, true, rtLedger, TurnRouteHint{})
	if planRT.RuntimeGroundingDisposition == nil || !planRT.RuntimeGroundingDisposition.IsActive() {
		t.Fatal("a genuine runtime witness must still mint the disposition")
	}
}
