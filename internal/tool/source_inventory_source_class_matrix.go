package tool

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const sourceInventorySourceClassSampleLimit = 4

func sourceInventorySourceClassUniverseForTrackedFiles(files []SourceInventoryTrackedSourceFile, complete bool) []types.SourceInventorySourceClassCount {
	if len(files) == 0 {
		return nil
	}
	counts := map[types.SourcePathRole]int{}
	samples := map[types.SourcePathRole][]string{}
	languageCounts := map[types.SourcePathRole]map[string]int{}
	languageSamples := map[types.SourcePathRole]map[string][]string{}
	for _, file := range files {
		if strings.TrimSpace(file.Path) == "" || file.Role == types.SourcePathRoleUnknown || strings.TrimSpace(file.Language) == "" {
			continue
		}
		counts[file.Role]++
		if len(samples[file.Role]) < sourceInventorySourceClassSampleLimit {
			samples[file.Role] = append(samples[file.Role], file.Path)
		}
		if languageCounts[file.Role] == nil {
			languageCounts[file.Role] = map[string]int{}
		}
		languageCounts[file.Role][file.Language]++
		if languageSamples[file.Role] == nil {
			languageSamples[file.Role] = map[string][]string{}
		}
		if len(languageSamples[file.Role][file.Language]) < sourceInventorySourceClassSampleLimit {
			languageSamples[file.Role][file.Language] = append(languageSamples[file.Role][file.Language], file.Path)
		}
	}
	if len(counts) == 0 {
		return nil
	}
	return sourceInventoryOrderedSourceClassCounts(counts, samples, languageCounts, languageSamples, complete)
}

func sourceInventoryOrderedSourceClassCounts(
	counts map[types.SourcePathRole]int,
	samples map[types.SourcePathRole][]string,
	languageCounts map[types.SourcePathRole]map[string]int,
	languageSamples map[types.SourcePathRole]map[string][]string,
	complete bool,
) []types.SourceInventorySourceClassCount {
	order := []types.SourcePathRole{
		types.SourcePathRoleProduction,
		types.SourcePathRoleTest,
		types.SourcePathRoleFixture,
		types.SourcePathRoleExample,
		types.SourcePathRoleDocumentation,
		types.SourcePathRolePromptSupport,
		types.SourcePathRoleThirdParty,
		types.SourcePathRoleVendor,
		types.SourcePathRoleGenerated,
	}
	var out []types.SourceInventorySourceClassCount
	add := func(role types.SourcePathRole) {
		count := counts[role]
		if count <= 0 {
			return
		}
		out = append(out, types.SourceInventorySourceClassCount{
			Role:       role,
			Count:      count,
			Complete:   complete,
			Samples:    append([]string(nil), samples[role]...),
			Languages:  sourceInventorySourceClassLanguageCounts(languageCounts[role], languageSamples[role]),
			Provenance: []string{"source_class_universe:git_tracked_or_filesystem"},
		})
	}
	for _, role := range order {
		add(role)
		delete(counts, role)
	}
	var rest []string
	for role := range counts {
		rest = append(rest, string(role))
	}
	sort.Strings(rest)
	for _, raw := range rest {
		role := types.SourcePathRole(raw)
		add(role)
	}
	return out
}

func sourceInventorySourceClassLanguageCounts(counts map[string]int, samples map[string][]string) []types.SourceInventoryLanguageCount {
	if len(counts) == 0 {
		return nil
	}
	langs := make([]string, 0, len(counts))
	for lang, count := range counts {
		if strings.TrimSpace(lang) == "" || count <= 0 {
			continue
		}
		langs = append(langs, lang)
	}
	sort.SliceStable(langs, func(i, j int) bool {
		if counts[langs[i]] != counts[langs[j]] {
			return counts[langs[i]] > counts[langs[j]]
		}
		return langs[i] < langs[j]
	})
	out := make([]types.SourceInventoryLanguageCount, 0, len(langs))
	for _, lang := range langs {
		out = append(out, types.SourceInventoryLanguageCount{
			Language: lang,
			Count:    counts[lang],
			InScope:  true,
			Samples:  append([]string(nil), samples[lang]...),
		})
	}
	return out
}
