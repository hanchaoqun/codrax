package index

// route_kotlin.go — framework route → handler resolver for Kotlin
// (Spring Web MVC mapping annotations).
//
// Post-pass over an already-parsed Kotlin AST (hooked at the end of
// extractKotlin) that recognises LITERAL Spring mapping declarations
// and emits:
//
//   - one Kind="route" types.Symbol per mapping, named
//     "<VERB> <path>" at the mapping annotation's line; Parent stays
//     empty (file_map renders Parent.Name and would mangle the
//     display);
//   - one Kind="reference" route→handler types.Relation per emitted
//     route. Spring binds handlers via annotations on NAMED functions —
//     there is no anonymous/lambda registration form, so every route
//     this pass emits has an identifier handler and therefore a
//     Relation (the shared contract's "anonymous handler ⇒ Symbol
//     only" branch is structurally unreachable for Spring), mirroring
//     route_java.go for the same framework.
//
// PRECISE GATE (architectural red line — precise signals for hard
// gates): the pass runs only for classes whose modifiers carry a
// @RestController or @Controller annotation NODE. The annotation node
// itself is the precise signal; org.springframework imports merely
// corroborate and receiver/class names are noisy and never consulted.
// kotlinModifiers (extract_kotlin.go) deliberately SKIPS annotation
// nodes — it collects modifier keywords only — so this file carries a
// fresh annotation reader written against the OBSERVED vendored
// tree-sitter-kotlin shapes (probed empirically, not guessed):
//
//	declaration > modifiers > annotation
//	  marker form:  annotation > "@" user_type > type_identifier+
//	  args form:    annotation > "@" constructor_invocation
//	                  > user_type > type_identifier ("." type_identifier)*
//	                  > value_arguments > value_argument*
//	  value_argument: positional  → one expression child
//	                  named (k=v) → simple_identifier "=" expression
//	  string_literal children: string_content* — interpolation appears
//	  as interpolated_identifier ("$x") / interpolated_expression
//	  ("${...}") siblings; an escaped Spring placeholder ("\${...}")
//	  keeps the backslash INSIDE string_content text.
//
// Mapping surface, verb table, ResolvedBy stamp, and the path-join
// rule are shared with route_java.go (springMappingVerbs /
// springRouteResolvedBy / javaJoinRoutePaths): same framework, same
// org.springframework.web.bind.annotation source of truth — a second
// copy could only drift. Per that surface:
//
//   - class-level @RequestMapping("..."|value=...|path=...) supplies
//     the type-level path prefix;
//   - method-level composed shortcuts @GetMapping / @PostMapping /
//     @PutMapping / @DeleteMapping / @PatchMapping pin one HTTP verb;
//   - method-level @RequestMapping is skipped: its verb comes from
//     the non-literal `method` attribute.
//
// Dynamic / non-literal path values — constant references, string
// templates with interpolation ("$x" / "${...}"), escaped "\${...}"
// Environment placeholders, concatenations, multi-element or
// multi-positional path arrays — emit NOTHING for that mapping (paths
// are never fabricated). A class prefix that exists but cannot be
// composed degrades method routes to their LOCAL path flagged
// Metadata["partial_path"]="true".
//
// One Kotlin-flavoured widening of route_java.go's literal rule:
// value=/path= on a Java String[] attribute REQUIRE the collection-
// literal spelling in Kotlin (value = ["/x"] — a bare string is not
// assignable to a vararg/array member by name), so a SINGLE-element
// collection literal of a clean string is accepted in path position.
// Multi-element arrays stay refused, like Java's array initializers.
//
// DETACHED-ANNOTATION RECOVERY (vendored-grammar quirk, probe-
// verified): when a class whose modifier annotations include an
// argumented one (the @RequestMapping("/prefix") shape) is FOLLOWED by
// any further top-level declaration, the vendored tree-sitter-kotlin
// resolves the ambiguity as an annotated EXPRESSION statement instead
// of an annotated declaration. Two observed shapes:
//
//	shape 1: (prefix_expression (annotation)+ (parenthesized_expression))
//	         followed by a SIBLING class_declaration without modifiers —
//	         the last annotation's argument list became the operand;
//	shape 2: (prefix_expression (annotation)+ (infix_expression
//	           (simple_identifier "class") (simple_identifier Name)
//	           (lambda_literal ...)))
//	         — the whole class swallowed as a pseudo-expression.
//
// Both recoveries stay PRECISE: `class` is a hard Kotlin keyword, so
// an infix_expression whose left operand is literally "class" cannot
// be valid expression code, and a bare top-level annotation chain
// immediately preceding a modifier-less class declaration can, in
// valid Kotlin source, only be that class's annotations. Any other
// operand/sibling shape bails and emits nothing.

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// kotlinExtractRoutes is the Spring route resolver entry point, hooked
// at the end of extractKotlin. Walks every class declaration
// (including nested ones; tree-sitter-kotlin parks interfaces under
// the same node type, but those cannot carry mapped instance methods
// Spring would register, and the controller-annotation gate decides
// anyway) and processes the ones that pass the gate.
func kotlinExtractRoutes(root *sitter.Node, src []byte, file string) ([]types.Symbol, []types.Relation) {
	var syms []types.Symbol
	var rels []types.Relation
	walkNamedChildren(root, true, func(node *sitter.Node) {
		if node.Type() != "class_declaration" {
			return
		}
		kotlinSpringClassRoutes(node, src, file, &syms, &rels)
	})
	return syms, rels
}

