package types

import (
	"path"
	"strings"
)

func sourceInventoryRequestedSurfaceFamilyBackedByCompleteLens(observation SourceInventoryObservation, rm RequestModel, roles []AnswerCandidateRole) bool {
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() ||
		len(rm.SourceInventoryProfile.SourceQuotes) == 0 ||
		len(observation.CompleteLenses) == 0 ||
		len(roles) == 0 {
		return false
	}
	roleSet := map[AnswerCandidateRole]bool{}
	for _, role := range roles {
		if role != "" && role != AnswerCandidateRoleUnknown {
			roleSet[role] = true
		}
	}
	if len(roleSet) == 0 {
		return false
	}
	scope, _ := sourceInventoryRowSetPrincipalScope(rm)
	var principal []SourceInventoryRow
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			row := sourceInventoryRowFromMember(member, set.Role, roleSet, scope)
			if row.Lane == SourceInventoryRowLanePrincipal {
				principal = append(principal, row)
			}
		}
	}
	if len(principal) == 0 {
		return false
	}
	requested := sourceInventoryRequestedSurfaceFamiliesByRole(rm.SourceInventoryProfile.SourceQuotes, principal)
	if len(requested) == 0 {
		return false
	}
	seenRequestedRow := false
	for _, row := range principal {
		family, ok := sourceInventoryProjectionSurfaceFamilyForRow(row)
		if !ok || !requested[row.Role][family.family] {
			continue
		}
		seenRequestedRow = true
		if !sourceInventoryRowBackedByCompleteLens(row, observation.CompleteLenses) {
			return false
		}
	}
	return seenRequestedRow
}

func sourceInventoryRowBackedByCompleteLens(row SourceInventoryRow, lenses []SourceInventoryCompleteLens) bool {
	if row.Role == "" || row.Role == AnswerCandidateRoleUnknown {
		return false
	}
	for _, lens := range lenses {
		lens = normalizeSourceInventoryCompleteLens(lens)
		if lens.Role != row.Role {
			continue
		}
		if sourceInventoryCompleteLensCoversRow(lens, row) {
			return true
		}
	}
	return false
}

func sourceInventoryCompleteLensCoversRow(lens SourceInventoryCompleteLens, row SourceInventoryRow) bool {
	matched := false
	lang := sourceInventoryRequestedSurfaceRowLanguage(row)
	if len(lens.Languages) > 0 {
		if lang == "" || !sourceInventoryCompleteLensStringContains(lens.Languages, lang) {
			return false
		}
		matched = true
	}
	class := row.SourceClass
	if class == "" || class == SourcePathRoleUnknown {
		class = ClassifySourcePathRole(row.Member.File)
	}
	if len(lens.SourceClasses) > 0 {
		if class == "" || class == SourcePathRoleUnknown || !sourceInventoryCompleteLensClassContains(lens.SourceClasses, class) {
			return false
		}
		matched = true
	}
	if len(lens.Scopes) > 0 {
		file := sourceInventoryCompleteLensRowFile(row)
		if file != "" && !sourceInventoryCompleteLensScopesCoverFile(lens.Scopes, file) {
			return false
		}
	}
	return matched
}

func sourceInventoryRequestedSurfaceRowLanguage(row SourceInventoryRow) string {
	lang := sourceInventoryRequestedSurfaceTextKey(row.Language)
	if lang != "" && lang != string(VerificationLanguageUnknown) && lang != string(VerificationLanguageConfigWorkflow) {
		return lang
	}
	for _, family := range VerificationLanguageFamiliesFromPath(row.Member.File) {
		lang = sourceInventoryRequestedSurfaceTextKey(string(family))
		if lang != "" && lang != string(VerificationLanguageUnknown) && lang != string(VerificationLanguageConfigWorkflow) {
			return lang
		}
	}
	return ""
}

func sourceInventoryCompleteLensStringContains(values []string, want string) bool {
	want = sourceInventoryRequestedSurfaceTextKey(want)
	for _, value := range values {
		if sourceInventoryRequestedSurfaceTextKey(value) == want {
			return true
		}
	}
	return false
}

func sourceInventoryCompleteLensClassContains(values []SourcePathRole, want SourcePathRole) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sourceInventoryCompleteLensRowFile(row SourceInventoryRow) string {
	if row.Member.File != "" {
		return row.Member.File
	}
	if _, loc, ok := ParseAnswerSupportRefMemberLocation(row.Member.SupportRef); ok {
		return loc.File
	}
	return ""
}

func sourceInventoryCompleteLensScopesCoverFile(scopes []string, file string) bool {
	file = sourceInventoryCompleteLensCleanPath(file)
	if file == "" {
		return true
	}
	for _, scope := range scopes {
		scope = sourceInventoryCompleteLensCleanPath(scope)
		if scope == "" || scope == "." {
			return true
		}
		if sourceInventoryCompleteLensPathHasPrefix(file, scope) {
			return true
		}
	}
	return false
}

func sourceInventoryCompleteLensCleanPath(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return "."
	}
	cleaned := path.Clean(raw)
	if cleaned == "." || cleaned == "/" {
		return "."
	}
	return strings.Trim(cleaned, "/")
}

func sourceInventoryCompleteLensPathHasPrefix(file, scope string) bool {
	if file == scope {
		return true
	}
	if scope == "" || scope == "." {
		return true
	}
	return strings.HasPrefix(file, scope+"/")
}
