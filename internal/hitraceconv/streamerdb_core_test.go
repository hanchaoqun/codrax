package hitraceconv

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLiteReadOnlyDSNUsesAuthorityFreeFileURI(t *testing.T) {
	cases := []struct {
		name       string
		path       string
		wantPrefix string
		wantPath   string
	}{
		{
			name:       "posix relative",
			path:       "relative trace.db",
			wantPrefix: "file:///relative%20trace.db?",
			wantPath:   "/relative trace.db",
		},
		{
			name:       "posix absolute",
			path:       "/tmp/trace db.sqlite",
			wantPrefix: "file:///tmp/trace%20db.sqlite?",
			wantPath:   "/tmp/trace db.sqlite",
		},
		{
			name:       "windows drive absolute",
			path:       `D:\opt\codrax-main\trace db.sqlite`,
			wantPrefix: "file:///D:/opt/codrax-main/trace%20db.sqlite?",
			wantPath:   "/D:/opt/codrax-main/trace db.sqlite",
		},
		{
			name:       "windows drive relative",
			path:       `D:trace db.sqlite`,
			wantPrefix: "file:///D:trace%20db.sqlite?",
			wantPath:   "/D:trace db.sqlite",
		},
		{
			name:       "windows unc",
			path:       `\\server\share\trace db.sqlite`,
			wantPrefix: "file:////server/share/trace%20db.sqlite?",
			wantPath:   "//server/share/trace db.sqlite",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dsn := sqliteReadOnlyDSNFromURIPath(tc.path)
			if !strings.HasPrefix(dsn, tc.wantPrefix) {
				t.Fatalf("dsn prefix mismatch:\ngot  %q\nwant prefix %q", dsn, tc.wantPrefix)
			}
			parsed, err := url.Parse(dsn)
			if err != nil {
				t.Fatalf("parse dsn %q: %v", dsn, err)
			}
			if parsed.Scheme != "file" || parsed.Host != "" || parsed.Path != tc.wantPath {
				t.Fatalf("dsn should be a file URI with empty authority: dsn=%q scheme=%q host=%q path=%q", dsn, parsed.Scheme, parsed.Host, parsed.Path)
			}
			if got := parsed.Query().Get("mode"); got != "ro" {
				t.Fatalf("dsn mode=%q, want ro in %q", got, dsn)
			}
		})
	}
}