// kotlinStrayPrecedingAnnotations recovers class annotations that
// tree-sitter-kotlin's error recovery split off into a stray sibling
// node. Probe-verified failure shape: with TWO annotated top-level
// classes in one file, the FIRST class's "@RestController
// @RequestMapping(...)" run is parsed as a separate top-level
// prefix_expression sibling and the class_declaration itself carries
// no modifiers. Multi-controller files are completely ordinary, so
// the route pass harvests annotation nodes from the immediately
// preceding sibling when (and only when) the class's own modifiers
// produced no controller gate. The deeper recovery shape — where the
// whole class body is swallowed into a lambda_literal — also defeats
// plain symbol extraction and is out of this pass's reach (known
// vendored-grammar limitation).
// kotlinStrayAnn pairs a recovered annotation with the argument node
// the error recovery split off it: in the stray shape,
// "@RequestMapping(\"/api\")" parses as a bare annotation followed by
// a SIBLING parenthesized_expression holding the string literal —
// the annotation itself carries no constructor_invocation.
type kotlinStrayAnn struct {
	ann      *sitter.Node
	splitArg *sitter.Node // parenthesized_expression sibling, may be nil
}

func kotlinStrayPrecedingAnnotations(cls *sitter.Node) []kotlinStrayAnn {
	prev := cls.PrevNamedSibling()
	if prev == nil {
		return nil
	}
	switch prev.Type() {
	case "prefix_expression", "annotated_expression":
	default:
		return nil
	}
	var anns []kotlinStrayAnn
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		for i := 0; i < int(n.NamedChildCount()); i++ {
			ch := n.NamedChild(i)
			if ch.Type() == "annotation" {
				entry := kotlinStrayAnn{ann: ch}
				if next := ch.NextNamedSibling(); next != nil && next.Type() == "parenthesized_expression" {
					entry.splitArg = next
				}
				anns = append(anns, entry)
				continue
			}
			walk(ch)
		}
	}
	walk(prev)
	return anns
}

// kotlinStrayAnnotationPath resolves the path literal of a recovered
// annotation: prefer the annotation's own constructor_invocation
// (intact form), else read the split-off parenthesized string
// literal. Interpolated templates stay non-literal (refused).
func kotlinStrayAnnotationPath(entry kotlinStrayAnn, src []byte) (string, bool) {
	if childByType(entry.ann, "constructor_invocation") != nil {
		return kotlinAnnotationPathLiteral(entry.ann, src)
	}
	if entry.splitArg == nil {
		return "", true // marker form
	}
	lit := childByType(entry.splitArg, "string_literal")
	if lit == nil {
		return "", false
	}
	return kotlinRouteStringLiteral(lit, src)
}

