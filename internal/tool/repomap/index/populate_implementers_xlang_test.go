package index

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// xlangCase is a single per-language fixture for the back-fill
// sanity tests. The extractor is invoked, the symbols are wrapped
// in a Graph, populateImplementers is called, and we assert that
// (a) the interface/trait/protocol symbol gained a non-empty
// RequiredMethods slice via the Parent-link back-fill and (b) the
// concrete type appears in ImplementersOf(ifaceName).
type xlangCase struct {
	name      string
	lang      string
	file      string
	src       string
	extract   func(root *sitter.Node, src []byte, file string) (string, []types.Symbol, []types.Import, []types.Relation)
	ifaceName string
	wantImpl  string
	wantSig   string // expected name(arity) entry in RequiredMethods
}

func runXLangCase(t *testing.T, tc xlangCase) {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(types.GetSitterLanguage(tc.lang))
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(tc.src))
	if err != nil || tree == nil {
		t.Skipf("tree-sitter for %s not available: %v", tc.lang, err)
	}
	pkg, syms, imps, rels := tc.extract(tree.RootNode(), []byte(tc.src), tc.file)
	_ = pkg
	_ = imps
	_ = rels
	fi := &types.FileInfo{
		RelPath:  tc.file,
		Language: tc.lang,
		Symbols:  syms,
	}
	g := &types.Graph{
		Root:       ".",
		Files:      []*types.FileInfo{fi},
		FileIndex:  map[string]*types.FileInfo{tc.file: fi},
		SymbolDefs: make(map[string][]*types.Symbol),
		SymbolByID: make(map[types.SymbolID]*types.Symbol),
	}
	for i := range fi.Symbols {
		s := &fi.Symbols[i]
		s.ID = types.DeriveSymbolID(fi, s)
		g.SymbolDefs[s.Name] = append(g.SymbolDefs[s.Name], s)
		g.SymbolByID[s.ID] = s
	}
	populateImplementers(g)

	var iface *types.Symbol
	for i := range fi.Symbols {
		if fi.Symbols[i].Name == tc.ifaceName {
			switch fi.Symbols[i].Kind {
			case "interface", "trait", "protocol":
				iface = &fi.Symbols[i]
			}
		}
	}
	if iface == nil {
		var dump []string
		for _, s := range fi.Symbols {
			dump = append(dump, s.Name+":"+s.Kind)
		}
		t.Fatalf("%s: interface/trait/protocol symbol %q not found; saw %v", tc.lang, tc.ifaceName, dump)
	}
	if len(iface.RequiredMethods) == 0 {
		t.Fatalf("%s: %s.RequiredMethods empty after back-fill; want %s", tc.lang, tc.ifaceName, tc.wantSig)
	}
	found := false
	for _, m := range iface.RequiredMethods {
		if m == tc.wantSig {
			found = true
		}
	}
	if !found {
		t.Errorf("%s: %s.RequiredMethods missing %q; got %v", tc.lang, tc.ifaceName, tc.wantSig, iface.RequiredMethods)
	}

	implementers := g.ImplementersOf(tc.ifaceName)
	gotImpl := false
	var implNames []string
	for _, id := range implementers {
		if sym, ok := g.SymbolByID[id]; ok {
			implNames = append(implNames, sym.Name)
			if sym.Name == tc.wantImpl {
				gotImpl = true
			}
		}
	}
	if !gotImpl {
		t.Errorf("%s: expected %q in ImplementersOf(%s); got %v", tc.lang, tc.wantImpl, tc.ifaceName, implNames)
	}
}

func TestPopulateImplementers_XLang_Java(t *testing.T) {
	runXLangCase(t, xlangCase{
		name:      "java",
		lang:      types.LangJava,
		file:      "Foo.java",
		extract:   extractJava,
		ifaceName: "Looper",
		wantImpl:  "MyLooper",
		wantSig:   "observe(1)",
		src: `package x;

public interface Looper {
    boolean observe(String s);
}

public class MyLooper implements Looper {
    public boolean observe(String s) { return true; }
}
`,
	})
}

