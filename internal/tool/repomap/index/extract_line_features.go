package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// extractLineFeatures walks the tree-sitter AST and records per-line
// typed LineFeature observations. Phase 6 stage 18 (2026-05-03)
// new-world replacement for the source-shape token tables that
// previously lived in explorer.go.
//
// Generic across languages — the per-grammar node-name mappings
// converge on tree-sitter conventions (return_statement,
// break_statement, call_expression, new_expression, arrow_function,
// composite_literal, etc.). Per-language quirks live in the
// switch below; unknown node types are simply skipped (no warnings,
// no fallback to byte tokens — the caller treats absence as "no
// signal").
//
// The returned map is keyed by 1-based source line number; each
// value is the deduplicated set of features observed at that line.
// nil/empty root returns nil so callers can fold the result into
// FileInfo.LineFeatures unconditionally.
func extractLineFeatures(root *sitter.Node, src []byte) map[int][]types.LineFeature {
	if root == nil {
		return nil
	}
	out := make(map[int][]types.LineFeature)
	add := func(line int, f types.LineFeature) {
		if line <= 0 {
			return
		}
		existing := out[line]
		for _, e := range existing {
			if e == f {
				return
			}
		}
		out[line] = append(existing, f)
	}
	walkLineFeatures(root, src, add)
	if len(out) == 0 {
		return nil
	}
	return out
}

// walkLineFeatures is the recursive AST visitor. Maps tree-sitter
// node Type() strings to the closed types.LineFeature enum.
//
// Mapping rules (closed list — every accepted node-name appears
// here verbatim, NOT a substring scan):
//
//   - return_statement / return_expression → LineFeatureReturnStmt
//   - break_statement → LineFeatureBreakStmt
//   - raise_statement (Python) → LineFeatureRaiseStmt
//   - throw_statement (Java/JS/TS) → LineFeatureThrowStmt
//   - call_expression / method_invocation → LineFeatureCallExpression
//   - new_expression / object_creation_expression → LineFeatureNewExpression
//   - arrow_function / lambda_expression → LineFeatureArrowFunction
//   - composite_literal (Go) / struct_expression (Rust) /
//     object (JS) → LineFeatureCompositeLiteral
//
// Adding a language tier means adding rows to this switch; no
// other site needs to change.
func walkLineFeatures(node *sitter.Node, src []byte, add func(int, types.LineFeature)) {
	if node == nil {
		return
	}
	line := int(node.StartPoint().Row) + 1
	switch node.Type() {
	// Phase 6 stage 27 (2026-05-03) — typed dynamic-dispatch
	// detection. Node-shape tier captures language constructs
	// that defeat static analysis regardless of identifier text:
	//   - Go's `defer_statement` (deferred side effects)
	//   - Rust's `unsafe_block`, `macro_invocation`
	//   - C / C++ `pointer_expression` (`->` / `*p`)
	// Identifier-tier dispatch detection (call_expression with
	// reserved callee like `getattr` / `Reflect.has` / `send`)
	// flows through the LineFeatureCallExpression branch below
	// + post-walk lookup against the language-specific dispatch
	// callee table.
	case "defer_statement":
		add(line, types.LineFeatureUnknownEffect)
	case "unsafe_block", "macro_invocation":
		add(line, types.LineFeatureUnknownEffect)
	case "pointer_expression":
		add(line, types.LineFeatureUnknownEffect)
	case "return_statement", "return_expression":
		add(line, types.LineFeatureReturnStmt)
	case "break_statement":
		add(line, types.LineFeatureBreakStmt)
	case "raise_statement":
		add(line, types.LineFeatureRaiseStmt)
	case "throw_statement", "throw_expression":
		add(line, types.LineFeatureThrowStmt)
	case "call_expression", "method_invocation",
		"call", "function_call":
		add(line, types.LineFeatureCallExpression)
		if isDynamicDispatchCall(node, src) {
			add(line, types.LineFeatureUnknownEffect)
		}
	case "new_expression", "object_creation_expression":
		add(line, types.LineFeatureNewExpression)
	case "arrow_function", "lambda_expression",
		"lambda", "closure_expression":
		add(line, types.LineFeatureArrowFunction)
	case "composite_literal", // Go
		"struct_expression", // Rust
		"object",            // JS object literal
		"object_pattern":    // TS / JS destructuring
		add(line, types.LineFeatureCompositeLiteral)
	// Phase 6 stage 28 (2026-05-03) — typed control-flow
	// guard detection. Maps every grammar's if/else/case/match/
	// guard/unless/until/switch node to LineFeatureGuard.
	case "if_statement", "if_expression",
		"else_clause", "else_if_clause", "elif_clause",
		"switch_statement", "switch_expression",
		"case_clause", "case_statement",
		"when_entry", "when_clause", "when_expression",
		"match_expression", "match_arm",
		"guard_statement",
		"unless_statement",
		"until_statement", "until_expression",
		"ternary_expression", "conditional_expression":
		add(line, types.LineFeatureGuard)
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkLineFeatures(node.NamedChild(i), src, add)
	}
}

