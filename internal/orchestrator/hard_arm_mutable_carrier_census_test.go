package orchestrator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// hardArmMutableCarrier is one MutableState read the accepted-closure
// retry authority (a hard arm) is allowed to consult. §40.14 V7-2 rule:
// every Mutable carrier read by a hard arm must have an explicit
// lifecycle writer pair (populate / reset) and the reset must have a
// production call site — a carrier whose reset has zero callers is a
// stale signal that can never lift, and a hard arm reading it fires on
// decisions from an earlier generation. Counters are generation
// witnesses (monotonic, compared as single integers) and carry no reset.
type hardArmMutableCarrier struct {
	getter string // MutableState method the arm may call
	// kind: "carrier" (needs a MutableState reset), "counter" (monotonic
	// witness), "retained" (deliberately sticky witness that survives
	// resets — documented, never cleared), "closure" (a sub-object whose own
	// pending sets are cleared by the named EvidenceClosure method).
	kind  string
	reset string // reset method; required for carriers and closures
}

// hardArmMutableCarriers is the single declared table. Adding a new
// `mut.<X>()` read to accepted_closure_retry_authority.go without
// registering it here is red; registering a carrier whose reset has no
// production caller is red.
var hardArmMutableCarriers = []hardArmMutableCarrier{
	{getter: "RetryState", kind: "carrier", reset: "ResetRetryState"},
	{getter: "ExploreBacktrackEpoch", kind: "counter"},
	{getter: "InvestigationCompleteGeneration", kind: "counter"},
	// §40.42 ①: the enclosing hard gate's own reads.
	{getter: "IsInvestigationComplete", kind: "carrier", reset: "ResetInvestigationComplete"},
	{getter: "StableInvestigationCompleteReason", kind: "retained"},
	{getter: "EvidenceClosure", kind: "closure", reset: "ClearPendingReads"},
}

const hardArmRetryAuthorityFile = "accepted_closure_retry_authority.go"

// hardArmGateFunctions are the hard-gate bodies censused beside the arm file:
// every `mut.X()` / `o.busCtx.Mutable.X()` call inside them must be
// registered (§40.42 ① — the census covers the gate body, not only the arm).
var hardArmGateFunctions = map[string][]string{
	"orchestrator.go":                     {"shouldAutoCompleteExploreWindowFromAcceptedClosure"},
	"accepted_closure_origin_debt.go":     {"acceptedClosureMissingRequiredOriginsForAutoComplete"},
	"accepted_closure_retry_authority.go": nil, // whole file, MutableState params
}

// TestHardArmMutableCarrierCensus_ArmReadsOnlyRegisteredCarriers (a):
// every method call on a *types.MutableState parameter inside the arm
// file must be a registered getter.
func TestHardArmMutableCarrierCensus_ArmReadsOnlyRegisteredCarriers(t *testing.T) {
	registered := map[string]bool{}
	for _, c := range hardArmMutableCarriers {
		switch c.kind {
		case "carrier", "closure":
			if c.reset == "" {
				t.Fatalf("%s %q must declare its reset method", c.kind, c.getter)
			}
		case "counter", "retained":
			if c.reset != "" {
				t.Fatalf("%s %q must not declare a reset", c.kind, c.getter)
			}
		default:
			t.Fatalf("table entry %q has unknown kind %q", c.getter, c.kind)
		}
		registered[c.getter] = true
	}
	fset := token.NewFileSet()
	var unregistered []string
	seen := map[string]bool{}
	for file, functions := range hardArmGateFunctions {
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		wanted := map[string]bool{}
		for _, name := range functions {
			wanted[name] = true
		}
		found := map[string]bool{}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || (len(functions) > 0 && !wanted[fn.Name.Name]) {
				continue
			}
			found[fn.Name.Name] = true
			mutableParams := mutableStateParamNames(fn)
			// Local aliases `mut := o.busCtx.Mutable` read the same carrier.
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if assign, ok := n.(*ast.AssignStmt); ok && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
					if lhs, ok := assign.Lhs[0].(*ast.Ident); ok && hardArmExprIsBusMutable(assign.Rhs[0]) {
						mutableParams[lhs.Name] = true
					}
				}
				return true
			})
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				receiver := ""
				if ident, ok := sel.X.(*ast.Ident); ok && mutableParams[ident.Name] {
					receiver = ident.Name
				} else if hardArmExprIsBusMutable(sel.X) {
					receiver = "o.busCtx.Mutable"
				}
				if receiver == "" {
					return true
				}
				seen[sel.Sel.Name] = true
				if !registered[sel.Sel.Name] {
					unregistered = append(unregistered, fset.Position(call.Pos()).String()+" "+receiver+"."+sel.Sel.Name+"()")
				}
				return true
			})
		}
		for name := range wanted {
			if !found[name] {
				t.Fatalf("hard gate function %s not found in %s — the census lost its subject", name, file)
			}
		}
	}
	if len(seen) < 4 {
		t.Fatalf("the censused hard gate reads only %v — the census lost its subject", seen)
	}
	if len(unregistered) > 0 {
		sort.Strings(unregistered)
		t.Fatalf("hard arm reads MutableState carriers that are not in hardArmMutableCarriers (register the getter with its lifecycle reset, or a counter kind):\n  %s",
			strings.Join(unregistered, "\n  "))
	}
}

