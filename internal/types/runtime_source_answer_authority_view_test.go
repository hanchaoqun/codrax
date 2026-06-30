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

func TestRuntimeSourceAnswerAuthoritySnapshot_SoftCurrentSourceProfileCanDowngradeToCaveat(t *testing.T) {
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
	if got.CurrentSourceRequirement != RuntimeSourceRequirementSoft ||
		!got.CurrentSourceRequired ||
		!got.NeedsCurrentSourceEvidence ||
		got.CanHardBlockCompletion ||
		!got.CanDowngradeToCaveat ||
		!got.CanUseRuntimeOnlyWithCaveat {
		t.Fatalf("soft current-source profile should caveat instead of hard-blocking: %+v", got)
	}
	if runtimeSourceAuthorityHasReason(got, RuntimeSourceAuthorityCurrentSourcePrecise) {
		t.Fatalf("soft profile must not publish precise reason: %+v", got.ReasonCodes)
	}
}

func TestRuntimeSourceAnswerAuthoritySnapshot_SoftExternalObservationCanDowngradeToCaveat(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		MCPResponses: []MCPResponse{{
			ServerName:  "fixture",
			Method:      "tools/call:lookup_trace_fact",
			Success:     true,
			ResourceURI: "mcp://fixture/trace/sleep-wakeup",
			MIMEType:    "application/vnd.codrax.observation+json",
			Observations: []MCPTypedObservation{{
				Summary:     "helper wakes target",
				ResourceURI: "mcp://fixture/trace/sleep-wakeup",
				LineStart:   12,
				LineEnd:     12,
				Selector:    "waker=helper",
			}},
		}},
	})
	rm := &RequestModel{
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
		},
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			SourceQuotes:                        []string{"current implementation"},
			Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism},
		},
	}

	got := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: rm,
		RouteHint: TurnRouteHint{
			Route:  "repo",
			Source: "external_tool",
		},
		Ledger: ledger,
	})
	if got.ExternalObservationSufficiency != ExternalObservationSufficiencySufficientForAnswer ||
		!got.RuntimeOnlySufficient ||
		got.RuntimeObservationCount != 0 {
		t.Fatalf("typed MCP rows should be sufficient external observations without becoming runtime rows: %+v", got)
	}
	if got.CurrentSourceRequirement != RuntimeSourceRequirementSoft ||
		!got.CurrentSourceRequired ||
		got.CanHardBlockCompletion ||
		!got.CanDowngradeToCaveat ||
		!got.CanUseRuntimeOnlyWithCaveat {
		t.Fatalf("soft current-source profile should caveat over sufficient external observations: %+v", got)
	}
}

func TestRuntimeSourceAnswerAuthoritySnapshot_PathAnchoredCurrentSourceProfileCanHardBlock(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("trace:root", "trace_query")}}
	rm := &RequestModel{
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			SourceQuotes:                        []string{"internal/tracequery/parse.go"},
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

func TestRuntimeSourceAnswerAuthoritySnapshot_UnanchoredCurrentKeyCodeCanDowngradeToCaveat(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("trace:root", "trace_query")}}
	rm := &RequestModel{
		PerfTrace: &PerfBundle{Observations: []PerfObservation{{
			Kind:       "trace_mark",
			Subject:    "H:RenderService:DoFrame",
			DurationMs: 86.111,
		}}},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    ExternalObservationCurrentSourceDefault,
		},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []RequestedAnswerDimension{{
				Label:       "current key code",
				Role:        RequestedAnswerDimensionCurrentKeyCode,
				SourceQuote: "current key code",
				Required:    true,
				Index:       1,
			}},
			Confidence: 0.9,
		},
	}

	got := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: rm,
		RouteHint:    TurnRouteHint{Source: "mixed", NeedsRepoAccess: true},
		Ledger:       ledger,
	})
	if got.CurrentSourceRequirement != RuntimeSourceRequirementSoft ||
		!got.CurrentSourceRequired ||
		!got.NeedsCurrentSourceEvidence ||
		got.CanHardBlockCompletion ||
		!got.CanDowngradeToCaveat ||
		!got.CanUseRuntimeOnlyWithCaveat {
		t.Fatalf("unanchored current-key-code obligation should stay soft and caveatable: %+v", got)
	}
	if runtimeSourceAuthorityHasReason(got, RuntimeSourceAuthorityCurrentSourcePrecise) {
		t.Fatalf("unanchored current-key-code obligation must not publish precise reason: %+v", got.ReasonCodes)
	}
}

