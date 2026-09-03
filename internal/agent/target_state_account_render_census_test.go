package agent

import (
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	gotypes "go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// target_state_account_render_census_test.go — V3-1 tripwire
// (colleague_merge_audit §40.20 ②, totality fold-in §40.49 T0/T1): the
// target-state account (TraceTargetStateScopeAuthority /
// TraceCausalProjectionTargetStateAccount) is ONE typed authority; the three
// prompt prose faces used to hand-format it with three different
// "uninterruptible wait" calibers (五表手抄病根). The census below is keyed on
// the account TYPES' selectors, resolved with go/types (a same-named field on
// another struct never counts, an account selector reached through any alias
// always counts), over every non-test function of the prompt-rendering
// packages internal/agent, internal/context, internal/orchestrator and
// internal/types:
//
//	(a) a function that references ANY account figure — lane (.RunningMS
//	    .RunnableMS .SleepMS .DStateMS .IOWaitMS .SleepIOWaitMS), total
//	    (.TotalMS .UnaccountedMS .WindowMS) or fold (.UninterruptibleWaitMS()
//	    .SchedulerMarkedWaitMS()) — and lets that figure, or a local it was
//	    assigned to, reach a string sink (fmt.*, strconv.*, Builder/Buffer
//	    Write*) is a HAND RENDERER and is red unless it is in the
//	    function-scoped allowlist below (typed key=value rows / the
//	    types-level formatter / fingerprint);
//	(b) arithmetic over an account figure outside internal/types' fold and
//	    identity functions is red in EVERY shape: `x + y`, `x += y`, a local
//	    accumulator (`d := a.DStateMS; d += a.IOWaitMS`), and a helper call
//	    that receives a figure (`max(a.DStateMS, a.IOWaitMS)`) — intra-
//	    function taint through assignments, not a single BinaryExpr;
//	(c) a function that references an account figure and builds ANY string
//	    without calling types.FormatTargetStateAccount is red (the ruling's
//	    literal arm), and a fold-word literal (不可中断 / uninterruptible) in
//	    such a function is red unless the function is the wording owner;
//	(d) the three prose renderers must call the formatter and carry no fold
//	    word; the typed-row functions may render lanes/totals only (never a
//	    fold selector), with no arithmetic and no fold word;
//	(e) every lane-named selector whose receiver type could not be resolved
//	    (a faked non-types import) is listed in an explicit allowlist, so
//	    the census can never go green by failing to see;
//	(f) stale allowlist entries are red; the self-red test injects each
//	    evasion shape into the real sources and runs the same gate.
type targetStateFnCensus struct {
	selectors map[string]bool
	// unresolved: lane-named selector → the receiver's DECLARED type text
	// (parameter / range-over-parameter), when go/types could not type it.
	unresolved    map[string]string
	handRendered  map[string]bool
	arithmetic    bool
	helperCalls   map[string]bool
	stringBuild   bool
	formatterCall bool
	foldWord      bool
}

func newTargetStateFnCensus() *targetStateFnCensus {
	return &targetStateFnCensus{
		selectors:    map[string]bool{},
		unresolved:   map[string]string{},
		handRendered: map[string]bool{},
		helperCalls:  map[string]bool{},
	}
}

type targetStateRenderCensus struct {
	filesWalked int
	fns         map[string]*targetStateFnCensus
}

func newTargetStateRenderCensus() *targetStateRenderCensus {
	return &targetStateRenderCensus{fns: map[string]*targetStateFnCensus{}}
}

// The account types (internal/types) and their figure selectors — the closed
// set this census is keyed on.
var targetStateAccountTypeNames = map[string]bool{
	"TraceTargetStateScopeAuthority":          true,
	"TraceCausalProjectionTargetStateAccount": true,
}

var targetStateLaneSelectors = map[string]bool{
	"RunningMS": true, "RunnableMS": true, "SleepMS": true,
	"DStateMS": true, "IOWaitMS": true, "SleepIOWaitMS": true,
}
var targetStateTotalSelectors = map[string]bool{"TotalMS": true, "UnaccountedMS": true, "WindowMS": true}
var targetStateFoldSelectors = map[string]bool{"UninterruptibleWaitMS": true, "SchedulerMarkedWaitMS": true}

func targetStateAccountSelector(name string) bool {
	return targetStateLaneSelectors[name] || targetStateTotalSelectors[name] || targetStateFoldSelectors[name]
}

const targetStateTypesPkgSuffix = "/internal/types"

func targetStateDerefNamed(typ gotypes.Type) *gotypes.Named {
	if typ == nil {
		return nil
	}
	if ptr, ok := typ.Underlying().(*gotypes.Pointer); ok {
		typ = ptr.Elem()
	}
	named, _ := typ.(*gotypes.Named)
	return named
}

func targetStateIsAccountType(typ gotypes.Type) bool {
	named := targetStateDerefNamed(typ)
	if named == nil || named.Obj().Pkg() == nil {
		return false
	}
	return strings.HasSuffix(named.Obj().Pkg().Path(), targetStateTypesPkgSuffix) && targetStateAccountTypeNames[named.Obj().Name()]
}

func targetStateFuncKey(rel string, fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return rel + ":" + fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	if idx, ok := recv.(*ast.IndexExpr); ok {
		recv = idx.X
	}
	if ident, ok := recv.(*ast.Ident); ok {
		return rel + ":" + ident.Name + "." + fn.Name.Name
	}
	return rel + ":?." + fn.Name.Name
}

func targetStateArithmeticOp(op token.Token) bool {
	switch op {
	case token.ADD, token.SUB, token.MUL, token.QUO, token.REM,
		token.ADD_ASSIGN, token.SUB_ASSIGN, token.MUL_ASSIGN, token.QUO_ASSIGN, token.REM_ASSIGN:
		return true
	}
	return false
}

// targetStateCalleeObject resolves the callee of a call to its object (a
// package function, a method, or a builtin) when go/types could.
func targetStateCalleeObject(info *gotypes.Info, fun ast.Expr) gotypes.Object {
	switch f := fun.(type) {
	case *ast.ParenExpr:
		return targetStateCalleeObject(info, f.X)
	case *ast.Ident:
		return info.Uses[f]
	case *ast.SelectorExpr:
		return info.Uses[f.Sel]
	}
	return nil
}

func targetStateCalleeDisplay(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.ParenExpr:
		return targetStateCalleeDisplay(f.X)
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return targetStateCalleeDisplay(f.X) + "." + f.Sel.Name
	case *ast.CallExpr:
		return targetStateCalleeDisplay(f.Fun) + "()"
	}
	return "?"
}

