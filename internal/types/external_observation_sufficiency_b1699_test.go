package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestB1699ExternalSufficiencyScopeSharesRequestPrecision(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configure  func(*RequestModel, *TurnRouteHint)
		precision  RuntimeSourceRequirementPrecision
		scope      ExternalObservationSufficiencyScope
		sufficient bool
	}{
		{"soft_required", nil, RuntimeSourceRequirementSoft, ExternalObservationSufficiencyScopeExternalLane, true},
		{"pure_external", func(rm *RequestModel, hint *TurnRouteHint) {
			hint.CurrentSourceEvidenceMode = TurnRouteCurrentSourceEvidenceOptional
		}, RuntimeSourceRequirementNone, ExternalObservationSufficiencyScopeSourceOptionalAnswer, true},
		{"precise_mixed", func(rm *RequestModel, hint *TurnRouteHint) {
			rm.RequestedAnswerDimensions = &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true,
				Dimensions: []RequestedAnswerDimension{{Index: 1, Label: "source location", Role: RequestedAnswerDimensionSourceLocation, Required: true}}}
		}, RuntimeSourceRequirementPrecise, ExternalObservationSufficiencyScopeUnknown, false},
		{"optional_route_keeps_independent_precise_source", func(rm *RequestModel, hint *TurnRouteHint) {
			hint.CurrentSourceEvidenceMode = TurnRouteCurrentSourceEvidenceOptional
			rm.AnalyzerHints.RequiredFileHints = []RequiredFileHint{{Path: "internal/parser.go", Confidence: 0.95}}
		}, RuntimeSourceRequirementPrecise, ExternalObservationSufficiencyScopeUnknown, false},
		{"source_excluded", func(rm *RequestModel, hint *TurnRouteHint) {
			rm.SourceScopeProfile = &SourceScopeProfile{RequestedScope: SourceScopeProduction, Confidence: 0.95}
			rm.ExternalObservationPolicy.CurrentSourceMode = ExternalObservationCurrentSourceExclude
			rm.ExternalObservationPolicy.ExclusionKind = ExternalObservationSourceExclusionExplicitUserBoundary
			rm.ExternalObservationPolicy.SourceQuotes = []string{"only the attached observations, do not analyze source"}
		}, RuntimeSourceRequirementNone, ExternalObservationSufficiencyScopeSourceOptionalAnswer, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rm := &RequestModel{ExternalObservationPolicy: &ExternalObservationPolicy{ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly}}
			hint := TurnRouteHint{Route: "repo", Source: "artifact", CurrentSourceEvidenceMode: TurnRouteCurrentSourceEvidenceRequired}
			if tc.configure != nil {
				tc.configure(rm, &hint)
			}
			records := []ObservationRecord{runtimeSourceTraceRecord("runtime:operation", "trace_query")}
			before, _ := json.Marshal(rm)
			suff := AssessExternalObservationSufficiency(records, rm, hint)
			authority := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{RequestModel: rm, RouteHint: hint, Ledger: ObservationLedger{Records: records}})
			if suff.Status.Sufficient() != tc.sufficient || suff.Scope != tc.scope || suff.CurrentSourceRequirement != tc.precision {
				t.Fatalf("scoped sufficiency differs from typed request: %+v", suff)
			}
			if authority.ExternalObservationScope != suff.Scope || authority.CurrentSourceRequirement != suff.CurrentSourceRequirement ||
				authority.CurrentSourceRequired != (tc.precision != RuntimeSourceRequirementNone) ||
				authority.CanHardBlockCompletion != (tc.precision == RuntimeSourceRequirementPrecise) {
				t.Fatalf("authority reinterpreted sufficiency or hardened soft: %+v / %+v", suff, authority)
			}
			if tc.precision == RuntimeSourceRequirementSoft && (!authority.CanDowngradeToCaveat || strings.Contains(suff.Reason, "optional")) {
				t.Fatalf("soft must preserve caveat eligibility without claiming source optionality: %+v / %+v", suff, authority)
			}
			if tc.sufficient {
				encoded, err := json.Marshal(authority)
				if err != nil || !strings.Contains(string(encoded), `"external_observation_scope":"`+string(tc.scope)+`"`) {
					t.Fatalf("diagnostic JSON must retain the scope without a new model input: %v %s", err, encoded)
				}
			}
			after, _ := json.Marshal(rm)
			if string(before) != string(after) {
				t.Fatal("assessment changed request ownership")
			}
		})
	}
}

func TestB1699MCPPlusSatisfiedSourcePreservesBothCarriers(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{MCPResponses: []MCPResponse{{
		ServerName: "fixture", Success: true, ResourceURI: "mcp://fixture/rows",
		Observations: []MCPTypedObservation{{Summary: "recorded operation completed", LineStart: 7, LineEnd: 7}},
	}}})
	ledger.Records = append(ledger.Records, ObservationRecord{ID: "source:operation", Origin: AnswerEvidenceOriginCurrentSource,
		SourceRef: ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "internal/parser.go"}, Span: ObservationSpan{LineStart: 42}})
	rm := &RequestModel{ExternalObservationPolicy: &ExternalObservationPolicy{ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly}}
	hint := TurnRouteHint{Route: "repo", Source: "external_tool", CurrentSourceEvidenceMode: TurnRouteCurrentSourceEvidenceRequired}
	authority := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{RequestModel: rm, RouteHint: hint, Ledger: ledger})
	if authority.RuntimeObservationCount != 0 || !authority.CurrentSourceSatisfied || authority.NeedsCurrentSourceEvidence ||
		!authority.HasRuntimeCarrier() || !authority.HasMixedRuntimeCurrentSourceCarrier() || authority.CanUseRuntimeOnlyWithCaveat ||
		authority.ExternalObservationScope != ExternalObservationSufficiencyScopeExternalLane {
		t.Fatalf("source satisfaction must not erase the non-runtime external carrier: %+v", authority)
	}
}
