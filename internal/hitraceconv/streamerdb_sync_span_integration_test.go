package hitraceconv

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func traceDBSyncSpanIntegrationBaseStatements() []string {
	return []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'app')",
		"INSERT INTO process VALUES (2, 200, 'other')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 100, 1, 'app-main', 0, 1, 1)",
		"INSERT INTO thread VALUES (2, 200, 2, 'other-main', 0, 1, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (1, 0, 9000000, 1, 'Running')",
		"INSERT INTO thread_state VALUES (2, 0, 9000000, 2, 'Running')",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT)",
		"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, itid INT, end_state TEXT, priority INT)",
		"CREATE TABLE callstack (id INT, ts INT, dur INT, itid INT, callid INT, name TEXT, flag TEXT, cookie INT, chainId TEXT, depth INT)",
		"CREATE TABLE syscall (ts INT, dur INT, syscall_number INT, itid INT)",
		"CREATE TABLE frame_slice (id INT, type INT, ts INT, itid INT)",
		"CREATE TABLE native_hook (id INT, start_ts INT, end_ts INT, event_type TEXT, all_heap_size INT, itid INT, ipid INT)",
		"CREATE TABLE app_startup (start_time INT, end_time INT, start_name INT, ipid INT)",
		"CREATE TABLE static_initalize (start_time INT, end_time INT, so_name TEXT, ipid INT, tid INT)",
		"CREATE TABLE data_dict (id INT, data TEXT)",
		"INSERT INTO data_dict VALUES (5, 'coldStart')",
	}
}

func exportTraceDBSyncSpanIntegrationFixture(t *testing.T, suffix string, rows ...string) (string, traceDBSystraceExport) {
	t.Helper()
	statements := append(traceDBSyncSpanIntegrationBaseStatements(), rows...)
	path := createTraceDBFixture(t, statements)
	outPath := filepath.Join(t.TempDir(), suffix+".systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export sync-span integration fixture %s: %v\ncoverage=%+v", suffix, err, result.Coverage)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read sync-span integration fixture %s: %v", suffix, err)
	}
	return string(body), result
}

