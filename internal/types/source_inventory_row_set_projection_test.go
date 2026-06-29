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

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_PreservesModelMembersWithInlineLocationsAndAttributes(t *testing.T) {
	rm := sourceInventoryProjectionRequestModel(nil)
	obs := sourceInventoryProjectionObservation(
		SourceInventoryObservationMember{Name: "native_add", Role: AnswerCandidateRoleFunction, File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 6, Language: "cangjie", Attributes: []SourceInventoryObservationAttribute{{Role: AnswerCandidateRolePackage, Name: "demo.bridge"}}},
		SourceInventoryObservationMember{Name: "native_add", Role: AnswerCandidateRoleFunction, File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj", Line: 6, Language: "cangjie", Attributes: []SourceInventoryObservationAttribute{{Role: AnswerCandidateRolePackage, Name: "demo.ffi"}}},
	)
	obs.Active = true
	existing := AnswerAggregateFact{
		Kind:       AnswerAggregateMemberSet,
		Label:      "foreign func 声明",
		Value:      "2",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "explorer",
		Members: []string{
			"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6 (package demo.bridge)",
			"native_add @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6 (package demo.ffi)",
		},
	}

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts([]AnswerAggregateFact{existing}, obs, rm)
	if len(got) != 1 {
		t.Fatalf("inline-located model fact should be preserved without synthetic duplicate, got %+v", got)
	}
	if got[0].Provenance != "explorer" || got[0].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("model fact was unexpectedly rewritten: %+v", got[0])
	}
	sets := CompileEnumerationDisplaySets(&rm, &AnswerSurfacePlan{
		StableAggregateFacts:       got,
		SourceInventoryObservation: obs,
	})
	if len(sets) != 1 || len(sets[0].Rows) != 2 {
		t.Fatalf("inline-located model fact should compile two same-name rows, got %+v", sets)
	}
	visible := sets[0].Rows[0].Location + "\n" + sets[0].Rows[1].Location
	for _, want := range []string{"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6", "internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("compiled rows lost source location %q:\n%s", want, visible)
		}
	}
}

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_UpgradesShortSupportRefsForDuplicateLabels(t *testing.T) {
	rm := sourceInventoryProjectionRequestModel(nil)
	obs := sourceInventoryProjectionObservation(
		SourceInventoryObservationMember{Name: "native_add", Role: AnswerCandidateRoleFunction, File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 6, Language: "cangjie", Attributes: []SourceInventoryObservationAttribute{{Role: AnswerCandidateRolePackage, Name: "demo.bridge"}}},
		SourceInventoryObservationMember{Name: "native_add", Role: AnswerCandidateRoleFunction, File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj", Line: 6, Language: "cangjie", Attributes: []SourceInventoryObservationAttribute{{Role: AnswerCandidateRolePackage, Name: "demo.ffi"}}},
	)
	existing := AnswerAggregateFact{
		Kind:       AnswerAggregateMemberSet,
		Label:      "foreign func 声明",
		Value:      "2",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "explorer",
		Members: []string{
			"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
			"native_add @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6",
		},
		SupportRefs: []string{
			"Bridge.cj:6",
			"07_foreign_ffi.cj:6",
		},
		MemberNotes: []string{
			"package demo.bridge",
			"package demo.ffi",
		},
	}
	facts, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{existing})
	if err != nil {
		t.Fatalf("NormalizeAnswerAggregateFacts failed: %v", err)
	}
	if len(facts) != 1 || len(facts[0].Members) != 2 || facts[0].Value != "2" {
		t.Fatalf("normalization should preserve two duplicate-label locations before projection, got %+v", facts)
	}

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts(facts, obs, rm)
	if len(got) != 1 {
		t.Fatalf("short support refs should not trigger a synthetic duplicate row-set, got %+v", got)
	}
	if got[0].Value != "2" || len(got[0].Members) != 2 || len(got[0].SupportRefs) != 2 {
		t.Fatalf("duplicate labels at distinct locations must remain two principal members, got %+v", got[0])
	}
	for _, want := range []string{
		"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
		"native_add @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6",
	} {
		if !strings.Contains(strings.Join(got[0].SupportRefs, "\n"), want) {
			t.Fatalf("support_refs should preserve precise member location %q, got %+v", want, got[0].SupportRefs)
		}
	}

	plan := &AnswerSurfacePlan{
		StableAggregateFacts:       got,
		SourceInventoryObservation: obs,
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "ev-bridge-native-add",
				Kind:            EvidenceDirect,
				Subject:         "Bridge.cj",
				Object:          "foreign func native_add",
				AnchorSymbol:    "native_add",
				AnchorKind:      AnchorDefinition,
				Source:          "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj",
				LineStart:       6,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "foreign func native_add belongs to package demo.bridge",
			},
			{
				ID:              "ev-ffi-native-add",
				Kind:            EvidenceDirect,
				Subject:         "07_foreign_ffi.cj",
				Object:          "foreign func native_add",
				AnchorSymbol:    "native_add",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj",
				LineStart:       6,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "foreign func native_add belongs to package demo.ffi",
			},
		},
	}
	sets := CompileEnumerationDisplaySets(&rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 2 {
		t.Fatalf("short support refs should still compile two duplicate-label rows, got %+v", sets)
	}
	seenRows := map[string]string{}
	for _, row := range sets[0].Rows {
		if row.DisplayLabel != "native_add" || row.Location == "" || len(row.Attributes) != 1 {
			t.Fatalf("row lost duplicate-label location or package attribute: %+v", row)
		}
		seenRows[normalizeAnswerSupportLocation(row.Location)] = row.Attributes[0].Name
	}
	for loc, wantPackage := range map[string]string{
		"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6":                  "demo.bridge",
		"internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6": "demo.ffi",
	} {
		if gotPackage := seenRows[normalizeAnswerSupportLocation(loc)]; gotPackage != wantPackage {
			t.Fatalf("display row %s package = %q, want %q; rows=%+v", loc, gotPackage, wantPackage, sets[0].Rows)
		}
	}
	supportPlan := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(supportPlan, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 2 {
		t.Fatalf("support plan should preserve two duplicate-label principal rows, got %+v", supportPlan)
	}
	seenEntries := map[string]bool{}
	for _, entry := range lane.Entries {
		if entry.Text != "native_add" || entry.Location == "" {
			t.Fatalf("support entry lost duplicate-label location: %+v", entry)
		}
		seenEntries[normalizeAnswerSupportLocation(entry.Location)] = true
	}
	for _, want := range []string{
		"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
		"internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6",
	} {
		if !seenEntries[normalizeAnswerSupportLocation(want)] {
			t.Fatalf("support plan lost location %s; entries=%+v", want, lane.Entries)
		}
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

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_MixedRoleUniverseCompletesSelectedSurfaceFamily(t *testing.T) {
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
			{Role: SourcePathRoleFixture, Count: 2, Complete: true},
			{Role: SourcePathRoleThirdParty, Count: 2, Complete: true},
		},
		Sets: []SourceInventoryObservationSet{
			{
				Role:     AnswerCandidateRoleFunction,
				Complete: true,
				Members: []SourceInventoryObservationMember{
					{Name: "native_add", Role: AnswerCandidateRoleFunction, File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj", Line: 6, Language: "cangjie", SurfaceTerms: []string{"foreign func", "foreign func native_add"}, CoverageState: SourceInventoryCoverageObserved},
				},
			},
			{
				Role:     AnswerCandidateRoleType,
				Complete: true,
				Members: []SourceInventoryObservationMember{
					{Name: "Bridge", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 15, Language: "cangjie", SurfaceTerms: []string{"public class", "public class Bridge"}, CoverageState: SourceInventoryCoverageObserved},
					{Name: "Greeter", Role: AnswerCandidateRoleType, File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj", Line: 6, Language: "cangjie", SurfaceTerms: []string{"public class", "public class Greeter"}, CoverageState: SourceInventoryCoverageObserved},
					{Name: "Version", Role: AnswerCandidateRoleType, File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/06_generic_where.cj", Line: 18, Language: "cangjie", SurfaceTerms: []string{"public class", "public class Version"}, CoverageState: SourceInventoryCoverageObserved},
					{Name: "Item", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", Line: 6, Language: "cangjie", SurfaceTerms: []string{"public struct", "public struct Item"}, CoverageState: SourceInventoryCoverageObserved},
				},
			},
			{
				Role:     AnswerCandidateRoleField,
				Complete: true,
				Members: []SourceInventoryObservationMember{
					{Name: "label", Role: AnswerCandidateRoleField, File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 16, Language: "cangjie", CoverageState: SourceInventoryCoverageObserved},
				},
			},
		},
	}
	existing := AnswerAggregateFact{
		Kind:       AnswerAggregateMemberSet,
		Label:      "public class declarations",
		Value:      "1",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "explorer",
		Members:    []string{"Bridge"},
		SupportRefs: []string{
			"Bridge: eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:15",
		},
	}

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts([]AnswerAggregateFact{existing}, obs, rm)
	if len(got) != 2 {
		t.Fatalf("expected existing fact plus completed typed surface-family projection, got %+v", got)
	}
	if got[0].Role != AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("partial model fact should be demoted under completed source-inventory family, got %+v", got[0])
	}
	systemFact := got[1]
	if systemFact.Provenance != SourceInventoryPrincipalRowSetAggregateProvenance || systemFact.Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("missing source-inventory principal projection: %+v", systemFact)
	}
	if gotMembers := strings.Join(systemFact.Members, ","); gotMembers != "Bridge,Greeter,Version" {
		t.Fatalf("projection should complete selected public-class family only, got %q", gotMembers)
	}
	for _, notWant := range []string{"native_add", "Item", "label"} {
		if strings.Contains(strings.Join(systemFact.Members, ","), notWant) {
			t.Fatalf("projection leaked non-selected mixed-family member %q: %+v", notWant, systemFact.Members)
		}
	}
}

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_DemotesOverBroadSurfaceFamilyFact(t *testing.T) {
	scope := SourceScopeAll
	rm := sourceInventoryProjectionRequestModel(&scope)
	rm.SourceInventoryProfile.TargetRoles = []AnswerCandidateRole{
		AnswerCandidateRoleType,
		AnswerCandidateRoleFunction,
		AnswerCandidateRoleConstant,
	}
	rm.SourceInventoryProfile.SourceQuotes = []string{"public class"}
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"."},
		SourceClasses: []SourceInventorySourceClassCount{
			{Role: SourcePathRoleFixture, Count: 2, Complete: true},
			{Role: SourcePathRoleThirdParty, Count: 2, Complete: true},
		},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: true,
			Members: []SourceInventoryObservationMember{
				{Name: "Bridge", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 15, Language: "cangjie", SurfaceTerms: []string{"public class", "public class Bridge"}, CoverageState: SourceInventoryCoverageObserved},
				{Name: "Service", Role: AnswerCandidateRoleType, File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/08_modifiers_combos.cj", Line: 32, Language: "cangjie", SurfaceTerms: []string{"public class", "public class Service", "public abstract class", "public abstract class Service"}, CoverageState: SourceInventoryCoverageObserved},
				{Name: "Item", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", Line: 6, Language: "cangjie", SurfaceTerms: []string{"public struct", "public struct Item"}, CoverageState: SourceInventoryCoverageObserved},
				{Name: "Drawable", Role: AnswerCandidateRoleType, File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/03_interfaces.cj", Line: 4, Language: "cangjie", SurfaceTerms: []string{"public interface", "public interface Drawable"}, CoverageState: SourceInventoryCoverageObserved},
			},
		}},
	}
	existing := AnswerAggregateFact{
		Kind:       AnswerAggregateMemberSet,
		Label:      "public class/type",
		Value:      "4",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "explorer",
		Members: []string{
			"Bridge",
			"Service",
			"Item",
			"Drawable",
		},
		SupportRefs: []string{
			"Bridge: eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:15",
			"Service: internal/thirdparty/tree-sitter-cangjie/corpus/sources/08_modifiers_combos.cj:32",
			"Item: eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:6",
			"Drawable: internal/thirdparty/tree-sitter-cangjie/corpus/sources/03_interfaces.cj:4",
		},
	}

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts([]AnswerAggregateFact{existing}, obs, rm)
	if len(got) != 2 {
		t.Fatalf("expected over-broad model fact plus narrowed source-inventory projection, got %+v", got)
	}
	if got[0].Role != AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("over-broad model fact should be demoted, got %+v", got[0])
	}
	systemFact := got[1]
	if systemFact.Provenance != SourceInventoryPrincipalRowSetAggregateProvenance ||
		systemFact.Role != AnswerAggregateRolePrincipalAnswer ||
		systemFact.Value != "2" {
		t.Fatalf("expected narrowed source-inventory principal fact, got %+v", systemFact)
	}
	if gotMembers := strings.Join(systemFact.Members, ","); gotMembers != "Bridge,Service" {
		t.Fatalf("projection should keep only requested public-class family, got %q", gotMembers)
	}
	for _, notWant := range []string{"Item", "Drawable"} {
		if stringSliceContains(systemFact.Members, notWant) {
			t.Fatalf("projection leaked non-requested family member %q: %+v", notWant, systemFact.Members)
		}
	}
}

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_CompleteLensProjectsRequestedFamilyWhenRoleSetIncomplete(t *testing.T) {
	scope := SourceScopeAll
	rm := sourceInventoryProjectionRequestModel(&scope)
	rm.SourceInventoryProfile.TargetRoles = []AnswerCandidateRole{AnswerCandidateRoleType}
	rm.SourceInventoryProfile.SourceQuotes = []string{"public class"}
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: false,
		Scopes: []string{
			".",
			"eval/fixtures/testdata/cangjie_minimal/bridge",
			"eval/fixtures/testdata/cangjie_minimal/cart",
			"internal/thirdparty/tree-sitter-cangjie/corpus/sources",
		},
		CompleteLenses: []SourceInventoryCompleteLens{{
			Role:          AnswerCandidateRoleType,
			Scopes:        []string{"eval/fixtures/testdata/cangjie_minimal/bridge", "eval/fixtures/testdata/cangjie_minimal/cart", "internal/thirdparty/tree-sitter-cangjie/corpus/sources"},
			Languages:     []string{"cangjie"},
			SourceClasses: []SourcePathRole{SourcePathRoleFixture, SourcePathRoleThirdParty},
			Count:         3,
			Total:         3,
			Provenance:    []string{"repo_lens:stage:explore"},
		}},
		Sets: []SourceInventoryObservationSet{
			{
				Role:     AnswerCandidateRoleType,
				Complete: false,
				Total:    23,
				Members: []SourceInventoryObservationMember{
					{Name: "Bridge", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 15, Language: "cangjie", SurfaceTerms: []string{"public class", "public class Bridge"}, CoverageState: SourceInventoryCoverageObserved},
					{Name: "Service", Role: AnswerCandidateRoleType, File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/08_modifiers_combos.cj", Line: 32, Language: "cangjie", SurfaceTerms: []string{"public class", "public class Service", "public abstract class", "public abstract class Service"}, CoverageState: SourceInventoryCoverageObserved},
					{Name: "Item", Role: AnswerCandidateRoleType, File: "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", Line: 6, Language: "cangjie", SurfaceTerms: []string{"public struct", "public struct Item"}, CoverageState: SourceInventoryCoverageObserved},
				},
			},
			{
				Role:     AnswerCandidateRoleConstant,
				Complete: false,
				Total:    43,
				Members: []SourceInventoryObservationMember{
					{Name: "KindSourceInventoryLensMissing", Role: AnswerCandidateRoleConstant, File: "internal/analysis/criterion/grammar.go", Line: 47, Language: "go", CoverageState: SourceInventoryCoverageObserved},
				},
			},
		},
	}
	existing := AnswerAggregateFact{
		Kind:       AnswerAggregateMemberSet,
		Label:      "public class 完整列表",
		Value:      "3",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "explorer",
		Members: []string{
			"Bridge @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:15",
			"Service @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/08_modifiers_combos.cj:32",
			"Item @ eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:6 (public struct)",
		},
		SupportRefs: []string{
			"Bridge: eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:15",
			"Service: internal/thirdparty/tree-sitter-cangjie/corpus/sources/08_modifiers_combos.cj:32",
			"Item: eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:6",
		},
	}

	got := ProjectSourceInventoryPrincipalRowSetAggregateFacts([]AnswerAggregateFact{existing}, obs, rm)
	if len(got) != 2 {
		t.Fatalf("expected demoted model fact plus precise complete-lens projection, got %+v", got)
	}
	if got[0].Role != AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("over-broad model fact should be demoted, got %+v", got[0])
	}
	systemFact := got[1]
	if systemFact.Provenance != SourceInventoryPrincipalRowSetAggregateProvenance ||
		systemFact.Role != AnswerAggregateRolePrincipalAnswer ||
		systemFact.Value != "2" {
		t.Fatalf("expected complete-lens backed public-class principal fact, got %+v", systemFact)
	}
	if gotMembers := strings.Join(systemFact.Members, ","); gotMembers != "Bridge,Service" {
		t.Fatalf("projection should keep only requested complete-lens public-class rows, got %q", gotMembers)
	}
	for _, notWant := range []string{"Item", "KindSourceInventoryLensMissing"} {
		if stringSliceContains(systemFact.Members, notWant) {
			t.Fatalf("projection leaked non-requested row %q: %+v", notWant, systemFact.Members)
		}
	}
}

