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
	if rm.RuntimeQuestionProfile != nil {
		if rm.RuntimeQuestionProfile.BoundedFactSet() {
			return true, false
		}
		if rm.RuntimeQuestionProfile.RequiresFullReport() {
			return true, true
		}
	}
	if NormalizeRequirementKind(rm.AnalyzerHints.Kind) == ReqCallChain ||
		rm.PredicateAxis == AxisCall || rm.Predicates.IsRelationalLookup {
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

// runtimeTraceBoundedFactFamilyMaterializationAllowed is the shared primitive
// for independently authorized bounded principal-value families. A full
// causal report may include them all. A narrow report may include only the
// explicitly requested typed family; bounded breadth by itself is
// insufficient because finite IPC peers, transaction IDs, direct wakers,
// timestamps, and other facts do not ask for a scheduler-state card.
//
// The decision consumes only the validated RequestModel and compiled typed
// projection. It never inspects request or answer prose.
func runtimeTraceBoundedFactFamilyMaterializationAllowed(
	rm *RequestModel,
	set TraceCausalProjectionSet,
	families ...RuntimeQuestionFactFamily,
) bool {
	if RuntimeTraceReportMaterializationAllowed(rm, set) {
		return true
	}
	if rm != nil && rm.RuntimeQuestionProfile != nil && rm.RuntimeQuestionProfile.BoundedFactSet() && len(rm.RuntimeQuestionProfile.FactFamilies) > 0 {
		for _, requested := range rm.RuntimeQuestionProfile.FactFamilies {
			for _, allowed := range families {
				if requested == allowed {
					return true
				}
			}
		}
		return false
	}
	// Compatibility for old serialized RequestModels and focused synthetic
	// fixtures created before fact_families existed. New analyzer emissions
	// must declare at least one family for bounded_fact_set, so production
	// requests do not enter this coarse legacy arm.
	return rm != nil && IsFocusedRuntimeFactQuestion(*rm)
}

// RuntimeTraceTargetStateMaterializationAllowed authorizes only the target's
// scheduler-state partition. A requested wait-occurrence roster does not
// imply that the user asked for running/runnable/sleep/D/io_wait totals.
func RuntimeTraceTargetStateMaterializationAllowed(rm *RequestModel, set TraceCausalProjectionSet) bool {
	return runtimeTraceBoundedFactFamilyMaterializationAllowed(
		rm,
		set,
		RuntimeQuestionFactTargetSchedulerState,
	)
}

// RuntimeTraceTargetWaitMaterializationAllowed authorizes only the exact
// target-wait occurrence roster. It is independent of the state partition.
func RuntimeTraceTargetWaitMaterializationAllowed(rm *RequestModel, set TraceCausalProjectionSet) bool {
	return runtimeTraceBoundedFactFamilyMaterializationAllowed(
		rm,
		set,
		RuntimeQuestionFactTargetWaitOccurrences,
	)
}

// RuntimeTracePrincipalValueMaterializationAllowed is the compatibility union
// for consumers that render both principal lanes. New consumers should prefer
// the state/wait-specific predicates and filter their rows independently.
func RuntimeTracePrincipalValueMaterializationAllowed(rm *RequestModel, set TraceCausalProjectionSet) bool {
	return RuntimeTraceTargetStateMaterializationAllowed(rm, set) ||
		RuntimeTraceTargetWaitMaterializationAllowed(rm, set)
}
