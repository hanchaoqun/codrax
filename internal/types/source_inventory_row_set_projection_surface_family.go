package types

import "strings"

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
		if family, ok := sourceInventoryProjectionSurfaceFamilyForRow(row); ok {
			out[family] = true
		}
	}
	return out
}

func sourceInventoryProjectionSurfaceFamilyForRow(row SourceInventoryRow) (sourceInventoryProjectionSurfaceFamily, bool) {
	family := sourceInventoryProjectionSurfaceFamilyKey(row.Member.SurfaceTerms)
	if family == "" || row.Role == "" || row.Role == AnswerCandidateRoleUnknown {
		return sourceInventoryProjectionSurfaceFamily{}, false
	}
	return sourceInventoryProjectionSurfaceFamily{role: row.Role, family: family}, true
}

func sourceInventoryProjectionSurfaceFamilyKey(terms []string) string {
	keys := sourceInventoryProjectionSurfaceTermKeys(terms)
	for _, candidate := range keys {
		for _, other := range keys {
			if other != candidate && strings.HasPrefix(other, candidate+" ") {
				return candidate
			}
		}
	}
	for _, key := range keys {
		if idx := strings.LastIndex(key, " "); idx > 0 {
			return strings.TrimSpace(key[:idx])
		}
	}
	return ""
}

func sourceInventoryProjectionSurfaceTermKeys(terms []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, term := range terms {
		key := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(strings.ReplaceAll(term, `\`, `/`))), " "))
		key = strings.Trim(key, "` \t\r\n")
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}
