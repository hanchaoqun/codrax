package types

import "testing"

func TestAggregateCensusDoesNotRequestInputRoster(t *testing.T) {
	base := RequestModel{
		Intent:                 IntentEnumerate,
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "entire tree"},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{
			{Role: RequestedAnswerDimensionCount, Required: true},
			{Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		}},
		SourceInventoryProfile: &SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []AnswerCandidateRole{AnswerCandidateRoleFile}, SourceQuotes: []string{"entire tree"}},
	}
	for _, scalar := range []bool{false, true} {
		rm := base
		rm.Predicates.IsCountQuestion, rm.Predicates.IsScalarAnswer = scalar, scalar
		if !RequestsAggregateWithoutMemberRoster(rm) || IsCategoryEnumerationAnswerShape(rm) || IsTypedSourceEnumerationShape(rm) || HasPrincipalAnswerSetObligation(rm) || RequiresExhaustiveEnumerationMemberSetHandoff(rm) || SourceInventoryPrincipalNavigationActive(rm) || SourceInventoryPrincipalAuthorityActive(rm) {
			t.Fatalf("input completeness became display/lens obligation (scalar=%t)", scalar)
		}
		if !rm.CompletenessObligation.IsActive() {
			t.Fatal("input coverage lost")
		}
	}
	for _, kind := range []string{"member_set", "category", "per_member", "bounded"} {
		rm := base
		switch kind {
		case "member_set":
			p := *base.RequestedAnswerDimensions
			p.Dimensions = append(append([]RequestedAnswerDimension(nil), p.Dimensions...), RequestedAnswerDimension{Role: RequestedAnswerDimensionMemberSet, Required: true})
			rm.RequestedAnswerDimensions = &p
		case "category":
			rm.Predicates.IsCategoryEnumeration = true
		case "per_member":
			rm.Predicates.HasPerMemberTable = true
		case "bounded":
			rm.EnumerationBoundary = &RequestedEnumerationBoundary{DeclaredCount: 3}
		}
		if RequestsAggregateWithoutMemberRoster(rm) || !HasPrincipalAnswerSetObligation(rm) || !RequiresExhaustiveEnumerationMemberSetHandoff(rm) {
			t.Fatalf("%s independent roster lost", kind)
		}
	}
}

func TestFileDiscoveryDoesNotDisableSemanticInventories(t *testing.T) {
	for _, roles := range [][]AnswerCandidateRole{{AnswerCandidateRoleFile}, {AnswerCandidateRoleFile, AnswerCandidateRoleFunction}, {AnswerCandidateRoleFunction}} {
		p := &SourceInventoryProfile{IsSourceInventory: true, TargetRoles: roles, SourceQuotes: []string{"scope"}}
		want := len(roles) == 1 && roles[0] == AnswerCandidateRoleFile
		if SourceInventoryIsPathDiscovery(p) != want {
			t.Fatalf("roles %v", roles)
		}
		rm := RequestModel{Intent: IntentEnumerate, SourceInventoryProfile: p}
		if SourceInventoryPrincipalNavigationActive(rm) == want {
			t.Fatalf("wrong lens routing: %v", roles)
		}
	}
}
