package retrieve

import (
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
