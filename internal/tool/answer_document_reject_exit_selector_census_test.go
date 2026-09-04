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

// answer_document_reject_exit_selector_census_test.go — §40.44 round-seven
// #3 (structural arm of TestPreDecodeRejectExitsResolveTheSelector), closed
// by §40.44 round-eight #0/#7.
//
// Ruling (§40.44 round-six #4, extended round-seven #2): every reject exit of
// the two answer-document executors whose payload is a JSON object resolves
// the optional root-cause selector first, so the deferred commit tail stages
// a valid selector (★16) or discloses + marks an invalid one. The behavioural
// pin enumerates today's twelve exits; this census makes the property hold
// for exits that do not exist yet.
//
// Model (§40.44 round-eight #0). Inside each executor body, BEFORE the
// accept-lane resolve (`rootCauseSelection = resolveTraceRootCauseSelectionForEmit(…)`)
// no non-reject exit exists — the executor cannot succeed before it has
// decoded — so EVERY exit positioned there is a reject exit: every return
// statement whatever its shape, and every `=` assignment to a named result
// (a defer/recover guard sets results without a return statement). Each must
// be DOMINATED by an assignment `rootCauseSelection = resolveTraceRootCauseSelection…(…)`
// earlier in the same statement list, or in an enclosing list before the
// branch that holds the exit (a resolve inside a branch / loop / closure
// dominates only its own body). EVOLUTION RECORD: the round-seven walker
// recognized only `return fail…(…)` with a bare Ident callee and silently
// skipped every other return — bare return over named results, two-step
// `res, e := failEmit(…); return res, e`, a defer/recover assigning the named
// results, a wrapped `return withNote(failEmit(…))`, a method-helper or
// package-qualified reject, a plain `return types.ToolResult{}, err` — all
// uncounted, undominated, green (overlay probe: exits=9 offenders=0 for each
// of seven shapes). Now every exit is audited and an exit shape the roster
// cannot key FAILS LOUD (the round-six ruling: fail-loud on unrecognized
// shapes ends the evasion enumeration).
//
// Roster (§40.44 round-eight #7). The `>=` vacuity floors are replaced by an
// EXACT registered roster of the exits that are not accept-lane-dominated:
// (file, executor, verbatim reject callee, verbatim message prefix, lane).
// Every such exit must match exactly one row and every row exactly one exit;
// a new, merged, renamed or removed exit is red until the roster says so. The
// one undominated lane (the patch executor's nil-context guard — no ctx means
// nothing to stage on) is a roster row with lane carve-out; a carve-out row
// that no longer names an exit is red.
//
// Named-result passthroughs: the deferred `result = carriers.finalize(result)`
// / `result = annotateAnswerDocumentPatchFailureOutcome(result, …)` transform
// whatever exit already happened; they are registered by verbatim callee and
// must take the named result as an argument. An unregistered named-result
// assignment is an exit; a registered passthrough that disappears is red.

const rejectExitSelectionVar = "rootCauseSelection"

var rejectExitSelectorResolverPrefix = "resolveTraceRootCauseSelection"

// rejectExitAcceptLaneResolver is the resolve after the strict decode; the
// pre-accept region of an executor ends at its top-level occurrence.
const rejectExitAcceptLaneResolver = "resolveTraceRootCauseSelectionForEmit"

// rejectExitExecutors are the executor functions the census walks, keyed by
// file: the FuncDecl name (receiver-qualified for methods).
var rejectExitExecutors = map[string]string{
	"emit_answer_document_v2.go":    "executeAnswerDocumentV2",
	"emit_answer_document_patch.go": "EmitAnswerDocumentPatch.Execute",
}

// rejectExitLane says how a rostered exit satisfies the ruling.
type rejectExitLane string

const (
	// rejectExitLaneRaw: dominated by the raw-params resolve (pre-decode and
	// strict-decode exits).
	rejectExitLaneRaw rejectExitLane = "raw"
	// rejectExitLaneCarveOut: undominated by design (nil-context guard).
	rejectExitLaneCarveOut rejectExitLane = "carve-out"
)

