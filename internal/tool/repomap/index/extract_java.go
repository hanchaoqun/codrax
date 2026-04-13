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
				Kind: "inheritance",
				From: file + ":" + name,
				To:   nodeText(id, src),
				File: file,
				Line: nodeLine(id),
			})
		}
	}

	// interfaces
	if ifaces := node.ChildByFieldName("interfaces"); ifaces != nil {
		walkNamedChildren(ifaces, true, func(ch *sitter.Node) {
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

	// extends
	if ext := node.ChildByFieldName("type_parameters"); ext != nil {
		walkNamedChildren(ext, true, func(ch *sitter.Node) {
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
			if params := member.ChildByFieldName("parameters"); params != nil {
				sig = nodeText(params, src)
				if len(sig) > 120 {
					sig = sig[:117] + "..."
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
				Kind: "call",
				From: file,
				To:   nodeText(nameNode, src),
				File: file,
				Line: nodeLine(nameNode),
			})
		}
	})
	return rels
}
