package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func extractJava(root *sitter.Node, src []byte, file string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation) {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		ch := root.NamedChild(i)
		switch ch.Type() {
		case "package_declaration":
			if id := childByType(ch, "scoped_identifier"); id != nil {
				pkg = nodeText(id, src)
			} else if id := childByType(ch, "identifier"); id != nil {
				pkg = nodeText(id, src)
			}

		case "import_declaration":
			if id := childByType(ch, "scoped_identifier"); id != nil {
				path := nodeText(id, src)
				imps = append(imps, types.Import{
					Raw:  nodeText(ch, src),
					Path: path,
					File: file,
					Line: nodeLine(ch),
				})
			}

		case "class_declaration":
			cls, methods, classRels := javaExtractClass(ch, src, file)
			syms = append(syms, cls...)
			syms = append(syms, methods...)
			rels = append(rels, classRels...)

		case "interface_declaration":
			cls, methods, classRels := javaExtractInterface(ch, src, file)
			syms = append(syms, cls...)
			syms = append(syms, methods...)
			rels = append(rels, classRels...)

		case "enum_declaration":
			if s, ok := javaExtractEnum(ch, src, file); ok {
				syms = append(syms, s)
			}
		}
	}

	rels = append(rels, javaExtractCalls(root, src, file)...)
	// framework route → handler resolver (Spring @*Mapping annotations);
	// gated on the @RestController / @Controller class annotation —
	// no-op for files without an annotated controller class.
	routeSyms, routeRels := javaExtractRoutes(root, src, file)
	syms = append(syms, routeSyms...)
	rels = append(rels, routeRels...)
	return
}

func javaExtractClass(node *sitter.Node, src []byte, file string) (cls []types.Symbol, methods []types.Symbol, rels []types.Relation) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nodeText(nameNode, src)
	exported := javaHasModifier(node, src, "public")

	cls = append(cls, types.Symbol{
		Name:     name,
		Kind:     "class",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: exported,
		Doc:      prevSiblingComment(node, src),
	})

	// superclass
	if sc := node.ChildByFieldName("superclass"); sc != nil {
		if id := childByType(sc, "type_identifier"); id != nil {
			rels = append(rels, types.Relation{
				Kind:       "inheritance",
				FromEP:     types.RelationEndpoint{Name: name, File: file, Line: nodeLine(id)},
				ToEP:       types.RelationEndpoint{Name: nodeText(id, src), File: file, Line: nodeLine(id)},
				File:       file,
				Line:       nodeLine(id),
				Confidence: types.ConfidenceAST,
				Provenance: types.ProvenanceTreeSitter,
				ResolvedBy: "java_superclass",
			})
		}
	}

	// interfaces
	if ifaces := node.ChildByFieldName("interfaces"); ifaces != nil {
		walkNamedChildren(ifaces, true, func(ch *sitter.Node) {
			if ch.Type() == "type_identifier" {
				rels = append(rels, types.Relation{
					Kind:       "inheritance",
					FromEP:     types.RelationEndpoint{Name: name, File: file, Line: nodeLine(ch)},
					ToEP:       types.RelationEndpoint{Name: nodeText(ch, src), File: file, Line: nodeLine(ch)},
					File:       file,
					Line:       nodeLine(ch),
					Confidence: types.ConfidenceAST,
					Provenance: types.ProvenanceTreeSitter,
					ResolvedBy: "java_interface_impl",
				})
			}
		})
	}

	// body
	if body := node.ChildByFieldName("body"); body != nil {
		methods, rels = javaExtractMembers(body, src, file, name, methods, rels)
	}
	return
}

