package types

import "strings"

// AnswerSymbolKind is the closed taxonomy of "what kind of thing is
// this answer symbol". Consumed by emit_answer_symbol and
// emit_answer_document as the `kind` field's closed enum, and
// rendered verbatim in the user-visible answer (e.g. `Foo
// (a.go:12) [function]`).
//
// Design principles:
//
//  1. Cross-language. codrax analyses Go, Python, Java, JS/TS, Rust,
//     C/C++, Swift, Ruby, and more. A Go-centric set (just function /
//     method / struct / interface / type / const / var) forces the
//     LLM to mis-label symbols from other languages — a Python
//     `@decorator` is not a "function", a Rust `trait` is not an
//     "interface", a Ruby `module` is not a "package". Every
//     language-idiomatic kind the LLM might legitimately name gets
//     its own slot.
//
//  2. Non-symbol terminals get a first-class slot. KindLiteral is
//     for list_of_symbols answers whose terminal is a value, not a
//     code identifier — e.g. the string `"explorer"` returned by an
//     agent's Name() method, a config key's literal default, an
//     enum member's value. Without this slot, the LLM that correctly
//     walked a resolution chain to its literal terminal has no legal
//     kind and silently retreats to emitting the chain's mechanism
//     node under kind=function instead.
//
//  3. Alias normalisation. Common language shorthands (Go `func`,
//     Rust `fn`, Python `def`) are accepted on input and normalised
//     to their canonical kind. The alias list is deliberately narrow
//     — only unambiguous shorthands, never a typo fix.
type AnswerSymbolKind string

const (
	// Callables — functions and member/receiver-bound functions.
	KindFunction AnswerSymbolKind = "function" // Go func, Python def, JS function, Rust fn
	KindMethod   AnswerSymbolKind = "method"   // receiver-bound / class-member callable

	// Type-shape definitions — cross-language taxonomy.
	KindType      AnswerSymbolKind = "type"      // Go type alias, TS type alias, Rust type alias
	KindStruct    AnswerSymbolKind = "struct"    // Go, Rust, C++, Swift
	KindClass     AnswerSymbolKind = "class"     // Python, Java, JS/TS, C++, Ruby, Swift, Kotlin
	KindInterface AnswerSymbolKind = "interface" // Go, Java, TS, C# (distinct from Rust trait)
	KindTrait     AnswerSymbolKind = "trait"     // Rust, Scala
	KindEnum      AnswerSymbolKind = "enum"      // most languages with nominal enums
	KindProtocol  AnswerSymbolKind = "protocol"  // Swift, Python typing.Protocol — distinct from interface

	// Data-level — named storage.
	KindConst    AnswerSymbolKind = "const"    // immutable binding
	KindVar      AnswerSymbolKind = "var"      // mutable binding
	KindField    AnswerSymbolKind = "field"    // struct / class member data
	KindProperty AnswerSymbolKind = "property" // JS/TS getter/setter, Swift property, Python @property

	// Module-level scope.
	KindModule  AnswerSymbolKind = "module"  // Python, Node, Rust, Ruby
	KindPackage AnswerSymbolKind = "package" // Go, Java, npm
	KindCrate   AnswerSymbolKind = "crate"   // Rust root compilation unit
	KindNamespace AnswerSymbolKind = "namespace" // C#, C++, TS

	// Metaprogramming / reflection-surface constructs.
	KindMacro      AnswerSymbolKind = "macro"      // Rust, C/C++ preprocessor, Elixir
	KindDecorator  AnswerSymbolKind = "decorator"  // Python, TS decorators
	KindAnnotation AnswerSymbolKind = "annotation" // Java, Kotlin

	// Non-symbol terminal — the answer IS a literal value, not an
	// identifier. Use when the resolution chain resolves to a string,
	// number, boolean, nil, or enum-member literal rather than a code
	// reference the user could `go to definition` on.
	KindLiteral AnswerSymbolKind = "literal"
)

