package tool

import "github.com/hanchaoqun/codrax/internal/types"

func sourceInventoryCompletionRepairRoleLabels(authority types.SourceInventoryCompletionAuthority, observation types.SourceInventoryObservation) []string {
	if debt := types.NormalizeSourceInventoryFollowupDebt(authority.FollowupDebt); debt.IsActive() {
		if labels := sourceInventoryLensExecutionRoleLabels(debt.Query.Roles); len(labels) > 0 {
			return labels
		}
	}
	if labels := sourceInventoryLensExecutionRoleLabels(authority.Roles); len(labels) > 0 {
		return labels
	}
	return sourceInventoryLensExecutionRoleLabels(sourceInventoryObservationRolesForCompletion(observation))
}

func sourceInventoryCompletionRepairScopes(authority types.SourceInventoryCompletionAuthority) []string {
	if debt := types.NormalizeSourceInventoryFollowupDebt(authority.FollowupDebt); debt.IsActive() && len(debt.Query.Scopes) > 0 {
		return append([]string(nil), debt.Query.Scopes...)
	}
	return append([]string(nil), authority.Scopes...)
}
