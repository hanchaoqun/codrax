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
	receiverDeclarations := navigationReceiverDeclarations(root, src)
	walkNamedChildren(root, true, func(node *sitter.Node) {
		if node.Type() != "call_expression" || node.NamedChildCount() == 0 {
			return
		}
		fn := node.NamedChild(0)
		name, receiver := navigationCallTarget(fn, src)
		if name == "" {
			return
		}
		if binding := navigationReceiverBinding(receiver); binding != "" {
			if receiverType, declared := navigationReceiverTypeAt(node, binding, receiverDeclarations); declared && receiverType != "" {
				receiver = receiverType
			}
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

type navigationScopeKey struct {
	nodeType  string
	startByte uint32
	endByte   uint32
}

type navigationReceiverAuthority struct {
	typeName   string
	conflicted bool
}

type navigationReceiverAuthorities map[navigationScopeKey]map[string]navigationReceiverAuthority

func navigationReceiverDeclarations(root *sitter.Node, src []byte) navigationReceiverAuthorities {
	declarations := make(navigationReceiverAuthorities)
	walkNamedChildren(root, true, func(node *sitter.Node) {
		switch node.Type() {
		case "parameter", "class_parameter", "property_declaration":
			name := navigationBindingName(node, src)
			typeName := navigationDeclaredTypeName(node, src)
			if name == "" {
				return
			}
			scope := navigationScopeIdentity(navigationBindingScope(node, root))
			name = strings.TrimSpace(name)
			typeName = strings.TrimSpace(typeName)
			byName := declarations[scope]
			if byName == nil {
				byName = make(map[string]navigationReceiverAuthority)
				declarations[scope] = byName
			}
			authority, exists := byName[name]
			if authority.conflicted {
				return
			}
			if typeName == "" || (exists && authority.typeName != typeName) {
				byName[name] = navigationReceiverAuthority{conflicted: true}
				return
			}
			byName[name] = navigationReceiverAuthority{typeName: typeName}
		}
	})
	return declarations
}

func navigationReceiverTypeAt(node *sitter.Node, binding string, declarations navigationReceiverAuthorities) (string, bool) {
	binding = strings.TrimSpace(binding)
	if node == nil || binding == "" {
		return "", false
	}
	for current := node; current != nil; current = current.Parent() {
		if !navigationScopeBoundary(current) && current.Parent() != nil {
			continue
		}
		key := navigationScopeIdentity(current)
		if authority, declared := declarations[key][binding]; declared {
			// An untyped/inferred or conflicting declaration is authoritative
			// shadowing evidence, but it cannot mint a concrete identity.
			if authority.conflicted {
				return "", true
			}
			return authority.typeName, true
		}
	}
	return "", false
}

func navigationBindingScope(node, root *sitter.Node) *sitter.Node {
	for current := node.Parent(); current != nil; current = current.Parent() {
		if navigationScopeBoundary(current) {
			return current
		}
	}
	return root
}

func navigationScopeBoundary(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	switch node.Type() {
	case "function_declaration", "anonymous_function", "lambda_literal", "closure_expression",
		"init_declaration", "deinit_declaration", "getter", "setter",
		"class_declaration", "object_declaration", "companion_object", "protocol_declaration":
		return true
	default:
		return false
	}
}

func navigationScopeIdentity(node *sitter.Node) navigationScopeKey {
	if node == nil {
		return navigationScopeKey{}
	}
	return navigationScopeKey{nodeType: node.Type(), startByte: node.StartByte(), endByte: node.EndByte()}
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
