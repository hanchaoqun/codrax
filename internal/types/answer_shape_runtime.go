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

// ExplanationAllowsAnchorSkeleton reports whether an explanation
// answer may legitimately carry the optional symbols[] anchor
// skeleton. This is reserved for multi-topic explanation answers,
// where each sub-topic gets one load-bearing anchor beneath the main
// prose. Single-topic explanations should keep symbols[] empty so
// auxiliary symbol validation cannot derail an otherwise valid
// narrative answer.
func ExplanationAllowsAnchorSkeleton(ir *AnalysisIR) bool {
	if ir == nil {
		return false
	}
	if EffectiveRequiredAnswerShape(ir, nil) != ShapeExplanation &&
		ir.AnswerContract.RequiredAnswerShape != ShapeExplanation {
		return false
	}
	return len(ir.RequestModel.SubTopics) > 1
}
