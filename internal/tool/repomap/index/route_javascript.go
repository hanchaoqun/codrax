package index

// route_javascript.go — framework route → handler resolver for
// JavaScript/TypeScript (Express + NestJS).
//
// Post-pass over an already-parsed JS/TS AST (hooked at the end of
// extractJS, alongside the call post-pass) that recognises LITERAL
// route registrations and emits:
//
//   - one Kind="route" types.Symbol per registration, named
//     "<VERB> <path>" at the registration line; Parent stays empty
//     (file_map renders Parent.Name and would mangle the display);
//   - one Kind="reference" route→handler types.Relation when the
//     handler is an identifier or member expression (Express) or a
//     decorated named method (NestJS). Arrow functions / function
//     expressions (and any other expression shape) get the Symbol but
//     never a Relation.
//
// PRECISE GATE (architectural red line — precise signals for hard
// gates): each framework's pass runs only when the file provably pulls
// the framework in — a types.Import whose Path is VERBATIM "express" /
// "@nestjs/common", or (Express only) a require() call whose sole
// argument is the VERBATIM string literal "express". The require()
// lane exists because extractJS's import extractor records ES `import`
// statements only — CommonJS require() bindings never reach the
// types.Import stream (verified against jsExtractImport), so gating on
// Import.Path alone would silently drop every CommonJS Express app.
// Receiver names are noisy and never gate the pass; an
// express-importing file calling client.get("/x", cb) on a non-router
// receiver is an accepted residual, priced by
// types.ConfidenceRouteLiteral.
//
// ArkTS inertness (HarmonyOS red lines): extractJS is shared by JS, TS
// and the ArkTS Tier-1 path (extract_arkts.go calls it with isTS=true).
// HarmonyOS sources import from '@ohos.*' / '@kit.*' / relative paths
// and never from 'express' or '@nestjs/common', so the verbatim import
// gates structurally cannot fire there; ArkUI decorators (@Component,
// @Entry, @Get-less state decorators) never carry the NestJS verb
// names AND sit behind the unfired @nestjs/common gate. Pinned by the
// ArkTS no-op regression test in route_javascript_test.go.
//
// Dynamic / non-literal path arguments (identifiers, concatenation,
// template strings with ${...} substitution holes, arrays, regexes)
// emit NOTHING for that registration — paths are never fabricated.

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// Verbatim gate module ids. Subpaths ("express/lib/router",
// "@nestjs/common/decorators") do NOT open the gates — route
// registration surface comes from the root module in both frameworks.
const (
	jsExpressModuleID    = "express"
	jsNestCommonModuleID = "@nestjs/common"
)

// Construction-stage ResolvedBy diagnostics for the two lanes.
const (
	jsExpressResolvedBy = "express_route_literal"
	jsNestResolvedBy    = "nestjs_decorator"
)

// jsExpressVerbs maps Express registration method names to HTTP verbs.
// Source of truth: the Express 4.x/5.x Application and Router API
// (https://expressjs.com/en/4x/api.html) — app.METHOD(path, callback
// [, callback ...]) is documented for the standard HTTP methods, with
// app.get / app.post / app.put / app.delete / app.patch / app.head /
// app.options spelled out, plus app.all(path, ...) which "matches all
// HTTP verbs" (rendered ANY, mirroring gin's Any); express.Router()
// exposes the identical router.METHOD / router.all surface. The
// generic app.METHOD spelling for exotic verbs and router.use (mounts
// middleware, binds no verb) are deliberately out of scope.
var jsExpressVerbs = map[string]string{
	"get":     "GET",
	"post":    "POST",
	"put":     "PUT",
	"delete":  "DELETE",
	"patch":   "PATCH",
	"head":    "HEAD",
	"options": "OPTIONS",
	"all":     "ANY",
}

// jsNestVerbs maps NestJS request-mapping decorator names to HTTP
// verbs. Source of truth: the @nestjs/common HTTP route decorators
// (packages/common/decorators/http/request-mapping.decorator.ts) —
// Get / Post / Put / Delete / Patch / Options / Head / Search, plus
// All which maps RequestMethod.ALL (rendered ANY, mirroring gin's
// Any). NOT from test fixtures (generalization red line).
var jsNestVerbs = map[string]string{
	"Get":     "GET",
	"Post":    "POST",
	"Put":     "PUT",
	"Delete":  "DELETE",
	"Patch":   "PATCH",
	"Options": "OPTIONS",
	"Head":    "HEAD",
	"Search":  "SEARCH",
	"All":     "ANY",
}

