package types

import "strings"

// ExactResolutionTargetIsConfigKey reports whether the exact-resolution
// contract is about a concrete config key.
func ExactResolutionTargetIsConfigKey(c *ExactResolutionContract) bool {
	if c == nil {
		return false
	}
	if c.TargetKind == SubjectConfigKey {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(c.TargetLabel), "config key")
}

// StableAbsentExactConfigRequiresExplanation reports whether the
// downstream answer shape must degrade from config_value to
// explanation because the exact config key was stably proven absent.
func StableAbsentExactConfigRequiresExplanation(ir *AnalysisIR, mutable *MutableState) bool {
	if ir == nil || mutable == nil {
		return false
	}
	if ir.AnswerContract.RequiredAnswerShape != ShapeConfigValue {
		return false
	}
	contract := ir.AnswerContract.ExactResolution
	if contract == nil || !contract.AllowAbsence || !ExactResolutionTargetIsConfigKey(contract) {
		return false
	}
	if strings.TrimSpace(mutable.StableInvestigationResultKind()) != "absence" {
		return false
	}
	return strings.TrimSpace(mutable.StableAbsenceJustification()) != ""
}

// EffectiveRequiredAnswerShape returns the actual answer shape that
// downstream stages should target after factoring in stable terminal
// investigation state.
func EffectiveRequiredAnswerShape(ir *AnalysisIR, mutable *MutableState) AnswerShape {
	if ir == nil {
		return ShapeNone
	}
	shape := ir.AnswerContract.RequiredAnswerShape
	if shape == "" || shape == ShapeNone {
		return ShapeNone
	}
	if StableAbsentExactConfigRequiresExplanation(ir, mutable) {
		return ShapeExplanation
	}
	return shape
}
