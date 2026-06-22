package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceInventoryRequestedUniverseCensusMissing(observation types.SourceInventoryObservation, covered sourceInventoryRequestedUniverseFamily) bool {
	for _, class := range observation.SourceClasses {
		if class.Role == types.SourcePathRoleUnknown || class.Count <= 0 || covered.classes[class.Role] {
			continue
		}
		if len(covered.languages) == 0 {
			return true
		}
		if len(class.Languages) > 0 {
			if sourceInventoryClassLanguagesIntersectLanguages(class.Languages, covered.languages) {
				return true
			}
			continue
		}
		if sourceInventoryClassSamplesIntersectLanguages(class.Samples, covered.languages) {
			return true
		}
		if len(class.Samples) == 0 {
			return true
		}
	}
	return false
}

func sourceInventoryClassLanguagesIntersectLanguages(classLanguages []types.SourceInventoryLanguageCount, languages map[string]bool) bool {
	for _, classLanguage := range classLanguages {
		lang := strings.ToLower(strings.TrimSpace(classLanguage.Language))
		if lang == "" || classLanguage.Count <= 0 {
			continue
		}
		if languages[lang] {
			return true
		}
	}
	return false
}

func sourceInventoryClassSamplesIntersectLanguages(samples []string, languages map[string]bool) bool {
	for _, sample := range samples {
		for _, lang := range sourceInventoryPathLanguages(sample) {
			if languages[lang] {
				return true
			}
		}
	}
	return false
}
