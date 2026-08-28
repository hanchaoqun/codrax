package index

import (
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// backfillCallableParameterBindings preserves syntax-explicit parameter-name
// to static-type bindings on callable symbols.  The post-pass is deliberately
// cross-language and fail-closed: an untyped, destructured, default-only, or
// ambiguous parameter publishes no binding.  Consumers may use these rows for
// endpoint identity alignment, but never as relation or execution authority.
func backfillCallableParameterBindings(root *sitter.Node, src []byte, syms []types.Symbol) {
	if root == nil || len(src) == 0 || len(syms) == 0 {
		return
	}
	var walk func(*sitter.Node)
	walk = func(node *sitter.Node) {
		if node == nil {
			return
		}
		if isFunctionNodeKind(node.Type()) {
			bindings := callableParameterBindings(node, src)
			if len(bindings) > 0 {
				attachCallableParameterBindings(syms, int(node.StartPoint().Row)+1, int(node.EndPoint().Row)+1, bindings)
			}
		}
		for i := 0; i < int(node.NamedChildCount()); i++ {
			walk(node.NamedChild(i))
		}
	}
	walk(root)
}

func callableParameterBindings(callable *sitter.Node, src []byte) []types.CallableParameterBinding {
	container := callable.ChildByFieldName("parameters")
	if container == nil {
		container = firstCallableParameterContainer(callable)
	}
	var params []*sitter.Node
	if container != nil {
		collectTypedParameterNodes(container, &params)
	} else {
		// Swift's grammar places `parameter` nodes directly on the callable
		// without a parameter-list wrapper. Keep this fallback direct-child only
		// so catch/lambda parameters inside the body cannot leak outward.
		for i := 0; i < int(callable.NamedChildCount()); i++ {
			child := callable.NamedChild(i)
			if child != nil && isTypedParameterNodeKind(child.Type()) {
				params = append(params, child)
			}
		}
	}
	if len(params) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var out []types.CallableParameterBinding
	for _, param := range params {
		for _, binding := range typedParameterBindings(param, src) {
			key := binding.Binding + "\x00" + binding.Type
			if binding.Binding == "" || binding.Type == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, binding)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Binding != out[j].Binding {
			return out[i].Binding < out[j].Binding
		}
		return out[i].Type < out[j].Type
	})
	return out
}

func firstCallableParameterContainer(callable *sitter.Node) *sitter.Node {
	if callable == nil {
		return nil
	}
	var found *sitter.Node
	var walk func(*sitter.Node)
	walk = func(node *sitter.Node) {
		if node == nil || found != nil {
			return
		}
		if node != callable && isFunctionNodeKind(node.Type()) {
			return
		}
		switch node.Type() {
		case "parameter_list", "formal_parameters", "parameters", "function_value_parameters", "lambda_parameters":
			found = node
			return
		}
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil && child.Type() == "block" {
				continue
			}
			walk(child)
		}
	}
	walk(callable)
	return found
}

func collectTypedParameterNodes(node *sitter.Node, out *[]*sitter.Node) {
	if node == nil {
		return
	}
	if isTypedParameterNodeKind(node.Type()) {
		*out = append(*out, node)
		return
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child != nil && !isFunctionNodeKind(child.Type()) {
			collectTypedParameterNodes(child, out)
		}
	}
}

func isTypedParameterNodeKind(kind string) bool {
	switch kind {
	case "parameter_declaration", "variadic_parameter_declaration",
		"formal_parameter", "spread_parameter", "catch_formal_parameter",
		"required_parameter", "optional_parameter", "rest_parameter",
		"parameter", "variadic_parameter", "simple_parameter",
		"typed_parameter", "default_parameter", "lambda_parameter":
		return true
	default:
		return false
	}
}

func typedParameterBindings(param *sitter.Node, src []byte) []types.CallableParameterBinding {
	if param == nil {
		return nil
	}
	typeNode := parameterTypeNode(param)
	if typeNode == nil {
		return nil
	}
	typeText := cleanParameterTypeText(nodeText(typeNode, src))
	if typeText == "" {
		return nil
	}
	nameRoot := firstNonNilNode(
		param.ChildByFieldName("name"),
		param.ChildByFieldName("pattern"),
		param.ChildByFieldName("declarator"),
	)
	var names []string
	if nameRoot != nil {
		collectParameterBindingNames(nameRoot, typeNode, src, &names)
	}
	if len(names) == 0 {
		// Go permits one declaration to bind several direct identifiers.
		// Swift also exposes both the external/local label and user type through
		// repeated `name` fields. Restrict this fallback to direct children
		// outside the typed subtree; recursive fallback would mistake qualified
		// type names for parameters.
		for i := 0; i < int(param.NamedChildCount()); i++ {
			child := param.NamedChild(i)
			if child == nil || syntaxNodeContains(typeNode, child) || syntaxNodeContains(child, typeNode) {
				continue
			}
			if isParameterBindingNameNode(child.Type()) {
				names = append(names, strings.TrimSpace(nodeText(child, src)))
			}
		}
	}
	seen := make(map[string]bool)
	out := make([]types.CallableParameterBinding, 0, len(names))
	for _, name := range names {
		name = cleanParameterBindingName(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, types.CallableParameterBinding{Binding: name, Type: typeText})
	}
	return out
}

