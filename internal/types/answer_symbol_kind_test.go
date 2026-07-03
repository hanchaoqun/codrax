package types

import (
	"strings"
	"testing"
)

func TestNormalizeAnswerSymbolKind_CanonicalKinds(t *testing.T) {
	// Every canonical kind must round-trip through the normaliser
	// unchanged. Guards against a kind being added to
	// AllAnswerSymbolKinds but forgotten in the validator.
	for _, k := range AllAnswerSymbolKinds {
		got, ok := NormalizeAnswerSymbolKind(string(k))
		if !ok {
			t.Errorf("canonical kind %q rejected by normaliser", k)
		}
		if got != k {
			t.Errorf("canonical %q normalised to %q", k, got)
		}
	}
}

func TestNormalizeAnswerSymbolKind_Aliases(t *testing.T) {
	cases := []struct {
		in   string
		want AnswerSymbolKind
	}{
		{"func", KindFunction},
		{"fn", KindFunction},
		{"FUNC", KindFunction},      // case-insensitive
		{"  method  ", KindMethod},  // whitespace tolerant
		{"Function", KindFunction},  // mixed case canonical
		{"struct_field", KindField}, // alias to field
	}
	for _, c := range cases {
		got, ok := NormalizeAnswerSymbolKind(c.in)
		if !ok {
			t.Errorf("%q should normalise, got rejected", c.in)
			continue
		}
		if got != c.want {
			t.Errorf("%q → %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeAnswerSymbolKind_Rejects(t *testing.T) {
	for _, in := range []string{"", "  ", "xyz", "functions", "meth", "Class ", "class x"} {
		if in == "Class " {
			continue // whitespace-trim test covered above
		}
		if _, ok := NormalizeAnswerSymbolKind(in); ok {
			t.Errorf("%q must be rejected", in)
		}
	}
}

func TestAllAnswerSymbolKinds_CoversMultiLanguageSurface(t *testing.T) {
	// Smoke test: every language-idiomatic kind we claim to support in
	// the doc comment must be reachable from AllAnswerSymbolKinds.
	// Prevents a cross-language kind being removed from the slice but
	// left in the documentation.
	must := []AnswerSymbolKind{
		KindFunction, KindMethod, // callables
		KindType, KindStruct, KindClass, KindInterface, KindTrait, KindEnum, KindProtocol, // type shapes
		KindConst, KindVar, KindField, KindProperty, // data
		KindModule, KindPackage, KindCrate, KindNamespace, // scopes
		KindMacro, KindDecorator, KindAnnotation, // metaprogramming
		KindLiteral, // non-symbol terminal
	}
	have := make(map[AnswerSymbolKind]bool, len(AllAnswerSymbolKinds))
	for _, k := range AllAnswerSymbolKinds {
		have[k] = true
	}
	for _, k := range must {
		if !have[k] {
			t.Errorf("declared canonical kind %q missing from AllAnswerSymbolKinds", k)
		}
	}
}

func TestAnswerSymbolKindSchemaEnum_RoundtripsThroughNormaliser(t *testing.T) {
	// Every token emitted into the JSON schema must be accepted by
	// NormalizeAnswerSymbolKind — otherwise the LLM produces a value
	// the schema validates but the Go validator rejects, surfacing as
	// a confusing "unknown kind" error after the tool call is already
	// in-flight. This test keeps the two sides welded.
	enum := AnswerSymbolKindSchemaEnum()
	parts := strings.Split(enum, ", ")
	if len(parts) < len(AllAnswerSymbolKinds) {
		t.Fatalf("schema enum has %d tokens, want ≥ %d (all canonical)", len(parts), len(AllAnswerSymbolKinds))
	}
	for _, p := range parts {
		unquoted := strings.Trim(p, `"`)
		if _, ok := NormalizeAnswerSymbolKind(unquoted); !ok {
			t.Errorf("schema-enum token %q not accepted by normaliser", unquoted)
		}
	}
}

func TestAnswerSymbolKindSchemaEnum_DeterministicOrder(t *testing.T) {
	// Two calls back-to-back must produce identical strings. Guards
	// against map-iteration-randomness leaking into the alias segment
	// of the schema string (which would show up as a schema diff on
	// every process restart).
	a := AnswerSymbolKindSchemaEnum()
	b := AnswerSymbolKindSchemaEnum()
	if a != b {
		t.Errorf("schema enum not deterministic:\n  first : %s\n  second: %s", a, b)
	}
}
