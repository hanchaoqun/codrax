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

func TestBuildSourceInventoryPrincipalRowSet_FiltersToRequestedSurfaceFamily(t *testing.T) {
	observation := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"."},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: true,
			Members: []SourceInventoryObservationMember{
				{Name: "Bridge", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 15, Language: "cangjie", SurfaceTerms: []string{"public class", "public class Bridge"}},
				{Name: "Item", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", Line: 6, Language: "cangjie", SurfaceTerms: []string{"public struct", "public struct Item"}},
				{Name: "Drawable", Role: AnswerCandidateRoleType, File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/03_interfaces.cj", Line: 4, Language: "cangjie", SurfaceTerms: []string{"public interface", "public interface Drawable"}},
				{Name: "Service", Role: AnswerCandidateRoleType, File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/08_modifiers_combos.cj", Line: 32, Language: "cangjie", SurfaceTerms: []string{"public class", "public class Service", "public abstract class", "public abstract class Service"}},
			},
		}},
	}
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
		SourceQuotes:      []string{"public class"},
	}}

	view := BuildSourceInventoryPrincipalRowSet(SourceInventoryPrincipalRowSetInput{
		Observation:      observation,
		RequestModel:     rm,
		MaxPrincipalRows: 10,
		MaxSupportRows:   10,
	})
	if view.PrincipalTotal != 2 || view.SupportTotal != 2 {
		t.Fatalf("requested public class family should split principal/support rows: %+v", view)
	}
	if !sourceInventoryPrincipalRowSetTestHasPrincipal(view, "Bridge") ||
		!sourceInventoryPrincipalRowSetTestHasPrincipal(view, "Service") ||
		sourceInventoryPrincipalRowSetTestHasPrincipal(view, "Item") ||
		sourceInventoryPrincipalRowSetTestHasPrincipal(view, "Drawable") {
		t.Fatalf("principal rows should contain only requested public class family: %+v", view.PrincipalRows)
	}
	for _, row := range view.SupportRows {
		if row.Member.Name == "Item" || row.Member.Name == "Drawable" {
			if row.ReasonCode != SourceInventoryRowReasonSurfaceFamily {
				t.Fatalf("non-requested family row should carry surface-family reason, got %+v", row)
			}
		}
	}
}

func TestBuildSourceInventoryPrincipalRowSet_SurfaceFamilyFilterRequiresExactTypedQuote(t *testing.T) {
	observation := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"."},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: true,
			Members: []SourceInventoryObservationMember{
				{Name: "Bridge", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 15, Language: "cangjie", SurfaceTerms: []string{"public class", "public class Bridge"}},
				{Name: "Item", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", Line: 6, Language: "cangjie", SurfaceTerms: []string{"public struct", "public struct Item"}},
			},
		}},
	}
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
		SourceQuotes:      []string{"typed construct surface"},
	}}

	view := BuildSourceInventoryPrincipalRowSet(SourceInventoryPrincipalRowSetInput{
		Observation:      observation,
		RequestModel:     rm,
		MaxPrincipalRows: 10,
		MaxSupportRows:   10,
	})
	if view.PrincipalTotal != 2 || view.SupportTotal != 0 {
		t.Fatalf("unmatched typed quote must not become a hard source-family filter: %+v", view)
	}
}

func TestBuildSourceInventoryPrincipalRowSet_FiltersEachRequestedSurfaceFamilyByRole(t *testing.T) {
	observation := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"."},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: true,
			Members: []SourceInventoryObservationMember{
				{Name: "Bridge", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 15, Language: "cangjie", SurfaceTerms: []string{"public class", "public class Bridge"}},
				{Name: "Item", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", Line: 6, Language: "cangjie", SurfaceTerms: []string{"public struct", "public struct Item"}},
				{Name: "extend String", Role: AnswerCandidateRoleType, File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj", Line: 6, Language: "cangjie", SurfaceTerms: []string{"extend", "extend String"}},
			},
		}, {
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Members: []SourceInventoryObservationMember{
				{Name: "native_add", Role: AnswerCandidateRoleFunction, File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 6, Language: "cangjie", SurfaceTerms: []string{"foreign func", "foreign func native_add"}},
				{Name: "helper", Role: AnswerCandidateRoleFunction, File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/01_basic_functions.cj", Line: 5, Language: "cangjie", SurfaceTerms: []string{"func", "func helper"}},
			},
		}},
	}
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType, AnswerCandidateRoleFunction},
		SourceQuotes:      []string{"extend 块", "foreign func 声明", "public class"},
	}}

	view := BuildSourceInventoryPrincipalRowSet(SourceInventoryPrincipalRowSetInput{
		Observation:      observation,
		RequestModel:     rm,
		MaxPrincipalRows: 10,
		MaxSupportRows:   10,
	})
	if view.PrincipalTotal != 3 || view.SupportTotal != 2 {
		t.Fatalf("requested families should filter independently by role: %+v", view)
	}
	for _, want := range []string{"Bridge", "extend String", "native_add"} {
		if !sourceInventoryPrincipalRowSetTestHasPrincipal(view, want) {
			t.Fatalf("principal rows missing %q: %+v", want, view.PrincipalRows)
		}
	}
	for _, notWant := range []string{"Item", "helper"} {
		if sourceInventoryPrincipalRowSetTestHasPrincipal(view, notWant) {
			t.Fatalf("non-requested family row %q leaked into principal rows: %+v", notWant, view.PrincipalRows)
		}
	}
}

func sourceInventoryPrincipalRowSetTestHasPrincipal(view SourceInventoryPrincipalRowSet, name string) bool {
	for _, row := range view.PrincipalRows {
		if row.Member.Name == name {
			return true
		}
	}
	return false
}
