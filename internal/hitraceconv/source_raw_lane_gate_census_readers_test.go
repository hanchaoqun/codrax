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
//     — a direct `<x>.Metadata["<key>"]` or a local tainted by one, through
//     any chain of single-identifier bindings — lands in a recognized consumer
//     position (a lookup into a package-level table keyed by the decode_state
//     constants, a registered classifier call, a binding, a forwarding write,
//     prose concatenation / fmt, a return). A hand-kept switch/case, a ==/!=
//     comparison, a strings.* prefix/substring test, a lookup through a map
//     the census cannot see, or any other shape is red (§40.50).

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
// target, a declared name, a parameter/result.
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
// Classification: every read occurrence — a direct read, a tainted local, a
// call to a helper with a tainted result — is judged by its nearest non-paren
// ancestor, except that an occurrence directly under a concatenation or a
// fmt.Sprint* argument is judged where that laundered expression lands.
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
	strip := func(expr ast.Expr) ast.Expr {
		for {
			paren, ok := expr.(*ast.ParenExpr)
			if !ok {
				return expr
			}
			expr = paren.X
		}
	}
	directRead := func(expr ast.Expr) (string, bool) {
		index, ok := strip(expr).(*ast.IndexExpr)
		if !ok {
			return "", false
		}
		sel, ok := index.X.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Metadata" {
			return "", false
		}
		key, ok := traceDBResolveStateExpr(index.Index, consts)
		return key, ok && traceDBStateReadKeys[key]
	}
	report := func(name string, node ast.Node, format string, args ...interface{}) {
		problems = append(problems, fmt.Sprintf("%s:%d: ", name, fset.Position(node.Pos()).Line)+fmt.Sprintf(format, args...))
	}
	// walk visits node's subtree with the ancestor stack (nearest last).
	walk := func(root ast.Node, visit func(node ast.Node, stack []ast.Node)) {
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
	nearest := func(stack []ast.Node) ast.Node {
		for i := len(stack) - 1; i >= 0; i-- {
			if _, paren := stack[i].(*ast.ParenExpr); !paren {
				return stack[i]
			}
		}
		return nil
	}

	// Same-package helpers and the per-function taint state.
	type fnState struct {
		file    string
		imports map[string]bool
		locals  map[string]string // ident → key (sticky)
		results map[int]string    // result position → key
	}
	functions := map[string]*ast.FuncDecl{}
	methods := map[string][]*ast.FuncDecl{}
	states := map[*ast.FuncDecl]*fnState{}
	var order []*ast.FuncDecl
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
			states[fn] = &fnState{file: name, imports: imports, locals: map[string]string{}, results: map[int]string{}}
			order = append(order, fn)
			if fn.Recv != nil {
				methods[fn.Name.Name] = append(methods[fn.Name.Name], fn)
			} else {
				functions[fn.Name.Name] = fn
			}
		}
	}
	sort.Slice(order, func(i, j int) bool {
		if states[order[i]].file != states[order[j]].file {
			return states[order[i]].file < states[order[j]].file
		}
		return order[i].Pos() < order[j].Pos()
	})
	// calleeResults: the tainted result positions of a same-package helper
	// call (nil for anything the census cannot name).
	calleeResults := func(st *fnState, call *ast.CallExpr) map[int]string {
		switch f := strip(call.Fun).(type) {
		case *ast.Ident:
			if fn, ok := functions[f.Name]; ok {
				return states[fn].results
			}
		case *ast.SelectorExpr:
			if x, ok := f.X.(*ast.Ident); ok && st.imports[x.Name] {
				return nil
			}
			var merged map[int]string
			for _, m := range methods[f.Sel.Name] {
				for i, key := range states[m].results {
					if merged == nil {
						merged = map[int]string{}
					}
					merged[i] = key
				}
			}
			return merged
		}
		return nil
	}
	// taintOf: the key a single-valued expression carries, if any.
	var taintOf func(st *fnState, expr ast.Expr) (string, bool)
	taintOf = func(st *fnState, expr ast.Expr) (string, bool) {
		switch e := strip(expr).(type) {
		case *ast.IndexExpr:
			return directRead(e)
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
		walk(expr, func(node ast.Node, stack []ast.Node) {
			switch n := node.(type) {
			case *ast.IndexExpr:
				if _, ok := directRead(n); ok {
					found = true
				}
			case *ast.Ident:
				if _, ok := st.locals[n.Name]; ok && !traceDBIdentIsName(n, nearest(stack)) {
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
		switch e := strip(expr).(type) {
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
	unfollowed := func(st *fnState, rhs ast.Expr) {
		if !contains(st, rhs) || consumeShape(rhs) {
			return
		}
		bindingProblems[fmt.Sprintf("%s:%d: unrecognized binding shape (%T) over a state read; the census cannot follow the bound value",
			st.file, fset.Position(rhs.Pos()).Line, strip(rhs))] = true
	}
	bind := func(st *fnState, lhs, rhs ast.Expr) {
		if key, ok := taintOf(st, rhs); ok {
			bindValue(st, lhs, key)
			return
		}
		unfollowed(st, rhs)
	}
	bindTuple := func(st *fnState, lhs []ast.Expr, rhs ast.Expr) {
		if key, ok := directRead(rhs); ok { // comma-ok over a direct read
			bindValue(st, lhs[0], key)
			return
		}
		if call, ok := strip(rhs).(*ast.CallExpr); ok {
			if results := calleeResults(st, call); len(results) > 0 {
				for i, key := range results {
					if i < len(lhs) {
						bindValue(st, lhs[i], key)
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
	for changed {
		changed = false
		for _, fn := range order {
			st := states[fn]
			walk(fn.Body, func(node ast.Node, stack []ast.Node) {
				switch n := node.(type) {
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
					switch {
					case len(n.Results) == 0: // bare return over named results
						position := 0
						for _, field := range fn.Type.Results.List {
							for _, name := range field.Names {
								if key, ok := st.locals[name.Name]; ok {
									setResult(st, position, key)
								}
								position++
							}
						}
					case len(n.Results) == 1 && fn.Type.Results.NumFields() > 1: // forwarded tuple
						if call, ok := strip(n.Results[0]).(*ast.CallExpr); ok {
							for i, key := range calleeResults(st, call) {
								setResult(st, i, key)
							}
						}
					default:
						for i, result := range n.Results {
							if key, ok := taintOf(st, result); ok {
								setResult(st, i, key)
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
	for _, fn := range order {
		st := states[fn]
		name := st.file
		walk(fn.Body, func(node ast.Node, stack []ast.Node) {
			parent := nearest(stack)
			var key string
			leaf := true
			switch n := node.(type) {
			case *ast.IndexExpr:
				k, ok := directRead(n)
				if !ok {
					return
				}
				key = k
			case *ast.Ident:
				k, ok := st.locals[n.Name]
				if !ok || traceDBIdentIsName(n, parent) {
					return
				}
				key = k
			case *ast.CallExpr:
				if results := calleeResults(st, n); len(results) > 0 {
					positions := make([]int, 0, len(results))
					for i := range results {
						positions = append(positions, i)
					}
					sort.Ints(positions)
					key = results[positions[0]]
					break
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
			case *ast.AssignStmt, *ast.ValueSpec, *ast.ReturnStmt, *ast.KeyValueExpr, *ast.CompositeLit:
				// binding, forwarding into a field/metadata/literal, return
			case *ast.IndexExpr:
				if p.Index != node && strip(p.Index) != node {
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
