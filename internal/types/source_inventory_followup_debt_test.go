package types

import "testing"

func TestDeriveSourceInventoryFollowupDebt_MissingSourceClassUsesSampleScope(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     false,
		Scopes:       []string{"."},
		SourceClasses: []SourceInventorySourceClassCount{
			{Role: SourcePathRoleProduction, Count: 2, Complete: true, Samples: []string{"src/app/main.go"}},
			{Role: SourcePathRoleThirdParty, Count: 1, Complete: true, Samples: []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj"}},
		},
		RepoLanguages: []SourceInventoryLanguageCount{{Language: "cangjie", Count: 1, InScope: false, Samples: []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj"}}},
		Page:          &SourceInventoryObservationPage{Offset: 0, Emitted: 24, Total: 48, NextCursor: "24", Complete: false},
		Execution:     &SourceInventoryExecutionState{Budgeted: true, CandidateBudgetTruncated: true},
		Sets: []SourceInventoryObservationSet{{
			Role: AnswerCandidateRoleType,
			Members: []SourceInventoryObservationMember{{
				Name: "Run",
				Role: AnswerCandidateRoleType,
				File: "src/app/main.go",
			}},
		}},
	}
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
	}}

	debt := DeriveSourceInventoryFollowupDebt(obs, rm)
	if !debt.IsActive() || debt.ReasonCode != SourceInventoryFollowupDebtMissingSourceClass {
		t.Fatalf("debt = %+v", debt)
	}
	if len(debt.MissingClasses) != 1 || debt.MissingClasses[0] != SourcePathRoleThirdParty {
		t.Fatalf("missing classes = %+v", debt.MissingClasses)
	}
	if got, want := debt.Query.Scopes, []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("scopes = %+v, want %+v", got, want)
	}
	if len(debt.Query.Roles) != 1 || debt.Query.Roles[0] != AnswerCandidateRoleType {
		t.Fatalf("roles = %+v", debt.Query.Roles)
	}
	if debt.Query.Cursor != "" || debt.Query.Offset != 0 {
		t.Fatalf("missing source-class follow-up must not inherit the prior broad lens page cursor: %+v", debt.Query)
	}
	if len(debt.MissingLanguages) != 1 || debt.MissingLanguages[0] != "cangjie" {
		t.Fatalf("missing languages = %+v", debt.MissingLanguages)
	}
}

func TestDeriveSourceInventoryFollowupDebt_MissingSourceClassBalancesLanguageFamilies(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     false,
		Scopes:       []string{"."},
		SourceClasses: []SourceInventorySourceClassCount{
			{Role: SourcePathRoleProduction, Count: 1, Complete: true, Samples: []string{"internal/tool/main.go"}},
			{
				Role:     SourcePathRoleThirdParty,
				Count:    14,
				Complete: true,
				Samples: []string{
					"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
					"internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets",
				},
				Languages: []SourceInventoryLanguageCount{
					{
						Language: "cangjie",
						Count:    8,
						InScope:  true,
						Samples: []string{
							"internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj",
							"internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj",
						},
					},
					{
						Language: "arkts",
						Count:    6,
						InScope:  true,
						Samples: []string{
							"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
						},
					},
				},
			},
		},
		Page:      &SourceInventoryObservationPage{Offset: 0, Emitted: 24, Total: 80, NextCursor: "24", Complete: false},
		Execution: &SourceInventoryExecutionState{Budgeted: true, CandidateBudgetTruncated: true},
		Sets: []SourceInventoryObservationSet{{
			Role: AnswerCandidateRoleType,
			Members: []SourceInventoryObservationMember{{
				Name: "Main",
				Role: AnswerCandidateRoleType,
				File: "internal/tool/main.go",
			}},
		}},
	}
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType, AnswerCandidateRoleFunction},
	}}

	debt := DeriveSourceInventoryFollowupDebt(obs, rm)
	if !debt.IsActive() || debt.ReasonCode != SourceInventoryFollowupDebtMissingSourceClass {
		t.Fatalf("debt = %+v", debt)
	}
	wantScopes := []string{
		"internal/thirdparty/tree-sitter-cangjie/corpus/sources",
		"internal/thirdparty/tree-sitter-arkts/corpus/sources",
	}
	if len(debt.Query.Scopes) != len(wantScopes) {
		t.Fatalf("scopes = %+v, want %+v", debt.Query.Scopes, wantScopes)
	}
	for i, want := range wantScopes {
		if debt.Query.Scopes[i] != want {
			t.Fatalf("scopes = %+v, want %+v", debt.Query.Scopes, wantScopes)
		}
	}
}

func TestNormalizeSourceInventoryFollowupDebt_ClearsCursorForScopeChangingDebt(t *testing.T) {
	debt := NormalizeSourceInventoryFollowupDebt(SourceInventoryFollowupDebt{
		Active:     true,
		ReasonCode: SourceInventoryFollowupDebtMissingSourceClass,
		Query: SourceInventoryLensQuery{
			Path:   ".",
			Scopes: []string{"thirdparty/corpus"},
			Roles:  []AnswerCandidateRole{AnswerCandidateRoleType},
			Cursor: "50",
			Offset: 50,
		},
		MissingClasses: []SourcePathRole{SourcePathRoleThirdParty},
		Roles:          []AnswerCandidateRole{AnswerCandidateRoleType},
	})
	if !debt.IsActive() {
		t.Fatalf("debt should remain active: %+v", debt)
	}
	if debt.Query.Cursor != "" || debt.Query.Offset != 0 {
		t.Fatalf("scope-changing debt must restart from the first page, got %+v", debt.Query)
	}
}

func TestDeriveSourceInventoryFollowupDebt_PaginationUsesCursor(t *testing.T) {
	obs := SourceInventoryObservation{
		Active: true,
		Scopes: []string{"."},
		SourceClasses: []SourceInventorySourceClassCount{{
			Role: SourcePathRoleProduction, Count: 2, Complete: true, Samples: []string{"src/a.go"},
		}},
		Page: &SourceInventoryObservationPage{Offset: 0, Emitted: 24, Total: 48, NextCursor: "24", Complete: false},
		Sets: []SourceInventoryObservationSet{{
			Role: AnswerCandidateRoleFunction,
			Members: []SourceInventoryObservationMember{{
				Name: "A",
				Role: AnswerCandidateRoleFunction,
				File: "src/a.go",
			}},
		}},
	}

	debt := DeriveSourceInventoryFollowupDebt(obs, RequestModel{})
	if !debt.IsActive() || debt.ReasonCode != SourceInventoryFollowupDebtPagination {
		t.Fatalf("debt = %+v", debt)
	}
	if debt.Query.Cursor != "24" || debt.Query.Offset != 24 {
		t.Fatalf("query page = %+v", debt.Query)
	}
}

func TestDeriveSourceInventoryFollowupDebt_NoBudgetDebtNoops(t *testing.T) {
	obs := SourceInventoryObservation{
		Active: true,
		SourceClasses: []SourceInventorySourceClassCount{{
			Role: SourcePathRoleThirdParty, Count: 1, Complete: true, Samples: []string{"thirdparty/a.cj"},
		}},
		Sets: []SourceInventoryObservationSet{{
			Role: AnswerCandidateRoleType,
			Members: []SourceInventoryObservationMember{{
				Name: "A",
				Role: AnswerCandidateRoleType,
				File: "thirdparty/a.cj",
			}},
		}},
	}
	if debt := DeriveSourceInventoryFollowupDebt(obs, RequestModel{}); debt.IsActive() {
		t.Fatalf("complete observation without budget/page debt must not schedule follow-up: %+v", debt)
	}
}
