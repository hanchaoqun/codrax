package tool

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// emit_validator_list_discipline_census_test.go — V2-4 (§40.51) tripwire.
//
// Ruling (§40.27 row V2-4): every emit validator returns []violation, not the
// first one — a payload with N independent mistakes must cost ONE reject
// round, not N (EMITBURN-1 §29.173 discipline generalized from
// aggregate_facts to the patch transaction and its sibling validators).
//
// Recognizer (precise, syntactic): inside the scanned executor files and the
// internal/types validators they reach by call data flow, a FuncDecl whose
// results carry a violation slot (`error` or a pointer type) and whose body
// holds a loop (RangeStmt / ForStmt, closures excluded) with a ReturnStmt
// yielding a non-nil value in that slot is a FIRST-HIT VALIDATOR: the loop
// stops at its first violation. Every such function must be rostered as
// `serial_with_collector` (a slice-returning collector walks the whole payload
// and a parity pin proves collector[0] == serial error) or as `exception`
// (with a reason: internal invariant, lookup, gating normalizer). Functions
// converted to the list discipline are rostered as `list` and must NOT be
// recognized (a list row that turns first-hit again is red) and must return a
// slice. A return shape inside such a loop the classifier cannot key FAILS
// LOUD (§40.50: unrecognized shapes never pass silently). Stale rows (no such
// function / no longer first-hit) are red so the roster tracks the tree.

type emitValidatorRowClass string

const (
	emitValidatorList                emitValidatorRowClass = "list"
	emitValidatorSerialWithCollector emitValidatorRowClass = "serial_with_collector"
	emitValidatorException           emitValidatorRowClass = "exception"
)

type emitValidatorRow struct {
	class     emitValidatorRowClass
	collector string // serial_with_collector: the slice-returning walker
	parityPin string // serial_with_collector: test function proving collector[0] == serial error
	reason    string // exception: why the first hit is the right shape
}

// emitValidatorScannedFiles are the executor-side files whose FuncDecls are
// audited directly (internal/tool). internal/types is reached by data flow:
// every `types.<Name>(…)` callee from these files, then the call closure
// inside internal/types (bare names and method selectors, bounded depth).
var emitValidatorScannedFiles = []string{
	"emit_answer_document_patch.go",
	"emit_answer_document_v2.go",
	"emit_investigation_complete.go",
	"emit_analysis.go",
	"answer_block_normalize.go",
	"answer_document_mutation_runtime.go",
}