// targetStateStringSink reports whether a call renders its arguments into a
// string: any fmt/strconv function, or a Write* method (strings.Builder,
// bytes.Buffer, io.Writer wrappers).
func targetStateStringSink(info *gotypes.Info, call *ast.CallExpr) bool {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		switch sel.Sel.Name {
		case "WriteString", "WriteRune", "WriteByte", "Write":
			return true
		}
	}
	obj := targetStateCalleeObject(info, call.Fun)
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	switch obj.Pkg().Path() {
	case "fmt", "strconv":
		return true
	}
	return false
}

func targetStateIsFormatterCall(info *gotypes.Info, call *ast.CallExpr) bool {
	obj := targetStateCalleeObject(info, call.Fun)
	if obj == nil || obj.Pkg() == nil || obj.Name() != "FormatTargetStateAccount" {
		return false
	}
	return strings.HasSuffix(obj.Pkg().Path(), targetStateTypesPkgSuffix)
}

// targetStateDeclaredTypes maps a function's parameter names (and range
// variables iterating a parameter) to their declared type text, so an
// unresolved receiver can be pinned by the type its declaration names.
func targetStateDeclaredTypes(fn *ast.FuncDecl) map[string]string {
	out := map[string]string{}
	fields := []*ast.Field{}
	if fn.Recv != nil {
		fields = append(fields, fn.Recv.List...)
	}
	if fn.Type.Params != nil {
		fields = append(fields, fn.Type.Params.List...)
	}
	for _, field := range fields {
		text := targetStateTypeText(field.Type)
		for _, name := range field.Names {
			out[name.Name] = text
		}
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		rng, ok := node.(*ast.RangeStmt)
		if !ok {
			return true
		}
		src, ok := rng.X.(*ast.Ident)
		if !ok {
			return true
		}
		text, ok := out[src.Name]
		if !ok {
			return true
		}
		if v, ok := rng.Value.(*ast.Ident); ok {
			out[v.Name] = "range " + text
		}
		return true
	})
	return out
}

func targetStateTypeText(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return targetStateTypeText(x.X) + "." + x.Sel.Name
	case *ast.StarExpr:
		return "*" + targetStateTypeText(x.X)
	case *ast.ArrayType:
		return "[]" + targetStateTypeText(x.Elt)
	case *ast.MapType:
		return "map[" + targetStateTypeText(x.Key) + "]" + targetStateTypeText(x.Value)
	case *ast.Ellipsis:
		return "..." + targetStateTypeText(x.Elt)
	}
	return "?"
}

