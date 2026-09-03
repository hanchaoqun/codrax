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

// run_tests_outcome_census_test.go — the ExecutedCommand.Outcome labels are
// single-sourced as types.ExecutedCommandOutcome* constants (F-run-tests
// round three, finding C) and every producer in this package is bound to
// them BY LOCAL DATA FLOW (fold-in round four, finding K — the previous
// census accepted identifier aliases, package-level constants,
// other-package selectors and selector/index-LHS writes, so three real
// producers wrote labels outside the declared set).
//
// Producer positions (go/ast, non-test files of this package):
//   - every `Outcome:` value of an ExecutedCommand composite literal —
//     `types.ExecutedCommand{…}`, `&types.ExecutedCommand{…}`, the elided
//     element literals of `[]types.ExecutedCommand{{…}}` and of locals /
//     fields typed as that slice;
//   - every assignment whose LHS is `<x>.Outcome` or `<x>[i].Outcome` where
//     <x> resolves, by the function's own declarations (params, var, :=,
//     range over an ExecutedCommand slice, `.ExecutedCommands` / `.Commands`
//     fields), to an ExecutedCommand or a slice of them. A `.Outcome` LHS
//     whose receiver type cannot be resolved is itself a violation, so a
//     new producer shape can never be silently skipped.
//
// The value at a producer position must resolve to a
// types.ExecutedCommandOutcome* selector through local data flow only:
//   - the selector itself;
//   - a local identifier whose every assignment in the function resolves,
//     or a parameter whose every call-site argument resolves (package
//     functions by name; closures by the local name they are bound to);
//   - a package function's return value (every `return` at that result
//     index resolves — the function-return feeder);
//   - `strings.TrimSpace(v)` of a resolvable v, or a copy of another
//     ExecutedCommand's Outcome field.
// A string literal, a package-level constant or variable (an alias), an
// other-package selector, or any other expression is a violation.
//
// Roster tables: verificationDriftLaunchedOutcomes ∪
// verificationDriftNotLaunchedOutcomes == AllExecutedCommandOutcomes and
// disjoint; the infra table ⊆ launched; each entry a constant selector with
// a real producer; every declared constant has a producer (dead label red);
// makeResourceExhaustionReport switches on the constants only.

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

// outcomeConstSelector accepts `types.ExecutedCommandOutcome*` (this package)
// and bare `ExecutedCommandOutcome*` (the types package itself).
func outcomeConstSelector(expr ast.Expr) (string, bool) {
	switch v := expr.(type) {
	case *ast.SelectorExpr:
		pkg, ok := v.X.(*ast.Ident)
		if !ok || pkg.Name != "types" || !strings.HasPrefix(v.Sel.Name, executedCommandOutcomeConstPrefix) {
			return "", false
		}
		return v.Sel.Name, true
	case *ast.Ident:
		if strings.HasPrefix(v.Name, executedCommandOutcomeConstPrefix) {
			return v.Name, true
		}
	}
	return "", false
}

func isExecutedCommandType(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.SelectorExpr:
		pkg, ok := v.X.(*ast.Ident)
		return ok && pkg.Name == "types" && v.Sel.Name == "ExecutedCommand"
	case *ast.StarExpr:
		return isExecutedCommandType(v.X)
	case *ast.ParenExpr:
		return isExecutedCommandType(v.X)
	}
	return false
}

func isExecutedCommandSliceType(expr ast.Expr) bool {
	arr, ok := expr.(*ast.ArrayType)
	return ok && isExecutedCommandType(arr.Elt)
}

// outcomeLocalKind is the census' view of a local identifier's type.
type outcomeLocalKind int

const (
	outcomeLocalUnknown      outcomeLocalKind = iota
	outcomeLocalCommand                       // types.ExecutedCommand (or pointer)
	outcomeLocalCommandSlice                  // []types.ExecutedCommand
	outcomeLocalOther                         // a declared non-ExecutedCommand type
)

// outcomeCensusScope is one function body's declaration view (closures
// inherit the enclosing scope and add their own parameters).
type outcomeCensusScope struct {
	locals map[string]outcomeLocalKind
	params map[string]int // parameter name → index (closure or declaration)
	fnName string         // package function name, or the local name a closure is bound to
}

func (s *outcomeCensusScope) child(fnName string) *outcomeCensusScope {
	out := &outcomeCensusScope{locals: map[string]outcomeLocalKind{}, params: map[string]int{}, fnName: fnName}
	for k, v := range s.locals {
		out.locals[k] = v
	}
	return out
}

