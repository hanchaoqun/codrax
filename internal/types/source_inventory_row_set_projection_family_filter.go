package types

import "strings"

func sourceInventoryProjectionFamilyFromRow(row SourceInventoryRow) (sourceInventoryProjectionFamily, bool) {
	if family, ok := sourceInventoryProjectionFamilyFromPath(row.Member.File, row.Language); ok {
		if row.SourceClass != "" && row.SourceClass != SourcePathRoleUnknown {
			family.sourceClass = row.SourceClass
		}
		return family, true
	}
	if family, ok := sourceInventoryProjectionFamilyFromSupportSurface(row.Member.SupportRef); ok {
		if row.SourceClass != "" && row.SourceClass != SourcePathRoleUnknown {
			family.sourceClass = row.SourceClass
		}
		return family, true
	}
	return sourceInventoryProjectionFamily{}, false
}

func sourceInventoryFilterPrincipalRowSetToExistingPrincipalFamilies(rowSet SourceInventoryPrincipalRowSet, refs []AnswerAggregateFactRef) SourceInventoryPrincipalRowSet {
	families := sourceInventoryProjectionFamiliesFromPrincipalRefs(refs)
	if len(families) == 0 || len(rowSet.PrincipalRows) == 0 {
		return rowSet
	}
	filtered := make([]SourceInventoryRow, 0, len(rowSet.PrincipalRows))
	for _, row := range rowSet.PrincipalRows {
		family, ok := sourceInventoryProjectionFamilyFromRow(row)
		if !ok {
			continue
		}
		if sourceInventoryProjectionFamilySetsLooselyOverlap([]sourceInventoryProjectionFamily{family}, families) {
			filtered = append(filtered, row)
		}
	}
	if len(filtered) == len(rowSet.PrincipalRows) {
		return rowSet
	}
	rowSet.PrincipalRows = filtered
	rowSet.PrincipalHiddenCount = 0
	rowSet.PrincipalTotal = len(filtered)
	return NormalizeSourceInventoryPrincipalRowSet(rowSet)
}

func sourceInventoryProjectionFamiliesFromPrincipalRefs(refs []AnswerAggregateFactRef) []sourceInventoryProjectionFamily {
	var out []sourceInventoryProjectionFamily
	for _, ref := range refs {
		if strings.TrimSpace(ref.Fact.Provenance) == SourceInventoryPrincipalRowSetAggregateProvenance {
			continue
		}
		if aggregateMemberSetSupportCoverage(ref.Fact) == 0 {
			continue
		}
		out = append(out, sourceInventoryProjectionFamiliesFromAggregateFact(ref.Fact)...)
	}
	return sourceInventoryProjectionUniqueFamilies(out)
}

func sourceInventoryProjectionFamilySetsLooselyOverlap(a, b []sourceInventoryProjectionFamily) bool {
	for _, left := range a {
		for _, right := range b {
			if sourceInventoryProjectionFamiliesLooselyCompatible(left, right) {
				return true
			}
		}
	}
	return false
}

func sourceInventoryProjectionFamiliesLooselyCompatible(a, b sourceInventoryProjectionFamily) bool {
	if a.ext != "" && b.ext != "" {
		return a.ext == b.ext
	}
	if a.language != "" && b.language != "" {
		return a.language == b.language
	}
	return sourceInventoryProjectionFamiliesCompatible(a, b)
}