// censusFunc classifies one top-level function (closures inside it count
// toward it).
func (c *targetStateRenderCensus) censusFunc(key string, fn *ast.FuncDecl, info *gotypes.Info) {
	rec := newTargetStateFnCensus()
	accountSel := map[*ast.SelectorExpr]bool{}
	declared := targetStateDeclaredTypes(fn)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		sel, ok := node.(*ast.SelectorExpr)
		if !ok || !targetStateAccountSelector(sel.Sel.Name) {
			return true
		}
		if selection, ok := info.Selections[sel]; ok {
			if targetStateIsAccountType(selection.Recv()) {
				accountSel[sel] = true
				rec.selectors[sel.Sel.Name] = true
			}
			return true
		}
		if tv, ok := info.Types[sel.X]; ok && tv.Type != nil && tv.Type != gotypes.Typ[gotypes.Invalid] {
			// A package-qualified identifier or a resolved non-account
			// receiver — not an account figure.
			return true
		}
		if obj := info.Uses[sel.Sel]; obj != nil {
			return true
		}
		origin := "?"
		if ident, ok := sel.X.(*ast.Ident); ok {
			if text, ok := declared[ident.Name]; ok {
				origin = text
			}
		}
		rec.unresolved[sel.Sel.Name] = origin
		return true
	})

	tainted := map[gotypes.Object]bool{}
	var isTainted func(e ast.Expr) bool
	isTainted = func(e ast.Expr) bool {
		switch x := e.(type) {
		case *ast.SelectorExpr:
			return accountSel[x]
		case *ast.Ident:
			if obj := info.ObjectOf(x); obj != nil {
				return tainted[obj]
			}
		case *ast.ParenExpr:
			return isTainted(x.X)
		case *ast.UnaryExpr:
			return isTainted(x.X)
		case *ast.BinaryExpr:
			return targetStateArithmeticOp(x.Op) && (isTainted(x.X) || isTainted(x.Y))
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && accountSel[sel] {
				return true // fold method call
			}
			if tv, ok := info.Types[x.Fun]; ok && tv.IsType() && len(x.Args) == 1 {
				return isTainted(x.Args[0]) // conversion passthrough
			}
		}
		return false
	}
	taint := func(e ast.Expr) bool {
		if id, ok := e.(*ast.Ident); ok {
			if obj := info.ObjectOf(id); obj != nil && !tainted[obj] {
				tainted[obj] = true
				return true
			}
		}
		return false
	}
	for changed := true; changed; {
		changed = false
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.AssignStmt:
				if len(n.Lhs) != len(n.Rhs) {
					return true
				}
				for i := range n.Lhs {
					if isTainted(n.Rhs[i]) && taint(n.Lhs[i]) {
						changed = true
					}
				}
			case *ast.ValueSpec:
				if len(n.Names) != len(n.Values) {
					return true
				}
				for i := range n.Names {
					if isTainted(n.Values[i]) && taint(n.Names[i]) {
						changed = true
					}
				}
			}
			return true
		})
	}
	renderedNames := func(e ast.Expr) {
		ast.Inspect(e, func(node ast.Node) bool {
			switch x := node.(type) {
			case *ast.SelectorExpr:
				if accountSel[x] {
					rec.handRendered[x.Sel.Name] = true
				}
			case *ast.Ident:
				if obj := info.ObjectOf(x); obj != nil && tainted[obj] {
					rec.handRendered["local:"+x.Name] = true
				}
			}
			return true
		})
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.BinaryExpr:
			if targetStateArithmeticOp(n.Op) && (isTainted(n.X) || isTainted(n.Y)) {
				rec.arithmetic = true
			}
			if tv, ok := info.Types[n]; ok && n.Op == token.ADD && tv.Type != nil {
				if basic, ok := tv.Type.Underlying().(*gotypes.Basic); ok && basic.Kind() == gotypes.String {
					rec.stringBuild = true
				}
			}
		case *ast.AssignStmt:
			if targetStateArithmeticOp(n.Tok) {
				for _, e := range append(append([]ast.Expr{}, n.Lhs...), n.Rhs...) {
					if isTainted(e) {
						rec.arithmetic = true
					}
				}
			}
		case *ast.IncDecStmt:
			if isTainted(n.X) {
				rec.arithmetic = true
			}
		case *ast.CallExpr:
			if tv, ok := info.Types[n.Fun]; ok && tv.IsType() {
				return true // conversion: handled by isTainted passthrough
			}
			if targetStateIsFormatterCall(info, n) {
				rec.formatterCall = true
				return true
			}
			if targetStateStringSink(info, n) {
				rec.stringBuild = true
				for _, arg := range n.Args {
					if isTainted(arg) {
						renderedNames(arg)
					}
				}
				return true
			}
			for _, arg := range n.Args {
				if isTainted(arg) {
					rec.helperCalls[targetStateCalleeDisplay(n.Fun)] = true
				}
			}
		case *ast.BasicLit:
			if n.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(n.Value)
			if err != nil {
				return true
			}
			if strings.Contains(value, "不可中断") || strings.Contains(strings.ToLower(value), "uninterruptible") {
				rec.foldWord = true
			}
		}
		return true
	})
	c.fns[key] = rec
}

// targetStateCensusImporter imports stdlib and the internal/types closure for
// real (source importer) and fakes every other package, so the four target
// packages type-check in seconds; selectors whose receiver comes from a faked
// package land in the unresolved bucket (arm e), never silently in neither.
type targetStateCensusImporter struct {
	real            gotypes.ImporterFrom
	realModulePaths map[string]bool
	fakes           map[string]*gotypes.Package
}

