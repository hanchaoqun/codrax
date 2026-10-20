package types

import (
	"encoding/json"
	"testing"
)

func hmc271SourceLocationRequest() RequestModel {
	return RequestModel{
		Intent: IntentExplain, PredicateAxis: AxisFlow,
		AnalyzerHints: AnalyzerHints{Kind: string(ReqMechanism)},
		RuntimeQuestionProfile: &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []RuntimeQuestionFactFamily{RuntimeQuestionFactCountOrDuration}},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true,
			Dimensions: []RequestedAnswerDimension{{Role: RequestedAnswerDimensionSourceLocation, Required: true, Index: 1}}},
	}
}

func TestHMC271SourceLocationScopePreservesIndependentSourceWork(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*RequestModel)
	}{
		{"bounded_effect", func(r *RequestModel) { r.RuntimeQuestionProfile.Scope = RuntimeQuestionScopeBoundedEffectVerdict }},
		{"causal", func(r *RequestModel) { r.RuntimeQuestionProfile.Scope = RuntimeQuestionScopeCausalDiagnosis }},
		{"legacy_source", func(r *RequestModel) { r.RuntimeQuestionProfile = nil }},
		{"current_key_code", func(r *RequestModel) {
			r.RequestedAnswerDimensions.Dimensions = append(r.RequestedAnswerDimensions.Dimensions,
				RequestedAnswerDimension{Role: RequestedAnswerDimensionCurrentKeyCode, Required: true, Index: 2})
		}},
		{"branch", func(r *RequestModel) {
			r.RequestedAnswerDimensions.Dimensions = append(r.RequestedAnswerDimensions.Dimensions,
				RequestedAnswerDimension{Role: RequestedAnswerDimensionBranchBehavior, Required: true, Index: 2})
		}},
		{"relation", func(r *RequestModel) {
			r.RequestedAnswerDimensions.Dimensions = append(r.RequestedAnswerDimensions.Dimensions,
				RequestedAnswerDimension{Role: RequestedAnswerDimensionRelationPath, Required: true, Index: 2})
		}},
		{"required_incident_diagram", func(r *RequestModel) {
			r.DiagramHint = &DiagramHint{Kind: DiagramSequence, Required: true,
				Participants: []DiagramParticipantHint{{Identity: "opaque", Role: DiagramParticipantIncidentRequired}}}
		}},
		{"call_chain", func(r *RequestModel) { r.AnalyzerHints.Kind = string(ReqCallChain) }},
		{"runtime_selection_without_endpoints", func(r *RequestModel) {
			r.CallChainEndpointProfile = &CallChainEndpointProfile{RuntimeSelectionRequired: true}
		}},
		{"source_inventory", func(r *RequestModel) {
			r.SourceInventoryProfile = &SourceInventoryProfile{IsSourceInventory: true, DeclarationOrigin: SourceInventoryDeclarationModelProvided,
				TargetRoles: []AnswerCandidateRole{AnswerCandidateRoleFunction}, SourceQuotes: []string{"functions"}}
		}},
		{"change_impact", func(r *RequestModel) { r.ChangeImpactProfile = &ChangeImpactProfile{IsChangeImpact: true} }},
		{"dropped_operation", func(r *RequestModel) {
			r.CurrentSourceObligationSignals = []CurrentSourceObligationSignal{{Kind: CurrentSourceObligationSignalDroppedRequestedDimension, Role: RequestedAnswerDimensionFunctionOrPurpose}}
		}},
		{"source_flow", func(r *RequestModel) {
			r.CurrentSourceExplanationProfile = &CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true,
				Modes: []CurrentSourceExplanationMode{CurrentSourceExplanationTraceCurrentFlow}, SourceQuotes: []string{"flow"}}
		}},
		{"source_mechanism", func(r *RequestModel) {
			r.CurrentSourceExplanationProfile = &CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true,
				Modes: []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism}, SourceQuotes: []string{"mechanism"}}
		}},
		{"unspecified_positive", func(r *RequestModel) {
			r.CurrentSourceExplanationProfile = &CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true, SourceQuotes: []string{"current source"}}
		}},
		{"verify_status", func(r *RequestModel) {
			r.CurrentSourceExplanationProfile = &CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true,
				Modes: []CurrentSourceExplanationMode{CurrentSourceExplanationVerifyCurrentStatus}, SourceQuotes: []string{"current status"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rm := hmc271SourceLocationRequest()
			tc.edit(&rm)
			before, _ := json.Marshal(rm)
			if RuntimeSourceLocationOnly(rm) {
				t.Fatal("independent source/causal obligation was waived as location-only")
			}
			after, _ := json.Marshal(rm)
			if string(before) != string(after) {
				t.Fatal("derived scope modified the accepted request")
			}
		})
	}
}

func TestHMC271SourceLocationOnlyRetainsNeutralPresentation(t *testing.T) {
	rm := hmc271SourceLocationRequest()
	rm.DiagramHint = &DiagramHint{Kind: DiagramSequence, Required: true}
	rm.RequestedAnswerDimensions.Dimensions = append(rm.RequestedAnswerDimensions.Dimensions,
		RequestedAnswerDimension{Role: RequestedAnswerDimensionStageWorkflow, Required: true, Index: 2})
	before, _ := json.Marshal(rm)
	if !RuntimeSourceLocationOnly(rm) {
		t.Fatal("generic diagram/workflow presentation alone recreated source transfer work")
	}
	if got := RuntimeSourceRequestCurrentSourceRequirementPrecision(&rm, TurnRouteHint{}); got != RuntimeSourceRequirementPrecise {
		t.Fatal("location-only scope removed source precision")
	}
	after, _ := json.Marshal(rm)
	if string(before) != string(after) {
		t.Fatal("presentation or nil source declaration was rewritten")
	}
}

func TestHMC271RestoredSynthesizedRuntimeUniverseRemainsSupport(t *testing.T) {
	rm := hmc271SourceLocationRequest()
	rm.Intent, rm.Predicates.HasPerMemberTable = IntentEnumerate, true
	rm.SourceScopeProfile = &SourceScopeProfile{RequestedScope: SourceScopeAll}
	rm.SourceInventoryProfile = &SourceInventoryProfile{IsSourceInventory: true, DeclarationOrigin: SourceInventoryDeclarationSynthesized,
		TargetRoles: []AnswerCandidateRole{AnswerCandidateRoleFunction}, RequestedFields: []SourceInventoryRequestedField{SourceInventoryFieldName},
		SourceQuotes: []string{"recorded members"}, Confidence: 1}
	if SourceInventoryPrincipalNavigationActive(rm) || SourceInventoryPrincipalAuthorityActive(rm) || SourceInventoryRequiredFileCoverageShape(rm) {
		t.Fatal("old synthesized runtime profile regained source-universe authority")
	}
	if got := RuntimeSourceRequestCurrentSourceRequirementPrecision(&rm, TurnRouteHint{}); got != RuntimeSourceRequirementPrecise {
		t.Fatalf("restored location obligation was weakened: %s", got)
	}
	rm.RuntimeQuestionProfile = nil
	if !SourceInventoryPrincipalNavigationActive(rm) || !SourceInventoryPrincipalAuthorityActive(rm) ||
		!SourceInventoryCanSynthesizePrincipalProfile(rm, TurnRouteHint{}, false) {
		t.Fatal("ordinary source-only synthesized inventory lost its prior behavior")
	}
}
