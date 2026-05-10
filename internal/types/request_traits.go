package types

import "strings"

// HasNonEmptyAmbiguity reports whether the request carries at least one
// analyzer-emitted ambiguity clause with real content. Shared by
// scenario reconcile and compiler template selection so both stages do
// not drift on what counts as "this question still needs branch
// collapse/reconciliation".
func HasNonEmptyAmbiguity(rm RequestModel) bool {
	for _, a := range rm.Ambiguities {
		if strings.TrimSpace(a.Clause) != "" {
			return true
		}
	}
	return false
}

// IsScalarSourceLiteralLookup reports whether the request resolves to
// one named source-code literal rather than a mechanism walkthrough or
// a set-valued answer. Shared by analyzer reconcile, prompt shaping,
// and answer-surface compilation so those stages do not re-derive the
// same scalar-lookup policy independently.
func IsScalarSourceLiteralLookup(rm RequestModel) bool {
	if len(rm.SubTopics) > 1 {
		return false
	}
	if rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsCrossComponent ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	if !isScalarSourceLiteralSubjectKind(rm.AnswerSubject.Kind) {
		if !(rm.Predicates.IsRoleLocateLookup && rm.AnswerSubject.Kind == SubjectConfigKey) {
			return false
		}
	}
	// 2026-05-02 — the LLM's role-locate signal is a "where is the
	// thing that plays this role" judgment, locally plausible for any
	// "X 来自哪里" wording. But when the user attached a multi-frame
	// log/perf artifact, that artifact is OBJECTIVE evidence the
	// answer surface is a multi-step mechanism, not a single source
	// literal. In that case the role-locate short-circuit is
	// contradicted by user-supplied artifact data and must yield to
	// the regular IsScalarAnswer / structural-fallback path below.
	// Pre-fix the short-circuit fired unconditionally and routed
	// panic / OOM / perf-trace root-cause requests into the scalar
	// lane, breaking the answer contract downstream.
	if rm.Predicates.IsRoleLocateLookup && !hasMultiFrameArtifactEvidence(rm) {
		return true
	}
	if rm.Predicates.IsScalarAnswer {
		return true
	}
	// Fallback for role-locate lookups: the analyzer sometimes
	// correctly identifies the subject kind (function / type / route /
	// file path) but still emits a prose/list carrier. For a single,
	// simple, non-relational request over one source literal, keep the
	// answer in the scalar lane even when is_scalar_answer=false.
	//
	// 2026-05-02 — same multi-frame artifact guard as the explicit
	// short-circuit above: when an attached log/perf bundle resolves
	// 2+ frames, the request is by definition NOT "over one source
	// literal" and must not enter the unnamed-fallback scalar lane.
	if hasMultiFrameArtifactEvidence(rm) {
		return false
	}
	if rm.Complexity != ComplexitySimple {
		return false
	}
	// Only activate this fallback when the user did NOT already name
	// a concrete source literal in the request. If there are
	// analyzer-detected entities / primary entities, the question is
	// more likely "explain Foo" than "locate the thing that plays this
	// role", and the strict scalar_answer=true path should decide.
	if len(rm.AnalyzerHints.PrimaryEntities) > 0 || len(rm.AnalyzerHints.Entities) > 0 {
		return false
	}
	switch rm.Intent {
	case IntentExplain, IntentUnknown, IntentReturnValue:
	default:
		return false
	}
	switch NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case ReqMechanism, ReqRegistration, ReqUnknown:
		return true
	default:
		return false
	}
}

// hasMultiFrameArtifactEvidence reports whether the request arrived
// bundled with an external artifact (attached log / htrace / atrace)
// that resolved 2+ frames or stalls. Such artifacts are objective
// proof that the answer surface is a multi-step mechanism rather
// than a single source literal, and therefore should temper the
// IsRoleLocateLookup scalar short-circuit in
// IsScalarSourceLiteralLookup. Returns false when no bundle is
// attached (preserving pre-2026-05-02 behaviour for plain text-only
// questions).
//
// Threshold rationale: 2 frames / janks / stalls is the same lower
// bound logBundleAuthoritativeFrames + renderLogCallChain already
// use to distinguish "real call chain" from "single sample" — keeps
// the multi-frame definition consistent across the codebase.
//
// Read-only on RequestModel; safe to call with a zero-value rm.
func hasMultiFrameArtifactEvidence(rm RequestModel) bool {
	if rm.LogTriage != nil {
		for _, e := range rm.LogTriage.Errors {
			if len(e.Frames) >= 2 {
				return true
			}
		}
	}
	if rm.PerfTrace != nil {
		if len(rm.PerfTrace.Frames) >= 2 ||
			len(rm.PerfTrace.Janks) >= 2 ||
			len(rm.PerfTrace.Stalls) >= 2 {
			return true
		}
	}
	return false
}

