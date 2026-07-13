package hitraceconv

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceDBNativeHookB2OriginAndRunningEndpointsStructurallyPinned(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve native-hook B2 test source")
	}
	path := filepath.Join(filepath.Dir(current), "streamerdb_export_native_hook.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse native-hook production source: %v", err)
	}
	isIdent := func(expression ast.Expr, name string) bool {
		ident, ok := expression.(*ast.Ident)
		return ok && ident.Name == name
	}
	isEventField := func(expression ast.Expr, field string) bool {
		selector, ok := expression.(*ast.SelectorExpr)
		return ok && selector.Sel.Name == field && isIdent(selector.X, "event")
	}
	hasEventField := func(expression ast.Expr, field string) bool {
		found := false
		ast.Inspect(expression, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == field && isIdent(selector.X, "event") {
				found = true
				return false
			}
			return !found
		})
		return found
	}
	declarations := 0
	pointCalls := 0
	runningCalls := 0
	authorityParams := 0
	runningParams := 0
	rawIndexParams := 0
	var pointPosition, runningPosition token.Pos
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "prepareTraceDBNativeHookEvent" || function.Body == nil {
			continue
		}
		declarations++
		for _, field := range function.Type.Params.List {
			ident, ok := field.Type.(*ast.Ident)
			if !ok {
				continue
			}
			switch ident.Name {
			case "traceDBSchedulerAuthority":
				authorityParams += len(field.Names)
			case "traceDBSchedulerRunningIndex":
				runningParams += len(field.Names)
			case "traceDBThreadIndex":
				rawIndexParams += len(field.Names)
			}
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ""
			receiver := ""
			switch callee := call.Fun.(type) {
			case *ast.Ident:
				name = callee.Name
			case *ast.SelectorExpr:
				name = callee.Sel.Name
				if ident, ok := callee.X.(*ast.Ident); ok {
					receiver = ident.Name
				}
			}
			for _, argument := range call.Args {
				if hasEventField(argument, "End") {
					t.Fatalf("native resource End entered call %s", name)
				}
			}
			switch name {
			case "threadPointAllows":
				if receiver != "authority" || len(call.Args) != 2 ||
					!isEventField(call.Args[0], "EmitterITID") || !isEventField(call.Args[1], "TS") {
					t.Fatalf("native origin point gate lost exact authority/TS arguments")
				}
				pointCalls++
				pointPosition = call.Pos()
			case "lookupCPUAt":
				if receiver != "running" || len(call.Args) != 2 ||
					!isEventField(call.Args[0], "EmitterITID") || !isEventField(call.Args[1], "TS") {
					t.Fatalf("native Running lookup lost exact typed origin arguments")
				}
				runningCalls++
				runningPosition = call.Pos()
			case "threadClosedEndpointAllows", "threadSourceIntervalAllows", "knownCPUAt",
				"traceDBKnownCPUAt", "traceDBExtendedRunningCPUAt":
				t.Fatalf("native origin-only producer acquired forbidden endpoint/CPU helper %s", name)
			}
			return true
		})
	}
	if declarations != 1 || authorityParams != 1 || runningParams != 1 || rawIndexParams != 0 {
		t.Fatalf("native prepare declaration/typed params=%d/%d/%d/%d, want 1/1/1/0",
			declarations, authorityParams, runningParams, rawIndexParams)
	}
	if pointCalls != 1 || runningCalls != 1 || pointPosition == 0 || runningPosition == 0 || pointPosition >= runningPosition {
		t.Fatalf("native endpoint call order point/running=%d/%d positions=%d/%d, want one origin gate before one typed lookup",
			pointCalls, runningCalls, pointPosition, runningPosition)
	}
}

