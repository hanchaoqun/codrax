package tool

import (
	"path"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceInventoryRequestedUniverseFollowupDebt(ctx *types.BusContext, observation types.SourceInventoryObservation, rm types.RequestModel, facts []types.AnswerAggregateFact) types.SourceInventoryFollowupDebt {
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return types.SourceInventoryFollowupDebt{}
	}
	covered := sourceInventoryAggregatePrincipalSourceFamily(ctx, facts, &rm)
	if len(covered.languages) == 0 {
		return types.SourceInventoryFollowupDebt{}
	}
	var missingClasses []types.SourcePathRole
	var scopes []string
	for _, class := range observation.SourceClasses {
		if class.Role == types.SourcePathRoleUnknown || class.Count <= 0 || covered.classes[class.Role] {
			continue
		}
		classScopes := sourceInventoryRequestedUniverseClassFollowupScopes(class, covered.languages)
		if len(classScopes) == 0 {
			continue
		}
		missingClasses = append(missingClasses, class.Role)
		scopes = append(scopes, classScopes...)
	}
	if len(missingClasses) == 0 || len(scopes) == 0 {
		return types.SourceInventoryFollowupDebt{}
	}
	roles := rm.SourceInventoryProfile.PrincipalTargetRoles()
	return types.NormalizeSourceInventoryFollowupDebt(types.SourceInventoryFollowupDebt{
		Active:           true,
		ReasonCode:       types.SourceInventoryFollowupDebtMissingSourceClass,
		Query:            sourceInventoryRequestedUniverseFollowupQuery(scopes, roles),
		MissingClasses:   missingClasses,
		CoveredClasses:   sourceInventoryRequestedUniverseCoveredClasses(covered.classes),
		MissingLanguages: sourceInventoryRequestedUniverseCoveredLanguages(covered.languages),
		Roles:            roles,
	})
}

func sourceInventoryRequestedUniverseFollowupQuery(scopes []string, roles []types.AnswerCandidateRole) types.SourceInventoryLensQuery {
	return types.SourceInventoryLensQuery{
		Path:              ".",
		Scopes:            sourceInventoryRequestedUniverseFollowupScopes(scopes),
		Roles:             roles,
		IncludeCounts:     true,
		IncludeAttributes: false,
		TopN:              24,
	}
}

func sourceInventoryRequestedUniverseClassFollowupScopes(class types.SourceInventorySourceClassCount, languages map[string]bool) []string {
	var samples []string
	for _, language := range class.Languages {
		lang := strings.ToLower(strings.TrimSpace(language.Language))
		if lang == "" || language.Count <= 0 || !languages[lang] {
			continue
		}
		samples = append(samples, language.Samples...)
	}
	if len(class.Languages) == 0 && sourceInventoryClassSamplesIntersectLanguages(class.Samples, languages) {
		samples = append(samples, class.Samples...)
	}
	return sourceInventoryRequestedUniverseSampleScopes(samples)
}

func sourceInventoryRequestedUniverseSampleScopes(samples []string) []string {
	var scopes []string
	for _, sample := range samples {
		scope := sourceInventoryRequestedUniverseScopeForSample(sample)
		if scope != "" {
			scopes = append(scopes, scope)
		}
	}
	return sourceInventoryRequestedUniverseFollowupScopes(scopes)
}

func sourceInventoryRequestedUniverseScopeForSample(sample string) string {
	sample = strings.Trim(strings.ReplaceAll(strings.TrimSpace(sample), `\`, `/`), "/")
	if sample == "" {
		return ""
	}
	dir := path.Dir(sample)
	if dir == "" || dir == "." || dir == "/" {
		return "."
	}
	return dir
}

func sourceInventoryRequestedUniverseFollowupScopes(scopes []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(scopes))
	for _, raw := range scopes {
		scope := strings.Trim(strings.ReplaceAll(strings.TrimSpace(raw), `\`, `/`), "/")
		if scope == "" {
			continue
		}
		if seen[scope] {
			continue
		}
		seen[scope] = true
		out = append(out, scope)
	}
	return out
}

func sourceInventoryRequestedUniverseCoveredClasses(classes map[types.SourcePathRole]bool) []types.SourcePathRole {
	out := make([]types.SourcePathRole, 0, len(classes))
	for class := range classes {
		out = append(out, class)
	}
	return out
}

func sourceInventoryRequestedUniverseCoveredLanguages(languages map[string]bool) []string {
	out := make([]string, 0, len(languages))
	for lang := range languages {
		out = append(out, lang)
	}
	return out
}
