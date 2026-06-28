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

func TestDeriveSourceInventoryFollowupDebt_RoleBindingSupportOnlyNoops(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     false,
		Scopes:       []string{"."},
		SourceClasses: []SourceInventorySourceClassCount{
			{Role: SourcePathRoleProduction, Count: 2, Complete: true, Samples: []string{"internal/agent/subagent.go"}},
			{Role: SourcePathRoleThirdParty, Count: 1, Complete: true, Samples: []string{"internal/thirdparty/example/source.go"}},
		},
		RepoLanguages: []SourceInventoryLanguageCount{{Language: "go", Count: 1, InScope: false, Samples: []string{"internal/thirdparty/example/source.go"}}},
		Page:          &SourceInventoryObservationPage{Offset: 0, Emitted: 24, Total: 48, NextCursor: "24", Complete: false},
		Execution:     &SourceInventoryExecutionState{Budgeted: true, CandidateBudgetTruncated: true},
		Sets: []SourceInventoryObservationSet{{
			Role: AnswerCandidateRoleType,
			Members: []SourceInventoryObservationMember{{
				Name: "SubAgentRegistry",
				Role: AnswerCandidateRoleType,
				File: "internal/agent/subagent.go",
			}},
		}},
	}
	rm := RequestModel{
		Intent:        IntentEnumerate,
		PredicateAxis: AxisRegister,
		Predicates:    SemanticPredicates{IsCategoryEnumeration: true},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction, AnswerCandidateRoleType},
			RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation},
		},
	}

	if debt := DeriveSourceInventoryFollowupDebt(obs, rm); debt.IsActive() {
		t.Fatalf("role-binding source_inventory support lane must not create follow-up debt: %+v", debt)
	}
}

func TestDeriveSourceInventoryFollowupDebt_FileRowsDoNotCoverPrincipalMemberRoles(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     false,
		Scopes:       []string{"."},
		SourceClasses: []SourceInventorySourceClassCount{
			{
				Role:  SourcePathRoleFixture,
				Count: 2,
				Languages: []SourceInventoryLanguageCount{{
					Language: "cangjie",
					Count:    2,
					InScope:  true,
					Samples:  []string{"eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj"},
				}},
			},
			{
				Role:  SourcePathRoleThirdParty,
				Count: 8,
				Languages: []SourceInventoryLanguageCount{{
					Language: "cangjie",
					Count:    8,
					InScope:  true,
					Samples:  []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj"},
				}},
			},
		},
		Page:      &SourceInventoryObservationPage{Offset: 0, Emitted: 24, Total: 80, NextCursor: "24", Complete: false},
		Execution: &SourceInventoryExecutionState{Budgeted: true, CandidateBudgetTruncated: true},
		Sets: []SourceInventoryObservationSet{{
			Role: AnswerCandidateRoleType,
			Members: []SourceInventoryObservationMember{{
				Name:     "Cart",
				Role:     AnswerCandidateRoleType,
				File:     "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj",
				Language: "cangjie",
			}},
		}, {
			Role: AnswerCandidateRoleFile,
			Members: []SourceInventoryObservationMember{{
				Name:     "04_extend_operator.cj",
				Role:     AnswerCandidateRoleFile,
				File:     "internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj",
				Language: "cangjie",
			}},
		}},
	}
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType, AnswerCandidateRoleFunction},
	}}

	debt := DeriveSourceInventoryFollowupDebt(obs, rm)
	if !debt.IsActive() || debt.ReasonCode != SourceInventoryFollowupDebtMissingSourceClass {
		t.Fatalf("file rows must not satisfy type/function source-class coverage, got %+v", debt)
	}
	if !sourceInventoryPathRolesContain(debt.MissingClasses, SourcePathRoleThirdParty) {
		t.Fatalf("missing classes = %+v, want thirdparty", debt.MissingClasses)
	}
	if len(debt.Query.Scopes) != 1 || debt.Query.Scopes[0] != "internal/thirdparty/tree-sitter-cangjie/corpus/sources" {
		t.Fatalf("query scopes = %+v", debt.Query.Scopes)
	}
}

func TestDeriveSourceInventoryFollowupDebt_CompleteZeroLensCoversExactMissingSourceClass(t *testing.T) {
	obs := sourceInventoryFollowupDebtMissingThirdPartyFixture()
	obs.CompleteLenses = []SourceInventoryCompleteLens{{
		Role:          AnswerCandidateRoleType,
		Scopes:        []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources"},
		SourceClasses: []SourcePathRole{SourcePathRoleThirdParty},
		Count:         0,
		Total:         0,
	}}
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
	}}

	debt := DeriveSourceInventoryFollowupDebt(obs, rm)
	if !debt.IsActive() || debt.ReasonCode != SourceInventoryFollowupDebtPagination {
		t.Fatalf("complete zero lens should demote missing-class debt to pagination/advisory debt, got %+v", debt)
	}
	if len(debt.MissingClasses) != 0 {
		t.Fatalf("missing classes = %+v, want none", debt.MissingClasses)
	}
	if !sourceInventoryPathRolesContain(debt.CoveredClasses, SourcePathRoleThirdParty) {
		t.Fatalf("covered classes = %+v, want thirdparty carried from exact zero lens", debt.CoveredClasses)
	}
}

