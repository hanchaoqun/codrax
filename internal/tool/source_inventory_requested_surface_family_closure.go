package tool

import (
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// sourceInventoryRequestedSurfaceFamilyClosureProven checks the requested
// construct universe at the same role × surface-family grain used by candidate
// filtering. The boolean pair is (applicable, proven): when no parser-backed
// requested family can be derived, callers retain the existing generic
// language/source-class closure path.
func sourceInventoryRequestedSurfaceFamilyClosureProven(
	ctx *types.BusContext,
	facts []types.AnswerAggregateFact,
) (bool, bool) {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil {
		return false, false
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	profile := ctx.AnalysisIR.RequestModel.SourceInventoryProfile
	observation := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	if graph == nil || profile == nil || !profile.Active() || !observation.IsActive() {
		return false, false
	}
	scopes := sourceInventoryRequestedScopes(ctx, graph)
	if len(scopes) == 0 {
		scopes = observation.Scopes
	}
	if len(scopes) == 0 {
		scopes = []string{"."}
	}
	requested := sourceInventoryRequestedSurfaceFamiliesByRole(
		ctx, newSourceInventoryGraphSymbolIndex(graph), scopes, profile,
	)
	if len(requested) == 0 {
		return false, false
	}

	rowSet := types.BuildSourceInventoryPrincipalRowSet(types.SourceInventoryPrincipalRowSetInput{
		Observation:  observation,
		RequestModel: ctx.AnalysisIR.RequestModel,
	})
	if !rowSet.Active {
		return true, false
	}
	groups := map[sourceInventoryRequestedSurfaceFamily][]types.SourceInventoryObservationMember{}
	for _, group := range sourceInventorySurfaceFamilyGroups(rowSet.PrincipalRows) {
		key := sourceInventoryRequestedSurfaceFamily{role: group.role, family: group.family}
		groups[key] = append(groups[key], group.members...)
	}
	// Single-row families are intentionally retained: the older partial-family
	// gap only needs groups of size >=2, while requested-universe closure must
	// also prove a requested family whose exact universe contains one member.
	for _, row := range rowSet.PrincipalRows {
		family := types.SourceInventorySurfaceTermKey(row.SurfaceFamily)
		if family == "" {
			family = types.SourceInventorySurfaceFamilyKey(row.Member.SurfaceTerms)
		}
		key := sourceInventoryRequestedSurfaceFamily{role: row.Role, family: family}
		if family == "" || sourceInventoryRequestedSurfaceFamilyMembersContain(groups[key], row.Member) {
			continue
		}
		groups[key] = append(groups[key], row.Member)
	}

	included, excluded, _ := sourceInventoryDuplicateAggregateCoverage(facts, &ctx.AnalysisIR.RequestModel)
	for role, families := range requested {
		for family := range families {
			key := sourceInventoryRequestedSurfaceFamily{role: role, family: family}
			if !sourceInventoryCompleteLensCoversSurfaceFamily(observation.CompleteLenses, key) {
				return true, false
			}
			members := groups[key]
			if len(members) == 0 {
				return true, false
			}
			for _, member := range members {
				location := sourceInventoryDuplicateRowLocationKey(member)
				if location == "" || (!included[location] && !excluded[location]) {
					return true, false
				}
			}
		}
	}
	return true, true
}

func sourceInventoryCompleteLensCoversSurfaceFamily(
	lenses []types.SourceInventoryCompleteLens,
	want sourceInventoryRequestedSurfaceFamily,
) bool {
	for _, lens := range lenses {
		if lens.Role != want.role {
			continue
		}
		for _, family := range lens.SurfaceFamilies {
			if types.SourceInventorySurfaceTermKey(family) == want.family {
				return true
			}
		}
	}
	return false
}

func sourceInventoryRequestedSurfaceFamilyMembersContain(
	members []types.SourceInventoryObservationMember,
	want types.SourceInventoryObservationMember,
) bool {
	key := sourceInventoryDuplicateRowLocationKey(want)
	if key == "" {
		return false
	}
	for _, member := range members {
		if sourceInventoryDuplicateRowLocationKey(member) == key {
			return true
		}
	}
	return false
}