// emitValidatorRoster is keyed "<pkg>/<func>" (methods as "<pkg>/<Recv>.<Method>").
var emitValidatorRoster = map[string]emitValidatorRow{
	// ── converted to the list discipline (V2-4) ──
	"types/collectPatchStructureViolations":                      {class: emitValidatorList},
	"types/collectAnswerDocumentPatchBaseIdentityViolations":     {class: emitValidatorList},
	"tool/validateAnswerDocumentPatchFieldEditsAgainstSchema":    {class: emitValidatorList},
	"tool/validateAnswerDocumentPatchReceiptEditsAgainstSchema":  {class: emitValidatorList},
	"tool/localDiagramLeaseWholeBlockMutationViolations":         {class: emitValidatorList},
	"tool/splitCompanionDispositionViolations":                   {class: emitValidatorList},
	"tool/convertEmitBlocksToTyped":                              {class: emitValidatorList},
	"tool/collectMergedV2DocBlockViolations":                     {class: emitValidatorList},
	"tool/collectRuntimeWorkRelationReceiptViolations":           {class: emitValidatorList},
	"tool/collectConceptualTerminalResolutionReceiptViolations":  {class: emitValidatorList},
	"tool/collectAggregateRequestedDecoratorAlignmentViolations": {class: emitValidatorList},
	// ── serial gate + accumulate walker (EMITBURN-1 §29.173) ──
	"types/NormalizeAnswerAggregateFacts": {class: emitValidatorSerialWithCollector,
		collector: "CollectAnswerAggregateFactsViolations",
		parityPin: "TestEmitInvestigationComplete_EMITBURN1CollectFirstElementMatchesSerialGate"},
	"tool/normalizeCompletionAggregateFacts": {class: emitValidatorSerialWithCollector,
		collector: "completionAggregateFactsCollectViolations",
		parityPin: "TestNormalizeCompletionAggregateFacts_CollectFirstElementMatchesSerialGate"},
	// ── exceptions (first hit is the right shape; reason recorded) ──
	"types/ApplyAnswerDocumentV2Patch": {class: emitValidatorException,
		reason: "post-validation internal invariants only: every model-facing violation was collected by collectPatchStructureViolations before these loops run"},
	"tool/NormalizeEmitAnswerBlock": {class: emitValidatorException,
		reason: "per-block gating normalizer (discriminator repair re-shapes later arms); the outer collectors (convertEmitBlocksToTyped / full-emit blocks loop) list one entry per block"},
	"tool/validateEmitAnswerStructuredTableRows": {class: emitValidatorException,
		reason: "every arm teaches the same table-wide canonical repair (one cells[] value per column for every row); the first failing row already prescribes the whole-table fix"},
	"tool/requireModelOwnedAnswerBlockWirePreserved": {class: emitValidatorException,
		reason: "system-side invariant guard (persist must not rewrite model blocks), not a model-input validator"},
	"tool/modelOwnedAnswerBlockWire": {class: emitValidatorException,
		reason: "json.Marshal failure of a typed block is an internal error, not a model-facing violation"},
	"tool/callChainResolvedTargetSymbolForEvidence": {class: emitValidatorException,
		reason: "lookup: the pointer result is the found symbol, not a violation"},
	"tool/runtimeTraceCausalProjectionDegradeLeadBlock": {class: emitValidatorException,
		reason: "lookup: the pointer result is the selected lead block, not a violation"},
	"tool/reconcileAnswerExclusionWithCurrentSourceBoundary": {class: emitValidatorException,
		reason: "the pointer result is the (possibly softened) policy carrier, not a violation"},
	"tool/pendingBlockingEmitEvidenceItemValidationRepair": {class: emitValidatorException,
		reason: "ledger lookup over prior tool results: the pointer result is a still-pending earlier repair, not a violation of the current payload"},
	"types/aggregateFactResultCount": {class: emitValidatorException,
		reason: "lookup: the pointer result is the first parseable count, not a violation"},
	"types/toolResultResultCount": {class: emitValidatorException,
		reason: "lookup: the pointer result is the first parseable count, not a violation"},
}

type emitValidatorFunc struct {
	key   string
	file  string
	decl  *ast.FuncDecl
	slots []int // violation result slots (error / pointer)
}

type emitValidatorCensus struct {
	funcs        map[string]*emitValidatorFunc
	allFuncs     map[string]*emitValidatorFunc // scanned tool funcs ∪ every internal/types FuncDecl (collector lookup)
	firstHit     map[string][]string           // key → positions of first-hit returns
	unrecognized []string
	offenders    []string
	typesReach   int
}

func emitValidatorFuncKey(pkg string, fn *ast.FuncDecl) string {
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		return pkg + "/" + emitValidatorTypeName(fn.Recv.List[0].Type) + "." + fn.Name.Name
	}
	return pkg + "/" + fn.Name.Name
}

func emitValidatorTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return emitValidatorTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.IndexExpr:
		return emitValidatorTypeName(t.X)
	case *ast.IndexListExpr:
		return emitValidatorTypeName(t.X)
	}
	return ""
}

// emitValidatorViolationSlots returns the result positions typed `error` or
// pointer; a slice result marks the list discipline.
func emitValidatorViolationSlots(results *ast.FieldList) (slots []int, hasSlice bool) {
	if results == nil {
		return nil, false
	}
	i := 0
	for _, r := range results.List {
		n := len(r.Names)
		if n == 0 {
			n = 1
		}
		for k := 0; k < n; k++ {
			switch t := r.Type.(type) {
			case *ast.Ident:
				if t.Name == "error" {
					slots = append(slots, i)
				}
			case *ast.StarExpr:
				slots = append(slots, i)
			case *ast.ArrayType:
				hasSlice = true
			}
			i++
		}
	}
	return slots, hasSlice
}

