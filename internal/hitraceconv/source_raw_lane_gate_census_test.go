package hitraceconv

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// source_raw_lane_gate_census_test.go — colleague_merge_audit §40.53 (V6-4)
// structural pins, extended by the batch-six review fold-in (G6-visibility
// #0/#1/#2). Every census here is bound by data flow over every shape it can
// meet and fails loud on a shape it does not recognize (§40.50): a state
// written through an expression the census cannot resolve is a red, never a
// silent skip.
//
//   - the decode_state closed set is total over the gate table and every
//     decode_state write site (assignment, setUnavailable argument or
//     constructor composite literal) refers to a declared constant;
//   - every lane state key of the class (the typed traceDBSourceRawLaneStateKey
//     constants) is gated through the one funnel, the two shared non-ready
//     states are minted only by that funnel, constructors seed only the
//     placeholder and no lane assigns it, and every lane that reads the decode
//     ledger classifies through the gate before it publishes;
//   - the visibility lane's publication_state closed set: every member is
//     minted, every minted value is a member, the rows table and the ordered
//     roster are the same set, and the reader accepts exactly the declared
//     (member, row-count) pairs;
//   - every publication_state value assigned anywhere in the package starts
//     with exactly one prefix of the class roster;
//   - readers of decode_state / publication_state classify through the tables
//     (no hand-kept switch, case, literal comparison, prefix/substring test or
//     lookup through an unseen map — direct reads and reads tainted through
//     local bindings alike; an unrecognized read shape is red), and every
//     table keyed by the decode_state constants is total over the closed set
//     with labels that agree with the gate kind;
//   - every function that writes a lane state key runs or inherits the class
//     gate in the same file for that key (a writer that consumes another
//     lane's coverage inherits; fold-in #7);
//   - both censuses resolve names and keys through the one
//     traceDBKeyResolver over per-declaration scopes (round seven): a key
//     spelled through a local, a constant, a conversion, a caller-resolved
//     parameter or a copy of one is a lane key like a literal, and the two
//     live writes under keys the census cannot resolve are a named,
//     load-bearing roster (traceDBUnresolvedKeyForwardingWrites).

// traceDBPackageStringConsts collects every package-level string constant of
// the non-test files of this package (value = literal, or another collected
// constant, resolved transitively) so a write site can be resolved by name.
func traceDBPackageStringConsts(t *testing.T) (map[string]string, map[string]*ast.File, *token.FileSet) {
	t.Helper()
	files, fset := traceDBParsePackageFiles(t)
	return traceDBStringConstsOf(files), files, fset
}

// traceDBParsePackageFiles parses every non-test file of this package.
func traceDBParsePackageFiles(t *testing.T) (map[string]*ast.File, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = file
	}
	return files, fset
}

// traceDBStringConstsOf resolves the package-level string constants declared
// by the given files (a self-red hands in the real files plus a synthetic
// one parsed with the same FileSet).
func traceDBStringConstsOf(files map[string]*ast.File) map[string]string {
	pending := map[string]ast.Expr{}
	for _, file := range files {
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
				for i, ident := range vs.Names {
					if i < len(vs.Values) {
						pending[ident.Name] = vs.Values[i]
					}
				}
			}
		}
	}
	consts := map[string]string{}
	var resolve func(name string, depth int) (string, bool)
	resolve = func(name string, depth int) (string, bool) {
		if value, ok := consts[name]; ok {
			return value, true
		}
		expr, ok := pending[name]
		if !ok || depth > 8 {
			return "", false
		}
		switch v := expr.(type) {
		case *ast.BasicLit:
			if v.Kind != token.STRING {
				return "", false
			}
			value, err := strconv.Unquote(v.Value)
			if err != nil {
				return "", false
			}
			consts[name] = value
			return value, true
		case *ast.Ident:
			value, ok := resolve(v.Name, depth+1)
			if ok {
				consts[name] = value
			}
			return value, ok
		}
		return "", false
	}
	for name := range pending {
		resolve(name, 0)
	}
	return consts
}

// traceDBTypedStringConsts returns the values of every package-level constant
// declared with the named type (e.g. the lane state keys).
func traceDBTypedStringConsts(t testing.TB, files map[string]*ast.File, consts map[string]string, typeName string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, file := range files {
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
				if ident, ok := vs.Type.(*ast.Ident); !ok || ident.Name != typeName {
					continue
				}
				for _, name := range vs.Names {
					value, ok := consts[name.Name]
					if !ok {
						t.Fatalf("%s constant %s does not resolve to a string", typeName, name.Name)
					}
					out[value] = name.Name
				}
			}
		}
	}
	return out
}

// traceDBMetadataIndexAssignment reports whether stmt assigns
// `<x>.Metadata[<index>] = <expr>` and returns the index and the RHS.
func traceDBMetadataIndexAssignment(stmt *ast.AssignStmt) (ast.Expr, ast.Expr, bool) {
	if len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 || stmt.Tok != token.ASSIGN {
		return nil, nil, false
	}
	index, ok := stmt.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return nil, nil, false
	}
	sel, ok := index.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Metadata" {
		return nil, nil, false
	}
	return index.Index, stmt.Rhs[0], true
}

// traceDBMetadataKeyAssignment reports whether stmt assigns
// `<x>.Metadata["<key>"] = <expr>` and returns the RHS.
func traceDBMetadataKeyAssignment(stmt *ast.AssignStmt, key string) (ast.Expr, bool) {
	index, rhs, ok := traceDBMetadataIndexAssignment(stmt)
	if !ok {
		return nil, false
	}
	lit, ok := index.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return nil, false
	}
	if got, err := strconv.Unquote(lit.Value); err != nil || got != key {
		return nil, false
	}
	return rhs, true
}

func traceDBStringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil
}

// traceDBStateWriteSite is one resolved lane-state write.
type traceDBStateWriteSite struct {
	file  string
	line  int
	key   string
	value string
	// constructor marks a composite-literal (coverage constructor) write; such
	// a write seeds the placeholder and is not a published value.
	constructor bool
	// viaGate marks a value minted by the class gate funnel (directly, or
	// through one of its wrappers) at a call site.
	viaGate bool
	// function is the enclosing top-level function.
	function string
}

// traceDBStripParens unwraps every enclosing parenthesis.
func traceDBStripParens(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}

// traceDBWalk visits node's subtree with the ancestor stack (nearest last).
func traceDBWalk(root ast.Node, visit func(node ast.Node, stack []ast.Node)) {
	var stack []ast.Node
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		visit(node, stack)
		stack = append(stack, node)
		return true
	})
}

// traceDBNearest is the nearest non-paren ancestor on the stack.
func traceDBNearest(stack []ast.Node) ast.Node {
	for i := len(stack) - 1; i >= 0; i-- {
		if _, paren := stack[i].(*ast.ParenExpr); !paren {
			return stack[i]
		}
	}
	return nil
}

// traceDBIsCallFun reports whether node is the function operand of parent.
func traceDBIsCallFun(node ast.Node, parent ast.Node) bool {
	call, ok := parent.(*ast.CallExpr)
	return ok && (call.Fun == node || traceDBStripParens(call.Fun) == node)
}

// traceDBInspectFuncBodies walks every top-level function of file, handing
// each node to visit together with the enclosing declaration — a
// declaration, never a name: same-named methods on different receivers are
// different functions with different scopes (round seven, #6).
func traceDBInspectFuncBodies(file *ast.File, visit func(fn *ast.FuncDecl, node ast.Node)) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			if node != nil {
				visit(fn, node)
			}
			return true
		})
	}
}

// traceDBLaneStateKeyTypeName is the typed lane state key; a parameter of
// this type (or of string) indexing the state map is resolved through every
// caller's argument by both censuses, and the type's constants are the lane
// key closed set of the write census.
const traceDBLaneStateKeyTypeName = "traceDBSourceRawLaneStateKey"

// traceDBIsKeyType: string or the lane key type (a variadic `...string` is
// an Ellipsis, never a key type).
func traceDBIsKeyType(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && (ident.Name == "string" || ident.Name == traceDBLaneStateKeyTypeName)
}

// traceDBStringBindings are the string bindings of one function body:
//
//   - single: the names whose every binding resolves to one value —
//     `x := "lit"`, `x = const`, `var x = "lit"`, `x := string(<constant>)`,
//     a copy of another single-valued name, a package variable never
//     re-bound — resolved through the function's own scope before the
//     package (round seven, #4: a copy of a local shadowing a package
//     constant carries the local's value, or nothing);
//   - once: the names bound exactly once, with the bound expression whatever
//     its shape — the lane a key spelled through a copy of a parameter, a
//     conversion of one, or a concatenation resolves through (round seven,
//     #5).
//
// A name bound through a tuple (`x, ok := f()`, `var x, y = f()`), an
// op-assignment, or more than once to different values is in neither: a
// variable, not a constant, and never the first value it happened to be
// bound to (round six, #3).
type traceDBStringBindings struct {
	single map[string]string
	once   map[string]ast.Expr
}

