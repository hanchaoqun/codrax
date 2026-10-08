package types

import (
	"encoding/json"
	"testing"
)

func TestExplanationOperationDomainDoesNotPromoteSoftSourceIntent(t *testing.T) {
	for _, tc := range []struct {
		name       string
		adjust     func(*RequestModel, *RuntimeSourceAnswerAuthorityInput)
		wantSource bool
	}{
		{name: "mixed required observation"},
		{name: "unrelated source read", adjust: func(_ *RequestModel, in *RuntimeSourceAnswerAuthorityInput) {
			in.Ledger.Records = append(in.Ledger.Records, ObservationRecord{ID: "readme", Origin: AnswerEvidenceOriginCurrentSource,
				SourceRef: ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "README.md"}, Span: ObservationSpan{LineStart: 1}})
		}},
		{name: "no observation", wantSource: true, adjust: func(_ *RequestModel, in *RuntimeSourceAnswerAuthorityInput) { in.Ledger = ObservationLedger{} }},
		{name: "triage only", wantSource: true, adjust: func(_ *RequestModel, in *RuntimeSourceAnswerAuthorityInput) {
			in.Ledger.Records[0].Producer = "perf_triager"
		}},
		{name: "unaddressable query", wantSource: true, adjust: func(_ *RequestModel, in *RuntimeSourceAnswerAuthorityInput) {
			in.Ledger.Records[0].SourceRef = ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact}
			in.Ledger.Records[0].Span = ObservationSpan{}
			in.Ledger.Records[0].SupportRefs = nil
		}},
		{name: "unaddressable query cannot borrow model address", wantSource: true, adjust: func(_ *RequestModel, in *RuntimeSourceAnswerAuthorityInput) {
			model := in.Ledger.Records[0]
			model.ID, model.Producer = "model:distribution", "model"
			in.Ledger.Records[0].SourceRef = ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact}
			in.Ledger.Records[0].Span = ObservationSpan{}
			in.Ledger.Records[0].SupportRefs = nil
			in.Ledger.Records = append(in.Ledger.Records, model)
		}},
		{name: "model aggregate", wantSource: true, adjust: func(_ *RequestModel, in *RuntimeSourceAnswerAuthorityInput) {
			in.Ledger.Records[0].Producer = "emit_answer_aggregate"
		}},
		{name: "model aggregate borrowing query producer", wantSource: true, adjust: func(_ *RequestModel, in *RuntimeSourceAnswerAuthorityInput) {
			in.Ledger.Records[0].ClaimAuthority = ObservationClaimAuthorityModelInference
		}},
		{name: "uncompiled unknown authority", wantSource: true, adjust: func(_ *RequestModel, in *RuntimeSourceAnswerAuthorityInput) {
			in.Ledger.Records[0].ClaimAuthority = ObservationClaimAuthorityUnknown
		}},
		{name: "independent aggregate is not direct query", wantSource: true, adjust: func(_ *RequestModel, in *RuntimeSourceAnswerAuthorityInput) {
			in.Ledger.Records[0].ClaimAuthority = ObservationClaimAuthorityIndependentlyProven
		}},
		{name: "independent source outcome", wantSource: true, adjust: func(_ *RequestModel, in *RuntimeSourceAnswerAuthorityInput) {
			in.RouteHint.RequiredOutcomes = TurnOutcomeMeasurement | TurnOutcomeSourceExplanation
		}},
		{name: "independent source explanation", wantSource: true, adjust: func(rm *RequestModel, _ *RuntimeSourceAnswerAuthorityInput) {
			rm.CurrentSourceExplanationProfile = &CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true,
				Modes: []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism}, SourceQuotes: []string{"explain the current implementation"}}
		}},
		{name: "exact source target", wantSource: true, adjust: func(rm *RequestModel, _ *RuntimeSourceAnswerAuthorityInput) {
			rm.AnalyzerHints.ExactTargets = []string{"worker.go:9"}
		}},
		{name: "Makefile owner", wantSource: true, adjust: func(rm *RequestModel, _ *RuntimeSourceAnswerAuthorityInput) {
			rm.AnalyzerHints.RequiredFileHints = []RequiredFileHint{{Path: "Makefile", Confidence: 1, RequestedDimensionIndices: []int{2}}}
		}},
		{name: "ordinary source despite incidental runtime query", wantSource: true, adjust: func(rm *RequestModel, in *RuntimeSourceAnswerAuthorityInput) {
			rm.PerfTrace = nil
			in.RouteHint = TurnRouteHint{}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rm := &RequestModel{PerfTrace: &PerfBundle{}, RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true,
				Dimensions: []RequestedAnswerDimension{{Index: 1, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true}, {Index: 2, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true}}}}
			in := RuntimeSourceAnswerAuthorityInput{RequestModel: rm,
				RouteHint: TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: TurnRouteCurrentSourceEvidenceRequired},
				Ledger:    ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("observed:distribution", "trace_query")}}}
			if tc.adjust != nil {
				tc.adjust(rm, &in)
			}
			authority := BuildRuntimeSourceAnswerAuthoritySnapshot(in)
			needs := RequestedExplanationOperationNeedsForAuthority(rm, authority)
			if (len(needs) > 0) != tc.wantSource {
				t.Fatalf("source operation applicability=%v want=%v; authority=%+v needs=%+v", len(needs) > 0, tc.wantSource, authority, needs)
			}
		})
	}
}