// emitValidatorReturnShape classifies one returned expression in a violation
// slot: "nil" (no hit), "value" (a violation is yielded), or "" (unrecognized
// → fail loud).
func emitValidatorReturnShape(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		if v.Name == "nil" {
			return "nil"
		}
		return "value"
	case *ast.CallExpr, *ast.UnaryExpr, *ast.SelectorExpr, *ast.ParenExpr, *ast.IndexExpr, *ast.TypeAssertExpr, *ast.CompositeLit, *ast.StarExpr:
		return "value"
	}
	return ""
}

func (c *emitValidatorCensus) scanFunc(fset *token.FileSet, f *emitValidatorFunc) {
	if f.decl.Body == nil || len(f.slots) == 0 {
		return
	}
	var walkLoop func(body *ast.BlockStmt, loopPos token.Pos)
	walkLoop = func(body *ast.BlockStmt, loopPos token.Pos) {
		ast.Inspect(body, func(n ast.Node) bool {
			if _, isLit := n.(*ast.FuncLit); isLit {
				return false
			}
			ret, ok := n.(*ast.ReturnStmt)
			if !ok {
				return true
			}
			for _, slot := range f.slots {
				if slot >= len(ret.Results) {
					// bare return over named results or a call spread: the
					// classifier cannot see the value → fail loud.
					if len(ret.Results) == 0 || len(ret.Results) == 1 {
						if len(ret.Results) == 1 {
							if _, isCall := ret.Results[0].(*ast.CallExpr); isCall && len(f.slots) > 0 && slot > 0 {
								c.unrecognized = append(c.unrecognized, fmt.Sprintf("%s %s: loop return spreads a call result over the violation slot", fset.Position(ret.Pos()), f.key))
								break
							}
						}
						c.unrecognized = append(c.unrecognized, fmt.Sprintf("%s %s: bare/short loop return hides the violation slot value", fset.Position(ret.Pos()), f.key))
					}
					break
				}
				switch emitValidatorReturnShape(ret.Results[slot]) {
				case "nil":
				case "value":
					c.firstHit[f.key] = append(c.firstHit[f.key], fset.Position(ret.Pos()).String())
				default:
					c.unrecognized = append(c.unrecognized, fmt.Sprintf("%s %s: unrecognized return shape %T in violation slot %d", fset.Position(ret.Pos()), f.key, ret.Results[slot], slot))
				}
			}
			return true
		})
	}
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.RangeStmt:
			walkLoop(v.Body, v.Pos())
		case *ast.ForStmt:
			walkLoop(v.Body, v.Pos())
		}
		return true
	})
}

func emitValidatorCollectFuncs(t *testing.T, fset *token.FileSet, pkg string, paths []string) map[string]*emitValidatorFunc {
	t.Helper()
	out := map[string]*emitValidatorFunc{}
	for _, path := range paths {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			slots, _ := emitValidatorViolationSlots(fn.Type.Results)
			out[emitValidatorFuncKey(pkg, fn)] = &emitValidatorFunc{key: emitValidatorFuncKey(pkg, fn), file: path, decl: fn, slots: slots}
		}
	}
	return out
}

