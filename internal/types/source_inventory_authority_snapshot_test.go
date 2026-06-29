package types

import "testing"

func TestSourceInventoryAuthoritySnapshot_MechanicalRowsCanLand(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"."},
		Lens:     []string{"members"},
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

func containsSourceInventorySnapshotReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
