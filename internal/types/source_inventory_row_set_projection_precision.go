package types

import "strings"

const SourceInventoryRowReasonMixedUniverse = "mixed_symbol_universe"

func sourceInventoryFilterMixedPrincipalRowSetToExistingRows(rowSet SourceInventoryPrincipalRowSet, refs []AnswerAggregateFactRef) (SourceInventoryPrincipalRowSet, bool) {
	if !sourceInventoryPrincipalRolesAreMixedSymbolUniverse(rowSet.PrincipalRoles) || len(refs) == 0 || len(rowSet.PrincipalRows) == 0 {
		return rowSet, false
	}
	existing := sourceInventoryPrincipalRefRowKeys(refs)
	if len(existing) == 0 {
		return rowSet, false
	}
	selectedFamilies := sourceInventorySelectedSurfaceFamilies(rowSet.PrincipalRows, existing)
	filtered := make([]SourceInventoryRow, 0, len(rowSet.PrincipalRows))
	var demoted []SourceInventoryRow
	for _, row := range rowSet.PrincipalRows {
		key := sourceInventoryPrincipalRowKey(row)
		if key != "" && existing[key] {
			filtered = append(filtered, row)
			continue
		}
		if family, ok := sourceInventoryProjectionSurfaceFamilyForRow(row); ok && selectedFamilies[family] {
			filtered = append(filtered, row)
			continue
		}
		row.Lane = SourceInventoryRowLaneSupport
		row.ReasonCode = SourceInventoryRowReasonMixedUniverse
		demoted = append(demoted, row)
	}
	if len(filtered) == len(rowSet.PrincipalRows) {
		return rowSet, false
	}
	rowSet.PrincipalRows = filtered
	rowSet.SupportRows = append(rowSet.SupportRows, demoted...)
	rowSet.PrincipalHiddenCount = 0
	rowSet.PrincipalTotal = len(filtered)
	rowSet.SupportTotal += len(demoted)
	return NormalizeSourceInventoryPrincipalRowSet(rowSet), true
}

func sourceInventoryPrincipalRefRowKeys(refs []AnswerAggregateFactRef) map[string]bool {
	out := map[string]bool{}
	for _, ref := range refs {
		if strings.TrimSpace(ref.Fact.Provenance) == SourceInventoryPrincipalRowSetAggregateProvenance {
			continue
		}
		if aggregateMemberSetSupportCoverage(ref.Fact) == 0 {
			continue
		}
		for key := range sourceInventoryAggregateFactRowKeys(ref.Fact) {
			if key != "" {
				out[key] = true
			}
		}
	}
	return out
}

func sourceInventoryPrincipalFactsCoverRows(refs []AnswerAggregateFactRef, want map[string]bool) bool {
	if len(refs) == 0 || len(want) == 0 {
		return false
	}
	covered := map[string]bool{}
	for _, ref := range refs {
		if aggregateMemberSetSupportCoverage(ref.Fact) == 0 {
			continue
		}
		for key := range sourceInventoryAggregateFactRowKeys(ref.Fact) {
			if want[key] {
				covered[key] = true
			}
		}
	}
	if len(covered) < len(want) {
		return false
	}
	for key := range want {
		if !covered[key] {
			return false
		}
	}
	return true
}

func sourceInventoryPrincipalRowAttributeNotes(attrs []SourceInventoryObservationAttribute) []string {
	var out []string
	for _, attr := range sourceInventoryRowContextAttributes(attrs) {
		name := strings.TrimSpace(attr.Name)
		if name == "" {
			continue
		}
		role := strings.TrimSpace(string(attr.Role))
		if role == "" {
			role = "attribute"
		}
		if attr.Location != "" {
			out = append(out, role+"="+name+" @ "+attr.Location)
		} else {
			out = append(out, role+"="+name)
		}
	}
	return out
}
