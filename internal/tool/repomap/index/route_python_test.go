package index

import (
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// parsePythonForRoutes mirrors parser.go's tree-sitter bootstrap so
// the tests can exercise extractPython directly (inline-source
// pattern, same as extract_kotlin_test.go).
func parsePythonForRoutes(t *testing.T, src []byte) *sitter.Node {
	t.Helper()
	root, ok := parseTreeSitterIfPossible(types.LangPython, src)
	if !ok {
		t.Fatalf("parseTreeSitterIfPossible(python) returned !ok — tree-sitter python binding missing?")
	}
	return root
}

// extractPyRoutes runs the full Python extractor and filters down to
// the route lane: Kind=route symbols and fastapi_decorator relations.
func extractPyRoutes(t *testing.T, src string) ([]types.Symbol, []types.Relation) {
	t.Helper()
	b := []byte(src)
	root := parsePythonForRoutes(t, b)
	_, syms, _, rels := extractPython(root, b, "api.py")
	var routeSyms []types.Symbol
	for _, s := range syms {
		if s.Kind == "route" {
			routeSyms = append(routeSyms, s)
		}
	}
	var routeRels []types.Relation
	for _, r := range rels {
		if r.ResolvedBy == pyResolvedByFastAPI {
			routeRels = append(routeRels, r)
		}
	}
	return routeSyms, routeRels
}

// TestPyFastAPIRoutes_SimpleRoute pins the canonical surface: one
// FastAPI-bound GET decorator produces one route symbol (full shared
// contract: Name/Kind/Line/Exported/Signature/empty Parent) and one
// route->handler relation carrying the construction-stage diagnostics
// plus the framework metadata, with NO partial_path.
func TestPyFastAPIRoutes_SimpleRoute(t *testing.T) {
	src := `from fastapi import FastAPI

app = FastAPI()

@app.get("/users/{user_id}")
def read_user(user_id: int):
    return {"user_id": user_id}
`
	syms, rels := extractPyRoutes(t, src)
	if len(syms) != 1 {
		t.Fatalf("want 1 route symbol, got %d: %+v", len(syms), syms)
	}
	s := syms[0]
	if s.Name != "GET /users/{user_id}" {
		t.Errorf("symbol name: want %q, got %q", "GET /users/{user_id}", s.Name)
	}
	if s.Kind != "route" {
		t.Errorf("symbol kind: want route, got %q", s.Kind)
	}
	if s.Line != 5 || s.EndLine != 5 {
		t.Errorf("symbol line: want decorator line 5/5, got %d/%d", s.Line, s.EndLine)
	}
	if !s.Exported {
		t.Errorf("route symbol must be Exported")
	}
	if s.Parent != "" {
		t.Errorf("route symbol Parent MUST stay empty (file_map renders Parent.Name); got %q", s.Parent)
	}
	if want := "GET /users/{user_id} -> read_user"; s.Signature != want {
		t.Errorf("signature: want %q, got %q", want, s.Signature)
	}

	if len(rels) != 1 {
		t.Fatalf("want 1 route relation, got %d: %+v", len(rels), rels)
	}
	r := rels[0]
	if r.Kind != "reference" {
		t.Errorf("relation kind: want reference, got %q", r.Kind)
	}
	if r.FromEP.Name != s.Name || r.FromEP.File != "api.py" || r.FromEP.Line != 5 {
		t.Errorf("FromEP: want {%q api.py 5}, got %+v", s.Name, r.FromEP)
	}
	if r.ToEP.Name != "read_user" || r.ToEP.Receiver != "" || r.ToEP.Line != 5 {
		t.Errorf("ToEP: want {read_user <no receiver> line 5}, got %+v", r.ToEP)
	}
	if r.Confidence != types.ConfidenceRouteLiteral {
		t.Errorf("confidence: want ConfidenceRouteLiteral, got %v", r.Confidence)
	}
	if r.Provenance != types.ProvenanceRouteResolver {
		t.Errorf("provenance: want ProvenanceRouteResolver, got %q", r.Provenance)
	}
	if r.ResolvedBy != "fastapi_decorator" {
		t.Errorf("resolved_by: want fastapi_decorator, got %q", r.ResolvedBy)
	}
	if r.Metadata["framework"] != "fastapi" || r.Metadata["method"] != "GET" || r.Metadata["path"] != "/users/{user_id}" {
		t.Errorf("metadata: want framework/method/path, got %+v", r.Metadata)
	}
	if _, has := r.Metadata["partial_path"]; has {
		t.Errorf("FastAPI-bound app path is complete — partial_path must be absent; got %+v", r.Metadata)
	}
}

// TestPyFastAPIRoutes_MethodHandlerCarriesClassReceiver covers the
// selector/attribute-handler flavor for Python: a decorated method
// inside a class stamps the enclosing class name into ToEP.Receiver
// (the slot the shared contract uses for Spring class names).
func TestPyFastAPIRoutes_MethodHandlerCarriesClassReceiver(t *testing.T) {
	src := `from fastapi import APIRouter

router = APIRouter()

class ItemService:
    @router.post("/items")
    def create_item(self, item):
        return item
`
	syms, rels := extractPyRoutes(t, src)
	if len(syms) != 1 {
		t.Fatalf("want 1 route symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "POST /items" {
		t.Errorf("symbol name: want %q, got %q", "POST /items", syms[0].Name)
	}
	if len(rels) != 1 {
		t.Fatalf("want 1 route relation, got %d: %+v", len(rels), rels)
	}
	r := rels[0]
	if r.ToEP.Name != "create_item" {
		t.Errorf("ToEP.Name: want create_item, got %q", r.ToEP.Name)
	}
	if r.ToEP.Receiver != "ItemService" {
		t.Errorf("ToEP.Receiver: want enclosing class ItemService, got %q", r.ToEP.Receiver)
	}
	if r.Metadata["partial_path"] != "true" {
		t.Errorf("APIRouter-bound route must carry partial_path=true; got %+v", r.Metadata)
	}
}

// TestPyFastAPIRoutes_AnonymousHandlerSymbolOnly: Python decorator
// syntax cannot host a lambda, so the anonymous-handler lane covers a
// decorated NON-function (here a class — not a FastAPI "path operation
// function"): the registration is real, so the route Symbol is
// emitted, but NO route->handler relation may be fabricated.
func TestPyFastAPIRoutes_AnonymousHandlerSymbolOnly(t *testing.T) {
	src := `from fastapi import FastAPI

app = FastAPI()

@app.get("/weird")
class NotAFunction:
    pass
`
	syms, rels := extractPyRoutes(t, src)
	if len(syms) != 1 {
		t.Fatalf("want 1 route symbol (registration is real), got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "GET /weird" {
		t.Errorf("symbol name: want %q, got %q", "GET /weird", syms[0].Name)
	}
	if len(rels) != 0 {
		t.Fatalf("non-function handler must NOT produce a route relation; got %+v", rels)
	}
}

// TestPyFastAPIRoutes_DynamicPathEmitsNothing: any non-literal first
// positional path argument — f-string with interpolation holes, a
// bare identifier, or a missing positional — emits NOTHING for that
// registration (never fabricate paths).
func TestPyFastAPIRoutes_DynamicPathEmitsNothing(t *testing.T) {
	src := `from fastapi import FastAPI

app = FastAPI()
BASE = "/computed"

@app.get(f"/v1/{BASE}")
def f_string_hole():
    return {}

@app.get(BASE)
def identifier_path():
    return {}

@app.get("/lit" + BASE)
def concatenated_path():
    return {}

@app.get(response_model=None)
def keyword_only():
    return {}
`
	syms, rels := extractPyRoutes(t, src)
	if len(syms) != 0 {
		t.Errorf("dynamic paths must emit no route symbols; got %+v", syms)
	}
	if len(rels) != 0 {
		t.Errorf("dynamic paths must emit no route relations; got %+v", rels)
	}
}

// TestPyFastAPIRoutes_PrefixComposition pins FastAPI's prefix flavor:
// APIRouter mount prefixes are composed cross-file by include_router,
// so APIRouter-bound decorators emit the LOCAL path tagged
// partial_path=true; FastAPI-bound decorators are complete; receivers
// matching neither tracked assignment still emit (the import gate
// already proved the framework) and are tagged partial_path=true.
func TestPyFastAPIRoutes_PrefixComposition(t *testing.T) {
	src := `from fastapi import FastAPI, APIRouter

app = FastAPI()
router = APIRouter(prefix="/api")

@app.get("/health")
def health():
    return {}

@router.get("/items")
def list_items():
    return []

@mystery.delete("/ghosts")
def evict_ghost():
    return {}
`
	syms, rels := extractPyRoutes(t, src)
	if len(syms) != 3 {
		t.Fatalf("want 3 route symbols, got %d: %+v", len(syms), syms)
	}
	if len(rels) != 3 {
		t.Fatalf("want 3 route relations, got %d: %+v", len(rels), rels)
	}
	byPath := map[string]types.Relation{}
	for _, r := range rels {
		byPath[r.Metadata["path"]] = r
	}

	if r, ok := byPath["/health"]; !ok {
		t.Errorf("missing /health relation")
	} else if _, has := r.Metadata["partial_path"]; has {
		t.Errorf("FastAPI-bound /health must not be partial; got %+v", r.Metadata)
	}

	if r, ok := byPath["/items"]; !ok {
		t.Errorf("missing /items relation (LOCAL path, not the router prefix composition)")
	} else {
		if r.Metadata["partial_path"] != "true" {
			t.Errorf("APIRouter-bound /items must carry partial_path=true; got %+v", r.Metadata)
		}
		if r.FromEP.Name != "GET /items" {
			t.Errorf("router route name must use the LOCAL path: want %q, got %q", "GET /items", r.FromEP.Name)
		}
	}

	if r, ok := byPath["/ghosts"]; !ok {
		t.Errorf("unknown receiver must still emit (import gate proved fastapi)")
	} else {
		if r.Metadata["partial_path"] != "true" {
			t.Errorf("unknown receiver must carry partial_path=true; got %+v", r.Metadata)
		}
		if r.Metadata["method"] != "DELETE" {
			t.Errorf("method: want DELETE, got %q", r.Metadata["method"])
		}
	}
}

// TestPyFastAPIRoutes_NotImportedEmitsNothing is the negative gate:
// route-LIKE decorator calls (Bottle exposes the same .get/.post
// decorator shape) in a file that does NOT import fastapi must emit
// nothing — the import gate is the only thing allowed to enable the
// pass, never receiver-name heuristics. (The fixture moved from Flask
// to Bottle when the Flask pass shipped: a Flask fixture now emits
// routes BY DESIGN through the flask import gate.)
func TestPyFastAPIRoutes_NotImportedEmitsNothing(t *testing.T) {
	src := `from bottle import Bottle

app = Bottle()

@app.get("/users/<user_id>")
def read_user(user_id):
    return {}

@app.post("/items")
def create_item():
    return {}
`
	syms, rels := extractPyRoutes(t, src)
	if len(syms) != 0 {
		t.Errorf("no fastapi import: want no route symbols, got %+v", syms)
	}
	if len(rels) != 0 {
		t.Errorf("no fastapi import: want no route relations, got %+v", rels)
	}
}

// TestPyFastAPIRoutes_StackedAndAsyncDecorators pins two adjacent
// behaviors: (1) async handlers are plain function_definitions and
// resolve normally; (2) several HTTP-method decorators stacked on one
// handler each register their own route, and non-route decorators in
// the stack are ignored.
func TestPyFastAPIRoutes_StackedAndAsyncDecorators(t *testing.T) {
	src := `import fastapi

app = fastapi.FastAPI()

@app.get("/multi")
@app.head("/multi")
@functools.cache
async def multi():
    return {}
`
	syms, rels := extractPyRoutes(t, src)
	if len(syms) != 2 || len(rels) != 2 {
		t.Fatalf("want 2 route symbols + 2 relations (GET+HEAD), got %d/%d: %+v / %+v",
			len(syms), len(rels), syms, rels)
	}
	verbs := map[string]bool{}
	for _, r := range rels {
		verbs[r.Metadata["method"]] = true
		if r.ToEP.Name != "multi" {
			t.Errorf("ToEP.Name: want multi, got %q", r.ToEP.Name)
		}
		if _, has := r.Metadata["partial_path"]; has {
			t.Errorf("fastapi.FastAPI()-bound app must not be partial; got %+v", r.Metadata)
		}
	}
	if !verbs["GET"] || !verbs["HEAD"] {
		t.Errorf("want GET+HEAD verbs, got %+v", verbs)
	}
}

// ---------------------------------------------------------------------------
// Flask section
// ---------------------------------------------------------------------------

// extractPyFlaskRoutes runs the full Python extractor and filters down
// to the Flask lane: Kind=route symbols and flask_route_literal
// relations. (Symbols carry no ResolvedBy; in flask-only fixtures the
// fastapi gate is closed, so every route symbol is the Flask pass's.)
func extractPyFlaskRoutes(t *testing.T, src string) ([]types.Symbol, []types.Relation) {
	t.Helper()
	b := []byte(src)
	root := parsePythonForRoutes(t, b)
	_, syms, _, rels := extractPython(root, b, "app.py")
	var routeSyms []types.Symbol
	for _, s := range syms {
		if s.Kind == "route" {
			routeSyms = append(routeSyms, s)
		}
	}
	var routeRels []types.Relation
	for _, r := range rels {
		if r.ResolvedBy == pyResolvedByFlask {
			routeRels = append(routeRels, r)
		}
	}
	return routeSyms, routeRels
}

// TestPyFlaskRoutes_SimpleRouteDefaultGET pins the canonical Flask
// surface: @app.route with NO methods kwarg defaults to GET only
// (Flask's documented add_url_rule default; HEAD/OPTIONS are implicit
// framework additions and must NOT be emitted), with the full shared
// symbol contract and one relation carrying the construction-stage
// diagnostics plus framework metadata, no partial_path.
func TestPyFlaskRoutes_SimpleRouteDefaultGET(t *testing.T) {
	src := `from flask import Flask

app = Flask(__name__)

@app.route("/users")
def list_users():
    return []
`
	syms, rels := extractPyFlaskRoutes(t, src)
	if len(syms) != 1 {
		t.Fatalf("want exactly 1 route symbol (GET only — no implicit HEAD/OPTIONS), got %d: %+v", len(syms), syms)
	}
	s := syms[0]
	if s.Name != "GET /users" {
		t.Errorf("symbol name: want %q, got %q", "GET /users", s.Name)
	}
	if s.Kind != "route" {
		t.Errorf("symbol kind: want route, got %q", s.Kind)
	}
	if s.Line != 5 || s.EndLine != 5 {
		t.Errorf("symbol line: want decorator line 5/5, got %d/%d", s.Line, s.EndLine)
	}
	if !s.Exported {
		t.Errorf("route symbol must be Exported")
	}
	if s.Parent != "" {
		t.Errorf("route symbol Parent MUST stay empty (file_map renders Parent.Name); got %q", s.Parent)
	}
	if want := "GET /users -> list_users"; s.Signature != want {
		t.Errorf("signature: want %q, got %q", want, s.Signature)
	}

	if len(rels) != 1 {
		t.Fatalf("want 1 route relation, got %d: %+v", len(rels), rels)
	}
	r := rels[0]
	if r.Kind != "reference" {
		t.Errorf("relation kind: want reference, got %q", r.Kind)
	}
	if r.FromEP.Name != s.Name || r.FromEP.File != "app.py" || r.FromEP.Line != 5 {
		t.Errorf("FromEP: want {%q app.py 5}, got %+v", s.Name, r.FromEP)
	}
	if r.ToEP.Name != "list_users" || r.ToEP.Receiver != "" || r.ToEP.Line != 5 {
		t.Errorf("ToEP: want {list_users <no receiver> line 5}, got %+v", r.ToEP)
	}
	if r.Confidence != types.ConfidenceRouteLiteral {
		t.Errorf("confidence: want ConfidenceRouteLiteral, got %v", r.Confidence)
	}
	if r.Provenance != types.ProvenanceRouteResolver {
		t.Errorf("provenance: want ProvenanceRouteResolver, got %q", r.Provenance)
	}
	if r.ResolvedBy != "flask_route_literal" {
		t.Errorf("resolved_by: want flask_route_literal, got %q", r.ResolvedBy)
	}
	if r.Metadata["framework"] != "flask" || r.Metadata["method"] != "GET" || r.Metadata["path"] != "/users" {
		t.Errorf("metadata: want framework/method/path, got %+v", r.Metadata)
	}
	if _, has := r.Metadata["partial_path"]; has {
		t.Errorf("Flask-bound app path is complete — partial_path must be absent; got %+v", r.Metadata)
	}
}

// TestPyFlaskRoutes_MethodsKwargOneRoutePerVerb: a literal
// methods=[...] list emits one Symbol+Relation PER string-literal
// verb; tuple flavor and lower-case literals are upper-cased the way
// werkzeug's Rule stores methods.
func TestPyFlaskRoutes_MethodsKwargOneRoutePerVerb(t *testing.T) {
	src := `from flask import Flask

app = Flask(__name__)

@app.route("/items", methods=["GET", "POST"])
def items():
    return []

@app.route("/legacy", methods=("delete",))
def drop_legacy():
    return {}
`
	syms, rels := extractPyFlaskRoutes(t, src)
	if len(syms) != 3 || len(rels) != 3 {
		t.Fatalf("want 3 route symbols + 3 relations (GET+POST+DELETE), got %d/%d: %+v / %+v",
			len(syms), len(rels), syms, rels)
	}
	byName := map[string]types.Relation{}
	for _, r := range rels {
		byName[r.FromEP.Name] = r
	}
	for name, line := range map[string]int{"GET /items": 5, "POST /items": 5, "DELETE /legacy": 9} {
		r, ok := byName[name]
		if !ok {
			t.Errorf("missing relation %q; got %+v", name, byName)
			continue
		}
		if r.Line != line {
			t.Errorf("%s: want registration line %d, got %d", name, line, r.Line)
		}
	}
	if byName["DELETE /legacy"].Metadata["method"] != "DELETE" {
		t.Errorf("lower-case tuple literal must be upper-cased to DELETE; got %+v", byName["DELETE /legacy"].Metadata)
	}
	if byName["DELETE /legacy"].ToEP.Name != "drop_legacy" {
		t.Errorf("ToEP.Name: want drop_legacy, got %q", byName["DELETE /legacy"].ToEP.Name)
	}
}

// TestPyFlaskRoutes_ShortcutDecorators covers the Flask 2.x shortcut
// table: .get/.post/.put/.delete/.patch resolve; .head does NOT —
// Flask's Scaffold defines no head/options/trace shortcuts, so a
// @app.head call must emit nothing (verb table comes from the
// framework API surface, never from fixtures).
func TestPyFlaskRoutes_ShortcutDecorators(t *testing.T) {
	src := `from flask import Flask

app = Flask(__name__)

@app.get("/ping")
def ping():
    return "pong"

@app.patch("/users/<int:user_id>")
def update_user(user_id):
    return {}

@app.head("/nope")
def not_a_flask_shortcut():
    return ""
`
	syms, rels := extractPyFlaskRoutes(t, src)
	if len(syms) != 2 || len(rels) != 2 {
		t.Fatalf("want 2 route symbols + 2 relations (GET+PATCH; head is not a Flask shortcut), got %d/%d: %+v / %+v",
			len(syms), len(rels), syms, rels)
	}
	verbs := map[string]bool{}
	for _, r := range rels {
		verbs[r.Metadata["method"]] = true
	}
	if !verbs["GET"] || !verbs["PATCH"] || verbs["HEAD"] {
		t.Errorf("want GET+PATCH and no HEAD, got %+v", verbs)
	}
}

// TestPyFlaskRoutes_MethodHandlerCarriesClassReceiver covers the
// selector/attribute-handler flavor for the Flask pass: a decorated
// method inside a class stamps the enclosing class name into
// ToEP.Receiver, and a blueprint with a LITERAL url_prefix composes
// the full path per-file (no partial_path).
func TestPyFlaskRoutes_MethodHandlerCarriesClassReceiver(t *testing.T) {
	src := `from flask import Blueprint

bp = Blueprint("items", __name__, url_prefix="/api")

class ItemView:
    @bp.route("/items", methods=["POST"])
    def create_item(self):
        return {}
`
	syms, rels := extractPyFlaskRoutes(t, src)
	if len(syms) != 1 {
		t.Fatalf("want 1 route symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "POST /api/items" {
		t.Errorf("symbol name: want joined %q, got %q", "POST /api/items", syms[0].Name)
	}
	if len(rels) != 1 {
		t.Fatalf("want 1 route relation, got %d: %+v", len(rels), rels)
	}
	r := rels[0]
	if r.ToEP.Name != "create_item" {
		t.Errorf("ToEP.Name: want create_item, got %q", r.ToEP.Name)
	}
	if r.ToEP.Receiver != "ItemView" {
		t.Errorf("ToEP.Receiver: want enclosing class ItemView, got %q", r.ToEP.Receiver)
	}
	if r.Metadata["path"] != "/api/items" {
		t.Errorf("metadata path: want joined /api/items, got %q", r.Metadata["path"])
	}
	if _, has := r.Metadata["partial_path"]; has {
		t.Errorf("literal url_prefix composes per-file — partial_path must be absent; got %+v", r.Metadata)
	}
}

// TestPyFlaskRoutes_AnonymousHandlerSymbolOnly: a decorated
// NON-function (here a class — not a Flask view function) keeps the
// route Symbol — the registration is real — but NO route->handler
// relation may be fabricated.
func TestPyFlaskRoutes_AnonymousHandlerSymbolOnly(t *testing.T) {
	src := `from flask import Flask

app = Flask(__name__)

@app.route("/weird")
class NotAFunction:
    pass
`
	syms, rels := extractPyFlaskRoutes(t, src)
	if len(syms) != 1 {
		t.Fatalf("want 1 route symbol (registration is real), got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "GET /weird" {
		t.Errorf("symbol name: want %q, got %q", "GET /weird", syms[0].Name)
	}
	if len(rels) != 0 {
		t.Fatalf("non-function handler must NOT produce a route relation; got %+v", rels)
	}
}

// TestPyFlaskRoutes_DynamicPathEmitsNothing: any non-literal first
// positional path argument — f-string with interpolation holes, a
// bare identifier, concatenation, or a missing positional — emits
// NOTHING for that registration (never fabricate paths).
func TestPyFlaskRoutes_DynamicPathEmitsNothing(t *testing.T) {
	src := `from flask import Flask

app = Flask(__name__)
BASE = "/computed"

@app.route(f"/v1/{BASE}")
def f_string_hole():
    return {}

@app.get(BASE)
def identifier_path():
    return {}

@app.route("/lit" + BASE)
def concatenated_path():
    return {}

@app.route(methods=["GET"])
def keyword_only():
    return {}
`
	syms, rels := extractPyFlaskRoutes(t, src)
	if len(syms) != 0 {
		t.Errorf("dynamic paths must emit no route symbols; got %+v", syms)
	}
	if len(rels) != 0 {
		t.Errorf("dynamic paths must emit no route relations; got %+v", rels)
	}
}

// TestPyFlaskRoutes_NonLiteralMethodsEmitsNothing: a methods kwarg
// that is not a list/tuple/set of plain string literals — identifier,
// comprehension, or a single dynamic element — emits NOTHING for that
// registration (verbs are never fabricated).
func TestPyFlaskRoutes_NonLiteralMethodsEmitsNothing(t *testing.T) {
	src := `from flask import Flask

app = Flask(__name__)
VERBS = ["GET", "POST"]

@app.route("/var", methods=VERBS)
def variable_methods():
    return {}

@app.route("/comp", methods=[m for m in VERBS])
def comprehension_methods():
    return {}

@app.route("/mixed", methods=["GET", VERBS[0]])
def mixed_methods():
    return {}
`
	syms, rels := extractPyFlaskRoutes(t, src)
	if len(syms) != 0 {
		t.Errorf("non-literal methods kwarg must emit no route symbols; got %+v", syms)
	}
	if len(rels) != 0 {
		t.Errorf("non-literal methods kwarg must emit no route relations; got %+v", rels)
	}
}

// TestPyFlaskRoutes_BlueprintPrefixComposition pins Flask's prefix
// flavor: a Blueprint with a LITERAL url_prefix joins per-file the
// way BlueprintSetupState.add_url_rule does (rstrip/lstrip "/"); a
// Blueprint with a non-literal OR absent url_prefix (register_
// blueprint may supply one cross-file) emits the LOCAL path tagged
// partial_path=true; Flask-bound paths are complete; receivers
// matching no tracked assignment still emit (the import gate already
// proved the framework) and are tagged partial_path=true.
func TestPyFlaskRoutes_BlueprintPrefixComposition(t *testing.T) {
	src := `from flask import Flask, Blueprint

app = Flask(__name__)
api = Blueprint("api", __name__, url_prefix="/api/")
floating = Blueprint("floating", __name__)
dyn = Blueprint("dyn", __name__, url_prefix=PREFIX)

@app.route("/health")
def health():
    return {}

@api.get("/items")
def list_items():
    return []

@floating.route("/orphans")
def orphans():
    return []

@dyn.route("/ghosts")
def ghosts():
    return []

@mystery.delete("/spooky")
def spooky():
    return {}
`
	syms, rels := extractPyFlaskRoutes(t, src)
	if len(syms) != 5 {
		t.Fatalf("want 5 route symbols, got %d: %+v", len(syms), syms)
	}
	if len(rels) != 5 {
		t.Fatalf("want 5 route relations, got %d: %+v", len(rels), rels)
	}
	byPath := map[string]types.Relation{}
	for _, r := range rels {
		byPath[r.Metadata["path"]] = r
	}

	if r, ok := byPath["/health"]; !ok {
		t.Errorf("missing /health relation")
	} else if _, has := r.Metadata["partial_path"]; has {
		t.Errorf("Flask-bound /health must not be partial; got %+v", r.Metadata)
	}

	if r, ok := byPath["/api/items"]; !ok {
		t.Errorf("missing /api/items relation (literal url_prefix must join rstrip/lstrip-style)")
	} else {
		if _, has := r.Metadata["partial_path"]; has {
			t.Errorf("literal-prefix blueprint /api/items must not be partial; got %+v", r.Metadata)
		}
		if r.FromEP.Name != "GET /api/items" {
			t.Errorf("blueprint route name must use the JOINED path: want %q, got %q", "GET /api/items", r.FromEP.Name)
		}
	}

	if r, ok := byPath["/orphans"]; !ok {
		t.Errorf("missing /orphans relation (LOCAL path — absent url_prefix)")
	} else if r.Metadata["partial_path"] != "true" {
		t.Errorf("prefixless blueprint must carry partial_path=true; got %+v", r.Metadata)
	}

	if r, ok := byPath["/ghosts"]; !ok {
		t.Errorf("missing /ghosts relation (LOCAL path — non-literal url_prefix)")
	} else if r.Metadata["partial_path"] != "true" {
		t.Errorf("non-literal url_prefix must carry partial_path=true; got %+v", r.Metadata)
	}

	if r, ok := byPath["/spooky"]; !ok {
		t.Errorf("unknown receiver must still emit (import gate proved flask)")
	} else {
		if r.Metadata["partial_path"] != "true" {
			t.Errorf("unknown receiver must carry partial_path=true; got %+v", r.Metadata)
		}
		if r.Metadata["method"] != "DELETE" {
			t.Errorf("method: want DELETE, got %q", r.Metadata["method"])
		}
	}
}

// TestPyFlaskRoutes_NotImportedEmitsNothing is the negative gate:
// route-LIKE decorator calls (Bottle exposes the same @app.route /
// @app.get decorator shape) in a file that imports NEITHER flask nor
// fastapi must emit nothing — the import gate is the only thing
// allowed to enable a pass, never receiver-name heuristics.
func TestPyFlaskRoutes_NotImportedEmitsNothing(t *testing.T) {
	src := `from bottle import Bottle

app = Bottle()

@app.route("/users", methods=["GET"])
def list_users():
    return []

@app.get("/ping")
def ping():
    return "pong"
`
	syms, rels := extractPyFlaskRoutes(t, src)
	if len(syms) != 0 {
		t.Errorf("no flask import: want no route symbols, got %+v", syms)
	}
	if len(rels) != 0 {
		t.Errorf("no flask import: want no route relations, got %+v", rels)
	}
}

// TestPyRoutes_DualImportDedupe: a file importing BOTH fastapi and
// flask runs both passes. Flask 2.x shares the @x.get decorator shape
// with FastAPI, so one physical registration matches both passes —
// dedupe by (verb, path, line) must keep exactly one emission, with
// the FastAPI pass winning ties (its behavior stays byte-identical).
// Registrations only one pass recognises (@x.route is not a FastAPI
// form) survive untouched.
func TestPyRoutes_DualImportDedupe(t *testing.T) {
	src := `from fastapi import FastAPI
from flask import Flask

app = FastAPI()
legacy = Flask(__name__)

@app.get("/dual")
def dual():
    return {}

@legacy.route("/migrate", methods=["POST"])
def migrate():
    return {}
`
	b := []byte(src)
	root := parsePythonForRoutes(t, b)
	_, syms, _, rels := extractPython(root, b, "app.py")
	var routeSyms []types.Symbol
	for _, s := range syms {
		if s.Kind == "route" {
			routeSyms = append(routeSyms, s)
		}
	}
	var routeRels []types.Relation
	for _, r := range rels {
		if r.Provenance == types.ProvenanceRouteResolver {
			routeRels = append(routeRels, r)
		}
	}

	if len(routeSyms) != 2 {
		t.Fatalf("want exactly 2 route symbols (GET /dual deduped to one), got %d: %+v", len(routeSyms), routeSyms)
	}
	names := map[string]int{}
	for _, s := range routeSyms {
		names[s.Name]++
	}
	if names["GET /dual"] != 1 || names["POST /migrate"] != 1 {
		t.Errorf("want one GET /dual + one POST /migrate symbol, got %+v", names)
	}

	if len(routeRels) != 2 {
		t.Fatalf("want exactly 2 route relations, got %d: %+v", len(routeRels), routeRels)
	}
	byName := map[string]types.Relation{}
	for _, r := range routeRels {
		byName[r.FromEP.Name] = r
	}
	if r, ok := byName["GET /dual"]; !ok {
		t.Errorf("missing GET /dual relation")
	} else {
		if r.ResolvedBy != pyResolvedByFastAPI {
			t.Errorf("dedupe tie must keep the FastAPI emission: want %q, got %q", pyResolvedByFastAPI, r.ResolvedBy)
		}
		if r.Metadata["framework"] != "fastapi" {
			t.Errorf("GET /dual framework: want fastapi, got %q", r.Metadata["framework"])
		}
	}
	if r, ok := byName["POST /migrate"]; !ok {
		t.Errorf("missing POST /migrate relation")
	} else {
		if r.ResolvedBy != pyResolvedByFlask {
			t.Errorf("POST /migrate: want %q, got %q", pyResolvedByFlask, r.ResolvedBy)
		}
		if _, has := r.Metadata["partial_path"]; has {
			t.Errorf("Flask-bound /migrate must not be partial; got %+v", r.Metadata)
		}
	}
}
