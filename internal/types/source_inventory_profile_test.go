package types

import "testing"

func TestNormalizeAnswerCandidateRole_FieldAliases(t *testing.T) {
	for _, raw := range []string{"struct_field", "object_field", "field_member"} {
		got, ok := NormalizeAnswerCandidateRole(raw)
		if !ok || got != AnswerCandidateRoleField {
			t.Fatalf("NormalizeAnswerCandidateRole(%q) = %q, %v; want field", raw, got, ok)
		}
	}
}

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
			SourceInventoryFieldPackage,
			SourceInventoryFieldModule,
			SourceInventoryFieldNamespace,
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
	if !profile.RequestsField(SourceInventoryFieldPackage) ||
		!profile.RequestsField(SourceInventoryFieldModule) ||
		!profile.RequestsField(SourceInventoryFieldNamespace) {
		t.Fatalf("package/module/namespace fields should be preserved: %+v", profile.RequestedFields)
	}
}

func TestSourceInventoryProfile_MechanicalRowsOnlyBoundary(t *testing.T) {
	mechanical := &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
		RequestedFields: []SourceInventoryRequestedField{
			SourceInventoryFieldName,
			SourceInventoryFieldLocation,
			SourceInventoryFieldCount,
			SourceInventoryFieldPackage,
		},
	}
	if !mechanical.MechanicalRowsOnly() || mechanical.RequestsSourceText() {
		t.Fatalf("mechanical inventory should not require source text: %+v", mechanical)
	}

	for _, field := range []SourceInventoryRequestedField{SourceInventoryFieldSummary, SourceInventoryFieldValues} {
		profile := &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
			RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, field},
		}
		if profile.MechanicalRowsOnly() || !profile.RequestsSourceText() {
			t.Fatalf("%s inventory should require source text: %+v", field, profile)
		}
	}
}
