package types

import (
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
// shapes that can mint a hint value: composite literals (`RequiredFileHint{`),
// `new(RequiredFileHint)`, zero-value `var` declarations, and `make` with a
// non-zero length (which mints zero-valued elements). Whole-struct copies
// (`out = append(out, hint)`, `h.Path = canon`) carry the origin and are not
// producers.

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
func requiredFileHintOriginCensus(relPath, src string) ([]requiredFileHintProducerSite, int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, relPath, src, 0)
	if err != nil {
		return nil, 0, err
	}
	var offenders []requiredFileHintProducerSite
	producers := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		key := relPath + "::" + fn.Name.Name
		report := func(n ast.Node, reason string) {
			producers++
			if requiredFileHintModelDecoders[key] {
				return
			}
			offenders = append(offenders, requiredFileHintProducerSite{
				position: fset.Position(n.Pos()).String(), function: fn.Name.Name, reason: reason,
			})
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CompositeLit:
				if !requiredFileHintTypeExpr(x.Type) {
					return true
				}
				origin, ok := requiredFileHintLiteralOrigin(x)
				switch {
				case !ok:
					report(x, "literal without an Origin key")
				case !requiredFileHintSystemOrigins[origin]:
					producers++
					offenders = append(offenders, requiredFileHintProducerSite{
						position: fset.Position(x.Pos()).String(), function: fn.Name.Name,
						reason: "literal stamps a non-system Origin " + origin,
					})
				default:
					producers++
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
			}
			return true
		})
	}
	return offenders, producers, nil
}

func requiredFileHintTypeExpr(expr ast.Expr) bool {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name == "RequiredFileHint"
	case *ast.SelectorExpr:
		pkg, ok := x.X.(*ast.Ident)
		return ok && pkg.Name == "types" && x.Sel.Name == "RequiredFileHint"
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
	// The walk covers every Go file in the repository (dot-directories,
	// vendor and testdata fixtures excluded): a producer in eval/ or main.go
	// would be as invisible to the marker as one in internal/.
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
			if !strings.Contains(string(body), "RequiredFileHint") {
				return nil
			}
			sites, n, cerr := requiredFileHintOriginCensus(rel, string(body))
			if cerr != nil {
				return cerr
			}
			producers += n
			for key := range requiredFileHintModelDecoders {
				if strings.HasPrefix(key, rel+"::") && strings.Contains(string(body), "func "+strings.TrimPrefix(key, rel+"::")+"(") {
					seenDecoder[key] = true
				}
			}
			for _, s := range sites {
				offenders = append(offenders, s.position+" in "+s.function+": "+s.reason)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("census walk failed (a silent green would defeat the tripwire): %v", err)
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
	}
	for name, body := range shapes {
		sites, _, err := requiredFileHintOriginCensus("internal/tool/foo.go", "package tool\nimport \"github.com/hanchaoqun/codrax/internal/types\"\n"+body+"\n")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(sites) == 0 {
			t.Fatalf("self-red %s: census must report the unregistered producer", name)
		}
	}
	// Self-green control: a system-stamped literal and a zero-length make are not offenders.
	for name, body := range map[string]string{
		"system literal":   "func projectFoo() types.RequiredFileHint { return types.RequiredFileHint{Path: \"x\", Origin: types.RequiredFileHintOriginAnalyzerPrescan} }",
		"zero-length make": "func projectFoo() []types.RequiredFileHint { return make([]types.RequiredFileHint, 0, 4) }",
	} {
		sites, _, err := requiredFileHintOriginCensus("internal/tool/foo.go", "package tool\nimport \"github.com/hanchaoqun/codrax/internal/types\"\n"+body+"\n")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(sites) != 0 {
			t.Fatalf("control %s: census must stay silent, got %+v", name, sites)
		}
	}
}
