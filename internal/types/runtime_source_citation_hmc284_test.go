package types

import (
	"encoding/json"
	"testing"
)

func TestHMC284RuntimeSourceCitationApplicability(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*RequestModel, *TurnRouteHint, *AnswerContract, *ObservationLedger)
		want bool
	}{
		{"native_only", nil, true},
		{"negative_source_navigation", func(_ *RequestModel, _ *TurnRouteHint, _ *AnswerContract, l *ObservationLedger) {
			l.Records = append(l.Records, ObservationRecord{Origin: AnswerEvidenceOriginRepoNegativeSearch, Negative: true, SourceRef: ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "support.go"}})
		}, true},
		{"incidental_source_read", func(_ *RequestModel, _ *TurnRouteHint, _ *AnswerContract, l *ObservationLedger) {
			l.Records = append(l.Records, ObservationRecord{Origin: AnswerEvidenceOriginCurrentSource, SourceRef: ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "support.go"}, Span: ObservationSpan{LineStart: 1}})
		}, true},
		{"independent_source_explanation", func(r *RequestModel, _ *TurnRouteHint, _ *AnswerContract, _ *ObservationLedger) {
			r.CurrentSourceExplanationProfile = &CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true, SourceQuotes: []string{"explain current mechanism"}}
		}, false},
		{"required_route", func(_ *RequestModel, h *TurnRouteHint, _ *AnswerContract, _ *ObservationLedger) {
			h.CurrentSourceEvidenceMode = TurnRouteCurrentSourceEvidenceRequired
		}, false},
		{"source_location", func(r *RequestModel, _ *TurnRouteHint, _ *AnswerContract, _ *ObservationLedger) {
			r.RequestedAnswerDimensions = &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{{Role: RequestedAnswerDimensionSourceLocation, Required: true, SourceQuote: "source location"}}}
		}, false},
		{"change_impact", func(r *RequestModel, _ *TurnRouteHint, _ *AnswerContract, _ *ObservationLedger) {
			r.ChangeImpactProfile = &ChangeImpactProfile{IsChangeImpact: true}
		}, false},
		{"current_version", func(r *RequestModel, _ *TurnRouteHint, _ *AnswerContract, _ *ObservationLedger) {
			r.DiagnosticProfile.CurrentVersionCheck = true
		}, false},
		{"current_status_contract", func(_ *RequestModel, _ *TurnRouteHint, c *AnswerContract, _ *ObservationLedger) {
			c.CurrentStatusDiagnostic = &CurrentStatusDiagnosticContract{Required: true}
		}, false},
		{"exact_source_contract", func(_ *RequestModel, _ *TurnRouteHint, c *AnswerContract, _ *ObservationLedger) {
			c.ExactResolution = &ExactResolutionContract{TargetLabel: "support.go:12"}
		}, false},
		{"no_native_query", func(_ *RequestModel, _ *TurnRouteHint, _ *AnswerContract, l *ObservationLedger) { l.Records = nil }, false},
		{"model_only_runtime", func(_ *RequestModel, _ *TurnRouteHint, _ *AnswerContract, l *ObservationLedger) {
			l.Records[0].Producer = "aggregate_fact"
			l.Records[0].ClaimAuthority = ObservationClaimAuthorityUnknown
		}, false},
		{"unaddressable_query", func(_ *RequestModel, _ *TurnRouteHint, _ *AnswerContract, l *ObservationLedger) {
			l.Records[0].SourceRef = ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact}
			l.Records[0].Span = ObservationSpan{}
		}, false},
		{"separate_model_address_cannot_lend_authority", func(_ *RequestModel, _ *TurnRouteHint, _ *AnswerContract, l *ObservationLedger) {
			model := l.Records[0]
			model.Producer = "aggregate_fact"
			model.ClaimAuthority = ObservationClaimAuthorityUnknown
			l.Records[0].SourceRef = ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact}
			l.Records[0].Span = ObservationSpan{}
			l.Records = append(l.Records, model)
		}, false},
		{"source_only", func(r *RequestModel, h *TurnRouteHint, _ *AnswerContract, _ *ObservationLedger) {
			*r = RequestModel{}
			*h = TurnRouteHint{}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rm := RequestModel{RuntimeQuestionProfile: &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeBoundedFactSet}}
			hint := TurnRouteHint{Source: "external_tool", CurrentSourceEvidenceMode: TurnRouteCurrentSourceEvidenceOptional}
			contract := AnswerContract{CitationReq: CitationReq{Required: true, MinCitations: 3}}
			ledger := ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("native", "trace_query")}}
			if tc.edit != nil {
				tc.edit(&rm, &hint, &contract, &ledger)
			}
			before, _ := json.Marshal(ledger)
			s := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{RequestModel: &rm, RouteHint: hint, Ledger: ledger})
			if got := s.AllowsNativeRuntimeAnswerWithoutSourceCitationFloor(&rm, &contract); got != tc.want {
				t.Fatalf("applicability=%v want=%v: %+v", got, tc.want, s)
			}
			if tc.name == "incidental_source_read" || tc.name == "negative_source_navigation" {
				if !s.CurrentSourceSatisfied || !s.KeepsCurrentSourceLaneLoadBearing() || s.AllowsRuntimeEvidenceWithoutCurrentSource() {
					t.Fatal("new citation applicability must not erase landed source or change general source retention")
				}
			}
			after, _ := json.Marshal(ledger)
			if string(before) != string(after) {
				t.Fatal("applicability mutated evidence")
			}
		})
	}
}