func TestTraceDBNativeHookB2OriginPointAndResourceEndOnly(t *testing.T) {
	runningRows := []string{
		"INSERT INTO thread_state VALUES (1, 90, 10, 1, 'Running', 501, 500)",
		"INSERT INTO thread_state VALUES (2, 100, 20, 2, 'Running', 501, 500)",
	}
	hookRows := []string{
		"INSERT INTO native_hook VALUES (1, 95, 100, 'AllocEvent', 100, 1, 1)",
		"INSERT INTO native_hook VALUES (2, 96, 110, 'MmapEvent', 200, 1, 1)",
		"INSERT INTO native_hook VALUES (3, 100, 110, 'FreeEvent', 90, 1, 1)",
		"INSERT INTO native_hook VALUES (4, 100, 110, 'MunmapEvent', 0, 2, 2)",
		"INSERT INTO native_hook VALUES (5, 97, 9223372036854775807, 'FD_Open_Event', 1, 1, 1)",
	}
	for _, test := range []struct {
		name       string
		threadCut  bool
		processCut bool
	}{
		{name: "thread cut", threadCut: true},
		{name: "process cut", processCut: true},
		{name: "thread and process cuts", threadCut: true, processCut: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, coverage, outPath := exportTraceDBNativeHookB2Fixture(t,
				traceDBNativeHookB2Statements(runningRows, hookRows),
				traceDBNativeHookB2CutLifecycle(test.threadCut, test.processCut), true)
			if coverage.RowsEmitted != 8 || coverage.Skipped != "lifecycle_rejected_event_origin=1" {
				t.Fatalf("origin-only lifecycle accounting mismatch: coverage=%+v\n%s", coverage, body)
			}
			for _, want := range []struct {
				event string
				cpu   int64
			}{
				{event: "AllocEvent", cpu: 1},
				{event: "MmapEvent", cpu: 1},
				{event: "MunmapEvent", cpu: 2},
				{event: "FD_Open_Event", cpu: 1},
			} {
				assertTraceDBNativeHookB2InstantCPU(t, body, want.event, want.cpu)
			}
			if traceDBNativeHookB2HasInstant(body, "FreeEvent") {
				t.Fatalf("old generation event at the cut escaped the origin point gate:\n%s", body)
			}
			semantics := coverage.FieldSources["event_semantics"]
			if !strings.Contains(semantics, "start_ts is the only emitter lifecycle point") ||
				!strings.Contains(semantics, "end_ts and any upstream dur derived from it are resource metadata") {
				t.Fatalf("native resource endpoint semantics are not disclosed: %+v", coverage.FieldSources)
			}
			actions := traceDBNativeHookB2Actions(t, outPath)
			if !reflect.DeepEqual(actions, map[string]int{"I": 4, "C": 4}) {
				t.Fatalf("native resource events minted non-I/C evidence: actions=%v\n%s", actions, body)
			}
		})
	}
}

func TestTraceDBNativeHookB2OriginAuthorityPoisonAndLocality(t *testing.T) {
	statements := traceDBNativeHookB2Statements(
		[]string{
			"INSERT INTO thread_state VALUES (1, 90, 10, 1, 'Running', 501, 500)",
			"INSERT INTO thread_state VALUES (3, 90, 10, 3, 'Running', 701, 700)",
		},
		[]string{
			"INSERT INTO native_hook VALUES (1, 95, 0, 'AllocEvent', 100, 1, 1)",
			"INSERT INTO native_hook VALUES (2, 95, 0, 'FreeEvent', 90, 3, 3)",
		},
	)
	for _, test := range []struct {
		name          string
		complete      bool
		lifecycle     traceDBLifecycleIndex
		wantOrigin    int
		wantInstants  []string
		absentInstant []string
	}{
		{
			name: "incomplete authority", complete: false, wantOrigin: 2,
			absentInstant: []string{"AllocEvent", "FreeEvent"},
		},
		{
			name: "global taint", complete: true, wantOrigin: 2,
			lifecycle:     traceDBLifecycleIndex{GlobalTaint: true},
			absentInstant: []string{"AllocEvent", "FreeEvent"},
		},
		{
			name: "global point poison", complete: true, wantOrigin: 2,
			lifecycle:     traceDBLifecycleIndex{GlobalPoison: []int64{95}},
			absentInstant: []string{"AllocEvent", "FreeEvent"},
		},
		{
			name: "thread lane poison is local", complete: true, wantOrigin: 1,
			lifecycle: traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{
				501: {PoisonPoints: []int64{95}},
			}},
			wantInstants: []string{"FreeEvent"}, absentInstant: []string{"AllocEvent"},
		},
		{
			name: "process lane poison is local", complete: true, wantOrigin: 1,
			lifecycle: traceDBLifecycleIndex{ByPID: map[int64]traceDBLifecycleLane{
				500: {PoisonPoints: []int64{95}},
			}},
			wantInstants: []string{"FreeEvent"}, absentInstant: []string{"AllocEvent"},
		},
		{
			name: "unrelated lane poison", complete: true,
			lifecycle: traceDBLifecycleIndex{
				ByTID: map[int64]traceDBLifecycleLane{999: {PoisonPoints: []int64{95}}},
				ByPID: map[int64]traceDBLifecycleLane{999: {PoisonPoints: []int64{95}}},
			},
			wantInstants: []string{"AllocEvent", "FreeEvent"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, coverage, _ := exportTraceDBNativeHookB2Fixture(t, statements, test.lifecycle, test.complete)
			wantRows := 2 * len(test.wantInstants)
			if coverage.RowsEmitted != wantRows {
				t.Fatalf("lane-local origin accounting rows=%d, want %d: coverage=%+v\n%s",
					coverage.RowsEmitted, wantRows, coverage, body)
			}
			wantSkipped := ""
			if test.wantOrigin > 0 {
				wantSkipped = fmt.Sprintf("lifecycle_rejected_event_origin=%d", test.wantOrigin)
			}
			if coverage.Skipped != wantSkipped {
				t.Fatalf("origin rejection summary=%q, want %q: %+v", coverage.Skipped, wantSkipped, coverage)
			}
			for _, event := range test.wantInstants {
				if !traceDBNativeHookB2HasInstant(body, event) {
					t.Fatalf("valid lane lost native event %q:\n%s", event, body)
				}
			}
			for _, event := range test.absentInstant {
				if traceDBNativeHookB2HasInstant(body, event) {
					t.Fatalf("poisoned lane published native event %q:\n%s", event, body)
				}
			}
		})
	}
}

