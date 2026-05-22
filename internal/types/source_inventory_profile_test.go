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

func TestSourceInventoryProfile_InfersTypeNameSubjectFromTypedInventory(t *testing.T) {
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

	subject, ok := AnswerSubjectForSourceInventoryProfile(profile)
	if !ok || subject.Kind != SubjectTypeName {
		t.Fatalf("string enum type inventory should infer type_name subject, got ok=%v subject=%+v", ok, subject)
	}
}

func TestSourceInventoryProfile_NormalizesValuesAfterTypeSubjectInference(t *testing.T) {
	profile := &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles: []AnswerCandidateRole{
			AnswerCandidateRoleType,
			AnswerCandidateRoleConstant,
		},
		TypeUnderlying:   SourceInventoryTypeUnderlyingString,
		RequiresConstSet: true,
		RequestedFields: []SourceInventoryRequestedField{
			SourceInventoryFieldName,
			SourceInventoryFieldLocation,
			SourceInventoryFieldValues,
		},
		Confidence: 0.95,
	}

	changed := NormalizeSourceInventoryRequestedFieldsForAnswerSubject(profile, AnswerSubject{Kind: SubjectTypeName})
	if !changed {
		t.Fatal("expected values field to be removed for type-name inventory")
	}
	if profile.RequestsField(SourceInventoryFieldValues) {
		t.Fatalf("values should not survive as a requested field: %+v", profile.RequestedFields)
	}
	if !profile.RequestsField(SourceInventoryFieldName) || !profile.RequestsField(SourceInventoryFieldLocation) {
		t.Fatalf("name/location fields should be preserved: %+v", profile.RequestedFields)
	}
}
