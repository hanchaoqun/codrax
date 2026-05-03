package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// extractReturnTypeNames walks the function/method node and
// returns the deduplicated set of bare type-name tokens parsed
// from its return signature. Phase 6 stage 20 (2026-05-03)
// new-world replacement for the retired IdentifierFactoryPrefixes
// heuristic — by reading the return type structurally, naming
// conventions (`New_`/`Create_`/`Make_`) become irrelevant.
//
// Generic across languages — relies on tree-sitter's conventional
// node names (return_type / type_identifier / generic_type /
// pointer_type / array_type / slice_type / qualified_identifier
// etc.). The unwrap-and-collect rule below is grammar-agnostic:
// walk every descendant, collect each `type_identifier` /
// `primitive_type` text, dedupe.
//
// nil/non-function nodes return nil so callers can fold the result
// unconditionally into Symbol.ReturnTypeNames.
//
// Per-language quirks (return-type position differs by grammar):
//
//   - Go (`function_declaration`/`method_declaration`): result child
//     under field name `result` — single param, parameter_list, or
//     parenthesized_type list.
//   - Java/Kotlin/C/C++ (`method_declaration` etc.): return type
//     appears as `type` field BEFORE the function name child.
//   - Python (`function_definition`): optional `return_type` field.
//   - Rust (`function_item`): `return_type` field after `->`.
//   - JS/TS (`method_definition`/`function_declaration`): TS adds
//     `return_type` field; pure JS has none.
//
// The implementation does NOT field-lookup per language; it walks
// the entire node subtree and collects every type identifier
// reachable. Function PARAMETER types are also collected — but
// for the immediate consumer (containsIdentifier's factory match)
// this is fine: a function whose parameters mention `Foo` AND
// whose body returns nothing is not normally called as a factory,
// but cross-validate matching is conservative on this axis.
//
// To restrict to TRUE return types (not parameters), per-language
// helpers extract the specific return-type subtree and call
// collectTypeNames only on that. The default extractReturnTypeNames
// caller below uses the conservative whole-signature scan; per-
// language wrappers refine.
func extractReturnTypeNames(node *sitter.Node, src []byte) []string {
	if node == nil {
		return nil
	}
	// Find the return-type subtree by field name (Go/Rust/Python/TS
	// all use "result" or "return_type"). Falls back to nil if
	// neither is present, in which case the function has no return
	// type (void / procedure / JS untyped).
	var resultNode *sitter.Node
	for _, field := range []string{"result", "return_type", "type"} {
		if n := node.ChildByFieldName(field); n != nil {
			resultNode = n
			break
		}
	}
	if resultNode == nil {
		// Fallback for grammars that don't expose a return-type
		// field: pre-name `type` for Java/C/C++. Look at the first
		// type-shaped child before the identifier child.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			ch := node.NamedChild(i)
			if ch == nil {
				continue
			}
			t := ch.Type()
			if t == "identifier" || t == "name" || t == "field_identifier" {
				break
			}
			if isReturnTypeShape(t) {
				resultNode = ch
				break
			}
		}
	}
	if resultNode == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	collectTypeNames(resultNode, src, seen, &out)
	if len(out) == 0 {
		return nil
	}
	return out
}

// collectTypeNames walks `node` and appends every `type_identifier`
// (or grammar equivalent) text to `out`, deduplicated via `seen`.
// Wrapping nodes (pointer_type / array_type / slice_type /
// generic_type / qualified_identifier / parenthesized_type /
// reference_type / parameter_list / parameter / parameter_declaration)
// are recursed into; their inner type identifier(s) get collected.
//
// Cross-language type-identifier node names this recognises:
//
//   - type_identifier (Go, Rust, Java, JS/TS, ArkTS, Cangjie)
//   - primitive_type (Go, C, C++, Java)
//   - identifier (Python — return annotations are bare names)
//   - simple_type (Kotlin)
//   - user_type (Kotlin)
//   - name (Java sometimes for fully-qualified types)
//   - scoped_type_identifier (Rust Foo::Bar — collect last segment)
//   - generic_type / type_arguments — recursed
//   - qualified_identifier — recursed
//
// Result: for `*Foo` / `[]Foo` / `Box<Foo>` / `Result<Foo, E>` the
// emitted set is `{Foo}` or `{Foo, E}`; for `(error, *Foo)` it's
// `{error, Foo}`.
func collectTypeNames(node *sitter.Node, src []byte, seen map[string]bool, out *[]string) {
	if node == nil {
		return
	}
	switch node.Type() {
	case "type_identifier", "primitive_type", "simple_type",
		"user_type", "predefined_type":
		text := strings.TrimSpace(node.Content(src))
		text = stripTypeWrappers(text)
		if text != "" && !seen[text] {
			seen[text] = true
			*out = append(*out, text)
		}
		return
	case "scoped_type_identifier", "qualified_type":
		// Rust `Foo::Bar` / Java `pkg.Foo` — last segment is the
		// canonical type name; recurse to find it.
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		collectTypeNames(node.NamedChild(i), src, seen, out)
	}
}

// stripTypeWrappers removes punctuation that survives the typed
// extraction (e.g. for grammars whose `type_identifier` includes
// trailing whitespace or parameter-list noise). Keeps the bare
// alphanumeric identifier — that's what containsIdentifier
// compares against the search target.
func stripTypeWrappers(s string) string {
	s = strings.TrimSpace(s)
	for strings.HasPrefix(s, "*") || strings.HasPrefix(s, "&") {
		s = strings.TrimSpace(s[1:])
	}
	for strings.HasSuffix(s, "?") || strings.HasSuffix(s, "!") {
		s = strings.TrimSpace(s[:len(s)-1])
	}
	if idx := strings.Index(s, "<"); idx > 0 {
		s = strings.TrimSpace(s[:idx])
	}
	if idx := strings.Index(s, "["); idx > 0 {
		s = strings.TrimSpace(s[:idx])
	}
	if idx := strings.Index(s, "("); idx > 0 {
		s = strings.TrimSpace(s[:idx])
	}
	if idx := strings.LastIndex(s, "."); idx > 0 {
		s = strings.TrimSpace(s[idx+1:])
	}
	if idx := strings.LastIndex(s, ":"); idx > 0 {
		s = strings.TrimSpace(s[idx+1:])
	}
	return s
}

// isReturnTypeShape reports whether a tree-sitter node Type()
// string looks like a type-bearing node (vs. modifiers, bodies,
// or names). Used by extractReturnTypeNames as a fallback when no
// return-type field is exposed by the grammar (Java/C/C++).
func isReturnTypeShape(t string) bool {
	switch t {
	case "type_identifier", "primitive_type", "simple_type",
		"user_type", "predefined_type", "void_type",
		"pointer_type", "array_type", "slice_type",
		"generic_type", "qualified_type", "reference_type",
		"scoped_type_identifier", "type", "return_type":
		return true
	}
	return false
}
