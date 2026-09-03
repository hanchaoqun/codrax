package tool

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// behavior_contract_id_resolution_census_test.go — V5-4 tripwire
// (colleague_merge_audit §40.24 item 3, V5-3 §40.23 item 3, §40.46 C4): the
// behavior-contract id space is resolved through ONE tombstone-aware,
// ledger-aware projection. The census is bound to DATA FLOW, not to the
// spelling of one call site: a value derived from the pre-rebase analyzer
// snapshot (`Request.BehaviorContracts`) is tainted through aliases
// (`x := ir.Request.BehaviorContracts`), range clauses, and call arguments
// into the parameters of censused functions (fixpoint over every function
// of tool/agent/writeflow/orchestrator), and no tainted value may reach a
// sink.
//
//	(a) no non-test file references the identifier WriteBehaviorContractIDs
//	    (the raw id set is unexported in types; this guards a re-export);
//	(b) no Required/HardRequired/PlacementRequiredWriteBehaviorContractIDs
//	    call receives a tainted argument (snapshot, alias, or tainted
//	    parameter);
//	(c) every refs gate in tool (a function with a bare string result whose
//	    body reads .ContractRefs / .PlacementRefs) that touches the id-source
//	    side — directly, through a tainted parameter, or through a
//	    same-package helper that does — resolves ids through an accepted
//	    authority (directly or via one same-package helper) and never
//	    consults WriteAnalysisIR() / the snapshot / a tainted value;
//	(d) SupersededBehaviorContracts / SupersededBehaviorContractIDs /
//	    BehaviorContractGeneration are written only by
//	    attachWriteBehaviorContracts — as assignment targets AND as composite
//	    literal keys;
//	(e) the pure projection ProjectWriteBehaviorContractGeneration is called
//	    outside package types only by the generation-0 workflow seed in
//	    writeflow; tool/agent/orchestrator reach the projection through
//	    Mutable.ProjectBehaviorContractGeneration, which supplies the run's
//	    tombstone ledger from state so no consumer can forget it;
//	(f) in the rendering packages (agent, writeflow — which never produce an
//	    IR) a tainted value is never iterated or indexed: the snapshot is only
//	    ever the input of the projection.
//
// A parse error is red; the file-count floor keeps a silently empty scan
// red; the drift floor counts resolution-checked gates, not skipped ones.

var behaviorContractIDAuthorities = map[string]bool{
	"resolveBehaviorContractIDs":              true,
	"ResolveChangePlanBehaviorContractIDs":    true,
	"ProjectBehaviorContractGeneration":       true,
	"ProjectWriteBehaviorContractGeneration":  true,
	"ChangePlanVerificationBehaviorContracts": true,
}

var behaviorContractRequiredIDFuncs = map[string]bool{
	"RequiredWriteBehaviorContractIDs":          true,
	"HardRequiredWriteBehaviorContractIDs":      true,
	"PlacementRequiredWriteBehaviorContractIDs": true,
}

var behaviorContractTombstoneWriterFields = map[string]bool{
	"SupersededBehaviorContracts":   true,
	"SupersededBehaviorContractIDs": true,
	"BehaviorContractGeneration":    true,
}

type behaviorContractCensusFile struct {
	name string
	src  string
	pkg  string // "tool" | "agent" | "writeflow" | "orchestrator"
}

func calleeName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

type behaviorContractCensusCall struct {
	callee string
	// pkg is the census package key the callee lives in: the caller's own
	// package for an unqualified call, or the imported package for a
	// `pkg.Callee(...)` call (resolved through the file's import block), so
	// call-argument taint crosses package boundaries.
	pkg  string
	args []ast.Expr
	node *ast.CallExpr
}

// behaviorContractCensusModulePath is the module root of every import the
// census resolves to a scanned package.
const behaviorContractCensusModulePath = "github.com/hanchaoqun/codrax/"

// behaviorContractCensusPackageKey maps a module-relative directory
// ("internal/tool", "internal/writeflow/convention", "cmd") to the census
// package key: the bare name for the four packages the fine-grained rules
// address, the relative path for every other scanned package (unique, never
// colliding with those names).
func behaviorContractCensusPackageKey(relDir string) string {
	switch relDir = filepath.ToSlash(relDir); relDir {
	case "internal/tool", "internal/agent", "internal/writeflow", "internal/orchestrator":
		return filepath.Base(relDir)
	}
	return relDir
}

