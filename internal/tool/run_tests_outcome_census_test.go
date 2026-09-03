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
// round three, finding C) and every producer in the module is bound to
// them BY LOCAL DATA FLOW (fold-in round four, finding K — the previous
// census accepted identifier aliases, package-level constants,
// other-package selectors and selector/index-LHS writes, so three real
// producers wrote labels outside the declared set; fold-in round five,
// findings CC/DD — the scan root is now EVERY non-test file under the repo
// root, internal/ and cmd/ (the patch-review lane in the orchestrator wrote
// a label outside the set), assignments inside closures are followed, an
// unassigned or unresolvable local is a violation, and package-level
// composite literals / tables are producer positions).
//
// Producer positions (go/ast, non-test files, every package analysed on its
// own so name resolution never crosses a package boundary):
//   - every `Outcome:` value of an ExecutedCommand composite literal —
//     `types.ExecutedCommand{…}`, `&types.ExecutedCommand{…}`, the elided
//     element literals of `[]types.ExecutedCommand{{…}}` / map values and of
//     locals / fields typed as that slice — in function bodies AND in
//     package-level var declarations (tables). Fold-in round six: composite
//     literal types are resolved through the package's own type
//     declarations (named types, aliases), package-level FuncLits (IIFE
//     tables, closure vars) are walked like any other body, and a literal
//     that writes an `Outcome:` key under a type the census CANNOT resolve
//     (a generic instantiation, an other-package named type) is a
//     violation — unrecognized shapes are red by default instead of
//     silently skipped. Package types itself must not export an
//     alias/named form of ExecutedCommand (the cross-package recognizers
//     key on the types.ExecutedCommand spelling);
//   - every assignment whose LHS is `<x>.Outcome` or `<x>[i].Outcome` where
//     <x> resolves, by the function's own declarations (params, var, :=,
//     range over an ExecutedCommand slice, `.ExecutedCommands` / `.Commands`
//     fields, the result type of a same-package function), to an
//     ExecutedCommand or a slice of them. A `.Outcome` LHS whose receiver
//     type cannot be resolved is itself a violation, so a new producer
//     shape can never be silently skipped.
//
// The value at a producer position must resolve to a
// types.ExecutedCommandOutcome* selector through local data flow only:
//   - the selector itself (the bare constant inside package types);
//   - a local identifier whose EVERY assignment — in the declaring function
//     and in every closure nested in it that does not shadow the name —
//     resolves; a local declared without any visible assignment, or fed by a
//     range clause, is a violation;
//   - a parameter whose every call-site argument resolves (package
//     functions by name; closures by the local name they are bound to;
//     a parameter captured by a nested closure resolves at the declaring
//     function's call sites);
//   - a package function's return value (every `return` at that result
//     index resolves — the function-return feeder);
//   - `strings.TrimSpace(v)` of a resolvable v, or a copy of another
//     ExecutedCommand's Outcome field.
// A string literal, a package-level constant or variable (an alias), an
// other-package selector, a package-level table value that is not the
// constant, or any other expression is a violation.
//
// Roster tables (tool package): verificationDriftLaunchedOutcomes ∪
// verificationDriftNotLaunchedOutcomes == AllExecutedCommandOutcomes and
// disjoint; the infra table ⊆ launched; each entry a constant selector with
// a real producer somewhere in the scan root; every declared constant has a
// producer (dead label red); makeResourceExhaustionReport switches on the
// constants only.

const executedCommandOutcomeConstPrefix = "ExecutedCommandOutcome"

type outcomeCensusFinding struct {
	pos  string
	text string
}

type outcomeCensusResult struct {
	violations []outcomeCensusFinding
	writers    map[string]bool // constant names written at producer positions
	producers  int             // producer positions found
}

func (r *outcomeCensusResult) violate(fset *token.FileSet, node ast.Node, text string) {
	pos := fset.Position(node.Pos()).String()
	for _, v := range r.violations {
		if v.pos == pos && v.text == text {
			return
		}
	}
	r.violations = append(r.violations, outcomeCensusFinding{pos: pos, text: text})
}

// outcomeConstSelector accepts `types.ExecutedCommandOutcome*` and, inside
// package types itself, the bare `ExecutedCommandOutcome*`.
func outcomeConstSelector(expr ast.Expr, pkgName string) (string, bool) {
	switch v := expr.(type) {
	case *ast.SelectorExpr:
		pkg, ok := v.X.(*ast.Ident)
		if !ok || pkg.Name != "types" || !strings.HasPrefix(v.Sel.Name, executedCommandOutcomeConstPrefix) {
			return "", false
		}
		return v.Sel.Name, true
	case *ast.Ident:
		if pkgName == "types" && strings.HasPrefix(v.Name, executedCommandOutcomeConstPrefix) {
			return v.Name, true
		}
	case *ast.ParenExpr:
		return outcomeConstSelector(v.X, pkgName)
	}
	return "", false
}

func isExecutedCommandType(expr ast.Expr, pkgName string) bool {
	switch v := expr.(type) {
	case *ast.SelectorExpr:
		pkg, ok := v.X.(*ast.Ident)
		return ok && pkg.Name == "types" && v.Sel.Name == "ExecutedCommand"
	case *ast.Ident:
		return pkgName == "types" && v.Name == "ExecutedCommand"
	case *ast.StarExpr:
		return isExecutedCommandType(v.X, pkgName)
	case *ast.ParenExpr:
		return isExecutedCommandType(v.X, pkgName)
	}
	return false
}

