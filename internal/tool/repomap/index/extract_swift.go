package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// extractSwift walks tree-sitter-swift output for top-level decls.
// Swift's grammar collapses several declaration forms into
// class_declaration, so the extractor derives the real kind from the
// source text and child nodes. The supported surface includes:
//
//   - function_declaration             → top-level func / method
//   - class_declaration                → class / struct / enum / actor / extension
//   - protocol_declaration             → protocol
//   - init_declaration                 → ctor
//   - subscript_declaration            → method
//   - import_declaration               → import path
//
// Inheritance / conformance edges land on `inheritance` Relations
// keyed by "<file>:<symbol>" → "<base>" same shape as Python.
func extractSwift(root *sitter.Node, src []byte, file string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation) {
	pkg = swiftDerivePackage(file)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		ch := root.NamedChild(i)
		switch ch.Type() {
		case "import_declaration":
			if imp, ok := swiftExtractImport(ch, src, file); ok {
				imps = append(imps, imp)
			}
		case "function_declaration":
			if s, ok := swiftExtractFunc(ch, src, file, ""); ok {
				syms = append(syms, s)
			}
		case "class_declaration":
			t, methods, classRels := swiftExtractType(ch, src, file)
			syms = append(syms, t...)
			syms = append(syms, methods...)
			rels = append(rels, classRels...)
		case "protocol_declaration":
			t, methods, classRels := swiftExtractType(ch, src, file)
			// Override kind on the head symbol to "protocol".
			for k := range t {
				t[k].Kind = "protocol"
			}
			syms = append(syms, t...)
			syms = append(syms, methods...)
			rels = append(rels, classRels...)
		case "property_declaration":
			if s, ok := swiftExtractProperty(ch, src, file, ""); ok {
				syms = append(syms, s)
			}
		}
	}
	return
}

func swiftDerivePackage(file string) string {
	parts := strings.Split(file, "/")
	for i, p := range parts {
		if (p == "Sources" || p == "Tests") && i+1 < len(parts)-1 {
			return parts[i+1]
		}
	}
	if len(parts) > 1 {
		return parts[len(parts)-2]
	}
	return ""
}

func swiftExtractImport(node *sitter.Node, src []byte, file string) (types.Import, bool) {
	// Find the identifier child (the imported module name).
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() == "identifier" || ch.Type() == "simple_identifier" {
			name := strings.TrimSpace(nodeText(ch, src))
			if name == "" {
				continue
			}
			return types.Import{
				Raw:  nodeText(node, src),
				Path: name,
				File: file,
				Line: nodeLine(node),
			}, true
		}
	}
	return types.Import{}, false
}

func swiftExtractFunc(node *sitter.Node, src []byte, file, parent string) (types.Symbol, bool) {
	nameNode := childByType(node, "simple_identifier")
	if nameNode == nil {
		nameNode = childByType(node, "identifier")
	}
	if nameNode == nil {
		return types.Symbol{}, false
	}
	name := nodeText(nameNode, src)
	if name == "" {
		return types.Symbol{}, false
	}
	kind := "function"
	if parent != "" {
		kind = "method"
	}
	exported := !swiftHasModifier(node, src, "private") && !swiftHasModifier(node, src, "fileprivate")
	arity := 0
	for k := 0; k < int(node.NamedChildCount()); k++ {
		if node.NamedChild(k).Type() == "parameter" {
			arity++
		}
	}
	return types.Symbol{
		Name:     name,
		Kind:     kind,
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: exported,
		Parent:   parent,
		Arity:    arity,
	}, true
}

