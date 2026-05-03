package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
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
	case "identifier":
		// Python / JS / Java fallback path: bare `identifier` nodes
		// inside a return-type subtree are themselves the type
		// names (Python `def f() -> Foo:` has return_type → type
		// → identifier "Foo"; Java may emit `name` or `identifier`
		// for unqualified return types). Restrict to leaf
		// identifiers (no children) to avoid double-counting.
		if int(node.NamedChildCount()) > 0 {
			break
		}
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

// isFunctionNodeKind reports whether a tree-sitter node Type()
// represents a callable declaration (function / method / lambda)
// across the languages this repomap parses. Phase 6 stage 21
// (2026-05-03): used by backfillReturnTypeNames to find every
// function-like AST node so ReturnTypeNames can be populated for
// all 11 supported languages from a single post-pass, without
// touching each per-language extractor.
//
// The set is the union of conventional tree-sitter function-
// declaration node names across Go, Python, JS/TS, Java, Kotlin,
// Rust, C/C++, Ruby, Swift, Lua, and ArkTS (which rides TS
// grammar). Adding a new language tier means adding rows here.
func isFunctionNodeKind(t string) bool {
	switch t {
	// Go
	case "function_declaration", "method_declaration":
		return true
	// Python
	case "function_definition":
		return true
	// JS / TS / ArkTS
	case "function_expression", "arrow_function",
		"method_definition", "function_signature",
		"method_signature", "abstract_method_signature":
		return true
	// Java
	case "constructor_declaration":
		return true
	// Kotlin
	case "function_declaration_kotlin", "function":
		return true
	// Rust
	case "function_item", "function_signature_item":
		return true
	// C / C++
	case "function_definition_c", "function_declarator":
		return true
	// Ruby
	case "method", "singleton_method":
		return true
	// Swift
	case "function_declaration_swift",
		"protocol_function_declaration",
		"init_declaration", "deinit_declaration":
		return true
	// Lua
	case "function_declaration_lua", "function_definition_lua",
		"local_function":
		return true
	}
	return false
}

// backfillReturnTypeNames is the parser-level post-pass that
// populates Symbol.ReturnTypeNames for every function-like
// declaration in the AST. Phase 6 stage 21 (2026-05-03) approach:
// rather than thread the call into every per-language extractor,
// walk the AST once after extraction completes, find each
// function/method node via isFunctionNodeKind, extract its
// return-type names via extractReturnTypeNames, and match to the
// matching Symbol by line range.
//
// Symbol matching: the function node's StartPoint().Row+1
// matches the Symbol's Line (1-based). Strict equality — every
// per-language extractor records the function's Line as the
// declaration's start line, which is also the AST node's
// StartPoint. Slack-based matching is intentionally NOT used:
// when two symbols sit on adjacent lines (e.g. a class on L1
// and the first def on L2), slack causes the second symbol to
// inherit the first's slot and never receive its own
// ReturnTypeNames.
//
// Existing ReturnTypeNames (e.g. Go's inline wiring in
// goExtractFunc / goExtractMethod) is preserved — the post-pass
// only sets the field when it's currently empty.
func backfillReturnTypeNames(root *sitter.Node, src []byte, syms []types.Symbol) {
	if root == nil || len(syms) == 0 {
		return
	}
	byLine := make(map[int]int, len(syms))
	for i := range syms {
		if syms[i].Line <= 0 {
			continue
		}
		// Strict line-equality — first symbol at a given line wins.
		// Subsequent symbols at the same line (rare, only nested
		// functions on a single line) are matched by walking nested
		// children below.
		if _, occupied := byLine[syms[i].Line]; !occupied {
			byLine[syms[i].Line] = i
		}
	}
	walkFunctionNodes(root, src, syms, byLine)
}

func walkFunctionNodes(node *sitter.Node, src []byte, syms []types.Symbol, byLine map[int]int) {
	if node == nil {
		return
	}
	if isFunctionNodeKind(node.Type()) {
		startLine := int(node.StartPoint().Row) + 1
		if idx, ok := byLine[startLine]; ok {
			if len(syms[idx].ReturnTypeNames) == 0 {
				syms[idx].ReturnTypeNames = extractReturnTypeNames(node, src)
			}
		}
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkFunctionNodes(node.NamedChild(i), src, syms, byLine)
	}
}