func TestTraceDBSyncSpanCrossProducerCrossingSuppressesWholePhysicalLane(t *testing.T) {
	body, result := exportTraceDBSyncSpanIntegrationFixture(t, "cross-producer-crossing",
		"INSERT INTO callstack VALUES (1, 1000000, 2000000, 1, NULL, 'cross-callstack', '', NULL, NULL, 0)",
		"INSERT INTO callstack VALUES (2, 6000000, 0, 1, NULL, 'async-survives', 'S', 77, NULL, 0)",
		"INSERT INTO callstack VALUES (3, 6500000, 0, 1, NULL, 'async-survives', 'C', 77, NULL, 0)",
		"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 2000000, 2000000, 1, 1)",
		"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (2, 1000000, 400000, 9, 2)",
		"INSERT INTO app_startup(rowid, start_time, end_time, start_name, ipid) VALUES (1, 5000000, 5500000, 5, 1)",
		"INSERT INTO static_initalize(rowid, start_time, end_time, so_name, ipid, tid) VALUES (1, 5600000, 5700000, 'libbad.so', 1, 100)",
		"INSERT INTO native_hook VALUES (1, 7000000, 0, 'malloc', 8192, 1, 1)",
	)

	for _, forbidden := range []string{
		"tracing_mark_write: B|100|app",
		"tracing_mark_write: B|100|cross-callstack",
		"tracing_mark_write: B|100|sys_1",
		"tracing_mark_write: B|100|AppStartup:coldStart",
		"tracing_mark_write: B|100|SoInit:libbad.so",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("crossing physical lane leaked %q:\n%s", forbidden, body)
		}
	}
	for _, want := range []string{
		"tracing_mark_write: B|200|other",
		"tracing_mark_write: B|200|sys_9",
		"tracing_mark_write: S|100|async-survives|77",
		"tracing_mark_write: F|100|async-survives|77",
		"tracing_mark_write: I|100|NativeHook:AllocEvent",
		"tracing_mark_write: C|100|HeapSize|8192",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("unrelated lane or non-B/E family lost %q:\n%s", want, body)
		}
	}

	authorityCoverage := requireTraceDBCoverage(t, result.Coverage, "integrity", "sync_span_authority")
	if authorityCoverage.RowsEmitted != 4 ||
		!strings.Contains(authorityCoverage.Skipped, "crossing_lanes=1") ||
		!strings.Contains(authorityCoverage.Skipped, "suppressed_spans=5") {
		t.Fatalf("cross-producer lane audit coverage mismatch: %+v", authorityCoverage)
	}
	checks := []struct {
		family, table string
		rows          int
		suppressed    bool
	}{
		{"metadata", "thread", 4, true},
		{"slice", "callstack", 2, true},
		{"slice", "syscall", 2, true},
		{"slice", "app_startup", 0, true},
		{"slice", "static_initalize", 0, true},
		{"resource", "native_hook", 2, false},
	}
	for _, check := range checks {
		coverage := requireTraceDBCoverage(t, result.Coverage, check.family, check.table)
		if coverage.RowsEmitted != check.rows {
			t.Fatalf("%s/%s RowsEmitted=%d want %d: %+v", check.family, check.table, coverage.RowsEmitted, check.rows, coverage)
		}
		hasSuppression := strings.Contains(coverage.Skipped, "sync_span_authority: suppressed_spans=")
		if hasSuppression != check.suppressed {
			t.Fatalf("%s/%s suppression=%t want %t: %+v", check.family, check.table, hasSuppression, check.suppressed, coverage)
		}
		if (check.table == "app_startup" || check.table == "static_initalize") &&
			!strings.Contains(coverage.FieldSources["source_admission"], "R1b-C") {
			t.Fatalf("%s/%s no longer exposes the open R1b-C source-admission gap: %+v",
				check.family, check.table, coverage)
		}
	}
}

func TestTraceDBSyncSpanLegalCrossProducerNestingAndAdjacentRoundTrip(t *testing.T) {
	body, _ := exportTraceDBSyncSpanIntegrationFixture(t, "cross-producer-laminar",
		"INSERT INTO callstack VALUES (1, 1000000, 4000000, 1, NULL, 'outer', '', NULL, NULL, 0)",
		"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (-1, 1500000, 500000, 1, 1)",
		"INSERT INTO app_startup(rowid, start_time, end_time, start_name, ipid) VALUES (0, 2200000, 3000000, 5, 1)",
		"INSERT INTO static_initalize(rowid, start_time, end_time, so_name, ipid, tid) VALUES (1, 3000000, 3500000, 'libok.so', 1, 100)",
	)
	for _, want := range []string{
		"tracing_mark_write: B|100|outer",
		"tracing_mark_write: B|100|sys_1",
		"tracing_mark_write: B|100|AppStartup:coldStart",
		"tracing_mark_write: B|100|SoInit:libok.so",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("legal cross-producer span missing %q:\n%s", want, body)
		}
	}

	path := filepath.Join(t.TempDir(), "roundtrip.systrace")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatalf("parse legal cross-producer output: %v", err)
	}
	stats := tracequery.ComputeWindowStats(index, tracequery.Query{TimeStart: 0, TimeEnd: 0.01, TimeStartSet: true, TimeEndSet: true})
	wantDurations := map[string]float64{
		"outer":                4.0,
		"sys_1":                0.5,
		"AppStartup:coldStart": 0.8,
		"SoInit:libok.so":      0.5,
	}
	for name, want := range wantDurations {
		found := false
		for _, span := range stats.TraceSpans {
			if span.Name == name && math.Abs(span.DurationMs-want) < 0.000001 {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("roundtrip missing %s=%fms: %+v\n%s", name, want, stats.TraceSpans, body)
		}
	}
}

