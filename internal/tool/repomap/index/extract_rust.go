package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	coretypes "github.com/hanchaoqun/codrax/internal/types"
)

func extractRust(root *sitter.Node, src []byte, file string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation) {
	rustExtractDeclarations(root, src, file, "", &syms, &imps, &rels)
	rels = append(rels, rustExtractCalls(root, src, file)...)
	return
}

// rustExtractDeclarations walks lexical declaration containers rather than
// only the source-file root. Rust inline modules are real identity scopes: a
// PyO3/JNI/FFI wrapper can legitimately have the same short name and arity as
// the crate-level implementation it delegates to. Keeping both callables with
// their module Parent prevents the symbol graph from collapsing that boundary.
//
// The scope carrier is language syntax, not a framework convention. It applies
// equally to ordinary nested modules and uses the same Symbol.Parent identity
// axis already shared by every repomap language.
func rustExtractDeclarations(node *sitter.Node, src []byte, file, scope string, syms *[]types.Symbol, imps *[]types.Import, rels *[]types.Relation) {
	if node == nil {
		return
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		switch ch.Type() {
		case "use_declaration":
			*imps = append(*imps, rustExtractUse(ch, src, file)...)

		case "mod_item":
			if nameNode := ch.ChildByFieldName("name"); nameNode != nil {
				name := nodeText(nameNode, src)
				// mod with body = inline module; without = file reference
				body := childByType(ch, "declaration_list")
				if body == nil {
					*imps = append(*imps, types.Import{
						Raw:  nodeText(ch, src),
						Path: name,
						File: file,
						Line: nodeLine(ch),
					})
				}
				*syms = append(*syms, types.Symbol{
					Name:     name,
					Kind:     "module",
					File:     file,
					Line:     nodeLine(ch),
					EndLine:  nodeEndLine(ch),
					Exported: rustIsPublic(ch, src),
					Parent:   scope,
				})
				if body != nil {
					rustExtractDeclarations(body, src, file, rustJoinScope(scope, name), syms, imps, rels)
				}
			}

		case "function_item":
			if s, ok := rustExtractFunc(ch, src, file, scope); ok {
				// A module-owned callable remains a function. rustExtractFunc's
				// historical non-empty-parent path denotes type/trait methods;
				// lexical module ownership is an identity qualifier, not dispatch.
				if scope != "" {
					s.Kind = "function"
				}
				*syms = append(*syms, s)
			}

		case "struct_item":
			*syms = append(*syms, rustScopeSymbols(rustExtractStruct(ch, src, file), scope)...)

		case "enum_item":
			if s, ok := rustExtractEnum(ch, src, file); ok {
				s.Parent = scope
				*syms = append(*syms, s)
			}

		case "trait_item":
			trait, traitMethods, traitRels := rustExtractTrait(ch, src, file)
			*syms = append(*syms, rustScopeSymbols(trait, scope)...)
			*syms = append(*syms, rustScopeSymbols(traitMethods, scope)...)
			*rels = append(*rels, traitRels...)

		case "impl_item":
			implMethods, implRels := rustExtractImpl(ch, src, file)
			*syms = append(*syms, rustScopeSymbols(implMethods, scope)...)
			*rels = append(*rels, implRels...)

		case "const_item", "static_item":
			if nameNode := ch.ChildByFieldName("name"); nameNode != nil {
				name := nodeText(nameNode, src)
				*syms = append(*syms, types.Symbol{
					Name:     name,
					Kind:     "const",
					File:     file,
					Line:     nodeLine(ch),
					EndLine:  nodeEndLine(ch),
					Exported: rustIsPublic(ch, src),
					Parent:   scope,
				})
			}

		case "type_item":
			if nameNode := ch.ChildByFieldName("name"); nameNode != nil {
				name := nodeText(nameNode, src)
				*syms = append(*syms, types.Symbol{
					Name:     name,
					Kind:     "type",
					File:     file,
					Line:     nodeLine(ch),
					EndLine:  nodeEndLine(ch),
					Exported: rustIsPublic(ch, src),
					Parent:   scope,
				})
			}
		}
	}
}

