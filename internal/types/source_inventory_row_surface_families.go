package types

func sourceInventoryRowSurfaceFamily(row SourceInventoryRow) string {
	families := sourceInventoryRowSurfaceFamilies(row)
	if len(families) == 0 {
		return ""
	}
	return families[0]
}

func sourceInventoryProjectionSurfaceFamiliesForRow(row SourceInventoryRow) []sourceInventoryProjectionSurfaceFamily {
	if row.Role == "" || row.Role == AnswerCandidateRoleUnknown {
		return nil
	}
	families := sourceInventoryRowSurfaceFamilies(row)
	out := make([]sourceInventoryProjectionSurfaceFamily, 0, len(families))
	for _, family := range families {
		out = append(out, sourceInventoryProjectionSurfaceFamily{role: row.Role, family: family})
	}
	return out
}

func sourceInventoryRowSurfaceFamilies(row SourceInventoryRow) []string {
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		family := SourceInventorySurfaceTermKey(raw)
		if family == "" || seen[family] {
			return
		}
		seen[family] = true
		out = append(out, family)
	}
	add(row.SurfaceFamily)
	for _, family := range SourceInventorySurfaceFamilyKeys(row.Member.SurfaceTerms) {
		add(family)
	}
	return out
}
