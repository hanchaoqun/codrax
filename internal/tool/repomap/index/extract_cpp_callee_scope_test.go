package index

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// extract_cpp_callee_scope_test.go — §40.59 收编复核三轮 (batch-six fold-in
// review round three #0/#1/#2): the C++ comparison-chain discriminator is
// keyed on the CALLEE. A callee that resolves to a declared value in the
// translation unit — a parameter or local of any enclosing function
// (through lambdas), a field of the enclosing class or of the class an
// out-of-line qualifier names (its in-file bases included), a namespace /
// file-scope variable (preprocessor branches included), a range / structured
// binding, a condition-clause or catch declaration, a non-type template
// parameter, an enumerator — can never take template arguments, so
// `callee<…>(…)` is a comparison chain and mints no row in any arm. A
// callee that resolves to a declared callable keeps the template reading
// whatever the interior spells. An unresolvable callee keeps the template
// reading except on two typed witnesses: an interior operand spelling the
// callee's own name (the `x < lo || x > (hi)` range check), or both interior
// operands being integer literals / declared RUNTIME values (the retained
// tie-breaker; constant-capable operands are legal bool non-type arguments
// and keep the row). Qualifier chains of any depth unwrap to the terminal
// name with the immediate scope as receiver.
//
// This file is self-contained (own helpers) so the identical pins run
// unchanged against scratch copies of both bases.
//
// EVOLUTION RECORD (run on live 480939385 and, in scratch copies, on
// 381f36cc9 and 8a1e5d695):
//   - red on 381f36cc9: the byte-whitelist interior guard dropped every
//     `&&` / `||` template reading, so the constexpr-field / constexpr-local
//     / non-type-template-parameter / declared-template-callee rows and the
//     undeclared-chain row were missing (zero rows); the qualified arm
//     published `chrono::duration_cast<std::chrono::milliseconds>`,
//     `b::c::f<int>` and `inner::a<b && c>`, and the compact chains over
//     declared callees that base minted nothing for were green by accident.
//   - red on 8a1e5d695: the adjacency guard minted a call row for every
//     compact chain — out-of-line members over in-file class fields, lambda
//     captures, declared parameter / global / namespace / binding / catch /
//     condition-clause / non-type-parameter callees, the comma and nested
//     interiors, the compact header-only range check — and published the
//     same nested-qualifier spellings.
//   - green on live: both bases' false rows are gone, every legitimate
//     template reading keeps its bare-name row, and nested qualifiers
//     publish the terminal name.
//   - the two disclosed residuals keep the grammar's own reading on all
//     three trees and are pinned as such: an unresolvable callee whose
//     chain has only one runtime witness, and a header-declared class field
//     whose chain carries no in-file witness.