type behaviorContractCensusFunc struct {
	name, file, pkg string
	decl            *ast.FuncDecl
	params          []string
	stringResult    bool
	readsRefs       bool
	idSource        bool
	authority       bool
	readsIR         bool
	writesField     bool
	calls           []behaviorContractCensusCall
	tainted         map[string]bool
}

// behaviorContractTaintSanitizers are calls whose RESULT never carries the
// snapshot's id space even when the snapshot is an argument: the projection
// (its result is the active generation) and len().
var behaviorContractTaintSanitizers = map[string]bool{
	"ProjectWriteBehaviorContractGeneration": true,
	"ProjectBehaviorContractGeneration":      true,
	"len":                                    true,
}

// exprTainted reports whether an expression derives from the analyzer
// snapshot: it selects `<x>.Request.BehaviorContracts` or references a
// tainted identifier (parameter or local), outside any sanitizer call.
func (fn *behaviorContractCensusFunc) exprTainted(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	tainted := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if tainted {
			return false
		}
		switch v := n.(type) {
		case *ast.CallExpr:
			if behaviorContractTaintSanitizers[calleeName(v)] {
				return false
			}
		case *ast.SelectorExpr:
			if v.Sel.Name == "BehaviorContracts" {
				if x, ok := v.X.(*ast.SelectorExpr); ok && x.Sel.Name == "Request" {
					tainted = true
					return false
				}
			}
			// Only the receiver side of a selector can carry taint.
			if fn.exprTainted(v.X) {
				tainted = true
			}
			return false
		case *ast.Ident:
			if fn.tainted[v.Name] {
				tainted = true
			}
		}
		return !tainted
	})
	return tainted
}

// propagateTaintLocally taints locals assigned from tainted expressions and
// range variables over tainted collections, to a fixpoint. Returns true when
// anything changed.
func (fn *behaviorContractCensusFunc) propagateTaintLocally() bool {
	changed := false
	for {
		round := false
		ast.Inspect(fn.decl.Body, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.AssignStmt:
				for i, lhs := range v.Lhs {
					id, ok := lhs.(*ast.Ident)
					if !ok || fn.tainted[id.Name] {
						continue
					}
					var rhs ast.Expr
					if len(v.Rhs) == len(v.Lhs) {
						rhs = v.Rhs[i]
					} else if len(v.Rhs) == 1 {
						rhs = v.Rhs[0]
					}
					if rhs != nil && fn.exprTainted(rhs) {
						fn.tainted[id.Name] = true
						round = true
					}
				}
			case *ast.ValueSpec:
				for i, name := range v.Names {
					if fn.tainted[name.Name] || i >= len(v.Values) {
						continue
					}
					if fn.exprTainted(v.Values[i]) {
						fn.tainted[name.Name] = true
						round = true
					}
				}
			case *ast.RangeStmt:
				if !fn.exprTainted(v.X) {
					return true
				}
				for _, e := range []ast.Expr{v.Key, v.Value} {
					if id, ok := e.(*ast.Ident); ok && id.Name != "_" && !fn.tainted[id.Name] {
						fn.tainted[id.Name] = true
						round = true
					}
				}
			}
			return true
		})
		if !round {
			return changed
		}
		changed = true
	}
}

