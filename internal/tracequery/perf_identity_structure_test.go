package tracequery

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// These caller-closure pins keep identity construction separate from hard
// consumers. A new CPU execution surface must deliberately reuse the combined
// predicate; a new source-only selector must deliberately reuse the typed
// thread gate instead of inspecting transport fields.
func TestPerfIdentityAndCPUAuthorityCallerClosure(t *testing.T) {
	calls := tracequeryProductionFunctionCalls(t)
	assertPerfHelperCallers(t, calls, "perfSampleHasKnownCPU", []string{
		"applyPerfBundleAdmission",
		"normalizePerfSampleClaims",
		"perfQualityAcc.add",
		"perfSampleHasOnCPUExecutionCoordinate",
	})
	assertPerfHelperCallers(t, calls, "perfSampleHasOnCPUExecutionCoordinate", []string{
		"perfSampleMatchesExecutionThread",
		"perfSampleOnCPUExecutionCPU",
	})
	assertPerfHelperCallers(t, calls, "perfSampleOnCPUExecutionCPU", []string{
		"BuildPerfTimeline",
		"computePerfContextFiltered",
		"perfContextForCPU",
		"perfContextForCPUs",
	})
	assertPerfHelperCallers(t, calls, "perfSampleMatchesExecutionThread", []string{
		"perfContextForExecutionThread",
		"perfContextForExecutionThreads",
	})
	assertPerfHelperCallers(t, calls, "perfSampleIsOnCPU", []string{
		"perfSampleHasOnCPUExecutionCoordinate",
	})
	assertPerfHelperCallers(t, calls, "perfSampleIsSourceOnlyIdentity", []string{
		"normalizePerfSampleClaims",
		"perfSampleHasTypedThreadIdentity",
	})
	assertPerfHelperCallers(t, calls, "perfSampleHasTypedThreadIdentity", []string{
		"eventMentionsPID",
		"eventMentionsThread",
		"perfSampleMatchesThread",
		"perfSampleThread",
	})
	assertPerfHelperCallers(t, calls, "perfThreadRefHasRosterIdentity", []string{
		"BuildPerfTimeline",
		"addPerfHotspot",
		"computePerfContextFiltered",
	})
	assertPerfHelperCallers(t, calls, "perfSampleCPUIsExplicitNoClaim", []string{
		"cpuInputValidationFailuresScan",
		"populatePerfSampleFields",
	})
	assertPerfHelperCallers(t, calls, "perfContextForExecutionThread", []string{
		"appendRootCauseRunnableCompetitorPerfContexts",
		"appendRootCauseStatsPerfContexts",
		"buildFramePerfContexts",
	})
	assertPerfHelperCallers(t, calls, "perfContextForExecutionThreads", []string{
		"buildFramePerfContexts",
	})
}

func TestPerfExecutionConsumersCannotReadEventCPUDirectly(t *testing.T) {
	allowed := map[string]bool{
		"perfSampleHasKnownCPU":       true,
		"perfSampleOnCPUExecutionCPU": true,
	}
	seenAllowed := map[string]bool{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !strings.Contains(strings.ToLower(fn.Name.Name), "perf") {
				continue
			}
			eventVars := perfStructureEventVariables(fn)
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				sel, ok := node.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "CPU" {
					return true
				}
				ident, ok := sel.X.(*ast.Ident)
				if !ok || !eventVars[ident.Name] {
					return true
				}
				if !allowed[fn.Name.Name] {
					t.Errorf("%s reads Event.%s directly; perf CPU execution consumers must use perfSampleOnCPUExecutionCPU", fn.Name.Name, sel.Sel.Name)
				} else {
					seenAllowed[fn.Name.Name] = true
				}
				return true
			})
		}
	}
	if !reflect.DeepEqual(seenAllowed, allowed) {
		t.Fatalf("direct Event.CPU authority closure drifted: got=%v want=%v", seenAllowed, allowed)
	}
}

func perfStructureEventVariables(fn *ast.FuncDecl) map[string]bool {
	out := map[string]bool{}
	addFields := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, field := range fields.List {
			ident, ok := field.Type.(*ast.Ident)
			if !ok || ident.Name != "Event" {
				continue
			}
			for _, name := range field.Names {
				out[name.Name] = true
			}
		}
	}
	addFields(fn.Type.Params)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.FuncLit:
			addFields(value.Type.Params)
		case *ast.RangeStmt:
			sel, ok := value.X.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Events" {
				return true
			}
			if ident, ok := value.Value.(*ast.Ident); ok {
				out[ident.Name] = true
			}
		}
		return true
	})
	return out
}

func assertPerfHelperCallers(t *testing.T, calls map[string][]string, helper string, want []string) {
	t.Helper()
	got := uniqueSortedStrings(calls[helper])
	want = uniqueSortedStrings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s caller closure drifted: got=%v want=%v", helper, got, want)
	}
}

func tracequeryProductionFunctionCalls(t *testing.T) map[string][]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	out := map[string][]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			caller := perfStructureFunctionName(fn)
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				ident, ok := call.Fun.(*ast.Ident)
				if !ok || !(strings.HasPrefix(ident.Name, "perfSample") || strings.HasPrefix(ident.Name, "perfContextForExecution") || strings.HasPrefix(ident.Name, "perfThreadRef")) {
					return true
				}
				out[ident.Name] = append(out[ident.Name], caller)
				return true
			})
		}
	}
	for helper := range out {
		sort.Strings(out[helper])
	}
	return out
}

func perfStructureFunctionName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	if ident, ok := recv.(*ast.Ident); ok {
		return ident.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}
