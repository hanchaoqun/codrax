package repl

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// data_task_output_contract_census_test.go — V9-4 tripwire (colleague_merge_audit
// §40.27 / §40.56, same shape as the §40.46 contract-id census): the data
// lane's output contract is resolved by ONE resolver and every gate that
// judges actions against a contract reads the resolver's snapshot. The
// census is bound to data flow over every shape in package repl's non-test
// files and fails loud on any shape it does not recognize (§40.50 ruling):
//
//	(a) writers — every `<x>.OutputContract = …` assignment and every
//	    `OutputContract:` key of a `dataquery.TaskPlan{…}` literal is one of
//	    the closed allowlisted (function, target) pairs: the resolver
//	    (dataTaskCarryDurableOutputContract), the draft decoder (toPlan), the
//	    two system freeform resets whose plans then pass the drift guard,
//	    the resume terminal fill (no actions) and the Result seed (not a
//	    plan). An unknown writer is red.
//	(b) gate baseline — every dataquery.NormalizeDataActionForOutputContract
//	    call reads `<plan>.OutputContract` where, earlier in the SAME
//	    function body, `<plan>, _ = dataTaskCarryDurableOutputContract(<plan>,
//	    <baseline>)` ran and `<baseline>` is a parameter of the enclosing
//	    declared function (the caller's snapshot, never a locally derived
//	    value). Any other argument shape is red.
//	(c) loop binding — in data_task_cli.go and repl.go the `protectPlan`
//	    closure carries exactly twice into the identifier
//	    `durableOutputContract`, and the `runtimeView` closure of the SAME
//	    enclosing function assigns that identifier to
//	    `view.ExecutionOutputContract` without re-declaring it (Go lexical
//	    scoping then makes the gate baseline the resolver's own variable).
//	(d) admission — dataTaskWorkflowActionStagingGuardResult calls
//	    dataworkflow.ActionOutputContractGuardResult on its plan parameter,
//	    so plans that never met the planner gate meet the same judge.
//	(e) gate callers (G6-data-contract #1, 合流复核收编) — every call of
//	    planDataTaskWithTool passes, as its execution baseline, either the
//	    undeclared literal `dataquery.OutputContract{}` (initial plan, no
//	    workflow yet) or `dataTaskExecutionOutputContractBaseline(<view>)`
//	    — the single reader of the loop's carried value. A caller handing
//	    the gate a locally derived snapshot (view.CurrentPlan.OutputContract,
//	    a fresh fold, a variable) is red; the caller count floor is ≥ 3.
//	(f) nested writers — a field-level write `<x>.OutputContract.<f> = …`,
//	    an index/slice write through it, an `&<x>.OutputContract` address
//	    taken (mutation by pointer), or an `OutputContract` field passed by
//	    pointer is red with no allowlist: the contract is written whole by
//	    rule (a)'s writers only.
//	(g) loop views — the function that owns the `protectPlan` closure never
//	    builds a `dataTaskWorkflowRuntimeView{…}` literal: every in-loop
//	    planner call takes runtimeView(), which is where the carried value
//	    is bound (rule (c)); a literal view would silently fall back to the
//	    seed fold.
//	(h) seed fold (batch-six fold-in #8, 收编复核再收编) — ONE declaration
//	    chain: the body of dataTaskWorkflowOutputContract reads OutputContract
//	    only through `<rec>.Plan.OutputContract` and the current-plan
//	    parameter (never a `.Result` — a Result.OutputContract is an
//	    execution echo the carry chain never reads); every
//	    `durableOutputContract :=` seed in data_task_cli.go / repl.go is
//	    that fold's call; and dataTaskExecutionOutputContractBaseline
//	    returns either the loop's carried value or that fold — no resume
//	    path derives its own snapshot.
//	(h2) declaration chains (same fold-in) — every ResolveOutputContract
//	    argument in the scanned files is a declaration: the seed fold, a
//	    bare carried identifier, or a `.OutputContract` selector whose chain
//	    has no `Result` segment and no result-named root. The ONE exception
//	    is the judged helper dataTaskCandidateJudgedOutputContract, where
//	    the candidate's own contract (a script-lane payload DECLARATION,
//	    dataquery.parseRunnerResult) is the FIRST argument with the fold
//	    after it, so the owed chain wins every tie; the three
//	    reference-grounding validators used to fold `result.OutputContract`
//	    last (highest) and let an echo outrank the carried revision.
//
// A parse error is red; the file-count floor keeps a silently empty scan
// red; every rule has a self-red witness below.

const dataTaskOutputContractCensusFileFloor = 10

var dataTaskOutputContractWriterAllowlist = map[string]string{
	"dataTaskCarryDurableOutputContract/candidate.OutputContract":   "the resolver",
	"dataTaskPlanDraft.toPlan/dataquery.TaskPlan{OutputContract:}":  "draft decoder; the gate carries the resolver snapshot right after",
	"dataTaskNoEmitterScriptObservationFallback/out.OutputContract": "system freeform observation reset; passes the admission drift guard",
	"dataTaskCandidateInventoryBootstrapPlan/out.OutputContract":    "system freeform inventory reset; passes the admission drift guard",
	"nextDataTaskPlanFromResumeForCLI/out.OutputContract":           "resume terminal fill of a complete plan without actions",
	"dataTaskActionRunnerSeed/out.OutputContract":                   "Result seed, not a plan",
}

func dataTaskOutputContractCensusFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files[name] = file
	}
	if len(files) < dataTaskOutputContractCensusFileFloor {
		t.Fatalf("scanned %d files, below the %d floor — census walk drifted", len(files), dataTaskOutputContractCensusFileFloor)
	}
	return files
}

func parseDataTaskCensusSource(t *testing.T, src string) map[string]*ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "probe.go", src, 0)
	if err != nil {
		t.Fatalf("parse probe: %v", err)
	}
	return map[string]*ast.File{"probe.go": file}
}

// walkWithFuncStack visits every node with the enclosing FuncDecl/FuncLit
// stack (outermost first).
func walkWithFuncStack(file *ast.File, visit func(node ast.Node, stack []ast.Node)) {
	var stack []ast.Node
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			return false
		}
		visit(node, stack)
		switch node.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			stack = append(stack, node)
		default:
			stack = append(stack, node)
		}
		return true
	})
}