func behaviorContractIDResolutionCensus(files []behaviorContractCensusFile) (offenders []string, resolutionChecked int, err error) {
	fset := token.NewFileSet()
	var funcs []*behaviorContractCensusFunc
	byPkgName := map[string]map[string]*behaviorContractCensusFunc{}
	for _, f := range files {
		file, perr := parser.ParseFile(fset, f.name, f.src, 0)
		if perr != nil {
			return nil, 0, perr
		}
		where := func(n ast.Node) string { return f.name + ":" + strconv.Itoa(fset.Position(n.Pos()).Line) }
		// Import block → census package key, so `pkg.Callee(...)` resolves to
		// the censused function it names in another scanned package.
		importKey := map[string]string{}
		for _, spec := range file.Imports {
			path, perr := strconv.Unquote(spec.Path.Value)
			if perr != nil || !strings.HasPrefix(path, behaviorContractCensusModulePath) {
				continue
			}
			rel := strings.TrimPrefix(path, behaviorContractCensusModulePath)
			local := filepath.Base(rel)
			if spec.Name != nil && spec.Name.Name != "" && spec.Name.Name != "_" && spec.Name.Name != "." {
				local = spec.Name.Name
			}
			importKey[local] = behaviorContractCensusPackageKey(rel)
		}
		// (a) over the whole file.
		ast.Inspect(file, func(n ast.Node) bool {
			if v, ok := n.(*ast.Ident); ok && v.Name == "WriteBehaviorContractIDs" {
				offenders = append(offenders, where(v)+" references WriteBehaviorContractIDs (raw id set; resolve through the projection)")
			}
			return true
		})
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			fn := &behaviorContractCensusFunc{name: fd.Name.Name, file: f.name, pkg: f.pkg, decl: fd, tainted: map[string]bool{}}
			if fd.Type.Params != nil {
				for _, field := range fd.Type.Params.List {
					if len(field.Names) == 0 {
						fn.params = append(fn.params, "")
						continue
					}
					for _, name := range field.Names {
						fn.params = append(fn.params, name.Name)
					}
				}
			}
			if fd.Type.Results != nil {
				for _, r := range fd.Type.Results.List {
					if id, ok := r.Type.(*ast.Ident); ok && id.Name == "string" {
						fn.stringResult = true
					}
				}
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.SelectorExpr:
					switch v.Sel.Name {
					case "ContractRefs", "PlacementRefs":
						fn.readsRefs = true
					case "BehaviorContracts":
						fn.idSource = true
						if x, ok := v.X.(*ast.SelectorExpr); ok && x.Sel.Name == "Request" {
							fn.readsIR = true
						}
					}
				case *ast.CallExpr:
					name := calleeName(v)
					calleePkg := f.pkg
					if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
						if qual, ok := sel.X.(*ast.Ident); ok {
							if key, ok := importKey[qual.Name]; ok {
								calleePkg = key
							}
						}
					}
					fn.calls = append(fn.calls, behaviorContractCensusCall{callee: name, pkg: calleePkg, args: v.Args, node: v})
					if strings.HasSuffix(name, "WriteBehaviorContractIDs") {
						fn.idSource = true
					}
					if behaviorContractIDAuthorities[name] {
						// An authority IS an id source: a helper that only
						// reads the projection is resolution-checked too.
						fn.authority = true
						fn.idSource = true
					}
					if name == "WriteAnalysisIR" {
						fn.readsIR = true
					}
				case *ast.AssignStmt:
					for _, lhs := range v.Lhs {
						if sel, ok := lhs.(*ast.SelectorExpr); ok && behaviorContractTombstoneWriterFields[sel.Sel.Name] {
							fn.writesField = true
						}
					}
				case *ast.KeyValueExpr:
					if key, ok := v.Key.(*ast.Ident); ok && behaviorContractTombstoneWriterFields[key.Name] {
						fn.writesField = true
					}
				}
				return true
			})
			funcs = append(funcs, fn)
			if byPkgName[f.pkg] == nil {
				byPkgName[f.pkg] = map[string]*behaviorContractCensusFunc{}
			}
			byPkgName[f.pkg][fn.name] = fn
		}
	}
	// Taint fixpoint: locals from the snapshot / tainted values, range
	// variables, and callee parameters fed a tainted argument — across every
	// censused function, in the same package or through a qualified call
	// into another scanned package.
	for changed := true; changed; {
		changed = false
		for _, fn := range funcs {
			if fn.propagateTaintLocally() {
				changed = true
			}
			for _, call := range fn.calls {
				callee, ok := byPkgName[call.pkg][call.callee]
				if !ok || callee == fn {
					continue
				}
				for i, arg := range call.args {
					if i >= len(callee.params) || callee.params[i] == "" || callee.tainted[callee.params[i]] {
						continue
					}
					if fn.exprTainted(arg) {
						callee.tainted[callee.params[i]] = true
						changed = true
					}
				}
			}
		}
	}
	// idSource propagates through same-package helpers (a gate whose ids come
	// from a helper is resolution-checked through that helper).
	for changed := true; changed; {
		changed = false
		for _, fn := range funcs {
			if fn.idSource {
				continue
			}
			for _, call := range fn.calls {
				if helper, ok := byPkgName[fn.pkg][call.callee]; ok && helper != fn && helper.idSource {
					fn.idSource = true
					changed = true
					break
				}
			}
		}
	}
	for _, fn := range funcs {
		where := func(n ast.Node) string { return fn.file + ":" + strconv.Itoa(fset.Position(n.Pos()).Line) }
		taintedIdents := make([]string, 0, len(fn.tainted))
		for id := range fn.tainted {
			taintedIdents = append(taintedIdents, id)
		}
		sort.Strings(taintedIdents)
		anyTaint := len(taintedIdents) > 0
		for _, call := range fn.calls {
			// (b) sinks: the required-id set functions.
			if behaviorContractRequiredIDFuncs[call.callee] {
				for _, arg := range call.args {
					if fn.exprTainted(arg) {
						offenders = append(offenders, where(call.node)+" feeds the pre-rebase analyzer snapshot (directly or via alias/parameter) to "+call.callee)
					}
				}
			}
			// (e) the pure projection is a types/writeflow-seed API.
			if call.callee == "ProjectWriteBehaviorContractGeneration" && fn.pkg != "writeflow" {
				offenders = append(offenders, where(call.node)+" calls the pure projection; use Mutable.ProjectBehaviorContractGeneration so the run's tombstone ledger is supplied from state")
			}
		}
		// (d) applies to every censused package, assignment and literal.
		if fn.writesField && fn.name != "attachWriteBehaviorContracts" {
			offenders = append(offenders, fn.file+": "+fn.name+" writes a tombstone/generation field (only attachWriteBehaviorContracts may)")
		}
		// (f) rendering packages never iterate/index the snapshot.
		if fn.pkg == "agent" || fn.pkg == "writeflow" {
			ast.Inspect(fn.decl.Body, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.RangeStmt:
					if fn.exprTainted(v.X) {
						offenders = append(offenders, where(v)+" iterates the pre-rebase analyzer snapshot (directly or via alias/parameter); render the projection")
					}
				case *ast.IndexExpr:
					if fn.exprTainted(v.X) {
						offenders = append(offenders, where(v)+" indexes the pre-rebase analyzer snapshot (directly or via alias/parameter); render the projection")
					}
				}
				return true
			})
		}
		// (c) refs gates in package tool.
		if fn.pkg != "tool" || !fn.stringResult || !fn.readsRefs {
			continue
		}
		if fn.readsIR || anyTaint {
			offenders = append(offenders, fn.file+": "+fn.name+" is a refs gate that consults the analyzer snapshot / WriteAnalysisIR() (directly or via alias/parameter "+strings.Join(taintedIdents, ",")+")")
			continue
		}
		if !fn.idSource {
			continue
		}
		resolutionChecked++
		resolved := fn.authority
		for _, call := range fn.calls {
			// A helper resolves the gate only when it is itself clean: an
			// authority caller that also consults the snapshot (or a tainted
			// parameter) resolves nothing.
			if helper, ok := byPkgName[fn.pkg][call.callee]; ok && helper != fn && helper.authority && !helper.readsRefs && !helper.readsIR && len(helper.tainted) == 0 {
				resolved = true
			}
		}
		if !resolved {
			offenders = append(offenders, fn.file+": "+fn.name+" resolves contract ids without resolveBehaviorContractIDs / the projection")
		}
	}
	sort.Strings(offenders)
	return offenders, resolutionChecked, nil
}

