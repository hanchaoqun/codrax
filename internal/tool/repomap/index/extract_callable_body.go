package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// backfillCallableBodyPresence records syntax, not behavior. A callable kind,
// its line count, a parent interface, or a missing call relation cannot prove
// whether a local body exists. Only an exact, error-free declaration node may
// supply that fact. Unknown grammar shapes and ambiguous same-line matches
// remain unknown. No source/model prose, modifiers, or name heuristics enter.
func backfillCallableBodyPresence(root *sitter.Node, source []byte, language string, symbols []types.Symbol) {
	if root == nil || len(source) == 0 || len(symbols) == 0 {
		return
	}
	type key struct {
		name       string
		start, end int
	}
	type observation struct {
		value              types.CallableBodyPresence
		count              int
		bodyStart, bodyEnd int
	}
	observations := make(map[key]observation)
	var walk func(*sitter.Node, bool)
	walk = func(node *sitter.Node, inError bool) {
		if node == nil {
			return
		}
		inError = inError || node.Type() == "ERROR" || node.IsMissing()
		if name, start, end, presence, body, ok := callableBodyObservation(node, source, language); ok {
			if inError || node.HasError() {
				presence = types.CallableBodyUnknown
			}
			k := key{name, start, end}
			old := observations[k]
			old.count++
			old.value = presence
			if presence == types.CallableBodyPresent && body != nil {
				old.bodyStart, old.bodyEnd = nodeLine(body), nodeEndLine(body)
			}
			observations[k] = old
		}
		for i := 0; i < int(node.NamedChildCount()); i++ {
			walk(node.NamedChild(i), inError)
		}
	}
	walk(root, false)
	for i := range symbols {
		symbol := &symbols[i]
		if !callableBodySymbolKind(symbol.Kind) {
			continue
		}
		observed := observations[key{symbol.Name, symbol.Line, symbol.EndLine}]
		if observed.count == 1 {
			symbol.BodyPresence = observed.value
			symbol.BodyStartLine, symbol.BodyEndLine = observed.bodyStart, observed.bodyEnd
		}
	}
}

func callableBodySymbolKind(kind string) bool {
	switch kind {
	case "function", "method", "ctor", "operator", "foreign-func", "builder", "ui-entry", "suspend-function", "extension-function":
		return true
	default:
		return false
	}
}