func isScalarSourceLiteralSubjectKind(kind AnswerSubjectKind) bool {
	switch kind {
	case SubjectFunctionName,
		SubjectTypeName,
		SubjectInterface,
		SubjectHandlerRoute,
		SubjectFilePath,
		SubjectStringLiteral,
		SubjectEnumValue,
		SubjectStructField:
		return true
	}
	return false
}

// IsScalarRoleLocateLookup is the narrower subset of scalar source-
// literal lookups where the user describes a role/clue ("the entry
// function that ...", "the file that ...") and wants the located
// literal itself. These answers should keep summary surface tight and
// avoid expanding into surrounding mechanism prose unless the user
// explicitly asked for it.
func IsScalarRoleLocateLookup(rm RequestModel) bool {
	if !IsScalarSourceLiteralLookup(rm) {
		return false
	}
	if rm.Predicates.IsRoleLocateLookup {
		return true
	}
	if rm.AnswerSubject.Kind == SubjectReturnValue {
		return false
	}
	if !rm.Predicates.IsScalarAnswer {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(rm.AnalyzerHints.Kind), "return_value") {
		return true
	}
	return rm.PredicateAxis == AxisReturn
}

// IsProjectOrientationQuestion reports whether the request is a
// project-orientation ask ("what does this repo do?", "summarise the
// project", "give me a tour"). Detection is structured-signal-only:
// no substring matching on RawRequest, language-neutral, driven
// entirely by the analyzer LLM's existing classification.
//
// Conditions (all must hold):
//
//   - intent: explain or unknown (root_cause / trace / etc. always
//     fall through to deep investigation)
//   - complexity: simple — the analyzer's own assessment that scope
//     is single-entity / 1-2 files
//   - predicates: is_cross_component / is_count_question /
//     is_history_lookup / is_diagnostic_question / is_scalar_answer ALL false
//   - len(PrimaryEntities) == 0 — the user didn't pin to specific code
//   - len(Entities) == 0 — no identifier-shaped tokens in the request
//
// Shared by:
//
//   - internal/tool/emit_investigation_complete.go:
//     applyMultiPathAnchorChecks (the multi-path symbol-anchored
//     gate) skips orientation questions because they don't need
//     cross-component depth.
//   - internal/analysis/budget/budget.go: tightens the EvidenceBudget
//     base so the explorer's existing MaxFiles / MaxReactIters caps
//     enforce a smaller ceiling — README + manifest + entry-point
//     answer needs ~5 files, not the moderate-default 30.
//
// Returns false on a zero-value RequestModel — callers default to
// "fire the gate / use full budget" which preserves pre-2026-04-29
// behaviour for any path that didn't run the analyzer.
func IsProjectOrientationQuestion(rm RequestModel) bool {
	switch rm.Intent {
	case IntentExplain, IntentUnknown:
		// continue
	default:
		return false
	}
	if rm.Complexity != ComplexitySimple {
		return false
	}
	if rm.Predicates.IsCrossComponent ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion ||
		rm.Predicates.IsScalarAnswer {
		return false
	}
	if len(rm.AnalyzerHints.PrimaryEntities) > 0 {
		return false
	}
	if len(rm.AnalyzerHints.Entities) > 0 {
		return false
	}
	return true
}

// IsSingleTopicStructuralTrace reports whether the request is a
// single-topic structural walkthrough (call-chain / flow / dispatch)
// that benefits from a lighter trace-oriented DAG rather than the
// heavier architecture-explain template with a dedicated reconcile
// window.
//
// Important distinction: a single ordered trace may legitimately cross
// files / packages / components without becoming a multi-topic
// architecture survey. What disqualifies the lighter lane is not
// "crossing modules" by itself, but structurally independent topics
// (multiple sub-topics), ambiguity that still needs reconciliation, or
// set-style / relational asks that are not one source-to-sink chain.
//
// The signals stay typed and language-neutral: trace intent +
// structural axis / question kind, while explicitly excluding
// multi-topic, ambiguity-bearing, relational, enumerative, and
// history-style questions that genuinely need broader orchestration.
func IsSingleTopicStructuralTrace(rm RequestModel) bool {
	if rm.Intent != IntentTrace {
		return false
	}
	if len(rm.SubTopics) > 1 || HasNonEmptyAmbiguity(rm) {
		return false
	}
	if rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	switch rm.PredicateAxis {
	case AxisCall, AxisCondition, AxisRegister:
		return true
	}
	switch NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case ReqCallChain, ReqConditional, ReqMechanism, ReqRegistration:
		return true
	}
	return false
}
