package types

import (
	"reflect"
	"testing"
)

func TestRequestedExplanationOperationNeedsConsumesSharedSourceAuthorityOnly(t *testing.T) {
	rm := &RequestModel{
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{
			{Index: 3, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true},
			{Index: 4, Role: RequestedAnswerDimensionBranchBehavior, Required: true},
		}},
		AnalyzerHints: AnalyzerHints{RequiredFileHints: []RequiredFileHint{{
			Path: "worker.go", Confidence: 1, RequestedDimensionIndices: []int{3},
		}}},
	}
	want := RequestedExplanationOperationNeeds(rm.RequestedAnswerDimensions, rm.AnalyzerHints.RequiredFileHints)
	for _, authority := range []RuntimeSourceAnswerAuthoritySnapshot{
		{},
		{CurrentSourceLane: CurrentSourceLaneRequired, CurrentSourceRequirement: RuntimeSourceRequirementPrecise},
		{CurrentSourceLane: CurrentSourceLaneAllowedOptional, RuntimeObservationCount: 2, RuntimeOnlySufficient: true},
		{Active: true, CurrentSourceLane: CurrentSourceLaneRequired, CurrentSourceRequirement: RuntimeSourceRequirementPrecise, RuntimeObservationCount: 2, RuntimeOnlySufficient: true},
		{Active: true, CurrentSourceLane: CurrentSourceLaneAllowedOptional, RuntimeOnlySufficient: true},
		{CurrentSourceLane: CurrentSourceLaneSatisfiedAbsent},
	} {
		if got := RequestedExplanationOperationNeedsForAuthority(rm, authority); !reflect.DeepEqual(got, want) {
			t.Fatalf("non-excluded authority must preserve exact operation seats: authority=%+v got=%+v", authority, got)
		}
	}
	if got := RequestedExplanationOperationNeedsForAuthority(rm, RuntimeSourceAnswerAuthoritySnapshot{CurrentSourceLane: CurrentSourceLaneExcluded}); len(got) != 0 {
		t.Fatalf("existing authority exclusion must not be re-derived from roles or file hints: %+v", got)
	}
	if got := RequestedExplanationOperationNeedsForAuthority(nil, RuntimeSourceAnswerAuthoritySnapshot{}); len(got) != 0 {
		t.Fatalf("nil request produced operation seats: %+v", got)
	}
}

