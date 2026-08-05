package tool

import (
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceInventoryCandidateSets(ctx *types.BusContext, graph *repotypes.Graph, view *sourceInventoryExecutionView, scopes []string, profile *types.SourceInventoryProfile, attributeRoles []types.AnswerCandidateRole, explicitAttributeRoles bool, includeAttributes bool, rawQuery string, budget sourceInventoryExecBudget) map[types.AnswerCandidateRole]sourceInventoryCandidateSet {
	out := map[types.AnswerCandidateRole]sourceInventoryCandidateSet{}
	if view == nil {
		view = newSourceInventoryExecutionView(graph, scopes)
	}
	scopeFilter := newSourceInventoryScopeFilter(ctx)
	queryFilter := sourceInventoryBuildQueryFilter(rawQuery)
	var symbolIndex *sourceInventoryGraphSymbolIndex
	getSymbolIndex := func() *sourceInventoryGraphSymbolIndex {
		if symbolIndex == nil {
			symbolIndex = newSourceInventoryGraphSymbolIndex(graph)
		}
		return symbolIndex
	}
	requestedSurfaceFamilies := sourceInventoryRequestedSurfaceFamiliesByRole(ctx, getSymbolIndex(), scopes, profile)
	// An explicit tool query remains the selector for roles that do not carry a
	// parser surface family (for example a route named in the same query as an
	// ArkTS decorator). The family-only closed filter applies only to the typed
	// implicit lane, where no explicit query exists.
	requireSurfaceFamilies := len(requestedSurfaceFamilies) > 0 && !queryFilter.Active()
	for _, role := range profile.PrincipalTargetRoles() {
		roleQueryFilter := sourceInventoryQueryFilterForRole(queryFilter, requestedSurfaceFamilies[role], requireSurfaceFamilies)
		switch {
		case role == types.AnswerCandidateRoleFile:
			var attributeIndex *sourceInventoryGraphSymbolIndex
			if includeAttributes && explicitAttributeRoles {
				attributeIndex = getSymbolIndex()
			}
			out[role] = sourceInventoryFileCandidates(ctx, view, attributeIndex, scopeFilter, profile, attributeRoles, explicitAttributeRoles, roleQueryFilter, budget)
		case role == types.AnswerCandidateRoleConfigFile:
			var attributeIndex *sourceInventoryGraphSymbolIndex
			if includeAttributes && explicitAttributeRoles {
				attributeIndex = getSymbolIndex()
			}
			out[role] = sourceInventoryConfigFileCandidates(ctx, view, attributeIndex, scopeFilter, profile, attributeRoles, explicitAttributeRoles, roleQueryFilter, budget)
		case role == types.AnswerCandidateRolePackage:
			var attributeIndex *sourceInventoryGraphSymbolIndex
			if includeAttributes {
				attributeIndex = getSymbolIndex()
			}
			out[role] = sourceInventoryPackageCandidates(ctx, view, attributeIndex, scopeFilter, profile, attributeRoles, explicitAttributeRoles, roleQueryFilter, budget)
		case role == types.AnswerCandidateRoleType && profile.IsStringEnumTypeInventory():
			out[role] = sourceInventoryGoStringEnumCandidates(ctx, graph, scopes, profile, budget)
		default:
			out[role] = sourceInventoryGraphCandidates(ctx, graph, view, getSymbolIndex(), scopeFilter, scopes, profile, role, roleQueryFilter, budget)
		}
		if len(out[role].candidates) == 0 && !out[role].truncated {
			delete(out, role)
		}
	}
	return out
}