// rejectExitRow is one registered reject exit: the verbatim fail* callee and
// a verbatim prefix of its literal format string (long enough to be unique
// inside the executor; failStrictDecode builds its prose inside and carries
// no literal).
type rejectExitRow struct {
	callee  string
	message string
	lane    rejectExitLane
}

// rejectExitRoster is the exact registered roster per file, in source order.
var rejectExitRoster = map[string][]rejectExitRow{
	"emit_answer_document_v2.go": {
		{"failEmit", "top-level field %q is not accepted; the answer is expressed through blocks[] only", rejectExitLaneRaw},
		{"failEmit", "top-level field %q is not accepted; place the exact typed claim object(s) under blocks[i].relation_claims", rejectExitLaneRaw},
		{"failEmitWithRepair", "answer_document carrier contains serialized JSON boundary text", rejectExitLaneRaw},
		{"failEmitWithRepair", "recovered blocks[] does not match the current dispatch schema", rejectExitLaneRaw},
		{"failStrictDecode", "", rejectExitLaneRaw},
	},
	"emit_answer_document_patch.go": {
		{"failEmit", "emit_answer_document_patch requires a writable context", rejectExitLaneCarveOut},
		{"failEmit", "emit_answer_document_patch: no previous emit found", rejectExitLaneRaw},
		{"failEmit", "top-level field %q is not accepted; place the exact typed claim object(s) under replace_blocks[i].relation_claims", rejectExitLaneRaw},
		{"failEmitWithRepair", "answer_document patch carrier contains serialized JSON boundary text", rejectExitLaneRaw},
		{"failEmitWithRepair", "answer_document patch placed a block operation in replace_snippets", rejectExitLaneRaw},
		{"failEmitWithRepair", "block_field_edits_v1[%d] does not match any exact field-edit branch", rejectExitLaneRaw},
		{"failEmitWithRepair", "block_receipt_edits_v1[%d] does not match any exact receipt-edit branch", rejectExitLaneRaw},
		{"failStrictDecode", "", rejectExitLaneRaw},
	},
}

// rejectExitPassthroughs registers, per file, the callees of the deferred
// named-result passthrough assignments (`result = <callee>(result, …)`).
var rejectExitPassthroughs = map[string][]string{
	"emit_answer_document_v2.go":    {"finalize"},
	"emit_answer_document_patch.go": {"finalize", "annotateAnswerDocumentPatchFailureOutcome"},
}

// rejectExitSeen is one exit found in the pre-accept region.
type rejectExitSeen struct {
	pos     string
	callee  string // "" when the shape is not a recognized `return fail…(…)`
	message string
	lane    rejectExitLane
	shape   string // "" when recognized, else why the roster cannot key it
}

type rejectExitCensus struct {
	exits           []rejectExitSeen // exits in the pre-accept region, source order
	postAcceptExits int              // returns dominated by the accept-lane resolve
	resolves        int              // resolve assignments seen
	acceptResolves  int              // accept-lane resolves at the top level of the body
	offenders       []string
}

func rejectExitFuncName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return exprTypeName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

// rejectExitNamedResults returns the executor's named result identifiers.
func rejectExitNamedResults(fn *ast.FuncDecl) map[string]bool {
	out := map[string]bool{}
	if fn.Type.Results == nil {
		return out
	}
	for _, field := range fn.Type.Results.List {
		for _, name := range field.Names {
			out[name.Name] = true
		}
	}
	return out
}

// rejectExitRecognize keys a return statement for the roster: a single
// result that is a call to a bare `fail…` helper whose format string is a
// literal (failStrictDecode carries none). Every other shape is reported
// verbatim as the reason the roster cannot key it.
func rejectExitRecognize(ret *ast.ReturnStmt) (callee, message, shape string) {
	if len(ret.Results) == 0 {
		return "", "", "bare `return` over the named results"
	}
	if len(ret.Results) != 1 {
		return "", "", fmt.Sprintf("%d-value return", len(ret.Results))
	}
	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok {
		return "", "", fmt.Sprintf("returns a %T, not a reject call", ret.Results[0])
	}
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return "", "", fmt.Sprintf("returns a call through a %T callee, not a bare fail* helper", call.Fun)
	}
	if !strings.HasPrefix(ident.Name, "fail") {
		return "", "", "returns a call to " + ident.Name + ", not a fail* helper"
	}
	for _, arg := range call.Args {
		if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			return ident.Name, strings.Trim(lit.Value, "\"`"), ""
		}
	}
	if ident.Name == "failStrictDecode" {
		return ident.Name, "", ""
	}
	return "", "", ident.Name + " without a literal format string"
}