func TestRuntimeSourceAnswerAuthoritySnapshot_PathAnchoredCurrentKeyCodeCanHardBlock(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("trace:root", "trace_query")}}
	rm := &RequestModel{
		PerfTrace: &PerfBundle{Observations: []PerfObservation{{
			Kind:       "trace_mark",
			Subject:    "H:RenderService:DoFrame",
			DurationMs: 86.111,
		}}},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    ExternalObservationCurrentSourceDefault,
		},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []RequestedAnswerDimension{{
				Label:       "current parser implementation",
				Role:        RequestedAnswerDimensionCurrentKeyCode,
				SourceQuote: "internal/tracequery/parse.go:42",
				Required:    true,
				Index:       1,
			}},
			Confidence: 0.9,
		},
	}

	got := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: rm,
		RouteHint:    TurnRouteHint{Source: "mixed", NeedsRepoAccess: true},
		Ledger:       ledger,
	})
	if got.CurrentSourceRequirement != RuntimeSourceRequirementPrecise ||
		!got.CurrentSourceRequired ||
		!got.NeedsCurrentSourceEvidence ||
		!got.CanHardBlockCompletion ||
		got.CanDowngradeToCaveat ||
		got.CanUseRuntimeOnlyWithCaveat {
		t.Fatalf("path-anchored current-key-code obligation should remain precise: %+v", got)
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

func TestRuntimeSourceAnswerAuthoritySnapshot_ContextBuildersUseSharedAuthority(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("trace:root", "trace_query")}}
	ir := &AnalysisIR{RequestModel: RequestModel{
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
		},
		CurrentSourceObligationSignals: []CurrentSourceObligationSignal{{
			Kind: CurrentSourceObligationSignalDroppedRequestedDimension,
			Role: RequestedAnswerDimensionFunctionOrPurpose,
		}},
	}}
	hint := TurnRouteHint{Source: "mixed", NeedsRepoAccess: true}

	agentGot := BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(&AgentContext{
		AnalysisIR:    ir,
		TurnRouteHint: hint,
	}, ledger)
	busGot := BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(&BusContext{
		AnalysisIR:    ir,
		TurnRouteHint: hint,
	}, ledger)

	for name, got := range map[string]RuntimeSourceAnswerAuthoritySnapshot{
		"agent": agentGot,
		"bus":   busGot,
	} {
		if got.CurrentSourceRequirement != RuntimeSourceRequirementSoft ||
			!got.CanDowngradeToCaveat ||
			got.CanHardBlockCompletion ||
			got.RuntimeObservationCount != 1 {
			t.Fatalf("%s shared authority mismatch: %+v", name, got)
		}
	}
}

