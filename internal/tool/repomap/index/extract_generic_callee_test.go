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
			src: "void run(Repository& repo, int a, int b, int c, int d) { auto s = std::make_unique<ConsoleSink>(); auto t = make_unique<Sink>(1); repo.get<int>(); repo.template fetch<int>(); ns::convert<Item>(2); helper(); if (a < b && c > (d)) { helper(); } }\n",
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
		types.LangRust: 12, types.LangC: 11, types.LangCpp: 15,
		types.LangSwift: 10, types.LangCangjie: 10,
	}
	for language, floor := range floors {
		if got := extractorVersions[language]; got < floor {
			t.Errorf("extractorVersions[%q]=%d, want >=%d after the generic-callee row change", language, got, floor)
		}
	}
}

// §40.59 收编复核再收编 (batch-six fold-in review #1/#2): the tree-sitter
// template_function reading is the precise witness on the AST tier. Every
// template call — whatever its template-argument interior spells (a type,
// a negative / arithmetic / comparison / logical non-type argument, an
// rvalue reference, a string literal, a lambda, sizeof, a ternary) —
// mints a Kind=call row carrying the BARE callee name in every arm
// (identifier, field, scoped/qualified); no instantiated spelling ever
// reaches ToEP.Name. The one grammar ambiguity, a comparison chain
// `a < b && c > (d)` the grammar also reads as a template call, is dropped
// only by the typed discriminator cppTemplateReadingIsComparisonChain,
// keyed on the callee (§40.59 收编复核三轮): a callee resolving to a
// declared value mints no row whatever the interior spells (the comma and
// nested-chain interiors over a declared parameter below flipped from
// "row" to "no row" in round three — a declared int can never take
// template arguments); an unresolvable callee is dropped only on the
// typed witnesses (both operands integer literals / runtime values, or the
// self-name operand). A chain over wholly undeclared names keeps the row —
// the disclosed residual.
//
// EVOLUTION RECORD: red on 381f36cc9 — the byte-whitelist interior guard
// dropped `f<-1>(x)`, `f<T&&>(x)`, `f<N + 1>(x)`, `f<!flag>(x)`, the
// ternary, string-literal and lambda non-type arguments (rows the declared
// base 8a1e5d695 minted), and the qualified arm published the instantiated
// spelling `get<N - 1>` / `make_index_sequence<N - 1>` /
// `integral_constant<int, -1>` / `a<b && c>` verbatim; red on 8a1e5d695 —
// the compact chain `a<b && c>(d)` over declared operands minted `a`, and
// the spaced `make_unique <T> ()` template call minted no row (adjacency
// guard). Green once genericCalleeBase always unwraps to the bare name and
// the comparison chain is keyed on the typed operator + operand shape.
func TestCppTemplateReadingsMintBareNamesExceptTypedComparisonChains(t *testing.T) {
	cases := []struct {
		name   string
		src    string            // one C++ translation unit
		want   map[string]string // callee → receiver that must appear on a call row ("" = any)
		forbid []string          // names that must not appear on any call row
	}{
		{"negative non-type argument", "void run(int x) { f<-1>(x); }", map[string]string{"f": ""}, []string{"f<-1>"}},
		{"rvalue reference argument", "void run(int x) { f<T&&>(x); }", map[string]string{"f": ""}, []string{"f<T&&>"}},
		{"arithmetic non-type argument", "void run(int x) { f<N + 1>(x); }", map[string]string{"f": ""}, []string{"f<N + 1>"}},
		{"negation non-type argument", "void run(int x) { f<!flag>(x); }", map[string]string{"f": ""}, []string{"f<!flag>"}},
		{"ternary non-type argument", "void run(int x) { f<(N > 0 ? 1 : 2)>(x); }", map[string]string{"f": ""}, []string{"f<(N > 0 ? 1 : 2)>"}},
		{"string literal argument", "void run(int x) { f<\"abc\">(x); }", map[string]string{"f": ""}, []string{"f<\"abc\">"}},
		{"lambda non-type argument", "void run(int x) { f<[](int v){ return v; }>(x); }", map[string]string{"f": ""}, nil},
		{"sizeof argument", "void run(int x) { f<sizeof(x)>(x); }", map[string]string{"f": ""}, []string{"f<sizeof(x)>"}},
		{"equality non-type argument", "void run(int x) { foo<a == b>(x); }", map[string]string{"foo": ""}, []string{"foo<a == b>"}},
		{"division non-type argument", "void run(int x) { foo<n / 2>(x); }", map[string]string{"foo": ""}, []string{"foo<n / 2>"}},
		{"qualified tuple recursion", "void run(T& t) { std::get<N - 1>(t); }", map[string]string{"get": "std"}, []string{"get<N - 1>"}},
		{"qualified index sequence", "void run() { std::make_index_sequence<N - 1>(); }", map[string]string{"make_index_sequence": "std"}, []string{"make_index_sequence<N - 1>"}},
		{"qualified integral constant", "void run() { std::integral_constant<int, -1>(); }", map[string]string{"integral_constant": "std"}, []string{"integral_constant<int, -1>"}},
		{"qualified arithmetic", "void run(T& t) { std::apply<N+1>(t); }", map[string]string{"apply": "std"}, []string{"apply<N+1>"}},
		{"qualified comparison argument", "void run(T& t) { std::foo<N == 1>(t); }", map[string]string{"foo": "std"}, []string{"foo<N == 1>"}},
		{"qualified type argument control", "void run(T& t) { std::get<N>(t); }", map[string]string{"get": "std"}, []string{"get<N>"}},
		{"spaced template call", "void run() { make_unique <T> (); }", map[string]string{"make_unique": ""}, []string{"make_unique <T>"}},
		{"bool literal conjunction is not a chain", "void run(int x, int flag) { f<true && flag>(x); }", map[string]string{"f": ""}, []string{"f<true && flag>"}},
		{"nested template", "void run(int x) { foo<std::vector<int>>(x); }", map[string]string{"foo": ""}, []string{"foo<std::vector<int>>"}},
		{"two type arguments over an undeclared callee", "void run(int b, int c, int d) { if (a<b, c>(d)) { helper(); } }", map[string]string{"a": "", "helper": ""}, []string{"a<b, c>"}},
		{"template method on receiver", "void run(Repository& repo, int x) { repo.get<int&>(x); }", map[string]string{"get": "Repository"}, []string{"get<int&>"}},

		// The typed comparison-chain discriminator is keyed on the callee
		// (§40.59 收编复核三轮): a callee that resolves to a declared value
		// mints no row in any arm whatever the interior spells; an
		// unresolvable callee over two runtime operands is the retained
		// tie-breaker.
		{"comma expression over a declared callee", "void run(int a, int b, int c, int d) { if (a<b, c>(d)) { helper(); } }", map[string]string{"helper": ""}, []string{"a", "a<b, c>"}},
		{"compact chain over parameters", "void run(int a, int b, int c, int d) { if (a<b && c>(d)) { helper(); } }", map[string]string{"helper": ""}, []string{"a", "a<b && c>", "c"}},
		{"spaced chain over parameters", "void run(int a, int b, int c, int d) { if (a < b && c > (d)) { helper(); } }", map[string]string{"helper": ""}, []string{"a", "c"}},
		{"or chain over parameters", "void run(int a, int b, int c, int d) { if (a<b || c>(d)) { helper(); } }", map[string]string{"helper": ""}, []string{"a", "a<b || c>", "c"}},
		{"chain with integer literal and local", "void run(int height) { int lhs = 1; int cnt = 2; if (lhs<0 && cnt>(height)) { helper(); } }", map[string]string{"helper": ""}, []string{"lhs", "lhs<0 && cnt>", "cnt"}},
		{"chain in initialiser over locals", "void run(int n, int k) { int i = 0; int j = 1; bool ok = i<n && j>(k); }", nil, []string{"i", "i<n && j>", "j"}},
		{"chain over for-loop locals", "void run(int n, int k) { for (int i = 0, j = 1; i<n && j>(k); ++i) { helper(); } }", map[string]string{"helper": ""}, []string{"i", "i<n && j>", "j"}},
		{"qualified chain over parameters", "void run(int b, int c, int d) { if (ns::a<b && c>(d)) { helper(); } }", map[string]string{"helper": ""}, []string{"a", "a<b && c>"}},
		{"chain over class fields", "class Svc { int lhs_; int limit_; int cnt_; int max_; void run() { if (lhs_<limit_ && cnt_>(max_)) { helper(); } } };", map[string]string{"helper": ""}, []string{"lhs_", "lhs_<limit_ && cnt_>", "cnt_"}},
		{"chain inside lambda over lambda parameters", "void run() { auto fn = [](int a, int b, int c, int d) { return a<b && c>(d); }; }", nil, []string{"a", "a<b && c>"}},

		{"nested chain over a declared callee", "void run(int a, int b, int c, int d, int e) { if (a<b && c || d>(e)) { helper(); } }", map[string]string{"helper": ""}, []string{"a", "a<b && c || d>"}},

		// Residual, disclosed: without any declared witness — callee and
		// operands all unresolvable — the template reading stands (the
		// grammar's own reading). The full scope / witness matrix lives in
		// extract_cpp_callee_scope_test.go.
		{"chain over undeclared names keeps the template reading", "void run() { if (a<b && c>(d)) { helper(); } }", map[string]string{"a": "", "helper": ""}, []string{"a<b && c>"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src + "\n")
			_, _, _, rels := extractCCpp(parseSourceFor(t, types.LangCpp, string(src)), src, "svc.cpp", types.LangCpp)
			names := genericCalleeNames(rels)
			for callee, receiver := range tc.want {
				if names[callee] == 0 {
					t.Errorf("%q: no call row named %q; rows=%v", tc.src, callee, names)
					continue
				}
				if receiver != "" {
					requireCallReceiver(t, rels, callee, receiver)
				}
			}
			for _, forbid := range tc.forbid {
				if names[forbid] != 0 {
					t.Errorf("%q: call row must not be named %q (instantiated spelling or comparison operand); rows=%v", tc.src, forbid, names)
				}
			}
		})
	}
}