func TestExplanationOperationDomainComposesDocumentationSourceAndMeasurements(t *testing.T) {
	rm := &RequestModel{PerfTrace: &PerfBundle{},
		ToolDocumentationRequest: &ToolDocumentationRequest{Scope: ToolDocumentationRequestMixed, DimensionIndices: []int{1}},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{
			{Index: 1, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true},
			{Index: 2, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true},
			{Index: 3, Role: RequestedAnswerDimensionObservedValue, Required: true},
		}},
		AnalyzerHints: AnalyzerHints{RequiredFileHints: []RequiredFileHint{{Path: "Makefile", Confidence: 1, RequestedDimensionIndices: []int{2}}}},
	}
	if err := ValidateToolDocumentationRequest(rm); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(rm)
	in := RuntimeSourceAnswerAuthorityInput{RequestModel: rm,
		RouteHint: TurnRouteHint{Route: "repo", Source: "mixed", CurrentSourceEvidenceMode: TurnRouteCurrentSourceEvidenceRequired},
		Ledger:    ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("measurement:1", "trace_query")}},
	}
	needs := RequestedExplanationOperationNeedsForAuthority(rm, BuildRuntimeSourceAnswerAuthoritySnapshot(in))
	if len(needs) != 1 || needs[0].Dimension.Index != 2 || needs[0].Source != "Makefile" {
		t.Fatalf("source obligation must survive sibling documentation and measurement: %+v", needs)
	}
	after, _ := json.Marshal(rm)
	if string(before) != string(after) {
		t.Fatal("domain projection must not change any requested answer surface")
	}
}

func TestObservedDimensionShapesDoNotRequestSourceOperations(t *testing.T) {
	// Labels are opaque presentation text. The same typed role owns every
	// measured shape; changing domain vocabulary must not create source work.
	for _, labels := range [][2]string{{"CPU distribution", "coverage"}, {"IO rates", "unknown sizes"}, {"memory time series", "resident proportions"}, {"log level distribution", "missing interval coverage"}} {
		rm := &RequestModel{RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{
			{Index: 1, Label: labels[0], Role: RequestedAnswerDimensionObservedValue, Required: true},
			{Index: 2, Label: labels[1], Role: RequestedAnswerDimensionObservedValue, Required: true},
		}}}
		if needs := RequestedExplanationOperationNeedsForAuthority(rm, RuntimeSourceAnswerAuthoritySnapshot{}); len(needs) != 0 {
			t.Fatalf("measured shape %v unexpectedly requests source operations: %+v", labels, needs)
		}
	}
}
