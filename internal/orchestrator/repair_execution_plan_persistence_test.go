package orchestrator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/types"
)

// repair_execution_plan_persistence_test.go — §40.39 rationale correction
// (F12). The RepairExecutionPlan persists across finalize attempts so the
// next failure's cluster closure counts StableAttempts against it, and a
// stuck deepest owner exits through FallbackFailLoud. Before F12
// ResetForFallback(Finalizer/Extract/Explore) cleared the plan right after
// every Advance, so prevPlan was nil on every failure and a stuck owner
// cycled until maxUpstreamFallbacksPerRun instead.
//
// EVOLUTION RECORD (§40.43 R1, fold-in round three): the persisted plan
// now feeds ONLY stability accounting and the stuck exit — the dispatch
// target is a fresh rebuild every round (repair_execution_plan_dispatch_test.go).
// The semantics pinned here (StableAttempts 0..budget-1 across resets,
// budget+1 → FailLoud with the stuck owner and an empty queue, a new
// cluster key restarts at zero) are unchanged; they now hold through the
// carry-over of StableAttempts across rebuilds instead of a stay arm.
//
// The replay below performs the scheduler's per-failure production sequence
// (AdvanceRepairExecutionPlan → populateRetryState → ResetForFallback(target
// of the chosen fallback)) on a real MutableState. The go/ast pin at the
// bottom keeps the replay faithful to runReadSchedulerLoop: Advance is
// called once before the `switch fallback`, and inside every arm that
// resets the Mutable the populate and the reset are TOP-LEVEL unconditional
// statements with populate first (§40.43 F-orch 三轮复核 T-i; the earlier
// whole-loop first-occurrence pin TestRunReadSchedulerLoop_FinalizeFailureSequenceOrder
// was retired in fold-in round three because a single reordered arm stayed
// green under it).

// replayFinalizeContractFailure mirrors the FallbackFinalizerOnly /
// BackToExtract / BackToExplore arms of runReadSchedulerLoop for one failed
// finalize attempt over `fresh`. FailLoud mirrors the scheduler too: no
// populate, no reset (the run terminates).
func replayFinalizeContractFailure(t *testing.T, mut *types.MutableState, fresh []types.Violation, finalizerLocalUsed *int) (RepairExecutionPlan, FallbackTarget) {
	t.Helper()
	plan, fallback, preDowngrade := AdvanceRepairExecutionPlan(mut, fresh, *finalizerLocalUsed)
	if fallback == FallbackFinalizerOnly && preDowngrade != FallbackFinalizerOnly {
		*finalizerLocalUsed++
	}
	if fallback == FallbackFailLoud {
		return plan, fallback
	}
	prevAttempt := 0
	if rs := mut.RetryState(); rs != nil {
		prevAttempt = rs.Attempt
	}
	populateRetryState(mut, contract.Result{Violations: fresh}, prevAttempt)
	switch fallback {
	case FallbackFinalizerOnly:
		mut.ResetForFallback(types.FallbackResetTargetFinalizer)
	case FallbackBackToExtract:
		mut.ResetForFallback(types.FallbackResetTargetExtract)
	case FallbackBackToExplore:
		mut.ResetForFallback(types.FallbackResetTargetExplore)
		mut.ResetInvestigationComplete()
	default:
		t.Fatalf("replay: unexpected fallback %v", fallback)
	}
	return plan, fallback
}

// stuckClusterCases: one single-cluster violation per owner locus. Each
// fingerprint is stable across attempts (typed detail token), so the same
// cluster identity recurs on every failure.
func stuckClusterCases() []struct {
	name     string
	fresh    types.Violation
	owner    RepairLocus
	fallback FallbackTarget
} {
	return []struct {
		name     string
		fresh    types.Violation
		owner    RepairLocus
		fallback FallbackTarget
	}{
		{name: "finalizer-owned", fresh: vBlock("summary"), owner: LocusFinalizer, fallback: FallbackFinalizerOnly},
		{name: "extract-owned", fresh: types.Violation{Kind: types.ViolSubjectAnchorMissing, Detail: `principal block "summary" has no subject anchor`}, owner: LocusExtract, fallback: FallbackBackToExtract},
		{name: "explore-owned", fresh: vFacet("diagram_spine"), owner: LocusExplore, fallback: FallbackBackToExplore},
	}
}

