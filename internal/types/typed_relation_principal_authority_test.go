package types

import "testing"

func TestPrincipalTypedRelationMemberNamesForRequestUsesOnlyMarkedPrincipalSets(t *testing.T) {
	rm := &RequestModel{
		Intent:        IntentEnumerate,
		PredicateAxis: AxisImplement,
		Predicates: SemanticPredicates{
			IsRelationalLookup:    true,
			IsCategoryEnumeration: true,
		},
	}
	facts := []AnswerAggregateFact{
		{
			Kind:       AnswerAggregateMemberSet,
			Role:       AnswerAggregateRolePrincipalAnswer,
			Provenance: TypedRelationPrincipalMemberSetAggregateProvenance,
			Members:    []string{"ProdOne", "pkg.ProdTwo", "ProdOne"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"TestHelper"},
		},
	}
	got := PrincipalTypedRelationMemberNamesForRequest(facts, rm)
	if len(got) != 2 || got[0] != "ProdOne" || got[1] != "pkg.ProdTwo" {
		t.Fatalf("principal typed members = %#v, want exact marked set only", got)
	}
	if !PrincipalTypedRelationMemberMatches("pkg::ProdTwo", got) {
		t.Fatal("language-native separator variants must match the shared relation identity bridge")
	}
	if PrincipalTypedRelationMemberMatches("TestHelper", got) {
		t.Fatal("unmarked supporting member expanded the principal relation slate")
	}
}