// TestHardArmMutableCarrierCensus_CarrierResetsHaveProductionCallers (b):
// every registered getter/reset is a real MutableState method, and every
// carrier's reset has at least one non-test call site under internal/
// or cmd/.
func TestHardArmMutableCarrierCensus_CarrierResetsHaveProductionCallers(t *testing.T) {
	methods := mutableStateMethodNames(t)
	callSites := map[string][]string{}
	for _, root := range []string{filepath.Join("..", "..", "internal"), filepath.Join("..", "..", "cmd")} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					callSites[sel.Sel.Name] = append(callSites[sel.Sel.Name], fset.Position(call.Pos()).String())
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	for _, c := range hardArmMutableCarriers {
		if !methods[c.getter] {
			t.Errorf("%s: getter %q is not a MutableState method", c.kind, c.getter)
		}
		if c.kind == "closure" {
			// The closure's own clear method must have a production caller.
			if len(callSites[c.reset]) == 0 {
				t.Errorf("closure %q: reset %q has zero non-test call sites", c.getter, c.reset)
			}
			continue
		}
		if c.kind != "carrier" {
			continue
		}
		if !methods[c.reset] {
			t.Errorf("carrier %q: reset %q is not a MutableState method", c.getter, c.reset)
			continue
		}
		if len(callSites[c.reset]) == 0 {
			t.Errorf("carrier %q: reset %q has zero non-test call sites under internal/ and cmd/ — a hard-arm carrier whose reset never runs in production is a stale signal that can never lift (§40.14 V7-2)",
				c.getter, c.reset)
		}
	}
}

// mutableStateParamNames returns the parameter names of fn typed
// *types.MutableState (or *MutableState).
func mutableStateParamNames(fn *ast.FuncDecl) map[string]bool {
	out := map[string]bool{}
	if fn.Type == nil || fn.Type.Params == nil {
		return out
	}
	for _, field := range fn.Type.Params.List {
		star, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		name := ""
		switch x := star.X.(type) {
		case *ast.SelectorExpr:
			name = x.Sel.Name
		case *ast.Ident:
			name = x.Name
		}
		if name != "MutableState" {
			continue
		}
		for _, id := range field.Names {
			out[id.Name] = true
		}
	}
	return out
}

// mutableStateMethodNames parses ../types and returns every method
// declared on *MutableState.
func mutableStateMethodNames(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	matches, err := filepath.Glob(filepath.Join("..", "types", "*.go"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("glob ../types: %v (matches=%d)", err, len(matches))
	}
	fset := token.NewFileSet()
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}
			star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			if id, ok := star.X.(*ast.Ident); ok && id.Name == "MutableState" {
				out[fn.Name.Name] = true
			}
		}
	}
	return out
}

// hardArmExprIsBusMutable matches the `o.busCtx.Mutable` receiver chain.
func hardArmExprIsBusMutable(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Mutable" {
		return false
	}
	inner, ok := sel.X.(*ast.SelectorExpr)
	if !ok || inner.Sel.Name != "busCtx" {
		return false
	}
	ident, ok := inner.X.(*ast.Ident)
	return ok && ident.Name == "o"
}
