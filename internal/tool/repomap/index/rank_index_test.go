package index

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestBuildGraphPopulatesRankIndex(t *testing.T) {
	files := []*types.FileInfo{
		{
			RelPath:  "core/service.go",
			Language: types.LangGo,
			Package:  "core",
			Symbols: []types.Symbol{
				{Name: "Service", Kind: "type", Exported: true, File: "core/service.go", Line: 3},
				{Name: "Run", Kind: "method", Receiver: "Service", Exported: true, File: "core/service.go", Line: 8},
			},
		},
		{
			RelPath:  "core/other.go",
			Language: types.LangGo,
			Package:  "core",
			Symbols: []types.Symbol{
				{Name: "Run", Kind: "function", Exported: true, File: "core/other.go", Line: 4},
			},
		},
		{
			RelPath:  "cmd/a.go",
			Language: types.LangGo,
			Package:  "cmd",
			Relations: []types.Relation{
				{Kind: "call", To: "Run", File: "cmd/a.go", Line: 12,
					ToEP: types.RelationEndpoint{Name: "Run", Receiver: "Service", File: "cmd/a.go", Line: 12}},
				{Kind: "call", To: "Run", File: "cmd/a.go", Line: 13,
					ToEP: types.RelationEndpoint{Name: "Run", Receiver: "Service", File: "cmd/a.go", Line: 13}},
				{Kind: "type_usage", To: "Service", File: "cmd/a.go", Line: 14},
			},
		},
		{
			RelPath:  "cmd/b.go",
			Language: types.LangGo,
			Package:  "cmd",
			Relations: []types.Relation{
				{Kind: "call", To: "Run", File: "cmd/b.go", Line: 9,
					ToEP: types.RelationEndpoint{Name: "Run", Receiver: "Service", File: "cmd/b.go", Line: 9}},
				{Kind: "call", To: "Missing", File: "cmd/b.go", Line: 10,
					ToEP: types.RelationEndpoint{Name: "Missing", Receiver: "Service", File: "cmd/b.go", Line: 10}},
			},
		},
	}
	g := BuildGraph("/tmp/repo", files)
	idx := g.RankIndex
	if idx == nil {
		t.Fatal("BuildGraph did not populate RankIndex")
	}

	if got := idx.SymbolDefCount["Run"]; got != 2 {
		t.Fatalf("SymbolDefCount[Run] = %d, want 2 files", got)
	}
	runID := types.MakeSymbolID(types.LangGo, "core", "Service", "Run", 0)
	if got := idx.CallCountByID[runID]; got != 3 {
		t.Fatalf("CallCountByID[%s] = %d, want 3", runID, got)
	}
	if got := idx.CallCount["Missing"]; got != 1 {
		t.Fatalf("CallCount[Missing] = %d, want 1", got)
	}
	if got := idx.RefCount["Service"]; got != 1 {
		t.Fatalf("RefCount[Service] = %d, want 1", got)
	}
	if got := idx.FileFanout["core/service.go"]; got != 0.5 {
		t.Fatalf("FileFanout[core/service.go] = %v, want 0.5", got)
	}
	if !idx.Entrypoints["core/service.go"] {
		t.Fatal("RankIndex entrypoints missing exported no-importer file")
	}
}