type traceDBSyncSpanBoundaryExporter func(context.Context, *traceDB, *traceDBRowSink,
	traceDBSchedulerAuthority, traceDBSchedulerRunningIndex, *traceDBSyncSpanAuthority, traceDBThreadIndex) (TraceDBCoverage, error)

type traceDBSyncSpanBoundaryCase struct {
	name          string
	family        string
	table         string
	createNormal  string
	signedRows    []string
	createShadow  string
	shadowRow     string
	createWithout string
	withoutRow    string
	wantTokens    []string
	export        traceDBSyncSpanBoundaryExporter
}

func traceDBSyncSpanBoundaryCases() []traceDBSyncSpanBoundaryCase {
	return []traceDBSyncSpanBoundaryCase{
		{
			name:         "syscall",
			family:       "slice",
			table:        "syscall",
			createNormal: "CREATE TABLE syscall (ts INT, dur INT, syscall_number INT, itid INT)",
			signedRows: []string{
				"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (-1, 1000000, 100000, 11, 1)",
				"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (0, 1200000, 100000, 12, 1)",
				"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1400000, 100000, 13, 1)",
			},
			createShadow:  "CREATE TABLE syscall (rowid TEXT, ts INT, dur INT, syscall_number INT, itid INT)",
			shadowRow:     "INSERT INTO syscall VALUES ('shadow', 1000000, 100000, 21, 1)",
			createWithout: "CREATE TABLE syscall (id INTEGER PRIMARY KEY, ts INT, dur INT, syscall_number INT, itid INT) WITHOUT ROWID",
			withoutRow:    "INSERT INTO syscall VALUES (1, 1000000, 100000, 31, 1)",
			wantTokens:    []string{"B|100|sys_11", "B|100|sys_12", "B|100|sys_13"},
			export: func(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, authority traceDBSchedulerAuthority, running traceDBSchedulerRunningIndex, spans *traceDBSyncSpanAuthority, _ traceDBThreadIndex) (TraceDBCoverage, error) {
				return exportTraceDBSyscall(ctx, tdb, sink, authority, running, spans)
			},
		},
		{
			name:         "app_startup",
			family:       "slice",
			table:        "app_startup",
			createNormal: "CREATE TABLE app_startup (start_time INT, end_time INT, start_name INT, ipid INT)",
			signedRows: []string{
				"INSERT INTO app_startup(rowid, start_time, end_time, start_name, ipid) VALUES (-1, 1000000, 1100000, 11, 1)",
				"INSERT INTO app_startup(rowid, start_time, end_time, start_name, ipid) VALUES (0, 1200000, 1300000, 12, 1)",
				"INSERT INTO app_startup(rowid, start_time, end_time, start_name, ipid) VALUES (1, 1400000, 1500000, 13, 1)",
			},
			createShadow:  "CREATE TABLE app_startup (rowid TEXT, start_time INT, end_time INT, start_name INT, ipid INT)",
			shadowRow:     "INSERT INTO app_startup VALUES ('shadow', 1000000, 1100000, 21, 1)",
			createWithout: "CREATE TABLE app_startup (id INTEGER PRIMARY KEY, start_time INT, end_time INT, start_name INT, ipid INT) WITHOUT ROWID",
			withoutRow:    "INSERT INTO app_startup VALUES (1, 1000000, 1100000, 31, 1)",
			wantTokens:    []string{"B|100|AppStartup:name-11", "B|100|AppStartup:name-12", "B|100|AppStartup:name-13"},
			export: func(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, _ traceDBSchedulerAuthority, _ traceDBSchedulerRunningIndex, spans *traceDBSyncSpanAuthority, index traceDBThreadIndex) (TraceDBCoverage, error) {
				return exportTraceDBAppStartup(ctx, tdb, sink, spans, index, map[int64]string{
					11: "name-11", 12: "name-12", 13: "name-13", 21: "shadow", 31: "without",
				})
			},
		},
		{
			name:         "static_initialize",
			family:       "slice",
			table:        "static_initalize",
			createNormal: "CREATE TABLE static_initalize (start_time INT, end_time INT, so_name TEXT, ipid INT, tid INT)",
			signedRows: []string{
				"INSERT INTO static_initalize(rowid, start_time, end_time, so_name, ipid, tid) VALUES (-1, 1000000, 1100000, 'lib11.so', 1, 100)",
				"INSERT INTO static_initalize(rowid, start_time, end_time, so_name, ipid, tid) VALUES (0, 1200000, 1300000, 'lib12.so', 1, 100)",
				"INSERT INTO static_initalize(rowid, start_time, end_time, so_name, ipid, tid) VALUES (1, 1400000, 1500000, 'lib13.so', 1, 100)",
			},
			createShadow:  "CREATE TABLE static_initalize (rowid TEXT, start_time INT, end_time INT, so_name TEXT, ipid INT, tid INT)",
			shadowRow:     "INSERT INTO static_initalize VALUES ('shadow', 1000000, 1100000, 'lib21.so', 1, 100)",
			createWithout: "CREATE TABLE static_initalize (id INTEGER PRIMARY KEY, start_time INT, end_time INT, so_name TEXT, ipid INT, tid INT) WITHOUT ROWID",
			withoutRow:    "INSERT INTO static_initalize VALUES (1, 1000000, 1100000, 'lib31.so', 1, 100)",
			wantTokens:    []string{"B|100|SoInit:lib11.so", "B|100|SoInit:lib12.so", "B|100|SoInit:lib13.so"},
			export: func(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, _ traceDBSchedulerAuthority, _ traceDBSchedulerRunningIndex, spans *traceDBSyncSpanAuthority, index traceDBThreadIndex) (TraceDBCoverage, error) {
				return exportTraceDBStaticInitialize(ctx, tdb, sink, spans, index)
			},
		},
	}
}

