package tool

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"sync"
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
// of eight shapes, the two-step return shadowing the named result included).
// Now every exit is audited and an exit shape the roster cannot key FAILS
// LOUD (the round-six ruling: fail-loud on unrecognized shapes ends the
// evasion enumeration).
//
// Round nine (§40.44 round-nine #0–#3) keeps the walker precise where it was
// loose: the reject recognizer keys the format string by the callee's
// SIGNATURE POSITION (rejectExitFailCallees) and resolves package consts and
// `"…" + "…"` concatenations, so the registrable refactors are registrable;
// the two selector resolvers are classified by EXACT name and any other
// `resolveTraceRootCauseSelection*` callee fails loud; the accept lane ends
// the reject-only region only at the top level (an accept resolve inside a
// branch dominates nothing and fails loud); a goto in the pre-accept region
// fails loud (domination is by statement order); and a return inside a
// closure whose results are not (types.ToolResult, error) is closure-local
// — it never exits the executor and is skipped, while a (types.ToolResult,
// error) closure and every named-result assignment stay audited.
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

// rejectExitRawResolver is the pre-decode resolve that reads the selector
// from the raw payload; it dominates the reject exits after it in its own
// statement list.
const rejectExitRawResolver = "resolveTraceRootCauseSelectionFromRawParams"

// rejectExitAcceptLaneResolver is the resolve after the strict decode; the
// pre-accept region of an executor ends at its top-level occurrence.
const rejectExitAcceptLaneResolver = "resolveTraceRootCauseSelectionForEmit"

// rejectExitFailCallees registers every reject helper by verbatim callee and
// the 0-based ARGUMENT POSITION of its format string; -1 marks a helper that
// builds its prose inside (rostered with message ""). The registry is pinned
// against the package: every callee must be a declared free function and
// the registered position a `string` parameter. EVOLUTION RECORD (§40.44
// round-nine #0): the round-eight recognizer keyed the FIRST string literal
// in ANY argument position and demanded one — a const-hoisted or
// concatenated format was "without a literal format string" with no roster
// path, an inlined tool-name literal silently keyed the exit by the tool
// name, and only failStrictDecode (hardcoded) was honoured without a
// literal, so the live sibling failStrictDecodeWithError or an extracted
// fail-prefixed helper could not be rostered at all.
var rejectExitFailCallees = map[string]int{
	"failEmit":                         2,
	"failEmitWithRepair":               3,
	"failStrictDecode":                 -1,
	"failStrictDecodeWithError":        -1,
	"failStrictDecodeWithErrorSchema":  -1,
	"failStrictDecodeMessage":          5,
	"failStrictDecodeWithErrorMessage": 5,
}

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
// a verbatim prefix of its format string as resolved from the callee's
// format position (long enough to be unique inside the executor; a helper
// registered at position -1 builds its prose inside and is rostered with
// message "").
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
// result that is a call to a bare `fail…` helper registered in callees,
// whose format string — at the callee's registered argument position —
// resolves to a string (a literal, a `+` concatenation of resolvable parts,
// or a package-level string const; position -1 = prose built inside, message
// ""). Every other shape is reported verbatim as the reason the roster
// cannot key it; an ambiguous key (a local identifier, a call, a non-string
// literal) is a shape, never a guess.
func rejectExitRecognize(ret *ast.ReturnStmt, callees map[string]int, consts map[string]ast.Expr) (callee, message, shape string) {
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
	pos, registered := callees[ident.Name]
	if !registered {
		return "", "", "returns a call to " + ident.Name + ", a fail* helper not registered in rejectExitFailCallees (register its format-argument position, or -1 for prose built inside)"
	}
	if pos < 0 {
		return ident.Name, "", ""
	}
	if pos >= len(call.Args) {
		return "", "", fmt.Sprintf("%s called with %d argument(s); its format string is argument %d", ident.Name, len(call.Args), pos)
	}
	resolved, why := rejectExitResolveFormat(call.Args[pos], consts)
	if why != "" {
		return "", "", fmt.Sprintf("%s format argument %d %s", ident.Name, pos, why)
	}
	return ident.Name, resolved, ""
}

