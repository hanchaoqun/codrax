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
	// TypedRelationImplicitSiblingAggregateDemotionProvenance marks an
	// otherwise-implicit member_set that yielded its default principal seat
	// after an exact typed-relation principal set was established. The model can
	// keep a genuinely requested second principal axis by explicitly emitting
	// role=principal_answer; the system never infers that intent from labels or
	// prose.
	TypedRelationImplicitSiblingAggregateDemotionProvenance = "demoted:implicit_sibling_of_typed_relation_principal_member_set"
	// TypedExclusionExactEmptyMemberSetAggregateProvenance marks a principal
	// member_set that became exactly empty because every pre-filter member was
	// removed by the analyzer-emitted typed exclusion/visibility policy. The
	// completion layer owns this token and keeps the removed rows in Excluded so
	// downstream consumers can distinguish a proven post-filter zero from a
	// malformed or accidentally emptied JSON carrier.
	TypedExclusionExactEmptyMemberSetAggregateProvenance = "system:typed_exclusion_exact_empty_member_set"
)

// AnswerAggregateFactHasTypedRelationPrincipalAuthority reports whether the
// completion tool attached the exact typed-relation authority marker.
func AnswerAggregateFactHasTypedRelationPrincipalAuthority(fact AnswerAggregateFact) bool {
	return strings.Contains(fact.Provenance, TypedRelationPrincipalMemberSetAggregateProvenance)
}

// PrincipalMemberSetRequiresTypedRelationAuthority reports the request shapes
// where individually grounded members still do not prove the principal set's
// relation, direction, order, or bridge. Relation enumerations use the
// established handoff predicate; source call-chain families add the narrative
// case where a model may otherwise line up separately valid call/value-flow
// components as one end-to-end path.
//
// The decision is entirely typed. It does not inspect the raw request,
// aggregate labels, model reasoning, or answer prose. Runtime root-cause trace
// families remain outside this source relation authority and keep their own
// causal projection contract.
func PrincipalMemberSetRequiresTypedRelationAuthority(rm RequestModel) bool {
	return RequiresRelationMemberSetHandoff(rm) || ResolveQuestionFamily(rm) == QFCallChain
}

// AnswerAggregateFactHasTypedExclusionExactEmptyAuthority reports whether the
// completion layer, rather than model prose, proved that a non-empty member set
// became empty under a typed exclusion policy.
func AnswerAggregateFactHasTypedExclusionExactEmptyAuthority(fact AnswerAggregateFact) bool {
	return strings.Contains(fact.Provenance, TypedExclusionExactEmptyMemberSetAggregateProvenance)
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
