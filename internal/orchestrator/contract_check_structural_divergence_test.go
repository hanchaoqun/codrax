package orchestrator

import (
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ── runStructuralEnumerationDivergenceOracle (P3 #6 precise, 2026-05-03) ──

// buildLooperGraph constructs a synthetic graph with:
//   - one Looper interface (iface.go)
//   - three concrete implementers: alpha, beta, gamma in distinct files
// SymbolDefs / SymbolByID populated so ImplementersOf returns all three.
func buildLooperGraph(t *testing.T) (*repotypes.Graph, *repotypes.Symbol) {
	t.Helper()
	ifaceFile := &repotypes.FileInfo{RelPath: "iface.go", Language: "go"}
	a := &repotypes.FileInfo{RelPath: "impl_alpha.go", Language: "go"}
	b := &repotypes.FileInfo{RelPath: "impl_beta.go", Language: "go"}
	c := &repotypes.FileInfo{RelPath: "impl_gamma.go", Language: "go"}

	ifaceSym := repotypes.Symbol{Name: "Looper", Kind: "interface", File: "iface.go"}
	ifaceSym.ID = repotypes.DeriveSymbolID(ifaceFile, &ifaceSym)
	ifaceFile.Symbols = []repotypes.Symbol{ifaceSym}

	alpha := repotypes.Symbol{Name: "alpha", Kind: "struct", File: "impl_alpha.go", Implements: []repotypes.SymbolID{ifaceSym.ID}}
	alpha.ID = repotypes.DeriveSymbolID(a, &alpha)
	a.Symbols = []repotypes.Symbol{alpha}

	beta := repotypes.Symbol{Name: "beta", Kind: "struct", File: "impl_beta.go", Implements: []repotypes.SymbolID{ifaceSym.ID}}
	beta.ID = repotypes.DeriveSymbolID(b, &beta)
	b.Symbols = []repotypes.Symbol{beta}

	gamma := repotypes.Symbol{Name: "gamma", Kind: "struct", File: "impl_gamma.go", Implements: []repotypes.SymbolID{ifaceSym.ID}}
	gamma.ID = repotypes.DeriveSymbolID(c, &gamma)
	c.Symbols = []repotypes.Symbol{gamma}

	g := &repotypes.Graph{
		Files:      []*repotypes.FileInfo{ifaceFile, a, b, c},
		FileIndex:  map[string]*repotypes.FileInfo{"iface.go": ifaceFile, "impl_alpha.go": a, "impl_beta.go": b, "impl_gamma.go": c},
		SymbolDefs: map[string][]*repotypes.Symbol{"Looper": {&ifaceFile.Symbols[0]}},
		SymbolByID: map[repotypes.SymbolID]*repotypes.Symbol{
			ifaceSym.ID: &ifaceFile.Symbols[0],
			alpha.ID:    &a.Symbols[0],
			beta.ID:     &b.Symbols[0],
			gamma.ID:    &c.Symbols[0],
		},
	}
	return g, &ifaceFile.Symbols[0]
}

func newMutableWithGraph(t *testing.T, g *repotypes.Graph) *types.MutableState {
	t.Helper()
	mut := &types.MutableState{}
	mut.SetSearchGraph(g)
	return mut
}

func looperRequestModel() *types.RequestModel {
	rm := &types.RequestModel{}
	rm.Predicates.IsCategoryEnumeration = true
	rm.AnalyzerHints.PrimaryEntities = []string{"Looper"}
	return rm
}

func TestStructuralDivergence_AllImplementersIncludedNoFire(t *testing.T) {
	g, _ := buildLooperGraph(t)
	mut := newMutableWithGraph(t, g)
	doc := &types.AnswerDocument{
		Shape: types.ShapeListOfSymbols,
		Symbols: []types.AnswerSymbol{
			{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"},
		},
	}
	if vs := runStructuralEnumerationDivergenceOracle(doc, looperRequestModel(), mut); len(vs) != 0 {
		t.Errorf("expected pass when all implementers included; got %+v", vs)
	}
}

func TestStructuralDivergence_SilentDropFires(t *testing.T) {
	g, _ := buildLooperGraph(t)
	mut := newMutableWithGraph(t, g)
	doc := &types.AnswerDocument{
		Shape: types.ShapeListOfSymbols,
		Symbols: []types.AnswerSymbol{
			{Name: "alpha"}, {Name: "beta"}, // gamma silently dropped
		},
		Summary: "We list two of the implementers found in the repo.",
	}
	vs := runStructuralEnumerationDivergenceOracle(doc, looperRequestModel(), mut)
	if len(vs) != 1 || vs[0].Kind != types.ViolStructuralEnumerationDivergence {
		t.Fatalf("expected ViolStructuralEnumerationDivergence on silent drop; got %+v", vs)
	}
	if !strings.Contains(vs[0].Detail, "gamma") {
		t.Errorf("expected detail to name the missing implementer; got %q", vs[0].Detail)
	}
	if vs[0].SuspectedRoot.IRField != "structural_enumeration_divergence" {
		t.Errorf("expected IRField=structural_enumeration_divergence; got %q", vs[0].SuspectedRoot.IRField)
	}
}

func TestStructuralDivergence_NamedInSummaryNoFire(t *testing.T) {
	// LLM omitted gamma from symbols[] but acknowledged it verbatim
	// in summary as an exception case — divergence is described, no
	// transparency failure, oracle should pass.
	g, _ := buildLooperGraph(t)
	mut := newMutableWithGraph(t, g)
	doc := &types.AnswerDocument{
		Shape: types.ShapeListOfSymbols,
		Symbols: []types.AnswerSymbol{
			{Name: "alpha"}, {Name: "beta"},
		},
		Summary: "Found alpha and beta as full implementers. Note: gamma also has the method set but the author comment at impl_gamma.go:5 states it should not be considered a real implementer.",
	}
	if vs := runStructuralEnumerationDivergenceOracle(doc, looperRequestModel(), mut); len(vs) != 0 {
		t.Errorf("expected pass when divergence named in summary; got %+v", vs)
	}
}

func TestStructuralDivergence_NotCategoryEnumerationNoFire(t *testing.T) {
	g, _ := buildLooperGraph(t)
	mut := newMutableWithGraph(t, g)
	rm := &types.RequestModel{}
	rm.Predicates.IsCategoryEnumeration = false // not an enumeration question
	rm.AnalyzerHints.PrimaryEntities = []string{"Looper"}
	doc := &types.AnswerDocument{
		Shape: types.ShapeExplanation,
		Symbols: []types.AnswerSymbol{
			{Name: "alpha"},
		},
	}
	if vs := runStructuralEnumerationDivergenceOracle(doc, rm, mut); len(vs) != 0 {
		t.Errorf("expected pass on non-enumeration question; got %+v", vs)
	}
}

func TestStructuralDivergence_NoInterfaceMatchNoFire(t *testing.T) {
	g, _ := buildLooperGraph(t)
	mut := newMutableWithGraph(t, g)
	rm := &types.RequestModel{}
	rm.Predicates.IsCategoryEnumeration = true
	rm.AnalyzerHints.PrimaryEntities = []string{"NotAnInterface"} // no resolves
	doc := &types.AnswerDocument{Shape: types.ShapeListOfSymbols}
	if vs := runStructuralEnumerationDivergenceOracle(doc, rm, mut); len(vs) != 0 {
		t.Errorf("expected pass when entity does not name an interface; got %+v", vs)
	}
}

func TestStructuralDivergence_NilGuards(t *testing.T) {
	g, _ := buildLooperGraph(t)
	mut := newMutableWithGraph(t, g)
	if vs := runStructuralEnumerationDivergenceOracle(nil, looperRequestModel(), mut); len(vs) != 0 {
		t.Errorf("expected nil pass on nil doc; got %+v", vs)
	}
	if vs := runStructuralEnumerationDivergenceOracle(&types.AnswerDocument{}, nil, mut); len(vs) != 0 {
		t.Errorf("expected nil pass on nil rm; got %+v", vs)
	}
	if vs := runStructuralEnumerationDivergenceOracle(&types.AnswerDocument{}, looperRequestModel(), nil); len(vs) != 0 {
		t.Errorf("expected nil pass on nil mut; got %+v", vs)
	}
}
