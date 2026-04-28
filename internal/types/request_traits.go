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
	if !rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsCrossComponent ||
		rm.Predicates.IsRelationalLookup {
		return false
	}
	switch rm.AnswerSubject.Kind {
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
	if rm.AnswerSubject.Kind == SubjectReturnValue {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(rm.AnalyzerHints.Kind), "return_value") {
		return true
	}
	return rm.PredicateAxis == AxisReturn
}

// IsSingleTopicStructuralTrace reports whether the request is a
// single-topic structural walkthrough (call-chain / flow / dispatch)
// that benefits from a lighter trace-oriented DAG rather than the
// heavier architecture-explain template with a dedicated reconcile
// window. The signals are typed and language-neutral: trace intent +
// structural axis / question kind, while explicitly excluding
// multi-topic, cross-component, and ambiguity-bearing questions that
// genuinely need reconciliation.
func IsSingleTopicStructuralTrace(rm RequestModel) bool {
	if rm.Intent != IntentTrace {
		return false
	}
	if len(rm.SubTopics) > 1 || rm.Predicates.IsCrossComponent || HasNonEmptyAmbiguity(rm) {
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
