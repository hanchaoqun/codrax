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

// hardArmRetainedClosureWriters is the closed set of package-types
// functions allowed to write the retained closure fields — the lane every
// Stable* consumer and the exhaustion release read after a backtrack's
// ResetInvestigationComplete. Keys are "Receiver.Method" for methods and
// the bare name for free functions (§40.43 round-six #11: the census scans
// EVERY function in the package, so a free function or a foreign-receiver
// method writing a retained field is red, exactly like the sibling
// G-patch-txn field census). The accepted-completion setters write it
// directly; MergeExploreFork may write it ONLY from a fork that recorded
// its own accepted completion (§40.43 F-orch 四轮复核 finding W) — every
// write inside it must sit under an `if` that REQUIRES the fork-decided
// identifier to be true (§40.43 round-six #10: positive polarity, over &&
// chains only), itself defined exactly once as the completion-generation
// comparison (no shadowing, no reassignment).
var hardArmRetainedClosureWriters = map[string]bool{
	"MutableState.SetInvestigationComplete":   true,
	"MutableState.SetInvestigationResultKind": true,
	"MutableState.SetAbsenceJustification":    true,
	"MutableState.MergeExploreFork":           true,
	// Accepted-completion promotion of the two collection lanes (§40.43
	// round-six #3; wording corrected round-seven #7): called from
	// emit_investigation_complete's accepted paths AND — via the exported
	// tool.RefreshExactTypedRelationPrincipalMemberSets — from
	// explorerEvaluator.ParseOutput's post-explore refresh, gated on
	// IsInvestigationComplete() (an already accepted completion), not on the
	// accepting attempt itself.
	"MutableState.RetainInvestigationAggregateFacts": true,
	"MutableState.RetainInvestigationRelationClaims": true,
	// Fork creation copies the parent's retained lane verbatim onto the new
	// fork carrier (out.retained* = clone(m.retained*)) — a copy, not a
	// mutation of the accepted state.
	"MutableState.ForkForExploreDispatch": true,
}

// hardArmRetainedNonClosureLanes registers the MutableState "retained*"
// fields that are NOT part of the retained-closure lane, each with its
// justification. §40.43 round-six #3: the closure field set is DERIVED from
// the struct (hardArmRetainedClosureFieldsFromStruct), and a retained field
// the walker cannot classify fails loud — never silently uncensused.
var hardArmRetainedNonClosureLanes = map[string]string{
	// §40.43 round-seven #7: the field has FOUR writers in context.go — the
	// non-nil promotion (RetainEvidenceFloorWaiver(true), after an accepted
	// completion) and three CLEARS: SetEvidenceFloorWaiver(nil) (called from
	// the tool's decode-time ignored-waiver branches, BEFORE any completion
	// gate), ClearEvidenceFloorWaiver (explicit retraction) and
	// RetainEvidenceFloorWaiver(false) (a waiver-less accepted completion).
	// The field doc says "promoted only after", never "written only after".
	"retainedEvidenceFloorWaiver": "promoted-waiver lane: PROMOTED (non-nil) only after a successful emit_investigation_complete; its other writers are clears — SetEvidenceFloorWaiver(nil) on decode-time ignored waivers, ClearEvidenceFloorWaiver, RetainEvidenceFloorWaiver(false) (own invariant doc at the field)",
}

// hardArmRetainedClosureFields is the static floor used by the self-red
// snippets; the census itself judges the DERIVED set so a newly added
// retained field is auto-covered.
var hardArmRetainedClosureFields = []string{
	"retainedInvestigationCompleteReason",
	"retainedInvestigationResultKind",
	"retainedAbsenceJustification",
	"retainedInvestigationAggregateFacts",
	"retainedInvestigationRelationClaims",
}

