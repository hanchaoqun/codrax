package orchestrator

import (
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func buildDivergenceScopeGraph(t *testing.T) *repotypes.Graph {
	t.Helper()
	ifaceFile := &repotypes.FileInfo{RelPath: "iface.go", Language: "go"}
	alphaFile := &repotypes.FileInfo{RelPath: "alpha.go", Language: "go"}
	betaFile := &repotypes.FileInfo{RelPath: "beta.go", Language: "go"}

	iface := repotypes.Symbol{Name: "Looper", Kind: "interface", File: "iface.go", Line: 7}
	iface.ID = repotypes.DeriveSymbolID(ifaceFile, &iface)
	ifaceFile.Symbols = []repotypes.Symbol{iface}

	alpha := repotypes.Symbol{Name: "alpha", Kind: "struct", File: "alpha.go", Line: 14, Implements: []repotypes.SymbolID{iface.ID}}
	alpha.ID = repotypes.DeriveSymbolID(alphaFile, &alpha)
	alphaFile.Symbols = []repotypes.Symbol{alpha}

	beta := repotypes.Symbol{Name: "beta", Kind: "struct", File: "beta.go", Line: 22, Implements: []repotypes.SymbolID{iface.ID}}
	beta.ID = repotypes.DeriveSymbolID(betaFile, &beta)
	betaFile.Symbols = []repotypes.Symbol{beta}

	return &repotypes.Graph{
		Files:      []*repotypes.FileInfo{ifaceFile, alphaFile, betaFile},
		FileIndex:  map[string]*repotypes.FileInfo{"iface.go": ifaceFile, "alpha.go": alphaFile, "beta.go": betaFile},
		SymbolDefs: map[string][]*repotypes.Symbol{"Looper": {&ifaceFile.Symbols[0]}},
		SymbolByID: map[repotypes.SymbolID]*repotypes.Symbol{
			iface.ID: &ifaceFile.Symbols[0],
			alpha.ID: &alphaFile.Symbols[0],
			beta.ID:  &betaFile.Symbols[0],
		},
	}
}

func divergenceScopeAnswer(labels ...string) *types.AnswerDocumentV2 {
	items := make([]types.AnswerBlockItem, 0, len(labels))
	for _, label := range labels {
		items = append(items, types.AnswerBlockItem{Label: label, Text: "covered"})
	}
	return &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:    "members",
			Kind:  types.BlockBulletList,
			Items: items,
		}},
	}
}

func divergenceScopeTable(cells ...string) *types.AnswerDocumentV2 {
	items := make([]types.AnswerBlockItem, 0, len(cells))
	for _, cell := range cells {
		items = append(items, types.AnswerBlockItem{Cells: []string{cell, "covered"}})
	}
	return &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:      "members",
			Kind:    types.BlockTable,
			Columns: []string{"member", "status"},
			Items:   items,
		}},
	}
}

func retainTypedRelationPrincipalMembers(mut *types.MutableState, members ...string) {
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:       types.AnswerAggregateMemberSet,
		Label:      "implementers",
		Value:      "2",
		Role:       types.AnswerAggregateRolePrincipalAnswer,
		Members:    members,
		Provenance: types.TypedRelationPrincipalMemberSetAggregateProvenance,
	}})
	mut.SetInvestigationComplete("accepted exact typed relation members")
	mut.RetainInvestigationAggregateFacts()
}

func TestStructuralEnumerationDivergence_SkipsDerivedContextEntity(t *testing.T) {
	mut := types.NewMutableState("")
	mut.SetSearchGraph(buildDivergenceScopeGraph(t))
	rm := &types.RequestModel{
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: types.AnalyzerHints{
			MentionedEntities: []string{"internal/analysis"},
			PrimaryEntities:   []string{"internal/analysis"},
			Entities:          []string{"internal/analysis", "Looper"},
			DerivedEntities:   []string{"Looper"},
		},
	}
	got := runStructuralEnumerationDivergenceOracleV2(divergenceScopeAnswer("aggregator", "compiler"), rm, mut, nil)
	if len(got) != 0 {
		t.Fatalf("derived context implementer relation must not gate unrelated enumeration, got %+v", got)
	}
}

func TestStructuralEnumerationDivergence_FiresForPrimaryRelationScope(t *testing.T) {
	mut := types.NewMutableState("")
	mut.SetSearchGraph(buildDivergenceScopeGraph(t))
	rm := &types.RequestModel{
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"Looper"},
		},
	}
	got := runStructuralEnumerationDivergenceOracleV2(divergenceScopeAnswer("alpha"), rm, mut, nil)
	if len(got) != 1 {
		t.Fatalf("primary relation scope omission should fire once, got %+v", got)
	}
	if got[0].Kind != types.ViolStructuralEnumerationDivergence {
		t.Fatalf("kind = %q, want structural divergence", got[0].Kind)
	}
	if !strings.Contains(got[0].Detail, "beta") || !strings.Contains(got[0].Detail, "Looper") {
		t.Fatalf("detail should name missing implementer and relation scope, got %q", got[0].Detail)
	}
}

