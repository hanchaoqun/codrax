package types

// RuntimeTraceReportShapeAuthority resolves the typed question shapes that
// inherently request a full system-authored trace report. A nil request model
// preserves legacy/synthetic render paths.
//
// Question breadth and artifact range are orthogonal. A validated
// RuntimeQuestionProfile is the higher-fidelity answer-shape authority: a
// bounded fact set stays bounded even when the user also supplies exact time
// endpoints, while causal/relation/overview requests retain the full report.
// The explicit-window fallback remains for legacy serialized RequestModels
// that predate that profile so noisy historical intent/scenario labels do not
// suppress causal projection.
// FrameCausalityQualifierApplicable (QUALGATE-1, §40.30 V-QUAL-1 plan A) is
// the ONE typed gate behind every frame-causality qualifier surface: the
// seat-level provider (tracefinding.BuildSeatFrameCausalityAuthority feeding
// the Markdown crown headline and the .root-causes.json causal_qualifier)
// and the system coverage-boundary sentence. It reads only the analyzer's
// typed RuntimeQuestionProfile.FrameCausalityRequested decision — never
// request text, keywords, scenario labels, or artifact contents. Absent
// profile ⇒ false (fail closed: no frame claim without a typed frame
// question).
func FrameCausalityQualifierApplicable(rm *RequestModel) bool {
	return rm != nil && rm.RuntimeQuestionProfile.RequestsFrameCausality()
}

// TraceAuthorityCausalUnprovenIsFrameOrigin reports whether a trace_query
// result's "unproven" causal conclusion rests ONLY on the frame family — typed
// causal rows exist but the frame evidence is absent/unavailable or the typed
// frame flow withholds causality. Under a closed frame-question gate
// (FrameCausalityQualifierApplicable == false) such a verdict must not be read
// as "no usable on-chain causal observation": the on-chain rows are there,
// only the frame claim is out of scope. A zero-row causal view stays a
// genuine causal-ceiling signal.
func TraceAuthorityCausalUnprovenIsFrameOrigin(a *TraceEvidenceAuthority) bool {
	if a == nil || a.CausalConclusion != "unproven" || a.TypedCausalRowCount <= 0 {
		return false
	}
	switch a.FrameEvidenceStatus {
	case "absent", "unavailable":
		return true
	}
	return a.FrameFlowCausalConclusion == "unproven"
}

func RuntimeTraceReportShapeAuthority(rm *RequestModel) (decided bool, allowed bool) {
	if rm == nil {
		return true, true
	}
	if rm.RuntimeQuestionProfile != nil {
		if rm.RuntimeQuestionProfile.CarriesBoundedFactFamilies() {
			return true, false
		}
		if rm.RuntimeQuestionProfile.RequiresFullReport() {
			return true, true
		}
	}
	if _, _, ok := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindow(); ok {
		return true, true
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
// explicitly requested typed family; finite breadth by itself is
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
	if rm != nil && rm.RuntimeQuestionProfile != nil && rm.RuntimeQuestionProfile.CarriesBoundedFactFamilies() && len(rm.RuntimeQuestionProfile.FactFamilies) > 0 {
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
	// must declare at least one family for either finite scope, so production
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
	if rm != nil && rm.RuntimeQuestionProfile.RequestsTargetWaitOccurrences() {
		return true
	}
	return runtimeTraceBoundedFactFamilyMaterializationAllowed(
		rm,
		set,
		RuntimeQuestionFactTargetWaitOccurrences,
	)
}

// RuntimeTraceWakeupEdgeMaterializationAllowed authorizes the exact endpoint
// role capsule for causal reports and bounded direct-waker lookups. It does
// not widen a finite scheduler-state/wait/count question into a wakeup report.
func RuntimeTraceWakeupEdgeMaterializationAllowed(rm *RequestModel, set TraceCausalProjectionSet) bool {
	return runtimeTraceBoundedFactFamilyMaterializationAllowed(
		rm,
		set,
		RuntimeQuestionFactDirectWaker,
		RuntimeQuestionFactRelationPeer,
	)
}

// RuntimeTraceBlockedReasonCensusMaterializationAllowed authorizes the exact
// record inventory either as part of a full trace report or as the two
// explicitly requested bounded axes (recorded reason + count/duration).
func RuntimeTraceBlockedReasonCensusMaterializationAllowed(rm *RequestModel, set TraceCausalProjectionSet) bool {
	return RuntimeTraceReportMaterializationAllowed(rm, set) ||
		(rm != nil && rm.RuntimeQuestionProfile.RequestsBlockedReasonCensus())
}

// RuntimeTraceIOLatencyMaterializationAllowed authorizes the finite storage
// latency-caliber lane without widening the answer into a root-cause report.
// Consumers must keep request residence, completion-closed issuer blocking,
// and aggregate pressure as distinct typed rulers.
func RuntimeTraceIOLatencyMaterializationAllowed(rm *RequestModel, set TraceCausalProjectionSet) bool {
	return runtimeTraceBoundedFactFamilyMaterializationAllowed(
		rm,
		set,
		RuntimeQuestionFactIOLatency,
	)
}

// RuntimeTracePrincipalValueMaterializationAllowed is the compatibility union
// for consumers that render both principal lanes. New consumers should prefer
// the state/wait-specific predicates and filter their rows independently.
func RuntimeTracePrincipalValueMaterializationAllowed(rm *RequestModel, set TraceCausalProjectionSet) bool {
	return RuntimeTraceTargetStateMaterializationAllowed(rm, set) ||
		RuntimeTraceTargetWaitMaterializationAllowed(rm, set)
}
