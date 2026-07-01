package types

import "testing"

func TestSourceInventoryAuthoritySnapshot_MechanicalRowsCanLand(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"."},
		Provenance: []string{
			SourceInventoryProvenanceRepoLensToolQuery,
			SourceInventoryProvenanceStageExplore,
		},
		Lens: []string{"members"},
		SourceClasses: []SourceInventorySourceClassCount{{
			Role: SourcePathRoleThirdParty, Count: 1, Complete: true, Samples: []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj"},
		}},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: true,
			Count:    1,
			Total:    1,
			Members: []SourceInventoryObservationMember{{
				Name:       "Greeter",
				Key:        "internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj::Greeter",
				SupportRef: "Greeter: internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj:6",
				Role:       AnswerCandidateRoleType,
				File:       "internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj",
				Line:       6,
				Language:   "cangjie",
				Attributes: []SourceInventoryObservationAttribute{{
					Name:       "demo.greeter",
					SupportRef: "package demo.greeter: internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj:4",
					Role:       AnswerCandidateRolePackage,
					File:       "internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj",
					Line:       4,
					Language:   "cangjie",
				}},
			}},
		}},
	}
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all public classes"},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType, AnswerCandidateRolePackage},
			RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation, SourceInventoryFieldPackage},
			SourceQuotes:      []string{"public class"},
			Confidence:        0.9,
		},
		SourceScopeProfile: &SourceScopeProfile{RequestedScope: SourceScopeAll},
	}

	snap := BuildSourceInventoryAuthoritySnapshot(SourceInventoryAuthoritySnapshotInput{
		Observation:      obs,
		RequestModel:     rm,
		RequiredFiles:    []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj"},
		MaxPrincipalRows: 4,
	})
	if !snap.Active || !snap.PrincipalAuthority || snap.SupportOnly {
		t.Fatalf("unexpected authority flags: %+v", snap)
	}
	if !snap.CanUseMechanicalRowsForCite || !snap.CanEnterMechanicalLanding {
		t.Fatalf("mechanical complete row-set should be landing-ready: %+v", snap)
	}
	if snap.NeedsFollowup || snap.PrincipalAggregateFactCount == 0 {
		t.Fatalf("complete row-set should not need follow-up and should project aggregate facts: %+v", snap)
	}
	if snap.PrincipalRowSet.PrincipalTotal != 1 || snap.PrincipalRowSet.PrincipalRows[0].Member.Name != "Greeter" {
		t.Fatalf("principal row-set = %+v", snap.PrincipalRowSet)
	}
	view := BuildSourceInventoryAnswerAuthorityView(snap)
	if !view.Active || view.CanBlockCompletion || view.CanOnlyCaveat {
		t.Fatalf("complete authority view should be active and non-blocking: %+v", view)
	}
	if !view.PrincipalAuthority {
		t.Fatalf("authority view should preserve principal-authority status: %+v", view)
	}
	if !view.CanUseMechanicalRowsForCitation || !view.CanEnterMechanicalLanding {
		t.Fatalf("authority view should preserve mechanical landing flags: %+v", view)
	}
	if len(view.CitationObligations) != 1 {
		t.Fatalf("citation obligations = %+v, want one principal row", view.CitationObligations)
	}
	if containsSourceInventorySnapshotReason(view.ReasonCodes, "principal_rowset_missing") {
		t.Fatalf("authority view must not claim rowset missing when rows are present: %+v", view.ReasonCodes)
	}
	if got := view.CitationObligations[0]; got.Member != "Greeter" ||
		got.File != "internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj" ||
		got.Line != 6 ||
		got.SourceClass != SourcePathRoleThirdParty {
		t.Fatalf("citation obligation not derived from typed row: %+v", got)
	}
}

func TestSourceInventoryAuthoritySnapshot_SourceTextFieldBlocksMechanicalLanding(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"."},
		Lens:     []string{"members"},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: true,
			Members: []SourceInventoryObservationMember{{
				Name: "Mode", Role: AnswerCandidateRoleType, File: "pkg/mode.go", Line: 10, Language: "go",
			}},
		}},
	}
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all types"},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
			RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation, SourceInventoryFieldSummary},
			Confidence:        0.9,
		},
		SourceScopeProfile: &SourceScopeProfile{RequestedScope: SourceScopeAll},
	}

	snap := BuildSourceInventoryAuthoritySnapshot(SourceInventoryAuthoritySnapshotInput{Observation: obs, RequestModel: rm})
	if snap.MechanicalRowsOnly || snap.CanUseMechanicalRowsForCite || snap.CanEnterMechanicalLanding {
		t.Fatalf("summary/source-text request must not be satisfied by mechanical rows alone: %+v", snap)
	}
	if snap.PrincipalAggregateFactCount == 0 {
		t.Fatalf("row-set projection should still be visible for audit/status: %+v", snap)
	}
}

