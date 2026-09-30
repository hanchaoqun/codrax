package agent

import "github.com/hanchaoqun/codrax/internal/types"

// Secondary obligations survive a count/return-value primary intent. This
// drives only existing soft relationship guidance, not a new source gate or
// generated mechanism conclusion. Route and producer checks remain upstream.
func measurementHasRequiredMechanismDimension(rm types.RequestModel) bool {
	p := rm.RequestedAnswerDimensions
	if p == nil || !p.Active() {
		return false
	}
	count, mechanism := rm.Predicates.IsCountQuestion, false
	for _, d := range p.Dimensions {
		if !d.Required {
			continue
		}
		switch d.Role {
		case types.RequestedAnswerDimensionCount:
			count = true
		case types.RequestedAnswerDimensionFunctionOrPurpose, types.RequestedAnswerDimensionBranchBehavior,
			types.RequestedAnswerDimensionRelationPath, types.RequestedAnswerDimensionStageWorkflow:
			mechanism = true
		}
	}
	return count && mechanism
}
