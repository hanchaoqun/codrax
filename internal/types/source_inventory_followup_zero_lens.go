package types

func sourceInventoryCompleteLensCoveredPathRoles(observation SourceInventoryObservation, roles []AnswerCandidateRole) []SourcePathRole {
	if len(observation.CompleteLenses) == 0 || len(observation.SourceClasses) == 0 {
		return nil
	}
	roleSet := map[AnswerCandidateRole]bool{}
	for _, role := range normalizeSourceInventoryFollowupRoles(roles) {
		roleSet[role] = true
	}
	var out []SourcePathRole
	for _, class := range observation.SourceClasses {
		if class.Role == SourcePathRoleUnknown || class.Count <= 0 {
			continue
		}
		if sourceInventoryCompleteLensCoversSourceClass(observation.CompleteLenses, roleSet, class) {
			out = append(out, class.Role)
		}
	}
	return normalizeSourceInventoryPathRoles(out)
}

func sourceInventoryCompleteLensCoversSourceClass(lenses []SourceInventoryCompleteLens, roles map[AnswerCandidateRole]bool, class SourceInventorySourceClassCount) bool {
	scopes := sourceInventoryClassFollowupScopes(class)
	if len(scopes) == 0 {
		return false
	}
	if len(roles) > 0 {
		for role := range roles {
			if !sourceInventoryCompleteLensCoversSourceClassRole(lenses, role, class.Role, scopes) {
				return false
			}
		}
		return true
	}
	return sourceInventoryCompleteLensCoversSourceClassRole(lenses, AnswerCandidateRoleUnknown, class.Role, scopes)
}

func sourceInventoryCompleteLensCoversSourceClassRole(lenses []SourceInventoryCompleteLens, requiredRole AnswerCandidateRole, class SourcePathRole, scopes []string) bool {
	covered := map[string]bool{}
	for _, lens := range lenses {
		lens = normalizeSourceInventoryCompleteLens(lens)
		if lens.Role == "" || lens.Role == AnswerCandidateRoleUnknown {
			continue
		}
		if requiredRole != "" && requiredRole != AnswerCandidateRoleUnknown && lens.Role != requiredRole {
			continue
		}
		if lens.Count != 0 || lens.Total != 0 {
			continue
		}
		if !sourceInventoryPathRolesContain(lens.SourceClasses, class) {
			continue
		}
		for _, scope := range scopes {
			if sourceInventoryObservationScopesCover(lens.Scopes, []string{scope}) {
				covered[scope] = true
			}
		}
	}
	return len(covered) == len(scopes)
}

func sourceInventoryPathRolesContain(classes []SourcePathRole, want SourcePathRole) bool {
	if want == "" || want == SourcePathRoleUnknown {
		return false
	}
	for _, class := range normalizeSourceInventoryPathRoles(classes) {
		if class == want {
			return true
		}
	}
	return false
}
