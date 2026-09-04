package tool

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// answer_document_reject_exit_selector_census_test.go — §40.43 round-seven
// #3 (structural arm of TestPreDecodeRejectExitsResolveTheSelector).
//
// Ruling (§40.44 round-six #4, extended round-seven #2): every reject exit of
// the two answer-document executors whose payload is a JSON object resolves
// the optional root-cause selector first, so the deferred commit tail stages
// a valid selector (★16) or discloses + marks an invalid one. The behavioural
// pin enumerates today's twelve exits; this census makes the property hold
// for exits that do not exist yet: inside each executor body, every reject
// return (`return fail…(…)`) must be DOMINATED by an assignment
// `rootCauseSelection = resolveTraceRootCauseSelection…(…)` — an assignment
// earlier in the same statement list, or in an enclosing list before the
// branch that holds the return. Precise signals only: the variable name, the
// resolver-name prefix and the `fail` callee prefix; the one carve-out (the
// patch executor's nil-context guard — no ctx means nothing to stage on) is
// registered by the verbatim format string of its reject, and a registered
// carve-out that no longer exists is itself an offender.
//
// Self-red: dropping a resolve at a pre-decode exit, dropping the
// strict-decode resolve, adding a fresh reject exit before the decode, and a
// stale carve-out registration are each red on a mutated copy of the source.

const rejectExitSelectionVar = "rootCauseSelection"

var rejectExitSelectorResolverPrefix = "resolveTraceRootCauseSelection"

// rejectExitExecutors are the executor functions the census walks, keyed by
// file: the FuncDecl name (receiver-qualified for methods).
var rejectExitExecutors = map[string]string{
	"emit_answer_document_v2.go":    "executeAnswerDocumentV2",
	"emit_answer_document_patch.go": "EmitAnswerDocumentPatch.Execute",
}

// rejectExitCarveOuts registers, per file, the verbatim format strings of the
// reject exits that legitimately return without a resolve: the nil-context
// guard (nothing to stage on). A payload that is not a JSON object is not a
// separate exit — the raw resolver returns the zero value for it.
var rejectExitCarveOuts = map[string][]string{
	"emit_answer_document_patch.go": {"emit_answer_document_patch requires a writable context"},
}

type rejectExitCensus struct {
	exits           int // reject returns seen
	preResolveExits int // reject returns whose dominating resolve is a raw-params resolve (before the strict decode)
	resolves        int // resolve assignments seen
	offenders       []string
}

func rejectExitFuncName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return exprTypeName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

// rejectExitIsRejectReturn reports whether a return statement's first result
// is a call to a fail* helper (failEmit, failEmitWithRepair, failStrictDecode…).
func rejectExitIsRejectReturn(ret *ast.ReturnStmt) bool {
	if len(ret.Results) == 0 {
		return false
	}
	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	return ok && strings.HasPrefix(ident.Name, "fail")
}

// rejectExitFormatString returns the verbatim format-string argument of a
// reject call (the third argument of every fail* helper except
// failStrictDecode, whose "invalid params" prose is built inside).
func rejectExitFormatString(ret *ast.ReturnStmt) string {
	call := ret.Results[0].(*ast.CallExpr)
	for _, arg := range call.Args {
		if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			return strings.Trim(lit.Value, "\"`")
		}
	}
	return ""
}

// rejectExitIsResolve reports whether a statement is
// `rootCauseSelection = resolveTraceRootCauseSelection…(…)` and whether that
// resolve is the raw-params (pre-decode) one.
func rejectExitIsResolve(stmt ast.Stmt) (isResolve, raw bool) {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return false, false
	}
	lhs, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name != rejectExitSelectionVar {
		return false, false
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false, false
	}
	fn, ok := call.Fun.(*ast.Ident)
	if !ok || !strings.HasPrefix(fn.Name, rejectExitSelectorResolverPrefix) {
		return false, false
	}
	return true, strings.HasSuffix(fn.Name, "FromRawParams")
}