// traceDBFuncScope is the static scope of one top-level function, keyed by
// declaration (round seven, #6), that both censuses resolve names through:
// the function's own names — parameters of any type, named results, closure
// parameters and results, range names, every name bound in the body —
// precede the package (round six, #5).
type traceDBFuncScope struct {
	fn      *ast.FuncDecl
	file    string
	imports map[string]bool
	// params: string / lane-key typed parameter → position (resolved through
	// every caller's argument).
	params map[string]int
	// declared: every parameter (of any type), named result and closure
	// parameter / result.
	declared map[string]bool
	// assigned: every name bound in the body (a parameter or range name
	// re-bound is unresolved).
	assigned map[string]bool
	// ranged: range key/value ident → number of range statements defining it.
	ranged map[string]int
	// listKeys: range ident over a key-list literal → the literal's resolved
	// elements.
	listKeys map[string][]string
	// keys: range key ident → the state key it is compared against in the
	// body of its (first) range statement; comparedRanges holds the same per
	// statement.
	keys           map[string]string
	comparedRanges map[*ast.RangeStmt]string
	// everyKey: uncompared range key ident over the state map — the
	// `.Metadata` selector statically; the reader census adds the ranges over
	// its tainted map locals and map-returning helper calls.
	everyKey map[string]bool
	bindings traceDBStringBindings
}

// declares reports whether the scope declares name — with or without a
// value the census can spell.
func (scope *traceDBFuncScope) declares(name string) bool {
	return scope.declared[name] || scope.assigned[name] || scope.ranged[name] > 0
}

// traceDBCallSite is one call expression and the scope it sits in.
type traceDBCallSite struct {
	scope *traceDBFuncScope
	call  *ast.CallExpr
}

// traceDBKeyParam names one key parameter of one function.
type traceDBKeyParam struct {
	fn       *ast.FuncDecl
	position int
}

// traceDBBoundName names one once-bound local of one scope.
type traceDBBoundName struct {
	scope *traceDBFuncScope
	name  string
}

// traceDBKeyPath is one resolution's path: the key parameters being resolved
// through their callers (outermost first entered) and the once-bound names
// being resolved through their expression.
type traceDBKeyPath struct {
	params map[traceDBKeyParam]bool
	names  map[traceDBBoundName]bool
}

func newTraceDBKeyPath() *traceDBKeyPath {
	return &traceDBKeyPath{params: map[traceDBKeyParam]bool{}, names: map[traceDBBoundName]bool{}}
}

// onPath reports whether any key parameter of fn is being resolved: a call
// site inside fn is the cycle of the resolution, not a caller of it.
func (path *traceDBKeyPath) onPath(fn *ast.FuncDecl) bool {
	for param := range path.params {
		if param.fn == fn {
			return true
		}
	}
	return false
}

// traceDBKeyResolution is how an index key resolved (round five; one
// resolver for both censuses since round seven):
//
//   - traceDBKeySpelled: written at the site or bound to exactly one value —
//     a literal, a package constant, `string(<constant>)`, a single-valued
//     local (`k := "lit"`, `const k = …`, `k := string(<constant>)`, a copy
//     of one) or a package variable never re-bound.
//   - traceDBKeyCarried: an identifier standing for a set of keys — a range
//     key compared against a state key, the value ranging over a key-list
//     literal, or a string / lane-key typed parameter (or a once-bound copy
//     or conversion of one) resolved through the key argument of every
//     direct caller (a wrapper's forwarded parameter recursively); the set
//     is every key the callers pass, plus every key a call site inside the
//     function's own cycle spells (round seven, #3).
//   - traceDBKeyEveryKey: an uncompared range key of the state map itself:
//     the identifier ranges over every key and carries no key signal (the
//     ruled range reading — a range read is a range value under a key
//     comparison; a range write is a forwarding loop).
//   - traceDBKeyExcluded: a concatenation — spelled at the site or bound once
//     to a local — whose leftmost or rightmost operand resolves to a literal
//     that no state key starts / ends with (`"retention_" + family +
//     "_state"`): whatever the rest is, the key is not a state key.
//   - traceDBKeyForwarded: a key parameter re-entered from inside its own
//     resolution — the cycle's forwarding of the parameter, which adds no
//     key and no caller of its own (round six, #6) — or a nested resolution
//     whose every call site is the cycle, carrying the keys those sites
//     spell (round seven, #3). Surfaces only inside paramKeys' caller loop:
//     a top-level resolution never returns it.
//   - traceDBKeyUnresolved: anything else — a re-bound parameter or range
//     name, a multi-valued local (tuple-bound, re-bound, bound to a value
//     the census cannot resolve, or a copy of one), a parameter of a
//     function that escapes as a value, has no caller outside its own cycle,
//     or has a caller whose argument does not resolve, a closure parameter,
//     a name shadowing a package constant without a single value, a
//     computed expression.
type traceDBKeyResolution int

const (
	traceDBKeyUnresolved traceDBKeyResolution = iota
	traceDBKeySpelled
	traceDBKeyCarried
	traceDBKeyEveryKey
	traceDBKeyExcluded
	traceDBKeyForwarded
)

// traceDBKeyResolver is the one static name and key resolver of both
// censuses (round seven: single source of truth — the reader and the writer
// resolve every key through the same lanes over the same per-declaration
// scopes). It is a pure function of the parsed files.
type traceDBKeyResolver struct {
	consts map[string]string
	// packageVars: package-level string variables with a single-valued
	// initializer never re-bound in any body (round five).
	packageVars map[string]string
	functions   map[string]*ast.FuncDecl
	methods     map[string][]*ast.FuncDecl
	scopes      map[*ast.FuncDecl]*traceDBFuncScope
	// order: every function with a body, by file then position.
	order     []*ast.FuncDecl
	callSites []traceDBCallSite
	// escaped: key-parameter functions used as values (their callers are
	// unknown: the parameter is unresolved).
	escaped map[*ast.FuncDecl]bool
}