func traceDBRunSyncSpanBoundaryExporter(t *testing.T, test traceDBSyncSpanBoundaryCase,
	statements []string, addControl bool,
) (TraceDBCoverage, traceDBSyncSpanReport, string) {
	t.Helper()
	base := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 100, 'app')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 100, 1, 'app-main', 0, 1, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (1, 0, 10000000, 1, 'Running')",
	}
	path := createTraceDBFixture(t, append(base, statements...))
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	index, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	intervals, integrity, _, err := tdb.loadRunningIntervals(context.Background(), index)
	if err != nil {
		t.Fatal(err)
	}
	authority := newTraceDBSchedulerAuthority(index, traceDBLifecycleCollection{
		CreationComplete: true, TerminalComplete: true, ActivityComplete: true,
	})
	running := newTraceDBSchedulerRunningIndex(authority, intervals, integrity, nil)
	spans := newTraceDBTestSyncSpanAuthority(t)
	coverage, err := test.export(context.Background(), tdb, sink, authority, running, spans, index)
	if err != nil {
		t.Fatalf("export %s hidden-rowid fixture: %v coverage=%+v", test.name, err, coverage)
	}
	items := []TraceDBCoverage{coverage}
	if addControl {
		control := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerRegistration, 99, 100, 100, 2000, 2000, "control")
		if err := spans.submit(context.Background(), control); err != nil {
			t.Fatalf("submit post-%s control: %v", test.name, err)
		}
		items = append(items, TraceDBCoverage{Family: "metadata", Table: "thread", Found: true})
	}
	items, report, _ := finalizeTraceDBTestSyncSpans(t, sink, spans, items)
	rows := append([]traceDBStoredRow(nil), sink.rows...)
	sortTraceDBStoredRows(rows)
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, row.line)
	}
	return items[0], report, strings.Join(lines, "\n")
}

