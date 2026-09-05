package hitraceconv

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// source_raw_lane_gate_census_readers_test.go — batch-six review fold-in
// #7 / #9 (colleague_merge_audit §40.53 收编复核再收编). Two censuses over the
// source-raw lane class, each a pure function over parsed files so every
// shape has a self-red witness through the real parser:
//
//   - traceDBUngatedLaneKeyWriters: every file that publishes a lane state
//     key (a plain write site of the write census) runs or inherits the class
//     gate for THAT key in the same file. The per-key rule ("some file gates
//     reconciliation_state") let a second writer of the key publish a
//     complete_ closure over a join that had said not-applicable.
//   - traceDBStateReadProblems: every read of decode_state / publication_state
//     — a state key indexed over `<x>.Metadata` or over a local tainted by
//     the map (an alias, a map[string]string parameter, a same-package
//     helper's map result), the key spelled at the site or resolved through
//     a single-valued local / package variable, a compared range key, a
//     key-list range, or a string / lane-key parameter's callers plus the
//     keys its own recursion spells (a once-bound copy, conversion or
//     concatenation of a key resolving as what it is bound to — the
//     traceDBKeyResolver shared with the write census); a range value
//     under a key comparison; a
//     local tainted by a read through any chain of bindings / launders /
//     helper results (function and method values included) — lands in a
//     recognized consumer position (a lookup into a package-level table
//     keyed by the decode_state constants, a registered classifier call, a
//     binding, a forwarding write, prose concatenation / fmt, a helper's
//     return). A write target (`<x>.Metadata[<key>] = …`) is not a read. A
//     hand-kept switch/case, a ==/!= comparison, a strings.* prefix/substring
//     test, a lookup through a map the census cannot see, a state key over
//     an expression that is not the map, a key the census cannot resolve
//     over the state map, a read returned from a closure the census cannot
//     follow, a map or producer value in a shape the walker cannot follow,
//     or any other shape is red (§40.50).

// traceDBUngatedLaneKeyWriters — rule: (file, key) groups of plain writes
// need a gate call (apply or inherit) under the same key in the same file.
func traceDBUngatedLaneKeyWriters(sites []traceDBStateWriteSite) []string {
	type group struct{ file, key string }
	gated := map[group]bool{}
	writers := map[group][]traceDBStateWriteSite{}
	for _, site := range sites {
		g := group{file: site.file, key: site.key}
		switch {
		case site.viaGate:
			gated[g] = true
		case site.constructor:
		default:
			writers[g] = append(writers[g], site)
		}
	}
	var problems []string
	for g, plain := range writers {
		if gated[g] {
			continue
		}
		functions := map[string]bool{}
		for _, site := range plain {
			functions[site.function] = true
		}
		var names []string
		for name := range functions {
			names = append(names, name)
		}
		sort.Strings(names)
		problems = append(problems, fmt.Sprintf("%s:%d: %s publishes %s without running or inheriting the class gate for that key in this file",
			g.file, plain[0].line, strings.Join(names, ","), g.key))
	}
	sort.Strings(problems)
	return problems
}

// traceDBStateReadKeys is the closed set of metadata keys the reader census
// follows; a key read through `string(<constant>)` resolves the same way.
var traceDBStateReadKeys = map[string]bool{"decode_state": true, "publication_state": true}

// traceDBStateReadClassifiers are the registered consumers of a state read:
// the roster predicate and the typed visibility conversion whose rows table
// is pinned total over the closed visibility set.
var traceDBStateReadClassifiers = map[string]bool{
	"traceDBSourceRawPublicationStateBlocksEvaluation": true,
	"traceDBSourceRawVisibilityState":                  true,
}

// traceDBStateReadSinks are the registered forwarding sinks of a state read:
// the class gate funnel receives the gate classifier's reason — the raw
// decode_state, returned by traceDBSourceRawDecodeGate at result position 1 —
// and forwards it verbatim into reason metadata and Skipped prose.
var traceDBStateReadSinks = map[string]bool{
	"traceDBMintSourceRawLaneGateOutcome": true,
}

// traceDBStateStringStdlib reports whether a stdlib call launders its
// arguments into its result: a state read handed to one of these taints the
// value the call produces (fold-in #8).
func traceDBStateStringStdlib(fun string) bool {
	return traceDBStateSprint(fun) || strings.HasPrefix(fun, "strings.")
}

// traceDBStateSprint reports the fmt calls whose result IS the formatted
// state: a read under one of them is judged where the call's result lands.
func traceDBStateSprint(fun string) bool {
	switch fun {
	case "fmt.Sprint", "fmt.Sprintf", "fmt.Sprintln":
		return true
	}
	return false
}

// traceDBIdentIsName reports whether ident occurs as a name rather than a
// value under parent: a selector field, a struct-literal key, a binding
// target, a declared name, a parameter/result, a range key/value.
func traceDBIdentIsName(ident *ast.Ident, parent ast.Node) bool {
	switch p := parent.(type) {
	case *ast.SelectorExpr:
		return p.Sel == ident
	case *ast.KeyValueExpr:
		return p.Key == ident
	case *ast.AssignStmt:
		for _, lhs := range p.Lhs {
			if lhs == ident {
				return true
			}
		}
	case *ast.ValueSpec:
		for _, declared := range p.Names {
			if declared == ident {
				return true
			}
		}
	case *ast.Field:
		return true
	case *ast.RangeStmt:
		return p.Key == ident || p.Value == ident
	}
	return false
}

