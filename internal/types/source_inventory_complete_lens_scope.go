package types

import "strings"

func sourceInventoryCompleteLensAddScope(raw string, classes map[SourcePathRole]bool) {
	raw = strings.Trim(strings.ReplaceAll(strings.TrimSpace(raw), `\`, `/`), "/")
	if raw == "" {
		raw = "."
	}
	if class := ClassifySourcePathRole(raw); class != SourcePathRoleUnknown {
		classes[class] = true
	}
}

func sourceInventoryCompleteLensPopulateSurface(lens *SourceInventoryCompleteLens, scopes []string, members []SourceInventoryObservationMember) {
	if lens == nil {
		return
	}
	langs := map[string]bool{}
	classes := map[SourcePathRole]bool{}
	for _, scope := range scopes {
		sourceInventoryCompleteLensAddScope(scope, classes)
	}
	for _, member := range members {
		sourceInventoryCompleteLensAddMember(lens.Role, member, langs, classes)
	}
	for lang := range langs {
		lens.Languages = append(lens.Languages, lang)
	}
	for class := range classes {
		lens.SourceClasses = append(lens.SourceClasses, class)
	}
}
