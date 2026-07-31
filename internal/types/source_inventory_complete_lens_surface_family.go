package types

import "sort"

func normalizeSourceInventoryCompleteLensSurfaceFamilies(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range in {
		family := SourceInventorySurfaceTermKey(raw)
		if family != "" && !seen[family] {
			seen[family] = true
			out = append(out, family)
		}
	}
	sort.Strings(out)
	return out
}

func sourceInventoryCompleteLensPopulateSurface(lens *SourceInventoryCompleteLens, scopes []string, members []SourceInventoryObservationMember) {
	if lens == nil {
		return
	}
	langs := map[string]bool{}
	classes := map[SourcePathRole]bool{}
	surfaceFamilies := map[string]bool{}
	for _, scope := range scopes {
		sourceInventoryCompleteLensAddScope(scope, classes)
	}
	for _, member := range members {
		sourceInventoryCompleteLensAddMember(lens.Role, member, langs, classes, surfaceFamilies)
	}
	for lang := range langs {
		lens.Languages = append(lens.Languages, lang)
	}
	for class := range classes {
		lens.SourceClasses = append(lens.SourceClasses, class)
	}
	for family := range surfaceFamilies {
		lens.SurfaceFamilies = append(lens.SurfaceFamilies, family)
	}
}
