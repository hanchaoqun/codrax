package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestExportTraceDBSchedulerFamiliesRoundTripsThroughTraceQuery(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (100)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 200, 'proc')",
		"INSERT INTO process VALUES (2, 300, 'waker_proc')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 200, 1, 'MainApp', 100, 1, 1)",
		"INSERT INTO thread VALUES (2, 201, 1, 'WorkerThread', 100, 0, 1)",
		"INSERT INTO thread VALUES (3, 301, 2, 'Waker', 100, 1, 1)",
		"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, end_state TEXT, priority INT, itid INT)",
		"INSERT INTO sched_slice VALUES (1000000, 200000, 1, 'S', 42, 2)",
		"INSERT INTO sched_slice VALUES (1300000, 100000, 1, 'R', 20, 3)",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT, value REAL)",
		"INSERT INTO instant VALUES (900000, 'sched_wakeup', 2, 3, 'itid', NULL)",
		"CREATE TABLE raw (ts INT, name TEXT, cpu INT, itid INT)",
		"INSERT INTO raw VALUES (900000, 'sched_wakeup', 7, 2)",
		"CREATE TABLE data_dict (id INT, data TEXT)",
		"INSERT INTO data_dict VALUES (1, 'irq')",
		"INSERT INTO data_dict VALUES (2, 'irq_ret')",
		"INSERT INTO data_dict VALUES (3, 'handled')",
		"INSERT INTO data_dict VALUES (4, 'vec')",
		"CREATE TABLE args (argset INT, key INT, datatype INT, value INT)",
		"INSERT INTO args VALUES (10, 1, 0, 32)",
		"INSERT INTO args VALUES (10, 2, 1, 3)",
		"INSERT INTO args VALUES (20, 4, 0, 9)",
		"CREATE TABLE irq (ts INT, dur INT, callid INT, cat TEXT, name TEXT, argsetid INT)",
		"INSERT INTO irq VALUES (1500000, 10000, 4, 'irq', 'uart', 10)",
		"INSERT INTO irq VALUES (1600000, 20000, 6, 'softirq', 'RCU', 20)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"CREATE TABLE callstack (callid INT)",
		"CREATE TABLE syscall (itid INT)",
		"CREATE TABLE native_hook (itid INT)",
		"CREATE TABLE frame_slice (itid INT)",
	})

	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatalf("open trace db: %v", err)
	}
	defer tdb.close()
	sink, err := newTraceDBRowSink(t.TempDir(), 2)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := exportTraceDBSchedulerFamilies(context.Background(), tdb, sink)
	if err != nil {
		t.Fatalf("export scheduler families: %v", err)
	}
	assertCoverageEmitted(t, coverage, "metadata", "thread", 9)
	assertCoverageEmitted(t, coverage, "scheduler", "sched_slice", 2)
	assertCoverageEmitted(t, coverage, "scheduler", "instant", 1)
	assertCoverageEmitted(t, coverage, "irq", "irq", 4)

	outPath := filepath.Join(t.TempDir(), "out.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	stats, writeErr := sink.writeTo(context.Background(), out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatalf("write sorted rows: %v", writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if stats.RowsWritten != 16 || stats.SpillChunks == 0 {
		t.Fatalf("unexpected row sink stats: %+v", stats)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"task_rename: pid=201 oldcomm=WorkerThread newcomm=WorkerThread",
		"sched_wakeup: comm=WorkerThread pid=201 prio=42 target_cpu=001",
		"[007] ....",
		"0.000900: sched_wakeup",
		"sched_switch: prev_comm=WorkerThread prev_pid=201 prev_prio=42 prev_state=S ==> next_comm=Waker next_pid=301 next_prio=20",
		"irq_handler_entry: irq=32 name=uart",
		"irq_handler_exit: irq=32 ret=handled",
		"softirq_entry: vec=9 [action=RCU]",
		"softirq_exit: vec=9 [action=RCU]",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("systrace missing %q:\n%s", want, body)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery parse DB scheduler output: %v", err)
	}
	if len(idx.Events) < 7 {
		t.Fatalf("tracequery should parse scheduler/irq rows, got %+v", idx.Events)
	}
	var wakeups []tracequery.Event
	for _, ev := range idx.Events {
		if ev.Type == tracequery.EventSchedWakeup {
			wakeups = append(wakeups, ev)
		}
	}
	if len(wakeups) != 1 || wakeups[0].WakeePID != 201 || wakeups[0].TargetCPU != 1 {
		t.Fatalf("wakeup metadata lost after round trip: %+v", wakeups)
	}
}

func TestExportTraceDBSchedulerFamiliesRecordsMissingTables(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (100)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatalf("open trace db: %v", err)
	}
	defer tdb.close()
	sink, err := newTraceDBRowSink(t.TempDir(), 100)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := exportTraceDBSchedulerFamilies(context.Background(), tdb, sink)
	if err != nil {
		t.Fatalf("missing tables should be coverage, not hard failure: %v", err)
	}
	if !coverageHasSkipped(coverage, "scheduler", "sched_slice", "missing table") ||
		!coverageHasSkipped(coverage, "scheduler", "instant", "missing table") ||
		!coverageHasSkipped(coverage, "irq", "irq", "missing table") {
		t.Fatalf("missing table coverage not recorded: %+v", coverage)
	}
}

func assertCoverageEmitted(t *testing.T, coverage []TraceDBCoverage, family, table string, minRows int) {
	t.Helper()
	for _, item := range coverage {
		if item.Family == family && item.Table == table {
			if item.RowsEmitted < minRows {
				t.Fatalf("coverage %s/%s emitted %d, want >=%d: %+v", family, table, item.RowsEmitted, minRows, item)
			}
			return
		}
	}
	t.Fatalf("coverage %s/%s not found: %+v", family, table, coverage)
}

func coverageHasSkipped(coverage []TraceDBCoverage, family, table, skipped string) bool {
	for _, item := range coverage {
		if item.Family == family && item.Table == table && strings.Contains(item.Skipped, skipped) {
			return true
		}
	}
	return false
}
