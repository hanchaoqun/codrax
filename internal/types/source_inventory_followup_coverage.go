package types

func sourceInventoryObservedPathRolesForRoles(observation SourceInventoryObservation, roles []AnswerCandidateRole) map[SourcePathRole]bool {
	out := map[SourcePathRole]bool{}
	allowedRoles := sourceInventoryFollowupRoleSet(roles)
	add := func(file string) {
		role := ClassifySourcePathRole(file)
		if role != SourcePathRoleUnknown {
			out[role] = true
		}
	}
	for _, set := range observation.Sets {
		if set.Role == "" || set.Role == AnswerCandidateRoleUnknown || !allowedRoles[set.Role] {
			continue
		}
		for _, member := range set.Members {
			add(member.File)
			for _, attr := range member.Attributes {
				add(attr.File)
			}
		}
	}
	for _, class := range sourceInventoryCompleteLensCoveredPathRoles(observation, roles) {
		out[class] = true
	}
	return out
}

func sourceInventoryFollowupRoleSet(roles []AnswerCandidateRole) map[AnswerCandidateRole]bool {
	out := map[AnswerCandidateRole]bool{}
	for _, role := range normalizeSourceInventoryFollowupRoles(roles) {
		out[role] = true
	}
	return out
}