// rejectExitResolveFormat resolves a format-position expression to its
// string: a string literal, `a + b` over resolvable parts, parentheses, or a
// package-level const (consts: name → value expression, chased to a bounded
// depth). Anything else is returned as the reason it cannot be keyed.
func rejectExitResolveFormat(expr ast.Expr, consts map[string]ast.Expr) (string, string) {
	var resolve func(e ast.Expr, depth int) (string, string)
	resolve = func(e ast.Expr, depth int) (string, string) {
		if depth > 16 {
			return "", "is a package-const chain deeper than 16 (cyclic?)"
		}
		switch x := e.(type) {
		case *ast.BasicLit:
			if x.Kind != token.STRING {
				return "", "is a " + strings.ToLower(x.Kind.String()) + " literal, not a string"
			}
			if s, err := strconv.Unquote(x.Value); err == nil {
				return s, ""
			}
			return strings.Trim(x.Value, "\"`"), ""
		case *ast.ParenExpr:
			return resolve(x.X, depth)
		case *ast.BinaryExpr:
			if x.Op != token.ADD {
				return "", "is a " + x.Op.String() + " expression, not a string literal or concatenation"
			}
			left, why := resolve(x.X, depth)
			if why != "" {
				return "", why
			}
			right, why := resolve(x.Y, depth)
			if why != "" {
				return "", why
			}
			return left + right, ""
		case *ast.Ident:
			value, ok := consts[x.Name]
			if !ok {
				return "", "is the identifier " + x.Name + ", not a string literal or a package-level string const"
			}
			return resolve(value, depth+1)
		}
		return "", fmt.Sprintf("is a %T, not a string literal, a concatenation or a package-level const", e)
	}
	return resolve(expr, 0)
}

// rejectExitLocalNames collects every name a function declares locally:
// parameters, receivers, named results, `:=` targets, var / const specs and
// range variables (closures included).
func rejectExitLocalNames(fn *ast.FuncDecl) map[string]bool {
	names := map[string]bool{}
	add := func(e ast.Expr) {
		if id, ok := e.(*ast.Ident); ok {
			names[id.Name] = true
		}
	}
	for _, list := range []*ast.FieldList{fn.Recv, fn.Type.Params, fn.Type.Results} {
		if list == nil {
			continue
		}
		for _, field := range list.List {
			for _, name := range field.Names {
				names[name.Name] = true
			}
		}
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				for _, lhs := range x.Lhs {
					add(lhs)
				}
			}
		case *ast.ValueSpec:
			for _, name := range x.Names {
				names[name.Name] = true
			}
		case *ast.RangeStmt:
			if x.Tok == token.DEFINE {
				add(x.Key)
				add(x.Value)
			}
		case *ast.FuncLit:
			for _, list := range []*ast.FieldList{x.Type.Params, x.Type.Results} {
				if list == nil {
					continue
				}
				for _, field := range list.List {
					for _, name := range field.Names {
						names[name.Name] = true
					}
				}
			}
		}
		return true
	})
	return names
}

// rejectExitPackageConsts collects the package-level const declarations of
// parsed files (name → value expression; specs without their own values —
// iota repetition — are skipped).
func rejectExitPackageConsts(files []*ast.File, into map[string]ast.Expr) {
	for _, file := range files {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i < len(vs.Values) {
						into[name.Name] = vs.Values[i]
					}
				}
			}
		}
	}
}

// rejectExitPackageFacts is the package as parsed from disk once per test
// binary: its package-level consts (format resolution reads consts from ANY
// file of the package) and its free functions (the fail* registry is pinned
// against real signatures).
type rejectExitPackageFactsT struct {
	consts map[string]ast.Expr
	funcs  map[string]*ast.FuncDecl
}

var (
	rejectExitPackageFactsOnce sync.Once
	rejectExitPackageFactsVal  *rejectExitPackageFactsT
	rejectExitPackageFactsErr  error
)

func rejectExitPackageFacts() (*rejectExitPackageFactsT, error) {
	rejectExitPackageFactsOnce.Do(func() {
		facts := &rejectExitPackageFactsT{consts: map[string]ast.Expr{}, funcs: map[string]*ast.FuncDecl{}}
		entries, err := os.ReadDir(".")
		if err != nil {
			rejectExitPackageFactsErr = err
			return
		}
		fset := token.NewFileSet()
		var files []*ast.File
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			src, err := os.ReadFile(name)
			if err != nil {
				rejectExitPackageFactsErr = err
				return
			}
			f, err := parser.ParseFile(fset, name, src, 0)
			if err != nil {
				rejectExitPackageFactsErr = err
				return
			}
			files = append(files, f)
			for _, decl := range f.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
					facts.funcs[fn.Name.Name] = fn
				}
			}
		}
		rejectExitPackageConsts(files, facts.consts)
		rejectExitPackageFactsVal = facts
	})
	return rejectExitPackageFactsVal, rejectExitPackageFactsErr
}