func outcomeKindOfTypeExpr(expr ast.Expr) outcomeLocalKind {
	switch {
	case expr == nil:
		return outcomeLocalUnknown
	case isExecutedCommandType(expr):
		return outcomeLocalCommand
	case isExecutedCommandSliceType(expr):
		return outcomeLocalCommandSlice
	default:
		return outcomeLocalOther
	}
}

// outcomeKindOfValue infers the census type of an expression from its shape.
func outcomeKindOfValue(expr ast.Expr, scope *outcomeCensusScope) outcomeLocalKind {
	switch v := expr.(type) {
	case *ast.CompositeLit:
		return outcomeKindOfTypeExpr(v.Type)
	case *ast.UnaryExpr:
		if v.Op == token.AND {
			return outcomeKindOfValue(v.X, scope)
		}
	case *ast.ParenExpr:
		return outcomeKindOfValue(v.X, scope)
	case *ast.Ident:
		if kind, ok := scope.locals[v.Name]; ok {
			return kind
		}
	case *ast.SelectorExpr:
		if v.Sel.Name == "ExecutedCommands" || v.Sel.Name == "Commands" {
			return outcomeLocalCommandSlice
		}
	case *ast.IndexExpr:
		if outcomeKindOfValue(v.X, scope) == outcomeLocalCommandSlice {
			return outcomeLocalCommand
		}
	case *ast.CallExpr:
		switch fn := v.Fun.(type) {
		case *ast.Ident:
			if (fn.Name == "append" || fn.Name == "make") && len(v.Args) > 0 {
				return outcomeKindOfValue(v.Args[0], scope)
			}
		case *ast.ArrayType:
			return outcomeKindOfTypeExpr(fn) // []types.ExecutedCommand(nil)
		}
	}
	return outcomeLocalUnknown
}

// outcomeRecordDecl records a declaration's census type in scope.
func outcomeRecordDecl(scope *outcomeCensusScope, name string, kind outcomeLocalKind) {
	if name == "_" || name == "" {
		return
	}
	if kind == outcomeLocalUnknown {
		if _, known := scope.locals[name]; known {
			return
		}
	}
	scope.locals[name] = kind
}

// outcomeCollectScope walks one function body (not entering nested
// closures) and records every declaration shape it understands.
func outcomeCollectScope(body *ast.BlockStmt, scope *outcomeCensusScope) {
	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.DeclStmt:
			if gen, ok := v.Decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
				for _, spec := range gen.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					kind := outcomeKindOfTypeExpr(vs.Type)
					for i, name := range vs.Names {
						if vs.Type == nil && i < len(vs.Values) {
							kind = outcomeKindOfValue(vs.Values[i], scope)
						}
						outcomeRecordDecl(scope, name.Name, kind)
					}
				}
			}
		case *ast.AssignStmt:
			if len(v.Lhs) == len(v.Rhs) {
				for i, lhs := range v.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok {
						if kind := outcomeKindOfValue(v.Rhs[i], scope); kind != outcomeLocalUnknown || v.Tok == token.DEFINE {
							outcomeRecordDecl(scope, ident.Name, kind)
						}
					}
				}
			} else if v.Tok == token.DEFINE {
				for _, lhs := range v.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok {
						outcomeRecordDecl(scope, ident.Name, outcomeLocalUnknown)
					}
				}
			}
		case *ast.RangeStmt:
			if v.Value != nil {
				if ident, ok := v.Value.(*ast.Ident); ok {
					kind := outcomeLocalUnknown
					if outcomeKindOfValue(v.X, scope) == outcomeLocalCommandSlice {
						kind = outcomeLocalCommand
					} else if v.Tok == token.DEFINE {
						kind = outcomeLocalOther
					}
					outcomeRecordDecl(scope, ident.Name, kind)
				}
			}
		}
		return true
	}
	ast.Inspect(body, visit)
}

// outcomeCensusFunc is one analysed function or closure.
type outcomeCensusFunc struct {
	name   string // package function name, or the local closure name ("" for anonymous)
	params []*ast.Field
	body   *ast.BlockStmt
	scope  *outcomeCensusScope
	parent *outcomeCensusFunc
	// producers are the (position, value) pairs found in this body.
	producers []ast.Expr
	// calls are the CallExprs in this body keyed by callee ident name.
	calls map[string][]*ast.CallExpr
	// returns are the ReturnStmts of this body (not of nested closures).
	returns []*ast.ReturnStmt
}

type outcomeCensusFile struct {
	fset   *token.FileSet
	funcs  []*outcomeCensusFunc
	byName map[string][]*outcomeCensusFunc
}

