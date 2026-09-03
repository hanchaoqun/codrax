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
//     (the bound identifiers, not a text match); every OTHER reference to
//     the identifier — an alias binding, a function value passed around, a
//     parenthesised callee outside the choke point — is red (fold-in round
//     five, finding EE(i));
//   - in every function or closure that calls installFinishedReport, every
//     types.ToolResult composite literal carries a Summary whose expression
//     is that call itself or an identifier defined from that call — later
//     `+=` appends are fine, any other assignment to the identifier, a bare
//     call statement whose result is discarded, or a Summary fed from
//     anything else is red. Fold-in round five, finding EE(ii): a `_`
//     binding is a discard (red); the choke-point result must reach a
//     Summary sink (a ToolResult literal Summary, a compliant `.Summary =`
//     field write, or a helper builder argument) — a bound result that
//     reaches none is red; a `.Summary =` field write in a choke-calling
//     body must be fed by the choke point; and a returned helper call that
//     receives the bound summary is followed by data flow — the helper's
//     returned ToolResult Summary must be the parameter that received it.

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
	ident := calleeIdent(call)
	if ident == nil || ident.Name != name {
		return nil, false
	}
	return call, true
}

// calleeIdent unwraps a call's callee through parentheses to its identifier
// (nil for selector / literal callees) — a parenthesised callee is the same
// call (fold-in round five, finding EE(i)).
func calleeIdent(call *ast.CallExpr) *ast.Ident {
	fun := call.Fun
	for {
		if paren, ok := fun.(*ast.ParenExpr); ok {
			fun = paren.X
			continue
		}
		break
	}
	ident, _ := fun.(*ast.Ident)
	return ident
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
	// Pass 2: every installRunTestsReport call site is inside a choke
	// point, and every OTHER reference to the identifier — an alias
	// binding, a function value, an argument — is red (fold-in round five,
	// finding EE(i)): a reference that is not a direct call cannot be
	// audited, so it is never allowed.
	for _, file := range files {
		consumed := map[*ast.Ident]bool{}
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
			if call, ok := n.(*ast.CallExpr); ok {
				if ident := calleeIdent(call); ident != nil && ident.Name == "installRunTestsReport" {
					consumed[ident] = true
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
			}
			if ident, ok := n.(*ast.Ident); ok && ident.Name == "installRunTestsReport" && !consumed[ident] {
				result.violate(fset, ident, "installRunTestsReport is referenced as a value (alias / function value / argument) — only the direct call inside the installFinishedReport choke point is allowed")
			}
			return true
		}
		for _, decl := range file.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok {
				if fd.Name.Name == "installRunTestsReport" {
					continue // the declaration itself
				}
				if fd.Body == nil {
					continue
				}
				ast.Inspect(fd.Body, visit)
				continue
			}
			// Package-level declarations can alias too (var f = installRunTestsReport).
			ast.Inspect(decl, visit)
		}
	}
	// Pass 3: in every body that calls installFinishedReport, bind the
	// choke-point result to every returned ToolResult's Summary by data
	// flow.
	helpers := map[string]*ast.FuncDecl{}
	for _, file := range files {
		for _, decl := range file.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Body != nil && fd.Recv == nil {
				helpers[fd.Name.Name] = fd
			}
		}
	}
	for _, file := range files {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			installChokePointBodies(fset, fd.Body, helpers, result)
		}
	}
}

