package orchestrator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// hardArmMutableCarrier is one state read the accepted-closure retry
// authority (a hard arm) or a censused gate body is allowed to consult.
// §40.14 V7-2 rule: every carrier read by a hard arm must have an explicit
// lifecycle writer pair (populate / reset) and the reset must have a
// production call site — a carrier whose reset has zero callers is a stale
// signal that can never lift, and a hard arm reading it fires on decisions
// from an earlier generation. Counters are generation witnesses (monotonic,
// compared as single integers) and carry no reset.
type hardArmMutableCarrier struct {
	getter string // MutableState method (or, for kind "signal", the ExecutionSignals field) the gate may read
	// kind: "carrier" (needs a MutableState reset), "counter" (monotonic
	// witness), "retained" (deliberately sticky witness that survives
	// resets — documented, never cleared), "closure" (a sub-object whose own
	// pending sets are cleared by the named EvidenceClosure method),
	// "signal" (a BusContext.ExecutionSignals field — §40.43 R2 — whose
	// reset site is the named arm of runReadSchedulerLoop's fallback switch,
	// which must assign it false; every accepted-closure exit raises it and
	// nothing else clears it), "decision" (§40.43 F-orch Q: a MutableState
	// writer that advances a registered counter — a typed decision that
	// releases a veto; it must have exactly one production call site and
	// every writer of the counter it advances must be a registered decision).
	kind  string
	reset string // reset method (carrier / closure) or fallback arm (signal)
	// advances names the registered counter a "decision" writer advances.
	advances string
}

// hardArmMutableCarriers is the single declared table. Adding a new
// `mut.<X>()` / `o.busCtx.Signals.<X>` read to a censused gate body
// without registering it here is red; registering a carrier whose reset
// has no production caller is red.
var hardArmMutableCarriers = []hardArmMutableCarrier{
	{getter: "RetryState", kind: "carrier", reset: "ResetRetryState"},
	{getter: "ExploreBacktrackEpoch", kind: "counter"},
	{getter: "InvestigationCompleteGeneration", kind: "counter"},
	// §40.42 ①: the enclosing hard gate's own reads.
	{getter: "IsInvestigationComplete", kind: "carrier", reset: "ResetInvestigationComplete"},
	{getter: "StableInvestigationCompleteReason", kind: "retained"},
	{getter: "EvidenceClosure", kind: "closure", reset: "ClearPendingReads"},
	// F14: the reconcile arm's criterion env reads the retained aggregate
	// handoff (sticky across explore-window resets, like the reason).
	{getter: "StableInvestigationAggregateFacts", kind: "retained"},
	// §40.43 R2: the reconcile arm's signal door reads the accepted-closure
	// signal (directly and through the criterion env copy). Its reset is
	// the explore backtrack arm — the explorer re-earns it on a fresh
	// completion.
	{getter: "HasEnoughFacts", kind: "signal", reset: "FallbackBackToExplore"},
	// §40.43 F-orch Q: the scheduler's typed ExploreBacktrackExhausted
	// decision — the explore window re-opened by a backtrack closed without
	// a fresh accepted completion — advances the completion generation the
	// veto is bound against (the same release path as a fresh completion).
	{getter: "RecordExploreBacktrackExhausted", kind: "decision", advances: "InvestigationCompleteGeneration"},
}

// hardArmCompletionGenerationDecisions is the closed set of MutableState
// methods allowed to advance investigationCompleteGeneration: accepted
// completion decisions (model emit or typed system closure), fork merges
// folding a fork's own decisions back, and the scheduler's typed
// exhaustion decision. A reset is not a decision and never appears here.
var hardArmCompletionGenerationDecisions = map[string]bool{
	"SetInvestigationComplete":        true,
	"MergeExploreFork":                true,
	"RecordExploreBacktrackExhausted": true,
}

const hardArmRetryAuthorityFile = "accepted_closure_retry_authority.go"

