package types

import (
	"path/filepath"
	"strings"
)

type sourceInventoryProjectionFamily struct {
	sourceClass SourcePathRole
	ext         string
	language    string
}

func sourceInventoryPrincipalFactUniverseComplete(refs []AnswerAggregateFactRef, want map[string]bool) bool {
	if len(refs) == 0 || len(want) == 0 {
		return false
	}
	for _, ref := range refs {
		got := sourceInventoryAggregateFactRowKeys(ref.Fact)
		if sourceInventoryRowKeySetsEqual(got, want) && aggregateMemberSetSupportCoverage(ref.Fact) >= len(want) {
			return true
		}
	}
	return false
}

func sourceInventoryPrincipalRowSetDisjointFromExistingPrincipal(refs []AnswerAggregateFactRef, rowSet SourceInventoryPrincipalRowSet) bool {
	rowFamilies := sourceInventoryProjectionFamiliesFromRowSet(rowSet)
	if len(refs) == 0 || len(rowFamilies) == 0 {
		return false
	}
	seenGroundedPrincipal := false
	for _, ref := range refs {
		if strings.TrimSpace(ref.Fact.Provenance) == SourceInventoryPrincipalRowSetAggregateProvenance {
			continue
		}
		if aggregateMemberSetSupportCoverage(ref.Fact) == 0 {
			continue
		}
		factFamilies := sourceInventoryProjectionFamiliesFromAggregateFact(ref.Fact)
		if len(factFamilies) == 0 {
			continue
		}
		seenGroundedPrincipal = true
		if sourceInventoryProjectionFamilySetsOverlap(factFamilies, rowFamilies) {
			return false
		}
	}
	return seenGroundedPrincipal
}

func sourceInventoryProjectionFamiliesFromRowSet(rowSet SourceInventoryPrincipalRowSet) []sourceInventoryProjectionFamily {
	var out []sourceInventoryProjectionFamily
	for _, row := range sourceInventoryAllPrincipalRows(rowSet) {
		if family, ok := sourceInventoryProjectionFamilyFromPath(row.Member.File, row.Language); ok {
			if row.SourceClass != "" && row.SourceClass != SourcePathRoleUnknown {
				family.sourceClass = row.SourceClass
			}
			out = append(out, family)
			continue
		}
		if family, ok := sourceInventoryProjectionFamilyFromSupportSurface(row.Member.SupportRef); ok {
			if row.SourceClass != "" && row.SourceClass != SourcePathRoleUnknown {
				family.sourceClass = row.SourceClass
			}
			out = append(out, family)
		}
	}
	return sourceInventoryProjectionUniqueFamilies(out)
}

func sourceInventoryProjectionFamiliesFromAggregateFact(fact AnswerAggregateFact) []sourceInventoryProjectionFamily {
	var out []sourceInventoryProjectionFamily
	for i, member := range fact.Members {
		if i < len(fact.SupportRefs) {
			if family, ok := sourceInventoryProjectionFamilyFromSupportSurface(fact.SupportRefs[i]); ok {
				out = append(out, family)
			}
		}
		if family, ok := sourceInventoryProjectionFamilyFromSupportSurface(member); ok {
			out = append(out, family)
		}
	}
	return sourceInventoryProjectionUniqueFamilies(out)
}

func sourceInventoryProjectionFamilyFromSupportSurface(raw string) (sourceInventoryProjectionFamily, bool) {
	_, loc, ok := ParseAnswerSupportRefMemberLocation(raw)
	if !ok {
		return sourceInventoryProjectionFamily{}, false
	}
	return sourceInventoryProjectionFamilyFromPath(loc.File, "")
}

func sourceInventoryProjectionFamilyFromPath(path, language string) (sourceInventoryProjectionFamily, bool) {
	normalized := strings.TrimSpace(strings.ReplaceAll(path, `\`, `/`))
	if normalized == "" {
		return sourceInventoryProjectionFamily{}, false
	}
	if surface, parsed := ParseAnswerSourceLocationSurface(normalized); parsed && strings.TrimSpace(surface.File) != "" {
		normalized = strings.TrimSpace(strings.ReplaceAll(surface.File, `\`, `/`))
	}
	family := sourceInventoryProjectionFamily{
		sourceClass: ClassifySourcePathRole(normalized),
		ext:         strings.ToLower(filepath.Ext(normalized)),
		language:    strings.ToLower(strings.TrimSpace(language)),
	}
	if family.sourceClass == SourcePathRoleUnknown && family.ext == "" && family.language == "" {
		return sourceInventoryProjectionFamily{}, false
	}
	return family, true
}

func sourceInventoryProjectionFamilySetsOverlap(a, b []sourceInventoryProjectionFamily) bool {
	for _, left := range a {
		for _, right := range b {
			if sourceInventoryProjectionFamiliesCompatible(left, right) {
				return true
			}
		}
	}
	return false
}

func sourceInventoryProjectionFamiliesCompatible(a, b sourceInventoryProjectionFamily) bool {
	if a.sourceClass != "" && a.sourceClass != SourcePathRoleUnknown &&
		b.sourceClass != "" && b.sourceClass != SourcePathRoleUnknown &&
		a.sourceClass != b.sourceClass {
		return false
	}
	if a.ext != "" && b.ext != "" && a.ext != b.ext {
		return false
	}
	if a.ext == "" && b.ext == "" && a.language != "" && b.language != "" && a.language != b.language {
		return false
	}
	return true
}

func sourceInventoryProjectionUniqueFamilies(families []sourceInventoryProjectionFamily) []sourceInventoryProjectionFamily {
	if len(families) <= 1 {
		return families
	}
	seen := map[sourceInventoryProjectionFamily]bool{}
	out := make([]sourceInventoryProjectionFamily, 0, len(families))
	for _, family := range families {
		if seen[family] {
			continue
		}
		seen[family] = true
		out = append(out, family)
	}
	return out
}
