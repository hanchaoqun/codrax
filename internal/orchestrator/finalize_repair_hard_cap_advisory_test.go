package orchestrator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// finalize_repair_hard_cap_advisory_test.go — §40.43 R1 finding C: the
// cluster-closure fail-loud exit fires on failure ClusterStableBudget()+1,
// but the P6 finalize repair hard cap (retryUsed >= hardCap) and the W2.6
// per-root cap both break the loop BEFORE AdvanceRepairExecutionPlan. At
// the default caps (stable 2 / hard cap 2 / per-root 3) P6 pre-empts on the
// third failure; the exit is a backstop for raised caps. The advisory is a
// log line over two exact integer comparisons — never a gate.

func TestClusterClosureExitReachability_ExactComparisons(t *testing.T) {
	cases := []struct {
		name                     string
		stable, hardCap, perRoot int
		wantUnreachable          bool
		wantReasonContains       string
	}{
		{name: "defaults: P6 pre-empts on the third failure", stable: 2, hardCap: 2, perRoot: 3, wantUnreachable: true, wantReasonContains: "hard cap 2"},
		{name: "P6 raised, per-root still pre-empts", stable: 2, hardCap: 3, perRoot: 3, wantUnreachable: true, wantReasonContains: "per-root attempt cap 3"},
		{name: "both raised: exit reachable", stable: 2, hardCap: 3, perRoot: 4, wantUnreachable: false},
		{name: "stable budget above hard cap", stable: 5, hardCap: 3, perRoot: 10, wantUnreachable: true, wantReasonContains: "hard cap 3"},
		{name: "stable budget one below hard cap, per-root generous", stable: 2, hardCap: 3, perRoot: 10, wantUnreachable: false},
		{name: "per-root equals stable+1 is still pre-empted", stable: 3, hardCap: 10, perRoot: 4, wantUnreachable: true, wantReasonContains: "per-root attempt cap 4"},
		{name: "per-root equals stable+2 is reachable", stable: 3, hardCap: 10, perRoot: 5, wantUnreachable: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			unreachable, reason := clusterClosureExitReachability(tc.stable, tc.hardCap, tc.perRoot)
			if unreachable != tc.wantUnreachable {
				t.Fatalf("stable=%d hardCap=%d perRoot=%d: unreachable=%t, want %t (%s)", tc.stable, tc.hardCap, tc.perRoot, unreachable, tc.wantUnreachable, reason)
			}
			if tc.wantUnreachable && !strings.Contains(reason, tc.wantReasonContains) {
				t.Fatalf("reason %q must name the pre-empting cap %q", reason, tc.wantReasonContains)
			}
			if !tc.wantUnreachable && reason != "" {
				t.Fatalf("reachable exit must carry no advisory, got %q", reason)
			}
		})
	}
}

// The advisory fires once per Orchestrator and reads the live caps.
func TestLogClusterClosureExitReachabilityOnce_EmitsOncePerOrchestrator(t *testing.T) {
	restoreStable := ClusterStableBudget()
	restorePerRoot := maxRepairAttemptsPerRootValue
	t.Cleanup(func() {
		SetClusterStableBudget(restoreStable)
		SetMaxRepairAttemptsPerRoot(restorePerRoot)
	})
	SetClusterStableBudget(2)
	SetMaxRepairAttemptsPerRoot(3)

	o := &Orchestrator{}
	o.SetFinalizeRepairHardCap(2)
	msg, emitted := o.logClusterClosureExitReachabilityOnce()
	if !emitted || !strings.Contains(msg, "unreachable under the current caps") {
		t.Fatalf("default caps must emit the advisory once, got emitted=%t msg=%q", emitted, msg)
	}
	if _, again := o.logClusterClosureExitReachabilityOnce(); again {
		t.Fatal("the advisory is emitted once per Orchestrator")
	}

	raised := &Orchestrator{}
	raised.SetFinalizeRepairHardCap(3)
	SetMaxRepairAttemptsPerRoot(4)
	if msg, emitted := raised.logClusterClosureExitReachabilityOnce(); emitted {
		t.Fatalf("with raised caps the exit is reachable; no advisory, got %q", msg)
	}
	if _, emitted := (*Orchestrator)(nil).logClusterClosureExitReachabilityOnce(); emitted {
		t.Fatal("nil orchestrator emits nothing")
	}
}

// The advisory is wired at scheduler entry: runReadSchedulerLoop calls
// logClusterClosureExitReachabilityOnce before its dispatch loop.
func TestRunReadSchedulerLoop_EmitsClusterClosureExitAdvisoryAtEntry(t *testing.T) {
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
		t.Fatal("runReadSchedulerLoop not found")
	}
	var advisoryPos, firstForPos token.Pos
	ast.Inspect(loop.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ForStmt:
			if firstForPos == 0 {
				firstForPos = x.Pos()
			}
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "logClusterClosureExitReachabilityOnce" && advisoryPos == 0 {
				advisoryPos = x.Pos()
			}
		}
		return true
	})
	if advisoryPos == 0 {
		t.Fatal("runReadSchedulerLoop must call logClusterClosureExitReachabilityOnce at scheduler entry")
	}
	if firstForPos != 0 && advisoryPos > firstForPos {
		t.Fatalf("the advisory must be emitted before the dispatch loop (advisory at %v, loop at %v)", fset.Position(advisoryPos), fset.Position(firstForPos))
	}
}
