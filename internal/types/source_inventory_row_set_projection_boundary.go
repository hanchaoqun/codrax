package types

func sourceInventoryPrincipalRowSetProjectionDisabled(observation SourceInventoryObservation, rm RequestModel) bool {
	return rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() || HasTypedRelationMemberSetShape(rm) ||
		!sourceInventoryCanProjectCompletePrincipalRowSet(observation, rm)
}

func sourceInventoryCanProjectCompletePrincipalRowSet(observation SourceInventoryObservation, rm RequestModel) bool {
	observation = CloneSourceInventoryObservation(observation)
	if len(observation.Sets) == 0 {
		return false
	}
	roles := sourceInventoryRowSetPrincipalRoles(rm, observation)
	if len(roles) == 0 {
		return false
	}
	roleSet := map[AnswerCandidateRole]bool{}
	for _, role := range roles {
		roleSet[role] = true
	}
	seenPrincipalRows := false
	for _, set := range observation.Sets {
		role := sourceInventoryObservationSetEffectiveRole(set)
		if !roleSet[role] || len(set.Members) == 0 {
			continue
		}
		if !set.Complete {
			return false
		}
		seenPrincipalRows = true
	}
	return seenPrincipalRows
}

func sourceInventoryObservationSetEffectiveRole(set SourceInventoryObservationSet) AnswerCandidateRole {
	if set.Role != "" && set.Role != AnswerCandidateRoleUnknown {
		return set.Role
	}
	for _, member := range set.Members {
		if member.Role != "" && member.Role != AnswerCandidateRoleUnknown {
			return member.Role
		}
	}
	return AnswerCandidateRoleUnknown
}
