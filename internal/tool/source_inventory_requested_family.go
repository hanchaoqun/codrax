package tool

import "github.com/hanchaoqun/codrax/internal/types"

func sourceInventoryRequestedUniverseAggregateFamilyCoversCensus(observation types.SourceInventoryObservation, covered sourceInventoryRequestedUniverseFamily) bool {
	if len(covered.languages) == 0 && len(covered.classes) == 0 {
		return false
	}
	if len(observation.SourceClasses) == 0 {
		return false
	}
	return !sourceInventoryRequestedUniverseCensusMissing(observation, covered)
}

func sourceInventoryAggregatePrincipalSourceFamily(ctx *types.BusContext, facts []types.AnswerAggregateFact, rm *types.RequestModel) sourceInventoryRequestedUniverseFamily {
	out := sourceInventoryRequestedUniverseFamily{
		languages: map[string]bool{},
		classes:   map[types.SourcePathRole]bool{},
	}
	evidence := sourceInventoryAcceptedEvidenceFamilyIndex(ctx)
	for _, fact := range facts {
		if types.AnswerAggregateFactRoleForRequest(fact, rm) != types.AnswerAggregateRolePrincipalAnswer ||
			!types.AnswerAggregateFactCarriesCompleteMemberSet(fact) {
			continue
		}
		sourceInventoryRequestedUniverseFamilyAddFact(&out, fact, evidence)
	}
	return out
}

func sourceInventoryRequestedUniverseFamilyAddSurface(family *sourceInventoryRequestedUniverseFamily, surface string) {
	if family == nil {
		return
	}
	var paths []string
	if _, loc, ok := types.ParseAnswerSupportRefMemberLocation(surface); ok {
		paths = append(paths, loc.File)
	} else if loc, ok := types.ParseAnswerSourceLocationSurface(surface); ok {
		paths = append(paths, loc.File)
	}
	for _, file := range paths {
		sourceInventoryRequestedUniverseFamilyAddClassifiedPath(family, file)
	}
}
