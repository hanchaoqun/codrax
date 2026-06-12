package index

// route_java.go — framework route → handler resolver for Java
// (Spring Web MVC mapping annotations).
//
// Post-pass over an already-parsed Java AST (hooked at the end of
// extractJava, alongside the method-invocation post-pass) that
// recognises LITERAL Spring mapping declarations and emits:
//
//   - one Kind="route" types.Symbol per mapping, named
//     "<VERB> <path>" at the mapping annotation's line; Parent stays
//     empty (file_map renders Parent.Name and would mangle the
//     display);
//   - one Kind="reference" route→handler types.Relation per emitted
//     route. Spring binds handlers via annotations on NAMED methods —
//     there is no anonymous/lambda registration form, so every route
//     this pass emits has an identifier handler and therefore a
//     Relation (the shared contract's "anonymous handler ⇒ Symbol
//     only" branch is structurally unreachable for Spring).
//
// PRECISE GATE (architectural red line — precise signals for hard
// gates): the pass runs only for classes whose modifiers carry a
// @RestController or @Controller annotation NODE. The annotation node
// itself is the precise signal; org.springframework imports merely
// corroborate and receiver/class names are noisy and never consulted.
// javaHasModifier's substring check over the modifiers text is
// deliberately NOT reused — a substring match would fire on comments
// or unrelated annotations (e.g. @MyControllerConfig).
//
// Mapping surface (source of truth: Spring Framework
// org.springframework.web.bind.annotation — NOT test fixtures):
//
//   - class-level @RequestMapping("..."|value=...|path=...) supplies
//     the type-level path prefix (RequestMappingHandlerMapping
//     combines type-level and method-level patterns);
//   - method-level composed shortcuts introduced in Spring 4.3:
//     @GetMapping / @PostMapping / @PutMapping / @DeleteMapping /
//     @PatchMapping, each pinning exactly one HTTP verb;
//   - method-level @RequestMapping is skipped: its verb comes from
//     the `method` attribute (a RequestMethod[] expression — defaults
//     to ALL verbs when absent), which is not a literal verb.
//
// Dynamic / non-literal path values — constant references, `${...}`
// Environment property placeholders, array initializers — emit
// NOTHING for that mapping (paths are never fabricated). A class
// prefix that exists but cannot be composed degrades method routes to
// their LOCAL path flagged Metadata["partial_path"]="true".

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// springMappingVerbs maps Spring's composed mapping annotations to
// HTTP verbs. Source of truth: the @RequestMapping composed shortcuts
// in org.springframework.web.bind.annotation (Spring Framework 4.3+).
// @RequestMapping itself is absent by design — see the file header.
var springMappingVerbs = map[string]string{
	"GetMapping":    "GET",
	"PostMapping":   "POST",
	"PutMapping":    "PUT",
	"DeleteMapping": "DELETE",
	"PatchMapping":  "PATCH",
}

// springRouteResolvedBy is the construction-stage diagnostic stamped
// on every relation this pass emits.
const springRouteResolvedBy = "spring_mapping_annotation"

// javaExtractRoutes is the Spring route resolver entry point, hooked
// at the end of extractJava. Walks every class declaration (including
// nested ones) and processes the ones that pass the controller-
// annotation gate.
func javaExtractRoutes(root *sitter.Node, src []byte, file string) ([]types.Symbol, []types.Relation) {
	var syms []types.Symbol
	var rels []types.Relation
	walkNamedChildren(root, true, func(node *sitter.Node) {
		if node.Type() != "class_declaration" {
			return
		}
		javaSpringClassRoutes(node, src, file, &syms, &rels)
	})
	return syms, rels
}

