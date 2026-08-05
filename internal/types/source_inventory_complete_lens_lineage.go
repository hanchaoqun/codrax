package types

import "sort"

func cloneSourceInventoryCompleteLenses(in []SourceInventoryCompleteLens) []SourceInventoryCompleteLens {
	if in == nil {
		return nil
	}
	out := make([]SourceInventoryCompleteLens, len(in))
	for i, lens := range in {
		out[i] = lens
		out[i].Scopes = append([]string(nil), lens.Scopes...)
		out[i].QueryPathScopes = append([]string(nil), lens.QueryPathScopes...)
		out[i].Languages = append([]string(nil), lens.Languages...)
		out[i].SourceClasses = append([]SourcePathRole(nil), lens.SourceClasses...)
		out[i].SurfaceFamilies = append([]string(nil), lens.SurfaceFamilies...)
		out[i].Provenance = append([]string(nil), lens.Provenance...)
	}
	return out
}

func normalizeSourceInventoryCompleteLensQueryPathScopes(in []string) []string {
	out := sourceInventoryNormalizeScopes(in)
	sort.Strings(out)
	return out
}