func enclosingFuncDecl(stack []ast.Node) *ast.FuncDecl {
	for i := len(stack) - 1; i >= 0; i-- {
		if decl, ok := stack[i].(*ast.FuncDecl); ok {
			return decl
		}
	}
	return nil
}

func enclosingFuncBody(stack []ast.Node) *ast.BlockStmt {
	for i := len(stack) - 1; i >= 0; i-- {
		switch fn := stack[i].(type) {
		case *ast.FuncLit:
			return fn.Body
		case *ast.FuncDecl:
			return fn.Body
		}
	}
	return nil
}

func funcDeclCensusName(decl *ast.FuncDecl) string {
	if decl == nil {
		return "<package>"
	}
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		return exprCensusText(decl.Recv.List[0].Type) + "." + decl.Name.Name
	}
	return decl.Name.Name
}

func exprCensusText(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprCensusText(e.X) + "." + e.Sel.Name
	case *ast.StarExpr:
		return "*" + exprCensusText(e.X)
	case *ast.CallExpr:
		return exprCensusText(e.Fun) + "(…)"
	default:
		return fmt.Sprintf("<%T>", expr)
	}
}

func isCall(node ast.Node, name string) (*ast.CallExpr, bool) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	if exprCensusText(call.Fun) != name {
		return nil, false
	}
	return call, true
}

// censusDataTaskOutputContractWriters — rule (a).
func censusDataTaskOutputContractWriters(files map[string]*ast.File, allow map[string]string) []string {
	var offenders []string
	for name, file := range files {
		walkWithFuncStack(file, func(node ast.Node, stack []ast.Node) {
			switch n := node.(type) {
			case *ast.AssignStmt:
				for _, lhs := range n.Lhs {
					sel, ok := lhs.(*ast.SelectorExpr)
					if !ok || sel.Sel.Name != "OutputContract" {
						continue
					}
					key := funcDeclCensusName(enclosingFuncDecl(stack)) + "/" + exprCensusText(sel)
					if _, ok := allow[key]; !ok {
						offenders = append(offenders, fmt.Sprintf("%s: unrecognized OutputContract writer %q", name, key))
					}
				}
			case *ast.CompositeLit:
				if exprCensusText(n.Type) != "dataquery.TaskPlan" {
					return
				}
				for _, elt := range n.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if ident, ok := kv.Key.(*ast.Ident); !ok || ident.Name != "OutputContract" {
						continue
					}
					key := funcDeclCensusName(enclosingFuncDecl(stack)) + "/dataquery.TaskPlan{OutputContract:}"
					if _, ok := allow[key]; !ok {
						offenders = append(offenders, fmt.Sprintf("%s: unrecognized OutputContract literal writer %q", name, key))
					}
				}
			}
		})
	}
	sort.Strings(offenders)
	return offenders
}

// censusDataTaskOutputContractGateBaseline — rule (b).
func censusDataTaskOutputContractGateBaseline(files map[string]*ast.File) (offenders []string, gates int) {
	for name, file := range files {
		walkWithFuncStack(file, func(node ast.Node, stack []ast.Node) {
			call, ok := isCall(node, "dataquery.NormalizeDataActionForOutputContract")
			if !ok {
				return
			}
			gates++
			where := fmt.Sprintf("%s: %s", name, funcDeclCensusName(enclosingFuncDecl(stack)))
			if len(call.Args) != 2 {
				offenders = append(offenders, where+": gate call arity is not (action, contract)")
				return
			}
			sel, ok := call.Args[1].(*ast.SelectorExpr)
			planIdent, identOK := sel.X.(*ast.Ident)
			if !ok || sel.Sel.Name != "OutputContract" || !identOK {
				offenders = append(offenders, where+": gate contract argument is not <plan>.OutputContract")
				return
			}
			body := enclosingFuncBody(stack)
			decl := enclosingFuncDecl(stack)
			params := map[string]bool{}
			if decl != nil && decl.Type.Params != nil {
				for _, field := range decl.Type.Params.List {
					for _, ident := range field.Names {
						params[ident.Name] = true
					}
				}
			}
			carried := false
			if body != nil {
				ast.Inspect(body, func(inner ast.Node) bool {
					if _, nested := inner.(*ast.FuncLit); nested {
						// A carry inside a nested closure is another scope's
						// statement order; only the gate's own body counts.
						return false
					}
					assign, ok := inner.(*ast.AssignStmt)
					if !ok || assign.Pos() >= call.Pos() || len(assign.Rhs) != 1 || len(assign.Lhs) < 1 {
						return true
					}
					carry, ok := isCall(assign.Rhs[0], "dataTaskCarryDurableOutputContract")
					if !ok || len(carry.Args) != 2 {
						return true
					}
					lhs, ok := assign.Lhs[0].(*ast.Ident)
					arg, argOK := carry.Args[0].(*ast.Ident)
					baseline, baseOK := carry.Args[1].(*ast.Ident)
					if !ok || !argOK || !baseOK || lhs.Name != planIdent.Name || arg.Name != planIdent.Name {
						return true
					}
					if !params[baseline.Name] {
						offenders = append(offenders, where+": carry baseline "+baseline.Name+" is not a parameter of the enclosing function (locally derived snapshot)")
						return true
					}
					carried = true
					return true
				})
			}
			if !carried {
				offenders = append(offenders, where+": gate judges "+planIdent.Name+".OutputContract without a preceding resolver carry into "+planIdent.Name)
			}
		})
	}
	sort.Strings(offenders)
	return offenders, gates
}

