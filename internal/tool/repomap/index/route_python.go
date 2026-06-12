package index

// route_python.go — Python framework route -> handler resolvers
// (framework route lane, P0.4): FastAPI decorators plus Flask
// decorators (Flask section further down). Each pass sits behind its
// OWN precise per-file import gate; a file importing both frameworks
// runs both passes and pyExtractRoutes dedupes by (verb, path, line).
//
// ---------------------------------------------------------------------------
// FastAPI section
// ---------------------------------------------------------------------------
//
// Recognises the static decorator registration form
//
//	@app.get("/users/{user_id}")
//	def read_user(user_id: int): ...
//
// and emits one Kind="route" Symbol per HTTP-method decorator plus a
// Kind="reference" Relation from the route to the decorated handler
// function. The pass is gated on a PRECISE per-file signal: an import
// whose Path is verbatim "fastapi" or starts with "fastapi." (receiver
// names like `app` / `router` are noisy and never gate anything by
// themselves — architectural red line: precise signals for hard gates).
//
// Dynamic paths (f-string interpolation, identifiers, any non-literal
// first positional argument) emit NOTHING for that registration —
// fabricating a path is worse than missing one.
//
// Prefix semantics: FastAPI composes prefixes at include_router() /
// APIRouter(prefix=...) time, and include_router calls live in OTHER
// files in real projects — cross-file composition is out of scope for
// this per-file pass. Decorators bound to an APIRouter-assigned name
// therefore emit the LOCAL path with Metadata["partial_path"]="true";
// names assigned from FastAPI(...) emit the path as-is; receivers that
// match neither tracked assignment still emit (the import gate already
// proved the framework) but are marked partial_path=true because their
// mount point is unknown.

