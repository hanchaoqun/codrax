package repomap

import (
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
)

func extractGo(root *sitter.Node, src []byte, file string) (pkg string, syms []Symbol, imps []Import, rels []Relation) {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		ch := root.NamedChild(i)
		switch ch.Type() {
		case "package_clause":
			if id := childByType(ch, "package_identifier"); id != nil {
				pkg = nodeText(id, src)
			}

		case "import_declaration":
			imps = append(imps, goExtractImports(ch, src, file)...)

		case "function_declaration":
			if s, ok := goExtractFunc(ch, src, file); ok {
				syms = append(syms, s)
			}

		case "method_declaration":
			if s, ok := goExtractMethod(ch, src, file); ok {
				syms = append(syms, s)
			}

		case "type_declaration":
			syms = append(syms, goExtractTypes(ch, src, file)...)
			rels = append(rels, goExtractEmbeddings(ch, src, file)...)

		case "const_declaration", "var_declaration":
			syms = append(syms, goExtractVarConst(ch, src, file)...)
		}
	}

	// extract call relations
	rels = append(rels, goExtractCalls(root, src, file)...)
	// extract type references
	rels = append(rels, goExtractTypeRefs(root, src, file)...)

	return
}

func goExtractImports(node *sitter.Node, src []byte, file string) []Import {
	var imps []Import
	// tree-sitter-go places grouped imports (import ( ... )) under an
	// import_spec_list child, not directly under import_declaration.
	// Verified against codrax/internal/orchestrator/orchestrator.go via
	// eval/repomap_v3/probe_ast (2026-04-13). Always check the
	// spec_list first; fall back to direct import_spec children for
	// parser variants; fall back to the single-import literal last.
	specs := childrenByType(node, "import_spec")
	if len(specs) == 0 {
		if list := childByType(node, "import_spec_list"); list != nil {
			specs = childrenByType(list, "import_spec")
		}
	}
	if len(specs) == 0 {
		// single import: import "path"
		if lit := childByType(node, "interpreted_string_literal"); lit != nil {
			path := strings.Trim(nodeText(lit, src), `"`)
			imps = append(imps, Import{
				Raw:  nodeText(node, src),
				Path: path,
				File: file,
				Line: nodeLine(node),
			})
		}
		return imps
	}
	for _, spec := range specs {
		var alias string
		if name := spec.ChildByFieldName("name"); name != nil {
			alias = nodeText(name, src)
		}
		if pathNode := spec.ChildByFieldName("path"); pathNode != nil {
			path := strings.Trim(nodeText(pathNode, src), `"`)
			imps = append(imps, Import{
				Raw:   nodeText(spec, src),
				Path:  path,
				Alias: alias,
				File:  file,
				Line:  nodeLine(spec),
			})
		}
	}
	return imps
}

func goExtractFunc(node *sitter.Node, src []byte, file string) (Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Symbol{}, false
	}
	name := nodeText(nameNode, src)
	sig := goFuncSignature(node, src)
	return Symbol{
		Name:      name,
		Kind:      "function",
		File:      file,
		Line:      nodeLine(node),
		EndLine:   nodeEndLine(node),
		Exported:  unicode.IsUpper(rune(name[0])),
		Signature: sig,
		Arity:     goFuncArity(node),
		Doc:       prevSiblingComment(node, src),
	}, true
}

func goExtractMethod(node *sitter.Node, src []byte, file string) (Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Symbol{}, false
	}
	name := nodeText(nameNode, src)

	var receiver string
	if recvNode := node.ChildByFieldName("receiver"); recvNode != nil {
		// parameter_list → parameter_declaration → type
		for j := 0; j < int(recvNode.NamedChildCount()); j++ {
			param := recvNode.NamedChild(j)
			if param.Type() == "parameter_declaration" {
				if t := param.ChildByFieldName("type"); t != nil {
					receiver = nodeText(t, src)
					receiver = strings.TrimPrefix(receiver, "*")
				}
			}
		}
	}

	sig := goFuncSignature(node, src)
	return Symbol{
		Name:      name,
		Kind:      "method",
		File:      file,
		Line:      nodeLine(node),
		EndLine:   nodeEndLine(node),
		Exported:  unicode.IsUpper(rune(name[0])),
		Receiver:  receiver,
		Signature: sig,
		Arity:     goFuncArity(node),
		Doc:       prevSiblingComment(node, src),
	}, true
}

