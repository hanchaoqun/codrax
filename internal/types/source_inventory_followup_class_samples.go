package types

import "strings"

func sourceInventoryFollowupClassSamples(class SourceInventorySourceClassCount) []string {
	return sourceInventoryFollowupClassSamplesForLanguages(class, nil)
}

func sourceInventoryFollowupClassSamplesForLanguages(class SourceInventorySourceClassCount, targetLanguages map[string]bool) []string {
	var out []string
	seen := map[string]bool{}
	add := func(sample string) {
		sample = strings.Trim(strings.ReplaceAll(strings.TrimSpace(sample), `\`, `/`), "/")
		if sample == "" || seen[sample] {
			return
		}
		seen[sample] = true
		out = append(out, sample)
	}
	for _, lang := range class.Languages {
		language := strings.ToLower(strings.TrimSpace(lang.Language))
		if language == "" || lang.Count <= 0 {
			continue
		}
		if len(targetLanguages) > 0 && !targetLanguages[language] {
			continue
		}
		for _, sample := range lang.Samples {
			add(sample)
		}
	}
	if len(targetLanguages) > 0 && len(out) == 0 {
		for _, sample := range class.Samples {
			if sourceInventoryFollowupSampleMatchesLanguages(sample, targetLanguages) {
				add(sample)
			}
		}
		return out
	}
	for _, sample := range class.Samples {
		if len(targetLanguages) > 0 && !sourceInventoryFollowupSampleMatchesLanguages(sample, targetLanguages) {
			continue
		}
		add(sample)
	}
	return out
}
