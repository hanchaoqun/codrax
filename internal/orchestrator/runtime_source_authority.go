package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

func skipAutoVerdictsForRuntimeSourceAuthority(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if runtimeSourceAuthorityAppliesToAutoVerdictSkip(authority) {
		return runtimeSourceAuthoritySkipsAutoVerdicts(authority)
	}
	return ctx.AnalysisIR.RequestModel.HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext(types.RuntimeArtifactContextActiveFromBus(ctx))
}

func runtimeSourceAuthorityAppliesToAutoVerdictSkip(authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	if !authority.Active {
		return false
	}
	return authority.RuntimeObservationCount > 0 ||
		authority.DeterministicRuntimeQueryCount > 0 ||
		authority.RuntimeOnlySufficient ||
		authority.CurrentSourceSatisfied
}

func runtimeSourceAuthoritySkipsAutoVerdicts(authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	if !authority.Active {
		return false
	}
	if authority.CurrentSourceSatisfied || authority.CanHardBlockCompletion {
		return false
	}
	return authority.RuntimeOnlySufficient ||
		authority.CanUseRuntimeOnlyWithCaveat ||
		authority.CanDowngradeToCaveat ||
		(authority.RuntimeObservationCount > 0 && !authority.CurrentSourceRequired)
}