// hardArmGateFunctions are the hard-gate bodies censused beside the arm file:
// every `mut.X()` / `o.busCtx.Mutable.X()` call and every
// `<recv>.Signals.<field>` read inside them must be registered (§40.42 ① —
// the census covers the gate body, not only the arm; F14 — the shared
// accepted-closure premise and the reconcile arm are gate bodies too, and
// TestHardArmMutableCarrierCensus_PremiseConsumersAreCensused keeps this
// table total over every premise consumer).
var hardArmGateFunctions = map[string][]string{
	"orchestrator.go":                     {"shouldAutoCompleteExploreWindowFromAcceptedClosure"},
	"accepted_closure_premise.go":         {"acceptedClosurePremise"},
	"accepted_closure_reconcile.go":       {"acceptedClosureCanSatisfyReconcileEnoughFacts", "shouldAutoCompleteReadyReconcileNode"},
	"accepted_closure_origin_debt.go":     {"acceptedClosureMissingRequiredOriginsForAutoComplete"},
	"accepted_closure_retry_authority.go": nil, // whole file, MutableState params
	// §40.43 F-orch Q: the exhaustion release reads the backtrack arm and
	// records the typed decision that consumes it.
	"explore_backtrack_exhaustion.go": {"releaseExhaustedExploreBacktrack"},
}

// hardArmPremiseEntryPoints are the calls that make a function an
// accepted-closure consumer (a hard-gate body): any non-test function in the
// package that calls one of them must be listed in hardArmGateFunctions.
var hardArmPremiseEntryPoints = map[string]bool{
	"acceptedClosurePremise":                           true,
	"acceptedClosureHasActiveExploreContractBacktrack": true,
}

// TestHardArmMutableCarrierCensus_PremiseConsumersAreCensused (F14 totality):
// the census cannot be satisfied by registering only the consumers we know
// about — every production function that reads the accepted-closure premise
// (directly or via the backtrack arm) must be a censused gate body.
func TestHardArmMutableCarrierCensus_PremiseConsumersAreCensused(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob package files: %v (n=%d)", err, len(files))
	}
	fset := token.NewFileSet()
	var uncensused []string
	consumers := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		listed := map[string]bool{}
		wholeFile := false
		if names, ok := hardArmGateFunctions[file]; ok {
			wholeFile = names == nil
			for _, name := range names {
				listed[name] = true
			}
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || hardArmPremiseEntryPoints[fn.Name.Name] {
				continue
			}
			consumes := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					consumes = consumes || hardArmPremiseEntryPoints[fun.Name]
				case *ast.SelectorExpr:
					consumes = consumes || hardArmPremiseEntryPoints[fun.Sel.Name]
				}
				return true
			})
			if !consumes {
				continue
			}
			consumers++
			if !wholeFile && !listed[fn.Name.Name] {
				uncensused = append(uncensused, fset.Position(fn.Pos()).String()+" "+fn.Name.Name)
			}
		}
	}
	if consumers < 2 {
		t.Fatalf("expected at least the explore-window and reconcile consumers, found %d — the census lost its subject", consumers)
	}
	if len(uncensused) > 0 {
		sort.Strings(uncensused)
		t.Fatalf("accepted-closure premise consumers must be censused hard-gate bodies (add them to hardArmGateFunctions):\n  %s",
			strings.Join(uncensused, "\n  "))
	}
}