func TestTraceDBNativeHookB2TypedRunningStatusesAndAntiRescue(t *testing.T) {
	tests := []struct {
		name        string
		lifecycle   traceDBLifecycleIndex
		runningRows []string
		wantReason  string
	}{
		{
			name: "source tainted lane",
			runningRows: []string{
				"INSERT INTO thread_state VALUES (1, 80, 10, 1, 'Running', 501, 500)",
				"INSERT INTO thread_state VALUES (1, 90, 5, 1, 'Running', 999, 500)",
				"INSERT INTO thread_state VALUES (3, 80, 10, 3, 'Running', 701, 700)",
			},
			wantReason: "tainted_running_cpu_witness=1",
		},
		{
			name:      "cross-cut lifecycle lane",
			lifecycle: traceDBNativeHookB2CutLifecycle(true, true),
			runningRows: []string{
				"INSERT INTO thread_state VALUES (1, 80, 10, 1, 'Running', 501, 500)",
				"INSERT INTO thread_state VALUES (1, 90, 11, 1, 'Running', 501, 500)",
				"INSERT INTO thread_state VALUES (3, 80, 10, 3, 'Running', 701, 700)",
			},
			wantReason: "lifecycle_rejected_running_cpu_witness=1",
		},
		{
			name: "unknown CPU",
			runningRows: []string{
				"INSERT INTO thread_state VALUES (3, 80, 10, 3, 'Running', 701, 700)",
			},
			wantReason: "unknown_event_cpu=1",
		},
	}
	hookRows := []string{
		"INSERT INTO native_hook VALUES (1, 85, 0, 'AllocEvent', 100, 1, 1)",
		"INSERT INTO native_hook VALUES (2, 85, 0, 'FreeEvent', 90, 3, 3)",
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, reverse := range []bool{false, true} {
				name := map[bool]string{false: "forward", true: "reverse"}[reverse]
				t.Run(name, func(t *testing.T) {
					runningRows := append([]string(nil), test.runningRows...)
					if reverse {
						reverseTraceDBNativeHookB2Statements(runningRows)
					}
					body, coverage, _ := exportTraceDBNativeHookB2Fixture(t,
						traceDBNativeHookB2Statements(runningRows, hookRows), test.lifecycle, true)
					if coverage.RowsEmitted != 2 || coverage.Skipped != test.wantReason {
						t.Fatalf("typed Running state collapsed or depended on order: coverage=%+v\n%s", coverage, body)
					}
					if traceDBNativeHookB2HasInstant(body, "AllocEvent") ||
						!traceDBNativeHookB2HasInstant(body, "FreeEvent") {
						t.Fatalf("bad Running lane rescued or unrelated lane lost:\n%s", body)
					}
					if strings.Contains(coverage.Skipped, "lifecycle_rejected_event_origin") {
						t.Fatalf("valid origin was misreported as an origin lifecycle failure: %+v", coverage)
					}
				})
			}
		})
	}
}