func TestRequestedExplanationOperationNeedsRuntimeOptionalAuthority(t *testing.T) {
	for _, tc := range []struct {
		name       string
		adjust     func(*RequestModel, *RuntimeSourceAnswerAuthorityInput)
		wantSource bool
	}{
		{name: "default runtime optional"},
		{name: "invalid exclusion has no authority", adjust: func(rm *RequestModel, _ *RuntimeSourceAnswerAuthorityInput) {
			rm.ExternalObservationPolicy = &ExternalObservationPolicy{CurrentSourceMode: ExternalObservationCurrentSourceExclude, ExclusionKind: ExternalObservationSourceExclusionExplicitUserBoundary}
		}},
		{name: "attachment without observation", wantSource: true, adjust: func(_ *RequestModel, in *RuntimeSourceAnswerAuthorityInput) { in.Ledger = ObservationLedger{} }},
		{name: "required route", wantSource: true, adjust: func(_ *RequestModel, in *RuntimeSourceAnswerAuthorityInput) {
			in.RouteHint.CurrentSourceEvidenceMode = TurnRouteCurrentSourceEvidenceRequired
		}},
		{name: "mixed required route", wantSource: true, adjust: func(_ *RequestModel, in *RuntimeSourceAnswerAuthorityInput) {
			in.RouteHint.Source = "mixed"
			in.RouteHint.CurrentSourceEvidenceMode = TurnRouteCurrentSourceEvidenceRequired
		}},
		{name: "precise target", wantSource: true, adjust: func(rm *RequestModel, _ *RuntimeSourceAnswerAuthorityInput) {
			rm.AnalyzerHints.ExactTargets = []string{"worker.go:9"}
		}},
		{name: "one exact file binding retains whole contract", wantSource: true, adjust: func(rm *RequestModel, _ *RuntimeSourceAnswerAuthorityInput) {
			rm.AnalyzerHints.RequiredFileHints = []RequiredFileHint{{Path: "worker.go", Confidence: 1, RequestedDimensionIndices: []int{3}}}
		}},
		{name: "extensionless exact binding retains whole contract", wantSource: true, adjust: func(rm *RequestModel, _ *RuntimeSourceAnswerAuthorityInput) {
			rm.AnalyzerHints.RequiredFileHints = []RequiredFileHint{{Path: "Makefile", Confidence: 1, RequestedDimensionIndices: []int{3}}}
		}},
		{name: "landed source proof", wantSource: true, adjust: func(_ *RequestModel, in *RuntimeSourceAnswerAuthorityInput) {
			in.Ledger.Records = append(in.Ledger.Records, ObservationRecord{ID: "source:worker", Origin: AnswerEvidenceOriginCurrentSource, SourceRef: ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "worker.go"}, Span: ObservationSpan{LineStart: 9}})
		}},
		{name: "ordinary source", wantSource: true, adjust: func(rm *RequestModel, in *RuntimeSourceAnswerAuthorityInput) {
			rm.PerfTrace = nil
			in.RouteHint = TurnRouteHint{}
			in.Ledger = ObservationLedger{}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rm := &RequestModel{RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{
				{Index: 3, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true},
				{Index: 4, Role: RequestedAnswerDimensionBranchBehavior, Required: true},
			}}, PerfTrace: &PerfBundle{}}
			in := RuntimeSourceAnswerAuthorityInput{RequestModel: rm, RouteHint: TurnRouteHint{Route: "repo", Source: "external_tool", NeedsRepoAccess: true, CurrentSourceEvidenceMode: TurnRouteCurrentSourceEvidenceOptional}, Ledger: ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("trace:distribution", "trace_query")}}}
			if tc.adjust != nil {
				tc.adjust(rm, &in)
			}
			authority := BuildRuntimeSourceAnswerAuthoritySnapshot(in)
			want := RequestedExplanationOperationNeeds(rm.RequestedAnswerDimensions, rm.AnalyzerHints.RequiredFileHints)
			if !tc.wantSource {
				if authority.CurrentSourceLane != CurrentSourceLaneAllowedOptional || authority.CurrentSourceRequired || authority.RuntimeObservationCount == 0 {
					t.Fatalf("fixture must exercise source optionality, not exclusion: %+v", authority)
				}
				want = nil
			}
			if got := RequestedExplanationOperationNeedsForAuthority(rm, authority); !reflect.DeepEqual(got, want) {
				t.Fatalf("source applicability contradicts shared authority: authority=%+v got=%+v want=%+v", authority, got, want)
			}
		})
	}
}

func TestRequestedExplanationOperationNeedsDoesNotWaiveFromBroadBits(t *testing.T) {
	rm := &RequestModel{RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{
			{Index: 1, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true},
			{Index: 2, Role: RequestedAnswerDimensionBranchBehavior, Required: true},
		},
	}}
	want := RequestedExplanationOperationNeeds(rm.RequestedAnswerDimensions, nil)
	for _, tc := range []struct {
		name      string
		authority RuntimeSourceAnswerAuthoritySnapshot
	}{
		{"broad runtime sufficiency alone", RuntimeSourceAnswerAuthoritySnapshot{Active: true, CurrentSourceLane: CurrentSourceLaneAllowedOptional, RuntimeOnlySufficient: true}},
		{"citation policy alone", RuntimeSourceAnswerAuthoritySnapshot{Active: true, CurrentSourceLane: CurrentSourceLaneAllowedOptional, RuntimeCitationPolicy: RuntimeGroundingCitationRuntimeObservation}},
		{"triage observation without deterministic query", RuntimeSourceAnswerAuthoritySnapshot{Active: true, CurrentSourceLane: CurrentSourceLaneAllowedOptional, RuntimeObservationCount: 1, RuntimeOnlySufficient: true}},
		{"inactive authority", RuntimeSourceAnswerAuthoritySnapshot{CurrentSourceLane: CurrentSourceLaneAllowedOptional, RuntimeObservationCount: 1, DeterministicRuntimeQueryCount: 1, RuntimeOnlySufficient: true}},
		{"precise source wins over runtime sufficiency", RuntimeSourceAnswerAuthoritySnapshot{Active: true, CurrentSourceLane: CurrentSourceLaneRequired, RuntimeObservationCount: 1, DeterministicRuntimeQueryCount: 1, RuntimeOnlySufficient: true, CurrentSourceRequirement: RuntimeSourceRequirementPrecise}},
		{"landed source remains load bearing", RuntimeSourceAnswerAuthoritySnapshot{Active: true, CurrentSourceLane: CurrentSourceLaneAllowedOptional, RuntimeObservationCount: 1, DeterministicRuntimeQueryCount: 1, RuntimeOnlySufficient: true, CurrentSourceSatisfied: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := RequestedExplanationOperationNeedsForAuthority(rm, tc.authority); !reflect.DeepEqual(got, want) {
				t.Fatalf("non-authoritative runtime signal waived source ownership: got=%+v authority=%+v", got, tc.authority)
			}
		})
	}
}