// helperSummaryParamIndex resolves a package helper by data flow (fold-in
// round five, finding EE(ii)): it returns the index of the parameter that
// feeds the Summary of every ToolResult composite literal the helper
// returns, or -1 when the helper's returned Summary cannot be proven to be
// a parameter pass-through.
func helperSummaryParamIndex(fd *ast.FuncDecl) int {
	var params []string
	if fd.Type.Params != nil {
		for _, field := range fd.Type.Params.List {
			for _, name := range field.Names {
				params = append(params, name.Name)
			}
		}
	}
	index := -1
	ok := true
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		ret, isRet := n.(*ast.ReturnStmt)
		if !isRet {
			return true
		}
		for _, res := range ret.Results {
			lit, isLit := res.(*ast.CompositeLit)
			if !isLit || !isToolResultType(lit.Type) {
				continue
			}
			var summary ast.Expr
			for _, elt := range lit.Elts {
				if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
					if key, isIdent := kv.Key.(*ast.Ident); isIdent && key.Name == "Summary" {
						summary = kv.Value
					}
				}
			}
			ident, isIdent := summary.(*ast.Ident)
			if !isIdent {
				ok = false
				continue
			}
			found := -1
			for i, name := range params {
				if name == ident.Name {
					found = i
				}
			}
			if found < 0 || (index >= 0 && index != found) {
				ok = false
				continue
			}
			index = found
		}
		return true
	})
	if !ok || index < 0 {
		return -1
	}
	return index
}

