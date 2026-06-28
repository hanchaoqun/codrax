package types

// NormalizeSourceInventoryDisplayAttributeRoles removes target_roles that are
// already requested as per-row display attributes when the same inventory also
// has real structural principal roles. This keeps declarations such as
// "functions/types with package path" from turning package/module/namespace
// into extra principal answer rows. A package-only inventory still remains a
// package inventory.
func NormalizeSourceInventoryDisplayAttributeRoles(profile *SourceInventoryProfile) []AnswerCandidateRole {
	if profile == nil || !profile.Active() {
		return nil
	}
	if !profile.requestsPackageLikeDisplayField() {
		return nil
	}
	if !profile.hasStructuralPrincipalRole() {
		return nil
	}
	out := make([]AnswerCandidateRole, 0, len(profile.TargetRoles))
	var removed []AnswerCandidateRole
	for _, role := range profile.TargetRoles {
		if sourceInventoryDisplayAttributeRole(role) {
			removed = append(removed, role)
			continue
		}
		out = append(out, role)
	}
	if len(removed) == 0 || len(out) == 0 {
		return nil
	}
	profile.TargetRoles = out
	return removed
}

func (p *SourceInventoryProfile) hasStructuralPrincipalRole() bool {
	if p == nil {
		return false
	}
	for _, role := range p.TargetRoles {
		if role == AnswerCandidateRoleUnknown || sourceInventoryDisplayAttributeRole(role) {
			continue
		}
		return true
	}
	return false
}

func (p *SourceInventoryProfile) requestsPackageLikeDisplayField() bool {
	return p != nil &&
		(p.RequestsField(SourceInventoryFieldPackage) ||
			p.RequestsField(SourceInventoryFieldModule) ||
			p.RequestsField(SourceInventoryFieldNamespace))
}

func sourceInventoryDisplayAttributeRole(role AnswerCandidateRole) bool {
	return role == AnswerCandidateRolePackage
}
