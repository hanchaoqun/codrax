package types

import "strings"

func sourceInventoryFollowupReconcileRequiredFileLanguages(required, observed map[string]bool) map[string]bool {
	if len(required) == 0 || len(observed) == 0 {
		return required
	}
	intersection := map[string]bool{}
	for lang := range required {
		if observed[lang] {
			intersection[lang] = true
		}
	}
	if len(intersection) > 0 {
		return intersection
	}
	return nil
}

func sourceInventoryFollowupObservedConstructLanguages(observation SourceInventoryObservation, rm RequestModel, roles []AnswerCandidateRole) map[string]bool {
	profile := rm.SourceInventoryProfile
	if profile == nil || !profile.Active() || len(profile.SourceQuotes) == 0 {
		return nil
	}
	allowedRoles := sourceInventoryFollowupRoleSet(roles)
	out := map[string]bool{}
	add := func(language string, terms []string) {
		language = strings.ToLower(strings.TrimSpace(language))
		if language != "" && sourceInventoryFollowupSurfaceTermsMatchQuotes(terms, profile.SourceQuotes) {
			out[language] = true
		}
	}
	for _, set := range observation.Sets {
		if set.Role == "" || set.Role == AnswerCandidateRoleUnknown || !allowedRoles[set.Role] {
			continue
		}
		for _, member := range set.Members {
			add(member.Language, member.SurfaceTerms)
			for _, attr := range member.Attributes {
				add(attr.Language, attr.SurfaceTerms)
			}
		}
	}
	return out
}

func sourceInventoryFollowupSurfaceTermsMatchQuotes(terms, quotes []string) bool {
	for _, term := range terms {
		term = sourceInventoryFollowupSurfaceTextKey(term)
		if term == "" {
			continue
		}
		for _, quote := range quotes {
			quote = sourceInventoryFollowupSurfaceTextKey(quote)
			if quote != "" && (strings.Contains(quote, term) || strings.Contains(term, quote)) {
				return true
			}
		}
	}
	return false
}

func sourceInventoryFollowupSurfaceTextKey(raw string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(raw))), " ")
}