func TestPopulateImplementers_XLang_Rust(t *testing.T) {
	runXLangCase(t, xlangCase{
		name:      "rust",
		lang:      types.LangRust,
		file:      "lib.rs",
		extract:   extractRust,
		ifaceName: "Looper",
		wantImpl:  "MyLooper",
		wantSig:   "observe(1)",
		src: `pub trait Looper {
    fn observe(&self, s: &str) -> bool;
}

pub struct MyLooper;

impl Looper for MyLooper {
    fn observe(&self, s: &str) -> bool { true }
}
`,
	})
}

func TestPopulateImplementers_XLang_Swift(t *testing.T) {
	runXLangCase(t, xlangCase{
		name:      "swift",
		lang:      types.LangSwift,
		file:      "Foo.swift",
		extract:   extractSwift,
		ifaceName: "Looper",
		wantImpl:  "MyLooper",
		wantSig:   "observe(1)",
		src: `protocol Looper {
    func observe(_ s: String) -> Bool
}

class MyLooper: Looper {
    func observe(_ s: String) -> Bool { return true }
}
`,
	})
}

func TestPopulateImplementers_XLang_Kotlin(t *testing.T) {
	runXLangCase(t, xlangCase{
		name:      "kotlin",
		lang:      types.LangKotlin,
		file:      "Foo.kt",
		extract:   extractKotlin,
		ifaceName: "Looper",
		wantImpl:  "MyLooper",
		wantSig:   "observe(1)",
		src: `interface Looper {
    fun observe(s: String): Boolean
}

class MyLooper : Looper {
    override fun observe(s: String): Boolean = true
}
`,
	})
}

func TestPopulateImplementers_XLang_Cangjie(t *testing.T) {
	src := `package x

interface Looper {
    func observe(s: String): Bool
}

class MyLooper <: Looper {
    public override func observe(s: String): Bool {
        return true
    }
}
`
	_, syms, _, _ := parseCangjie([]byte(src), "x.cj")
	fi := &types.FileInfo{
		RelPath:  "x.cj",
		Language: types.LangCangjie,
		Symbols:  syms,
	}
	g := &types.Graph{
		Root:       ".",
		Files:      []*types.FileInfo{fi},
		FileIndex:  map[string]*types.FileInfo{"x.cj": fi},
		SymbolDefs: make(map[string][]*types.Symbol),
		SymbolByID: make(map[types.SymbolID]*types.Symbol),
	}
	for i := range fi.Symbols {
		s := &fi.Symbols[i]
		s.ID = types.DeriveSymbolID(fi, s)
		g.SymbolDefs[s.Name] = append(g.SymbolDefs[s.Name], s)
		g.SymbolByID[s.ID] = s
	}
	populateImplementers(g)

	var iface *types.Symbol
	for i := range fi.Symbols {
		if fi.Symbols[i].Name == "Looper" && fi.Symbols[i].Kind == "interface" {
			iface = &fi.Symbols[i]
		}
	}
	if iface == nil {
		var dump []string
		for _, s := range fi.Symbols {
			dump = append(dump, s.Name+":"+s.Kind)
		}
		t.Fatalf("cangjie: Looper interface symbol not found; saw %v", dump)
	}
	if len(iface.RequiredMethods) == 0 {
		t.Fatalf("cangjie: Looper.RequiredMethods empty after back-fill")
	}
	t.Logf("cangjie Looper.RequiredMethods=%v", iface.RequiredMethods)
	implementers := g.ImplementersOf("Looper")
	gotMy := false
	for _, id := range implementers {
		if sym, ok := g.SymbolByID[id]; ok && sym.Name == "MyLooper" {
			gotMy = true
		}
	}
	if !gotMy {
		t.Errorf("cangjie: expected MyLooper in ImplementersOf(Looper); got %d implementers", len(implementers))
	}
}

func TestPopulateImplementers_XLang_TypeScript(t *testing.T) {
	runXLangCase(t, xlangCase{
		name: "typescript",
		lang: types.LangTypeScript,
		file: "foo.ts",
		extract: func(root *sitter.Node, src []byte, file string) (string, []types.Symbol, []types.Import, []types.Relation) {
			return extractJS(root, src, file, true)
		},
		ifaceName: "Looper",
		wantImpl:  "MyLooper",
		wantSig:   "observe(1)",
		src: `interface Looper {
    observe(s: string): boolean;
}

class MyLooper implements Looper {
    observe(s: string): boolean { return true; }
}
`,
	})
}