// jsExtractRoutes is the Express + NestJS route resolver entry point,
// hooked at the end of extractJS. The imps slice is the one extractJS
// already collected for this file.
func jsExtractRoutes(root *sitter.Node, src []byte, file string, imps []types.Import) ([]types.Symbol, []types.Relation) {
	if root == nil {
		return nil, nil
	}
	var syms []types.Symbol
	var rels []types.Relation
	if jsHasImportPath(imps, jsExpressModuleID) || jsHasRequireOf(root, src, jsExpressModuleID) {
		jsExpressRoutes(root, src, file, &syms, &rels)
	}
	if jsHasImportPath(imps, jsNestCommonModuleID) {
		jsNestRoutes(root, src, file, &syms, &rels)
	}
	return syms, rels
}

// jsHasImportPath is the precise ES-import gate: verbatim Import.Path
// match only.
func jsHasImportPath(imps []types.Import, moduleID string) bool {
	for _, imp := range imps {
		if imp.Path == moduleID {
			return true
		}
	}
	return false
}

// jsHasRequireOf is the precise CommonJS gate: a require() call whose
// single argument is the verbatim string literal moduleID, anywhere in
// the file. The callee must be the bare `require` identifier — member
// shapes (foo.require) and computed ids stay closed.
func jsHasRequireOf(root *sitter.Node, src []byte, moduleID string) bool {
	found := false
	walkNamedChildren(root, true, func(n *sitter.Node) {
		if found || !jsIsRequireOf(n, src, moduleID) {
			return
		}
		found = true
	})
	return found
}

// jsIsRequireOf reports whether n is a call_expression of the exact
// shape require("<moduleID>").
func jsIsRequireOf(n *sitter.Node, src []byte, moduleID string) bool {
	if n == nil || n.Type() != "call_expression" {
		return false
	}
	fn := n.ChildByFieldName("function")
	if fn == nil || fn.Type() != "identifier" || nodeText(fn, src) != "require" {
		return false
	}
	args := jsRouteCallArgs(n)
	if len(args) != 1 {
		return false
	}
	lit, ok := jsRouteStringLiteral(args[0], src)
	return ok && lit == moduleID
}

// ---------------------------------------------------------------------------
// Express lane
// ---------------------------------------------------------------------------

// jsExpressBindings carries the module-level value tracking that
// decides partial_path. Express has no in-file prefix composition on
// the registration itself: app.use("/prefix", r) mounting is a
// SEPARATE statement (frequently a separate file), one router can be
// mounted under several prefixes, and connecting registration to mount
// would need whole-program, order-sensitive analysis — so cross-
// statement composition is out of scope by design. Routes on
// express()-bound applications are complete; routes on Router()-bound
// values (and on unresolvable receivers) keep their LOCAL path flagged
// Metadata["partial_path"]="true".
type jsExpressBindings struct {
	module     map[string]bool // names bound to the express module (default/namespace import, require)
	routerCtor map[string]bool // local names of the named Router export
	app        map[string]bool // names provably bound to express() applications
	router     map[string]bool // names provably bound to Router-constructor results
}

// jsExpressRoutes runs the Express half: collect module-level bindings
// in document order, then emit a route per literal registration found
// anywhere in the tree.
func jsExpressRoutes(root *sitter.Node, src []byte, file string, syms *[]types.Symbol, rels *[]types.Relation) {
	b := jsCollectExpressBindings(root, src)
	walkNamedChildren(root, true, func(n *sitter.Node) {
		if n.Type() != "call_expression" {
			return
		}
		jsExpressEmit(n, src, file, b, syms, rels)
	})
}

