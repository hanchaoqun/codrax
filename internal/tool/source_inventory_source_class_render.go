package tool

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func renderSourceInventorySourceClassCounts(classes []types.SourceInventorySourceClassCount) string {
	if len(classes) == 0 {
		return ""
	}
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
	byRole := make(map[types.SourcePathRole]types.SourceInventorySourceClassCount, len(classes))
	for _, class := range classes {
		if class.Role == types.SourcePathRoleUnknown || class.Count <= 0 {
			continue
		}
		byRole[class.Role] = class
	}
	var parts []string
	for _, role := range order {
		class, ok := byRole[role]
		if !ok {
			continue
		}
		delete(byRole, role)
		parts = append(parts, renderSourceInventorySourceClassCount(class))
	}
	var rest []string
	for role := range byRole {
		rest = append(rest, string(role))
	}
	sort.Strings(rest)
	for _, role := range rest {
		parts = append(parts, renderSourceInventorySourceClassCount(byRole[types.SourcePathRole(role)]))
	}
	return strings.Join(parts, ",")
}

func renderSourceInventorySourceClassCount(class types.SourceInventorySourceClassCount) string {
	item := fmt.Sprintf("%s:%d", class.Role, class.Count)
	if languages := renderSourceInventorySourceClassLanguages(class.Languages); languages != "" {
		item += "[" + languages + "]"
	}
	if !class.Complete {
		item += "(partial)"
	}
	return item
}

func renderSourceInventorySourceClassLanguages(in []types.SourceInventoryLanguageCount) string {
	if len(in) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, lang := range in {
		name := strings.TrimSpace(lang.Language)
		if name == "" || lang.Count <= 0 {
			continue
		}
		if lang.Count > counts[name] {
			counts[name] = lang.Count
		}
	}
	if len(counts) == 0 {
		return ""
	}
	order := make([]string, 0, len(counts))
	for lang := range counts {
		order = append(order, lang)
	}
	sort.SliceStable(order, func(i, j int) bool {
		if counts[order[i]] != counts[order[j]] {
			return counts[order[i]] > counts[order[j]]
		}
		return order[i] < order[j]
	})
	const limit = 3
	if len(order) > limit {
		order = order[:limit]
	}
	parts := make([]string, 0, len(order)+1)
	for _, lang := range order {
		parts = append(parts, fmt.Sprintf("%s:%d", lang, counts[lang]))
	}
	if len(counts) > limit {
		parts = append(parts, "...")
	}
	return strings.Join(parts, ",")
}