// traceDBStateReadProblems is the reader census over the given files.
//
// Taint (fold-in #8): a local bound to a state read is tainted — through any
// chain of bindings (sticky; re-binding never launders), through parentheses,
// string concatenation, the arguments of fmt.Sprint* / strings.* calls, the
// comma-ok form of a direct read, and the result positions of same-package
// helpers (plain functions by name; methods by selector name, a qualifier
// that is one of the file's imports never being a method) — computed as a
// package-wide fixpoint so a helper that returns a read taints its callers.
// A binding whose right-hand side contains a read but is neither a taint
// carrier nor a consume shape (lookup, call, comparison, literal) is red:
// the census cannot follow the bound value.
//
// Map taint (fold-in #8, round four): the state map itself is tainted — a
// bare `<x>.Metadata` selector, any local bound to one (sticky), and every
// `map[string]string` parameter of a function or closure — so a read is an
// index by a state key over the `.Metadata` selector OR over a tainted map
// local. A range whose key is compared (==/!=, or a switch case) against a
// state key literal taints the range value with that key and resolves the
// key identifier to it, so `m[k]` under the comparison is a read. A
// function or method value of a same-package producer bound to a local is
// followed into the local's calls; the bare value occurrence is a read. A
// state key indexed over an expression that is neither the selector nor a
// tainted map, a map occurrence in a shape the walker cannot classify, and
// a producer value in any position but a binding to a local are red.
//
// Key resolution (round five; the shared traceDBKeyResolver since round
// seven): the index key resolves through the lanes of resolveKey — spelled
// at the site, a single-valued local or package variable, a compared range
// key, a key-list range, a string / lane-key typed parameter through every
// direct caller's argument (wrappers recursively), a concatenation whose
// literal prefix / suffix rules every state key out. A key that resolves to
// no state key is not a read; a key the census cannot resolve over the
// state map is red.
//
// Round six: a name is single-valued only when the binding helper says so —
// a tuple binding (`k, _ = f()`) makes it multi-valued, never its first
// value; the function's own scope (parameters of any type, named results,
// closure parameters, range names, every name bound in the body) precedes
// the package, so a local or parameter shadowing a package constant
// resolves through its own lane or not at all; a call site inside the
// function itself, or inside a function on the current resolution path, is
// the function's own cycle rather than a caller; and a same-package helper
// whose result position returns the state map taints its callers' map
// locals and direct indexes like any alias.
//
// Round seven: a call site inside the cycle contributes the key it spells
// (only its forwarding of a parameter on the path adds nothing); a copy of
// a shadowing local carries the local's value or nothing; a key spelled
// through a once-bound copy or conversion of a parameter resolves through
// the parameter's callers, and through a once-bound concatenation as the
// concatenation — one resolver and one binding helper, keyed by
// declaration, shared with the write census.
//
// Classification: every read occurrence — a direct read (a plain write
// target is not one), a tainted local, a call to a helper with a tainted
// result — is judged by its nearest non-paren ancestor, except that an
// occurrence directly under a concatenation or a fmt.Sprint* argument is
// judged where that laundered expression lands. A return inside a closure
// the census cannot follow (any function literal not bound to a local) is
// red: the literal's value is judged nowhere.
func traceDBStateReadProblems(files map[string]*ast.File, fset *token.FileSet, consts map[string]string) (problems []string, reads int) {
	// Package-level tables keyed by the decode_state constants: the one
	// lookup target a reader may classify through.
	tables := map[string]bool{}
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
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.CompositeLit)
					if !ok {
						continue
					}
					for _, element := range lit.Elts {
						kv, ok := element.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						if ident, ok := kv.Key.(*ast.Ident); ok && strings.HasPrefix(ident.Name, "traceDBRawDecodeState") {
							tables[name.Name] = true
							break
						}
					}
				}
			}
		}
	}
	report := func(name string, node ast.Node, format string, args ...interface{}) {
		problems = append(problems, fmt.Sprintf("%s:%d: ", name, fset.Position(node.Pos()).Line)+fmt.Sprintf(format, args...))
	}
	isStringMapType := func(expr ast.Expr) bool {
		m, ok := expr.(*ast.MapType)
		if !ok {
			return false
		}
		key, ok := m.Key.(*ast.Ident)
		if !ok || key.Name != "string" {
			return false
		}
		value, ok := m.Value.(*ast.Ident)
		return ok && value.Name == "string"
	}
	// The shared static scopes and key resolver (round seven).
	r := newTraceDBKeyResolver(files, consts)

	// Same-package helpers and the per-function taint state (identifier
	// scoping is per function, sticky, shared with the function's closures).
	type fnState struct {
		scope     *traceDBFuncScope
		locals    map[string]string          // ident → key (sticky)
		maps      map[string]bool            // ident → bound to the state map / a map[string]string parameter
		producers map[string][]*ast.FuncDecl // ident → same-package function/method value bound to it
		results   map[int]string             // result position → key
		// mapResults: the result positions that return the state map (a bare
		// selector or a tainted map local), so a caller's binding to or index
		// of the call is the map (round six, #4).
		mapResults map[int]bool
	}
	states := map[*ast.FuncDecl]*fnState{}
	for _, fn := range r.order {
		st := &fnState{scope: r.scopes[fn], locals: map[string]string{}, maps: map[string]bool{}, producers: map[string][]*ast.FuncDecl{}, results: map[int]string{}, mapResults: map[int]bool{}}
		for _, field := range fn.Type.Params.List {
			if !isStringMapType(field.Type) {
				continue
			}
			for _, param := range field.Names {
				st.maps[param.Name] = true
			}
		}
		states[fn] = st
	}
	// producersOf: the same-package declarations a function or method VALUE
	// (not a call) names — a plain function by identifier, a local bound to
	// one, a method by selector name (an import-qualified selector never).
	producersOf := func(st *fnState, expr ast.Expr) []*ast.FuncDecl {
		switch f := traceDBStripParens(expr).(type) {
		case *ast.Ident:
			if decls, ok := st.producers[f.Name]; ok {
				return decls
			}
			if fn, ok := r.functions[f.Name]; ok {
				return []*ast.FuncDecl{fn}
			}
		case *ast.SelectorExpr:
			if x, ok := f.X.(*ast.Ident); ok && st.scope.imports[x.Name] {
				return nil
			}
			return r.methods[f.Sel.Name]
		}
		return nil
	}
	mergedResults := func(decls []*ast.FuncDecl) map[int]string {
		var merged map[int]string
		for _, decl := range decls {
			for i, key := range states[decl].results {
				if merged == nil {
					merged = map[int]string{}
				}
				merged[i] = key
			}
		}
		return merged
	}
	// calleeResults: the tainted result positions of a same-package helper
	// call (nil for anything the census cannot name).
	calleeResults := func(st *fnState, call *ast.CallExpr) map[int]string {
		return mergedResults(producersOf(st, call.Fun))
	}
	mergedMapResults := func(decls []*ast.FuncDecl) map[int]bool {
		var merged map[int]bool
		for _, decl := range decls {
			for i := range states[decl].mapResults {
				if merged == nil {
					merged = map[int]bool{}
				}
				merged[i] = true
			}
		}
		return merged
	}
	// calleeMapResults: the result positions of a same-package helper call
	// that return the state map (round six, #4).
	calleeMapResults := func(st *fnState, call *ast.CallExpr) map[int]bool {
		return mergedMapResults(producersOf(st, call.Fun))
	}
	// mapValue: a bare state map occurrence — the `.Metadata` selector, a
	// tainted map local, or a call whose first result is the map.
	mapValue := func(st *fnState, expr ast.Expr) bool {
		switch e := traceDBStripParens(expr).(type) {
		case *ast.SelectorExpr:
			return e.Sel.Name == "Metadata"
		case *ast.Ident:
			return st.maps[e.Name]
		case *ast.CallExpr:
			return calleeMapResults(st, e)[0]
		}
		return false
	}
	// readKind classifies an index expression for the census.
	type readKind int
	const (
		readNone       readKind = iota // not an index, no state key named, an every-key range, a carried key over something else
		readSeen                       // a state key over the state map
		readUnseen                     // a spelled state key over an expression that is not the state map
		readUnresolved                 // a key the census cannot resolve over the state map
	)
	// directRead: an index by a state key over the state map (the selector
	// or a tainted map local). A spelled key over anything else is a read
	// the census cannot see (readUnseen): the spelled key is the precise
	// signal. A carried key over anything else is not a read: a forwarding
	// loop's `out[k] = v` ranges over every key, a caller-resolved
	// parameter over some other map is that map's business. An unresolved
	// key over the state map is red (readUnresolved): the census cannot say
	// what was read.
	directRead := func(st *fnState, expr ast.Expr) (key string, kind readKind) {
		index, isIndex := traceDBStripParens(expr).(*ast.IndexExpr)
		if !isIndex {
			return "", readNone
		}
		keys, resolution := r.resolveKey(st.scope, index.Index, newTraceDBKeyPath())
		seen := mapValue(st, index.X)
		if resolution == traceDBKeyUnresolved {
			if seen {
				return "", readUnresolved
			}
			return "", readNone
		}
		for _, key := range keys {
			if !traceDBStateReadKeys[key] {
				continue
			}
			if seen {
				return key, readSeen
			}
			if resolution == traceDBKeySpelled {
				return key, readUnseen
			}
			return "", readNone
		}
		return "", readNone
	}
	isRead := func(kind readKind) bool { return kind == readSeen || kind == readUnseen }
	// taintOf: the key a single-valued expression carries, if any.
	var taintOf func(st *fnState, expr ast.Expr) (string, bool)
	taintOf = func(st *fnState, expr ast.Expr) (string, bool) {
		switch e := traceDBStripParens(expr).(type) {
		case *ast.IndexExpr:
			key, kind := directRead(st, e)
			return key, isRead(kind)
		case *ast.Ident:
			key, ok := st.locals[e.Name]
			return key, ok
		case *ast.CallExpr:
			if key, ok := calleeResults(st, e)[0]; ok {
				return key, true
			}
			if traceDBStateStringStdlib(exprText(e.Fun)) {
				for _, arg := range e.Args {
					if key, ok := taintOf(st, arg); ok {
						return key, true
					}
				}
			}
		case *ast.BinaryExpr:
			if e.Op == token.ADD {
				if key, ok := taintOf(st, e.X); ok {
					return key, true
				}
				return taintOf(st, e.Y)
			}
		}
		return "", false
	}
	// contains: whether expr holds a read occurrence anywhere.
	contains := func(st *fnState, expr ast.Expr) bool {
		found := false
		traceDBWalk(expr, func(node ast.Node, stack []ast.Node) {
			switch n := node.(type) {
			case *ast.IndexExpr:
				if _, kind := directRead(st, n); isRead(kind) {
					found = true
				}
			case *ast.Ident:
				if _, ok := st.locals[n.Name]; ok && !traceDBIdentIsName(n, traceDBNearest(stack)) {
					found = true
				}
			case *ast.CallExpr:
				if len(calleeResults(st, n)) > 0 {
					found = true
				}
			}
		})
		return found
	}
	// consumeShape: right-hand sides whose value is not the state itself and
	// whose read occurrences the classification pass judges in place — a
	// lookup, a call, a comparison, a literal the read is forwarded into.
	consumeShape := func(expr ast.Expr) bool {
		switch e := traceDBStripParens(expr).(type) {
		case *ast.IndexExpr, *ast.CallExpr, *ast.BinaryExpr, *ast.CompositeLit:
			return true
		case *ast.UnaryExpr:
			_, lit := e.X.(*ast.CompositeLit)
			return lit && e.Op == token.AND
		}
		return false
	}

	// Taint fixpoint over the package.
	bindingProblems := map[string]bool{}
	changed := true
	bindValue := func(st *fnState, lhs ast.Expr, key string) {
		ident, ok := lhs.(*ast.Ident)
		if !ok || ident.Name == "_" {
			return
		}
		if st.locals[ident.Name] == "" {
			st.locals[ident.Name] = key
			changed = true
		}
	}
	bindMap := func(st *fnState, lhs ast.Expr) {
		ident, ok := lhs.(*ast.Ident)
		if !ok || ident.Name == "_" {
			return
		}
		if !st.maps[ident.Name] {
			st.maps[ident.Name] = true
			changed = true
		}
	}
	bindProducers := func(st *fnState, lhs ast.Expr, decls []*ast.FuncDecl) {
		ident, ok := lhs.(*ast.Ident)
		if !ok || ident.Name == "_" {
			return
		}
		if _, bound := st.producers[ident.Name]; !bound {
			st.producers[ident.Name] = decls
			changed = true
		}
	}
	unfollowed := func(st *fnState, rhs ast.Expr) {
		if !contains(st, rhs) || consumeShape(rhs) {
			return
		}
		bindingProblems[fmt.Sprintf("%s:%d: unrecognized binding shape (%T) over a state read; the census cannot follow the bound value",
			st.scope.file, fset.Position(rhs.Pos()).Line, traceDBStripParens(rhs))] = true
	}
	bind := func(st *fnState, lhs, rhs ast.Expr) {
		if key, ok := taintOf(st, rhs); ok {
			bindValue(st, lhs, key)
			return
		}
		if mapValue(st, rhs) {
			bindMap(st, lhs)
			return
		}
		if decls := producersOf(st, rhs); len(decls) > 0 {
			bindProducers(st, lhs, decls)
			return
		}
		unfollowed(st, rhs)
	}
	bindTuple := func(st *fnState, lhs []ast.Expr, rhs ast.Expr) {
		if key, kind := directRead(st, rhs); isRead(kind) { // comma-ok over a direct read
			bindValue(st, lhs[0], key)
			return
		}
		if call, ok := traceDBStripParens(rhs).(*ast.CallExpr); ok {
			results, mapResults := calleeResults(st, call), calleeMapResults(st, call)
			if len(results) > 0 || len(mapResults) > 0 {
				for i, key := range results {
					if i < len(lhs) {
						bindValue(st, lhs[i], key)
					}
				}
				for i := range mapResults {
					if i < len(lhs) {
						bindMap(st, lhs[i])
					}
				}
				return
			}
		}
		unfollowed(st, rhs)
	}
	setResult := func(st *fnState, i int, key string) {
		if st.results[i] == "" {
			st.results[i] = key
			changed = true
		}
	}
	setMapResult := func(st *fnState, i int) {
		if !st.mapResults[i] {
			st.mapResults[i] = true
			changed = true
		}
	}
	for changed {
		changed = false
		for _, fn := range r.order {
			st := states[fn]
			traceDBWalk(fn.Body, func(node ast.Node, stack []ast.Node) {
				switch n := node.(type) {
				case *ast.FuncLit:
					for _, field := range n.Type.Params.List {
						if !isStringMapType(field.Type) {
							continue
						}
						for _, param := range field.Names {
							bindMap(st, param)
						}
					}
				case *ast.RangeStmt:
					// A compared range key (resolved statically by the shared
					// scope) taints the range value with its key; an uncompared
					// range over a tainted map local or a map-returning helper
					// call ranges over every key.
					key, ok := n.Key.(*ast.Ident)
					if !ok || key.Name == "_" {
						return
					}
					state, compared := st.scope.comparedRanges[n]
					if !compared {
						if mapValue(st, n.X) && !st.scope.everyKey[key.Name] {
							st.scope.everyKey[key.Name] = true
							changed = true
						}
						return
					}
					if n.Value != nil {
						bindValue(st, n.Value, state)
					}
				case *ast.AssignStmt:
					switch {
					case len(n.Lhs) == len(n.Rhs):
						for i := range n.Lhs {
							bind(st, n.Lhs[i], n.Rhs[i])
						}
					case len(n.Rhs) == 1:
						bindTuple(st, n.Lhs, n.Rhs[0])
					}
				case *ast.ValueSpec:
					names := make([]ast.Expr, len(n.Names))
					for i, name := range n.Names {
						names[i] = name
					}
					switch {
					case len(n.Names) == len(n.Values):
						for i := range n.Names {
							bind(st, names[i], n.Values[i])
						}
					case len(n.Values) == 1:
						bindTuple(st, names, n.Values[0])
					}
				case *ast.ReturnStmt:
					for _, ancestor := range stack {
						if _, closure := ancestor.(*ast.FuncLit); closure {
							return // a closure's return is not the helper's
						}
					}
					if fn.Type.Results == nil {
						return
					}
					// A result position returning a read taints the callers'
					// bindings; one returning the state map (round six, #4)
					// makes the callers' bindings and direct indexes the map.
					switch {
					case len(n.Results) == 0: // bare return over named results
						position := 0
						for _, field := range fn.Type.Results.List {
							for _, name := range field.Names {
								if key, ok := st.locals[name.Name]; ok {
									setResult(st, position, key)
								}
								if st.maps[name.Name] {
									setMapResult(st, position)
								}
								position++
							}
						}
					case len(n.Results) == 1 && fn.Type.Results.NumFields() > 1: // forwarded tuple
						if call, ok := traceDBStripParens(n.Results[0]).(*ast.CallExpr); ok {
							for i, key := range calleeResults(st, call) {
								setResult(st, i, key)
							}
							for i := range calleeMapResults(st, call) {
								setMapResult(st, i)
							}
						}
					default:
						for i, result := range n.Results {
							if key, ok := taintOf(st, result); ok {
								setResult(st, i, key)
							} else if mapValue(st, result) {
								setMapResult(st, i)
							}
						}
					}
				}
			})
		}
	}
	for problem := range bindingProblems {
		problems = append(problems, problem)
	}

	// Classification.
	// boundToLocal reports whether node is a right-hand side bound to a local
	// identifier (or a declared name) under parent — the one position a
	// map occurrence or a producer value is followed from.
	boundToLocal := func(node ast.Node, parent ast.Node) (ast.Expr, bool) {
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
			return nil, true
		}
		return nil, false
	}
	// isLHS: a plain write target (`=` / `:=`); an op-assign target reads
	// the old value and is judged as a read.
	isLHS := func(node ast.Node, parent ast.Node) bool {
		p, ok := parent.(*ast.AssignStmt)
		if !ok || (p.Tok != token.ASSIGN && p.Tok != token.DEFINE) {
			return false
		}
		for _, lhs := range p.Lhs {
			if lhs == node {
				return true
			}
		}
		return false
	}
	// closureReturn: whether a return under node's ancestors belongs to a
	// closure (the innermost function literal) the census cannot follow —
	// one that is not bound to a local, where the binding arm already
	// reports it (an invoked literal, an argument, a literal element…).
	closureReturn := func(stack []ast.Node) bool {
		for i := len(stack) - 1; i >= 0; i-- {
			if _, closure := stack[i].(*ast.FuncLit); !closure {
				continue
			}
			var parent ast.Node
			for j := i - 1; j >= 0; j-- {
				if _, paren := stack[j].(*ast.ParenExpr); !paren {
					parent = stack[j]
					break
				}
			}
			_, bound := boundToLocal(stack[i], parent)
			return !bound
		}
		return false
	}
	isNil := func(expr ast.Expr) bool {
		ident, ok := traceDBStripParens(expr).(*ast.Ident)
		return ok && ident.Name == "nil"
	}
	// classifyMap judges a bare state map occurrence (a selector, a tainted
	// local, a helper call returning the map) by its parent: an indexed
	// occurrence is judged at the index, a ranged one through its key
	// comparison (an uncompared range carries no key signal); a write target,
	// a binding to a local or to another `.Metadata` field, a call argument
	// (a same-package map[string]string parameter is tainted by type; the
	// map leaving the package is a disclosed residual), a nil comparison, a
	// literal element, a return, a field/method selection (a witness the
	// value is not the string map) and a discarded call result are green;
	// every other shape is red.
	classifyMap := func(name string, node ast.Node, parent ast.Node) {
		if isLHS(node, parent) {
			return
		}
		switch p := parent.(type) {
		case *ast.IndexExpr:
			if p.X == node || traceDBStripParens(p.X) == node {
				return
			}
		case *ast.RangeStmt:
			if p.X == node || traceDBStripParens(p.X) == node {
				return
			}
		case *ast.AssignStmt:
			lhs, ok := boundToLocal(node, parent)
			if !ok {
				break
			}
			if _, ident := lhs.(*ast.Ident); ident {
				return
			}
			if sel, isSel := lhs.(*ast.SelectorExpr); isSel && sel.Sel.Name == "Metadata" {
				return
			}
			report(name, node, "state map bound to a %T the census cannot follow", lhs)
			return
		case *ast.ValueSpec, *ast.CallExpr, *ast.KeyValueExpr, *ast.CompositeLit, *ast.ReturnStmt, *ast.ExprStmt:
			return
		case *ast.BinaryExpr:
			if (p.Op == token.EQL || p.Op == token.NEQ) && (isNil(p.X) || isNil(p.Y)) {
				return
			}
		case *ast.SelectorExpr:
			if p.X == node || traceDBStripParens(p.X) == node {
				return
			}
		}
		report(name, node, "unrecognized state map shape (%T); the census cannot follow the map", parent)
	}
	// classifyMapCall judges a helper call whose result positions return the
	// state map: under a tuple binding each map position is judged as a
	// binding (a local is followed, anything else cannot be); everywhere
	// else the call is a bare map occurrence.
	classifyMapCall := func(name string, node ast.Node, parent ast.Node, mapResults map[int]bool) {
		if p, ok := parent.(*ast.AssignStmt); ok && len(p.Rhs) == 1 && len(p.Lhs) > 1 {
			for i := range mapResults {
				if i >= len(p.Lhs) {
					continue
				}
				if _, ident := p.Lhs[i].(*ast.Ident); !ident {
					report(name, node, "state map bound to a %T the census cannot follow", p.Lhs[i])
				}
			}
			return
		}
		classifyMap(name, node, parent)
	}
	// classifyProducer judges a bare function/method value of a tainted
	// producer: bound to a local it is followed into the local's calls;
	// everywhere else the census cannot follow the value.
	classifyProducer := func(name string, node ast.Node, parent ast.Node, key string) {
		if lhs, ok := boundToLocal(node, parent); ok {
			if lhs == nil {
				return
			}
			if _, ident := lhs.(*ast.Ident); ident {
				return
			}
		}
		report(name, node, "function value of a %s producer in a %T; the census cannot follow the value", key, parent)
	}
	firstResult := func(results map[int]string) string {
		positions := make([]int, 0, len(results))
		for i := range results {
			positions = append(positions, i)
		}
		sort.Ints(positions)
		return results[positions[0]]
	}
	for _, fn := range r.order {
		st := states[fn]
		name := st.scope.file
		traceDBWalk(fn.Body, func(node ast.Node, stack []ast.Node) {
			parent := traceDBNearest(stack)
			var key string
			leaf := true
			switch n := node.(type) {
			case *ast.IndexExpr:
				if isLHS(node, parent) {
					return // a write target is the write census's business, not a read
				}
				k, kind := directRead(st, n)
				switch kind {
				case readNone:
					return
				case readUnresolved:
					report(name, node, "state key the census cannot resolve over the state map (%s); readers spell the state key", exprText(n.Index))
					return
				case readUnseen:
					report(name, node, "state key %s indexed over a %T the census cannot see; readers index the state map", k, traceDBStripParens(n.X))
					return
				}
				key = k
			case *ast.SelectorExpr:
				if n.Sel.Name == "Metadata" {
					classifyMap(name, node, parent)
					return
				}
				if traceDBIsCallFun(node, parent) || isLHS(node, parent) {
					return
				}
				results := mergedResults(producersOf(st, n))
				if len(results) == 0 {
					return
				}
				reads++
				classifyProducer(name, node, parent, firstResult(results))
				return
			case *ast.Ident:
				if traceDBIdentIsName(n, parent) {
					return
				}
				if k, ok := st.locals[n.Name]; ok {
					key = k
					break
				}
				if st.maps[n.Name] {
					classifyMap(name, node, parent)
					return
				}
				if traceDBIsCallFun(node, parent) {
					return
				}
				results := mergedResults(producersOf(st, n))
				if len(results) == 0 {
					return
				}
				reads++
				classifyProducer(name, node, parent, firstResult(results))
				return
			case *ast.CallExpr:
				if results := calleeResults(st, n); len(results) > 0 {
					key = firstResult(results)
					break
				}
				if mapResults := calleeMapResults(st, n); len(mapResults) > 0 {
					classifyMapCall(name, node, parent, mapResults)
					return
				}
				k, ok := "", false
				if traceDBStateSprint(exprText(n.Fun)) {
					k, ok = taintOf(st, n)
				}
				if !ok {
					return
				}
				key, leaf = k, false
			case *ast.BinaryExpr:
				k, ok := "", false
				if n.Op == token.ADD {
					k, ok = taintOf(st, n)
				}
				if !ok {
					return
				}
				key, leaf = k, false
			default:
				return
			}
			if leaf {
				reads++
			}
			// Laundered through a concatenation or a fmt.Sprint* argument:
			// judged where that expression lands.
			switch p := parent.(type) {
			case *ast.BinaryExpr:
				if p.Op == token.ADD {
					return
				}
			case *ast.CallExpr:
				if traceDBStateSprint(exprText(p.Fun)) {
					for _, arg := range p.Args {
						if arg == node {
							return
						}
					}
				}
			}
			switch p := parent.(type) {
			case *ast.AssignStmt:
				// binding, forwarding into a field/metadata; an op-assign
				// target is read back through an operator the census
				// cannot classify
				if p.Tok != token.ASSIGN && p.Tok != token.DEFINE {
					report(name, node, "unrecognized read shape (%s) over %s", p.Tok, key)
				}
			case *ast.ReturnStmt:
				// a helper's return is followed into its callers; a
				// closure's return is not the helper's (fold-in #8, round
				// five: the invoked / passed / stored literal's value is
				// judged nowhere)
				if closureReturn(stack) {
					report(name, node, "read returned from a closure the census cannot follow (%s)", key)
				}
			case *ast.ValueSpec, *ast.KeyValueExpr, *ast.CompositeLit:
				// binding, forwarding into a literal
			case *ast.IndexExpr:
				if p.Index != node && traceDBStripParens(p.Index) != node {
					report(name, node, "unrecognized read shape (indexed) over %s", key)
					return
				}
				table, ok := p.X.(*ast.Ident)
				if !ok || !tables[table.Name] {
					report(name, node, "lookup over %s through a map the census cannot see; readers classify through a package-level table keyed by the decode_state constants", key)
				}
			case *ast.CallExpr:
				fun := exprText(p.Fun)
				switch {
				case strings.HasPrefix(fun, "strings."):
					report(name, node, "prefix/substring classification over %s (%s); readers classify through the gate table", key, fun)
				case traceDBStateReadClassifiers[fun], traceDBStateReadSinks[fun], strings.HasPrefix(fun, "fmt."):
				default:
					report(name, node, "unrecognized reader call %s over %s", fun, key)
				}
			case *ast.SwitchStmt:
				report(name, node, "hand-kept switch over %s; readers classify through the gate table", key)
			case *ast.CaseClause:
				report(name, node, "hand-kept case over %s; readers classify through the gate table", key)
			case *ast.BinaryExpr:
				switch p.Op {
				case token.EQL, token.NEQ:
					report(name, node, "literal comparison over %s; readers classify through the gate table", key)
				default:
					report(name, node, "unrecognized read shape (%s) over %s", p.Op, key)
				}
			default:
				report(name, node, "unrecognized read shape (%T) over %s", parent, key)
			}
		})
	}
	sort.Strings(problems)
	return problems, reads
}

func exprText(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprText(e.X) + "." + e.Sel.Name
	case *ast.ParenExpr:
		return exprText(e.X)
	default:
		return fmt.Sprintf("<%T>", expr)
	}
}

// traceDBCensusFilesWithProbe parses the package's non-test files plus one
// synthetic probe file (same FileSet) and resolves the constants over both.
func traceDBCensusFilesWithProbe(t *testing.T, probe string) (map[string]string, map[string]*ast.File, *token.FileSet) {
	t.Helper()
	files, fset := traceDBParsePackageFiles(t)
	file, err := parser.ParseFile(fset, "zz_probe.go", probe, 0)
	if err != nil {
		t.Fatal(err)
	}
	files["zz_probe.go"] = file
	return traceDBStringConstsOf(files), files, fset
}

func traceDBProbeProblems(problems []string) []string {
	var out []string
	for _, problem := range problems {
		if strings.HasPrefix(problem, "zz_probe.go:") {
			out = append(out, problem)
		}
	}
	return out
}

