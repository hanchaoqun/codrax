package types

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// required_file_hint_origin_census_test.go — §40.47 fold-in (A0) structural
// tripwire for the RequiredFileHint producer class.
//
// Rule: the unresolved-owner marker describes only what the MODEL declared,
// and it tells model-declared hints from deterministic system projections by
// the typed Origin field alone. So every producer of a RequiredFileHint value
// in the repository must either be the registered model decoder or stamp a
// non-model Origin constant. The census is total over the construction
// shapes that can mint a hint value: composite literals (`RequiredFileHint{`
// spelled with its type OR as an elided-type element of a slice/array/map/
// pointer-element literal, at function or package level), `new(RequiredFileHint)`,
// zero-value `var` declarations (function-local and package-level), `make`
// with a non-zero length (which mints zero-valued elements), named function
// results of the hint type (a zero value on entry), and struct types carrying
// a value-typed RequiredFileHint field (every instantiation mints a zero
// hint). A write to `.Origin` on a hint-typed value RE-mints its provenance,
// so it is a producer too: it must stamp a system origin constant. Whole-
// struct copies (`out = append(out, hint)`, `h.Path = canon`) carry the
// origin and are not producers.

// requiredFileHintModelDecoders is the single declared registry of functions
// allowed to construct an origin-less (model-declared) hint.
var requiredFileHintModelDecoders = map[string]bool{
	"internal/tool/emit_analysis.go::validateAndBuildRequiredFileHintsWithContext": true,
}

// requiredFileHintSystemOrigins is the set of Origin constant names a system
// producer may stamp; RequiredFileHintOriginModel is deliberately absent.
var requiredFileHintSystemOrigins = map[string]bool{
	"RequiredFileHintOriginRuntimeArtifactPath":     true,
	"RequiredFileHintOriginAnalyzerPrescan":         true,
	"RequiredFileHintOriginPrincipalScopePromotion": true,
	"RequiredFileHintOriginUserPinnedPath":          true,
}

type requiredFileHintProducerSite struct {
	position string
	function string
	reason   string
}

// requiredFileHintOriginCensus reports every hint producer in src that is
// neither a registered model decoder nor stamps a system origin. relPath is
// the repo-relative path used for registry keys.
// requiredFileHintAliasNames is the module-wide alias / defined-type name
// set over RequiredFileHint (§40.43 round-six #9), collected by
// requiredFileHintCollectTypeNames before the census runs and consulted by
// requiredFileHintTypeExpr so an alias-spelled mint (`type myHint =
// types.RequiredFileHint; myHint{...}`) is judged like the direct spelling.
// Package-test-local state: the census is single-threaded.
var requiredFileHintAliasNames map[string]bool

// requiredFileHintCollectTypeNames walks every parsed file for TypeSpecs
// over the hint type, to a fixpoint. Recognized shapes: a plain
// alias/defined type (Ident / types.Selector / parenthesised). A struct or
// interface carrying the hint is a container the census's own field lanes
// judge. EVERY OTHER type declaration mentioning the hint type (a named
// slice, pointer, map, …) FAILS LOUD — a shape the walker cannot classify
// ends the enumeration instead of minting invisibly.
func requiredFileHintCollectTypeNames(files map[string]string) (map[string]bool, error) {
	names := map[string]bool{}
	parsed := map[string]*ast.File{}
	fset := token.NewFileSet()
	for rel, src := range files {
		f, err := parser.ParseFile(fset, rel, src, 0)
		if err != nil {
			return nil, err
		}
		parsed[rel] = f
	}
	recognized := func(expr ast.Expr) bool {
		for {
			if paren, ok := expr.(*ast.ParenExpr); ok {
				expr = paren.X
				continue
			}
			break
		}
		switch x := expr.(type) {
		case *ast.Ident:
			return x.Name == "RequiredFileHint" || names[x.Name]
		case *ast.SelectorExpr:
			pkg, ok := x.X.(*ast.Ident)
			return ok && pkg.Name == "types" && (x.Sel.Name == "RequiredFileHint" || names[x.Sel.Name])
		}
		return false
	}
	for changed := true; changed; {
		changed = false
		for _, f := range parsed {
			ast.Inspect(f, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok || ts.Name.Name == "RequiredFileHint" || names[ts.Name.Name] {
					return true
				}
				if recognized(ts.Type) {
					names[ts.Name.Name] = true
					changed = true
				}
				return true
			})
		}
	}
	var unrecognized error
	for rel, f := range parsed {
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || ts.Name.Name == "RequiredFileHint" || names[ts.Name.Name] || unrecognized != nil {
				return true
			}
			switch ts.Type.(type) {
			case *ast.StructType, *ast.InterfaceType:
				return true // container shapes: the census's own field lanes judge them
			}
			mentions := false
			ast.Inspect(ts.Type, func(m ast.Node) bool {
				if id, ok := m.(*ast.Ident); ok && (id.Name == "RequiredFileHint" || names[id.Name]) {
					mentions = true
				}
				return !mentions
			})
			if mentions {
				unrecognized = fmt.Errorf("%s declares type %s over RequiredFileHint in a shape the origin census cannot classify — use []types.RequiredFileHint / *types.RequiredFileHint directly or extend the census", rel, ts.Name.Name)
			}
			return true
		})
	}
	return names, unrecognized
}