// kotlinSpringClassRoutes applies the controller gate to one class
// and, when it opens, emits routes for the class body's mapped
// functions.
func kotlinSpringClassRoutes(cls *sitter.Node, src []byte, file string, syms *[]types.Symbol, rels *[]types.Relation) {
	gate := false
	prefix := ""
	prefixPartial := false
	var anns []kotlinStrayAnn
	for _, ann := range kotlinModifierAnnotations(cls) {
		anns = append(anns, kotlinStrayAnn{ann: ann})
	}
	if !kotlinAnnotationsCarryControllerGate(anns, src) {
		if stray := kotlinStrayPrecedingAnnotations(cls); kotlinAnnotationsCarryControllerGate(stray, src) {
			anns = stray
		}
	}
	for _, entry := range anns {
		switch kotlinAnnotationSimpleName(entry.ann, src) {
		case "RestController", "Controller":
			// org.springframework.web.bind.annotation.RestController /
			// org.springframework.stereotype.Controller — the precise
			// class-level gate.
			gate = true
		case "RequestMapping":
			if p, ok := kotlinStrayAnnotationPath(entry, src); ok {
				prefix = p
			} else {
				// A type-level prefix exists but is not a clean
				// literal: method routes keep their LOCAL paths,
				// flagged partial (never fabricate the prefix).
				prefixPartial = true
			}
		}
	}
	if !gate {
		return
	}
	className := firstKotlinIdentifier(cls, src)
	if className == "" {
		return
	}
	for i := 0; i < int(cls.NamedChildCount()); i++ {
		body := cls.NamedChild(i)
		if body.Type() != "class_body" && body.Type() != "enum_class_body" {
			continue
		}
		for j := 0; j < int(body.NamedChildCount()); j++ {
			member := body.NamedChild(j)
			if member.Type() != "function_declaration" {
				continue
			}
			kotlinSpringFunctionRoutes(member, src, file, className, prefix, prefixPartial, syms, rels)
		}
	}
}

// kotlinSpringFunctionRoutes emits the route Symbol + route→handler
// Relation for each composed mapping annotation on one handler
// function.
func kotlinSpringFunctionRoutes(fn *sitter.Node, src []byte, file, className, prefix string, prefixPartial bool, syms *[]types.Symbol, rels *[]types.Relation) {
	fnName := firstKotlinIdentifier(fn, src)
	if fnName == "" {
		return
	}
	for _, ann := range kotlinModifierAnnotations(fn) {
		verb := springMappingVerbs[kotlinAnnotationSimpleName(ann, src)]
		if verb == "" {
			// Not a composed mapping shortcut. Method-level
			// @RequestMapping lands here too: its verb is the
			// non-literal `method` attribute — skipped by design.
			continue
		}
		local, ok := kotlinAnnotationPathLiteral(ann, src)
		if !ok {
			// Constant reference / template interpolation / \${...}
			// placeholder / multi-path array: emit NOTHING for this
			// mapping (never fabricate).
			continue
		}
		fullPath := javaJoinRoutePaths(prefix, local)
		routeName := verb + " " + fullPath
		line := nodeLine(ann)
		*syms = append(*syms, types.Symbol{
			Name:      routeName,
			Kind:      "route",
			File:      file,
			Line:      line,
			EndLine:   line,
			Exported:  true,
			Signature: routeName + " -> " + className + "." + fnName,
			// Parent MUST stay empty: file_map renders Parent.Name
			// and would mangle the route display.
		})
		md := map[string]string{"framework": "spring", "method": verb, "path": fullPath}
		if prefixPartial {
			md["partial_path"] = "true"
		}
		*rels = append(*rels, types.Relation{
			Kind:       "reference",
			FromEP:     types.RelationEndpoint{Name: routeName, File: file, Line: line},
			ToEP:       types.RelationEndpoint{Name: fnName, Receiver: className, File: file, Line: line},
			File:       file,
			Line:       line,
			Confidence: types.ConfidenceRouteLiteral,
			Provenance: types.ProvenanceRouteResolver,
			ResolvedBy: springRouteResolvedBy,
			Metadata:   md,
		})
	}
}

