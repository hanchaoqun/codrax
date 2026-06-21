package types

import "strings"

func CloneSourceInventoryAdvisory(in SourceInventoryAdvisory) SourceInventoryAdvisory {
	out := in
	out.Scopes = append([]string(nil), in.Scopes...)
	out.Provenance = append([]string(nil), in.Provenance...)
	if in.Sets != nil {
		out.Sets = make([]SourceInventoryAdvisorySet, len(in.Sets))
		for i, set := range in.Sets {
			out.Sets[i] = set
			if set.Candidates != nil {
				out.Sets[i].Candidates = make([]SourceInventoryAdvisoryCandidate, len(set.Candidates))
				for j, candidate := range set.Candidates {
					out.Sets[i].Candidates[j] = candidate
					out.Sets[i].Candidates[j].SurfaceTerms = append([]string(nil), candidate.SurfaceTerms...)
					out.Sets[i].Candidates[j].Attributes = append([]SourceInventoryAdvisoryAttribute(nil), candidate.Attributes...)
					for k := range out.Sets[i].Candidates[j].Attributes {
						out.Sets[i].Candidates[j].Attributes[k].SurfaceTerms = append([]string(nil), candidate.Attributes[k].SurfaceTerms...)
					}
				}
			}
			if out.Sets[i].Total < len(out.Sets[i].Candidates) {
				out.Sets[i].Total = len(out.Sets[i].Candidates)
			}
		}
	}
	return out
}

func MergeSourceInventoryAdvisory(prior, current SourceInventoryAdvisory) SourceInventoryAdvisory {
	if !prior.IsActive() {
		return CloneSourceInventoryAdvisory(current)
	}
	if !current.IsActive() {
		return CloneSourceInventoryAdvisory(prior)
	}
	merged := CloneSourceInventoryAdvisory(prior)
	merged.Active = true
	merged.AdvisoryOnly = prior.AdvisoryOnly && current.AdvisoryOnly
	merged.Complete = prior.Complete && current.Complete
	merged.Scopes = mergeSourceInventoryAdvisoryStrings(merged.Scopes, current.Scopes)
	merged.Provenance = mergeSourceInventoryAdvisoryStrings(merged.Provenance, current.Provenance)
	byRole := make(map[AnswerCandidateRole]int, len(merged.Sets))
	for i := range merged.Sets {
		byRole[merged.Sets[i].Role] = i
	}
	for _, set := range current.Sets {
		if idx, ok := byRole[set.Role]; ok {
			merged.Sets[idx].Complete = merged.Sets[idx].Complete && set.Complete
			merged.Sets[idx].Candidates = mergeSourceInventoryAdvisoryCandidates(merged.Sets[idx].Candidates, set.Candidates)
			merged.Sets[idx].Total = maxSourceInventoryAdvisoryTotal(merged.Sets[idx].Total, set.Total, len(merged.Sets[idx].Candidates))
			continue
		}
		byRole[set.Role] = len(merged.Sets)
		merged.Sets = append(merged.Sets, CloneSourceInventoryAdvisory(SourceInventoryAdvisory{Active: true, Sets: []SourceInventoryAdvisorySet{set}}).Sets[0])
	}
	return merged
}

func mergeSourceInventoryAdvisoryStrings(existing, incoming []string) []string {
	if len(existing) == 0 {
		return append([]string(nil), incoming...)
	}
	out := append([]string(nil), existing...)
	seen := make(map[string]bool, len(out)+len(incoming))
	for _, item := range out {
		if key := strings.TrimSpace(item); key != "" {
			seen[key] = true
		}
	}
	for _, item := range incoming {
		key := strings.TrimSpace(item)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func mergeSourceInventoryAdvisoryCandidates(existing, incoming []SourceInventoryAdvisoryCandidate) []SourceInventoryAdvisoryCandidate {
	if len(existing) == 0 {
		return CloneSourceInventoryAdvisory(SourceInventoryAdvisory{Active: true, Sets: []SourceInventoryAdvisorySet{{Candidates: incoming}}}).Sets[0].Candidates
	}
	out := CloneSourceInventoryAdvisory(SourceInventoryAdvisory{Active: true, Sets: []SourceInventoryAdvisorySet{{Candidates: existing}}}).Sets[0].Candidates
	byKey := make(map[string]int, len(out)+len(incoming))
	for i, candidate := range out {
		if key := sourceInventoryAdvisoryCandidateKey(candidate); key != "" {
			byKey[key] = i
		}
	}
	for _, candidate := range incoming {
		key := sourceInventoryAdvisoryCandidateKey(candidate)
		if key == "" {
			continue
		}
		if idx, ok := byKey[key]; ok {
			out[idx].Attributes = mergeSourceInventoryAdvisoryAttributes(out[idx].Attributes, candidate.Attributes)
			out[idx].SurfaceTerms = MergeEvidenceStringSet(out[idx].SurfaceTerms, candidate.SurfaceTerms)
			continue
		}
		byKey[key] = len(out)
		cloned := candidate
		cloned.SurfaceTerms = append([]string(nil), candidate.SurfaceTerms...)
		cloned.Attributes = append([]SourceInventoryAdvisoryAttribute(nil), candidate.Attributes...)
		for i := range cloned.Attributes {
			cloned.Attributes[i].SurfaceTerms = append([]string(nil), candidate.Attributes[i].SurfaceTerms...)
		}
		out = append(out, cloned)
	}
	return out
}

func mergeSourceInventoryAdvisoryAttributes(existing, incoming []SourceInventoryAdvisoryAttribute) []SourceInventoryAdvisoryAttribute {
	if len(existing) == 0 {
		out := append([]SourceInventoryAdvisoryAttribute(nil), incoming...)
		for i := range out {
			out[i].SurfaceTerms = append([]string(nil), incoming[i].SurfaceTerms...)
		}
		return out
	}
	out := append([]SourceInventoryAdvisoryAttribute(nil), existing...)
	for i := range out {
		out[i].SurfaceTerms = append([]string(nil), existing[i].SurfaceTerms...)
	}
	seen := make(map[string]bool, len(out)+len(incoming))
	for _, attr := range out {
		if key := sourceInventoryAdvisoryAttributeKey(attr); key != "" {
			seen[key] = true
		}
	}
	for _, attr := range incoming {
		key := sourceInventoryAdvisoryAttributeKey(attr)
		if key == "" || seen[key] {
			if key != "" {
				for idx := range out {
					if sourceInventoryAdvisoryAttributeKey(out[idx]) == key {
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

func sourceInventoryAdvisoryCandidateKey(candidate SourceInventoryAdvisoryCandidate) string {
	key := strings.TrimSpace(candidate.Key)
	if key == "" {
		key = strings.TrimSpace(candidate.Member)
	}
	if key == "" {
		return ""
	}
	return string(candidate.Role) + "\x00" + key + "\x00" + strings.TrimSpace(candidate.File) + "\x00" + strings.TrimSpace(candidate.SupportRef)
}

func sourceInventoryAdvisoryAttributeKey(attr SourceInventoryAdvisoryAttribute) string {
	key := strings.TrimSpace(attr.Key)
	if key == "" {
		key = strings.TrimSpace(attr.Member)
	}
	if key == "" {
		return ""
	}
	return string(attr.Role) + "\x00" + key + "\x00" + strings.TrimSpace(attr.File) + "\x00" + strings.TrimSpace(attr.SupportRef)
}
