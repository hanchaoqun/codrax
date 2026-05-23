package relation

import (
	"testing"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTypedRelationCandidates_ExactFileImportsAndExports(t *testing.T) {
	g := importGraphFixture(rmtypes.LangGo)

	imports := TypedRelationCandidates(g, types.TypedRelationQuery{
		Kinds:   []types.TypedRelationKind{types.TypedRelationImports},
		Sources: []string{"cmd/root.go"},
		Purpose: types.TypedRelationPurposeCoverageGate,
	})
	if len(imports) != 1 {
		t.Fatalf("imports candidates = %+v, want one", imports)
	}
	got := imports[0]
	if got.Relation != types.TypedRelationImports ||
		got.SourceName != "cmd/root.go" ||
		got.SourceKind != "file" ||
		got.SourceFile != "cmd/root.go" ||
		got.Member.Name != "internal/tool/tool.go" ||
		got.Member.File != "internal/tool/tool.go" ||
		got.Precision != types.TypedRelationPrecisionExactFile {
		t.Fatalf("unexpected import candidate: %+v", got)
	}

	exports := TypedRelationCandidates(g, types.TypedRelationQuery{
		Kinds:   []types.TypedRelationKind{types.TypedRelationExports},
		Sources: []string{"internal/tool/tool.go"},
		Purpose: types.TypedRelationPurposeCoverageGate,
	})
	if len(exports) != 1 {
		t.Fatalf("exports candidates = %+v, want one", exports)
	}
	got = exports[0]
	if got.Relation != types.TypedRelationExports ||
		got.SourceName != "internal/tool/tool.go" ||
		got.SourceKind != "file" ||
		got.SourceFile != "cmd/root.go" ||
		got.Member.Name != "cmd/root.go" ||
		got.Member.File != "cmd/root.go" ||
		got.Precision != types.TypedRelationPrecisionExactFile {
		t.Fatalf("unexpected export candidate: %+v", got)
	}
}

func TestTypedRelationCandidates_DirectoryAndPackageArePromptOnly(t *testing.T) {
	g := importGraphFixture(rmtypes.LangJava)
	g.FileIndex["cmd/root.go"].Package = "com.example.app"
	g.FileIndex["cmd/extra.go"] = &rmtypes.FileInfo{RelPath: "cmd/extra.go", Language: rmtypes.LangJava, Package: "com.example.app"}
	g.Files = append(g.Files, g.FileIndex["cmd/extra.go"])
	g.ImportGraph["cmd/extra.go"] = []string{"internal/tool/tool.go"}
	g.ReverseImports["internal/tool/tool.go"] = append(g.ReverseImports["internal/tool/tool.go"], "cmd/extra.go")

	dirRows := TypedRelationCandidates(g, types.TypedRelationQuery{
		Kinds:      []types.TypedRelationKind{types.TypedRelationImports},
		Sources:    []string{"cmd"},
		Purpose:    types.TypedRelationPurposePromptHint,
		MaxMembers: 10,
	})
	if len(dirRows) != 1 {
		t.Fatalf("directory prompt candidates = %+v, want one de-duplicated import target", dirRows)
	}
	for _, row := range dirRows {
		if row.SourceName != "cmd" || row.SourceKind != "directory" || row.Precision != types.TypedRelationPrecisionNameOnly {
			t.Fatalf("directory rows must stay prompt-only/name-only, got %+v", row)
		}
	}

	coverageRows := TypedRelationCandidates(g, types.TypedRelationQuery{
		Kinds:   []types.TypedRelationKind{types.TypedRelationImports},
		Sources: []string{"cmd"},
		Purpose: types.TypedRelationPurposeCoverageGate,
	})
	if len(coverageRows) != 0 {
		t.Fatalf("directory scope must not become hard-gate candidates, got %+v", coverageRows)
	}

	pkgRows := TypedRelationCandidates(g, types.TypedRelationQuery{
		Kinds:      []types.TypedRelationKind{types.TypedRelationImports},
		Sources:    []string{"com.example.app"},
		Purpose:    types.TypedRelationPurposePromptHint,
		MaxMembers: 10,
	})
	if len(pkgRows) != 1 {
		t.Fatalf("package prompt candidates = %+v, want one de-duplicated import target", pkgRows)
	}
	for _, row := range pkgRows {
		if row.SourceName != "com.example.app" || row.SourceKind != "package" || row.Precision != types.TypedRelationPrecisionNameOnly {
			t.Fatalf("package rows must stay prompt-only/name-only, got %+v", row)
		}
	}
}

func TestTypedRelationCandidates_ImportsAreLanguageNeutral(t *testing.T) {
	for _, lang := range rmtypes.SupportedReadLanguages() {
		t.Run(lang, func(t *testing.T) {
			rows := TypedRelationCandidates(importGraphFixture(lang), types.TypedRelationQuery{
				Kinds:   []types.TypedRelationKind{types.TypedRelationImports},
				Sources: []string{"cmd/root.go"},
				Purpose: types.TypedRelationPurposeCoverageGate,
			})
			if len(rows) != 1 {
				t.Fatalf("language %s import rows = %+v, want one", lang, rows)
			}
			if rows[0].Member.File != "internal/tool/tool.go" || rows[0].Precision != types.TypedRelationPrecisionExactFile {
				t.Fatalf("language %s import row lost exact file semantics: %+v", lang, rows[0])
			}
		})
	}
}

func importGraphFixture(lang string) *rmtypes.Graph {
	root := &rmtypes.FileInfo{
		RelPath:  "cmd/root.go",
		Language: lang,
		Symbols:  []rmtypes.Symbol{{Name: "Run", Kind: "function", File: "cmd/root.go", Line: 7}},
	}
	dep := &rmtypes.FileInfo{
		RelPath:  "internal/tool/tool.go",
		Language: lang,
		Symbols:  []rmtypes.Symbol{{Name: "Tool", Kind: "type", File: "internal/tool/tool.go", Line: 11}},
	}
	return &rmtypes.Graph{
		Files:          []*rmtypes.FileInfo{root, dep},
		FileIndex:      map[string]*rmtypes.FileInfo{root.RelPath: root, dep.RelPath: dep},
		ImportGraph:    map[string][]string{root.RelPath: []string{dep.RelPath}},
		ReverseImports: map[string][]string{dep.RelPath: []string{root.RelPath}},
		SymbolDefs:     map[string][]*rmtypes.Symbol{},
		SymbolByID:     map[rmtypes.SymbolID]*rmtypes.Symbol{},
	}
}
