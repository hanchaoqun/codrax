package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func extractCCpp(root *sitter.Node, src []byte, file, lang string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation) {
	isCpp := lang == types.LangCpp
	isHeader := strings.HasSuffix(file, ".h") || strings.HasSuffix(file, ".hpp") || strings.HasSuffix(file, ".hh")

	for i := 0; i < int(root.NamedChildCount()); i++ {
		ch := root.NamedChild(i)
		switch ch.Type() {
		case "preproc_include":
			if path := ch.ChildByFieldName("path"); path != nil {
				p := nodeText(path, src)
				p = strings.Trim(p, `"<>`)
				imps = append(imps, types.Import{
					Raw:  nodeText(ch, src),
					Path: p,
					File: file,
					Line: nodeLine(ch),
				})
			}

		case "function_definition":
			if s, ok := cExtractFunc(ch, src, file, isHeader); ok {
				syms = append(syms, s)
			}

		case "declaration":
			// Could be function declaration (prototype), variable, or typedef
			syms = append(syms, cExtractDeclaration(ch, src, file, isHeader, "")...)

		case "struct_specifier":
			syms = append(syms, cExtractStruct(ch, src, file, isHeader)...)

		case "enum_specifier":
			if s, ok := cExtractEnumSpec(ch, src, file, isHeader); ok {
				syms = append(syms, s)
			}

		case "type_definition":
			if s, ok := cExtractTypedef(ch, src, file, isHeader); ok {
				syms = append(syms, s)
			}

		// C++ specific
		case "class_specifier":
			if isCpp {
				cls, methods, classRels := cppExtractClass(ch, src, file, isHeader)
				syms = append(syms, cls...)
				syms = append(syms, methods...)
				rels = append(rels, classRels...)
			}

		case "namespace_definition":
			if isCpp {
				if nameNode := ch.ChildByFieldName("name"); nameNode != nil {
					pkg = nodeText(nameNode, src)
				}
				// recurse into namespace body
				if body := ch.ChildByFieldName("body"); body != nil {
					_, innerSyms, innerImps, innerRels := extractCCpp(body, src, file, lang)
					syms = append(syms, innerSyms...)
					imps = append(imps, innerImps...)
					rels = append(rels, innerRels...)
				}
			}
		}
	}

	rels = append(rels, cExtractCalls(root, src, file)...)
	return
}

func cExtractFunc(node *sitter.Node, src []byte, file string, isHeader bool) (types.Symbol, bool) {
	declr := node.ChildByFieldName("declarator")
	if declr == nil {
		return types.Symbol{}, false
	}

	// declarator is function_declarator
	var name string
	if declr.Type() == "function_declarator" {
		if nameNode := declr.ChildByFieldName("declarator"); nameNode != nil {
			name = nodeText(nameNode, src)
			// strip pointer/reference markers
			name = strings.TrimLeft(name, "*& ")
		}
	} else if declr.Type() == "pointer_declarator" {
		// *func_name(...)
		if inner := childByType(declr, "function_declarator"); inner != nil {
			if nameNode := inner.ChildByFieldName("declarator"); nameNode != nil {
				name = nodeText(nameNode, src)
				name = strings.TrimLeft(name, "*& ")
			}
		}
	}

	if name == "" {
		return types.Symbol{}, false
	}

	var sig string
	if declr.Type() == "function_declarator" {
		if params := declr.ChildByFieldName("parameters"); params != nil {
			sig = nodeText(params, src)
			if len(sig) > 120 {
				sig = sig[:117] + "..."
			}
		}
	}

	return types.Symbol{
		Name:      name,
		Kind:      "function",
		File:      file,
		Line:      nodeLine(node),
		EndLine:   nodeEndLine(node),
		Exported:  isHeader, // header declarations are "exported"
		Signature: sig,
		Doc:       prevSiblingComment(node, src),
	}, true
}

func cExtractDeclaration(node *sitter.Node, src []byte, file string, isHeader bool, parent string) []types.Symbol {
	var syms []types.Symbol
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() == "struct_specifier" {
			syms = append(syms, cExtractStruct(ch, src, file, isHeader)...)
		}
	}
	// look for function declarators (prototypes) in the declaration
	walkNamedChildren(node, false, func(ch *sitter.Node) {
		if ch.Type() == "function_declarator" {
			if nameNode := ch.ChildByFieldName("declarator"); nameNode != nil {
				name := nodeText(nameNode, src)
				name = strings.TrimLeft(name, "*& ")
				if name != "" {
					kind := "function"
					if parent != "" {
						kind = "method"
					}
					syms = append(syms, types.Symbol{
						Name:     name,
						Kind:     kind,
						File:     file,
						Line:     nodeLine(node),
						EndLine:  nodeEndLine(node),
						Exported: isHeader,
						Parent:   parent,
					})
				}
			}
		}
	})
	if parent != "" {
		syms = append(syms, cExtractFieldDeclarators(node, src, file, isHeader, parent)...)
	}
	return syms
}

func cExtractStruct(node *sitter.Node, src []byte, file string, isHeader bool) []types.Symbol {
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
		Exported: isHeader,
		Doc:      prevSiblingComment(node, src),
	}}
	if body := childByType(node, "field_declaration_list"); body != nil {
		for i := 0; i < int(body.NamedChildCount()); i++ {
			field := body.NamedChild(i)
			if field.Type() != "field_declaration" && field.Type() != "declaration" {
				continue
			}
			out = append(out, cExtractFieldDeclarators(field, src, file, isHeader, name)...)
		}
	}
	return out
}

func cExtractEnumSpec(node *sitter.Node, src []byte, file string, isHeader bool) (types.Symbol, bool) {
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
		Exported: isHeader,
	}, true
}