func isExecutedCommandSliceType(expr ast.Expr, pkgName string) bool {
	arr, ok := expr.(*ast.ArrayType)
	return ok && isExecutedCommandType(arr.Elt, pkgName)
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

// outcomeCensusFile is one analysed package.
type outcomeCensusFile struct {
	fset    *token.FileSet
	pkgName string
	funcs   []*outcomeCensusFunc
	byName  map[string][]*outcomeCensusFunc
	// resultKinds is the census type of the first result of every package
	// function / method by simple name (pre-pass), so `x := build()` can
	// be classified as a declared receiver.
	resultKinds map[string]outcomeLocalKind
	// typeDecls maps the package's declared type names to their TypeSpecs
	// (fold-in round six): named types, aliases and generic declarations
	// are resolved instead of silently trusted.
	typeDecls map[string]*ast.TypeSpec
}

// outcomeBuiltinTypeNames are the predeclared type names a bare identifier
// may legitimately resolve to outside the package's own declarations.
var outcomeBuiltinTypeNames = map[string]bool{
	"bool": true, "byte": true, "complex64": true, "complex128": true,
	"error": true, "float32": true, "float64": true, "int": true,
	"int8": true, "int16": true, "int32": true, "int64": true, "rune": true,
	"string": true, "uint": true, "uint8": true, "uint16": true,
	"uint32": true, "uint64": true, "uintptr": true, "any": true,
	"comparable": true,
}

func (file *outcomeCensusFile) kindOfTypeExpr(expr ast.Expr) outcomeLocalKind {
	return file.kindOfTypeExprSeen(expr, map[string]bool{})
}

// kindOfTypeExprSeen classifies a type expression, resolving package-local
// named types and aliases through typeDecls (fold-in round six). An
// expression it cannot classify — an undeclared bare name (a type
// parameter, a dot import), an other-package named type outside package
// types, a generic instantiation — is outcomeLocalUnknown, which the
// producer positions treat as a violation rather than a silent skip.
func (file *outcomeCensusFile) kindOfTypeExprSeen(expr ast.Expr, seen map[string]bool) outcomeLocalKind {
	switch {
	case expr == nil:
		return outcomeLocalUnknown
	case isExecutedCommandType(expr, file.pkgName):
		return outcomeLocalCommand
	}
	switch v := expr.(type) {
	case *ast.ArrayType:
		if file.kindOfTypeExprSeen(v.Elt, seen) == outcomeLocalCommand {
			return outcomeLocalCommandSlice
		}
		return outcomeLocalOther
	case *ast.Ident:
		if outcomeBuiltinTypeNames[v.Name] || seen[v.Name] {
			return outcomeLocalOther
		}
		if ts, ok := file.typeDecls[v.Name]; ok {
			if ts == nil {
				return outcomeLocalUnknown // ambiguous duplicate declaration
			}
			seen[v.Name] = true
			return file.kindOfTypeExprSeen(ts.Type, seen)
		}
		return outcomeLocalUnknown
	case *ast.ParenExpr:
		return file.kindOfTypeExprSeen(v.X, seen)
	case *ast.StarExpr:
		return file.kindOfTypeExprSeen(v.X, seen)
	case *ast.SelectorExpr:
		if pkg, ok := v.X.(*ast.Ident); ok && pkg.Name == "types" {
			// A non-command types.X name. The types package is pinned to
			// declare no alias/named form of ExecutedCommand (see
			// analyseTypeDecls), so this is a recognized non-command type.
			return outcomeLocalOther
		}
		return outcomeLocalUnknown
	case *ast.StructType, *ast.InterfaceType, *ast.FuncType, *ast.ChanType, *ast.MapType:
		return outcomeLocalOther
	}
	return outcomeLocalUnknown
}

// underlyingContainerType resolves expr through parentheses and
// package-local named types to an array/slice or map type, or nil.
func (file *outcomeCensusFile) underlyingContainerType(expr ast.Expr) ast.Expr {
	seen := map[string]bool{}
	for {
		switch v := expr.(type) {
		case *ast.ParenExpr:
			expr = v.X
		case *ast.Ident:
			ts, ok := file.typeDecls[v.Name]
			if !ok || ts == nil || seen[v.Name] {
				return nil
			}
			seen[v.Name] = true
			expr = ts.Type
		case *ast.ArrayType, *ast.MapType:
			return expr
		default:
			return nil
		}
	}
}

// elementTypeExpr is the type expression an elided element literal of a
// literal typed expr assumes (the array element / map value), nil when expr
// is not a container the census can resolve.
func (file *outcomeCensusFile) elementTypeExpr(expr ast.Expr) ast.Expr {
	switch v := file.underlyingContainerType(expr).(type) {
	case *ast.ArrayType:
		return v.Elt
	case *ast.MapType:
		return v.Value
	}
	return nil
}

// kindOfValue infers the census type of an expression from its shape.
func (file *outcomeCensusFile) kindOfValue(expr ast.Expr, scope *outcomeCensusScope) outcomeLocalKind {
	switch v := expr.(type) {
	case *ast.CompositeLit:
		return file.kindOfTypeExpr(v.Type)
	case *ast.UnaryExpr:
		if v.Op == token.AND {
			return file.kindOfValue(v.X, scope)
		}
	case *ast.ParenExpr:
		return file.kindOfValue(v.X, scope)
	case *ast.Ident:
		if kind, ok := scope.locals[v.Name]; ok {
			return kind
		}
	case *ast.SelectorExpr:
		if v.Sel.Name == "ExecutedCommands" || v.Sel.Name == "Commands" {
			return outcomeLocalCommandSlice
		}
	case *ast.IndexExpr:
		if file.kindOfValue(v.X, scope) == outcomeLocalCommandSlice {
			return outcomeLocalCommand
		}
	case *ast.CallExpr:
		switch fn := v.Fun.(type) {
		case *ast.Ident:
			if (fn.Name == "append" || fn.Name == "make") && len(v.Args) > 0 {
				return file.kindOfValue(v.Args[0], scope)
			}
			if kind, ok := file.resultKinds[fn.Name]; ok {
				return kind
			}
		case *ast.SelectorExpr:
			if kind, ok := file.resultKinds[fn.Sel.Name]; ok {
				return kind
			}
		case *ast.ArrayType:
			return file.kindOfTypeExpr(fn) // []types.ExecutedCommand(nil)
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

// collectScope walks one function body (not entering nested closures) and
// records every declaration shape it understands.
func (file *outcomeCensusFile) collectScope(body *ast.BlockStmt, scope *outcomeCensusScope) {
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
					kind := file.kindOfTypeExpr(vs.Type)
					for i, name := range vs.Names {
						if vs.Type == nil && i < len(vs.Values) {
							kind = file.kindOfValue(vs.Values[i], scope)
						}
						outcomeRecordDecl(scope, name.Name, kind)
					}
				}
			}
		case *ast.AssignStmt:
			if len(v.Lhs) == len(v.Rhs) {
				for i, lhs := range v.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok {
						if kind := file.kindOfValue(v.Rhs[i], scope); kind != outcomeLocalUnknown || v.Tok == token.DEFINE {
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
					if file.kindOfValue(v.X, scope) == outcomeLocalCommandSlice {
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

// outcomeCensusFunc is one analysed function, closure, or the package-level
// declaration pseudo-body.
type outcomeCensusFunc struct {
	name   string // package function name, or the local closure name ("" for anonymous)
	body   *ast.BlockStmt
	lit    *ast.FuncLit // the closure literal (nil for declarations)
	scope  *outcomeCensusScope
	parent *outcomeCensusFunc
	// producers are the (position, value) pairs found in this body.
	producers []ast.Expr
	// calls are the CallExprs in this body keyed by callee ident name.
	calls map[string][]*ast.CallExpr
	// returns are the ReturnStmts of this body (not of nested closures).
	returns []*ast.ReturnStmt
}

func outcomeParamsInto(file *outcomeCensusFile, scope *outcomeCensusScope, params *ast.FieldList) {
	if params == nil {
		return
	}
	idx := 0
	for _, field := range params.List {
		kind := file.kindOfTypeExpr(field.Type)
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

// walkProducers collects producer positions (composite literals and .Outcome
// writes) in node, not entering closures. elemType is the type expression a
// nil-typed (elided) composite literal at the top of node assumes, nil when
// there is none.
func (file *outcomeCensusFile) walkProducers(fn *outcomeCensusFunc, node ast.Node, elemType ast.Expr, result *outcomeCensusResult) {
	fset := file.fset
	var walk func(node ast.Node, elemType ast.Expr)
	walk = func(node ast.Node, elemType ast.Expr) {
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
							file.analyseClosure(fn, ident.Name, lit, result)
							return false
						}
					}
				}
				for _, lhs := range v.Lhs {
					sel, ok := lhs.(*ast.SelectorExpr)
					if !ok || sel.Sel.Name != "Outcome" {
						continue
					}
					switch file.kindOfValue(sel.X, fn.scope) {
					case outcomeLocalCommand:
						if len(v.Rhs) == len(v.Lhs) {
							for i, l := range v.Lhs {
								if l == lhs {
									fn.producers = append(fn.producers, v.Rhs[i])
									result.producers++
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
				// The literal's own type: explicit, or — for an elided
				// element — the container's element type. It is resolved
				// through the package's type declarations; a literal that
				// writes an `Outcome:` key under a type the census cannot
				// classify is a violation (fold-in round six), never a
				// silent skip.
				typeExpr := v.Type
				if typeExpr == nil {
					typeExpr = elemType
				}
				self := file.kindOfTypeExpr(typeExpr)
				childElem := file.elementTypeExpr(typeExpr)
				for _, elt := range v.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Outcome" {
							switch self {
							case outcomeLocalCommand:
								fn.producers = append(fn.producers, kv.Value)
								result.producers++
							case outcomeLocalOther:
								// A recognized non-command literal (a
								// diagnostic, a probe status, …).
							default:
								result.violate(fset, kv, "composite literal writes an Outcome key but its type cannot be resolved by the census (unrecognized shape — name the type resolvably or extend the census)")
							}
						}
						if inner, ok := kv.Value.(*ast.CompositeLit); ok && inner.Type == nil {
							walk(inner, childElem) // map value with elided type
							continue
						}
						walk(kv.Value, nil)
						continue
					}
					if inner, ok := elt.(*ast.CompositeLit); ok && inner.Type == nil {
						walk(inner, childElem) // slice element with elided type
						continue
					}
					walk(elt, nil)
				}
				return false
			}
			return true
		})
	}
	walk(node, elemType)
}

// analyseBody collects producer positions, calls and returns of one body,
// recursing into closures with an inherited scope.
func (file *outcomeCensusFile) analyseBody(fn *outcomeCensusFunc, result *outcomeCensusResult) {
	file.collectScope(fn.body, fn.scope)
	fn.calls = map[string][]*ast.CallExpr{}
	file.walkProducers(fn, fn.body, nil, result)
	// Anonymous closures (not bound to a name) are analysed too: their
	// producers resolve in their own inherited scope.
	ast.Inspect(fn.body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncLit:
			if outcomeClosureIsNamed(fn.body, v) {
				return false
			}
			file.analyseClosure(fn, "", v, result)
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

func (file *outcomeCensusFile) analyseClosure(parent *outcomeCensusFunc, name string, lit *ast.FuncLit, result *outcomeCensusResult) {
	child := &outcomeCensusFunc{name: name, body: lit.Body, lit: lit, scope: parent.scope.child(name), parent: parent}
	if lit.Type != nil {
		outcomeParamsInto(file, child.scope, lit.Type.Params)
	}
	file.funcs = append(file.funcs, child)
	if name != "" {
		file.byName[name] = append(file.byName[name], child)
	}
	file.analyseBody(child, result)
}

// analyseFile analyses every function declaration and every package-level
// var declaration (tables) of a file.
func (file *outcomeCensusFile) analyseFile(f *ast.File, result *outcomeCensusResult) {
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Body == nil {
				continue
			}
			fn := &outcomeCensusFunc{name: d.Name.Name, body: d.Body, scope: &outcomeCensusScope{locals: map[string]outcomeLocalKind{}, params: map[string]int{}, fnName: d.Name.Name}}
			if d.Recv != nil {
				outcomeParamsInto(file, fn.scope, d.Recv)
				fn.scope.params = map[string]int{} // receiver is never a feeder position
			}
			outcomeParamsInto(file, fn.scope, d.Type.Params)
			file.funcs = append(file.funcs, fn)
			if d.Recv == nil {
				file.byName[d.Name.Name] = append(file.byName[d.Name.Name], fn)
			}
			file.analyseBody(fn, result)
		case *ast.GenDecl:
			if d.Tok != token.VAR {
				continue
			}
			// Package-level tables (fold-in round five, finding DD): the
			// values are producer positions resolved in an empty scope, so
			// only the constant selector itself is accepted. Fold-in round
			// six: a FuncLit in a package-level value (an IIFE table, a
			// closure var, a hook field) is analysed as a closure of the
			// pseudo-body — the round-five walker skipped every FuncLit, so
			// an IIFE-built command table was invisible.
			fn := &outcomeCensusFunc{name: "<package-level " + file.pkgName + ">", body: &ast.BlockStmt{},
				scope: &outcomeCensusScope{locals: map[string]outcomeLocalKind{}, params: map[string]int{}, fnName: "<package-level>"}}
			fn.calls = map[string][]*ast.CallExpr{}
			for _, spec := range d.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, value := range vs.Values {
					if lit, ok := value.(*ast.FuncLit); ok {
						name := ""
						if i < len(vs.Names) {
							name = vs.Names[i].Name
						}
						file.analyseClosure(fn, name, lit, result)
						continue
					}
					if lit, ok := value.(*ast.CompositeLit); ok && lit.Type == nil {
						file.walkProducers(fn, lit, vs.Type, result)
					} else {
						file.walkProducers(fn, value, nil, result)
					}
					ast.Inspect(value, func(n ast.Node) bool {
						if inner, ok := n.(*ast.FuncLit); ok {
							file.analyseClosure(fn, "", inner, result)
							return false
						}
						return true
					})
				}
			}
			if len(fn.producers) > 0 {
				file.funcs = append(file.funcs, fn)
			}
		}
	}
}

// analyseTypeDecls records the package's type declarations (fold-in round
// six) and, for package types itself, rejects any alias/named form of the
// command type — the cross-package recognizers key on the
// types.ExecutedCommand spelling, so an exported alias would let another
// package build command rows under a name no census resolves.
func (file *outcomeCensusFile) analyseTypeDecls(files []*ast.File, result *outcomeCensusResult) {
	file.typeDecls = map[string]*ast.TypeSpec{}
	// Every TypeSpec of the package, package-level AND function-local (a
	// struct type declared inside a function body is a legitimate receiver
	// shape). Simple names are the key; when the same name is declared
	// twice with different census kinds the entry becomes a nil sentinel,
	// which kindOfTypeExpr treats as unresolvable (fail-loud, never a
	// silent guess).
	specs := map[string][]*ast.TypeSpec{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if ts, ok := n.(*ast.TypeSpec); ok {
				specs[ts.Name.Name] = append(specs[ts.Name.Name], ts)
			}
			return true
		})
	}
	for name, list := range specs {
		file.typeDecls[name] = list[0]
	}
	for name, list := range specs {
		if len(list) < 2 {
			continue
		}
		kind := file.kindOfTypeExpr(list[0].Type)
		for _, ts := range list[1:] {
			if file.kindOfTypeExpr(ts.Type) != kind {
				file.typeDecls[name] = nil
			}
		}
	}
	if file.pkgName != "types" {
		return
	}
	for name, ts := range file.typeDecls {
		if name == "ExecutedCommand" || ts == nil {
			continue
		}
		switch file.kindOfTypeExpr(ts.Type) {
		case outcomeLocalCommand, outcomeLocalCommandSlice:
			result.violate(file.fset, ts, "package types declares "+name+" as an alias/named form of ExecutedCommand — the cross-package censuses key on the types.ExecutedCommand spelling; do not alias the command type")
		}
	}
}

// resultKindsPrePass records the first-result census type of every function
// and method of the package by simple name.
func (file *outcomeCensusFile) resultKindsPrePass(files []*ast.File) {
	file.resultKinds = map[string]outcomeLocalKind{}
	for _, f := range files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Type.Results == nil || len(fd.Type.Results.List) == 0 {
				continue
			}
			kind := file.kindOfTypeExpr(fd.Type.Results.List[0].Type)
			if prev, ok := file.resultKinds[fd.Name.Name]; ok && prev != kind {
				kind = outcomeLocalUnknown // ambiguous simple name: stay conservative
			}
			file.resultKinds[fd.Name.Name] = kind
		}
	}
}