// rejectExitRegistryOffenders pins the fail* registry against the package:
// every registered callee is a declared free function and its registered
// format position names a `string` parameter (-1: the helper takes no
// format position — its prose is built inside).
func rejectExitRegistryOffenders(callees map[string]int, facts *rejectExitPackageFactsT) (offenders []string) {
	for name, pos := range callees {
		fn := facts.funcs[name]
		if fn == nil {
			offenders = append(offenders, fmt.Sprintf("rejectExitFailCallees registers %s, which is not a free function of package tool; update the registry", name))
			continue
		}
		if pos < 0 {
			continue
		}
		var params []ast.Expr
		for _, field := range fn.Type.Params.List {
			n := len(field.Names)
			if n == 0 {
				n = 1
			}
			for i := 0; i < n; i++ {
				params = append(params, field.Type)
			}
		}
		if pos >= len(params) {
			offenders = append(offenders, fmt.Sprintf("rejectExitFailCallees registers %s's format at argument %d, but it declares %d parameter(s); update the registry", name, pos, len(params)))
			continue
		}
		if id, ok := params[pos].(*ast.Ident); !ok || id.Name != "string" {
			offenders = append(offenders, fmt.Sprintf("rejectExitFailCallees registers %s's format at argument %d, which is not a string parameter; update the registry", name, pos))
		}
	}
	return offenders
}

// rejectExitResolveKind classifies `rootCauseSelection = <resolver>(…)`.
type rejectExitResolveKind int

const (
	rejectExitNotAResolve    rejectExitResolveKind = iota
	rejectExitResolveRaw                           // the raw-params (pre-decode) resolve
	rejectExitResolveAccept                        // the accept-lane resolve
	rejectExitResolveUnknown                       // a resolveTraceRootCauseSelection* callee of neither exact name: fail loud
)

// rejectExitResolveOf classifies a statement as a selector resolve by the
// EXACT resolver name. EVOLUTION RECORD (§40.44 round-nine #1): the
// round-eight walker treated any prefix-sharing callee without the
// FromRawParams suffix as the accept lane at any scope, so a new resolver
// variant or the accept resolver inside a pre-decode branch silently opened
// an unaudited region.
func rejectExitResolveOf(stmt ast.Stmt) (kind rejectExitResolveKind, name string) {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return rejectExitNotAResolve, ""
	}
	lhs, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name != rejectExitSelectionVar {
		return rejectExitNotAResolve, ""
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return rejectExitNotAResolve, ""
	}
	fn, ok := call.Fun.(*ast.Ident)
	if !ok || !strings.HasPrefix(fn.Name, rejectExitSelectorResolverPrefix) {
		return rejectExitNotAResolve, ""
	}
	switch fn.Name {
	case rejectExitRawResolver:
		return rejectExitResolveRaw, fn.Name
	case rejectExitAcceptLaneResolver:
		return rejectExitResolveAccept, fn.Name
	}
	return rejectExitResolveUnknown, fn.Name
}

