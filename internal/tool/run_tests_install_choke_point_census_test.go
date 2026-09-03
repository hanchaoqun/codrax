package tool

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// run_tests_install_choke_point_census_test.go — fold-in round four of V5-2
// (colleague_merge_audit §40.36 四轮收编, finding N): the round-three pin
// counted the raw substring "installRunTestsReport(ctx, " and a literal
// "return base + …" line, so an exit written as a bare
// `installFinishedReport(report, base)` statement followed by
// `Summary: base` (or a differently spaced `installRunTestsReport( ctx,`
// call) installed the report and dropped the audit sentence while the pin
// stayed green. This census reads the package through go/ast and binds by
// data flow:
//   - every call of installRunTestsReport in the package's non-test files
//     is lexically inside the FuncLit bound to `installFinishedReport`, and
//     that FuncLit returns `base + renderRunTestsWorktreeAuditSummary(report)`
//     (the bound identifiers, not a text match);
//   - in every function or closure that calls installFinishedReport, every
//     types.ToolResult composite literal carries a Summary whose expression
//     is that call itself or an identifier defined from that call — later
//     `+=` appends are fine, any other assignment to the identifier, a bare
//     call statement whose result is discarded, or a Summary fed from
//     anything else is red.

type installChokePointFinding struct {
	pos  string
	text string
}

type installChokePointResult struct {
	violations   []installChokePointFinding
	chokePoints  int // FuncLits bound to installFinishedReport
	installCalls int // installRunTestsReport calls found
	guarded      int // ToolResult literals proven to use the choke point
}

func (r *installChokePointResult) violate(fset *token.FileSet, node ast.Node, text string) {
	r.violations = append(r.violations, installChokePointFinding{pos: fset.Position(node.Pos()).String(), text: text})
}

func isCallTo(expr ast.Expr, name string) (*ast.CallExpr, bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	ident, ok := call.Fun.(*ast.Ident)
	if !ok || ident.Name != name {
		return nil, false
	}
	return call, true
}

func isToolResultType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "types" && sel.Sel.Name == "ToolResult"
}

// installChokePointCensus analyses the given files as one package.
func installChokePointCensus(fset *token.FileSet, files []*ast.File, result *installChokePointResult) {
	// Pass 1: locate every FuncLit bound to `installFinishedReport` and
	// check its body: exactly one installRunTestsReport call and a return
	// of `base + renderRunTestsWorktreeAuditSummary(report)` where base and
	// report are its parameters.
	chokeRanges := map[*ast.FuncLit]bool{}
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
				return true
			}
			ident, ok := assign.Lhs[0].(*ast.Ident)
			if !ok || ident.Name != "installFinishedReport" {
				return true
			}
			lit, ok := assign.Rhs[0].(*ast.FuncLit)
			if !ok {
				result.violate(fset, assign, "installFinishedReport must be bound to a function literal")
				return true
			}
			chokeRanges[lit] = true
			result.chokePoints++
			var params []string
			for _, field := range lit.Type.Params.List {
				for _, name := range field.Names {
					params = append(params, name.Name)
				}
			}
			if len(params) != 2 {
				result.violate(fset, lit, "installFinishedReport must take (report, base)")
				return true
			}
			reportParam, baseParam := params[0], params[1]
			returns := 0
			ast.Inspect(lit.Body, func(n ast.Node) bool {
				ret, ok := n.(*ast.ReturnStmt)
				if !ok {
					return true
				}
				returns++
				if len(ret.Results) != 1 {
					result.violate(fset, ret, "the choke point returns exactly one summary")
					return true
				}
				bin, ok := ret.Results[0].(*ast.BinaryExpr)
				if !ok || bin.Op != token.ADD {
					result.violate(fset, ret, "the choke point must return base + renderRunTestsWorktreeAuditSummary(report)")
					return true
				}
				left, ok := bin.X.(*ast.Ident)
				call, isRender := isCallTo(bin.Y, "renderRunTestsWorktreeAuditSummary")
				if !ok || left.Name != baseParam || !isRender || len(call.Args) != 1 {
					result.violate(fset, ret, "the choke point must return base + renderRunTestsWorktreeAuditSummary(report)")
					return true
				}
				if arg, ok := call.Args[0].(*ast.Ident); !ok || arg.Name != reportParam {
					result.violate(fset, ret, "the audit sentence must be rendered from the installed report")
				}
				return true
			})
			if returns != 1 {
				result.violate(fset, lit, "the choke point has exactly one return")
			}
			return true
		})
	}
	// Pass 2: every installRunTestsReport call site is inside a choke point.
	for _, file := range files {
		var stack []*ast.FuncLit
		var visit func(n ast.Node) bool
		visit = func(n ast.Node) bool {
			if n == nil {
				return false
			}
			if lit, ok := n.(*ast.FuncLit); ok {
				stack = append(stack, lit)
				ast.Inspect(lit.Body, visit)
				stack = stack[:len(stack)-1]
				return false
			}
			expr, isExpr := n.(ast.Expr)
			if !isExpr {
				return true
			}
			if call, ok := isCallTo(expr, "installRunTestsReport"); ok {
				result.installCalls++
				inside := false
				for _, lit := range stack {
					if chokeRanges[lit] {
						inside = true
					}
				}
				if !inside {
					result.violate(fset, call, "installRunTestsReport is called outside the installFinishedReport choke point")
				}
			}
			return true
		}
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil || fd.Name.Name == "installRunTestsReport" {
				continue
			}
			ast.Inspect(fd.Body, visit)
		}
	}
	// Pass 3: in every body that calls installFinishedReport, bind each
	// ToolResult literal's Summary to that call by data flow.
	for _, file := range files {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			installChokePointBodies(fset, fd.Body, result)
		}
	}
}

