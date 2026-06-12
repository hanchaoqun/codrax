package index

import (
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// parseGoRouteSrc mirrors parser.go's tree-sitter bootstrap so the
// tests can exercise the full extractGo path (including the route
// resolver hook) on inline sources.
func parseGoRouteSrc(t *testing.T, src []byte) *sitter.Node {
	t.Helper()
	root, ok := parseTreeSitterIfPossible(types.LangGo, src)
	if !ok {
		t.Fatalf("parseTreeSitterIfPossible(go) returned !ok — tree-sitter-go binding missing?")
	}
	return root
}

// extractGoRoutesForTest runs extractGo end-to-end (pinning the
// post-pass hook) and filters down to the route resolver's output.
func extractGoRoutesForTest(t *testing.T, src []byte) (map[string]types.Symbol, map[string]types.Relation) {
	t.Helper()
	root := parseGoRouteSrc(t, src)
	_, syms, _, rels := extractGo(root, src, "routes.go")
	routeSyms := map[string]types.Symbol{}
	for _, s := range syms {
		if s.Kind == "route" {
			routeSyms[s.Name] = s
		}
	}
	routeRels := map[string]types.Relation{}
	for _, r := range rels {
		if r.Provenance == types.ProvenanceRouteResolver {
			routeRels[r.FromEP.Name] = r
		}
	}
	return routeSyms, routeRels
}

// TestGoRoutes_GinSimple pins the full Symbol + Relation contract for
// a plain literal registration with an identifier handler.
func TestGoRoutes_GinSimple(t *testing.T) {
	src := []byte(`package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.Default()
	r.GET("/users/:id", GetUser)
}
`)
	syms, rels := extractGoRoutesForTest(t, src)
	sym, ok := syms["GET /users/:id"]
	if !ok {
		t.Fatalf("missing route symbol GET /users/:id; have %+v", syms)
	}
	if sym.Kind != "route" || !sym.Exported || sym.Parent != "" {
		t.Errorf("symbol contract violated: %+v", sym)
	}
	if sym.Signature != "GET /users/:id -> GetUser" {
		t.Errorf("signature: want %q, got %q", "GET /users/:id -> GetUser", sym.Signature)
	}
	if sym.Line == 0 || sym.EndLine != sym.Line {
		t.Errorf("line contract: want Line==EndLine>0, got Line=%d EndLine=%d", sym.Line, sym.EndLine)
	}

	rel, ok := rels["GET /users/:id"]
	if !ok {
		t.Fatalf("missing route relation; have %+v", rels)
	}
	if rel.Kind != "reference" {
		t.Errorf("relation kind: want reference, got %q", rel.Kind)
	}
	if rel.ToEP.Name != "GetUser" || rel.ToEP.Receiver != "" {
		t.Errorf("ToEP: want {GetUser, \"\"}, got %+v", rel.ToEP)
	}
	if rel.Confidence != types.ConfidenceRouteLiteral {
		t.Errorf("confidence: want %v, got %v", types.ConfidenceRouteLiteral, rel.Confidence)
	}
	if rel.ResolvedBy != "gin_route_literal" {
		t.Errorf("resolved_by: want gin_route_literal, got %q", rel.ResolvedBy)
	}
	if rel.Metadata["framework"] != "gin" || rel.Metadata["method"] != "GET" || rel.Metadata["path"] != "/users/:id" {
		t.Errorf("metadata: %+v", rel.Metadata)
	}
	if _, has := rel.Metadata["partial_path"]; has {
		t.Errorf("root-resolved registration must not be flagged partial: %+v", rel.Metadata)
	}
	if rel.FromEP.Line != sym.Line || rel.Line != sym.Line {
		t.Errorf("relation line must be the registration line %d: FromEP.Line=%d Line=%d", sym.Line, rel.FromEP.Line, rel.Line)
	}
}

// TestGoRoutes_GinSelectorHandlerAndVariadic pins selector handlers
// (ToEP carries the selector receiver) and gin's variadic form where
// the terminal handler is the LAST argument.
func TestGoRoutes_GinSelectorHandlerAndVariadic(t *testing.T) {
	src := []byte(`package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.New()
	r.POST("/users", userHandler.Create)
	r.DELETE("/users/:id", authMiddleware(), DeleteUser)
}
`)
	_, rels := extractGoRoutesForTest(t, src)
	post, ok := rels["POST /users"]
	if !ok {
		t.Fatalf("missing POST /users relation; have %+v", rels)
	}
	if post.ToEP.Name != "Create" || post.ToEP.Receiver != "userHandler" {
		t.Errorf("selector handler ToEP: want {Create, userHandler}, got %+v", post.ToEP)
	}
	del, ok := rels["DELETE /users/:id"]
	if !ok {
		t.Fatalf("missing DELETE relation (variadic last-arg handler); have %+v", rels)
	}
	if del.ToEP.Name != "DeleteUser" {
		t.Errorf("variadic: handler must be the LAST argument; got %+v", del.ToEP)
	}
}

// TestGoRoutes_GinAnonymousHandlerSymbolOnly: a func-literal handler
// still yields the route Symbol but never a Relation.
func TestGoRoutes_GinAnonymousHandlerSymbolOnly(t *testing.T) {
	src := []byte(`package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.Default()
	r.GET("/ping", func(c *gin.Context) { c.String(200, "pong") })
}
`)
	syms, rels := extractGoRoutesForTest(t, src)
	if _, ok := syms["GET /ping"]; !ok {
		t.Fatalf("anonymous handler must still produce the route symbol; have %+v", syms)
	}
	if len(rels) != 0 {
		t.Errorf("anonymous handler must not produce a relation; got %+v", rels)
	}
}

// TestGoRoutes_DynamicPathEmitsNothing: any non-literal path argument
// emits neither symbol nor relation (never fabricate paths).
func TestGoRoutes_DynamicPathEmitsNothing(t *testing.T) {
	src := []byte(`package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
)

func ginSide(somePath, suffix string) {
	r := gin.Default()
	r.GET(somePath, GetUser)
	r.POST("/a"+suffix, CreateUser)
	r.PUT(fmt.Sprintf("/%s", suffix), UpdateUser)
}

func chiSide(pattern string) {
	m := chi.NewRouter()
	m.Get(pattern, Health)
}
`)
	syms, rels := extractGoRoutesForTest(t, src)
	if len(syms) != 0 || len(rels) != 0 {
		t.Errorf("dynamic paths must emit nothing; syms=%+v rels=%+v", syms, rels)
	}
}

// TestGoRoutes_GinGroupPrefixComposition pins gin's prefix flavor:
// nested same-function Group bindings concatenate; inline Group
// chains compose; an unresolvable receiver keeps the LOCAL path with
// Metadata["partial_path"]="true"; *gin.Engine params are provably
// prefix-free roots.
func TestGoRoutes_GinGroupPrefixComposition(t *testing.T) {
	src := []byte(`package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.New()
	api := r.Group("/api")
	v1 := api.Group("/v1")
	v1.GET("/users", ListUsers)
	r.Group("/direct").POST("/items", CreateItem)
}

func Register(rg *gin.RouterGroup) {
	g := rg.Group("/sub")
	g.PUT("/thing", UpdateThing)
}

func Mount(e *gin.Engine) {
	e.GET("/health", Health)
}
`)
	syms, rels := extractGoRoutesForTest(t, src)

	rel, ok := rels["GET /api/v1/users"]
	if !ok {
		t.Fatalf("nested groups must concatenate (/api + /v1 + /users); have %+v", syms)
	}
	if rel.Metadata["partial_path"] != "" {
		t.Errorf("fully composed prefix must not be partial: %+v", rel.Metadata)
	}
	if rel.Metadata["path"] != "/api/v1/users" {
		t.Errorf("metadata path: %+v", rel.Metadata)
	}

	if _, ok := rels["POST /direct/items"]; !ok {
		t.Errorf("inline Group(...).POST chain must compose; have %+v", rels)
	}

	put, ok := rels["PUT /sub/thing"]
	if !ok {
		t.Fatalf("unresolvable group receiver must keep the LOCAL path; have %+v", rels)
	}
	if put.Metadata["partial_path"] != "true" {
		t.Errorf("unresolvable receiver must set partial_path=true: %+v", put.Metadata)
	}

	health, ok := rels["GET /health"]
	if !ok {
		t.Fatalf("missing GET /health; have %+v", rels)
	}
	if _, has := health.Metadata["partial_path"]; has {
		t.Errorf("*gin.Engine param is a provable root — no partial flag: %+v", health.Metadata)
	}
}

// TestGoRoutes_ChiBasicsAndRouteComposition pins chi's contract:
// handler is the SECOND argument, capitalized verb methods, Route /
// Group inline subrouters compose prefixes, anonymous handlers are
// symbol-only.
func TestGoRoutes_ChiBasicsAndRouteComposition(t *testing.T) {
	src := []byte(`package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func main() {
	r := chi.NewRouter()
	r.Get("/health", Health)
	r.Post("/login", auth.Login)
	r.Route("/api", func(r chi.Router) {
		r.Get("/users", ListUsers)
		r.Route("/admin", func(r chi.Router) {
			r.Delete("/users/{id}", admin.DeleteUser)
		})
	})
	r.Group(func(gr chi.Router) {
		gr.Patch("/grouped", PatchIt)
	})
	r.Get("/anon", func(w http.ResponseWriter, req *http.Request) {})
}
`)
	syms, rels := extractGoRoutesForTest(t, src)

	for _, want := range []string{"GET /health", "POST /login", "GET /api/users", "DELETE /api/admin/users/{id}", "PATCH /grouped", "GET /anon"} {
		if _, ok := syms[want]; !ok {
			t.Errorf("missing route symbol %q; have %+v", want, syms)
		}
	}
	if _, ok := rels["GET /anon"]; ok {
		t.Errorf("anonymous chi handler must not produce a relation")
	}

	get, ok := rels["GET /health"]
	if !ok {
		t.Fatalf("missing GET /health relation; have %+v", rels)
	}
	if get.ToEP.Name != "Health" || get.ResolvedBy != "chi_route_literal" || get.Metadata["framework"] != "chi" {
		t.Errorf("chi relation contract: %+v", get)
	}

	del, ok := rels["DELETE /api/admin/users/{id}"]
	if !ok {
		t.Fatalf("nested Route prefixes must compose; have %+v", rels)
	}
	if del.ToEP.Name != "DeleteUser" || del.ToEP.Receiver != "admin" {
		t.Errorf("selector handler ToEP: want {DeleteUser, admin}, got %+v", del.ToEP)
	}
	if del.Metadata["partial_path"] != "" {
		t.Errorf("root-rooted Route nesting is fully composed: %+v", del.Metadata)
	}
}

// TestGoRoutes_NotImportedEmitsNothing: route-LIKE calls without the
// framework's root import must produce nothing — the import gate is
// the PRECISE signal, receiver shapes are noise. A chi subpackage
// import (middleware) must not open the gate either.
func TestGoRoutes_NotImportedEmitsNothing(t *testing.T) {
	noFramework := []byte(`package main

import "net/http"

func main() {
	r := newRouter()
	r.GET("/users", GetUser)
	r.Get("/health", Health)
	g := r.Group("/api")
	g.POST("/x", CreateX)
	_ = http.StatusOK
}
`)
	syms, rels := extractGoRoutesForTest(t, noFramework)
	if len(syms) != 0 || len(rels) != 0 {
		t.Errorf("no framework import: want nothing, got syms=%+v rels=%+v", syms, rels)
	}

	middlewareOnly := []byte(`package main

import "github.com/go-chi/chi/v5/middleware"

func main() {
	r := newRouter()
	r.Use(middleware.Logger)
	r.Get("/health", Health)
}
`)
	syms, rels = extractGoRoutesForTest(t, middlewareOnly)
	if len(syms) != 0 || len(rels) != 0 {
		t.Errorf("chi subpackage import must not open the gate; got syms=%+v rels=%+v", syms, rels)
	}
}
