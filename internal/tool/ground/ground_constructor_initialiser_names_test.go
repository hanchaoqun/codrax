package ground

import "testing"

// ground_constructor_initialiser_names_test.go — live-only unit pin for
// the constructor member-initialiser form's name-agreement signal (§40.59
// 收编复核三轮 #4); the behavioural matrix that also runs against both bases
// is ground_definition_shape_scope_test.go.

// The constructor form's name-agreement signal is a pure function of the
// regex's two capture groups.
func TestConstructorInitialiserNamesAgreeReadsTheTwoGroups(t *testing.T) {
	for _, tc := range []struct {
		line string
		want bool
	}{
		{"Parser::Parser(int x) : x_(x) {", true},
		{"Parser<T>::Parser(int x) : x_(x) {", true},
		{"ns::Parser::Parser(int x) : x_(x) {", true},
		{"template<typename T> Parser<T>::Parser(int x) : x_(x) {", true},
		{"Derived::Derived(int x) : Base<int>(x) {", true},
		{"Foo::bar(x) : Foo::baz(y);", false},
		{"Foo::bar(x) : baz(y) {", false},
	} {
		groups := constructorInitialiserDefinitionLineRe.FindStringSubmatch(tc.line)
		if groups == nil {
			t.Fatalf("%q must match the constructor regex textually", tc.line)
		}
		if got := constructorInitialiserNamesAgree(groups); got != tc.want {
			t.Fatalf("%q: names agree=%t, want %t (groups=%q)", tc.line, got, tc.want, groups[1:3])
		}
	}
	if constructorInitialiserNamesAgree(nil) || constructorInitialiserNamesAgree([]string{"", "", ""}) {
		t.Fatal("empty groups never agree")
	}
}
