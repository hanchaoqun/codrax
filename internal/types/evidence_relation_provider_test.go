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

func TestEvidenceRelationCandidateSource_BridgeLiteralJoinToRegisteredIdentity(t *testing.T) {
	terminal := EvidenceItem{
		ID:              "terminal-explorer",
		Kind:            EvidenceConcrete,
		Subject:         "SubExplorer.Name",
		Predicate:       "returns",
		Object:          `"explorer"`,
		Source:          "internal/agent/sub_explorer.go",
		LineStart:       33,
		LineEnd:         33,
		Producer:        "bridge_literal_terminal",
		Scope:           ScopeLine,
		AnchorKind:      AnchorReturn,
		AnchorSymbol:    "Name",
		OwnerSymbol:     "SubExplorer",
		GroundingStatus: GroundingGrounded,
	}
	bridge := EvidenceItem{
		ID:              "bridge-default-subagent",
		Kind:            EvidenceDataflowPath,
		Predicate:       "resolution_chain",
		Object:          `"explorer"`,
		Source:          "internal/agent/subagent.go",
		LineStart:       64,
		LineEnd:         64,
		DerivedFrom:     []string{terminal.ID},
		Producer:        "bridge_literal",
		Scope:           ScopeLine,
		AnchorSymbol:    "SubExplorer.Name",
		OwnerSymbol:     "RegisterDefaultSubAgents",
		GroundingStatus: GroundingGrounded,
	}
	rows := (EvidenceRelationCandidateSource{Items: []EvidenceItem{bridge, terminal}}).TypedRelationCandidates(TypedRelationQuery{
		Kinds:   []TypedRelationKind{TypedRelationRegisters},
		Sources: []string{"RegisterDefaultSubAgents"},
		Purpose: TypedRelationPurposeCoverageGate,
	})
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want one exact composite registration row", rows)
	}
	row := rows[0]
	if row.Relation != TypedRelationRegisters || row.SourceName != "RegisterDefaultSubAgents" ||
		row.SourceFile != "internal/agent/subagent.go" || row.SourceLine != 64 ||
		row.Member.Name != "explorer" || row.Member.File != "internal/agent/sub_explorer.go" || row.Member.Line != 33 {
		t.Fatalf("unexpected bridge-literal registration candidate: %+v", row)
	}
	if row.Carrier != TypedRelationCarrierEvidence || row.Precision != TypedRelationPrecisionExactEvidence {
		t.Fatalf("bridge-literal relation lost exact evidence authority: %+v", row)
	}
}

func TestEvidenceRelationCandidateSource_BridgeLiteralJoinFailsClosedWithoutExactTerminal(t *testing.T) {
	terminal := EvidenceItem{
		ID: "terminal-explorer", Kind: EvidenceConcrete, Subject: "SubExplorer.Name",
		Predicate: "returns", Object: `"explorer"`, Source: "internal/agent/sub_explorer.go", LineStart: 33,
		Producer: "bridge_literal_terminal", Scope: ScopeLine, GroundingStatus: GroundingGrounded,
	}
	base := EvidenceItem{
		ID: "bridge-default-subagent", Kind: EvidenceDataflowPath, Predicate: "resolution_chain",
		Object: `"explorer"`, Source: "internal/agent/subagent.go", LineStart: 64,
		DerivedFrom: []string{terminal.ID}, Producer: "bridge_literal", Scope: ScopeLine,
		AnchorSymbol: "SubExplorer.Name", OwnerSymbol: "RegisterDefaultSubAgents", GroundingStatus: GroundingGrounded,
	}
	query := TypedRelationQuery{
		Kinds: []TypedRelationKind{TypedRelationRegisters}, Sources: []string{"RegisterDefaultSubAgents"},
		Purpose: TypedRelationPurposeCoverageGate,
	}
	assertNone := func(name string, items []EvidenceItem) {
		t.Helper()
		if rows := (EvidenceRelationCandidateSource{Items: items}).TypedRelationCandidates(query); len(rows) != 0 {
			t.Fatalf("%s must fail closed, got %+v", name, rows)
		}
	}
	assertNone("missing terminal", []EvidenceItem{base})
	mismatch := terminal
	mismatch.Object = `"writer"`
	assertNone("literal mismatch", []EvidenceItem{base, mismatch})
	wrongOwner := base
	wrongOwner.OwnerSymbol = "RegisterOtherAgents"
	assertNone("source mismatch", []EvidenceItem{wrongOwner, terminal})
	ungrounded := terminal
	ungrounded.GroundingStatus = GroundingUngrounded
	assertNone("ungrounded terminal", []EvidenceItem{base, ungrounded})
	spoof := base
	spoof.Producer = "model_bridge_literal"
	assertNone("wrong producer", []EvidenceItem{spoof, terminal})
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

func TestEvidenceRelationCandidateSource_ConfigKeyToRuntimeSite(t *testing.T) {
	rows := (EvidenceRelationCandidateSource{Items: []EvidenceItem{{
		Kind:            EvidenceMechanism,
		Subject:         "LoadRuntimeConfig",
		Object:          "providers_config",
		Source:          "internal/config/runtime.go",
		LineStart:       87,
		AnchorKind:      AnchorStringLiteral,
		AnchorSymbol:    "providers_config",
		SurfaceTerms:    []string{"`providers_config`", "providers_config"},
		GroundingStatus: GroundingGrounded,
		GroundingTier:   TierLineText,
	}}}).TypedRelationCandidates(TypedRelationQuery{
		Kinds:      []TypedRelationKind{TypedRelationConfigures},
		Sources:    []string{"providers_config"},
		Purpose:    TypedRelationPurposePromptHint,
		MaxMembers: 10,
	})
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want one config relation row", rows)
	}
	row := rows[0]
	if row.Relation != TypedRelationConfigures || row.SourceKind != "config_key" || row.SourceName != "providers_config" {
		t.Fatalf("unexpected config relation header: %+v", row)
	}
	if row.Member.Name != "LoadRuntimeConfig" || row.Member.File != "internal/config/runtime.go" || row.Member.Line != 87 {
		t.Fatalf("unexpected config relation member: %+v", row.Member)
	}
	if row.Carrier != TypedRelationCarrierEvidence || row.Precision != TypedRelationPrecisionExactEvidence {
		t.Fatalf("unexpected config relation carrier: carrier=%s precision=%s", row.Carrier, row.Precision)
	}
}