func TestTraceDBNativeHookB2CanonicalITIDProfile(t *testing.T) {
	highITID := int64(maxTraceDBInternalID)
	statements := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 500, 'app')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		fmt.Sprintf("INSERT INTO thread VALUES (%d, 501, 1, 'high-worker', 0, 0, 1)", highITID),
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
		fmt.Sprintf("INSERT INTO thread_state VALUES (%d, 0, 100, 2, 'Running', 501, 500)", highITID),
		"CREATE TABLE native_hook (id, start_ts, end_ts, event_type, all_heap_size, itid, ipid)",
		fmt.Sprintf("INSERT INTO native_hook VALUES (1, 10, 0, 'AllocEvent', 100, %d, 1)", highITID),
		"INSERT INTO native_hook VALUES (2, 20, 0, 'FreeEvent', 90, -2, 1)",
	}
	body, coverage, _ := exportTraceDBNativeHookB2Fixture(t, statements, traceDBLifecycleIndex{}, true)
	if coverage.RowsEmitted != 2 || coverage.Skipped != "invalid_emitter_itid=1" ||
		!traceDBNativeHookB2HasInstant(body, "AllocEvent") || traceDBNativeHookB2HasInstant(body, "FreeEvent") {
		t.Fatalf("native canonical ITID profile accepted a signed alias or lost high-half canonical identity: coverage=%+v\n%s", coverage, body)
	}
	if !strings.Contains(coverage.FieldSources["identity"], "canonical native_hook.itid/ipid") {
		t.Fatalf("native canonical identity profile is not disclosed: %+v", coverage.FieldSources)
	}
}

func TestTraceDBNativeHookB2SignedStableIDOrderAndDuplicates(t *testing.T) {
	t.Run("positive high-half alias is not a current producer row identity", func(t *testing.T) {
		rows := []string{
			"INSERT INTO native_hook VALUES (4294967294, 10, 0, 'AllocEvent', 100, 1, 1)",
			"INSERT INTO native_hook VALUES (-2, 11, 0, 'FreeEvent', 90, 1, 1)",
		}
		body, coverage, _ := exportTraceDBNativeHookB2Fixture(t,
			traceDBNativeHookB2Statements(
				[]string{"INSERT INTO thread_state VALUES (1, 0, 100, 1, 'Running', 501, 500)"}, rows),
			traceDBLifecycleIndex{}, true)
		if coverage.RowsEmitted != 2 || coverage.Skipped != "invalid_row_identity=1" ||
			traceDBNativeHookB2HasInstant(body, "AllocEvent") || !traceDBNativeHookB2HasInstant(body, "FreeEvent") {
			t.Fatalf("native stable ID accepted a forbidden positive high-half alias: coverage=%+v\n%s", coverage, body)
		}
	})

	t.Run("decoded uint32 order is physical-order independent", func(t *testing.T) {
		rows := []string{
			"INSERT INTO native_hook VALUES (1, 10, 0, 'AllocEvent', 100, 1, 1)",
			"INSERT INTO native_hook VALUES (-2147483648, 10, 0, 'FreeEvent', 90, 1, 1)",
			"INSERT INTO native_hook VALUES (-2, 10, 0, 'MmapEvent', 80, 1, 1)",
			"INSERT INTO native_hook VALUES (-1, 10, 0, 'MunmapEvent', 70, 1, 1)",
		}
		want := []string{"AllocEvent", "FreeEvent", "MmapEvent", "MunmapEvent"}
		var baseline string
		for _, reverse := range []bool{false, true} {
			ordered := append([]string(nil), rows...)
			if reverse {
				reverseTraceDBNativeHookB2Statements(ordered)
			}
			body, coverage, _ := exportTraceDBNativeHookB2Fixture(t,
				traceDBNativeHookB2Statements(
					[]string{"INSERT INTO thread_state VALUES (1, 0, 100, 1, 'Running', 501, 500)"}, ordered),
				traceDBLifecycleIndex{}, true)
			if coverage.RowsEmitted != 8 || coverage.Skipped != "" {
				t.Fatalf("signed stable row IDs were rejected or lost: coverage=%+v\n%s", coverage, body)
			}
			if got := traceDBNativeHookB2InstantNames(body); !reflect.DeepEqual(got, want) {
				t.Fatalf("decoded uint32 stable order=%v, want %v\n%s", got, want, body)
			}
			if baseline == "" {
				baseline = body
			} else if body != baseline {
				t.Fatalf("stable ID order depends on physical insertion order:\nforward=%s\nreverse=%s", baseline, body)
			}
		}
	})

	t.Run("negative duplicate cohort rejects without losing sibling", func(t *testing.T) {
		rows := []string{
			"INSERT INTO native_hook VALUES (-2, 10, 0, 'AllocEvent', 100, 1, 1)",
			"INSERT INTO native_hook VALUES (-2, 11, 0, 'FreeEvent', 90, 1, 1)",
			"INSERT INTO native_hook VALUES (-1, 12, 0, 'MmapEvent', 80, 1, 1)",
		}
		for _, reverse := range []bool{false, true} {
			ordered := append([]string(nil), rows...)
			if reverse {
				reverseTraceDBNativeHookB2Statements(ordered)
			}
			body, coverage, _ := exportTraceDBNativeHookB2Fixture(t,
				traceDBNativeHookB2Statements(
					[]string{"INSERT INTO thread_state VALUES (1, 0, 100, 1, 'Running', 501, 500)"}, ordered),
				traceDBLifecycleIndex{}, true)
			if coverage.RowsEmitted != 2 || coverage.Skipped != "duplicate_row_identity=2" ||
				!reflect.DeepEqual(traceDBNativeHookB2InstantNames(body), []string{"MmapEvent"}) {
				t.Fatalf("signed duplicate cohort or valid sibling changed with row order: coverage=%+v\n%s", coverage, body)
			}
		}
	})
}

