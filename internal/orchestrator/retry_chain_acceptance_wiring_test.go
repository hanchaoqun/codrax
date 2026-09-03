package orchestrator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// retry_chain_acceptance_wiring_test.go — §40.14 V7-2 复核: the production
// reset pair is wired, not merely defined.
//
// EVOLUTION RECORD (F13 fold-in, §40.39 → §40.42 follow-up): the original pin
// was a structural lower bound — it counted CallExpr names only, so a call
// nested under a never-true branch inside acceptFinalizeNode passed, and the
// "≥ 4 acceptFinalizeNode calls in orchestrator.go" count accepted calls from
// ANY function (reverting the four scheduler exits and adding a dead helper
// with four calls passed). Both halves are now pinned against those exact
// variants: (a) a behavioral pin that acceptFinalizeNode actually clears the
// carriers, (b) the close call must be a direct top-level statement of
// acceptFinalizeNode's body, (c) the ≥ 4 count is restricted to the scheduler
// loop that owns the finalize acceptance exits (runReadSchedulerLoop) and no
// other non-test function in the package may call acceptFinalizeNode.
//
// EVOLUTION RECORD (§40.43 R3 G/H, fold-in round three): (a) seeds BOTH a
// finalizer-owned and an explore-owned RetryState (a table) — the explore-
// owned one is the carrier the backtrack veto reads; (b) additionally forbids
// any ReturnStmt (direct or nested) before the closeFinalizeRetryChain
// statement, so an early-return guard cannot skip the close; (c) counts only
// calls outside function literals — a FuncLit body inside the scheduler loop
// is not one of its acceptance exits.

// finalizeAcceptanceExitOwner is the scheduler function whose four contract
// acceptance branches (strict-review-disabled accept, arbitration-restored
// first draft, contract pass, soft-only accept) close the retry chain.
const finalizeAcceptanceExitOwner = "runReadSchedulerLoop"

// (a) behavioral: an accepted finalize node leaves no retry-chain carrier
// behind — RetryState and its paired RepairExecutionPlan are both nil — for
// a finalizer-owned chain and for an explore-owned (backtrack-bound) chain.
func TestAcceptFinalizeNode_ClosesRetryChainCarriers(t *testing.T) {
	cases := []struct {
		name string
		seed func(mut *types.MutableState)
	}{
		{name: "finalizer-owned retry state", seed: func(mut *types.MutableState) {
			mut.SetRetryState(&types.RetryState{Attempt: 2, LastPrimaryOwner: string(LocusFinalizer),
				ActiveViolations: []types.ScoredViolation{{Kind: types.ViolSelfContradiction, Severity: types.SeverityHigh, Layer: "contract_check"}}})
			mut.ResetForFallback(types.FallbackResetTargetFinalizer)
		}},
		{name: "explore-owned retry state bound by a backtrack", seed: func(mut *types.MutableState) {
			mut.SetInvestigationComplete("first accepted closure")
			productionExploreBacktrack(t, mut, exploreOwnedRetryState())
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mut := types.NewMutableState("accept finalize node")
			tc.seed(mut)
			mut.SetRepairExecutionPlan(RepairExecutionPlan{CurrentOwner: LocusFinalizer, EscalationAllowed: true})
			if mut.RetryState() == nil || mut.RepairExecutionPlan() == nil {
				t.Fatal("fixture: both carriers must be live before acceptance")
			}
			graph := types.TaskGraph{Nodes: []types.TaskNode{{ID: "n_finalize", Type: types.NodeFinalize}}}
			state := newGraphState(graph)
			fin := &state.graph.Nodes[0]
			state.markRunning(fin.ID)
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

			o.acceptFinalizeNode(state, fin, &agent.StageOutput{FinalAnswer: "accepted"})

			if mut.RetryState() != nil {
				t.Fatal("acceptFinalizeNode must clear the RetryState carrier")
			}
			if mut.RepairExecutionPlan() != nil {
				t.Fatal("acceptFinalizeNode must clear the paired RepairExecutionPlan")
			}
			if state.nodeStatus(fin.ID) != nodeDone {
				t.Fatalf("acceptFinalizeNode must mark the finalize node done, got %v", state.nodeStatus(fin.ID))
			}
			if acceptedClosureHasActiveExploreContractBacktrack(mut) {
				t.Fatal("an accepted answer closes the chain: no backtrack veto may survive it")
			}
		})
	}
}