// resolve decides whether value resolves to a typed constant through local
// data flow; it records the constant name(s) reached.
func (file *outcomeCensusFile) resolve(value ast.Expr, fn *outcomeCensusFunc, result *outcomeCensusResult, seen map[string]bool, depth int) {
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
		file.resolve(v.X, fn, result, seen, depth+1)
		return
	case *ast.SelectorExpr:
		if name, ok := outcomeConstSelector(v, file.pkgName); ok {
			result.writers[name] = true
			return
		}
		if v.Sel.Name == "Outcome" && file.kindOfValue(v.X, fn.scope) == outcomeLocalCommand {
			return // copy of another ExecutedCommand's already-declared label
		}
		result.violate(fset, v, "ExecutedCommand.Outcome producer writes the other-package / non-constant selector "+outcomeExprText(v)+" instead of a types.ExecutedCommandOutcome* constant")
		return
	case *ast.CallExpr:
		if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "strings" && sel.Sel.Name == "TrimSpace" && len(v.Args) == 1 {
				file.resolve(v.Args[0], fn, result, seen, depth+1)
				return
			}
		}
		if ident, ok := v.Fun.(*ast.Ident); ok {
			file.resolveReturn(ident.Name, 0, v, result, seen, depth)
			return
		}
		result.violate(fset, v, "ExecutedCommand.Outcome producer is an unresolvable call "+outcomeExprText(v))
		return
	case *ast.Ident:
		if name, ok := outcomeConstSelector(v, file.pkgName); ok {
			result.writers[name] = true
			return
		}
		key := fn.scope.fnName + "\x00" + v.Name + "\x00" + fset.Position(fn.body.Pos()).String()
		if seen[key] {
			return
		}
		seen[key] = true
		decl, isParam, idx := outcomeDeclaringFunc(v.Name, fn)
		if decl == nil {
			// Not declared in this function or its enclosing functions: a
			// package-level constant / variable alias (or an unknown name).
			result.violate(fset, v, "ExecutedCommand.Outcome producer writes the identifier "+v.Name+" that is not fed by local data flow (package-level alias / constant) — write the types.ExecutedCommandOutcome* constant")
			return
		}
		if isParam {
			file.resolveParam(v, idx, decl, result, seen, depth)
			return
		}
		file.resolveLocal(v, decl, result, seen, depth)
		return
	}
	result.violate(fset, value, "ExecutedCommand.Outcome producer is an unresolvable expression "+outcomeExprText(value))
}

