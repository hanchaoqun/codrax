package orchestrator

import (
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
						if len(inner.Body.List) != 1 {
							t.Fatalf("the release branch must only `continue` (%v)", fset.Position(stmt.Pos()))
						}
						if _, ok := inner.Body.List[0].(*ast.BranchStmt); !ok {
							t.Fatalf("the release branch must `continue` the scheduler loop (%v)", fset.Position(stmt.Pos()))
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