// censusDataTaskOutputContractLoopBinding — rule (c) for one file.
func censusDataTaskOutputContractLoopBinding(name string, file *ast.File) []string {
	var offenders []string
	type closure struct {
		lit  *ast.FuncLit
		decl *ast.FuncDecl
	}
	closures := map[string][]closure{}
	walkWithFuncStack(file, func(node ast.Node, stack []ast.Node) {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return
		}
		ident, ok := assign.Lhs[0].(*ast.Ident)
		lit, litOK := assign.Rhs[0].(*ast.FuncLit)
		if !ok || !litOK || (ident.Name != "protectPlan" && ident.Name != "runtimeView") {
			return
		}
		closures[ident.Name] = append(closures[ident.Name], closure{lit: lit, decl: enclosingFuncDecl(stack)})
	})
	if len(closures["protectPlan"]) != 1 || len(closures["runtimeView"]) != 1 {
		return append(offenders, fmt.Sprintf("%s: protectPlan closures=%d runtimeView closures=%d, want exactly one each", name, len(closures["protectPlan"]), len(closures["runtimeView"])))
	}
	protect, view := closures["protectPlan"][0], closures["runtimeView"][0]
	if protect.decl != view.decl {
		offenders = append(offenders, name+": protectPlan and runtimeView closures live in different functions")
	}
	carries := 0
	ast.Inspect(protect.lit.Body, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 {
			return true
		}
		if _, ok := isCall(assign.Rhs[0], "dataTaskCarryDurableOutputContract"); !ok {
			return true
		}
		if len(assign.Lhs) != 2 {
			offenders = append(offenders, name+": protectPlan carry does not bind (plan, durable)")
			return true
		}
		if ident, ok := assign.Lhs[1].(*ast.Ident); !ok || ident.Name != "durableOutputContract" {
			offenders = append(offenders, name+": protectPlan carry writes a different durable identifier")
			return true
		}
		carries++
		return true
	})
	if carries != 2 {
		offenders = append(offenders, fmt.Sprintf("%s: protectPlan durable carries=%d, want exactly 2 around plan preparation", name, carries))
	}
	bound := false
	ast.Inspect(view.lit.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && ident.Name == "durableOutputContract" {
					offenders = append(offenders, name+": runtimeView re-declares or rewrites durableOutputContract")
				}
			}
			if len(n.Lhs) != 1 || len(n.Rhs) != 1 {
				return true
			}
			if exprCensusText(n.Lhs[0]) != "view.ExecutionOutputContract" {
				return true
			}
			if ident, ok := n.Rhs[0].(*ast.Ident); ok && ident.Name == "durableOutputContract" {
				bound = true
			} else {
				offenders = append(offenders, name+": runtimeView binds ExecutionOutputContract to something other than durableOutputContract")
			}
		case *ast.ValueSpec:
			for _, ident := range n.Names {
				if ident.Name == "durableOutputContract" {
					offenders = append(offenders, name+": runtimeView declares a shadowing durableOutputContract")
				}
			}
		}
		return true
	})
	if !bound {
		offenders = append(offenders, name+": runtimeView does not bind view.ExecutionOutputContract = durableOutputContract")
	}
	sort.Strings(offenders)
	return offenders
}

// censusDataTaskOutputContractAdmission — rule (d).
func censusDataTaskOutputContractAdmission(files map[string]*ast.File) []string {
	var offenders []string
	found := false
	for name, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "dataTaskWorkflowActionStagingGuardResult" || fn.Body == nil {
				continue
			}
			found = true
			planParam := ""
			for _, field := range fn.Type.Params.List {
				if exprCensusText(field.Type) == "dataquery.TaskPlan" && len(field.Names) > 0 {
					planParam = field.Names[0].Name
				}
			}
			called := false
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := isCall(node, "dataworkflow.ActionOutputContractGuardResult")
				if !ok {
					return true
				}
				if len(call.Args) == 1 {
					if ident, ok := call.Args[0].(*ast.Ident); ok && ident.Name == planParam {
						called = true
						return true
					}
				}
				offenders = append(offenders, name+": staging guard calls the drift judge on something other than its plan parameter")
				return true
			})
			if !called {
				offenders = append(offenders, name+": dataTaskWorkflowActionStagingGuardResult does not judge its plan with dataworkflow.ActionOutputContractGuardResult")
			}
		}
	}
	if !found {
		offenders = append(offenders, "dataTaskWorkflowActionStagingGuardResult not found — census walk drifted")
	}
	sort.Strings(offenders)
	return offenders
}

// censusDataTaskOutputContractGateCallers — rule (e).
func censusDataTaskOutputContractGateCallers(files map[string]*ast.File) (offenders []string, callers int) {
	for name, file := range files {
		walkWithFuncStack(file, func(node ast.Node, stack []ast.Node) {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "planDataTaskWithTool" {
				return
			}
			callers++
			where := fmt.Sprintf("%s: %s", name, funcDeclCensusName(enclosingFuncDecl(stack)))
			if len(call.Args) != 5 {
				offenders = append(offenders, where+": planDataTaskWithTool call arity is not (ctx, scope, prompt, tool, executionBaseline)")
				return
			}
			switch arg := call.Args[4].(type) {
			case *ast.CompositeLit:
				if exprCensusText(arg.Type) == "dataquery.OutputContract" && len(arg.Elts) == 0 {
					return
				}
				offenders = append(offenders, where+": execution baseline is a non-empty or foreign literal "+exprCensusText(arg.Type))
			case *ast.CallExpr:
				if exprCensusText(arg.Fun) == "dataTaskExecutionOutputContractBaseline" && len(arg.Args) == 1 {
					if _, isIdent := arg.Args[0].(*ast.Ident); isIdent {
						return
					}
				}
				offenders = append(offenders, where+": execution baseline is derived by "+exprCensusText(arg)+" instead of dataTaskExecutionOutputContractBaseline(<view>)")
			default:
				offenders = append(offenders, where+": execution baseline is a locally derived snapshot "+exprCensusText(arg))
			}
		})
	}
	sort.Strings(offenders)
	return offenders, callers
}

// outputContractSelectorInChain reports whether expr (an lvalue) reaches
// through a `.OutputContract` selector below its top level — a field,
// index or slice write into the contract rather than a whole-value write.
func outputContractSelectorInChain(expr ast.Expr) bool {
	for {
		var inner ast.Expr
		switch e := expr.(type) {
		case *ast.SelectorExpr:
			inner = e.X
		case *ast.IndexExpr:
			inner = e.X
		case *ast.SliceExpr:
			inner = e.X
		case *ast.ParenExpr:
			expr = e.X
			continue
		case *ast.StarExpr:
			expr = e.X
			continue
		default:
			return false
		}
		if sel, ok := inner.(*ast.SelectorExpr); ok && sel.Sel.Name == "OutputContract" {
			return true
		}
		expr = inner
	}
}

