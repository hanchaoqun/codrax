package types

import "strings"

func sourceInventoryObservationSupersedesPrior(prior, current SourceInventoryObservation) bool {
	if !current.IsActive() || !sourceInventoryObservationStructurallyComplete(current) {
		return false
	}
	if !sourceInventoryObservationScopesCover(current.Scopes, prior.Scopes) {
		return false
	}
	currentRoles := sourceInventoryObservationRoleSet(current.Sets)
	if len(currentRoles) == 0 {
		return len(prior.Sets) == 0
	}
	for _, set := range prior.Sets {
		if set.Role == "" || set.Role == AnswerCandidateRoleUnknown || !sourceInventoryObservationSetBlocksCompleteness(set) {
			continue
		}
		if !currentRoles[set.Role] {
			return false
		}
	}
	return true
}

func sourceInventoryObservationRoleSet(sets []SourceInventoryObservationSet) map[AnswerCandidateRole]bool {
	out := map[AnswerCandidateRole]bool{}
	for _, set := range sets {
		if set.Role == "" || set.Role == AnswerCandidateRoleUnknown {
			continue
		}
		out[set.Role] = true
	}
	return out
}

func sourceInventoryObservationScopesCover(current, prior []string) bool {
	current = sourceInventoryNormalizeScopes(current)
	prior = sourceInventoryNormalizeScopes(prior)
	if len(prior) == 0 {
		return true
	}
	if len(current) == 0 {
		return false
	}
	currentSet := map[string]bool{}
	for _, scope := range current {
		if scope == "." {
			return true
		}
		currentSet[scope] = true
	}
	for _, scope := range prior {
		if currentSet[scope] {
			continue
		}
		covered := false
		for currentScope := range currentSet {
			if sourceInventoryScopeCovers(currentScope, scope) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func sourceInventoryNormalizeScopes(scopes []string) []string {
	out := make([]string, 0, len(scopes))
	seen := map[string]bool{}
	for _, raw := range scopes {
		scope := strings.Trim(strings.ReplaceAll(strings.TrimSpace(raw), `\`, `/`), "/")
		if scope == "" {
			scope = "."
		}
		if seen[scope] {
			continue
		}
		seen[scope] = true
		out = append(out, scope)
	}
	return out
}

func sourceInventoryScopeCovers(parent, child string) bool {
	parent = strings.Trim(strings.ReplaceAll(strings.TrimSpace(parent), `\`, `/`), "/")
	child = strings.Trim(strings.ReplaceAll(strings.TrimSpace(child), `\`, `/`), "/")
	if parent == "" {
		parent = "."
	}
	if child == "" {
		child = "."
	}
	if parent == "." {
		return true
	}
	if parent == child {
		return true
	}
	return strings.HasPrefix(child, parent+"/")
}

func sourceInventoryObservationMergedSetComplete(previous, incoming, merged SourceInventoryObservationSet) bool {
	if incoming.Complete && sourceInventoryObservationSetCoversMerged(incoming, merged) {
		return true
	}
	if previous.Complete && sourceInventoryObservationSetCoversMerged(previous, merged) {
		return true
	}
	if sourceInventoryObservationSetCompleteEnough(previous) && sourceInventoryObservationSetCompleteEnough(incoming) {
		return true
	}
	return !sourceInventoryObservationSetBlocksCompleteness(merged)
}

func sourceInventoryObservationSetCoversMerged(candidate, merged SourceInventoryObservationSet) bool {
	total := candidate.Total
	if total < len(candidate.Members) {
		total = len(candidate.Members)
	}
	mergedTotal := merged.Total
	if mergedTotal < len(merged.Members) {
		mergedTotal = len(merged.Members)
	}
	return total >= mergedTotal
}

func sourceInventoryObservationSetCompleteEnough(set SourceInventoryObservationSet) bool {
	return set.Complete || !sourceInventoryObservationSetBlocksCompleteness(set)
}

func sourceInventoryObservationStructurallyComplete(observation SourceInventoryObservation) bool {
	if !observation.IsActive() {
		return false
	}
	if observation.Execution != nil && observation.Execution.CandidateBudgetTruncated {
		return false
	}
	if observation.Page != nil && (!observation.Page.Complete || strings.TrimSpace(observation.Page.NextCursor) != "") {
		return false
	}
	for _, class := range observation.SourceClasses {
		if class.Role == SourcePathRoleUnknown || class.Count <= 0 {
			continue
		}
		if !class.Complete {
			return false
		}
	}
	for _, set := range observation.Sets {
		if sourceInventoryObservationSetBlocksCompleteness(set) {
			return false
		}
	}
	if len(observation.Sets) > 0 {
		return true
	}
	if len(observation.SourceClasses) > 0 {
		return SourceInventorySourceClassesComplete(observation.SourceClasses)
	}
	return observation.Complete
}

func sourceInventoryObservationSetBlocksCompleteness(set SourceInventoryObservationSet) bool {
	if set.Complete {
		return false
	}
	if set.Count <= 0 && set.Total <= 0 && len(set.Members) == 0 {
		return false
	}
	return true
}

func sourceInventoryMergedClassComplete(existing, incoming SourceInventorySourceClassCount) bool {
	if incoming.Complete && incoming.Count >= existing.Count {
		return true
	}
	if existing.Complete && existing.Count >= incoming.Count {
		return true
	}
	return existing.Complete && incoming.Complete
}