// (F12 a) red→green: the SAME stable cluster failing ClusterStableBudget()+1
// consecutive finalize attempts reaches FallbackFailLoud through the
// persisted plan (stay … stay → stuck → fail-loud), for every owner locus
// and therefore every ResetForFallback target. On the untouched code the
// plan was cleared by each reset, every attempt rebuilt, and the sequence
// never left the owner's own fallback.
func TestRepairExecutionPlan_StuckClusterReachesFailLoudAcrossResets(t *testing.T) {
	budget := ClusterStableBudget()
	if budget < 1 {
		t.Fatalf("fixture: ClusterStableBudget()=%d", budget)
	}
	for _, tc := range stuckClusterCases() {
		t.Run(tc.name, func(t *testing.T) {
			mut := types.NewMutableState(tc.name)
			mut.SetInvestigationComplete("accepted closure before the finalize chain")
			used := 0
			var last RepairExecutionPlan
			var lastFallback FallbackTarget
			for attempt := 1; attempt <= budget; attempt++ {
				last, lastFallback = replayFinalizeContractFailure(t, mut, []types.Violation{tc.fresh}, &used)
				if lastFallback != tc.fallback || last.HasFailLoud {
					t.Fatalf("attempt %d: fallback=%v fail_loud=%t, want %v within the stable budget", attempt, lastFallback, last.HasFailLoud, tc.fallback)
				}
				if last.CurrentOwner != tc.owner {
					t.Fatalf("attempt %d: CurrentOwner=%v, want %v", attempt, last.CurrentOwner, tc.owner)
				}
				stashed, ok := mut.RepairExecutionPlan().(RepairExecutionPlan)
				if !ok {
					t.Fatalf("attempt %d: the execution plan must persist across ResetForFallback(%v) so the next failure classifies against it; got %T",
						attempt, tc.fallback, mut.RepairExecutionPlan())
				}
				if len(stashed.ClusterStates) != 1 {
					t.Fatalf("attempt %d: single-cluster plan expected, got %+v", attempt, stashed.ClusterStates)
				}
				if got := stashed.ClusterStates[0].StableAttempts; got != attempt-1 {
					t.Fatalf("attempt %d: StableAttempts=%d, want %d (closure counted against the persisted plan)", attempt, got, attempt-1)
				}
			}
			last, lastFallback = replayFinalizeContractFailure(t, mut, []types.Violation{tc.fresh}, &used)
			if lastFallback != FallbackFailLoud || !last.HasFailLoud {
				t.Fatalf("attempt %d (budget %d exhausted): fallback=%v fail_loud=%t, want FallbackFailLoud — a stuck deepest owner must exit through the cluster-closure fail-loud path, not cycle on its own fallback",
					budget+1, budget, lastFallback, last.HasFailLoud)
			}
			if last.CurrentOwner != tc.owner || len(last.RemainingOwners) != 0 {
				t.Fatalf("fail-loud plan must keep the stuck owner with an empty queue, got %+v", last)
			}
		})
	}
}