func callableBodyObservation(node *sitter.Node, source []byte, language string) (string, int, int, types.CallableBodyPresence, *sitter.Node, bool) {
	unknown := types.CallableBodyUnknown
	if node == nil {
		return "", 0, 0, unknown, nil, false
	}
	typ := node.Type()
	declaration := node
	var name string
	var body *sitter.Node
	allowAbsent, bodyRequired := false, false
	switch language {
	case types.LangC, types.LangCpp:
		if typ != "function_definition" && typ != "declaration" && typ != "field_declaration" {
			return "", 0, 0, unknown, nil, false
		}
		decl := node.ChildByFieldName("declarator")
		if decl == nil {
			decl = childByType(node, "function_declarator")
		}
		for decl != nil && (decl.Type() == "pointer_declarator" || decl.Type() == "reference_declarator" || decl.Type() == "parenthesized_declarator") {
			next := decl.ChildByFieldName("declarator")
			if next == nil && decl.NamedChildCount() == 1 {
				next = decl.NamedChild(0)
			}
			decl = next
		}
		if decl == nil || decl.Type() != "function_declarator" {
			return "", 0, 0, unknown, nil, false
		}
		if n := decl.ChildByFieldName("declarator"); n != nil {
			name = strings.TrimLeft(nodeText(n, source), "*& ")
		}
		body = node.ChildByFieldName("body")
		allowAbsent = typ != "function_definition" || childByType(node, "pure_virtual_clause") != nil ||
			childByType(node, "default_method_clause") != nil || childByType(node, "delete_method_clause") != nil
	case types.LangTypeScript, types.LangJavaScript, types.LangArkTS:
		switch typ {
		case "function_declaration", "generator_function_declaration", "function_expression", "function", "generator_function", "method_definition":
			body = node.ChildByFieldName("body")
			bodyRequired = true
		case "function_signature", "method_signature", "abstract_method_signature":
			allowAbsent = true
		case "arrow_function":
			body = node.ChildByFieldName("body")
			bodyRequired = true
		default:
			return "", 0, 0, unknown, nil, false
		}
		if n := node.ChildByFieldName("name"); n != nil {
			name = nodeText(n, source)
		}
		if name == "" && node.Parent() != nil && node.Parent().Type() == "variable_declarator" {
			parent := node.Parent()
			if value := parent.ChildByFieldName("value"); value != nil && value.Equal(node) {
				if n := parent.ChildByFieldName("name"); n != nil && n.Type() == "identifier" {
					name, declaration = nodeText(n, source), parent
				}
			}
		}
	case types.LangGo:
		if typ != "function_declaration" && typ != "method_declaration" {
			return "", 0, 0, unknown, nil, false
		}
		body, allowAbsent = node.ChildByFieldName("body"), true
	case types.LangJava:
		if typ != "method_declaration" && typ != "constructor_declaration" {
			return "", 0, 0, unknown, nil, false
		}
		body, allowAbsent = node.ChildByFieldName("body"), typ == "method_declaration"
	case types.LangRust:
		if typ != "function_item" && typ != "function_signature_item" {
			return "", 0, 0, unknown, nil, false
		}
		body, allowAbsent = node.ChildByFieldName("body"), typ == "function_signature_item"
	case types.LangPython:
		if typ != "function_definition" {
			return "", 0, 0, unknown, nil, false
		}
		body, bodyRequired = node.ChildByFieldName("body"), true
	case types.LangKotlin:
		if typ != "function_declaration" {
			return "", 0, 0, unknown, nil, false
		}
		name = firstKotlinIdentifier(node, source)
		body, allowAbsent = childByType(node, "function_body"), true
	case types.LangSwift:
		if typ != "function_declaration" && typ != "protocol_function_declaration" && typ != "init_declaration" && typ != "deinit_declaration" {
			return "", 0, 0, unknown, nil, false
		}
		if n := childByType(node, "simple_identifier"); n != nil {
			name = nodeText(n, source)
		}
		body = childByType(node, "function_body")
		allowAbsent = typ == "protocol_function_declaration"
	case types.LangRuby:
		if typ != "method" && typ != "singleton_method" {
			return "", 0, 0, unknown, nil, false
		}
		// An empty method has no body_statement. Its actual closing token
		// still bounds the empty body, never the preceding parameter header.
		bodyRequired = true
		body = node.ChildByFieldName("body")
		if body == nil {
			body = callableBodyClosingToken(node, "end")
		}
	case types.LangLua:
		if typ != "function_statement" {
			return "", 0, 0, unknown, nil, false
		}
		if symbol, ok := luaExtractFuncStmt(node, source, ""); ok {
			name = symbol.Name
		}
		bodyRequired, body = true, childByType(node, "function_body")
		if body == nil {
			body = childByType(node, "function_end")
		}
	default:
		return "", 0, 0, unknown, nil, false
	}
	if name == "" {
		if n := node.ChildByFieldName("name"); n != nil {
			name = nodeText(n, source)
		}
	}
	if name == "" {
		return "", 0, 0, unknown, nil, false
	}
	presence := unknown
	if body != nil && !body.IsMissing() && !body.HasError() {
		presence = types.CallableBodyPresent
	} else if allowAbsent && !bodyRequired && body == nil {
		presence = types.CallableBodyAbsent
	}
	return name, nodeLine(declaration), nodeEndLine(declaration), presence, body, true
}

func callableBodyClosingToken(node *sitter.Node, token string) *sitter.Node {
	if node == nil || node.ChildCount() == 0 {
		return nil
	}
	last := node.Child(int(node.ChildCount()) - 1)
	if last != nil && last.Type() == token && !last.IsMissing() {
		return last
	}
	return nil
}
