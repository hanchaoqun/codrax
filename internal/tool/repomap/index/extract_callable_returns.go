package index

import (
	"sort"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	sitter "github.com/smacker/go-tree-sitter"
)

// extractCallableReturnExpressions consumes the already-parsed AST. Named
// callable/body matching is shared with body-presence annotation; anonymous
// callables and same-line declaration ambiguities never borrow an outer owner.
func extractCallableReturnExpressions(root *sitter.Node, source []byte, language string, symbols []types.Symbol) []types.CallableReturnExpression {
	type key struct {
		name       string
		start, end int
	}
	type observed struct {
		node, body *sitter.Node
		valid      bool
	}
	observations := make(map[key][]observed)
	var visit func(*sitter.Node, bool)
	visit = func(n *sitter.Node, bad bool) {
		if n == nil {
			return
		}
		bad = bad || n.Type() == "ERROR" || n.IsMissing()
		if name, start, end, presence, body, ok := callableBodyObservation(n, source, language); ok {
			k := key{name, start, end}
			observations[k] = append(observations[k], observed{n, body, !bad && !n.HasError() && presence == types.CallableBodyPresent})
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			visit(n.NamedChild(i), bad)
		}
	}
	visit(root, false)
	symbolCounts := make(map[key]int)
	for _, sym := range symbols {
		symbolCounts[key{sym.Name, sym.Line, sym.EndLine}]++
	}
	var out []types.CallableReturnExpression
	for _, sym := range symbols {
		if !sym.HasParserOwnedBody() {
			continue
		}
		obs := observations[key{sym.Name, sym.Line, sym.EndLine}]
		if len(obs) != 1 || !obs[0].valid {
			continue
		}
		if symbolCounts[key{sym.Name, sym.Line, sym.EndLine}] != 1 {
			continue
		}
		o := obs[0]
		add := func(expr *sitter.Node, kind types.CallableReturnKind) {
			if expr == nil || expr.IsMissing() || expr.HasError() || expr.EndByte() <= expr.StartByte() {
				return
			}
			r := types.CallableReturnExpression{CallableName: sym.Name, CallableReceiver: sym.Receiver, CallableParent: sym.Parent, CallableLine: sym.Line, CallableEndLine: sym.EndLine,
				Kind: kind, Expression: nodeText(expr, source), LineStart: nodeLine(expr), LineEnd: nodeEndLine(expr), StartByte: expr.StartByte(), EndByte: expr.EndByte(), Provenance: types.ProvenanceTreeSitter, ResolvedBy: "tree_sitter_callable_return"}
			if r.MatchesCallable(sym) {
				out = append(out, r)
			}
		}
		var returns func(*sitter.Node)
		returns = func(n *sitter.Node) {
			if n == nil || n.HasError() || n.IsMissing() || callableReturnBoundary(n.Type()) {
				return
			}
			if expr, ok := explicitCallableReturnValue(n, language); ok {
				add(expr, types.CallableReturnExplicit)
				return
			}
			for i := 0; i < int(n.NamedChildCount()); i++ {
				returns(n.NamedChild(i))
			}
		}
		returns(o.body)
		if (language == types.LangJavaScript || language == types.LangTypeScript || language == types.LangArkTS) && o.node.Type() == "arrow_function" && o.body.Type() != "statement_block" {
			add(o.body, types.CallableReturnExpressionBody)
		} else if language == types.LangRust {
			add(rustCallableTail(o.body), types.CallableReturnImplicitTail)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].StartByte != out[j].StartByte {
			return out[i].StartByte < out[j].StartByte
		}
		return out[i].CallableLine < out[j].CallableLine
	})
	return out
}

// These are grammar node identities, not source-token scans. Even an unnamed
// nested body is a boundary; missing a public Symbol must never make it outer.
func callableReturnBoundary(kind string) bool {
	switch kind {
	case "function_declaration", "method_declaration", "function_definition", "function_expression", "function", "generator_function", "generator_function_declaration", "arrow_function", "method_definition", "constructor_declaration", "function_item", "function_signature_item", "func_literal", "lambda_expression", "lambda", "closure_expression", "closure", "lambda_literal", "anonymous_function", "method", "singleton_method", "function_statement", "local_function", "async_block", "const_block", "class_definition", "class_declaration", "class", "impl_item", "trait_item", "mod_item", "computed_property", "computed_getter", "computed_setter", "getter", "setter":
		return true
	default:
		return false
	}
}

func explicitCallableReturnValue(n *sitter.Node, language string) (*sitter.Node, bool) {
	accepted := false
	switch n.Type() {
	case "return_statement", "return_expression":
		accepted = true
	case "return":
		accepted = language == types.LangRuby
	case "jump_expression":
		accepted = language == types.LangKotlin && callableReturnHasToken(n, "return")
	case "control_transfer_statement":
		accepted = language == types.LangSwift && callableReturnHasToken(n, "return")
	}
	if !accepted {
		return nil, false
	}
	// Return labels target another lexical scope. Only the unlabelled grammar
	// arm is supported here; do not reinterpret return@label as a function exit.
	var children []*sitter.Node
	for i := 0; i < int(n.NamedChildCount()); i++ {
		child := n.NamedChild(i)
		if child.Type() == "comment" || child.Type() == "line_comment" || child.Type() == "block_comment" {
			continue
		}
		children = append(children, child)
	}
	if len(children) != 1 {
		return nil, true
	}
	return children[0], true
}

func callableReturnHasToken(n *sitter.Node, token string) bool {
	for i := 0; i < int(n.ChildCount()); i++ {
		c := n.Child(i)
		if c != nil && !c.IsNamed() && c.Type() == token {
			return true
		}
	}
	return false
}

func rustCallableTail(body *sitter.Node) *sitter.Node {
	if body == nil || body.Type() != "block" || body.NamedChildCount() == 0 {
		return nil
	}
	var last *sitter.Node
	for i := 0; i < int(body.NamedChildCount()); i++ {
		n := body.NamedChild(i)
		if n.Type() != "line_comment" && n.Type() != "block_comment" {
			last = n
		}
	}
	if last == nil {
		return nil
	}
	if last.Type() == "expression_statement" {
		if callableReturnHasToken(last, ";") || last.NamedChildCount() != 1 {
			return nil
		}
		last = last.NamedChild(0)
	}
	switch last.Type() {
	case "return_expression", "let_declaration", "assignment_expression", "compound_assignment_expr", "empty_statement", "function_item", "struct_item", "enum_item", "type_item", "const_item", "static_item", "impl_item", "trait_item", "mod_item", "use_declaration", "macro_definition":
		return nil
	}
	return last
}
