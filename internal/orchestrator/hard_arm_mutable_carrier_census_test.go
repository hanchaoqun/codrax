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
	kind   string // "carrier" (needs a reset) | "counter" (monotonic witness)
	reset  string // MutableState reset method; required for carriers
}

// hardArmMutableCarriers is the single declared table. Adding a new
// `mut.<X>()` read to accepted_closure_retry_authority.go without
// registering it here is red; registering a carrier whose reset has no
// production caller is red.
var hardArmMutableCarriers = []hardArmMutableCarrier{
	{getter: "RetryState", kind: "carrier", reset: "ResetRetryState"},
	{getter: "ExploreBacktrackEpoch", kind: "counter"},
	{getter: "InvestigationCompleteGeneration", kind: "counter"},
}

const hardArmRetryAuthorityFile = "accepted_closure_retry_authority.go"

// TestHardArmMutableCarrierCensus_ArmReadsOnlyRegisteredCarriers (a):
// every method call on a *types.MutableState parameter inside the arm
// file must be a registered getter.
func TestHardArmMutableCarrierCensus_ArmReadsOnlyRegisteredCarriers(t *testing.T) {
	registered := map[string]bool{}
	for _, c := range hardArmMutableCarriers {
		if c.kind != "carrier" && c.kind != "counter" {
			t.Fatalf("table entry %q has unknown kind %q", c.getter, c.kind)
		}
		if c.kind == "carrier" && c.reset == "" {
			t.Fatalf("carrier %q must declare its reset method", c.getter)
		}
		if c.kind == "counter" && c.reset != "" {
			t.Fatalf("counter %q must not declare a reset (monotonic witness)", c.getter)
		}
		registered[c.getter] = true
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, hardArmRetryAuthorityFile, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", hardArmRetryAuthorityFile, err)
	}
	var unregistered []string
	seen := map[string]bool{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		mutableParams := mutableStateParamNames(fn)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok || !mutableParams[ident.Name] {
				return true
			}
			seen[sel.Sel.Name] = true
			if !registered[sel.Sel.Name] {
				unregistered = append(unregistered, fset.Position(call.Pos()).String()+" "+ident.Name+"."+sel.Sel.Name+"()")
			}
			return true
		})
	}
	if len(seen) == 0 {
		t.Fatalf("%s reads no MutableState carrier — the census lost its subject", hardArmRetryAuthorityFile)
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