// jsCollectExpressBindings tracks same-file MODULE-LEVEL bindings (a
// statement-order forward pass, mirroring the Python pass's module-
// level FastAPI/APIRouter tracking; function-local app factories fall
// into the unknown-receiver lane: emit + partial_path):
//
//	import express from "express"        → module name
//	import * as express from "express"   → module name
//	import { Router [as R] } from "express" → Router-constructor name
//	const express = require("express")   → module name
//	const { Router[: R] } = require("express") → Router-constructor name
//	const app = express()                → application (complete paths)
//	const app = require("express")()     → application (complete paths)
//	const r = express.Router()           → router (partial paths)
//	const r = Router()                   → router (partial paths)
//
// `export const app = express()` unwraps the export_statement. Later
// assignments to a tracked name with a non-matching value invalidate
// the old binding, mirroring the Go pass.
func jsCollectExpressBindings(root *sitter.Node, src []byte) *jsExpressBindings {
	b := &jsExpressBindings{
		module:     map[string]bool{},
		routerCtor: map[string]bool{},
		app:        map[string]bool{},
		router:     map[string]bool{},
	}
	for i := 0; i < int(root.NamedChildCount()); i++ {
		stmt := root.NamedChild(i)
		if stmt.Type() == "export_statement" {
			for j := 0; j < int(stmt.NamedChildCount()); j++ {
				jsExpressBindingStmt(stmt.NamedChild(j), src, b)
			}
			continue
		}
		jsExpressBindingStmt(stmt, src, b)
	}
	return b
}

// jsExpressBindingStmt feeds one top-level statement into the binding
// maps.
func jsExpressBindingStmt(stmt *sitter.Node, src []byte, b *jsExpressBindings) {
	switch stmt.Type() {
	case "import_statement":
		jsExpressImportClause(stmt, src, b)
	case "lexical_declaration", "variable_declaration":
		for i := 0; i < int(stmt.NamedChildCount()); i++ {
			decl := stmt.NamedChild(i)
			if decl.Type() == "variable_declarator" {
				jsExpressDeclarator(decl, src, b)
			}
		}
	}
}

// jsExpressImportClause records what an `import ... from "express"`
// statement binds locally. Non-express sources are ignored here — the
// pass-level gate has already fired, but per-binding precision still
// requires the verbatim source match.
func jsExpressImportClause(imp *sitter.Node, src []byte, b *jsExpressBindings) {
	source := imp.ChildByFieldName("source")
	if source == nil {
		return
	}
	if lit, ok := jsRouteStringLiteral(source, src); !ok || lit != jsExpressModuleID {
		return
	}
	clause := childByType(imp, "import_clause")
	if clause == nil {
		return // bare `import "express"` binds nothing
	}
	for i := 0; i < int(clause.NamedChildCount()); i++ {
		ch := clause.NamedChild(i)
		switch ch.Type() {
		case "identifier": // default import: the callable express module
			b.module[nodeText(ch, src)] = true
		case "namespace_import": // import * as express — same registration surface
			if id := childByType(ch, "identifier"); id != nil {
				b.module[nodeText(id, src)] = true
			}
		case "named_imports":
			for j := 0; j < int(ch.NamedChildCount()); j++ {
				spec := ch.NamedChild(j)
				if spec.Type() != "import_specifier" {
					continue
				}
				name := spec.ChildByFieldName("name")
				if name == nil || nodeText(name, src) != "Router" {
					continue
				}
				local := name
				if alias := spec.ChildByFieldName("alias"); alias != nil {
					local = alias
				}
				b.routerCtor[nodeText(local, src)] = true
			}
		}
	}
}

