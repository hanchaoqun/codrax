package tool

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// sourceInventoryRequestedFileUniverseClosedByCompleteLens accepts a complete
// source-inventory result for the exact file boundary carried by
// EvidencePlan.RequiredFiles, even when an earlier repo-wide lens left stale
// same-role debt in the merged observation.
//
// This is deliberately a positive proof, not a bounded-scope guess. In an
// active source-inventory lane analyzerRequiredFiles contains only existing
// file paths explicitly resolved from MentionedEntities/ExactTargets (ranker
// candidates are excluded by analyzerSourceInventoryExplicitRequiredFiles).
// Completion additionally requires an executed, non-root complete lens over
// those exact files, count/row parity, a model-authored principal member_set,
// and a scoped location for every included row. No raw request, model reason,
// aggregate label/dimension, or final-answer prose participates in the gate.
func sourceInventoryRequestedFileUniverseClosedByCompleteLens(
	ctx *types.BusContext,
	observation types.SourceInventoryObservation,
	facts []types.AnswerAggregateFact,
	roles map[types.AnswerCandidateRole]bool,
	included, excluded map[string]bool,
) bool {
	closed, _ := sourceInventoryRequestedFileClosureProof(ctx, observation, facts, roles, included, excluded)
	return closed
}

// SourceInventoryRequestedFileCoverageGap returns the exact missing rows from
// an executed complete lens over analyzer-resolved requested files. It is the
// repair twin of sourceInventoryRequestedFileUniverseClosedByCompleteLens:
// complete coverage grants closure; partial coverage names the still-unhandled
// typed rows instead of sending the model into unrelated repo-wide families.
func SourceInventoryRequestedFileCoverageGap(ctx *types.BusContext, facts []types.AnswerAggregateFact) SourceInventoryCandidateUniverseGap {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || len(facts) == 0 {
		return SourceInventoryCandidateUniverseGap{}
	}
	rm := ctx.AnalysisIR.RequestModel
	observation := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	included, excluded := sourceInventoryAggregateCoverageKeys(facts, &rm)
	if len(included) == 0 {
		return SourceInventoryCandidateUniverseGap{}
	}
	roles := sourceInventoryRequestedUniverseRoleSet(rm, observation, included)
	_, gap := sourceInventoryRequestedFileClosureProof(ctx, observation, facts, roles, included, excluded)
	return gap
}

func sourceInventoryRequestedFileClosureProof(
	ctx *types.BusContext,
	observation types.SourceInventoryObservation,
	facts []types.AnswerAggregateFact,
	roles map[types.AnswerCandidateRole]bool,
	included, excluded map[string]bool,
) (bool, SourceInventoryCandidateUniverseGap) {
	if ctx == nil || ctx.AnalysisIR == nil || len(observation.CompleteLenses) == 0 || len(included) == 0 {
		return false, SourceInventoryCandidateUniverseGap{}
	}
	rm := ctx.AnalysisIR.RequestModel
	// An explicit source-class scope (all/production/test/etc.) owns the
	// universe. A navigation file inside such a request must never narrow it.
	if rm.SourceScopeProfile != nil {
		return false, SourceInventoryCandidateUniverseGap{}
	}
	requested := sourceInventoryRequestedFileSet(ctx.AnalysisIR.EvidencePlan.RequiredFiles)
	if len(requested) == 0 {
		return false, SourceInventoryCandidateUniverseGap{}
	}
	located := sourceInventoryPrincipalAggregateLocatedKeys(facts, &rm)
	if len(located) == 0 {
		return false, SourceInventoryCandidateUniverseGap{}
	}
	coveredFiles := map[string]bool{}
	bestGap := SourceInventoryCandidateUniverseGap{}
	for _, lens := range observation.CompleteLenses {
		if !sourceInventoryRequestedUniverseRoleAllowed(roles, lens.Role) ||
			lens.Count <= 0 || lens.Total != lens.Count ||
			!sourceInventoryCompleteLensHasExecutionCredential(lens) {
			continue
		}
		scopes, ok := sourceInventoryCompleteLensExactRequestedFiles(lens.Scopes, requested)
		if !ok {
			continue
		}
		members := sourceInventoryCompleteLensMembers(observation, lens.Role, scopes)
		if len(members) != lens.Count {
			continue
		}
		gap := SourceInventoryCandidateUniverseGap{
			Role:  lens.Role,
			Scope: strings.Join(sourceInventoryRequestedFileSortedSet(scopes), ","),
			Count: len(members),
		}
		for _, member := range members {
			keys := sourceInventoryUniverseMemberKeys(member)
			switch {
			case sourceInventoryUniverseAnyKey(keys, excluded):
				// Explicit model exclusion closes the row without promoting it
				// into the answer slate.
				gap.Excluded++
			case sourceInventoryUniverseAnyKey(keys, included):
				if !sourceInventoryLocatedKeyWithinScopes(keys, located, scopes) {
					gap.Missing = append(gap.Missing, member)
					continue
				}
				gap.Covered++
			default:
				gap.Missing = append(gap.Missing, member)
			}
		}
		if len(gap.Missing) > 0 {
			gap.Blocking = gap.Covered > 0
			if gap.Blocking && sourceInventoryCandidateUniverseGapBetter(gap, bestGap) {
				bestGap = gap
			}
			continue
		}
		if gap.Covered == 0 {
			continue
		}
		for file := range scopes {
			coveredFiles[file] = true
		}
	}
	if len(coveredFiles) != len(requested) {
		return false, bestGap
	}
	for file := range requested {
		if !coveredFiles[file] {
			return false, bestGap
		}
	}
	return true, SourceInventoryCandidateUniverseGap{}
}

