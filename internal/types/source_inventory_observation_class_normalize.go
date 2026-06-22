package types

import "strings"

func normalizeSourceInventorySourceClassCounts(in []SourceInventorySourceClassCount) []SourceInventorySourceClassCount {
	if len(in) == 0 {
		return nil
	}
	out := in[:0]
	for _, item := range in {
		if item.Role == SourcePathRoleUnknown || item.Count <= 0 {
			continue
		}
		item.Samples = normalizeSourceInventoryClassSamples(item.Samples)
		item.Languages = normalizeSourceInventoryClassLanguages(item.Languages)
		out = append(out, item)
	}
	return out
}

func normalizeSourceInventoryClassLanguages(in []SourceInventoryLanguageCount) []SourceInventoryLanguageCount {
	if len(in) == 0 {
		return nil
	}
	out := make([]SourceInventoryLanguageCount, 0, len(in))
	byLang := map[string]int{}
	for _, item := range in {
		lang := strings.TrimSpace(item.Language)
		if lang == "" || item.Count <= 0 {
			continue
		}
		item.Language = lang
		item.Samples = normalizeSourceInventoryClassSamples(item.Samples)
		if idx, ok := byLang[lang]; ok {
			if item.Count > out[idx].Count {
				out[idx].Count = item.Count
			}
			out[idx].InScope = out[idx].InScope || item.InScope
			out[idx].Samples = mergeSourceInventoryAdvisoryStrings(out[idx].Samples, item.Samples)
			continue
		}
		byLang[lang] = len(out)
		out = append(out, item)
	}
	return out
}

func normalizeSourceInventoryClassSamples(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		path := strings.Trim(strings.ReplaceAll(strings.TrimSpace(raw), `\`, `/`), "/")
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}