// auditRejectExitSelector walks one executor body. `resolved` is the
// domination state inherited from the enclosing statement list.
func auditRejectExitSelector(fset *token.FileSet, file string, fn *ast.FuncDecl, carveOuts map[string]bool) rejectExitCensus {
	var c rejectExitCensus
	pos := func(n ast.Node) string { return fset.Position(n.Pos()).String() }
	seenCarveOuts := map[string]bool{}
	var walkList func(list []ast.Stmt, resolved, rawResolved bool) (bool, bool)
	var walkStmt func(stmt ast.Stmt, resolved, rawResolved bool) (bool, bool)
	walkList = func(list []ast.Stmt, resolved, rawResolved bool) (bool, bool) {
		for _, stmt := range list {
			resolved, rawResolved = walkStmt(stmt, resolved, rawResolved)
		}
		return resolved, rawResolved
	}
	// walkBranch audits a nested scope that may not execute (branch / loop /
	// closure): its own resolves dominate only inside it.
	walkBranch := func(list []ast.Stmt, resolved, rawResolved bool) {
		walkList(list, resolved, rawResolved)
	}
	walkStmt = func(stmt ast.Stmt, resolved, rawResolved bool) (bool, bool) {
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			if isResolve, raw := rejectExitIsResolve(s); isResolve {
				c.resolves++
				return true, raw
			}
		case *ast.ReturnStmt:
			if !rejectExitIsRejectReturn(s) {
				return resolved, rawResolved
			}
			c.exits++
			if resolved {
				if rawResolved {
					c.preResolveExits++
				}
				return resolved, rawResolved
			}
			if format := rejectExitFormatString(s); carveOuts[format] {
				seenCarveOuts[format] = true
				return resolved, rawResolved
			}
			c.offenders = append(c.offenders, fmt.Sprintf("%s %s: reject exit is not dominated by a `%s = %s…(…)` assignment — a selector riding this reject would be lost (§40.44 round-six #4 / round-seven #2: every reject exit whose payload is a JSON object resolves the selector; register a nil-context guard by its verbatim format string)",
				pos(s), rejectExitFuncName(fn), rejectExitSelectionVar, rejectExitSelectorResolverPrefix))
			return resolved, rawResolved
		case *ast.BlockStmt:
			// A bare block always executes: its resolves dominate what follows.
			return walkList(s.List, resolved, rawResolved)
		case *ast.LabeledStmt:
			return walkStmt(s.Stmt, resolved, rawResolved)
		case *ast.IfStmt:
			if s.Init != nil {
				resolved, rawResolved = walkStmt(s.Init, resolved, rawResolved)
			}
			walkBranch(s.Body.List, resolved, rawResolved)
			if s.Else != nil {
				walkStmt(s.Else, resolved, rawResolved)
			}
			return resolved, rawResolved
		case *ast.ForStmt:
			if s.Init != nil {
				resolved, rawResolved = walkStmt(s.Init, resolved, rawResolved)
			}
			walkBranch(s.Body.List, resolved, rawResolved)
			return resolved, rawResolved
		case *ast.RangeStmt:
			walkBranch(s.Body.List, resolved, rawResolved)
			return resolved, rawResolved
		case *ast.SwitchStmt:
			if s.Init != nil {
				resolved, rawResolved = walkStmt(s.Init, resolved, rawResolved)
			}
			for _, clause := range s.Body.List {
				walkBranch(clause.(*ast.CaseClause).Body, resolved, rawResolved)
			}
			return resolved, rawResolved
		case *ast.TypeSwitchStmt:
			if s.Init != nil {
				resolved, rawResolved = walkStmt(s.Init, resolved, rawResolved)
			}
			for _, clause := range s.Body.List {
				walkBranch(clause.(*ast.CaseClause).Body, resolved, rawResolved)
			}
			return resolved, rawResolved
		case *ast.SelectStmt:
			for _, clause := range s.Body.List {
				walkBranch(clause.(*ast.CommClause).Body, resolved, rawResolved)
			}
			return resolved, rawResolved
		}
		// Closures (defer / go / expression / assignment operands) inherit the
		// domination state at their definition point; a reject return inside
		// one is audited like any other.
		ast.Inspect(stmt, func(n ast.Node) bool {
			if lit, ok := n.(*ast.FuncLit); ok {
				walkBranch(lit.Body.List, resolved, rawResolved)
				return false
			}
			return true
		})
		return resolved, rawResolved
	}
	walkList(fn.Body.List, false, false)
	for format := range carveOuts {
		if !seenCarveOuts[format] {
			c.offenders = append(c.offenders, fmt.Sprintf("%s %s: registered carve-out %q no longer names a reject exit; update the table", file, rejectExitFuncName(fn), format))
		}
	}
	return c
}

