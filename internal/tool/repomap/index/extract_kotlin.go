package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// extractKotlin walks a parsed Kotlin file and extracts the
// top-level declarations. The signature matches the other
// tree-sitter extractors so parser.go's switch can dispatch to it
// uniformly.
//
// Kotlin surface that matters for repomap:
//
//   - package_header          → FileInfo.Package
//   - import_header           → types.Import
//   - function_declaration    → Symbol{Kind:function}; modifier-aware
//     (suspend / inline / operator / infix / internal / private)
//   - class_declaration       → Symbol{Kind:class|data-class|sealed|
//     interface|enum|object}; modifier-aware. The same grammar
//     node represents interfaces / enums / data classes; the
//     modifier flags produced by tree-sitter tell us which.
//   - object_declaration      → Symbol{Kind:object}. Kotlin objects
//     are singleton class definitions; treated here as a class
//     with kind="object" so the graph can still trace calls.
//   - property_declaration    → top-level val / var → Symbol{Kind:var|const}.
//     Inside a class these become fields; the extractor treats
//     top-level and member properties uniformly.
//   - type_alias              → Symbol{Kind:type}
//
// Kotlin frames surfaced in an Android panic look like:
//
//	at com.example.ui.MainActivity$onCreate(MainActivity.kt:42)
//
// i.e. JVM-style. The existing Java basename resolver in
// internal/analysis/logtriage does NOT pick up .kt frames because
// it is ext-gated to .java. logtriage.isKotlinBasename and
// ResolveKotlinFile (see resolver_harmonyos.go) supply the parallel
// path for Kotlin.
func extractKotlin(root *sitter.Node, src []byte, file string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation) {
	if root == nil {
		return
	}

	walkKotlinDecls(root, src, file, "", &pkg, &syms, &imps, &rels)
	return
}

// walkKotlinDecls recurses into the tree-sitter tree. `parent` is the
// name of the containing class / object, used as Symbol.Parent for
// member declarations.
func walkKotlinDecls(node *sitter.Node, src []byte, file, parent string, pkg *string, syms *[]types.Symbol, imps *[]types.Import, rels *[]types.Relation) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "package_header":
		// Shape: `package` IDENT (`.` IDENT)*
		if name := collectKotlinQualifiedName(node, src); name != "" {
			*pkg = name
		}
		return

	case "import_header":
		// Shape: `import` IDENT (`.` IDENT)* (`as` IDENT)?
		raw := nodeText(node, src)
		path := collectKotlinImportPath(node, src)
		alias := collectKotlinImportAlias(node, src)
		if path != "" {
			*imps = append(*imps, types.Import{
				Raw:   strings.TrimSpace(raw),
				Path:  path,
				Alias: alias,
				File:  file,
				Line:  nodeLine(node),
			})
		}
		return

	case "class_declaration":
		sym := extractKotlinTypeDecl(node, src, file, parent, "class")
		if sym.Name != "" {
			*syms = append(*syms, sym)
			// Walk into class body to pick up nested declarations.
			// tree-sitter-kotlin does not expose `body` as a named
			// field on class_declaration; we must iterate NamedChildren
			// looking for `class_body` / `enum_class_body`.
			for i := 0; i < int(node.NamedChildCount()); i++ {
				body := node.NamedChild(i)
				if body == nil {
					continue
				}
				if body.Type() != "class_body" && body.Type() != "enum_class_body" {
					continue
				}
				for j := 0; j < int(body.NamedChildCount()); j++ {
					walkKotlinDecls(body.NamedChild(j), src, file, sym.Name, pkg, syms, imps, rels)
				}
			}
			// Inheritance edges: `:` SUPER_TYPE list
			for _, superName := range kotlinSuperTypes(node, src) {
				*rels = append(*rels, types.Relation{
					Kind: "inheritance",
					From: file + ":" + sym.Name,
					To:   superName,
					File: file,
					Line: sym.Line,
					ToEP: types.RelationEndpoint{Name: superName, File: file, Line: sym.Line},
				})
			}
		}
		return

	case "object_declaration":
		sym := extractKotlinTypeDecl(node, src, file, parent, "object")
		if sym.Name != "" {
			*syms = append(*syms, sym)
			for i := 0; i < int(node.NamedChildCount()); i++ {
				body := node.NamedChild(i)
				if body == nil {
					continue
				}
				if body.Type() != "class_body" && body.Type() != "enum_class_body" {
					continue
				}
				for j := 0; j < int(body.NamedChildCount()); j++ {
					walkKotlinDecls(body.NamedChild(j), src, file, sym.Name, pkg, syms, imps, rels)
				}
			}
		}
		return

	case "function_declaration":
		if s, ok := extractKotlinFunction(node, src, file, parent); ok {
			*syms = append(*syms, s)
		}
		return

	case "property_declaration":
		if s, ok := extractKotlinProperty(node, src, file, parent); ok {
			*syms = append(*syms, s)
		}
		return

	case "type_alias":
		if s, ok := extractKotlinTypeAlias(node, src, file, parent); ok {
			*syms = append(*syms, s)
		}
		return
	}

	// Recurse into container nodes (source_file, etc.).
	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkKotlinDecls(node.NamedChild(i), src, file, parent, pkg, syms, imps, rels)
	}
}