// censusDataTaskOutputContractNestedWriters — rule (f).
func censusDataTaskOutputContractNestedWriters(files map[string]*ast.File) []string {
	var offenders []string
	for name, file := range files {
		walkWithFuncStack(file, func(node ast.Node, stack []ast.Node) {
			where := fmt.Sprintf("%s: %s", name, funcDeclCensusName(enclosingFuncDecl(stack)))
			switch n := node.(type) {
			case *ast.AssignStmt:
				for _, lhs := range n.Lhs {
					if outputContractSelectorInChain(lhs) {
						offenders = append(offenders, where+": nested OutputContract write "+exprCensusText(lhs))
					}
				}
			case *ast.IncDecStmt:
				if outputContractSelectorInChain(n.X) {
					offenders = append(offenders, where+": nested OutputContract write "+exprCensusText(n.X))
				}
			case *ast.UnaryExpr:
				if n.Op != token.AND {
					return
				}
				target := n.X
				for {
					paren, ok := target.(*ast.ParenExpr)
					if !ok {
						break
					}
					target = paren.X
				}
				if sel, ok := target.(*ast.SelectorExpr); ok && sel.Sel.Name == "OutputContract" {
					offenders = append(offenders, where+": OutputContract address taken "+exprCensusText(n.X))
				} else if outputContractSelectorInChain(target) {
					offenders = append(offenders, where+": address of a field inside OutputContract "+exprCensusText(n.X))
				}
			}
		})
	}
	sort.Strings(offenders)
	return offenders
}

// censusDataTaskOutputContractLoopViews — rule (g) for one file.
func censusDataTaskOutputContractLoopViews(name string, file *ast.File) []string {
	var offenders []string
	var owner *ast.FuncDecl
	var viewClosure *ast.FuncLit
	walkWithFuncStack(file, func(node ast.Node, stack []ast.Node) {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return
		}
		ident, ok := assign.Lhs[0].(*ast.Ident)
		lit, litOK := assign.Rhs[0].(*ast.FuncLit)
		if !ok || !litOK {
			return
		}
		switch ident.Name {
		case "protectPlan":
			owner = enclosingFuncDecl(stack)
		case "runtimeView":
			viewClosure = lit
		}
	})
	if owner == nil || owner.Body == nil {
		return append(offenders, name+": no function owns a protectPlan closure — census walk drifted")
	}
	ast.Inspect(owner.Body, func(node ast.Node) bool {
		if node == viewClosure {
			// The runtimeView closure is the one place the loop composes
			// its view; rule (c) pins that it binds ExecutionOutputContract.
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || exprCensusText(lit.Type) != "dataTaskWorkflowRuntimeView" {
			return true
		}
		offenders = append(offenders, fmt.Sprintf("%s: %s builds a dataTaskWorkflowRuntimeView literal inside the loop (bypasses runtimeView(), falls back to the seed fold)", name, funcDeclCensusName(owner)))
		return true
	})
	sort.Strings(offenders)
	return offenders
}

// censusDataTaskOutputContractSeedFold — rule (h).
func censusDataTaskOutputContractSeedFold(files map[string]*ast.File) []string {
	var offenders []string
	folds, baselines := 0, 0
	for name, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv != nil {
				continue
			}
			switch fn.Name.Name {
			case "dataTaskWorkflowOutputContract":
				folds++
				current := ""
				if params := fn.Type.Params; params != nil && len(params.List) == 2 && len(params.List[1].Names) == 1 {
					current = params.List[1].Names[0].Name
				}
				if current == "" {
					offenders = append(offenders, name+": (h) seed fold signature drifted from (records, current)")
				}
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					sel, ok := node.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					switch sel.Sel.Name {
					case "Result":
						offenders = append(offenders, fmt.Sprintf("%s: (h) seed fold reads %s — a Result.OutputContract is an execution echo, never a declaration", name, exprCensusText(sel)))
					case "OutputContract":
						x := sel.X
						if inner, ok := x.(*ast.SelectorExpr); ok && inner.Sel.Name == "Plan" {
							if _, ok := inner.X.(*ast.Ident); ok {
								return true
							}
						}
						if ident, ok := x.(*ast.Ident); ok && (ident.Name == current || ident.Name == "dataquery") {
							// The current plan's declaration, or the
							// package-qualified TYPE dataquery.OutputContract
							// (the fold's value slice) — not a read.
							return true
						}
						offenders = append(offenders, fmt.Sprintf("%s: (h) seed fold reads OutputContract through %q, not a Plan-level declaration", name, exprCensusText(sel)))
						// The offending chain is reported once; its inner
						// selectors are not re-reported as separate reads.
						return false
					}
					return true
				})
			case "dataTaskExecutionOutputContractBaseline":
				baselines++
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					ret, ok := node.(*ast.ReturnStmt)
					if !ok || len(ret.Results) != 1 {
						return true
					}
					if exprCensusText(ret.Results[0]) == "view.ExecutionOutputContract" {
						return true
					}
					if _, ok := isCall(ret.Results[0], "dataTaskWorkflowOutputContract"); ok {
						return true
					}
					offenders = append(offenders, fmt.Sprintf("%s: (h) gate baseline returns %q, neither the loop's carried value nor the seed fold", name, exprCensusText(ret.Results[0])))
					return true
				})
			}
		}
		if name != "data_task_cli.go" && name != "repl.go" {
			continue
		}
		seeds := 0
		ast.Inspect(file, func(node ast.Node) bool {
			assign, ok := node.(*ast.AssignStmt)
			if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
				return true
			}
			if ident, ok := assign.Lhs[0].(*ast.Ident); !ok || ident.Name != "durableOutputContract" {
				return true
			}
			seeds++
			if _, ok := isCall(assign.Rhs[0], "dataTaskWorkflowOutputContract"); !ok {
				offenders = append(offenders, fmt.Sprintf("%s: (h) durableOutputContract is seeded from %q, not the seed fold", name, exprCensusText(assign.Rhs[0])))
			}
			return true
		})
		if seeds != 1 {
			offenders = append(offenders, fmt.Sprintf("%s: (h) durableOutputContract seeds=%d, want exactly one loop seed", name, seeds))
		}
	}
	if folds != 1 || baselines != 1 {
		offenders = append(offenders, fmt.Sprintf("(h) seed folds=%d gate baselines=%d, want exactly one each — census walk drifted", folds, baselines))
	}
	sort.Strings(offenders)
	return offenders
}

