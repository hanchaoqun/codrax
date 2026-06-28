package types

import (
	"path"
	"strings"
)

func sourceInventoryMissingClassScopes(classes []SourceInventorySourceClassCount, covered map[SourcePathRole]bool) ([]SourcePathRole, []string) {
	return sourceInventoryMissingClassScopesForLanguages(classes, covered, nil)
}

func sourceInventoryMissingClassScopesForLanguages(classes []SourceInventorySourceClassCount, covered map[SourcePathRole]bool, targetLanguages map[string]bool) ([]SourcePathRole, []string) {
	var missing []SourcePathRole
	var scopes []string
	for _, class := range classes {
		if class.Role == SourcePathRoleUnknown || class.Count <= 0 || covered[class.Role] {
			continue
		}
		classScopes := sourceInventoryClassFollowupScopesForLanguages(class, targetLanguages)
		if len(targetLanguages) > 0 && len(classScopes) == 0 {
			continue
		}
		missing = append(missing, class.Role)
		scopes = append(scopes, classScopes...)
	}
	return normalizeSourceInventoryPathRoles(missing), sourceInventoryFollowupScopes(scopes)
}

func sourceInventoryClassFollowupScopes(class SourceInventorySourceClassCount) []string {
	return sourceInventoryClassFollowupScopesForLanguages(class, nil)
}

func sourceInventoryClassFollowupScopesForLanguages(class SourceInventorySourceClassCount, targetLanguages map[string]bool) []string {
	var scopes []string
	for _, sample := range sourceInventoryFollowupClassSamplesForLanguages(class, targetLanguages) {
		if scope := sourceInventoryFollowupScopeForSample(sample); scope != "" {
			scopes = append(scopes, scope)
		}
	}
	if len(targetLanguages) > 0 && len(scopes) == 0 {
		return nil
	}
	return sourceInventoryFollowupScopes(scopes)
}

func sourceInventoryFollowupScopeForSample(sample string) string {
	sample = strings.Trim(strings.ReplaceAll(strings.TrimSpace(sample), `\`, `/`), "/")
	if sample == "" {
		return ""
	}
	dir := path.Dir(sample)
	if dir == "." || dir == "/" {
		return "."
	}
	return dir
}
