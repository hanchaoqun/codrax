package tool

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The identity disclosure needs both the concrete active plan and the report
// installed at this exit, unlike the existing report-only audit addends. Keep
// that exact context-aware enrichment inside the one installation choke point.
func TestB1788NativeTestIdentityInstalledExitAndMutation(t *testing.T) {
	source, err := os.ReadFile("run_tests.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(string) string
		want bool
	}{
		{"production", func(s string) string { return s }, true},
		{"omitted", func(s string) string {
			return strings.Replace(s, "types.RenderCurrentNativeTestIdentitySnapshot(plan.ID, report)", "unrelated(plan.ID, report)", 1)
		}, false},
		{"wrong_report", func(s string) string {
			return strings.Replace(s, "types.RenderCurrentNativeTestIdentitySnapshot(plan.ID, report)", "types.RenderCurrentNativeTestIdentitySnapshot(plan.ID, other)", 1)
		}, false},
		{"missing_active_plan", func(s string) string {
			return strings.Replace(s, "types.RenderCurrentNativeTestIdentitySnapshot(plan.ID, report)", "types.RenderCurrentNativeTestIdentitySnapshot(report.PlanID, report)", 1)
		}, false},
		{"uninstalled_report", func(s string) string {
			return strings.Replace(s, "installRunTestsReport(ctx, report, dryRunProbe)", "installRunTestsReport(ctx, other, dryRunProbe)", 1)
		}, false},
		{"before_install", func(s string) string {
			s = strings.Replace(s, "\t\tinstallRunTestsReport(ctx, report, dryRunProbe)\n", "", 1)
			return strings.Replace(s, "\t\treturn base + renderRunTestsWorktreeAuditSummary(report)", "\t\tinstallRunTestsReport(ctx, report, dryRunProbe)\n\t\treturn base + renderRunTestsWorktreeAuditSummary(report)", 1)
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := b1788IdentityInstallShape(t, tc.edit(string(source))); got != tc.want {
				t.Fatalf("identity installation shape=%t want=%t", got, tc.want)
			}
		})
	}
}

func b1788IdentityInstallShape(t *testing.T, source string) bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "run_tests.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	text := func(node ast.Node) string {
		var b bytes.Buffer
		if err := format.Node(&b, fset, node); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	var choke *ast.FuncLit
	ast.Inspect(file, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if ok && len(assign.Lhs) == 1 && text(assign.Lhs[0]) == "installFinishedReport" && len(assign.Rhs) == 1 {
			choke, _ = assign.Rhs[0].(*ast.FuncLit)
		}
		return true
	})
	if choke == nil {
		return false
	}
	var installed, rendered token.Pos
	valid, renderCount := false, 0
	ast.Inspect(choke.Body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok && text(call) == "installRunTestsReport(ctx, report, dryRunProbe)" {
			installed = call.Pos()
		}
		if branch, ok := node.(*ast.IfStmt); ok && branch.Init != nil && text(branch.Init) == "plan := ctx.Mutable.ChangePlan()" && text(branch.Cond) == "plan != nil" {
			ast.Inspect(branch.Body, func(child ast.Node) bool {
				if call, ok := child.(*ast.CallExpr); ok && text(call) == "types.RenderCurrentNativeTestIdentitySnapshot(plan.ID, report)" {
					valid = true
				}
				return true
			})
		}
		return true
	})
	ast.Inspect(file, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok && text(call.Fun) == "types.RenderCurrentNativeTestIdentitySnapshot" {
			renderCount++
			rendered = call.Pos()
		}
		return true
	})
	return valid && renderCount == 1 && installed != token.NoPos && installed < rendered && rendered < choke.End()
}