func outcomeParamsInto(scope *outcomeCensusScope, params *ast.FieldList) {
	if params == nil {
		return
	}
	idx := 0
	for _, field := range params.List {
		kind := outcomeKindOfTypeExpr(field.Type)
		if len(field.Names) == 0 {
			idx++
			continue
		}
		for _, name := range field.Names {
			scope.locals[name.Name] = kind
			scope.params[name.Name] = idx
			idx++
		}
	}
}

// outcomeAnalyseBody collects producer positions, calls and returns of one
// body, recursing into closures with an inherited scope.
func outcomeAnalyseBody(fset *token.FileSet, fn *outcomeCensusFunc, result *outcomeCensusResult, out *outcomeCensusFile) {
	outcomeCollectScope(fn.body, fn.scope)
	fn.calls = map[string][]*ast.CallExpr{}
	var walk func(node ast.Node, elemTyped bool)
	walk = func(node ast.Node, elemTyped bool) {
		ast.Inspect(node, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.FuncLit:
				return false
			case *ast.ReturnStmt:
				fn.returns = append(fn.returns, v)
			case *ast.CallExpr:
				if ident, ok := v.Fun.(*ast.Ident); ok {
					fn.calls[ident.Name] = append(fn.calls[ident.Name], v)
				}
			case *ast.AssignStmt:
				// Closure bound to a local name: analysed as a named function.
				if v.Tok == token.DEFINE && len(v.Lhs) == 1 && len(v.Rhs) == 1 {
					if lit, ok := v.Rhs[0].(*ast.FuncLit); ok {
						if ident, ok := v.Lhs[0].(*ast.Ident); ok {
							outcomeAnalyseClosure(fset, fn, ident.Name, lit, result, out)
							return false
						}
					}
				}
				for _, lhs := range v.Lhs {
					sel, ok := lhs.(*ast.SelectorExpr)
					if !ok || sel.Sel.Name != "Outcome" {
						continue
					}
					switch outcomeKindOfValue(sel.X, fn.scope) {
					case outcomeLocalCommand:
						if len(v.Rhs) == len(v.Lhs) {
							for i, l := range v.Lhs {
								if l == lhs {
									fn.producers = append(fn.producers, v.Rhs[i])
								}
							}
						} else {
							result.violate(fset, v, "ExecutedCommand.Outcome written from a tuple assignment (unresolvable by the census)")
						}
					case outcomeLocalOther:
						// A declared non-ExecutedCommand receiver (diagnostic,
						// probe status, locked re-verify record).
					default:
						result.violate(fset, sel, "assignment to .Outcome whose receiver type the census cannot resolve (declare it as types.ExecutedCommand or another named type in this function)")
					}
				}
			case *ast.CompositeLit:
				// A literal is a command when its type says so or when it is
				// an elided element of a command slice / array / map value.
				typed := isExecutedCommandType(v.Type) || (v.Type == nil && elemTyped)
				elemIsCommand := isExecutedCommandSliceType(v.Type)
				if m, ok := v.Type.(*ast.MapType); ok && isExecutedCommandType(m.Value) {
					elemIsCommand = true
				}
				for _, elt := range v.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Outcome" && typed {
							fn.producers = append(fn.producers, kv.Value)
						}
						if inner, ok := kv.Value.(*ast.CompositeLit); ok && inner.Type == nil {
							walk(inner, elemIsCommand) // map value with elided type
							continue
						}
						walk(kv.Value, false)
						continue
					}
					if inner, ok := elt.(*ast.CompositeLit); ok && inner.Type == nil {
						walk(inner, elemIsCommand) // slice element with elided type
						continue
					}
					walk(elt, false)
				}
				return false
			}
			return true
		})
	}
	walk(fn.body, false)
	// Anonymous closures (not bound to a name) are analysed too: their
	// producers resolve in their own inherited scope.
	ast.Inspect(fn.body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncLit:
			if outcomeClosureIsNamed(fn.body, v) {
				return false
			}
			outcomeAnalyseClosure(fset, fn, "", v, result, out)
			return false
		}
		return true
	})
}

func outcomeClosureIsNamed(body *ast.BlockStmt, lit *ast.FuncLit) bool {
	named := false
	ast.Inspect(body, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
			if assign.Rhs[0] == lit {
				if _, ok := assign.Lhs[0].(*ast.Ident); ok {
					named = true
				}
			}
		}
		return !named
	})
	return named
}