func TestRuntimeSourceAnswerAuthoritySnapshot_CarrierHelpers(t *testing.T) {
	runtimeOnly := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RouteHint: TurnRouteHint{Source: "artifact"},
		Ledger:    ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("trace:root", "trace_query")}},
	})
	if !runtimeOnly.HasRuntimeCarrier() || runtimeOnly.HasCurrentSourceCarrier() || runtimeOnly.HasMixedRuntimeCurrentSourceCarrier() {
		t.Fatalf("runtime-only carrier mismatch: %+v", runtimeOnly)
	}

	softMixed := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: &RequestModel{
			CurrentSourceObligationSignals: []CurrentSourceObligationSignal{{
				Kind: CurrentSourceObligationSignalDroppedRequestedDimension,
				Role: RequestedAnswerDimensionFunctionOrPurpose,
			}},
		},
		RouteHint: TurnRouteHint{Source: "mixed", NeedsRepoAccess: true},
		Ledger:    ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("trace:root", "trace_query")}},
	})
	if !softMixed.HasRuntimeCarrier() || !softMixed.HasCurrentSourceCarrier() || !softMixed.HasMixedRuntimeCurrentSourceCarrier() {
		t.Fatalf("soft mixed runtime/source carrier mismatch: %+v", softMixed)
	}
	if softMixed.CanHardBlockCompletion {
		t.Fatalf("soft mixed carrier helper must not imply a hard gate: %+v", softMixed)
	}

	sourceOnly := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: &RequestModel{
			CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				SourceQuotes:                        []string{"internal/tracequery/query.go"},
				Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism},
			},
		},
		RouteHint: TurnRouteHint{Source: "repo", NeedsRepoAccess: true},
	})
	if sourceOnly.HasRuntimeCarrier() || !sourceOnly.HasCurrentSourceCarrier() || sourceOnly.HasMixedRuntimeCurrentSourceCarrier() {
		t.Fatalf("source-only carrier mismatch: %+v", sourceOnly)
	}
	if !sourceOnly.CanHardBlockCompletion {
		t.Fatalf("precise source-only obligation should remain a hard gate candidate: %+v", sourceOnly)
	}
}

func TestRuntimeSourceAuthorityRequestCarrierActiveUsesSharedAuthority(t *testing.T) {
	runtimeLedger := ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("trace:root", "trace_query")}}
	runtimeOnly := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		Ledger: runtimeLedger,
	})
	if RuntimeSourceAuthorityRequestCarrierActive(TurnRouteHint{}, &RequestModel{}, runtimeOnly) {
		t.Fatalf("runtime-only ledger without a request carrier must not activate mixed prompt budget: %+v", runtimeOnly)
	}
	if !RuntimeSourceAuthorityRequestCarrierActive(TurnRouteHint{Source: "mixed", NeedsRepoAccess: true}, &RequestModel{}, runtimeOnly) {
		t.Fatalf("typed mixed route should activate runtime/source prompt budget when runtime evidence exists: %+v", runtimeOnly)
	}

	softRM := &RequestModel{
		CurrentSourceObligationSignals: []CurrentSourceObligationSignal{{
			Kind: CurrentSourceObligationSignalDroppedRequestedDimension,
			Role: RequestedAnswerDimensionFunctionOrPurpose,
		}},
	}
	softMixed := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: softRM,
		RouteHint:    TurnRouteHint{Source: "mixed", NeedsRepoAccess: true},
		Ledger:       runtimeLedger,
	})
	if !RuntimeSourceAuthorityRequestCarrierActive(TurnRouteHint{}, softRM, softMixed) {
		t.Fatalf("soft source obligation should activate shared prompt budget without becoming a hard gate: %+v", softMixed)
	}
	if softMixed.CanHardBlockCompletion {
		t.Fatalf("shared prompt budget activation must not imply hard completion blocking: %+v", softMixed)
	}

	preciseRM := &RequestModel{
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			SourceQuotes:                        []string{"internal/tracequery/parse.go"},
			Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism},
		},
	}
	preciseMixed := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: preciseRM,
		RouteHint:    TurnRouteHint{Source: "mixed", NeedsRepoAccess: true},
		Ledger:       runtimeLedger,
	})
	if !RuntimeSourceAuthorityRequestCarrierActive(TurnRouteHint{}, preciseRM, preciseMixed) {
		t.Fatalf("precise source obligation should share the same request carrier predicate: %+v", preciseMixed)
	}
	if !preciseMixed.CanHardBlockCompletion {
		t.Fatalf("precise source obligation should remain the only hard-gate candidate: %+v", preciseMixed)
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
