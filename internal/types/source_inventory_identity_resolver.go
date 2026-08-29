package types

import "strings"

// sourceInventoryResolveUniquePrincipalRow keeps short display paths on the
// identity lane: a suffix is accepted only with an exact line, compatible
// structured labels, and one unique typed principal-row key.
func sourceInventoryResolveUniquePrincipalRow(
	rows []SourceInventoryRow,
	rowsByLocation map[string]SourceInventoryRow,
	loc AnswerSourceLocationSurface,
	refLabel string,
	memberLabel string,
) (SourceInventoryRow, bool) {
	if row, ok := rowsByLocation[sourceInventoryProjectionExactLocationKey(loc)]; ok {
		return row, true
	}
	wantFile := normalizeAnswerSupportPath(loc.File)
	if wantFile == "" || loc.LineStart <= 0 {
		return SourceInventoryRow{}, false
	}
	byKey := map[string]SourceInventoryRow{}
	for _, row := range rows {
		rowLocation := sourceInventoryPrincipalRowLocation(row)
		_, rowLoc, ok := ParseAnswerSupportRefMemberLocation(rowLocation)
		if !ok {
			if parsed, parsedOK := ParseAnswerSourceLocationSurface(rowLocation); parsedOK {
				rowLoc = parsed
				ok = true
			}
		}
		if !ok || rowLoc.LineStart != loc.LineStart ||
			!sourceInventoryProjectionPathSuffixMatches(wantFile, normalizeAnswerSupportPath(rowLoc.File)) {
			continue
		}
		canonical := sourceInventoryPrincipalRowMemberLabel(row)
		if canonical == "" ||
			!aggregateSupportRefCanDescribeMember(refLabel, canonical) ||
			!aggregateSupportRefCanDescribeMember(memberLabel, canonical) {
			continue
		}
		key := sourceInventoryPrincipalRowKey(row)
		if key == "" {
			continue
		}
		byKey[key] = row
	}
	if len(byKey) != 1 {
		return SourceInventoryRow{}, false
	}
	for _, row := range byKey {
		return row, true
	}
	return SourceInventoryRow{}, false
}

func sourceInventoryProjectionPathSuffixMatches(a string, b string) bool {
	a = strings.Trim(strings.TrimSpace(a), "/")
	b = strings.Trim(strings.TrimSpace(b), "/")
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.HasSuffix(a, "/"+b) || strings.HasSuffix(b, "/"+a)
}
