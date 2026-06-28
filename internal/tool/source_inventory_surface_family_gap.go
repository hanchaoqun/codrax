package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// SourceInventoryObservedSurfaceFamilyCoverageGap catches precise partial
// coverage inside a typed source-inventory surface family. It only consumes
// source_inventory SurfaceTerms plus exact file:line coverage from structured
// aggregate facts; broad pagination, user prose, and model rationale are never
// load-bearing.
func SourceInventoryObservedSurfaceFamilyCoverageGap(ctx *types.BusContext, facts []types.AnswerAggregateFact) SourceInventoryCandidateUniverseGap {
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
	included, excluded, _ := sourceInventoryDuplicateAggregateCoverage(facts, &ctx.AnalysisIR.RequestModel)
	best := SourceInventoryCandidateUniverseGap{}
	for _, group := range sourceInventorySurfaceFamilyGroups(rowSet.PrincipalRows) {
		if len(group.members) < 2 {
			continue
		}
		gap := sourceInventorySurfaceFamilyCoverageForGroup(group, included, excluded)
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

type sourceInventorySurfaceFamilyGroup struct {
	family  string
	role    types.AnswerCandidateRole
	members []types.SourceInventoryObservationMember
}

func sourceInventorySurfaceFamilyGroups(rows []types.SourceInventoryRow) []sourceInventorySurfaceFamilyGroup {
	groups := map[string]*sourceInventorySurfaceFamilyGroup{}
	var order []string
	for _, row := range rows {
		member := row.Member
		if sourceInventoryDuplicateRowLocationKey(member) == "" {
			continue
		}
		family := sourceInventorySurfaceFamilyKey(member.SurfaceTerms)
		if family == "" {
			continue
		}
		key := string(row.Role) + "\x00" + family
		group := groups[key]
		if group == nil {
			group = &sourceInventorySurfaceFamilyGroup{family: family, role: row.Role}
			groups[key] = group
			order = append(order, key)
		}
		group.members = append(group.members, member)
	}
	out := make([]sourceInventorySurfaceFamilyGroup, 0, len(order))
	for _, key := range order {
		group := groups[key]
		if group == nil || sourceInventoryDuplicateDistinctLocationCount(group.members) < 2 {
			continue
		}
		out = append(out, *group)
	}
	return out
}

func sourceInventorySurfaceFamilyCoverageForGroup(group sourceInventorySurfaceFamilyGroup, included, excluded map[string]bool) SourceInventoryCandidateUniverseGap {
	gap := SourceInventoryCandidateUniverseGap{
		Role:  group.role,
		Scope: "surface_family:" + group.family,
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

func sourceInventorySurfaceCoverageKeys(raw string) []string {
	var out []string
	add := func(raw string) {
		key := sourceInventoryUniverseSurfaceKey(raw)
		if key == "" {
			return
		}
		out = append(out, key)
		if idx := strings.Index(key, "("); idx > 0 {
			out = append(out, strings.TrimSpace(key[:idx]))
		}
	}
	add(raw)
	for _, key := range sourceInventoryUniverseAggregateMemberKeys(raw) {
		add(key)
	}
	if label, loc, ok := types.ParseAnswerSupportRefMemberLocation(raw); ok {
		add(label)
		add(loc.File)
	}
	return sourceInventoryUniverseDedupKeys(out)
}

func sourceInventoryMostSpecificSurfaceKey(terms []string) string {
	best := ""
	for _, term := range terms {
		key := sourceInventoryUniverseSurfaceKey(term)
		if key == "" || len(key) <= len(best) {
			continue
		}
		best = key
	}
	return best
}

func sourceInventorySurfaceFamilyKey(terms []string) string {
	keys := sourceInventoryUniverseDedupKeys(sourceInventorySurfaceTermKeys(terms))
	for _, candidate := range keys {
		for _, other := range keys {
			if other != candidate && strings.HasPrefix(other, candidate+" ") {
				return candidate
			}
		}
	}
	for _, key := range keys {
		if idx := strings.LastIndex(key, " "); idx > 0 {
			return strings.TrimSpace(key[:idx])
		}
	}
	return ""
}

func sourceInventorySurfaceTermKeys(terms []string) []string {
	var out []string
	for _, term := range terms {
		if key := sourceInventoryUniverseSurfaceKey(term); key != "" {
			out = append(out, key)
		}
	}
	return out
}

func sourceInventorySurfaceSelected(surface string, selected map[string]bool) bool {
	surface = sourceInventoryUniverseSurfaceKey(surface)
	if surface == "" {
		return false
	}
	if selected[surface] {
		return true
	}
	for key := range selected {
		if strings.HasPrefix(key, surface+" ") {
			return true
		}
	}
	return false
}