func cppCalleeScopeRows(t *testing.T, src string) ([]types.Relation, map[string]int) {
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

func cppCalleeScopeReceiver(t *testing.T, rels []types.Relation, name, receiver string) {
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

func TestCppComparisonChainDiscriminatorKeysOnTheCallee(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		want   map[string]string // callee → receiver that must appear on a call row ("" = any)
		forbid []string          // names that must not appear on any call row
	}{
		// Out-of-line members resolve through the class the qualifier
		// names, in this file (own members and in-file bases).
		{"out-of-line member spaced or-chain over a field (protobuf shape)",
			"class Reader { int field_number_; bool Check(); };\nbool Reader::Check() {\n  if (field_number_ < 1 || field_number_ > (1 << 29) - 1) {\n    fail();\n  }\n  return true;\n}",
			map[string]string{"fail": ""}, []string{"field_number_"}},
		{"out-of-line member compact chain over fields",
			"class Svc { int lhs_, limit_, cnt_, max_; bool run(); };\nbool Svc::run() { if (lhs_<limit_ && cnt_>(max_)) { helper(); } return true; }",
			map[string]string{"helper": ""}, []string{"lhs_", "lhs_<limit_ && cnt_>", "cnt_"}},
		{"out-of-line member of a namespaced class",
			"namespace ns { class Svc { int lhs_; int limit_; int cnt_; int max_; void run(); }; }\nvoid ns::Svc::run() { if (lhs_<limit_ && cnt_>(max_)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"lhs_"}},
		{"out-of-line member of a class template",
			"template<typename T> class Parser { T lhs_; T limit_; T cnt_; T max_; void run(); };\ntemplate<typename T> void Parser<T>::run() { if (lhs_<limit_ && cnt_>(max_)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"lhs_"}},
		{"out-of-line member over an in-file base-class field",
			"struct Base { int lhs_; int limit_; int cnt_; int max_; };\nstruct Derived : Base { void run(); };\nvoid Derived::run() { if (lhs_<limit_ && cnt_>(max_)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"lhs_"}},
		{"in-class member spaced chain over fields",
			"class Svc { int lhs_; int limit_; int cnt_; int max_; void run() { if (lhs_ < limit_ && cnt_ > (max_)) { helper(); } } };",
			map[string]string{"helper": ""}, []string{"lhs_"}},

		// Lambdas resolve outward through their enclosing functions.
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

		// A declared callee decides the reading whatever the interior spells.
		{"parameter callee with a macro operand",
			"void run(int n, int count, int limit) { if (n<MAX_N && count>(limit)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"n", "n<MAX_N && count>"}},
		{"spaced bounds check over macros",
			"void run(int x, int y) { if (x < WIDTH && y > (HEIGHT - 1)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"x"}},
		{"parameter callee with a call operand",
			"void run(int n, int count, int limit) { if (n < size() && count > (limit)) { helper(); } }",
			map[string]string{"helper": "", "size": ""}, []string{"n"}},
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
		{"comma interior over a declared callee is a comma expression",
			"void run(int a, int b, int c, int d) { if (a<b, c>(d)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a", "a<b, c>"}},
		{"nested chain interior over a declared callee",
			"void run(int a, int b, int c, int d, int e) { if (a<b && c || d>(e)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a", "a<b && c || d>"}},
		{"single type-argument interior over a declared callee",
			"void run(int a, int T, int x) { if (a<T>(x)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a", "a<T>"}},

		// The template reading stands: declared callables, constant-capable
		// operands, undeclared names.
		{"constexpr fields as bool non-type arguments",
			"struct Cfg { static constexpr bool kFast = true; static constexpr bool kSafe = false; void run(int x) { dispatch<kFast && kSafe>(x); } };",
			map[string]string{"dispatch": ""}, []string{"dispatch<kFast && kSafe>"}},
		{"or over constexpr fields",
			"struct Cfg { static constexpr bool kFast = true; static constexpr bool kSafe = false; void run(int x) { dispatch<kFast || kSafe>(x); } };",
			map[string]string{"dispatch": ""}, nil},
		{"constexpr locals as bool non-type arguments",
			"void run(int x) { constexpr bool a = true; constexpr bool b = false; f<a && b>(x); }",
			map[string]string{"f": ""}, []string{"f<a && b>"}},
		{"non-type template parameters as bool arguments",
			"template<bool A, bool B> void run(int x) { f<A && B>(x); }",
			map[string]string{"f": ""}, []string{"f<A && B>"}},
		{"declared function template callee over runtime operands",
			"template<bool F> void f(int); void run(int a, int b, int x) { f<a && b>(x); }",
			map[string]string{"f": ""}, []string{"f<a && b>"}},
		{"declared method template callee over runtime operands",
			"struct S { template<bool F> void dispatch(int); void run(int x, bool a, bool b) { dispatch<a && b>(x); } };",
			map[string]string{"dispatch": ""}, nil},
		{"declared template callee with constexpr field arguments",
			"struct Cfg { static constexpr bool kFast = true; static constexpr bool kSafe = false; template<bool F> void dispatch(int); void run(int x) { dispatch<kFast && kSafe>(x); } };",
			map[string]string{"dispatch": ""}, nil},
		{"undeclared chain keeps the template reading",
			"void run() { if (a<b && c>(d)) { helper(); } }",
			map[string]string{"a": "", "helper": ""}, []string{"a<b && c>"}},

		// Unresolvable callee: the two typed witnesses.
		{"range check over a header-declared field (self-name witness)",
			"bool Reader::Check() {\n  if (field_number_ < 1 || field_number_ > (1 << 29) - 1) {\n    fail();\n  }\n  return true;\n}",
			map[string]string{"fail": ""}, []string{"field_number_"}},
		{"compact range check with the self-name witness",
			"bool Svc::ok() { return pos_<lo_ || pos_>(hi_); }",
			nil, []string{"pos_"}},
		{"unresolvable callee over two runtime operands (tie-breaker)",
			"void run(int b, int c, int d) { if (a<b && c>(d)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a", "a<b && c>"}},
		{"unresolvable qualified callee over runtime operands",
			"void run(int b, int c, int d) { if (std::a<b && c>(d)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a", "a<b && c>"}},
		{"unresolvable callee over a literal and a runtime local",
			"void run(int height) { int cnt = 2; if (lhs<0 && cnt>(height)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"lhs"}},
		{"unresolvable callee over constexpr operands keeps the template reading",
			"void run(int x) { constexpr bool a = true; constexpr bool b = false; if (f<a && b>(x)) { helper(); } }",
			map[string]string{"f": "", "helper": ""}, nil},

		// Disclosed residuals: the grammar's own reading stands without a
		// typed witness.
		{"residual: unresolvable callee with one runtime witness keeps the template reading",
			"void run(int b, int d) { if (a<b && MAX>(d)) { helper(); } }",
			map[string]string{"a": "", "helper": ""}, nil},
		{"residual: header-declared class field with no in-file witness",
			"bool Svc::run() { if (lhs_<limit_ && cnt_>(max_)) { helper(); } return true; }",
			map[string]string{"lhs_": "", "helper": ""}, nil},

		// Qualifier chains unwrap to the terminal name in every arm.
		{"two-level qualified template callee",
			"void run(D d) { auto ms = std::chrono::duration_cast<std::chrono::milliseconds>(d); }",
			map[string]string{"duration_cast": "chrono"}, []string{"chrono::duration_cast<std::chrono::milliseconds>", "duration_cast<std::chrono::milliseconds>"}},
		{"three-level qualified template callee",
			"void run(int x) { a::b::c::f<int>(x); }",
			map[string]string{"f": "c"}, []string{"b::c::f<int>", "c::f<int>", "f<int>"}},
		{"two-level qualified plain callee",
			"void run() { auto n = std::chrono::steady_clock::now(); }",
			map[string]string{"now": "steady_clock"}, []string{"chrono::steady_clock::now", "steady_clock::now"}},
		{"two-level qualified chain over parameters",
			"void run(int b, int c, int d) { if (ns::inner::a<b && c>(d)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a", "inner::a<b && c>", "a<b && c>"}},
		{"two-level qualified namespace variable callee",
			"namespace ns { namespace inner { int a; } } void run(int b, int c, int d) { if (ns::inner::a<b && c>(d)) { helper(); } }",
			map[string]string{"helper": ""}, []string{"a"}},
		{"single-level qualified template control",
			"void run(int x) { ns::f<int>(x); }",
			map[string]string{"f": "ns"}, []string{"f<int>"}},
		{"template-type qualifier keeps its spelling as receiver",
			"void run(int x) { Parser<int>::parse<int>(x); }",
			map[string]string{"parse": "Parser<int>"}, []string{"parse<int>"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rels, names := cppCalleeScopeRows(t, tc.src)
			for callee, receiver := range tc.want {
				if names[callee] == 0 {
					t.Errorf("%q: no call row named %q; rows=%v", tc.src, callee, names)
					continue
				}
				if receiver != "" {
					cppCalleeScopeReceiver(t, rels, callee, receiver)
				}
			}
			for _, forbid := range tc.forbid {
				if names[forbid] != 0 {
					t.Errorf("%q: call row must not be named %q (comparison operand or instantiated spelling); rows=%v", tc.src, forbid, names)
				}
			}
		})
	}
}
