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

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_MixedRoleUniverseDoesNotForceAuxiliaryRows(t *testing.T) {
	scope := SourceScopeAll
	rm := sourceInventoryProjectionRequestModel(&scope)
	rm.SourceInventoryProfile.TargetRoles = []AnswerCandidateRole{
		AnswerCandidateRoleFunction,
		AnswerCandidateRoleType,
		AnswerCandidateRoleField,
	}
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"."},
		SourceClasses: []SourceInventorySourceClassCount{
			{Role: SourcePathRoleFixture, Count: 7, Complete: true},
		},
		Sets: []SourceInventoryObservationSet{
			{
				Role:     AnswerCandidateRoleFunction,
				Complete: true,
				Members: []SourceInventoryObservationMember{
					{Name: "extend Cart", Role: AnswerCandidateRoleFunction, File: "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", Line: 30, Language: "cangjie", CoverageState: SourceInventoryCoverageObserved},
					{Name: "native_add", Role: AnswerCandidateRoleFunction, File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 6, Language: "cangjie", CoverageState: SourceInventoryCoverageObserved},
					{Name: "main", Role: AnswerCandidateRoleFunction, File: "eval/fixtures/testdata/cangjie_minimal/main.cj", Line: 11, Language: "cangjie", CoverageState: SourceInventoryCoverageObserved},
				},
			},
			{
				Role:     AnswerCandidateRoleType,
				Complete: true,
				Members: []SourceInventoryObservationMember{
					{Name: "Bridge", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 15, Language: "cangjie", CoverageState: SourceInventoryCoverageObserved},
					{Name: "Cart", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", Line: 14, Language: "cangjie", CoverageState: SourceInventoryCoverageObserved},
					{Name: "App", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/main.cj", Line: 11, Language: "cangjie", CoverageState: SourceInventoryCoverageObserved},
					{Name: "Item", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", Line: 4, Language: "cangjie", CoverageState: SourceInventoryCoverageObserved},
				},
			},
			{
				Role:     AnswerCandidateRoleField,
				Complete: true,
				Members: []SourceInventoryObservationMember{
					{Name: "items", Role: AnswerCandidateRoleField, File: "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", Line: 15, Language: "cangjie", CoverageState: SourceInventoryCoverageObserved},
					{Name: "label", Role: AnswerCandidateRoleField, File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 16, Language: "cangjie", CoverageState: SourceInventoryCoverageObserved},
				},
			},
		},
	}
	existing := AnswerAggregateFact{
		Kind:       AnswerAggregateMemberSet,
		Label:      "requested Cangjie declarations",
		Value:      "5",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "explorer",
		Members: []string{
			"extend Cart",
			"native_add",
			"Bridge",
			"Cart",
			"App",
		},
		SupportRefs: []string{
			"extend Cart @ eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:30",
			"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
			"Bridge @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:15",
			"Cart @ eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:14",
			"App @ eval/fixtures/testdata/cangjie_minimal/main.cj:11",
		},
	}

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts([]AnswerAggregateFact{existing}, obs, rm)
	if len(got) != 1 {
		t.Fatalf("mixed role source_inventory must not synthesize field/helper principal rows over a grounded answer, got %+v", got)
	}
	if got[0].Provenance != "explorer" || got[0].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("grounded model fact should remain authoritative, got %+v", got[0])
	}
	for _, unexpected := range []string{"main", "Item", "items", "label"} {
		if stringSliceContains(got[0].Members, unexpected) {
			t.Fatalf("unexpected auxiliary member %q promoted into principal fact: %+v", unexpected, got[0].Members)
		}
	}
}

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_PreservesRowAttributesInSyntheticNotes(t *testing.T) {
	scope := SourceScopeAll
	rm := sourceInventoryProjectionRequestModel(&scope)
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources"},
		SourceClasses: []SourceInventorySourceClassCount{
			{Role: SourcePathRoleThirdParty, Count: 1, Complete: true},
		},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Members: []SourceInventoryObservationMember{{
				Name:          "extend String",
				Role:          AnswerCandidateRoleFunction,
				File:          "internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj",
				Line:          6,
				Language:      "cangjie",
				CoverageState: SourceInventoryCoverageObserved,
				Attributes: []SourceInventoryObservationAttribute{{
					Name:          "demo.stringext",
					Role:          AnswerCandidateRolePackage,
					File:          "internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj",
					Line:          4,
					Language:      "cangjie",
					CoverageState: SourceInventoryCoverageObserved,
				}},
			}},
		}},
	}

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts(nil, obs, rm)
	if len(got) != 1 {
		t.Fatalf("expected synthetic source-inventory row-set fact, got %+v", got)
	}
	if len(got[0].MemberNotes) != 1 ||
		!strings.Contains(got[0].MemberNotes[0], "package=demo.stringext") ||
		!strings.Contains(got[0].MemberNotes[0], "04_extend_operator.cj:4") {
		t.Fatalf("package attribute should be preserved in member note, got %+v", got[0].MemberNotes)
	}
	sets := CompileEnumerationDisplaySets(&rm, &AnswerSurfacePlan{
		StableAggregateFacts:       got,
		SourceInventoryObservation: obs,
	})
	if len(sets) != 1 || len(sets[0].Rows) != 1 {
		t.Fatalf("enumeration rows = %+v", sets)
	}
	attrs := sets[0].Rows[0].Attributes
	if len(attrs) != 1 || attrs[0].Role != AnswerCandidateRolePackage || attrs[0].Name != "demo.stringext" || attrs[0].Location != "internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj:4" {
		t.Fatalf("package attribute not preserved on row: %+v", sets[0].Rows[0])
	}
}

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_DemotesPackageAttributeMemberSetWhenPackageIsNotPrincipalRole(t *testing.T) {
	scope := SourceScopeAll
	rm := sourceInventoryProjectionRequestModel(&scope)
	rm.SourceInventoryProfile.TargetRoles = []AnswerCandidateRole{
		AnswerCandidateRoleFunction,
		AnswerCandidateRoleType,
	}
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"eval/fixtures/testdata/cangjie_minimal"},
		SourceClasses: []SourceInventorySourceClassCount{
			{Role: SourcePathRoleFixture, Count: 2, Complete: true},
		},
		Sets: []SourceInventoryObservationSet{
			{
				Role:     AnswerCandidateRoleFunction,
				Complete: true,
				Members: []SourceInventoryObservationMember{{
					Name:          "native_add",
					Role:          AnswerCandidateRoleFunction,
					File:          "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj",
					Line:          6,
					Language:      "cangjie",
					CoverageState: SourceInventoryCoverageObserved,
					Attributes: []SourceInventoryObservationAttribute{{
						Name:          "demo.bridge",
						Role:          AnswerCandidateRolePackage,
						File:          "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj",
						Line:          4,
						Language:      "cangjie",
						CoverageState: SourceInventoryCoverageObserved,
					}},
				}},
			},
			{
				Role:     AnswerCandidateRoleType,
				Complete: true,
				Members: []SourceInventoryObservationMember{{
					Name:          "Bridge",
					Role:          AnswerCandidateRoleType,
					File:          "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj",
					Line:          15,
					Language:      "cangjie",
					CoverageState: SourceInventoryCoverageObserved,
					Attributes: []SourceInventoryObservationAttribute{{
						Name:          "demo.bridge",
						Role:          AnswerCandidateRolePackage,
						File:          "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj",
						Line:          4,
						Language:      "cangjie",
						CoverageState: SourceInventoryCoverageObserved,
					}},
				}},
			},
		},
	}
	facts := []AnswerAggregateFact{{
		Kind:        AnswerAggregateMemberSet,
		Label:       "package paths",
		Value:       "1",
		Role:        AnswerAggregateRolePrincipalAnswer,
		Provenance:  "model",
		Members:     []string{"demo.bridge"},
		SupportRefs: []string{"demo.bridge @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:4"},
		MemberNotes: []string{"demo.bridge @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:4"},
	}}

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts(facts, obs, rm)
	if len(got) != 2 {
		t.Fatalf("expected demoted package attribute fact plus synthetic principal row set, got %+v", got)
	}
	if got[0].Role != AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("package attribute member-set should be supporting coverage, got %+v", got[0])
	}
	if !strings.Contains(got[0].Provenance, "demoted:source_inventory_attribute_member_set") {
		t.Fatalf("demotion provenance missing: %+v", got[0])
	}
	refs := PrincipalAggregateMemberSetFactRefsForRequest(got, &rm)
	if len(refs) != 1 || refs[0].Fact.Provenance != SourceInventoryPrincipalRowSetAggregateProvenance {
		t.Fatalf("package attribute fact should not remain a principal obligation, refs=%+v got=%+v", refs, got)
	}
}

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_KeepsPackageMemberSetWhenPackageIsPrincipalRole(t *testing.T) {
	scope := SourceScopeAll
	rm := sourceInventoryProjectionRequestModel(&scope)
	rm.SourceInventoryProfile.TargetRoles = []AnswerCandidateRole{AnswerCandidateRolePackage}
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"eval/fixtures/testdata/cangjie_minimal"},
		SourceClasses: []SourceInventorySourceClassCount{
			{Role: SourcePathRoleFixture, Count: 1, Complete: true},
		},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Members: []SourceInventoryObservationMember{{
				Name:          "native_add",
				Role:          AnswerCandidateRoleFunction,
				File:          "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj",
				Line:          6,
				Language:      "cangjie",
				CoverageState: SourceInventoryCoverageObserved,
				Attributes: []SourceInventoryObservationAttribute{{
					Name:          "demo.bridge",
					Role:          AnswerCandidateRolePackage,
					File:          "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj",
					Line:          4,
					Language:      "cangjie",
					CoverageState: SourceInventoryCoverageObserved,
				}},
			}},
		}},
	}
	facts := []AnswerAggregateFact{{
		Kind:        AnswerAggregateMemberSet,
		Label:       "package paths",
		Value:       "1",
		Role:        AnswerAggregateRolePrincipalAnswer,
		Provenance:  "model",
		Members:     []string{"demo.bridge"},
		SupportRefs: []string{"demo.bridge @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:4"},
	}}

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts(facts, obs, rm)
	if len(got) != 1 {
		t.Fatalf("package inventory should preserve package fact without synthesizing function rows, got %+v", got)
	}
	if got[0].Role != AnswerAggregateRolePrincipalAnswer || strings.Contains(got[0].Provenance, "demoted:source_inventory_attribute_member_set") {
		t.Fatalf("package principal fact should remain principal, got %+v", got[0])
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
