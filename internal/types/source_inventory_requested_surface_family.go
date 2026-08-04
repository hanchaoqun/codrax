package types

import "strings"

func sourceInventoryFilterRowsToRequestedSurfaceFamilies(rm RequestModel, principal, support []SourceInventoryRow) ([]SourceInventoryRow, []SourceInventoryRow) {
	if len(principal) == 0 || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() || len(rm.SourceInventoryProfile.SourceQuotes) == 0 {
		return principal, support
	}
	requested := sourceInventoryRequestedSurfaceFamiliesByRole(rm.SourceInventoryProfile.SourceQuotes, principal)
	if len(requested) == 0 {
		return principal, support
	}
	filtered := make([]SourceInventoryRow, 0, len(principal))
	demoted := make([]SourceInventoryRow, 0, len(principal))
	for _, row := range principal {
		byRole := requested[row.Role]
		if len(byRole) == 0 {
			if sourceInventorySymbolRoleRequiresRequestedSurfaceFamily(row.Role) {
				row.Lane = SourceInventoryRowLaneSupport
				row.ReasonCode = SourceInventoryRowReasonSurfaceFamily
				demoted = append(demoted, row)
				continue
			}
			filtered = append(filtered, row)
			continue
		}
		if sourceInventoryRowMatchesRequestedSurfaceFamilies(row, byRole) {
			filtered = append(filtered, row)
			continue
		}
		row.Lane = SourceInventoryRowLaneSupport
		row.ReasonCode = SourceInventoryRowReasonSurfaceFamily
		demoted = append(demoted, row)
	}
	if len(filtered) == 0 || len(filtered) == len(principal) {
		return principal, support
	}
	support = append(support, demoted...)
	return filtered, support
}

func sourceInventoryRequestedSurfaceFamiliesByRole(quotes []string, rows []SourceInventoryRow) map[AnswerCandidateRole]map[string]bool {
	out := map[AnswerCandidateRole]map[string]bool{}
	quoteKeys := sourceInventoryRequestedSurfaceQuoteKeys(quotes)
	if len(quoteKeys) == 0 {
		return out
	}
	for _, row := range rows {
		if row.Role == "" || row.Role == AnswerCandidateRoleUnknown {
			continue
		}
		for _, family := range sourceInventoryRowSurfaceFamilies(row) {
			if !sourceInventorySurfaceFamilyRequestedByQuotes(family, quoteKeys) {
				continue
			}
			byRole := out[row.Role]
			if byRole == nil {
				byRole = map[string]bool{}
				out[row.Role] = byRole
			}
			byRole[family] = true
		}
	}
	return out
}

func sourceInventoryRowMatchesRequestedSurfaceFamilies(row SourceInventoryRow, requested map[string]bool) bool {
	if len(requested) == 0 {
		return false
	}
	for _, family := range sourceInventoryRowSurfaceFamilies(row) {
		if requested[family] {
			return true
		}
	}
	return false
}

func sourceInventorySurfaceFamilyRequestedByQuotes(family string, quoteKeys []string) bool {
	family = sourceInventoryRequestedSurfaceTextKey(family)
	if family == "" {
		return false
	}
	singleTokenFamily := !strings.Contains(family, " ")
	for _, quote := range quoteKeys {
		if quote == family {
			return true
		}
		if singleTokenFamily {
			if strings.HasPrefix(quote, family+" ") {
				return true
			}
			continue
		}
		if strings.Contains(" "+quote+" ", " "+family+" ") {
			return true
		}
	}
	return false
}