func swiftExtractType(node *sitter.Node, src []byte, file string) (cls []types.Symbol, methods []types.Symbol, rels []types.Relation) {
	name := swiftTypeName(node, src)
	if name == "" {
		return
	}
	kind := swiftDeclKind(node, src)
	cls = append(cls, types.Symbol{
		Name:     name,
		Kind:     kind,
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: !swiftHasModifier(node, src, "private") && !swiftHasModifier(node, src, "fileprivate"),
	})
	// Walk body for methods and inherited types.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		switch ch.Type() {
		case "type_inheritance_clause", "inheritance_specifier":
			for j := 0; j < int(ch.NamedChildCount()); j++ {
				base := ch.NamedChild(j)
				baseName := strings.TrimSpace(nodeText(base, src))
				if baseName == "" {
					continue
				}
				rels = append(rels, types.Relation{
					Kind: "inheritance",
					From: file + ":" + name,
					To:   baseName,
					File: file,
					Line: nodeLine(base),
				})
			}
		case "class_body", "protocol_body", "enum_class_body":
			for j := 0; j < int(ch.NamedChildCount()); j++ {
				member := ch.NamedChild(j)
				switch member.Type() {
				case "function_declaration", "protocol_function_declaration",
					"protocol_property_declaration":
					if s, ok := swiftExtractFunc(member, src, file, name); ok {
						methods = append(methods, s)
					}
				case "init_declaration":
					if s, ok := swiftExtractInit(member, src, file, name); ok {
						methods = append(methods, s)
					}
				case "subscript_declaration":
					if s, ok := swiftExtractSubscript(member, src, file, name); ok {
						methods = append(methods, s)
					}
				case "property_declaration":
					if s, ok := swiftExtractProperty(member, src, file, name); ok {
						methods = append(methods, s)
					}
				}
			}
		}
	}
	return
}

func swiftTypeName(node *sitter.Node, src []byte) string {
	if nameNode := childByType(node, "type_identifier"); nameNode != nil {
		return nodeText(nameNode, src)
	}
	if nameNode := childByType(node, "simple_identifier"); nameNode != nil {
		return nodeText(nameNode, src)
	}
	if userType := childByType(node, "user_type"); userType != nil {
		if nameNode := childByType(userType, "type_identifier"); nameNode != nil {
			return nodeText(nameNode, src)
		}
		if nameNode := childByType(userType, "simple_identifier"); nameNode != nil {
			return nodeText(nameNode, src)
		}
	}
	return ""
}

func swiftDeclKind(node *sitter.Node, src []byte) string {
	if node.Type() == "protocol_declaration" {
		return "protocol"
	}
	text := strings.TrimSpace(nodeText(node, src))
	switch {
	case strings.HasPrefix(text, "struct "):
		return "struct"
	case strings.HasPrefix(text, "enum "):
		return "enum"
	case strings.HasPrefix(text, "actor "):
		return "actor"
	case strings.HasPrefix(text, "extension "):
		return "extend"
	default:
		return "class"
	}
}

func swiftExtractInit(node *sitter.Node, src []byte, file, parent string) (types.Symbol, bool) {
	if parent == "" {
		return types.Symbol{}, false
	}
	return types.Symbol{
		Name:     "init",
		Kind:     "ctor",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: !swiftHasModifier(node, src, "private") && !swiftHasModifier(node, src, "fileprivate"),
		Parent:   parent,
	}, true
}

func swiftExtractSubscript(node *sitter.Node, src []byte, file, parent string) (types.Symbol, bool) {
	if parent == "" {
		return types.Symbol{}, false
	}
	return types.Symbol{
		Name:     "subscript",
		Kind:     "method",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: !swiftHasModifier(node, src, "private") && !swiftHasModifier(node, src, "fileprivate"),
		Parent:   parent,
	}, true
}

func swiftExtractProperty(node *sitter.Node, src []byte, file, parent string) (types.Symbol, bool) {
	nameNode := childByType(node, "simple_identifier")
	if nameNode == nil {
		nameNode = childByType(node, "identifier")
	}
	if nameNode == nil {
		return types.Symbol{}, false
	}
	name := nodeText(nameNode, src)
	if name == "" {
		return types.Symbol{}, false
	}
	return types.Symbol{
		Name:     name,
		Kind:     "field",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: !swiftHasModifier(node, src, "private"),
		Parent:   parent,
	}, true
}

func swiftHasModifier(node *sitter.Node, src []byte, modifier string) bool {
	mods := childByType(node, "modifiers")
	if mods == nil {
		return false
	}
	return strings.Contains(nodeText(mods, src), modifier)
}