// kotlinModifierAnnotations returns the annotation named children of a
// declaration's "modifiers" child — tree-sitter-kotlin parks
// annotations inside the modifiers node (probe-verified: modifiers →
// (annotation | <modifier classes>)*). Deliberately NOT folded into
// kotlinModifiers (extract_kotlin.go): that helper collects modifier
// KEYWORDS and skips annotation nodes by design.
func kotlinModifierAnnotations(decl *sitter.Node) []*sitter.Node {
	mods := childByType(decl, "modifiers")
	if mods == nil {
		return nil
	}
	var anns []*sitter.Node
	for i := 0; i < int(mods.NamedChildCount()); i++ {
		if ch := mods.NamedChild(i); ch.Type() == "annotation" {
			anns = append(anns, ch)
		}
	}
	return anns
}

// kotlinAnnotationSimpleName returns the unqualified annotation type
// name: "RestController" for both @RestController and the fully-
// qualified @org.springframework.web.bind.annotation.RestController.
// Probe-verified shapes: a marker annotation holds the user_type
// directly; an argumented one nests it under constructor_invocation;
// a qualified user_type is a flat "." chain of type_identifier nodes —
// the simple name is the LAST one.
func kotlinAnnotationSimpleName(ann *sitter.Node, src []byte) string {
	ut := childByType(ann, "user_type")
	if ut == nil {
		if ci := childByType(ann, "constructor_invocation"); ci != nil {
			ut = childByType(ci, "user_type")
		}
	}
	if ut == nil {
		return ""
	}
	var last *sitter.Node
	for i := 0; i < int(ut.NamedChildCount()); i++ {
		if ch := ut.NamedChild(i); ch.Type() == "type_identifier" {
			last = ch
		}
	}
	if last == nil {
		return ""
	}
	return nodeText(last, src)
}

// kotlinAnnotationPathLiteral extracts the literal route path bound by
// a mapping annotation. Returns ok=false when a path-position argument
// exists but is not a clean literal — constant references, template
// interpolation, concatenations, multi-element arrays, multiple
// positional paths — so callers can refuse to fabricate. The marker
// form (@PostMapping), empty parentheses, and attribute-only arguments
// without value=/path= yield ("", true): @RequestMapping declares
// value/path with default {} (no path), so absence is the precise
// empty path, not an unknown.
func kotlinAnnotationPathLiteral(ann *sitter.Node, src []byte) (string, bool) {
	ci := childByType(ann, "constructor_invocation")
	if ci == nil {
		return "", true // marker form: no path attribute
	}
	args := childByType(ci, "value_arguments")
	if args == nil {
		return "", true
	}
	var positionals []*sitter.Node
	for i := 0; i < int(args.NamedChildCount()); i++ {
		arg := args.NamedChild(i)
		if arg.Type() != "value_argument" {
			continue // interleaved comments
		}
		key, val := kotlinValueArgumentKV(arg, src)
		if val == nil {
			// Unrecognised argument shape: refuse the whole
			// annotation rather than risk missing a bound path.
			return "", false
		}
		if key == "" {
			positionals = append(positionals, val)
			continue
		}
		// value and path are aliased attributes on every @*Mapping
		// annotation (@AliasFor pairs in
		// org.springframework.web.bind.annotation).
		if key == "value" || key == "path" {
			return kotlinRoutePathValue(val, src)
		}
		// produces= / consumes= / headers= / ... — not path-position.
	}
	switch len(positionals) {
	case 0:
		return "", true // attribute-only arguments: no path bound
	case 1:
		return kotlinRoutePathValue(positionals[0], src)
	default:
		// vararg value with several positional paths: one mapping
		// binding multiple routes — refusing beats picking one.
		return "", false
	}
}

