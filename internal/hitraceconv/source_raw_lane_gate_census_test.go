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
//     lane's coverage inherits; fold-in #7).

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
func traceDBTypedStringConsts(t *testing.T, files map[string]*ast.File, consts map[string]string, typeName string) map[string]string {
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

// traceDBResolveStateExpr resolves a state-valued expression at a write site:
// a string literal, a package constant (or a local binding handed in through
// consts), or `string(<constant>)`. Any other shape is unrecognized and the
// caller must fail loud.
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

// traceDBLocalStringBindings collects, per top-level function, the local
// identifiers bound to exactly one resolvable string (`x := "lit"`,
// `x = const`, `var x = "lit"`); an identifier bound more than once to
// different values is left unresolved (a variable, not a constant).
func traceDBLocalStringBindings(file *ast.File, consts map[string]string) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		out[fn.Name.Name] = traceDBFuncStringBindings(fn, consts)
	}
	return out
}

// traceDBFuncStringBindings is the per-function half of
// traceDBLocalStringBindings, shared with the reader census (which keys its
// state by declaration, not by name).
func traceDBFuncStringBindings(fn *ast.FuncDecl, consts map[string]string) map[string]string {
	locals := map[string]string{}
	conflict := map[string]bool{}
	bind := func(name string, expr ast.Expr) {
		value, ok := traceDBResolveStateExpr(expr, consts)
		if !ok {
			conflict[name] = true
			return
		}
		if prior, seen := locals[name]; seen && prior != value {
			conflict[name] = true
			return
		}
		locals[name] = value
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.AssignStmt:
			if len(n.Lhs) != len(n.Rhs) {
				return true
			}
			for i, lhs := range n.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					bind(ident.Name, n.Rhs[i])
				}
			}
		case *ast.ValueSpec:
			for i, name := range n.Names {
				if i < len(n.Values) {
					bind(name.Name, n.Values[i])
				}
			}
		}
		return true
	})
	for name := range conflict {
		delete(locals, name)
	}
	return locals
}

// traceDBLaneStateKeyTypeName is the typed lane state key; a parameter of
// this type (or of string) indexing the state map is resolved through every
// caller's argument by the reader census, and the type's constants are the
// lane key closed set of the write census.
const traceDBLaneStateKeyTypeName = "traceDBSourceRawLaneStateKey"

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

// traceDBLaneStateWriteSites walks every non-test file and returns every
// write of a lane state key (the typed traceDBSourceRawLaneStateKey constants):
//
//   - `<x>.Metadata["<key>"] = <expr>` assignments (value resolved through
//     package constants and single-binding locals);
//   - `"<key>": <expr>` composite-literal entries (coverage constructors);
//   - calls of the visibility lane's typed setter;
//   - calls of the gate funnel or one of its wrappers, expanded to the values
//     the funnel body mints under the key argument;
//   - the marker-async ledger's `ledger.state = <expr>` writes, published
//     under raw_async_replacement_state by applyCoverage.
//
// The funnel body (`out.Metadata[stateKey] = <const>`), the visibility
// setter body, applyCoverage's forwarding of ledger.state and the
// ledger.state forwarding of the gate coverage are the recognized non-constant
// writes; every other unresolvable RHS, every gate call whose key argument
// does not resolve to a declared key, and every state-shaped value written
// under a computed Metadata key is reported as a failure (fail-loud on
// unrecognized shapes).
func traceDBLaneStateWriteSites(t *testing.T) ([]traceDBStateWriteSite, map[string]string) {
	t.Helper()
	consts, files, fset := traceDBPackageStringConsts(t)
	return traceDBLaneStateWriteSitesOf(t, consts, files, fset)
}