// jsExpressDeclarator classifies one module-level variable_declarator.
func jsExpressDeclarator(decl *sitter.Node, src []byte, b *jsExpressBindings) {
	nameNode := decl.ChildByFieldName("name")
	value := decl.ChildByFieldName("value")
	if nameNode == nil || value == nil {
		return
	}

	// Destructured require: const { Router } / { Router: R } = require("express").
	if nameNode.Type() == "object_pattern" && jsIsRequireOf(value, src, jsExpressModuleID) {
		for i := 0; i < int(nameNode.NamedChildCount()); i++ {
			prop := nameNode.NamedChild(i)
			switch prop.Type() {
			case "shorthand_property_identifier_pattern":
				if nodeText(prop, src) == "Router" {
					b.routerCtor["Router"] = true
				}
			case "pair_pattern":
				key := prop.ChildByFieldName("key")
				val := prop.ChildByFieldName("value")
				if key != nil && val != nil && nodeText(key, src) == "Router" && val.Type() == "identifier" {
					b.routerCtor[nodeText(val, src)] = true
				}
			}
		}
		return
	}

	if nameNode.Type() != "identifier" {
		return
	}
	name := nodeText(nameNode, src)
	// Rebinding a tracked name to a non-matching value invalidates the
	// old binding (mirror of the Go pass's invalidation rule).
	delete(b.module, name)
	delete(b.app, name)
	delete(b.router, name)

	if value.Type() != "call_expression" {
		return
	}
	if jsIsRequireOf(value, src, jsExpressModuleID) {
		b.module[name] = true // const express = require("express")
		return
	}
	callee := value.ChildByFieldName("function")
	if callee == nil {
		return
	}
	switch callee.Type() {
	case "identifier":
		id := nodeText(callee, src)
		switch {
		case b.module[id]:
			b.app[name] = true // const app = express()
		case b.routerCtor[id]:
			b.router[name] = true // const r = Router()
		}
	case "member_expression":
		obj := callee.ChildByFieldName("object")
		prop := callee.ChildByFieldName("property")
		if obj != nil && prop != nil && obj.Type() == "identifier" &&
			b.module[nodeText(obj, src)] && nodeText(prop, src) == "Router" {
			b.router[name] = true // const r = express.Router()
		}
	case "call_expression":
		if jsIsRequireOf(callee, src, jsExpressModuleID) {
			b.app[name] = true // const app = require("express")()
		}
	}
}

// jsExpressEmit recognises a single literal Express registration —
// <recv>.get/post/put/delete/patch/head/options/all("<path>",
// ...handlers) — and emits the route Symbol (always) plus the
// route→handler Relation (identifier / member-expression handlers
// only). Per the Express app.METHOD(path, callback [, callback ...])
// signature the terminal handler is the LAST argument; earlier ones
// are per-route middleware (mirror of the gin variadic rule). The
// len>=2 floor also keeps Express's one-argument application settings
// GETTER app.get(name) out of the route lane. app.route("/x").get(h)
// chaining registers via a zero-path .get(h) and is out of scope (the
// chained call carries no literal path of its own — never fabricate).
func jsExpressEmit(call *sitter.Node, src []byte, file string, b *jsExpressBindings, syms *[]types.Symbol, rels *[]types.Relation) {
	fn := call.ChildByFieldName("function")
	if fn == nil || fn.Type() != "member_expression" {
		return
	}
	prop := fn.ChildByFieldName("property")
	if prop == nil {
		return
	}
	verb := jsExpressVerbs[nodeText(prop, src)]
	if verb == "" {
		return
	}
	args := jsRouteCallArgs(call)
	if len(args) < 2 {
		return
	}
	path, ok := jsRouteStringLiteral(args[0], src)
	if !ok || !strings.HasPrefix(path, "/") {
		// Non-literal / dynamic path (or a non-path first argument —
		// regex, array, settings string): emit NOTHING (never
		// fabricate). The leading-slash floor mirrors the Go pass.
		return
	}

	// partial_path: app-bound receivers carry complete paths; Router-
	// bound and unresolvable receivers have an unknown mount prefix.
	partial := true
	if obj := fn.ChildByFieldName("object"); obj != nil && obj.Type() == "identifier" && b.app[nodeText(obj, src)] {
		partial = false
	}

	handler := args[len(args)-1]
	display := "func(...)"
	var handlerName, handlerRecv string
	hasTarget := false
	switch handler.Type() {
	case "identifier":
		handlerName = nodeText(handler, src)
		display = handlerName
		hasTarget = true
	case "member_expression":
		if p := handler.ChildByFieldName("property"); p != nil {
			handlerName = nodeText(p, src)
			if hobj := handler.ChildByFieldName("object"); hobj != nil {
				handlerRecv = nodeText(hobj, src)
			}
			display = nodeText(handler, src)
			hasTarget = true
		}
	}

	routeName := verb + " " + path
	line := nodeLine(call)
	*syms = append(*syms, types.Symbol{
		Name:      routeName,
		Kind:      "route",
		File:      file,
		Line:      line,
		EndLine:   line,
		Exported:  true,
		Signature: routeName + " -> " + display,
		// Parent MUST stay empty: file_map renders Parent.Name and
		// would mangle the route display.
	})
	if !hasTarget {
		// Arrow function / function expression / spread / any other
		// expression shape: Symbol only, no Relation.
		return
	}
	md := map[string]string{"framework": "express", "method": verb, "path": path}
	if partial {
		md["partial_path"] = "true"
	}
	*rels = append(*rels, types.Relation{
		Kind:       "reference",
		FromEP:     types.RelationEndpoint{Name: routeName, File: file, Line: line},
		ToEP:       types.RelationEndpoint{Name: handlerName, Receiver: handlerRecv, File: file, Line: line},
		File:       file,
		Line:       line,
		Confidence: types.ConfidenceRouteLiteral,
		Provenance: types.ProvenanceRouteResolver,
		ResolvedBy: jsExpressResolvedBy,
		Metadata:   md,
	})
}