// javaSpringClassRoutes applies the controller gate to one class and,
// when it opens, emits routes for the class body's mapped methods.
func javaSpringClassRoutes(cls *sitter.Node, src []byte, file string, syms *[]types.Symbol, rels *[]types.Relation) {
	gate := false
	prefix := ""
	prefixPartial := false
	for _, ann := range javaModifierAnnotations(cls) {
		switch javaAnnotationSimpleName(ann, src) {
		case "RestController", "Controller":
			// org.springframework.web.bind.annotation.RestController /
			// org.springframework.stereotype.Controller — the precise
			// class-level gate.
			gate = true
		case "RequestMapping":
			if p, ok := javaAnnotationPathLiteral(ann, src); ok {
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
	nameNode := cls.ChildByFieldName("name")
	body := cls.ChildByFieldName("body")
	if nameNode == nil || body == nil {
		return
	}
	className := nodeText(nameNode, src)
	for i := 0; i < int(body.NamedChildCount()); i++ {
		member := body.NamedChild(i)
		if member.Type() != "method_declaration" {
			continue
		}
		javaSpringMethodRoutes(member, src, file, className, prefix, prefixPartial, syms, rels)
	}
}

// javaSpringMethodRoutes emits the route Symbol + route→handler
// Relation for each composed mapping annotation on one handler method.
func javaSpringMethodRoutes(method *sitter.Node, src []byte, file, className, prefix string, prefixPartial bool, syms *[]types.Symbol, rels *[]types.Relation) {
	nameNode := method.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	methodName := nodeText(nameNode, src)
	for _, ann := range javaModifierAnnotations(method) {
		verb := springMappingVerbs[javaAnnotationSimpleName(ann, src)]
		if verb == "" {
			// Not a composed mapping shortcut. Method-level
			// @RequestMapping lands here too: its verb is the
			// non-literal `method` attribute — skipped by design.
			continue
		}
		local, ok := javaAnnotationPathLiteral(ann, src)
		if !ok {
			// Constant reference / ${placeholder} / array path:
			// emit NOTHING for this mapping (never fabricate).
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
			Signature: routeName + " -> " + className + "." + methodName,
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
			ToEP:       types.RelationEndpoint{Name: methodName, Receiver: className, File: file, Line: line},
			File:       file,
			Line:       line,
			Confidence: types.ConfidenceRouteLiteral,
			Provenance: types.ProvenanceRouteResolver,
			ResolvedBy: springRouteResolvedBy,
			Metadata:   md,
		})
	}
}

// javaModifierAnnotations returns the annotation / marker_annotation
// named children of a declaration's "modifiers" child — tree-sitter-
// java parks annotations inside the modifiers node (verified against
// the vendored grammar: modifiers → (marker_annotation | annotation |
// keyword)*). Distinct from javaHasModifier on purpose: that helper
// substring-checks the modifiers TEXT, which is too noisy to gate on.
func javaModifierAnnotations(decl *sitter.Node) []*sitter.Node {
	mods := childByType(decl, "modifiers")
	if mods == nil {
		return nil
	}
	var anns []*sitter.Node
	for i := 0; i < int(mods.NamedChildCount()); i++ {
		ch := mods.NamedChild(i)
		if t := ch.Type(); t == "annotation" || t == "marker_annotation" {
			anns = append(anns, ch)
		}
	}
	return anns
}

// javaAnnotationSimpleName returns the unqualified annotation type
// name: "RestController" for both @RestController and the fully-
// qualified @org.springframework.web.bind.annotation.RestController
// (the name field holds a scoped_identifier in the qualified form).
func javaAnnotationSimpleName(ann *sitter.Node, src []byte) string {
	nameNode := ann.ChildByFieldName("name")
	if nameNode == nil {
		return ""
	}
	name := nodeText(nameNode, src)
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// javaAnnotationPathLiteral extracts the literal route path bound by
// a mapping annotation. Returns ok=false when a path-position
// argument exists but is not a clean string literal — constant
// references (field_access / identifier element values), array
// initializers, concatenation expressions — so callers can refuse to
// fabricate. A marker annotation, an empty argument list, or
// attribute-only arguments without value=/path= yield ("", true):
// @RequestMapping declares value/path with default {} (no path), so
// absence is the precise empty path, not an unknown.
func javaAnnotationPathLiteral(ann *sitter.Node, src []byte) (string, bool) {
	args := ann.ChildByFieldName("arguments")
	if args == nil {
		return "", true // marker_annotation: no path attribute
	}
	for i := 0; i < int(args.NamedChildCount()); i++ {
		ch := args.NamedChild(i)
		switch ch.Type() {
		case "line_comment", "block_comment":
			continue
		case "element_value_pair":
			key := ch.ChildByFieldName("key")
			if key == nil {
				continue
			}
			// value and path are aliased attributes on every
			// @*Mapping annotation (@AliasFor pairs in
			// org.springframework.web.bind.annotation).
			if k := nodeText(key, src); k != "value" && k != "path" {
				continue // produces= / consumes= / headers= / ...
			}
			return javaRouteStringLiteral(ch.ChildByFieldName("value"), src)
		default:
			// Single positional element value == the `value`
			// attribute (@GetMapping("/x")).
			return javaRouteStringLiteral(ch, src)
		}
	}
	return "", true
}

// javaRouteStringLiteral unquotes a string_literal node. Any other
// expression shape reports !ok. Literals carrying a `${...}` Spring
// property placeholder resolve at runtime against the Environment and
// also report !ok — the static path would be a fabrication.
func javaRouteStringLiteral(n *sitter.Node, src []byte) (string, bool) {
	if n == nil || n.Type() != "string_literal" {
		return "", false
	}
	raw := nodeText(n, src)
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", false
	}
	v := raw[1 : len(raw)-1]
	if strings.Contains(v, "${") {
		return "", false
	}
	return v, true
}

// javaJoinRoutePaths concatenates the class-level prefix and the
// method-level literal with exactly one slash at the join, mirroring
// Spring's pattern combination (PathPattern.combine /
// AntPathMatcher.combine) for the literal forms this pass accepts:
// the all-empty combination maps to "/" and a nonempty result is
// normalised to a leading slash, as Spring's parsed patterns are.
func javaJoinRoutePaths(prefix, local string) string {
	joined := strings.TrimSuffix(prefix, "/")
	if local != "" && !strings.HasPrefix(local, "/") {
		local = "/" + local
	}
	joined += local
	if joined == "" {
		return "/"
	}
	if !strings.HasPrefix(joined, "/") {
		joined = "/" + joined
	}
	return joined
}
