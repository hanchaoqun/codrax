package hitraceconv

import (
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
// structural pins. Every census here is bound by data flow over every shape
// it can meet and fails loud on a shape it does not recognize (§40.50): a
// state written through an expression the census cannot resolve is a red,
// never a silent skip.
//
//   - the decode_state closed set is total over the gate table and every
//     decode_state write site refers to a declared constant;
//   - the visibility lane's publication_state closed set: every member is
//     minted, every minted value is a member, the rows table and the ordered
//     roster are the same set, and the reader accepts exactly the declared
//     (member, row-count) pairs;
//   - every publication_state literal minted anywhere in the package starts
//     with exactly one prefix of the class roster.

// traceDBPackageStringConsts collects every package-level string constant of
// the non-test files of this package (value = literal, or another collected
// constant, resolved transitively) so a write site can be resolved by name.
func traceDBPackageStringConsts(t *testing.T) (map[string]string, map[string]*ast.File, *token.FileSet) {
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
	return consts, files, fset
}

// traceDBResolveStateExpr resolves a state-valued expression at a write site:
// a string literal, a package constant, or `string(<constant>)`. Any other
// shape is unrecognized and the caller must fail loud.
func traceDBResolveStateExpr(expr ast.Expr, consts map[string]string) (string, bool) {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(v.Value)
		return value, err == nil
	case *ast.Ident:
		value, ok := consts[v.Name]
		return value, ok
	case *ast.CallExpr:
		fun, ok := v.Fun.(*ast.Ident)
		if !ok || fun.Name != "string" || len(v.Args) != 1 {
			return "", false
		}
		return traceDBResolveStateExpr(v.Args[0], consts)
	}
	return "", false
}

// traceDBMetadataKeyAssignment reports whether stmt assigns
// `<x>.Metadata["<key>"] = <expr>` and returns the RHS.
func traceDBMetadataKeyAssignment(stmt *ast.AssignStmt, key string) (ast.Expr, bool) {
	if len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 || stmt.Tok != token.ASSIGN {
		return nil, false
	}
	index, ok := stmt.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return nil, false
	}
	sel, ok := index.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Metadata" {
		return nil, false
	}
	lit, ok := index.Index.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return nil, false
	}
	if got, err := strconv.Unquote(lit.Value); err != nil || got != key {
		return nil, false
	}
	return stmt.Rhs[0], true
}

// traceDBStateWriteSite is one resolved `publication_state` write.
type traceDBStateWriteSite struct {
	file  string
	line  int
	value string
}

// traceDBInspectFuncBodies walks every top-level function of file, handing
// each node to visit together with the enclosing function's name so a
// census can recognize a declared funnel (a setter whose body forwards its
// parameter) from a stray parameter-shaped write anywhere else.
func traceDBInspectFuncBodies(file *ast.File, visit func(funcName string, node ast.Node)) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			if node != nil {
				visit(fn.Name.Name, node)
			}
			return true
		})
	}
}