// goFuncArity counts the number of parameters in a function or
// method declaration. tree-sitter-go exposes the parameters as a
// `parameter_list` under the `parameters` field, containing
// `parameter_declaration` nodes. A single parameter_declaration can
// bind multiple names (`func f(a, b int)`) — we count each name,
// not each declaration, so the arity matches the positional call
// arity the Go runtime uses.
func goFuncArity(node *sitter.Node) int {
	params := node.ChildByFieldName("parameters")
	if params == nil {
		return 0
	}
	count := 0
	for i := 0; i < int(params.NamedChildCount()); i++ {
		decl := params.NamedChild(i)
		if decl.Type() != "parameter_declaration" && decl.Type() != "variadic_parameter_declaration" {
			continue
		}
		// Count names under this declaration; fall back to 1 for
		// anonymous params (`func f(int)`).
		names := 0
		for j := 0; j < int(decl.NamedChildCount()); j++ {
			ch := decl.NamedChild(j)
			if ch.Type() == "identifier" {
				names++
			}
		}
		if names == 0 {
			names = 1
		}
		count += names
	}
	return count
}

func goExtractTypes(node *sitter.Node, src []byte, file string) []Symbol {
	var syms []Symbol
	for i := 0; i < int(node.NamedChildCount()); i++ {
		spec := node.NamedChild(i)
		if spec.Type() != "type_spec" {
			continue
		}
		nameNode := spec.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nodeText(nameNode, src)
		typeNode := spec.ChildByFieldName("type")
		kind := "type"
		if typeNode != nil {
			switch typeNode.Type() {
			case "struct_type":
				kind = "struct"
			case "interface_type":
				kind = "interface"
			}
		}
		syms = append(syms, Symbol{
			Name:     name,
			Kind:     kind,
			File:     file,
			Line:     nodeLine(spec),
			EndLine:  nodeEndLine(spec),
			Exported: unicode.IsUpper(rune(name[0])),
			Doc:      prevSiblingComment(node, src),
		})
	}
	return syms
}

func goExtractVarConst(node *sitter.Node, src []byte, file string) []Symbol {
	kind := "var"
	if node.Type() == "const_declaration" {
		kind = "const"
	}
	var syms []Symbol
	// specs are const_spec or var_spec
	for i := 0; i < int(node.NamedChildCount()); i++ {
		spec := node.NamedChild(i)
		if spec.Type() != "const_spec" && spec.Type() != "var_spec" {
			continue
		}
		nameNode := spec.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nodeText(nameNode, src)
		syms = append(syms, Symbol{
			Name:     name,
			Kind:     kind,
			File:     file,
			Line:     nodeLine(spec),
			EndLine:  nodeEndLine(spec),
			Exported: unicode.IsUpper(rune(name[0])),
		})
	}
	return syms
}

