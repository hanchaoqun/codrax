package types

import "strings"

func cloneSourceInventorySourceClassCounts(in []SourceInventorySourceClassCount) []SourceInventorySourceClassCount {
	if in == nil {
		return nil
	}
	out := make([]SourceInventorySourceClassCount, len(in))
	for i, item := range in {
		out[i] = item
		out[i].Samples = append([]string(nil), item.Samples...)
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
			out[idx].Provenance = mergeSourceInventoryAdvisoryStrings(out[idx].Provenance, item.Provenance)
			continue
		}
		byRole[item.Role] = len(out)
		cloned := item
		cloned.Samples = append([]string(nil), item.Samples...)
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
		item.Samples = normalizeSourceInventoryClassSamples(item.Samples)
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
