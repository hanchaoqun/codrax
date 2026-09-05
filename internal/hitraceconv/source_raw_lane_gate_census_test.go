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
//     load-bearing roster (traceDBUnresolvedKeyForwardingWrites); a
//     scope-declared name re-bound in the body is never single-valued and a
//     closure parameter shadows the outer name inside its literal,
//     resolving through the closure's own call sites (round eight); the
//     scopes are lexical — the body the root, every closure a child, the
//     receiver a root declaration, every name and binding keyed by its
//     declaration — and a compared range key resolves its operand through
//     the key lanes or fails loud at the range (round nine).

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

// traceDBBoundTo reports whether node is a right-hand side bound one-to-one
// under parent (`x := node`, `x = node`, `var x = node`) and returns the
// target it is bound to.
func traceDBBoundTo(node ast.Node, parent ast.Node) (ast.Expr, bool) {
	switch p := parent.(type) {
	case *ast.AssignStmt:
		if len(p.Lhs) != len(p.Rhs) {
			return nil, false
		}
		for i, rhs := range p.Rhs {
			if rhs == node || traceDBStripParens(rhs) == node {
				return p.Lhs[i], true
			}
		}
	case *ast.ValueSpec:
		if len(p.Names) != len(p.Values) {
			return nil, false
		}
		for i, value := range p.Values {
			if value == node || traceDBStripParens(value) == node {
				return p.Names[i], true
			}
		}
	}
	return nil, false
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

// traceDBIsMetadataSelector: the `<x>.Metadata` selector — the state map,
// statically.
func traceDBIsMetadataSelector(expr ast.Expr) bool {
	sel, ok := traceDBStripParens(expr).(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Metadata"
}

// traceDBName is one declaration of one lexical scope (round nine, #0 /
// #1): a parameter — the receiver included — a named result, a local
// declared with `:=` / `var`, or a range name; the unit every binding,
// every resolution lane and the entry seeding are keyed by. The same
// spelling in another scope is another name.
//
// Bindings: the body's `=` / `:=` / var bindings of the name, a tuple, an
// op-assignment or a range assignment binding it to nothing the census can
// spell (nil). A parameter, result, receiver or range name is seeded with
// its incoming binding — nil, the body cannot spell it (round eight, #2) —
// so bound in the body it is multi-valued, never single and never once.
//
//   - single: every binding spells the same value — `x := "lit"`, `x =
//     const`, `var x = "lit"`, `x := string(<constant>)`, a copy of another
//     single-valued name, a package variable never re-bound — resolved to a
//     fixpoint through the function's scopes before the package (round
//     seven, #4: a copy of a local shadowing a package constant carries the
//     local's value, or nothing);
//   - once: bound exactly once, to an expression of any shape — the lane a
//     key spelled through a copy of a parameter, a conversion of one, or a
//     concatenation resolves through (round seven, #5).
//
// A name bound through a tuple (`x, ok := f()`, `var x, y = f()`), an
// op-assignment, or more than once to different values is neither: a
// variable, not a constant, and never the first value it happened to be
// bound to (round six, #3).
type traceDBName struct {
	scope *traceDBLexicalScope
	name  string
	// pos: where the name is visible from — the end of its first `:=` / var
	// statement, the body of its range statement; token.NoPos (the whole
	// body) for a parameter, result or receiver.
	pos token.Pos
	// position: a string / lane-key typed parameter's position, resolved
	// through the callers of the function or the call sites of the closure;
	// -1 for every other declaration (the receiver included).
	position int
	// ranges: the range statements declaring the name.
	ranges int
	// bound: the body's bindings of the name (a zero-value `var` included);
	// a parameter or range name bound in the body is unresolved.
	bound int
	// exprs: the bindings — the entry seed and every body binding, nil for
	// one the census cannot spell.
	exprs []ast.Expr
	// listKeys: a range value over a key-list literal → the literal's keys
	// (resolved through the key lanes, round nine).
	listKeys []string
	// key: a range key compared against a state key → that key.
	key string
	// everyKey: an uncompared range key over the state map — the `.Metadata`
	// selector statically; the reader census adds the ranges over its
	// tainted map locals and map-returning helper calls.
	everyKey bool
	// escaped: used as a value other than a call's function operand — a
	// local bound to a closure and used this way escapes, so the closure's
	// call sites are unknown.
	escaped bool
	// single / isSingle and once: the string bindings above.
	single   string
	isSingle bool
	once     ast.Expr
}

// traceDBLexicalScope is one lexical scope of a function (round nine): the
// function body — the root, declaring the receiver, the parameters and the
// named results — or a closure literal, a child of the scope it sits in,
// declaring its own parameters and results. Locals declared with `:=` /
// `var` and range names belong to the scope they are declared in. An
// occurrence resolves to the innermost scope declaring the name at that
// point: outer names are visible inside a closure unless the closure
// declares the same spelling, and a closure parameter is the closure's own
// declaration (round eight, #3). Blocks inside one scope are not modelled:
// a name declared twice in one scope is one multi-valued name (unresolved,
// never the first value).
type traceDBLexicalScope struct {
	owner  *traceDBFuncScope
	parent *traceDBLexicalScope
	lit    *ast.FuncLit // nil at the root
	names  map[string]*traceDBName
}

// declare: the scope's declaration of name, visible from pos (the earliest
// declaration wins); nil for the blank identifier.
func (scope *traceDBLexicalScope) declare(name string, pos token.Pos) *traceDBName {
	if name == "_" {
		return nil
	}
	decl, ok := scope.names[name]
	if !ok {
		decl = &traceDBName{scope: scope, name: name, pos: pos, position: -1}
		scope.names[name] = decl
	} else if pos < decl.pos {
		decl.pos = pos
	}
	return decl
}

// declareFields: the receiver, the parameters or the results of the scope's
// function — each seeded with its entry binding; a key-typed parameter at
// its position.
func (scope *traceDBLexicalScope) declareFields(fields *ast.FieldList, params bool) {
	if fields == nil {
		return
	}
	position := 0
	for _, field := range fields.List {
		for _, name := range field.Names {
			if decl := scope.declare(name.Name, token.NoPos); decl != nil {
				decl.exprs = append(decl.exprs, nil)
				if params && traceDBIsKeyType(field.Type) {
					decl.position = position
				}
			}
			position++
		}
	}
}

// lookup: the innermost declaration of name visible at pos; nil for a
// package-level name.
func (scope *traceDBLexicalScope) lookup(name string, pos token.Pos) *traceDBName {
	for s := scope; s != nil; s = s.parent {
		if decl, ok := s.names[name]; ok && decl.pos <= pos {
			return decl
		}
	}
	return nil
}

// traceDBFuncScope is the static scope of one top-level function, keyed by
// declaration (round seven, #6) and lexically nested (round nine): the root
// scope and one scope per closure literal, the range statements' key
// comparisons, and the local each closure is bound to.
type traceDBFuncScope struct {
	fn      *ast.FuncDecl
	file    string
	imports map[string]bool
	root    *traceDBLexicalScope
	lits    map[*ast.FuncLit]*traceDBLexicalScope
	// comparedRanges: range statement → the state key its key is compared
	// against in its body.
	comparedRanges map[*ast.RangeStmt]string
	// rangeProblems: range statement → the report of a key comparison the
	// census cannot place — an operand it cannot resolve, several state keys
	// — raised by the reader census over the state map and by the write
	// census over the `.Metadata` selector (fail-loud, §40.50), the key
	// staying unresolved.
	rangeProblems map[*ast.RangeStmt]string
	// closureLocals: closure literal → the local it is bound to (`f :=
	// func…`, `var f = func…`, `f = func…`).
	closureLocals map[*ast.FuncLit]*traceDBName
}

// at: the lexical scope of a node under stack — the innermost closure
// literal, else the root.
func (scope *traceDBFuncScope) at(stack []ast.Node) *traceDBLexicalScope {
	for i := len(stack) - 1; i >= 0; i-- {
		if lit, ok := stack[i].(*ast.FuncLit); ok {
			return scope.lits[lit]
		}
	}
	return scope.root
}

// decls: every declaration of the function, the root's and the closures'.
func (scope *traceDBFuncScope) decls() []*traceDBName {
	var out []*traceDBName
	for _, decl := range scope.root.names {
		out = append(out, decl)
	}
	for _, lit := range scope.lits {
		for _, decl := range lit.names {
			out = append(out, decl)
		}
	}
	return out
}

// traceDBCallSite is one call expression, the scope it sits in and the
// closures enclosing it (outermost first).
type traceDBCallSite struct {
	scope *traceDBFuncScope
	call  *ast.CallExpr
	lits  []*ast.FuncLit
}

// inside reports whether the site sits in the closure's body.
func (site traceDBCallSite) inside(lit *ast.FuncLit) bool {
	for _, enclosing := range site.lits {
		if enclosing == lit {
			return true
		}
	}
	return false
}

// traceDBKeyParam names one key parameter of one function.
type traceDBKeyParam struct {
	fn       *ast.FuncDecl
	position int
}

// traceDBKeyPath is one resolution's path: the key parameters being resolved
// through their callers (outermost first entered), the closure parameters
// being resolved through their closure's call sites, and the once-bound
// names being resolved through their expression.
type traceDBKeyPath struct {
	params   map[traceDBKeyParam]bool
	closures map[*traceDBName]bool
	names    map[*traceDBName]bool
}

func newTraceDBKeyPath() *traceDBKeyPath {
	return &traceDBKeyPath{params: map[traceDBKeyParam]bool{}, closures: map[*traceDBName]bool{}, names: map[*traceDBName]bool{}}
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
//     function's own cycle spells (round seven, #3). A closure's key-typed
//     parameter is the closure's own declaration (round eight, #3): inside
//     the literal it shadows the outer name and resolves the same way
//     through the closure's own call sites — the literal bound exactly once
//     to a local that is only ever called.
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
//     spell (round seven, #3). Surfaces only inside the caller loops of
//     paramKeys and closureParamKeys: a top-level resolution never returns
//     it.
//   - traceDBKeyUnresolved: anything else — a re-bound parameter, receiver,
//     closure parameter, named result or range name (a scope-declared name
//     is seeded with its entry binding and never single-valued from the
//     body's bindings alone, so a copy of it carries nothing — round eight,
//     #2; the receiver, round nine, #1), a multi-valued local
//     (tuple-bound, re-bound, bound to a value the census cannot resolve,
//     or a copy of one), a parameter of a function that escapes as a value,
//     has no caller outside its own cycle, or has a caller whose argument
//     does not resolve, a closure parameter whose closure is not bound once
//     to a local that is only ever called, or has no call site outside its
//     own body, or has a site whose argument does not resolve, a closure
//     result or non-key closure parameter, a range key under a comparison
//     the census cannot place (its range carries the fail-loud report), a
//     name shadowing a package constant without a single value, a computed
//     expression.
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
	// decls: every identifier occurrence of every body → the declaration it
	// resolves to; nil for a package-level name (round nine).
	decls map[*ast.Ident]*traceDBName
}

func newTraceDBKeyResolver(files map[string]*ast.File, consts map[string]string) *traceDBKeyResolver {
	r := &traceDBKeyResolver{
		consts:      consts,
		packageVars: map[string]string{},
		functions:   map[string]*ast.FuncDecl{},
		methods:     map[string][]*ast.FuncDecl{},
		scopes:      map[*ast.FuncDecl]*traceDBFuncScope{},
		escaped:     map[*ast.FuncDecl]bool{},
		decls:       map[*ast.Ident]*traceDBName{},
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
			scope := &traceDBFuncScope{fn: fn, file: name, imports: imports, lits: map[*ast.FuncLit]*traceDBLexicalScope{},
				comparedRanges: map[*ast.RangeStmt]string{}, rangeProblems: map[*ast.RangeStmt]string{}, closureLocals: map[*ast.FuncLit]*traceDBName{}}
			scope.root = &traceDBLexicalScope{owner: scope, names: map[string]*traceDBName{}}
			scope.root.declareFields(fn.Recv, false)
			scope.root.declareFields(fn.Type.Params, true)
			scope.root.declareFields(fn.Type.Results, false)
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
	// like a constant — spelled through the constants alone, whatever the
	// order of the declarations.
	packageVars := map[string]string{}
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
					if value, ok := r.spelled(vs.Values[i]); ok {
						packageVars[ident.Name] = value
					}
				}
			}
		}
	}
	r.packageVars = packageVars
	sort.Slice(r.order, func(i, j int) bool {
		if r.scopes[r.order[i]].file != r.scopes[r.order[j]].file {
			return r.scopes[r.order[i]].file < r.scopes[r.order[j]].file
		}
		return r.order[i].Pos() < r.order[j].Pos()
	})
	// Static pre-pass, stage one, first pass: the declarations of every
	// lexical scope — the root's receiver, parameters and results above; a
	// closure's own parameters and results; the `:=` / var locals and the
	// range names of the scope they sit in (round nine).
	for _, fn := range r.order {
		scope := r.scopes[fn]
		traceDBWalk(fn.Body, func(node ast.Node, stack []ast.Node) {
			at := scope.at(stack)
			switch n := node.(type) {
			case *ast.FuncLit:
				child := &traceDBLexicalScope{owner: scope, parent: at, lit: n, names: map[string]*traceDBName{}}
				scope.lits[n] = child
				child.declareFields(n.Type.Params, true)
				child.declareFields(n.Type.Results, false)
			case *ast.AssignStmt:
				if n.Tok != token.DEFINE {
					return
				}
				for _, lhs := range n.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok {
						at.declare(ident.Name, n.End())
					}
				}
			case *ast.ValueSpec:
				for _, name := range n.Names {
					at.declare(name.Name, n.End())
				}
			case *ast.RangeStmt:
				if n.Tok != token.DEFINE {
					return
				}
				for _, expr := range []ast.Expr{n.Key, n.Value} {
					if ident, ok := expr.(*ast.Ident); ok {
						if decl := at.declare(ident.Name, n.Body.Pos()); decl != nil {
							decl.ranges++
							decl.exprs = append(decl.exprs, nil)
						}
					}
				}
			}
		})
	}
	// Stage one, second pass: every identifier occurrence resolved to its
	// declaration — a binding target to the name it binds, a value
	// occurrence to the innermost declaration visible at it — with the
	// bindings each records (a package variable re-bound in any body is a
	// variable, not a constant); the local each closure is bound to; every
	// call site with its enclosing closures; the names and the key-parameter
	// functions used as values.
	keyParamDecls := func(scope *traceDBFuncScope, expr ast.Expr) []*ast.FuncDecl {
		var decls []*ast.FuncDecl
		switch f := expr.(type) {
		case *ast.Ident:
			if fn, ok := r.functions[f.Name]; ok && r.decls[f] == nil {
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
			if r.scopes[decl].root.hasKeyParams() {
				out = append(out, decl)
			}
		}
		return out
	}
	for _, fn := range r.order {
		scope := r.scopes[fn]
		traceDBWalk(fn.Body, func(node ast.Node, stack []ast.Node) {
			at, parent := scope.at(stack), traceDBNearest(stack)
			switch n := node.(type) {
			case *ast.FuncLit:
				if target, ok := traceDBBoundTo(node, parent); ok {
					if ident, ok := target.(*ast.Ident); ok {
						if local := r.decls[ident]; local != nil {
							scope.closureLocals[n] = local
						}
					}
				}
			case *ast.CallExpr:
				var lits []*ast.FuncLit
				for _, ancestor := range stack {
					if lit, ok := ancestor.(*ast.FuncLit); ok {
						lits = append(lits, lit)
					}
				}
				r.callSites = append(r.callSites, traceDBCallSite{scope: scope, call: n, lits: lits})
			case *ast.Ident:
				if n.Name == "_" {
					return
				}
				if r.bindTarget(at, n, parent) {
					return
				}
				if traceDBIdentIsName(n, parent) {
					return
				}
				decl := at.lookup(n.Name, n.Pos())
				r.decls[n] = decl
				if traceDBIsCallFun(node, parent) {
					return
				}
				if decl != nil {
					decl.escaped = true
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
	// Stage two: the string bindings of every declaration, spelled through
	// the scopes above.
	for _, fn := range r.order {
		r.bind(r.scopes[fn])
	}
	// Stage three: the key-list ranges and the compared range keys, resolved
	// through the key lanes as a package-wide fixpoint — a comparison against
	// a key carried from another range resolves once that range is decided —
	// then the every-key ranges over the `.Metadata` selector; a comparison
	// the fixpoint cannot place is the range's fail-loud report (round nine,
	// #0: never an every-key range).
	type rangeCase struct {
		scope      *traceDBFuncScope
		stmt       *ast.RangeStmt
		name       *traceDBName // the key, or the value over a key-list literal
		list       *ast.CompositeLit
		unresolved string
	}
	var pending []*rangeCase
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
				if ident, ok := n.Value.(*ast.Ident); ok {
					if decl := r.decls[ident]; decl != nil && decl.ranges > 0 {
						pending = append(pending, &rangeCase{scope: scope, stmt: n, name: decl, list: lit})
					}
				}
				return true
			}
			if ident, ok := n.Key.(*ast.Ident); ok && ident.Name != "_" {
				pending = append(pending, &rangeCase{scope: scope, stmt: n, name: r.decls[ident]})
			}
			return true
		})
	}
	decide := func(rc *rangeCase) bool {
		if rc.list != nil {
			keys := []string{}
			for _, element := range rc.list.Elts {
				resolved, resolution := r.resolveKey(element, newTraceDBKeyPath())
				switch resolution {
				case traceDBKeySpelled, traceDBKeyCarried:
					keys = append(keys, resolved...)
				case traceDBKeyExcluded:
				default:
					return false
				}
			}
			rc.name.listKeys = keys
			return true
		}
		states, unresolved := r.rangeKeyComparison(rc.name, rc.stmt.Body)
		ranged := rc.name != nil && rc.name.ranges > 0
		switch {
		case len(unresolved) > 0:
			rc.unresolved = unresolved[0]
			return false
		case len(states) > 1:
			rc.scope.rangeProblems[rc.stmt] = fmt.Sprintf("compared range key names several state keys (%s)", strings.Join(states, ", "))
		case len(states) == 1:
			rc.scope.comparedRanges[rc.stmt] = states[0]
			if ranged && rc.name.key == "" {
				rc.name.key = states[0]
			}
		case ranged && traceDBIsMetadataSelector(rc.stmt.X):
			rc.name.everyKey = true
		}
		return true
	}
	for progress := true; progress; {
		progress = false
		var rest []*rangeCase
		for _, rc := range pending {
			if decide(rc) {
				progress = true
			} else {
				rest = append(rest, rc)
			}
		}
		pending = rest
	}
	for _, rc := range pending {
		if rc.list == nil {
			rc.scope.rangeProblems[rc.stmt] = fmt.Sprintf("compared range key the census cannot resolve (%s)", rc.unresolved)
		}
	}
	return r
}

// hasKeyParams reports whether the root scope declares a key-typed
// parameter.
func (scope *traceDBLexicalScope) hasKeyParams() bool {
	for _, decl := range scope.names {
		if decl.position >= 0 {
			return true
		}
	}
	return false
}

// bindTarget records ident as a binding target under parent — a `=` / `:=`
// left-hand side, a var name, a range key or value — resolving it to the
// name it binds (a `:=`, var or `:=`-range target is the declaration of
// its own scope; a `=` target the innermost visible declaration, a
// package variable when there is none) and recording the binding: the
// right-hand side one-to-one, nil for a tuple, an op-assignment, a range
// assignment or a zero-value var. Reports whether ident was a target.
func (r *traceDBKeyResolver) bindTarget(at *traceDBLexicalScope, ident *ast.Ident, parent ast.Node) bool {
	record := func(decl *traceDBName, expr ast.Expr, bound bool) {
		r.decls[ident] = decl
		if decl == nil {
			delete(r.packageVars, ident.Name)
			return
		}
		decl.bound++
		if bound {
			decl.exprs = append(decl.exprs, expr)
		}
	}
	switch p := parent.(type) {
	case *ast.AssignStmt:
		for i, lhs := range p.Lhs {
			if lhs != ident {
				continue
			}
			decl := at.lookup(ident.Name, ident.Pos())
			if p.Tok == token.DEFINE {
				decl = at.names[ident.Name]
			}
			if len(p.Lhs) != len(p.Rhs) || (p.Tok != token.ASSIGN && p.Tok != token.DEFINE) {
				record(decl, nil, true)
			} else {
				record(decl, p.Rhs[i], true)
			}
			return true
		}
	case *ast.ValueSpec:
		for i, name := range p.Names {
			if name != ident {
				continue
			}
			decl := at.names[ident.Name]
			switch {
			case len(p.Values) > 0 && len(p.Names) != len(p.Values):
				record(decl, nil, true)
			case i < len(p.Values):
				record(decl, p.Values[i], true)
			default:
				record(decl, nil, false)
			}
			return true
		}
	case *ast.RangeStmt:
		if p.Key != ident && p.Value != ident {
			return false
		}
		if p.Tok == token.DEFINE {
			r.decls[ident] = at.names[ident.Name]
			return true
		}
		record(at.lookup(ident.Name, ident.Pos()), nil, true)
		return true
	}
	return false
}

// bind computes the string bindings of every declaration of scope: a name is
// single-valued when its every binding spells the same value — resolved to
// a fixpoint over the function's scopes, so a copy of a single-valued name
// is single (round seven, #4) — and once-bound when the body binds it
// exactly once to an expression. A declared parameter, result, receiver or
// range name carries its entry seed, which nothing spells (round eight,
// #2): bound in the body it is multi-valued, never single and never once,
// so a copy of it carries nothing and the name resolves through its own
// lane or not at all.
func (r *traceDBKeyResolver) bind(scope *traceDBFuncScope) {
	pending := map[*traceDBName]bool{}
	for _, decl := range scope.decls() {
		if len(decl.exprs) == 1 && decl.exprs[0] != nil {
			decl.once = decl.exprs[0]
		}
		single := len(decl.exprs) > 0
		for _, expr := range decl.exprs {
			single = single && expr != nil
		}
		if single {
			pending[decl] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for decl := range pending {
			value, resolved, agree := "", true, true
			for i, expr := range decl.exprs {
				spelled, ok := r.spelled(expr)
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
				delete(pending, decl)
			case resolved:
				decl.single, decl.isSingle = value, true
				delete(pending, decl)
				changed = true
			}
		}
	}
}

// spelled: the one value a string-valued operand names — a literal, a
// `string(<x>)` / `<keyType>(<x>)` conversion of one, a single-valued
// declaration, or, for a name no scope declares, a package variable never
// re-bound or a package constant. The declaration precedes the package
// (round six, #5; round seven, #4 through every copy): a declared name
// without a single value never resolves as the constant it shadows.
func (r *traceDBKeyResolver) spelled(expr ast.Expr) (string, bool) {
	switch e := traceDBStripParens(expr).(type) {
	case *ast.BasicLit:
		return traceDBStringLiteral(e)
	case *ast.CallExpr:
		if fun, ok := e.Fun.(*ast.Ident); ok && traceDBIsKeyType(fun) && len(e.Args) == 1 {
			return r.spelled(e.Args[0])
		}
	case *ast.Ident:
		if decl := r.decls[e]; decl != nil {
			return decl.single, decl.isSingle
		}
		if value, ok := r.packageVars[e.Name]; ok {
			return value, true
		}
		value, ok := r.consts[e.Name]
		return value, ok
	}
	return "", false
}

// rangeKeyComparison: the state keys the range key is compared against
// inside body — `k == <key>` / `k != <key>` either way round, or a switch
// over k with a case naming a key — every operand resolved through the key
// lanes (round nine, #0: a key parameter through its callers, a local in its
// own scope); the operands the lanes cannot resolve, by text. An operand
// that resolves to no state key compares against nothing the census
// follows.
func (r *traceDBKeyResolver) rangeKeyComparison(key *traceDBName, body *ast.BlockStmt) (states, unresolved []string) {
	isKey := func(expr ast.Expr) bool {
		ident, ok := traceDBStripParens(expr).(*ast.Ident)
		return ok && key != nil && r.decls[ident] == key
	}
	seen := map[string]bool{}
	operand := func(expr ast.Expr) {
		keys, resolution := r.resolveKey(expr, newTraceDBKeyPath())
		switch resolution {
		case traceDBKeySpelled, traceDBKeyCarried:
			for _, k := range keys {
				if traceDBStateReadKeys[k] && !seen[k] {
					seen[k] = true
					states = append(states, k)
				}
			}
		case traceDBKeyExcluded:
		default:
			unresolved = append(unresolved, exprText(expr))
		}
	}
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.BinaryExpr:
			if n.Op != token.EQL && n.Op != token.NEQ {
				return true
			}
			if isKey(n.X) {
				operand(n.Y)
			} else if isKey(n.Y) {
				operand(n.X)
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
					operand(expr)
				}
			}
		}
		return true
	})
	sort.Strings(states)
	return states, unresolved
}

