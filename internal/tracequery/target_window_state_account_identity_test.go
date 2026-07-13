package tracequery

// §29.53.1 SMR state-account identity/admission pins.  A user-facing thread
// name is only a selector: once ThreadTimeline resolves it to a positive TID,
// every refinement must consume that precise identity.  Window admission is
// likewise based on the caller's pre-normalization endpoint presence, so an
// explicit zero is valid while a defaulted zero is not silently promoted.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"testing"
)

func smrStateAccountIdentityTrace(t *testing.T) *Index {
	t.Helper()
	return buildTraceIndex(t, "smr_state_account_identity.systrace", `
        idle-0 (    0) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=oldname next_pid=61 next_prio=20
     oldname-61 (   61) [001] .... 1.010000: sched_switch: prev_comm=oldname prev_pid=61 prev_prio=20 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        waker-2 (    2) [000] .... 1.020000: sched_wakeup: comm=newname pid=61 prio=20 target_cpu=001
        waker-2 (    2) [000] .... 1.020001: sched_blocked_reason: pid=61 iowait=1 caller=f2fs_wait_on_block+0x10/0x20
        idle-0 (    0) [001] .... 1.021000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=newname next_pid=61 next_prio=20
     newname-61 (   61) [001] .... 1.022000: tracing_mark_write: B|61|VerifyClass RenamedClass
     newname-61 (   61) [001] .... 1.028000: tracing_mark_write: E|61
     newname-61 (   61) [001] .... 1.030000: sched_switch: prev_comm=newname prev_pid=61 prev_prio=20 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`)
}

// 修前红: the timeline resolved oldname to TID 61, but the generic Run arm
// passed the raw PID=0/comm=oldname selector into the refinements.  The
// blocked-reason lookup therefore missed WakeePID=61 and the renamed semantic
// span failed comm matching, even though both belonged to the selected task.
func TestTargetWindowStatesCarryResolvedIdentityAcrossRename(t *testing.T) {
	idx := smrStateAccountIdentityTrace(t)
	res := Run(idx, Query{
		View: "window_stats", Thread: "oldname",
		TimeStart: 1.0, TimeStartSet: true,
		TimeEnd: 1.04, TimeEndSet: true,
	})
	account := res.TargetWindowStates
	if account == nil {
		t.Fatal("a uniquely resolved name-only target must publish its state account")
	}
	if account.Thread.PID != 61 {
		t.Fatalf("the account must publish the resolved scheduler identity: %+v", account.Thread)
	}
	if math.Abs(account.SleepIOWaitMs-10) > 0.01 {
		t.Fatalf("S+iowait refinement must follow resolved TID 61 across the rename: got %.6fms want 10ms", account.SleepIOWaitMs)
	}
	if math.Abs(account.DeterministicRunningMs-6) > 0.01 {
		t.Fatalf("semantic-running refinement must follow resolved TID 61 across the rename: got %.6fms want 6ms", account.DeterministicRunningMs)
	}
	lanes := account.RunningMs + account.RunnableMs + account.SleepMs + account.DStateMs + account.IOWaitMs
	if math.Abs(account.TotalMs-lanes) > 1e-6 {
		t.Fatalf("identity refinement must not alter the additive state partition: %+v", account)
	}
}

func TestTargetWindowStatesAdmissionUsesCallerEndpoints(t *testing.T) {
	idx := smrStateAccountIdentityTrace(t)
	base := Query{View: "window_stats", PID: 61, TimeStart: 0, TimeStartSet: true, TimeEnd: 1.04, TimeEndSet: true}
	if got := Run(idx, base).TargetWindowStates; got == nil {
		t.Fatal("an explicitly supplied [0,x] window must publish the state account")
	}

	missingStart := base
	missingStart.TimeStartSet = false
	if got := Run(idx, missingStart).TargetWindowStates; got != nil {
		t.Fatalf("normalizeQuery defaults must not manufacture a caller-supplied start endpoint: %+v", got)
	}

	missingEnd := base
	missingEnd.TimeEnd = 0
	missingEnd.TimeEndSet = false
	if got := Run(idx, missingEnd).TargetWindowStates; got != nil {
		t.Fatalf("normalizeQuery defaults must not manufacture a caller-supplied end endpoint: %+v", got)
	}

	invalid := base
	invalid.TimeStart = 1.04
	if got := Run(idx, invalid).TargetWindowStates; got != nil {
		t.Fatalf("a non-positive window must never publish a state account: %+v", got)
	}
	reversed := base
	reversed.TimeStart = 1.05
	if got := Run(idx, reversed).TargetWindowStates; got != nil {
		t.Fatalf("a reversed window must never publish a state account: %+v", got)
	}

	legacy := base
	legacy.TimeStart = 1.0
	legacy.TimeStartSet = false
	legacy.TimeEndSet = false
	if got := Run(idx, legacy).TargetWindowStates; got == nil {
		t.Fatal("legacy nonzero endpoint values must retain bounded-window compatibility")
	}
}