// TestTraceDBUngatedLaneKeyWritersSelfRed: the writer-gate rule bites the
// ungated eighth-writer shape (fold-in #7 at 381f36cc9) through the real
// write-site census, and is satisfied by an inherit or an apply under the
// same key — never by a gate call under a different key in the same file.
func TestTraceDBUngatedLaneKeyWritersSelfRed(t *testing.T) {
	const header = "package hitraceconv\n"
	for _, test := range []struct {
		name string
		src  string
		want string
	}{
		{name: "ungated_consumer", src: header + `
func zzUngated(join TraceDBCoverage) TraceDBCoverage {
	out := TraceDBCoverage{Metadata: map[string]string{"reconciliation_state": traceDBSourceRawLanePlaceholderState}}
	if join.Metrics["raw_records_retained"] == 0 {
		out.Metadata["reconciliation_state"] = "complete_exact_raw_record_closure"
	}
	return out
}
`, want: "zz_probe.go:6: zzUngated publishes reconciliation_state without running or inheriting the class gate for that key in this file"},
		{name: "gated_under_another_key", src: header + `
func zzOtherKey(join TraceDBCoverage, inventory *traceDBSourceNameInventory) TraceDBCoverage {
	out := TraceDBCoverage{Metadata: map[string]string{"reconciliation_state": traceDBSourceRawLanePlaceholderState}}
	if stop, _ := traceDBApplySourceRawLaneGateKeyed(&out, inventory, traceDBSourceRawLaneStateKeyJoin, "probe"); stop {
		return out
	}
	out.Metadata["reconciliation_state"] = "complete_exact_raw_record_closure"
	return out
}
`, want: "zz_probe.go:8: zzOtherKey publishes reconciliation_state without running or inheriting the class gate for that key in this file"},
		{name: "inherits", src: header + `
func zzInherits(join TraceDBCoverage) TraceDBCoverage {
	out := TraceDBCoverage{Metadata: map[string]string{"reconciliation_state": traceDBSourceRawLanePlaceholderState}}
	if traceDBInheritSourceRawLaneGate(&out, join, traceDBSourceRawLaneStateKeyJoin, traceDBSourceRawLaneStateKeyReconciliation, "probe") {
		return out
	}
	out.Metadata["reconciliation_state"] = "complete_exact_raw_record_closure"
	return out
}
`},
		{name: "applies", src: header + `
func zzApplies(inventory *traceDBSourceNameInventory) TraceDBCoverage {
	out := TraceDBCoverage{Metadata: map[string]string{"reconciliation_state": traceDBSourceRawLanePlaceholderState}}
	if stop, _ := traceDBApplySourceRawLaneGateKeyed(&out, inventory, traceDBSourceRawLaneStateKeyReconciliation, "probe"); stop {
		return out
	}
	out.Metadata["reconciliation_state"] = "complete_exact_raw_record_closure"
	return out
}
`},
	} {
		t.Run(test.name, func(t *testing.T) {
			consts, files, fset := traceDBCensusFilesWithProbe(t, test.src)
			sites, _ := traceDBLaneStateWriteSitesOf(t, consts, files, fset)
			got := traceDBProbeProblems(traceDBUngatedLaneKeyWriters(sites))
			if test.want == "" {
				if len(got) != 0 {
					t.Fatalf("gated probe reported: %v", got)
				}
				return
			}
			if len(got) != 1 || got[0] != test.want {
				t.Fatalf("problems=%v\nwant %q", got, test.want)
			}
		})
	}
}

// TestTraceDBLaneStateWriteSitesFailLoudOnMultiValuedKey (round six, #3):
// the write census fails loud on a value written under a local key the
// binding helper cannot resolve — tuple-bound, re-bound to another value —
// the way it fails on its other unresolvable shapes; a single-valued local
// key and the state-shaped arm (which speaks first) are the controls.
// Through 6f98f839d a tuple re-binding left the name at its first value and
// the write minted nothing (see the EVOLUTION RECORD on
// TestTraceDBStateReadTupleMapScopeCycleShapesEvadedTheBaseCensus).
// EVOLUTION RECORD (round seven, #5): through 42fcf3fd1 the arm was gated on
// the value — a value the census could not resolve under the same
// unresolvable key was a green control; the key is the precise signal, so
// that shape is red now (tuple_key_unresolvable_value).
func TestTraceDBLaneStateWriteSitesFailLoudOnMultiValuedKey(t *testing.T) {
	const header = "package hitraceconv\nfunc zzPick() (string, bool) { return \"publication_state\", true }\nfunc zzFormat() string { return \"x\" }\n"
	for _, test := range []struct {
		name string
		src  string
		want []string
	}{
		{name: "tuple_rebound_key", src: header + `
func zz(out *TraceDBCoverage) { k := "reason"; k, _ = zzPick(); out.Metadata[k] = "strict_target_ledger_complete" }
`, want: []string{`zz_probe.go:5: "strict_target_ledger_complete" written under a Metadata key the census cannot resolve (k)`}},
		{name: "tuple_only_key", src: header + `
func zz(out *TraceDBCoverage) { k, _ := zzPick(); out.Metadata[k] = "strict_target_ledger_complete" }
`, want: []string{`zz_probe.go:5: "strict_target_ledger_complete" written under a Metadata key the census cannot resolve (k)`}},
		{name: "rebound_to_another_value", src: header + `
func zz(out *TraceDBCoverage) { k := "reason"; k = "decode_state"; out.Metadata[k] = "strict_target_ledger_complete" }
`, want: []string{`zz_probe.go:5: "strict_target_ledger_complete" written under a Metadata key the census cannot resolve (k)`}},
		{name: "state_shaped_value_speaks_first", src: header + `
func zz(out *TraceDBCoverage) { k, _ := zzPick(); out.Metadata[k] = "complete_exact_raw_record_closure" }
`, want: []string{`zz_probe.go:5: state-shaped value "complete_exact_raw_record_closure" written under a computed Metadata key`}},
		{name: "tuple_key_unresolvable_value", src: header + `
func zz(out *TraceDBCoverage) { k, _ := zzPick(); out.Metadata[k] = zzFormat() }
`, want: []string{`zz_probe.go:5: a value the census cannot resolve written under a Metadata key the census cannot resolve (k)`}},
		{name: "controls", src: header + `
func zzSingle(out *TraceDBCoverage) { k := "reason"; out.Metadata[k] = "strict_target_ledger_complete" }
`},
	} {
		t.Run(test.name, func(t *testing.T) {
			consts, files, fset := traceDBCensusFilesWithProbe(t, test.src)
			recorder := &traceDBRecordingTB{TB: t}
			traceDBLaneStateWriteSitesOf(recorder, consts, files, fset)
			got := traceDBProbeProblems(recorder.problems)
			if strings.Join(got, "\n") != strings.Join(test.want, "\n") {
				t.Fatalf("problems=\n%s\nwant=\n%s", strings.Join(got, "\n"), strings.Join(test.want, "\n"))
			}
		})
	}
}