// rejectExitIsResolve reports whether a statement is
// `rootCauseSelection = resolveTraceRootCauseSelection…(…)` and whether that
// resolve is the raw-params (pre-decode) one.
func rejectExitIsResolve(stmt ast.Stmt) (isResolve, raw, accept bool) {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return false, false, false
	}
	lhs, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name != rejectExitSelectionVar {
		return false, false, false
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false, false, false
	}
	fn, ok := call.Fun.(*ast.Ident)
	if !ok || !strings.HasPrefix(fn.Name, rejectExitSelectorResolverPrefix) {
		return false, false, false
	}
	return true, strings.HasSuffix(fn.Name, "FromRawParams"), fn.Name == rejectExitAcceptLaneResolver
}

// rejectExitNamedResultAssign reports whether an assignment writes a named
// result with `=` (a `:=` declares a shadow, never the result), and whether
// it is a registered passthrough `result = <callee>(…, result, …)`.
func rejectExitNamedResultAssign(assign *ast.AssignStmt, named map[string]bool, passthroughs map[string]bool) (writes bool, passthrough string) {
	if assign.Tok != token.ASSIGN {
		return false, ""
	}
	for _, lhs := range assign.Lhs {
		if id, ok := lhs.(*ast.Ident); ok && named[id.Name] {
			writes = true
		}
	}
	if !writes || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return writes, ""
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return writes, ""
	}
	var callee string
	switch f := call.Fun.(type) {
	case *ast.Ident:
		callee = f.Name
	case *ast.SelectorExpr:
		callee = f.Sel.Name
	}
	if !passthroughs[callee] {
		return writes, ""
	}
	target := assign.Lhs[0].(*ast.Ident).Name
	for _, arg := range call.Args {
		if id, ok := arg.(*ast.Ident); ok && id.Name == target {
			return writes, callee
		}
	}
	return writes, ""
}