func newTraceDBKeyResolver(files map[string]*ast.File, consts map[string]string) *traceDBKeyResolver {
	r := &traceDBKeyResolver{
		consts:      consts,
		packageVars: map[string]string{},
		functions:   map[string]*ast.FuncDecl{},
		methods:     map[string][]*ast.FuncDecl{},
		scopes:      map[*ast.FuncDecl]*traceDBFuncScope{},
		escaped:     map[*ast.FuncDecl]bool{},
	}
	declare := func(scope *traceDBFuncScope, fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, field := range fields.List {
			for _, name := range field.Names {
				scope.declared[name.Name] = true
			}
		}
	}
	for name, file := range files {
		imports := map[string]bool{}
		for _, spec := range file.Imports {
			path := strings.Trim(spec.Path.Value, `"`)
			alias := path[strings.LastIndex(path, "/")+1:]
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			imports[alias] = true
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			scope := &traceDBFuncScope{fn: fn, file: name, imports: imports, params: map[string]int{}, declared: map[string]bool{}, assigned: map[string]bool{},
				ranged: map[string]int{}, listKeys: map[string][]string{}, keys: map[string]string{}, comparedRanges: map[*ast.RangeStmt]string{}, everyKey: map[string]bool{}}
			declare(scope, fn.Type.Params)
			declare(scope, fn.Type.Results)
			position := 0
			for _, field := range fn.Type.Params.List {
				for _, param := range field.Names {
					if traceDBIsKeyType(field.Type) {
						scope.params[param.Name] = position
					}
					position++
				}
			}
			r.scopes[fn] = scope
			r.order = append(r.order, fn)
			if fn.Recv != nil {
				r.methods[fn.Name.Name] = append(r.methods[fn.Name.Name], fn)
			} else {
				r.functions[fn.Name.Name] = fn
			}
		}
	}
	// Package-level string variables: a single-valued initializer (a literal,
	// a constant, `string(<constant>)`) never re-bound in any body resolves
	// like a constant.
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, ident := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					if value, ok := r.spelled(nil, vs.Values[i]); ok {
						r.packageVars[ident.Name] = value
					}
				}
			}
		}
	}
	sort.Slice(r.order, func(i, j int) bool {
		if r.scopes[r.order[i]].file != r.scopes[r.order[j]].file {
			return r.scopes[r.order[i]].file < r.scopes[r.order[j]].file
		}
		return r.order[i].Pos() < r.order[j].Pos()
	})
	// Static pre-pass, stage one: the names each body binds (a package
	// variable re-bound in any body is a variable, not a constant), the
	// closures' own parameters and results (the function's scope, round six),
	// the range statements defining a name, every call site, and the
	// key-parameter functions that escape as values.
	keyParamDecls := func(scope *traceDBFuncScope, expr ast.Expr) []*ast.FuncDecl {
		var decls []*ast.FuncDecl
		switch f := expr.(type) {
		case *ast.Ident:
			if fn, ok := r.functions[f.Name]; ok {
				decls = []*ast.FuncDecl{fn}
			}
		case *ast.SelectorExpr:
			if x, ok := f.X.(*ast.Ident); ok && scope.imports[x.Name] {
				return nil
			}
			decls = r.methods[f.Sel.Name]
		}
		var out []*ast.FuncDecl
		for _, decl := range decls {
			if len(r.scopes[decl].params) > 0 {
				out = append(out, decl)
			}
		}
		return out
	}
	for _, fn := range r.order {
		scope := r.scopes[fn]
		noteBound := func(expr ast.Expr) {
			if ident, ok := expr.(*ast.Ident); ok && ident.Name != "_" {
				scope.assigned[ident.Name] = true
				delete(r.packageVars, ident.Name)
			}
		}
		traceDBWalk(fn.Body, func(node ast.Node, stack []ast.Node) {
			parent := traceDBNearest(stack)
			switch n := node.(type) {
			case *ast.AssignStmt:
				for _, lhs := range n.Lhs {
					noteBound(lhs)
				}
			case *ast.ValueSpec:
				for _, name := range n.Names {
					noteBound(name)
				}
			case *ast.FuncLit:
				declare(scope, n.Type.Params)
				declare(scope, n.Type.Results)
			case *ast.RangeStmt:
				for _, expr := range []ast.Expr{n.Key, n.Value} {
					if ident, ok := expr.(*ast.Ident); ok && ident.Name != "_" {
						scope.ranged[ident.Name]++
					}
				}
			case *ast.CallExpr:
				r.callSites = append(r.callSites, traceDBCallSite{scope: scope, call: n})
			case *ast.Ident:
				if traceDBIdentIsName(n, parent) || traceDBIsCallFun(node, parent) {
					return
				}
				for _, decl := range keyParamDecls(scope, n) {
					r.escaped[decl] = true
				}
			case *ast.SelectorExpr:
				if traceDBIsCallFun(node, parent) {
					return
				}
				for _, decl := range keyParamDecls(scope, n) {
					r.escaped[decl] = true
				}
			}
		})
	}
	// Stage two: the string bindings of every body, spelled through the
	// scope above.
	for _, fn := range r.order {
		r.bind(r.scopes[fn])
	}
	// Stage three: the key-list ranges (elements spelled through the
	// bindings), the compared range keys, and the every-key ranges over the
	// `.Metadata` selector.
	for _, fn := range r.order {
		scope := r.scopes[fn]
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			n, ok := node.(*ast.RangeStmt)
			if !ok {
				return true
			}
			if lit, ok := traceDBStripParens(n.X).(*ast.CompositeLit); ok {
				if _, array := lit.Type.(*ast.ArrayType); !array {
					return true
				}
				var elements []string
				for _, element := range lit.Elts {
					value, ok := r.spelled(scope, element)
					if !ok {
						return true
					}
					elements = append(elements, value)
				}
				if ident, ok := n.Value.(*ast.Ident); ok && ident.Name != "_" {
					scope.listKeys[ident.Name] = elements
				}
				return true
			}
			key, ok := n.Key.(*ast.Ident)
			if !ok || key.Name == "_" {
				return true
			}
			if state, compared := r.rangeKeyComparison(scope, key.Name, n.Body); compared {
				scope.comparedRanges[n] = state
				if scope.keys[key.Name] == "" {
					scope.keys[key.Name] = state
				}
			} else if sel, ok := traceDBStripParens(n.X).(*ast.SelectorExpr); ok && sel.Sel.Name == "Metadata" {
				scope.everyKey[key.Name] = true
			}
			return true
		})
	}
	return r
}

// bind collects scope's string bindings: every `=` / `:=` / var binding of
// a name, a tuple or op-assignment binding it to nothing it can spell. A
// name is single-valued when its every binding spells the same value —
// resolved to a fixpoint, so a copy of a single-valued local is single
// (round seven, #4) — and once-bound when the body binds it exactly once
// to an expression.
func (r *traceDBKeyResolver) bind(scope *traceDBFuncScope) {
	exprs := map[string][]ast.Expr{}
	record := func(name string, expr ast.Expr) {
		if name != "_" {
			exprs[name] = append(exprs[name], expr)
		}
	}
	ast.Inspect(scope.fn.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.AssignStmt:
			tuple := len(n.Lhs) != len(n.Rhs) || (n.Tok != token.ASSIGN && n.Tok != token.DEFINE)
			for i, lhs := range n.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok {
					continue
				}
				if tuple {
					record(ident.Name, nil)
					continue
				}
				record(ident.Name, n.Rhs[i])
			}
		case *ast.ValueSpec:
			tuple := len(n.Values) > 0 && len(n.Names) != len(n.Values)
			for i, name := range n.Names {
				switch {
				case tuple:
					record(name.Name, nil)
				case i < len(n.Values):
					record(name.Name, n.Values[i])
				}
			}
		}
		return true
	})
	scope.bindings = traceDBStringBindings{single: map[string]string{}, once: map[string]ast.Expr{}}
	pending := map[string]bool{}
	for name, list := range exprs {
		if len(list) == 1 && list[0] != nil {
			scope.bindings.once[name] = list[0]
		}
		single := true
		for _, expr := range list {
			single = single && expr != nil
		}
		if single {
			pending[name] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for name := range pending {
			value, resolved, agree := "", true, true
			for i, expr := range exprs[name] {
				spelled, ok := r.spelled(scope, expr)
				if !ok {
					resolved = false
					break
				}
				if i > 0 && spelled != value {
					agree = false
					break
				}
				value = spelled
			}
			switch {
			case !agree:
				delete(pending, name)
			case resolved:
				scope.bindings.single[name] = value
				delete(pending, name)
				changed = true
			}
		}
	}
}

// spelled: the one value a string-valued operand names — a literal, a
// `string(<x>)` / `<keyType>(<x>)` conversion of one, a single-valued local,
// or, for a name the function scope does not declare, a package variable
// never re-bound or a package constant. Function scope precedes the package
// (round six, #5; round seven, #4 through every copy): a name the scope
// declares without a single value never resolves as the constant it
// shadows. A nil scope is the package level: a literal, a constant, a
// conversion of one.
func (r *traceDBKeyResolver) spelled(scope *traceDBFuncScope, expr ast.Expr) (string, bool) {
	switch e := traceDBStripParens(expr).(type) {
	case *ast.BasicLit:
		return traceDBStringLiteral(e)
	case *ast.CallExpr:
		if fun, ok := e.Fun.(*ast.Ident); ok && traceDBIsKeyType(fun) && len(e.Args) == 1 {
			return r.spelled(scope, e.Args[0])
		}
	case *ast.Ident:
		if scope != nil {
			if value, ok := scope.bindings.single[e.Name]; ok {
				return value, true
			}
			if scope.declares(e.Name) {
				return "", false
			}
			if value, ok := r.packageVars[e.Name]; ok {
				return value, true
			}
		}
		value, ok := r.consts[e.Name]
		return value, ok
	}
	return "", false
}

// rangeKeyComparison: the state key a range key identifier is compared
// against inside body — `k == "<key>"` / `k != "<key>"` either way round,
// or a switch over k with a case naming the key; the key spelled through
// the function's scope before the package (round six, #5).
func (r *traceDBKeyResolver) rangeKeyComparison(scope *traceDBFuncScope, key string, body *ast.BlockStmt) (string, bool) {
	isKey := func(expr ast.Expr) bool {
		ident, ok := traceDBStripParens(expr).(*ast.Ident)
		return ok && ident.Name == key
	}
	stateKey := func(expr ast.Expr) (string, bool) {
		k, ok := r.spelled(scope, expr)
		return k, ok && traceDBStateReadKeys[k]
	}
	found, ok := "", false
	ast.Inspect(body, func(node ast.Node) bool {
		if ok {
			return false
		}
		switch n := node.(type) {
		case *ast.BinaryExpr:
			if n.Op != token.EQL && n.Op != token.NEQ {
				return true
			}
			if isKey(n.X) {
				found, ok = stateKey(n.Y)
			} else if isKey(n.Y) {
				found, ok = stateKey(n.X)
			}
		case *ast.SwitchStmt:
			if n.Tag == nil || !isKey(n.Tag) {
				return true
			}
			for _, stmt := range n.Body.List {
				clause, isClause := stmt.(*ast.CaseClause)
				if !isClause {
					continue
				}
				for _, expr := range clause.List {
					if found, ok = stateKey(expr); ok {
						return false
					}
				}
			}
		}
		return true
	})
	return found, ok
}