func (i *targetStateCensusImporter) Import(path string) (*gotypes.Package, error) {
	return i.ImportFrom(path, "", 0)
}

func (i *targetStateCensusImporter) ImportFrom(path, dir string, mode gotypes.ImportMode) (*gotypes.Package, error) {
	if path == "unsafe" {
		return gotypes.Unsafe, nil
	}
	firstSeg := path
	if slash := strings.Index(path, "/"); slash >= 0 {
		firstSeg = path[:slash]
	}
	if !strings.Contains(firstSeg, ".") || i.realModulePaths[path] {
		return i.real.ImportFrom(path, dir, mode)
	}
	if pkg, ok := i.fakes[path]; ok {
		return pkg, nil
	}
	name := path[strings.LastIndex(path, "/")+1:]
	pkg := gotypes.NewPackage(path, name)
	pkg.MarkComplete()
	i.fakes[path] = pkg
	return pkg, nil
}

func targetStateReadModulePath(t *testing.T, gomod string) string {
	t.Helper()
	data, err := os.ReadFile(gomod)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	t.Fatalf("no module line in %s", gomod)
	return ""
}

// targetStateCensusTarget is one censused package. strict packages must
// type-check cleanly (the real-import universe) so the account selectors
// there are exact; the others are checked against fakes (arm e catches what
// the fakes hide).
type targetStateCensusTarget struct {
	rel    string
	strict bool
}

var targetStateCensusTargets = []targetStateCensusTarget{
	{rel: "internal/types", strict: true},
	{rel: "internal/agent"},
	{rel: "internal/context"},
	{rel: "internal/orchestrator"},
}

// targetStateCensusRealPaths is internal/types' module-internal import
// closure (go list -deps), imported for real so the strict check stays exact.
var targetStateCensusRealPaths = []string{
	"internal/types", "internal/tracefence", "internal/canonpath", "internal/logging", "internal/mermaidcompat",
}

type targetStateCensusSession struct {
	t       *testing.T
	root    string
	modPath string
	fset    *token.FileSet
	imp     *targetStateCensusImporter
}

func newTargetStateCensusSession(t *testing.T) *targetStateCensusSession {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	modPath := targetStateReadModulePath(t, filepath.Join(root, "go.mod"))
	fset := token.NewFileSet()
	src, ok := importer.ForCompiler(fset, "source", nil).(gotypes.ImporterFrom)
	if !ok {
		t.Fatal("source importer must implement types.ImporterFrom")
	}
	imp := &targetStateCensusImporter{real: src, realModulePaths: map[string]bool{}, fakes: map[string]*gotypes.Package{}}
	for _, rel := range targetStateCensusRealPaths {
		imp.realModulePaths[modPath+"/"+rel] = true
	}
	return &targetStateCensusSession{t: t, root: root, modPath: modPath, fset: fset, imp: imp}
}

// censusPackage type-checks the non-test, build-selected files of one
// package (overrides substitute in-memory sources by file name — the
// self-red test's injection lane) and classifies every top-level function.
func (s *targetStateCensusSession) censusPackage(census *targetStateRenderCensus, target targetStateCensusTarget, overrides map[string]string) {
	s.t.Helper()
	dir := filepath.Join(s.root, target.rel)
	bpkg, err := build.Default.ImportDir(dir, 0)
	if err != nil {
		s.t.Fatalf("build.ImportDir(%s): %v", dir, err)
	}
	var files []*ast.File
	rels := map[*ast.File]string{}
	for _, name := range bpkg.GoFiles {
		var src interface{}
		if override, ok := overrides[name]; ok {
			src = override
		}
		f, err := parser.ParseFile(s.fset, filepath.Join(dir, name), src, 0)
		if err != nil {
			s.t.Fatalf("parse %s/%s: %v", dir, name, err)
		}
		files = append(files, f)
		rels[f] = target.rel + "/" + name
	}
	info := &gotypes.Info{
		Types:      map[ast.Expr]gotypes.TypeAndValue{},
		Defs:       map[*ast.Ident]gotypes.Object{},
		Uses:       map[*ast.Ident]gotypes.Object{},
		Selections: map[*ast.SelectorExpr]*gotypes.Selection{},
	}
	var typeErrs []error
	conf := gotypes.Config{
		Importer:    s.imp,
		FakeImportC: true,
		Error:       func(err error) { typeErrs = append(typeErrs, err) },
	}
	_, _ = conf.Check(s.modPath+"/"+target.rel, s.fset, files, info)
	if target.strict && len(typeErrs) > 0 {
		s.t.Fatalf("type-checking %s must be clean for the census to be exact; first error: %v", target.rel, typeErrs[0])
	}
	for _, f := range files {
		census.filesWalked++
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			census.censusFunc(targetStateFuncKey(rels[f], fn), fn, info)
		}
	}
}

