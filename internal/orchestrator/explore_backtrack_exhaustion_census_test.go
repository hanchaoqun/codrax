package orchestrator

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// explore_backtrack_exhaustion_census_test.go — §40.43 F-orch 三轮复核
// finding Q census pins: the typed ExploreBacktrackExhausted decision is
// registered in the hard-arm carrier census (kind "decision"), every writer
// of the completion generation is a registered decision, and the release
// has exactly one scheduler call site (the blocked-window branch of
// runReadSchedulerLoop). Also the self-red cases of the hardened reset-site
// checker (T-i).

// assertNoExploreBacktrackExhaustionDecision pins that a run whose explorer
// re-earned the closure with a FRESH completion released the veto through
// the fresh generation, not through the exhaustion decision.
func assertNoExploreBacktrackExhaustionDecision(t *testing.T, bus *types.BusContext) {
	t.Helper()
	if n := bus.Mutable.ExploreBacktrackExhaustedDecisions(); n != 0 {
		t.Fatalf("the fresh completion must release the veto by generation; got %d exhaustion decision(s): %+v", n, bus.Mutable.LastExploreBacktrackExhausted())
	}
}

// TestHardArmMutableCarrierCensus_CompletionGenerationWritersAreRegisteredDecisions:
// the set of MutableState methods in ../types whose bodies advance
// `m.investigationCompleteGeneration` (IncDec or compound assignment) is
// EXACTLY hardArmCompletionGenerationDecisions. A new writer (or a reset
// that starts bumping the generation) is red; a registered decision whose
// method no longer advances the counter is red too.
func TestHardArmMutableCarrierCensus_CompletionGenerationWritersAreRegisteredDecisions(t *testing.T) {
	writers := map[string]bool{}
	for _, decl := range typesPackageDecls(t) {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 || fn.Body == nil {
			continue
		}
		star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if id, ok := star.X.(*ast.Ident); !ok || id.Name != "MutableState" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.IncDecStmt:
				if selectorIsField(x.X, "investigationCompleteGeneration") {
					writers[fn.Name.Name] = true
				}
			case *ast.AssignStmt:
				for _, lhs := range x.Lhs {
					if selectorIsField(lhs, "investigationCompleteGeneration") {
						writers[fn.Name.Name] = true
					}
				}
			}
			return true
		})
	}
	var got, want []string
	for name := range writers {
		got = append(got, name)
	}
	for name := range hardArmCompletionGenerationDecisions {
		want = append(want, name)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("completion-generation writers in ../types = %v, registered decisions = %v — every writer of the veto's generation witness must be a registered typed decision (and every registered decision must still write it)", got, want)
	}
	if writers["ResetInvestigationComplete"] {
		t.Fatal("ResetInvestigationComplete must never advance the generation — a reset is not a decision")
	}
	for name := range hardArmCompletionGenerationDecisions {
		if !mutableStateMethodNames(t)[name] {
			t.Fatalf("registered decision %q is not a MutableState method", name)
		}
	}
}

// hardArmRetainedClosureWriters is the closed set of MutableState methods
// allowed to write the retained closure fields
// (retainedInvestigationCompleteReason / retainedInvestigationResultKind /
// retainedAbsenceJustification) — the lane every Stable* consumer and the
// exhaustion release read after a backtrack's ResetInvestigationComplete.
// The accepted-completion setters write it directly; MergeExploreFork may
// write it ONLY from a fork that recorded its own accepted completion
// (§40.43 F-orch 四轮复核 finding W) — every write inside it must sit under
// an `if` whose condition reads the fork-decided identifier, itself defined
// as the completion-generation comparison.
var hardArmRetainedClosureWriters = map[string]bool{
	"SetInvestigationComplete":   true,
	"SetInvestigationResultKind": true,
	"SetAbsenceJustification":    true,
	"MergeExploreFork":           true,
}

var hardArmRetainedClosureFields = []string{
	"retainedInvestigationCompleteReason",
	"retainedInvestigationResultKind",
	"retainedAbsenceJustification",
}

const hardArmForkDecidedIdent = "forkDecidedCompletion"

