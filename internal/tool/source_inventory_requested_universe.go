package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// SourceInventoryAcceptedClosureCoversRequestedUniverse reports whether a broad
// source-inventory observation has a narrower typed requested universe that is
// already closed by exact source-inventory rows and structured aggregate
// member_set coverage. It is intentionally stricter than
// SourceInventoryAcceptedClosureCoversExactUniverse: a single bounded exact
// scope does not close a broader request while the typed census still exposes
// uncovered same-family source classes.
func SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx *types.BusContext, facts []types.AnswerAggregateFact) bool {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || len(facts) == 0 {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false
	}
	observation := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	aggregateFamily := sourceInventoryAggregatePrincipalSourceFamily(ctx, facts, &rm)
	universes := sourceInventoryExactUniverseSets(observation)
	if len(universes) == 0 {
		return sourceInventoryRequestedUniverseAggregateFamilyCoversCensus(observation, aggregateFamily)
	}
	included, excluded := sourceInventoryAggregateCoverageKeys(facts, &rm)
	if len(included) == 0 {
		return sourceInventoryRequestedUniverseAggregateFamilyCoversCensus(observation, aggregateFamily)
	}
	roleSet := sourceInventoryRequestedUniverseRoleSet(rm, observation, included)
	familyClosedByCompleteLens := sourceInventoryRequestedUniverseAggregateFamilyCoversCensus(observation, aggregateFamily) &&
		sourceInventoryRequestedUniverseFamilyBackedByCompleteLens(observation, aggregateFamily, roleSet)
	covered := sourceInventoryRequestedUniverseFamily{
		languages: map[string]bool{},
		classes:   map[types.SourcePathRole]bool{},
	}
	coveredUniverse := map[string]bool{}
	for _, universe := range universes {
		if !sourceInventoryRequestedUniverseRoleAllowed(roleSet, universe.role) {
			continue
		}
		gap := sourceInventoryCoverageForUniverse(universe, included, excluded)
		if gap.Count > 0 && gap.Covered > 0 && gap.Covered+gap.Excluded == gap.Count {
			coveredUniverse[sourceInventoryRequestedUniverseKey(universe)] = true
			covered.addUniverse(universe)
		}
	}
	if len(covered.languages) == 0 && len(covered.classes) == 0 {
		if familyClosedByCompleteLens {
			return true
		}
		return sourceInventoryRequestedUniverseAggregateFamilyCoversCensus(observation, aggregateFamily)
	}
	for _, universe := range universes {
		if !sourceInventoryRequestedUniverseRoleAllowed(roleSet, universe.role) ||
			!covered.intersectsUniverse(universe) {
			continue
		}
		if coveredUniverse[sourceInventoryRequestedUniverseKey(universe)] {
			continue
		}
		gap := sourceInventoryCoverageForUniverse(universe, included, excluded)
		if gap.Count > 0 && gap.Covered+gap.Excluded < gap.Count {
			return familyClosedByCompleteLens
		}
	}
	if !sourceInventoryRequestedUniverseCensusMissing(observation, covered) {
		return true
	}
	return familyClosedByCompleteLens
}

type sourceInventoryRequestedUniverseFamily struct {
	languages map[string]bool
	classes   map[types.SourcePathRole]bool
}

func (f sourceInventoryRequestedUniverseFamily) addUniverse(universe sourceInventoryExactUniverseSet) {
	for _, member := range universe.members {
		for _, lang := range sourceInventoryMemberLanguages(member) {
			f.languages[lang] = true
		}
		if class := sourceInventoryMemberClass(member); class != types.SourcePathRoleUnknown {
			f.classes[class] = true
		}
	}
}

func (f sourceInventoryRequestedUniverseFamily) intersectsUniverse(universe sourceInventoryExactUniverseSet) bool {
	for _, member := range universe.members {
		for _, lang := range sourceInventoryMemberLanguages(member) {
			if f.languages[lang] {
				return true
			}
		}
		if class := sourceInventoryMemberClass(member); class != types.SourcePathRoleUnknown && f.classes[class] {
			return true
		}
	}
	return false
}

func sourceInventoryRequestedUniverseRoleSet(rm types.RequestModel, observation types.SourceInventoryObservation, included map[string]bool) map[types.AnswerCandidateRole]bool {
	roles := []types.AnswerCandidateRole{}
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		roles = rm.SourceInventoryProfile.PrincipalTargetRoles()
		if len(roles) == 0 {
			roles = append([]types.AnswerCandidateRole(nil), rm.SourceInventoryProfile.TargetRoles...)
		}
	}
	if len(roles) == 0 {
		for _, set := range observation.Sets {
			roles = append(roles, set.Role)
		}
	}
	out := map[types.AnswerCandidateRole]bool{}
	for _, role := range roles {
		if role != "" && role != types.AnswerCandidateRoleUnknown {
			out[role] = true
		}
	}
	for _, set := range observation.Sets {
		if set.Role == "" || set.Role == types.AnswerCandidateRoleUnknown {
			continue
		}
		for _, member := range set.Members {
			if sourceInventoryUniverseAnyKey(sourceInventoryUniverseMemberKeys(member), included) {
				out[set.Role] = true
				break
			}
		}
	}
	return out
}

func sourceInventoryRequestedUniverseRoleAllowed(roles map[types.AnswerCandidateRole]bool, role types.AnswerCandidateRole) bool {
	if role == "" || role == types.AnswerCandidateRoleUnknown {
		return false
	}
	return len(roles) == 0 || roles[role]
}

func sourceInventoryRequestedUniverseKey(universe sourceInventoryExactUniverseSet) string {
	return string(universe.role) + "\x00" + strings.TrimSpace(universe.scope)
}

func sourceInventoryMemberClass(member types.SourceInventoryObservationMember) types.SourcePathRole {
	for _, raw := range []string{member.File, member.SupportRef, member.Key} {
		class := types.ClassifySourcePathRole(raw)
		if class != types.SourcePathRoleUnknown {
			return class
		}
	}
	return types.SourcePathRoleUnknown
}

func sourceInventoryMemberLanguages(member types.SourceInventoryObservationMember) []string {
	var out []string
	add := func(lang string) {
		lang = strings.ToLower(strings.TrimSpace(lang))
		if lang == "" || lang == string(types.VerificationLanguageUnknown) || lang == string(types.VerificationLanguageConfigWorkflow) {
			return
		}
		for _, existing := range out {
			if existing == lang {
				return
			}
		}
		out = append(out, lang)
	}
	add(member.Language)
	for _, raw := range []string{member.File, member.SupportRef, member.Key} {
		for _, lang := range sourceInventoryPathLanguages(raw) {
			add(lang)
		}
	}
	return out
}

func sourceInventoryPathLanguages(raw string) []string {
	var out []string
	for _, family := range types.VerificationLanguageFamiliesFromPath(raw) {
		lang := strings.ToLower(strings.TrimSpace(string(family)))
		if lang == "" || lang == string(types.VerificationLanguageUnknown) || lang == string(types.VerificationLanguageConfigWorkflow) {
			continue
		}
		seen := false
		for _, existing := range out {
			if existing == lang {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, lang)
		}
	}
	return out
}
