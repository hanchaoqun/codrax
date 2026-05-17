package retrieve

import (
	"fmt"
	"sync"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestDetectEntrypoints_ExtendedLanguages(t *testing.T) {
	graph := &repotypes.Graph{
		Files: []*repotypes.FileInfo{
			{
				RelPath:  "app/src/main/kotlin/com/acme/Main.kt",
				Language: repotypes.LangKotlin,
				Symbols: []repotypes.Symbol{
					{Name: "main", Kind: "function", Exported: true},
				},
			},
			{
				RelPath:  "Sources/App/main.swift",
				Language: repotypes.LangSwift,
				Symbols: []repotypes.Symbol{
					{Name: "main", Kind: "function", Exported: true},
				},
			},
			{
				RelPath:  "bin/serve.rb",
				Language: repotypes.LangRuby,
				Symbols: []repotypes.Symbol{
					{Name: "Serve", Kind: "class", Exported: true},
				},
			},
			{
				RelPath:  "lua/main.lua",
				Language: repotypes.LangLua,
				Symbols: []repotypes.Symbol{
					{Name: "bootstrap", Kind: "function", Exported: true},
				},
			},
		},
		ReverseImports: map[string][]string{},
	}

	got := detectEntrypoints(graph)
	for _, path := range []string{
		"app/src/main/kotlin/com/acme/Main.kt",
		"Sources/App/main.swift",
		"bin/serve.rb",
		"lua/main.lua",
	} {
		if !got[path] {
			t.Fatalf("detectEntrypoints missing %q", path)
		}
	}
}

func TestRankGraphScores_DoesNotMutateSharedGraphScores(t *testing.T) {
	graph := testRankingGraph(20)
	graph.Scores = map[string]float64{"sentinel.go": 42}
	graph.QueryScores = map[string]float64{"sentinel.go": 7}

	ranking := RankGraphScores(graph, "Handler7")
	if ranking.Scores["pkg/handler_07.go"] <= 0 {
		t.Fatalf("query ranking did not score target file: %+v", ranking.Scores)
	}
	if got := graph.Scores["sentinel.go"]; got != 42 {
		t.Fatalf("RankGraphScores mutated Graph.Scores, got %v", got)
	}
	if got := graph.QueryScores["sentinel.go"]; got != 7 {
		t.Fatalf("RankGraphScores mutated Graph.QueryScores, got %v", got)
	}
}

func TestRankGraph_ConcurrentSameGraphDoesNotCrash(t *testing.T) {
	graph := testRankingGraph(80)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 32; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 20; j++ {
				RankGraph(graph, fmt.Sprintf("Handler%d", (i+j)%80))
				if files := TopFiles(graph, 5); len(files) == 0 {
					t.Errorf("TopFiles returned no files")
				}
			}
		}()
	}
	close(start)
	wg.Wait()
}

func testRankingGraph(n int) *repotypes.Graph {
	files := make([]*repotypes.FileInfo, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("Handler%d", i)
		files = append(files, &repotypes.FileInfo{
			RelPath:  fmt.Sprintf("pkg/handler_%02d.go", i),
			Language: repotypes.LangGo,
			Package:  "pkg",
			Symbols: []repotypes.Symbol{{
				Name:     name,
				Kind:     "function",
				Exported: true,
			}},
		})
	}
	g := &repotypes.Graph{
		Root:           "",
		Files:          files,
		FileIndex:      make(map[string]*repotypes.FileInfo, n),
		SymbolDefs:     make(map[string][]*repotypes.Symbol),
		SymbolByID:     make(map[repotypes.SymbolID]*repotypes.Symbol),
		MethodIndex:    make(map[repotypes.MethodKey]*repotypes.Symbol),
		ImportGraph:    make(map[string][]string),
		ReverseImports: make(map[string][]string),
	}
	for _, fi := range files {
		g.FileIndex[fi.RelPath] = fi
	}
	g.RankIndex = repotypes.BuildRankIndex(g)
	return g
}
