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

	// framework route → handler resolver (Spring @*Mapping annotations);
	// gated on the @RestController / @Controller class annotation —
	// no-op for files without an annotated controller class.
	routeSyms, routeRels := kotlinExtractRoutes(root, src, file)
	syms = append(syms, routeSyms...)
	rels = append(rels, routeRels...)
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
					Kind:       "inheritance",
					FromEP:     types.RelationEndpoint{Name: sym.Name, File: file, Line: sym.Line},
					ToEP:       types.RelationEndpoint{Name: superName, File: file, Line: sym.Line},
					File:       file,
					Line:       sym.Line,
					Confidence: types.ConfidenceAST,
					Provenance: types.ProvenanceTreeSitter,
					ResolvedBy: "kotlin_supertype",
				})
			}
		}
		return

	case "object_declaration", "companion_object":
		kindHint := "object"
		if node.Type() == "companion_object" {
			kindHint = "companion-object"
		}
		sym := extractKotlinTypeDecl(node, src, file, parent, kindHint)
		// companion_object may omit the explicit name (`companion object
		// { ... }` without a tag); assign a synthetic name tied to the
		// enclosing class so the graph can still reference it.
		if sym.Name == "" && kindHint == "companion-object" && parent != "" {
			sym.Name = "Companion"
			sym.Line = nodeLine(node)
			sym.EndLine = nodeEndLine(node)
			sym.Parent = parent
			sym.Exported = true
			sym.Kind = kindHint
		}
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

// firstKotlinIdentifier finds the declared name of a Kotlin
// declaration node.
//
// Grammar-specific shape caveats (from the vendored tree-sitter-
// kotlin grammar):
//
//   - function_declaration:    `fun RECEIVER? name(...) ...` — the
//     receiver (user_type / nullable_type / bare type_identifier)
//     appears before the name, so we skip it.
//   - property_declaration:    `val TAG = ...` — the name sits
//     inside a `variable_declaration` child, not as a direct
//     identifier.
//   - class/object/interface:  `class Name` — direct
//     type_identifier or simple_identifier child is the name.
//
// Returning "" means the node has no recognisable name field; the
// caller drops the symbol.
func firstKotlinIdentifier(node *sitter.Node, src []byte) string {
	isFn := node.Type() == "function_declaration"
	isProp := node.Type() == "property_declaration"
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		switch ch.Type() {
		case "simple_identifier", "identifier":
			return nodeText(ch, src)
		case "type_identifier":
			if !isFn {
				return nodeText(ch, src)
			}
			// function receiver via bare type_identifier — skip.
		case "user_type", "nullable_type":
			if !isFn {
				return nodeText(ch, src)
			}
			// function receiver — skip.
		case "variable_declaration":
			if !isProp {
				continue
			}
			// Descend: the first simple_identifier child carries
			// the property name. Multi-var declarations like
			// `val (a, b) = pair` produce multi_variable_declaration
			// instead, which we intentionally skip (capturing only
			// the first name would be misleading).
			for j := 0; j < int(ch.NamedChildCount()); j++ {
				id := ch.NamedChild(j)
				if id.Type() == "simple_identifier" || id.Type() == "identifier" {
					return nodeText(id, src)
				}
			}
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
//
// The `.*` wildcard suffix is intentionally preserved (`a.b.*` stays
// `a.b.*`), mirroring the Java extractor: kotlinImportResolver needs
// it to tell a whole-package wildcard apart from a member import of
// the same dotted text. Stripping it here loses that distinction and
// makes `import a.b.*` resolve to nothing.
func collectKotlinImportPath(node *sitter.Node, src []byte) string {
	text := nodeText(node, src)
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "import ")
	text = strings.TrimPrefix(text, "import\t")
	if idx := strings.Index(text, " as "); idx >= 0 {
		text = text[:idx]
	}
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
//
// Actual tree-sitter-kotlin shape (verified against the vendored
// grammar):
//
//	class_declaration
//	  delegation_specifier               ← one per supertype
//	    constructor_invocation           ← `Foo()` — class super
//	      user_type → type_identifier "Foo"
//	    or user_type → type_identifier   ← bare interface ref
//	    or explicit_delegation           ← `by xxx`
//
// We accept both `user_type` direct child and the nested
// `constructor_invocation > user_type` form so Android Activity
// subclasses (`: ComponentActivity()`) surface as inheritance
// edges alongside bare interface implementations.
func kotlinSuperTypes(node *sitter.Node, src []byte) []string {
	var out []string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() != "delegation_specifier" && ch.Type() != "delegation_specifiers" {
			continue
		}
		// Each delegation_specifier may wrap either a user_type
		// directly (interface inheritance) or a constructor_invocation
		// (class super). Descend once to handle both.
		for j := 0; j < int(ch.NamedChildCount()); j++ {
			sp := ch.NamedChild(j)
			var ut *sitter.Node
			switch sp.Type() {
			case "user_type":
				ut = sp
			case "constructor_invocation":
				for k := 0; k < int(sp.NamedChildCount()); k++ {
					inner := sp.NamedChild(k)
					if inner.Type() == "user_type" {
						ut = inner
						break
					}
				}
			case "explicit_delegation":
				// `by foo` delegation — capture the delegate type.
				for k := 0; k < int(sp.NamedChildCount()); k++ {
					inner := sp.NamedChild(k)
					if inner.Type() == "user_type" {
						ut = inner
						break
					}
				}
			}
			if ut == nil {
				continue
			}
			name := nodeText(ut, src)
			name = strings.SplitN(name, "<", 2)[0]
			name = strings.TrimSpace(name)
			if name != "" {
				out = append(out, name)
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

// hasKotlinReceiverType reports whether a function_declaration is
// an extension function. Grammar shape (observed from the vendored
// tree-sitter-kotlin):
//
//	function_declaration
//	  [modifiers]
//	  user_type          ← ONLY present for extension functions
//	  simple_identifier  ← function name
//	  function_value_parameters
//	  [user_type]        ← return type (optional)
//	  [function_body]
//
// So the rule is: if the first named child (after any `modifiers`)
// is `user_type` OR `nullable_type`, it is a receiver — the function
// is an extension function.
func hasKotlinReceiverType(node *sitter.Node) bool {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		switch ch.Type() {
		case "modifiers":
			continue
		case "user_type", "nullable_type":
			return true
		case "simple_identifier", "type_identifier":
			// Function name reached without a receiver → not an
			// extension function.
			return false
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
