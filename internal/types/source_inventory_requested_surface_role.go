package types

func sourceInventorySymbolRoleRequiresRequestedSurfaceFamily(role AnswerCandidateRole) bool {
	switch role {
	case AnswerCandidateRoleType,
		AnswerCandidateRoleFunction,
		AnswerCandidateRoleMethod,
		AnswerCandidateRoleField,
		AnswerCandidateRoleConstant,
		AnswerCandidateRoleVariable,
		AnswerCandidateRoleRoute:
		return true
	default:
		return false
	}
}