func (s *targetStateCensusSession) walk(overrides map[string]map[string]string) *targetStateRenderCensus {
	s.t.Helper()
	census := newTargetStateRenderCensus()
	for _, target := range targetStateCensusTargets {
		s.censusPackage(census, target, overrides[target.rel])
	}
	if census.filesWalked < 400 {
		s.t.Fatalf("census walked only %d files — the repo root resolution (%q) is wrong", census.filesWalked, s.root)
	}
	return census
}

func targetStateUnresolvedNames(fn *targetStateFnCensus) map[string]bool {
	out := map[string]bool{}
	for name := range fn.unresolved {
		out[name] = true
	}
	return out
}

func sortedCensusKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// targetStateAllow is what one allowlisted function may do with account
// figures; everything not granted stays red inside the allowlist too.
type targetStateAllow struct {
	render     bool // figures may reach a string sink
	renderFold bool // the fold selectors may reach a string sink (formatter only)
	arithmetic bool // arithmetic / helper calls over figures
	foldWord   bool // owns the 不可中断 / uninterruptible wording
	why        string
}

var (
	targetStateAllowTypedRow    = targetStateAllow{render: true, why: "typed key=value row printing the disjoint engine lanes (partition_members=five_engine_lanes)"}
	targetStateAllowFormatter   = targetStateAllow{render: true, renderFold: true, foldWord: true, why: "the single prompt-face formatter"}
	targetStateAllowFoldSource  = targetStateAllow{arithmetic: true, why: "the fold definition"}
	targetStateAllowIdentity    = targetStateAllow{render: true, arithmetic: true, why: "partition identity / coverage classification, never a reader-facing sentence"}
	targetStateAllowFingerprint = targetStateAllow{render: true, why: "hashed fingerprint, never a reader-facing sentence"}
	targetStateAllowCompiler    = targetStateAllow{why: "typed compiler (election / id keys only, figures compared or copied, never rendered)"}
)

// targetStateCensusAllowlist — function-scoped; a new arm in one of these
// files still goes red.
var targetStateCensusAllowlist = map[string]targetStateAllow{
	// typed rows (internal/agent): five disjoint lanes, never a fold.
	"internal/agent/answer_document_trace_principal_value_authority.go:renderAnswerDocTracePrincipalValueAuthority":     targetStateAllowTypedRow,
	"internal/agent/answer_document_final_decision_boundary.go:renderTraceFinalBlockedReasonStateRelation":              targetStateAllowTypedRow,
	"internal/agent/answer_document_final_decision_boundary.go:renderTraceFinalTimeRoleAuthority":                       targetStateAllowTypedRow,
	"internal/agent/answer_document_trace_decision_handoff.go:renderAnswerDocTraceDecisionHandoffSetWithAggregateFacts": targetStateAllowTypedRow,
	// internal/types: the formatter, the fold sources, the identity checks.
	"internal/types/trace_target_state_scope_authority.go:FormatTargetStateAccount":                             targetStateAllowFormatter,
	"internal/types/trace_target_state_scope_authority.go:TraceTargetStateScopeAuthority.UninterruptibleWaitMS": targetStateAllowFoldSource,
	"internal/types/trace_target_state_scope_authority.go:TraceTargetStateScopeAuthority.SchedulerMarkedWaitMS": targetStateAllowFoldSource,
	"internal/types/trace_causal_projection.go:TraceCausalProjectionTargetStateAccount.UninterruptibleWaitMS":   targetStateAllowFoldSource,
	"internal/types/trace_target_state_scope_authority.go:BuildTraceTargetStateScopeAuthorities":                targetStateAllowIdentity,
	"internal/types/answer_relation_claim.go:answerRelationTargetStatePartitionClosed":                          targetStateAllowIdentity,
	"internal/types/answer_relation_claim.go:answerRelationTargetStateFingerprint":                              targetStateAllowFingerprint,
	"internal/types/trace_target_state_scope_authority.go:BuildTraceTargetStateScopeAuthoritiesFromLedger":      targetStateAllowCompiler,
	"internal/types/answer_relation_claim.go:CompileTraceAnswerRelationAuthorities":                             targetStateAllowCompiler,
}

// targetStateUnresolvedAllowlist — functions whose lane-NAMED selectors sit
// on a receiver the faked-import universe cannot type (arm e). The value is
// the receiver's DECLARED type text and must match what the census read from
// the declaration, so the entry only ever admits that one non-account type
// (a receiver re-declared as an account type resolves and is judged for
// real); stale entries are red.
var targetStateUnresolvedAllowlist = map[string]string{
	// The CR-3 reconciliation appendix row (tool.RuntimeTraceReconciliationRow)
	// — the answer-side system cross-check, not a prompt face.
	"internal/orchestrator/prose_typed_reconciliation.go:proseSelectsTargetStateReconciliation": "tool.RuntimeTraceReconciliationRow",
	"internal/orchestrator/prose_typed_reconciliation.go:renderTargetStateReconciliation":       "tool.RuntimeTraceReconciliationRow",
}