// rejectExitCensusOver runs the census over a set of file sources.
func rejectExitCensusOver(t *testing.T, srcs map[string]string) map[string]rejectExitCensus {
	t.Helper()
	out := map[string]rejectExitCensus{}
	fset := token.NewFileSet()
	for file, want := range rejectExitExecutors {
		src, ok := srcs[file]
		if !ok {
			t.Fatalf("census input lacks %s", file)
		}
		f, err := parser.ParseFile(fset, file, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		carveOuts := map[string]bool{}
		for _, format := range rejectExitCarveOuts[file] {
			carveOuts[format] = true
		}
		found := false
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || rejectExitFuncName(fn) != want {
				continue
			}
			found = true
			out[file] = auditRejectExitSelector(fset, file, fn, carveOuts)
		}
		if !found {
			t.Fatalf("%s: executor %s not found — the census lost its subject", file, want)
		}
	}
	return out
}

func rejectExitLiveSources(t *testing.T) map[string]string {
	t.Helper()
	srcs := map[string]string{}
	for file := range rejectExitExecutors {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		srcs[file] = string(src)
	}
	return srcs
}

func TestAnswerDocumentRejectExitsResolveTheSelectorCensus(t *testing.T) {
	live := rejectExitLiveSources(t)
	results := rejectExitCensusOver(t, live)
	for file, c := range results {
		for _, o := range c.offenders {
			t.Errorf("%s: %s", file, o)
		}
	}
	// Vacuity guards: the executors carry the enumerated exits — full-emit
	// four pre-decode + the strict decode (five raw-resolved), patch six
	// pre-decode + the strict decode (seven raw-resolved) plus the carve-out —
	// and at least one resolve each.
	if c := results["emit_answer_document_v2.go"]; c.preResolveExits < 5 || c.resolves < 2 {
		t.Fatalf("full-emit executor: expected >= 5 raw-resolved reject exits and >= 2 resolves, got %+v", c)
	}
	if c := results["emit_answer_document_patch.go"]; c.preResolveExits < 7 || c.resolves < 2 || c.exits < 8 {
		t.Fatalf("patch executor: expected >= 7 raw-resolved reject exits (plus the nil-context carve-out) and >= 2 resolves, got %+v", c)
	}

	mutate := func(t *testing.T, file, old, new string) map[string]string {
		t.Helper()
		out := map[string]string{}
		for k, v := range live {
			out[k] = v
		}
		if !strings.Contains(out[file], old) {
			t.Fatalf("self-red anchor %q not found in %s", old, file)
		}
		out[file] = strings.Replace(out[file], old, new, 1)
		return out
	}
	expect := func(t *testing.T, results map[string]rejectExitCensus, file, want string) {
		t.Helper()
		for _, o := range results[file].offenders {
			if strings.Contains(o, want) {
				return
			}
		}
		t.Fatalf("self-red shape not reported in %s (want %q); offenders: %v", file, want, results[file].offenders)
	}
	const rawResolveV2 = "rootCauseSelection = resolveTraceRootCauseSelectionFromRawParams(ctx, carriers, raw, false)"
	const rawResolvePatch = "rootCauseSelection = resolveTraceRootCauseSelectionFromRawParams(ctx, carriers, params, true)"
	t.Run("self_red_full_emit_pre_decode_exit_without_resolve", func(t *testing.T) {
		// The carrier-corruption exit (one of the three round-six-unpinned
		// exits) loses its resolve line.
		src := mutate(t, "emit_answer_document_v2.go",
			rawResolveV2+" // §40.43 round-six #4\n\t\treturn failEmitWithRepair(toolName, now, answerDocumentStructuralCarrierCorruptionRepair(paths),",
			"return failEmitWithRepair(toolName, now, answerDocumentStructuralCarrierCorruptionRepair(paths),")
		expect(t, rejectExitCensusOver(t, src), "emit_answer_document_v2.go", "reject exit is not dominated")
	})
	t.Run("self_red_patch_pre_decode_exit_without_resolve", func(t *testing.T) {
		src := mutate(t, "emit_answer_document_patch.go",
			rawResolvePatch+" // §40.43 round-six #4\n\t\treturn failEmitWithRepair(t.Name(), now, answerDocumentStructuralCarrierCorruptionRepair(paths),",
			"return failEmitWithRepair(t.Name(), now, answerDocumentStructuralCarrierCorruptionRepair(paths),")
		expect(t, rejectExitCensusOver(t, src), "emit_answer_document_patch.go", "reject exit is not dominated")
	})
	t.Run("self_red_strict_decode_exit_without_resolve", func(t *testing.T) {
		src := mutate(t, "emit_answer_document_v2.go",
			rawResolveV2+"\n\t\treturn failStrictDecode(", "return failStrictDecode(")
		expect(t, rejectExitCensusOver(t, src), "emit_answer_document_v2.go", "reject exit is not dominated")
		src = mutate(t, "emit_answer_document_patch.go",
			rawResolvePatch+"\n\t\treturn failStrictDecode(", "return failStrictDecode(")
		expect(t, rejectExitCensusOver(t, src), "emit_answer_document_patch.go", "reject exit is not dominated")
	})
	t.Run("self_red_new_exit_before_the_decode", func(t *testing.T) {
		src := mutate(t, "emit_answer_document_v2.go",
			"\tdec := json.NewDecoder(bytes.NewReader(raw))",
			"\tif len(raw) > 1<<20 {\n\t\treturn failEmit(toolName, now, \"payload too large\")\n\t}\n\tdec := json.NewDecoder(bytes.NewReader(raw))")
		expect(t, rejectExitCensusOver(t, src), "emit_answer_document_v2.go", "reject exit is not dominated")
	})
	t.Run("self_red_resolve_inside_a_branch_does_not_dominate_a_later_exit", func(t *testing.T) {
		src := mutate(t, "emit_answer_document_v2.go",
			"\tdec := json.NewDecoder(bytes.NewReader(raw))",
			"\tif len(raw) > 1<<20 {\n\t\t"+rawResolveV2+"\n\t}\n\tif len(raw) > 1<<21 {\n\t\treturn failEmit(toolName, now, \"payload too large\")\n\t}\n\tdec := json.NewDecoder(bytes.NewReader(raw))")
		expect(t, rejectExitCensusOver(t, src), "emit_answer_document_v2.go", "reject exit is not dominated")
	})
	t.Run("self_red_stale_carve_out", func(t *testing.T) {
		src := mutate(t, "emit_answer_document_patch.go",
			"emit_answer_document_patch requires a writable context", "emit_answer_document_patch requires a writable ctx")
		// The renamed guard is no longer registered: it is an undominated
		// exit AND the registration is stale.
		results := rejectExitCensusOver(t, src)
		expect(t, results, "emit_answer_document_patch.go", "reject exit is not dominated")
		expect(t, results, "emit_answer_document_patch.go", "no longer names a reject exit")
	})
}