// AllAnswerSymbolKinds is the canonical ordered list used by the
// tool-schema enum generator (AnswerSymbolKindSchemaEnum) and the
// validator (NormalizeAnswerSymbolKind). Order is:
//
//  1. Callables (most common answer shape)
//  2. Type-shape definitions (cross-language)
//  3. Data bindings
//  4. Module-level scopes
//  5. Metaprogramming constructs
//  6. Literal (last so the LLM's default-first bias doesn't pick it)
//
// The order is stable — never reordered without simultaneously
// updating the schema enum expected by existing tool call validators.
var AllAnswerSymbolKinds = []AnswerSymbolKind{
	KindFunction, KindMethod,
	KindType, KindStruct, KindClass, KindInterface, KindTrait, KindEnum, KindProtocol,
	KindConst, KindVar, KindField, KindProperty,
	KindModule, KindPackage, KindCrate, KindNamespace,
	KindMacro, KindDecorator, KindAnnotation,
	KindLiteral,
}

// answerSymbolKindAliases maps language-specific shorthand the LLM
// might emit (per its training on that language's idioms) to the
// canonical kind. Narrow by design — only unambiguous shorthands
// whose meaning has ONE mapping. Keyword "def" alone is ambiguous
// across languages (Python def is function; Ruby def is method); we
// do not include it.
var answerSymbolKindAliases = map[string]AnswerSymbolKind{
	"func":         KindFunction, // Go, Swift shorthand
	"fn":           KindFunction, // Rust shorthand
	"struct_field": KindField,    // some extractors emit this
}

// NormalizeAnswerSymbolKind coerces an LLM-emitted kind string to
// the canonical AnswerSymbolKind. Returns (kind, true) on match;
// (empty, false) when the input is neither a canonical kind nor an
// alias. The validator uses the boolean to reject unknown kinds with
// a clear error rather than silently coerce to a default.
//
// Matching is case-insensitive and strips surrounding whitespace;
// anything else is treated as a deliberate LLM error.
func NormalizeAnswerSymbolKind(raw string) (AnswerSymbolKind, bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return "", false
	}
	if alias, ok := answerSymbolKindAliases[key]; ok {
		return alias, true
	}
	for _, k := range AllAnswerSymbolKinds {
		if string(k) == key {
			return k, true
		}
	}
	return "", false
}

// AnswerSymbolKindSchemaEnum returns the comma-separated JSON-array-
// element list of accepted kinds, suitable for embedding in a tool
// schema's `"enum": [...]` field. Both canonical kinds and aliases
// are included so the schema layer at the LLM boundary accepts any
// shorthand the LLM might produce; the Go-side validator then
// normalises to the canonical spelling.
//
// Returned as one pre-built string to avoid per-tool-call allocation
// and to make the schema JSON body a single string literal that
// reads cleanly alongside the rest of the schema.
func AnswerSymbolKindSchemaEnum() string {
	parts := make([]string, 0, len(AllAnswerSymbolKinds)+len(answerSymbolKindAliases))
	for _, k := range AllAnswerSymbolKinds {
		parts = append(parts, `"`+string(k)+`"`)
	}
	// Aliases emitted in a deterministic order so a schema diff on
	// upgrade stays readable. Map iteration order is randomised by
	// Go, so we materialise into a slice and sort.
	aliasKeys := make([]string, 0, len(answerSymbolKindAliases))
	for k := range answerSymbolKindAliases {
		aliasKeys = append(aliasKeys, k)
	}
	sortStrings(aliasKeys)
	for _, a := range aliasKeys {
		parts = append(parts, `"`+a+`"`)
	}
	return strings.Join(parts, ", ")
}

// sortStrings is a tiny local insertion sort so this file does not
// pull in the "sort" package just for a handful of alias keys.
func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j-1] > xs[j]; j-- {
			xs[j-1], xs[j] = xs[j], xs[j-1]
		}
	}
}