// censusDataTaskOutputContractDeclarationChains — rule (h2): every
// ResolveOutputContract call in the scanned files folds DECLARATIONS only.
// An argument is declaration-shaped when it is the seed fold
// (dataTaskWorkflowOutputContract(...)), a bare identifier (the loop's
// carried value, or the fold's value slice), or a `.OutputContract`
// selector whose chain has no `Result` segment and whose root identifier is
// not result-named. A `result.OutputContract` / `rec.Result.OutputContract`
// argument is an execution echo entering a declaration chain (batch-six
// fold-in #8: four completion/reference authorities ranked the echo highest
// and a json_only echo under a revised plain_single_line plan drove the
// validator proposal and the CLI resume back to json_only).
func censusDataTaskOutputContractDeclarationChains(files map[string]*ast.File) []string {
	var offenders []string
	calls, judged := 0, 0
	for name, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			inJudged := fn.Recv == nil && fn.Name.Name == "dataTaskCandidateJudgedOutputContract"
			if inJudged {
				judged++
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				fun := exprCensusText(call.Fun)
				if fun != "dataworkflow.ResolveOutputContract" && fun != "ResolveOutputContract" {
					return true
				}
				calls++
				foldAt := -1
				for i, arg := range call.Args {
					if _, ok := isCall(arg, "dataTaskWorkflowOutputContract"); ok {
						foldAt = i
					}
				}
				for i, arg := range call.Args {
					if _, ok := isCall(arg, "dataTaskWorkflowOutputContract"); ok {
						continue
					}
					switch a := arg.(type) {
					case *ast.Ident:
						continue
					case *ast.SelectorExpr:
						if a.Sel.Name != "OutputContract" {
							break
						}
						if !censusSelectorChainNamesResult(a.X) {
							continue
						}
						// The candidate's own contract enters ONE resolve: the
						// judged helper, first (lowest), with the fold after
						// it, so the owed chain wins every tie.
						if inJudged && i == 0 && foldAt > 0 {
							continue
						}
						if inJudged {
							offenders = append(offenders, fmt.Sprintf("%s: (h2) judged helper folds %q at position %d (fold at %d) — the candidate contract must be the FIRST argument with the seed fold after it", name, exprCensusText(arg), i, foldAt))
							continue
						}
					}
					offenders = append(offenders, fmt.Sprintf("%s: (h2) declaration chain folds %q — a Result.OutputContract is an execution echo, never a declaration", name, exprCensusText(arg)))
				}
				return true
			})
		}
	}
	if calls < 3 || judged != 1 {
		offenders = append(offenders, fmt.Sprintf("(h2) ResolveOutputContract calls=%d judged helpers=%d, want the seed fold, the carry resolver and exactly one judged helper — census walk drifted", calls, judged))
	}
	sort.Strings(offenders)
	return offenders
}

// censusSelectorChainNamesResult reports whether a selector chain has a
// `Result` segment or a result-named root identifier (result, answerResult,
// res, …): the shapes through which an execution echo reaches a chain.
func censusSelectorChainNamesResult(expr ast.Expr) bool {
	for {
		switch e := expr.(type) {
		case *ast.SelectorExpr:
			if e.Sel.Name == "Result" {
				return true
			}
			expr = e.X
		case *ast.Ident:
			return strings.Contains(strings.ToLower(e.Name), "result") || e.Name == "res"
		case *ast.ParenExpr:
			expr = e.X
		case *ast.StarExpr:
			expr = e.X
		default:
			return false
		}
	}
}

func TestDataTaskOutputContractSnapshotCensus(t *testing.T) {
	files := dataTaskOutputContractCensusFiles(t)
	if offenders := censusDataTaskOutputContractWriters(files, dataTaskOutputContractWriterAllowlist); len(offenders) > 0 {
		t.Fatalf("(a) unrecognized OutputContract writers:\n%s", strings.Join(offenders, "\n"))
	}
	offenders, gates := censusDataTaskOutputContractGateBaseline(files)
	if len(offenders) > 0 {
		t.Fatalf("(b) gate baseline offenders:\n%s", strings.Join(offenders, "\n"))
	}
	if gates < 1 {
		t.Fatalf("(b) judged gates=%d, want at least the planner pre-dispatch gate — census walk drifted", gates)
	}
	for _, name := range []string{"data_task_cli.go", "repl.go"} {
		file, ok := files[name]
		if !ok {
			t.Fatalf("(c) %s not scanned", name)
		}
		if offenders := censusDataTaskOutputContractLoopBinding(name, file); len(offenders) > 0 {
			t.Fatalf("(c) loop binding offenders:\n%s", strings.Join(offenders, "\n"))
		}
	}
	if offenders := censusDataTaskOutputContractAdmission(files); len(offenders) > 0 {
		t.Fatalf("(d) admission offenders:\n%s", strings.Join(offenders, "\n"))
	}
	callerOffenders, callers := censusDataTaskOutputContractGateCallers(files)
	if len(callerOffenders) > 0 {
		t.Fatalf("(e) gate caller offenders:\n%s", strings.Join(callerOffenders, "\n"))
	}
	if callers < 3 {
		t.Fatalf("(e) planDataTaskWithTool callers=%d, want at least initial/repair/continuation — census walk drifted", callers)
	}
	if offenders := censusDataTaskOutputContractNestedWriters(files); len(offenders) > 0 {
		t.Fatalf("(f) nested OutputContract writers:\n%s", strings.Join(offenders, "\n"))
	}
	for _, name := range []string{"data_task_cli.go", "repl.go"} {
		if offenders := censusDataTaskOutputContractLoopViews(name, files[name]); len(offenders) > 0 {
			t.Fatalf("(g) loop view offenders:\n%s", strings.Join(offenders, "\n"))
		}
	}
	if offenders := censusDataTaskOutputContractSeedFold(files); len(offenders) > 0 {
		t.Fatalf("(h) seed fold offenders:\n%s", strings.Join(offenders, "\n"))
	}
	if offenders := censusDataTaskOutputContractDeclarationChains(files); len(offenders) > 0 {
		t.Fatalf("(h2) declaration chain offenders:\n%s", strings.Join(offenders, "\n"))
	}
}

