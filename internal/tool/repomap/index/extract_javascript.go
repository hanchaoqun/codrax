package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	coretypes "github.com/hanchaoqun/codrax/internal/types"
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

	rels = append(rels, jsExtractCalls(root, src, file, isTS)...)

	// Express + NestJS route -> handler post-pass (route_javascript.go).
	// Runs after the top-level loop so it can gate on the per-file
	// imports already collected. extractJS is shared with the ArkTS
	// Tier-1 path (extract_arkts.go): the pass MUST stay inert there —
	// its verbatim "express" / "@nestjs/common" gates structurally
	// cannot fire on HarmonyOS imports (pinned by the ArkTS no-op
	// regression test in route_javascript_test.go).
	routeSyms, routeRels := jsExtractRoutes(root, src, file, imps)
	syms = append(syms, routeSyms...)
	rels = append(rels, routeRels...)
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
			sig = coretypes.CutPrefixRuneSafe(sig, 117) + "..."
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

func jsExtractCalls(root *sitter.Node, src []byte, file string, typed bool) []types.Relation {
	var rels []types.Relation
	// JavaScript lacks declaration annotations, but `new Type(...)` is still a
	// precise constructor binding. The shared census retains source identities
	// for everything else, so it is safe for JS as well as TS/ArkTS.
	receiverDeclarations := jsReceiverDeclarations(root, src)
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
				receiver := ""
				if object := fn.ChildByFieldName("object"); object != nil {
					receiver = strings.TrimSpace(nodeText(object, src))
				}
				// `this` is a parser-owned receiver identity inside a named
				// class. Resolve it directly to that class instead of leaving
				// `this.method` disconnected from the method definition. An
				// anonymous or malformed class stays unresolved.
				if receiver == "this" {
					receiver = jsEnclosingNamedClass(node, src)
				} else if binding := jsReceiverBinding(receiver); binding != "" && receiverDeclarations != nil {
					if receiverType, declared := lexicalReceiverTypeAt(node, binding, receiverDeclarations, jsReceiverScopeBoundary); declared && receiverType != "" {
						receiver = receiverType
					}
				}
				rels = append(rels, types.Relation{
					Kind:       "call",
					FromEP:     types.RelationEndpoint{File: file, Line: nodeLine(fn)},
					ToEP:       types.RelationEndpoint{Name: nodeText(prop, src), Receiver: receiver, File: file, Line: nodeLine(fn)},
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

func jsReceiverDeclarations(root *sitter.Node, src []byte) lexicalReceiverAuthorities {
	declarations := make(lexicalReceiverAuthorities)
	constructors := jsStaticConstructorAuthorities(root, src)
	walkNamedChildren(root, true, func(node *sitter.Node) {
		switch node.Type() {
		case "required_parameter", "optional_parameter", "variable_declarator", "public_field_definition":
			name := node.ChildByFieldName("pattern")
			if name == nil {
				name = node.ChildByFieldName("name")
			}
			if name != nil {
				typeName := jsDeclaredTypeName(node.ChildByFieldName("type"), src)
				// A direct `new Type(...)` initializer is an exact AST type
				// binding even in JavaScript and in TypeScript code that omits
				// a redundant annotation. Do not infer from arbitrary calls,
				// assignments, text, or constructor-looking names.
				scope := lexicalReceiverBindingScope(node, root, jsReceiverScopeBoundary)
				scopeWide := node.Type() != "variable_declarator"
				if typeName == "" && node.Type() == "variable_declarator" && jsConstVariableDeclarator(node) {
					value := node.ChildByFieldName("value")
					constructor := jsNewExpressionConstructorType(value, src)
					if constructorType, declared := lexicalReceiverTypeAt(value, constructor, constructors, jsReceiverScopeBoundary); declared {
						typeName = constructorType
					}
				}
				addScopedReceiverAuthority(declarations, scope, node, nodeText(name, src), typeName, scopeWide)
			}
		}
	})
	return declarations
}

// jsStaticConstructorAuthorities is the precise lexical identity admission set
// for initializer-derived receiver types. Named classes and imported bindings
// carry stable identities; parameters, local variables and function bindings
// are recorded as shadows so `new Ctor()` cannot borrow an outer import merely
// because the source names match.
func jsStaticConstructorAuthorities(root *sitter.Node, src []byte) lexicalReceiverAuthorities {
	authorities := make(lexicalReceiverAuthorities)
	walkNamedChildren(root, true, func(node *sitter.Node) {
		switch node.Type() {
		case "class_declaration":
			if name := node.ChildByFieldName("name"); name != nil {
				if value := strings.TrimSpace(nodeText(name, src)); value != "" {
					scope := lexicalReceiverBindingScope(node, root, jsReceiverScopeBoundary)
					addScopedReceiverAuthority(authorities, scope, node, value, value, false)
				}
			}
		case "import_statement":
			for local, declared := range jsImportedConstructorBindings(node, src) {
				addScopedReceiverAuthority(authorities, root, node, local, declared, true)
			}
		case "required_parameter", "optional_parameter", "variable_declarator":
			name := node.ChildByFieldName("pattern")
			if name == nil {
				name = node.ChildByFieldName("name")
			}
			if name != nil {
				scope := lexicalReceiverBindingScope(node, root, jsReceiverScopeBoundary)
				addScopedReceiverAuthority(authorities, scope, node, nodeText(name, src), "", node.Type() != "variable_declarator")
			}
		case "function_declaration", "generator_function_declaration":
			if name := node.ChildByFieldName("name"); name != nil {
				scope := lexicalReceiverBindingScope(node, root, jsReceiverScopeBoundary)
				addScopedReceiverAuthority(authorities, scope, node, nodeText(name, src), "", true)
			}
		}
	})
	return authorities
}

// jsImportedConstructorBindings returns only local import bindings. For a
// named alias (`Imported as Local`) the local source name resolves to the
// declared identity, while namespace objects are deliberately excluded because
// member-expression constructors require a separate qualified-name carrier.
func jsImportedConstructorBindings(node *sitter.Node, src []byte) map[string]string {
	out := make(map[string]string)
	walkNamedChildren(node, true, func(child *sitter.Node) {
		switch child.Type() {
		case "import_specifier":
			name := child.ChildByFieldName("name")
			if name == nil {
				return
			}
			declared := strings.TrimSpace(nodeText(name, src))
			local := declared
			if alias := child.ChildByFieldName("alias"); alias != nil {
				local = strings.TrimSpace(nodeText(alias, src))
			}
			if local != "" && declared != "" {
				out[local] = declared
			}
		case "identifier":
			// A direct identifier child of import_clause is the default
			// binding. Identifiers inside import_specifier are handled above.
			if parent := child.Parent(); parent != nil && parent.Type() == "import_clause" {
				local := strings.TrimSpace(nodeText(child, src))
				if local != "" {
					out[local] = local
				}
			}
		}
	})
	return out
}

func jsConstVariableDeclarator(node *sitter.Node) bool {
	if node == nil || node.Type() != "variable_declarator" {
		return false
	}
	parent := node.Parent()
	if parent == nil || parent.Type() != "lexical_declaration" {
		return false
	}
	for i := 0; i < int(parent.ChildCount()); i++ {
		if child := parent.Child(i); child != nil && child.Type() == "const" {
			return true
		}
	}
	return false
}

func jsReceiverScopeBoundary(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	switch node.Type() {
	case "statement_block", "function_declaration", "generator_function_declaration", "function_expression", "generator_function",
		"arrow_function", "method_definition", "class_declaration", "class":
		return true
	default:
		return false
	}
}

func jsDeclaredTypeName(node *sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	switch node.Type() {
	case "type_identifier", "predefined_type":
		return strings.TrimSpace(nodeText(node, src))
	case "generic_type":
		if name := node.ChildByFieldName("name"); name != nil {
			return jsDeclaredTypeName(name, src)
		}
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		if name := jsDeclaredTypeName(node.NamedChild(i), src); name != "" {
			return name
		}
	}
	return ""
}

func jsReceiverBinding(receiver string) string {
	receiver = strings.TrimSpace(receiver)
	if receiver == "" {
		return ""
	}
	if idx := strings.LastIndex(receiver, "."); idx >= 0 {
		receiver = receiver[idx+1:]
	}
	return strings.TrimSpace(receiver)
}

// jsNewExpressionConstructorType returns only the direct identifier carried
// by a `new Type(...)` AST node. Member expressions, factory calls, casts and
// dynamic constructor values deliberately remain unresolved: this helper is
// receiver identity authority, not a name-shape heuristic.
func jsNewExpressionConstructorType(node *sitter.Node, src []byte) string {
	if node == nil || node.Type() != "new_expression" {
		return ""
	}
	constructor := node.ChildByFieldName("constructor")
	if constructor == nil && node.NamedChildCount() > 0 {
		constructor = node.NamedChild(0)
	}
	if constructor == nil {
		return ""
	}
	switch constructor.Type() {
	case "identifier", "type_identifier":
		return strings.TrimSpace(nodeText(constructor, src))
	default:
		return ""
	}
}

// jsEnclosingNamedClass resolves the language-defined `this` receiver from
// tree structure only. Arrow functions preserve the surrounding `this`, while
// ordinary functions and non-class methods bind their own receiver and must
// stop the walk. This prevents a nested callback from borrowing class identity.
func jsEnclosingNamedClass(node *sitter.Node, src []byte) string {
	for current := node; current != nil; current = current.Parent() {
		switch current.Type() {
		case "function", "function_declaration", "function_expression", "generator_function", "generator_function_declaration":
			return ""
		case "method_definition":
			body := current.Parent()
			if body == nil || body.Type() != "class_body" {
				return ""
			}
			class := body.Parent()
			if class == nil || class.Type() != "class_declaration" {
				return ""
			}
			return jsNamedClassName(class, src)
		case "class_declaration":
			// Class field initializers and static blocks can reach the class
			// without crossing a method_definition. Arrow functions nested
			// inside those constructs intentionally remain transparent.
			return jsNamedClassName(current, src)
		}
	}
	return ""
}

func jsNamedClassName(class *sitter.Node, src []byte) string {
	if class == nil || class.Type() != "class_declaration" {
		return ""
	}
	name := class.ChildByFieldName("name")
	if name == nil {
		return ""
	}
	return strings.TrimSpace(nodeText(name, src))
}