func outcomeAnalyseClosure(fset *token.FileSet, parent *outcomeCensusFunc, name string, lit *ast.FuncLit, result *outcomeCensusResult, out *outcomeCensusFile) {
	child := &outcomeCensusFunc{name: name, body: lit.Body, scope: parent.scope.child(name), parent: parent}
	if lit.Type != nil {
		child.params = nil
		outcomeParamsInto(child.scope, lit.Type.Params)
	}
	out.funcs = append(out.funcs, child)
	if name != "" {
		out.byName[name] = append(out.byName[name], child)
	}
	outcomeAnalyseBody(fset, child, result, out)
}

// outcomeAnalyseFile analyses every function declaration of a file.
func outcomeAnalyseFile(fset *token.FileSet, file *ast.File, result *outcomeCensusResult, out *outcomeCensusFile) {
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		fn := &outcomeCensusFunc{name: fd.Name.Name, body: fd.Body, scope: &outcomeCensusScope{locals: map[string]outcomeLocalKind{}, params: map[string]int{}, fnName: fd.Name.Name}}
		if fd.Recv != nil {
			outcomeParamsInto(fn.scope, fd.Recv)
			fn.scope.params = map[string]int{} // receiver is never a feeder position
		}
		outcomeParamsInto(fn.scope, fd.Type.Params)
		out.funcs = append(out.funcs, fn)
		if fd.Recv == nil {
			out.byName[fd.Name.Name] = append(out.byName[fd.Name.Name], fn)
		}
		outcomeAnalyseBody(fset, fn, result, out)
	}
}

// outcomeResolve decides whether value resolves to a typed constant through
// local data flow; it records the constant name(s) reached.
func outcomeResolve(value ast.Expr, fn *outcomeCensusFunc, file *outcomeCensusFile, result *outcomeCensusResult, seen map[string]bool, depth int) {
	fset := file.fset
	if depth > 24 {
		result.violate(fset, value, "ExecutedCommand.Outcome producer resolution exceeded the data-flow depth bound")
		return
	}
	switch v := value.(type) {
	case *ast.BasicLit:
		result.violate(fset, v, "ExecutedCommand.Outcome producer writes the literal "+v.Value+" instead of a types.ExecutedCommandOutcome* constant")
		return
	case *ast.ParenExpr:
		outcomeResolve(v.X, fn, file, result, seen, depth+1)
		return
	case *ast.SelectorExpr:
		if name, ok := outcomeConstSelector(v); ok {
			result.writers[name] = true
			return
		}
		if v.Sel.Name == "Outcome" && outcomeKindOfValue(v.X, fn.scope) == outcomeLocalCommand {
			return // copy of another ExecutedCommand's already-declared label
		}
		result.violate(fset, v, "ExecutedCommand.Outcome producer writes the other-package / non-constant selector "+outcomeExprText(v)+" instead of a types.ExecutedCommandOutcome* constant")
		return
	case *ast.CallExpr:
		if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "strings" && sel.Sel.Name == "TrimSpace" && len(v.Args) == 1 {
				outcomeResolve(v.Args[0], fn, file, result, seen, depth+1)
				return
			}
		}
		if ident, ok := v.Fun.(*ast.Ident); ok {
			outcomeResolveReturn(ident.Name, 0, v, fn, file, result, seen, depth)
			return
		}
		result.violate(fset, v, "ExecutedCommand.Outcome producer is an unresolvable call "+outcomeExprText(v))
		return
	case *ast.Ident:
		key := fn.scope.fnName + "\x00" + v.Name + "\x00" + fset.Position(fn.body.Pos()).String()
		if seen[key] {
			return
		}
		seen[key] = true
		if outcomeResolveLocal(v, fn, file, result, seen, depth) {
			return
		}
		if idx, isParam := fn.scope.params[v.Name]; isParam {
			outcomeResolveParam(v, idx, fn, file, result, seen, depth)
			return
		}
		// Not declared in this function or its enclosing functions: a
		// package-level constant / variable alias (or an unknown name).
		result.violate(fset, v, "ExecutedCommand.Outcome producer writes the identifier "+v.Name+" that is not fed by local data flow (package-level alias / constant) — write the types.ExecutedCommandOutcome* constant")
		return
	}
	result.violate(fset, value, "ExecutedCommand.Outcome producer is an unresolvable expression "+outcomeExprText(value))
}

