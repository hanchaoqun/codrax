package types

import "testing"

func TestRuntimeSourceAnswerAuthoritySnapshot_RuntimeOnlyCanCaveat(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("trace:root", "trace_query")}}
	got := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RouteHint: TurnRouteHint{
			Source: "artifact",
		},
		Ledger: ledger,
	})
	if !got.Active || got.NeedsCurrentSourceEvidence || got.CurrentSourceRequired {
		t.Fatalf("runtime-only optional source should not require current-source evidence: %+v", got)
	}
	if !got.RuntimeOnlySufficient || !got.CanUseRuntimeOnlyWithCaveat || !got.CanCompleteWithCombinedProof {
		t.Fatalf("runtime-only trace record should be sufficient with caveat: %+v", got)
	}
	if got.CurrentSourceRequirement != RuntimeSourceRequirementNone || got.CanHardBlockCompletion || got.CanDowngradeToCaveat {
		t.Fatalf("optional runtime-only answer must not carry source block flags: %+v", got)
	}
	if got.CurrentSourceRecordCount != 0 || got.ExactCurrentSourceSupportCount != 0 {
		t.Fatalf("runtime artifact must not masquerade as current-source proof: %+v", got)
	}
	if !runtimeSourceAuthorityHasReason(got, RuntimeSourceAuthorityRuntimeOnlyCaveat) ||
		!runtimeSourceAuthorityHasReason(got, RuntimeSourceAuthorityDeterministicRuntimeQuery) {
		t.Fatalf("expected runtime-only caveat/query reasons: %+v", got.ReasonCodes)
	}
}

func TestRuntimeSourceAnswerAuthoritySnapshot_SoftRequirementCanDowngradeToCaveat(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("trace:root", "trace_query")}}
	rm := &RequestModel{
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
		},
		CurrentSourceObligationSignals: []CurrentSourceObligationSignal{{
			Kind: CurrentSourceObligationSignalDroppedRequestedDimension,
			Role: RequestedAnswerDimensionFunctionOrPurpose,
		}},
	}
	got := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: rm,
		RouteHint: TurnRouteHint{
			Source:          "mixed",
			NeedsRepoAccess: true,
		},
		Ledger: ledger,
	})
	if !got.CurrentSourceRequired || !got.NeedsCurrentSourceEvidence {
		t.Fatalf("typed current-source obligation should request current-source evidence: %+v", got)
	}
	if got.CurrentSourceRequirement != RuntimeSourceRequirementSoft || got.CanHardBlockCompletion || !got.CanDowngradeToCaveat {
		t.Fatalf("dropped-dimension/route obligation should be soft and downgradable: %+v", got)
	}
	if !got.CanUseRuntimeOnlyWithCaveat || got.CanCompleteWithCombinedProof {
		t.Fatalf("soft missing source may caveat runtime-only but not claim combined proof: %+v", got)
	}
	if !runtimeSourceAuthorityHasReason(got, RuntimeSourceAuthorityCurrentSourceMissing) {
		t.Fatalf("missing current-source reason not present: %+v", got.ReasonCodes)
	}
}

func TestRuntimeSourceAnswerAuthoritySnapshot_PreciseRequirementCanHardBlock(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("trace:root", "trace_query")}}
	rm := &RequestModel{
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			SourceQuotes:                        []string{"compare with current parser"},
			Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism},
		},
	}
	got := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: rm,
		RouteHint: TurnRouteHint{
			Source:          "mixed",
			NeedsRepoAccess: true,
		},
		Ledger: ledger,
	})
	if got.CurrentSourceRequirement != RuntimeSourceRequirementPrecise ||
		!got.CurrentSourceRequired ||
		!got.NeedsCurrentSourceEvidence ||
		!got.CanHardBlockCompletion ||
		got.CanDowngradeToCaveat ||
		got.CanUseRuntimeOnlyWithCaveat {
		t.Fatalf("anchored current-source profile should remain a precise hard requirement: %+v", got)
	}
	if !runtimeSourceAuthorityHasReason(got, RuntimeSourceAuthorityCurrentSourcePrecise) {
		t.Fatalf("precise reason missing: %+v", got.ReasonCodes)
	}
}

func TestRuntimeSourceAnswerAuthoritySnapshot_CombinedProofSatisfiesCurrentSourceLane(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		runtimeSourceTraceRecord("trace:root", "trace_query:run2"),
		{
			ID:     "source:parse",
			Origin: AnswerEvidenceOriginCurrentSource,
			SourceRef: ObservationSourceRef{
				Kind: ObservationSourceCurrentSource,
				Path: "internal/tracequery/parse.go",
			},
			Span: ObservationSpan{LineStart: 42},
		},
	}}
	rm := &RequestModel{
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
		},
		CurrentSourceObligationSignals: []CurrentSourceObligationSignal{{
			Kind: CurrentSourceObligationSignalDroppedRequestedDimension,
			Role: RequestedAnswerDimensionFunctionOrPurpose,
		}},
	}
	got := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: rm,
		RouteHint: TurnRouteHint{
			Source:          "mixed",
			NeedsRepoAccess: true,
		},
		Ledger: ledger,
	})
	if !got.CurrentSourceRequired || !got.CurrentSourceSatisfied || got.NeedsCurrentSourceEvidence {
		t.Fatalf("current-source record should satisfy the required lane: %+v", got)
	}
	if !got.CanCompleteWithCombinedProof || got.CanUseRuntimeOnlyWithCaveat {
		t.Fatalf("mixed proof should complete as combined proof, not runtime-only caveat: %+v", got)
	}
	if got.ExactCurrentSourceSupportCount != 1 {
		t.Fatalf("exact current-source support count = %d", got.ExactCurrentSourceSupportCount)
	}
	if !runtimeSourceAuthorityHasReason(got, RuntimeSourceAuthorityCombinedProofReady) ||
		!runtimeSourceAuthorityHasReason(got, RuntimeSourceAuthorityCurrentSourceSatisfied) {
		t.Fatalf("combined/satisfied reasons missing: %+v", got.ReasonCodes)
	}
}

func runtimeSourceTraceRecord(id, producer string) ObservationRecord {
	return ObservationRecord{
		ID:       id,
		Origin:   AnswerEvidenceOriginRuntimeArtifact,
		Producer: producer,
		Summary:  "trace_query observed a runtime span",
		SourceRef: ObservationSourceRef{
			Kind:         ObservationSourceRuntimeArtifact,
			ArtifactID:   "attached_trace",
			ArtifactKind: "trace",
			PayloadRef:   "blob://trace",
		},
		Span: ObservationSpan{StartTsMs: 62380029.1},
	}
}

func runtimeSourceAuthorityHasReason(snapshot RuntimeSourceAnswerAuthoritySnapshot, want RuntimeSourceAuthorityReasonCode) bool {
	for _, got := range snapshot.ReasonCodes {
		if got == want {
			return true
		}
	}
	return false
}
