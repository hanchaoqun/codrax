package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func extractRust(root *sitter.Node, src []byte, file string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation) {
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
					imps = append(imps, types.Import{
						Raw:  nodeText(ch, src),
						Path: name,
						File: file,
						Line: nodeLine(ch),
					})
				}
				syms = append(syms, types.Symbol{
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
			syms = append(syms, rustExtractStruct(ch, src, file)...)

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
				syms = append(syms, types.Symbol{
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
				syms = append(syms, types.Symbol{
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
			sig = sig[:117] + "..."
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
			Name:      fieldName,
			Kind:      "field",
			File:      file,
			Line:      nodeLine(field),
			EndLine:   nodeEndLine(field),
			Exported:  rustIsPublic(field, src),
			Parent:    name,
			Signature: signature,
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

func rustExtractCalls(root *sitter.Node, src []byte, file string) []types.Relation {
	var rels []types.Relation
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
			rels = append(rels, types.Relation{
				Kind: "call",
				From: file,
				To:   nodeText(fn, src),
				File: file,
				Line: nodeLine(fn),
			})
		case "field_expression":
			if field := fn.ChildByFieldName("field"); field != nil {
				rels = append(rels, types.Relation{
					Kind: "call",
					From: file,
					To:   nodeText(field, src),
					File: file,
					Line: nodeLine(fn),
				})
			}
		case "scoped_identifier":
			if nameNode := fn.ChildByFieldName("name"); nameNode != nil {
				rels = append(rels, types.Relation{
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
