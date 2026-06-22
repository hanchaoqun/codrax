package types

import "strings"

func sourceInventoryCompletionObservationIncomplete(observation SourceInventoryObservation) bool {
	if !observation.Complete {
		return true
	}
	if observation.Execution != nil && observation.Execution.CandidateBudgetTruncated {
		return true
	}
	if observation.Page != nil && (!observation.Page.Complete || strings.TrimSpace(observation.Page.NextCursor) != "") {
		return true
	}
	for _, set := range observation.Sets {
		if !set.Complete {
			return true
		}
	}
	return false
}

func sourceInventoryCompletionIncompleteSets(observation SourceInventoryObservation) []AnswerCandidateRole {
	var out []AnswerCandidateRole
	for _, set := range observation.Sets {
		if set.Complete || set.Role == "" || set.Role == AnswerCandidateRoleUnknown {
			continue
		}
		out = append(out, set.Role)
	}
	return normalizeSourceInventoryFollowupRoles(out)
}

func sourceInventoryCompletionRoles(observation SourceInventoryObservation, rm RequestModel) []AnswerCandidateRole {
	roles := sourceInventoryFollowupPrincipalRoles(observation, rm)
	if len(roles) > 0 {
		return roles
	}
	for _, set := range observation.Sets {
		if set.Role == "" || set.Role == AnswerCandidateRoleUnknown {
			continue
		}
		roles = append(roles, set.Role)
	}
	return normalizeSourceInventoryFollowupRoles(roles)
}