func TestEvidenceRelationCandidateSource_ConfigRuntimeSiteToKey(t *testing.T) {
	rows := (EvidenceRelationCandidateSource{Items: []EvidenceItem{{
		Kind:            EvidenceRelationship,
		Subject:         "LoadRuntimeConfig",
		Object:          "providers_config",
		Source:          "./internal/config/runtime.go",
		LineStart:       91,
		AnchorKind:      AnchorCall,
		AnchorSymbol:    "Lookup",
		SurfaceTerms:    []string{"providers_config"},
		GroundingStatus: GroundingGrounded,
		GroundingTier:   TierLineText,
	}}}).TypedRelationCandidates(TypedRelationQuery{
		Kinds:   []TypedRelationKind{TypedRelationConfigures},
		Sources: []string{"LoadRuntimeConfig"},
		Purpose: TypedRelationPurposePromptHint,
	})
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want one config reverse relation row", rows)
	}
	if rows[0].SourceKind != "config_site" || rows[0].Member.Name != "providers_config" || rows[0].Member.File != "internal/config/runtime.go" {
		t.Fatalf("unexpected config reverse relation: %+v", rows[0])
	}
}

func TestEvidenceRelationCandidateSource_ConfigPromptOnlyDoesNotLeakToOtherRelationKinds(t *testing.T) {
	item := EvidenceItem{
		Kind:            EvidenceMechanism,
		Subject:         "LoadRuntimeConfig",
		Object:          "providers_config",
		Source:          "internal/config/runtime.go",
		LineStart:       87,
		AnchorKind:      AnchorStringLiteral,
		AnchorSymbol:    "providers_config",
		GroundingStatus: GroundingGrounded,
	}
	rows := (EvidenceRelationCandidateSource{Items: []EvidenceItem{item}}).TypedRelationCandidates(TypedRelationQuery{
		Kinds:   []TypedRelationKind{TypedRelationRegisters},
		Sources: []string{"providers_config"},
		Purpose: TypedRelationPurposePromptHint,
	})
	if len(rows) != 0 {
		t.Fatalf("config evidence must not leak into registration relation queries: %+v", rows)
	}
}

func TestEvidenceRelationCandidateSource_RouteToHandler(t *testing.T) {
	rows := (EvidenceRelationCandidateSource{Items: []EvidenceItem{{
		Kind:            EvidenceRegistration,
		Subject:         "ListUsersHandler",
		Object:          "GET /api/users",
		Source:          "server/routes.ts",
		LineStart:       31,
		AnchorKind:      AnchorStringLiteral,
		AnchorSymbol:    "/api/users",
		SurfaceTerms:    []string{"GET /api/users", "`/api/users`"},
		GroundingStatus: GroundingGrounded,
		GroundingTier:   TierLineText,
	}}}).TypedRelationCandidates(TypedRelationQuery{
		Kinds:      []TypedRelationKind{TypedRelationRoutesTo},
		Sources:    []string{"GET /api/users"},
		Purpose:    TypedRelationPurposePromptHint,
		MaxMembers: 10,
	})
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want one route relation row", rows)
	}
	row := rows[0]
	if row.Relation != TypedRelationRoutesTo || row.SourceKind != "route" || row.SourceName != "GET /api/users" {
		t.Fatalf("unexpected route relation header: %+v", row)
	}
	if row.Member.Name != "ListUsersHandler" || row.Member.File != "server/routes.ts" || row.Member.Line != 31 {
		t.Fatalf("unexpected route relation member: %+v", row.Member)
	}
	if row.Carrier != TypedRelationCarrierEvidence || row.Precision != TypedRelationPrecisionExactEvidence {
		t.Fatalf("unexpected route relation carrier: carrier=%s precision=%s", row.Carrier, row.Precision)
	}
}

func TestEvidenceRelationCandidateSource_RoutePromptOnlyDoesNotLeakToConfigRelation(t *testing.T) {
	item := EvidenceItem{
		Kind:            EvidenceRegistration,
		Subject:         "ListUsersHandler",
		Object:          "GET /api/users",
		Source:          "server/routes.ts",
		LineStart:       31,
		AnchorKind:      AnchorStringLiteral,
		AnchorSymbol:    "/api/users",
		GroundingStatus: GroundingGrounded,
	}
	rows := (EvidenceRelationCandidateSource{Items: []EvidenceItem{item}}).TypedRelationCandidates(TypedRelationQuery{
		Kinds:   []TypedRelationKind{TypedRelationConfigures},
		Sources: []string{"GET /api/users"},
		Purpose: TypedRelationPurposePromptHint,
	})
	if len(rows) != 0 {
		t.Fatalf("route evidence must not leak into config relation queries: %+v", rows)
	}
}
