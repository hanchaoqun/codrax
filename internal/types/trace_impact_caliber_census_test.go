package types

// trace_impact_caliber_census_test.go — V1-1 (colleague_merge_audit §40.25,
// 2026-09-03): the public sidecar caliber tokens are a closed set owned by
// trace_root_cause_report.go (TraceImpactCaliber* / AllTraceImpactCalibers).
// Producers (the candidate compiler) and consumers (the binder, the roster
// teaching, the tool schema) spell them through the constants only — an AST
// literal scan of the production sources fails on any hand-typed token
// outside the owning file, so a third caliber can never be added as a
// literal in one place and missed by the validator, the word table or the
// teaching.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestTraceImpactCaliberTokensSpelledOnlyThroughConstants(t *testing.T) {
	tokens := map[string]bool{}
	for _, caliber := range AllTraceImpactCalibers() {
		tokens[caliber] = true
	}
	if len(tokens) < 2 {
		t.Fatalf("closed set looks broken: %v", AllTraceImpactCalibers())
	}
	fset := token.NewFileSet()
	for _, dir := range []string{".", "../analysis/tracefinding", "../agent", "../tool"} {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
				(dir == "." && filepath.Base(path) == "trace_root_cause_report.go") {
				return err
			}
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				raw, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				if tokens[raw] {
					t.Errorf("%s: hand-typed impact caliber token %q — use types.TraceImpactCaliber* (closed set owned by trace_root_cause_report.go)", fset.Position(lit.Pos()), raw)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("sweep %s: %v", dir, err)
		}
	}
}