func TestSourceInventoryAuthoritySnapshot_FollowupDebtBlocksLanding(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: false,
		Scopes:   []string{"."},
		Lens:     []string{"members"},
		Provenance: []string{
			SourceInventoryProvenanceRepoLensToolQuery,
			SourceInventoryProvenanceStageExplore,
		},
		SourceClasses: []SourceInventorySourceClassCount{{
			Role: SourcePathRoleProduction, Count: 1, Complete: true, Samples: []string{"src/a.cj"},
		}, {
			Role: SourcePathRoleThirdParty, Count: 1, Complete: true, Samples: []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj"},
		}},
		Execution: &SourceInventoryExecutionState{Budgeted: true, CandidateBudgetTruncated: true},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: false,
			Members: []SourceInventoryObservationMember{{
				Name: "LocalOnly", Role: AnswerCandidateRoleType, File: "src/a.cj", Line: 3, Language: "cangjie",
			}},
		}},
	}
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all types"},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
			RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation},
			Confidence:        0.9,
		},
		SourceScopeProfile: &SourceScopeProfile{RequestedScope: SourceScopeAll},
	}

	snap := BuildSourceInventoryAuthoritySnapshot(SourceInventoryAuthoritySnapshotInput{Observation: obs, RequestModel: rm})
	if !snap.NeedsFollowup || !snap.FollowupDebt.IsActive() {
		t.Fatalf("truncated missing source-class universe should remain follow-up debt: %+v", snap)
	}
	if snap.CanUseMechanicalRowsForCite || snap.CanEnterMechanicalLanding {
		t.Fatalf("follow-up debt must block mechanical landing: %+v", snap)
	}
}

func TestSourceInventoryAuthoritySnapshot_AcceptedRequestedUniverseSuppressesStaleFollowup(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: false,
		Scopes:   []string{"."},
		Lens:     []string{"members"},
		Provenance: []string{
			SourceInventoryProvenanceRepoLensToolQuery,
			SourceInventoryProvenanceStageExplore,
		},
		SourceClasses: []SourceInventorySourceClassCount{{
			Role: SourcePathRoleProduction, Count: 1, Complete: true, Samples: []string{"src/a.cj"},
		}, {
			Role: SourcePathRoleThirdParty, Count: 1, Complete: true, Samples: []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj"},
		}},
		Execution: &SourceInventoryExecutionState{Budgeted: true, CandidateBudgetTruncated: true},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: false,
			Members: []SourceInventoryObservationMember{{
				Name: "LocalOnly", Role: AnswerCandidateRoleType, File: "src/a.cj", Line: 3, Language: "cangjie",
			}},
		}},
	}
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all types"},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
			RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation},
			Confidence:        0.9,
		},
		SourceScopeProfile: &SourceScopeProfile{RequestedScope: SourceScopeAll},
	}

	snap := BuildSourceInventoryAuthoritySnapshot(SourceInventoryAuthoritySnapshotInput{
		Observation:               obs,
		RequestModel:              rm,
		AcceptedRequestedUniverse: true,
	})
	if snap.CompletionAuthority.IsBlocking() {
		t.Fatalf("accepted requested universe should close completion authority: %+v", snap)
	}
	if snap.NeedsFollowup || snap.FollowupDebt.IsActive() {
		t.Fatalf("accepted requested universe must suppress lower-level stale follow-up debt: %+v", snap)
	}
}

func TestSourceInventoryAuthoritySnapshot_RequiredFileGapBlocksLanding(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"src"},
		Lens:     []string{"members"},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Members: []SourceInventoryObservationMember{{
				Name: "Run", Role: AnswerCandidateRoleFunction, File: "src/run.go", Line: 7, Language: "go",
			}},
		}},
	}
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all functions"},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
			RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation},
			Confidence:        0.9,
		},
		SourceScopeProfile: &SourceScopeProfile{RequestedScope: SourceScopeProduction},
	}

	snap := BuildSourceInventoryAuthoritySnapshot(SourceInventoryAuthoritySnapshotInput{
		Observation:   obs,
		RequestModel:  rm,
		RequiredFiles: []string{"src/run.go", "src/missing.go"},
	})
	if snap.RequiredFilesCovered || snap.CanEnterMechanicalLanding {
		t.Fatalf("uncovered typed required file must block landing readiness: %+v", snap)
	}
	if !containsSourceInventorySnapshotReason(snap.ReasonCodes, "required_files_uncovered") {
		t.Fatalf("reason codes should preserve required-files gap: %+v", snap.ReasonCodes)
	}
}

