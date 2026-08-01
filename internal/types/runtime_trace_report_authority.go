package types

// RuntimeTraceReportShapeAuthority resolves the typed question shapes that
// inherently request a full system-authored trace report. A nil request model
// preserves legacy/synthetic render paths.
//
// Explicit typed windows have the highest authority so causal projection,
// root ranking, wakeup chains, removable work, and automatic supplementation
// remain available regardless of a noisy analyzer scenario label.
func RuntimeTraceReportShapeAuthority(rm *RequestModel) (decided bool, allowed bool) {
	if rm == nil {
		return true, true
	}
	if _, _, ok := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindow(); ok {
		return true, true
	}
	if NormalizeRequirementKind(rm.AnalyzerHints.Kind) == ReqCallChain ||
		rm.PredicateAxis == AxisCall || rm.Predicates.IsRelationalLookup {
		return true, true
	}
	if rm.RuntimeQuestionProfile != nil && rm.RuntimeQuestionProfile.RequiresFullReport() {
		return true, true
	}
	if IsNarrowRuntimeArtifactFactShape(*rm) {
		return true, false
	}
	if rm.Intent == IntentRootCause ||
		rm.Predicates.IsDiagnosticQuestion ||
		rm.DiagnosticProfile.RequiresDiagnosticRootCause() ||
		rm.Scenario == ScenarioPerformanceBottleneck {
		return true, true
	}
	switch ResolveQuestionFamily(*rm) {
	case QFRootCauseTrace, QFCallChain:
		return true, true
	default:
		return false, false
	}
}

// TraceCausalProjectionSetHasPublicationGradeRows is deliberately narrower
// than "has any renderable context". Standalone/off-chain SemanticSpans are
// useful background observations, but cannot by themselves mint a root-cause
// report for a generic artifact coverage/comparison request. A semantic span
// that is actually on-chain is already represented in OnChainCauses.
func TraceCausalProjectionSetHasPublicationGradeRows(set TraceCausalProjectionSet) bool {
	for _, projection := range set.Projections {
		if projection.PrimaryRootCause != nil ||
			len(projection.PrimaryRootCauses) > 0 ||
			len(projection.OnChainCauses) > 0 ||
			len(projection.AdjacentCauses) > 0 ||
			len(projection.WakeupPath) > 0 ||
			len(projection.SupportingHops) > 0 {
			return true
		}
	}
	return false
}

// RuntimeTraceReportMaterializationAllowed is the shared publication authority
// for structured AnswerDocument blocks and last-mile trace observation
// supplements. It consumes only the validated request model and compiled typed
// projection set; it never scans the user request or model-authored answer.
func RuntimeTraceReportMaterializationAllowed(rm *RequestModel, set TraceCausalProjectionSet) bool {
	if decided, allowed := RuntimeTraceReportShapeAuthority(rm); decided {
		return allowed
	}
	return TraceCausalProjectionSetHasPublicationGradeRows(set)
}
