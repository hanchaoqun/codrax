package types

import "strings"

const (
	SourceInventoryPrincipalRowSetAggregateProvenance = "system:source_inventory_principal_row_set"
	// TypedRelationPrincipalMemberSetAggregateProvenance marks a model-authored
	// principal member_set that was checked against an exact typed relation
	// roster by the completion tool. The marker lets later source-inventory
	// projection/finalization preserve the selected relation axis instead of
	// replacing it with a broader mechanical type/function inventory.
	//
	// The marker is system-authored only after structured member matching. It
	// is never inferred from labels, request prose, closure prose, or answer
	// text.
	TypedRelationPrincipalMemberSetAggregateProvenance = "system:typed_relation_principal_member_set"
)

// AnswerAggregateFactHasTypedRelationPrincipalAuthority reports whether the
// completion tool attached the exact typed-relation authority marker.
func AnswerAggregateFactHasTypedRelationPrincipalAuthority(fact AnswerAggregateFact) bool {
	return strings.Contains(fact.Provenance, TypedRelationPrincipalMemberSetAggregateProvenance)
}

// PrincipalTypedRelationMemberSetFactRefsForRequest selects marked principal
// member sets. Plain member labels are valid here because the system marker,
// rather than a punctuation parser or localized label, proves the relation
// axis.
func PrincipalTypedRelationMemberSetFactRefsForRequest(facts []AnswerAggregateFact, rm *RequestModel) []AnswerAggregateFactRef {
	refs := PrincipalAggregateMemberSetFactRefsForRequest(facts, rm)
	out := make([]AnswerAggregateFactRef, 0, len(refs))
	for _, ref := range refs {
		if AnswerAggregateFactHasTypedRelationPrincipalAuthority(ref.Fact) {
			out = append(out, ref)
		}
	}
	return out
}

func sourceInventoryAllPrincipalRows(rowSet SourceInventoryPrincipalRowSet) []SourceInventoryRow {
	rows := append([]SourceInventoryRow(nil), rowSet.PrincipalRows...)
	return sourceInventoryFamilyBalancedRows(rows)
}
