package index

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// kotlinRoutesFromSource runs the full extractKotlin (route pass hooked
// at the end) and returns just the route symbols and route relations,
// so every test here also exercises the extract_kotlin.go hook.
func kotlinRoutesFromSource(t *testing.T, src []byte, file string) ([]types.Symbol, []types.Relation) {
	t.Helper()
	root := parseKotlin(t, src)
	_, syms, _, rels := extractKotlin(root, src, file)
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

// TestKotlinRoutes_SimpleRoute pins the canonical single-mapping shape:
// route Symbol contract (Kind/Line/Exported/Signature/empty Parent)
// and the route→handler Relation diagnostics.
func TestKotlinRoutes_SimpleRoute(t *testing.T) {
	src := []byte(`package com.example

import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.RestController

@RestController
class PingController {
    @GetMapping("/ping")
    fun ping(): String = "pong"
}
`)
	syms, rels := kotlinRoutesFromSource(t, src, "PingController.kt")
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
	if r.FromEP.Name != "GET /ping" || r.FromEP.File != "PingController.kt" || r.FromEP.Line != 8 {
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
	if r.ResolvedBy != "spring_mapping_annotation" {
		t.Errorf("resolvedBy: want spring_mapping_annotation, got %q", r.ResolvedBy)
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

// TestKotlinRoutes_PrefixComposition covers Spring's prefix flavor in
// Kotlin: class-level @RequestMapping + every composed verb shortcut,
// the marker form (empty local path → the class prefix itself), the
// value=/path= attribute spellings — which in Kotlin REQUIRE the
// single-element collection-literal form (value = ["/x"]) — the
// exactly-one-slash join for a trailing-slash prefix, and the
// fully-qualified annotation spelling. @Controller (non-REST) also
// opens the gate.
func TestKotlinRoutes_PrefixComposition(t *testing.T) {
	src := []byte(`package com.example

import org.springframework.web.bind.annotation.*

@RestController
@RequestMapping("/api/users")
class UserController {
    @GetMapping("/{id}")
    fun get(): String = "g"

    @PostMapping
    fun create(): String = "c"

    @PutMapping(value = ["/{id}"])
    fun update(): String = "u"

    @DeleteMapping(path = ["/{id}"])
    fun remove(): String = "d"

    @PatchMapping("/{id}/status")
    fun patch(): String = "p"
}

@Controller
@RequestMapping("/v1/")
class LegacyController {
    @GetMapping("/health")
    fun health(): String = "ok"

    @org.springframework.web.bind.annotation.PostMapping("/reload")
    fun reload(): String = "r"
}
`)
	syms, rels := kotlinRoutesFromSource(t, src, "UserController.kt")
	wantNames := map[string]string{
		"GET /api/users/{id}":          "UserController.get",
		"POST /api/users":              "UserController.create",
		"PUT /api/users/{id}":          "UserController.update",
		"DELETE /api/users/{id}":       "UserController.remove",
		"PATCH /api/users/{id}/status": "UserController.patch",
		"GET /v1/health":               "LegacyController.health",
		"POST /v1/reload":              "LegacyController.reload",
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

// TestKotlinRoutes_DynamicPathEmitsNothing pins the never-fabricate
// rule across every Kotlin non-literal flavor: constant references,
// string templates WITH interpolation ($id and ${...}), the escaped
// "\${...}" Spring Environment placeholder, concatenation, multi-
// element path arrays, multiple positional vararg paths, and
// method-level @RequestMapping (non-literal verb). All emit nothing —
// no symbol, no relation.
func TestKotlinRoutes_DynamicPathEmitsNothing(t *testing.T) {
	src := []byte(`package com.example

import org.springframework.web.bind.annotation.*

@RestController
class DynController {
    @GetMapping(PathConstants.USERS)
    fun constantRef(): String = "a"

    @GetMapping("/users/$id/posts")
    fun templateIdentifier(): String = "b"

    @GetMapping("/users/${base}/posts")
    fun templateExpression(): String = "c"

    @GetMapping("\${app.prefix}/users")
    fun environmentPlaceholder(): String = "d"

    @GetMapping("/a" + SUFFIX)
    fun concatenation(): String = "e"

    @GetMapping(value = ["/a", "/b"])
    fun multiElementArray(): String = "f"

    @GetMapping("/a", "/b")
    fun multiPositional(): String = "g"

    @RequestMapping(method = [RequestMethod.GET], path = ["/manual"])
    fun requestMapping(): String = "h"
}
`)
	syms, rels := kotlinRoutesFromSource(t, src, "DynController.kt")
	if len(syms) != 0 || len(rels) != 0 {
		t.Fatalf("dynamic/non-literal mappings must emit NOTHING; got %+v %+v", syms, rels)
	}
}

// TestKotlinRoutes_PartialPrefix pins the degradation contract: a
// class prefix that exists but is not a clean literal keeps the
// method's LOCAL path and flags the relation partial_path=true.
func TestKotlinRoutes_PartialPrefix(t *testing.T) {
	src := []byte(`package com.example

import org.springframework.web.bind.annotation.*

@RestController
@RequestMapping(ApiPaths.BASE)
class PartialController {
    @GetMapping("/ping")
    fun ping(): String = "pong"
}
`)
	syms, rels := kotlinRoutesFromSource(t, src, "PartialController.kt")
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

// TestKotlinRoutes_NegativeCustomRouteAnnotation is the mandatory
// negative: route-LIKE annotations on classes that do NOT carry
// @RestController/@Controller (custom @Route(path = "/echo")) emit
// ZERO route symbols/relations, even when a @GetMapping-named
// annotation appears on a non-controller class and even though no
// Spring framework import exists. The class annotation gate decides.
func TestKotlinRoutes_NegativeCustomRouteAnnotation(t *testing.T) {
	src := []byte(`package demo

class EchoHandler {
    @Route(path = "/echo")
    fun echo(msg: String): String = msg

    @Route(path = "/reverse")
    fun reverse(msg: String): String = msg
}

class NotAController {
    @GetMapping("/looks-like-spring")
    fun nope(): String = "no"
}
`)
	syms, rels := kotlinRoutesFromSource(t, src, "EchoHandler.kt")
	if len(syms) != 0 || len(rels) != 0 {
		t.Fatalf("ungated classes must emit ZERO routes; got %+v %+v", syms, rels)
	}
}

// TestKotlinRoutes_NoAnonymousHandlerForm documents how the shared
// contract's anonymous-handler branch (Symbol only, no Relation) maps
// onto Spring in Kotlin: the annotation model binds handlers to NAMED
// functions only — annotated lambdas/properties are not a Spring
// registration surface and this pass reads function_declaration
// members exclusively — so every emitted route must carry a Relation
// with a named ToEP and the declaring class as Receiver.
func TestKotlinRoutes_NoAnonymousHandlerForm(t *testing.T) {
	src := []byte(`package com.example

import org.springframework.web.bind.annotation.*

@RestController
@RequestMapping("/api")
class ShapeController {
    @GetMapping("/a")
    fun a(): String = "a"

    @PostMapping("/b")
    fun b(): String = "b"
}
`)
	syms, rels := kotlinRoutesFromSource(t, src, "ShapeController.kt")
	if len(syms) != 2 || len(rels) != 2 {
		t.Fatalf("want 2 symbols + 2 relations (one per mapped function), got %d/%d", len(syms), len(rels))
	}
	for _, r := range rels {
		if r.ToEP.Name == "" || r.ToEP.Receiver != "ShapeController" {
			t.Errorf("every Spring route resolves to a named function on the controller; got ToEP %+v", r.ToEP)
		}
	}
}