func requiredFileHintOriginCensus(relPath, src string, aliasNames map[string]bool) ([]requiredFileHintProducerSite, int, error) {
	requiredFileHintAliasNames = aliasNames
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, relPath, src, 0)
	if err != nil {
		return nil, 0, err
	}
	var offenders []requiredFileHintProducerSite
	producers := 0
	reportIn := func(fnName string, exempt bool) func(n ast.Node, reason string) {
		return func(n ast.Node, reason string) {
			producers++
			if exempt {
				return
			}
			offenders = append(offenders, requiredFileHintProducerSite{
				position: fset.Position(n.Pos()).String(), function: fnName, reason: reason,
			})
		}
	}
	// judgeLit judges one RequiredFileHint composite literal (explicit or
	// elided type). A missing Origin key mints a model declaration (only the
	// registered decoder may); a non-system Origin constant is red anywhere.
	judgeLit := func(lit *ast.CompositeLit, fnName string, report func(ast.Node, string)) {
		origin, ok := requiredFileHintLiteralOrigin(lit)
		switch {
		case !ok:
			report(lit, "literal without an Origin key")
		case !requiredFileHintSystemOrigins[origin]:
			producers++
			offenders = append(offenders, requiredFileHintProducerSite{
				position: fset.Position(lit.Pos()).String(), function: fnName,
				reason: "literal stamps a non-system Origin " + origin,
			})
		default:
			producers++
		}
	}
	// walkLit judges lit against the type it is spelled with — or, for an
	// elided-type element literal, the element type its enclosing literal
	// implies — and recurses into elided children only (explicit-typed
	// children are judged by the AST walk itself).
	var walkLit func(lit *ast.CompositeLit, implied ast.Expr, fnName string, report func(ast.Node, string))
	walkLit = func(lit *ast.CompositeLit, implied ast.Expr, fnName string, report func(ast.Node, string)) {
		t := lit.Type
		if t == nil {
			t = implied
		}
		if star, ok := t.(*ast.StarExpr); ok {
			t = star.X // []*RequiredFileHint{{...}}: the elided element is &T{...}
		}
		if requiredFileHintTypeExpr(t) {
			judgeLit(lit, fnName, report)
			return
		}
		var elem ast.Expr
		switch x := t.(type) {
		case *ast.ArrayType:
			elem = x.Elt
		case *ast.MapType:
			elem = x.Value
		}
		if elem == nil {
			return
		}
		for _, elt := range lit.Elts {
			v := elt
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				v = kv.Value
			}
			if child, ok := v.(*ast.CompositeLit); ok && child.Type == nil {
				walkLit(child, elem, fnName, report)
			}
		}
	}
	// judgeResults reports named results of the hint type: a zero-valued hint
	// exists the moment the function is entered.
	judgeResults := func(ft *ast.FuncType, report func(ast.Node, string)) {
		if ft.Results == nil {
			return
		}
		for _, field := range ft.Results.List {
			if len(field.Names) > 0 && requiredFileHintTypeExpr(field.Type) {
				report(field, "named result of type RequiredFileHint mints an origin-less hint on entry")
			}
		}
	}
	// Struct types anywhere in the file: a value-typed hint field mints a
	// zero hint at every instantiation of the struct.
	ast.Inspect(file, func(n ast.Node) bool {
		st, ok := n.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range st.Fields.List {
			if requiredFileHintTypeExpr(field.Type) {
				reportIn("<struct type>", false)(field, "value-typed RequiredFileHint struct field mints an origin-less hint at every instantiation — use a slice/pointer or the registered decoder")
			}
		}
		return true
	})
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			report := reportIn("<package scope>", false)
			for _, spec := range d.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				if vs.Values == nil && requiredFileHintTypeExpr(vs.Type) {
					report(vs, "package-level zero-value var declaration mints an origin-less hint")
				}
				for _, v := range vs.Values {
					if lit, ok := v.(*ast.CompositeLit); ok {
						if lit.Type != nil {
							walkLit(lit, nil, "<package scope>", report)
						} else if vs.Type != nil {
							walkLit(lit, vs.Type, "<package scope>", report)
						}
					}
				}
			}
		case *ast.FuncDecl:
			if d.Body == nil {
				continue
			}
			report := reportIn(d.Name.Name, requiredFileHintModelDecoders[relPath+"::"+d.Name.Name])
			judgeResults(d.Type, report)
			hintVars, hintSlices := requiredFileHintTrackedVars(d)
			ast.Inspect(d.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CompositeLit:
					if x.Type != nil {
						walkLit(x, nil, d.Name.Name, report)
					}
				case *ast.FuncLit:
					judgeResults(x.Type, report)
				case *ast.IndexExpr:
					// §40.43 round-six #9 fail-loud lane: an explicit generic
					// instantiation at the hint type (`zero[types.RequiredFileHint]()`)
					// mints outside every tracked shape — the census cannot
					// classify it, so it is red by default.
					if requiredFileHintTypeExpr(x.Index) {
						report(x, "generic instantiation at RequiredFileHint — the census cannot track its value flow; mint through a recognized producer shape instead")
					}
				case *ast.IndexListExpr:
					for _, idx := range x.Indices {
						if requiredFileHintTypeExpr(idx) {
							report(x, "generic instantiation at RequiredFileHint — the census cannot track its value flow; mint through a recognized producer shape instead")
						}
					}
				case *ast.CallExpr:
					name := requiredFileHintCallee(x)
					if name == "new" && len(x.Args) == 1 && requiredFileHintTypeExpr(x.Args[0]) {
						report(x, "new(RequiredFileHint) mints an origin-less hint")
					}
					if name == "make" && len(x.Args) >= 2 {
						if arr, ok := x.Args[0].(*ast.ArrayType); ok && requiredFileHintTypeExpr(arr.Elt) {
							if lit, ok := x.Args[1].(*ast.BasicLit); !ok || lit.Value != "0" {
								report(x, "make([]RequiredFileHint, n>0) mints origin-less elements")
							}
						}
					}
				case *ast.ValueSpec:
					if x.Values == nil && requiredFileHintTypeExpr(x.Type) {
						report(x, "zero-value var declaration mints an origin-less hint")
					}
				case *ast.AssignStmt:
					// An Origin write on a hint-typed value RE-mints its
					// provenance: it is a producer and must stamp a system
					// origin constant.
					for i, lhs := range x.Lhs {
						sel, ok := lhs.(*ast.SelectorExpr)
						if !ok || sel.Sel.Name != "Origin" || !requiredFileHintValueExpr(sel.X, hintVars, hintSlices) {
							continue
						}
						var value ast.Expr
						switch {
						case len(x.Rhs) == len(x.Lhs):
							value = x.Rhs[i]
						case len(x.Rhs) == 1:
							value = x.Rhs[0]
						}
						origin := "<non-constant>"
						switch v := value.(type) {
						case *ast.Ident:
							origin = v.Name
						case *ast.SelectorExpr:
							origin = v.Sel.Name
						case *ast.BasicLit:
							origin = v.Value
						}
						if requiredFileHintSystemOrigins[origin] {
							producers++
							continue
						}
						report(sel, "Origin write re-mints the hint with non-system "+origin)
					}
				}
				return true
			})
		}
	}
	return offenders, producers, nil
}

