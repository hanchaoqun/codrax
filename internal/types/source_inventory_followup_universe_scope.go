package types

func sourceInventoryTargetLanguageUniverseScopes(classes []SourceInventorySourceClassCount, primary []string, targetLanguages map[string]bool) []string {
	primary = normalizeSourceInventoryFollowupStrings(primary)
	if len(targetLanguages) == 0 {
		return sourceInventoryFollowupScopes(primary)
	}
	scopes := append([]string(nil), primary...)
	for _, class := range classes {
		if class.Role == SourcePathRoleUnknown || class.Count <= 0 {
			continue
		}
		for _, scope := range sourceInventoryClassFollowupScopesForLanguages(class, targetLanguages) {
			scopes = append(scopes, scope)
		}
	}
	return sourceInventoryFollowupScopes(scopes)
}
