package types_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRequestObligationsIndependentObligationProjection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		keep   bool
		mutate func(*types.RequestModel, *types.TurnRouteHint)
	}{
		{name: "accepted false only route obligation"},
		{name: "absent declaration", keep: true, mutate: func(r *types.RequestModel, _ *types.TurnRouteHint) { r.CurrentSourceExplanationProfile = nil }},
		{name: "positive declaration", keep: true, mutate: func(r *types.RequestModel, _ *types.TurnRouteHint) {
			r.CurrentSourceExplanationProfile.IsCurrentSourceExplanationRequested = true
			r.CurrentSourceExplanationProfile.SourceQuotes = []string{"current code"}
		}},
		{name: "ordinary source", keep: true, mutate: func(_ *types.RequestModel, h *types.TurnRouteHint) { h.Source = "repo" }},
		{name: "independent mode without outcome", keep: true, mutate: func(_ *types.RequestModel, h *types.TurnRouteHint) {
			h.RequiredOutcomes &^= types.TurnOutcomeSourceExplanation
		}},
		{name: "current key code", keep: true, mutate: func(r *types.RequestModel, _ *types.TurnRouteHint) {
			r.RequestedAnswerDimensions.Dimensions[0].Role = types.RequestedAnswerDimensionCurrentKeyCode
		}},
		{name: "source location", keep: true, mutate: func(r *types.RequestModel, _ *types.TurnRouteHint) {
			r.RequestedAnswerDimensions.Dimensions[0].Role = types.RequestedAnswerDimensionSourceLocation
		}},
		{name: "source attribute", keep: true, mutate: func(r *types.RequestModel, _ *types.TurnRouteHint) {
			r.RequestedAnswerDimensions.Dimensions[0].Role = types.RequestedAnswerDimensionSourceAttribute
		}},
		{name: "one Makefile dimension", keep: true, mutate: func(r *types.RequestModel, _ *types.TurnRouteHint) {
			r.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{Path: "Makefile", Confidence: 1, RequestedDimensionIndices: []int{1}}}
		}},
		{name: "exact source file", keep: true, mutate: func(r *types.RequestModel, _ *types.TurnRouteHint) {
			r.AnalyzerHints.ExactTargets = []string{"engine.go"}
		}},
		{name: "user pinned extensionless source", keep: true, mutate: func(r *types.RequestModel, _ *types.TurnRouteHint) { r.UserPinnedFiles = []string{"Makefile"} }},
		{name: "dropped precise source dimension", keep: true, mutate: func(r *types.RequestModel, _ *types.TurnRouteHint) {
			r.CurrentSourceObligationSignals = []types.CurrentSourceObligationSignal{{Kind: types.CurrentSourceObligationSignalDroppedRequestedDimension, Role: types.RequestedAnswerDimensionCurrentKeyCode, Index: 2}}
		}},
		{name: "declared precise source inventory", keep: true, mutate: func(r *types.RequestModel, _ *types.TurnRouteHint) {
			r.SourceInventoryProfile = &types.SourceInventoryProfile{DeclarationOrigin: types.SourceInventoryDeclarationModelProvided, IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleType}, RequiresConstSet: true, SourceQuotes: []string{"source enum types"}}
		}},
		{name: "synthesized display inventory", mutate: func(r *types.RequestModel, _ *types.TurnRouteHint) {
			r.SourceInventoryProfile = &types.SourceInventoryProfile{DeclarationOrigin: types.SourceInventoryDeclarationSynthesized, IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction}}
		}},
		{name: "source call chain exact", keep: true, mutate: func(r *types.RequestModel, _ *types.TurnRouteHint) {
			r.CallChainEndpointProfile = &types.CallChainEndpointProfile{Source: "Foo.Run", Sink: "Bar.Save", SinkMode: types.CallChainSinkResolutionExact}
		}},
		{name: "source call chain discovery", keep: true, mutate: func(r *types.RequestModel, _ *types.TurnRouteHint) {
			r.CallChainEndpointProfile = &types.CallChainEndpointProfile{SinkMode: types.CallChainSinkResolutionDiscoverPath}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rm := types.RequestModel{
				PerfTrace:                       &types.PerfBundle{Observations: []types.PerfObservation{{Kind: "attached_runtime_artifact", Subject: "capture"}}},
				CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{},
				RequestedAnswerDimensions:       &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Index: 1, Required: true, Role: types.RequestedAnswerDimensionFunctionOrPurpose}}},
			}
			h := types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, NeedsDataAccess: true, Confidence: .99, RequiredOutcomes: types.TurnOutcomeAnswer | types.TurnOutcomeMeasurement | types.TurnOutcomeSourceExplanation, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
			if tc.mutate != nil {
				tc.mutate(&rm, &h)
			}
			before, _ := json.Marshal(rm)
			original := h
			got := types.EffectiveRequestRouteHint(&rm, h)
			if got.RequiresCurrentSourceEvidence() != tc.keep {
				t.Fatalf("source requirement=%v want=%v original=%+v effective=%+v", got.RequiresCurrentSourceEvidence(), tc.keep, h, got)
			}
			if tc.keep && !reflect.DeepEqual(got, h) {
				t.Fatalf("independent source obligation changed: %+v -> %+v", h, got)
			}
			if !tc.keep {
				want := h
				want.RequiredOutcomes &^= types.TurnOutcomeSourceExplanation
				want.CurrentSourceEvidenceMode = types.TurnRouteCurrentSourceEvidenceOptional
				if got != want {
					t.Fatalf("changed non-obligation fields: %+v want %+v", got, want)
				}
			}
			if again := types.EffectiveRequestRouteHint(&rm, got); again != got {
				t.Fatal("projection not idempotent")
			}
			after, _ := json.Marshal(rm)
			if string(before) != string(after) || original != h {
				t.Fatal("mutated accepted request/raw route")
			}
		})
	}
}

func TestRequestObligationsIndependentFalseCannotWaiveAnswerContract(t *testing.T) {
	rm := &types.RequestModel{PerfTrace: &types.PerfBundle{}, CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{}}
	h := types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, RequiredOutcomes: types.TurnOutcomeAnswer | types.TurnOutcomeSourceExplanation, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
	contract := &types.AnswerContract{CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{Required: true}}
	if got := types.RuntimeSourceRequestCurrentSourceRequirementPrecisionForContract(rm, h, contract); got != types.RuntimeSourceRequirementPrecise {
		t.Fatalf("false waived exact answer contract: %q", got)
	}
}