func sourceInventoryRequestedFileSet(files []string) map[string]bool {
	out := map[string]bool{}
	for _, raw := range files {
		file := sourceInventoryRequestedFileCleanPath(raw)
		if file == "" || file == "." || !types.HasCodeOrConfigPathSuffix(file) {
			continue
		}
		out[file] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sourceInventoryRequestedFileSortedSet(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sourceInventoryCompleteLensHasExecutionCredential(lens types.SourceInventoryCompleteLens) bool {
	hasTool := false
	hasExecutableStage := false
	for _, raw := range lens.Provenance {
		switch strings.TrimSpace(raw) {
		case types.SourceInventoryProvenanceRepoLensToolQuery:
			hasTool = true
		case types.SourceInventoryProvenancePreExplore, types.SourceInventoryProvenanceStageExplore:
			hasExecutableStage = true
		}
	}
	// A same-key analyzer lens can be merged with the later executable lens;
	// the positive explore credential remains authoritative in that union.
	return hasTool && hasExecutableStage
}

func sourceInventoryCompleteLensExactRequestedFiles(scopes []string, requested map[string]bool) (map[string]bool, bool) {
	if len(scopes) == 0 {
		return nil, false
	}
	out := map[string]bool{}
	for _, raw := range scopes {
		scope := sourceInventoryRequestedFileCleanPath(raw)
		if scope == "" || scope == "." || !requested[scope] {
			return nil, false
		}
		out[scope] = true
	}
	return out, len(out) > 0
}

func sourceInventoryCompleteLensMembers(observation types.SourceInventoryObservation, role types.AnswerCandidateRole, scopes map[string]bool) []types.SourceInventoryObservationMember {
	seen := map[string]bool{}
	var out []types.SourceInventoryObservationMember
	for _, set := range observation.Sets {
		setRole := set.Role
		if setRole == "" || setRole == types.AnswerCandidateRoleUnknown {
			setRole = role
		}
		if setRole != role {
			continue
		}
		for _, member := range set.Members {
			memberRole := member.Role
			if memberRole == "" || memberRole == types.AnswerCandidateRoleUnknown {
				memberRole = setRole
			}
			if memberRole != role {
				continue
			}
			file := sourceInventoryRequestedFileMemberPath(member)
			if !scopes[file] {
				continue
			}
			key := string(memberRole) + "\x00" + file + "\x00" + strconv.Itoa(member.Line) + "\x00" + strings.TrimSpace(member.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, member)
		}
	}
	return out
}

func sourceInventoryRequestedFileMemberPath(member types.SourceInventoryObservationMember) string {
	if file := sourceInventoryRequestedFileCleanPath(member.File); file != "" && file != "." {
		return file
	}
	for _, raw := range []string{member.SupportRef, member.Key} {
		if _, loc, ok := types.ParseAnswerSupportRefMemberLocation(raw); ok {
			return sourceInventoryRequestedFileCleanPath(loc.File)
		}
		if loc, ok := types.ParseAnswerSourceLocationSurface(raw); ok {
			return sourceInventoryRequestedFileCleanPath(loc.File)
		}
	}
	return ""
}

func sourceInventoryPrincipalAggregateLocatedKeys(facts []types.AnswerAggregateFact, rm *types.RequestModel) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	add := func(raw string) {
		var label string
		var file string
		if parsedLabel, loc, ok := types.ParseAnswerSupportRefMemberLocation(raw); ok {
			label, file = parsedLabel, loc.File
		} else if loc, ok := types.ParseAnswerSourceLocationSurface(raw); ok {
			file = loc.File
		} else {
			return
		}
		file = sourceInventoryRequestedFileCleanPath(file)
		if file == "" || file == "." {
			return
		}
		keys := sourceInventoryUniverseAggregateMemberKeys(raw)
		if label != "" {
			keys = append(keys, sourceInventoryUniverseAggregateMemberKeys(label)...)
		}
		for _, key := range sourceInventoryUniverseDedupKeys(keys) {
			if out[key] == nil {
				out[key] = map[string]bool{}
			}
			out[key][file] = true
		}
	}
	for _, fact := range facts {
		if types.AnswerAggregateFactRoleForRequest(fact, rm) != types.AnswerAggregateRolePrincipalAnswer ||
			!types.AnswerAggregateFactCarriesCompleteMemberSet(fact) {
			continue
		}
		for _, member := range fact.Members {
			add(member)
		}
		for _, ref := range fact.SupportRefs {
			add(ref)
		}
	}
	return out
}

func sourceInventoryLocatedKeyWithinScopes(keys []string, located map[string]map[string]bool, scopes map[string]bool) bool {
	for _, key := range keys {
		for file := range located[key] {
			if scopes[file] {
				return true
			}
		}
	}
	return false
}

func sourceInventoryRequestedFileCleanPath(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\`, "/"))
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return ""
	}
	clean := path.Clean(raw)
	if clean == "." || clean == "/" {
		return "."
	}
	return strings.Trim(clean, "/")
}
