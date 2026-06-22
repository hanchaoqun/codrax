package types

import (
	"strings"
	"testing"
)

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_SynthesizesCompleteFact(t *testing.T) {
	rm := sourceInventoryProjectionRequestModel(nil)
	obs := sourceInventoryProjectionObservation(
		SourceInventoryObservationMember{Name: "Run", Role: AnswerCandidateRoleFunction, File: "thirdparty/cangjie/run.cj", Line: 7, Language: "cangjie"},
		SourceInventoryObservationMember{Name: "Serve", Role: AnswerCandidateRoleFunction, File: "src/serve.cj", Line: 12, Language: "cangjie", Note: "entry function"},
	)

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts(nil, obs, rm)
	if len(got) != 1 {
		t.Fatalf("projected facts = %+v, want one source-inventory aggregate", got)
	}
	fact := got[0]
	if fact.Provenance != SourceInventoryPrincipalRowSetAggregateProvenance ||
		fact.Role != AnswerAggregateRolePrincipalAnswer ||
		fact.Kind != AnswerAggregateMemberSet ||
		fact.Value != "2" {
		t.Fatalf("projected fact shape drifted: %+v", fact)
	}
	if !sourceInventoryProjectionStringsEqual(fact.Members, []string{"Run", "Serve"}) {
		t.Fatalf("members = %+v, want Run and Serve", fact.Members)
	}
	if !sourceInventoryProjectionStringsEqual(fact.SupportRefs, []string{"Run @ thirdparty/cangjie/run.cj:7", "Serve @ src/serve.cj:12"}) {
		t.Fatalf("support_refs = %+v", fact.SupportRefs)
	}
	sets := CompileEnumerationDisplaySets(&rm, &AnswerSurfacePlan{
		StableAggregateFacts:       got,
		SourceInventoryObservation: obs,
	})
	if len(sets) != 1 || len(sets[0].Rows) != 2 {
		t.Fatalf("enumeration rows = %+v, want two rows from projected aggregate", sets)
	}
	citations := []string{sets[0].Rows[0].CitationKey, sets[0].Rows[1].CitationKey}
	if !sets[0].Rows[0].HasCitation || !sets[0].Rows[1].HasCitation ||
		!sourceInventoryProjectionStringsEqual(citations, []string{"thirdparty/cangjie/run.cj:7", "src/serve.cj:12"}) {
		t.Fatalf("row citations = %+v", sets[0].Rows)
	}
}

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_PreservesEqualCompleteModelFact(t *testing.T) {
	rm := sourceInventoryProjectionRequestModel(nil)
	obs := sourceInventoryProjectionObservation(
		SourceInventoryObservationMember{Name: "Run", Role: AnswerCandidateRoleFunction, File: "thirdparty/cangjie/run.cj", Line: 7, Language: "cangjie"},
		SourceInventoryObservationMember{Name: "Serve", Role: AnswerCandidateRoleFunction, File: "src/serve.cj", Line: 12, Language: "cangjie"},
	)
	existing := AnswerAggregateFact{
		Kind:        AnswerAggregateMemberSet,
		Label:       "model inventory",
		Value:       "2",
		Role:        AnswerAggregateRolePrincipalAnswer,
		Provenance:  "explorer",
		Members:     []string{"Run", "Serve"},
		SupportRefs: []string{"Run @ thirdparty/cangjie/run.cj:7", "Serve @ src/serve.cj:12"},
	}

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts([]AnswerAggregateFact{existing}, obs, rm)
	if len(got) != 1 {
		t.Fatalf("equal complete model fact should be preserved without duplicate, got %+v", got)
	}
	if got[0].Provenance != "explorer" || got[0].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("model fact was unexpectedly rewritten: %+v", got[0])
	}
}

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_DemotesIncompleteModelFact(t *testing.T) {
	rm := sourceInventoryProjectionRequestModel(nil)
	obs := sourceInventoryProjectionObservation(
		SourceInventoryObservationMember{Name: "Run", Role: AnswerCandidateRoleFunction, File: "thirdparty/cangjie/run.cj", Line: 7, Language: "cangjie"},
		SourceInventoryObservationMember{Name: "Serve", Role: AnswerCandidateRoleFunction, File: "src/serve.cj", Line: 12, Language: "cangjie"},
	)
	partial := AnswerAggregateFact{
		Kind:        AnswerAggregateMemberSet,
		Label:       "partial fixture inventory",
		Value:       "1",
		Role:        AnswerAggregateRolePrincipalAnswer,
		Provenance:  "explorer",
		Members:     []string{"Run"},
		SupportRefs: []string{"Run @ thirdparty/cangjie/run.cj:7"},
	}

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts([]AnswerAggregateFact{partial}, obs, rm)
	if len(got) != 2 {
		t.Fatalf("projected facts = %+v, want demoted partial plus synthetic complete fact", got)
	}
	if got[0].Role != AnswerAggregateRoleSupportingCoverage ||
		!strings.Contains(got[0].Provenance, "demoted:shadowed_by_source_inventory_principal_row_set") {
		t.Fatalf("partial model fact was not demoted: %+v", got[0])
	}
	refs := PrincipalAggregateMemberSetFactRefsForRequest(got, &rm)
	if len(refs) != 1 || refs[0].Fact.Provenance != SourceInventoryPrincipalRowSetAggregateProvenance {
		t.Fatalf("principal refs = %+v, want only synthetic row-set fact", refs)
	}
}

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_ProductionScopeExcludesAuxiliaryRows(t *testing.T) {
	scope := SourceScopeProduction
	rm := sourceInventoryProjectionRequestModel(&scope)
	obs := sourceInventoryProjectionObservation(
		SourceInventoryObservationMember{Name: "FixtureRun", Role: AnswerCandidateRoleFunction, File: "eval/fixtures/run.cj", Line: 3, Language: "cangjie"},
		SourceInventoryObservationMember{Name: "Serve", Role: AnswerCandidateRoleFunction, File: "src/serve.cj", Line: 12, Language: "cangjie"},
	)

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts(nil, obs, rm)
	if len(got) != 1 {
		t.Fatalf("projected facts = %+v", got)
	}
	if gotMembers := strings.Join(got[0].Members, ","); gotMembers != "Serve" {
		t.Fatalf("production-scoped members = %q, want Serve", gotMembers)
	}
}

func sourceInventoryProjectionRequestModel(scope *SourceScope) RequestModel {
	rm := RequestModel{
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
		},
	}
	if scope != nil {
		rm.SourceScopeProfile = &SourceScopeProfile{RequestedScope: *scope}
	}
	return rm
}

func sourceInventoryProjectionObservation(members ...SourceInventoryObservationMember) SourceInventoryObservation {
	for i := range members {
		if members[i].CoverageState == "" {
			members[i].CoverageState = SourceInventoryCoverageObserved
		}
	}
	return SourceInventoryObservation{
		Complete: true,
		Scopes:   []string{"."},
		SourceClasses: []SourceInventorySourceClassCount{
			{Role: SourcePathRoleProduction, Count: 1, Complete: true},
			{Role: SourcePathRoleThirdParty, Count: 1, Complete: true},
			{Role: SourcePathRoleFixture, Count: 1, Complete: true},
		},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Members:  members,
		}},
	}
}

func sourceInventoryProjectionStringsEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]int{}
	for _, item := range got {
		seen[item]++
	}
	for _, item := range want {
		if seen[item] == 0 {
			return false
		}
		seen[item]--
	}
	return true
}