func rustJoinScope(parent, child string) string {
	parent = strings.TrimSpace(parent)
	child = strings.TrimSpace(child)
	if parent == "" {
		return child
	}
	if child == "" {
		return parent
	}
	return parent + "::" + child
}

func rustScopeSymbols(symbols []types.Symbol, scope string) []types.Symbol {
	if scope == "" {
		return symbols
	}
	for i := range symbols {
		symbols[i].Parent = rustJoinScope(scope, symbols[i].Parent)
	}
	return symbols
}

func rustExtractUse(node *sitter.Node, src []byte, file string) []types.Import {
	raw := nodeText(node, src)
	// Extract the use path - simplify to the scoped identifier
	path := raw
	path = strings.TrimPrefix(path, "pub ")
	path = strings.TrimPrefix(path, "use ")
	path = strings.TrimSuffix(path, ";")
	return []types.Import{{
		Raw:  raw,
		Path: strings.TrimSpace(path),
		File: file,
		Line: nodeLine(node),
	}}
}

func rustExtractFunc(node *sitter.Node, src []byte, file, parent string) (types.Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return types.Symbol{}, false
	}
	name := nodeText(nameNode, src)
	kind := "function"
	if parent != "" {
		kind = "method"
	}

	var sig string
	arity := 0
	if params := node.ChildByFieldName("parameters"); params != nil {
		sig = nodeText(params, src)
		if len(sig) > 120 {
			sig = coretypes.CutPrefixRuneSafe(sig, 117) + "..."
		}
		// rust grammar: each declared param is `parameter` /
		// `self_parameter` / `variadic_parameter`. `self_parameter`
		// (the `&self` / `&mut self` / `self` receiver) is excluded
		// from arity for parity with Go's method-set view.
		for k := 0; k < int(params.NamedChildCount()); k++ {
			p := params.NamedChild(k)
			switch p.Type() {
			case "parameter", "variadic_parameter":
				arity++
			}
		}
	}

	return types.Symbol{
		Name:      name,
		Kind:      kind,
		File:      file,
		Line:      nodeLine(node),
		EndLine:   nodeEndLine(node),
		Exported:  rustIsPublic(node, src),
		Parent:    parent,
		Arity:     arity,
		Signature: sig,
		Doc:       prevSiblingComment(node, src),
	}, true
}

func rustExtractStruct(node *sitter.Node, src []byte, file string) []types.Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nodeText(nameNode, src)
	out := []types.Symbol{{
		Name:     nodeText(nameNode, src),
		Kind:     "struct",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: rustIsPublic(node, src),
		Doc:      prevSiblingComment(node, src),
	}}
	for _, field := range rustStructFieldNodes(node) {
		nameNode := field.ChildByFieldName("name")
		if nameNode == nil {
			nameNode = childByType(field, "field_identifier")
		}
		if nameNode == nil {
			continue
		}
		fieldName := strings.TrimSpace(nodeText(nameNode, src))
		if fieldName == "" {
			continue
		}
		signature := ""
		if typeNode := field.ChildByFieldName("type"); typeNode != nil {
			signature = strings.TrimSpace(nodeText(typeNode, src))
		}
		out = append(out, types.Symbol{
			Name:         fieldName,
			Kind:         "field",
			File:         file,
			Line:         nodeLine(field),
			EndLine:      nodeEndLine(field),
			Exported:     rustIsPublic(field, src),
			Parent:       name,
			Signature:    signature,
			DeclaredType: signature,
		})
	}
	return out
}

func rustStructFieldNodes(node *sitter.Node) []*sitter.Node {
	var out []*sitter.Node
	walkNamedChildren(node, true, func(ch *sitter.Node) {
		switch ch.Type() {
		case "field_declaration", "field_declaration_list_field", "struct_field":
			out = append(out, ch)
		}
	})
	return out
}

func rustExtractEnum(node *sitter.Node, src []byte, file string) (types.Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return types.Symbol{}, false
	}
	return types.Symbol{
		Name:     nodeText(nameNode, src),
		Kind:     "enum",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: rustIsPublic(node, src),
		Doc:      prevSiblingComment(node, src),
	}, true
}

