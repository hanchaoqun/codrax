package types

import "strings"

// sourceInventoryCanonicalizePrincipalFactMemberIdentities separates a
// model-owned source-inventory selection from presentation detail embedded in
// its structured member strings. It is deliberately all-or-nothing per fact:
// every member must have an index-aligned support_ref and both model labels
// must agree with one unique typed row. Full coordinates win; short paths need
// exact line, compatible labels, and one unique key. Ambiguity stays fail-closed.
func sourceInventoryCanonicalizePrincipalFactMemberIdentities(
	facts []AnswerAggregateFact,
	rowSet SourceInventoryPrincipalRowSet,
	rm RequestModel,
) []AnswerAggregateFact {
	if len(facts) == 0 || !rowSet.Active || len(rowSet.PrincipalRows) == 0 {
		return facts
	}
	rowsByLocation := sourceInventoryUniquePrincipalRowsByLocation(rowSet.PrincipalRows)
	if len(rowsByLocation) == 0 {
		return facts
	}
	for i := range facts {
		fact := facts[i]
		if fact.Kind != AnswerAggregateMemberSet ||
			AnswerAggregateFactRoleForRequest(fact, &rm) != AnswerAggregateRolePrincipalAnswer ||
			len(fact.Members) == 0 || len(fact.SupportRefs) != len(fact.Members) {
			continue
		}
		members := make([]string, len(fact.Members))
		refs := make([]string, len(fact.Members))
		complete := true
		for memberIdx, member := range fact.Members {
			refLabel, loc, ok := ParseAnswerSupportRefMemberLocation(fact.SupportRefs[memberIdx])
			if !ok {
				complete = false
				break
			}
			memberLabel := sourceInventoryStructuredMemberBaseLabel(member)
			row, ok := sourceInventoryResolveUniquePrincipalRow(rowSet.PrincipalRows, rowsByLocation, loc, refLabel, memberLabel)
			if !ok {
				complete = false
				break
			}
			canonical := sourceInventoryPrincipalRowMemberLabel(row)
			if canonical == "" ||
				!sourceInventoryPrincipalRowAcceptsStructuredLabel(row, refLabel) ||
				!sourceInventoryPrincipalRowAcceptsStructuredLabel(row, memberLabel) {
				complete = false
				break
			}
			members[memberIdx] = canonical
			refs[memberIdx] = sourceInventoryPrincipalRowSupportRef(row, canonical)
			if refs[memberIdx] == "" {
				complete = false
				break
			}
		}
		if !complete {
			continue
		}
		sourceInventoryApplyCanonicalPrincipalFact(&facts[i], members, refs)
	}
	return facts
}

func sourceInventoryUniquePrincipalRowsByLocation(rows []SourceInventoryRow) map[string]SourceInventoryRow {
	out := map[string]SourceInventoryRow{}
	ambiguous := map[string]bool{}
	for _, row := range rows {
		location := sourceInventoryPrincipalRowLocation(row)
		if location == "" {
			continue
		}
		surface, ok := ParseAnswerSourceLocationSurface(location)
		if !ok {
			continue
		}
		key := sourceInventoryProjectionExactLocationKey(surface)
		if key == "" || ambiguous[key] {
			continue
		}
		if existing, found := out[key]; found {
			if sourceInventoryPrincipalRowKey(existing) != sourceInventoryPrincipalRowKey(row) {
				delete(out, key)
				ambiguous[key] = true
			}
			continue
		}
		out[key] = row
	}
	return out
}

func sourceInventoryProjectionExactLocationKey(loc AnswerSourceLocationSurface) string {
	file := normalizeAnswerSupportPath(loc.File)
	if file == "" || loc.LineStart <= 0 {
		return ""
	}
	return aggregateSupportLocationKeyForDisplay(file, loc.LineStart)
}

func sourceInventoryStructuredMemberBaseLabel(member string) string {
	member = strings.TrimSpace(member)
	if label, _, ok := ParseAnswerSupportRefMemberLocation(member); ok && strings.TrimSpace(label) != "" {
		member = strings.TrimSpace(label)
	}
	return aggregateMemberSupportSurfaceLabel(member)
}
