package types

import "strings"

// SourceInventoryLanguageCount is a repo-wide language census row attached to
// source-inventory observations. Unlike SourceClasses (path-role partitioned
// and limited to the lens query scope), this census is computed over the WHOLE
// repo's tracked source files, so the model can see that source files of a
// language exist even when its current navigation scope — frequently a
// grep-derived candidate list — surfaces none of them. It is the precise
// repomap language classification used purely as SOFT navigation guidance: it
// never gates an answer and never participates in the candidate-universe
// coverage comparison. Samples are a bounded set of representative repo-relative
// paths so the model can open the real files instead of inferring absence.
type SourceInventoryLanguageCount struct {
	Language string `json:"language,omitempty"`
	Count    int    `json:"count,omitempty"`
	// InScope reports whether AT LEAST ONE file of this language falls under
	// the lens query scopes. It is computed over the full repo file walk (not
	// just Samples), so a repo-dominant language whose sample paths happen to
	// sit outside a narrow scope is not misreported as scope-absent. The
	// out-of-scope navigation hint fires only for languages with InScope=false
	// — the ones the current navigation pass would miss entirely.
	InScope bool     `json:"in_scope,omitempty"`
	Samples []string `json:"samples,omitempty"`
}

func cloneSourceInventoryLanguageCounts(in []SourceInventoryLanguageCount) []SourceInventoryLanguageCount {
	if in == nil {
		return nil
	}
	out := make([]SourceInventoryLanguageCount, len(in))
	for i, item := range in {
		out[i] = item
		out[i].Samples = append([]string(nil), item.Samples...)
	}
	return out
}

// mergeSourceInventoryLanguageCounts unions two repo-wide language censuses.
// The census is repo-wide and identical across lens calls in a run, so merge is
// keyed by language: it keeps the larger count and the union of sample paths.
func mergeSourceInventoryLanguageCounts(existing, incoming []SourceInventoryLanguageCount) []SourceInventoryLanguageCount {
	if len(existing) == 0 {
		return cloneSourceInventoryLanguageCounts(incoming)
	}
	out := cloneSourceInventoryLanguageCounts(existing)
	byLang := make(map[string]int, len(out)+len(incoming))
	for i, item := range out {
		if strings.TrimSpace(item.Language) != "" {
			byLang[item.Language] = i
		}
	}
	for _, item := range incoming {
		if strings.TrimSpace(item.Language) == "" {
			continue
		}
		if idx, ok := byLang[item.Language]; ok {
			if item.Count > out[idx].Count {
				out[idx].Count = item.Count
			}
			out[idx].InScope = out[idx].InScope || item.InScope
			out[idx].Samples = mergeSourceInventoryAdvisoryStrings(out[idx].Samples, item.Samples)
			continue
		}
		byLang[item.Language] = len(out)
		out = append(out, SourceInventoryLanguageCount{
			Language: item.Language,
			Count:    item.Count,
			InScope:  item.InScope,
			Samples:  append([]string(nil), item.Samples...),
		})
	}
	return out
}
