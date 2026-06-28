package types

import "strings"

func sourceInventoryFollowupTargetLanguages(rm RequestModel, requiredFiles []string) map[string]bool {
	hintLanguages := map[string]bool{}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if hint.Confidence < 0.8 {
			continue
		}
		sourceInventoryFollowupAddPathLanguages(hintLanguages, hint.Path)
	}
	if sourceInventoryFollowupLanguageSetCoherent(hintLanguages) {
		return hintLanguages
	}
	fileLanguages := map[string]bool{}
	for _, file := range requiredFiles {
		sourceInventoryFollowupAddPathLanguages(fileLanguages, file)
	}
	if sourceInventoryFollowupLanguageSetCoherent(fileLanguages) {
		return fileLanguages
	}
	return nil
}

func sourceInventoryFollowupAddPathLanguages(out map[string]bool, raw string) {
	for _, family := range VerificationLanguageFamiliesFromPath(raw) {
		value := strings.ToLower(strings.TrimSpace(string(family)))
		if value != "" {
			out[value] = true
		}
	}
}

func sourceInventoryFollowupLanguageSetCoherent(in map[string]bool) bool {
	return len(in) > 0 && len(in) <= 2
}

func sourceInventoryFollowupSampleMatchesLanguages(sample string, targetLanguages map[string]bool) bool {
	if len(targetLanguages) == 0 {
		return true
	}
	for _, family := range VerificationLanguageFamiliesFromPath(sample) {
		if targetLanguages[strings.ToLower(strings.TrimSpace(string(family)))] {
			return true
		}
	}
	return false
}
