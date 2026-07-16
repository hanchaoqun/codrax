package hitraceconv

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestTraceDBPerfB3AProductionAuthorityAndResolverStructure(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files[name] = file
	}

	callers := map[string]int{}
	var productionCall *ast.CallExpr
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				ident, ok := call.Fun.(*ast.Ident)
				if ok && ident.Name == "exportTraceDBPerfSamples" {
					callers[fn.Name.Name]++
					productionCall = call
				}
				return true
			})
		}
	}
	if !reflect.DeepEqual(callers, map[string]int{"exportTraceDBExtendedFamilies": 1}) || productionCall == nil {
		t.Fatalf("perf exporter production callers=%v", callers)
	}
	if len(productionCall.Args) != 5 || !traceDBPerfASTIdent(productionCall.Args[3], "authority") ||
		!traceDBPerfASTIdent(productionCall.Args[4], "lifecycleRunning") {
		t.Fatalf("perf exporter does not receive the shared authority/typed Running values: %#v", productionCall.Args)
	}

	perfFile := files["streamerdb_export_perf.go"]
	if perfFile == nil {
		t.Fatal("strict perf exporter file missing")
	}
	forbidden := map[string]bool{
		"loadThreadIndex": true, "loadStrictThreadIndex": true,
		"loadRunningIntervals": true, "loadExtendedLegacyRunningIntervals": true,
		"collectTraceDBLifecycle": true, "newTraceDBSchedulerAuthority": true,
		"newTraceDBSchedulerRunningIndex": true, "traceDBExtendedRunningCPUAt": true,
	}
	lookupCPUCalls := 0
	pointCalls := 0
	ast.Inspect(perfFile, func(node ast.Node) bool {
		switch item := node.(type) {
		case *ast.CallExpr:
			switch callee := item.Fun.(type) {
			case *ast.Ident:
				if forbidden[callee.Name] {
					t.Errorf("strict perf exporter reloads/rebuilds shared authority through %s", callee.Name)
				}
			case *ast.SelectorExpr:
				if forbidden[callee.Sel.Name] {
					t.Errorf("strict perf exporter reloads/rebuilds shared authority through %s", callee.Sel.Name)
				}
				if callee.Sel.Name == "lookupCPUAt" {
					lookupCPUCalls++
					if len(item.Args) != 2 || !traceDBPerfASTThreadITID(item.Args[0]) || !traceDBPerfASTIdent(item.Args[1], "ts") {
						t.Errorf("typed Running lookup drifted from exact (resolved ITID,sample ts): %#v", item.Args)
					}
				}
				if callee.Sel.Name == "threadPointAllows" {
					pointCalls++
					if !traceDBPerfASTIdent(callee.X, "authority") || len(item.Args) != 2 ||
						!traceDBPerfASTThreadITID(item.Args[0]) || !traceDBPerfASTIdent(item.Args[1], "ts") {
						t.Errorf("lifecycle point gate drifted from authority.threadPointAllows(resolved ITID,sample ts): %#v", item.Args)
					}
				}
			}
		case *ast.BasicLit:
			if item.Kind != token.STRING {
				break
			}
			value, err := strconv.Unquote(item.Value)
			if err != nil {
				break
			}
			upper := strings.ToUpper(value)
			if strings.Contains(upper, "SELECT") && (strings.Contains(upper, " JOIN ") || strings.Contains(upper, "COALESCE(")) {
				t.Errorf("perf resolver/sample SQL can fan out or repair scalar values: %q", value)
			}
		}
		return true
	})
	if lookupCPUCalls != 1 {
		t.Fatalf("typed Running lookup call count=%d, want one single authority chokepoint", lookupCPUCalls)
	}
	if pointCalls != 1 {
		t.Fatalf("thread lifecycle point-gate call count=%d, want one authority chokepoint", pointCalls)
	}

	source, err := os.ReadFile("streamerdb_export_perf.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, token := range []string{
		"perf-unverified", "traceDBPerfSampleKindOffCPU", "tracewire.BuildPerfSampleBody",
		"tracewire.PerfSampleLayoutSourceOnlyIdentity", "tracewire.PerfSampleLayoutResolvedIdentity",
		"tracewire.PerfSampleKindSourceSchedulerRunning",
		`tsColumn := "timestamp_trace"`, "maxTraceDBIdentityDisplayBytes", "traceDBPerfAddressOnlyLabel",
		"duplicateIDs", "conflicting_depth", "RejectedPublicTID", "symbolized.Tainted", "files.Tainted", "dict.Tainted",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("strict perf wire/profile token %q missing", token)
		}
	}
	if strings.Contains(text, "LEFT JOIN") || strings.Contains(text, "traceDBPerfSampleIdentity(") {
		t.Fatalf("retired fanout/source-only identity implementation survived strict exporter")
	}
	if strings.Contains(text, "perf_sample:") {
		t.Fatal("SQL exporter bypassed the shared typed perf_sample wire builder")
	}

	wireSource, err := os.ReadFile(filepath.Join("..", "tracewire", "perf_sample.go"))
	if err != nil {
		t.Fatal(err)
	}
	wireText := string(wireSource)
	for _, token := range []string{
		`appendBare("thread_identity_known", "false")`, `appendBare("resolution", "perf_source_only")`,
		`appendBare("lifecycle_unverified", "true")`, `appendBare("perf_source_tid"`,
		`appendBare("perf_source_pid"`, `appendQuoted("perf_source_comm"`,
		`appendBare("thread_identity_known", "true")`, `appendBare("resolution", "resolved")`,
		`appendBare("lifecycle_unverified", "false")`, `appendBare("sample_kind_source"`,
	} {
		if !strings.Contains(wireText, token) {
			t.Fatalf("shared strict perf wire token %q missing", token)
		}
	}
}

func traceDBPerfASTIdent(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

func traceDBPerfASTThreadITID(expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "ITID" {
		return false
	}
	thread, ok := selector.X.(*ast.SelectorExpr)
	return ok && thread.Sel.Name == "Thread" && traceDBPerfASTIdent(thread.X, "identity")
}