// requiredFileHintTrackedVars collects, flow-insensitively, the identifiers
// inside fn that carry a RequiredFileHint (value or pointer) and the
// identifiers that carry a []RequiredFileHint, so an `.Origin` write can be
// tied to the hint type without go/types: parameters, named results, typed
// declarations, hint literals / new / tracked copies, elements of tracked
// slices (indexing and range), and the AnalyzerHints.RequiredFileHints field
// by its unique name.
func requiredFileHintTrackedVars(fn *ast.FuncDecl) (hintVars, hintSlices map[string]bool) {
	hintVars, hintSlices = map[string]bool{}, map[string]bool{}
	track := func(names []*ast.Ident, t ast.Expr) {
		if star, ok := t.(*ast.StarExpr); ok {
			t = star.X
		}
		set := hintVars
		if arr, ok := t.(*ast.ArrayType); ok {
			et := arr.Elt
			if star, ok := et.(*ast.StarExpr); ok {
				et = star.X
			}
			if !requiredFileHintTypeExpr(et) {
				return
			}
			set = hintSlices
		} else if !requiredFileHintTypeExpr(t) {
			return
		}
		for _, n := range names {
			set[n.Name] = true
		}
	}
	for _, field := range fn.Type.Params.List {
		track(field.Names, field.Type)
	}
	if fn.Type.Results != nil {
		for _, field := range fn.Type.Results.List {
			track(field.Names, field.Type)
		}
	}
	for pass := 0; pass < 4; pass++ { // fixpoint over copy chains, bounded
		before := len(hintVars) + len(hintSlices)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.ValueSpec:
				if x.Type != nil {
					track(x.Names, x.Type)
				}
			case *ast.AssignStmt:
				for i, lhs := range x.Lhs {
					id, ok := lhs.(*ast.Ident)
					if !ok {
						continue
					}
					var rhs ast.Expr
					switch {
					case len(x.Rhs) == len(x.Lhs):
						rhs = x.Rhs[i]
					case len(x.Rhs) == 1 && i == 0:
						rhs = x.Rhs[0]
					}
					if rhs == nil {
						continue
					}
					if requiredFileHintValueExpr(rhs, hintVars, hintSlices) {
						hintVars[id.Name] = true
					}
					if requiredFileHintSliceExpr(rhs, hintSlices) {
						hintSlices[id.Name] = true
					}
				}
			case *ast.RangeStmt:
				if id, ok := x.Value.(*ast.Ident); ok && requiredFileHintSliceExpr(x.X, hintSlices) {
					hintVars[id.Name] = true
				}
			}
			return true
		})
		if len(hintVars)+len(hintSlices) == before {
			break
		}
	}
	return hintVars, hintSlices
}