// traceDBPublicationStateWriteSites walks every non-test file and returns
// every publication_state write: direct `Metadata["publication_state"] =`
// assignments and calls of the visibility lane's typed setter. The setter's
// own body (the funnel forwarding its typed parameter) is the one recognized
// non-constant write; every other unresolvable RHS shape is reported as a
// failure (fail-loud on unrecognized shapes).
func traceDBPublicationStateWriteSites(t *testing.T) []traceDBStateWriteSite {
	t.Helper()
	consts, files, fset := traceDBPackageStringConsts(t)
	var sites []traceDBStateWriteSite
	setterFunnels := 0
	for name, file := range files {
		traceDBInspectFuncBodies(file, func(funcName string, node ast.Node) {
			switch n := node.(type) {
			case *ast.AssignStmt:
				rhs, ok := traceDBMetadataKeyAssignment(n, "publication_state")
				if !ok {
					return
				}
				if funcName == "traceDBSetSourceRawVisibilityState" {
					setterFunnels++
					return
				}
				value, ok := traceDBResolveStateExpr(rhs, consts)
				if !ok {
					t.Errorf("%s:%d: publication_state written through an unrecognized expression shape", name, fset.Position(n.Pos()).Line)
					return
				}
				sites = append(sites, traceDBStateWriteSite{file: name, line: fset.Position(n.Pos()).Line, value: value})
			case *ast.CallExpr:
				fun, ok := n.Fun.(*ast.Ident)
				if !ok || fun.Name != "traceDBSetSourceRawVisibilityState" || len(n.Args) != 2 {
					return
				}
				value, ok := traceDBResolveStateExpr(n.Args[1], consts)
				if !ok {
					t.Errorf("%s:%d: visibility state set through an unrecognized expression shape", name, fset.Position(n.Pos()).Line)
					return
				}
				sites = append(sites, traceDBStateWriteSite{file: name, line: fset.Position(n.Pos()).Line, value: value})
			}
		})
	}
	if setterFunnels != 1 {
		t.Fatalf("visibility state setter funnel count = %d, want exactly one forwarding write", setterFunnels)
	}
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].file != sites[j].file {
			return sites[i].file < sites[j].file
		}
		return sites[i].line < sites[j].line
	})
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
	// Every decode_state write site (setUnavailable's first argument, or a
	// direct Metadata["decode_state"] assignment) refers to a declared
	// constant; setUnavailable's own body is the one recognized funnel that
	// forwards its parameter.
	sites, funnels := 0, 0
	for name, file := range files {
		traceDBInspectFuncBodies(file, func(funcName string, node ast.Node) {
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
	// three direct assignments.
	if sites < 8 || funnels != 1 {
		t.Fatalf("decode_state write census: sites=%d funnels=%d", sites, funnels)
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
		inventory.RawDecode.Found = !found
		if got, _ := traceDBSourceRawLaneGate(&inventory); got != traceDBSourceRawGateUnset {
			t.Errorf("%s with contradicting Found=%t resolved to %d instead of Unset", state, !found, got)
		}
	}
	if kind, reason := traceDBSourceRawLaneGate(nil); kind != traceDBSourceRawGateNotApplicable || reason != "" {
		t.Fatalf("absent inventory: gate=%d reason=%q", kind, reason)
	}
}

func TestTraceDBSourceRawVisibilityStatesAreTheDeclaredClosedSet(t *testing.T) {
	consts, files, _ := traceDBPackageStringConsts(t)
	file := files["source_raw_visibility_recovery.go"]
	if file == nil {
		t.Fatal("source_raw_visibility_recovery.go not parsed")
	}
	declared := map[string]bool{}
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
			if ident, ok := vs.Type.(*ast.Ident); !ok || ident.Name != "traceDBSourceRawVisibilityState" {
				continue
			}
			for _, name := range vs.Names {
				value, ok := consts[name.Name]
				if !ok {
					t.Fatalf("visibility state constant %s does not resolve to a string", name.Name)
				}
				declared[value] = true
			}
		}
	}
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
		if !declared[value] {
			t.Errorf("allTraceDBSourceRawVisibilityStates names %q, which is not a declared constant", value)
		}
	}
	for state := range traceDBSourceRawVisibilityStateRows {
		if !declared[string(state)] {
			t.Errorf("row table names %q, which is not a declared constant", state)
		}
	}
	// Minted values: the visibility file's typed setter calls plus the two
	// gate outcomes minted for every lane by source_raw_lane_gate.go.
	minted := map[string]bool{}
	for _, site := range traceDBPublicationStateWriteSites(t) {
		if site.file == "source_raw_visibility_recovery.go" || site.file == "source_raw_lane_gate.go" {
			minted[site.value] = true
		}
	}
	for value := range declared {
		if !minted[value] {
			t.Errorf("declared state %q is never minted", value)
		}
	}
	for value := range minted {
		if !declared[value] {
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
		matches := 0
		for _, entry := range traceDBSourceRawPublicationStatePrefixes {
			if strings.HasPrefix(site.value, entry.Prefix) {
				matches++
			}
		}
		if matches != 1 {
			t.Errorf("%s:%d: publication_state %q matches %d roster prefixes, want exactly one", site.file, site.line, site.value, matches)
		}
	}
	for _, want := range []string{
		"source_raw_block_recovery.go", "source_raw_blocked_recovery.go", "source_raw_dma_lifecycle_recovery.go",
		"source_raw_dma_wait_recovery.go", "source_raw_exact_recovery.go", "source_raw_marker_sync_recovery.go",
		"source_raw_visibility_recovery.go", "source_raw_lane_gate.go",
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