func cExtractTypedef(node *sitter.Node, src []byte, file string, isHeader bool) (types.Symbol, bool) {
	// typedef ... name;
	// The last declarator child is usually the name
	var name string
	for j := int(node.NamedChildCount()) - 1; j >= 0; j-- {
		ch := node.NamedChild(j)
		if ch.Type() == "type_identifier" {
			name = nodeText(ch, src)
			break
		}
	}
	if name == "" {
		return types.Symbol{}, false
	}
	return types.Symbol{
		Name:     name,
		Kind:     "type",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: isHeader,
	}, true
}

func cppExtractClass(node *sitter.Node, src []byte, file string, isHeader bool) (cls []types.Symbol, methods []types.Symbol, rels []types.Relation) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nodeText(nameNode, src)
	cls = append(cls, types.Symbol{
		Name:     name,
		Kind:     "class",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: isHeader,
		Doc:      prevSiblingComment(node, src),
	})

	// base classes
	if bases := childByType(node, "base_class_clause"); bases != nil {
		walkNamedChildren(bases, true, func(ch *sitter.Node) {
			if ch.Type() == "type_identifier" {
				rels = append(rels, types.Relation{
					Kind:       "inheritance",
					FromEP:     types.RelationEndpoint{Name: name, File: file, Line: nodeLine(ch)},
					ToEP:       types.RelationEndpoint{Name: nodeText(ch, src), File: file, Line: nodeLine(ch)},
					File:       file,
					Line:       nodeLine(ch),
					Confidence: types.ConfidenceAST,
					Provenance: types.ProvenanceTreeSitter,
					ResolvedBy: "cpp_base_class",
				})
			}
		})
	}

	// body → methods and fields
	if body := node.ChildByFieldName("body"); body != nil {
		for j := 0; j < int(body.NamedChildCount()); j++ {
			member := body.NamedChild(j)
			switch member.Type() {
			case "function_definition":
				if s, ok := cExtractFunc(member, src, file, isHeader); ok {
					s.Parent = name
					s.Kind = "method"
					methods = append(methods, s)
				}
			case "declaration":
				methods = append(methods, cExtractDeclaration(member, src, file, isHeader, name)...)
			case "field_declaration":
				methods = append(methods, cExtractFieldDeclarators(member, src, file, isHeader, name)...)
			case "access_specifier":
				// skip
			}
		}
	}
	return
}

func cExtractFieldDeclarators(node *sitter.Node, src []byte, file string, isHeader bool, parent string) []types.Symbol {
	if node == nil || parent == "" {
		return nil
	}
	var out []types.Symbol
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() == "function_declarator" || ch.Type() == "struct_specifier" ||
			ch.Type() == "primitive_type" || ch.Type() == "type_identifier" ||
			ch.Type() == "storage_class_specifier" || ch.Type() == "type_qualifier" {
			continue
		}
		name := cDeclaratorName(ch, src)
		if name == "" {
			continue
		}
		out = append(out, types.Symbol{
			Name:      name,
			Kind:      "field",
			File:      file,
			Line:      nodeLine(node),
			EndLine:   nodeEndLine(node),
			Exported:  isHeader,
			Parent:    parent,
			Signature: strings.TrimSpace(nodeText(node, src)),
		})
	}
	return out
}

func cDeclaratorName(node *sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	switch node.Type() {
	case "identifier", "field_identifier":
		return strings.TrimLeft(strings.TrimSpace(nodeText(node, src)), "*& ")
	case "init_declarator", "array_declarator", "pointer_declarator", "reference_declarator",
		"parenthesized_declarator":
		if inner := node.ChildByFieldName("declarator"); inner != nil {
			return cDeclaratorName(inner, src)
		}
	case "declaration":
		for i := 0; i < int(node.NamedChildCount()); i++ {
			if name := cDeclaratorName(node.NamedChild(i), src); name != "" {
				return name
			}
		}
	}
	return ""
}

func cExtractCalls(root *sitter.Node, src []byte, file string) []types.Relation {
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
				Kind:       "call",
				FromEP:     types.RelationEndpoint{File: file, Line: nodeLine(fn)},
				ToEP:       types.RelationEndpoint{Name: nodeText(fn, src), File: file, Line: nodeLine(fn)},
				File:       file,
				Line:       nodeLine(fn),
				Confidence: types.ConfidenceAST,
				Provenance: types.ProvenanceTreeSitter,
				ResolvedBy: "c_ast_identifier_call",
			})
		case "field_expression":
			if field := fn.ChildByFieldName("field"); field != nil {
				rels = append(rels, types.Relation{
					Kind:       "call",
					FromEP:     types.RelationEndpoint{File: file, Line: nodeLine(fn)},
					ToEP:       types.RelationEndpoint{Name: nodeText(field, src), File: file, Line: nodeLine(fn)},
					File:       file,
					Line:       nodeLine(fn),
					Confidence: types.ConfidenceAST,
					Provenance: types.ProvenanceTreeSitter,
					ResolvedBy: "c_ast_field_call",
				})
			}
		case "scoped_identifier":
			if nameNode := fn.ChildByFieldName("name"); nameNode != nil {
				rels = append(rels, types.Relation{
					Kind:       "call",
					FromEP:     types.RelationEndpoint{File: file, Line: nodeLine(fn)},
					ToEP:       types.RelationEndpoint{Name: nodeText(nameNode, src), File: file, Line: nodeLine(fn)},
					File:       file,
					Line:       nodeLine(fn),
					Confidence: types.ConfidenceAST,
					Provenance: types.ProvenanceTreeSitter,
					ResolvedBy: "cpp_ast_scoped_call",
				})
			}
		}
	})
	return rels
}