// TestHardArmMutableCarrierCensus_ArmReadsOnlyRegisteredCarriers (a):
// every method call on a *types.MutableState parameter / the bus Mutable
// inside a censused gate body must be a registered getter, and every
// `<recv>.Signals.<field>` read (the bus signals or the criterion env copy
// buildEnv makes of them) must be a registered "signal" (§40.43 R2).
func TestHardArmMutableCarrierCensus_ArmReadsOnlyRegisteredCarriers(t *testing.T) {
	registered := map[string]bool{}
	signals := map[string]bool{}
	for _, c := range hardArmMutableCarriers {
		switch c.kind {
		case "carrier", "closure":
			if c.reset == "" {
				t.Fatalf("%s %q must declare its reset method", c.kind, c.getter)
			}
		case "counter", "retained":
			if c.reset != "" {
				t.Fatalf("%s %q must not declare a reset", c.kind, c.getter)
			}
		case "signal":
			if c.reset == "" {
				t.Fatalf("signal %q must name the fallback arm that resets it", c.getter)
			}
			signals[c.getter] = true
			continue
		case "decision":
			if c.reset != "" || c.advances == "" {
				t.Fatalf("decision %q must name the counter it advances and no reset", c.getter)
			}
		default:
			t.Fatalf("table entry %q has unknown kind %q", c.getter, c.kind)
		}
		registered[c.getter] = true
	}
	for _, c := range hardArmMutableCarriers {
		if c.kind != "decision" {
			continue
		}
		advancesCounter := false
		for _, other := range hardArmMutableCarriers {
			if other.getter == c.advances && other.kind == "counter" {
				advancesCounter = true
			}
		}
		if !advancesCounter {
			t.Fatalf("decision %q advances %q, which is not a registered counter", c.getter, c.advances)
		}
	}
	fset := token.NewFileSet()
	var unregistered []string
	seen := map[string]bool{}
	seenSignals := map[string]bool{}
	for file, functions := range hardArmGateFunctions {
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		wanted := map[string]bool{}
		for _, name := range functions {
			wanted[name] = true
		}
		found := map[string]bool{}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || (len(functions) > 0 && !wanted[fn.Name.Name]) {
				continue
			}
			found[fn.Name.Name] = true
			mutableParams := mutableStateParamNames(fn)
			// Local aliases `mut := o.busCtx.Mutable` read the same carrier.
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if assign, ok := n.(*ast.AssignStmt); ok && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
					if lhs, ok := assign.Lhs[0].(*ast.Ident); ok && hardArmExprIsBusMutable(assign.Rhs[0]) {
						mutableParams[lhs.Name] = true
					}
				}
				return true
			})
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				// `<recv>.Signals.<field>` — a BusContext signal read, either
				// on the bus itself or on a criterion.Env copy of it.
				if inner, ok := sel.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "Signals" {
					seenSignals[sel.Sel.Name] = true
					if !signals[sel.Sel.Name] {
						unregistered = append(unregistered, fset.Position(sel.Pos()).String()+" "+hardArmExprString(sel))
					}
					return true
				}
				return true
			})
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				receiver := ""
				if ident, ok := sel.X.(*ast.Ident); ok && mutableParams[ident.Name] {
					receiver = ident.Name
				} else if hardArmExprIsBusMutable(sel.X) {
					receiver = "o.busCtx.Mutable"
				}
				if receiver == "" {
					return true
				}
				seen[sel.Sel.Name] = true
				if !registered[sel.Sel.Name] {
					unregistered = append(unregistered, fset.Position(call.Pos()).String()+" "+receiver+"."+sel.Sel.Name+"()")
				}
				return true
			})
		}
		for name := range wanted {
			if !found[name] {
				t.Fatalf("hard gate function %s not found in %s — the census lost its subject", name, file)
			}
		}
	}
	if len(seen) < 6 {
		t.Fatalf("the censused hard gates read only %v — the census lost its subject", seen)
	}
	if len(seenSignals) == 0 {
		t.Fatal("the censused hard gates read no BusContext signal — the signal census lost its subject (the reconcile arm's signal door)")
	}
	if len(unregistered) > 0 {
		sort.Strings(unregistered)
		t.Fatalf("hard arm reads carriers that are not in hardArmMutableCarriers (register the getter with its lifecycle reset, a counter kind, or a signal with its reset arm):\n  %s",
			strings.Join(unregistered, "\n  "))
	}
}

