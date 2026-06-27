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

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_IgnoresIncompleteObservation(t *testing.T) {
	rm := sourceInventoryProjectionRequestModel(nil)
	obs := sourceInventoryProjectionObservation(
		SourceInventoryObservationMember{Name: "Run", Role: AnswerCandidateRoleFunction, File: "thirdparty/cangjie/run.cj", Line: 7, Language: "cangjie"},
		SourceInventoryObservationMember{Name: "command_count", Role: AnswerCandidateRoleFunction, File: "eval/fixtures/c-macro-platform/src/dispatch.c", Line: 18, Language: "c"},
	)
	obs.Complete = false
	obs.Sets[0].Complete = false
	obs.Sets[0].Count = 110
	existing := AnswerAggregateFact{
		Kind:        AnswerAggregateMemberSet,
		Label:       "requested cangjie inventory",
		Value:       "1",
		Role:        AnswerAggregateRolePrincipalAnswer,
		Provenance:  "explorer",
		Members:     []string{"Run"},
		SupportRefs: []string{"Run @ thirdparty/cangjie/run.cj:7"},
	}

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts([]AnswerAggregateFact{existing}, obs, rm)
	if len(got) != 1 {
		t.Fatalf("incomplete observations must not synthesize required all-inventory facts, got %+v", got)
	}
	if got[0].Provenance != "explorer" || strings.Join(got[0].Members, ",") != "Run" {
		t.Fatalf("existing requested aggregate should remain authoritative, got %+v", got[0])
	}
}

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_DoesNotOverrideDisjointPrincipalFamily(t *testing.T) {
	rm := sourceInventoryProjectionRequestModel(nil)
	obs := sourceInventoryProjectionObservation(
		SourceInventoryObservationMember{Name: "parseCangjie", Role: AnswerCandidateRoleFunction, File: "internal/tool/repomap/index/extract_cangjie.go", Line: 41, Language: "go"},
		SourceInventoryObservationMember{Name: "cangjieParser", Role: AnswerCandidateRoleFunction, File: "internal/tool/repomap/index/cangjie_parser.go", Line: 18, Language: "go"},
	)
	existing := AnswerAggregateFact{
		Kind:       AnswerAggregateMemberSet,
		Label:      "requested source rows",
		Value:      "3",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "explorer",
		Members: []string{
			"Bridge",
			"Cart",
			"App",
		},
		SupportRefs: []string{
			"Bridge @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:8",
			"Cart @ eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:4",
			"App @ eval/fixtures/testdata/cangjie_minimal/main.cj:5",
		},
	}

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts([]AnswerAggregateFact{existing}, obs, rm)
	if len(got) != 1 {
		t.Fatalf("disjoint navigation inventory must not become a synthetic hard principal row-set, got %+v", got)
	}
	if got[0].Provenance != "explorer" || got[0].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("existing grounded principal fact should remain authoritative, got %+v", got[0])
	}
	if strings.Join(got[0].Members, ",") != "Bridge,Cart,App" {
		t.Fatalf("existing principal members changed: %+v", got[0].Members)
	}
}

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_FiltersMixedLanguageRowsWhenModelFactIsGrounded(t *testing.T) {
	rm := sourceInventoryProjectionRequestModel(nil)
	rm.SourceInventoryProfile.TargetRoles = []AnswerCandidateRole{AnswerCandidateRoleFunction, AnswerCandidateRoleType}
	obs := sourceInventoryProjectionObservation(
		SourceInventoryObservationMember{Name: "Index", Role: AnswerCandidateRoleType, File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", Line: 5, Language: "arkts", SurfaceTerms: []string{"@Entry", "@Component"}},
		SourceInventoryObservationMember{Name: "defaultHeader", Role: AnswerCandidateRoleFunction, File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets", Line: 8, Language: "arkts", SurfaceTerms: []string{"@Builder"}},
		SourceInventoryObservationMember{Name: "extractStableBuilderIdentity", Role: AnswerCandidateRoleFunction, File: "internal/agent/explorer.go", Line: 16120, Language: "go"},
	)
	obs.Sets = append(obs.Sets, SourceInventoryObservationSet{
		Role:     AnswerCandidateRoleType,
		Complete: true,
		Members: []SourceInventoryObservationMember{{
			Name:          "Index",
			Role:          AnswerCandidateRoleType,
			File:          "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
			Line:          5,
			Language:      "arkts",
			SurfaceTerms:  []string{"@Entry", "@Component"},
			CoverageState: SourceInventoryCoverageObserved,
		}},
	})
	existing := AnswerAggregateFact{
		Kind:       AnswerAggregateMemberSet,
		Label:      "ArkTS decorator members",
		Value:      "2",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "explorer",
		Members: []string{
			"Index",
			"defaultHeader",
		},
		SupportRefs: []string{
			"Index @ internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets:5",
			"defaultHeader @ internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets:8",
		},
	}

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts([]AnswerAggregateFact{existing}, obs, rm)
	if len(got) != 1 {
		t.Fatalf("mixed-language source_inventory helpers must stay advisory once a grounded principal family exists, got %+v", got)
	}
	if got[0].Provenance != "explorer" || got[0].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("existing ArkTS principal fact should remain authoritative, got %+v", got[0])
	}
	if strings.Join(got[0].Members, ",") != "Index,defaultHeader" {
		t.Fatalf("existing ArkTS members changed: %+v", got[0].Members)
	}
}

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_PrincipleUsesRequestedRoleOnly(t *testing.T) {
	rm := sourceInventoryProjectionRequestModel(nil)
	obs := sourceInventoryProjectionObservation(
		SourceInventoryObservationMember{Name: "Run", Role: AnswerCandidateRoleFunction, File: "thirdparty/cangjie/run.cj", Line: 7, Language: "cangjie"},
	)
	obs.Sets = append(obs.Sets, SourceInventoryObservationSet{
		Role:     AnswerCandidateRoleType,
		Complete: true,
		Members: []SourceInventoryObservationMember{{
			Name:          "HelperType",
			Role:          AnswerCandidateRoleType,
			File:          "internal/support/helper.go",
			Line:          22,
			Language:      "go",
			CoverageState: SourceInventoryCoverageObserved,
		}},
	})

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts(nil, obs, rm)
	if len(got) != 1 {
		t.Fatalf("projected facts = %+v, want one function-only source-inventory aggregate", got)
	}
	if strings.Join(got[0].Members, ",") != "Run" {
		t.Fatalf("projection must not import complete-but-unrequested support roles into the hard principal row-set: %+v", got[0])
	}
	if got[0].Provenance != SourceInventoryPrincipalRowSetAggregateProvenance {
		t.Fatalf("projected fact provenance drifted: %+v", got[0])
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

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_ArchitectureNarrativeKeepsInventoryAdvisory(t *testing.T) {
	rm := RequestModel{
		Intent:      IntentExplain,
		Scenario:    ScenarioArchitectureExplain,
		Complexity:  ComplexityComplex,
		Predicates:  SemanticPredicates{IsCategoryEnumeration: true, IsCrossComponent: true},
		DiagramHint: &DiagramHint{Kind: DiagramArchitecture},
		SubTopics: []SubTopic{
			{Summary: "agent roles"},
			{Summary: "agent relationships"},
		},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction, AnswerCandidateRoleType},
		},
	}
	obs := sourceInventoryProjectionObservation(
		SourceInventoryObservationMember{Name: "NewSubAgentRegistry", Role: AnswerCandidateRoleFunction, File: "internal/agent/subagent.go", Line: 22, Language: "go"},
		SourceInventoryObservationMember{Name: "SubAgentRegistry", Role: AnswerCandidateRoleType, File: "internal/agent/subagent.go", Line: 12, Language: "go"},
	)
	obs.Sets = append(obs.Sets, SourceInventoryObservationSet{
		Role:     AnswerCandidateRoleType,
		Complete: true,
		Members: []SourceInventoryObservationMember{{
			Name:          "SubAgentRegistry",
			Role:          AnswerCandidateRoleType,
			File:          "internal/agent/subagent.go",
			Line:          12,
			Language:      "go",
			CoverageState: SourceInventoryCoverageObserved,
		}},
	})

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts(nil, obs, rm)
	if len(got) != 0 {
		t.Fatalf("architecture narrative source_inventory rows must stay advisory, got %+v", got)
	}
}

func sourceInventoryProjectionRequestModel(scope *SourceScope) RequestModel {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqEnumeration),
			Entities: []string{"Run", "Serve"},
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all functions"},
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