// (b) + (c) structural.
func TestFinalizeAcceptanceExitsCloseTheRetryChain(t *testing.T) {
	fset := token.NewFileSet()
	parse := func(file string) *ast.File {
		t.Helper()
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		return f
	}
	funcDecl := func(f *ast.File, name string) *ast.FuncDecl {
		t.Helper()
		for _, decl := range f.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == name && fd.Body != nil {
				return fd
			}
		}
		t.Fatalf("function %s not found — the pin lost its subject", name)
		return nil
	}
	callName := func(expr ast.Expr) string {
		call, ok := expr.(*ast.CallExpr)
		if !ok {
			return ""
		}
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			return fun.Sel.Name
		case *ast.Ident:
			return fun.Name
		}
		return ""
	}
	// countCalls counts calls named `name` in body, NOT descending into
	// function literals — a call inside a closure is not a statement of
	// the enclosing function.
	countCalls := func(body ast.Node, name string) int {
		n := 0
		ast.Inspect(body, func(node ast.Node) bool {
			if _, ok := node.(*ast.FuncLit); ok {
				return false
			}
			if expr, ok := node.(ast.Expr); ok && callName(expr) == name {
				n++
			}
			return true
		})
		return n
	}

	// (b) closeFinalizeRetryChain is a direct top-level statement of
	// acceptFinalizeNode's body — not nested under if/for/switch/func
	// literal — appears exactly once, and NO return statement (direct or
	// nested) precedes it: an early-return guard must not skip the close.
	accept := funcDecl(parse("retry_state.go"), "acceptFinalizeNode")
	topLevel := 0
	var closePos token.Pos
	for _, stmt := range accept.Body.List {
		if es, ok := stmt.(*ast.ExprStmt); ok && callName(es.X) == "closeFinalizeRetryChain" {
			topLevel++
			closePos = es.Pos()
		}
	}
	if total := countCalls(accept.Body, "closeFinalizeRetryChain"); total != 1 || topLevel != 1 {
		t.Fatalf("acceptFinalizeNode must call closeFinalizeRetryChain exactly once as an unconditional top-level statement (top-level=%d total=%d)", topLevel, total)
	}
	ast.Inspect(accept.Body, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		if ret, ok := node.(*ast.ReturnStmt); ok && ret.Pos() < closePos {
			t.Fatalf("acceptFinalizeNode returns at %v before closing the retry chain at %v — every accepted answer must close the chain", fset.Position(ret.Pos()), fset.Position(closePos))
		}
		return true
	})

	// (c) the four acceptance exits live in the owning scheduler loop
	// itself (not inside any function literal it declares) …
	owner := funcDecl(parse("orchestrator.go"), finalizeAcceptanceExitOwner)
	if n := countCalls(owner.Body, "acceptFinalizeNode"); n < 4 {
		t.Fatalf("%s must route its finalize acceptance exits through acceptFinalizeNode (found %d calls outside function literals, want ≥ 4)", finalizeAcceptanceExitOwner, n)
	}
	// … and nowhere else in the package's production code (function
	// literals included — a closure calling it is a stray caller too).
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob package files: %v (n=%d)", err, len(files))
	}
	var stray []string
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		for _, decl := range parse(file).Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			n := 0
			ast.Inspect(fd.Body, func(node ast.Node) bool {
				if fl, ok := node.(*ast.FuncLit); ok && fd.Name.Name == finalizeAcceptanceExitOwner {
					// Closures inside the owner are strays too — count them
					// separately from the owner's own exits.
					if countCallsIncludingLits(fl.Body, "acceptFinalizeNode", callName) > 0 {
						stray = append(stray, fset.Position(fl.Pos()).String()+" func literal in "+fd.Name.Name)
					}
					return false
				}
				if expr, ok := node.(ast.Expr); ok && callName(expr) == "acceptFinalizeNode" {
					n++
				}
				return true
			})
			if fd.Name.Name != finalizeAcceptanceExitOwner && n > 0 {
				stray = append(stray, fset.Position(fd.Pos()).String()+" "+fd.Name.Name)
			}
		}
	}
	if len(stray) > 0 {
		t.Fatalf("acceptFinalizeNode is the scheduler's acceptance exit; only %s may call it, found callers:\n  %s",
			finalizeAcceptanceExitOwner, strings.Join(stray, "\n  "))
	}
}

// countCallsIncludingLits counts calls named `name` anywhere under body,
// function literals included.
func countCallsIncludingLits(body ast.Node, name string, callName func(ast.Expr) string) int {
	n := 0
	ast.Inspect(body, func(node ast.Node) bool {
		if expr, ok := node.(ast.Expr); ok && callName(expr) == name {
			n++
		}
		return true
	})
	return n
}
