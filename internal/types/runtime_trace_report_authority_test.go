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
	boundedCallRelation := callRelation
	boundedCallRelation.RuntimeQuestionProfile = &RuntimeQuestionProfile{
		Scope: RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []RuntimeQuestionFactFamily{
			RuntimeQuestionFactRelationPeer,
			RuntimeQuestionFactTransactionID,
			RuntimeQuestionFactDirectWaker,
		},
	}
	if RuntimeTraceReportMaterializationAllowed(&boundedCallRelation, withRoot) {
		t.Fatal("a finite relation fact set must not widen into the full causal report")
	}
	if RuntimeTracePrincipalValueMaterializationAllowed(&boundedCallRelation, withRoot) {
		t.Fatal("a finite relation fact set must not inherit a target-state principal-value card")
	}
	boundedState := boundedCallRelation
	boundedState.AnalyzerHints.Kind = string(ReqMechanism)
	boundedState.PredicateAxis = AxisUnknown
	boundedState.RuntimeTargets = []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 59566}}
	boundedState.RuntimeQuestionProfile = &RuntimeQuestionProfile{
		Scope:        RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []RuntimeQuestionFactFamily{RuntimeQuestionFactTargetSchedulerState},
	}
	if !RuntimeTracePrincipalValueMaterializationAllowed(&boundedState, withRoot) {
		t.Fatal("a bounded target-state family must retain its exact principal values")
	}
	boundedWait := boundedState
	boundedWait.RuntimeQuestionProfile = &RuntimeQuestionProfile{
		Scope:        RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []RuntimeQuestionFactFamily{RuntimeQuestionFactTargetWaitOccurrences, RuntimeQuestionFactDirectWaker},
	}
	if RuntimeTraceTargetStateMaterializationAllowed(&boundedWait, withRoot) {
		t.Fatal("a target-wait roster must not authorize the scheduler-state partition")
	}
	if !RuntimeTraceTargetWaitMaterializationAllowed(&boundedWait, withRoot) {
		t.Fatal("a target-wait roster must retain only its own principal-value lane")
	}
	boundedCallRelation.RuntimeArtifactScopeProfile = windowed.RuntimeArtifactScopeProfile
	if !RuntimeTraceReportMaterializationAllowed(&boundedCallRelation, TraceCausalProjectionSet{}) {
		t.Fatal("an explicit typed window must still outrank a bounded relation fact set")
	}
	if !RuntimeTracePrincipalValueMaterializationAllowed(&boundedCallRelation, TraceCausalProjectionSet{}) {
		t.Fatal("an explicit typed window must retain target-state principal values inside the full report")
	}

	focusedConditional := *generic
	focusedConditional.RuntimeTargets = []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 59566}}
	focusedConditional.AnalyzerHints.Kind = string(ReqConditional)
	focusedConditional.PredicateAxis = AxisCondition
	if RuntimeTraceReportMaterializationAllowed(&focusedConditional, withRoot) {
		t.Fatal("non-diagnostic explain/conditional target fact must stay narrow even when exploration collected causal rows")
	}

	focusedTraceMechanism := *generic
	focusedTraceMechanism.Intent = IntentTrace
	focusedTraceMechanism.AnalyzerHints.Kind = string(ReqMechanism)
	focusedTraceMechanism.RuntimeTargets = []RuntimeTarget{{Kind: RuntimeTargetKindProcess, PID: 59566}}
	if RuntimeTraceReportMaterializationAllowed(&focusedTraceMechanism, withRoot) {
		t.Fatal("non-diagnostic trace/mechanism target fact must stay narrow even when exploration collected causal rows")
	}
	if !RuntimeTracePrincipalValueMaterializationAllowed(&focusedTraceMechanism, withRoot) {
		t.Fatal("a focused target-state fact must retain its target-state principal values")
	}
	focusedTraceMechanism.Scenario = ScenarioPerformanceBottleneck
	if !RuntimeTraceReportMaterializationAllowed(&focusedTraceMechanism, withRoot) {
		t.Fatal("typed performance-bottleneck scenario must retain full trace report authority")
	}

	// The dedicated v17 breadth declaration outranks unstable legacy labels:
	// the same bounded fact request has appeared as root_cause+diagnostic in
	// real replays. Explicit windows remain stronger positive authority;
	// relation shape is orthogonal and only widens when breadth is not bounded.
	declaredFactSet := rootCause
	declaredFactSet.Scenario = ScenarioRootCause
	declaredFactSet.Predicates.IsDiagnosticQuestion = true
	declaredFactSet.DiagnosticProfile.IsDiagnostic = true
	declaredFactSet.AnalyzerHints.Kind = string(ReqConditional)
	declaredFactSet.PredicateAxis = AxisCondition
	declaredFactSet.RuntimeTargets = []RuntimeTarget{{Kind: RuntimeTargetKindProcess, PID: 59566}}
	declaredFactSet.RuntimeQuestionProfile = &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeBoundedFactSet}
	if RuntimeTraceReportMaterializationAllowed(&declaredFactSet, withRoot) {
		t.Fatal("declared bounded fact set must outrank root-cause/diagnostic label variance")
	}
	declaredFactSet.RuntimeArtifactScopeProfile = windowed.RuntimeArtifactScopeProfile
	if !RuntimeTraceReportMaterializationAllowed(&declaredFactSet, TraceCausalProjectionSet{}) {
		t.Fatal("explicit typed window must still outrank a bounded fact-set declaration")
	}

	missingTargetScalar := *generic
	missingTargetScalar.Intent = IntentReturnValue
	missingTargetScalar.AnalyzerHints.Kind = string(ReqReturnValue)
	missingTargetScalar.PredicateAxis = AxisCondition
	missingTargetScalar.Predicates.IsScalarAnswer = true
	if RuntimeTraceReportMaterializationAllowed(&missingTargetScalar, withRoot) {
		t.Fatal("non-diagnostic scalar runtime fact must not widen into a causal report when its target is missing")
	}
	declaredCausal := missingTargetScalar
	declaredCausal.RuntimeQuestionProfile = &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeCausalDiagnosis}
	if !RuntimeTraceReportMaterializationAllowed(&declaredCausal, TraceCausalProjectionSet{}) {
		t.Fatal("declared causal diagnosis must retain a full-report authority boundary despite scalar label noise")
	}
	missingTargetScalar.RuntimeArtifactScopeProfile = windowed.RuntimeArtifactScopeProfile
	if !RuntimeTraceReportMaterializationAllowed(&missingTargetScalar, TraceCausalProjectionSet{}) {
		t.Fatal("an explicit typed user window must outrank the scalar narrow-fact rule")
	}
}