func rustExtractTrait(node *sitter.Node, src []byte, file string) (traits []types.Symbol, methods []types.Symbol, rels []types.Relation) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nodeText(nameNode, src)
	traits = append(traits, types.Symbol{
		Name:     name,
		Kind:     "trait",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: rustIsPublic(node, src),
		Doc:      prevSiblingComment(node, src),
	})

	// trait bounds (supertraits)
	if bounds := node.ChildByFieldName("bounds"); bounds != nil {
		walkNamedChildren(bounds, true, func(ch *sitter.Node) {
			if ch.Type() == "type_identifier" {
				rels = append(rels, types.Relation{
					Kind:       "inheritance",
					FromEP:     types.RelationEndpoint{Name: name, File: file, Line: nodeLine(ch)},
					ToEP:       types.RelationEndpoint{Name: nodeText(ch, src), File: file, Line: nodeLine(ch)},
					File:       file,
					Line:       nodeLine(ch),
					Confidence: types.ConfidenceAST,
					Provenance: types.ProvenanceTreeSitter,
					ResolvedBy: "rust_supertrait",
				})
			}
		})
	}

	// body → method signatures
	if body := node.ChildByFieldName("body"); body != nil {
		for j := 0; j < int(body.NamedChildCount()); j++ {
			member := body.NamedChild(j)
			if member.Type() == "function_item" || member.Type() == "function_signature_item" {
				if s, ok := rustExtractFunc(member, src, file, name); ok {
					methods = append(methods, s)
				}
			}
		}
	}
	return
}

func rustExtractImpl(node *sitter.Node, src []byte, file string) (methods []types.Symbol, rels []types.Relation) {
	// impl Type { ... } or impl Trait for Type { ... }
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return
	}
	typeName := nodeText(typeNode, src)

	// Check for trait impl
	if traitNode := node.ChildByFieldName("trait"); traitNode != nil {
		traitName := nodeText(traitNode, src)
		rels = append(rels, types.Relation{
			Kind:       "inheritance",
			FromEP:     types.RelationEndpoint{Name: typeName, File: file, Line: nodeLine(node)},
			ToEP:       types.RelationEndpoint{Name: traitName, File: file, Line: nodeLine(node)},
			File:       file,
			Line:       nodeLine(node),
			Confidence: types.ConfidenceAST,
			Provenance: types.ProvenanceTreeSitter,
			ResolvedBy: "rust_impl_trait",
		})
	}

	if body := node.ChildByFieldName("body"); body != nil {
		for j := 0; j < int(body.NamedChildCount()); j++ {
			member := body.NamedChild(j)
			if member.Type() == "function_item" {
				if s, ok := rustExtractFunc(member, src, file, typeName); ok {
					methods = append(methods, s)
				}
			}
		}
	}
	return
}

func rustIsPublic(node *sitter.Node, src []byte) bool {
	if vis := childByType(node, "visibility_modifier"); vis != nil {
		return strings.Contains(nodeText(vis, src), "pub")
	}
	return false
}

