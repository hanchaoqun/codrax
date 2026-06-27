package types

import "strings"

func sourceInventoryFollowupClassSamples(class SourceInventorySourceClassCount) []string {
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
		if strings.TrimSpace(lang.Language) == "" || lang.Count <= 0 {
			continue
		}
		for _, sample := range lang.Samples {
			add(sample)
		}
	}
	for _, sample := range class.Samples {
		add(sample)
	}
	return out
}