// emitValidatorTypesReach binds internal/types by DATA FLOW: the
// `types.<Name>` callees of the scanned tool files seed the set; the call
// closure inside internal/types (bare callees and method selectors that name
// a types FuncDecl) is followed to a bounded depth.
func emitValidatorTypesReach(toolFuncs map[string]*emitValidatorFunc, typesFuncs map[string]*emitValidatorFunc) map[string]*emitValidatorFunc {
	byName := map[string][]*emitValidatorFunc{}
	for _, f := range typesFuncs {
		byName[f.decl.Name.Name] = append(byName[f.decl.Name.Name], f)
	}
	reached := map[string]*emitValidatorFunc{}
	var frontier []*emitValidatorFunc
	for _, f := range toolFuncs {
		if f.decl.Body == nil {
			continue
		}
		ast.Inspect(f.decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// `types.Name(…)` binds the package-level function; any other
			// selector call `x.Method(…)` binds every types METHOD of that
			// name (the mutation dispatch `mutation.Apply(prev)` reaches
			// ApplyAnswerDocumentV2Patch this way — no direct call exists by
			// the base-constructor census).
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "types" {
				for _, target := range byName[sel.Sel.Name] {
					if target.decl.Recv == nil {
						if _, seen := reached[target.key]; !seen {
							reached[target.key] = target
							frontier = append(frontier, target)
						}
					}
				}
				return true
			}
			for _, target := range byName[sel.Sel.Name] {
				if target.decl.Recv != nil {
					if _, seen := reached[target.key]; !seen {
						reached[target.key] = target
						frontier = append(frontier, target)
					}
				}
			}
			return true
		})
	}
	for depth := 0; depth < 3 && len(frontier) > 0; depth++ {
		var next []*emitValidatorFunc
		for _, f := range frontier {
			if f.decl.Body == nil {
				continue
			}
			ast.Inspect(f.decl.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := ""
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					name = fun.Name
				case *ast.SelectorExpr:
					name = fun.Sel.Name
				}
				for _, target := range byName[name] {
					if _, seen := reached[target.key]; !seen {
						reached[target.key] = target
						next = append(next, target)
					}
				}
				return true
			})
		}
		frontier = next
	}
	return reached
}

func emitValidatorPackageFiles(t *testing.T, dir string, only []string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if only != nil {
			keep := false
			for _, want := range only {
				if want == name {
					keep = true
				}
			}
			if !keep {
				continue
			}
		}
		out = append(out, filepath.Join(dir, name))
	}
	sort.Strings(out)
	return out
}

func emitValidatorTestFuncNames(t *testing.T, dirs ...string) map[string]*ast.FuncDecl {
	t.Helper()
	fset := token.NewFileSet()
	out := map[string]*ast.FuncDecl{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", entry.Name(), err)
			}
			for _, d := range file.Decls {
				if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil {
					out[fn.Name.Name] = fn
				}
			}
		}
	}
	return out
}

func emitValidatorBodyMentions(fn *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return !found
	})
	return found
}

