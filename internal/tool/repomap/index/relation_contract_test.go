package index

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRelationConstructionSitesCarryDiagnostics is the primary
// defense for the schema-v4 relation contract (the load-side
// presence validation in validateCachedFileInfos is only the
// backstop): every durable types.Relation composite literal in this
// package's production code must set Confidence, Provenance and
// ResolvedBy at the construction site. A writer regression fails here
// at `make test` instead of surfacing as a silent full-rescan loop in
// production (one invalid edge rejects the whole cache).
//
// Static source audit by design — precise, exhaustive over
// construction sites, and independent of per-language fixture
// fragility (静态可审计 red line).
func TestRelationConstructionSitesCarryDiagnostics(t *testing.T) {
	required := []string{"Confidence", "Provenance", "ResolvedBy"}
	// The relations.md sidecar writer synthesizes transient
	// import rows purely for the greppable sidecar; they never
	// enter FileInfo.Relations or the cache payload.
	exemptFiles := map[string]bool{"cache.go": true}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	audited := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Relation" {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "types" {
				return true
			}
			audited++
			if exemptFiles[name] {
				return true
			}
			fields := map[string]bool{}
			for _, elt := range lit.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if key, ok := kv.Key.(*ast.Ident); ok {
						fields[key.Name] = true
					}
				}
			}
			pos := fset.Position(lit.Pos())
			for _, want := range required {
				if !fields[want] {
					t.Errorf("%s: types.Relation literal missing %s — every durable relation must carry construction-stage diagnostics", pos, want)
				}
			}
			if !fields["ToEP"] {
				t.Errorf("%s: types.Relation literal missing ToEP — relations carry structured endpoints only", pos)
			}
			return true
		})
	}
	if audited < 20 {
		t.Fatalf("audited only %d Relation literals — the scan is broken (expected the ~30 extractor sites)", audited)
	}

	// Cross-check the sole exempt file really only holds the
	// transient sidecar literal, so the exemption cannot silently
	// widen.
	raw, err := os.ReadFile(filepath.Join(".", "cache.go"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), "types.Relation{"); n != 1 {
		t.Errorf("cache.go now has %d types.Relation literals (expected exactly 1, the transient sidecar import row) — re-audit the exemption", n)
	}
}