// outcomeResolveLocal follows every assignment to ident inside fn (and its
// enclosing functions for captured variables). Returns false when no
// assignment feeds it.
func outcomeResolveLocal(ident *ast.Ident, fn *outcomeCensusFunc, file *outcomeCensusFile, result *outcomeCensusResult, seen map[string]bool, depth int) bool {
	found := false
	for f := fn; f != nil; f = f.parent {
		ast.Inspect(f.body, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.FuncLit:
				return false
			case *ast.AssignStmt:
				for i, lhs := range v.Lhs {
					l, ok := lhs.(*ast.Ident)
					if !ok || l.Name != ident.Name {
						continue
					}
					found = true
					if len(v.Rhs) == len(v.Lhs) {
						outcomeResolve(v.Rhs[i], f, file, result, seen, depth+1)
					} else if call, ok := v.Rhs[0].(*ast.CallExpr); ok && len(v.Rhs) == 1 {
						if callee, ok := call.Fun.(*ast.Ident); ok {
							outcomeResolveReturn(callee.Name, i, call, f, file, result, seen, depth)
						} else {
							result.violate(file.fset, call, "ExecutedCommand.Outcome feeder "+ident.Name+" comes from an unresolvable tuple call")
						}
					} else {
						result.violate(file.fset, v, "ExecutedCommand.Outcome feeder "+ident.Name+" comes from an unresolvable tuple assignment")
					}
				}
			case *ast.DeclStmt:
				if gen, ok := v.Decl.(*ast.GenDecl); ok {
					for _, spec := range gen.Specs {
						vs, ok := spec.(*ast.ValueSpec)
						if !ok {
							continue
						}
						for i, name := range vs.Names {
							if name.Name != ident.Name {
								continue
							}
							found = true
							if i < len(vs.Values) {
								outcomeResolve(vs.Values[i], f, file, result, seen, depth+1)
							} else if gen.Tok == token.CONST {
								result.violate(file.fset, vs, "ExecutedCommand.Outcome feeder "+ident.Name+" is a local constant without a value")
							}
							// `var x string` with later assignments: those
							// assignments are visited by the AssignStmt arm.
						}
					}
				}
			}
			return true
		})
		if found {
			return true
		}
		if _, isParam := f.scope.params[ident.Name]; isParam {
			return false
		}
	}
	return false
}

// outcomeResolveParam follows a parameter to every call-site argument.
func outcomeResolveParam(ident *ast.Ident, idx int, fn *outcomeCensusFunc, file *outcomeCensusFile, result *outcomeCensusResult, seen map[string]bool, depth int) {
	if fn.name == "" {
		result.violate(file.fset, ident, "ExecutedCommand.Outcome producer is the parameter "+ident.Name+" of an anonymous closure (unresolvable call sites)")
		return
	}
	calls := 0
	for _, caller := range file.funcs {
		for _, call := range caller.calls[fn.name] {
			if idx >= len(call.Args) {
				continue
			}
			calls++
			outcomeResolve(call.Args[idx], caller, file, result, seen, depth+1)
		}
	}
	if calls == 0 {
		result.violate(file.fset, ident, "ExecutedCommand.Outcome producer is the parameter "+ident.Name+" of "+fn.name+", which has no call site in this package")
	}
}

// outcomeResolveReturn follows a call to a package function to that
// function's return expressions at result index idx (function-return feeder).
func outcomeResolveReturn(callee string, idx int, call *ast.CallExpr, caller *outcomeCensusFunc, file *outcomeCensusFile, result *outcomeCensusResult, seen map[string]bool, depth int) {
	targets := file.byName[callee]
	if len(targets) == 0 {
		result.violate(file.fset, call, "ExecutedCommand.Outcome producer is fed by "+callee+"(…), which is not a function of this package")
		return
	}
	key := "return\x00" + callee + "\x00" + strconv.Itoa(idx)
	if seen[key] {
		return
	}
	seen[key] = true
	for _, target := range targets {
		if len(target.returns) == 0 {
			result.violate(file.fset, call, "ExecutedCommand.Outcome producer is fed by "+callee+"(…), which has no return statement")
		}
		for _, ret := range target.returns {
			if idx >= len(ret.Results) {
				result.violate(file.fset, ret, "ExecutedCommand.Outcome feeder "+callee+" returns fewer results than the census expects")
				continue
			}
			outcomeResolve(ret.Results[idx], target, file, result, seen, depth+1)
		}
	}
}

func outcomeExprText(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return outcomeExprText(v.X) + "." + v.Sel.Name
	case *ast.CallExpr:
		return outcomeExprText(v.Fun) + "(…)"
	case *ast.BasicLit:
		return v.Value
	}
	return "<expr>"
}

