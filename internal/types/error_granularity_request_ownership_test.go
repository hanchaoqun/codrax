package types

import (
	"reflect"
	"testing"
)

func TestErrorGranularityRequestOwnership(t *testing.T) {
	for _, role := range []RequestedAnswerDimensionRole{RequestedAnswerDimensionMemberSet, RequestedAnswerDimensionFunctionOrPurpose, RequestedAnswerDimensionObservedValue, RequestedAnswerDimensionCausalAttribution, RequestedAnswerDimensionRelationPath} {
		t.Run(string(role), func(t *testing.T) {
			rm := granularityOwnedQuoteRequest(role)
			before := *rm.ErrorGranularityProfile
			if ShouldCarryErrorGranularityHardContract(rm) || IsFailureScopeDecisionAnswer(rm) {
				t.Fatal("duplicate event/actor/relation request gained an independent failure-scope duty")
			}
			if !reflect.DeepEqual(before, *rm.ErrorGranularityProfile) {
				t.Fatal("softening mutated the original classifier audit")
			}
			if got := BuildAnswerSemanticView(&AnalysisIR{RequestModel: rm}, nil); got.ErrorGranularityProfile != nil {
				t.Fatal("semantic view resurrected softened profile")
			}
		})
	}
}

func TestErrorGranularityRequestOwnershipNarrowBoundary(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*RequestModel)
	}{
		{"independent_quote", func(rm *RequestModel) {
			rm.ErrorGranularityProfile.SourceQuotes = append(rm.ErrorGranularityProfile.SourceQuotes, "does one bad record roll back the transaction")
		}},
		{"broader_quote_remains_model_owned", func(rm *RequestModel) { rm.ErrorGranularityProfile.SourceQuotes[0] += " and operation outcome" }},
		{"optional_dimension", func(rm *RequestModel) { rm.RequestedAnswerDimensions.Dimensions[0].Required = false }},
		{"unanchored_dimension", func(rm *RequestModel) { rm.RequestedAnswerDimensions.Dimensions[0].SourceQuote = "" }},
		{"unknown_role", func(rm *RequestModel) {
			rm.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionOther
		}},
		{"branch_role", func(rm *RequestModel) {
			rm.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionBranchBehavior
		}},
		{"effect_verdict_role", func(rm *RequestModel) {
			rm.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionTargetEffectVerdict
		}},
		{"ambiguous_shared_quote", func(rm *RequestModel) {
			dim := rm.RequestedAnswerDimensions.Dimensions[0]
			dim.Role = RequestedAnswerDimensionBranchBehavior
			rm.RequestedAnswerDimensions.Dimensions = append(rm.RequestedAnswerDimensions.Dimensions, dim)
		}},
		{"return_value_intent", func(rm *RequestModel) { rm.Intent = IntentReturnValue }},
		{"return_value_kind", func(rm *RequestModel) { rm.AnalyzerHints.Kind = string(ReqReturnValue) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rm := granularityOwnedQuoteRequest(RequestedAnswerDimensionMemberSet)
			tc.edit(&rm)
			if ErrorGranularityQuotesBelongToOtherDimensions(rm) || !ShouldCarryErrorGranularityHardContract(rm) {
				t.Fatal("narrow duplicate-quote softening was broadened into semantic inference")
			}
		})
	}
	for _, runtime := range []bool{false, true} {
		rm := granularityOwnedQuoteRequest(RequestedAnswerDimensionCausalAttribution)
		rm.Intent, rm.Scenario, rm.Predicates.IsDiagnosticQuestion = IntentRootCause, ScenarioRootCause, true
		rm.ErrorGranularityProfile.SourceQuotes = append(rm.ErrorGranularityProfile.SourceQuotes, "does one bad record roll back the transaction")
		if runtime {
			rm.RuntimeQuestionProfile = &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeCausalDiagnosis}
		}
		if !ShouldCarryErrorGranularityHardContract(rm) || ErrorGranularityConflictsWithDiagnosticMechanism(rm) {
			t.Fatalf("independent cause-plus-failure-scope question dropped; runtime=%t", runtime)
		}
		if IsFailureScopeDecisionAnswer(rm) || ResolveQuestionFamily(rm) != QFRootCauseTrace {
			t.Fatalf("mixed scope replaced the independently required causal family; runtime=%t", runtime)
		}
	}
}

func TestErrorGranularitySinglePropositionAllowsNegative(t *testing.T) {
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{{Kind: BlockDecision, SurfaceRole: SurfacePrincipal, ErrorGranularityVerdict: ErrorGranularityWholeBatch}}}
	for _, options := range [][]ErrorGranularityVerdict{nil, {ErrorGranularityPerItemRejection}, {ErrorGranularityPerItemRejection, ErrorGranularityPerItemRejection}} {
		profile := &ErrorGranularityProfile{IsGranularityQuestion: true, RequestedVerdictOptions: options}
		if profile.HasRequestedContrast() {
			t.Fatalf("zero/one distinct proposition became a closed contrast: %v", options)
		}
		if _, mismatch := ErrorGranularityVerdictOptionMismatch(doc, profile); mismatch {
			t.Fatalf("supported alternative disallowed: %v", options)
		}
	}
}

func granularityOwnedQuoteRequest(role RequestedAnswerDimensionRole) RequestModel {
	return RequestModel{
		Intent: IntentExplain, AnalyzerHints: AnalyzerHints{Kind: string(ReqMechanism)},
		ErrorGranularityProfile:   &ErrorGranularityProfile{IsGranularityQuestion: true, SourceQuotes: []string{"show the recorded events"}, RequestedVerdictOptions: []ErrorGranularityVerdict{ErrorGranularityPerItemRejection}},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{{Label: "events", Role: role, SourceQuote: "show the recorded events", Required: true}}},
	}
}