func TestTraceDBSyncSpanHiddenRowIDBoundaries(t *testing.T) {
	for _, test := range traceDBSyncSpanBoundaryCases() {
		t.Run(test.name+"/signed-negative-zero-positive", func(t *testing.T) {
			statements := append([]string{test.createNormal}, test.signedRows...)
			coverage, report, body := traceDBRunSyncSpanBoundaryExporter(t, test, statements, false)
			if coverage.RowsRead != 3 || coverage.RowsEmitted != 6 || report.SubmittedSpans != 3 || report.EmittedEndpoints != 6 {
				t.Fatalf("%s signed hidden rowids rejected: coverage=%+v report=%+v body=%q", test.name, coverage, report, body)
			}
			if coverage.FieldSources["stable_identity"] != test.table+".hidden_rowid; signed hidden rowid is used only for deterministic typed candidate identity/order" {
				t.Fatalf("%s signed hidden rowid provenance mismatch: %+v", test.name, coverage)
			}
			for _, want := range test.wantTokens {
				if !strings.Contains(body, want) {
					t.Fatalf("%s signed hidden rowid output missing %q:\n%s", test.name, want, body)
				}
			}
		})

		t.Run(test.name+"/declared-rowid-alias-shadow", func(t *testing.T) {
			coverage, report, body := traceDBRunSyncSpanBoundaryExporter(t, test,
				[]string{test.createShadow, test.shadowRow}, false)
			if coverage.RowsRead != 1 || coverage.RowsEmitted != 2 || report.SubmittedSpans != 1 || report.EmittedEndpoints != 2 {
				t.Fatalf("%s alias-shadow row rejected: coverage=%+v report=%+v body=%q", test.name, coverage, report, body)
			}
			if coverage.FieldSources["stable_identity"] != test.table+".hidden__rowid_; signed hidden rowid is used only for deterministic typed candidate identity/order" {
				t.Fatalf("%s did not fall through declared rowid to hidden _rowid_: %+v", test.name, coverage)
			}
		})

		for _, nonempty := range []bool{false, true} {
			label := "empty-no-drag"
			statements := []string{test.createWithout}
			if nonempty {
				label = "nonempty-fail-close"
				statements = append(statements, test.withoutRow)
			}
			t.Run(test.name+"/without-rowid-"+label, func(t *testing.T) {
				coverage, report, body := traceDBRunSyncSpanBoundaryExporter(t, test, statements, true)
				if test.name == "syscall" && nonempty {
					if coverage.Error != "" || coverage.RowsRead != 1 || coverage.RowsEmitted != 0 ||
						report.SubmittedSpans != 1 || report.EmittedEndpoints != 0 || report.PoisonedLanes != 1 || body != "" ||
						!strings.Contains(coverage.Skipped, "stable_row_identity_unavailable=1") ||
						!strings.Contains(coverage.Skipped, "exact_lane_poison_declarations=1") {
						t.Fatalf("syscall WITHOUT ROWID did not fail close its exact physical lane: coverage=%+v report=%+v body=%q",
							coverage, report, body)
					}
					return
				}
				if coverage.Error != "" || coverage.RowsEmitted != 0 || report.SubmittedSpans != 1 || report.EmittedEndpoints != 2 ||
					!strings.Contains(body, "B|100|control") {
					t.Fatalf("%s WITHOUT ROWID %s dragged authority: coverage=%+v report=%+v body=%q", test.name, label, coverage, report, body)
				}
				hasStableGap := strings.Contains(coverage.Skipped, "stable_row_identity_unavailable=")
				if nonempty {
					if coverage.RowsRead != 1 || !hasStableGap {
						t.Fatalf("%s nonempty WITHOUT ROWID did not fail close: %+v", test.name, coverage)
					}
				} else if coverage.RowsRead != 0 || hasStableGap {
					t.Fatalf("%s empty WITHOUT ROWID was not a local no-op: %+v", test.name, coverage)
				}
			})
		}
	}
}
