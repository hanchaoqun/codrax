package repomap

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// extractJS handles both JavaScript and TypeScript.
func extractJS(root *sitter.Node, src []byte, file string, isTS bool) (pkg string, syms []Symbol, imps []Import, rels []Relation) {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		ch := root.NamedChild(i)
		switch ch.Type() {
		case "import_statement":
			imps = append(imps, jsExtractImport(ch, src, file)...)

		case "export_statement":
			s, imp, rel := jsExtractExport(ch, src, file, isTS)
			syms = append(syms, s...)
			imps = append(imps, imp...)
			rels = append(rels, rel...)

		case "function_declaration":
			if s, ok := jsExtractFunc(ch, src, file, false); ok {
				syms = append(syms, s)
			}

		case "class_declaration":
			cls, methods, classRels := jsExtractClass(ch, src, file)
			syms = append(syms, cls...)
			syms = append(syms, methods...)
			rels = append(rels, classRels...)

		case "lexical_declaration", "variable_declaration":
			syms = append(syms, jsExtractVarDecl(ch, src, file, false)...)

		// TypeScript-specific
		case "interface_declaration":
			if isTS {
				syms = append(syms, jsExtractInterface(ch, src, file)...)
			}
		case "type_alias_declaration":
			if isTS {
				if s, ok := jsExtractTypeAlias(ch, src, file); ok {
					syms = append(syms, s)
				}
			}
		case "enum_declaration":
			if isTS {
				if s, ok := jsExtractEnum(ch, src, file); ok {
					syms = append(syms, s)
				}
			}
		}
	}

	rels = append(rels, jsExtractCalls(root, src, file)...)
	return
}

func jsExtractImport(node *sitter.Node, src []byte, file string) []Import {
	raw := nodeText(node, src)
	var path string
	if source := node.ChildByFieldName("source"); source != nil {
		path = strings.Trim(nodeText(source, src), `"'`)
	} else {
		// find string child
		walkNamedChildren(node, false, func(ch *sitter.Node) {
			if ch.Type() == "string" && path == "" {
				path = strings.Trim(nodeText(ch, src), `"'`)
			}
		})
	}
	return []Import{{
		Raw:  raw,
		Path: path,
		File: file,
		Line: nodeLine(node),
	}}
}

func jsExtractExport(node *sitter.Node, src []byte, file string, isTS bool) (syms []Symbol, imps []Import, rels []Relation) {
	// export can wrap function_declaration, class_declaration, variable_declaration, etc.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		switch ch.Type() {
		case "function_declaration":
			if s, ok := jsExtractFunc(ch, src, file, true); ok {
				syms = append(syms, s)
			}
		case "class_declaration":
			cls, methods, classRels := jsExtractClass(ch, src, file)
			for idx := range cls {
				cls[idx].Exported = true
			}
			syms = append(syms, cls...)
			syms = append(syms, methods...)
			rels = append(rels, classRels...)
		case "lexical_declaration", "variable_declaration":
			syms = append(syms, jsExtractVarDecl(ch, src, file, true)...)
		case "interface_declaration":
			if isTS {
				ifaces := jsExtractInterface(ch, src, file)
				for idx := range ifaces {
					ifaces[idx].Exported = true
				}
				syms = append(syms, ifaces...)
			}
		case "type_alias_declaration":
			if isTS {
				if s, ok := jsExtractTypeAlias(ch, src, file); ok {
					s.Exported = true
					syms = append(syms, s)
				}
			}
		case "enum_declaration":
			if isTS {
				if s, ok := jsExtractEnum(ch, src, file); ok {
					s.Exported = true
					syms = append(syms, s)
				}
			}
		}
	}

	// re-export: export { ... } from '...'
	if source := node.ChildByFieldName("source"); source != nil {
		path := strings.Trim(nodeText(source, src), `"'`)
		imps = append(imps, Import{
			Raw:  nodeText(node, src),
			Path: path,
			File: file,
			Line: nodeLine(node),
		})
	}
	return
}

func jsExtractFunc(node *sitter.Node, src []byte, file string, exported bool) (Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Symbol{}, false
	}
	name := nodeText(nameNode, src)
	var sig string
	if params := node.ChildByFieldName("parameters"); params != nil {
		sig = nodeText(params, src)
		if len(sig) > 120 {
			sig = sig[:117] + "..."
		}
	}
	return Symbol{
		Name:      name,
		Kind:      "function",
		File:      file,
		Line:      nodeLine(node),
		EndLine:   nodeEndLine(node),
		Exported:  exported,
		Signature: sig,
		Doc:       prevSiblingComment(node, src),
	}, true
}