// (F12 a) a DIFFERENT cluster still rebuilds: after the finalizer-owned
// cluster has accumulated stable attempts, a fresh (kind, fingerprint) the
// persisted plan has never seen drops the old plan and rebuilds from the
// fresh set — new owner, zero stable attempts, no fail-loud — and that new
// cluster then walks its own budget to fail-loud.
func TestRepairExecutionPlan_DifferentClusterRebuildsPersistedPlan(t *testing.T) {
	budget := ClusterStableBudget()
	mut := types.NewMutableState("different cluster rebuilds")
	mut.SetInvestigationComplete("accepted closure before the finalize chain")
	used := 0
	for attempt := 1; attempt <= budget; attempt++ {
		plan, fallback := replayFinalizeContractFailure(t, mut, []types.Violation{vBlock("summary")}, &used)
		if fallback != FallbackFinalizerOnly || plan.CurrentOwner != LocusFinalizer {
			t.Fatalf("attempt %d: fallback=%v owner=%v, want finalizer-only", attempt, fallback, plan.CurrentOwner)
		}
	}
	stashed, ok := mut.RepairExecutionPlan().(RepairExecutionPlan)
	if !ok || len(stashed.ClusterStates) != 1 || stashed.ClusterStates[0].StableAttempts != budget-1 {
		t.Fatalf("fixture: the persisted plan must carry %d stable attempts, got %+v (present=%t)", budget-1, stashed.ClusterStates, ok)
	}

	plan, fallback := replayFinalizeContractFailure(t, mut, []types.Violation{vFacet("diagram_spine")}, &used)
	if plan.HasFailLoud || fallback != FallbackBackToExplore || plan.CurrentOwner != LocusExplore {
		t.Fatalf("a new (kind, fingerprint) must rebuild the plan for the fresh cluster: fallback=%v owner=%v fail_loud=%t", fallback, plan.CurrentOwner, plan.HasFailLoud)
	}
	if len(plan.ClusterStates) != 1 || plan.ClusterStates[0].PrimaryKind != types.ViolFacetUncovered || plan.ClusterStates[0].StableAttempts != 0 {
		t.Fatalf("rebuilt plan must start the new cluster at zero stable attempts, got %+v", plan.ClusterStates)
	}
	for attempt := 2; attempt <= budget; attempt++ {
		plan, fallback = replayFinalizeContractFailure(t, mut, []types.Violation{vFacet("diagram_spine")}, &used)
		if fallback != FallbackBackToExplore || plan.HasFailLoud {
			t.Fatalf("new cluster attempt %d: fallback=%v fail_loud=%t, want explore within budget", attempt, fallback, plan.HasFailLoud)
		}
	}
	plan, fallback = replayFinalizeContractFailure(t, mut, []types.Violation{vFacet("diagram_spine")}, &used)
	if fallback != FallbackFailLoud || !plan.HasFailLoud {
		t.Fatalf("the rebuilt cluster must reach fail-loud after its own budget, got fallback=%v fail_loud=%t", fallback, plan.HasFailLoud)
	}
}

// callNameOf returns the bare callee name of a CallExpr node ("" otherwise).
func callNameOf(n ast.Node) string {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return ""
	}
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}
	return ""
}

// fallbackArmVerdict is what fallbackArmPopulateResetVerdict reads off one
// CaseClause: the positions of the TOP-LEVEL populateRetryState and
// ResetForFallback statements (an ExprStmt call, or an AssignStmt whose
// single RHS is the call — `cleared := ...ResetForFallback(...)`), and
// whether either call also appears nested inside an if / for / block /
// func literal (guarded or dead code, which does not satisfy the pin).
type fallbackArmVerdict struct {
	populatePos, resetPos       token.Pos
	nestedPopulate, nestedReset bool
}

func topLevelCallPos(stmt ast.Stmt, name string) token.Pos {
	switch x := stmt.(type) {
	case *ast.ExprStmt:
		if callNameOf(x.X) == name {
			return x.Pos()
		}
	case *ast.AssignStmt:
		if len(x.Rhs) == 1 && callNameOf(x.Rhs[0]) == name {
			return x.Pos()
		}
	}
	return 0
}