// extractKotlinTypeDecl produces a Symbol for a class / interface /
// object declaration. The caller sets the `kindHint` to "class" or
// "object"; this function refines "class" to "data-class" /
// "sealed-class" / "interface" / "enum" based on observed modifiers.
func extractKotlinTypeDecl(node *sitter.Node, src []byte, file, parent, kindHint string) types.Symbol {
	name := firstKotlinIdentifier(node, src)
	if name == "" {
		return types.Symbol{}
	}
	modifiers := kotlinModifiers(node, src)
	kind := kindHint
	switch {
	case kindHint == "class" && containsAny(modifiers, "data"):
		kind = "data-class"
	case kindHint == "class" && containsAny(modifiers, "sealed"):
		kind = "sealed-class"
	case kindHint == "class" && containsAny(modifiers, "enum"):
		kind = "enum"
	case kindHint == "class" && containsAny(modifiers, "annotation"):
		kind = "annotation"
	case kindHint == "class" && hasKotlinInterfaceKeyword(node, src):
		// tree-sitter-kotlin lumps `interface` under class_declaration
		// — we look for the literal `interface` token to split.
		kind = "interface"
	}
	return types.Symbol{
		Name:     name,
		Kind:     kind,
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: kotlinIsExportedByModifiers(modifiers),
		Parent:   parent,
		Doc:      strings.Join(modifiers, " "),
	}
}

// extractKotlinFunction produces a Symbol for a function_declaration
// node. Modifier-aware (suspend / operator / infix / inline / internal /
// private / open / override / abstract / companion / external).
func extractKotlinFunction(node *sitter.Node, src []byte, file, parent string) (types.Symbol, bool) {
	name := firstKotlinIdentifier(node, src)
	if name == "" {
		return types.Symbol{}, false
	}
	modifiers := kotlinModifiers(node, src)
	kind := "function"
	if parent != "" {
		kind = "method"
	}
	if containsAny(modifiers, "operator") {
		kind = "operator"
	}
	if containsAny(modifiers, "suspend") {
		kind = "suspend-function"
	}
	// Extension function: has a receiver type field. tree-sitter-kotlin
	// exposes this via the function_value_parameters preceded by a
	// `user_type` node directly inside the function_declaration.
	if hasKotlinReceiverType(node) {
		kind = "extension-function"
	}
	return types.Symbol{
		Name:     name,
		Kind:     kind,
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: kotlinIsExportedByModifiers(modifiers),
		Parent:   parent,
		Arity:    countKotlinParameters(node),
		Doc:      strings.Join(modifiers, " "),
	}, true
}

// extractKotlinProperty produces a Symbol for a top-level or member
// `val` / `var` / `const val` declaration. `const` → Kind=const.
func extractKotlinProperty(node *sitter.Node, src []byte, file, parent string) (types.Symbol, bool) {
	name := firstKotlinIdentifier(node, src)
	if name == "" {
		return types.Symbol{}, false
	}
	modifiers := kotlinModifiers(node, src)
	kind := "field"
	if parent == "" {
		kind = "var"
	}
	// `const val` → constant
	text := nodeText(node, src)
	if strings.Contains(text, "const ") {
		kind = "const"
	} else if strings.HasPrefix(strings.TrimLeft(text, " \t"), "val ") || strings.Contains(text, " val ") {
		if parent == "" {
			kind = "val"
		}
	}
	return types.Symbol{
		Name:     name,
		Kind:     kind,
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: kotlinIsExportedByModifiers(modifiers),
		Parent:   parent,
		Doc:      strings.Join(modifiers, " "),
	}, true
}

// extractKotlinTypeAlias produces a Symbol for a `typealias X = Y`
// declaration.
func extractKotlinTypeAlias(node *sitter.Node, src []byte, file, parent string) (types.Symbol, bool) {
	name := firstKotlinIdentifier(node, src)
	if name == "" {
		return types.Symbol{}, false
	}
	modifiers := kotlinModifiers(node, src)
	return types.Symbol{
		Name:     name,
		Kind:     "type",
		File:     file,
		Line:     nodeLine(node),
		Exported: kotlinIsExportedByModifiers(modifiers),
		Parent:   parent,
		Doc:      "typealias",
	}, true
}

// kotlinModifiers collects modifier keywords from the node's child
// `modifiers` group. Returns them as a flat string slice in source
// order. Visibility modifiers, inheritance modifiers, and member
// modifiers are all lumped in the same slice — callers that care
// about a specific class filter by name.
func kotlinModifiers(node *sitter.Node, src []byte) []string {
	var out []string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() == "modifiers" {
			for j := 0; j < int(ch.NamedChildCount()); j++ {
				m := ch.NamedChild(j)
				switch m.Type() {
				case "visibility_modifier", "function_modifier",
					"inheritance_modifier", "member_modifier",
					"property_modifier", "class_modifier",
					"parameter_modifier", "platform_modifier",
					"reification_modifier":
					out = append(out, nodeText(m, src))
				case "annotation":
					// Skip annotations — they're not modifier tokens
					// though tree-sitter groups them inside modifiers.
				}
			}
		}
	}
	return out
}