// concatExcludes: the concatenation's outermost literal operands rule every
// state key out.
func (r *traceDBKeyResolver) concatExcludes(scope *traceDBFuncScope, e *ast.BinaryExpr) bool {
	if e.Op != token.ADD {
		return false
	}
	leftmost, rightmost := ast.Expr(e), ast.Expr(e)
	for {
		b, ok := traceDBStripParens(leftmost).(*ast.BinaryExpr)
		if !ok || b.Op != token.ADD {
			break
		}
		leftmost = b.X
	}
	for {
		b, ok := traceDBStripParens(rightmost).(*ast.BinaryExpr)
		if !ok || b.Op != token.ADD {
			break
		}
		rightmost = b.Y
	}
	excludes := func(operand ast.Expr, matches func(key, literal string) bool) bool {
		literal, ok := r.spelled(scope, operand)
		if !ok || literal == "" {
			return false
		}
		for key := range traceDBStateReadKeys {
			if matches(key, literal) {
				return false
			}
		}
		return true
	}
	return excludes(leftmost, strings.HasPrefix) || excludes(rightmost, strings.HasSuffix)
}

// names reports whether the call names fn (a plain function by identifier,
// a method by selector name; an import-qualified selector never).
func (r *traceDBKeyResolver) names(site traceDBCallSite, fn *ast.FuncDecl) bool {
	switch f := traceDBStripParens(site.call.Fun).(type) {
	case *ast.Ident:
		return fn.Recv == nil && r.functions[f.Name] == fn
	case *ast.SelectorExpr:
		if x, ok := f.X.(*ast.Ident); ok && site.scope.imports[x.Name] {
			return false
		}
		return fn.Recv != nil && f.Sel.Name == fn.Name.Name
	}
	return false
}

// paramKeys: the keys every direct caller passes at the parameter's
// position, plus the keys spelled at the call sites inside the function's
// own cycle (a site inside fn's body, or inside a function on the resolution
// path — round seven, #3: the cycle injects the key it spells; only its
// forwarding of a parameter on the path adds nothing). Unresolved when the
// function escapes as a value, has no caller outside its own cycle, or any
// site's argument does not resolve; forwarded when, resolved from inside its
// cycle, every site is the cycle.
func (r *traceDBKeyResolver) paramKeys(fn *ast.FuncDecl, position int, path *traceDBKeyPath) ([]string, traceDBKeyResolution) {
	if r.escaped[fn] {
		return nil, traceDBKeyUnresolved
	}
	param := traceDBKeyParam{fn: fn, position: position}
	if path.params[param] {
		return nil, traceDBKeyForwarded // re-entered from inside its own cycle
	}
	nested := len(path.params) > 0
	path.params[param] = true
	defer delete(path.params, param)
	var keys []string
	sites, callers := 0, 0
	for _, site := range r.callSites {
		if !r.names(site, fn) {
			continue
		}
		sites++
		if site.call.Ellipsis.IsValid() || position >= len(site.call.Args) {
			return nil, traceDBKeyUnresolved
		}
		passed, resolution := r.resolveKey(site.scope, site.call.Args[position], path)
		switch resolution {
		case traceDBKeyUnresolved, traceDBKeyEveryKey:
			return nil, traceDBKeyUnresolved
		case traceDBKeyForwarded:
			keys = append(keys, passed...) // the cycle's own spelled keys, no caller
			continue
		}
		// keyExcluded passes a key that is not a state key: nothing to add.
		keys = append(keys, passed...)
		if !path.onPath(site.scope.fn) {
			callers++
		}
	}
	switch {
	case callers > 0:
		return keys, traceDBKeyCarried
	case nested && sites > 0:
		return keys, traceDBKeyForwarded
	}
	return nil, traceDBKeyUnresolved
}

// resolveKey: the keys an index expression names and how they resolved.
func (r *traceDBKeyResolver) resolveKey(scope *traceDBFuncScope, expr ast.Expr, path *traceDBKeyPath) ([]string, traceDBKeyResolution) {
	switch e := traceDBStripParens(expr).(type) {
	case *ast.BasicLit:
		if key, ok := traceDBStringLiteral(e); ok {
			return []string{key}, traceDBKeySpelled
		}
	case *ast.CallExpr:
		// A conversion (`string(k)` / `<keyType>(k)`) over a resolvable
		// operand resolves as the operand.
		if fun, ok := e.Fun.(*ast.Ident); ok && traceDBIsKeyType(fun) && len(e.Args) == 1 {
			return r.resolveKey(scope, e.Args[0], path)
		}
	case *ast.BinaryExpr:
		if r.concatExcludes(scope, e) {
			return nil, traceDBKeyExcluded
		}
	case *ast.Ident:
		// Function scope first (round six, #5): the key-parameter, compared
		// range key, key-list range, single-valued local and once-bound local
		// lanes, then any other name the scope declares (unresolved), and
		// only for a name the scope does not declare the package variable or
		// constant it names.
		name := e.Name
		if name == "_" {
			return nil, traceDBKeyUnresolved
		}
		if position, isParam := scope.params[name]; isParam {
			if scope.assigned[name] || scope.ranged[name] > 0 {
				return nil, traceDBKeyUnresolved
			}
			return r.paramKeys(scope.fn, position, path)
		}
		if key := scope.keys[name]; key != "" {
			return []string{key}, traceDBKeyCarried
		}
		if scope.ranged[name] > 0 {
			if scope.assigned[name] || scope.ranged[name] > 1 {
				return nil, traceDBKeyUnresolved
			}
			if elements, ok := scope.listKeys[name]; ok {
				return elements, traceDBKeyCarried
			}
			if scope.everyKey[name] {
				return nil, traceDBKeyEveryKey
			}
			return nil, traceDBKeyUnresolved
		}
		if value, ok := scope.bindings.single[name]; ok {
			return []string{value}, traceDBKeySpelled
		}
		if bound, ok := scope.bindings.once[name]; ok {
			// Bound once to an expression the bindings cannot spell: a copy
			// or conversion of a parameter, a concatenation — resolved as
			// that expression (round seven, #5).
			local := traceDBBoundName{scope: scope, name: name}
			if path.names[local] {
				return nil, traceDBKeyUnresolved
			}
			path.names[local] = true
			defer delete(path.names, local)
			return r.resolveKey(scope, bound, path)
		}
		if scope.declares(name) {
			return nil, traceDBKeyUnresolved
		}
		if value, ok := r.packageVars[name]; ok {
			return []string{value}, traceDBKeySpelled
		}
		if value, ok := r.consts[name]; ok {
			return []string{value}, traceDBKeySpelled
		}
	}
	return nil, traceDBKeyUnresolved
}

// traceDBRecordingTB collects the fail-loud reports of a census that speaks
// through Errorf, so a self-red can pin the exact report; everything else
// reaches the real test.
type traceDBRecordingTB struct {
	testing.TB
	problems []string
}

func (r *traceDBRecordingTB) Errorf(format string, args ...interface{}) {
	r.problems = append(r.problems, fmt.Sprintf(format, args...))
}

// The class gate write funnel and the wrappers that forward a lane key into
// it. The value is the index of the key argument; -1 means the wrapper fixes
// the key to publication_state.
const traceDBSourceRawGateFunnelName = "traceDBMintSourceRawLaneGateOutcome"

var traceDBSourceRawGateCallers = map[string]int{
	traceDBSourceRawGateFunnelName:       1,
	"traceDBApplySourceRawDecodeGate":    2,
	"traceDBApplySourceRawLaneGateKeyed": 2,
	"traceDBInheritSourceRawLaneGate":    3,
	"traceDBApplySourceRawLaneGate":      -1,
}

// traceDBForwardingWrite names one Metadata write by file, enclosing
// function and the key expression as written.
type traceDBForwardingWrite struct {
	file, function, key string
}