// targetStateProseRenderers — the three prompt prose faces of the account;
// each must speak through the types-level formatter.
var targetStateProseRenderers = []string{
	"internal/agent/answer_document_evaluator.go:renderAnswerDocTraceTargetStateScopeAuthority",
	"internal/agent/answer_document_final_decision_boundary.go:renderTraceFinalReaderDecisionCards",
	"internal/agent/answer_document_final_decision_boundary.go:renderAnswerDocBoundedRuntimeFinalReaderHandoff",
}

// targetStateCensusOffenders is THE gate: key → reasons. Both the live test
// and the self-red test judge through it.
func targetStateCensusOffenders(census *targetStateRenderCensus) map[string][]string {
	out := map[string][]string{}
	add := func(key, reason string) { out[key] = append(out[key], reason) }
	for key, fn := range census.fns {
		if len(fn.unresolved) > 0 {
			want, ok := targetStateUnresolvedAllowlist[key]
			for _, name := range sortedCensusKeys(targetStateUnresolvedNames(fn)) {
				if origin := fn.unresolved[name]; !ok || origin != want {
					add(key, "lane-named selector ."+name+" on an unresolved receiver declared as "+strconv.Quote(origin)+" — import the receiver's package for real or list the function in targetStateUnresolvedAllowlist with exactly that declared type")
				}
			}
		}
		if len(fn.selectors) == 0 {
			continue
		}
		allow, allowed := targetStateCensusAllowlist[key]
		if !allowed {
			if len(fn.handRendered) > 0 {
				add(key, "hand-renders account figures "+strings.Join(sortedCensusKeys(fn.handRendered), ",")+" into a string — prose must call types.FormatTargetStateAccount; a typed key=value row goes in targetStateCensusAllowlist")
			}
			if fn.arithmetic {
				add(key, "arithmetic over account figures outside internal/types — the single fold source is types.TraceUninterruptibleWaitMS / UninterruptibleWaitMS() / SchedulerMarkedWaitMS()")
			}
			if len(fn.helperCalls) > 0 {
				add(key, "passes account figures to helper "+strings.Join(sortedCensusKeys(fn.helperCalls), ",")+" — a fold-by-helper is still a fold outside internal/types")
			}
			if fn.foldWord {
				add(key, "carries a fold-word literal (不可中断/uninterruptible) — the wording is owned by types.FormatTargetStateAccount / tracefence Table ⑧")
			}
			if fn.stringBuild && !fn.formatterCall {
				add(key, "references account figures and builds a string without calling types.FormatTargetStateAccount")
			}
			continue
		}
		if !allow.render && len(fn.handRendered) > 0 {
			add(key, "allowlisted as "+allow.why+" but renders account figures "+strings.Join(sortedCensusKeys(fn.handRendered), ","))
		}
		if !allow.renderFold {
			for name := range fn.handRendered {
				if targetStateFoldSelectors[name] {
					add(key, "allowlisted as "+allow.why+" but renders the fold "+name+" — rows print disjoint lanes, never a fold")
				}
			}
		}
		if !allow.arithmetic && (fn.arithmetic || len(fn.helperCalls) > 0) {
			add(key, "allowlisted as "+allow.why+" but does arithmetic / helper calls over account figures — a fold lives only in internal/types")
		}
		if !allow.foldWord && fn.foldWord {
			add(key, "allowlisted as "+allow.why+" but carries a fold word (不可中断/uninterruptible)")
		}
	}
	for key := range targetStateCensusAllowlist {
		if fn, ok := census.fns[key]; !ok || len(fn.selectors) == 0 {
			add(key, "allowlisted function no longer references account figures — prune the allowlist instead of leaving a stale door")
		}
	}
	for key := range targetStateUnresolvedAllowlist {
		if fn, ok := census.fns[key]; !ok || len(fn.unresolved) == 0 {
			add(key, "unresolved-allowlist entry no longer has an unresolved lane-named selector — prune it")
		}
	}
	for _, key := range targetStateProseRenderers {
		fn, ok := census.fns[key]
		if !ok || !fn.formatterCall {
			add(key, "prose renderer must render the account via types.FormatTargetStateAccount")
		}
		if ok && fn.foldWord {
			add(key, "prose renderer carries its own fold-word literal — the wording is owned by types.FormatTargetStateAccount / tracefence Table ⑧")
		}
	}
	for _, reasons := range out {
		sort.Strings(reasons)
	}
	return out
}

func targetStateOffenderLines(offenders map[string][]string) []string {
	var lines []string
	for _, key := range sortedCensusKeys(func() map[string]bool {
		m := map[string]bool{}
		for k := range offenders {
			m[k] = true
		}
		return m
	}()) {
		for _, reason := range offenders[key] {
			lines = append(lines, key+": "+reason)
		}
	}
	return lines
}