// installChokePointBodies analyses one body (recursing into closures, each
// as its own body) for the Summary data-flow rule.
func installChokePointBodies(fset *token.FileSet, body *ast.BlockStmt, result *installChokePointResult) {
	// Closures are separate bodies.
	ast.Inspect(body, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok {
			installChokePointBodies(fset, lit.Body, result)
			return false
		}
		return true
	})
	// Statements of this body only (closures excluded).
	callsChoke := false
	bound := map[string]bool{}      // identifiers defined from the choke point call
	poisoned := map[string]string{} // identifiers reassigned from something else
	var literals []*ast.CompositeLit
	var bareCalls []*ast.CallExpr
	ast.Inspect(body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ExprStmt:
			if call, ok := isCallTo(v.X, "installFinishedReport"); ok {
				callsChoke = true
				bareCalls = append(bareCalls, call)
			}
		case *ast.AssignStmt:
			for i, lhs := range v.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok {
					continue
				}
				var rhs ast.Expr
				if len(v.Rhs) == len(v.Lhs) {
					rhs = v.Rhs[i]
				}
				if rhs != nil {
					if _, ok := isCallTo(rhs, "installFinishedReport"); ok && (v.Tok == token.DEFINE || v.Tok == token.ASSIGN) {
						callsChoke = true
						bound[ident.Name] = true
						delete(poisoned, ident.Name)
						continue
					}
				}
				if bound[ident.Name] {
					if v.Tok == token.ADD_ASSIGN {
						continue // appending to the choke-point summary is fine
					}
					poisoned[ident.Name] = fset.Position(v.Pos()).String()
				}
			}
		case *ast.CompositeLit:
			if isToolResultType(v.Type) {
				literals = append(literals, v)
			}
		case *ast.CallExpr:
			if _, ok := isCallTo(v, "installFinishedReport"); ok {
				callsChoke = true
			}
		}
		return true
	})
	if !callsChoke {
		return
	}
	for _, call := range bareCalls {
		result.violate(fset, call, "installFinishedReport result discarded: the exit summary must be the choke point's result")
	}
	for _, lit := range literals {
		var summary ast.Expr
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Summary" {
				summary = kv.Value
			}
		}
		if summary == nil {
			result.violate(fset, lit, "types.ToolResult literal without a Summary in a body that installs a finished report")
			continue
		}
		if _, ok := isCallTo(summary, "installFinishedReport"); ok {
			result.guarded++
			continue
		}
		ident, ok := summary.(*ast.Ident)
		if !ok || !bound[ident.Name] {
			result.violate(fset, summary, "types.ToolResult.Summary is not the installFinishedReport result (bind it: summary := installFinishedReport(report, base))")
			continue
		}
		if pos, bad := poisoned[ident.Name]; bad {
			result.violate(fset, summary, "types.ToolResult.Summary identifier "+ident.Name+" was reassigned from something other than the choke point at "+pos)
			continue
		}
		result.guarded++
	}
}

