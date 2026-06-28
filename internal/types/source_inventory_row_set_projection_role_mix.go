package types

func sourceInventoryPrincipalRolesAreMixedSymbolUniverse(roles []AnswerCandidateRole) bool {
	roles = normalizeSourceInventoryFollowupRoles(roles)
	if len(roles) >= 3 {
		return true
	}
	if len(roles) <= 1 {
		return false
	}
	seen := map[AnswerCandidateRole]bool{}
	for _, role := range roles {
		seen[role] = true
	}
	if !seen[AnswerCandidateRoleField] {
		return false
	}
	for _, role := range []AnswerCandidateRole{
		AnswerCandidateRoleFunction,
		AnswerCandidateRoleMethod,
		AnswerCandidateRoleType,
		AnswerCandidateRoleConstant,
		AnswerCandidateRoleVariable,
	} {
		if seen[role] {
			return true
		}
	}
	return false
}
