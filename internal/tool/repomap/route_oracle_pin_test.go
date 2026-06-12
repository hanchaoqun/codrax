package repomap

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// Route symbols enter the SymbolOracle universe (BuildGraph indexes
// every kind unfiltered into SymbolDefs), which feeds downstream
// hard hallucination gates (mermaid bare-identifier check, finalizer
// coherence, enumeration labels). That vocabulary expansion was an
// explicit design sign-off (design doc §3.3): route names are real
// registered code facts, and these two pins are the conditions the
// sign-off rests on.

func routePinGraph(t *testing.T) *Graph {
	t.Helper()
	files := []*rmtypes.FileInfo{
		{
			RelPath:  "api/routes.go",
			Language: rmtypes.LangGo,
			Package:  "api",
			Symbols: []rmtypes.Symbol{
				{Name: "GET /users/:id", Kind: "route", File: "api/routes.go", Line: 7, EndLine: 7, Exported: true, Signature: "GET /users/:id -> GetUser"},
				{Name: "GetUser", Kind: "function", File: "api/routes.go", Line: 12, EndLine: 20, Exported: true},
			},
		},
	}
	return index.BuildGraph("/repo", files)
}

// Pin 1: a route symbol makes SymbolExists true for its exact name —
// the oracle reflects what the graph registered.
func TestRouteSymbolEntersOracleVocabulary(t *testing.T) {
	g := routePinGraph(t)
	if g.SymbolDefs["GET /users/:id"] == nil {
		t.Fatalf("route symbol not indexed into SymbolDefs")
	}
}

// Pin 2: route names (space/slash/colon shaped) must NOT create new
// identifier-shaped flat-index hits — a model writing `getusersid` or
// `get_users` in a diagram must not be legitimized by a route symbol.
func TestRouteSymbolDoesNotPerturbIdentifierShapedFlatLookups(t *testing.T) {
	g := routePinGraph(t)
	oracle := NewSymbolOracle(g)
	for _, probe := range []string{"getusersid", "get_users", "GETusersid"} {
		if ok, _ := oracle.SymbolExistsFlat(probe); ok {
			t.Errorf("flat lookup %q unexpectedly matched a route symbol — identifier-shaped oracle vocabulary must not absorb route names", probe)
		}
	}
	// The handler itself stays resolvable — routes add vocabulary
	// without displacing function symbols.
	if ok, _ := oracle.SymbolExistsFlat("GetUser"); !ok {
		t.Errorf("handler symbol lost from flat index")
	}
}
