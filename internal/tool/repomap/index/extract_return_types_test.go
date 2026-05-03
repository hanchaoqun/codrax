package index

import (
	"context"
	"reflect"
	"sort"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestBackfillReturnTypeNames_MultiLanguage exercises the parser-
// level post-pass across the languages that supply tree-sitter
// grammar. Phase 6 stage 21 (2026-05-03): proves that
// Symbol.ReturnTypeNames flows from AST extraction for every
// covered language without per-extractor wiring (other than the
// generic isFunctionNodeKind switch + extractReturnTypeNames
// walker).
//
// Each fixture is a tiny snippet declaring a function that
// returns a custom type. The expectation is that after running
// the language extractor + backfillReturnTypeNames, the function
// symbol carries the type name in ReturnTypeNames.
//
// Languages with hand-written parsers (Cangjie) are exercised in
// their dedicated test files since they do not flow through
// tree-sitter.
func TestBackfillReturnTypeNames_MultiLanguage(t *testing.T) {
	cases := []struct {
		lang   string
		source string
		fn     string
		expect string
	}{
		{
			lang: types.LangGo,
			source: `package x
type Foo struct{}
func NewFoo() *Foo { return &Foo{} }
`,
			fn:     "NewFoo",
			expect: "Foo",
		},
		{
			lang: types.LangPython,
			source: `class Foo: ...
def make_foo() -> Foo:
    return Foo()
`,
			fn:     "make_foo",
			expect: "Foo",
		},
		{
			lang: types.LangRust,
			source: `pub struct Foo;
pub fn make_foo() -> Foo { Foo }
`,
			fn:     "make_foo",
			expect: "Foo",
		},
		{
			lang: types.LangJava,
			source: `public class Builder {
    public Foo buildFoo() { return new Foo(); }
}
`,
			fn:     "buildFoo",
			expect: "Foo",
		},
	}
	for _, c := range cases {
		t.Run(c.lang, func(t *testing.T) {
			syms, root, src, ok := parseLanguageForTest(t, c.lang, c.source)
			if !ok {
				t.Skipf("language %s: tree-sitter parse not available in this build", c.lang)
				return
			}
			backfillReturnTypeNames(root, src, syms)
			var sym *types.Symbol
			for i := range syms {
				if syms[i].Name == c.fn {
					sym = &syms[i]
					break
				}
			}
			if sym == nil {
				t.Fatalf("symbol %q not found in extracted symbols (%d total)", c.fn, len(syms))
			}
			got := append([]string(nil), sym.ReturnTypeNames...)
			sort.Strings(got)
			found := false
			for _, g := range got {
				if g == c.expect {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ReturnTypeNames missing %q for %s.%s; got %v",
					c.expect, c.lang, c.fn, got)
			}
		})
	}
}

// parseLanguageForTest parses `source` with the language's
// tree-sitter grammar and runs the per-language extractor,
// returning the resulting symbol slice + AST root for the post-
// pass to consume.
func parseLanguageForTest(t *testing.T, lang, source string) ([]types.Symbol, *sitter.Node, []byte, bool) {
	t.Helper()
	sl := types.GetSitterLanguage(lang)
	if sl == nil {
		return nil, nil, nil, false
	}
	parser := sitter.NewParser()
	parser.SetLanguage(sl)
	src := []byte(source)
	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil || tree == nil {
		return nil, nil, nil, false
	}
	root := tree.RootNode()
	if root == nil {
		return nil, nil, nil, false
	}
	var syms []types.Symbol
	switch lang {
	case types.LangGo:
		_, syms, _, _ = extractGo(root, src, "test.go")
	case types.LangPython:
		_, syms, _, _ = extractPython(root, src, "test.py")
	case types.LangRust:
		_, syms, _, _ = extractRust(root, src, "test.rs")
	case types.LangJava:
		_, syms, _, _ = extractJava(root, src, "Test.java")
	default:
		return nil, nil, nil, false
	}
	return syms, root, src, true
}

// TestBackfillReturnTypeNames_NoFunctionReturnsNil verifies the
// post-pass handles files without function declarations
// gracefully (nil ReturnTypeNames on every symbol).
func TestBackfillReturnTypeNames_NoFunctionReturnsNil(t *testing.T) {
	syms := []types.Symbol{
		{Name: "X", Kind: "type", Line: 1},
	}
	// Nil root → safe early return; symbols unchanged.
	backfillReturnTypeNames(nil, nil, syms)
	if got := syms[0].ReturnTypeNames; !reflect.DeepEqual(got, []string(nil)) {
		t.Errorf("expected nil ReturnTypeNames; got %v", got)
	}
}
