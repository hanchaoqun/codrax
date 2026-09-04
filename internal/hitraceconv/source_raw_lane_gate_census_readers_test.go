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

// traceDBStateReadProblems is the reader census over the given files.
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
	for name, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			// Taint: locals bound (transitively) to a state read. Sticky —
			// a later re-binding never launders the name.
			tainted := map[string]string{} // ident → key
			occurrence := func(expr ast.Expr) (string, bool) {
				if key, ok := directRead(expr); ok {
					return key, true
				}
				if ident, ok := strip(expr).(*ast.Ident); ok {
					key, ok := tainted[ident.Name]
					return key, ok
				}
				return "", false
			}
			for changed := true; changed; {
				changed = false
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					bind := func(lhs ast.Expr, rhs ast.Expr) {
						ident, ok := lhs.(*ast.Ident)
						if !ok || ident.Name == "_" {
							return
						}
						if key, ok := occurrence(rhs); ok && tainted[ident.Name] == "" {
							tainted[ident.Name] = key
							changed = true
						}
					}
					switch n := node.(type) {
					case *ast.AssignStmt:
						if len(n.Lhs) == len(n.Rhs) {
							for i := range n.Lhs {
								bind(n.Lhs[i], n.Rhs[i])
							}
						}
					case *ast.ValueSpec:
						for i := range n.Names {
							if i < len(n.Values) {
								bind(n.Names[i], n.Values[i])
							}
						}
					}
					return true
				})
			}
			// Classification: every occurrence by its nearest non-paren parent.
			var stack []ast.Node
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if node == nil {
					stack = stack[:len(stack)-1]
					return true
				}
				defer func() { stack = append(stack, node) }()
				var key string
				switch n := node.(type) {
				case *ast.IndexExpr:
					k, ok := directRead(n)
					if !ok {
						return true
					}
					key = k
				case *ast.Ident:
					k, ok := tainted[n.Name]
					if !ok {
						return true
					}
					if len(stack) > 0 {
						switch parent := stack[len(stack)-1].(type) {
						case *ast.SelectorExpr:
							if parent.Sel == n {
								return true // a field named like the local
							}
						case *ast.KeyValueExpr:
							if parent.Key == n {
								return true // a struct field key
							}
						case *ast.AssignStmt:
							for _, lhs := range parent.Lhs {
								if lhs == n {
									return true // the binding itself
								}
							}
						case *ast.ValueSpec:
							for _, declared := range parent.Names {
								if declared == n {
									return true
								}
							}
						case *ast.Field:
							return true // a parameter/result named like the local
						}
					}
					key = k
				default:
					return true
				}
				reads++
				// Nearest non-paren ancestor.
				var parent ast.Node
				for i := len(stack) - 1; i >= 0; i-- {
					if _, paren := stack[i].(*ast.ParenExpr); !paren {
						parent = stack[i]
						break
					}
				}
				switch p := parent.(type) {
				case *ast.AssignStmt, *ast.ValueSpec, *ast.ReturnStmt, *ast.KeyValueExpr, *ast.CompositeLit:
					// binding, forwarding into a field/metadata/literal, return
				case *ast.IndexExpr:
					if p.Index != node && strip(p.Index) != node {
						report(name, node, "unrecognized read shape (indexed) over %s", key)
						return true
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
					case traceDBStateReadClassifiers[fun], strings.HasPrefix(fun, "fmt."):
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
					case token.ADD:
					default:
						report(name, node, "unrecognized read shape (%s) over %s", p.Op, key)
					}
				default:
					report(name, node, "unrecognized read shape (%T) over %s", parent, key)
				}
				return true
			})
		}
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
