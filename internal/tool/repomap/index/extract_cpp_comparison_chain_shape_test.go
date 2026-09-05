package index

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// extract_cpp_comparison_chain_shape_test.go — §40.59 收编复核四轮 (batch-six
// fold-in review round four #0/#1/#2/#4/#5): the C++ comparison-chain
// discriminator is keyed on the GRAMMAR SHAPE alone. A call_expression whose
// callee is a template_function — bare, or the terminal of a qualified chain
// of any depth — whose template_argument_list is exactly one top-level
// binary_expression with operator `&&` or `||` is the comparison-chain
// ambiguity and mints no call row in any arm, whatever the operands spell
// and whatever the callee names. Every other template reading keeps its
// bare-name row with the immediate qualifier as receiver.
//
// The rule ends the name-resolution arms race of rounds two and three: the
// per-file cppScopeResolver (block locals, lambdas, out-of-line classes,
// namespaces, non-type template parameters, enumerators, …) is deleted —
// most-vexing-parse locals, anonymous / inline namespaces, same-named
// classes, init-captures, case-label declarations and header-only classes
// each defeated it, and each fix was one more noisy heuristic under a
// row-minting gate. The disclosed residual of the shape rule is the
// legitimate constexpr-bool non-type argument spelled as one `&&` / `||`
// expression (`dispatch<kFast && kSafe>(x)`): same shape, no row.
//
// This file is self-contained (own helpers, distinct names) so the identical
// pins run unchanged against scratch copies of both bases.
//
// EVOLUTION RECORD (run on live and, in git-archive scratch copies, on
// 480939385 and b6f7eeec3; the retired extract_cpp_callee_scope_test.go /
// extract_cpp_scope_resolver_test.go pinned the resolver these rows replace):
//   - red on 480939385, 43 of 101 sub-pins (operand-keyed tie-breaker over
//     the nearest function's locals / class fields only, one-level
//     qualifier unwrap): a false callee row for every chain whose operands
//     were not both integer literals / nearest-scope declarations — the
//     protobuf spaced range check (class in file and header-only), the
//     compact out-of-line chains (header-only, in-file, namespaced, class
//     template, base-class field), the self-name range check, the lambdas
//     over outer parameters / outer locals via std::sort / two levels, the
//     three init-captures, the four anonymous / inline namespace shapes,
//     the three same-named-class shapes, the two paren-initialised range
//     checks, the four case / default / goto-label declarations, the macro,
//     spaced-macro, call-operand, local-with-global, file-scope static and
//     structured-binding callees, the nested and arithmetic-operand
//     interiors, the wholly undeclared and single-witness chains, the
//     two-level qualified chain, and the non-type-parameter / bool-literal
//     residual shapes; plus the three nested-qualifier spellings
//     (`chrono::duration_cast<…>` / `b::c::f<int>` / `steady_clock::now`
//     published verbatim). The plain most-vexing-parse locals, the
//     parameter / global / preproc / namespace / binding / for-init /
//     condition-clause / catch / non-type-parameter / enumerator callees
//     and the constexpr-field / constexpr-local / declared-template
//     residuals were green there by the operand tie-breaker's accident.
//   - red on b6f7eeec3, 32 of 101 sub-pins (callee-keyed per-file resolver):
//     the five most-vexing-parse locals and the two paren-initialised range
//     checks (`Timer t(clock)` resolved callable), the three init-captures,
//     the four anonymous / inline namespace shapes, the second same-named
//     class and the detail-before-public class (visited keyed on the bare
//     class name), the four case / default / goto-label declarations, the
//     wholly undeclared and single-witness chains, the header-only compact
//     chain, and all seven constexpr-bool residual shapes (declared
//     template callees included) minted a row; `a<b, c>(d)` / `a<T>(x)`
//     over a declared parameter minted nothing (the resolver's "declared
//     value takes no template arguments" reading, retired — the extractor
//     no longer decides what a name denotes).
//   - green on live, 101/101: every `&&` / `||` shape mints no row, every
//     other template reading mints its bare-name row.

func cppChainShapeRows(t *testing.T, src string) ([]types.Relation, map[string]int) {
	t.Helper()
	source := []byte(src + "\n")
	_, _, _, rels := extractCCpp(parseSourceFor(t, types.LangCpp, string(source)), source, "svc.cpp", types.LangCpp)
	names := map[string]int{}
	for _, rel := range rels {
		if rel.Kind == "call" {
			names[rel.ToEP.Name]++
		}
	}
	return rels, names
}

