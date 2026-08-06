package types

func sourceInventoryCompletionSupportBoundary(rm RequestModel) bool {
	return SourceInventoryProfileCompletionIsSupportOnly(rm.SourceInventoryProfile) ||
		SourceInventoryProfileConflictsWithRoleBinding(rm) ||
		SourceInventoryLaneConflictsWithArchitectureNarrative(rm) ||
		sourceInventoryFiniteExactScalarLookupIsSupportOnly(rm)
}

// sourceInventoryFiniteExactScalarLookupIsSupportOnly keeps an incidental
// source-inventory lens from expanding a finite exact-target value comparison
// into an exhaustive repository declaration inventory. The exact targets and
// scalar answer-subject kind are typed analyzer outputs validated against the
// current request; no request wording or model answer prose participates.
//
// Explicit count/member-set/completeness and structural const/type facets stay
// principal. Those shapes really do ask source_inventory to own a set rather
// than merely help locate the implementation behind a bounded value answer.
func sourceInventoryFiniteExactScalarLookupIsSupportOnly(rm RequestModel) bool {
	profile := rm.SourceInventoryProfile
	if profile == nil || !profile.Active() || len(rm.AnalyzerHints.ExactTargets) == 0 {
		return false
	}
	if !isScalarSourceLiteralSubjectKind(rm.AnswerSubject.Kind) ||
		rm.Predicates.IsCountQuestion ||
		rm.CompletenessObligation.IsActive() ||
		profile.RequiresConstSet ||
		(profile.TypeUnderlying != "" && profile.TypeUnderlying != SourceInventoryTypeUnderlyingUnknown) {
		return false
	}
	if rm.RequestedAnswerDimensions != nil {
		for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
			if !dim.Required {
				continue
			}
			switch dim.Role {
			case RequestedAnswerDimensionCount, RequestedAnswerDimensionMemberSet:
				return false
			}
		}
	}
	return true
}
