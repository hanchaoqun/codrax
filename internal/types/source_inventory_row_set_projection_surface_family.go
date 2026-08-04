package types

type sourceInventoryProjectionSurfaceFamily struct {
	role   AnswerCandidateRole
	family string
}

func sourceInventorySelectedSurfaceFamilies(rows []SourceInventoryRow, existing map[string]bool) map[sourceInventoryProjectionSurfaceFamily]bool {
	out := map[sourceInventoryProjectionSurfaceFamily]bool{}
	if len(rows) == 0 || len(existing) == 0 {
		return out
	}
	for _, row := range rows {
		key := sourceInventoryPrincipalRowKey(row)
		if key == "" || !existing[key] {
			continue
		}
		for _, family := range sourceInventoryProjectionSurfaceFamiliesForRow(row) {
			out[family] = true
		}
	}
	return out
}

func sourceInventoryProjectionSurfaceFamilyForRow(row SourceInventoryRow) (sourceInventoryProjectionSurfaceFamily, bool) {
	families := sourceInventoryProjectionSurfaceFamiliesForRow(row)
	if len(families) == 0 {
		return sourceInventoryProjectionSurfaceFamily{}, false
	}
	return families[0], true
}