func TestDeriveSourceInventoryFollowupDebt_CompleteZeroLensRequiresExactRoleAndScope(t *testing.T) {
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
	}}
	for name, lens := range map[string]SourceInventoryCompleteLens{
		"wrong role": {
			Role:          AnswerCandidateRoleFunction,
			Scopes:        []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources"},
			SourceClasses: []SourcePathRole{SourcePathRoleThirdParty},
		},
		"wrong scope": {
			Role:          AnswerCandidateRoleType,
			Scopes:        []string{"internal/thirdparty/tree-sitter-arkts/corpus/sources"},
			SourceClasses: []SourcePathRole{SourcePathRoleThirdParty},
		},
		"wrong source class": {
			Role:          AnswerCandidateRoleType,
			Scopes:        []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources"},
			SourceClasses: []SourcePathRole{SourcePathRoleFixture},
		},
	} {
		t.Run(name, func(t *testing.T) {
			obs := sourceInventoryFollowupDebtMissingThirdPartyFixture()
			lens.Count = 0
			lens.Total = 0
			obs.CompleteLenses = []SourceInventoryCompleteLens{lens}
			debt := DeriveSourceInventoryFollowupDebt(obs, rm)
			if !debt.IsActive() || debt.ReasonCode != SourceInventoryFollowupDebtMissingSourceClass {
				t.Fatalf("mismatched zero lens must not cover missing class, got %+v", debt)
			}
			if len(debt.MissingClasses) != 1 || debt.MissingClasses[0] != SourcePathRoleThirdParty {
				t.Fatalf("missing classes = %+v", debt.MissingClasses)
			}
		})
	}
}

func TestDeriveSourceInventoryFollowupDebt_CompleteZeroLensRequiresEveryRequestedRole(t *testing.T) {
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType, AnswerCandidateRoleFunction},
	}}
	obs := sourceInventoryFollowupDebtMissingThirdPartyFixture()
	obs.CompleteLenses = []SourceInventoryCompleteLens{{
		Role:          AnswerCandidateRoleType,
		Scopes:        []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources"},
		SourceClasses: []SourcePathRole{SourcePathRoleThirdParty},
		Count:         0,
		Total:         0,
	}}
	debt := DeriveSourceInventoryFollowupDebt(obs, rm)
	if !debt.IsActive() || debt.ReasonCode != SourceInventoryFollowupDebtMissingSourceClass {
		t.Fatalf("single-role zero lens must not cover multi-role requested family, got %+v", debt)
	}

	obs.CompleteLenses = append(obs.CompleteLenses, SourceInventoryCompleteLens{
		Role:          AnswerCandidateRoleFunction,
		Scopes:        []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources"},
		SourceClasses: []SourcePathRole{SourcePathRoleThirdParty},
		Count:         0,
		Total:         0,
	})
	debt = DeriveSourceInventoryFollowupDebt(obs, rm)
	if !debt.IsActive() || debt.ReasonCode != SourceInventoryFollowupDebtPagination {
		t.Fatalf("all requested roles covered by exact zero lenses should demote missing-class debt, got %+v", debt)
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

func TestDeriveSourceInventoryFollowupDebt_TargetLanguageFiltersMixedClassScopes(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     false,
		Scopes:       []string{"."},
		SourceClasses: []SourceInventorySourceClassCount{
			{Role: SourcePathRoleProduction, Count: 1, Complete: true, Samples: []string{"internal/tool/main.go"}},
			{
				Role:  SourcePathRoleThirdParty,
				Count: 14,
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
				Name:     "Cart",
				Role:     AnswerCandidateRoleType,
				File:     "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj",
				Language: "cangjie",
			}},
		}},
	}
	rm := RequestModel{
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType, AnswerCandidateRoleFunction},
		},
	}

	debt := DeriveSourceInventoryFollowupDebtWithRequiredFiles(obs, rm, []string{
		"internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj",
	})
	if !debt.IsActive() || debt.ReasonCode != SourceInventoryFollowupDebtMissingSourceClass {
		t.Fatalf("debt = %+v", debt)
	}
	if len(debt.Query.Scopes) != 1 || debt.Query.Scopes[0] != "internal/thirdparty/tree-sitter-cangjie/corpus/sources" {
		t.Fatalf("query scopes = %+v, want only cangjie thirdparty scope", debt.Query.Scopes)
	}
	for _, scope := range debt.Query.Scopes {
		if scope == "internal/thirdparty/tree-sitter-arkts/corpus/sources" {
			t.Fatalf("target-language follow-up leaked arkts scope: %+v", debt.Query.Scopes)
		}
	}
}