// rejectExitClosureCarriesExits reports whether a func literal's results are
// (types.ToolResult, error) — the executor's own signature, so a return
// inside it can carry an executor exit. Any other closure is closure-local:
// its returns never leave the executor.
func rejectExitClosureCarriesExits(lit *ast.FuncLit) bool {
	if lit.Type.Results == nil {
		return false
	}
	var types []ast.Expr
	for _, field := range lit.Type.Results.List {
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		for i := 0; i < n; i++ {
			types = append(types, field.Type)
		}
	}
	if len(types) != 2 {
		return false
	}
	if exprTypeName(types[0]) != "types.ToolResult" && exprTypeName(types[0]) != "ToolResult" {
		return false
	}
	return exprTypeName(types[1]) == "error"
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
// domination state inherited from the enclosing statement list; `local` says
// the statement list belongs to a closure-local func literal (its returns
// are not executor exits).
func auditRejectExitSelector(fset *token.FileSet, file string, fn *ast.FuncDecl, roster []rejectExitRow, passthroughs []string, callees map[string]int, consts map[string]ast.Expr) rejectExitCensus {
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
	// postAccept: the accept-lane-dominated region, which the census does not
	// audit (only a TOP-LEVEL accept resolve opens it).
	postAccept := func(resolved, rawResolved bool) bool { return resolved && !rawResolved }
	var walkList func(list []ast.Stmt, resolved, rawResolved bool, top, local bool) (bool, bool)
	var walkStmt func(stmt ast.Stmt, resolved, rawResolved bool, top, local bool) (bool, bool)
	walkList = func(list []ast.Stmt, resolved, rawResolved bool, top, local bool) (bool, bool) {
		for _, stmt := range list {
			resolved, rawResolved = walkStmt(stmt, resolved, rawResolved, top, local)
		}
		return resolved, rawResolved
	}
	// walkBranch audits a nested scope that may not execute (branch / loop /
	// closure): its own resolves dominate only inside it.
	walkBranch := func(list []ast.Stmt, resolved, rawResolved bool, local bool) {
		walkList(list, resolved, rawResolved, false, local)
	}
	walkStmt = func(stmt ast.Stmt, resolved, rawResolved bool, top, local bool) (bool, bool) {
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			switch kind, name := rejectExitResolveOf(s); kind {
			case rejectExitResolveRaw:
				c.resolves++
				return true, true
			case rejectExitResolveAccept:
				c.resolves++
				if top {
					c.acceptResolves++
					return true, false
				}
				// §40.44 round-nine #1: inside a branch / closure the accept
				// resolve neither ends the reject-only region nor dominates a
				// reject exit — the pre-decode resolve is the raw resolver.
				if !postAccept(resolved, rawResolved) {
					c.offenders = append(c.offenders, fmt.Sprintf("%s %s: accept-lane resolve inside a branch (not at the top level of the body); it neither ends the reject-only region nor dominates a reject exit — resolve pre-decode exits with `%s = %s(…)` (§40.44 round-nine #1)",
						pos(s), rejectExitFuncName(fn), rejectExitSelectionVar, rejectExitRawResolver))
				}
				return resolved, rawResolved
			case rejectExitResolveUnknown:
				if !postAccept(resolved, rawResolved) {
					c.offenders = append(c.offenders, fmt.Sprintf("%s %s: unrecognized selector resolver %s; the census classifies only %s (raw lane) and %s (accept lane) by exact name — register a new resolver deliberately (§40.44 round-nine #1: fail loud on unrecognized shapes)",
						pos(s), rejectExitFuncName(fn), name, rejectExitRawResolver, rejectExitAcceptLaneResolver))
				}
				return resolved, rawResolved
			}
			if writes, passthrough := rejectExitNamedResultAssign(s, named, passthroughSet); writes {
				if passthrough != "" {
					seenPassthrough[passthrough] = true
				} else {
					exit(s, resolved, rawResolved, "", "", "assigns the named result(s) outside a registered passthrough")
				}
			}
		case *ast.ReturnStmt:
			if local {
				// §40.44 round-nine #3: a closure-local return never exits
				// the executor.
				return resolved, rawResolved
			}
			callee, message, shape := rejectExitRecognize(s, callees, consts)
			exit(s, resolved, rawResolved, callee, message, shape)
			return resolved, rawResolved
		case *ast.BranchStmt:
			// §40.44 round-nine #2: domination is by statement order; a goto
			// can jump over the raw resolve to a labelled reject.
			if s.Tok == token.GOTO && !postAccept(resolved, rawResolved) {
				label := ""
				if s.Label != nil {
					label = " " + s.Label.Name
				}
				c.offenders = append(c.offenders, fmt.Sprintf("%s %s: goto%s in the pre-accept region: domination is by statement order and a jump can bypass the raw resolve — restructure the reject without goto (§40.44 round-nine #2)",
					pos(s), rejectExitFuncName(fn), label))
			}
			return resolved, rawResolved
		case *ast.BlockStmt:
			// A bare block always executes: its resolves dominate what follows.
			return walkList(s.List, resolved, rawResolved, false, local)
		case *ast.LabeledStmt:
			return walkStmt(s.Stmt, resolved, rawResolved, top, local)
		case *ast.IfStmt:
			if s.Init != nil {
				resolved, rawResolved = walkStmt(s.Init, resolved, rawResolved, false, local)
			}
			walkBranch(s.Body.List, resolved, rawResolved, local)
			if s.Else != nil {
				walkStmt(s.Else, resolved, rawResolved, false, local)
			}
			return resolved, rawResolved
		case *ast.ForStmt:
			if s.Init != nil {
				resolved, rawResolved = walkStmt(s.Init, resolved, rawResolved, false, local)
			}
			walkBranch(s.Body.List, resolved, rawResolved, local)
			return resolved, rawResolved
		case *ast.RangeStmt:
			walkBranch(s.Body.List, resolved, rawResolved, local)
			return resolved, rawResolved
		case *ast.SwitchStmt:
			if s.Init != nil {
				resolved, rawResolved = walkStmt(s.Init, resolved, rawResolved, false, local)
			}
			for _, clause := range s.Body.List {
				walkBranch(clause.(*ast.CaseClause).Body, resolved, rawResolved, local)
			}
			return resolved, rawResolved
		case *ast.TypeSwitchStmt:
			if s.Init != nil {
				resolved, rawResolved = walkStmt(s.Init, resolved, rawResolved, false, local)
			}
			for _, clause := range s.Body.List {
				walkBranch(clause.(*ast.CaseClause).Body, resolved, rawResolved, local)
			}
			return resolved, rawResolved
		case *ast.SelectStmt:
			for _, clause := range s.Body.List {
				walkBranch(clause.(*ast.CommClause).Body, resolved, rawResolved, local)
			}
			return resolved, rawResolved
		}
		// Closures (defer / go / expression / assignment operands) inherit the
		// domination state at their definition point. A named-result
		// assignment inside one (the defer/recover guard shape) is audited
		// like any other exit; a return is audited only when the closure's
		// results are (types.ToolResult, error) — any other closure is
		// closure-local (§40.44 round-nine #3).
		ast.Inspect(stmt, func(n ast.Node) bool {
			if lit, ok := n.(*ast.FuncLit); ok {
				walkBranch(lit.Body.List, resolved, rawResolved, !rejectExitClosureCarriesExits(lit))
				return false
			}
			return true
		})
		return resolved, rawResolved
	}
	endResolved, endRaw := walkList(fn.Body.List, false, false, true, false)
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
	return rejectExitCensusOverTables(t, srcs, roster, passthroughs, rejectExitFailCallees)
}

