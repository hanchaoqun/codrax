package dataflow

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The candidate budget (MaxFiles) is spent on the files the caller named
// first; graph neighbours reached through imports, reverse imports and
// relation targets only fill what is left. The fixtures below make every
// neighbour sort (and, with a bias, rank) ahead of the named files and make
// the neighbours alone exceed the budget, which is the shape under which a
// single ordering over both groups evicts every file the caller named.

// candidateBudgetGraph builds a graph whose named files live under "z/" and
// whose expansion neighbours live under "a/", so that every neighbour sorts
// before every named file. Each named file reaches its neighbours through all
// three expansion edges: files it imports, files importing it, and the
// definitions of its relation targets.
func candidateBudgetGraph(named map[string][]string, neighboursPerEdge int, extraNeighbours map[string][]string) *repomap.Graph {
	g := &repomap.Graph{
		FileIndex:      make(map[string]*repomap.FileInfo),
		SymbolDefs:     make(map[string][]*repomap.Symbol),
		ImportGraph:    make(map[string][]string),
		ReverseImports: make(map[string][]string),
	}
	addFile := func(path string, symbols []string) *repomap.FileInfo {
		fi := g.FileIndex[path]
		if fi == nil {
			fi = &repomap.FileInfo{RelPath: path, Language: repomap.LangGo}
			g.FileIndex[path] = fi
			g.Files = append(g.Files, fi)
		}
		for _, name := range symbols {
			fi.Symbols = append(fi.Symbols, repomap.Symbol{Name: name, File: path})
		}
		return fi
	}
	idx := 0
	next := func(kind string) string {
		idx++
		return fmt.Sprintf("a/%s_%03d.go", kind, idx)
	}
	for path, symbols := range named {
		fi := addFile(path, symbols)
		for i := 0; i < neighboursPerEdge; i++ {
			dep := next("dep")
			addFile(dep, nil)
			g.ImportGraph[path] = append(g.ImportGraph[path], dep)

			importer := next("importer")
			addFile(importer, nil)
			g.ReverseImports[path] = append(g.ReverseImports[path], importer)

			target := next("target")
			targetName := fmt.Sprintf("Target%03d", idx)
			addFile(target, []string{targetName})
			fi.Relations = append(fi.Relations, repomap.Relation{ToEP: repomap.RelationEndpoint{Name: targetName}})
		}
	}
	for path, symbols := range extraNeighbours {
		addFile(path, symbols)
		for namedPath := range named {
			g.ReverseImports[namedPath] = append(g.ReverseImports[namedPath], path)
		}
	}
	for _, fi := range g.Files {
		for i := range fi.Symbols {
			sym := &fi.Symbols[i]
			g.SymbolDefs[sym.Name] = append(g.SymbolDefs[sym.Name], sym)
		}
	}
	return g
}

func TestSelectCandidateFiles_NamedFilesSurviveExpansionThatSortsFirstWithoutBias(t *testing.T) {
	graph := candidateBudgetGraph(map[string][]string{
		"z/named_b.go": nil,
		"z/named_a.go": nil,
	}, 4, nil)
	full, _ := selectCandidateFiles(graph, Options{
		CandidateFiles: []string{"z/named_b.go", "z/named_a.go"},
		MaxFiles:       1 << 20,
	})
	if len(full) != 2+2*3*4 {
		t.Fatalf("fixture must expand through imports, importers and relation targets: got %d files %v", len(full), full)
	}

	got, truncated := selectCandidateFiles(graph, Options{
		// Named out of alphabetical order: within the named group the
		// order is alphabetical, as it is within the expansion group.
		CandidateFiles: []string{"z/named_b.go", "z/named_a.go"},
		MaxFiles:       5,
	})
	want := []string{
		"z/named_a.go",
		"z/named_b.go",
		"a/dep_001.go",
		"a/dep_004.go",
		"a/dep_007.go",
	}
	if !truncated {
		t.Fatalf("expansion exceeds the budget; truncated must be reported")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("named files must be kept ahead of expansion files, each group alphabetical:\n got=%v\nwant=%v", got, want)
	}
}

func TestSelectCandidateFiles_NamedFilesSurviveExpansionThatRanksFirstWithBias(t *testing.T) {
	// "z/omega.go" defines a symbol matching the bias (score 1); "z/alpha.go"
	// matches nothing (score 0). The neighbour "a/zz_dispatch_neighbour.go"
	// matches in its path (score 2), so a ranking over both groups would put
	// it first; within the expansion group it must still lead the unmatched
	// neighbours that sort before it.
	graph := candidateBudgetGraph(map[string][]string{
		"z/alpha.go": nil,
		"z/omega.go": {"DispatchWork"},
	}, 4, map[string][]string{
		"a/zz_dispatch_neighbour.go": nil,
	})
	got, truncated := selectCandidateFiles(graph, Options{
		CandidateFiles: []string{"z/alpha.go", "z/omega.go"},
		EntityBias:     []string{"dispatch"},
		MaxFiles:       5,
	})
	want := []string{
		"z/omega.go",
		"z/alpha.go",
		"a/zz_dispatch_neighbour.go",
		"a/dep_001.go",
		"a/dep_004.go",
	}
	if !truncated {
		t.Fatalf("expansion exceeds the budget; truncated must be reported")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("named files must be kept ahead of expansion files, each group ranked by the bias alone:\n got=%v\nwant=%v", got, want)
	}
}