// TestHardArmMutableCarrierCensus_RetainedClosureWritersAreAcceptedCompletions
// (finding W): (1) the set of MutableState methods in ../types that assign
// any retained closure field is EXACTLY hardArmRetainedClosureWriters; (2)
// inside MergeExploreFork every such assignment is guarded by the
// fork-decided identifier; (3) that identifier is defined as
// `fork.investigationCompleteGeneration > fork.exploreForkCompletionGenerationBase`.
func TestHardArmMutableCarrierCensus_RetainedClosureWritersAreAcceptedCompletions(t *testing.T) {
	writers := map[string]bool{}
	var merge *ast.FuncDecl
	for _, decl := range typesPackageDecls(t) {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 || fn.Body == nil {
			continue
		}
		star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if id, ok := star.X.(*ast.Ident); !ok || id.Name != "MutableState" {
			continue
		}
		if fn.Name.Name == "MergeExploreFork" {
			merge = fn
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range as.Lhs {
				for _, field := range hardArmRetainedClosureFields {
					if selectorIsField(lhs, field) {
						writers[fn.Name.Name] = true
					}
				}
			}
			return true
		})
	}
	var got, want []string
	for name := range writers {
		got = append(got, name)
	}
	for name := range hardArmRetainedClosureWriters {
		want = append(want, name)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("retained-closure writers in ../types = %v, registered = %v — the retained lane is the most recently accepted terminal state; register (and justify) every writer", got, want)
	}
	if merge == nil {
		t.Fatal("MergeExploreFork not found")
	}
	if problems := retainedWriteGuardVerdict(merge, hardArmRetainedClosureFields, hardArmForkDecidedIdent); len(problems) != 0 {
		t.Fatalf("MergeExploreFork writes the retained lane outside the fork-decided guard:\n  %s", strings.Join(problems, "\n  "))
	}
	if !forkDecidedIdentIsGenerationComparison(merge, hardArmForkDecidedIdent) {
		t.Fatalf("%s must be defined in MergeExploreFork as `fork.investigationCompleteGeneration > fork.exploreForkCompletionGenerationBase` (the fork's own accepted completion)", hardArmForkDecidedIdent)
	}
}

// retainedWriteGuardVerdict returns one line per assignment to a retained
// closure field inside fn that is NOT nested under an IfStmt whose condition
// mentions guardIdent (an enclosing `if` anywhere up the statement path
// counts; a func literal breaks the path).
func retainedWriteGuardVerdict(fn *ast.FuncDecl, fields []string, guardIdent string) []string {
	var problems []string
	var walk func(n ast.Node, guarded bool)
	// leaf inspects one statement that carries no further branch structure:
	// a direct assignment is judged by the guard status; a write tucked
	// inside a func literal ANYWHERE in the statement (a closure argument,
	// an assignment's RHS, a deferred call) is always red — the guard's
	// scope does not survive into a function value.
	leaf := func(n ast.Node, guarded bool) {
		ast.Inspect(n, func(m ast.Node) bool {
			switch y := m.(type) {
			case *ast.FuncLit:
				ast.Inspect(y.Body, func(k ast.Node) bool {
					if as, ok := k.(*ast.AssignStmt); ok {
						for _, lhs := range as.Lhs {
							for _, field := range fields {
								if selectorIsField(lhs, field) {
									problems = append(problems, field+" is written inside a func literal")
								}
							}
						}
					}
					return true
				})
				return false
			case *ast.AssignStmt:
				for _, lhs := range y.Lhs {
					for _, field := range fields {
						if selectorIsField(lhs, field) && !guarded {
							problems = append(problems, field+" is assigned outside an `if "+guardIdent+"` guard")
						}
					}
				}
			}
			return true
		})
	}
	walk = func(n ast.Node, guarded bool) {
		switch x := n.(type) {
		case nil:
			return
		case *ast.IfStmt:
			g := guarded || condMentionsIdent(x.Cond, guardIdent)
			walk(x.Body, g)
			if x.Else != nil {
				// The else branch of the guard is exactly the unguarded case.
				walk(x.Else, guarded)
			}
			return
		case *ast.BlockStmt:
			for _, s := range x.List {
				walk(s, guarded)
			}
			return
		case *ast.ForStmt:
			walk(x.Body, guarded)
			return
		case *ast.RangeStmt:
			walk(x.Body, guarded)
			return
		case *ast.SwitchStmt:
			walk(x.Body, guarded)
			return
		case *ast.TypeSwitchStmt:
			walk(x.Body, guarded)
			return
		case *ast.CaseClause:
			for _, s := range x.Body {
				walk(s, guarded)
			}
			return
		default:
			leaf(n, guarded)
			return
		}
	}
	walk(fn.Body, false)
	return problems
}

