package index

import (
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// extract_cpp_scope_resolver_test.go — live-only unit pins for the
// cppScopeResolver oracle (§40.59 收编复核三轮); the behavioural matrix that
// also runs against both bases is extract_cpp_callee_scope_test.go.

// The scope resolver is the oracle the discriminator reads: one kind per
// declaration shape, innermost scope first, order-aware for block and file
// scope, never guessing across files. Live-only (the resolver is new).
func TestCppScopeResolverKinds(t *testing.T) {
	kindName := map[cppNameKind]string{cppNameUnresolved: "unresolved", cppNameValue: "value", cppNameConstant: "constant", cppNameCallable: "callable"}
	cases := []struct {
		name string
		src  string // the first call_expression's callee subtree holds `probe`
		want cppNameKind
	}{
		{"parameter", "void run(int probe) { probe(1); }", cppNameValue},
		{"local", "void run() { int probe = 0; probe(1); }", cppNameValue},
		{"const local", "void run() { const int probe = 0; probe(1); }", cppNameConstant},
		{"constexpr local", "void run() { constexpr int probe = 0; probe(1); }", cppNameConstant},
		{"global", "int probe; void run() { probe(1); }", cppNameValue},
		{"static global", "static int probe; void run() { probe(1); }", cppNameValue},
		{"static const global", "static const int probe = 1; void run() { probe(1); }", cppNameConstant},
		{"global in preprocessor branch", "#ifdef X\nint probe;\n#endif\nvoid run() { probe(1); }", cppNameValue},
		{"global in extern C block", "extern \"C\" { int probe; }\nvoid run() { probe(1); }", cppNameValue},
		{"single extern C declaration", "extern \"C\" int probe;\nvoid run() { probe(1); }", cppNameValue},
		{"single extern C prototype", "extern \"C\" int probe(int);\nvoid run() { probe(1); }", cppNameCallable},
		{"field", "struct S { int probe; void run() { probe(1); } };", cppNameValue},
		{"const non-static field is a runtime value", "struct S { const int probe; void run() { probe(1); } };", cppNameValue},
		{"static constexpr field", "struct S { static constexpr int probe = 1; void run() { probe(1); } };", cppNameConstant},
		{"static const field", "struct S { static const int probe = 1; void run() { probe(1); } };", cppNameConstant},
		{"method", "struct S { void probe(int); void run() { probe(1); } };", cppNameCallable},
		{"method template", "struct S { template<bool F> void probe(int); void run() { probe(1); } };", cppNameCallable},
		{"in-class method definition", "struct S { void probe(int) {} void run() { probe(1); } };", cppNameCallable},
		{"function prototype", "void probe(int); void run() { probe(1); }", cppNameCallable},
		{"function definition", "void probe(int) {} void run() { probe(1); }", cppNameCallable},
		{"function template", "template<typename T> T probe(T); void run() { probe(1); }", cppNameCallable},
		{"pointer-returning function", "int* probe(int); void run() { probe(1); }", cppNameCallable},
		{"function pointer local is a value", "void run() { int (*probe)(int); probe(1); }", cppNameValue},
		{"non-type template parameter", "template<int probe> void run() { probe(1); }", cppNameConstant},
		{"type template parameter is not a value", "template<typename probe> void run() { probe(1); }", cppNameUnresolved},
		{"enumerator", "enum E { probe }; void run() { probe(1); }", cppNameConstant},
		{"range binding", "void run(V& v) { for (auto probe : v) { probe(1); } }", cppNameValue},
		{"structured binding", "void run(M& m) { for (auto& [key, probe] : m) { probe(1); } }", cppNameValue},
		{"catch parameter", "void run() { try {} catch (int probe) { probe(1); } }", cppNameValue},
		{"if-init declaration", "void run() { if (int probe = 1) { probe(1); } }", cppNameValue},
		{"if-init statement", "void run() { if (int probe = 1; probe) { probe(1); } }", cppNameValue},
		{"while condition declaration", "void run() { while (int probe = 1) { probe(1); } }", cppNameValue},
		{"lambda parameter", "void run() { auto f = [](int probe) { probe(1); }; }", cppNameValue},
		{"outer parameter through a lambda", "void run(int probe) { auto f = [&]() { probe(1); }; }", cppNameValue},
		{"outer local through two lambdas", "void run() { int probe = 0; auto f = [&]() { auto g = [&]() { probe(1); }; }; }", cppNameValue},
		{"out-of-line member field", "class R { int probe; bool ok(); }; bool R::ok() { probe(1); }", cppNameValue},
		{"out-of-line member method", "class R { void probe(int); bool ok(); }; bool R::ok() { probe(1); }", cppNameCallable},
		{"out-of-line member of a namespaced class", "namespace ns { class R { int probe; bool ok(); }; } bool ns::R::ok() { probe(1); }", cppNameValue},
		{"out-of-line member of a class template", "template<typename T> class R { T probe; bool ok(); }; template<typename T> bool R<T>::ok() { probe(1); }", cppNameValue},
		{"out-of-line member through an in-file base", "struct B { int probe; }; struct D : B { bool ok(); }; bool D::ok() { probe(1); }", cppNameValue},
		{"out-of-line member through a templated in-file base", "template<typename T> struct B { T probe; }; struct D : B<int> { bool ok(); }; bool D::ok() { probe(1); }", cppNameValue},
		{"header-declared class is unresolved", "bool R::ok() { probe(1); }", cppNameUnresolved},
		{"qualified namespace variable", "namespace ns { int probe; } void run() { ns::probe(1); }", cppNameValue},
		{"qualified class static", "struct S { static constexpr int probe = 1; }; void run() { S::probe(1); }", cppNameConstant},
		{"qualified unknown scope", "void run() { std::probe(1); }", cppNameUnresolved},
		{"nested qualified namespace variable", "namespace ns { namespace inner { int probe; } } void run() { ns::inner::probe(1); }", cppNameValue},
		{"local declared after the use is not visible", "void run() { probe(1); int probe = 0; }", cppNameUnresolved},
		{"global declared after the use is not visible", "void run() { probe(1); } int probe;", cppNameUnresolved},
		{"sibling-block local is not visible", "void run() { { int probe = 0; } probe(1); }", cppNameUnresolved},
		{"innermost declaration wins", "void run(int probe) { constexpr int probe2 = 0; { int probe = 1; probe(1); } }", cppNameValue},
		{"innermost constant wins over an outer value", "int probe; void run() { constexpr int probe = 1; probe(1); }", cppNameConstant},
		{"macro or header name", "void run() { probe(1); }", cppNameUnresolved},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src + "\n")
			root := parseSourceFor(t, types.LangCpp, string(src))
			var ident *sitter.Node
			walkNamedChildren(root, true, func(node *sitter.Node) {
				if ident != nil || node.Type() != "call_expression" {
					return
				}
				walkNamedChildren(node.ChildByFieldName("function"), true, func(inner *sitter.Node) {
					if ident == nil && inner.Type() == "identifier" && nodeText(inner, src) == "probe" {
						ident = inner
					}
				})
				if fn := node.ChildByFieldName("function"); ident == nil && fn != nil && fn.Type() == "identifier" && nodeText(fn, src) == "probe" {
					ident = fn
				}
			})
			if ident == nil {
				t.Fatalf("%q: fixture must call `probe`", tc.src)
			}
			got := newCppScopeResolver(root, src).resolve(ident)
			if got != tc.want {
				t.Fatalf("%q: probe resolves to %s, want %s", tc.src, kindName[got], kindName[tc.want])
			}
		})
	}
}