// The discriminator reads the typed oracle answer for the callee first
// (a declared value: chain in any interior; a declared callable: never a
// chain) and, only for an unresolvable callee, the typed operator + operand
// shape of the interior (the self-name witness, or `&&`/`||` over integer
// literals and RUNTIME values); any other interior returns false.
func TestCppTemplateComparisonChainDiscriminatorReadsTypedShapeOnly(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		kinds map[string]cppNameKind // stub oracle; absent names are unresolved
		want  bool
	}{
		{"value callee with a type-argument interior", "f<T>(x);", map[string]cppNameKind{"f": cppNameValue}, true},
		{"constant callee with a type-argument interior", "f<T>(x);", map[string]cppNameKind{"f": cppNameConstant}, true},
		{"value callee with a comma interior", "if (a<b, c>(d)) {}", map[string]cppNameKind{"a": cppNameValue}, true},
		{"value callee with a nested chain interior", "if (a<b && c || d>(e)) {}", map[string]cppNameKind{"a": cppNameValue}, true},
		{"callable callee over runtime operands", "if (a<b && c>(d)) {}", map[string]cppNameKind{"a": cppNameCallable, "b": cppNameValue, "c": cppNameValue}, false},
		{"unresolved callee over runtime operands", "if (a<b && c>(d)) {}", map[string]cppNameKind{"b": cppNameValue, "c": cppNameValue}, true},
		{"unresolved callee over undeclared operands", "if (a<b && c>(d)) {}", nil, false},
		{"unresolved callee over one runtime operand", "if (a<b && MAX>(d)) {}", map[string]cppNameKind{"b": cppNameValue}, false},
		{"unresolved callee over constant operands", "f<a && b>(x);", map[string]cppNameKind{"a": cppNameConstant, "b": cppNameConstant}, false},
		{"unresolved callee or with integer literal", "if (a<1 || c>(d)) {}", map[string]cppNameKind{"c": cppNameValue}, true},
		{"unresolved callee with the self-name witness", "if (x<lo || x>(hi)) {}", nil, true},
		{"unresolved qualified callee over runtime operands", "if (ns::a<b && c>(d)) {}", map[string]cppNameKind{"b": cppNameValue, "c": cppNameValue}, true},
		{"unresolved nested-qualified callee over runtime operands", "if (ns::inner::a<b && c>(d)) {}", map[string]cppNameKind{"b": cppNameValue, "c": cppNameValue}, true},
		{"single type argument", "f<T>(x);", nil, false},
		{"two arguments", "f<a, b>(x);", map[string]cppNameKind{"a": cppNameValue, "b": cppNameValue}, false},
		{"arithmetic", "f<a + b>(x);", map[string]cppNameKind{"a": cppNameValue, "b": cppNameValue}, false},
		{"equality", "f<a == b>(x);", map[string]cppNameKind{"a": cppNameValue, "b": cppNameValue}, false},
		{"negative literal", "f<-1>(x);", nil, false},
		{"bool literal operand", "f<true && b>(x);", map[string]cppNameKind{"b": cppNameValue}, false},
		{"nested chain operand", "if (a<b && c || d>(e)) {}", map[string]cppNameKind{"b": cppNameValue, "c": cppNameValue, "d": cppNameValue}, false},
		{"plain callee", "f(x);", map[string]cppNameKind{"f": cppNameValue}, false},
		{"template method on a receiver is never consulted", "r.template f<a && b>(x);", map[string]cppNameKind{"f": cppNameValue, "a": cppNameValue, "b": cppNameValue}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte("void run() { " + tc.src + " }\n")
			root := parseSourceFor(t, types.LangCpp, string(src))
			var got, seen bool
			walkNamedChildren(root, true, func(node *sitter.Node) {
				if node.Type() != "call_expression" || seen {
					return
				}
				seen = true
				got = cppTemplateReadingIsComparisonChain(node.ChildByFieldName("function"), src, func(ident *sitter.Node) cppNameKind {
					return tc.kinds[nodeText(ident, src)]
				})
			})
			if !seen {
				t.Fatalf("%q: fixture must parse to a call_expression", tc.src)
			}
			if got != tc.want {
				t.Fatalf("%q kinds=%v: comparison chain=%t, want %t", tc.src, tc.kinds, got, tc.want)
			}
		})
	}
}
