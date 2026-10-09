package types

import (
	"encoding/json"
	"testing"
)

func TestRuntimeRequestObligationsRefineOnlyRouteDerivedSource(t *testing.T) {
	for _, tc := range []struct {
		name        string
		adjust      func(*RequestModel, *TurnRouteHint)
		wantSource  bool
		wantPrecise bool
	}{
		{name: "explicit false refines route-only requirement"},
		{name: "omitted profile preserves legacy", wantSource: true, adjust: func(rm *RequestModel, _ *TurnRouteHint) { rm.CurrentSourceExplanationProfile = nil }},
		{name: "positive mixed explanation", wantSource: true, adjust: func(rm *RequestModel, _ *TurnRouteHint) {
			rm.CurrentSourceExplanationProfile = &CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true, SourceQuotes: []string{"current implementation"}}
		}},
		{name: "required mode without outcome is independent", wantSource: true, adjust: func(_ *RequestModel, h *TurnRouteHint) { h.RequiredOutcomes = TurnOutcomeMeasurement }},
		{name: "ordinary source request", wantSource: true, adjust: func(rm *RequestModel, h *TurnRouteHint) { rm.PerfTrace = nil; h.Source = "repo" }},
		{name: "current key code remains soft", wantSource: true, adjust: func(rm *RequestModel, _ *TurnRouteHint) {
			rm.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionCurrentKeyCode
		}},
		{name: "source location remains precise", wantSource: true, wantPrecise: true, adjust: func(rm *RequestModel, _ *TurnRouteHint) {
			rm.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionSourceLocation
		}},
		{name: "exact current source target", wantSource: true, wantPrecise: true, adjust: func(rm *RequestModel, _ *TurnRouteHint) { rm.AnalyzerHints.ExactTargets = []string{"engine.go"} }},
		{name: "file line target preserves existing soft precision", wantSource: true, adjust: func(rm *RequestModel, _ *TurnRouteHint) { rm.AnalyzerHints.ExactTargets = []string{"engine.go:7"} }},
		{name: "Makefile dimension binding", wantSource: true, adjust: func(rm *RequestModel, _ *TurnRouteHint) {
			rm.AnalyzerHints.RequiredFileHints = []RequiredFileHint{{Path: "Makefile", Confidence: 1, RequestedDimensionIndices: []int{1}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rm := RequestModel{PerfTrace: &PerfBundle{}, CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{}, RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{{Index: 1, Required: true, Role: RequestedAnswerDimensionFunctionOrPurpose}}}}
			hint := TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: TurnRouteCurrentSourceEvidenceRequired, RequiredOutcomes: TurnOutcomeAnswer | TurnOutcomeMeasurement | TurnOutcomeSourceExplanation}
			if tc.adjust != nil {
				tc.adjust(&rm, &hint)
			}
			before, _ := json.Marshal(rm)
			originalHint := hint
			authority := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{RequestModel: &rm, RouteHint: hint, Ledger: ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("measurement", "trace_query")}}})
			if authority.CurrentSourceRequired != tc.wantSource || authority.CanHardBlockCompletion != tc.wantPrecise {
				t.Fatalf("source=%v precise=%v want source=%v precise=%v: %+v", authority.CurrentSourceRequired, authority.CanHardBlockCompletion, tc.wantSource, tc.wantPrecise, authority)
			}
			if !tc.wantSource && (authority.CurrentSourceExplanationRequested || len(RequestedExplanationOperationNeedsForAuthority(&rm, authority)) != 0) {
				t.Fatalf("refined route retained explanation seats: %+v", authority)
			}
			after, _ := json.Marshal(rm)
			if string(before) != string(after) || hint != originalHint || authority.CurrentSourceLane == CurrentSourceLaneExcluded {
				t.Fatal("refinement mutated request/route or minted source exclusion")
			}
		})
	}
}

func TestCurrentSourceExplicitFalseSurvivesNormalizationAndJSON(t *testing.T) {
	profile, warnings := NormalizeCurrentSourceExplanationProfile("a runtime question", &CurrentSourceExplanationProfile{SourceQuotes: []string{"stale quote"}, Modes: []CurrentSourceExplanationMode{CurrentSourceExplanationLocateCurrentCode}})
	if profile == nil || profile.Active() || len(warnings) != 0 || len(profile.SourceQuotes) != 0 || len(profile.Modes) != 0 {
		t.Fatalf("explicit false not retained as clean declaration: %+v %v", profile, warnings)
	}
	data, err := json.Marshal(RequestModel{CurrentSourceExplanationProfile: profile})
	if err != nil {
		t.Fatal(err)
	}
	var replay RequestModel
	if err := json.Unmarshal(data, &replay); err != nil {
		t.Fatal(err)
	}
	if replay.CurrentSourceExplanationProfile == nil || replay.CurrentSourceExplanationProfile.Active() {
		t.Fatal("false became omitted during handoff")
	}
	if profile, _ := NormalizeCurrentSourceExplanationProfile("question", nil); profile != nil {
		t.Fatal("omission invented a negative declaration")
	}
}

func TestRouteBackedHistoryExplanationUsesAcceptedObligations(t *testing.T) {
	rm := RequestModel{Intent: IntentExplain, Scenario: ScenarioArchitectureExplain, Predicates: SemanticPredicates{IsHistoryLookup: true}, CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{}}
	hint := TurnRouteHint{Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: TurnRouteCurrentSourceEvidenceRequired, RequiredOutcomes: TurnOutcomeSourceExplanation}
	if RouteBackedHistoryCurrentCodeExplanation(rm, hint) {
		t.Fatal("explicit false still synthesized a history source obligation")
	}
	rm.CurrentSourceExplanationProfile = nil
	if !RouteBackedHistoryCurrentCodeExplanation(rm, hint) {
		t.Fatal("legacy omitted profile lost its independent history/current-code outcome")
	}
	rm.CurrentSourceExplanationProfile = &CurrentSourceExplanationProfile{}
	rm.AnalyzerHints.ExactTargets = []string{"worker.go"}
	if !RouteBackedHistoryCurrentCodeExplanation(rm, hint) {
		t.Fatal("false erased exact history/current-code target")
	}
}
