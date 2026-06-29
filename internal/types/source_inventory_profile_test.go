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

func TestNormalizeAnswerCandidateRole_SourceInventoryConstructFamilyAliases(t *testing.T) {
	tests := []struct {
		raw  string
		want AnswerCandidateRole
	}{
		{raw: "public_class", want: AnswerCandidateRoleType},
		{raw: "class_declaration", want: AnswerCandidateRoleType},
		{raw: "extend_block", want: AnswerCandidateRoleType},
		{raw: "foreign_func", want: AnswerCandidateRoleFunction},
		{raw: "native_function", want: AnswerCandidateRoleFunction},
		{raw: "method_declaration", want: AnswerCandidateRoleMethod},
		{raw: "module_declaration", want: AnswerCandidateRolePackage},
	}
	for _, tt := range tests {
		got, ok := NormalizeAnswerCandidateRole(tt.raw)
		if !ok || got != tt.want {
			t.Fatalf("NormalizeAnswerCandidateRole(%q) = %q, %v; want %q", tt.raw, got, ok, tt.want)
		}
	}
	if got, ok := NormalizeAnswerCandidateRole("decorator_occurrence"); ok || got != AnswerCandidateRoleUnknown {
		t.Fatalf("ambiguous construct occurrence should remain invalid, got %q ok=%v", got, ok)
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

func TestSourceInventoryProfile_NormalizesPackageDisplayRoleDrift(t *testing.T) {
	profile := &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles: []AnswerCandidateRole{
			AnswerCandidateRoleFunction,
			AnswerCandidateRoleType,
			AnswerCandidateRolePackage,
		},
		RequestedFields: []SourceInventoryRequestedField{
			SourceInventoryFieldName,
			SourceInventoryFieldLocation,
			SourceInventoryFieldPackage,
			SourceInventoryFieldModule,
			SourceInventoryFieldNamespace,
		},
		Confidence: 0.95,
	}

	removed := NormalizeSourceInventoryDisplayAttributeRoles(profile)
	if len(removed) != 1 || removed[0] != AnswerCandidateRolePackage {
		t.Fatalf("removed roles = %+v, want package", removed)
	}
	got := profile.PrincipalTargetRoles()
	if len(got) != 2 || got[0] != AnswerCandidateRoleFunction || got[1] != AnswerCandidateRoleType {
		t.Fatalf("package display field should not remain principal with structural roles, got %+v", got)
	}
	if profile.RequiresPrincipalRole(AnswerCandidateRolePackage) {
		t.Fatalf("package requested field must not be a principal role: %+v", profile)
	}
}

func TestSourceInventoryProfile_KeepsPackageOnlyInventoryPrincipal(t *testing.T) {
	profile := &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRolePackage},
		RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldPackage},
		Confidence:        0.95,
	}

	if removed := NormalizeSourceInventoryDisplayAttributeRoles(profile); len(removed) != 0 {
		t.Fatalf("package-only inventory should stay principal, removed %+v", removed)
	}
	got := profile.PrincipalTargetRoles()
	if len(got) != 1 || got[0] != AnswerCandidateRolePackage {
		t.Fatalf("package-only inventory principal roles = %+v", got)
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

func TestSourceInventoryProfile_NormalizesValuesAfterFunctionNameSubjectInference(t *testing.T) {
	profile := &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
		RequestedFields: []SourceInventoryRequestedField{
			SourceInventoryFieldName,
			SourceInventoryFieldLocation,
			SourceInventoryFieldValues,
		},
		Confidence: 0.95,
	}

	changed := NormalizeSourceInventoryRequestedFieldsForAnswerSubject(profile, AnswerSubject{Kind: SubjectFunctionName})
	if !changed {
		t.Fatal("expected values field to be removed for function-name inventory")
	}
	if profile.RequestsField(SourceInventoryFieldValues) {
		t.Fatalf("values should not survive as a requested field: %+v", profile.RequestedFields)
	}
	if !profile.MechanicalRowsOnly() {
		t.Fatalf("function name/location inventory should stay mechanical after normalization: %+v", profile)
	}
}

func TestSourceInventoryProfile_KeepsValuesForReturnValueSubject(t *testing.T) {
	profile := &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
		RequestedFields: []SourceInventoryRequestedField{
			SourceInventoryFieldName,
			SourceInventoryFieldValues,
		},
		Confidence: 0.95,
	}

	if NormalizeSourceInventoryRequestedFieldsForAnswerSubject(profile, AnswerSubject{Kind: SubjectReturnValue}) {
		t.Fatal("return-value subject must keep values as source-text support")
	}
	if !profile.RequestsField(SourceInventoryFieldValues) {
		t.Fatalf("values should remain for return-value inventory support: %+v", profile.RequestedFields)
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