func TestBehaviorContractIDResolutionCensus(t *testing.T) {
	// Every non-test Go file of every package under internal/ and cmd/ except
	// internal/types (the authority itself) is scanned: the guarded APIs are
	// exported, so totality means "every package that can import types", not
	// the four packages that consume them today. Rules (c)/(f) are keyed on
	// the package; rules (a)/(b)/(d)/(e) and the cross-package call taint
	// apply everywhere.
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var files []behaviorContractCensusFile
	counts := map[string]int{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			if !(rel == "internal" || rel == "cmd" || strings.HasPrefix(rel, "internal/") || strings.HasPrefix(rel, "cmd/")) {
				return filepath.SkipDir
			}
			if rel == "internal/types" || d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		files = append(files, behaviorContractCensusFile{name: rel, src: string(src), pkg: behaviorContractCensusPackageKey(dir)})
		counts[dir]++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"internal/tool", "internal/agent", "internal/writeflow", "internal/orchestrator"} {
		if counts[dir] < 10 {
			t.Fatalf("census floor: only %d non-test files under %s", counts[dir], dir)
		}
	}
	if len(counts) < 20 || counts["cmd"] == 0 {
		t.Fatalf("census floor: only %d packages scanned (cmd=%d) — walk drift", len(counts), counts["cmd"])
	}
	offenders, checked, err := behaviorContractIDResolutionCensus(files)
	if err != nil {
		t.Fatal(err)
	}
	// Drift floor over RESOLUTION-CHECKED gates: the combined refs gate and
	// the proof follow-up gate (through its id helper) must both be judged.
	if checked < 2 {
		t.Fatalf("census resolution-checked only %d refs gates — collector drift", checked)
	}
	if len(offenders) > 0 {
		t.Fatalf("behavior-contract id resolution census:\n  %s", strings.Join(offenders, "\n  "))
	}
}