func goExtractEmbeddings(typeDecl *sitter.Node, src []byte, file string) []Relation {
	var rels []Relation
	for i := 0; i < int(typeDecl.NamedChildCount()); i++ {
		spec := typeDecl.NamedChild(i)
		if spec.Type() != "type_spec" {
			continue
		}
		nameNode := spec.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		hostName := nodeText(nameNode, src)

		typeNode := spec.ChildByFieldName("type")
		if typeNode == nil {
			continue
		}

		switch typeNode.Type() {
		case "struct_type":
			if body := childByType(typeNode, "field_declaration_list"); body != nil {
				for j := 0; j < int(body.NamedChildCount()); j++ {
					field := body.NamedChild(j)
					if field.Type() != "field_declaration" {
						continue
					}
					// embedded field: no name, just type
					if field.ChildByFieldName("name") == nil {
						if t := field.ChildByFieldName("type"); t != nil {
							embedded := nodeText(t, src)
							embedded = strings.TrimPrefix(embedded, "*")
							rels = append(rels, Relation{
								Kind: "embedding",
								From: file + ":" + hostName,
								To:   embedded,
								File: file,
								Line: nodeLine(field),
							})
						}
					}
				}
			}
		case "interface_type":
			// interface embedding
			if body := childByType(typeNode, "method_spec_list"); body != nil {
				for j := 0; j < int(body.NamedChildCount()); j++ {
					ch := body.NamedChild(j)
					if ch.Type() == "type_identifier" || ch.Type() == "qualified_type" {
						embedded := nodeText(ch, src)
						rels = append(rels, Relation{
							Kind: "inheritance",
							From: file + ":" + hostName,
							To:   embedded,
							File: file,
							Line: nodeLine(ch),
						})
					}
				}
			}
		}
	}
	return rels
}

func goExtractCalls(root *sitter.Node, src []byte, file string) []Relation {
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
			// Bare call: `fn(args)`. No receiver text.
			name := nodeText(fn, src)
			line := nodeLine(fn)
			rels = append(rels, Relation{
				Kind: "call",
				From: file,
				To:   name,
				File: file,
				Line: line,
				ToEP: RelationEndpoint{
					Name: name,
					File: file,
					Line: line,
				},
			})
		case "selector_expression":
			// Qualified call: `recv.Method(args)`. Record the raw
			// receiver text — it is either a variable name, a package
			// name, or a type name. The Phase 1 resolver in CallersOf
			// /rank heuristically classifies it against the package
			// symbol table; when ambiguous the call falls back to the
			// legacy name-only resolution.
			field := fn.ChildByFieldName("field")
			if field == nil {
				return
			}
			name := nodeText(field, src)
			line := nodeLine(fn)
			var receiver string
			if op := fn.ChildByFieldName("operand"); op != nil {
				receiver = nodeText(op, src)
				// Strip pointer/dereference and parentheses so the
				// raw receiver reads as an identifier. We accept
				// false conflations (e.g. `(*ptr).Method` and
				// `ptr.Method` become `ptr`) because the ambiguity
				// metric already downgrades them to heuristic
				// resolution downstream.
				receiver = strings.TrimPrefix(receiver, "*")
				receiver = strings.TrimPrefix(receiver, "&")
				receiver = strings.Trim(receiver, "()")
			}
			rels = append(rels, Relation{
				Kind: "call",
				From: file,
				To:   name,
				File: file,
				Line: line,
				ToEP: RelationEndpoint{
					Name:     name,
					Receiver: receiver,
					File:     file,
					Line:     line,
				},
			})
		}
	})
	return rels
}

func goExtractTypeRefs(root *sitter.Node, src []byte, file string) []Relation {
	var rels []Relation
	seen := make(map[string]bool)
	walkNamedChildren(root, true, func(node *sitter.Node) {
		if node.Type() != "type_identifier" {
			return
		}
		name := nodeText(node, src)
		key := file + "→" + name
		if seen[key] {
			return
		}
		seen[key] = true
		rels = append(rels, Relation{
			Kind: "type_usage",
			From: file,
			To:   name,
			File: file,
			Line: nodeLine(node),
		})
	})
	return rels
}

func goFuncSignature(node *sitter.Node, src []byte) string {
	params := node.ChildByFieldName("parameters")
	result := node.ChildByFieldName("result")
	sig := ""
	if params != nil {
		sig = nodeText(params, src)
	}
	if result != nil {
		sig += " " + nodeText(result, src)
	}
	if len(sig) > 120 {
		sig = sig[:117] + "..."
	}
	return sig
}
