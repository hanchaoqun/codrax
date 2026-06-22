package types

import "strings"

func CloneSourceInventoryObservation(in SourceInventoryObservation) SourceInventoryObservation {
	out := in
	out.Scopes = append([]string(nil), in.Scopes...)
	out.Provenance = append([]string(nil), in.Provenance...)
	out.Lens = append([]string(nil), in.Lens...)
	out.SourceClasses = cloneSourceInventorySourceClassCounts(in.SourceClasses)
	out.RepoLanguages = cloneSourceInventoryLanguageCounts(in.RepoLanguages)
	out.CompleteLenses = cloneSourceInventoryCompleteLenses(in.CompleteLenses)
	out.Page = cloneSourceInventoryObservationPage(in.Page)
	out.Execution = cloneSourceInventoryExecutionState(in.Execution)
	if in.Sets != nil {
		out.Sets = make([]SourceInventoryObservationSet, len(in.Sets))
		for i, set := range in.Sets {
			out.Sets[i] = set
			out.Sets[i].Members = cloneSourceInventoryObservationMembers(set.Members)
			out.Sets[i].Count = len(out.Sets[i].Members)
		}
	}
	return normalizeSourceInventoryObservation(out)
}

func MergeSourceInventoryObservation(prior, current SourceInventoryObservation) SourceInventoryObservation {
	if !prior.IsActive() {
		return CloneSourceInventoryObservation(current)
	}
	if !current.IsActive() {
		return CloneSourceInventoryObservation(prior)
	}
	merged := CloneSourceInventoryObservation(prior)
	merged.Active = true
	merged.AdvisoryOnly = prior.AdvisoryOnly && current.AdvisoryOnly
	merged.Complete = prior.Complete && current.Complete
	merged.Scopes = mergeSourceInventoryAdvisoryStrings(merged.Scopes, current.Scopes)
	merged.Provenance = mergeSourceInventoryAdvisoryStrings(merged.Provenance, current.Provenance)
	merged.Lens = mergeSourceInventoryAdvisoryStrings(merged.Lens, current.Lens)
	merged.SourceClasses = mergeSourceInventorySourceClassCounts(merged.SourceClasses, current.SourceClasses)
	merged.RepoLanguages = mergeSourceInventoryLanguageCounts(merged.RepoLanguages, current.RepoLanguages)
	merged.CompleteLenses = mergeSourceInventoryCompleteLenses(merged.CompleteLenses, current.CompleteLenses, sourceInventoryCompleteLensesFromObservation(current))
	if current.Page != nil {
		merged.Page = cloneSourceInventoryObservationPage(current.Page)
	}
	merged.Execution = mergeSourceInventoryExecutionState(merged.Execution, current.Execution)
	currentSupersedesPrior := sourceInventoryObservationSupersedesPrior(prior, current)
	if currentSupersedesPrior {
		merged.Page = cloneSourceInventoryObservationPage(current.Page)
		merged.Execution = cloneSourceInventoryExecutionState(current.Execution)
	}
	byRole := make(map[AnswerCandidateRole]int, len(merged.Sets))
	for i := range merged.Sets {
		byRole[merged.Sets[i].Role] = i
	}
	for _, set := range current.Sets {
		if idx, ok := byRole[set.Role]; ok {
			previous := merged.Sets[idx]
			merged.Sets[idx].Members = mergeSourceInventoryObservationMembers(merged.Sets[idx].Members, set.Members)
			merged.Sets[idx].Count = len(merged.Sets[idx].Members)
			merged.Sets[idx].Total = maxSourceInventoryObservationTotal(merged.Sets[idx].Total, set.Total, merged.Sets[idx].Count)
			merged.Sets[idx].Complete = sourceInventoryObservationMergedSetComplete(previous, set, merged.Sets[idx])
			continue
		}
		byRole[set.Role] = len(merged.Sets)
		cloned := SourceInventoryObservationSet{
			Role:     set.Role,
			Complete: set.Complete,
			Total:    set.Total,
			Members:  cloneSourceInventoryObservationMembers(set.Members),
		}
		cloned.Count = len(cloned.Members)
		if cloned.Total < cloned.Count {
			cloned.Total = cloned.Count
		}
		merged.Sets = append(merged.Sets, cloned)
	}
	if currentSupersedesPrior {
		merged.Complete = true
	} else {
		merged.Complete = sourceInventoryObservationStructurallyComplete(merged)
	}
	return normalizeSourceInventoryObservation(merged)
}