func TestProjectSourceInventoryPrincipalRowSetAggregateFacts_DoesNotMirrorTypedMetadataInSyntheticNotes(t *testing.T) {
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
				Note:          "user-facing declaration note",
				SurfaceTerms:  []string{"extend extend String"},
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
		got[0].MemberNotes[0] != "user-facing declaration note" {
		t.Fatalf("projection should keep only row-authored notes, got %+v", got[0].MemberNotes)
	}
	for _, forbidden := range []string{"package=", "source_class=", "language=", "surface="} {
		if strings.Contains(strings.Join(got[0].MemberNotes, "\n"), forbidden) {
			t.Fatalf("typed metadata should not be mirrored into member notes: %+v", got[0].MemberNotes)
		}
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

func TestSourceInventoryPrincipalRowNote_DropsSurfaceCarrierNotes(t *testing.T) {
	row := SourceInventoryRow{
		Member: SourceInventoryObservationMember{
			Name:         "Animal",
			Note:         "surface=public class public class Animal",
			SurfaceTerms: []string{"public class Animal", "public class public class Animal"},
		},
	}
	if got := sourceInventoryPrincipalRowNote(row); got != "" {
		t.Fatalf("surface carrier note should not render as member note, got %q", got)
	}
	row.Member.Attributes = []SourceInventoryObservationAttribute{{
		Name: "demo.modifiers",
		Role: AnswerCandidateRolePackage,
	}}
	row.Member.Note = "surface=public class Animal; package=demo.modifiers; primary Cangjie class declaration"
	if got := sourceInventoryPrincipalRowNote(row); got != "primary Cangjie class declaration" {
		t.Fatalf("mixed system carriers should be stripped while preserving human note, got %q", got)
	}
	row.Member.Note = "visibility=public; primary Cangjie class declaration"
	if got := sourceInventoryPrincipalRowNote(row); got != "visibility=public; primary Cangjie class declaration" {
		t.Fatalf("unmatched key/value note should be preserved, got %q", got)
	}
	row.Member.Note = "primary Cangjie class declaration"
	if got := sourceInventoryPrincipalRowNote(row); got != "primary Cangjie class declaration" {
		t.Fatalf("human note should be preserved, got %q", got)
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
	rm.AnswerExclusionPolicy = &AnswerExclusionPolicy{
		IsExclusionRequested:   true,
		ExcludedCandidateRoles: []AnswerCandidateRole{AnswerCandidateRoleFixture},
	}
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
		Active:   true,
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
