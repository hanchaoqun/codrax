package repomap

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

func extractRust(root *sitter.Node, src []byte, file string) (pkg string, syms []Symbol, imps []Import, rels []Relation) {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		ch := root.NamedChild(i)
		switch ch.Type() {
		case "use_declaration":
			imps = append(imps, rustExtractUse(ch, src, file)...)

		case "mod_item":
			if nameNode := ch.ChildByFieldName("name"); nameNode != nil {
				name := nodeText(nameNode, src)
				// mod with body = inline module; without = file reference
				if childByType(ch, "declaration_list") == nil {
					imps = append(imps, Import{
						Raw:  nodeText(ch, src),
						Path: name,
						File: file,
						Line: nodeLine(ch),
					})
				}
				syms = append(syms, Symbol{
					Name:     name,
					Kind:     "module",
					File:     file,
					Line:     nodeLine(ch),
					EndLine:  nodeEndLine(ch),
					Exported: rustIsPublic(ch, src),
				})
			}

		case "function_item":
			if s, ok := rustExtractFunc(ch, src, file, ""); ok {
				syms = append(syms, s)
			}

		case "struct_item":
			if s, ok := rustExtractStruct(ch, src, file); ok {
				syms = append(syms, s)
			}

		case "enum_item":
			if s, ok := rustExtractEnum(ch, src, file); ok {
				syms = append(syms, s)
			}

		case "trait_item":
			trait, traitMethods, traitRels := rustExtractTrait(ch, src, file)
			syms = append(syms, trait...)
			syms = append(syms, traitMethods...)
			rels = append(rels, traitRels...)

		case "impl_item":
			implMethods, implRels := rustExtractImpl(ch, src, file)
			syms = append(syms, implMethods...)
			rels = append(rels, implRels...)

		case "const_item", "static_item":
			if nameNode := ch.ChildByFieldName("name"); nameNode != nil {
				name := nodeText(nameNode, src)
				syms = append(syms, Symbol{
					Name:     name,
					Kind:     "const",
					File:     file,
					Line:     nodeLine(ch),
					EndLine:  nodeEndLine(ch),
					Exported: rustIsPublic(ch, src),
				})
			}

		case "type_item":
			if nameNode := ch.ChildByFieldName("name"); nameNode != nil {
				name := nodeText(nameNode, src)
				syms = append(syms, Symbol{
					Name:     name,
					Kind:     "type",
					File:     file,
					Line:     nodeLine(ch),
					EndLine:  nodeEndLine(ch),
					Exported: rustIsPublic(ch, src),
				})
			}
		}
	}

	rels = append(rels, rustExtractCalls(root, src, file)...)
	return
}

func rustExtractUse(node *sitter.Node, src []byte, file string) []Import {
	raw := nodeText(node, src)
	// Extract the use path - simplify to the scoped identifier
	path := raw
	path = strings.TrimPrefix(path, "pub ")
	path = strings.TrimPrefix(path, "use ")
	path = strings.TrimSuffix(path, ";")
	return []Import{{
		Raw:  raw,
		Path: strings.TrimSpace(path),
		File: file,
		Line: nodeLine(node),
	}}
}

func rustExtractFunc(node *sitter.Node, src []byte, file, parent string) (Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Symbol{}, false
	}
	name := nodeText(nameNode, src)
	kind := "function"
	if parent != "" {
		kind = "method"
	}

	var sig string
	if params := node.ChildByFieldName("parameters"); params != nil {
		sig = nodeText(params, src)
		if len(sig) > 120 {
			sig = sig[:117] + "..."
		}
	}

	return Symbol{
		Name:      name,
		Kind:      kind,
		File:      file,
		Line:      nodeLine(node),
		EndLine:   nodeEndLine(node),
		Exported:  rustIsPublic(node, src),
		Parent:    parent,
		Signature: sig,
		Doc:       prevSiblingComment(node, src),
	}, true
}

func rustExtractStruct(node *sitter.Node, src []byte, file string) (Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Symbol{}, false
	}
	return Symbol{
		Name:     nodeText(nameNode, src),
		Kind:     "struct",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: rustIsPublic(node, src),
		Doc:      prevSiblingComment(node, src),
	}, true
}

func rustExtractEnum(node *sitter.Node, src []byte, file string) (Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Symbol{}, false
	}
	return Symbol{
		Name:     nodeText(nameNode, src),
		Kind:     "enum",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: rustIsPublic(node, src),
		Doc:      prevSiblingComment(node, src),
	}, true
}

func rustExtractTrait(node *sitter.Node, src []byte, file string) (traits []Symbol, methods []Symbol, rels []Relation) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nodeText(nameNode, src)
	traits = append(traits, Symbol{
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
				rels = append(rels, Relation{
					Kind: "inheritance",
					From: file + ":" + name,
					To:   nodeText(ch, src),
					File: file,
					Line: nodeLine(ch),
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

func rustExtractImpl(node *sitter.Node, src []byte, file string) (methods []Symbol, rels []Relation) {
	// impl Type { ... } or impl Trait for Type { ... }
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return
	}
	typeName := nodeText(typeNode, src)

	// Check for trait impl
	if traitNode := node.ChildByFieldName("trait"); traitNode != nil {
		traitName := nodeText(traitNode, src)
		rels = append(rels, Relation{
			Kind: "inheritance",
			From: file + ":" + typeName,
			To:   traitName,
			File: file,
			Line: nodeLine(node),
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

func rustExtractCalls(root *sitter.Node, src []byte, file string) []Relation {
	var rels []Relation
	walkNamedChildren(root, true, func(node *sitter.Node) {
		if node.Type() != "call_expression" {
			return
		}
		fn := node.ChildByFieldName("function")
		if fn == nil {
			return
		}
		switch fn.Type() {
		case "identifier":
			rels = append(rels, Relation{
				Kind: "call",
				From: file,
				To:   nodeText(fn, src),
				File: file,
				Line: nodeLine(fn),
			})
		case "field_expression":
			if field := fn.ChildByFieldName("field"); field != nil {
				rels = append(rels, Relation{
					Kind: "call",
					From: file,
					To:   nodeText(field, src),
					File: file,
					Line: nodeLine(fn),
				})
			}
		case "scoped_identifier":
			if nameNode := fn.ChildByFieldName("name"); nameNode != nil {
				rels = append(rels, Relation{
					Kind: "call",
					From: file,
					To:   nodeText(nameNode, src),
					File: file,
					Line: nodeLine(fn),
				})
			}
		}
	})
	return rels
}