// traceDBUnresolvedKeyForwardingWrites is the disclosed residual of the
// write census (round seven, #5): the live writes under a Metadata key the
// census cannot resolve, tolerated by name. Each forwards the rows of
// another collection into the coverage's own map — the device-info fields
// projected from a struct-list literal (`field.name`), the parser metadata
// rows read from the trace database (`name`) — so the keys are the rows',
// not a lane key the census could place. Every other unresolvable key over
// the state map is red whatever its value, and a roster entry no live write
// matches is red (TestTraceDBUnresolvedKeyForwardingRosterIsLoadBearing).
var traceDBUnresolvedKeyForwardingWrites = map[traceDBForwardingWrite]bool{
	{file: "streamerdb_metadata.go", function: "inspectTraceDBDeviceInfoMetadata", key: "field.name"}: true,
	{file: "streamerdb_metadata.go", function: "inspectTraceDBParserMetadata", key: "name"}:           true,
}

// traceDBPublicationStatePrefixMatches counts the class roster prefixes a
// publication_state value starts with; a minted value matches exactly one.
func traceDBPublicationStatePrefixMatches(value string) int {
	matches := 0
	for _, entry := range traceDBSourceRawPublicationStatePrefixes {
		if strings.HasPrefix(value, entry.Prefix) {
			matches++
		}
	}
	return matches
}

// traceDBLaneStateWriteSites walks every non-test file and returns every
// write of a lane state key (the typed traceDBSourceRawLaneStateKey constants):
//
//   - `<x>.Metadata[<key>] = <expr>` assignments — the key resolved through
//     the reader census's lanes (spelled at the site, through a local, a
//     constant or a conversion; carried through every caller of a key
//     parameter; a compared range key; a key-list range), the value
//     through package constants and single-binding locals (round seven,
//     #5: a resolved lane key is a site like a literal one, whatever the
//     value);
//   - `"<key>": <expr>` composite-literal entries (coverage constructors);
//   - calls of the visibility lane's typed setter;
//   - calls of the gate funnel or one of its wrappers, expanded to the values
//     the funnel body mints under the key argument (resolved through the
//     same lanes);
//   - the marker-async ledger's `ledger.state = <expr>` writes, published
//     under raw_async_replacement_state by applyCoverage.
//
// The funnel body (`out.Metadata[stateKey] = <const>`, minted at every gate
// call site instead), the visibility setter body, applyCoverage's forwarding
// of ledger.state and the ledger.state forwarding of the gate coverage are
// the recognized non-constant writes; every other unresolvable RHS under a
// lane key, every gate call whose key argument does not resolve to a
// declared key, every state-shaped value written under a computed Metadata
// key that is not a lane key, and every write under a computed key the
// census cannot resolve — whatever the value, unless the write is on the
// disclosed roster traceDBUnresolvedKeyForwardingWrites — is reported as a
// failure (fail-loud on unrecognized shapes). A computed key that resolves
// to no lane key (a spelled or carried non-key, an every-key forwarding
// loop, a concatenation whose literal prefix / suffix excludes every lane
// key) is not a lane write: the key is the precise signal, not the value.
func traceDBLaneStateWriteSites(t *testing.T) ([]traceDBStateWriteSite, map[string]string) {
	t.Helper()
	consts, files, fset := traceDBPackageStringConsts(t)
	return traceDBLaneStateWriteSitesOf(t, consts, files, fset)
}

// traceDBLaneStateWriteSitesOf is the write-site census over an explicit
// file map (the package's non-test files, plus a synthetic file in a
// self-red; a traceDBRecordingTB pins its fail-loud reports).
func traceDBLaneStateWriteSitesOf(t testing.TB, consts map[string]string, files map[string]*ast.File, fset *token.FileSet) ([]traceDBStateWriteSite, map[string]string) {
	t.Helper()
	laneKeys := traceDBTypedStringConsts(t, files, consts, traceDBLaneStateKeyTypeName)
	if len(laneKeys) < 7 {
		t.Fatalf("lane state key closed set lost members: %v", laneKeys)
	}
	stateShaped := func(value string) bool {
		return value == traceDBSourceRawLanePlaceholderState || traceDBPublicationStatePrefixMatches(value) > 0
	}
	resolver := newTraceDBKeyResolver(files, consts)
	// Pass one: the funnel's minted values.
	funnelValues := map[string]bool{}
	gateFile := files["source_raw_lane_gate.go"]
	if gateFile == nil {
		t.Fatal("source_raw_lane_gate.go not parsed")
	}
	traceDBInspectFuncBodies(gateFile, func(fn *ast.FuncDecl, node ast.Node) {
		stmt, ok := node.(*ast.AssignStmt)
		if !ok || fn.Name.Name != traceDBSourceRawGateFunnelName {
			return
		}
		index, rhs, ok := traceDBMetadataIndexAssignment(stmt)
		if !ok {
			return
		}
		if ident, ok := index.(*ast.Ident); ok && ident.Name == "stateKey" {
			value, ok := resolver.spelled(resolver.scopes[fn], rhs)
			if !ok {
				t.Errorf("source_raw_lane_gate.go:%d: gate funnel mints an unresolvable value", fset.Position(stmt.Pos()).Line)
				return
			}
			funnelValues[value] = true
		}
	})
	if len(funnelValues) == 0 {
		t.Fatal("gate funnel mints no value")
	}
	// laneKeysOf: the distinct declared lane keys a resolved key set names.
	laneKeysOf := func(keys []string) []string {
		seen := map[string]bool{}
		var out []string
		for _, key := range keys {
			if _, lane := laneKeys[key]; lane && !seen[key] {
				seen[key] = true
				out = append(out, key)
			}
		}
		sort.Strings(out)
		return out
	}
	var sites []traceDBStateWriteSite
	setterFunnels, asyncFunnels := 0, 0
	forwarded := map[traceDBForwardingWrite]bool{}
	for name, file := range files {
		traceDBInspectFuncBodies(file, func(fn *ast.FuncDecl, node ast.Node) {
			funcName := fn.Name.Name
			scope := resolver.scopes[fn]
			line := func(n ast.Node) int { return fset.Position(n.Pos()).Line }
			resolve := func(expr ast.Expr) (string, bool) { return resolver.spelled(scope, expr) }
			switch n := node.(type) {
			case *ast.AssignStmt:
				if len(n.Lhs) == 1 && len(n.Rhs) == 1 && n.Tok == token.ASSIGN {
					// The marker-async ledger's own state field.
					if sel, ok := n.Lhs[0].(*ast.SelectorExpr); ok && sel.Sel.Name == "state" &&
						name == "source_raw_marker_async_recovery.go" {
						if index, ok := n.Rhs[0].(*ast.IndexExpr); ok {
							if inner, ok := index.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "Metadata" {
								asyncFunnels++ // forwarding of the gate coverage
								return
							}
						}
						value, ok := resolve(n.Rhs[0])
						if !ok {
							t.Errorf("%s:%d: ledger.state written through an unrecognized expression shape", name, line(n))
							return
						}
						sites = append(sites, traceDBStateWriteSite{file: name, line: line(n), key: string(traceDBSourceRawLaneStateKeyAsyncReplacement), value: value, function: funcName})
						return
					}
				}
				index, rhs, ok := traceDBMetadataIndexAssignment(n)
				if !ok {
					return
				}
				// The funnel's own write: its values are minted at every gate
				// call site.
				if ident, ok := index.(*ast.Ident); ok && ident.Name == "stateKey" && funcName == traceDBSourceRawGateFunnelName {
					return
				}
				_, literal := traceDBStringLiteral(index)
				keys, resolution := resolver.resolveKey(scope, index, newTraceDBKeyPath())
				var lanes []string
				if resolution == traceDBKeySpelled || resolution == traceDBKeyCarried {
					lanes = laneKeysOf(keys)
				}
				for _, key := range lanes {
					if funcName == "traceDBSetSourceRawVisibilityState" && key == string(traceDBSourceRawLaneStateKeyPublication) {
						setterFunnels++
						continue
					}
					if funcName == "applyCoverage" && key == string(traceDBSourceRawLaneStateKeyAsyncReplacement) {
						if sel, ok := rhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "state" {
							asyncFunnels++
							continue
						}
					}
					value, ok := resolve(rhs)
					if !ok {
						t.Errorf("%s:%d: %s written through an unrecognized expression shape", name, line(n), key)
						continue
					}
					sites = append(sites, traceDBStateWriteSite{file: name, line: line(n), key: key, value: value, function: funcName})
				}
				if len(lanes) > 0 || literal {
					return
				}
				// A computed key that names no lane key: a state-shaped value
				// under it is an evasion of the census; a key the census cannot
				// resolve is a write it cannot place, whatever the value (round
				// seven, #5), unless disclosed on the roster; a spelled or
				// carried non-key, an every-key forwarding loop and an excluded
				// concatenation are not lane writes.
				value, ok := resolve(rhs)
				if ok && stateShaped(value) {
					t.Errorf("%s:%d: state-shaped value %q written under a computed Metadata key", name, line(n), value)
					return
				}
				if resolution != traceDBKeyUnresolved {
					return
				}
				entry := traceDBForwardingWrite{file: name, function: funcName, key: exprText(index)}
				if traceDBUnresolvedKeyForwardingWrites[entry] {
					forwarded[entry] = true
					return
				}
				written := "a value the census cannot resolve"
				if ok {
					written = fmt.Sprintf("%q", value)
				}
				t.Errorf("%s:%d: %s written under a Metadata key the census cannot resolve (%s)", name, line(n), written, entry.key)
			case *ast.KeyValueExpr:
				key, ok := traceDBStringLiteral(n.Key)
				if !ok {
					return
				}
				if _, lane := laneKeys[key]; !lane {
					return
				}
				value, ok := resolve(n.Value)
				if !ok {
					t.Errorf("%s:%d: constructor seeds %s through an unrecognized expression shape", name, line(n), key)
					return
				}
				sites = append(sites, traceDBStateWriteSite{file: name, line: line(n), key: key, value: value, constructor: true, function: funcName})
			case *ast.CallExpr:
				fun, ok := n.Fun.(*ast.Ident)
				if !ok {
					return
				}
				if fun.Name == "traceDBSetSourceRawVisibilityState" {
					if len(n.Args) != 2 {
						t.Errorf("%s:%d: visibility setter arity drifted", name, line(n))
						return
					}
					value, ok := resolve(n.Args[1])
					if !ok {
						t.Errorf("%s:%d: visibility state set through an unrecognized expression shape", name, line(n))
						return
					}
					sites = append(sites, traceDBStateWriteSite{file: name, line: line(n), key: string(traceDBSourceRawLaneStateKeyPublication), value: value, function: funcName})
					return
				}
				keyArg, gate := traceDBSourceRawGateCallers[fun.Name]
				if !gate {
					return
				}
				keys := []string{string(traceDBSourceRawLaneStateKeyPublication)}
				if keyArg >= 0 {
					if len(n.Args) <= keyArg {
						t.Errorf("%s:%d: gate call %s arity drifted", name, line(n), fun.Name)
						return
					}
					if ident, ok := n.Args[keyArg].(*ast.Ident); ok {
						_, isParam := scope.params[ident.Name]
						_, wrapper := traceDBSourceRawGateCallers[funcName]
						if isParam && wrapper {
							return // the wrapper forwards its own key parameter; its callers mint
						}
					}
					resolved, resolution := resolver.resolveKey(scope, n.Args[keyArg], newTraceDBKeyPath())
					if resolution != traceDBKeySpelled && resolution != traceDBKeyCarried {
						t.Errorf("%s:%d: gate call %s key argument does not resolve to a declared lane key", name, line(n), fun.Name)
						return
					}
					keys = resolved
				}
				for _, key := range keys {
					if _, lane := laneKeys[key]; !lane {
						t.Errorf("%s:%d: gate call %s uses %q, which is not a declared lane key", name, line(n), fun.Name, key)
					}
				}
				for _, key := range laneKeysOf(keys) {
					for value := range funnelValues {
						sites = append(sites, traceDBStateWriteSite{file: name, line: line(n), key: key, value: value, viaGate: true, function: funcName})
					}
				}
			}
		})
	}
	for entry := range traceDBUnresolvedKeyForwardingWrites {
		if !forwarded[entry] {
			t.Errorf("%s: the forwarding-write roster names %s[%s], which no live write matches", entry.file, entry.function, entry.key)
		}
	}
	if setterFunnels != 1 {
		t.Fatalf("visibility state setter funnel count = %d, want exactly one forwarding write", setterFunnels)
	}
	if asyncFunnels != 2 {
		t.Fatalf("marker-async state forwarding count = %d, want the gate forwarding and the applyCoverage publication", asyncFunnels)
	}
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].file != sites[j].file {
			return sites[i].file < sites[j].file
		}
		if sites[i].line != sites[j].line {
			return sites[i].line < sites[j].line
		}
		if sites[i].key != sites[j].key {
			return sites[i].key < sites[j].key
		}
		return sites[i].value < sites[j].value
	})
	return sites, laneKeys
}

