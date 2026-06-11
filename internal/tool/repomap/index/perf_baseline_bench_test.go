package index

import (
	"fmt"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// Baseline benchmarks for the constant-factor backlog items in
// docs/design/polyglot_eval_and_perf_remediation_20260611.md §3 批 5.
// Optimization without a material baseline delta is forbidden.

func BenchmarkStripTypeWrappers(b *testing.B) {
	inputs := []string{
		"Foo",
		"*Foo",
		"**[]*Foo?",
		"Optional<List<Map<String, Foo>>>",
		"std::vector<std::unique_ptr<Foo>>&&",
		"Promise<Array<Foo | null>>",
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		stripTypeWrappers(inputs[i%len(inputs)])
	}
}

func buildImplementerGraph(nTypes, nIfaces, nMethods int) *types.Graph {
	fi := &types.FileInfo{RelPath: "bench/types.java", Language: "java", ParseTier: 1}
	for i := 0; i < nIfaces; i++ {
		ifaceName := fmt.Sprintf("Iface%d", i)
		fi.Symbols = append(fi.Symbols, types.Symbol{Name: ifaceName, Kind: "interface", File: fi.RelPath})
		for m := 0; m < nMethods; m++ {
			fi.Symbols = append(fi.Symbols, types.Symbol{Name: fmt.Sprintf("ifm%d_%d", i, m), Kind: "method", Parent: ifaceName, File: fi.RelPath})
		}
	}
	for t := 0; t < nTypes; t++ {
		typeName := fmt.Sprintf("Type%d", t)
		fi.Symbols = append(fi.Symbols, types.Symbol{Name: typeName, Kind: "class", File: fi.RelPath})
		// each type implements one interface's contract
		iface := t % nIfaces
		for m := 0; m < nMethods; m++ {
			fi.Symbols = append(fi.Symbols, types.Symbol{Name: fmt.Sprintf("ifm%d_%d", iface, m), Kind: "method", Receiver: typeName, Parent: typeName, File: fi.RelPath})
		}
	}
	g := &types.Graph{
		Root:       ".",
		Files:      []*types.FileInfo{fi},
		FileIndex:  map[string]*types.FileInfo{fi.RelPath: fi},
		SymbolDefs: make(map[string][]*types.Symbol),
		SymbolByID: make(map[types.SymbolID]*types.Symbol),
	}
	for i := range fi.Symbols {
		sym := &fi.Symbols[i]
		sym.ID = types.DeriveSymbolID(fi, sym)
		g.SymbolDefs[sym.Name] = append(g.SymbolDefs[sym.Name], sym)
		g.SymbolByID[sym.ID] = sym
	}
	return g
}

func BenchmarkPopulateImplementers_200T_50I(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		g := buildImplementerGraph(200, 50, 8)
		b.StartTimer()
		populateImplementers(g)
	}
}

func BenchmarkPopulateImplementers_2000T_500I(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		g := buildImplementerGraph(2000, 500, 8)
		b.StartTimer()
		populateImplementers(g)
	}
}