// TestBehaviorContractIDResolutionCensusSelfRed: every evasion shape the
// census claims to close is flagged on a probe source (red witnesses of the
// tripwire itself). The three shapes named in §40.46 C4 — alias, parameter-
// passing helper, composite-literal writer — plus the pure-projection call and
// the original direct shapes.
func TestBehaviorContractIDResolutionCensusSelfRed(t *testing.T) {
	run := func(t *testing.T, name, pkg, src string, wantChecked int, wants ...string) {
		t.Helper()
		offenders, checked, err := behaviorContractIDResolutionCensus([]behaviorContractCensusFile{{name: name, src: src, pkg: pkg}})
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(offenders, "\n")
		for _, want := range wants {
			if !strings.Contains(joined, want) {
				t.Fatalf("self-check missed %q in:\n%s", want, joined)
			}
		}
		if checked != wantChecked {
			t.Fatalf("self-check resolution-checked %d gates, want %d (offenders:\n%s)", checked, wantChecked, joined)
		}
	}
	t.Run("direct", func(t *testing.T) {
		run(t, "probe.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func probeRefsGate(ctx *types.BusContext, probes []types.VerificationProbe) string {
	ir := ctx.Mutable.WriteAnalysisIR()
	ids := types.WriteBehaviorContractIDs(ir.Request.BehaviorContracts)
	required := types.RequiredWriteBehaviorContractIDs(ir.Request.BehaviorContracts, true)
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if _, ok := ids[ref]; !ok {
				return ref
			}
			if _, ok := required[ref]; !ok {
				return ref
			}
		}
	}
	return ""
}
func probeWriter(plan *types.ChangePlan) { plan.SupersededBehaviorContractIDs = nil }
`, 0, "references WriteBehaviorContractIDs", "feeds the pre-rebase analyzer snapshot", "is a refs gate that consults the analyzer snapshot", "writes a tombstone/generation field")
	})
	t.Run("unresolved_gate", func(t *testing.T) {
		run(t, "probe.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func probeGate(plan *types.ChangePlan, probes []types.VerificationProbe) string {
	ids := map[string]bool{}
	for _, c := range plan.BehaviorContracts {
		ids[c.ID] = true
	}
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if !ids[ref] {
				return ref
			}
		}
	}
	return ""
}
`, 1, "resolves contract ids without")
	})
	t.Run("agent_alias", func(t *testing.T) {
		run(t, "x.go", "agent", `package agent
import "github.com/hanchaoqun/codrax/internal/types"
func render(ir *types.WriteAnalysisIR) string {
	contracts := ir.Request.BehaviorContracts
	out := ""
	for _, c := range contracts {
		out += c.ID
	}
	_ = types.RequiredWriteBehaviorContractIDs(contracts, true)
	return out
}
`, 0, "feeds the pre-rebase analyzer snapshot (directly or via alias/parameter) to RequiredWriteBehaviorContractIDs", "iterates the pre-rebase analyzer snapshot")
	})
	t.Run("tool_parameter_helper", func(t *testing.T) {
		run(t, "x.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func validateRefsRaw(contracts []types.WriteBehaviorContract, probes []types.VerificationProbe) string {
	ids := map[string]bool{}
	for _, c := range contracts {
		ids[c.ID] = true
	}
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if !ids[ref] {
				return ref
			}
		}
	}
	return ""
}
func caller(ctx *types.BusContext, probes []types.VerificationProbe) (string, error) {
	ir := ctx.Mutable.WriteAnalysisIR()
	return validateRefsRaw(ir.Request.BehaviorContracts, probes), nil
}
`, 0, "validateRefsRaw is a refs gate that consults the analyzer snapshot / WriteAnalysisIR() (directly or via alias/parameter c,contracts)")
	})
	t.Run("tool_two_hop_parameter", func(t *testing.T) {
		run(t, "x.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func idsOf(contracts []types.WriteBehaviorContract) map[string]bool {
	ids := map[string]bool{}
	for _, c := range contracts {
		ids[c.ID] = true
	}
	return ids
}
func gate(contracts []types.WriteBehaviorContract, probes []types.VerificationProbe) string {
	ids := idsOf(contracts)
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if !ids[ref] {
				return ref
			}
		}
	}
	return ""
}
func caller(ctx *types.BusContext, probes []types.VerificationProbe) (string, error) {
	snapshot := ctx.Mutable.WriteAnalysisIR().Request.BehaviorContracts
	return gate(snapshot, probes), nil
}
`, 0, "gate is a refs gate that consults the analyzer snapshot")
	})
	t.Run("cross_package_parameter_taint", func(t *testing.T) {
		// An orchestrator caller feeding the snapshot into an exported tool
		// refs gate through a qualified call: the taint crosses the package
		// boundary via the import block, so the gate is judged tainted.
		offenders, checked, err := behaviorContractIDResolutionCensus([]behaviorContractCensusFile{
			{name: "internal/tool/x.go", pkg: "tool", src: `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func ValidateRefsRaw(contracts []types.WriteBehaviorContract, probes []types.VerificationProbe) string {
	ids := map[string]bool{}
	for _, c := range contracts {
		ids[c.ID] = true
	}
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if !ids[ref] {
				return ref
			}
		}
	}
	return ""
}
`},
			{name: "internal/orchestrator/y.go", pkg: "orchestrator", src: `package orchestrator
import (
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)
func run(ir *types.WriteAnalysisIR, probes []types.VerificationProbe) (string, error) {
	return tool.ValidateRefsRaw(ir.Request.BehaviorContracts, probes), nil
}
`},
			{name: "internal/writeflow/z.go", pkg: "writeflow", src: `package writeflow
import wt "github.com/hanchaoqun/codrax/internal/tool"
func alias(contracts []int) string { return wt.Other(contracts) }
`},
		})
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(offenders, "\n")
		if !strings.Contains(joined, "ValidateRefsRaw is a refs gate that consults the analyzer snapshot / WriteAnalysisIR() (directly or via alias/parameter c,contracts)") {
			t.Fatalf("cross-package taint missed:\n%s", joined)
		}
		if checked != 0 {
			t.Fatalf("tainted gate must not count as resolution-checked (%d)", checked)
		}
	})
	t.Run("orchestrator_composite_literal_writer", func(t *testing.T) {
		run(t, "x.go", "orchestrator", `package orchestrator
import "github.com/hanchaoqun/codrax/internal/types"
func mint() *types.ChangePlan {
	return &types.ChangePlan{SupersededBehaviorContractIDs: nil, BehaviorContractGeneration: ""}
}
`, 0, "mint writes a tombstone/generation field")
	})
	t.Run("tool_pure_projection", func(t *testing.T) {
		run(t, "x.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func project(ctx *types.BusContext) types.WriteBehaviorContractResolution {
	ir := ctx.Mutable.WriteAnalysisIR()
	return types.ProjectWriteBehaviorContractGeneration(ir.Request.BehaviorContracts, nil, ctx.Mutable.VerifyFailureHandoff(), nil, nil)
}
`, 0, "calls the pure projection; use Mutable.ProjectBehaviorContractGeneration")
	})
	t.Run("proof_followup_helper_is_resolution_checked", func(t *testing.T) {
		// A gate that sources ids through a helper is judged through the
		// helper: unresolved helper ⇒ offender, resolved helper ⇒ checked.
		run(t, "x.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func requiredIDs(ctx *types.BusContext) map[string]struct{} {
	out := map[string]struct{}{}
	for _, c := range ctx.Mutable.WriteAnalysisIR().Request.BehaviorContracts {
		out[c.ID] = struct{}{}
	}
	return out
}
func gate(ctx *types.BusContext, probes []types.VerificationProbe) string {
	required := requiredIDs(ctx)
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if _, ok := required[ref]; !ok {
				return ref
			}
		}
	}
	return ""
}
`, 1, "gate resolves contract ids without")
		run(t, "y.go", "tool", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func requiredIDs(ctx *types.BusContext) map[string]struct{} {
	out := map[string]struct{}{}
	for _, c := range ctx.Mutable.ProjectBehaviorContractGeneration(nil, nil).Contracts {
		out[c.ID] = struct{}{}
	}
	return out
}
func gate(ctx *types.BusContext, probes []types.VerificationProbe) string {
	required := requiredIDs(ctx)
	for _, p := range probes {
		for _, ref := range p.ContractRefs {
			if _, ok := required[ref]; !ok {
				return ref
			}
		}
	}
	return ""
}
`, 1)
	})
}
