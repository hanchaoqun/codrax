package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceInventoryRequestedUniverseFamilyBackedByCompleteLens(observation types.SourceInventoryObservation, family sourceInventoryRequestedUniverseFamily, roles map[types.AnswerCandidateRole]bool) bool {
	if (len(family.languages) == 0 && len(family.classes) == 0) || !observation.IsActive() {
		return false
	}
	backed := sourceInventoryRequestedUniverseFamily{
		languages: map[string]bool{},
		classes:   map[types.SourcePathRole]bool{},
	}
	for _, lens := range observation.CompleteLenses {
		if !sourceInventoryRequestedUniverseRoleAllowed(roles, lens.Role) {
			continue
		}
		for _, lang := range lens.Languages {
			lang = sourceInventoryRequestedUniverseNormalizeLanguage(lang)
			if lang != "" {
				backed.languages[lang] = true
			}
		}
		for _, class := range lens.SourceClasses {
			if class != types.SourcePathRoleUnknown {
				backed.classes[class] = true
			}
		}
	}
	for _, set := range observation.Sets {
		if !set.Complete || !sourceInventoryRequestedUniverseRoleAllowed(roles, set.Role) {
			continue
		}
		for _, member := range set.Members {
			for _, lang := range sourceInventoryMemberLanguages(member) {
				backed.languages[lang] = true
			}
			if class := sourceInventoryMemberClass(member); class != types.SourcePathRoleUnknown {
				backed.classes[class] = true
			}
		}
	}
	return sourceInventoryRequestedUniverseFamilyContains(backed, family)
}

func sourceInventoryRequestedUniverseNormalizeLanguage(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" ||
		raw == string(types.VerificationLanguageUnknown) ||
		raw == string(types.VerificationLanguageConfigWorkflow) {
		return ""
	}
	return raw
}

func sourceInventoryRequestedUniverseFamilyContains(backed, want sourceInventoryRequestedUniverseFamily) bool {
	if len(backed.languages) == 0 && len(backed.classes) == 0 {
		return false
	}
	for lang := range want.languages {
		if !backed.languages[lang] {
			return false
		}
	}
	for class := range want.classes {
		if !backed.classes[class] {
			return false
		}
	}
	return len(want.languages) > 0 || len(want.classes) > 0
}
