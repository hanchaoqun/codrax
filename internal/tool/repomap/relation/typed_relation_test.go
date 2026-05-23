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

func TestTypedRelationCandidates_CalledByExactFunction(t *testing.T) {
	rows := TypedRelationCandidates(callGraphFixture(rmtypes.LangGo), types.TypedRelationQuery{
		Kinds:   []types.TypedRelationKind{types.TypedRelationCalledBy},
		Sources: []string{"Target"},
		Purpose: types.TypedRelationPurposeCoverageGate,
	})
	if len(rows) != 1 {
		t.Fatalf("called-by candidates = %+v, want one", rows)
	}
	got := rows[0]
	if got.Relation != types.TypedRelationCalledBy ||
		got.SourceName != "Target" ||
		got.SourceKind != "function" ||
		got.SourceFile != "lib/target.go" ||
		got.SourceLine != 7 ||
		got.Member.Name != "Run" ||
		got.Member.File != "cmd/main.go" ||
		got.Member.Line != 22 ||
		got.Precision != types.TypedRelationPrecisionExactSymbolID {
		t.Fatalf("unexpected called-by candidate: %+v", got)
	}
}

func TestTypedRelationCandidates_CalledByDisambiguatesReceiver(t *testing.T) {
	rows := TypedRelationCandidates(receiverCallGraphFixture(rmtypes.LangGo), types.TypedRelationQuery{
		Kinds:   []types.TypedRelationKind{types.TypedRelationCalledBy},
		Sources: []string{"ToolA.Execute"},
		Purpose: types.TypedRelationPurposeCoverageGate,
	})
	if len(rows) != 1 {
		t.Fatalf("receiver-specific called-by candidates = %+v, want one", rows)
	}
	if rows[0].Member.File != "agent/caller_a.go" || rows[0].Member.Name != "UseA" {
		t.Fatalf("called-by should only include ToolA.Execute caller, got %+v", rows[0])
	}
}

func TestTypedRelationCandidates_CalledByAmbiguousNameIsPromptOnly(t *testing.T) {
	g := receiverCallGraphFixture(rmtypes.LangGo)
	promptRows := TypedRelationCandidates(g, types.TypedRelationQuery{
		Kinds:   []types.TypedRelationKind{types.TypedRelationCalledBy},
		Sources: []string{"Execute"},
		Purpose: types.TypedRelationPurposePromptHint,
	})
	if len(promptRows) != 2 {
		t.Fatalf("ambiguous prompt called-by candidates = %+v, want two soft rows", promptRows)
	}
	for _, row := range promptRows {
		if row.Precision != types.TypedRelationPrecisionNameOnly {
			t.Fatalf("ambiguous called-by prompt row must be name-only, got %+v", row)
		}
	}
	coverageRows := TypedRelationCandidates(g, types.TypedRelationQuery{
		Kinds:   []types.TypedRelationKind{types.TypedRelationCalledBy},
		Sources: []string{"Execute"},
		Purpose: types.TypedRelationPurposeCoverageGate,
	})
	if len(coverageRows) != 0 {
		t.Fatalf("ambiguous called-by rows must not feed coverage gate, got %+v", coverageRows)
	}
}