func cppChainShapeReceiver(t *testing.T, rels []types.Relation, name, receiver string) {
	t.Helper()
	for _, rel := range rels {
		if rel.Kind == "call" && rel.ToEP.Name == name {
			if rel.ToEP.Receiver != receiver {
				t.Fatalf("%s receiver=%q, want %q: %+v", name, rel.ToEP.Receiver, receiver, rel)
			}
			return
		}
	}
	t.Fatalf("missing call relation for %s in %+v", name, rels)
}

func TestCppComparisonChainIsTheGrammarShapeAlone(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		want   map[string]string // callee → receiver that must appear on a call row ("" = any)
		forbid []string          // names that must not appear on any call row
	}{
		// ---- the ruled shape: one top-level `&&` / `||` → no row ----

		// Out-of-line members, class in the file or header-only alike.
		{"protobuf spaced range check, class in file",
			"class Reader { int field_number_; bool Check(); };\nbool Reader::Check() {\n  if (field_number_ < 1 || field_number_ > (1 << 29) - 1) {\n    fail();\n  }\n  return true;\n}",
			map[string]string{"fail": ""}, []string{"field_number_"}},
		{"protobuf spaced range check, header-only class",
			"bool Reader::Check() {\n  if (field_number_ < 1 || field_number_ > (1 << 29) - 1) {\n    fail();\n  }\n  return true;\n}",
			map[string]string{"fail": ""}, []string{"field_number_"}},
		{"compact chain in an out-of-line member, header-only class",
			"bool Svc::run() { if (lhs_<limit_ && cnt_>(max_)) { helper(); } return true; }",
			map[string]string{"helper": ""}, []string{"lhs_", "lhs_<limit_ && cnt_>", "cnt_"}},
		{"compact chain in an out-of-line member, class in file",
			"class Svc { int lhs_, limit_, cnt_, max_; bool run(); };\nbool Svc::run() { if (lhs_<limit_ && cnt_>(max_)) { helper(); } return true; }",
			map[string]string{"helper": ""}, []string{"lhs_"}},
		{"out-of-line member of a namespaced class",
			"namespace ns { class Svc { int lhs_; int limit_; int cnt_; int max_; void run(); }; }\nvoid ns::Svc::run() { if (lhs_<limit_ && cnt_>(max_)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"lhs_"}},
		{"out-of-line member of a class template",
			"template<typename T> class Parser { T lhs_; T limit_; T cnt_; T max_; void run(); };\ntemplate<typename T> void Parser<T>::run() { if (lhs_<limit_ && cnt_>(max_)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"lhs_"}},
		{"out-of-line member over a base-class field",
			"struct Base { int lhs_; int limit_; int cnt_; int max_; };\nstruct Derived : Base { void run(); };\nvoid Derived::run() { if (lhs_<limit_ && cnt_>(max_)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"lhs_"}},
		{"in-class member spaced chain",
			"class Svc { int lhs_; int limit_; int cnt_; int max_; void run() { if (lhs_ < limit_ && cnt_ > (max_)) { helper(); } } };",
			map[string]string{"helper": ""}, []string{"lhs_"}},
		{"compact self-name range check",
			"bool Svc::ok() { return pos_<lo_ || pos_>(hi_); }",
			nil, []string{"pos_"}},

		// Lambdas: plain captures, parameters, init-captures.
		{"lambda capturing outer parameters",
			"void run(int a,int b,int c,int d){ auto fn=[&](){ if (a<b && c>(d)) { helper(); } }; }",
			map[string]string{"helper": ""}, []string{"a", "a<b && c>"}},
		{"lambda argument capturing outer locals",
			"void run(V& v) { int a = 1, b = 2, c = 3, d = 4; std::sort(v.begin(), v.end(), [&](int x, int y){ return a<b && c>(d); }); }",
			map[string]string{"sort": "std", "begin": "", "end": ""}, []string{"a"}},
		{"nested lambdas through two levels",
			"void run(int a, int b, int c, int d) { auto outer = [&]() { auto inner = [&]() { return a<b && c>(d); }; }; }",
			nil, []string{"a"}},
		{"lambda over its own parameters",
			"void run() { auto fn = [](int a, int b, int c, int d) { return a<b && c>(d); }; }",
			nil, []string{"a"}},
		{"lambda init-capture callee over constexpr operands",
			"constexpr int kLo = 1, kHi = 2;\nvoid g(int d) { auto fn = [a = 1]() { return a<kLo && kHi>(d); }; }",
			nil, []string{"a"}},
		{"lambda reference init-capture callee",
			"constexpr int kLo = 1, kHi = 2;\nvoid g(int d, int z) { auto fn = [&r = z]() { return r<kLo && kHi>(d); }; }",
			nil, []string{"r"}},
		{"lambda init-captured bounds",
			"void g(int a, int b) { auto in = [lo = a, hi = b](int v) { return lo<v && hi>(v); }; }",
			nil, []string{"lo"}},

		// Anonymous and inline namespaces.
		{"anonymous-namespace variable callee",
			"constexpr int kLo = 1, kHi = 2;\nnamespace { int v; }\nvoid g() { if (v<kLo && kHi>(3)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"v"}},
		{"anonymous-namespace variable callee, spaced",
			"constexpr int kLo = 1, kHi = 2;\nnamespace { int v; }\nvoid g() { if (v < kLo && kHi > (3)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"v"}},
		{"inline-namespace variable callee",
			"constexpr int kLo = 1, kHi = 2;\ninline namespace lib { int v; }\nvoid g() { if (v<kLo && kHi>(3)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"v"}},
		{"anonymous-namespace operands with a header-only field callee",
			"namespace { int lo, hi; }\nbool Svc::run(int y) { if (x_<lo && hi>(y)) { helper(); } return true; }",
			map[string]string{"helper": ""}, []string{"x_"}},

		// Same-named classes in sibling namespaces, either order.
		{"same-named classes, second class member",
			"constexpr int kLo = 1, kHi = 2;\nnamespace v1 { struct Config { int n_; }; }\nnamespace v2 { struct Config { int lhs_; void run(); }; }\nvoid v2::Config::run() { if (lhs_<kLo && kHi>(3)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"lhs_"}},
		{"same-named classes, first class member",
			"constexpr int kLo = 1, kHi = 2;\nnamespace v2 { struct Config { int lhs_; void run(); }; }\nnamespace v1 { struct Config { int n_; }; }\nvoid v2::Config::run() { if (lhs_<kLo && kHi>(3)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"lhs_"}},
		{"detail class before the public class of the same name",
			"constexpr int kLo = 1, kHi = 2;\nnamespace detail { struct Impl { int n_; }; }\nstruct Impl { int lhs_; void run(); };\nvoid Impl::run() { if (lhs_<kLo && kHi>(3)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"lhs_"}},
		{"qualified static member chain of the second same-named class",
			"constexpr int kLo = 1, kHi = 2;\nnamespace v1 { struct Config { static int lhs_; }; }\nnamespace v2 { struct Config { static int lhs_; }; }\nvoid g() { if (v2::Config::lhs_<kLo && kHi>(3)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"lhs_"}},

		// Most-vexing-parse locals: direct-initialised objects.
		{"most-vexing-parse local object",
			"void run(int clock, int lo, int hi, int x) { Timer t(clock); if (t<lo && hi>(x)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"t"}},
		{"most-vexing-parse local object, spaced",
			"void run(int clock, int lo, int hi, int x) { Timer t(clock); if (t < lo && hi > (x)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"t"}},
		{"most-vexing-parse vector local",
			"void run(int n, int lo, int hi, int x) { std::vector<int> v(n); if (v<lo && hi>(x)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"v"}},
		{"most-vexing-parse size_t local",
			"void run(int a, int lo, int hi, int x) { size_t n(a); if (n<lo && hi>(x)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"n"}},
		{"most-vexing-parse file-scope object",
			"static Logger logger(cfg);\nvoid run(int lo, int hi, int x) { if (logger<lo && hi>(x)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"logger"}},
		{"range check over a paren-initialised local",
			"void run(int tag) { int n(tag); if (n < 1 || n > (1 << 29) - 1) { helper(); } }",
			map[string]string{"helper": ""}, []string{"n"}},
		{"range check over a paren-initialised local in a constructor body",
			"struct S { int lo_, hi_; S(int tag) { int n(tag); if (n < lo_ || n > (hi_)) { helper(); } } };",
			map[string]string{"helper": ""}, []string{"n"}},

		// Declarations under a case / default / goto label.
		{"declaration under an unbraced case label",
			"constexpr int kLo = 1, kHi = 2;\nvoid g(int s) { switch (s) { case 1: int q = 2; if (q<kLo && kHi>(3)) { helper(); } break; } }",
			map[string]string{"helper": ""}, []string{"q"}},
		{"declaration under an unbraced default label",
			"constexpr int kLo = 1, kHi = 2;\nvoid g(int s) { switch (s) { default: int q = 2; if (q<kLo && kHi>(3)) { helper(); } break; } }",
			map[string]string{"helper": ""}, []string{"q"}},
		{"declaration under a goto label",
			"constexpr int kLo = 1, kHi = 2;\nvoid g() { retry: int q = 2; if (q<kLo && kHi>(3)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"q"}},
		{"declaration used across case labels",
			"constexpr int kLo = 1, kHi = 2;\nvoid g(int s) { switch (s) { case 1: int q; q = 2; break; case 2: if (q<kLo && kHi>(3)) { helper(); } break; } }",
			map[string]string{"helper": ""}, []string{"q"}},

		// Every declaration kind the retired resolver enumerated.
		{"compact chain over parameters",
			"void run(int a, int b, int c, int d) { if (a<b && c>(d)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a", "a<b && c>", "c"}},
		{"spaced chain over parameters",
			"void run(int a, int b, int c, int d) { if (a < b && c > (d)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a", "c"}},
		{"or chain over parameters",
			"void run(int a, int b, int c, int d) { if (a<b || c>(d)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a", "a<b || c>", "c"}},
		{"parameter callee with a macro operand",
			"void run(int n, int count, int limit) { if (n<MAX_N && count>(limit)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"n", "n<MAX_N && count>"}},
		{"spaced bounds check over macros",
			"void run(int x, int y) { if (x < WIDTH && y > (HEIGHT - 1)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"x"}},
		{"parameter callee with a call operand",
			"void run(int n, int count, int limit) { if (n < size() && count > (limit)) { helper(); } }",
			map[string]string{"helper": "", "size": ""}, []string{"n"}},
		{"chain with an integer literal and a local",
			"void run(int height) { int lhs = 1; int cnt = 2; if (lhs<0 && cnt>(height)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"lhs", "cnt"}},
		{"chain in an initialiser over locals",
			"void run(int n, int k) { int i = 0; int j = 1; bool ok = i<n && j>(k); }",
			nil, []string{"i", "j"}},
		{"local callee with a global operand",
			"int kMax; void run(int count, int limit) { int n = 0; if (n<kMax && count>(limit)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"n"}},
		{"file-scope global callee",
			"int kMax; void run(int n, int count, int limit) { if (kMax<n && count>(limit)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"kMax"}},
		{"file-scope static callee",
			"static int counter_; void run(int n, int limit) { if (counter_<n || counter_>(limit)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"counter_"}},
		{"global declared in a preprocessor branch",
			"#ifdef X\nint gx;\n#endif\nvoid run(int n, int limit) { if (gx<n && n>(limit)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"gx"}},
		{"namespace-scope variable callee",
			"namespace ns { int a; void run(int b, int c, int d) { if (a<b && c>(d)) { helper(); } } }",
			map[string]string{"helper": ""}, []string{"a"}},
		{"qualified namespace variable callee",
			"namespace ns { int a; } void run(int b, int c, int d) { if (ns::a<b && c>(d)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a", "a<b && c>"}},
		{"range-for binding callee",
			"void run(V& xs, int n, int k) { for (auto x : xs) { if (x<n && k>(n)) { helper(); } } }",
			map[string]string{"helper": ""}, []string{"x"}},
		{"structured binding callee",
			"void run(M& m, int n, int k) { for (auto& [key, val] : m) { if (key<n && val>(k)) { helper(); } } }",
			map[string]string{"helper": ""}, []string{"key"}},
		{"for-init local callee",
			"void run(int n, int k) { for (int i = 0, j = 1; i<n && j>(k); ++i) { helper(); } }",
			map[string]string{"helper": ""}, []string{"i"}},
		{"condition-clause declaration callee",
			"void run(int n, int k) { if (int v = next()) { if (v<n && k>(n)) { helper(); } } }",
			map[string]string{"next": "", "helper": ""}, []string{"v"}},
		{"catch parameter callee",
			"void run(int n, int k) { try { } catch (int e) { if (e<n && k>(n)) { helper(); } } }",
			map[string]string{"helper": ""}, []string{"e"}},
		{"non-type template parameter callee",
			"template<int N> void run(int n, int k) { if (N<n && k>(n)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"N"}},
		{"enumerator callee",
			"enum Limits { kLimit }; void run(int n, int k) { if (kLimit<n && k>(n)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"kLimit"}},

		// Interiors that are still one top-level `&&` / `||`.
		{"nested chain interior",
			"void run(int a, int b, int c, int d, int e) { if (a<b && c || d>(e)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a", "a<b && c || d>"}},
		{"arithmetic operand inside the chain",
			"void run(int x, int n, int y, int m) { if (x<n + 1 && y>(m)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"x"}},
		{"chain over wholly undeclared names",
			"void run() { if (a<b && c>(d)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a", "a<b && c>"}},
		{"chain with a single declared operand",
			"void run(int b, int d) { if (a<b && MAX>(d)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a"}},
		{"chain with a comment inside the interior",
			"void run(int a, int b, int c, int d) { if (a<b /* lo */ && c>(d)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a"}},

		// Qualified chains of any depth end in the same shape.
		{"qualified chain over parameters",
			"void run(int b, int c, int d) { if (std::a<b && c>(d)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a", "a<b && c>"}},
		{"two-level qualified chain",
			"void run(int b, int c, int d) { if (ns::inner::a<b && c>(d)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a", "inner::a<b && c>", "a<b && c>"}},
		{"two-level qualified namespace variable callee",
			"namespace ns { namespace inner { int a; } } void run(int b, int c, int d) { if (ns::inner::a<b && c>(d)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a"}},

		// Disclosed residual: a constexpr-bool non-type argument spelled as
		// one `&&` / `||` expression is the same grammar shape → no row.
		{"residual: constexpr fields as a bool non-type argument",
			"struct Cfg { static constexpr bool kFast = true; static constexpr bool kSafe = false; void run(int x) { dispatch<kFast && kSafe>(x); } };",
			nil, []string{"dispatch", "dispatch<kFast && kSafe>"}},
		{"residual: or over constexpr fields",
			"struct Cfg { static constexpr bool kFast = true; static constexpr bool kSafe = false; void run(int x) { dispatch<kFast || kSafe>(x); } };",
			nil, []string{"dispatch"}},
		{"residual: constexpr locals as a bool non-type argument",
			"void run(int x) { constexpr bool a = true; constexpr bool b = false; f<a && b>(x); }",
			nil, []string{"f", "f<a && b>"}},
		{"residual: non-type template parameters as a bool argument",
			"template<bool A, bool B> void run(int x) { f<A && B>(x); }",
			nil, []string{"f", "f<A && B>"}},
		{"residual: declared function template callee over runtime operands",
			"template<bool F> void f(int); void run(int a, int b, int x) { f<a && b>(x); }",
			nil, []string{"f", "f<a && b>"}},
		{"residual: declared method template callee over runtime operands",
			"struct S { template<bool F> void dispatch(int); void run(int x, bool a, bool b) { dispatch<a && b>(x); } };",
			nil, []string{"dispatch"}},
		{"residual: bool literal conjunction",
			"void run(int x, int flag) { f<true && flag>(x); }",
			nil, []string{"f", "f<true && flag>"}},

		// ---- every other template reading: bare-name row ----

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
		{"bitwise-and non-type argument", "void run(int x) { f<a & b>(x); }", map[string]string{"f": ""}, []string{"f<a & b>"}},
		{"parenthesised conjunction is not the ruled shape", "void run(int x) { f<(a && b)>(x); }", map[string]string{"f": ""}, []string{"f<(a && b)>"}},
		{"conjunction beside a second argument is not the ruled shape", "void run(int x) { f<a && b, c>(x); }", map[string]string{"f": ""}, []string{"f<a && b, c>"}},
		{"comma interior over declared parameters keeps the grammar reading",
			"void run(int a, int b, int c, int d) { if (a<b, c>(d)) { helper(); } }",
			map[string]string{"a": "", "helper": ""}, []string{"a<b, c>"}},
		{"single type-argument interior over a declared parameter keeps the grammar reading",
			"void run(int a, int T, int x) { if (a<T>(x)) { helper(); } }",
			map[string]string{"a": "", "helper": ""}, []string{"a<T>"}},
		{"spaced template call", "void run() { make_unique <T> (); }", map[string]string{"make_unique": ""}, []string{"make_unique <T>"}},
		{"nested template", "void run(int x) { foo<std::vector<int>>(x); }", map[string]string{"foo": ""}, []string{"foo<std::vector<int>>"}},
		{"template method on a receiver", "void run(Repository& repo, int x) { repo.get<int&>(x); }", map[string]string{"get": "Repository"}, []string{"get<int&>"}},

		// Qualifier chains unwrap to the terminal name in every arm; the
		// immediate qualifier is the receiver.
		{"qualified tuple recursion", "void run(T& t) { std::get<N - 1>(t); }", map[string]string{"get": "std"}, []string{"get<N - 1>"}},
		{"qualified index sequence", "void run() { std::make_index_sequence<N - 1>(); }", map[string]string{"make_index_sequence": "std"}, []string{"make_index_sequence<N - 1>"}},
		{"qualified integral constant", "void run() { std::integral_constant<int, -1>(); }", map[string]string{"integral_constant": "std"}, []string{"integral_constant<int, -1>"}},
		{"qualified comparison argument", "void run(T& t) { std::foo<N == 1>(t); }", map[string]string{"foo": "std"}, []string{"foo<N == 1>"}},
		{"qualified make_unique", "void run() { auto p = std::make_unique<Sink>(); }", map[string]string{"make_unique": "std"}, []string{"make_unique<Sink>"}},
		{"two-level qualified template callee",
			"void run(D d) { auto ms = std::chrono::duration_cast<std::chrono::milliseconds>(d); }",
			map[string]string{"duration_cast": "chrono"}, []string{"chrono::duration_cast<std::chrono::milliseconds>", "duration_cast<std::chrono::milliseconds>"}},
		{"three-level qualified template callee",
			"void run(int x) { a::b::c::f<int>(x); }",
			map[string]string{"f": "c"}, []string{"b::c::f<int>", "c::f<int>", "f<int>"}},
		{"two-level qualified plain callee",
			"void run() { auto n = std::chrono::steady_clock::now(); }",
			map[string]string{"now": "steady_clock"}, []string{"chrono::steady_clock::now", "steady_clock::now"}},
		{"single-level qualified template control",
			"void run(int x) { ns::f<int>(x); }",
			map[string]string{"f": "ns"}, []string{"f<int>"}},
		{"template-type qualifier keeps its spelling as receiver",
			"void run(int x) { Parser<int>::parse<int>(x); }",
			map[string]string{"parse": "Parser<int>"}, []string{"parse<int>"}},

		// Disclosed: a template_method behind `.` / `->` / `template` is
		// never consulted — the grammar reading stands on every tree.
		{"residual: template method behind the template keyword keeps its row",
			"void run(R& r, int a, int b, int x) { r.template f<a && b>(x); }",
			map[string]string{"f": ""}, nil},
		// A field-access chain (`obj.f<a && b>(x)`) is read by the grammar
		// itself as a comparison — no call_expression, nothing to consult.
		{"field-access chain is a comparison in the grammar itself",
			"void run(R& obj, int a, int b, int x) { obj.f<a && b>(x); }",
			nil, []string{"f"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rels, names := cppChainShapeRows(t, tc.src)
			for callee, receiver := range tc.want {
				if names[callee] == 0 {
					t.Errorf("%q: no call row named %q; rows=%v", tc.src, callee, names)
					continue
				}
				if receiver != "" {
					cppChainShapeReceiver(t, rels, callee, receiver)
				}
			}
			for _, forbid := range tc.forbid {
				if names[forbid] != 0 {
					t.Errorf("%q: call row must not be named %q (comparison chain or instantiated spelling); rows=%v", tc.src, forbid, names)
				}
			}
		})
	}
}
