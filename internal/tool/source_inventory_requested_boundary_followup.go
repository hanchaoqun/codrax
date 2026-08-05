package tool

import "github.com/hanchaoqun/codrax/internal/types"

func sourceInventoryRequestedBoundaryFollowupDebt(ctx *types.BusContext, rm types.RequestModel) types.SourceInventoryFollowupDebt {
	scopes := types.SourceInventoryRequestedPathScopes(rm)
	if len(scopes) == 0 {
		return sourceInventoryRequestedFileFollowupDebt(ctx, rm)
	}
	if rm.SourceInventoryProfile == nil {
		return types.SourceInventoryFollowupDebt{}
	}
	roles := rm.SourceInventoryProfile.PrincipalTargetRoles()
	if len(roles) == 0 {
		roles = append([]types.AnswerCandidateRole(nil), rm.SourceInventoryProfile.TargetRoles...)
	}
	return types.NormalizeSourceInventoryFollowupDebt(types.SourceInventoryFollowupDebt{
		Active: true, ReasonCode: types.SourceInventoryFollowupDebtRequestedPathBoundary,
		Query: types.SourceInventoryLensQuery{
			Path: ".", Scopes: scopes, Roles: roles,
			IncludeCounts: true, IncludeAttributes: false, TopN: 24,
		},
		Roles: roles,
	})
}

func sourceInventoryRequestedFileFollowupDebt(ctx *types.BusContext, rm types.RequestModel) types.SourceInventoryFollowupDebt {
	if ctx == nil || ctx.AnalysisIR == nil || rm.SourceScopeProfile != nil {
		return types.SourceInventoryFollowupDebt{}
	}
	requested := sourceInventoryRequestedFileSet(ctx.AnalysisIR.EvidencePlan.RequiredFiles)
	if len(requested) == 0 {
		return types.SourceInventoryFollowupDebt{}
	}
	roles := rm.SourceInventoryProfile.PrincipalTargetRoles()
	if len(roles) == 0 {
		roles = append([]types.AnswerCandidateRole(nil), rm.SourceInventoryProfile.TargetRoles...)
	}
	return types.NormalizeSourceInventoryFollowupDebt(types.SourceInventoryFollowupDebt{
		Active: true, ReasonCode: types.SourceInventoryFollowupDebtRequestedFiles,
		Query: types.SourceInventoryLensQuery{
			Path: ".", Scopes: sourceInventoryRequestedFileSortedSet(requested), Roles: roles,
			IncludeCounts: true, IncludeAttributes: false, TopN: 50,
		},
		Roles: roles,
	})
}