func parameterTypeNode(param *sitter.Node) *sitter.Node {
	for _, field := range []string{"type", "type_annotation"} {
		if node := param.ChildByFieldName(field); node != nil {
			return node
		}
	}
	for i := 0; i < int(param.NamedChildCount()); i++ {
		child := param.NamedChild(i)
		if child != nil && isParameterTypeNodeKind(child.Type()) {
			return child
		}
	}
	return nil
}

func isParameterTypeNodeKind(kind string) bool {
	switch kind {
	case "type_annotation", "type_identifier", "primitive_type", "predefined_type",
		"simple_type", "user_type", "reference_type", "pointer_type",
		"array_type", "slice_type", "generic_type", "qualified_type",
		"scoped_type_identifier", "nullable_type", "function_type", "type":
		return true
	default:
		return false
	}
}

func collectParameterBindingNames(node, typeNode *sitter.Node, src []byte, out *[]string) {
	if node == nil || syntaxNodeContains(typeNode, node) {
		return
	}
	if isParameterBindingNameNode(node.Type()) && node.NamedChildCount() == 0 {
		*out = append(*out, strings.TrimSpace(nodeText(node, src)))
		return
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		collectParameterBindingNames(node.NamedChild(i), typeNode, src, out)
	}
}

func isParameterBindingNameNode(kind string) bool {
	switch kind {
	case "identifier", "field_identifier", "property_identifier", "simple_identifier", "variable_name":
		return true
	default:
		return false
	}
}

func syntaxNodeContains(outer, inner *sitter.Node) bool {
	return outer != nil && inner != nil && outer.StartByte() <= inner.StartByte() && outer.EndByte() >= inner.EndByte()
}

func firstNonNilNode(nodes ...*sitter.Node) *sitter.Node {
	for _, node := range nodes {
		if node != nil {
			return node
		}
	}
	return nil
}

func cleanParameterTypeText(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSpace(strings.TrimPrefix(raw, ":"))
	if raw == "" || raw == "_" {
		return ""
	}
	return raw
}

func cleanParameterBindingName(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "...")
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "mut "))
	raw = strings.TrimSpace(strings.TrimLeft(raw, "*&"))
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '$':
		default:
			return ""
		}
	}
	return raw
}

func attachCallableParameterBindings(syms []types.Symbol, startLine, endLine int, bindings []types.CallableParameterBinding) {
	best := -1
	bestSpan := int(^uint(0) >> 1)
	for i := range syms {
		if syms[i].Kind != "function" && syms[i].Kind != "method" && syms[i].Kind != "constructor" && syms[i].Kind != "ctor" && syms[i].Kind != "operator" {
			continue
		}
		end := syms[i].EndLine
		if end < syms[i].Line {
			end = syms[i].Line
		}
		if startLine < syms[i].Line || startLine > end || (syms[i].Line != startLine && endLine > end) {
			continue
		}
		span := end - syms[i].Line
		if best < 0 || syms[i].Line == startLine && syms[best].Line != startLine || span < bestSpan {
			best, bestSpan = i, span
		}
	}
	if best < 0 {
		return
	}
	seen := make(map[string]bool)
	for _, existing := range syms[best].ParameterBindings {
		seen[existing.Binding+"\x00"+existing.Type] = true
	}
	for _, binding := range bindings {
		key := binding.Binding + "\x00" + binding.Type
		if !seen[key] {
			seen[key] = true
			syms[best].ParameterBindings = append(syms[best].ParameterBindings, binding)
		}
	}
	sort.Slice(syms[best].ParameterBindings, func(i, j int) bool {
		if syms[best].ParameterBindings[i].Binding != syms[best].ParameterBindings[j].Binding {
			return syms[best].ParameterBindings[i].Binding < syms[best].ParameterBindings[j].Binding
		}
		return syms[best].ParameterBindings[i].Type < syms[best].ParameterBindings[j].Type
	})
}