func jsExtractClass(node *sitter.Node, src []byte, file string) (cls []Symbol, methods []Symbol, rels []Relation) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nodeText(nameNode, src)
	cls = append(cls, Symbol{
		Name:    name,
		Kind:    "class",
		File:    file,
		Line:    nodeLine(node),
		EndLine: nodeEndLine(node),
		Doc:     prevSiblingComment(node, src),
	})

	// heritage: extends, implements
	if heritage := childByType(node, "class_heritage"); heritage != nil {
		text := nodeText(heritage, src)
		if strings.Contains(text, "extends") || strings.Contains(text, "implements") {
			// extract identifiers from heritage clause
			walkNamedChildren(heritage, true, func(ch *sitter.Node) {
				if ch.Type() == "identifier" || ch.Type() == "type_identifier" {
					baseName := nodeText(ch, src)
					if baseName != "extends" && baseName != "implements" {
						rels = append(rels, Relation{
							Kind: "inheritance",
							From: file + ":" + name,
							To:   baseName,
							File: file,
							Line: nodeLine(ch),
						})
					}
				}
			})
		}
	}

	// body → methods
	if body := node.ChildByFieldName("body"); body != nil {
		for j := 0; j < int(body.NamedChildCount()); j++ {
			member := body.NamedChild(j)
			if member.Type() == "method_definition" || member.Type() == "public_field_definition" {
				mName := member.ChildByFieldName("name")
				if mName != nil {
					mn := nodeText(mName, src)
					kind := "method"
					if member.Type() == "public_field_definition" {
						kind = "field"
					}
					methods = append(methods, Symbol{
						Name:    mn,
						Kind:    kind,
						File:    file,
						Line:    nodeLine(member),
						EndLine: nodeEndLine(member),
						Parent:  name,
					})
				}
			}
		}
	}
	return
}

func jsExtractVarDecl(node *sitter.Node, src []byte, file string, exported bool) []Symbol {
	var syms []Symbol
	for i := 0; i < int(node.NamedChildCount()); i++ {
		decl := node.NamedChild(i)
		if decl.Type() != "variable_declarator" {
			continue
		}
		nameNode := decl.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nodeText(nameNode, src)

		kind := "var"
		// Check if value is an arrow function or function
		if val := decl.ChildByFieldName("value"); val != nil {
			if val.Type() == "arrow_function" || val.Type() == "function" {
				kind = "function"
			}
		}

		syms = append(syms, Symbol{
			Name:     name,
			Kind:     kind,
			File:     file,
			Line:     nodeLine(decl),
			EndLine:  nodeEndLine(decl),
			Exported: exported,
		})
	}
	return syms
}

func jsExtractInterface(node *sitter.Node, src []byte, file string) []Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	return []Symbol{{
		Name:    nodeText(nameNode, src),
		Kind:    "interface",
		File:    file,
		Line:    nodeLine(node),
		EndLine: nodeEndLine(node),
		Doc:     prevSiblingComment(node, src),
	}}
}

func jsExtractTypeAlias(node *sitter.Node, src []byte, file string) (Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Symbol{}, false
	}
	return Symbol{
		Name:    nodeText(nameNode, src),
		Kind:    "type",
		File:    file,
		Line:    nodeLine(node),
		EndLine: nodeEndLine(node),
		Doc:     prevSiblingComment(node, src),
	}, true
}

func jsExtractEnum(node *sitter.Node, src []byte, file string) (Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Symbol{}, false
	}
	return Symbol{
		Name:    nodeText(nameNode, src),
		Kind:    "enum",
		File:    file,
		Line:    nodeLine(node),
		EndLine: nodeEndLine(node),
		Doc:     prevSiblingComment(node, src),
	}, true
}

func jsExtractCalls(root *sitter.Node, src []byte, file string) []Relation {
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
		case "member_expression":
			if prop := fn.ChildByFieldName("property"); prop != nil {
				rels = append(rels, Relation{
					Kind: "call",
					From: file,
					To:   nodeText(prop, src),
					File: file,
					Line: nodeLine(fn),
				})
			}
		}
	})
	return rels
}