// kotlinValueArgumentKV splits one value_argument into its optional
// attribute key and its expression node. Probe-verified shapes: a
// named argument is simple_identifier "=" expression (the "=" is an
// anonymous token); a positional argument is a single expression
// child. Returns val=nil for any other shape (e.g. spread "*expr").
func kotlinValueArgumentKV(arg *sitter.Node, src []byte) (string, *sitter.Node) {
	hasEq := false
	for i := 0; i < int(arg.ChildCount()); i++ {
		if ch := arg.Child(i); !ch.IsNamed() && nodeText(ch, src) == "=" {
			hasEq = true
			break
		}
	}
	var named []*sitter.Node
	for i := 0; i < int(arg.NamedChildCount()); i++ {
		ch := arg.NamedChild(i)
		switch ch.Type() {
		case "comment", "line_comment", "multiline_comment":
			continue
		}
		named = append(named, ch)
	}
	if hasEq && len(named) == 2 && named[0].Type() == "simple_identifier" {
		return nodeText(named[0], src), named[1]
	}
	if !hasEq && len(named) == 1 {
		return "", named[0]
	}
	return "", nil
}

// kotlinRoutePathValue resolves a path-position expression to its
// literal string. Accepts a clean string_literal, or — the mandatory
// Kotlin spelling for the Java String[] attributes value=/path= — a
// SINGLE-element collection literal of one ([" /x"]-style). Everything
// else (multi-element arrays, arrayOf(...) calls, identifiers,
// concatenations) reports !ok.
func kotlinRoutePathValue(n *sitter.Node, src []byte) (string, bool) {
	if n == nil {
		return "", false
	}
	switch n.Type() {
	case "string_literal":
		return kotlinRouteStringLiteral(n, src)
	case "collection_literal":
		var elems []*sitter.Node
		for i := 0; i < int(n.NamedChildCount()); i++ {
			ch := n.NamedChild(i)
			switch ch.Type() {
			case "comment", "line_comment", "multiline_comment":
				continue
			}
			elems = append(elems, ch)
		}
		if len(elems) == 1 && elems[0].Type() == "string_literal" {
			return kotlinRouteStringLiteral(elems[0], src)
		}
		return "", false
	}
	return "", false
}

// kotlinRouteStringLiteral resolves a string_literal node to its
// static text. Probe-verified: a clean literal holds only
// string_content children (zero for ""); template interpolation
// surfaces as interpolated_identifier / interpolated_expression
// siblings and reports !ok. Content carrying a backslash also reports
// !ok: escape sequences make the source text differ from the runtime
// value, and the escaped "\${...}" Spring Environment placeholder (its
// backslash stays inside string_content) resolves only at runtime —
// the static path would be a fabrication, mirroring
// javaRouteStringLiteral's ${...} rule.
func kotlinRouteStringLiteral(n *sitter.Node, src []byte) (string, bool) {
	if n == nil || n.Type() != "string_literal" {
		return "", false
	}
	var sb strings.Builder
	for i := 0; i < int(n.NamedChildCount()); i++ {
		ch := n.NamedChild(i)
		if ch.Type() != "string_content" {
			return "", false // template interpolation or other shape
		}
		sb.WriteString(nodeText(ch, src))
	}
	v := sb.String()
	if strings.Contains(v, "${") || strings.ContainsRune(v, '\\') {
		return "", false
	}
	return v, true
}

// kotlinAnnotationsCarryControllerGate reports whether the annotation
// set contains the Spring controller class gate.
func kotlinAnnotationsCarryControllerGate(anns []kotlinStrayAnn, src []byte) bool {
	for _, entry := range anns {
		switch kotlinAnnotationSimpleName(entry.ann, src) {
		case "RestController", "Controller":
			return true
		}
	}
	return false
}