// Structural pin: there is one generic state-account mint in Run; it consumes
// tl.Thread, and its admission condition consumes the two boundedness facts
// frozen before normalizeQuery.  Behavior tests above protect the semantics;
// this pin prevents a second raw-selector/raw-time implementation from
// drifting around them.
func TestTargetWindowStatesRunIdentityAndAdmissionPin(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "query.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var run *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "Run" {
			run = fn
			break
		}
	}
	if run == nil {
		t.Fatal("Run declaration not found")
	}

	normalizePos := token.NoPos
	startFreezePos := token.NoPos
	endFreezePos := token.NoPos
	startAssignments := 0
	endAssignments := 0
	var accountCalls []*ast.CallExpr
	var admission *ast.IfStmt
	ast.Inspect(run.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok {
			switch smrCalledName(call) {
			case "normalizeQuery":
				if normalizePos == token.NoPos {
					normalizePos = call.Pos()
				}
			case "buildTargetWindowStateAccount":
				accountCalls = append(accountCalls, call)
			}
		}
		assign, ok := node.(*ast.AssignStmt)
		if ok && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
			name, nameOK := assign.Lhs[0].(*ast.Ident)
			call, callOK := assign.Rhs[0].(*ast.CallExpr)
			if nameOK {
				switch name.Name {
				case "stateAccountTimeStartBounded":
					startAssignments++
				case "stateAccountTimeEndBounded":
					endAssignments++
				}
			}
			if nameOK && callOK {
				switch {
				case name.Name == "stateAccountTimeStartBounded" && smrCalledName(call) == "queryBoundedTimeStart":
					startFreezePos = assign.Pos()
				case name.Name == "stateAccountTimeEndBounded" && smrCalledName(call) == "queryBoundedTimeEnd":
					endFreezePos = assign.Pos()
				}
			}
		}
		if stmt, ok := node.(*ast.IfStmt); ok && smrContainsCall(stmt.Body, "buildTargetWindowStateAccount") &&
			smrContainsIdent(stmt.Cond, "stateAccountTimeStartBounded") && smrContainsIdent(stmt.Cond, "stateAccountTimeEndBounded") &&
			smrContainsEndAfterStart(stmt.Cond) {
			admission = stmt
		}
		return true
	})
	if normalizePos == token.NoPos || startFreezePos == token.NoPos || endFreezePos == token.NoPos ||
		startFreezePos >= normalizePos || endFreezePos >= normalizePos {
		t.Fatalf("state-account endpoint boundedness must be frozen before normalizeQuery: start=%v end=%v normalize=%v", startFreezePos, endFreezePos, normalizePos)
	}
	if startAssignments != 1 || endAssignments != 1 {
		t.Fatalf("frozen endpoint facts must be single-assignment: start=%d end=%d", startAssignments, endAssignments)
	}
	if len(accountCalls) != 1 {
		t.Fatalf("Run must have exactly one generic state-account mint, got %d", len(accountCalls))
	}
	call := accountCalls[0]
	if len(call.Args) != 6 {
		t.Fatalf("state-account mint argument shape drifted: %#v", call.Args)
	}
	for arg, want := range map[int]string{1: "tl", 2: "ok", 4: "window"} {
		ident, ok := call.Args[arg].(*ast.Ident)
		if !ok || ident.Name != want {
			t.Fatalf("state-account mint arg %d must be %s, got %#v", arg, want, call.Args[arg])
		}
	}
	selector, ok := call.Args[3].(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Thread" {
		t.Fatalf("state-account refinements must consume tl.Thread, got %#v", call.Args[3])
	}
	base, ok := selector.X.(*ast.Ident)
	if !ok || base.Name != "tl" {
		t.Fatalf("state-account refinements must consume tl.Thread, got %#v", call.Args[3])
	}
	stats, ok := call.Args[5].(*ast.SelectorExpr)
	if !ok || stats.Sel.Name != "WindowStats" {
		t.Fatalf("state-account semantic refinement must consume res.WindowStats, got %#v", call.Args[5])
	}
	statsBase, ok := stats.X.(*ast.Ident)
	if !ok || statsBase.Name != "res" {
		t.Fatalf("state-account semantic refinement must consume res.WindowStats, got %#v", call.Args[5])
	}
	if admission == nil {
		t.Fatal("generic state-account mint must share the two frozen endpoint admission facts")
	}
}

func smrCalledName(call *ast.CallExpr) string {
	if call == nil {
		return ""
	}
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	default:
		return ""
	}
}

func smrContainsCall(node ast.Node, name string) bool {
	found := false
	ast.Inspect(node, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok && smrCalledName(call) == name {
			found = true
			return false
		}
		return !found
	})
	return found
}

func smrContainsIdent(node ast.Node, name string) bool {
	found := false
	ast.Inspect(node, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok && ident.Name == name {
			found = true
			return false
		}
		return !found
	})
	return found
}

func smrContainsEndAfterStart(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(node ast.Node) bool {
		expr, ok := node.(*ast.BinaryExpr)
		if !ok || expr.Op != token.GTR {
			return !found
		}
		left, leftOK := expr.X.(*ast.SelectorExpr)
		right, rightOK := expr.Y.(*ast.SelectorExpr)
		if !leftOK || !rightOK || left.Sel.Name != "TimeEnd" || right.Sel.Name != "TimeStart" {
			return !found
		}
		leftBase, leftBaseOK := left.X.(*ast.Ident)
		rightBase, rightBaseOK := right.X.(*ast.Ident)
		if leftBaseOK && rightBaseOK && leftBase.Name == "q" && rightBase.Name == "q" {
			found = true
			return false
		}
		return !found
	})
	return found
}
