package tool

import "github.com/hanchaoqun/codrax/internal/types"

func sourceInventoryExactUniverseRoleCanBlock(role types.AnswerCandidateRole, rm *types.RequestModel) bool {
	if role == "" || role == types.AnswerCandidateRoleUnknown {
		return false
	}
	if rm == nil || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return true
	}
	roles := rm.SourceInventoryProfile.PrincipalTargetRoles()
	if len(roles) == 0 {
		roles = rm.SourceInventoryProfile.TargetRoles
	}
	for _, requested := range roles {
		if requested == role {
			return true
		}
	}
	return false
}

func sourceInventoryExactUniverseRoleCanProveClosure(role types.AnswerCandidateRole, rm *types.RequestModel) bool {
	if sourceInventoryExactUniverseRoleCanBlock(role, rm) {
		return true
	}
	if rm == nil || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return true
	}
	return role != types.AnswerCandidateRoleFile
}