func TestTypedRelationCandidates_CalledByIsLanguageNeutral(t *testing.T) {
	for _, lang := range rmtypes.SupportedReadLanguages() {
		t.Run(lang, func(t *testing.T) {
			rows := TypedRelationCandidates(callGraphFixture(lang), types.TypedRelationQuery{
				Kinds:   []types.TypedRelationKind{types.TypedRelationCalledBy},
				Sources: []string{"Target"},
				Purpose: types.TypedRelationPurposeCoverageGate,
			})
			if len(rows) != 1 {
				t.Fatalf("language %s called-by rows = %+v, want one", lang, rows)
			}
			if rows[0].Member.File != "cmd/main.go" || rows[0].Precision != types.TypedRelationPrecisionExactSymbolID {
				t.Fatalf("language %s called-by row lost exact semantics: %+v", lang, rows[0])
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

func callGraphFixture(lang string) *rmtypes.Graph {
	target := &rmtypes.Symbol{Name: "Target", Kind: "function", File: "lib/target.go", Line: 7, Arity: 0}
	caller := rmtypes.Symbol{Name: "Run", Kind: "function", File: "cmd/main.go", Line: 20, EndLine: 24, Arity: 0}
	targetFile := &rmtypes.FileInfo{RelPath: "lib/target.go", Language: lang, Package: "main", Symbols: []rmtypes.Symbol{*target}}
	callerFile := &rmtypes.FileInfo{
		RelPath:  "cmd/main.go",
		Language: lang,
		Package:  "main",
		Symbols:  []rmtypes.Symbol{caller},
		Relations: []rmtypes.Relation{{
			Kind: "call",
			From: "cmd/main.go",
			To:   "Target",
			File: "cmd/main.go",
			Line: 22,
			ToEP: rmtypes.RelationEndpoint{Name: "Target", File: "cmd/main.go", Line: 22},
		}},
	}
	targetFile.Symbols[0].ID = rmtypes.DeriveSymbolID(targetFile, &targetFile.Symbols[0])
	callerFile.Symbols[0].ID = rmtypes.DeriveSymbolID(callerFile, &callerFile.Symbols[0])
	return &rmtypes.Graph{
		Files:      []*rmtypes.FileInfo{targetFile, callerFile},
		FileIndex:  map[string]*rmtypes.FileInfo{targetFile.RelPath: targetFile, callerFile.RelPath: callerFile},
		SymbolDefs: map[string][]*rmtypes.Symbol{"Target": {&targetFile.Symbols[0]}, "Run": {&callerFile.Symbols[0]}},
		SymbolByID: map[rmtypes.SymbolID]*rmtypes.Symbol{
			targetFile.Symbols[0].ID: &targetFile.Symbols[0],
			callerFile.Symbols[0].ID: &callerFile.Symbols[0],
		},
		MethodIndex: map[rmtypes.MethodKey]*rmtypes.Symbol{
			{Pkg: "main", Receiver: "", Name: "Target"}: &targetFile.Symbols[0],
			{Pkg: "main", Receiver: "", Name: "Run"}:    &callerFile.Symbols[0],
		},
		ImportGraph:    map[string][]string{},
		ReverseImports: map[string][]string{},
	}
}

func receiverCallGraphFixture(lang string) *rmtypes.Graph {
	targetFile := &rmtypes.FileInfo{RelPath: "agent/tools.go", Language: lang, Package: "agent"}
	targetFile.Symbols = []rmtypes.Symbol{
		{Name: "Execute", Kind: "method", File: "agent/tools.go", Line: 10, Receiver: "ToolA", Arity: 0},
		{Name: "Execute", Kind: "method", File: "agent/tools.go", Line: 20, Receiver: "ToolB", Arity: 0},
	}
	callerA := &rmtypes.FileInfo{
		RelPath:  "agent/caller_a.go",
		Language: lang,
		Package:  "agent",
		Symbols:  []rmtypes.Symbol{{Name: "UseA", Kind: "function", File: "agent/caller_a.go", Line: 5, EndLine: 8}},
		Relations: []rmtypes.Relation{{
			Kind: "call", From: "agent/caller_a.go", To: "Execute", File: "agent/caller_a.go", Line: 6,
			ToEP: rmtypes.RelationEndpoint{Name: "Execute", Receiver: "ToolA", File: "agent/caller_a.go", Line: 6},
		}},
	}
	callerB := &rmtypes.FileInfo{
		RelPath:  "agent/caller_b.go",
		Language: lang,
		Package:  "agent",
		Symbols:  []rmtypes.Symbol{{Name: "UseB", Kind: "function", File: "agent/caller_b.go", Line: 5, EndLine: 8}},
		Relations: []rmtypes.Relation{{
			Kind: "call", From: "agent/caller_b.go", To: "Execute", File: "agent/caller_b.go", Line: 6,
			ToEP: rmtypes.RelationEndpoint{Name: "Execute", Receiver: "ToolB", File: "agent/caller_b.go", Line: 6},
		}},
	}
	files := []*rmtypes.FileInfo{targetFile, callerA, callerB}
	symbolDefs := map[string][]*rmtypes.Symbol{}
	symbolByID := map[rmtypes.SymbolID]*rmtypes.Symbol{}
	methodIndex := map[rmtypes.MethodKey]*rmtypes.Symbol{}
	for _, fi := range files {
		for i := range fi.Symbols {
			fi.Symbols[i].ID = rmtypes.DeriveSymbolID(fi, &fi.Symbols[i])
			sym := &fi.Symbols[i]
			symbolDefs[sym.Name] = append(symbolDefs[sym.Name], sym)
			symbolByID[sym.ID] = sym
			receiver := sym.Receiver
			if receiver == "" {
				receiver = sym.Parent
			}
			methodIndex[rmtypes.MethodKey{Pkg: fi.Package, Receiver: receiver, Name: sym.Name}] = sym
		}
	}
	return &rmtypes.Graph{
		Files: []*rmtypes.FileInfo{targetFile, callerA, callerB},
		FileIndex: map[string]*rmtypes.FileInfo{
			targetFile.RelPath: targetFile,
			callerA.RelPath:    callerA,
			callerB.RelPath:    callerB,
		},
		SymbolDefs:     symbolDefs,
		SymbolByID:     symbolByID,
		MethodIndex:    methodIndex,
		ImportGraph:    map[string][]string{},
		ReverseImports: map[string][]string{},
	}
}