// requiredFileHintValueExpr reports whether e evaluates to a RequiredFileHint
// value (or pointer) under the lexical tracking rules.
func requiredFileHintValueExpr(e ast.Expr, hintVars, hintSlices map[string]bool) bool {
	switch x := e.(type) {
	case *ast.ParenExpr:
		return requiredFileHintValueExpr(x.X, hintVars, hintSlices)
	case *ast.StarExpr:
		return requiredFileHintValueExpr(x.X, hintVars, hintSlices)
	case *ast.UnaryExpr:
		return x.Op == token.AND && requiredFileHintValueExpr(x.X, hintVars, hintSlices)
	case *ast.Ident:
		return hintVars[x.Name]
	case *ast.CompositeLit:
		return requiredFileHintTypeExpr(x.Type)
	case *ast.IndexExpr:
		return requiredFileHintSliceExpr(x.X, hintSlices)
	case *ast.CallExpr:
		return requiredFileHintCallee(x) == "new" && len(x.Args) == 1 && requiredFileHintTypeExpr(x.Args[0])
	}
	return false
}

// requiredFileHintSliceExpr reports whether e evaluates to a
// []RequiredFileHint under the lexical tracking rules; the field name
// RequiredFileHints is unique to AnalyzerHints.
func requiredFileHintSliceExpr(e ast.Expr, hintSlices map[string]bool) bool {
	switch x := e.(type) {
	case *ast.ParenExpr:
		return requiredFileHintSliceExpr(x.X, hintSlices)
	case *ast.Ident:
		return hintSlices[x.Name]
	case *ast.SelectorExpr:
		return x.Sel.Name == "RequiredFileHints"
	case *ast.CompositeLit:
		if arr, ok := x.Type.(*ast.ArrayType); ok {
			return requiredFileHintTypeExpr(arr.Elt)
		}
	case *ast.CallExpr:
		name := requiredFileHintCallee(x)
		if name == "append" && len(x.Args) > 0 {
			return requiredFileHintSliceExpr(x.Args[0], hintSlices)
		}
		if name == "make" && len(x.Args) >= 2 {
			if arr, ok := x.Args[0].(*ast.ArrayType); ok {
				return requiredFileHintTypeExpr(arr.Elt)
			}
		}
	case *ast.SliceExpr:
		return requiredFileHintSliceExpr(x.X, hintSlices)
	}
	return false
}

