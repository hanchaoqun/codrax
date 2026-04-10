package repomap

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

func extractCCpp(root *sitter.Node, src []byte, file, lang string) (pkg string, syms []Symbol, imps []Import, rels []Relation) {
	isCpp := lang == LangCpp
	isHeader := strings.HasSuffix(file, ".h") || strings.HasSuffix(file, ".hpp") || strings.HasSuffix(file, ".hh")

	for i := 0; i < int(root.NamedChildCount()); i++ {
		ch := root.NamedChild(i)
		switch ch.Type() {
		case "preproc_include":
			if path := ch.ChildByFieldName("path"); path != nil {
				p := nodeText(path, src)
				p = strings.Trim(p, `"<>`)
				imps = append(imps, Import{
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
			syms = append(syms, cExtractDeclaration(ch, src, file, isHeader)...)

		case "struct_specifier":
			if s, ok := cExtractStruct(ch, src, file, isHeader); ok {
				syms = append(syms, s)
			}

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

func cExtractFunc(node *sitter.Node, src []byte, file string, isHeader bool) (Symbol, bool) {
	declr := node.ChildByFieldName("declarator")
	if declr == nil {
		return Symbol{}, false
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
		return Symbol{}, false
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

	return Symbol{
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

func cExtractDeclaration(node *sitter.Node, src []byte, file string, isHeader bool) []Symbol {
	var syms []Symbol
	// look for function declarators (prototypes) in the declaration
	walkNamedChildren(node, false, func(ch *sitter.Node) {
		if ch.Type() == "function_declarator" {
			if nameNode := ch.ChildByFieldName("declarator"); nameNode != nil {
				name := nodeText(nameNode, src)
				name = strings.TrimLeft(name, "*& ")
				if name != "" {
					syms = append(syms, Symbol{
						Name:     name,
						Kind:     "function",
						File:     file,
						Line:     nodeLine(node),
						EndLine:  nodeEndLine(node),
						Exported: isHeader,
					})
				}
			}
		}
	})
	return syms
}

func cExtractStruct(node *sitter.Node, src []byte, file string, isHeader bool) (Symbol, bool) {
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
		Exported: isHeader,
		Doc:      prevSiblingComment(node, src),
	}, true
}

func cExtractEnumSpec(node *sitter.Node, src []byte, file string, isHeader bool) (Symbol, bool) {
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
		Exported: isHeader,
	}, true
}

func cExtractTypedef(node *sitter.Node, src []byte, file string, isHeader bool) (Symbol, bool) {
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
		return Symbol{}, false
	}
	return Symbol{
		Name:     name,
		Kind:     "type",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: isHeader,
	}, true
}

func cppExtractClass(node *sitter.Node, src []byte, file string, isHeader bool) (cls []Symbol, methods []Symbol, rels []Relation) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nodeText(nameNode, src)
	cls = append(cls, Symbol{
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
				decls := cExtractDeclaration(member, src, file, isHeader)
				for idx := range decls {
					decls[idx].Parent = name
				}
				methods = append(methods, decls...)
			case "access_specifier":
				// skip
			}
		}
	}
	return
}

func cExtractCalls(root *sitter.Node, src []byte, file string) []Relation {
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