func condMentionsIdent(cond ast.Expr, ident string) bool {
	found := false
	ast.Inspect(cond, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == ident {
			found = true
		}
		return !found
	})
	return found
}

// forkDecidedIdentIsGenerationComparison pins the guard's definition: a
// top-level `ident := fork.investigationCompleteGeneration >
// fork.exploreForkCompletionGenerationBase` in fn.
func forkDecidedIdentIsGenerationComparison(fn *ast.FuncDecl, ident string) bool {
	for _, stmt := range fn.Body.List {
		as, ok := stmt.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			continue
		}
		if id, ok := as.Lhs[0].(*ast.Ident); !ok || id.Name != ident {
			continue
		}
		bin, ok := as.Rhs[0].(*ast.BinaryExpr)
		if !ok || bin.Op != token.GTR {
			return false
		}
		return selectorIsField(bin.X, "investigationCompleteGeneration") && selectorIsField(bin.Y, "exploreForkCompletionGenerationBase")
	}
	return false
}

// TestRetainedWriteGuardVerdict_SelfRed: the guard checker flags an
// unguarded write, a write guarded by a different condition, a write in the
// else branch, and a write inside a func literal; it accepts writes nested
// under the guard (directly or through an inner `if`).
func TestRetainedWriteGuardVerdict_SelfRed(t *testing.T) {
	cases := []struct {
		name string
		body string
		red  bool
	}{
		{name: "guarded write", body: "if forkDecidedCompletion {\n\t\tm.retainedInvestigationCompleteReason = r\n\t}", red: false},
		{name: "guarded through an inner if", body: "if forkDecidedCompletion {\n\t\tif r != \"\" {\n\t\t\tm.retainedInvestigationCompleteReason = r\n\t\t}\n\t}", red: false},
		{name: "guard combined with another condition", body: "if complete && forkDecidedCompletion {\n\t\tm.retainedAbsenceJustification = r\n\t}", red: false},
		{name: "unguarded write", body: "m.retainedInvestigationCompleteReason = r", red: true},
		{name: "guarded by a different condition", body: "if r != \"\" {\n\t\tm.retainedInvestigationResultKind = r\n\t}", red: true},
		{name: "write in the else branch of the guard", body: "if forkDecidedCompletion {\n\t\tm.x = r\n\t} else {\n\t\tm.retainedInvestigationCompleteReason = r\n\t}", red: true},
		{name: "write inside a func literal", body: "if forkDecidedCompletion {\n\t\tfunc() { m.retainedInvestigationCompleteReason = r }()\n\t}", red: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "package p\n\nfunc (m *M) MergeExploreFork(fork *M) {\n\tforkDecidedCompletion := fork.investigationCompleteGeneration > fork.exploreForkCompletionGenerationBase\n\t" + tc.body + "\n}\n"
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "snippet.go", src, 0)
			if err != nil {
				t.Fatalf("parse snippet: %v", err)
			}
			fn := f.Decls[0].(*ast.FuncDecl)
			problems := retainedWriteGuardVerdict(fn, hardArmRetainedClosureFields, hardArmForkDecidedIdent)
			if tc.red && len(problems) == 0 {
				t.Fatalf("shape must be red:\n%s", tc.body)
			}
			if !tc.red && len(problems) != 0 {
				t.Fatalf("shape must be accepted, got %v", problems)
			}
			if !forkDecidedIdentIsGenerationComparison(fn, hardArmForkDecidedIdent) {
				t.Fatal("snippet defines the guard as the generation comparison")
			}
		})
	}
	// The definition pin itself is red for a guard bound to anything else.
	src := "package p\n\nfunc (m *M) MergeExploreFork(fork *M) {\n\tforkDecidedCompletion := fork.investigationComplete\n}\n"
	f, err := parser.ParseFile(token.NewFileSet(), "snippet.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	if forkDecidedIdentIsGenerationComparison(f.Decls[0].(*ast.FuncDecl), hardArmForkDecidedIdent) {
		t.Fatal("a guard bound to the live flag instead of the generation comparison must be red")
	}
}

// selectorIsField matches `<recv>.<field>` for any receiver identifier.
func selectorIsField(expr ast.Expr, field string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != field {
		return false
	}
	_, ok = sel.X.(*ast.Ident)
	return ok
}

