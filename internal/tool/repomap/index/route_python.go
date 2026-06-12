package index

// route_python.go — FastAPI decorator route -> handler resolver
// (framework route lane, P0.4).
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
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

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
