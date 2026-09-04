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
}