func rustExtractCalls(root *sitter.Node, src []byte, file string) []types.Relation {
	var rels []types.Relation
	receiverDeclarations := rustReceiverDeclarations(root, src)
	walkNamedChildren(root, true, func(node *sitter.Node) {
		if node.Type() != "call_expression" {
			return
		}
		caller := rustCallSourceEndpoint(node, src, file)
		fn := node.ChildByFieldName("function")
		if fn == nil {
			return
		}
		switch fn.Type() {
		case "identifier":
			rels = append(rels, types.Relation{
				Kind:       "call",
				FromEP:     caller,
				ToEP:       types.RelationEndpoint{Name: nodeText(fn, src), File: file, Line: nodeLine(fn)},
				File:       file,
				Line:       nodeLine(fn),
				Confidence: types.ConfidenceAST,
				Provenance: types.ProvenanceTreeSitter,
				ResolvedBy: "rust_ast_identifier_call",
			})
		case "field_expression":
			if field := fn.ChildByFieldName("field"); field != nil {
				receiver := ""
				if value := fn.ChildByFieldName("value"); value != nil {
					receiver = strings.TrimSpace(nodeText(value, src))
				}
				if binding := rustReceiverBinding(receiver); binding != "" {
					if receiverType, declared := lexicalReceiverTypeAt(node, binding, receiverDeclarations, rustReceiverScopeBoundary); declared && receiverType != "" {
						receiver = receiverType
					}
				}
				rels = append(rels, types.Relation{
					Kind:       "call",
					FromEP:     caller,
					ToEP:       types.RelationEndpoint{Name: nodeText(field, src), Receiver: receiver, File: file, Line: nodeLine(fn)},
					File:       file,
					Line:       nodeLine(fn),
					Confidence: types.ConfidenceAST,
					Provenance: types.ProvenanceTreeSitter,
					ResolvedBy: "rust_ast_field_call",
				})
			}
		case "scoped_identifier":
			if nameNode := fn.ChildByFieldName("name"); nameNode != nil {
				receiver := ""
				if path := fn.ChildByFieldName("path"); path != nil {
					receiver = strings.TrimSpace(nodeText(path, src))
				}
				rels = append(rels, types.Relation{
					Kind:       "call",
					FromEP:     caller,
					ToEP:       types.RelationEndpoint{Name: nodeText(nameNode, src), Receiver: receiver, File: file, Line: nodeLine(fn)},
					File:       file,
					Line:       nodeLine(fn),
					Confidence: types.ConfidenceAST,
					Provenance: types.ProvenanceTreeSitter,
					ResolvedBy: "rust_ast_scoped_call",
				})
			}
		}
	})
	return rels
}

// rustCallSourceEndpoint derives the caller from parser-owned lexical
// ancestry. Relation consumers can therefore distinguish a module wrapper
// from a same-named crate function without consulting model-authored labels.
func rustCallSourceEndpoint(node *sitter.Node, src []byte, file string) types.RelationEndpoint {
	ep := types.RelationEndpoint{File: file, Line: nodeLine(node)}
	var modules []string
	for current := node.Parent(); current != nil; current = current.Parent() {
		switch current.Type() {
		case "function_item":
			if ep.Name == "" {
				if name := current.ChildByFieldName("name"); name != nil {
					ep.Name = strings.TrimSpace(nodeText(name, src))
				}
			}
		case "mod_item":
			if name := current.ChildByFieldName("name"); name != nil {
				modules = append(modules, strings.TrimSpace(nodeText(name, src)))
			}
		}
	}
	for i := len(modules) - 1; i >= 0; i-- {
		ep.Receiver = rustJoinScope(ep.Receiver, modules[i])
	}
	return ep
}

func rustReceiverDeclarations(root *sitter.Node, src []byte) lexicalReceiverAuthorities {
	declarations := make(lexicalReceiverAuthorities)
	walkNamedChildren(root, true, func(node *sitter.Node) {
		switch node.Type() {
		case "parameter", "let_declaration":
			name := node.ChildByFieldName("pattern")
			if name == nil {
				name = node.ChildByFieldName("name")
			}
			if name != nil {
				scope := lexicalReceiverBindingScope(node, root, rustReceiverScopeBoundary)
				addScopedReceiverAuthority(declarations, scope, node, nodeText(name, src), rustDeclaredTypeName(node.ChildByFieldName("type"), src), node.Type() == "parameter")
			}
		}
	})
	return declarations
}

func rustReceiverScopeBoundary(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	switch node.Type() {
	case "block", "function_item", "closure_expression":
		return true
	default:
		return false
	}
}

func rustDeclaredTypeName(node *sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	if node.Type() == "type_identifier" {
		return strings.TrimSpace(nodeText(node, src))
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		if name := rustDeclaredTypeName(node.NamedChild(i), src); name != "" {
			return name
		}
	}
	return ""
}

func rustReceiverBinding(receiver string) string {
	receiver = strings.TrimSpace(receiver)
	if idx := strings.LastIndex(receiver, "."); idx >= 0 {
		receiver = receiver[idx+1:]
	}
	return strings.TrimSpace(receiver)
}
