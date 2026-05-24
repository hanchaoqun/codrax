package types

import "testing"

func TestEvidenceRelationCandidateSource_RegistrationTargetToRegistrar(t *testing.T) {
	rows := (EvidenceRelationCandidateSource{Items: []EvidenceItem{{
		Kind:            EvidenceRegistration,
		Subject:         "RegisterUserUnlockObserver",
		Object:          "USER_UNLOCK",
		Source:          "services/user_unlock_observer.cpp",
		LineStart:       24,
		AnchorKind:      AnchorCall,
		AnchorSymbol:    "RegisterUserUnlockObserver",
		GroundingStatus: GroundingGrounded,
		GroundingTier:   TierLineText,
	}}}).TypedRelationCandidates(TypedRelationQuery{
		Kinds:      []TypedRelationKind{TypedRelationRegisters},
		Sources:    []string{"USER_UNLOCK"},
		Purpose:    TypedRelationPurposePromptHint,
		MaxMembers: 10,
	})
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want one registration row", rows)
	}
	row := rows[0]
	if row.Relation != TypedRelationRegisters || row.SourceName != "USER_UNLOCK" {
		t.Fatalf("unexpected relation/source: %+v", row)
	}
	if row.Member.Name != "RegisterUserUnlockObserver" || row.Member.File != "services/user_unlock_observer.cpp" || row.Member.Line != 24 {
		t.Fatalf("unexpected member: %+v", row.Member)
	}
	if row.Carrier != TypedRelationCarrierEvidence || row.Precision != TypedRelationPrecisionExactEvidence {
		t.Fatalf("unexpected provenance: carrier=%s precision=%s", row.Carrier, row.Precision)
	}
}

func TestEvidenceRelationCandidateSource_RegistrarToRegisteredTarget(t *testing.T) {
	rows := (EvidenceRelationCandidateSource{Items: []EvidenceItem{{
		Kind:            EvidenceRegistration,
		Subject:         "RegisterFeature",
		Object:          "FeatureA",
		Source:          "./internal/registry.go",
		LineStart:       42,
		AnchorKind:      AnchorAssignment,
		AnchorSymbol:    "FeatureA",
		GroundingStatus: GroundingGrounded,
		GroundingTier:   TierLineText,
	}}}).TypedRelationCandidates(TypedRelationQuery{
		Kinds:   []TypedRelationKind{TypedRelationRegisters},
		Sources: []string{"RegisterFeature"},
		Purpose: TypedRelationPurposePromptHint,
	})
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want one registration row", rows)
	}
	if rows[0].Member.Name != "FeatureA" || rows[0].Member.File != "internal/registry.go" {
		t.Fatalf("unexpected registered target member: %+v", rows[0].Member)
	}
}

func TestEvidenceRelationCandidateSource_CoverageRequiresPrincipalEvidence(t *testing.T) {
	supporting := EvidenceItem{
		Kind:            EvidenceRegistration,
		Subject:         "RegisterFeature",
		Object:          "FeatureA",
		Source:          "internal/registry.go",
		LineStart:       42,
		AnchorKind:      AnchorAssignment,
		AnchorSymbol:    "FeatureA",
		GroundingStatus: GroundingGrounded,
		GroundingTier:   TierLineText,
		Salience:        SalienceSupporting,
	}
	query := TypedRelationQuery{
		Kinds:   []TypedRelationKind{TypedRelationRegisters},
		Sources: []string{"FeatureA"},
		Purpose: TypedRelationPurposeCoverageGate,
	}
	if rows := (EvidenceRelationCandidateSource{Items: []EvidenceItem{supporting}}).TypedRelationCandidates(query); len(rows) != 0 {
		t.Fatalf("supporting evidence must not become coverage-gate candidate: %+v", rows)
	}
	supporting.Salience = SalienceExhaustListed
	rows := (EvidenceRelationCandidateSource{Items: []EvidenceItem{supporting}}).TypedRelationCandidates(query)
	if len(rows) != 1 || rows[0].Precision != TypedRelationPrecisionExactEvidence {
		t.Fatalf("principal registration evidence should become exact coverage candidate: %+v", rows)
	}
}

func TestEvidenceRelationCandidateSource_IgnoresNonRegistrationAndUngroundedRows(t *testing.T) {
	items := []EvidenceItem{
		{
			Kind:            EvidenceRelationship,
			Subject:         "RegisterFeature",
			Object:          "FeatureA",
			Source:          "internal/registry.go",
			LineStart:       42,
			AnchorKind:      AnchorAssignment,
			AnchorSymbol:    "FeatureA",
			GroundingStatus: GroundingGrounded,
		},
		{
			Kind:            EvidenceRegistration,
			Subject:         "RegisterOther",
			Object:          "FeatureA",
			Source:          "internal/registry.go",
			LineStart:       45,
			AnchorKind:      AnchorAssignment,
			AnchorSymbol:    "FeatureA",
			GroundingStatus: GroundingUngrounded,
		},
	}
	rows := (EvidenceRelationCandidateSource{Items: items}).TypedRelationCandidates(TypedRelationQuery{
		Kinds:   []TypedRelationKind{TypedRelationRegisters},
		Sources: []string{"FeatureA"},
		Purpose: TypedRelationPurposePromptHint,
	})
	if len(rows) != 0 {
		t.Fatalf("non-registration/ungrounded rows must be ignored: %+v", rows)
	}
}