func TestTargetStateAccountRenderersSpeakThroughOneFormatter(t *testing.T) {
	session := newTargetStateCensusSession(t)
	census := session.walk(nil)
	if lines := targetStateOffenderLines(targetStateCensusOffenders(census)); len(lines) > 0 {
		t.Fatalf("target-state account census (V3-1 §40.20 ②) offenders:\n  %s", strings.Join(lines, "\n  "))
	}
	// Coverage proof of the census itself: the three prose renderers and
	// every allowlisted function were SEEN referencing the account (a
	// walk that resolved nothing would otherwise be green).
	for _, key := range targetStateProseRenderers {
		if fn := census.fns[key]; fn == nil || !fn.formatterCall {
			t.Fatalf("census did not see %s call the formatter", key)
		}
	}
	if fn := census.fns["internal/agent/answer_document_final_decision_boundary.go:renderAnswerDocBoundedRuntimeFinalReaderHandoff"]; fn == nil || !fn.selectors["SchedulerMarkedWaitMS"] {
		t.Fatalf("census did not resolve the handoff's SchedulerMarkedWaitMS() zero test — go/types resolution of internal/agent is broken")
	}
	if fn := census.fns["internal/orchestrator/prose_wallclock_conservation_check.go:proseWallClockAccountsFromLedger"]; fn == nil || !fn.selectors["IOWaitMS"] {
		t.Fatalf("census did not resolve the orchestrator's authority reads — go/types resolution of internal/orchestrator is broken")
	}
}