// installChokePointBodies analyses one body (recursing into closures, each
// as its own body) for the Summary data-flow rule.
func installChokePointBodies(fset *token.FileSet, body *ast.BlockStmt, helpers map[string]*ast.FuncDecl, result *installChokePointResult) {
	// Closures are separate bodies.
	ast.Inspect(body, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok {
			installChokePointBodies(fset, lit.Body, helpers, result)
			return false
		}
		return true
	})
	// Statements of this body only (closures excluded).
	callsChoke := false
	bound := map[string]bool{}      // identifiers defined from the choke point call
	sinks := map[string]int{}       // Summary sinks reached per bound identifier
	poisoned := map[string]string{} // identifiers reassigned from something else
	toolResultLocals := map[string]bool{}
	summaryFieldWritten := map[string]bool{} // locals with a compliant .Summary = write
	var literals []*ast.CompositeLit
	var bareCalls []*ast.CallExpr
	var discarded []*ast.CallExpr // `_ =` bindings (fold-in round five, EE(ii))
	var summaryWrites []*ast.AssignStmt
	var returns []*ast.ReturnStmt
	ast.Inspect(body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ExprStmt:
			if call, ok := isCallTo(v.X, "installFinishedReport"); ok {
				callsChoke = true
				bareCalls = append(bareCalls, call)
			}
		case *ast.DeclStmt:
			if gen, ok := v.Decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
				for _, spec := range gen.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok && isToolResultType(vs.Type) {
						for _, name := range vs.Names {
							toolResultLocals[name.Name] = true
						}
					}
				}
			}
		case *ast.ReturnStmt:
			returns = append(returns, v)
		case *ast.AssignStmt:
			// `.Summary =` field writes are Summary sinks and must be
			// compliant (fold-in round five, EE(ii)).
			for _, lhs := range v.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "Summary" {
					summaryWrites = append(summaryWrites, v)
				}
			}
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
					if call, ok := isCallTo(rhs, "installFinishedReport"); ok && (v.Tok == token.DEFINE || v.Tok == token.ASSIGN) {
						callsChoke = true
						if ident.Name == "_" {
							discarded = append(discarded, call)
							continue
						}
						bound[ident.Name] = true
						delete(poisoned, ident.Name)
						continue
					}
					if lit, ok := rhs.(*ast.CompositeLit); ok && isToolResultType(lit.Type) && v.Tok == token.DEFINE {
						toolResultLocals[ident.Name] = true
						for _, elt := range lit.Elts {
							if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
								if key, isIdent := kv.Key.(*ast.Ident); isIdent && key.Name == "Summary" {
									// The literal rule validates the value;
									// the local needs no field write.
									summaryFieldWritten[ident.Name] = true
								}
							}
						}
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
	for _, call := range discarded {
		result.violate(fset, call, "installFinishedReport result discarded (`_ =` binding): the exit summary must be the choke point's result")
	}
	// summaryValueCompliant checks one Summary-position expression against
	// the choke-point binding; it also counts the sink for the identifier.
	summaryValueCompliant := func(expr ast.Expr) (compliant bool, reason string) {
		if _, ok := isCallTo(expr, "installFinishedReport"); ok {
			return true, ""
		}
		ident, ok := expr.(*ast.Ident)
		if !ok || !bound[ident.Name] {
			return false, "types.ToolResult.Summary is not the installFinishedReport result (bind it: summary := installFinishedReport(report, base))"
		}
		if pos, bad := poisoned[ident.Name]; bad {
			return false, "types.ToolResult.Summary identifier " + ident.Name + " was reassigned from something other than the choke point at " + pos
		}
		sinks[ident.Name]++
		return true, ""
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
		if ok, reason := summaryValueCompliant(summary); !ok {
			result.violate(fset, summary, reason)
			continue
		}
		result.guarded++
	}
	// `.Summary =` field writes (fold-in round five, EE(ii)): the var +
	// field-assignment construction obeys the same rule as a literal.
	for _, assign := range summaryWrites {
		for i, lhs := range assign.Lhs {
			sel, ok := lhs.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Summary" || len(assign.Rhs) != len(assign.Lhs) {
				continue
			}
			if assign.Tok == token.ADD_ASSIGN {
				continue // appending after a compliant write is fine
			}
			if ok, reason := summaryValueCompliant(assign.Rhs[i]); !ok {
				result.violate(fset, assign.Rhs[i], reason)
				continue
			}
			if recv, isIdent := sel.X.(*ast.Ident); isIdent {
				summaryFieldWritten[recv.Name] = true
			}
			result.guarded++
		}
	}
	// Returns (fold-in round five, EE(ii)): a returned ToolResult local
	// must carry the choke-point summary (a literal Summary is checked by
	// the literal rule; a var-declared local needs a compliant field
	// write), and a returned helper call that RECEIVES the bound summary is
	// followed by data flow — the helper's returned ToolResult Summary must
	// be the parameter the summary arrived in. A helper call that receives
	// no bound summary is a non-installing exit (pass 2 pins that no other
	// path can install) and is exempt.
	for _, ret := range returns {
		for _, res := range ret.Results {
			switch v := res.(type) {
			case *ast.Ident:
				if toolResultLocals[v.Name] && !summaryFieldWritten[v.Name] {
					result.violate(fset, v, "types.ToolResult local "+v.Name+" flows to a return without a compliant Summary (write out.Summary = installFinishedReport(report, base) or build the literal with it)")
				}
			case *ast.CallExpr:
				callee := calleeIdent(v)
				if callee == nil || callee.Name == "installFinishedReport" {
					continue
				}
				boundArg := -1
				for i, arg := range v.Args {
					if ident, ok := arg.(*ast.Ident); ok && bound[ident.Name] {
						boundArg = i
					}
				}
				if boundArg < 0 {
					continue
				}
				fd, ok := helpers[callee.Name]
				if !ok {
					result.violate(fset, v, "helper "+callee.Name+" receives the choke-point summary but cannot be resolved in this package")
					continue
				}
				if helperSummaryParamIndex(fd) != boundArg {
					result.violate(fset, v, "helper "+callee.Name+" receives the choke-point summary but its returned types.ToolResult.Summary is not that parameter (the audit sentence is dropped)")
					continue
				}
				if ident, ok := v.Args[boundArg].(*ast.Ident); ok {
					sinks[ident.Name]++
				}
				result.guarded++
			}
		}
	}
	// Every bound choke-point result must reach at least one Summary sink.
	for name := range bound {
		if sinks[name] == 0 {
			result.violate(fset, body, "the installFinishedReport result "+name+" never reaches a returned types.ToolResult.Summary (the audit sentence is dropped)")
		}
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
	// Fold-in round five, finding EE(i): references that are not direct
	// calls — aliases, function values, parenthesised callees — are red.
	expect("alias_binding_of_install", chokePointPrelude+`
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	f := installRunTestsReport
	f(ctx, report, dryRunProbe)
	return types.ToolResult{Summary: installFinishedReport(report, base)}
}
`, "referenced as a value")
	expect("package_level_alias_of_install", chokePointPrelude+`
var installAlias = installRunTestsReport
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	return types.ToolResult{Summary: installFinishedReport(report, base)}
}
`, "referenced as a value")
	expect("install_passed_as_argument", chokePointPrelude+`
func runWith(fn func(*types.BusContext, *types.ChangeReport, bool)) {}
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	runWith(installRunTestsReport)
	return types.ToolResult{Summary: installFinishedReport(report, base)}
}
`, "referenced as a value")
	expect("parenthesised_callee_outside_choke_point", chokePointPrelude+`
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	(installRunTestsReport)(ctx, report, dryRunProbe)
	return types.ToolResult{Summary: installFinishedReport(report, base)}
}
`, "called outside the installFinishedReport choke point")
	// Fold-in round five, finding EE(ii): a `_` binding is a discard, a
	// bound result must reach a returned Summary, the var + field-write
	// construction obeys the rule, and a returned helper builder is
	// followed by data flow.
	expect("underscore_binding_is_a_discard", chokePointPrelude+`
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	_ = installFinishedReport(report, base)
	return types.ToolResult{Summary: base}
}
`, "installFinishedReport result discarded (`_ =` binding)")
	expect("bound_result_never_reaches_a_summary", chokePointPrelude+`
func log(s string) {}
func refusal() types.ToolResult { return types.ToolResult{} }
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	summary := installFinishedReport(report, base)
	log(summary)
	return refusal()
}
`, "never reaches a returned types.ToolResult.Summary")
	expect("var_plus_field_write_from_elsewhere", chokePointPrelude+`
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	summary := installFinishedReport(report, base)
	_ = summary
	var out types.ToolResult
	out.Summary = base
	return out
}
`, "Summary is not the installFinishedReport result")
	expect("var_without_any_summary_flows_to_return", chokePointPrelude+`
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	summary := installFinishedReport(report, base)
	_ = summary
	var out types.ToolResult
	out.Success = true
	return out
}
`, "flows to a return without a compliant Summary")
	expect("helper_builder_drops_the_summary", chokePointPrelude+`
func buildResult(summary string) types.ToolResult { return types.ToolResult{Summary: "rebuilt elsewhere"} }
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	summary := installFinishedReport(report, base)
	return buildResult(summary)
}
`, "its returned types.ToolResult.Summary is not that parameter")
	expect("helper_builder_unresolvable", chokePointPrelude+`
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	summary := installFinishedReport(report, base)
	return otherPackageBuild(summary)
}
`, "cannot be resolved in this package")
	t.Run("round_five_accepted_shapes_stay_green", func(t *testing.T) {
		texts := chokePointSelfRed(t, chokePointPrelude+`
func buildResult(prefix, summary string) types.ToolResult { return types.ToolResult{Summary: summary, Success: true} }
func errResult(name, msg string) types.ToolResult { return types.ToolResult{Summary: msg} }
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {`+chokePointDefinition+`
	if base == "refused" {
		// A refusal exit installs nothing and receives no bound summary:
		// following errResult by data flow is not required.
		return errResult("run_tests", "rejected")
	}
	if base == "helper" {
		summary := installFinishedReport(report, base)
		return buildResult("[run_tests]", summary)
	}
	if base == "field" {
		summary := installFinishedReport(report, base)
		var out types.ToolResult
		out.Summary = summary
		out.Summary += "\nprobe output"
		return out
	}
	summary := installFinishedReport(report, base)
	return types.ToolResult{Summary: summary}
}
`)
		if len(texts) != 0 {
			t.Fatalf("round-five accepted shapes must be green: %v", texts)
		}
	})
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
