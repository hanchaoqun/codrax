package hitraceconv

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestTraceDBExtendedProductionReceivesOneSchedulerAuthority(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source")
	}
	dir := filepath.Dir(current)
	parse := func(name string) *ast.File {
		t.Helper()
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		return file
	}

	exportFile := parse("streamerdb_export.go")
	schedulerHandoffs := 0
	extendedHandoffs := 0
	schedulerStatement := -1
	extendedStatement := -1
	schedulerErrorGuarded := false
	for _, declaration := range exportFile.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "exportTraceDBToSystrace" || function.Body == nil {
			continue
		}
		for i, statement := range function.Body.List {
			assign, ok := statement.(*ast.AssignStmt)
			if !ok || len(assign.Rhs) != 1 {
				continue
			}
			call, ok := assign.Rhs[0].(*ast.CallExpr)
			if !ok {
				continue
			}
			callee, ok := call.Fun.(*ast.Ident)
			if !ok {
				continue
			}
			switch callee.Name {
			case "exportTraceDBSchedulerFamilies":
				schedulerStatement = i
				if i+1 >= len(function.Body.List) {
					continue
				}
				guard, ok := function.Body.List[i+1].(*ast.IfStmt)
				if !ok {
					continue
				}
				condition, ok := guard.Cond.(*ast.BinaryExpr)
				if !ok || condition.Op != token.NEQ {
					continue
				}
				left, leftOK := condition.X.(*ast.Ident)
				right, rightOK := condition.Y.(*ast.Ident)
				if !leftOK || !rightOK || left.Name != "err" || right.Name != "nil" {
					continue
				}
				for _, guardedStatement := range guard.Body.List {
					if _, ok := guardedStatement.(*ast.ReturnStmt); ok {
						schedulerErrorGuarded = true
					}
				}
			case "exportTraceDBExtendedFamilies":
				extendedStatement = i
			}
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.AssignStmt:
				if len(typed.Rhs) != 1 {
					return true
				}
				call, ok := typed.Rhs[0].(*ast.CallExpr)
				if !ok {
					return true
				}
				callee, ok := call.Fun.(*ast.Ident)
				if !ok || callee.Name != "exportTraceDBSchedulerFamilies" {
					return true
				}
				want := []string{"schedulerCoverage", "authority", "err"}
				if len(typed.Lhs) != len(want) {
					t.Fatalf("scheduler handoff results=%d, want %d", len(typed.Lhs), len(want))
				}
				for i, expression := range typed.Lhs {
					ident, ok := expression.(*ast.Ident)
					if !ok || ident.Name != want[i] {
						t.Fatalf("scheduler handoff result %d is not %s", i, want[i])
					}
				}
				schedulerHandoffs++
			case *ast.CallExpr:
				callee, ok := typed.Fun.(*ast.Ident)
				if !ok || callee.Name != "exportTraceDBExtendedFamilies" {
					return true
				}
				if len(typed.Args) != 4 {
					t.Fatalf("extended handoff args=%d, want 4", len(typed.Args))
				}
				authority, ok := typed.Args[3].(*ast.Ident)
				if !ok || authority.Name != "authority" {
					t.Fatal("extended exporter did not receive the scheduler authority result")
				}
				extendedHandoffs++
			}
			return true
		})
	}
	if schedulerHandoffs != 1 || extendedHandoffs != 1 {
		t.Fatalf("production authority handoffs scheduler=%d extended=%d, want 1/1", schedulerHandoffs, extendedHandoffs)
	}
	if schedulerStatement < 0 || extendedStatement <= schedulerStatement || !schedulerErrorGuarded {
		t.Fatalf("scheduler error guard does not dominate extended handoff: scheduler=%d extended=%d guarded=%t",
			schedulerStatement, extendedStatement, schedulerErrorGuarded)
	}

	schedulerFile := parse("streamerdb_export_sched.go")
	invalidAuthorityDecls := 0
	invalidAuthorityAssignments := 0
	for _, declaration := range schedulerFile.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "exportTraceDBSchedulerFamilies" || function.Body == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.ValueSpec:
				if len(typed.Names) != 1 || typed.Names[0].Name != "invalidHandoffAuthority" || len(typed.Values) != 0 {
					return true
				}
				kind, ok := typed.Type.(*ast.Ident)
				if !ok || kind.Name != "traceDBSchedulerAuthority" {
					t.Fatal("invalid handoff authority is not a zero-value scheduler authority")
				}
				invalidAuthorityDecls++
			case *ast.AssignStmt:
				if len(typed.Lhs) != 1 || len(typed.Rhs) != 1 {
					return true
				}
				left, leftOK := typed.Lhs[0].(*ast.Ident)
				right, rightOK := typed.Rhs[0].(*ast.Ident)
				if leftOK && rightOK && left.Name == "authority" && right.Name == "invalidHandoffAuthority" {
					invalidAuthorityAssignments++
				}
			}
			return true
		})
	}
	if invalidAuthorityDecls != 1 || invalidAuthorityAssignments != 1 {
		t.Fatalf("invalid handoff authority declaration/assignment=%d/%d, want 1/1",
			invalidAuthorityDecls, invalidAuthorityAssignments)
	}

	extendedFile := parse("streamerdb_export_extended.go")
	extendedFunctions := 0
	identitySelectors := 0
	for _, declaration := range extendedFile.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "exportTraceDBExtendedFamilies" || function.Body == nil {
			continue
		}
		extendedFunctions++
		authorityParams := 0
		for _, field := range function.Type.Params.List {
			if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "traceDBSchedulerAuthority" {
				authorityParams += len(field.Names)
			}
		}
		if authorityParams != 1 {
			t.Fatalf("extended authority params=%d, want 1", authorityParams)
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.CallExpr:
				name := ""
				switch callee := typed.Fun.(type) {
				case *ast.Ident:
					name = callee.Name
				case *ast.SelectorExpr:
					name = callee.Sel.Name
				}
				switch name {
				case "loadThreadIndex", "collectTraceDBLifecycle", "newTraceDBSchedulerAuthority":
					t.Fatalf("extended exporter rebuilt authority through %s", name)
				}
			case *ast.SelectorExpr:
				owner, ok := typed.X.(*ast.Ident)
				if ok && owner.Name == "authority" && typed.Sel.Name == "identities" {
					identitySelectors++
				}
			}
			return true
		})
	}
	if extendedFunctions != 1 || identitySelectors != 1 {
		t.Fatalf("extended authority function/identity selectors=%d/%d, want 1/1", extendedFunctions, identitySelectors)
	}

	callers := map[string]map[string]int{"loadActiveThreadIDs": {}}
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := ""
				switch callee := call.Fun.(type) {
				case *ast.Ident:
					name = callee.Name
				case *ast.SelectorExpr:
					name = callee.Sel.Name
				}
				switch name {
				case "exportTraceDBExtendedFamilies", "loadThreadIndex", "collectTraceDBLifecycle",
					"newTraceDBSchedulerAuthority", "loadActiveThreadIDs":
					if callers[name] == nil {
						callers[name] = map[string]int{}
					}
					callers[name][function.Name.Name]++
				}
				return true
			})
		}
	}
	wantCallers := map[string]map[string]int{
		"exportTraceDBExtendedFamilies": {"exportTraceDBToSystrace": 1},
		"loadThreadIndex":               {"exportTraceDBSchedulerFamilies": 1},
		"collectTraceDBLifecycle":       {"exportTraceDBSchedulerFamilies": 1, "loadActiveThreadIDs": 1},
		"newTraceDBSchedulerAuthority":  {"exportTraceDBSchedulerFamilies": 1},
		"loadActiveThreadIDs":           {},
	}
	if !reflect.DeepEqual(callers, wantCallers) {
		t.Fatalf("authority production call graph=%v, want %v", callers, wantCallers)
	}
}

