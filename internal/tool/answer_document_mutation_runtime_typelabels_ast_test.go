package tool

// answer_document_mutation_runtime_typelabels_ast_test.go — RULE3-1 双复核修复
// F4 (2026-07-21): the zh and EN verdict/class word tables
// (runtimeTraceRootCauseTypeZHLabel / runtimeTraceRootCauseTypeENLabel) must
// enumerate the SAME case set. The existing weight-universe pin
// (TestRootCauseTypeZHLabelCoversWeightUniverse) covers only tokens reachable
// from the tracequery ranking switch — a token added to one table outside that
// universe (e.g. a display-only alias) could silently gain a zh word while its
// EN face regresses to snake_case, or vice versa. This pin extracts both
// switches' case sets via go/ast (tool-package lint-test precedent:
// trace_query_dominant_state_pin_test.go) and asserts set equality.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// typelabelsASTCaseSet extracts the string case constants of the top-level
// switch inside the named function of typelabels.go.
func typelabelsASTCaseSet(t *testing.T, funcName string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "answer_document_mutation_runtime_typelabels.go", nil, 0)
	if err != nil {
		t.Fatalf("parse typelabels.go: %v", err)
	}
	out := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != funcName || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			clause, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expr := range clause.List {
				lit, ok := expr.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("%s: unquote %s: %v", funcName, lit.Value, err)
				}
				out[value] = true
			}
			return true
		})
		return out
	}
	t.Fatalf("function %s not found in typelabels.go", funcName)
	return nil
}

func TestRootCauseTypeLabelTablesEnumerateEqualCaseSets(t *testing.T) {
	zh := typelabelsASTCaseSet(t, "runtimeTraceRootCauseTypeZHLabel")
	en := typelabelsASTCaseSet(t, "runtimeTraceRootCauseTypeENLabel")
	if len(zh) == 0 || len(en) == 0 {
		t.Fatalf("case extraction looks broken: zh=%d en=%d", len(zh), len(en))
	}
	var missing []string
	for tok := range zh {
		if !en[tok] {
			missing = append(missing, tok+" (zh-only)")
		}
	}
	for tok := range en {
		if !zh[tok] {
			missing = append(missing, tok+" (en-only)")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("F4: the zh/EN verdict-word tables diverge — one-table tokens: %s", strings.Join(missing, ", "))
	}
}
