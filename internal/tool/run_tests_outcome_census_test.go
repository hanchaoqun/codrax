package tool

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// run_tests_outcome_census_test.go — F-run-tests round-three fold-in
// (§40.36 finding C): the ExecutedCommand.Outcome labels are single-sourced
// as types.ExecutedCommandOutcome* constants. This census reads the REAL
// producers through go/ast — every `Outcome:` value of an ExecutedCommand
// composite literal, every setLastExecOutcome argument, every assignment to
// an identifier that feeds such a value, and every makeResourceExhaustionReport
// kind argument in the non-test files of this package — and binds the
// worktree-drift roster tables to them by shared constant identity:
//   - a producer that writes a string literal is red (a renamed literal can
//     no longer drift away from the roster / makeResourceExhaustionReport);
//   - a roster entry that is not a constant, or a constant no producer
//     writes, is red (the tables are bound to real producers, not parallel
//     literals; a dead constant is red);
//   - makeResourceExhaustionReport's switch reads the constants.

const executedCommandOutcomeConstPrefix = "ExecutedCommandOutcome"

type outcomeCensusFinding struct {
	pos  string
	text string
}

type outcomeCensusResult struct {
	violations []outcomeCensusFinding
	writers    map[string]bool // constant names written at producer positions
}

func (r *outcomeCensusResult) violate(fset *token.FileSet, node ast.Node, text string) {
	r.violations = append(r.violations, outcomeCensusFinding{pos: fset.Position(node.Pos()).String(), text: text})
}

func outcomeConstSelector(expr ast.Expr) (string, bool) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "types" || !strings.HasPrefix(sel.Sel.Name, executedCommandOutcomeConstPrefix) {
		return "", false
	}
	return sel.Sel.Name, true
}

func isExecutedCommandType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "types" && sel.Sel.Name == "ExecutedCommand"
}

// executedCommandOutcomeCensus inspects one file and records producer-position
// expressions. Literal producers are violations; constant producers are
// recorded; identifiers / calls / field selectors are pass-through carriers
// whose own literal assignments (inside the same function) are checked.
func executedCommandOutcomeCensus(fset *token.FileSet, file *ast.File, result *outcomeCensusResult) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		var producers []ast.Expr
		feeders := map[string]bool{}
		record := func(expr ast.Expr) {
			producers = append(producers, expr)
			if ident, ok := expr.(*ast.Ident); ok {
				feeders[ident.Name] = true
			}
		}
		// Pass 1: producer positions.
		var walk func(node ast.Node, inExecutedCommand bool)
		walk = func(node ast.Node, inExecutedCommand bool) {
			ast.Inspect(node, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.CompositeLit:
					typed := isExecutedCommandType(v.Type)
					elementTyped := false
					if arr, ok := v.Type.(*ast.ArrayType); ok && isExecutedCommandType(arr.Elt) {
						elementTyped = true
					}
					if v.Type == nil && inExecutedCommand {
						typed = true
					}
					for _, elt := range v.Elts {
						if kv, ok := elt.(*ast.KeyValueExpr); ok {
							if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Outcome" && typed {
								record(kv.Value)
							}
							walk(kv.Value, false)
							continue
						}
						if inner, ok := elt.(*ast.CompositeLit); ok && inner.Type == nil {
							walk(inner, elementTyped)
							continue
						}
						walk(elt, false)
					}
					return false
				case *ast.CallExpr:
					if ident, ok := v.Fun.(*ast.Ident); ok {
						switch ident.Name {
						case "setLastExecOutcome":
							if len(v.Args) == 1 {
								record(v.Args[0])
							}
						case "makeResourceExhaustionReport":
							if len(v.Args) >= 1 {
								record(v.Args[0])
							}
						}
					}
				}
				return true
			})
		}
		walk(fn.Body, false)
		// Pass 2: assignments feeding producer identifiers inside this function.
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for i, lhs := range assign.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || !feeders[ident.Name] {
					continue
				}
				if len(assign.Rhs) == len(assign.Lhs) {
					producers = append(producers, assign.Rhs[i])
				} else {
					producers = append(producers, assign.Rhs...)
				}
			}
			return true
		})
		for _, expr := range producers {
			switch e := expr.(type) {
			case *ast.BasicLit:
				result.violate(fset, e, "ExecutedCommand.Outcome producer writes the literal "+e.Value+" instead of a types.ExecutedCommandOutcome* constant")
			default:
				if name, ok := outcomeConstSelector(expr); ok {
					result.writers[name] = true
				}
			}
		}
	}
}

func parseToolPackageNonTestFiles(t *testing.T, dir string) (*token.FileSet, []*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			files = append(files, file)
		}
	}
	if len(files) == 0 {
		t.Fatalf("no non-test files parsed in %s", dir)
	}
	return fset, files
}

// declaredExecutedCommandOutcomeConstants reads the closed label set from the
// types package source (name → value) so the census is bound to the real
// declaration, not to a hand-copied list.
func declaredExecutedCommandOutcomeConstants(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join("..", "types", "test_surface.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, executedCommandOutcomeConstPrefix) || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok {
					t.Fatalf("%s must be a string literal constant", name.Name)
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				out[name.Name] = value
			}
		}
	}
	if len(out) == 0 {
		t.Fatalf("no %s* constants declared in types/test_surface.go", executedCommandOutcomeConstPrefix)
	}
	return out
}