// hardArmRetainedClosureFieldsFromStruct derives the retained-closure field
// set from the MutableState struct declaration: every field named
// "retained*" is either a closure-lane field (retainedInvestigation* or
// retainedAbsenceJustification), a registered non-closure lane, or a
// FAIL-LOUD violation (a shape the census cannot classify ends the
// enumeration).
func hardArmRetainedClosureFieldsFromStruct(t *testing.T) []string {
	t.Helper()
	var fields []string
	for _, decl := range typesPackageDecls(t) {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != "MutableState" {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				t.Fatal("MutableState is not a struct")
			}
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					if !strings.HasPrefix(name.Name, "retained") {
						continue
					}
					if _, registered := hardArmRetainedNonClosureLanes[name.Name]; registered {
						continue
					}
					if strings.HasPrefix(name.Name, "retainedInvestigation") || name.Name == "retainedAbsenceJustification" {
						fields = append(fields, name.Name)
						continue
					}
					t.Fatalf("unrecognized retained field MutableState.%s — classify it: a retained-closure field (retainedInvestigation*) or a registered lane in hardArmRetainedNonClosureLanes with a justification", name.Name)
				}
			}
		}
	}
	sort.Strings(fields)
	for _, want := range hardArmRetainedClosureFields {
		found := false
		for _, got := range fields {
			found = found || got == want
		}
		if !found {
			t.Fatalf("derived retained-closure fields %v lost the known field %s — the census lost its subject", fields, want)
		}
	}
	return fields
}

const hardArmForkDecidedIdent = "forkDecidedCompletion"