// rejectExitCensusOverTables parses every file of srcs (the executor files,
// possibly mutated), overlays their package-level consts on the package's
// on-disk consts (format resolution reads consts from any file of the
// package), and audits the executors with the given roster, passthroughs
// and fail* registry.
func rejectExitCensusOverTables(t *testing.T, srcs map[string]string, roster map[string][]rejectExitRow, passthroughs map[string][]string, callees map[string]int) map[string]rejectExitCensus {
	t.Helper()
	pkg, err := rejectExitPackageFacts()
	if err != nil {
		t.Fatalf("parse package tool: %v", err)
	}
	pkgConsts := map[string]ast.Expr{}
	for name, value := range pkg.consts {
		pkgConsts[name] = value
	}
	fset := token.NewFileSet()
	parsed := map[string]*ast.File{}
	var files []*ast.File
	for file, src := range srcs {
		f, err := parser.ParseFile(fset, file, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		parsed[file] = f
		files = append(files, f)
	}
	rejectExitPackageConsts(files, pkgConsts)
	out := map[string]rejectExitCensus{}
	for file, want := range rejectExitExecutors {
		f, ok := parsed[file]
		if !ok {
			t.Fatalf("census input lacks %s", file)
		}
		found := false
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || rejectExitFuncName(fn) != want {
				continue
			}
			found = true
			// A name the executor declares locally (parameter, result,
			// `:=`, var, range) shadows a package const of the same name:
			// it is not resolvable as a const inside this body.
			consts := map[string]ast.Expr{}
			for name, value := range pkgConsts {
				consts[name] = value
			}
			for name := range rejectExitLocalNames(fn) {
				delete(consts, name)
			}
			out[file] = auditRejectExitSelector(fset, file, fn, roster[file], passthroughs[file], callees, consts)
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
	// The fail* registry is pinned against the package's real signatures.
	pkg, err := rejectExitPackageFacts()
	if err != nil {
		t.Fatalf("parse package tool: %v", err)
	}
	for _, o := range rejectExitRegistryOffenders(rejectExitFailCallees, pkg) {
		t.Error(o)
	}
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

	// edit rewrites one file of a source set (first occurrence; the anchor
	// must exist); mutate edits the live set.
	edit := func(t *testing.T, base map[string]string, file, old, new string) map[string]string {
		t.Helper()
		out := map[string]string{}
		for k, v := range base {
			out[k] = v
		}
		if !strings.Contains(out[file], old) {
			t.Fatalf("self-red anchor %q not found in %s", old, file)
		}
		out[file] = strings.Replace(out[file], old, new, 1)
		return out
	}
	mutate := func(t *testing.T, file, old, new string) map[string]string {
		t.Helper()
		return edit(t, live, file, old, new)
	}
	// expectGreen: the census over a recognized, registrable shape reports
	// nothing for the file — the shape is not taxed.
	expectGreen := func(t *testing.T, results map[string]rejectExitCensus, file string) {
		t.Helper()
		if len(results[file].offenders) > 0 {
			t.Fatalf("a recognized shape must not be reported in %s; offenders: %v", file, results[file].offenders)
		}
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
	// reject exit — the eight shapes below were uncounted (exits=9,
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

	// §40.44 round-nine #0: the reject recognizer keys the format string by
	// the callee's SIGNATURE POSITION and resolves package consts and
	// concatenations. EVOLUTION RECORD: the round-eight recognizer keyed the
	// first string literal in any argument position and demanded one, so on
	// 49efc4a2e every registrable shape below was a dead end — const-hoisted
	// / concatenated formats red as "without a literal format string", an
	// inlined tool name silently keyed the exit by the tool name, the live
	// sibling failStrictDecodeWithError and an extracted fail-prefixed helper
	// unregistrable (message-"" rows were honoured for one hardcoded name).
	const v1FieldsFormat = "\"top-level field %q is not accepted; the answer is expressed through blocks[] only — move any answer payload into the appropriate block kind\","
	const v1FieldsConst = "answerDocumentV1FieldsRejectFormat"
	t.Run("self_green_const_hoisted_format_resolves", func(t *testing.T) {
		src := mutate(t, "emit_answer_document_v2.go", v1FieldsFormat, v1FieldsConst+",")
		src["emit_answer_document_v2.go"] += "\n\nconst " + v1FieldsConst + " = " + strings.TrimSuffix(v1FieldsFormat, ",") + "\n"
		expectGreen(t, rejectExitCensusOver(t, src), "emit_answer_document_v2.go")
	})
	t.Run("self_green_cross_file_const_format_resolves", func(t *testing.T) {
		// The const lives in ANOTHER file of the package: the resolver reads
		// package-level consts, not the executor file alone.
		src := mutate(t, "emit_answer_document_v2.go", v1FieldsFormat, v1FieldsConst+",")
		src["emit_answer_document_patch.go"] += "\n\nconst " + v1FieldsConst + " = " + strings.TrimSuffix(v1FieldsFormat, ",") + "\n"
		expectGreen(t, rejectExitCensusOver(t, src), "emit_answer_document_v2.go")
	})
	t.Run("self_red_const_hoisted_format_drift", func(t *testing.T) {
		// A drifted const is a drifted message: the row is stale and the exit
		// unregistered (the same red as a drifted literal).
		src := mutate(t, "emit_answer_document_v2.go", v1FieldsFormat, v1FieldsConst+",")
		src["emit_answer_document_v2.go"] += "\n\nconst " + v1FieldsConst + " = \"top-level field %q is refused\"\n"
		results := rejectExitCensusOver(t, src)
		expect(t, results, "emit_answer_document_v2.go", "no longer names a reject exit")
		expect(t, results, "emit_answer_document_v2.go", "is not registered in the exit roster")
	})
	t.Run("self_green_concatenated_format_resolves", func(t *testing.T) {
		src := mutate(t, "emit_answer_document_v2.go", v1FieldsFormat,
			"\"top-level field %q is not accepted; \" +\n\t\t\t\t\"the answer is expressed through blocks[] only — move any answer payload into the appropriate block kind\",")
		expectGreen(t, rejectExitCensusOver(t, src), "emit_answer_document_v2.go")
	})
	t.Run("self_red_format_argument_is_a_local_identifier", func(t *testing.T) {
		// A local variable in the format position is not a package const:
		// the key is ambiguous, fail loud.
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go", v1FieldsFormat, "violation,"))
		expect(t, results, "emit_answer_document_v2.go", "unrecognized reject exit shape")
		expect(t, results, "emit_answer_document_v2.go", "is the identifier violation, not a string literal or a package-level string const")
	})
	t.Run("self_red_local_shadowing_a_package_const_is_ambiguous", func(t *testing.T) {
		// The package const exists, but the executor declares a local of the
		// same name before the exit: the format is the local, not the const.
		src := mutate(t, "emit_answer_document_v2.go", "\t\treturn failEmit(toolName, now,\n\t\t\t"+v1FieldsFormat,
			"\t\t"+v1FieldsConst+" := \"shadow\"\n\t\treturn failEmit(toolName, now,\n\t\t\t"+v1FieldsConst+",")
		src["emit_answer_document_v2.go"] += "\n\nconst " + v1FieldsConst + " = " + strings.TrimSuffix(v1FieldsFormat, ",") + "\n"
		results := rejectExitCensusOver(t, src)
		expect(t, results, "emit_answer_document_v2.go", "is the identifier "+v1FieldsConst+", not a string literal or a package-level string const")
	})
	t.Run("self_red_format_argument_is_a_call", func(t *testing.T) {
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go", v1FieldsFormat, "strings.ToUpper(\"top-level field %q is not accepted\"),"))
		expect(t, results, "emit_answer_document_v2.go", "unrecognized reject exit shape")
		expect(t, results, "emit_answer_document_v2.go", "format argument 2 is a *ast.CallExpr")
	})
	t.Run("self_red_registered_callee_arity_short_of_the_format_position", func(t *testing.T) {
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go",
			"\t\treturn failEmit(toolName, now,\n\t\t\t"+v1FieldsFormat+"\n\t\t\tviolation)", "\t\treturn failEmit(toolName, now)"))
		expect(t, results, "emit_answer_document_v2.go", "failEmit called with 2 argument(s); its format string is argument 2")
	})
	t.Run("self_green_sibling_strict_decode_callee_with_message_empty_row", func(t *testing.T) {
		// Switching to the live sibling helper is registrable by editing the
		// roster row's callee: message-"" rows are honoured for every
		// registered prose-building helper, not one hardcoded name.
		src := mutate(t, "emit_answer_document_v2.go", "return failStrictDecode(", "return failStrictDecodeWithError(")
		roster := map[string][]rejectExitRow{}
		for k, v := range rejectExitRoster {
			roster[k] = append([]rejectExitRow{}, v...)
		}
		rows := roster["emit_answer_document_v2.go"]
		rows[len(rows)-1] = rejectExitRow{"failStrictDecodeWithError", "", rejectExitLaneRaw}
		expectGreen(t, rejectExitCensusOverRoster(t, src, roster, rejectExitPassthroughs), "emit_answer_document_v2.go")
	})
	t.Run("self_red_sibling_strict_decode_callee_without_roster_update", func(t *testing.T) {
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go", "return failStrictDecode(", "return failStrictDecodeWithError("))
		expect(t, results, "emit_answer_document_v2.go", "registered exit failStrictDecode(\"\"…) [raw] no longer names a reject exit")
		expect(t, results, "emit_answer_document_v2.go", "reject exit failStrictDecodeWithError(\"\"…) is not registered")
	})
	const relationClaimsReject = "\t\treturn failEmit(toolName, now,\n\t\t\t\"top-level field %q is not accepted; place the exact typed claim object(s) under blocks[i].relation_claims on the model-authored block that uses the values (never at $.relation_claims)\",\n\t\t\t\"relation_claims\")"
	t.Run("self_green_extracted_fail_helper_registered", func(t *testing.T) {
		// An extracted fail-prefixed helper that builds its prose inside is
		// registrable: a registry entry (-1) plus a message-"" roster row.
		src := mutate(t, "emit_answer_document_v2.go", relationClaimsReject, "\t\treturn failTopLevelRelationClaims(toolName, now)")
		callees := map[string]int{}
		for k, v := range rejectExitFailCallees {
			callees[k] = v
		}
		callees["failTopLevelRelationClaims"] = -1
		roster := map[string][]rejectExitRow{}
		for k, v := range rejectExitRoster {
			roster[k] = append([]rejectExitRow{}, v...)
		}
		roster["emit_answer_document_v2.go"][1] = rejectExitRow{"failTopLevelRelationClaims", "", rejectExitLaneRaw}
		expectGreen(t, rejectExitCensusOverTables(t, src, roster, rejectExitPassthroughs, callees), "emit_answer_document_v2.go")
	})
	t.Run("self_red_extracted_fail_helper_unregistered", func(t *testing.T) {
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go", relationClaimsReject, "\t\treturn failTopLevelRelationClaims(toolName, now)"))
		expect(t, results, "emit_answer_document_v2.go", "unrecognized reject exit shape")
		expect(t, results, "emit_answer_document_v2.go", "failTopLevelRelationClaims, a fail* helper not registered in rejectExitFailCallees")
	})
	t.Run("self_green_inlined_tool_name_literal_keys_by_format_position", func(t *testing.T) {
		// A tool-name literal before the format position is not the key.
		src := mutate(t, "emit_answer_document_patch.go",
			"\t\treturn failEmit(t.Name(), now,\n\t\t\t\"emit_answer_document_patch requires a writable context\")",
			"\t\treturn failEmit(\"emit_answer_document_patch\", now,\n\t\t\t\"emit_answer_document_patch requires a writable context\")")
		src = edit(t, src, "emit_answer_document_v2.go", "\t\treturn failEmit(toolName, now,\n\t\t\t"+v1FieldsFormat, "\t\treturn failEmit(\"emit_answer_document\", now,\n\t\t\t"+v1FieldsFormat)
		results := rejectExitCensusOver(t, src)
		expectGreen(t, results, "emit_answer_document_patch.go")
		expectGreen(t, results, "emit_answer_document_v2.go")
	})

	// §40.44 round-nine #1: the selector resolvers are classified by EXACT
	// name — the raw resolver and the accept-lane resolver — and the accept
	// lane ends the reject-only region only at the top level. EVOLUTION
	// RECORD: on 49efc4a2e any `resolveTraceRootCauseSelection*` callee
	// without the FromRawParams suffix was the accept lane at ANY scope, so
	// the accept resolver inside a pre-decode branch, or a new resolver
	// variant at the top level, opened a region whose exits were neither
	// shape-checked nor reconciled (overlay probes B1/B1b/B4/B5: offenders=0).
	const acceptResolveV2 = "rootCauseSelection = resolveTraceRootCauseSelectionForEmit(ctx, carriers, nil, false)"
	t.Run("self_red_accept_lane_resolve_inside_a_pre_decode_branch_does_not_dominate", func(t *testing.T) {
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go", decodeAnchorV2,
			"\tif len(raw) > 1<<20 {\n\t\t"+acceptResolveV2+"\n\t\tres, e := failEmit(toolName, now, \"payload too large\")\n\t\treturn res, e\n\t}\n"+decodeAnchorV2))
		expect(t, results, "emit_answer_document_v2.go", "accept-lane resolve inside a branch")
		expect(t, results, "emit_answer_document_v2.go", "unrecognized reject exit shape")
		expect(t, results, "emit_answer_document_v2.go", "reject exit is not dominated")
	})
	t.Run("self_red_accept_lane_resolve_inside_a_branch_leaves_a_plain_reject_unregistered", func(t *testing.T) {
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go", decodeAnchorV2,
			"\tif len(raw) > 1<<20 {\n\t\t"+acceptResolveV2+"\n\t\treturn failEmit(toolName, now, \"payload too large\")\n\t}\n"+decodeAnchorV2))
		expect(t, results, "emit_answer_document_v2.go", "accept-lane resolve inside a branch")
		expect(t, results, "emit_answer_document_v2.go", "is not registered in the exit roster")
		expect(t, results, "emit_answer_document_v2.go", "reject exit is not dominated")
	})
	t.Run("self_red_unrecognized_resolver_variant_fails_loud", func(t *testing.T) {
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go", decodeAnchorV2,
			"\trootCauseSelection = resolveTraceRootCauseSelectionFromRawParamsLenient(ctx, carriers, raw, false)\n\tif len(raw) > 1<<20 {\n\t\treturn types.ToolResult{}, fmt.Errorf(\"payload too large\")\n\t}\n"+decodeAnchorV2))
		expect(t, results, "emit_answer_document_v2.go", "unrecognized selector resolver resolveTraceRootCauseSelectionFromRawParamsLenient")
		expect(t, results, "emit_answer_document_v2.go", "unrecognized reject exit shape")
		expect(t, results, "emit_answer_document_v2.go", "reject exit is not dominated")
	})
	t.Run("self_red_unrecognized_resolver_noop_fails_loud", func(t *testing.T) {
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go", decodeAnchorV2,
			"\trootCauseSelection = resolveTraceRootCauseSelectionNoop()\n\tif len(raw) > 1<<20 {\n\t\treturn failEmit(toolName, now, \"payload too large\")\n\t}\n"+decodeAnchorV2))
		expect(t, results, "emit_answer_document_v2.go", "unrecognized selector resolver resolveTraceRootCauseSelectionNoop")
		expect(t, results, "emit_answer_document_v2.go", "is not registered in the exit roster")
		expect(t, results, "emit_answer_document_v2.go", "reject exit is not dominated")
	})

	// §40.44 round-nine #2: domination is by statement order, so a goto in
	// the pre-accept region fails loud (a jump over the raw resolve to a
	// labelled reject was "dominated" textually on 49efc4a2e: offenders=1,
	// the missing roster row only, and 0 once rostered).
	t.Run("self_red_goto_over_the_raw_resolve_fails_loud", func(t *testing.T) {
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go", decodeAnchorV2,
			"\tif len(raw) > 1<<20 {\n\t\tif ctx == nil {\n\t\t\tgoto rejectLarge\n\t\t}\n\t\t"+rawResolveV2+"\n\trejectLarge:\n\t\treturn failEmit(toolName, now, \"payload too large\")\n\t}\n"+decodeAnchorV2))
		expect(t, results, "emit_answer_document_v2.go", "goto rejectLarge in the pre-accept region")
	})

	// §40.44 round-nine #3: a return inside a closure whose results are not
	// (types.ToolResult, error) never exits the executor — closure-local,
	// skipped; a closure returning (types.ToolResult, error) is audited like
	// any exit, and named-result assignments inside any closure stay exits
	// (the defer/recover guard shape above). EVOLUTION RECORD: on 49efc4a2e a
	// sort comparator before the raw resolve was red as "returns a
	// *ast.BinaryExpr, not a reject call" + "not dominated" — a false offender
	// on a routine edit with no roster path.
	t.Run("self_green_closure_local_return_is_not_an_exit", func(t *testing.T) {
		src := mutate(t, "emit_answer_document_v2.go",
			"\t\t"+rawResolveV2+" // §40.43 round-six #4\n\t\treturn failEmitWithRepair(toolName, now, answerDocumentStructuralCarrierCorruptionRepair(paths),",
			"\t\tsort.Slice(paths, func(i, j int) bool { return paths[i] < paths[j] })\n\t\t"+rawResolveV2+" // §40.43 round-six #4\n\t\treturn failEmitWithRepair(toolName, now, answerDocumentStructuralCarrierCorruptionRepair(paths),")
		expectGreen(t, rejectExitCensusOver(t, src), "emit_answer_document_v2.go")
	})
	t.Run("self_red_tool_result_closure_return_is_audited", func(t *testing.T) {
		results := rejectExitCensusOver(t, mutate(t, "emit_answer_document_v2.go", decodeAnchorV2,
			"\treject := func() (types.ToolResult, error) {\n\t\tres, e := failEmit(toolName, now, \"payload too large\")\n\t\treturn res, e\n\t}\n\t_ = reject\n"+decodeAnchorV2))
		expect(t, results, "emit_answer_document_v2.go", "unrecognized reject exit shape")
		expect(t, results, "emit_answer_document_v2.go", "reject exit is not dominated")
	})
}