// executedCommandOutcomeCensus analyses the given files as one package.
func executedCommandOutcomeCensus(fset *token.FileSet, files []*ast.File, result *outcomeCensusResult) {
	file := &outcomeCensusFile{fset: fset, byName: map[string][]*outcomeCensusFunc{}}
	for _, f := range files {
		outcomeAnalyseFile(fset, f, result, file)
	}
	for _, fn := range file.funcs {
		for _, producer := range fn.producers {
			outcomeResolve(producer, fn, file, result, map[string]bool{}, 0)
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
		t.Fatal("no ExecutedCommandOutcome* constants found in types/test_surface.go")
	}
	return out
}

// rosterTableSelectors returns the constant names of a package-level
// []string table; any non-constant element is a test failure.
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
						t.Fatalf("%s: element %s is not a types.%s* constant", fset.Position(elt.Pos()), outcomeExprText(elt), executedCommandOutcomeConstPrefix)
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
	executedCommandOutcomeCensus(fset, files, result)
	for _, v := range result.violations {
		t.Errorf("%s: %s", v.pos, v.text)
	}
	// Totality: every declared constant is written by a real producer, and
	// every written constant is declared.
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
	// Runtime identity: the declared set is exactly AllExecutedCommandOutcomes.
	all := types.AllExecutedCommandOutcomes()
	if len(all) != len(declared) {
		t.Errorf("AllExecutedCommandOutcomes has %d members, %d constants declared", len(all), len(declared))
	}
	for name, value := range declared {
		if !verificationDriftListContains(all, value) {
			t.Errorf("AllExecutedCommandOutcomes lacks types.%s (%q)", name, value)
		}
	}
	// Roster tables: constants only, each written by a producer; launched
	// and not-launched partition the closed set; infra ⊆ launched.
	launched := rosterTableSelectors(t, fset, files, "verificationDriftLaunchedOutcomes")
	notLaunched := rosterTableSelectors(t, fset, files, "verificationDriftNotLaunchedOutcomes")
	infra := rosterTableSelectors(t, fset, files, "verificationDriftSuiteInfraOutcomes")
	classified := map[string]string{}
	for _, name := range launched {
		classified[name] = "launched"
		if !result.writers[name] {
			t.Errorf("launched roster entry types.%s has no producer in this package", name)
		}
	}
	for _, name := range notLaunched {
		if prev, dup := classified[name]; dup {
			t.Errorf("types.%s is in both the %s and the not-launched roster", name, prev)
		}
		classified[name] = "not_launched"
		if !result.writers[name] {
			t.Errorf("not-launched roster entry types.%s has no producer in this package", name)
		}
	}
	for name := range declared {
		if classified[name] == "" {
			t.Errorf("types.%s is classified in neither the launched nor the not-launched drift roster", name)
		}
	}
	for _, name := range infra {
		if classified[name] != "launched" {
			t.Errorf("infra roster entry types.%s is not in the launched roster", name)
		}
		if kind := makeResourceExhaustionReport(declared[name], "x").FailureKind; kind != types.FailureKindTimeout && kind != types.FailureKindOOM && kind != types.FailureKindCPULimit {
			t.Errorf("infra roster entry types.%s (%q) is not a resource-exhaustion kind for makeResourceExhaustionReport: %s", name, declared[name], kind)
		}
	}
	// Runtime identity: the tables hold exactly the declared values and the
	// predicates agree with the tables.
	for _, name := range launched {
		if !verificationDriftCommandLaunched(types.ExecutedCommand{Outcome: declared[name]}) {
			t.Errorf("launched roster lost %q at runtime", declared[name])
		}
	}
	for _, name := range notLaunched {
		if verificationDriftCommandLaunched(types.ExecutedCommand{Outcome: declared[name]}) {
			t.Errorf("not-launched member %q is launched at runtime", declared[name])
		}
	}
	// makeResourceExhaustionReport's switch reads the constants, never literals,
	// and enumerates every member (the consumer census pins totality; the
	// literal ban is local).
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
					if lit, ok := expr.(*ast.BasicLit); ok && lit.Value != `""` {
						t.Errorf("%s: makeResourceExhaustionReport switches on the literal %s instead of a types.%s* constant", fset.Position(lit.Pos()), lit.Value, executedCommandOutcomeConstPrefix)
					}
				}
				return true
			})
		}
	}
}

// selfRedCensus runs the census over one synthetic source and returns the
// violation texts in source order.
func selfRedCensus(t *testing.T, src string) ([]string, map[string]bool) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "self_red.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	result := &outcomeCensusResult{writers: map[string]bool{}}
	executedCommandOutcomeCensus(fset, []*ast.File{file}, result)
	sort.Slice(result.violations, func(i, j int) bool { return result.violations[i].pos < result.violations[j].pos })
	var texts []string
	for _, v := range result.violations {
		texts = append(texts, v.text)
	}
	return texts, result.writers
}