// TestTraceDBStateReadersSelfRed: each evasion shape of fold-in #9 is red
// through the real parser, the green shapes (table lookup, registered
// classifier, binding, forwarding, prose, return) are not.
func TestTraceDBStateReadersSelfRed(t *testing.T) {
	const header = "package hitraceconv\nimport (\n\t\"fmt\"\n\t\"strings\"\n)\nvar _ = fmt.Sprintf\nvar _ = strings.HasPrefix\n"
	for _, test := range []struct {
		name string
		src  string
		want []string
	}{
		{name: "local_switch", src: header + `
func zz(c TraceDBCoverage) bool { s := c.Metadata["decode_state"]; switch s { case "strict_target_ledger_complete": return true }; return false }
`, want: []string{"zz_probe.go:9: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "prefix", src: header + `
func zz(c TraceDBCoverage) bool { return strings.HasPrefix(c.Metadata["decode_state"], "withheld_") }
`, want: []string{"zz_probe.go:9: prefix/substring classification over decode_state (strings.HasPrefix); readers classify through the gate table"}},
		{name: "literal_keyed_map", src: header + `
func zz(c TraceDBCoverage) bool { return map[string]bool{"strict_target_ledger_complete": true}[c.Metadata["decode_state"]] }
`, want: []string{"zz_probe.go:9: lookup over decode_state through a map the census cannot see; readers classify through a package-level table keyed by the decode_state constants"}},
		{name: "local_table", src: header + `
func zz(c TraceDBCoverage) bool { m := map[string]bool{traceDBRawDecodeStateComplete: true}; return m[c.Metadata["decode_state"]] }
`, want: []string{"zz_probe.go:9: lookup over decode_state through a map the census cannot see; readers classify through a package-level table keyed by the decode_state constants"}},
		{name: "local_comparison", src: header + `
func zz(c TraceDBCoverage) bool { s := c.Metadata["decode_state"]; return s == traceDBRawDecodeStateComplete }
`, want: []string{"zz_probe.go:9: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "transitive_taint_case", src: header + `
func zz(c TraceDBCoverage, other string) bool { s := c.Metadata["decode_state"]; u := s; switch other { case u: return true }; return false }
`, want: []string{"zz_probe.go:9: hand-kept case over decode_state; readers classify through the gate table"}},
		{name: "relaundered_binding", src: header + `
func zz(c TraceDBCoverage) bool { s := c.Metadata["publication_state"]; s = "x"; switch s { case "y": return true }; return false }
`, want: []string{"zz_probe.go:9: hand-kept switch over publication_state; readers classify through the gate table"}},
		{name: "unrecognized_call", src: header + `
func zz(c TraceDBCoverage) string { s := c.Metadata["decode_state"]; return strings.ToUpper(s) }
`, want: []string{"zz_probe.go:9: prefix/substring classification over decode_state (strings.ToUpper); readers classify through the gate table"}},
		{name: "unregistered_helper", src: header + `
func zzHelper(s string) bool { return s != "" }
func zz(c TraceDBCoverage) bool { return zzHelper(c.Metadata["decode_state"]) }
`, want: []string{"zz_probe.go:10: unrecognized reader call zzHelper over decode_state"}},
		{name: "constant_key_read", src: header + `
func zz(c TraceDBCoverage) bool { switch c.Metadata[string(traceDBSourceRawLaneStateKeyPublication)] { case "x": return true }; return false }
`, want: []string{"zz_probe.go:9: hand-kept switch over publication_state; readers classify through the gate table"}},
		{name: "green_shapes", src: header + `
func zzTable(c TraceDBCoverage) traceDBSourceRawGateKind { return traceDBRawDecodeStateGates[c.Metadata["decode_state"]] }
func zzLocalTable(c TraceDBCoverage) traceDBSourceRawGateKind { s := c.Metadata["decode_state"]; return traceDBRawDecodeStateGates[s] }
func zzClassifier(c TraceDBCoverage) bool { return traceDBSourceRawPublicationStateBlocksEvaluation(c.Metadata["publication_state"]) }
func zzForward(c *TraceDBCoverage) { c.Metadata["reason"] = c.Metadata["decode_state"] }
func zzReturn(c TraceDBCoverage) string { s := c.Metadata["decode_state"]; return s }
func zzProse(c TraceDBCoverage) string { s := c.Metadata["decode_state"]; return "census incomplete (" + s + ")" + fmt.Sprintf("%s", s) }
func zzLiteral(c TraceDBCoverage) map[string]string { return map[string]string{"census_incomplete_reason": c.Metadata["decode_state"]} }
`},
		// Fold-in #8: laundering a read through an expression before a
		// hand-kept consumer. Each shape stayed green at 480939385 (taint
		// only through single-identifier bindings); see the EVOLUTION RECORD
		// on TestTraceDBStateReadLaunderShapesEvadedTheBaseCensus.
		{name: "concat_launder", src: header + `
func zz(c TraceDBCoverage) bool { s := c.Metadata["decode_state"] + ""; switch s { case "x": return true }; return false }
`, want: []string{"zz_probe.go:9: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "sprint_launder", src: header + `
func zz(c TraceDBCoverage) bool { s := fmt.Sprint(c.Metadata["decode_state"]); switch s { case "x": return true }; return false }
`, want: []string{"zz_probe.go:9: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "sprint_comparison", src: header + `
func zz(c TraceDBCoverage) bool { return fmt.Sprintf("%s", c.Metadata["decode_state"]) == "x" }
`, want: []string{"zz_probe.go:9: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "concat_into_unregistered_call", src: header + `
func zzUse(s string) bool { return s != "" }
func zz(c TraceDBCoverage) bool { return zzUse("x" + c.Metadata["decode_state"]) }
`, want: []string{"zz_probe.go:10: unrecognized reader call zzUse over decode_state"}},
		{name: "strings_launder_then_switch", src: header + `
func zz(c TraceDBCoverage) bool { s := strings.TrimSpace(c.Metadata["decode_state"]); switch s { case "x": return true }; return false }
`, want: []string{
			"zz_probe.go:9: hand-kept switch over decode_state; readers classify through the gate table",
			"zz_probe.go:9: prefix/substring classification over decode_state (strings.TrimSpace); readers classify through the gate table",
		}},
		{name: "return_helper_switch", src: header + `
func zzState(c TraceDBCoverage) string { return c.Metadata["decode_state"] }
func zz(c TraceDBCoverage) bool { switch zzState(c) { case "x": return true }; return false }
`, want: []string{"zz_probe.go:10: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "comma_ok_launder", src: header + `
func zz(c TraceDBCoverage) bool { s, ok := c.Metadata["decode_state"]; if !ok { return false }; switch s { case "x": return true }; return false }
`, want: []string{"zz_probe.go:9: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "multi_result_helper", src: header + `
func zzState(c TraceDBCoverage) (string, bool) { s, ok := c.Metadata["publication_state"]; return s, ok }
func zz(c TraceDBCoverage) bool { s, _ := zzState(c); return s == "x" }
`, want: []string{"zz_probe.go:10: literal comparison over publication_state; readers classify through the gate table"}},
		{name: "forwarded_tuple_helper", src: header + `
func zzInner(c TraceDBCoverage) (string, bool) { return c.Metadata["decode_state"], true }
func zzOuter(c TraceDBCoverage) (string, bool) { return zzInner(c) }
func zz(c TraceDBCoverage) bool { s, _ := zzOuter(c); switch s { case "x": return true }; return false }
`, want: []string{"zz_probe.go:11: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "named_result_helper", src: header + `
func zzState(c TraceDBCoverage) (s string) { s = c.Metadata["decode_state"]; return }
func zz(c TraceDBCoverage) bool { return strings.HasPrefix(zzState(c), "withheld_") }
`, want: []string{"zz_probe.go:10: prefix/substring classification over decode_state (strings.HasPrefix); readers classify through the gate table"}},
		{name: "method_helper", src: header + `
type zzView struct{ c TraceDBCoverage }
func (v zzView) state() string { return v.c.Metadata["decode_state"] }
func zz(v zzView) bool { switch v.state() { case "x": return true }; return false }
`, want: []string{"zz_probe.go:11: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "closure_binding", src: header + `
func zz(c TraceDBCoverage) bool { st := func() string { return c.Metadata["decode_state"] }; switch st() { case "x": return true }; return false }
`, want: []string{"zz_probe.go:9: unrecognized binding shape (*ast.FuncLit) over a state read; the census cannot follow the bound value"}},
		{name: "slice_binding", src: header + `
func zz(c TraceDBCoverage) bool { s := c.Metadata["decode_state"][1:]; return s == "x" }
`, want: []string{
			"zz_probe.go:9: unrecognized binding shape (*ast.SliceExpr) over a state read; the census cannot follow the bound value",
			"zz_probe.go:9: unrecognized read shape (*ast.SliceExpr) over decode_state",
		}},
		{name: "laundered_green_shapes", src: header + `
func zzProseBound(c TraceDBCoverage) string { s := fmt.Sprintf("state=%s", c.Metadata["decode_state"]); return s }
func zzConcatForward(c *TraceDBCoverage) { r := "not_evaluated_" + c.Metadata["publication_state"]; c.Metadata["reason"] = r }
func zzKind(c TraceDBCoverage) traceDBSourceRawGateKind { return traceDBRawDecodeStateGates[c.Metadata["decode_state"]] }
func zzKindSwitch(c TraceDBCoverage) bool { switch zzKind(c) { case traceDBSourceRawGateReady: return true }; return false }
func zzGateReason(c TraceDBCoverage) (traceDBSourceRawGateKind, string) { s := c.Metadata["decode_state"]; return traceDBRawDecodeStateGates[s], s }
func zzFunnel(out *TraceDBCoverage, c TraceDBCoverage) { kind, reason := zzGateReason(c); traceDBMintSourceRawLaneGateOutcome(out, traceDBSourceRawLaneStateKeyPublication, "probe", kind, reason) }
func zzTupleTable(c TraceDBCoverage) bool { label, ok := traceDBRawDecoderAuthorityByDecodeState[c.Metadata["decode_state"]]; return ok && label.DecodeAuthority != "" }
func zzErrorf(c TraceDBCoverage) error { return fmt.Errorf("state %s", c.Metadata["decode_state"]) }
func zzParamOnly(state string) bool { switch state { case "x": return true }; return false }
`},
		// Fold-in #8, round four: the state map itself, range keys, and
		// function/method values of producers. Each shape stayed green at
		// 480939385 and b6f7eeec3 (a read had to spell `<x>.Metadata[<key>]`
		// and a call had to name a function or a method); see the EVOLUTION
		// RECORD on TestTraceDBStateReadMapAndValueShapesEvadedTheBaseCensus.
		{name: "alias_map_switch", src: header + `
func zz(c TraceDBCoverage) bool { m := c.Metadata; s := m["decode_state"]; switch s { case "x": return true }; return false }
`, want: []string{"zz_probe.go:9: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "alias_map_comparison", src: header + `
func zz(c TraceDBCoverage) bool { m := c.Metadata; return m["publication_state"] == "x" }
`, want: []string{"zz_probe.go:9: literal comparison over publication_state; readers classify through the gate table"}},
		{name: "alias_chain_prefix", src: header + `
func zz(c TraceDBCoverage) bool { m := c.Metadata; n := m; return strings.HasPrefix(n["decode_state"], "withheld_") }
`, want: []string{"zz_probe.go:9: prefix/substring classification over decode_state (strings.HasPrefix); readers classify through the gate table"}},
		{name: "map_param_helper", src: header + `
func zzSt(md map[string]string) string { return md["decode_state"] }
func zz(c TraceDBCoverage) bool { switch zzSt(c.Metadata) { case "x": return true }; return false }
`, want: []string{"zz_probe.go:10: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "map_param_switch_inside", src: header + `
func zzSt(md map[string]string) bool { switch md["publication_state"] { case "x": return true }; return false }
`, want: []string{"zz_probe.go:9: hand-kept switch over publication_state; readers classify through the gate table"}},
		{name: "map_param_closure", src: header + `
func zz(c TraceDBCoverage) bool { st := func(md map[string]string) string { return md["decode_state"] }; return st(c.Metadata) == "x" }
`, want: []string{"zz_probe.go:9: unrecognized binding shape (*ast.FuncLit) over a state read; the census cannot follow the bound value"}},
		{name: "range_key_comparison", src: header + `
func zz(c TraceDBCoverage) bool { for k, v := range c.Metadata { if k == "decode_state" { switch v { case "x": return true } } }; return false }
`, want: []string{"zz_probe.go:9: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "range_key_switch_index", src: header + `
func zz(c TraceDBCoverage) bool { for k := range c.Metadata { switch k { case "publication_state": return c.Metadata[k] == "x" } }; return false }
`, want: []string{"zz_probe.go:9: literal comparison over publication_state; readers classify through the gate table"}},
		{name: "range_key_flipped_neq_alias", src: header + `
func zz(c TraceDBCoverage) bool { m := c.Metadata; for k, v := range m { if "decode_state" != k { continue }; return strings.HasPrefix(v, "withheld_") }; return false }
`, want: []string{"zz_probe.go:9: prefix/substring classification over decode_state (strings.HasPrefix); readers classify through the gate table"}},
		{name: "range_key_constant_case", src: header + `
func zz(c TraceDBCoverage) bool { for k, v := range c.Metadata { switch k { case string(traceDBSourceRawLaneStateKeyPublication): return v == "x" } }; return false }
`, want: []string{"zz_probe.go:9: literal comparison over publication_state; readers classify through the gate table"}},
		{name: "method_value", src: header + `
type zzView struct{ c TraceDBCoverage }
func (v zzView) state() string { return v.c.Metadata["decode_state"] }
func zz(v zzView) bool { f := v.state; switch f() { case "x": return true }; return false }
`, want: []string{"zz_probe.go:11: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "function_value", src: header + `
func zzState(c TraceDBCoverage) string { return c.Metadata["decode_state"] }
func zz(c TraceDBCoverage) bool { f := zzState; switch f(c) { case "x": return true }; return false }
`, want: []string{"zz_probe.go:10: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "function_value_rebound_tuple", src: header + `
func zzState(c TraceDBCoverage) (string, bool) { s, ok := c.Metadata["publication_state"]; return s, ok }
func zz(c TraceDBCoverage) bool { f := zzState; g := f; s, _ := g(c); return s == "x" }
`, want: []string{"zz_probe.go:10: literal comparison over publication_state; readers classify through the gate table"}},
		{name: "function_value_argument", src: header + `
func zzState(c TraceDBCoverage) string { return c.Metadata["decode_state"] }
func zzApply(c TraceDBCoverage, f func(TraceDBCoverage) string) bool { return f(c) == "x" }
func zz(c TraceDBCoverage) bool { return zzApply(c, zzState) }
`, want: []string{"zz_probe.go:11: function value of a decode_state producer in a *ast.CallExpr; the census cannot follow the value"}},
		{name: "method_value_in_literal", src: header + `
type zzView struct{ c TraceDBCoverage }
func (v zzView) state() string { return v.c.Metadata["decode_state"] }
func zz(v zzView) []func() string { return []func() string{v.state} }
`, want: []string{"zz_probe.go:11: function value of a decode_state producer in a *ast.CompositeLit; the census cannot follow the value"}},
		// EVOLUTION RECORD (round six, #4): through 6f98f839d the helper's
		// map result carried no taint, so the spelled key over the call was
		// red as "indexed over a *ast.CallExpr the census cannot see"; the
		// call is now the map and the comparison is judged as any read.
		{name: "state_key_over_unseen_map", src: header + `
func zzMd(c TraceDBCoverage) map[string]string { return c.Metadata }
func zz(c TraceDBCoverage) bool { return zzMd(c)["decode_state"] == "x" }
`, want: []string{"zz_probe.go:10: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "state_key_over_untainted_local", src: header + `
func zz(c TraceDBCoverage) bool { m := map[string]string{}; return m[string(traceDBSourceRawLaneStateKeyPublication)] == "x" }
`, want: []string{"zz_probe.go:9: state key publication_state indexed over a *ast.Ident the census cannot see; readers index the state map"}},
		{name: "map_bound_to_field", src: header + `
type zzBox struct{ md map[string]string }
func zz(c TraceDBCoverage, b *zzBox) { b.md = c.Metadata }
`, want: []string{"zz_probe.go:10: state map bound to a *ast.SelectorExpr the census cannot follow"}},
		{name: "map_address", src: header + `
func zz(c TraceDBCoverage) *map[string]string { return &c.Metadata }
`, want: []string{"zz_probe.go:9: unrecognized state map shape (*ast.UnaryExpr); the census cannot follow the map"}},
		{name: "map_green_shapes", src: header + `
func zzNil(c TraceDBCoverage) bool { return c.Metadata == nil || nil != c.Metadata }
func zzLen(c TraceDBCoverage) int { return len(c.Metadata) }
func zzDelete(c *TraceDBCoverage) { delete(c.Metadata, "reason") }
func zzAliasWrite(c TraceDBCoverage) { m := c.Metadata; m["reason"] = "x" }
func zzAliasTable(c TraceDBCoverage) traceDBSourceRawGateKind { m := c.Metadata; n := m; return traceDBRawDecodeStateGates[n["decode_state"]] }
func zzAliasForward(c TraceDBCoverage) { m := c.Metadata; m["reason"] = m["decode_state"] }
func zzClone(c TraceDBCoverage) map[string]string { out := map[string]string{}; for k, v := range c.Metadata { out[k] = v }; return out }
func zzSkip(c TraceDBCoverage) map[string]string { out := map[string]string{}; for k, v := range c.Metadata { if k == "decode_state" { continue }; out[k] = v }; return out }
func zzCloneCall(c TraceDBCoverage) TraceDBCoverage { c.Metadata = cloneTraceDBStringMap(c.Metadata); return c }
func zzShare(a, b *TraceDBCoverage) { a.Metadata = b.Metadata }
func zzReturnMap(c TraceDBCoverage) map[string]string { return c.Metadata }
func zzLiteral(c TraceDBCoverage) TraceDBCoverage { return TraceDBCoverage{Metadata: c.Metadata} }
type zzBucket struct{ Metadata struct{ Count int } }
func zzStruct(b zzBucket) int { census := b.Metadata; return census.Count + b.Metadata.Count }
func zzPlain(c TraceDBCoverage) string { return c.Metadata["reason"] }
func zzPlainValue(c TraceDBCoverage) string { f := zzPlain; return f(c) }
func zzParamTable(md map[string]string) traceDBSourceRawGateKind { return traceDBRawDecodeStateGates[md["decode_state"]] }
func zzParamKeys(md map[string]string) int { n := 0; for k := range md { if k != "" { n++ } }; return n }
`},
		// Round five (colleague_merge_audit §40.53 收编复核五轮): write targets
		// are not reads (#7), a read returned from a closure the census
		// cannot follow is red while the bound closure stays on its binding
		// pin (#8), and an index key resolves through the same bindings the
		// write census resolves — a local constant, a local bound to the key
		// constant, a package variable, a key-list range, a string / typed
		// key parameter through its callers — with an unresolvable key over
		// the state map red (#6). Each shape stayed green at 480939385,
		// b6f7eeec3 and 533a939fb; see the EVOLUTION RECORD on
		// TestTraceDBStateReadKeyAndClosureShapesEvadedTheBaseCensus.
		{name: "op_assign_target_is_a_read", src: header + `
func zz(out *TraceDBCoverage) { out.Metadata["decode_state"] += "_suffix" }
`, want: []string{"zz_probe.go:9: unrecognized read shape (+=) over decode_state"}},
		{name: "local_const_key", src: header + `
func zz(c TraceDBCoverage) bool { const key = "decode_state"; switch c.Metadata[key] { case "x": return true }; return false }
`, want: []string{"zz_probe.go:9: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "local_binding_of_key_constant", src: header + `
func zz(c TraceDBCoverage) bool { stateKey := string(traceDBSourceRawLaneStateKeyPublication); switch c.Metadata[stateKey] { case "x": return true }; return false }
`, want: []string{"zz_probe.go:9: hand-kept switch over publication_state; readers classify through the gate table"}},
		{name: "package_var_key", src: header + `
var zzKey = "decode_state"
func zz(c TraceDBCoverage) bool { switch c.Metadata[zzKey] { case "x": return true }; return false }
`, want: []string{"zz_probe.go:10: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "package_var_rebound_is_unresolved", src: header + `
var zzKey = "decode_state"
func zzSet() { zzKey = "reason" }
func zz(c TraceDBCoverage) bool { switch c.Metadata[zzKey] { case "x": return true }; return false }
`, want: []string{"zz_probe.go:11: state key the census cannot resolve over the state map (zzKey); readers spell the state key"}},
		{name: "key_list_range", src: header + `
func zz(c TraceDBCoverage) bool { for _, k := range []string{"reason", "decode_state"} { if c.Metadata[k] == "x" { return true } }; return false }
`, want: []string{"zz_probe.go:9: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "string_key_param_literal_caller", src: header + `
func zzRead(c TraceDBCoverage, k string) bool { return c.Metadata[k] == "x" }
func zz(c TraceDBCoverage) bool { return zzRead(c, "decode_state") }
`, want: []string{"zz_probe.go:9: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "typed_key_param_read_caller", src: header + `
func zzRead(c TraceDBCoverage, k traceDBSourceRawLaneStateKey) bool { switch c.Metadata[string(k)] { case "x": return true }; return false }
func zz(c TraceDBCoverage) bool { return zzRead(c, traceDBSourceRawLaneStateKeyPublication) }
`, want: []string{"zz_probe.go:9: hand-kept switch over publication_state; readers classify through the gate table"}},
		{name: "typed_key_param_forwarded_by_wrapper", src: header + `
func zzRead(c TraceDBCoverage, k traceDBSourceRawLaneStateKey) bool { switch c.Metadata[string(k)] { case "x": return true }; return false }
func zzWrap(c TraceDBCoverage, key traceDBSourceRawLaneStateKey) bool { return zzRead(c, key) }
func zz(c TraceDBCoverage) bool { return zzWrap(c, traceDBSourceRawLaneStateKeyPublication) }
`, want: []string{"zz_probe.go:9: hand-kept switch over publication_state; readers classify through the gate table"}},
		{name: "typed_key_param_gate_keys_only", src: header + `
func zzRead(c TraceDBCoverage, k traceDBSourceRawLaneStateKey) bool { switch c.Metadata[string(k)] { case "x": return true }; return false }
func zz(c TraceDBCoverage) bool { return zzRead(c, traceDBSourceRawLaneStateKeyLedger) || zzRead(c, traceDBSourceRawLaneStateKeyJoin) }
`},
		{name: "key_param_without_caller", src: header + `
func zzRead(c TraceDBCoverage, k string) bool { return c.Metadata[k] == "x" }
`, want: []string{"zz_probe.go:9: state key the census cannot resolve over the state map (k); readers spell the state key"}},
		{name: "key_param_function_escapes_as_value", src: header + `
func zzRead(c TraceDBCoverage, k string) bool { return c.Metadata[k] == "x" }
func zzApply(c TraceDBCoverage, f func(TraceDBCoverage, string) bool) bool { return f(c, "decode_state") }
func zz(c TraceDBCoverage) bool { return zzRead(c, "reason") && zzApply(c, zzRead) }
`, want: []string{"zz_probe.go:9: state key the census cannot resolve over the state map (k); readers spell the state key"}},
		{name: "key_param_rebound", src: header + `
func zzRead(c TraceDBCoverage, k string) bool { k = "decode_state"; return c.Metadata[k] == "x" }
func zz(c TraceDBCoverage) bool { return zzRead(c, "reason") }
`, want: []string{"zz_probe.go:9: state key the census cannot resolve over the state map (k); readers spell the state key"}},
		{name: "unresolvable_range_key", src: header + `
func zz(c TraceDBCoverage, keys []string) bool { for _, k := range keys { if c.Metadata[k] == "x" { return true } }; return false }
`, want: []string{"zz_probe.go:9: state key the census cannot resolve over the state map (k); readers spell the state key"}},
		{name: "unresolvable_local_key", src: header + `
func zzName() string { return "decode_state" }
func zz(c TraceDBCoverage) bool { k := zzName(); return c.Metadata[k] == "x" }
`, want: []string{"zz_probe.go:10: state key the census cannot resolve over the state map (k); readers spell the state key"}},
		{name: "closure_param_key", src: header + `
func zz(c TraceDBCoverage) bool { f := func(k string) bool { return c.Metadata[k] == "x" }; return f("decode_state") }
`, want: []string{"zz_probe.go:9: state key the census cannot resolve over the state map (k); readers spell the state key"}},
		{name: "concat_key_unresolved", src: header + `
func zz(c TraceDBCoverage, lane string) bool { return c.Metadata[lane+"_state"] == "x" }
`, want: []string{"zz_probe.go:9: state key the census cannot resolve over the state map (<*ast.BinaryExpr>); readers spell the state key"}},
		{name: "bound_key_over_unseen_map", src: header + `
func zz(c TraceDBCoverage) bool { k := "decode_state"; m := map[string]string{}; return m[k] == "x" }
`, want: []string{"zz_probe.go:9: state key decode_state indexed over a *ast.Ident the census cannot see; readers index the state map"}},
		{name: "invoked_closure_return", src: header + `
func zz(c TraceDBCoverage) bool { return func(md map[string]string) string { return md["decode_state"] }(c.Metadata) == "x" }
`, want: []string{"zz_probe.go:9: read returned from a closure the census cannot follow (decode_state)"}},
		{name: "captured_closure_return", src: header + `
func zz(c TraceDBCoverage) bool { switch func() string { return c.Metadata["decode_state"] }() { case "x": return true }; return false }
`, want: []string{"zz_probe.go:9: read returned from a closure the census cannot follow (decode_state)"}},
		{name: "passed_closure_return", src: header + `
func zzApply(c TraceDBCoverage, f func(map[string]string) string) bool { return f(c.Metadata) == "x" }
func zz(c TraceDBCoverage) bool { return zzApply(c, func(md map[string]string) string { return md["publication_state"] }) }
`, want: []string{"zz_probe.go:10: read returned from a closure the census cannot follow (publication_state)"}},
		{name: "closure_returns_tainted_local", src: header + `
func zz(c TraceDBCoverage) bool { return strings.HasPrefix(func() string { s := c.Metadata["decode_state"]; return s }(), "withheld_") }
`, want: []string{"zz_probe.go:9: read returned from a closure the census cannot follow (decode_state)"}},
		{name: "invoked_closure_tuple_binding", src: header + `
func zz(c TraceDBCoverage) bool { s, ok := func() (string, bool) { return c.Metadata["decode_state"], true }(); return ok && s == "x" }
`, want: []string{"zz_probe.go:9: read returned from a closure the census cannot follow (decode_state)"}},
		// Green controls of round five: a write-only body, the live
		// prefixed-key read (`"retention_" + family + "_state"` names no
		// state key whatever family is), a carried key over some other map,
		// and the ruled range reading — a range over the state map without a
		// key comparison, as the value or as `m[k]`, carries no key signal
		// (disclosed residual, not silently expanded).
		{name: "round_five_green_shapes", src: header + `
func zzWrite(out *TraceDBCoverage) { out.Metadata["publication_state"] = "x"; out.Metadata["decode_state"] = "y" }
func zzRetention(c TraceDBCoverage, family string) bool { s, ok := c.Metadata["retention_"+family+"_state"]; return ok && s == "x" }
func zzSuffixed(c TraceDBCoverage, key string) string { return c.Metadata[key+"_witnesses"] }
func zzSeen(seen map[string]bool, k string) bool { return seen[k] }
func zzSeenCaller() bool { return zzSeen(map[string]bool{}, "decode_state") }
func zzValues(c TraceDBCoverage) bool { for _, v := range c.Metadata { switch v { case "x": return true } }; return false }
func zzKeysIndex(c TraceDBCoverage) bool { for k := range c.Metadata { if c.Metadata[k] == "" { return true } }; return false }
`},
		// Round six (colleague_merge_audit §40.53 收编复核六轮): a tuple
		// binding makes a name multi-valued, never its first value (#3); a
		// same-package helper's map result is the map (#4); the function's
		// own scope precedes a package constant of the same name (#5); a
		// function's own cycle is not a caller (#6); a variadic key parameter
		// is never a key parameter (#7). Each shape stayed green at
		// 6f98f839d (and at 480939385 / b6f7eeec3 / 533a939fb) except where
		// the EVOLUTION RECORD on
		// TestTraceDBStateReadTupleMapScopeCycleShapesEvadedTheBaseCensus says
		// otherwise.
		{name: "tuple_rebound_local_key", src: header + `
func zzPick() (string, bool) { return "decode_state", true }
func zz(c TraceDBCoverage) bool { k := "reason"; k, _ = zzPick(); return c.Metadata[k] == "x" }
`, want: []string{"zz_probe.go:10: state key the census cannot resolve over the state map (k); readers spell the state key"}},
		{name: "tuple_rebound_concat_prefix", src: header + `
func zzPick() (string, bool) { return "x_", true }
func zz(c TraceDBCoverage) bool { p := "x_"; p, _ = zzPick(); return c.Metadata[p+"decode_state"] == "x" }
`, want: []string{"zz_probe.go:10: state key the census cannot resolve over the state map (<*ast.BinaryExpr>); readers spell the state key"}},
		{name: "tuple_only_binding", src: header + `
func zzPick() (string, bool) { return "decode_state", true }
func zz(c TraceDBCoverage) bool { k, _ := zzPick(); return c.Metadata[k] == "x" }
`, want: []string{"zz_probe.go:10: state key the census cannot resolve over the state map (k); readers spell the state key"}},
		{name: "tuple_var_binding", src: header + `
func zzPick() (string, bool) { return "decode_state", true }
func zz(c TraceDBCoverage) bool { var k, ok = zzPick(); return ok && c.Metadata[k] == "x" }
`, want: []string{"zz_probe.go:10: state key the census cannot resolve over the state map (k); readers spell the state key"}},
		{name: "helper_map_bound_carried_key", src: header + `
func zzMap(c TraceDBCoverage) map[string]string { return c.Metadata }
func zzRead(c TraceDBCoverage, k string) bool { m := zzMap(c); return m[k] == "x" }
func zz(c TraceDBCoverage) bool { return zzRead(c, "decode_state") }
`, want: []string{"zz_probe.go:10: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "helper_map_direct_carried_key", src: header + `
func zzMap(c TraceDBCoverage) map[string]string { return c.Metadata }
func zzRead(c TraceDBCoverage, k string) bool { return zzMap(c)[k] == "x" }
func zz(c TraceDBCoverage) bool { return zzRead(c, "decode_state") }
`, want: []string{"zz_probe.go:10: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "helper_map_bound_spelled_key", src: header + `
func zzMap(c TraceDBCoverage) map[string]string { return c.Metadata }
func zz(c TraceDBCoverage) bool { m := zzMap(c); switch m["decode_state"] { case "x": return true }; return false }
`, want: []string{"zz_probe.go:10: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "helper_map_tuple_bound", src: header + `
func zzMapOK(c TraceDBCoverage) (map[string]string, bool) { return c.Metadata, c.Metadata != nil }
func zz(c TraceDBCoverage) bool { m, ok := zzMapOK(c); return ok && m["publication_state"] == "x" }
`, want: []string{"zz_probe.go:10: literal comparison over publication_state; readers classify through the gate table"}},
		{name: "helper_map_forwarded_by_wrapper", src: header + `
func zzMap(c TraceDBCoverage) map[string]string { return c.Metadata }
func zzOuter(c TraceDBCoverage) map[string]string { return zzMap(c) }
func zz(c TraceDBCoverage) bool { return strings.HasPrefix(zzOuter(c)["decode_state"], "withheld_") }
`, want: []string{"zz_probe.go:11: prefix/substring classification over decode_state (strings.HasPrefix); readers classify through the gate table"}},
		{name: "helper_map_param_returned", src: header + `
func zzEnsure(md map[string]string) map[string]string { return md }
func zz(c TraceDBCoverage) bool { return zzEnsure(c.Metadata)["decode_state"] == "x" }
`, want: []string{"zz_probe.go:10: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "helper_map_named_result", src: header + `
func zzMap(c TraceDBCoverage) (m map[string]string) { m = c.Metadata; return }
func zz(c TraceDBCoverage) bool { return zzMap(c)["decode_state"] == "x" }
`, want: []string{"zz_probe.go:10: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "helper_map_bound_to_field", src: header + `
type zzBox struct{ md map[string]string }
func zzMap(c TraceDBCoverage) map[string]string { return c.Metadata }
func zz(c TraceDBCoverage, b *zzBox) { b.md = zzMap(c) }
`, want: []string{"zz_probe.go:11: state map bound to a *ast.SelectorExpr the census cannot follow"}},
		{name: "helper_map_tuple_bound_to_field", src: header + `
type zzBox struct{ md map[string]string }
func zzMapOK(c TraceDBCoverage) (map[string]string, bool) { return c.Metadata, true }
func zz(c TraceDBCoverage, b *zzBox) { var ok bool; b.md, ok = zzMapOK(c); _ = ok }
`, want: []string{"zz_probe.go:11: state map bound to a *ast.SelectorExpr the census cannot follow"}},
		{name: "local_shadows_constant_bound_to_state_key", src: header + `
func zz(c TraceDBCoverage) bool { traceDBRawDecodeStateComplete := "decode_state"; return c.Metadata[traceDBRawDecodeStateComplete] == "x" }
`, want: []string{"zz_probe.go:9: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "param_shadows_constant_resolved_through_callers", src: header + `
func zzRead(c TraceDBCoverage, traceDBRawDecodeStateComplete string) bool { return c.Metadata[traceDBRawDecodeStateComplete] == "x" }
func zz(c TraceDBCoverage) bool { return zzRead(c, "decode_state") }
`, want: []string{"zz_probe.go:9: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "local_shadows_constant_unresolvably", src: header + `
func zzName() string { return "decode_state" }
func zz(c TraceDBCoverage) bool { traceDBRawDecodeStateComplete := zzName(); return c.Metadata[traceDBRawDecodeStateComplete] == "x" }
`, want: []string{"zz_probe.go:10: state key the census cannot resolve over the state map (traceDBRawDecodeStateComplete); readers spell the state key"}},
		{name: "closure_param_shadows_constant", src: header + `
func zz(c TraceDBCoverage) bool { f := func(traceDBRawDecodeStateComplete string) bool { return c.Metadata[traceDBRawDecodeStateComplete] == "x" }; return f("decode_state") }
`, want: []string{"zz_probe.go:9: state key the census cannot resolve over the state map (traceDBRawDecodeStateComplete); readers spell the state key"}},
		{name: "named_result_shadows_constant", src: header + `
func zz(c TraceDBCoverage) (traceDBRawDecodeStateComplete string) { return c.Metadata[traceDBRawDecodeStateComplete] }
`, want: []string{"zz_probe.go:9: state key the census cannot resolve over the state map (traceDBRawDecodeStateComplete); readers spell the state key"}},
		{name: "range_key_compared_to_shadowing_local", src: header + `
func zz(c TraceDBCoverage) bool { traceDBRawDecodeStateComplete := "decode_state"; for k, v := range c.Metadata { if k == traceDBRawDecodeStateComplete { switch v { case "x": return true } } }; return false }
`, want: []string{"zz_probe.go:9: hand-kept switch over decode_state; readers classify through the gate table"}},
		{name: "self_recursion_only_caller", src: header + `
func zzRead(c TraceDBCoverage, k string, n int) bool { if n > 0 { return zzRead(c, k, n-1) }; return c.Metadata[k] == "x" }
`, want: []string{"zz_probe.go:9: state key the census cannot resolve over the state map (k); readers spell the state key"}},
		{name: "mutual_recursion_only_callers", src: header + `
func zzA(c TraceDBCoverage, k string, n int) bool { if n > 0 { return zzB(c, k, n-1) }; return c.Metadata[k] == "x" }
func zzB(c TraceDBCoverage, k string, n int) bool { if n > 0 { return zzA(c, k, n-1) }; return c.Metadata[k] == "y" }
`, want: []string{
			"zz_probe.go:10: state key the census cannot resolve over the state map (k); readers spell the state key",
			"zz_probe.go:9: state key the census cannot resolve over the state map (k); readers spell the state key",
		}},
		{name: "recursion_with_external_caller", src: header + `
func zzRead(c TraceDBCoverage, k string, n int) bool { if n > 0 { return zzRead(c, k, n-1) }; return c.Metadata[k] == "x" }
func zz(c TraceDBCoverage) bool { return zzRead(c, "decode_state", 2) }
`, want: []string{"zz_probe.go:9: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "mutual_recursion_with_external_caller", src: header + `
func zzA(c TraceDBCoverage, k string, n int) bool { if n > 0 { return zzB(c, k, n-1) }; return c.Metadata[k] == "x" }
func zzB(c TraceDBCoverage, k string, n int) bool { if n > 0 { return zzA(c, k, n-1) }; return c.Metadata[k] == "y" }
func zz(c TraceDBCoverage) bool { return zzA(c, "publication_state", 2) }
`, want: []string{
			"zz_probe.go:10: literal comparison over publication_state; readers classify through the gate table",
			"zz_probe.go:9: literal comparison over publication_state; readers classify through the gate table",
		}},
		{name: "uncalled_wrapper_forwards_unresolved", src: header + `
func zzRead(c TraceDBCoverage, k string) bool { return c.Metadata[k] == "x" }
func zzWrap(c TraceDBCoverage, key string) bool { return zzRead(c, key) }
func zz(c TraceDBCoverage) bool { return zzRead(c, "decode_state") }
`, want: []string{"zz_probe.go:9: state key the census cannot resolve over the state map (k); readers spell the state key"}},
		{name: "variadic_key_param_is_unresolved", src: header + `
func zzRead(c TraceDBCoverage, keys ...string) bool { return c.Metadata[keys[0]] == "x" }
func zz(c TraceDBCoverage) bool { return zzRead(c, "decode_state") }
`, want: []string{"zz_probe.go:9: state key the census cannot resolve over the state map (<*ast.IndexExpr>); readers spell the state key"}},
		// Green controls of round six: a multi-valued key over some other map
		// (not a read), a shadowing local bound to a non-key, helpers
		// returning a fresh map (a literal, make, the live clone) under a
		// carried key, and the map-returning helper in every green map shape
		// (a table lookup, a forwarding write, len, an uncompared range, a
		// discarded result).
		{name: "round_six_green_shapes", src: header + `
func zzPick() (string, bool) { return "decode_state", true }
func zzTupleOverOtherMap(c TraceDBCoverage) bool { k, _ := zzPick(); m := map[string]string{}; return m[k] == "x" }
func zzShadowNonKey(c TraceDBCoverage) bool { traceDBRawDecodeStateComplete := "reason"; return c.Metadata[traceDBRawDecodeStateComplete] == "x" }
func zzFresh(c TraceDBCoverage) map[string]string { out := map[string]string{}; for k, v := range c.Metadata { out[k] = v }; return out }
func zzFreshRead(c TraceDBCoverage, k string) bool { m := zzFresh(c); return m[k] == "x" }
func zzFreshCaller(c TraceDBCoverage) bool { return zzFreshRead(c, "decode_state") }
func zzMade(c TraceDBCoverage) map[string]string { m := make(map[string]string); return m }
func zzMadeRead(c TraceDBCoverage, k string) bool { return zzMade(c)[k] == "x" }
func zzMadeCaller(c TraceDBCoverage) bool { return zzMadeRead(c, "decode_state") }
func zzCloneRead(c TraceDBCoverage, k string) bool { return cloneTraceDBStringMap(c.Metadata)[k] == "x" }
func zzCloneCaller(c TraceDBCoverage) bool { return zzCloneRead(c, "decode_state") }
func zzMap(c TraceDBCoverage) map[string]string { return c.Metadata }
func zzMapTable(c TraceDBCoverage) traceDBSourceRawGateKind { return traceDBRawDecodeStateGates[zzMap(c)["decode_state"]] }
func zzMapForward(c TraceDBCoverage) { m := zzMap(c); m["reason"] = m["decode_state"] }
func zzMapLen(c TraceDBCoverage) int { return len(zzMap(c)) }
func zzMapRange(c TraceDBCoverage) int { n := 0; for range zzMap(c) { n++ }; return n }
func zzMapDiscard(c TraceDBCoverage) { zzMap(c) }
func zzMapPassed(c TraceDBCoverage) traceDBSourceRawGateKind { return zzParamTable(zzMap(c)) }
func zzParamTable(md map[string]string) traceDBSourceRawGateKind { return traceDBRawDecodeStateGates[md["decode_state"]] }
`},
		// Round seven (colleague_merge_audit §40.53 收编复核七轮): a call site
		// inside the function's own cycle contributes the key it spells (#3 —
		// silent at 42fcf3fd1, red at 6f98f839d); the shared binding helper
		// resolves a copy scope-first, so a copy of a local or parameter
		// shadowing a package constant carries the shadowing binding's value
		// or nothing (#4); a key spelled through a once-bound copy or
		// conversion of a parameter resolves through the callers, through a
		// once-bound concatenation as the concatenation (#5). See the
		// EVOLUTION RECORD on
		// TestTraceDBStateReadCycleLiteralAndCopyShapesEvadedTheBaseCensus.
		{name: "self_recursion_in_cycle_literal", src: header + `
func zzRead(c TraceDBCoverage, k string, n int) bool { if n > 0 { return zzRead(c, "decode_state", n-1) }; return c.Metadata[k] == "x" }
func zz(c TraceDBCoverage) bool { return zzRead(c, "reason", 1) }
`, want: []string{"zz_probe.go:9: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "mutual_recursion_in_cycle_literal", src: header + `
func zzA(c TraceDBCoverage, k string, n int) bool { if n > 0 { return zzB(c, "decode_state", n-1) }; return c.Metadata[k] == "x" }
func zzB(c TraceDBCoverage, k string, n int) bool { if n > 0 { return zzA(c, k, n-1) }; return c.Metadata[k] == "y" }
func zz(c TraceDBCoverage) bool { return zzA(c, "reason", 1) }
`, want: []string{
			"zz_probe.go:10: literal comparison over decode_state; readers classify through the gate table",
			"zz_probe.go:9: literal comparison over decode_state; readers classify through the gate table",
		}},
		{name: "in_cycle_literal_without_off_cycle_caller", src: header + `
func zzRead(c TraceDBCoverage, k string, n int) bool { if n > 0 { return zzRead(c, "decode_state", n-1) }; return c.Metadata[k] == "x" }
`, want: []string{"zz_probe.go:9: state key the census cannot resolve over the state map (k); readers spell the state key"}},
		{name: "recursion_swapped_parameters", src: header + `
func zzA(c TraceDBCoverage, k, j string, n int) bool { if n > 0 { return zzA(c, j, k, n-1) }; return c.Metadata[k] == "x" }
func zz(c TraceDBCoverage) bool { return zzA(c, "reason", "decode_state", 1) }
`, want: []string{"zz_probe.go:9: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "shadowed_constant_copied_through_local", src: header + `
func zz(c TraceDBCoverage) bool { traceDBRawDecodeStateComplete := "decode_state"; k := traceDBRawDecodeStateComplete; return c.Metadata[k] == "x" }
`, want: []string{"zz_probe.go:9: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "shadowed_constant_param_copied_through_local", src: header + `
func zzRead(c TraceDBCoverage, traceDBRawDecodeStateComplete string) bool { k := traceDBRawDecodeStateComplete; return c.Metadata[k] == "x" }
func zz(c TraceDBCoverage) bool { return zzRead(c, "decode_state") }
`, want: []string{"zz_probe.go:9: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "param_copied_through_local", src: header + `
func zzRead(c TraceDBCoverage, k string) bool { key := k; return c.Metadata[key] == "x" }
func zz(c TraceDBCoverage) bool { return zzRead(c, "decode_state") }
`, want: []string{"zz_probe.go:9: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "typed_param_converted_through_local", src: header + `
func zzRead(c TraceDBCoverage, key traceDBSourceRawLaneStateKey) bool { stateKey := string(key); switch c.Metadata[stateKey] { case "x": return true }; return false }
func zz(c TraceDBCoverage) bool { return zzRead(c, traceDBSourceRawLaneStateKeyPublication) }
`, want: []string{"zz_probe.go:9: hand-kept switch over publication_state; readers classify through the gate table"}},
		{name: "copy_chain_of_literal", src: header + `
func zz(c TraceDBCoverage) bool { a := "decode_state"; b := a; k := b; return c.Metadata[k] == "x" }
`, want: []string{"zz_probe.go:9: literal comparison over decode_state; readers classify through the gate table"}},
		{name: "concat_bound_local_unresolved", src: header + `
func zz(c TraceDBCoverage, lane string) bool { k := lane + "_state"; return c.Metadata[k] == "x" }
`, want: []string{"zz_probe.go:9: state key the census cannot resolve over the state map (k); readers spell the state key"}},
		{name: "copy_of_tuple_bound_local_unresolved", src: header + `
func zzPick() (string, bool) { return "decode_state", true }
func zz(c TraceDBCoverage) bool { k, _ := zzPick(); j := k; return c.Metadata[j] == "x" }
`, want: []string{"zz_probe.go:10: state key the census cannot resolve over the state map (j); readers spell the state key"}},
		// Green controls of round seven: a cycle that only forwards its
		// parameter under a non-key caller, a once-bound concatenation whose
		// literal prefix excludes every state key, a copy of a shadowing local
		// bound to a non-key.
		{name: "round_seven_green_shapes", src: header + `
func zzForward(c TraceDBCoverage, k string, n int) bool { if n > 0 { return zzForward(c, k, n-1) }; return c.Metadata[k] == "x" }
func zzForwardCaller(c TraceDBCoverage) bool { return zzForward(c, "reason", 1) }
func zzRetentionCopy(c TraceDBCoverage, family string) bool { k := "retention_" + family + "_state"; return c.Metadata[k] == "x" }
func zzShadowCopyNonKey(c TraceDBCoverage) bool { traceDBRawDecodeStateComplete := "reason"; k := traceDBRawDecodeStateComplete; return c.Metadata[k] == "x" }
`},
	} {
		t.Run(test.name, func(t *testing.T) {
			consts, files, fset := traceDBCensusFilesWithProbe(t, test.src)
			problems, _ := traceDBStateReadProblems(files, fset, consts)
			got := traceDBProbeProblems(problems)
			if strings.Join(got, "\n") != strings.Join(test.want, "\n") {
				t.Fatalf("problems=\n%s\nwant=\n%s", strings.Join(got, "\n"), strings.Join(test.want, "\n"))
			}
		})
	}
}

// TestTraceDBStateReadSinkRegistrationIsLoadBearing (fold-in #8): the
// result-position taint crosses the live gate classifier —
// traceDBSourceRawDecodeGate returns the raw decode_state as its reason at
// position 1 — into traceDBApplySourceRawDecodeGate, where the reason is
// handed to the class funnel. The funnel's roster entry is what keeps that
// hand-off green: with the entry withdrawn the live tree names exactly that
// call, so the roster is bound to the data flow it sanctions and cannot go
// stale silently.
func TestTraceDBStateReadSinkRegistrationIsLoadBearing(t *testing.T) {
	const funnel = "traceDBMintSourceRawLaneGateOutcome"
	if !traceDBStateReadSinks[funnel] {
		t.Fatalf("%s is not a registered sink", funnel)
	}
	delete(traceDBStateReadSinks, funnel)
	defer func() { traceDBStateReadSinks[funnel] = true }()
	consts, files, fset := traceDBPackageStringConsts(t)
	problems, _ := traceDBStateReadProblems(files, fset, consts)
	want := "source_raw_lane_gate.go:203: unrecognized reader call " + funnel + " over decode_state"
	if len(problems) != 1 || problems[0] != want {
		t.Fatalf("with the funnel unregistered the live tree reported %v\nwant exactly %q", problems, want)
	}
}

// TestTraceDBStateReadLaunderShapesEvadedTheBaseCensus is the EVOLUTION
// RECORD of fold-in #8: one probe file carrying the launder shapes, fed to
// the census as a non-test file.
//
//   - 8a1e5d695: no reader census exists (TestTraceDBRawDecodeStateReaders…
//     is absent); the probe minted nothing.
//   - 381f36cc9: the reader census recognized only a direct
//     `Metadata["decode_state"]` IndexExpr as a switch tag or a ==/!= operand;
//     the probe minted nothing (PASS over the probe).
//   - 480939385: traceDBStateReadProblems tainted through single-identifier
//     bindings only; the probe minted nothing (PASS over the probe).
//   - now: every launder site of the probe is named, and nothing else.
func TestTraceDBStateReadLaunderShapesEvadedTheBaseCensus(t *testing.T) {
	consts, files, fset := traceDBCensusFilesWithProbe(t, traceDBStateReadLaunderProbe)
	problems, _ := traceDBStateReadProblems(files, fset, consts)
	got := traceDBProbeProblems(problems)
	want := []string{ // sorted as strings: the concat switch on line 7 sorts last
		"zz_probe.go:16: hand-kept switch over decode_state; readers classify through the gate table",
		"zz_probe.go:26: hand-kept switch over decode_state; readers classify through the gate table",
		"zz_probe.go:35: hand-kept switch over decode_state; readers classify through the gate table",
		"zz_probe.go:44: unrecognized reader call zzLaunderUse over decode_state",
		"zz_probe.go:7: hand-kept switch over decode_state; readers classify through the gate table",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("problems=\n%s\nwant=\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// traceDBStateReadLaunderProbe is the probe file of the EVOLUTION RECORD
// above, kept verbatim so the base runs and this pin read the same bytes.
const traceDBStateReadLaunderProbe = `package hitraceconv

import "fmt"

func zzLaunderConcat(c TraceDBCoverage) bool {
	s := c.Metadata["decode_state"] + ""
	switch s {
	case "strict_target_ledger_complete":
		return true
	}
	return false
}

func zzLaunderSprint(c TraceDBCoverage) bool {
	s := fmt.Sprint(c.Metadata["decode_state"])
	switch s {
	case "strict_target_ledger_complete":
		return true
	}
	return false
}

func zzLaunderState(c TraceDBCoverage) string { return c.Metadata["decode_state"] }

func zzLaunderHelper(c TraceDBCoverage) bool {
	switch zzLaunderState(c) {
	case "strict_target_ledger_complete":
		return true
	}
	return false
}

func zzLaunderCommaOk(c TraceDBCoverage) bool {
	s, _ := c.Metadata["decode_state"]
	switch s {
	case "strict_target_ledger_complete":
		return true
	}
	return false
}

func zzLaunderUse(s string) bool { return s != "" }

func zzLaunderCall(c TraceDBCoverage) bool { return zzLaunderUse("x" + c.Metadata["decode_state"]) }
`

// TestTraceDBStateReadMapAndValueShapesEvadedTheBaseCensus is the EVOLUTION
// RECORD of fold-in #8, round four: one probe file carrying the map / range /
// value shapes, fed to the census as a non-test file.
//
//   - 480939385 and b6f7eeec3: traceDBStateReadProblems recognized a read only
//     as the spelled `<x>.Metadata[<key>]` selector and a helper only by a
//     function name or a method selector; the probe minted nothing (PASS over
//     the probe, live reads unchanged) — an aliased map, a map[string]string
//     parameter, a range value, a method value and a function value carried
//     no taint and never reached the unrecognized-binding arm.
//   - now: every site of the probe is named, and nothing else; the live
//     reader floor is unchanged (0 problems; the "58 reads" recorded at
//     round four counted index occurrences — 42 write targets among them —
//     and reads 16 genuine reads since round five, see
//     TestTraceDBStateReadKeyAndClosureShapesEvadedTheBaseCensus).
func TestTraceDBStateReadMapAndValueShapesEvadedTheBaseCensus(t *testing.T) {
	consts, files, fset := traceDBCensusFilesWithProbe(t, traceDBStateReadMapValueProbe)
	problems, _ := traceDBStateReadProblems(files, fset, consts)
	got := traceDBProbeProblems(problems)
	want := []string{ // sorted as strings
		"zz_probe.go:17: literal comparison over publication_state; readers classify through the gate table",
		"zz_probe.go:23: hand-kept switch over decode_state; readers classify through the gate table",
		"zz_probe.go:35: prefix/substring classification over decode_state (strings.HasPrefix); readers classify through the gate table",
		"zz_probe.go:48: hand-kept switch over decode_state; readers classify through the gate table",
		"zz_probe.go:59: hand-kept switch over decode_state; readers classify through the gate table",
		"zz_probe.go:8: hand-kept switch over decode_state; readers classify through the gate table",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("problems=\n%s\nwant=\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// traceDBStateReadMapValueProbe is the probe file of the EVOLUTION RECORD
// above, kept verbatim so the base runs and this pin read the same bytes.
const traceDBStateReadMapValueProbe = `package hitraceconv

import "strings"

func zzAliasMap(c TraceDBCoverage) bool {
	m := c.Metadata
	s := m["decode_state"]
	switch s {
	case "strict_target_ledger_complete":
		return true
	}
	return false
}

func zzAliasCompare(c TraceDBCoverage) bool {
	m := c.Metadata
	return m["publication_state"] == "withheld_visibility_envelope_incomplete"
}

func zzMapParam(md map[string]string) string { return md["decode_state"] }

func zzMapParamSwitch(c TraceDBCoverage) bool {
	switch zzMapParam(c.Metadata) {
	case "strict_target_ledger_complete":
		return true
	}
	return false
}

func zzRange(c TraceDBCoverage) bool {
	for k, v := range c.Metadata {
		if k != "decode_state" {
			continue
		}
		if strings.HasPrefix(v, "withheld_") {
			return true
		}
	}
	return false
}

type zzHolder struct{ c TraceDBCoverage }

func (h zzHolder) rawState() string { return h.c.Metadata["decode_state"] }

func zzMethodValue(h zzHolder) bool {
	f := h.rawState
	switch f() {
	case "strict_target_ledger_complete":
		return true
	}
	return false
}

func zzState(c TraceDBCoverage) string { return c.Metadata["decode_state"] }

func zzFuncValue(c TraceDBCoverage) bool {
	f := zzState
	switch f(c) {
	case "strict_target_ledger_complete":
		return true
	}
	return false
}
`

// TestTraceDBStateReadKeyAndClosureShapesEvadedTheBaseCensus is the EVOLUTION
// RECORD of round five (§40.53 收编复核五轮, findings #6 / #7 / #8): one probe
// file carrying the key-resolution and closure-return shapes, fed to the
// census as a non-test file, plus a write-only probe for the read count.
//
//   - 480939385 / b6f7eeec3 / 533a939fb: the census resolved an index key
//     only when spelled at the site (a literal, a package constant,
//     `string(<constant>)`) or carried by a compared range key; every other
//     key over the state map was silently not a read (the local constant,
//     the local bound to the key constant, the package variable, the
//     key-list range, the string and typed-key parameters — a caller passing
//     the publication key to the live inherit funnel minted nothing at any
//     base). A read returned from a closure was counted where the closure's
//     parameter or capture was tainted and judged nowhere. The probe minted
//     0 problems at every base; its reads delta was +1 (480939385,
//     b6f7eeec3: the captured literal's inner read only) and +3 (533a939fb:
//     the map-parameter closures' inner reads too, tainted by type since
//     round four). Plain write targets were counted as reads: the
//     write-only probe (two writes, one forwarding read) moved the count by
//     +3 at every base, and the plain live figures — 55 at 480939385, 58 at
//     b6f7eeec3 and 533a939fb — were index occurrences, 42 of the 58 being
//     write targets.
//   - now: every read site of the probe is named, and nothing else; the
//     probe adds exactly its nine genuine reads, the write-only probe adds
//     exactly its one forwarding read, and the live floor reads 16 genuine
//     reads with 0 problems.
func TestTraceDBStateReadKeyAndClosureShapesEvadedTheBaseCensus(t *testing.T) {
	consts, files, fset := traceDBPackageStringConsts(t)
	problems, live := traceDBStateReadProblems(files, fset, consts)
	if len(problems) != 0 {
		t.Fatalf("live tree reported %v", problems)
	}
	consts, files, fset = traceDBCensusFilesWithProbe(t, traceDBStateReadKeyClosureProbe)
	problems, reads := traceDBStateReadProblems(files, fset, consts)
	got := traceDBProbeProblems(problems)
	want := []string{ // sorted as strings
		"zz_probe.go:18: hand-kept switch over publication_state; readers classify through the gate table",
		"zz_probe.go:26: literal comparison over decode_state; readers classify through the gate table",
		"zz_probe.go:31: prefix/substring classification over decode_state (strings.HasPrefix); readers classify through the gate table",
		"zz_probe.go:39: literal comparison over decode_state; readers classify through the gate table",
		"zz_probe.go:45: hand-kept switch over publication_state; readers classify through the gate table",
		"zz_probe.go:58: state key the census cannot resolve over the state map (k); readers spell the state key",
		"zz_probe.go:66: read returned from a closure the census cannot follow (decode_state)",
		"zz_probe.go:70: read returned from a closure the census cannot follow (decode_state)",
		"zz_probe.go:82: read returned from a closure the census cannot follow (publication_state)",
		"zz_probe.go:9: hand-kept switch over decode_state; readers classify through the gate table",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("problems=\n%s\nwant=\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// The probe's genuine reads: the local-constant, local-binding,
	// package-variable, key-list, string-parameter and typed-parameter
	// reads, and the three closures' inner reads; the unresolved key is
	// not counted.
	if reads != live+9 {
		t.Fatalf("key/closure probe moved the read count %d → %d, want +9 genuine reads", live, reads)
	}
	consts, files, fset = traceDBCensusFilesWithProbe(t, traceDBStateReadWriteTargetProbe)
	problems, reads = traceDBStateReadProblems(files, fset, consts)
	if got := traceDBProbeProblems(problems); len(got) != 0 {
		t.Fatalf("write-only probe reported %v", got)
	}
	if reads != live+1 {
		t.Fatalf("write-only probe moved the read count %d → %d, want +1 (its forwarding read; write targets are not reads)", live, reads)
	}
}

// traceDBStateReadKeyClosureProbe is the probe file of the EVOLUTION RECORD
// above, kept verbatim so the base runs and this pin read the same bytes.
const traceDBStateReadKeyClosureProbe = `package hitraceconv

import "strings"

var zzPackageKey = "decode_state"

func zzLocalConst(c TraceDBCoverage) bool {
	const key = "decode_state"
	switch c.Metadata[key] {
	case "strict_target_ledger_complete":
		return true
	}
	return false
}

func zzLocalBinding(c TraceDBCoverage) bool {
	stateKey := string(traceDBSourceRawLaneStateKeyPublication)
	switch c.Metadata[stateKey] {
	case "withheld_visibility_envelope_incomplete":
		return true
	}
	return false
}

func zzPackageVar(c TraceDBCoverage) bool {
	return c.Metadata[zzPackageKey] == "strict_target_ledger_complete"
}

func zzKeyList(c TraceDBCoverage) bool {
	for _, k := range []string{"reason", "decode_state"} {
		if strings.HasPrefix(c.Metadata[k], "withheld_") {
			return true
		}
	}
	return false
}

func zzStringKeyRead(c TraceDBCoverage, k string) bool {
	return c.Metadata[k] == "strict_target_ledger_complete"
}

func zzStringKeyCaller(c TraceDBCoverage) bool { return zzStringKeyRead(c, "decode_state") }

func zzTypedKeyRead(c TraceDBCoverage, k traceDBSourceRawLaneStateKey) bool {
	switch c.Metadata[string(k)] {
	case "withheld_visibility_envelope_incomplete":
		return true
	}
	return false
}

func zzTypedKeyCaller(c TraceDBCoverage) bool {
	return zzTypedKeyRead(c, traceDBSourceRawLaneStateKeyPublication)
}

func zzUnresolvedKey(c TraceDBCoverage, keys []string) bool {
	for _, k := range keys {
		if c.Metadata[k] == "strict_target_ledger_complete" {
			return true
		}
	}
	return false
}

func zzInvokedClosure(c TraceDBCoverage) bool {
	return func(md map[string]string) string { return md["decode_state"] }(c.Metadata) == "strict_target_ledger_complete"
}

func zzCapturedClosure(c TraceDBCoverage) bool {
	switch func() string { return c.Metadata["decode_state"] }() {
	case "strict_target_ledger_complete":
		return true
	}
	return false
}

func zzApplyClosure(c TraceDBCoverage, f func(map[string]string) string) bool {
	return f(c.Metadata) == "withheld_visibility_envelope_incomplete"
}

func zzPassedClosure(c TraceDBCoverage) bool {
	return zzApplyClosure(c, func(md map[string]string) string { return md["publication_state"] })
}
`

// traceDBStateReadWriteTargetProbe is the write-only probe of the EVOLUTION
// RECORD above: two plain write targets and one forwarding read.
const traceDBStateReadWriteTargetProbe = `package hitraceconv

func zzWriteTargets(out *TraceDBCoverage) {
	out.Metadata["publication_state"] = "withheld_visibility_envelope_incomplete"
	out.Metadata["decode_state"] = "strict_target_ledger_complete"
	out.Metadata["reason"] = out.Metadata["decode_state"]
}
`

// TestTraceDBStateReadTupleMapScopeCycleShapesEvadedTheBaseCensus is the
// EVOLUTION RECORD of round six (§40.53 收编复核六轮, findings #3 / #4 / #5 /
// #6): one probe file carrying the tuple re-binding, helper-returned map,
// constant-shadowing and recursion-only shapes, fed to the reader census as
// a non-test file, plus a write probe for the write census.
//
//   - 480939385 / b6f7eeec3: the reader census resolved no local, parameter
//     or helper map at all; the probe minted 0 problems and moved the read
//     count by 0.
//   - 533a939fb: the spelled key over the helper call was red as "indexed
//     over a *ast.CallExpr the census cannot see" (round four's map taint
//     stopped at the call); every other shape was silent — 1 problem, reads
//     +0.
//   - 6f98f839d: the same single problem; the tuple re-bound local resolved
//     to its stale first value ("reason", keySpelled, not a read), the
//     helper's map result carried no taint (the carried key over it was
//     "some other map's business"), the shadowing local and parameter
//     resolved to the package constant's value ("strict_target_ledger_complete",
//     not a state key), and the self-call / mutual calls counted as callers
//     with an empty carried set — 1 problem, reads +0.
//   - write probe, every base: the tuple re-bound key resolved to "reason"
//     (or to nothing) and the write minted no site and no report.
//   - now: every site of the read probe is named and nothing else; the probe
//     adds exactly its six genuine reads (the two helper-map reads, the
//     spelled helper-map read, the shadowing local and parameter reads, and
//     the prefix test's result laundered into its caller's return — a
//     strings.* call taints its result, fold-in #8; the unresolved keys are
//     not counted); the write probe draws exactly one fail-loud report and
//     no site.
func TestTraceDBStateReadTupleMapScopeCycleShapesEvadedTheBaseCensus(t *testing.T) {
	consts, files, fset := traceDBPackageStringConsts(t)
	problems, live := traceDBStateReadProblems(files, fset, consts)
	if len(problems) != 0 {
		t.Fatalf("live tree reported %v", problems)
	}
	consts, files, fset = traceDBCensusFilesWithProbe(t, traceDBStateReadTupleMapScopeCycleProbe)
	problems, reads := traceDBStateReadProblems(files, fset, consts)
	got := traceDBProbeProblems(problems)
	want := []string{ // sorted as strings
		"zz_probe.go:10: state key the census cannot resolve over the state map (k); readers spell the state key",
		"zz_probe.go:16: state key the census cannot resolve over the state map (<*ast.BinaryExpr>); readers spell the state key",
		"zz_probe.go:23: literal comparison over decode_state; readers classify through the gate table",
		"zz_probe.go:29: prefix/substring classification over decode_state (strings.HasPrefix); readers classify through the gate table",
		"zz_probe.go:35: hand-kept switch over decode_state; readers classify through the gate table",
		"zz_probe.go:44: literal comparison over decode_state; readers classify through the gate table",
		"zz_probe.go:48: hand-kept switch over decode_state; readers classify through the gate table",
		"zz_probe.go:61: state key the census cannot resolve over the state map (k); readers spell the state key",
		"zz_probe.go:68: state key the census cannot resolve over the state map (k); readers spell the state key",
		"zz_probe.go:75: state key the census cannot resolve over the state map (k); readers spell the state key",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("problems=\n%s\nwant=\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if reads != live+6 {
		t.Fatalf("tuple/map/scope/cycle probe moved the read count %d → %d, want +6 genuine reads", live, reads)
	}
	consts, files, fset = traceDBCensusFilesWithProbe(t, traceDBStateWriteTupleKeyProbe)
	recorder := &traceDBRecordingTB{TB: t}
	sites, _ := traceDBLaneStateWriteSitesOf(recorder, consts, files, fset)
	for _, site := range sites {
		if site.file == "zz_probe.go" {
			t.Fatalf("write probe minted a site: %+v", site)
		}
	}
	reports := traceDBProbeProblems(recorder.problems)
	wantReport := `zz_probe.go:8: "strict_target_ledger_complete" written under a Metadata key the census cannot resolve (k)`
	if len(reports) != 1 || reports[0] != wantReport {
		t.Fatalf("write probe reported %v\nwant exactly %q", reports, wantReport)
	}
}

// traceDBStateReadTupleMapScopeCycleProbe is the read probe of the EVOLUTION
// RECORD above, kept verbatim so the base runs and this pin read the same
// bytes.
const traceDBStateReadTupleMapScopeCycleProbe = `package hitraceconv

import "strings"

func zzPick() (string, bool) { return "decode_state", true }

func zzTupleRebound(c TraceDBCoverage) bool {
	k := "reason"
	k, _ = zzPick()
	return c.Metadata[k] == "strict_target_ledger_complete"
}

func zzTupleConcat(c TraceDBCoverage) bool {
	p := "x_"
	p, _ = zzPick()
	return c.Metadata[p+"decode_state"] == "strict_target_ledger_complete"
}

func zzMap(c TraceDBCoverage) map[string]string { return c.Metadata }

func zzHelperMapRead(c TraceDBCoverage, k string) bool {
	m := zzMap(c)
	return m[k] == "strict_target_ledger_complete"
}

func zzHelperMapCaller(c TraceDBCoverage) bool { return zzHelperMapRead(c, "decode_state") }

func zzHelperMapDirect(c TraceDBCoverage, k string) bool {
	return strings.HasPrefix(zzMap(c)[k], "withheld_")
}

func zzHelperMapDirectCaller(c TraceDBCoverage) bool { return zzHelperMapDirect(c, "decode_state") }

func zzHelperMapSpelled(c TraceDBCoverage) bool {
	switch zzMap(c)["decode_state"] {
	case "strict_target_ledger_complete":
		return true
	}
	return false
}

func zzShadowConst(c TraceDBCoverage) bool {
	traceDBRawDecodeStateComplete := "decode_state"
	return c.Metadata[traceDBRawDecodeStateComplete] == "strict_target_ledger_complete"
}

func zzShadowParam(c TraceDBCoverage, traceDBRawDecodeStateComplete string) bool {
	switch c.Metadata[traceDBRawDecodeStateComplete] {
	case "strict_target_ledger_complete":
		return true
	}
	return false
}

func zzShadowParamCaller(c TraceDBCoverage) bool { return zzShadowParam(c, "decode_state") }

func zzSelfRecursive(c TraceDBCoverage, k string, n int) bool {
	if n > 0 {
		return zzSelfRecursive(c, k, n-1)
	}
	return c.Metadata[k] == "strict_target_ledger_complete"
}

func zzMutualA(c TraceDBCoverage, k string, n int) bool {
	if n > 0 {
		return zzMutualB(c, k, n-1)
	}
	return c.Metadata[k] == "strict_target_ledger_complete"
}

func zzMutualB(c TraceDBCoverage, k string, n int) bool {
	if n > 0 {
		return zzMutualA(c, k, n-1)
	}
	return c.Metadata[k] == "withheld_visibility_envelope_incomplete"
}
`

// traceDBStateWriteTupleKeyProbe is the write probe of the EVOLUTION RECORD
// above: a tuple re-bound local as the Metadata key of a constant write.
const traceDBStateWriteTupleKeyProbe = `package hitraceconv

func zzPickKey() (string, bool) { return "publication_state", true }

func zzTupleKeyWrite(out *TraceDBCoverage) {
	k := "reason"
	k, _ = zzPickKey()
	out.Metadata[k] = "strict_target_ledger_complete"
}
`

// TestTraceDBLaneStateWriteSitesResolveKeysThroughTheReaderLanes (round
// seven, #5 / #6): the write census resolves a Metadata key through the
// reader's lanes — a single-valued local, a caller-resolved parameter, a
// `string(<constant>)` conversion, a once-bound conversion of a typed
// parameter, a carried set, a gate call's key argument — and a resolved lane
// key is a site like a literal one whatever the value: it reaches the
// prefix roster and the gate-before-write rule. An unresolvable key over the
// state map is red whatever the value; a concatenation whose literal prefix
// excludes every lane key, and an every-key forwarding loop, are not lane
// writes (residual (a) restated on the key). Bindings are keyed by
// declaration: same-named methods in either order keep their own (#6).
// Through 42fcf3fd1 every non-literal key was silent (see the EVOLUTION
// RECORD on TestTraceDBStateReadCycleLiteralAndCopyShapesEvadedTheBaseCensus).
func TestTraceDBLaneStateWriteSitesResolveKeysThroughTheReaderLanes(t *testing.T) {
	const header = "package hitraceconv\nfunc zzPick() (string, bool) { return \"publication_state\", true }\nfunc zzFormat() string { return \"x\" }\n"
	type expect struct {
		name    string
		src     string
		sites   []string // line:key=value[ gate]
		reports []string
		ungated int // sites the gate-before-write rule reports
		roster  int // sites whose value matches no roster prefix, or more than one
	}
	for _, test := range []expect{
		{name: "single_local_lane_key", src: header + `
func zz(out *TraceDBCoverage) { k := "publication_state"; out.Metadata[k] = "strict_target_ledger_complete" }
`, sites: []string{"5:publication_state=strict_target_ledger_complete"}, ungated: 1, roster: 1},
		{name: "param_key_carried_from_caller", src: header + `
func zzW(out *TraceDBCoverage, k string) { out.Metadata[k] = "strict_target_ledger_complete" }
func zz(out *TraceDBCoverage) { zzW(out, "publication_state") }
`, sites: []string{"5:publication_state=strict_target_ledger_complete"}, ungated: 1, roster: 1},
		{name: "typed_constant_conversion_key", src: header + `
func zz(out *TraceDBCoverage) { out.Metadata[string(traceDBSourceRawLaneStateKeyPublication)] = "strict_target_ledger_complete" }
`, sites: []string{"5:publication_state=strict_target_ledger_complete"}, ungated: 1, roster: 1},
		{name: "typed_param_converted_through_local", src: header + `
func zzW(out *TraceDBCoverage, key traceDBSourceRawLaneStateKey) { stateKey := string(key); out.Metadata[stateKey] = "strict_target_ledger_complete" }
func zz(out *TraceDBCoverage) { zzW(out, traceDBSourceRawLaneStateKeyJoin) }
`, sites: []string{"5:join_state=strict_target_ledger_complete"}, ungated: 1, roster: 1},
		{name: "carried_set_mints_each_lane_key", src: header + `
func zzW(out *TraceDBCoverage, k traceDBSourceRawLaneStateKey) { out.Metadata[string(k)] = "complete_probe" }
func zz(out *TraceDBCoverage) { zzW(out, traceDBSourceRawLaneStateKeyJoin); zzW(out, traceDBSourceRawLaneStateKeyLedger) }
`, sites: []string{"5:join_state=complete_probe", "5:ledger_state=complete_probe"}, ungated: 2},
		{name: "shadowed_constant_copied_into_key", src: header + `
func zz(out *TraceDBCoverage) { traceDBRawDecodeStateComplete := "publication_state"; k := traceDBRawDecodeStateComplete; out.Metadata[k] = "complete_probe" }
`, sites: []string{"5:publication_state=complete_probe"}, ungated: 1},
		{name: "gate_call_key_carried_from_caller", src: header + `
func zzGate(out *TraceDBCoverage, inventory *traceDBSourceNameInventory, k traceDBSourceRawLaneStateKey) { traceDBApplySourceRawLaneGateKeyed(out, inventory, k, "probe") }
func zz(out *TraceDBCoverage, inventory *traceDBSourceNameInventory) { zzGate(out, inventory, traceDBSourceRawLaneStateKeyJoin) }
`, sites: []string{"5:join_state=census_incomplete_source_raw_decode gate", "5:join_state=not_applicable_source_profile gate"}},
		{name: "gate_call_key_param_without_caller", src: header + `
func zzGate(out *TraceDBCoverage, inventory *traceDBSourceNameInventory, k traceDBSourceRawLaneStateKey) { traceDBApplySourceRawLaneGateKeyed(out, inventory, k, "probe") }
`, reports: []string{"zz_probe.go:5: gate call traceDBApplySourceRawLaneGateKeyed key argument does not resolve to a declared lane key"}},
		{name: "unresolved_key_unresolvable_value", src: header + `
func zz(out *TraceDBCoverage) { k, _ := zzPick(); out.Metadata[k] = zzFormat() }
`, reports: []string{"zz_probe.go:5: a value the census cannot resolve written under a Metadata key the census cannot resolve (k)"}},
		{name: "concat_bound_key_state_suffix_unresolved", src: header + `
func zz(out *TraceDBCoverage, prefix string) { key := prefix + "_state"; out.Metadata[key] = zzFormat() }
`, reports: []string{"zz_probe.go:5: a value the census cannot resolve written under a Metadata key the census cannot resolve (key)"}},
		{name: "every_key_state_shaped_value", src: header + `
func zz(out *TraceDBCoverage, c TraceDBCoverage) { for k := range c.Metadata { out.Metadata[k] = "complete_probe" } }
`, reports: []string{`zz_probe.go:5: state-shaped value "complete_probe" written under a computed Metadata key`}},
		{name: "compared_range_key_unresolvable_value", src: header + `
func zz(out *TraceDBCoverage, c TraceDBCoverage) { for k, v := range c.Metadata { if k == "publication_state" { out.Metadata[k] = v } } }
`, reports: []string{"zz_probe.go:5: publication_state written through an unrecognized expression shape"}},
		{name: "shadowed_constant_value_through_local", src: header + `
func zz(out *TraceDBCoverage) { traceDBSourceRawLaneCensusIncompleteState := zzFormat(); out.Metadata["publication_state"] = traceDBSourceRawLaneCensusIncompleteState }
`, reports: []string{"zz_probe.go:5: publication_state written through an unrecognized expression shape"}},
		{name: "same_named_methods_single_then_tuple", src: header + `
type zzA struct{}
type zzB struct{}
func (zzA) write(out *TraceDBCoverage) { k := "reason"; out.Metadata[k] = "strict_target_ledger_complete" }
func (zzB) write(out *TraceDBCoverage) { k, _ := zzPick(); out.Metadata[k] = "strict_target_ledger_complete" }
`, reports: []string{`zz_probe.go:8: "strict_target_ledger_complete" written under a Metadata key the census cannot resolve (k)`}},
		{name: "same_named_methods_tuple_then_single", src: header + `
type zzA struct{}
type zzB struct{}
func (zzA) write(out *TraceDBCoverage) { k, _ := zzPick(); out.Metadata[k] = "strict_target_ledger_complete" }
func (zzB) write(out *TraceDBCoverage) { k := "reason"; out.Metadata[k] = "strict_target_ledger_complete" }
`, reports: []string{`zz_probe.go:7: "strict_target_ledger_complete" written under a Metadata key the census cannot resolve (k)`}},
		// Green controls: the excluded concatenation (the live 2320 shape),
		// the every-key forwarding loop, a spelled non-key with a non-state
		// value.
		{name: "round_seven_green_shapes", src: header + `
func zzConcat(out *TraceDBCoverage, suffix string) { key := "official_viewer_" + suffix; out.Metadata[key] = zzFormat() }
func zzForward(out *TraceDBCoverage, c TraceDBCoverage) { for k, v := range c.Metadata { out.Metadata[k] = v } }
func zzNonKey(out *TraceDBCoverage) { k := "reason"; out.Metadata[k] = "strict_target_ledger_complete" }
`},
	} {
		t.Run(test.name, func(t *testing.T) {
			consts, files, fset := traceDBCensusFilesWithProbe(t, test.src)
			recorder := &traceDBRecordingTB{TB: t}
			all, _ := traceDBLaneStateWriteSitesOf(recorder, consts, files, fset)
			var sites []string
			roster := 0
			for _, site := range all {
				if site.file != "zz_probe.go" {
					continue
				}
				text := fmt.Sprintf("%d:%s=%s", site.line, site.key, site.value)
				if site.viaGate {
					text += " gate"
				}
				sites = append(sites, text)
				if traceDBPublicationStatePrefixMatches(site.value) != 1 {
					roster++
				}
			}
			ungated := len(traceDBProbeProblems(traceDBUngatedLaneKeyWriters(all)))
			reports := traceDBProbeProblems(recorder.problems)
			if strings.Join(sites, "\n") != strings.Join(test.sites, "\n") || strings.Join(reports, "\n") != strings.Join(test.reports, "\n") ||
				ungated != test.ungated || roster != test.roster {
				t.Fatalf("sites=%v reports=%v ungated=%d roster=%d\nwant sites=%v reports=%v ungated=%d roster=%d",
					sites, reports, ungated, roster, test.sites, test.reports, test.ungated, test.roster)
			}
		})
	}
}

// TestTraceDBUnresolvedKeyForwardingRosterIsLoadBearing (round seven, #5):
// the disclosed residual is bound to the live tree — with the roster
// withdrawn the write census names exactly the two forwarding writes of
// streamerdb_metadata.go (the device-info fields projected from a struct
// list, the parser metadata rows), a roster entry no live write matches is
// red, and the roster as declared draws no report.
func TestTraceDBUnresolvedKeyForwardingRosterIsLoadBearing(t *testing.T) {
	consts, files, fset := traceDBPackageStringConsts(t)
	census := func() []string {
		recorder := &traceDBRecordingTB{TB: t}
		traceDBLaneStateWriteSitesOf(recorder, consts, files, fset)
		sort.Strings(recorder.problems)
		return recorder.problems
	}
	if problems := census(); len(problems) != 0 {
		t.Fatalf("live tree reported %v", problems)
	}
	declared := map[traceDBForwardingWrite]bool{}
	for entry := range traceDBUnresolvedKeyForwardingWrites {
		declared[entry] = true
		delete(traceDBUnresolvedKeyForwardingWrites, entry)
	}
	defer func() {
		for entry := range traceDBUnresolvedKeyForwardingWrites {
			delete(traceDBUnresolvedKeyForwardingWrites, entry)
		}
		for entry := range declared {
			traceDBUnresolvedKeyForwardingWrites[entry] = true
		}
	}()
	if len(declared) != 2 {
		t.Fatalf("roster declares %d writes, want the two of streamerdb_metadata.go", len(declared))
	}
	want := []string{
		"streamerdb_metadata.go:186: a value the census cannot resolve written under a Metadata key the census cannot resolve (name)",
		"streamerdb_metadata.go:82: a value the census cannot resolve written under a Metadata key the census cannot resolve (field.name)",
	}
	if got := census(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("with the roster withdrawn the live tree reported\n%s\nwant exactly\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for entry := range declared {
		traceDBUnresolvedKeyForwardingWrites[entry] = true
	}
	stale := traceDBForwardingWrite{file: "source_raw_lane_gate.go", function: "traceDBMintSourceRawLaneGateOutcome", key: "stateKey"}
	traceDBUnresolvedKeyForwardingWrites[stale] = true
	wantStale := "source_raw_lane_gate.go: the forwarding-write roster names traceDBMintSourceRawLaneGateOutcome[stateKey], which no live write matches"
	if got := census(); len(got) != 1 || got[0] != wantStale {
		t.Fatalf("with a stale roster entry the live tree reported %v\nwant exactly %q", got, wantStale)
	}
}

// TestTraceDBStateReadCycleLiteralAndCopyShapesEvadedTheBaseCensus is the
// EVOLUTION RECORD of round seven (§40.53 收编复核七轮, findings #3 / #4 / #5
// / #6): one probe file carrying the in-cycle literal, shadow-copy,
// parameter-copy and concatenation-copy read shapes, fed to the reader
// census as a non-test file, plus a write probe carrying the resolved-key
// write shapes and the same-named-method pair.
//
//   - read probe, 42fcf3fd1 (this round's base): the in-cycle literal was
//     dropped with its site (the self-recursion silent, only
//     zzMutualLiteralB red), the copies of the shadowing local and
//     parameter resolved to the package constant's value (silent), and the
//     once-bound conversion and concatenation were multi-valued locals
//     (both red as "cannot resolve") — 3 problems, reads +1.
//   - read probe, 6f98f839d: the in-cycle literal resolved (the self read
//     and both mutual reads red), the copies silent, the conversion and
//     concatenation unresolved — 5 problems, reads +3.
//   - read probe, 533a939fb / b6f7eeec3 / 480939385: no parameter or local
//     resolution at all — 0 problems, reads +0.
//   - write probe, every base: the local and parameter keys minted no site
//     and no report (literal keys only), the shadowed constant's value was
//     fabricated into a publication_state site
//     (census_incomplete_source_raw_decode), the unresolvable key with an
//     unresolvable value and the tuple-keyed method behind its same-named
//     neighbour were silent — 1 site, 0 reports, and the write census green
//     at 6f98f839d and before; at 42fcf3fd1 the same site plus one false
//     report, the once-bound conversion key filed as a key the census
//     cannot resolve (stateKey).
//   - now: every read site of the probe is named and nothing else; the
//     probe adds exactly its six genuine reads (the self-cycle read, the two
//     mutual-cycle reads, the two shadow-copy reads, the conversion-copy
//     read; the excluded concatenation is not a read); the write probe
//     mints exactly its three resolved-key sites and draws exactly its
//     three reports.
func TestTraceDBStateReadCycleLiteralAndCopyShapesEvadedTheBaseCensus(t *testing.T) {
	consts, files, fset := traceDBPackageStringConsts(t)
	problems, live := traceDBStateReadProblems(files, fset, consts)
	if len(problems) != 0 {
		t.Fatalf("live tree reported %v", problems)
	}
	consts, files, fset = traceDBCensusFilesWithProbe(t, traceDBStateReadCycleLiteralCopyProbe)
	problems, reads := traceDBStateReadProblems(files, fset, consts)
	got := traceDBProbeProblems(problems)
	want := []string{ // sorted as strings
		"zz_probe.go:16: literal comparison over publication_state; readers classify through the gate table",
		"zz_probe.go:23: literal comparison over publication_state; readers classify through the gate table",
		"zz_probe.go:31: literal comparison over decode_state; readers classify through the gate table",
		"zz_probe.go:36: hand-kept switch over decode_state; readers classify through the gate table",
		"zz_probe.go:47: literal comparison over publication_state; readers classify through the gate table",
		"zz_probe.go:7: literal comparison over decode_state; readers classify through the gate table",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("problems=\n%s\nwant=\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if reads != live+6 {
		t.Fatalf("cycle-literal/copy probe moved the read count %d → %d, want +6 genuine reads", live, reads)
	}
	consts, files, fset = traceDBCensusFilesWithProbe(t, traceDBStateWriteResolvedKeyProbe)
	recorder := &traceDBRecordingTB{TB: t}
	all, _ := traceDBLaneStateWriteSitesOf(recorder, consts, files, fset)
	var sites []string
	for _, site := range all {
		if site.file == "zz_probe.go" {
			sites = append(sites, fmt.Sprintf("%d:%s=%s", site.line, site.key, site.value))
		}
	}
	wantSites := []string{
		"5:publication_state=strict_target_ledger_complete",
		"9:publication_state=strict_target_ledger_complete",
		"16:join_state=strict_target_ledger_complete",
	}
	if strings.Join(sites, "\n") != strings.Join(wantSites, "\n") {
		t.Fatalf("write probe minted\n%s\nwant\n%s", strings.Join(sites, "\n"), strings.Join(wantSites, "\n"))
	}
	reports := traceDBProbeProblems(recorder.problems)
	wantReports := []string{
		"zz_probe.go:29: a value the census cannot resolve written under a Metadata key the census cannot resolve (k)",
		"zz_probe.go:39: publication_state written through an unrecognized expression shape",
		`zz_probe.go:48: "strict_target_ledger_complete" written under a Metadata key the census cannot resolve (k)`,
	}
	if strings.Join(reports, "\n") != strings.Join(wantReports, "\n") {
		t.Fatalf("write probe reported\n%s\nwant\n%s", strings.Join(reports, "\n"), strings.Join(wantReports, "\n"))
	}
}

// traceDBStateReadCycleLiteralCopyProbe is the read probe of the EVOLUTION
// RECORD above, kept verbatim so the base runs and this pin read the same
// bytes.
const traceDBStateReadCycleLiteralCopyProbe = `package hitraceconv

func zzCycleLiteral(c TraceDBCoverage, k string, n int) bool {
	if n > 0 {
		return zzCycleLiteral(c, "decode_state", n-1)
	}
	return c.Metadata[k] == "strict_target_ledger_complete"
}

func zzCycleLiteralCaller(c TraceDBCoverage) bool { return zzCycleLiteral(c, "reason", 1) }

func zzMutualLiteralA(c TraceDBCoverage, k string, n int) bool {
	if n > 0 {
		return zzMutualLiteralB(c, "publication_state", n-1)
	}
	return c.Metadata[k] == "withheld_visibility_envelope_incomplete"
}

func zzMutualLiteralB(c TraceDBCoverage, k string, n int) bool {
	if n > 0 {
		return zzMutualLiteralA(c, k, n-1)
	}
	return c.Metadata[k] == "withheld_visibility_envelope_incomplete"
}

func zzMutualLiteralCaller(c TraceDBCoverage) bool { return zzMutualLiteralA(c, "reason", 1) }

func zzShadowCopy(c TraceDBCoverage) bool {
	traceDBRawDecodeStateComplete := "decode_state"
	k := traceDBRawDecodeStateComplete
	return c.Metadata[k] == "strict_target_ledger_complete"
}

func zzShadowParamCopy(c TraceDBCoverage, traceDBRawDecodeStateComplete string) bool {
	k := traceDBRawDecodeStateComplete
	switch c.Metadata[k] {
	case "strict_target_ledger_complete":
		return true
	}
	return false
}

func zzShadowParamCopyCaller(c TraceDBCoverage) bool { return zzShadowParamCopy(c, "decode_state") }

func zzConversionCopy(c TraceDBCoverage, key traceDBSourceRawLaneStateKey) bool {
	stateKey := string(key)
	return c.Metadata[stateKey] == "withheld_visibility_envelope_incomplete"
}

func zzConversionCopyCaller(c TraceDBCoverage) bool {
	return zzConversionCopy(c, traceDBSourceRawLaneStateKeyPublication)
}

func zzConcatCopy(c TraceDBCoverage, family string) bool {
	k := "retention_" + family + "_state"
	return c.Metadata[k] == "strict_target_ledger_complete"
}
`

// traceDBStateWriteResolvedKeyProbe is the write probe of the EVOLUTION
// RECORD above: lane keys spelled through a local, a caller-resolved
// parameter and a once-bound conversion; an unresolvable key with an
// unresolvable value; the excluded concatenation; a shadowed constant's
// value; and a same-named method pair (tuple-keyed first).
const traceDBStateWriteResolvedKeyProbe = `package hitraceconv

func zzLocalKeyWrite(out *TraceDBCoverage) {
	k := "publication_state"
	out.Metadata[k] = "strict_target_ledger_complete"
}

func zzParamKeyWrite(out *TraceDBCoverage, k string) {
	out.Metadata[k] = "strict_target_ledger_complete"
}

func zzParamKeyWriteCaller(out *TraceDBCoverage) { zzParamKeyWrite(out, "publication_state") }

func zzConversionKeyWrite(out *TraceDBCoverage, key traceDBSourceRawLaneStateKey) {
	stateKey := string(key)
	out.Metadata[stateKey] = "strict_target_ledger_complete"
}

func zzConversionKeyWriteCaller(out *TraceDBCoverage) {
	zzConversionKeyWrite(out, traceDBSourceRawLaneStateKeyJoin)
}

func zzPickKey() (string, bool) { return "publication_state", true }

func zzFormatValue() string { return "x" }

func zzUnresolvedKeyWrite(out *TraceDBCoverage) {
	k, _ := zzPickKey()
	out.Metadata[k] = zzFormatValue()
}

func zzConcatKeyWrite(out *TraceDBCoverage, suffix string) {
	key := "official_viewer_typed_only_sync_witnesses_" + suffix
	out.Metadata[key] = zzFormatValue()
}

func zzShadowValueWrite(out *TraceDBCoverage) {
	traceDBSourceRawLaneCensusIncompleteState := zzFormatValue()
	out.Metadata["publication_state"] = traceDBSourceRawLaneCensusIncompleteState
}

type zzWriterA struct{}

type zzWriterB struct{}

func (zzWriterA) write(out *TraceDBCoverage) {
	k, _ := zzPickKey()
	out.Metadata[k] = "strict_target_ledger_complete"
}

func (zzWriterB) write(out *TraceDBCoverage) {
	k := "reason"
	out.Metadata[k] = "strict_target_ledger_complete"
}
`

// TestTraceDBStateReadInheritFunnelResolvesThroughItsCallers (round five,
// #6): the live inherit funnel reads the upstream lane's state through its
// key parameter (`from.Metadata[string(fromKey)]`, a hand-kept switch by
// shape). The census resolves that parameter through the key argument of
// every caller — today the ledger and join keys, outside the read set — so
// the live tree is green; a caller passing the publication key turns
// exactly that switch red, so the resolution is bound to the callers and
// cannot go silently green.
func TestTraceDBStateReadInheritFunnelResolvesThroughItsCallers(t *testing.T) {
	const caller = "package hitraceconv\n\nfunc zzInheritPublication(out *TraceDBCoverage, c TraceDBCoverage) bool {\n\treturn traceDBInheritSourceRawLaneGate(out, c, traceDBSourceRawLaneStateKeyPublication, traceDBSourceRawLaneStateKeyReconciliation, \"probe\")\n}\n"
	consts, files, fset := traceDBCensusFilesWithProbe(t, caller)
	problems, _ := traceDBStateReadProblems(files, fset, consts)
	want := "source_raw_lane_gate.go:248: hand-kept switch over publication_state; readers classify through the gate table"
	if len(problems) != 1 || problems[0] != want {
		t.Fatalf("with a publication-key caller the live tree reported %v\nwant exactly %q", problems, want)
	}
}

// TestTraceDBRawSchedSwitchLiteJoinReconcilableIsTotalOverTheLaneVocabulary:
// the reconciliation consumer classifies join_state through
// traceDBRawSchedSwitchLiteJoinReconcilable; the table and the set of values
// the switch join writes under join_state (plain write sites plus the two
// funnel outcomes of its gate call, from the write-site census) are the same
// set, both ways.
func TestTraceDBRawSchedSwitchLiteJoinReconcilableIsTotalOverTheLaneVocabulary(t *testing.T) {
	sites, _ := traceDBLaneStateWriteSites(t)
	for _, problem := range traceDBJoinStateTableProblems(sites, traceDBRawSchedSwitchLiteJoinReconcilable) {
		t.Error(problem)
	}
	// Self-red: a written value absent from the table, and a table arm no
	// write site mints.
	extra := append([]traceDBStateWriteSite(nil), sites...)
	extra = append(extra, traceDBStateWriteSite{file: "source_raw_scheduler_lite_join.go", line: 1, key: "join_state", value: "complete_new_arm", function: "zz"})
	problems := traceDBJoinStateTableProblems(extra, traceDBRawSchedSwitchLiteJoinReconcilable)
	if len(problems) != 1 || !strings.Contains(problems[0], `writes join_state "complete_new_arm", which has no arm`) {
		t.Fatalf("undeclared written value not reported: %v", problems)
	}
	wider := map[string]bool{"published_stale_arm": true}
	for state, value := range traceDBRawSchedSwitchLiteJoinReconcilable {
		wider[state] = value
	}
	problems = traceDBJoinStateTableProblems(sites, wider)
	if len(problems) != 1 || !strings.Contains(problems[0], `names join_state "published_stale_arm", which no switch join write site mints`) {
		t.Fatalf("stale table arm not reported: %v", problems)
	}
}

func traceDBJoinStateTableProblems(sites []traceDBStateWriteSite, table map[string]bool) []string {
	written := map[string]int{}
	for _, site := range sites {
		if site.file != "source_raw_scheduler_lite_join.go" || site.key != "join_state" || site.constructor {
			continue
		}
		if _, ok := written[site.value]; !ok {
			written[site.value] = site.line
		}
	}
	var problems []string
	for value, line := range written {
		if _, ok := table[value]; !ok {
			problems = append(problems, fmt.Sprintf("source_raw_scheduler_lite_join.go:%d: writes join_state %q, which has no arm in traceDBRawSchedSwitchLiteJoinReconcilable", line, value))
		}
	}
	for value := range table {
		if _, ok := written[value]; !ok {
			problems = append(problems, fmt.Sprintf("traceDBRawSchedSwitchLiteJoinReconcilable names join_state %q, which no switch join write site mints", value))
		}
	}
	if len(written) < 8 {
		problems = append(problems, fmt.Sprintf("switch join write census found only %d join_state values; the walk drifted", len(written)))
	}
	sort.Strings(problems)
	return problems
}