func TestSourceInventoryAuthoritySnapshot_RequestedUniverseSuppressesOutOfUniverseRows(t *testing.T) {
	scope := SourceScopeAll
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"."},
		SourceClasses: []SourceInventorySourceClassCount{
			{Role: SourcePathRoleFixture, Count: 2, Complete: true},
			{Role: SourcePathRoleThirdParty, Count: 1, Complete: true},
		},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: true,
			Members: []SourceInventoryObservationMember{
				{Name: "Bridge", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 15, Language: "cangjie", SurfaceTerms: []string{"public class", "public class Bridge"}, CoverageState: SourceInventoryCoverageObserved},
				{Name: "Greeter", Role: AnswerCandidateRoleType, File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj", Line: 6, Language: "cangjie", SurfaceTerms: []string{"public class", "public class Greeter"}, CoverageState: SourceInventoryCoverageObserved},
				{Name: "JavaWidget", Role: AnswerCandidateRoleType, File: "eval/fixtures/java/JavaWidget.java", Line: 12, Language: "java", SurfaceTerms: []string{"public class", "public class JavaWidget"}, CoverageState: SourceInventoryCoverageObserved},
			},
		}},
	}
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all public classes"},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
			RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation},
			SourceQuotes:      []string{"public class"},
			Confidence:        0.9,
		},
		SourceScopeProfile: &SourceScopeProfile{RequestedScope: scope},
	}
	existing := AnswerAggregateFact{
		Kind:       AnswerAggregateMemberSet,
		Label:      "public class declarations",
		Value:      "1",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "explorer",
		Members:    []string{"Bridge"},
		SupportRefs: []string{
			"Bridge @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:15",
		},
	}

	snap := BuildSourceInventoryAuthoritySnapshot(SourceInventoryAuthoritySnapshotInput{
		Observation:            obs,
		RequestModel:           rm,
		ExistingAggregateFacts: []AnswerAggregateFact{existing},
		MaxPrincipalRows:       10,
		MaxAuditRows:           10,
	})
	if snap.PrincipalRowSet.PrincipalTotal != 2 {
		t.Fatalf("requested universe should keep two Cangjie public-class rows, got %+v", snap.PrincipalRowSet)
	}
	for _, row := range snap.PrincipalRowSet.PrincipalRows {
		if row.Language != "cangjie" {
			t.Fatalf("out-of-universe language leaked into principal rows: %+v", snap.PrincipalRowSet.PrincipalRows)
		}
	}
	if got := snap.RequestedUniverse.InventoryOutOfUniverseRowsSuppressed; got != 1 {
		t.Fatalf("suppressed rows = %d, want 1; universe=%+v", got, snap.RequestedUniverse)
	}
	if !sourceInventoryAuthorityTestContainsString(snap.RequestedUniverse.Languages, "cangjie") ||
		sourceInventoryAuthorityTestContainsString(snap.RequestedUniverse.Languages, "java") {
		t.Fatalf("requested universe languages = %+v, want cangjie only", snap.RequestedUniverse.Languages)
	}
	if len(snap.PrincipalRowSet.AuditRows) == 0 || snap.PrincipalRowSet.AuditRows[0].ReasonCode != SourceInventoryRowReasonOutOfRequestedUniverse {
		t.Fatalf("suppressed row should be retained as audit metadata, got %+v", snap.PrincipalRowSet.AuditRows)
	}
	view := BuildSourceInventoryAnswerAuthorityView(snap)
	if view.InventoryOutOfUniverseRowsSuppressed != 1 {
		t.Fatalf("answer authority view lost suppression telemetry: %+v", view)
	}
}

