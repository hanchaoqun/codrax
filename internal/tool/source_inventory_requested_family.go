package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceInventoryRequestedUniverseAggregateFamilyCoversCensus(observation types.SourceInventoryObservation, covered sourceInventoryRequestedUniverseFamily) bool {
	if len(covered.languages) == 0 && len(covered.classes) == 0 {
		return false
	}
	if len(observation.SourceClasses) == 0 {
		return false
	}
	return !sourceInventoryRequestedUniverseCensusMissing(observation, covered)
}

func sourceInventoryAggregatePrincipalSourceFamily(facts []types.AnswerAggregateFact, rm *types.RequestModel) sourceInventoryRequestedUniverseFamily {
	out := sourceInventoryRequestedUniverseFamily{
		languages: map[string]bool{},
		classes:   map[types.SourcePathRole]bool{},
	}
	for _, fact := range facts {
		if types.AnswerAggregateFactRoleForRequest(fact, rm) != types.AnswerAggregateRolePrincipalAnswer ||
			!types.AnswerAggregateFactCarriesCompleteMemberSet(fact) {
			continue
		}
		for _, surface := range append(append([]string(nil), fact.Members...), fact.SupportRefs...) {
			sourceInventoryRequestedUniverseFamilyAddSurface(&out, surface)
		}
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
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		for _, lang := range sourceInventoryPathLanguages(file) {
			family.languages[lang] = true
		}
		if class := types.ClassifySourcePathRole(file); class != types.SourcePathRoleUnknown {
			family.classes[class] = true
		}
	}
}