func TestTraceDBNativeHookB2ThreadRenameIsDisplayOnly(t *testing.T) {
	threadRows := []string{
		"INSERT INTO thread VALUES (1, 501, 1, 'old-worker', 0, 0, 1)",
		"INSERT INTO thread VALUES (1, 501, 1, 'renamed-worker', 0, 0, 1)",
		"INSERT INTO thread VALUES (1, 501, 1, 'bad\nname', 0, 0, 1)",
	}
	var baseline string
	for _, reverse := range []bool{false, true} {
		ordered := append([]string(nil), threadRows...)
		if reverse {
			reverseTraceDBNativeHookB2Statements(ordered)
		}
		statements := []string{
			"CREATE TABLE trace_range (start_ts)",
			"INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid, pid, name)",
			"INSERT INTO process VALUES (1, 500, 'app')",
			"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		}
		statements = append(statements, ordered...)
		statements = append(statements,
			"CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
			"INSERT INTO thread_state VALUES (1, 0, 100, 1, 'Running', 501, 500)",
			"CREATE TABLE native_hook (id, start_ts, end_ts, event_type, all_heap_size, itid, ipid)",
			"INSERT INTO native_hook VALUES (1, 10, 0, 'AllocEvent', 100, 1, 1)",
		)
		body, coverage, _ := exportTraceDBNativeHookB2Fixture(t, statements, traceDBLifecycleIndex{}, true)
		if coverage.RowsEmitted != 2 || coverage.Skipped != "" || !strings.Contains(body, "renamed-worker") ||
			!traceDBNativeHookB2HasInstant(body, "AllocEvent") {
			t.Fatalf("display rename changed hard native-hook evidence: coverage=%+v\n%s", coverage, body)
		}
		if baseline == "" {
			baseline = body
		} else if body != baseline {
			t.Fatalf("display-only rename depends on physical order:\nforward=%s\nreverse=%s", baseline, body)
		}
	}
}

func TestTraceDBNativeHookB2RejectsKernelProcessOwner(t *testing.T) {
	statements := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 0, 'kernel')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 101, 1, 'kernel-worker', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
		"INSERT INTO thread_state VALUES (1, 0, 100, 1, 'Running', 101, 0)",
		"CREATE TABLE native_hook (id, start_ts, end_ts, event_type, all_heap_size, itid, ipid)",
		"INSERT INTO native_hook VALUES (1, 10, 0, 'AllocEvent', 100, 1, 1)",
	}
	body, coverage, _ := exportTraceDBNativeHookB2Fixture(t, statements, traceDBLifecycleIndex{}, true)
	if coverage.RowsEmitted != 0 || coverage.Skipped != "unresolved_owner_process=1" || traceDBNativeHookB2HasInstant(body, "AllocEvent") {
		t.Fatalf("PID0 owner borrowed the scheduler kernel exception: coverage=%+v\n%s", coverage, body)
	}
}

