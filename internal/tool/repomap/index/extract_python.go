package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func extractPython(root *sitter.Node, src []byte, file string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation) {
	// Python "package" is the module name derived from the file path
	pkg = strings.TrimSuffix(strings.ReplaceAll(file, "/", "."), ".py")

	for i := 0; i < int(root.NamedChildCount()); i++ {
		ch := root.NamedChild(i)
		switch ch.Type() {
		case "import_statement":
			imps = append(imps, pyExtractImport(ch, src, file)...)
		case "import_from_statement":
			imps = append(imps, pyExtractFromImport(ch, src, file)...)
		case "function_definition":
			if s, ok := pyExtractFunc(ch, src, file, ""); ok {
				syms = append(syms, s)
			}
		case "decorated_definition":
			syms = append(syms, pyExtractDecorated(ch, src, file, "")...)
		case "class_definition":
			cls, methods, classRels := pyExtractClass(ch, src, file)
			syms = append(syms, cls...)
			syms = append(syms, methods...)
			rels = append(rels, classRels...)
		}
	}

	rels = append(rels, pyExtractCalls(root, src, file)...)

	// Framework route -> handler post-passes (route_python.go):
	// FastAPI + Flask, each behind its own precise per-file import
	// gate, deduped by (verb, path, line). Runs after the import loop
	// above so the passes can gate before doing any AST walking.
	routeSyms, routeRels := pyExtractRoutes(root, src, file, imps)
	syms = append(syms, routeSyms...)
	rels = append(rels, routeRels...)
	return
}

func pyExtractImport(node *sitter.Node, src []byte, file string) []types.Import {
	// import foo, import foo.bar
	var imps []types.Import
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() == "dotted_name" || ch.Type() == "aliased_import" {
			path := nodeText(ch, src)
			var alias string
			if ch.Type() == "aliased_import" {
				if nameNode := ch.ChildByFieldName("alias"); nameNode != nil {
					alias = nodeText(nameNode, src)
				}
				if nameNode := ch.ChildByFieldName("name"); nameNode != nil {
					path = nodeText(nameNode, src)
				}
			}
			imps = append(imps, types.Import{
				Raw:   nodeText(node, src),
				Path:  path,
				Alias: alias,
				File:  file,
				Line:  nodeLine(node),
			})
		}
	}
	return imps
}

func pyExtractFromImport(node *sitter.Node, src []byte, file string) []types.Import {
	// from foo import bar, baz
	modNode := node.ChildByFieldName("module_name")
	if modNode == nil {
		// try dotted_name child
		modNode = childByType(node, "dotted_name")
	}
	modName := ""
	if modNode != nil {
		modName = nodeText(modNode, src)
	}
	// relative import prefix
	if strings.Contains(nodeText(node, src), "from .") && modName == "" {
		modName = "."
	}

	var imps []types.Import
	imps = append(imps, types.Import{
		Raw:  nodeText(node, src),
		Path: modName,
		File: file,
		Line: nodeLine(node),
	})
	return imps
}

func pyExtractFunc(node *sitter.Node, src []byte, file, parent string) (types.Symbol, bool) {
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
	if params := node.ChildByFieldName("parameters"); params != nil {
		sig = nodeText(params, src)
		if len(sig) > 120 {
			sig = sig[:117] + "..."
		}
	}

	return types.Symbol{
		Name:      name,
		Kind:      kind,
		File:      file,
		Line:      nodeLine(node),
		EndLine:   nodeEndLine(node),
		Exported:  !strings.HasPrefix(name, "_"),
		Parent:    parent,
		Signature: sig,
		Doc:       pyDocstring(node, src),
	}, true
}

func pyExtractDecorated(node *sitter.Node, src []byte, file, parent string) []types.Symbol {
	var syms []types.Symbol
	// decorated_definition wraps function_definition or class_definition
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		switch ch.Type() {
		case "function_definition":
			if s, ok := pyExtractFunc(ch, src, file, parent); ok {
				syms = append(syms, s)
			}
		case "class_definition":
			cls, methods, _ := pyExtractClass(ch, src, file)
			syms = append(syms, cls...)
			syms = append(syms, methods...)
		}
	}
	return syms
}