// signalArmVerdict is what signalResetSiteVerdict reads off one CaseClause
// for one signal field: whether a TOP-LEVEL unconditional statement of the
// arm body assigns it `false`, whether any assignment of `false` sits
// nested (inside an if / for / block / func literal — guarded or dead
// code, which does not count), and whether the arm assigns it `true`
// anywhere after the clear (clear-then-re-raise).
type signalArmVerdict struct {
	clearedTopLevel bool
	clearedNested   bool
	reraised        bool
}

// signalAssignment returns (field matched, rhs identifier) for an
// AssignStmt of the shape `o.busCtx.Signals.<field> = <ident>`.
func signalAssignment(n ast.Node, field string) (bool, string) {
	assign, ok := n.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return false, ""
	}
	lhs, ok := assign.Lhs[0].(*ast.SelectorExpr)
	if !ok || lhs.Sel.Name != field {
		return false, ""
	}
	inner, ok := lhs.X.(*ast.SelectorExpr)
	if !ok || inner.Sel.Name != "Signals" || !hardArmExprIsBusCtx(inner.X) {
		return false, ""
	}
	rhs, ok := assign.Rhs[0].(*ast.Ident)
	if !ok {
		return true, ""
	}
	return true, rhs.Name
}

// signalResetSiteVerdict (§40.43 F-orch 三轮复核 T-i): the reset-site pin
// is satisfiable only by a top-level unconditional statement of the
// CaseClause body. Nested clears (guarded, looped, func literal) are
// reported separately so the pin can name the shape; a `true` assignment
// to the same field after the clear is a re-raise.
func signalResetSiteVerdict(cc *ast.CaseClause, field string) signalArmVerdict {
	var v signalArmVerdict
	var clearPos token.Pos
	for _, stmt := range cc.Body {
		if matched, rhs := signalAssignment(stmt, field); matched && rhs == "false" {
			v.clearedTopLevel = true
			if clearPos == 0 {
				clearPos = stmt.Pos()
			}
		}
	}
	for _, stmt := range cc.Body {
		ast.Inspect(stmt, func(n ast.Node) bool {
			matched, rhs := signalAssignment(n, field)
			if !matched {
				return true
			}
			switch rhs {
			case "false":
				if n.Pos() != clearPos && !isTopLevelStmt(cc, n) {
					v.clearedNested = true
				}
			case "true":
				if clearPos == 0 || n.Pos() > clearPos {
					v.reraised = true
				}
			}
			return true
		})
	}
	return v
}

// isTopLevelStmt reports whether n is one of the CaseClause's direct body
// statements.
func isTopLevelStmt(cc *ast.CaseClause, n ast.Node) bool {
	for _, stmt := range cc.Body {
		if stmt.Pos() == n.Pos() {
			return true
		}
	}
	return false
}