func requiredFileHintTypeExpr(expr ast.Expr) bool {
	switch x := expr.(type) {
	case *ast.ParenExpr:
		return requiredFileHintTypeExpr(x.X)
	case *ast.Ident:
		return x.Name == "RequiredFileHint" || requiredFileHintAliasNames[x.Name]
	case *ast.SelectorExpr:
		pkg, ok := x.X.(*ast.Ident)
		return ok && pkg.Name == "types" && (x.Sel.Name == "RequiredFileHint" || requiredFileHintAliasNames[x.Sel.Name])
	}
	return false
}

func requiredFileHintLiteralOrigin(lit *ast.CompositeLit) (string, bool) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Origin" {
			continue
		}
		switch v := kv.Value.(type) {
		case *ast.Ident:
			return v.Name, true
		case *ast.SelectorExpr:
			return v.Sel.Name, true
		}
		return "<non-constant>", true
	}
	return "", false
}

func requiredFileHintCallee(call *ast.CallExpr) string {
	if id, ok := call.Fun.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func TestRequiredFileHintOriginCensus(t *testing.T) {
	for name := range requiredFileHintSystemOrigins {
		if name == "RequiredFileHintOriginModel" {
			t.Fatal("the model origin must never be registered as a system origin")
		}
	}
	repo := filepath.Join("..", "..")
	var offenders []string
	producers := 0
	seenDecoder := map[string]bool{}
	bodies := map[string]string{}
	// The walk covers every Go file in the module's source trees — root-level
	// files, internal/ and cmd/ (dot-directories, vendor, testdata and
	// sibling fixture directories such as eval/ excluded): a producer in
	// cmd/ or main.go would be as invisible to the marker as one in
	// internal/.
	{
		err := filepath.WalkDir(repo, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				name := d.Name()
				if path != repo && (strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "node_modules") {
					return filepath.SkipDir
				}
				// Only the module's source trees can hold producers the
				// marker would meet: root-level files, internal/ and cmd/.
				// Sibling directories such as eval/ hold fixture repos with
				// deliberately broken Go.
				if filepath.Dir(path) == repo && name != "internal" && name != "cmd" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, rerr := filepath.Rel(repo, path)
			if rerr != nil {
				return rerr
			}
			rel = filepath.ToSlash(rel)
			body, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			bodies[rel] = string(body)
			return nil
		})
		if err != nil {
			t.Fatalf("census walk failed (a silent green would defeat the tripwire): %v", err)
		}
	}
	// §40.43 round-six #9: collect alias / defined-type names over the hint
	// type MODULE-WIDE first (an unclassifiable declaration fails loud), so
	// a file that uses only the alias name is censused too — the old
	// substring skip never even parsed it.
	aliasNames, err := requiredFileHintCollectTypeNames(bodies)
	if err != nil {
		t.Fatalf("hint type-name collection failed loud: %v", err)
	}
	mentionsHint := func(body string) bool {
		if strings.Contains(body, "RequiredFileHint") {
			return true
		}
		for name := range aliasNames {
			if strings.Contains(body, name) {
				return true
			}
		}
		return false
	}
	for rel, body := range bodies {
		if !mentionsHint(body) {
			continue
		}
		sites, n, cerr := requiredFileHintOriginCensus(rel, body, aliasNames)
		if cerr != nil {
			t.Fatalf("census parse failed on %s: %v", rel, cerr)
		}
		producers += n
		for key := range requiredFileHintModelDecoders {
			if strings.HasPrefix(key, rel+"::") && strings.Contains(body, "func "+strings.TrimPrefix(key, rel+"::")+"(") {
				seenDecoder[key] = true
			}
		}
		for _, s := range sites {
			offenders = append(offenders, s.position+" in "+s.function+": "+s.reason)
		}
	}
	if producers < 5 {
		t.Fatalf("census saw only %d RequiredFileHint producers — it lost its subject", producers)
	}
	for key := range requiredFileHintModelDecoders {
		if !seenDecoder[key] {
			t.Fatalf("stale model-decoder registry row %q — the function no longer exists at that path", key)
		}
	}
	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Fatalf("RequiredFileHint producer(s) neither registered as the model decoder nor stamping a system Origin (§40.47 A0 — the unresolved-owner marker tells model declarations from system projections by Origin alone):\n  %s",
			strings.Join(offenders, "\n  "))
	}

	// Self-red: each construction shape in an unregistered function must be reported.
	shapes := map[string]string{
		"origin-less literal":  "func projectFoo(rm *types.RequestModel) { rm.AnalyzerHints.RequiredFileHints = append(rm.AnalyzerHints.RequiredFileHints, types.RequiredFileHint{Path: \"x\", Confidence: 0.8}) }",
		"model-origin literal": "func projectFoo() types.RequiredFileHint { return types.RequiredFileHint{Path: \"x\", Origin: types.RequiredFileHintOriginModel} }",
		"new":                  "func projectFoo() *types.RequiredFileHint { return new(types.RequiredFileHint) }",
		"zero-value var":       "func projectFoo() types.RequiredFileHint { var h types.RequiredFileHint; h.Path = \"x\"; return h }",
		"make with length":     "func projectFoo() []types.RequiredFileHint { return make([]types.RequiredFileHint, 2) }",
		// §40.47 fold-in round five: the shapes the first-round census missed.
		"elided-type element in a slice literal":     "func projectFoo(rm *types.RequestModel, token string) { rm.AnalyzerHints.RequiredFileHints = append(rm.AnalyzerHints.RequiredFileHints, []types.RequiredFileHint{{Path: token, Confidence: 0.8}}...) }",
		"elided-type element in a carrier literal":   "func projectFoo() types.AnalyzerHints { return types.AnalyzerHints{RequiredFileHints: []types.RequiredFileHint{{Path: \"x\"}}} }",
		"elided-type element in a map literal":       "func projectFoo() map[string]types.RequiredFileHint { return map[string]types.RequiredFileHint{\"a\": {Path: \"x\"}} }",
		"elided-type element under a pointer slice":  "func projectFoo() []*types.RequiredFileHint { return []*types.RequiredFileHint{{Path: \"x\"}} }",
		"origin reset on a copy":                     "func projectFoo(hint types.RequiredFileHint) types.RequiredFileHint { h := hint; h.Origin = \"\"; return h }",
		"origin reset to the model constant":         "func projectFoo(h *types.RequiredFileHint) { h.Origin = types.RequiredFileHintOriginModel }",
		"origin reset on a ranged element":           "func projectFoo(rm *types.RequestModel) { for _, h := range rm.AnalyzerHints.RequiredFileHints { h.Origin = \"\"; _ = h } }",
		"origin reset through an index write":        "func projectFoo(rm *types.RequestModel) { rm.AnalyzerHints.RequiredFileHints[0].Origin = \"\" }",
		"package-level literal":                      "var fooHint = types.RequiredFileHint{Path: \"x\"}",
		"package-level zero-value var":               "var fooHint types.RequiredFileHint",
		"package-level elided slice element literal": "var fooHints = []types.RequiredFileHint{{Path: \"x\"}}",
		"struct field zero value":                    "type fooCarrier struct {\n	Hint types.RequiredFileHint\n}",
		"helper with a named hint result":            "func projectFoo() (h types.RequiredFileHint) { h.Path = \"x\"; return }",
		"func literal with a named hint result":      "func projectFoo() { f := func() (h types.RequiredFileHint) { return }; _ = f }",
	}
	for name, body := range shapes {
		sites, _, err := requiredFileHintOriginCensus("internal/tool/foo.go", "package tool\nimport \"github.com/hanchaoqun/codrax/internal/types\"\n"+body+"\n", nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(sites) == 0 {
			t.Fatalf("self-red %s: census must report the unregistered producer", name)
		}
	}
	// §40.43 round-six #9: alias / defined-type / generic shapes. The alias
	// set is what the module-wide collector would return for the probe file
	// pair; a file using ONLY the alias name is still censused (the walk
	// keys on the collected names, not the literal substring).
	aliasProbe := map[string]string{
		"internal/tool/foo_alias.go": "package tool\nimport \"github.com/hanchaoqun/codrax/internal/types\"\ntype myHint = types.RequiredFileHint\ntype myOwnHint types.RequiredFileHint\n",
	}
	probeNames, err := requiredFileHintCollectTypeNames(aliasProbe)
	if err != nil || !probeNames["myHint"] || !probeNames["myOwnHint"] {
		t.Fatalf("collector must recognize plain alias/defined types, got %v err=%v", probeNames, err)
	}
	if _, cerr := requiredFileHintCollectTypeNames(map[string]string{
		"internal/tool/foo_bad.go": "package tool\nimport \"github.com/hanchaoqun/codrax/internal/types\"\ntype myHints []types.RequiredFileHint\n",
	}); cerr == nil {
		t.Fatal("an unclassifiable named type over the hint must fail the collection loudly")
	}
	for name, body := range map[string]string{
		"alias-typed literal mint":   "func projectFoo() myHint { return myHint{Path: \"x\"} }",
		"alias-typed zero-value var": "func projectFoo() myHint { var h myHint; h.Path = \"x\"; return h }",
		"defined-type literal mint":  "func projectFoo() myOwnHint { return myOwnHint{Path: \"x\"} }",
		"origin write through alias": "func projectFoo(h *myHint) { h.Origin = \"\" }",
		"generic zero instantiation": "func zeroFoo[T any]() T { var t T; return t }\nfunc projectFoo() types.RequiredFileHint { return zeroFoo[types.RequiredFileHint]() }",
		"generic zero at the alias":  "func zeroFoo[T any]() T { var t T; return t }\nfunc projectFoo() myHint { return zeroFoo[myHint]() }",
	} {
		sites, _, err := requiredFileHintOriginCensus("internal/tool/foo.go", "package tool\nimport \"github.com/hanchaoqun/codrax/internal/types\"\ntype myHint = types.RequiredFileHint\ntype myOwnHint types.RequiredFileHint\n"+body+"\n", probeNames)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(sites) == 0 {
			t.Fatalf("self-red %s: census must report the alias/defined/generic mint", name)
		}
	}
	// Self-green control: a system-stamped literal, a zero-length make, a
	// system re-stamp, whole-struct copies and slice-typed fields are not
	// offenders.
	for name, body := range map[string]string{
		"system literal":               "func projectFoo() types.RequiredFileHint { return types.RequiredFileHint{Path: \"x\", Origin: types.RequiredFileHintOriginAnalyzerPrescan} }",
		"zero-length make":             "func projectFoo() []types.RequiredFileHint { return make([]types.RequiredFileHint, 0, 4) }",
		"system origin re-stamp":       "func projectFoo(h *types.RequiredFileHint) { h.Origin = types.RequiredFileHintOriginUserPinnedPath }",
		"whole-struct copy":            "func projectFoo(out []types.RequiredFileHint, hint types.RequiredFileHint) []types.RequiredFileHint { hint.Path = \"x\"; return append(out, hint) }",
		"slice-typed struct field":     "type fooCarrier struct {\n	Hints []types.RequiredFileHint\n}",
		"elided system-origin element": "func projectFoo() []types.RequiredFileHint { return []types.RequiredFileHint{{Path: \"x\", Origin: types.RequiredFileHintOriginAnalyzerPrescan}} }",
	} {
		sites, _, err := requiredFileHintOriginCensus("internal/tool/foo.go", "package tool\nimport \"github.com/hanchaoqun/codrax/internal/types\"\n"+body+"\n", nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(sites) != 0 {
			t.Fatalf("control %s: census must stay silent, got %+v", name, sites)
		}
	}
}
