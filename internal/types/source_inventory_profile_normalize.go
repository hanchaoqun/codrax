package types

// NormalizeSourceInventoryRequestedFieldsForAnswerSubject removes contradictory
// requested-field drift after the answer subject has been inferred. The
// decision consumes only typed subject/profile structure, never raw request
// wording or model rationale. A name/location inventory asks for declaration
// identities; `values` is reserved for literal/member values or source text and
// must not demote a mechanical row-set to support-only when the typed subject is
// already an identity lane.
func NormalizeSourceInventoryRequestedFieldsForAnswerSubject(profile *SourceInventoryProfile, answerSubject AnswerSubject) bool {
	if profile == nil || !profile.Active() || !profile.RequestsField(SourceInventoryFieldValues) {
		return false
	}
	if !sourceInventoryValuesFieldConflictsWithAnswerSubject(profile, answerSubject) {
		return false
	}
	fields := make([]SourceInventoryRequestedField, 0, len(profile.RequestedFields))
	removed := false
	for _, field := range profile.RequestedFields {
		if field == SourceInventoryFieldValues {
			removed = true
			continue
		}
		fields = append(fields, field)
	}
	if !removed {
		return false
	}
	profile.RequestedFields = fields
	return true
}

func sourceInventoryValuesFieldConflictsWithAnswerSubject(profile *SourceInventoryProfile, answerSubject AnswerSubject) bool {
	if profile == nil || !profile.Active() {
		return false
	}
	principalRoles := profile.PrincipalTargetRoles()
	if len(principalRoles) == 0 {
		return false
	}
	if answerSubject.Kind == SubjectTypeName &&
		profile.TypeUnderlying == SourceInventoryTypeUnderlyingString &&
		profile.RequiresConstSet &&
		sourceInventoryPrincipalRolesAll(principalRoles, AnswerCandidateRoleType) {
		return true
	}
	if sourceInventoryProfileHasValueBearingFacet(profile) {
		return false
	}
	switch answerSubject.Kind {
	case SubjectFunctionName:
		return sourceInventoryPrincipalRolesAll(principalRoles, AnswerCandidateRoleFunction, AnswerCandidateRoleMethod)
	case SubjectTypeName:
		return sourceInventoryPrincipalRolesAll(principalRoles, AnswerCandidateRoleType)
	default:
		return false
	}
}

func sourceInventoryProfileHasValueBearingFacet(profile *SourceInventoryProfile) bool {
	if profile == nil {
		return false
	}
	return profile.RequiresConstSet ||
		(profile.TypeUnderlying != "" && profile.TypeUnderlying != SourceInventoryTypeUnderlyingUnknown)
}

func sourceInventoryPrincipalRolesAll(roles []AnswerCandidateRole, allowed ...AnswerCandidateRole) bool {
	if len(roles) == 0 || len(allowed) == 0 {
		return false
	}
	allowedSet := map[AnswerCandidateRole]bool{}
	for _, role := range allowed {
		allowedSet[role] = true
	}
	for _, role := range roles {
		if !allowedSet[role] {
			return false
		}
	}
	return true
}