func TestSelectCandidateFiles_MoreNamedFilesThanBudgetAreCutByTheirOwnOrder(t *testing.T) {
	named := map[string][]string{
		"z/n1.go": nil,
		"z/n2.go": nil,
		"z/n3.go": {"DispatchWork"},
		"z/n4.go": nil,
		"z/n5.go": {"DispatchLoop", "DispatchOnce"},
	}
	graph := candidateBudgetGraph(named, 2, map[string][]string{
		"a/dispatch_neighbour.go": {"Dispatch"},
	})
	callerOrder := []string{"z/n4.go", "z/n2.go", "z/n5.go", "z/n1.go", "z/n3.go"}

	t.Run("without_bias", func(t *testing.T) {
		got, truncated := selectCandidateFiles(graph, Options{
			CandidateFiles: callerOrder,
			MaxFiles:       3,
		})
		want := []string{"z/n1.go", "z/n2.go", "z/n3.go"}
		if !truncated || !reflect.DeepEqual(got, want) {
			t.Fatalf("named files beyond the budget are cut alphabetically and no neighbour enters:\n got=%v truncated=%v\nwant=%v", got, truncated, want)
		}
	})
	t.Run("with_bias", func(t *testing.T) {
		got, truncated := selectCandidateFiles(graph, Options{
			CandidateFiles: callerOrder,
			EntityBias:     []string{"dispatch"},
			MaxFiles:       3,
		})
		want := []string{"z/n5.go", "z/n3.go", "z/n1.go"}
		if !truncated || !reflect.DeepEqual(got, want) {
			t.Fatalf("named files beyond the budget are cut by the bias ranking and no neighbour enters:\n got=%v truncated=%v\nwant=%v", got, truncated, want)
		}
	})
}

func TestSelectCandidateFiles_NamedNeighbourStaysInTheNamedGroup(t *testing.T) {
	// A named file that is also a graph neighbour of another named file is a
	// named file: it keeps its place ahead of the expansion.
	graph := candidateBudgetGraph(map[string][]string{
		"z/caller.go": nil,
		"z/callee.go": nil,
	}, 3, nil)
	graph.ImportGraph["z/caller.go"] = append([]string{"z/callee.go"}, graph.ImportGraph["z/caller.go"]...)
	got, _ := selectCandidateFiles(graph, Options{
		CandidateFiles: []string{"z/caller.go", "z/callee.go"},
		MaxFiles:       3,
	})
	want := []string{"z/callee.go", "z/caller.go", "a/dep_001.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
}

func TestSelectCandidateFiles_ScopeStillFiltersNamedAndExpansionFiles(t *testing.T) {
	graph := candidateBudgetGraph(map[string][]string{
		"z/in_scope.go": nil,
	}, 3, nil)
	graph.FileIndex["y/out_of_scope.go"] = &repomap.FileInfo{RelPath: "y/out_of_scope.go", Language: repomap.LangGo}
	got, truncated := selectCandidateFiles(graph, Options{
		CandidateFiles: []string{"y/out_of_scope.go", "z/in_scope.go"},
		Scope:          []string{"z/"},
		MaxFiles:       5,
	})
	want := []string{"z/in_scope.go"}
	if truncated || !reflect.DeepEqual(got, want) {
		t.Fatalf("scope must drop out-of-scope named files and neighbours as before: got=%v truncated=%v want=%v", got, truncated, want)
	}
}

func TestAnalyze_NamedFileEvidenceSurvivesNeighboursThatFillTheBudget(t *testing.T) {
	const named = "z/service.go"
	files := []testFile{{
		relPath: named,
		lang:    repomap.LangGo,
		content: "package z\n\nfunc Port() string {\n\treturn \"8080\"\n}\n",
		symbols: []repomap.Symbol{{Name: "Port", Kind: "function", Line: 3, EndLine: 5, Exported: true}},
	}}
	var importers []string
	for i := 0; i < 6; i++ {
		rel := fmt.Sprintf("a/importer_%02d.go", i)
		importers = append(importers, rel)
		files = append(files, testFile{
			relPath: rel,
			lang:    repomap.LangGo,
			content: "package a\n\nfunc F() string {\n\treturn \"v\"\n}\n",
			symbols: []repomap.Symbol{{Name: "F", Kind: "function", Line: 3, EndLine: 5}},
		})
	}
	// With the bias, every importer matches in its path (score 2) and the
	// named file only in a symbol name (score 1), so a ranking over both
	// groups puts all importers first; "port" also keeps the named file's
	// symbol through the per-symbol relevance gate.
	for _, bias := range [][]string{nil, {"importer", "port"}} {
		t.Run(fmt.Sprintf("bias=%v", bias), func(t *testing.T) {
			root, graph := setupTestRepo(t, files)
			graph.ReverseImports[named] = importers
			result := Analyze(graph, Options{
				RepoRoot:       root,
				CandidateFiles: []string{named},
				EntityBias:     bias,
				MaxFiles:       3,
				WorkDir:        t.TempDir(),
				SkipFindings:   true,
			})
			var namedEvidence, truncatedEvidence int
			var sources []string
			for _, ev := range result.Evidence {
				sources = append(sources, ev.Source)
				if ev.Source == named {
					namedEvidence++
				}
				if ev.Kind == types.EvidenceTruncated {
					truncatedEvidence++
				}
			}
			if namedEvidence == 0 {
				t.Fatalf("the named file must be analyzed when its importers alone exceed the budget; evidence sources=%q", sources)
			}
			if truncatedEvidence != 1 {
				t.Fatalf("the truncation must still be reported once, got %d", truncatedEvidence)
			}
		})
	}
}