// dynamicDispatchCallees is the closed enum of identifier-callee
// names that mark a call expression as dynamic dispatch / runtime
// reflection / unsafe escape across the supported languages.
// Phase 6 stage 27 (2026-05-03) typed-AST replacement for
// dataflow.detectUnknownEffect's per-language byte-token tables.
//
// The names are matched against the typed callee identifier
// extracted from the call_expression's `function` field — NOT
// against raw line text. This eliminates the false positives the
// retired byte scan suffered (a variable named "send" in a
// non-Ruby file matching the Ruby send dispatch).
var dynamicDispatchCallees = map[string]bool{
	// Python
	"getattr": true, "setattr": true, "importlib": true,
	// JS / TS / ArkTS
	"Reflect": true, "require": true, "eval": true,
	// Java / Kotlin (last-segment of qualified call)
	"forName": true, "newInstance": true,
	// Ruby
	"send": true, "public_send": true,
	"const_get": true, "method_missing": true,
	// Swift
	"Mirror": true, "NSClassFromString": true, "unsafeBitCast": true,
	// Lua
	"load": true, "loadfile": true, "loadstring": true,
	// Go reflect / unsafe pulled out of selector_expression
	"reflect": true, "unsafe": true,
}

// isDynamicDispatchCall reports whether a `call_expression` /
// `method_invocation` / `call` node's callee identifier appears
// in dynamicDispatchCallees. Walks the function field's children
// to extract every bare identifier (handles `obj.method(...)`,
// `pkg.fn(...)`, `Class.forName(...)` — both leading and trailing
// identifiers contribute, so any matching segment flags the call).
func isDynamicDispatchCall(node *sitter.Node, src []byte) bool {
	if node == nil {
		return false
	}
	fnNode := node.ChildByFieldName("function")
	if fnNode == nil {
		// Some grammars expose the callee as the first named
		// child (Lua's `function_call`). Try that as a fallback.
		if int(node.NamedChildCount()) > 0 {
			fnNode = node.NamedChild(0)
		}
	}
	if fnNode == nil {
		return false
	}
	for _, name := range collectCalleeIdentifiers(fnNode, src) {
		if dynamicDispatchCallees[name] {
			return true
		}
	}
	return false
}

// collectCalleeIdentifiers walks the function-position node and
// returns every bare identifier text encountered. Stops recursion
// at identifier-shaped leaves; non-identifier wrappers
// (selector_expression, qualified_identifier, member_expression)
// are descended into so their inner identifier(s) get collected.
func collectCalleeIdentifiers(node *sitter.Node, src []byte) []string {
	if node == nil {
		return nil
	}
	var out []string
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n == nil {
			return
		}
		switch n.Type() {
		case "identifier", "field_identifier", "type_identifier",
			"property_identifier", "name":
			text := strings.TrimSpace(n.Content(src))
			if text != "" {
				out = append(out, text)
			}
			return
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(node)
	return out
}