func runEmitValidatorCensus(t *testing.T, toolFiles, typesFiles []string, roster map[string]emitValidatorRow) *emitValidatorCensus {
	t.Helper()
	fset := token.NewFileSet()
	toolFuncs := emitValidatorCollectFuncs(t, fset, "tool", toolFiles)
	typesFuncs := emitValidatorCollectFuncs(t, fset, "types", typesFiles)
	reached := emitValidatorTypesReach(toolFuncs, typesFuncs)
	c := &emitValidatorCensus{funcs: map[string]*emitValidatorFunc{}, allFuncs: map[string]*emitValidatorFunc{}, firstHit: map[string][]string{}, typesReach: len(reached)}
	for k, f := range toolFuncs {
		c.funcs[k] = f
		c.allFuncs[k] = f
	}
	for k, f := range typesFuncs {
		c.allFuncs[k] = f
	}
	for k, f := range reached {
		c.funcs[k] = f
	}
	keys := make([]string, 0, len(c.funcs))
	for k := range c.funcs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		c.scanFunc(fset, c.funcs[k])
	}
	// ① every first-hit validator is rostered as serial_with_collector or exception.
	for _, k := range keys {
		hits := c.firstHit[k]
		if len(hits) == 0 {
			continue
		}
		row, ok := roster[k]
		if !ok {
			c.offenders = append(c.offenders, fmt.Sprintf("%s returns the FIRST violation of a loop (%s) and is not rostered: convert it to a []violation collector (list) or register serial_with_collector / exception with a reason (V2-4 §40.51)", k, strings.Join(hits, ", ")))
			continue
		}
		if row.class == emitValidatorList {
			c.offenders = append(c.offenders, fmt.Sprintf("%s is rostered as list but a loop returns its first violation again (%s)", k, strings.Join(hits, ", ")))
		}
	}
	// ② rostered rows track the tree.
	rowKeys := make([]string, 0, len(roster))
	for k := range roster {
		rowKeys = append(rowKeys, k)
	}
	sort.Strings(rowKeys)
	for _, k := range rowKeys {
		row := roster[k]
		f, ok := c.funcs[k]
		if !ok {
			c.offenders = append(c.offenders, fmt.Sprintf("roster row %s names no function in the scanned set (stale row)", k))
			continue
		}
		switch row.class {
		case emitValidatorList:
			if _, hasSlice := emitValidatorViolationSlots(f.decl.Type.Results); !hasSlice {
				c.offenders = append(c.offenders, fmt.Sprintf("%s is rostered as list but returns no slice", k))
			}
		case emitValidatorSerialWithCollector:
			if len(c.firstHit[k]) == 0 {
				c.offenders = append(c.offenders, fmt.Sprintf("%s is rostered as serial_with_collector but no loop returns a first violation (stale row: register it as list)", k))
			}
			pkg := strings.SplitN(k, "/", 2)[0]
			// collectors may live outside the reach closure; look them up by name in the package.
			collector, ok := c.allFuncs[pkg+"/"+row.collector]
			if !ok {
				c.offenders = append(c.offenders, fmt.Sprintf("%s: collector %s not found in package %s", k, row.collector, pkg))
			} else if _, hasSlice := emitValidatorViolationSlots(collector.decl.Type.Results); !hasSlice {
				c.offenders = append(c.offenders, fmt.Sprintf("%s: collector %s must return a slice of violations", k, row.collector))
			}
			if row.parityPin == "" {
				c.offenders = append(c.offenders, fmt.Sprintf("%s: serial_with_collector requires a parity pin (collector[0] == serial error)", k))
			}
		case emitValidatorException:
			if len(c.firstHit[k]) == 0 {
				c.offenders = append(c.offenders, fmt.Sprintf("%s is rostered as exception but no loop returns a first violation (stale row)", k))
			}
			if strings.TrimSpace(row.reason) == "" {
				c.offenders = append(c.offenders, fmt.Sprintf("%s: exception rows must state a reason", k))
			}
		default:
			c.offenders = append(c.offenders, fmt.Sprintf("%s: unknown roster class %q", k, row.class))
		}
	}
	sort.Strings(c.offenders)
	return c
}

func TestEmitValidatorsReturnEveryViolationByConstruction(t *testing.T) {
	toolFiles := emitValidatorPackageFiles(t, ".", emitValidatorScannedFiles)
	if len(toolFiles) != len(emitValidatorScannedFiles) {
		t.Fatalf("scanned executor files drifted: want %v, found %v", emitValidatorScannedFiles, toolFiles)
	}
	typesFiles := emitValidatorPackageFiles(t, filepath.Join("..", "types"), nil)
	if len(typesFiles) < 50 {
		t.Fatalf("internal/types scan found only %d files — the census scan is broken", len(typesFiles))
	}
	c := runEmitValidatorCensus(t, toolFiles, typesFiles, emitValidatorRoster)
	for _, u := range c.unrecognized {
		t.Errorf("fail-loud: %s", u)
	}
	for _, o := range c.offenders {
		t.Errorf("list discipline: %s", o)
	}
	// Vacuity guards: the recognizer must see the live producers.
	if c.typesReach < 20 {
		t.Fatalf("data-flow reach into internal/types found only %d functions — the hop is not bound to the executors", c.typesReach)
	}
	for _, want := range []string{"types/ApplyAnswerDocumentV2Patch", "types/NormalizeAnswerAggregateFacts", "types/collectPatchStructureViolations"} {
		if _, ok := c.funcs[want]; !ok {
			t.Errorf("expected %s in the data-flow reach (mutation dispatch / aggregate gate / patch collector)", want)
		}
	}
	if len(c.firstHit) < 8 {
		t.Fatalf("recognizer found only %d first-hit validators — expected the rostered exceptions and serial gates", len(c.firstHit))
	}
	// Parity pins exist and reference both the serial gate and its collector.
	tests := emitValidatorTestFuncNames(t, ".", filepath.Join("..", "types"))
	for k, row := range emitValidatorRoster {
		if row.class != emitValidatorSerialWithCollector {
			continue
		}
		pin, ok := tests[row.parityPin]
		if !ok {
			t.Errorf("%s: parity pin %s does not exist", k, row.parityPin)
			continue
		}
		serial := k[strings.LastIndex(k, "/")+1:]
		if !emitValidatorBodyMentions(pin, serial) || !emitValidatorBodyMentions(pin, row.collector) {
			t.Errorf("%s: parity pin %s must exercise both %s and %s", k, row.parityPin, serial, row.collector)
		}
	}
}

