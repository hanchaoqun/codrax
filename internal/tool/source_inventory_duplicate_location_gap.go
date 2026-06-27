package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// SourceInventoryObservedDuplicateLocationCoverageGap catches a precise
// duplicate-location omission inside an otherwise accepted source-inventory
// member_set. It only uses typed source_inventory rows plus structured
// aggregate member/support refs; broad pagination and user prose stay out.
func SourceInventoryObservedDuplicateLocationCoverageGap(ctx *types.BusContext, facts []types.AnswerAggregateFact) SourceInventoryCandidateUniverseGap {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || len(facts) == 0 {
		return SourceInventoryCandidateUniverseGap{}
	}
	rowSet := types.BuildSourceInventoryPrincipalRowSet(types.SourceInventoryPrincipalRowSetInput{
		Observation:  types.SourceInventoryObservationFromMutable(ctx.Mutable),
		RequestModel: ctx.AnalysisIR.RequestModel,
	})
	if !rowSet.Active || len(rowSet.PrincipalRows) == 0 {
		return SourceInventoryCandidateUniverseGap{}
	}
	included, excluded, selected := sourceInventoryDuplicateAggregateCoverage(facts, &ctx.AnalysisIR.RequestModel)
	best := SourceInventoryCandidateUniverseGap{}
	for _, group := range sourceInventoryDuplicateLocationGroups(rowSet.PrincipalRows) {
		if len(group.members) < 2 || !sourceInventorySurfaceSelected(sourceInventoryDuplicateGroupLabel(group.key), selected) {
			continue
		}
		gap := sourceInventoryDuplicateCoverageForGroup(group, included, excluded)
		if !gap.IsActive() || gap.Covered == 0 {
			continue
		}
		gap.Blocking = true
		if sourceInventoryCandidateUniverseGapBetter(gap, best) {
			best = gap
		}
	}
	return best
}

type sourceInventoryDuplicateLocationGroup struct {
	key     string
	role    types.AnswerCandidateRole
	members []types.SourceInventoryObservationMember
}

func sourceInventoryDuplicateLocationGroups(rows []types.SourceInventoryRow) []sourceInventoryDuplicateLocationGroup {
	groups := map[string]*sourceInventoryDuplicateLocationGroup{}
	var order []string
	for _, row := range rows {
		member := row.Member
		if len(member.SurfaceTerms) == 0 || sourceInventoryDuplicateRowLocationKey(member) == "" {
			continue
		}
		surfaceKey := sourceInventoryDuplicateRowSurfaceKey(member)
		if surfaceKey == "" {
			continue
		}
		key := string(row.Role) + "\x00" + surfaceKey
		group := groups[key]
		if group == nil {
			group = &sourceInventoryDuplicateLocationGroup{key: key, role: row.Role}
			groups[key] = group
			order = append(order, key)
		}
		group.members = append(group.members, member)
	}
	out := make([]sourceInventoryDuplicateLocationGroup, 0, len(order))
	for _, key := range order {
		group := groups[key]
		if group == nil || sourceInventoryDuplicateDistinctLocationCount(group.members) < 2 {
			continue
		}
		out = append(out, *group)
	}
	return out
}

func sourceInventoryDuplicateCoverageForGroup(group sourceInventoryDuplicateLocationGroup, included, excluded map[string]bool) SourceInventoryCandidateUniverseGap {
	gap := SourceInventoryCandidateUniverseGap{
		Role:  group.role,
		Scope: "duplicate_location:" + sourceInventoryDuplicateGroupLabel(group.key),
		Count: len(group.members),
	}
	for _, member := range group.members {
		loc := sourceInventoryDuplicateRowLocationKey(member)
		switch {
		case included[loc]:
			gap.Covered++
		case excluded[loc]:
			gap.Excluded++
		default:
			gap.Missing = append(gap.Missing, member)
		}
	}
	return gap
}

func sourceInventoryDuplicateAggregateCoverage(facts []types.AnswerAggregateFact, rm *types.RequestModel) (map[string]bool, map[string]bool, map[string]bool) {
	included, excluded, selected := map[string]bool{}, map[string]bool{}, map[string]bool{}
	addSurface := func(raw string) {
		for _, key := range sourceInventorySurfaceCoverageKeys(raw) {
			selected[key] = true
		}
		if label, _, ok := types.ParseAnswerSupportRefMemberLocation(raw); ok {
			for _, key := range sourceInventorySurfaceCoverageKeys(label) {
				selected[key] = true
			}
		}
	}
	addLocations := func(target map[string]bool, raw string) {
		for _, loc := range sourceInventoryDuplicateLocationKeys(raw) {
			target[loc] = true
		}
	}
	for _, fact := range facts {
		role := types.AnswerAggregateFactRoleForRequest(fact, rm)
		if role == types.AnswerAggregateRolePrincipalAnswer && types.AnswerAggregateFactCarriesCompleteMemberSet(fact) {
			for _, member := range fact.Members {
				addSurface(member)
				addLocations(included, member)
			}
			for _, ref := range fact.SupportRefs {
				addLocations(included, ref)
			}
		}
		for _, member := range fact.Excluded {
			addLocations(excluded, member)
		}
		if fact.Kind == types.AnswerAggregateExcluded {
			for _, member := range fact.Members {
				addLocations(excluded, member)
			}
		}
	}
	return included, excluded, selected
}

func sourceInventoryDuplicateLocationKeys(raw string) []string {
	var out []string
	if _, loc, ok := types.ParseAnswerSupportRefMemberLocation(raw); ok {
		out = append(out, sourceInventoryDuplicateLocationKey(loc.File, loc.LineStart))
	}
	if loc, ok := types.ParseAnswerSourceLocationSurface(raw); ok {
		out = append(out, sourceInventoryDuplicateLocationKey(loc.File, loc.LineStart))
	}
	return sourceInventoryUniverseDedupKeys(out)
}

func sourceInventoryDuplicateRowLocationKey(member types.SourceInventoryObservationMember) string {
	if key := sourceInventoryDuplicateLocationKey(member.File, member.Line); key != "" {
		return key
	}
	keys := sourceInventoryDuplicateLocationKeys(member.SupportRef)
	if len(keys) > 0 {
		return keys[0]
	}
	return ""
}

func sourceInventoryDuplicateRowSurfaceKey(member types.SourceInventoryObservationMember) string {
	if key := sourceInventoryMostSpecificSurfaceKey(member.SurfaceTerms); key != "" {
		return key
	}
	return sourceInventoryUniverseSurfaceKey(member.Name)
}

func sourceInventoryDuplicateLocationKey(file string, line int) string {
	file = strings.Trim(strings.ReplaceAll(strings.TrimSpace(file), `\`, `/`), "/")
	if file == "" || line <= 0 {
		return ""
	}
	return strings.ToLower(fmt.Sprintf("%s:%d", file, line))
}

func sourceInventoryDuplicateDistinctLocationCount(members []types.SourceInventoryObservationMember) int {
	seen := map[string]bool{}
	for _, member := range members {
		if loc := sourceInventoryDuplicateRowLocationKey(member); loc != "" {
			seen[loc] = true
		}
	}
	return len(seen)
}

func sourceInventoryDuplicateGroupLabel(key string) string {
	if idx := strings.Index(key, "\x00"); idx >= 0 {
		return key[idx+1:]
	}
	return key
}