func TestSourceInventoryAuthoritySnapshot_RepoWideAllLanguageUniverseKeepsCrossLanguageRows(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"."},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: true,
			Members: []SourceInventoryObservationMember{
				{Name: "Bridge", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 15, Language: "cangjie", CoverageState: SourceInventoryCoverageObserved},
				{Name: "JavaWidget", Role: AnswerCandidateRoleType, File: "eval/fixtures/java/JavaWidget.java", Line: 12, Language: "java", CoverageState: SourceInventoryCoverageObserved},
			},
		}},
	}
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all types"},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
			RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation},
			Confidence:        0.9,
		},
		SourceScopeProfile: &SourceScopeProfile{RequestedScope: SourceScopeAll},
	}

	snap := BuildSourceInventoryAuthoritySnapshot(SourceInventoryAuthoritySnapshotInput{
		Observation:      obs,
		RequestModel:     rm,
		MaxPrincipalRows: 10,
	})
	if snap.PrincipalRowSet.PrincipalTotal != 2 {
		t.Fatalf("repo-wide all-language inventory should keep both rows, got %+v", snap.PrincipalRowSet)
	}
	if got := snap.RequestedUniverse.InventoryOutOfUniverseRowsSuppressed; got != 0 {
		t.Fatalf("unexpected suppression for explicit all-language inventory: %+v", snap.RequestedUniverse)
	}
	if !sourceInventoryAuthorityTestContainsString(snap.RequestedUniverse.Languages, "cangjie") ||
		!sourceInventoryAuthorityTestContainsString(snap.RequestedUniverse.Languages, "java") {
		t.Fatalf("requested universe should expose both typed languages, got %+v", snap.RequestedUniverse.Languages)
	}
}

func TestSourceInventoryAnswerAuthorityView_NavigationDebtCanOnlyCaveat(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: false,
		Scopes:   []string{"."},
		Provenance: []string{
			SourceInventoryProvenanceRepoLensToolQuery,
			SourceInventoryProvenanceStageExplore,
		},
		Lens: []string{"members"},
		Execution: &SourceInventoryExecutionState{
			CandidateBudgetTruncated: true,
		},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: false,
			Count:    24,
			Total:    120,
			Members: []SourceInventoryObservationMember{{
				Name:     "Greeter",
				Role:     AnswerCandidateRoleType,
				File:     "src/greeter.go",
				Line:     8,
				Language: "go",
			}},
		}},
	}
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all types"},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
			RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation},
			Confidence:        0.9,
		},
		SourceScopeProfile: &SourceScopeProfile{RequestedScope: SourceScopeAll},
	}

	snap := BuildSourceInventoryAuthoritySnapshot(SourceInventoryAuthoritySnapshotInput{
		Observation:   obs,
		RequestModel:  rm,
		RequiredFiles: []string{"src/greeter.go"},
	})
	view := BuildSourceInventoryAnswerAuthorityView(snap)
	if !view.Active {
		t.Fatalf("authority view inactive: %+v", view)
	}
	if !view.CanOnlyCaveat {
		t.Fatalf("navigation debt should be caveat-only, got %+v", view)
	}
	if view.CanBlockCompletion {
		t.Fatalf("non-precise navigation debt must not become answer-hard blocking: %+v", view)
	}
	if !SourceInventoryCompletionAuthorityCanOnlyCaveat(snap.CompletionAuthority) {
		t.Fatalf("completion authority helper should agree with view: %+v", snap.CompletionAuthority)
	}
}

func TestSourceInventoryAnswerAuthorityView_ExecutableMissingClassCanBlock(t *testing.T) {
	authority := SourceInventoryCompletionAuthority{
		Active:     true,
		Blocking:   true,
		ReasonCode: SourceInventoryCompletionReasonFollowupDebt,
		FollowupDebt: SourceInventoryFollowupDebt{
			Active:     true,
			ReasonCode: SourceInventoryFollowupDebtMissingSourceClass,
			Query: SourceInventoryLensQuery{
				Scopes: []string{"internal/thirdparty"},
				Roles:  []AnswerCandidateRole{AnswerCandidateRoleType},
			},
			MissingClasses:   []SourcePathRole{SourcePathRoleThirdParty},
			CoveredClasses:   []SourcePathRole{SourcePathRoleProduction},
			MissingLanguages: []string{"cangjie"},
		},
	}
	snap := NormalizeSourceInventoryAuthoritySnapshot(SourceInventoryAuthoritySnapshot{
		Active:              true,
		CompletionAuthority: authority,
	})
	view := BuildSourceInventoryAnswerAuthorityView(snap)
	if view.CanOnlyCaveat {
		t.Fatalf("executable missing-class debt should stay block-capable: %+v", view)
	}
	if !view.CanBlockCompletion {
		t.Fatalf("executable missing-class debt should be block-capable: %+v", view)
	}
	if !SourceInventoryCompletionAuthorityHasExecutableMissingClass(authority) {
		t.Fatalf("missing-class helper should detect executable typed follow-up: %+v", authority)
	}
}

func containsSourceInventorySnapshotReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func sourceInventoryAuthorityTestContainsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
