package index

import (
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// extract_generic_callee_test.go — colleague B1554 matrix
// (colleague_merge_audit §40.59), in-repo replacement for the colleague's
// out-of-tree `.codrax/tmp/b1554-matrix-isolated-20260902.txt`: every
// tree-sitter / lexer language that spells explicit type arguments on a
// call publishes the BARE callee name on the call row — the plain call, the
// generic call and the generic method-on-receiver call all resolve to the
// same endpoint spelling — and no instantiated spelling
// (`make_unique<ConsoleSink>`) ever reaches ToEP.Name. Languages without a
// call-site type-argument syntax (C, Python, JavaScript, Ruby, Lua) pin the
// plain arm only. Comparison chains are the negative: `lhs < rhs && cnt >
// (limit)` must never mint a call row headed by `lhs`.

type genericCalleeCase struct {
	lang    string
	file    string
	src     string
	extract func(t *testing.T, src []byte, file string) []types.Relation
	want    []string // callee names that must appear on call rows
	forbid  []string // instantiated / comparison-shaped names that must not
}

func genericCalleeNames(rels []types.Relation) map[string]int {
	out := make(map[string]int)
	for _, rel := range rels {
		if rel.Kind == "call" {
			out[rel.ToEP.Name]++
		}
	}
	return out
}

func genericCalleeMatrix() []genericCalleeCase {
	return []genericCalleeCase{
		{
			lang: types.LangRust, file: "svc.rs",
			src: "fn run(repo: &Repository, s: &str) { let a = parse::<u32>(s); let b = repo.load::<Item>(1); Repository::open::<Item>(); std::mem::size_of::<u8>(); helper(s); if a < 3 && b > (2) { helper(s) } }\n",
			extract: func(t *testing.T, src []byte, file string) []types.Relation {
				_, _, _, rels := extractRust(parseSourceFor(t, types.LangRust, string(src)), src, file)
				return rels
			},
			want:   []string{"parse", "load", "open", "size_of", "helper"},
			forbid: []string{"parse::<u32>", "load::<Item>", "a", "b"},
		},
		{
			lang: types.LangCpp, file: "svc.cpp",
			src: "void run(Repository& repo) { auto s = std::make_unique<ConsoleSink>(); auto t = make_unique<Sink>(1); repo.get<int>(); repo.template fetch<int>(); ns::convert<Item>(2); helper(); if (a < b && c > (d)) { helper(); } }\n",
			extract: func(t *testing.T, src []byte, file string) []types.Relation {
				_, _, _, rels := extractCCpp(parseSourceFor(t, types.LangCpp, string(src)), src, file, types.LangCpp)
				return rels
			},
			want:   []string{"make_unique", "get", "fetch", "convert", "helper"},
			forbid: []string{"make_unique<ConsoleSink>", "make_unique<Sink>", "get<int>", "fetch<int>", "convert<Item>", "a", "c"},
		},
		{
			lang: types.LangC, file: "svc.c",
			src: "void run(Repository* repo) { helper(repo); repo->load(1); if (a < b && c > (d)) { helper(repo); } }\n",
			extract: func(t *testing.T, src []byte, file string) []types.Relation {
				_, _, _, rels := extractCCpp(parseSourceFor(t, types.LangC, string(src)), src, file, types.LangC)
				return rels
			},
			want:   []string{"helper", "load"},
			forbid: []string{"a", "c"},
		},
		{
			lang: types.LangSwift, file: "Svc.swift",
			src: "class Svc { func run(x: Int) { let a = Wrapper<Int>(x); let b = Box(x); let c = decode(x); if a < 3 && b > (2) { helper() } } }\n",
			extract: func(t *testing.T, src []byte, file string) []types.Relation {
				_, _, _, rels := extractSwift(parseSourceFor(t, types.LangSwift, string(src)), src, file)
				return rels
			},
			want:   []string{"Wrapper", "Box", "decode", "helper"},
			forbid: []string{"Wrapper<Int>", "a", "b"},
		},
		{
			lang: types.LangKotlin, file: "Svc.kt",
			src: "class Svc { fun run(repo: Repository, x: Int) { val a = decode<Item>(x); val b = repo.load<Item>(x); val c = listOf<Int>(1); if (a < 3 && b > (2)) { helper() } } }\n",
			extract: func(t *testing.T, src []byte, file string) []types.Relation {
				_, _, _, rels := extractKotlin(parseSourceFor(t, types.LangKotlin, string(src)), src, file)
				return rels
			},
			want:   []string{"decode", "load", "listOf", "helper"},
			forbid: []string{"decode<Item>", "load<Item>", "a", "b"},
		},
		{
			lang: types.LangTypeScript, file: "svc.ts",
			src: "function run(repo: Repository, s: string) { const a = parse<number>(s); const b = repo.load<Item>(s); const c = new Box<string>(1); if (a < 3 && b > (2)) { helper(); } }\n",
			extract: func(t *testing.T, src []byte, file string) []types.Relation {
				_, _, _, rels := extractJS(parseSourceFor(t, types.LangTypeScript, string(src)), src, file, true)
				return rels
			},
			want:   []string{"parse", "load", "helper"},
			forbid: []string{"parse<number>", "load<Item>", "a", "b"},
		},
		{
			lang: types.LangJava, file: "Svc.java",
			src: "class Svc { void run(Repository repo, String s) { Object a = Util.<String>parse(s); Object b = repo.<Item>load(s); Object c = new Box<String>(1); if (x < 3 && y > (2)) { helper(); } } }\n",
			extract: func(t *testing.T, src []byte, file string) []types.Relation {
				_, _, _, rels := extractJava(parseSourceFor(t, types.LangJava, string(src)), src, file)
				return rels
			},
			want:   []string{"parse", "load", "helper"},
			forbid: []string{"<String>parse", "x", "y"},
		},
		{
			lang: types.LangCangjie, file: "svc.cj",
			src: "package demo\nfunc run(repo: Repository, s: String): Unit { let a = parse<Int64>(s); let b = repo.load<Item>(s); let c = build<Array<Int64>>(s); let d = wrap<(Int64)->Unit>(s); helper(s); if (a < b && c > (d)) { helper(s) } }\n",
			extract: func(t *testing.T, src []byte, file string) []types.Relation {
				_, _, _, rels, _ := extractCangjie(src, file)
				return rels
			},
			want:   []string{"parse", "load", "build", "wrap", "helper"},
			forbid: []string{"parse<Int64>", "a", "c"},
		},
		{
			lang: types.LangPython, file: "svc.py",
			src: "def run(repo, s):\n    a = parse(s)\n    b = repo.load(s)\n    if a < 3 and b > (2):\n        helper()\n",
			extract: func(t *testing.T, src []byte, file string) []types.Relation {
				_, _, _, rels := extractPython(parseSourceFor(t, types.LangPython, string(src)), src, file)
				return rels
			},
			want:   []string{"parse", "load", "helper"},
			forbid: []string{"a", "b"},
		},
	}
}

func TestGenericCalleeRowsPublishBareNamesAcrossLanguages(t *testing.T) {
	for _, tc := range genericCalleeMatrix() {
		t.Run(tc.lang, func(t *testing.T) {
			names := genericCalleeNames(tc.extract(t, []byte(tc.src), tc.file))
			for _, want := range tc.want {
				if names[want] == 0 {
					t.Errorf("%s: no call row named %q; rows=%v", tc.lang, want, names)
				}
			}
			for _, forbid := range tc.forbid {
				if names[forbid] != 0 {
					t.Errorf("%s: call row must not be named %q (instantiated spelling or comparison operand); rows=%v", tc.lang, forbid, names)
				}
			}
		})
	}
}

// Generic method-on-receiver calls keep the receiver axis the plain arm
// resolves: the type arguments never displace the lexical receiver type.
func TestGenericCalleeRowsKeepReceiverIdentity(t *testing.T) {
	t.Run("rust", func(t *testing.T) {
		src := []byte("fn run(repo: &Repository) { repo.load::<Item>(1); Repository::open::<Item>(); }\n")
		_, _, _, rels := extractRust(parseSourceFor(t, types.LangRust, string(src)), src, "svc.rs")
		requireCallReceiver(t, rels, "load", "Repository")
		requireCallReceiver(t, rels, "open", "Repository")
	})
	t.Run("cpp", func(t *testing.T) {
		src := []byte("void run(Repository& repo) { repo.get<int>(); Repository::open<int>(); }\n")
		_, _, _, rels := extractCCpp(parseSourceFor(t, types.LangCpp, string(src)), src, "svc.cpp", types.LangCpp)
		requireCallReceiver(t, rels, "get", "Repository")
		requireCallReceiver(t, rels, "open", "Repository")
	})
	t.Run("kotlin", func(t *testing.T) {
		src := []byte("class Svc { fun run(repo: Repository) { repo.load<Item>(1) } }\n")
		_, _, _, rels := extractKotlin(parseSourceFor(t, types.LangKotlin, string(src)), src, "Svc.kt")
		requireCallReceiver(t, rels, "load", "Repository")
	})
	t.Run("cangjie", func(t *testing.T) {
		src := []byte("package demo\nfunc run(repo: Repository): Unit { repo.load<Item>(1) }\n")
		_, _, _, rels, _ := extractCangjie(src, "svc.cj")
		requireCallReceiver(t, rels, "load", "Repository")
	})
}

// The Cangjie generic head is a precise adjacency shape: `ident<…>(` with
// the `<` touching the identifier and the `>` touching the `(`. Every
// relaxation is a comparison chain or an expression and yields no row.
func TestCangjieGenericCallHeadRequiresAdjacentBalancedTypeArguments(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"generic call", "parse<Int64>(s)", true},
		{"nested generic call", "build<Array<Int64>>(s)", true},
		{"function-type argument", "wrap<(Int64)->Unit>(s)", true},
		{"optional-type argument", "wrap<?Int64>(s)", true},
		{"space before angle", "parse <Int64>(s)", false},
		{"space before paren", "parse<Int64> (s)", false},
		{"comparison chain", "parse<limit && cnt>(s)", false},
		{"unbalanced", "parse<Int64(s)", false},
		{"expression operand", "parse<limit + 1>(s)", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tokens := lexCangjie([]byte(tc.src))
			got := cangjieCallParenIndex(tokens, 0) >= 0
			if got != tc.want {
				t.Fatalf("%q: call head=%t, want %t (tokens=%+v)", tc.src, got, tc.want, tokens)
			}
		})
	}
}

