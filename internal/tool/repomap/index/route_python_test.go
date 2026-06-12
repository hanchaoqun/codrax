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
// route-LIKE decorator calls (Flask 2.x exposes the same .get/.post
// decorator shape) in a file that does NOT import fastapi must emit
// nothing — the import gate is the only thing allowed to enable the
// pass, never receiver-name heuristics.
func TestPyFastAPIRoutes_NotImportedEmitsNothing(t *testing.T) {
	src := `from flask import Flask

app = Flask(__name__)

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
