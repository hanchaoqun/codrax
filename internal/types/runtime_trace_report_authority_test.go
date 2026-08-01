package types

import "testing"

func TestRuntimeTraceReportMaterializationAuthorityMatrix(t *testing.T) {
	generic := &RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioGeneric,
		Predicates: SemanticPredicates{
			IsCrossComponent: true,
		},
	}
	if RuntimeTraceReportMaterializationAllowed(generic, TraceCausalProjectionSet{}) {
		t.Fatal("generic comparison without causal rows must not publish the full trace report")
	}

	semanticOnly := TraceCausalProjectionSet{Projections: []TraceCausalProjection{{
		SemanticSpans: []TraceCausalProjectionNode{{
			Subject:        "background compilation",
			ChainRelevance: "background",
		}},
	}}}
	if RuntimeTraceReportMaterializationAllowed(generic, semanticOnly) {
		t.Fatal("standalone background semantic rows must not mint full trace report authority")
	}

	withRoot := TraceCausalProjectionSet{Projections: []TraceCausalProjection{{
		PrimaryRootCause: &TraceCausalProjectionNode{Subject: "ui-thread"},
	}}}
	if !RuntimeTraceReportMaterializationAllowed(generic, withRoot) {
		t.Fatal("publication-grade root row must retain the full trace report")
	}

	start, end := 20.0, 20.1
	windowed := *generic
	windowed.RuntimeArtifactScopeProfile = &RuntimeArtifactScopeProfile{
		RequestedScope: RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start,
		TimeEnd:        &end,
		SourceQuote:    "20.0..20.1",
	}
	if !RuntimeTraceReportMaterializationAllowed(&windowed, TraceCausalProjectionSet{}) {
		t.Fatal("explicit typed window must retain full trace report authority")
	}

	rootCause := *generic
	rootCause.Intent = IntentRootCause
	if !RuntimeTraceReportMaterializationAllowed(&rootCause, TraceCausalProjectionSet{}) {
		t.Fatal("typed root-cause request must retain an empty causal authority boundary")
	}

	callRelation := *generic
	callRelation.AnalyzerHints.Kind = string(ReqCallChain)
	callRelation.PredicateAxis = AxisCall
	if !RuntimeTraceReportMaterializationAllowed(&callRelation, TraceCausalProjectionSet{}) {
		t.Fatal("typed call relation must retain full trace report authority regardless of broad intent label")
	}

	focusedConditional := *generic
	focusedConditional.RuntimeTargets = []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 59566}}
	focusedConditional.AnalyzerHints.Kind = string(ReqConditional)
	focusedConditional.PredicateAxis = AxisCondition
	if RuntimeTraceReportMaterializationAllowed(&focusedConditional, withRoot) {
		t.Fatal("non-diagnostic explain/conditional target fact must stay narrow even when exploration collected causal rows")
	}

	missingTargetScalar := *generic
	missingTargetScalar.Intent = IntentReturnValue
	missingTargetScalar.AnalyzerHints.Kind = string(ReqReturnValue)
	missingTargetScalar.PredicateAxis = AxisCondition
	missingTargetScalar.Predicates.IsScalarAnswer = true
	if RuntimeTraceReportMaterializationAllowed(&missingTargetScalar, withRoot) {
		t.Fatal("non-diagnostic scalar runtime fact must not widen into a causal report when its target is missing")
	}
	missingTargetScalar.RuntimeArtifactScopeProfile = windowed.RuntimeArtifactScopeProfile
	if !RuntimeTraceReportMaterializationAllowed(&missingTargetScalar, TraceCausalProjectionSet{}) {
		t.Fatal("an explicit typed user window must outrank the scalar narrow-fact rule")
	}
}
