package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// extractNavigationCalls covers the shared call-expression surface used by
// the Kotlin and Swift tree-sitter grammars. Both grammars represent
// receiver.method(...) as call_expression(navigation_expression(...)). The
// helper preserves the receiver expression instead of collapsing every call
// to a bare operation name; downstream graph resolution may later promote a
// uniquely typed receiver, while dynamic or ambiguous expressions remain
// bounded to their source identity.
func extractNavigationCalls(root *sitter.Node, src []byte, file, resolvedBy string) []types.Relation {
	var rels []types.Relation
	receiverTypes := navigationUniqueReceiverTypeBindings(root, src)
	walkNamedChildren(root, true, func(node *sitter.Node) {
		if node.Type() != "call_expression" || node.NamedChildCount() == 0 {
			return
		}
		fn := node.NamedChild(0)
		name, receiver := navigationCallTarget(fn, src)
		if name == "" {
			return
		}
		if binding := navigationReceiverBinding(receiver); binding != "" && receiverTypes[binding] != "" {
			receiver = receiverTypes[binding]
		}
		line := nodeLine(fn)
		rels = append(rels, types.Relation{
			Kind:       "call",
			FromEP:     types.RelationEndpoint{File: file, Line: line},
			ToEP:       types.RelationEndpoint{Name: name, Receiver: receiver, File: file, Line: line},
			File:       file,
			Line:       line,
			Confidence: types.ConfidenceAST,
			Provenance: types.ProvenanceTreeSitter,
			ResolvedBy: resolvedBy,
		})
	})
	return rels
}

func navigationUniqueReceiverTypeBindings(root *sitter.Node, src []byte) map[string]string {
	bindings := make(map[string]string)
	conflicts := make(map[string]bool)
	add := func(name, typeName string) {
		name = strings.TrimSpace(name)
		typeName = strings.TrimSpace(typeName)
		if name == "" || typeName == "" || conflicts[name] {
			return
		}
		if have := bindings[name]; have != "" && have != typeName {
			delete(bindings, name)
			conflicts[name] = true
			return
		}
		bindings[name] = typeName
	}
	walkNamedChildren(root, true, func(node *sitter.Node) {
		switch node.Type() {
		case "parameter", "class_parameter", "property_declaration":
			name := navigationBindingName(node, src)
			typeName := navigationDeclaredTypeName(node, src)
			add(name, typeName)
		}
	})
	return bindings
}

func navigationBindingName(node *sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	if name := node.ChildByFieldName("name"); name != nil &&
		(name.Type() == "simple_identifier" || name.Type() == "identifier") {
		return strings.TrimSpace(nodeText(name, src))
	}
	var found string
	walkNamedChildren(node, true, func(ch *sitter.Node) {
		if found == "" && (ch.Type() == "simple_identifier" || ch.Type() == "identifier") {
			found = strings.TrimSpace(nodeText(ch, src))
		}
	})
	return found
}

func navigationDeclaredTypeName(node *sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	if typed := node.ChildByFieldName("type"); typed != nil {
		return navigationTypeIdentifier(typed, src)
	}
	// Kotlin exposes declaration types through direct grammar carriers rather
	// than a stable `type` field. Stay inside those carriers: walking the whole
	// declaration also sees annotation arguments and initializer expressions,
	// whose first type_identifier is not the binding's declared type.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "user_type", "type_annotation", "nullable_type", "type_identifier":
			if typeName := navigationTypeIdentifier(child, src); typeName != "" {
				return typeName
			}
		case "parameter_with_optional_type", "variable_declaration":
			if typeName := navigationDeclaredTypeName(child, src); typeName != "" {
				return typeName
			}
		}
	}
	return ""
}

func navigationTypeIdentifier(node *sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	if node.Type() == "type_identifier" {
		return strings.TrimSpace(nodeText(node, src))
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		if typeName := navigationTypeIdentifier(node.NamedChild(i), src); typeName != "" {
			return typeName
		}
	}
	return ""
}

func navigationReceiverBinding(receiver string) string {
	receiver = strings.TrimSpace(receiver)
	if idx := strings.LastIndex(receiver, "."); idx >= 0 {
		receiver = receiver[idx+1:]
	}
	return strings.TrimSpace(receiver)
}

func navigationCallTarget(fn *sitter.Node, src []byte) (name, receiver string) {
	if fn == nil {
		return "", ""
	}
	switch fn.Type() {
	case "simple_identifier", "identifier", "type_identifier":
		return strings.TrimSpace(nodeText(fn, src)), ""
	case "navigation_expression":
		target := fn.ChildByFieldName("target")
		if target == nil && fn.NamedChildCount() > 0 {
			target = fn.NamedChild(0)
		}
		suffix := fn.ChildByFieldName("suffix")
		if suffix == nil {
			suffix = childByType(fn, "navigation_suffix")
		}
		if target == nil || suffix == nil {
			return "", ""
		}
		var nameNode *sitter.Node
		walkNamedChildren(suffix, true, func(ch *sitter.Node) {
			if ch.Type() == "simple_identifier" || ch.Type() == "identifier" || ch.Type() == "type_identifier" {
				nameNode = ch
			}
		})
		if nameNode == nil {
			return "", ""
		}
		return strings.TrimSpace(nodeText(nameNode, src)), strings.TrimSpace(nodeText(target, src))
	}
	return "", ""
}
