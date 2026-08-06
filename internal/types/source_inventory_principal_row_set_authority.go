package types

func sourceInventoryRowSetPrincipalRoles(rm RequestModel, observation SourceInventoryObservation) []AnswerCandidateRole {
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		if roles := rm.SourceInventoryProfile.PrincipalTargetRoles(); len(roles) > 0 {
			return normalizeSourceInventoryFollowupRoles(roles)
		}
		return normalizeSourceInventoryFollowupRoles(rm.SourceInventoryProfile.TargetRoles)
	}
	var roles []AnswerCandidateRole
	for _, set := range observation.Sets {
		if set.Role != "" && set.Role != AnswerCandidateRoleUnknown {
			roles = append(roles, set.Role)
		}
	}
	return normalizeSourceInventoryFollowupRoles(roles)
}

// sourceInventoryRowsByPrincipalAuthority makes the shared row-set constructor
// obey the same authority decision as completion and finalization. A
// support-only inventory may still provide citations/navigation, but a
// complete parser observation cannot promote it back into the principal answer
// universe. Centralizing this here covers projection, pre-emit checks,
// candidate gaps, and authority snapshots without caller-specific guards.
func sourceInventoryRowsByPrincipalAuthority(
	rm RequestModel,
	principal []SourceInventoryRow,
	support []SourceInventoryRow,
) ([]SourceInventoryRow, []SourceInventoryRow) {
	if SourceInventoryPrincipalAuthorityActive(rm) {
		return principal, support
	}
	return nil, append(support, principal...)
}
