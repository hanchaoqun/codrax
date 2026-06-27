package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceInventoryRequestedUniverseClassCoveredByCompleteZeroLens(observation types.SourceInventoryObservation, class types.SourcePathRole, scopes []string, roles []types.AnswerCandidateRole) bool {
	roles = sourceInventoryRequestedUniverseNormalizeRoles(roles)
	if len(roles) == 0 || class == "" || class == types.SourcePathRoleUnknown || len(scopes) == 0 {
		return false
	}
	for _, role := range roles {
		if !sourceInventoryRequestedUniverseClassCoveredByCompleteZeroLensRole(observation, class, scopes, role) {
			return false
		}
	}
	return true
}

func sourceInventoryRequestedUniverseClassCoveredByCompleteZeroLensRole(observation types.SourceInventoryObservation, class types.SourcePathRole, scopes []string, role types.AnswerCandidateRole) bool {
	covered := map[string]bool{}
	for _, lens := range observation.CompleteLenses {
		if lens.Role != role || lens.Count != 0 || lens.Total != 0 {
			continue
		}
		if !sourceInventoryRequestedUniversePathRolesContain(lens.SourceClasses, class) {
			continue
		}
		for _, scope := range scopes {
			if sourceInventoryRequestedUniverseScopesCover(lens.Scopes, scope) {
				covered[scope] = true
			}
		}
	}
	return len(covered) == len(scopes)
}

func sourceInventoryRequestedUniverseNormalizeRoles(in []types.AnswerCandidateRole) []types.AnswerCandidateRole {
	seen := map[types.AnswerCandidateRole]bool{}
	var out []types.AnswerCandidateRole
	for _, role := range in {
		if role == "" || role == types.AnswerCandidateRoleUnknown || seen[role] {
			continue
		}
		seen[role] = true
		out = append(out, role)
	}
	return out
}

func sourceInventoryRequestedUniversePathRolesContain(classes []types.SourcePathRole, want types.SourcePathRole) bool {
	for _, class := range classes {
		if class == want {
			return true
		}
	}
	return false
}

func sourceInventoryRequestedUniverseScopesCover(scopes []string, target string) bool {
	target = strings.Trim(strings.ReplaceAll(strings.TrimSpace(target), `\`, `/`), "/")
	if target == "" {
		return false
	}
	for _, raw := range scopes {
		scope := strings.Trim(strings.ReplaceAll(strings.TrimSpace(raw), `\`, `/`), "/")
		if scope == "" {
			continue
		}
		if scope == "." || scope == target || strings.HasPrefix(target, scope+"/") {
			return true
		}
	}
	return false
}
