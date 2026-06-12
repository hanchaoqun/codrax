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

// TestGoRoutes_MuxSimpleMethods pins the full Symbol + Relation
// contract for a mux registration with a chained .Methods(...): one
// route PER string-literal verb, upper-cased the way Route.Methods
// upper-cases, at the REGISTRATION line (even when the chain wraps).
func TestGoRoutes_MuxSimpleMethods(t *testing.T) {
	src := []byte(`package main

import "github.com/gorilla/mux"

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/users/{id}", GetUser).Methods("GET")
	r.HandleFunc("/users", UpsertUser).Methods("POST", "put")
	r.HandleFunc("/ml", DeleteML).
		Methods("DELETE")
}
`)
	syms, rels := extractGoRoutesForTest(t, src)

	sym, ok := syms["GET /users/{id}"]
	if !ok {
		t.Fatalf("missing route symbol GET /users/{id}; have %+v", syms)
	}
	if sym.Kind != "route" || !sym.Exported || sym.Parent != "" {
		t.Errorf("symbol contract violated: %+v", sym)
	}
	if sym.Signature != "GET /users/{id} -> GetUser" {
		t.Errorf("signature: want %q, got %q", "GET /users/{id} -> GetUser", sym.Signature)
	}
	if sym.Line == 0 || sym.EndLine != sym.Line {
		t.Errorf("line contract: want Line==EndLine>0, got Line=%d EndLine=%d", sym.Line, sym.EndLine)
	}

	rel, ok := rels["GET /users/{id}"]
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
	if rel.ResolvedBy != "mux_route_literal" {
		t.Errorf("resolved_by: want mux_route_literal, got %q", rel.ResolvedBy)
	}
	if rel.Metadata["framework"] != "mux" || rel.Metadata["method"] != "GET" || rel.Metadata["path"] != "/users/{id}" {
		t.Errorf("metadata: %+v", rel.Metadata)
	}
	if _, has := rel.Metadata["partial_path"]; has {
		t.Errorf("root-resolved registration must not be flagged partial: %+v", rel.Metadata)
	}
	if rel.FromEP.Line != sym.Line || rel.Line != sym.Line {
		t.Errorf("relation line must be the registration line %d: FromEP.Line=%d Line=%d", sym.Line, rel.FromEP.Line, rel.Line)
	}

	// Multi-verb chain: one route per literal verb, upper-cased.
	for _, want := range []string{"POST /users", "PUT /users"} {
		r, ok := rels[want]
		if !ok {
			t.Fatalf("Methods(\"POST\", \"put\") must emit %q; have %+v", want, rels)
		}
		if r.ToEP.Name != "UpsertUser" {
			t.Errorf("%s ToEP: %+v", want, r.ToEP)
		}
	}
	if rels["POST /users"].Metadata["method"] != "POST" || rels["PUT /users"].Metadata["method"] != "PUT" {
		t.Errorf("per-verb metadata: %+v / %+v", rels["POST /users"].Metadata, rels["PUT /users"].Metadata)
	}

	// Wrapped chain: Line stays on the HandleFunc registration line.
	ml, ok := syms["DELETE /ml"]
	if !ok {
		t.Fatalf("wrapped .Methods chain must still resolve; have %+v", syms)
	}
	if ml.Line != 9 {
		t.Errorf("registration line: want 9 (HandleFunc line, not Methods line), got %d", ml.Line)
	}
}

// TestGoRoutes_MuxHandleShapesAndAny pins Handle's http.Handler
// shapes — identifier/selector emit the Relation, composite
// expressions are Symbol-only — and the unchained-registration "ANY"
// verb (mux matches all methods when no Methods matcher is chained).
func TestGoRoutes_MuxHandleShapesAndAny(t *testing.T) {
	src := []byte(`package main

import (
	"net/http"

	"github.com/gorilla/mux"
)

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/users", h.Create).Methods("POST")
	r.Handle("/items", itemHandler)
	r.Handle("/static", http.StripPrefix("/static", fs)).Methods("GET")
	r.Handle("/ptr", &myHandler{})
}
`)
	syms, rels := extractGoRoutesForTest(t, src)

	post, ok := rels["POST /users"]
	if !ok {
		t.Fatalf("missing POST /users; have %+v", rels)
	}
	if post.ToEP.Name != "Create" || post.ToEP.Receiver != "h" {
		t.Errorf("selector handler ToEP: want {Create, h}, got %+v", post.ToEP)
	}

	items, ok := rels["ANY /items"]
	if !ok {
		t.Fatalf("Handle without .Methods must emit verb ANY; have %+v", rels)
	}
	if items.ToEP.Name != "itemHandler" || items.Metadata["method"] != "ANY" {
		t.Errorf("ANY relation: %+v", items)
	}

	st, ok := syms["GET /static"]
	if !ok {
		t.Fatalf("composite http.Handler must still produce the route symbol; have %+v", syms)
	}
	if st.Signature != "GET /static -> func(...)" {
		t.Errorf("composite handler signature: %q", st.Signature)
	}
	if _, has := rels["GET /static"]; has {
		t.Errorf("composite http.Handler expression must not produce a relation")
	}
	if _, ok := syms["ANY /ptr"]; !ok {
		t.Fatalf("&myHandler{} must still produce the route symbol; have %+v", syms)
	}
	if _, has := rels["ANY /ptr"]; has {
		t.Errorf("composite (&myHandler{}) must not produce a relation")
	}
}