// concatExcludes: the concatenation's outermost literal operands rule every
// state key out.
func (r *traceDBKeyResolver) concatExcludes(e *ast.BinaryExpr) bool {
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
		literal, ok := r.spelled(operand)
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

// names reports whether the call names fn (a plain function by an
// identifier no scope declares, a method by selector name; an
// import-qualified selector never).
func (r *traceDBKeyResolver) names(site traceDBCallSite, fn *ast.FuncDecl) bool {
	switch f := traceDBStripParens(site.call.Fun).(type) {
	case *ast.Ident:
		return fn.Recv == nil && r.decls[f] == nil && r.functions[f.Name] == fn
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
		passed, resolution := r.resolveKey(site.call.Args[position], path)
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

// closureParamKeys: the keys a closure's key-typed parameter carries (round
// eight, #3) — the argument every call site of the closure passes at the
// parameter's position, plus the keys a site inside the closure's own body
// spells, the closure being bound exactly once to a local that is only
// ever called. Unresolved for a parameter bound in the body, a literal not
// bound to a local, a local bound more than once or used as a value, no
// call site outside the closure's own body, or a site whose argument does
// not resolve; forwarded when, resolved from inside a cycle, every site is
// the cycle.
func (r *traceDBKeyResolver) closureParamKeys(param *traceDBName, path *traceDBKeyPath) ([]string, traceDBKeyResolution) {
	scope, lit := param.scope.owner, param.scope.lit
	local, bound := scope.closureLocals[lit]
	if !bound || traceDBStripParens(local.once) != ast.Expr(lit) || local.escaped {
		return nil, traceDBKeyUnresolved
	}
	if path.closures[param] {
		return nil, traceDBKeyForwarded // re-entered from inside its own cycle
	}
	nested := len(path.params) > 0 || len(path.closures) > 0
	path.closures[param] = true
	defer delete(path.closures, param)
	var keys []string
	sites, callers := 0, 0
	for _, site := range r.callSites {
		if site.scope != scope {
			continue
		}
		fun, ok := traceDBStripParens(site.call.Fun).(*ast.Ident)
		if !ok || r.decls[fun] != local {
			continue
		}
		sites++
		if site.call.Ellipsis.IsValid() || param.position >= len(site.call.Args) {
			return nil, traceDBKeyUnresolved
		}
		passed, resolution := r.resolveKey(site.call.Args[param.position], path)
		switch resolution {
		case traceDBKeyUnresolved, traceDBKeyEveryKey:
			return nil, traceDBKeyUnresolved
		case traceDBKeyForwarded:
			keys = append(keys, passed...) // the cycle's own spelled keys, no caller
			continue
		}
		keys = append(keys, passed...)
		if !site.inside(lit) {
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
func (r *traceDBKeyResolver) resolveKey(expr ast.Expr, path *traceDBKeyPath) ([]string, traceDBKeyResolution) {
	switch e := traceDBStripParens(expr).(type) {
	case *ast.BasicLit:
		if key, ok := traceDBStringLiteral(e); ok {
			return []string{key}, traceDBKeySpelled
		}
	case *ast.CallExpr:
		// A conversion (`string(k)` / `<keyType>(k)`) over a resolvable
		// operand resolves as the operand.
		if fun, ok := e.Fun.(*ast.Ident); ok && traceDBIsKeyType(fun) && len(e.Args) == 1 {
			return r.resolveKey(e.Args[0], path)
		}
	case *ast.BinaryExpr:
		if r.concatExcludes(e) {
			return nil, traceDBKeyExcluded
		}
	case *ast.Ident:
		// The declaration the occurrence resolves to (round nine), then its
		// lane: a key parameter — of the function through its callers, of a
		// closure through the closure's call sites (round eight, #3) — a
		// compared range key, a key-list range value, an every-key range, a
		// single-valued or once-bound name; any other declaration is
		// unresolved. Only a name no scope declares is the package variable
		// or constant it names.
		if e.Name == "_" {
			return nil, traceDBKeyUnresolved
		}
		decl := r.decls[e]
		if decl == nil {
			if value, ok := r.packageVars[e.Name]; ok {
				return []string{value}, traceDBKeySpelled
			}
			if value, ok := r.consts[e.Name]; ok {
				return []string{value}, traceDBKeySpelled
			}
			return nil, traceDBKeyUnresolved
		}
		if decl.position >= 0 {
			if decl.bound > 0 || decl.ranges > 0 {
				return nil, traceDBKeyUnresolved
			}
			if decl.scope.lit == nil {
				return r.paramKeys(decl.scope.owner.fn, decl.position, path)
			}
			return r.closureParamKeys(decl, path)
		}
		if decl.ranges > 0 {
			if decl.bound > 0 || decl.ranges > 1 {
				return nil, traceDBKeyUnresolved
			}
			if decl.key != "" {
				return []string{decl.key}, traceDBKeyCarried
			}
			if decl.listKeys != nil {
				return decl.listKeys, traceDBKeyCarried
			}
			if decl.everyKey {
				return nil, traceDBKeyEveryKey
			}
			return nil, traceDBKeyUnresolved
		}
		if decl.isSingle {
			return []string{decl.single}, traceDBKeySpelled
		}
		if decl.once != nil {
			// Bound once to an expression the bindings cannot spell: a copy
			// or conversion of a parameter, a concatenation — resolved as
			// that expression (round seven, #5).
			if path.names[decl] {
				return nil, traceDBKeyUnresolved
			}
			path.names[decl] = true
			defer delete(path.names, decl)
			return r.resolveKey(decl.once, path)
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
// disclosed roster traceDBUnresolvedKeyForwardingWrites — and every range
// over the `.Metadata` selector whose key comparison the census cannot place
// (round nine) is reported as a failure (fail-loud on unrecognized shapes).
// A computed key that resolves
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
			value, ok := resolver.spelled(rhs)
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
			resolve := func(expr ast.Expr) (string, bool) { return resolver.spelled(expr) }
			switch n := node.(type) {
			case *ast.RangeStmt:
				// A range over the `.Metadata` selector whose key comparison the
				// census cannot place (round nine, #0): the key stays
				// unresolved, the range is red.
				if text, ok := scope.rangeProblems[n]; ok && traceDBIsMetadataSelector(n.X) {
					t.Errorf("%s:%d: %s", name, line(n), text)
				}
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
				keys, resolution := resolver.resolveKey(index, newTraceDBKeyPath())
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
						decl := resolver.decls[ident]
						isParam := decl != nil && decl.scope.lit == nil && decl.position >= 0
						_, wrapper := traceDBSourceRawGateCallers[funcName]
						if isParam && wrapper {
							return // the wrapper forwards its own key parameter; its callers mint
						}
					}
					resolved, resolution := resolver.resolveKey(n.Args[keyArg], newTraceDBKeyPath())
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
