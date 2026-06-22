package types

import (
	"path/filepath"
	"sort"
	"strings"
)

// SourceInventoryCompleteLens records the per-query complete boundary that
// produced a source-inventory set. It survives later role-level merges so
// completion gates can trust a scoped complete lens without confusing it with
// stale repo-wide debt for the same role.
type SourceInventoryCompleteLens struct {
	Role          AnswerCandidateRole `json:"role,omitempty"`
	Scopes        []string            `json:"scopes,omitempty"`
	Languages     []string            `json:"languages,omitempty"`
	SourceClasses []SourcePathRole    `json:"source_classes,omitempty"`
	Count         int                 `json:"count,omitempty"`
	Total         int                 `json:"total,omitempty"`
	Provenance    []string            `json:"provenance,omitempty"`
}

func cloneSourceInventoryCompleteLenses(in []SourceInventoryCompleteLens) []SourceInventoryCompleteLens {
	if in == nil {
		return nil
	}
	out := make([]SourceInventoryCompleteLens, len(in))
	for i, lens := range in {
		out[i] = lens
		out[i].Scopes = append([]string(nil), lens.Scopes...)
		out[i].Languages = append([]string(nil), lens.Languages...)
		out[i].SourceClasses = append([]SourcePathRole(nil), lens.SourceClasses...)
		out[i].Provenance = append([]string(nil), lens.Provenance...)
	}
	return out
}

func mergeSourceInventoryCompleteLenses(existing []SourceInventoryCompleteLens, groups ...[]SourceInventoryCompleteLens) []SourceInventoryCompleteLens {
	out := cloneSourceInventoryCompleteLenses(existing)
	index := map[string]int{}
	for i, lens := range out {
		lens = normalizeSourceInventoryCompleteLens(lens)
		if key := sourceInventoryCompleteLensKey(lens); key != "" {
			out[i] = lens
			index[key] = i
		}
	}
	for _, group := range groups {
		for _, incoming := range group {
			incoming = normalizeSourceInventoryCompleteLens(incoming)
			key := sourceInventoryCompleteLensKey(incoming)
			if key == "" {
				continue
			}
			if idx, ok := index[key]; ok {
				if incoming.Count > out[idx].Count {
					out[idx].Count = incoming.Count
				}
				if incoming.Total > out[idx].Total {
					out[idx].Total = incoming.Total
				}
				out[idx].Provenance = mergeSourceInventoryAdvisoryStrings(out[idx].Provenance, incoming.Provenance)
				continue
			}
			index[key] = len(out)
			out = append(out, incoming)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return string(out[i].Role) < string(out[j].Role)
		}
		return sourceInventoryCompleteLensKey(out[i]) < sourceInventoryCompleteLensKey(out[j])
	})
	return out
}

func normalizeSourceInventoryCompleteLens(in SourceInventoryCompleteLens) SourceInventoryCompleteLens {
	if role, ok := NormalizeAnswerCandidateRole(string(in.Role)); ok {
		in.Role = role
	} else {
		in.Role = AnswerCandidateRoleUnknown
	}
	in.Scopes = sourceInventoryNormalizeScopes(in.Scopes)
	in.Languages = normalizeSourceInventoryCompleteLensLanguages(in.Languages)
	in.SourceClasses = normalizeSourceInventoryCompleteLensClasses(in.SourceClasses)
	in.Provenance = mergeSourceInventoryAdvisoryStrings(nil, in.Provenance)
	if in.Total < in.Count {
		in.Total = in.Count
	}
	return in
}

func sourceInventoryCompleteLensesFromObservation(observation SourceInventoryObservation) []SourceInventoryCompleteLens {
	if len(observation.Sets) == 0 {
		return nil
	}
	var out []SourceInventoryCompleteLens
	for _, set := range observation.Sets {
		role, ok := NormalizeAnswerCandidateRole(string(set.Role))
		if !ok {
			role = AnswerCandidateRoleUnknown
		}
		if role == "" || role == AnswerCandidateRoleUnknown || !set.Complete || len(set.Members) == 0 {
			continue
		}
		lens := SourceInventoryCompleteLens{
			Role:       role,
			Scopes:     append([]string(nil), observation.Scopes...),
			Count:      len(set.Members),
			Total:      set.Total,
			Provenance: append([]string(nil), observation.Provenance...),
		}
		langs := map[string]bool{}
		classes := map[SourcePathRole]bool{}
		for _, member := range set.Members {
			sourceInventoryCompleteLensAddMember(lens.Role, member, langs, classes)
		}
		for lang := range langs {
			lens.Languages = append(lens.Languages, lang)
		}
		for class := range classes {
			lens.SourceClasses = append(lens.SourceClasses, class)
		}
		out = append(out, lens)
	}
	return mergeSourceInventoryCompleteLenses(nil, out)
}