func TestRequestedExplanationOperationNeedsUsesOnlyExplicitHighConfidenceFileBindings(t *testing.T) {
	profile := &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{
		{Index: 1, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		{Index: 2, Role: RequestedAnswerDimensionObservedValue, Required: true},
		{Index: 3, Role: RequestedAnswerDimensionBranchBehavior, Required: true},
	}}
	hints := []RequiredFileHint{
		{Path: "./config/load.go", Confidence: 0.95, RequestedDimensionIndices: []int{1}},
		{Path: "cmd\\root.go", Confidence: 0.9, RequestedDimensionIndices: []int{3}},
		{Path: "noise.go", Confidence: 0.7, RequestedDimensionIndices: []int{1}},
	}
	needs := RequestedExplanationOperationNeeds(profile, hints)
	if len(needs) != 2 || needs[0].Dimension.Index != 1 || needs[0].Source != "config/load.go" ||
		needs[1].Dimension.Index != 3 || needs[1].Source != "cmd/root.go" {
		t.Fatalf("needs=%+v", needs)
	}

	wrongFile := EvidenceItem{Kind: EvidenceMechanism, AnchorKind: AnchorCall, Source: "cmd/root.go", GroundingStatus: GroundingGrounded, RequestedDimensionIndices: []int{1}}
	if RequestedExplanationOperationNeedCovered(needs[0], wrongFile) {
		t.Fatal("sibling-file operation must not close the config/load.go seat")
	}
	rightFile := wrongFile
	rightFile.Source = "./config/load.go"
	if !RequestedExplanationOperationNeedCovered(needs[0], rightFile) {
		t.Fatal("exact file-scoped grounded operation should close its seat")
	}
}

func TestRequestedExplanationOperationNeedRejectsDefinitionRegardlessOfEvidenceKind(t *testing.T) {
	need := RequestedExplanationOperationNeed{Dimension: RequestedAnswerDimension{
		Index: 1, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true,
	}}
	for _, kind := range []EvidenceKind{
		EvidenceMechanism, EvidenceRelationship, EvidenceRegistration,
		EvidenceConditional, EvidenceDataflowPath, EvidenceControlFlow,
	} {
		item := EvidenceItem{
			Kind: kind, AnchorKind: AnchorDefinition,
			GroundingStatus: GroundingGrounded, RequestedDimensionIndices: []int{1},
		}
		if RequestedExplanationOperationNeedCovered(need, item) {
			t.Fatalf("definition anchor must not become an operation through evidence kind %q", kind)
		}
	}
	item := EvidenceItem{
		Kind: EvidenceMechanism, AnchorKind: AnchorCall,
		GroundingStatus: GroundingGrounded, RequestedDimensionIndices: []int{1},
	}
	if !RequestedExplanationOperationNeedCovered(need, item) {
		t.Fatal("grounded call anchor should close the unscoped operation seat")
	}
}

func TestRequestedExplanationOperationNeedsPreservesLegacyUnscopedBehavior(t *testing.T) {
	profile := &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{
		{Index: 1, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		{Index: 3, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true},
	}}
	needs := RequestedExplanationOperationNeeds(profile, []RequiredFileHint{{Path: "nav.go", Confidence: 1}})
	if len(needs) != 2 || needs[0].Source != "" || needs[1].Source != "" {
		t.Fatalf("navigation-only required files must preserve unscoped operation seats: %+v", needs)
	}
}