// genericCalleeBase is a closed node-type list: anything else passes
// through untouched, so the plain arms keep their exact behaviour.
func TestGenericCalleeBasePassesPlainCalleesThrough(t *testing.T) {
	src := "fn run() { helper(1); }\n"
	root := parseSourceFor(t, types.LangRust, src)
	var seen bool
	walkNamedChildren(root, true, func(node *sitter.Node) {
		if node.Type() != "call_expression" {
			return
		}
		fn := node.ChildByFieldName("function")
		if got := genericCalleeBase(fn); got != fn {
			t.Fatalf("plain callee must pass through unchanged: %s -> %s", fn.Type(), got.Type())
		}
		seen = true
	})
	if !seen || genericCalleeBase(nil) != nil {
		t.Fatalf("fixture must contain a call (seen=%t) and nil must pass through", seen)
	}
}

// Swift's constructor_expression is a construction site for the line
// feature index, so the grounder's typed "this line is a call site" signal
// covers `Box<Int>(x)` exactly like `new Box<string>(1)` in TypeScript.
func TestSwiftConstructorExpressionRegistersNewExpressionLineFeature(t *testing.T) {
	src := "class Svc { func run(x: Int) {\n let a = Box<Int>(x)\n } }\n"
	root := parseSourceFor(t, types.LangSwift, src)
	features := map[int][]types.LineFeature{}
	walkNamedChildren(root, true, func(node *sitter.Node) {
		walkLineFeatures(node, []byte(src), func(line int, f types.LineFeature) {
			features[line] = append(features[line], f)
		})
	})
	found := false
	for _, f := range features[2] {
		if f == types.LineFeatureNewExpression {
			found = true
		}
	}
	if !found {
		t.Fatalf("line 2 must carry LineFeatureNewExpression: %v", features)
	}
}

// The AST callee spelling and the persisted warm cache are one source:
// every language whose call rows changed shape rejects pre-B1554 caches.
func TestGenericCalleeExtractorCacheEpochFloors(t *testing.T) {
	floors := map[string]int{
		types.LangRust: 12, types.LangC: 11, types.LangCpp: 12,
		types.LangSwift: 10, types.LangCangjie: 10,
	}
	for language, floor := range floors {
		if got := extractorVersions[language]; got < floor {
			t.Errorf("extractorVersions[%q]=%d, want >=%d after the generic-callee row change", language, got, floor)
		}
	}
}
