package types

func cloneSourceInventorySourceClassCounts(in []SourceInventorySourceClassCount) []SourceInventorySourceClassCount {
	if in == nil {
		return nil
	}
	out := make([]SourceInventorySourceClassCount, len(in))
	for i, item := range in {
		out[i] = item
		out[i].Samples = append([]string(nil), item.Samples...)
		out[i].Languages = cloneSourceInventoryLanguageCounts(item.Languages)
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
			out[idx].Complete = sourceInventoryMergedClassComplete(out[idx], item)
			out[idx].Samples = mergeSourceInventoryAdvisoryStrings(out[idx].Samples, item.Samples)
			out[idx].Languages = mergeSourceInventoryLanguageCounts(out[idx].Languages, item.Languages)
			out[idx].Provenance = mergeSourceInventoryAdvisoryStrings(out[idx].Provenance, item.Provenance)
			continue
		}
		byRole[item.Role] = len(out)
		cloned := item
		cloned.Samples = append([]string(nil), item.Samples...)
		cloned.Languages = cloneSourceInventoryLanguageCounts(item.Languages)
		cloned.Provenance = append([]string(nil), item.Provenance...)
		out = append(out, cloned)
	}
	return normalizeSourceInventorySourceClassCounts(out)
}
