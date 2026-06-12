package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// extractJS handles both JavaScript and TypeScript.
func extractJS(root *sitter.Node, src []byte, file string, isTS bool) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation) {
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

func jsExtractImport(node *sitter.Node, src []byte, file string) []types.Import {
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
	return []types.Import{{
		Raw:  raw,
		Path: path,
		File: file,
		Line: nodeLine(node),
	}}
}

func jsExtractExport(node *sitter.Node, src []byte, file string, isTS bool) (syms []types.Symbol, imps []types.Import, rels []types.Relation) {
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
		imps = append(imps, types.Import{
			Raw:  nodeText(node, src),
			Path: path,
			File: file,
			Line: nodeLine(node),
		})
	}
	return
}

func jsExtractFunc(node *sitter.Node, src []byte, file string, exported bool) (types.Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return types.Symbol{}, false
	}
	name := nodeText(nameNode, src)
	var sig string
	if params := node.ChildByFieldName("parameters"); params != nil {
		sig = nodeText(params, src)
		if len(sig) > 120 {
			sig = sig[:117] + "..."
		}
	}
	return types.Symbol{
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

func jsExtractClass(node *sitter.Node, src []byte, file string) (cls []types.Symbol, methods []types.Symbol, rels []types.Relation) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nodeText(nameNode, src)
	cls = append(cls, types.Symbol{
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
						rels = append(rels, types.Relation{
							Kind:       "inheritance",
							FromEP:     types.RelationEndpoint{Name: name, File: file, Line: nodeLine(ch)},
							ToEP:       types.RelationEndpoint{Name: baseName, File: file, Line: nodeLine(ch)},
							File:       file,
							Line:       nodeLine(ch),
							Confidence: types.ConfidenceAST,
							Provenance: types.ProvenanceTreeSitter,
							ResolvedBy: "js_class_heritage",
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
			if member.Type() == "method_definition" || member.Type() == "public_field_definition" ||
				member.Type() == "field_definition" {
				mName := member.ChildByFieldName("name")
				if mName == nil {
					mName = childByType(member, "property_identifier")
				}
				if mName == nil {
					mName = childByType(member, "identifier")
				}
				if mName != nil {
					mn := nodeText(mName, src)
					kind := "method"
					if member.Type() == "public_field_definition" || member.Type() == "field_definition" {
						kind = "field"
					}
					arity := 0
					if kind == "method" {
						if params := member.ChildByFieldName("parameters"); params != nil {
							for k := 0; k < int(params.NamedChildCount()); k++ {
								p := params.NamedChild(k)
								if p.Type() == "required_parameter" || p.Type() == "optional_parameter" {
									arity++
								}
							}
						}
					}
					methods = append(methods, types.Symbol{
						Name:    mn,
						Kind:    kind,
						File:    file,
						Line:    nodeLine(member),
						EndLine: nodeEndLine(member),
						Parent:  name,
						Arity:   arity,
					})
				}
			}
		}
	}
	return
}

func jsExtractVarDecl(node *sitter.Node, src []byte, file string, exported bool) []types.Symbol {
	var syms []types.Symbol
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

		syms = append(syms, types.Symbol{
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

func jsExtractInterface(node *sitter.Node, src []byte, file string) []types.Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	ifaceName := nodeText(nameNode, src)
	out := []types.Symbol{{
		Name:    ifaceName,
		Kind:    "interface",
		File:    file,
		Line:    nodeLine(node),
		EndLine: nodeEndLine(node),
		Doc:     prevSiblingComment(node, src),
	}}
	// Walk interface_body for method signatures so the typed
	// interface-implementation back-fill in build.go can derive
	// Symbol.RequiredMethods from the Parent links.
	if body := node.ChildByFieldName("body"); body != nil {
		for j := 0; j < int(body.NamedChildCount()); j++ {
			member := body.NamedChild(j)
			switch member.Type() {
			case "method_signature", "method_definition":
				mn := member.ChildByFieldName("name")
				if mn == nil {
					continue
				}
				arity := 0
				if params := member.ChildByFieldName("parameters"); params != nil {
					for k := 0; k < int(params.NamedChildCount()); k++ {
						p := params.NamedChild(k)
						if p.Type() == "required_parameter" || p.Type() == "optional_parameter" {
							arity++
						}
					}
				}
				out = append(out, types.Symbol{
					Name:    nodeText(mn, src),
					Kind:    "method",
					File:    file,
					Line:    nodeLine(member),
					EndLine: nodeEndLine(member),
					Parent:  ifaceName,
					Arity:   arity,
				})
			case "property_signature", "public_field_definition", "field_definition":
				mn := member.ChildByFieldName("name")
				if mn == nil {
					mn = childByType(member, "property_identifier")
				}
				if mn == nil {
					mn = childByType(member, "identifier")
				}
				if mn == nil {
					continue
				}
				out = append(out, types.Symbol{
					Name:    nodeText(mn, src),
					Kind:    "field",
					File:    file,
					Line:    nodeLine(member),
					EndLine: nodeEndLine(member),
					Parent:  ifaceName,
				})
			}
		}
	}
	return out
}

func jsExtractTypeAlias(node *sitter.Node, src []byte, file string) (types.Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return types.Symbol{}, false
	}
	return types.Symbol{
		Name:    nodeText(nameNode, src),
		Kind:    "type",
		File:    file,
		Line:    nodeLine(node),
		EndLine: nodeEndLine(node),
		Doc:     prevSiblingComment(node, src),
	}, true
}

func jsExtractEnum(node *sitter.Node, src []byte, file string) (types.Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return types.Symbol{}, false
	}
	return types.Symbol{
		Name:    nodeText(nameNode, src),
		Kind:    "enum",
		File:    file,
		Line:    nodeLine(node),
		EndLine: nodeEndLine(node),
		Doc:     prevSiblingComment(node, src),
	}, true
}

func jsExtractCalls(root *sitter.Node, src []byte, file string) []types.Relation {
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
				ResolvedBy: "js_ast_identifier_call",
			})
		case "member_expression":
			if prop := fn.ChildByFieldName("property"); prop != nil {
				rels = append(rels, types.Relation{
					Kind:       "call",
					FromEP:     types.RelationEndpoint{File: file, Line: nodeLine(fn)},
					ToEP:       types.RelationEndpoint{Name: nodeText(prop, src), File: file, Line: nodeLine(fn)},
					File:       file,
					Line:       nodeLine(fn),
					Confidence: types.ConfidenceAST,
					Provenance: types.ProvenanceTreeSitter,
					ResolvedBy: "js_ast_member_call",
				})
			}
		}
	})
	return rels
}