// outcomeDeclaringFunc finds the innermost function (walking outward through
// enclosing closures) that declares name — as a parameter (isParam, index)
// or as a local (:=, var, range) — nil when no enclosing function declares
// it (a package-level name).
func outcomeDeclaringFunc(name string, fn *outcomeCensusFunc) (decl *outcomeCensusFunc, isParam bool, idx int) {
	for f := fn; f != nil; f = f.parent {
		if i, ok := f.scope.params[name]; ok {
			return f, true, i
		}
		if outcomeBodyDeclares(f.body, name) {
			return f, false, 0
		}
	}
	return nil, false, 0
}

// outcomeBodyDeclares reports whether body (not its nested closures)
// declares name with :=, var/const, or a range clause.
func outcomeBodyDeclares(body *ast.BlockStmt, name string) bool {
	declared := false
	ast.Inspect(body, func(n ast.Node) bool {
		if declared {
			return false
		}
		switch v := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.AssignStmt:
			if v.Tok == token.DEFINE {
				for _, lhs := range v.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok && ident.Name == name {
						declared = true
					}
				}
			}
		case *ast.DeclStmt:
			if gen, ok := v.Decl.(*ast.GenDecl); ok {
				for _, spec := range gen.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, n := range vs.Names {
							if n.Name == name {
								declared = true
							}
						}
					}
				}
			}
		case *ast.RangeStmt:
			if v.Tok == token.DEFINE {
				for _, e := range []ast.Expr{v.Key, v.Value} {
					if ident, ok := e.(*ast.Ident); ok && ident.Name == name {
						declared = true
					}
				}
			}
		}
		return true
	})
	return declared
}