// traceDBPublicationStateWriteSites returns the publication_state subset of
// the lane-state census (constructor placeholders excluded).
func traceDBPublicationStateWriteSites(t *testing.T) []traceDBStateWriteSite {
	t.Helper()
	all, _ := traceDBLaneStateWriteSites(t)
	var sites []traceDBStateWriteSite
	for _, site := range all {
		if site.key == string(traceDBSourceRawLaneStateKeyPublication) && !site.constructor {
			sites = append(sites, site)
		}
	}
	return sites
}

func TestTraceDBRawDecodeStateGatesAreTotalOverDeclaredStates(t *testing.T) {
	consts, files, fset := traceDBPackageStringConsts(t)
	declared := map[string]string{}
	for name, value := range consts {
		if strings.HasPrefix(name, "traceDBRawDecodeState") {
			declared[value] = name
		}
	}
	if len(declared) < 8 {
		t.Fatalf("decode_state closed set lost members: %v", declared)
	}
	for value, name := range declared {
		if _, ok := traceDBRawDecodeStateGates[value]; !ok {
			t.Errorf("declared decode_state %s=%q has no gate kind", name, value)
		}
	}
	for value := range traceDBRawDecodeStateGates {
		if _, ok := declared[value]; !ok {
			t.Errorf("gate table names %q, which is not a declared decode_state constant", value)
		}
	}
	// Every decode_state write site (setUnavailable's first argument, a
	// direct Metadata["decode_state"] assignment, or the constructor's
	// composite-literal entry) refers to a declared constant; setUnavailable's
	// own body is the one recognized funnel that forwards its parameter.
	sites, funnels, constructors := 0, 0, 0
	for name, file := range files {
		traceDBInspectFuncBodies(file, func(fn *ast.FuncDecl, node ast.Node) {
			funcName := fn.Name.Name
			var expr ast.Expr
			switch n := node.(type) {
			case *ast.AssignStmt:
				rhs, ok := traceDBMetadataKeyAssignment(n, "decode_state")
				if !ok {
					return
				}
				if funcName == "setUnavailable" {
					funnels++
					return
				}
				expr = rhs
			case *ast.KeyValueExpr:
				if key, ok := traceDBStringLiteral(n.Key); !ok || key != "decode_state" {
					return
				}
				constructors++
				expr = n.Value
			case *ast.CallExpr:
				sel, ok := n.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "setUnavailable" || len(n.Args) != 3 {
					return
				}
				expr = n.Args[0]
			default:
				return
			}
			sites++
			line := fset.Position(expr.Pos()).Line
			ident, ok := expr.(*ast.Ident)
			if !ok {
				t.Errorf("%s:%d: decode_state written through a non-constant expression", name, line)
				return
			}
			if _, ok := declared[consts[ident.Name]]; !ok || !strings.HasPrefix(ident.Name, "traceDBRawDecodeState") {
				t.Errorf("%s:%d: decode_state written from %s, which is not a declared decode_state constant", name, line, ident.Name)
			}
		})
	}
	// The probe's two setUnavailable calls, finalize's three calls and its
	// three direct assignments, plus the coverage constructor's seed.
	if sites < 9 || funnels != 1 || constructors != 1 {
		t.Fatalf("decode_state write census: sites=%d funnels=%d constructors=%d", sites, funnels, constructors)
	}
}

func TestTraceDBSourceRawLaneGateKindPerDeclaredState(t *testing.T) {
	for state, kind := range traceDBRawDecodeStateGates {
		found := kind != traceDBSourceRawGateNotApplicable
		inventory := newTraceDBSourceNameInventory()
		inventory.RawDecode.Found = found
		inventory.RawDecode.Metadata["decode_state"] = state
		if got, reason := traceDBSourceRawLaneGate(&inventory); got != kind || reason != state {
			t.Errorf("%s: gate=%d reason=%q, want gate=%d reason=%q", state, got, reason, kind, state)
		}
		if got, reason := traceDBSourceRawDecodeGate(&inventory.RawDecode); got != kind || reason != state {
			t.Errorf("%s: decode gate=%d reason=%q, want gate=%d reason=%q", state, got, reason, kind, state)
		}
		inventory.RawDecode.Found = !found
		if got, _ := traceDBSourceRawLaneGate(&inventory); got != traceDBSourceRawGateUnset {
			t.Errorf("%s with contradicting Found=%t resolved to %d instead of Unset", state, !found, got)
		}
	}
	if kind, reason := traceDBSourceRawLaneGate(nil); kind != traceDBSourceRawGateNotApplicable || reason != "" {
		t.Fatalf("absent inventory: gate=%d reason=%q", kind, reason)
	}
	if kind, reason := traceDBSourceRawDecodeGate(nil); kind != traceDBSourceRawGateNotApplicable || reason != "" {
		t.Fatalf("absent ledger: gate=%d reason=%q", kind, reason)
	}
}