func cloneSourceInventoryObservationMembers(in []SourceInventoryObservationMember) []SourceInventoryObservationMember {
	if in == nil {
		return nil
	}
	out := make([]SourceInventoryObservationMember, len(in))
	for i, member := range in {
		out[i] = member
		out[i].Provenance = append([]string(nil), member.Provenance...)
		out[i].SurfaceTerms = append([]string(nil), member.SurfaceTerms...)
		out[i].Attributes = append([]SourceInventoryObservationAttribute(nil), member.Attributes...)
		for j := range out[i].Attributes {
			out[i].Attributes[j].SurfaceTerms = append([]string(nil), member.Attributes[j].SurfaceTerms...)
		}
	}
	return out
}

func mergeSourceInventoryObservationMembers(existing, incoming []SourceInventoryObservationMember) []SourceInventoryObservationMember {
	if len(existing) == 0 {
		return cloneSourceInventoryObservationMembers(incoming)
	}
	out := cloneSourceInventoryObservationMembers(existing)
	byKey := make(map[string]int, len(out)+len(incoming))
	for i, member := range out {
		if key := sourceInventoryObservationMemberKey(member); key != "" {
			byKey[key] = i
		}
	}
	for _, member := range incoming {
		key := sourceInventoryObservationMemberKey(member)
		if key == "" {
			continue
		}
		if idx, ok := byKey[key]; ok {
			out[idx].Attributes = mergeSourceInventoryObservationAttributes(out[idx].Attributes, member.Attributes)
			out[idx].Provenance = mergeSourceInventoryAdvisoryStrings(out[idx].Provenance, member.Provenance)
			out[idx].SurfaceTerms = MergeEvidenceStringSet(out[idx].SurfaceTerms, member.SurfaceTerms)
			if out[idx].CoverageState == SourceInventoryCoverageUnknown {
				out[idx].CoverageState = member.CoverageState
			}
			continue
		}
		byKey[key] = len(out)
		cloned := member
		cloned.SurfaceTerms = append([]string(nil), member.SurfaceTerms...)
		cloned.Attributes = append([]SourceInventoryObservationAttribute(nil), member.Attributes...)
		for i := range cloned.Attributes {
			cloned.Attributes[i].SurfaceTerms = append([]string(nil), member.Attributes[i].SurfaceTerms...)
		}
		out = append(out, cloned)
	}
	return out
}

func mergeSourceInventoryObservationAttributes(existing, incoming []SourceInventoryObservationAttribute) []SourceInventoryObservationAttribute {
	if len(existing) == 0 {
		out := append([]SourceInventoryObservationAttribute(nil), incoming...)
		for i := range out {
			out[i].SurfaceTerms = append([]string(nil), incoming[i].SurfaceTerms...)
		}
		return out
	}
	out := append([]SourceInventoryObservationAttribute(nil), existing...)
	for i := range out {
		out[i].SurfaceTerms = append([]string(nil), existing[i].SurfaceTerms...)
	}
	seen := make(map[string]bool, len(out)+len(incoming))
	for _, attr := range out {
		if key := sourceInventoryObservationAttributeKey(attr); key != "" {
			seen[key] = true
		}
	}
	for _, attr := range incoming {
		key := sourceInventoryObservationAttributeKey(attr)
		if key == "" || seen[key] {
			if key != "" {
				for idx := range out {
					if sourceInventoryObservationAttributeKey(out[idx]) == key {
						out[idx].SurfaceTerms = MergeEvidenceStringSet(out[idx].SurfaceTerms, attr.SurfaceTerms)
						break
					}
				}
			}
			continue
		}
		seen[key] = true
		attr.SurfaceTerms = append([]string(nil), attr.SurfaceTerms...)
		out = append(out, attr)
	}
	return out
}

func sourceInventoryObservationMemberKey(member SourceInventoryObservationMember) string {
	key := strings.TrimSpace(member.Key)
	if key == "" {
		key = strings.TrimSpace(member.Name)
	}
	if key == "" {
		return ""
	}
	return string(member.Role) + "\x00" + key + "\x00" + strings.TrimSpace(member.File) + "\x00" + strings.TrimSpace(member.SupportRef)
}

func sourceInventoryObservationAttributeKey(attr SourceInventoryObservationAttribute) string {
	key := strings.TrimSpace(attr.Key)
	if key == "" {
		key = strings.TrimSpace(attr.Name)
	}
	if key == "" {
		return ""
	}
	return string(attr.Role) + "\x00" + key + "\x00" + strings.TrimSpace(attr.File) + "\x00" + strings.TrimSpace(attr.SupportRef)
}
