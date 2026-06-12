package index

import (
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// parseJSForRoutes mirrors parser.go's tree-sitter bootstrap so the
// tests can exercise extractJS directly (inline-source pattern, same
// as route_python_test.go).
func parseJSForRoutes(t *testing.T, lang string, src []byte) *sitter.Node {
	t.Helper()
	root, ok := parseTreeSitterIfPossible(lang, src)
	if !ok {
		t.Fatalf("parseTreeSitterIfPossible(%s) returned !ok — tree-sitter binding missing?", lang)
	}
	return root
}

// extractJSRouteLane runs the full JS/TS extractor and filters down to
// the route lane: Kind=route symbols and express/nestjs relations.
func extractJSRouteLane(t *testing.T, src, file string, isTS bool) ([]types.Symbol, []types.Relation) {
	t.Helper()
	lang := types.LangJavaScript
	if isTS {
		lang = types.LangTypeScript
	}
	b := []byte(src)
	root := parseJSForRoutes(t, lang, b)
	_, syms, _, rels := extractJS(root, b, file, isTS)
	var routeSyms []types.Symbol
	for _, s := range syms {
		if s.Kind == "route" {
			routeSyms = append(routeSyms, s)
		}
	}
	var routeRels []types.Relation
	for _, r := range rels {
		if r.ResolvedBy == jsExpressResolvedBy || r.ResolvedBy == jsNestResolvedBy {
			routeRels = append(routeRels, r)
		}
	}
	return routeSyms, routeRels
}

// TestJSExpressRoutes_SimpleRoute pins the canonical Express surface:
// one app-bound GET registration produces one route symbol (full
// shared contract: Name/Kind/Line/Exported/Signature/empty Parent) and
// one route->handler relation carrying the construction-stage
// diagnostics plus the framework metadata, with NO partial_path.
func TestJSExpressRoutes_SimpleRoute(t *testing.T) {
	src := `import express from 'express';

const app = express();

app.get('/users', getUsers);
`
	syms, rels := extractJSRouteLane(t, src, "app.js", false)
	if len(syms) != 1 {
		t.Fatalf("want 1 route symbol, got %d: %+v", len(syms), syms)
	}
	s := syms[0]
	if s.Name != "GET /users" {
		t.Errorf("symbol name: want %q, got %q", "GET /users", s.Name)
	}
	if s.Kind != "route" {
		t.Errorf("symbol kind: want route, got %q", s.Kind)
	}
	if s.Line != 5 || s.EndLine != 5 {
		t.Errorf("symbol line: want registration line 5/5, got %d/%d", s.Line, s.EndLine)
	}
	if !s.Exported {
		t.Errorf("route symbol must be Exported")
	}
	if s.Parent != "" {
		t.Errorf("route symbol Parent MUST stay empty (file_map renders Parent.Name); got %q", s.Parent)
	}
	if want := "GET /users -> getUsers"; s.Signature != want {
		t.Errorf("signature: want %q, got %q", want, s.Signature)
	}

	if len(rels) != 1 {
		t.Fatalf("want 1 route relation, got %d: %+v", len(rels), rels)
	}
	r := rels[0]
	if r.Kind != "reference" {
		t.Errorf("relation kind: want reference, got %q", r.Kind)
	}
	if r.FromEP.Name != s.Name || r.FromEP.File != "app.js" || r.FromEP.Line != 5 {
		t.Errorf("FromEP: want {%q app.js 5}, got %+v", s.Name, r.FromEP)
	}
	if r.ToEP.Name != "getUsers" || r.ToEP.Receiver != "" || r.ToEP.Line != 5 {
		t.Errorf("ToEP: want {getUsers <no receiver> line 5}, got %+v", r.ToEP)
	}
	if r.Confidence != types.ConfidenceRouteLiteral {
		t.Errorf("confidence: want ConfidenceRouteLiteral, got %v", r.Confidence)
	}
	if r.Provenance != types.ProvenanceRouteResolver {
		t.Errorf("provenance: want ProvenanceRouteResolver, got %q", r.Provenance)
	}
	if r.ResolvedBy != "express_route_literal" {
		t.Errorf("resolved_by: want express_route_literal, got %q", r.ResolvedBy)
	}
	if r.Metadata["framework"] != "express" || r.Metadata["method"] != "GET" || r.Metadata["path"] != "/users" {
		t.Errorf("metadata: want framework/method/path, got %+v", r.Metadata)
	}
	if _, has := r.Metadata["partial_path"]; has {
		t.Errorf("express()-bound app path is complete — partial_path must be absent; got %+v", r.Metadata)
	}
}

// TestJSExpressRoutes_MemberHandlerAndMiddleware covers the
// selector/member-expression handler flavor plus the variadic
// middleware rule: the terminal handler is the LAST argument (Express
// app.METHOD(path, callback [, callback ...]), mirroring gin).
func TestJSExpressRoutes_MemberHandlerAndMiddleware(t *testing.T) {
	src := `const express = require('express');
const app = express();

app.post('/users', auth, controllers.createUser);
`
	syms, rels := extractJSRouteLane(t, src, "app.js", false)
	if len(syms) != 1 {
		t.Fatalf("want 1 route symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "POST /users" {
		t.Errorf("symbol name: want %q, got %q", "POST /users", syms[0].Name)
	}
	if want := "POST /users -> controllers.createUser"; syms[0].Signature != want {
		t.Errorf("signature: want %q, got %q", want, syms[0].Signature)
	}
	if len(rels) != 1 {
		t.Fatalf("want 1 route relation, got %d: %+v", len(rels), rels)
	}
	r := rels[0]
	if r.ToEP.Name != "createUser" {
		t.Errorf("ToEP.Name: want terminal handler createUser (LAST arg), got %q", r.ToEP.Name)
	}
	if r.ToEP.Receiver != "controllers" {
		t.Errorf("ToEP.Receiver: want member receiver controllers, got %q", r.ToEP.Receiver)
	}
	if _, has := r.Metadata["partial_path"]; has {
		t.Errorf("require('express')-bound app is complete — partial_path must be absent; got %+v", r.Metadata)
	}
}

// TestJSExpressRoutes_AnonymousHandlerSymbolOnly: arrow-function and
// function-expression handlers register a real route, so the Symbol is
// emitted, but NO route->handler relation may be fabricated.
func TestJSExpressRoutes_AnonymousHandlerSymbolOnly(t *testing.T) {
	src := `import express from 'express';
const app = express();

app.get('/ping', (req, res) => res.send('pong'));
app.put('/legacy', function (req, res) { res.end(); });
`
	syms, rels := extractJSRouteLane(t, src, "app.js", false)
	if len(syms) != 2 {
		t.Fatalf("want 2 route symbols (registrations are real), got %d: %+v", len(syms), syms)
	}
	names := map[string]bool{}
	for _, s := range syms {
		names[s.Name] = true
	}
	if !names["GET /ping"] || !names["PUT /legacy"] {
		t.Errorf("want GET /ping + PUT /legacy symbols, got %+v", names)
	}
	if len(rels) != 0 {
		t.Fatalf("anonymous handlers must NOT produce route relations; got %+v", rels)
	}
}

// TestJSExpressRoutes_DynamicPathEmitsNothing: any non-literal path —
// identifier, concatenation, template string with a ${...} hole — and
// the one-argument application settings getter app.get(name) emit
// NOTHING (never fabricate paths).
func TestJSExpressRoutes_DynamicPathEmitsNothing(t *testing.T) {
	src := "import express from 'express';\n" +
		"const app = express();\n" +
		"const BASE = '/computed';\n" +
		"app.get(BASE, h1);\n" +
		"app.get('/v1' + BASE, h2);\n" +
		"app.get(`/v2/${BASE}`, h3);\n" +
		"app.get('title');\n"
	syms, rels := extractJSRouteLane(t, src, "app.js", false)
	if len(syms) != 0 {
		t.Errorf("dynamic paths must emit no route symbols; got %+v", syms)
	}
	if len(rels) != 0 {
		t.Errorf("dynamic paths must emit no route relations; got %+v", rels)
	}
}

// TestJSExpressRoutes_RouterPrefixFlavor pins Express's prefix flavor:
// Router()-bound registrations emit the LOCAL path tagged
// partial_path=true (app.use("/prefix", r) mounting is cross-statement
// /cross-file composition, out of scope by design); express()-bound
// registrations are complete; unresolvable receivers still emit (the
// import gate already proved the framework) tagged partial_path=true.
// All three Router construction forms open the router lane.
func TestJSExpressRoutes_RouterPrefixFlavor(t *testing.T) {
	src := `import express, { Router as MkRouter } from 'express';

const app = express();
const r1 = express.Router();
const r2 = MkRouter();

app.get('/health', health);
r1.get('/items', listItems);
r2.delete('/items/:id', removeItem);
mystery.patch('/ghosts', evictGhost);

app.use('/api', r1);
`
	syms, rels := extractJSRouteLane(t, src, "app.js", false)
	if len(syms) != 4 {
		t.Fatalf("want 4 route symbols, got %d: %+v", len(syms), syms)
	}
	if len(rels) != 4 {
		t.Fatalf("want 4 route relations, got %d: %+v", len(rels), rels)
	}
	byPath := map[string]types.Relation{}
	for _, r := range rels {
		byPath[r.Metadata["path"]] = r
	}

	if r, ok := byPath["/health"]; !ok {
		t.Errorf("missing /health relation")
	} else if _, has := r.Metadata["partial_path"]; has {
		t.Errorf("app-bound /health must not be partial; got %+v", r.Metadata)
	}

	if r, ok := byPath["/items"]; !ok {
		t.Errorf("missing /items relation (LOCAL path, not the app.use prefix composition)")
	} else {
		if r.Metadata["partial_path"] != "true" {
			t.Errorf("express.Router()-bound /items must carry partial_path=true; got %+v", r.Metadata)
		}
		if r.FromEP.Name != "GET /items" {
			t.Errorf("router route name must use the LOCAL path: want %q, got %q", "GET /items", r.FromEP.Name)
		}
	}

	if r, ok := byPath["/items/:id"]; !ok {
		t.Errorf("missing /items/:id relation (named-import Router constructor)")
	} else {
		if r.Metadata["partial_path"] != "true" {
			t.Errorf("Router()-bound route must carry partial_path=true; got %+v", r.Metadata)
		}
		if r.Metadata["method"] != "DELETE" {
			t.Errorf("method: want DELETE, got %q", r.Metadata["method"])
		}
	}

	if r, ok := byPath["/ghosts"]; !ok {
		t.Errorf("unknown receiver must still emit (import gate proved express)")
	} else if r.Metadata["partial_path"] != "true" {
		t.Errorf("unknown receiver must carry partial_path=true; got %+v", r.Metadata)
	}
}

// TestJSExpressRoutes_NotImportedEmitsNothing is the negative gate:
// route-LIKE member calls (axios and friends expose the same
// .get/.post surface) in a file that neither imports nor requires
// express must emit nothing — the import gate is the only thing
// allowed to enable the pass, never receiver-name heuristics.
func TestJSExpressRoutes_NotImportedEmitsNothing(t *testing.T) {
	src := `import axios from 'axios';

const app = makeServerishThing();

app.get('/users', getUsers);
app.post('/items', auth, createItem);
axios.delete('/remote/users', opts);
`
	syms, rels := extractJSRouteLane(t, src, "client.js", false)
	if len(syms) != 0 {
		t.Errorf("no express import: want no route symbols, got %+v", syms)
	}
	if len(rels) != 0 {
		t.Errorf("no express import: want no route relations, got %+v", rels)
	}
}

// TestJSNestRoutes_ControllerPrefixComposition pins the NestJS lane's
// canonical surface and its prefix flavor in one controller: the
// @Controller prefix composes with method decorator paths in-file
// (same-file class decorator — fully composable, NO partial_path), an
// empty @Get() maps to the class prefix as the path, and the handler
// relation stamps the class into ToEP.Receiver.
func TestJSNestRoutes_ControllerPrefixComposition(t *testing.T) {
	src := `import { Controller, Get, Post } from '@nestjs/common';

@Controller('cats')
export class CatsController {
  @Get()
  findAll(): string { return 'all'; }

  @Get('breeds')
  findBreeds(): string { return 'b'; }

  @Post('new')
  create(): string { return 'c'; }
}
`
	syms, rels := extractJSRouteLane(t, src, "cats.controller.ts", true)
	if len(syms) != 3 {
		t.Fatalf("want 3 route symbols, got %d: %+v", len(syms), syms)
	}
	if len(rels) != 3 {
		t.Fatalf("want 3 route relations, got %d: %+v", len(rels), rels)
	}
	byName := map[string]types.Symbol{}
	for _, s := range syms {
		byName[s.Name] = s
		if s.Parent != "" {
			t.Errorf("route symbol Parent MUST stay empty; got %q", s.Parent)
		}
		if !s.Exported {
			t.Errorf("route symbol must be Exported: %+v", s)
		}
	}
	if _, ok := byName["GET /cats"]; !ok {
		t.Errorf("empty @Get() must map to the class prefix as the path; got %+v", byName)
	}
	if _, ok := byName["GET /cats/breeds"]; !ok {
		t.Errorf("missing composed GET /cats/breeds; got %+v", byName)
	}
	if s, ok := byName["POST /cats/new"]; !ok {
		t.Errorf("missing composed POST /cats/new; got %+v", byName)
	} else {
		if s.Line != 11 || s.EndLine != 11 {
			t.Errorf("route line: want decorator line 11/11, got %d/%d", s.Line, s.EndLine)
		}
		if want := "POST /cats/new -> CatsController.create"; s.Signature != want {
			t.Errorf("signature: want %q, got %q", want, s.Signature)
		}
	}
	for _, r := range rels {
		if r.ResolvedBy != "nestjs_decorator" {
			t.Errorf("resolved_by: want nestjs_decorator, got %q", r.ResolvedBy)
		}
		if r.Confidence != types.ConfidenceRouteLiteral || r.Provenance != types.ProvenanceRouteResolver {
			t.Errorf("relation diagnostics: got %+v", r)
		}
		if r.ToEP.Receiver != "CatsController" {
			t.Errorf("ToEP.Receiver: want CatsController, got %q", r.ToEP.Receiver)
		}
		if r.Metadata["framework"] != "nestjs" {
			t.Errorf("metadata framework: want nestjs, got %+v", r.Metadata)
		}
		if _, has := r.Metadata["partial_path"]; has {
			t.Errorf("same-file literal controller prefix is fully composable — partial_path must be absent; got %+v", r.Metadata)
		}
	}
}

// TestJSNestRoutes_NonLiteralPaths: a non-literal METHOD decorator path
// (constant, array) emits NOTHING for that mapping; a non-literal
// CONTROLLER prefix (options object) degrades method routes to their
// LOCAL path flagged partial_path=true (mirror of the Spring pass).
func TestJSNestRoutes_NonLiteralPaths(t *testing.T) {
	src := `import { Controller, Get, Post } from '@nestjs/common';

const P = 'computed';

@Controller({ path: 'dogs' })
export class DogsController {
  @Get(P)
  dynamic(): string { return 'x'; }

  @Get(['a', 'b'])
  multi(): string { return 'y'; }

  @Post('walk')
  walk(): string { return 'z'; }
}
`
	syms, rels := extractJSRouteLane(t, src, "dogs.controller.ts", true)
	if len(syms) != 1 {
		t.Fatalf("want exactly 1 route symbol (literal @Post only), got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "POST /walk" {
		t.Errorf("non-literal prefix must degrade to LOCAL path: want %q, got %q", "POST /walk", syms[0].Name)
	}
	if len(rels) != 1 {
		t.Fatalf("want 1 route relation, got %d: %+v", len(rels), rels)
	}
	if rels[0].Metadata["partial_path"] != "true" {
		t.Errorf("non-literal controller prefix must flag partial_path=true; got %+v", rels[0].Metadata)
	}
}

// TestJSNestRoutes_NotImportedEmitsNothing is the NestJS negative
// gate: decorator shapes identical to NestJS registrations in a file
// that does not import @nestjs/common must emit nothing.
func TestJSNestRoutes_NotImportedEmitsNothing(t *testing.T) {
	src := `import { Controller, Get } from './my-own-decorators';

@Controller('cats')
export class CatsController {
  @Get('breeds')
  findBreeds(): string { return 'b'; }
}
`
	syms, rels := extractJSRouteLane(t, src, "cats.controller.ts", true)
	if len(syms) != 0 {
		t.Errorf("no @nestjs/common import: want no route symbols, got %+v", syms)
	}
	if len(rels) != 0 {
		t.Errorf("no @nestjs/common import: want no route relations, got %+v", rels)
	}
}

// TestJSNestRoutes_PlainJSGrammarNoCrash drives a decorated controller
// through the plain-JS grammar path (isTS=false). The vendored
// tree-sitter-javascript nests method decorators INSIDE the
// method_definition (unlike TS's class_body-sibling shape) — the pass
// must handle both shapes and must never crash on JS parses.
func TestJSNestRoutes_PlainJSGrammarNoCrash(t *testing.T) {
	src := `import { Controller, Get } from '@nestjs/common';

@Controller('cats')
class CatsController {
  @Get('breeds')
  findBreeds() { return 'b'; }
}
`
	syms, rels := extractJSRouteLane(t, src, "cats.controller.js", false)
	if len(syms) != 1 {
		t.Fatalf("JS-grammar decorator shape: want 1 route symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "GET /cats/breeds" {
		t.Errorf("symbol name: want %q, got %q", "GET /cats/breeds", syms[0].Name)
	}
	if len(rels) != 1 || rels[0].ToEP.Name != "findBreeds" {
		t.Fatalf("want 1 relation to findBreeds, got %+v", rels)
	}
}

// TestJSRoutes_ArkTSSourcesStayInert is the HarmonyOS hard
// deliverable: extractJS is shared with the ArkTS Tier-1 path
// (extract_arkts.go rides it with isTS=true), so an .ets-shaped source
// with ArkUI decorators plus route-LIKE member calls — driven through
// the REAL ArkTS path — must produce ZERO route symbols and ZERO
// route relations. The verbatim express/@nestjs/common import gates
// structurally cannot fire on '@ohos.*' / '@kit.*' imports.
func TestJSRoutes_ArkTSSourcesStayInert(t *testing.T) {
	src := []byte(`import router from '@ohos.router';
import { http } from '@kit.NetworkKit';

@Entry
@Component
struct Index {
  @State message: string = 'Hello'

  build() {
    Row() { Text(this.message) }
  }
}

const x = http.createHttp();
x.get("/p", fn);
x.post("/q", auth, handlers.create);
router.all("/r", cb);
`)
	_, syms, _, rels, _ := extractArkTS(src, "pages/Index.ets")
	for _, s := range syms {
		if s.Kind == "route" {
			t.Errorf("ArkTS source must produce ZERO route symbols; got %+v", s)
		}
	}
	for _, r := range rels {
		if r.ResolvedBy == jsExpressResolvedBy || r.ResolvedBy == jsNestResolvedBy {
			t.Errorf("ArkTS source must produce ZERO route relations; got %+v", r)
		}
		if r.Metadata["framework"] == "express" || r.Metadata["framework"] == "nestjs" {
			t.Errorf("ArkTS source leaked a web-framework relation; got %+v", r)
		}
	}
}