func TestTraceDBExtendedRejectsMissingSharedAuthority(t *testing.T) {
	path := createTraceDBFixture(t, []string{"CREATE TABLE placeholder (id)"})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := exportTraceDBExtendedFamilies(context.Background(), tdb, sink, traceDBSchedulerAuthority{})
	if err == nil || len(coverage) != 0 || sink.stats.RowsAccepted != 0 {
		t.Fatalf("missing shared authority failed open: coverage=%+v sink=%+v err=%v", coverage, sink.stats, err)
	}
}

func TestTraceDBExtendedUsesHandoffIdentitiesWithoutReload(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (1, 90, 20, 3, 'Running')",
		"CREATE TABLE native_hook (id, start_ts, end_ts, event_type, all_heap_size, itid, ipid)",
		"INSERT INTO native_hook VALUES (1, 100, 0, 'malloc', 64, 1, 1)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := exportTraceDBExtendedFamilies(context.Background(), tdb, sink,
		traceDBSchedulerAuthorityFixture(true, traceDBLifecycleIndex{}))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range coverage {
		if item.Family == "resource" && item.Table == "native_hook" {
			found = item.RowsEmitted == 2
		}
	}
	if !found || sink.stats.RowsAccepted != 2 {
		t.Fatalf("extended stage ignored handoff identities: coverage=%+v sink=%+v", coverage, sink.stats)
	}
}

