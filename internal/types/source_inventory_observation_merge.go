package types

import "strings"

func CloneSourceInventoryObservation(in SourceInventoryObservation) SourceInventoryObservation {
	out := in
	out.Scopes = append([]string(nil), in.Scopes...)
	out.Provenance = append([]string(nil), in.Provenance...)
	out.Lens = append([]string(nil), in.Lens...)
	out.SourceClasses = cloneSourceInventorySourceClassCounts(in.SourceClasses)
	out.RepoLanguages = cloneSourceInventoryLanguageCounts(in.RepoLanguages)
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
	if current.Page != nil {
		merged.Page = cloneSourceInventoryObservationPage(current.Page)
	}
	merged.Execution = mergeSourceInventoryExecutionState(merged.Execution, current.Execution)
	byRole := make(map[AnswerCandidateRole]int, len(merged.Sets))
	for i := range merged.Sets {
		byRole[merged.Sets[i].Role] = i
	}
	for _, set := range current.Sets {
		if idx, ok := byRole[set.Role]; ok {
			merged.Sets[idx].Complete = merged.Sets[idx].Complete && set.Complete
			merged.Sets[idx].Members = mergeSourceInventoryObservationMembers(merged.Sets[idx].Members, set.Members)
			merged.Sets[idx].Count = len(merged.Sets[idx].Members)
			merged.Sets[idx].Total = maxSourceInventoryObservationTotal(merged.Sets[idx].Total, set.Total, merged.Sets[idx].Count)
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
	return normalizeSourceInventoryObservation(merged)
}

func normalizeSourceInventoryObservation(in SourceInventoryObservation) SourceInventoryObservation {
	in.SourceClasses = normalizeSourceInventorySourceClassCounts(in.SourceClasses)
	if len(in.Sets) == 0 && len(in.SourceClasses) == 0 {
		return SourceInventoryObservation{}
	}
	in.Active = true
	for i := range in.Sets {
		in.Sets[i].Count = len(in.Sets[i].Members)
		if in.Sets[i].Total < in.Sets[i].Count {
			in.Sets[i].Total = in.Sets[i].Count
		}
	}
	return in
}

func cloneSourceInventorySourceClassCounts(in []SourceInventorySourceClassCount) []SourceInventorySourceClassCount {
	if in == nil {
		return nil
	}
	out := make([]SourceInventorySourceClassCount, len(in))
	for i, item := range in {
		out[i] = item
		out[i].Provenance = append([]string(nil), item.Provenance...)
	}
	return out
}

func mergeSourceInventorySourceClassCounts(existing, incoming []SourceInventorySourceClassCount) []SourceInventorySourceClassCount {
	if len(existing) == 0 {
		return cloneSourceInventorySourceClassCounts(incoming)
	}
	out := cloneSourceInventorySourceClassCounts(existing)
	byRole := make(map[SourcePathRole]int, len(out)+len(incoming))
	for i, item := range out {
		if item.Role != SourcePathRoleUnknown {
			byRole[item.Role] = i
		}
	}
	for _, item := range incoming {
		if item.Role == SourcePathRoleUnknown {
			continue
		}
		if idx, ok := byRole[item.Role]; ok {
			if item.Count > out[idx].Count {
				out[idx].Count = item.Count
			}
			out[idx].Complete = out[idx].Complete && item.Complete
			out[idx].Provenance = mergeSourceInventoryAdvisoryStrings(out[idx].Provenance, item.Provenance)
			continue
		}
		byRole[item.Role] = len(out)
		cloned := item
		cloned.Provenance = append([]string(nil), item.Provenance...)
		out = append(out, cloned)
	}
	return normalizeSourceInventorySourceClassCounts(out)
}

func normalizeSourceInventorySourceClassCounts(in []SourceInventorySourceClassCount) []SourceInventorySourceClassCount {
	if len(in) == 0 {
		return nil
	}
	out := in[:0]
	for _, item := range in {
		if item.Role == SourcePathRoleUnknown || item.Count <= 0 {
			continue
		}
		out = append(out, item)
	}
	return out
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