func TestOpenTraceDBRelativePathWithSpacesReadOnly(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Mkdir("db dir", 0o755); err != nil {
		t.Fatal(err)
	}
	relPath := filepath.Join("db dir", "trace data.sqlite")
	db, err := sql.Open("sqlite", relPath)
	if err != nil {
		t.Fatalf("open sqlite fixture: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE trace_range (start_ts INT)"); err != nil {
		_ = db.Close()
		t.Fatalf("create trace_range: %v", err)
	}
	if _, err := db.Exec("INSERT INTO trace_range VALUES (123456)"); err != nil {
		_ = db.Close()
		t.Fatalf("insert trace_range: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite fixture: %v", err)
	}

	tdb, err := openTraceDB(context.Background(), relPath)
	if err != nil {
		t.Fatalf("open relative trace db read-only: %v", err)
	}
	defer tdb.close()
	start, coverage, err := tdb.traceStart(context.Background())
	if err != nil {
		t.Fatalf("read trace start: %v", err)
	}
	if start != 123456 || !coverage.Found {
		t.Fatalf("trace start mismatch start=%d coverage=%+v", start, coverage)
	}
}

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
	if coverage.ElapsedUS <= 0 {
		t.Fatalf("coverage should expose elapsed timing: %+v", coverage)
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
	if missing.ElapsedUS <= 0 {
		t.Fatalf("missing table coverage should expose elapsed timing: %+v", missing)
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
	if len(coverage) != 3 || index.TraceStart != 100 || !index.TraceStartKnown || index.ByITID[10].TID != 201 || index.Processes[1].PID != 200 {
		t.Fatalf("thread index mismatch coverage=%+v index=%+v", coverage, index)
	}
	argsets, coverage, err := tdb.loadArgsets(context.Background())
	if err != nil {
		t.Fatalf("load argsets: %v", err)
	}
	if len(coverage) != 3 || coverage[2].Table != "data_type" || coverage[2].Found ||
		argsets.Sets[7]["irq"].Text != "32" || argsets.Sets[7]["irq_ret"].Text != "handled" {
		t.Fatalf("argsets mismatch coverage=%+v argsets=%+v", coverage, argsets)
	}
	rawWakeups, rawCoverage, err := tdb.loadRawWakeups(context.Background())
	if err != nil {
		t.Fatalf("load raw wakeups: %v", err)
	}
	if !rawCoverage.Found || len(rawWakeups) != 1 || rawWakeups[0].RowID == 0 ||
		rawWakeups[0].TS != 1000 || rawWakeups[0].Name != "sched_wakeup" ||
		rawWakeups[0].TargetCPU != 6 || rawWakeups[0].ITID != 10 {
		t.Fatalf("typed raw wakeup mismatch coverage=%+v wakeups=%+v", rawCoverage, rawWakeups)
	}
	starts, schedCoverage, err := tdb.loadSchedStarts(context.Background(), traceDBTestCompleteSchedulerAuthority(index))
	if err != nil {
		t.Fatalf("load sched starts: %v", err)
	}
	if !schedCoverage.Found {
		t.Fatalf("sched coverage missing: %+v", schedCoverage)
	}
	if cpu, prio, known := traceDBNextSchedMeta(starts, 10, 1000); !known || cpu != 5 || prio != 42 {
		t.Fatalf("next sched meta mismatch cpu=%d prio=%d known=%t starts=%+v", cpu, prio, known, starts)
	}
	intervals, _, stateCoverage, err := tdb.loadRunningIntervals(context.Background(), index)
	if err != nil {
		t.Fatalf("load running intervals: %v", err)
	}
	cpu, cpuKnown := traceDBKnownCPUAt(intervals, 10, 950)
	_, outsideKnown := traceDBKnownCPUAt(intervals, 10, 1300)
	if !stateCoverage.Found || !cpuKnown || cpu != 3 || outsideKnown {
		t.Fatalf("running interval mismatch coverage=%+v intervals=%+v", stateCoverage, intervals)
	}
	if stateCoverage.RowsRead != 1 || stateCoverage.RowsEmitted != 1 {
		t.Fatalf("running interval coverage should distinguish table rows from emitted windows: %+v", stateCoverage)
	}
	active, activeCoverage, err := tdb.loadActiveThreadIDs(context.Background(), index)
	if err != nil {
		t.Fatalf("load active threads: %v", err)
	}
	if !active[10] || len(activeCoverage) != 6 {
		t.Fatalf("active thread mismatch coverage=%+v active=%+v", activeCoverage, active)
	}
}

func TestTraceDBArgsetsAuditOfficialDataTypeRegistry(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 'irq')",
		"INSERT INTO data_dict VALUES (2, 'irq_ret')",
		"INSERT INTO data_dict VALUES (3, 'handled')",
		"INSERT INTO data_dict VALUES (4, 'is_reply')",
		"CREATE TABLE data_type (id, typeId, desc)",
		"INSERT INTO data_type VALUES (0, 0, 'int32_t')",
		"INSERT INTO data_type VALUES (1, 1, 'string')",
		"INSERT INTO data_type VALUES (2, 2, 'double')",
		"INSERT INTO data_type VALUES (3, 3, 'boolean')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (7, 1, 0, 32)",
		"INSERT INTO args VALUES (7, 2, 1, 3)",
		"INSERT INTO args VALUES (7, 4, 3, 1)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	argsets, coverage, err := tdb.loadArgsets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if argsets.Sets[7]["irq"].Text != "32" || argsets.Sets[7]["irq_ret"].Text != "handled" ||
		!argsets.InvalidKeys[7]["is_reply"] {
		t.Fatalf("closed data type admission mismatch: argsets=%+v", argsets)
	}
	dataTypes := requireTraceDBCoverage(t, coverage, "resolver", "data_type")
	if dataTypes.RowsRead != 4 || dataTypes.RowsEmitted != 4 || dataTypes.Skipped != "" {
		t.Fatalf("official data type registry coverage mismatch: %+v", dataTypes)
	}
}

func TestTraceDBArgsetsLocalizeConflictingDataTypeRegistryID(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 'integer_key')",
		"INSERT INTO data_dict VALUES (2, 'string_key')",
		"INSERT INTO data_dict VALUES (3, 'value')",
		"CREATE TABLE data_type (typeId, desc)",
		"INSERT INTO data_type VALUES (0, 'int32_t')",
		"INSERT INTO data_type VALUES (1, 'string')",
		"INSERT INTO data_type VALUES (1, 'bytes')",
		"INSERT INTO data_type VALUES (2, 'double')",
		"INSERT INTO data_type VALUES (3, 'boolean')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (7, 1, 0, 9)",
		"INSERT INTO args VALUES (8, 2, 1, 3)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	argsets, coverage, err := tdb.loadArgsets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if argsets.Sets[7]["integer_key"].Text != "9" || !argsets.InvalidKeys[8]["string_key"] {
		t.Fatalf("conflicting type ID was not localized: %+v", argsets)
	}
	dataTypes := requireTraceDBCoverage(t, coverage, "resolver", "data_type")
	for _, want := range []string{"duplicate_or_conflicting_type=1", "invalid_closed_type=1"} {
		if !strings.Contains(dataTypes.Skipped, want) {
			t.Fatalf("data type conflict ledger missing %q: %+v", want, dataTypes)
		}
	}
	if dataTypes.RowsEmitted != 3 {
		t.Fatalf("unrelated data type IDs were suppressed: %+v", dataTypes)
	}
}