func traceDBNativeHookB2Statements(runningRows, hookRows []string) []string {
	statements := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 500, 'old-process')",
		"INSERT INTO process VALUES (2, 500, 'new-process')",
		"INSERT INTO process VALUES (3, 700, 'other-process')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 501, 1, 'old-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 501, 2, 'new-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (3, 701, 3, 'other-thread', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
	}
	statements = append(statements, runningRows...)
	statements = append(statements, "CREATE TABLE native_hook (id, start_ts, end_ts, event_type, all_heap_size, itid, ipid)")
	return append(statements, hookRows...)
}

func traceDBNativeHookB2CutLifecycle(threadCut, processCut bool) traceDBLifecycleIndex {
	lifecycle := traceDBLifecycleIndex{}
	boundary := traceDBLifecycleBoundary{TS: 100, NewITID: 2, NewIPID: 2}
	if threadCut {
		lifecycle.ByTID = map[int64]traceDBLifecycleLane{501: {Cuts: []traceDBLifecycleBoundary{boundary}}}
	}
	if processCut {
		lifecycle.ByPID = map[int64]traceDBLifecycleLane{500: {Cuts: []traceDBLifecycleBoundary{boundary}}}
	}
	return lifecycle
}

func exportTraceDBNativeHookB2Fixture(t *testing.T, statements []string, lifecycle traceDBLifecycleIndex,
	complete bool,
) (string, TraceDBCoverage, string) {
	t.Helper()
	ctx := context.Background()
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(ctx, path)
	if err != nil {
		t.Fatalf("open native B2 fixture: %v", err)
	}
	defer tdb.close()
	identities, _, err := tdb.loadThreadIndex(ctx)
	if err != nil {
		t.Fatalf("load native B2 identities: %v", err)
	}
	intervals, integrity, _, err := tdb.loadRunningIntervals(ctx, identities)
	if err != nil {
		t.Fatalf("load native B2 Running: %v", err)
	}
	authority := newTraceDBSchedulerAuthority(identities, traceDBLifecycleCollection{
		Lifecycle: lifecycle, CreationComplete: complete, TerminalComplete: complete, ActivityComplete: complete,
	})
	running := newTraceDBSchedulerRunningIndex(authority, intervals, integrity, nil)
	sink, err := newTraceDBRowSink(t.TempDir(), 4)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err := exportTraceDBNativeHook(ctx, tdb, sink, authority, running)
	if err != nil {
		t.Fatalf("export native B2 fixture: %v coverage=%+v", err, coverage)
	}
	outPath := filepath.Join(t.TempDir(), "native-hook-b2.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := sink.prepareAndWriteForTest(ctx, out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatalf("write native B2 fixture: %v", writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(bodyBytes), coverage, outPath
}

func traceDBNativeHookB2HasInstant(body, event string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "tracing_mark_write: I|") && strings.Contains(line, "|NativeHook:"+event) {
			return true
		}
	}
	return false
}

func assertTraceDBNativeHookB2InstantCPU(t *testing.T, body, event string, cpu int64) {
	t.Helper()
	wantCPU := fmt.Sprintf("[%03d]", cpu)
	wantEvent := "|NativeHook:" + event
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "tracing_mark_write: I|") && strings.Contains(line, wantEvent) {
			if !strings.Contains(line, wantCPU) {
				t.Fatalf("native event %s CPU line lacks %s: %q", event, wantCPU, line)
			}
			return
		}
	}
	t.Fatalf("missing native instant %s on CPU %d:\n%s", event, cpu, body)
}

func traceDBNativeHookB2Actions(t *testing.T, path string) map[string]int {
	t.Helper()
	index, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatalf("parse native B2 output: %v", err)
	}
	actions := map[string]int{}
	for _, event := range index.Events {
		if event.Type == tracequery.EventTraceMark {
			actions[event.SpanAction]++
		}
	}
	return actions
}

func traceDBNativeHookB2InstantNames(body string) []string {
	names := []string{}
	const marker = "|NativeHook:"
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, "tracing_mark_write: I|") {
			continue
		}
		position := strings.Index(line, marker)
		if position >= 0 {
			names = append(names, strings.TrimSpace(line[position+len(marker):]))
		}
	}
	return names
}

func reverseTraceDBNativeHookB2Statements(items []string) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}