// ---------------------------------------------------------------------------
// NestJS lane
// ---------------------------------------------------------------------------

// jsNestRoutes walks every class declaration (nested and exported
// included) and processes the ones carrying a @Controller(...)
// decorator. NestJS binds handlers via decorators on NAMED class
// methods — there is no anonymous registration form, so every route
// this lane emits has an identifier handler and therefore a Relation
// (the shared contract's "anonymous handler ⇒ Symbol only" branch is
// structurally unreachable, as for Spring).
func jsNestRoutes(root *sitter.Node, src []byte, file string, syms *[]types.Symbol, rels *[]types.Relation) {
	walkNamedChildren(root, true, func(n *sitter.Node) {
		if n.Type() != "class_declaration" {
			return
		}
		jsNestClassRoutes(n, src, file, syms, rels)
	})
}

// jsNestClassDecorators collects the decorator nodes attached to a
// class declaration. Grammar duality (probed against the vendored
// tree-sitter grammars): tree-sitter-typescript parks the decorators
// of an EXPORTED class on the export_statement and those of a plain
// class on the class_declaration; tree-sitter-javascript nests them in
// the class_declaration in both cases. Reading both sites covers every
// shape; nothing else ever produces decorator nodes there.
func jsNestClassDecorators(cls *sitter.Node) []*sitter.Node {
	decs := childrenByType(cls, "decorator")
	if parent := cls.Parent(); parent != nil && parent.Type() == "export_statement" {
		decs = append(decs, childrenByType(parent, "decorator")...)
	}
	return decs
}

// jsNestClassRoutes applies the controller gate to one class and, when
// it opens, emits routes for the class body's decorated methods.
func jsNestClassRoutes(cls *sitter.Node, src []byte, file string, syms *[]types.Symbol, rels *[]types.Relation) {
	gate := false
	prefix := ""
	prefixPartial := false
	for _, dec := range jsNestClassDecorators(cls) {
		// Registration form is always a decorator FACTORY call —
		// @Controller is invalid undecorated; only call_expression
		// children count.
		call := dec.NamedChild(0)
		if call == nil || call.Type() != "call_expression" {
			continue
		}
		if jsDecoratorCalleeName(call, src) != "Controller" {
			continue
		}
		gate = true
		args := jsRouteCallArgs(call)
		if len(args) == 0 {
			continue // @Controller() — empty prefix, precisely
		}
		if lit, ok := jsRouteStringLiteral(args[0], src); ok {
			prefix = lit
		} else {
			// A prefix argument exists but is not a clean literal
			// (options object, array of prefixes, constant): method
			// routes keep their LOCAL paths, flagged partial (never
			// fabricate the prefix).
			prefixPartial = true
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

	// Method decorators live in two grammar shapes (probed): the TS
	// grammar emits them as class_body siblings PRECEDING the
	// method_definition; the JS grammar nests them INSIDE the
	// method_definition. Accumulate the sibling shape and merge with
	// the nested shape at each method; any non-decorator member closes
	// the pending stack (its decorators belong to that member).
	var pending []*sitter.Node
	for i := 0; i < int(body.NamedChildCount()); i++ {
		member := body.NamedChild(i)
		switch member.Type() {
		case "decorator":
			pending = append(pending, member)
		case "method_definition":
			decs := append(append([]*sitter.Node{}, pending...), childrenByType(member, "decorator")...)
			jsNestMethodRoutes(member, decs, src, file, className, prefix, prefixPartial, syms, rels)
			pending = nil
		default:
			pending = nil
		}
	}
}

// jsNestMethodRoutes emits the route Symbol + route→handler Relation
// for each request-mapping decorator on one handler method.
func jsNestMethodRoutes(method *sitter.Node, decs []*sitter.Node, src []byte, file, className, prefix string, prefixPartial bool, syms *[]types.Symbol, rels *[]types.Relation) {
	nameNode := method.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	methodName := nodeText(nameNode, src)
	for _, dec := range decs {
		call := dec.NamedChild(0)
		if call == nil || call.Type() != "call_expression" {
			continue // bare @Get is not a registration (factory call required)
		}
		verb := jsNestVerbs[jsDecoratorCalleeName(call, src)]
		if verb == "" {
			continue // @UseGuards / @HttpCode / pipes / ... — not a mapping
		}
		local := ""
		if args := jsRouteCallArgs(call); len(args) > 0 {
			lit, ok := jsRouteStringLiteral(args[0], src)
			if !ok {
				// Constant reference / array of paths / template hole:
				// emit NOTHING for this mapping (never fabricate).
				continue
			}
			local = lit
		}
		// Empty decorator argument (or none): the class prefix IS the
		// path — jsJoinNestPaths(prefix, "") yields the prefix.
		fullPath := jsJoinNestPaths(prefix, local)
		routeName := verb + " " + fullPath
		line := nodeLine(dec)
		*syms = append(*syms, types.Symbol{
			Name:      routeName,
			Kind:      "route",
			File:      file,
			Line:      line,
			EndLine:   line,
			Exported:  true,
			Signature: routeName + " -> " + className + "." + methodName,
			// Parent MUST stay empty: file_map renders Parent.Name and
			// would mangle the route display.
		})
		md := map[string]string{"framework": "nestjs", "method": verb, "path": fullPath}
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
			ResolvedBy: jsNestResolvedBy,
			Metadata:   md,
		})
	}
}