func sourceInventoryCompleteLensAddMember(_ AnswerCandidateRole, member SourceInventoryObservationMember, languages map[string]bool, classes map[SourcePathRole]bool) {
	addLanguage := func(raw string) {
		raw = strings.ToLower(strings.TrimSpace(raw))
		if raw == "" || raw == string(VerificationLanguageUnknown) || raw == string(VerificationLanguageConfigWorkflow) {
			return
		}
		languages[raw] = true
	}
	addPath := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if _, loc, ok := ParseAnswerSupportRefMemberLocation(raw); ok {
			raw = loc.File
		}
		for _, family := range VerificationLanguageFamiliesFromPath(raw) {
			addLanguage(string(family))
		}
		if strings.Contains(raw, "/") && IsCodeOrConfigPathExtension(filepath.Ext(raw)) {
			if class := ClassifySourcePathRole(raw); class != SourcePathRoleUnknown {
				classes[class] = true
			}
		}
		if !strings.Contains(raw, "/") && IsCodeOrConfigPathExtension(filepath.Ext(raw)) {
			if class := ClassifySourcePathRole(raw); class != SourcePathRoleUnknown {
				classes[class] = true
			}
		}
	}
	addCodePathClass := func(raw string) {
		raw = strings.TrimSpace(raw)
		if _, loc, ok := ParseAnswerSupportRefMemberLocation(raw); ok {
			raw = loc.File
		}
		if !IsCodeOrConfigPathExtension(filepath.Ext(raw)) {
			return
		}
		if class := ClassifySourcePathRole(raw); class != SourcePathRoleUnknown {
			classes[class] = true
		}
	}
	addLanguage(member.Language)
	addPath(member.File)
	addPath(member.SupportRef)
	addPath(member.Key)
	addCodePathClass(member.File)
	addCodePathClass(member.SupportRef)
	addCodePathClass(member.Key)
}

func sourceInventoryCompleteLensKey(lens SourceInventoryCompleteLens) string {
	if lens.Role == "" || lens.Role == AnswerCandidateRoleUnknown ||
		(len(lens.Languages) == 0 && len(lens.SourceClasses) == 0) {
		return ""
	}
	return string(lens.Role) + "\x00" +
		strings.Join(lens.Scopes, "\x1f") + "\x00" +
		strings.Join(lens.Languages, "\x1f") + "\x00" +
		sourceInventoryCompleteLensClassKey(lens.SourceClasses)
}

func sourceInventoryCompleteLensClassKey(classes []SourcePathRole) string {
	parts := make([]string, 0, len(classes))
	for _, class := range classes {
		if class == "" || class == SourcePathRoleUnknown {
			continue
		}
		parts = append(parts, string(class))
	}
	sort.Strings(parts)
	return strings.Join(parts, "\x1f")
}

func normalizeSourceInventoryCompleteLensLanguages(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range in {
		raw = strings.ToLower(strings.TrimSpace(raw))
		if raw == "" || raw == string(VerificationLanguageUnknown) || raw == string(VerificationLanguageConfigWorkflow) || seen[raw] {
			continue
		}
		seen[raw] = true
		out = append(out, raw)
	}
	sort.Strings(out)
	return out
}

func normalizeSourceInventoryCompleteLensClasses(in []SourcePathRole) []SourcePathRole {
	seen := map[SourcePathRole]bool{}
	var out []SourcePathRole
	for _, class := range in {
		if class == "" || class == SourcePathRoleUnknown || seen[class] {
			continue
		}
		seen[class] = true
		out = append(out, class)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}
