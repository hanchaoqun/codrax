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
		familySelected := false
		for _, family := range sourceInventoryProjectionSurfaceFamiliesForRow(row) {
			if selectedFamilies[family] {
				familySelected = true
				break
			}
		}
		if familySelected {
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

func sourceInventoryPrincipalFactsExactlyCoverRows(refs []AnswerAggregateFactRef, want map[string]bool) bool {
	if len(refs) == 0 || len(want) == 0 {
		return false
	}
	covered := map[string]bool{}
	for _, ref := range refs {
		if aggregateMemberSetSupportCoverage(ref.Fact) == 0 {
			continue
		}
		factKeys := sourceInventoryAggregateFactRowKeys(ref.Fact)
		overlaps := false
		for key := range factKeys {
			if want[key] {
				overlaps = true
				break
			}
		}
		if !overlaps {
			// A separate typed selection family may legitimately share the same
			// answer while remaining outside this row-set universe. It neither
			// proves nor contaminates row-set coverage.
			continue
		}
		for key := range factKeys {
			if !want[key] {
				// A fact that overlaps the typed universe and also adds an
				// out-of-universe member is still an unsafe superset.
				return false
			}
			covered[key] = true
		}
	}
	// Coverage alone is insufficient: an overlapping model member_set may
	// contain every requested production row and append test/generated rows.
	// Treating that superset as equivalent makes those out-of-universe rows hard
	// answer obligations. Equality remains the precise typed contract for the
	// facts that actually overlap this row-set universe.
	if len(covered) != len(want) {
		return false
	}
	for key := range want {
		if !covered[key] {
			return false
		}
	}
	return true
}
