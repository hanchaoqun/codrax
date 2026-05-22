package types

import "testing"

func TestSourceInventoryProfile_PrincipalTargetRolesTreatsConstSetAsQualifier(t *testing.T) {
	profile := &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles: []AnswerCandidateRole{
			AnswerCandidateRoleType,
			AnswerCandidateRoleConstant,
		},
		TypeUnderlying:   SourceInventoryTypeUnderlyingString,
		RequiresConstSet: true,
		Confidence:       0.95,
	}
	got := profile.PrincipalTargetRoles()
	if len(got) != 1 || got[0] != AnswerCandidateRoleType {
		t.Fatalf("const-set string enum inventory should expose type as the principal role, got %+v", got)
	}
	if !profile.RequiresPrincipalRole(AnswerCandidateRoleType) ||
		profile.RequiresPrincipalRole(AnswerCandidateRoleConstant) {
		t.Fatalf("principal role predicate should distinguish type from const-set qualifier")
	}
}