func rosterTableSelectors(t *testing.T, fset *token.FileSet, files []*ast.File, varName string) []string {
	t.Helper()
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || len(vs.Names) != 1 || vs.Names[0].Name != varName || len(vs.Values) != 1 {
					continue
				}
				lit, ok := vs.Values[0].(*ast.CompositeLit)
				if !ok {
					t.Fatalf("%s must be a composite literal", varName)
				}
				var names []string
				for _, elt := range lit.Elts {
					name, ok := outcomeConstSelector(elt)
					if !ok {
						t.Fatalf("%s entry at %s is not a types.%s* constant: roster tables are bound to the producers' constants, never parallel literals",
							varName, fset.Position(elt.Pos()), executedCommandOutcomeConstPrefix)
					}
					names = append(names, name)
				}
				return names
			}
		}
	}
	t.Fatalf("%s not found", varName)
	return nil
}

func TestExecutedCommandOutcomeProducersAndRostersShareTypedConstants(t *testing.T) {
	declared := declaredExecutedCommandOutcomeConstants(t)
	fset, files := parseToolPackageNonTestFiles(t, ".")
	result := &outcomeCensusResult{writers: map[string]bool{}}
	for _, file := range files {
		executedCommandOutcomeCensus(fset, file, result)
	}
	for _, v := range result.violations {
		t.Errorf("%s: %s", v.pos, v.text)
	}
	// Totality: every declared constant is written by a real producer, and
	// every written constant is declared (the writers set can only contain
	// names that compile, but pin the direction explicitly).
	for name := range declared {
		if !result.writers[name] {
			t.Errorf("types.%s is declared but no producer in this package writes it (dead label)", name)
		}
	}
	for name := range result.writers {
		if _, ok := declared[name]; !ok {
			t.Errorf("producer writes undeclared constant types.%s", name)
		}
	}
	// Roster tables: constants only, each written by a producer, infra ⊆ launched.
	launched := rosterTableSelectors(t, fset, files, "verificationDriftLaunchedOutcomes")
	infra := rosterTableSelectors(t, fset, files, "verificationDriftSuiteInfraOutcomes")
	launchedSet := map[string]bool{}
	for _, name := range launched {
		launchedSet[name] = true
		if !result.writers[name] {
			t.Errorf("launched roster entry types.%s has no producer in this package", name)
		}
	}
	for _, name := range infra {
		if !launchedSet[name] {
			t.Errorf("infra roster entry types.%s is not in the launched roster", name)
		}
		if kind := makeResourceExhaustionReport(declared[name], "x").FailureKind; kind != types.FailureKindTimeout && kind != types.FailureKindOOM && kind != types.FailureKindCPULimit {
			t.Errorf("infra roster entry types.%s (%q) is not a resource-exhaustion kind for makeResourceExhaustionReport: %s", name, declared[name], kind)
		}
	}
	// Runtime identity: the tables hold exactly the declared values.
	sort.Strings(launched)
	for _, name := range launched {
		if !verificationDriftListContains(verificationDriftLaunchedOutcomes, declared[name]) {
			t.Errorf("launched roster lost %q at runtime", declared[name])
		}
	}
	// makeResourceExhaustionReport's switch reads the constants, never literals.
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "makeResourceExhaustionReport" {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				clause, ok := n.(*ast.CaseClause)
				if !ok {
					return true
				}
				for _, expr := range clause.List {
					if lit, ok := expr.(*ast.BasicLit); ok {
						t.Errorf("%s: makeResourceExhaustionReport switches on the literal %s instead of a types.%s* constant", fset.Position(lit.Pos()), lit.Value, executedCommandOutcomeConstPrefix)
					}
				}
				return true
			})
		}
	}
}

// Self-red: the checker must flag a literal producer in each producer
// position (composite literal, slice element, setLastExecOutcome argument,
// feeder assignment, makeResourceExhaustionReport kind) and must record a
// constant producer.
func TestExecutedCommandOutcomeCensusSelfRed(t *testing.T) {
	src := `package tool

import "github.com/hanchaoqun/codrax/internal/types"

func setLastExecOutcome(string) {}
func makeResourceExhaustionReport(kind, detail string) *types.ChangeReport { return nil }

func producers() {
	_ = types.ExecutedCommand{Outcome: "executed"}
	_ = []types.ExecutedCommand{{Outcome: "timeout"}}
	setLastExecOutcome("oom")
	outcome := "parser_error"
	_ = types.ExecutedCommand{Outcome: outcome}
	_ = makeResourceExhaustionReport("cpu_limit", "x")
	_ = types.VerificationDiagnostic{Outcome: "not_an_executed_command_row"}
	_ = types.ExecutedCommand{Outcome: types.ExecutedCommandOutcomeZeroTests}
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "self_red.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	result := &outcomeCensusResult{writers: map[string]bool{}}
	executedCommandOutcomeCensus(fset, file, result)
	var literals []string
	for _, v := range result.violations {
		literals = append(literals, v.text[strings.Index(v.text, "literal ")+len("literal "):])
	}
	sort.Strings(literals)
	want := []string{`"cpu_limit" instead of a types.ExecutedCommandOutcome* constant`, `"executed" instead of a types.ExecutedCommandOutcome* constant`,
		`"oom" instead of a types.ExecutedCommandOutcome* constant`, `"parser_error" instead of a types.ExecutedCommandOutcome* constant`,
		`"timeout" instead of a types.ExecutedCommandOutcome* constant`}
	if strings.Join(literals, "\n") != strings.Join(want, "\n") {
		t.Fatalf("self-red violations = %v, want %v", literals, want)
	}
	if !result.writers["ExecutedCommandOutcomeZeroTests"] || len(result.writers) != 1 {
		t.Fatalf("constant producer must be recorded exactly: %v", result.writers)
	}
}
