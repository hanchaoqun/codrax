package types

// RuntimeSourceLocationOnly identifies the narrow finite-observation request
// whose independent source responsibility is locating current code, not proving
// producer/transfer/consumer operations. Source-location precision, required
// reads and answer obligations remain intact. Missing source declarations stay
// unknown; they are never rewritten to false.
//
// This is a derived view over existing typed carriers, not a new model field.
// An explicit source mechanism, relation, selection, impact or value request
// keeps its existing operation rules. A generic AxisFlow or per-member table
// cannot by itself widen a location attribute into such a request.
func RuntimeSourceLocationOnly(rm RequestModel) bool {
	if rm.RuntimeQuestionProfile == nil || !rm.RuntimeQuestionProfile.BoundedFactSet() ||
		!rm.HasRequiredSourceLocationDimension() {
		return false
	}
	if profile := rm.CurrentSourceExplanationProfile; profile != nil && profile.IsCurrentSourceExplanationRequested {
		if !profile.Active() || len(profile.Modes) == 0 {
			return false
		}
		for _, mode := range profile.Modes {
			if mode != CurrentSourceExplanationLocateCurrentCode {
				return false
			}
		}
	}
	if SourceInventoryHasDeclaredNavigationRequest(rm) ||
		rm.ChangeImpactProfile.Active() || rm.FieldValueProfile.Active() ||
		rm.CallChainEndpointProfile.Active() || rm.CallChainEndpointProfile.RequiresRuntimeSelectionEvidence() {
		return false
	}
	if diagram := rm.DiagramHint; diagram != nil && diagram.Required {
		for _, participant := range diagram.Participants {
			if participant.Role == DiagramParticipantIncidentRequired {
				// There is an independently required relationship surface, not
				// only a location attribute. Without a narrower typed domain for
				// that relation, preserve its existing operation contract.
				return false
			}
		}
	}
	switch NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case ReqCallChain, ReqRegistration, ReqConfigMapping:
		return false
	}
	for _, signal := range rm.CurrentSourceObligationSignals {
		if signal.Active() && signal.Role != RequestedAnswerDimensionSourceLocation {
			return false
		}
	}
	for _, dimension := range rm.RequestedAnswerDimensions.Dimensions {
		if !dimension.Required {
			continue
		}
		switch dimension.Role {
		case RequestedAnswerDimensionCurrentKeyCode, RequestedAnswerDimensionFunctionOrPurpose,
			RequestedAnswerDimensionBranchBehavior, RequestedAnswerDimensionRelationPath:
			return false
		}
	}
	return true
}
