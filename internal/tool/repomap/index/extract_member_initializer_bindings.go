package index

import (
	"sort"
	"strconv"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// extractMemberInitializerBindings preserves the syntax-explicit container
// identity of keyed/member initializers. A line feature alone only proves that
// `field: value` is a member initializer; without this tuple downstream code
// sees the receiver as bare `field` and can no longer connect it to a requested
// `Container.field` participant.
//
// The extractor is deliberately fail-closed. Untyped JS/Python object/dict
// literals and named function arguments publish no owner. For nested literals,
// only the nearest composite is considered, so an untyped inner object cannot
// borrow the type of an outer object.
func extractMemberInitializerBindings(root *sitter.Node, src []byte, language string) map[int][]types.MemberInitializerBinding {
	if root == nil {
		return nil
	}
	out := make(map[int][]types.MemberInitializerBinding)
	seen := make(map[string]bool)
	var walk func(*sitter.Node)
	walk = func(node *sitter.Node) {
		if node == nil {
			return
		}
		switch node.Type() {
		case "keyed_element", "pair", "field_initializer", "initializer_pair", "designated_initializer":
			member := memberInitializerIdentity(node, src)
			owner := memberInitializerOwnerType(node, src, language)
			line := int(node.StartPoint().Row) + 1
			if member != "" && owner != "" && line > 0 {
				key := strings.Join([]string{owner, member, strconv.Itoa(line)}, "\x00")
				if !seen[key] {
					seen[key] = true
					out[line] = append(out[line], types.MemberInitializerBinding{
						Member: member, OwnerType: owner,
					})
				}
			}
		}
		for i := 0; i < int(node.NamedChildCount()); i++ {
			walk(node.NamedChild(i))
		}
	}
	walk(root)
	if len(out) == 0 {
		return nil
	}
	for line := range out {
		sort.Slice(out[line], func(i, j int) bool {
			if out[line][i].OwnerType != out[line][j].OwnerType {
				return out[line][i].OwnerType < out[line][j].OwnerType
			}
			return out[line][i].Member < out[line][j].Member
		})
	}
	return out
}

func memberInitializerIdentity(node *sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	for _, field := range []string{"field", "key", "designator"} {
		if child := node.ChildByFieldName(field); child != nil {
			if identity := memberInitializerIdentityNode(child, src); identity != "" {
				return identity
			}
		}
	}
	// Go keyed_element does not expose a named `key` field. Its first named
	// literal_element contains the exact field identifier.
	if node.Type() == "keyed_element" && node.NamedChildCount() > 0 {
		return memberInitializerIdentityNode(node.NamedChild(0), src)
	}
	return ""
}

func memberInitializerIdentityNode(node *sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	switch node.Type() {
	case "identifier", "field_identifier", "property_identifier", "simple_identifier":
		return strings.TrimSpace(nodeText(node, src))
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		if identity := memberInitializerIdentityNode(node.NamedChild(i), src); identity != "" {
			return identity
		}
	}
	return ""
}

func memberInitializerOwnerType(node *sitter.Node, src []byte, language string) string {
	for current := node.Parent(); current != nil; current = current.Parent() {
		switch current.Type() {
		case "composite_literal":
			return cleanMemberInitializerOwnerType(goCleanTypeName(current.ChildByFieldName("type"), src))
		case "struct_expression":
			return cleanMemberInitializerOwnerType(rustDeclaredTypeName(current.ChildByFieldName("name"), src))
		case "object":
			// An object literal itself has no nominal identity. Admit only a
			// direct TS/ArkTS variable declaration with an explicit type.
			parent := current.Parent()
			if parent == nil || parent.Type() != "variable_declarator" ||
				!sameSyntaxNode(parent.ChildByFieldName("value"), current) {
				return ""
			}
			if language != types.LangTypeScript && language != types.LangArkTS {
				return ""
			}
			return cleanMemberInitializerOwnerType(jsDeclaredTypeName(parent.ChildByFieldName("type"), src))
		case "initializer_list":
			// A nested C/C++ initializer list has no independent explicit
			// owner. It must not borrow the declaration type of its ancestor.
			init := current.Parent()
			if init == nil || init.Type() != "init_declarator" ||
				!sameSyntaxNode(init.ChildByFieldName("value"), current) {
				return ""
			}
			decl := init.Parent()
			if decl == nil || decl.Type() != "declaration" ||
				(language != types.LangC && language != types.LangCpp) {
				return ""
			}
			return cleanMemberInitializerOwnerType(cDeclaredTypeName(decl.ChildByFieldName("type"), src))
		}
	}
	return ""
}

func sameSyntaxNode(left, right *sitter.Node) bool {
	return left != nil && right != nil &&
		left.StartByte() == right.StartByte() && left.EndByte() == right.EndByte() &&
		left.Type() == right.Type()
}

func cleanMemberInitializerOwnerType(raw string) string {
	raw = strings.TrimSpace(strings.Trim(raw, "*&(){}[]"))
	if raw == "" {
		return ""
	}
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '$', r == '.', r == ':':
		default:
			return ""
		}
	}
	return strings.Trim(raw, ".:")
}