// TestTargetStateAccountRenderCensusSelfRed — the census must flag every
// evasion shape when it is injected into the real sources (pin-as-judge: a
// tripwire that cannot go red proves nothing). Each shape is injected alone,
// the whole package is re-type-checked, and the SAME gate must name exactly
// the injected function with the expected reason.
func TestTargetStateAccountRenderCensusSelfRed(t *testing.T) {
	session := newTargetStateCensusSession(t)
	const boundary = "answer_document_final_decision_boundary.go"
	const handoff = "answer_document_trace_decision_handoff.go"
	boundaryRel := "internal/agent/" + boundary
	handoffRel := "internal/agent/" + handoff

	read := func(name string) string {
		src, err := os.ReadFile(filepath.Join(session.root, "internal/agent", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(src)
	}
	insertAfter := func(src, anchor, text string) string {
		idx := strings.Index(src, anchor)
		if idx < 0 {
			t.Fatalf("anchor %q no longer present", anchor)
		}
		idx += len(anchor)
		return src[:idx] + text + src[idx:]
	}
	insertBefore := func(src, anchor, text string) string {
		idx := strings.Index(src, anchor)
		if idx < 0 {
			t.Fatalf("anchor %q no longer present", anchor)
		}
		return src[:idx] + text + src[idx:]
	}
	const cardsAnchor = "func renderTraceFinalReaderDecisionCards("
	const timeRoleAnchor = "\t\tfmt.Fprintf(&b, \"  - selected_window_target_state subject="
	const blockedAnchor = "\t\t\tif account.IOWaitMS == 0 {\n"
	const handoffAnchor = "\t\t\tfmt.Fprintf(&b, \"- target_state_symptom: subject="

	bodyStart := func(src, anchor string) string {
		idx := strings.Index(src, anchor)
		if idx < 0 {
			t.Fatalf("anchor %q no longer present", anchor)
		}
		brace := strings.Index(src[idx:], "{\n")
		if brace < 0 {
			t.Fatalf("cannot locate the body after %q", anchor)
		}
		return src[:idx+brace+len("{\n")]
	}

	cases := []struct {
		name   string
		file   string
		mutate func(src string) string
		key    string
		reason string // substring of the expected reason
	}{
		{
			name: "fold-method sentence in a new renderer (T0: fold selector + non-lane fields + own fold word)",
			file: boundary,
			mutate: func(src string) string {
				return src + "\nfunc renderProbeFourthFace(a types.TraceTargetStateScopeAuthority) string {\n" +
					"\treturn fmt.Sprintf(\"- 目标线程 %s：不可中断等待 %.3f 毫秒，运行 %.3f 毫秒，合计 %.3f 毫秒\", a.Subject, a.UninterruptibleWaitMS(), a.RunningMS, a.TotalMS)\n}\n"
			},
			key:    boundaryRel + ":renderProbeFourthFace",
			reason: "hand-renders account figures RunningMS,TotalMS,UninterruptibleWaitMS",
		},
		{
			name: "fold-method sentence without a fold word (wording-free fourth face)",
			file: boundary,
			mutate: func(src string) string {
				return src + "\nfunc renderProbeQuietFace(a types.TraceTargetStateScopeAuthority, lang string) string {\n" +
					"\tvar b strings.Builder\n\tb.WriteString(strconv.FormatFloat(a.UninterruptibleWaitMS(), 'f', 3, 64))\n\tb.WriteString(\" / \")\n\tb.WriteString(fmt.Sprint(a.TotalMS))\n\treturn b.String()\n}\n"
			},
			key:    boundaryRel + ":renderProbeQuietFace",
			reason: "hand-renders account figures TotalMS,UninterruptibleWaitMS",
		},
		{
			name: "the retired natural fold (x + y) in a prose renderer",
			file: boundary,
			mutate: func(src string) string {
				head := bodyStart(src, cardsAnchor)
				return head + "\tdStateAndIOWaitMS := set.Projections[0].TargetStateAccount.DStateMS + set.Projections[0].TargetStateAccount.IOWaitMS\n\t_ = dStateAndIOWaitMS\n" + src[len(head):]
			},
			key:    boundaryRel + ":renderTraceFinalReaderDecisionCards",
			reason: "arithmetic over account figures outside internal/types",
		},
		{
			name: "+= fold inside an allowlisted typed-row function (T1)",
			file: boundary,
			mutate: func(src string) string {
				return insertBefore(src, timeRoleAnchor, "\t\tzzFold := account.DStateMS\n\t\tzzFold += account.IOWaitMS\n\t\t_ = zzFold\n")
			},
			key:    boundaryRel + ":renderTraceFinalTimeRoleAuthority",
			reason: "allowlisted as typed key=value row",
		},
		{
			name: "local-accumulator fold inside an allowlisted typed-row function (T1)",
			file: boundary,
			mutate: func(src string) string {
				return insertAfter(src, blockedAnchor, "\t\t\t\tzd := account.DStateMS\n\t\t\t\tzi := account.IOWaitMS\n\t\t\t\t_ = zd + zi\n")
			},
			key:    boundaryRel + ":renderTraceFinalBlockedReasonStateRelation",
			reason: "arithmetic / helper calls over account figures",
		},
		{
			name: "helper-call fold (builtin max) inside an allowlisted typed-row function (T1)",
			file: handoff,
			mutate: func(src string) string {
				return insertBefore(src, handoffAnchor, "\t\t\t_ = max(account.DStateMS, account.IOWaitMS)\n")
			},
			key:    handoffRel + ":renderAnswerDocTraceDecisionHandoffSetWithAggregateFacts",
			reason: "arithmetic / helper calls over account figures",
		},
		{
			name: "a fold selector printed by an allowlisted typed-row function",
			file: boundary,
			mutate: func(src string) string {
				return insertBefore(src, timeRoleAnchor, "\t\tfmt.Fprintf(&b, \"  - probe=%.3f\\n\", account.UninterruptibleWaitMS())\n")
			},
			key:    boundaryRel + ":renderTraceFinalTimeRoleAuthority",
			reason: "renders the fold UninterruptibleWaitMS",
		},
		{
			name: "a rendered local that was assigned from a lane (taint through assignment)",
			file: boundary,
			mutate: func(src string) string {
				return src + "\nfunc renderProbeViaLocal(a types.TraceTargetStateScopeAuthority) string {\n" +
					"\tblocked := a.DStateMS\n\treturn fmt.Sprintf(\"blocked %.3f\", blocked)\n}\n"
			},
			key:    boundaryRel + ":renderProbeViaLocal",
			reason: "hand-renders account figures local:blocked",
		},
		{
			name: "string built next to an account figure without the formatter (ruling arm)",
			file: boundary,
			mutate: func(src string) string {
				return src + "\nfunc renderProbeSideBySide(a types.TraceTargetStateScopeAuthority) string {\n" +
					"\tif a.SleepIOWaitMS > 0 {\n\t\treturn fmt.Sprintf(\"- sleep-side IO marker present for %s\", a.Subject)\n\t}\n\treturn \"\"\n}\n"
			},
			key:    boundaryRel + ":renderProbeSideBySide",
			reason: "builds a string without calling types.FormatTargetStateAccount",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			census := session.walk(map[string]map[string]string{
				"internal/agent": {tc.file: tc.mutate(read(tc.file))},
			})
			offenders := targetStateCensusOffenders(census)
			reasons, ok := offenders[tc.key]
			if !ok {
				t.Fatalf("census failed to flag %s; offenders: %v", tc.key, targetStateOffenderLines(offenders))
			}
			found := false
			for _, reason := range reasons {
				if strings.Contains(reason, tc.reason) {
					found = true
				}
			}
			if !found {
				t.Fatalf("census flagged %s for the wrong reason: %v (want %q)", tc.key, reasons, tc.reason)
			}
			for key := range offenders {
				if key != tc.key {
					t.Fatalf("injection into %s also flagged %s: %v — the shape must be attributed to its function only", tc.key, key, offenders[key])
				}
			}
		})
	}
}