// hardArmRetainedWriterKey names one function for the writers census:
// "Receiver.Method" for methods (any receiver type, star or value),
// the bare name for free functions.
func hardArmRetainedWriterKey(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	if id, ok := recv.(*ast.Ident); ok {
		return id.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// TestHardArmMutableCarrierCensus_RetainedClosureWritersAreAcceptedCompletions
// (finding W; §40.43 round-six #3/#10/#11): (1) the retained-closure field
// set is DERIVED from the MutableState struct (a new retained field is
// auto-covered, an unclassifiable one fails loud); (2) the set of functions
// in ../types — free functions and methods on ANY type included — that
// assign a retained closure field (composite-literal keys included) is
// EXACTLY hardArmRetainedClosureWriters; (3) inside MergeExploreFork every
// such assignment sits under an `if` that REQUIRES the fork-decided
// identifier true (positive polarity over && chains; a negated or or-ed
// guard does not count); (4) that identifier is defined exactly once, at
// the top level, as `fork.investigationCompleteGeneration >
// fork.exploreForkCompletionGenerationBase` — a nested shadow or a
// reassignment is red.
func TestHardArmMutableCarrierCensus_RetainedClosureWritersAreAcceptedCompletions(t *testing.T) {
	fields := hardArmRetainedClosureFieldsFromStruct(t)
	writers := map[string]bool{}
	var merge *ast.FuncDecl
	for _, decl := range typesPackageDecls(t) {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		key := hardArmRetainedWriterKey(fn)
		if key == "MutableState.MergeExploreFork" {
			merge = fn
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.AssignStmt:
				for _, lhs := range x.Lhs {
					for _, field := range fields {
						if selectorIsField(lhs, field) {
							writers[key] = true
						}
					}
				}
			case *ast.KeyValueExpr:
				if id, ok := x.Key.(*ast.Ident); ok {
					for _, field := range fields {
						if id.Name == field {
							writers[key] = true
						}
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
	if problems := retainedWriteGuardVerdict(merge, fields, hardArmForkDecidedIdent); len(problems) != 0 {
		t.Fatalf("MergeExploreFork writes the retained lane outside the fork-decided guard:\n  %s", strings.Join(problems, "\n  "))
	}
	if !forkDecidedIdentIsGenerationComparison(merge, hardArmForkDecidedIdent) {
		t.Fatalf("%s must be defined in MergeExploreFork exactly once, at the top level, as `fork.investigationCompleteGeneration > fork.exploreForkCompletionGenerationBase` (the fork's own accepted completion; shadowing or reassignment is red)", hardArmForkDecidedIdent)
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
			g := guarded || condRequiresIdentTrue(x.Cond, guardIdent)
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

// condRequiresIdentTrue (§40.43 round-six #10) reports whether the branch
// taken when cond is true REQUIRES the guard identifier to be true: the
// identifier itself, possibly parenthesised, or an operand of an && chain.
// A negated guard (`!ident` — the exact inverse of the ruling), an || arm
// (the branch can run with the guard false), a comparison, or any other
// shape does NOT count. Polarity-precise by construction.
func condRequiresIdentTrue(cond ast.Expr, ident string) bool {
	switch x := cond.(type) {
	case *ast.Ident:
		return x.Name == ident
	case *ast.ParenExpr:
		return condRequiresIdentTrue(x.X, ident)
	case *ast.BinaryExpr:
		if x.Op == token.LAND {
			return condRequiresIdentTrue(x.X, ident) || condRequiresIdentTrue(x.Y, ident)
		}
	}
	return false
}

// forkDecidedIdentIsGenerationComparison pins the guard's definition
// (§40.43 round-six #10 hardening): the identifier is written EXACTLY once
// anywhere in fn — nested shadows (`ident := true` inside a block or func
// literal) and top-level reassignments are red — and that single write is a
// top-level `ident := fork.investigationCompleteGeneration >
// fork.exploreForkCompletionGenerationBase`.
func forkDecidedIdentIsGenerationComparison(fn *ast.FuncDecl, ident string) bool {
	writes := 0
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range x.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name == ident {
					writes++
				}
			}
		case *ast.ValueSpec:
			for _, name := range x.Names {
				if name.Name == ident {
					writes++
				}
			}
		}
		return true
	})
	if writes != 1 {
		return false
	}
	for _, stmt := range fn.Body.List {
		as, ok := stmt.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			continue
		}
		if id, ok := as.Lhs[0].(*ast.Ident); !ok || id.Name != ident {
			continue
		}
		if as.Tok != token.DEFINE {
			return false
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
		// §40.43 round-six #10: polarity — the exact inverse of the ruling
		// (writing the retained lane from a fork that did NOT record an
		// accepted completion) used to count as "guarded".
		{name: "negated guard is not a guard", body: "if !forkDecidedCompletion {\n\t\tm.retainedInvestigationCompleteReason = r\n\t}", red: true},
		{name: "or-guard is not a guard", body: "if complete || forkDecidedCompletion {\n\t\tm.retainedInvestigationCompleteReason = r\n\t}", red: true},
		{name: "guard on the new collection lanes", body: "if forkDecidedCompletion {\n\t\tm.retainedInvestigationAggregateFacts = f\n\t\tm.retainedInvestigationRelationClaims = c\n\t}", red: false},
		{name: "unguarded write to a collection lane", body: "m.retainedInvestigationRelationClaims = c", red: true},
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
	// The definition pin itself is red for a guard bound to anything else,
	// and (§40.43 round-six #10) for a shadowed or reassigned guard — the
	// original pin checked only the first TOP-LEVEL assignment, so a nested
	// `forkDecidedCompletion := true` (or a later `forkDecidedCompletion =
	// true`) laundered every write through a true guard.
	defRed := []struct{ name, body string }{
		{name: "bound to the live flag", body: "forkDecidedCompletion := fork.investigationComplete"},
		{name: "nested shadow", body: "forkDecidedCompletion := fork.investigationCompleteGeneration > fork.exploreForkCompletionGenerationBase\n\tif fork != nil {\n\t\tforkDecidedCompletion := true\n\t\t_ = forkDecidedCompletion\n\t}"},
		{name: "top-level reassignment", body: "forkDecidedCompletion := fork.investigationCompleteGeneration > fork.exploreForkCompletionGenerationBase\n\tforkDecidedCompletion = true"},
		{name: "func-literal shadow", body: "forkDecidedCompletion := fork.investigationCompleteGeneration > fork.exploreForkCompletionGenerationBase\n\tfunc() { forkDecidedCompletion := true; _ = forkDecidedCompletion }()"},
	}
	for _, tc := range defRed {
		t.Run("definition pin: "+tc.name, func(t *testing.T) {
			src := "package p\n\nfunc (m *M) MergeExploreFork(fork *M) {\n\t" + tc.body + "\n}\n"
			f, err := parser.ParseFile(token.NewFileSet(), "snippet.go", src, 0)
			if err != nil {
				t.Fatal(err)
			}
			if forkDecidedIdentIsGenerationComparison(f.Decls[0].(*ast.FuncDecl), hardArmForkDecidedIdent) {
				t.Fatalf("definition-pin shape must be red:\n%s", tc.body)
			}
		})
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
	var aliasRefs []string
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
			calls, aliases := exhaustionEntryPointReferences(fd)
			for name, n := range calls {
				switch name {
				case "releaseExhaustedExploreBacktrack":
					releaseCallers[fd.Name.Name] += n
				case "RecordExploreBacktrackExhausted":
					recordCallers[fd.Name.Name] += n
				}
			}
			for _, alias := range aliases {
				aliasRefs = append(aliasRefs, file+":"+fd.Name.Name+" "+alias)
			}
		}
	}
	// §40.43 round-six #17: a method-value alias (`release :=
	// o.releaseExhaustedExploreBacktrack; release(1)`) is a second,
	// UNCOUNTED caller — any non-call reference to either entry point is
	// red, matching the G-patch-txn staging-entry-point standard.
	if len(aliasRefs) != 0 {
		t.Fatalf("the release/record entry points may only be called directly — non-call references found:\n  %s", strings.Join(aliasRefs, "\n  "))
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

// exhaustionEntryPointReferences (§40.43 round-six #17) scans one function
// body for the release/record entry points: `calls` counts DIRECT selector
// calls per entry-point name; `aliases` reports every OTHER reference — a
// method value (`release := o.releaseExhaustedExploreBacktrack`) is a
// second caller the call-only matcher cannot count, so any non-call
// reference is red. The G-patch-txn staging census set this standard
// (§40.45); this sibling census escaped the hardening until round six.
func exhaustionEntryPointReferences(fd *ast.FuncDecl) (calls map[string]int, aliases []string) {
	names := map[string]bool{
		"releaseExhaustedExploreBacktrack": true,
		"RecordExploreBacktrackExhausted":  true,
	}
	calls = map[string]int{}
	callFuns := map[*ast.SelectorExpr]bool{}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && names[sel.Sel.Name] {
			callFuns[sel] = true
			calls[sel.Sel.Name]++
		}
		return true
	})
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || !names[sel.Sel.Name] || callFuns[sel] {
			return true
		}
		aliases = append(aliases, "references "+sel.Sel.Name+" without calling it (method value / alias)")
		return true
	})
	return calls, aliases
}

// TestExhaustionEntryPointReferences_SelfRed: the matcher counts direct
// calls and reds every aliased spelling.
func TestExhaustionEntryPointReferences_SelfRed(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantCalls   int
		wantAliases int
	}{
		{name: "direct call", body: "o.releaseExhaustedExploreBacktrack(1)", wantCalls: 1},
		{name: "method-value alias", body: "release := o.releaseExhaustedExploreBacktrack\n\trelease(1)", wantAliases: 1},
		{name: "record method-value alias", body: "record := mut.RecordExploreBacktrackExhausted\n\trecord(nil)", wantAliases: 1},
		{name: "alias passed as argument", body: "use(o.releaseExhaustedExploreBacktrack)", wantAliases: 1},
		{name: "direct call plus alias", body: "o.releaseExhaustedExploreBacktrack(1)\n\tr := o.releaseExhaustedExploreBacktrack\n\t_ = r", wantCalls: 1, wantAliases: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "package p\n\nfunc f() {\n\t" + tc.body + "\n}\n"
			f, err := parser.ParseFile(token.NewFileSet(), "snippet.go", src, 0)
			if err != nil {
				t.Fatalf("parse snippet: %v", err)
			}
			calls, aliases := exhaustionEntryPointReferences(f.Decls[0].(*ast.FuncDecl))
			total := 0
			for _, n := range calls {
				total += n
			}
			if total != tc.wantCalls || len(aliases) != tc.wantAliases {
				t.Fatalf("calls=%d aliases=%v, want calls=%d aliases=%d", total, aliases, tc.wantCalls, tc.wantAliases)
			}
		})
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