func TestStructuralEnumerationDivergence_DefaultProductionDoesNotPromoteTestImplementer(t *testing.T) {
	mut := types.NewMutableState("")
	graph := buildDivergenceScopeGraph(t)
	iface := graph.SymbolDefs["Looper"][0]
	testFile := &repotypes.FileInfo{RelPath: "loop_test.go", Language: "go"}
	testImpl := repotypes.Symbol{Name: "loopTestDouble", Kind: "struct", File: "loop_test.go", Line: 9, Implements: []repotypes.SymbolID{iface.ID}}
	testImpl.ID = repotypes.DeriveSymbolID(testFile, &testImpl)
	testFile.Symbols = []repotypes.Symbol{testImpl}
	graph.Files = append(graph.Files, testFile)
	graph.FileIndex[testFile.RelPath] = testFile
	graph.SymbolByID[testImpl.ID] = &testFile.Symbols[0]
	mut.SetSearchGraph(graph)

	rm := &types.RequestModel{
		Predicates:         types.SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints:      types.AnalyzerHints{PrimaryEntities: []string{"Looper"}},
		SourceScopeProfile: &types.SourceScopeProfile{RequestedScope: types.SourceScopeProduction},
	}
	got := runStructuralEnumerationDivergenceOracleV2(divergenceScopeAnswer("alpha", "beta"), rm, mut, nil)
	if len(got) != 0 {
		t.Fatalf("test implementer must remain supporting coverage under production scope, got %+v", got)
	}
}

func TestStructuralEnumerationDivergence_AllScopePromotesTestImplementer(t *testing.T) {
	mut := types.NewMutableState("")
	graph := buildDivergenceScopeGraph(t)
	iface := graph.SymbolDefs["Looper"][0]
	testFile := &repotypes.FileInfo{RelPath: "loop_test.go", Language: "go"}
	testImpl := repotypes.Symbol{Name: "loopTestDouble", Kind: "struct", File: "loop_test.go", Line: 9, Implements: []repotypes.SymbolID{iface.ID}}
	testImpl.ID = repotypes.DeriveSymbolID(testFile, &testImpl)
	testFile.Symbols = []repotypes.Symbol{testImpl}
	graph.Files = append(graph.Files, testFile)
	graph.FileIndex[testFile.RelPath] = testFile
	graph.SymbolByID[testImpl.ID] = &testFile.Symbols[0]
	mut.SetSearchGraph(graph)

	rm := &types.RequestModel{
		Predicates:         types.SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints:      types.AnalyzerHints{PrimaryEntities: []string{"Looper"}},
		SourceScopeProfile: &types.SourceScopeProfile{RequestedScope: types.SourceScopeAll, IncludeAuxiliaryAsPrincipal: true},
	}
	got := runStructuralEnumerationDivergenceOracleV2(divergenceScopeAnswer("alpha", "beta"), rm, mut, nil)
	if len(got) != 1 || !strings.Contains(got[0].Detail, "loopTestDouble") {
		t.Fatalf("all-source scope must keep omitted test implementer in principal divergence, got %+v", got)
	}
}

func TestV2EmittedNameSet_UsesFirstCellAsCanonicalTableIdentity(t *testing.T) {
	names := v2EmittedNameSet(divergenceScopeTable("alpha", "beta"))
	if !names["alpha"] || !names["beta"] || len(names) != 2 {
		t.Fatalf("first cells must be recognized as the two visible member identities, got %+v", names)
	}
}

func TestStructuralEnumerationDivergence_ExactTypedPrincipalSetBoundsAllSourceOracle(t *testing.T) {
	mut := types.NewMutableState("")
	graph := buildDivergenceScopeGraph(t)
	iface := graph.SymbolDefs["Looper"][0]
	testFile := &repotypes.FileInfo{RelPath: "loop_test.go", Language: "go"}
	testImpl := repotypes.Symbol{Name: "loopTestDouble", Kind: "struct", File: "loop_test.go", Line: 9, Implements: []repotypes.SymbolID{iface.ID}}
	testImpl.ID = repotypes.DeriveSymbolID(testFile, &testImpl)
	testFile.Symbols = []repotypes.Symbol{testImpl}
	graph.Files = append(graph.Files, testFile)
	graph.FileIndex[testFile.RelPath] = testFile
	graph.SymbolByID[testImpl.ID] = &testFile.Symbols[0]
	mut.SetSearchGraph(graph)
	retainTypedRelationPrincipalMembers(mut, "alpha", "beta")

	rm := &types.RequestModel{
		Intent:             types.IntentEnumerate,
		Predicates:         types.SemanticPredicates{IsCategoryEnumeration: true, IsRelationalLookup: true},
		AnalyzerHints:      types.AnalyzerHints{PrimaryEntities: []string{"Looper"}},
		SourceScopeProfile: &types.SourceScopeProfile{RequestedScope: types.SourceScopeAll, IncludeAuxiliaryAsPrincipal: true},
	}
	bus := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: *rm}, Mutable: mut}
	got := runStructuralEnumerationDivergenceOracleV2(divergenceScopeTable("alpha", "beta"), rm, mut, bus)
	if len(got) != 0 {
		t.Fatalf("support-only test implementer and canonical table cells must not create a false principal omission, got %+v", got)
	}

	got = runStructuralEnumerationDivergenceOracleV2(divergenceScopeTable("alpha"), rm, mut, bus)
	if len(got) != 1 || !strings.Contains(got[0].Detail, "beta") || strings.Contains(got[0].Detail, "loopTestDouble") {
		t.Fatalf("a real principal omission must still fire without promoting support-only members, got %+v", got)
	}
}