func selfRedExpectViolation(t *testing.T, shape, src, wantSubstring string) {
	t.Helper()
	t.Run(shape, func(t *testing.T) {
		texts, _ := selfRedCensus(t, src)
		for _, text := range texts {
			if strings.Contains(text, wantSubstring) {
				return
			}
		}
		t.Fatalf("shape %q escaped the census (want a violation mentioning %q); violations=%v", shape, wantSubstring, texts)
	})
}

const selfRedPrelude = `package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/other"
)

const packageLevelLabel = "expected_failure_observed"
const packageLevelAlias = types.ExecutedCommandOutcomeExecuted

type probeResult struct{ Commands []types.ExecutedCommand }
type probeStatus struct{ Outcome string }
`

// Self-red: every evasion shape the round-three census accepted is a
// violation now — alias, package const, selector-LHS literal, index-LHS
// literal, other-package selector, function-return feeder — and the
// accepted shapes stay green (constant selector, local feeder, parameter
// fed by constants, closure parameter, function return of constants,
// TrimSpace pass-through, ExecutedCommand copy).
func TestExecutedCommandOutcomeCensusSelfRed(t *testing.T) {
	selfRedExpectViolation(t, "composite_literal_string", selfRedPrelude+`
func producer() { _ = types.ExecutedCommand{Outcome: "executed"} }
`, `literal "executed"`)
	selfRedExpectViolation(t, "slice_element_literal", selfRedPrelude+`
func producer() { _ = []types.ExecutedCommand{{Outcome: "timeout"}} }
`, `literal "timeout"`)
	selfRedExpectViolation(t, "alias_of_typed_constant_at_package_level", selfRedPrelude+`
func producer() { _ = types.ExecutedCommand{Outcome: packageLevelAlias} }
`, "identifier packageLevelAlias that is not fed by local data flow")
	selfRedExpectViolation(t, "package_level_const", selfRedPrelude+`
func producer() { _ = types.ExecutedCommand{Outcome: packageLevelLabel} }
`, "identifier packageLevelLabel that is not fed by local data flow")
	selfRedExpectViolation(t, "selector_lhs_literal", selfRedPrelude+`
func producer() {
	command := types.ExecutedCommand{Outcome: types.ExecutedCommandOutcomeExecuted}
	command.Outcome = "baseline_unavailable"
	_ = command
}
`, `literal "baseline_unavailable"`)
	selfRedExpectViolation(t, "selector_lhs_package_const", selfRedPrelude+`
func producer() {
	command := types.ExecutedCommand{Outcome: types.ExecutedCommandOutcomeExecuted}
	command.Outcome = packageLevelLabel
	_ = command
}
`, "identifier packageLevelLabel that is not fed by local data flow")
	selfRedExpectViolation(t, "index_lhs_literal", selfRedPrelude+`
func producer() {
	commands := []types.ExecutedCommand{{Outcome: types.ExecutedCommandOutcomeExecuted}}
	commands[0].Outcome = "parser_error"
	_ = commands
}
`, `literal "parser_error"`)
	selfRedExpectViolation(t, "index_lhs_on_var_declared_slice", selfRedPrelude+`
func producer() {
	var commands []types.ExecutedCommand
	commands = append(commands, types.ExecutedCommand{Outcome: types.ExecutedCommandOutcomeExecuted})
	commands[len(commands)-1].Outcome = "oom"
}
`, `literal "oom"`)
	selfRedExpectViolation(t, "other_package_selector", selfRedPrelude+`
func producer() { _ = types.ExecutedCommand{Outcome: other.Executed} }
`, "other-package / non-constant selector other.Executed")
	selfRedExpectViolation(t, "function_return_feeder_literal", selfRedPrelude+`
func classify(err error) (string, types.FailureKind) {
	if err == nil {
		return types.ExecutedCommandOutcomeExecuted, ""
	}
	return "runner_missing", types.FailureKindRunnerMissing
}
func producer(err error) {
	outcome, _ := classify(err)
	commands := []types.ExecutedCommand{{Outcome: types.ExecutedCommandOutcomeSyntaxPreflight}}
	commands[0].Outcome = outcome
}
`, `literal "runner_missing"`)
	selfRedExpectViolation(t, "closure_parameter_fed_by_literal", selfRedPrelude+`
func producer() {
	var executedCmds []types.ExecutedCommand
	setLastExecOutcome := func(outcome string) {
		executedCmds[len(executedCmds)-1].Outcome = outcome
	}
	setLastExecOutcome("cpu_limit")
}
`, `literal "cpu_limit"`)
	selfRedExpectViolation(t, "package_function_parameter_fed_by_literal", selfRedPrelude+`
func makeReport(kind string) *types.ChangeReport {
	_ = types.ExecutedCommand{Outcome: kind}
	return nil
}
func caller() { _ = makeReport("zero_tests") }
`, `literal "zero_tests"`)
	selfRedExpectViolation(t, "local_feeder_literal", selfRedPrelude+`
func producer() {
	outcome := "not_configured"
	_ = types.ExecutedCommand{Outcome: outcome}
}
`, `literal "not_configured"`)
	selfRedExpectViolation(t, "range_over_commands_field_literal", selfRedPrelude+`
func producer(result probeResult) {
	for i := range result.Commands {
		result.Commands[i].Outcome = "suite_skipped"
	}
}
`, `literal "suite_skipped"`)
	selfRedExpectViolation(t, "unresolvable_receiver_of_outcome_write", selfRedPrelude+`
func producer(row interface{ Row() *types.ExecutedCommand }) {
	row.Row().Outcome = types.ExecutedCommandOutcomeExecuted
}
`, "receiver type the census cannot resolve")
	selfRedExpectViolation(t, "pointer_literal_string", selfRedPrelude+`
func producer() { _ = &types.ExecutedCommand{Outcome: "probe_config_error"} }
`, `literal "probe_config_error"`)

	t.Run("accepted_shapes_stay_green", func(t *testing.T) {
		texts, writers := selfRedCensus(t, selfRedPrelude+`
func classify(err error) (string, types.FailureKind) {
	if err == nil {
		return types.ExecutedCommandOutcomeExecuted, ""
	}
	return types.ExecutedCommandOutcomeRunnerMissing, types.FailureKindRunnerMissing
}
func makeReport(kind string) *types.ChangeReport {
	_ = types.ExecutedCommand{Outcome: kind}
	return nil
}
func producer(err error, result probeResult, status probeStatus) {
	var executedCmds []types.ExecutedCommand
	executedCmds = append(executedCmds, types.ExecutedCommand{Outcome: types.ExecutedCommandOutcomeExecuted})
	setLastExecOutcome := func(outcome string) {
		executedCmds[len(executedCmds)-1].Outcome = outcome
	}
	setLastExecOutcome(types.ExecutedCommandOutcomeTimeout)
	local := types.ExecutedCommandOutcomeZeroTests
	_ = []types.ExecutedCommand{{Outcome: local}}
	compileOutcome, _ := classify(err)
	commands := []types.ExecutedCommand{{Outcome: types.ExecutedCommandOutcomeSyntaxPreflight}}
	commands[0].Outcome = compileOutcome
	command := types.ExecutedCommand{Outcome: types.ExecutedCommandOutcomeBaselineUnavailable}
	if len(result.Commands) > 0 {
		command = result.Commands[0]
	}
	command.Outcome = strings.TrimSpace(types.ExecutedCommandOutcomeExpectedFailureObserved)
	copyRow := types.ExecutedCommand{Outcome: command.Outcome}
	_ = copyRow
	status.Outcome = "passed"
	_ = types.VerificationDiagnostic{Outcome: "not_an_executed_command_row"}
	_ = makeReport(types.ExecutedCommandOutcomeOOM)
	for _, cmd := range result.Commands {
		cmd.Outcome = types.ExecutedCommandOutcomeParserError
	}
}
`)
		if len(texts) != 0 {
			t.Fatalf("accepted shapes must be green: %v", texts)
		}
		var names []string
		for name := range writers {
			names = append(names, name)
		}
		sort.Strings(names)
		want := []string{"ExecutedCommandOutcomeBaselineUnavailable", "ExecutedCommandOutcomeExecuted", "ExecutedCommandOutcomeExpectedFailureObserved",
			"ExecutedCommandOutcomeOOM", "ExecutedCommandOutcomeParserError", "ExecutedCommandOutcomeRunnerMissing",
			"ExecutedCommandOutcomeSyntaxPreflight", "ExecutedCommandOutcomeTimeout", "ExecutedCommandOutcomeZeroTests"}
		if strings.Join(names, ",") != strings.Join(want, ",") {
			t.Fatalf("recorded writers = %v, want %v", names, want)
		}
	})
}
