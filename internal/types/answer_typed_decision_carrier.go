package types

// AnswerBlockHasActiveTypedDecisionCarrier reports whether a decision block
// already carries the typed verdict field for an active decision lane in the
// semantic view. This is intentionally a carrier-level predicate: it reads only
// schema-validated enum fields and the typed view, never rendered prose.
func AnswerBlockHasActiveTypedDecisionCarrier(block AnswerBlock, view *AnswerSemanticView) bool {
	if view == nil || block.Kind != BlockDecision {
		return false
	}
	if view.CurrentStatusDiagnostic != nil &&
		view.CurrentStatusDiagnostic.Required &&
		CurrentStatusVerdictAllowed(block.CurrentStatusVerdict, view.CurrentStatusDiagnostic) {
		return true
	}
	if view.ErrorGranularityProfile != nil &&
		view.ErrorGranularityProfile.Active() &&
		block.ErrorGranularityVerdict != ErrorGranularityUnknown &&
		block.ErrorGranularityVerdict.IsValid() {
		return true
	}
	return false
}
