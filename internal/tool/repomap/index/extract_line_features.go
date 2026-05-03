package index

import (
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
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkLineFeatures(node.NamedChild(i), src, add)
	}
}
