package tool

import "github.com/hanchaoqun/codrax/internal/types"

func sourceInventoryAggregateFactIsPrincipalCoverage(fact types.AnswerAggregateFact, rm *types.RequestModel) bool {
	role := types.AnswerAggregateFactRoleForRequest(fact, rm)
	if role == types.AnswerAggregateRolePrincipalAnswer {
		return true
	}
	if rm == nil ||
		rm.SourceInventoryProfile == nil ||
		!rm.SourceInventoryProfile.Active() ||
		types.SourceInventoryCompletionIsSupportOnly(*rm) ||
		types.NormalizeAnswerAggregateRole(fact.Role) != types.AnswerAggregateRolePrincipalAnswer ||
		fact.Kind != types.AnswerAggregateMemberSet {
		return false
	}
	return true
}