// TestTraceDBSourceRawLaneKeysAreGatedThroughTheOneFunnel (G6-visibility
// #0/#1): every declared lane state key has a gate call site; the two shared
// non-ready states are minted only by the funnel; the funnel mints exactly
// those two; constructors seed only the placeholder and no lane assigns it;
// and every function that both reads the strict decode ledger and publishes
// a lane state runs the gate before the read.
func TestTraceDBSourceRawLaneKeysAreGatedThroughTheOneFunnel(t *testing.T) {
	sites, laneKeys := traceDBLaneStateWriteSites(t)
	gatedKeys := map[string]int{}
	funnelValues := map[string]bool{}
	for _, site := range sites {
		if site.viaGate {
			gatedKeys[site.key]++
			funnelValues[site.value] = true
			continue
		}
		switch site.value {
		case traceDBSourceRawLaneNotApplicableState, traceDBSourceRawLaneCensusIncompleteState:
			t.Errorf("%s:%d: %s mints shared gate state %q outside the gate funnel", site.file, site.line, site.key, site.value)
		case traceDBSourceRawLanePlaceholderState:
			if !site.constructor {
				t.Errorf("%s:%d: %s assigns the placeholder %q; only constructors seed it", site.file, site.line, site.key, site.value)
			}
			continue
		}
		if site.constructor {
			t.Errorf("%s:%d: constructor seeds %s with %q instead of the placeholder", site.file, site.line, site.key, site.value)
		}
	}
	for value, name := range laneKeys {
		if gatedKeys[value] == 0 {
			t.Errorf("lane state key %s=%q has no gate call site", name, value)
		}
	}
	// Every writer runs or inherits the gate (fold-in #7): a per-key gate
	// call site anywhere in the package let an eighth writer of
	// reconciliation_state publish a complete_ closure over a join that had
	// said not-applicable / census-incomplete.
	for _, problem := range traceDBUngatedLaneKeyWriters(sites) {
		t.Error(problem)
	}
	want := map[string]bool{traceDBSourceRawLaneNotApplicableState: true, traceDBSourceRawLaneCensusIncompleteState: true}
	for value := range funnelValues {
		if !want[value] {
			t.Errorf("gate funnel mints %q, which is not one of the two shared non-ready states", value)
		}
	}
	for value := range want {
		if !funnelValues[value] {
			t.Errorf("gate funnel no longer mints %q", value)
		}
	}
	// Gate-before-read: bound by data flow over every function of the package
	// that publishes a lane state (a lane-key write site) AND reads the strict
	// decode ledger through traceDBRawDecodeFamilyComplete /
	// traceDBRawDecodeCensusComplete.
	_, files, fset := traceDBPackageStringConsts(t)
	writers := map[string]map[string]bool{} // file → function
	for _, site := range sites {
		if site.constructor {
			continue
		}
		if writers[site.file] == nil {
			writers[site.file] = map[string]bool{}
		}
		writers[site.file][site.function] = true
	}
	gatedReaders := 0
	for name, file := range files {
		if name == "source_raw_lane_gate.go" {
			continue
		}
		firstGate := map[string]token.Pos{}
		firstRead := map[string]token.Pos{}
		traceDBInspectFuncBodies(file, func(fn *ast.FuncDecl, node ast.Node) {
			funcName := fn.Name.Name
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return
			}
			fun, ok := call.Fun.(*ast.Ident)
			if !ok {
				return
			}
			switch fun.Name {
			case "traceDBRawDecodeFamilyComplete", "traceDBRawDecodeCensusComplete":
				if prior, seen := firstRead[funcName]; !seen || call.Pos() < prior {
					firstRead[funcName] = call.Pos()
				}
			default:
				if _, gate := traceDBSourceRawGateCallers[fun.Name]; gate {
					if prior, seen := firstGate[funcName]; !seen || call.Pos() < prior {
						firstGate[funcName] = call.Pos()
					}
				}
			}
		})
		for funcName, read := range firstRead {
			if !writers[name][funcName] {
				continue
			}
			gate, gated := firstGate[funcName]
			if !gated {
				t.Errorf("%s: %s publishes a lane state and reads the decode ledger without the class gate", name, funcName)
				continue
			}
			if gate > read {
				t.Errorf("%s:%d: %s reads the decode ledger before the class gate", name, fset.Position(read).Line, funcName)
				continue
			}
			gatedReaders++
		}
	}
	// The retained-family lanes (DMA wait/lifecycle, marker sync, block,
	// exact) and the keyed lanes (switch/wakeup joins, CPU fallback, wakeup
	// name, blocked key, marker async).
	if gatedReaders < 11 {
		t.Fatalf("gate-before-read census found only %d gated ledger readers", gatedReaders)
	}
}

func TestTraceDBSourceRawVisibilityStatesAreTheDeclaredClosedSet(t *testing.T) {
	consts, files, _ := traceDBPackageStringConsts(t)
	declared := traceDBTypedStringConsts(t, files, consts, "traceDBSourceRawVisibilityState")
	if len(declared) != 5 {
		t.Fatalf("visibility closed set must have exactly five members, declared %v", declared)
	}
	roster := map[string]bool{}
	for _, state := range allTraceDBSourceRawVisibilityStates() {
		if roster[string(state)] {
			t.Fatalf("allTraceDBSourceRawVisibilityStates repeats %q", state)
		}
		roster[string(state)] = true
	}
	for value := range declared {
		if !roster[value] {
			t.Errorf("declared state %q is missing from allTraceDBSourceRawVisibilityStates", value)
		}
		if _, ok := traceDBSourceRawVisibilityStateRows[traceDBSourceRawVisibilityState(value)]; !ok {
			t.Errorf("declared state %q has no row-count entry", value)
		}
	}
	for value := range roster {
		if _, ok := declared[value]; !ok {
			t.Errorf("allTraceDBSourceRawVisibilityStates names %q, which is not a declared constant", value)
		}
	}
	for state := range traceDBSourceRawVisibilityStateRows {
		if _, ok := declared[string(state)]; !ok {
			t.Errorf("row table names %q, which is not a declared constant", state)
		}
	}
	// Minted values: the visibility file's typed setter calls plus the two
	// gate outcomes minted for the visibility lane by its gate call.
	minted := map[string]bool{}
	for _, site := range traceDBPublicationStateWriteSites(t) {
		if site.file == "source_raw_visibility_recovery.go" {
			minted[site.value] = true
		}
	}
	for value := range declared {
		if !minted[value] {
			t.Errorf("declared state %q is never minted", value)
		}
	}
	for value := range minted {
		if _, ok := declared[value]; !ok {
			t.Errorf("visibility lane mints %q, which is not a declared member", value)
		}
	}
}

func TestTraceDBSourceRawVisibilityPublishedRowsAcceptsEveryZeroRowState(t *testing.T) {
	coverageWith := func(state string, rows int) TraceDBCoverage {
		out := newTraceDBSourceRawVisibilityCoverage()
		out.Metadata["publication_state"] = state
		out.RowsEmitted = rows
		return out
	}
	for _, state := range allTraceDBSourceRawVisibilityStates() {
		zeroRow := traceDBSourceRawVisibilityStateRows[state]
		right, wrong := 0, 3
		if !zeroRow {
			right, wrong = 3, 0
		}
		if rows, err := traceDBSourceRawVisibilityPublishedRows([]TraceDBCoverage{coverageWith(string(state), right)}); err != nil || rows != right {
			t.Errorf("%s with %d rows rejected: rows=%d err=%v", state, right, rows, err)
		}
		if _, err := traceDBSourceRawVisibilityPublishedRows([]TraceDBCoverage{coverageWith(string(state), wrong)}); err == nil {
			t.Errorf("%s with %d rows accepted", state, wrong)
		}
	}
	for _, state := range []string{"", "unavailable", "withheld_raw_decode_incomplete", "published_partial"} {
		for _, rows := range []int{0, 3} {
			_, err := traceDBSourceRawVisibilityPublishedRows([]TraceDBCoverage{coverageWith(state, rows)})
			var invariant *traceDBOutputInvariantError
			if err == nil || !asTraceDBOutputInvariant(err, &invariant) || invariant.Reason != "source_raw_visibility_coverage_invalid" {
				t.Errorf("unknown state %q with %d rows was not rejected: %v", state, rows, err)
			}
		}
	}
}

