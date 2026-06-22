package types

// SourceInventoryProfileCompletionIsSupportOnly reports whether the analyzer
// emitted a source-inventory profile for source-code support facts while the
// principal answer is a value/member result outside the source-inventory row
// universe. It is intentionally schema-only: requested_fields, role enums, and
// enum/const-set flags decide the boundary, never request wording or model
// rationale.
func SourceInventoryProfileCompletionIsSupportOnly(profile *SourceInventoryProfile) bool {
	if profile == nil || !profile.Active() || !profile.RequestsField(SourceInventoryFieldValues) {
		return false
	}
	underlying := profile.TypeUnderlying
	if underlying == "" {
		underlying = SourceInventoryTypeUnderlyingUnknown
	}
	if profile.RequiresConstSet || underlying != SourceInventoryTypeUnderlyingUnknown {
		return false
	}
	roles := profile.PrincipalTargetRoles()
	if len(roles) == 0 {
		return false
	}
	for _, role := range roles {
		switch role {
		case AnswerCandidateRoleFunction,
			AnswerCandidateRoleMethod,
			AnswerCandidateRoleType:
			continue
		default:
			return false
		}
	}
	return true
}