// outcomeClosureShadows reports whether a closure literal redeclares name
// (as a parameter or with := / var inside it, not in deeper closures).
func outcomeClosureShadows(lit *ast.FuncLit, name string) bool {
	if lit.Type != nil && lit.Type.Params != nil {
		for _, field := range lit.Type.Params.List {
			for _, n := range field.Names {
				if n.Name == name {
					return true
				}
			}
		}
	}
	return outcomeBodyDeclares(lit.Body, name)
}

// resolveLocal follows EVERY assignment to ident inside its declaring
// function decl — including assignments made by closures nested in it that
// do not shadow the name (fold-in round five, finding DD). A local declared
// without any visible assignment, or fed by a range clause, is a violation.
func (file *outcomeCensusFile) resolveLocal(ident *ast.Ident, decl *outcomeCensusFunc, result *outcomeCensusResult, seen map[string]bool, depth int) {
	feeders := 0
	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncLit:
			if outcomeClosureShadows(v, ident.Name) {
				return false
			}
			return true
		case *ast.AssignStmt:
			for i, lhs := range v.Lhs {
				l, ok := lhs.(*ast.Ident)
				if !ok || l.Name != ident.Name {
					continue
				}
				feeders++
				if len(v.Rhs) == len(v.Lhs) {
					file.resolve(v.Rhs[i], decl, result, seen, depth+1)
				} else if call, ok := v.Rhs[0].(*ast.CallExpr); ok && len(v.Rhs) == 1 {
					if callee, ok := call.Fun.(*ast.Ident); ok {
						file.resolveReturn(callee.Name, i, call, result, seen, depth)
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
						if i < len(vs.Values) {
							feeders++
							file.resolve(vs.Values[i], decl, result, seen, depth+1)
						} else if gen.Tok == token.CONST {
							feeders++
							result.violate(file.fset, vs, "ExecutedCommand.Outcome feeder "+ident.Name+" is a local constant without a value")
						}
						// `var x string` with later assignments: those
						// assignments are visited by the AssignStmt arm.
					}
				}
			}
		case *ast.RangeStmt:
			for _, e := range []ast.Expr{v.Key, v.Value} {
				if id, ok := e.(*ast.Ident); ok && id.Name == ident.Name {
					feeders++
					result.violate(file.fset, id, "ExecutedCommand.Outcome feeder "+ident.Name+" is bound by a range clause (unresolvable by the census)")
				}
			}
		}
		return true
	}
	ast.Inspect(decl.body, visit)
	if feeders == 0 {
		result.violate(file.fset, ident, "ExecutedCommand.Outcome producer reads the local "+ident.Name+" which is declared without any visible assignment (unassigned or written through an unresolvable path)")
	}
}