func pyExtractClass(node *sitter.Node, src []byte, file string) (cls []types.Symbol, methods []types.Symbol, rels []types.Relation) {
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
		Exported: !strings.HasPrefix(name, "_"),
		Doc:      pyDocstring(node, src),
	})

	// superclasses → inheritance
	if bases := node.ChildByFieldName("superclasses"); bases != nil {
		for j := 0; j < int(bases.NamedChildCount()); j++ {
			base := bases.NamedChild(j)
			baseName := nodeText(base, src)
			if baseName != "" {
				rels = append(rels, types.Relation{
					Kind:       "inheritance",
					FromEP:     types.RelationEndpoint{Name: name, File: file, Line: nodeLine(base)},
					ToEP:       types.RelationEndpoint{Name: baseName, File: file, Line: nodeLine(base)},
					File:       file,
					Line:       nodeLine(base),
					Confidence: types.ConfidenceAST,
					Provenance: types.ProvenanceTreeSitter,
					ResolvedBy: "python_base_class",
				})
			}
		}
	}

	// body → methods
	if body := node.ChildByFieldName("body"); body != nil {
		for j := 0; j < int(body.NamedChildCount()); j++ {
			ch := body.NamedChild(j)
			switch ch.Type() {
			case "function_definition":
				if s, ok := pyExtractFunc(ch, src, file, name); ok {
					methods = append(methods, s)
				}
			case "decorated_definition":
				methods = append(methods, pyExtractDecorated(ch, src, file, name)...)
			case "expression_statement":
				if s, ok := pyExtractClassField(ch, src, file, name); ok {
					methods = append(methods, s)
				}
			}
		}
	}
	return
}

func pyExtractClassField(node *sitter.Node, src []byte, file, parent string) (types.Symbol, bool) {
	if node == nil || parent == "" || node.NamedChildCount() == 0 {
		return types.Symbol{}, false
	}
	assign := node.NamedChild(0)
	if assign == nil || assign.Type() != "assignment" {
		return types.Symbol{}, false
	}
	left := assign.ChildByFieldName("left")
	if left == nil {
		left = assign.NamedChild(0)
	}
	if left == nil || left.Type() != "identifier" {
		return types.Symbol{}, false
	}
	name := strings.TrimSpace(nodeText(left, src))
	if name == "" {
		return types.Symbol{}, false
	}
	return types.Symbol{
		Name:     name,
		Kind:     "field",
		File:     file,
		Line:     nodeLine(assign),
		EndLine:  nodeEndLine(assign),
		Exported: !strings.HasPrefix(name, "_"),
		Parent:   parent,
	}, true
}

func pyDocstring(node *sitter.Node, src []byte) string {
	body := node.ChildByFieldName("body")
	if body == nil {
		return ""
	}
	if body.NamedChildCount() == 0 {
		return ""
	}
	first := body.NamedChild(0)
	if first.Type() == "expression_statement" && first.NamedChildCount() > 0 {
		expr := first.NamedChild(0)
		if expr.Type() == "string" || expr.Type() == "concatenated_string" {
			doc := nodeText(expr, src)
			doc = strings.Trim(doc, `"'`)
			return firstLine(doc)
		}
	}
	return ""
}

func pyExtractCalls(root *sitter.Node, src []byte, file string) []types.Relation {
	var rels []types.Relation
	walkNamedChildren(root, true, func(node *sitter.Node) {
		if node.Type() != "call" {
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
				ResolvedBy: "python_ast_identifier_call",
			})
		case "attribute":
			if attr := fn.ChildByFieldName("attribute"); attr != nil {
				rels = append(rels, types.Relation{
					Kind:       "call",
					FromEP:     types.RelationEndpoint{File: file, Line: nodeLine(fn)},
					ToEP:       types.RelationEndpoint{Name: nodeText(attr, src), File: file, Line: nodeLine(fn)},
					File:       file,
					Line:       nodeLine(fn),
					Confidence: types.ConfidenceAST,
					Provenance: types.ProvenanceTreeSitter,
					ResolvedBy: "python_ast_attribute_call",
				})
			}
		}
	})
	return rels
}
