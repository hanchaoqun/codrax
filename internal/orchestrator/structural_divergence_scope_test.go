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