// TestHardArmMutableCarrierCensus_SignalResetSiteIsTheBacktrackArm (§40.43
// R2, hardened by §40.43 F-orch 三轮复核 T-i): every registered "signal" is
// assigned false by a TOP-LEVEL unconditional statement of the named
// CaseClause of runReadSchedulerLoop's fallback switch — no enclosing if /
// for / func literal, never re-raised to true later in the arm — and is
// not cleared at any depth in any other arm (finalizer-only / extract
// fallbacks never re-open exploration and must not drop the signal).
// Precise: an AssignStmt whose LHS is `o.busCtx.Signals.<field>` and whose
// RHS is the identifier `false`. The checker's own red cases are pinned by
// TestSignalResetSiteVerdict_SelfRed.
func TestHardArmMutableCarrierCensus_SignalResetSiteIsTheBacktrackArm(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "orchestrator.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	sw := fallbackSwitchOf(t, f)
	signalsChecked := 0
	for _, c := range hardArmMutableCarriers {
		if c.kind != "signal" {
			continue
		}
		signalsChecked++
		resetArms := 0
		for _, stmt := range sw.Body.List {
			cc, ok := stmt.(*ast.CaseClause)
			if !ok {
				continue
			}
			isResetArm := caseClauseNames(cc, c.reset)
			v := signalResetSiteVerdict(cc, c.getter)
			if !isResetArm {
				if v.clearedTopLevel || v.clearedNested {
					t.Fatalf("signal %q is cleared in a fallback arm other than %s (%v) — only the explore backtrack re-opens the window", c.getter, c.reset, fset.Position(cc.Pos()))
				}
				continue
			}
			if !v.clearedTopLevel {
				t.Fatalf("signal %q must be assigned false by a top-level unconditional statement of the %s arm of runReadSchedulerLoop's fallback switch (%v; nested/guarded clear=%t) — the stale pre-backtrack signal keeps the reconcile auto-complete door open while the veto is in force",
					c.getter, c.reset, fset.Position(cc.Pos()), v.clearedNested)
			}
			if v.clearedNested {
				t.Fatalf("signal %q: the %s arm also clears the signal inside a guarded / nested statement (%v) — the clear must be one top-level unconditional statement", c.getter, c.reset, fset.Position(cc.Pos()))
			}
			if v.reraised {
				t.Fatalf("signal %q is cleared and then re-raised to true inside the %s arm (%v) — the explorer or the typed exhaustion decision re-earns it, never the arm", c.getter, c.reset, fset.Position(cc.Pos()))
			}
			resetArms++
		}
		if resetArms != 1 {
			t.Fatalf("signal %q: expected exactly one reset arm named %s, found %d", c.getter, c.reset, resetArms)
		}
	}
	if signalsChecked == 0 {
		t.Fatal("no signal registered — the reset-site census lost its subject")
	}
}

// fallbackSwitchOf returns the `switch fallback {` statement of
// runReadSchedulerLoop (the finalize contract-failure dispatch).
func fallbackSwitchOf(t *testing.T, f *ast.File) *ast.SwitchStmt {
	t.Helper()
	var loop *ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == "runReadSchedulerLoop" {
			loop = fd
		}
	}
	if loop == nil || loop.Body == nil {
		t.Fatal("runReadSchedulerLoop not found")
	}
	var found []*ast.SwitchStmt
	ast.Inspect(loop.Body, func(n ast.Node) bool {
		if sw, ok := n.(*ast.SwitchStmt); ok {
			if tag, ok := sw.Tag.(*ast.Ident); ok && tag.Name == "fallback" {
				found = append(found, sw)
			}
		}
		return true
	})
	if len(found) != 1 {
		t.Fatalf("expected exactly one `switch fallback` in runReadSchedulerLoop, found %d", len(found))
	}
	return found[0]
}

// caseClauseNames reports whether the CaseClause lists the identifier
// `name` among its case expressions.
func caseClauseNames(cc *ast.CaseClause, name string) bool {
	for _, expr := range cc.List {
		if id, ok := expr.(*ast.Ident); ok && id.Name == name {
			return true
		}
	}
	return false
}

