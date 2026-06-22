package hitraceconv

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTraceDBCoreInspectsCoverageAndSchemaDrift(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (100)",
		"CREATE TABLE thread (itid INT, tid INT, name TEXT)",
		"INSERT INTO thread VALUES (1, 20, 'main')",
	})

	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatalf("open trace db: %v", err)
	}
	defer tdb.close()

	if ok, err := tdb.tableExists(context.Background(), "thread"); err != nil || !ok {
		t.Fatalf("tableExists(thread)=%v err=%v", ok, err)
	}
	if ok, err := tdb.columnExists(context.Background(), "thread", "tid"); err != nil || !ok {
		t.Fatalf("columnExists(thread.tid)=%v err=%v", ok, err)
	}
	coverage, err := tdb.inspectCoverage(context.Background(), "resolver", "thread", []string{"itid", "tid", "ipid", "name"})
	if err != nil {
		t.Fatalf("inspect coverage: %v", err)
	}
	if !coverage.Found || coverage.RowsRead != 1 || coverage.Skipped == "" {
		t.Fatalf("coverage should expose missing columns and row count: %+v", coverage)
	}
	if !containsExact(coverage.ColumnsPresent, "itid") || !containsExact(coverage.ColumnsMissing, "ipid") {
		t.Fatalf("coverage columns mismatch: %+v", coverage)
	}
	missing, err := tdb.inspectCoverage(context.Background(), "resolver", "missing_table", []string{"id"})
	if err != nil {
		t.Fatalf("inspect missing table: %v", err)
	}
	if missing.Found || missing.Skipped != "missing table" {
		t.Fatalf("missing table coverage mismatch: %+v", missing)
	}
}

func TestTraceDBCoreLoadsResolvers(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (100)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 200, 'proc')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (10, 201, 1, 'worker', 100, 0, 2)",
		"CREATE TABLE data_dict (id INT, data TEXT)",
		"INSERT INTO data_dict VALUES (1, 'irq')",
		"INSERT INTO data_dict VALUES (2, 'irq_ret')",
		"INSERT INTO data_dict VALUES (3, 'handled')",
		"CREATE TABLE args (argset INT, key INT, datatype INT, value INT)",
		"INSERT INTO args VALUES (7, 1, 0, 32)",
		"INSERT INTO args VALUES (7, 2, 1, 3)",
		"CREATE TABLE raw (ts INT, name TEXT, cpu INT, itid INT)",
		"INSERT INTO raw VALUES (1000, 'sched_wakeup', 6, 10)",
		"CREATE TABLE sched_slice (itid INT, ts INT, cpu INT, priority INT)",
		"INSERT INTO sched_slice VALUES (10, 1200, 5, 42)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (10, 900, 300, 3, 'Running')",
		"CREATE TABLE callstack (callid INT)",
		"INSERT INTO callstack VALUES (10)",
		"CREATE TABLE syscall (itid INT)",
		"CREATE TABLE native_hook (itid INT)",
		"CREATE TABLE frame_slice (itid INT)",
	})

	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatalf("open trace db: %v", err)
	}
	defer tdb.close()

	index, coverage, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatalf("load thread index: %v", err)
	}
	if len(coverage) != 3 || index.TraceStart != 100 || index.ByITID[10].TID != 201 || index.Processes[1].PID != 200 {
		t.Fatalf("thread index mismatch coverage=%+v index=%+v", coverage, index)
	}
	argsets, coverage, err := tdb.loadArgsets(context.Background())
	if err != nil {
		t.Fatalf("load argsets: %v", err)
	}
	if len(coverage) != 2 || argsets[7]["irq"].Text != "32" || argsets[7]["irq_ret"].Text != "handled" {
		t.Fatalf("argsets mismatch coverage=%+v argsets=%+v", coverage, argsets)
	}
	rawCPUs, rawCoverage, err := tdb.loadRawEventCPUs(context.Background())
	if err != nil {
		t.Fatalf("load raw cpus: %v", err)
	}
	if !rawCoverage.Found || rawCPUs[traceDBInstantKey{TS: 1000, Name: "sched_wakeup"}] != 6 {
		t.Fatalf("raw cpu mismatch coverage=%+v cpus=%+v", rawCoverage, rawCPUs)
	}
	starts, schedCoverage, err := tdb.loadSchedStarts(context.Background())
	if err != nil {
		t.Fatalf("load sched starts: %v", err)
	}
	if !schedCoverage.Found {
		t.Fatalf("sched coverage missing: %+v", schedCoverage)
	}
	if cpu, prio := traceDBNextSchedMeta(starts, 10, 1000, 0, 120); cpu != 5 || prio != 42 {
		t.Fatalf("next sched meta mismatch cpu=%d prio=%d starts=%+v", cpu, prio, starts)
	}
	intervals, stateCoverage, err := tdb.loadRunningIntervals(context.Background())
	if err != nil {
		t.Fatalf("load running intervals: %v", err)
	}
	if !stateCoverage.Found || traceDBCPUAt(intervals, 10, 950, 0) != 3 || traceDBCPUAt(intervals, 10, 1300, 0) != 0 {
		t.Fatalf("running interval mismatch coverage=%+v intervals=%+v", stateCoverage, intervals)
	}
	active, activeCoverage, err := tdb.loadActiveThreadIDs(context.Background())
	if err != nil {
		t.Fatalf("load active threads: %v", err)
	}
	if !active[10] || len(activeCoverage) != 6 {
		t.Fatalf("active thread mismatch coverage=%+v active=%+v", activeCoverage, active)
	}
}

func TestTraceBundleIncludesTraceDBCoverage(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.htrace")
	coverage := []TraceDBCoverage{{
		Family:         "resolver",
		Table:          "thread",
		Found:          true,
		ColumnsPresent: []string{"itid", "tid"},
		ColumnsMissing: []string{"ipid"},
		RowsRead:       2,
		Skipped:        "missing required columns: ipid",
	}}
	artifact, err := writeTraceBundleWithCoverage(input, "", []Artifact{{
		Type:      ArtifactTraceDB,
		Path:      filepath.Join(dir, "trace.db"),
		Converter: traceStreamerConverter,
	}}, nil, nil, nil, coverage)
	if err != nil {
		t.Fatalf("write trace bundle: %v", err)
	}
	body, err := os.ReadFile(artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Coverage []TraceDBCoverage `json:"trace_db_coverage"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse bundle: %v\n%s", err, body)
	}
	if len(parsed.Coverage) != 1 || parsed.Coverage[0].Family != "resolver" ||
		!containsExact(parsed.Coverage[0].ColumnsMissing, "ipid") {
		t.Fatalf("trace db coverage not serialized: %+v\n%s", parsed.Coverage, body)
	}
}

func createTraceDBFixture(t *testing.T, statements []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trace.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite fixture: %v", err)
	}
	defer db.Close()
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}
	return path
}

func containsExact(items []string, want string) bool {
	for _, item := range items {
		if strings.TrimSpace(item) == want {
			return true
		}
	}
	return false
}