// auditRejectExitSelector walks one executor body. `resolved` is the
// domination state inherited from the enclosing statement list.
func auditRejectExitSelector(fset *token.FileSet, file string, fn *ast.FuncDecl, roster []rejectExitRow, passthroughs []string) rejectExitCensus {
	var c rejectExitCensus
	pos := func(n ast.Node) string { return fset.Position(n.Pos()).String() }
	named := rejectExitNamedResults(fn)
	passthroughSet := map[string]bool{}
	seenPassthrough := map[string]bool{}
	for _, callee := range passthroughs {
		passthroughSet[callee] = true
	}
	// exit records an exit found in the walk; the accept-lane-dominated
	// region is not audited (the accept-lane resolve dominates it).
	exit := func(n ast.Node, resolved, rawResolved bool, callee, message, shape string) {
		if resolved && !rawResolved {
			c.postAcceptExits++
			return
		}
		seen := rejectExitSeen{pos: pos(n), callee: callee, message: message, shape: shape}
		if resolved {
			seen.lane = rejectExitLaneRaw
		} else {
			seen.lane = rejectExitLaneCarveOut
		}
		c.exits = append(c.exits, seen)
		if shape != "" {
			c.offenders = append(c.offenders, fmt.Sprintf("%s %s: unrecognized reject exit shape before the accept-lane resolve (%s); the exit roster cannot key it — spell the reject as `return fail…(…)` with a literal format string (§40.44 round-eight #0: every exit before the accept-lane resolve is a reject exit; unrecognized shapes fail loud)",
				pos(n), rejectExitFuncName(fn), shape))
		}
		if resolved {
			return
		}
		for _, row := range roster {
			if row.lane == rejectExitLaneCarveOut && row.callee == callee && shape == "" && strings.HasPrefix(message, row.message) {
				return
			}
		}
		c.offenders = append(c.offenders, fmt.Sprintf("%s %s: reject exit is not dominated by a `%s = %s…(…)` assignment — a selector riding this reject would be lost (§40.44 round-six #4 / round-seven #2 / round-eight #0: every exit before the accept-lane resolve is a reject exit that resolves the selector; register a nil-context guard as a carve-out roster row)",
			pos(n), rejectExitFuncName(fn), rejectExitSelectionVar, rejectExitSelectorResolverPrefix))
	}
	var walkList func(list []ast.Stmt, resolved, rawResolved bool, top bool) (bool, bool)
	var walkStmt func(stmt ast.Stmt, resolved, rawResolved bool, top bool) (bool, bool)
	walkList = func(list []ast.Stmt, resolved, rawResolved bool, top bool) (bool, bool) {
		for _, stmt := range list {
			resolved, rawResolved = walkStmt(stmt, resolved, rawResolved, top)
		}
		return resolved, rawResolved
	}
	// walkBranch audits a nested scope that may not execute (branch / loop /
	// closure): its own resolves dominate only inside it.
	walkBranch := func(list []ast.Stmt, resolved, rawResolved bool) {
		walkList(list, resolved, rawResolved, false)
	}
	walkStmt = func(stmt ast.Stmt, resolved, rawResolved bool, top bool) (bool, bool) {
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			if isResolve, raw, accept := rejectExitIsResolve(s); isResolve {
				c.resolves++
				if accept && top {
					c.acceptResolves++
				}
				return true, raw
			}
			if writes, passthrough := rejectExitNamedResultAssign(s, named, passthroughSet); writes {
				if passthrough != "" {
					seenPassthrough[passthrough] = true
				} else {
					exit(s, resolved, rawResolved, "", "", "assigns the named result(s) outside a registered passthrough")
				}
			}
		case *ast.ReturnStmt:
			callee, message, shape := rejectExitRecognize(s)
			exit(s, resolved, rawResolved, callee, message, shape)
			return resolved, rawResolved
		case *ast.BlockStmt:
			// A bare block always executes: its resolves dominate what follows.
			return walkList(s.List, resolved, rawResolved, false)
		case *ast.LabeledStmt:
			return walkStmt(s.Stmt, resolved, rawResolved, top)
		case *ast.IfStmt:
			if s.Init != nil {
				resolved, rawResolved = walkStmt(s.Init, resolved, rawResolved, false)
			}
			walkBranch(s.Body.List, resolved, rawResolved)
			if s.Else != nil {
				walkStmt(s.Else, resolved, rawResolved, false)
			}
			return resolved, rawResolved
		case *ast.ForStmt:
			if s.Init != nil {
				resolved, rawResolved = walkStmt(s.Init, resolved, rawResolved, false)
			}
			walkBranch(s.Body.List, resolved, rawResolved)
			return resolved, rawResolved
		case *ast.RangeStmt:
			walkBranch(s.Body.List, resolved, rawResolved)
			return resolved, rawResolved
		case *ast.SwitchStmt:
			if s.Init != nil {
				resolved, rawResolved = walkStmt(s.Init, resolved, rawResolved, false)
			}
			for _, clause := range s.Body.List {
				walkBranch(clause.(*ast.CaseClause).Body, resolved, rawResolved)
			}
			return resolved, rawResolved
		case *ast.TypeSwitchStmt:
			if s.Init != nil {
				resolved, rawResolved = walkStmt(s.Init, resolved, rawResolved, false)
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
		// domination state at their definition point; an exit inside one — a
		// return, or a named-result assignment (the defer/recover guard shape)
		// — is audited like any other.
		ast.Inspect(stmt, func(n ast.Node) bool {
			if lit, ok := n.(*ast.FuncLit); ok {
				walkBranch(lit.Body.List, resolved, rawResolved)
				return false
			}
			return true
		})
		return resolved, rawResolved
	}
	endResolved, endRaw := walkList(fn.Body.List, false, false, true)
	if c.acceptResolves != 1 || !endResolved || endRaw {
		c.offenders = append(c.offenders, fmt.Sprintf("%s %s: expected exactly one top-level `%s = %s(…)` (the accept-lane resolve that ends the reject-only region), found %d", file, rejectExitFuncName(fn), rejectExitSelectionVar, rejectExitAcceptLaneResolver, c.acceptResolves))
	}
	for _, callee := range passthroughs {
		if !seenPassthrough[callee] {
			c.offenders = append(c.offenders, fmt.Sprintf("%s %s: registered named-result passthrough %q is no longer assigned as `result = %s(…, result, …)`; update the table", file, rejectExitFuncName(fn), callee, callee))
		}
	}
	// Exact roster reconciliation: every recognized pre-accept exit matches
	// exactly one row (same callee, message prefix, lane) and every row
	// exactly one exit.
	matched := make([]int, len(roster))
	for _, seen := range c.exits {
		if seen.shape != "" {
			continue
		}
		hits := 0
		for i, row := range roster {
			if row.callee == seen.callee && strings.HasPrefix(seen.message, row.message) {
				if row.lane != seen.lane {
					c.offenders = append(c.offenders, fmt.Sprintf("%s %s: reject exit %s(%q…) is registered on lane %s but observed on lane %s; update the roster", seen.pos, rejectExitFuncName(fn), seen.callee, seen.message, row.lane, seen.lane))
				}
				matched[i]++
				hits++
			}
		}
		switch {
		case hits == 0:
			c.offenders = append(c.offenders, fmt.Sprintf("%s %s: reject exit %s(%q…) is not registered in the exit roster; register it (file, executor, verbatim callee, message prefix, lane) (§40.44 round-eight #7)", seen.pos, rejectExitFuncName(fn), seen.callee, seen.message))
		case hits > 1:
			c.offenders = append(c.offenders, fmt.Sprintf("%s %s: reject exit %s(%q…) matches %d roster rows; make the registered prefixes unique", seen.pos, rejectExitFuncName(fn), seen.callee, seen.message, hits))
		}
	}
	for i, row := range roster {
		switch matched[i] {
		case 0:
			c.offenders = append(c.offenders, fmt.Sprintf("%s %s: registered exit %s(%q…) [%s] no longer names a reject exit; update the roster", file, rejectExitFuncName(fn), row.callee, row.message, row.lane))
		case 1:
		default:
			c.offenders = append(c.offenders, fmt.Sprintf("%s %s: registered exit %s(%q…) matches %d exits; make the registered prefix unique", file, rejectExitFuncName(fn), row.callee, row.message, matched[i]))
		}
	}
	return c
}

// rejectExitCensusOver runs the census over a set of file sources.
func rejectExitCensusOver(t *testing.T, srcs map[string]string) map[string]rejectExitCensus {
	t.Helper()
	return rejectExitCensusOverRoster(t, srcs, rejectExitRoster, rejectExitPassthroughs)
}

func rejectExitCensusOverRoster(t *testing.T, srcs map[string]string, roster map[string][]rejectExitRow, passthroughs map[string][]string) map[string]rejectExitCensus {
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
		found := false
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || rejectExitFuncName(fn) != want {
				continue
			}
			found = true
			out[file] = auditRejectExitSelector(fset, file, fn, roster[file], passthroughs[file])
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
		// The roster is exact: the pre-accept exits ARE the roster, in
		// number as well as identity (reconciled above), and every one is
		// recognized.
		if got, want := len(c.exits), len(rejectExitRoster[file]); got != want {
			t.Errorf("%s: %d pre-accept reject exits, roster has %d rows", file, got, want)
		}
		if c.postAcceptExits == 0 {
			t.Errorf("%s: no accept-lane-dominated return seen — the walker lost the body after the accept-lane resolve", file)
		}
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
	const decodeAnchorV2 = "\tdec := json.NewDecoder(bytes.NewReader(raw))"
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
		src := mutate(t, "emit_answer_document_v2.go", decodeAnchorV2,
			"\tif len(raw) > 1<<20 {\n\t\treturn failEmit(toolName, now, \"payload too large\")\n\t}\n"+decodeAnchorV2)
		results := rejectExitCensusOver(t, src)
		expect(t, results, "emit_answer_document_v2.go", "reject exit is not dominated")
		expect(t, results, "emit_answer_document_v2.go", "is not registered in the exit roster")
	})
	t.Run("self_red_resolve_inside_a_branch_does_not_dominate_a_later_exit", func(t *testing.T) {
		src := mutate(t, "emit_answer_document_v2.go", decodeAnchorV2,
			"\tif len(raw) > 1<<20 {\n\t\t"+rawResolveV2+"\n\t}\n\tif len(raw) > 1<<21 {\n\t\treturn failEmit(toolName, now, \"payload too large\")\n\t}\n"+decodeAnchorV2)
		expect(t, rejectExitCensusOver(t, src), "emit_answer_document_v2.go", "reject exit is not dominated")
	})

	// §40.44 round-eight #0: EVERY exit before the accept-lane resolve is a
	// reject exit — the seven shapes below were uncounted (exits=9,
	// offenders=0) on 154b1a5c5; each is now undominated AND fails loud as a
	// shape the roster cannot key.
	shapes := map[string]string{
		"bare_return_over_named_results":             "\tif len(raw) > 1<<20 {\n\t\tresult, err = failEmit(toolName, now, \"payload too large\")\n\t\treturn\n\t}\n",
		"two_step_return":                            "\tif len(raw) > 1<<20 {\n\t\tres, e := failEmit(toolName, now, \"payload too large\")\n\t\treturn res, e\n\t}\n",
		"two_step_return_shadowing_the_named_result": "\tif len(raw) > 1<<20 {\n\t\tresult, resultErr := failEmit(toolName, now, \"payload too large\")\n\t\treturn result, resultErr\n\t}\n",
		"defer_recover_assigning_named_results":      "\tdefer func() {\n\t\tif r := recover(); r != nil {\n\t\t\tresult, err = failEmit(toolName, now, \"panic: %v\", r)\n\t\t}\n\t}()\n",
		"wrapped_reject_call":                        "\tif len(raw) > 1<<20 {\n\t\treturn withNote(failEmit(toolName, now, \"payload too large\"))\n\t}\n",
		"method_helper_reject":                       "\tif len(raw) > 1<<20 {\n\t\treturn carriers.reject(toolName, now, \"payload too large\")\n\t}\n",
		"package_qualified_reject":                   "\tif len(raw) > 1<<20 {\n\t\treturn toolparam.FailEmit(toolName, now, \"payload too large\")\n\t}\n",
		"plain_error_return":                         "\tif len(raw) > 1<<20 {\n\t\treturn types.ToolResult{}, fmt.Errorf(\"payload too large\")\n\t}\n",
	}
	for name, insert := range shapes {
		t.Run("self_red_shape_"+name, func(t *testing.T) {
			results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go", decodeAnchorV2, insert+decodeAnchorV2))
			expect(t, results, "emit_answer_document_v2.go", "reject exit is not dominated")
			expect(t, results, "emit_answer_document_v2.go", "unrecognized reject exit shape")
		})
	}
	t.Run("self_red_dominated_but_unrecognized_shape_fails_loud", func(t *testing.T) {
		// Fail-loud is independent of domination: a resolved two-step reject
		// is still a shape the roster cannot key.
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go", decodeAnchorV2,
			"\tif len(raw) > 1<<20 {\n\t\t"+rawResolveV2+"\n\t\tres, e := failEmit(toolName, now, \"payload too large\")\n\t\treturn res, e\n\t}\n"+decodeAnchorV2))
		expect(t, results, "emit_answer_document_v2.go", "unrecognized reject exit shape")
		for _, o := range results["emit_answer_document_v2.go"].offenders {
			if strings.Contains(o, "reject exit is not dominated") {
				t.Fatalf("a dominated exit must not be reported as undominated: %s", o)
			}
		}
	})
	t.Run("self_red_dominated_new_exit_is_unregistered", func(t *testing.T) {
		// §40.44 round-eight #7: a dominated new exit is red until rostered.
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go", decodeAnchorV2,
			"\tif len(raw) > 1<<20 {\n\t\t"+rawResolveV2+"\n\t\treturn failEmit(toolName, now, \"payload too large\")\n\t}\n"+decodeAnchorV2))
		expect(t, results, "emit_answer_document_v2.go", "is not registered in the exit roster")
	})
	t.Run("self_red_registered_exit_message_drift", func(t *testing.T) {
		// A renamed message is a merged/removed exit to the roster: the row
		// is stale AND the exit is unregistered.
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go",
			"\"top-level field %q is not accepted; the answer is expressed through blocks[] only",
			"\"top-level field %q is refused; the answer is expressed through blocks[] only"))
		expect(t, results, "emit_answer_document_v2.go", "no longer names a reject exit")
		expect(t, results, "emit_answer_document_v2.go", "is not registered in the exit roster")
	})
	t.Run("self_red_merged_exits_leave_a_stale_row", func(t *testing.T) {
		// Folding the relation_claims reject into the v1-fields message leaves
		// the relation_claims row without an exit and the v1-fields row with
		// two.
		src := mutate(t, "emit_answer_document_v2.go",
			"\t\treturn failEmit(toolName, now,\n\t\t\t\"top-level field %q is not accepted; place the exact typed claim object(s) under blocks[i].relation_claims",
			"\t\treturn failEmit(toolName, now,\n\t\t\t\"top-level field %q is not accepted; the answer is expressed through blocks[] only — relation_claims")
		results := rejectExitCensusOver(t, src)
		expect(t, results, "emit_answer_document_v2.go", "no longer names a reject exit")
		expect(t, results, "emit_answer_document_v2.go", "matches 2 exits")
	})
	t.Run("self_red_stale_roster_row", func(t *testing.T) {
		roster := map[string][]rejectExitRow{}
		for k, v := range rejectExitRoster {
			roster[k] = append([]rejectExitRow{}, v...)
		}
		roster["emit_answer_document_v2.go"] = append(roster["emit_answer_document_v2.go"], rejectExitRow{"failEmit", "ghost exit", rejectExitLaneRaw})
		results := rejectExitCensusOverRoster(t, live, roster, rejectExitPassthroughs)
		expect(t, results, "emit_answer_document_v2.go", "registered exit failEmit(\"ghost exit\"…) [raw] no longer names a reject exit")
	})
	t.Run("self_red_lane_drift", func(t *testing.T) {
		// The carve-out gains a resolve: its row must move to the raw lane.
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_patch.go",
			"\tif ctx == nil || ctx.Mutable == nil {\n\t\treturn failEmit(t.Name(), now,\n\t\t\t\"emit_answer_document_patch requires a writable context\")",
			"\tif ctx == nil || ctx.Mutable == nil {\n\t\t"+rawResolvePatch+"\n\t\treturn failEmit(t.Name(), now,\n\t\t\t\"emit_answer_document_patch requires a writable context\")"))
		expect(t, results, "emit_answer_document_patch.go", "registered on lane carve-out but observed on lane raw")
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
	t.Run("self_red_unregistered_named_result_passthrough", func(t *testing.T) {
		// A renamed passthrough is an unregistered named-result assignment
		// before any resolve (an exit: undominated, unrecognized) and a stale
		// passthrough registration.
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go",
			"defer func() { result = carriers.finalize(result) }()", "defer func() { result = carriers.finalise(result) }()"))
		expect(t, results, "emit_answer_document_v2.go", "assigns the named result(s) outside a registered passthrough")
		expect(t, results, "emit_answer_document_v2.go", "reject exit is not dominated")
		expect(t, results, "emit_answer_document_v2.go", "registered named-result passthrough \"finalize\" is no longer assigned")
	})
	t.Run("self_red_passthrough_without_the_named_result_argument", func(t *testing.T) {
		// The registered callee name alone is not a passthrough: it must take
		// the named result it rewrites.
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go",
			"defer func() { result = carriers.finalize(result) }()", "defer func() { result = carriers.finalize(types.ToolResult{}) }()"))
		expect(t, results, "emit_answer_document_v2.go", "assigns the named result(s) outside a registered passthrough")
	})
	t.Run("self_red_accept_lane_resolve_moved_into_a_branch", func(t *testing.T) {
		// The accept-lane resolve must sit at the top level: inside a branch
		// it would not end the reject-only region.
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go",
			"\trootCauseSelection = resolveTraceRootCauseSelectionForEmit(ctx, carriers, p.TraceRootCauses, false)\n",
			"\tif len(p.Blocks) > 0 {\n\t\trootCauseSelection = resolveTraceRootCauseSelectionForEmit(ctx, carriers, p.TraceRootCauses, false)\n\t}\n"))
		expect(t, results, "emit_answer_document_v2.go", "expected exactly one top-level")
	})
}