// TestGoRoutes_MuxAnonymousHandlerSymbolOnly: a func-literal handler
// still yields the route Symbol but never a Relation.
func TestGoRoutes_MuxAnonymousHandlerSymbolOnly(t *testing.T) {
	src := []byte(`package main

import (
	"net/http"

	"github.com/gorilla/mux"
)

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/ping", func(w http.ResponseWriter, req *http.Request) {}).Methods("GET")
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

// TestGoRoutes_MuxDynamicPathEmitsNothing: any non-literal path
// argument emits neither symbol nor relation (never fabricate paths).
func TestGoRoutes_MuxDynamicPathEmitsNothing(t *testing.T) {
	src := []byte(`package main

import (
	"fmt"

	"github.com/gorilla/mux"
)

func routes(pattern, suffix string) {
	r := mux.NewRouter()
	r.HandleFunc(pattern, GetUser).Methods("GET")
	r.Handle("/a"+suffix, CreateUser)
	r.HandleFunc(fmt.Sprintf("/%s", suffix), UpdateUser).Methods("PUT")
}
`)
	syms, rels := extractGoRoutesForTest(t, src)
	if len(syms) != 0 || len(rels) != 0 {
		t.Errorf("dynamic paths must emit nothing; syms=%+v rels=%+v", syms, rels)
	}
}

// TestGoRoutes_MuxNonLiteralVerbsSkipped: a variable inside
// .Methods(...) is skipped (only literal verbs emit); when EVERY verb
// is non-literal the registration emits nothing at all — verbs are
// never fabricated.
func TestGoRoutes_MuxNonLiteralVerbsSkipped(t *testing.T) {
	src := []byte(`package main

import "github.com/gorilla/mux"

func routes(verb string) {
	r := mux.NewRouter()
	r.HandleFunc("/a", AHandler).Methods("GET", verb)
	r.HandleFunc("/b", BHandler).Methods(verb)
}
`)
	syms, rels := extractGoRoutesForTest(t, src)
	if _, ok := rels["GET /a"]; !ok {
		t.Fatalf("literal verb beside a variable must still emit; have %+v", rels)
	}
	for name := range syms {
		if name != "GET /a" {
			t.Errorf("unexpected route %q (non-literal verbs must be skipped, all-non-literal registrations emit nothing); have %+v", name, syms)
		}
	}
	if len(rels) != 1 {
		t.Errorf("want exactly the GET /a relation; got %+v", rels)
	}
}

// TestGoRoutes_MuxSubrouterPrefix pins mux's prefix flavor:
// PathPrefix("<lit>").Subrouter() value bindings compose (including
// nested and inline chains); a *mux.Router parameter is NOT a
// provable root (Subrouter() returns *Router too) so its routes keep
// the LOCAL path flagged partial_path=true.
func TestGoRoutes_MuxSubrouterPrefix(t *testing.T) {
	src := []byte(`package main

import "github.com/gorilla/mux"

func main() {
	r := mux.NewRouter()
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/users", ListUsers).Methods("GET")
	v1 := api.PathPrefix("/v1").Subrouter()
	v1.Handle("/items", itemHandler)
	r.PathPrefix("/inline").Subrouter().HandleFunc("/x", XHandler).Methods("POST")
}

func Register(r *mux.Router) {
	r.HandleFunc("/sub", SubHandler).Methods("PUT")
}
`)
	syms, rels := extractGoRoutesForTest(t, src)

	get, ok := rels["GET /api/users"]
	if !ok {
		t.Fatalf("PathPrefix subrouter must compose (/api + /users); have %+v", syms)
	}
	if get.Metadata["partial_path"] != "" || get.Metadata["path"] != "/api/users" {
		t.Errorf("root-rooted subrouter is fully composed: %+v", get.Metadata)
	}

	items, ok := rels["ANY /api/v1/items"]
	if !ok {
		t.Fatalf("nested subrouters must concatenate (/api + /v1 + /items); have %+v", rels)
	}
	if items.Metadata["partial_path"] != "" {
		t.Errorf("nested root-rooted subrouter must not be partial: %+v", items.Metadata)
	}

	if _, ok := rels["POST /inline/x"]; !ok {
		t.Errorf("inline PathPrefix().Subrouter().HandleFunc chain must compose; have %+v", rels)
	}

	sub, ok := rels["PUT /sub"]
	if !ok {
		t.Fatalf("*mux.Router param must keep the LOCAL path; have %+v", rels)
	}
	if sub.Metadata["partial_path"] != "true" {
		t.Errorf("*mux.Router param is not a provable root — want partial_path=true: %+v", sub.Metadata)
	}
}

// TestGoRoutes_MuxNotImportedEmitsNothing: mux-LIKE registrations
// without the verbatim "github.com/gorilla/mux" import must produce
// nothing — neither ungated (no framework at all) nor cross-leaked
// through another framework's open gate.
func TestGoRoutes_MuxNotImportedEmitsNothing(t *testing.T) {
	noFramework := []byte(`package main

import "net/http"

func main() {
	r := newRouter()
	r.HandleFunc("/users", GetUser).Methods("GET")
	s := r.PathPrefix("/api").Subrouter()
	s.Handle("/items", itemHandler)
	_ = http.StatusOK
}
`)
	syms, rels := extractGoRoutesForTest(t, noFramework)
	if len(syms) != 0 || len(rels) != 0 {
		t.Errorf("no framework import: want nothing, got syms=%+v rels=%+v", syms, rels)
	}

	ginOnly := []byte(`package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.Default()
	r.HandleFunc("/users", GetUser).Methods("GET")
}
`)
	syms, rels = extractGoRoutesForTest(t, ginOnly)
	if len(syms) != 0 || len(rels) != 0 {
		t.Errorf("gin's gate must not admit mux-shaped registrations; got syms=%+v rels=%+v", syms, rels)
	}
}