// fallbackArmPopulateResetVerdict (§40.43 F-orch 三轮复核 T-i): only
// top-level unconditional statements of the CaseClause body count; nested
// occurrences are reported so the pin can name the shape.
func fallbackArmPopulateResetVerdict(cc *ast.CaseClause) fallbackArmVerdict {
	var v fallbackArmVerdict
	for _, stmt := range cc.Body {
		if p := topLevelCallPos(stmt, "populateRetryState"); p != 0 && v.populatePos == 0 {
			v.populatePos = p
		}
		if p := topLevelCallPos(stmt, "ResetForFallback"); p != 0 && v.resetPos == 0 {
			v.resetPos = p
		}
	}
	for _, stmt := range cc.Body {
		topPopulate := topLevelCallPos(stmt, "populateRetryState")
		topReset := topLevelCallPos(stmt, "ResetForFallback")
		ast.Inspect(stmt, func(n ast.Node) bool {
			switch callNameOf(n) {
			case "populateRetryState":
				if topPopulate == 0 {
					v.nestedPopulate = true
				}
			case "ResetForFallback":
				if topReset == 0 {
					v.nestedReset = true
				}
			}
			return true
		})
	}
	return v
}

// Replay fidelity (§40.43 R3 E ii, PER ARM; hardened by §40.43 F-orch 三轮
// 复核 T-i): AdvanceRepairExecutionPlan is called exactly once in
// runReadSchedulerLoop and precedes the `switch fallback` dispatch; inside
// that switch, EVERY CaseClause that resets the Mutable does so with a
// TOP-LEVEL unconditional ResetForFallback statement preceded by a
// TOP-LEVEL unconditional populateRetryState statement in the SAME arm (the
// retry state must be captured before the reset clears the answer slate),
// and neither call appears guarded / nested anywhere in the arm. A
// first-occurrence comparison across the whole loop is not enough — a
// single reordered arm stays green under it (proved red by reordering the
// BackToExtract arm in a scratch copy); a nested-aware search is not enough
// either — a guarded or func-literal reset stayed green under it. The
// checker's own red shapes are pinned by TestFallbackArmPopulateResetVerdict_SelfRed.
func TestRunReadSchedulerLoop_FallbackArmsPopulateBeforeReset(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "orchestrator.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var loop *ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == "runReadSchedulerLoop" {
			loop = fd
		}
	}
	if loop == nil || loop.Body == nil {
		t.Fatal("runReadSchedulerLoop not found — the replay lost its subject")
	}
	var advancePos []token.Pos
	ast.Inspect(loop.Body, func(n ast.Node) bool {
		if callNameOf(n) == "AdvanceRepairExecutionPlan" {
			advancePos = append(advancePos, n.Pos())
		}
		return true
	})
	sw := fallbackSwitchOf(t, f)
	if len(advancePos) != 1 || advancePos[0] > sw.Pos() {
		t.Fatalf("AdvanceRepairExecutionPlan must be called exactly once, before the fallback switch (calls=%d)", len(advancePos))
	}
	armsWithReset := 0
	for _, stmt := range sw.Body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		v := fallbackArmPopulateResetVerdict(cc)
		if v.nestedReset || v.nestedPopulate {
			t.Fatalf("fallback arm at %v calls populateRetryState / ResetForFallback inside a guarded or nested statement (nested populate=%t reset=%t) — both must be top-level unconditional statements of the arm",
				fset.Position(cc.Pos()), v.nestedPopulate, v.nestedReset)
		}
		if v.resetPos == 0 {
			continue
		}
		armsWithReset++
		if v.populatePos == 0 || v.populatePos > v.resetPos {
			t.Fatalf("fallback arm at %v resets the Mutable (%v) without an earlier top-level populateRetryState in the same arm (populate at %v) — the retry state must be captured before the reset clears the answer slate",
				fset.Position(cc.Pos()), fset.Position(v.resetPos), fset.Position(v.populatePos))
		}
	}
	if armsWithReset < 3 {
		t.Fatalf("expected the finalizer-only / extract / explore arms to reset the Mutable, found %d arms calling ResetForFallback", armsWithReset)
	}
}

