package types

import "strconv"

// sourceInventoryApplyCanonicalPrincipalFact collapses presentation aliases
// only after every member in the fact resolved to an exact typed declaration.
// Equal canonical names at different source locations remain separate.
func sourceInventoryApplyCanonicalPrincipalFact(fact *AnswerAggregateFact, members, refs []string) {
	if fact == nil {
		return
	}
	members, refs, notes := normalizeAggregateMemberSetMemberSupportNoteSurfaces(
		members,
		refs,
		fact.MemberNotes,
	)
	fact.Members = members
	fact.SupportRefs = refs
	fact.MemberNotes = notes
	fact.Value = strconv.Itoa(len(members))
	fact.Label = normalizeAggregateMemberSetLabelCardinality(fact.Label, len(members))
}
