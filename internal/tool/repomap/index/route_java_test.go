package index

import (
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// parseJava is a test helper that mirrors parser.go's tree-sitter
// bootstrap so we can exercise extractJava directly.
func parseJava(t *testing.T, src []byte) *sitter.Node {
	t.Helper()
	root, ok := parseTreeSitterIfPossible(types.LangJava, src)
	if !ok {
		t.Fatalf("parseTreeSitterIfPossible(java) returned !ok — tree-sitter-java binding missing?")
	}
	return root
}

// javaRoutesFromSource runs the full extractJava (route pass hooked at
// the end) and returns just the route symbols and the route relations.
func javaRoutesFromSource(t *testing.T, src []byte, file string) ([]types.Symbol, []types.Relation) {
	t.Helper()
	root := parseJava(t, src)
	_, syms, _, rels := extractJava(root, src, file)
	var routeSyms []types.Symbol
	for _, s := range syms {
		if s.Kind == "route" {
			routeSyms = append(routeSyms, s)
		}
	}
	var routeRels []types.Relation
	for _, r := range rels {
		if r.ResolvedBy == springRouteResolvedBy {
			routeRels = append(routeRels, r)
		}
	}
	return routeSyms, routeRels
}

// TestJavaRoutes_SimpleRoute pins the canonical single-mapping shape:
// route Symbol contract (Kind/Line/Exported/Signature/empty Parent)
// and the route→handler Relation diagnostics.
func TestJavaRoutes_SimpleRoute(t *testing.T) {
	src := []byte(`package com.example;

import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class PingController {
    @GetMapping("/ping")
    public String ping() { return "pong"; }
}
`)
	syms, rels := javaRoutesFromSource(t, src, "PingController.java")
	if len(syms) != 1 || len(rels) != 1 {
		t.Fatalf("want 1 route symbol + 1 relation, got %d/%d: %+v %+v", len(syms), len(rels), syms, rels)
	}
	s := syms[0]
	if s.Name != "GET /ping" {
		t.Errorf("symbol name: want %q, got %q", "GET /ping", s.Name)
	}
	if s.Kind != "route" || !s.Exported || s.Parent != "" {
		t.Errorf("symbol contract: want kind=route exported parent-empty, got %+v", s)
	}
	if s.Signature != "GET /ping -> PingController.ping" {
		t.Errorf("signature: got %q", s.Signature)
	}
	if s.Line != 8 || s.EndLine != 8 {
		t.Errorf("symbol line: want 8/8 (the @GetMapping annotation line), got %d/%d", s.Line, s.EndLine)
	}

	r := rels[0]
	if r.Kind != "reference" {
		t.Errorf("relation kind: want reference, got %q", r.Kind)
	}
	if r.FromEP.Name != "GET /ping" || r.FromEP.File != "PingController.java" || r.FromEP.Line != 8 {
		t.Errorf("FromEP: got %+v", r.FromEP)
	}
	if r.ToEP.Name != "ping" || r.ToEP.Receiver != "PingController" {
		t.Errorf("ToEP: want ping/PingController, got %+v", r.ToEP)
	}
	if r.Confidence != types.ConfidenceRouteLiteral {
		t.Errorf("confidence: want %v, got %v", types.ConfidenceRouteLiteral, r.Confidence)
	}
	if r.Provenance != types.ProvenanceRouteResolver {
		t.Errorf("provenance: want %q, got %q", types.ProvenanceRouteResolver, r.Provenance)
	}
	want := map[string]string{"framework": "spring", "method": "GET", "path": "/ping"}
	for k, v := range want {
		if r.Metadata[k] != v {
			t.Errorf("metadata[%s]: want %q, got %q", k, v, r.Metadata[k])
		}
	}
	if r.Metadata["partial_path"] != "" {
		t.Errorf("fully composed path must not carry partial_path, got %q", r.Metadata["partial_path"])
	}
}

// TestJavaRoutes_PrefixComposition covers Spring's prefix flavor:
// class-level @RequestMapping + every composed verb shortcut, the
// value=/path= attribute spellings, the marker form (empty local
// path → the class prefix itself), and the exactly-one-slash join for
// a trailing-slash prefix. @Controller (non-REST) also opens the gate.
func TestJavaRoutes_PrefixComposition(t *testing.T) {
	src := []byte(`package com.example;

import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/users")
public class UserController {
    @GetMapping("/{id}")
    public String get() { return "g"; }

    @PostMapping
    public String create() { return "c"; }

    @PutMapping(value = "/{id}")
    public String update() { return "u"; }

    @DeleteMapping(path = "/{id}")
    public String remove() { return "d"; }

    @PatchMapping("/{id}/status")
    public String patch() { return "p"; }
}

@Controller
@RequestMapping("/v1/")
class LegacyController {
    @GetMapping("/health")
    public String health() { return "ok"; }
}
`)
	syms, rels := javaRoutesFromSource(t, src, "UserController.java")
	wantNames := map[string]string{
		"GET /api/users/{id}":          "UserController.get",
		"POST /api/users":              "UserController.create",
		"PUT /api/users/{id}":          "UserController.update",
		"DELETE /api/users/{id}":       "UserController.remove",
		"PATCH /api/users/{id}/status": "UserController.patch",
		"GET /v1/health":               "LegacyController.health",
	}
	if len(syms) != len(wantNames) || len(rels) != len(wantNames) {
		t.Fatalf("want %d symbols + relations, got %d/%d: %+v", len(wantNames), len(syms), len(rels), syms)
	}
	for _, s := range syms {
		handler, ok := wantNames[s.Name]
		if !ok {
			t.Errorf("unexpected route symbol %q", s.Name)
			continue
		}
		if want := s.Name + " -> " + handler; s.Signature != want {
			t.Errorf("%s: signature want %q, got %q", s.Name, want, s.Signature)
		}
	}
	for _, r := range rels {
		if r.Metadata["partial_path"] != "" {
			t.Errorf("%s: composed prefix must not be partial", r.FromEP.Name)
		}
	}
}

// TestJavaRoutes_DynamicPathEmitsNothing pins the never-fabricate
// rule: constant-reference paths, ${...} property placeholders, and
// method-level @RequestMapping (non-literal verb) all emit nothing —
// no symbol, no relation.
func TestJavaRoutes_DynamicPathEmitsNothing(t *testing.T) {
	src := []byte(`package com.example;

import org.springframework.web.bind.annotation.*;

@RestController
public class DynController {
    @GetMapping(PathConstants.USERS)
    public String constantRef() { return "a"; }

    @GetMapping("${app.prefix}/users")
    public String placeholder() { return "b"; }

    @RequestMapping(method = RequestMethod.GET, path = "/manual")
    public String requestMapping() { return "c"; }
}
`)
	syms, rels := javaRoutesFromSource(t, src, "DynController.java")
	if len(syms) != 0 || len(rels) != 0 {
		t.Fatalf("dynamic/non-literal mappings must emit NOTHING; got %+v %+v", syms, rels)
	}
}

// TestJavaRoutes_PartialPrefix pins the degradation contract: a class
// prefix that exists but is not a clean literal keeps the method's
// LOCAL path and flags the relation partial_path=true.
func TestJavaRoutes_PartialPrefix(t *testing.T) {
	src := []byte(`package com.example;

import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping(ApiPaths.BASE)
public class PartialController {
    @GetMapping("/ping")
    public String ping() { return "pong"; }
}
`)
	syms, rels := javaRoutesFromSource(t, src, "PartialController.java")
	if len(syms) != 1 || len(rels) != 1 {
		t.Fatalf("want 1 symbol + 1 relation, got %d/%d", len(syms), len(rels))
	}
	if syms[0].Name != "GET /ping" {
		t.Errorf("local path: want %q, got %q", "GET /ping", syms[0].Name)
	}
	if rels[0].Metadata["partial_path"] != "true" {
		t.Errorf("unresolvable class prefix must flag partial_path=true, got %+v", rels[0].Metadata)
	}
	if rels[0].Metadata["path"] != "/ping" {
		t.Errorf("metadata path: want /ping, got %q", rels[0].Metadata["path"])
	}
}

// TestJavaRoutes_NegativeCustomRouteAnnotation is the mandatory
// negative: route-LIKE annotations on classes that do NOT carry
// @RestController/@Controller (the eval/fixtures/java-annotation-
// router shape — custom @Route(path="/echo")) emit ZERO route
// symbols/relations, even when a @GetMapping-named annotation appears
// on a non-controller class. The class annotation gate decides.
func TestJavaRoutes_NegativeCustomRouteAnnotation(t *testing.T) {
	src := []byte(`package demo;

public class EchoHandler {
    @Route(path = "/echo")
    public String echo(String msg) { return msg; }

    @Route(path = "/reverse")
    public String reverse(String msg) { return msg; }
}

class NotAController {
    @GetMapping("/looks-like-spring")
    public String nope() { return "no"; }
}
`)
	syms, rels := javaRoutesFromSource(t, src, "EchoHandler.java")
	if len(syms) != 0 || len(rels) != 0 {
		t.Fatalf("ungated classes must emit ZERO routes; got %+v %+v", syms, rels)
	}
}

// TestJavaRoutes_NoAnonymousHandlerForm documents how the shared
// contract's anonymous-handler branch (Symbol only, no Relation) maps
// onto Spring: the annotation model binds handlers to NAMED methods
// only — there is no anonymous/lambda registration surface — so every
// emitted route must carry a Relation with a named ToEP and the
// declaring class as Receiver (the MethodIndex key shape).
func TestJavaRoutes_NoAnonymousHandlerForm(t *testing.T) {
	src := []byte(`package com.example;

import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api")
public class ShapeController {
    @GetMapping("/a")
    public String a() { return "a"; }

    @PostMapping("/b")
    public String b() { return "b"; }
}
`)
	syms, rels := javaRoutesFromSource(t, src, "ShapeController.java")
	if len(syms) != 2 || len(rels) != 2 {
		t.Fatalf("want 2 symbols + 2 relations (one per mapped method), got %d/%d", len(syms), len(rels))
	}
	for _, r := range rels {
		if r.ToEP.Name == "" || r.ToEP.Receiver != "ShapeController" {
			t.Errorf("every Spring route resolves to a named method on the controller; got ToEP %+v", r.ToEP)
		}
	}
}

// TestExtractJava_InterfaceExtends pins the extends_interfaces fix:
// an interface's extends clause lives in the `extends_interfaces`
// named child (not the "type_parameters" field the old code read), so
// `interface A extends B` must yield an inheritance relation with
// ToEP.Name "B" — and generic bounds must NOT fabricate edges.
func TestExtractJava_InterfaceExtends(t *testing.T) {
	src := []byte(`package demo;

interface A extends B {}

interface Multi extends X, Y {}

interface Generic<T extends Bound> {}
`)
	root := parseJava(t, src)
	_, _, _, rels := extractJava(root, src, "ifaces.java")
	got := map[string]string{} // FromEP.Name -> comma-free set via key "from->to"
	for _, r := range rels {
		if r.ResolvedBy != "java_interface_extends" {
			continue
		}
		got[r.FromEP.Name+"->"+r.ToEP.Name] = r.Kind
	}
	for _, want := range []string{"A->B", "Multi->X", "Multi->Y"} {
		if got[want] != "inheritance" {
			t.Errorf("missing inheritance edge %s; got %v", want, got)
		}
	}
	for edge := range got {
		if edge == "Generic->Bound" {
			t.Errorf("generic bound must not fabricate an extends edge: %v", got)
		}
	}
	if len(got) != 3 {
		t.Errorf("want exactly 3 interface-extends edges, got %v", got)
	}
}