func TestTraceDBThreadIndexRejectsDivergentCurrentAliases(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (id INT, ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (31, 7, 500, 'demo')",
		"INSERT INTO process VALUES (8, 8, 600, 'control')",
		"CREATE TABLE thread (id INT, itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (41, 9, 501, 7, 'ambiguous-worker', 0, 0, 1)",
		"INSERT INTO thread VALUES (10, 10, 601, 8, 'control-worker', 0, 0, 1)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()

	index, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatalf("load current identity index: %v", err)
	}
	if _, ok := index.ByITID[9]; ok || !index.AmbiguousITID[9] {
		t.Fatalf("divergent thread.id/itid survived current-profile audit: %+v", index)
	}
	if _, ok := index.Processes[7]; ok || !index.AmbiguousIPID[7] {
		t.Fatalf("divergent process.id/ipid survived current-profile audit: %+v", index)
	}
	if control, ok := index.ByITID[10]; !ok || control.TID != 601 || control.IPID != 8 || index.Processes[8].PID != 600 {
		t.Fatalf("valid current alias sibling was lost: thread=%+v process=%+v index=%+v", control, index.Processes[8], index)
	}
}

func TestTraceDBCoreThreadStateRunningCoverage(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (10, 900, 300, 3, 'Running')",
		"INSERT INTO thread_state VALUES (10, 1300, 300, 4, 'running')",
		"INSERT INTO thread_state VALUES (10, 1700, 0, 5, 'Running')",
		"INSERT INTO thread_state VALUES (10, 1900, 300, 6, 'Runnable')",
		"INSERT INTO thread_state VALUES (10, 2300, 300, NULL, 'Running')",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatalf("open trace db: %v", err)
	}
	defer tdb.close()

	intervals, integrity, coverage, err := tdb.loadRunningIntervals(context.Background(), newTraceDBThreadIndex(0, true))
	if err != nil {
		t.Fatalf("load running intervals: %v", err)
	}
	if coverage.RowsRead != 5 || coverage.RowsEmitted != 1 {
		t.Fatalf("thread_state coverage mismatch: %+v", coverage)
	}
	if !integrity.TaintedITIDs[10] {
		t.Fatalf("malformed Running rows must taint their itid: %+v", integrity)
	}
	if got, known := traceDBKnownCPUAt(intervals, 10, 950); !known || got != 3 {
		t.Fatalf("CPU at first running window = %d known=%t, want 3/true", got, known)
	}
	if got, known := traceDBKnownCPUAt(intervals, 10, 1350); known {
		t.Fatalf("case-drifted Running token must not become a CPU witness, got %d", got)
	}
	if got, known := traceDBKnownCPUAt(intervals, 10, 1750); known {
		t.Fatalf("zero-duration row must not become a running window, got CPU %d", got)
	}
	if got, known := traceDBKnownCPUAt(intervals, 10, 1950); known {
		t.Fatalf("Runnable row must not become a Running window, got CPU %d", got)
	}
	if !traceDBThreadStateIsRunning("Running") || traceDBThreadStateIsRunning(" Running ") ||
		traceDBThreadStateIsRunning("running") || traceDBThreadStateIsRunning("R") {
		t.Fatal("thread_state CPU witness must accept only the exact Running token")
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
		ElapsedUS:      123,
		Skipped:        "missing required columns: ipid",
	}, {
		Family: "capture_completeness", Table: "stat", Role: "capture_completeness", Found: true, RowsRead: 5,
		CaptureCompleteness: &TraceCaptureCompleteness{
			State: "parser_self_audit_degraded", RowsAccepted: 5, Received: 10, DataLost: 2,
			ErrorIssues: 2, NonzeroIssueRows: 1,
			Issues: []TraceCaptureCompletenessIssue{{
				EventName: "sched_switch", StatType: "data_lost", Count: 2, Source: "trace", Severity: "error",
			}},
		},
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
		Coverage       []TraceDBCoverage     `json:"trace_db_coverage"`
		TraceToolGates []TraceToolGateStatus `json:"trace_tool_gates"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse bundle: %v\n%s", err, body)
	}
	if len(parsed.Coverage) != 2 || parsed.Coverage[0].Family != "resolver" ||
		parsed.Coverage[0].ElapsedUS != 123 ||
		!containsExact(parsed.Coverage[0].ColumnsMissing, "ipid") ||
		parsed.Coverage[1].CaptureCompleteness == nil ||
		parsed.Coverage[1].CaptureCompleteness.State != "parser_self_audit_degraded" ||
		parsed.Coverage[1].CaptureCompleteness.DataLost != 2 ||
		len(parsed.Coverage[1].CaptureCompleteness.Issues) != 1 {
		t.Fatalf("trace db coverage not serialized: %+v\n%s", parsed.Coverage, body)
	}
	if len(parsed.TraceToolGates) != 1 ||
		parsed.TraceToolGates[0].Name != traceToolGateNameSysBinaryParity ||
		parsed.TraceToolGates[0].State == "" ||
		parsed.TraceToolGates[0].RequiredEvidence == "" ||
		!containsExact(parsed.TraceToolGates[0].Evidence, traceToolGateSysParitySyntheticEvidence) {
		t.Fatalf("trace tool gate not serialized: %+v\n%s", parsed.TraceToolGates, body)
	}
	for _, want := range []string{`"trace_tool_gates"`, `"fixture_manifest_count"`, `"required_evidence"`, `"capture_completeness"`, `"rows_accepted"`, `"nonzero_issue_rows"`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("tracebundle gate should use stable snake_case field %q:\n%s", want, body)
		}
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
