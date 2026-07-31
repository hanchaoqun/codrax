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