// Self-red: each evasion shape is flagged by the same walker; the canonical
// list shape passes.
func TestEmitValidatorCensusFlagsEachEvasionShape(t *testing.T) {
	write := func(t *testing.T, dir, name, src string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	typesSrc := "package types\n\ntype Doc struct{ Blocks []string }\n\nfunc CollectDocViolations(d *Doc) []string {\n\tvar out []string\n\tfor _, b := range d.Blocks {\n\t\tif b == \"\" {\n\t\t\tout = append(out, \"empty\")\n\t\t}\n\t}\n\treturn out\n}\n\nfunc ValidateDoc(d *Doc) error {\n\tfor _, b := range d.Blocks {\n\t\tif b == \"\" {\n\t\t\treturn fmt.Errorf(\"empty\")\n\t\t}\n\t}\n\treturn nil\n}\n"
	cases := []struct {
		name    string
		toolSrc string
		roster  map[string]emitValidatorRow
		want    string // substring of an offender / unrecognized line; "" = must be clean
	}{
		{
			name:    "unregistered first-hit loop is red",
			toolSrc: "package tool\n\nfunc checkBlocks(in []string) error {\n\tfor _, b := range in {\n\t\tif b == \"\" {\n\t\t\treturn fmt.Errorf(\"empty\")\n\t\t}\n\t}\n\treturn nil\n}\n",
			roster:  map[string]emitValidatorRow{},
			want:    "tool/checkBlocks returns the FIRST violation",
		},
		{
			name:    "pointer-violation first hit is red",
			toolSrc: "package tool\n\ntype v struct{}\n\nfunc checkBlocks(in []string) *v {\n\tfor i := 0; i < len(in); i++ {\n\t\tif in[i] == \"\" {\n\t\t\treturn &v{}\n\t\t}\n\t}\n\treturn nil\n}\n",
			roster:  map[string]emitValidatorRow{},
			want:    "tool/checkBlocks returns the FIRST violation",
		},
		{
			name:    "list row that turned first-hit again is red",
			toolSrc: "package tool\n\nfunc checkBlocks(in []string) ([]string, error) {\n\tfor _, b := range in {\n\t\tif b == \"\" {\n\t\t\treturn nil, fmt.Errorf(\"empty\")\n\t\t}\n\t}\n\treturn nil, nil\n}\n",
			roster:  map[string]emitValidatorRow{"tool/checkBlocks": {class: emitValidatorList}},
			want:    "rostered as list but a loop returns its first violation",
		},
		{
			name:    "unrecognized return shape fails loud",
			toolSrc: "package tool\n\nfunc checkBlocks(in []string, a, b error) error {\n\tfor _, s := range in {\n\t\tif s == \"\" {\n\t\t\treturn a + b\n\t\t}\n\t}\n\treturn nil\n}\n",
			roster:  map[string]emitValidatorRow{"tool/checkBlocks": {class: emitValidatorException, reason: "x"}},
			want:    "unrecognized return shape",
		},
		{
			name:    "bare return over named results fails loud",
			toolSrc: "package tool\n\nfunc checkBlocks(in []string) (err error) {\n\tfor _, s := range in {\n\t\tif s == \"\" {\n\t\t\terr = fmt.Errorf(\"empty\")\n\t\t\treturn\n\t\t}\n\t}\n\treturn nil\n}\n",
			roster:  map[string]emitValidatorRow{},
			want:    "bare/short loop return hides the violation slot value",
		},
		{
			name:    "exception without reason is red",
			toolSrc: "package tool\n\nfunc checkBlocks(in []string) error {\n\tfor _, b := range in {\n\t\tif b == \"\" {\n\t\t\treturn fmt.Errorf(\"empty\")\n\t\t}\n\t}\n\treturn nil\n}\n",
			roster:  map[string]emitValidatorRow{"tool/checkBlocks": {class: emitValidatorException}},
			want:    "exception rows must state a reason",
		},
		{
			name:    "serial row without collector is red",
			toolSrc: "package tool\n\nfunc checkBlocks(in []string) error {\n\tfor _, b := range in {\n\t\tif b == \"\" {\n\t\t\treturn fmt.Errorf(\"empty\")\n\t\t}\n\t}\n\treturn nil\n}\n",
			roster:  map[string]emitValidatorRow{"tool/checkBlocks": {class: emitValidatorSerialWithCollector, collector: "collectBlocks", parityPin: "TestX"}},
			want:    "collector collectBlocks not found",
		},
		{
			name:    "stale row is red",
			toolSrc: "package tool\n\nfunc checkBlocks(in []string) []string {\n\tvar out []string\n\tfor _, b := range in {\n\t\tif b == \"\" {\n\t\t\tout = append(out, \"empty\")\n\t\t}\n\t}\n\treturn out\n}\n",
			roster:  map[string]emitValidatorRow{"tool/checkBlocks": {class: emitValidatorList}, "tool/gone": {class: emitValidatorException, reason: "x"}},
			want:    "roster row tool/gone names no function",
		},
		{
			name:    "types first-hit reached by data flow is red",
			toolSrc: "package tool\n\nimport \"types\"\n\nfunc execute(d *types.Doc) error {\n\treturn types.ValidateDoc(d)\n}\n",
			roster:  map[string]emitValidatorRow{},
			want:    "types/ValidateDoc returns the FIRST violation",
		},
		{
			name:    "types serial gate with collector and pin passes",
			toolSrc: "package tool\n\nimport \"types\"\n\nfunc execute(d *types.Doc) error {\n\treturn types.ValidateDoc(d)\n}\n",
			roster:  map[string]emitValidatorRow{"types/ValidateDoc": {class: emitValidatorSerialWithCollector, collector: "CollectDocViolations", parityPin: "TestParity"}},
			want:    "",
		},
		{
			name:    "canonical list shape passes",
			toolSrc: "package tool\n\nfunc checkBlocks(in []string) []string {\n\tvar out []string\n\tfor _, b := range in {\n\t\tif b == \"\" {\n\t\t\tout = append(out, \"empty\")\n\t\t}\n\t}\n\treturn out\n}\n\nfunc lookup(in []string) (string, bool) {\n\tfor _, b := range in {\n\t\tif b != \"\" {\n\t\t\treturn b, true\n\t\t}\n\t}\n\treturn \"\", false\n}\n",
			roster:  map[string]emitValidatorRow{"tool/checkBlocks": {class: emitValidatorList}},
			want:    "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			toolPath := write(t, dir, "exec.go", tc.toolSrc)
			typesPath := write(t, dir, "types.go", typesSrc)
			c := runEmitValidatorCensus(t, []string{toolPath}, []string{typesPath}, tc.roster)
			all := append(append([]string(nil), c.unrecognized...), c.offenders...)
			if tc.want == "" {
				if len(all) != 0 {
					t.Fatalf("canonical shape must be clean, got %v", all)
				}
				return
			}
			for _, line := range all {
				if strings.Contains(line, tc.want) {
					return
				}
			}
			t.Fatalf("expected a finding containing %q, got %v", tc.want, all)
		})
	}
}