// TestExploreBacktrackExhaustion_ReleaseHasOneSchedulerSite: the release is
// a top-level statement of the blocked-nodes branch of runReadSchedulerLoop
// (the `if len(blocked) > 0` block that precedes the blocked-DAG forced
// finalize), called exactly once in the package's production code, and the
// typed decision is recorded only inside the release.
func TestExploreBacktrackExhaustion_ReleaseHasOneSchedulerSite(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob: %v", err)
	}
	fset := token.NewFileSet()
	releaseCallers := map[string]int{}
	recordCallers := map[string]int{}
	var loop *ast.FuncDecl
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
			if file == "orchestrator.go" && fd.Name.Name == "runReadSchedulerLoop" {
				loop = fd
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					switch sel.Sel.Name {
					case "releaseExhaustedExploreBacktrack":
						releaseCallers[fd.Name.Name]++
					case "RecordExploreBacktrackExhausted":
						recordCallers[fd.Name.Name]++
					}
				}
				return true
			})
		}
	}
	if len(releaseCallers) != 1 || releaseCallers["runReadSchedulerLoop"] != 1 {
		t.Fatalf("releaseExhaustedExploreBacktrack must be called exactly once, in runReadSchedulerLoop; got %v", releaseCallers)
	}
	if len(recordCallers) != 1 || recordCallers["releaseExhaustedExploreBacktrack"] != 1 {
		t.Fatalf("RecordExploreBacktrackExhausted must be recorded only by releaseExhaustedExploreBacktrack; got %v", recordCallers)
	}
	if loop == nil {
		t.Fatal("runReadSchedulerLoop not found")
	}
	// The call sits as the condition of a top-level `if` statement of the
	// `if len(blocked) > 0` block, before the blocked-DAG profile write.
	found := false
	ast.Inspect(loop.Body, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok || !isLenBlockedGuard(ifStmt.Cond) {
			return true
		}
		var releasePos, profilePos token.Pos
		for _, stmt := range ifStmt.Body.List {
			inner, ok := stmt.(*ast.IfStmt)
			if ok {
				if call, ok := inner.Cond.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "releaseExhaustedExploreBacktrack" {
						releasePos = stmt.Pos()
						if reason := exhaustionReleaseBranchVerdict(inner); reason != "" {
							t.Fatalf("the release branch must be exactly a bare `continue` of the scheduler loop: %s (%v)", reason, fset.Position(stmt.Pos()))
						}
					}
				}
			}
			ast.Inspect(stmt, func(m ast.Node) bool {
				if call, ok := m.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "SetTerminationProfile" && profilePos == 0 {
						profilePos = stmt.Pos()
					}
				}
				return true
			})
		}
		if releasePos != 0 && profilePos != 0 && releasePos < profilePos {
			found = true
		}
		return true
	})
	if !found {
		t.Fatal("the exhaustion release must be a top-level `if o.releaseExhaustedExploreBacktrack(...) { continue }` statement of the `if len(blocked) > 0` branch, before the blocked-DAG termination profile is written")
	}
}

// exhaustionReleaseBranchVerdict (§40.43 F-orch 四轮复核 finding Z) accepts
// only the production shape of the release branch: a body of exactly one
// statement that is an unlabeled `continue`. Any other BranchStmt (`break`
// leaves the scheduler loop with the veto released but nothing re-evaluated;
// `goto` / `fallthrough` jump elsewhere; a labeled continue may target an
// outer loop) or any other statement is red. Returns "" when accepted.
func exhaustionReleaseBranchVerdict(inner *ast.IfStmt) string {
	if inner == nil || inner.Body == nil {
		return "no branch body"
	}
	if len(inner.Body.List) != 1 {
		return fmt.Sprintf("body has %d statements, want exactly one", len(inner.Body.List))
	}
	br, ok := inner.Body.List[0].(*ast.BranchStmt)
	if !ok {
		return fmt.Sprintf("body statement is %T, want a continue statement", inner.Body.List[0])
	}
	if br.Tok != token.CONTINUE {
		return fmt.Sprintf("branch statement is `%s`, want `continue`", br.Tok)
	}
	if br.Label != nil {
		return fmt.Sprintf("continue carries the label %q, want an unlabeled continue of the scheduler loop", br.Label.Name)
	}
	return ""
}