// jsDecoratorCalleeName returns the trailing name of a decorator
// factory callee: "Get" for both @Get(...) and the namespace-import
// form @common.Get(...).
func jsDecoratorCalleeName(call *sitter.Node, src []byte) string {
	fn := call.ChildByFieldName("function")
	if fn == nil {
		return ""
	}
	switch fn.Type() {
	case "identifier":
		return nodeText(fn, src)
	case "member_expression":
		if prop := fn.ChildByFieldName("property"); prop != nil {
			return nodeText(prop, src)
		}
	}
	return ""
}

// jsJoinNestPaths concatenates the controller prefix and the method
// path with exactly one slash at the join, mirroring NestJS's path
// normalisation (RouterExplorer/PathsExplorer strip and re-add
// slashes; controller prefixes are accepted with or without a leading
// slash): the all-empty combination maps to "/" and a nonempty result
// is normalised to a leading slash.
func jsJoinNestPaths(prefix, local string) string {
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

// ---------------------------------------------------------------------------
// Shared literal helpers
// ---------------------------------------------------------------------------

// jsRouteCallArgs returns the named argument expressions of a call,
// skipping interleaved comments.
func jsRouteCallArgs(call *sitter.Node) []*sitter.Node {
	argList := call.ChildByFieldName("arguments")
	if argList == nil {
		return nil
	}
	var out []*sitter.Node
	for i := 0; i < int(argList.NamedChildCount()); i++ {
		ch := argList.NamedChild(i)
		if ch.Type() == "comment" {
			continue
		}
		out = append(out, ch)
	}
	return out
}

// jsRouteStringLiteral returns the content of a string literal:
// quoted strings ('x' / "x") and template strings WITHOUT
// substitution holes (their value is a compile-time constant — the JS
// analogue of the Python pass's hole-free f-string acceptance).
// ok=false for template strings with ${...} holes and every other
// expression shape — those are dynamic paths.
func jsRouteStringLiteral(n *sitter.Node, src []byte) (string, bool) {
	if n == nil {
		return "", false
	}
	switch n.Type() {
	case "string", "template_string":
		var b strings.Builder
		for i := 0; i < int(n.NamedChildCount()); i++ {
			ch := n.NamedChild(i)
			switch ch.Type() {
			case "template_substitution":
				return "", false // ${...} hole — dynamic path
			case "string_fragment", "escape_sequence":
				b.WriteString(nodeText(ch, src))
			}
		}
		return b.String(), true
	}
	return "", false
}
