package types

import (
	"fmt"
	"strings"
)

type sourceInventoryPrincipalEnumerationRowRef struct {
	setIndex int
	rowIndex int
	score    int
}

// CanonicalizeSourceInventoryPrincipalEnumerationSets collapses overlapping
// model aggregate rows onto the exact typed declaration universe. It preserves
// disjoint/partial families already selected by the answer plan and chooses
// one display row for each unique typed family+location coordinate. Ambiguous
// typed coordinates fail open. No request, reasoning, prompt or answer prose is
// read; this is the shared projection for finalizer teaching and hard checks.
func CanonicalizeSourceInventoryPrincipalEnumerationSets(
	sets []EnumerationDisplaySet,
	principalRows []SourceInventoryRow,
) []EnumerationDisplaySet {
	if len(sets) == 0 || len(principalRows) == 0 {
		return sets
	}
	canonicalNames := map[string]string{}
	ambiguous := map[string]bool{}
	for _, row := range principalRows {
		coord := sourceInventoryPrincipalEnumerationCoordinate(row.SurfaceFamily, sourceInventoryPrincipalEnumerationLocation(row))
		if coord == "" {
			continue
		}
		name := strings.TrimSpace(row.Member.Name)
		if previous, ok := canonicalNames[coord]; ok && !strings.EqualFold(previous, name) {
			ambiguous[coord] = true
			continue
		}
		canonicalNames[coord] = name
	}
	for coord := range ambiguous {
		delete(canonicalNames, coord)
	}
	if len(canonicalNames) == 0 {
		return sets
	}

	winners := map[string]sourceInventoryPrincipalEnumerationRowRef{}
	for setIndex, set := range sets {
		for rowIndex, row := range set.Rows {
			coord := sourceInventoryPrincipalEnumerationCoordinate(SourceInventorySurfaceFamilyKey(row.SurfaceTerms), row.Location)
			canonicalName, ok := canonicalNames[coord]
			if !ok {
				continue
			}
			candidate := sourceInventoryPrincipalEnumerationRowRef{
				setIndex: setIndex,
				rowIndex: rowIndex,
				score:    sourceInventoryPrincipalEnumerationCanonicalRowScore(row, canonicalName),
			}
			if previous, exists := winners[coord]; !exists || candidate.score > previous.score {
				winners[coord] = candidate
			}
		}
	}

	out := make([]EnumerationDisplaySet, 0, len(sets))
	for setIndex, set := range sets {
		rows := make([]EnumerationDisplayRow, 0, len(set.Rows))
		for rowIndex, row := range set.Rows {
			coord := sourceInventoryPrincipalEnumerationCoordinate(SourceInventorySurfaceFamilyKey(row.SurfaceTerms), row.Location)
			winner, canonical := winners[coord]
			if canonical && (winner.setIndex != setIndex || winner.rowIndex != rowIndex) {
				continue
			}
			rows = append(rows, row)
		}
		if len(rows) == 0 {
			continue
		}
		set.Rows = rows
		out = append(out, set)
	}
	return out
}

func sourceInventoryPrincipalEnumerationCoordinate(family, location string) string {
	family = strings.ToLower(strings.TrimSpace(family))
	location = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(location, `\`, `/`)))
	if family == "" || location == "" {
		return ""
	}
	return family + "\x00" + location
}

func sourceInventoryPrincipalEnumerationLocation(row SourceInventoryRow) string {
	if _, loc, ok := ParseAnswerSupportRefMemberLocation(row.Member.SupportRef); ok {
		file := strings.TrimSpace(strings.ReplaceAll(loc.File, `\`, `/`))
		if file != "" && loc.LineStart > 0 {
			return fmt.Sprintf("%s:%d", file, loc.LineStart)
		}
	}
	file := strings.TrimSpace(strings.ReplaceAll(row.Member.File, `\`, `/`))
	if file == "" || row.Member.Line <= 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d", file, row.Member.Line)
}

func sourceInventoryPrincipalEnumerationCanonicalRowScore(row EnumerationDisplayRow, canonicalName string) int {
	score := 0
	canonicalName = strings.TrimSpace(canonicalName)
	if canonicalName != "" && strings.EqualFold(strings.TrimSpace(row.Member), canonicalName) {
		score += 8
	}
	if canonicalName != "" && strings.EqualFold(strings.TrimSpace(row.DisplayLabel), canonicalName) {
		score += 4
	}
	if row.MemberSurface == PrincipalMemberSurfaceSymbolLike {
		score += 2
	}
	if strings.TrimSpace(row.EvidenceID) != "" {
		score++
	}
	return score
}