func TestTraceDBAuthorityHandoffErrorBoundaries(t *testing.T) {
	t.Run("failure before construction returns zero authority", func(t *testing.T) {
		path := createTraceDBFixture(t, []string{"CREATE TABLE placeholder (id)"})
		tdb, err := openTraceDB(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		defer tdb.close()
		sink, err := newTraceDBRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, authority, err := exportTraceDBSchedulerFamilies(ctx, tdb, sink)
		if err == nil || authority.initialized {
			t.Fatalf("pre-construction failure leaked authority: initialized=%t err=%v", authority.initialized, err)
		}
	})

	t.Run("failure after construction invalidates handoff authority", func(t *testing.T) {
		path := createTraceDBFixture(t, []string{
			"CREATE TABLE trace_range (start_ts)",
			"INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid, pid, name)",
			"INSERT INTO process VALUES (1, 100, 'proc')",
			"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
			"INSERT INTO thread VALUES (1, 42, 1, 'worker', 0, 0, 1)",
			"CREATE TABLE instant (ts, name, ref, ref_type)",
			"CREATE TABLE thread_state (ts, itid, state)",
			"CREATE TABLE sched_slice (ts, dur, itid, end_state)",
			"CREATE TABLE callstack (ts, itid, callid)",
			"INSERT INTO callstack VALUES (10, 1, NULL)",
			"CREATE TABLE syscall (ts, itid)",
			"CREATE TABLE native_hook (start_ts, itid)",
			"CREATE TABLE frame_slice (id, type, ts, itid)",
		})
		tdb, err := openTraceDB(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		defer tdb.close()
		badTempDir := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(badTempDir, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		sink, err := newTraceDBRowSink(badTempDir, 1)
		if err != nil {
			t.Fatal(err)
		}
		_, authority, err := exportTraceDBSchedulerFamilies(context.Background(), tdb, sink)
		if err == nil || authority.initialized {
			t.Fatalf("post-construction failure leaked authority: initialized=%t err=%v", authority.initialized, err)
		}
	})
}

func TestTraceDBAuthorityHandoffKeepsSingleIdentityCoverageAndLifecycleSuffix(t *testing.T) {
	statements := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"CREATE TABLE instant (ts, name, ref, ref_type)",
		"CREATE TABLE thread_state (ts, itid, state)",
		"CREATE TABLE sched_slice (ts, dur, itid, end_state)",
		"CREATE TABLE callstack (ts, itid, callid)",
		"CREATE TABLE syscall (ts, itid)",
		"CREATE TABLE native_hook (start_ts, itid)",
		"CREATE TABLE frame_slice (id, type, ts, itid)",
	}
	path := createTraceDBFixture(t, statements)
	result, err := exportTraceDBToSystrace(context.Background(), path, filepath.Join(t.TempDir(), "unused.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	wantIdentity := []string{"trace_range", "process", "thread"}
	counts := map[string]int{}
	for i, table := range wantIdentity {
		if i >= len(result.Coverage) || result.Coverage[i].Family != "resolver" || result.Coverage[i].Table != table {
			t.Fatalf("identity coverage prefix[%d]=%+v, want resolver/%s", i, result.Coverage[i], table)
		}
	}
	for _, item := range result.Coverage {
		if item.Family == "resolver" {
			for _, table := range wantIdentity {
				if item.Table == table {
					counts[table]++
				}
			}
		}
	}
	if !reflect.DeepEqual(counts, map[string]int{"trace_range": 1, "process": 1, "thread": 1}) {
		t.Fatalf("identity coverage was duplicated or lost: %v", counts)
	}
	if len(result.Coverage) < 2 || result.Coverage[len(result.Coverage)-1].Family != "sorter" {
		t.Fatalf("sorter coverage is not final: %+v", result.Coverage)
	}
	firstLifecycle := -1
	schedulerRegular := -1
	extendedRegular := -1
	for i, item := range result.Coverage[:len(result.Coverage)-1] {
		if item.Family == "scheduler" && item.Table == "sched_slice" && schedulerRegular < 0 {
			schedulerRegular = i
		}
		if item.Family == "perf" && item.Table == "perf_sample" && extendedRegular < 0 {
			extendedRegular = i
		}
		if strings.HasPrefix(item.Family, "resolver.lifecycle") {
			firstLifecycle = i
			break
		}
	}
	if firstLifecycle < 0 {
		t.Fatalf("lifecycle coverage suffix missing: %+v", result.Coverage)
	}
	if schedulerRegular < 0 || extendedRegular <= schedulerRegular || firstLifecycle <= extendedRegular {
		t.Fatalf("coverage stage order scheduler=%d extended=%d lifecycle=%d", schedulerRegular, extendedRegular, firstLifecycle)
	}
	for i := firstLifecycle; i < len(result.Coverage)-1; i++ {
		if !strings.HasPrefix(result.Coverage[i].Family, "resolver.lifecycle") {
			t.Fatalf("non-lifecycle coverage entered deferred suffix at %d: %+v", i, result.Coverage[i])
		}
	}
}