// TestExhaustionReleaseBranchVerdict_SelfRed: the verdict rejects every
// non-`continue` branch shape and accepts only the production one. Before
// this checker any *ast.BranchStmt (break / goto / fallthrough / labeled
// continue) satisfied the census.
func TestExhaustionReleaseBranchVerdict_SelfRed(t *testing.T) {
	cases := []struct {
		name string
		body string
		red  bool
	}{
		{name: "production: bare continue", body: "continue", red: false},
		{name: "break", body: "break", red: true},
		{name: "goto", body: "goto done", red: true},
		{name: "labeled continue", body: "continue outer", red: true},
		{name: "fallthrough-shaped branch", body: "break outer", red: true},
		{name: "return", body: "return stepsUsed", red: true},
		{name: "continue plus another statement", body: "stepsUsed++\n\t\t\tcontinue", red: true},
		{name: "empty body", body: "", red: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "package p\n\nfunc f() int {\n\tstepsUsed := 0\nouter:\n\tfor {\n\t\tif o.releaseExhaustedExploreBacktrack(1) {\n\t\t\t" + tc.body + "\n\t\t}\n\t\tbreak\n\t}\ndone:\n\treturn stepsUsed\n}\n"
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "snippet.go", src, 0)
			if err != nil {
				t.Fatalf("parse snippet: %v", err)
			}
			var inner *ast.IfStmt
			ast.Inspect(f, func(n ast.Node) bool {
				if s, ok := n.(*ast.IfStmt); ok && inner == nil {
					inner = s
				}
				return true
			})
			if inner == nil {
				t.Fatal("snippet has no if statement")
			}
			reason := exhaustionReleaseBranchVerdict(inner)
			if tc.red && reason == "" {
				t.Fatalf("shape %q must be red", tc.body)
			}
			if !tc.red && reason != "" {
				t.Fatalf("production shape must be accepted, got %q", reason)
			}
		})
	}
}

func isLenBlockedGuard(cond ast.Expr) bool {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.GTR {
		return false
	}
	call, ok := bin.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	if fn, ok := call.Fun.(*ast.Ident); !ok || fn.Name != "len" {
		return false
	}
	arg, ok := call.Args[0].(*ast.Ident)
	lit, okLit := bin.Y.(*ast.BasicLit)
	return ok && arg.Name == "blocked" && okLit && lit.Value == "0"
}

// TestSignalResetSiteVerdict_SelfRed (T-i): the hardened reset-site checker
// rejects the guarded, func-literal and clear-then-re-raise shapes and
// accepts only the production shape (one top-level unconditional clear).
func TestSignalResetSiteVerdict_SelfRed(t *testing.T) {
	cases := []struct {
		name string
		body string
		want signalArmVerdict
	}{
		{name: "production: top-level clear", body: `o.busCtx.Signals.HasEnoughFacts = false`, want: signalArmVerdict{clearedTopLevel: true}},
		{name: "guarded clear does not count", body: `if cond { o.busCtx.Signals.HasEnoughFacts = false }`, want: signalArmVerdict{clearedNested: true}},
		{name: "func-literal clear does not count", body: `func() { o.busCtx.Signals.HasEnoughFacts = false }()`, want: signalArmVerdict{clearedNested: true}},
		{name: "loop-body clear does not count", body: `for i := 0; i < 1; i++ { o.busCtx.Signals.HasEnoughFacts = false }`, want: signalArmVerdict{clearedNested: true}},
		{name: "clear then re-raise", body: "o.busCtx.Signals.HasEnoughFacts = false\n\t\to.busCtx.Signals.HasEnoughFacts = true", want: signalArmVerdict{clearedTopLevel: true, reraised: true}},
		{name: "clear then guarded re-raise", body: "o.busCtx.Signals.HasEnoughFacts = false\n\t\tif cond { o.busCtx.Signals.HasEnoughFacts = true }", want: signalArmVerdict{clearedTopLevel: true, reraised: true}},
		{name: "top-level clear plus a guarded duplicate", body: "o.busCtx.Signals.HasEnoughFacts = false\n\t\tif cond { o.busCtx.Signals.HasEnoughFacts = false }", want: signalArmVerdict{clearedTopLevel: true, clearedNested: true}},
		{name: "no clear", body: `state.requeue(id)`, want: signalArmVerdict{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "package p\n\nfunc f() {\n\tswitch fallback {\n\tcase FallbackBackToExplore:\n\t\t" + tc.body + "\n\t}\n}\n"
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
			if cc == nil {
				t.Fatal("snippet has no case clause")
			}
			if got := signalResetSiteVerdict(cc, "HasEnoughFacts"); got != tc.want {
				t.Fatalf("verdict %+v, want %+v", got, tc.want)
			}
		})
	}
}