func javaExtractInterface(node *sitter.Node, src []byte, file string) (cls []types.Symbol, methods []types.Symbol, rels []types.Relation) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nodeText(nameNode, src)

	cls = append(cls, types.Symbol{
		Name:     name,
		Kind:     "interface",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: javaHasModifier(node, src, "public"),
		Doc:      prevSiblingComment(node, src),
	})

	// extends — tree-sitter-java parks an interface's extends clause
	// in an `extends_interfaces` named child (not a field; verified
	// against the vendored grammar: extends_interfaces → type_list →
	// type_identifier+). The old read of the "type_parameters" FIELD
	// never saw the extends clause and could fabricate bound-type
	// edges for `interface A<T extends X>`.
	if ext := childByType(node, "extends_interfaces"); ext != nil {
		walkNamedChildren(ext, true, func(ch *sitter.Node) {
			if ch.Type() == "type_identifier" {
				rels = append(rels, types.Relation{
					Kind:       "inheritance",
					FromEP:     types.RelationEndpoint{Name: name, File: file, Line: nodeLine(ch)},
					ToEP:       types.RelationEndpoint{Name: nodeText(ch, src), File: file, Line: nodeLine(ch)},
					File:       file,
					Line:       nodeLine(ch),
					Confidence: types.ConfidenceAST,
					Provenance: types.ProvenanceTreeSitter,
					ResolvedBy: "java_interface_extends",
				})
			}
		})
	}

	if body := node.ChildByFieldName("body"); body != nil {
		methods, rels = javaExtractMembers(body, src, file, name, methods, rels)
	}
	return
}

func javaExtractMembers(body *sitter.Node, src []byte, file, parent string, methods []types.Symbol, rels []types.Relation) ([]types.Symbol, []types.Relation) {
	for j := 0; j < int(body.NamedChildCount()); j++ {
		member := body.NamedChild(j)
		switch member.Type() {
		case "method_declaration", "constructor_declaration":
			nameNode := member.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			mname := nodeText(nameNode, src)
			var sig string
			arity := 0
			if params := member.ChildByFieldName("parameters"); params != nil {
				sig = nodeText(params, src)
				if len(sig) > 120 {
					sig = sig[:117] + "..."
				}
				for k := 0; k < int(params.NamedChildCount()); k++ {
					p := params.NamedChild(k)
					if p.Type() == "formal_parameter" || p.Type() == "spread_parameter" {
						arity++
					}
				}
			}
			methods = append(methods, types.Symbol{
				Name:      mname,
				Kind:      "method",
				File:      file,
				Line:      nodeLine(member),
				EndLine:   nodeEndLine(member),
				Exported:  javaHasModifier(member, src, "public"),
				Parent:    parent,
				Arity:     arity,
				Signature: sig,
				Doc:       prevSiblingComment(member, src),
			})
		case "field_declaration":
			// extract field names
			for k := 0; k < int(member.NamedChildCount()); k++ {
				decl := member.NamedChild(k)
				if decl.Type() == "variable_declarator" {
					if nameNode := decl.ChildByFieldName("name"); nameNode != nil {
						methods = append(methods, types.Symbol{
							Name:     nodeText(nameNode, src),
							Kind:     "field",
							File:     file,
							Line:     nodeLine(decl),
							EndLine:  nodeEndLine(decl),
							Exported: javaHasModifier(member, src, "public"),
							Parent:   parent,
						})
					}
				}
			}
		}
	}
	return methods, rels
}

func javaExtractEnum(node *sitter.Node, src []byte, file string) (types.Symbol, bool) {
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
		Exported: javaHasModifier(node, src, "public"),
		Doc:      prevSiblingComment(node, src),
	}, true
}

func javaHasModifier(node *sitter.Node, src []byte, modifier string) bool {
	if mods := childByType(node, "modifiers"); mods != nil {
		return strings.Contains(nodeText(mods, src), modifier)
	}
	return false
}

func javaExtractCalls(root *sitter.Node, src []byte, file string) []types.Relation {
	var rels []types.Relation
	walkNamedChildren(root, true, func(node *sitter.Node) {
		if node.Type() != "method_invocation" {
			return
		}
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			rels = append(rels, types.Relation{
				Kind:       "call",
				FromEP:     types.RelationEndpoint{File: file, Line: nodeLine(nameNode)},
				ToEP:       types.RelationEndpoint{Name: nodeText(nameNode, src), File: file, Line: nodeLine(nameNode)},
				File:       file,
				Line:       nodeLine(nameNode),
				Confidence: types.ConfidenceAST,
				Provenance: types.ProvenanceTreeSitter,
				ResolvedBy: "java_method_invocation",
			})
		}
	})
	return rels
}