// TestHardArmMutableCarrierCensus_CarrierResetsHaveProductionCallers (b):
// every registered getter/reset is a real MutableState method, and every
// carrier's reset has at least one non-test call site under internal/
// or cmd/. Signals are checked by
// TestHardArmMutableCarrierCensus_SignalResetSiteIsTheBacktrackArm.
func TestHardArmMutableCarrierCensus_CarrierResetsHaveProductionCallers(t *testing.T) {
	methods := mutableStateMethodNames(t)
	callSites := map[string][]string{}
	for _, root := range []string{filepath.Join("..", "..", "internal"), filepath.Join("..", "..", "cmd")} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					callSites[sel.Sel.Name] = append(callSites[sel.Sel.Name], fset.Position(call.Pos()).String())
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	for _, c := range hardArmMutableCarriers {
		if c.kind == "signal" {
			if !executionSignalsFieldNames(t)[c.getter] {
				t.Errorf("signal %q is not a types.ExecutionSignals field", c.getter)
			}
			continue
		}
		if !methods[c.getter] {
			t.Errorf("%s: getter %q is not a MutableState method", c.kind, c.getter)
		}
		if c.kind == "decision" {
			// A typed decision has exactly one production site: the
			// scheduler lane that observed the condition it records.
			if n := len(callSites[c.getter]); n != 1 {
				t.Errorf("decision %q must have exactly one non-test call site under internal/ and cmd/, found %d: %v", c.getter, n, callSites[c.getter])
			}
			continue
		}
		if c.kind == "closure" {
			// The closure's own clear method must have a production caller.
			if len(callSites[c.reset]) == 0 {
				t.Errorf("closure %q: reset %q has zero non-test call sites", c.getter, c.reset)
			}
			continue
		}
		if c.kind != "carrier" {
			continue
		}
		if !methods[c.reset] {
			t.Errorf("carrier %q: reset %q is not a MutableState method", c.getter, c.reset)
			continue
		}
		if len(callSites[c.reset]) == 0 {
			t.Errorf("carrier %q: reset %q has zero non-test call sites under internal/ and cmd/ — a hard-arm carrier whose reset never runs in production is a stale signal that can never lift (§40.14 V7-2)",
				c.getter, c.reset)
		}
	}
}

// mutableStateParamNames returns the parameter names of fn typed
// *types.MutableState (or *MutableState).
func mutableStateParamNames(fn *ast.FuncDecl) map[string]bool {
	out := map[string]bool{}
	if fn.Type == nil || fn.Type.Params == nil {
		return out
	}
	for _, field := range fn.Type.Params.List {
		star, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		name := ""
		switch x := star.X.(type) {
		case *ast.SelectorExpr:
			name = x.Sel.Name
		case *ast.Ident:
			name = x.Name
		}
		if name != "MutableState" {
			continue
		}
		for _, id := range field.Names {
			out[id.Name] = true
		}
	}
	return out
}

// mutableStateMethodNames parses ../types and returns every method
// declared on *MutableState.
func mutableStateMethodNames(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, decl := range typesPackageDecls(t) {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if id, ok := star.X.(*ast.Ident); ok && id.Name == "MutableState" {
			out[fn.Name.Name] = true
		}
	}
	return out
}

// executionSignalsFieldNames parses ../types and returns the field names
// of the ExecutionSignals struct.
func executionSignalsFieldNames(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, decl := range typesPackageDecls(t) {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != "ExecutionSignals" {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					out[name.Name] = true
				}
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("types.ExecutionSignals not found — the signal census lost its subject")
	}
	return out
}

func typesPackageDecls(t *testing.T) []ast.Decl {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("..", "types", "*.go"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("glob ../types: %v (matches=%d)", err, len(matches))
	}
	fset := token.NewFileSet()
	var out []ast.Decl
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		out = append(out, f.Decls...)
	}
	return out
}

// hardArmExprIsBusMutable matches the `o.busCtx.Mutable` receiver chain.
func hardArmExprIsBusMutable(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Mutable" {
		return false
	}
	return hardArmExprIsBusCtx(sel.X)
}

// hardArmExprIsBusCtx matches the `o.busCtx` receiver chain.
func hardArmExprIsBusCtx(expr ast.Expr) bool {
	inner, ok := expr.(*ast.SelectorExpr)
	if !ok || inner.Sel.Name != "busCtx" {
		return false
	}
	ident, ok := inner.X.(*ast.Ident)
	return ok && ident.Name == "o"
}

// hardArmExprString renders a selector chain for diagnostics.
func hardArmExprString(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.SelectorExpr:
		return hardArmExprString(x.X) + "." + x.Sel.Name
	case *ast.Ident:
		return x.Name
	case *ast.CallExpr:
		return hardArmExprString(x.Fun) + "()"
	}
	return "<expr>"
}