// kotlinIsExportedByModifiers reports whether a symbol with the
// given modifier list is module-visible. Kotlin's default is
// `public` so absence of modifiers yields true; presence of
// `private` / `internal` / `protected` yields false.
func kotlinIsExportedByModifiers(mods []string) bool {
	for _, m := range mods {
		switch m {
		case "private", "internal", "protected":
			return false
		}
	}
	return true
}

// firstKotlinIdentifier finds the first `simple_identifier` (or
// `type_identifier`) direct child of node. Kotlin's class /
// function / property declarations follow the shape
// `modifiers? KEYWORD IDENT rest…` so the first identifier child
// is always the declared name.
func firstKotlinIdentifier(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		switch ch.Type() {
		case "simple_identifier", "type_identifier", "identifier":
			return nodeText(ch, src)
		}
	}
	return ""
}

// collectKotlinQualifiedName reads a dotted name like `a.b.c` from a
// package_header / user_type / qualified_user_type node. Uses the
// raw text between the first identifier and the node's end, with
// surrounding whitespace trimmed, skipping the leading `package`
// keyword if present.
func collectKotlinQualifiedName(node *sitter.Node, src []byte) string {
	text := nodeText(node, src)
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "package ")
	text = strings.TrimPrefix(text, "package\t")
	text = strings.TrimSpace(text)
	// Drop trailing semicolons — rare but legal in Kotlin.
	text = strings.TrimSuffix(text, ";")
	return strings.TrimSpace(text)
}

// collectKotlinImportPath returns the dotted import path from an
// import_header, dropping `import `, `as alias`, and trailing
// whitespace. `import a.b.c as d` → `a.b.c`.
func collectKotlinImportPath(node *sitter.Node, src []byte) string {
	text := nodeText(node, src)
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "import ")
	text = strings.TrimPrefix(text, "import\t")
	if idx := strings.Index(text, " as "); idx >= 0 {
		text = text[:idx]
	}
	// Strip wildcard suffix: `a.b.*` → `a.b`.
	text = strings.TrimSuffix(text, ".*")
	return strings.TrimSpace(text)
}

// collectKotlinImportAlias returns the aliased name from an
// `import a.b.c as d` header, or "" when absent.
func collectKotlinImportAlias(node *sitter.Node, src []byte) string {
	text := nodeText(node, src)
	idx := strings.Index(text, " as ")
	if idx < 0 {
		return ""
	}
	alias := strings.TrimSpace(text[idx+len(" as "):])
	return strings.TrimSuffix(alias, ";")
}

// kotlinSuperTypes walks the delegation_specifier list and returns
// parent type names (for inheritance relations).
func kotlinSuperTypes(node *sitter.Node, src []byte) []string {
	var out []string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() != "delegation_specifier" && ch.Type() != "delegation_specifiers" {
			continue
		}
		for j := 0; j < int(ch.NamedChildCount()); j++ {
			sp := ch.NamedChild(j)
			switch sp.Type() {
			case "user_type":
				name := nodeText(sp, src)
				name = strings.SplitN(name, "<", 2)[0]
				name = strings.TrimSpace(name)
				if name != "" {
					out = append(out, name)
				}
			}
		}
	}
	return out
}

// hasKotlinInterfaceKeyword reports whether a class_declaration
// actually carries the `interface` keyword rather than `class`.
// tree-sitter-kotlin uses the same grammar node type for both.
func hasKotlinInterfaceKeyword(node *sitter.Node, src []byte) bool {
	// The keyword is not a named child; walk all children (including
	// anonymous tokens) looking for literal "interface".
	for i := 0; i < int(node.ChildCount()); i++ {
		ch := node.Child(i)
		if !ch.IsNamed() && strings.TrimSpace(nodeText(ch, src)) == "interface" {
			return true
		}
	}
	return false
}

// hasKotlinReceiverType reports whether a function_declaration
// has a receiver type (i.e. it's an extension function). The
// receiver appears as a `user_type` or `nullable_type` directly
// inside the function_declaration before the name.
func hasKotlinReceiverType(node *sitter.Node) bool {
	sawKeyword := false
	for i := 0; i < int(node.ChildCount()); i++ {
		ch := node.Child(i)
		if !ch.IsNamed() {
			continue
		}
		if ch.Type() == "user_type" || ch.Type() == "nullable_type" {
			if !sawKeyword {
				continue
			}
			return true
		}
		if ch.Type() == "simple_identifier" {
			return false // name came before any receiver — not an extension fn
		}
	}
	return false
}

// countKotlinParameters returns the param count of a
// function_value_parameters child, or 0 when absent.
func countKotlinParameters(node *sitter.Node) int {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() == "function_value_parameters" {
			return int(ch.NamedChildCount())
		}
	}
	return 0
}