// resolveParam follows a parameter of decl to every call-site argument.
func (file *outcomeCensusFile) resolveParam(ident *ast.Ident, idx int, decl *outcomeCensusFunc, result *outcomeCensusResult, seen map[string]bool, depth int) {
	if decl.name == "" {
		result.violate(file.fset, ident, "ExecutedCommand.Outcome producer is the parameter "+ident.Name+" of an anonymous closure (unresolvable call sites)")
		return
	}
	calls := 0
	for _, caller := range file.funcs {
		for _, call := range caller.calls[decl.name] {
			if idx >= len(call.Args) {
				continue
			}
			calls++
			file.resolve(call.Args[idx], caller, result, seen, depth+1)
		}
	}
	if calls == 0 {
		result.violate(file.fset, ident, "ExecutedCommand.Outcome producer is the parameter "+ident.Name+" of "+decl.name+", which has no call site in this package")
	}
}

// resolveReturn follows a call to a package function to that function's
// return expressions at result index idx (function-return feeder).
func (file *outcomeCensusFile) resolveReturn(callee string, idx int, call *ast.CallExpr, result *outcomeCensusResult, seen map[string]bool, depth int) {
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
			file.resolve(ret.Results[idx], target, result, seen, depth+1)
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
	if len(files) == 0 {
		return
	}
	file := &outcomeCensusFile{fset: fset, pkgName: files[0].Name.Name, byName: map[string][]*outcomeCensusFunc{}}
	file.analyseTypeDecls(files, result)
	file.resultKindsPrePass(files)
	for _, f := range files {
		file.analyseFile(f, result)
	}
	for _, fn := range file.funcs {
		for _, producer := range fn.producers {
			file.resolve(producer, fn, result, map[string]bool{}, 0)
		}
	}
}

func parseToolPackageNonTestFiles(t *testing.T, dir string) (*token.FileSet, []*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	files := parseNonTestFilesOfDir(t, fset, dir)
	if len(files) == 0 {
		t.Fatalf("no non-test files parsed in %s", dir)
	}
	return fset, files
}

// parseNonTestFilesOfDir parses the non-test .go files of one directory
// (every package found there) into fset; nil when the directory holds none.
func parseNonTestFilesOfDir(t *testing.T, fset *token.FileSet, dir string) []*ast.File {
	t.Helper()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for name := range pkgs {
		names = append(names, name)
	}
	sort.Strings(names)
	var files []*ast.File
	for _, name := range names {
		var paths []string
		for path := range pkgs[name].Files {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			files = append(files, pkgs[name].Files[path])
		}
	}
	return files
}