func asTraceDBOutputInvariant(err error, target **traceDBOutputInvariantError) bool {
	invariant, ok := err.(*traceDBOutputInvariantError)
	if ok {
		*target = invariant
	}
	return ok
}

func TestTraceDBSourceRawLanePublicationStatePrefixRoster(t *testing.T) {
	sites := traceDBPublicationStateWriteSites(t)
	if len(sites) < 40 {
		t.Fatalf("publication_state write census found only %d sites", len(sites))
	}
	files := map[string]bool{}
	for _, site := range sites {
		files[site.file] = true
		if matches := traceDBPublicationStatePrefixMatches(site.value); matches != 1 {
			t.Errorf("%s:%d: publication_state %q matches %d roster prefixes, want exactly one", site.file, site.line, site.value, matches)
		}
	}
	for _, want := range []string{
		"source_raw_block_recovery.go", "source_raw_blocked_recovery.go", "source_raw_dma_lifecycle_recovery.go",
		"source_raw_dma_wait_recovery.go", "source_raw_exact_recovery.go", "source_raw_marker_sync_recovery.go",
		"source_raw_visibility_recovery.go", "source_raw_lane_gate.go", "streamerdb_export_blocked.go",
	} {
		if !files[want] {
			t.Errorf("no publication_state write site found in %s", want)
		}
	}
	// The roster's own prefixes are distinct and each carries one verdict.
	seen := map[string]bool{}
	for _, entry := range traceDBSourceRawPublicationStatePrefixes {
		if seen[entry.Prefix] || !strings.HasSuffix(entry.Prefix, "_") {
			t.Errorf("roster prefix %q repeated or malformed", entry.Prefix)
		}
		seen[entry.Prefix] = true
	}
	for _, blocked := range []string{"not_applicable_", "census_incomplete_", "withheld_"} {
		if !traceDBSourceRawPublicationStateBlocksEvaluation(blocked + "x") {
			t.Errorf("%s states must block evaluation", blocked)
		}
	}
	for _, evaluates := range []string{"complete_", "published_", "submitted_"} {
		if traceDBSourceRawPublicationStateBlocksEvaluation(evaluates + "x") {
			t.Errorf("%s states must allow evaluation", evaluates)
		}
	}
}

// TestTraceDBRawDecodeStateReadersClassifyThroughTheTables (G6-visibility
// #2): no reader hand-keeps a switch or a literal comparison over
// decode_state / publication_state; every map keyed by the decode_state
// constants is total over the closed set (missing arms red, undeclared arms
// red); and the decoder-authority labels agree with the gate kind of the
// state they describe (a census-incomplete state is never presented as
// ready, a not-applicable state is never presented as withheld).
func TestTraceDBRawDecodeStateReadersClassifyThroughTheTables(t *testing.T) {
	consts, files, fset := traceDBPackageStringConsts(t)
	declared := map[string]string{}
	for name, value := range consts {
		if strings.HasPrefix(name, "traceDBRawDecodeState") {
			declared[value] = name
		}
	}
	// Reader census (fold-in #9): every read of decode_state /
	// publication_state — direct, or tainted through local bindings — lands in
	// a recognized consumer position; the live reader floor keeps a silently
	// empty walk red. Since round five (#7) the count is genuine reads only —
	// a plain write target `<x>.Metadata[<key>] = …` is not a read — 16 on
	// the live tree; the floor is a floor, not the number.
	problems, reads := traceDBStateReadProblems(files, fset, consts)
	for _, problem := range problems {
		t.Error(problem)
	}
	if reads < 5 {
		t.Fatalf("state reader census found only %d genuine reads; the walk drifted", reads)
	}
	tables := 0
	for name, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.CompositeLit:
				keyed := map[string]bool{}
				stateKeyed := false
				for _, element := range n.Elts {
					kv, ok := element.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					ident, ok := kv.Key.(*ast.Ident)
					if !ok || !strings.HasPrefix(ident.Name, "traceDBRawDecodeState") {
						continue
					}
					stateKeyed = true
					keyed[consts[ident.Name]] = true
				}
				if !stateKeyed {
					return true
				}
				tables++
				for value, constName := range declared {
					if !keyed[value] {
						t.Errorf("%s:%d: table keyed by decode_state has no arm for %s", name, fset.Position(n.Pos()).Line, constName)
					}
				}
				for value := range keyed {
					if _, ok := declared[value]; !ok {
						t.Errorf("%s:%d: table keyed by decode_state names undeclared %q", name, fset.Position(n.Pos()).Line, value)
					}
				}
			}
			return true
		})
	}
	// The gate table and the decoder-authority table.
	if tables < 2 {
		t.Fatalf("decode_state keyed tables found: %d", tables)
	}
	labels := map[string]bool{}
	for state, kind := range traceDBRawDecodeStateGates {
		label, ok := traceDBRawDecoderAuthorityByDecodeState[state]
		if !ok {
			t.Errorf("decoder authority table has no arm for %s", state)
			continue
		}
		if labels[label.DecodeAuthority] {
			t.Errorf("decoder authority label %q is shared by two states", label.DecodeAuthority)
		}
		labels[label.DecodeAuthority] = true
		var wantPrefix string
		switch kind {
		case traceDBSourceRawGateNotApplicable:
			wantPrefix = "not_applicable_"
		case traceDBSourceRawGateCensusIncomplete:
			wantPrefix = "withheld_"
		case traceDBSourceRawGateReady:
			wantPrefix = "available_"
		}
		if !strings.HasPrefix(label.DecodeAuthority, wantPrefix) {
			t.Errorf("%s (gate kind %d) publishes decode_authority %q, want prefix %q", state, kind, label.DecodeAuthority, wantPrefix)
		}
		if (label.RecoveryAuthority != "") != (kind == traceDBSourceRawGateReady) {
			t.Errorf("%s: recovery_authority presence %t disagrees with readiness", state, label.RecoveryAuthority != "")
		}
	}
	for state := range traceDBRawDecoderAuthorityByDecodeState {
		if _, ok := traceDBRawDecodeStateGates[state]; !ok {
			t.Errorf("decoder authority table names %q, which has no gate kind", state)
		}
	}
	// An undeclared decode_state is not absorbed: the reconcile fails loud.
	authority := TraceDBCoverage{Metadata: map[string]string{
		"decode_authority": "closed_target_decoders_pending_strict_profile_validation",
	}}
	err := reconcileTraceDBSourceRawDecoderAuthority(&authority,
		TraceDBCoverage{Metadata: map[string]string{"page_layout_state": "qword_length_cpu_candidate_all_pages"}},
		TraceDBCoverage{Metadata: map[string]string{"decode_state": "strict_target_ledger_partially_new"}})
	var invariant *traceDBOutputInvariantError
	if !asTraceDBOutputInvariant(err, &invariant) || invariant.Reason != "source_raw_decoder_authority_unresolved" ||
		authority.Metadata["decode_authority"] != "closed_target_decoders_pending_strict_profile_validation" {
		t.Fatalf("undeclared decode_state was absorbed: err=%v authority=%+v", err, authority)
	}
}

// TestTraceDBRawAsyncLedgerFailsLoudOnUnrecognizedGateShape: the marker-async
// ledger has no coverage of its own, so the gate's fail-loud Unset shape is
// carried as gateErr for the callstack exporter; no state is minted and the
// placeholder stays.
func TestTraceDBRawAsyncLedgerFailsLoudOnUnrecognizedGateShape(t *testing.T) {
	inventory := newTraceDBSourceNameInventory()
	inventory.RawDecode.Found = false
	inventory.RawDecode.Metadata["decode_state"] = traceDBRawDecodeStateComplete
	ledger := newTraceDBRawAsyncMatchLedger(&inventory, traceDBSchedulerAuthority{})
	var invariant *traceDBOutputInvariantError
	if !asTraceDBOutputInvariant(ledger.gateErr, &invariant) || invariant.Reason != traceDBSourceRawLaneGateUnresolvedReason ||
		ledger.state != traceDBSourceRawLanePlaceholderState {
		t.Fatalf("unrecognized gate shape was absorbed by the async ledger: err=%v state=%q", ledger.gateErr, ledger.state)
	}
}