// TestDataTaskOutputContractSnapshotCensusSelfRed proves every rule bites a
// synthetic evasion shape (red witnesses for the census itself).
func TestDataTaskOutputContractSnapshotCensusSelfRed(t *testing.T) {
	const header = "package repl\nimport \"github.com/hanchaoqun/codrax/internal/dataquery\"\n"
	t.Run("unknown_writer", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, header+`
func sneakyResolver(p dataquery.TaskPlan) dataquery.TaskPlan { p.OutputContract = dataquery.OutputContract{}; return p }
`)
		if offenders := censusDataTaskOutputContractWriters(files, dataTaskOutputContractWriterAllowlist); len(offenders) != 1 || !strings.Contains(offenders[0], "sneakyResolver/p.OutputContract") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("unknown_literal_writer", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, header+`
func sneakyBuilder() dataquery.TaskPlan { return dataquery.TaskPlan{OutputContract: dataquery.OutputContract{}} }
`)
		if offenders := censusDataTaskOutputContractWriters(files, dataTaskOutputContractWriterAllowlist); len(offenders) != 1 || !strings.Contains(offenders[0], "sneakyBuilder/dataquery.TaskPlan{OutputContract:}") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("gate_without_carry", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, header+`
func gate(plan dataquery.TaskPlan, baseline dataquery.OutputContract) error {
	for _, a := range plan.Actions { if _, err := dataquery.NormalizeDataActionForOutputContract(a, plan.OutputContract); err != nil { return err } }
	return nil
}
`)
		offenders, gates := censusDataTaskOutputContractGateBaseline(files)
		if gates != 1 || len(offenders) != 1 || !strings.Contains(offenders[0], "without a preceding resolver carry") {
			t.Fatalf("gates=%d offenders=%v", gates, offenders)
		}
	})
	t.Run("gate_with_local_baseline", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, header+`
func gate(plan dataquery.TaskPlan) error {
	local := plan.OutputContract
	plan, _ = dataTaskCarryDurableOutputContract(plan, local)
	for _, a := range plan.Actions { if _, err := dataquery.NormalizeDataActionForOutputContract(a, plan.OutputContract); err != nil { return err } }
	return nil
}
`)
		offenders, _ := censusDataTaskOutputContractGateBaseline(files)
		if len(offenders) != 2 || !strings.Contains(offenders[0], "not a parameter") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("gate_after_carry_in_closure", func(t *testing.T) {
		// The carry inside a different closure does not count for the gate.
		files := parseDataTaskCensusSource(t, header+`
func gate(plan dataquery.TaskPlan, baseline dataquery.OutputContract) error {
	prepare := func() { plan, _ = dataTaskCarryDurableOutputContract(plan, baseline) }
	prepare()
	_, err := dataquery.NormalizeDataActionForOutputContract(plan.Actions[0], plan.OutputContract)
	return err
}
`)
		offenders, _ := censusDataTaskOutputContractGateBaseline(files)
		if len(offenders) != 1 {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("gate_green_shape", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, header+`
func gate(plan dataquery.TaskPlan, baseline dataquery.OutputContract) error {
	plan, _ = dataTaskCarryDurableOutputContract(plan, baseline)
	_, err := dataquery.NormalizeDataActionForOutputContract(plan.Actions[0], plan.OutputContract)
	return err
}
`)
		offenders, gates := censusDataTaskOutputContractGateBaseline(files)
		if gates != 1 || len(offenders) != 0 {
			t.Fatalf("gates=%d offenders=%v", gates, offenders)
		}
	})
	loopSource := func(viewBody string) string {
		return header + `
func loop(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) {
	durableOutputContract := dataTaskWorkflowOutputContract(records, plan)
	protectPlan := func(p dataquery.TaskPlan) dataquery.TaskPlan {
		p, durableOutputContract = dataTaskCarryDurableOutputContract(p, durableOutputContract)
		p, durableOutputContract = dataTaskCarryDurableOutputContract(p, durableOutputContract)
		return p
	}
	runtimeView := func() dataTaskWorkflowRuntimeView {
		view := dataTaskWorkflowRuntimeView{}
` + viewBody + `
		return view
	}
	_, _ = protectPlan, runtimeView
}
`
	}
	t.Run("loop_missing_binding", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, loopSource(""))
		if offenders := censusDataTaskOutputContractLoopBinding("probe.go", files["probe.go"]); len(offenders) != 1 || !strings.Contains(offenders[0], "does not bind") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("loop_shadowed_binding", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, loopSource("durableOutputContract := dataquery.OutputContract{}\n\t\tview.ExecutionOutputContract = durableOutputContract"))
		if offenders := censusDataTaskOutputContractLoopBinding("probe.go", files["probe.go"]); len(offenders) != 1 || !strings.Contains(offenders[0], "re-declares") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("loop_foreign_binding", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, loopSource("view.ExecutionOutputContract = dataTaskWorkflowOutputContract(records, plan)"))
		if offenders := censusDataTaskOutputContractLoopBinding("probe.go", files["probe.go"]); len(offenders) != 2 {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("loop_green_shape", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, loopSource("view.ExecutionOutputContract = durableOutputContract"))
		if offenders := censusDataTaskOutputContractLoopBinding("probe.go", files["probe.go"]); len(offenders) != 0 {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("admission_missing_judge", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, header+`
func dataTaskWorkflowActionStagingGuardResult(repoRoot string, records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	return dataworkflow.GuardResult{}
}
`)
		if offenders := censusDataTaskOutputContractAdmission(files); len(offenders) != 1 || !strings.Contains(offenders[0], "does not judge") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("admission_judges_other_plan", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, header+`
func dataTaskWorkflowActionStagingGuardResult(repoRoot string, records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	other := plan
	other.OutputContract = dataquery.OutputContract{}
	return dataworkflow.ActionOutputContractGuardResult(other)
}
`)
		if offenders := censusDataTaskOutputContractAdmission(files); len(offenders) != 2 {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	callerSource := func(baseline string) string {
		return header + `
func (p *llmDataTaskPlanner) sneakyRepair(view dataTaskWorkflowRuntimeView) (dataquery.TaskPlan, error) {
	local := dataTaskWorkflowOutputContract(view.Records, view.CurrentPlan)
	_ = local
	return p.planDataTaskWithTool(nil, "scope", "prompt", dataTaskPlanTool, ` + baseline + `)
}
`
	}
	t.Run("caller_current_plan_snapshot", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, callerSource("view.CurrentPlan.OutputContract"))
		offenders, callers := censusDataTaskOutputContractGateCallers(files)
		if callers != 1 || len(offenders) != 1 || !strings.Contains(offenders[0], "locally derived snapshot view.CurrentPlan.OutputContract") {
			t.Fatalf("callers=%d offenders=%v", callers, offenders)
		}
	})
	t.Run("caller_fresh_fold", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, callerSource("dataTaskWorkflowOutputContract(view.Records, view.CurrentPlan)"))
		offenders, _ := censusDataTaskOutputContractGateCallers(files)
		if len(offenders) != 1 || !strings.Contains(offenders[0], "derived by dataTaskWorkflowOutputContract(…)") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("caller_local_variable", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, callerSource("local"))
		offenders, _ := censusDataTaskOutputContractGateCallers(files)
		if len(offenders) != 1 || !strings.Contains(offenders[0], "locally derived snapshot local") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("caller_green_shapes", func(t *testing.T) {
		for _, baseline := range []string{"dataquery.OutputContract{}", "dataTaskExecutionOutputContractBaseline(view)"} {
			files := parseDataTaskCensusSource(t, callerSource(baseline))
			if offenders, callers := censusDataTaskOutputContractGateCallers(files); callers != 1 || len(offenders) != 0 {
				t.Fatalf("baseline %s: callers=%d offenders=%v", baseline, callers, offenders)
			}
		}
	})
	t.Run("nested_field_write", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, header+`
func sneakyFormat(p dataquery.TaskPlan) dataquery.TaskPlan { p.OutputContract.Format = dataquery.OutputJSONOnly; return p }
`)
		if offenders := censusDataTaskOutputContractNestedWriters(files); len(offenders) != 1 || !strings.Contains(offenders[0], "sneakyFormat: nested OutputContract write p.OutputContract.Format") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("nested_pointer_write", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, header+`
func mutate(c *dataquery.OutputContract) { c.Format = dataquery.OutputJSONOnly }
func sneakyPointer(p dataquery.TaskPlan) dataquery.TaskPlan { mutate(&p.OutputContract); return p }
`)
		if offenders := censusDataTaskOutputContractNestedWriters(files); len(offenders) != 1 || !strings.Contains(offenders[0], "sneakyPointer: OutputContract address taken p.OutputContract") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("nested_green_shapes", func(t *testing.T) {
		// Reads through the contract and whole-value writes are rule (a)'s
		// business, not rule (f)'s.
		files := parseDataTaskCensusSource(t, header+`
func reader(p dataquery.TaskPlan) bool { format := p.OutputContract.Format; return format == "" }
func whole(p dataquery.TaskPlan) dataquery.TaskPlan { p.OutputContract = dataquery.OutputContract{}; return p }
`)
		if offenders := censusDataTaskOutputContractNestedWriters(files); len(offenders) != 0 {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("loop_literal_view", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, loopSource("view.ExecutionOutputContract = durableOutputContract")+`
func elsewhere(records []dataTaskWorkflowRecord) dataTaskWorkflowRuntimeView { return dataTaskWorkflowRuntimeView{Records: records} }
`)
		if offenders := censusDataTaskOutputContractLoopViews("probe.go", files["probe.go"]); len(offenders) != 0 {
			t.Fatalf("literal outside the loop owner must pass: offenders=%v", offenders)
		}
		// The runtimeView closure is where the loop composes its view (a
		// literal there is bound by rule (c)); a literal anywhere else in
		// the loop owner bypasses it and is red.
		files = parseDataTaskCensusSource(t, loopSource("view.ExecutionOutputContract = durableOutputContract"))
		if offenders := censusDataTaskOutputContractLoopViews("probe.go", files["probe.go"]); len(offenders) != 0 {
			t.Fatalf("literal inside runtimeView must pass: offenders=%v", offenders)
		}
		files = parseDataTaskCensusSource(t, strings.Replace(loopSource("view.ExecutionOutputContract = durableOutputContract"), "\t_, _ = protectPlan, runtimeView\n", "\t_, _ = protectPlan, runtimeView\n\tliteral := dataTaskWorkflowRuntimeView{Records: records, CurrentPlan: plan}\n\t_ = literal\n", 1))
		if offenders := censusDataTaskOutputContractLoopViews("probe.go", files["probe.go"]); len(offenders) != 1 || !strings.Contains(offenders[0], "loop builds a dataTaskWorkflowRuntimeView literal inside the loop") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("loop_owner_missing", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, header+"func nothing() {}\n")
		if offenders := censusDataTaskOutputContractLoopViews("probe.go", files["probe.go"]); len(offenders) != 1 || !strings.Contains(offenders[0], "census walk drifted") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	// Rule (h): the seed fold and every resume seed read ONE Plan-level chain.
	seedHeader := header + "import \"github.com/hanchaoqun/codrax/internal/dataworkflow\"\n"
	greenFold := `
func dataTaskWorkflowOutputContract(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataquery.OutputContract {
	var values []dataquery.OutputContract
	for _, rec := range records { values = append(values, rec.Plan.OutputContract) }
	values = append(values, current.OutputContract)
	return dataworkflow.ResolveOutputContract(values...)
}
func dataTaskExecutionOutputContractBaseline(view dataTaskWorkflowRuntimeView) dataquery.OutputContract {
	if dataworkflow.OutputContractDeclared(view.ExecutionOutputContract) { return view.ExecutionOutputContract }
	return dataTaskWorkflowOutputContract(view.Records, view.CurrentPlan)
}
func dataTaskCandidateJudgedOutputContract(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) dataquery.OutputContract {
	return dataworkflow.ResolveOutputContract(result.OutputContract, dataTaskWorkflowOutputContract(records, current)).Normalize()
}
func carryProbe(candidate dataquery.TaskPlan, durable dataquery.OutputContract) dataquery.OutputContract {
	return dataworkflow.ResolveOutputContract(durable, candidate.OutputContract)
}
`
	seedFoldFiles := func(t *testing.T, src string, loopFile string, loopSrc string) map[string]*ast.File {
		t.Helper()
		files := parseDataTaskCensusSource(t, src)
		if loopFile != "" {
			files[loopFile] = parseDataTaskCensusSource(t, loopSrc)["probe.go"]
		}
		return files
	}
	t.Run("fold_reads_result", func(t *testing.T) {
		files := seedFoldFiles(t, seedHeader+strings.Replace(greenFold, "values = append(values, rec.Plan.OutputContract)", "values = append(values, rec.Plan.OutputContract); if rec.Result != nil { values = append(values, rec.Result.OutputContract) }", 1), "", "")
		offenders := censusDataTaskOutputContractSeedFold(files)
		if len(offenders) != 2 || !strings.Contains(offenders[0], `reads OutputContract through "rec.Result.OutputContract"`) || !strings.Contains(offenders[1], "seed fold reads rec.Result —") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("fold_foreign_selector", func(t *testing.T) {
		files := seedFoldFiles(t, seedHeader+strings.Replace(greenFold, "rec.Plan.OutputContract", "rec.Admission.Plan.OutputContract", 1), "", "")
		offenders := censusDataTaskOutputContractSeedFold(files)
		if len(offenders) != 1 || !strings.Contains(offenders[0], `reads OutputContract through "rec.Admission.Plan.OutputContract", not a Plan-level declaration`) {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("baseline_local_snapshot", func(t *testing.T) {
		files := seedFoldFiles(t, seedHeader+strings.Replace(greenFold, "return dataTaskWorkflowOutputContract(view.Records, view.CurrentPlan)", "return view.CurrentPlan.OutputContract", 1), "", "")
		offenders := censusDataTaskOutputContractSeedFold(files)
		if len(offenders) != 1 || !strings.Contains(offenders[0], `gate baseline returns "view.CurrentPlan.OutputContract"`) {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("seed_not_from_fold", func(t *testing.T) {
		files := seedFoldFiles(t, seedHeader+greenFold, "repl.go", header+`
func loop(plan dataquery.TaskPlan) { durableOutputContract := plan.OutputContract; _ = durableOutputContract }
`)
		offenders := censusDataTaskOutputContractSeedFold(files)
		if len(offenders) != 1 || !strings.Contains(offenders[0], `repl.go: (h) durableOutputContract is seeded from "plan.OutputContract", not the seed fold`) {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("seed_missing", func(t *testing.T) {
		files := seedFoldFiles(t, seedHeader+greenFold, "data_task_cli.go", header+"func loop() {}\n")
		offenders := censusDataTaskOutputContractSeedFold(files)
		if len(offenders) != 1 || !strings.Contains(offenders[0], "data_task_cli.go: (h) durableOutputContract seeds=0") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("fold_missing", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, header+"func nothing() {}\n")
		offenders := censusDataTaskOutputContractSeedFold(files)
		if len(offenders) != 1 || !strings.Contains(offenders[0], "census walk drifted") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("seed_green_shape", func(t *testing.T) {
		files := seedFoldFiles(t, seedHeader+greenFold, "repl.go", header+`
func loop(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) { durableOutputContract := dataTaskWorkflowOutputContract(records, plan); _ = durableOutputContract }
`)
		if offenders := censusDataTaskOutputContractSeedFold(files); len(offenders) != 0 {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	// Rule (h2): declaration chains fold declarations only.
	chainHeader := seedHeader + greenFold
	t.Run("chain_reads_result_echo", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, chainHeader+`
func authority(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) dataquery.OutputContract {
	return dataworkflow.ResolveOutputContract(current.OutputContract, dataTaskWorkflowOutputContract(records, current), result.OutputContract)
}
`)
		offenders := censusDataTaskOutputContractDeclarationChains(files)
		if len(offenders) != 1 || !strings.Contains(offenders[0], `(h2) declaration chain folds "result.OutputContract"`) {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("chain_reads_record_result", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, chainHeader+`
func authority(rec dataTaskWorkflowRecord, answerResult *dataquery.Result) dataquery.OutputContract {
	return dataworkflow.ResolveOutputContract(rec.Plan.OutputContract, rec.Result.OutputContract, answerResult.OutputContract)
}
`)
		offenders := censusDataTaskOutputContractDeclarationChains(files)
		if len(offenders) != 2 || !strings.Contains(offenders[0], `folds "answerResult.OutputContract"`) || !strings.Contains(offenders[1], `folds "rec.Result.OutputContract"`) {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("chain_green_shapes", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, chainHeader+`
func carry(candidate dataquery.TaskPlan, durable dataquery.OutputContract, rec dataTaskWorkflowRecord, records []dataTaskWorkflowRecord) dataquery.OutputContract {
	return dataworkflow.ResolveOutputContract(durable, candidate.OutputContract, rec.Plan.OutputContract, dataTaskWorkflowOutputContract(records, candidate))
}
`)
		if offenders := censusDataTaskOutputContractDeclarationChains(files); len(offenders) != 0 {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("chain_walk_drifted", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, header+"func nothing() {}\n")
		offenders := censusDataTaskOutputContractDeclarationChains(files)
		if len(offenders) != 1 || !strings.Contains(offenders[0], "(h2) ResolveOutputContract calls=0 judged helpers=0") {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("judged_helper_result_last", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, strings.Replace(chainHeader, "dataworkflow.ResolveOutputContract(result.OutputContract, dataTaskWorkflowOutputContract(records, current))", "dataworkflow.ResolveOutputContract(dataTaskWorkflowOutputContract(records, current), result.OutputContract)", 1))
		offenders := censusDataTaskOutputContractDeclarationChains(files)
		if len(offenders) != 1 || !strings.Contains(offenders[0], `(h2) judged helper folds "result.OutputContract" at position 1 (fold at 0)`) {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("judged_helper_without_fold", func(t *testing.T) {
		files := parseDataTaskCensusSource(t, strings.Replace(chainHeader, "dataworkflow.ResolveOutputContract(result.OutputContract, dataTaskWorkflowOutputContract(records, current))", "dataworkflow.ResolveOutputContract(result.OutputContract, current.OutputContract)", 1))
		offenders := censusDataTaskOutputContractDeclarationChains(files)
		if len(offenders) != 1 || !strings.Contains(offenders[0], `(h2) judged helper folds "result.OutputContract" at position 0 (fold at -1)`) {
			t.Fatalf("offenders=%v", offenders)
		}
	})
	t.Run("judged_helper_green", func(t *testing.T) {
		if offenders := censusDataTaskOutputContractDeclarationChains(parseDataTaskCensusSource(t, chainHeader)); len(offenders) != 0 {
			t.Fatalf("offenders=%v", offenders)
		}
	})
}
