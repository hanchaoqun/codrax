package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTraceDBIdentityPoisonNeverGlobalizesThreadOrProcessScopedRows(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (id, ipid, pid, name)",
		"INSERT INTO process VALUES (31, 7, 500, 'ambiguous-process')",
		"INSERT INTO process VALUES (8, 8, 600, 'control-process')",
		"CREATE TABLE thread (id, itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (9, 9, 501, 7, 'canonical-owner-interpretation', 0, 0, 1)",
		"INSERT INTO thread VALUES (11, 11, 511, 31, 'source-owner-interpretation', 0, 0, 1)",
		"INSERT INTO thread VALUES (10, 10, 601, 8, 'control-thread', 0, 0, 1)",
		"CREATE TABLE syscall (ts, dur, syscall_number, itid)",
		"INSERT INTO syscall VALUES (3000, 1000, 2, 10)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (10, 0, 5000, 3, 'Running')",
		"CREATE TABLE callstack (id, ts, dur)",
		"INSERT INTO callstack VALUES (100, 5000, 0)",
		"INSERT INTO callstack VALUES (101, 6000, 1000)",
		"INSERT INTO callstack VALUES (102, 8000, 0)",
		"INSERT INTO callstack VALUES (103, 9000, 1000)",
		"INSERT INTO callstack VALUES (104, 11000, 0)",
		"INSERT INTO callstack VALUES (105, 12000, 1000)",
		"CREATE TABLE task_pool (task_id, allocation_task_row, execute_task_row, allocation_itid, execute_itid)",
		"INSERT INTO task_pool VALUES (91, 100, 101, 11, 10)",
		"INSERT INTO task_pool VALUES (92, 102, 103, 10, 9)",
		"INSERT INTO task_pool VALUES (93, 104, 105, 10, 10)",
		"INSERT INTO task_pool VALUES (94, 104, 105, CAST(10 AS TEXT), 10)",
		"CREATE TABLE app_startup (start_time, end_time, start_name, ipid)",
		"INSERT INTO app_startup VALUES (14000, 15000, 1, 7)",
		"INSERT INTO app_startup VALUES (16000, 17000, 1, 31)",
		"INSERT INTO app_startup VALUES (18000, 19000, 1, 8)",
		"INSERT INTO app_startup VALUES (19500, 19700, 1, CAST(8 AS TEXT))",
		"CREATE TABLE static_initalize (start_time, end_time, so_name, ipid, tid)",
		"INSERT INTO static_initalize VALUES (20000, 21000, 'bad-canonical.so', 7, 501)",
		"INSERT INTO static_initalize VALUES (22000, 23000, 'bad-source.so', 31, 511)",
		"INSERT INTO static_initalize VALUES (24000, 25000, 'good.so', 8, 601)",
		"INSERT INTO static_initalize VALUES (25100, 25200, 'text-ipid.so', CAST(8 AS TEXT), 601)",
		"INSERT INTO static_initalize VALUES (25300, 25400, 'text-tid.so', 8, CAST(601 AS TEXT))",
		"CREATE TABLE process_measure_filter (id, name, ipid)",
		"INSERT INTO process_measure_filter VALUES (1, 'bad_canonical_counter', 7)",
		"INSERT INTO process_measure_filter VALUES (2, 'bad_source_counter', 31)",
		"INSERT INTO process_measure_filter VALUES (3, 'good_counter', 8)",
		"INSERT INTO process_measure_filter VALUES (4, 'text_ipid_counter', CAST(8 AS TEXT))",
		"CREATE TABLE process_measure (ts, value, filter_id)",
		"INSERT INTO process_measure VALUES (26000, 1, 1)",
		"INSERT INTO process_measure VALUES (27000, 2, 2)",
		"INSERT INTO process_measure VALUES (28000, 3, 3)",
		"INSERT INTO process_measure VALUES (29000, 4, 4)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	index, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	intervals, integrity, _, err := tdb.loadRunningIntervals(context.Background(), index)
	if err != nil {
		t.Fatal(err)
	}
	authority := newTraceDBSchedulerAuthority(index, traceDBLifecycleCollection{
		CreationComplete: true, TerminalComplete: true, ActivityComplete: true,
	})
	running := newTraceDBSchedulerRunningIndex(authority, intervals, integrity, nil)

	syscallCoverage, err := exportTraceDBSyscall(context.Background(), tdb, sink, authority, running, syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	taskCoverage, err := exportTraceDBTaskPool(context.Background(), tdb, sink, index, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	startupCoverage, err := exportTraceDBAppStartup(context.Background(), tdb, sink, syncSpans, index, map[int64]string{1: "cold"})
	if err != nil {
		t.Fatal(err)
	}
	staticCoverage, err := exportTraceDBStaticInitialize(context.Background(), tdb, sink, syncSpans, index)
	if err != nil {
		t.Fatal(err)
	}
	measureCoverage, err := exportTraceDBProcessMeasures(context.Background(), tdb, sink, index, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	items, _, _ := finalizeTraceDBTestSyncSpans(t, sink, syncSpans, []TraceDBCoverage{
		syscallCoverage, taskCoverage, startupCoverage, staticCoverage, measureCoverage,
	})
	syscallCoverage, taskCoverage, startupCoverage, staticCoverage, measureCoverage =
		items[0], items[1], items[2], items[3], items[4]

	if syscallCoverage.RowsEmitted != 2 || syscallCoverage.Skipped != "" {
		t.Fatalf("syscall identity fail-close mismatch: %+v", syscallCoverage)
	}
	if taskCoverage.RowsEmitted != 2 || !strings.Contains(taskCoverage.Skipped, "unresolved_allocation_identity=1") ||
		!strings.Contains(taskCoverage.Skipped, "unresolved_execute_identity=1") ||
		!strings.Contains(taskCoverage.Skipped, "invalid_allocation_itid=1") {
		t.Fatalf("TaskPool identity/atomicity fail-close mismatch: %+v", taskCoverage)
	}
	if startupCoverage.RowsEmitted != 2 || !strings.Contains(startupCoverage.Skipped, "unresolved_owner_process=2") ||
		!strings.Contains(startupCoverage.Skipped, "invalid_owner_ipid=1") {
		t.Fatalf("AppStartup identity fail-close mismatch: %+v", startupCoverage)
	}
	if staticCoverage.RowsEmitted != 2 || !strings.Contains(staticCoverage.Skipped, "unresolved_owner_process=2") ||
		!strings.Contains(staticCoverage.Skipped, "invalid_owner_ipid=1") ||
		!strings.Contains(staticCoverage.Skipped, "invalid_emitter_tid=1") {
		t.Fatalf("static-init identity fail-close mismatch: %+v", staticCoverage)
	}
	if measureCoverage.RowsEmitted != 1 || !strings.Contains(measureCoverage.Skipped, "unresolved_owner_process=2") ||
		!strings.Contains(measureCoverage.Skipped, "invalid_owner_ipid=1") {
		t.Fatalf("process-measure identity fail-close mismatch: %+v", measureCoverage)
	}

	outPath := filepath.Join(t.TempDir(), "identity-consumers.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := sink.writeTo(context.Background(), out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{"sys_2", "TaskPool-93", "AppStartup:cold", "good.so", "good_counter"} {
		if !strings.Contains(body, want) {
			t.Fatalf("valid identity sibling %q missing:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{
		"sys_1", "sys_3", "TaskPool-91", "TaskPool-92", "TaskPool-94", "bad-canonical.so", "bad-source.so",
		"text-ipid.so", "text-tid.so", "bad_canonical_counter", "bad_source_counter", "text_ipid_counter",
		"(    0)", "B|0|", "S|0|", "C|0|",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("poisoned identity was globalized as %q:\n%s", forbidden, body)
		}
	}
}
