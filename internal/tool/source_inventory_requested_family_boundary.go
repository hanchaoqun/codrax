package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceInventoryRequestedUniverseFamilyAddClassifiedPath(family *sourceInventoryRequestedUniverseFamily, file string) {
	if family == nil {
		return
	}
	file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
	if file == "" {
		return
	}
	for _, lang := range sourceInventoryPathLanguages(file) {
		family.languages[lang] = true
	}
	if strings.Contains(file, "/") {
		if class := types.ClassifySourcePathRole(file); class != types.SourcePathRoleUnknown {
			family.classes[class] = true
		}
	}
}

func sourceInventoryRequestedUniverseMergeFamilies(families ...sourceInventoryRequestedUniverseFamily) sourceInventoryRequestedUniverseFamily {
	out := sourceInventoryRequestedUniverseFamily{
		languages: map[string]bool{},
		classes:   map[types.SourcePathRole]bool{},
	}
	for _, family := range families {
		for lang := range family.languages {
			out.languages[lang] = true
		}
		for class := range family.classes {
			out.classes[class] = true
		}
	}
	return out
}

func sourceInventoryRequestedUniverseCompleteLensBackedFamily(observation types.SourceInventoryObservation, family sourceInventoryRequestedUniverseFamily, roles map[types.AnswerCandidateRole]bool) sourceInventoryRequestedUniverseFamily {
	out := sourceInventoryRequestedUniverseFamily{
		languages: map[string]bool{},
		classes:   map[types.SourcePathRole]bool{},
	}
	if (len(family.languages) == 0 && len(family.classes) == 0) || !observation.IsActive() {
		return out
	}
	addLens := func(languages []string, classes []types.SourcePathRole) {
		lensLanguages := map[string]bool{}
		lensClasses := map[types.SourcePathRole]bool{}
		for _, lang := range languages {
			lang = sourceInventoryRequestedUniverseNormalizeLanguage(lang)
			if lang != "" {
				lensLanguages[lang] = true
			}
		}
		for _, class := range classes {
			if class != types.SourcePathRoleUnknown {
				lensClasses[class] = true
			}
		}
		aligned := false
		for lang := range lensLanguages {
			if family.languages[lang] {
				aligned = true
				break
			}
		}
		if !aligned {
			for class := range lensClasses {
				if family.classes[class] {
					aligned = true
					break
				}
			}
		}
		if !aligned {
			return
		}
		for lang := range lensLanguages {
			out.languages[lang] = true
		}
		for class := range lensClasses {
			out.classes[class] = true
		}
	}
	for _, lens := range observation.CompleteLenses {
		if !sourceInventoryRequestedUniverseRoleAllowed(roles, lens.Role) {
			continue
		}
		addLens(lens.Languages, lens.SourceClasses)
	}
	for _, set := range observation.Sets {
		if !set.Complete || !sourceInventoryRequestedUniverseRoleAllowed(roles, set.Role) {
			continue
		}
		var languages []string
		var classes []types.SourcePathRole
		for _, member := range set.Members {
			languages = append(languages, sourceInventoryMemberLanguages(member)...)
			if class := sourceInventoryMemberClass(member); class != types.SourcePathRoleUnknown {
				classes = append(classes, class)
			}
		}
		addLens(languages, classes)
	}
	return out
}