// traceDBLaneStateWriteSitesOf is the write-site census over an explicit
// file map (the package's non-test files, plus a synthetic file in a
// self-red).
func traceDBLaneStateWriteSitesOf(t *testing.T, consts map[string]string, files map[string]*ast.File, fset *token.FileSet) ([]traceDBStateWriteSite, map[string]string) {
	t.Helper()
	laneKeys := traceDBTypedStringConsts(t, files, consts, traceDBLaneStateKeyTypeName)
	if len(laneKeys) < 7 {
		t.Fatalf("lane state key closed set lost members: %v", laneKeys)
	}
	stateShaped := func(value string) bool {
		if value == traceDBSourceRawLanePlaceholderState {
			return true
		}
		for _, entry := range traceDBSourceRawPublicationStatePrefixes {
			if strings.HasPrefix(value, entry.Prefix) {
				return true
			}
		}
		return false
	}
	// Pass one: the funnel's minted values.
	funnelValues := map[string]bool{}
	gateFile := files["source_raw_lane_gate.go"]
	if gateFile == nil {
		t.Fatal("source_raw_lane_gate.go not parsed")
	}
	traceDBInspectFuncBodies(gateFile, func(funcName string, node ast.Node) {
		stmt, ok := node.(*ast.AssignStmt)
		if !ok || funcName != traceDBSourceRawGateFunnelName {
			return
		}
		index, rhs, ok := traceDBMetadataIndexAssignment(stmt)
		if !ok {
			return
		}
		if ident, ok := index.(*ast.Ident); ok && ident.Name == "stateKey" {
			value, ok := traceDBResolveStateExpr(rhs, consts)
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
	var sites []traceDBStateWriteSite
	setterFunnels, asyncFunnels := 0, 0
	for name, file := range files {
		locals := traceDBLocalStringBindings(file, consts)
		traceDBInspectFuncBodies(file, func(funcName string, node ast.Node) {
			line := func(n ast.Node) int { return fset.Position(n.Pos()).Line }
			resolve := func(expr ast.Expr) (string, bool) {
				if value, ok := traceDBResolveStateExpr(expr, consts); ok {
					return value, true
				}
				if ident, ok := expr.(*ast.Ident); ok {
					value, ok := locals[funcName][ident.Name]
					return value, ok
				}
				return "", false
			}
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
				if key, ok := traceDBStringLiteral(index); ok {
					if _, lane := laneKeys[key]; !lane {
						return
					}
					if funcName == "traceDBSetSourceRawVisibilityState" && key == string(traceDBSourceRawLaneStateKeyPublication) {
						setterFunnels++
						return
					}
					if funcName == "applyCoverage" && key == string(traceDBSourceRawLaneStateKeyAsyncReplacement) {
						if sel, ok := rhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "state" {
							asyncFunnels++
							return
						}
					}
					value, ok := resolve(rhs)
					if !ok {
						t.Errorf("%s:%d: %s written through an unrecognized expression shape", name, line(n), key)
						return
					}
					sites = append(sites, traceDBStateWriteSite{file: name, line: line(n), key: key, value: value, function: funcName})
					return
				}
				// Computed Metadata key: the funnel's own write is recognized;
				// anywhere else a state-shaped value under a computed key is an
				// evasion of the census.
				if ident, ok := index.(*ast.Ident); ok && ident.Name == "stateKey" && funcName == traceDBSourceRawGateFunnelName {
					return
				}
				if value, ok := resolve(rhs); ok && stateShaped(value) {
					t.Errorf("%s:%d: state-shaped value %q written under a computed Metadata key", name, line(n), value)
				}
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
				key := string(traceDBSourceRawLaneStateKeyPublication)
				if keyArg >= 0 {
					if len(n.Args) <= keyArg {
						t.Errorf("%s:%d: gate call %s arity drifted", name, line(n), fun.Name)
						return
					}
					if ident, ok := n.Args[keyArg].(*ast.Ident); ok && ident.Name == "key" {
						if _, wrapper := traceDBSourceRawGateCallers[funcName]; wrapper {
							return // the wrapper forwards its own key parameter
						}
					}
					resolved, ok := resolve(n.Args[keyArg])
					if !ok {
						t.Errorf("%s:%d: gate call %s key argument does not resolve to a declared lane key", name, line(n), fun.Name)
						return
					}
					key = resolved
				}
				if _, lane := laneKeys[key]; !lane {
					t.Errorf("%s:%d: gate call %s uses %q, which is not a declared lane key", name, line(n), fun.Name, key)
					return
				}
				for value := range funnelValues {
					sites = append(sites, traceDBStateWriteSite{file: name, line: line(n), key: key, value: value, viaGate: true, function: funcName})
				}
			}
		})
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
		traceDBInspectFuncBodies(file, func(funcName string, node ast.Node) {
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