func TestDeriveSourceInventoryFollowupDebt_BroadRequiredFilesDoNotOverrideClassUniverse(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     false,
		Scopes:       []string{"."},
		SourceClasses: []SourceInventorySourceClassCount{
			{
				Role:  SourcePathRoleProduction,
				Count: 3,
				Languages: []SourceInventoryLanguageCount{{
					Language: "go",
					Count:    3,
					InScope:  true,
					Samples:  []string{"cmd/root.go"},
				}},
			},
			{
				Role:  SourcePathRoleFixture,
				Count: 3,
				Languages: []SourceInventoryLanguageCount{{
					Language: "cpp",
					Count:    3,
					InScope:  true,
					Samples:  []string{"eval/fixtures/github_issues/fmt_tm_year_overflow/tests/test_tmfmt.cpp"},
				}},
			},
			{
				Role:  SourcePathRoleThirdParty,
				Count: 14,
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
				Name: "Root",
				Role: AnswerCandidateRoleType,
				File: "cmd/root.go",
			}},
		}},
	}
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
	}}

	debt := DeriveSourceInventoryFollowupDebtWithRequiredFiles(obs, rm, []string{
		"cmd/root.go",
		"eval/fixtures/github_issues/chrono_duration_min/tests/check_duration_min.py",
		"eval/fixtures/github_issues/fmt_tm_year_overflow/tests/test_tmfmt.cpp",
		"eval/fixtures/github_issues/dayjs_duration_nan/tests/duration_nan.js",
	})
	if debt.IsActive() {
		t.Fatalf("broad mixed-language required files must not create a forced follow-up route: %+v", debt)
	}
}

func TestDeriveSourceInventoryFollowupDebt_ObservedConstructLanguageOverridesSupportRequiredFiles(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     false,
		Scopes:       []string{"."},
		SourceClasses: []SourceInventorySourceClassCount{
			{
				Role:  SourcePathRoleProduction,
				Count: 3,
				Languages: []SourceInventoryLanguageCount{{
					Language: "go",
					Count:    3,
					InScope:  true,
					Samples:  []string{"internal/skill/defaults.go"},
				}},
			},
			{
				Role:  SourcePathRoleFixture,
				Count: 3,
				Languages: []SourceInventoryLanguageCount{{
					Language: "cangjie",
					Count:    3,
					InScope:  true,
					Samples:  []string{"eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj"},
				}},
			},
			{
				Role:  SourcePathRoleThirdParty,
				Count: 8,
				Languages: []SourceInventoryLanguageCount{{
					Language: "cangjie",
					Count:    8,
					InScope:  false,
					Samples:  []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj"},
				}},
			},
		},
		RepoLanguages: []SourceInventoryLanguageCount{{Language: "cangjie", Count: 8, InScope: false}},
		Page:          &SourceInventoryObservationPage{Offset: 0, Emitted: 24, Total: 80, NextCursor: "24", Complete: false},
		Execution:     &SourceInventoryExecutionState{Budgeted: true, CandidateBudgetTruncated: true},
		Sets: []SourceInventoryObservationSet{{
			Role: AnswerCandidateRoleType,
			Members: []SourceInventoryObservationMember{{
				Name:         "ParserHelper",
				Role:         AnswerCandidateRoleType,
				File:         "internal/skill/defaults.go",
				Language:     "go",
				SurfaceTerms: nil,
			}, {
				Name:         "Cart",
				Role:         AnswerCandidateRoleType,
				File:         "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj",
				Language:     "cangjie",
				SurfaceTerms: []string{"public class", "public class Cart"},
			}},
		}},
	}
	rm := RequestModel{
		AnalyzerHints: AnalyzerHints{RequiredFileHints: []RequiredFileHint{{
			Path:       "internal/skill/defaults.go",
			Confidence: 0.95,
		}}},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
			SourceQuotes:      []string{"public class"},
		},
	}

	debt := DeriveSourceInventoryFollowupDebtWithRequiredFiles(obs, rm, []string{"internal/skill/defaults.go"})
	if !debt.IsActive() || debt.ReasonCode != SourceInventoryFollowupDebtMissingSourceClass {
		t.Fatalf("debt = %+v", debt)
	}
	if len(debt.Query.Scopes) != 1 || debt.Query.Scopes[0] != "internal/thirdparty/tree-sitter-cangjie/corpus/sources" {
		t.Fatalf("observed construct language should route to cangjie thirdparty, got %+v", debt.Query.Scopes)
	}
	if len(debt.MissingLanguages) != 1 || debt.MissingLanguages[0] != "cangjie" {
		t.Fatalf("missing languages = %+v, want cangjie", debt.MissingLanguages)
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

func sourceInventoryFollowupDebtMissingThirdPartyFixture() SourceInventoryObservation {
	return SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     false,
		Scopes:       []string{"."},
		SourceClasses: []SourceInventorySourceClassCount{
			{Role: SourcePathRoleProduction, Count: 2, Complete: true, Samples: []string{"src/app/main.go"}},
			{
				Role:     SourcePathRoleThirdParty,
				Count:    1,
				Complete: true,
				Samples:  []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj"},
				Languages: []SourceInventoryLanguageCount{{
					Language: "cangjie",
					Count:    1,
					InScope:  true,
					Samples:  []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj"},
				}},
			},
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
}