import (
	"strconv"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// pyExtractRoutes is the Python framework-route post-pass entry point,
// hooked at the end of extractPython. It runs every per-framework pass
// independently — each behind its own precise import gate — and then
// dedupes by (verb, path, line): Flask 2.x exposes the same
// @x.get/.post decorator shape as FastAPI, so a file importing BOTH
// frameworks would otherwise emit one physical registration twice.
// The FastAPI pass wins ties, keeping its emissions byte-identical to
// the pre-Flask era.
func pyExtractRoutes(root *sitter.Node, src []byte, file string, imps []types.Import) ([]types.Symbol, []types.Relation) {
	syms, rels := pyExtractFastAPIRoutes(root, src, file, imps)
	flaskSyms, flaskRels := pyExtractFlaskRoutes(root, src, file, imps)
	if len(flaskSyms) == 0 && len(flaskRels) == 0 {
		return syms, rels
	}
	// Route Symbol Name is "<VERB> <path>" by the shared contract, so
	// Name+Line IS the (verb, path, line) dedupe key; relations reuse
	// it via FromEP.Name (always the route name) + the same line.
	seen := map[string]bool{}
	for _, s := range syms {
		seen[s.Name+"\n"+strconv.Itoa(s.Line)] = true
	}
	for _, s := range flaskSyms {
		if !seen[s.Name+"\n"+strconv.Itoa(s.Line)] {
			syms = append(syms, s)
		}
	}
	for _, r := range flaskRels {
		if !seen[r.FromEP.Name+"\n"+strconv.Itoa(r.Line)] {
			rels = append(rels, r)
		}
	}
	return syms, rels
}

// pyFastAPIVerbs maps the HTTP-method decorator names exposed by
// FastAPI's APIRouter / FastAPI application class to canonical verbs.
// The table comes from FastAPI's API surface — APIRouter defines
// exactly the decorators .get / .put / .post / .delete / .options /
// .head / .patch / .trace (fastapi/routing.py, documented at
// https://fastapi.tiangolo.com/reference/apirouter/) — NOT from test
// fixtures (generalization red line). `websocket` is deliberately
// absent (not an HTTP verb route) and `api_route` is absent because
// its methods=[...] list argument is not a single-verb registration.
var pyFastAPIVerbs = map[string]string{
	"get":     "GET",
	"put":     "PUT",
	"post":    "POST",
	"delete":  "DELETE",
	"options": "OPTIONS",
	"head":    "HEAD",
	"patch":   "PATCH",
	"trace":   "TRACE",
}

// pyResolvedByFastAPI is the ResolvedBy tag for route edges produced
// by this pass.
const pyResolvedByFastAPI = "fastapi_decorator"

// pyAppBinding classifies what a module-level name was assigned from.
type pyAppBinding int

const (
	pyAppUnknown   pyAppBinding = iota
	pyAppFastAPI                // x = FastAPI(...) — full app, path is complete
	pyAppAPIRouter              // x = APIRouter(...) — mounted later, path is partial
)

// pyFastAPIImported is the precise per-file framework gate: a verbatim
// import-path match on "fastapi" or a "fastapi." submodule. The imps
// slice is the one extractPython already collected for this file.
func pyFastAPIImported(imps []types.Import) bool {
	for _, imp := range imps {
		if imp.Path == "fastapi" || strings.HasPrefix(imp.Path, "fastapi.") {
			return true
		}
	}
	return false
}

// pyExtractFastAPIRoutes is the post-pass entry point, hooked at the
// end of extractPython. Returns route symbols + route->handler
// relations; both empty when the file does not import fastapi.
func pyExtractFastAPIRoutes(root *sitter.Node, src []byte, file string, imps []types.Import) ([]types.Symbol, []types.Relation) {
	if root == nil || !pyFastAPIImported(imps) {
		return nil, nil
	}
	bindings := pyCollectFastAPIBindings(root, src)

	var syms []types.Symbol
	var rels []types.Relation

	// Recursive walk carrying the enclosing class name so decorated
	// methods (class-based registration style) can stamp the class
	// into ToEP.Receiver — the Python analogue of the Spring class
	// name slot in the shared route contract.
	var walk func(n *sitter.Node, enclosingClass string)
	walk = func(n *sitter.Node, enclosingClass string) {
		for i := 0; i < int(n.NamedChildCount()); i++ {
			ch := n.NamedChild(i)
			switch ch.Type() {
			case "decorated_definition":
				s, r := pyRoutesFromDecorated(ch, src, file, enclosingClass, bindings)
				syms = append(syms, s...)
				rels = append(rels, r...)
				// Recurse into the decorated definition itself:
				// nested defs/classes may register further routes.
				if def := ch.ChildByFieldName("definition"); def != nil {
					next := enclosingClass
					if def.Type() == "class_definition" {
						if nm := def.ChildByFieldName("name"); nm != nil {
							next = nodeText(nm, src)
						}
					}
					walk(def, next)
				}
			case "class_definition":
				next := enclosingClass
				if nm := ch.ChildByFieldName("name"); nm != nil {
					next = nodeText(nm, src)
				}
				walk(ch, next)
			default:
				walk(ch, enclosingClass)
			}
		}
	}
	walk(root, "")
	return syms, rels
}

// pyCollectFastAPIBindings tracks same-file MODULE-LEVEL assignments
// of the form `x = FastAPI(...)` / `x = APIRouter(...)` (plain or
// annotated, identifier or attribute callee — `fastapi.FastAPI(...)`
// matches on the trailing constructor name; the file-level import
// gate has already proven the framework). Later assignments to the
// same name win, mirroring module execution order. Function-local
// app factories are out of scope: decorators referencing them fall
// into the unknown-receiver lane (emit + partial_path).
func pyCollectFastAPIBindings(root *sitter.Node, src []byte) map[string]pyAppBinding {
	out := map[string]pyAppBinding{}
	for i := 0; i < int(root.NamedChildCount()); i++ {
		stmt := root.NamedChild(i)
		if stmt.Type() != "expression_statement" {
			continue
		}
		for j := 0; j < int(stmt.NamedChildCount()); j++ {
			assign := stmt.NamedChild(j)
			if assign.Type() != "assignment" {
				continue
			}
			left := assign.ChildByFieldName("left")
			right := assign.ChildByFieldName("right")
			if left == nil || right == nil || left.Type() != "identifier" || right.Type() != "call" {
				continue
			}
			callee := right.ChildByFieldName("function")
			ctor := ""
			switch {
			case callee == nil:
				continue
			case callee.Type() == "identifier":
				ctor = nodeText(callee, src)
			case callee.Type() == "attribute":
				if attr := callee.ChildByFieldName("attribute"); attr != nil {
					ctor = nodeText(attr, src)
				}
			}
			switch ctor {
			case "FastAPI":
				out[nodeText(left, src)] = pyAppFastAPI
			case "APIRouter":
				out[nodeText(left, src)] = pyAppAPIRouter
			}
		}
	}
	return out
}

// pyRoutesFromDecorated processes one decorated_definition: every
// HTTP-method decorator in registration form (decorator -> call ->
// attribute, literal first positional path) yields a route Symbol;
// the route->handler Relation is added only when the decorated
// definition is a named function (FastAPI's documented "path operation
// function"). A decorated non-function (e.g. a class) keeps the route
// Symbol — the registration is real — but gets NO relation, the
// Python flavor of the shared contract's anonymous-handler lane.
func pyRoutesFromDecorated(node *sitter.Node, src []byte, file, enclosingClass string, bindings map[string]pyAppBinding) ([]types.Symbol, []types.Relation) {
	def := node.ChildByFieldName("definition")
	handlerName := ""
	handlerIsFunc := false
	if def != nil {
		if nm := def.ChildByFieldName("name"); nm != nil {
			handlerName = nodeText(nm, src)
		}
		handlerIsFunc = def.Type() == "function_definition"
	}

	var syms []types.Symbol
	var rels []types.Relation
	for i := 0; i < int(node.NamedChildCount()); i++ {
		dec := node.NamedChild(i)
		if dec.Type() != "decorator" || dec.NamedChildCount() == 0 {
			continue
		}
		// Registration form: decorator -> call -> attribute.
		call := dec.NamedChild(0)
		if call.Type() != "call" {
			continue
		}
		fn := call.ChildByFieldName("function")
		if fn == nil || fn.Type() != "attribute" {
			continue
		}
		attr := fn.ChildByFieldName("attribute")
		if attr == nil {
			continue
		}
		verb, ok := pyFastAPIVerbs[nodeText(attr, src)]
		if !ok {
			continue
		}
		path, ok := pyRoutePathArg(call, src)
		if !ok {
			// Non-literal / dynamic / missing path: emit NOTHING
			// for this registration (never fabricate paths).
			continue
		}

		// partial_path: APIRouter-bound and unknown receivers have an
		// unknown mount prefix; FastAPI-bound paths are complete.
		partial := true
		if obj := fn.ChildByFieldName("object"); obj != nil && obj.Type() == "identifier" {
			if bindings[nodeText(obj, src)] == pyAppFastAPI {
				partial = false
			}
		}

		regLine := nodeLine(dec)
		// TrimSpace keeps the rare-but-real empty router path
		// (`@router.get("")`) from leaving a trailing space in the
		// symbol name; Metadata["path"] still carries it verbatim.
		routeName := strings.TrimSpace(verb + " " + path)
		sig := routeName
		if handlerName != "" {
			sig += " -> " + handlerName
		}
		syms = append(syms, types.Symbol{
			Name:      routeName,
			Kind:      "route",
			File:      file,
			Line:      regLine,
			EndLine:   regLine,
			Exported:  true,
			Signature: sig,
			// Parent stays empty by contract: file_map renders
			// Parent.Name and would mangle the route display.
		})

		if !handlerIsFunc || handlerName == "" {
			continue // anonymous-handler lane: symbol only, no relation
		}
		md := map[string]string{
			"framework": "fastapi",
			"method":    verb,
			"path":      path,
		}
		if partial {
			md["partial_path"] = "true"
		}
		rels = append(rels, types.Relation{
			Kind:       "reference",
			FromEP:     types.RelationEndpoint{Name: routeName, File: file, Line: regLine},
			ToEP:       types.RelationEndpoint{Name: handlerName, Receiver: enclosingClass, File: file, Line: regLine},
			File:       file,
			Line:       regLine,
			Confidence: types.ConfidenceRouteLiteral,
			Provenance: types.ProvenanceRouteResolver,
			ResolvedBy: pyResolvedByFastAPI,
			Metadata:   md,
		})
	}
	return syms, rels
}

// pyRoutePathArg extracts the first positional argument of a
// registration call and returns its literal text. ok=false for
// anything dynamic: f-string interpolation, concatenation,
// identifiers, splats, or a missing positional argument.
func pyRoutePathArg(call *sitter.Node, src []byte) (string, bool) {
	args := call.ChildByFieldName("arguments")
	if args == nil {
		return "", false
	}
	for i := 0; i < int(args.NamedChildCount()); i++ {
		ch := args.NamedChild(i)
		switch ch.Type() {
		case "comment":
			continue
		case "keyword_argument":
			// Valid Python places positionals before keywords; once
			// keywords start there is no positional path argument.
			return "", false
		default:
			return pyStringLiteral(ch, src)
		}
	}
	return "", false
}

// pyStringLiteral returns the content of a plain string literal
// (quotes stripped, escape sequences kept verbatim). ok=false for
// non-string nodes and for f-strings containing interpolation holes —
// those are dynamic paths. An f-string WITHOUT holes is accepted: its
// value is a compile-time constant.
func pyStringLiteral(n *sitter.Node, src []byte) (string, bool) {
	if n == nil || n.Type() != "string" {
		return "", false
	}
	var b strings.Builder
	for i := 0; i < int(n.NamedChildCount()); i++ {
		ch := n.NamedChild(i)
		switch ch.Type() {
		case "interpolation":
			return "", false // f-string hole — dynamic path
		case "string_content", "escape_sequence":
			b.WriteString(nodeText(ch, src))
		}
		// string_start / string_end are named in this grammar; skip.
	}
	return b.String(), true
}

// ---------------------------------------------------------------------------
// Flask section
// ---------------------------------------------------------------------------
//
// Recognises Flask's static decorator registration forms
//
//	@app.route("/users", methods=["GET", "POST"])   # one route per verb
//	@app.route("/users")                            # GET only (see below)
//	@app.get("/users")                              # Flask 2.x shortcuts
//
// gated on a PRECISE per-file signal: an import whose Path is verbatim
// "flask" or starts with "flask." — deliberately separate from the
// fastapi gate; receiver names never gate anything by themselves.
//
// Verb sources (Flask's API surface, NOT fixtures):
//
//   - @x.route(rule, methods=[...]): flask.sansio.scaffold.Scaffold
//     .route delegates to add_url_rule, whose `methods` parameter is
//     documented as "a list of methods this rule should be limited
//     to". Each STRING-LITERAL element yields one Symbol+Relation,
//     upper-cased because werkzeug.routing.Rule upper-cases every
//     method it stores. A `methods` kwarg that is not a list/tuple/set
//     of plain string literals (identifier, comprehension, f-string
//     hole, splat...) emits NOTHING for that registration — verbs are
//     never fabricated, mirroring the dynamic-path rule.
//   - No methods kwarg: add_url_rule documents the default as GET
//     only. HEAD (werkzeug answers it via GET) and OPTIONS (Flask's
//     automatic provide_automatic_options) are implicit framework
//     additions to EVERY route — the author never wrote them, so
//     emitting them would triple the route surface with zero
//     navigational value; deliberately not emitted.
//   - Shortcuts: Flask 2.0's Scaffold defines exactly .get / .post /
//     .put / .delete / .patch ("Shortcut for route() with
//     methods=['GET']" etc.). There are NO head/options/trace
//     shortcuts on the Flask API surface, so none appear in the table.
//
// Prefix semantics: flask.Blueprint(name, import_name,
// url_prefix=...) documents url_prefix as "a path to prepend to all
// the blueprint's URLs"; composition happens in
// flask.blueprints.BlueprintSetupState.add_url_rule as
// "/".join((url_prefix.rstrip("/"), rule.lstrip("/"))), which
// pyFlaskJoinPrefix mirrors. A LITERAL url_prefix is therefore
// composed per-file and the route emitted complete; a non-literal (or
// absent — app.register_blueprint may supply one) url_prefix emits the
// LOCAL path with Metadata["partial_path"]="true". register_blueprint
// overrides live in other files in real projects — cross-file
// composition is out of scope, the same stance the FastAPI section
// takes for include_router. Receivers matching no tracked assignment
// still emit (the import gate already proved the framework) but are
// marked partial_path=true because their mount point is unknown.

// pyFlaskVerbShortcuts maps the Flask 2.x HTTP-method shortcut
// decorator names to canonical verbs. The table is exactly the five
// shortcuts Scaffold defines (flask/sansio/scaffold.py: get / post /
// put / delete / patch, each "Shortcut for route() with
// methods=[...]") — NOT from test fixtures (generalization red line).
var pyFlaskVerbShortcuts = map[string]string{
	"get":    "GET",
	"post":   "POST",
	"put":    "PUT",
	"delete": "DELETE",
	"patch":  "PATCH",
}

// pyResolvedByFlask is the ResolvedBy tag for route edges produced by
// the Flask pass.
const pyResolvedByFlask = "flask_route_literal"

// pyFlaskBindingKind classifies what a module-level name was assigned
// from, Flask flavor.
type pyFlaskBindingKind int

const (
	pyFlaskUnknown   pyFlaskBindingKind = iota // not a tracked Flask receiver (map zero value)
	pyFlaskApp                                 // x = Flask(__name__) — full app, path is complete
	pyFlaskBlueprint                           // x = Blueprint(...) — mounted by register_blueprint
)

// pyFlaskBinding is one tracked module-level Flask receiver: its kind,
// the blueprint's literal url_prefix (when composable), and whether
// the mount prefix is partial (non-literal or absent url_prefix).
type pyFlaskBinding struct {
	kind    pyFlaskBindingKind
	prefix  string
	partial bool
}

// pyFlaskImported is the precise per-file framework gate for the
// Flask pass: a verbatim import-path match on "flask" or a "flask."
// submodule. Deliberately separate from pyFastAPIImported — each
// framework pass opens on its own import only.
func pyFlaskImported(imps []types.Import) bool {
	for _, imp := range imps {
		if imp.Path == "flask" || strings.HasPrefix(imp.Path, "flask.") {
			return true
		}
	}
	return false
}

// pyExtractFlaskRoutes is the Flask pass entry point, invoked from
// pyExtractRoutes. Returns route symbols + route->handler relations;
// both empty when the file does not import flask. The walker mirrors
// the FastAPI one: it carries the enclosing class name so decorated
// methods stamp the class into ToEP.Receiver.
func pyExtractFlaskRoutes(root *sitter.Node, src []byte, file string, imps []types.Import) ([]types.Symbol, []types.Relation) {
	if root == nil || !pyFlaskImported(imps) {
		return nil, nil
	}
	bindings := pyCollectFlaskBindings(root, src)

	var syms []types.Symbol
	var rels []types.Relation

	var walk func(n *sitter.Node, enclosingClass string)
	walk = func(n *sitter.Node, enclosingClass string) {
		for i := 0; i < int(n.NamedChildCount()); i++ {
			ch := n.NamedChild(i)
			switch ch.Type() {
			case "decorated_definition":
				s, r := pyFlaskRoutesFromDecorated(ch, src, file, enclosingClass, bindings)
				syms = append(syms, s...)
				rels = append(rels, r...)
				if def := ch.ChildByFieldName("definition"); def != nil {
					next := enclosingClass
					if def.Type() == "class_definition" {
						if nm := def.ChildByFieldName("name"); nm != nil {
							next = nodeText(nm, src)
						}
					}
					walk(def, next)
				}
			case "class_definition":
				next := enclosingClass
				if nm := ch.ChildByFieldName("name"); nm != nil {
					next = nodeText(nm, src)
				}
				walk(ch, next)
			default:
				walk(ch, enclosingClass)
			}
		}
	}
	walk(root, "")
	return syms, rels
}

// pyCollectFlaskBindings tracks same-file MODULE-LEVEL assignments of
// the form `x = Flask(...)` / `x = Blueprint(...)` (identifier or
// attribute callee — `flask.Flask(...)` matches on the trailing
// constructor name; the file-level import gate has already proven the
// framework). Later assignments to the same name win, mirroring
// module execution order. Function-local app factories fall into the
// unknown-receiver lane (emit + partial_path).
func pyCollectFlaskBindings(root *sitter.Node, src []byte) map[string]pyFlaskBinding {
	out := map[string]pyFlaskBinding{}
	for i := 0; i < int(root.NamedChildCount()); i++ {
		stmt := root.NamedChild(i)
		if stmt.Type() != "expression_statement" {
			continue
		}
		for j := 0; j < int(stmt.NamedChildCount()); j++ {
			assign := stmt.NamedChild(j)
			if assign.Type() != "assignment" {
				continue
			}
			left := assign.ChildByFieldName("left")
			right := assign.ChildByFieldName("right")
			if left == nil || right == nil || left.Type() != "identifier" || right.Type() != "call" {
				continue
			}
			callee := right.ChildByFieldName("function")
			ctor := ""
			switch {
			case callee == nil:
				continue
			case callee.Type() == "identifier":
				ctor = nodeText(callee, src)
			case callee.Type() == "attribute":
				if attr := callee.ChildByFieldName("attribute"); attr != nil {
					ctor = nodeText(attr, src)
				}
			}
			switch ctor {
			case "Flask":
				out[nodeText(left, src)] = pyFlaskBinding{kind: pyFlaskApp}
			case "Blueprint":
				out[nodeText(left, src)] = pyFlaskBlueprintBinding(right, src)
			}
		}
	}
	return out
}

// pyFlaskBlueprintBinding classifies one Blueprint(...) constructor
// call by its url_prefix keyword argument: literal string -> prefix
// composable per-file; non-literal or absent (register_blueprint may
// still supply one cross-file) -> partial. url_prefix is read as a
// keyword only — that is its documented usage; a (vanishingly rare)
// positional url_prefix lands in the partial lane, which never
// fabricates a prefix.
func pyFlaskBlueprintBinding(call *sitter.Node, src []byte) pyFlaskBinding {
	b := pyFlaskBinding{kind: pyFlaskBlueprint, partial: true}
	args := call.ChildByFieldName("arguments")
	if args == nil {
		return b
	}
	for i := 0; i < int(args.NamedChildCount()); i++ {
		kw := args.NamedChild(i)
		if kw.Type() != "keyword_argument" {
			continue
		}
		name := kw.ChildByFieldName("name")
		if name == nil || nodeText(name, src) != "url_prefix" {
			continue
		}
		if v, ok := pyStringLiteral(kw.ChildByFieldName("value"), src); ok {
			b.prefix = v
			b.partial = false
		}
		return b
	}
	return b
}

// pyFlaskRoutesFromDecorated processes one decorated_definition for
// the Flask pass: every registration-form decorator (decorator ->
// call -> attribute, literal first positional path, resolvable verb
// set) yields one route Symbol per verb; the route->handler Relation
// is added only when the decorated definition is a named function (a
// Flask view function). A decorated non-function keeps the route
// Symbol — the registration is real — but gets NO relation: the
// Python flavor of the shared contract's anonymous-handler lane.
func pyFlaskRoutesFromDecorated(node *sitter.Node, src []byte, file, enclosingClass string, bindings map[string]pyFlaskBinding) ([]types.Symbol, []types.Relation) {
	def := node.ChildByFieldName("definition")
	handlerName := ""
	handlerIsFunc := false
	if def != nil {
		if nm := def.ChildByFieldName("name"); nm != nil {
			handlerName = nodeText(nm, src)
		}
		handlerIsFunc = def.Type() == "function_definition"
	}

	var syms []types.Symbol
	var rels []types.Relation
	for i := 0; i < int(node.NamedChildCount()); i++ {
		dec := node.NamedChild(i)
		if dec.Type() != "decorator" || dec.NamedChildCount() == 0 {
			continue
		}
		// Registration form: decorator -> call -> attribute.
		call := dec.NamedChild(0)
		if call.Type() != "call" {
			continue
		}
		fn := call.ChildByFieldName("function")
		if fn == nil || fn.Type() != "attribute" {
			continue
		}
		attr := fn.ChildByFieldName("attribute")
		if attr == nil {
			continue
		}
		var verbs []string
		if attrName := nodeText(attr, src); attrName == "route" {
			vs, ok := pyFlaskRouteMethods(call, src)
			if !ok {
				// Non-literal methods kwarg: emit NOTHING for this
				// registration (never fabricate verbs).
				continue
			}
			verbs = vs
		} else if v, ok := pyFlaskVerbShortcuts[attrName]; ok {
			verbs = []string{v}
		} else {
			continue
		}
		path, ok := pyRoutePathArg(call, src)
		if !ok {
			// Non-literal / dynamic / missing path: emit NOTHING
			// for this registration (never fabricate paths).
			continue
		}

		// partial_path: Flask-bound paths are complete; blueprints
		// with a literal url_prefix compose per-file (complete);
		// everything else has an unknown mount prefix.
		partial := true
		if obj := fn.ChildByFieldName("object"); obj != nil && obj.Type() == "identifier" {
			switch b := bindings[nodeText(obj, src)]; {
			case b.kind == pyFlaskApp:
				partial = false
			case b.kind == pyFlaskBlueprint && !b.partial:
				path = pyFlaskJoinPrefix(b.prefix, path)
				partial = false
			}
		}

		regLine := nodeLine(dec)
		for _, verb := range verbs {
			// TrimSpace keeps an empty rule (`@bp.route("")` with no
			// composable prefix) from leaving a trailing space in the
			// symbol name; Metadata["path"] still carries it verbatim.
			routeName := strings.TrimSpace(verb + " " + path)
			sig := routeName
			if handlerName != "" {
				sig += " -> " + handlerName
			}
			syms = append(syms, types.Symbol{
				Name:      routeName,
				Kind:      "route",
				File:      file,
				Line:      regLine,
				EndLine:   regLine,
				Exported:  true,
				Signature: sig,
				// Parent stays empty by contract: file_map renders
				// Parent.Name and would mangle the route display.
			})

			if !handlerIsFunc || handlerName == "" {
				continue // anonymous-handler lane: symbol only, no relation
			}
			md := map[string]string{
				"framework": "flask",
				"method":    verb,
				"path":      path,
			}
			if partial {
				md["partial_path"] = "true"
			}
			rels = append(rels, types.Relation{
				Kind:       "reference",
				FromEP:     types.RelationEndpoint{Name: routeName, File: file, Line: regLine},
				ToEP:       types.RelationEndpoint{Name: handlerName, Receiver: enclosingClass, File: file, Line: regLine},
				File:       file,
				Line:       regLine,
				Confidence: types.ConfidenceRouteLiteral,
				Provenance: types.ProvenanceRouteResolver,
				ResolvedBy: pyResolvedByFlask,
				Metadata:   md,
			})
		}
	}
	return syms, rels
}

// pyFlaskRouteMethods resolves the verb list of one @x.route(...)
// registration from its methods=... keyword argument. No methods
// kwarg -> Flask's documented default of GET only (HEAD/OPTIONS are
// implicit framework additions — see the section comment). A
// list/tuple/set whose every element is a plain string literal ->
// one upper-cased verb per element (werkzeug Rule upper-cases stored
// methods). Anything else — identifier, comprehension, splat, any
// non-literal element — returns ok=false: emit NOTHING for that
// registration.
func pyFlaskRouteMethods(call *sitter.Node, src []byte) ([]string, bool) {
	args := call.ChildByFieldName("arguments")
	if args == nil {
		return []string{"GET"}, true
	}
	for i := 0; i < int(args.NamedChildCount()); i++ {
		kw := args.NamedChild(i)
		if kw.Type() != "keyword_argument" {
			continue
		}
		name := kw.ChildByFieldName("name")
		if name == nil || nodeText(name, src) != "methods" {
			continue
		}
		v := kw.ChildByFieldName("value")
		if v == nil {
			return nil, false
		}
		switch v.Type() {
		case "list", "tuple", "set":
			var verbs []string
			for j := 0; j < int(v.NamedChildCount()); j++ {
				el := v.NamedChild(j)
				if el.Type() == "comment" {
					continue
				}
				s, ok := pyStringLiteral(el, src)
				if !ok {
					return nil, false // one dynamic element poisons the registration
				}
				verbs = append(verbs, strings.ToUpper(s))
			}
			return verbs, true
		default:
			return nil, false
		}
	}
	return []string{"GET"}, true
}

// pyFlaskJoinPrefix mirrors the prefix composition in
// flask.blueprints.BlueprintSetupState.add_url_rule:
// "/".join((url_prefix.rstrip("/"), rule.lstrip("/"))); an empty rule
// mounts the route at the prefix itself.
func pyFlaskJoinPrefix(prefix, rule string) string {
	if rule == "" {
		return prefix
	}
	return strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(rule, "/")
}
