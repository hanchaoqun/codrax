package tool

import "github.com/hanchaoqun/codrax/internal/types"

// Carry current-run source applicability through nested source-inventory
// sensors without mutating the persisted IR or dropping accepted observations.
func sourceInventoryCurrentRunContext(ctx *types.BusContext) *types.BusContext {
	rm := types.SourceInventoryRequestModelFromBusContext(ctx)
	if rm == nil {
		return ctx
	}
	out := ctx.ShallowClone()
	ir := *ctx.AnalysisIR
	ir.RequestModel = *rm
	out.AnalysisIR = &ir
	return out
}

// SourceInventoryLensExecutionGapForContext reports whether an applicable
// principal source inventory has reached the executable repo-map boundary.
func SourceInventoryLensExecutionGapForContext(ctx *types.BusContext) SourceInventoryLensExecutionGap {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return SourceInventoryLensExecutionGap{}
	}
	rm := ctx.AnalysisIR.RequestModel
	if !types.SourceInventoryCurrentSourceApplicableFromBus(ctx) {
		return SourceInventoryLensExecutionGap{}
	}
	profile := rm.SourceInventoryProfile
	advisory := ctx.Mutable.SourceInventoryAdvisory()
	var roles []types.AnswerCandidateRole
	if types.SourceInventoryPrincipalAuthorityActive(rm) {
		roles = profile.PrincipalTargetRoles()
		if len(roles) == 0 {
			roles = append([]types.AnswerCandidateRole(nil), profile.TargetRoles...)
		}
	} else if sourceInventoryAdvisoryIsTypedQueryLane(ctx, advisory) {
		roles = sourceInventoryLensExecutionRolesFromAdvisory(advisory)
	} else {
		return SourceInventoryLensExecutionGap{}
	}
	observation := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	gap := SourceInventoryLensExecutionGap{
		Roles:       sourceInventoryLensExecutionRoles(roles),
		Scopes:      sourceInventoryLensExecutionScopes(advisory, observation),
		HasAdvisory: advisory.IsActive(),
	}
	if sourceInventoryObservationHasListFilesDirect(observation) {
		gap.HasListFiles = true
	}
	if sourceInventoryAdvisoryHasRepoLensToolQuery(advisory) ||
		sourceInventoryObservationHasRepoLensToolQuery(observation) ||
		sourceInventoryObservationIsExecutedEmptyLens(observation) ||
		sourceInventoryToolResultsHaveSourceInventoryLens(ctx.Mutable.DispatchToolResults()) ||
		sourceInventoryToolResultsHaveSourceInventoryLens(ctx.ToolResults) {
		return gap
	}
	gap.Blocking = true
	return gap
}
