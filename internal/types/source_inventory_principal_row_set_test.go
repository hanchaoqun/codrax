package types

import "testing"

func TestBuildSourceInventoryPrincipalRowSet_RepoWideKeepsAuxiliaryPrincipal(t *testing.T) {
	observation := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"."},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: true,
			Members: []SourceInventoryObservationMember{
				{Name: "ProductionType", Role: AnswerCandidateRoleType, File: "src/main.cj", Line: 1, Language: "cangjie"},
				{Name: "CorpusType", Role: AnswerCandidateRoleType, File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/01.cj", Line: 2, Language: "cangjie"},
			},
		}},
	}
	rm := RequestModel{
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
		},
	}

	view := BuildSourceInventoryPrincipalRowSet(SourceInventoryPrincipalRowSetInput{
		Observation:      observation,
		RequestModel:     rm,
		MaxPrincipalRows: 10,
		MaxSupportRows:   10,
	})
	if !view.Active || !view.RepoWidePrincipal || view.PrincipalScope != SourceScopeAll {
		t.Fatalf("view should be active repo-wide principal: %+v", view)
	}
	if view.PrincipalTotal != 2 || len(view.PrincipalRows) != 2 || view.SupportTotal != 0 {
		t.Fatalf("repo-wide source inventory should keep production and thirdparty rows principal: %+v", view)
	}
	if view.PrincipalRows[1].SourceClass != SourcePathRoleThirdParty ||
		view.PrincipalRows[1].Lane != SourceInventoryRowLanePrincipal {
		t.Fatalf("thirdparty row should stay principal under repo-wide source inventory: %+v", view.PrincipalRows)
	}
}

func TestBuildSourceInventoryPrincipalRowSet_ProductionWithoutExclusionKeepsRepoWidePrincipal(t *testing.T) {
	observation := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"."},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Members: []SourceInventoryObservationMember{
				{Name: "Run", Role: AnswerCandidateRoleFunction, File: "src/run.go", Line: 10, Language: "go"},
				{Name: "FixtureRun", Role: AnswerCandidateRoleFunction, File: "testdata/run_fixture.go", Line: 11, Language: "go"},
			},
		}},
	}
	rm := RequestModel{
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
		},
		SourceScopeProfile: &SourceScopeProfile{RequestedScope: SourceScopeProduction},
	}

	view := BuildSourceInventoryPrincipalRowSet(SourceInventoryPrincipalRowSetInput{
		Observation:      observation,
		RequestModel:     rm,
		MaxPrincipalRows: 10,
		MaxSupportRows:   10,
	})
	if view.PrincipalScope != SourceScopeAll || !view.RepoWidePrincipal || view.PrincipalTotal != 2 || view.SupportTotal != 0 {
		t.Fatalf("production scope without typed auxiliary exclusion should stay repo-wide principal: %+v", view)
	}
}

func TestBuildSourceInventoryPrincipalRowSet_ExplicitProductionExclusionDemotesAuxiliary(t *testing.T) {
	observation := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"."},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Members: []SourceInventoryObservationMember{
				{Name: "Run", Role: AnswerCandidateRoleFunction, File: "src/run.go", Line: 10, Language: "go"},
				{Name: "FixtureRun", Role: AnswerCandidateRoleFunction, File: "testdata/run_fixture.go", Line: 11, Language: "go"},
			},
		}},
	}
	rm := RequestModel{
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
		},
		SourceScopeProfile: &SourceScopeProfile{RequestedScope: SourceScopeProduction},
		AnswerExclusionPolicy: &AnswerExclusionPolicy{
			IsExclusionRequested:   true,
			ExcludedCandidateRoles: []AnswerCandidateRole{AnswerCandidateRoleFixture},
		},
	}

	view := BuildSourceInventoryPrincipalRowSet(SourceInventoryPrincipalRowSetInput{
		Observation:      observation,
		RequestModel:     rm,
		MaxPrincipalRows: 10,
		MaxSupportRows:   10,
	})
	if view.PrincipalScope != SourceScopeProduction || view.PrincipalTotal != 1 || view.SupportTotal != 1 {
		t.Fatalf("production scope should split principal/support rows: %+v", view)
	}
	if got := view.PrincipalRows[0].Member.Name; got != "Run" {
		t.Fatalf("principal row = %q, want Run", got)
	}
	if got := view.SupportRows[0].ReasonCode; got != SourceInventoryRowReasonSupportScope {
		t.Fatalf("support reason = %q, want %q", got, SourceInventoryRowReasonSupportScope)
	}
}

func TestBuildSourceInventoryPrincipalRowSet_FamilyBalancedBeforeLimit(t *testing.T) {
	var members []SourceInventoryObservationMember
	for _, name := range []string{"ProdA", "ProdB", "ProdC", "ProdD"} {
		members = append(members, SourceInventoryObservationMember{
			Name:     name,
			Role:     AnswerCandidateRoleType,
			File:     "src/" + name + ".cj",
			Line:     1,
			Language: "cangjie",
		})
	}
	members = append(members, SourceInventoryObservationMember{
		Name:     "ThirdPartyA",
		Role:     AnswerCandidateRoleType,
		File:     "internal/thirdparty/tree-sitter-cangjie/corpus/sources/third.cj",
		Line:     1,
		Language: "cangjie",
	})
	observation := SourceInventoryObservation{
		Active: true,
		Scopes: []string{"."},
		Sets: []SourceInventoryObservationSet{{
			Role:    AnswerCandidateRoleType,
			Members: members,
		}},
	}
	rm := RequestModel{
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
		},
	}

	view := BuildSourceInventoryPrincipalRowSet(SourceInventoryPrincipalRowSetInput{
		Observation:      observation,
		RequestModel:     rm,
		MaxPrincipalRows: 3,
	})
	if view.PrincipalTotal != 5 || len(view.PrincipalRows) != 3 || view.PrincipalHiddenCount != 2 {
		t.Fatalf("unexpected row counts: %+v", view)
	}
	if view.PrincipalRows[1].SourceClass != SourcePathRoleThirdParty ||
		view.PrincipalRows[1].Member.Name != "ThirdPartyA" {
		t.Fatalf("rare family should appear before repeated production rows: %+v", view.PrincipalRows)
	}
}