// TestFallbackArmPopulateResetVerdict_SelfRed (T-i): the checker rejects
// the guarded, func-literal and reset-before-populate shapes and accepts the
// production shapes (ExprStmt reset; `cleared := ...` assignment reset).
func TestFallbackArmPopulateResetVerdict_SelfRed(t *testing.T) {
	cases := []struct {
		name          string
		body          string
		wantOK        bool
		wantNestedPop bool
		wantNestedRst bool
	}{
		{name: "production: populate then reset", body: "populateRetryState(m, r, a)\n\t\tm.ResetForFallback(target)", wantOK: true},
		{name: "production: populate then assigned reset", body: "populateRetryState(m, r, a)\n\t\tcleared := m.ResetForFallback(target)\n\t\t_ = cleared", wantOK: true},
		{name: "guarded reset", body: "populateRetryState(m, r, a)\n\t\tif m != nil { m.ResetForFallback(target) }", wantNestedRst: true},
		{name: "func-literal populate", body: "func() { populateRetryState(m, r, a) }()\n\t\tm.ResetForFallback(target)", wantNestedPop: true},
		{name: "reset before populate", body: "m.ResetForFallback(target)\n\t\tpopulateRetryState(m, r, a)"},
		{name: "reset without populate", body: "m.ResetForFallback(target)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "package p\n\nfunc f() {\n\tswitch fallback {\n\tcase FallbackFinalizerOnly:\n\t\t" + tc.body + "\n\t}\n}\n"
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "snippet.go", src, 0)
			if err != nil {
				t.Fatalf("parse snippet: %v", err)
			}
			var cc *ast.CaseClause
			ast.Inspect(f, func(n ast.Node) bool {
				if c, ok := n.(*ast.CaseClause); ok {
					cc = c
				}
				return true
			})
			v := fallbackArmPopulateResetVerdict(cc)
			ok := !v.nestedPopulate && !v.nestedReset && v.resetPos != 0 && v.populatePos != 0 && v.populatePos < v.resetPos
			if ok != tc.wantOK || v.nestedPopulate != tc.wantNestedPop || v.nestedReset != tc.wantNestedRst {
				t.Fatalf("verdict %+v (ok=%t), want ok=%t nestedPopulate=%t nestedReset=%t", v, ok, tc.wantOK, tc.wantNestedPop, tc.wantNestedRst)
			}
		})
	}
}

// Writer census (§40.43 R3 E i): the persisted plan has exactly two
// production writers — AdvanceRepairExecutionPlan (stash) and
// closeFinalizeRetryChain (clear, via ResetRetryState). A plan clear
// re-added in a scheduler arm (or anywhere else) is red: the plan must
// persist across ResetForFallback so stability keeps counting.
func TestRepairExecutionPlan_ProductionWritersAreAdvanceAndChainClose(t *testing.T) {
	allowed := map[string]bool{"AdvanceRepairExecutionPlan": true, "closeFinalizeRetryChain": true}
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob: %v", err)
	}
	fset := token.NewFileSet()
	var stray []string
	writers := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || (sel.Sel.Name != "SetRepairExecutionPlan" && sel.Sel.Name != "ResetRepairExecutionPlan") {
					return true
				}
				writers++
				if !allowed[fd.Name.Name] {
					stray = append(stray, fset.Position(call.Pos()).String()+" "+fd.Name.Name+"."+sel.Sel.Name)
				}
				return true
			})
		}
	}
	if writers == 0 {
		t.Fatal("no RepairExecutionPlan writer found — the census lost its subject")
	}
	if len(stray) > 0 {
		sort.Strings(stray)
		t.Fatalf("RepairExecutionPlan may be written only by AdvanceRepairExecutionPlan / closeFinalizeRetryChain, found:\n  %s", strings.Join(stray, "\n  "))
	}
}