// executedCommandProducerScanDirs returns every directory of the producer
// scan root (fold-in round five, finding CC): the repository root (its own
// files only), internal/ and cmd/ recursively. eval/ holds fixture repos
// with deliberately broken Go and is never scanned.
func executedCommandProducerScanDirs(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..")
	dirs := []string{root}
	for _, sub := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, sub), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				return nil
			}
			if name := d.Name(); name == "testdata" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			dirs = append(dirs, path)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return dirs
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
					name, ok := outcomeConstSelector(elt, "tool")
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
	// Producer census over the whole scan root, one package at a time.
	result := &outcomeCensusResult{writers: map[string]bool{}}
	scanned := 0
	for _, dir := range executedCommandProducerScanDirs(t) {
		fset := token.NewFileSet()
		files := parseNonTestFilesOfDir(t, fset, dir)
		if len(files) == 0 {
			continue
		}
		scanned++
		executedCommandOutcomeCensus(fset, files, result)
	}
	if scanned < 20 {
		t.Fatalf("producer census scanned only %d package directories; the scan root must cover internal/ and cmd/", scanned)
	}
	for _, v := range result.violations {
		t.Errorf("%s: %s", v.pos, v.text)
	}
	// Sanity floor: the scan root holds 29 real producer positions today
	// (run_tests* / java / probe files plus the two orchestrator
	// patch-review rows); a collapse below 25 means the walker went blind,
	// not that producers vanished.
	if result.producers < 25 {
		t.Errorf("producer census found only %d producer positions", result.producers)
	}
	// Totality: every declared constant is written by a real producer, and
	// every written constant is declared.
	for name := range declared {
		if !result.writers[name] {
			t.Errorf("types.%s is declared but no producer in the scan root writes it (dead label)", name)
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
	fset, files := parseToolPackageNonTestFiles(t, ".")
	launched := rosterTableSelectors(t, fset, files, "verificationDriftLaunchedOutcomes")
	notLaunched := rosterTableSelectors(t, fset, files, "verificationDriftNotLaunchedOutcomes")
	infra := rosterTableSelectors(t, fset, files, "verificationDriftSuiteInfraOutcomes")
	classified := map[string]string{}
	for _, name := range launched {
		classified[name] = "launched"
		if !result.writers[name] {
			t.Errorf("launched roster entry types.%s has no producer in the scan root", name)
		}
	}
	for _, name := range notLaunched {
		if prev, dup := classified[name]; dup {
			t.Errorf("types.%s is in both the %s and the not-launched roster", name, prev)
		}
		classified[name] = "not_launched"
		if !result.writers[name] {
			t.Errorf("not-launched roster entry types.%s has no producer in the scan root", name)
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
// literal, other-package selector, function-return feeder — and so are the
// round-five shapes (closure-captured writes, unassigned locals, a constant
// recorded before a closure rewrites the value, package-level tables); the
// accepted shapes stay green (constant selector, local feeder, parameter
// fed by constants, closure parameter, function return of constants,
// TrimSpace pass-through, ExecutedCommand copy, closure writes of
// constants, package-level tables of constants).
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

	// Fold-in round five, finding DD (i): writes made inside closures.
	selfRedExpectViolation(t, "closure_captured_local_written_with_literal", selfRedPrelude+`
func producer() {
	outcome := types.ExecutedCommandOutcomeExecuted
	func() {
		outcome = "timeout"
	}()
	_ = types.ExecutedCommand{Outcome: outcome}
}
`, `literal "timeout"`)
	selfRedExpectViolation(t, "closure_bound_to_name_writes_captured_local_with_literal", selfRedPrelude+`
func producer() {
	outcome := types.ExecutedCommandOutcomeExecuted
	classify := func(err error) {
		if err != nil {
			outcome = "oom"
		}
	}
	classify(nil)
	_ = types.ExecutedCommand{Outcome: outcome}
}
`, `literal "oom"`)
	selfRedExpectViolation(t, "nested_closure_writes_captured_local_with_package_const", selfRedPrelude+`
func producer() {
	outcome := types.ExecutedCommandOutcomeExecuted
	go func() {
		defer func() { outcome = packageLevelLabel }()
	}()
	_ = types.ExecutedCommand{Outcome: outcome}
}
`, "identifier packageLevelLabel that is not fed by local data flow")
	selfRedExpectViolation(t, "var_declared_without_any_assignment", selfRedPrelude+`
func producer() {
	var outcome string
	_ = types.ExecutedCommand{Outcome: outcome}
}
`, "declared without any visible assignment")
	selfRedExpectViolation(t, "var_assigned_only_through_pointer_is_unresolvable", selfRedPrelude+`
func producer() {
	var outcome string
	p := &outcome
	*p = types.ExecutedCommandOutcomeExecuted
	_ = types.ExecutedCommand{Outcome: outcome}
}
`, "declared without any visible assignment")
	selfRedExpectViolation(t, "range_bound_local", selfRedPrelude+`
func producer(labels []string) {
	for _, outcome := range labels {
		_ = types.ExecutedCommand{Outcome: outcome}
	}
}
`, "bound by a range clause")
	selfRedExpectViolation(t, "captured_parent_parameter_fed_by_literal", selfRedPrelude+`
func producer(outcome string) {
	emit := func() { _ = types.ExecutedCommand{Outcome: outcome} }
	emit()
}
func caller() { producer("oom") }
`, `literal "oom"`)
	// Fold-in round five, finding DD (ii): package-level tables.
	selfRedExpectViolation(t, "package_level_command_literal", selfRedPrelude+`
var refusedRow = types.ExecutedCommand{Runner: "x", Outcome: "runner_missing"}
`, `literal "runner_missing"`)
	selfRedExpectViolation(t, "package_level_typed_var_elided_literal", selfRedPrelude+`
var refusedRow types.ExecutedCommand = types.ExecutedCommand{Outcome: "not_configured"}
`, `literal "not_configured"`)
	selfRedExpectViolation(t, "package_level_slice_table", selfRedPrelude+`
var rows = []types.ExecutedCommand{{Outcome: types.ExecutedCommandOutcomeExecuted}, {Outcome: "zero_tests"}}
`, `literal "zero_tests"`)
	selfRedExpectViolation(t, "package_level_map_table", selfRedPrelude+`
var rowsByRunner = map[string]types.ExecutedCommand{"go": {Outcome: "syntax_preflight"}}
`, `literal "syntax_preflight"`)
	selfRedExpectViolation(t, "package_level_table_alias", selfRedPrelude+`
var rows = []types.ExecutedCommand{{Outcome: packageLevelAlias}}
`, "identifier packageLevelAlias that is not fed by local data flow")
	selfRedExpectViolation(t, "package_level_nested_struct_table", selfRedPrelude+`
type fixture struct{ Commands []types.ExecutedCommand }
var fixtures = []fixture{{Commands: []types.ExecutedCommand{{Outcome: "cpu_limit"}}}}
`, `literal "cpu_limit"`)

	// Fold-in round six: the shapes the round-five walker silently SKIPPED
	// are closed by the fail-loud default — a package-level FuncLit (IIFE
	// table, closure var) is walked like any other body, and a composite
	// literal that writes an Outcome key under a type the census cannot
	// resolve (generic instantiation, other-package named type) is a
	// violation instead of an invisible producer. Named types and aliases
	// of the command type declared in the analysed package are resolved
	// through the package's type declarations, and package types itself
	// must not export an alias/named form of ExecutedCommand (the
	// cross-package recognizers key on the types.ExecutedCommand spelling).
	selfRedExpectViolation(t, "package_level_iife_table_literal", selfRedPrelude+`
var iifeTable = func() []types.ExecutedCommand {
	return []types.ExecutedCommand{{Outcome: "executed"}}
}()
`, `literal "executed"`)
	selfRedExpectViolation(t, "package_level_closure_var_writes_literal", selfRedPrelude+`
var fillOutcome = func(cmd *types.ExecutedCommand) { cmd.Outcome = "timeout" }
`, `literal "timeout"`)
	selfRedExpectViolation(t, "named_slice_type_table_literal", selfRedPrelude+`
type commandRows []types.ExecutedCommand

var namedTable = commandRows{{Outcome: "oom"}}
`, `literal "oom"`)
	selfRedExpectViolation(t, "alias_type_literal", selfRedPrelude+`
type commandAlias = types.ExecutedCommand

func producer() { _ = commandAlias{Outcome: "parser_error"} }
`, `literal "parser_error"`)
	selfRedExpectViolation(t, "generic_instantiation_literal_is_unresolvable", selfRedPrelude+`
type commandList[T any] []T

func producer() {
	_ = commandList[types.ExecutedCommand]{{Outcome: types.ExecutedCommandOutcomeExecuted}}
}
`, "cannot be resolved by the census")
	selfRedExpectViolation(t, "other_package_named_type_with_outcome_key", selfRedPrelude+`
func producer() { _ = other.Rows{{Outcome: "zero_tests"}} }
`, "cannot be resolved by the census")
	t.Run("types_package_must_not_alias_the_command_type", func(t *testing.T) {
		texts, _ := selfRedCensus(t, `package types

type CmdRow = ExecutedCommand
type CmdRows []ExecutedCommand
`)
		joined := strings.Join(texts, "\n")
		if !strings.Contains(joined, "CmdRow") || !strings.Contains(joined, "CmdRows") ||
			!strings.Contains(joined, "alias/named form of ExecutedCommand") {
			t.Fatalf("violations = %v", texts)
		}
	})

	t.Run("accepted_shapes_stay_green", func(t *testing.T) {
		texts, writers := selfRedCensus(t, selfRedPrelude+`
var packageTable = []types.ExecutedCommand{{Outcome: types.ExecutedCommandOutcomeNotConfigured}}
var packageRow = types.ExecutedCommand{Outcome: types.ExecutedCommandOutcomeSuiteSkipped}

// Fold-in round six recognized shapes: named types and aliases resolve
// through the package's type declarations, IIFE tables are walked, and
// non-command Outcome-bearing literals stay green.
type namedRows []types.ExecutedCommand
type rowAlias = types.ExecutedCommand

var namedConstTable = namedRows{{Outcome: types.ExecutedCommandOutcomeSuiteContinued}}
var aliasRow = rowAlias{Outcome: types.ExecutedCommandOutcomeExecuted}
var iifeConstTable = func() []types.ExecutedCommand {
	return []types.ExecutedCommand{{Outcome: types.ExecutedCommandOutcomeSyntheticNoTests}}
}()
var statusTable = []probeStatus{{Outcome: "passed"}}
var diagTable = []types.VerificationDiagnostic{{Outcome: "not_an_executed_command_row"}}
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
func buildRow() types.ExecutedCommand { return types.ExecutedCommand{Outcome: types.ExecutedCommandOutcomeSyntaxCheckFallback} }
func producer(err error, result probeResult, status probeStatus, captured string) {
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
	outcome := types.ExecutedCommandOutcomeExecuted
	func() {
		outcome = types.ExecutedCommandOutcomeCPULimit
	}()
	_ = types.ExecutedCommand{Outcome: outcome}
	shadow := types.ExecutedCommandOutcomeExecuted
	func(shadow string) { shadow = "not the outer variable"; _ = shadow }("x")
	_ = types.ExecutedCommand{Outcome: shadow}
	built := buildRow()
	built.Outcome = types.ExecutedCommandOutcomeExpectedStdoutMissing
	emit := func() { _ = types.ExecutedCommand{Outcome: captured} }
	emit()
}
func caller() { producer(nil, probeResult{}, probeStatus{}, types.ExecutedCommandOutcomeProbeConfigError) }
`)
		if len(texts) != 0 {
			t.Fatalf("accepted shapes must be green: %v", texts)
		}
		var names []string
		for name := range writers {
			names = append(names, name)
		}
		sort.Strings(names)
		want := []string{"ExecutedCommandOutcomeBaselineUnavailable", "ExecutedCommandOutcomeCPULimit", "ExecutedCommandOutcomeExecuted",
			"ExecutedCommandOutcomeExpectedFailureObserved", "ExecutedCommandOutcomeExpectedStdoutMissing", "ExecutedCommandOutcomeNotConfigured",
			"ExecutedCommandOutcomeOOM", "ExecutedCommandOutcomeParserError", "ExecutedCommandOutcomeProbeConfigError",
			"ExecutedCommandOutcomeRunnerMissing", "ExecutedCommandOutcomeSuiteContinued", "ExecutedCommandOutcomeSuiteSkipped",
			"ExecutedCommandOutcomeSyntaxCheckFallback", "ExecutedCommandOutcomeSyntaxPreflight", "ExecutedCommandOutcomeSyntheticNoTests",
			"ExecutedCommandOutcomeTimeout", "ExecutedCommandOutcomeZeroTests"}
		if strings.Join(names, ",") != strings.Join(want, ",") {
			t.Fatalf("recorded writers = %v, want %v", names, want)
		}
	})
	t.Run("types_package_bare_constant_is_the_constant", func(t *testing.T) {
		texts, writers := selfRedCensus(t, `package types

func producer() ExecutedCommand { return ExecutedCommand{Outcome: ExecutedCommandOutcomeFailed} }
func literal() ExecutedCommand { return ExecutedCommand{Outcome: "failed"} }
`)
		if !writers["ExecutedCommandOutcomeFailed"] || len(texts) != 1 || !strings.Contains(texts[0], `literal "failed"`) {
			t.Fatalf("writers=%v violations=%v", writers, texts)
		}
	})
}