func TestRunTestsInstallsFinishedReportsThroughOneChokePoint(t *testing.T) {
	fset, files := parseToolPackageNonTestFiles(t, ".")
	result := &installChokePointResult{}
	installChokePointCensus(fset, files, result)
	for _, v := range result.violations {
		t.Errorf("%s: %s", v.pos, v.text)
	}
	if result.chokePoints != 1 {
		t.Errorf("exactly one installFinishedReport choke point expected, found %d", result.chokePoints)
	}
	if result.installCalls != 1 {
		t.Errorf("installRunTestsReport must be called exactly once (inside the choke point), found %d", result.installCalls)
	}
	if result.guarded < 14 {
		t.Errorf("expected the 14 install exits of Execute to be bound to the choke point, proved %d", result.guarded)
	}
}

func chokePointSelfRed(t *testing.T, src string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "self_red.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	result := &installChokePointResult{}
	installChokePointCensus(fset, []*ast.File{file}, result)
	sort.Slice(result.violations, func(i, j int) bool { return result.violations[i].pos < result.violations[j].pos })
	var texts []string
	for _, v := range result.violations {
		texts = append(texts, v.text)
	}
	return texts
}

const chokePointPrelude = `package tool

import "github.com/hanchaoqun/codrax/internal/types"

func installRunTestsReport(ctx *types.BusContext, report *types.ChangeReport, dryRunProbe bool) {}
func renderRunTestsWorktreeAuditSummary(report *types.ChangeReport) string { return "" }
`

const chokePointDefinition = `
	installFinishedReport := func(report *types.ChangeReport, base string) string {
		installRunTestsReport(ctx, report, dryRunProbe)
		return base + renderRunTestsWorktreeAuditSummary(report)
	}
`

// Self-red: the bare-call-plus-Summary-base shape, the differently spaced
// install call outside the choke point, a reassigned summary identifier, a
// Summary fed from elsewhere, and a choke point that drops the audit
// sentence are all red; the bound shapes (call in place, identifier with
// `+=` appends) stay green.
func TestRunTestsInstallChokePointCensusSelfRed(t *testing.T) {
	expect := func(shape, src, want string) {
		t.Helper()
		t.Run(shape, func(t *testing.T) {
			texts := chokePointSelfRed(t, src)
			for _, text := range texts {
				if strings.Contains(text, want) {
					return
				}
			}
			t.Fatalf("shape %q escaped (want %q); violations=%v", shape, want, texts)
		})
	}
	expect("bare_call_then_summary_base", chokePointPrelude+`
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	installFinishedReport(report, base)
	return types.ToolResult{Summary: base}
}
`, "installFinishedReport result discarded")
	expect("bare_call_then_summary_base_summary_side", chokePointPrelude+`
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	installFinishedReport(report, base)
	return types.ToolResult{Summary: base}
}
`, "Summary is not the installFinishedReport result")
	expect("differently_spaced_install_outside_choke_point", chokePointPrelude+`
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	installRunTestsReport( ctx,
		report, dryRunProbe )
	return types.ToolResult{Summary: installFinishedReport(report, base)}
}
`, "called outside the installFinishedReport choke point")
	expect("summary_identifier_reassigned", chokePointPrelude+`
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	summary := installFinishedReport(report, base)
	summary = base
	return types.ToolResult{Summary: summary}
}
`, "was reassigned from something other than the choke point")
	expect("choke_point_drops_audit_sentence", chokePointPrelude+`
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {
	installFinishedReport := func(report *types.ChangeReport, base string) string {
		installRunTestsReport(ctx, report, dryRunProbe)
		return base
	}
	return types.ToolResult{Summary: installFinishedReport(report, base)}
}
`, "must return base + renderRunTestsWorktreeAuditSummary(report)")
	expect("tool_result_without_summary", chokePointPrelude+`
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	_ = installFinishedReport(report, base)
	return types.ToolResult{Success: true}
}
`, "without a Summary")
	t.Run("bound_shapes_stay_green", func(t *testing.T) {
		texts := chokePointSelfRed(t, chokePointPrelude+`
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	if base == "" {
		return types.ToolResult{Summary: installFinishedReport(report, "x")}
	}
	summary := installFinishedReport(report, base)
	if report != nil {
		summary += "\nProbe output"
	}
	go func() {
		summary := installFinishedReport(report, "closure")
		_ = types.ToolResult{Summary: summary}
	}()
	return types.ToolResult{Summary: summary}
}
func unrelated() types.ToolResult { return types.ToolResult{Summary: "no report installed here"} }
`)
		if len(texts) != 0 {
			t.Fatalf("bound shapes must be green: %v", texts)
		}
	})
}
